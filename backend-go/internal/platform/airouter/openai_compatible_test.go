package airouter

import (
	"context"
	"encoding/json"
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

func wireProvider(baseURL string) models.AIProvider {
	return models.AIProvider{Name: "wire", ProviderType: ProviderTypeOpenAICompatible, BaseURL: baseURL, Model: "m", Enabled: true}
}

func TestChatParsesStandardUsageKeys(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":"hi there"}}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`)

	resp, err := NewOpenAICompatibleClient().Chat(context.Background(), wireProvider(srv.URL), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 9, resp.Usage.PromptTokens)
	require.Equal(t, 12, resp.Usage.CompletionTokens)
	require.Equal(t, 21, resp.Usage.TotalTokens)
}

func TestChatWithoutUsageBlockYieldsNilUsage(t *testing.T) {
	srv := newWireServer(t, `{"choices":[{"message":{"content":"x"}}]}`)

	resp, err := NewOpenAICompatibleClient().Chat(context.Background(), wireProvider(srv.URL), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Nil(t, resp.Usage)
}

func TestEmbedParsesUsage(t *testing.T) {
	srv := newWireServer(t, `{"data":[{"embedding":[0.1,0.2],"index":0}],"model":"e","usage":{"prompt_tokens":7,"total_tokens":7}}`)

	res, err := NewOpenAICompatibleClient().Embed(context.Background(), wireProvider(srv.URL), EmbeddingRequest{
		Input: []string{"hello"},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Usage)
	require.Equal(t, 7, res.Usage.PromptTokens)
	require.Equal(t, 7, res.Usage.TotalTokens)
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

func TestBuildPayload_EnableThinking(t *testing.T) {
	// chat_template_kwargs.enable_thinking is sent for BOTH true and false.
	// Rationale: the Qwen3/Qwythos chat template defaults to thinking ON when the
	// kwarg is absent (it only inserts an empty <think></think> to suppress reasoning
	// when enable_thinking=false is sent explicitly). So omitting the kwarg does NOT
	// mean "off" — it means "default", which for this model family is ON. We must
	// always send the kwarg explicitly so the per-request toggle actually takes effect.
	t.Run("enable_thinking true propagates chat_template_kwargs", func(t *testing.T) {
		provider := models.AIProvider{Model: "qwythos", EnableThinking: true}
		req := ChatRequest{
			Messages: []Message{{Role: "user", Content: "hi"}},
		}
		payload := buildPayload(provider, req)
		kwargs, ok := payload["chat_template_kwargs"].(map[string]any)
		if !ok {
			t.Fatalf("expected chat_template_kwargs map, got %T", payload["chat_template_kwargs"])
		}
		if kwargs["enable_thinking"] != true {
			t.Fatalf("expected enable_thinking=true, got %v", kwargs["enable_thinking"])
		}
	})

	// EnableThinking=false → payload MUST still contain chat_template_kwargs.enable_thinking=false.
	// (Previously this asserted the field was omitted — that was wrong: omitting lets the
	// model fall back to its default, which is thinking ON for Qwen3. Sending false explicitly
	// is the only way to actually suppress reasoning.)
	t.Run("enable_thinking false sends chat_template_kwargs=false", func(t *testing.T) {
		provider := models.AIProvider{Model: "qwythos", EnableThinking: false}
		req := ChatRequest{
			Messages: []Message{{Role: "user", Content: "hi"}},
		}
		payload := buildPayload(provider, req)
		kwargs, ok := payload["chat_template_kwargs"].(map[string]any)
		if !ok {
			t.Fatalf("expected chat_template_kwargs map even when EnableThinking=false, got %T", payload["chat_template_kwargs"])
		}
		if kwargs["enable_thinking"] != false {
			t.Fatalf("expected enable_thinking=false, got %v", kwargs["enable_thinking"])
		}
	})

	// 既有行为不回归：model/messages/temperature/max_tokens 仍在
	t.Run("preserves base payload fields", func(t *testing.T) {
		temp := 0.3
		maxTok := 2000
		provider := models.AIProvider{Model: "qwythos"}
		req := ChatRequest{
			Messages:    []Message{{Role: "user", Content: "hi"}},
			Temperature: &temp,
			MaxTokens:   &maxTok,
		}
		payload := buildPayload(provider, req)
		if payload["model"] != "qwythos" {
			t.Fatalf("expected model=qwythos, got %v", payload["model"])
		}
		if payload["temperature"] != 0.3 {
			t.Fatalf("expected temperature=0.3, got %v", payload["temperature"])
		}
		if payload["max_tokens"] != 2000 {
			t.Fatalf("expected max_tokens=2000, got %v", payload["max_tokens"])
		}
	})
}

// TestBuildPayload_TemperatureMaxTokensPrecedence guards the request→provider→default
// fallback chain for temperature/max_tokens — the most regression-prone part of buildPayload.
func TestBuildPayload_TemperatureMaxTokensPrecedence(t *testing.T) {
	reqTemp, reqMax := 0.7, 1000
	provTemp, provMax := 0.5, 500

	tests := []struct {
		name            string
		reqTemperature  *float64
		reqMaxTokens    *int
		provTemperature *float64
		provMaxTokens   *int
		wantTemp        float64
		wantMax         int
	}{
		{"both nil uses defaults", nil, nil, nil, nil, 0.3, 16384},
		{"req wins over provider", &reqTemp, &reqMax, &provTemp, &provMax, 0.7, 1000},
		{"provider used when req nil", nil, nil, &provTemp, &provMax, 0.5, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := models.AIProvider{Model: "qwythos", Temperature: tt.provTemperature, MaxTokens: tt.provMaxTokens}
			req := ChatRequest{
				Messages:    []Message{{Role: "user", Content: "hi"}},
				Temperature: tt.reqTemperature,
				MaxTokens:   tt.reqMaxTokens,
			}
			payload := buildPayload(provider, req)
			if payload["temperature"] != tt.wantTemp {
				t.Errorf("temperature = %v, want %v", payload["temperature"], tt.wantTemp)
			}
			if payload["max_tokens"] != tt.wantMax {
				t.Errorf("max_tokens = %v, want %v", payload["max_tokens"], tt.wantMax)
			}
		})
	}
}
