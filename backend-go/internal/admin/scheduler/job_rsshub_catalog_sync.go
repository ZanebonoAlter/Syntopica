package scheduler

import (
	"context"
	"fmt"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/discovery"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/scheduler"
)

// RSSHubCatalogSyncJob 从自建 RSSHub 实例同步全量路由目录（design D2/D8）。
// 失败仅记日志（实例不可达时 SyncAll 内部已处理，保留既有目录）。
func RSSHubCatalogSyncJob(ctx context.Context) (*scheduler.JobResult, error) {
	svc := discovery.NewCatalogSyncService(repository.Repo.DB(), "")
	summary, err := svc.SyncAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("rsshub catalog sync failed: %w", err)
	}
	// 同步完成后生成新路由 embedding（design D8；best-effort，失败仅记日志不阻塞）。
	embedded, embErr := svc.EmbedPendingRoutes(ctx, airouter.NewRouter())
	if embErr != nil {
		logging.Infof("rsshub catalog sync: embed pending routes failed: %v", embErr)
	}
	return &scheduler.JobResult{
		Data: map[string]interface{}{
			"total":        summary.Total,
			"inserted":     summary.Inserted,
			"updated":      summary.Updated,
			"gone":         summary.Gone,
			"new_to_embed": summary.NewToEmbed,
			"embedded":     embedded,
		},
		Summary: fmt.Sprintf("catalog sync: total=%d inserted=%d updated=%d gone=%d embedded=%d",
			summary.Total, summary.Inserted, summary.Updated, summary.Gone, embedded),
	}, nil
}
