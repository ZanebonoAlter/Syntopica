package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// sessionCtxKey is the context key for the daily report SessionID.
type sessionCtxKey struct{}

// WithSessionID injects a daily report SessionID into the context.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.WithSessionID")
	defer span.End()
	return context.WithValue(ctx, sessionCtxKey{}, sessionID)
}

// SessionIDFromContext extracts the daily report SessionID from context.
// Returns empty string when no SessionID is present.
func SessionIDFromContext(ctx context.Context) string {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.SessionIDFromContext")
	defer span.End()
	if ctx == nil {
		return ""
	}
	sid, _ := ctx.Value(sessionCtxKey{}).(string)
	return sid
}

// GenerateAndSaveReport is the unified entry point for daily report generation.
// It replaces the older pattern: GenerateDailyReport → SaveReport.
// After saving the report (and its sections), it runs EvaluateWatchHits
// OUTSIDE the SaveReport transaction as a read-only overlay.
func GenerateAndSaveReport(ctx context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.GenerateAndSaveReport")
	defer span.End()
	// Generate a SessionID BEFORE the LLM calls so all calls within
	// this orchestration share the same session_id. boardID + uuid8
	// gives a unique key even before SaveReport fills report.ID.
	sessionID := fmt.Sprintf("daily_report_%d_%s", boardID, uuid.New().String()[:8])
	ctx = WithSessionID(ctx, sessionID)

	report, sections, threadBatches, err := GenerateDailyReport(ctx, boardID, date)
	if err != nil {
		return nil, fmt.Errorf("generate daily report: %w", err)
	}
	if report == nil {
		return nil, nil
	}

	if err := repository.Repo.SaveReport(report, sections, threadBatches); err != nil {
		return nil, fmt.Errorf("save daily report: %w", err)
	}

	// EvaluateWatchHits runs AFTER SaveReport (outside its transaction).
	// Failure is swallowed — hits are a read-only overlay that SHALL NOT
	// block daily report generation.
	if watchErr := EvaluateWatchHits(ctx, boardID, report, sections); watchErr != nil {
		logging.Warnf("daily-report: watch hit evaluation failed for board %d report %d: %v",
			boardID, report.ID, watchErr)
	}

	return report, nil
}

// watchChatFunc mirrors airouter.Router.Chat so tests can inject a mock
// instead of hitting the real AI provider pipeline.
type watchChatFunc func(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error)

// EvaluateWatchHits evaluates every active hint-track watch (label: batch
// AI; keyword: pure text) against all sections of a newly-saved daily
// report. Materialized tracks (keyword_topic / sentence_topic) are skipped —
// they own sections and SHALL NOT produce hint hits. Detected matches are
// written as TopicWatchHit rows. This function SHALL NOT change any section's
// persistent_topic_id or any topic's consecutive_hits. On failure it returns
// an error that the caller SHOULD swallow (log.Warnf + continue).
func EvaluateWatchHits(ctx context.Context, boardID uint, report *repository.BoardDailyReport, sections []repository.DailyReportSection) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.EvaluateWatchHits")
	defer span.End()
	return evaluateWatchHitsWithChat(ctx, boardID, report, sections, airouter.NewRouter().Chat)
}

// evaluateWatchHitsWithChat is the testable core of EvaluateWatchHits.
// It accepts a chat function so tests can inject a mock instead of hitting
// the real AI provider.
//
// Tri-track split (watch-materialize-llm-adjudication): active watches split
// by type — label watches keep the batch-AI single-shot path; keyword watches
// are matched by pure text (matchKeywordSections, zero AI); materialized
// tracks (keyword_topic / sentence_topic) are skipped entirely: they own
// their daily sections and per spec SHALL NOT produce hint hits. Hits from
// label+keyword tracks merge into one batch upsert.
func evaluateWatchHitsWithChat(
	ctx context.Context,
	boardID uint,
	report *repository.BoardDailyReport,
	sections []repository.DailyReportSection,
	chat watchChatFunc,
) error {
	watches, err := repository.Repo.ListActiveWatchesByBoard(boardID)
	if err != nil {
		return fmt.Errorf("list active watches: %w", err)
	}
	if len(watches) == 0 {
		return nil // nothing to evaluate
	}
	if len(sections) == 0 {
		return nil
	}

	var labelWatches, keywordWatches []repository.BoardTopicWatch
	for _, w := range watches {
		switch w.Type {
		case repository.WatchTypeKeyword:
			keywordWatches = append(keywordWatches, w)
		case repository.WatchTypeLabel:
			labelWatches = append(labelWatches, w)
		default:
			// Materialized tracks (keyword_topic / sentence_topic) own their
			// own sections — spec: 物化轨 SHALL NOT 产生命中提示记录. Skipping
			// them entirely also avoids wasted AI calls on watches that already
			// surface their content as sections.
			continue
		}
	}

	var allHits []repository.TopicWatchHit

	// Materialized sections are invisible to the label track too (spec:
	// 物化 section 不被提示轨扫描命中) — a keyword section trivially containing
	// its own keyword is signal-free noise.
	hintSections := make([]repository.DailyReportSection, 0, len(sections))
	for _, s := range sections {
		if s.LaneTier == LaneTierWatchKeyword || s.LaneTier == LaneTierWatchSentence {
			continue
		}
		hintSections = append(hintSections, s)
	}
	if len(hintSections) == 0 {
		return nil
	}

	// ── Label track: existing batch AI single-shot (unchanged behavior) ──
	if len(labelWatches) > 0 {
		labelHits, labelErr := evaluateLabelWatchHitsWithChat(ctx, boardID, report, hintSections, labelWatches, chat)
		if labelErr != nil {
			return labelErr
		}
		allHits = append(allHits, labelHits...)
	}

	// ── Keyword track: pure text matching, zero AI calls ──
	if len(keywordWatches) > 0 {
		keywordHits, keywordErr := evaluateKeywordWatchHits(ctx, report, keywordWatches)
		if keywordErr != nil {
			return keywordErr
		}
		allHits = append(allHits, keywordHits...)
	}

	if len(allHits) == 0 {
		return nil
	}

	// Batch upsert hits — silently skip duplicates on (watch_id, section_id, report_id)
	// so the daily report is never blocked by a duplicate AI response (or by a
	// hit already written by the keyword instant match at creation time).
	if err := repository.Repo.DB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "watch_id"}, {Name: "section_id"}, {Name: "report_id"}},
		DoNothing: true,
	}).Create(&allHits).Error; err != nil {
		return fmt.Errorf("write watch hits: %w", err)
	}

	logging.Infof("daily-report: wrote %d watch hits (label watches=%d, keyword watches=%d) for board %d report %d",
		len(allHits), len(labelWatches), len(keywordWatches), boardID, report.ID)
	return nil
}

// evaluateLabelWatchHitsWithChat is the pre-dual-track AI matching logic,
// extracted unchanged: one batch call over all label watches + the report's
// sections, hallucinated IDs filtered.
func evaluateLabelWatchHitsWithChat(
	ctx context.Context,
	boardID uint,
	report *repository.BoardDailyReport,
	sections []repository.DailyReportSection,
	labelWatches []repository.BoardTopicWatch,
	chat watchChatFunc,
) ([]repository.TopicWatchHit, error) {
	// Build a set of valid IDs to filter AI hallucinations.
	validWatchIDs := make(map[uint]bool, len(labelWatches))
	for _, w := range labelWatches {
		validWatchIDs[w.ID] = true
	}
	validSectionIDs := make(map[uint]bool, len(sections))
	for _, s := range sections {
		validSectionIDs[s.ID] = true
	}

	prompt := buildWatchHitPrompt(labelWatches, sections)

	temperature := 0.1
	maxTokens := 4096
	result, err := chat(ctx, airouter.ChatRequest{
		Operation:  "topic_watch.evaluate",
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: watchHitSystemPrompt()},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"hits": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"watch_id":   {Type: "integer", Description: "关注标记 ID"},
							"section_id": {Type: "integer", Description: "日报节 ID"},
							"reason":     {Type: "string", Description: "一句话命中理由"},
						},
						Required: []string{"watch_id", "section_id", "reason"},
					},
				},
			},
			Required: []string{"hits"},
		},
		Metadata: map[string]any{
			"operation":     "daily_report_watch_hit",
			"watch_count":   len(labelWatches),
			"section_count": len(sections),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("AI watch hit call failed: %w", err)
	}

	logging.Infof("daily-report: watch hit LLM response length=%d for board %d report %d",
		len(result.Content), boardID, report.ID)

	hits, err := parseWatchHitResponse(result.Content, validWatchIDs, validSectionIDs, report)
	if err != nil {
		return nil, fmt.Errorf("parse watch hit response: %w", err)
	}
	return hits, nil
}

// evaluateKeywordWatchHits matches every keyword watch against the report's
// persisted threads text (title+summary, lowercased). Deterministic — no AI
// involved; the reason is the mechanical 含关键字『XX』 text.
func evaluateKeywordWatchHits(
	ctx context.Context,
	report *repository.BoardDailyReport,
	keywordWatches []repository.BoardTopicWatch,
) ([]repository.TopicWatchHit, error) {
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.evaluateKeywordWatchHits")
	defer span.End()

	// Sections + threads text are re-read from the DB: EvaluateWatchHits runs
	// AFTER SaveReport (threads persisted there), and the in-memory sections
	// slice does not carry thread text.
	texts, err := repository.Repo.ListWatchSectionTextsByReport(report.ID)
	if err != nil {
		return nil, fmt.Errorf("list section texts: %w", err)
	}
	if len(texts) == 0 {
		return nil, nil
	}

	var hits []repository.TopicWatchHit
	for _, w := range keywordWatches {
		for _, h := range matchKeywordSections(w.Label, texts) {
			hits = append(hits, repository.TopicWatchHit{
				WatchID:    w.ID,
				SectionID:  h.SectionID,
				ReportID:   report.ID,
				PeriodDate: h.PeriodDate,
				Reason:     buildKeywordHitReason(h.MatchedWords),
			})
		}
	}
	return hits, nil
}

func watchHitSystemPrompt() string {
	return `你是一名新闻分析助手。你的任务是：给定若干个"关注标记"和一期日报的所有"节(section)"，判断哪些节与哪些关注标记内容相关。

规则：
1. 每个关注标记代表用户关心的一个话题方向
2. 对每个关注标记，判断哪些节的内容与之相关（节可能匹配多个关注）
3. 只记录确实相关的命中，不要强凑
4. 每个命中需要给出一句话的理由，说明为什么这个节与这个关注相关`
}

func buildWatchHitPrompt(watches []repository.BoardTopicWatch, sections []repository.DailyReportSection) string {
	var sb strings.Builder
	sb.WriteString("## 关注标记\n\n")
	for _, w := range watches {
		fmt.Fprintf(&sb, "- [id:%d] %s\n", w.ID, w.Label)
	}
	sb.WriteString("\n## 日报节列表\n\n")
	for _, s := range sections {
		fmt.Fprintf(&sb, "- [section_id:%d] %s\n", s.ID, s.ClusterLabel)
	}
	sb.WriteString("\n请输出命中列表，格式：{\"hits\":[{\"watch_id\":1,\"section_id\":101,\"reason\":\"原因\"},...]}\n")
	return sb.String()
}

type rawWatchHit struct {
	WatchID   uint   `json:"watch_id"`
	SectionID uint   `json:"section_id"`
	Reason    string `json:"reason"`
}

type rawWatchHitResponse struct {
	Hits []rawWatchHit `json:"hits"`
}

func parseWatchHitResponse(content string, validWatchIDs, validSectionIDs map[uint]bool, report *repository.BoardDailyReport) ([]repository.TopicWatchHit, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw rawWatchHitResponse
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse watch hit JSON: %w", err)
	}

	var result []repository.TopicWatchHit
	for _, h := range raw.Hits {
		if !validWatchIDs[h.WatchID] || !validSectionIDs[h.SectionID] {
			continue // filter AI hallucinations
		}
		result = append(result, repository.TopicWatchHit{
			WatchID:    h.WatchID,
			SectionID:  h.SectionID,
			ReportID:   report.ID,
			PeriodDate: report.PeriodDate,
			Reason:     strings.TrimSpace(h.Reason),
		})
	}
	return result, nil
}
