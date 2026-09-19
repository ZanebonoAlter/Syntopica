package airouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/tracing"
	// "syntopica-backend/internal/platform/logging"
)

var thinkTagRe = regexp.MustCompile(`(?s)<think\s*>.*?</think\s*>`) //nolint:unused // retained defensively; see design.md Risks/Trade-offs

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type JSONSchema struct {
	Type       string                    `json:"type"`
	Items      *JSONSchema               `json:"items,omitempty"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

type SchemaProperty struct {
	Type        string                    `json:"type,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
	Items       *SchemaProperty           `json:"items,omitempty"`
	Properties  map[string]SchemaProperty `json:"properties,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Description string                    `json:"description,omitempty"`
}

type ChatRequest struct {
	Capability  Capability
	Operation   string // 必填，业务操作名（如 daily_report.cluster_tags）
	SessionID   string // 可选，编排分组键
	Messages    []Message
	Temperature *float64
	MaxTokens   *int
	Metadata    map[string]any
	JSONMode    bool
	JSONSchema  *JSONSchema
}

type ChatResult struct {
	Content      string      `json:"content"`
	ProviderID   uint        `json:"provider_id"`
	ProviderName string      `json:"provider_name"`
	RouteName    string      `json:"route_name"`
	UsedFallback bool        `json:"used_fallback"`
	AttemptCount int         `json:"attempt_count"`
	Usage        *TokenUsage `json:"usage,omitempty"`
}

// TokenUsage mirrors the OpenAI usage block.
type TokenUsage struct {
	PromptTokens     int `json:"prompt"`
	CompletionTokens int `json:"completion"`
	TotalTokens      int `json:"total"`
}

// UnmarshalJSON accepts both the OpenAI-compatible wire keys
// (prompt_tokens/completion_tokens/total_tokens) and the legacy stored keys
// (prompt/completion/total). Providers on the OpenAI-compatible protocol
// return the *_tokens family; the legacy family keeps round-trips of the
// stored jsonb shape ({"prompt":N,...}) working defensively. When both are
// present the standard wire keys win. Missing keys leave zero values without
// error. MarshalJSON is intentionally left default so the stored jsonb shape
// (consumed by session aggregation) never changes.
func (u *TokenUsage) UnmarshalJSON(data []byte) error {
	type alias TokenUsage
	wire := struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
		TotalTokens      *int `json:"total_tokens"`
		*alias
	}{
		alias: (*alias)(u),
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.PromptTokens != nil {
		u.PromptTokens = *wire.PromptTokens
	}
	if wire.CompletionTokens != nil {
		u.CompletionTokens = *wire.CompletionTokens
	}
	if wire.TotalTokens != nil {
		u.TotalTokens = *wire.TotalTokens
	}
	return nil
}

// ChatResponse is the return of ProviderClient.Chat (replaces bare string).
type ChatResponse struct {
	Content string      `json:"content"`
	Usage   *TokenUsage `json:"usage,omitempty"`
}

type ProviderClient interface {
	Chat(ctx context.Context, provider models.AIProvider, req ChatRequest) (*ChatResponse, error)
	Embed(ctx context.Context, provider models.AIProvider, req EmbeddingRequest) (*EmbeddingResult, error)
}

type ProviderError struct {
	Message   string
	Code      string
	Retryable bool
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type openAICompatibleClient struct{}

func NewOpenAICompatibleClient() ProviderClient {
	return &openAICompatibleClient{}
}

func (c *openAICompatibleClient) Chat(ctx context.Context, provider models.AIProvider, req ChatRequest) (*ChatResponse, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "openAICompatibleClient.Chat")
	defer span.End()
	payload := buildPayload(provider, req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	timeout := time.Duration(provider.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	resp, err := httpclient.New(httpclient.WithTimeout(timeout)).Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Message: err.Error(), Code: "network_error", Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Message: err.Error(), Code: "read_error", Retryable: true}
	}
	// logging.Infof("openai: message=%s ", req.Messages)
	// logging.Infof("openai_chat_raw: provider=%s model=%s status=%d body=%s", provider.Name, provider.Model, resp.StatusCode, truncateDebugBody(string(responseBody)))

	var parsed struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string      `json:"message"`
			Type    string      `json:"type"`
			Code    interface{} `json:"code"`
		} `json:"error,omitempty"`
		Usage *TokenUsage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, &ProviderError{Message: fmt.Sprintf("failed to parse response: %v", err), Code: "parse_error", Retryable: resp.StatusCode >= 500}
	}

	if resp.StatusCode >= 400 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		message := string(responseBody)
		code := fmt.Sprintf("http_%d", resp.StatusCode)
		if parsed.Error != nil {
			message = parsed.Error.Message
			codeStr := fmt.Sprintf("%v", parsed.Error.Code)
			if codeStr != "" && codeStr != "<nil>" {
				code = codeStr
			}
		}
		return nil, &ProviderError{Message: message, Code: code, Retryable: retryable}
	}

	if parsed.Error != nil {
		return nil, &ProviderError{Message: parsed.Error.Message, Code: fmt.Sprintf("%v", parsed.Error.Code), Retryable: false}
	}
	if len(parsed.Choices) == 0 {
		return nil, &ProviderError{Message: "no response from AI", Code: "no_response", Retryable: true}
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	return &ChatResponse{Content: content, Usage: parsed.Usage}, nil
}

// buildPayload constructs the OpenAI-compatible request payload from provider settings and chat request.
// It ALWAYS propagates chat_template_kwargs.enable_thinking (true or false) per-request: the
// Qwen3/Qwythos chat template defaults to thinking ON when this kwarg is absent (it only inserts an
// empty <think></think> to suppress reasoning when enable_thinking=false is sent explicitly). So
// omitting the kwarg does not mean "off" — we must send it explicitly for the toggle to take effect.
// For non-llama.cpp servers this is an unknown field that is harmlessly ignored.
func buildPayload(provider models.AIProvider, req ChatRequest) map[string]any {
	temperature := 0.3
	if req.Temperature != nil {
		temperature = *req.Temperature
	} else if provider.Temperature != nil {
		temperature = *provider.Temperature
	}

	maxTokens := 16384
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	} else if provider.MaxTokens != nil {
		maxTokens = *provider.MaxTokens
	}

	payload := map[string]any{
		"model":       provider.Model,
		"messages":    req.Messages,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	// Always send enable_thinking explicitly. See buildPayload doc comment for rationale.
	payload["chat_template_kwargs"] = map[string]any{"enable_thinking": provider.EnableThinking}

	if provider.ProviderType == ProviderTypeOllama {
		if req.JSONMode && req.JSONSchema != nil {
			payload["format"] = req.JSONSchema
		} else if req.JSONMode {
			payload["format"] = "json"
		}
	} else if req.JSONMode {
		if req.JSONSchema != nil {
			payload["response_format"] = map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "response",
					"strict": true,
					"schema": req.JSONSchema,
				},
			}
		} else {
			payload["response_format"] = map[string]any{"type": "json_object"}
		}
	}
	return payload
}

// stripThinkTags removes <think>...</think> reasoning blocks from content. It is no longer
// called by Chat (servers that support reasoning now separate it into a reasoning_content field),
// but is retained as a defensive helper: if a future provider returns <think> tags embedded in
// content, this can be reapplied without re-deriving the regex.
//
//nolint:unused // retained defensively; see design.md Risks/Trade-offs
func stripThinkTags(content string) string {
	return strings.TrimSpace(thinkTagRe.ReplaceAllString(content, ""))
}

func (c *openAICompatibleClient) Embed(ctx context.Context, provider models.AIProvider, req EmbeddingRequest) (*EmbeddingResult, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "openAICompatibleClient.Embed")
	defer span.End()
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	model := req.Model
	if model == "" {
		model = provider.Model
	}

	body := EmbeddingRequest{
		Input:          req.Input,
		Model:          model,
		EncodingFormat: req.EncodingFormat,
		Dimensions:     req.Dimensions,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	timeout := time.Duration(provider.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	resp, err := httpclient.New(httpclient.WithTimeout(timeout)).Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Message: err.Error(), Code: "network_error", Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Message: err.Error(), Code: "read_error", Retryable: true}
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error *struct {
				Message string      `json:"message"`
				Type    string      `json:"type"`
				Code    interface{} `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(responseBody, &errResp)
		msg := string(responseBody)
		if errResp.Error != nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, &ProviderError{Message: msg, Code: fmt.Sprintf("http_%d", resp.StatusCode), Retryable: retryable}
	}

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Model string      `json:"model"`
		Usage *TokenUsage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if len(parsed.Data) == 0 {
		return nil, &ProviderError{Message: "no embeddings in response", Code: "no_response", Retryable: false}
	}

	embeddings := make([][]float64, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}

	dimensions := 0
	if len(embeddings) > 0 && len(embeddings[0]) > 0 {
		dimensions = len(embeddings[0])
	}

	return &EmbeddingResult{
		Embeddings: embeddings,
		Model:      parsed.Model,
		Dimensions: dimensions,
		Provider:   provider.Name,
		Usage:      parsed.Usage,
	}, nil
}
