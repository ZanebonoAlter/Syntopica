package repository

import (
	"fmt"
	"math"
	"sort"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

const (
	HeatRuleVersion        = "heat-v1-48h-hl24h"
	StoryHeatHalfLifeHours = 24
	StoryHeatWindowHours   = 48
	LaneHeatHalfLifeHours  = 72
	LaneHeatWindowDays     = 14
)

var storyHeatZone = time.FixedZone("CST", 8*3600)

type storyHeatIdentity struct {
	ID         uint
	MergedInto *uint
	LaneID     *uint
	Status     string
	// Title is the event line's display title — an附加字段 for the radar
	// story feed (it never takes part in the heat computation).
	Title string
}

type storyHeatSignal struct {
	StoryID uint
	// ContentKey 内容身份键（规范化链接）：同一篇内容的多个 feed 副本相同；
	// 空串 = 内容身份不可判定（link 为空 / 存量行未回填）→ 退化为
	// ParticipantKey。
	ContentKey     string
	ParticipantKey string
	ObservedAt     time.Time
}

// heatParticipantKey 是参与者去重键的**唯一**实现（fix-duplicate-article-heat
// design D3）：内容身份优先、feed 兜底。
//
// SQL 侧的同一口径是 COALESCE(NULLIF(s.content_key,”), s.participant_key)
// （见 recomputeStoryParticipantsWith）——两处必须逐字等价，否则会出现
// 「热度按内容计、来源数按 feed 计」的混用（红线：消费面同源）。
//
// contentKey 非空（内容身份可判定）→ 同一篇内容被多个 feed 抓取时只算 1 个
// 参与者；contentKey 为空 → 退化为 participantKey（feed 粒度），不同 feed
// 不合并。
func heatParticipantKey(contentKey, participantKey string) string {
	if contentKey != "" {
		return contentKey
	}
	return participantKey
}

// storyIdentityEdge is the minimal identity projection needed to resolve a
// story's one-hop survivor: id + merged_into + status.
type storyIdentityEdge struct {
	ID         uint
	MergedInto *uint
	Status     string
}

// strictStorySurvivors maps every story id to its one-hop survivor. The merge
// invariant is one hop, so broken links, self references, cycles and
// multi-hop chains are dropped conservatively (same discipline as
// highlightStorySurvivors, which is built on this helper).
func strictStorySurvivors(edges []storyIdentityEdge) map[uint]uint {
	byID := make(map[uint]storyIdentityEdge, len(edges))
	for _, edge := range edges {
		byID[edge.ID] = edge
	}
	out := make(map[uint]uint, len(edges))
	for _, edge := range edges {
		target := edge
		if edge.MergedInto != nil {
			next, ok := byID[*edge.MergedInto]
			if !ok || next.ID == edge.ID || next.MergedInto != nil {
				continue
			}
			target = next
		}
		if target.Status == StoryStatusMerged {
			continue
		}
		out[edge.ID] = target.ID
	}
	return out
}

type boardHeatEvidence struct {
	survivors map[uint]uint
	// strict is the one-hop survivor map (broken links / cycles / multi-hop
	// chains dropped). survivors keeps the heat口径's own resolution (a
	// multi-hop chain still resolves to its final survivor), strict marks the
	// identities that are ambiguous for presentation purposes.
	strict map[uint]uint
	// titles carries the survivor's own title (survivor id → title).
	titles  map[uint]string
	lanes   map[uint]uint
	signals []storyHeatSignal
}

// loadBoardHeatEvidence uses the indexed signal time/story columns; only
// evidence, identity and article existence participate, never report geometry.
func (r *TopicGraphRepository) loadBoardHeatEvidence(boardID uint, since, until time.Time) (boardHeatEvidence, error) {
	evidence := boardHeatEvidence{
		survivors: make(map[uint]uint),
		strict:    make(map[uint]uint),
		titles:    make(map[uint]string),
		lanes:     make(map[uint]uint),
	}
	var stories []storyHeatIdentity
	if err := r.db.Model(&BoardStory{}).Select("id", "merged_into", "lane_id", "status", "title").
		Where("board_id = ?", boardID).Find(&stories).Error; err != nil {
		return evidence, fmt.Errorf("load board %d heat stories: %w", boardID, err)
	}
	byID := make(map[uint]storyHeatIdentity, len(stories))
	edges := make([]storyIdentityEdge, 0, len(stories))
	for _, story := range stories {
		byID[story.ID] = story
		edges = append(edges, storyIdentityEdge{ID: story.ID, MergedInto: story.MergedInto, Status: story.Status})
	}
	evidence.strict = strictStorySurvivors(edges)
	for _, story := range stories {
		survivor := story
		// Inherited lane: first non-nil LaneID on the merge chain (from-side
		// first). Used only when the survivor itself has no lane, so a laned
		// story merged into a laneless one (reverse-direction merge) keeps its
		// lane_heat instead of dropping it.
		inheritedLane, hasInheritedLane := uint(0), false
		seen := make(map[uint]bool)
		valid := true
		for survivor.MergedInto != nil {
			if !hasInheritedLane && survivor.LaneID != nil {
				inheritedLane, hasInheritedLane = *survivor.LaneID, true
			}
			if seen[survivor.ID] {
				valid = false
				break
			}
			seen[survivor.ID] = true
			next, ok := byID[*survivor.MergedInto]
			if !ok {
				valid = false
				break
			}
			if next.MergedInto != nil {
				logging.Warnf("story heat: resolve unexpected merged chain at story %d", next.ID)
			}
			survivor = next
		}
		if !valid || survivor.Status == StoryStatusMerged {
			logging.Warnf("story heat: skip invalid survivor for story %d", story.ID)
			continue
		}
		evidence.survivors[story.ID] = survivor.ID
		evidence.titles[survivor.ID] = survivor.Title
		// Survivor's own lane wins; otherwise inherit the chain's lane.
		switch {
		case survivor.LaneID != nil:
			evidence.lanes[survivor.ID] = *survivor.LaneID
		case hasInheritedLane:
			evidence.lanes[survivor.ID] = inheritedLane
		}
	}
	// Follow sourcestats' window-query rule: never filter on archived.
	// Archived means rotated out of the feed's active window; the reporting
	// fact still contributes to heat.
	if err := r.db.Table("story_signals AS s").
		Select("s.story_id, s.participant_key, s.content_key, s.observed_at").
		Joins("JOIN board_stories AS b ON b.id = s.story_id").
		Joins("JOIN articles AS a ON a.id = s.article_id").
		Where("b.board_id = ? AND s.observed_at >= ? AND s.observed_at <= ?", boardID, since.In(storyHeatZone), until.In(storyHeatZone)).
		Scan(&evidence.signals).Error; err != nil {
		return evidence, fmt.Errorf("load board %d heat signals: %w", boardID, err)
	}
	return evidence, nil
}

// ComputeBoardHeat returns independent story (24h/48h) and lane (72h/14d)
// decays. The latter is NOT a sum of the short-window story heat.
func (r *TopicGraphRepository) ComputeBoardHeat(boardID uint, at time.Time) (storyHeat map[uint]float64, laneHeat map[uint]float64, err error) {
	evidence, err := r.loadBoardHeatEvidence(boardID, at.Add(-LaneHeatWindowDays*24*time.Hour), at)
	if err != nil {
		return nil, nil, err
	}
	storyHeat, laneHeat = evidence.compute(at)
	return storyHeat, laneHeat, nil
}

// HeatCoverage — task 4.4 落后源标记：对该版块热度有贡献的 feed 里，哪些
// 采集已落后（证据可能缺最新报道），调用方据此标 incomplete 并在恢复后重算。
type HeatCoverage struct {
	Incomplete   bool
	StaleFeedIDs []uint
	CheckedAt    time.Time
}

const (
	// heatStaleFloor：落后判定的最短窗口（分钟）。低于它的 refresh_interval
	// 也按 90 分钟算，避免高频源被采集抖动误标。
	heatStaleFloor = 90 * time.Minute
	// heatStaleIntervalFactor：落后窗口 = refresh_interval 的倍数。
	heatStaleIntervalFactor = 3
)

// BoardHeatCoverage checks collection staleness of every feed that
// contributes heat evidence to the board (any signal on a board story;
// merged-from stories keep their signals, and merges stay in-board).
//
// The feeds table only carries LastRefreshAt/RefreshStatus — there is no
// last_success column — so staleness is a proxy: never refreshed, beyond
// max(3×refresh_interval, 90min) since the last refresh, or stuck
// "refreshing"/failed "error" beyond the 90min floor (recent failures stay
// unmarked to avoid noise). Tighten this once a last_success field exists.
// Staleness compares against now, never against the heat date.
func (r *TopicGraphRepository) BoardHeatCoverage(boardID uint, now time.Time) (HeatCoverage, error) {
	coverage := HeatCoverage{CheckedAt: now}
	var feedIDs []uint
	if err := r.db.Table("story_signals AS ss").
		Joins("JOIN board_stories AS b ON b.id = ss.story_id").
		Distinct("ss.feed_id").
		Where("b.board_id = ?", boardID).
		Scan(&feedIDs).Error; err != nil {
		return coverage, fmt.Errorf("load board %d coverage feeds: %w", boardID, err)
	}
	if len(feedIDs) == 0 {
		return coverage, nil
	}
	var feeds []models.Feed
	if err := r.db.Select("id", "refresh_interval", "refresh_status", "last_refresh_at").
		Where("id IN ?", feedIDs).Find(&feeds).Error; err != nil {
		return coverage, fmt.Errorf("load board %d coverage feed rows: %w", boardID, err)
	}
	sort.Slice(feeds, func(i, j int) bool { return feeds[i].ID < feeds[j].ID })
	for _, feed := range feeds {
		if heatFeedStale(feed, now) {
			coverage.StaleFeedIDs = append(coverage.StaleFeedIDs, feed.ID)
		}
	}
	coverage.Incomplete = len(coverage.StaleFeedIDs) > 0
	return coverage, nil
}

func heatFeedStale(feed models.Feed, now time.Time) bool {
	if feed.LastRefreshAt == nil {
		return true // never collected successfully
	}
	since := now.Sub(*feed.LastRefreshAt)
	if since > max(heatStaleIntervalFactor*time.Duration(feed.RefreshInterval)*time.Minute, heatStaleFloor) {
		return true
	}
	return (feed.RefreshStatus == "refreshing" || feed.RefreshStatus == "error") && since > heatStaleFloor
}

// RecomputeHeatForBoard is the recovery entry point (task 4.4): heat is a
// deterministic function of signals, so after a stale feed catches up,
// calling this yields the same values as a fresh compute at the same
// instant — no cache involved, nothing written back. `at` is the heat
// instant (may be a historical day end), `now` drives staleness only.
func (r *TopicGraphRepository) RecomputeHeatForBoard(boardID uint, at, now time.Time) (map[uint]float64, map[uint]float64, HeatCoverage, error) {
	storyHeat, laneHeat, err := r.ComputeBoardHeat(boardID, at)
	if err != nil {
		return nil, nil, HeatCoverage{CheckedAt: now}, err
	}
	coverage, err := r.BoardHeatCoverage(boardID, now)
	if err != nil {
		return nil, nil, coverage, err
	}
	return storyHeat, laneHeat, coverage, nil
}

type storyParticipant struct {
	story uint
	// participant 是参与者去重键（heatParticipantKey 的产物：内容身份优先、
	// feed 兜底）——同一篇内容被多个 feed 抓到时只占一个键。
	participant string
}

func (e boardHeatEvidence) compute(at time.Time) (map[uint]float64, map[uint]float64) {
	storyHeat, laneHeat := make(map[uint]float64), make(map[uint]float64)
	latest := make(map[storyParticipant]time.Time)
	for _, signal := range e.signals {
		survivor, ok := e.survivors[signal.StoryID]
		if !ok || signal.ObservedAt.After(at) || signal.ObservedAt.Before(at.Add(-LaneHeatWindowDays*24*time.Hour)) {
			continue
		}
		key := storyParticipant{survivor, heatParticipantKey(signal.ContentKey, signal.ParticipantKey)}
		if previous, exists := latest[key]; !exists || signal.ObservedAt.After(previous) {
			latest[key] = signal.ObservedAt
		}
	}
	// Stable summation order also makes repeated historical computations exact.
	keys := make([]storyParticipant, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].story != keys[j].story {
			return keys[i].story < keys[j].story
		}
		return keys[i].participant < keys[j].participant
	})
	for _, key := range keys {
		hours := at.Sub(latest[key]).Hours()
		if hours <= StoryHeatWindowHours {
			storyHeat[key.story] += math.Pow(0.5, hours/StoryHeatHalfLifeHours)
		}
		if lane, ok := e.lanes[key.story]; ok {
			laneHeat[lane] += math.Pow(0.5, hours/LaneHeatHalfLifeHours)
		}
	}
	return storyHeat, laneHeat
}

// ComputeBoardStoryHeatByDay computes CST day-end story totals from one load.
// Future evidence is excluded separately for every historical day.
func (r *TopicGraphRepository) ComputeBoardStoryHeatByDay(boardID uint, days []time.Time) (map[string]float64, error) {
	out := make(map[string]float64, len(days))
	if len(days) == 0 {
		return out, nil
	}
	ends := make([]time.Time, 0, len(days))
	var first, last time.Time
	for _, day := range days {
		local := day.In(storyHeatZone)
		end := time.Date(local.Year(), local.Month(), local.Day(), 23, 59, 59, 0, storyHeatZone)
		ends = append(ends, end)
		if first.IsZero() || end.Before(first) {
			first = end
		}
		if last.IsZero() || end.After(last) {
			last = end
		}
	}
	since := last.Add(-LaneHeatWindowDays * 24 * time.Hour)
	// The radar asks for today/last3. Wider callers still get a full 48h
	// window for their earliest requested day without additional DB loads.
	if earliest := first.Add(-StoryHeatWindowHours * time.Hour); earliest.Before(since) {
		since = earliest
	}
	evidence, err := r.loadBoardHeatEvidence(boardID, since, last)
	if err != nil {
		return nil, err
	}
	for _, end := range ends {
		heat, _ := evidence.compute(end)
		ids := make([]uint, 0, len(heat))
		for id := range heat {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var total float64
		for _, id := range ids {
			total += heat[id]
		}
		out[end.Format("2006-01-02")] = total
	}
	return out, nil
}
