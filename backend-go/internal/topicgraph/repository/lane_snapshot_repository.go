package repository

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm/clause"
)

// ── 泳道动态（overview-lane-dynamics）────────────────────────────────────────
//
// 两块职责：
//  1. 结算侧（design D1/D3）：MAX(period_date) 锚定的 14 天素材读取 + 快照 upsert；
//  2. 聚合侧（design D4）：GET /semantic-boards/:id/lane-dynamics 单实现查询。

const (
	// LaneSnapshotMaxPerBoard caps settlements per board per report day
	// (design D1): aligned with the situation-card 12-lane precedent, guards
	// lane-heavy boards against stalling the settlement tail.
	LaneSnapshotMaxPerBoard = 20
	// LaneSnapshotThreadTitlesPerSection bounds thread titles per section in
	// the settlement material (design D3: 前 3 条 thread 标题).
	LaneSnapshotThreadTitlesPerSection = 3
	// LaneTimelineEventsPerSection bounds thread titles per section in the
	// dynamics timeline; the rest folds into folded_count (design D4).
	LaneTimelineEventsPerSection = 5
	// LaneDynamicsDefaultDays is the default window for the endpoint
	// (spec: days 参数默认 14).
	LaneDynamicsDefaultDays = 14
)

// GetBoardReportAnchor returns the board's latest report period_date
// (normalized noon UTC). ok=false when the board has no reports — the caller
// uses it to distinguish the no-report empty state (tasks 2.2).
// Uses the GORM struct-scan path (not sql.Row.Scan) so time columns coming
// back as strings (SQLite driver in service tests) are converted the same
// way every other period_date scan in this package is.
func (r *TopicGraphRepository) GetBoardReportAnchor(boardID uint) (time.Time, bool, error) {
	var row struct {
		PeriodDate time.Time
	}
	err := r.db.Model(&BoardDailyReport{}).
		Select("period_date").
		Where("semantic_board_id = ?", boardID).
		Order("period_date DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return time.Time{}, false, fmt.Errorf("lane snapshot: board report anchor: %w", err)
	}
	if row.PeriodDate.IsZero() {
		return time.Time{}, false, nil
	}
	return NormalizeReportDate(row.PeriodDate), true, nil
}

// ListActiveLanesForSnapshot returns the board's active topics ordered by
// last_seen_date DESC (tie: id ASC). limit<=0 means no limit; the settlement
// passes LaneSnapshotMaxPerBoard to clamp (design D1).
func (r *TopicGraphRepository) ListActiveLanesForSnapshot(boardID uint, limit int) ([]BoardPersistentTopic, error) {
	q := r.db.Where("semantic_board_id = ? AND status = ?", boardID, TopicStatusActive).
		Order("last_seen_date DESC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var topics []BoardPersistentTopic
	if err := q.Find(&topics).Error; err != nil {
		return nil, fmt.Errorf("lane snapshot: list active lanes: %w", err)
	}
	return topics, nil
}

// LaneMaterialRow is one section's contribution to the settlement prompt
// (design D3): date + section title + top-N thread titles.
type LaneMaterialRow struct {
	PeriodDate   time.Time
	SectionID    uint
	SectionLabel string
	ThreadTitles []string
}

// ListLaneSnapshotMaterial loads a lane's in-window anchored sections
// (date ASC, section id ASC) with up to threadsPerSection thread titles each.
// from/to are inclusive; the window is anchored at MAX(period_date), not
// now(), so it never drifts on days the report did not run (design D3).
func (r *TopicGraphRepository) ListLaneSnapshotMaterial(topicID uint, from, to time.Time, threadsPerSection int) ([]LaneMaterialRow, error) {
	if threadsPerSection <= 0 {
		threadsPerSection = LaneSnapshotThreadTitlesPerSection
	}
	var secs []struct {
		ID           uint
		ClusterLabel string
		PeriodDate   time.Time
	}
	err := r.db.Table("daily_report_sections AS s").
		Select("s.id, s.cluster_label, r.period_date").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Where("s.persistent_topic_id = ? AND r.period_date >= ? AND r.period_date <= ?", topicID, from, to).
		Order("r.period_date ASC, s.id ASC").
		Scan(&secs).Error
	if err != nil {
		return nil, fmt.Errorf("lane snapshot: material sections: %w", err)
	}
	if len(secs) == 0 {
		return nil, nil
	}

	sectionIDs := make([]uint, 0, len(secs))
	for _, s := range secs {
		sectionIDs = append(sectionIDs, s.ID)
	}
	// Threads batch-load in generation order (id ASC) so the top-N slice is
	// stable across runs.
	var ths []struct {
		SectionID uint
		Title     string
	}
	if err := r.db.Table("daily_report_threads").
		Select("section_id, title").
		Where("section_id IN ?", sectionIDs).
		Order("id ASC").
		Scan(&ths).Error; err != nil {
		return nil, fmt.Errorf("lane snapshot: material threads: %w", err)
	}
	titlesBySection := make(map[uint][]string, len(secs))
	for _, th := range ths {
		if len(titlesBySection[th.SectionID]) < threadsPerSection {
			titlesBySection[th.SectionID] = append(titlesBySection[th.SectionID], th.Title)
		}
	}

	rows := make([]LaneMaterialRow, 0, len(secs))
	for _, s := range secs {
		rows = append(rows, LaneMaterialRow{
			PeriodDate:   NormalizeReportDate(s.PeriodDate),
			SectionID:    s.ID,
			SectionLabel: s.ClusterLabel,
			ThreadTitles: titlesBySection[s.ID],
		})
	}
	return rows, nil
}

// UpsertLaneSnapshot inserts or overwrites the lane's single rolling snapshot
// row (ON CONFLICT (persistent_topic_id) DO UPDATE) — settlement is idempotent:
// same-day report reruns replace the row instead of duplicating it.
func (r *TopicGraphRepository) UpsertLaneSnapshot(snap *TopicLaneSnapshot) error {
	if err := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "persistent_topic_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"rolling_summary", "as_of_date", "updated_at",
		}),
	}).Create(snap).Error; err != nil {
		return fmt.Errorf("lane snapshot: upsert: %w", err)
	}
	return nil
}

// GetLaneSnapshotsByTopicIDs batch-loads snapshots into a map keyed by topic
// id. Missing topics are simply absent from the map (→ 待结算 null).
func (r *TopicGraphRepository) GetLaneSnapshotsByTopicIDs(topicIDs []uint) (map[uint]TopicLaneSnapshot, error) {
	out := make(map[uint]TopicLaneSnapshot, len(topicIDs))
	if len(topicIDs) == 0 {
		return out, nil
	}
	var snaps []TopicLaneSnapshot
	if err := r.db.Where("persistent_topic_id IN ?", topicIDs).Find(&snaps).Error; err != nil {
		return nil, fmt.Errorf("lane snapshot: list by topics: %w", err)
	}
	for _, s := range snaps {
		out[s.PersistentTopicID] = s
	}
	return out, nil
}

// ListActiveWatchLinkedTopicIDs returns topic ids referenced by ACTIVE watches
// of the board. Only sentence_topic watches ever set persistent_topic_id, so
// this is exactly the 「我在追踪」 lane set (design D4 watch_linked 判定).
func (r *TopicGraphRepository) ListActiveWatchLinkedTopicIDs(boardID uint) (map[uint]bool, error) {
	var ids []uint
	err := r.db.Model(&BoardTopicWatch{}).
		Where("semantic_board_id = ? AND status = ? AND persistent_topic_id IS NOT NULL",
			boardID, WatchStatusActive).
		Distinct().
		Pluck("persistent_topic_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("lane dynamics: watch-linked topics: %w", err)
	}
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ── 聚合端点响应形状（design D4）────────────────────────────────────────────

// LaneDynamicsTimelineSection is one section node in a lane's timeline:
// section label + up to LaneTimelineEventsPerSection thread titles
// (events) + folded_count for the rest (spec: 数量可截断但 SHALL 提示被折叠数).
type LaneDynamicsTimelineSection struct {
	SectionID   uint     `json:"section_id"`
	Label       string   `json:"label"`
	Events      []string `json:"events"`
	FoldedCount int      `json:"folded_count"`
}

// LaneDynamicsTimelineDay groups one report date's sections (ascending
// chronological order; the date↔event mapping comes from the backend so the
// frontend never infers it — spec).
type LaneDynamicsTimelineDay struct {
	Date     string                        `json:"date"` // YYYY-MM-DD
	Sections []LaneDynamicsTimelineSection `json:"sections"`
}

// LaneDynamicsSnapshot is the settled rolling summary; nil on the lane means
// "no snapshot yet" (前端「待结算」占位，时间线照常渲染 — spec).
type LaneDynamicsSnapshot struct {
	Summary string `json:"summary"`
	AsOf    string `json:"as_of"` // YYYY-MM-DD
}

// LaneDynamicsLane is one lane card's full data.
type LaneDynamicsLane struct {
	TopicID         uint                      `json:"topic_id"`
	Label           string                    `json:"label"`
	WatchLinked     bool                      `json:"watch_linked"`
	SectionCount14d int                       `json:"section_count_14d"` // 排序键（D4 命名）
	Snapshot        *LaneDynamicsSnapshot     `json:"snapshot"`
	Timeline        []LaneDynamicsTimelineDay `json:"timeline"`
}

// LaneDynamicsCandidate is one candidate-bar entry (只读提示，转正走话题管理).
type LaneDynamicsCandidate struct {
	TopicID      uint   `json:"topic_id"`
	Label        string `json:"label"`
	LastSeenDate string `json:"last_seen_date"`
	RecentHint   string `json:"recent_hint"` // 最新 section 标题
}

// LaneDynamicsResponse is the GET /semantic-boards/:id/lane-dynamics payload.
// Empty states (tasks 2.2): no reports → HasReports=false + empty non-nil
// lanes/candidates; reports but no active lanes → HasReports=true + empty lanes.
type LaneDynamicsResponse struct {
	WindowDays int                     `json:"window_days"`
	HasReports bool                    `json:"has_reports"`
	Lanes      []LaneDynamicsLane      `json:"lanes"`
	Candidates []LaneDynamicsCandidate `json:"candidates"`
}

// laneSectionRow mirrors the windowed anchored-section projection shared by
// the timeline and the candidate hint queries.
type laneSectionRow struct {
	ID           uint
	TopicID      uint
	ClusterLabel string
	PeriodDate   time.Time
}

// listLaneWindowSections loads all sections anchored to topics of this board
// within [from, anchor] (inclusive), ordered oldest-first. GORM-safe standard
// SQL (JOIN sections↔reports), PG/SQLite compatible.
func (r *TopicGraphRepository) listLaneWindowSections(boardID uint, from, to time.Time, topicIDs []uint) ([]laneSectionRow, error) {
	q := r.db.Table("daily_report_sections AS s").
		Select("s.id, s.persistent_topic_id AS topic_id, s.cluster_label, r.period_date").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Where("r.semantic_board_id = ? AND r.period_date >= ? AND r.period_date <= ? AND s.persistent_topic_id IS NOT NULL",
			boardID, from, to)
	if topicIDs != nil {
		q = q.Where("s.persistent_topic_id IN ?", topicIDs)
	}
	var rows []laneSectionRow
	if err := q.Order("r.period_date ASC, s.id ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("lane dynamics: window sections: %w", err)
	}
	return rows, nil
}

// listLaneSectionThreads batch-loads thread titles for the given sections in
// generation order (id ASC) so the top-N + fold split is deterministic.
func (r *TopicGraphRepository) listLaneSectionThreads(sectionIDs []uint) (map[uint][]string, error) {
	out := make(map[uint][]string, len(sectionIDs))
	if len(sectionIDs) == 0 {
		return out, nil
	}
	var ths []struct {
		SectionID uint
		Title     string
	}
	if err := r.db.Table("daily_report_threads").
		Select("section_id, title").
		Where("section_id IN ?", sectionIDs).
		Order("id ASC").
		Scan(&ths).Error; err != nil {
		return nil, fmt.Errorf("lane dynamics: section threads: %w", err)
	}
	for _, th := range ths {
		out[th.SectionID] = append(out[th.SectionID], th.Title)
	}
	return out, nil
}

// GetBoardLaneDynamics is the single-shot aggregate behind
// GET /semantic-boards/:id/lane-dynamics?days=14 (design D4):
//   - window anchored at the board's MAX(period_date) going back `days` days
//     (inclusive both ends — same convention as the landscape date axis), NOT
//     now(), so 态势句素材与时间线同窗 (spec);
//   - lanes = active ∪ watch-linked (non-archived), ranked by in-window
//     section count DESC; zero-count (沉寂) lanes are not returned;
//   - candidates = visible-gate candidates (hit_count >= upgrade_threshold,
//     FilterVisibleTopics 口径) with their latest section title as hint.
func (r *TopicGraphRepository) GetBoardLaneDynamics(boardID uint, days int) (*LaneDynamicsResponse, error) {
	if days <= 0 {
		days = LaneDynamicsDefaultDays
	}
	resp := &LaneDynamicsResponse{
		WindowDays: days,
		Lanes:      []LaneDynamicsLane{},
		Candidates: []LaneDynamicsCandidate{},
	}

	anchor, ok, err := r.GetBoardReportAnchor(boardID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// 无日报：空数组非 null（tasks 2.2）。
		return resp, nil
	}
	resp.HasReports = true
	from := anchor.AddDate(0, 0, -days)

	watchLinked, err := r.ListActiveWatchLinkedTopicIDs(boardID)
	if err != nil {
		return nil, err
	}

	// Non-archived topics of the board (lanes draw from active ∪ watch-linked;
	// archived lanes are excluded either way).
	var topics []BoardPersistentTopic
	if err := r.db.Where("semantic_board_id = ? AND status != ?", boardID, TopicStatusArchived).
		Find(&topics).Error; err != nil {
		return nil, fmt.Errorf("lane dynamics: board topics: %w", err)
	}
	topicByID := make(map[uint]BoardPersistentTopic, len(topics))
	for _, t := range topics {
		topicByID[t.ID] = t
	}

	secs, err := r.listLaneWindowSections(boardID, from, anchor, nil)
	if err != nil {
		return nil, err
	}

	// Group sections per lane-set topic; lanes with zero in-window sections
	// never materialize (沉寂不展示 — spec).
	type laneAgg struct {
		topic  BoardPersistentTopic
		count  int
		byDate map[string][]laneSectionRow
		dates  []string
	}
	aggs := make(map[uint]*laneAgg)
	ensureAgg := func(topicID uint) *laneAgg {
		a, exists := aggs[topicID]
		if !exists {
			a = &laneAgg{topic: topicByID[topicID], byDate: map[string][]laneSectionRow{}}
			aggs[topicID] = a
		}
		return a
	}
	for _, s := range secs {
		t, exists := topicByID[s.TopicID]
		if !exists {
			continue
		}
		// Lane set: active ∪ watch-linked (archived excluded above either way).
		if t.Status != TopicStatusActive && !watchLinked[t.ID] {
			continue
		}
		a := ensureAgg(s.TopicID)
		dateKey := NormalizeReportDate(s.PeriodDate).Format("2006-01-02")
		if _, seen := a.byDate[dateKey]; !seen {
			a.dates = append(a.dates, dateKey) // secs are date-ASC → dates ASC
		}
		a.byDate[dateKey] = append(a.byDate[dateKey], s)
		a.count++
	}

	// Threads + snapshots for the displayed lanes.
	laneIDs := make([]uint, 0, len(aggs))
	sectionIDs := make([]uint, 0)
	for id := range aggs {
		laneIDs = append(laneIDs, id)
	}
	for _, id := range laneIDs {
		for _, list := range aggs[id].byDate {
			for _, s := range list {
				sectionIDs = append(sectionIDs, s.ID)
			}
		}
	}
	threadsBySection, err := r.listLaneSectionThreads(sectionIDs)
	if err != nil {
		return nil, err
	}
	snaps, err := r.GetLaneSnapshotsByTopicIDs(laneIDs)
	if err != nil {
		return nil, err
	}

	for _, id := range laneIDs {
		a := aggs[id]
		lane := LaneDynamicsLane{
			TopicID:         id,
			Label:           a.topic.Label,
			WatchLinked:     watchLinked[id],
			SectionCount14d: a.count,
			Timeline:        make([]LaneDynamicsTimelineDay, 0, len(a.dates)),
		}
		if snap, ok := snaps[id]; ok {
			lane.Snapshot = &LaneDynamicsSnapshot{
				Summary: snap.RollingSummary,
				AsOf:    NormalizeReportDate(snap.AsOfDate).Format("2006-01-02"),
			}
		}
		// Timeline renders newest-first (user-facing contract: latest day on
		// top), so emit days in reverse collection order (dates collected ASC).
		for i := len(a.dates) - 1; i >= 0; i-- {
			dateKey := a.dates[i]
			day := LaneDynamicsTimelineDay{Date: dateKey, Sections: []LaneDynamicsTimelineSection{}}
			for _, s := range a.byDate[dateKey] {
				titles := threadsBySection[s.ID]
				events := titles
				if len(events) > LaneTimelineEventsPerSection {
					events = events[:LaneTimelineEventsPerSection]
				}
				if events == nil {
					events = []string{}
				}
				folded := len(titles) - len(events)
				day.Sections = append(day.Sections, LaneDynamicsTimelineSection{
					SectionID:   s.ID,
					Label:       s.ClusterLabel,
					Events:      events,
					FoldedCount: folded,
				})
			}
			lane.Timeline = append(lane.Timeline, day)
		}
		resp.Lanes = append(resp.Lanes, lane)
	}

	// Rank by in-window section count DESC (spec: 活跃排序); tie: id ASC for
	// deterministic output.
	sort.SliceStable(resp.Lanes, func(i, j int) bool {
		if resp.Lanes[i].SectionCount14d != resp.Lanes[j].SectionCount14d {
			return resp.Lanes[i].SectionCount14d > resp.Lanes[j].SectionCount14d
		}
		return resp.Lanes[i].TopicID < resp.Lanes[j].TopicID
	})

	// Candidate bar: visible-gate candidates (hit_count >= upgrade_threshold —
	// FilterVisibleTopics 口径 restricted to status=candidate) with their
	// latest section title as hint (最近动向，不window限—最新一条可在窗口外).
	cfg := LoadPersistentTopicConfig(r.db)
	var candIDs []uint
	for _, t := range topics {
		if t.Status == TopicStatusCandidate && t.HitCount >= cfg.UpgradeThreshold {
			candIDs = append(candIDs, t.ID)
		}
	}
	if len(candIDs) > 0 {
		var hintRows []laneSectionRow
		if err := r.db.Table("daily_report_sections AS s").
			Select("s.id, s.persistent_topic_id AS topic_id, s.cluster_label, r.period_date").
			Joins("JOIN board_daily_reports r ON r.id = s.report_id").
			Where("s.persistent_topic_id IN ?", candIDs).
			Order("r.period_date DESC, s.id ASC").
			Scan(&hintRows).Error; err != nil {
			return nil, fmt.Errorf("lane dynamics: candidate hints: %w", err)
		}
		hintByTopic := make(map[uint]string, len(candIDs))
		for _, h := range hintRows {
			if _, ok := hintByTopic[h.TopicID]; !ok {
				hintByTopic[h.TopicID] = h.ClusterLabel
			}
		}
		for _, id := range candIDs {
			t := topicByID[id]
			resp.Candidates = append(resp.Candidates, LaneDynamicsCandidate{
				TopicID:      id,
				Label:        t.Label,
				LastSeenDate: NormalizeReportDate(t.LastSeenDate).Format("2006-01-02"),
				RecentHint:   hintByTopic[id],
			})
		}
		// Stable order: most recently seen first (tie: id ASC).
		sort.SliceStable(resp.Candidates, func(i, j int) bool {
			li, lj := topicByID[resp.Candidates[i].TopicID], topicByID[resp.Candidates[j].TopicID]
			if !li.LastSeenDate.Equal(lj.LastSeenDate) {
				return li.LastSeenDate.After(lj.LastSeenDate)
			}
			return li.ID < lj.ID
		})
	}

	return resp, nil
}
