package daily_report

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterTags_Empty(t *testing.T) {
	groups, err := ClusterTags(nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, groups)
}

func TestClusterTags_SingleTag(t *testing.T) {
	tags := []TagInput{{ID: 1, Label: "Test Event", ArticleCount: 3}}
	groups, err := ClusterTags(nil, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, "Test Event", groups[0].GroupName)
	assert.Equal(t, []uint{1}, groups[0].TagIDs)
}

func TestClusterTags_TwoTags(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Event A", ArticleCount: 2},
		{ID: 2, Label: "Event B", ArticleCount: 3},
	}
	groups, err := ClusterTags(nil, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 2)
}

func TestParseClusterResponse_Valid(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "G7 Summit"},
		{ID: 2, Label: "G7 Statement"},
		{ID: 3, Label: "Fed Rate Hike"},
	}
	content := `{"groups":[{"group_name":"G7峰会","tag_ids":[1,2]},{"group_name":"美联储加息","tag_ids":[3]}]}`
	groups, err := parseClusterResponse(content, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 2)
	assert.Equal(t, "G7峰会", groups[0].GroupName)
	assert.Equal(t, []uint{1, 2}, groups[0].TagIDs)
}

func TestParseClusterResponse_UnassignedTags(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Event A"},
		{ID: 2, Label: "Event B"},
		{ID: 3, Label: "Event C"},
	}
	// Only ID 1 assigned in response; 2 and 3 should get their own groups.
	content := `{"groups":[{"group_name":"Group A","tag_ids":[1]}]}`
	groups, err := parseClusterResponse(content, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 3)
}

func TestParseClusterResponse_UnknownIDs(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Event A"},
	}
	content := `{"groups":[{"group_name":"Group","tag_ids":[1, 999]}]}`
	groups, err := parseClusterResponse(content, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, []uint{1}, groups[0].TagIDs)
}

func TestParseClusterResponse_EmptyGroupName(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Event A"},
	}
	content := `{"groups":[{"group_name":"","tag_ids":[1]}]}`
	groups, err := parseClusterResponse(content, tags)
	assert.NoError(t, err)
	// Empty group name is skipped, so the tag gets its own fallback group.
	assert.Len(t, groups, 1)
	assert.Equal(t, "Event A", groups[0].GroupName)
}

func TestParseClusterResponse_DuplicateTagIDs(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Event A"},
		{ID: 2, Label: "Event B"},
	}
	content := `{"groups":[{"group_name":"G1","tag_ids":[1,2]},{"group_name":"G2","tag_ids":[1]}]}`
	groups, err := parseClusterResponse(content, tags)
	assert.NoError(t, err)
	// Tag 1 appears in G1 first, so G2 should be empty and skipped.
	// Tag 2 is only in G1.
	assert.Len(t, groups, 1)
	assert.Equal(t, "G1", groups[0].GroupName)
}

func TestParseClusterResponse_InvalidJSON(t *testing.T) {
	tags := []TagInput{{ID: 1, Label: "A"}}
	content := `not json`
	_, err := parseClusterResponse(content, tags)
	assert.Error(t, err)
}

func TestBuildClusterPrompt(t *testing.T) {
	tags := []TagInput{
		{ID: 1, Label: "Test", ArticleCount: 5, Description: "A test event"},
	}
	prompt := buildClusterPrompt(tags)
	assert.Contains(t, prompt, "[ID:1]")
	assert.Contains(t, prompt, "Test")
	assert.Contains(t, prompt, "A test event")
}

func TestClusterTags_ManyTagsSkipLLM(t *testing.T) {
	// With >2 tags, the real LLM would be called. This test just verifies
	// the logic path — we can't easily test the LLM path without a mock.
	// Instead we test parseClusterResponse directly.
	input := `{"groups":[{"group_name":"Tech","tag_ids":[1,2,3]},{"group_name":"Politics","tag_ids":[4,5]}]}`
	tags := []TagInput{
		{ID: 1, Label: "Apple"},
		{ID: 2, Label: "Google"},
		{ID: 3, Label: "Microsoft"},
		{ID: 4, Label: "Election"},
		{ID: 5, Label: "Congress"},
	}
	groups, err := parseClusterResponse(input, tags)
	assert.NoError(t, err)
	assert.Len(t, groups, 2)

	// Verify the JSON round-trip works
	data, err := json.Marshal(groups)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "Tech")
}
