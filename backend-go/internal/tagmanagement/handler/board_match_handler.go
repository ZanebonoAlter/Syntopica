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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service"
)

type boardArticleTagDTO struct {
	ID       uint   `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	// Description 是标签自身描述（取 topic_tags.description），归档重做
	// 需要它做标签卡 popover；标签无描述时为空串而非缺字段。
	Description       string  `json:"description"`
	MatchReason       string  `json:"match_reason"`
	Score             float64 `json:"score"`
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
	reportStatus, ok := parseReportStatusFilter(c)
	if !ok {
		if strings.TrimSpace(c.Query("report_status")) == "" {
			respondError(c, http.StatusBadRequest, fmt.Errorf("invalid in_report (want true|false)"))
		} else {
			respondError(c, http.StatusBadRequest, fmt.Errorf("invalid report_status (want all|in_report|not_in_report|weak)"))
		}
		return
	}

	ctx := c.Request.Context()

	// Step 1: Get tag IDs belonging to this board
	var boardTagIDs []uint
	if err := h.db.WithContext(ctx).Model(&tagmodels.TopicTagBoardLabel{}).
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

	// 归档口径（rework-workshop-into-radar-and-desk D5）：in_report 由窗口内
	// 日报分区线索派生，不落文章表；报告窗口 = 显式日期或最近 6 份日报。
	var windowStart, windowEnd *time.Time
	if startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			windowStart = &t
		}
	}
	if endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			windowEnd = &t
		}
	}
	membership, err := service.LoadReportMembership(ctx, h.db, boardID, service.ReportWindow{Start: windowStart, End: windowEnd})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	reportedIDs := service.SortArticleIDs(membership.ArticleIDs)

	// Step 2: Query articles
	query := h.db.WithContext(ctx).Table("articles").
		Select("DISTINCT articles.*, feeds.title AS feed_name").
		Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", boardTagIDs).
		Joins("JOIN feeds ON feeds.id = articles.feed_id")

	if feedID > 0 {
		query = query.Where("articles.feed_id = ?", feedID)
	}
	if dateCond, dateArgs := articleDateWindowCond("articles", startDate, endDate); dateCond != "" {
		query = query.Where(dateCond, dateArgs...)
	}
	if auxiliaryLabelID > 0 {
		query = query.Where("articles.id IN (SELECT att_aux.article_id FROM article_topic_tags att_aux WHERE att_aux.topic_tag_id IN (SELECT topic_tag_id FROM topic_tag_semantic_labels WHERE semantic_label_id = ?))", auxiliaryLabelID)
	}
	query = applyReportStatusFilter(query, reportStatus, reportedIDs, boardID, showDirectionMismatch)

	// Count
	var total int64
	countQuery := h.db.WithContext(ctx).Table("articles").
		Joins("JOIN article_topic_tags att ON att.article_id = articles.id AND att.topic_tag_id IN ?", boardTagIDs)
	if feedID > 0 {
		countQuery = countQuery.Where("articles.feed_id = ?", feedID)
	}
	if dateCond, dateArgs := articleDateWindowCond("articles", startDate, endDate); dateCond != "" {
		countQuery = countQuery.Where(dateCond, dateArgs...)
	}
	if auxiliaryLabelID > 0 {
		countQuery = countQuery.Where("articles.id IN (SELECT att_aux.article_id FROM article_topic_tags att_aux WHERE att_aux.topic_tag_id IN (SELECT topic_tag_id FROM topic_tag_semantic_labels WHERE semantic_label_id = ?))", auxiliaryLabelID)
	}
	countQuery = applyReportStatusFilter(countQuery, reportStatus, reportedIDs, boardID, showDirectionMismatch)
	countQuery.Select("COUNT(DISTINCT articles.id)").Scan(&total)

	// Fetch page
	offset := (page - 1) * perPage
	if sortMode == "time" {
		query = query.Order("articles.pub_date DESC, articles.id DESC").Offset(offset).Limit(perPage)
	} else {
		// quality 序：排序键下推 SQL（先 ORDER BY 再 LIMIT/OFFSET），排序因此
		// 是**全局序**而非页内序——跨分页稳定（见 boardArticleQualitySortJoin）。
		sortJoin, sortArgs := boardArticleQualitySortJoin(boardID, showDirectionMismatch, startDate, endDate)
		query = query.
			Select(fmt.Sprintf("DISTINCT articles.*, feeds.title AS feed_name, COALESCE(qk.best_tier, %d) AS match_best_tier, COALESCE(qk.best_score, 0) AS match_best_score", boardArticleNoSortTier)).
			Joins(sortJoin, sortArgs...).
			Order("match_best_tier ASC, match_best_score DESC, articles.pub_date DESC, articles.id ASC").
			Offset(offset).Limit(perPage)
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
		Description       string  `gorm:"column:description"`
		MatchReason       string  `gorm:"column:match_reason"`
		Score             float64 `gorm:"column:score"`
		DirectionMismatch bool    `gorm:"column:direction_mismatch"`
	}
	var tagRows []filteredTagRow
	tagQuery := h.db.WithContext(ctx).Table("article_topic_tags att").
		Select("att.article_id, tt.id, tt.label, tt.category, tt.description, tbl.match_reason, tbl.score, tbl.direction_mismatch").
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

	// Group by article_id；组内按证据强度排序（tier→score→id），归档以
	// 「首条匹配标签」归组时必须稳定拿到最强证据。
	tagMap := make(map[uint][]boardArticleTagDTO)
	for _, tr := range tagRows {
		tagMap[tr.ArticleID] = append(tagMap[tr.ArticleID], boardArticleTagDTO{
			ID:                tr.ID,
			Label:             tr.Label,
			Category:          tr.Category,
			Description:       tr.Description,
			MatchReason:       tr.MatchReason,
			Score:             tr.Score,
			DirectionMismatch: tr.DirectionMismatch,
		})
	}
	for id := range tagMap {
		tags := tagMap[id]
		sort.SliceStable(tags, func(i, j int) bool {
			ti := service.ArchiveEvidenceRank(tags[i].MatchReason)
			tj := service.ArchiveEvidenceRank(tags[j].MatchReason)
			if ti != tj {
				return ti < tj
			}
			if tags[i].Score != tags[j].Score {
				return tags[i].Score > tags[j].Score
			}
			return tags[i].ID < tags[j].ID
		})
	}
	strongEvidence := make(map[uint]bool, len(tagRows))
	for _, tr := range tagRows {
		if service.IsWeakEvidence(tr.MatchReason) {
			continue
		}
		if !showDirectionMismatch && tr.DirectionMismatch {
			continue
		}
		strongEvidence[tr.ArticleID] = true
	}

	// 排序不再在 Go 内存里做：quality 序的排序键（文章级 best_tier /
	// best_score）已在 SQL 层聚合并参与 ORDER BY，分页发生在排序之后，返回
	// 序列即全局质量序（页内序遗留已下线，见 boardArticleQualitySortJoin）。
	// sort=time 的序同样在 SQL 层（pub_date DESC, id DESC）。

	// Step 4: Assemble response
	data := make([]gin.H, len(articles))
	for i, a := range articles {
		articleData := a.ToDict()
		articleData["feed_name"] = a.FeedName
		// 必须赋非 nil 的空切片：typed nil 装进 interface 后 != nil，会序列化成
		// JSON null，前端归档分组对 filtered_tags 做 .length 会炸。
		tags := tagMap[a.ID]
		if len(tags) == 0 {
			tags = []boardArticleTagDTO{}
		}
		articleData["filtered_tags"] = tags
		if _, in := membership.ArticleIDs[a.ID]; in {
			articleData["in_report"] = true
		} else {
			articleData["in_report"] = false
		}
		articleData["weak_evidence"] = !strongEvidence[a.ID]
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
	var stored tagmodels.TopicTagBoardLabel
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

// parseReportStatusFilter 解析归档筛选。兼容旧调用：未给 report_status 时
// 可用 in_report=true|false（true=只看已入，false=只看未入）。
func parseReportStatusFilter(c *gin.Context) (string, bool) {
	status := strings.TrimSpace(c.Query("report_status"))
	if status == "" {
		raw, exists := c.GetQuery("in_report")
		if !exists {
			return "all", true
		}
		switch strings.TrimSpace(raw) {
		case "true", "1":
			return "in_report", true
		case "false", "0":
			return "not_in_report", true
		default:
			return "", false
		}
	}
	switch status {
	case "all", "in_report", "not_in_report", "weak":
		return status, true
	default:
		return "", false
	}
}

// applyReportStatusFilter 把归档筛选作用到文章查询（主查询与计数共用）。
func applyReportStatusFilter(q *gorm.DB, status string, reportedIDs []uint, boardID uint, showDirectionMismatch bool) *gorm.DB {
	switch status {
	case "not_in_report":
		if len(reportedIDs) == 0 {
			return q
		}
		return q.Where("articles.id NOT IN ?", reportedIDs)
	case "in_report":
		if len(reportedIDs) == 0 {
			return q.Where("1 = 0")
		}
		return q.Where("articles.id IN ?", reportedIDs)
	case "weak":
		return service.ApplyWeakEvidenceFilter(q, boardID, showDirectionMismatch)
	default:
		return q
	}
}

// boardArticleNoSortTier 是「拿不到文章级排序键」文章的哨兵 tier：比
// MatchTier 的最差档（3）更差，保证这类文章排在所有有可见匹配证据的文章之后
// ——与旧的页内内存排序「无 key 排最后」一致。
const boardArticleNoSortTier = 99

// articleDateWindowCond 返回归档窗口的日期过滤条件（闭区间：[start_date,
// end_date]）。alias 是文章表在查询中的别名：主查询与计数查询用 articles，
// 排序键子查询用 a（子查询内的 articles 别名）。三个查询共用同一份口径，
// 避免窗口边界漂移。
func articleDateWindowCond(alias, startDate, endDate string) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if startDate != "" {
		conds = append(conds, fmt.Sprintf("DATE(%s.pub_date) >= ?", alias))
		args = append(args, startDate)
	}
	if endDate != "" {
		// end_date is inclusive: include the full day
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			nextDay := t.AddDate(0, 0, 1).Format("2006-01-02")
			conds = append(conds, fmt.Sprintf("DATE(%s.pub_date) < ?", alias))
			args = append(args, nextDay)
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return strings.Join(conds, " AND "), args
}

// boardArticleQualitySortJoin 构造 quality 排序键的 LEFT JOIN 子查询（qk 别名）。
//
// 目的（R-02）：把文章级排序键从 Go 内存排序下推到 SQL，让 quality 序成为
// **全局序**——先 ORDER BY 再 LIMIT/OFFSET，跨分页稳定。旧实现是「先按
// articles.id ASC 切页、再在页内按 tier/score 排」，窗口超过 per_page 时会
// 强弱混排（页 2 的 direct_hit 排在页 1 的 max_sim 之后）。
//
// 口径复刻 MatchTier（口径本身不改）：direct_hit=0 → hit_rate=1 →
// max_sim(未降级)=2 → 其余（max_sim 降级 / weighted / composite_hit / 未知）=3；
// 同 tier 内取最高分作为 best_score。注意 hit_rate 的判定先于降级判定（降级的
// hit_rate 仍是 tier 1），与 MatchTier 一致。
//
// 参与聚合的标签集合与 filtered_tags 完全一致：标签 active
// （topic_tags.status）、属于本版块（topic_tag_board_labels.semantic_board_id）、
// 方向未通过默认排除。因此「没有可见匹配证据」的文章拿不到排序键
// （best_tier 为 NULL），排序时落到最差档之后。
//
// 窗口日期条件只用于收窄子查询的扫描范围（与主查询同一份口径，故不会漏掉主
// 查询命中的文章）；缺省时子查询覆盖全部带本版块标签的文章，与主查询同量级。
func boardArticleQualitySortJoin(boardID uint, showDirectionMismatch bool, startDate, endDate string) (string, []any) {
	dateCond, dateArgs := articleDateWindowCond("a", startDate, endDate)

	conds := make([]string, 0, 2)
	if !showDirectionMismatch {
		conds = append(conds, "NOT COALESCE(tbl.direction_mismatch, false)")
	}
	if dateCond != "" {
		conds = append(conds, dateCond)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	articlesJoin := ""
	if dateCond != "" {
		// 窗口过滤需要文章表：只让窗口内的文章参与排序键聚合。
		articlesJoin = "JOIN articles a ON a.id = att.article_id"
	}

	sortJoin := fmt.Sprintf(`LEFT JOIN (
		SELECT ranked.article_id AS article_id, ranked.tier AS best_tier, ranked.score AS best_score
		FROM (
			SELECT keyed.article_id AS article_id, keyed.tier AS tier, keyed.score AS score,
				ROW_NUMBER() OVER (PARTITION BY keyed.article_id ORDER BY keyed.tier ASC, keyed.score DESC) AS rn
			FROM (
				SELECT att.article_id AS article_id,
					CASE
						WHEN tbl.match_reason = 'direct_hit' THEN 0
						WHEN tbl.match_reason = 'hit_rate' THEN 1
						WHEN tbl.match_reason = 'max_sim' THEN 2
						ELSE 3
					END AS tier,
					COALESCE(tbl.score, 0) AS score
				FROM article_topic_tags att
				JOIN topic_tags tt ON tt.id = att.topic_tag_id AND tt.status = 'active'
				JOIN topic_tag_board_labels tbl ON tbl.topic_tag_id = att.topic_tag_id AND tbl.semantic_board_id = ?
				%s
				%s
			) keyed
		) ranked
		WHERE ranked.rn = 1
	) qk ON qk.article_id = articles.id`, articlesJoin, where)

	args := make([]any, 0, 1+len(dateArgs))
	args = append(args, boardID)
	args = append(args, dateArgs...)
	return sortJoin, args
}

// MatchTier 是挂载证据的强度档（0 最强）。downgraded 标记已于 2026-11-12
// 退役：max_sim 统一为 2 档（原降级 max_sim 与 weighted 同 3 档是低估）。
func MatchTier(matchReason string) int {
	switch matchReason {
	case "direct_hit":
		return 0
	case "hit_rate":
		return 1
	case "max_sim":
		return 2
	default: // weighted
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
		Model(&tagmodels.TopicTagBoardLabel{}).
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
	// 先校验全部字段、再在同一事务内写入：任一键非法 → 整体拒绝且不部分落库
	// （enable-story-cross-day-continuation：相关事件阈值与其他匹配参数必须同
	// 事务生效；旧实现边校验边写，map 迭代顺序还把「哪些写进去了」变成随机）。
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pending := make([]models.AISettings, 0, len(keys))
	for _, key := range keys {
		if !isSemanticBoardConfigKey(key) {
			respondError(c, http.StatusBadRequest, fmt.Errorf("unsupported config key %q", key))
			return
		}
		value := strings.TrimSpace(fmt.Sprint(body[key]))
		if value == "" {
			respondError(c, http.StatusBadRequest, fmt.Errorf("config value for %s is required", key))
			return
		}
		if err := validateSemanticBoardConfigValue(key, value); err != nil {
			respondError(c, http.StatusBadRequest, err)
			return
		}
		pending = append(pending, models.AISettings{Key: key, Value: value, Description: "SemanticBoard matching config"})
	}
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		for i := range pending {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_at"})}).Create(&pending[i]).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
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
		"semantic_board_match_direct_hit_min_ratio":        config.DirectHitMinRatio,
		"semantic_board_match_direction_sim_threshold":     config.DirectionSimThreshold,
	}
}
