package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// 辅助标签约束的读写端点（topic-lane-auxiliary-label-constraint tasks 5.1~5.3）。
//
// 契约要点（spec「约束管理接口」）：
//   - GET 只回元信息，**SHALL NOT** 返回 `term_embedding` 原始向量；
//   - PUT 是**全量覆盖**语义且幂等（相同请求重提结果相同；删除=在某次提交里省略）；
//   - 话题不存在 → 404；重复 term / 非法 mode / 越界 margin / 越界 max_distance
//     → 400 且 **SHALL NOT** 部分写入；
//   - 新增约束时顺带计算并持久化锤向量，计算失败返回错误且**不落库**。

// laneConstraintResponse is the API shape of one constraint. Deliberately a
// separate struct (not the model) so `term_embedding` can never leak into the
// response by accident.
type laneConstraintResponse struct {
	ID             uint     `json:"id"`
	TopicID        uint     `json:"topic_id"`
	Term           string   `json:"term"`
	Mode           string   `json:"mode"`
	Margin         *float64 `json:"margin"`
	MaxDistance    *float64 `json:"max_distance"`
	EmbeddingModel string   `json:"embedding_model"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func toLaneConstraintResponses(rows []repository.TopicLaneConstraint) []laneConstraintResponse {
	out := make([]laneConstraintResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, laneConstraintResponse{
			ID:             row.ID,
			TopicID:        row.TopicID,
			Term:           row.Term,
			Mode:           row.Mode,
			Margin:         row.Margin,
			MaxDistance:    row.MaxDistance,
			EmbeddingModel: row.EmbeddingModel,
			CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:      row.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out
}

// getTopicLaneConstraints handles GET /api/daily-reports/topics/:id/constraints.
func getTopicLaneConstraints(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return // parseTopicID already wrote the 400
	}
	if _, err := repository.Repo.GetTopic(topicID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
			return
		}
		logging.Errorf("load topic %d for constraints: %v", topicID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load topic"})
		return
	}
	rows, err := repository.Repo.ListTopicLaneConstraints(topicID)
	if err != nil {
		logging.Errorf("list topic %d constraints: %v", topicID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list constraints"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"constraints": toLaneConstraintResponses(rows),
	}})
}

// respondLaneConstraintError maps a constraint write failure onto HTTP: client
// payload problems answer 400 with the message verbatim, anchor computation
// failures answer 503 with the actionable Chinese reason, everything else stays
// a generic 500 (details only in the log).
//
// 为什么锤算不出来给 503 而不是 500：这条链路的失败源几乎总在 embedding 侧
// （服务没起 / 断网 / 换模型导致维度不符），客户端处置一致——修好 embedding
// 再重试（PUT 幂等、重试安全）。带原因的文案避免用户对着
// "failed to compute constraint anchors" 猜是短词填错还是服务挂了。
func respondLaneConstraintError(c *gin.Context, err error, logPrefix, fallback string) {
	var badRequest *service.LaneConstraintValidationError
	if errors.As(err, &badRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": badRequest.Error()})
		return
	}
	var anchorErr *service.LaneConstraintAnchorError
	if errors.As(err, &anchorErr) {
		logging.Errorf("%s: %v", logPrefix, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": anchorErr.UserMessage()})
		return
	}
	logging.Errorf("%s: %v", logPrefix, err)
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fallback})
}

// putTopicLaneConstraints handles PUT /api/daily-reports/topics/:id/constraints.
// Body: {"constraints":[{"term":"以色列","mode":"required","margin":null,"max_distance":null}]}
// — full replacement (omit an entry to delete it), idempotent.
func putTopicLaneConstraints(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	var req struct {
		Constraints []service.LaneConstraintRequest `json:"constraints"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	if _, err := repository.Repo.GetTopic(topicID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
			return
		}
		logging.Errorf("load topic %d for constraint replace: %v", topicID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load topic"})
		return
	}

	rows, err := service.ReplaceTopicLaneConstraints(c.Request.Context(), topicID, req.Constraints)
	if err != nil {
		// 校验错 → 400（不落库）；锚算不出（embedding 不可达 / 维度不符）→ 503
		// 且原文不变（什么都没写）。
		respondLaneConstraintError(c, err, fmt.Sprintf("replace topic %d constraints", topicID), "failed to persist constraints")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"constraints": toLaneConstraintResponses(rows),
	}})
}
