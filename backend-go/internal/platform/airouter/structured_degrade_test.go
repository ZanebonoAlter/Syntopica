package airouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// scriptedResponse 是 fake server 的一个编排响应：状态码 + 响应体。
type scriptedResponse struct {
	status int
	body   string
}

// scriptServer 记录每次请求的 payload（unmarshal 成 map），按序回放编排响应；
// 编排耗尽后回 500（用例应断言调用次数，走到这里即为测试失败信号）。
type scriptServer struct {
	srv       *httptest.Server
	mu        sync.Mutex
	script    []scriptedResponse
	bodies    []map[string]any
	authHeads []string
	idx       int
}

func newScriptedServer(t *testing.T, script []scriptedResponse) *scriptServer {
	t.Helper()
	s := &scriptServer{script: script}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var payload map[string]any
		if bodyBytes, err := io.ReadAll(req.Body); err == nil {
			_ = json.Unmarshal(bodyBytes, &payload)
		}
		s.bodies = append(s.bodies, payload)
		s.authHeads = append(s.authHeads, req.Header.Get("Authorization"))

		if s.idx >= len(s.script) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"script exhausted"}}`))
			s.idx++
			return
		}
		resp := s.script[s.idx]
		s.idx++
		if resp.status != http.StatusOK {
			w.WriteHeader(resp.status)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *scriptServer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

func (s *scriptServer) bodyAt(i int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[i]
}

func chatOKBody(content string) string {
	raw, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	return string(raw)
}

func bad400Body() string {
	return `{"error":{"message":"json_schema not supported","code":"http_400"}}`
}

// degradeVendor 返回挂到 fake server 的 json_schema 档 vendor 夹具。
func degradeVendor(baseURL string) models.AIVendor {
	return models.AIVendor{Name: "degrade-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: baseURL, Enabled: true, StructuredOutput: StructOutputJSONSchema}
}

// structuredRequest 返回带 schema 的结构化请求夹具。
func structuredRequest() ChatRequest {
	return ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONMode:   true,
		JSONSchema: payloadSchema(),
	}
}

// requireRFType 断言第 i 次请求的 response_format.type。
func requireRFType(t *testing.T, s *scriptServer, i int, want string) {
	t.Helper()
	rf, ok := s.bodyAt(i)["response_format"].(map[string]any)
	require.True(t, ok, "第 %d 次请求应有 response_format，got %v", i, s.bodyAt(i))
	require.Equal(t, want, rf["type"])
}

// TestDegradeChain400ThenObjectSuccess: json_schema 400 → json_object 重试成功（W4）。
// 断言：HTTP 调用次数 2、返回内容、StructuredAttempt 长度与 Stage 序列、
// vendor 档位字段未被修改、请求体逐档形态。
func TestDegradeChain400ThenObjectSuccess(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusOK, body: chatOKBody(`{"a":1}`)},
	})
	vendor := degradeVendor(s.srv.URL)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, resp.Content)
	require.Equal(t, int(2), s.calls())

	require.Len(t, attempts, 2)
	require.Equal(t, StageJSONSchema, attempts[0].Stage)
	require.NotEqual(t, "", attempts[0].ErrorCode)
	require.Contains(t, attempts[0].ErrorCode, "http_400")
	require.Contains(t, attempts[0].ErrorMsg, "json_schema not supported")
	require.Equal(t, "", attempts[0].Content, "失败档 Content 为空")
	require.Equal(t, StageJSONObject, attempts[1].Stage)
	require.Equal(t, "", attempts[1].ErrorCode, "成功档 ErrorCode 为空")
	require.Equal(t, `{"a":1}`, attempts[1].Content)
	require.GreaterOrEqual(t, attempts[0].LatencyMs, 0)

	// 降级链不修改 vendor（档位保持 json_schema）。
	require.Equal(t, StructOutputJSONSchema, vendor.StructuredOutput)

	// 请求体逐档形态：首档 json_schema + strict；次档 json_object + 注入锚句。
	requireRFType(t, s, 0, "json_schema")
	raw0, _ := json.Marshal(s.bodyAt(0))
	require.Contains(t, string(raw0), `"strict":true`)
	requireRFType(t, s, 1, "json_object")
	msgs := s.bodyAt(1)["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	require.Contains(t, last["content"], schemaInjectionAnchor)
}

// TestDegradeChain400x2ThenPlainSuccess: 400,400 → plain 成功（attempts 三档序列）。
func TestDegradeChain400x2ThenPlainSuccess(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusOK, body: chatOKBody(`plain ok`)},
	})
	vendor := degradeVendor(s.srv.URL)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.NoError(t, err)
	require.Equal(t, "plain ok", resp.Content)
	require.Equal(t, int(3), s.calls())

	require.Len(t, attempts, 3)
	require.Equal(t, []string{StageJSONSchema, StageJSONObject, StagePlain},
		[]string{attempts[0].Stage, attempts[1].Stage, attempts[2].Stage})
	require.NotEqual(t, "", attempts[0].ErrorCode)
	require.NotEqual(t, "", attempts[1].ErrorCode)
	require.Equal(t, "", attempts[2].ErrorCode)
	require.Equal(t, "plain ok", attempts[2].Content)

	// plain 档请求体无 response_format 但仍注入。
	require.NotContains(t, s.bodyAt(2), "response_format")
	msgs := s.bodyAt(2)["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	require.Contains(t, last["content"], schemaInjectionAnchor)
}

// TestDegradeChainAll400Fails: 400,400,400 → 三档穷尽，该尝试失败返回最后错误。
func TestDegradeChainAll400Fails(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusBadRequest, body: bad400Body()},
	})
	vendor := degradeVendor(s.srv.URL)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, int(3), s.calls())

	require.Len(t, attempts, 3)
	for i, want := range []string{StageJSONSchema, StageJSONObject, StagePlain} {
		require.Equal(t, want, attempts[i].Stage)
		require.NotEqual(t, "", attempts[i].ErrorCode, "失败档必须带 ErrorCode")
		require.Equal(t, "", attempts[i].Content)
	}
	require.Contains(t, err.Error(), "json_schema not supported")
}

// TestDegradeChainNoDegradeOn429And5xx: 429/5xx 不降级——错误直接上抛，但首档
// 终错仍作为一行 attempt 上抛（红线5：每次尝试必写审计，1 HTTP=1 行）。
func TestDegradeChainNoDegradeOn429And5xx(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantCode string
	}{
		{name: "429 rate limited", status: http.StatusTooManyRequests, wantCode: "http_429"},
		{name: "500 server error", status: http.StatusInternalServerError, wantCode: "http_500"},
		{name: "503 unavailable", status: http.StatusServiceUnavailable, wantCode: "http_503"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, []scriptedResponse{
				{status: tc.status, body: `{"error":{"message":"upstream"},"code":"x"}`},
			})
			vendor := degradeVendor(s.srv.URL)

			resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
			require.Error(t, err)
			require.Nil(t, resp)
			require.Len(t, attempts, 1, "非 4xx 失败不降级，但首档终错仍逐行上抛")
			require.Equal(t, StageJSONSchema, attempts[0].Stage)
			require.Equal(t, tc.wantCode, attempts[0].ErrorCode)
			require.Equal(t, int(1), s.calls(), "不得重试")

			provErr := &ProviderError{}
			require.ErrorAs(t, err, &provErr)
			require.Equal(t, tc.wantCode, provErr.Code)
			require.True(t, provErr.Retryable)
		})
	}
}

// TestDegradeChainAbortOnMidChain429: json_schema 400 后 json_object 遇 429 →
// 整链按普通尝试失败上抛（不降第三档）。
func TestDegradeChainAbortOnMidChain429(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusTooManyRequests, body: `{"error":{"message":"rate limited"}}`},
	})
	vendor := degradeVendor(s.srv.URL)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.Error(t, err)
	require.Nil(t, resp)
	require.Len(t, attempts, 2, "中途 429 中止：已发生的两档各留一行审计")
	require.Equal(t, StageJSONSchema, attempts[0].Stage)
	require.Equal(t, "http_400", attempts[0].ErrorCode)
	require.Equal(t, StageJSONObject, attempts[1].Stage)
	require.Equal(t, "http_429", attempts[1].ErrorCode)
	require.Equal(t, int(2), s.calls())
}

// TestNoChainWithoutSchemaOrWithoutJSONSchemaVendor: 降级链仅当
// JSONMode && JSONSchema!=nil && vendor 档=json_schema 时启用（W4 触发条件）。
func TestNoChainWithoutSchemaOrWithoutJSONSchemaVendor(t *testing.T) {
	t.Run("no schema: single shot even on 400", func(t *testing.T) {
		s := newScriptedServer(t, []scriptedResponse{
			{status: http.StatusBadRequest, body: bad400Body()},
		})
		vendor := degradeVendor(s.srv.URL)
		req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}, JSONMode: true}

		resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", req)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Nil(t, attempts)
		require.Equal(t, int(1), s.calls(), "无 schema 不得进入降级链")
	})

	t.Run("json_object vendor: single shot even on 400", func(t *testing.T) {
		s := newScriptedServer(t, []scriptedResponse{
			{status: http.StatusBadRequest, body: bad400Body()},
		})
		vendor := degradeVendor(s.srv.URL)
		vendor.StructuredOutput = StructOutputJSONObject

		_, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
		require.Error(t, err)
		require.Nil(t, attempts)
		require.Equal(t, int(1), s.calls(), "非 json_schema 档不得进入降级链")
	})

	t.Run("no schema success: single call, no response_format", func(t *testing.T) {
		s := newScriptedServer(t, []scriptedResponse{
			{status: http.StatusOK, body: chatOKBody(`{}`)},
		})
		vendor := degradeVendor(s.srv.URL)
		req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}, JSONMode: true}

		resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Nil(t, attempts)
		require.Equal(t, int(1), s.calls())
		// 无 schema 可带：json_schema 档降按 json_object。
		requireRFType(t, s, 0, "json_object")
	})
}

// TestDegradeChainRestartsFromJSONSchemaEachCall: 降级不持久化档位——第二次调用
// 仍从 json_schema 起步（spec「降级不持久化档位」/ W4 重跑行）。
func TestDegradeChainRestartsFromJSONSchemaEachCall(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusOK, body: chatOKBody(`{"a":1}`)},
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusOK, body: chatOKBody(`{"a":2}`)},
	})
	vendor := degradeVendor(s.srv.URL)
	client := NewOpenAICompatibleClient()

	first, _, err := client.Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, first.Content)

	second, _, err := client.Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.NoError(t, err)
	require.Equal(t, `{"a":2}`, second.Content)
	require.Equal(t, int(4), s.calls())

	// 两次调用的首个请求都从 json_schema 起步。
	requireRFType(t, s, 0, "json_schema")
	requireRFType(t, s, 2, "json_schema")
	// vendor 档位保持不变。
	require.Equal(t, StructOutputJSONSchema, vendor.StructuredOutput)
}

// TestNetworkErrorNoDegrade: 网络错误不降级（连接不到服务端，降档无意义）。
func TestNetworkErrorNoDegrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // 立即关闭制造连接失败
	vendor := degradeVendor(srv.URL)

	resp, attempts, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.Error(t, err)
	require.Nil(t, resp)
	require.Len(t, attempts, 1, "网络错误不降级，但首档终错仍作为一行 attempt 上抛（审计不丢）")
	require.Equal(t, "network_error", attempts[0].ErrorCode)
	provErr := &ProviderError{}
	require.ErrorAs(t, err, &provErr)
	require.Equal(t, "network_error", provErr.Code)
}

// TestDegradeChainEmptyKeyNoAuthHeader: 降级链各档均不发 Authorization（空 key）。
func TestDegradeChainEmptyKeyNoAuthHeader(t *testing.T) {
	s := newScriptedServer(t, []scriptedResponse{
		{status: http.StatusBadRequest, body: bad400Body()},
		{status: http.StatusOK, body: chatOKBody(`{"a":1}`)},
	})
	vendor := degradeVendor(s.srv.URL)
	vendor.APIKey = ""

	_, _, err := NewOpenAICompatibleClient().Chat(context.Background(), vendor, wireModel(), "medium", structuredRequest())
	require.NoError(t, err)
	for i := 0; i < s.calls(); i++ {
		require.Empty(t, s.authHeads[i], "第 %d 次请求不得携带 Authorization", i)
	}
}

// ─── extractBalancedJSON（W5 全部六行 + 字符串内花括号额外用例）──────────────

func TestExtractBalancedJSON(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "pure json unchanged", input: `{"a":1}`, want: `{"a":1}`, wantOK: true},
		{name: "wrapped text extracts balanced block", input: "以下是结果：{\"a\":{\"b\":2}}\n完毕", want: `{"a":{"b":2}}`, wantOK: true},
		{name: "first balanced block wins", input: `前缀 {嵌套{深}层} 尾部 {"second":1}`, want: `{嵌套{深}层}`, wantOK: true},
		{name: "no braces fails", input: `完全没有任何花括号`, want: "", wantOK: false},
		{name: "unbalanced fails", input: `{"a":1`, want: "", wantOK: false},
		{name: "braces inside string ignored", input: `{"a":"}{ {"}`, want: `{"a":"}{ {"}`, wantOK: true},
		{name: "string braces in wrapped text", input: `prefix {"a":"}{ {"} suffix`, want: `{"a":"}{ {"}`, wantOK: true},
		{name: "escaped quote in string", input: `x {"a":"}{"} y`, want: `{"a":"}{"}`, wantOK: true},
		{name: "escaped backslash then quote", input: `{"a":"ends with backslash\\"} trailing`, want: `{"a":"ends with backslash\\"}`, wantOK: true},
		{name: "empty input", input: "", want: "", wantOK: false},
		{name: "whitespace only", input: "   \n\t ", want: "", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractBalancedJSON(tc.input)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCorrectStructuredJSON: Router 侧纠错入口——原文合法原样；包裹文本提取后再校验；
// 提取块非 JSON（如 {嵌套{深}层}）判失败。
func TestCorrectStructuredJSON(t *testing.T) {
	t.Run("valid raw returned as-is", func(t *testing.T) {
		got, ok := correctStructuredJSON(`{"a":1}`)
		require.True(t, ok)
		require.Equal(t, `{"a":1}`, got)
	})
	t.Run("wrapped valid extracted", func(t *testing.T) {
		got, ok := correctStructuredJSON("以下是结果：{\"a\":1}\n完毕")
		require.True(t, ok)
		require.Equal(t, `{"a":1}`, got)
	})
	t.Run("non-json block fails", func(t *testing.T) {
		_, ok := correctStructuredJSON(`前缀 {嵌套{深}层} 尾部`)
		require.False(t, ok, "提取块非合法 JSON 必须判失败")
	})
	t.Run("no json fails", func(t *testing.T) {
		_, ok := correctStructuredJSON("完全没有任何花括号")
		require.False(t, ok)
	})
}
