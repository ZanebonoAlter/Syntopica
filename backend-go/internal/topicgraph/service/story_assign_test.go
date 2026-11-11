package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// recordingClassifier wraps a canned classifier with a call counter — the
// "vetoed tiers must not reach the LLM" and "no candidates → no LLM" rules
// are asserted via the count.
type recordingClassifier struct {
	calls      int
	id         uint
	decision   StoryDecision
	confidence float64
	reason     string
	err        error
}

func (c *recordingClassifier) fn(ctx context.Context, input AssignmentInput, candidates []StoryCandidate) (uint, StoryDecision, float64, string, error) {
	c.calls++
	if c.err != nil {
		return 0, "", 0, "", c.err
	}
	return c.id, c.decision, c.confidence, c.reason, nil
}

func trustedInput() AssignmentInput {
	return AssignmentInput{
		ThreadID: 100, Title: "A公司发布新品",
		Subject: "A公司", SubjectKey: "company:a",
		Action: "发布", Object: "X产品", OccurredAt: "2026-11-05",
		FingerprintTrusted: true,
	}
}

func candidate(id uint, key, action, object string, distance float64) StoryCandidate {
	return StoryCandidate{OccurredAt: "2026-11-05", ID: id, Subject: key, SubjectKey: key, Action: action, Object: object, Distance: distance, Origin: repository.StoryOriginRule}
}

// TestDecideAssignmentBranches walks test-cases.md §5.1's seven rows.
func TestDecideAssignmentBranches(t *testing.T) {
	t.Run("row1 subject veto beats everything", func(t *testing.T) {
		cl := &recordingClassifier{}
		in := trustedInput()
		// Vector distance 0 (identical embedding) must NOT override the veto.
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:b", "发布", "X产品", 0),
		}, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs, "vetoed candidate must not be attached")
		assert.Equal(t, repository.StoryOriginRule, result.Origin)
		assert.Equal(t, assignReasonSubjectVeto, result.Reason)
		assert.Equal(t, 0, cl.calls, "an all-vetoed candidate set must not reach the LLM")
	})

	t.Run("row2 full fingerprint match is SAME_OCCURRENCE and collects all hits", func(t *testing.T) {
		cl := &recordingClassifier{}
		result := DecideAssignment(context.Background(), trustedInput(), []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.4),
			candidate(2, "company:b", "发布", "X产品", 0.1),  // subject veto
			candidate(3, "company:a", "发布", "X产品", 0.45), // duplicate story line, also hits
		}, cl.fn)
		require.Equal(t, StoryDecisionSameOccurrence, result.Decision)
		assert.Equal(t, []uint{1, 3}, result.StoryIDs, "ALL occurrence hits are collected for the 2c merge")
		assert.False(t, result.IsDevelopment)
		assert.Equal(t, repository.StoryOriginRule, result.Origin)
		assert.Equal(t, float64(1), result.Confidence)
		assert.Equal(t, 0, cl.calls)
	})

	t.Run("row3 stage development is SAME_STORY with is_development", func(t *testing.T) {
		cl := &recordingClassifier{}
		in := trustedInput()
		in.Action = "开售"
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.3),
		}, cl.fn)
		require.Equal(t, StoryDecisionSameStory, result.Decision)
		assert.Equal(t, []uint{1}, result.StoryIDs)
		assert.True(t, result.IsDevelopment)
		assert.Equal(t, repository.StoryOriginRule, result.Origin)
		assert.Equal(t, assignReasonStageDevelopment, result.Reason)
		assert.Equal(t, 0, cl.calls)
	})

	t.Run("row3 tie among SAME_STORY hits picks the nearest", func(t *testing.T) {
		cl := &recordingClassifier{}
		in := trustedInput()
		in.Action = "降价"
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(5, "company:a", "发布", "X产品", 0.40),
			candidate(6, "company:a", "发布", "X产品", 0.12), // nearest wins
			candidate(7, "company:a", "发布", "X产品", 0.35),
		}, cl.fn)
		require.Equal(t, StoryDecisionSameStory, result.Decision)
		assert.Equal(t, []uint{6}, result.StoryIDs)
	})

	t.Run("row4 reverse action falls to LLM but SAME_STORY cannot attach", func(t *testing.T) {
		cl := &recordingClassifier{id: 1, decision: StoryDecisionSameStory, confidence: 0.9}
		in := trustedInput()
		in.Action = "召回"
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.3),
		}, cl.fn)
		require.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.False(t, result.IsDevelopment)
		assert.Equal(t, repository.StoryOriginModel, result.Origin)
		assert.Equal(t, "llm_same_story_blocked", result.Reason)
		assert.Zero(t, result.Confidence)
		assert.Equal(t, 1, cl.calls, "reverse action must be routed to the LLM, not the stage rule")
	})

	t.Run("row4 reverse action with LLM failure creates a new story", func(t *testing.T) {
		cl := &recordingClassifier{err: errors.New("provider down")}
		in := trustedInput()
		in.Action = "召回"
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.3),
		}, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, repository.StoryOriginModel, result.Origin)
		assert.Equal(t, assignReasonLLMError, result.Reason)
	})

	t.Run("row4 compound reverse phrase also skips the stage rule", func(t *testing.T) {
		// Review R-1: 提取侧 action 是自由动词短语，「宣布召回」对「发布」
		// 精确查表 miss——包含式兜底须接住，否则落入阶段规则直接合并。
		cl := &recordingClassifier{decision: StoryDecisionUnrelated, confidence: 0.9}
		in := trustedInput()
		in.Action = "宣布召回"
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.3),
		}, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision, "rule tier must not fuse a launch with its recall")
		assert.Empty(t, result.StoryIDs)
		assert.False(t, result.IsDevelopment, "is_development must not be set by the stage rule")
		assert.NotEqual(t, assignReasonStageDevelopment, result.Reason)
		assert.Equal(t, 1, cl.calls, "compound reverse action must be routed to the LLM, not the stage rule")
	})

	t.Run("row5 same subject different object is a new story", func(t *testing.T) {
		cl := &recordingClassifier{}
		result := DecideAssignment(context.Background(), trustedInput(), []StoryCandidate{
			candidate(1, "company:a", "裁员", "研发部门", 0.05),
		}, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonObjectVeto, result.Reason)
		assert.Equal(t, 0, cl.calls, "all-object-vetoed must not reach the LLM")
	})

	t.Run("row6 untrusted fingerprint still applies the subject veto", func(t *testing.T) {
		cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 0.9}
		in := trustedInput()
		in.FingerprintTrusted = false
		// Blatant subject-key mismatch: the veto is a rule fact and holds even
		// when the fingerprint is untrusted — the LLM never gets a chance to
		// override spec layer 1.
		result := DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:zzz", "whatever", "whatever", 0.3),
		}, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, repository.StoryOriginRule, result.Origin)
		assert.Equal(t, assignReasonSubjectVeto, result.Reason)
		assert.Equal(t, 0, cl.calls, "key-mismatched candidates must never reach the LLM")
	})

	t.Run("incomplete trusted fingerprint cannot use model fallback", func(t *testing.T) {
		in := trustedInput()
		in.Object = ""
		cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 1}
		got := DecideAssignment(t.Context(), in, []StoryCandidate{candidate(1, "company:a", "发布", "X产品", .1)}, cl.fn)
		assert.Empty(t, got.StoryIDs)
		assert.Equal(t, assignReasonMergeGateUntrusted, got.Reason)
		assert.Zero(t, cl.calls)
	})
}

func TestDecideAssignmentLLMConservativePaths(t *testing.T) {
	reverse := func() AssignmentInput {
		in := trustedInput()
		in.Action = "召回"
		return in
	}
	cands := []StoryCandidate{candidate(1, "company:a", "发布", "X产品", 0.3)}

	t.Run("low confidence creates new", func(t *testing.T) {
		cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 0.7499}
		result := DecideAssignment(context.Background(), reverse(), cands, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonLLMLowConfidence, result.Reason)
	})

	t.Run("exact threshold confidence cannot attach", func(t *testing.T) {
		cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: StoryClassifierMinConfidence}
		result := DecideAssignment(context.Background(), reverse(), cands, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonLLMSameOccurrenceBlocked, result.Reason)
	})

	t.Run("UNRELATED creates new", func(t *testing.T) {
		cl := &recordingClassifier{decision: StoryDecisionUnrelated, confidence: 0.99}
		result := DecideAssignment(context.Background(), reverse(), cands, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonLLMUnrelated, result.Reason)
	})

	t.Run("ROUNDUP keeps its label with no attach", func(t *testing.T) {
		cl := &recordingClassifier{decision: StoryDecisionRoundup, confidence: 0.99}
		result := DecideAssignment(context.Background(), reverse(), cands, cl.fn)
		assert.Equal(t, StoryDecisionRoundup, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonLLMRoundup, result.Reason)
	})

	t.Run("hallucinated candidate id creates new", func(t *testing.T) {
		cl := &recordingClassifier{id: 99, decision: StoryDecisionSameOccurrence, confidence: 0.95}
		result := DecideAssignment(context.Background(), reverse(), cands, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, assignReasonLLMInvalidID, result.Reason)
	})

	t.Run("vetoed candidates never reach the LLM input", func(t *testing.T) {
		// Mixed set: one reverse-action candidate (undecidable) plus one
		// subject-vetoed candidate. Only the undecidable one may be sent.
		sent := make([]StoryCandidate, 0, 2)
		classifier := func(ctx context.Context, input AssignmentInput, candidates []StoryCandidate) (uint, StoryDecision, float64, string, error) {
			sent = append(sent, candidates...)
			return 0, StoryDecisionUnrelated, 0.99, "", nil
		}
		in := trustedInput()
		in.Action = "召回"
		DecideAssignment(context.Background(), in, []StoryCandidate{
			candidate(1, "company:a", "发布", "X产品", 0.3),      // reverse → LLM
			candidate(2, "company:other", "发布", "X产品", 0.01), // subject veto → excluded
		}, classifier)
		require.Len(t, sent, 1)
		assert.Equal(t, uint(1), sent[0].ID)
	})
}

func TestDecideAssignmentNoCandidates(t *testing.T) {
	t.Run("trusted fingerprint, empty board day one", func(t *testing.T) {
		cl := &recordingClassifier{}
		result := DecideAssignment(context.Background(), trustedInput(), nil, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, repository.StoryOriginRule, result.Origin)
		assert.Equal(t, assignReasonNoCandidates, result.Reason)
		assert.Equal(t, 0, cl.calls, "no candidates must never trigger an LLM call")
	})

	t.Run("untrusted fingerprint, no candidates either", func(t *testing.T) {
		cl := &recordingClassifier{}
		in := trustedInput()
		in.FingerprintTrusted = false
		result := DecideAssignment(context.Background(), in, nil, cl.fn)
		assert.Equal(t, StoryDecisionUnrelated, result.Decision)
		assert.Empty(t, result.StoryIDs)
		assert.Equal(t, 0, cl.calls, "no candidates must never trigger an LLM call, even untrusted")
	})
}

func TestDecideAssignmentKeylessCandidate(t *testing.T) {
	// A story line with no subject_key fails the merge gate.
	cl := &recordingClassifier{decision: StoryDecisionUnrelated, confidence: 0.9}
	result := DecideAssignment(context.Background(), trustedInput(), []StoryCandidate{
		{ID: 1, Subject: "A公司", Action: "发布", Object: "X产品"},
	}, cl.fn)
	assert.Equal(t, StoryDecisionUnrelated, result.Decision)
	assert.Zero(t, cl.calls, "keyless candidates must not reach the LLM")
	assert.Equal(t, assignReasonMergeGateUntrusted, result.Reason)
}

func TestMatchEntityTextContainment(t *testing.T) {
	assert.True(t, matchEntityText("A产品", "X公司发布A产品"), "shorter side contained in longer")
	assert.True(t, matchEntityText("股价暴涨", "股价"), "either direction counts")
	assert.True(t, matchEntityText("ＧＰＴ－５", "gpt-5"), "full-width folds to ASCII")
	assert.False(t, matchEntityText("", "X产品"), "empty never matches")
	assert.False(t, matchEntityText("A产品", ""), "missing ≠ equal")
	assert.False(t, matchEntityText("A产品", "B产品"), "different products do not match")
}

func TestIsReverseAction(t *testing.T) {
	assert.True(t, IsReverseAction("发布", "召回"))
	assert.True(t, IsReverseAction("召回", "发布"), "symmetric")
	assert.True(t, IsReverseAction("发布", "下架"))
	assert.True(t, IsReverseAction("宣布", "否认"))
	assert.True(t, IsReverseAction("上架", "下架"))
	assert.True(t, IsReverseAction("推出", "召回"))
	assert.True(t, IsReverseAction(" 发布 ", "召回"), "trim + case fold")
	assert.True(t, IsReverseAction("Ｌａｕｎｃｈ", "launch") == false, "english words are not in the table")

	assert.False(t, IsReverseAction("发布", "涨价"))
	assert.False(t, IsReverseAction("发布", "发布"), "identical actions are stage-equal, not reverse")
	assert.True(t, IsReverseAction("发布", "召回产品"), "compound phrase 「召回产品」 carries the reverse word 召回 (exact-only lookup was the R-1 bug)")
	assert.False(t, IsReverseAction("", "召回"))
}

// TestIsReverseActionCompoundPhrases（review R-1）：提取侧 action 是自由动
// 词短语（「宣布召回」「正式下架」「公开否认」），精确查表漏复合形态——
// 包含式兜底：一侧含词表中某词、另一侧含与之成对的反向词，即判反向，
// 调用方转入 undecidable → LLM。误触发方向安全（LLM 保守处理），故不加
// 长度差约束——见 IsReverseAction 注释的权衡说明。
func TestIsReverseActionCompoundPhrases(t *testing.T) {
	assert.True(t, IsReverseAction("宣布召回", "发布"), "「宣布召回」含 召回，对侧含 发布：成对反向")
	assert.True(t, IsReverseAction("发布", "宣布召回"), "symmetric")
	assert.True(t, IsReverseAction("正式下架", "推出"), "「正式下架」含 下架，对侧含 推出：成对反向")
	assert.True(t, IsReverseAction("公开否认", "宣布"), "「公开否认」含 否认，对侧含 宣布：成对反向")
	assert.True(t, IsReverseAction("因质量问题全球召回", "发布"), "修饰较长的真反向短语也命中：误抑制会让阶段规则把发布与召回直接合并（spec 第 3 层例外条款）")

	// 同侧同词不构成对：相同/同向动作是 stage-equal，不是反向。
	assert.False(t, IsReverseAction("发布", "发布产品"), "both sides carry 发布 only: no pair direction")
	assert.False(t, IsReverseAction("发布召回公告", "发布召回公告"), "identical compound phrases are stage-equal, not reverse")
	assert.False(t, IsReverseAction("召回", "下架产品"), "召回 对 下架 不是词表成对（同为反向但不互为对子），交阶段规则/LLM")
}

// The subject alias has already been folded by subject normalization before assignment.
func TestDecideAssignmentStrictObjectAndStageDate(t *testing.T) {
	for _, tt := range []struct {
		name, object, inputDate, candidateDate, action string
		want                                           StoryDecision
	}{
		{"real Trump Iran containment three days", "伊朗问题将很快结束", "2026-11-05", "2026-11-02", "重申", StoryDecisionUnrelated},
		{"containment same day", "伊朗问题将很快结束", "2026-11-05", "2026-11-05", "重申", StoryDecisionUnrelated},
		{"stage adjacent day", "伊朗", "2026-11-05", "2026-11-04", "重申", StoryDecisionSameStory},
		{"stage two days", "伊朗", "2026-11-05", "2026-11-03", "重申", StoryDecisionUnrelated},
		{"stage missing input date", "伊朗", "", "2026-11-04", "重申", StoryDecisionUnrelated},
		{"stage missing candidate date", "伊朗", "2026-11-05", "", "重申", StoryDecisionUnrelated},
		{"stage both dates missing", "伊朗", "", "", "重申", StoryDecisionUnrelated},
		{"strict occurrence without date", "伊朗", "", "", "计划恢复打击", StoryDecisionSameOccurrence},
		{"occurrence cannot use containment", "伊朗问题将很快结束", "2026-11-05", "2026-11-05", "计划恢复打击", StoryDecisionUnrelated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := AssignmentInput{Subject: "特朗普", SubjectKey: "person:特朗普", Action: "计划恢复打击", Object: "伊朗", OccurredAt: tt.inputDate, FingerprintTrusted: true}
			c := StoryCandidate{ID: 1, Subject: "美国总统特朗普", SubjectKey: "person:特朗普", Action: tt.action, Object: tt.object, OccurredAt: tt.candidateDate}
			cl := &recordingClassifier{decision: StoryDecisionUnrelated, confidence: .99}
			got := DecideAssignment(t.Context(), in, []StoryCandidate{c}, cl.fn)
			assert.Equal(t, tt.want, got.Decision)
			assert.Equal(t, tt.want == StoryDecisionSameStory, got.IsDevelopment)
			if tt.want == StoryDecisionUnrelated {
				assert.Empty(t, got.StoryIDs)
			}
		})
	}
}

func TestDecideAssignmentStagePeriodDateFallback(t *testing.T) {
	for _, tt := range []struct {
		name, inputDate, candidateDate string
		gap                            int
		want                           StoryDecision
	}{
		{"report adjacent", "", "", 1, StoryDecisionSameStory},
		{"report two days", "", "", 2, StoryDecisionUnrelated},
		{"occurred date overrides near report", "2026-10-01", "2026-10-03", 1, StoryDecisionUnrelated},
		{"occurred date overrides distant report", "2026-10-01", "2026-10-02", 5, StoryDecisionSameStory},
		{"mixed date sources", "2026-11-04", "", 1, StoryDecisionSameStory},
		{"invalid occurred date cannot use report", "invalid", "", 1, StoryDecisionUnrelated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := trustedInput()
			in.OccurredAt = tt.inputDate
			in.PeriodDate = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
			c := candidate(1, "company:a", "开售", "X产品", .1)
			c.OccurredAt = tt.candidateDate
			c.PeriodDate = in.PeriodDate.AddDate(0, 0, -tt.gap)
			cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 1}
			got := DecideAssignment(t.Context(), in, []StoryCandidate{c}, cl.fn)
			assert.Equal(t, tt.want, got.Decision)
			assert.Equal(t, tt.want == StoryDecisionSameStory, got.IsDevelopment)
			assert.Zero(t, cl.calls, "date rejection must not be revived by LLM")
			got = DecideAssignment(t.Context(), AssignmentInput{SubjectKey: c.SubjectKey, Subject: c.Subject, Action: c.Action, Object: c.Object, OccurredAt: c.OccurredAt, PeriodDate: c.PeriodDate, FingerprintTrusted: true}, []StoryCandidate{{ID: 2, SubjectKey: in.SubjectKey, Action: in.Action, Object: in.Object, OccurredAt: in.OccurredAt, PeriodDate: in.PeriodDate}}, cl.fn)
			assert.Equal(t, tt.want, got.Decision, "date distance is symmetric")
		})
	}
}

func TestSameObjectStrict(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{" 伊朗 ", "伊朗", true}, {"Ｘ 产品", "x产品", true},
		{"伊朗", "伊朗核问题", false}, {"", "", false}, {"", "伊朗", false}, {"伊朗", " ", false},
	} {
		assert.Equal(t, tt.want, sameObjectStrict(tt.a, tt.b), "%q vs %q", tt.a, tt.b)
	}
}

// Untrusted or keyless evidence must never reach a model-assisted merge.
func TestDecideAssignmentMergeGate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*AssignmentInput, *StoryCandidate)
		want   StoryDecision
	}{
		{"untrusted input", func(in *AssignmentInput, _ *StoryCandidate) { in.FingerprintTrusted = false }, StoryDecisionUnrelated},
		{"missing input key", func(in *AssignmentInput, _ *StoryCandidate) { in.SubjectKey = "" }, StoryDecisionUnrelated},
		{"missing candidate key", func(_ *AssignmentInput, c *StoryCandidate) { c.SubjectKey = "" }, StoryDecisionUnrelated},
		{"trusted occurrence", func(_ *AssignmentInput, _ *StoryCandidate) {}, StoryDecisionSameOccurrence},
		{"trusted same day stage", func(in *AssignmentInput, _ *StoryCandidate) { in.Action = "开售" }, StoryDecisionSameStory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, c := trustedInput(), candidate(1, "company:a", "发布", "X产品", .1)
			tc.mutate(&in, &c)
			cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 1}
			got := DecideAssignment(t.Context(), in, []StoryCandidate{c}, cl.fn)
			assert.Equal(t, tc.want, got.Decision)
			assert.Zero(t, cl.calls)
			if tc.want == StoryDecisionUnrelated {
				assert.Empty(t, got.StoryIDs)
				assert.Equal(t, "merge_gate_untrusted", got.Reason)
			} else {
				assert.Equal(t, []uint{1}, got.StoryIDs)
			}
		})
	}
}

func TestDecideAssignmentLLMCannotMergeOccurrence(t *testing.T) {
	in := trustedInput()
	in.Action = "召回" // Reverse actions bypass stage rules; only ROUNDUP may survive the classifier.
	cl := &recordingClassifier{id: 1, decision: StoryDecisionSameOccurrence, confidence: 1}
	got := DecideAssignment(t.Context(), in, []StoryCandidate{candidate(1, "company:a", "发布", "X产品", .1)}, cl.fn)
	assert.Equal(t, 1, cl.calls)
	assert.Empty(t, got.StoryIDs)
	assert.Equal(t, StoryDecisionUnrelated, got.Decision)
}
