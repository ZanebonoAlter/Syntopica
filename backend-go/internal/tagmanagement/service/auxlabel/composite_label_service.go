package auxlabel

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"

	"gorm.io/gorm"
)

// Composite label lifecycle (add-composite-labels).
//
// A composite label is a directed semantic unit built from 2-5 ordered active
// auxiliary labels (「美国国债 × 收益率」→「美债收益率」). It reuses semantic_labels
// with label_type="composite" plus the composite_components join table. Three
// red lines from the spec:
//   - embedding is LLM-generated from the composite phrase (label + ". " +
//     description, mirroring board embedding); NEVER synthesized from component
//     vectors — synthesis loses the directional semantics that make composites
//     worth existing (design D2);
//   - dedup is two-level: L1 unordered canonical component-ID set equality,
//     then L2 composite-embedding cosine ≥ composite_label_dedupe_sim (0.95
//     default, ai_settings-configurable). L2 only appends an alias — never
//     relabels, never recomputes embeddings (anti-blackhole discipline);
//   - disabling drops vectors in the same update (row/components/aliases stay),
//     enabling re-embeds from the phrase.

const (
	// defaultCompositeDedupeSim mirrors auxiliary_label_dedupe_sim's default.
	defaultCompositeDedupeSim = 0.95
	// CompositeMinComponents / CompositeMaxComponents bound the join table.
	CompositeMinComponents = 2
	CompositeMaxComponents = 5
)

// CompositeCreateOutcome tells the caller which path CreateCompositeLabel took.
type CompositeCreateOutcome string

const (
	// CompositeOutcomeCreated — L1/L2 both missed, a new row was inserted.
	CompositeOutcomeCreated CompositeCreateOutcome = "created"
	// CompositeOutcomeReusedL1 — canonical component set matched an existing
	// composite; ref_count incremented, nothing else changed.
	CompositeOutcomeReusedL1 CompositeCreateOutcome = "reused_l1"
	// CompositeOutcomeAliasL2 — component sets differ but composite embedding
	// cosine ≥ threshold; the new label was appended as an alias + ref_count++.
	CompositeOutcomeAliasL2 CompositeCreateOutcome = "alias_l2"
)

// CompositeLabelCreateResult is returned by CreateCompositeLabel. ReusedLabel
// points at the existing row for the L1/L2 hit paths.
type CompositeLabelCreateResult struct {
	Label   *models.SemanticLabel  `json:"label"`
	Outcome CompositeCreateOutcome `json:"outcome"`
}

// CompositeComponentView is a list-facing component entry (ordered by position).
type CompositeComponentView struct {
	LabelID  uint   `json:"label_id"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

// CompositeLabelView is a list entry with its ordered component sequence.
type CompositeLabelView struct {
	ID          uint                     `json:"id"`
	Label       string                   `json:"label"`
	Slug        string                   `json:"slug"`
	Description string                   `json:"description"`
	Source      string                   `json:"source"`
	Status      string                   `json:"status"`
	RefCount    int                      `json:"ref_count"`
	Aliases     []string                 `json:"aliases"`
	CreatedAt   string                   `json:"created_at"`
	Components  []CompositeComponentView `json:"components"`
}

type CompositeLabelService struct {
	db       *gorm.DB
	embedder AuxiliaryLabelEmbedder
}

func NewCompositeLabelService(db *gorm.DB, embedder AuxiliaryLabelEmbedder) *CompositeLabelService {
	if db == nil {
		db = repository.Repo.DB()
	}
	if embedder == nil {
		embedder = DefaultAuxiliaryLabelEmbedder
	}
	return &CompositeLabelService{db: db, embedder: embedder}
}

// CreateCompositeLabel creates (or dedup-reuses) a composite label. The
// embedder is called before any DB write so an embed failure leaves zero rows
// behind (transactional-rollback semantics without holding a tx across LLM I/O).
//
// L1 compares against all composites (active AND disabled — exact identity
// must not fork the space); a disabled hit is returned as-is, callers surface
// its status. L2 only compares active composites (disabled rows carry NULL
// vectors by the disable-drops-vectors red line).
func (s *CompositeLabelService) CreateCompositeLabel(ctx context.Context, label, description string, componentIDs []uint, source string) (*CompositeLabelCreateResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("composite label must not be empty")
	}
	if source == "" {
		source = "manual"
	}
	if source != "manual" && source != "upgrade_suggest" {
		return nil, fmt.Errorf("invalid composite label source %q, must be manual or upgrade_suggest", source)
	}

	ids, err := normalizeCompositeComponentIDs(componentIDs)
	if err != nil {
		return nil, err
	}

	// Components must be existing active auxiliary labels — an active aux row
	// is canonical by construction (the resolve path canonicalizes text to it),
	// so the component-ID set below is the canonical set (design D3).
	var components []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND label_type = ? AND status = ?", ids, "auxiliary", "active").
		Find(&components).Error; err != nil {
		return nil, fmt.Errorf("query composite components: %w", err)
	}
	if len(components) != len(ids) {
		seen := make(map[uint]bool, len(components))
		for _, c := range components {
			seen[c.ID] = true
		}
		for _, id := range ids {
			if !seen[id] {
				return nil, fmt.Errorf("component label %d is not an active auxiliary label", id)
			}
		}
		return nil, fmt.Errorf("composite components must all be active auxiliary labels")
	}

	// L1: unordered canonical component-ID set equality vs existing composites.
	existing, err := s.loadCompositeComponentSets(ctx)
	if err != nil {
		return nil, err
	}
	want := idSet(ids)
	for compositeID, set := range existing {
		if sameIDSet(set, want) {
			var row models.SemanticLabel
			if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", compositeID, "composite").First(&row).Error; err != nil {
				return nil, err
			}
			if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
				Where("id = ?", compositeID).
				UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error; err != nil {
				return nil, err
			}
			row.RefCount++
			return &CompositeLabelCreateResult{Label: &row, Outcome: CompositeOutcomeReusedL1}, nil
		}
	}

	// L2 gate: composite-embedding cosine. The embed call doubles as the
	// creation vector when both gates miss — one LLM call total.
	pgVector, vector, err := s.embedCompositePhrase(ctx, label, description)
	if err != nil {
		return nil, fmt.Errorf("generate composite embedding: %w", err)
	}

	threshold := loadCompositeDedupeSim(s.db)
	best, err := s.bestCompositeByEmbedding(ctx, vector, threshold)
	if err != nil {
		return nil, err
	}
	if best != nil {
		if err := s.addCompositeAlias(ctx, best, label); err != nil {
			return nil, err
		}
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id = ?", best.ID).
			UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error; err != nil {
			return nil, err
		}
		best.RefCount++
		return &CompositeLabelCreateResult{Label: best, Outcome: CompositeOutcomeAliasL2}, nil
	}

	// Miss on both gates → create (single tx: label row + ordered components).
	slug := core.Slugify(label)
	if slug == "" {
		return nil, fmt.Errorf("composite label slug is empty")
	}
	created := models.SemanticLabel{
		Label:     label,
		Slug:      UniqueSemanticLabelSlug(s.db.WithContext(ctx), slug),
		LabelType: "composite",
		Source:    source,
		Status:    "active",
		Embedding: &pgVector,
		Aliases:   []string{},
	}
	if desc := strings.TrimSpace(description); desc != "" {
		created.Description = desc
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		rows := make([]models.CompositeComponent, 0, len(ids))
		for i, id := range ids {
			rows = append(rows, models.CompositeComponent{
				CompositeID:      created.ID,
				ComponentLabelID: id,
				Position:         i + 1,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	return &CompositeLabelCreateResult{Label: &created, Outcome: CompositeOutcomeCreated}, nil
}

// DisableCompositeLabel flips status to disabled and drops both vectors in the
// same update (disable-drops-vectors red line). Row, components and aliases
// are preserved; matching input loaders skip disabled composites.
func (s *CompositeLabelService) DisableCompositeLabel(ctx context.Context, id uint) error {
	if id == 0 {
		return fmt.Errorf("composite label id is required")
	}
	res := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("id = ? AND label_type = ?", id, "composite").
		Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// EnableCompositeLabel re-activates a disabled composite by regenerating its
// embedding from the phrase (label + ". " + description). Embed failure leaves
// the row disabled — the vector red line means a disabled row must never be
// re-activated without a fresh vector.
func (s *CompositeLabelService) EnableCompositeLabel(ctx context.Context, id uint) error {
	if id == 0 {
		return fmt.Errorf("composite label id is required")
	}
	var row models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", id, "composite").First(&row).Error; err != nil {
		return err
	}
	pgVector, _, err := s.embedCompositePhrase(ctx, row.Label, row.Description)
	if err != nil {
		return fmt.Errorf("regenerate composite embedding: %w", err)
	}
	return s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("id = ? AND label_type = ? AND status = ?", id, "composite", "disabled").
		Updates(map[string]any{"status": "active", "embedding": pgVector}).Error
}

// ListCompositeLabels returns composites with their ordered component
// sequences. status filters when non-empty ("active"/"disabled").
func (s *CompositeLabelService) ListCompositeLabels(ctx context.Context, status string) ([]CompositeLabelView, error) {
	query := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ?", "composite")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var rows []models.SemanticLabel
	if err := query.Order("ref_count DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []CompositeLabelView{}, nil
	}

	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	var comps []models.CompositeComponent
	if err := s.db.WithContext(ctx).Where("composite_id IN ?", ids).Order("composite_id, position").Find(&comps).Error; err != nil {
		return nil, err
	}
	var auxLabels []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Select("id, label").
		Where("id IN (SELECT component_label_id FROM composite_components WHERE composite_id IN ?)", ids).
		Find(&auxLabels).Error; err != nil {
		return nil, err
	}
	labelByID := make(map[uint]string, len(auxLabels))
	for _, a := range auxLabels {
		labelByID[a.ID] = a.Label
	}
	compsByComposite := make(map[uint][]CompositeComponentView, len(rows))
	for _, c := range comps {
		compsByComposite[c.CompositeID] = append(compsByComposite[c.CompositeID], CompositeComponentView{
			LabelID:  c.ComponentLabelID,
			Label:    labelByID[c.ComponentLabelID],
			Position: c.Position,
		})
	}

	views := make([]CompositeLabelView, 0, len(rows))
	for _, r := range rows {
		views = append(views, CompositeLabelView{
			ID:          r.ID,
			Label:       r.Label,
			Slug:        r.Slug,
			Description: r.Description,
			Source:      r.Source,
			Status:      r.Status,
			RefCount:    r.RefCount,
			Aliases:     r.Aliases,
			CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04:05"),
			Components:  compsByComposite[r.ID],
		})
	}
	return views, nil
}

// ComponentOptionView 是手动创建对话框的组件候选（design D7：推荐排序）。
type ComponentOptionView struct {
	ID            uint              `json:"id"`
	Label         string            `json:"label"`
	RefCount      int               `json:"ref_count"`
	BoardCount    int               `json:"board_count"`
	InBoard       bool              `json:"in_board"`
	Cooccurrence  int               `json:"cooccurrence"`
	MountedBoards []MountedBoardRef `json:"mounted_boards"`
}

// MountedBoardRef 携带挂载版块名，供前端展示推荐信号。
type MountedBoardRef struct {
	ID    uint   `json:"id"`
	Label string `json:"label"`
}

// ListComponentOptions 返回 active aux 候选，按推荐度排序（design D7）：
// relatedAuxID>0（联动模式）：与该组件同 tag 共现频次最高者优先（选中组件后
// 候选实时重排的联动信号）→ 版块维度 → 挂载数 → ref_count；
// boardID>0（版块上下文）：该版块已挂载的 aux 置顶（in_board）。
// limit<=0 时默认 50。
func (s *CompositeLabelService) ListComponentOptions(ctx context.Context, limit int, boardID uint, relatedAuxID uint) ([]ComponentOptionView, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]struct {
		ID           uint
		Label        string
		RefCount     int
		BoardCount   int
		InBoard      bool
		Cooccurrence int
	}, 0, limit)

	inBoardExpr := "FALSE"
	if boardID > 0 {
		// 数字类型直接内插（uint 无注入面），避免 GORM Select 参数化限制
		inBoardExpr = fmt.Sprintf("EXISTS(SELECT 1 FROM board_composition bc2 WHERE bc2.board_id = %d AND bc2.auxiliary_label_id = semantic_labels.id)", boardID)
	}
	selectSQL := fmt.Sprintf("semantic_labels.id, semantic_labels.label, semantic_labels.ref_count, COUNT(boards.id) AS board_count, %s AS in_board", inBoardExpr)
	coocJoin := ""
	orderBy := "in_board DESC, board_count DESC, semantic_labels.ref_count DESC, semantic_labels.label ASC"
	if relatedAuxID > 0 {
		selectSQL += ", MAX(COALESCE(co.cooc, 0)) AS cooccurrence"
		coocJoin = fmt.Sprintf("LEFT JOIN (SELECT t2.semantic_label_id AS cand, COUNT(*) AS cooc FROM topic_tag_semantic_labels t1 JOIN topic_tag_semantic_labels t2 ON t1.topic_tag_id = t2.topic_tag_id AND t2.semantic_label_id != t1.semantic_label_id WHERE t1.semantic_label_id = %d GROUP BY t2.semantic_label_id) co ON co.cand = semantic_labels.id", relatedAuxID)
		orderBy = "cooccurrence DESC, " + orderBy
	} else {
		selectSQL += ", 0 AS cooccurrence"
	}

	if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select(selectSQL).
		Joins("LEFT JOIN board_composition ON board_composition.auxiliary_label_id = semantic_labels.id").
		Joins("LEFT JOIN semantic_labels AS boards ON boards.id = board_composition.board_id AND boards.status = 'active' AND boards.label_type = 'board'").
		Joins(coocJoin).
		Where("semantic_labels.label_type = ? AND semantic_labels.status = ?", "auxiliary", "active").
		Group("semantic_labels.id, semantic_labels.label, semantic_labels.ref_count").
		Order(orderBy).
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ComponentOptionView{}, nil
	}

	// 二次查询挂载版块名（仅 board_count>0 的候选）
	boardIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		if r.BoardCount > 0 {
			boardIDs = append(boardIDs, r.ID)
		}
	}
	mountedByAux := make(map[uint][]MountedBoardRef, len(boardIDs))
	if len(boardIDs) > 0 {
		var mounts []struct {
			AuxID      uint
			BoardID    uint
			BoardLabel string
		}
		if err := s.db.WithContext(ctx).Model(&models.BoardComposition{}).
			Select("board_composition.auxiliary_label_id AS aux_id, board_composition.board_id AS board_id, boards.label AS board_label").
			Joins("JOIN semantic_labels AS boards ON boards.id = board_composition.board_id AND boards.status = 'active' AND boards.label_type = 'board'").
			Where("board_composition.auxiliary_label_id IN ?", boardIDs).
			Order("board_composition.auxiliary_label_id, boards.label").
			Scan(&mounts).Error; err != nil {
			return nil, err
		}
		for _, m := range mounts {
			mountedByAux[m.AuxID] = append(mountedByAux[m.AuxID], MountedBoardRef{ID: m.BoardID, Label: m.BoardLabel})
		}
	}

	views := make([]ComponentOptionView, 0, len(rows))
	for _, r := range rows {
		views = append(views, ComponentOptionView{
			ID:            r.ID,
			Label:         r.Label,
			RefCount:      r.RefCount,
			BoardCount:    r.BoardCount,
			InBoard:       r.InBoard,
			Cooccurrence:  r.Cooccurrence,
			MountedBoards: mountedByAux[r.ID],
		})
	}
	return views, nil
}

// embedCompositePhrase builds the LLM embedding input the same way board
// embedding does: label alone, or label + ". " + description when present.
func (s *CompositeLabelService) embedCompositePhrase(ctx context.Context, label, description string) (string, []float64, error) {
	input := strings.TrimSpace(label)
	if desc := strings.TrimSpace(description); desc != "" {
		input = input + ". " + desc
	}
	return s.embedder(ctx, input, AuxiliaryLabelEmbeddingModeStorage)
}

// loadCompositeComponentSets returns the component-ID set of every composite
// (any status) keyed by composite ID.
func (s *CompositeLabelService) loadCompositeComponentSets(ctx context.Context) (map[uint]map[uint]struct{}, error) {
	var rows []models.CompositeComponent
	if err := s.db.WithContext(ctx).
		Joins("JOIN semantic_labels sl ON sl.id = composite_components.composite_id AND sl.label_type = 'composite'").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query composite component sets: %w", err)
	}
	result := make(map[uint]map[uint]struct{}, len(rows))
	for _, r := range rows {
		set, ok := result[r.CompositeID]
		if !ok {
			set = map[uint]struct{}{}
			result[r.CompositeID] = set
		}
		set[r.ComponentLabelID] = struct{}{}
	}
	return result, nil
}

// bestCompositeByEmbedding finds the best active composite whose embedding
// cosine ≥ threshold against vector. Ties break by higher ref_count, then
// smaller ID (mirroring the aux L2 matcher).
func (s *CompositeLabelService) bestCompositeByEmbedding(ctx context.Context, vector []float64, threshold float64) (*models.SemanticLabel, error) {
	var rows []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Select("id, label, slug, label_type, aliases, ref_count, description, status, source, embedding").
		Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "composite", "active").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query composite embeddings: %w", err)
	}
	var best *models.SemanticLabel
	for i := range rows {
		if rows[i].Embedding == nil || *rows[i].Embedding == "" {
			continue
		}
		existing, err := ParsePgVector(*rows[i].Embedding)
		if err != nil {
			continue
		}
		sim, err := airouter.CosineSimilarity(vector, existing)
		if err != nil || sim < threshold {
			continue
		}
		candidate := rows[i]
		if best == nil || candidate.RefCount > best.RefCount || (candidate.RefCount == best.RefCount && candidate.ID < best.ID) {
			best = &candidate
		}
	}
	return best, nil
}

// addCompositeAlias appends alias when absent (idempotent) without touching
// label or embeddings. Uses Save so the Aliases jsonb serializer applies
// (UpdateColumn with a raw slice bypasses it and writes invalid json).
func (s *CompositeLabelService) addCompositeAlias(ctx context.Context, label *models.SemanticLabel, alias string) error {
	if semanticAliasesContain(label.Aliases, alias) || strings.EqualFold(label.Label, alias) {
		return nil
	}
	label.Aliases = append(label.Aliases, alias)
	return s.db.WithContext(ctx).Save(label).Error
}

// loadCompositeDedupeSim reads ai_settings.composite_label_dedupe_sim,
// falling back to defaultCompositeDedupeSim on missing/invalid values.
func loadCompositeDedupeSim(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", "composite_label_dedupe_sim").First(&setting).Error; err != nil {
		return defaultCompositeDedupeSim
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(setting.Value), 64)
	if err != nil || v <= 0 || v > 1 {
		return defaultCompositeDedupeSim
	}
	return v
}

// normalizeCompositeComponentIDs deduplicates while preserving first-seen
// order (position semantics), then enforces the 2-5 window.
func normalizeCompositeComponentIDs(ids []uint) ([]uint, error) {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			return nil, fmt.Errorf("component label id must not be zero")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) < CompositeMinComponents || len(out) > CompositeMaxComponents {
		return nil, fmt.Errorf("composite label requires %d-%d distinct components, got %d", CompositeMinComponents, CompositeMaxComponents, len(out))
	}
	return out, nil
}

func idSet(ids []uint) map[uint]struct{} {
	set := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func sameIDSet(a, b map[uint]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}
