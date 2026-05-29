package tagging

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
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

func (q *TagJobQueue) Enqueue(req TagJobRequest) error {
	now := time.Now()

	return q.db.Transaction(func(tx *gorm.DB) error {
		var existing []models.TagJob
		err := tx.Where("article_id = ? AND status IN ?", req.ArticleID, []string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
			Order("id DESC").
			Limit(1).
			Find(&existing).Error
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			updates := map[string]any{}
			if req.ForceRetag && !existing[0].ForceRetag {
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
			return tx.Model(&existing[0]).Updates(updates).Error
		}

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
		return tx.Create(&job).Error
	})
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

		if err := tx.Where("status = ? AND available_at <= ?", string(models.JobStatusPending), now).
			Order("priority DESC").
			Order("available_at ASC").
			Order("id ASC").
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
