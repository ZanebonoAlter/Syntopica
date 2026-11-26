package boardhealth

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
)

// cosineSim is the plain cosine of the matching pipeline's in-memory
// implementation (semantic_board_matching.go CosineSimilarity). Duplicated
// here on purpose: boardhealth must not import the board package (board's
// expand recall consumes boardhealth — design D7), and this is the repo's
// third identical math copy already (airouter/board).
func cosineSim(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// matchedRow is one topic_tag_board_labels row for the board. The affinity
// recompute (口径②修正) no longer annotates aux counts: every mounted tag
// participates in every composition label's distribution regardless of how
// many aux labels it carries.
type matchedRow struct {
	TopicTagID        uint
	MatchReason       string
	DirectionMismatch bool
}

// loadMatchedRows fetches every match row of the board (pure read).
func loadMatchedRows(ctx context.Context, db *gorm.DB, boardID uint) ([]matchedRow, error) {
	var rows []matchedRow
	err := db.WithContext(ctx).Raw(`
		SELECT ttbl.topic_tag_id,
		       COALESCE(ttbl.match_reason, '') AS match_reason,
		       COALESCE(ttbl.direction_mismatch, false) AS direction_mismatch
		FROM topic_tag_board_labels ttbl
		WHERE ttbl.semantic_board_id = ?
		ORDER BY ttbl.topic_tag_id ASC`, boardID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// loadTagIdentityEmbeddings batch-loads identity embeddings for the given
// tags. Missing rows / unparsable vectors simply stay absent from the map.
func loadTagIdentityEmbeddings(ctx context.Context, db *gorm.DB, tagIDs []uint) (map[uint][]float64, error) {
	result := make(map[uint][]float64, len(tagIDs))
	if len(tagIDs) == 0 {
		return result, nil
	}
	type embRow struct {
		TopicTagID   uint   `gorm:"column:topic_tag_id"`
		EmbeddingVec string `gorm:"column:embedding"`
	}
	var rows []embRow
	err := db.WithContext(ctx).Raw(`
		SELECT topic_tag_id, embedding
		FROM topic_tag_embeddings
		WHERE embedding_type = 'identity' AND topic_tag_id IN ?`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.EmbeddingVec == "" {
			continue
		}
		if vector, err := auxlabel.ParsePgVector(row.EmbeddingVec); err == nil && len(vector) > 0 {
			result[row.TopicTagID] = vector
		}
	}
	return result, nil
}

// loadCompositionLabels returns the board's composition entries (aux +
// composite mixed, active only) in stable id order, plus each label's own
// embedding keyed by id (absent when the label has no parsable embedding —
// such labels go insufficient in the affinity recompute, they do not count
// into vector_missing which is tag-side only).
func loadCompositionLabels(ctx context.Context, db *gorm.DB, boardID uint) ([]tagmodels.BoardHealthLabelMetrics, map[uint][]float64, error) {
	var labels []struct {
		ID          uint   `gorm:"column:id"`
		Label       string `gorm:"column:label"`
		LabelType   string `gorm:"column:label_type"`
		Description string `gorm:"column:description"`
		// pgvector 列名 embedding 与字段蛇形名 embedding_vec 不同，必须显式
		// column 标签映射；否则永远扫不到值（2026-10-07 回归：全标签 insufficient）。
		EmbeddingVec *string `gorm:"column:embedding"`
	}
	err := db.WithContext(ctx).Raw(`
		SELECT l.id, l.label, l.label_type, l.description, l.embedding
		FROM board_composition bc
		JOIN semantic_labels l ON l.id = bc.auxiliary_label_id AND l.status = 'active'
		WHERE bc.board_id = ?
		ORDER BY l.id ASC`, boardID).Scan(&labels).Error
	if err != nil {
		return nil, nil, err
	}
	metrics := make([]tagmodels.BoardHealthLabelMetrics, 0, len(labels))
	auxEmb := make(map[uint][]float64, len(labels))
	for _, label := range labels {
		metrics = append(metrics, tagmodels.BoardHealthLabelMetrics{
			AuxID: label.ID,
			Label: label.Label,
			// label_type 透传（auxiliary | composite）：构成里混有组合标签，
			// 前端据此把组合标签的「重写描述」标为不可执行（修复链只认 auxiliary），
			// 避免用户点进一条永远 404 的修复路径。
			LabelType:   label.LabelType,
			Description: label.Description, // 病C 证据源：desc 预筛与 rewrite 判断都要看它
		})
		if label.EmbeddingVec == nil || *label.EmbeddingVec == "" {
			continue
		}
		if vector, err := auxlabel.ParsePgVector(*label.EmbeddingVec); err == nil && len(vector) > 0 {
			auxEmb[label.ID] = vector
		}
	}
	return metrics, auxEmb, nil
}

// computeCoreSubjects marks the composition labels that ARE the board's own
// subject (核心主体) — fix-board-health-magnet-signal design D3（review
// R-6 收紧后的规则）:
//
//   - 标签名（空白/大小写归一后）出现在版块 **description** 或任一
//     **alias** 中 → 核心主体成立（不限字数）；
//   - **仅**出现在版块 **标题**（board.Label）中 → 还需标签名归一后 ≥ 3 个
//     字符（rune 计）才成立：2 字短词既可能是简称（「美伊」之于「伊朗」），
//     也可能是泛化地域词（「中东」）——只凭标题不足以判定，更不得据简称推断
//     全名；
//   - 三处均不出现 → 非核心主体。
//
// 纯字面匹配（归一后子串），MUST NOT 用向量相似度或语义推断：判定要可核、
// 可解释。命中即 CoreSubject=true —— 规则层闸门 G1 会把 LLM 给核心主体的
// remove/rewrite 改判 keep（提示词层也禁判，但提示词已被实证会翻转，规则层
// 才是唯一不依赖模型听话的防线）。
//
// 边界：board 为 nil、三个文本全空、标签名为空/纯空白 → 不标记且不 panic。
func computeCoreSubjects(board *models.SemanticLabel, labels []tagmodels.BoardHealthLabelMetrics) {
	if board == nil {
		return
	}
	title := normalizeSubjectText(board.Label)
	description := normalizeSubjectText(board.Description)
	aliases := make([]string, 0, len(board.Aliases))
	for _, alias := range board.Aliases {
		if normalized := normalizeSubjectText(alias); normalized != "" {
			aliases = append(aliases, normalized)
		}
	}
	for i := range labels {
		name := normalizeSubjectText(labels[i].Label)
		if name == "" {
			continue // 空名 / 纯空白名不参与匹配（构成标签名不会为空，防御）
		}
		// description / aliases 命中即成立（不限字数）。
		if description != "" && strings.Contains(description, name) {
			labels[i].CoreSubject = true
			continue
		}
		for _, alias := range aliases {
			if strings.Contains(alias, name) {
				labels[i].CoreSubject = true
				break
			}
		}
		if labels[i].CoreSubject {
			continue
		}
		// 仅标题命中：需标签名 ≥ 3 字符（简称/泛化词不据标题推断）。
		if title != "" && strings.Contains(title, name) && utf8.RuneCountInString(name) >= coreSubjectTitleMinRunes {
			labels[i].CoreSubject = true
		}
	}
}

// coreSubjectTitleMinRunes 是「仅标题命中」时标签名的最小字符数（归一后按
// rune 计）。2 字短词可能是简称（「美伊」之于「伊朗」）也可能是泛化地域词
// （「中东」）——两者都不足以据标题认定核心主体（design D3 / review R-6）。
const coreSubjectTitleMinRunes = 3

// normalizeSubjectText lowercases the text and drops every whitespace rune
// (含全角空格) so the substring match ignores spacing and case differences.
func normalizeSubjectText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ComputeBoardMetrics produces the board-level aggregate (口径①：全部匹配行).
// Reason distribution buckets empty reasons as "unknown".
func ComputeBoardMetrics(rows []matchedRow) tagmodels.BoardHealthBoardMetrics {
	metrics := tagmodels.BoardHealthBoardMetrics{
		ReasonDist: map[string]int{},
	}
	metrics.MatchedTags = len(rows)
	for _, row := range rows {
		reason := row.MatchReason
		if reason == "" {
			reason = "unknown"
		}
		metrics.ReasonDist[reason]++
		if row.DirectionMismatch {
			metrics.GhostMatches++
		}
	}
	return metrics
}

// computeLabelAttribution produces per-composition-label affinity
// distributions (口径②修正：构成标签 embedding × 全部挂载 tags 的 identity
// embedding——不按 aux 挂数筛样本，不声称功劳归因，只量语义贴合) and
// returns the tag-side vector-missing count. Rules:
//   - every mounted tag with an identity embedding participates in every
//     label's distribution (multi-aux tags are NOT excluded);
//   - cosine is RECOMPUTED from label embedding × tag identity embedding
//     (rule scores never enter the distribution);
//   - a label missing its OWN embedding → insufficient (samples=0); it does
//     NOT count into vectorMissing (that counter is tag-side only);
//   - a tag missing its identity embedding is skipped for every label and
//     counted once into vectorMissing (never zero-filled);
//   - samples < MinSampleSize → insufficient, distribution fields stay null;
//   - the low-sim line is RELATIVE: P25 of the sufficient labels' cosine
//     means within this board. A fixed global line misfires on label×tag
//     cosine — measured in-board minimum is 0.51+ (same-domain text
//     embeddings carry a high floor), so a fixed 0.5 line yields a permanent
//     0% low-sim share (2026-10-08 real-data finding, design D4).
func computeLabelAttribution(rows []matchedRow, compositions []tagmodels.BoardHealthLabelMetrics, auxEmb map[uint][]float64, tagEmb map[uint][]float64, config Config) ([]tagmodels.BoardHealthLabelMetrics, int) {
	// Tag-side vectors; missing ones counted once, skipped for all labels.
	vectors := make([][]float64, 0, len(rows))
	vectorMissing := 0
	for _, row := range rows {
		if v, ok := tagEmb[row.TopicTagID]; ok {
			vectors = append(vectors, v)
		} else {
			vectorMissing++
		}
	}

	// Phase 1: per-label distribution over every vector-bearing tag.
	simsByAux := make(map[uint][]float64, len(compositions))
	for i := range compositions {
		entry := &compositions[i]
		labelVector, has := auxEmb[entry.AuxID]
		if !has {
			entry.Samples = 0
			entry.Insufficient = true
			continue
		}
		sims := make([]float64, 0, len(vectors))
		for _, tv := range vectors {
			sims = append(sims, cosineSim(labelVector, tv))
		}
		entry.Samples = len(sims)
		if entry.Samples < config.MinSampleSize {
			entry.Insufficient = true
			continue
		}
		mean, median, p25, p75 := cosineDistribution(sims)
		entry.CosineMean = &mean
		entry.CosineMedian = &median
		entry.CosineP25 = &p25
		entry.CosineP75 = &p75
		simsByAux[entry.AuxID] = sims // sorted in place by cosineDistribution
	}

	// Phase 2: relative low-sim line = P25 of sufficient labels' means.
	if len(simsByAux) == 0 {
		return compositions, vectorMissing
	}
	means := make([]float64, 0, len(simsByAux))
	for i := range compositions {
		if compositions[i].CosineMean != nil {
			means = append(means, *compositions[i].CosineMean)
		}
	}
	sort.Float64s(means)
	line := quantileSorted(means, 0.25)
	for i := range compositions {
		entry := &compositions[i]
		if entry.Insufficient {
			continue
		}
		lowRatio := lowShare(simsByAux[entry.AuxID], line)
		entry.LowSimRatio = &lowRatio
	}
	return compositions, vectorMissing
}

// cosineDistribution computes mean/median/p25/p75 over the samples, sorting
// them in place (the sorted slice feeds lowShare afterwards).
func cosineDistribution(samples []float64) (mean, median, p25, p75 float64) {
	if len(samples) == 0 {
		return 0, 0, 0, 0
	}
	sort.Float64s(samples)
	sum := 0.0
	for _, s := range samples {
		sum += s
	}
	mean = sum / float64(len(samples))
	median = quantileSorted(samples, 0.5)
	p25 = quantileSorted(samples, 0.25)
	p75 = quantileSorted(samples, 0.75)
	return mean, median, p25, p75
}

// lowShare returns the fraction of ascending-sorted samples strictly below
// the low-sim line.
func lowShare(sorted []float64, line float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	low := 0
	for _, s := range sorted {
		if s < line {
			low++
		}
	}
	return float64(low) / float64(len(sorted))
}

// quantileSorted returns the linear-interpolated q-quantile of an ascending
// slice (q ∈ [0,1]).
func quantileSorted(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	pos := q * float64(n-1)
	lower := int(pos)
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}
	frac := pos - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
