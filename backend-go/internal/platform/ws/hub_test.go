package ws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagCompletedMessageMarshal(t *testing.T) {
	msg := TagCompletedMessage{
		Type:      "tag_completed",
		ArticleID: 42,
		JobID:     7,
		Tags: []TagCompletedItem{
			{
				Slug:     "ai-agent",
				Label:    "AI Agent",
				Category: "keyword",
				Score:    0.9,
				Icon:     "mdi:robot",
			},
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	require.Equal(t, "tag_completed", body["type"])
	require.Equal(t, float64(42), body["article_id"])
	require.Equal(t, float64(7), body["job_id"])

	tags, ok := body["tags"].([]any)
	require.True(t, ok)
	require.Len(t, tags, 1)

	firstTag, ok := tags[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ai-agent", firstTag["slug"])
	require.Equal(t, "AI Agent", firstTag["label"])
	require.Equal(t, "keyword", firstTag["category"])
	require.Equal(t, 0.9, firstTag["score"])
	require.Equal(t, "mdi:robot", firstTag["icon"])
}
