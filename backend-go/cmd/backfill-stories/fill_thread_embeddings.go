package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// fillThreadEmbeddingBatchSize 限定一轮向模型请求的线程标题条数。
// 线程向量是一次性批量调用（与在线建线 computeThreadFitDistances 同形），
// 32 条一批既不撑大单次请求体，也不至于把 30s 超时耗尽。
const fillThreadEmbeddingBatchSize = 32

// fillThreadTitleEmbedderFunc 批量嵌入一组线程标题，返回与输入等长的向量列表。
type fillThreadTitleEmbedderFunc func(ctx context.Context, titles []string) ([][]float64, error)

// fillThreadTitleEmbedder 是线程标题向量计算的注入点（测试替换，避免真调模型）。
// 生产实现走 airouter 的 embedding 路由（CapabilityEmbedding），与在线建线
// （service/daily_report_thread_fit.go:computeThreadFitDistances）同一模型路线
// → 存量回填与在线生成的线程向量在同一向量空间（召回 `embedding <=> ?` 口径可用）。
var fillThreadTitleEmbedder fillThreadTitleEmbedderFunc = defaultFillThreadEmbedder

// defaultFillThreadEmbedder 生产实现：一次请求嵌入整批线程标题
// （operation topicgraph.thread_title_embedding、CapabilityEmbedding、30s 超时）。
func defaultFillThreadEmbedder(ctx context.Context, titles []string) ([][]float64, error) {
	if len(titles) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
		Input:     titles,
		Operation: "topicgraph.thread_title_embedding",
		Metadata: map[string]any{
			"operation": "daily_report_thread_embedding_backfill",
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.Embeddings, nil
}

// fillThreadEmbeddingsStats 是 --fill-thread-embeddings 的统计口径。
type fillThreadEmbeddingsStats struct {
	// Scanned 扫到的 embedding IS NULL 且标题非空的线程行数（= 待补条数）。
	Scanned int64
	// Updated 成功写入向量的行数（dry-run 时恒为 0）。
	Updated int64
	// Skipped 扫到但补不了、保持 NULL 的行数（模型返回空向量等）。
	Skipped int64
	// Failed 失败行数（整批调用失败按行计；逐条失败不中断，末尾汇总）。
	Failed int64
	// Batches 实际发出的批量向量调用次数（dry-run 时恒为 0）。
	Batches int64
	// Excluded 因属于 watch 物化板块而未纳入回填的行数（默认排除；它们按设计
	// 本就没有向量，补了会把物化轨从「文章集合」召回切到「向量」召回）。
	Excluded int64
}

// fillThreadEmbeddings 回填存量日报线程的召回向量（enable-story-cross-day-continuation
// D13 补充）：只处理 daily_report_threads.embedding IS NULL 的行，用线程标题批量
// 现算 embedding 写回 **embedding 这一列**。
//
// 为什么必须补：线程向量是**召回的实际输入**（service/story_attribute.go:
// recallCandidates 拿 thread.Embedding 去查 `board_stories.embedding <=> ?`），
// 缺向量的线程只能走「文章重叠」兜底 → 召不到同一事件线。线程向量原本由
// computeThreadFitDistances 按「所属 section 有向量 + 标题非空」批量生成，
// section 无向量或批量调用失败时整批留 NULL。
//
// 边界：
//   - 只写 embedding 列 —— 不动 story_id / fit_distance / section_id /
//     report_id / title 等任何其它列；
//   - 不覆盖已有向量（WHERE embedding IS NULL）→ 可重复执行，补过的行不再处理；
//   - dry-run 只统计条数：不发模型调用、不写库；
//   - 逐批/逐条失败不中断整轮回填，最后汇总失败数。
//
// **watch 物化线程默认排除**：物化 section 按设计就没有 embedding
// （flow/daily-report.md 红线 16「物化 section 无 embedding（落库走 Omit 路径写
// NULL）」），其线程的 NULL 向量同理——物化轨的事件线归属走的是「跳过向量召回、
// 只比文章集合」的降级路径（add-event-story-heat design 降级 3）。给它们补向量
// 会把物化轨从「文章集合」切到「向量召回」= 新增一条召回面，且新生成的物化线程
// 永远不会有向量（SaveThreads 永远 Omit）→ 存量与增量不一致。故默认排除，
// 需要时用 --include-watch-threads 显式打开（覆盖设计态，请知悉后果）。
func fillThreadEmbeddings(ctx context.Context, db *gorm.DB, opts commandOptions) (fillThreadEmbeddingsStats, error) {
	stats := fillThreadEmbeddingsStats{}
	db = db.WithContext(ctx)

	type threadRow struct {
		ID    uint   `gorm:"column:id"`
		Title string `gorm:"column:title"`
	}

	// 过滤条件是否需要关联日报表（board / 日期窗口）。
	needReportJoin := opts.BoardID != nil || opts.ExplicitFrom || opts.ExplicitTo

	var lastID uint
	for {
		query := `SELECT t.id AS id, t.title AS title FROM daily_report_threads t
			LEFT JOIN daily_report_sections s ON s.id = t.section_id`
		args := make([]any, 0, 4)
		if needReportJoin {
			query += ` JOIN board_daily_reports r ON r.id = t.report_id`
		}
		query += ` WHERE t.embedding IS NULL AND TRIM(t.title) <> '' AND t.id > ?`
		args = append(args, lastID)
		filter, filterArgs := threadFilterClause(opts, !opts.IncludeWatchThreads)
		query += filter
		args = append(args, filterArgs...)
		query += ` ORDER BY t.id LIMIT ?`
		args = append(args, fillThreadEmbeddingBatchSize)

		var rows []threadRow
		if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
			return stats, fmt.Errorf("scan daily_report_threads without embedding: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].ID
		stats.Scanned += int64(len(rows))
		if opts.DryRun {
			// dry-run 不发模型调用、不写库：只统计待补条数。
			continue
		}

		titles := make([]string, 0, len(rows))
		for _, row := range rows {
			titles = append(titles, strings.TrimSpace(row.Title))
		}
		stats.Batches++
		vectors, err := fillThreadTitleEmbedder(ctx, titles)
		if err != nil {
			// 整批失败不中断：按行计入失败数，继续下一批（重跑可补）。
			stats.Failed += int64(len(rows))
			logging.Warnf("fill thread embeddings: batch embed failed (%d threads): %v", len(rows), err)
			continue
		}
		for i, row := range rows {
			if i >= len(vectors) || len(vectors[i]) == 0 {
				stats.Skipped++
				logging.Warnf("fill thread embeddings: thread %d got no vector", row.ID)
				continue
			}
			// 只写 embedding 列；WHERE embedding IS NULL 保证已补过的行不被覆盖。
			res := db.Exec(`UPDATE daily_report_threads SET embedding = ? WHERE id = ? AND embedding IS NULL`,
				repository.FloatsToPgVector(vectors[i]), row.ID)
			if res.Error != nil {
				stats.Failed++
				logging.Warnf("fill thread embeddings: update thread %d failed: %v", row.ID, res.Error)
				continue
			}
			stats.Updated += res.RowsAffected
		}
	}
	if !opts.IncludeWatchThreads {
		// 统计被排除的 watch 物化线程（它们按设计没有向量，留在 NULL）。
		stats.Excluded = countExcludedWatchThreads(ctx, db, opts)
		logging.Infof("fill thread embeddings: excluded %d watch-materialized threads (lane_tier 'watch_*') — they carry no embedding by design; use --include-watch-threads to override",
			stats.Excluded)
	}
	return stats, nil
}

// threadFilterClause 构造线程扫描的过滤子句（watch 排除 / 版块 / 日期窗口）。
// excludeWatch=true 时排除 watch 物化板块的线程。
func threadFilterClause(opts commandOptions, excludeWatch bool) (string, []any) {
	clause := ""
	args := make([]any, 0, 3)
	if excludeWatch {
		// 物化板块以 lane_tier 前缀 watch_ 为唯一判据（flow/daily-report.md
		// 红线 16）；历史物化 section 的 watch_id 可能为 NULL，故看 lane_tier。
		// ESCAPE 写法：PG 默认把 \ 当转义符、SQLite 不认 → 用 ESCAPE 关键字
		// 写死转义符，两库行为一致。
		clause += ` AND (s.lane_tier IS NULL OR s.lane_tier NOT LIKE 'watch!_%' ESCAPE '!')`
	}
	if opts.BoardID != nil {
		clause += ` AND r.semantic_board_id = ?`
		args = append(args, *opts.BoardID)
	}
	// 日期窗口只在**显式给出** --from/--to 时生效（默认窗口是「最近 30 天」，
	// 拿默认窗口当回填范围会把更早的存量漏掉）。
	if opts.ExplicitFrom {
		clause += ` AND r.period_date >= ?`
		args = append(args, opts.From)
	}
	if opts.ExplicitTo {
		// 半开区间：含当日（PG DATE 与 SQLite 日期列通用口径，
		// 与 service/story_backfill.go:backfillReportQuery 一致）。
		clause += ` AND r.period_date < ?`
		args = append(args, opts.To.AddDate(0, 0, 1).Format(time.DateOnly))
	}
	return clause, args
}

// countExcludedWatchThreads 统计因属于 watch 物化板块而未纳入回填的线程数
// （只读；同口径的 board / 日期窗口过滤）。
func countExcludedWatchThreads(ctx context.Context, db *gorm.DB, opts commandOptions) int64 {
	query := `SELECT count(*) FROM daily_report_threads t
		LEFT JOIN daily_report_sections s ON s.id = t.section_id`
	if opts.BoardID != nil || opts.ExplicitFrom || opts.ExplicitTo {
		query += ` JOIN board_daily_reports r ON r.id = t.report_id`
	}
	query += ` WHERE t.embedding IS NULL AND TRIM(t.title) <> '' AND s.lane_tier LIKE 'watch!_%' ESCAPE '!'`
	filter, filterArgs := threadFilterClause(opts, false)
	query += filter
	var count int64
	if err := db.WithContext(ctx).Raw(query, filterArgs...).Scan(&count).Error; err != nil {
		logging.Warnf("fill thread embeddings: count excluded watch threads failed: %v", err)
		return 0
	}
	return count
}
