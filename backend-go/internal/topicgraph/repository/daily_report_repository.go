package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
)

// SaveReport saves a daily report and its sections, replacing any existing
// report for the same board and date.

// GetReportByID retrieves a single daily report by its primary key.

// ActiveWatchSummary is the minimal watch descriptor attached to a report
// list item. It is populated only for active watches with a hit in that report.
type ActiveWatchSummary struct {
	WatchID uint   `json:"watch_id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
}

// ReportListItem is a summary view for list endpoints.
type ReportListItem struct {
	ID                   uint                 `json:"id"`
	SemanticBoardID      uint                 `json:"semantic_board_id"`
	PeriodDate           string               `json:"period_date"`
	Title                string               `json:"title"`
	Summary              string               `json:"summary"`
	ArticleCount         int                  `json:"article_count"`
	EventTagCount        int                  `json:"event_tag_count"`
	ClusterCount         int                  `json:"cluster_count"`
	Status               string               `json:"status"`
	CreatedAt            time.Time            `json:"created_at"`
	ActiveWatchSummaries []ActiveWatchSummary `json:"active_watch_summaries"`
}

// ListReports returns recent reports for a board.

// ListReportsForAllBoards returns reports for all boards within a date range.

func NormalizeReportDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
}

// collectBoardIDsForDate returns all board IDs that have active event tags on a date.

// SaveThreads persists a batch of DailyReportThread rows.

// SectionTimelineNode represents a section in a timeline view.
type SectionTimelineNode struct {
	ID               uint            `json:"id"`
	ReportID         uint            `json:"report_id"`
	PeriodDate       time.Time       `json:"period_date"`
	ClusterLabel     string          `json:"cluster_label"`
	Status           string          `json:"status"`
	ArticleCount     int             `json:"article_count"`
	ThreadCount      int             `json:"thread_count"`
	ImageURL         string          `json:"image_url"`
	QualityBreakdown json.RawMessage `json:"quality_breakdown"`
	// Persistent topic assignment. All optional so historical / unmatched
	// sections (persistent_topic_id IS NULL) still serialize cleanly.
	PersistentTopicID    *uint                 `json:"persistent_topic_id,omitempty"`
	TopicMatchDistance   float64               `json:"topic_match_distance,omitempty"`
	TopicMatchConfidence string                `json:"topic_match_confidence,omitempty"`
	PersistentTopic      *PersistentTopicBrief `json:"persistent_topic,omitempty"`
}

// PersistentTopicBrief is the nested topic descriptor attached to each
// timeline node. Color is a stable hash of the topic id so the UI can colour
// same-topic cards consistently without re-hashing on each render.
type PersistentTopicBrief struct {
	ID              uint   `json:"id"`
	Label           string `json:"label"`
	Status          string `json:"status"`
	Color           string `json:"color"`
	HitCount        int    `json:"hit_count"`
	ConsecutiveHits int    `json:"consecutive_hits"`
	CanActivate     bool   `json:"can_activate"`
}

// GetBoardSectionTimeline fetches all sections and their relations for a board within a date range.

// SectionRelationResult represents a relation record for API responses.
// RelationType is "identity" (same persistent topic, bypasses match penalty)
// or "similarity" (Hungarian bipartite match). The UI renders them as a solid
// vs dashed line respectively.
type SectionRelationResult struct {
	FromID       uint    `json:"from_id"`
	ToID         uint    `json:"to_id"`
	Distance     float64 `json:"distance"`
	RelationType string  `json:"relation_type,omitempty"`
}

// SectionTimelineResponse is the response for section timeline/lifecycle APIs.
type SectionTimelineResponse struct {
	Sections  []SectionTimelineNode   `json:"sections"`
	Relations []SectionRelationResult `json:"relations"`
}

// DeriveSectionStatuses computes status for each section based on its relation graph.
//
// Priority: merge > split > continuing > ending > emerging
func DeriveSectionStatuses(sectionIDs []uint, relations []SectionRelationResult, sectionDateMap map[uint]time.Time, latestDate time.Time) map[uint]string {
	statuses := make(map[uint]string, len(sectionIDs))

	// Build degree maps
	outDegree := make(map[uint]int) // from_section_id count
	inDegree := make(map[uint]int)  // to_section_id count
	hasIncoming := make(map[uint]bool)
	hasOutgoing := make(map[uint]bool)

	for _, r := range relations {
		if r.RelationType == "identity" {
			continue
		}
		outDegree[r.FromID]++
		inDegree[r.ToID]++
		hasOutgoing[r.FromID] = true
		hasIncoming[r.ToID] = true
	}

	idSet := make(map[uint]bool, len(sectionIDs))
	for _, id := range sectionIDs {
		idSet[id] = true
	}

	for _, id := range sectionIDs {
		switch {
		case !hasIncoming[id]:
			statuses[id] = "emerging"
		case inDegree[id] > 1:
			statuses[id] = "merge"
		default:
			// Check if any of its from-sections has out-degree > 1 (split)
			split := false
			for _, r := range relations {
				if r.RelationType == "identity" {
					continue
				}
				if r.ToID == id && outDegree[r.FromID] > 1 {
					split = true
					break
				}
			}
			if split {
				statuses[id] = "split"
			} else {
				statuses[id] = "continuing"
			}
		}

		// Override to ending if no outgoing relations and not on latest date
		if !hasOutgoing[id] {
			if d, ok := sectionDateMap[id]; ok && d.Before(latestDate) {
				statuses[id] = "ending"
			}
		}
	}

	return statuses
}

// BackfillSectionEmbeddings generates embeddings for sections that don't have one,
// then runs pgvector matching to set prev_section_id for all sections.
// It overwrites all prev_section_id values (including those from the old tag Jaccard matching).

// BackfillAllRelations rebuilds relations for all boards that have sections with embeddings.
// Daily Report Repository
// =============================================================================

// SaveReport saves a daily report and its sections, replacing any existing
// report for the same board and date.
func (r *TopicGraphRepository) SaveReport(report *BoardDailyReport, sections []DailyReportSection, threadBatches [][]DailyReportThread) error {
	report.PeriodDate = NormalizeReportDate(report.PeriodDate)
	// touchedTopicIDs is captured inside the transaction so the post-commit
	// centroid/vacuum refresh (which reads via r.db and must see committed
	// sections) knows which topics changed.
	var touchedTopicIDs []uint
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Upsert report: find existing by (semantic_board_id, period_date)
		var existing BoardDailyReport
		findErr := tx.Where("semantic_board_id = ? AND period_date = ?",
			report.SemanticBoardID,
			report.PeriodDate.Format("2006-01-02")).
			First(&existing).Error

		if findErr == nil {
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
		} else {
			// Create new report
			if err := tx.Create(report).Error; err != nil {
				return fmt.Errorf("create report: %w", err)
			}
		}

		if findErr == nil {
			// Delete old threads
			if err := tx.Where("report_id = ?", existing.ID).Delete(&DailyReportThread{}).Error; err != nil {
				return fmt.Errorf("delete old threads: %w", err)
			}
			// Delete old sections
			if err := tx.Where("report_id = ?", existing.ID).Delete(&DailyReportSection{}).Error; err != nil {
				return fmt.Errorf("delete old sections: %w", err)
			}
		}

		// Insert new sections (with embedding)
		for i := range sections {
			sections[i].ReportID = report.ID
		}
		// Insert new sections. Two batches: sections WITH embeddings insert
		// normally; watch-materialized sections (or any section whose Embedding
		// is blank) insert with the embedding column omitted — GORM would
		// otherwise write '' which pgvector rejects (watch-materialized-topic:
		// 物化 section Embedding 留空 = NULL).
		withEmb := make([]DailyReportSection, 0, len(sections))
		withoutEmb := make([]DailyReportSection, 0)
		for i := range sections {
			if sections[i].Embedding == "" {
				withoutEmb = append(withoutEmb, sections[i])
			} else {
				withEmb = append(withEmb, sections[i])
			}
		}
		if len(withEmb) > 0 {
			if err := tx.CreateInBatches(withEmb, 20).Error; err != nil {
				return fmt.Errorf("create sections: %w", err)
			}
		}
		if len(withoutEmb) > 0 {
			if err := tx.Omit("embedding").CreateInBatches(withoutEmb, 20).Error; err != nil {
				return fmt.Errorf("create sections (no embedding): %w", err)
			}
		}
		// Copy the generated IDs back into the caller's slice so subsequent
		// steps (assignment by SectionID, thread persistence) keep working —
		// GORM back-fills IDs only into the batch slices we passed in.
		{
			wi, wo := 0, 0
			for i := range sections {
				if sections[i].Embedding == "" {
					sections[i].ID = withoutEmb[wo].ID
					wo++
				} else {
					sections[i].ID = withEmb[wi].ID
					wi++
				}
			}
		}

		// Assign sections to persistent topics and advance the topic
		// lifecycle, before rebuilding relations — the identity edges
		// written by RebuildBoardRelations depend on persistent_topic_id
		// being set. Best-effort non-fatal: a failure degrades to the old
		// similarity-only graph rather than aborting the whole save.
		if len(sections) > 0 {
			if touched, assignErr := assignAndUpdateTopics(tx, report.SemanticBoardID, report.PeriodDate, sections); assignErr != nil {
				logging.Warnf("SaveReport: topic assignment failed for board %d: %v", report.SemanticBoardID, assignErr)
			} else {
				touchedTopicIDs = touched
			}
		}

		// Rebuild all relations for this board using bipartite matching
		if err := RebuildBoardRelations(tx, report.SemanticBoardID); err != nil {
			logging.Warnf("SaveReport: relation rebuild failed: %v", err)
		}

		// Save threads for each section (sections now have IDs after insertion)
		for secIdx, sec := range sections {
			if secIdx < len(threadBatches) && len(threadBatches[secIdx]) > 0 {
				if err := r.SaveThreads(tx, report.ID, sec.ID, threadBatches[secIdx]); err != nil {
					return fmt.Errorf("save threads for section %d: %w", secIdx, err)
				}
			}
		}

		logging.Infof("daily-report: saved report %d for board %d on %s (%d sections)",
			report.ID, report.SemanticBoardID, report.PeriodDate.Format("2006-01-02"), len(sections))
		return nil
	})
	if err != nil {
		return err
	}

	// Post-commit refresh: centroids + vacuum stats. Both read via r.db, so they
	// must run AFTER the transaction commits (sections + persistent_topic_id
	// are otherwise invisible to ComputeTopicCentroid's r.db query). Best-effort:
	// a failure degrades to a stale centroid/vacuum flag, not a save failure.
	for _, tid := range touchedTopicIDs {
		if cerr := r.UpdateCentroidOnSectionChange(nil, tid); cerr != nil {
			logging.Warnf("SaveReport: centroid refresh failed for topic %d: %v", tid, cerr)
		}
	}
	if verr := r.RecomputeVacuumStats(report.SemanticBoardID); verr != nil {
		logging.Warnf("SaveReport: vacuum stats recompute failed for board %d: %v", report.SemanticBoardID, verr)
	}
	return nil
}

// GetReportByID retrieves a single daily report by its primary key.
func (r *TopicGraphRepository) GetReportByID(id uint) (*BoardDailyReport, error) {
	var report BoardDailyReport
	err := r.db.Where("id = ?", id).
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
	// Attach persistent-topic briefs so the detail API exposes topic status
	// (active/candidate) for UI classification. Without this the frontend's
	// qualityZones treats every assigned section as "breaking".
	AttachTopicBriefsToReport(r.db, &report)
	// Fill watch-name decorations on materialized sections (read-path
	// transient; the LLM daily title hides the watch name otherwise).
	AttachWatchLabelsToReport(r.db, &report)
	// GORM leaves Threads nil when a section has no thread rows (legal
	// degradation path); keep the API contract "threads is always an array"
	// for the frontend reader.
	for i := range report.Sections {
		if report.Sections[i].Threads == nil {
			report.Sections[i].Threads = []DailyReportThread{}
		}
	}
	return &report, nil
}

// ListReports returns recent reports for a board.
func (r *TopicGraphRepository) ListReports(boardID uint, days int) ([]ReportListItem, error) {
	if days <= 0 {
		days = 7
	}

	now := NormalizeReportDate(time.Now())
	rangeStart := now.AddDate(0, 0, -(days - 1))
	rangeEnd := now.AddDate(0, 0, 1)

	var reports []BoardDailyReport
	err := r.db.Where("semantic_board_id = ? AND period_date >= ? AND period_date < ?",
		boardID, rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02")).
		Order("period_date DESC").
		Find(&reports).Error
	if err != nil {
		return nil, fmt.Errorf("list reports for board %d: %w", boardID, err)
	}

	summariesByReport := make(map[uint][]ActiveWatchSummary, len(reports))
	if len(reports) > 0 {
		reportIDs := make([]uint, len(reports))
		for i, report := range reports {
			reportIDs[i] = report.ID
			summariesByReport[report.ID] = []ActiveWatchSummary{}
		}

		type activeWatchSummaryRow struct {
			ReportID uint   `gorm:"column:report_id"`
			WatchID  uint   `gorm:"column:watch_id"`
			Label    string `gorm:"column:label"`
			Type     string `gorm:"column:type"`
		}
		var rows []activeWatchSummaryRow
		err = r.db.Table("topic_watch_hits AS h").
			Select("DISTINCT h.report_id, w.id AS watch_id, w.label, w.type").
			Joins("JOIN board_topic_watches AS w ON w.id = h.watch_id").
			Where("h.report_id IN ? AND w.semantic_board_id = ? AND w.status = ?",
				reportIDs, boardID, WatchStatusActive).
			Order("h.report_id ASC, w.id ASC").
			Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("list active watch summaries for board %d: %w", boardID, err)
		}
		for _, row := range rows {
			summariesByReport[row.ReportID] = append(summariesByReport[row.ReportID], ActiveWatchSummary{
				WatchID: row.WatchID,
				Label:   row.Label,
				Type:    row.Type,
			})
		}
	}

	items := make([]ReportListItem, len(reports))
	for i, rpt := range reports {
		items[i] = ReportListItem{
			ID:                   rpt.ID,
			SemanticBoardID:      rpt.SemanticBoardID,
			PeriodDate:           rpt.PeriodDate.Format("2006-01-02"),
			Title:                rpt.Title,
			Summary:              rpt.Summary,
			ArticleCount:         rpt.ArticleCount,
			EventTagCount:        rpt.EventTagCount,
			ClusterCount:         rpt.ClusterCount,
			Status:               rpt.Status,
			CreatedAt:            rpt.CreatedAt,
			ActiveWatchSummaries: summariesByReport[rpt.ID],
		}
	}
	return items, nil
}

// ListReportsForAllBoards returns reports for all boards within a date range.
func (r *TopicGraphRepository) ListReportsForAllBoards(days int) ([]BoardDailyReport, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	now := NormalizeReportDate(time.Now())
	rangeStart := now.AddDate(0, 0, -(days - 1))
	rangeEnd := now.AddDate(0, 0, 1)

	var reports []BoardDailyReport
	err := r.db.Where("period_date >= ? AND period_date < ?",
		rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02")).
		Order("period_date DESC, semantic_board_id ASC").
		Preload("Sections", func(db *gorm.DB) *gorm.DB {
			return db.Order("cluster_index ASC")
		}).
		Find(&reports).Error
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	for i := range reports {
		AttachTopicBriefsToReport(r.db, &reports[i])
		// Keep the API contract "threads is always an array" (see GetReportByID).
		for j := range reports[i].Sections {
			if reports[i].Sections[j].Threads == nil {
				reports[i].Sections[j].Threads = []DailyReportThread{}
			}
		}
	}
	return reports, nil
}

// CollectBoardIDsForDate returns all board IDs that have active event tags on a date.
func (r *TopicGraphRepository) CollectBoardIDsForDate(date time.Time) ([]uint, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	type row struct {
		SemanticBoardID uint `json:"semantic_board_id"`
	}
	var rows []row
	err := r.db.Model(&models.TopicTag{}).
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
	for i, rw := range rows {
		ids[i] = rw.SemanticBoardID
	}
	return ids, nil
}

// SaveThreads persists a batch of DailyReportThread rows using the given DB handle.
// Accepts *gorm.DB so it can participate in an outer transaction.
func (r *TopicGraphRepository) SaveThreads(tx *gorm.DB, reportID, sectionID uint, threads []DailyReportThread) error {
	for i := range threads {
		threads[i].ReportID = reportID
		threads[i].SectionID = sectionID
	}
	// Watch-materialized threads carry no embedding: GORM's zero-value "" is
	// an invalid pgvector literal, so blank-embedding threads insert with the
	// column omitted (NULL) — same treatment as blank-embedding sections
	// (watch-materialized-topic: 物化 thread Embedding 留空).
	withEmb := make([]DailyReportThread, 0, len(threads))
	withoutEmb := make([]DailyReportThread, 0)
	for i := range threads {
		if threads[i].Embedding == "" {
			withoutEmb = append(withoutEmb, threads[i])
		} else {
			withEmb = append(withEmb, threads[i])
		}
	}
	if len(withEmb) > 0 {
		if err := tx.Create(&withEmb).Error; err != nil {
			return err
		}
	}
	if len(withoutEmb) > 0 {
		if err := tx.Omit("embedding").Create(&withoutEmb).Error; err != nil {
			return err
		}
		idx := 0
		for i := range threads {
			if threads[i].Embedding == "" {
				threads[i].ID = withoutEmb[idx].ID
				idx++
			}
		}
	}
	return nil
}

// GetBoardSectionTimeline fetches all sections and their relations for a board within a date range.
func (r *TopicGraphRepository) GetBoardSectionTimeline(boardID uint, days int) (SectionTimelineResponse, error) {
	// days <= 0 表示"全部历史"：跳过 90 天窗口上限，用一个远超真实数据跨度的
	// 窗口等效"不按天过滤"（保留单条大 SELECT，避免重写易错的 SQL 字面量）。
	if days <= 0 {
		days = 100000
	} else if days > 90 {
		days = 90
	}
	var nodes []SectionTimelineNode
	err := r.db.Raw(`
		SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
		       ds.quality_breakdown,
		       ds.article_count,
		       ds.persistent_topic_id,
		       ds.topic_match_distance,
		       ds.topic_match_confidence,
		       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
		       COALESCE(
		         (
		           SELECT a.image_url
		           FROM daily_report_threads t
		           JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(t.related_article_ids)='array' THEN t.related_article_ids ELSE '[]'::jsonb END) aid(article_id) ON true
		           JOIN articles a ON a.id = aid.article_id::bigint
		           WHERE t.section_id = ds.id
		             AND a.image_url IS NOT NULL
		             AND a.image_url != ''
		           ORDER BY t.id ASC, a.pub_date DESC NULLS LAST, a.id ASC
		           LIMIT 1
		         ),
		         (
		           SELECT a.image_url
		           FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(ds.cluster_tag_ids)='array' THEN ds.cluster_tag_ids ELSE '[]'::jsonb END) tid(tag_id)
		           JOIN article_topic_tags att ON att.topic_tag_id = tid.tag_id::bigint
		           JOIN articles a ON a.id = att.article_id
		           WHERE a.pub_date >= bdr.period_date
		             AND a.pub_date < bdr.period_date + INTERVAL '1 day'
		             AND a.image_url IS NOT NULL
		             AND a.image_url != ''
		           ORDER BY a.pub_date DESC NULLS LAST, a.id ASC
		           LIMIT 1
		         ),
		         ''
		       ) AS image_url
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE bdr.semantic_board_id = ?
		  AND bdr.period_date >= (
		    SELECT MAX(latest.period_date)
		    FROM board_daily_reports latest
		    WHERE latest.semantic_board_id = ? AND latest.status = 'completed'
		  ) - (? - 1) * INTERVAL '1 day'
		  AND bdr.status = 'completed'
		ORDER BY bdr.period_date DESC, ds.id ASC
	`, boardID, boardID, days).Scan(&nodes).Error
	if err != nil {
		return SectionTimelineResponse{}, fmt.Errorf("get board section timeline: %w", err)
	}

	// Attach topic briefs (label/status/colour). Colour is hashed from the id
	// so it is stable across renders; fetching label/status here avoids an N+1.
	attachTopicBriefs(r.db, nodes)

	if len(nodes) == 0 {
		return SectionTimelineResponse{
			Sections:  []SectionTimelineNode{},
			Relations: []SectionRelationResult{},
		}, nil
	}

	// Collect section IDs and find latest date
	sectionIDs := make([]uint, len(nodes))
	sectionDateMap := make(map[uint]time.Time, len(nodes))
	var latestDate time.Time
	for i, n := range nodes {
		sectionIDs[i] = n.ID
		sectionDateMap[n.ID] = n.PeriodDate
		if n.PeriodDate.After(latestDate) {
			latestDate = n.PeriodDate
		}
	}

	// Query relations involving these sections
	var relations []SectionRelationResult
	if err := r.db.Raw(`
		SELECT from_section_id AS from_id, to_section_id AS to_id, distance, relation_type
		FROM daily_report_section_relations
		WHERE from_section_id IN ? OR to_section_id IN ?
	`, sectionIDs, sectionIDs).Scan(&relations).Error; err != nil {
		logging.Warnf("GetBoardSectionTimeline: query relations failed: %v", err)
	}

	// Derive statuses
	statuses := DeriveSectionStatuses(sectionIDs, relations, sectionDateMap, latestDate)
	for i := range nodes {
		if s, ok := statuses[nodes[i].ID]; ok {
			nodes[i].Status = s
		}
	}

	if relations == nil {
		relations = []SectionRelationResult{}
	}
	return SectionTimelineResponse{Sections: nodes, Relations: relations}, nil
}

// GetSectionLifecycle fetches the full connected component containing sectionID
// by traversing daily_report_section_relations bidirectionally.
func (r *TopicGraphRepository) GetSectionLifecycle(sectionID uint) (SectionTimelineResponse, error) {
	// Recursive CTE to find the full connected component
	var allIDs []uint
	if err := r.db.Raw(`
		WITH RECURSIVE component AS (
			SELECT ?::bigint AS id
			UNION
			SELECT CASE
				WHEN rel.from_section_id = c.id THEN rel.to_section_id
				ELSE rel.from_section_id
			END AS id
			FROM component c
			JOIN daily_report_section_relations rel
				ON rel.from_section_id = c.id OR rel.to_section_id = c.id
		)
		SELECT DISTINCT id FROM component
	`, sectionID).Scan(&allIDs).Error; err != nil {
		logging.Warnf("GetSectionLifecycle: recursive CTE failed for section %d: %v", sectionID, err)
		// Fallback: return just the section itself
		allIDs = []uint{sectionID}
	}

	if len(allIDs) == 0 {
		allIDs = []uint{sectionID}
	}

	if len(allIDs) == 0 {
		return SectionTimelineResponse{
			Sections:  []SectionTimelineNode{},
			Relations: []SectionRelationResult{},
		}, nil
	}

	// Query section details
	var nodes []SectionTimelineNode
	if err := r.db.Raw(`
		SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
		       ds.quality_breakdown,
		       ds.article_count,
		       ds.persistent_topic_id,
		       ds.topic_match_distance,
		       ds.topic_match_confidence,
		       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
	       COALESCE(
	         (
	           SELECT a.image_url
	           FROM daily_report_threads t
	           JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(t.related_article_ids)='array' THEN t.related_article_ids ELSE '[]'::jsonb END) aid(article_id) ON true
	           JOIN articles a ON a.id = aid.article_id::bigint
	           WHERE t.section_id = ds.id
	             AND a.image_url IS NOT NULL
	             AND a.image_url != ''
	           ORDER BY t.id ASC, a.pub_date DESC NULLS LAST, a.id ASC
	           LIMIT 1
	         ),
	         (
	           SELECT a.image_url
	           FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(ds.cluster_tag_ids)='array' THEN ds.cluster_tag_ids ELSE '[]'::jsonb END) tid(tag_id)
	           JOIN article_topic_tags att ON att.topic_tag_id = tid.tag_id::bigint
	           JOIN articles a ON a.id = att.article_id
	           WHERE a.pub_date >= bdr.period_date
	             AND a.pub_date < bdr.period_date + INTERVAL '1 day'
	             AND a.image_url IS NOT NULL
	             AND a.image_url != ''
	           ORDER BY a.pub_date DESC NULLS LAST, a.id ASC
	           LIMIT 1
	         ),
	         ''
	       ) AS image_url
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE ds.id IN ?
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, allIDs).Scan(&nodes).Error; err != nil {
		return SectionTimelineResponse{}, fmt.Errorf("get section lifecycle: %w", err)
	}
	attachTopicBriefs(r.db, nodes)

	// Query relations where BOTH endpoints are in the returned sections
	var relations []SectionRelationResult
	if err := r.db.Raw(`
		SELECT from_section_id AS from_id, to_section_id AS to_id, distance, relation_type
		FROM daily_report_section_relations
		WHERE from_section_id IN ? AND to_section_id IN ?
	`, allIDs, allIDs).Scan(&relations).Error; err != nil {
		logging.Warnf("GetSectionLifecycle: query relations failed: %v", err)
	}

	// Derive statuses
	sectionDateMap := make(map[uint]time.Time, len(nodes))
	var latestDate time.Time
	for _, n := range nodes {
		sectionDateMap[n.ID] = n.PeriodDate
		if n.PeriodDate.After(latestDate) {
			latestDate = n.PeriodDate
		}
	}
	statuses := DeriveSectionStatuses(allIDs, relations, sectionDateMap, latestDate)
	for i := range nodes {
		if s, ok := statuses[nodes[i].ID]; ok {
			nodes[i].Status = s
		}
	}

	if relations == nil {
		relations = []SectionRelationResult{}
	}
	return SectionTimelineResponse{Sections: nodes, Relations: relations}, nil
}

// attachTopicBriefs fills in PersistentTopicBrief (label/status/colour) on each
// node by fetching the referenced topics in a single query. Colour is derived
// deterministically from the topic id. No-op for nodes without a topic.
// loadTopicBriefMap loads the minimal topic descriptor (label/status/colour)
// for the given ids in one query. Used by both the section-timeline and the
// daily-report detail APIs to avoid N+1 and to keep the JSON payload small
// (no embedding vectors).
func loadTopicBriefMap(db *gorm.DB, ids []uint) map[uint]PersistentTopicBrief {
	if len(ids) == 0 {
		return map[uint]PersistentTopicBrief{}
	}
	var topics []BoardPersistentTopic
	if err := db.Where("id IN ?", ids).Find(&topics).Error; err != nil {
		logging.Warnf("loadTopicBriefMap: load topics failed: %v", err)
		return map[uint]PersistentTopicBrief{}
	}
	cfg := LoadPersistentTopicConfig(db)
	briefByID := make(map[uint]PersistentTopicBrief, len(topics))
	for _, t := range topics {
		briefByID[t.ID] = PersistentTopicBrief{
			ID: t.ID, Label: t.Label, Status: t.Status,
			Color: PersistentTopicColor(t.ID), HitCount: t.HitCount, ConsecutiveHits: t.ConsecutiveHits,
			CanActivate: t.Status == TopicStatusCandidate && t.HitCount >= cfg.UpgradeThreshold,
		}
	}
	return briefByID
}

// AttachTopicBriefsToReport fills the transient PersistentTopic brief on each
// section of a daily report, so the report detail API exposes the same topic
// status the section-timeline API does. Used by GetReportByID and the report
// list handler.
func AttachTopicBriefsToReport(db *gorm.DB, report *BoardDailyReport) {
	if report == nil {
		return
	}
	attachBriefsToSections(db, report.Sections)
}

// AttachWatchLabelsToReport fills the transient WatchLabel on each
// materialized (watch_*) section so the UI badge can show the tracking
// source next to the LLM daily title (watch-materialize-llm-adjudication).
// New sections resolve via the persisted WatchID (deleted watches keep their
// name — the lookup deliberately ignores status); historical sections
// (WatchID NULL) fall back to fixed-name parsing (keyword track) or the
// section title itself (sentence track was titled with the watch label
// before LLM titles existed).
func AttachWatchLabelsToReport(db *gorm.DB, report *BoardDailyReport) {
	if report == nil {
		return
	}
	AttachWatchLabelsToSections(db, report.Sections)
}

// AttachWatchLabelsToSections fills the transient WatchLabel in place.
func AttachWatchLabelsToSections(db *gorm.DB, sections []DailyReportSection) {
	anyMaterialized := false
	idSet := make(map[uint]bool)
	for i := range sections {
		if !strings.HasPrefix(sections[i].LaneTier, "watch_") {
			continue
		}
		anyMaterialized = true
		if sections[i].WatchID != nil {
			idSet[*sections[i].WatchID] = true
		}
	}
	if !anyMaterialized {
		return
	}
	nameByID := make(map[uint]string, len(idSet))
	if len(idSet) > 0 {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var watches []BoardTopicWatch
		if err := db.Where("id IN ?", ids).Find(&watches).Error; err == nil {
			for _, w := range watches {
				nameByID[w.ID] = w.Label
			}
		}
	}
	for i := range sections {
		s := &sections[i]
		if !strings.HasPrefix(s.LaneTier, "watch_") {
			continue
		}
		if s.WatchID != nil {
			if n, ok := nameByID[*s.WatchID]; ok {
				s.WatchLabel = n
			}
			continue
		}
		if s.LaneTier == "watch_keyword" {
			if expr, ok := parseKeywordWatchSectionLabel(s.ClusterLabel); ok {
				s.WatchLabel = expr
			}
		} else {
			s.WatchLabel = s.ClusterLabel // historical sentence title = watch label
		}
	}
}

// parseKeywordWatchSectionLabel reverses buildKeywordWatchSectionLabel
// (service layer): 关键字『X』相关话题 → X. ok=false when the label is not
// the fixed-name shape (e.g. an LLM daily title — those sections carry a
// persisted WatchID and never reach this fallback).
func parseKeywordWatchSectionLabel(label string) (string, bool) {
	const pre, suf = "关键字『", "』相关话题"
	if len(label) > len(pre)+len(suf) && strings.HasPrefix(label, pre) && strings.HasSuffix(label, suf) {
		return label[len(pre) : len(label)-len(suf)], true
	}
	return "", false
}

// attachBriefsToSections fills the transient PersistentTopic brief in place.
func attachBriefsToSections(db *gorm.DB, sections []DailyReportSection) {
	topicIDs := make(map[uint]bool)
	for i := range sections {
		if sections[i].PersistentTopicID != nil {
			topicIDs[*sections[i].PersistentTopicID] = true
		}
	}
	if len(topicIDs) == 0 {
		return
	}
	ids := make([]uint, 0, len(topicIDs))
	for id := range topicIDs {
		ids = append(ids, id)
	}
	briefByID := loadTopicBriefMap(db, ids)
	for i := range sections {
		if sections[i].PersistentTopicID == nil {
			continue
		}
		if brief, ok := briefByID[*sections[i].PersistentTopicID]; ok {
			sections[i].PersistentTopic = &brief
		}
	}
}

func attachTopicBriefs(db *gorm.DB, nodes []SectionTimelineNode) {
	topicIDs := make(map[uint]bool)
	for i := range nodes {
		if nodes[i].PersistentTopicID != nil {
			topicIDs[*nodes[i].PersistentTopicID] = true
		}
	}
	if len(topicIDs) == 0 {
		return
	}
	ids := make([]uint, 0, len(topicIDs))
	for id := range topicIDs {
		ids = append(ids, id)
	}
	briefByID := loadTopicBriefMap(db, ids)
	for i := range nodes {
		if nodes[i].PersistentTopicID == nil {
			continue
		}
		if brief, ok := briefByID[*nodes[i].PersistentTopicID]; ok {
			nodes[i].PersistentTopic = &brief
		}
	}
}

// GetTopicLifeline returns every section assigned to a persistent topic (no
// day limit) plus the internal relations among them. Unlike
// GetSectionLifecycle — which walks the embedding relation graph — this
// aggregates purely by persistent_topic_id, so a narrative chain persists even
// when cluster-label drift broke the similarity edges.
func (r *TopicGraphRepository) GetTopicLifeline(topicID uint) (SectionTimelineResponse, error) {
	var nodes []SectionTimelineNode
	err := r.db.Raw(`
		SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
		       ds.quality_breakdown,
		       ds.article_count,
		       ds.persistent_topic_id,
		       ds.topic_match_distance,
		       ds.topic_match_confidence,
		       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
		       '' AS image_url
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE ds.persistent_topic_id = ?
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, topicID).Scan(&nodes).Error
	if err != nil {
		return SectionTimelineResponse{}, fmt.Errorf("get topic lifeline: %w", err)
	}
	attachTopicBriefs(r.db, nodes)

	if len(nodes) == 0 {
		return SectionTimelineResponse{
			Sections: []SectionTimelineNode{}, Relations: []SectionRelationResult{},
		}, nil
	}

	sectionIDs := make([]uint, len(nodes))
	sectionDateMap := make(map[uint]time.Time, len(nodes))
	var latestDate time.Time
	for i, n := range nodes {
		sectionIDs[i] = n.ID
		sectionDateMap[n.ID] = n.PeriodDate
		if n.PeriodDate.After(latestDate) {
			latestDate = n.PeriodDate
		}
	}
	var relations []SectionRelationResult
	if err := r.db.Raw(`
		SELECT from_section_id AS from_id, to_section_id AS to_id, distance, relation_type
		FROM daily_report_section_relations
		WHERE from_section_id IN ? AND to_section_id IN ?
	`, sectionIDs, sectionIDs).Scan(&relations).Error; err != nil {
		logging.Warnf("GetTopicLifeline: query relations failed: %v", err)
	}
	statuses := DeriveSectionStatuses(sectionIDs, relations, sectionDateMap, latestDate)
	for i := range nodes {
		if s, ok := statuses[nodes[i].ID]; ok {
			nodes[i].Status = s
		}
	}
	if relations == nil {
		relations = []SectionRelationResult{}
	}
	return SectionTimelineResponse{Sections: nodes, Relations: relations}, nil
}

// BackfillSectionEmbeddings regenerates section embeddings using the
// content-assembly rules (tags' label/description/article excerpts — same as
// the live pipeline; fallback chain: thread titles → cluster_label).
//
// Modes (fix-section-embedding-content-based):
//   - fill mode (recompute=false, the legacy default): only sections whose
//     embedding IS NULL, text now assembled by the content rules;
//   - recompute mode (recompute=true): ALL sections in range get re-embedded
//     (boardID nil = all boards; sinceDays default 30, 0 = unlimited).
//
// After embedding, affected topics' centroids are recomputed, then relations
// are rebuilt per board. Per-section embed failures skip that section and
// continue; stats are logged and returned.
func (r *TopicGraphRepository) BackfillSectionEmbeddings(ctx context.Context, recompute bool, boardID *uint, sinceDays int) (embedded, skipped, matched int, err error) {
	// 1. Load candidate sections (id, board, period_date, cluster_tag_ids,
	//    cluster_label) plus their thread titles for the fallback chain.
	type sectionRow struct {
		ID            uint
		BoardID       uint
		TopicID       uint
		PeriodDate    time.Time
		ClusterTagIDs JSON
		ClusterLabel  string
	}

	query := r.db.Table("daily_report_sections s").
		Select("s.id, r.semantic_board_id AS board_id, s.persistent_topic_id AS topic_id, r.period_date, s.cluster_tag_ids, s.cluster_label").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id")
	if !recompute {
		query = query.Where("s.embedding IS NULL")
	} else {
		if boardID != nil {
			query = query.Where("r.semantic_board_id = ?", *boardID)
		}
		if sinceDays > 0 {
			query = query.Where("r.period_date >= ?", time.Now().AddDate(0, 0, -sinceDays))
		}
	}
	var rows []sectionRow
	if err := query.Scan(&rows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("query backfill sections: %w", err)
	}
	if len(rows) == 0 {
		logging.Infof("backfill-section-embeddings: no sections in range (recompute=%v)", recompute)
		return 0, 0, 0, nil
	}

	// 2. Resolve per-section embed texts. Tag facts (label/description) are
	//    batch-loaded once; representative-article context is fetched per tag
	//    over the section's own day window (buildArticleContextForTag lives in
	//    the service package, so the query is reimplemented here against
	//    models.Article with the same precedence).
	texts := make([]string, 0, len(rows))
	var targets []sectionRow
	for _, row := range rows {
		var tagIDs []uint
		_ = json.Unmarshal(row.ClusterTagIDs, &tagIDs)
		text := r.assembleSectionEmbedText(tagIDs, row.PeriodDate, row.ID)
		if strings.TrimSpace(text) == "" {
			skipped++
			continue
		}
		texts = append(texts, text)
		targets = append(targets, row)
	}

	// 3. Batch embed + persist. Affected boards/topics are collected for the
	//    centroid + relation refresh below.
	batchSize := 50
	touchedBoards := make(map[uint]bool)
	touchedTopics := make(map[uint]bool)
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		result, embedErr := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
			Input:     texts[start:end],
			Operation: "section.embedding_backfill",
			Metadata: map[string]any{
				"operation": "daily_report_section_backfill",
				"recompute": recompute,
			},
		}, airouter.CapabilityEmbedding)
		if embedErr != nil {
			// Whole-batch failure: count as skipped, keep going with next batch.
			logging.Warnf("backfill-section-embeddings: batch embed failed (%d sections skipped): %v", end-start, embedErr)
			skipped += end - start
			continue
		}
		for i := 0; i < end-start && i < len(result.Embeddings); i++ {
			row := targets[start+i]
			pgVec := FloatsToPgVector(result.Embeddings[i])
			if err := r.db.Model(&DailyReportSection{}).Where("id = ?", row.ID).
				Update("embedding", pgVec).Error; err != nil {
				logging.Warnf("backfill-section-embeddings: update section %d failed: %v", row.ID, err)
				skipped++
				continue
			}
			embedded++
			touchedBoards[row.BoardID] = true
			if row.TopicID != 0 {
				touchedTopics[row.TopicID] = true
			}
		}
	}

	// 4. Refresh centroids for affected topics (now reading committed
	//    embeddings via r.db).
	for tid := range touchedTopics {
		if cerr := r.UpdateCentroidOnSectionChange(nil, tid); cerr != nil {
			logging.Warnf("backfill-section-embeddings: centroid refresh failed for topic %d: %v", tid, cerr)
		}
	}

	// 5. Rebuild cross-day relations for affected boards.
	for bid := range touchedBoards {
		rebuilt, rerr := r.BackfillRelations(bid)
		if rerr != nil {
			logging.Warnf("backfill-section-embeddings: backfill board %d failed: %v", bid, rerr)
			continue
		}
		matched += rebuilt
	}

	logging.Infof("backfill-section-embeddings: complete recompute=%v embedded=%d skipped=%d relations=%d", recompute, embedded, skipped, matched)
	return embedded, skipped, matched, nil
}

// Mirrors of the service-layer assembly constants
// (daily_report_embed_text.go); kept local to avoid the import cycle
// (service imports repository) — same pattern as repoParsePgVector.
const (
	backfillArticleContextRunes = 100
	// backfillSectionEmbedRunes mirrors the service cap; sized for the
	// embedding gateway's 512-token per-input limit (see service comments).
	backfillSectionEmbedRunes = 480
	backfillContextArticles   = 3
)

// assembleSectionEmbedText builds the content-based embedding text for one
// historical section: its tags' label/description/representative-article
// excerpts over the section's own day window, falling back to thread titles,
// then cluster_label. Mirrors service.buildSectionEmbedText for backfill.
func (r *TopicGraphRepository) assembleSectionEmbedText(tagIDs []uint, periodDate time.Time, sectionID uint) string {
	var sb strings.Builder
	if len(tagIDs) > 0 {
		type tagRow struct {
			ID          uint
			Label       string
			Description string
		}
		var tags []tagRow
		if err := r.db.Model(&models.TopicTag{}).
			Select("id, label, description").
			Where("id IN ?", tagIDs).
			Find(&tags).Error; err != nil {
			tags = nil
		}
		byID := make(map[uint]tagRow, len(tags))
		for _, t := range tags {
			byID[t.ID] = t
		}
		for _, id := range tagIDs {
			t, ok := byID[id]
			if !ok {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(strings.TrimSpace(t.Label))
			if d := strings.TrimSpace(t.Description); d != "" {
				sb.WriteString("：")
				sb.WriteString(d)
			}
			if a := strings.TrimSpace(r.tagArticleContext(id, periodDate)); a != "" {
				sb.WriteString("；代表文章：")
				if utf8.RuneCountInString(a) > backfillArticleContextRunes {
					a = string([]rune(a)[:backfillArticleContextRunes])
				}
				sb.WriteString(a)
			}
			if text := sb.String(); utf8.RuneCountInString(text) >= backfillSectionEmbedRunes {
				break
			}
		}
	}
	if text := sb.String(); strings.TrimSpace(text) != "" {
		return truncateRunesBackfill(text)
	}
	// Fallback 1: thread titles.
	var titles []string
	if err := r.db.Model(&DailyReportThread{}).
		Where("section_id = ? AND COALESCE(TRIM(title), '') != ''", sectionID).
		Order("id ASC").Limit(10).
		Pluck("title", &titles).Error; err != nil {
		titles = nil
	}
	if len(titles) > 0 {
		return truncateRunesBackfill(strings.Join(titles, "\n"))
	}
	// Fallback 2: cluster label.
	var label string
	if err := r.db.Model(&DailyReportSection{}).Select("cluster_label").
		Where("id = ?", sectionID).Scan(&label).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(label)
}

// tagArticleContext loads the representative-article context for one tag over
// the section's day window. Same precedence as the service-layer
// buildArticleContextForTag (AIContentSummary > FirecrawlContent > Content >
// Description); reimplemented here to avoid the service import.
func (r *TopicGraphRepository) tagArticleContext(tagID uint, day time.Time) string {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.Add(24 * time.Hour)
	type articleRow struct {
		Title            string
		AIContentSummary string
		FirecrawlContent string
		Content          string
		Description      string
	}
	var rows []articleRow
	if err := r.db.Model(&models.Article{}).
		Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
		Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagID, start, end).
		Order("articles.pub_date DESC").
		Limit(backfillContextArticles).
		Find(&rows).Error; err != nil {
		return ""
	}
	var parts []string
	for _, a := range rows {
		summary := firstNonBlank(a.AIContentSummary, a.FirecrawlContent, a.Content, a.Description)
		if strings.TrimSpace(summary) == "" {
			continue
		}
		if title := strings.TrimSpace(a.Title); title != "" {
			parts = append(parts, fmt.Sprintf("《%s》%s", title, summary))
		} else {
			parts = append(parts, summary)
		}
	}
	return strings.Join(parts, " ; ")
}

func firstNonBlank(fields ...string) string {
	for _, s := range fields {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateRunesBackfill(s string) string {
	if utf8.RuneCountInString(s) <= backfillSectionEmbedRunes {
		return s
	}
	return string([]rune(s)[:backfillSectionEmbedRunes])
}

// BackfillRelations rebuilds relations for a single board.
func (r *TopicGraphRepository) BackfillRelations(boardID uint) (rebuilt int, err error) {
	tx := r.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if err = RebuildBoardRelations(tx, boardID); err != nil {
		return 0, fmt.Errorf("rebuild board relations: %w", err)
	}

	if err = tx.Raw(`
		SELECT COUNT(*) FROM daily_report_section_relations
		WHERE from_section_id IN (
			SELECT s.id FROM daily_report_sections s
			JOIN board_daily_reports rpt ON rpt.id = s.report_id
			WHERE rpt.semantic_board_id = ?
		)
	`, boardID).Scan(&rebuilt).Error; err != nil {
		return 0, fmt.Errorf("count relations: %w", err)
	}

	if err = tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("commit backfill: %w", err)
	}
	return rebuilt, nil
}

// BackfillAllRelations rebuilds relations for all boards that have sections with embeddings.
func (r *TopicGraphRepository) BackfillAllRelations() (map[uint]int, error) {
	type boardEntry struct {
		BoardID uint
	}
	var boards []boardEntry
	if err := r.db.Raw(`
		SELECT DISTINCT rpt.semantic_board_id AS board_id
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.embedding IS NOT NULL
	`).Scan(&boards).Error; err != nil {
		return nil, fmt.Errorf("query boards: %w", err)
	}

	results := make(map[uint]int, len(boards))
	for _, b := range boards {
		rebuilt, bErr := r.BackfillRelations(b.BoardID)
		if bErr != nil {
			logging.Warnf("BackfillAllRelations: board %d failed: %v", b.BoardID, bErr)
			continue
		}
		results[b.BoardID] = rebuilt
		logging.Infof("BackfillAllRelations: board %d rebuilt %d relations", b.BoardID, rebuilt)
	}
	return results, nil
}

// =============================================================================
// ListTopicRecentBriefs — 泳道上下文注入（切片 D）
// =============================================================================

// ListTopicRecentBriefs fetches, for every active AND candidate persistent
// topic on the board (candidate-topic-l2-gate: candidates now flow through L2
// adjudication and need content to judge), sections from the last `sinceDays`
// days with up to 5 active tag labels per section — the section's actual
// tag-label fact fingerprint resolved from cluster_tag_ids. Sections are
// sorted by period_date DESC and trimmed to `perTopicLimit` per topic.
// Returns map[topicID][]TopicRecentBrief.
//
// The tag labels replace the old cluster_label source (frozen by the
// orchestrator's topic-label overwrite → zero information) and the old
// thread-title LATERAL (prompt-hygiene red line: no historical narrative
// injection). Merged/disabled tags are filtered by topic_tags.status='active'.
//
// Degradation contract: when the query fails (DB down etc.), the caller SHALL
// fall back to label-only injection — the briefs are purely an information
// enrichment layer and SHALL NOT block ClusterTags.
func (r *TopicGraphRepository) ListTopicRecentBriefs(boardID uint, sinceDays int, perTopicLimit int) (map[uint][]TopicRecentBrief, error) {
	cutoff := time.Now().AddDate(0, 0, -sinceDays).Truncate(24 * time.Hour)
	// Same-day exclusion: briefs inject only facts from BEFORE today. A same-day
	// rerun would otherwise feed back the current run's own (possibly wrong)
	// attachments as "recent content" evidence — the self-corroboration loop
	// behind the Kalibaf candidate case. Yesterday becomes evidence tomorrow.
	today := NormalizeReportDate(time.Now())

	// 1) Collect anchorable topic IDs (active + candidate) on this board.
	type topicRow struct {
		ID     uint
		Status string
	}
	var topics []topicRow
	err := r.db.Model(&BoardPersistentTopic{}).
		Select("id, status").
		Where("semantic_board_id = ? AND status IN ?", boardID, []string{TopicStatusActive, TopicStatusCandidate}).
		Find(&topics).Error
	if err != nil {
		return nil, fmt.Errorf("listTopicRecentBriefs: load anchorable topics: %w", err)
	}
	if len(topics) == 0 {
		return nil, nil
	}

	anchorIDs := make([]uint, len(topics))
	for i, t := range topics {
		anchorIDs[i] = t.ID
	}

	// 2) Raw query: sections + tag labels (fact fingerprint). Each section's
	// cluster_tag_ids JSON array is unnested with ordinality and LEFT JOINed
	// against topic_tags (active only), so merged/disabled tags yield NULL
	// labels that are dropped at assembly while the section itself survives
	// (label-only degradation at prompt level). Duplicate tag ids inside one
	// array are collapsed via DISTINCT ON; tag_ord preserves the array order
	// so the per-section cap of 5 keeps the cluster's own tag ordering.
	type row struct {
		TopicID    uint      `gorm:"column:persistent_topic_id"`
		SectionID  uint      `gorm:"column:section_id"`
		PeriodDate time.Time `gorm:"column:period_date"`
		TagLabel   *string   `gorm:"column:tag_label"`
	}

	var rows []row
	err = r.db.Raw(`
		WITH expanded AS (
			SELECT
				ds.persistent_topic_id,
				ds.id AS section_id,
				bdr.period_date,
				tt.label AS tag_label,
				ord.n AS tag_ord
			FROM daily_report_sections ds
			JOIN board_daily_reports bdr ON bdr.id = ds.report_id
			LEFT JOIN LATERAL jsonb_array_elements_text(ds.cluster_tag_ids) WITH ORDINALITY AS ord(elem, n) ON true
			LEFT JOIN topic_tags tt ON tt.id = ord.elem::bigint AND tt.status = 'active'
			WHERE ds.persistent_topic_id IN ?
				AND bdr.period_date >= ?
				AND bdr.period_date < ?
		)
		SELECT DISTINCT ON (persistent_topic_id, section_id, tag_ord, tag_label)
			persistent_topic_id, section_id, period_date, tag_label
		FROM expanded
		ORDER BY persistent_topic_id, section_id, tag_ord, tag_label
	`, anchorIDs, cutoff, today).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listTopicRecentBriefs: query sections+tag labels: %w", err)
	}

	// 3) Group rows by topic → section → tag labels (cap 5 per section).
	// Re-sort by (topic_id, period_date DESC, section_id ASC) so per-topic
	// trimming keeps the newest sections; a section→index map keeps each
	// section's labels assembling in tag_ord order regardless of sort moves.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TopicID != rows[j].TopicID {
			return rows[i].TopicID < rows[j].TopicID
		}
		if !rows[i].PeriodDate.Equal(rows[j].PeriodDate) {
			return rows[i].PeriodDate.After(rows[j].PeriodDate)
		}
		return rows[i].SectionID < rows[j].SectionID
	})

	const tagsPerSectionCap = 5

	result := make(map[uint][]TopicRecentBrief)
	type sectionKey struct {
		TopicID   uint
		SectionID uint
	}
	briefIdx := make(map[sectionKey]int) // section → index into result[topicID]

	for _, row := range rows {
		sk := sectionKey{row.TopicID, row.SectionID}
		idx, exists := briefIdx[sk]
		if !exists {
			// Enforce per-topic section cap.
			if len(result[row.TopicID]) >= perTopicLimit {
				continue
			}
			result[row.TopicID] = append(result[row.TopicID], TopicRecentBrief{
				TopicID:    row.TopicID,
				SectionID:  row.SectionID,
				PeriodDate: row.PeriodDate,
			})
			idx = len(result[row.TopicID]) - 1
			briefIdx[sk] = idx
		}
		// Append tag label (cap per section; NULL = merged/disabled tag → dropped).
		if row.TagLabel != nil && *row.TagLabel != "" && len(result[row.TopicID][idx].TagLabels) < tagsPerSectionCap {
			result[row.TopicID][idx].TagLabels = append(result[row.TopicID][idx].TagLabels, *row.TagLabel)
		}
	}

	return result, nil
}

// CountSectionsByTopic aggregates the section count per persistent_topic_id across
// all daily_report_sections (null topics excluded). Returns a map[topicID]count.
func (r *TopicGraphRepository) CountSectionsByTopic() (map[uint]int, error) {
	type countRow struct {
		PersistentTopicID uint
		N                 int
	}
	var counts []countRow
	if err := r.db.Table("daily_report_sections").
		Select("persistent_topic_id, count(*) AS n").
		Where("persistent_topic_id IS NOT NULL").
		Group("persistent_topic_id").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]int, len(counts))
	for _, cr := range counts {
		result[cr.PersistentTopicID] = cr.N
	}
	return result, nil
}

// =============================================================================
