package airouter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	otelCodes "go.opentelemetry.io/otel/codes"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

const maxResponseSnippet = 10000
const maxPromptRunes = 20000
const maxCachePreviewRunes = 200

var defaultConcurrency = map[Capability]int{
	CapabilityTopicTagging: 3,
	CapabilityDigestPolish: 2,
	CapabilityOpenNotebook: 2,
	CapabilityEmbedding:    5,
	// CapabilityFeedDiscovery: 推荐精排 LLM，低频突发（手动刷新/问答），与总结同档并发。
	CapabilityFeedDiscovery: 2,
}

func truncateSnippet(s string) string {
	runes := []rune(s)
	if len(runes) > maxResponseSnippet {
		return string(runes[:maxResponseSnippet]) + "..."
	}
	return s
}

// formatMessages joins ChatRequest messages into a human-readable prompt string
// for logging. Format: [role]\n{content}\n\n[role]\n{content}. If the result
// exceeds 20000 runes, the first 18000 and last 2000 are kept with a truncation
// marker in between.
func formatMessages(messages []Message) string {
	var b strings.Builder
	for i, m := range messages {
		b.WriteString("[")
		b.WriteString(m.Role)
		b.WriteString("]\n")
		b.WriteString(m.Content)
		if i < len(messages)-1 {
			b.WriteString("\n\n")
		}
	}
	raw := b.String()
	runes := []rune(raw)
	if len(runes) <= maxPromptRunes {
		return raw
	}
	keptHead := runes[:18000]
	keptTail := runes[len(runes)-2000:]
	truncated := len(runes) - 18000 - 2000
	return fmt.Sprintf("%s\n...[truncated %d runes]...\n%s", string(keptHead), truncated, string(keptTail))
}

// encodeTokenUsage marshals a TokenUsage pointer into a JSON string suitable for
// a jsonb column. Returns empty string when usage is nil.
func encodeTokenUsage(u *TokenUsage) string {
	if u == nil {
		return ""
	}
	b, _ := json.Marshal(u)
	return string(b)
}

// isEmptyChatResponse reports whether a transport-successful provider call
// carries no usable assistant content: a nil response or blank content
// (whitespace-only counts as blank). Router.Chat normalizes such results into
// a retryable ProviderError{Code:"empty_response"} so the ordered-fallback
// chain treats them like any other provider failure — observed in production
// as glm board_synthesize returning HTTP 200 with empty assistant content.
func isEmptyChatResponse(resp *ChatResponse) bool {
	return resp == nil || strings.TrimSpace(resp.Content) == ""
}

type Router struct {
	store   *Store
	clients map[string]ProviderClient
	semMap  sync.Map // map[Capability]chan struct{}
}

func NewRouter() *Router {
	store := NewStore(database.DB)
	return &Router{
		store: store,
		clients: map[string]ProviderClient{
			ProviderTypeOpenAICompatible: NewOpenAICompatibleClient(),
			ProviderTypeOllama:           NewOpenAICompatibleClient(),
		},
	}
}

func NewRouterWithStore(store *Store) *Router {
	return &Router{
		store: store,
		clients: map[string]ProviderClient{
			ProviderTypeOpenAICompatible: NewOpenAICompatibleClient(),
			ProviderTypeOllama:           NewOpenAICompatibleClient(),
		},
	}
}

func (r *Router) RegisterClient(providerType string, client ProviderClient) {
	if client == nil {
		return
	}
	r.clients[providerType] = client
}

func (r *Router) resolveConcurrency(capability Capability, route *models.AIRoute) int {
	if route != nil && route.MaxConcurrency > 0 {
		return route.MaxConcurrency
	}
	if n, ok := defaultConcurrency[capability]; ok {
		return n
	}
	return 3
}

func (r *Router) getSemaphore(capability Capability, route *models.AIRoute) chan struct{} {
	n := r.resolveConcurrency(capability, route)
	ch, _ := r.semMap.LoadOrStore(capability, make(chan struct{}, n))
	return ch.(chan struct{})
}

func (r *Router) acquireSem(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Router) releaseSem(sem chan struct{}) {
	select {
	case <-sem:
	default:
	}
}

// levelPtr 把字符串档位转 *string（空串=nil 未指定），适配 effectiveThinkingLevel 的三级指针入参。
func levelPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// structuredStageMeta clone 调用方 metadata 并附加 structured_stage（观测还原降级方向
// 的错点，D6）。返回新 map，不修改调用方数据。
func structuredStageMeta(base map[string]any, stage string) map[string]any {
	meta := make(map[string]any, len(base)+1)
	for k, v := range base {
		meta[k] = v
	}
	meta["structured_stage"] = stage
	return meta
}

// correctStructuredJSON 校验结构化响应内容（W5 解析纠错，Router 在拿到 2xx 内容后、
// 返回 ChatResult 前执行）：原文合法 JSON → 原样；否则提取首个平衡 JSON 块再校验。
// 均失败 → false（调用方按 parse_error 尝试失败处理，不把非 JSON 原文冒充结果返回）。
func correctStructuredJSON(content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if json.Valid([]byte(trimmed)) {
		return trimmed, true
	}
	extracted, ok := extractBalancedJSON(trimmed)
	if !ok || !json.Valid([]byte(extracted)) {
		return "", false
	}
	return extracted, true
}

func (r *Router) Chat(ctx context.Context, req ChatRequest) (result *ChatResult, err error) {
	if req.Operation == "" {
		return nil, errors.New("airouter: Operation is required")
	}

	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "Router.Chat")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(otelCodes.Error, "error")
			span.RecordError(err)
		}
	}()
	span.SetAttributes(attribute.String("ai.capability", string(req.Capability)))
	span.SetAttributes(attribute.String("ai.operation", req.Operation))
	if req.SessionID != "" {
		span.SetAttributes(attribute.String("ai.session_id", req.SessionID))
	}
	if b := baggage.FromContext(ctx); b.Len() > 0 {
		for _, m := range b.Members() {
			span.SetAttributes(attribute.String("baggage."+m.Key(), m.Value()))
		}
	}
	route, bindings, err := r.store.LoadRouteWithModels(req.Capability)
	if err != nil {
		return nil, err
	}

	sem := r.getSemaphore(req.Capability, route)
	if err := r.acquireSem(ctx, sem); err != nil {
		return nil, err
	}
	defer r.releaseSem(sem)

	structured := req.JSONMode && req.JSONSchema != nil
	var attemptErrors []error
	for idx, b := range bindings {
		client := r.clients[b.Vendor.ProviderType]
		if client == nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("provider type %s unsupported", b.Vendor.ProviderType))
			continue
		}

		// 四级解析（请求级>绑定级>模型默认>medium）+ 按模型支持度钳制（钳制发生在 wire 之前）。
		effLevel := clampLevel(effectiveThinkingLevel(levelPtr(req.ThinkingLevel), levelPtr(b.ThinkingLevel), b.Model.ThinkingLevel), b.Model.SupportsXhighMax)

		start := time.Now()
		chatResp, attempts, callErr := client.Chat(ctx, b.Vendor, b.Model, effLevel, req)
		latencyMs := int(time.Since(start).Milliseconds())

		// 降级链失败档逐行审计（红线5：每档真实 HTTP 尝试各留一行；W4 RequestMeta
		// 带 structured_stage 供观测还原降级方向）。
		for _, at := range attempts {
			if at.ErrorCode == "" {
				continue // 成功档由下方常规成功行覆盖
			}
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:       req.Operation,
				Capability:      string(req.Capability),
				RouteName:       route.Name,
				ProviderName:    b.Vendor.Name,
				Model:           b.Model.Name,
				Success:         false,
				IsFallback:      idx > 0,
				LatencyMs:       at.LatencyMs,
				ErrorCode:       at.ErrorCode,
				ErrorMessage:    at.ErrorMsg,
				Prompt:          formatMessages(req.Messages),
				RequestMeta:     encodeMeta(structuredStageMeta(req.Metadata, at.Stage)),
				ResponseSnippet: truncateSnippet(at.ErrorMsg),
				SessionID:       req.SessionID,
			})
		}
		// 降级链穷尽（最后一档仍 4xx）时失败档已逐行留痕，HTTP 层无未记录调用，
		// 不再补一行笼统失败行（1 次 HTTP = 1 行，D5）。
		chainExhausted := len(attempts) > 0 && attempts[len(attempts)-1].ErrorCode != ""

		if callErr == nil && isEmptyChatResponse(chatResp) {
			// Provider boundary normalization: HTTP success with no usable
			// assistant content is a retryable provider failure, not a success.
			// Applies to every provider type uniformly (no per-client special
			// casing) and flows into the existing failure log / fallback path.
			callErr = &ProviderError{
				Code:      "empty_response",
				Retryable: true,
				Message:   "provider returned empty response content",
			}
		}

		if callErr == nil {
			content := chatResp.Content
			// 输出清理跟随生效档（specs/ai-thinking-levels）：off 保留原文（含 <think>
			// 标签），非 off 剥离后 TrimSpace。清理在解析纠错之前（先剥标签再提取 JSON）；
			// reasoning_content 独立字段行为保持（client 解析只取 content）。
			if effLevel != ThinkingOff {
				content = stripThinkTags(content)
			}
			if structured {
				if cleaned, ok := correctStructuredJSON(content); ok {
					content = cleaned
				} else {
					callErr = &ProviderError{
						Code:      "parse_error",
						Retryable: true,
						Message:   "response does not contain extractable valid JSON",
					}
				}
			}
			if callErr == nil {
				chatResp.Content = content
				meta := req.Metadata
				if structured {
					// 降级链成功时取成功档；单档时取 vendor 声明档位（观测识别结构化调用）。
					stage := b.Vendor.StructuredOutput
					if n := len(attempts); n > 0 {
						stage = attempts[n-1].Stage
					}
					meta = structuredStageMeta(req.Metadata, stage)
				}
				r.store.LogCall(ctx, &models.AICallLog{
					Operation:       req.Operation,
					Capability:      string(req.Capability),
					RouteName:       route.Name,
					ProviderName:    b.Vendor.Name,
					Model:           b.Model.Name,
					Success:         true,
					IsFallback:      idx > 0,
					LatencyMs:       latencyMs,
					Prompt:          formatMessages(req.Messages),
					RequestMeta:     encodeMeta(meta),
					ResponseSnippet: truncateSnippet(chatResp.Content),
					TokenUsage:      encodeTokenUsage(chatResp.Usage),
					SessionID:       req.SessionID,
				})

				return &ChatResult{
					Content:      chatResp.Content,
					ProviderID:   b.Vendor.ID,
					ProviderName: b.Vendor.Name,
					ModelName:    b.Model.Name,
					RouteName:    route.Name,
					UsedFallback: idx > 0,
					AttemptCount: idx + 1,
					Usage:        chatResp.Usage,
				}, nil
			}
		}

		providerErr := &ProviderError{}
		code := "provider_error"
		if errors.As(callErr, &providerErr) {
			if providerErr.Code != "" {
				code = providerErr.Code
			}
		}
		if !chainExhausted {
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:       req.Operation,
				Capability:      string(req.Capability),
				RouteName:       route.Name,
				ProviderName:    b.Vendor.Name,
				Model:           b.Model.Name,
				Success:         false,
				IsFallback:      idx > 0,
				LatencyMs:       latencyMs,
				ErrorCode:       code,
				ErrorMessage:    callErr.Error(),
				Prompt:          formatMessages(req.Messages),
				RequestMeta:     encodeMeta(req.Metadata),
				ResponseSnippet: truncateSnippet(callErr.Error()),
				SessionID:       req.SessionID,
			})
		}
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", b.Vendor.Name, callErr))
	}

	finalErr := errors.Join(attemptErrors...)

	return nil, finalErr
}

// saveEmbeddingCache persists a successful embedding result. Best-effort:
// failures are logged and swallowed so the provider result still returns.
func (r *Router) saveEmbeddingCache(req EmbeddingRequest, model string, res *EmbeddingResult) {
	if !embeddingCacheable(req.Operation) {
		return
	}
	payload := models.EncodeEmbeddingVectors(res.Embeddings)
	rec := &models.AIEmbeddingCache{
		CacheKey:     embeddingCacheKey(model, req.Input),
		Model:        model,
		Operation:    req.Operation,
		Embedding:    payload,
		Dimensions:   res.Dimensions,
		InputPreview: truncateRunes(strings.Join(req.Input, " "), maxCachePreviewRunes),
		CreatedAt:    time.Now(),
	}
	if err := r.store.SaveEmbeddingCache(context.Background(), rec); err != nil {
		logging.Warnf("airouter: embedding cache write failed (key=%s): %v", rec.CacheKey, err)
	}
}

func (r *Router) ResolvePrimaryModel(capability Capability) (*models.AIModel, *models.AIVendor, *models.AIRoute, error) {
	return r.store.ResolvePrimaryModel(capability)
}

// embeddingCacheOperations is the allowlist of operations whose results are
// persisted to ai_embedding_cache. Only operations with recurring identical
// inputs benefit from caching. One-shot content embeddings (article section
// text, aux-label "label + article context" combos, route backfill) are
// effectively write-only: production data showed ~0-10% hit rates while each
// row costs ~30KB, so they are excluded.
var embeddingCacheOperations = map[string]struct{}{
	"tagmanagement.embedding": {},
}

// embeddingCacheable reports whether an operation participates in the
// embedding cache (both lookup and save sides).
func embeddingCacheable(operation string) bool {
	_, ok := embeddingCacheOperations[operation]
	return ok
}

// embeddingCacheKey derives the ai_embedding_cache primary key for an
// embedding request. The route's effective model participates in the key so
// different models (or fallback providers with different models) never share
// vectors across incompatible spaces. NUL-separated inputs make the join
// unambiguous.
func embeddingCacheKey(model string, inputs []string) string {
	h := sha256.New()
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write([]byte(strings.Join(inputs, "\x00")))
	return hex.EncodeToString(h.Sum(nil))
}

// truncateRunes keeps at most max runes of s.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}

func (r *Router) Embed(ctx context.Context, req EmbeddingRequest, capability Capability) (result *EmbeddingResult, err error) {
	if req.Operation == "" {
		return nil, errors.New("airouter: Operation is required")
	}

	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "Router.Embed")
	defer span.End()
	span.SetAttributes(attribute.String("ai.capability", string(capability)))
	span.SetAttributes(attribute.String("ai.operation", req.Operation))
	if req.SessionID != "" {
		span.SetAttributes(attribute.String("ai.session_id", req.SessionID))
	}

	route, bindings, err := r.store.LoadRouteWithModels(capability)
	if err != nil {
		return nil, err
	}

	// Cache lookup happens BEFORE the semaphore: a hit is a local DB read and
	// must neither queue behind real provider calls nor consume a slot.
	// Operations outside embeddingCacheOperations skip the lookup entirely
	// (their rows are never written, so lookup would always miss) and go
	// straight to the provider below.
	// The cache key intentionally uses the route binding's Model name, not
	// req.Model: no call site sets req.Model today, and the client layer
	// falls back to the binding's model when req.Model is empty. If a future
	// call site starts setting req.Model, normalize the effective model before
	// it participates in the key, otherwise the cached Model field observability
	// becomes inaccurate.
	var lookupStart time.Time
	var cacheKey string
	var cached *models.AIEmbeddingCache
	var lookupErr error
	if embeddingCacheable(req.Operation) {
		lookupStart = time.Now()
		cacheKey = embeddingCacheKey(bindings[0].Model.Name, req.Input)
		cached, lookupErr = r.store.LookupEmbeddingCache(ctx, cacheKey)
	}
	if lookupErr != nil {
		// A broken cache (e.g. table missing) must never block real calls.
		logging.Warnf("airouter: embedding cache lookup failed (key=%s): %v", cacheKey, lookupErr)
	} else if cached != nil {
		vectors, err := models.DecodeEmbeddingVectors(cached.Embedding)
		if err != nil {
			logging.Warnf("airouter: embedding cache decode failed (key=%s): %v", cacheKey, err)
		} else {
			hitMeta := map[string]any{}
			for k, v := range req.Metadata {
				hitMeta[k] = v
			}
			// Set after copying so upstream metadata can never shadow the marker.
			hitMeta["cache_hit"] = true
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:    req.Operation,
				Capability:   string(capability),
				RouteName:    route.Name,
				ProviderName: bindings[0].Vendor.Name,
				Model:        cached.Model,
				Success:      true,
				IsFallback:   false,
				LatencyMs:    int(time.Since(lookupStart).Milliseconds()),
				RequestMeta:  encodeMeta(hitMeta),
				SessionID:    req.SessionID,
			})
			return &EmbeddingResult{
				Embeddings: vectors,
				Model:      cached.Model,
				Dimensions: cached.Dimensions,
				Provider:   bindings[0].Vendor.Name,
			}, nil
		}
	}

	sem := r.getSemaphore(capability, route)
	if err := r.acquireSem(ctx, sem); err != nil {
		return nil, err
	}
	defer r.releaseSem(sem)

	var attemptErrors []error
	for idx, b := range bindings {
		client := r.clients[b.Vendor.ProviderType]
		if client == nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("provider type %s unsupported", b.Vendor.ProviderType))
			continue
		}

		start := time.Now()
		res, callErr := client.Embed(ctx, b.Vendor, b.Model, req)
		latencyMs := int(time.Since(start).Milliseconds())
		if callErr == nil {
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:    req.Operation,
				Capability:   string(capability),
				RouteName:    route.Name,
				ProviderName: b.Vendor.Name,
				Model:        b.Model.Name,
				Success:      true,
				IsFallback:   idx > 0,
				LatencyMs:    latencyMs,
				TokenUsage:   encodeTokenUsage(res.Usage),
				RequestMeta:  encodeMeta(req.Metadata),
				SessionID:    req.SessionID,
			})
			r.saveEmbeddingCache(req, b.Model.Name, res)
			return res, nil
		}

		providerErr := &ProviderError{}
		code := "provider_error"
		if errors.As(callErr, &providerErr) {
			if providerErr.Code != "" {
				code = providerErr.Code
			}
		}
		r.store.LogCall(ctx, &models.AICallLog{
			Operation:    req.Operation,
			Capability:   string(capability),
			RouteName:    route.Name,
			ProviderName: b.Vendor.Name,
			Model:        b.Model.Name,
			Success:      false,
			IsFallback:   idx > 0,
			LatencyMs:    latencyMs,
			ErrorCode:    code,
			ErrorMessage: callErr.Error(),
			RequestMeta:  encodeMeta(req.Metadata),
			SessionID:    req.SessionID,
		})
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", b.Vendor.Name, callErr))
	}

	return nil, errors.Join(attemptErrors...)
}
