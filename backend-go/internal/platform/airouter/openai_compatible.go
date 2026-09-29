package airouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

var thinkTagRe = regexp.MustCompile(`(?s)<think\s*>.*?</think\s*>`)

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
	// ThinkingLevel 是请求级思考档（specs/ai-thinking-levels 四级解析的最高优先级），
	// 空=调用方未指定，逐级回落到绑定级/模型默认/全局默认。
	ThinkingLevel string
}

// ChatResult 的 ProviderID/ProviderName 在 vendor/model 两层化后承载 vendor 来源
// （ai-provider-model-overhaul）：ProviderID=vendor.ID、ProviderName=vendor.Name，
// 字段名保持不变使业务调用方（extractor/dataenrichment/topicgraph）零改动；
// ModelName 为新增字段承载实际使用的模型名。
type ChatResult struct {
	Content      string      `json:"content"`
	ProviderID   uint        `json:"provider_id"`
	ProviderName string      `json:"provider_name"`
	ModelName    string      `json:"model_name"`
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

// 结构化输出档位（vendor.StructuredOutput 值域）与降级链尝试档共用同一词表：
// 降级方向 json_schema → json_object → plain（design D5）。
const (
	StructOutputJSONSchema = "json_schema"
	StructOutputJSONObject = "json_object"
	StructOutputNone       = "none"
	StageJSONSchema        = "json_schema"
	StageJSONObject        = "json_object"
	StagePlain             = "plain"
)

// StructuredAttempt 记录 client 内 400 降级链中每档真实 HTTP 尝试（W4）。
// 无降级链时成功返回 nil 列表、失败也 nil——错误本身即一次普通尝试。
type StructuredAttempt struct {
	Stage     string // "json_schema" | "json_object" | "plain"
	LatencyMs int
	ErrorCode string // 成功档为 ""
	ErrorMsg  string
	Content   string // 成功档的响应内容（失败档空）
}

// ProviderClient 的 model 参数承载模型名与 SupportsXhighMax（思考钳制），vendor 承载
// 协议/端点/认证/方言/结构化档位。effLevel 为 Router 侧四级解析+钳制后的生效思考档。
type ProviderClient interface {
	Chat(ctx context.Context, vendor models.AIVendor, model models.AIModel, effLevel string, req ChatRequest) (*ChatResponse, []StructuredAttempt, error)
	Embed(ctx context.Context, vendor models.AIVendor, model models.AIModel, req EmbeddingRequest) (*EmbeddingResult, error)
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

func (c *openAICompatibleClient) Chat(ctx context.Context, vendor models.AIVendor, model models.AIModel, effLevel string, req ChatRequest) (*ChatResponse, []StructuredAttempt, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "openAICompatibleClient.Chat")
	defer span.End()

	// 400 降级链仅在「结构化请求 + vendor 声明 json_schema 档」时启用（W4）；
	// 其余情况单档直接请求（vendor 档位内构造，无链无 attempts）。
	if req.JSONMode && req.JSONSchema != nil && vendor.StructuredOutput == StructOutputJSONSchema {
		return c.chatWithDegradeChain(ctx, vendor, model, effLevel, req)
	}

	payload := buildPayload(vendor, model, effLevel, req, "")
	resp, err := c.postChat(ctx, vendor, payload)
	if err != nil {
		return nil, nil, err
	}
	return resp, nil, nil
}

// chatWithDegradeChain 实现 client 内三档降级：json_schema --4xx--> json_object
// --4xx--> plain --失败--> 该尝试失败（design D5 / test-cases W4）。仅 4xx（非 429）
// 降档；429/5xx/网络错误不降级，但已发生的每档真实 HTTP 调用仍随 attempts
// 逐行上抛（中途非 4xx 中止时终错追加为当前档 attempt，Router 逐行记账，
// 1 次 HTTP = 1 行，红线5）。
// 降级链不修改 vendor（不落库、不改内存对象字段）。
func (c *openAICompatibleClient) chatWithDegradeChain(ctx context.Context, vendor models.AIVendor, model models.AIModel, effLevel string, req ChatRequest) (*ChatResponse, []StructuredAttempt, error) {
	stages := []string{StageJSONSchema, StageJSONObject, StagePlain}
	var attempts []StructuredAttempt
	var lastErr error
	for _, stage := range stages {
		start := time.Now()
		payload := buildPayload(vendor, model, effLevel, req, stage)
		resp, status, err := c.postChatWithStatus(ctx, vendor, payload)
		latencyMs := int(time.Since(start).Milliseconds())
		if err == nil {
			// 中途任一档 2xx 即止：追加成功档信息，返回成功。
			attempts = append(attempts, StructuredAttempt{Stage: stage, LatencyMs: latencyMs, Content: resp.Content})
			return resp, attempts, nil
		}
		code := "http_error"
		if provErr := (*ProviderError)(nil); errors.As(err, &provErr) && provErr.Code != "" {
			code = provErr.Code
		}
		attempts = append(attempts, StructuredAttempt{Stage: stage, LatencyMs: latencyMs, ErrorCode: code, ErrorMsg: err.Error()})
		if isDegradeableStatus(status) {
			lastErr = err
			continue
		}
		// 429/5xx/网络/读取错误：降档救不了，不降级；终错已作为当前档 attempt
		// 追加（含此前已发生档），交由 Router 逐行记账后按普通尝试失败上抛
		// （红线5：每次尝试必写审计，1 次 HTTP = 1 行）。
		return nil, attempts, err
	}
	return nil, attempts, lastErr
}

// isDegradeableStatus 判定该 HTTP 状态是否触发降档：4xx 且非 429（429 是限流，
// 降档无用；5xx 是服务端问题；status=0 表示请求未达服务端，属网络错误）。
func isDegradeableStatus(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusTooManyRequests
}

// postChat sends one chat request and parses the response; status is the HTTP
// status code (0 when the request never reached a response, e.g. network errors).
func (c *openAICompatibleClient) postChat(ctx context.Context, vendor models.AIVendor, payload map[string]any) (*ChatResponse, error) {
	resp, _, err := c.postChatWithStatus(ctx, vendor, payload)
	return resp, err
}

func (c *openAICompatibleClient) postChatWithStatus(ctx context.Context, vendor models.AIVendor, payload map[string]any) (*ChatResponse, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	endpoint := strings.TrimRight(vendor.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// API key 为空时不发 Authorization 头（W8-1：本地服务无需 key）。
	if vendor.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+vendor.APIKey)
	}

	timeout := time.Duration(vendor.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		// FB-10：与 store 层 normalizeVendor 缺省一致（600s）
		timeout = 600 * time.Second
	}
	resp, err := httpclient.New(httpclient.WithTimeout(timeout)).Do(httpReq)
	if err != nil {
		return nil, 0, &ProviderError{Message: err.Error(), Code: "network_error", Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, &ProviderError{Message: err.Error(), Code: "read_error", Retryable: true}
	}

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
	// 4xx 响应体可能是非 JSON（HTML 错误页等），此时仍按 HTTP 状态构造错误，
	// 保证降级链的 4xx 判定不被响应体格式破坏。
	parseErr := json.Unmarshal(responseBody, &parsed)

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
		return nil, resp.StatusCode, &ProviderError{Message: message, Code: code, Retryable: retryable}
	}
	if parseErr != nil {
		return nil, resp.StatusCode, &ProviderError{Message: fmt.Sprintf("failed to parse response: %v", parseErr), Code: "parse_error", Retryable: resp.StatusCode >= 500}
	}

	if parsed.Error != nil {
		return nil, resp.StatusCode, &ProviderError{Message: parsed.Error.Message, Code: fmt.Sprintf("%v", parsed.Error.Code), Retryable: false}
	}
	if len(parsed.Choices) == 0 {
		return nil, resp.StatusCode, &ProviderError{Message: "no response from AI", Code: "no_response", Retryable: true}
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	return &ChatResponse{Content: content, Usage: parsed.Usage}, resp.StatusCode, nil
}

// buildPayload 按 vendor 思考方言与结构化输出档位构造 OpenAI 兼容请求 payload。
//
// 思考参数：thinkingToWire（dialect × effLevel × supportsXhighMax）返回的 map merge 进
// payload（nil=不发任何思考控制参数，none 方言 / reasoning-effort 的 off 档）。
//
// 结构化输出三态（design D5 / test-cases W3）：仅 req.JSONMode 时生效，档位取
// structuredStage 显式值（降级链传入，"json_schema"/"json_object"/"plain"），
// 空则回落 vendor.StructuredOutput（""未配置按 none 防御）：
//
//   - json_schema：openai_compatible 发 response_format={type:json_schema,
//     json_schema:{name:"response",strict:true,schema}}；ollama 特例发 format=<schema>；
//     不注入 prompt（schema 已随 response_format 下发）。无 schema 可带时降按 json_object。
//   - json_object：发 response_format={type:json_object}（ollama 发 format="json"），
//     并把 schema 以指令文本注入最后一条 user message 末尾。
//   - plain / none：不发 response_format / format，schema 仍注入。
//
// 注入模板锚句（观测/测试断言用，一字不差）：
// "\n\n你必须只输出符合以下 JSON 结构的 JSON 文本，不要包含任何其他文字、解释或代码块围栏：" + compact JSON。
// 注入时 clone messages，原 req.Messages 不被修改。
func buildPayload(vendor models.AIVendor, model models.AIModel, effLevel string, req ChatRequest, structuredStage string) map[string]any {
	temperature := 0.3
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	maxTokens := 16384
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	payload := map[string]any{
		"model":       model.Name,
		"messages":    req.Messages,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	// 思考参数：方言映射纯函数；nil（none 方言等）跳过 merge。
	if thinking := thinkingToWire(vendor.ThinkingDialect, effLevel, model.SupportsXhighMax); thinking != nil {
		for k, v := range thinking {
			payload[k] = v
		}
	}

	if req.JSONMode {
		stage := structuredStage
		if stage == "" {
			stage = vendor.StructuredOutput
		}
		if stage == "" {
			stage = StructOutputNone // 防御：未配置档位按 none（不发 response_format、仍注入）
		}
		// json_schema 档无 schema 可带时降按 json_object 处理。
		effective := stage
		if effective == StageJSONSchema && req.JSONSchema == nil {
			effective = StageJSONObject
		}
		isOllama := vendor.ProviderType == ProviderTypeOllama
		switch effective {
		case StageJSONSchema:
			if isOllama {
				payload["format"] = req.JSONSchema
			} else {
				payload["response_format"] = map[string]any{
					"type": "json_schema",
					"json_schema": map[string]any{
						"name":   "response",
						"strict": true,
						"schema": req.JSONSchema,
					},
				}
			}
		case StageJSONObject:
			if isOllama {
				payload["format"] = "json"
			} else {
				payload["response_format"] = map[string]any{"type": "json_object"}
			}
			if req.JSONSchema != nil {
				payload["messages"] = injectSchemaInstruction(req.Messages, req.JSONSchema)
			}
		default: // plain / none：不发 response_format / format，schema 仍注入
			if req.JSONSchema != nil {
				payload["messages"] = injectSchemaInstruction(req.Messages, req.JSONSchema)
			}
		}
	}
	return payload
}

// schemaInjectionAnchor 是 schema 注入指令锚句（观测/测试断言用，一字不差）。
const schemaInjectionAnchor = "\n\n你必须只输出符合以下 JSON 结构的 JSON 文本，不要包含任何其他文字、解释或代码块围栏："

// injectSchemaInstruction clone messages 并把 schema 指令追加到最后一条 user message
// 的 Content 末尾（无 user message 时回退最后一条；空 messages 不注入）。
// 原 slice 与其中的 Message 值均不被修改。
func injectSchemaInstruction(messages []Message, schema *JSONSchema) []Message {
	cloned := make([]Message, len(messages))
	copy(cloned, messages)
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		// 防御：schema 序列化失败则仅返回 clone，不带指令（不应发生）。
		return cloned
	}
	instruction := schemaInjectionAnchor + string(schemaJSON)
	idx := -1
	for i := len(cloned) - 1; i >= 0; i-- {
		if cloned[i].Role == "user" {
			idx = i
			break
		}
	}
	if idx == -1 && len(cloned) > 0 {
		idx = len(cloned) - 1
	}
	if idx >= 0 {
		cloned[idx].Content += instruction
	}
	return cloned
}

// extractBalancedJSON 从自由文本中提取首个花括号平衡的 JSON 对象块（W5 解析纠错）。
// 原文本身是合法 JSON 时原样返回；否则扫描首个 '{'，计数花括号平衡——带引号字符串
// 内的花括号与转义（\" 、\\）不参与计数——平衡闭合处截取。无 '{' 或不平衡 → false。
// 提取块的 JSON 合法性由调用方再校验（本函数只负责平衡截取）。
func extractBalancedJSON(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, true
	}
	start := strings.IndexByte(trimmed, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(trimmed); i++ {
		ch := trimmed[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1], true
			}
		}
	}
	return "", false
}

// stripThinkTags removes <think>...</think> reasoning blocks from content and
// trims the remainder. Router.Chat applies it when the effective thinking level
// is not off（specs/ai-thinking-levels「输出清理跟随思考档」：off 档保留原始内容）。
func stripThinkTags(content string) string {
	return strings.TrimSpace(thinkTagRe.ReplaceAllString(content, ""))
}

func (c *openAICompatibleClient) Embed(ctx context.Context, vendor models.AIVendor, model models.AIModel, req EmbeddingRequest) (*EmbeddingResult, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "openAICompatibleClient.Embed")
	defer span.End()
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	modelName := req.Model
	if modelName == "" {
		modelName = model.Name
	}

	body := EmbeddingRequest{
		Input:          req.Input,
		Model:          modelName,
		EncodingFormat: req.EncodingFormat,
		Dimensions:     req.Dimensions,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	endpoint := strings.TrimRight(vendor.BaseURL, "/") + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if vendor.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+vendor.APIKey)
	}

	timeout := time.Duration(vendor.TimeoutSeconds) * time.Second
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
		Provider:   vendor.Name,
		Usage:      parsed.Usage,
	}, nil
}
