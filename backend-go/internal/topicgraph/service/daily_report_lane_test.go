package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

// unit returns the unit vector for a Pythagorean (cos, sin) pair so cosine
// distances from [1,0] are exact-ish: cosineDist([1,0], unit(c,s)) = 1-c.
func unit(c, s float64) []float64 { return []float64{c, s} }

func laneCfg() repository.PersistentTopicConfig {
	return repository.PersistentTopicConfig{
		LaneL1Threshold: 0.18,
		LaneL2Threshold: 0.30,
		L2CandidateK:    5,
	}
}

// ---- BucketTagsByCentroid ----

func TestBucketTagsByCentroid_L1L2L3(t *testing.T) {
	// Topic at [1,0]. Tags at clean Pythagorean distances:
	//   [1,0]     → dist 0.0  (< 0.18 → L1)
	//   [0.8,0.6] → dist 0.2  (in [0.18,0.30] → L2)
	//   [0.6,0.8] → dist 0.4  (> 0.30 → L3)
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0))}
	tags := []repository.TagInput{
		{ID: 1, Label: "strong"},
		{ID: 2, Label: "weak"},
		{ID: 3, Label: "far"},
	}
	emb := map[uint][]float64{
		1: unit(1, 0),
		2: unit(0.8, 0.6),
		3: unit(0.6, 0.8),
	}
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, laneCfg())

	require.Len(t, b.L1, 1)
	assert.Equal(t, uint(1), b.L1[0].Tag.ID)
	assert.Equal(t, uint(7), b.L1[0].TopicID)

	require.Len(t, b.L2, 1)
	assert.Equal(t, uint(2), b.L2[0].Tag.ID)
	assert.Equal(t, uint(7), b.L2[0].TopicID)
	require.Len(t, b.L2[0].Candidates, 1, "L2 carries its candidate topic")

	require.Len(t, b.L3, 1)
	assert.Equal(t, uint(3), b.L3[0].ID)
}

func TestBucketTagsByCentroid_BoundaryL1IsL2(t *testing.T) {
	// dist == L1 (strict <) → L2. Use a threshold just below the tag's distance.
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0))}
	tags := []repository.TagInput{{ID: 1, Label: "at-l1"}}
	emb := map[uint][]float64{1: unit(0.8, 0.6)} // dist ~0.2
	cfg := laneCfg()
	cfg.LaneL1Threshold = 0.1999 // tag dist 0.2 is NOT < 0.1999 → L2
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, cfg)
	require.Len(t, b.L2, 1)
	assert.Empty(t, b.L1)
}

func TestBucketTagsByCentroid_BoundaryL2IsL2(t *testing.T) {
	// dist <= L2 → L2 (not L3). Tag dist ~0.2, L2 threshold 0.2.
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0))}
	tags := []repository.TagInput{{ID: 1, Label: "at-l2"}}
	emb := map[uint][]float64{1: unit(0.8, 0.6)}
	cfg := laneCfg()
	cfg.LaneL1Threshold = 0.10
	cfg.LaneL2Threshold = 0.21 // tag dist 0.2 <= 0.21 → L2
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, cfg)
	require.Len(t, b.L2, 1)
	assert.Empty(t, b.L3)
}

func TestBucketTagsByCentroid_BeyondL2IsL3(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0))}
	tags := []repository.TagInput{{ID: 1, Label: "far"}}
	emb := map[uint][]float64{1: unit(0.6, 0.8)} // dist ~0.4 > 0.30
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, laneCfg())
	require.Len(t, b.L3, 1)
}

func TestBucketTagsByCentroid_VacuumDowngrade(t *testing.T) {
	// Tag strong-matches a vacuum topic (dist < L1) → downgraded to L2.
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0)), IsVacuum: true}
	tags := []repository.TagInput{{ID: 1, Label: "would-be-l1"}}
	emb := map[uint][]float64{1: unit(1, 0)} // dist 0.0
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, laneCfg())
	require.Len(t, b.L2, 1, "vacuum topic forces strong-match tag to L2")
	assert.Empty(t, b.L1)
}

// ---- candidate L1 gate (candidate-topic-l2-gate) ----

// gateCfg returns a lane config with the candidate gate explicitly on
// (mirrors the production default from DefaultPersistentTopicConfig).
func gateCfg() repository.PersistentTopicConfig {
	cfg := laneCfg()
	cfg.CandidateL1GateEnabled = true
	return cfg
}

// TestBucketTagsByCentroid_CandidateNearDistanceDowngradesL2 covers spec
// scenario「candidate 近距离降级 L2（观察期门禁）」: with the gate on, a
// near-distance tag (dist < L1) whose nearest topic is candidate SHALL land in
// L2 carrying its candidate set, NOT direct-attach.
func TestBucketTagsByCentroid_CandidateNearDistanceDowngradesL2(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0)), Status: repository.TopicStatusCandidate}
	tags := []repository.TagInput{{ID: 1, Label: "candidate-near"}}
	emb := map[uint][]float64{1: unit(1, 0)} // dist 0.0 < 0.18
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, gateCfg())
	require.Len(t, b.L2, 1, "gate on: candidate near-distance tag downgraded to L2")
	assert.Empty(t, b.L1, "gate on: no direct attach to candidate topic")
	require.Len(t, b.L2[0].Candidates, 1, "L2 assign carries its candidate set")
	assert.Equal(t, uint(7), b.L2[0].Candidates[0].TopicID)
}

// TestBucketTagsByCentroid_GateOffRestoresLegacyL1 covers spec scenario
// 「门禁开关关闭回退旧行为」: with the gate off, a near-distance tag whose
// nearest topic is candidate direct-attaches (legacy behavior).
func TestBucketTagsByCentroid_GateOffRestoresLegacyL1(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0)), Status: repository.TopicStatusCandidate}
	tags := []repository.TagInput{{ID: 1, Label: "legacy-l1"}}
	emb := map[uint][]float64{1: unit(1, 0)}
	cfg := laneCfg()
	cfg.CandidateL1GateEnabled = false
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, cfg)
	require.Len(t, b.L1, 1, "gate off: candidate topic direct-attaches (legacy)")
	assert.Empty(t, b.L2)
}

// TestBucketTagsByCentroid_ActiveKeepsL1WithGateOn is a regression guard for
// spec scenario「L1 直挂命中」: with the gate on, ACTIVE topics keep direct
// attach — the gate must only affect candidates.
func TestBucketTagsByCentroid_ActiveKeepsL1WithGateOn(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0)), Status: repository.TopicStatusActive}
	tags := []repository.TagInput{{ID: 1, Label: "active-near"}}
	emb := map[uint][]float64{1: unit(1, 0)}
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, gateCfg())
	require.Len(t, b.L1, 1, "gate on: active topic still direct-attaches")
	assert.Empty(t, b.L2)
}

// TestBucketTagsByCentroid_VacuumActiveStillL2WithGateOn covers spec scenario
// 「vacuum active 不直挂（既有规则保持）」under the gate: vacuum downgrade is
// orthogonal to the candidate gate.
func TestBucketTagsByCentroid_VacuumActiveStillL2WithGateOn(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0)), IsVacuum: true, Status: repository.TopicStatusActive}
	tags := []repository.TagInput{{ID: 1, Label: "vacuum-near"}}
	emb := map[uint][]float64{1: unit(1, 0)}
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, emb, gateCfg())
	require.Len(t, b.L2, 1, "vacuum active still downgrades strong-match tag to L2")
	assert.Empty(t, b.L1)
}

func TestBucketTagsByCentroid_NoTopicsAllL3(t *testing.T) {
	tags := []repository.TagInput{{ID: 1, Label: "x"}, {ID: 2, Label: "y"}}
	emb := map[uint][]float64{1: unit(1, 0), 2: unit(0, 1)}
	b := BucketTagsByCentroid(tags, nil, emb, laneCfg())
	require.Len(t, b.L3, 2)
}

func TestBucketTagsByCentroid_MissingEmbeddingL3(t *testing.T) {
	topic := repository.BoardPersistentTopic{ID: 7, Centroid: repository.FloatsToPgVector(unit(1, 0))}
	tags := []repository.TagInput{{ID: 1, Label: "no-emb"}}
	b := BucketTagsByCentroid(tags, []repository.BoardPersistentTopic{topic}, map[uint][]float64{}, laneCfg())
	require.Len(t, b.L3, 1)
}

func TestBucketTagsByCentroid_TopKTruncation(t *testing.T) {
	// Tag at [1,0]; two topics both within L2 (dist 0.2 and 0.25); top-K=1
	// keeps only the nearest (topic 7).
	topics := []repository.BoardPersistentTopic{
		{ID: 7, Centroid: repository.FloatsToPgVector(unit(0.8, 0.6))},      // dist 0.2 (nearest)
		{ID: 8, Centroid: repository.FloatsToPgVector(unit(0.75, 0.66144))}, // dist 0.25
	}
	tags := []repository.TagInput{{ID: 1, Label: "weak"}}
	emb := map[uint][]float64{1: unit(1, 0)}
	cfg := laneCfg()
	cfg.L2CandidateK = 1
	b := BucketTagsByCentroid(tags, topics, emb, cfg)
	require.Len(t, b.L2, 1)
	require.Len(t, b.L2[0].Candidates, 1, "top-K=1 truncates candidates")
	assert.Equal(t, uint(7), b.L2[0].Candidates[0].TopicID, "nearest candidate kept")
}

// ---- parseL2Response ----

func l2Assign(tagID uint, cands ...LaneCandidate) LaneTagAssign {
	return LaneTagAssign{
		Tag: repository.TagInput{ID: tagID},
		TopicID: func() uint {
			if len(cands) > 0 {
				return cands[0].TopicID
			}
			return 0
		}(),
		Candidates: cands,
	}
}

func TestParseL2Response_KeepUsesNearest(t *testing.T) {
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2}, LaneCandidate{8, 0.25})}
	content := `{"decisions":[{"tag_id":1,"decision":"keep"}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "keep", dec[0].decision)
	assert.Equal(t, uint(7), dec[0].targetTopicID)
	assert.False(t, dec[0].offShortlist)
}

func TestParseL2Response_KeepHonorsInShortlistTarget(t *testing.T) {
	// 小模型常态混用 keep/switch（实测 ~10-20% 的 keep 会填非最近 target）：
	// keep 却显式指定候选集内另一话题 = LLM 的语义异议。parser SHALL 尊重该
	// 指定，而非静默改写回最近候选——否则 keep 会把 tag 无条件吸附回
	// embedding 最近处（含僵尸 candidate），LLM 判断被丢弃（卡里巴夫案例根因）。
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2}, LaneCandidate{8, 0.25})}
	content := `{"decisions":[{"tag_id":1,"decision":"keep","target_topic_id":8}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "keep", dec[0].decision)
	assert.Equal(t, uint(8), dec[0].targetTopicID, "in-shortlist keep target honored")
	assert.False(t, dec[0].offShortlist)
}

func TestParseL2Response_KeepOffShortlistTargetFallsBackNearest(t *testing.T) {
	// keep + 集外/空 target：维持安全网回最近候选（keep 不携带换轨强意图，
	// 不像 off-shortlist switch 那样降级 new）。
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2}, LaneCandidate{8, 0.25})}
	content := `{"decisions":[{"tag_id":1,"decision":"keep","target_topic_id":999}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "keep", dec[0].decision)
	assert.Equal(t, uint(7), dec[0].targetTopicID, "off-shortlist keep target falls back to nearest")
	assert.False(t, dec[0].offShortlist)
}

func TestParseL2Response_SwitchInShortlist(t *testing.T) {
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2}, LaneCandidate{8, 0.25})}
	content := `{"decisions":[{"tag_id":1,"decision":"switch","target_topic_id":8}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "switch", dec[0].decision)
	assert.Equal(t, uint(8), dec[0].targetTopicID)
}

func TestParseL2Response_SwitchOffShortlistDowngradesNew(t *testing.T) {
	// LLM picks a topic NOT in the candidate set → downgrade to new + flag.
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2})}
	content := `{"decisions":[{"tag_id":1,"decision":"switch","target_topic_id":999}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "new", dec[0].decision, "off-shortlist switch degrades to new")
	assert.True(t, dec[0].offShortlist, "off-shortlist flag set")
	assert.Equal(t, uint(0), dec[0].targetTopicID)
}

func TestParseL2Response_NewRouteToL3(t *testing.T) {
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2})}
	content := `{"decisions":[{"tag_id":1,"decision":"new"}]}`
	dec := parseL2Response(content, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "new", dec[0].decision)
}

func TestParseL2Response_MissingTagDefaultsKeep(t *testing.T) {
	// LLM omits a tag → default keep (anchor nearest).
	l2 := []LaneTagAssign{l2Assign(1, LaneCandidate{7, 0.2})}
	dec := parseL2Response(`{"decisions":[]}`, l2)
	require.Len(t, dec, 1)
	assert.Equal(t, "keep", dec[0].decision)
	assert.Equal(t, uint(7), dec[0].targetTopicID)
}

func TestParseL2Response_InvalidJSONAllKeep(t *testing.T) {
	l2 := []LaneTagAssign{
		l2Assign(1, LaneCandidate{7, 0.2}),
		l2Assign(2, LaneCandidate{8, 0.22}),
	}
	dec := parseL2Response(`not json`, l2)
	require.Len(t, dec, 2)
	for _, d := range dec {
		assert.Equal(t, "keep", d.decision)
	}
}

// ---- assembleLaneGroups ----

func TestAssembleLaneGroups_L1AndL2MergePerTopic(t *testing.T) {
	// L1 tag on topic 7 + L2 keep on topic 7 → one group, lane l1 (any L1 tag
	// makes the section l1_direct).
	l1 := []LaneTagAssign{{Tag: repository.TagInput{ID: 1}, TopicID: 7}}
	l2dec := []l2Decision{{tagID: 2, decision: "keep", targetTopicID: 7}}
	l3 := []repository.ClusterGroup{{GroupName: "new", TagIDs: []uint{3}, Lane: "l3"}}

	groups := assembleLaneGroups(l1, l2dec, l3)
	require.Len(t, groups, 2)
	// First group: topic 7, lane l1, both tags.
	require.NotNil(t, groups[0].MatchedTopicID)
	assert.Equal(t, uint(7), *groups[0].MatchedTopicID)
	assert.Equal(t, "l1", groups[0].Lane)
	assert.ElementsMatch(t, []uint{1, 2}, groups[0].TagIDs)
	// L3 group appended.
	assert.Equal(t, "l3", groups[1].Lane)
}

func TestAssembleLaneGroups_L2OnlyIsL2(t *testing.T) {
	l2dec := []l2Decision{{tagID: 1, decision: "keep", targetTopicID: 7}}
	groups := assembleLaneGroups(nil, l2dec, nil)
	require.Len(t, groups, 1)
	assert.Equal(t, "l2", groups[0].Lane)
	require.NotNil(t, groups[0].MatchedTopicID)
	assert.Equal(t, uint(7), *groups[0].MatchedTopicID)
}

func TestAssembleLaneGroups_L2SwitchMergesToTarget(t *testing.T) {
	l2dec := []l2Decision{{tagID: 1, decision: "switch", targetTopicID: 9}}
	groups := assembleLaneGroups(nil, l2dec, nil)
	require.Len(t, groups, 1)
	assert.Equal(t, "l2", groups[0].Lane)
	assert.Equal(t, uint(9), *groups[0].MatchedTopicID)
	assert.Equal(t, []uint{1}, groups[0].TagIDs)
}

func TestAssembleLaneGroups_L2NewRoutedAway(t *testing.T) {
	// L2 "new" decisions are NOT merged here (they were routed to the L3 pool).
	l2dec := []l2Decision{{tagID: 1, decision: "new"}}
	groups := assembleLaneGroups(nil, l2dec, nil)
	assert.Empty(t, groups)
}

// ---- clusterLaneToTier ----

func TestClusterLaneToTier(t *testing.T) {
	assert.Equal(t, laneL1Direct, clusterLaneToTier("l1"))
	assert.Equal(t, laneL2LLM, clusterLaneToTier("l2"))
	assert.Equal(t, laneL3New, clusterLaneToTier("l3"))
	assert.Equal(t, "", clusterLaneToTier(""), "unknown lane leaves column NULL")
}

// ---- buildL2Prompt (prompt hygiene: no historical thread narratives) ----

func l2PromptFixture() ([]LaneTagAssign, map[uint]repository.BoardPersistentTopic, map[uint][]repository.TopicRecentBrief) {
	l2 := []LaneTagAssign{{
		Tag:        repository.TagInput{ID: 1, Label: "中芯国际"},
		Candidates: []LaneCandidate{{TopicID: 10, Distance: 0.22}},
	}}
	topics := map[uint]repository.BoardPersistentTopic{
		10: {
			ID:           10,
			Label:        "半导体产业链",
			Status:       repository.TopicStatusActive,
			LastSeenDate: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
			HitCount:     5,
		},
	}
	briefs := map[uint][]repository.TopicRecentBrief{
		10: {{
			TopicID:    10,
			TagLabels:  []string{"半导体国产替代", "光刻机出口管制"},
			PeriodDate: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
		}},
	}
	return l2, topics, briefs
}

// TestBuildL2Prompt_ExcludesThreadTitles guards the prompt-hygiene invariant:
// historical thread titles must NOT be injected into the L2 adjudication prompt.
func TestBuildL2Prompt_ExcludesThreadTitles(t *testing.T) {
	l2, topics, briefs := l2PromptFixture()
	_, user := buildL2Prompt(l2, topics, briefs)
	assert.NotContains(t, user, "半导体链全线跌停引发市场恐慌",
		"L2 prompt must not contain historical thread title narrative")
	assert.NotContains(t, user, "thread",
		"L2 prompt must not render thread entries at all")
}

// TestBuildL2Prompt_KeepsTagLabelFingerprintAndMeta guards that non-narrative
// signals (label / status / hit meta / distance / recent tag labels) remain
// injected (candidate-topic-l2-gate: tag-label fact fingerprint replaces the
// frozen section_label).
func TestBuildL2Prompt_KeepsTagLabelFingerprintAndMeta(t *testing.T) {
	l2, topics, briefs := l2PromptFixture()
	_, user := buildL2Prompt(l2, topics, briefs)
	assert.Contains(t, user, "半导体产业链", "topic label kept")
	assert.Contains(t, user, "正式", "active status label kept")
	assert.Contains(t, user, "2026-03-07", "last-seen date kept")
	assert.Contains(t, user, "累计5天", "hit count kept")
	assert.Contains(t, user, "0.220", "distance kept")
	assert.Contains(t, user, "section (2026-03-06): 半导体国产替代 / 光刻机出口管制",
		"recent tag-label fact fingerprint rendered in `section (date): tag1 / tag2` form")
}

// TestBuildL2Prompt_CandidateStrictAdjudication covers spec scenario「L2 裁决中
// 观察中话题从严判断」(candidate-topic-l2-gate D5)：system prompt SHALL 含观察期
// 从严指令，观察中候选话题的近期 tag 标签 SHALL 注入 user prompt。
func TestBuildL2Prompt_CandidateStrictAdjudication(t *testing.T) {
	l2 := []LaneTagAssign{{
		Tag:        repository.TagInput{ID: 1, Label: "伊朗代防长发表强硬言论"},
		Candidates: []LaneCandidate{{TopicID: 20, Distance: 0.08}},
	}}
	topics := map[uint]repository.BoardPersistentTopic{
		20: {
			ID:           20,
			Label:        "伊朗议长透露凌晨应急决断",
			Status:       repository.TopicStatusCandidate,
			LastSeenDate: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
			HitCount:     10,
		},
	}
	briefs := map[uint][]repository.TopicRecentBrief{
		20: {{
			TopicID:    20,
			TagLabels:  []string{"伊朗议长透露哈梅内伊殉难后决断"},
			PeriodDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		}},
	}
	system, user := buildL2Prompt(l2, topics, briefs)
	// System carries the observation-period strict-keep instruction (D5).
	assert.Contains(t, system, "观察中")
	assert.Contains(t, system, "从严")
	// User renders the candidate's recent tag labels (fact fingerprint).
	assert.Contains(t, user, "状态:观察中", "candidate status rendered as 观察中")
	assert.Contains(t, user, "section (2026-03-01): 伊朗议长透露哈梅内伊殉难后决断",
		"candidate's recent tag labels injected")
}

// ---- 辅助标签约束闸门（topic-lane-auxiliary-label-constraint tasks 4.2~4.5）----
//
// 几何构造（二维单位向量，距离 = 1 - cos）：
//   话题质心 C = 0°（标签距质心 0.060 < L1 阈值 0.18 → 直挂候选）
//   以色列锤 A1 = -40°，黎巴嫩锤 A2 = +40°（两锤相距 0.234）
//   标签 A「以色列空袭加沙」= -20°：d(A1)=0.060、d(A2)=0.500 → 只覆盖以色列腿
//   标签 B「真主党与以军交火」= +20°：d(A2)=0.060、d(A1)=0.500 → 只覆盖黎巴嫩腿
// A+B 在同一个 group 里 → 分腿覆盖满足；只有 A → 黎巴嫩腿未覆盖 → 违例。

type constraintGateFixture struct {
	topics       []repository.BoardPersistentTopic
	tags         []repository.TagInput
	emb          map[uint][]float64
	labelVectors map[uint]repository.LaneLabelVector
	option       LaneConstraintOption
}

func newConstraintGateFixture() constraintGateFixture {
	israelOnly := unit(0.939693, -0.342020)   // -20°
	lebanonOnly := unit(0.939693, 0.342020)   // +20°
	israelAnchor := unit(0.766044, -0.642788) // -40°
	lebanonAnchor := unit(0.766044, 0.642788) // +40°

	topic := repository.BoardPersistentTopic{
		ID:       7,
		Label:    "以黎冲突升级：停火破裂与军事打击",
		Centroid: repository.FloatsToPgVector(unit(1, 0)),
		Status:   repository.TopicStatusActive,
	}
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, israelAnchor, nil, nil),
		laneConstraintFixture("黎巴嫩", repository.LaneConstraintModeRequired, lebanonAnchor, nil, nil),
	}
	return constraintGateFixture{
		topics: []repository.BoardPersistentTopic{topic},
		tags: []repository.TagInput{
			{ID: 1, Label: "以色列空袭加沙"},
			{ID: 2, Label: "真主党与以军交火"},
		},
		emb: map[uint][]float64{1: israelOnly, 2: lebanonOnly},
		labelVectors: map[uint]repository.LaneLabelVector{
			1: labelFixture(1, "qwen3-embedding", israelOnly),
			2: labelFixture(2, "qwen3-embedding", lebanonOnly),
		},
		option: WithLaneConstraints(
			map[uint][]repository.TopicLaneConstraint{topic.ID: constraints},
			map[uint]repository.LaneLabelVector{
				1: labelFixture(1, "qwen3-embedding", israelOnly),
				2: labelFixture(2, "qwen3-embedding", lebanonOnly),
			},
		),
	}
}

// stubLaneChat replaces clusterChatFn, records the calls, and returns the given
// content. The operation names are asserted by the individual tests.
func stubLaneChat(t *testing.T, content string) *[]string {
	t.Helper()
	calls := new([]string)
	prev := clusterChatFn
	clusterChatFn = func(_ context.Context, _, _ string, _ *airouter.JSONSchema, operation string) (string, error) {
		*calls = append(*calls, operation)
		return content, nil
	}
	t.Cleanup(func() { clusterChatFn = prev })
	return calls
}

// TestConstraintGate_SatisfiedGroupKeepsL1 covers Scenario「约束满足时 L1 直挂
// 不变」/「满足约束保持 L1 直挂」：分腿覆盖满足时组仍 l1_direct 且 SHALL NOT
// 调用 LLM。
func TestConstraintGate_SatisfiedGroupKeepsL1(t *testing.T) {
	f := newConstraintGateFixture()
	calls := stubLaneChat(t, `{"decisions":[]}`)

	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].MatchedTopicID)
	assert.Equal(t, uint(7), *groups[0].MatchedTopicID)
	assert.Equal(t, "l1", groups[0].Lane, "约束满足 → 保持 l1_direct")
	assert.Empty(t, *calls, "约束满足 SHALL NOT 产生额外 LLM 调用")
}

// TestConstraintGate_ViolatedGroupIsNotDirectAttached covers Scenario
// 「违反约束的 L1 直挂降级 L2」 / 「required 约束未覆盖时不得直挂」.
func TestConstraintGate_ViolatedGroupIsNotDirectAttached(t *testing.T) {
	f := newConstraintGateFixture()
	// 只保留以色列侧标签 → 黎巴嫩腿未覆盖。
	f.tags = f.tags[:1]
	f.emb = map[uint][]float64{1: f.emb[1]}
	f.labelVectors = map[uint]repository.LaneLabelVector{1: f.labelVectors[1]}
	f.option = WithLaneConstraints(f.option.ConstraintsByTopic, f.labelVectors)

	// LLM 判 keep（最近候选），但最近候选正是违例话题 → 沿用「降级 new」范式。
	calls := stubLaneChat(t, `{"decisions":[{"tag_id":1,"decision":"keep"}]}`)

	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, *calls, 1, "违例 group 补一次 L2 裁决（且只补一次）")
	assert.Equal(t, "daily_report.constraint_l2_rework", (*calls)[0])
	assert.Nil(t, groups[0].MatchedTopicID, "SHALL NOT 锚定到约束不满足的话题")
	assert.Equal(t, "l3", groups[0].Lane, "降级 new → lane_tier=l3_new")
}

// TestConstraintGate_BypassingL1DowngradesToL2Tier pins the tier contract: when
// a violating group is rescued onto another topic that satisfies its
// constraints, the final lane stays l2_llm (spec：违例的 L1 直挂降级进 L2 band).
func TestConstraintGate_BypassingL1DowngradesToL2Tier(t *testing.T) {
	f := newConstraintGateFixture()
	f.tags = f.tags[:1]
	f.emb = map[uint][]float64{1: f.emb[1]}
	f.labelVectors = map[uint]repository.LaneLabelVector{1: f.labelVectors[1]}
	f.option = WithLaneConstraints(f.option.ConstraintsByTopic, f.labelVectors)

	// 另一条无约束话题（ID 8）在候选集内：LLM 判 switch → 8。
	other := repository.BoardPersistentTopic{
		ID:       8,
		Label:    "以色列内政",
		Centroid: repository.FloatsToPgVector(unit(0.9, 0.435890)),
		Status:   repository.TopicStatusCandidate,
	}
	f.topics = append(f.topics, other)
	calls := stubLaneChat(t, `{"decisions":[{"tag_id":1,"decision":"switch","target_topic_id":8}]}`)

	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err)
	require.Len(t, *calls, 1)
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].MatchedTopicID)
	assert.Equal(t, uint(8), *groups[0].MatchedTopicID, "LLM 换到无约束话题 → 允许归属")
	assert.Equal(t, "l2", groups[0].Lane, "违例 group 的最终 lane_tier 仍是 l2_llm")
}

// TestConstraintGate_L2KeepOnViolatingTopicDowngrades covers Scenario
// 「L2 裁决选中约束不满足的话题时降级」：LLM 显式 switch 回违例话题也不行。
func TestConstraintGate_L2KeepOnViolatingTopicDowngrades(t *testing.T) {
	f := newConstraintGateFixture()
	f.tags = f.tags[:1]
	f.emb = map[uint][]float64{1: f.emb[1]}
	f.labelVectors = map[uint]repository.LaneLabelVector{1: f.labelVectors[1]}
	f.option = WithLaneConstraints(f.option.ConstraintsByTopic, f.labelVectors)

	calls := stubLaneChat(t, `{"decisions":[{"tag_id":1,"decision":"switch","target_topic_id":7}]}`)
	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err)
	require.Len(t, *calls, 1)
	require.Len(t, groups, 1)
	assert.Nil(t, groups[0].MatchedTopicID, "显式 switch 回违例话题 SHALL 被拦下")
	assert.Equal(t, "l3", groups[0].Lane)
}

// TestConstraintGate_MalformedLLMResponseDoesNotBypass covers tasks 4.5：漏答 /
// JSON 畸形时 parseL2Response 的安全网（keep 最近候选）SHALL NOT 绕过约束闸门。
func TestConstraintGate_MalformedLLMResponseDoesNotBypass(t *testing.T) {
	f := newConstraintGateFixture()
	f.tags = f.tags[:1]
	f.emb = map[uint][]float64{1: f.emb[1]}
	f.labelVectors = map[uint]repository.LaneLabelVector{1: f.labelVectors[1]}
	f.option = WithLaneConstraints(f.option.ConstraintsByTopic, f.labelVectors)

	calls := stubLaneChat(t, `not json at all`)
	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err)
	require.Len(t, *calls, 1)
	require.Len(t, groups, 1)
	assert.Nil(t, groups[0].MatchedTopicID, "畸形响应不得把违例 group 锚回违例话题")
	assert.Equal(t, "l3", groups[0].Lane)
}

// TestConstraintGate_NoConstraintsIsByteIdentical covers Scenario「无约束话题行为
// 不变」/「无约束话题的 L1 直挂不变」：不传约束上下文时输出与引入闸门之前逐字
// 一致，且零额外 LLM 调用。
func TestConstraintGate_NoConstraintsIsByteIdentical(t *testing.T) {
	f := newConstraintGateFixture()
	calls := stubLaneChat(t, `{"decisions":[]}`)

	withoutGate, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg())
	require.NoError(t, err)
	withEmptyGate, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), LaneConstraintOption{})
	require.NoError(t, err)

	assert.Equal(t, withoutGate, withEmptyGate, "无约束时输出 SHALL 逐字一致")
	assert.Empty(t, *calls, "无约束 SHALL NOT 产生额外 LLM 调用")
	require.Len(t, withoutGate, 1)
	assert.Equal(t, "l1", withoutGate[0].Lane)
}

// TestConstraintGate_LLMFailureNeverAnchors covers the failure path: when the
// rework call itself fails, violating groups degrade to new rather than being
// anchored to the violating topic.
func TestConstraintGate_LLMFailureNeverAnchors(t *testing.T) {
	f := newConstraintGateFixture()
	f.tags = f.tags[:1]
	f.emb = map[uint][]float64{1: f.emb[1]}
	f.labelVectors = map[uint]repository.LaneLabelVector{1: f.labelVectors[1]}
	f.option = WithLaneConstraints(f.option.ConstraintsByTopic, f.labelVectors)

	prev := clusterChatFn
	clusterChatFn = func(context.Context, string, string, *airouter.JSONSchema, string) (string, error) {
		return "", errors.New("provider down")
	}
	t.Cleanup(func() { clusterChatFn = prev })

	groups, err := ClusterTagsLane(context.Background(), f.tags, f.topics, f.emb, nil, laneCfg(), f.option)
	require.NoError(t, err, "闸门失败不阻断日报生成")
	require.Len(t, groups, 1)
	assert.Nil(t, groups[0].MatchedTopicID)
	assert.Equal(t, "l3", groups[0].Lane)
}
