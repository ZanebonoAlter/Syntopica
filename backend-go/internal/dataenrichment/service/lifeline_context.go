package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/tracing"
)

// LifelineContextService manages topic lifeline context generation for cycle A.
// Produces period-archival rows: each (topic, granularity, period) is an independent
// row. See design.md §2.1-§2.3 for the archival model.
type LifelineContextService struct {
	airouter      AirRouter
	repo          *repository.Repository
	sectionReader SectionReader
	capability    airouter.Capability
}

// NewLifelineContextService creates a new LifelineContextService.
func NewLifelineContextService(
	airouter AirRouter,
	repo *repository.Repository,
	sectionReader SectionReader,
	capability airouter.Capability,
) *LifelineContextService {
	return &LifelineContextService{
		airouter:      airouter,
		repo:          repo,
		sectionReader: sectionReader,
		capability:    capability,
	}
}

// ── Public methods ──────────────────────────────────────────────────────────

// RefreshPeriod generates context for a specific (granularity, period). It is
// the primary entry point — all other methods delegate to it.
//
// For week/month/year: reads sections within the period's date range, produces
// a fresh standalone summary (no incremental merge with old periods).
// For 'all': reads all sections since the old 'all' as_of_date and incrementally
// merges them with the old content (single rolling row, period="all").
func (s *LifelineContextService) RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshPeriod")
	defer span.End()
	switch granularity {
	case string(repository.GranularityWeek), string(repository.GranularityMonth), string(repository.GranularityYear):
		return s.refreshArchive(ctx, topicID, granularity, period, now)
	case string(repository.GranularityAll):
		return s.refreshRolling(ctx, topicID, now)
	default:
		return fmt.Errorf("unknown granularity: %s", granularity)
	}
}

// RefreshWeek generates context for the current ISO week.
func (s *LifelineContextService) RefreshWeek(ctx context.Context, topicID uint, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshWeek")
	defer span.End()
	period := FormatWeek(now)
	return s.RefreshPeriod(ctx, topicID, string(repository.GranularityWeek), period, now)
}

// RefreshMonth generates context for the current month.
func (s *LifelineContextService) RefreshMonth(ctx context.Context, topicID uint, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshMonth")
	defer span.End()
	period := FormatMonth(now)
	return s.RefreshPeriod(ctx, topicID, string(repository.GranularityMonth), period, now)
}

// RefreshYear generates context for the current year.
func (s *LifelineContextService) RefreshYear(ctx context.Context, topicID uint, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshYear")
	defer span.End()
	period := FormatYear(now)
	return s.RefreshPeriod(ctx, topicID, string(repository.GranularityYear), period, now)
}

// RefreshAll incrementally merges new sections with the old 'all' context (rolling).
func (s *LifelineContextService) RefreshAll(ctx context.Context, topicID uint, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshAll")
	defer span.End()
	return s.RefreshPeriod(ctx, topicID, string(repository.GranularityAll), "all", now)
}

// RefreshGranularity dispatches to the appropriate refresh method. Delegates
// to RefreshPeriod with the current period derived from now.
func (s *LifelineContextService) RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.RefreshGranularity")
	defer span.End()
	switch granularity {
	case string(repository.GranularityWeek):
		return s.RefreshWeek(ctx, topicID, now)
	case string(repository.GranularityMonth):
		return s.RefreshMonth(ctx, topicID, now)
	case string(repository.GranularityYear):
		return s.RefreshYear(ctx, topicID, now)
	case string(repository.GranularityAll):
		return s.RefreshAll(ctx, topicID, now)
	default:
		return fmt.Errorf("unknown granularity: %s", granularity)
	}
}

// HealMissing scans active topics for missing periods and fills them in.
// Bidirectional: finds every period that has section data but no lifeline_context
// row yet, and generates it (up to current period, no future).
func (s *LifelineContextService) HealMissing(ctx context.Context, granularity string, now time.Time, lister TopicLister) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.HealMissing")
	defer span.End()
	topicIDs, err := lister.ListActiveTopicIDs(ctx)
	if err != nil {
		return fmt.Errorf("heal missing: list topics: %w", err)
	}

	currentPeriod := PeriodForGranularity(now, granularity)

	for _, topicID := range topicIDs {
		// Which periods have section data?
		dates, err := s.sectionReader.SectionDates(ctx, topicID)
		if err != nil {
			return fmt.Errorf("heal missing: topic %d section dates: %w", topicID, err)
		}
		dataPeriods := make(map[string]bool)
		for _, d := range dates {
			dataPeriods[PeriodForGranularity(d, granularity)] = true
		}

		// Which periods already have context rows?
		existing, err := s.repo.ListTopicLifelineContextsByGranularity(ctx, topicID, granularity)
		if err != nil {
			return fmt.Errorf("heal missing: topic %d: %w", topicID, err)
		}
		have := make(map[string]bool, len(existing))
		for _, e := range existing {
			have[e.Period] = true
		}

		// Collect missing periods (have data, no context yet, not after current).
		var missing []string
		for p := range dataPeriods {
			if have[p] {
				continue
			}
			if ComparePeriods(p, currentPeriod) > 0 {
				continue
			}
			missing = append(missing, p)
		}

		// Stable order for deterministic backfill.
		sort.Strings(missing)
		for _, p := range missing {
			if err := s.refreshArchive(ctx, topicID, granularity, p, now); err != nil {
				return fmt.Errorf("heal missing: topic %d %s %s: %w", topicID, granularity, p, err)
			}
		}
	}
	return nil
}

// ArchivePrune deletes rows older than the retention policy:
//
//	week: keep last 8 weeks
//	month: keep last 12 months
//	year/all: never prune
func (s *LifelineContextService) ArchivePrune(ctx context.Context, granularity string, now time.Time) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "LifelineContextService.ArchivePrune")
	defer span.End()
	switch granularity {
	case string(repository.GranularityWeek):
		cutoff := time.Now().AddDate(0, 0, -8*7)
		cutoffPeriod := FormatWeek(cutoff)
		return s.repo.DeleteTopicLifelineContextsOlderThan(ctx, granularity, cutoffPeriod)
	case string(repository.GranularityMonth):
		cutoff := time.Now().AddDate(0, -12, 0)
		cutoffPeriod := FormatMonth(cutoff)
		return s.repo.DeleteTopicLifelineContextsOlderThan(ctx, granularity, cutoffPeriod)
	default:
		// year and all: don't prune
		return nil
	}
}

// ── TopicLister (minimal interface, avoids importing dataenrichment pkg) ───

// TopicLister returns active topic IDs. This is the same interface as
// dataenrichment.ActiveTopicLister but declared here to avoid circular imports.
type TopicLister interface {
	ListActiveTopicIDs(ctx context.Context) ([]uint, error)
}

// ── Private helpers ─────────────────────────────────────────────────────────

// refreshArchive generates fresh context for an archive period (week/month/year).
// Reads sections within the period's date range and produces a standalone summary.
func (s *LifelineContextService) refreshArchive(ctx context.Context, topicID uint, granularity, period string, now time.Time) error {
	from, to, err := ParsePeriodRange(period, granularity)
	if err != nil {
		return fmt.Errorf("refresh archive: parse period %q: %w", period, err)
	}

	sectionsText, err := s.sectionReader.ReadSections(ctx, topicID, from, to)
	if err != nil {
		return fmt.Errorf("refresh archive: read sections: %w", err)
	}

	if sectionsText == "" {
		sectionsText = "(本周期暂无相关新闻)"
	}

	content, err := s.summarizeArchive(ctx, topicID, granularity, period, sectionsText)
	if err != nil {
		return fmt.Errorf("refresh archive: summarize: %w", err)
	}

	// Clamp as_of to now: a mid-period write must not claim the future period
	// boundary (a future-dated as_of forever reads "fresh" and blinds the
	// completeness gate; fix-board-analysis-material 7.2).
	asOf := to
	if asOf.After(now) {
		asOf = now
	}
	return s.repo.UpsertTopicLifelineContext(ctx, &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       granularity,
		Period:            period,
		Content:           content,
		AsOfDate:          asOf,
		Source:            "llm_assisted",
	})
}

// SectionDates forwards the section-reader's data-bearing dates for a topic
// (the completeness gate derives which periods are worth having from them;
// satisfies FreshnessRefresher).
func (s *LifelineContextService) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	return s.sectionReader.SectionDates(ctx, topicID)
}

// refreshRolling incrementally merges new sections with the old 'all' context.
// Uses the same approach as the old refreshIncremental for 'all' granularity.
func (s *LifelineContextService) refreshRolling(ctx context.Context, topicID uint, now time.Time) error {
	gran := string(repository.GranularityAll)

	// Read old 'all' context.
	oldCtx, _ := s.repo.GetTopicLifelineContext(ctx, topicID, gran, "all")
	var oldAsOf time.Time
	if oldCtx != nil {
		oldAsOf = oldCtx.AsOfDate
	}

	sectionsText, err := s.sectionReader.ReadSections(ctx, topicID, oldAsOf, now)
	if err != nil {
		return fmt.Errorf("refresh rolling: read sections: %w", err)
	}

	oldContent := ""
	if oldCtx != nil {
		oldContent = oldCtx.Content
	}

	content, err := s.summarizeIncremental(ctx, topicID, gran, sectionsText, oldContent)
	if err != nil {
		return fmt.Errorf("refresh rolling: summarize: %w", err)
	}

	return s.repo.UpsertTopicLifelineContext(ctx, &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       gran,
		Period:            "all",
		Content:           content,
		AsOfDate:          now,
		Source:            "llm_assisted",
	})
}

// summarizeArchive produces a fresh standalone summary for one archive period.
func (s *LifelineContextService) summarizeArchive(ctx context.Context, topicID uint, granularity, period, sectionsText string) (string, error) {
	periodLabel := FormatPeriodLabel(period, granularity)

	prompt := fmt.Sprintf(
		"你是话题新闻汇总助手。下面是一个持久话题在%s的新闻报道内容，请用中文客观总结，为后续的演进分析提供素材：\n\n"+
			"1.【事件脉络】这段时间主要发生了哪些事件，按时间线列出（每条带日期）。\n"+
			"2.【关键动向】相比常规进展，这段时间出现了哪些值得注意的新变化——例如态势升级/缓和、新参与者介入、影响范围扩大、或方向反转。客观陈述事实差异，不做事态预测。\n"+
			"3.【涉及主体与领域】列出关键主体（国家/组织/公司/人物）以及波及的领域、地域、行业。\n\n"+
			"要求：只陈述新闻里的客观事实；不做事态走向判断，不做任何涨跌或预测。\n\n"+
			"--- %s 新闻内容 ---\n%s", periodLabel, periodLabel, sectionsText)

	return s.callLLM(ctx, topicID, granularity, prompt)
}

// summarizeIncremental merges new sections with old context via LLM.
func (s *LifelineContextService) summarizeIncremental(ctx context.Context, topicID uint, granularity, sectionsText, oldContent string) (string, error) {
	var prompt string
	if oldContent != "" {
		prompt = fmt.Sprintf(
			"你是话题新闻汇总助手。下面是已有历史汇总 + 新的增量新闻，请将它们合并成新的汇总。\n\n"+
				"--- 已有汇总 ---\n%s\n\n--- 增量新闻 ---\n%s\n\n"+
				"请合并输出：\n1. 话题整体发展脉络（新进展融入历史背景）\n2. 本期增量亮点\n3. 相关数据波动\n"+
				"只陈述客观事实，不做分析判断。", oldContent, sectionsText)
	} else {
		prompt = fmt.Sprintf(
			"你是话题新闻汇总助手。下面是增量新闻内容，请总结：\n\n"+
				"--- 新闻内容 ---\n%s\n\n"+
				"请输出：\n1. 话题发展脉络\n2. 主要事件与数据波动\n"+
				"只陈述客观事实，不做分析判断。", sectionsText)
	}

	return s.callLLM(ctx, topicID, granularity, prompt)
}

// callLLM invokes airouter.Chat with the proper capability, operation, and session_id.
func (s *LifelineContextService) callLLM(ctx context.Context, topicID uint, granularity, prompt string) (string, error) {
	sessionID := fmt.Sprintf("lifeline_context_%d_%s_%s", topicID, granularity, uuid8())
	temp := 0.3
	result, err := s.airouter.Chat(ctx, airouter.ChatRequest{
		Capability: s.capability,
		Operation:  "data_enrichment.summarize_context",
		SessionID:  sessionID,
		Messages: []airouter.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: &temp,
	})
	if err != nil {
		return "", fmt.Errorf("llm call: %w", err)
	}
	return result.Content, nil
}

// uuid8 returns 8 hex characters suitable for session ID uniqueness.
func uuid8() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Period enumeration (gap-fill for HealMissing) ───────────────────────────

// PeriodsBetween returns all periods strictly between latestPeriod and
// currentPeriod (exclusive of latest, inclusive of current). If latestPeriod
// is empty, returns {currentPeriod} only.
func PeriodsBetween(latestPeriod, currentPeriod, granularity string) []string {
	if latestPeriod == "" {
		// No existing period — just generate the current one.
		if currentPeriod != "" && currentPeriod != "all" {
			return []string{currentPeriod}
		}
		return nil
	}
	if latestPeriod == currentPeriod {
		return nil
	}
	if ComparePeriods(latestPeriod, currentPeriod) >= 0 {
		return nil
	}

	var result []string
	next := nextPeriodAfter(latestPeriod, granularity)
	for next != "" && ComparePeriods(next, currentPeriod) <= 0 {
		result = append(result, next)
		next = nextPeriodAfter(next, granularity)
	}

	// Sort to ensure chronological order.
	sort.Slice(result, func(i, j int) bool {
		return ComparePeriods(result[i], result[j]) < 0
	})
	return result
}

// nextPeriodAfter returns the next chronological period after the given one,
// or empty string if we can't compute it (e.g. for "all").
func nextPeriodAfter(period, granularity string) string {
	switch granularity {
	case "week":
		from, _, err := ParsePeriodRange(period, granularity)
		if err != nil {
			return ""
		}
		return FormatWeek(from.AddDate(0, 0, 7))
	case "month":
		from, _, err := ParsePeriodRange(period, granularity)
		if err != nil {
			return ""
		}
		return FormatMonth(from.AddDate(0, 1, 0))
	case "year":
		from, _, err := ParsePeriodRange(period, granularity)
		if err != nil {
			return ""
		}
		return FormatYear(from.AddDate(1, 0, 0))
	default:
		return ""
	}
}
