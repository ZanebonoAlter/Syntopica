package topicanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

type ZombieTagCriteria struct {
	MinAgeDays int
	Categories []string
}

func CleanupZombieTags(criteria ZombieTagCriteria) (int, error) {
	result := buildZombieTagQuery(database.DB.Model(&models.TopicTag{}), criteria)

	var count int64
	if err := result.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count zombie tags: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	if err := result.Updates(map[string]interface{}{
		"status": "inactive",
	}).Error; err != nil {
		return 0, fmt.Errorf("deactivate zombie tags: %w", err)
	}

	logging.Infof("CleanupZombieTags: deactivated %d zombie tags", count)
	return int(count), nil
}

func buildZombieTagQuery(db *gorm.DB, criteria ZombieTagCriteria) *gorm.DB {
	query := db.Where("status = ?", "active")
	if len(criteria.Categories) > 0 {
		query = query.Where("category IN ?", criteria.Categories)
	}
	if criteria.MinAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -criteria.MinAgeDays)
		query = query.Where("created_at < ?", cutoff)
	}

	return query.
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE (r.parent_id = topic_tags.id OR r.child_id = topic_tags.id) AND r.relation_type = ?)", "abstract").
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)")
}

func BuildZombieTagSubQuery(criteria ZombieTagCriteria) string {
	return fmt.Sprintf(`
		SELECT t.id FROM topic_tags t
		WHERE t.status = 'active'
		  AND t.category IN (%s)
		  AND t.created_at < NOW() - INTERVAL '%d days'
		  AND NOT EXISTS (
		    SELECT 1 FROM topic_tag_relations r
		    WHERE (r.parent_id = t.id OR r.child_id = t.id) AND r.relation_type = 'abstract'
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM article_topic_tags att
		    WHERE att.topic_tag_id = t.id
		  )
		`, quoteCategories(criteria.Categories), criteria.MinAgeDays)
}

func quoteCategories(categories []string) string {
	quoted := ""
	for i, c := range categories {
		if i > 0 {
			quoted += ", "
		}
		quoted += fmt.Sprintf("'%s'", c)
	}
	return quoted
}

type FlatTagInfo struct {
	ID           uint               `json:"id"`
	Label        string             `json:"label"`
	Description  string             `json:"description"`
	Source       string             `json:"source"`
	ArticleCount int                `json:"article_count"`
	ChildCount   int                `json:"child_count"`
	Metadata     models.MetadataMap `json:"person_attrs,omitempty"`
}

type flatMergeJudgment struct {
	Merges []flatMergeItem `json:"merges,omitempty"`
	Notes  string          `json:"notes,omitempty"`
}

type flatMergeItem struct {
	SourceID uint   `json:"source_id"`
	TargetID uint   `json:"target_id"`
	Reason   string `json:"reason"`
}

func CollectFlatTagBatch(category string, batchSize int) ([]FlatTagInfo, error) {
	var tags []models.TopicTag
	if err := database.DB.
		Where("category = ? AND status = 'active' AND source = 'abstract'", category).
		Limit(batchSize).
		Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("load abstract tags: %w", err)
	}

	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	articleCounts := countArticlesByTag(tagIDs, "")

	childCounts := make(map[uint]int)
	var childRows []struct {
		ParentID uint `gorm:"column:parent_id"`
		Cnt      int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTagRelation{}).
		Select("parent_id, count(*) as cnt").
		Where("parent_id IN ? AND relation_type = 'abstract'", tagIDs).
		Group("parent_id").
		Scan(&childRows)
	for _, r := range childRows {
		childCounts[r.ParentID] = r.Cnt
	}

	result := make([]FlatTagInfo, len(tags))
	for i, t := range tags {
		result[i] = FlatTagInfo{
			ID:           t.ID,
			Label:        t.Label,
			Description:  truncateStr(t.Description, 200),
			Source:       t.Source,
			ArticleCount: articleCounts[t.ID],
			ChildCount:   childCounts[t.ID],
			Metadata:     t.Metadata,
		}
	}
	return result, nil
}

func BuildFlatMergePrompt(tags []FlatTagInfo, category string) string {
	promptData := map[string]interface{}{
		"category": category,
		"total":    len(tags),
		"tags":     tags,
	}

	promptJSON, _ := json.MarshalIndent(promptData, "", "  ")

	return fmt.Sprintf(`你是一位标签分类专家。请分析以下 %s 类别的抽象标签列表，找出语义重复或高度相似的标签对。

标签列表：
%s

请返回以下格式的 JSON：
{
  "merges": [
    {
      "source_id": 123,
      "target_id": 456,
      "reason": "这两个标签描述的是同一个概念，应该合并"
    }
  ],
  "notes": "其他观察（可选）"
}

规则：
1. merges 是可选的，可以为空数组
2. source_id: 被合并的标签（子标签数更少或描述更窄的那个）
3. target_id: 保留的目标标签（子标签数更多或描述更广的那个）
4. 只合并真正描述同一核心概念的标签，不要合并仅有部分重叠的标签
5. 如果没有需要合并的，返回空数组
6. 只返回真正有把握的建议`, category, string(promptJSON))
}

func ExecuteFlatMerge(ctx context.Context, category string, batchSize int) (int, []string, error) {
	tags, err := CollectFlatTagBatch(category, batchSize)
	if err != nil {
		return 0, nil, fmt.Errorf("collect tags: %w", err)
	}
	if len(tags) == 0 {
		return 0, nil, nil
	}

	prompt := BuildFlatMergePrompt(tags, category)
	judgment, err := callFlatMergeLLM(ctx, prompt)
	if err != nil {
		return 0, nil, fmt.Errorf("LLM call: %w", err)
	}

	tagMap := make(map[uint]*FlatTagInfo)
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	var errors []string
	merged := 0
	for _, merge := range judgment.Merges {
		if err := validateFlatMerge(merge, tagMap); err != nil {
			errors = append(errors, fmt.Sprintf("merge %d→%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		if err := MergeTags(merge.SourceID, merge.TargetID); err != nil {
			errors = append(errors, fmt.Sprintf("merge %d→%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		merged++
	}

	logging.Infof("ExecuteFlatMerge(%s): %d tags analyzed, %d merges applied", category, len(tags), merged)
	return merged, errors, nil
}

func validateFlatMerge(merge flatMergeItem, tagMap map[uint]*FlatTagInfo) error {
	source, ok := tagMap[merge.SourceID]
	if !ok {
		return fmt.Errorf("source %d not found", merge.SourceID)
	}
	target, ok := tagMap[merge.TargetID]
	if !ok {
		return fmt.Errorf("target %d not found", merge.TargetID)
	}
	if merge.SourceID == merge.TargetID {
		return fmt.Errorf("same tag")
	}
	if source.ChildCount > 0 && target.ChildCount > 0 {
		if source.ChildCount > target.ChildCount {
			return fmt.Errorf("source has more children (%d) than target (%d), swap recommended", source.ChildCount, target.ChildCount)
		}
	}
	return nil
}

func callFlatMergeLLM(ctx context.Context, prompt string) (*flatMergeJudgment, error) {
	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy cleanup assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
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
				"notes": {Type: "string"},
			},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata:    map[string]any{"operation": "tag_flat_merge"},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment flatMergeJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	return &judgment, nil
}

func CleanupOrphanedRelations() (int, error) {
	result := database.DB.Where(
		"relation_type = 'abstract' AND (parent_id IN (SELECT id FROM topic_tags WHERE status != 'active') OR child_id IN (SELECT id FROM topic_tags WHERE status != 'active'))",
	).Delete(&models.TopicTagRelation{})

	if result.Error != nil {
		return 0, fmt.Errorf("cleanup orphaned relations: %w", result.Error)
	}

	deleted := int(result.RowsAffected)
	if deleted > 0 {
		logging.Infof("CleanupOrphanedRelations: removed %d orphaned relations", deleted)
	}
	return deleted, nil
}

func CleanupMultiParentConflicts() (int, []string, error) {
	var conflicts []struct {
		ChildID uint `gorm:"column:child_id"`
		Cnt     int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTagRelation{}).
		Select("child_id, count(*) as cnt").
		Where("relation_type = 'abstract'").
		Group("child_id").
		Having("count(*) > 1").
		Scan(&conflicts)

	if len(conflicts) == 0 {
		return 0, nil, nil
	}

	// 收集所有冲突详情
	var multiConflicts []multiParentConflict
	for _, c := range conflicts {
		var relations []models.TopicTagRelation
		if err := database.DB.Where("child_id = ? AND relation_type = ?", c.ChildID, "abstract").
			Preload("Parent").Find(&relations).Error; err != nil {
			continue
		}
		var parents []parentWithInfo
		var childTag models.TopicTag
		for _, r := range relations {
			if r.Parent != nil {
				parents = append(parents, parentWithInfo{RelationID: r.ID, Parent: r.Parent, SimilarityScore: r.SimilarityScore})
			}
		}
		if len(parents) <= 1 {
			continue
		}
		if err := database.DB.First(&childTag, c.ChildID).Error; err != nil {
			continue
		}
		multiConflicts = append(multiConflicts, multiParentConflict{
			ChildID: c.ChildID,
			Parents: parents,
			Child:   &childTag,
		})
	}

	// 批量解决（每批最多 10 个冲突）
	batchSize := 10
	totalResolved := 0
	var allErrors []string
	for i := 0; i < len(multiConflicts); i += batchSize {
		end := i + batchSize
		if end > len(multiConflicts) {
			end = len(multiConflicts)
		}
		batch := multiConflicts[i:end]
		resolved, errors := batchResolveMultiParentConflicts(batch)
		totalResolved += resolved
		allErrors = append(allErrors, errors...)
	}

	logging.Infof("CleanupMultiParentConflicts: resolved %d conflicts", totalResolved)
	return totalResolved, allErrors, nil
}

func deactivateTagsWithCleanup(tagIDs []uint) error {
	if len(tagIDs) == 0 {
		return nil
	}
	database.DB.Where("topic_tag_id IN ?", tagIDs).Delete(&models.TopicTagEmbedding{})
	database.DB.Where("parent_id IN ? OR child_id IN ?", tagIDs, tagIDs).
		Where("relation_type = ?", "abstract").Delete(&models.TopicTagRelation{})
	return database.DB.Model(&models.TopicTag{}).Where("id IN ?", tagIDs).
		Updates(map[string]interface{}{"status": "inactive"}).Error
}

func CleanupZeroArticleTags(categories []string) (int, error) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND kind != ? AND source != ?", "active", "abstract", "abstract").
		Where("category IN ?", categories).
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)")

	var ids []uint
	if err := query.Pluck("topic_tags.id", &ids).Error; err != nil {
		return 0, fmt.Errorf("pluck zero-article tag ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	if err := deactivateTagsWithCleanup(ids); err != nil {
		return 0, fmt.Errorf("cleanup zero-article tags: %w", err)
	}

	logging.Infof("CleanupZeroArticleTags: deactivated %d zero-article tags in categories %v", len(ids), categories)
	return len(ids), nil
}

func CleanupLowQualitySingleArticleTags(category string, maxScore float64) (int, error) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND kind != ? AND source != ?", "active", "abstract", "abstract").
		Where("category = ?", category).
		Where("quality_score < ?", maxScore).
		Where("(SELECT COUNT(*) FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id) = 1")

	var ids []uint
	if err := query.Pluck("topic_tags.id", &ids).Error; err != nil {
		return 0, fmt.Errorf("pluck low-quality single-article tag ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	if err := deactivateTagsWithCleanup(ids); err != nil {
		return 0, fmt.Errorf("cleanup low-quality single-article tags: %w", err)
	}

	logging.Infof("CleanupLowQualitySingleArticleTags: deactivated %d low-quality single-article tags in category %s (maxScore=%.2f)", len(ids), category, maxScore)
	return len(ids), nil
}

func CleanupStaleZeroScoreTags(ageDays int) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -ageDays)
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND kind != ? AND source != ?", "active", "abstract", "abstract").
		Where("quality_score < ?", 0.05).
		Where("created_at < ?", cutoff)

	var ids []uint
	if err := query.Pluck("topic_tags.id", &ids).Error; err != nil {
		return 0, fmt.Errorf("pluck stale zero-score tag ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	if err := deactivateTagsWithCleanup(ids); err != nil {
		return 0, fmt.Errorf("cleanup stale zero-score tags: %w", err)
	}

	logging.Infof("CleanupStaleZeroScoreTags: deactivated %d stale zero-score tags (age > %d days)", len(ids), ageDays)
	return len(ids), nil
}

func CleanupEmptyAbstractNodes() (int, error) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("source = ? AND status = ?", "abstract", "active").
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE r.parent_id = topic_tags.id AND r.relation_type = ?)", "abstract")

	var ids []uint
	if err := query.Pluck("topic_tags.id", &ids).Error; err != nil {
		return 0, fmt.Errorf("load empty abstract ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	if err := database.DB.Model(&models.TopicTag{}).Where("id IN ?", ids).Updates(map[string]interface{}{
		"status": "inactive",
	}).Error; err != nil {
		return 0, fmt.Errorf("cleanup empty abstracts: %w", err)
	}

	logging.Infof("CleanupEmptyAbstractNodes: deactivated %d empty abstract tags", len(ids))
	return len(ids), nil
}

type singleChildRow struct {
	ParentID uint
	ChildID  uint
}

func CleanupSingleChildAbstractNodes() (int, error) {
	var rows []singleChildRow
	if err := database.DB.Raw(`
		SELECT r.parent_id, MIN(r.child_id) AS child_id
		FROM topic_tag_relations r
		JOIN topic_tags p ON p.id = r.parent_id
		WHERE r.relation_type = 'abstract'
		  AND p.source = 'abstract'
		  AND p.status = 'active'
		GROUP BY r.parent_id
		HAVING COUNT(DISTINCT r.child_id) = 1
	`).Scan(&rows).Error; err != nil {
		return 0, fmt.Errorf("load single-child abstract nodes: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	count := 0
	for _, row := range rows {
		if err := promoteSingleChild(row.ParentID, row.ChildID); err != nil {
			logging.Warnf("CleanupSingleChildAbstractNodes: failed to promote child %d for parent %d: %v",
				row.ChildID, row.ParentID, err)
			continue
		}
		count++
	}

	logging.Infof("CleanupSingleChildAbstractNodes: promoted %d children, deactivated single-child abstract parents", count)
	return count, nil
}

func promoteSingleChild(parentID, childID uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var grandparentIDs []uint
		tx.Model(&models.TopicTagRelation{}).
			Where("relation_type = ? AND child_id = ?", "abstract", parentID).
			Pluck("parent_id", &grandparentIDs)

		for _, gpID := range grandparentIDs {
			var existing int64
			tx.Model(&models.TopicTagRelation{}).
				Where("relation_type = ? AND parent_id = ? AND child_id = ?", "abstract", gpID, childID).
				Count(&existing)
			if existing == 0 {
				if err := tx.Create(&models.TopicTagRelation{
					ParentID:        gpID,
					ChildID:         childID,
					RelationType:    "abstract",
					SimilarityScore: 0,
				}).Error; err != nil {
					return fmt.Errorf("link grandparent %d to child %d: %w", gpID, childID, err)
				}
			}
		}

		tx.Where("relation_type = ? AND child_id = ?", "abstract", parentID).Delete(&models.TopicTagRelation{})
		tx.Where("relation_type = ? AND parent_id = ? AND child_id = ?", "abstract", parentID, childID).Delete(&models.TopicTagRelation{})

		tx.Where("topic_tag_id = ?", parentID).Delete(&models.TopicTagEmbedding{})
		return tx.Model(&models.TopicTag{}).Where("id = ?", parentID).
			Updates(map[string]interface{}{"status": "inactive"}).Error
	})
}

// normalizeSlugForComparison strips all whitespace from a slug for dedup comparison.
var whitespacePattern = regexp.MustCompile(`\s+`)

func normalizeSlugForComparison(slug string) string {
	return whitespacePattern.ReplaceAllString(slug, "")
}

// CleanupWhitespaceDuplicateTags finds active tags whose slugs differ only in whitespace
// and merges them, keeping the tag with more article associations.
func CleanupWhitespaceDuplicateTags() (int, error) {
	var tags []models.TopicTag
	if err := database.DB.Where("status = ?", "active").Find(&tags).Error; err != nil {
		return 0, fmt.Errorf("load active tags: %w", err)
	}

	// Group by category + normalized slug (spaces stripped)
	type groupKey struct {
		Category       string
		NormalizedSlug string
	}
	groups := make(map[groupKey][]*models.TopicTag)
	for i := range tags {
		key := groupKey{
			Category:       tags[i].Category,
			NormalizedSlug: normalizeSlugForComparison(tags[i].Slug),
		}
		groups[key] = append(groups[key], &tags[i])
	}

	merged := 0
	for key, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Find the tag with most articles to keep as survivor
		articleCounts := countArticlesByTag(tagPtrsToIDs(group), "")
		var survivor *models.TopicTag
		maxCount := -1
		for _, tag := range group {
			count := articleCounts[tag.ID]
			if count > maxCount {
				maxCount = count
				survivor = tag
			}
		}
		if survivor == nil {
			continue
		}

		for _, tag := range group {
			if tag.ID == survivor.ID {
				continue
			}
			if err := MergeTags(tag.ID, survivor.ID); err != nil {
				logging.Warnf("CleanupWhitespaceDuplicateTags: failed to merge tag %d (%q) into %d (%q): %v",
					tag.ID, tag.Label, survivor.ID, survivor.Label, err)
				continue
			}
			merged++
			logging.Infof("CleanupWhitespaceDuplicateTags: merged whitespace-variant %d (%q, slug=%q) into %d (%q, slug=%q) [category=%s]",
				tag.ID, tag.Label, tag.Slug, survivor.ID, survivor.Label, survivor.Slug, key.Category)
		}
	}

	logging.Infof("CleanupWhitespaceDuplicateTags: merged %d whitespace-variant duplicates", merged)
	return merged, nil
}

func tagPtrsToIDs(tags []*models.TopicTag) []uint {
	ids := make([]uint, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	return ids
}

// CleanupDegenerateAbstractTrees walks abstract tag chains, computes leaf-to-depth ratio,
// and flattens degenerate chains by promoting children to the nearest ancestor with
// a healthy ratio (>= 1.5). Intermediate nodes in degenerate chains are deactivated.
func CleanupDegenerateAbstractTrees() (int, error) {
	// Find root abstract tags (active abstract with no active abstract parent)
	var rootIDs []uint
	database.DB.Raw(`
		SELECT t.id FROM topic_tags t
		WHERE t.source = 'abstract' AND t.status = 'active'
		AND NOT EXISTS (
			SELECT 1 FROM topic_tag_relations r
			JOIN topic_tags p ON p.id = r.parent_id
			WHERE r.child_id = t.id AND r.relation_type = 'abstract'
			AND p.status = 'active'
		)
	`).Pluck("id", &rootIDs)

	flattened := 0
	for _, rootID := range rootIDs {
		count, err := flattenDegenerateAbstractSubtree(database.DB, rootID, 0)
		if err != nil {
			logging.Warnf("CleanupDegenerateAbstractTrees: error on root %d: %v", rootID, err)
			continue
		}
		flattened += count
	}

	logging.Infof("CleanupDegenerateAbstractTrees: flattened %d degenerate nodes", flattened)
	return flattened, nil
}

// flattenDegenerateAbstractSubtree recurses through an abstract tree.
// If the leaf-to-depth ratio for the subtree rooted at tagID is < 1.5,
// it flattens intermediate nodes by promoting their children upward.
// Returns the count of nodes deactivated.
func flattenDegenerateAbstractSubtree(db *gorm.DB, tagID uint, depth int) (int, error) {
	var childIDs []uint
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = ?", tagID, "abstract").
		Pluck("child_id", &childIDs)

	if len(childIDs) == 0 {
		// Leaf node: count as 1 leaf at current depth
		return 0, nil
	}

	// Recursively process children first (bottom-up flattening)
	flattened := 0
	for _, childID := range childIDs {
		count, err := flattenDegenerateAbstractSubtree(db, childID, depth+1)
		if err != nil {
			return flattened, err
		}
		flattened += count
	}

	// Check if this subtree is degenerate (using local subtree depth)
	var leafIDs []uint
	var maxLeafDepth int
	collectLeaves(db, tagID, 0, &leafIDs, &maxLeafDepth)
	totalDepth := maxLeafDepth
	if totalDepth < 1 {
		totalDepth = 1
	}
	ratio := float64(len(leafIDs)) / float64(totalDepth)

	if ratio < 1.5 && depth > 0 {
		// This node is intermediate and creates a degenerate chain
		// Promote its children to its parent(s)
		flattened += flattenDegenerateNode(db, tagID, childIDs)
	}

	return flattened, nil
}

// collectLeaves finds all leaf tags in the subtree and the max depth.
func collectLeaves(db *gorm.DB, tagID uint, currentDepth int, leafIDs *[]uint, maxDepth *int) int {
	visited := make(map[uint]bool)
	return collectLeavesVisited(db, tagID, currentDepth, leafIDs, maxDepth, visited)
}

func collectLeavesVisited(db *gorm.DB, tagID uint, currentDepth int, leafIDs *[]uint, maxDepth *int, visited map[uint]bool) int {
	if visited[tagID] {
		return currentDepth
	}
	visited[tagID] = true

	var childIDs []uint
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = ?", tagID, "abstract").
		Pluck("child_id", &childIDs)

	if currentDepth > *maxDepth {
		*maxDepth = currentDepth
	}

	if len(childIDs) == 0 {
		*leafIDs = append(*leafIDs, tagID)
		return currentDepth
	}

	deepestChild := currentDepth
	for _, childID := range childIDs {
		d := collectLeavesVisited(db, childID, currentDepth+1, leafIDs, maxDepth, visited)
		if d > deepestChild {
			deepestChild = d
		}
	}
	return deepestChild
}

// flattenDegenerateNode promotes children of a degenerate intermediate node to its
// parent(s) and deactivates the intermediate node. Returns the count (1 if successful).
func flattenDegenerateNode(db *gorm.DB, tagID uint, childIDs []uint) int {
	err := db.Transaction(func(tx *gorm.DB) error {
		var parentIDs []uint
		tx.Model(&models.TopicTagRelation{}).
			Where("child_id = ? AND relation_type = ?", tagID, "abstract").
			Pluck("parent_id", &parentIDs)

		// Link each child to each grandparent
		for _, childID := range childIDs {
			for _, parentID := range parentIDs {
				var count int64
				tx.Model(&models.TopicTagRelation{}).
					Where("parent_id = ? AND child_id = ? AND relation_type = ?",
						parentID, childID, "abstract").
					Count(&count)
				if count > 0 {
					continue
				}
				if wouldCycle, err := wouldCreateCycle(tx, parentID, childID); err != nil || wouldCycle {
					continue
				}
				if err := tx.Create(&models.TopicTagRelation{
					ParentID:     parentID,
					ChildID:      childID,
					RelationType: "abstract",
				}).Error; err != nil {
					return fmt.Errorf("link grandparent %d to child %d: %w", parentID, childID, err)
				}
			}
		}

		// Remove old relations for this node
		tx.Where("parent_id = ? AND relation_type = ?", tagID, "abstract").Delete(&models.TopicTagRelation{})
		tx.Where("child_id = ? AND relation_type = ?", tagID, "abstract").Delete(&models.TopicTagRelation{})

		// Deactivate the node
		tx.Where("topic_tag_id = ?", tagID).Delete(&models.TopicTagEmbedding{})
		if err := tx.Model(&models.TopicTag{}).Where("id = ?", tagID).
			Updates(map[string]interface{}{"status": "inactive"}).Error; err != nil {
			return fmt.Errorf("deactivate tag %d: %w", tagID, err)
		}

		logging.Infof("CleanupDegenerateAbstractTrees: flattened node %d, promoted %d children", tagID, len(childIDs))
		return nil
	})

	if err != nil {
		logging.Warnf("CleanupDegenerateAbstractTrees: failed to flatten node %d: %v", tagID, err)
		return 0
	}
	return 1
}

type TemplateViolationResult struct {
	DepthExceeded int `json:"depth_exceeded"`
	CrossCategory int `json:"cross_category"`
	PendingAdded  int `json:"pending_added"`
}

func CleanupTemplateViolations() (*TemplateViolationResult, error) {
	result := &TemplateViolationResult{}

	var relations []models.TopicTagRelation
	if err := database.DB.Where("relation_type = 'abstract'").
		Preload("Parent").Preload("Child").Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("load relations: %w", err)
	}

	for _, r := range relations {
		if r.Parent == nil || r.Child == nil {
			continue
		}

		tmpl := GetHierarchyManager().GetTemplate(r.Parent.Category, "")
		if tmpl == nil {
			continue
		}

		childLevel := GetTagLevel(r.Child)
		if childLevel > tmpl.MaxLevel {
			result.DepthExceeded++
			createPendingChange(r.ChildID, r.Child.Label, "depth_exceeded", &r.ParentID, r.Parent.Label,
				fmt.Sprintf("Depth %d exceeds max %d for template %s", childLevel, tmpl.MaxLevel, tmpl.TemplateKey()))
		}

		if r.Parent.Category != r.Child.Category {
			result.CrossCategory++
			createPendingChange(r.ChildID, r.Child.Label, "cross_category", &r.ParentID, r.Parent.Label,
				fmt.Sprintf("Parent category %s != child category %s", r.Parent.Category, r.Child.Category))
		}
	}

	result.PendingAdded = result.DepthExceeded + result.CrossCategory
	return result, nil
}

func createPendingChange(tagID uint, label, changeType string, parentID *uint, parentLabel, reason string) {
	change := models.HierarchyPendingChange{
		TagID: tagID, TagLabel: label, ChangeType: changeType,
		CurrentParentID: parentID, CurrentParentLabel: parentLabel,
		Reason: reason, Status: "pending",
	}
	if err := database.DB.Create(&change).Error; err != nil {
		logging.Warnf("Failed to create pending change for tag %d (%s): %v", tagID, changeType, err)
	}
}
