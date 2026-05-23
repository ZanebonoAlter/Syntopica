package jsonutil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeLLMJSON_StripsMarkdownFence(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	result := SanitizeLLMJSON(input)
	require.Equal(t, `{"key": "value"}`, result)
}

func TestSanitizeLLMJSON_ExtractsJSONArray(t *testing.T) {
	input := "Here are tags:\n[{\"label\":\"AI\"}]"
	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)))

	var parsed []struct {
		Label string `json:"label"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Len(t, parsed, 1)
}

func TestSanitizeLLMJSON_FixesUnescapedQuotes(t *testing.T) {
	input := `[{"label":"李飞飞","category":"person","confidence":1.0,"aliases":["AI 教母"],"description":"文中提到"李飞飞团队""},{"label":"World Labs","category":"keyword","confidence":1.0,"aliases":[],"description":"ok"}]`

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed []struct {
		Label       string   `json:"label"`
		Description string   `json:"description"`
		Aliases     []string `json:"aliases"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Len(t, parsed, 2)
	require.Equal(t, "李飞飞", parsed[0].Label)
	require.Equal(t, `文中提到"李飞飞团队"`, parsed[0].Description)
	require.Equal(t, []string{"AI 教母"}, parsed[0].Aliases)
}

func TestSanitizeLLMJSON_FixesMultipleUnescapedQuotesInSameString(t *testing.T) {
	input := `[{"label":"AI","description":"文中提到"李飞飞"和"黄仁勋""}]`

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed []struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Equal(t, `文中提到"李飞飞"和"黄仁勋"`, parsed[0].Description)
}

func TestSanitizeLLMJSON_FixesQuotesInArrayValues(t *testing.T) {
	input := `[{"label":"AI","aliases":["AI "教母"","人工智能"]}]`

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed []struct {
		Aliases []string `json:"aliases"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Equal(t, []string{`AI "教母"`, "人工智能"}, parsed[0].Aliases)
}

func TestSanitizeLLMJSON_FixesQuotesInObjectValues(t *testing.T) {
	input := `{"decision":"reuse","reason":"标签"OpenAI"与现有标签匹配"}`

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Equal(t, "reuse", parsed.Decision)
	require.Equal(t, `标签"OpenAI"与现有标签匹配`, parsed.Reason)
}

func TestSanitizeLLMJSON_FixesTruncatedArray(t *testing.T) {
	input := `[{"label":"AI"},{"label":"ML"`

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed []struct {
		Label string `json:"label"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Len(t, parsed, 1)
	require.Equal(t, "AI", parsed[0].Label)
}

func TestSanitizeLLMJSON_PreservesValidJSON(t *testing.T) {
	input := `[{"label":"OpenAI","category":"keyword","confidence":0.9}]`

	result := SanitizeLLMJSON(input)
	require.Equal(t, input, result)
}

func TestSanitizeLLMJSON_HandlesAlreadyEscapedQuotes(t *testing.T) {
	input := `[{"label":"AI","description":"文中提到\"李飞飞\"团队"}]`

	result := SanitizeLLMJSON(input)
	require.Equal(t, input, result)

	var parsed []struct {
		Description string `json:"description"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Equal(t, `文中提到"李飞飞"团队`, parsed[0].Description)
}

func TestSanitizeLLMJSON_RealOllamaResponse(t *testing.T) {
	input := "```json\n[\n  {\n    \"label\": \"李飞飞\",\n    \"category\": \"person\",\n    \"confidence\": 1.0,\n    \"aliases\": [\"AI 教母\"],\n    \"description\": \"文中明确提到\"\n  },\n  {\n    \"label\": \"World Labs\",\n    \"category\": \"keyword\",\n    \"confidence\": 1.0,\n    \"aliases\": [\"世界模型团队\"],\n    \"description\": \"文中提到\"\n  }\n]\n```"

	result := SanitizeLLMJSON(input)
	require.True(t, json.Valid([]byte(result)), "result should be valid JSON: %s", result)

	var parsed []struct {
		Label       string   `json:"label"`
		Description string   `json:"description"`
		Aliases     []string `json:"aliases"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))
	require.Len(t, parsed, 2)
}
