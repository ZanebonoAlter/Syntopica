package board

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"syntopica-backend/internal/models"
)

// SemanticBoardUpgradeDecisionCompose adjudicates a composite-label candidate
// (co-tag co-occurrence pair/triple) worth composing into one label.
const SemanticBoardUpgradeDecisionCompose SemanticBoardUpgradeDecision = "compose"

// ComposeCandidate is one co-tag co-occurrence candidate (component aux labels
// that co-occur inside the same article) queued for LLM adjudication.
// Components are sorted ascending; Cooccurrence counts DISTINCT articles in
// the co-tag window whose tags mount every component.
type ComposeCandidate struct {
	ComponentIDs []uint
	Cooccurrence int
	// RepresentativeTitles carries ≤3 recent co-occurring article titles for
	// the suggestion card's evidence block (spec: 代表事件标题).
	RepresentativeTitles []string
}

// composeCandidateLimit caps how many candidates are sent to the LLM per run
// (top-N by co-occurrence, stable order), preventing O(n²) pair explosion.
const composeCandidateLimit = 20

// collectComposeCandidates gathers composite-label candidates from co-tag
// co-occurrence (spec: 同一文章内共现). A component is an active auxiliary
// label with ref_count ≥ the upgrade threshold. Counting happens over
// articles created within CoTagWindowDays: an article contributes a pair (A,B)
// when its topic tags mount both aux labels; a triple (A,B,C) when all three.
//
// Selection: pairs meeting CompositeCoTagMinCooccurrence become candidates;
// a triple whose three sub-pairs all qualify AND whose own co-occurrence also
// meets the threshold replaces its sub-pairs (the stronger composite signal
// wins). Output is capped at composeCandidateLimit (frequency desc, then
// component ids asc for stability).
func (s *SemanticBoardUpgradeService) collectComposeCandidates(ctx context.Context, config SemanticBoardUpgradeConfig) ([]ComposeCandidate, error) {
	cutoff := time.Now().AddDate(0, 0, -config.CoTagWindowDays)

	// Article → distinct component aux ids mounted by that article's tags.
	var rows []struct {
		ArticleID uint   `gorm:"column:article_id"`
		AuxID     uint   `gorm:"column:aux_id"`
		Title     string `gorm:"column:title"`
	}
	err := s.db.WithContext(ctx).
		Table("article_topic_tags AS att").
		Select("DISTINCT att.article_id AS article_id, ttsl.semantic_label_id AS aux_id, article.title AS title").
		Joins("JOIN topic_tags AS tag ON tag.id = att.topic_tag_id AND tag.status = ?", "active").
		Joins("JOIN topic_tag_semantic_labels AS ttsl ON ttsl.topic_tag_id = att.topic_tag_id").
		Joins("JOIN semantic_labels AS aux ON aux.id = ttsl.semantic_label_id AND aux.label_type = ? AND aux.status = ? AND aux.ref_count >= ?", "auxiliary", "active", config.RefCountThreshold).
		Joins("JOIN articles AS article ON article.id = att.article_id AND article.created_at >= ?", cutoff).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	articleAux := make(map[uint][]uint)
	articleTitle := make(map[uint]string)
	articleOrder := make([]uint, 0)
	for _, row := range rows {
		if _, seen := articleTitle[row.ArticleID]; !seen {
			articleOrder = append(articleOrder, row.ArticleID)
			articleTitle[row.ArticleID] = ""
		}
		articleAux[row.ArticleID] = append(articleAux[row.ArticleID], row.AuxID)
		if articleTitle[row.ArticleID] == "" && strings.TrimSpace(row.Title) != "" {
			articleTitle[row.ArticleID] = strings.TrimSpace(row.Title)
		}
	}
	for articleID, auxIDs := range articleAux {
		sort.Slice(auxIDs, func(i, j int) bool { return auxIDs[i] < auxIDs[j] })
		articleAux[articleID] = UniqueUintSlice(auxIDs)
	}

	minCooccurrence := config.CompositeCoTagMinCooccurrence
	if minCooccurrence <= 0 {
		minCooccurrence = 10
	}

	type pairKey [2]uint
	pairCount := make(map[pairKey]int)
	for _, auxIDs := range articleAux {
		for i := 0; i < len(auxIDs); i++ {
			for j := i + 1; j < len(auxIDs); j++ {
				pairCount[pairKey{auxIDs[i], auxIDs[j]}]++
			}
		}
	}

	qualifiedPairs := make(map[pairKey]int, len(pairCount))
	for pair, count := range pairCount {
		if count >= minCooccurrence {
			qualifiedPairs[pair] = count
		}
	}
	if len(qualifiedPairs) == 0 {
		return nil, nil
	}

	// Triple expansion: only over articles whose aux set contains a fully
	// connected triple among qualified pairs (bounded work — qualified pairs
	// are few and capped implicitly by the threshold).
	tripleCount := make(map[[3]uint]int)
	for _, auxIDs := range articleAux {
		for i := 0; i < len(auxIDs); i++ {
			for j := i + 1; j < len(auxIDs); j++ {
				for k := j + 1; k < len(auxIDs); k++ {
					a, b, c := auxIDs[i], auxIDs[j], auxIDs[k]
					if _, ok := qualifiedPairs[pairKey{a, b}]; !ok {
						continue
					}
					if _, ok := qualifiedPairs[pairKey{a, c}]; !ok {
						continue
					}
					if _, ok := qualifiedPairs[pairKey{b, c}]; !ok {
						continue
					}
					tripleCount[[3]uint{a, b, c}]++
				}
			}
		}
	}
	qualifiedTriples := make(map[[3]uint]int, len(tripleCount))
	for triple, count := range tripleCount {
		if count >= minCooccurrence {
			qualifiedTriples[triple] = count
		}
	}

	// Candidates: qualified triples plus qualified pairs NOT absorbed by a
	// qualified triple.
	// Representative titles: first ≤3 co-occurring articles (insertion order)
	// carrying every component, for evidence display.
	representTitles := func(componentIDs []uint) []string {
		wanted := make(map[uint]struct{}, len(componentIDs))
		for _, id := range componentIDs {
			wanted[id] = struct{}{}
		}
		titles := make([]string, 0, 3)
		for _, articleID := range articleOrder {
			if len(titles) >= 3 {
				break
			}
			covered := 0
			for _, id := range articleAux[articleID] {
				if _, ok := wanted[id]; ok {
					covered++
				}
			}
			if covered == len(wanted) {
				if title := articleTitle[articleID]; title != "" {
					titles = append(titles, title)
				}
			}
		}
		return titles
	}

	var candidates []ComposeCandidate
	for triple := range qualifiedTriples {
		candidates = append(candidates, ComposeCandidate{ComponentIDs: triple[:], Cooccurrence: qualifiedTriples[triple], RepresentativeTitles: representTitles(triple[:])})
	}
	for pair, count := range qualifiedPairs {
		if pairAbsorbed(pair, qualifiedTriples) {
			continue
		}
		candidates = append(candidates, ComposeCandidate{ComponentIDs: pair[:], Cooccurrence: count, RepresentativeTitles: representTitles(pair[:])})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Cooccurrence == candidates[j].Cooccurrence {
			return compareUintSlice(candidates[i].ComponentIDs, candidates[j].ComponentIDs) < 0
		}
		return candidates[i].Cooccurrence > candidates[j].Cooccurrence
	})
	if len(candidates) > composeCandidateLimit {
		candidates = candidates[:composeCandidateLimit]
	}
	return candidates, nil
}

func pairAbsorbed(pair [2]uint, triples map[[3]uint]int) bool {
	for triple := range triples {
		contained := 0
		for _, id := range triple {
			for _, p := range pair {
				if id == p {
					contained++
					break
				}
			}
		}
		if contained == 2 {
			return true
		}
	}
	return false
}

func compareUintSlice(a, b []uint) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// buildComposeCandidatesPrompt renders the co-occurrence candidates for LLM
// adjudication (mode "compose"). The LLM must, per candidate, decide compose
// (with a composite label + description) or skip.
func buildComposeCandidatesPrompt(candidates []ComposeCandidate, labels map[uint]string) string {
	var sb strings.Builder
	sb.WriteString("以下标签组合在高频文章内反复共同出现（co-tag 共现统计，按共现文章数降序）。\n")
	sb.WriteString("请逐个判断：这些标签组合成一个明确的指向性主题是否有意义（如「美国国债」+「收益率」=「美债收益率」有意义；「日本」+「市场」这类泛化搭配无意义）。\n")
	sb.WriteString("对值得组合的候选输出 decision=\"compose\"（含 board_label=组合标签名、description=组合含义一句话、auxiliary_label_ids=组件标签 ID 列表）；\n")
	sb.WriteString("无明确指向语义的组合输出 decision=\"skip\"。\n\n")
	sb.WriteString("候选列表：\n")
	for i, candidate := range candidates {
		parts := make([]string, 0, len(candidate.ComponentIDs))
		for _, id := range candidate.ComponentIDs {
			label := labels[id]
			if label == "" {
				label = fmt.Sprintf("#%d", id)
			}
			parts = append(parts, fmt.Sprintf("%s(ID:%d)", label, id))
		}
		fmt.Fprintf(&sb, "%d. [%s] 共现文章数 %d\n", i+1, strings.Join(parts, " × "), candidate.Cooccurrence)
	}
	return sb.String()
}

// filterComposeSuggestions keeps only valid compose/skip decisions whose
// component ids are 2-5 known component labels (anti-hallucination: ids must
// come from the candidate pool's component universe).
func filterComposeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, componentUniverse map[uint]struct{}) []SemanticBoardUpgradeSuggestion {
	result := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.Decision != SemanticBoardUpgradeDecisionCompose && sug.Decision != SemanticBoardUpgradeDecisionSkip {
			continue // compose round only emits compose/skip
		}
		if sug.Decision == SemanticBoardUpgradeDecisionSkip {
			continue // skip decisions are never persisted; no need to carry them
		}
		ids := UniqueUintSlice(sug.AuxiliaryLabelIDs)
		if len(ids) < 2 || len(ids) > 5 {
			continue
		}
		valid := true
		for _, id := range ids {
			if _, ok := componentUniverse[id]; !ok {
				valid = false
				break
			}
		}
		if !valid || strings.TrimSpace(sug.BoardLabel) == "" {
			continue
		}
		sug.AuxiliaryLabelIDs = ids
		result = append(result, sug)
	}
	return result
}

// loadComponentLabels resolves display labels for the candidate component ids.
// filterExistingComposeCandidates 排除「已存在组合」候选，防止重复建议（2026-09-07
// 报障：确认创建过的组合再次出现在建议列表）。create 路（targetBoardID=nil）：
// 组件集与既有 active composite 完全一致的候选整体排除——组合已存在，无需再
// 建议创建。expand 路（targetBoardID 非 nil）：上述候选中已挂载目标版块的排除
// （挂载已完成，无操作空间）；未挂载的保留（语义退化为「挂载既有组合进目标
// 版块」，确认时 CreateCompositeLabel 按组件去重复用）。部分重叠不算重复：
// 既有组合 [a,b,c] 不排除候选 [a,b]（LLM 仍可裁决，确认时 L1/L2 去重兑底）。
func (s *SemanticBoardUpgradeService) filterExistingComposeCandidates(ctx context.Context, candidates []ComposeCandidate, targetBoardID *uint) ([]ComposeCandidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	var rows []struct {
		CompositeID uint `gorm:"column:composite_id"`
		ComponentID uint `gorm:"column:component_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("semantic_labels AS comp").
		Select("cc.composite_id, cc.component_label_id AS component_id").
		Joins("JOIN composite_components AS cc ON cc.composite_id = comp.id").
		Where("comp.label_type = ? AND comp.status = ?", "composite", "active").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return candidates, nil
	}
	byComposite := make(map[uint][]uint)
	for _, r := range rows {
		byComposite[r.CompositeID] = append(byComposite[r.CompositeID], r.ComponentID)
	}
	existingKeys := make(map[string]struct{}, len(byComposite))
	for _, ids := range byComposite {
		existingKeys[composeComponentKey(ids)] = struct{}{}
	}
	mountedKeys := make(map[string]struct{})
	if targetBoardID != nil && *targetBoardID != 0 {
		compositeIDs := make([]uint, 0, len(byComposite))
		for id := range byComposite {
			compositeIDs = append(compositeIDs, id)
		}
		var mounted []uint
		if err := s.db.WithContext(ctx).
			Table("board_composition").
			Select("auxiliary_label_id").
			Where("board_id = ? AND auxiliary_label_id IN ?", *targetBoardID, compositeIDs).
			Scan(&mounted).Error; err != nil {
			return nil, err
		}
		for _, id := range mounted {
			mountedKeys[composeComponentKey(byComposite[id])] = struct{}{}
		}
	}
	filtered := make([]ComposeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := composeComponentKey(candidate.ComponentIDs)
		if _, exists := existingKeys[key]; exists {
			if targetBoardID == nil {
				continue // create 路：组合已存在，无需再建议创建
			}
			if _, mounted := mountedKeys[key]; mounted {
				continue // expand 路：已挂载目标版块，无操作空间
			}
			// expand 路：组合存在但未挂载目标 → 保留（挂载语义，确认时复用既有组合）
		}
		filtered = append(filtered, candidate)
	}
	return filtered, nil
}

func (s *SemanticBoardUpgradeService) loadComponentLabels(ctx context.Context, ids []uint) (map[uint]string, error) {
	if len(ids) == 0 {
		return map[uint]string{}, nil
	}
	var labels []models.SemanticLabel
	if err := s.db.WithContext(ctx).Select("id, label").Where("id IN ?", ids).Find(&labels).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]string, len(labels))
	for _, label := range labels {
		result[label.ID] = label.Label
	}
	return result, nil
}
