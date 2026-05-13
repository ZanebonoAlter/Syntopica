package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

const (
	treeBridgeMaxPairs  = 50
	treeBridgeBatchSize = 20
	treeBridgeMinSim    = 0.78
	treeBridgeMaxKids   = 8
)

type treeBridgePair struct {
	TagA       uint    `json:"tag_a"`
	TagB       uint    `json:"tag_b"`
	Similarity float64 `json:"similarity"`
}

type treeBridgeJudgment struct {
	Pairs []treeBridgeJudgmentPair `json:"pairs"`
}

type treeBridgeJudgmentPair struct {
	Index  int    `json:"index"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type TreeBridgeResult struct {
	MergesApplied int      `json:"merges_applied"`
	LinksApplied  int      `json:"links_applied"`
	Errors        []string `json:"errors"`
}

var findTreeBridgeSimilarFn = func(rootID uint, category string, limit int) ([]TagCandidate, error) {
	es := NewEmbeddingService()
	return es.FindSimilarAbstractTags(context.Background(), rootID, category, limit)
}

var callTreeBridgeLLMFn = callTreeBridgeLLM

func collectTreeBridgePairs(category string) ([]treeBridgePair, error) {
	forest, err := BuildTagForest(category, 1)
	if err != nil {
		return nil, fmt.Errorf("build forest: %w", err)
	}

	rootIDs := make(map[uint]bool)
	for _, root := range forest {
		if root.Tag != nil {
			rootIDs[root.Tag.ID] = true
		}
	}

	var isolatedIDs []uint
	database.DB.Model(&models.TopicTag{}).
		Where("category = ? AND source = 'abstract' AND status = 'active'", category).
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE (r.parent_id = topic_tags.id OR r.child_id = topic_tags.id) AND r.relation_type = 'abstract')").
		Pluck("id", &isolatedIDs)
	for _, id := range isolatedIDs {
		rootIDs[id] = true
	}

	if len(rootIDs) == 0 {
		return nil, nil
	}

	pairSet := make(map[string]treeBridgePair)

	for rootID := range rootIDs {
		candidates, err := findTreeBridgeSimilarFn(rootID, category, 15)
		if err != nil {
			logging.Warnf("tree bridge: skip root %d: %v", rootID, err)
			continue
		}

		for _, cand := range candidates {
			if cand.Similarity < treeBridgeMinSim {
				continue
			}
			if rootID == cand.Tag.ID {
				continue
			}

			minID, maxID := rootID, cand.Tag.ID
			if minID > maxID {
				minID, maxID = maxID, minID
			}
			key := fmt.Sprintf("%d|%d", minID, maxID)
			if existing, exists := pairSet[key]; exists {
				if cand.Similarity > existing.Similarity {
					pairSet[key] = treeBridgePair{TagA: minID, TagB: maxID, Similarity: cand.Similarity}
				}
				continue
			}
			pairSet[key] = treeBridgePair{TagA: minID, TagB: maxID, Similarity: cand.Similarity}
		}
	}

	pairs := make([]treeBridgePair, 0, len(pairSet))
	for _, p := range pairSet {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Similarity > pairs[j].Similarity
	})

	if len(pairs) > treeBridgeMaxPairs {
		pairs = pairs[:treeBridgeMaxPairs]
	}

	return pairs, nil
}

func buildTreeBridgePrompt(pairs []treeBridgePair) string {
	tagIDSet := make(map[uint]bool)
	for _, p := range pairs {
		tagIDSet[p.TagA] = true
		tagIDSet[p.TagB] = true
	}
	tagIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		tagIDs = append(tagIDs, id)
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ?", tagIDs).Find(&tags)
	tagMap := make(map[uint]*models.TopicTag)
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	articleCounts := countArticlesByTag(tagIDs, "")

	childMap := make(map[uint][]string)
	for _, id := range tagIDs {
		var childLabels []string
		var childRows []struct {
			Label string `gorm:"column:label"`
		}
		database.DB.Table("topic_tags").
			Select("topic_tags.label").
			Joins("JOIN topic_tag_relations r ON r.child_id = topic_tags.id").
			Where("r.parent_id = ? AND r.relation_type = 'abstract' AND topic_tags.status = 'active'", id).
			Order("topic_tags.created_at ASC").
			Limit(treeBridgeMaxKids).
			Scan(&childRows)
		for _, cr := range childRows {
			childLabels = append(childLabels, cr.Label)
		}
		childMap[id] = childLabels
	}

	var entries []string
	for i, p := range pairs {
		tagA := tagMap[p.TagA]
		tagB := tagMap[p.TagB]
		if tagA == nil || tagB == nil {
			continue
		}

		childACount := len(childMap[p.TagA])
		childBCount := len(childMap[p.TagB])

		ctxA := formatTagPromptContext(tagA)
		ctxB := formatTagPromptContext(tagB)

		extraA := ""
		if ctxA != "" {
			extraA = ", " + ctxA
		}
		if len(childMap[p.TagA]) > 0 {
			extraA += fmt.Sprintf(", 子标签: [%s]", strings.Join(childMap[p.TagA], ", "))
		}

		extraB := ""
		if ctxB != "" {
			extraB = ", " + ctxB
		}
		if len(childMap[p.TagB]) > 0 {
			extraB += fmt.Sprintf(", 子标签: [%s]", strings.Join(childMap[p.TagB], ", "))
		}

		entry := fmt.Sprintf(
			"%d. 树根 A: \"%s\" (%d 子节点, %d 文章%s)\n   树根 B: \"%s\" (%d 子节点, %d 文章%s)\n   相似度: %.2f",
			i+1,
			tagA.Label, childACount, articleCounts[p.TagA], extraA,
			tagB.Label, childBCount, articleCounts[p.TagB], extraB,
			p.Similarity,
		)
		entries = append(entries, entry)
	}

	return fmt.Sprintf(`你是一位标签分类专家。请分析以下同类别的树根标签对，判断它们之间是否应该桥接（合并或建立父子关系）。

候选对列表：
%s

请对每一对返回判断结果，格式为 JSON：
{
  "pairs": [
    {"index": 1, "action": "merge", "reason": "描述同一概念"},
    {"index": 2, "action": "parent_A", "reason": "B 是 A 的子领域"},
    {"index": 3, "action": "parent_B", "reason": "A 是 B 的子领域"},
    {"index": 4, "action": "skip", "reason": "概念不相关"}
  ]
}

规则：
1. merge: 两棵树描述同一概念/同义词/翻译，应合并（保留子节点多、文章多的一方）
2. parent_A: B 是 A 的窄概念/子领域，B 树整体应挂在 A 下
3. parent_B: A 是 B 的窄概念/子领域，A 树整体应挂在 B 下
4. skip: 两棵树概念不相关，无需连接
5. 只为真正语义相关的标签对建立连接，merge 仅用于严格同义词/翻译
6. index 必须与候选对编号一致`, strings.Join(entries, "\n\n"))
}

func callTreeBridgeLLM(prompt string) (*treeBridgeJudgment, error) {
	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy bridge assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"pairs": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"index":  {Type: "integer"},
							"action": {Type: "string", Enum: []string{"merge", "parent_A", "parent_B", "skip"}},
							"reason": {Type: "string"},
						},
						Required: []string{"index", "action", "reason"},
					},
				},
			},
			Required: []string{"pairs"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "tree_bridge",
		},
	}

	result, err := router.Chat(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("tree bridge LLM call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment treeBridgeJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse tree bridge response: %w", err)
	}

	return &judgment, nil
}

func executeTreeBridgePairs(pairs []treeBridgePair, judgment *treeBridgeJudgment, category string) (int, int, []string, error) {
	var errors []string
	merges := 0
	links := 0
	skipSet := make(map[uint]bool)

	pairMap := make(map[int]treeBridgePair)
	for i, p := range pairs {
		pairMap[i+1] = p
	}

	type judgedAction struct {
		pair   treeBridgePair
		action string
		reason string
	}
	var mergesToApply []judgedAction
	var parentsToApply []judgedAction

	for _, jp := range judgment.Pairs {
		pair, ok := pairMap[jp.Index]
		if !ok {
			errors = append(errors, fmt.Sprintf("judgment index %d not found in pairs", jp.Index))
			continue
		}
		switch jp.Action {
		case "merge":
			mergesToApply = append(mergesToApply, judgedAction{pair: pair, action: jp.Action, reason: jp.Reason})
		case "parent_A", "parent_B":
			parentsToApply = append(parentsToApply, judgedAction{pair: pair, action: jp.Action, reason: jp.Reason})
		case "skip":
		default:
			errors = append(errors, fmt.Sprintf("unknown action %q for pair %d", jp.Action, jp.Index))
		}
	}

	for _, ma := range mergesToApply {
		sourceID, targetID := determineTreeBridgeMergeDirection(ma.pair)
		if sourceID == 0 || targetID == 0 {
			errors = append(errors, fmt.Sprintf("merge pair (%d,%d): cannot determine direction", ma.pair.TagA, ma.pair.TagB))
			continue
		}

		logging.Infof("Tree bridge (%s): merging tag %d into %d (reason: %s)", category, sourceID, targetID, ma.reason)
		if err := MergeTags(sourceID, targetID); err != nil {
			errors = append(errors, fmt.Sprintf("merge %d→%d: %v", sourceID, targetID, err))
			continue
		}
		skipSet[sourceID] = true
		merges++
	}

	for _, pa := range parentsToApply {
		if skipSet[pa.pair.TagA] || skipSet[pa.pair.TagB] {
			logging.Infof("Tree bridge (%s): skip parent link for (%d,%d) — one tag was merged", category, pa.pair.TagA, pa.pair.TagB)
			continue
		}

		var parentID, childID uint
		if pa.action == "parent_A" {
			parentID = pa.pair.TagA
			childID = pa.pair.TagB
		} else {
			parentID = pa.pair.TagB
			childID = pa.pair.TagA
		}

		logging.Infof("Tree bridge (%s): linking child %d under parent %d (reason: %s)", category, childID, parentID, pa.reason)
		if err := linkAbstractParentChild(childID, parentID); err != nil {
			errors = append(errors, fmt.Sprintf("link %d→%d: %v", childID, parentID, err))
			continue
		}
		links++
	}

	logging.Infof("Tree bridge (%s): %d merges, %d parent links applied", category, merges, links)
	return merges, links, errors, nil
}

func determineTreeBridgeMergeDirection(pair treeBridgePair) (sourceID, targetID uint) {
	tagIDs := []uint{pair.TagA, pair.TagB}
	articleCounts := countArticlesByTag(tagIDs, "")

	var childRows []struct {
		ParentID uint `gorm:"column:parent_id"`
		Cnt      int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTagRelation{}).
		Select("parent_id, count(*) as cnt").
		Where("parent_id IN ? AND relation_type = 'abstract'", tagIDs).
		Group("parent_id").
		Scan(&childRows)

	childCounts := make(map[uint]int)
	for _, r := range childRows {
		childCounts[r.ParentID] = r.Cnt
	}

	countA := childCounts[pair.TagA]
	countB := childCounts[pair.TagB]

	if countA < countB {
		return pair.TagA, pair.TagB
	}
	if countB < countA {
		return pair.TagB, pair.TagA
	}
	artsA := articleCounts[pair.TagA]
	artsB := articleCounts[pair.TagB]
	if artsA <= artsB {
		return pair.TagA, pair.TagB
	}
	return pair.TagB, pair.TagA
}

func ExecuteTreeBridge(category string, budget LLMBudget) (*TreeBridgeResult, error) {
	result := &TreeBridgeResult{}

	pairs, err := collectTreeBridgePairs(category)
	if err != nil {
		return result, fmt.Errorf("collect pairs: %w", err)
	}
	if len(pairs) == 0 {
		return result, nil
	}

	logging.Infof("Tree bridge (%s): collected %d unique pairs", category, len(pairs))

	for i := 0; i < len(pairs); i += treeBridgeBatchSize {
		end := i + treeBridgeBatchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[i:end]

		if budget != nil {
			if budget.IsTimedOut() {
				logging.Warnf("Tree bridge (%s): budget timed out, stopping", category)
				break
			}
			if !budget.ConsumeForPhase("phase3_5") {
				logging.Warnf("Tree bridge (%s): LLM budget exhausted, stopping", category)
				break
			}
		}

		prompt := buildTreeBridgePrompt(batch)
		judgment, err := callTreeBridgeLLMFn(prompt)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("batch %d-%d LLM: %v", i+1, end, err))
			continue
		}

		merges, links, execErrors, execErr := executeTreeBridgePairs(batch, judgment, category)
		if execErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("batch %d-%d execute: %v", i+1, end, execErr))
		}
		result.Errors = append(result.Errors, execErrors...)
		result.MergesApplied += merges
		result.LinksApplied += links
	}

	return result, nil
}
