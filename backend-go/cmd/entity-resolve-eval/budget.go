package main

// Budget — the four mechanically-derived budget lines (design D11). No
// made-up multipliers: every line derives from the ACTUAL pipeline shape.
//
//	① 文章抽取调用次数 = 实际抽取文章数（影子第一轮，同次调用已含
//	   subject_desc，无额外调用）
//	② 二判调用次数 = 实际提及数 + 实际发生的有限重试（上限口径 =
//	   提及数 × 每提及尝试上限；实际以运行时计数为准，预算取上限）
//	③ embedding 提交文本数 = 两路查询文本（提及数 ≤ 2 条/提及，短说明
//	   为空时仅 1 条）+ 实体创建/别名登记/材料读取（本实现里创建复用
//	   查询向量、别名与材料不 embed → 0；上限口径仍按 2 条/提及计，
//	   含重复发送与模型降级重复）
//	④ HTTP 次数 = 发送前计数口径（send-guard 在 provider client 之前
//	   拦截计数；不支持批量的 embed 每条输入一次 HTTP，结构化输出降级
//	   链与客户端多次尝试一并计入——所以 ④ 是运行时的真实闸门，
//	   ①–③ 是计划口径）

import (
	"context"
	"fmt"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// BudgetPlan is the frozen pre-run plan (upper bounds) plus the runtime
// counters the guard enforces.
type BudgetPlan struct {
	ExtractionCalls  int `json:"extraction_calls"`   // ① 按实际抽取文章数
	SecondRoundCalls int `json:"second_round_calls"` // ② 提及数 × 尝试上限（上界）
	EmbedTexts       int `json:"embed_texts"`        // ③ 提交文本上界
	HTTPRequests     int `json:"http_requests"`      // ④ 发送前计数的硬闸门
}

// secondRoundMaxAttempts mirrors entity.ResolverConfig.MaxAttempts (kept as
// a literal here so the plan tool does not import the whole resolver).
const secondRoundMaxAttempts = 2

// MechanicalBudget derives the four lines from the actual sample list.
// extractionArticles = distinct article titles (first round runs per
// article/section; plan line uses the article count as the lower bound of
// real extraction calls actually performed — the runner counts the real
// number as it goes).
func MechanicalBudget(samples []EvalSample) BudgetPlan {
	articles := map[string]bool{}
	mentions := 0
	for _, s := range samples {
		if !s.Disputed {
			articles[s.ArticleTitle] = true
			mentions++
		}
	}
	embedTexts := 0
	for _, s := range samples {
		if s.Disputed {
			continue
		}
		embedTexts++ // name path (always exists)
		if s.SubjectDesc != "" {
			embedTexts++ // name+desc path (only when a description was produced)
		}
	}
	return BudgetPlan{
		ExtractionCalls:  len(articles),
		SecondRoundCalls: mentions * secondRoundMaxAttempts,
		EmbedTexts:       embedTexts,
		// HTTP hard gate: worst case = extraction calls (structured-fallback
		// chains included) + second-round upper bound + one HTTP per embed
		// text (the router embeds one input per HTTP outside the cache
		// whitelist) + a small routing-probe margin.
		HTTPRequests: len(articles)*3 + mentions*secondRoundMaxAttempts*3 + embedTexts*2,
	}
}

// BudgetReport is the post-run actuals line-up (真实阶段机械列示).
//
// 每个「实际」字段的构成公式（机械可复算，design D11 ②③④；计数器均在
// SendGuard 内、发送前自增，被拒的调用不计）：
//
//	ActualExtraction  = guard.ChatCount() − ActualSecondRound
//	                    二判之外经 guard 发出的 chat 尝试只可能来自文章抽取；
//	                    回放冻结样本的跑法不重跑第一轮 → 该字段为 0（不是漏接线，
//	                    是「实际抽取调用数」的真实值）；现场重跑第一轮时为正。
//	ActualSecondRound = Σ records.Attempts（每次尝试都是一次真实 chat 尝试）
//	ActualEmbedTexts  = guard.EmbedTexts()（提交文本数，含降级/重试导致的重复提交）
//	ActualHTTP        = guard.HTTPCount()（chat 尝试数 + embed 调用数，发送前计数）
type BudgetReport struct {
	Plan              BudgetPlan `json:"plan"`
	ActualExtraction  int        `json:"actual_extraction_calls"`
	ActualSecondRound int        `json:"actual_second_round_calls"`
	ActualEmbedTexts  int        `json:"actual_embed_texts_submitted"`
	ActualHTTP        int        `json:"actual_http_requests_pre_send"`
	Rejected          int        `json:"rejected_before_send"`
}

// ---- send-guard: intercept BEFORE the wire (D11 hard budget) ----

// SendGuard wraps provider clients so every REAL attempt is counted and
// rejected locally BEFORE any HTTP leaves the process. Registered ONLY at
// the evaluation entry (router.RegisterClient) — the production routing
// architecture is untouched. When counting cannot be established the runner
// pre-blocks instead of flying blind (FailClosed below).
type SendGuard struct {
	label         string
	plan          BudgetPlan
	httpCount     int
	chatCount     int
	embedTexts    int
	rejected      int
	allowedModels map[string]bool
	inner         airouter.ProviderClient
}

// NewSendGuard builds the guard around an inner provider client. allowedModels
// empty = no model whitelist (count-only mode).
func NewSendGuard(label string, plan BudgetPlan, allowedModels []string, inner airouter.ProviderClient) *SendGuard {
	g := &SendGuard{label: label, plan: plan, allowedModels: map[string]bool{}, inner: inner}
	for _, m := range allowedModels {
		g.allowedModels[m] = true
	}
	return g
}

// Rejections returns how many attempts were locally rejected (never sent).
func (g *SendGuard) Rejections() int { return g.rejected }

// HTTPCount returns attempts that actually went out (counted pre-send).
func (g *SendGuard) HTTPCount() int { return g.httpCount }

// ChatCount returns chat attempts that actually went out (counted pre-send).
// Used to derive the actual first-round extraction count: every chat attempt
// beyond the recorded second-round attempts is an extraction call.
func (g *SendGuard) ChatCount() int { return g.chatCount }

// EmbedTexts returns embedding input texts actually submitted (pre-send
// count, duplicates from retries/degrades included) — budget line ③ actual.
func (g *SendGuard) EmbedTexts() int { return g.embedTexts }

func (g *SendGuard) reject(reason string) error {
	g.rejected++
	return fmt.Errorf("budget guard[%s]: 发送前拦截：%s（本地拒绝，无 HTTP、无 token 消耗）", g.label, reason)
}

// Chat implements airouter.ProviderClient. One call = one real provider
// attempt = counted BEFORE the inner client runs; over-budget → rejected
// locally (airouter still writes its failure audit row — observable, fine).
func (g *SendGuard) Chat(ctx context.Context, vendor models.AIVendor, model models.AIModel, effLevel string, req airouter.ChatRequest) (*airouter.ChatResponse, []airouter.StructuredAttempt, error) {
	if len(g.allowedModels) > 0 && !g.allowedModels[model.Name] {
		return nil, nil, g.reject(fmt.Sprintf("模型 %s 不在白名单（跨模型/降级请求不发出）", model.Name))
	}
	if g.httpCount >= g.plan.HTTPRequests {
		return nil, nil, g.reject(fmt.Sprintf("HTTP 次数已达上限 %d", g.plan.HTTPRequests))
	}
	g.httpCount++ // pre-send count: this attempt WILL hit the wire
	g.chatCount++
	return g.inner.Chat(ctx, vendor, model, effLevel, req)
}

// Embed implements airouter.ProviderClient. Inputs are counted pre-send
// (submit-text budget) plus one HTTP per call batch.
func (g *SendGuard) Embed(ctx context.Context, vendor models.AIVendor, model models.AIModel, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	if len(g.allowedModels) > 0 && !g.allowedModels[model.Name] {
		return nil, g.reject(fmt.Sprintf("模型 %s 不在白名单", model.Name))
	}
	if g.embedTexts+len(req.Input) > g.plan.EmbedTexts {
		return nil, g.reject(fmt.Sprintf("embedding 提交文本数将超上限 %d（已 %d + 本次 %d）", g.plan.EmbedTexts, g.embedTexts, len(req.Input)))
	}
	if g.httpCount >= g.plan.HTTPRequests {
		return nil, g.reject(fmt.Sprintf("HTTP 次数已达上限 %d", g.plan.HTTPRequests))
	}
	g.embedTexts += len(req.Input)
	g.httpCount++ // pre-send count
	return g.inner.Embed(ctx, vendor, model, req)
}

// EnsureGuardRegistered pre-blocks the run when the send-guard cannot be
// wired (spec: 无法建立发送前计数能力则在发起任何请求前预先阻断).
func EnsureGuardRegistered(registered bool) error {
	if !registered {
		return fmt.Errorf("发送前计数能力未建立（guard 未注册）：按协议预先阻断，不发起任何请求")
	}
	return nil
}
