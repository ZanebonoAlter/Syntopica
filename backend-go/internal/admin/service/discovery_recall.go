package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── improve-discovery-recommendations 4.3：有保底的双路召回（design D4）──
//
// 每轮 refresh 三路召回：
//   - 版块基础路：每个有兼容向量的 active 真实版块（label_type='board'）以自身
//     label embedding 对候选向量取 top-8；资格过滤在截 top-8 之前，被过滤后不足
//     8 就 <8、不凑满。每版块独立 8 个名额，不跨版块争抢、也不被行为路挤占
//     （保底=进入精排，LLM 仍可全不选）。
//   - 行为路：每个有 behavior 向量（preference_vectors source=behavior）的版块
//     top-8，同资格过滤；全局桶（board_id=NULL）单独成批、不冒充版块
//     （批次 boardLabel=全局、board_id 保持 NULL）。
//   - seed 路：discovery_interest_entries 参与集 + 配额（4.2 成果复用），受限补充。
//
// 版块基础路与该版块行为路合并为单批（候选级去重、来源徽标进精排上下文，同候选
// 只进精排集合一次）；seed 批独立（上下文含查询文本摘要）。任何批次上限
// recallBatchMaxCandidates（基础8+行为8+受限seed=20）。ask 直接以查询向量取 20 单批。
//
// 候选向量存储 = candidate_embeddings（D6 统一候选向量；route_embeddings 仅作迁移
// 输入/回滚资料，新召回不再读取）。画像/查询 embedding 与候选向量必须同 model 同维
// （画像来自 PreferenceVector.Model / DiscoveryInterestEntry.Model；查询来自 embedding
// result.Model；候选来自 CandidateEmbedding.Model）——不一致整轮失败（configuration
// 失败码），不混算不发布。semantic_labels 无 model 列无从比对：版块 label 向量按
// dimension-only 校验（dimension 与候选向量一致才参与），候选/画像路严格双检。

// 召回路径键（recall_sources 的键；ask 用 query 键）。
const (
	recallPathBoard    = "board"    // 版块基础路（版块自身 label embedding）
	recallPathBehavior = "behavior" // 行为路（preference_vectors source=behavior）
	recallPathSeed     = "seed"     // seed 路（discovery_interest_entries 参与集）
	recallPathQuery    = "query"    // ask 查询向量直查
)

// recallBatchMaxCandidates 单批精排候选上限（design D4：基础8+行为8+受限seed=20）。
const recallBatchMaxCandidates = 20

// askRecallTopN 是 ask 直查的 top-N（design D4：问答直接以查询向量取 20 个）。
const askRecallTopN = 20

// errRecallModelMismatch 标记向量 model/dimension 不兼容（混算阻断；调用方映射为
// configuration 失败码——spec 错误码家族的兼容性错误）。
var errRecallModelMismatch = errors.New("recall vector model mismatch")

// recallBatch 是一次精排的输入批次。
// 版块批（path=board）= 基础路 ± 该版块行为路合并（候选带来源徽标）；行为批
// （path=behavior）为全局桶或无 label 向量版块的独立批；seed 批（path=seed）为
// 单条兴趣条目的受限名额批；ask 批（path=query）为查询向量直查批。
// tagSummary 仅是行为证据、不声明长期兴趣。
type recallBatch struct {
	boardID    *uint
	boardLabel string // 版块名；全局桶固定「全局」
	tagSummary string // 行为标签摘要（含行为路的批次证据上下文）
	queryText  string // ask 查询原文 / seed 批的兴趣条目查询原文
	source     string // RecommendationSourceManualRefresh | RecommendationSourceQA
	path       string // recallPathBoard | recallPathBehavior | recallPathSeed | recallPathQuery
	candidates []recallCandidate
}

// recallCandidate 是带召回来源徽标的候选（同候选跨路去重后保留全部命中路径）。
type recallCandidate struct {
	candidateRow
	paths []string // 命中路径（board/behavior/seed/query），进精排上下文与 recall_sources
}

// recallBatcher 产出精排批次（pgvector 实现与测试假实现共享契约）。
type recallBatcher interface {
	askBatches(ctx context.Context, query string, vec []float64, dim int, model string) ([]recallBatch, error)
	refreshBatches(ctx context.Context) ([]recallBatch, error)
}

// pgRecallBatcher 生产召回：candidate_embeddings × feed_candidates × rsshub_routes。
type pgRecallBatcher struct {
	recSvc *RecommendationService
}

// recallVectorRef 是本轮画像侧的 (model, dimension) 参考。
type recallVectorRef struct {
	model string
	dim   int
}

// behaviorProfile 是已解析的行为画像（版块桶或全局桶）。
type behaviorProfile struct {
	boardID    *uint
	boardLabel string // 全局桶固定「全局」
	vec        []float64
	model      string
	tagSummary string
}

// askBatches 问答召回：单批、全局桶（board_id=NULL）、top-20 直查。
// 查询 embedding 的 (model,dim) 与候选向量全表校验，不一致报 errRecallModelMismatch。
func (p *pgRecallBatcher) askBatches(ctx context.Context, query string, vec []float64, dim int, model string) ([]recallBatch, error) {
	ref := recallVectorRef{model: model, dim: dim}
	if err := p.checkCandidateVectorConsistency(ctx, &ref); err != nil {
		return nil, err
	}
	rows, err := p.searchCandidates(ctx, vec, dim, model, nil, askRecallTopN)
	if err != nil {
		return nil, err
	}
	return []recallBatch{{
		boardLabel: "全局", queryText: query, source: RecommendationSourceQA,
		path: recallPathQuery, candidates: wrapCandidates(rows, recallPathQuery),
	}}, nil
}

// refreshBatches 刷新召回：全局行为批 + 版块批（基础路±行为路合并）+ 版块行为
// 独立批（label 无向量/维度不兼容的版块）+ seed 批。同模型校验先行。
func (p *pgRecallBatcher) refreshBatches(ctx context.Context) ([]recallBatch, error) {
	behaviors, err := p.loadBehaviorProfiles(ctx)
	if err != nil {
		return nil, err
	}
	seedItems, err := p.loadSeedRecallPlan(ctx)
	if err != nil {
		return nil, err
	}

	// 同模型校验：画像侧（行为 + 参与 seed）内部一致 → 候选向量全表与参考一致。
	refs := make([]recallVectorRef, 0, len(behaviors.boards)+len(seedItems)+1)
	if behaviors.global != nil {
		refs = append(refs, recallVectorRef{model: behaviors.global.model, dim: len(behaviors.global.vec)})
	}
	for _, id := range behaviors.sortedBoardIDs() {
		b := behaviors.boards[id]
		refs = append(refs, recallVectorRef{model: b.model, dim: len(b.vec)})
	}
	for _, it := range seedItems {
		refs = append(refs, recallVectorRef{model: it.model, dim: it.dim})
	}
	ref, err := singleRecallReference(refs)
	if err != nil {
		return nil, err
	}
	if err := p.checkCandidateVectorConsistency(ctx, ref); err != nil {
		return nil, err
	}

	var batches []recallBatch

	// 1. 全局行为批：board_id=NULL 单独成批，不冒充版块。
	if global := behaviors.global; global != nil {
		rows, serr := p.searchCandidates(ctx, global.vec, len(global.vec), global.model, nil, RecommendationTopNDefault)
		if serr != nil {
			return nil, serr
		}
		if len(rows) > 0 {
			batches = append(batches, recallBatch{
				boardID: nil, boardLabel: "全局", tagSummary: global.tagSummary,
				source: RecommendationSourceManualRefresh, path: recallPathBehavior,
				candidates: wrapCandidates(rows, recallPathBehavior),
			})
		}
	}

	// 2. 版块批：基础路 top-8 + 该版块行为路 top-8 合并去重（基础路保底、每版块
	//    独立名额不跨版块争抢；cap 20）。有合格候选即成批，欠额不凑满。
	boardIDs := behaviors.sortedBoardIDs()
	covered := make(map[uint]struct{}, len(boardIDs))
	for _, b := range p.loadBoardLabels(ctx) {
		dim, model := len(b.vec), ""
		if ref != nil {
			if ref.dim != len(b.vec) {
				continue // semantic_labels 无 model 列：版块 label 向量 dimension-only 校验
			}
			dim, model = ref.dim, ref.model
		}
		covered[b.id] = struct{}{}
		base, cerr := p.searchCandidates(ctx, b.vec, dim, model, &b.id, RecommendationTopNDefault)
		if cerr != nil {
			return nil, cerr
		}
		var behaviorRows []candidateRow
		var tagSummary string
		if bh := behaviors.boards[b.id]; bh != nil {
			behaviorRows, cerr = p.searchCandidates(ctx, bh.vec, dim, model, &b.id, RecommendationTopNDefault)
			if cerr != nil {
				return nil, cerr
			}
			tagSummary = bh.tagSummary
		}
		merged := mergePathCandidates(base, behaviorRows)
		if len(merged) == 0 {
			continue
		}
		batch := recallBatch{
			boardID: &b.id, boardLabel: b.label, tagSummary: tagSummary,
			source: RecommendationSourceManualRefresh, path: recallPathBoard, candidates: merged,
		}
		batches = append(batches, batch)
	}

	// 3. 版块行为独立批：有 behavior 画像但 label 无向量/维度不兼容的版块——
	//    单路缺失不伪造候选，行为路照常参与。
	for _, id := range boardIDs {
		if _, ok := covered[id]; ok {
			continue
		}
		bh := behaviors.boards[id]
		rows, serr := p.searchCandidates(ctx, bh.vec, len(bh.vec), bh.model, bh.boardID, RecommendationTopNDefault)
		if serr != nil {
			return nil, serr
		}
		if len(rows) == 0 {
			continue
		}
		batches = append(batches, recallBatch{
			boardID: bh.boardID, boardLabel: bh.boardLabel, tagSummary: bh.tagSummary,
			source: RecommendationSourceManualRefresh, path: recallPathBehavior,
			candidates: wrapCandidates(rows, recallPathBehavior),
		})
	}

	// 4. seed 批：每条参与条目以其 embedding 粗筛 top-名额（一批进入精排，
	//    上下文含查询文本摘要；名额未用完不转赠其他条目）。
	for _, it := range seedItems {
		rows, serr := p.searchCandidates(ctx, it.vec, it.dim, it.model, it.boardID, it.share)
		if serr != nil {
			return nil, serr
		}
		if len(rows) == 0 {
			continue
		}
		batches = append(batches, recallBatch{
			boardID: it.boardID, boardLabel: it.boardLabel, queryText: it.queryText,
			source: RecommendationSourceManualRefresh, path: recallPathSeed,
			candidates: wrapCandidates(rows, recallPathSeed),
		})
	}
	return batches, nil
}

// behaviorProfileSet 行为画像集合（全局桶 + 版块桶）。
type behaviorProfileSet struct {
	global *behaviorProfile
	boards map[uint]*behaviorProfile
}

// sortedBoardIDs 版块 id 升序（批次顺序稳定）。
func (s *behaviorProfileSet) sortedBoardIDs() []uint {
	ids := make([]uint, 0, len(s.boards))
	for id := range s.boards {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// loadBehaviorProfiles 装载 source=behavior 的偏好画像：版块桶需有效 active 真实
// 版块名（label_type='board'，无有效名/向量不可解析的版块桶跳过）；全局桶保留。
func (p *pgRecallBatcher) loadBehaviorProfiles(ctx context.Context) (*behaviorProfileSet, error) {
	type pvRow struct {
		BoardID      *uint
		EmbeddingVec string
		Model        string
		BoardLabel   *string
		TagWeights   models.MetadataMap
	}
	var pvs []pvRow
	if err := p.recSvc.db.WithContext(ctx).Raw(`
		SELECT pv.board_id, pv.embedding AS embedding_vec, pv.model,
		       sl.label AS board_label, pv.tag_weights
		FROM preference_vectors pv
		LEFT JOIN semantic_labels sl
		       ON sl.id = pv.board_id AND sl.label_type = 'board' AND sl.status = 'active'
		WHERE pv.source = 'behavior'
		ORDER BY pv.board_id ASC NULLS FIRST, pv.id ASC`).Scan(&pvs).Error; err != nil {
		return nil, err
	}
	out := &behaviorProfileSet{boards: map[uint]*behaviorProfile{}}
	for _, pv := range pvs {
		vec, err := parsePgVector(pv.EmbeddingVec)
		if err != nil || len(vec) == 0 {
			continue
		}
		if pv.BoardID == nil {
			out.global = &behaviorProfile{boardID: nil, boardLabel: "全局", vec: vec, model: pv.Model,
				tagSummary: formatTagSummary(pv.TagWeights)}
			continue
		}
		if pv.BoardLabel == nil || *pv.BoardLabel == "" {
			continue // 版块桶无有效版块名（版块已删/非 board 类型）：不参与本轮
		}
		out.boards[*pv.BoardID] = &behaviorProfile{boardID: pv.BoardID, boardLabel: *pv.BoardLabel,
			vec: vec, model: pv.Model, tagSummary: formatTagSummary(pv.TagWeights)}
	}
	return out, nil
}

// boardLabelVec 是可参与基础路的版块（label 向量已解析）。
type boardLabelVec struct {
	id    uint
	label string
	vec   []float64
}

// loadBoardLabels 装载 active 真实版块的 label 向量（label_type='board' 且 status
// =active、embedding 非空——历史 bug 教训：漏 label_type 过滤会把辅助/复合标签也
// 当版块）。维度与候选向量的一致性由调用方判定（dimension-only）。
func (p *pgRecallBatcher) loadBoardLabels(ctx context.Context) []boardLabelVec {
	type row struct {
		ID        uint
		Label     string
		Embedding *string
	}
	var rows []row
	if err := p.recSvc.db.WithContext(ctx).Raw(`
		SELECT id, label, embedding FROM semantic_labels
		WHERE label_type = 'board' AND status = 'active' AND embedding IS NOT NULL
		ORDER BY id ASC`).Scan(&rows).Error; err != nil {
		return nil // 装载失败按无版块处理（单路缺失不伪造候选），不阻断其他路
	}
	out := make([]boardLabelVec, 0, len(rows))
	for _, r := range rows {
		if r.Embedding == nil {
			continue
		}
		v, err := parsePgVector(*r.Embedding)
		if err != nil || len(v) == 0 {
			continue
		}
		out = append(out, boardLabelVec{id: r.ID, label: r.Label, vec: v})
	}
	return out
}

// seedRecallItem 是 seed 路的实际查询条目（share>0 且向量可解析；ID 升序稳定）。
type seedRecallItem struct {
	entryID    uint
	queryText  string
	boardID    *uint
	boardLabel string
	vec        []float64
	dim        int
	model      string
	share      int
}

// loadSeedRecallPlan 执行 4.2 seed 政策（超窗置 inactive → 参与集 → 行为成熟度 →
// 名额分配），返回 share>0 且向量可解析的条目。名额未用完不转赠（D3）。
func (p *pgRecallBatcher) loadSeedRecallPlan(ctx context.Context) ([]seedRecallItem, error) {
	db := p.recSvc.db
	cfg, err := loadSeedPolicyConfig(db)
	if err != nil {
		return nil, fmt.Errorf("seed policy config: %w", err)
	}
	now := time.Now()
	cutoff := now.AddDate(0, 0, -cfg.WindowDays)

	// 1. 超窗条目退出参与（status=inactive；与参与判据 now < created_at+window 对齐）。
	if err := db.WithContext(ctx).Model(&models.DiscoveryInterestEntry{}).
		Where("status = ? AND created_at <= ?", "active", cutoff).
		Update("status", "inactive").Error; err != nil {
		return nil, fmt.Errorf("expire stale interest entries: %w", err)
	}

	// 2. 装载窗口内 active 条目（DB 层先粗滤，纯函数再取前 K）。
	var rows []seedInterestRow
	if err := db.WithContext(ctx).Raw(`
		SELECT die.id, die.query_text, die.board_id, sl.label AS board_label,
		       die.embedding AS embedding_vec, die.dimension, die.model, die.status, die.created_at
		FROM discovery_interest_entries die
		LEFT JOIN semantic_labels sl ON sl.id = die.board_id
		WHERE die.status = 'active' AND die.created_at > ?
		ORDER BY die.id ASC`, cutoff).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load interest entries: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	byID := make(map[uint]seedInterestRow, len(rows))
	facts := make([]EntryFacts, 0, len(rows))
	for _, r := range rows {
		var boardID *uint64
		if r.BoardID != nil {
			b := uint64(*r.BoardID)
			boardID = &b
		}
		facts = append(facts, EntryFacts{ID: uint64(r.ID), Status: r.Status, BoardID: boardID, CreatedAt: r.CreatedAt})
		byID[r.ID] = r
	}
	parts := ParticipatingEntries(facts, now, cfg)

	// 3. 行为成熟度 N：窗口内每版块去重阅读文章数 + 全局去重数。
	boardN, globalN, err := behaviorArticleCounts(ctx, db, cutoff)
	if err != nil {
		return nil, fmt.Errorf("count behavior articles: %w", err)
	}

	// 4. 权重与名额（配额按 ID 升序消费，批次顺序稳定）。
	weighted := make([]WeightedEntry, 0, len(parts))
	for _, pe := range parts {
		if !pe.Participates {
			continue
		}
		n := globalN
		if pe.Entry.BoardID != nil {
			n = boardN[*pe.Entry.BoardID]
		}
		weighted = append(weighted, WeightedEntry{ID: pe.Entry.ID, Weight: InterestWeight(pe.Entry, n, now, cfg)})
	}
	shares := SeedShareAllocation(weighted, cfg)
	sorted := append([]WeightedEntry(nil), weighted...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	out := make([]seedRecallItem, 0, len(sorted))
	for _, we := range sorted {
		share := shares[we.ID]
		if share <= 0 {
			continue // 成熟让位/预算耗尽：条目保留显示，不产召回批次
		}
		row := byID[uint(we.ID)]
		vec, perr := parsePgVector(row.EmbeddingVec)
		if perr != nil || len(vec) == 0 || len(vec) != row.Dimension {
			continue // 向量不可解析/维度不符：本名额作废，不转赠（D3）
		}
		label := "全局"
		if row.BoardLabel != nil && *row.BoardLabel != "" {
			label = *row.BoardLabel
		}
		out = append(out, seedRecallItem{
			entryID: row.ID, queryText: row.QueryText, boardID: row.BoardID,
			boardLabel: label, vec: vec, dim: len(vec), model: row.Model, share: share,
		})
	}
	return out, nil
}

// seedInterestRow 兴趣条目的召回装载形态（含版块名，label 供精排上下文）。
type seedInterestRow struct {
	ID           uint
	QueryText    string
	BoardID      *uint
	BoardLabel   *string
	EmbeddingVec string
	Dimension    int
	Model        string
	Status       string
	CreatedAt    time.Time
}

// singleRecallReference 汇总画像侧 (model,dim)：必须全部一致，否则报
// errRecallModelMismatch（不混算）；空集返回 nil（仅版块路，dimension-only）。
func singleRecallReference(refs []recallVectorRef) (*recallVectorRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	first := refs[0]
	for _, r := range refs[1:] {
		if r.model != first.model || r.dim != first.dim {
			return nil, fmt.Errorf("%w: 画像侧向量不一致（%s/%d vs %s/%d）",
				errRecallModelMismatch, first.model, first.dim, r.model, r.dim)
		}
	}
	return &first, nil
}

// checkCandidateVectorConsistency 候选向量与画像参考的兼容性校验（Medium 4 / Low 12 修复）：
// 改为「按候选粒度」判断是否存在可用兼容向量——有兼容向量的候选正常参与检索；
// 兼容向量缺失的候选不计入本轮（记 skipped 日志），不再用全表不一致把整轮卡死。
// 仅当「存在候选向量行，但零个候选有兼容向量」时才整轮 configuration 失败（真正的模型
// 不兼容/未回补）。model 为 NULL 的行按不兼容跳过（COALESCE 后不等于参考）。
// ref 为 nil（无画像侧，仅版块路）不校验。
func (p *pgRecallBatcher) checkCandidateVectorConsistency(ctx context.Context, ref *recallVectorRef) error {
	if ref == nil {
		return nil
	}
	var stats struct {
		TotalRows            int64
		TotalCandidates      int64
		CompatibleCandidates int64
	}
	if err := p.recSvc.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS total_rows,
		       COUNT(DISTINCT candidate_id) AS total_candidates,
		       COUNT(DISTINCT CASE WHEN COALESCE(model, '') = ? AND dimension = ? THEN candidate_id END) AS compatible_candidates
		FROM candidate_embeddings`, ref.model, ref.dim).Scan(&stats).Error; err != nil {
		return err
	}
	if stats.TotalRows == 0 {
		return nil
	}
	if stats.CompatibleCandidates == 0 {
		return fmt.Errorf("%w: candidate_embeddings 有 %d 个候选、零个兼容 (model=%s, dimension=%d) 向量",
			errRecallModelMismatch, stats.TotalCandidates, ref.model, ref.dim)
	}
	if skipped := stats.TotalCandidates - stats.CompatibleCandidates; skipped > 0 {
		// 遗留/旧模型向量行不参与本轮检索（不混算），但不再阻断整轮。
		logging.Warnf("recall: %d candidate(s) skipped: no compatible embedding vector (model=%s dim=%d); %d candidate(s) participate",
			skipped, ref.model, ref.dim, stats.CompatibleCandidates)
	}
	return nil
}

// searchCandidates 以单向量检索候选（candidate_embeddings × feed_candidates，
// rsshub 候选 LEFT JOIN rsshub_routes；原生 RSS 候选经 CandidateEmbedding 直接参与）。
// 资格过滤全部在截 top-N 之前（design D4）：
//   - rsshub：route status gone/broken 排除、已 accepted 路由排除、直订路由按已订阅
//     feeds.url（base+example）去重；
//   - 原生 rss：candidate_availability.status=broken 排除（Medium 8；unknown/ok/
//     requires_parameters 不硬过滤），按已订阅 feeds.url（候选 feed_url）去重；
//   - 共同：recommendation_enabled=false、candidate_preferences 冷却/长期排除。
//
// unknown 可用性保留（可用性不进粗筛硬过滤）。model 为空 = dimension-only（版块路无
// 画像参考时）。
func (p *pgRecallBatcher) searchCandidates(ctx context.Context, vec []float64, dim int, model string, boardID *uint, limit int) ([]candidateRow, error) {
	if limit <= 0 || len(vec) == 0 || dim <= 0 {
		return nil, nil
	}
	if limit > recallBatchMaxCandidates {
		limit = recallBatchMaxCandidates // 批次上限 20（design D4）
	}
	vecStr := floatsToPgVector(vec)
	baseURL := resolveRSSHubBaseURL(p.recSvc.db)
	q := `
		SELECT ce.candidate_id AS candidate_id, COALESCE(fc.route_id, 0) AS route_id, fc.kind AS kind,
		       COALESCE(r.namespace, '') AS namespace, COALESCE(r.path, '') AS path,
		       COALESCE(r.name, '') AS name, COALESCE(r.description, '') AS description,
		       COALESCE(r.example, '') AS example,
		       COALESCE(r.usable_directly, TRUE) AS usable_directly,
		       COALESCE(r.requires_parameters, FALSE) AS requires_parameters,
		       COALESCE(r.parameters, '{}') AS parameters,
		       fc.manual_metadata AS manual_metadata, fc.feed_url AS feed_url,
		       (ce.embedding <=> ?::vector) AS distance
		FROM candidate_embeddings ce
		JOIN feed_candidates fc ON fc.id = ce.candidate_id
		LEFT JOIN rsshub_routes r ON r.id = fc.route_id
		LEFT JOIN candidate_availability ca ON ca.candidate_id = fc.id
		WHERE ce.dimension = ?`
	args := []any{vecStr, dim}
	if model != "" {
		q += ` AND ce.model = ?`
		args = append(args, model)
	}
	q += `
		  AND (fc.recommendation_enabled IS NULL OR fc.recommendation_enabled = TRUE)
		  AND (
		        (fc.kind = 'rsshub' AND r.status IS NOT NULL AND r.status NOT IN ('broken','gone')
		         AND r.id NOT IN (SELECT route_id FROM feed_recommendations WHERE status = 'accepted'))
		     OR (fc.kind = 'rss' AND (ca.status IS NULL OR ca.status <> 'broken'))
		  )
		  AND NOT EXISTS (
		        SELECT 1 FROM candidate_preferences cp
		        WHERE cp.candidate_id = fc.id
		          AND (cp.excluded_at IS NOT NULL
		               OR (cp.snoozed_until IS NOT NULL AND cp.snoozed_until > ?)))
		  AND (
		        (fc.kind = 'rsshub' AND (NOT COALESCE(r.usable_directly, TRUE) OR NOT EXISTS (
		             SELECT 1 FROM feeds f WHERE f.url = (?::text || r.example))))
		     OR (fc.kind = 'rss' AND (fc.feed_url IS NULL OR NOT EXISTS (
		             SELECT 1 FROM feeds f2 WHERE f2.url = fc.feed_url)))
		  )
		ORDER BY ce.embedding <=> ?::vector
		LIMIT ?`
	args = append(args, time.Now(), baseURL, vecStr, limit)
	type row struct {
		CandidateID        uint
		RouteID            uint
		Kind               string
		Namespace          string
		Path               string
		Name               string
		Description        string
		Example            string
		UsableDirectly     bool
		RequiresParameters bool
		Parameters         string
		ManualMetadata     models.MetadataMap
		FeedURL            *string
		Distance           float64
	}
	var rows []row
	if err := p.recSvc.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]candidateRow, 0, len(rows))
	for _, r := range rows {
		name, description, example := r.Name, r.Description, r.Example
		if r.Kind == "rss" {
			// 原生 RSS：有效展示字段来自人工元数据（无上游）；地址即订阅地址。
			eff := EffectiveMetadata(r.ManualMetadata, "", "", "", "")
			name, description = eff.Name, eff.Description
			if r.FeedURL != nil {
				example = *r.FeedURL
			}
			if strings.TrimSpace(name) == "" {
				name = example
			}
		}
		out = append(out, candidateRow{
			CandidateID: r.CandidateID, RouteID: r.RouteID, Kind: r.Kind,
			Namespace: r.Namespace, Path: r.Path, Name: name, Description: description,
			Example: example, UsableDirectly: r.UsableDirectly,
			RequiresParameters: r.RequiresParameters, Parameters: r.Parameters,
			BoardID: boardID, Distance: r.Distance,
		})
	}
	return out, nil
}

// mergePathCandidates 合并版块基础路与行为路候选：同候选（CandidateID）去重并保留双来源
// 徽标（只进精排集合一次），基础路在前（保底不被挤占）；总量截批次上限，欠额不凑满。
func mergePathCandidates(base, behavior []candidateRow) []recallCandidate {
	out := make([]recallCandidate, 0, len(base)+len(behavior))
	byCandidate := make(map[uint]int, len(base)+len(behavior))
	add := func(cs []candidateRow, path string) {
		for _, c := range cs {
			if idx, ok := byCandidate[c.CandidateID]; ok {
				out[idx].paths = appendPathUnique(out[idx].paths, path)
				continue
			}
			if len(out) >= recallBatchMaxCandidates {
				return // 批次上限 20：后续候选不再占用本批名额
			}
			byCandidate[c.CandidateID] = len(out)
			out = append(out, recallCandidate{candidateRow: c, paths: []string{path}})
		}
	}
	add(base, recallPathBoard)
	add(behavior, recallPathBehavior)
	return out
}

// wrapCandidates 单路候选包装为带来源徽标的精排输入。
func wrapCandidates(rows []candidateRow, path string) []recallCandidate {
	out := make([]recallCandidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, recallCandidate{candidateRow: r, paths: []string{path}})
	}
	return out
}

// appendPathUnique 追加来源路径（去重、序稳定）。
func appendPathUnique(paths []string, path string) []string {
	for _, x := range paths {
		if x == path {
			return paths
		}
	}
	return append(paths, path)
}

// candidateRecallSources 由批次 + 候选命中路径构造 recall_sources 快照：
// board=[版块名] / behavior=[版块名或全局] / seed=[查询文本] / query=[查询文本]。
// 跨批同候选由 rerankAndPublish 经 mergeRecallSources 合并，不冒称单路独占。
func candidateRecallSources(b recallBatch, c recallCandidate) models.MetadataMap {
	src := models.MetadataMap{}
	for _, path := range c.paths {
		switch path {
		case recallPathBoard:
			src["board"] = []string{b.boardLabel}
		case recallPathBehavior:
			src["behavior"] = []string{b.boardLabel}
		case recallPathSeed:
			src["seed"] = []string{truncateRunesSafe(b.queryText, 100)}
		case recallPathQuery:
			src["query"] = []string{truncateRunesSafe(b.queryText, 100)}
		}
	}
	return src
}

// recallPathBadges 来源徽标文案（进精排上下文，供 LLM 理解候选为何出现）。
func recallPathBadges(paths []string) string {
	if len(paths) == 0 {
		return "无"
	}
	labels := make([]string, 0, len(paths))
	for _, p := range paths {
		switch p {
		case recallPathBoard:
			labels = append(labels, "版块基础")
		case recallPathBehavior:
			labels = append(labels, "行为画像")
		case recallPathSeed:
			labels = append(labels, "历史查询")
		case recallPathQuery:
			labels = append(labels, "本次查询")
		default:
			labels = append(labels, p)
		}
	}
	return strings.Join(labels, "+")
}

// behaviorArticleCounts 统计窗口内有阅读行为的去重文章数：每版块（文章经
// article_topic_tags × active topic_tags × topic_tag_board_labels 归属）+ 全局。
// 口径对齐偏好画像 computeArticleWeights：reading_behaviors 全事件按文章去重
// （articleBehaviorLevel 恒 > 0，无需再按档位过滤）。
func behaviorArticleCounts(ctx context.Context, db *gorm.DB, cutoff time.Time) (map[uint64]int, int, error) {
	type boardCount struct {
		BoardID uint
		N       int
	}
	var boards []boardCount
	if err := db.WithContext(ctx).Raw(`
		SELECT tbl.semantic_board_id AS board_id, COUNT(DISTINCT rb.article_id) AS n
		FROM reading_behaviors rb
		JOIN article_topic_tags att ON att.article_id = rb.article_id
		JOIN topic_tags tt ON tt.id = att.topic_tag_id AND tt.status = 'active'
		JOIN topic_tag_board_labels tbl ON tbl.topic_tag_id = att.topic_tag_id
		WHERE rb.created_at >= ?
		GROUP BY tbl.semantic_board_id`, cutoff).Scan(&boards).Error; err != nil {
		return nil, 0, err
	}
	out := make(map[uint64]int, len(boards))
	for _, b := range boards {
		out[uint64(b.BoardID)] = b.N
	}
	var global struct {
		N int
	}
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(DISTINCT article_id) AS n
		FROM reading_behaviors
		WHERE created_at >= ?`, cutoff).Scan(&global).Error; err != nil {
		return nil, 0, err
	}
	return out, global.N, nil
}

// formatTagSummary 把 tag_weights 摘成「标签(权重)」逗号串（行为证据，非长期兴趣声明）。
func formatTagSummary(w models.MetadataMap) string {
	if len(w) == 0 {
		return ""
	}
	type tw struct {
		label string
		w     float64
	}
	items := make([]tw, 0, len(w))
	for k, v := range w {
		weight, _ := v.(float64)
		items = append(items, tw{label: k, w: weight})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].w != items[j].w {
			return items[i].w > items[j].w
		}
		return items[i].label < items[j].label
	})
	parts := make([]string, 0, len(items))
	for i, it := range items {
		if i >= 8 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%.2f)", it.label, it.w))
	}
	return strings.Join(parts, ", ")
}
