package airouter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// payloadSchema 是三档断言共用的 JSON schema 夹具。
func payloadSchema() *JSONSchema {
	return &JSONSchema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"title": {Type: "string", Description: "标题"},
		},
		Required: []string{"title"},
	}
}

// compactSchemaJSON 是 schema 的 compact 序列化（注入文本锚句后半段）。
func compactSchemaJSON(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(payloadSchema())
	require.NoError(t, err)
	return string(b)
}

// payloadMessages 取回 payload 里的 messages 切片。
func payloadMessages(t *testing.T, payload map[string]any) []Message {
	t.Helper()
	msgs, ok := payload["messages"].([]Message)
	require.True(t, ok, "messages 应为 []Message，got %T", payload["messages"])
	return msgs
}

// TestBuildPayload_StructuredThreeStates 覆盖 W3 全部行：三档 ×
// (openai_compatible/ollama) × (有 schema/无 schema)，含 ollama 特例与注入锚句。
func TestBuildPayload_StructuredThreeStates(t *testing.T) {
	schema := payloadSchema()
	anchor := schemaInjectionAnchor

	cases := []struct {
		name          string
		providerType  string
		vendorStage   string // vendor.StructuredOutput
		explicitStage string // structuredStage 参数（空=按 vendor 档）
		schemaPresent bool
		jsonModeOff   bool   // true=非 JSONMode 请求
		wantFormatKey string // "" = 不得出现任何 response_format/format 键
		wantInject    bool
	}{
		{
			name:         "openai json_schema stage carries full schema, no inject",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONSchema,
			schemaPresent: true, wantFormatKey: "response_format", wantInject: false,
		},
		{
			name:         "openai json_object stage injects schema",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONObject,
			schemaPresent: true, wantFormatKey: "response_format", wantInject: true,
		},
		{
			name:         "openai none stage plain text + inject",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputNone,
			schemaPresent: true, wantFormatKey: "", wantInject: true,
		},
		{
			name:         "ollama json_schema stage format=schema",
			providerType: ProviderTypeOllama, vendorStage: StructOutputJSONSchema,
			schemaPresent: true, wantFormatKey: "format", wantInject: false,
		},
		{
			name:         "ollama json_object stage format=json + inject",
			providerType: ProviderTypeOllama, vendorStage: StructOutputJSONObject,
			schemaPresent: true, wantFormatKey: "format", wantInject: true,
		},
		{
			name:         "ollama none stage no format + inject",
			providerType: ProviderTypeOllama, vendorStage: StructOutputNone,
			schemaPresent: true, wantFormatKey: "", wantInject: true,
		},
		{
			name:         "explicit plain stage overrides json_schema vendor",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONSchema,
			explicitStage: StagePlain, schemaPresent: true, wantFormatKey: "", wantInject: true,
		},
		{
			name:         "explicit json_schema stage overrides json_object vendor",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONObject,
			explicitStage: StageJSONSchema, schemaPresent: true, wantFormatKey: "response_format", wantInject: false,
		},
		{
			name:         "json_schema stage without schema degrades to json_object",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONSchema,
			schemaPresent: false, wantFormatKey: "response_format", wantInject: false,
		},
		{
			name:         "json_object stage without schema no inject",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONObject,
			schemaPresent: false, wantFormatKey: "response_format", wantInject: false,
		},
		{
			name:         "none stage without schema nothing",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputNone,
			schemaPresent: false, wantFormatKey: "", wantInject: false,
		},
		{
			name:         "non JSONMode request unchanged",
			providerType: ProviderTypeOpenAICompatible, vendorStage: StructOutputJSONSchema,
			schemaPresent: true, wantFormatKey: "", wantInject: false, jsonModeOff: true,
		},
		{
			name:         "unset vendor stage treated as none",
			providerType: ProviderTypeOpenAICompatible, vendorStage: "",
			schemaPresent: true, wantFormatKey: "", wantInject: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vendor := models.AIVendor{ProviderType: tc.providerType, StructuredOutput: tc.vendorStage}
			model := models.AIModel{Name: "m"}
			req := ChatRequest{
				Messages:   []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}},
				JSONMode:   !tc.jsonModeOff,
				JSONSchema: nil,
			}
			if tc.schemaPresent {
				req.JSONSchema = schema
			}

			payload := buildPayload(vendor, model, "medium", req, tc.explicitStage)

			// response_format / format 键按档位出现或缺席。
			switch tc.wantFormatKey {
			case "":
				require.NotContains(t, payload, "response_format")
				require.NotContains(t, payload, "format")
			case "response_format":
				rf, ok := payload["response_format"].(map[string]any)
				require.True(t, ok, "response_format 应为 map，got %T", payload["response_format"])
				if tc.explicitStage == StageJSONSchema || (tc.explicitStage == "" && tc.vendorStage == StructOutputJSONSchema) {
					if req.JSONSchema != nil {
						// json_schema 档：完整结构 {type, json_schema:{name,strict,schema}}。
						require.Equal(t, "json_schema", rf["type"])
						js := rf["json_schema"].(map[string]any)
						require.Equal(t, "response", js["name"])
						require.Equal(t, true, js["strict"])
						require.Equal(t, schema, js["schema"])
					} else {
						// 无 schema 可带：json_schema 档降按 json_object 处理。
						require.Equal(t, "json_object", rf["type"])
					}
				} else {
					require.Equal(t, "json_object", rf["type"])
					// json_object 档 payload 不含 json_schema 结构。
					raw, err := json.Marshal(payload)
					require.NoError(t, err)
					require.NotContains(t, string(raw), "json_schema")
				}
			case "format":
				if tc.vendorStage == StructOutputJSONSchema && req.JSONSchema != nil {
					require.Equal(t, schema, payload["format"], "ollama json_schema 档 format=<schema 对象>")
				} else {
					require.Equal(t, "json", payload["format"], "ollama json_object 档 format=\"json\"")
				}
			}

			// schema 注入：最后一条 user message 末尾追加锚句 + compact JSON。
			msgs := payloadMessages(t, payload)
			require.Len(t, msgs, 2)
			if tc.wantInject {
				want := "hi" + anchor + compactSchemaJSON(t)
				require.Equal(t, want, msgs[1].Content)
				require.Contains(t, msgs[1].Content, "你必须只输出符合以下 JSON 结构的 JSON 文本，不要包含任何其他文字、解释或代码块围栏：")
			} else {
				require.Equal(t, "hi", msgs[1].Content)
			}
		})
	}
}

// TestBuildPayload_InjectionClonesMessages: 注入不修改原 req.Messages（clone 后追加）。
func TestBuildPayload_InjectionClonesMessages(t *testing.T) {
	vendor := models.AIVendor{StructuredOutput: StructOutputJSONObject}
	model := models.AIModel{Name: "m"}
	req := ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONMode:   true,
		JSONSchema: payloadSchema(),
	}
	original := req.Messages

	payload := buildPayload(vendor, model, "medium", req, "")
	msgs := payloadMessages(t, payload)
	require.Len(t, msgs, 1)
	require.Contains(t, msgs[0].Content, schemaInjectionAnchor)
	// 原 messages 不被修改。
	require.Equal(t, "hi", original[0].Content)
	// payload 内的注入副本确实生效（与原内容不同）。
	require.NotEqual(t, original[0].Content, msgs[0].Content)
}

// TestBuildPayload_NonStructuredRequestUnchanged: 非 JSONMode 请求 payload 无任何
// 结构化痕迹（既有行为不回归）。
func TestBuildPayload_NonStructuredRequestUnchanged(t *testing.T) {
	vendor := models.AIVendor{ProviderType: ProviderTypeOllama, StructuredOutput: StructOutputJSONSchema}
	payload := buildPayload(vendor, models.AIModel{Name: "m"}, "medium", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, "")
	require.NotContains(t, payload, "response_format")
	require.NotContains(t, payload, "format")
	msgs := payloadMessages(t, payload)
	require.Equal(t, "hi", msgs[0].Content)
}
