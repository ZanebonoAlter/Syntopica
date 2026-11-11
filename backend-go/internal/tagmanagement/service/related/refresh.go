package related

import (
	"context"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

const (
	// refreshBatchSize caps one provider call (same 32-item ceiling the tag
	// embedding batches use).
	refreshBatchSize = 32
	// refreshCallTimeout bounds a single provider call; a timed-out batch is a
	// recorded failure, and the rest of the board still gets its chance.
	refreshCallTimeout = 30 * time.Second
)

// Embedder is the only capability the refresh needs (airouter.Router satisfies
// it). Tests inject a stub so no test ever calls a real provider.
type Embedder interface {
	Embed(ctx context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error)
}

// Failure reasons recorded per tag (never written to the cache).
const (
	ReasonInvalidSource = "invalid_source"
	ReasonTextTooLong   = "text_too_long"
	ReasonEmbedFailed   = "embed_failed"
	ReasonSpaceMismatch = "space_mismatch"
	ReasonEmptyVector   = "empty_vector"
	ReasonDimension     = "dimension_mismatch"
	ReasonNonFinite     = "non_finite_vector"
	ReasonZeroNorm      = "zero_norm_vector"
	ReasonWriteFailed   = "cache_write_failed"
)

// RefreshFailure is one non-indexed tag with its reason.
type RefreshFailure struct {
	TagID  uint   `json:"tag_id"`
	Label  string `json:"label"`
	Reason string `json:"reason"`
	Error  string `json:"error,omitempty"`
}

// RefreshResult is the honest accounting of one board refresh.
type RefreshResult struct {
	BoardID               uint             `json:"board_id"`
	TagCount              int              `json:"tag_count"`
	Indexed               int              `json:"indexed"`
	Reused                int              `json:"reused"`
	Failed                int              `json:"failed"`
	Failures              []RefreshFailure `json:"failures,omitempty"`
	SpaceKey              string           `json:"space_key"`
	RepresentationVersion string           `json:"representation_version"`
	DurationMS            int64            `json:"duration_ms"`
}

// Refresher builds the related-event vector index for one board. It is the ONLY
// writer of related_event_embeddings and never touches source tags, auxiliary
// links, articles, threads or legacy vectors.
type Refresher struct {
	db       *gorm.DB
	embedder Embedder
	now      func() time.Time
}

// NewRefresher builds a refresher over the production airouter.
func NewRefresher(db *gorm.DB) *Refresher {
	return &Refresher{db: db, embedder: airouter.NewRouter(), now: time.Now}
}

// NewRefresherWithEmbedder builds a refresher with an explicit embedder (tests).
func NewRefresherWithEmbedder(db *gorm.DB, embedder Embedder) *Refresher {
	return &Refresher{db: db, embedder: embedder, now: time.Now}
}

// RefreshBoard indexes every effective event tag of the board under the current
// representation version and space. Valid cache rows (same text hash, same
// space) are reused instead of recomputed; failures are reported and leave any
// previous valid row untouched. Re-running after a failure recovers.
func (r *Refresher) RefreshBoard(ctx context.Context, boardID uint, generation string) (*RefreshResult, error) {
	started := r.now()
	ok, err := boardExists(ctx, r.db, boardID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBoardNotFound
	}

	space, err := ResolveSpace(ctx, r.db, generation)
	if err != nil {
		return nil, err
	}
	result := &RefreshResult{
		BoardID:               boardID,
		SpaceKey:              space.Key(),
		RepresentationVersion: RepresentationVersion,
	}

	tags, err := loadBoardEventTags(ctx, r.db, boardID, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	result.TagCount = len(tags)

	pending, err := r.classify(ctx, tags, space, result)
	if err != nil {
		return nil, err
	}

	for start := 0; start < len(pending); start += refreshBatchSize {
		end := min(start+refreshBatchSize, len(pending))
		batch := pending[start:end]
		texts := make([]string, len(batch))
		for i := range batch {
			texts[i] = batch[i].text
		}

		callCtx, cancel := context.WithTimeout(ctx, refreshCallTimeout)
		res, embedErr := r.embedder.Embed(callCtx, airouter.EmbeddingRequest{
			Input:     texts,
			Operation: Operation,
			Metadata: map[string]any{
				"board_id":               boardID,
				"representation_version": RepresentationVersion,
				"space_key":              space.Key(),
				"batch_size":             len(texts),
			},
		}, airouter.CapabilityEmbedding)
		cancel()
		if embedErr != nil {
			for _, item := range batch {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: ReasonEmbedFailed, Error: embedErr.Error(),
				})
			}
			result.Failed += len(batch)
			continue
		}
		if res == nil {
			for _, item := range batch {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: ReasonEmbedFailed, Error: "empty provider result",
				})
			}
			result.Failed += len(batch)
			continue
		}
		// A fallback provider may answer with a DIFFERENT model: writing those
		// vectors under the primary space key would mix spaces silently.
		if res.Provider != space.Provider || res.Model != space.Model {
			got := fmt.Sprintf("%s/%s", res.Provider, res.Model)
			for _, item := range batch {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: ReasonSpaceMismatch, Error: "provider answered as " + got,
				})
			}
			result.Failed += len(batch)
			continue
		}
		if len(res.Embeddings) != len(batch) {
			for _, item := range batch {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: ReasonEmbedFailed,
					Error: fmt.Sprintf("provider returned %d vectors for %d inputs", len(res.Embeddings), len(batch)),
				})
			}
			result.Failed += len(batch)
			continue
		}

		dimension := res.Dimensions
		for i, item := range batch {
			vector := res.Embeddings[i]
			if reason, reasonErr := validateVector(vector, dimension); reason != "" {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: reason, Error: reasonErr,
				})
				result.Failed++
				continue
			}
			if dimension == 0 {
				dimension = len(vector)
			}
			row := &tagmodels.RelatedEventEmbedding{
				TopicTagID:            item.tag.ID,
				RepresentationVersion: RepresentationVersion,
				SpaceKey:              space.Key(),
				TextHash:              item.hash,
				Dimension:             len(vector),
				EmbeddingVec:          core.FloatsToPgVector(vector),
				UpdatedAt:             r.now(),
			}
			if err := upsertCachedVector(ctx, r.db, row); err != nil {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: item.tag.ID, Label: item.tag.Label, Reason: ReasonWriteFailed, Error: err.Error(),
				})
				result.Failed++
				continue
			}
			result.Indexed++
		}
	}

	result.DurationMS = r.now().Sub(started).Milliseconds()
	return result, nil
}

// pendingEmbedding is one tag whose text is ready and whose cache row is
// missing or stale.
type pendingEmbedding struct {
	tag  SourceTag
	text string
	hash string
}

// classify splits the tags into "already valid" (reused) and "needs embedding",
// recording the tags whose text cannot be built at all.
func (r *Refresher) classify(ctx context.Context, tags []SourceTag, space Space, result *RefreshResult) ([]pendingEmbedding, error) {
	ids := make([]uint, len(tags))
	for i := range tags {
		ids[i] = tags[i].ID
	}
	cached, err := loadCachedVectors(ctx, r.db, ids, RepresentationVersion, space.Key())
	if err != nil {
		return nil, err
	}

	pending := make([]pendingEmbedding, 0, len(tags))
	for _, tag := range tags {
		text, err := BuildRepresentation(tag.Label, tag.Description, tag.AuxLabels)
		if err != nil {
			reason := ReasonInvalidSource
			if err == ErrTextTooLong {
				reason = ReasonTextTooLong
			}
			result.Failures = append(result.Failures, RefreshFailure{
				TagID: tag.ID, Label: tag.Label, Reason: reason, Error: err.Error(),
			})
			result.Failed++
			continue
		}
		hash := HashText(text)
		if row, ok := cached[tag.ID]; ok && row.TextHash == hash && row.Dimension > 0 {
			result.Reused++
			continue
		}
		pending = append(pending, pendingEmbedding{tag: tag, text: text, hash: hash})
	}
	return pending, nil
}

// validateVector enforces the per-item shape rules: non-empty, finite values,
// non-zero norm, and (when the provider declares one) the declared dimension.
func validateVector(vector []float64, dimension int) (string, string) {
	if len(vector) == 0 {
		return ReasonEmptyVector, "provider returned an empty vector"
	}
	if dimension > 0 && len(vector) != dimension {
		return ReasonDimension, fmt.Sprintf("vector has %d values, provider declared %d", len(vector), dimension)
	}
	var normSq float64
	for _, v := range vector {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ReasonNonFinite, "vector contains a non-finite value"
		}
		normSq += v * v
	}
	if normSq == 0 {
		return ReasonZeroNorm, "vector has zero norm"
	}
	return "", ""
}
