package board

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
)

// 版块扩充方向（design D3/D4）：用户锁定单版块触发（spec board-upgrade-expand）。
// 候选召回（相似路 + 共现路）→ 版块画像 prompt → LLM 二分类裁决 →
// merge_into_existing（单标签）/ compose+target（组合）。target 由服务端注入，
// LLM 输出不含目标字段——从根上杜绝缺 target/off-target 兜底逻辑复活。

// expandRecallLimit 是扩充召回每路（相似/共现）的候选上限（design D3）。
const expandRecallLimit = 40

// expandRecentSectionLimit 是版块画像注入的近期 section 标题上限（design D4 ≤8）。
const expandRecentSectionLimit = 8

// boardExpandProfile is the full picture of the expand target board (design D4).
type boardExpandProfile struct {
	BoardID          uint
	BoardLabel       string
	BoardDescription string
	Embedding        []float64
	// Composition 是版块现有构成（aux 与 composite 混合，prompt 标注类型）。
	Composition []expandCompositionEntry
	// compositionIDs 是构成的标签 ID 集（组合路相关性过滤用）。
	compositionIDs map[uint]struct{}
	// RecentSections 是近期匹配的 section 标题（≤8，查询失败降级为空）。
	RecentSections []string
}

type expandCompositionEntry struct {
	Label     string
	LabelType string
}

// expandAuxCandidate is one recalled aux candidate for expand×aux, carrying
// its evidence for prompt injection and post-LLM evidence persistence.
type expandAuxCandidate struct {
	ID       uint
	Label    string
	RefCount int
	SimDist  *float64 // 相似路证据：与版块 embedding 的余弦距离（共现路召回时为 nil）
	Cooccur  int      // 共现路证据：与版块构成标签的同文章共现次数
}

// generateExpandSuggestions 是扩充方向分发：expand×aux（merge 二分类）与
// expand×composite（compose+target 二分类）。days 在扩充路忽略（共现走
// CoTagWindowDays、相似是全库语义，spec: 扩充候选召回）。
func (s *SemanticBoardUpgradeService) generateExpandSuggestions(ctx context.Context, config SemanticBoardUpgradeConfig, req UpgradeGenerateRequest) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
	profile, err := s.loadBoardExpandProfile(ctx, req.TargetBoardID)
	if err != nil {
		return nil, nil, err
	}
	if req.Source == UpgradeSourceComposite {
		suggestions, err := s.generateExpandCompose(ctx, config, profile)
		if err != nil {
			return nil, nil, err
		}
		return suggestions, nil, nil
	}

	candidates, validIDs, err := s.recallExpandAuxCandidates(ctx, config, profile)
	if err != nil {
		return nil, nil, err
	}
	if len(candidates) == 0 {
		// 召回为空：正常空结果（spec: 召回为空），不报错。
		return []SemanticBoardUpgradeSuggestion{}, nil, nil
	}
	raw, err := s.llm.SuggestSemanticBoardUpgrades(ctx, buildExpandAuxPrompt(profile, candidates), "expand:aux")
	if err != nil {
		return nil, nil, err
	}
	target := req.TargetBoardID
	suggestions := filterExpandMergeSuggestions(raw, validIDs, target)
	for i := range suggestions {
		if suggestions[i].Confidence == "" {
			suggestions[i].Confidence = "llm"
		}
	}
	return suggestions, nil, nil
}

// loadBoardExpandProfile loads the target board's full picture: label,
// description, embedding, composition (aux + composite mixed) and ≤8 recent
// section titles. Recent-section query failure degrades to empty (不阻断生成).
func (s *SemanticBoardUpgradeService) loadBoardExpandProfile(ctx context.Context, boardID uint) (*boardExpandProfile, error) {
	var board models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Where("id = ? AND label_type = ? AND status = ?", boardID, "board", "active").
		First(&board).Error; err != nil {
		return nil, fmt.Errorf("active target board not found: %w", err)
	}
	profile := &boardExpandProfile{BoardID: board.ID, BoardLabel: board.Label, BoardDescription: board.Description}
	if board.Embedding != nil {
		if vector, err := auxlabel.ParsePgVector(*board.Embedding); err == nil {
			profile.Embedding = vector
		}
	}

	var comps []models.SemanticLabel
	if err := s.db.WithContext(ctx).
		Table("board_composition").
		Select("semantic_labels.*").
		Joins("JOIN semantic_labels ON semantic_labels.id = board_composition.auxiliary_label_id AND semantic_labels.status = ?", "active").
		Where("board_composition.board_id = ?", boardID).
		Order("semantic_labels.id ASC").
		Scan(&comps).Error; err != nil {
		return nil, err
	}
	profile.compositionIDs = make(map[uint]struct{}, len(comps))
	for _, comp := range comps {
		profile.Composition = append(profile.Composition, expandCompositionEntry{Label: comp.Label, LabelType: comp.LabelType})
		profile.compositionIDs[comp.ID] = struct{}{}
	}

	profile.RecentSections = s.loadBoardRecentSections(ctx, boardID)
	return profile, nil
}

// loadBoardRecentSections returns ≤8 recent section titles for the board's
// active topics (lane-brief query, per-board variant). Failure degrades to
// empty（画像降级为名称+描述+构成，不阻断扩充生成）.
func (s *SemanticBoardUpgradeService) loadBoardRecentSections(ctx context.Context, boardID uint) []string {
	cutoff := time.Now().AddDate(0, 0, -30)
	var rows []struct {
		SectionLabel string
	}
	err := s.db.WithContext(ctx).Raw(`
		SELECT s.cluster_label AS section_label
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		JOIN board_persistent_topics t ON t.id = s.persistent_topic_id
		WHERE r.semantic_board_id = ? AND t.status = ? AND s.cluster_label <> ''
		  AND r.period_date >= ?
		ORDER BY r.period_date DESC
	`, boardID, "active", cutoff).Scan(&rows).Error
	if err != nil {
		logging.Warnf("[semantic-board-expand] recent sections query failed, degrading to name+description only: %v", err)
		return nil
	}
	titles := make([]string, 0, len(rows))
	for _, r := range rows {
		if t := strings.TrimSpace(r.SectionLabel); t != "" {
			titles = append(titles, t)
		}
		if len(titles) >= expandRecentSectionLimit {
			break
		}
	}
	return titles
}

// recallExpandAuxCandidates 召回扩充×单标签候选（design D3）：相似路（与版块
// embedding 余弦距离 ≤ ExpandSimDistance）+ 共现路（与版块构成标签在
// CoTagWindowDays 窗口内同文章共现 ≥ ExpandCooccurrence），两路并集去重、
// 排除已挂载进目标版块与 disabled 的 aux，各路上限 expandRecallLimit。
// 返回排序候选 + 有效 ID 集（LLM 输出过滤用）。
func (s *SemanticBoardUpgradeService) recallExpandAuxCandidates(ctx context.Context, config SemanticBoardUpgradeConfig, profile *boardExpandProfile) ([]expandAuxCandidate, map[uint]struct{}, error) {
	compositionIDs := make([]uint, 0, len(profile.Composition))
	if err := s.db.WithContext(ctx).
		Table("board_composition").
		Where("board_composition.board_id = ?", profile.BoardID).
		Pluck("board_composition.auxiliary_label_id", &compositionIDs).Error; err != nil {
		return nil, nil, err
	}
	mounted := make(map[uint]struct{}, len(compositionIDs))
	for _, id := range compositionIDs {
		mounted[id] = struct{}{}
	}

	// ── 相似路：active aux（不在目标版块构成中）与版块 embedding 的距离 ──
	byID := make(map[uint]*expandAuxCandidate)
	if len(profile.Embedding) > 0 {
		var labels []models.SemanticLabel
		if err := s.db.WithContext(ctx).
			Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "auxiliary", "active").
			Order("id ASC").
			Find(&labels).Error; err != nil {
			return nil, nil, err
		}
		type simHit struct {
			candidate expandAuxCandidate
			dist      float64
		}
		var simHits []simHit
		for _, label := range labels {
			if _, mountedAlready := mounted[label.ID]; mountedAlready {
				continue
			}
			vector, err := auxlabel.ParsePgVector(*label.Embedding)
			if err != nil || len(vector) == 0 {
				continue
			}
			dist := semanticBoardUpgradeDistance(vector, profile.Embedding)
			if dist <= config.ExpandSimDistance {
				simHits = append(simHits, simHit{candidate: expandAuxCandidate{ID: label.ID, Label: label.Label, RefCount: label.RefCount}, dist: dist})
			}
		}
		// 距离升序截断上限（design D3：相似路优先）。
		sort.SliceStable(simHits, func(i, j int) bool { return simHits[i].dist < simHits[j].dist })
		if len(simHits) > expandRecallLimit {
			simHits = simHits[:expandRecallLimit]
		}
		for _, hit := range simHits {
			d := hit.dist
			hit.candidate.SimDist = &d
			copied := hit.candidate
			byID[hit.candidate.ID] = &copied
		}
	}

	// ── 共现路：与版块构成标签同文章共现的 aux ──
	if len(compositionIDs) > 0 {
		pairs, err := s.loadExpandCooccurrence(ctx, config, compositionIDs, mounted)
		if err != nil {
			return nil, nil, err
		}
		sort.SliceStable(pairs, func(i, j int) bool {
			if pairs[i].Cooccur != pairs[j].Cooccur {
				return pairs[i].Cooccur > pairs[j].Cooccur
			}
			return pairs[i].ID < pairs[j].ID
		})
		limit := expandRecallLimit
		for _, pair := range pairs {
			if limit <= 0 {
				break
			}
			if existing, seen := byID[pair.ID]; seen {
				// 并集去重：共现证据补充到既有候选（相似路优先保留）。
				existing.Cooccur = pair.Cooccur
				continue
			}
			copied := pair
			byID[pair.ID] = &copied
			limit--
		}
	}

	candidates := make([]expandAuxCandidate, 0, len(byID))
	for _, c := range byID {
		candidates = append(candidates, *c)
	}
	// 送裁排序：相似距离升序（无距离的排后），稳定可复现。
	sort.SliceStable(candidates, func(i, j int) bool {
		di, dj := 1.0, 1.0
		if candidates[i].SimDist != nil {
			di = *candidates[i].SimDist
		}
		if candidates[j].SimDist != nil {
			dj = *candidates[j].SimDist
		}
		if di != dj {
			return di < dj
		}
		return candidates[i].ID < candidates[j].ID
	})
	validIDs := make(map[uint]struct{}, len(candidates))
	for _, c := range candidates {
		validIDs[c.ID] = struct{}{}
	}
	return candidates, validIDs, nil
}

// loadExpandCooccurrence counts same-article co-occurrences between each
// un-mounted active aux and the board composition set within the co-tag
// window（复用 compose 段的文章→aux 映射查询语义）.
func (s *SemanticBoardUpgradeService) loadExpandCooccurrence(ctx context.Context, config SemanticBoardUpgradeConfig, compositionIDs []uint, mounted map[uint]struct{}) ([]expandAuxCandidate, error) {
	cutoff := time.Now().AddDate(0, 0, -config.CoTagWindowDays)
	compositionSet := make(map[uint]struct{}, len(compositionIDs))
	for _, id := range compositionIDs {
		compositionSet[id] = struct{}{}
	}

	var rows []struct {
		ArticleID uint `gorm:"column:article_id"`
		AuxID     uint `gorm:"column:aux_id"`
	}
	err := s.db.WithContext(ctx).
		Table("article_topic_tags AS att").
		Select("DISTINCT att.article_id AS article_id, ttsl.semantic_label_id AS aux_id").
		Joins("JOIN topic_tags AS tag ON tag.id = att.topic_tag_id AND tag.status = ?", "active").
		Joins("JOIN topic_tag_semantic_labels AS ttsl ON ttsl.topic_tag_id = att.topic_tag_id").
		Joins("JOIN semantic_labels AS aux ON aux.id = ttsl.semantic_label_id AND aux.label_type = ? AND aux.status = ?", "auxiliary", "active").
		Joins("JOIN articles AS article ON article.id = att.article_id AND article.created_at >= ?", cutoff).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	articleHasComposition := make(map[uint]bool, len(rows))
	for _, row := range rows {
		if _, inComposition := compositionSet[row.AuxID]; inComposition {
			articleHasComposition[row.ArticleID] = true
		}
	}
	counts := make(map[uint]int)
	for _, row := range rows {
		if _, mountedAlready := mounted[row.AuxID]; mountedAlready {
			continue
		}
		if _, inComposition := compositionSet[row.AuxID]; inComposition {
			continue
		}
		if articleHasComposition[row.ArticleID] {
			counts[row.AuxID]++
		}
	}

	minCooccur := config.ExpandCooccurrence
	if minCooccur <= 0 {
		minCooccur = 1
	}
	var result []expandAuxCandidate
	for id, count := range counts {
		if count >= minCooccur {
			result = append(result, expandAuxCandidate{ID: id, Cooccur: count})
		}
	}
	// 补 label/ref_count（counts 只带 ID）。
	if len(result) > 0 {
		ids := make([]uint, 0, len(result))
		for _, r := range result {
			ids = append(ids, r.ID)
		}
		var labels []models.SemanticLabel
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Select("id, label, ref_count").Find(&labels).Error; err != nil {
			return nil, err
		}
		info := make(map[uint]models.SemanticLabel, len(labels))
		for _, l := range labels {
			info[l.ID] = l
		}
		for i := range result {
			if l, ok := info[result[i].ID]; ok {
				result[i].Label = l.Label
				result[i].RefCount = l.RefCount
			}
		}
	}
	return result, nil
}

// generateExpandCompose 是扩充×组合路（design D3 组合路过滤）：compose 候选中
// 至少一组件 ∈ 召回集 ∪ 版块构成集，LLM 裁决「值得组合且属于目标版块」，
// 输出 compose 建议（服务端注入 target，spec: 扩充方向的组合建议携带目标）。
func (s *SemanticBoardUpgradeService) generateExpandCompose(ctx context.Context, config SemanticBoardUpgradeConfig, profile *boardExpandProfile) ([]SemanticBoardUpgradeSuggestion, error) {
	// 召回集复用单标签路（相关性判据同源），失败即空（构成集兜底）。
	recalled, _, err := s.recallExpandAuxCandidates(ctx, config, profile)
	if err != nil {
		logging.Warnf("[semantic-board-expand] composite recall failed, falling back to composition-only filter: %v", err)
		recalled = nil
	}
	related := make(map[uint]struct{})
	for _, c := range recalled {
		related[c.ID] = struct{}{}
	}
	for id := range profile.compositionIDSet() {
		related[id] = struct{}{}
	}

	candidates, err := s.collectComposeCandidates(ctx, config)
	if err != nil {
		return nil, err
	}
	labels, err := s.loadComponentLabels(ctx, collectComposeCandidateIDs(candidates))
	if err != nil {
		return nil, err
	}
	filteredCandidates := make([]ComposeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		relatedEnough := false
		for _, id := range candidate.ComponentIDs {
			if _, ok := related[id]; ok {
				relatedEnough = true
				break
			}
		}
		if relatedEnough {
			filteredCandidates = append(filteredCandidates, candidate)
		}
	}
	if len(filteredCandidates) == 0 {
		return []SemanticBoardUpgradeSuggestion{}, nil
	}
	// 排除已存在且已挂载目标版块的组合（防重复建议；未挂载的保留——挂载语义，
	// 确认时复用既有组合）。create/create×composite 不走此路（见 generateComposeSuggestions）。
	expandTarget := profile.BoardID
	filteredCandidates, err = s.filterExistingComposeCandidates(ctx, filteredCandidates, &expandTarget)
	if err != nil {
		return nil, err
	}
	if len(filteredCandidates) == 0 {
		return []SemanticBoardUpgradeSuggestion{}, nil
	}

	raw, err := s.llm.SuggestSemanticBoardUpgrades(ctx, buildExpandCompositePrompt(profile, filteredCandidates, labels), "expand:composite")
	if err != nil {
		return nil, err
	}
	componentUniverse := make(map[uint]struct{})
	for _, candidate := range filteredCandidates {
		for _, id := range candidate.ComponentIDs {
			componentUniverse[id] = struct{}{}
		}
	}
	target := profile.BoardID
	suggestions := filterExpandComposeSuggestions(raw, componentUniverse, target, filteredCandidates)
	return suggestions, nil
}

// compositionIDSet returns the board composition id set (prompt-filter helper).
func (p *boardExpandProfile) compositionIDSet() map[uint]struct{} {
	if p.compositionIDs == nil {
		return map[uint]struct{}{}
	}
	return p.compositionIDs
}

// filterExpandMergeSuggestions 过滤扩充×单标签 LLM 输出：只保留 merge_into_existing
// 决策（越权 create/compose/skip 丢弃），ids 过滤到召回集内且非空，target 注入锁定版块。
func filterExpandMergeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, validIDs map[uint]struct{}, targetBoardID uint) []SemanticBoardUpgradeSuggestion {
	filtered := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Decision != SemanticBoardUpgradeDecisionMergeIntoExisting {
			continue
		}
		ids := filterKnownAuxiliaryIDs(suggestion.AuxiliaryLabelIDs, validIDs)
		if len(ids) == 0 {
			continue
		}
		suggestion.AuxiliaryLabelIDs = ids
		target := targetBoardID
		suggestion.TargetBoardID = &target
		if suggestion.Evidence == nil {
			suggestion.Evidence = map[string]any{}
		}
		suggestion.Evidence["source"] = "expand"
		filtered = append(filtered, suggestion)
	}
	return filtered
}

// filterExpandComposeSuggestions 过滤扩充×组合 LLM 输出：只保留 compose 决策，
// 组件过滤到候选宇宙内且数量合法，target 注入锁定版块，证据带共现频次。
func filterExpandComposeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, componentUniverse map[uint]struct{}, targetBoardID uint, candidates []ComposeCandidate) []SemanticBoardUpgradeSuggestion {
	candidateByComponents := make(map[string]ComposeCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByComponents[composeComponentKey(candidate.ComponentIDs)] = candidate
	}
	filtered := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Decision != SemanticBoardUpgradeDecisionCompose {
			continue
		}
		ids := filterKnownAuxiliaryIDs(suggestion.AuxiliaryLabelIDs, componentUniverse)
		if len(ids) < 2 {
			continue
		}
		suggestion.AuxiliaryLabelIDs = ids
		target := targetBoardID
		suggestion.TargetBoardID = &target
		if suggestion.Evidence == nil {
			suggestion.Evidence = map[string]any{}
		}
		suggestion.Evidence["source"] = "expand_compose"
		if candidate, ok := candidateByComponents[composeComponentKey(ids)]; ok {
			suggestion.Evidence["compose_cooccurrence"] = candidate.Cooccurrence
		}
		filtered = append(filtered, suggestion)
	}
	return filtered
}

// collectComposeCandidateIDs flattens candidate component ids (label lookup).
func collectComposeCandidateIDs(candidates []ComposeCandidate) []uint {
	seen := make(map[uint]struct{})
	ids := make([]uint, 0)
	for _, candidate := range candidates {
		for _, id := range candidate.ComponentIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids
}
