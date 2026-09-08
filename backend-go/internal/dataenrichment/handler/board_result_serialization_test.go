package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
)

func TestSerializeBoardResultKindParentAndQuestionKey(t *testing.T) {
	parentID := uint(42)
	questionKey := repository.ComputeQuestionKey("为什么投资增长没有带来就业增长？")
	serialized := serializeBoardResult(&repository.TopicEnrichmentResult{
		ID:              43,
		SemanticBoardID: repository.BoardIDPtr(9),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardInvestigation,
		ParentResultID:  &parentID,
		QuestionKey:     &questionKey,
	})

	require.Equal(t, repository.ResultKindBoardInvestigation, serialized["result_kind"])
	require.Equal(t, &parentID, serialized["parent_result_id"])
	require.Equal(t, &questionKey, serialized["question_key"])
}

func TestSerializeBoardResultKindLegacyFallback(t *testing.T) {
	serialized := serializeBoardResult(&repository.TopicEnrichmentResult{AnalysisScope: "board"})
	require.Equal(t, repository.ResultKindLegacyBoardAnalysis, serialized["result_kind"])
	require.Nil(t, serialized["parent_result_id"])
	require.Nil(t, serialized["question_key"])
}
