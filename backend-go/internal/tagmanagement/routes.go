package tagging

import (
	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
)

// RegisterRoutes registers all tagmanagement module routes under the given router group.
func RegisterRoutes(rg *gin.RouterGroup) {
	RegisterEmbeddingQueueRoutes(rg)
	RegisterMergeReembeddingQueueRoutes(rg)
	RegisterTagQueueRoutes(rg)
	RegisterTagManagementRoutes(rg)
	RegisterWatchedTagsRoutes(rg)
	RegisterTagMergePreviewRoutes(rg)
	RegisterSemanticBoardRoutes(rg)

	// 辅助标签描述修复链（board-health-triage-advice tasks 2.2/2.5/2.6）：
	// 四个资源（context / 创建 / 任务查询 / 明确重试）用**同一个**修复服务实例
	// ——后台执行器也用它建任务，两边必须是同一扇门。
	// 服务未初始化时（只可能发生在没跑 InitDescriptionRepairChain 的嵌入方）
	// 记一条错误日志；app 启动路径会在初始化失败时直接 Fatalf，不会走到这里。
	if svc := DescriptionRepairService(); svc != nil {
		RegisterAuxiliaryDescriptionRepairRoutes(rg, svc)
	} else {
		logging.Errorf("description-repair chain is not initialized: the repair endpoints are unavailable")
	}
}
