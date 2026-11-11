package service

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoryClassificationValid(t *testing.T) {
	ids := []uint{1, 5, 9}

	t.Run("SAME_OCCURRENCE with valid id", func(t *testing.T) {
		id, decision, conf, reason, err := parseStoryClassification(
			`{"candidate_id":5,"decision":"SAME_OCCURRENCE","confidence":0.93,"reason":"同一天同一发布会"}`, ids)
		require.NoError(t, err)
		assert.Equal(t, uint(5), id)
		assert.Equal(t, StoryDecisionSameOccurrence, decision)
		assert.InDelta(t, 0.93, conf, 1e-9)
		assert.Equal(t, "同一天同一发布会", reason)
	})

	t.Run("SAME_STORY with valid id", func(t *testing.T) {
		id, decision, conf, _, err := parseStoryClassification(
			`{"candidate_id":1,"decision":"SAME_STORY","confidence":0.88,"reason":"开售是发布的推进"}`, ids)
		require.NoError(t, err)
		assert.Equal(t, uint(1), id)
		assert.Equal(t, StoryDecisionSameStory, decision)
		assert.InDelta(t, 0.88, conf, 1e-9)
	})

	t.Run("UNRELATED drops the candidate id", func(t *testing.T) {
		id, decision, _, _, err := parseStoryClassification(
			`{"candidate_id":5,"decision":"UNRELATED","confidence":0.95,"reason":"主体不同"}`, ids)
		require.NoError(t, err)
		assert.Equal(t, uint(0), id, "UNRELATED never attaches")
		assert.Equal(t, StoryDecisionUnrelated, decision)
	})

	t.Run("ROUNDUP drops the candidate id", func(t *testing.T) {
		id, decision, _, _, err := parseStoryClassification(
			`{"decision":"ROUNDUP","confidence":0.9,"reason":"盘点类稿件"}`, ids)
		require.NoError(t, err)
		assert.Equal(t, uint(0), id)
		assert.Equal(t, StoryDecisionRoundup, decision)
	})

	t.Run("lowercase decision folds up", func(t *testing.T) {
		_, decision, _, _, err := parseStoryClassification(
			`{"decision":"same_story","candidate_id":9,"confidence":0.8}`, ids)
		require.NoError(t, err)
		assert.Equal(t, StoryDecisionSameStory, decision)
	})
}

func TestParseStoryClassificationInvalid(t *testing.T) {
	ids := []uint{1, 5}

	t.Run("illegal decision", func(t *testing.T) {
		_, _, _, _, err := parseStoryClassification(
			`{"candidate_id":1,"decision":"MAYBE","confidence":0.9}`, ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid decision")
	})

	t.Run("hallucinated candidate id", func(t *testing.T) {
		_, _, _, _, err := parseStoryClassification(
			`{"candidate_id":99,"decision":"SAME_STORY","confidence":0.9}`, ids)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in candidate list")
	})

	t.Run("garbage JSON", func(t *testing.T) {
		_, _, _, _, err := parseStoryClassification(`不是 JSON`, ids)
		require.Error(t, err)
	})
}

func TestParseStoryClassificationRobustness(t *testing.T) {
	ids := []uint{1, 5}

	t.Run("confidence clamped from above and below", func(t *testing.T) {
		_, _, conf, _, err := parseStoryClassification(
			`{"candidate_id":1,"decision":"SAME_OCCURRENCE","confidence":1.7}`, ids)
		require.NoError(t, err)
		assert.InDelta(t, 1.0, conf, 1e-9)

		_, _, conf, _, err = parseStoryClassification(
			`{"candidate_id":1,"decision":"SAME_OCCURRENCE","confidence":-0.3}`, ids)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, conf, 1e-9)
	})

	t.Run("markdown fence and prose prefix are cleaned", func(t *testing.T) {
		content := "判定如下：\n```json\n{\"candidate_id\":5,\"decision\":\"SAME_STORY\",\"confidence\":0.91,\"reason\":\"阶段推进\"}\n```"
		id, decision, _, _, err := parseStoryClassification(content, ids)
		require.NoError(t, err)
		assert.Equal(t, uint(5), id)
		assert.Equal(t, StoryDecisionSameStory, decision)
	})
}

func TestBuildStoryClassifyPrompt(t *testing.T) {
	in := AssignmentInput{
		ThreadID: 42, Title: "A公司开售X产品",
		Summary: "A公司今日宣布X产品正式开售。", Subject: "A公司", SubjectKey: "company:a",
		Action: "开售", Object: "X产品", OccurredAt: "2026-11-05",
	}
	prompt := buildStoryClassifyPrompt(in, []StoryCandidate{
		{ID: 7, Subject: "A公司", SubjectKey: "company:a", Action: "发布", Object: "X产品", OccurredAt: "2026-11-01"},
	})
	assert.Contains(t, prompt, "A公司开售X产品")
	assert.Contains(t, prompt, "company:a")
	assert.Contains(t, prompt, "[id:7]")
	assert.Contains(t, prompt, "2026-11-01")
	assert.Contains(t, prompt, `"decision"`)
}

func TestNewStoryClassifierRejectsEmptyCandidates(t *testing.T) {
	// Pure guard: the production classifier errors on an empty candidate set
	// instead of burning an AI call (DecideAssignment never sends one).
	_, _, _, _, err := NewStoryClassifier()(t.Context(), AssignmentInput{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no candidates")
}

func TestStoryClassifierMissingIDNeverAttaches(t *testing.T) {
	for _, content := range []string{
		`{"decision":"SAME_OCCURRENCE","confidence":0.99}`,
		`{"candidate_id":999,"decision":"SAME_STORY","confidence":0.99}`,
	} {
		classifier := func(_ context.Context, _ AssignmentInput, candidates []StoryCandidate) (uint, StoryDecision, float64, string, error) {
			return parseStoryClassification(content, []uint{7})
		}
		in := trustedInput()
		in.Action = "召回"
		result := DecideAssignment(t.Context(), in, []StoryCandidate{candidate(7, "company:a", "发布", "X产品", .1)}, classifier)
		assert.Empty(t, result.StoryIDs, "invalid output must never attach")
		assert.Equal(t, assignReasonLLMError, result.Reason)
	}
	_, _, _, _, err := parseStoryClassification(`{"decision":"SAME_OCCURRENCE","confidence":0.99}`, []uint{0, 7})
	require.Error(t, err, "missing ID must be invalid even if caller supplies a zero candidate")
	id, decision, _, _, err := parseStoryClassification(`{"candidate_id":0,"decision":"UNRELATED","confidence":0.99}`, []uint{7})
	require.NoError(t, err)
	assert.Zero(t, id)
	assert.Equal(t, StoryDecisionUnrelated, decision)
}

func TestStoryClassifierPromptEvidenceAndBudget(t *testing.T) {
	assert.Contains(t, storyClassifySystemPrompt(), "SAME_OCCURRENCE 必须")
	candidates := make([]StoryCandidate, 11)
	for i := range candidates {
		candidates[i].ID = uint(i + 1)
		candidates[i].Title = strings.Repeat("题", 10000)
		candidates[i].Summary = strings.Repeat("摘", 10000)
		candidates[i].Subject = strings.Repeat("长", 10000)
		candidates[i].SubjectKey = strings.Repeat("键", 10000)
		candidates[i].Action = strings.Repeat("动", 10000)
		candidates[i].Object = strings.Repeat("物", 10000)
		candidates[i].OccurredAt = strings.Repeat("日", 10000)
	}
	prompt := buildStoryClassifyPrompt(AssignmentInput{Title: strings.Repeat("题", 10000)}, candidates)
	assert.Contains(t, buildStoryClassifyPrompt(AssignmentInput{}, []StoryCandidate{{ID: 7}}), "指纹缺失：不可信")
	assert.Contains(t, prompt, "[id:10]")
	assert.NotContains(t, prompt, "[id:11]")
	assert.LessOrEqual(t, utf8.RuneCountInString(prompt), 6000)
	assert.Contains(t, prompt, "输出 JSON")
}

func TestStoryClassifierSchemaRequiresCandidateID(t *testing.T) {
	assert.ElementsMatch(t, []string{"candidate_id", "decision", "confidence", "reason"}, storyClassifySchema().Required)
}

func TestStoryClassifierCandidateSummaryTruncated(t *testing.T) {
	prompt := buildStoryClassifyPrompt(AssignmentInput{}, []StoryCandidate{{ID: 7, Title: "候选代表标题", Summary: strings.Repeat("摘", 120) + "不应出现"}})
	assert.Contains(t, prompt, "候选代表标题")
	assert.Contains(t, prompt, strings.Repeat("摘", 120))
	assert.NotContains(t, prompt, "不应出现")
}

func TestStoryClassifierCannotMerge(t *testing.T) {
	for _, tc := range []struct {
		decision     string
		wantDecision StoryDecision
		wantIDs      []uint
	}{
		{"SAME_STORY", StoryDecisionUnrelated, nil},
		{"SAME_OCCURRENCE", StoryDecisionUnrelated, nil},
	} {
		t.Run(tc.decision, func(t *testing.T) {
			classifier := func(_ context.Context, _ AssignmentInput, _ []StoryCandidate) (uint, StoryDecision, float64, string, error) {
				return parseStoryClassification(`{"candidate_id":7,"decision":"`+tc.decision+`","confidence":0.99}`, []uint{7})
			}
			in := trustedInput()
			in.Action = "召回"
			result := DecideAssignment(t.Context(), in, []StoryCandidate{candidate(7, "company:a", "发布", "X产品", .1)}, classifier)
			assert.Equal(t, tc.wantDecision, result.Decision)
			assert.Equal(t, tc.wantIDs, result.StoryIDs)
			assert.False(t, result.IsDevelopment)
		})
	}
}

func TestStoryClassifierSchemaOnlyAllowsOccurrenceMerge(t *testing.T) {
	assert.ElementsMatch(t, []string{"SAME_OCCURRENCE", "UNRELATED", "ROUNDUP"}, storyClassifySchema().Properties["decision"].Enum)
	assert.NotContains(t, storyClassifySystemPrompt(), "SAME_STORY")
	assert.Contains(t, storyClassifySystemPrompt(), "同一主题但不同次发生一律 UNRELATED")
	assert.NotContains(t, buildStoryClassifyPrompt(AssignmentInput{}, []StoryCandidate{{ID: 7}}), "SAME_STORY")
}
