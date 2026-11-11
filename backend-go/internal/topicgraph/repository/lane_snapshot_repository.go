package repository

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm/clause"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
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

// LaneDailyCount is one report day's magnitude signal for the settlement
// prompt (slim-lane-snapshot-trend D1): date + anchored section count +
// thread count. System-computed — the LLM never counts for itself.
type LaneDailyCount struct {
	PeriodDate time.Time
	SectionCnt int
	ThreadCnt  int
}

// ListDailyThreadCounts aggregates, per report day in [from, to], how many
// anchored sections and threads the lane has (slim-lane-snapshot-trend D1:
// 量级信号由系统计算供给). Days without a report produce no row — the
// caller renders them explicitly rather than leaving gaps to interpretation.
// Same inclusive window semantics as ListLaneSnapshotMaterial so the
// magnitude signal always covers exactly the material window.
func (r *TopicGraphRepository) ListDailyThreadCounts(topicID uint, from, to time.Time) ([]LaneDailyCount, error) {
	var rows []LaneDailyCount
	err := r.db.Table("daily_report_sections AS s").
		Select("r.period_date AS period_date, COUNT(DISTINCT s.id) AS section_cnt, COUNT(t.id) AS thread_cnt").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Joins("LEFT JOIN daily_report_threads t ON t.section_id = s.id").
		Where("s.persistent_topic_id = ? AND r.period_date >= ? AND r.period_date <= ?", topicID, from, to).
		Group("r.period_date").
		Order("r.period_date ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("lane snapshot: daily thread counts: %w", err)
	}
	return rows, nil
}

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
			"rolling_summary", "rolling_detail", "as_of_date", "updated_at",
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
	ThreadCount int      `json:"thread_count"`
	Events      []string `json:"events"`
	FoldedCount int      `json:"folded_count"`
}

// LaneDynamicsTimelineDay groups one report date's sections (ascending
// chronological order; the date↔event mapping comes from the backend so the
// frontend never infers it — spec).
//
// Story aggregates (add-event-story-heat task 5.1): StoryCount is the day's
// DEDUPLICATED event-line count across ALL of the lane's sections (thread
// story_id resolved through merged_into survivors — the same story split into
// sibling buckets counts once, never a bucket max). Degraded boards
// (stories_backfilled=false) report the day's non-composite thread count as
// an upper bound instead of masquerading as zero. MaxParticipantCount is the
// hottest survivor's board_stories.participant_count (degraded: distinct
// feeds behind the day's non-composite threads). UnassignedEventCount counts
// story_id-NULL non-composite threads — title/aria payload only, never a
// heat or ordering input.
type LaneDynamicsTimelineDay struct {
	Date                 string                        `json:"date"` // YYYY-MM-DD
	Sections             []LaneDynamicsTimelineSection `json:"sections"`
	StoryCount           int                           `json:"story_count"`
	MaxParticipantCount  int                           `json:"max_participant_count"`
	HottestStoryID       *uint                         `json:"hottest_story_id"`
	HottestStoryTitle    string                        `json:"hottest_story_title"`
	UnassignedEventCount int                           `json:"unassigned_event_count"`
}

// LaneDynamicsSnapshot is the settled rolling summary; nil on the lane means
// "no snapshot yet" (前端「待结算」占位，时间线照常渲染 — spec).
// Detail carries the legacy long-form narrative (存量历史值透出; slim-lane-snapshot-trend: 新结算恒空→nil)
// (legacy rows / degraded generations — lane-trend-overview design D3)，与
// 整份快照缺失（snapshot=null）两级缺失可区分；空串归一为 nil。
type LaneDynamicsSnapshot struct {
	Summary string  `json:"summary"`
	AsOf    string  `json:"as_of"`            // YYYY-MM-DD
	Detail  *string `json:"detail,omitempty"` // 存量历史长版透出；新结算恒空→nil（omitempty 不出现）
}

// LaneDynamicsLane is one lane card's full data.
type LaneDynamicsLane struct {
	TopicID     uint   `json:"topic_id"`
	Label       string `json:"label"`
	WatchLinked bool   `json:"watch_linked"`
	// SectionCount14d is the in-window anchored SECTION count, kept for
	// display only. Ordering uses lane_heat (see GetBoardLaneDynamics),
	// so a day split into several sibling buckets cannot inflate a lane's rank.
	SectionCount14d int `json:"section_count_14d"`
	// ThreadCount14d is the in-window clue (thread) count — kept for display /
	// compatibility (a bucket split must not masquerade as more heat).
	ThreadCount14d int `json:"thread_count_14d"`
	// LaneHeat is ComputeBoardHeat's laneHeat[topic_id] at the window's
	// anchorEnd (72h half-life / 14d window — story-heat evidence, never
	// report geometry). Zero when the lane has no signal evidence.
	LaneHeat float64 `json:"lane_heat"`
	// StoryCount14d sums the window's daily StoryCount (deduplicated event
	// lines; degraded boards: sum of non-composite upper bounds).
	StoryCount14d int                       `json:"story_count_14d"`
	Snapshot      *LaneDynamicsSnapshot     `json:"snapshot"`
	Timeline      []LaneDynamicsTimelineDay `json:"timeline"`
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
//
// Heat meta (add-event-story-heat task 5.1): StoriesBackfilled tells whether
// ANY in-window thread carries a story_id — false ⇒ every day degrades to
// non-composite upper bounds and the matrix must flag 「事件身份未回填」.
// HeatRuleVersion pins the deterministic formula behind lane_heat.
// HeatIncomplete / HeatStaleFeedIDs mirror BoardHeatCoverage (stale feeds ⇒
// heat evidence may lag); coverage failure degrades to false/empty with a
// warning and never blocks the response.
type LaneDynamicsResponse struct {
	WindowDays        int                     `json:"window_days"`
	HasReports        bool                    `json:"has_reports"`
	StoriesBackfilled bool                    `json:"stories_backfilled"`
	HeatRuleVersion   string                  `json:"heat_rule_version"`
	HeatIncomplete    bool                    `json:"heat_incomplete"`
	HeatStaleFeedIDs  []uint                  `json:"heat_stale_feed_ids"`
	Lanes             []LaneDynamicsLane      `json:"lanes"`
	Candidates        []LaneDynamicsCandidate `json:"candidates"`
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

// laneThreadStat is one thread's story-identity projection — everything the
// day-level story aggregates (task 5.1) need beyond the title list.
type laneThreadStat struct {
	StoryID           *uint
	Scope             string
	RelatedArticleIDs JSON
}

// listLaneSectionThreadStats batch-loads the story/scope/evidence columns of
// the given sections' threads in generation order (id ASC). Standard GORM
// scan — PG/SQLite compatible.
func (r *TopicGraphRepository) listLaneSectionThreadStats(sectionIDs []uint) (map[uint][]laneThreadStat, error) {
	out := make(map[uint][]laneThreadStat, len(sectionIDs))
	if len(sectionIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SectionID         uint
		StoryID           *uint
		Scope             string
		RelatedArticleIDs JSON
	}
	if err := r.db.Table("daily_report_threads").
		Select("section_id, story_id, scope, related_article_ids").
		Where("section_id IN ?", sectionIDs).
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("lane dynamics: section thread stats: %w", err)
	}
	for _, row := range rows {
		out[row.SectionID] = append(out[row.SectionID], laneThreadStat{
			StoryID:           row.StoryID,
			Scope:             row.Scope,
			RelatedArticleIDs: row.RelatedArticleIDs,
		})
	}
	return out, nil
}

// laneStoryRow is the survivor-resolution projection of one board_stories row.
type laneStoryRow struct {
	Title            string
	ID               uint
	MergedInto       *uint
	ParticipantCount int
}

// laneStoryAggregates carries the board-window story evidence shared by every
// per-day fold (task 5.1). Backfilled boards resolve thread story ids through
// merged_into to distinct survivors; un-backfilled boards degrade to
// non-composite counts + content-identity participants (articles lookup).
type laneStoryAggregates struct {
	backfilled bool
	// survivors: thread story id → survivor id (merged_into resolved, cycle-guarded).
	survivors map[uint]uint
	// participants: survivor id → board_stories.participant_count.
	participants map[uint]int
	titles       map[uint]string
	// participantByArticle: article id → 参与者去重键（降级模式的证据文章），
	// 内容身份优先（规范化链接）、feed 兜底——与 story_signals 的
	// COALESCE(NULLIF(content_key,''), participant_key) 同一口径
	// （fix-duplicate-article-heat design D3）。
	participantByArticle map[uint]string
}

// loadLaneStoryAggregates prepares the fold evidence from the window's thread
// stats. Survivor resolution loads ALL of the board's stories (same precedent
// as loadBoardHeatEvidence) so merge chains resolve regardless of which ids
// the window happens to reference; the merge invariant is one hop, so deeper
// chains are anomalies — resolved best-effort, warned, cycle-guarded.
func (r *TopicGraphRepository) loadLaneStoryAggregates(boardID uint, statsBySection map[uint][]laneThreadStat) (laneStoryAggregates, error) {
	agg := laneStoryAggregates{
		survivors:            make(map[uint]uint),
		participants:         make(map[uint]int),
		titles:               make(map[uint]string),
		participantByArticle: make(map[uint]string),
	}
	articleIDs := make([]uint, 0)
	for _, stats := range statsBySection {
		for _, st := range stats {
			if st.StoryID != nil {
				agg.backfilled = true
				continue
			}
			if st.Scope != ThreadScopeComposite {
				for _, id := range jsonUintIDs(st.RelatedArticleIDs) {
					articleIDs = appendUniqueUint(articleIDs, id)
				}
			}
		}
	}
	if agg.backfilled {
		var stories []laneStoryRow
		if err := r.db.Table("board_stories").
			Select("id, title, merged_into, participant_count").
			Where("board_id = ?", boardID).
			Scan(&stories).Error; err != nil {
			return agg, fmt.Errorf("lane dynamics: board stories: %w", err)
		}
		byID := make(map[uint]laneStoryRow, len(stories))
		for _, s := range stories {
			byID[s.ID] = s
		}
		for _, story := range stories {
			cur, hops, valid := story, 0, true
			seen := map[uint]bool{cur.ID: true}
			for cur.MergedInto != nil {
				next, ok := byID[*cur.MergedInto]
				if !ok || seen[next.ID] {
					valid = false
					break
				}
				seen[next.ID] = true
				cur = next
				hops++
			}
			if !valid {
				logging.Warnf("lane dynamics: board %d skips unresolvable story %d", boardID, story.ID)
				continue
			}
			if hops > 1 {
				logging.Warnf("lane dynamics: board %d story %d resolved an unexpected %d-hop merge chain", boardID, story.ID, hops)
			}
			agg.survivors[story.ID] = cur.ID
			agg.participants[cur.ID] = cur.ParticipantCount
			agg.titles[cur.ID] = cur.Title
		}
	} else if len(articleIDs) > 0 {
		// 未回填降级（design §8.3）：参与者按当日非综合稿 thread 的证据文章
		// 取「内容身份优先」的去重键——不要用事件行数冒充。
		// 口径与 story_signals 一致（design D3）：规范化链接相同 → 1 个参与者；
		// link 为空（内容身份不可判定）→ 退化为 feed:<id>，不同 feed 不合并。
		// 规范化只在 Go 侧做（textutil.NormalizeArticleLink，链接清洗的唯一
		// 实现），SQL 侧不再实现一遍。
		var arts []struct {
			ID     uint
			FeedID uint
			Link   string
		}
		if err := r.db.Table("articles").
			Select("id, feed_id, link").
			Where("id IN ?", articleIDs).
			Scan(&arts).Error; err != nil {
			return agg, fmt.Errorf("lane dynamics: fallback article participants: %w", err)
		}
		for _, a := range arts {
			agg.participantByArticle[a.ID] = heatParticipantKey(
				textutil.NormalizeArticleLink(a.Link),
				"feed:"+strconv.FormatUint(uint64(a.FeedID), 10))
		}
	}
	return agg, nil
}

// hottestStory binds the winner to the same batched evidence as foldDay.
func (a laneStoryAggregates) hottestStory(sections []laneSectionRow, stats map[uint][]laneThreadStat) (*uint, string) {
	var winner uint
	for _, section := range sections {
		for _, thread := range stats[section.ID] {
			if thread.StoryID == nil {
				continue
			}
			id := a.survivors[*thread.StoryID]
			if id != 0 && (winner == 0 || a.participants[id] > a.participants[winner] || (a.participants[id] == a.participants[winner] && id < winner)) {
				winner = id
			}
		}
	}
	if winner == 0 {
		return nil, ""
	}
	return &winner, a.titles[winner]
}

// foldDay turns one lane-day's sections + thread stats into the day-level
// story aggregates. Backfilled mode counts DISTINCT survivors across sections
// and takes the max participant_count over them; degraded mode counts
// non-composite threads (upper bound) and distinct content-identity keys
// (participants, feed as fallback).
// Unassigned (non-composite, story_id NULL) is counted in both modes.
func (a laneStoryAggregates) foldDay(sections []laneSectionRow, statsBySection map[uint][]laneThreadStat) (storyCount, maxParticipants, unassigned int) {
	daySurvivors := make(map[uint]bool)
	for _, s := range sections {
		for _, st := range statsBySection[s.ID] {
			if st.StoryID == nil {
				if st.Scope == ThreadScopeComposite {
					continue // 综合稿不建事件身份，也不算未归组
				}
				unassigned++
				if !a.backfilled {
					// 降级模式下每个非综合稿 thread 当作一条独立事件线：
					// 参与者取「该事件自身」的 distinct 内容身份键（feed 兜底），
					// 再对事件取最大；不能把当天所有事件的键并集当成最热事件
					// （会虚高颜色）。
					storyCount++
					threadParticipants := make(map[string]bool)
					for _, articleID := range jsonUintIDs(st.RelatedArticleIDs) {
						if key, ok := a.participantByArticle[articleID]; ok {
							threadParticipants[key] = true
						}
					}
					if len(threadParticipants) > maxParticipants {
						maxParticipants = len(threadParticipants)
					}
				}
				continue
			}
			survivor, ok := a.survivors[*st.StoryID]
			if !ok {
				continue // broken story link — warned at load, never counted
			}
			if daySurvivors[survivor] {
				continue
			}
			daySurvivors[survivor] = true
			if p := a.participants[survivor]; p > maxParticipants {
				maxParticipants = p
			}
		}
	}
	if a.backfilled {
		storyCount = len(daySurvivors)
	}
	return storyCount, maxParticipants, unassigned
}

// laneHeatAnchorEnd converts the window anchor (noon UTC of the latest report
// day) into that day's CST 23:59:59 — the heat instant lane_heat is computed
// at (same zone convention as story_heat.go).
func laneHeatAnchorEnd(anchor time.Time) time.Time {
	local := anchor.In(storyHeatZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 23, 59, 59, 0, storyHeatZone)
}

// GetBoardLaneDynamics is the single-shot aggregate behind
// GET /semantic-boards/:id/lane-dynamics?days=14 (design D4):
//   - window anchored at the board's MAX(period_date) going back `days` days
//     (inclusive both ends — same convention as the landscape date axis), NOT
//     now(), so 态势句素材与时间线同窗 (spec);
//   - lanes = active ∪ watch-linked (non-archived), ranked by lane_heat DESC
//     (tie: story_count_14d DESC, then topic id ASC) — the distinct-day /
//     section-count orderings are retired: bucketing geometry must never
//     decide rank (add-event-story-heat spec 行序); zero-day (沉寂) lanes are
//     not returned;
//   - candidates = visible-gate candidates (hit_count >= upgrade_threshold,
//     FilterVisibleTopics 口径) with their latest section title as hint.
func (r *TopicGraphRepository) GetBoardLaneDynamics(boardID uint, days int) (*LaneDynamicsResponse, error) {
	if days <= 0 {
		days = LaneDynamicsDefaultDays
	}
	resp := &LaneDynamicsResponse{
		WindowDays:       days,
		HeatRuleVersion:  HeatRuleVersion,
		HeatStaleFeedIDs: []uint{},
		Lanes:            []LaneDynamicsLane{},
		Candidates:       []LaneDynamicsCandidate{},
	}

	anchor, ok, err := r.GetBoardReportAnchor(boardID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// 无日报：空数组非 null（tasks 2.2）。热度字段保持默认（未回填/完
		// 整/空 stale 列表），不额外探库。
		return resp, nil
	}
	resp.HasReports = true
	from := anchor.AddDate(0, 0, -(days - 1))

	// Heat coverage：落后源标记一次取全板，失败降级不阻断（task 5.1）。
	if coverage, err := r.BoardHeatCoverage(boardID, time.Now()); err != nil {
		logging.Warnf("lane dynamics: board %d heat coverage degraded: %v", boardID, err)
	} else {
		resp.HeatIncomplete = coverage.Incomplete
		if coverage.StaleFeedIDs != nil {
			resp.HeatStaleFeedIDs = coverage.StaleFeedIDs
		}
	}

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
	statsBySection, err := r.listLaneSectionThreadStats(sectionIDs)
	if err != nil {
		return nil, err
	}
	storyAgg, err := r.loadLaneStoryAggregates(boardID, statsBySection)
	if err != nil {
		return nil, err
	}
	resp.StoriesBackfilled = storyAgg.backfilled

	// lane_heat：全板一次取（task 5.1 实现要点 4——不逐 lane 调），锚定窗口
	// 最新日报日的 CST 23:59:59；时间线为空（无 lane）时跳过探库，热度默认。
	laneHeat := make(map[uint]float64, len(laneIDs))
	if len(laneIDs) > 0 {
		if _, heat, err := r.ComputeBoardHeat(boardID, laneHeatAnchorEnd(anchor)); err != nil {
			logging.Warnf("lane dynamics: board %d heat compute degraded: %v", boardID, err)
		} else {
			laneHeat = heat
		}
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
			LaneHeat:        laneHeat[id],
			Timeline:        make([]LaneDynamicsTimelineDay, 0, len(a.dates)),
		}
		for _, dateSections := range a.byDate {
			for _, section := range dateSections {
				lane.ThreadCount14d += len(threadsBySection[section.ID])
			}
		}
		if snap, ok := snaps[id]; ok {
			lane.Snapshot = &LaneDynamicsSnapshot{
				Summary: snap.RollingSummary,
				AsOf:    NormalizeReportDate(snap.AsOfDate).Format("2006-01-02"),
			}
			// 非空串才携带长版（空串归一 nil = 长版缺失，design D3）。拷贝到局部
			// 变量再取址，避免 range/map 变量逃逸陷阱。
			if snap.RollingDetail != "" {
				detail := snap.RollingDetail
				lane.Snapshot.Detail = &detail
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
					ThreadCount: len(titles),
					Events:      events,
					FoldedCount: folded,
				})
			}
			day.StoryCount, day.MaxParticipantCount, day.UnassignedEventCount =
				storyAgg.foldDay(a.byDate[dateKey], statsBySection)
			lane.StoryCount14d += day.StoryCount
			day.HottestStoryID, day.HottestStoryTitle = storyAgg.hottestStory(a.byDate[dateKey], statsBySection)
			lane.Timeline = append(lane.Timeline, day)
		}
		resp.Lanes = append(resp.Lanes, lane)
	}

	// Rank by lane_heat DESC (72h/14d story-signal decay — the only heat
	// ordering), then story_count_14d DESC, then topic id ASC for
	// deterministic output (add-event-story-heat spec 行序). The former
	// distinct-anchor-day / last-hit-date ordering is retired: intra-lane
	// theme bucketing splits one day into several sections, so any day/section
	// geometry sort would reward "content that happened to be split more".
	sort.SliceStable(resp.Lanes, func(i, j int) bool {
		if resp.Lanes[i].LaneHeat != resp.Lanes[j].LaneHeat {
			return resp.Lanes[i].LaneHeat > resp.Lanes[j].LaneHeat
		}
		if resp.Lanes[i].StoryCount14d != resp.Lanes[j].StoryCount14d {
			return resp.Lanes[i].StoryCount14d > resp.Lanes[j].StoryCount14d
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
