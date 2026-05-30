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
func SaveReport(report *BoardDailyReport, sections []DailyReportSection, threadBatches [][]DailyReportThread) error {
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
			// Nullify downstream prev_thread_id references before deleting old threads
			if err := tx.Model(&DailyReportThread{}).
				Where("prev_thread_id IN (SELECT id FROM daily_report_threads WHERE report_id = ?)", existing.ID).
				Update("prev_thread_id", nil).Error; err != nil {
				return fmt.Errorf("nullify downstream prev_thread_id: %w", err)
			}
			// Nullify downstream prev_section_id references before deleting old sections
			if err := tx.Model(&DailyReportSection{}).
				Where("prev_section_id IN (SELECT id FROM daily_report_sections WHERE report_id = ?)", existing.ID).
				Update("prev_section_id", nil).Error; err != nil {
				return fmt.Errorf("nullify downstream prev_section_id: %w", err)
			}
			// Delete old threads
			if err := tx.Where("report_id = ?", existing.ID).Delete(&DailyReportThread{}).Error; err != nil {
				return fmt.Errorf("delete old threads: %w", err)
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

		// Save threads for each section (sections now have IDs after insertion)
		for secIdx, sec := range sections {
			if secIdx < len(threadBatches) && len(threadBatches[secIdx]) > 0 {
				if err := SaveThreads(tx, report.ID, sec.ID, threadBatches[secIdx]); err != nil {
					return fmt.Errorf("save threads for section %d: %w", secIdx, err)
				}
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
		Preload("Sections.Threads", func(db *gorm.DB) *gorm.DB {
			return db.Order("id ASC")
		}).
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

// SaveThreads persists a batch of DailyReportThread rows.
func SaveThreads(tx *gorm.DB, reportID, sectionID uint, threads []DailyReportThread) error {
	for i := range threads {
		threads[i].ReportID = reportID
		threads[i].SectionID = sectionID
	}
	return tx.Create(&threads).Error
}

// GetThreadsBySection returns all threads for a section, ordered by id.
func GetThreadsBySection(sectionID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := database.DB.Where("section_id = ?", sectionID).Order("id ASC").Find(&threads).Error
	return threads, err
}

// GetThreadsByReport returns all threads for a report.
func GetThreadsByReport(reportID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := database.DB.Where("report_id = ?", reportID).Order("section_id ASC, id ASC").Find(&threads).Error
	return threads, err
}

// GetThreadByID returns a single thread by its primary key.
func GetThreadByID(id uint) (*DailyReportThread, error) {
	var thread DailyReportThread
	err := database.DB.First(&thread, id).Error
	if err != nil {
		return nil, fmt.Errorf("thread %d not found: %w", id, err)
	}
	return &thread, nil
}

// DeleteThreadsByReport deletes all threads for a report.
func DeleteThreadsByReport(reportID uint) error {
	return database.DB.Where("report_id = ?", reportID).Delete(&DailyReportThread{}).Error
}

// ThreadLineageNode represents a thread in a lineage chain with its report date.
type ThreadLineageNode struct {
	DailyReportThread
	PeriodDate   time.Time `json:"period_date"`
	ClusterLabel string    `json:"cluster_label"`
}

// GetThreadLineage fetches the full lineage chain for a thread using recursive CTE.
func GetThreadLineage(threadID uint) ([]ThreadLineageNode, error) {
	var nodes []ThreadLineageNode
	err := database.DB.Raw(`
		WITH RECURSIVE chain AS (
			-- Base: the target thread
			SELECT t.id, t.report_id, t.section_id, t.title, t.summary, t.status,
			       t.tag_ids, t.confidence, t.prev_thread_id, t.related_article_ids, t.created_at,
			       bdr.period_date, ds.cluster_label
			FROM daily_report_threads t
			JOIN board_daily_reports bdr ON bdr.id = t.report_id
			JOIN daily_report_sections ds ON ds.id = t.section_id
			WHERE t.id = ?

			UNION ALL

			-- Walk up to ancestors via prev_thread_id
			SELECT parent.id, parent.report_id, parent.section_id, parent.title, parent.summary, parent.status,
			       parent.tag_ids, parent.confidence, parent.prev_thread_id, parent.related_article_ids, parent.created_at,
			       bdr.period_date, ds.cluster_label
			FROM daily_report_threads parent
			JOIN chain c ON c.prev_thread_id = parent.id
			JOIN board_daily_reports bdr ON bdr.id = parent.report_id
			JOIN daily_report_sections ds ON ds.id = parent.section_id
		)
		SELECT * FROM chain ORDER BY period_date ASC
	`, threadID).Scan(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get thread lineage: %w", err)
	}
	return nodes, nil
}

// GetBoardThreadTimeline fetches all threads for a board within a date range.
func GetBoardThreadTimeline(boardID uint, days int) ([]ThreadLineageNode, error) {
	if days <= 0 {
		days = 30
	}
	if days > 90 {
		days = 90
	}
	var nodes []ThreadLineageNode
	err := database.DB.Raw(`
		SELECT t.id, t.report_id, t.section_id, t.title, t.summary, t.status,
		       t.tag_ids, t.confidence, t.prev_thread_id, t.related_article_ids, t.created_at,
		       bdr.period_date, ds.cluster_label
		FROM daily_report_threads t
		JOIN board_daily_reports bdr ON bdr.id = t.report_id
		JOIN daily_report_sections ds ON ds.id = t.section_id
		WHERE bdr.semantic_board_id = ?
		  AND bdr.period_date >= CURRENT_DATE - ? * INTERVAL '1 day'
		  AND bdr.status = 'completed'
		ORDER BY t.prev_thread_id NULLS FIRST, bdr.period_date ASC, t.id ASC
	`, boardID, days).Scan(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get board thread timeline: %w", err)
	}
	return nodes, nil
}
