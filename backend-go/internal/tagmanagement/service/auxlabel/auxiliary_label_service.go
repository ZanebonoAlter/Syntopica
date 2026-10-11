package auxlabel

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/tagmanagement/repository"
)

type AuxLabelGCMode string

const (
	AuxLabelGCModeDryRun      AuxLabelGCMode = "dry_run"
	AuxLabelGCModeDisable     AuxLabelGCMode = "disable"
	AuxLabelGCModeDelete      AuxLabelGCMode = "delete"
	AuxLabelGCModeRecalculate AuxLabelGCMode = "recalculate"
)

type AuxLabelGCRequest struct {
	Mode      AuxLabelGCMode `json:"mode"`
	GraceDays int            `json:"grace_days"`
}

type AuxLabelGCResult struct {
	EligibleCount  int                     `json:"eligible_count"`
	AffectedCount  int                     `json:"affected_count"`
	CorrectedCount int                     `json:"corrected_count,omitempty"`
	TotalCount     int                     `json:"total_count,omitempty"`
	Preview        []AuxLabelGCPreviewItem `json:"preview,omitempty"`
}

type AuxLabelGCPreviewItem struct {
	ID        uint   `json:"id"`
	Label     string `json:"label"`
	RefCount  int    `json:"ref_count"`
	CreatedAt string `json:"created_at"`
}

// defaultAuxLabelDedupeSim is the fallback L2 merge similarity threshold when
// ai_settings.auxiliary_label_dedupe_sim is missing or invalid. The effective
// threshold is loaded per L2 evaluation by loadAuxLabelDedupeSim (design D5).
const defaultAuxLabelDedupeSim = 0.95

var ensureVectorDimOnce sync.Once

type AuxiliaryLabelEmbeddingMode string

const (
	AuxiliaryLabelEmbeddingModeMerge   AuxiliaryLabelEmbeddingMode = "merge"
	AuxiliaryLabelEmbeddingModeStorage AuxiliaryLabelEmbeddingMode = "storage"
)

type AuxiliaryLabelEmbedder func(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error)

// AuxiliaryLabelBatchEmbedder embeds a batch of inputs in one call per mode.
// Returns two parallel slices: pgvector strings and raw float vectors, both
// indexed like inputs. Backed by airouter.Embed (whose Input is []string —
// the batch capability is native); kept as a distinct type so tests can mock
// and count batch calls (design D1 phase one).
type AuxiliaryLabelBatchEmbedder func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error)

type mergeMatcherFunc func(ctx context.Context, db *gorm.DB, labels []models.SemanticLabel, mergePgVector string, mergeVector []float64) (*models.SemanticLabel, error)

const (
	auxActiveLabelsTTL    = 5 * time.Minute
	auxMergeEmbeddingsTTL = 10 * time.Minute
)

type auxLabelCache struct {
	mu sync.RWMutex

	// Active auxiliary labels cache
	activeLabels   []models.SemanticLabel
	activeLabelsAt time.Time

	// Merge embedding vectors cache
	mergeEmbeddings   map[uint][]float64
	mergeEmbeddingsAt time.Time
}

type AuxiliaryLabelService struct {
	db            *gorm.DB
	embedder      AuxiliaryLabelEmbedder
	batchEmbedder AuxiliaryLabelBatchEmbedder
	mergeMatcher  mergeMatcherFunc
	cache         *auxLabelCache
}

// package-level cache for AuxiliaryLabelService — shared across instances
var packageAuxLabelCache = &auxLabelCache{}

// InvalidateAuxLabelCache clears the package-level auxiliary label cache.
// Tests that mutate semantic_labels (or whose ResetTestData truncates tables)
// must call this so stale cached labels do not span test boundaries —
// ResetTestData clears the DB but not this in-memory cache.
func InvalidateAuxLabelCache() {
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.activeLabels = nil
	packageAuxLabelCache.mergeEmbeddings = nil
	packageAuxLabelCache.activeLabelsAt = time.Time{}
	packageAuxLabelCache.mergeEmbeddingsAt = time.Time{}
	packageAuxLabelCache.mu.Unlock()
}

func NewAuxiliaryLabelService(db *gorm.DB, embedder AuxiliaryLabelEmbedder) *AuxiliaryLabelService {
	if db == nil {
		db = repository.Repo.DB()
	}
	if embedder == nil {
		// Production path: single + batch units both hit airouter (true batching).
		return &AuxiliaryLabelService{
			db: db, embedder: DefaultAuxiliaryLabelEmbedder, batchEmbedder: DefaultAuxiliaryLabelBatchEmbedder,
			mergeMatcher: sqlMergeMatcher, cache: packageAuxLabelCache,
		}
	}
	// Custom (usually mock) single embedder: wrap it sequentially so tests that
	// only need deterministic vectors keep working without touching the network.
	return &AuxiliaryLabelService{
		db: db, embedder: embedder, batchEmbedder: batchFromSingleEmbedder(embedder),
		mergeMatcher: sqlMergeMatcher, cache: packageAuxLabelCache,
	}
}

// SetBatchEmbedder overrides the batch embedding unit (test seam — the default
// for custom single embedders loops the single one per input, which is correct
// but not observable as a batch; inject a batch mock to assert call counts).
func (s *AuxiliaryLabelService) SetBatchEmbedder(batch AuxiliaryLabelBatchEmbedder) {
	if batch != nil {
		s.batchEmbedder = batch
	}
}

// vectorDimEnsurers holds callbacks registered by other packages to ensure their
// vector columns have the correct dimension. Called once at startup after the
// embedding dimension is determined.
var vectorDimEnsurers []func(dim int)

// RegisterVectorDimEnsurer registers a callback to be invoked at startup with the
// detected embedding dimension. Call from init() in domain packages that own vector columns.
func RegisterVectorDimEnsurer(fn func(dim int)) {
	vectorDimEnsurers = append(vectorDimEnsurers, fn)
}

// EnsureVectorDimensionOnce ensures all vector column dimensions match the embedder output.
// Called once at startup. Uses the global DB to avoid calling DDL inside a transaction.
func EnsureVectorDimensionOnce(ctx context.Context) {
	ensureVectorDimOnce.Do(func() {
		_, vector, err := DefaultAuxiliaryLabelEmbedder(ctx, "dimension-check", AuxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			logging.Warnf("Failed to determine embedding dimension: %v", err)
			return
		}
		dim := len(vector)
		if err := EnsureSemanticLabelVectorDimension(dim); err != nil {
			logging.Errorf("Failed to ensure semantic_labels.embedding vector dimension: %v; "+
				"INSERT into semantic_labels will fail until this is resolved", err)
		}
		if err := EnsureSemanticLabelMergeVectorDimension(dim); err != nil {
			logging.Errorf("Failed to ensure semantic_labels.merge_embedding vector dimension: %v; "+
				"INSERT into semantic_labels will fail until this is resolved", err)
		}
		for _, fn := range vectorDimEnsurers {
			fn(dim)
		}
	})
}

func (s *AuxiliaryLabelService) AttachAuxiliaryLabels(ctx context.Context, topicTagID uint, labels []core.AuxiliaryLabel) error {
	if topicTagID == 0 || len(labels) == 0 {
		return nil
	}
	// 两阶段（design D1）：阶段一在事务外完成全部慢 I/O（缓存加载、批量
	// embedding LLM 调用、L1/L2 匹配决策）；阶段二是毫秒级纯写短事务。历史实现
	// 在单事务内逐标签调外部 AI API，是 idle in transaction 4-40s 的元凶
	// （事务持 xmin 阻塞 vacuum）。全有或全无原子性由阶段二保留。
	plans, err := s.planAuxiliaryLabels(ctx, labels)
	if err != nil {
		return err
	}
	return s.executeAuxiliaryPlans(ctx, topicTagID, plans)
}

// auxPlanKind enumerates the three resolve outcomes of phase one.
type auxPlanKind int

const (
	auxPlanInvalid auxPlanKind = iota // defensive: unfilled placeholder
	auxPlanReuse                      // L1 hit or batch-internal duplicate → link only
	auxPlanAlias                      // L2 hit → append alias + link
	auxPlanCreate                     // L3 miss → INSERT + link
)

// auxLabelPlan is one label's phase-one decision. Only the fields for its
// kind are populated; refIndex is set for batch-internal duplicates (same
// slug/normalize-key as an earlier entry — the legacy sequential loop saw
// those via cache-reload L1 re-matching, the maps replicate it).
type auxLabelPlan struct {
	kind        auxPlanKind
	refIndex    int
	existing    *models.SemanticLabel // reuse: matched row copy
	target      *models.SemanticLabel // alias: L2 best match copy
	alias       string                // alias: incoming label text
	aliasAdded  bool                  // alias: set at execution for cache step
	created     *models.SemanticLabel // create: fully populated, ID zero until phase two
	mergeVector []float64             // create: merge vector for incremental cache absorb
}

// planAuxiliaryLabels is phase one of the two-phase attach (design D1):
// validation, L1 in-memory matching, one batch merge-embedding call, L2
// cosine decisions and one batch storage-embedding call — all OUTSIDE any
// transaction. N new labels cost exactly 2 batch embedder calls.
func (s *AuxiliaryLabelService) planAuxiliaryLabels(ctx context.Context, labels []core.AuxiliaryLabel) ([]auxLabelPlan, error) {
	gateStart := time.Now()
	plans := make([]auxLabelPlan, len(labels))

	active, err := s.loadActiveAuxiliaryLabels(ctx)
	if err != nil {
		return nil, err
	}

	type missEntry struct {
		label, description, slug, normKey string
		planIdx                           int
	}
	var misses []missEntry
	seenSlug := map[string]int{}
	seenNorm := map[string]int{}

	for i := range labels {
		item := labels[i]
		label := strings.TrimSpace(item.Label)
		if label == "" {
			return nil, fmt.Errorf("auxiliary label must not be empty")
		}
		if _, generic := core.GenericAuxiliaryLabels[label]; generic {
			return nil, fmt.Errorf("auxiliary label %q is too generic", label)
		}
		slug := core.Slugify(label)
		if slug == "" {
			return nil, fmt.Errorf("auxiliary label slug is empty")
		}
		normKey := textutil.NormalizeLabelKey(label)

		// Batch-internal dedupe (slug then normalize key): identical/equivalent
		// texts in one attach share the earlier plan's outcome.
		if idx, dup := seenSlug[slug]; dup {
			plans[i] = auxLabelPlan{kind: auxPlanReuse, refIndex: idx}
			continue
		}
		if idx, dup := seenNorm[normKey]; dup {
			plans[i] = auxLabelPlan{kind: auxPlanReuse, refIndex: idx}
			continue
		}
		seenSlug[slug] = i
		seenNorm[normKey] = i

		// L1: exact match by slug, alias, or whitespace-insensitive normalize key.
		for j := range active {
			if active[j].Slug == slug || semanticAliasesContain(active[j].Aliases, label) ||
				textutil.NormalizeLabelKey(active[j].Label) == normKey {
				row := active[j] // heap copy — never hand out cache-owned storage
				plans[i] = auxLabelPlan{kind: auxPlanReuse, existing: &row}
				break
			}
		}
		if plans[i].kind == auxPlanReuse {
			continue
		}
		misses = append(misses, missEntry{
			label: label, description: strings.TrimSpace(item.Description),
			slug: slug, normKey: normKey, planIdx: i,
		})
	}

	if len(misses) > 0 {
		// L2: one batch merge embedding for every L1 miss.
		inputs := make([]string, len(misses))
		for i, m := range misses {
			inputs[i] = m.label
		}
		mergePgVectors, mergeVectors, err := s.batchEmbedder(ctx, inputs, AuxiliaryLabelEmbeddingModeMerge)
		if err != nil {
			return nil, err
		}
		if len(mergePgVectors) != len(misses) || len(mergeVectors) != len(misses) {
			return nil, fmt.Errorf("batch merge embedder returned %d vectors for %d inputs", len(mergeVectors), len(misses))
		}

		var l3Misses []int
		for i, m := range misses {
			best, err := s.mergeMatcher(ctx, s.db, active, mergePgVectors[i], mergeVectors[i])
			if err != nil {
				return nil, err
			}
			if best != nil {
				plans[m.planIdx] = auxLabelPlan{kind: auxPlanAlias, target: best, alias: m.label}
				continue
			}
			l3Misses = append(l3Misses, i)
		}

		// L3: one batch storage embedding ("label: description") for every L2 miss;
		// the merge vector from above is reused as the row's merge_embedding.
		if len(l3Misses) > 0 {
			storageInputs := make([]string, len(l3Misses))
			for j, i := range l3Misses {
				if misses[i].description != "" {
					storageInputs[j] = misses[i].label + ": " + misses[i].description
				} else {
					storageInputs[j] = misses[i].label
				}
			}
			storagePgVectors, _, err := s.batchEmbedder(ctx, storageInputs, AuxiliaryLabelEmbeddingModeStorage)
			if err != nil {
				return nil, err
			}
			if len(storagePgVectors) != len(l3Misses) {
				return nil, fmt.Errorf("batch storage embedder returned %d vectors for %d inputs", len(storagePgVectors), len(l3Misses))
			}
			for j, i := range l3Misses {
				m := misses[i]
				created := &models.SemanticLabel{
					Label:          m.label,
					Slug:           UniqueSemanticLabelSlug(s.db.WithContext(ctx), m.slug),
					LabelType:      "auxiliary",
					Source:         "llm_extract",
					Status:         "active",
					Embedding:      &storagePgVectors[j],
					MergeEmbedding: &mergePgVectors[i],
				}
				if m.description != "" {
					created.Description = m.description
				}
				plans[m.planIdx] = auxLabelPlan{kind: auxPlanCreate, created: created, mergeVector: mergeVectors[i]}
			}
		}
	}

	for i := range plans {
		if plans[i].kind == auxPlanInvalid {
			return nil, fmt.Errorf("internal error: auxiliary plan %d left unresolved", i)
		}
	}
	if elapsed := time.Since(gateStart); elapsed > time.Duration(50*len(labels))*time.Millisecond {
		logging.Warnf("auxiliary label dedup gate slow: %d labels took=%s (>50ms/label), review L1/L2 (embedder/merge) performance", len(labels), elapsed)
	}
	return plans, nil
}

// executeAuxiliaryPlans is phase two (design D1): a short pure-write
// transaction — alias UPDATE, L3 INSERT, link OnConflict create, ref_count
// bump. No embedder or cache load runs inside. All-or-nothing atomicity is
// preserved: any write failure rolls back the whole attach.
func (s *AuxiliaryLabelService) executeAuxiliaryPlans(ctx context.Context, topicTagID uint, plans []auxLabelPlan) error {
	resolved := make([]uint, len(plans))
	// Two plans may L2-match the same target with different aliases; writes
	// accumulate per target ID so the later UPDATE never clobbers the earlier.
	aliasFinal := map[uint][]string{}
	aliasChanged := false

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range plans {
			p := &plans[i]
			switch p.kind {
			case auxPlanReuse:
				if p.existing != nil {
					resolved[i] = p.existing.ID
				} else {
					resolved[i] = resolved[p.refIndex]
				}
			case auxPlanAlias:
				aliases, ok := aliasFinal[p.target.ID]
				if !ok {
					aliases = p.target.Aliases
				}
				if !semanticAliasesContain(aliases, p.alias) && !strings.EqualFold(p.target.Label, p.alias) {
					aliases = append(aliases, p.alias)
					aliasFinal[p.target.ID] = aliases
					p.target.Aliases = aliases
					p.aliasAdded = true
					aliasChanged = true
					if err := tx.Model(&models.SemanticLabel{}).
						Where("id = ?", p.target.ID).
						Select("aliases").
						Updates(models.SemanticLabel{Aliases: aliases}).Error; err != nil {
						return err
					}
				} else {
					aliasFinal[p.target.ID] = aliases
				}
				resolved[i] = p.target.ID
			case auxPlanCreate:
				if err := tx.Create(p.created).Error; err != nil {
					return err
				}
				resolved[i] = p.created.ID
			default:
				return fmt.Errorf("internal error: corrupt auxiliary plan kind %d", p.kind)
			}

			link := tagmodels.TopicTagSemanticLabel{TopicTagID: topicTagID, SemanticLabelID: resolved[i]}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", resolved[i]).UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Post-commit cache absorb (design D2): a rolled-back attach must never
	// poison the cache with rows that do not exist.
	for i := range plans {
		if plans[i].kind == auxPlanCreate {
			s.cache.applyCreate(plans[i].created, plans[i].mergeVector)
		}
	}
	if aliasChanged {
		s.invalidateOnAliasChange()
	}
	return nil
}

func (s *AuxiliaryLabelService) ResolveAuxiliaryLabel(ctx context.Context, rawLabel, description string) (*models.SemanticLabel, error) {
	plans, err := s.planAuxiliaryLabels(ctx, []core.AuxiliaryLabel{{Label: rawLabel, Description: description}})
	if err != nil {
		return nil, err
	}
	plan := &plans[0]
	switch plan.kind {
	case auxPlanReuse:
		return plan.existing, nil
	case auxPlanAlias:
		// 单条路径无批内累积需求，但保持与批量一致的列级更新写法。
		if _, err := applyAuxAliasWrite(s.db.WithContext(ctx), plan.target, plan.alias); err != nil {
			return nil, err
		}
		s.invalidateOnAliasChange()
		return plan.target, nil
	case auxPlanCreate:
		if err := s.db.WithContext(ctx).Create(plan.created).Error; err != nil {
			return nil, err
		}
		s.cache.applyCreate(plan.created, plan.mergeVector)
		return plan.created, nil
	default:
		return nil, fmt.Errorf("internal error: corrupt auxiliary plan kind %d", plan.kind)
	}
}

// applyAuxAliasWrite appends alias (when absent) via the column-level update —
// only the aliases column is written; the label row may come from the
// vector-less cache projection where a full Save would NULL the vectors
// (fix-alias-save-vector-wipe). Mirrors the appended alias into target.
func applyAuxAliasWrite(db *gorm.DB, target *models.SemanticLabel, alias string) (bool, error) {
	if semanticAliasesContain(target.Aliases, alias) || strings.EqualFold(target.Label, alias) {
		return false, nil
	}
	target.Aliases = append(target.Aliases, alias)
	if err := db.Model(&models.SemanticLabel{}).
		Where("id = ?", target.ID).
		Select("aliases").
		Updates(models.SemanticLabel{Aliases: target.Aliases}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (s *AuxiliaryLabelService) DisableAuxiliaryLabel(ctx context.Context, labelID uint) error {
	if labelID == 0 {
		return fmt.Errorf("auxiliary label id is required")
	}

	var label models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", labelID, "auxiliary").First(&label).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&label).Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil}).Error; err != nil {
		return err
	}
	// 增量失效（design D2）：只删本行条目，不再全清触发 216MB 重拉。
	s.cache.applyDisable(labelID)
	return nil
}

func (s *AuxiliaryLabelService) MergeAuxiliaryLabelAlias(ctx context.Context, sourceID uint, targetID uint) error {
	if sourceID == 0 || targetID == 0 {
		return fmt.Errorf("source and target auxiliary label ids are required")
	}
	if sourceID == targetID {
		return fmt.Errorf("source and target auxiliary label ids must be different")
	}

	var target models.SemanticLabel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source models.SemanticLabel
		if err := tx.Where("id = ? AND label_type = ?", sourceID, "auxiliary").First(&source).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND label_type = ?", targetID, "auxiliary").First(&target).Error; err != nil {
			return err
		}

		for _, alias := range append([]string{source.Label}, source.Aliases...) {
			if !strings.EqualFold(target.Label, alias) && !semanticAliasesContain(target.Aliases, alias) {
				target.Aliases = append(target.Aliases, alias)
			}
		}
		// 列级更新只写 aliases 列：target 虽为全列加载、全量 Save 当前恰好安全，但这是
		// 巧合式安全——任何后续改用缓存结构体（loadActiveAuxiliaryLabels 刻意不加载
		// 向量列）就会复发 addAlias 同款清向量 bug。统一列级更新把「alias 合并不碰
		// 向量列」变成路径无关的硬事实（fix-alias-save-vector-wipe）。
		if err := tx.Model(&models.SemanticLabel{}).
			Where("id = ?", target.ID).
			Select("aliases").
			Updates(models.SemanticLabel{Aliases: target.Aliases}).Error; err != nil {
			return err
		}

		var links []tagmodels.TopicTagSemanticLabel
		if err := tx.Where("semantic_label_id = ?", sourceID).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			migrated := tagmodels.TopicTagSemanticLabel{TopicTagID: link.TopicTagID, SemanticLabelID: targetID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrated).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("semantic_label_id = ?", sourceID).Delete(&tagmodels.TopicTagSemanticLabel{}).Error; err != nil {
			return err
		}

		var targetRefCount int64
		if err := tx.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", targetID).Count(&targetRefCount).Error; err != nil {
			return err
		}
		var sourceRefCount int64
		if err := tx.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", sourceID).Count(&sourceRefCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", targetID).Update("ref_count", int(targetRefCount)).Error; err != nil {
			return err
		}
		return tx.Model(&models.SemanticLabel{}).Where("id = ?", sourceID).Updates(map[string]any{"ref_count": int(sourceRefCount), "status": "disabled", "embedding": nil, "merge_embedding": nil}).Error
	})
	if err != nil {
		return err
	}
	// 提交后增量失效（design D2）：source 按行删除，target 以最终 aliases 刷新。
	s.cache.applyMerge(sourceID, &target)
	return nil
}

func (s *AuxiliaryLabelService) RemoveBoardComposition(ctx context.Context, boardID uint, auxiliaryLabelID uint) error {
	if boardID == 0 || auxiliaryLabelID == 0 {
		return fmt.Errorf("board and auxiliary label ids are required")
	}

	var board models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		return err
	}
	var auxiliary models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", auxiliaryLabelID, "auxiliary").First(&auxiliary).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("board_id = ? AND auxiliary_label_id = ?", boardID, auxiliaryLabelID).
			Delete(&tagmodels.BoardComposition{}).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`DELETE FROM topic_tag_board_labels
			 WHERE semantic_board_id = ?
			 AND EXISTS (
			   SELECT 1 FROM topic_tag_semantic_labels
			   WHERE topic_tag_semantic_labels.topic_tag_id = topic_tag_board_labels.topic_tag_id
			   AND topic_tag_semantic_labels.semantic_label_id = ?
			 )`, boardID, auxiliaryLabelID,
		).Error; err != nil {
			return err
		}
		return nil
	})
}

// RecountRefs recalculates ref_count for the given auxiliary label IDs by counting
// actual topic_tag_semantic_labels rows. This is self-healing: it corrects any
// accumulated drift in ref_count values.
func (s *AuxiliaryLabelService) RecountRefs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		var count int64
		if err := s.db.WithContext(ctx).Model(&tagmodels.TopicTagSemanticLabel{}).
			Where("semantic_label_id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("recount refs for semantic_label %d: %w", id, err)
		}
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id = ?", id).Update("ref_count", int(count)).Error; err != nil {
			return fmt.Errorf("update ref_count for semantic_label %d: %w", id, err)
		}
	}
	return nil
}

// auxLabelBackfillBatchSize is the per-batch row count when scanning
// dead-vector auxiliary labels (design D3). Cursor pagination (id > lastID)
// ensures every qualifying row is visited exactly once per trigger, even
// when some rows fail generation and stay NULL.
const auxLabelBackfillBatchSize = 50

// AuxLabelBackfillSummary reports what one backfill pass did.
type AuxLabelBackfillSummary struct {
	Scanned int `json:"scanned"` // dead-vector rows visited this pass
	Fixed   int `json:"fixed"`   // rows whose missing vectors were rebuilt
	Skipped int `json:"skipped"` // rows skipped due to generation/persist failure
}

// CountMissingEmbeddings counts active auxiliary labels whose embedding or
// merge_embedding is NULL — the dead-vector rows targeted by backfill
// (fix-alias-save-vector-wipe: addAlias 全量 Save 清零向量的受害行).
func (s *AuxiliaryLabelService) CountMissingEmbeddings(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Where("embedding IS NULL OR merge_embedding IS NULL").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count dead-vector auxiliary labels: %w", err)
	}
	return count, nil
}

// BackfillMissingEmbeddings rebuilds missing vectors for active auxiliary
// labels using the exact L3 generation rules (merge embedding ← label,
// storage embedding ← "label: description" or bare label). Only the NULL
// column(s) of each row are written — existing vectors are never touched,
// which also makes the pass idempotent: re-triggering processes only rows
// that are still NULL. Row-level failures (LLM error, persist error) are
// logged as warnings and skipped so one bad row cannot block the rest.
func (s *AuxiliaryLabelService) BackfillMissingEmbeddings(ctx context.Context) (*AuxLabelBackfillSummary, error) {
	summary := &AuxLabelBackfillSummary{}
	var lastID uint
	for {
		var rows []models.SemanticLabel
		if err := s.db.WithContext(ctx).
			Select("id, label, description, embedding, merge_embedding").
			Where("label_type = ? AND status = ?", "auxiliary", "active").
			Where("id > ?", lastID).
			Where("embedding IS NULL OR merge_embedding IS NULL").
			Order("id ASC").
			Limit(auxLabelBackfillBatchSize).
			Find(&rows).Error; err != nil {
			return summary, fmt.Errorf("query dead-vector auxiliary labels: %w", err)
		}
		if len(rows) == 0 {
			if summary.Fixed > 0 {
				// 恢复的 merge 向量此前不在 L2 去重缓存里，失效后下次打标即吃到新向量
				s.cache.mu.Lock()
				s.cache.mergeEmbeddings = nil
				s.cache.mergeEmbeddingsAt = time.Time{}
				s.cache.mu.Unlock()
			}
			return summary, nil
		}
		for i := range rows {
			lastID = rows[i].ID
			if err := s.backfillRow(ctx, &rows[i]); err != nil {
				logging.Warnf("aux label backfill: skip id=%d label=%q: %v", rows[i].ID, rows[i].Label, err)
				summary.Skipped++
				continue
			}
			summary.Fixed++
		}
		summary.Scanned += len(rows)
	}
}

// backfillRow rebuilds the NULL vector columns of one auxiliary label with
// L3-identical generation rules, writing only the columns it rebuilt.
func (s *AuxiliaryLabelService) backfillRow(ctx context.Context, row *models.SemanticLabel) error {
	updates := map[string]any{}
	if row.MergeEmbedding == nil {
		mergePgVector, _, err := s.embedder(ctx, row.Label, AuxiliaryLabelEmbeddingModeMerge)
		if err != nil {
			return fmt.Errorf("merge embedding: %w", err)
		}
		updates["merge_embedding"] = mergePgVector
	}
	if row.Embedding == nil {
		storageInput := row.Label
		if desc := strings.TrimSpace(row.Description); desc != "" {
			storageInput = row.Label + ": " + desc
		}
		storagePgVector, _, err := s.embedder(ctx, storageInput, AuxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			return fmt.Errorf("storage embedding: %w", err)
		}
		updates["embedding"] = storagePgVector
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("id = ?", row.ID).
		Updates(updates).Error
}

// GC performs garbage collection on auxiliary labels based on the requested mode.
func (s *AuxiliaryLabelService) GC(ctx context.Context, req AuxLabelGCRequest) (*AuxLabelGCResult, error) {
	switch req.Mode {
	case AuxLabelGCModeRecalculate:
		return s.gcRecalculate(ctx)
	case AuxLabelGCModeDryRun, AuxLabelGCModeDisable, AuxLabelGCModeDelete:
		return s.gcCleanup(ctx, req)
	default:
		return nil, fmt.Errorf("invalid mode %q, must be one of: dry_run, disable, delete, recalculate", req.Mode)
	}
}

func (s *AuxiliaryLabelService) gcRecalculate(ctx context.Context) (*AuxLabelGCResult, error) {
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ?", "auxiliary").
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("query auxiliary labels: %w", err)
	}

	if len(ids) == 0 {
		return &AuxLabelGCResult{TotalCount: 0, CorrectedCount: 0}, nil
	}

	// Collect before-state for diff
	type refRow struct {
		ID       uint
		RefCount int
	}
	var before []refRow
	s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, ref_count").
		Where("id IN ?", ids).
		Find(&before)
	beforeCounts := make(map[uint]int, len(before))
	for _, r := range before {
		beforeCounts[r.ID] = r.RefCount
	}

	if err := s.RecountRefs(ctx, ids); err != nil {
		return nil, fmt.Errorf("recount refs: %w", err)
	}

	// Count how many actually changed
	var after []refRow
	s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, ref_count").
		Where("id IN ?", ids).
		Find(&after)
	corrected := 0
	for _, r := range after {
		if beforeCounts[r.ID] != r.RefCount {
			corrected++
		}
	}

	return &AuxLabelGCResult{TotalCount: len(ids), CorrectedCount: corrected}, nil
}

func (s *AuxiliaryLabelService) gcCleanup(ctx context.Context, req AuxLabelGCRequest) (*AuxLabelGCResult, error) {
	if req.GraceDays <= 0 {
		req.GraceDays = 1
	}

	cutoff := time.Now().AddDate(0, 0, -req.GraceDays)

	// Find eligible labels: active, unprotected, past grace period,
	// no topic_tag_semantic_labels AND not mounted on any board
	var eligible []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND protected = false", "auxiliary", "active").
		Where("created_at < ?", cutoff).
		Where("id NOT IN (SELECT DISTINCT semantic_label_id FROM topic_tag_semantic_labels)").
		Where("id NOT IN (SELECT DISTINCT auxiliary_label_id FROM board_composition)").
		Find(&eligible).Error
	if err != nil {
		return nil, fmt.Errorf("query eligible labels: %w", err)
	}

	result := &AuxLabelGCResult{EligibleCount: len(eligible)}

	// Preview (first 20)
	previewLimit := 20
	if len(eligible) < previewLimit {
		previewLimit = len(eligible)
	}
	result.Preview = make([]AuxLabelGCPreviewItem, previewLimit)
	for i := 0; i < previewLimit; i++ {
		result.Preview[i] = AuxLabelGCPreviewItem{
			ID:        eligible[i].ID,
			Label:     eligible[i].Label,
			RefCount:  eligible[i].RefCount,
			CreatedAt: eligible[i].CreatedAt.Format(time.RFC3339),
		}
	}

	if req.Mode == AuxLabelGCModeDryRun || len(eligible) == 0 {
		return result, nil
	}

	ids := make([]uint, len(eligible))
	for i, l := range eligible {
		ids[i] = l.ID
	}

	if req.Mode == AuxLabelGCModeDisable {
		// Delete board_composition rows referencing these labels
		if err := s.db.WithContext(ctx).Where("auxiliary_label_id IN ?", ids).
			Delete(&tagmodels.BoardComposition{}).Error; err != nil {
			return nil, fmt.Errorf("delete board_composition: %w", err)
		}
		// Soft-delete: update status to disabled (vectors dropped — re-enable regenerates)
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id IN ?", ids).
			Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil}).Error; err != nil {
			return nil, fmt.Errorf("disable labels: %w", err)
		}
		result.AffectedCount = len(ids)
	} else { // delete
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).
			Delete(&models.SemanticLabel{}).Error; err != nil {
			return nil, fmt.Errorf("delete labels: %w", err)
		}
		result.AffectedCount = len(ids)
	}

	return result, nil
}

func (s *AuxiliaryLabelService) loadActiveAuxiliaryLabels(ctx context.Context) ([]models.SemanticLabel, error) {
	s.cache.mu.RLock()
	if s.cache.activeLabels != nil && time.Since(s.cache.activeLabelsAt) < auxActiveLabelsTTL {
		labels := s.cache.activeLabels
		s.cache.mu.RUnlock()
		return labels, nil
	}
	s.cache.mu.RUnlock()

	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	// Double-check after acquiring write lock
	if s.cache.activeLabels != nil && time.Since(s.cache.activeLabelsAt) < auxActiveLabelsTTL {
		return s.cache.activeLabels, nil
	}

	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Select("id, label, slug, label_type, aliases, ref_count, description, status, protected, source, display_order, created_at, updated_at").
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Find(&labels).Error
	if err != nil {
		return nil, err
	}
	s.cache.activeLabels = labels
	s.cache.activeLabelsAt = time.Now()
	return labels, nil
}

// lightweightLabelCopy projects a label row onto the column set
// loadActiveAuxiliaryLabels selects — vectors are deliberately excluded
// (the 17.5k-row memory budget), so incremental cache appends must nil them.
func lightweightLabelCopy(row *models.SemanticLabel) models.SemanticLabel {
	copy := *row
	copy.Embedding = nil
	copy.MergeEmbedding = nil
	return copy
}

// applyCreate incrementally absorbs one L3-created label (design D2): a
// lightweight row copy is appended to the active list. The merge-embedding
// map is deliberately NOT touched: readers (sqlMergeMatcher) index it
// without holding the lock — an in-place map write races them into
// `fatal error: concurrent map read and map write` (unrecoverable, kills
// the whole process; observed 2026-10-07). The new vector becomes visible
// to L2 matching on the next full reload (10min TTL). Timestamps are NOT
// reset — an incremental upsert is not a cache expiry, so the 216MB full
// pull never fires mid-batch. Cold (nil) cache halves stay nil; the next
// full load picks the row up.
func (c *auxLabelCache) applyCreate(created *models.SemanticLabel, mergeVector []float64) {
	_ = mergeVector
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeLabels != nil {
		// append beyond any reader's snapshot length — safe against concurrent
		// iterators, which never touch indices ≥ their snapshot len.
		c.activeLabels = append(c.activeLabels, lightweightLabelCopy(created))
	}
}

// applyDisable incrementally removes disabled labels (design D2): the
// active list is filtered. The merge-embedding map is deliberately NOT
// touched (see applyCreate: in-place map mutation races lock-free readers
// into a fatal). Stale vectors linger until the next full reload (10min
// TTL). Fresh slices only — in-place filtering would race with readers
// holding the old slice header.
func (c *auxLabelCache) applyDisable(ids ...uint) {
	if len(ids) == 0 {
		return
	}
	drop := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		drop[id] = struct{}{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeLabels != nil {
		kept := make([]models.SemanticLabel, 0, len(c.activeLabels))
		for _, l := range c.activeLabels {
			if _, gone := drop[l.ID]; !gone {
				kept = append(kept, l)
			}
		}
		c.activeLabels = kept
	}
}

// applyMerge absorbs one MergeAuxiliaryLabelAlias outcome (design D2): the
// disabled source is dropped from the active list and the target's cached
// copy is refreshed with its final aliases — no full invalidation. The
// merge-embedding map is deliberately NOT touched (see applyCreate).
func (c *auxLabelCache) applyMerge(sourceID uint, target *models.SemanticLabel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeLabels == nil {
		return
	}
	refreshed := lightweightLabelCopy(target)
	kept := make([]models.SemanticLabel, 0, len(c.activeLabels))
	for _, l := range c.activeLabels {
		switch l.ID {
		case sourceID: // disabled — drop
		case target.ID:
			kept = append(kept, refreshed)
		default:
			kept = append(kept, l)
		}
	}
	c.activeLabels = kept
}

func (s *AuxiliaryLabelService) invalidateOnAliasChange() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.activeLabels = nil
	s.cache.activeLabelsAt = time.Time{}
	// mergeEmbeddings preserved — vectors unchanged by alias changes
}

// loadAuxLabelDedupeSim reads the configurable L2 merge similarity threshold
// from ai_settings (key auxiliary_label_dedupe_sim). Falls back to
// defaultAuxLabelDedupeSim (0.95) when the row is missing or the value is not a
// valid float in (0,1]. Called once per L2 evaluation (only when L1 misses — the
// new-entity path), so a single DB read is acceptable (design D5).
func loadAuxLabelDedupeSim(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", "auxiliary_label_dedupe_sim").First(&setting).Error; err != nil {
		return defaultAuxLabelDedupeSim
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(setting.Value), 64)
	if err != nil || v <= 0 || v > 1 {
		return defaultAuxLabelDedupeSim
	}
	return v
}

// sqlMergeMatcher loads merge embeddings from cache and computes cosine
// similarity in Go. Eliminates the 216 MB DB transfer on every call.
func sqlMergeMatcher(ctx context.Context, db *gorm.DB, labels []models.SemanticLabel, _ string, mergeVector []float64) (*models.SemanticLabel, error) {
	// Use package-level cache
	embeddings, err := packageAuxLabelCacheGetOrLoad(ctx, db)
	if err != nil {
		return nil, err
	}
	threshold := loadAuxLabelDedupeSim(db)

	var best *models.SemanticLabel
	for i := range labels {
		existingVec, ok := embeddings[labels[i].ID]
		if !ok {
			continue
		}
		sim, err := airouter.CosineSimilarity(mergeVector, existingVec)
		if err != nil || sim < threshold {
			continue
		}
		candidate := labels[i]
		if best == nil || candidate.RefCount > best.RefCount || (candidate.RefCount == best.RefCount && candidate.ID < best.ID) {
			best = &candidate
		}
	}
	return best, nil
}

// packageAuxLabelCacheGetOrLoad loads merge embeddings using the package-level cache.
// This is a standalone helper since sqlMergeMatcher is a package-level function (not a method).
func packageAuxLabelCacheGetOrLoad(ctx context.Context, db *gorm.DB) (map[uint][]float64, error) {
	packageAuxLabelCache.mu.RLock()
	if packageAuxLabelCache.mergeEmbeddings != nil && time.Since(packageAuxLabelCache.mergeEmbeddingsAt) < auxMergeEmbeddingsTTL {
		emb := packageAuxLabelCache.mergeEmbeddings
		packageAuxLabelCache.mu.RUnlock()
		return emb, nil
	}
	packageAuxLabelCache.mu.RUnlock()

	packageAuxLabelCache.mu.Lock()
	defer packageAuxLabelCache.mu.Unlock()
	if packageAuxLabelCache.mergeEmbeddings != nil && time.Since(packageAuxLabelCache.mergeEmbeddingsAt) < auxMergeEmbeddingsTTL {
		return packageAuxLabelCache.mergeEmbeddings, nil
	}

	type row struct {
		ID             uint
		MergeEmbedding *string
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, merge_embedding").
		Where("label_type = ? AND status = ? AND merge_embedding IS NOT NULL", "auxiliary", "active").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uint][]float64, len(rows))
	for _, r := range rows {
		if r.MergeEmbedding == nil || *r.MergeEmbedding == "" {
			continue
		}
		vec, err := ParsePgVector(*r.MergeEmbedding)
		if err != nil {
			continue
		}
		result[r.ID] = vec
	}
	packageAuxLabelCache.mergeEmbeddings = result
	packageAuxLabelCache.mergeEmbeddingsAt = time.Now()
	return result, nil
}

// applyAuxAliasWrite（列级只写 aliases 列）取代了历史 addAlias：label 来自缓存加载
// （Select 刻意排除向量列，17.5k 标签的内存预算），历史实现的全量 Save 会把 nil
// 向量字段写成 NULL——标签向量静默清零的根源（fix-alias-save-vector-wipe）。
// struct 路径才会对 jsonb serializer 字段（Aliases）做序列化，updated_at 照常自动维护。

func semanticAliasesContain(aliases []string, label string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(label)) || core.Slugify(alias) == core.Slugify(label) {
			return true
		}
	}
	return false
}

func DefaultAuxiliaryLabelEmbedder(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	opName := "auxiliary_label_storage_embedding"
	if mode == AuxiliaryLabelEmbeddingModeMerge {
		opName = "auxiliary_label_merge_embedding"
	}
	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{input},
		Operation: "tagmanagement.auxlabel_embedding",
		Metadata: map[string]any{
			"operation": opName,
			"label":     input,
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return "", nil, err
	}
	if result == nil || len(result.Embeddings) == 0 {
		return "", nil, fmt.Errorf("empty embedding result")
	}
	vector := result.Embeddings[0]
	return core.FloatsToPgVector(vector), vector, nil
}

// auxBatchEmbedChunk caps per-request inputs of the default batch embedder
// (provider payload safety); one attach batch rarely exceeds this.
const auxBatchEmbedChunk = 32

// DefaultAuxiliaryLabelBatchEmbedder implements AuxiliaryLabelBatchEmbedder
// against airouter.Embed — Input []string is native there, so one call per mode
// per chunk replaces N per-label round trips (design D1).
func DefaultAuxiliaryLabelBatchEmbedder(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
	opName := "auxiliary_label_storage_embedding"
	if mode == AuxiliaryLabelEmbeddingModeMerge {
		opName = "auxiliary_label_merge_embedding"
	}
	router := airouter.NewRouter()
	pgVectors := make([]string, 0, len(inputs))
	vectors := make([][]float64, 0, len(inputs))
	for _, chunk := range chunkStrings(inputs, auxBatchEmbedChunk) {
		result, err := router.Embed(ctx, airouter.EmbeddingRequest{
			Input:     chunk,
			Operation: "tagmanagement.auxlabel_embedding",
			Metadata: map[string]any{
				"operation": opName,
				"batch":     len(chunk),
			},
		}, airouter.CapabilityEmbedding)
		if err != nil {
			return nil, nil, err
		}
		if result == nil || len(result.Embeddings) != len(chunk) {
			got := 0
			if result != nil {
				got = len(result.Embeddings)
			}
			return nil, nil, fmt.Errorf("batch embedding returned %d vectors for %d inputs", got, len(chunk))
		}
		for _, vector := range result.Embeddings {
			pgVectors = append(pgVectors, core.FloatsToPgVector(vector))
			vectors = append(vectors, vector)
		}
	}
	return pgVectors, vectors, nil
}

// batchFromSingleEmbedder adapts a single-input embedder (usually a test
// mock) into the batch signature by looping — correct, just not batched.
func batchFromSingleEmbedder(single AuxiliaryLabelEmbedder) AuxiliaryLabelBatchEmbedder {
	return func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
		pgVectors := make([]string, len(inputs))
		vectors := make([][]float64, len(inputs))
		for i, input := range inputs {
			pg, vec, err := single(ctx, input, mode)
			if err != nil {
				return nil, nil, err
			}
			pgVectors[i], vectors[i] = pg, vec
		}
		return pgVectors, vectors, nil
	}
}

// chunkStrings splits s into consecutive chunks of at most size elements.
func chunkStrings(s []string, size int) [][]string {
	if size <= 0 || len(s) <= size {
		return [][]string{s}
	}
	var out [][]string
	for start := 0; start < len(s); start += size {
		end := start + size
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[start:end])
	}
	return out
}

func ParsePgVector(value string) ([]float64, error) {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if value == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(value, ",")
	result := make([]float64, 0, len(parts))
	for _, part := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, nil
}

func UniqueSemanticLabelSlug(db *gorm.DB, base string) string {
	slug := base
	for i := 2; ; i++ {
		var count int64
		db.Model(&models.SemanticLabel{}).Where("slug = ?", slug).Count(&count)
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// ensureSemanticLabelVectorColumnDim checks if a vector column on semantic_labels
// matches the required dimension and alters it if not. column is a hardcoded column
// name ("embedding" / "merge_embedding"), never user input, so it is safe to splice
// into SQL.
//
// Should only be called at startup; DDL operations use a 5s lock timeout to avoid
// blocking if other connections hold table locks.
//
// On ALTER failure it diagnoses whether stale data is blocking the resize and returns
// an actionable error: pgvector cannot truncate existing vectors, so the caller must
// clear the rows before the dimension can change.
func ensureSemanticLabelVectorColumnDim(column string, dim int) error {
	db := repository.Repo.DB()
	if err := db.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
		logging.Warnf("Failed to set lock_timeout: %v", err)
	}

	var typeStr string
	if err := db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'semantic_labels' AND a.attname = ?`, column,
	).Row().Scan(&typeStr); err != nil {
		return nil // column may not exist yet, let migration handle it
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	if typeStr == expected {
		return nil
	}

	logging.Infof("Altering semantic_labels.%s column from %s to %s", column, typeStr, expected)

	_ = db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS idx_semantic_labels_%s", column)).Error

	if err := db.Exec(fmt.Sprintf(
		"ALTER TABLE semantic_labels ALTER COLUMN %s TYPE %s", column, expected,
	)).Error; err != nil {
		var staleRows int64
		_ = db.Raw(fmt.Sprintf(
			"SELECT count(*) FROM semantic_labels WHERE %s IS NOT NULL", column,
		)).Scan(&staleRows).Error
		if staleRows > 0 {
			return fmt.Errorf(
				"alter semantic_labels.%s from %s to %s failed: %d row(s) contain stale vectors of the old dimension; "+
					"clear them (e.g. UPDATE semantic_labels SET %s = NULL) then restart: %w",
				column, typeStr, expected, staleRows, column, err,
			)
		}
		return fmt.Errorf("alter semantic_labels.%s column to %s: %w", column, expected, err)
	}

	return nil
}

// EnsureSemanticLabelVectorDimension checks if the semantic_labels.embedding column
// matches the required dimension and alters it if not.
func EnsureSemanticLabelVectorDimension(dim int) error {
	return ensureSemanticLabelVectorColumnDim("embedding", dim)
}

// EnsureSemanticLabelMergeVectorDimension checks if the semantic_labels.merge_embedding
// column matches the required dimension and alters it if not.
func EnsureSemanticLabelMergeVectorDimension(dim int) error {
	return ensureSemanticLabelVectorColumnDim("merge_embedding", dim)
}
