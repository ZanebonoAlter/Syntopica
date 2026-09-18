// Package sourcestats answers "which feed feeds me useful stuff": read-only
// aggregations that walk the existing forward chain
// feeds → articles → article_topic_tags → topic_tag_board_labels →
// semantic_labels(label_type='board', status='active') backwards, once per
// source (FeedBoardHitStats) and once per board (BoardSourceBreakdown).
//
// The measurement rules (口径) have a single authority:
// openspec/specs/source-board-hit-rate/spec.md.
// Its three hard constraints are enforced HERE and nowhere else:
//
//  1. 按文章去重 — every count is per distinct article
//     (COUNT(DISTINCT a.id) / COUNT(DISTINCT CASE … THEN a.id END)); articles
//     are never JOIN-multiplied by tags or boards (root cause of the recorded
//     1662→5193 miscount). The outer aggregation never JOINs a 1:N relation;
//     boolean flags are EXISTS subqueries (design D2).
//  2. 含已归档文章 — window queries MUST NOT filter on `archived`; archived
//     articles inside the window are counted. High-volume feeds keep almost
//     all their hits in the archived range (CleanupOldArticles), so filtering
//     would invert the conclusion. Do not "helpfully" add an archived clause.
//  3. 限窗口 — windowDays is whitelisted to {7,30,90} (ParseWindow); the
//     cutoff is computed on the Go side and passed as a SQL parameter
//     (design D2), keeping the SQL sqlite/Postgres portable.
package sourcestats

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
)

// Hit-predicate matching constants (spec: 命中口径 — an article hits when ≥1
// of its tags is linked by topic_tag_board_labels to a semantic label with
// label_type='board' AND status='active'; auxiliary/composite labels are not
// boards). Bound as SQL parameters so the predicate mirrors the existing
// board code (internal/tagmanagement/service/board/semantic_board_backfill.go).
const (
	labelTypeBoard    = "board"
	labelStatusActive = "active"
)

// DefaultWindowDays is the window used when the caller passes no value.
const DefaultWindowDays = 7

// allowedWindows is the spec whitelist {7,30,90} (hard constraint ③ 限窗口).
var allowedWindows = map[int]struct{}{7: {}, 30: {}, 90: {}}

// Sentinel errors mapped by handlers: ErrInvalidWindow → 400,
// ErrBoardNotFound → 404 (design D4).
var (
	ErrInvalidWindow = errors.New("invalid window: allowed values are 7, 30, 90")
	ErrBoardNotFound = errors.New("semantic board not found")
)

// nowFunc is the replaceable clock seam (test-cases §1): the window cutoff is
// always computed on the Go side and passed into SQL as a parameter, so tests
// can inject a fixed now instead of relative datetimes.
var nowFunc = time.Now

// ParseWindow resolves the raw window parameter against the spec whitelist:
// "" → DefaultWindowDays; "7"/"30"/"90" → the value; anything else (including
// "0", "-7", "14", "abc") → an error wrapping ErrInvalidWindow. Handlers MUST
// map that error to 400 and MUST NOT fall back to the default silently
// (spec: 非法窗口值被拒绝). Shared by both endpoints so the whitelist has one
// implementation.
func ParseWindow(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultWindowDays, nil
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: got %q", ErrInvalidWindow, raw)
	}
	if _, ok := allowedWindows[days]; !ok {
		return 0, fmt.Errorf("%w: got %q", ErrInvalidWindow, raw)
	}
	return days, nil
}

// BoardHit is one entry of a feed's hit-board distribution (spec: boards[]).
// An article hitting several boards is counted once per board, so the sum of
// Articles across entries MAY exceed the feed's deduplicated InBoard.
type BoardHit struct {
	BoardID  uint   `json:"board_id"`
	Label    string `json:"label"`
	Articles int64  `json:"articles"`
}

// FeedStat is one feed's window statistics (spec: 按订阅源聚合端点).
type FeedStat struct {
	FeedID          uint       `json:"feed_id"`
	Title           string     `json:"title"`
	TaggingEnabled  bool       `json:"tagging_enabled"`
	Articles        int64      `json:"articles"`
	InBoard         int64      `json:"in_board"`
	TaggedNoBoard   int64      `json:"tagged_no_board"`
	UntaggedPending int64      `json:"untagged_pending"`
	UntaggedSettled int64      `json:"untagged_settled"`
	HitRate         float64    `json:"hit_rate"`
	Boards          []BoardHit `json:"boards"`
}

// SourceBreakdown is one feed's contribution to a board (spec: sources[]).
// FeedArticles / FeedHitRate carry the feed's own whole-window context so
// sources that only occasionally land in this board stay recognizable
// (spec: 上下文列帮助识别只偶尔命中的源).
type SourceBreakdown struct {
	FeedID       uint    `json:"feed_id"`
	Title        string  `json:"title"`
	Articles     int64   `json:"articles"`
	Share        float64 `json:"share"`
	FeedArticles int64   `json:"feed_articles"`
	FeedHitRate  float64 `json:"feed_hit_rate"`
}

// BoardBreakdown is the board-view aggregation (spec: 按板块聚合端点).
type BoardBreakdown struct {
	TotalArticles int64             `json:"total_articles"`
	SourceCount   int               `json:"source_count"`
	Sources       []SourceBreakdown `json:"sources"`
}

// hitPredicate is the single definition of the 命中 judgement (spec: 命中口径):
// the article has at least one topic tag that topic_tag_board_labels links to
// an active semantic board. EXISTS — not a JOIN — so an article is never
// row-multiplied by its tags/boards (hard constraint ① 按文章去重).
const hitPredicate = `EXISTS (
SELECT 1 FROM article_topic_tags att
JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
WHERE att.article_id = a.id AND sl.label_type = @label_type AND sl.status = @label_status)`

// hasTagPredicate distinguishes "tagged but no board" from the untagged split.
const hasTagPredicate = `EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.article_id = a.id)`

// tagPendingPredicate — spec: 未打标两分. An untagged article counts as
// untagged_pending while an unfinished tagging job (pending/leased) exists;
// completed/failed jobs and never-enqueued articles (tagging disabled) are
// untagged_settled. Status values come from the models constants.
const tagPendingPredicate = `EXISTS (
SELECT 1 FROM tag_jobs j
WHERE j.article_id = a.id AND j.status IN (@job_pending, @job_leased))`

// feedCoreRow is the per-feed three-way split produced by feedCoreRows.
type feedCoreRow struct {
	FeedID          uint
	Title           string
	TaggingEnabled  bool
	Articles        int64
	InBoard         int64
	TaggedNoBoard   int64
	UntaggedPending int64
	UntaggedSettled int64
}

// feedCoreRows is THE source-level aggregation: one batched query over all
// feeds (no per-feed N+1). The LEFT JOIN carries the window condition in its
// ON clause so feeds with zero in-window articles still appear with zeros
// (spec: 含窗口内 0 篇的源). It deliberately has NO archived filter
// (hard constraint ② 含已归档文章). The identity
// articles == in_board + tagged_no_board + untagged_pending + untagged_settled
// holds per feed: every article is tagged (hit or not) or untagged
// (pending or settled).
func feedCoreRows(ctx context.Context, db *gorm.DB, cutoff time.Time) ([]feedCoreRow, error) {
	query := `SELECT
	f.id AS feed_id,
	f.title AS title,
	f.tagging_enabled AS tagging_enabled,
	COUNT(DISTINCT a.id) AS articles,
	COUNT(DISTINCT CASE WHEN ` + hitPredicate + ` THEN a.id END) AS in_board,
	COUNT(DISTINCT CASE WHEN ` + hasTagPredicate + ` AND NOT ` + hitPredicate + ` THEN a.id END) AS tagged_no_board,
	COUNT(DISTINCT CASE WHEN NOT ` + hasTagPredicate + ` AND ` + tagPendingPredicate + ` THEN a.id END) AS untagged_pending,
	COUNT(DISTINCT CASE WHEN NOT ` + hasTagPredicate + ` AND NOT ` + tagPendingPredicate + ` THEN a.id END) AS untagged_settled
FROM feeds f
LEFT JOIN articles a
	ON a.feed_id = f.id
	AND COALESCE(a.pub_date, a.created_at) >= @cutoff
GROUP BY f.id, f.title, f.tagging_enabled
ORDER BY f.id ASC`

	rows := []feedCoreRow{}
	err := db.WithContext(ctx).Raw(query, map[string]interface{}{
		"cutoff":       cutoff,
		"label_type":   labelTypeBoard,
		"label_status": labelStatusActive,
		"job_pending":  string(models.JobStatusPending),
		"job_leased":   string(models.JobStatusLeased),
	}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// boardDistribution is the SECOND, independent aggregation behind
// FeedBoardHitStats: hits per (feed_id, semantic_board_id), deduplicated per
// article pair. JOINs are safe here precisely because COUNT(DISTINCT a.id)
// collapses the tag/board multiplication back to distinct articles — the
// design-D2 no-JOIN rule applies to row-counting aggregations, and this one
// counts by (feed, board) pairs (spec: 板块分布是按板块计次的独立口径).
func boardDistribution(ctx context.Context, db *gorm.DB, cutoff time.Time) (map[uint][]BoardHit, error) {
	query := `SELECT
	a.feed_id AS feed_id,
	ttbl.semantic_board_id AS board_id,
	sl.label AS label,
	COUNT(DISTINCT a.id) AS articles
FROM articles a
JOIN article_topic_tags att ON att.article_id = a.id
JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
	AND sl.label_type = @label_type AND sl.status = @label_status
WHERE COALESCE(a.pub_date, a.created_at) >= @cutoff
GROUP BY a.feed_id, ttbl.semantic_board_id, sl.label
ORDER BY a.feed_id ASC, articles DESC, board_id ASC`

	var rows []struct {
		FeedID   uint
		BoardID  uint
		Label    string
		Articles int64
	}
	err := db.WithContext(ctx).Raw(query, map[string]interface{}{
		"cutoff":       cutoff,
		"label_type":   labelTypeBoard,
		"label_status": labelStatusActive,
	}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	dist := make(map[uint][]BoardHit, len(rows))
	for _, r := range rows {
		dist[r.FeedID] = append(dist[r.FeedID], BoardHit{
			BoardID:  r.BoardID,
			Label:    r.Label,
			Articles: r.Articles,
		})
	}
	return dist, nil
}

// cutoffFor computes now-N days on the Go side (design D2). The whitelist is
// re-checked here so the single 口径 implementation can never run with an
// off-spec window, even if a future caller skips ParseWindow.
func cutoffFor(windowDays int) (time.Time, error) {
	if _, ok := allowedWindows[windowDays]; !ok {
		return time.Time{}, fmt.Errorf("%w: got %d", ErrInvalidWindow, windowDays)
	}
	return nowFunc().AddDate(0, 0, -windowDays), nil
}

// ratio is num/den with the spec's zero-denominator rule: 0 when den == 0
// (hit_rate with articles=0; share with total_articles=0).
func ratio(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// FeedBoardHitStats returns the source-view window statistics for ALL feeds
// in one batched aggregation (spec: 按订阅源聚合端点 — 不逐源 N+1). Read-only.
func FeedBoardHitStats(ctx context.Context, db *gorm.DB, windowDays int) ([]FeedStat, error) {
	cutoff, err := cutoffFor(windowDays)
	if err != nil {
		return nil, err
	}

	rows, err := feedCoreRows(ctx, db, cutoff)
	if err != nil {
		return nil, err
	}
	dist, err := boardDistribution(ctx, db, cutoff)
	if err != nil {
		return nil, err
	}

	stats := make([]FeedStat, 0, len(rows))
	for _, r := range rows {
		boards := dist[r.FeedID]
		if boards == nil {
			boards = []BoardHit{} // JSON "[]", never null (spec: boards 为数组)
		}
		stats = append(stats, FeedStat{
			FeedID:          r.FeedID,
			Title:           r.Title,
			TaggingEnabled:  r.TaggingEnabled,
			Articles:        r.Articles,
			InBoard:         r.InBoard,
			TaggedNoBoard:   r.TaggedNoBoard,
			UntaggedPending: r.UntaggedPending,
			UntaggedSettled: r.UntaggedSettled,
			HitRate:         ratio(r.InBoard, r.Articles),
			Boards:          boards,
		})
	}
	return stats, nil
}

// BoardSourceBreakdown returns the board-view source composition for the
// given window (spec: 按板块聚合端点). The board must exist with
// label_type='board' (status is deliberately not part of the check: a
// disabled board simply aggregates to zero hits under the 命中口径, it is not
// "missing"). Read-only.
func BoardSourceBreakdown(ctx context.Context, db *gorm.DB, boardID uint, windowDays int) (BoardBreakdown, error) {
	cutoff, err := cutoffFor(windowDays)
	if err != nil {
		return BoardBreakdown{}, err
	}

	// Spec: 板块不存在或非 board 类型 → 404（复用既有 label_type='board' 校验语义）。
	var boardCount int64
	if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("id = ? AND label_type = ?", boardID, labelTypeBoard).
		Count(&boardCount).Error; err != nil {
		return BoardBreakdown{}, err
	}
	if boardCount == 0 {
		return BoardBreakdown{}, ErrBoardNotFound
	}

	boardArgs := map[string]interface{}{
		"cutoff":       cutoff,
		"board_id":     boardID,
		"label_type":   labelTypeBoard,
		"label_status": labelStatusActive,
	}

	// total_articles — computed independently from the per-source rows so the
	// "Σ sources.articles == total_articles" invariant (spec) stays a real
	// cross-check, not a tautology. Same dedup rules: COUNT(DISTINCT a.id),
	// no archived filter (hard constraint ②).
	var total int64
	if err := db.WithContext(ctx).Raw(`SELECT COUNT(DISTINCT a.id)
FROM articles a
JOIN article_topic_tags att ON att.article_id = a.id
JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
	AND sl.label_type = @label_type AND sl.status = @label_status
WHERE ttbl.semantic_board_id = @board_id
	AND COALESCE(a.pub_date, a.created_at) >= @cutoff`, boardArgs).Scan(&total).Error; err != nil {
		return BoardBreakdown{}, err
	}

	var rows []struct {
		FeedID   uint
		Title    string
		Articles int64
	}
	err = db.WithContext(ctx).Raw(`SELECT
	a.feed_id AS feed_id,
	f.title AS title,
	COUNT(DISTINCT a.id) AS articles
FROM articles a
JOIN article_topic_tags att ON att.article_id = a.id
JOIN topic_tag_board_labels ttbl ON ttbl.topic_tag_id = att.topic_tag_id
JOIN semantic_labels sl ON sl.id = ttbl.semantic_board_id
	AND sl.label_type = @label_type AND sl.status = @label_status
JOIN feeds f ON f.id = a.feed_id
WHERE ttbl.semantic_board_id = @board_id
	AND COALESCE(a.pub_date, a.created_at) >= @cutoff
GROUP BY a.feed_id, f.title
ORDER BY articles DESC, a.feed_id ASC`, boardArgs).Scan(&rows).Error
	if err != nil {
		return BoardBreakdown{}, err
	}

	// Context columns (feed_articles / feed_hit_rate) come from the very same
	// source-level aggregation — 口径唯一, no duplicated formulas here.
	coreRows, err := feedCoreRows(ctx, db, cutoff)
	if err != nil {
		return BoardBreakdown{}, err
	}
	coreByID := make(map[uint]feedCoreRow, len(coreRows))
	for _, cr := range coreRows {
		coreByID[cr.FeedID] = cr
	}

	sources := make([]SourceBreakdown, 0, len(rows))
	for _, r := range rows {
		core := coreByID[r.FeedID]
		sources = append(sources, SourceBreakdown{
			FeedID:       r.FeedID,
			Title:        r.Title,
			Articles:     r.Articles,
			Share:        ratio(r.Articles, total),
			FeedArticles: core.Articles,
			FeedHitRate:  ratio(core.InBoard, core.Articles),
		})
	}
	return BoardBreakdown{
		TotalArticles: total,
		SourceCount:   len(sources),
		Sources:       sources,
	}, nil
}
