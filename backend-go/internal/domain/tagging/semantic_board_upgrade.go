package tagging

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

type SemanticBoardUpgradeService struct {
	db  *gorm.DB
	llm semanticBoardUpgradeLLM
}

type semanticBoardUpgradeLLM interface {
	SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error)
}

type SemanticBoardUpgradeConfig struct {
	RefCountThreshold        int
	ClusterDistanceThreshold float64
	CoTagWindowDays          int
	CoTagTopN                int
	CoTagDedupeSimThreshold  float64
	CoTagHardLimit           int
}

type SemanticBoardUpgradeCandidate struct {
	ID        uint
	Label     string
	Slug      string
	RefCount  int
	Embedding []float64
}

type SemanticBoardUpgradeCluster struct {
	Candidates                   []SemanticBoardUpgradeCandidate
	Centroid                     []float64
	ExistingBoardID              *uint
	ExistingBoardLabel           string
	ExistingBoardDescription     string
	ExistingBoardAuxiliaryLabels []string
	Events                       []SemanticBoardUpgradeEventContext
}

type SemanticBoardUpgradeEventContext struct {
	TopicTagID uint
	Label      string
	Frequency  int
}

type SemanticBoardUpgradeSuggestion struct {
	Decision          SemanticBoardUpgradeDecision
	BoardLabel        string
	Description       string
	AuxiliaryLabelIDs []uint
	TargetBoardID     *uint
	Reason            string
}

type SemanticBoardUpgradeDecision string

const (
	SemanticBoardUpgradeDecisionCreateNew         SemanticBoardUpgradeDecision = "create_new"
	SemanticBoardUpgradeDecisionMergeIntoExisting SemanticBoardUpgradeDecision = "merge_into_existing"
	SemanticBoardUpgradeDecisionSkip              SemanticBoardUpgradeDecision = "skip"
)

type ConfirmSemanticBoardUpgradeRequest struct {
	Decision          SemanticBoardUpgradeDecision
	BoardLabel        string
	Description       string
	AuxiliaryLabelIDs []uint
	TargetBoardID     *uint
}

type ConfirmSemanticBoardUpgradeResult struct {
	SemanticBoardID   uint
	AuxiliaryLabelIDs []uint
}

type semanticBoardContext struct {
	BoardID          uint
	BoardLabel       string
	BoardDescription string
	AuxiliaryLabelID uint
	AuxiliaryLabel   string
	Embedding        []float64
}

func NewSemanticBoardUpgradeService(db *gorm.DB, llm semanticBoardUpgradeLLM) *SemanticBoardUpgradeService {
	if db == nil {
		db = database.DB
	}
	return &SemanticBoardUpgradeService{db: db, llm: llm}
}

func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context) ([]SemanticBoardUpgradeSuggestion, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("semantic board upgrade llm is required")
	}
	config := s.LoadUpgradeConfig(ctx)
	candidates, err := s.collectCandidates(ctx, config)
	if err != nil {
		return nil, err
	}
	if len(candidates) < config.RefCountThreshold {
		return []SemanticBoardUpgradeSuggestion{}, nil
	}
	clusters, err := s.clusterCandidates(ctx, candidates, config)
	if err != nil {
		return nil, err
	}
	for i := range clusters {
		clusters[i].Events, err = s.loadCoTagEventContext(ctx, clusters[i], config)
		if err != nil {
			return nil, err
		}
	}

	suggestions, err := s.llm.SuggestSemanticBoardUpgrades(ctx, buildSemanticBoardUpgradePrompt(clusters))
	if err != nil {
		return nil, err
	}
	validAuxiliaryIDs := map[uint]struct{}{}
	for _, candidate := range candidates {
		validAuxiliaryIDs[candidate.ID] = struct{}{}
	}
	return filterSemanticBoardUpgradeSuggestions(suggestions, validAuxiliaryIDs), nil
}

func (s *SemanticBoardUpgradeService) ConfirmSuggestion(ctx context.Context, req ConfirmSemanticBoardUpgradeRequest) (*ConfirmSemanticBoardUpgradeResult, error) {
	auxiliaryIDs := uniqueUintSlice(req.AuxiliaryLabelIDs)
	if len(auxiliaryIDs) == 0 {
		return nil, fmt.Errorf("auxiliary label ids are required")
	}

	var result ConfirmSemanticBoardUpgradeResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateActiveAuxiliaryLabels(tx, auxiliaryIDs); err != nil {
			return err
		}

		var boardID uint
		switch req.Decision {
		case SemanticBoardUpgradeDecisionCreateNew:
			label := strings.TrimSpace(req.BoardLabel)
			if label == "" {
				return fmt.Errorf("board label is required")
			}
			board := models.SemanticLabel{
				Label:       label,
				Slug:        uniqueSemanticLabelSlug(tx, Slugify(label)),
				LabelType:   "board",
				Description: req.Description,
				Source:      "llm_suggest",
				Status:      "active",
			}
			if err := tx.Create(&board).Error; err != nil {
				return err
			}
			boardID = board.ID
		case SemanticBoardUpgradeDecisionMergeIntoExisting:
			if req.TargetBoardID == nil || *req.TargetBoardID == 0 {
				return fmt.Errorf("target board id is required")
			}
			var count int64
			if err := tx.Model(&models.SemanticLabel{}).Where("id = ? AND label_type = ? AND status = ?", *req.TargetBoardID, "board", "active").Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("active target board not found")
			}
			boardID = *req.TargetBoardID
		default:
			return fmt.Errorf("unsupported decision: %s", req.Decision)
		}

		rows := make([]models.BoardComposition, 0, len(auxiliaryIDs))
		for _, auxiliaryID := range auxiliaryIDs {
			rows = append(rows, models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: auxiliaryID})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		result = ConfirmSemanticBoardUpgradeResult{SemanticBoardID: boardID, AuxiliaryLabelIDs: auxiliaryIDs}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *SemanticBoardUpgradeService) collectCandidates(ctx context.Context, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCandidate, error) {
	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND ref_count >= ? AND embedding IS NOT NULL", "auxiliary", "active", config.RefCountThreshold).
		Where("NOT EXISTS (SELECT 1 FROM board_composition WHERE board_composition.auxiliary_label_id = semantic_labels.id)").
		Order("id ASC").
		Find(&labels).Error
	if err != nil {
		return nil, err
	}

	candidates := make([]SemanticBoardUpgradeCandidate, 0, len(labels))
	for _, label := range labels {
		vector, err := parsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		candidates = append(candidates, SemanticBoardUpgradeCandidate{ID: label.ID, Label: label.Label, Slug: label.Slug, RefCount: label.RefCount, Embedding: vector})
	}
	return candidates, nil
}

func (s *SemanticBoardUpgradeService) clusterCandidates(ctx context.Context, candidates []SemanticBoardUpgradeCandidate, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCluster, error) {
	boardContexts, err := s.loadExistingBoardContexts(ctx)
	if err != nil {
		return nil, err
	}
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	boardClusterByID := map[uint]int{}
	boardDetails := semanticBoardDetailsByID(boardContexts)

	for _, candidate := range candidates {
		if boardContext, ok := closestBoardContext(candidate, boardContexts, config.ClusterDistanceThreshold); ok {
			clusterIndex, exists := boardClusterByID[boardContext.BoardID]
			if !exists {
				boardID := boardContext.BoardID
				details := boardDetails[boardContext.BoardID]
				clusters = append(clusters, SemanticBoardUpgradeCluster{
					ExistingBoardID:              &boardID,
					ExistingBoardLabel:           boardContext.BoardLabel,
					ExistingBoardDescription:     boardContext.BoardDescription,
					ExistingBoardAuxiliaryLabels: details,
				})
				clusterIndex = len(clusters) - 1
				boardClusterByID[boardContext.BoardID] = clusterIndex
			}
			clusters[clusterIndex].Candidates = append(clusters[clusterIndex].Candidates, candidate)
			continue
		}

		matched := false
		for i := range clusters {
			if clusters[i].ExistingBoardID != nil {
				continue
			}
			if candidateFitsCluster(candidate, &clusters[i], config.ClusterDistanceThreshold) {
				addCandidateToCluster(candidate, &clusters[i])
				matched = true
				break
			}
		}
		if !matched {
			clusters = append(clusters, SemanticBoardUpgradeCluster{
				Candidates: []SemanticBoardUpgradeCandidate{candidate},
				Centroid:   candidate.Embedding,
			})
		}
	}
	return clusters, nil
}

func (s *SemanticBoardUpgradeService) loadExistingBoardContexts(ctx context.Context) ([]semanticBoardContext, error) {
	var rows []struct {
		BoardID          uint
		BoardLabel       string
		BoardDescription string
		AuxiliaryLabelID uint
		AuxiliaryLabel   string
		Embedding        *string
	}
	err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("board_composition.board_id, board.label AS board_label, board.description AS board_description, board_composition.auxiliary_label_id, auxiliary.label AS auxiliary_label, auxiliary.embedding").
		Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
		Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ? AND auxiliary.embedding IS NOT NULL", "auxiliary", "active").
		Order("board_composition.board_id ASC, board_composition.auxiliary_label_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	contexts := make([]semanticBoardContext, 0, len(rows))
	for _, row := range rows {
		vector, err := parsePgVector(*row.Embedding)
		if err != nil {
			continue
		}
		contexts = append(contexts, semanticBoardContext{BoardID: row.BoardID, BoardLabel: row.BoardLabel, BoardDescription: row.BoardDescription, AuxiliaryLabelID: row.AuxiliaryLabelID, AuxiliaryLabel: row.AuxiliaryLabel, Embedding: vector})
	}
	return contexts, nil
}

func (s *SemanticBoardUpgradeService) loadCoTagEventContext(ctx context.Context, cluster SemanticBoardUpgradeCluster, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeEventContext, error) {
	auxiliaryIDs := make([]uint, 0, len(cluster.Candidates))
	for _, candidate := range cluster.Candidates {
		auxiliaryIDs = append(auxiliaryIDs, candidate.ID)
	}
	if len(auxiliaryIDs) == 0 {
		return []SemanticBoardUpgradeEventContext{}, nil
	}

	var seedTopicIDs []uint
	if err := s.db.WithContext(ctx).Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id IN ?", auxiliaryIDs).Distinct().Pluck("topic_tag_id", &seedTopicIDs).Error; err != nil {
		return nil, err
	}
	if len(seedTopicIDs) == 0 {
		return []SemanticBoardUpgradeEventContext{}, nil
	}

	cutoff := time.Now().AddDate(0, 0, -config.CoTagWindowDays)
	topN := config.CoTagTopN
	if topN <= 0 {
		topN = 20
	}
	var rows []struct {
		TopicTagID uint
		Label      string
		Embedding  *string
		Frequency  int
	}
	err := s.db.WithContext(ctx).
		Table("article_topic_tags AS event_att").
		Select("event_tag.id AS topic_tag_id, event_tag.label, event_embedding.embedding AS embedding, COUNT(*) AS frequency").
		Joins("JOIN topic_tags AS event_tag ON event_tag.id = event_att.topic_tag_id AND event_tag.category = ? AND event_tag.status = ?", models.TagCategoryEvent, "active").
		Joins("JOIN articles ON articles.id = event_att.article_id AND articles.created_at >= ?", cutoff).
		Joins("JOIN article_topic_tags AS seed_att ON seed_att.article_id = event_att.article_id AND seed_att.topic_tag_id IN ?", seedTopicIDs).
		Joins("LEFT JOIN topic_tag_embeddings AS event_embedding ON event_embedding.topic_tag_id = event_tag.id AND event_embedding.embedding_type = ?", "semantic").
		Where("event_tag.id NOT IN ?", seedTopicIDs).
		Group("event_tag.id, event_tag.label, event_embedding.embedding").
		Order("frequency DESC, event_tag.id ASC").
		Limit(topN).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	contexts := make([]SemanticBoardUpgradeEventContext, 0, len(rows))
	keptVectors := [][]float64{}
	seenLabels := map[string]struct{}{}
	for _, row := range rows {
		labelKey := strings.ToLower(strings.TrimSpace(row.Label))
		if _, exists := seenLabels[labelKey]; exists {
			continue
		}
		if row.Embedding != nil {
			vector, err := parsePgVector(*row.Embedding)
			if err == nil && isNearKeptVector(vector, keptVectors, config.CoTagDedupeSimThreshold) {
				continue
			}
			if err == nil {
				keptVectors = append(keptVectors, vector)
			}
		}
		seenLabels[labelKey] = struct{}{}
		contexts = append(contexts, SemanticBoardUpgradeEventContext{TopicTagID: row.TopicTagID, Label: row.Label, Frequency: row.Frequency})
		if config.CoTagHardLimit > 0 && len(contexts) >= config.CoTagHardLimit {
			break
		}
	}
	return contexts, nil
}

func (s *SemanticBoardUpgradeService) LoadUpgradeConfig(ctx context.Context) SemanticBoardUpgradeConfig {
	config := SemanticBoardUpgradeConfig{
		RefCountThreshold:        5,
		ClusterDistanceThreshold: 0.35,
		CoTagWindowDays:          30,
		CoTagTopN:                20,
		CoTagDedupeSimThreshold:  0.85,
		CoTagHardLimit:           15,
	}
	var settings []models.AISettings
	if err := s.db.WithContext(ctx).Where("key IN ?", []string{
		"semantic_board_upgrade_ref_count_threshold",
		"semantic_board_upgrade_cluster_distance_threshold",
		"semantic_board_upgrade_cotag_window_days",
		"semantic_board_upgrade_cotag_top_n",
		"semantic_board_upgrade_cotag_dedupe_sim_threshold",
		"semantic_board_upgrade_cotag_hard_limit",
	}).Find(&settings).Error; err != nil {
		return config
	}
	for _, setting := range settings {
		switch setting.Key {
		case "semantic_board_upgrade_ref_count_threshold":
			config.RefCountThreshold = parseSemanticBoardUpgradeInt(setting.Value, config.RefCountThreshold)
		case "semantic_board_upgrade_cluster_distance_threshold":
			config.ClusterDistanceThreshold = parseSemanticBoardUpgradeFloat(setting.Value, config.ClusterDistanceThreshold)
		case "semantic_board_upgrade_cotag_window_days":
			config.CoTagWindowDays = parseSemanticBoardUpgradeInt(setting.Value, config.CoTagWindowDays)
		case "semantic_board_upgrade_cotag_top_n":
			config.CoTagTopN = parseSemanticBoardUpgradeInt(setting.Value, config.CoTagTopN)
		case "semantic_board_upgrade_cotag_dedupe_sim_threshold":
			config.CoTagDedupeSimThreshold = parseSemanticBoardUpgradeFloat(setting.Value, config.CoTagDedupeSimThreshold)
		case "semantic_board_upgrade_cotag_hard_limit":
			config.CoTagHardLimit = parseSemanticBoardUpgradeInt(setting.Value, config.CoTagHardLimit)
		}
	}
	return config
}

func closestBoardContext(candidate SemanticBoardUpgradeCandidate, contexts []semanticBoardContext, threshold float64) (semanticBoardContext, bool) {
	for _, context := range contexts {
		if semanticBoardUpgradeDistance(candidate.Embedding, context.Embedding) <= threshold {
			return context, true
		}
	}
	return semanticBoardContext{}, false
}

func candidateFitsCluster(candidate SemanticBoardUpgradeCandidate, cluster *SemanticBoardUpgradeCluster, threshold float64) bool {
	if len(cluster.Centroid) == 0 {
		return false
	}
	return semanticBoardUpgradeDistance(candidate.Embedding, cluster.Centroid) <= threshold
}

func addCandidateToCluster(candidate SemanticBoardUpgradeCandidate, cluster *SemanticBoardUpgradeCluster) {
	cluster.Candidates = append(cluster.Candidates, candidate)
	cluster.Centroid = updateCentroid(cluster.Centroid, candidate.Embedding, len(cluster.Candidates)-1)
}

func updateCentroid(current []float64, newVec []float64, currentCount int) []float64 {
	if len(current) != len(newVec) {
		return current
	}
	// weighted average: (current * n + new) / (n + 1)
	next := make([]float64, len(current))
	n := float64(currentCount)
	for i := range current {
		next[i] = (current[i]*n + newVec[i]) / (n + 1)
	}
	return next
}

func semanticBoardUpgradeDistance(a []float64, b []float64) float64 {
	similarity, err := airouter.CosineSimilarity(a, b)
	if err != nil {
		return 1
	}
	return 1 - similarity
}

func isNearKeptVector(vector []float64, keptVectors [][]float64, threshold float64) bool {
	for _, kept := range keptVectors {
		similarity, err := airouter.CosineSimilarity(vector, kept)
		if err == nil && similarity > threshold {
			return true
		}
	}
	return false
}

func semanticBoardDetailsByID(contexts []semanticBoardContext) map[uint][]string {
	details := map[uint][]string{}
	seen := map[uint]map[string]struct{}{}
	for _, context := range contexts {
		if _, ok := seen[context.BoardID]; !ok {
			seen[context.BoardID] = map[string]struct{}{}
		}
		label := strings.TrimSpace(context.AuxiliaryLabel)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, exists := seen[context.BoardID][key]; exists {
			continue
		}
		seen[context.BoardID][key] = struct{}{}
		details[context.BoardID] = append(details[context.BoardID], label)
	}
	return details
}

func buildSemanticBoardUpgradePrompt(clusters []SemanticBoardUpgradeCluster) string {
	var builder strings.Builder
	builder.WriteString("你是一个语义板块分析助手。根据以下辅助标签聚类信息，判断每个簇应该：create_new（创建新板块）、merge_into_existing（合并到已有板块）、skip（跳过不处理）。\n\n")
	builder.WriteString("判断原则：\n")
	builder.WriteString("- 如果簇内标签语义集中、有明确主题且不存在对应板块 → create_new\n")
	builder.WriteString("- 如果簇内标签与某个已有板块高度相关 → merge_into_existing\n")
	builder.WriteString("- 如果簇内标签过于分散或过于泛化，不足以形成独立板块 → skip\n\n")
	builder.WriteString("返回 JSON 格式：{\"suggestions\": [{\"decision\": \"create_new|merge_into_existing|skip\", \"board_label\": \"板块名称\", \"description\": \"板块描述\", \"auxiliary_label_ids\": [id1, id2], \"target_board_id\": null或已有板块ID, \"reason\": \"判断理由\"}]}\n\n")
	for i, cluster := range clusters {
		fmt.Fprintf(&builder, "【簇 %d】\n", i+1)
		if cluster.ExistingBoardID != nil {
			fmt.Fprintf(&builder, "关联已有板块：%s（ID=%d）\n", cluster.ExistingBoardLabel, *cluster.ExistingBoardID)
			if strings.TrimSpace(cluster.ExistingBoardDescription) != "" {
				fmt.Fprintf(&builder, "板块描述：%s\n", cluster.ExistingBoardDescription)
			}
			if len(cluster.ExistingBoardAuxiliaryLabels) > 0 {
				builder.WriteString("板块现有构成标签：\n")
				for _, label := range cluster.ExistingBoardAuxiliaryLabels {
					fmt.Fprintf(&builder, "  - %s\n", label)
				}
			}
		}
		builder.WriteString("候选辅助标签：\n")
		for _, candidate := range cluster.Candidates {
			fmt.Fprintf(&builder, "  - ID=%d: %s（引用次数=%d）\n", candidate.ID, candidate.Label, candidate.RefCount)
		}
		if len(cluster.Events) > 0 {
			builder.WriteString("关联事件（近期共现）：\n")
			for _, event := range cluster.Events {
				fmt.Fprintf(&builder, "  - %s（共现次数=%d）\n", event.Label, event.Frequency)
			}
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func filterSemanticBoardUpgradeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, validAuxiliaryIDs map[uint]struct{}) []SemanticBoardUpgradeSuggestion {
	filtered := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Decision != SemanticBoardUpgradeDecisionCreateNew && suggestion.Decision != SemanticBoardUpgradeDecisionMergeIntoExisting && suggestion.Decision != SemanticBoardUpgradeDecisionSkip {
			continue
		}
		suggestion.AuxiliaryLabelIDs = filterKnownAuxiliaryIDs(uniqueUintSlice(suggestion.AuxiliaryLabelIDs), validAuxiliaryIDs)
		if suggestion.Decision != SemanticBoardUpgradeDecisionSkip && len(suggestion.AuxiliaryLabelIDs) == 0 {
			continue
		}
		filtered = append(filtered, suggestion)
	}
	return filtered
}

func filterKnownAuxiliaryIDs(ids []uint, valid map[uint]struct{}) []uint {
	filtered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := valid[id]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func validateActiveAuxiliaryLabels(tx *gorm.DB, ids []uint) error {
	var count int64
	if err := tx.Model(&models.SemanticLabel{}).Where("id IN ? AND label_type = ? AND status = ?", ids, "auxiliary", "active").Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("all auxiliary labels must be active auxiliary labels")
	}
	return nil
}

func uniqueUintSlice(ids []uint) []uint {
	seen := map[uint]struct{}{}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func parseSemanticBoardUpgradeFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return fallback
	}
	return parsed
}

func parseSemanticBoardUpgradeInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
