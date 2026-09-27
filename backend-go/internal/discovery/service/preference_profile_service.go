package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// ── preference-profile service（design D1/D7，preference-profile spec）──
//
// 偏好向量 = 行为加权的标签向量质心，按 SemanticBoard 分桶 + 全局桶（board_id=NULL）。
// 重算零 LLM/embedding 调用（纯向量算术，复用 topic_tag_embeddings 的 semantic 轨）。
// source=behavior 行由 RecomputeAll 全量重建（不覆盖 source=seed 行）；
// source=seed 行由 WriteSeed 加权合并累积（D7/A）。

// PreferenceVectorSource 枚举 preference_vectors.source 取值。
const (
	PreferenceSourceBehavior = "behavior"
	PreferenceSourceSeed     = "seed"
)

// tagWeightsTopN 是画像可视化保留的 top 标签数。
const tagWeightsTopN = 10

// PreferenceProfileService 实现偏好画像的聚合、种子写入与读取。
type PreferenceProfileService struct {
	db *gorm.DB
}

// NewPreferenceProfileService 构造 service。
func NewPreferenceProfileService(db *gorm.DB) *PreferenceProfileService {
	return &PreferenceProfileService{db: db}
}

// RecomputeSummary 描述一次全量重算的产出。
type RecomputeSummary struct {
	BoardsComputed int // 产出向量的版块数（含全局桶）
	TagsUsed       int // 参与聚合的不同标签数
	ArticleCount   int // 参与聚合的文章数
}

// RecomputeAll 全量重建 source=behavior 的偏好向量（D1）。
// 幂等：先删除现有 source=behavior 行再重算；MUST NOT 触及 source=seed 行。
// 重算只读 topic_tag_embeddings 现有向量，不调用 LLM/embedding 接口。
func (s *PreferenceProfileService) RecomputeAll(ctx context.Context) (*RecomputeSummary, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "PreferenceProfileService.RecomputeAll")
	defer span.End()
	minTags := MinTagsPerBoardDefault

	// 1. 近 30 天行为 → 每文章权重（behavior_level × time_decay）。
	articleWeights, err := s.computeArticleWeights(ctx)
	if err != nil {
		return nil, fmt.Errorf("compute article weights: %w", err)
	}
	if len(articleWeights) == 0 {
		// 无行为数据：清空 behavior 行（保持表为行为的纯派生），seed 行不动。
		if err := s.db.WithContext(ctx).Where("source = ?", PreferenceSourceBehavior).
			Delete(&discmodels.PreferenceVector{}).Error; err != nil {
			return nil, fmt.Errorf("clear behavior rows: %w", err)
		}
		return &RecomputeSummary{}, nil
	}

	// 2. 文章权重 → 标签权重，按版块分桶（+ 全局桶）。
	boardTagWeights, globalTagWeights, tagLabels, err := s.distributeTagWeights(ctx, articleWeights)
	if err != nil {
		return nil, fmt.Errorf("distribute tag weights: %w", err)
	}

	// 3. 取所有相关标签的 semantic 向量。
	tagIDs := collectTagIDs(boardTagWeights, globalTagWeights)
	tagVecs, embModel, embDim, err := s.loadTagEmbeddings(ctx, tagIDs)
	if err != nil {
		return nil, fmt.Errorf("load tag embeddings: %w", err)
	}

	// 4. 每桶算质心向量（normalize(Σ w×vec)）；不足最小标签数的桶其标签并入全局。
	boardCentroids, globalCentroid, mergedGlobalTags, boardsComputed := s.computeCentroids(
		boardTagWeights, globalTagWeights, tagVecs, minTags)

	// 5. 持久化：事务内删 behavior 行 + 插新行（不动 seed）。
	rows := buildPreferenceRows(boardCentroids, globalCentroid,
		boardTagWeights, mergedGlobalTags, tagLabels, embModel, embDim, time.Now())
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source = ?", PreferenceSourceBehavior).
			Delete(&discmodels.PreferenceVector{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			return tx.Create(&rows).Error
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("persist behavior rows: %w", err)
	}

	logging.Infof("preference recompute: boards=%d tags=%d articles=%d",
		boardsComputed, len(tagIDs), len(articleWeights))
	return &RecomputeSummary{
		BoardsComputed: boardsComputed,
		TagsUsed:       len(tagIDs),
		ArticleCount:   len(articleWeights),
	}, nil
}

// computeArticleWeights 聚合近 30 天 reading_behaviors，按文章返回 D1 权重。
func (s *PreferenceProfileService) computeArticleWeights(ctx context.Context) (map[uint]float64, error) {
	type row struct {
		ArticleID uint
		HasFav    int
		MaxScroll int
		MaxTime   int
		LastAt    time.Time
	}
	var rows []row
	err := s.db.WithContext(ctx).Raw(`
		SELECT article_id,
			MAX(CASE WHEN event_type = 'favorite' THEN 1 ELSE 0 END) AS has_fav,
			MAX(scroll_depth) AS max_scroll,
			MAX(reading_time) AS max_time,
			MAX(created_at) AS last_at
		FROM reading_behaviors
		WHERE created_at >= NOW() - INTERVAL '30 days'
		GROUP BY article_id
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	weights := make(map[uint]float64, len(rows))
	now := time.Now()
	for _, r := range rows {
		level := articleBehaviorLevel(r.HasFav > 0, r.MaxScroll, r.MaxTime)
		if level == 0 {
			continue
		}
		days := now.Sub(r.LastAt).Hours() / 24
		weights[r.ArticleID] = level * timeDecay(days)
	}
	return weights, nil
}

// distributeTagWeights 把文章权重经 article_topic_tags 分发到标签，
// 再经 topic_tag_board_labels 分桶到版块；无版块归属的标签计入全局桶。
func (s *PreferenceProfileService) distributeTagWeights(
	ctx context.Context, articleWeights map[uint]float64,
) (map[uint]map[uint]float64, map[uint]float64, map[uint]string, error) {
	if len(articleWeights) == 0 {
		return nil, nil, nil, nil
	}
	articleIDs := make([]uint, 0, len(articleWeights))
	for id := range articleWeights {
		articleIDs = append(articleIDs, id)
	}

	type tagLink struct {
		ArticleID  uint
		TopicTagID uint
		Label      string
	}
	var links []tagLink
	err := s.db.WithContext(ctx).Raw(`
		SELECT att.article_id, att.topic_tag_id, tt.label
		FROM article_topic_tags att
		JOIN topic_tags tt ON tt.id = att.topic_tag_id
		WHERE tt.status = 'active' AND att.article_id IN ?
	`, articleIDs).Scan(&links).Error
	if err != nil {
		return nil, nil, nil, err
	}

	tagIDs := make([]uint, 0, len(links))
	seen := make(map[uint]struct{})
	for _, l := range links {
		if _, ok := seen[l.TopicTagID]; !ok {
			seen[l.TopicTagID] = struct{}{}
			tagIDs = append(tagIDs, l.TopicTagID)
		}
	}
	type boardLink struct {
		TopicTagID      uint
		SemanticBoardID uint
	}
	var bLinks []boardLink
	tagBoardMap := make(map[uint][]uint)
	if len(tagIDs) > 0 {
		err = s.db.WithContext(ctx).Raw(`
			SELECT topic_tag_id, semantic_board_id
			FROM topic_tag_board_labels
			WHERE topic_tag_id IN ?
		`, tagIDs).Scan(&bLinks).Error
		if err != nil {
			return nil, nil, nil, err
		}
		for _, bl := range bLinks {
			tagBoardMap[bl.TopicTagID] = append(tagBoardMap[bl.TopicTagID], bl.SemanticBoardID)
		}
	}

	boardTagWeights := make(map[uint]map[uint]float64)
	globalTagWeights := make(map[uint]float64)
	tagLabels := make(map[uint]string)
	for _, l := range links {
		w := articleWeights[l.ArticleID]
		if w == 0 {
			continue
		}
		tagLabels[l.TopicTagID] = l.Label
		boards := tagBoardMap[l.TopicTagID]
		if len(boards) == 0 {
			globalTagWeights[l.TopicTagID] += w
			continue
		}
		for _, b := range boards {
			if boardTagWeights[b] == nil {
				boardTagWeights[b] = make(map[uint]float64)
			}
			boardTagWeights[b][l.TopicTagID] += w
		}
	}
	return boardTagWeights, globalTagWeights, tagLabels, nil
}

// loadTagEmbeddings 批量取标签的 semantic 轨向量（pgvector 文本 → []float64）。
func (s *PreferenceProfileService) loadTagEmbeddings(
	ctx context.Context, tagIDs []uint,
) (map[uint][]float64, string, int, error) {
	vecs := make(map[uint][]float64)
	if len(tagIDs) == 0 {
		return vecs, "", 0, nil
	}
	type embRow struct {
		TopicTagID uint
		Embedding  string
		Dimension  int
		Model      string
	}
	var rows []embRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT topic_tag_id, embedding, dimension, model
		FROM topic_tag_embeddings
		WHERE embedding_type = 'semantic' AND topic_tag_id IN ?
	`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, "", 0, err
	}
	var model string
	dim := 0
	for _, r := range rows {
		v, perr := parsePgVector(r.Embedding)
		if perr != nil || len(v) == 0 {
			continue
		}
		vecs[r.TopicTagID] = v
		if model == "" {
			model = r.Model
			dim = r.Dimension
		}
	}
	return vecs, model, dim, nil
}

// computeCentroids 对每桶算 normalize(Σ w×vec)；不足最小标签数的桶其标签并入全局。
// 返回 boardCentroids、globalCentroid、合并后的全局标签权重（含原全局 + 不足阈值并入）、产出桶数。
func (s *PreferenceProfileService) computeCentroids(
	boardTagWeights map[uint]map[uint]float64,
	globalTagWeights map[uint]float64,
	tagVecs map[uint][]float64,
	minTags int,
) (map[uint][]float64, []float64, map[uint]float64, int) {
	boardCentroids := make(map[uint][]float64)
	globalAcc := make(map[uint]float64)
	for tag, w := range globalTagWeights {
		globalAcc[tag] += w
	}
	boardsComputed := 0
	for board, tags := range boardTagWeights {
		if len(tags) < minTags {
			for tag, w := range tags {
				globalAcc[tag] += w
			}
			continue
		}
		centroid := weightedCentroid(tags, tagVecs)
		if centroid == nil {
			for tag, w := range tags {
				globalAcc[tag] += w
			}
			continue
		}
		boardCentroids[board] = centroid
		boardsComputed++
	}
	var globalCentroid []float64
	if len(globalAcc) > 0 {
		globalCentroid = weightedCentroid(globalAcc, tagVecs)
	}
	return boardCentroids, globalCentroid, globalAcc, boardsComputed
}

// weightedCentroid 算 normalize(Σ_tag w(tag) × vec(tag))；无任何可用向量或维度不一致返回 nil。
func weightedCentroid(tagWeights map[uint]float64, tagVecs map[uint][]float64) []float64 {
	var sum []float64
	for tag, w := range tagWeights {
		v, ok := tagVecs[tag]
		if !ok || len(v) == 0 {
			continue
		}
		if sum == nil {
			sum = make([]float64, len(v))
		}
		if len(v) != len(sum) {
			return nil // 维度不一致：拒绝混合（design Risks）
		}
		for i := range v {
			sum[i] += w * v[i]
		}
	}
	return normalizeVector(sum)
}

// buildPreferenceRows 构造 source=behavior 的持久化行（版块桶 + 全局桶），含 top 标签权重。
func buildPreferenceRows(
	boardCentroids map[uint][]float64,
	globalCentroid []float64,
	boardTagWeights map[uint]map[uint]float64,
	globalTagWeights map[uint]float64,
	tagLabels map[uint]string,
	model string, dim int, now time.Time,
) []discmodels.PreferenceVector {
	var rows []discmodels.PreferenceVector
	for board, vec := range boardCentroids {
		rows = append(rows, discmodels.PreferenceVector{
			BoardID:        &board,
			Source:         PreferenceSourceBehavior,
			EmbeddingVec:   floatsToPgVector(vec),
			Dimension:      dim,
			Model:          model,
			TagWeights:     topTagWeights(boardTagWeights[board], tagLabels),
			LastComputedAt: now,
		})
	}
	if globalCentroid != nil {
		rows = append(rows, discmodels.PreferenceVector{
			BoardID:        nil,
			Source:         PreferenceSourceBehavior,
			EmbeddingVec:   floatsToPgVector(globalCentroid),
			Dimension:      dim,
			Model:          model,
			TagWeights:     topTagWeights(globalTagWeights, tagLabels),
			LastComputedAt: now,
		})
	}
	return rows
}

// topTagWeights 取权重 top-N 标签 → {label: weight}（画像可视化用）。
func topTagWeights(tagWeights map[uint]float64, tagLabels map[uint]string) models.MetadataMap {
	if len(tagWeights) == 0 {
		return models.MetadataMap{}
	}
	type kv struct {
		id uint
		w  float64
	}
	list := make([]kv, 0, len(tagWeights))
	for id, w := range tagWeights {
		list = append(list, kv{id, w})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].w > list[j].w })
	if len(list) > tagWeightsTopN {
		list = list[:tagWeightsTopN]
	}
	out := models.MetadataMap{}
	for _, x := range list {
		label := tagLabels[x.id]
		if label == "" {
			label = fmt.Sprintf("tag#%d", x.id)
		}
		out[label] = x.w
	}
	return out
}

// collectTagIDs 汇总分桶涉及的标签 id。
func collectTagIDs(boardTagWeights map[uint]map[uint]float64, globalTagWeights map[uint]float64) []uint {
	seen := make(map[uint]struct{})
	var ids []uint
	add := func(id uint) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, tags := range boardTagWeights {
		for t := range tags {
			add(t)
		}
	}
	for t := range globalTagWeights {
		add(t)
	}
	return ids
}

// PreferenceProfileItem 是 GET /api/preference-profile 的单条画像。
type PreferenceProfileItem struct {
	BoardID        *uint              `json:"board_id,omitempty"`
	BoardLabel     string             `json:"board_label"`
	Source         string             `json:"source"`
	TagWeights     models.MetadataMap `json:"tag_weights"`
	Dimension      int                `json:"dimension"`
	Model          string             `json:"model"`
	LastComputedAt *time.Time         `json:"last_computed_at,omitempty"`
}

// GetProfile 返回兴趣画像（按版块分组 + 全局桶），无数据返回空列表。
func (s *PreferenceProfileService) GetProfile(ctx context.Context) ([]PreferenceProfileItem, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "PreferenceProfileService.GetProfile")
	defer span.End()
	var vecs []discmodels.PreferenceVector
	err := s.db.WithContext(ctx).
		Preload("Board").
		Order("board_id ASC NULLS LAST, source ASC").
		Find(&vecs).Error
	if err != nil {
		return nil, err
	}
	items := make([]PreferenceProfileItem, 0, len(vecs))
	for _, v := range vecs {
		label := "全局"
		if v.Board != nil {
			label = v.Board.Label
		}
		var last *time.Time
		if !v.LastComputedAt.IsZero() {
			last = &v.LastComputedAt
		}
		items = append(items, PreferenceProfileItem{
			BoardID:        v.BoardID,
			BoardLabel:     label,
			Source:         v.Source,
			TagWeights:     v.TagWeights,
			Dimension:      v.Dimension,
			Model:          v.Model,
			LastComputedAt: last,
		})
	}
	return items, nil
}

// WriteSeed 将兴趣文本向量写入 source=seed 行（D7/A 加权合并累积）。
// boardVecs 为各版块向量（用于匹配落版块）；incomingVec 为问题文本 embedding。
// 维度与现有 seed 行不一致时拒绝（返回错误），避免污染画像。
func (s *PreferenceProfileService) WriteSeed(
	ctx context.Context, incomingVec []float64, dim int, model string,
	boardVecs map[uint][]float64,
) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "PreferenceProfileService.WriteSeed")
	defer span.End()
	if len(incomingVec) == 0 {
		return nil
	}
	boardID := matchSeedBoard(incomingVec, boardVecs, SeedMatchThresholdDefault)

	var existing discmodels.PreferenceVector
	mergedVec := incomingVec
	mergedWeights := models.MetadataMap{}
	err := s.db.WithContext(ctx).
		Where("board_id IS NOT DISTINCT FROM ? AND source = ?", boardID, PreferenceSourceSeed).
		First(&existing).Error
	if err == nil {
		existingVec, perr := parsePgVector(existing.EmbeddingVec)
		if perr == nil && len(existingVec) > 0 {
			if len(existingVec) != len(incomingVec) {
				return fmt.Errorf("seed 维度不一致: existing=%d incoming=%d（需重新问答重建种子）",
					len(existingVec), len(incomingVec))
			}
			mergedVec = mergeSeedVectors(incomingVec, existingVec, SeedMergeAlphaDefault)
			mergedWeights = existing.TagWeights
		}
	} else if !isNotFound(err) {
		return fmt.Errorf("query existing seed: %w", err)
	}

	now := time.Now()
	row := discmodels.PreferenceVector{
		BoardID:        boardID,
		Source:         PreferenceSourceSeed,
		EmbeddingVec:   floatsToPgVector(mergedVec),
		Dimension:      dim,
		Model:          model,
		TagWeights:     mergedWeights,
		LastComputedAt: now,
	}
	// upsert：同 (board_id, source=seed) 单行。board_id 可空 → 应用层 find-or-create 保证单行。
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cur discmodels.PreferenceVector
		q := tx.Where("board_id IS NOT DISTINCT FROM ? AND source = ?", boardID, PreferenceSourceSeed)
		if e := q.First(&cur).Error; e == nil {
			row.ID = cur.ID
			return tx.Save(&row).Error
		} else if !isNotFound(e) {
			return e
		}
		return tx.Create(&row).Error
	})
}

// matchSeedBoard 返回 incomingVec 最匹配（余弦 ≥ 阈值）的版块 id；无匹配返回 nil（全局桶）。
func matchSeedBoard(incoming []float64, boardVecs map[uint][]float64, threshold float64) *uint {
	keys := make([]uint, 0, len(boardVecs))
	for k := range boardVecs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var bestBoard uint
	bestSim := threshold
	hit := false
	for _, b := range keys {
		sim, ok := cosineSim(incoming, boardVecs[b])
		if !ok {
			continue
		}
		if sim >= bestSim {
			bestSim = sim
			bestBoard = b
			hit = true
		}
	}
	if !hit {
		return nil
	}
	return &bestBoard
}

// cosineSim 余弦相似度；维度不一致或零向量返回 (0,false)/(0,true)。
func cosineSim(a, b []float64) (float64, bool) {
	if len(a) != len(b) || len(a) == 0 {
		return 0, false
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0, true
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), true
}

// isNotFound 判断 gorm 未找到错误。
func isNotFound(err error) bool {
	return err != nil && err == gorm.ErrRecordNotFound
}
