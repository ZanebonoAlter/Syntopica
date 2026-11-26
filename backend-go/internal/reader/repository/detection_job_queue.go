package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

const defaultDetectionJobMaxAttempts = 5

// 判定作业的入队来源（models.DetectionJob.Reason）。取值同时承载「是否显式重判」：
// manual_recheck 是用户显式触发的重新判定，判定执行时绕过「人工结论优先」的跳过
// （design D10）；其余三值来自三个内容就绪节点（design D4）。
const (
	// DetectionJobReasonFirecrawlCompleted 全文抓取成功完成（输入优先用全文）。
	DetectionJobReasonFirecrawlCompleted = "firecrawl_completed"
	// DetectionJobReasonFirecrawlFailed 全文抓取终态失败降级（输入回退 RSS 正文）。
	DetectionJobReasonFirecrawlFailed = "firecrawl_failed"
	// DetectionJobReasonArticleIngested feed 未开全文抓取时文章入库。
	DetectionJobReasonArticleIngested = "article_ingested"
	// DetectionJobReasonManualRecheck 用户显式重新判定（绕过人工结论跳过）。
	DetectionJobReasonManualRecheck = "manual_recheck"
)

// IsForceDetectionReason 报告某作业是否为显式重判。
//
// models.DetectionJob 没有独立的 force 列（本 change 冻结 models 包），force 语义
// 由 Reason 承载：只有 manual_recheck 会绕过 worker 的「人工结论优先」跳过。
func IsForceDetectionReason(reason string) bool {
	return reason == DetectionJobReasonManualRecheck
}

// DetectionJobRequest 是一次判定入队请求。
type DetectionJobRequest struct {
	ArticleID uint
	// Reason 是入队来源（见 DetectionJobReason* 常量）。manual_recheck 表示
	// 显式重判。
	Reason string
	// Priority 越大越先被领取；普通内容就绪节点用 0，显式重判用更高值插队。
	Priority int
}

// DetectionJobQueue 是「疑似 AI 生成」判定作业队列（add-ai-generated-article-
// detection design D1）。骨架与 TagJobQueue 同构：一篇一作业、Enqueue 同文章幂等
// （已有未完成作业时原地更新）、Claim 条件 UPDATE 乐观锁 + 租约、MarkFailed 达重试
// 上限后终止为失败。
//
// 与 tag_jobs / firecrawl_jobs 完全独立：不共表、不共用 worker，判定失败不影响打标。
type DetectionJobQueue struct {
	db *gorm.DB
}

func NewDetectionJobQueue(db *gorm.DB) *DetectionJobQueue {
	return &DetectionJobQueue{db: db}
}

// Enqueue 幂等入队：同文章已有 pending / leased 作业时原地更新（不新建行），
// 已 completed / failed 的文章可重新入队。
//
// 原地更新的取向：reason 取新值（显式重判会把 reason 提升为 manual_recheck）、
// priority 只升不降、已到期的租约保持不动（正在跑的作业不被重置）；重试计数只在
// 显式重判时归零（review R-15：用户的重新判定是一次新尝试，不该继承此前失败烧掉的
// 预算，否则连点几次就把作业提前推到 max_attempts 判死）。
//
// 数据库级兜底（与 tag_jobs 同型，fix-feed-refresh-churn D4）：上面的「先查后插」
// 在并发节点（抓取完成 / 抓取失败 / 入库即判定可能同秒落到同篇文章）下会双插，
// 插入分支带 ON CONFLICT DO NOTHING——部分唯一索引 uq_detection_jobs_article_active
// 把竞态收敛成「插入零行 → 按已存在分支同语义原地更新」。
func (q *DetectionJobQueue) Enqueue(req DetectionJobRequest) error {
	if q == nil || q.db == nil {
		return gorm.ErrInvalidDB
	}
	if req.ArticleID == 0 {
		return errors.New("detection job: article id is required")
	}

	now := time.Now()

	return q.db.Transaction(func(tx *gorm.DB) error {
		job, found, err := findActiveDetectionJob(tx, req.ArticleID)
		if err != nil {
			return err
		}
		if found {
			return applyDetectionJobUpdates(tx, job, req)
		}
		return insertDetectionJob(tx, req, now)
	})
}

// insertDetectionJob 插入一条 pending 作业；撞部分唯一索引（另一事务抢先插入了同
// 文章的活动作业并已提交）时插入零行，随即按「已存在」分支同语义原地更新。两次
// 插入机会覆盖一个极窄窗口：冲突行在「插入零行」与「重查」之间转了终态（worker
// 正好完成/失败），否则这次入队会静默丢一个作业。两试皆空时降级接受，见
// rideDetectionJobConflict。
func insertDetectionJob(tx *gorm.DB, req DetectionJobRequest, now time.Time) error {
	return rideDetectionJobConflict(detectionJobInsertOps{
		articleID: req.ArticleID,
		insert: func() (bool, error) {
			job := models.DetectionJob{
				ArticleID:    req.ArticleID,
				Status:       string(models.JobStatusPending),
				Priority:     req.Priority,
				AttemptCount: 0,
				MaxAttempts:  defaultDetectionJobMaxAttempts,
				AvailableAt:  now,
				Reason:       req.Reason,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&job)
			if res.Error != nil {
				return false, res.Error
			}
			return res.RowsAffected > 0, nil
		},
		findActive: func() (models.DetectionJob, bool, error) {
			return findActiveDetectionJob(tx, req.ArticleID)
		},
		reapply: func(job models.DetectionJob) error { return applyDetectionJobUpdates(tx, job, req) },
	})
}

// detectionJobInsertOps 把插入路径的数据库动作抽成闭包，让「两试皆空」兜底分支可以
// 直接测：该窗口只有微秒级宽，数据库内无法稳定构造（review R-3）。
type detectionJobInsertOps struct {
	articleID  uint
	insert     func() (created bool, err error)
	findActive func() (models.DetectionJob, bool, error)
	reapply    func(job models.DetectionJob) error
}

// rideDetectionJobConflict 跑「插入 → 撞索引 → 按已存在复用」的两试循环。
//
// 两试皆空（冲突行在插入与重查之间连转两次终态）时按「已存在」语义降级接受：
// 只记一条告警并返回 nil，不把内部竞争暴露成调用方错误——判定入队失败本就不该
// 阻断入库/抓取/打标（spec「入队失败只记日志」）。
func rideDetectionJobConflict(ops detectionJobInsertOps) error {
	for attempt := 0; attempt < 2; attempt++ {
		created, err := ops.insert()
		if err != nil {
			return err
		}
		if created {
			return nil
		}

		active, found, err := ops.findActive()
		if err != nil {
			return err
		}
		if found {
			return ops.reapply(active)
		}
	}
	logging.Warnf("enqueue detection job for article %d: insert conflicted without an active job; treating as already enqueued", ops.articleID)
	return nil
}

// findActiveDetectionJob 取回同文章尚未完成的作业。部分唯一索引保证至多一条；
// ORDER/LIMIT 保留原有的「取最新一条」语义（索引创建前的存量库）。
func findActiveDetectionJob(tx *gorm.DB, articleID uint) (models.DetectionJob, bool, error) {
	var active []models.DetectionJob
	if err := tx.Where("article_id = ? AND status IN ?", articleID,
		[]string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Order("id DESC").
		Limit(1).
		Find(&active).Error; err != nil {
		return models.DetectionJob{}, false, err
	}
	if len(active) == 0 {
		return models.DetectionJob{}, false, nil
	}
	return active[0], true, nil
}

// applyDetectionJobUpdates 按「已存在」语义原地更新活动作业：reason 取新值、
// priority 只升不降，显式重判另外把重试预算归零。
func applyDetectionJobUpdates(tx *gorm.DB, job models.DetectionJob, req DetectionJobRequest) error {
	updates := map[string]any{}
	if req.Reason != "" && req.Reason != job.Reason {
		updates["reason"] = req.Reason
	}
	if req.Priority > job.Priority {
		updates["priority"] = req.Priority
	}
	// 显式重判不看 reason 是否变化：用户再点一次就该重新拿到完整预算。
	if IsForceDetectionReason(req.Reason) && job.AttemptCount > 0 {
		updates["attempt_count"] = 0
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&job).Updates(updates).Error
}

// Claim 领取一批到期作业并把它们标记为 leased（attempt_count + 1、写租约）。
//
// 三步与 TagJobQueue 同款：① 回收租约过期的 leased 作业为 pending；② 把重试次数
// 已达上限的 pending 作业终止为 failed；③ 取到期 pending 作业，逐条用条件 UPDATE
// （where id + status=pending）抢锁——RowsAffected=0 说明被别的 worker 抢先，跳过。
// 消费顺序：priority DESC → created_at DESC（显式重判可插队）。
func (q *DetectionJobQueue) Claim(limit int, lease time.Duration) ([]models.DetectionJob, error) {
	if limit <= 0 {
		return []models.DetectionJob{}, nil
	}
	if q == nil || q.db == nil {
		return nil, gorm.ErrInvalidDB
	}

	now := time.Now()
	var jobs []models.DetectionJob

	err := q.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.DetectionJob{}).
			Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", string(models.JobStatusLeased), now).
			Updates(map[string]any{
				"status":           string(models.JobStatusPending),
				"leased_at":        nil,
				"lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.DetectionJob{}).
			Where("status = ? AND attempt_count >= max_attempts", string(models.JobStatusPending)).
			Updates(map[string]any{
				"status":       string(models.JobStatusFailed),
				"available_at": now,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("status = ? AND available_at <= ?", string(models.JobStatusPending), now).
			Order("priority DESC").
			Order("created_at DESC").
			Limit(limit).
			Find(&jobs).Error; err != nil {
			return err
		}

		leaseExpiry := now.Add(lease)
		claimed := jobs[:0]
		for i := range jobs {
			job := &jobs[i]
			res := tx.Model(&models.DetectionJob{}).
				Where("id = ? AND status = ?", job.ID, string(models.JobStatusPending)).
				Updates(map[string]any{
					"status":           string(models.JobStatusLeased),
					"attempt_count":    gorm.Expr("attempt_count + 1"),
					"leased_at":        now,
					"lease_expires_at": leaseExpiry,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				continue
			}
			job.Status = string(models.JobStatusLeased)
			job.AttemptCount++
			job.LeasedAt = &now
			job.LeaseExpiresAt = &leaseExpiry
			claimed = append(claimed, *job)
		}
		jobs = claimed
		return nil
	})
	return jobs, err
}

// MarkCompleted 把作业标记为完成并释放租约。
func (q *DetectionJobQueue) MarkCompleted(jobID uint) error {
	if q == nil || q.db == nil {
		return gorm.ErrInvalidDB
	}
	return q.db.Model(&models.DetectionJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":           string(models.JobStatusCompleted),
			"leased_at":        nil,
			"lease_expires_at": nil,
			"last_error":       "",
		}).Error
}

// MarkFailed 记录失败并按退避重新排队；尝试次数达上限时终止为 failed。
//
// 终止后文章判定状态保持原值（未判定）——判定失败不写结论（spec「判定失败的静默
// 降级」）。
func (q *DetectionJobQueue) MarkFailed(job models.DetectionJob, errMsg string, backoff time.Duration) error {
	if q == nil || q.db == nil {
		return gorm.ErrInvalidDB
	}

	updates := map[string]any{
		"status":           string(models.JobStatusPending),
		"leased_at":        nil,
		"lease_expires_at": nil,
		"last_error":       errMsg,
		"available_at":     time.Now().Add(backoff),
	}
	if job.AttemptCount >= job.MaxAttempts {
		updates["status"] = string(models.JobStatusFailed)
		updates["available_at"] = time.Now()
	}
	return q.db.Model(&models.DetectionJob{}).Where("id = ?", job.ID).Updates(updates).Error
}
