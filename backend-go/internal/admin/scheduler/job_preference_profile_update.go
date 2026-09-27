package scheduler

import (
	"context"
	"fmt"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/discovery"
	"syntopica-backend/internal/platform/scheduler"
)

// PreferenceProfileUpdateJob 全量重算偏好向量（design D1/D8，零 LLM/embedding）。
// 失败仅记日志（框架层），不阻塞同轮兄弟 job。
func PreferenceProfileUpdateJob(ctx context.Context) (*scheduler.JobResult, error) {
	svc := discovery.NewPreferenceProfileService(repository.Repo.DB())
	summary, err := svc.RecomputeAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("preference profile update failed: %w", err)
	}
	return &scheduler.JobResult{
		Data: map[string]interface{}{
			"boards_computed": summary.BoardsComputed,
			"tags_used":       summary.TagsUsed,
			"article_count":   summary.ArticleCount,
		},
		Summary: fmt.Sprintf("recomputed preference profile: boards=%d tags=%d articles=%d",
			summary.BoardsComputed, summary.TagsUsed, summary.ArticleCount),
	}, nil
}
