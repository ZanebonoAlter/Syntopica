package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Repo is the package-level repository singleton.
var Repo *Repository

// Repository provides data access for data enrichment entities.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new repository with the given DB connection.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// InitRepo creates and stores the global repository singleton.
func InitRepo(db *gorm.DB) {
	Repo = NewRepository(db)
}

// DB returns the underlying *gorm.DB.
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// SetRepo replaces the global repo (for testing).
func SetRepo(r *Repository) {
	Repo = r
}

// ── BoardDataSource ──────────────────────────────────────────────────────────

// CreateBoardDataSource inserts a new board data source.
func (r *Repository) CreateBoardDataSource(ctx context.Context, ds *BoardDataSource) error {
	if err := ValidateSourceType(ds.SourceType); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(ds).Error
}

// GetBoardDataSourceByBoardAndType fetches a single data source by board + source type.
func (r *Repository) GetBoardDataSourceByBoardAndType(ctx context.Context, boardID uint, sourceType string) (*BoardDataSource, error) {
	var ds BoardDataSource
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND source_type = ?", boardID, sourceType).
		First(&ds).Error
	if err != nil {
		return nil, fmt.Errorf("get board data source: %w", err)
	}
	return &ds, nil
}

// UpsertBoardDataSource creates or updates a data source (by board_id + source_type unique key).
// Wraps read-then-write in a transaction to avoid TOCTOU race.
func (r *Repository) UpsertBoardDataSource(ctx context.Context, ds *BoardDataSource) error {
	if err := ValidateSourceType(ds.SourceType); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing BoardDataSource
		err := tx.Where("semantic_board_id = ? AND source_type = ?",
			ds.SemanticBoardID, ds.SourceType).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(ds).Error
		}
		if err != nil {
			return err
		}
		ds.ID = existing.ID
		ds.CreatedAt = existing.CreatedAt
		return tx.Save(ds).Error
	})
}

// ListBoardDataSourcesByBoardID returns all data sources for a board.
func (r *Repository) ListBoardDataSourcesByBoardID(ctx context.Context, boardID uint) ([]BoardDataSource, error) {
	var list []BoardDataSource
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ?", boardID).
		Order("source_type ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list board data sources: %w", err)
	}
	return list, nil
}

// DeleteBoardDataSource removes a data source by ID.
func (r *Repository) DeleteBoardDataSource(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&BoardDataSource{}, id).Error
}

// ── TopicLifelineContext ────────────────────────────────────────────────────

// UpsertTopicLifelineContext creates or updates a lifeline context (by topic_id + granularity + period).
// Wraps read-then-write in a transaction to avoid TOCTOU race.
func (r *Repository) UpsertTopicLifelineContext(ctx context.Context, lc *TopicLifelineContext) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing TopicLifelineContext
		err := tx.Where("persistent_topic_id = ? AND granularity = ? AND period = ?",
			lc.PersistentTopicID, lc.Granularity, lc.Period).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(lc).Error
		}
		if err != nil {
			return err
		}
		lc.ID = existing.ID
		lc.CreatedAt = existing.CreatedAt
		return tx.Save(lc).Error
	})
}

// GetTopicLifelineContext fetches a single lifeline context by topic + granularity + period.
func (r *Repository) GetTopicLifelineContext(ctx context.Context, topicID uint, granularity, period string) (*TopicLifelineContext, error) {
	var lc TopicLifelineContext
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ? AND granularity = ? AND period = ?", topicID, granularity, period).
		First(&lc).Error
	if err != nil {
		return nil, fmt.Errorf("get topic lifeline context: %w", err)
	}
	return &lc, nil
}

// GetTopicLifelineContextLatest returns the row with the highest period for a given granularity.
// Used by orchestrator readContextLayers to pick the freshest period.
func (r *Repository) GetTopicLifelineContextLatest(ctx context.Context, topicID uint, granularity string) (*TopicLifelineContext, error) {
	var lc TopicLifelineContext
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ? AND granularity = ?", topicID, granularity).
		Order("period DESC").
		First(&lc).Error
	if err != nil {
		return nil, fmt.Errorf("get latest topic lifeline context: %w", err)
	}
	return &lc, nil
}

// ListTopicLifelineContextsByTopic returns all granularity+period contexts for a topic.
func (r *Repository) ListTopicLifelineContextsByTopic(ctx context.Context, topicID uint) ([]TopicLifelineContext, error) {
	var list []TopicLifelineContext
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ?", topicID).
		Order("granularity ASC, period DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list topic lifeline contexts: %w", err)
	}
	return list, nil
}

// ListTopicLifelineContextsByGranularity returns all periods for a topic + granularity.
func (r *Repository) ListTopicLifelineContextsByGranularity(ctx context.Context, topicID uint, granularity string) ([]TopicLifelineContext, error) {
	var list []TopicLifelineContext
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ? AND granularity = ?", topicID, granularity).
		Order("period DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list lifeline contexts by granularity: %w", err)
	}
	return list, nil
}

// DeleteTopicLifelineContextsOlderThan deletes rows whose period is before the cutoff
// for a given granularity. Used by archive/prune logic.
func (r *Repository) DeleteTopicLifelineContextsOlderThan(ctx context.Context, granularity, cutoffPeriod string) error {
	return r.db.WithContext(ctx).
		Where("granularity = ? AND period < ?", granularity, cutoffPeriod).
		Delete(&TopicLifelineContext{}).Error
}

// ── TopicEnrichmentResult ───────────────────────────────────────────────────

// CreateTopicEnrichmentResult inserts a new immutable result snapshot. Empty
// kind values from pre-result-kind callers are classified compatibly by scope.
func (r *Repository) CreateTopicEnrichmentResult(ctx context.Context, result *TopicEnrichmentResult) error {
	if result.AnalysisScope == "" {
		if result.SemanticBoardID != nil {
			result.AnalysisScope = "board"
		} else {
			result.AnalysisScope = "topic"
		}
	}
	if result.ResultKind == "" {
		result.ResultKind = EffectiveResultKind(result)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateResultShape(tx, result); err != nil {
			return err
		}
		return tx.Create(result).Error
	})
}

// CreateBoardInvestigationResult classifies and validates a child investigation
// before inserting it. The parent brief remains immutable and may have many
// investigation children.
func (r *Repository) CreateBoardInvestigationResult(ctx context.Context, result *TopicEnrichmentResult) error {
	result.ResultKind = ResultKindBoardInvestigation
	return r.CreateTopicEnrichmentResult(ctx, result)
}

func validateResultShape(tx *gorm.DB, result *TopicEnrichmentResult) error {
	hasParentData := result.ParentResultID != nil || result.QuestionKey != nil
	switch result.ResultKind {
	case ResultKindTopicAnalysis:
		if result.AnalysisScope != "topic" || result.PersistentTopicID == nil || result.SemanticBoardID != nil || hasParentData {
			return fmt.Errorf("invalid %s result shape", result.ResultKind)
		}
	case ResultKindBoardBrief, ResultKindLegacyBoardAnalysis:
		if result.AnalysisScope != "board" || result.SemanticBoardID == nil || result.PersistentTopicID != nil || hasParentData {
			return fmt.Errorf("invalid %s result shape", result.ResultKind)
		}
	case ResultKindBoardInvestigation:
		if result.AnalysisScope != "board" || result.SemanticBoardID == nil || result.PersistentTopicID != nil || result.ParentResultID == nil || result.QuestionKey == nil || !IsValidQuestionKey(*result.QuestionKey) {
			return fmt.Errorf("invalid %s result shape", result.ResultKind)
		}
		var parent TopicEnrichmentResult
		err := tx.Where(
			"id = ? AND semantic_board_id = ? AND analysis_scope = ? AND result_kind = ?",
			*result.ParentResultID, *result.SemanticBoardID, "board", ResultKindBoardBrief,
		).First(&parent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("board investigation parent must be a board_brief on the same board")
		}
		if err != nil {
			return fmt.Errorf("validate board investigation parent: %w", err)
		}
	default:
		return fmt.Errorf("unknown result_kind: %s", result.ResultKind)
	}
	return nil
}

func isBoardResultKind(kind string) bool {
	switch kind {
	case ResultKindBoardBrief, ResultKindBoardInvestigation, ResultKindLegacyBoardAnalysis:
		return true
	default:
		return false
	}
}

// ListTopicEnrichmentResultsByTopic returns all results for a topic, newest first.
func (r *Repository) ListTopicEnrichmentResultsByTopic(ctx context.Context, topicID uint) ([]TopicEnrichmentResult, error) {
	var list []TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ?", topicID).
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list enrichment results: %w", err)
	}
	return list, nil
}

// GetLatestTopicEnrichmentResult returns the newest result for a topic.
func (r *Repository) GetLatestTopicEnrichmentResult(ctx context.Context, topicID uint) (*TopicEnrichmentResult, error) {
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ?", topicID).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get latest enrichment result: %w", err)
	}
	return &result, nil
}

// GetTopicEnrichmentResultByID fetches a single result by ID.
func (r *Repository) GetTopicEnrichmentResultByID(ctx context.Context, id uint) (*TopicEnrichmentResult, error) {
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).First(&result, id).Error
	if err != nil {
		return nil, fmt.Errorf("get enrichment result: %w", err)
	}
	return &result, nil
}

// GetPrevLatestTopicEnrichmentResult returns the newest result before the given ID.
func (r *Repository) GetPrevLatestTopicEnrichmentResult(ctx context.Context, topicID uint, beforeID uint) (*TopicEnrichmentResult, error) {
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ? AND id < ?", topicID, beforeID).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get prev enrichment result: %w", err)
	}
	return &result, nil
}

// ── Board-scoped enrichment results (board-level-deep-analysis) ───────────

// ListBoardEnrichmentResults returns all board-scope results for a board,
// newest first. Topic-scope rows are excluded by the scope filter — board
// reports never mix with single-lane reports even though they share the table.
func (r *Repository) ListBoardEnrichmentResults(ctx context.Context, boardID uint) ([]TopicEnrichmentResult, error) {
	var list []TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ?", boardID, "board").
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list board enrichment results: %w", err)
	}
	return list, nil
}

// GetLatestBoardEnrichmentResult returns the newest board-scope result for a board.
func (r *Repository) GetLatestBoardEnrichmentResult(ctx context.Context, boardID uint) (*TopicEnrichmentResult, error) {
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ?", boardID, "board").
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get latest board enrichment result: %w", err)
	}
	return &result, nil
}

// GetPrevLatestBoardEnrichmentResult returns the newest board-scope result
// before the given ID (review judge compares against this one).
func (r *Repository) GetPrevLatestBoardEnrichmentResult(ctx context.Context, boardID uint, beforeID uint) (*TopicEnrichmentResult, error) {
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ? AND id < ?", boardID, "board", beforeID).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get prev board enrichment result: %w", err)
	}
	return &result, nil
}

// ListBoardEnrichmentResultsByKind returns board results of one explicit kind,
// newest first. The unfiltered method above remains the compatibility API.
func (r *Repository) ListBoardEnrichmentResultsByKind(ctx context.Context, boardID uint, kind string) ([]TopicEnrichmentResult, error) {
	if !isBoardResultKind(kind) {
		return nil, fmt.Errorf("invalid board result_kind: %s", kind)
	}
	var list []TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ? AND result_kind = ?", boardID, "board", kind).
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list board enrichment results by kind: %w", err)
	}
	return list, nil
}

// GetLatestBoardEnrichmentResultByKind returns the newest result of kind.
func (r *Repository) GetLatestBoardEnrichmentResultByKind(ctx context.Context, boardID uint, kind string) (*TopicEnrichmentResult, error) {
	if !isBoardResultKind(kind) {
		return nil, fmt.Errorf("invalid board result_kind: %s", kind)
	}
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ? AND result_kind = ?", boardID, "board", kind).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get latest board enrichment result by kind: %w", err)
	}
	return &result, nil
}

// GetPrevLatestBoardEnrichmentResultByKind returns the newest same-kind result
// before beforeID, for kind-isolated review comparisons.
func (r *Repository) GetPrevLatestBoardEnrichmentResultByKind(ctx context.Context, boardID uint, kind string, beforeID uint) (*TopicEnrichmentResult, error) {
	if !isBoardResultKind(kind) {
		return nil, fmt.Errorf("invalid board result_kind: %s", kind)
	}
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ? AND result_kind = ? AND id < ?", boardID, "board", kind, beforeID).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get prev board enrichment result by kind: %w", err)
	}
	return &result, nil
}

// GetPrevBoardInvestigationByQuestion returns the newest board_investigation
// rerun of the SAME brief parent and question key before beforeID (task 4.7:
// 调查 review 只比较同 parent_result_id + question_key 链上的重跑).
// Rows of other kinds (brief/legacy/topic), other parents, other keys, other
// boards, and pre-backfill rows (question_key NULL) can never match — NULL
// never equals a non-NULL key in SQL, so legacy rows are excluded by the key
// predicate itself.
func (r *Repository) GetPrevBoardInvestigationByQuestion(ctx context.Context, boardID, parentResultID uint, questionKey string, beforeID uint) (*TopicEnrichmentResult, error) {
	if !IsValidQuestionKey(questionKey) {
		return nil, fmt.Errorf("invalid question key: %s", questionKey)
	}
	var result TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND analysis_scope = ? AND result_kind = ? AND parent_result_id = ? AND question_key = ? AND id < ?",
			boardID, "board", ResultKindBoardInvestigation, parentResultID, questionKey, beforeID).
		Order("id DESC").
		First(&result).Error
	if err != nil {
		return nil, fmt.Errorf("get prev board investigation by question: %w", err)
	}
	return &result, nil
}

// ListBoardEnrichmentResultsByParent returns all investigation children of one
// immutable brief, newest first.
func (r *Repository) ListBoardEnrichmentResultsByParent(ctx context.Context, parentResultID uint) ([]TopicEnrichmentResult, error) {
	var list []TopicEnrichmentResult
	err := r.db.WithContext(ctx).
		Where("parent_result_id = ? AND result_kind = ?", parentResultID, ResultKindBoardInvestigation).
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list board enrichment results by parent: %w", err)
	}
	return list, nil
}

// ── ReferenceRole (methodology profiles; design D5) ──────────────────────

// CreateReferenceRole inserts a new reference role.
func (r *Repository) CreateReferenceRole(ctx context.Context, role *ReferenceRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

// GetReferenceRoleByID fetches one reference role.
func (r *Repository) GetReferenceRoleByID(ctx context.Context, id uint) (*ReferenceRole, error) {
	var role ReferenceRole
	err := r.db.WithContext(ctx).First(&role, id).Error
	if err != nil {
		return nil, fmt.Errorf("get reference role: %w", err)
	}
	return &role, nil
}

// ListReferenceRoles returns all roles, newest-updated last.
func (r *Repository) ListReferenceRoles(ctx context.Context) ([]ReferenceRole, error) {
	var list []ReferenceRole
	err := r.db.WithContext(ctx).Order("updated_at DESC").Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list reference roles: %w", err)
	}
	return list, nil
}

// ListEnabledReferenceRoles returns enabled roles ordered updated_at DESC —
// the injection order (design D5: newest first when truncating to the ~4k cap).
// Queried fresh on every orchestration so enable/disable takes effect
// immediately without restart (M7.5).
func (r *Repository) ListEnabledReferenceRoles(ctx context.Context) ([]ReferenceRole, error) {
	var list []ReferenceRole
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("updated_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list enabled reference roles: %w", err)
	}
	return list, nil
}

// UpdateReferenceRole saves mutable fields (name/title/content/enabled).
func (r *Repository) UpdateReferenceRole(ctx context.Context, role *ReferenceRole) error {
	return r.db.WithContext(ctx).Save(role).Error
}

// DeleteReferenceRole removes a role permanently.
func (r *Repository) DeleteReferenceRole(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&ReferenceRole{}, id).Error
}

// ── AnalysisMethod (global method-card library; design D6) ─────────────────

func (r *Repository) CreateAnalysisMethod(ctx context.Context, method *AnalysisMethod) error {
	return r.db.WithContext(ctx).Create(method).Error
}

func (r *Repository) GetAnalysisMethodByID(ctx context.Context, id uint) (*AnalysisMethod, error) {
	var method AnalysisMethod
	if err := r.db.WithContext(ctx).First(&method, id).Error; err != nil {
		return nil, fmt.Errorf("get analysis method: %w", err)
	}
	return &method, nil
}

func (r *Repository) ListAnalysisMethods(ctx context.Context) ([]AnalysisMethod, error) {
	var list []AnalysisMethod
	if err := r.db.WithContext(ctx).Order("updated_at DESC, id DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list analysis methods: %w", err)
	}
	return list, nil
}

// ListEnabledAnalysisMethodSummaries returns selector-safe metadata without
// loading Content. Soft-deleted rows are excluded by GORM's default scope.
func (r *Repository) ListEnabledAnalysisMethodSummaries(ctx context.Context) ([]AnalysisMethod, error) {
	var list []AnalysisMethod
	if err := r.db.WithContext(ctx).
		Select("id", "name", "title", "summary", "selection_meta", "enabled", "legacy", "created_at", "updated_at").
		Where("enabled = ?", true).
		Order("updated_at DESC, id DESC").
		Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list enabled analysis method summaries: %w", err)
	}
	return list, nil
}

// GetAnalysisMethodsByIDs loads full method cards while preserving the caller's
// relevance order. Missing or soft-deleted IDs are omitted.
func (r *Repository) GetAnalysisMethodsByIDs(ctx context.Context, ids []uint) ([]AnalysisMethod, error) {
	if len(ids) == 0 {
		return []AnalysisMethod{}, nil
	}
	var rows []AnalysisMethod
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get analysis methods by ids: %w", err)
	}
	byID := make(map[uint]AnalysisMethod, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	ordered := make([]AnalysisMethod, 0, len(rows))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if row, ok := byID[id]; ok {
			ordered = append(ordered, row)
			seen[id] = true
		}
	}
	return ordered, nil
}

func (r *Repository) UpdateAnalysisMethod(ctx context.Context, method *AnalysisMethod) error {
	return r.db.WithContext(ctx).Save(method).Error
}

func (r *Repository) SetAnalysisMethodEnabled(ctx context.Context, id uint, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&AnalysisMethod{}).Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("set analysis method enabled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("set analysis method enabled: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (r *Repository) DeleteAnalysisMethod(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&AnalysisMethod{}, id)
	if res.Error != nil {
		return fmt.Errorf("delete analysis method: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("delete analysis method: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// ── TopicEnrichmentReview ───────────────────────────────────────────────────

// CreateTopicEnrichmentReview inserts a new review.
func (r *Repository) CreateTopicEnrichmentReview(ctx context.Context, review *TopicEnrichmentReview) error {
	return r.db.WithContext(ctx).Create(review).Error
}

// ListTopicEnrichmentReviewsByTopic returns all reviews for a topic, newest first.
func (r *Repository) ListTopicEnrichmentReviewsByTopic(ctx context.Context, topicID uint) ([]TopicEnrichmentReview, error) {
	var list []TopicEnrichmentReview
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ?", topicID).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list enrichment reviews: %w", err)
	}
	return list, nil
}

// ListAppliedTopicEnrichmentReviews returns reviews with applied=true for a topic.
func (r *Repository) ListAppliedTopicEnrichmentReviews(ctx context.Context, topicID uint) ([]TopicEnrichmentReview, error) {
	var list []TopicEnrichmentReview
	err := r.db.WithContext(ctx).
		Where("persistent_topic_id = ? AND applied = ?", topicID, true).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list applied reviews: %w", err)
	}
	return list, nil
}

// ListAppliedBoardEnrichmentReviews returns applied=true reviews scoped to a board.
func (r *Repository) ListAppliedBoardEnrichmentReviews(ctx context.Context, boardID uint) ([]TopicEnrichmentReview, error) {
	var list []TopicEnrichmentReview
	err := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND applied = ?", boardID, true).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list applied board reviews: %w", err)
	}
	return list, nil
}

// ListAppliedBoardEnrichmentReviewsByKind returns applied=true board reviews
// whose CURRENT result (curr_result_id) is a same-board result of the given
// kind. The result join enforces strict kind isolation (task 3.5 / design
// D11): only board_brief-chain applied reviews may feed the next brief's
// digest — legacy_board_analysis, investigation and other-board reviews never
// leak in. Ordered created_at DESC with an id DESC tie-break so same-second
// writes (bulk seeds, clock granularity) keep a deterministic newest-first
// order for the bounded digest. The unfiltered method above stays as the
// legacy-chain API.
func (r *Repository) ListAppliedBoardEnrichmentReviewsByKind(ctx context.Context, boardID uint, kind string) ([]TopicEnrichmentReview, error) {
	if !isBoardResultKind(kind) {
		return nil, fmt.Errorf("invalid board result_kind: %s", kind)
	}
	var list []TopicEnrichmentReview
	err := r.db.WithContext(ctx).
		Table("topic_enrichment_review AS r").
		Select("r.*").
		Joins("JOIN topic_enrichment_result AS res ON res.id = r.curr_result_id").
		Where("r.semantic_board_id = ? AND r.applied = ? AND res.result_kind = ? AND res.semantic_board_id = ?", boardID, true, kind, boardID).
		Order("r.created_at DESC, r.id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list applied board reviews by kind: %w", err)
	}
	return list, nil
}

// UpdateTopicEnrichmentReviewDeviation updates the deviation_summary of a review.
func (r *Repository) UpdateTopicEnrichmentReviewDeviation(ctx context.Context, id uint, summary string) error {
	return r.db.WithContext(ctx).
		Model(&TopicEnrichmentReview{}).
		Where("id = ?", id).
		Update("deviation_summary", summary).Error
}

// ApplyTopicEnrichmentReview sets applied=true on a review.
func (r *Repository) ApplyTopicEnrichmentReview(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&TopicEnrichmentReview{}).
		Where("id = ?", id).
		Update("applied", true).Error
}

// GetTopicEnrichmentReviewByID fetches a single review by ID.
func (r *Repository) GetTopicEnrichmentReviewByID(ctx context.Context, id uint) (*TopicEnrichmentReview, error) {
	var review TopicEnrichmentReview
	err := r.db.WithContext(ctx).First(&review, id).Error
	if err != nil {
		return nil, fmt.Errorf("get enrichment review: %w", err)
	}
	return &review, nil
}

// ── StockDebateResult ─────────────────────────────────────────────────────

// CreateStockDebateResult inserts a new debate result.
func (r *Repository) CreateStockDebateResult(ctx context.Context, result *StockDebateResult) error {
	return r.db.WithContext(ctx).Create(result).Error
}

// ListStockDebateResultsByResult returns all debate results for a result ID, newest first.
func (r *Repository) ListStockDebateResultsByResult(ctx context.Context, resultID uint) ([]StockDebateResult, error) {
	var list []StockDebateResult
	err := r.db.WithContext(ctx).
		Where("topic_enrichment_result_id = ?", resultID).
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list debate results: %w", err)
	}
	return list, nil
}

// ── TopicEnrichmentQA ─────────────────────────────────────────────────────

// CreateTopicEnrichmentQA inserts a new append-only Q&A round. The report
// itself is never rewritten; each round is a new row under the same result_id.
func (r *Repository) CreateTopicEnrichmentQA(ctx context.Context, qa *TopicEnrichmentQA) error {
	return r.db.WithContext(ctx).Create(qa).Error
}

// ListTopicEnrichmentQAByResultID returns all Q&A rounds for a result, oldest
// first (multi-round history order). Append-only: never mutates the report.
func (r *Repository) ListTopicEnrichmentQAByResultID(ctx context.Context, resultID uint) ([]TopicEnrichmentQA, error) {
	var list []TopicEnrichmentQA
	err := r.db.WithContext(ctx).
		Where("topic_enrichment_result_id = ?", resultID).
		Order("created_at ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list enrichment qa: %w", err)
	}
	return list, nil
}

// GetTopicEnrichmentQAByID fetches a single Q&A round by ID.
func (r *Repository) GetTopicEnrichmentQAByID(ctx context.Context, id uint) (*TopicEnrichmentQA, error) {
	var qa TopicEnrichmentQA
	err := r.db.WithContext(ctx).First(&qa, id).Error
	if err != nil {
		return nil, fmt.Errorf("get enrichment qa: %w", err)
	}
	return &qa, nil
}

// MarkQASedimented pins a Q&A round as a durable note (sedimented=true).
// Only flips the flag on the qa row — the report itself (topic_enrichment_result)
// is never rewritten (业务约束#2: result 不可变).
func (r *Repository) MarkQASedimented(ctx context.Context, qaID uint) error {
	return r.db.WithContext(ctx).
		Model(&TopicEnrichmentQA{}).
		Where("id = ?", qaID).
		Update("sedimented", true).Error
}
