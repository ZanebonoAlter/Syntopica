package daily_report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeduplicateTags_Empty(t *testing.T) {
	result := DeduplicateTags(nil, nil)
	assert.Nil(t, result)
}

func TestDeduplicateTags_IdenticalArticleSets(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Tag A", ArticleCount: 3},
		{ID: 2, Label: "Tag B", ArticleCount: 3},
		{ID: 3, Label: "Tag C", ArticleCount: 5},
	}
	articleSets := [][]uint{
		{10, 20, 30},
		{10, 20, 30},
		{40, 50},
	}

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 2)
	// Tag B has same set as Tag A but same article count and higher ID → Tag A survives.
	// Tag A (ID:1) vs Tag B (ID:2): same article set, same count → keep lower ID (Tag A)
	assert.Equal(t, uint(1), result[0].ID)
	assert.Equal(t, uint(3), result[1].ID)
}

func TestDeduplicateTags_SingleArticleOverlap(t *testing.T) {
	tags := []TagInput{
		{ID: 10, Label: "Tag X", ArticleCount: 1},
		{ID: 20, Label: "Tag Y", ArticleCount: 1},
		{ID: 30, Label: "Tag Z", ArticleCount: 1},
	}
	articleSets := [][]uint{
		{100},
		{100},
		{200},
	}

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 2)
	// Tag X and Tag Y share article 100 → keep lower ID (10)
	// Tag Z has different article (200) → survives
	ids := tagIDs(result)
	assert.Contains(t, ids, uint(10))
	assert.Contains(t, ids, uint(30))
}

func TestDeduplicateTags_HigherArticleCountWins(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Small", ArticleCount: 2},
		{ID: 5, Label: "Big", ArticleCount: 5},
	}
	articleSets := [][]uint{
		{10, 20},
		{10, 20},
	}

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(5), result[0].ID) // Higher article_count wins
}

func TestDeduplicateTags_MismatchedLength(t *testing.T) {
	tags := []TagInput{{ID: 1, Label: "A"}}
	articleSets := [][]uint{} // mismatch

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 1) // Returns original
}

func TestDeduplicateTags_NoDuplicates(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "A", ArticleCount: 2},
		{ID: 2, Label: "B", ArticleCount: 3},
		{ID: 3, Label: "C", ArticleCount: 1},
	}
	articleSets := [][]uint{
		{10, 20},
		{30, 40, 50},
		{60},
	}

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 3)
}

func TestDeduplicateTags_EmptyArticleSet(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Empty", ArticleCount: 0},
		{ID: 2, Label: "Valid", ArticleCount: 2},
	}
	articleSets := [][]uint{
		{},
		{10, 20},
	}

	result := DeduplicateTags(tags, articleSets)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(2), result[0].ID)
}

func tagIDs(tags []TagInput) []uint {
	ids := make([]uint, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	return ids
}
