package airouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// newWireServer spins an httptest server returning body for any request, so
// client-level wire parsing can be tested against real HTTP responses (the
// fake client bypasses parsing entirely — see change test-cases.md 盲区备忘).
func newWireServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wireVendor(baseURL string) models.AIVendor {
	return models.AIVendor{Name: "wire", ProviderType: ProviderTypeOpenAICompatible, BaseURL: baseURL, Enabled: true}
}

func wireModel() models.AIModel {
	return models.AIModel{Name: "m", ModelKind: ModelKindLLM, Enabled: true}
}

func TestChatParsesStandardUsageKeys(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":"hi there"}}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), wireVendor(srv.URL), wireModel(), "medium", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Nil(t, attempts, "非降级链路径无 StructuredAttempt 列表")
	require.NotNil(t, resp.Usage)
	require.Equal(t, 9, resp.Usage.PromptTokens)
	require.Equal(t, 12, resp.Usage.CompletionTokens)
	require.Equal(t, 21, resp.Usage.TotalTokens)
}

func TestChatWithoutUsageBlockYieldsNilUsage(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":"x"}}]}`)

	resp, _, err := NewOpenAICompatibleClient().Chat(context.Background(), wireVendor(srv.URL), wireModel(), "medium", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Nil(t, resp.Usage)
}

func TestChat4xxWithoutChainReturnsProviderError(t *testing.T) {
	srv := newWireServer(t, `{"error":{"message":"bad request","code":"http_400"}}`)
	// vendor 档位 json_object（非 json_schema）→ 无降级链，400 直接按尝试失败返回。
	vendor := wireVendor(srv.URL)
	vendor.StructuredOutput = StructOutputJSONObject

	_, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hello"}},
		JSONMode:   true,
		JSONSchema: &JSONSchema{Type: "object"},
	})
	require.Error(t, err)
	require.Nil(t, attempts)
	provErr := &ProviderError{}
	require.ErrorAs(t, err, &provErr)
	require.Equal(t, "http_400", provErr.Code)
	require.False(t, provErr.Retryable)
}

func TestEmbedParsesUsage(t *testing.T) {
	srv := newWireServer(t, `{"data":[{"embedding":[0.1,0.2],"index":0}],"model":"e","usage":{"prompt_tokens":7,"total_tokens":7}}`)

	res, err := NewOpenAICompatibleClient().Embed(context.Background(), wireVendor(srv.URL), wireModel(), EmbeddingRequest{
		Input: []string{"hello"},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Usage)
	require.Equal(t, 7, res.Usage.PromptTokens)
	require.Equal(t, 7, res.Usage.TotalTokens)
}

// TestChatWithoutAPIKeySendsNoAuthorizationHeader: API key 为空的 vendor 请求
// MUST NOT 携带 Authorization 头（W8-1：本地服务无需 key）。
func TestChatWithoutAPIKeySendsNoAuthorizationHeader(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Values("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)

	vendor := wireVendor(srv.URL)
	vendor.APIKey = ""
	_, _, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Empty(t, gotAuth, "空 key 不得发 Authorization 头")

	// 对照：非空 key 发 Bearer 头。
	vendor.APIKey = "secret"
	_, _, err = NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer secret"}, gotAuth)
}

// TestTokenUsageUnmarshalJSONKeyFamilies pins the dual-key parsing contract:
// standard wire keys, legacy stored keys (defensive), missing keys, and
// standard-wins-on-conflict (change test-cases.md §4 分支表).
func TestTokenUsageUnmarshalJSONKeyFamilies(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want TokenUsage
	}{
		{"standard wire keys", `{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}`, TokenUsage{PromptTokens: 9, CompletionTokens: 12, TotalTokens: 21}},
		{"legacy stored keys", `{"prompt":9,"completion":12,"total":21}`, TokenUsage{PromptTokens: 9, CompletionTokens: 12, TotalTokens: 21}},
		{"missing keys stays zero", `{}`, TokenUsage{}},
		{"standard wins on conflict", `{"prompt":1,"completion":1,"total":1,"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}`, TokenUsage{PromptTokens: 9, CompletionTokens: 12, TotalTokens: 21}},
		// reasoning 细分（fix-deepseek-thinking-empty-response）：wire 键
		// completion_tokens_details.reasoning_tokens / legacy 平键 reasoning 各自可解析。
		{"reasoning detail wire key", `{"prompt_tokens":100,"completion_tokens":1536,"total_tokens":1636,"completion_tokens_details":{"reasoning_tokens":1536}}`, TokenUsage{PromptTokens: 100, CompletionTokens: 1536, TotalTokens: 1636, ReasoningTokens: 1536}},
		{"reasoning legacy flat key", `{"prompt":1,"completion":2,"total":3,"reasoning":2}`, TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, ReasoningTokens: 2}},
		{"reasoning detail wins over flat key", `{"prompt":1,"completion":5,"total":6,"reasoning":1,"completion_tokens_details":{"reasoning_tokens":4}}`, TokenUsage{PromptTokens: 1, CompletionTokens: 5, TotalTokens: 6, ReasoningTokens: 4}},
		{"empty details object keeps zero", `{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"completion_tokens_details":{}}`, TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var u TokenUsage
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &u))
			require.Equal(t, tc.want, u)
		})
	}

	var u TokenUsage
	require.Error(t, json.Unmarshal([]byte("not-json"), &u))
}

// TestTokenUsageMarshalReasoningOptionalKey pins the stored jsonb shape
// (fix-deepseek-thinking-empty-response)：基础三键永不变，reasoning 仅在 >0
// 时作为第四键出现——session 聚合等消费方按三键读取，附加键不破坏兼容。
func TestTokenUsageMarshalReasoningOptionalKey(t *testing.T) {
	b, err := json.Marshal(&TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30})
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt":10,"completion":20,"total":30}`, string(b), "无 reasoning 细分时必须是基础三键形状")

	b, err = json.Marshal(&TokenUsage{PromptTokens: 100, CompletionTokens: 1536, TotalTokens: 1636, ReasoningTokens: 1536})
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt":100,"completion":1536,"total":1636,"reasoning":1536}`, string(b), "带 reasoning 细分时为四键，且三键同形")
}

// TestChatParsesFinishReasonOnEmptyContent：DeepSeek 空 content 事故的真实响应
// 形状（fix-deepseek-thinking-empty-response）——HTTP 200 + content 空 +
// finish_reason=length + usage 全为 reasoning。client 层不判空（归一化在
// Router），但必须把 usage/finish_reason 解析上行，否则 Router 无法归因。
func TestChatParsesFinishReasonOnEmptyContent(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":100,"completion_tokens":1536,"total_tokens":1636,"completion_tokens_details":{"reasoning_tokens":1536}}}`)
	resp, _, err := NewOpenAICompatibleClient().Chat(context.Background(), wireVendor(srv.URL), wireModel(), "low", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err, "client 层空 content 不是错误（归一化在 Router）")
	require.Empty(t, resp.Content)
	require.Equal(t, "length", resp.FinishReason)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 1536, resp.Usage.ReasoningTokens)
	require.Equal(t, 1636, resp.Usage.TotalTokens)
}

// TestChatFinishReasonAbsentStaysEmpty：无 finish_reason 的正常响应（本地 qwen 形状）
// 上行为空串，不产生伪归因。
func TestChatFinishReasonAbsentStaysEmpty(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`)
	resp, _, err := NewOpenAICompatibleClient().Chat(context.Background(), wireVendor(srv.URL), wireModel(), "off", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.Empty(t, resp.FinishReason)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 0, resp.Usage.ReasoningTokens)
}

func TestStripThinkTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no think tags",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "simple think tags",
			input: "<think\n>\nLet me think about this\n</think\n>\nThe answer is 42",
			want:  "The answer is 42",
		},
		{
			name:  "think tags with multiline content",
			input: "<think\n>\nStep 1: analyze\nStep 2: reason\nStep 3: conclude\n</think\n>\nFinal answer",
			want:  "Final answer",
		},
		{
			name:  "only think content no trailing text",
			input: "<think\n>\nJust thinking\n</think\n>",
			want:  "",
		},
		{
			name:  "think tags with extra whitespace in tags",
			input: "<think  >\nreasoning\n</think  >\nresult",
			want:  "result",
		},
		{
			name:  "multiple think blocks",
			input: "<think\n>first</think\n>middle<think\n>second</think\n>end",
			want:  "middleend",
		},
		{
			name:  "empty think tags",
			input: "<think\n></think\n>content",
			want:  "content",
		},
		{
			name:  "think with code blocks inside",
			input: "<think\n>\n```go\nfmt.Println(\"hi\")\n```\n</think\n>\nHere is the answer",
			want:  "Here is the answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkTags(tt.input)
			if got != tt.want {
				t.Errorf("stripThinkTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestBuildPayload_ThinkingWireEffect: buildPayload 把 thinkingToWire 的产物 merge
// 进 payload（qwen 方言全档显式发 chat_template_kwargs.enable_thinking——off→false，
// 不发 ≠ off，沿用既有语义；none 方言不发任何思考字段）。enable_thinking 布尔直挂
// 的旧断言已随 vendor 化删除，档位语义由 thinking.go（W1 矩阵）承担。
func TestBuildPayload_ThinkingWireEffect(t *testing.T) {
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}

	t.Run("qwen dialect off sends explicit false", func(t *testing.T) {
		vendor := models.AIVendor{ThinkingDialect: "qwen-chat-template"}
		model := models.AIModel{Name: "qwythos"}
		payload := buildPayload(vendor, model, ThinkingOff, req, "")
		kwargs, ok := payload["chat_template_kwargs"].(map[string]any)
		require.True(t, ok, "expected chat_template_kwargs map, got %T", payload["chat_template_kwargs"])
		require.Equal(t, false, kwargs["enable_thinking"], "off 档必须显式发 false（缺省会被模板当 ON）")
	})

	t.Run("qwen dialect non-off sends true with clamp", func(t *testing.T) {
		vendor := models.AIVendor{ThinkingDialect: "qwen-chat-template"}
		// xhigh 未声明支持 → 钳 high → true（钳制发生在 wire 之前）。
		payload := buildPayload(vendor, models.AIModel{Name: "qwythos"}, ThinkingXhigh, req, "")
		kwargs := payload["chat_template_kwargs"].(map[string]any)
		require.Equal(t, true, kwargs["enable_thinking"])
	})

	t.Run("none dialect sends no thinking params", func(t *testing.T) {
		vendor := models.AIVendor{ThinkingDialect: "none"}
		payload := buildPayload(vendor, models.AIModel{Name: "m"}, ThinkingMedium, req, "")
		require.NotContains(t, payload, "chat_template_kwargs")
		require.NotContains(t, payload, "reasoning_effort")
		require.NotContains(t, payload, "thinking")
	})
}

// TestBuildPayload_BaseFields: 既有行为不回归——model 取 model.Name，temperature/max_tokens
// 请求级优先、缺省 0.3/16384（vendor 两层化后不再有 provider 级默认覆盖）。
func TestBuildPayload_BaseFields(t *testing.T) {
	vendor := models.AIVendor{}
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}

	payload := buildPayload(vendor, models.AIModel{Name: "m"}, "medium", req, "")
	require.Equal(t, "m", payload["model"])
	require.Equal(t, 0.3, payload["temperature"])
	require.Equal(t, 16384, payload["max_tokens"])

	temp, maxTok := 0.7, 1000
	payload = buildPayload(vendor, models.AIModel{Name: "m"}, "medium", ChatRequest{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	}, "")
	require.Equal(t, 0.7, payload["temperature"])
	require.Equal(t, 1000, payload["max_tokens"])
}

// TestEmbedUsesModelNameInRequest: Embed 请求体 model 取绑定模型名（req.Model 空时回退）。
func TestEmbedUsesModelNameInRequest(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1],"index":0}],"model":"qwen3-embedding"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := NewOpenAICompatibleClient().Embed(context.Background(), wireVendor(srv.URL), models.AIModel{Name: "qwen3-embedding"}, EmbeddingRequest{
		Input: []string{"hello"},
	})
	require.NoError(t, err)
	require.Equal(t, "qwen3-embedding", gotBody["model"])
}
