package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubjectExplicitKeysCannotBeRewritten(t *testing.T) {
	for _, tc := range []struct{ name, subject, key, candidateSubject, candidateKey string }{
		{"real government versus president", "美国总统特朗普", "person:美国总统特朗普", "特朗普政府", "person:特朗普政府"},
		{"different types even with identical display", "特朗普", "person:特朗普", "特朗普", "org:特朗普政府"},
		{"literal cannot override keys", "特朗普", "person:donald_trump", "特朗普", "person:trump"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			embed := &countingEmbed{}
			match := ResolveSubjectKey(SubjectInput{Subject: tc.subject, SubjectKey: tc.key}, []SubjectCandidate{{Subject: tc.candidateSubject, SubjectKey: tc.candidateKey, SubjectEmbedding: []float64{0.99, math.Sqrt(1 - 0.99*0.99)}}}, embed.fn)
			assert.Equal(t, tc.key, match.SubjectKey)
			assert.Equal(t, SubjectMethodNew, match.Method)
			assert.Zero(t, embed.calls)
		})
	}
}

func TestSubjectKeylessAliasesEmbedding(t *testing.T) {
	for _, subject := range []string{"Trump", "川普", "特朗普"} {
		t.Run(subject, func(t *testing.T) {
			match := ResolveSubjectKey(SubjectInput{Subject: subject}, []SubjectCandidate{{Subject: "Donald Trump", SubjectKey: "person:trump", SubjectEmbedding: unit0p92}}, (&countingEmbed{}).fn)
			assert.Equal(t, SubjectMethodEmbedding, match.Method)
			assert.Equal(t, "person:trump", match.SubjectKey)
		})
	}
}

func TestDecideAssignmentRealGovernmentPresidentPair(t *testing.T) {
	candidate := StoryCandidate{ID: 3047, Subject: "特朗普政府", SubjectKey: "person:特朗普政府", Action: "计划发动打击", Object: "伊朗", OccurredAt: "2026-10-30"}
	input := AssignmentInput{Subject: "美国总统特朗普", SubjectKey: "person:美国总统特朗普", Action: "重申", Object: "伊朗问题将很快结束", OccurredAt: "2026-10-30", FingerprintTrusted: true}
	classifier := func(context.Context, AssignmentInput, []StoryCandidate) (uint, StoryDecision, float64, string, error) {
		t.Fatal("veto must not reach LLM")
		return 0, "", 0, "", nil
	}
	match := ResolveSubjectKey(SubjectInput{Subject: input.Subject, SubjectKey: input.SubjectKey}, []SubjectCandidate{{Subject: candidate.Subject, SubjectKey: candidate.SubjectKey, SubjectEmbedding: []float64{1, 0}}}, (&countingEmbed{}).fn)
	input.SubjectKey = match.SubjectKey
	result := DecideAssignment(context.Background(), input, []StoryCandidate{candidate}, classifier)
	assert.Equal(t, assignReasonSubjectVeto, result.Reason)
	assert.Empty(t, result.StoryIDs)
	assert.False(t, result.IsDevelopment)
	// Even a legacy rewritten key cannot bypass the strict object veto.
	input.SubjectKey = candidate.SubjectKey
	result = DecideAssignment(context.Background(), input, []StoryCandidate{candidate}, classifier)
	assert.Equal(t, assignReasonObjectVeto, result.Reason)
	assert.Empty(t, result.StoryIDs)
	assert.False(t, result.IsDevelopment)
	// Matching government identity and object still permits same-day development.
	input.Subject = candidate.Subject
	input.Object = candidate.Object
	result = DecideAssignment(context.Background(), input, []StoryCandidate{candidate}, classifier)
	require.Equal(t, StoryDecisionSameStory, result.Decision)
	assert.True(t, result.IsDevelopment)
	input.Action = candidate.Action
	result = DecideAssignment(context.Background(), input, []StoryCandidate{candidate}, classifier)
	assert.Equal(t, StoryDecisionSameOccurrence, result.Decision)
}
