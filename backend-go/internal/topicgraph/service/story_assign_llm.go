package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

// LLM occurrence-only story classification (add-event-story-heat task 3.1 LLM half,
// specs/event-story「事件归属判定顺序」tier 5). The classifier only sees the
// narrow band the rule tiers could not decide: untrusted fingerprints,
// keyless candidates, and reverse-action pairs. Every failure mode degrades
// conservatively in DecideAssignment (error / low confidence / UNRELATED /
// ROUNDUP / hallucinated id → new story line) — this file only produces the
// verdict, never the fallback.
//
// Call style follows watch_adjudicate.go: one batch chat call, temperature
// 0.1, JSONMode + JSONSchema, hallucinated candidate ids filtered at parse.

// NewStoryClassifier returns the production StoryClassifier backed by
// airouter (CapabilityTopicTagging — the same capability as label
// extraction, so it rides the same provider routing and logging).
func NewStoryClassifier() StoryClassifier {
	return func(ctx context.Context, input AssignmentInput, candidates []StoryCandidate) (uint, StoryDecision, float64, string, error) {
		if len(candidates) == 0 {
			// DecideAssignment guarantees non-empty candidates; guard anyway —
			// classifying against nothing is a caller bug, not an AI failure.
			return 0, "", 0, "", fmt.Errorf("story classify: no candidates to compare")
		}
		if len(candidates) > storyClassifyCandidateLimit {
			candidates = candidates[:storyClassifyCandidateLimit]
		}
		temperature := 0.1
		maxTokens := 1024
		result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
			Operation:  "topicgraph.story_assign_classify",
			SessionID:  SessionIDFromContext(ctx),
			Capability: airouter.CapabilityTopicTagging,
			Messages: []airouter.Message{
				{Role: "system", Content: storyClassifySystemPrompt()},
				{Role: "user", Content: buildStoryClassifyPrompt(input, candidates)},
			},
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
			JSONMode:    true,
			JSONSchema:  storyClassifySchema(),
			Metadata: map[string]any{
				"operation":       "topicgraph_story_assign_classify",
				"thread_id":       input.ThreadID,
				"candidate_count": len(candidates),
				"prompt_version":  storyClassifyPromptVersion,
			},
		})
		if err != nil {
			return 0, "", 0, "", fmt.Errorf("story classify AI call failed: %w", err)
		}

		ids := make([]uint, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		id, decision, confidence, reason, err := parseStoryClassification(result.Content, ids)
		if err != nil {
			logging.Warnf("story classify: invalid output for thread %d: %v", input.ThreadID, err)
		}
		return id, decision, confidence, reason, err
	}
}

// storyClassifyPromptVersion versions the classification prompt wording for
// observability (bump on any criteria change).
const storyClassifyPromptVersion = "3"

// storyClassifySummaryRunes caps the event summary in the prompt.
const storyClassifySummaryRunes = 300

const (
	storyClassifyCandidateLimit        = 10
	storyClassifyCandidateSummaryRunes = 120
	storyClassifyTitleRunes            = 80
	storyClassifyFieldRunes            = 40
	storyClassifyPromptRunes           = 6000
)

// storyClassifySystemPrompt allows occurrence merges only. Without trusted
// fingerprints, LLM stage merging is unconstrained (44% measured false merges);
// stage merging must be handled by the trusted-fingerprint rule tiers.
func storyClassifySystemPrompt() string {
	return `你是一名新闻编辑，判断一条新事件与候选事件线的关系。只从三类中选一类：

- SAME_OCCURRENCE：新事件是某条候选事件线同一次发生的复述——同一主体、同一动作、同一对象，只是不同来源在报道同一天的同件事。
- UNRELATED：新事件与所有候选都不是同一事件，或拿不准是不是同一次发生。
- ROUNDUP：新事件本身是综述/盘点/多事件汇总，没有单一事件身份。

规则：
- 只有确认是同一次发生才判 SAME_OCCURRENCE；同一主题但不同次发生一律 UNRELATED，包括同一事件线的不同阶段/进展
- 拿不准时判 UNRELATED 并给低置信度，宁可拆分不可错并
- SAME_OCCURRENCE 必须给出实际候选列表中的 candidate_id，否则判定无效；不得猜测或默认选择第一个候选
- candidate_id 必须输出；UNRELATED/ROUNDUP 时填 0
- confidence 取 0~1，表达你对判定的把握`
}

// buildStoryClassifyPrompt renders the new event + candidate list.
func buildStoryClassifyPrompt(input AssignmentInput, candidates []StoryCandidate) string {
	var sb strings.Builder
	sb.WriteString("## 新事件\n\n")
	fmt.Fprintf(&sb, "标题：%s\n", truncateRunes(input.Title, storyClassifyTitleRunes))
	if s := truncateRunes(input.Summary, storyClassifySummaryRunes); s != "" {
		fmt.Fprintf(&sb, "摘要：%s\n", s)
	}
	fmt.Fprintf(&sb, "主体：%s（键 %s）｜动作：%s｜对象：%s\n", storyPromptField(input.Subject), storyPromptField(input.SubjectKey), storyPromptField(input.Action), storyPromptField(input.Object))
	if input.OccurredAt != "" {
		fmt.Fprintf(&sb, "发生日期：%s\n", storyPromptField(input.OccurredAt))
	}

	sb.WriteString("\n## 候选事件线\n\n")
	for i, c := range candidates {
		if i >= storyClassifyCandidateLimit {
			break
		}
		fmt.Fprintf(&sb, "- [id:%d]", c.ID)
		if c.Title != "" {
			fmt.Fprintf(&sb, " 标题：%s｜摘要：%s", truncateRunes(c.Title, storyClassifyTitleRunes), truncateRunes(c.Summary, storyClassifyCandidateSummaryRunes))
		}
		if strings.TrimSpace(c.Subject) == "" || strings.TrimSpace(c.SubjectKey) == "" || strings.TrimSpace(c.Action) == "" || strings.TrimSpace(c.Object) == "" {
			sb.WriteString("（指纹缺失：不可信）")
		} else {
			fmt.Fprintf(&sb, " 主体：%s（键 %s）｜动作：%s｜对象：%s", storyPromptField(c.Subject), storyPromptField(c.SubjectKey), storyPromptField(c.Action), storyPromptField(c.Object))
		}
		if c.OccurredAt != "" {
			fmt.Fprintf(&sb, "｜发生日期：%s", storyPromptField(c.OccurredAt))
		}
		sb.WriteByte('\n')
	}

	// Field and count caps keep the complete candidate list below this guard.
	// Keep the output contract intact even for pathological input.
	const output = "\n输出 JSON：{\"candidate_id\":1,\"decision\":\"SAME_OCCURRENCE\",\"confidence\":0.9,\"reason\":\"一句话理由\"}\n"
	return truncateRunes(sb.String(), storyClassifyPromptRunes-len([]rune(output))) + output
}

// rawStoryClassification mirrors the LLM JSON contract.
type rawStoryClassification struct {
	CandidateID uint    `json:"candidate_id"`
	Decision    string  `json:"decision"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
}

// parseStoryClassification sanitizes and validates the LLM response:
//   - legacy SAME_STORY is parsed so DecideAssignment can warn and reject it;
//     decision must otherwise be one of the three schema values (anything else is an
//     error — the caller treats it as a conservative new story);
//   - SAME_* verdicts must reference a candidate id from the sent list
//     (hallucinated ids are errors, not silent new-story);
//   - UNRELATED/ROUNDUP drop any candidate id (nothing to attach);
//   - confidence is clamped to [0,1].
func parseStoryClassification(content string, candidateIDs []uint) (uint, StoryDecision, float64, string, error) {
	content = jsonutil.SanitizeLLMJSON(content)
	var raw rawStoryClassification
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return 0, "", 0, "", fmt.Errorf("parse story classification JSON: %w", err)
	}

	decision := StoryDecision(strings.ToUpper(strings.TrimSpace(raw.Decision)))
	switch decision {
	case StoryDecisionSameOccurrence, StoryDecisionSameStory:
		if raw.CandidateID == 0 {
			return 0, "", 0, "", fmt.Errorf("story classification %s requires a nonzero candidate_id", decision)
		}
		if !uintIDsContain(candidateIDs, raw.CandidateID) {
			return 0, "", 0, "", fmt.Errorf("story classification candidate_id %d not in candidate list %v", raw.CandidateID, candidateIDs)
		}
		return raw.CandidateID, decision, clampStoryConfidence(raw.Confidence), strings.TrimSpace(raw.Reason), nil
	case StoryDecisionUnrelated, StoryDecisionRoundup:
		return 0, decision, clampStoryConfidence(raw.Confidence), strings.TrimSpace(raw.Reason), nil
	default:
		return 0, "", 0, "", fmt.Errorf("story classification invalid decision %q", raw.Decision)
	}
}

// uintIDsContain reports membership for the hallucination filter.
func uintIDsContain(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// clampStoryConfidence bounds an LLM-reported confidence to [0,1].
func clampStoryConfidence(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

func storyClassifySchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"candidate_id": {Type: "integer", Description: "判定的候选事件线 ID；UNRELATED/ROUNDUP 时为 0"},
			"decision":     {Type: "string", Enum: []string{"SAME_OCCURRENCE", "UNRELATED", "ROUNDUP"}},
			"confidence":   {Type: "number", Description: "判定置信度 0~1"},
			"reason":       {Type: "string", Description: "一句话判定理由"},
		},
		Required: []string{"candidate_id", "decision", "confidence", "reason"},
	}
}

func storyPromptField(value string) string {
	return truncateRunes(value, storyClassifyFieldRunes)
}
