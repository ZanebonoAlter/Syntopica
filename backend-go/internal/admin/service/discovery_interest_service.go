package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/tracing"
)

// ── 兴趣记录列表（improve-discovery-recommendations High 1 修复，design D9）──
//
// GET /api/discovery/interests：逐条问答兴趣（不合成平均画像），按创建时间倒序分页。
// 字段形状对齐前端 front/app/types/discovery.ts 的 DiscoveryInterest（经 api/discovery.ts
// InterestPayload 归一）：id / query_text / board_id / board_label / status / created_at。
// board_id NULL = 未匹配版块（前端渲染为未匹配，不挂标签）；board_label 取 semantic_labels
// 名称（LEFT JOIN，NULL/空名回落 null）。
//
// status 语义映射：存储态 active|inactive|legacy_inactive → 展示态 active|faded|legacy
// （前端 InterestStatus 只认这三档；inactive = 窗口/成熟度让位的「已衰减」，legacy_inactive
// = 迁移来的旧 seed 行）。

// InterestListDefaultPageSize 是兴趣记录分页默认条数（design D9：默认 30、上限 100）。
const InterestListDefaultPageSize = 30

// InterestListMaxPageSize 是兴趣记录分页上限。
const InterestListMaxPageSize = 100

// InterestEntryView 是兴趣记录列表单条（api 边界 snake_case，数字 id 保持数字由前端转 string）。
type InterestEntryView struct {
	ID         uint      `json:"id"`
	QueryText  string    `json:"query_text"`
	BoardID    *uint     `json:"board_id"`
	BoardLabel *string   `json:"board_label"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// InterestListQuery 是兴趣记录列表查询参数（page 从 1 起；page>=1 时用 page_size 截段）。
type InterestListQuery struct {
	Page     int
	PageSize int
}

// InterestListResult 是兴趣记录分页结果。
type InterestListResult struct {
	Items []InterestEntryView
	Total int64
	Page  int
	Size  int
}

// ListInterestEntries 返回兴趣记录分页列表（created_at DESC，id DESC 稳定序）。
func (s *DiscoveryRunService) ListInterestEntries(ctx context.Context, q InterestListQuery) (*InterestListResult, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "DiscoveryRunService.ListInterestEntries")
	defer span.End()
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = InterestListDefaultPageSize
	}
	if q.PageSize > InterestListMaxPageSize {
		q.PageSize = InterestListMaxPageSize
	}

	var total int64
	if err := s.db.WithContext(ctx).Model(&models.DiscoveryInterestEntry{}).Count(&total).Error; err != nil {
		return nil, err
	}

	type row struct {
		ID         uint
		QueryText  string
		BoardID    *uint
		BoardLabel *string
		Status     string
		CreatedAt  time.Time
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT die.id, die.query_text, die.board_id, sl.label AS board_label,
		       die.status, die.created_at
		FROM discovery_interest_entries die
		LEFT JOIN semantic_labels sl ON sl.id = die.board_id
		ORDER BY die.created_at DESC, die.id DESC
		LIMIT ? OFFSET ?`, q.PageSize, (q.Page-1)*q.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]InterestEntryView, 0, len(rows))
	for _, r := range rows {
		label := r.BoardLabel
		if label != nil && *label == "" {
			label = nil // 空名等同未匹配，前端统一按 null 渲染
		}
		items = append(items, InterestEntryView{
			ID: r.ID, QueryText: r.QueryText, BoardID: r.BoardID, BoardLabel: label,
			Status: interestDisplayStatus(r.Status), CreatedAt: r.CreatedAt,
		})
	}
	return &InterestListResult{Items: items, Total: total, Page: q.Page, Size: q.PageSize}, nil
}

// interestDisplayStatus 把存储态状态映射为前端展示态（active|faded|legacy）。
func interestDisplayStatus(stored string) string {
	switch stored {
	case "active":
		return "active"
	case "inactive":
		return "faded"
	default:
		return "legacy"
	}
}
