package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

var semanticBoardLabelEmbedder service.AuxiliaryLabelEmbedder = service.DefaultAuxiliaryLabelEmbedder
var semanticBoardUpgradeLLMFactory = service.NewDefaultSemanticBoardUpgradeLLM

type semanticBoardHandler struct {
	db        *gorm.DB
	auxiliary *service.AuxiliaryLabelService
	backfill  *service.SemanticBoardBackfillService
}

type semanticBoardRequest struct {
	Label             string   `json:"label"`
	Description       string   `json:"description"`
	DisplayOrder      *int     `json:"display_order"`
	Protected         *bool    `json:"protected"`
	Status            string   `json:"status"`
	AuxiliaryLabels   []uint   `json:"auxiliary_labels"`
	EnrichmentEnabled *bool    `json:"enrichment_enabled"`
	WindowDays        *int     `json:"window_days"`
	ContextLayers     []string `json:"context_layers"`
	// RelationAutoDiscoveryEnabled — per-board auto relation discovery switch
	// (nil = untouched; absent column on old rows reads as false).
	RelationAutoDiscoveryEnabled *bool `json:"relation_auto_discovery_enabled"`
}

type suggestedAuxiliaryDTO struct {
	ID         uint     `json:"id"`
	Label      string   `json:"label"`
	Slug       string   `json:"slug"`
	Aliases    []string `json:"aliases"`
	RefCount   int      `json:"ref_count"`
	Similarity float64  `json:"similarity"`
}

type suggestAuxiliariesResponse struct {
	Items    []suggestedAuxiliaryDTO `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type addCompositionRequest struct {
	AuxiliaryLabelID uint `json:"auxiliary_label_id"`
}

type semanticBoardDTO struct {
	ID           uint     `json:"id"`
	Label        string   `json:"label"`
	Slug         string   `json:"slug"`
	Aliases      []string `json:"aliases"`
	RefCount     int      `json:"ref_count"`
	TagCount     int64    `json:"tag_count"`
	Description  string   `json:"description"`
	DisplayOrder int      `json:"display_order"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Protected    bool     `json:"protected"`
	// 循环 B 配置（fix-board-analysis-material：DTO 丢字段导致前端开关永远显示
	// 关，且编辑弹窗保存会把已开开关覆盖回 false）。
	EnrichmentEnabled            bool     `json:"enrichment_enabled"`
	WindowDays                   int      `json:"window_days"`
	ContextLayers                []string `json:"context_layers"`
	RelationAutoDiscoveryEnabled bool     `json:"relation_auto_discovery_enabled"`
	CreatedAt                    any      `json:"created_at"`
	UpdatedAt                    any      `json:"updated_at"`
}

type semanticBoardAuxiliaryDTO struct {
	ID           uint     `json:"id"`
	Label        string   `json:"label"`
	Slug         string   `json:"slug"`
	Aliases      []string `json:"aliases"`
	RefCount     int      `json:"ref_count"`
	Description  string   `json:"description"`
	DisplayOrder int      `json:"display_order"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Protected    bool     `json:"protected"`
}

type mergeAuxiliaryAliasRequest struct {
	SourceID uint `json:"source_id"`
	TargetID uint `json:"target_id"`
}

func RegisterSemanticBoardRoutes(rg *gin.RouterGroup) {
	handler := &semanticBoardHandler{
		db:        repository.Repo.DB(),
		auxiliary: service.NewAuxiliaryLabelService(repository.Repo.DB(), nil),
		backfill:  service.NewSemanticBoardBackfillService(repository.Repo.DB()),
	}

	boards := rg.Group("/semantic-boards")
	{
		boards.POST("/upgrade-suggest", handler.suggestUpgrades)
		boards.POST("/upgrade-execute", handler.executeUpgrade)

		// §5: persisted upgrade-suggestions resource (query / dismiss / generate).
		// Legacy upgrade-suggest is retained for a compatibility window.
		boards.GET("/upgrade-suggestions", handler.listUpgradeSuggestions)
		boards.POST("/upgrade-suggestions/:id/dismiss", handler.dismissUpgradeSuggestion)
		boards.POST("/upgrade-suggestions/generate", handler.generateUpgradeSuggestions)
		boards.POST("/backfill", handler.enqueueBackfill)
		boards.GET("/backfill/:id", handler.getBackfillJob)
		boards.POST("/backfill-embeddings", handler.backfillBoardEmbeddings)
		boards.POST("/rematch-all", handler.rematchAll)
		boards.GET("/matching-config", handler.getMatchingConfig)
		boards.PUT("/matching-config", handler.updateMatchingConfig)

		boards.GET("/suggest-auxiliaries", handler.suggestAuxiliaries)

		boards.GET("", handler.listSemanticBoards)
		boards.POST("", handler.createSemanticBoard)
		boards.GET("/:id", handler.getSemanticBoard)
		boards.PUT("/:id", handler.updateSemanticBoard)
		boards.DELETE("/:id", handler.deleteSemanticBoard)
		boards.GET("/:id/suggest-auxiliaries", handler.suggestAuxiliariesForBoard)
		boards.GET("/:id/articles", handler.getBoardArticles)
		boards.GET("/:id/match-detail/:tagId", handler.getTagMatchDetail)
		boards.GET("/:id/composition", handler.getBoardComposition)
		boards.POST("/:id/composition", handler.addBoardComposition)
		boards.DELETE("/:id/composition/:auxiliary_label_id", handler.removeBoardComposition)
	}

	auxiliary := rg.Group("/auxiliary-labels")
	{
		auxiliary.GET("", handler.listAuxiliaryLabels)
		auxiliary.GET("/clusters", handler.clusterAuxiliaryLabels)
		auxiliary.POST("/merge-alias", handler.mergeAuxiliaryAlias)
		auxiliary.POST("/gc", handler.gcAuxiliaryLabels)
		auxiliary.POST("/:id/disable", handler.disableAuxiliaryLabel)
	}

	tags := rg.Group("/tags")
	{
		tags.GET("/:id/auxiliary-labels", handler.getTagAuxiliaryLabels)
		tags.GET("/:id/semantic-boards", handler.getTagSemanticBoards)
	}

	registerCompositeLabelRoutes(rg)
}

func (h *semanticBoardHandler) listSemanticBoards(c *gin.Context) {
	boards, err := h.loadSemanticBoards(c.Request.Context(), c.Query("search"), c.Query("status"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"items": boards, "total": len(boards)})
}

func (h *semanticBoardHandler) getSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var label models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", id, "board").First(&label).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	tagCounts, err := h.loadSemanticBoardTagCounts(c.Request.Context(), []uint{label.ID})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, semanticBoardToDTO(label, tagCounts[label.ID]))
}

func (h *semanticBoardHandler) createSemanticBoard(c *gin.Context) {
	var req semanticBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		respondError(c, http.StatusBadRequest, fmt.Errorf("label is required"))
		return
	}
	description := strings.TrimSpace(req.Description)
	input := semanticBoardEmbeddingInput(label, description)
	pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), input, service.AuxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	protected := true
	if req.Protected != nil {
		protected = *req.Protected
	}
	board := models.SemanticLabel{
		Label:        label,
		Slug:         service.UniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()), service.Slugify(label)),
		Embedding:    &pgVector,
		LabelType:    "board",
		Description:  description,
		Source:       "manual",
		Status:       "active",
		Protected:    protected,
		DisplayOrder: intValue(req.DisplayOrder),
	}
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&board).Error; err != nil {
			return err
		}
		return insertBoardComposition(tx, board.ID, req.AuxiliaryLabels)
	}); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": board.ID})
}

func semanticBoardEmbeddingInput(label, description string) string {
	label = strings.TrimSpace(label)
	description = strings.TrimSpace(description)
	if description != "" {
		return label + ". " + description
	}
	return label
}

func (h *semanticBoardHandler) updateSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req semanticBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", id, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	boardOrigLabel := board.Label
	boardOrigDesc := board.Description
	if label := strings.TrimSpace(req.Label); label != "" && label != board.Label {
		board.Label = label
		board.Slug = service.UniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()).Where("id <> ?", board.ID), service.Slugify(label))
	}
	if desc := strings.TrimSpace(req.Description); desc != board.Description {
		board.Description = desc
	}
	if board.Label != boardOrigLabel || board.Description != boardOrigDesc {
		input := semanticBoardEmbeddingInput(board.Label, board.Description)
		pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), input, service.AuxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		board.Embedding = &pgVector
	}
	if req.DisplayOrder != nil {
		board.DisplayOrder = *req.DisplayOrder
	}
	if req.Protected != nil {
		board.Protected = *req.Protected
	}
	if req.Status == "active" || req.Status == "disabled" {
		if req.Status == "disabled" && board.Status != "disabled" {
			// Disabled labels drop their vectors (~2×2560-dim per row); re-enable
			// regenerates via backfill-board-embeddings (llm_extract for aux labels).
			board.Embedding = nil
			board.MergeEmbedding = nil
		}
		board.Status = req.Status
	}
	if req.EnrichmentEnabled != nil {
		board.EnrichmentEnabled = *req.EnrichmentEnabled
	}
	if req.RelationAutoDiscoveryEnabled != nil {
		board.RelationAutoDiscoveryEnabled = *req.RelationAutoDiscoveryEnabled
	}
	if req.WindowDays != nil && *req.WindowDays >= 1 {
		board.WindowDays = *req.WindowDays
	}
	if req.ContextLayers != nil {
		board.ContextLayers = req.ContextLayers
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&board).Error; err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": board.ID})
}

func (h *semanticBoardHandler) deleteSemanticBoard(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	result := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).Where("id = ? AND label_type = ?", id, "board").Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil})
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}
	respondOK(c, gin.H{"id": id})
}

// BuildDirectHitAuxiliaryDTOs builds direct hit DTOs for the match detail view.

// MatchTier returns a priority tier for a given match reason and downgrade status.
// Lower tiers indicate higher-quality matches.

func (h *semanticBoardHandler) getBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var rows []models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
		Joins("JOIN board_composition ON board_composition.auxiliary_label_id = semantic_labels.id").
		Where("board_composition.board_id = ? AND semantic_labels.label_type = ?", boardID, "auxiliary").
		Order("semantic_labels.ref_count DESC, semantic_labels.id ASC").
		Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]semanticBoardAuxiliaryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, auxiliaryToDTO(row))
	}

	// add-composite-labels：组合挂载条目单独返回（aux 的 auxiliary_label_id 列复用）
	compositeMounts := make([]gin.H, 0)
	var compositeRows []models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
		Joins("JOIN board_composition bc ON bc.auxiliary_label_id = semantic_labels.id").
		Where("bc.board_id = ? AND semantic_labels.label_type = ?", boardID, "composite").
		Order("semantic_labels.ref_count DESC, semantic_labels.id ASC").
		Find(&compositeRows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if len(compositeRows) > 0 {
		ids := make([]uint, 0, len(compositeRows))
		for _, r := range compositeRows {
			ids = append(ids, r.ID)
		}
		var comps []models.CompositeComponent
		if err := h.db.WithContext(c.Request.Context()).Where("composite_id IN ?", ids).Order("composite_id, position").Find(&comps).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		var compLabels []models.SemanticLabel
		if err := h.db.WithContext(c.Request.Context()).
			Select("id, label").
			Where("id IN (SELECT component_label_id FROM composite_components WHERE composite_id IN ?)", ids).
			Find(&compLabels).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		labelByID := make(map[uint]string, len(compLabels))
		for _, l := range compLabels {
			labelByID[l.ID] = l.Label
		}
		chainByComposite := make(map[uint][]string, len(comps))
		for _, comp := range comps {
			chainByComposite[comp.CompositeID] = append(chainByComposite[comp.CompositeID], labelByID[comp.ComponentLabelID])
		}
		for _, r := range compositeRows {
			compositeMounts = append(compositeMounts, gin.H{
				"id": r.ID, "label": r.Label, "slug": r.Slug, "status": r.Status,
				"ref_count": r.RefCount, "components": chainByComposite[r.ID],
			})
		}
	}
	respondOK(c, gin.H{"items": items, "total": len(items), "composites": compositeMounts})
}

func (h *semanticBoardHandler) removeBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	auxiliaryID, ok := parseUintParam(c, "auxiliary_label_id")
	if !ok {
		return
	}
	if err := h.auxiliary.RemoveBoardComposition(c.Request.Context(), boardID, auxiliaryID); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	service.InvalidateMatchCache()
	respondOK(c, gin.H{"board_id": boardID, "auxiliary_label_id": auxiliaryID})
}

func (h *semanticBoardHandler) listAuxiliaryLabels(c *gin.Context) {
	query := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).Where("label_type = ?", "auxiliary")
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(search)+"%", "%"+strings.ToLower(service.Slugify(search))+"%")
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	var labels []models.SemanticLabel
	offset := (page - 1) * perPage
	if err := query.Order("ref_count DESC, id ASC").Offset(offset).Limit(perPage).Find(&labels).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	items := make([]semanticBoardAuxiliaryDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, auxiliaryToDTO(label))
	}

	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": items,
			"total": total,
		},
		"pagination": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
			"pages":    pages,
		},
	})
}

type auxiliaryLabelClusterDTO struct {
	ID       uint   `json:"id"`
	Label    string `json:"label"`
	Slug     string `json:"slug"`
	RefCount int    `json:"ref_count"`
}

type labelClusterDTO struct {
	Labels []auxiliaryLabelClusterDTO `json:"labels"`
	Size   int                        `json:"size"`
	Label  string                     `json:"label"`
}

// auxClusterEmbeddingRow is a slim projection of semantic_labels used by the
// in-memory auxiliary-label clustering.
type auxClusterEmbeddingRow struct {
	ID        uint    `gorm:"column:id"`
	Label     string  `gorm:"column:label"`
	Slug      string  `gorm:"column:slug"`
	RefCount  int     `gorm:"column:ref_count"`
	Embedding *string `gorm:"column:embedding"`
}

var (
	// auxClusterCache holds the last computed clustering. Clustering is a
	// read-only aggregate over rarely-changing embeddings, so the result is
	// reused until the TTL expires.
	auxClusterCacheMu   sync.RWMutex
	auxClusterCache     *auxClusterCacheData
	auxClusterComputeMu sync.Mutex
	auxClusterCacheTTL  = 10 * time.Minute
)

type auxClusterCacheData struct {
	clusters         []labelClusterDTO
	unclusteredCount int
	createdAt        time.Time
}

// auxClusterDistance is the cosine distance below which two auxiliary labels
// are treated as neighbors (cosine similarity > 1 - distance).
const auxClusterDistance = 0.2

func (h *semanticBoardHandler) clusterAuxiliaryLabels(c *gin.Context) {
	ctx := c.Request.Context()

	force := strings.EqualFold(c.Query("refresh"), "true") || c.Query("refresh") == "1"
	if !force {
		auxClusterCacheMu.RLock()
		entry := auxClusterCache
		auxClusterCacheMu.RUnlock()
		if entry != nil && time.Since(entry.createdAt) < auxClusterCacheTTL {
			respondOK(c, gin.H{"clusters": entry.clusters, "unclustered_count": entry.unclusteredCount})
			return
		}
	}

	// Serialize computation so concurrent requests reuse one pass instead of
	// duplicating the O(N^2) work.
	auxClusterComputeMu.Lock()
	defer auxClusterComputeMu.Unlock()
	if !force {
		auxClusterCacheMu.RLock()
		entry := auxClusterCache
		auxClusterCacheMu.RUnlock()
		if entry != nil && time.Since(entry.createdAt) < auxClusterCacheTTL {
			respondOK(c, gin.H{"clusters": entry.clusters, "unclustered_count": entry.unclusteredCount})
			return
		}
	}

	var rows []auxClusterEmbeddingRow
	if err := h.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "auxiliary", "active").
		Select("id, label, slug, ref_count, embedding").
		Order("ref_count DESC, id ASC").
		Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	if len(rows) < 2 {
		respondOK(c, gin.H{"clusters": []labelClusterDTO{}, "unclustered_count": len(rows)})
		return
	}

	// Embedding comparison is done in Go, not SQL: pgvector cannot build an ANN
	// index on vector(2560) (2000-dim limit for vector, 4000 for halfvec), so the
	// old self-join was a single-threaded O(N^2) brute force (~95 min for ~10k
	// labels). Here we L2-normalize into a flat float32 buffer and run concurrent
	// dot products; cosine distance = 1 - dot after normalization.
	dim, flat, valid := normalizeAuxEmbeddings(rows)
	clusters, unclusteredCount := clusterAuxLabels(rows, dim, flat, valid)

	auxClusterCacheMu.Lock()
	auxClusterCache = &auxClusterCacheData{
		clusters:         clusters,
		unclusteredCount: unclusteredCount,
		createdAt:        time.Now(),
	}
	auxClusterCacheMu.Unlock()

	respondOK(c, gin.H{"clusters": clusters, "unclustered_count": unclusteredCount})
}

// normalizeAuxEmbeddings parses each row's pgvector embedding, L2-normalizes
// it, and packs the result into a contiguous float32 buffer at row index i.
// The common dimension is inferred from the first parseable embedding; rows
// with a missing/malformed/mismatched-dimension embedding are marked invalid.
func normalizeAuxEmbeddings(rows []auxClusterEmbeddingRow) (dim int, flat []float32, valid []bool) {
	valid = make([]bool, len(rows))
	for _, r := range rows {
		if r.Embedding == nil || *r.Embedding == "" {
			continue
		}
		v, err := service.ParsePgVector(*r.Embedding)
		if err != nil || len(v) == 0 {
			continue
		}
		dim = len(v)
		break
	}
	if dim == 0 {
		return 0, nil, valid
	}
	flat = make([]float32, len(rows)*dim)
	for i, r := range rows {
		if r.Embedding == nil || *r.Embedding == "" {
			continue
		}
		v, err := service.ParsePgVector(*r.Embedding)
		if err != nil || len(v) != dim {
			continue
		}
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		norm := math.Sqrt(sum)
		if norm == 0 {
			continue
		}
		off := i * dim
		inv := float32(1.0 / norm)
		for k, x := range v {
			flat[off+k] = float32(x) * inv
		}
		valid[i] = true
	}
	return dim, flat, valid
}

// clusterAuxLabels builds a neighbor graph (edges with cosine distance <
// auxClusterDistance) via concurrent dot products and returns connected
// components of size >= 2, preserving the original ordering/sorting behavior.
func clusterAuxLabels(rows []auxClusterEmbeddingRow, dim int, flat []float32, valid []bool) ([]labelClusterDTO, int) {
	n := len(rows)
	adj := make([][]int, n)

	if dim > 0 {
		simThreshold := float32(1.0 - auxClusterDistance)

		type edge struct{ a, b int }
		workers := runtime.NumCPU()
		if workers < 1 {
			workers = 1
		}
		if workers > n {
			workers = n
		}
		results := make([][]edge, workers)
		var wg sync.WaitGroup
		step := (n + workers - 1) / workers
		for w := 0; w < workers; w++ {
			start := w * step
			end := start + step
			if end > n {
				end = n
			}
			if start >= end {
				continue
			}
			wg.Add(1)
			go func(start, end, wid int) {
				defer wg.Done()
				var local []edge
				for i := start; i < end; i++ {
					if !valid[i] {
						continue
					}
					ai := i * dim
					vi := flat[ai : ai+dim]
					for j := i + 1; j < n; j++ {
						if !valid[j] {
							continue
						}
						aj := j * dim
						vj := flat[aj : aj+dim]
						var dot float32
						for k := 0; k < dim; k++ {
							dot += vi[k] * vj[k]
						}
						if dot > simThreshold {
							local = append(local, edge{i, j})
						}
					}
				}
				results[wid] = local
			}(start, end, w)
		}
		wg.Wait()

		for _, local := range results {
			for _, e := range local {
				adj[e.a] = append(adj[e.a], e.b)
				adj[e.b] = append(adj[e.b], e.a)
			}
		}
	}

	visited := make([]bool, n)
	var clusters []labelClusterDTO
	for i := range rows {
		if visited[i] {
			continue
		}
		comp := []int{}
		queue := []int{i}
		visited[i] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			comp = append(comp, cur)
			for _, nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		if len(comp) < 2 {
			continue
		}

		members := make([]auxiliaryLabelClusterDTO, 0, len(comp))
		representative := ""
		maxRef := -1
		for _, idx := range comp {
			r := rows[idx]
			members = append(members, auxiliaryLabelClusterDTO{
				ID:       r.ID,
				Label:    r.Label,
				Slug:     r.Slug,
				RefCount: r.RefCount,
			})
			if r.RefCount > maxRef {
				maxRef = r.RefCount
				representative = r.Label
			}
		}
		clusters = append(clusters, labelClusterDTO{
			Labels: members,
			Size:   len(members),
			Label:  representative,
		})
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Size > clusters[j].Size
	})
	if len(clusters) > 50 {
		clusters = clusters[:50]
	}

	unclusteredCount := 0
	for i := range rows {
		if !visited[i] {
			unclusteredCount++
		}
	}

	return clusters, unclusteredCount
}

func (h *semanticBoardHandler) disableAuxiliaryLabel(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.auxiliary.DisableAuxiliaryLabel(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"id": id})
}

func (h *semanticBoardHandler) gcAuxiliaryLabels(c *gin.Context) {
	var req struct {
		Mode      string `json:"mode" binding:"required"`
		GraceDays int    `json:"grace_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "mode is required"})
		return
	}

	mode := service.AuxLabelGCMode(req.Mode)
	if mode != service.AuxLabelGCModeDryRun && mode != service.AuxLabelGCModeDisable &&
		mode != service.AuxLabelGCModeDelete && mode != service.AuxLabelGCModeRecalculate {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid mode, must be one of: dry_run, disable, delete, recalculate",
		})
		return
	}

	result, err := h.auxiliary.GC(c.Request.Context(), service.AuxLabelGCRequest{
		Mode:      mode,
		GraceDays: req.GraceDays,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *semanticBoardHandler) mergeAuxiliaryAlias(c *gin.Context) {
	var req mergeAuxiliaryAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	if err := h.auxiliary.MergeAuxiliaryLabelAlias(c.Request.Context(), req.SourceID, req.TargetID); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"source_id": req.SourceID, "target_id": req.TargetID})
}

func (h *semanticBoardHandler) getTagAuxiliaryLabels(c *gin.Context) {
	tagID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var labels []models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Model(&models.SemanticLabel{}).
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.semantic_label_id = semantic_labels.id").
		Where("topic_tag_semantic_labels.topic_tag_id = ? AND semantic_labels.label_type = ?", tagID, "auxiliary").
		Order("semantic_labels.ref_count DESC, semantic_labels.id ASC").
		Find(&labels).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]semanticBoardAuxiliaryDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, auxiliaryToDTO(label))
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) getTagSemanticBoards(c *gin.Context) {
	tagID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	type row struct {
		models.SemanticLabel
		Score       float64
		MatchReason string
	}
	var rows []row
	if err := h.db.WithContext(c.Request.Context()).Table("semantic_labels").
		Select("semantic_labels.*, topic_tag_board_labels.score, topic_tag_board_labels.match_reason").
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.semantic_board_id = semantic_labels.id").
		Where("topic_tag_board_labels.topic_tag_id = ? AND semantic_labels.label_type = ? AND semantic_labels.status = ?", tagID, "board", "active").
		Order("topic_tag_board_labels.score DESC, semantic_labels.id ASC").
		Scan(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"board": semanticBoardToDTO(row.SemanticLabel, 0), "score": row.Score, "match_reason": row.MatchReason})
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *semanticBoardHandler) loadSemanticBoards(ctx context.Context, search string, status string) ([]semanticBoardDTO, error) {
	query := h.db.WithContext(ctx).Where("label_type = ?", "board")
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	} else {
		query = query.Where("status = ?", "active")
	}
	if search = strings.TrimSpace(search); search != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(search)+"%", "%"+strings.ToLower(service.Slugify(search))+"%")
	}
	var labels []models.SemanticLabel
	if err := query.Order("display_order ASC, id ASC").Find(&labels).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(labels))
	for _, label := range labels {
		ids = append(ids, label.ID)
	}
	tagCounts, err := h.loadSemanticBoardTagCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]semanticBoardDTO, 0, len(labels))
	for _, label := range labels {
		items = append(items, semanticBoardToDTO(label, tagCounts[label.ID]))
	}
	return items, nil
}

func (h *semanticBoardHandler) loadSemanticBoardTagCounts(ctx context.Context, boardIDs []uint) (map[uint]int64, error) {
	counts := map[uint]int64{}
	if len(boardIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		SemanticBoardID uint
		Count           int64
	}
	if err := h.db.WithContext(ctx).Model(&models.TopicTagBoardLabel{}).
		Select("semantic_board_id, COUNT(*) AS count").
		Where("semantic_board_id IN ?", boardIDs).
		Group("semantic_board_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.SemanticBoardID] = row.Count
	}
	return counts, nil
}

func insertBoardComposition(tx *gorm.DB, boardID uint, auxiliaryIDs []uint) error {
	ids := service.UniqueUintSlice(auxiliaryIDs)
	if len(ids) == 0 {
		return nil
	}
	if err := service.ValidateActiveAuxiliaryLabels(tx, ids); err != nil {
		return err
	}
	rows := make([]models.BoardComposition, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: id})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func semanticBoardToDTO(label models.SemanticLabel, tagCount int64) semanticBoardDTO {
	return semanticBoardDTO{ID: label.ID, Label: label.Label, Slug: label.Slug, Aliases: label.Aliases, RefCount: label.RefCount, TagCount: tagCount, Description: label.Description, DisplayOrder: label.DisplayOrder, Source: label.Source, Status: label.Status, Protected: label.Protected, EnrichmentEnabled: label.EnrichmentEnabled, WindowDays: label.WindowDays, ContextLayers: label.ContextLayers, RelationAutoDiscoveryEnabled: label.RelationAutoDiscoveryEnabled, CreatedAt: label.CreatedAt, UpdatedAt: label.UpdatedAt}
}

func auxiliaryToDTO(label models.SemanticLabel) semanticBoardAuxiliaryDTO {
	return semanticBoardAuxiliaryDTO{ID: label.ID, Label: label.Label, Slug: label.Slug, Aliases: label.Aliases, RefCount: label.RefCount, Description: label.Description, DisplayOrder: label.DisplayOrder, Source: label.Source, Status: label.Status, Protected: label.Protected}
}

func (h *semanticBoardHandler) getAllConfigs(c *gin.Context) gin.H {
	matchConfig := semanticBoardMatchConfigToMap(service.NewSemanticBoardMatchingService(h.db).LoadConfig(c.Request.Context()))
	upgradeConfig := semanticBoardUpgradeConfigToMap(service.NewSemanticBoardUpgradeService(h.db, nil, nil).LoadUpgradeConfig(c.Request.Context()))
	merged := make(gin.H, len(matchConfig)+len(upgradeConfig))
	for k, v := range matchConfig {
		merged[k] = v
	}
	for k, v := range upgradeConfig {
		merged[k] = v
	}
	return merged
}

func isSemanticBoardConfigKey(key string) bool {
	switch key {
	case "semantic_board_match_sim_threshold", "semantic_board_match_direct_hit_rate", "semantic_board_match_direct_max_sim", "semantic_board_match_direct_max_sim_min_hits", "semantic_board_match_direct_max_sim_min_hit_rate", "semantic_board_match_min_effective_sample", "semantic_board_match_hit_rate_sim_blend", "semantic_board_match_weight_sim", "semantic_board_match_weight_density", "semantic_board_match_weighted_threshold", "semantic_board_match_max_boards", "semantic_board_match_direct_hit_min_overlap", "semantic_board_match_direction_sim_threshold",
		"semantic_board_upgrade_ref_count_threshold", "semantic_board_upgrade_cluster_distance_threshold", "semantic_board_upgrade_cotag_window_days", "semantic_board_upgrade_cotag_top_n", "semantic_board_upgrade_cotag_dedupe_sim_threshold", "semantic_board_upgrade_cotag_hard_limit", "semantic_board_upgrade_cluster_method":
		return true
	default:
		return false
	}
}

func validateSemanticBoardConfigValue(key string, value string) error {
	switch key {
	case "semantic_board_match_max_boards", "semantic_board_match_direct_max_sim_min_hits", "semantic_board_match_min_effective_sample", "semantic_board_match_direct_hit_min_overlap", "semantic_board_upgrade_ref_count_threshold", "semantic_board_upgrade_cotag_window_days", "semantic_board_upgrade_cotag_top_n", "semantic_board_upgrade_cotag_hard_limit":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
		return nil
	case "semantic_board_upgrade_cluster_method":
		if value != "average_link" && value != "centroid" {
			return fmt.Errorf("%s must be 'average_link' or 'centroid'", key)
		}
		return nil
	default:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return fmt.Errorf("%s must be a number between 0 and 1", key)
		}
		return nil
	}
}

func (h *semanticBoardHandler) suggestAuxiliaries(c *gin.Context) {
	label := strings.TrimSpace(c.Query("label"))
	if label == "" {
		respondError(c, http.StatusBadRequest, fmt.Errorf("label is required"))
		return
	}
	description := strings.TrimSpace(c.Query("description"))
	queryText := label
	if description != "" {
		queryText = label + " " + description
	}

	page, pageSize := parsePaginationParams(c)

	_, queryVector, err := semanticBoardLabelEmbedder(c.Request.Context(), queryText, service.AuxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	excludeBoardID := uint(0)
	if excludeStr := strings.TrimSpace(c.Query("exclude_board_id")); excludeStr != "" {
		excludeID, parseErr := strconv.ParseUint(excludeStr, 10, 64)
		if parseErr == nil && excludeID > 0 {
			excludeBoardID = uint(excludeID)
		}
	}

	results, err := h.computeAuxiliarySuggestions(c.Request.Context(), queryVector, c.Query("search"), excludeBoardID, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondOK(c, results)
}

func (h *semanticBoardHandler) suggestAuxiliariesForBoard(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}

	queryText := board.Label
	if board.Description != "" {
		queryText = board.Label + " " + board.Description
	}

	page, pageSize := parsePaginationParams(c)

	_, queryVector, err := semanticBoardLabelEmbedder(c.Request.Context(), queryText, service.AuxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	results, err := h.computeAuxiliarySuggestions(c.Request.Context(), queryVector, c.Query("search"), boardID, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondOK(c, results)
}

func (h *semanticBoardHandler) addBoardComposition(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req addCompositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}
	if req.AuxiliaryLabelID == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("auxiliary_label_id is required"))
		return
	}

	var board models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("semantic board not found"))
		return
	}

	// add-composite-labels：挂载单元可为 aux 或 composite（列名 auxiliary_label_id 复用）
	var auxiliary models.SemanticLabel
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND label_type IN ? AND status = ?", req.AuxiliaryLabelID, []string{"auxiliary", "composite"}, "active").First(&auxiliary).Error; err != nil {
		respondError(c, http.StatusBadRequest, fmt.Errorf("active auxiliary or composite label not found"))
		return
	}

	row := models.BoardComposition{BoardID: boardID, AuxiliaryLabelID: req.AuxiliaryLabelID}
	if err := h.db.WithContext(c.Request.Context()).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	// 匹配输入缓存（auxiliaries/composites/embeddings）无 TTL，composition 变更必须失效
	service.InvalidateMatchCache()
	respondOK(c, gin.H{"board_id": boardID, "auxiliary_label_id": req.AuxiliaryLabelID})
}

type scoredAuxiliary struct {
	label      models.SemanticLabel
	similarity float64
}

func (h *semanticBoardHandler) computeAuxiliarySuggestions(ctx context.Context, queryVector []float64, search string, excludeBoardID uint, page, pageSize int) (*suggestAuxiliariesResponse, error) {
	query := h.db.WithContext(ctx).Where("label_type = ? AND status = ?", "auxiliary", "active")
	if s := strings.TrimSpace(search); s != "" {
		query = query.Where("LOWER(label) LIKE ? OR LOWER(slug) LIKE ?", "%"+strings.ToLower(s)+"%", "%"+strings.ToLower(service.Slugify(s))+"%")
	}

	// Exclude labels already in the board's composition
	if excludeBoardID > 0 {
		query = query.Where("id NOT IN (?)", h.db.Table("board_composition").Select("auxiliary_label_id").Where("board_id = ?", excludeBoardID))
	}

	var labels []models.SemanticLabel
	if err := query.Find(&labels).Error; err != nil {
		return nil, err
	}

	scored := make([]scoredAuxiliary, 0, len(labels))
	for _, label := range labels {
		if label.Embedding == nil || *label.Embedding == "" {
			continue
		}
		vec, err := service.ParsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		sim, err := airouter.CosineSimilarity(queryVector, vec)
		if err != nil {
			continue
		}
		scored = append(scored, scoredAuxiliary{label: label, similarity: sim})
	}

	// Sort by similarity descending
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].similarity == scored[j].similarity {
			return scored[i].label.ID < scored[j].label.ID
		}
		return scored[i].similarity > scored[j].similarity
	})

	total := len(scored)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	items := make([]suggestedAuxiliaryDTO, 0, end-start)
	for i := start; i < end; i++ {
		s := scored[i]
		items = append(items, suggestedAuxiliaryDTO{
			ID:         s.label.ID,
			Label:      s.label.Label,
			Slug:       s.label.Slug,
			Aliases:    s.label.Aliases,
			RefCount:   s.label.RefCount,
			Similarity: roundSimilarity(s.similarity),
		})
	}

	return &suggestAuxiliariesResponse{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func parsePaginationParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

func roundSimilarity(v float64) float64 {
	return float64(int(v*10000)) / 10000
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || parsed == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid %s", name))
		return 0, false
	}
	return uint(parsed), true
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func respondError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"success": false, "error": err.Error()})
}
