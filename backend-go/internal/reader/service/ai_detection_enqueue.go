package service

import (
	"errors"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 疑似 AI 判定入队（add-ai-generated-article-detection design D4 / D10）
// ============================================================================

// 判定入队的来源取值（持久化在 detection_jobs.reason）。权威定义在 repository
// 包（队列与 force 语义的唯一真相源），这里别名导出，供域外调用方（admin 调度器）
// 直接引用。
const (
	// AIDetectionReasonFirecrawlCompleted 全文抓取成功完成（判定输入优先用全文）。
	AIDetectionReasonFirecrawlCompleted = repository.DetectionJobReasonFirecrawlCompleted
	// AIDetectionReasonFirecrawlFailed 全文抓取终态失败降级（输入回退 RSS 正文）。
	AIDetectionReasonFirecrawlFailed = repository.DetectionJobReasonFirecrawlFailed
	// AIDetectionReasonArticleIngested feed 未开全文抓取时文章入库。
	AIDetectionReasonArticleIngested = repository.DetectionJobReasonArticleIngested
	// AIDetectionReasonManualRecheck 用户显式重新判定（绕过人工结论跳过）。
	AIDetectionReasonManualRecheck = repository.DetectionJobReasonManualRecheck
)

// aiDetectionRecheckPriority 让显式重判插到自动入队的作业前面（用户等结果）。
const aiDetectionRecheckPriority = 10

// EnqueueAIDetectionJob 是判定入队的统一辅助函数：三个内容就绪节点（全文抓取
// 成功、全文抓取终态失败降级、feed 未开全文抓取时文章入库）都走它。
//
// 开关口径（design D4）：只有 feed.ai_detection_enabled 为 true 才入队；feed 为
// nil（订阅源不存在/查不到）按关闭处理——不 panic、不入队。同文章已有未完成作业
// 时由队列原地更新（幂等，spec「重复触发判定不产生重复作业」）。
//
// 调用方必须把返回的 error 当「仅记日志」处理：判定入队失败不得阻断文章入库、
// 抓取完成或打标（spec「判定失败不影响打标」）。
func EnqueueAIDetectionJob(db *gorm.DB, feed *models.Feed, articleID uint, reason string) error {
	if feed == nil || !feed.AIDetectionEnabled || articleID == 0 {
		return nil
	}
	if db == nil {
		return errors.New("ai detection: database is not initialized")
	}
	return repository.NewDetectionJobQueue(db).Enqueue(repository.DetectionJobRequest{
		ArticleID: articleID,
		Reason:    reason,
	})
}

// RequestAIDetectionRecheck 是显式重新判定入口（design D10）：入队一条
// manual_recheck 作业，绕过判定执行前的「人工结论优先」跳过，并覆盖人工结论。
//
// 这里刻意不判 feed.ai_detection_enabled：订阅源开关管的是「内容就绪时是否自动
// 判定」，用户显式点重判是独立动作，开关关闭时点击也应生效（否则入口静默失效）。
func RequestAIDetectionRecheck(db *gorm.DB, articleID uint) error {
	if articleID == 0 {
		return errors.New("ai detection: article id is required")
	}
	if db == nil {
		return errors.New("ai detection: database is not initialized")
	}
	return repository.NewDetectionJobQueue(db).Enqueue(repository.DetectionJobRequest{
		ArticleID: articleID,
		Reason:    repository.DetectionJobReasonManualRecheck,
		Priority:  aiDetectionRecheckPriority,
	})
}
