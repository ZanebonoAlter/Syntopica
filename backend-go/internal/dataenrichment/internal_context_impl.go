package dataenrichment

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/service"
)

// dbInternalContextSearcher implements service.InternalContextSearcher against
// semantic_labels + board_persistent_topics (add-evidence-backed-cross-board-
// relations). Lexical scoring: label exact/prefix/substring on the query plus a
// description-substring bonus; hit_count weight prefers established lanes.
// Archived lanes and disabled boards never surface (the tool exists to pick a
// LIVE internal target). Results are compact: no timelines, no lifelines.
type dbInternalContextSearcher struct {
	db *gorm.DB
}

// NewDBInternalContextSearcher creates the production searcher.
func NewDBInternalContextSearcher(db *gorm.DB) service.InternalContextSearcher {
	return &dbInternalContextSearcher{db: db}
}

const internalContextMaxSummaryRunes = 160

// SearchInternalContext returns interleaved board + lane hits sorted by score
// desc, hit_count desc, id asc (deterministic beyond score ties).
func (s *dbInternalContextSearcher) SearchInternalContext(ctx context.Context, query string, maxResults int) ([]service.InternalContextHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search internal context: empty query")
	}
	if maxResults <= 0 || maxResults > 20 {
		maxResults = 8
	}
	like := "%" + escapeLike(query) + "%"

	// Lane hits: label match (weight 3) + description match (weight 1) +
	// log-ish hit_count preference via ordinal ordering only. Active and
	// candidate lanes qualify (candidates carry emerging cross-domain signal).
	type laneRow struct {
		LaneID      uint
		BoardID     uint
		Label       string
		Status      string
		HitCount    int
		Description string
		LabelHit    bool
		DescHit     bool
	}
	var laneRows []laneRow
	if err := s.db.WithContext(ctx).
		Table("board_persistent_topics").
		Select(`id AS lane_id, semantic_board_id AS board_id, label, status, hit_count,
			COALESCE(description, '') AS description,
			(label ILIKE ?) AS label_hit, (COALESCE(description, '') ILIKE ?) AS desc_hit`, like, like).
		Where("status IN ('active', 'candidate') AND (label ILIKE ? OR COALESCE(description, '') ILIKE ?)", like, like).
		Order("hit_count DESC, id ASC").
		Limit(maxResults * 3). // overfetch; scoring/ranking happens in Go
		Scan(&laneRows).Error; err != nil {
		return nil, fmt.Errorf("search internal context lanes: %w", err)
	}

	// Board hits: label/alias/description match.
	type boardRow struct {
		ID          uint
		Label       string
		Status      string
		Description string
		LabelHit    bool
		DescHit     bool
	}
	var boardRows []boardRow
	if err := s.db.WithContext(ctx).
		Table("semantic_labels").
		Select(`id, label, COALESCE(status, 'active') AS status, COALESCE(description, '') AS description,
			(label ILIKE ?) AS label_hit, (COALESCE(description, '') ILIKE ?) AS desc_hit`, like, like).
		Where(`label_type = 'board' AND COALESCE(status, 'active') = 'active'
			AND (label ILIKE ? OR COALESCE(description, '') ILIKE ?)`, like, like).
		Order("id ASC").
		Limit(maxResults).
		Scan(&boardRows).Error; err != nil {
		return nil, fmt.Errorf("search internal context boards: %w", err)
	}

	hits := make([]service.InternalContextHit, 0, len(laneRows)+len(boardRows))
	for _, r := range laneRows {
		score := 0.0
		if r.LabelHit {
			score += 3.0
		}
		if r.DescHit {
			score += 1.0
		}
		laneID := r.LaneID
		hits = append(hits, service.InternalContextHit{
			Kind:     "lane",
			BoardID:  r.BoardID,
			LaneID:   &laneID,
			Label:    r.Label,
			Status:   r.Status,
			HitCount: r.HitCount,
			Summary:  truncateRunesForContext(r.Description),
			Score:    score,
		})
	}
	for _, r := range boardRows {
		score := 0.0
		if r.LabelHit {
			score += 3.0
		}
		if r.DescHit {
			score += 1.0
		}
		hits = append(hits, service.InternalContextHit{
			Kind:    "board",
			BoardID: r.ID,
			Label:   r.Label,
			Status:  r.Status,
			Summary: truncateRunesForContext(r.Description),
			Score:   score,
		})
	}

	// Deterministic ranking: score desc, lane-before-board on tie (lanes are
	// the actionable targets for get_lane_detail), then hit_count desc, id asc.
	stableSortHits(hits)
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits, nil
}

// stableSortHits sorts by (score desc, hit_count desc, id asc) with a simple
// insertion sort — the set is tiny (≤ 3*maxResults + maxResults).
func stableSortHits(hits []service.InternalContextHit) {
	for i := 1; i < len(hits); i++ {
		h := hits[i]
		j := i - 1
		for j >= 0 && hitLess(h, hits[j]) {
			hits[j+1] = hits[j]
			j--
		}
		hits[j+1] = h
	}
}

func hitLess(a, b service.InternalContextHit) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.HitCount != b.HitCount {
		return a.HitCount > b.HitCount
	}
	aid, bid := a.BoardID, b.BoardID
	if a.LaneID != nil {
		aid = *a.LaneID
	}
	if b.LaneID != nil {
		bid = *b.LaneID
	}
	return aid < bid
}

func truncateRunesForContext(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= internalContextMaxSummaryRunes {
		return s
	}
	return string(r[:internalContextMaxSummaryRunes]) + "…"
}

// escapeLike neutralizes LIKE metacharacters in user input so the query is a
// literal substring probe, not a pattern.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
