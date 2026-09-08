package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// Keyword-track article materialization (watch-materialized-topic design §D2).
//
// Unlike the hint track (which matches section threads text), the
// keyword_topic track scans the day's ARTICLES themselves — including
// articles the tag system never covered — and aggregates the hits into an
// ephemeral section appended to the daily report:
//
//	fetch day's unarchived articles (SQL, summary-preference coalesce)
//	  → match in memory with the existing DNF matcher (title+summary text)
//	  → one thread per hit article (mechanical, zero AI)
//	  → one fixed-name section (lane_tier=watch_keyword, no persistent topic)
//
// Deterministic by construction: no AI calls anywhere in this path.

// KeywordArticleScanLimit caps the per-day article scan (design §Risks:
// extreme volume guard). Articles beyond the cap are skipped with a warning —
// materialization degrades breadth, never correctness.
const KeywordArticleScanLimit = 5000

// keywordArticleText concatenates an article's title + summary into the
// lowercase-ready matching text. A missing summary degrades to title-only
// (legal — same degradation as the hint track's threads text).
func keywordArticleText(a repository.WatchScanArticle) string {
	return strings.ToLower(a.Title + "\n" + a.Summary)
}

// parseWatchTagIDs parses the raw JSON array string from
// WatchScanArticle.TagIDs ("[1,2,3]") into []uint. NULL / empty / malformed
// input yields nil (the article is tag-less — legal for keyword
// materialization, which deliberately scans outside the tag system).
func parseWatchTagIDs(raw *string) []uint {
	if raw == nil || *raw == "" {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(*raw), &ids); err != nil {
		return nil
	}
	return ids
}

// matchKeywordArticles matches a keyword expression against the scanned
// articles. An article hits when ANY conjunction group of the expression has
// ALL its terms present in title+summary (case-insensitive literal contains —
// the same parseKeywordExpr DNF the hint track uses, so the two tracks can
// never drift semantically). An invalid expression matches nothing.
func matchKeywordArticles(expr string, articles []repository.WatchScanArticle) []repository.WatchScanArticle {
	groups := parseKeywordExpr(expr)
	if len(groups) == 0 {
		return nil
	}
	var hits []repository.WatchScanArticle
	for _, a := range articles {
		text := keywordArticleText(a)
		if _, ok := matchKeywordGroups(groups, text); ok {
			hits = append(hits, a)
		}
	}
	return hits
}

// MaterializeKeywordWatches appends one ephemeral section per active
// keyword_topic watch of the board to the pending report assembly (sections /
// threadBatches, ClusterIndex continuing after the regular clusters). The
// recall layer (scan + DNF match) stays zero-AI; the adjudication layer
// (matCfg.Enabled, default on) runs one batch AI call per watch over the hit
// articles and only aggregates the ones fitting the tracking intent —
// adjudication failure degrades to the full recall result (design D4). A
// watch with no matching (or no adjudicated) articles yields no section
// (spec: 无命中不产空 section). Returns the appended count; failures of
// individual watches are logged and skipped (design §D1: degradation, never
// blocking).
func MaterializeKeywordWatches(
	ctx context.Context,
	boardID uint,
	watches []repository.BoardTopicWatch,
	nextClusterIndex int,
	matCfg WatchMaterializeConfig,
	chat watchChatFunc,
) ([]repository.DailyReportSection, [][]repository.DailyReportThread, error) {
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.MaterializeKeywordWatches")
	defer span.End()

	if len(watches) == 0 {
		return nil, nil, nil
	}

	// Shared scan: one query per day serves every keyword_topic watch.
	articles, err := scanBoardDayArticles(boardID, time.Time{})
	if err != nil {
		return nil, nil, fmt.Errorf("scan day articles: %w", err)
	}

	var sections []repository.DailyReportSection
	var batches [][]repository.DailyReportThread
	for _, w := range watches {
		hits := matchKeywordArticles(w.Label, articles)
		if len(hits) == 0 {
			continue
		}
		// Adjudication layer (watch-materialize-llm-adjudication): filter the
		// recall hits down to articles actually fitting the tracking intent.
		kept := hits
		var adj *watchAdjudicationResult
		if matCfg.Enabled {
			var aErr error
			adj, aErr = adjudicateWatchArticles(ctx, boardID, w.ID, "关键字追踪："+w.Label, hits, matCfg, chat)
			if aErr != nil {
				logging.Warnf("watch-adjudicate: keyword watch %d degraded to recall result: %v", w.ID, aErr)
				adj = nil // degrade: keep all hits, default confidence, fallback title
			} else {
				kept = filterAdjudicated(hits, adj)
			}
		}
		if len(kept) == 0 {
			logging.Infof("watch-adjudicate: keyword watch %d — %d hits all rejected, no section today", w.ID, len(hits))
			continue
		}
		sec, batch := buildKeywordWatchSection(w, kept, nextClusterIndex+len(sections))
		if adj != nil && adj.Title != "" {
			sec.ClusterLabel = adj.Title // LLM same-batch title; fallback = fixed derived name
		}
		applyThreadConfidence(batch, adj)
		sections = append(sections, sec)
		batches = append(batches, batch)
	}
	if len(sections) > 0 {
		logging.Infof("daily-report: keyword materialization appended %d sections for board %d (%d articles scanned)",
			len(sections), boardID, len(articles))
	}
	return sections, batches, nil
}

// buildKeywordWatchSection assembles the ephemeral section for one
// keyword_topic watch plus its threads (one per kept article).
// Field contract (design §D1): ClusterLabel = fixed derived name (the LLM
// same-batch title, when adjudication produced one, overrides it at the
// caller), ClusterTagIDs = empty array (articles may be tag-less — the
// section is article-anchored, not tag-anchored), BestTier=4 / AvgScore=0 /
// empty quality breakdown (no board-match signal exists for this section),
// Embedding empty, lane_tier = watch_keyword, PersistentTopicID NULL (spec:
// keyword track never owns a persistent topic).
func buildKeywordWatchSection(w repository.BoardTopicWatch, hits []repository.WatchScanArticle, clusterIndex int) (repository.DailyReportSection, []repository.DailyReportThread) {
	watchID := w.ID
	section := repository.DailyReportSection{
		ClusterIndex:     clusterIndex,
		ClusterLabel:     buildKeywordWatchSectionLabel(w.Label),
		ClusterTagIDs:    repository.JSON("[]"),
		WatchID:          &watchID,
		ArticleCount:     len(hits),
		BestTier:         4,
		AvgScore:         0,
		QualityBreakdown: repository.JSON("{}"),
		LaneTier:         LaneTierWatchKeyword,
	}

	threads := make([]repository.DailyReportThread, 0, len(hits))
	for _, a := range hits {
		tagIDs := mustMarshalUintArray(parseWatchTagIDs(a.TagIDs))
		threads = append(threads, repository.DailyReportThread{
			Title:             a.Title,
			Summary:           truncateRunes(a.Summary, keywordWatchSummaryRunes),
			TagIDs:            tagIDs,
			Confidence:        1.0, // mechanical deterministic match
			RelatedArticleIDs: mustMarshalUintArray([]uint{a.ID}),
		})
	}
	return section, threads
}

// keywordWatchSummaryRunes caps each materialized thread's summary so the
// section stays readable when the summary layer is a full article body
// (Content fallback). 300 runes ≈ 2-3 rendered lines in the timeline card.
const keywordWatchSummaryRunes = 300

// buildKeywordWatchSectionLabel renders the fixed section name, e.g.
// 关键字『harness』相关话题. Derived from the raw expression so the name is
// predictable from what the user typed (spec: 固定名称可预期).
func buildKeywordWatchSectionLabel(expr string) string {
	return "关键字『" + expr + "』相关话题"
}

// truncateRunes clamps s to at most n runes (UTF-8 safe).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// mustMarshalUintArray marshals []uint to repository.JSON, never failing
// (uint slices always serialize).
func mustMarshalUintArray(ids []uint) repository.JSON {
	if len(ids) == 0 {
		return repository.JSON("[]")
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%d", id)
	}
	sb.WriteByte(']')
	return repository.JSON(sb.String())
}

// localDayWindow returns the [start, end) window matching the report
// pipeline's article-day caliber: local-midnight to local-midnight
// (collectBoardTags uses the same convention — watch-materialized-topic
// must aggregate the exact same article set the regular clusters see).
func localDayWindow(date time.Time) (time.Time, time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	return start, start.Add(24 * time.Hour)
}

// scanBoardDayArticles loads the keyword-scan pool: all unarchived articles
// published in the local day window of date (zero value means today). Summary preference is coalesced in SQL
// (AIContentSummary > FirecrawlContent > Content > Description, empty-string
// NULLIF'd away) mirroring buildArticleContextForTag's precedence.
func scanBoardDayArticles(boardID uint, date time.Time) ([]repository.WatchScanArticle, error) {
	if date.IsZero() {
		date = time.Now()
	}
	start, end := localDayWindow(date)
	articles, err := repository.Repo.ListWatchScanArticles(start, end, KeywordArticleScanLimit)
	if err != nil {
		return nil, err
	}
	return articles, nil
}
