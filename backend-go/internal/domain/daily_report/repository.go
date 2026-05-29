package daily_report

import (
	"fmt"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
)

// SaveReport saves a daily report and its sections, replacing any existing
// report for the same board and date.
func SaveReport(report *BoardDailyReport, sections []DailyReportSection) error {
	report.PeriodDate = normalizeReportDate(report.PeriodDate)
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Upsert report: find existing by (semantic_board_id, period_date)
		var existing BoardDailyReport
		err := tx.Where("semantic_board_id = ? AND period_date = ?",
			report.SemanticBoardID,
			report.PeriodDate.Format("2006-01-02")).
			First(&existing).Error

		if err == nil {
			// Update existing report
			report.ID = existing.ID
			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"title":                     report.Title,
				"summary":                   report.Summary,
				"highlights":                report.Highlights,
				"dynamics":                  report.Dynamics,
				"article_count":             report.ArticleCount,
				"event_tag_count":           report.EventTagCount,
				"cluster_count":             report.ClusterCount,
				"status":                    report.Status,
				"raw_clusters":              report.RawClusters,
				"prev_report_id":            report.PrevReportID,
				"generation_prompt_version": report.GenerationPromptVersion,
			}).Error; err != nil {
				return fmt.Errorf("update report: %w", err)
			}
			// Delete old sections
			if err := tx.Where("report_id = ?", existing.ID).Delete(&DailyReportSection{}).Error; err != nil {
				return fmt.Errorf("delete old sections: %w", err)
			}
		} else {
			// Create new report
			if err := tx.Create(report).Error; err != nil {
				return fmt.Errorf("create report: %w", err)
			}
		}

		// Insert new sections
		for i := range sections {
			sections[i].ReportID = report.ID
		}
		if len(sections) > 0 {
			if err := tx.CreateInBatches(sections, 20).Error; err != nil {
				return fmt.Errorf("create sections: %w", err)
			}
		}

		logging.Infof("daily-report: saved report %d for board %d on %s (%d sections)",
			report.ID, report.SemanticBoardID, report.PeriodDate.Format("2006-01-02"), len(sections))
		return nil
	})
}

// GetReport retrieves a single daily report with its sections.
func GetReport(boardID uint, date time.Time) (*BoardDailyReport, error) {
	reportDate := normalizeReportDate(date)

	var report BoardDailyReport
	err := database.DB.Where("semantic_board_id = ? AND period_date = ?",
		boardID, reportDate.Format("2006-01-02")).
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("cluster_index ASC")
		}).
		First(&report).Error
	if err != nil {
		return nil, fmt.Errorf("report not found for board %d on %s: %w", boardID, date.Format("2006-01-02"), err)
	}
	return &report, nil
}

// GetReportByID retrieves a single daily report by its primary key.
func GetReportByID(id uint) (*BoardDailyReport, error) {
	var report BoardDailyReport
	err := database.DB.Where("id = ?", id).
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("cluster_index ASC")
		}).
		First(&report).Error
	if err != nil {
		return nil, fmt.Errorf("report %d not found: %w", id, err)
	}
	return &report, nil
}

// ReportListItem is a summary view for list endpoints.
type ReportListItem struct {
	ID              uint      `json:"id"`
	SemanticBoardID uint      `json:"semantic_board_id"`
	PeriodDate      string    `json:"period_date"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	ArticleCount    int       `json:"article_count"`
	EventTagCount   int       `json:"event_tag_count"`
	ClusterCount    int       `json:"cluster_count"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// ListReports returns recent reports for a board.
func ListReports(boardID uint, days int) ([]ReportListItem, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	now := normalizeReportDate(time.Now())
	rangeStart := now.AddDate(0, 0, -(days - 1))
	rangeEnd := now.AddDate(0, 0, 1)

	var reports []BoardDailyReport
	err := database.DB.Where("semantic_board_id = ? AND period_date >= ? AND period_date < ?",
		boardID, rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02")).
		Order("period_date DESC").
		Find(&reports).Error
	if err != nil {
		return nil, fmt.Errorf("list reports for board %d: %w", boardID, err)
	}

	items := make([]ReportListItem, len(reports))
	for i, r := range reports {
		items[i] = ReportListItem{
			ID:              r.ID,
			SemanticBoardID: r.SemanticBoardID,
			PeriodDate:      r.PeriodDate.Format("2006-01-02"),
			Title:           r.Title,
			Summary:         r.Summary,
			ArticleCount:    r.ArticleCount,
			EventTagCount:   r.EventTagCount,
			ClusterCount:    r.ClusterCount,
			Status:          r.Status,
			CreatedAt:       r.CreatedAt,
		}
	}
	return items, nil
}

// ListReportsForAllBoards returns reports for all boards within a date range.
func ListReportsForAllBoards(days int) ([]BoardDailyReport, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	now := normalizeReportDate(time.Now())
	rangeStart := now.AddDate(0, 0, -(days - 1))
	rangeEnd := now.AddDate(0, 0, 1)

	var reports []BoardDailyReport
	err := database.DB.Where("period_date >= ? AND period_date < ?",
		rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02")).
		Order("period_date DESC, semantic_board_id ASC").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("cluster_index ASC")
		}).
		Find(&reports).Error
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}

	return reports, nil
}

// SetReportStatus updates the status field of a report.
func SetReportStatus(id uint, status string) error {
	return database.DB.Model(&BoardDailyReport{}).Where("id = ?", id).
		Update("status", status).Error
}

func normalizeReportDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
}

// collectBoardIDsForDate returns all board IDs that have active event tags on a date.
func CollectBoardIDsForDate(date time.Time) ([]uint, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	type row struct {
		SemanticBoardID uint `json:"semantic_board_id"`
	}
	var rows []row
	err := database.DB.Model(&models.TopicTag{}).
		Select("DISTINCT topic_tag_board_labels.semantic_board_id").
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	ids := make([]uint, len(rows))
	for i, r := range rows {
		ids[i] = r.SemanticBoardID
	}
	return ids, nil
}
