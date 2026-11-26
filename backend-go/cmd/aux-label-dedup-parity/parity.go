package main

// --- fix-aux-label-dedup-ann design D8：新旧 L2 去重实现的一次性对账 ---
//
// 本文件是**运维工具**（不是服务代码）：它冻结了被淘汰的旧算法（进程内对全量
// active 辅助标签算余弦），并与现役的数据库 ANN 召回路径逐条对账。
//
// 为什么住在 cmd 包下（review R-6）：旧算法复刻常驻 service 包会被编进 server
// 二进制——一次性对账程序没有退役计划，等于把 392 行被淘汰的实现永远挂在生产
// 进程里。挪进 cmd 后它只是仓库里的一次性工具，不进 server 二进制。
//
// 口径一致性要求（**改服务侧实现 MUST 同步本文件**，否则对账结论失真）：
//   - 召回 SQL：与 auxlabel 服务的 mergeRecallSQL 同款——WHERE 与部分索引谓词
//     逐字一致，halfvec(<dim>) typmod 带运行时维度；
//   - 精排：用原列 merge_embedding <=> ?::vector（float32 精确距离），halfvec
//     近似距离不参与阈值判断；
//   - 裁决：cosine ≥ 阈值、RefCount 降序、同 RefCount 取 ID 小者。
// 服务侧对应实现：internal/tagmanagement/service/auxlabel/auxiliary_label_service.go
// 的 sqlMergeMatcher / mergeRecallSQL / queryMergeCandidates / pickMergeTarget。
//
// 旧侧刻意做成**流式**（分批读行 × 逐样本累积）：全量驻留是这次要消灭的病灶本身
// （26,767 × 2560 float64 ≈ 547MB），对账程序没必要重现这个峰值。FLOPs 与全量扫描
// 等价，结果逐条相同。
//
// 调用方以 default_transaction_read_only=on 连接真实库。

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
)

// L2 召回调参（与服务侧同一组常量）：hnsw.ef_search MUST ≥ 候选上限（pgvector
// 默认 40 会截断更大的 LIMIT）；候选池被填满且全部在阈值内时，用更宽的池重查一次。
const (
	mergeCandidateLimit     = 32
	mergeCandidateLimitWide = 128
	mergeEfSearch           = 100
	mergeEfSearchWide       = 200
)

// defaultDedupeThreshold is the fallback L2 merge similarity threshold when
// ai_settings.auxiliary_label_dedupe_sim is missing or invalid.
const defaultDedupeThreshold = 0.95

// DedupParityOptions tunes one parity run.
type DedupParityOptions struct {
	SampleSize int // existing active auxiliary labels replayed as inputs (default 200)
	BatchSize  int // rows streamed per legacy scan batch (default 512)
}

// DedupParityTarget is one resolved merge target (or the absence of one).
type DedupParityTarget struct {
	ID       uint    `json:"id"`
	Label    string  `json:"label"`
	RefCount int     `json:"ref_count"`
	Sim      float64 `json:"sim"`
	Found    bool    `json:"found"`
}

// DedupParityMismatch records one sample where the two implementations disagree.
type DedupParityMismatch struct {
	SampleID      uint              `json:"sample_id"`
	SampleLabel   string            `json:"sample_label"`
	Legacy        DedupParityTarget `json:"legacy"`
	Current       DedupParityTarget `json:"current"`
	NeighborCount int               `json:"legacy_neighbor_count"`
	Attribution   string            `json:"attribution"`
}

// DedupParityReport is the full parity outcome.
type DedupParityReport struct {
	SampleSize               int                   `json:"sample_size"`
	ActiveLabels             int                   `json:"active_labels"`
	Dimension                int                   `json:"dimension"`
	Threshold                float64               `json:"threshold"`
	RecallPlan               []string              `json:"recall_plan"`
	RecallPlanUsesIndex      bool                  `json:"recall_plan_uses_index"`
	Mismatches               []DedupParityMismatch `json:"mismatches"`
	SamplesWithMultiNeighbor int                   `json:"samples_with_two_or_more_neighbors"`
	Elapsed                  time.Duration         `json:"elapsed"`
}

// RunDedupParity compares the live matcher against the frozen legacy algorithm on
// sampleSize vectors taken from the real table (replaying stored vectors costs no
// LLM call and is reproducible — design D8).
func RunDedupParity(ctx context.Context, db *gorm.DB, opts DedupParityOptions) (*DedupParityReport, error) {
	start := time.Now()
	if opts.SampleSize <= 0 {
		opts.SampleSize = 200
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 512
	}
	threshold := loadDedupeThreshold(db)

	var activeCount int64
	if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ? AND status = ? AND merge_embedding IS NOT NULL", "auxiliary", "active").
		Count(&activeCount).Error; err != nil {
		return nil, fmt.Errorf("count active auxiliary labels: %w", err)
	}

	samples, err := loadDedupParitySamples(ctx, db, opts.SampleSize)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("no active auxiliary labels with merge_embedding to sample")
	}
	report := &DedupParityReport{
		SampleSize:   len(samples),
		ActiveLabels: int(activeCount),
		Dimension:    len(samples[0].vec),
		Threshold:    threshold,
	}

	report.RecallPlan, report.RecallPlanUsesIndex, err = dedupRecallPlan(ctx, db, samples[0])
	if err != nil {
		return nil, err
	}

	legacy, err := legacyDedupScan(ctx, db, samples, threshold, opts)
	if err != nil {
		return nil, err
	}

	for i := range samples {
		sample := samples[i]
		current, err := matchMergeCandidate(ctx, db, sample.pgVector, sample.vec)
		if err != nil {
			return nil, fmt.Errorf("current matcher on sample %d (%s): %w", sample.id, sample.label, err)
		}
		l := legacy[i]
		// Metric: peers other than the sample's own row (self is always a perfect
		// match and would make every sample look like a multi-neighbor one).
		if l.neighbors >= 2 {
			report.SamplesWithMultiNeighbor++
		}
		if sameTarget(l.best, current) {
			continue
		}
		currentTarget := targetFromRow(current)
		if current != nil {
			currentTarget.Sim, err = dedupExactSim(ctx, db, sample.pgVector, current.ID)
			if err != nil {
				return nil, err
			}
		}
		report.Mismatches = append(report.Mismatches, DedupParityMismatch{
			SampleID:      sample.id,
			SampleLabel:   sample.label,
			Legacy:        targetFromRow(l.best),
			Current:       currentTarget,
			NeighborCount: l.neighbors,
			Attribution:   attributeMismatch(l.best, current, threshold),
		})
	}

	sort.Slice(report.Mismatches, func(a, b int) bool {
		return report.Mismatches[a].SampleID < report.Mismatches[b].SampleID
	})
	report.Elapsed = time.Since(start)
	return report, nil
}

// loadDedupeThreshold reads the L2 merge similarity threshold from ai_settings
// (key auxiliary_label_dedupe_sim), falling back to defaultDedupeThreshold.
// Mirrors the service-side loader (the service keeps it unexported — the parity
// tool must not add a production export for its own sake).
func loadDedupeThreshold(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", "auxiliary_label_dedupe_sim").First(&setting).Error; err != nil {
		return defaultDedupeThreshold
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(setting.Value), 64)
	if err != nil || v <= 0 || v > 1 {
		return defaultDedupeThreshold
	}
	return v
}

type dedupParitySample struct {
	id       uint
	label    string
	refCount int
	vec      []float64
	pgVector string
}

// loadDedupParitySamples reads sampleSize existing active auxiliary labels and
// parses their merge vectors.
func loadDedupParitySamples(ctx context.Context, db *gorm.DB, sampleSize int) ([]dedupParitySample, error) {
	type row struct {
		ID             uint
		Label          string
		RefCount       int
		MergeEmbedding *string
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, label, ref_count, merge_embedding").
		Where("label_type = ? AND status = ? AND merge_embedding IS NOT NULL", "auxiliary", "active").
		Order("ref_count DESC, id ASC").
		Limit(sampleSize).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load parity samples: %w", err)
	}
	out := make([]dedupParitySample, 0, len(rows))
	for _, r := range rows {
		if r.MergeEmbedding == nil || strings.TrimSpace(*r.MergeEmbedding) == "" {
			continue
		}
		vec, err := auxlabel.ParsePgVector(*r.MergeEmbedding)
		if err != nil {
			continue // unparsable row: skip rather than abort the whole run
		}
		out = append(out, dedupParitySample{
			id: r.ID, label: r.Label, refCount: r.RefCount, vec: vec, pgVector: *r.MergeEmbedding,
		})
	}
	return out, nil
}

// dedupRecallPlan runs EXPLAIN on the stage-1 recall statement with the sample
// vector, reporting whether the halfvec HNSW index is used.
func dedupRecallPlan(ctx context.Context, db *gorm.DB, sample dedupParitySample) ([]string, bool, error) {
	planSQL := "EXPLAIN (COSTS OFF) " + strings.TrimSpace(mergeRecallSQL(len(sample.vec)))
	var plan []string
	if err := db.WithContext(ctx).Raw(planSQL, sample.pgVector, mergeCandidateLimit).Scan(&plan).Error; err != nil {
		return nil, false, fmt.Errorf("explain recall query: %w", err)
	}
	usesIndex := false
	for _, line := range plan {
		if strings.Contains(line, "semantic_labels_merge_halfvec_hnsw") {
			usesIndex = true
		}
	}
	return plan, usesIndex, nil
}

// dedupExactSim reads the exact float32 cosine between the sample vector and one
// target row (report-only, no index).
func dedupExactSim(ctx context.Context, db *gorm.DB, samplePgVector string, targetID uint) (float64, error) {
	var sim float64
	if err := db.WithContext(ctx).Raw(`
		SELECT 1 - (merge_embedding <=> ?::vector)
		FROM semantic_labels WHERE id = ?`, samplePgVector, targetID).Scan(&sim).Error; err != nil {
		return 0, fmt.Errorf("exact similarity for target %d: %w", targetID, err)
	}
	return sim, nil
}

// legacyBest is the legacy algorithm's outcome for one sample.
type legacyBest struct {
	best      *models.SemanticLabel
	sim       float64
	neighbors int // PEER rows at or above the threshold (the sample's own row excluded)
}

// legacyRow is one streamed row with its vector and norm precomputed.
type legacyRow struct {
	id       uint
	label    string
	refCount int
	vec      []float64
	norm     float64
}

// legacyDedupScan replays the pre-change algorithm (in-process full cosine against
// every active auxiliary label). Memory stays bounded: the table is streamed in
// batches, parsed once per batch, then scored against all samples. The sample
// rows participate in adjudication exactly as in production (a stored label IS a
// candidate), while the neighbor metric excludes them.
func legacyDedupScan(ctx context.Context, db *gorm.DB, samples []dedupParitySample, threshold float64, opts DedupParityOptions) ([]legacyBest, error) {
	results := make([]legacyBest, len(samples))
	for i := range samples {
		results[i].sim = math.Inf(-1)
	}
	sampleNorms := make([]float64, len(samples))
	for i := range samples {
		sampleNorms[i] = vectorNorm(samples[i].vec)
	}
	workers := runtime.GOMAXPROCS(0)

	var lastID uint
	for {
		type row struct {
			ID             uint
			Label          string
			RefCount       int
			MergeEmbedding *string
		}
		var rows []row
		if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Select("id, label, ref_count, merge_embedding").
			Where("label_type = ? AND status = ? AND merge_embedding IS NOT NULL", "auxiliary", "active").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(opts.BatchSize).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("stream active auxiliary labels: %w", err)
		}
		if len(rows) == 0 {
			return results, nil
		}
		batch := make([]legacyRow, 0, len(rows))
		for i := range rows {
			lastID = rows[i].ID
			if rows[i].MergeEmbedding == nil {
				continue
			}
			vec, err := auxlabel.ParsePgVector(*rows[i].MergeEmbedding)
			if err != nil {
				continue
			}
			batch = append(batch, legacyRow{
				id: rows[i].ID, label: rows[i].Label, refCount: rows[i].RefCount,
				vec: vec, norm: vectorNorm(vec),
			})
		}

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for s := w; s < len(samples); s += workers {
					scoreSampleAgainstBatch(&results[s], samples[s], sampleNorms[s], batch, threshold)
				}
			}(w)
		}
		wg.Wait()
	}
}

// scoreSampleAgainstBatch updates one sample's best/neighbor counters with a batch
// of rows. Straight scalar loop: exactly the legacy formula, no vectorization
// tricks that could change the arithmetic.
func scoreSampleAgainstBatch(result *legacyBest, sample dedupParitySample, sampleNorm float64, batch []legacyRow, threshold float64) {
	for i := range batch {
		row := batch[i]
		if len(row.vec) != len(sample.vec) || sampleNorm == 0 || row.norm == 0 {
			continue
		}
		sim := cosineWithNorms(sample.vec, row.vec, sampleNorm, row.norm)
		if sim < threshold {
			continue
		}
		if row.id != sample.id {
			result.neighbors++
		}
		if result.best == nil || row.refCount > result.best.RefCount ||
			(row.refCount == result.best.RefCount && row.id < result.best.ID) {
			candidate := models.SemanticLabel{ID: row.id, Label: row.label, RefCount: row.refCount}
			result.best = &candidate
			result.sim = sim
		}
	}
}

// cosineWithNorms is the pre-change similarity formula (airouter.CosineSimilarity):
// float64 dot product over the stored float32 components, scaled by the norms.
func cosineWithNorms(a, b []float64, normA, normB float64) float64 {
	if normA == 0 || normB == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func vectorNorm(v []float64) float64 {
	var sum float64
	for i := range v {
		sum += v[i] * v[i]
	}
	return sum
}

func sameTarget(legacy *models.SemanticLabel, current *models.SemanticLabel) bool {
	switch {
	case legacy == nil && current == nil:
		return true
	case legacy == nil || current == nil:
		return false
	default:
		return legacy.ID == current.ID
	}
}

func targetFromRow(row *models.SemanticLabel) DedupParityTarget {
	if row == nil {
		return DedupParityTarget{}
	}
	return DedupParityTarget{ID: row.ID, Label: row.Label, RefCount: row.RefCount, Found: true}
}

// attributeMismatch explains a disagreement using only evidence the run observed.
func attributeMismatch(legacy *models.SemanticLabel, current *models.SemanticLabel, threshold float64) string {
	switch {
	case legacy == nil && current != nil:
		return fmt.Sprintf("current hit id=%d while the legacy scan found no row at/above threshold %.4f", current.ID, threshold)
	case legacy != nil && current == nil:
		return fmt.Sprintf("legacy hit id=%d but the current recall returned no candidate at/above threshold %.4f (possible ANN miss)", legacy.ID, threshold)
	case legacy != nil && current != nil:
		return fmt.Sprintf("both matched but picked different rows (legacy ref=%d id=%d vs current ref=%d id=%d) — RefCount/ID adjudication or recall pool difference",
			legacy.RefCount, legacy.ID, current.RefCount, current.ID)
	default:
		return "unexpected: neither side matched"
	}
}
