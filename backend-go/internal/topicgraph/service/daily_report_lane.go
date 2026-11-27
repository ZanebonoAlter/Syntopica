package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// Lane tier values persisted on DailyReportSection.LaneTier
// (l1_direct / l2_llm / l3_new + the watch materialization tiers
// watch_keyword / watch_sentence, watch-materialized-topic). Empty string
// leaves the column NULL.
const (
	laneL1Direct = "l1_direct"
	laneL2LLM    = "l2_llm"
	laneL3New    = "l3_new"

	LaneTierWatchKeyword  = "watch_keyword"
	LaneTierWatchSentence = "watch_sentence"
)

// LaneCandidate is one candidate topic for an L2 tag together with its cosine
// distance to the tag. Field order is TopicID, Distance so the unit tests can
// positionally initialize it as LaneCandidate{7, 0.2}.
type LaneCandidate struct {
	TopicID  uint
	Distance float64
}

// LaneTagAssign pairs a tag with the topic it nearest-matches and, for L2
// tags, the candidate set the LLM adjudicates over.
type LaneTagAssign struct {
	Tag        repository.TagInput
	TopicID    uint
	Distance   float64
	Candidates []LaneCandidate
}

// LaneBuckets holds the three lanes produced by BucketTagsByCentroid.
type LaneBuckets struct {
	L1 []LaneTagAssign
	L2 []LaneTagAssign
	L3 []repository.TagInput
}

// l2Decision is the parsed LLM adjudication for one L2 tag: keep / switch / new,
// the resolved target topic id, and an off-shortlist flag set when an LLM
// "switch" targeted a topic outside the candidate set (degraded to new).
type l2Decision struct {
	tagID         uint
	decision      string // keep / switch / new
	targetTopicID uint
	offShortlist  bool
}

// clusterChatFn is the swappable LLM hook for the lane pipeline. Tests replace
// it with a fake; the default implementation calls airouter exactly as the
// legacy ClusterTags entry did (temperature 0.1, maxTokens 8192, JSONMode with
// schema, CapabilityDigestPolish). The operation string distinguishes the
// daily_report.decide_l2_tags and daily_report.cluster_new_narrative calls.
var clusterChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
	temperature := 0.1
	maxTokens := 8192
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  operation,
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema:  schema,
		Metadata:    map[string]any{"operation": operation},
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// cosineFloats is the float-slice cosine distance (1 - cosine similarity).
// Mismatched, empty, or zero-norm vectors return MaxFloat64 so the caller
// treats them as non-matches. Distinct from merge.go's cosineDistance, which
// operates on pgvector strings.
func cosineFloats(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return math.MaxFloat64
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return math.MaxFloat64
	}
	return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
}

// topicAnchorVec returns the topic's matching anchor: the Centroid (mean of
// recent sections) when present, degrading to the 首义 Embedding otherwise.
// nil when neither parses — the topic cannot anchor any tag.
func topicAnchorVec(t repository.BoardPersistentTopic) []float64 {
	if v, err := parsePgVector(t.Centroid); err == nil && len(v) > 0 {
		return v
	}
	if v, err := parsePgVector(t.Embedding); err == nil && len(v) > 0 {
		return v
	}
	return nil
}

// BucketTagsByCentroid routes each tag to its lane by cosine distance to the
// nearest topic anchor (centroid, else 首义 embedding). Pure — no DB, no LLM.
//
//	dist < L1 && !vacuum && (gate off || active) → L1 (direct attach)
//	dist < L1 && (vacuum || gate on && candidate) → L2 (downgrade)
//	L1 <= dist <= L2                          → L2 (carries top-K candidates)
//	dist > L2                                 → L3
//	missing embedding / no anchors            → L3
//
// The candidate gate (candidate-topic-l2-gate, default on) strips the L1
// direct-attach privilege from candidate topics: system guesses must pass L2
// LLM adjudication for every attach, while user-confirmed active topics keep
// direct attach. Turning the gate off restores the legacy behavior where both
// active and candidate direct-attach.
func BucketTagsByCentroid(tags []repository.TagInput, topics []repository.BoardPersistentTopic, tagEmbeddings map[uint][]float64, cfg repository.PersistentTopicConfig) LaneBuckets {
	var buckets LaneBuckets

	type anchor struct {
		topic *repository.BoardPersistentTopic
		vec   []float64
	}
	anchors := make([]anchor, 0, len(topics))
	for i := range topics {
		vec := topicAnchorVec(topics[i])
		if len(vec) == 0 {
			continue
		}
		anchors = append(anchors, anchor{&topics[i], vec})
	}

	k := cfg.L2CandidateK
	if k <= 0 {
		k = len(anchors)
	}

	for _, tag := range tags {
		emb := tagEmbeddings[tag.ID]
		if len(emb) == 0 || len(anchors) == 0 {
			buckets.L3 = append(buckets.L3, tag)
			continue
		}

		type cand struct {
			id   uint
			dist float64
		}
		cands := make([]cand, 0, len(anchors))
		for _, a := range anchors {
			cands = append(cands, cand{a.topic.ID, cosineFloats(emb, a.vec)})
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].dist < cands[j].dist })

		nearestID := cands[0].id
		nearestDist := cands[0].dist
		var nearestTopic *repository.BoardPersistentTopic
		for _, a := range anchors {
			if a.topic.ID == nearestID {
				nearestTopic = a.topic
				break
			}
		}

		topK := k
		if topK > len(cands) {
			topK = len(cands)
		}
		candidates := make([]LaneCandidate, 0, topK)
		for i := 0; i < topK; i++ {
			candidates = append(candidates, LaneCandidate{TopicID: cands[i].id, Distance: cands[i].dist})
		}

		switch {
		case nearestDist < cfg.LaneL1Threshold && nearestTopic != nil && !nearestTopic.IsVacuum &&
			(!cfg.CandidateL1GateEnabled || nearestTopic.Status == repository.TopicStatusActive):
			buckets.L1 = append(buckets.L1, LaneTagAssign{Tag: tag, TopicID: nearestID, Distance: nearestDist})
		case nearestDist <= cfg.LaneL2Threshold:
			// (dist < L1 && vacuum) OR (dist < L1 && gate on && candidate) OR
			// (L1 <= dist <= L2) → L2.
			buckets.L2 = append(buckets.L2, LaneTagAssign{Tag: tag, TopicID: nearestID, Distance: nearestDist, Candidates: candidates})
		default:
			buckets.L3 = append(buckets.L3, tag)
		}
	}
	return buckets
}

// parseL2Response maps the LLM's L2 adjudication JSON onto a per-l2-tag
// decision slice, parallel and equal-length to the input l2. Unknown / invalid
// JSON or an omitted tag defaults to keep (anchor nearest). An LLM "switch" to
// a topic outside the candidate set degrades to new and sets offShortlist.
func parseL2Response(content string, l2 []LaneTagAssign) []l2Decision {
	type rawDecision struct {
		TagID         uint   `json:"tag_id"`
		Decision      string `json:"decision"`
		TargetTopicID *uint  `json:"target_topic_id"`
	}
	var raw struct {
		Decisions []rawDecision `json:"decisions"`
	}
	byTag := make(map[uint]rawDecision, len(l2))
	if cleaned := jsonutil.SanitizeLLMJSON(content); cleaned != "" {
		if err := json.Unmarshal([]byte(cleaned), &raw); err == nil {
			for _, d := range raw.Decisions {
				byTag[d.TagID] = d
			}
		}
	}

	out := make([]l2Decision, len(l2))
	for i, a := range l2 {
		var nearest uint
		if len(a.Candidates) > 0 {
			nearest = a.Candidates[0].TopicID
		}
		dec := l2Decision{tagID: a.Tag.ID, decision: "keep", targetTopicID: nearest}
		if rd, ok := byTag[a.Tag.ID]; ok {
			switch rd.Decision {
			case "keep":
				dec.decision = "keep"
				dec.targetTopicID = nearest
				// 小模型常态混用 keep/switch：keep 却显式指定候选集内另一话题，
				// 是 LLM 的语义异议（不想归最近候选）。尊重其指定；空/集外 target
				// 维持安全网回最近候选。否则 keep 会把 tag 无条件吸附回 embedding
				// 最近处（含僵尸 candidate），LLM 判断被静默丢弃（卡里巴夫案例根因）。
				if rd.TargetTopicID != nil && *rd.TargetTopicID != nearest && inCandidateSet(*rd.TargetTopicID, a.Candidates) {
					dec.targetTopicID = *rd.TargetTopicID
				}
			case "switch":
				if rd.TargetTopicID != nil && inCandidateSet(*rd.TargetTopicID, a.Candidates) {
					dec.decision = "switch"
					dec.targetTopicID = *rd.TargetTopicID
				} else {
					dec.decision = "new"
					dec.targetTopicID = 0
					dec.offShortlist = true
				}
			case "new":
				dec.decision = "new"
				dec.targetTopicID = 0
			default:
				dec.decision = "keep"
				dec.targetTopicID = nearest
			}
		}
		out[i] = dec
	}
	return out
}

func inCandidateSet(topicID uint, cands []LaneCandidate) bool {
	for _, c := range cands {
		if c.TopicID == topicID {
			return true
		}
	}
	return false
}

// assembleLaneGroups aggregates L1 + L2-keep/switch tags by their owning topic
// (L1 tags first, then L2 keep/switch), sets Lane="l1" on any topic group that
// has at least one L1 tag (else "l2"), and appends the L3 new-narrative groups
// verbatim. L2 "new" decisions are skipped here — they were routed to the L3
// pool upstream. Topic groups are emitted in ascending topicID order.
func assembleLaneGroups(l1 []LaneTagAssign, l2dec []l2Decision, l3 []repository.ClusterGroup) []repository.ClusterGroup {
	type acc struct {
		tagIDs []uint
		hasL1  bool
	}
	groups := make(map[uint]*acc)
	ensure := func(id uint) *acc {
		if groups[id] == nil {
			groups[id] = &acc{}
		}
		return groups[id]
	}

	for _, a := range l1 {
		g := ensure(a.TopicID)
		g.tagIDs = append(g.tagIDs, a.Tag.ID)
		g.hasL1 = true
	}
	for _, d := range l2dec {
		if d.decision != "keep" && d.decision != "switch" {
			continue
		}
		g := ensure(d.targetTopicID)
		g.tagIDs = append(g.tagIDs, d.tagID)
	}

	topicIDs := make([]uint, 0, len(groups))
	for id := range groups {
		topicIDs = append(topicIDs, id)
	}
	sort.Slice(topicIDs, func(i, j int) bool { return topicIDs[i] < topicIDs[j] })

	result := make([]repository.ClusterGroup, 0, len(topicIDs)+len(l3))
	for _, id := range topicIDs {
		tid := id
		lane := "l2"
		if groups[id].hasL1 {
			lane = "l1"
		}
		result = append(result, repository.ClusterGroup{
			GroupName:      "",
			TagIDs:         groups[id].tagIDs,
			MatchedTopicID: &tid,
			Lane:           lane,
		})
	}
	result = append(result, l3...)
	return result
}

// clusterLaneToTier maps a lane tag onto the persisted LaneTier column value.
// Unknown lanes (incl. "") map to "" so the column stays NULL.
func clusterLaneToTier(lane string) string {
	switch lane {
	case "l1":
		return laneL1Direct
	case "l2":
		return laneL2LLM
	case "l3":
		return laneL3New
	default:
		return ""
	}
}

// ClusterTagsLane is the lane-driven clustering entry point for the daily
// report orchestrator. It buckets tags by centroid distance (L1/L2/L3),
// adjudicates the L2 band with an LLM, groups the L3 band as new narrative,
// and emits []ClusterGroup carrying Lane + MatchedTopicID for the orchestrator
// to stamp onto each section.
// Variadic opts carry the auxiliary-label constraint context
// (topic-lane-auxiliary-label-constraint). Callers that pass nothing (and every
// test written before the change) get the exact legacy behaviour: no
// constraints ⇒ no gate ⇒ zero extra LLM calls.
func ClusterTagsLane(ctx context.Context, tags []repository.TagInput, topics []repository.BoardPersistentTopic, tagEmbeddings map[uint][]float64, briefs map[uint][]repository.TopicRecentBrief, cfg repository.PersistentTopicConfig, opts ...LaneOption) ([]repository.ClusterGroup, error) {
	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.cluster_tags_lane")
	defer span.End()

	if len(tags) == 0 {
		return nil, nil
	}

	topicByID := make(map[uint]repository.BoardPersistentTopic, len(topics))
	for _, t := range topics {
		topicByID[t.ID] = t
	}

	buckets := BucketTagsByCentroid(tags, topics, tagEmbeddings, cfg)
	l1 := buckets.L1
	l2 := buckets.L2
	laneOpts := laneConstraintOption(opts)
	laneRecalled := laneOpts.LaneRecalled

	// Lane-first-recall D4: tags admitted by the lane channel may only feed
	// EXISTING lanes. A lane witness that still landed in the L3 bucket (no day
	// embedding, or far from every anchorable topic) is dropped here instead of
	// seeding a new narrative group.
	l3pool, laneDropped := dropLaneRecalledTags(buckets.L3, laneRecalled)
	if laneDropped > 0 {
		logging.Warnf("daily-report: lane cluster dropped %d lane-recalled tags from the L3 pool (lane evidence must not seed new lanes)", laneDropped)
	}

	// Fallback: too few ambiguous tags to justify any LLM call. L1 still
	// aggregates per-topic; each L2 tag anchors to its nearest candidate
	// (keep); each L3 tag forms its own group. Zero LLM calls.
	if len(l2)+len(l3pool) <= 2 {
		l2dec := make([]l2Decision, len(l2))
		for i, a := range l2 {
			l2dec[i] = l2Decision{tagID: a.Tag.ID, decision: "keep", targetTopicID: a.TopicID}
		}
		l3groups := make([]repository.ClusterGroup, 0, len(l3pool))
		for _, t := range l3pool {
			l3groups = append(l3groups, repository.ClusterGroup{GroupName: t.Label, TagIDs: []uint{t.ID}, Lane: "l3"})
		}
		groups := assembleLaneGroups(l1, l2dec, l3groups)
		groups = ApplyLaneConstraintGate(ctx, groups, tags, topics, briefs, cfg, laneOpts)
		logging.Infof("daily-report: lane cluster (fallback, no LLM) %d tags → l1=%d l2=%d l3=%d groups=%d",
			len(tags), len(l1), len(l2), len(l3pool), len(groups))
		return groups, nil
	}

	// Normal path: L2 LLM adjudication over each tag's candidate set.
	system, user := buildL2Prompt(l2, topicByID, briefs)
	content, err := clusterChatFn(ctx, system, user, l2DecisionSchema(), "daily_report.decide_l2_tags")
	if err != nil {
		return nil, fmt.Errorf("l2 decide AI call failed: %w", err)
	}
	l2dec := parseL2Response(content, l2)

	// Route L2 "new" decisions (incl. off-shortlist downgrades) into the L3 pool.
	// Lane-recalled tags are dropped instead (lane-first-recall D4): their only
	// admission evidence is lane proximity, which justifies feeding an existing
	// lane, not opening a new one.
	l2Dropped := 0
	for i, d := range l2dec {
		if d.decision != "new" || i >= len(l2) {
			continue
		}
		if laneRecalled[l2[i].Tag.ID] {
			l2Dropped++
			continue
		}
		l3pool = append(l3pool, l2[i].Tag)
	}
	if l2Dropped > 0 {
		logging.Warnf("daily-report: lane cluster dropped %d lane-recalled tags adjudicated as new by L2 (no L3 seeding)", l2Dropped)
	}

	// L3 new-narrative grouping.
	var l3groups []repository.ClusterGroup
	if len(l3pool) > 2 {
		// Pure new-narrative grouping: no existing-topic framework injection
		// (pass nil topics so matched_topic_id stays null). Reuses the legacy
		// title rules from buildClusterSystemPrompt/buildClusterPrompt.
		sys := buildClusterSystemPrompt(len(l3pool), nil, nil)
		usr := buildClusterPrompt(l3pool)
		l3content, l3err := clusterChatFn(ctx, sys, usr, l3GroupsSchema(), "daily_report.cluster_new_narrative")
		if l3err != nil {
			return nil, fmt.Errorf("l3 cluster AI call failed: %w", l3err)
		}
		parsed, perr := parseClusterResponse(l3content, l3pool, nil)
		if perr != nil {
			return nil, fmt.Errorf("parse l3 cluster response: %w", perr)
		}
		l3groups = parsed
		for i := range l3groups {
			l3groups[i].Lane = "l3"
			l3groups[i].MatchedTopicID = nil
		}
	} else {
		l3groups = make([]repository.ClusterGroup, 0, len(l3pool))
		for _, t := range l3pool {
			l3groups = append(l3groups, repository.ClusterGroup{GroupName: t.Label, TagIDs: []uint{t.ID}, Lane: "l3"})
		}
	}

	groups := assembleLaneGroups(l1, l2dec, l3groups)
	groups = ApplyLaneConstraintGate(ctx, groups, tags, topics, briefs, cfg, laneOpts)
	logging.Infof("daily-report: lane cluster %d tags → l1=%d l2=%d l3=%d groups=%d",
		len(tags), len(l1), len(l2), len(l3pool), len(groups))
	return groups, nil
}

// buildL2Prompt constructs the L2 adjudication system + user prompts. Each L2
// tag is rendered with its top-K candidate topics (label, status, distance) and
// each candidate's recent section/thread content (when briefs are available) so
// the LLM judges attribution by narrative substance, not label surface.
func buildL2Prompt(l2 []LaneTagAssign, topicByID map[uint]repository.BoardPersistentTopic, briefs map[uint][]repository.TopicRecentBrief) (system, user string) {
	system = `你是一名专业的新闻叙事分析师。下面给出若干"待定"标签，每个标签附带其最近的若干已有话题候选（含近期内容）。请判断每个标签的归属：
- keep：标签应归入其最近候选话题（语义最贴近的现有框架），target_topic_id 填该最近候选
- switch：标签应归入候选集内的另一个话题（而非最近候选），target_topic_id 填该话题
- new：标签属于尚未覆盖的全新叙事，不归属任何候选，target_topic_id 省略或为 0

判断须基于候选话题的实际近期内容，而非仅凭标题字面沾边；两个话题即使标题字面相似，若各自近期内容分属不同叙事，应视为不同框架。
状态为「观察中」的候选话题尚未经用户确认：请依据其近期实际内容从严判断 keep；若其近期内容与待判标签分属不同事件，应判 new。
只返回合法 JSON，不要 Markdown 代码块或解释文字：{"decisions":[{"tag_id":整数,"decision":"keep|switch|new","target_topic_id":整数}]}`

	var sb strings.Builder
	sb.WriteString("## 待定标签与候选话题\n\n")
	for _, a := range l2 {
		fmt.Fprintf(&sb, "### 标签 [ID:%d] %s\n", a.Tag.ID, a.Tag.Label)
		if a.Tag.Description != "" {
			fmt.Fprintf(&sb, "描述: %s\n", a.Tag.Description)
		}
		if a.Tag.ArticleContext != "" {
			fmt.Fprintf(&sb, "代表文章: %s\n", a.Tag.ArticleContext)
		}
		sb.WriteString("候选话题:\n")
		for _, c := range a.Candidates {
			t, ok := topicByID[c.TopicID]
			if !ok {
				continue
			}
			statusLabel := "正式"
			if t.Status == repository.TopicStatusCandidate {
				statusLabel = "观察中"
			}
			fmt.Fprintf(&sb, "- [topic:%d] %s（状态:%s，最近命中:%s，累计%d天，距离%.3f）\n",
				t.ID, t.Label, statusLabel, t.LastSeenDate.Format("2006-01-02"), t.HitCount, c.Distance)
			if items := briefs[t.ID]; len(items) > 0 {
				var frame strings.Builder
				for _, item := range items {
					if len(item.TagLabels) == 0 {
						continue // merged/disabled tags only → no fact fingerprint to show
					}
					fmt.Fprintf(&frame, "\n  - section (%s): %s",
						item.PeriodDate.Format("2006-01-02"), strings.Join(item.TagLabels, " / "))
				}
				if frame.Len() > 0 {
					sb.WriteString("  近期 section 实际标签:")
					sb.WriteString(frame.String())
					sb.WriteString("\n")
				}
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("请为以上每个标签给出 keep / switch / new 决策。\n")
	return system, sb.String()
}

// l2DecisionSchema is the JSON schema for the L2 adjudication response.
func l2DecisionSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"decisions": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"tag_id":          {Type: "integer"},
						"decision":        {Type: "string", Enum: []string{"keep", "switch", "new"}},
						"target_topic_id": {Type: "integer"},
					},
					Required: []string{"tag_id", "decision"},
				},
			},
		},
		Required: []string{"decisions"},
	}
}

// l3GroupsSchema is the JSON schema for the L3 new-narrative grouping response.
// No matched_topic_id: L3 groups are brand-new narratives.
func l3GroupsSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"groups": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"group_name": {Type: "string", Description: "组名，不超过20字"},
						"tag_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
					},
					Required: []string{"group_name", "tag_ids"},
				},
			},
		},
		Required: []string{"groups"},
	}
}

// ── 辅助标签约束闸门（topic-lane-auxiliary-label-constraint tasks 4.2~4.5）──────

// LaneOption carries the optional lane-pipeline context into ClusterTagsLane:
// the auxiliary-label constraint gate and the lane-first-recall admission
// marker. It is optional on purpose: a caller that passes nothing (and every
// topic without constraint rows) skips the gate entirely — the spec's
// 「无约束话题行为不变，SHALL NOT 产生额外 LLM 调用」is structurally true, not a
// branch that could regress.
type LaneOption struct {
	// ConstraintsByTopic holds the constraint rows per topic. Empty map = no
	// gate (存量话题零开销).
	ConstraintsByTopic map[uint][]repository.TopicLaneConstraint
	// LabelVectors holds the day's tag vectors + their embedding model, resolved
	// through article_topic_tags (NOT cluster_tag_ids — 92.8% dangling). Tags
	// absent from the map cannot be judged and are skipped.
	LabelVectors map[uint]repository.LaneLabelVector
	// LaneRecalled marks tags whose seat in the candidate set rests on the lane
	// recall channel (lane-first-recall D4). Such a tag SHALL NOT seed a new
	// narrative group: lane proximity is a "belongs to this existing lane"
	// signal, never a "a new lane should exist" signal.
	LaneRecalled map[uint]bool
}

// LaneConstraintOption is the historical name of LaneOption (introduced by
// topic-lane-auxiliary-label-constraint). Kept as an alias so existing callers
// keep compiling.
type LaneConstraintOption = LaneOption

// WithLaneConstraints builds the gate option from the loaded constraint rows.
func WithLaneConstraints(constraints map[uint][]repository.TopicLaneConstraint, labelVectors map[uint]repository.LaneLabelVector) LaneOption {
	return LaneOption{ConstraintsByTopic: constraints, LabelVectors: labelVectors}
}

// WithLaneRecalledTags carries the lane-recall admission marker into the lane
// pipeline (lane-first-recall D4).
func WithLaneRecalledTags(laneRecalled map[uint]bool) LaneOption {
	return LaneOption{LaneRecalled: laneRecalled}
}

// laneConstraintOption merges the variadic options into one context. Merging
// (instead of taking opts[0]) keeps constraint gating and lane recall
// composable — the two call sites pass separate options.
func laneConstraintOption(opts []LaneOption) LaneOption {
	var out LaneOption
	for _, opt := range opts {
		if opt.ConstraintsByTopic != nil {
			out.ConstraintsByTopic = opt.ConstraintsByTopic
		}
		if opt.LabelVectors != nil {
			out.LabelVectors = opt.LabelVectors
		}
		if opt.LaneRecalled != nil {
			out.LaneRecalled = opt.LaneRecalled
		}
	}
	return out
}

// dropLaneRecalledTags removes lane-recalled tags from the L3 pool: the pool's
// only purpose is to seed new narrative lanes, and lane evidence must not do
// that (lane-first-recall D4 硬上界). Returns the filtered pool and the number
// of dropped tags.
func dropLaneRecalledTags(pool []repository.TagInput, laneRecalled map[uint]bool) ([]repository.TagInput, int) {
	if len(laneRecalled) == 0 || len(pool) == 0 {
		return pool, 0
	}
	kept := make([]repository.TagInput, 0, len(pool))
	dropped := 0
	for _, tag := range pool {
		if laneRecalled[tag.ID] {
			dropped++
			continue
		}
		kept = append(kept, tag)
	}
	return kept, dropped
}

// laneGroupLabelVectors projects a lane group's tag ids onto the judgeable
// label set (cross-space / dangling / deleted tags already filtered upstream).
func laneGroupLabelVectors(tagIDs []uint, labelVectors map[uint]repository.LaneLabelVector) []repository.LaneLabelVector {
	out := make([]repository.LaneLabelVector, 0, len(tagIDs))
	for _, id := range tagIDs {
		if v, ok := labelVectors[id]; ok {
			out = append(out, v)
		}
	}
	return out
}

// laneGroupConstraintCoverage judges one lane group against one topic's
// constraints. hasConstraints=false means the topic is unconstrained and the
// group must be left untouched.
func laneGroupConstraintCoverage(tagIDs []uint, topicID uint, opt LaneConstraintOption, cfg repository.PersistentTopicConfig) (LaneConstraintCoverage, bool) {
	constraints := opt.ConstraintsByTopic[topicID]
	if len(constraints) == 0 {
		return LaneConstraintCoverage{Verdict: LaneConstraintSatisfied}, false
	}
	return EvaluateLaneConstraintCoverage(constraints, laneGroupLabelVectors(tagIDs, opt.LabelVectors), cfg), true
}

// ApplyLaneConstraintGate enforces the auxiliary-label constraints on the lane
// groups produced by assembleLaneGroups: a group whose tag set does not cover
// its topic's required constraints SHALL NOT anchor that topic (design D5).
//
// 违例 group 不直接丢弃，而是**补一次 L2 裁决**（tasks 4.3）：LLM 可能识别出该
// 内容更属于另一个话题。裁决结果仍选中一个约束未覆盖的话题时，沿用既有
// 「switch 到候选集外 → 降级 new」范式转 L3（tasks 4.4）；漏答 / JSON 畸形时
// parseL2Response 的安全网（keep 最近候选）同样要再过一遍约束闸门（tasks 4.5）。
//
// 全程只发**一次** LLM 调用（所有违例 group 合并成一轮），且只在确有违例时发。
// 调用失败时违例 group 一律降级 new——SHALL NOT 因为上游不可用就把内容锚到
// 违例话题上（红线优先于「report 不能失败」的便利）。
func ApplyLaneConstraintGate(ctx context.Context, groups []repository.ClusterGroup, tags []repository.TagInput, topics []repository.BoardPersistentTopic, briefs map[uint][]repository.TopicRecentBrief, cfg repository.PersistentTopicConfig, opt LaneConstraintOption) []repository.ClusterGroup {
	if len(groups) == 0 || len(opt.ConstraintsByTopic) == 0 {
		return groups
	}

	violating := make([]int, 0, len(groups))
	for i := range groups {
		if groups[i].MatchedTopicID == nil {
			continue
		}
		cov, hasConstraints := laneGroupConstraintCoverage(groups[i].TagIDs, *groups[i].MatchedTopicID, opt, cfg)
		if !hasConstraints || !cov.Blocks() {
			continue
		}
		logging.Infof("daily-report: lane group (tags=%v) violates topic %d constraints (uncovered=%v) → L2 rework",
			groups[i].TagIDs, *groups[i].MatchedTopicID, cov.Uncovered)
		violating = append(violating, i)
	}
	if len(violating) == 0 {
		return groups
	}

	topicByID := make(map[uint]repository.BoardPersistentTopic, len(topics))
	topicVecs := make(map[uint][]float64, len(topics))
	for _, t := range topics {
		topicByID[t.ID] = t
		if v := topicAnchorVec(t); len(v) > 0 {
			topicVecs[t.ID] = v
		}
	}
	tagByID := make(map[uint]repository.TagInput, len(tags))
	for _, t := range tags {
		tagByID[t.ID] = t
	}

	assigns := make([]LaneTagAssign, 0, len(violating))
	groupTagIDs := make([][]uint, 0, len(violating))
	for _, idx := range violating {
		assigns = append(assigns, buildLaneConstraintAssign(groups[idx], tagByID, opt.LabelVectors, topicVecs, cfg))
		groupTagIDs = append(groupTagIDs, groups[idx].TagIDs)
	}

	system, user := buildL2Prompt(assigns, topicByID, briefs)
	system += "\n注意：下面部分候选话题绑定了 required 辅助标签约束，其约束未被覆盖的话题不可归属——这些话题请判 new。"
	user += buildLaneConstraintStatusBlock(assigns, groupTagIDs, opt, cfg)

	content, err := clusterChatFn(ctx, system, user, l2DecisionSchema(), "daily_report.constraint_l2_rework")
	if err != nil {
		logging.Warnf("daily-report: constraint rework AI call failed (%v) — %d violating lane group(s) degrade to new (SHALL NOT anchor a violating topic)", err, len(violating))
		for _, idx := range violating {
			groups[idx] = downgradeLaneGroupToNew(groups[idx], tagByID)
		}
		return groups
	}
	decisions := parseL2Response(content, assigns)

	for k, idx := range violating {
		dec := decisions[k]
		target := dec.targetTopicID
		if dec.decision != "keep" && dec.decision != "switch" {
			target = 0
		}
		if target != 0 {
			// 裁决结果必须再过一遍约束闸门（tasks 4.4/4.5）：LLM 仍选中一个约束
			// 未覆盖的话题时降级 new，而不是「LLM 说了算」。
			if cov, hasConstraints := laneGroupConstraintCoverage(groups[idx].TagIDs, target, opt, cfg); hasConstraints && cov.Blocks() {
				logging.Infof("daily-report: constraint rework picked topic %d whose constraints are still uncovered (uncovered=%v) → new narrative",
					target, cov.Uncovered)
				target = 0
			}
		}
		if target == 0 {
			groups[idx] = downgradeLaneGroupToNew(groups[idx], tagByID)
			continue
		}
		topicID := target
		groups[idx].MatchedTopicID = &topicID
		// 违例 group 的最终 lane_tier 仍是 l2_llm（spec：违例的 L1 直挂降级进 L2 band）。
		groups[idx].Lane = "l2"
	}
	return groups
}

// buildLaneConstraintAssign renders one violating lane group as a single L2
// adjudication unit. The synthetic tag id is the group's smallest tag id — it
// is unique across groups (groups partition the day's tags), so parseL2Response
// can map decisions back without inventing ids the LLM would have to echo.
func buildLaneConstraintAssign(group repository.ClusterGroup, tagByID map[uint]repository.TagInput, labelVectors map[uint]repository.LaneLabelVector, topicVecs map[uint][]float64, cfg repository.PersistentTopicConfig) LaneTagAssign {
	anchorID := uint(0)
	labels := make([]string, 0, len(group.TagIDs))
	for _, id := range group.TagIDs {
		if anchorID == 0 || id < anchorID {
			anchorID = id
		}
		if t, ok := tagByID[id]; ok && t.Label != "" {
			labels = append(labels, t.Label)
		}
	}
	if len(labels) > 5 {
		labels = labels[:5]
	}
	base := tagByID[anchorID]
	tag := repository.TagInput{
		ID:             anchorID,
		Label:          strings.Join(labels, " / "),
		Category:       base.Category,
		ArticleContext: base.ArticleContext,
	}

	groupVectors := laneGroupLabelVectors(group.TagIDs, labelVectors)

	// Candidate ordering is only a hint for the prompt: the nearest distance
	// from the group's tag set to each topic anchor, ascending.
	type cand struct {
		id   uint
		dist float64
	}
	cands := make([]cand, 0, len(topicVecs))
	for id, vec := range topicVecs {
		best := math.MaxFloat64
		for _, label := range groupVectors {
			d := cosineFloats(label.Vector, vec)
			if d < best {
				best = d
			}
		}
		if !isFiniteDistance(best) {
			best = 1.0 // 无法计算距离时的中性占位（提示用，不参与准入判定）
		}
		cands = append(cands, cand{id: id, dist: best})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist == cands[j].dist {
			return cands[i].id < cands[j].id
		}
		return cands[i].dist < cands[j].dist
	})
	k := cfg.L2CandidateK
	if k <= 0 || k > len(cands) {
		k = len(cands)
	}
	out := make([]LaneCandidate, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, LaneCandidate{TopicID: cands[i].id, Distance: cands[i].dist})
	}
	nearestID := uint(0)
	nearestDist := 0.0
	if len(out) > 0 {
		nearestID = out[0].TopicID
		nearestDist = out[0].Distance
	}
	return LaneTagAssign{Tag: tag, TopicID: nearestID, Distance: nearestDist, Candidates: out}
}

// buildLaneConstraintStatusBlock renders, for every reworked group, each
// candidate topic's constraint coverage state (tasks 4.3「候选含各话题的约束
// 状态」）. Only required legs are listed — preferred constraints never block.
// groupTagIDs is parallel to assigns (each entry = that group's real tag ids).
func buildLaneConstraintStatusBlock(assigns []LaneTagAssign, groupTagIDs [][]uint, opt LaneConstraintOption, cfg repository.PersistentTopicConfig) string {
	var sb strings.Builder
	sb.WriteString("\n## 约束准入（required 辅助标签）\n\n")
	sb.WriteString("约束未被完整覆盖的话题不可归属，请判 new：\n")
	for i, a := range assigns {
		var tagIDs []uint
		if i < len(groupTagIDs) {
			tagIDs = groupTagIDs[i]
		}
		labels := laneGroupLabelVectors(tagIDs, opt.LabelVectors)
		fmt.Fprintf(&sb, "\n标签 [ID:%d] %s\n", a.Tag.ID, a.Tag.Label)
		for _, c := range a.Candidates {
			constraints := opt.ConstraintsByTopic[c.TopicID]
			if len(constraints) == 0 {
				fmt.Fprintf(&sb, "- [topic:%d] 无约束\n", c.TopicID)
				continue
			}
			cov := EvaluateLaneConstraintCoverage(constraints, labels, cfg)
			preferred := laneConstraintPreferredTerms(constraints)
			if preferred != "" {
				fmt.Fprintf(&sb, "- [topic:%d] 要求:%s（偏好:%s，仅提示不拦截） → %s\n",
					c.TopicID, laneConstraintTerms(constraints), preferred, laneConstraintCoverageLabel(cov))
				continue
			}
			fmt.Fprintf(&sb, "- [topic:%d] 要求:%s → %s\n",
				c.TopicID, laneConstraintTerms(constraints), laneConstraintCoverageLabel(cov))
		}
	}
	return sb.String()
}

// laneConstraintTerms renders the required terms of a topic (the legs the LLM
// must see covered).
func laneConstraintTerms(constraints []repository.TopicLaneConstraint) string {
	terms := make([]string, 0, len(constraints))
	for _, c := range constraints {
		mode := c.Mode
		if mode == "" {
			mode = repository.LaneConstraintModeRequired
		}
		if mode != repository.LaneConstraintModeRequired {
			continue
		}
		terms = append(terms, c.Term)
	}
	if len(terms) == 0 {
		return "无 required"
	}
	return strings.Join(terms, ", ")
}

// laneConstraintPreferredTerms renders the preferred (non-blocking) terms of a
// topic. Preferred constraints never gate anchoring (spec「约束覆盖判定语义」);
// they only reach the L2 prompt as a preference hint so the mode is not a dead
// switch in the UI.
func laneConstraintPreferredTerms(constraints []repository.TopicLaneConstraint) string {
	terms := make([]string, 0, len(constraints))
	for _, c := range constraints {
		if c.Mode != repository.LaneConstraintModePreferred {
			continue
		}
		terms = append(terms, c.Term)
	}
	return strings.Join(terms, ", ")
}

// laneConstraintCoverageLabel renders the coverage verdict in prompt language.
func laneConstraintCoverageLabel(cov LaneConstraintCoverage) string {
	switch {
	case cov.Blocks():
		return "未覆盖 " + strings.Join(cov.Uncovered, ", ")
	case cov.Verdict == LaneConstraintUndecided:
		return "无法判定（放行）"
	default:
		return "已全部覆盖"
	}
}

// downgradeLaneGroupToNew turns a group into an L3 new-narrative group (no
// topic anchor). The group name is filled from the first tag label so the
// section still renders a readable title.
func downgradeLaneGroupToNew(group repository.ClusterGroup, tagByID map[uint]repository.TagInput) repository.ClusterGroup {
	if group.GroupName == "" && len(group.TagIDs) > 0 {
		if t, ok := tagByID[group.TagIDs[0]]; ok {
			group.GroupName = t.Label
		}
	}
	group.MatchedTopicID = nil
	group.Lane = "l3"
	return group
}
