package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 泳道优先召回：候选集合并（lane-first-recall tasks 3.4 / 4.x）───────────────
//
// laneRecallAdmission 是 collectBoardTags 里「两条通道并集」的判定核心：谁是新进来的
// （newIDs）、谁是版块通道已有但靠泳道证据撑住的（rescued）。SQL 侧的召回本身由
// review.md 记录的真库离线核对覆盖；这些用例锁纯逻辑，不碰数据库。

// TestLaneRecallAdmissionNewEventTag covers Scenario「新事件标签经泳道通道进入候选集」:
// a single-article event tag the board channel never had (unmounted or over the
// fallback's random 50) enters the candidate set with lane evidence alone.
func TestLaneRecallAdmissionNewEventTag(t *testing.T) {
	hits := []repository.LaneRecalledTag{
		{TagID: 303909, LaneID: 2688, Distance: 0.1181},
		{TagID: 286352, LaneID: 2688, Distance: 0.1450},
	}
	// Neither tag carries a board row for this board.
	existing := map[uint]string{}
	allowed := map[uint]bool{303909: true, 286352: true}

	newIDs, rescued := laneRecallAdmission(hits, existing, allowed)
	assert.Equal(t, []uint{303909, 286352}, newIDs, "lane radius is enough to admit a new event tag")
	assert.Empty(t, rescued, "nothing to rescue when the board channel had nothing")

	// The stamp the orchestrator puts on such a tag: lane_recall, with the lane
	// similarity as the score — it must never masquerade as a board match.
	assert.Equal(t, "lane_recall", laneRecallTagReason)
	assert.False(t, boardQualityMatchReason(laneRecallTagReason),
		"a lane witness is not board statistical evidence")
}

// TestLaneRecallAdmissionDualChannel covers task 3.4: a tag recalled by both
// channels keeps ONE row, and only the weakest board tiers owe their seat to the
// lane channel.
func TestLaneRecallAdmissionDualChannel(t *testing.T) {
	hits := []repository.LaneRecalledTag{
		{TagID: 1, LaneID: 2687, Distance: 0.08},
		{TagID: 2, LaneID: 2687, Distance: 0.10},
		{TagID: 3, LaneID: 2687, Distance: 0.12},
		{TagID: 4, LaneID: 2687, Distance: 0.14},
	}
	existing := map[uint]string{
		1: "weighted",
		2: "max_sim",
		3: "composite_hit",
		4: "direct_hit",
	}
	allowed := map[uint]bool{1: true, 2: true, 3: true, 4: true}

	newIDs, rescued := laneRecallAdmission(hits, existing, allowed)
	assert.Empty(t, newIDs, "a dual-channel hit must not be duplicated in the candidate set")
	assert.True(t, rescued[1], "weighted + lane witness → the lane channel is what keeps it")
	assert.False(t, rescued[2], "max_sim already proves board membership")
	assert.True(t, rescued[3], "composite_hit sits outside the quality filter's kept set")
	assert.False(t, rescued[4])
}

// TestLaneRecallAdmissionExclusivity keeps the cross-board exclusivity verdict
// authoritative for lane-recalled tags too (design D6: 同一 tag 在别板明显更强时不收).
func TestLaneRecallAdmissionExclusivity(t *testing.T) {
	hits := []repository.LaneRecalledTag{
		{TagID: 10, LaneID: 2687, Distance: 0.09},
		{TagID: 11, LaneID: 2687, Distance: 0.11},
	}
	allowed := map[uint]bool{10: true, 11: false}

	newIDs, rescued := laneRecallAdmission(hits, nil, allowed)
	assert.Equal(t, []uint{10}, newIDs)
	assert.Empty(t, rescued)

	// Missing verdicts are closed-by-default: an unknown tag is not admitted.
	newIDs, _ = laneRecallAdmission(hits, nil, map[uint]bool{10: true})
	assert.Equal(t, []uint{10}, newIDs)
}

// TestLaneRecallAdmissionDedupesHits guards against counting the same witness
// twice when a lane list repeats a tag id.
func TestLaneRecallAdmissionDedupesHits(t *testing.T) {
	hits := []repository.LaneRecalledTag{
		{TagID: 7, LaneID: 1, Distance: 0.1},
		{TagID: 7, LaneID: 2, Distance: 0.12},
	}
	newIDs, _ := laneRecallAdmission(hits, nil, map[uint]bool{7: true})
	assert.Equal(t, []uint{7}, newIDs)
}

// TestLaneRecallEmptyLaneContributesNothing covers Scenario「当天没有专属新闻的泳道
// 保持空白」at the service layer: no witness → no new candidate, no marker, so the
// lane channel cannot inflate the report with neighbour content.
func TestLaneRecallEmptyLaneContributesNothing(t *testing.T) {
	newIDs, rescued := laneRecallAdmission(nil, map[uint]string{1: "weighted"}, nil)
	assert.Empty(t, newIDs)
	assert.Empty(t, rescued, "a hungry lane marks nothing, it does not relax the radius")

	// And the board side of the same rule: an empty witness list is nothing for
	// this board either.
	id, marked := laneRecallAdmission([]repository.LaneRecalledTag{}, nil, nil)
	assert.Empty(t, id)
	assert.Empty(t, marked)
}

// TestLaneRecallExclusivityVerdict pins the read-only exclusivity arithmetic:
// a tag's best mount here vs its best mount elsewhere, with the configured gap.
// It mirrors the main query's NOT EXISTS predicate (same tag in a clearly
// stronger home elsewhere is not collected here).
func TestLaneRecallExclusivityVerdict(t *testing.T) {
	// 2026-11-26 board 15220 shapes: (1) mounted here strongest, (2) mounted here
	// weaker but within the gap, (3) unmounted here, strongly mounted elsewhere,
	// (4) unmounted anywhere (the 86% pool the lane channel exists for).
	rows := []laneMountRow{
		{TopicTagID: 1, SemanticBoardID: 15220, Score: 1.00},
		{TopicTagID: 1, SemanticBoardID: 1980, Score: 0.70},
		{TopicTagID: 2, SemanticBoardID: 15220, Score: 0.62},
		{TopicTagID: 2, SemanticBoardID: 2197, Score: 0.70},
		{TopicTagID: 3, SemanticBoardID: 1980, Score: 0.90},
	}
	verdict := laneRecallExclusivityVerdict([]uint{1, 2, 3, 4}, rows, 15220, 0.3)
	assert.True(t, verdict[1], "1.00 here vs 0.70 elsewhere → kept")
	assert.True(t, verdict[2], "0.62 here vs 0.70 elsewhere is inside the 0.3 gap → kept")
	assert.False(t, verdict[3], "0.90 elsewhere with no mount here → excluded")
	assert.True(t, verdict[4], "unmounted anywhere is not 'stronger elsewhere' → kept")
}

// TestLaneRecallExclusivityAllowedGapDisabled pins the kill switch by executing
// the production function itself (review R-13): gap <= 0 admits every witness and
// returns before the mount lookup. Proven by running with a nil repository — any
// DB access would panic. Note the pure verdict alone cannot express this switch
// (`bestOther-own < 0` is never true at gap 0), so the caller's short path is the
// switch; this test is what keeps it covered.
func TestLaneRecallExclusivityAllowedGapDisabled(t *testing.T) {
	prev := repository.Repo
	repository.Repo = nil
	t.Cleanup(func() { repository.Repo = prev })

	hits := []repository.LaneRecalledTag{
		{TagID: 3, LaneID: 2687, Distance: 0.09},
		{TagID: 4, LaneID: 2687, Distance: 0.12},
	}
	for _, gap := range []float64{0, -0.3} {
		allowed := laneRecallExclusivityAllowed(hits, nil, 15220, gap)
		require.Len(t, allowed, len(hits), "gap=%v must admit every witness", gap)
		for _, hit := range hits {
			assert.True(t, allowed[hit.TagID],
				"gap=%v must not weigh stronger mounts (the lookup is skipped entirely)", gap)
		}
	}

	// Nothing recalled → nothing to judge, still without touching the DB.
	assert.Empty(t, laneRecallExclusivityAllowed(nil, nil, 15220, 0.3))
}

// TestLaneRecallExclusivityAllowedBoundary runs the real gap>0 path against
// Postgres (review R-13): the rule is `bestOther - own < gap`, so a difference
// exactly at the gap is already "clearly stronger elsewhere".
func TestLaneRecallExclusivityAllowedBoundary(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	home := models.SemanticLabel{Label: "excl-home", Slug: "excl-home", LabelType: "board"}
	weak := models.SemanticLabel{Label: "excl-weak", Slug: "excl-weak", LabelType: "board"}
	require.NoError(t, db.Create(&home).Error)
	require.NoError(t, db.Create(&weak).Error)

	// The mounts reference real tags (topic_tag_id carries an FK).
	for _, slug := range []string{"excl-t1", "excl-t2", "excl-t3", "excl-t4"} {
		require.NoError(t, db.Create(&models.TopicTag{
			Slug: slug, Label: slug, Category: models.TagCategoryEvent, Status: "active",
		}).Error)
	}
	mount := func(tagID, boardID uint, score float64) {
		t.Helper()
		require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
			TopicTagID: tagID, SemanticBoardID: boardID, MatchReason: "weighted", Score: score,
		}).Error)
	}
	mount(1, weak.ID, 0.70)
	mount(1, home.ID, 1.00) // 1.00-0.70 == gap → excluded
	mount(2, weak.ID, 0.62)
	mount(2, home.ID, 0.70) // 0.08 < gap → kept
	mount(3, home.ID, 0.90) // unmounted here → excluded
	mount(4, weak.ID, 0.50) // sole mount here → kept

	hits := []repository.LaneRecalledTag{
		{TagID: 1, LaneID: 2687, Distance: 0.10},
		{TagID: 2, LaneID: 2687, Distance: 0.11},
		{TagID: 3, LaneID: 2687, Distance: 0.12},
		{TagID: 4, LaneID: 2687, Distance: 0.13},
	}
	allowed := laneRecallExclusivityAllowed(hits, nil, weak.ID, 0.3)
	require.Len(t, allowed, len(hits))
	assert.False(t, allowed[1], "difference == gap is 'clearly stronger elsewhere' → excluded")
	assert.True(t, allowed[2], "inside the gap → kept")
	assert.False(t, allowed[3], "no mount here while another board holds 0.90 → excluded")
	assert.True(t, allowed[4], "a sole mount here cannot be 'stronger elsewhere' → kept")

	// Board-channel tags were already judged by the main query, so the lane
	// channel does not re-judge them: they never enter the lookup, and the
	// admission step keeps their row whatever the verdict map says.
	allowed = laneRecallExclusivityAllowed(hits, map[uint]string{3: "weighted"}, weak.ID, 0.3)
	assert.NotContains(t, allowed, uint(3), "board-channel tags are exempt from the lane verdict")
	newIDs, rescued := laneRecallAdmission(hits, map[uint]string{3: "weighted"}, allowed)
	assert.NotContains(t, newIDs, uint(3), "a dual-channel hit is not added twice")
	assert.True(t, rescued[3], "a board row the lane merely re-witnesses keeps its seat")
}

// ── 真库守护：泳道通道零写入（tasks 3.3 / 3.5）────────────────────────────────
//
// 这条红线只能用真 pgvector 容器验（含副作用的 SQL 一律不碰真库、集成走
// testcontainers）：泳道召回可以往候选集加标签，但 MUST NOT 往
// topic_tag_board_labels 落挂载行——挂载是永久事实，召回是单日行为。

const laneRecallDim = 2560

// laneRecallUnitFloats builds a 2560-dim one-hot vector, so test rows live in the
// same vector space the production halfvec(2560) index is built on.
func laneRecallUnitFloats(axis int) []float64 {
	v := make([]float64, laneRecallDim)
	v[axis] = 1
	return v
}

// laneRecallUnitVector is the pgvector literal form of laneRecallUnitFloats.
func laneRecallUnitVector(axis int) string {
	return repository.FloatsToPgVector(laneRecallUnitFloats(axis))
}

func countBoardLabels(t *testing.T, db *gorm.DB, tagID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Where("topic_tag_id = ?", tagID).Count(&n).Error)
	return n
}

// TestLaneRecallChannelWritesNoMount is the DB-backed guard for the change's
// hardest boundary: a tag that is mounted nowhere and invisible to the board
// channel (no auxiliary labels → the fallback's JOIN skips it) still enters the
// candidate set through the lane channel, and no mount row appears.
func TestLaneRecallChannelWritesNoMount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	feed := models.Feed{Title: "lane-recall-feed", URL: "https://example.com/feed-lane-recall"}
	require.NoError(t, db.Create(&feed).Error)

	board := models.SemanticLabel{Label: "lane-recall-board", Slug: "lane-recall-board", LabelType: "board"}
	require.NoError(t, db.Create(&board).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	article := models.Article{FeedID: feed.ID, Title: "英伟达暂停向东南亚芯片交付", PubDate: &pubDate}
	require.NoError(t, db.Create(&article).Error)

	tag := models.TopicTag{
		Slug: "lane-recall-nvidia-sea", Label: "英伟达暂停向东南亚芯片交付",
		Category: models.TagCategoryEvent, Status: "active",
	}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tag.ID, EmbeddingType: "semantic", EmbeddingVec: laneRecallUnitVector(0),
		Dimension: laneRecallDim, Model: "test", TextHash: "lane-recall-tag",
	}).Error)

	// An active lane whose anchor equals the tag vector → distance ~0.
	lane := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID, Label: "芯片与算力管制", Status: repository.TopicStatusActive,
		Source: "manual", FirstSeenDate: now, LastSeenDate: now, HitCount: 8,
		Centroid:  laneRecallUnitVector(0),
		Embedding: laneRecallUnitVector(0),
	}
	require.NoError(t, db.Create(&lane).Error)

	require.Equal(t, int64(0), countBoardLabels(t, db, tag.ID), "seed precondition: unmounted")

	tags, _, laneRecalled, err := collectBoardTags(board.ID, now)
	require.NoError(t, err)

	var picked *repository.TagInput
	for i := range tags {
		if tags[i].ID == tag.ID {
			picked = &tags[i]
			break
		}
	}
	require.NotNil(t, picked, "the lane channel must collect a tag the board channel cannot see")
	assert.Equal(t, laneRecallTagReason, picked.MatchReason)
	assert.True(t, laneRecalled[tag.ID], "the tag must carry the lane-recall marker")
	assert.InDelta(t, 1.0, picked.Score, 1e-4, "score is the lane similarity (distance ~0)")
	assert.Equal(t, 1, picked.ArticleCount)

	// Red line: the recall writes no mount row.
	assert.Equal(t, int64(0), countBoardLabels(t, db, tag.ID),
		"lane recall MUST NOT write topic_tag_board_labels")
}

// TestLaneRecallSkipsUnusableLanesKeepsOthers is the behaviour-level counterpart
// of the pure anchor tests (review R-12): RecallTagsForBoard skips a lane whose
// vector is NULL and a lane whose dimension disagrees with the tag space
// (Postgres rejects the halfvec cast), keeps recalling through the healthy lane,
// and still returns no error.
func TestLaneRecallSkipsUnusableLanesKeepsOthers(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	feed := models.Feed{Title: "lane-recall-skip-feed", URL: "https://example.com/feed-lane-recall-skip"}
	require.NoError(t, db.Create(&feed).Error)
	board := models.SemanticLabel{Label: "lane-recall-skip-board", Slug: "lane-recall-skip-board", LabelType: "board"}
	require.NoError(t, db.Create(&board).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	article := models.Article{FeedID: feed.ID, Title: "可跳过泳道不影响健康泳道召回", PubDate: &pubDate}
	require.NoError(t, db.Create(&article).Error)

	tag := models.TopicTag{
		Slug: "lane-recall-skip-tag", Label: "可跳过泳道不影响健康泳道召回",
		Category: models.TagCategoryEvent, Status: "active",
	}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tag.ID, EmbeddingType: "semantic", EmbeddingVec: laneRecallUnitVector(0),
		Dimension: laneRecallDim, Model: "test", TextHash: "lane-recall-skip-tag",
	}).Error)

	newLane := func(label string, centroid, embedding string) repository.BoardPersistentTopic {
		t.Helper()
		lane := repository.BoardPersistentTopic{
			SemanticBoardID: board.ID, Label: label, Status: repository.TopicStatusActive,
			Source: "manual", FirstSeenDate: now, LastSeenDate: now, HitCount: 8,
			Centroid: centroid, Embedding: embedding,
		}
		require.NoError(t, db.Create(&lane).Error)
		return lane
	}

	// Lane 1: `embedding IS NULL` (and no centroid) — the spec's first GIVEN.
	nullLane := newLane("空向量泳道", laneRecallUnitVector(0), laneRecallUnitVector(0))
	require.NoError(t, db.Exec(
		"UPDATE board_persistent_topics SET centroid = NULL, embedding = NULL WHERE id = ?", nullLane.ID).Error)

	// Lane 2: a 2-dim anchor against a 2560-dim tag space. The dim pre-check does
	// not catch it, so Postgres rejects `halfvec(2)` and the per-lane error branch
	// is what saves the board's report.
	wrongDim := repository.FloatsToPgVector([]float64{1, 0})
	dimLane := newLane("维度不符泳道", wrongDim, wrongDim)

	// Lane 3: healthy anchor equal to the tag vector → distance ~0.
	goodLane := newLane("芯片与算力管制", laneRecallUnitVector(0), laneRecallUnitVector(0))

	recalled, err := repository.Repo.RecallTagsForBoard(board.ID, now, repository.DefaultLaneRecallConfig())
	require.NoError(t, err, "an unusable lane must never fail the board's recall")
	require.Len(t, recalled, 1, "only the healthy lane contributes a witness")
	assert.Contains(t, recalled, tag.ID)
	assert.Equal(t, goodLane.ID, recalled[tag.ID].LaneID)
	assert.NotEqual(t, nullLane.ID, recalled[tag.ID].LaneID)
	assert.NotEqual(t, dimLane.ID, recalled[tag.ID].LaneID)
	assert.InDelta(t, 0.0, recalled[tag.ID].Distance, 1e-4)

	// Pin WHICH branch covers each lane, so a future change cannot silently move
	// them: the NULL vector is caught by the anchor pre-check (no query, no
	// error), while the dimension mismatch is not — it fails inside the query and
	// is swallowed by the per-lane `failed++` branch.
	_, err = repository.Repo.RecallTagsByLane(nullLane.ID, now, 8, 0.16)
	require.NoError(t, err, "a NULL-vector lane is skipped before the query")
	_, err = repository.Repo.RecallTagsByLane(dimLane.ID, now, 8, 0.16)
	require.Error(t, err, "a dimension mismatch surfaces as a query error, not a pre-check skip")
}

// TestLaneRecallEmptyLaneKeepsItsDayBlank covers the second half of Scenario
// 「当天没有专属新闻的泳道保持空白」 (review R-14): a lane whose nearest neighbour
// sits beyond the radius contributes no candidate AND produces no cluster group —
// i.e. no section of its own — while the lane that does own the day's news keeps
// its group.
func TestLaneRecallEmptyLaneKeepsItsDayBlank(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	feed := models.Feed{Title: "lane-recall-blank-feed", URL: "https://example.com/feed-lane-recall-blank"}
	require.NoError(t, db.Create(&feed).Error)
	board := models.SemanticLabel{Label: "lane-recall-blank-board", Slug: "lane-recall-blank-board", LabelType: "board"}
	require.NoError(t, db.Create(&board).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	article := models.Article{FeedID: feed.ID, Title: "英伟达暂停向东南亚芯片交付", PubDate: &pubDate}
	require.NoError(t, db.Create(&article).Error)

	tag := models.TopicTag{
		Slug: "lane-recall-blank-tag", Label: "英伟达暂停向东南亚芯片交付",
		Category: models.TagCategoryEvent, Status: "active",
	}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tag.ID, EmbeddingType: "semantic", EmbeddingVec: laneRecallUnitVector(0),
		Dimension: laneRecallDim, Model: "test", TextHash: "lane-recall-blank-tag",
	}).Error)

	newLane := func(label string, axis int) repository.BoardPersistentTopic {
		t.Helper()
		lane := repository.BoardPersistentTopic{
			SemanticBoardID: board.ID, Label: label, Status: repository.TopicStatusActive,
			Source: "manual", FirstSeenDate: now, LastSeenDate: now, HitCount: 8,
			Centroid: laneRecallUnitVector(axis), Embedding: laneRecallUnitVector(axis),
		}
		require.NoError(t, db.Create(&lane).Error)
		return lane
	}
	// Orthogonal anchor: distance 1.0 >> max_distance 0.16, so the far lane finds
	// no neighbour within its radius — the measured "hungry lane" shape.
	nearLane := newLane("芯片与算力管制", 0)
	farLane := newLane("中东冲突波及旅游业", 1)

	recalled, err := repository.Repo.RecallTagsForBoard(board.ID, now, repository.DefaultLaneRecallConfig())
	require.NoError(t, err)
	require.Len(t, recalled, 1, "the orthogonal lane must recall nothing instead of pulling neighbours in")
	assert.Equal(t, nearLane.ID, recalled[tag.ID].LaneID)
	assert.NotEqual(t, farLane.ID, recalled[tag.ID].LaneID)

	tags, _, laneRecalled, err := collectBoardTags(board.ID, now)
	require.NoError(t, err)
	require.True(t, laneRecalled[tag.ID], "the near lane's witness is what puts the tag in the candidate set")

	calls := stubLaneChat(t, `{"decisions":[]}`)
	groups, err := ClusterTagsLane(context.Background(), tags,
		[]repository.BoardPersistentTopic{nearLane, farLane},
		map[uint][]float64{tag.ID: laneRecallUnitFloats(0)}, nil, laneCfg(),
		WithLaneRecalledTags(laneRecalled))
	require.NoError(t, err)
	require.Len(t, groups, 1, "only the lane that owns the day's news gets a group")
	require.NotNil(t, groups[0].MatchedTopicID)
	assert.Equal(t, nearLane.ID, *groups[0].MatchedTopicID)
	assert.Equal(t, []uint{tag.ID}, groups[0].TagIDs)
	for _, g := range groups {
		if g.MatchedTopicID == nil {
			continue
		}
		assert.NotEqual(t, farLane.ID, *g.MatchedTopicID,
			"an empty lane MUST NOT produce a group, hence no section of its own that day")
	}
	assert.Empty(t, *calls, "one L1 tag: the no-LLM fallback path keeps zero calls")
}

// TestLaneRecallExclusivityVerdictGapDisabledIsSafe pins review R-13's footgun
// at the pure function itself: with gap <= 0 the formula degenerates to
// `0 - 0 < 0` = false for a tag mounted nowhere, i.e. "everything rejected"
// instead of the intended "rule disabled". The guard belongs in the function so
// a future caller cannot reintroduce the inversion by skipping the caller's
// short circuit.
func TestLaneRecallExclusivityVerdictGapDisabledIsSafe(t *testing.T) {
	pending := []uint{1, 2}
	// Row set that would make the raw formula reject tag 1 and accept tag 2.
	rows := []laneMountRow{
		{TopicTagID: 1, SemanticBoardID: 999, Score: 1.0},
		{TopicTagID: 2, SemanticBoardID: 999, Score: 1.0},
	}
	for _, gap := range []float64{0, -0.3} {
		verdict := laneRecallExclusivityVerdict(pending, rows, 100, gap)
		assert.Equal(t, map[uint]bool{1: true, 2: true}, verdict,
			"gap=%v must disable the rule inside the pure function, not only in the caller", gap)
	}
	// Sanity: with a positive gap the arithmetic is unchanged.
	verdict := laneRecallExclusivityVerdict(pending, rows, 100, 0.3)
	assert.Equal(t, map[uint]bool{1: false, 2: false}, verdict)
}
