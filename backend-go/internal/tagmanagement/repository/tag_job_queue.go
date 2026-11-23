package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

const defaultTagJobMaxAttempts = 5

type TagJobRequest struct {
	ArticleID    uint
	FeedName     string
	CategoryName string
	ForceRetag   bool
	Reason       string
	Priority     int
}

type TagJobQueue struct {
	db *gorm.DB
}

func NewTagJobQueue(db *gorm.DB) *TagJobQueue {
	return &TagJobQueue{db: db}
}

// Enqueue 幂等入队：同文章已有 pending / leased 作业时原地更新（不新建行），
// 已 completed / failed 的文章可重新入队。
//
// 数据库级兜底（fix-feed-refresh-churn D4）：上面的「先查后插」在并发回调下会双插
// （文章 178599 实证），因此插入分支带 ON CONFLICT DO NOTHING——部分唯一索引
// uq_tag_jobs_article_active 把竞态收敛成「插入零行 → 按已存在分支同语义原地更新」，
// 对外契约仍是复用而非报错。
func (q *TagJobQueue) Enqueue(req TagJobRequest) error {
	now := time.Now()

	return q.db.Transaction(func(tx *gorm.DB) error {
		job, found, err := findActiveTagJob(tx, req.ArticleID)
		if err != nil {
			return err
		}
		if found {
			return applyTagJobUpdates(tx, job, req)
		}
		return insertTagJob(tx, req, now)
	})
}

// insertTagJob 插入一条 pending 作业；撞部分唯一索引（另一事务抢先插入了同文章的
// 活动作业并已提交）时插入零行，随即按「已存在」分支同语义原地更新。
//
// 两次插入机会覆盖一个极窄窗口：冲突行在「插入零行」与「重查」之间转了终态
// （worker 正好完成/失败），此时重查不到活动作业，再插一次接住它——否则这次入队
// 会静默丢一个作业。两试皆空（连转两次终态）时降级接受，见 rideTagJobConflict。
func insertTagJob(tx *gorm.DB, req TagJobRequest, now time.Time) error {
	return rideTagJobConflict(tagJobInsertOps{
		articleID: req.ArticleID,
		insert: func() (bool, error) {
			job := models.TagJob{
				ArticleID:            req.ArticleID,
				Status:               string(models.JobStatusPending),
				Priority:             req.Priority,
				AttemptCount:         0,
				MaxAttempts:          defaultTagJobMaxAttempts,
				AvailableAt:          now,
				FeedNameSnapshot:     req.FeedName,
				CategoryNameSnapshot: req.CategoryName,
				ForceRetag:           req.ForceRetag,
				Reason:               req.Reason,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&job)
			if res.Error != nil {
				return false, res.Error
			}
			return res.RowsAffected > 0, nil
		},
		findActive: func() (models.TagJob, bool, error) { return findActiveTagJob(tx, req.ArticleID) },
		reapply:    func(job models.TagJob) error { return applyTagJobUpdates(tx, job, req) },
	})
}

// tagJobInsertOps 把插入路径的数据库动作抽成闭包，让「两试皆空」兜底分支可以直接
// 测：该窗口只有微秒级宽，数据库内无法稳定构造（review R-3）。
type tagJobInsertOps struct {
	articleID  uint
	insert     func() (created bool, err error)
	findActive func() (models.TagJob, bool, error)
	reapply    func(job models.TagJob) error
}

// rideTagJobConflict 跑「插入 → 撞索引 → 按已存在复用」的两试循环。
//
// 两试皆空（冲突行在插入与重查之间连转两次终态）时按「已存在」语义降级接受：
// 只记一条告警并返回 nil，不把内部竞争暴露成调用方错误（spec「并发入队撞索引时
// SHALL 按复用处理，SHALL NOT 报错中断调用方」；review R-3）。
func rideTagJobConflict(ops tagJobInsertOps) error {
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
	logging.Warnf("enqueue tag job for article %d: insert conflicted without an active job; treating as already enqueued", ops.articleID)
	return nil
}

// findActiveTagJob 取回同文章尚未完成的作业。部分唯一索引保证至多一条；
// ORDER/LIMIT 保留原有的「取最新一条」语义（索引创建前的存量库）。
func findActiveTagJob(tx *gorm.DB, articleID uint) (models.TagJob, bool, error) {
	var active []models.TagJob
	if err := tx.Where("article_id = ? AND status IN ?", articleID, []string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Order("id DESC").
		Limit(1).
		Find(&active).Error; err != nil {
		return models.TagJob{}, false, err
	}
	if len(active) == 0 {
		return models.TagJob{}, false, nil
	}
	return active[0], true, nil
}

// applyTagJobUpdates 按「已存在」语义原地更新活动作业：force_retag 只升不降、
// 快照非空才覆盖、reason 以新值为准。
func applyTagJobUpdates(tx *gorm.DB, job models.TagJob, req TagJobRequest) error {
	updates := map[string]any{}
	if req.ForceRetag && !job.ForceRetag {
		updates["force_retag"] = true
	}
	if req.FeedName != "" {
		updates["feed_name_snapshot"] = req.FeedName
	}
	if req.CategoryName != "" {
		updates["category_name_snapshot"] = req.CategoryName
	}
	if req.Reason != "" {
		updates["reason"] = req.Reason
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&job).Updates(updates).Error
}

func (q *TagJobQueue) Claim(limit int, lease time.Duration) ([]models.TagJob, error) {
	if limit <= 0 {
		return []models.TagJob{}, nil
	}

	now := time.Now()
	var jobs []models.TagJob
	err := q.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.TagJob{}).
			Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", string(models.JobStatusLeased), now).
			Updates(map[string]any{
				"status":           string(models.JobStatusPending),
				"leased_at":        nil,
				"lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.TagJob{}).
			Where("status = ? AND attempt_count >= max_attempts", string(models.JobStatusPending)).
			Updates(map[string]any{
				"status":       string(models.JobStatusFailed),
				"available_at": now,
			}).Error; err != nil {
			return err
		}

		// 消费顺序（night-window-alignment D2）：新任务优先——priority 可插队，
		// 同顺位内按入队时间新→旧（created_at DESC；入队时间与到达时间近似单调
		// 同步，避免 join articles 的成本与 pub_date 缺失边界）。
		// 注：退避语义由上方 WHERE available_at <= now 保证（未来时刻不可 lease，
		// B3）；不把 available_at ASC 作排序键——Enqueue 时 available_at =
		// created_at，对 due 任务按它升序等于旧任务先出，会复活 FIFO。
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
			res := tx.Model(&models.TagJob{}).
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

func (q *TagJobQueue) MarkCompleted(jobID uint) error {
	return q.db.Model(&models.TagJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":           string(models.JobStatusCompleted),
			"leased_at":        nil,
			"lease_expires_at": nil,
			"last_error":       "",
		}).Error
}

func (q *TagJobQueue) MarkFailed(job models.TagJob, errMsg string, backoff time.Duration) error {
	status := string(models.JobStatusPending)
	availableAt := time.Now().Add(backoff)
	updates := map[string]any{
		"status":           status,
		"leased_at":        nil,
		"lease_expires_at": nil,
		"last_error":       errMsg,
		"available_at":     availableAt,
	}
	if job.AttemptCount >= job.MaxAttempts {
		updates["status"] = string(models.JobStatusFailed)
		updates["available_at"] = time.Now()
	}
	return q.db.Model(&models.TagJob{}).Where("id = ?", job.ID).Updates(updates).Error
}

func (q *TagJobQueue) Stats() (map[string]int64, error) {
	stats := map[string]int64{}
	for key, status := range map[string]string{
		"pending": string(models.JobStatusPending),
		"leased":  string(models.JobStatusLeased),
		"failed":  string(models.JobStatusFailed),
	} {
		var count int64
		if err := q.db.Model(&models.TagJob{}).Where("status = ?", status).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count %s tag jobs: %w", key, err)
		}
		stats[key] = count
	}
	return stats, nil
}
