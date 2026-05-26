package tagging

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

type SemanticBoardMatchingService struct {
	db *gorm.DB
}

func NewSemanticBoardMatchingService(db *gorm.DB) *SemanticBoardMatchingService {
	if db == nil {
		db = database.DB
	}
	return &SemanticBoardMatchingService{db: db}
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
}

type SemanticBoardMatchResult struct {
	SemanticBoardID uint
	Score           float64
	MatchReason     string
}

type boardAuxiliaryLabel struct {
	BoardID          uint
	AuxiliaryLabelID uint
	Label            string
	Embedding        *string
}

func (s *SemanticBoardMatchingService) MatchTopicTag(ctx context.Context, topicTagID uint) ([]SemanticBoardMatchResult, error) {
	if topicTagID == 0 {
		return nil, fmt.Errorf("topic tag id is required")
	}

	config := s.loadConfig(ctx)
	tagAuxiliaries, err := s.loadTagAuxiliaries(ctx, topicTagID)
	if err != nil {
		return nil, err
	}
	if len(tagAuxiliaries) == 0 {
		return []SemanticBoardMatchResult{}, s.replaceTopicTagBoardLabels(ctx, topicTagID, nil)
	}

	boardAuxiliaries, err := s.loadBoardAuxiliaries(ctx)
	if err != nil {
		return nil, err
	}
	if len(boardAuxiliaries) == 0 {
		return []SemanticBoardMatchResult{}, s.replaceTopicTagBoardLabels(ctx, topicTagID, nil)
	}

	matches := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
	if len(matches) > config.MaxBoards {
		matches = matches[:config.MaxBoards]
	}
	if err := s.replaceTopicTagBoardLabels(ctx, topicTagID, matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (s *SemanticBoardMatchingService) loadTagAuxiliaries(ctx context.Context, topicTagID uint) ([]models.SemanticLabel, error) {
	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.semantic_label_id = semantic_labels.id").
		Where("topic_tag_semantic_labels.topic_tag_id = ? AND semantic_labels.label_type = ? AND semantic_labels.status = ?", topicTagID, "auxiliary", "active").
		Find(&labels).Error
	return labels, err
}

func (s *SemanticBoardMatchingService) loadBoardAuxiliaries(ctx context.Context) ([]boardAuxiliaryLabel, error) {
	var labels []boardAuxiliaryLabel
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
		Scan(&labels).Error
	return labels, err
}

func (s *SemanticBoardMatchingService) loadBoardAuxiliariesByBoardID(ctx context.Context, boardID uint) ([]boardAuxiliaryLabel, error) {
	var labels []boardAuxiliaryLabel
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
		Where("board_composition.board_id = ?", boardID).
		Scan(&labels).Error
	return labels, err
}

func evaluateSemanticBoardMatches(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []boardAuxiliaryLabel, config SemanticBoardMatchConfig) []SemanticBoardMatchResult {
	tagAuxiliaryIDs := make(map[uint]struct{}, len(tagAuxiliaries))
	tagVectors := make([][]float64, 0, len(tagAuxiliaries))
	for _, label := range tagAuxiliaries {
		tagAuxiliaryIDs[label.ID] = struct{}{}
		if label.Embedding == nil {
			continue
		}
		vector, err := parsePgVector(*label.Embedding)
		if err == nil {
			tagVectors = append(tagVectors, vector)
		}
	}

	grouped := make(map[uint][]boardAuxiliaryLabel)
	for _, auxiliary := range boardAuxiliaries {
		grouped[auxiliary.BoardID] = append(grouped[auxiliary.BoardID], auxiliary)
	}

	matches := make([]SemanticBoardMatchResult, 0, len(grouped))
	for boardID, auxiliaries := range grouped {
		overlapCount := countDirectSemanticBoardHits(tagAuxiliaryIDs, auxiliaries)
		if overlapCount >= config.DirectHitMinOverlap {
			matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: 1.0, MatchReason: "direct_hit"})
			continue
		}

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
		switch {
		case hitRate > config.DirectHitRate:
			score = config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate
			matchReason = "hit_rate"
		case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
			score = maxSimilarity
			matchReason = "max_sim"
		case weighted >= config.WeightedThreshold:
			score = weighted
			matchReason = "weighted"
		}
		if matchReason != "" {
			matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: score, MatchReason: matchReason})
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

type matchDetailPair struct {
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
	Pairs         []matchDetailPair
}

func computeMatchDetail(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []boardAuxiliaryLabel, config SemanticBoardMatchConfig) computedMatchDetail {
	detail := computedMatchDetail{Pairs: []matchDetailPair{}}
	if len(tagAuxiliaries) == 0 || len(boardAuxiliaries) == 0 {
		return detail
	}

	boardVectors := make([]struct {
		label  boardAuxiliaryLabel
		vector []float64
	}, 0, len(boardAuxiliaries))
	for _, boardAuxiliary := range boardAuxiliaries {
		if boardAuxiliary.Embedding == nil {
			continue
		}
		vector, err := parsePgVector(*boardAuxiliary.Embedding)
		if err == nil {
			boardVectors = append(boardVectors, struct {
				label  boardAuxiliaryLabel
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
		tagVector, err := parsePgVector(*tagAuxiliary.Embedding)
		if err != nil {
			continue
		}

		bestSimilarity := -1.0
		var bestBoard boardAuxiliaryLabel
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
		detail.Pairs = append(detail.Pairs, matchDetailPair{
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

func countDirectSemanticBoardHits(tagAuxiliaryIDs map[uint]struct{}, boardAuxiliaries []boardAuxiliaryLabel) int {
	count := 0
	for _, auxiliary := range boardAuxiliaries {
		if _, ok := tagAuxiliaryIDs[auxiliary.AuxiliaryLabelID]; ok {
			count++
		}
	}
	return count
}

func parseBoardAuxiliaryVectors(auxiliaries []boardAuxiliaryLabel) [][]float64 {
	vectors := make([][]float64, 0, len(auxiliaries))
	for _, auxiliary := range auxiliaries {
		if auxiliary.Embedding == nil {
			continue
		}
		vector, err := parsePgVector(*auxiliary.Embedding)
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
			row := models.TopicTagBoardLabel{TopicTagID: topicTagID, SemanticBoardID: match.SemanticBoardID, Score: match.Score, MatchReason: match.MatchReason}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SemanticBoardMatchingService) loadConfig(ctx context.Context) SemanticBoardMatchConfig {
	config := SemanticBoardMatchConfig{
		SimThreshold:           0.72,
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
		}
	}
	return config
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
