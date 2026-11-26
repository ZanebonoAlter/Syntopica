package main

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// fillStoryEmbeddingBatchSize 限定一轮扫描的行数（keyset 分页，不带 OFFSET）。
// 树莓派上 200 行一批足够快，又不会让单次事务长时间占着表锁。
const fillStoryEmbeddingBatchSize = 200

// fillStoryTitleEmbedder 是标题向量计算的注入点（测试替换，避免真调模型）。
// 生产实现与服务端建线现算走同一个 operation（topicgraph.story_title_embedding），
// 保证存量回填与在线建线的向量在同一空间。
var fillStoryTitleEmbedder = service.EmbedStoryTitle

// fillStoryEmbeddingsStats 是 --fill-story-embeddings 的统计口径。
type fillStoryEmbeddingsStats struct {
	// Scanned 扫到的 board_stories.embedding IS NULL 行数（= 待补条数）。
	Scanned int64
	// Updated 成功写入向量的行数（dry-run 时恒为 0）。
	Updated int64
	// Skipped 标题为空或向量为空、无法补的行数（保持 NULL）。
	Skipped int64
	// Failed 单条失败数（逐条失败不中断，最后汇总）。
	Failed int64
}

// fillStoryEmbeddings 回填存量事件线的召回向量（enable-story-cross-day-continuation
// D13）：只处理 board_stories.embedding IS NULL 的行，用事件标题现算一次
// embedding 写回 **embedding 这一列**。
//
// 边界：
//   - 只写 embedding 列 —— 不动 first_seen_date / last_seen_date / thread_count
//     / participant_count / story_id 等任何其它列；
//   - 不覆盖已有向量（WHERE embedding IS NULL）→ 可重复执行，补过的行不再处理；
//   - dry-run 只统计条数：不发模型调用、不写库；
//   - 逐条失败不中断整轮回填，最后汇总失败数。
func fillStoryEmbeddings(ctx context.Context, db *gorm.DB, opts commandOptions) (fillStoryEmbeddingsStats, error) {
	stats := fillStoryEmbeddingsStats{}
	db = db.WithContext(ctx)
	type storyRow struct {
		ID    uint   `gorm:"column:id"`
		Title string `gorm:"column:title"`
	}
	var lastID uint
	for {
		var rows []storyRow
		if err := db.Raw(`
			SELECT id, title FROM board_stories
			WHERE embedding IS NULL AND id > ?
			ORDER BY id
			LIMIT ?`, lastID, fillStoryEmbeddingBatchSize).Scan(&rows).Error; err != nil {
			return stats, fmt.Errorf("scan board_stories without embedding: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			stats.Scanned++
			lastID = row.ID
			title := strings.TrimSpace(row.Title)
			if title == "" {
				stats.Skipped++
				logging.Warnf("fill story embeddings: story %d has no title (keep NULL embedding)", row.ID)
				continue
			}
			if opts.DryRun {
				// dry-run 不发模型调用、不写库：只统计待补条数。
				continue
			}
			vec, err := fillStoryTitleEmbedder(ctx, title)
			if err != nil {
				stats.Failed++
				logging.Warnf("fill story embeddings: embed story %d failed: %v", row.ID, err)
				continue
			}
			if len(vec) == 0 {
				stats.Skipped++
				logging.Warnf("fill story embeddings: embed story %d returned an empty vector", row.ID)
				continue
			}
			// 只写 embedding 列；WHERE embedding IS NULL 保证已补过的行不被覆盖。
			res := db.Exec(`UPDATE board_stories SET embedding = ? WHERE id = ? AND embedding IS NULL`,
				repository.FloatsToPgVector(vec), row.ID)
			if res.Error != nil {
				stats.Failed++
				logging.Warnf("fill story embeddings: update story %d failed: %v", row.ID, res.Error)
				continue
			}
			stats.Updated += res.RowsAffected
		}
	}
	return stats, nil
}
