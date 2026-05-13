package tagging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractTopicsFindsCanonicalAITopicsAndEntities(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "OpenAI pushes GPT-5 agent workflow",
		Summary:      "OpenAI is shipping a new AI agent workflow around GPT-5 with multimodal planning and coding automation.",
		FeedName:     "Latent Space",
		CategoryName: "AI",
	})

	require.GreaterOrEqual(t, len(result), 3)
	require.Contains(t, topicLabels(result), "OpenAI")
	require.Contains(t, topicLabels(result), "AI Agent")
	require.Contains(t, topicLabels(result), "GPT-5")
	require.Contains(t, topicSlugs(result), "openai")
	require.Contains(t, topicSlugs(result), "ai-agent")
	require.Contains(t, topicSlugs(result), "gpt-5")
}

func TestExtractTopicsDeduplicatesAliases(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "OpenAI API update",
		Summary:      "OpenAI says the Open AI API now supports agent memory. OPENAI tooling remains the focus.",
		FeedName:     "OpenAI Blog",
		CategoryName: "AI",
	})

	labels := topicLabels(result)
	require.Equal(t, 1, countMatches(labels, "OpenAI"))
	openAI := findTopic(result, "OpenAI")
	require.NotNil(t, openAI)
	require.Greater(t, openAI.Score, 0.0)
}

func TestExtractTopicsFallsBackToFeedAndCategoryWhenTextIsSparse(t *testing.T) {
	result := ExtractTopics(ExtractionInput{
		Title:        "Daily Brief",
		Summary:      "Short update.",
		FeedName:     "NVIDIA Research",
		CategoryName: "Infra",
	})

	require.Contains(t, topicLabels(result), "NVIDIA")
	require.Contains(t, topicLabels(result), "Infra")
}

func TestParseExtractedTagsAcceptsWrappedTagsObject(t *testing.T) {
	parsed, err := parseExtractedTags(`{"tags":[{"label":"OpenAI","category":"keyword","confidence":0.9,"aliases":["Open AI"]}]}`)

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "OpenAI", parsed[0].Label)
	require.Equal(t, "keyword", parsed[0].Category)
	require.Equal(t, 0.9, parsed[0].Confidence)
	require.Equal(t, []string{"Open AI"}, parsed[0].Aliases)
}

func TestParseExtractedTagsAcceptsSurroundingText(t *testing.T) {
	parsed, err := parseExtractedTags("Here are the extracted tags:\n```json\n[{\"label\":\"AI Agent\",\"category\":\"keyword\",\"confidence\":0.8}]\n```\nThese are the best matches.")

	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "AI Agent", parsed[0].Label)
	require.Equal(t, "keyword", parsed[0].Category)
	require.Equal(t, 0.8, parsed[0].Confidence)
}

func topicLabels(items []TopicTag) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

func topicSlugs(items []TopicTag) []string {
	slugs := make([]string, 0, len(items))
	for _, item := range items {
		slugs = append(slugs, item.Slug)
	}
	return slugs
}

func countMatches(items []string, needle string) int {
	count := 0
	for _, item := range items {
		if item == needle {
			count++
		}
	}
	return count
}

func findTopic(items []TopicTag, label string) *TopicTag {
	for _, item := range items {
		if item.Label == label {
			return &item
		}
	}
	return nil
}

func TestParseExtractedTagsFromRealOllamaResponse(t *testing.T) {
	input := "```json\n[\n  {\n    \"label\": \"李飞飞\",\n    \"category\": \"person\",\n    \"confidence\": 1.0,\n    \"aliases\": [\"AI 教母\"],\n    \"evidence\": \"文中明确提到\"\n  },\n  {\n    \"label\": \"World Labs\",\n    \"category\": \"keyword\",\n    \"confidence\": 1.0,\n    \"aliases\": [\"世界模型团队\"],\n    \"evidence\": \"文中提到\"\n  },\n  {\n    \"label\": \"Spark 2.0\",\n    \"category\": \"keyword\",\n    \"confidence\": 1.0,\n    \"aliases\": [],\n    \"evidence\": \"文中多次提及\"\n  }\n]\n```"
	parsed, err := parseExtractedTags(input)
	require.NoError(t, err)
	require.Len(t, parsed, 3)
	require.Equal(t, "李飞飞", parsed[0].Label)
	require.Equal(t, "person", parsed[0].Category)
	require.Equal(t, "World Labs", parsed[1].Label)
	require.Equal(t, "Spark 2.0", parsed[2].Label)
}

func TestParseExtractedTagsWithUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","confidence":1.0,"aliases":["AI 教母"],"evidence":"文中提到\"李飞飞团队\""},{"label":"World Labs","category":"keyword","confidence":1.0,"aliases":[],"evidence":"文中提到开源"}]`
	parsed, err := parseExtractedTags(input)
	require.NoError(t, err, "valid JSON should parse fine")
	require.Len(t, parsed, 2)
}

func TestBuildExtractionSystemPromptLimitsAndOrdersTags(t *testing.T) {
	prompt := buildExtractionSystemPrompt()

	require.True(t, strings.Contains(prompt, "最多返回 5 个标签") || strings.Contains(prompt, "最多返回5个标签"))
	require.True(t, strings.Contains(prompt, "按优先级从高到低排序") || strings.Contains(prompt, "按优先级排序"))
}

func TestFixBrokenJSONWithUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","confidence":1.0,"aliases":["AI 教母"],"evidence":"文中提到"李飞飞团队""},{"label":"World Labs","category":"keyword","confidence":1.0,"aliases":[],"evidence":"ok"}]`

	parsed, err := parseExtractedTags(input)
	require.NoError(t, err, "broken JSON with unescaped quotes should be auto-repaired")
	require.Len(t, parsed, 2)
	require.Equal(t, "李飞飞", parsed[0].Label)
}
