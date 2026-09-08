package board

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/core"
)

type SemanticBoardUpgradeService struct {
	db             *gorm.DB
	llm            SemanticBoardUpgradeLLM
	embedder       auxlabel.AuxiliaryLabelEmbedder
	suggestionRepo *repository.BoardUpgradeSuggestionRepository
}

type SemanticBoardUpgradeLLM interface {
	SuggestSemanticBoardUpgrades(ctx context.Context, prompt string, mode string) ([]SemanticBoardUpgradeSuggestion, error)
}

type SemanticBoardUpgradeConfig struct {
	RefCountThreshold        int
	ClusterDistanceThreshold float64
	CoTagWindowDays          int
	CoTagTopN                int
	CoTagDedupeSimThreshold  float64
	CoTagHardLimit           int
	ClusterMethod            string
	// CompositeCoTagMinCooccurrence is the minimum number of co-occurring
	// articles (same-article aux pair/triple) for a compose candidate
	// (ai_settings semantic_board_upgrade_composite_min_cooccurrence, default 10).
	CompositeCoTagMinCooccurrence int
	// ExpandSimDistance is the cosine-distance threshold for the expand
	// direction's similarity recall path (design D3, ai_settings
	// semantic_board_expand_sim_distance, default 0.35).
	ExpandSimDistance float64
	// ExpandCooccurrence is the minimum same-article co-occurrence count with
	// the target board composition for the expand recall co-occurrence path
	// (design D3, ai_settings semantic_board_expand_cooccurrence, default 3).
	ExpandCooccurrence int
}

// 升级建议生成请求：方向 × 来源四格矩阵（design D1）。
// Direction ∈ {create, expand}；Source ∈ {aux, composite}；
// TargetBoardID 在 expand 方向必填（生成前锁定单版块），create 方向必须为 0；
// Days 仅 create×aux 生效（候选时间窗，0 = 不过滤）。
type UpgradeGenerateRequest struct {
	Direction     string
	Source        string
	TargetBoardID uint
	Days          int
}

const (
	UpgradeDirectionCreate = "create"
	UpgradeDirectionExpand = "expand"
	UpgradeSourceAux       = "aux"
	UpgradeSourceComposite = "composite"
	// UpgradeBoardListLimit caps the active-board list injected into the
	// create×aux prompt (token guard, design D2).
	UpgradeBoardListLimit = 60
)

// ModeKey 是 suggestion_hash 的 mode 维度值（design D5：direction:source）。
func (r UpgradeGenerateRequest) ModeKey() string {
	return r.Direction + ":" + r.Source
}

func (r UpgradeGenerateRequest) Validate() error {
	switch r.Direction {
	case UpgradeDirectionCreate, UpgradeDirectionExpand:
	default:
		return fmt.Errorf("invalid direction %q (expect create|expand)", r.Direction)
	}
	switch r.Source {
	case UpgradeSourceAux, UpgradeSourceComposite:
	default:
		return fmt.Errorf("invalid source %q (expect aux|composite)", r.Source)
	}
	if r.Direction == UpgradeDirectionExpand && r.TargetBoardID == 0 {
		return fmt.Errorf("target_board_id is required for expand direction")
	}
	if r.Direction == UpgradeDirectionCreate && r.TargetBoardID != 0 {
		return fmt.Errorf("target_board_id must not be set for create direction")
	}
	if r.Days < 0 {
		return fmt.Errorf("days must be >= 0")
	}
	return nil
}

type SemanticBoardUpgradeCandidate struct {
	ID        uint
	Label     string
	Slug      string
	RefCount  int
	Embedding []float64
}

type SemanticBoardUpgradeCluster struct {
	Candidates []SemanticBoardUpgradeCandidate
	Centroid   []float64
	Events     []SemanticBoardUpgradeEventContext
	origIdx    int // internal: tracks Pass 1 cluster index during reassignment
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
	// Confidence is "high" (dual-signature agreement bypassed the LLM) or "llm"
	// (LLM-adjudicated). Empty defaults to "llm" at persist time (§4.3).
	Confidence string
	// Evidence is the jsonb snapshot persisted with the suggestion
	// ({shortlist, margins, cotag_events, lane_briefs}) for audit/prompt grounding (§4.3/§4.4).
	Evidence map[string]any
}

type SemanticBoardUpgradeDecision string

const (
	SemanticBoardUpgradeDecisionCreateNew         SemanticBoardUpgradeDecision = "create_new"
	SemanticBoardUpgradeDecisionMergeIntoExisting SemanticBoardUpgradeDecision = "merge_into_existing"
	SemanticBoardUpgradeDecisionSkip              SemanticBoardUpgradeDecision = "skip"
	SemanticBoardUpgradeDecisionWatch             SemanticBoardUpgradeDecision = "watch"
)

type ConfirmSemanticBoardUpgradeRequest struct {
	Decision          SemanticBoardUpgradeDecision
	BoardLabel        string
	Description       string
	AuxiliaryLabelIDs []uint
	TargetBoardID     *uint
	// SuggestionID, when set, links this confirm to a pending suggestion that
	// is marked confirmed inside the same transaction (spec: confirm 联动).
	// Omitted (nil) for back-compat with callers that don't carry a suggestion.
	SuggestionID *uint
}

type ConfirmSemanticBoardUpgradeResult struct {
	SemanticBoardID   uint
	AuxiliaryLabelIDs []uint
	// CompositeLabelID is set for decision=compose confirms: the created (or
	// dedupe-reused) composite label id.
	CompositeLabelID *uint
}

func NewSemanticBoardUpgradeService(db *gorm.DB, llm SemanticBoardUpgradeLLM, embedder auxlabel.AuxiliaryLabelEmbedder) *SemanticBoardUpgradeService {
	if db == nil {
		db = repository.Repo.DB()
	}
	return &SemanticBoardUpgradeService{db: db, llm: llm, embedder: embedder, suggestionRepo: repository.NewBoardUpgradeSuggestionRepository(db)}
}

func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context, req UpgradeGenerateRequest) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
	if s.llm == nil {
		return nil, nil, fmt.Errorf("semantic board upgrade llm is required")
	}
	if err := req.Validate(); err != nil {
		return nil, nil, err
	}
	config := s.LoadUpgradeConfig(ctx)
	switch {
	case req.Direction == UpgradeDirectionCreate && req.Source == UpgradeSourceAux:
		return s.generateCreateAux(ctx, config, req)
	case req.Direction == UpgradeDirectionCreate && req.Source == UpgradeSourceComposite:
		suggestions, err := s.generateComposeSuggestions(ctx, config)
		if err != nil {
			return nil, nil, err
		}
		return suggestions, nil, nil
	case req.Direction == UpgradeDirectionExpand:
		return s.generateExpandSuggestions(ctx, config, req)
	}
	return nil, nil, fmt.Errorf("unsupported generation request")
}

// generateCreateAux 是创建×单标签管线（design D2 瘦身版）：收集未挂载候选 →
// 纯自聚类 → co-tag 事件上下文 → 全量活跃版块清单防重 → LLM 单决策空间 {create_new|skip}。
// 单例簇（size=1）不产建议（watch 观察池已退役，spec: 升级建议生成路径单一化）；
// 全部建议都经 LLM 裁决（高置信自动合成已退役，无合成旁路）；skip 不落库不返回。
func (s *SemanticBoardUpgradeService) generateCreateAux(ctx context.Context, config SemanticBoardUpgradeConfig, req UpgradeGenerateRequest) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
	candidates, err := s.CollectCandidates(ctx, config, req.Days)
	if err != nil {
		return nil, nil, err
	}
	if len(candidates) < config.RefCountThreshold {
		// 冷启动：候选不足不产 create 建议（组合候选走独立入口 source=composite，
		// 与旧单入口混跑时代的「冷启动自动补 compose 段」行为解耦）。
		return []SemanticBoardUpgradeSuggestion{}, []SemanticBoardUpgradeCluster{}, nil
	}
	clusters, err := s.ClusterCandidates(ctx, candidates, config)
	if err != nil {
		return nil, nil, err
	}
	for i := range clusters {
		clusters[i].Events, err = s.loadCoTagEventContext(ctx, clusters[i], config)
		if err != nil {
			return nil, nil, err
		}
	}

	validAuxiliaryIDs := map[uint]struct{}{}
	for _, candidate := range candidates {
		validAuxiliaryIDs[candidate.ID] = struct{}{}
	}
	boards, err := s.loadActiveBoards(ctx)
	if err != nil {
		return nil, nil, err
	}

	var llmClusters []SemanticBoardUpgradeCluster
	for i := range clusters {
		if len(clusters[i].Candidates) == 1 {
			continue // 单例簇不产建议：等待未来成簇后参与（观察池退役）
		}
		llmClusters = append(llmClusters, clusters[i])
	}
	var suggestions []SemanticBoardUpgradeSuggestion
	if len(llmClusters) > 0 {
		raw, err := s.llm.SuggestSemanticBoardUpgrades(ctx, buildCreateAuxPrompt(llmClusters, boards), "create:aux")
		if err != nil {
			return nil, nil, err
		}
		for _, sug := range filterSemanticBoardUpgradeSuggestions(raw, validAuxiliaryIDs) {
			if sug.Confidence == "" {
				sug.Confidence = "llm"
			}
			suggestions = append(suggestions, sug)
		}
	}
	return suggestions, clusters, nil
}

// activeBoardBrief is one active board for the create×aux prompt board list.
type activeBoardBrief struct {
	BoardID          uint
	BoardLabel       string
	BoardDescription string
	Embedding        []float64
}

// loadActiveBoards returns all active boards with embeddings. Injected into
// the create×aux prompt as the duplicate-guard board list（按簇质心相似度截
// top-60，design D2）。
func (s *SemanticBoardUpgradeService) loadActiveBoards(ctx context.Context) ([]activeBoardBrief, error) {
	var labels []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "board", "active").
		Order("id ASC").
		Find(&labels).Error; err != nil {
		return nil, err
	}
	boards := make([]activeBoardBrief, 0, len(labels))
	for _, label := range labels {
		vector, err := auxlabel.ParsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		boards = append(boards, activeBoardBrief{BoardID: label.ID, BoardLabel: label.Label, BoardDescription: label.Description, Embedding: vector})
	}
	return boards, nil
}

// generateComposeSuggestions runs the compose candidate pipeline: collect
// co-occurrence pairs/triples, adjudicate via LLM (mode "compose"), and return
// valid compose suggestions (skip decisions are dropped here — they are never
// persisted).
func (s *SemanticBoardUpgradeService) generateComposeSuggestions(ctx context.Context, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeSuggestion, error) {
	candidates, err := s.collectComposeCandidates(ctx, config)
	if err != nil {
		return nil, err
	}
	// 排除已存在组合（防重复建议：确认创建过的组合不再入候选）。
	candidates, err = s.filterExistingComposeCandidates(ctx, candidates, nil)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	componentIDs := make([]uint, 0)
	for _, candidate := range candidates {
		componentIDs = append(componentIDs, candidate.ComponentIDs...)
	}
	labels, err := s.loadComponentLabels(ctx, UniqueUintSlice(componentIDs))
	if err != nil {
		return nil, err
	}
	raw, err := s.llm.SuggestSemanticBoardUpgrades(ctx, buildComposeCandidatesPrompt(candidates, labels), "create:composite")
	if err != nil {
		return nil, err
	}
	componentUniverse := make(map[uint]struct{}, len(labels))
	for id := range labels {
		componentUniverse[id] = struct{}{}
	}
	filtered := filterComposeSuggestions(raw, componentUniverse)
	// Enrich evidence with the candidate's co-occurrence stats for the
	// suggestion card (spec: 共现证据——频次+窗口+代表事件标题).
	candidateByComponents := make(map[string]ComposeCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByComponents[composeComponentKey(candidate.ComponentIDs)] = candidate
	}
	for i := range filtered {
		if filtered[i].Confidence == "" {
			filtered[i].Confidence = "llm"
		}
		filtered[i].Evidence = map[string]any{
			"source":                        "compose",
			"compose_window_days":           config.CoTagWindowDays,
			"compose_representative_titles": representativeTitlesOf(candidateByComponents, filtered[i].AuxiliaryLabelIDs),
		}
		if candidate, ok := candidateByComponents[composeComponentKey(filtered[i].AuxiliaryLabelIDs)]; ok {
			filtered[i].Evidence["compose_cooccurrence"] = candidate.Cooccurrence
		}
	}
	return filtered, nil
}

func composeComponentKey(ids []uint) string {
	sorted := UniqueUintSlice(ids)
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

func representativeTitlesOf(candidates map[string]ComposeCandidate, ids []uint) []string {
	if candidate, ok := candidates[composeComponentKey(ids)]; ok {
		return candidate.RepresentativeTitles
	}
	return nil
}

func (s *SemanticBoardUpgradeService) ConfirmSuggestion(ctx context.Context, req ConfirmSemanticBoardUpgradeRequest) (*ConfirmSemanticBoardUpgradeResult, error) {
	auxiliaryIDs := UniqueUintSlice(req.AuxiliaryLabelIDs)
	if len(auxiliaryIDs) == 0 {
		return nil, fmt.Errorf("auxiliary label ids are required")
	}

	var result ConfirmSemanticBoardUpgradeResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 跳过已失效（status≠active）的辅助标签：建议生成后标签可能被禁用/合并，
		// 硬性全量校验会让陈旧建议整体 400。这里软过滤，只保留仍 active 的标签。
		activeIDs, err := FilterActiveAuxiliaryLabels(tx, auxiliaryIDs)
		if err != nil {
			return err
		}
		if len(activeIDs) == 0 {
			return fmt.Errorf("所有传入的辅助标签均已失效（status≠active），请重新选择有效的辅助标签后重试")
		}
		auxiliaryIDs = activeIDs

		var boardID uint
		switch req.Decision {
		case SemanticBoardUpgradeDecisionCreateNew:
			label := strings.TrimSpace(req.BoardLabel)
			if label == "" {
				return fmt.Errorf("board label is required")
			}
			board := models.SemanticLabel{
				Label:       label,
				Slug:        auxlabel.UniqueSemanticLabelSlug(tx, core.Slugify(label)),
				LabelType:   "board",
				Description: req.Description,
				Source:      "llm_suggest",
				Status:      "active",
			}
			if s.embedder != nil {
				input := label
				if desc := strings.TrimSpace(req.Description); desc != "" {
					input = label + ". " + desc
				}
				pgVector, _, embedErr := s.embedder(ctx, input, auxlabel.AuxiliaryLabelEmbeddingModeStorage)
				if embedErr != nil {
					return fmt.Errorf("generate board embedding: %w", embedErr)
				}
				board.Embedding = &pgVector
			}
			if err := tx.Create(&board).Error; err != nil {
				return err
			}
			boardID = board.ID
		case SemanticBoardUpgradeDecisionMergeIntoExisting:
			if req.TargetBoardID == nil || *req.TargetBoardID == 0 {
				// target_off_shortlist 方案 B 会保留 target_board_id=NULL 的 merge
				// 建议供用户裁决；执行层必须提示用户先选目标板块，否则前端一键确认会 400。
				return fmt.Errorf("合并到已有板块时必须指定目标板块，请先选择目标版块后再执行")
			}
			var count int64
			if err := tx.Model(&models.SemanticLabel{}).Where("id = ? AND label_type = ? AND status = ?", *req.TargetBoardID, "board", "active").Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("active target board not found")
			}
			boardID = *req.TargetBoardID
		case SemanticBoardUpgradeDecisionCompose:
			// 组件已由上方 FilterActiveAuxiliaryLabels 软过滤为 active。同一事务内
			// 创建组合标签（含 L1/L2 去重复用路径）；扩充方向（建议携带 target）
			// 同一事务内将组合标签挂载进目标版块的 board_composition（spec: compose
			// 建议确认执行——挂载失败整体回滚）；embedder 失败等任何错误回滚，
			// 建议保持 pending。
			if len(auxiliaryIDs) < auxlabel.CompositeMinComponents || len(auxiliaryIDs) > auxlabel.CompositeMaxComponents {
				return fmt.Errorf("组合建议需要 %d-%d 个组件，当前 %d 个", auxlabel.CompositeMinComponents, auxlabel.CompositeMaxComponents, len(auxiliaryIDs))
			}
			label := strings.TrimSpace(req.BoardLabel)
			if label == "" {
				return fmt.Errorf("composite label is required")
			}
			var composeTargetBoard uint
			if req.TargetBoardID != nil && *req.TargetBoardID != 0 {
				// 扩充方向：目标版块须仍为活跃 board（生成后可能被禁用，spec:
				// 目标版块被禁用后确认失败）。
				var count int64
				if err := tx.Model(&models.SemanticLabel{}).Where("id = ? AND label_type = ? AND status = ?", *req.TargetBoardID, "board", "active").Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return fmt.Errorf("active target board not found")
				}
				composeTargetBoard = *req.TargetBoardID
			}
			compositeService := auxlabel.NewCompositeLabelService(tx, s.embedder)
			createResult, createErr := compositeService.CreateCompositeLabel(ctx, label, req.Description, auxiliaryIDs, "upgrade_suggest")
			if createErr != nil {
				return createErr
			}
			result.CompositeLabelID = &createResult.Label.ID
			boardID = 0 // compose creates a composite label, not a board
			if composeTargetBoard != 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.BoardComposition{BoardID: composeTargetBoard, AuxiliaryLabelID: createResult.Label.ID}).Error; err != nil {
					return fmt.Errorf("mount composite to board %d: %w", composeTargetBoard, err)
				}
			}
		default:
			return fmt.Errorf("unsupported decision: %s", req.Decision)
		}

		if req.Decision != SemanticBoardUpgradeDecisionCompose {
			rows := make([]models.BoardComposition, 0, len(auxiliaryIDs))
			for _, auxiliaryID := range auxiliaryIDs {
				rows = append(rows, models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: auxiliaryID})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
				return err
			}
		}
		// Link the confirm to a pending suggestion inside the same transaction: a
		// board_composition write failure above already returned, and any error
		// here rolls both back (suggestion state unchanged on tx failure).
		if req.SuggestionID != nil {
			if err := s.suggestionRepo.MarkConfirmed(tx, *req.SuggestionID); err != nil {
				return err
			}
		}
		result = ConfirmSemanticBoardUpgradeResult{SemanticBoardID: boardID, AuxiliaryLabelIDs: auxiliaryIDs, CompositeLabelID: result.CompositeLabelID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	packageBoardCache.InvalidateBoardData()
	return &result, nil
}

// CollectCandidates 收集未挂载的 active 辅助标签候选（创建×单标签路入口）。
// days > 0 时按文章活动时间双重过滤（upgrade-candidate-time-window 契约：
// 仅收集最近 N 天文章中出现的候选）；days = 0 不过滤。
func (s *SemanticBoardUpgradeService) CollectCandidates(ctx context.Context, config SemanticBoardUpgradeConfig, days int) ([]SemanticBoardUpgradeCandidate, error) {
	var labels []models.SemanticLabel
	query := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND ref_count >= ? AND embedding IS NOT NULL", "auxiliary", "active", config.RefCountThreshold).
		Where("NOT EXISTS (SELECT 1 FROM board_composition WHERE board_composition.auxiliary_label_id = semantic_labels.id)")
	if days > 0 {
		cutoff := time.Now().AddDate(0, 0, -days)
		query = query.Where(
			`EXISTS (SELECT 1 FROM article_topic_tags att
				JOIN topic_tag_semantic_labels ttsl ON ttsl.topic_tag_id = att.topic_tag_id
				JOIN articles a ON a.id = att.article_id
				WHERE ttsl.semantic_label_id = semantic_labels.id AND a.created_at >= ?)`, cutoff)
	}
	if err := query.Order("id ASC").Find(&labels).Error; err != nil {
		return nil, err
	}

	candidates := make([]SemanticBoardUpgradeCandidate, 0, len(labels))
	for _, label := range labels {
		vector, err := auxlabel.ParsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		candidates = append(candidates, SemanticBoardUpgradeCandidate{ID: label.ID, Label: label.Label, Slug: label.Slug, RefCount: label.RefCount, Embedding: vector})
	}
	return candidates, nil
}

func (s *SemanticBoardUpgradeService) ClusterCandidates(ctx context.Context, candidates []SemanticBoardUpgradeCandidate, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCluster, error) {
	if config.ClusterMethod == "average_link" {
		return clusterAverageLink(candidates, config.ClusterDistanceThreshold), nil
	}
	return clusterCentroid(candidates, config.ClusterDistanceThreshold), nil
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
			vector, err := auxlabel.ParsePgVector(*row.Embedding)
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
		ClusterDistanceThreshold: 0.25,
		// 调研修调（2026-09-07 报障追踪，scripts/research/candidate_freshness_probe.py）：
		// 0.35 下人物类标签（如“默茨”）embedding 与国际政客桶均在阈值内，贪心
		// average-link 传递混簇把干净主题簇（[选择党,基民盟]）稀释成大杂烩，LLM
		// 全裁 skip；降至 0.25 后全部窗口下稳定产出干净小主题簇（代价：送 LLM
		// 簇量 -15%~30%，拆掉的多为本来就被全裁 skip 的桶簇）。
		CoTagWindowDays:               30,
		CoTagTopN:                     20,
		CoTagDedupeSimThreshold:       0.85,
		CoTagHardLimit:                15,
		ClusterMethod:                 "average_link",
		CompositeCoTagMinCooccurrence: 10,
		ExpandSimDistance:             0.35,
		ExpandCooccurrence:            3,
	}
	var settings []models.AISettings
	if err := s.db.WithContext(ctx).Where("key IN ?", []string{
		"semantic_board_upgrade_ref_count_threshold",
		"semantic_board_upgrade_cluster_distance_threshold",
		"semantic_board_upgrade_cotag_window_days",
		"semantic_board_upgrade_cotag_top_n",
		"semantic_board_upgrade_cotag_dedupe_sim_threshold",
		"semantic_board_upgrade_cotag_hard_limit",
		"semantic_board_upgrade_cluster_method",
		"semantic_board_upgrade_composite_min_cooccurrence",
		"semantic_board_expand_sim_distance",
		"semantic_board_expand_cooccurrence",
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
		case "semantic_board_upgrade_cluster_method":
			if v := strings.TrimSpace(setting.Value); v == "average_link" || v == "centroid" {
				config.ClusterMethod = v
			}
		case "semantic_board_upgrade_composite_min_cooccurrence":
			config.CompositeCoTagMinCooccurrence = parseSemanticBoardUpgradeInt(setting.Value, config.CompositeCoTagMinCooccurrence)
		case "semantic_board_expand_sim_distance":
			config.ExpandSimDistance = parseSemanticBoardUpgradeFloat(setting.Value, config.ExpandSimDistance)
		case "semantic_board_expand_cooccurrence":
			config.ExpandCooccurrence = parseSemanticBoardUpgradeInt(setting.Value, config.ExpandCooccurrence)
		}
	}
	return config
}

func clusterCentroid(candidates []SemanticBoardUpgradeCandidate, threshold float64) []SemanticBoardUpgradeCluster {
	// Pass 1: Greedy initial assignment (produces initial clusters with running-mean centroids)
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	for _, candidate := range candidates {
		matched := false
		for i := range clusters {
			if candidateFitsCluster(candidate, &clusters[i], threshold) {
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

	// Pass 2: Reassign candidates to nearest stable centroid to correct greedy drift.
	if len(clusters) > 1 {
		stableCentroids := make([][]float64, len(clusters))
		for i, cl := range clusters {
			stableCentroids[i] = computeStableCentroid(cl.Candidates)
		}

		newClusters := make([]SemanticBoardUpgradeCluster, 0, len(clusters))
		for _, candidate := range candidates {
			bestIdx := -1
			bestDist := threshold + 1
			for i := range stableCentroids {
				if len(stableCentroids[i]) == 0 {
					continue
				}
				dist := semanticBoardUpgradeDistance(candidate.Embedding, stableCentroids[i])
				if dist <= threshold && dist < bestDist {
					bestDist = dist
					bestIdx = i
				}
			}
			if bestIdx >= 0 {
				found := false
				for j := range newClusters {
					if newClusters[j].origIdx == bestIdx {
						newClusters[j].Candidates = append(newClusters[j].Candidates, candidate)
						found = true
						break
					}
				}
				if !found {
					newClusters = append(newClusters, SemanticBoardUpgradeCluster{
						Candidates: []SemanticBoardUpgradeCandidate{candidate},
						origIdx:    bestIdx,
					})
				}
			} else {
				newClusters = append(newClusters, SemanticBoardUpgradeCluster{
					Candidates: []SemanticBoardUpgradeCandidate{candidate},
					origIdx:    -1,
				})
			}
		}

		for i := range newClusters {
			newClusters[i].Centroid = computeStableCentroid(newClusters[i].Candidates)
		}
		clusters = newClusters
	}
	return clusters
}

func clusterAverageLink(candidates []SemanticBoardUpgradeCandidate, threshold float64) []SemanticBoardUpgradeCluster {
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	for _, candidate := range candidates {
		bestIdx := -1
		bestAvgDist := threshold + 1
		for i := range clusters {
			fits, avgDist := candidateFitsClusterAverageLink(candidate, &clusters[i], threshold)
			if fits && avgDist < bestAvgDist {
				bestAvgDist = avgDist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			clusters[bestIdx].Candidates = append(clusters[bestIdx].Candidates, candidate)
		} else {
			clusters = append(clusters, SemanticBoardUpgradeCluster{
				Candidates: []SemanticBoardUpgradeCandidate{candidate},
			})
		}
	}
	// Compute centroids for each cluster (for display/DTO, not used in clustering)
	for i := range clusters {
		clusters[i].Centroid = computeStableCentroid(clusters[i].Candidates)
	}
	return clusters
}

func candidateFitsClusterAverageLink(candidate SemanticBoardUpgradeCandidate, cluster *SemanticBoardUpgradeCluster, threshold float64) (bool, float64) {
	if len(cluster.Candidates) == 0 {
		return false, 1
	}
	totalDist := 0.0
	hasConnected := false
	for _, member := range cluster.Candidates {
		dist := semanticBoardUpgradeDistance(candidate.Embedding, member.Embedding)
		totalDist += dist
		if dist <= threshold {
			hasConnected = true
		}
	}
	avgDist := totalDist / float64(len(cluster.Candidates))
	return hasConnected && avgDist <= threshold, avgDist
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

// computeStableCentroid computes the true mean of all candidate embeddings.
func computeStableCentroid(candidates []SemanticBoardUpgradeCandidate) []float64 {
	if len(candidates) == 0 {
		return nil
	}
	dim := len(candidates[0].Embedding)
	centroid := make([]float64, dim)
	for _, c := range candidates {
		for i, v := range c.Embedding {
			centroid[i] += v
		}
	}
	n := float64(len(candidates))
	for i := range centroid {
		centroid[i] /= n
	}
	return centroid
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

// BuildSemanticBoardUpgradeSystemPrompt returns the mode-scoped single-decision-
// space schema. Each generation round exposes exactly one decision pair to the
// LLM（design D1/D4：单一决策空间，LLM 输出不含目标字段——target 由服务端注入）：
// create:aux → create_new|skip；create:composite / expand:composite → compose|skip；
// expand:aux → merge_into_existing|skip。
func BuildSemanticBoardUpgradeSystemPrompt(mode string) string {
	switch mode {
	case "create:composite", "expand:composite":
		return `Return JSON only in this shape: {"suggestions":[{"decision":"compose|skip","board_label":"","description":"","auxiliary_label_ids":[1],"reason":""}]}`
	case "expand:aux":
		return `Return JSON only in this shape: {"suggestions":[{"decision":"merge_into_existing|skip","board_label":"","description":"","auxiliary_label_ids":[1],"reason":""}]}`
	default: // create:aux
		return `Return JSON only in this shape: {"suggestions":[{"decision":"create_new|skip","board_label":"","description":"","auxiliary_label_ids":[1],"reason":""}]}`
	}
}

// buildCreateAuxPrompt 渲染创建×单标签裁决 prompt：候选簇 + co-tag 事件 +
// 全量活跃版块清单（防重复创建，按簇质心相似度截断 top-60，design D2）。
func buildCreateAuxPrompt(clusters []SemanticBoardUpgradeCluster, boards []activeBoardBrief) string {
	var builder strings.Builder
	builder.WriteString("你是一个语义板块分析助手。判断以下每个辅助标签簇是否值得升级为新板块。\n")
	builder.WriteString("决策空间只有两种：create_new（创建新板块）或 skip（跳过不处理）。\n\n")
	builder.WriteString("判断原则：\n")
	builder.WriteString("- 簇内标签语义集中、有明确主题且与已有板块清单都不重复 → create_new\n")
	builder.WriteString("- 簇的主题已被某个已有板块覆盖、或簇内标签过于分散/泛化不足以形成独立板块 → skip\n\n")
	builder.WriteString("返回 JSON 格式：{\"suggestions\": [{\"decision\": \"create_new|skip\", \"board_label\": \"板块名称\", \"description\": \"板块描述\", \"auxiliary_label_ids\": [id1, id2], \"reason\": \"判断理由\"}]}\n\n")

	for i, cluster := range clusters {
		fmt.Fprintf(&builder, "【簇 %d】\n", i+1)
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
		if listed := listBoardsForCluster(boards, cluster.Centroid); len(listed) > 0 {
			builder.WriteString("已有板块清单（主题与其中某个板块重复的簇应输出 skip）：\n")
			for _, b := range listed {
				line := fmt.Sprintf("  - %s（ID=%d）", b.BoardLabel, b.BoardID)
				if d := strings.TrimSpace(b.BoardDescription); d != "" {
					line += "：" + d
				}
				builder.WriteString(line + "\n")
			}
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// listBoardsForCluster 按版块 embedding 与簇质心的余弦距离升序截断
// UpgradeBoardListLimit 条（design D2：token 上界 + 防重复参考的相关性排序）。
func listBoardsForCluster(boards []activeBoardBrief, centroid []float64) []activeBoardBrief {
	if len(boards) <= UpgradeBoardListLimit {
		return boards
	}
	sorted := make([]activeBoardBrief, len(boards))
	copy(sorted, boards)
	sort.SliceStable(sorted, func(i, j int) bool {
		return semanticBoardUpgradeDistance(sorted[i].Embedding, centroid) < semanticBoardUpgradeDistance(sorted[j].Embedding, centroid)
	})
	return sorted[:UpgradeBoardListLimit]
}

// filterSemanticBoardUpgradeSuggestions 过滤创建轮 LLM 输出：只保留 create_new
// 决策（越权 merge/watch/compose 丢弃——单一决策空间纪律）且辅助标签引用全部
// 在本轮候选集内、非空。skip 同样不返回（不落库不展示）。
func filterSemanticBoardUpgradeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, validAuxiliaryIDs map[uint]struct{}) []SemanticBoardUpgradeSuggestion {
	filtered := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Decision != SemanticBoardUpgradeDecisionCreateNew {
			continue
		}
		ids := filterKnownAuxiliaryIDs(suggestion.AuxiliaryLabelIDs, validAuxiliaryIDs)
		if len(ids) == 0 {
			continue
		}
		suggestion.AuxiliaryLabelIDs = ids
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

func ValidateActiveAuxiliaryLabels(tx *gorm.DB, ids []uint) error {
	var count int64
	if err := tx.Model(&models.SemanticLabel{}).Where("id IN ? AND label_type = ? AND status = ?", ids, "auxiliary", "active").Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("all auxiliary labels must be active auxiliary labels")
	}
	return nil
}

// FilterActiveAuxiliaryLabels 返回 ids 中仍为 active 辅助标签的子集（保持入参顺序）。
// 与 ValidateActiveAuxiliaryLabels 的取舍：后者要求全部 active 否则报错，适合「手动
// 新增单个 composition」这类必须严格 active 的场景；前者软过滤，适合升级建议确认——
// 建议生成后标签可能被禁用/合并，跳过失效项让有效项继续落库，避免陈旧建议整体 400。
func FilterActiveAuxiliaryLabels(tx *gorm.DB, ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var found []uint
	if err := tx.Model(&models.SemanticLabel{}).
		Where("id IN ? AND label_type = ? AND status = ?", ids, "auxiliary", "active").
		Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	active := make(map[uint]struct{}, len(found))
	for _, id := range found {
		active[id] = struct{}{}
	}
	keep := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := active[id]; ok {
			keep = append(keep, id)
		}
	}
	return keep, nil
}

func UniqueUintSlice(ids []uint) []uint {
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
