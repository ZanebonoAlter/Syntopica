package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── 质量分级对泳道证据放行（lane-first-recall D3 / tasks 4.1~4.3）──────────────
//
// filterTagsByQuality 的 `kept >= 10` 是一个关于「能否证明自己属于这个版块」的
// 二元开关；泳道证据回答的是另一个问题（离某条已成立泳道多近），所以它既不参与
// 该阈值计数（不把开关顶过阈值而连带丢弃别的 weighted），也不被它丢弃。

func qualityTag(id uint, reason string) repository.TagInput {
	return repository.TagInput{ID: id, Label: fmt.Sprintf("q-%d", id), MatchReason: reason, Score: 1}
}

func weightedTag(id uint) repository.TagInput {
	return repository.TagInput{ID: id, Label: fmt.Sprintf("w-%d", id), MatchReason: "weighted", Score: 0.7}
}

func keptIDs(tags []repository.TagInput) []uint {
	ids := make([]uint, 0, len(tags))
	for _, t := range tags {
		ids = append(ids, t.ID)
	}
	return ids
}

// TestLaneRecallQualityGateAdmitsLaneTags covers Scenario「高质量标签充足时新事件
// 仍能入场」: with >= 10 quality tags the switch drops plain `weighted` tags, but
// lane-recalled ones survive.
func TestLaneRecallQualityGateAdmitsLaneTags(t *testing.T) {
	tags := make([]repository.TagInput, 0, 16)
	for i := uint(1); i <= 12; i++ {
		tags = append(tags, qualityTag(i, "max_sim"))
	}
	tags = append(tags,
		weightedTag(101), // lane-recalled (see map below)
		weightedTag(102), // lane-recalled
		weightedTag(103), // plain weighted → dropped by the switch
	)
	laneRecalled := map[uint]bool{101: true, 102: true}

	kept := filterTagsByQuality(tags, laneRecalled)
	ids := keptIDs(kept)
	assert.Len(t, kept, 14, "12 quality + the 2 lane-recalled new events")
	assert.Contains(t, ids, uint(101))
	assert.Contains(t, ids, uint(102))
	assert.NotContains(t, ids, uint(103), "未经泳道召回的其他 weighted 仍按既有规则丢弃")
}

// TestLaneRecallQualityGateThresholdNotInflated is the zero-regression guard: lane
// witnesses must NOT push `kept` over the >= 10 threshold, otherwise the legacy
// 保底回捞 would stop firing for the remaining weighted tags.
func TestLaneRecallQualityGateThresholdNotInflated(t *testing.T) {
	tags := make([]repository.TagInput, 0, 16)
	for i := uint(1); i <= 8; i++ { // kept = 8 < 10 → legacy 保底 fires
		tags = append(tags, qualityTag(i, "hit_rate"))
	}
	for i := uint(201); i <= 205; i++ {
		tags = append(tags, weightedTag(i))
	}
	laneRecalled := map[uint]bool{201: true, 202: true, 203: true, 204: true, 205: true}

	kept := filterTagsByQuality(tags, laneRecalled)
	assert.Len(t, kept, 13, "kept(8) < 10 still pulls every weighted back, lane or not")
}

// TestLaneRecallQualityGateBypassesCap keeps the 30-tag board-evidence cap from
// silently cutting the lane channel: lane witnesses are bounded by top_k already.
func TestLaneRecallQualityGateBypassesCap(t *testing.T) {
	tags := make([]repository.TagInput, 0, 40)
	for i := uint(1); i <= 35; i++ {
		tags = append(tags, qualityTag(i, "direct_hit"))
	}
	tags = append(tags, weightedTag(301), weightedTag(302))
	laneRecalled := map[uint]bool{301: true, 302: true}

	kept := filterTagsByQuality(tags, laneRecalled)
	require.Len(t, kept, 32, "30 quality after the cap + 2 lane witnesses")
	ids := keptIDs(kept)
	assert.Contains(t, ids, uint(301))
	assert.Contains(t, ids, uint(302))
}

// TestLaneRecallQualityGateNilMarkerIsLegacy pins that every pre-existing caller
// (nil marker, e.g. rebucket / older tests) keeps the exact legacy behaviour.
func TestLaneRecallQualityGateNilMarkerIsLegacy(t *testing.T) {
	tags := make([]repository.TagInput, 0, 14)
	for i := uint(1); i <= 12; i++ {
		tags = append(tags, qualityTag(i, "max_sim"))
	}
	tags = append(tags, weightedTag(401), weightedTag(402))

	assert.Len(t, filterTagsByQuality(tags, nil), 12, ">= 10 quality → weighted dropped, as before")

	small := []repository.TagInput{qualityTag(1, "max_sim"), weightedTag(402)}
	assert.Len(t, filterTagsByQuality(small, nil), 2, "< 10 quality → weighted pulled back, as before")
}

// TestLaneRecallTagReasonIsNotQuality documents the honest tier of a lane witness:
// it is not board statistical evidence, so on its own it counts as `weighted`.
func TestLaneRecallTagReasonIsNotQuality(t *testing.T) {
	assert.False(t, boardQualityMatchReason(laneRecallTagReason))

	tags := make([]repository.TagInput, 0, 13)
	for i := uint(1); i <= 12; i++ {
		tags = append(tags, qualityTag(i, "max_sim"))
	}
	tags = append(tags, repository.TagInput{ID: 501, Label: "laned", MatchReason: laneRecallTagReason, Score: 0.88})

	// Without the marker the row is just another weighted guess and the switch
	// drops it — which is exactly why the marker exists.
	assert.Len(t, filterTagsByQuality(tags, nil), 12)
	// With the marker it is admitted.
	kept := filterTagsByQuality(tags, map[uint]bool{501: true})
	require.Len(t, kept, 13)
	assert.Contains(t, keptIDs(kept), uint(501))
}
