package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/service"
)

type boardArticleTagDTO struct {
	ID                uint    `json:"id"`
	Label             string  `json:"label"`
	Category          string  `json:"category"`
	MatchReason       string  `json:"match_reason"`
	Score             float64 `json:"score"`
	Downgraded        bool    `json:"downgraded"`
	DirectionMismatch bool    `json:"direction_mismatch"`
}
type matchDetailConfigDTO struct {
	SimThreshold           float64 `json:"sim_threshold"`
	HitRateSimBlend        float64 `json:"hit_rate_sim_blend"`
	MinEffectiveSample     int     `json:"min_effective_sample"`
	DirectHitRate          float64 `json:"direct_hit_rate"`
	DirectMaxSim           float64 `json:"direct_max_sim"`
	DirectMaxSimMinHits    int     `json:"direct_max_sim_min_hits"`
	DirectMaxSimMinHitRate float64 `json:"direct_max_sim_min_hit_rate"`
	WeightSim              float64 `json:"weight_sim"`
	WeightDensity          float64 `json:"weight_density"`
	WeightedThreshold      float64 `json:"weighted_threshold"`
	DirectHitMinOverlap    int     `json:"direct_hit_min_overlap"`
	DirectionSimThreshold  float64 `json:"direction_sim_threshold"`
	DirectHitScoreFactor   float64 `json:"direct_hit_score_factor"`
}
type DirectHitAuxiliaryDTO struct {
	TagAuxiliaryID   uint   `json:"tag_auxiliary_id"`
	TagLabel         string `json:"tag_label"`
	BoardAuxiliaryID uint   `json:"board_auxiliary_id"`
	BoardLabel       string `json:"board_label"`
}
type MatchDetailPairDTO struct {
	TagAuxiliaryID      uint    `json:"tag_auxiliary_id"`
	TagAuxiliaryLabel   string  `json:"tag_auxiliary_label"`
	BoardAuxiliaryID    uint    `json:"board_auxiliary_id"`
	BoardAuxiliaryLabel string  `json:"board_auxiliary_label"`
	Similarity          float64 `json:"similarity"`
	IsHit               bool    `json:"is_hit"`
}
type matchDetailResponse struct {
	TopicTagID           uint                    `json:"topic_tag_id"`
	TopicTagLabel        string                  `json:"topic_tag_label"`
	SemanticBoardID      uint                    `json:"semantic_board_id"`
	MatchReason          string                  `json:"match_reason"`
	Score                float64                 `json:"score"`
	Downgraded           bool                    `json:"downgraded"`
	DirectionMismatch    bool                    `json:"direction_mismatch"`
	EffectiveMinHits     int                     `json:"effective_min_hits"`
	DirectionSim         *float64                `json:"direction_sim"`
	Config               matchDetailConfigDTO    `json:"config"`
	DirectHitAuxiliaries []DirectHitAuxiliaryDTO `json:"direct_hit_auxiliaries"`
	CompositeHits        []CompositeHitDTO       `json:"composite_hits"`
	TagAuxiliaryCount    int                     `json:"tag_auxiliary_count"`
	Hits                 int                     `json:"hits"`
	HitRate              float64                 `json:"hit_rate"`
	MaxSimilarity        float64                 `json:"max_similarity"`
	Pairs                []MatchDetailPairDTO    `json:"pairs"`
}

// CompositeHitDTO describes a composite label that produced a composite_hit
// for this tag-board pair (label + ordered component sequence).
type CompositeHitDTO struct {
	ID         uint                    `json:"id"`
	Label      string                  `json:"label"`
	Components []CompositeComponentDTO `json:"components"`
}

type CompositeComponentDTO struct {
	ID       uint   `json:"id"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

func (h *semanticBoardHandler) getBoardArticles(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	feedID, _ := strconv.Atoi(c.Query("feed_id"))
	auxiliaryLabelID, _ := strconv.Atoi(c.Query("auxiliary_label_id"))
	startDate := strings.TrimSpace(c.Query("start_date"))
	endDate := strings.TrimSpace(c.Query("end_date"))
	showDirectionMismatch := c.Query("show_direction_mismatch") == "true"
	sortMode := c.DefaultQuery("sort", "quality") // "quality" | "time"

	ctx := c.Request.Context()

	// Step 1: Get tag IDs belonging to this board
	var boardTagIDs []uint
	if err := h.db.WithContext(ctx).Model(&models.TopicTagBoardLabel{}).
		Select("topic_tag_id").
		Where("semantic_board_id = ?", boardID).
		Pluck("topic_tag_id", &boardTagIDs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if len(boardTagIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"data":       []any{},
			"pagination": gin.H{"page": page, "per_page": perPage, "total": 0, "pages": 0},
		})
		return
	}

	// Step 2: Query articles
	query := h.db.WithContext(ctx).Table("articles").
		Select("DISTINCT articles.*, feeds.title AS feed_name").
		Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", boardTagIDs).
		Joins("JOIN feeds ON feeds.id = articles.feed_id")

	if feedID > 0 {
		query = query.Where("articles.feed_id = ?", feedID)
	}
	if startDate != "" {
		query = query.Where("DATE(articles.pub_date) >= ?", startDate)
	}
	if endDate != "" {
		// end_date is inclusive: include the full day
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			nextDay := t.AddDate(0, 0, 1).Format("2006-01-02")
			query = query.Where("DATE(articles.pub_date) < ?", nextDay)
		}
	}
	if auxiliaryLabelID > 0 {
		query = query.Where("articles.id IN (SELECT att_aux.article_id FROM article_topic_tags att_aux WHERE att_aux.topic_tag_id IN (SELECT topic_tag_id FROM topic_tag_semantic_labels WHERE semantic_label_id = ?))", auxiliaryLabelID)
	}

	// Count
	var total int64
	countQuery := h.db.WithContext(ctx).Table("articles").
		Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", boardTagIDs)
	if feedID > 0 {
		countQuery = countQuery.Where("articles.feed_id = ?", feedID)
	}
	if startDate != "" {
		countQuery = countQuery.Where("DATE(articles.pub_date) >= ?", startDate)
	}
	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			nextDay := t.AddDate(0, 0, 1).Format("2006-01-02")
			countQuery = countQuery.Where("DATE(articles.pub_date) < ?", nextDay)
		}
	}
	if auxiliaryLabelID > 0 {
		countQuery = countQuery.Where("articles.id IN (SELECT att_aux.article_id FROM article_topic_tags att_aux WHERE att_aux.topic_tag_id IN (SELECT topic_tag_id FROM topic_tag_semantic_labels WHERE semantic_label_id = ?))", auxiliaryLabelID)
	}
	countQuery.Select("COUNT(DISTINCT articles.id)").Scan(&total)

	// Fetch page
	offset := (page - 1) * perPage
	if sortMode == "time" {
		query = query.Order("articles.pub_date DESC, articles.id DESC").Offset(offset).Limit(perPage)
	} else {
		query = query.Order("articles.id ASC").Offset(offset).Limit(perPage)
	}

	type articleRow struct {
		models.Article
		FeedName string `gorm:"column:feed_name"`
	}
	var articles []articleRow
	if err := query.Scan(&articles).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	if len(articles) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"data":       []any{},
			"pagination": gin.H{"page": page, "per_page": perPage, "total": total, "pages": 0},
		})
		return
	}

	// Step 3: Batch query filtered_tags for current page articles
	articleIDs := make([]uint, len(articles))
	for i, a := range articles {
		articleIDs[i] = a.ID
	}

	type filteredTagRow struct {
		ArticleID         uint    `gorm:"column:article_id"`
		ID                uint    `gorm:"column:id"`
		Label             string  `gorm:"column:label"`
		Category          string  `gorm:"column:category"`
		MatchReason       string  `gorm:"column:match_reason"`
		Score             float64 `gorm:"column:score"`
		Downgraded        bool    `gorm:"column:downgraded"`
		DirectionMismatch bool    `gorm:"column:direction_mismatch"`
	}
	var tagRows []filteredTagRow
	tagQuery := h.db.WithContext(ctx).Table("article_topic_tags att").
		Select("att.article_id, tt.id, tt.label, tt.category, tbl.match_reason, tbl.score, tbl.downgraded, tbl.direction_mismatch").
		Joins("JOIN topic_tags tt ON tt.id = att.topic_tag_id").
		Joins("JOIN topic_tag_board_labels tbl ON tbl.topic_tag_id = tt.id AND tbl.semantic_board_id = ?", boardID).
		Where("att.article_id IN ?", articleIDs).
		Where("tt.status = ?", "active")
	if !showDirectionMismatch {
		tagQuery = tagQuery.Where("NOT COALESCE(tbl.direction_mismatch, false)")
	}
	if err := tagQuery.Find(&tagRows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	// Group by article_id
	tagMap := make(map[uint][]boardArticleTagDTO)
	for _, tr := range tagRows {
		tagMap[tr.ArticleID] = append(tagMap[tr.ArticleID], boardArticleTagDTO{
			ID:                tr.ID,
			Label:             tr.Label,
			Category:          tr.Category,
			MatchReason:       tr.MatchReason,
			Score:             tr.Score,
			Downgraded:        tr.Downgraded,
			DirectionMismatch: tr.DirectionMismatch,
		})
	}

	// Sort articles by match quality: tier ASC, score DESC, pub_date DESC
	// (only when sort=quality; sort=time is already handled by DB query)
	if sortMode != "time" {
		type articleSortKey struct {
			BestTier  int
			BestScore float64
		}
		sortKeys := make(map[uint]articleSortKey)
		for _, ft := range tagRows {
			t := MatchTier(ft.MatchReason, ft.Downgraded)
			existing, ok := sortKeys[ft.ArticleID]
			if !ok || t < existing.BestTier || (t == existing.BestTier && ft.Score > existing.BestScore) {
				sortKeys[ft.ArticleID] = articleSortKey{BestTier: t, BestScore: ft.Score}
			}
		}
		sort.SliceStable(articles, func(i, j int) bool {
			ki, oki := sortKeys[articles[i].ID]
			kj, okj := sortKeys[articles[j].ID]
			if !oki {
				return false
			}
			if !okj {
				return true
			}
			if ki.BestTier != kj.BestTier {
				return ki.BestTier < kj.BestTier
			}
			if ki.BestScore != kj.BestScore {
				return ki.BestScore > kj.BestScore
			}
			pdi, pdj := articles[i].PubDate, articles[j].PubDate
			if pdi != nil && pdj != nil {
				return pdi.After(*pdj)
			}
			if pdi != nil {
				return true
			}
			return false
		})
	}

	// Step 4: Assemble response
	data := make([]gin.H, len(articles))
	for i, a := range articles {
		articleData := a.ToDict()
		articleData["feed_name"] = a.FeedName
		articleData["filtered_tags"] = tagMap[a.ID]
		if articleData["filtered_tags"] == nil {
			articleData["filtered_tags"] = []boardArticleTagDTO{}
		}
		data[i] = articleData
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"pagination": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
			"pages":    pages,
		},
	})
}
func (h *semanticBoardHandler) getTagMatchDetail(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tagID, ok := parseUintParam(c, "tagId")
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var stored models.TopicTagBoardLabel
	if err := h.db.WithContext(ctx).
		Where("semantic_board_id = ? AND topic_tag_id = ?", boardID, tagID).
		First(&stored).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("match detail not found"))
		return
	}

	var tag models.TopicTag
	if err := h.db.WithContext(ctx).First(&tag, tagID).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("topic tag not found"))
		return
	}

	matcher := service.NewSemanticBoardMatchingService(h.db)
	tagAuxiliaries, err := matcher.LoadTagAuxiliaries(ctx, tagID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	boardAuxiliaries, err := matcher.LoadBoardAuxiliariesByBoardID(ctx, boardID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	config := matcher.LoadConfig(ctx)
	DirectHitAuxiliaries := BuildDirectHitAuxiliaryDTOs(tagAuxiliaries, boardAuxiliaries)
	detail := service.ComputeMatchDetail(tagAuxiliaries, boardAuxiliaries, config)
	pairs := MatchDetailPairsToDTOs(detail.Pairs)
	hits := detail.Hits
	hitRate := detail.HitRate
	maxSimilarity := detail.MaxSimilarity

	var directionSim *float64
	if tagEmb, _ := matcher.LoadTagIdentityEmbedding(ctx, tagID); len(tagEmb) > 0 {
		var board models.SemanticLabel
		if err := h.db.WithContext(ctx).
			Select("id, embedding").
			Where("id = ? AND label_type = ?", boardID, "board").
			First(&board).Error; err == nil && board.Embedding != nil {
			if boardVec, parseErr := service.ParsePgVector(*board.Embedding); parseErr == nil {
				sim := service.CosineSimilarity(tagEmb, boardVec)
				directionSim = &sim
			}
		}
	}

	effectiveMinHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))

	var compositeHits []CompositeHitDTO
	if stored.MatchReason == "composite_hit" {
		compositeHits = h.loadCompositeHitDTOs(ctx, boardID, tagID)
	}

	respondOK(c, matchDetailResponse{
		TopicTagID:           tag.ID,
		TopicTagLabel:        tag.Label,
		SemanticBoardID:      boardID,
		MatchReason:          stored.MatchReason,
		Score:                stored.Score,
		Downgraded:           stored.Downgraded,
		DirectionMismatch:    stored.DirectionMismatch,
		EffectiveMinHits:     effectiveMinHits,
		DirectionSim:         directionSim,
		Config:               matchDetailConfigToDTO(config),
		DirectHitAuxiliaries: DirectHitAuxiliaries,
		CompositeHits:        compositeHits,
		TagAuxiliaryCount:    len(tagAuxiliaries),
		Hits:                 hits,
		HitRate:              hitRate,
		MaxSimilarity:        maxSimilarity,
		Pairs:                pairs,
	})
}

// loadCompositeHitDTOs resolves the composite labels shared between a board's
// mounted composites and a tag's attached composites, with their ordered
// component sequences (spec: 匹配详情展示组合名+组件序列).
func (h *semanticBoardHandler) loadCompositeHitDTOs(ctx context.Context, boardID uint, tagID uint) []CompositeHitDTO {
	var composites []struct {
		ID    uint
		Label string
	}
	if err := h.db.WithContext(ctx).
		Table("board_composition AS bc").
		Select("DISTINCT sl.id, sl.label").
		Joins("JOIN topic_tag_semantic_labels AS ttsl ON ttsl.semantic_label_id = bc.auxiliary_label_id AND ttsl.topic_tag_id = ?", tagID).
		Joins("JOIN semantic_labels AS sl ON sl.id = bc.auxiliary_label_id AND sl.label_type = ? AND sl.status = ?", "composite", "active").
		Where("bc.board_id = ?", boardID).
		Scan(&composites).Error; err != nil {
		return nil
	}
	if len(composites) == 0 {
		return nil
	}
	result := make([]CompositeHitDTO, 0, len(composites))
	for _, composite := range composites {
		var components []CompositeComponentDTO
		if err := h.db.WithContext(ctx).
			Table("composite_components AS cc").
			Select("cc.component_label_id AS id, sl.label, cc.position").
			Joins("JOIN semantic_labels AS sl ON sl.id = cc.component_label_id").
			Where("cc.composite_id = ?", composite.ID).
			Order("cc.position ASC").
			Scan(&components).Error; err != nil {
			components = nil
		}
		result = append(result, CompositeHitDTO{ID: composite.ID, Label: composite.Label, Components: components})
	}
	return result
}
func BuildDirectHitAuxiliaryDTOs(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []service.BoardAuxiliaryLabel) []DirectHitAuxiliaryDTO {
	byID := make(map[uint]models.SemanticLabel, len(tagAuxiliaries))
	for _, tagAuxiliary := range tagAuxiliaries {
		byID[tagAuxiliary.ID] = tagAuxiliary
	}

	directHits := []DirectHitAuxiliaryDTO{}
	for _, boardAuxiliary := range boardAuxiliaries {
		tagAuxiliary, ok := byID[boardAuxiliary.AuxiliaryLabelID]
		if !ok {
			continue
		}
		directHits = append(directHits, DirectHitAuxiliaryDTO{
			TagAuxiliaryID:   tagAuxiliary.ID,
			TagLabel:         tagAuxiliary.Label,
			BoardAuxiliaryID: boardAuxiliary.AuxiliaryLabelID,
			BoardLabel:       boardAuxiliary.Label,
		})
	}
	return directHits
}
func MatchTier(matchReason string, downgraded bool) int {
	switch {
	case matchReason == "direct_hit":
		return 0
	case matchReason == "hit_rate":
		return 1
	case matchReason == "max_sim" && !downgraded:
		return 2
	default: // max_sim(downgraded) or weighted
		return 3
	}
}
func MatchDetailPairsToDTOs(pairs []service.MatchDetailPair) []MatchDetailPairDTO {
	dtos := make([]MatchDetailPairDTO, 0, len(pairs))
	for _, pair := range pairs {
		dtos = append(dtos, MatchDetailPairDTO{
			TagAuxiliaryID:      pair.TagAuxiliaryID,
			TagAuxiliaryLabel:   pair.TagAuxiliaryLabel,
			BoardAuxiliaryID:    pair.BoardAuxiliaryID,
			BoardAuxiliaryLabel: pair.BoardAuxiliaryLabel,
			Similarity:          pair.Similarity,
			IsHit:               pair.IsHit,
		})
	}
	return dtos
}
func matchDetailConfigToDTO(config service.SemanticBoardMatchConfig) matchDetailConfigDTO {
	return matchDetailConfigDTO{
		SimThreshold:           config.SimThreshold,
		HitRateSimBlend:        config.HitRateSimBlend,
		MinEffectiveSample:     config.MinEffectiveSample,
		DirectHitRate:          config.DirectHitRate,
		DirectMaxSim:           config.DirectMaxSim,
		DirectMaxSimMinHits:    config.DirectMaxSimMinHits,
		DirectMaxSimMinHitRate: config.DirectMaxSimMinHitRate,
		WeightSim:              config.WeightSim,
		WeightDensity:          config.WeightDensity,
		WeightedThreshold:      config.WeightedThreshold,
		DirectHitMinOverlap:    config.DirectHitMinOverlap,
		DirectionSimThreshold:  config.DirectionSimThreshold,
		DirectHitScoreFactor:   config.DirectHitScoreFactor,
	}
}
func (h *semanticBoardHandler) rematchAll(c *gin.Context) {
	ctx := c.Request.Context()
	var tagIDs []uint
	if err := h.db.WithContext(ctx).
		Model(&models.TopicTagBoardLabel{}).
		Distinct("topic_tag_id").
		Pluck("topic_tag_id", &tagIDs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	matcher := service.NewSemanticBoardMatchingService(h.db)
	success, failed := 0, 0
	for _, tid := range tagIDs {
		if _, err := matcher.MatchTopicTag(ctx, tid); err != nil {
			logging.Warnf("[rematch-all] failed for tag %d: %v", tid, err)
			failed++
			continue
		}
		success++
	}
	respondOK(c, gin.H{"success": success, "failed": failed, "total": len(tagIDs)})
}
func (h *semanticBoardHandler) getMatchingConfig(c *gin.Context) {
	respondOK(c, h.getAllConfigs(c))
}
func (h *semanticBoardHandler) updateMatchingConfig(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	for key, raw := range body {
		if !isSemanticBoardConfigKey(key) {
			respondError(c, http.StatusBadRequest, fmt.Errorf("unsupported config key %q", key))
			return
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			respondError(c, http.StatusBadRequest, fmt.Errorf("config value for %s is required", key))
			return
		}
		if err := validateSemanticBoardConfigValue(key, value); err != nil {
			respondError(c, http.StatusBadRequest, err)
			return
		}
		setting := models.AISettings{Key: key, Value: value, Description: "SemanticBoard matching config"}
		if err := h.db.WithContext(c.Request.Context()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_at"})}).Create(&setting).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
	}
	// Invalidate config cache so LoadConfig reads fresh values from DB
	service.InvalidateMatchingConfigCache()
	respondOK(c, h.getAllConfigs(c))
}
func semanticBoardMatchConfigToMap(config service.SemanticBoardMatchConfig) gin.H {
	return gin.H{
		"semantic_board_match_sim_threshold":               config.SimThreshold,
		"semantic_board_match_direct_hit_rate":             config.DirectHitRate,
		"semantic_board_match_direct_max_sim":              config.DirectMaxSim,
		"semantic_board_match_direct_max_sim_min_hits":     config.DirectMaxSimMinHits,
		"semantic_board_match_direct_max_sim_min_hit_rate": config.DirectMaxSimMinHitRate,
		"semantic_board_match_min_effective_sample":        config.MinEffectiveSample,
		"semantic_board_match_hit_rate_sim_blend":          config.HitRateSimBlend,
		"semantic_board_match_weight_sim":                  config.WeightSim,
		"semantic_board_match_weight_density":              config.WeightDensity,
		"semantic_board_match_weighted_threshold":          config.WeightedThreshold,
		"semantic_board_match_max_boards":                  config.MaxBoards,
		"semantic_board_match_direct_hit_min_overlap":      config.DirectHitMinOverlap,
		"semantic_board_match_direction_sim_threshold":     config.DirectionSimThreshold,
	}
}
