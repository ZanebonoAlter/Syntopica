package dataenrichment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/service"
)

// NewTopicGraphSectionReader creates a SectionReader that queries daily_report_sections
// joined with daily_report_threads to produce news content text for a topic.
// Uses raw SQL to avoid circular imports with the topicgraph package.
func NewTopicGraphSectionReader(db *gorm.DB) service.SectionReader {
	return &topicGraphSectionReader{db: db}
}

type topicGraphSectionReader struct {
	db *gorm.DB
}

// SectionDates returns distinct period_date values for sections belonging to a topic, ascending.
func (r *topicGraphSectionReader) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	var dates []time.Time
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT bdr.period_date
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE ds.persistent_topic_id = ?
		ORDER BY bdr.period_date ASC
	`, topicID).Scan(&dates).Error
	if err != nil {
		return nil, fmt.Errorf("query section dates: %w", err)
	}
	return dates, nil
}

func (r *topicGraphSectionReader) ReadSections(ctx context.Context, topicID uint, from, to time.Time) (string, error) {
	// Query sections within the date range for this topic.
	type sectionRow struct {
		ID           uint      `gorm:"column:id"`
		PeriodDate   time.Time `gorm:"column:period_date"`
		ClusterLabel string    `gorm:"column:cluster_label"`
	}

	var sections []sectionRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT ds.id, bdr.period_date, ds.cluster_label
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE ds.persistent_topic_id = ?
		  AND bdr.period_date >= ? AND bdr.period_date < ?
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, topicID, from, to).Scan(&sections).Error
	if err != nil {
		return "", fmt.Errorf("query sections: %w", err)
	}

	if len(sections) == 0 {
		return "(该周期暂无相关新闻)", nil
	}

	// For each section, load thread titles.
	var parts []string
	for _, s := range sections {
		var titles []string
		err := r.db.WithContext(ctx).Raw(`
			SELECT title FROM daily_report_threads
			WHERE section_id = ? ORDER BY id ASC LIMIT 5
		`, s.ID).Scan(&titles).Error
		if err != nil {
			continue
		}
		dateStr := s.PeriodDate.Format("2006-01-02")
		if len(titles) > 0 {
			parts = append(parts, fmt.Sprintf("%s [%s]: %s", dateStr, s.ClusterLabel, strings.Join(titles, "; ")))
		} else {
			parts = append(parts, fmt.Sprintf("%s [%s]", dateStr, s.ClusterLabel))
		}
	}

	return strings.Join(parts, "\n"), nil
}

// NewDBTopicLister creates an ActiveTopicLister that queries board_persistent_topics.
func NewDBTopicLister(db *gorm.DB) ActiveTopicLister {
	return &dbTopicLister{db: db}
}

type dbTopicLister struct {
	db *gorm.DB
}

func (l *dbTopicLister) ListActiveTopicIDs(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := l.db.WithContext(ctx).Raw(`
		SELECT id FROM board_persistent_topics WHERE status = 'active' ORDER BY id ASC
	`).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("list active topics: %w", err)
	}
	return ids, nil
}

// NewDBLifelineReader creates a production service.LifelineReader backed by gorm.DB.
// Queries board_persistent_topics + daily_report_sections + daily_report_threads
// using raw SQL to avoid circular imports with topicgraph.
func NewDBLifelineReader(db *gorm.DB) service.LifelineReader {
	return &dbLifelineReader{db: db}
}

type dbLifelineReader struct {
	db *gorm.DB
}

// GetTopicLifelineArchive returns the lane's month/year background memory
// from topic_lifeline_context: latest 2 month rows + latest 1 year row,
// newest first within each granularity (matches the agent archive rendering
// budget in lifeline_renderer).
func (r *dbLifelineReader) GetTopicLifelineArchive(topicID uint) ([]service.LifelineArchiveRow, error) {
	type archiveRow struct {
		Granularity string    `gorm:"column:granularity"`
		Period      string    `gorm:"column:period"`
		AsOfDate    time.Time `gorm:"column:as_of_date"`
		Content     string    `gorm:"column:content"`
	}
	var rows []archiveRow
	if err := r.db.Raw(`
		SELECT granularity, period, as_of_date, content
		FROM topic_lifeline_context
		WHERE persistent_topic_id = ? AND granularity IN ('month', 'year')
		ORDER BY granularity ASC, period DESC
	`, topicID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query lifeline archive: %w", err)
	}
	out := make([]service.LifelineArchiveRow, 0, len(rows))
	monthTaken, yearTaken := 0, 0
	for _, rw := range rows {
		var take int
		switch rw.Granularity {
		case "month":
			take = 2
		case "year":
			take = 1
		default:
			continue
		}
		if rw.Granularity == "month" {
			if monthTaken >= take {
				continue
			}
			monthTaken++
		} else {
			if yearTaken >= take {
				continue
			}
			yearTaken++
		}
		out = append(out, service.LifelineArchiveRow{
			Granularity: rw.Granularity,
			Period:      rw.Period,
			AsOfDate:    rw.AsOfDate,
			Content:     rw.Content,
		})
	}
	// Month entries first (recent substance), year entry as long-horizon tail.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Granularity != out[j].Granularity {
			return out[i].Granularity == "month"
		}
		return out[i].Period > out[j].Period
	})
	return out, nil
}

func (r *dbLifelineReader) GetTopicLifeline(topicID uint) (service.SectionTimelineData, error) {
	var data service.SectionTimelineData

	// 1. Topic info.
	type topicRow struct {
		ID              uint      `gorm:"column:id"`
		Label           string    `gorm:"column:label"`
		Description     string    `gorm:"column:description"`
		Status          string    `gorm:"column:status"`
		Source          string    `gorm:"column:source"`
		FirstSeenDate   time.Time `gorm:"column:first_seen_date"`
		LastSeenDate    time.Time `gorm:"column:last_seen_date"`
		HitCount        int       `gorm:"column:hit_count"`
		ConsecutiveHits int       `gorm:"column:consecutive_hits"`
	}
	var topicRows []topicRow
	if err := r.db.Raw(`
		SELECT id, label, description, status, source,
		       first_seen_date, last_seen_date, hit_count, consecutive_hits
		FROM board_persistent_topics WHERE id = ?
	`, topicID).Scan(&topicRows).Error; err != nil {
		return data, fmt.Errorf("query topic: %w", err)
	}
	if len(topicRows) == 0 {
		return data, fmt.Errorf("topic %d not found", topicID)
	}
	tr := topicRows[0]
	data.Topic = service.TopicBrief{
		ID:              tr.ID,
		Label:           tr.Label,
		Description:     tr.Description,
		Status:          tr.Status,
		Source:          tr.Source,
		FirstSeenDate:   tr.FirstSeenDate,
		LastSeenDate:    tr.LastSeenDate,
		HitCount:        tr.HitCount,
		ConsecutiveHits: tr.ConsecutiveHits,
	}

	// 2. Sections.
	type sectionRow struct {
		ID         uint      `gorm:"column:id"`
		PeriodDate time.Time `gorm:"column:period_date"`
		Label      string    `gorm:"column:label"`
		Confidence string    `gorm:"column:confidence"`
		ArtCount   int       `gorm:"column:art_count"`
		ThreadCnt  int       `gorm:"column:thread_cnt"`
	}
	var sectionRows []sectionRow
	if err := r.db.Raw(`
		SELECT ds.id, bdr.period_date, ds.cluster_label AS label,
		       ds.topic_match_confidence AS confidence,
		       ds.article_count AS art_count,
		       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_cnt
		FROM daily_report_sections ds
		JOIN board_daily_reports bdr ON bdr.id = ds.report_id
		WHERE ds.persistent_topic_id = ?
		ORDER BY bdr.period_date ASC, ds.id ASC
	`, topicID).Scan(&sectionRows).Error; err != nil {
		return data, fmt.Errorf("query sections: %w", err)
	}

	sections := make([]service.TimelineSectionNode, 0, len(sectionRows))
	sectionIDs := make([]uint, 0, len(sectionRows))
	for _, sr := range sectionRows {
		// Load thread titles.
		var titles []string
		if err := r.db.Raw(`SELECT title FROM daily_report_threads WHERE section_id = ? ORDER BY id ASC`, sr.ID).Scan(&titles).Error; err != nil {
			titles = nil
		}
		sections = append(sections, service.TimelineSectionNode{
			SectionID:            sr.ID,
			PeriodDate:           sr.PeriodDate,
			ClusterLabel:         sr.Label,
			TopicMatchConfidence: sr.Confidence,
			ArticleCount:         sr.ArtCount,
			ThreadCount:          sr.ThreadCnt,
			ThreadTitles:         titles,
		})
		sectionIDs = append(sectionIDs, sr.ID)
	}
	data.Sections = sections

	// 3. Section relations.
	if len(sectionIDs) > 0 {
		type relationRow struct {
			FromID       uint    `gorm:"column:from_id"`
			ToID         uint    `gorm:"column:to_id"`
			Distance     float64 `gorm:"column:distance"`
			RelationType string  `gorm:"column:relation_type"`
		}
		var relRows []relationRow
		if err := r.db.Raw(`
			SELECT from_section_id AS from_id, to_section_id AS to_id, distance, relation_type
			FROM daily_report_section_relations
			WHERE from_section_id IN ? OR to_section_id IN ?
		`, sectionIDs, sectionIDs).Scan(&relRows).Error; err != nil {
			return data, fmt.Errorf("query relations: %w", err)
		}
		relations := make([]service.SectionRelation, len(relRows))
		for i, rr := range relRows {
			relations[i] = service.SectionRelation{
				FromID:       rr.FromID,
				ToID:         rr.ToID,
				Distance:     rr.Distance,
				RelationType: rr.RelationType,
			}
		}
		data.Relations = relations
	}

	return data, nil
}
