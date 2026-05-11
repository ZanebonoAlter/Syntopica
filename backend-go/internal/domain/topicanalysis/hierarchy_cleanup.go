package topicanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topictypes"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

// TreeNode represents a node in the tag hierarchy tree

const MinTreeDepthForCleanup = 3

type TreeNode struct {
	Tag          *models.TopicTag
	Depth        int
	Children     []*TreeNode
	Parent       *TreeNode
	ArticleCount int
}

type treeCleanupAbstract struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ChildrenIDs []uint `json:"children_ids"`
	Reason      string `json:"reason"`
}

type treeReviewMove struct {
	TagID    uint   `json:"tag_id"`
	ToParent uint   `json:"to_parent"`
	Reason   string `json:"reason"`
}

type treeReviewAbstract struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ChildrenIDs []uint `json:"children_ids"`
	Reason      string `json:"reason"`
}

type treeReviewMerge struct {
	SourceID uint   `json:"source_id"`
	TargetID uint   `json:"target_id"`
	Reason   string `json:"reason"`
}

type treeReviewJudgment struct {
	Moves        []treeReviewMove     `json:"moves"`
	Merges       []treeReviewMerge    `json:"merges"`
	NewAbstracts []treeReviewAbstract `json:"new_abstracts"`
	Notes        string               `json:"notes"`
}

type LLMBudget interface {
	ConsumeForPhase(phase string) bool
	IsTimedOut() bool
}

const smallTreeThreshold = 20 // 节点数 ≤ 此值的小树可合并审查

// reviewForestBatched merges small trees into batched LLM reviews.
// Trees with nodeCount > smallTreeThreshold are reviewed individually.
func reviewForestBatched(forest []*TreeNode, category string, result *TreeReviewResult, budget LLMBudget) {
	canConsume := func() bool {
		if budget == nil {
			return true
		}
		return budget.ConsumeForPhase("phase6")
	}
	isTimedOut := func() bool {
		if budget == nil {
			return false
		}
		return budget.IsTimedOut()
	}

	var smallTrees []*TreeNode
	for _, root := range forest {
		if isTimedOut() {
			return
		}
		if countNodes(root) <= smallTreeThreshold {
			smallTrees = append(smallTrees, root)
		} else {
			for _, tree := range splitReviewTrees(root, 50) {
				if !canConsume() {
					logging.Warnf("Phase 6: LLM budget exhausted, stopping tree review")
					return
				}
				reviewOneTree(tree, category, result)
			}
		}
	}

	if len(smallTrees) == 0 {
		return
	}

	batch := mergeSmallTreesForReview(smallTrees, 5, 100)
	for _, group := range batch {
		if isTimedOut() {
			return
		}
		if !canConsume() {
			logging.Warnf("Phase 6: LLM budget exhausted, stopping small tree batch review")
			return
		}
		reviewOneTree(group, category, result)
	}
}

// mergeSmallTreesForReview merges multiple small trees under a virtual root for batched review.
// maxTrees: max trees per batch. maxNodes: max total nodes per batch.
func mergeSmallTreesForReview(trees []*TreeNode, maxTrees, maxNodes int) []*TreeNode {
	var batches []*TreeNode
	var currentBatch []*TreeNode
	currentNodes := 0

	for _, tree := range trees {
		treeNodes := countNodes(tree)
		if len(currentBatch) >= maxTrees || currentNodes+treeNodes > maxNodes {
			if len(currentBatch) > 0 {
				batches = append(batches, createVirtualRoot(currentBatch))
			}
			currentBatch = nil
			currentNodes = 0
		}
		currentBatch = append(currentBatch, tree)
		currentNodes += treeNodes
	}
	if len(currentBatch) > 0 {
		batches = append(batches, createVirtualRoot(currentBatch))
	}
	return batches
}

// createVirtualRoot creates a virtual root node wrapping multiple trees for review.
func createVirtualRoot(trees []*TreeNode) *TreeNode {
	virtualRoot := &TreeNode{
		Tag: &models.TopicTag{
			ID:       0, // 虚拟根节点
			Label:    "[合并审查]",
			Category: trees[0].Tag.Category,
			Source:   "virtual",
		},
		Depth: 0,
	}
	for _, tree := range trees {
		tree.Parent = virtualRoot
		virtualRoot.Children = append(virtualRoot.Children, tree)
	}
	return virtualRoot
}

// BuildTagForest builds all tag trees for a given category, filtering trees with depth >= minDepth.
func BuildTagForest(category string, minDepth ...int) ([]*TreeNode, error) {
	var relations []models.TopicTagRelation
	if err := database.DB.
		Table("topic_tag_relations").
		Joins("JOIN topic_tags p ON topic_tag_relations.parent_id = p.id AND p.status = ?", "active").
		Joins("JOIN topic_tags c ON topic_tag_relations.child_id = c.id AND c.status = ?", "active").
		Where("topic_tag_relations.relation_type = ?", "abstract").
		Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("query tag relations: %w", err)
	}

	if len(relations) == 0 {
		return nil, nil
	}

	// Build parent->children map
	childrenMap := make(map[uint][]uint)
	parentSet := make(map[uint]bool)
	childSet := make(map[uint]bool)

	for _, r := range relations {
		childrenMap[r.ParentID] = append(childrenMap[r.ParentID], r.ChildID)
		parentSet[r.ParentID] = true
		childSet[r.ChildID] = true
	}

	// Find root nodes (nodes that are parents but not children)
	var rootIDs []uint
	for parentID := range parentSet {
		if !childSet[parentID] {
			rootIDs = append(rootIDs, parentID)
		}
	}

	if len(rootIDs) == 0 {
		// Handle cycles: find entry points
		rootIDs = findCycleRoots(relations, parentSet)
	}

	// Load all tags in the hierarchy
	allTagIDs := make(map[uint]bool)
	for _, r := range relations {
		allTagIDs[r.ParentID] = true
		allTagIDs[r.ChildID] = true
	}

	tagIDs := make([]uint, 0, len(allTagIDs))
	for id := range allTagIDs {
		tagIDs = append(tagIDs, id)
	}

	var tags []models.TopicTag
	if err := database.DB.Where("id IN ? AND category = ?", tagIDs, category).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("load tags: %w", err)
	}

	tagMap := make(map[uint]*models.TopicTag)
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	// Build article counts
	articleCounts := countArticlesByTag(tagIDs, "")

	// Build trees
	md := MinTreeDepthForCleanup
	if len(minDepth) > 0 {
		md = minDepth[0]
	}
	var forest []*TreeNode
	for _, rootID := range rootIDs {
		rootTag, ok := tagMap[rootID]
		if !ok {
			continue
		}
		root := buildTreeNode(rootTag, 1, childrenMap, tagMap, articleCounts)
		depth := calculateTreeDepth(root)
		if depth >= md {
			forest = append(forest, root)
		}
	}

	return forest, nil
}

// buildTreeNode recursively builds a tree from the root
func buildTreeNode(tag *models.TopicTag, depth int, childrenMap map[uint][]uint, tagMap map[uint]*models.TopicTag, articleCounts map[uint]int) *TreeNode {
	return buildTreeNodeWithVisited(tag, depth, childrenMap, tagMap, articleCounts, make(map[uint]bool))
}

func buildTreeNodeWithVisited(tag *models.TopicTag, depth int, childrenMap map[uint][]uint, tagMap map[uint]*models.TopicTag, articleCounts map[uint]int, visited map[uint]bool) *TreeNode {
	if visited[tag.ID] {
		return nil
	}
	visited[tag.ID] = true

	node := &TreeNode{
		Tag:          tag,
		Depth:        depth,
		ArticleCount: articleCounts[tag.ID],
	}

	for _, childID := range childrenMap[tag.ID] {
		childTag, ok := tagMap[childID]
		if !ok {
			continue
		}
		childNode := buildTreeNodeWithVisited(childTag, depth+1, childrenMap, tagMap, articleCounts, visited)
		if childNode == nil {
			continue
		}
		childNode.Parent = node
		node.Children = append(node.Children, childNode)
	}

	return node
}

// calculateTreeDepth calculates the maximum depth of a tree
func calculateTreeDepth(node *TreeNode) int {
	return calculateTreeDepthVisited(node, make(map[uint]bool))
}

func calculateTreeDepthVisited(node *TreeNode, visited map[uint]bool) int {
	if node == nil || node.Tag == nil || visited[node.Tag.ID] {
		return 0
	}
	visited[node.Tag.ID] = true
	if len(node.Children) == 0 {
		return 1
	}
	maxChildDepth := 0
	for _, child := range node.Children {
		d := calculateTreeDepthVisited(child, visited)
		if d > maxChildDepth {
			maxChildDepth = d
		}
	}
	return maxChildDepth + 1
}

// countNodes counts total nodes in a tree
func countNodes(node *TreeNode) int {
	return countNodesVisited(node, make(map[uint]bool))
}

func countNodesVisited(node *TreeNode, visited map[uint]bool) int {
	if node == nil || node.Tag == nil {
		return 0
	}
	if visited[node.Tag.ID] {
		return 0
	}
	visited[node.Tag.ID] = true
	count := 1
	for _, child := range node.Children {
		count += countNodesVisited(child, visited)
	}
	return count
}

func collectAllTags(node *TreeNode) []*TreeNode {
	return collectAllTagsVisited(node, make(map[uint]bool))
}

func collectAllTagsVisited(node *TreeNode, visited map[uint]bool) []*TreeNode {
	if node == nil || node.Tag == nil {
		return nil
	}
	if visited[node.Tag.ID] {
		return nil
	}
	visited[node.Tag.ID] = true
	result := []*TreeNode{node}
	for _, child := range node.Children {
		result = append(result, collectAllTagsVisited(child, visited)...)
	}
	return result
}

type TreeReviewResult struct {
	TreesReviewed int
	MergesApplied int
	MovesApplied  int
	GroupsCreated int
	GroupsReused  int
	Errors        []string
}

func ReviewHierarchyTrees(category string, windowDays int, budget LLMBudget) (*TreeReviewResult, error) {
	forest, err := BuildTagForest(category, 2)
	if err != nil {
		return nil, fmt.Errorf("build forest: %w", err)
	}
	if len(forest) == 0 {
		return &TreeReviewResult{}, nil
	}

	forest = filterTreesWithRecentRelations(forest, windowDays)
	if len(forest) == 0 {
		return &TreeReviewResult{}, nil
	}

	result := &TreeReviewResult{}
	reviewForestBatched(forest, category, result, budget)
	return result, nil
}

func filterTreesWithRecentRelations(forest []*TreeNode, windowDays int) []*TreeNode {
	if windowDays <= 0 {
		return forest
	}
	cutoff := time.Now().AddDate(0, 0, -windowDays)

	forestTagIDs := make(map[uint]bool)
	for _, root := range forest {
		for _, node := range collectAllTags(root) {
			forestTagIDs[node.Tag.ID] = true
		}
	}

	var relations []models.TopicTagRelation
	if err := database.DB.Where("relation_type = ? AND created_at >= ?", "abstract", cutoff).Find(&relations).Error; err != nil {
		logging.Warnf("filterTreesWithRecentRelations: failed to load recent relations: %v", err)
		return nil
	}
	recentTagSet := make(map[uint]bool)
	for _, r := range relations {
		if forestTagIDs[r.ParentID] && forestTagIDs[r.ChildID] {
			recentTagSet[r.ParentID] = true
			recentTagSet[r.ChildID] = true
		}
	}

	var filtered []*TreeNode
	for _, root := range forest {
		if treeContainsTag(root, recentTagSet) {
			filtered = append(filtered, root)
		}
	}
	return filtered
}

func treeContainsTag(node *TreeNode, tagSet map[uint]bool) bool {
	return treeContainsTagVisited(node, tagSet, make(map[uint]bool))
}

func treeContainsTagVisited(node *TreeNode, tagSet map[uint]bool, visited map[uint]bool) bool {
	if node == nil || node.Tag == nil || visited[node.Tag.ID] {
		return false
	}
	visited[node.Tag.ID] = true
	if tagSet[node.Tag.ID] {
		return true
	}
	for _, child := range node.Children {
		if treeContainsTagVisited(child, tagSet, visited) {
			return true
		}
	}
	return false
}

func splitReviewTrees(root *TreeNode, maxNodes int) []*TreeNode {
	return splitReviewTreesVisited(root, maxNodes, make(map[uint]bool))
}

func splitReviewTreesVisited(root *TreeNode, maxNodes int, visited map[uint]bool) []*TreeNode {
	if root == nil || root.Tag == nil || visited[root.Tag.ID] {
		return nil
	}
	visited[root.Tag.ID] = true
	if countNodes(root) <= maxNodes {
		return []*TreeNode{root}
	}
	parts := []*TreeNode{rootLevelReviewTree(root)}
	for _, child := range root.Children {
		parts = append(parts, splitReviewTreesVisited(child, maxNodes, visited)...)
	}
	return parts
}

func rootLevelReviewTree(root *TreeNode) *TreeNode {
	clone := &TreeNode{Tag: root.Tag, Depth: root.Depth, ArticleCount: root.ArticleCount}
	seen := make(map[uint]bool)
	seen[root.Tag.ID] = true
	for _, child := range root.Children {
		if child.Tag == nil || seen[child.Tag.ID] {
			continue
		}
		seen[child.Tag.ID] = true
		childClone := &TreeNode{Tag: child.Tag, Depth: child.Depth, ArticleCount: child.ArticleCount, Parent: clone}
		clone.Children = append(clone.Children, childClone)
	}
	return clone
}

func reviewOneTree(tree *TreeNode, category string, result *TreeReviewResult) {
	treeStr := serializeTreeForReview(tree)
	prompt := buildTreeReviewPrompt(treeStr, category)

	judgment, err := callTreeReviewLLMFn(prompt)
	if err != nil {
		rootID := uint(0)
		if tree.Tag != nil {
			rootID = tree.Tag.ID
		}
		result.Errors = append(result.Errors, fmt.Sprintf("tree root %d: %v", rootID, err))
		return
	}
	result.TreesReviewed++

	tagMap := make(map[uint]*TreeNode)
	for _, node := range collectAllTags(tree) {
		if node.Tag != nil && node.Tag.ID != 0 { // 跳过虚拟根节点
			tagMap[node.Tag.ID] = node
		}
	}

	// 虚拟根节点时，所有 merge/move 都允许（无 root 保护）
	isVirtual := tree.Tag == nil || tree.Tag.ID == 0

	for _, merge := range judgment.Merges {
		if !isVirtual && merge.SourceID == tree.Tag.ID {
			result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: root node cannot be used as merge source", merge.SourceID, merge.TargetID))
			continue
		}
		if err := validateTreeReviewMerge(merge, tagMap); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		if err := MergeTags(merge.SourceID, merge.TargetID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		delete(tagMap, merge.SourceID)
		result.MergesApplied++
	}

	for _, move := range judgment.Moves {
		if !isVirtual && move.TagID == tree.Tag.ID {
			result.Errors = append(result.Errors, fmt.Sprintf("move %d: review root node cannot be moved", move.TagID))
			continue
		}
		if _, ok := tagMap[move.TagID]; !ok {
			continue
		}
		if err := validateTreeReviewMove(move, tagMap); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("move %d: %v", move.TagID, err))
			continue
		}
		if err := executeTreeReviewMove(move); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("move %d: %v", move.TagID, err))
			continue
		}
		result.MovesApplied++
	}

	for _, abs := range judgment.NewAbstracts {
		created, err := validateAndCreateReviewAbstract(abs, tagMap, category)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("abstract %s: %v", abs.Name, err))
			continue
		}
		if created {
			result.GroupsCreated++
		} else {
			result.GroupsReused++
		}
	}
}

// findCycleRoots finds root nodes in cyclic graphs
func findCycleRoots(relations []models.TopicTagRelation, parentSet map[uint]bool) []uint {
	childToParent := make(map[uint]uint)
	for _, r := range relations {
		childToParent[r.ChildID] = r.ParentID
	}

	cycleRoots := make(map[uint]bool)
	globalVisited := make(map[uint]bool)

	for pid := range parentSet {
		if globalVisited[pid] {
			continue
		}
		path := make(map[uint]bool)
		current := pid
		for {
			if path[current] {
				cycleRoots[current] = true
				break
			}
			if globalVisited[current] {
				break
			}
			path[current] = true
			p, ok := childToParent[current]
			if !ok {
				break
			}
			current = p
		}
		for id := range path {
			globalVisited[id] = true
		}
	}

	var result []uint
	for id := range cycleRoots {
		result = append(result, id)
	}
	return result
}

func serializeTreeForReview(node *TreeNode) string {
	var sb strings.Builder
	serializeNodeForReview(&sb, node, "", true, make(map[uint]bool))
	return sb.String()
}

func serializeNodeForReview(sb *strings.Builder, node *TreeNode, prefix string, isLast bool, visited map[uint]bool) {
	if node == nil || node.Tag == nil || visited[node.Tag.ID] {
		return
	}
	visited[node.Tag.ID] = true
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		fmt.Fprintf(sb, "[id:%d] %s", node.Tag.ID, node.Tag.Label)
	} else {
		fmt.Fprintf(sb, "%s%s[id:%d] %s", prefix, connector, node.Tag.ID, node.Tag.Label)
	}
	if contextInfo := formatTagPromptContext(node.Tag); contextInfo != "" {
		fmt.Fprintf(sb, " (%s)", truncateStr(contextInfo, 160))
	}
	sb.WriteString("\n")

	for i, child := range node.Children {
		var newPrefix string
		switch {
		case prefix == "":
			newPrefix = "  "
		case isLast:
			newPrefix = prefix + "    "
		default:
			newPrefix = prefix + "│   "
		}
		serializeNodeForReview(sb, child, newPrefix, i == len(node.Children)-1, visited)
	}
}

func callTreeReviewLLM(prompt string) (*treeReviewJudgment, error) {
	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy review assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"moves": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"tag_id":    {Type: "integer"},
							"to_parent": {Type: "integer"},
							"reason":    {Type: "string"},
						},
						Required: []string{"tag_id", "to_parent", "reason"},
					},
				},
				"merges": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"source_id": {Type: "integer"},
							"target_id": {Type: "integer"},
							"reason":    {Type: "string"},
						},
						Required: []string{"source_id", "target_id", "reason"},
					},
				},
				"new_abstracts": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":         {Type: "string"},
							"description":  {Type: "string"},
							"children_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"reason":       {Type: "string"},
						},
						Required: []string{"name", "description", "children_ids", "reason"},
					},
				},
				"notes": {Type: "string"},
			},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "tree_review",
		},
	}

	result, err := router.Chat(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("tree review LLM call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment treeReviewJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse tree review response: %w", err)
	}

	logging.Infof("Tree review LLM judgment: %d merges, %d moves, %d new abstracts",
		len(judgment.Merges), len(judgment.Moves), len(judgment.NewAbstracts))

	return &judgment, nil
}

func buildTreeReviewPrompt(treeStr string, category string) string {
	return fmt.Sprintf(`请审查以下 %s 类别的标签树，检查子标签的归属是否合理，并给出调整建议。

%s

规则:
- 检查每个子标签是否真正属于其父标签
- 地理/区域不同且无直接关联的标签，不应在同一抽象父下
- 概念领域明显不同的标签，不应在同一父下
- 树的顶级根节点（第一个 [id:...] ）不允许被 move 为其他节点的子节点，也不允许作为 merge 的 source
- [id:0] 是虚拟根节点（合并审查用），不是真实标签，不允许在 moves/merges/new_abstracts 中引用 id=0
- 非 root 的子节点可以 merge（source 合并进 target），合并后 source 的子节点会自动迁移到 target 下
- to_parent=0 表示脱离成为独立根节点
- 非零 to_parent 表示迁移到树中已有标签下
- merges 用于合并树中语义重复的抽象标签，source 合并进 target（target 保留）
- new_abstracts 用于建议创建新分组，children_ids 至少 2 个
- 可以同时返回 moves、merges、new_abstracts，不必只选一种
- 如果树结构合理无需调整，返回空的 moves、merges 和 new_abstracts

返回 JSON:
{
  "moves": [
    {"tag_id": 123, "to_parent": 0, "reason": "..."}
  ],
  "merges": [
    {"source_id": 123, "target_id": 456, "reason": "..."}
  ],
  "new_abstracts": [
    {"name": "新抽象名", "description": "描述", "children_ids": [123, 456], "reason": "..."}
  ]
}`, category, treeStr)
}

func validateTreeReviewMove(move treeReviewMove, tagMap map[uint]*TreeNode) error {
	node, ok := tagMap[move.TagID]
	if !ok {
		return fmt.Errorf("tag %d not found in tree", move.TagID)
	}
	if node.Tag.Status != "active" {
		return fmt.Errorf("tag %d is not active", move.TagID)
	}
	if move.ToParent == 0 {
		return nil
	}
	if move.ToParent == move.TagID {
		return fmt.Errorf("tag %d cannot be its own parent", move.TagID)
	}
	target, ok := tagMap[move.ToParent]
	if !ok {
		return fmt.Errorf("target parent %d not found in tree", move.ToParent)
	}
	if target.Tag.Status != "active" {
		return fmt.Errorf("target parent %d is not active", move.ToParent)
	}
	if database.DB == nil {
		return nil
	}
	wouldCycle, err := wouldCreateCycle(database.DB, move.TagID, move.ToParent)
	if err != nil {
		return fmt.Errorf("check cycle for move %d -> %d: %w", move.TagID, move.ToParent, err)
	}
	if wouldCycle {
		return fmt.Errorf("move %d -> %d would create cycle", move.TagID, move.ToParent)
	}
	childSubtreeDepth := getAbstractSubtreeDepth(database.DB, move.TagID)
	parentAncestryDepth := getTagDepthFromRoot(move.ToParent)
	if childSubtreeDepth+parentAncestryDepth+1 > maxHierarchyDepth {
		return fmt.Errorf("move %d -> %d would exceed max depth %d", move.TagID, move.ToParent, maxHierarchyDepth)
	}
	return nil
}

func validateTreeReviewMerge(merge treeReviewMerge, tagMap map[uint]*TreeNode) error {
	sourceNode, ok := tagMap[merge.SourceID]
	if !ok {
		return fmt.Errorf("source tag %d not found in tree", merge.SourceID)
	}
	if sourceNode.Tag.Status != "active" {
		return fmt.Errorf("source tag %d is not active (status=%s)", merge.SourceID, sourceNode.Tag.Status)
	}
	targetNode, ok := tagMap[merge.TargetID]
	if !ok {
		return fmt.Errorf("target tag %d not found in tree", merge.TargetID)
	}
	if targetNode.Tag.Status != "active" {
		return fmt.Errorf("target tag %d is not active", merge.TargetID)
	}
	if merge.SourceID == merge.TargetID {
		return fmt.Errorf("cannot merge tag %d into itself", merge.SourceID)
	}
	if isAncestorReviewNode(sourceNode, targetNode) || isAncestorReviewNode(targetNode, sourceNode) {
		return fmt.Errorf("cannot merge ancestor and descendant tags %d -> %d", merge.SourceID, merge.TargetID)
	}
	if database.DB == nil {
		return nil
	}
	wouldCycle, err := wouldCreateCycle(database.DB, merge.TargetID, merge.SourceID)
	if err != nil {
		return fmt.Errorf("check cycle for merge %d -> %d: %w", merge.SourceID, merge.TargetID, err)
	}
	if wouldCycle {
		return fmt.Errorf("merge %d -> %d would create cycle (source is ancestor of target)", merge.SourceID, merge.TargetID)
	}
	childSubtreeDepth := getAbstractSubtreeDepth(database.DB, merge.SourceID)
	parentAncestryDepth := getTagDepthFromRoot(merge.TargetID)
	if childSubtreeDepth+parentAncestryDepth+1 > maxHierarchyDepth {
		return fmt.Errorf("merge %d -> %d would exceed max depth %d after migration", merge.SourceID, merge.TargetID, maxHierarchyDepth)
	}
	if err := validateTreeReviewMergeRelationMigrations(database.DB, merge); err != nil {
		return err
	}
	return nil
}

func isAncestorReviewNode(ancestor *TreeNode, node *TreeNode) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if current.Tag != nil && ancestor.Tag != nil && current.Tag.ID == ancestor.Tag.ID {
			return true
		}
	}
	return false
}

func validateTreeReviewMergeRelationMigrations(db *gorm.DB, merge treeReviewMerge) error {
	var sourceParentRelations []models.TopicTagRelation
	if err := db.Where("parent_id = ? AND relation_type = ?", merge.SourceID, "abstract").Find(&sourceParentRelations).Error; err != nil {
		return fmt.Errorf("load source child relations for merge %d -> %d: %w", merge.SourceID, merge.TargetID, err)
	}
	for _, rel := range sourceParentRelations {
		if rel.ChildID == merge.TargetID || hasAbstractRelation(db, merge.TargetID, rel.ChildID) {
			continue
		}
		wouldCycle, err := wouldCreateCycle(db, rel.ChildID, merge.TargetID)
		if err != nil {
			return fmt.Errorf("check migrated child relation %d -> %d: %w", merge.TargetID, rel.ChildID, err)
		}
		if wouldCycle {
			return fmt.Errorf("merge %d -> %d would create cycle by migrating child relation %d -> %d", merge.SourceID, merge.TargetID, merge.TargetID, rel.ChildID)
		}
		if err := checkDepthLimit(db, merge.TargetID, rel.ChildID); err != nil {
			return fmt.Errorf("merge %d -> %d would exceed max depth for migrated child %d: %w", merge.SourceID, merge.TargetID, rel.ChildID, err)
		}
	}

	var sourceChildRelations []models.TopicTagRelation
	if err := db.Where("child_id = ? AND relation_type = ?", merge.SourceID, "abstract").Find(&sourceChildRelations).Error; err != nil {
		return fmt.Errorf("load source parent relations for merge %d -> %d: %w", merge.SourceID, merge.TargetID, err)
	}
	for _, rel := range sourceChildRelations {
		if rel.ParentID == merge.TargetID || hasAbstractRelation(db, rel.ParentID, merge.TargetID) {
			continue
		}
		wouldCycle, err := wouldCreateCycle(db, merge.TargetID, rel.ParentID)
		if err != nil {
			return fmt.Errorf("check migrated parent relation %d -> %d: %w", rel.ParentID, merge.TargetID, err)
		}
		if wouldCycle {
			return fmt.Errorf("merge %d -> %d would create cycle by migrating parent relation %d -> %d", merge.SourceID, merge.TargetID, rel.ParentID, merge.TargetID)
		}
		if err := checkDepthLimit(db, rel.ParentID, merge.TargetID); err != nil {
			return fmt.Errorf("merge %d -> %d would exceed max depth for migrated parent %d: %w", merge.SourceID, merge.TargetID, rel.ParentID, err)
		}
	}
	return nil
}

func hasAbstractRelation(db *gorm.DB, parentID uint, childID uint) bool {
	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND child_id = ? AND relation_type = ?", parentID, childID, "abstract").
		Count(&count)
	return count > 0
}

func executeTreeReviewMove(move treeReviewMove) error {
	if move.ToParent == 0 {
		var oldParents []models.TopicTagRelation
		if err := database.DB.Where(
			"child_id = ? AND relation_type = ?", move.TagID, "abstract",
		).Find(&oldParents).Error; err != nil {
			return fmt.Errorf("load old parents for detach tag %d: %w", move.TagID, err)
		}
		result := database.DB.Where(
			"child_id = ? AND relation_type = ?", move.TagID, "abstract",
		).Delete(&models.TopicTagRelation{})
		if result.Error != nil {
			return fmt.Errorf("detach tag %d: %w", move.TagID, result.Error)
		}
		for _, old := range oldParents {
			EnqueueAbstractTagUpdate(old.ParentID, "child_moved")
		}
		logging.Infof("Tree review: detached tag %d (reason: %s)", move.TagID, move.Reason)
		return nil
	}

	var oldParents []models.TopicTagRelation
	if err := database.DB.Where("child_id = ? AND relation_type = ?", move.TagID, "abstract").Find(&oldParents).Error; err != nil {
		return fmt.Errorf("load old parents for tag %d: %w", move.TagID, err)
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		var parent, child models.TopicTag
		if err := tx.First(&parent, move.ToParent).Error; err != nil {
			return fmt.Errorf("load parent tag %d: %w", move.ToParent, err)
		}
		if err := tx.First(&child, move.TagID).Error; err != nil {
			return fmt.Errorf("load child tag %d: %w", move.TagID, err)
		}
		if parent.Kind != "abstract" && parent.Source != "abstract" {
			return fmt.Errorf("parent %d (%q) is not abstract", move.ToParent, parent.Label)
		}
		if child.Kind != "abstract" && child.Source != "abstract" {
			return fmt.Errorf("child %d (%q) is not abstract", move.TagID, child.Label)
		}
		wouldCycle, err := wouldCreateCycle(tx, move.TagID, move.ToParent)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", move.ToParent, move.TagID)
		}
		childSubtreeDepth := getAbstractSubtreeDepth(tx, move.TagID)
		parentAncestryDepth := getTagDepthFromRootDB(tx, move.ToParent)
		if childSubtreeDepth+parentAncestryDepth+1 > maxHierarchyDepth {
			return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth %d", childSubtreeDepth, parentAncestryDepth, maxHierarchyDepth)
		}

		var count int64
		if err := tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ? AND relation_type = ?", move.ToParent, move.TagID, "abstract").
			Count(&count).Error; err != nil {
			return fmt.Errorf("check existing relation: %w", err)
		}
		if count == 0 {
			relation := models.TopicTagRelation{ParentID: move.ToParent, ChildID: move.TagID, RelationType: "abstract"}
			if err := tx.Create(&relation).Error; err != nil {
				return fmt.Errorf("create new parent relation: %w", err)
			}
		}

		for _, old := range oldParents {
			if old.ParentID == move.ToParent {
				continue
			}
			if err := tx.Delete(&models.TopicTagRelation{}, old.ID).Error; err != nil {
				return fmt.Errorf("delete old parent relation %d for tag %d: %w", old.ID, move.TagID, err)
			}
		}
		return nil
	}); err != nil {
		logging.Warnf("Tree review: move %d -> %d failed, keeping old parents: %v", move.TagID, move.ToParent, err)
		return err
	}

	for _, old := range oldParents {
		if old.ParentID == move.ToParent {
			continue
		}
		EnqueueAbstractTagUpdate(old.ParentID, "child_moved")
	}
	EnqueueAbstractTagUpdate(move.ToParent, "child_adopted")

	logging.Infof("Tree review: moved tag %d under %d (reason: %s)", move.TagID, move.ToParent, move.Reason)
	return nil
}

func validateAndCreateReviewAbstract(abs treeReviewAbstract, tagMap map[uint]*TreeNode, category string) (bool, error) {
	if len(abs.ChildrenIDs) < 2 {
		return false, fmt.Errorf("need at least 2 children, got %d", len(abs.ChildrenIDs))
	}
	for _, id := range abs.ChildrenIDs {
		node, ok := tagMap[id]
		if !ok {
			return false, fmt.Errorf("child %d not in tree", id)
		}
		if node.Tag.Status != "active" {
			return false, fmt.Errorf("child %d not active", id)
		}
	}

	var candidates []TagCandidate
	for _, id := range abs.ChildrenIDs {
		candidates = append(candidates, TagCandidate{Tag: tagMap[id].Tag, Similarity: 0.9})
	}

	if existing := findSimilarExistingAbstractFn(context.Background(), abs.Name, abs.Description, category, candidates); existing != nil {
		logging.Infof("Tree review: reusing existing abstract %d (%q) instead of creating %q", existing.ID, existing.Label, abs.Name)
		if err := attachChildrenToReviewAbstract(existing.ID, abs.ChildrenIDs); err != nil {
			return false, err
		}
		return false, nil
	}

	slug := topictypes.Slugify(abs.Name)
	if slug == "" {
		return false, fmt.Errorf("generated empty slug for abstract name %q", abs.Name)
	}
	var existingBySlug models.TopicTag
	if err := database.DB.Where("slug = ? AND category = ? AND status = ?", slug, category, "active").First(&existingBySlug).Error; err == nil {
		if err := attachChildrenToReviewAbstract(existingBySlug.ID, abs.ChildrenIDs); err != nil {
			return false, err
		}
		return false, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("check existing abstract slug %q: %w", slug, err)
	}
	for _, id := range abs.ChildrenIDs {
		if tagMap[id].Tag.Slug == slug {
			logging.Infof("Tree review: abstract name %q (slug=%s) collides with candidate tag, skipping", abs.Name, slug)
			return false, nil
		}
	}

	treeCleanupAbs := treeCleanupAbstract(abs)
	return true, createAbstractTagDirectly(treeCleanupAbs, tagMap, category)
}

func attachChildrenToReviewAbstract(parentID uint, childIDs []uint) error {
	for _, childID := range childIDs {
		if childID == parentID {
			continue
		}
		if err := createReviewAbstractRelation(parentID, childID); err != nil {
			return err
		}
	}
	return nil
}

func createReviewAbstractRelation(parentID, childID uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var parent, child models.TopicTag
		if err := tx.First(&parent, parentID).Error; err != nil {
			return fmt.Errorf("load parent tag %d: %w", parentID, err)
		}
		if err := tx.First(&child, childID).Error; err != nil {
			return fmt.Errorf("load child tag %d: %w", childID, err)
		}
		if parent.Kind != "abstract" && parent.Source != "abstract" {
			return fmt.Errorf("parent %d (%q) is not abstract", parentID, parent.Label)
		}
		if child.Kind != "abstract" && child.Source != "abstract" {
			return fmt.Errorf("child %d (%q) is not abstract", childID, child.Label)
		}
		wouldCycle, err := wouldCreateCycle(tx, childID, parentID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", parentID, childID)
		}
		childSubtreeDepth := getAbstractSubtreeDepth(tx, childID)
		parentAncestryDepth := getTagDepthFromRootDB(tx, parentID)
		if childSubtreeDepth+parentAncestryDepth+1 > maxHierarchyDepth {
			return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth %d", childSubtreeDepth, parentAncestryDepth, maxHierarchyDepth)
		}
		var count int64
		if err := tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ? AND relation_type = ?", parentID, childID, "abstract").
			Count(&count).Error; err != nil {
			return fmt.Errorf("check existing relation: %w", err)
		}
		if count > 0 {
			return nil
		}
		relation := models.TopicTagRelation{ParentID: parentID, ChildID: childID, RelationType: "abstract", SimilarityScore: 0.9}
		return tx.Create(&relation).Error
	})
}

func createAbstractTagDirectly(abstract treeCleanupAbstract, tagMap map[uint]*TreeNode, category string) error {
	slug := topictypes.Slugify(abstract.Name)
	if slug == "" {
		return fmt.Errorf("generated empty slug for abstract name %q", abstract.Name)
	}

	for _, childID := range abstract.ChildrenIDs {
		node := tagMap[childID]
		if node.Tag.Slug == slug {
			logging.Infof("Hierarchy cleanup: abstract name %q (slug=%s) collides with candidate tag, skipping", abstract.Name, slug)
			return nil
		}
	}

	var candidates []TagCandidate
	for _, childID := range abstract.ChildrenIDs {
		node := tagMap[childID]
		candidates = append(candidates, TagCandidate{
			Tag:        node.Tag,
			Similarity: 0.9,
		})
	}

	var abstractTag *models.TopicTag
	if existingAbstract := findSimilarExistingAbstractFn(context.Background(), abstract.Name, abstract.Description, category, candidates); existingAbstract != nil {
		abstractTag = existingAbstract
	}

	var abstractChildren []*models.TopicTag

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if abstractTag == nil {
			var existing models.TopicTag
			if err := tx.Where("slug = ? AND category = ? AND status = ?", slug, category, "active").First(&existing).Error; err == nil {
				abstractTag = &existing
			}
		}

		if abstractTag == nil {
			abstractTag = &models.TopicTag{
				Slug:        slug,
				Label:       abstract.Name,
				Category:    category,
				Kind:        category,
				Source:      "abstract",
				Status:      "active",
				Description: abstract.Description,
			}
			if err := tx.Create(abstractTag).Error; err != nil {
				return fmt.Errorf("create abstract tag: %w", err)
			}
		}

		for _, childID := range abstract.ChildrenIDs {
			node := tagMap[childID]
			if node.Tag.ID == abstractTag.ID {
				continue
			}

			wouldCycle, err := wouldCreateCycle(tx, abstractTag.ID, node.Tag.ID)
			if err != nil {
				return fmt.Errorf("check cycle for child %d: %w", node.Tag.ID, err)
			}
			if wouldCycle {
				logging.Warnf("Hierarchy cleanup: skipping cyclic relation: abstract %d -> child %d", abstractTag.ID, node.Tag.ID)
				continue
			}

			childSubtreeDepth := getAbstractSubtreeDepth(tx, node.Tag.ID)
			parentAncestryDepth := getTagDepthFromRootDB(tx, abstractTag.ID)
			if childSubtreeDepth+parentAncestryDepth+1 > maxHierarchyDepth {
				logging.Warnf("Hierarchy cleanup: skipping depth overflow relation: abstract %d -> child %d (subtree=%d, ancestry=%d)",
					abstractTag.ID, node.Tag.ID, childSubtreeDepth, parentAncestryDepth)
				continue
			}

			var count int64
			tx.Model(&models.TopicTagRelation{}).
				Where("parent_id = ? AND child_id = ? AND relation_type = ?", abstractTag.ID, node.Tag.ID, "abstract").
				Count(&count)
			if count > 0 {
				abstractChildren = append(abstractChildren, node.Tag)
				continue
			}

			relation := models.TopicTagRelation{
				ParentID:        abstractTag.ID,
				ChildID:         node.Tag.ID,
				RelationType:    "abstract",
				SimilarityScore: 0.9,
			}
			if err := tx.Create(&relation).Error; err != nil {
				return fmt.Errorf("create tag relation for child %d: %w", node.Tag.ID, err)
			}
			abstractChildren = append(abstractChildren, node.Tag)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("abstract tag transaction failed: %w", err)
	}

	logging.Infof("Hierarchy cleanup: created abstract tag %d (%s) with %d children",
		abstractTag.ID, abstractTag.Label, len(abstractChildren))

	go func(tagID uint, name, cat string) {
		defer func() {
			if r := recover(); r != nil {
				logging.Warnf("Hierarchy cleanup: async post-create task panic for abstract tag %d: %v", tagID, r)
			}
		}()
		es := NewEmbeddingService()
		tag := &models.TopicTag{ID: tagID, Label: name, Category: cat}
		for _, embType := range []string{EmbeddingTypeIdentity, EmbeddingTypeSemantic} {
			emb, genErr := es.GenerateEmbedding(context.Background(), tag, embType)
			if genErr != nil {
				logging.Warnf("Hierarchy cleanup: failed to generate %s embedding for abstract tag %d: %v", embType, tagID, genErr)
				continue
			}
			emb.TopicTagID = tagID
			if saveErr := es.SaveEmbedding(emb); saveErr != nil {
				logging.Warnf("Hierarchy cleanup: failed to save %s embedding for abstract tag %d: %v", embType, tagID, saveErr)
			}
		}
		MatchAbstractTagHierarchy(context.Background(), tagID)
		EnqueueAdoptNarrower(tagID, "createAbstractTagDirectly")
	}(abstractTag.ID, abstract.Name, category)

	go EnqueueAbstractTagUpdate(abstractTag.ID, "new_child_added")

	for _, child := range abstractChildren {
		go func(childID uint) {
			defer func() {
				if r := recover(); r != nil {
					logging.Warnf("Hierarchy cleanup: multi-parent conflict task panic for child tag %d: %v", childID, r)
				}
			}()
			_, _ = resolveMultiParentConflict(childID)
		}(child.ID)
	}

	return nil
}

func Phase6_CheckLevelAlignment(forest []*TreeNode, tmpl *CategoryHierarchyTemplate) []string {
	var issues []string
	for _, root := range forest {
		checkNodeLevelAlignment(root, tmpl, &issues)
	}
	return issues
}

func checkNodeLevelAlignment(node *TreeNode, tmpl *CategoryHierarchyTemplate, issues *[]string) {
	depth := node.Depth
	if depth+1 > tmpl.MaxLevel {
		*issues = append(*issues, fmt.Sprintf("tag %d(%s) depth %d exceeds max %d",
			node.Tag.ID, node.Tag.Label, depth+1, tmpl.MaxLevel))
	}
	for _, child := range node.Children {
		checkNodeLevelAlignment(child, tmpl, issues)
	}
}

func Phase6_DedupL1(ctx context.Context, tmpl *CategoryHierarchyTemplate) (int, error) {
	l1Tags, err := loadExistingL1Tags(tmpl.Category, tmpl)
	if err != nil {
		return 0, fmt.Errorf("load L1 tags: %w", err)
	}
	merged := 0
	for _, tag := range l1Tags {
		if tag.Status != "active" {
			continue
		}
		dedupL1(ctx, tag)
		var count int64
		database.DB.Model(&models.TopicTag{}).Where("id = ? AND status = 'active'", tag.ID).Count(&count)
		if count == 0 {
			merged++
		}
	}
	return merged, nil
}

func Phase6_DedupL2(ctx context.Context, tmpl *CategoryHierarchyTemplate) (int, error) {
	var l2Tags []models.TopicTag
	if err := database.DB.Where("category = ? AND source = ? AND status = 'active'", tmpl.Category, "abstract").Find(&l2Tags).Error; err != nil {
		return 0, fmt.Errorf("load L2 tags: %w", err)
	}
	merged := 0
	for i := range l2Tags {
		depth := getTagDepthFromRoot(l2Tags[i].ID)
		if depth == 1 {
			dedupL2(ctx, &l2Tags[i])
			var count int64
			database.DB.Model(&models.TopicTag{}).Where("id = ? AND status = 'active'", l2Tags[i].ID).Count(&count)
			if count == 0 {
				merged++
			}
		}
	}
	return merged, nil
}

func Phase6_SampleAuditLeaves(ctx context.Context, tmpl *CategoryHierarchyTemplate) (int, error) {
	leafLevel := tmpl.GetLeafLevel()
	var leafTags []models.TopicTag
	if err := database.DB.Where("category = ? AND status = 'active'", tmpl.Category).Find(&leafTags).Error; err != nil {
		return 0, fmt.Errorf("load leaf tags: %w", err)
	}

	issues := 0
	for _, tag := range leafTags {
		depth := getTagDepthFromRoot(tag.ID)
		level := ResolveLevelFromDepth(tag.Category, depth)
		if level == leafLevel {
			var parentRel models.TopicTagRelation
			if err := database.DB.Where("child_id = ? AND relation_type = 'abstract'", tag.ID).
				Preload("Parent").First(&parentRel).Error; err != nil {
				continue
			}
			if parentRel.Parent != nil && parentRel.Parent.Category != tag.Category {
				issues++
			}
		}
	}
	return issues, nil
}
