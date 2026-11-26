package board

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// 归档口径（rework-workshop-into-radar-and-desk D5）：
//
// in_report 不是文章表上的冗余列，而是「查询窗口内任一分区线索的
// related_article_ids 是否包含该文章」的派生事实。口径只在本文件实现一次，
// handler 只消费 LoadReportMembership 的结果与 ApplyWeakEvidenceFilter 的
// 过滤条件，不得自行展开 jsonb 或复制匹配公式。
//
// 窗口两种形态：
//   - 显式日期（前端归档窗口）：Start/End 闭区间；
//   - 未给日期（旧调用 / 深链）：取该版块最近 MaxReports 份日报（默认 6）。
const defaultArchiveReportWindow = 6

// ReportWindow 是归档取数的日报窗口。Start/End 非空时按日期闭区间过滤；
// 两者都为空时回退「最近 MaxReports 份日报」。
type ReportWindow struct {
	Start      *time.Time
	End        *time.Time
	MaxReports int
}

// ReportMembership 是窗口内日报分区线索的文章集。
type ReportMembership struct {
	ArticleIDs       map[uint]struct{}
	ReportCount      int
	ThreadCount      int
	BadThreads       int
	LatestReportDate *time.Time
}

// LoadReportMembership 读取窗口内所有分区线索的 related_article_ids，展开成
// 文章 ID 集。损坏的 jsonb（历史 null 标量 / 非数组）按缺失处理并计入
// BadThreads，不中断整批——与信号取源同一降级哲学。
func LoadReportMembership(ctx context.Context, db *gorm.DB, boardID uint, win ReportWindow) (*ReportMembership, error) {
	if db == nil {
		return nil, fmt.Errorf("board archive: nil db")
	}
	sub := db.WithContext(ctx).
		Table("board_daily_reports").
		Select("id").
		Where("semantic_board_id = ?", boardID)
	// 必须以日期字符串下推，不能传 time.Time：period_date 是 date 列，PG 会话
	// 时区为 Asia/Shanghai 时，`date >= '2026-10-14T00:00:00Z'` 会把 date 抬到
	// 当天 00:00+08 再比较，起始日整份日报被静默排除（归档误报「未入日报」）。
	if win.Start != nil {
		sub = sub.Where("period_date >= ?", win.Start.Format("2006-01-02"))
	}
	if win.End != nil {
		sub = sub.Where("period_date <= ?", win.End.Format("2006-01-02"))
	}
	if win.Start == nil && win.End == nil {
		limit := win.MaxReports
		if limit <= 0 {
			limit = defaultArchiveReportWindow
		}
		sub = sub.Order("period_date DESC").Limit(limit)
	}

	var rows []struct {
		Refs       []byte     `gorm:"column:related_article_ids"`
		SectionID  uint       `gorm:"column:section_id"`
		ReportID   uint       `gorm:"column:report_id"`
		PeriodDate *time.Time `gorm:"column:period_date"`
	}
	if err := db.WithContext(ctx).
		Table("daily_report_threads AS t").
		Select("t.related_article_ids, t.section_id, r.id AS report_id, r.period_date").
		Joins("JOIN daily_report_sections s ON s.id = t.section_id").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Where("r.id IN (?)", sub).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("board archive: load report threads: %w", err)
	}

	out := &ReportMembership{ArticleIDs: make(map[uint]struct{})}
	reports := make(map[uint]struct{})
	for _, row := range rows {
		out.ThreadCount++
		reports[row.ReportID] = struct{}{}
		if row.PeriodDate != nil && (out.LatestReportDate == nil || row.PeriodDate.After(*out.LatestReportDate)) {
			date := *row.PeriodDate
			out.LatestReportDate = &date
		}
		ids, ok := decodeArchiveRefIDs(row.Refs)
		if !ok {
			out.BadThreads++
			continue
		}
		for _, id := range ids {
			out.ArticleIDs[id] = struct{}{}
		}
	}
	out.ReportCount = len(reports)
	if out.BadThreads > 0 {
		logging.Warnf("[board-archive] board=%d threads=%d bad_related_article_ids=%d (按缺失处理，in_report 可能少量漏标)",
			boardID, out.ThreadCount, out.BadThreads)
	}
	return out, nil
}

// decodeArchiveRefIDs 解析 jsonb 文章 ID 数组。返回 ok=false 表示该列不是
// 合法数组（JSON null 标量 / SQL NULL / 非数组字符串），调用方按空集处理。
func decodeArchiveRefIDs(raw []byte) ([]uint, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(trimmed, &elements); err != nil {
		return nil, false
	}
	ids := make([]uint, 0, len(elements))
	for _, element := range elements {
		text := strings.Trim(strings.TrimSpace(string(element)), `"`)
		id, err := strconv.ParseUint(text, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ids = append(ids, uint(id))
	}
	return ids, true
}

// ApplyWeakEvidenceFilter 把弱证据口径作用到文章查询上（articles 表别名固定
// 为 articles，handler 的主查询与计数查询共用）。弱证据 = 该文章在本版块
// 没有任何「强」匹配：强 = 非 max_sim；未通过方向校验的匹配默认
// 不算强（与 filtered_tags 默认隐藏一致），调用方要求展示方向未通过时
// includeDirectionMismatch=true。（downgraded 标记已于 2026-11-12 退役）
//
// 无匹配标签的文章同样落入弱证据（NOT EXISTS 对空集为真），这样归档的
// 「未打标」组与 `report_status=weak` 口径一致。
func ApplyWeakEvidenceFilter(q *gorm.DB, boardID uint, includeDirectionMismatch bool) *gorm.DB {
	strong := "COALESCE(wtbl.match_reason, '') <> 'max_sim'"
	if !includeDirectionMismatch {
		strong += " AND NOT COALESCE(wtbl.direction_mismatch, false)"
	}
	cond := `NOT EXISTS (
		SELECT 1 FROM article_topic_tags watt
		JOIN topic_tags wtt ON wtt.id = watt.topic_tag_id AND wtt.status = 'active'
		JOIN topic_tag_board_labels wtbl ON wtbl.topic_tag_id = wtt.id AND wtbl.semantic_board_id = ?
		WHERE watt.article_id = articles.id AND ` + strong + `
	)`
	return q.Where(cond, boardID)
}

// ArchiveEvidenceRank 是归档「首条标签」的强度序（0 最强）：
// direct_hit > composite_hit > hit_rate > weighted > max_sim/未知。
// 与 MatchTier 分开维护：MatchTier 服务于日报生成/旧工坊排序，改动会波及
// report 管线；归档只在自己的查询里用本函数，避免跨域口径漂移。
// downgraded 标记已于 2026-11-12 退役，max_sim 统一档。
func ArchiveEvidenceRank(matchReason string) int {
	switch matchReason {
	case "direct_hit":
		return 0
	case "composite_hit":
		return 1
	case "hit_rate":
		return 2
	case "weighted":
		return 3
	case "max_sim":
		return 4
	default:
		return 5
	}
}

// IsWeakEvidence 是弱证据口径的单条匹配版本（handler 组装 weak_evidence
// 字段用），必须与 ApplyWeakEvidenceFilter 的「强」定义同步。
func IsWeakEvidence(matchReason string) bool {
	return strings.TrimSpace(matchReason) == "max_sim"
}

// SortArticleIDs 把 ID 集转成稳定升序切片（IN/NOT IN 参数顺序可复现）。
func SortArticleIDs(set map[uint]struct{}) []uint {
	ids := make([]uint, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
