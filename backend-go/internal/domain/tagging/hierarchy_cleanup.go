package tagging

import (
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

const MinTreeDepthForCleanup = 3

type LLMBudget interface {
	ConsumeForPhase(phase string) bool
	IsTimedOut() bool
}

type TreeNode struct {
	Tag          *models.TopicTag
	Depth        int
	Children     []*TreeNode
	Parent       *TreeNode
	ArticleCount int
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


