package related

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
)

// ListBoardEventTags exposes the effective-event-tag projection for read-only
// callers (the historical experiment). It never writes.
func ListBoardEventTags(ctx context.Context, db *gorm.DB, boardID uint, from, to *time.Time) ([]SourceTag, error) {
	ok, err := boardExists(ctx, db, boardID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBoardNotFound
	}
	return loadBoardEventTags(ctx, db, boardID, from, to, nil)
}

// ExperimentPair is one comparable tag pair with its exact cosine similarity.
// Auxiliary labels are included so a reviewer can judge "worth reading together"
// without opening the database.
type ExperimentPair struct {
	AID    uint     `json:"a_id"`
	ALabel string   `json:"a_label"`
	AAux   []string `json:"a_aux"`
	BID    uint     `json:"b_id"`
	BLabel string   `json:"b_label"`
	BAux   []string `json:"b_aux"`
	Score  float64  `json:"score"`
}

// ExperimentOptions scopes one read-only historical check.
type ExperimentOptions struct {
	BoardID     uint
	WindowDays  int
	Threshold   float64
	SampleLimit int
	NearLimit   int
}

// ExperimentResult is the auditable output of the historical check (design D8):
// model/representation/scope/coverage plus the sampled pairs.
type ExperimentResult struct {
	BoardID               uint             `json:"board_id"`
	RepresentationVersion string           `json:"representation_version"`
	Provider              string           `json:"provider"`
	Model                 string           `json:"model"`
	Threshold             float64          `json:"threshold"`
	WindowDays            int              `json:"window_days"`
	AsOf                  string           `json:"as_of"`
	WindowStart           string           `json:"window_start"`
	TagCount              int              `json:"tag_count"`
	Embedded              int              `json:"embedded"`
	Failed                int              `json:"failed"`
	Failures              []RefreshFailure `json:"failures,omitempty"`
	PairCount             int              `json:"pair_count"`
	AboveCount            int              `json:"above_threshold_count"`
	SampleRule            string           `json:"sample_rule"`
	Direct                []ExperimentPair `json:"direct_pairs"`
	Near                  []ExperimentPair `json:"near_threshold_pairs"`
	// CacheWrites is always 0: the experiment never writes the related-event
	// cache, business identity, ownership or any relation edge. The numbers are
	// stated explicitly so "read-only" is checkable in the output.
	CacheWrites int `json:"cache_writes"`
}

// Experiment computes pairwise similarity in memory for a historical check.
// The only side effect is the audited airouter embedding call (design D8);
// no DB write happens here.
type Experiment struct {
	db       *gorm.DB
	embedder Embedder
	now      func() time.Time
}

// NewExperiment builds the experiment over the production airouter.
func NewExperiment(db *gorm.DB) *Experiment {
	return &Experiment{db: db, embedder: airouter.NewRouter(), now: time.Now}
}

// NewExperimentWithEmbedder builds the experiment with an explicit embedder (tests).
func NewExperimentWithEmbedder(db *gorm.DB, embedder Embedder) *Experiment {
	return &Experiment{db: db, embedder: embedder, now: time.Now}
}

// Run computes the historical similarity landscape of one board.
func (e *Experiment) Run(ctx context.Context, opts ExperimentOptions) (*ExperimentResult, error) {
	if opts.WindowDays <= 0 {
		opts.WindowDays = WindowDays
	}
	if opts.SampleLimit <= 0 {
		opts.SampleLimit = 50
	}
	if opts.NearLimit <= 0 {
		opts.NearLimit = 10
	}
	asOf := e.now().UTC()
	from := asOf.Add(-time.Duration(opts.WindowDays) * 24 * time.Hour)

	tags, err := ListBoardEventTags(ctx, e.db, opts.BoardID, &from, &asOf)
	if err != nil {
		return nil, err
	}

	result := &ExperimentResult{
		BoardID:               opts.BoardID,
		RepresentationVersion: RepresentationVersion,
		Threshold:             opts.Threshold,
		WindowDays:            opts.WindowDays,
		AsOf:                  asOf.Format(time.RFC3339),
		WindowStart:           from.Format(time.RFC3339),
		TagCount:              len(tags),
		SampleRule:            fmt.Sprintf("pairs scored with cosine over representation %s; direct sample = stratified over ALL pairs with score >= %.4f sorted by score desc then a_id/b_id asc (every k-th, so the whole qualifying band is represented, not only the top scores); near sample = the %d pairs closest below the threshold; no manual cherry-picking", RepresentationVersion, opts.Threshold, opts.NearLimit),
		Direct:                []ExperimentPair{},
		Near:                  []ExperimentPair{},
	}

	type embeddedTag struct {
		tag    SourceTag
		vector []float64
	}
	embedded := make([]embeddedTag, 0, len(tags))
	for start := 0; start < len(tags); start += refreshBatchSize {
		end := min(start+refreshBatchSize, len(tags))
		batch := tags[start:end]
		texts := make([]string, 0, len(batch))
		usable := make([]SourceTag, 0, len(batch))
		for _, tag := range batch {
			text, buildErr := BuildRepresentation(tag.Label, tag.Description, tag.AuxLabels)
			if buildErr != nil {
				reason := ReasonInvalidSource
				if buildErr == ErrTextTooLong {
					reason = ReasonTextTooLong
				}
				result.Failures = append(result.Failures, RefreshFailure{TagID: tag.ID, Label: tag.Label, Reason: reason, Error: buildErr.Error()})
				result.Failed++
				continue
			}
			texts = append(texts, text)
			usable = append(usable, tag)
		}
		if len(texts) == 0 {
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, refreshCallTimeout)
		res, embedErr := e.embedder.Embed(callCtx, airouter.EmbeddingRequest{
			Input:     texts,
			Operation: Operation,
			Metadata: map[string]any{
				"mode":                   "experiment",
				"board_id":               opts.BoardID,
				"representation_version": RepresentationVersion,
				"batch_size":             len(texts),
			},
		}, airouter.CapabilityEmbedding)
		cancel()
		if embedErr != nil {
			for _, tag := range usable {
				result.Failures = append(result.Failures, RefreshFailure{TagID: tag.ID, Label: tag.Label, Reason: ReasonEmbedFailed, Error: embedErr.Error()})
				result.Failed++
			}
			continue
		}
		if res == nil || len(res.Embeddings) != len(texts) {
			for _, tag := range usable {
				result.Failures = append(result.Failures, RefreshFailure{TagID: tag.ID, Label: tag.Label, Reason: ReasonEmbedFailed, Error: "provider returned a mismatched batch"})
				result.Failed++
			}
			continue
		}
		if result.Provider == "" {
			result.Provider = res.Provider
			result.Model = res.Model
		} else if result.Provider != res.Provider || result.Model != res.Model {
			// 混入另一个模型空间的结果无法两两比较：整批如实记为失败。
			for _, tag := range usable {
				result.Failures = append(result.Failures, RefreshFailure{
					TagID: tag.ID, Label: tag.Label, Reason: ReasonSpaceMismatch,
					Error: fmt.Sprintf("provider answered as %s/%s inside one run", res.Provider, res.Model),
				})
				result.Failed++
			}
			continue
		}

		for i, tag := range usable {
			vector := res.Embeddings[i]
			if reason, reasonErr := validateVector(vector, res.Dimensions); reason != "" {
				result.Failures = append(result.Failures, RefreshFailure{TagID: tag.ID, Label: tag.Label, Reason: reason, Error: reasonErr})
				result.Failed++
				continue
			}
			embedded = append(embedded, embeddedTag{tag: tag, vector: vector})
		}
	}
	result.Embedded = len(embedded)

	pairs := make([]ExperimentPair, 0)
	for i := 0; i < len(embedded); i++ {
		for j := i + 1; j < len(embedded); j++ {
			score, cosineErr := airouter.CosineSimilarity(embedded[i].vector, embedded[j].vector)
			if cosineErr != nil {
				continue
			}
			a, b := embedded[i].tag, embedded[j].tag
			pairs = append(pairs, ExperimentPair{
				AID: a.ID, ALabel: a.Label, AAux: a.AuxLabels,
				BID: b.ID, BLabel: b.Label, BAux: b.AuxLabels,
				Score: score,
			})
		}
	}
	result.PairCount = len(pairs)
	direct, near := SelectExperimentPairs(pairs, opts.Threshold, opts.SampleLimit, opts.NearLimit)
	result.Direct = direct
	result.Near = near
	for _, pair := range pairs {
		if pair.Score >= opts.Threshold {
			result.AboveCount++
		}
	}
	return result, nil
}

// SelectExperimentPairs applies the documented sampling rule:
//   - qualifying pairs (score >= threshold) are sorted by score desc then ids
//     asc and sampled STRATIFIED across the whole qualifying band, so a reviewer
//     sees the near-threshold end too, not only the (trivially high) top scores;
//   - the near-threshold sample is the closest qualifying-below pairs.
//
// Both are fully deterministic: the same data always yields the same sample.
func SelectExperimentPairs(pairs []ExperimentPair, threshold float64, sampleLimit, nearLimit int) ([]ExperimentPair, []ExperimentPair) {
	direct := make([]ExperimentPair, 0)
	near := make([]ExperimentPair, 0)
	for _, pair := range pairs {
		if pair.Score >= threshold {
			direct = append(direct, pair)
		} else {
			near = append(near, pair)
		}
	}
	sortPairs(direct)
	sortPairs(near)
	direct = stratifiedSample(direct, sampleLimit)
	if len(near) > nearLimit {
		near = near[:nearLimit]
	}
	return direct, near
}

// stratifiedSample picks up to limit entries spread evenly over an already
// sorted slice (every k-th entry). len <= limit returns the slice unchanged.
func stratifiedSample(sorted []ExperimentPair, limit int) []ExperimentPair {
	if limit <= 0 || len(sorted) <= limit {
		return sorted
	}
	out := make([]ExperimentPair, 0, limit)
	for i := 0; i < limit; i++ {
		idx := i * len(sorted) / limit
		out = append(out, sorted[idx])
	}
	return out
}

func sortPairs(pairs []ExperimentPair) {
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].Score != pairs[j].Score {
			return pairs[i].Score > pairs[j].Score
		}
		if pairs[i].AID != pairs[j].AID {
			return pairs[i].AID < pairs[j].AID
		}
		return pairs[i].BID < pairs[j].BID
	})
}
