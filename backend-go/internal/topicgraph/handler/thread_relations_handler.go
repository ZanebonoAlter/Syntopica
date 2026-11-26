package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 线索相关事件 HTTP 端点（replace-lane-lifeline-with-topic-events D2）─────
//
//	GET /api/daily-reports/:id/thread-relations?days=7|30
//	    → 该期日报**所有**线索的相关线索推荐（thread 向量口径），按 thread_id
//	      分组；一次请求覆盖整份日报，前端不为每条线索各发一个请求
//
// 契约要点：
//   - `days` 只接受 7 / 30；其它值（空串、14、365、abc、-1）一律回落 7 并在
//     响应 `days` 回显实际生效值——**不报 400**（spec「非法天数回落并回显」）。
//   - `reportId` 不存在 → 404。
//   - 窗口 `[report_date-days, report_date-1]` 不含当期（当天的线索同屏可见）。
//   - 三态 `status`：`partial` 当且仅当有当期线索因缺向量被跳过
//     （`skipped_no_embedding > 0`）；`empty` 当且仅当没有任何推荐且 `threads`
//     为 `{}`。旧的「悬空标签 / stale_candidates」态随标签口径一起删除。
//   - 整条路径只读：不调模型、不计算/写 embedding、不写任何表（spec「推荐读
//     模型只读」）。

// RegisterThreadRelationRoutes wires the thread-relations endpoint under /api.
// NOTE: the report-scoped route reuses gin's existing ":id" wildcard name —
// gin forbids mixing param names at the same path position.
func RegisterThreadRelationRoutes(api *gin.RouterGroup) {
	api.GET("/daily-reports/:id/thread-relations", getThreadRelations)
}

// parseThreadRelationDays resolves the raw `days` query value to the window
// actually used: 7 / 30 pass through, everything else (including unparsable
// input) falls back to 7 — never a 400.
func parseThreadRelationDays(raw string) int {
	days, err := strconv.Atoi(raw)
	if err != nil {
		return repository.NormalizeThreadRelationDays(0)
	}
	return repository.NormalizeThreadRelationDays(days)
}

// getThreadRelations handles GET /api/daily-reports/:id/thread-relations.
func getThreadRelations(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}
	resp, err := repository.Repo.BuildThreadRelations(uint(reportID), parseThreadRelationDays(c.Query("days")))
	if err != nil {
		if errors.Is(err, repository.ErrThreadRelationReportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report not found"})
			return
		}
		logging.Errorf("build thread relations for report %d: %v", reportID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load thread relations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}
