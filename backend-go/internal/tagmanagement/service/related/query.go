package related

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Coverage statuses reported by the read-only query (spec: 索引未就绪与依赖失败).
const (
	StatusReady    = "ready"
	StatusPartial  = "partial"
	StatusNotReady = "not_ready"
)

// not_ready reasons (never conflated with an empty result).
const (
	ReasonRouteUnavailable = "route_unavailable"
	ReasonSourceNotIndexed = "source_not_indexed"
)

// Query answers "which other events in this board are worth reading alongside
// this one". It is strictly read-only: no model call, no cache write, no
// identity/business mutation.
type Query struct {
	db        *gorm.DB
	now       func() time.Time
	threshold *float64
}

// NewQuery builds the read-only query over a DB handle.
func NewQuery(db *gorm.DB) *Query {
	return &Query{db: db, now: time.Now}
}

// WithThreshold overrides the configured threshold (tests only).
func (q *Query) WithThreshold(threshold float64) *Query {
	q.threshold = &threshold
	return q
}

// QueryResult is the full response payload: the results plus the coverage
// accounting that lets the UI distinguish "nothing related" from "not indexed".
type QueryResult struct {
	BoardID               uint           `json:"board_id"`
	TagID                 uint           `json:"tag_id"`
	Status                string         `json:"status"`
	Reason                string         `json:"reason,omitempty"`
	Threshold             float64        `json:"threshold"`
	WindowDays            int            `json:"window_days"`
	WindowStart           time.Time      `json:"window_start"`
	WindowEnd             time.Time      `json:"window_end"`
	CandidateCount        int            `json:"candidate_count"`
	IndexedCount          int            `json:"indexed_count"`
	RepresentationVersion string         `json:"representation_version"`
	SpaceKey              string         `json:"space_key,omitempty"`
	Items                 []RelatedEvent `json:"items"`
}

// RelatedEvent is one directly related event tag.
type RelatedEvent struct {
	TagID         uint           `json:"tag_id"`
	Label         string         `json:"label"`
	Description   string         `json:"description"`
	Similarity    float64        `json:"similarity"`
	SourceArticle *SourceArticle `json:"source_article"`
}

// scoredCandidateRow is one candidate with its exact cosine score.
type scoredCandidateRow struct {
	TagID       uint    `gorm:"column:tag_id"`
	Label       string  `gorm:"column:label"`
	Description string  `gorm:"column:description"`
	Score       float64 `gorm:"column:score"`
}

// RelatedEvents computes the directly related events of one board event tag.
//
// Order of operations (design D5): resolve the business scope first (board +
// source membership), then the window, then the current text/space, then the
// full candidate cosine set, and only afterwards the threshold + stable
// ordering + top-10 truncation. Candidates are never pre-truncated by an
// unrelated top-K, and nothing is expanded transitively.
func (q *Query) RelatedEvents(ctx context.Context, boardID, tagID uint) (*QueryResult, error) {
	asOf := q.now().UTC()
	threshold := q.resolveThreshold()

	result := &QueryResult{
		BoardID:               boardID,
		TagID:                 tagID,
		Status:                StatusNotReady,
		Threshold:             threshold,
		WindowDays:            WindowDays,
		WindowStart:           asOf.Add(-time.Duration(WindowDays) * 24 * time.Hour),
		WindowEnd:             asOf,
		RepresentationVersion: RepresentationVersion,
		Items:                 []RelatedEvent{},
	}

	ok, err := boardExists(ctx, q.db, boardID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBoardNotFound
	}

	sourceTags, err := loadBoardEventTags(ctx, q.db, boardID, nil, nil, []uint{tagID})
	if err != nil {
		return nil, err
	}
	if len(sourceTags) == 0 {
		return nil, ErrTagNotInBoard
	}
	source := sourceTags[0]

	generation := LoadGeneration(q.db)
	space, spaceErr := ResolveSpace(ctx, q.db, generation)
	if spaceErr != nil {
		result.Reason = ReasonRouteUnavailable
		return result, nil
	}
	result.SpaceKey = space.Key()

	sourceText, textErr := BuildRepresentation(source.Label, source.Description, source.AuxLabels)
	if textErr != nil {
		result.Reason = ReasonInvalidSource
		if textErr == ErrTextTooLong {
			result.Reason = ReasonTextTooLong
		}
		return result, nil
	}
	sourceHash := HashText(sourceText)

	sourceCached, err := loadCachedVectors(ctx, q.db, []uint{source.ID}, RepresentationVersion, space.Key())
	if err != nil {
		return nil, err
	}
	sourceVector, ok := sourceCached[source.ID]
	if !ok || sourceVector.TextHash != sourceHash || sourceVector.Dimension <= 0 {
		result.Reason = ReasonSourceNotIndexed
		return result, nil
	}

	from, to := result.WindowStart, result.WindowEnd
	candidates, err := loadBoardEventTags(ctx, q.db, boardID, &from, &to, nil)
	if err != nil {
		return nil, err
	}

	type scoredCandidate struct {
		tag  SourceTag
		hash string
	}
	ready := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == source.ID {
			continue
		}
		result.CandidateCount++
		text, buildErr := BuildRepresentation(candidate.Label, candidate.Description, candidate.AuxLabels)
		if buildErr != nil {
			continue
		}
		ready = append(ready, scoredCandidate{tag: candidate, hash: HashText(text)})
	}

	if result.CandidateCount == 0 {
		result.Status = StatusReady
		return result, nil
	}

	readyIDs := make([]uint, 0, len(ready))
	hashes := make(map[uint]string, len(ready))
	for _, item := range ready {
		readyIDs = append(readyIDs, item.tag.ID)
		hashes[item.tag.ID] = item.hash
	}
	cached, err := loadCachedVectors(ctx, q.db, readyIDs, RepresentationVersion, space.Key())
	if err != nil {
		return nil, err
	}

	scoreIDs := make([]uint, 0, len(readyIDs))
	for _, id := range readyIDs {
		row, ok := cached[id]
		if !ok || row.TextHash != hashes[id] || row.Dimension != sourceVector.Dimension {
			continue
		}
		scoreIDs = append(scoreIDs, id)
	}
	result.IndexedCount = len(scoreIDs)
	if result.IndexedCount < result.CandidateCount {
		result.Status = StatusPartial
	} else {
		result.Status = StatusReady
	}

	if len(scoreIDs) == 0 {
		return result, nil
	}

	scored, err := q.scoreCandidates(ctx, boardID, source.ID, space.Key(), scoreIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range scored {
		if row.Score+boundaryEpsilon < threshold {
			continue
		}
		result.Items = append(result.Items, RelatedEvent{
			TagID:       row.TagID,
			Label:       row.Label,
			Description: row.Description,
			Similarity:  row.Score,
		})
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].Similarity != result.Items[j].Similarity {
			return result.Items[i].Similarity > result.Items[j].Similarity
		}
		return result.Items[i].TagID < result.Items[j].TagID
	})
	if len(result.Items) > MaxResults {
		result.Items = result.Items[:MaxResults]
	}

	itemIDs := make([]uint, len(result.Items))
	for i := range result.Items {
		itemIDs[i] = result.Items[i].TagID
	}
	sources, err := loadSourceArticles(ctx, q.db, itemIDs)
	if err != nil {
		return nil, err
	}
	for i := range result.Items {
		if article, ok := sources[result.Items[i].TagID]; ok {
			copied := article
			result.Items[i].SourceArticle = &copied
		}
	}
	return result, nil
}

// scoreCandidates computes the exact cosine similarity for every prepared
// candidate in one query. The score is computed by pgvector (cosine distance)
// over the stored vectors; no approximate index is used, and the threshold is
// applied in Go AFTER the full candidate set is scored.
func (q *Query) scoreCandidates(ctx context.Context, boardID, sourceTagID uint, spaceKey string, tagIDs []uint) ([]scoredCandidateRow, error) {
	var rows []scoredCandidateRow
	err := q.db.WithContext(ctx).Raw(`
		SELECT cand.topic_tag_id AS tag_id,
		       tt.label AS label,
		       tt.description AS description,
		       1 - (cand.embedding <=> src.embedding) AS score
		FROM related_event_embeddings cand
		JOIN related_event_embeddings src
		  ON src.topic_tag_id = ?
		 AND src.representation_version = cand.representation_version
		 AND src.space_key = cand.space_key
		JOIN topic_tags tt ON tt.id = cand.topic_tag_id
		JOIN topic_tag_board_labels tbl
		  ON tbl.topic_tag_id = cand.topic_tag_id
		 AND tbl.semantic_board_id = ?
		WHERE cand.topic_tag_id IN ?
		  AND cand.representation_version = ?
		  AND cand.space_key = ?
		  AND NOT COALESCE(tbl.direction_mismatch, false)
		ORDER BY score DESC, cand.topic_tag_id ASC`,
		sourceTagID, boardID, tagIDs, RepresentationVersion, spaceKey).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("score related event candidates: %w", err)
	}
	return rows, nil
}

func (q *Query) resolveThreshold() float64 {
	if q.threshold != nil {
		return *q.threshold
	}
	return LoadThreshold(q.db)
}
