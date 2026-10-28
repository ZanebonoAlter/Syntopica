package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// ── 重新分桶（add-intra-lane-theme-bucketing design D7/D11）────────────────────
//
// rebucket 不重跑整份日报：它以 (report_id, cluster_index) 定位桶组，复跑两阶段
// 分桶后**整组替换 section 行**，thread 行只迁移 section_id、绝不删除。因此
// 关键事件注记（锚在 thread 上）与文章引用都不会丢。
//
// 与 SaveReport 的串行化（D11）：两者取**同一把** (board, date) 事务级
// advisory lock。SaveReport 同日重跑会整份删旧 sections/threads，若 rebucket
// 不参与同一把锁，会出现「thread 已被删、UPDATE 静默 0 行」或「向已重建的报告
// 插入旧 cluster_index 的幽灵 section」。

// ReportStatusCompleted is the persisted "done" state of a daily report. The
// spec writes it as `done`; the column value has always been "completed"
// (orchestrator + findPreviousReportBrief agree), so the gate keys on this.
const ReportStatusCompleted = "completed"

// Rebucket sentinel errors — the handler maps them to HTTP status codes.
var (
	ErrRebucketReportNotFound  = errors.New("daily report not found")
	ErrRebucketSectionNotFound = errors.New("section not found in this report")
	ErrRebucketReportNotDone   = errors.New("daily report is not completed")
	ErrRebucketWatchGroup      = errors.New("watch-materialized sections cannot be rebucketed")
	ErrRebucketStaleGroup      = errors.New("section group changed while rebucketing (retry)")
)

// RebucketGroup is one (report, cluster_index) bucket group: the sibling
// sections plus every thread they own (threads are the rebucket input).
type RebucketGroup struct {
	Report   BoardDailyReport
	Sections []DailyReportSection // ascending bucket_index, id
	Threads  []DailyReportThread  // ascending id
}

// LoadRebucketGroupMeta resolves the report's (board, period_date) so the
// caller can take the SaveReport advisory lock BEFORE reading the group.
func (r *TopicGraphRepository) LoadRebucketGroupMeta(reportID, sectionID uint) (uint, time.Time, error) {
	var report BoardDailyReport
	if err := r.db.Select("id, semantic_board_id, period_date, status").First(&report, reportID).Error; err != nil {
		return 0, time.Time{}, ErrRebucketReportNotFound
	}
	var count int64
	if err := r.db.Model(&DailyReportSection{}).
		Where("id = ? AND report_id = ?", sectionID, reportID).
		Count(&count).Error; err != nil {
		return 0, time.Time{}, fmt.Errorf("load rebucket section: %w", err)
	}
	if count == 0 {
		return 0, time.Time{}, ErrRebucketSectionNotFound
	}
	return report.SemanticBoardID, NormalizeReportDate(report.PeriodDate), nil
}

// DailyReportSaveLockKey exposes the advisory-lock key derivation so the
// rebucket path and SaveReport provably serialize on the same key (D11).
func DailyReportSaveLockKey(boardID uint, date time.Time) int64 {
	return dailyReportSaveXactKey(boardID, date)
}

// LockDailyReportSave takes the same transaction-level advisory lock SaveReport
// holds for the (board, date) pair. It auto-releases with the transaction.
//
// SQLite (handler unit tests) has no advisory locks; it is a single-writer
// engine there, so skipping the lock keeps those tests on SQLite without
// weakening the Postgres production guarantee.
func LockDailyReportSave(tx *gorm.DB, boardID uint, date time.Time) error {
	if tx.Name() != "postgres" {
		return nil
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", DailyReportSaveLockKey(boardID, date)).Error; err != nil {
		return fmt.Errorf("acquire rebucket advisory lock: %w", err)
	}
	return nil
}

// LoadRebucketGroup loads the whole bucket group inside the caller's
// transaction (after the lock). A missing report/section maps onto the
// sentinel errors so the handler can answer 404.
func LoadRebucketGroup(tx *gorm.DB, reportID, sectionID uint) (*RebucketGroup, error) {
	var report BoardDailyReport
	if err := tx.First(&report, reportID).Error; err != nil {
		return nil, ErrRebucketReportNotFound
	}
	var target DailyReportSection
	if err := tx.Where("id = ? AND report_id = ?", sectionID, reportID).First(&target).Error; err != nil {
		return nil, ErrRebucketSectionNotFound
	}

	var sections []DailyReportSection
	if err := tx.Where("report_id = ? AND cluster_index = ?", reportID, target.ClusterIndex).
		Order("bucket_index ASC, id ASC").Find(&sections).Error; err != nil {
		return nil, fmt.Errorf("load rebucket sections: %w", err)
	}
	if len(sections) == 0 {
		return nil, ErrRebucketSectionNotFound
	}

	sectionIDs := make([]uint, 0, len(sections))
	for _, sec := range sections {
		sectionIDs = append(sectionIDs, sec.ID)
	}
	var threads []DailyReportThread
	if err := tx.Where("section_id IN ?", sectionIDs).Order("id ASC").Find(&threads).Error; err != nil {
		return nil, fmt.Errorf("load rebucket threads: %w", err)
	}
	return &RebucketGroup{Report: report, Sections: sections, Threads: threads}, nil
}

// RebucketBucket is one bucket's derived content, computed by the service
// (LLM split + embedding) before the write side runs.
type RebucketBucket struct {
	SectionTitle     string
	BucketTitle      string
	TagIDs           []uint
	ArticleCount     int
	BestTier         int
	AvgScore         float64
	QualityBreakdown []byte
	Embedding        string
	// ThreadIDs are the EXISTING daily_report_threads rows owned by this bucket
	// (stable identity across the split — threads are never deleted). At least
	// one thread per bucket (repairBuckets guarantees ≥2).
	ThreadIDs []uint
}

// RebucketApplySpec is the write-side input for one group replacement.
type RebucketApplySpec struct {
	ReportID     uint
	BoardID      uint
	PeriodDate   time.Time
	ClusterIndex int
	// TargetSectionID re-locates the group after the lock; it guards against a
	// concurrent same-day rerun that already replaced the rows.
	TargetSectionID uint
	Buckets         []RebucketBucket
}

// ApplyRebucketGroup replaces one bucket group inside the caller's transaction
// and returns the group's newest sections (bucket_index ASC).
//
// It is deliberately diff-based so section IDs churn as little as possible:
// existing rows are UPDATEd in place (bucket 0 keeps the group head's ID, which
// keeps the old `#report-section-<id>` deep link alive), missing buckets are
// CREATEd, surplus rows are DELETEd. Threads keep their rows and only move
// `section_id`; markers / watch hits / annotations are re-derived afterwards.
func ApplyRebucketGroup(tx *gorm.DB, spec RebucketApplySpec) ([]DailyReportSection, error) {
	if len(spec.Buckets) < 2 {
		return nil, fmt.Errorf("rebucket apply needs ≥2 buckets, got %d", len(spec.Buckets))
	}

	group, err := LoadRebucketGroup(tx, spec.ReportID, spec.TargetSectionID)
	if err != nil {
		return nil, err
	}
	// Re-validate INSIDE the lock (D11: the status check must not live outside,
	// otherwise a same-day rerun could have replaced the threads meanwhile).
	if group.Report.Status != ReportStatusCompleted {
		return nil, ErrRebucketReportNotDone
	}
	if group.Sections[0].ClusterIndex != spec.ClusterIndex {
		return nil, ErrRebucketStaleGroup
	}
	head := group.Sections[0]
	if isWatchLaneTier(head.LaneTier) {
		return nil, ErrRebucketWatchGroup
	}

	// Thread → owning group membership check: every requested thread must
	// currently belong to this group, otherwise the group was rewritten.
	groupThreadIDs := make(map[uint]bool, len(group.Threads))
	for _, th := range group.Threads {
		groupThreadIDs[th.ID] = true
	}
	for _, bucket := range spec.Buckets {
		if len(bucket.ThreadIDs) == 0 {
			return nil, fmt.Errorf("rebucket bucket %q owns no thread", bucket.BucketTitle)
		}
		for _, id := range bucket.ThreadIDs {
			if !groupThreadIDs[id] {
				return nil, ErrRebucketStaleGroup
			}
		}
	}

	anchor := topicAnchorForSection(tx, head.PersistentTopicID)

	oldSections := group.Sections
	newSectionIDs := make([]uint, len(spec.Buckets))
	for i, bucket := range spec.Buckets {
		distance := head.TopicMatchDistance
		if len(anchor) > 0 && strings.TrimSpace(bucket.Embedding) != "" {
			if vec, perr := repoParsePgVector(bucket.Embedding); perr == nil && len(vec) > 0 {
				// cosineDistance returns MaxFloat64 for unusable vectors; a real
				// 0.0 (perfect match) is a legitimate distance and must be kept.
				if d := cosineDistance(vec, anchor); !math.IsNaN(d) && d < math.MaxFloat64/2 {
					distance = d
				}
			}
		}
		updates := map[string]any{
			"cluster_label":          truncateRunes(bucket.SectionTitle, 200),
			"bucket_title":           truncateRunes(bucket.BucketTitle, 200),
			"bucket_index":           i,
			"cluster_tag_ids":        marshalUintArrayJSON(bucket.TagIDs),
			"article_count":          bucket.ArticleCount,
			"best_tier":              bucket.BestTier,
			"avg_score":              bucket.AvgScore,
			"quality_breakdown":      bucket.QualityBreakdown,
			"topic_match_distance":   distance,
			"lane_tier":              head.LaneTier,
			"persistent_topic_id":    head.PersistentTopicID,
			"topic_match_confidence": head.TopicMatchConfidence,
		}
		if strings.TrimSpace(bucket.Embedding) != "" {
			updates["embedding"] = bucket.Embedding
		}

		if i < len(oldSections) {
			res := tx.Model(&DailyReportSection{}).Where("id = ?", oldSections[i].ID).Updates(updates)
			if res.Error != nil {
				return nil, fmt.Errorf("update section %d: %w", oldSections[i].ID, res.Error)
			}
			if res.RowsAffected != 1 {
				return nil, fmt.Errorf("update section %d: %d rows affected (want 1)", oldSections[i].ID, res.RowsAffected)
			}
			newSectionIDs[i] = oldSections[i].ID
			continue
		}

		// Surplus bucket: create a fresh row, inheriting the report-time topic
		// status snapshot (history is never back-filled) and the group identity.
		section := DailyReportSection{
			ReportID:             spec.ReportID,
			ClusterIndex:         spec.ClusterIndex,
			ClusterLabel:         truncateRunes(bucket.SectionTitle, 200),
			ClusterTagIDs:        JSON(marshalUintArrayJSON(bucket.TagIDs)),
			ArticleCount:         bucket.ArticleCount,
			BestTier:             bucket.BestTier,
			AvgScore:             bucket.AvgScore,
			QualityBreakdown:     JSON(bucket.QualityBreakdown),
			Embedding:            bucket.Embedding,
			PersistentTopicID:    head.PersistentTopicID,
			TopicMatchDistance:   distance,
			TopicMatchConfidence: head.TopicMatchConfidence,
			TopicStatusAtReport:  head.TopicStatusAtReport,
			LaneTier:             head.LaneTier,
			BucketTitle:          truncateRunes(bucket.BucketTitle, 200),
			BucketIndex:          i,
		}
		if strings.TrimSpace(section.Embedding) == "" {
			if err := tx.Omit("embedding").Create(&section).Error; err != nil {
				return nil, fmt.Errorf("create bucket section %d: %w", i, err)
			}
		} else if err := tx.Create(&section).Error; err != nil {
			return nil, fmt.Errorf("create bucket section %d: %w", i, err)
		}
		newSectionIDs[i] = section.ID
	}

	// Thread → new bucket (row identity preserved; only section_id moves).
	threadToSection := make(map[uint]uint, len(group.Threads))
	for i, bucket := range spec.Buckets {
		if len(bucket.ThreadIDs) == 0 {
			continue
		}
		res := tx.Model(&DailyReportThread{}).
			Where("id IN ?", bucket.ThreadIDs).
			Update("section_id", newSectionIDs[i])
		if res.Error != nil {
			return nil, fmt.Errorf("move threads to bucket %d: %w", i, res.Error)
		}
		if int(res.RowsAffected) != len(bucket.ThreadIDs) {
			// A silent 0-row UPDATE is exactly the concurrent-rerun symptom D11
			// exists to prevent; surface it instead of leaving dangling threads.
			return nil, fmt.Errorf("move threads to bucket %d: %d rows affected (want %d)", i, res.RowsAffected, len(bucket.ThreadIDs))
		}
		for _, id := range bucket.ThreadIDs {
			threadToSection[id] = newSectionIDs[i]
		}
	}

	// Key-event markers re-anchor by thread (never by section): the user's
	// confirmed annotation follows its thread into the new bucket.
	for threadID, sectionID := range threadToSection {
		if err := tx.Model(&TopicKeyEventMarker{}).
			Where("source_thread_id = ?", threadID).
			Update("source_section_id", sectionID).Error; err != nil {
			return nil, fmt.Errorf("re-derive key event marker for thread %d: %w", threadID, err)
		}
	}

	if err := rebucketWatchHits(tx, spec, group, newSectionIDs); err != nil {
		return nil, err
	}
	if err := rebucketAnnotations(tx, spec, oldSections, threadToSection, newSectionIDs[0]); err != nil {
		return nil, err
	}

	// Surplus bucket rows go away only after every referrer has been re-derived.
	if len(oldSections) > len(spec.Buckets) {
		extra := make([]uint, 0, len(oldSections)-len(spec.Buckets))
		for _, sec := range oldSections[len(spec.Buckets):] {
			extra = append(extra, sec.ID)
		}
		if err := tx.Where("id IN ?", extra).Delete(&DailyReportSection{}).Error; err != nil {
			return nil, fmt.Errorf("delete surplus bucket sections: %w", err)
		}
	}

	// Relations are rebuilt board-wide: this deletes both outgoing AND incoming
	// edges of the replaced rows and regenerates similarity + identity edges.
	if err := RebuildBoardRelations(tx, spec.BoardID); err != nil {
		return nil, fmt.Errorf("rebuild relations after rebucket: %w", err)
	}

	var sections []DailyReportSection
	if err := tx.Preload("Threads", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("report_id = ? AND cluster_index = ?", spec.ReportID, spec.ClusterIndex).
		Order("bucket_index ASC, id ASC").Find(&sections).Error; err != nil {
		return nil, fmt.Errorf("reload rebucketed sections: %w", err)
	}
	// The API contract is "threads is always an array" (same normalisation as
	// GetReportByID): GORM leaves a nil slice when a bucket has no thread rows.
	for i := range sections {
		if sections[i].Threads == nil {
			sections[i].Threads = []DailyReportThread{}
		}
	}
	// The response is consumed by the reader's in-place section replacement, so
	// it must carry the same transient topic brief the GET-detail path attaches
	// (otherwise the siblings fall out of their topic group until a reload).
	attachBriefsToSections(tx, sections)
	return sections, nil
}

// rebucketWatchHits re-points the group's watch hits at the bucket that holds
// most of the hit's original section threads (ties → lowest bucket). Duplicate
// (watch, section, report) triples are dropped instead of violating the unique
// index.
func rebucketWatchHits(tx *gorm.DB, spec RebucketApplySpec, group *RebucketGroup, newSectionIDs []uint) error {
	oldSectionIDs := make([]uint, 0, len(group.Sections))
	for _, sec := range group.Sections {
		oldSectionIDs = append(oldSectionIDs, sec.ID)
	}
	oldSectionThreads := make(map[uint][]uint, len(group.Sections))
	for _, th := range group.Threads {
		oldSectionThreads[th.SectionID] = append(oldSectionThreads[th.SectionID], th.ID)
	}

	var hits []struct {
		ID        uint
		WatchID   uint
		SectionID uint
	}
	if err := tx.Table("topic_watch_hits").
		Select("id, watch_id, section_id").
		Where("report_id = ? AND section_id IN ?", spec.ReportID, oldSectionIDs).
		Scan(&hits).Error; err != nil {
		return fmt.Errorf("load watch hits for rebucket: %w", err)
	}
	if len(hits) == 0 {
		return nil
	}

	threadsOfOld := func(sectionID uint) map[uint]bool {
		out := make(map[uint]bool, len(oldSectionThreads[sectionID]))
		for _, id := range oldSectionThreads[sectionID] {
			out[id] = true
		}
		return out
	}
	bestBucket := func(sectionID uint) int {
		members := threadsOfOld(sectionID)
		best, bestCount := 0, -1
		for i, bucket := range spec.Buckets {
			count := 0
			for _, id := range bucket.ThreadIDs {
				if members[id] {
					count++
				}
			}
			if count > bestCount {
				best, bestCount = i, count
			}
		}
		return best
	}

	for _, hit := range hits {
		target := newSectionIDs[bestBucket(hit.SectionID)]
		var conflict int64
		if err := tx.Table("topic_watch_hits").
			Where("watch_id = ? AND section_id = ? AND report_id = ? AND id <> ?", hit.WatchID, target, spec.ReportID, hit.ID).
			Count(&conflict).Error; err != nil {
			return fmt.Errorf("check watch hit conflict: %w", err)
		}
		if conflict > 0 {
			logging.Infof("daily-report rebucket: dropping duplicate watch hit %d (already mapped to section %d)", hit.ID, target)
			if err := tx.Exec("DELETE FROM topic_watch_hits WHERE id = ?", hit.ID).Error; err != nil {
				return fmt.Errorf("delete duplicate watch hit %d: %w", hit.ID, err)
			}
			continue
		}
		if err := tx.Exec("UPDATE topic_watch_hits SET section_id = ? WHERE id = ?", target, hit.ID).Error; err != nil {
			return fmt.Errorf("re-derive watch hit %d: %w", hit.ID, err)
		}
	}
	return nil
}

// rebucketAnnotations re-derives report_annotations.section_id from the note's
// thread; masthead notes without a thread attach to the group head
// (bucket_index=0). Note content (quoted_text / QAs) is never touched.
func rebucketAnnotations(tx *gorm.DB, spec RebucketApplySpec, oldSections []DailyReportSection, threadToSection map[uint]uint, headSectionID uint) error {
	oldSectionIDs := make([]uint, 0, len(oldSections))
	for _, sec := range oldSections {
		oldSectionIDs = append(oldSectionIDs, sec.ID)
	}
	var annotations []struct {
		ID       uint
		ThreadID *uint
	}
	if err := tx.Table("report_annotations").
		Select("id, thread_id").
		Where("report_id = ? AND section_id IN ?", spec.ReportID, oldSectionIDs).
		Scan(&annotations).Error; err != nil {
		return fmt.Errorf("load annotations for rebucket: %w", err)
	}
	for _, ann := range annotations {
		target := headSectionID
		if ann.ThreadID != nil {
			if sectionID, ok := threadToSection[*ann.ThreadID]; ok {
				target = sectionID
			}
		}
		if err := tx.Exec("UPDATE report_annotations SET section_id = ? WHERE id = ?", target, ann.ID).Error; err != nil {
			return fmt.Errorf("re-derive annotation %d: %w", ann.ID, err)
		}
	}
	return nil
}

// topicAnchorForSection returns the section's topic matching anchor (centroid,
// else 首义 embedding). nil when the section has no topic or the topic has no
// usable vector — the caller then keeps the head's recorded distance.
func topicAnchorForSection(tx *gorm.DB, topicID *uint) []float64 {
	if topicID == nil {
		return nil
	}
	var topic BoardPersistentTopic
	if err := tx.Select("id, centroid, embedding").First(&topic, *topicID).Error; err != nil {
		return nil
	}
	return topicAnchorVec(topic)
}

// isWatchLaneTier reports whether the tier belongs to a watch-materialized
// section (those are owned by the watch track and reject rebucketing). The
// tier values are defined in the service lane file; duplicated as literals here
// to avoid a repository→service import cycle.
func isWatchLaneTier(tier string) bool {
	return strings.HasPrefix(tier, "watch_")
}

// truncateRunes caps s at max runes (DB column budgets).
func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// marshalUintArrayJSON marshals a uint slice as a JSON array, normalizing nil /
// empty to "[]" (jsonb_array_elements_text rejects a bare null scalar).
func marshalUintArrayJSON(ids []uint) []byte {
	if len(ids) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// ListTagInputsByIDs loads the minimal TagInput fields the embedding text
// assembly needs (label + description) for the given tag ids. Missing ids are
// simply absent from the map — rebucket keeps going with the ids it has.
func ListTagInputsByIDs(db *gorm.DB, ids []uint) (map[uint]TagInput, error) {
	out := make(map[uint]TagInput, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []TagInput
	if err := db.Table("topic_tags").
		Select("id, label, description").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load tag inputs: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}
