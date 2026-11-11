package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/tagmanagement/service/related"
)

// 相关事件只读端点（enable-story-cross-day-continuation tasks 1.2 / ui-design）：
// GET /api/semantic-boards/:id/tags/:tagId/related-events
//
// 契约（design D4/D5）：
//   - 只读：不调模型、不写缓存、不改标签/辅助/文章/日报/事件线；
//   - 同版块、近 30 天（标签 created_at）、阈值过滤后按分数降序 + id 升序取 10；
//   - 覆盖状态 ready / partial / not_ready 如实返回，缺索引不得伪装成空结果。

type relatedEventArticleDTO struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
}

type relatedEventItemDTO struct {
	TagID         uint                    `json:"tag_id"`
	Label         string                  `json:"label"`
	Description   string                  `json:"description"`
	Similarity    float64                 `json:"similarity"`
	SourceArticle *relatedEventArticleDTO `json:"source_article"`
}

type relatedEventsDTO struct {
	BoardID               uint                  `json:"board_id"`
	TagID                 uint                  `json:"tag_id"`
	Status                string                `json:"status"`
	Reason                string                `json:"reason,omitempty"`
	Threshold             float64               `json:"threshold"`
	WindowDays            int                   `json:"window_days"`
	WindowStart           string                `json:"window_start"`
	WindowEnd             string                `json:"window_end"`
	CandidateCount        int                   `json:"candidate_count"`
	IndexedCount          int                   `json:"indexed_count"`
	RepresentationVersion string                `json:"representation_version"`
	SpaceKey              string                `json:"space_key,omitempty"`
	Items                 []relatedEventItemDTO `json:"items"`
}

func (h *semanticBoardHandler) getRelatedEvents(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tagID, ok := parseUintParam(c, "tagId")
	if !ok {
		return
	}

	result, err := related.NewQuery(h.db).RelatedEvents(c.Request.Context(), boardID, tagID)
	if err != nil {
		switch {
		case errors.Is(err, related.ErrBoardNotFound), errors.Is(err, related.ErrTagNotInBoard):
			respondError(c, http.StatusNotFound, err)
		default:
			respondError(c, http.StatusInternalServerError, err)
		}
		return
	}
	respondOK(c, relatedEventsToDTO(result))
}

func relatedEventsToDTO(result *related.QueryResult) relatedEventsDTO {
	items := make([]relatedEventItemDTO, 0, len(result.Items))
	for _, item := range result.Items {
		dto := relatedEventItemDTO{
			TagID:       item.TagID,
			Label:       item.Label,
			Description: item.Description,
			Similarity:  item.Similarity,
		}
		if item.SourceArticle != nil {
			dto.SourceArticle = &relatedEventArticleDTO{ID: item.SourceArticle.ID, Title: item.SourceArticle.Title}
		}
		items = append(items, dto)
	}
	return relatedEventsDTO{
		BoardID:               result.BoardID,
		TagID:                 result.TagID,
		Status:                result.Status,
		Reason:                result.Reason,
		Threshold:             result.Threshold,
		WindowDays:            result.WindowDays,
		WindowStart:           result.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:             result.WindowEnd.UTC().Format(time.RFC3339),
		CandidateCount:        result.CandidateCount,
		IndexedCount:          result.IndexedCount,
		RepresentationVersion: result.RepresentationVersion,
		SpaceKey:              result.SpaceKey,
		Items:                 items,
	}
}
