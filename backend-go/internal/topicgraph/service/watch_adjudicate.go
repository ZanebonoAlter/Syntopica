package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// Materialized-track LLM adjudication (watch-materialize-llm-adjudication).
//
// The recall layers stay untouched (keyword DNF matching / sentence vector
// retrieval) — they only guarantee recall. This layer is the precision pass:
// one batch AI call per watch over the day's candidate articles, deciding
// which articles actually fit the watch's tracking intent. Reuses the label
// hint track's chat pattern (JSONMode + JSONSchema + hallucinated-ID
// filtering, temperature 0.1, CapabilityDigestPolish).
//
// Degradation contract (spec 物化失败降级): any adjudication failure
// (call error / bad JSON / all-hallucinated verdicts) degrades to the recall
// result — caller keeps the full candidate set with default confidence and
// the fallback section title. Never blocks the daily report.

// WatchMaterializeConfig holds the adjudication-layer parameters
// (ai_settings-overridable, same mechanism as WatchSentenceConfig).
type WatchMaterializeConfig struct {
	// Enabled toggles the adjudication layer (ai_settings:
	// watch_materialize_llm_filter_enabled, default true). Disabled = the
	// pre-adjudication behavior: recall result aggregated as-is, zero AI.
	Enabled bool
	// CandidateLimit caps the candidates sent to one adjudication call
	// (ai_settings: watch_materialize_candidate_limit, default 40). Beyond
	// the cap candidates are truncated by article id with a warning — the
	// recall layer itself is never limited by this.
	CandidateLimit int
}

// DefaultWatchMaterializeConfig — enabled, 40 candidates per call (design D6).
func DefaultWatchMaterializeConfig() WatchMaterializeConfig {
	return WatchMaterializeConfig{Enabled: true, CandidateLimit: 40}
}

// LoadWatchMaterializeConfig loads the adjudication config from ai_settings,
// falling back to defaults for absent/invalid rows.
func LoadWatchMaterializeConfig(db *gorm.DB) WatchMaterializeConfig {
	cfg := DefaultWatchMaterializeConfig()
	if db == nil {
		return cfg
	}
	type kv struct {
		Key   string
		Value string
	}
	var rows []kv
	if err := db.Table("ai_settings").
		Where("key IN ?", []string{"watch_materialize_llm_filter_enabled", "watch_materialize_candidate_limit"}).
		Find(&rows).Error; err != nil {
		return cfg
	}
	for _, r := range rows {
		switch r.Key {
		case "watch_materialize_llm_filter_enabled":
			// Only the literal "false" disables — anything else (typos, empty)
			// keeps the default-enabled behavior.
			if r.Value == "false" {
				cfg.Enabled = false
			}
		case "watch_materialize_candidate_limit":
			if v, err := parseIntPositive(r.Value); err == nil && v > 0 {
				cfg.CandidateLimit = v
			}
		}
	}
	return cfg
}

// watchAdjudicateSummaryRunes caps each candidate's title+summary text sent
// to the adjudication call (design D2: 200 runes, prompt-explosion guard).
const watchAdjudicateSummaryRunes = 200

// watchSectionTitleRunes caps the LLM-generated section title (cluster_label
// column sanity; the fallback names are naturally short).
const watchSectionTitleRunes = 200

// watchAdjudicationResult is the parsed, hallucination-filtered outcome of
// one watch's adjudication call.
type watchAdjudicationResult struct {
	// Confidence maps article_id → adjudication confidence for the
	// related=true verdicts only. Absent ids were judged unrelated.
	Confidence map[uint]float64
	// Title is the LLM's same-batch section title; empty means "use fallback".
	Title string
}

// adjudicateWatchArticles runs one batch adjudication call for a single
// watch over its candidate articles. intent is the human-readable tracking
// intent (sentence query, or the keyword expression with a track tag).
// Returns an error on call/parse failure — the caller degrades to the
// recall result (never blocks). An empty candidate list short-circuits with
// an empty result (callers skip before reaching here, but stay safe).
func adjudicateWatchArticles(
	ctx context.Context,
	boardID, watchID uint,
	intent string,
	candidates []repository.WatchScanArticle,
	cfg WatchMaterializeConfig,
	chat watchChatFunc,
) (*watchAdjudicationResult, error) {
	if len(candidates) == 0 {
		return &watchAdjudicationResult{Confidence: map[uint]float64{}}, nil
	}
	sent := candidates
	if len(sent) > cfg.CandidateLimit {
		sent = sent[:cfg.CandidateLimit]
		logging.Warnf("watch-adjudicate: watch %d candidates %d exceed limit %d — truncated by article id (recall layer unaffected)",
			watchID, len(candidates), cfg.CandidateLimit)
	}

	temperature := 0.1
	maxTokens := 4096
	result, err := chat(ctx, airouter.ChatRequest{
		Operation:  "watch_materialize.adjudicate",
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: watchAdjudicateSystemPrompt()},
			{Role: "user", Content: buildWatchAdjudicatePrompt(intent, sent)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"verdicts": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"article_id": {Type: "integer", Description: "候选文章 ID"},
							"related":    {Type: "boolean", Description: "是否贴合追踪意图"},
							"confidence": {Type: "number", Description: "贴合置信度 0~1"},
							"reason":     {Type: "string", Description: "一句话裁决理由"},
						},
						Required: []string{"article_id", "related", "confidence"},
					},
				},
				"section_title": {Type: "string", Description: "基于全部贴合文章的板块当日标题，15字内中文"},
			},
			Required: []string{"verdicts"},
		},
		Metadata: map[string]any{
			"operation":       "watch_materialize_adjudicate",
			"board_id":        boardID,
			"watch_id":        watchID,
			"candidate_count": len(sent),
			"prompt_version":  watchAdjudicatePromptVersion,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("adjudication AI call failed: %w", err)
	}

	parsed, err := parseWatchAdjudication(result.Content, sent)
	if err != nil {
		return nil, err
	}
	logging.Infof("watch-adjudicate: watch %d — %d candidates adjudicated, %d kept, title=%q",
		watchID, len(sent), len(parsed.Confidence), parsed.Title)
	return parsed, nil
}

// watchAdjudicatePromptVersion versions the adjudication prompt wording for
// observability (metadata on every call; bump on any criteria change).
const watchAdjudicatePromptVersion = "1"

// watchAdjudicateSystemPrompt is the adjudication criteria (user-calibrated,
// spec-backed): intent focus + substantial causal chain.
func watchAdjudicateSystemPrompt() string {
	return `你是一名新闻编辑，负责把关用户追踪板块的内容质量。给定用户的追踪意图和候选文章列表，判断每篇文章是否应进入该追踪板块。

裁决标准（两层都要考虑）：
1. 意图重心：文章须贴合追踪意图的重心与限定词，而不只是字面出现关键词。例如意图是「美伊形势对市场影响」时，与市场无关联的纯军事战报、人道新闻不算贴合。
2. 实质因果链：与追踪主题有直接因果关联的事件算贴合。例如美伊冲突导致沙特船只遇袭，是美伊形势的直接波及，应判贴合；仅平行存在、或顺带提及关键词但主题无关的文章不算贴合。

规则：
- 只把确实贴合的文章判 related=true，宁缺毋滥，不强凑
- confidence 取 0~1，表达你对贴合程度的把握
- 基于全部贴合文章，为板块拟一个当日中文标题（15字内，概括当日重点），标题只能概括文章中实际出现的内容，不得编造事件、数字或因果`
}

// buildWatchAdjudicatePrompt renders the per-article candidate list.
func buildWatchAdjudicatePrompt(intent string, candidates []repository.WatchScanArticle) string {
	var sb strings.Builder
	sb.WriteString("## 追踪意图\n\n")
	sb.WriteString(intent)
	sb.WriteString("\n\n## 候选文章\n\n")
	for _, a := range candidates {
		fmt.Fprintf(&sb, "- [id:%d] %s", a.ID, a.Title)
		if s := truncateRunes(a.Summary, watchAdjudicateSummaryRunes); s != "" {
			fmt.Fprintf(&sb, "｜%s", s)
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\n输出 JSON：{\"verdicts\":[{\"article_id\":1,\"related\":true,\"confidence\":0.9,\"reason\":\"一句话理由\"},...],\"section_title\":\"…\"}\n")
	return sb.String()
}

type rawWatchVerdict struct {
	ArticleID  uint    `json:"article_id"`
	Related    bool    `json:"related"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type rawWatchAdjudication struct {
	Verdicts     []rawWatchVerdict `json:"verdicts"`
	SectionTitle string            `json:"section_title"`
}

// parseWatchAdjudication parses the LLM response and filters hallucinated
// article ids against the sent candidate set. A response whose verdicts
// reference no valid article at all is treated as a parse failure (caller
// degrades) — it is indistinguishable from a garbage response.
func parseWatchAdjudication(content string, sent []repository.WatchScanArticle) (*watchAdjudicationResult, error) {
	content = jsonutil.SanitizeLLMJSON(content)
	var raw rawWatchAdjudication
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse adjudication JSON: %w", err)
	}

	valid := make(map[uint]bool, len(sent))
	for _, a := range sent {
		valid[a.ID] = true
	}
	out := &watchAdjudicationResult{Confidence: map[uint]float64{}}
	for _, v := range raw.Verdicts {
		if !valid[v.ArticleID] || !v.Related {
			continue // hallucinated id or judged unrelated
		}
		conf := v.Confidence
		if conf <= 0 || conf > 1 {
			conf = 1 // clamp garbage confidence to a sane value
		}
		out.Confidence[v.ArticleID] = conf
	}
	if len(raw.Verdicts) > 0 && len(out.Confidence) == 0 {
		// All verdicts were unrelated or hallucinated. Both-zero-related is a
		// legal "no fitting article today" outcome ONLY when verdicts were
		// valid ids; hallucination-only is garbage. Distinguish:
		anyValidID := false
		for _, v := range raw.Verdicts {
			if valid[v.ArticleID] {
				anyValidID = true
				break
			}
		}
		if !anyValidID {
			return nil, fmt.Errorf("adjudication response contains no valid article ids")
		}
		// Valid ids, all related=false → legal full-reject; empty Confidence.
	}
	out.Title = strings.TrimSpace(truncateRunes(raw.SectionTitle, watchSectionTitleRunes))
	return out, nil
}

// parseIntPositive parses a positive int (ai_settings helper; empty/invalid
// returns an error so callers keep the default).
func parseIntPositive(s string) (int, error) {
	var v int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}

// filterAdjudicated keeps only the articles the adjudication marked related
// (present in Confidence). Call ONLY with a non-nil adj — nil means
// "degraded, keep the recall result" and is the caller's branch, not this
// filter's.
func filterAdjudicated(articles []repository.WatchScanArticle, adj *watchAdjudicationResult) []repository.WatchScanArticle {
	kept := make([]repository.WatchScanArticle, 0, len(articles))
	for _, a := range articles {
		if _, ok := adj.Confidence[a.ID]; ok {
			kept = append(kept, a)
		}
	}
	return kept
}

// applyThreadConfidence overrides each materialized thread's Confidence with
// its adjudication verdict. Materialized threads carry exactly one related
// article id (RelatedArticleIDs = "[id]"); a nil adj (degraded/disabled)
// leaves the default 1.0 untouched.
func applyThreadConfidence(threads []repository.DailyReportThread, adj *watchAdjudicationResult) {
	if adj == nil {
		return
	}
	for i := range threads {
		ids := parseUintJSONArray(threads[i].RelatedArticleIDs)
		if len(ids) == 0 {
			continue
		}
		if c, ok := adj.Confidence[ids[0]]; ok {
			threads[i].Confidence = c
		}
	}
}

// parseUintJSONArray parses a repository.JSON "[1,2]" into []uint. NULL /
// malformed input yields nil (best-effort: threads are built by us).
func parseUintJSONArray(raw repository.JSON) []uint {
	if len(raw) == 0 {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}
