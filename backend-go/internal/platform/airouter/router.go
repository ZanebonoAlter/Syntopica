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
	route, providers, err := r.store.LoadRouteWithProviders(req.Capability)
	if err != nil {
		return nil, err
	}

	sem := r.getSemaphore(req.Capability, route)
	if err := r.acquireSem(ctx, sem); err != nil {
		return nil, err
	}
	defer r.releaseSem(sem)

	var attemptErrors []error
	for idx, provider := range providers {
		client := r.clients[provider.ProviderType]
		if client == nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("provider type %s unsupported", provider.ProviderType))
			continue
		}

		start := time.Now()
		chatResp, callErr := client.Chat(ctx, provider, req)
		latencyMs := int(time.Since(start).Milliseconds())
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
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:       req.Operation,
				Capability:      string(req.Capability),
				RouteName:       route.Name,
				ProviderName:    provider.Name,
				Model:           provider.Model,
				Success:         true,
				IsFallback:      idx > 0,
				LatencyMs:       latencyMs,
				Prompt:          formatMessages(req.Messages),
				RequestMeta:     encodeMeta(req.Metadata),
				ResponseSnippet: truncateSnippet(chatResp.Content),
				TokenUsage:      encodeTokenUsage(chatResp.Usage),
				SessionID:       req.SessionID,
			})

			return &ChatResult{
				Content:      chatResp.Content,
				ProviderID:   provider.ID,
				ProviderName: provider.Name,
				RouteName:    route.Name,
				UsedFallback: idx > 0,
				AttemptCount: idx + 1,
				Usage:        chatResp.Usage,
			}, nil
		}

		providerErr := &ProviderError{}
		code := "provider_error"
		if errors.As(callErr, &providerErr) {
			if providerErr.Code != "" {
				code = providerErr.Code
			}
		}
		r.store.LogCall(ctx, &models.AICallLog{
			Operation:       req.Operation,
			Capability:      string(req.Capability),
			RouteName:       route.Name,
			ProviderName:    provider.Name,
			Model:           provider.Model,
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
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", provider.Name, callErr))
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

func (r *Router) ResolvePrimaryProvider(capability Capability) (*models.AIProvider, *models.AIRoute, error) {
	return r.store.ResolvePrimaryProvider(capability)
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

	route, providers, err := r.store.LoadRouteWithProviders(capability)
	if err != nil {
		return nil, err
	}

	// Cache lookup happens BEFORE the semaphore: a hit is a local DB read and
	// must neither queue behind real provider calls nor consume a slot.
	// Operations outside embeddingCacheOperations skip the lookup entirely
	// (their rows are never written, so lookup would always miss) and go
	// straight to the provider below.
	// The cache key intentionally uses the route provider's Model, not
	// req.Model: no call site sets req.Model today, and the client layer
	// falls back to provider.Model when req.Model is empty. If a future call
	// site starts setting req.Model, normalize the effective model before it
	// participates in the key, otherwise the cached Model field observability
	// becomes inaccurate.
	var lookupStart time.Time
	var cacheKey string
	var cached *models.AIEmbeddingCache
	var lookupErr error
	if embeddingCacheable(req.Operation) {
		lookupStart = time.Now()
		cacheKey = embeddingCacheKey(providers[0].Model, req.Input)
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
				ProviderName: providers[0].Name,
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
				Provider:   providers[0].Name,
			}, nil
		}
	}

	sem := r.getSemaphore(capability, route)
	if err := r.acquireSem(ctx, sem); err != nil {
		return nil, err
	}
	defer r.releaseSem(sem)

	var attemptErrors []error
	for idx, provider := range providers {
		client := r.clients[provider.ProviderType]
		if client == nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("provider type %s unsupported", provider.ProviderType))
			continue
		}

		start := time.Now()
		res, callErr := client.Embed(ctx, provider, req)
		latencyMs := int(time.Since(start).Milliseconds())
		if callErr == nil {
			r.store.LogCall(ctx, &models.AICallLog{
				Operation:    req.Operation,
				Capability:   string(capability),
				RouteName:    route.Name,
				ProviderName: provider.Name,
				Model:        provider.Model,
				Success:      true,
				IsFallback:   idx > 0,
				LatencyMs:    latencyMs,
				RequestMeta:  encodeMeta(req.Metadata),
				SessionID:    req.SessionID,
			})
			r.saveEmbeddingCache(req, provider.Model, res)
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
			ProviderName: provider.Name,
			Model:        provider.Model,
			Success:      false,
			IsFallback:   idx > 0,
			LatencyMs:    latencyMs,
			ErrorCode:    code,
			ErrorMessage: callErr.Error(),
			RequestMeta:  encodeMeta(req.Metadata),
			SessionID:    req.SessionID,
		})
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", provider.Name, callErr))
	}

	return nil, errors.Join(attemptErrors...)
}
