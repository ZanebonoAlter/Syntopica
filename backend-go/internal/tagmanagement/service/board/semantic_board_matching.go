package board

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
)

type SemanticBoardMatchingService struct {
	db    *gorm.DB
	cache *boardCache
}

func NewSemanticBoardMatchingService(db *gorm.DB) *SemanticBoardMatchingService {
	if db == nil {
		db = repository.Repo.DB()
	}
	return &SemanticBoardMatchingService{db: db, cache: packageBoardCache}
}

var (
	semanticBoardMatchingService     *SemanticBoardMatchingService
	semanticBoardMatchingServiceOnce sync.Once
)

func getSemanticBoardMatchingService() *SemanticBoardMatchingService {
	semanticBoardMatchingServiceOnce.Do(func() {
		semanticBoardMatchingService = NewSemanticBoardMatchingService(repository.Repo.DB())
	})
	return semanticBoardMatchingService
}

type SemanticBoardMatchConfig struct {
	SimThreshold           float64
	DirectHitRate          float64
	DirectMaxSim           float64
	DirectMaxSimMinHits    int
	DirectMaxSimMinHitRate float64
	MinEffectiveSample     int
	HitRateSimBlend        float64
	WeightSim              float64
	WeightDensity          float64
	WeightedThreshold      float64
	MaxBoards              int
	DirectHitMinOverlap    int
	DirectionSimThreshold  float64
	DirectHitScoreFactor   float64
}

type SemanticBoardMatchResult struct {
	SemanticBoardID   uint
	Score             float64
	MatchReason       string
	Downgraded        bool
	DirectionMismatch bool
}

// BoardCompositeLabel is one composite-label row mounted on a board via
// board_composition (the auxiliary_label_id column is reused for composites).
type BoardCompositeLabel struct {
	BoardID          uint
	CompositeLabelID uint
	Label            string
	Embedding        *string
}

type BoardAuxiliaryLabel struct {
	BoardID          uint
	AuxiliaryLabelID uint
	Label            string
	Embedding        *string
}

func (s *SemanticBoardMatchingService) MatchTopicTag(ctx context.Context, topicTagID uint) ([]SemanticBoardMatchResult, error) {
	if topicTagID == 0 {
		return nil, fmt.Errorf("topic tag id is required")
	}

	config := s.LoadConfig(ctx)
	tagAuxiliaries, err := s.LoadTagAuxiliaries(ctx, topicTagID)
	if err != nil {
		return nil, err
	}
	tagAuxIDs := make(map[uint]struct{}, len(tagAuxiliaries))
	for _, a := range tagAuxiliaries {
		tagAuxIDs[a.ID] = struct{}{}
	}
	tagComposites, err := s.LoadTagComposites(ctx, topicTagID, tagAuxIDs)
	if err != nil {
		return nil, err
	}
	if len(tagAuxiliaries) == 0 && len(tagComposites) == 0 {
		return []SemanticBoardMatchResult{}, s.replaceTopicTagBoardLabels(ctx, topicTagID, nil)
	}

	boardAuxiliaries, err := s.loadBoardAuxiliaries(ctx)
	if err != nil {
		return nil, err
	}
	boardComposites, err := s.loadBoardComposites(ctx)
	if err != nil {
		return nil, err
	}
	if len(boardAuxiliaries) == 0 && len(boardComposites) == 0 {
		return []SemanticBoardMatchResult{}, s.replaceTopicTagBoardLabels(ctx, topicTagID, nil)
	}

	boardAuxiliariesFiltered := filterBoardAuxiliaries(boardAuxiliaries, config.DirectHitMinOverlap)
	if len(boardAuxiliariesFiltered) == 0 && len(boardComposites) == 0 {
		return []SemanticBoardMatchResult{}, s.replaceTopicTagBoardLabels(ctx, topicTagID, nil)
	}

	tagEmbedding, _ := s.LoadTagIdentityEmbedding(ctx, topicTagID)
	boardEmbeddings, _ := s.loadBoardEmbeddings(ctx)

	matches := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliariesFiltered, tagComposites, boardComposites, config, tagEmbedding, boardEmbeddings)
	if len(matches) > config.MaxBoards {
		matches = matches[:config.MaxBoards]
	}
	if err := s.replaceTopicTagBoardLabels(ctx, topicTagID, matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (s *SemanticBoardMatchingService) LoadTagAuxiliaries(ctx context.Context, topicTagID uint) ([]models.SemanticLabel, error) {
	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.semantic_label_id = semantic_labels.id").
		Where("topic_tag_semantic_labels.topic_tag_id = ? AND semantic_labels.label_type = ? AND semantic_labels.status = ?", topicTagID, "auxiliary", "active").
		Find(&labels).Error
	return labels, err
}

func (s *SemanticBoardMatchingService) loadBoardAuxiliaries(ctx context.Context) ([]BoardAuxiliaryLabel, error) {
	if cached, ok := s.cache.GetBoardAuxiliaries(); ok {
		return cached, nil
	}
	var labels []BoardAuxiliaryLabel
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
		Scan(&labels).Error
	if err != nil {
		return nil, err
	}
	s.cache.SetBoardAuxiliaries(labels)
	return labels, nil
}

// LoadTagComposites 返回 tag 的组合标签：显式关联（topic_tag_semantic_labels，
// 提取端将来写入）∪ 推导命中（tag 挂齐某 active 组合的全部组件 aux ⇒ 视为挂
// 该组合——确认组合后重算即生效的闭环来源，add-composite-labels）。
// auxIDs 为 tag 当前挂载的 aux ID 集合（调用方已加载，避免重查）；nil 表示
// 调用方未提供，此时仅返回显式关联。
func (s *SemanticBoardMatchingService) LoadTagComposites(ctx context.Context, topicTagID uint, auxIDs map[uint]struct{}) ([]models.SemanticLabel, error) {
	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.semantic_label_id = semantic_labels.id").
		Where("topic_tag_semantic_labels.topic_tag_id = ? AND semantic_labels.label_type = ? AND semantic_labels.status = ?", topicTagID, "composite", "active").
		Find(&labels).Error
	if err != nil || auxIDs == nil {
		return labels, err
	}

	sets, err := s.loadAllCompositeSets(ctx)
	if err != nil {
		return nil, err
	}
	if len(sets) == 0 {
		return labels, nil
	}
	explicit := make(map[uint]struct{}, len(labels))
	for _, l := range labels {
		explicit[l.ID] = struct{}{}
	}
	var derivedIDs []uint
	for _, set := range sets {
		if _, ok := explicit[set.CompositeLabelID]; ok {
			continue
		}
		if len(set.ComponentIDs) == 0 {
			continue
		}
		complete := true
		for _, cid := range set.ComponentIDs {
			if _, ok := auxIDs[cid]; !ok {
				complete = false
				break
			}
		}
		if complete {
			derivedIDs = append(derivedIDs, set.CompositeLabelID)
		}
	}
	if len(derivedIDs) == 0 {
		return labels, nil
	}
	var derived []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND label_type = ? AND status = ?", derivedIDs, "composite", "active").
		Find(&derived).Error; err != nil {
		return nil, err
	}
	return append(labels, derived...), nil
}

// loadAllCompositeSets 加载全部 active 组合的组件 ID 集合（组合数量小，整表缓存，
// 随 InvalidateBoardData 一起失效——组合创建/禁用路径都会触发）。
func (s *SemanticBoardMatchingService) loadAllCompositeSets(ctx context.Context) ([]CompositeComponentSet, error) {
	if cached, ok := s.cache.GetAllCompositeSets(); ok {
		return cached, nil
	}
	var rows []models.CompositeComponent
	if err := s.db.WithContext(ctx).
		Joins("JOIN semantic_labels ON semantic_labels.id = composite_components.composite_id AND semantic_labels.label_type = ? AND semantic_labels.status = ?", "composite", "active").
		Order("composite_id, position").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	byComposite := make(map[uint][]uint)
	for _, r := range rows {
		byComposite[r.CompositeID] = append(byComposite[r.CompositeID], r.ComponentLabelID)
	}
	sets := make([]CompositeComponentSet, 0, len(byComposite))
	for cid, comps := range byComposite {
		sets = append(sets, CompositeComponentSet{CompositeLabelID: cid, ComponentIDs: comps})
	}
	s.cache.SetAllCompositeSets(sets)
	return sets, nil
}

func (s *SemanticBoardMatchingService) loadBoardComposites(ctx context.Context) ([]BoardCompositeLabel, error) {
	if cached, ok := s.cache.GetBoardComposites(); ok {
		return cached, nil
	}
	var labels []BoardCompositeLabel
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id AS composite_label_id, composite.label, composite.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS composite ON composite.id = board_composition.auxiliary_label_id AND composite.label_type = ? AND composite.status = ?", "composite", "active").
		Scan(&labels).Error
	if err != nil {
		return nil, err
	}
	s.cache.SetBoardComposites(labels)
	return labels, nil
}

func (s *SemanticBoardMatchingService) LoadTagIdentityEmbedding(ctx context.Context, topicTagID uint) ([]float64, error) {
	var emb models.TopicTagEmbedding
	err := s.db.WithContext(ctx).
		Where("topic_tag_id = ? AND embedding_type = ?", topicTagID, "identity").
		First(&emb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if emb.EmbeddingVec == "" {
		return nil, nil
	}
	return auxlabel.ParsePgVector(emb.EmbeddingVec)
}

func (s *SemanticBoardMatchingService) loadBoardEmbeddings(ctx context.Context) (map[uint][]float64, error) {
	if cached, ok := s.cache.GetBoardEmbeddings(); ok {
		return cached, nil
	}
	type boardEmbeddingRow struct {
		ID        uint    `gorm:"column:id"`
		Embedding *string `gorm:"column:embedding"`
	}
	var rows []boardEmbeddingRow
	err := s.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Select("id, embedding").
		Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "board", "active").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uint][]float64, len(rows))
	for _, row := range rows {
		if row.Embedding == nil {
			continue
		}
		vec, err := auxlabel.ParsePgVector(*row.Embedding)
		if err != nil {
			continue
		}
		result[row.ID] = vec
	}
	s.cache.SetBoardEmbeddings(result)
	return result, nil
}

func (s *SemanticBoardMatchingService) LoadBoardAuxiliariesByBoardID(ctx context.Context, boardID uint) ([]BoardAuxiliaryLabel, error) {
	var labels []BoardAuxiliaryLabel
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
		Where("board_composition.board_id = ?", boardID).
		Scan(&labels).Error
	return labels, err
}

func evaluateSemanticBoardMatches(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []BoardAuxiliaryLabel, tagComposites []models.SemanticLabel, boardComposites []BoardCompositeLabel, config SemanticBoardMatchConfig, tagEmbedding []float64, boardEmbeddings map[uint][]float64) []SemanticBoardMatchResult {
	tagAuxiliaryIDs := make(map[uint]struct{}, len(tagAuxiliaries))
	tagVectors := make([][]float64, 0, len(tagAuxiliaries))
	for _, label := range tagAuxiliaries {
		tagAuxiliaryIDs[label.ID] = struct{}{}
		if label.Embedding == nil {
			continue
		}
		vector, err := auxlabel.ParsePgVector(*label.Embedding)
		if err == nil {
			tagVectors = append(tagVectors, vector)
		}
	}

	// Composite labels without an embedding (disabled rows have NULL vectors)
	// never participate in composite_hit.
	tagCompositeIDs := make(map[uint]struct{}, len(tagComposites))
	for _, label := range tagComposites {
		if label.Embedding == nil {
			continue
		}
		tagCompositeIDs[label.ID] = struct{}{}
	}

	grouped := make(map[uint][]BoardAuxiliaryLabel)
	for _, auxiliary := range boardAuxiliaries {
		grouped[auxiliary.BoardID] = append(grouped[auxiliary.BoardID], auxiliary)
	}
	compositesByBoard := make(map[uint][]uint)
	for _, composite := range boardComposites {
		if composite.Embedding == nil {
			continue
		}
		compositesByBoard[composite.BoardID] = append(compositesByBoard[composite.BoardID], composite.CompositeLabelID)
	}

	allBoards := make(map[uint]struct{}, len(grouped)+len(compositesByBoard))
	for boardID := range grouped {
		allBoards[boardID] = struct{}{}
	}
	for boardID := range compositesByBoard {
		allBoards[boardID] = struct{}{}
	}

	// Direction check applies to every match reason except composite_hit
	// (a composite hit already implies aligned direction). Missing tag or
	// board embeddings keep directionMismatch=false, matching the indirect
	// rules' existing semantics.
	directionMismatch := func(boardID uint) bool {
		if len(tagEmbedding) == 0 {
			return false
		}
		boardEmb, ok := boardEmbeddings[boardID]
		if !ok || len(boardEmb) == 0 {
			return false
		}
		return CosineSimilarity(tagEmbedding, boardEmb) < config.DirectionSimThreshold
	}

	matches := make([]SemanticBoardMatchResult, 0, len(allBoards))
	for boardID := range allBoards {
		auxiliaries := grouped[boardID]

		// Priority 1: composite_hit — strongest signal, score 1.0, exempt
		// from direction check. When a tag-board pair also has single-label
		// overlap, only composite_hit is recorded.
		if hasCompositeIntersection(tagCompositeIDs, compositesByBoard[boardID]) {
			matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: 1.0, MatchReason: "composite_hit"})
			continue
		}

		// Priority 2: direct_hit — downgraded to DirectHitScoreFactor and now
		// subject to the direction check like every non-composite reason.
		if countDirectSemanticBoardHits(tagAuxiliaryIDs, auxiliaries) >= config.DirectHitMinOverlap {
			matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: config.DirectHitScoreFactor, MatchReason: "direct_hit", DirectionMismatch: directionMismatch(boardID)})
			continue
		}

		// Priority 3: indirect similarity rules (unchanged).
		if len(tagVectors) == 0 {
			continue
		}
		boardVectors := parseBoardAuxiliaryVectors(auxiliaries)
		if len(boardVectors) == 0 {
			continue
		}

		hitRate, maxSimilarity := scoreSemanticBoardSimilarity(tagVectors, boardVectors, len(tagAuxiliaries), config.SimThreshold, config.MinEffectiveSample)
		weighted := config.WeightSim*maxSimilarity + config.WeightDensity*hitRate
		score := 0.0
		matchReason := ""
		hits := int(math.Round(hitRate * float64(max(len(tagAuxiliaries), config.MinEffectiveSample))))
		minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))
		downgraded := false
		switch {
		case hitRate > config.DirectHitRate:
			score = config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate
			matchReason = "hit_rate"
		case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
			score = maxSimilarity
			matchReason = "max_sim"
			if minHits < config.DirectMaxSimMinHits {
				downgraded = true
			}
		case weighted >= config.WeightedThreshold:
			score = weighted
			matchReason = "weighted"
		}

		if matchReason != "" {
			matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: score, MatchReason: matchReason, Downgraded: downgraded, DirectionMismatch: directionMismatch(boardID)})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].SemanticBoardID < matches[j].SemanticBoardID
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}

// hasCompositeIntersection reports whether any board-mounted composite is
// also attached to the tag.
func hasCompositeIntersection(tagCompositeIDs map[uint]struct{}, boardCompositeIDs []uint) bool {
	if len(tagCompositeIDs) == 0 || len(boardCompositeIDs) == 0 {
		return false
	}
	for _, compositeID := range boardCompositeIDs {
		if _, ok := tagCompositeIDs[compositeID]; ok {
			return true
		}
	}
	return false
}

type MatchDetailPair struct {
	TagAuxiliaryID      uint
	TagAuxiliaryLabel   string
	BoardAuxiliaryID    uint
	BoardAuxiliaryLabel string
	Similarity          float64
	IsHit               bool
}

type computedMatchDetail struct {
	Hits          int
	HitRate       float64
	MaxSimilarity float64
	Pairs         []MatchDetailPair
}

func ComputeMatchDetail(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []BoardAuxiliaryLabel, config SemanticBoardMatchConfig) computedMatchDetail {
	detail := computedMatchDetail{Pairs: []MatchDetailPair{}}
	if len(tagAuxiliaries) == 0 || len(boardAuxiliaries) == 0 {
		return detail
	}

	boardVectors := make([]struct {
		label  BoardAuxiliaryLabel
		vector []float64
	}, 0, len(boardAuxiliaries))
	for _, boardAuxiliary := range boardAuxiliaries {
		if boardAuxiliary.Embedding == nil {
			continue
		}
		vector, err := auxlabel.ParsePgVector(*boardAuxiliary.Embedding)
		if err == nil {
			boardVectors = append(boardVectors, struct {
				label  BoardAuxiliaryLabel
				vector []float64
			}{label: boardAuxiliary, vector: vector})
		}
	}
	if len(boardVectors) == 0 {
		return detail
	}

	for _, tagAuxiliary := range tagAuxiliaries {
		if tagAuxiliary.Embedding == nil {
			continue
		}
		tagVector, err := auxlabel.ParsePgVector(*tagAuxiliary.Embedding)
		if err != nil {
			continue
		}

		bestSimilarity := -1.0
		var bestBoard BoardAuxiliaryLabel
		for _, boardVector := range boardVectors {
			similarity, err := airouter.CosineSimilarity(tagVector, boardVector.vector)
			if err != nil {
				continue
			}
			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestBoard = boardVector.label
			}
			if similarity > detail.MaxSimilarity {
				detail.MaxSimilarity = similarity
			}
		}
		if bestSimilarity < 0 {
			continue
		}

		isHit := bestSimilarity >= config.SimThreshold
		if isHit {
			detail.Hits++
		}
		detail.Pairs = append(detail.Pairs, MatchDetailPair{
			TagAuxiliaryID:      tagAuxiliary.ID,
			TagAuxiliaryLabel:   tagAuxiliary.Label,
			BoardAuxiliaryID:    bestBoard.AuxiliaryLabelID,
			BoardAuxiliaryLabel: bestBoard.Label,
			Similarity:          bestSimilarity,
			IsHit:               isHit,
		})
	}

	if len(tagAuxiliaries) > 0 {
		denominator := math.Max(float64(len(tagAuxiliaries)), float64(config.MinEffectiveSample))
		if denominator > 0 {
			detail.HitRate = float64(detail.Hits) / denominator
		}
	}
	return detail
}

func filterBoardAuxiliaries(auxiliaries []BoardAuxiliaryLabel, minAuxiliaries int) []BoardAuxiliaryLabel {
	if minAuxiliaries <= 1 {
		return auxiliaries
	}
	counts := make(map[uint]int)
	for _, a := range auxiliaries {
		counts[a.BoardID]++
	}
	valid := make(map[uint]struct{})
	for boardID, count := range counts {
		if count >= minAuxiliaries {
			valid[boardID] = struct{}{}
		}
	}
	if len(valid) == len(counts) {
		return auxiliaries
	}
	filtered := make([]BoardAuxiliaryLabel, 0, len(auxiliaries))
	for _, a := range auxiliaries {
		if _, ok := valid[a.BoardID]; ok {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func countDirectSemanticBoardHits(tagAuxiliaryIDs map[uint]struct{}, boardAuxiliaries []BoardAuxiliaryLabel) int {
	count := 0
	for _, auxiliary := range boardAuxiliaries {
		if _, ok := tagAuxiliaryIDs[auxiliary.AuxiliaryLabelID]; ok {
			count++
		}
	}
	return count
}

func parseBoardAuxiliaryVectors(auxiliaries []BoardAuxiliaryLabel) [][]float64 {
	vectors := make([][]float64, 0, len(auxiliaries))
	for _, auxiliary := range auxiliaries {
		if auxiliary.Embedding == nil {
			continue
		}
		vector, err := auxlabel.ParsePgVector(*auxiliary.Embedding)
		if err == nil {
			vectors = append(vectors, vector)
		}
	}
	return vectors
}

func scoreSemanticBoardSimilarity(tagVectors [][]float64, boardVectors [][]float64, tagAuxiliaryCount int, threshold float64, minEffectiveSample int) (float64, float64) {
	hits := 0
	maxSimilarity := 0.0
	for _, tagVector := range tagVectors {
		bestSimilarity := 0.0
		for _, boardVector := range boardVectors {
			similarity, err := airouter.CosineSimilarity(tagVector, boardVector)
			if err != nil {
				continue
			}
			if similarity > bestSimilarity {
				bestSimilarity = similarity
			}
			if similarity > maxSimilarity {
				maxSimilarity = similarity
			}
		}
		if bestSimilarity >= threshold {
			hits++
		}
	}
	effectiveDenominator := math.Max(float64(tagAuxiliaryCount), float64(minEffectiveSample))
	return float64(hits) / effectiveDenominator, maxSimilarity
}

func (s *SemanticBoardMatchingService) replaceTopicTagBoardLabels(ctx context.Context, topicTagID uint, matches []SemanticBoardMatchResult) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("topic_tag_id = ?", topicTagID).Delete(&models.TopicTagBoardLabel{}).Error; err != nil {
			return err
		}
		for _, match := range matches {
			row := models.TopicTagBoardLabel{TopicTagID: topicTagID, SemanticBoardID: match.SemanticBoardID, Score: match.Score, MatchReason: match.MatchReason, Downgraded: match.Downgraded, DirectionMismatch: match.DirectionMismatch}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SemanticBoardMatchingService) LoadConfig(ctx context.Context) SemanticBoardMatchConfig {
	if cached, ok := s.cache.GetConfig(); ok {
		return *cached
	}
	config := SemanticBoardMatchConfig{
		SimThreshold:           0.8,
		DirectHitRate:          0.5,
		DirectMaxSim:           0.8,
		DirectMaxSimMinHits:    2,
		DirectMaxSimMinHitRate: 0.3,
		MinEffectiveSample:     3,
		HitRateSimBlend:        0.7,
		WeightSim:              0.6,
		WeightDensity:          0.4,
		WeightedThreshold:      0.6,
		MaxBoards:              3,
		DirectHitMinOverlap:    2,
		DirectionSimThreshold:  0.5,
		DirectHitScoreFactor:   0.7,
	}

	var settings []models.AISettings
	if err := s.db.WithContext(ctx).Where("key IN ?", []string{
		"semantic_board_match_sim_threshold",
		"semantic_board_match_direct_hit_rate",
		"semantic_board_match_direct_max_sim",
		"semantic_board_match_direct_max_sim_min_hits",
		"semantic_board_match_direct_max_sim_min_hit_rate",
		"semantic_board_match_min_effective_sample",
		"semantic_board_match_hit_rate_sim_blend",
		"semantic_board_match_weight_sim",
		"semantic_board_match_weight_density",
		"semantic_board_match_weighted_threshold",
		"semantic_board_match_max_boards",
		"semantic_board_match_direct_hit_min_overlap",
		"semantic_board_match_direction_sim_threshold",
		"semantic_board_match_direct_hit_score_factor",
	}).Find(&settings).Error; err != nil {
		return config
	}
	for _, setting := range settings {
		switch setting.Key {
		case "semantic_board_match_sim_threshold":
			config.SimThreshold = parseSemanticBoardMatchFloat(setting.Value, config.SimThreshold)
		case "semantic_board_match_direct_hit_rate":
			config.DirectHitRate = parseSemanticBoardMatchFloat(setting.Value, config.DirectHitRate)
		case "semantic_board_match_direct_max_sim":
			config.DirectMaxSim = parseSemanticBoardMatchFloat(setting.Value, config.DirectMaxSim)
		case "semantic_board_match_direct_max_sim_min_hits":
			config.DirectMaxSimMinHits = parseSemanticBoardMatchInt(setting.Value, config.DirectMaxSimMinHits)
		case "semantic_board_match_direct_max_sim_min_hit_rate":
			config.DirectMaxSimMinHitRate = parseSemanticBoardMatchFloat(setting.Value, config.DirectMaxSimMinHitRate)
		case "semantic_board_match_min_effective_sample":
			config.MinEffectiveSample = parseSemanticBoardMatchInt(setting.Value, config.MinEffectiveSample)
		case "semantic_board_match_hit_rate_sim_blend":
			config.HitRateSimBlend = parseSemanticBoardMatchFloat(setting.Value, config.HitRateSimBlend)
		case "semantic_board_match_weight_sim":
			config.WeightSim = parseSemanticBoardMatchFloat(setting.Value, config.WeightSim)
		case "semantic_board_match_weight_density":
			config.WeightDensity = parseSemanticBoardMatchFloat(setting.Value, config.WeightDensity)
		case "semantic_board_match_weighted_threshold":
			config.WeightedThreshold = parseSemanticBoardMatchFloat(setting.Value, config.WeightedThreshold)
		case "semantic_board_match_max_boards":
			config.MaxBoards = parseSemanticBoardMatchInt(setting.Value, config.MaxBoards)
		case "semantic_board_match_direct_hit_min_overlap":
			config.DirectHitMinOverlap = parseSemanticBoardMatchInt(setting.Value, config.DirectHitMinOverlap)
		case "semantic_board_match_direction_sim_threshold":
			config.DirectionSimThreshold = parseSemanticBoardMatchFloat(setting.Value, config.DirectionSimThreshold)
		case "semantic_board_match_direct_hit_score_factor":
			config.DirectHitScoreFactor = parseSemanticBoardMatchFloat(setting.Value, config.DirectHitScoreFactor)
		}
	}
	s.cache.SetConfig(config)
	return config
}

func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func parseSemanticBoardMatchFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return fallback
	}
	return parsed
}

func parseSemanticBoardMatchInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
