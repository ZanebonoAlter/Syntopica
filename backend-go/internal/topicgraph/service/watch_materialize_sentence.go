package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// Sentence-track materialization (watch-materialized-topic design §D3/D4).
//
//	sentence ──embed once, cached on the watch──▶ query vector
//	  → cosine-retrieve the board's auxiliary-label pool (threshold + top-K)
//	  → labels → event tags → day's articles
//	  → one section owned by the watch's dedicated persistent topic
//	    (source=manual, active — a full citizen: lane anchor, lifecycle)
//
// The embedding is computed at watch creation and cached in
// board_topic_watches.embedding_cache; label/query PATCHes invalidate the
// cache; the generation path lazily recomputes and writes it back.

// WatchSentenceConfig holds the sentence-track retrieval parameters
// (ai_settings-overridable, same mechanism as PersistentTopicConfig).
type WatchSentenceConfig struct {
	// RetrievalThreshold is the minimum cosine similarity for an auxiliary
	// label to count as hit (ai_settings: watch_sentence_retrieval_threshold).
	RetrievalThreshold float64
	// RetrievalTopK caps the hit label set (ai_settings:
	// watch_sentence_retrieval_top_k).
	RetrievalTopK int
}

// DefaultWatchSentenceConfig — threshold 0.55 / top-K 8 (design D3 initial
// values, marked for tuning; per-generation hit counts are logged to make
// calibration observable).
func DefaultWatchSentenceConfig() WatchSentenceConfig {
	return WatchSentenceConfig{
		RetrievalThreshold: 0.55,
		RetrievalTopK:      8,
	}
}

// LoadWatchSentenceConfig loads the sentence-track config from ai_settings,
// falling back to defaults for absent/invalid rows.
func LoadWatchSentenceConfig(db *gorm.DB) WatchSentenceConfig {
	cfg := DefaultWatchSentenceConfig()
	var rows []models.AISettings
	if err := db.Where("key IN ?", []string{
		"watch_sentence_retrieval_threshold",
		"watch_sentence_retrieval_top_k",
	}).Find(&rows).Error; err != nil {
		return cfg
	}
	for _, r := range rows {
		switch r.Key {
		case "watch_sentence_retrieval_threshold":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil && v > 0 && v <= 1 {
				cfg.RetrievalThreshold = v
			}
		case "watch_sentence_retrieval_top_k":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.RetrievalTopK = v
			}
		}
	}
	return cfg
}

// retrieveAuxLabels retrieves the board's auxiliary-label pool and returns
// the labels whose cosine similarity to queryVec passes cfg's threshold,
// capped at top-K, best first. Labels without embeddings are skipped (legal
// degradation — the repository already filters NULL). Pool size is
// tens-to-hundreds — in-memory cosine per design D3.
func retrieveAuxLabels(boardID uint, queryVec []float64, cfg WatchSentenceConfig) ([]repository.WatchSentenceLabel, error) {
	pool, err := repository.Repo.ListBoardAuxLabelEmbeddings(boardID)
	if err != nil {
		return nil, err
	}
	type scored struct {
		label repository.WatchSentenceLabel
		score float64
	}
	var hits []scored
	for _, l := range pool {
		if len(l.Embedding) == 0 || len(queryVec) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, l.Embedding)
		if sim >= cfg.RetrievalThreshold {
			hits = append(hits, scored{label: l, score: sim})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].label.ID < hits[j].label.ID
	})
	if len(hits) > cfg.RetrievalTopK {
		hits = hits[:cfg.RetrievalTopK]
	}
	out := make([]repository.WatchSentenceLabel, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.label)
	}
	return out, nil
}

// cosineSimilarity computes the cosine of two equal-length vectors. Vectors
// of differing length or zero norm yield 0 (never a hit).
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// watchQuerySentence resolves the retrieval sentence: explicit Query wins,
// Label is the fallback (spec: query 为空时回退 label).
func watchQuerySentence(w repository.BoardTopicWatch) string {
	if q := w.Query; q != "" {
		return q
	}
	return w.Label
}

// ensureWatchQueryVec returns the watch's cached query vector, lazily
// recomputing (embedding watchQuerySentence(w)) and writing back when the
// cache is NULL. embed failures degrade to nil (caller skips the watch for
// this generation — never blocks the report).
func ensureWatchQueryVec(ctx context.Context, w *repository.BoardTopicWatch, embed embedFunc) []float64 {
	if w.EmbeddingCache != nil {
		if v, err := parsePgVector(*w.EmbeddingCache); err == nil && len(v) > 0 {
			return v
		}
		// Malformed cache: fall through to recompute (and overwrite below).
	}
	if embed == nil {
		return nil
	}
	result, err := embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{watchQuerySentence(*w)},
		Operation: "watch_sentence.embedding",
		SessionID: SessionIDFromContext(ctx),
		Metadata: map[string]any{
			"operation": "watch_sentence_embedding",
			"board_id":  w.SemanticBoardID,
			"watch_id":  w.ID,
		},
	}, airouter.CapabilityEmbedding)
	if err != nil || len(result.Embeddings) == 0 {
		logging.Warnf("watch-materialize: lazy query embedding failed for watch %d: %v", w.ID, err)
		return nil
	}
	vec := result.Embeddings[0]
	if wErr := updateWatchCacheSafe(w.ID, repository.FloatsToPgVector(vec)); wErr != nil {
		logging.Warnf("watch-materialize: cache write-back failed for watch %d: %v", w.ID, wErr)
	}
	w.EmbeddingCache = strPtrHelper(repository.FloatsToPgVector(vec))
	return vec
}

// updateWatchCacheSafe guards the global-repo write-back: unit tests run
// without a DB, where repository.Repo is nil — degrade instead of panicking
// (the production path always has Repo initialized).
func updateWatchCacheSafe(watchID uint, pgVec string) error {
	if repository.Repo == nil {
		return nil
	}
	return repository.Repo.UpdateWatchEmbeddingCache(watchID, pgVec)
}

// strPtrHelper boxes a string (small alias-free helper).
func strPtrHelper(s string) *string { return &s }

// MaterializeSentenceWatch produces the sentence-track section for one watch:
// retrieval → tag resolution → day's article union → LLM adjudication →
// section + threads, owned by the watch's dedicated persistent topic (created
// at first materialization). The adjudication layer (matCfg.Enabled, default
// on) filters the union down to articles fitting the retrieval sentence's
// intent; failure degrades to the full union (design D4). Returns nil
// sections when nothing hit (or everything was rejected) — legal: no section
// that day. Any failure returns an error the orchestrator logs-and-skips.
func MaterializeSentenceWatch(
	ctx context.Context,
	w repository.BoardTopicWatch,
	date time.Time,
	cfg WatchSentenceConfig,
	matCfg WatchMaterializeConfig,
	embed embedFunc,
	chat watchChatFunc,
) (*repository.DailyReportSection, []repository.DailyReportThread, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "service.MaterializeSentenceWatch")
	defer span.End()

	queryVec := ensureWatchQueryVec(ctx, &w, embed)
	if len(queryVec) == 0 {
		return nil, nil, fmt.Errorf("no query vector available for watch %d", w.ID)
	}

	labels, err := retrieveAuxLabels(w.SemanticBoardID, queryVec, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve aux labels: %w", err)
	}
	if len(labels) == 0 {
		logging.Infof("watch-materialize: watch %d retrieved 0 aux labels (threshold %.2f) — no section today", w.ID, cfg.RetrievalThreshold)
		return nil, nil, nil
	}
	labelIDs := make([]uint, 0, len(labels))
	for _, l := range labels {
		labelIDs = append(labelIDs, l.ID)
	}

	start, end := localDayWindow(date)
	tagRows, err := repository.Repo.ListActiveEventTagsBySemanticLabels(labelIDs, start, end)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve labels to day tags: %w", err)
	}
	if len(tagRows) == 0 {
		return nil, nil, nil // labels hit but nothing published today
	}
	tagIDs := make([]uint, 0, len(tagRows))
	tagSet := make(map[uint]bool, len(tagRows))
	for _, t := range tagRows {
		if !tagSet[t] {
			tagSet[t] = true
			tagIDs = append(tagIDs, t)
		}
	}

	// Article union across the day's resolved tags (deduped, id-ordered).
	articles, err := repository.Repo.ListArticlesByTagsForDay(tagIDs, start, end)
	if err != nil {
		return nil, nil, fmt.Errorf("collect day articles for tags: %w", err)
	}
	if len(articles) == 0 {
		return nil, nil, nil
	}

	// Adjudication layer (watch-materialize-llm-adjudication): filter the
	// tag-union down to articles actually fitting the sentence's intent —
	// the union itself never expressed the sentence's qualifiers.
	kept := articles
	var adj *watchAdjudicationResult
	if matCfg.Enabled {
		var aErr error
		adj, aErr = adjudicateWatchArticles(ctx, w.SemanticBoardID, w.ID, "一句话追踪："+watchQuerySentence(w), articles, matCfg, chat)
		if aErr != nil {
			logging.Warnf("watch-adjudicate: sentence watch %d degraded to recall result: %v", w.ID, aErr)
			adj = nil // degrade: keep the full union, default confidence, fallback title
		} else {
			kept = filterAdjudicated(articles, adj)
		}
	}
	if len(kept) == 0 {
		logging.Infof("watch-adjudicate: sentence watch %d — %d candidates all rejected, no section today", w.ID, len(articles))
		return nil, nil, nil // no fitting article — no section, no topic creation
	}

	// Dedicated topic: create at first materialization, reuse forever after
	// (design D4). Embedding+Centroid = query vector — Centroid is the lane
	// anchor, so it MUST be written alongside Embedding.
	topicID := w.PersistentTopicID
	if topicID == nil {
		pgVec := repository.FloatsToPgVector(queryVec)
		created, cErr := repository.Repo.CreateWatchTopic(w.SemanticBoardID, w.Label, pgVec, start)
		if cErr != nil {
			return nil, nil, fmt.Errorf("create watch persistent topic: %w", cErr)
		}
		topicID = &created
		if sErr := repository.Repo.SetWatchPersistentTopic(w.ID, created); sErr != nil {
			logging.Warnf("watch-materialize: topic link write-back failed for watch %d: %v", w.ID, sErr)
		}
	}

	watchID := w.ID
	section := &repository.DailyReportSection{
		ClusterLabel:      w.Label,
		WatchID:           &watchID,
		ClusterTagIDs:     mustMarshalUintArray(tagIDs),
		ArticleCount:      len(kept),
		BestTier:          4,
		AvgScore:          0,
		QualityBreakdown:  repository.JSON("{}"),
		LaneTier:          LaneTierWatchSentence,
		PersistentTopicID: topicID,
		// User-intent ownership: same semantics as the manual lane
		// (persistent-topic spec: manual = 用户主权声明), distance is not a
		// similarity measurement here — lane_tier carries the real origin.
		TopicMatchConfidence: "manual",
		TopicMatchDistance:   0,
	}
	if adj != nil && adj.Title != "" {
		section.ClusterLabel = adj.Title // LLM same-batch title; fallback = watch label
	}
	// Snapshot field: the topic is active by construction.
	active := repository.TopicStatusActive
	section.TopicStatusAtReport = &active

	threads := make([]repository.DailyReportThread, 0, len(kept))
	for _, a := range kept {
		threads = append(threads, repository.DailyReportThread{
			Title:             a.Title,
			Summary:           truncateRunes(a.Summary, keywordWatchSummaryRunes),
			TagIDs:            mustMarshalUintArray(parseWatchTagIDs(a.TagIDs)),
			Confidence:        1.0, // default; overridden by the adjudication verdict below
			RelatedArticleIDs: mustMarshalUintArray([]uint{a.ID}),
		})
	}
	applyThreadConfidence(threads, adj)

	logging.Infof("watch-materialize: watch %d section '%s' — %d labels → %d tags → %d articles (%d kept, topic %d)",
		w.ID, section.ClusterLabel, len(labels), len(tagIDs), len(articles), len(kept), *topicID)
	return section, threads, nil
}

// ArchiveWatchTopic archives a sentence_topic watch's dedicated persistent
// topic (soft archive: status=archived only — the user-explicit-action gate
// from the topic-graph invariants is satisfied by the handler's
// confirm_archive_topic requirement). Called before the watch row is deleted.
// Historical materialized sections keep their persistent_topic_id (snapshots
// are immutable); archived topics simply stop being anchorable.
func ArchiveWatchTopic(ctx context.Context, watchID uint) error {
	if repository.Repo == nil {
		return fmt.Errorf("repository not initialized")
	}
	watch, err := repository.Repo.GetWatchByID(watchID)
	if err != nil {
		return fmt.Errorf("load watch: %w", err)
	}
	if watch.PersistentTopicID == nil {
		return nil // topic never created (no materialization ever hit)
	}
	// Soft archive via the existing reversible path (UpdateTopic
	// status=archived) — the same operation the topic-management UI uses.
	archived := repository.TopicStatusArchived
	if _, err := repository.Repo.UpdateTopic(*watch.PersistentTopicID, nil, &archived); err != nil {
		return fmt.Errorf("archive topic %d: %w", *watch.PersistentTopicID, err)
	}
	logging.Infof("watch-materialize: archived dedicated topic %d of watch %d", *watch.PersistentTopicID, watchID)
	return nil
}
