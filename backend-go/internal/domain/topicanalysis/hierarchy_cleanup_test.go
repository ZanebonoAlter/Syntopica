package topicanalysis

import (
	"context"
	"strings"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
)

func makeTag(id uint, label string) *models.TopicTag {
	return &models.TopicTag{ID: id, Label: label, Status: "active", Category: "event"}
}

func TestSerializeTreeForReviewIncludesTreeShapeAndDescriptions(t *testing.T) {
	root := &TreeNode{Tag: &models.TopicTag{ID: 1, Label: "政治人物", Description: "人物分组", Status: "active"}, Depth: 1}
	child := &TreeNode{Tag: &models.TopicTag{ID: 2, Label: "伊朗政治人物", Description: "伊朗相关政治人物", Status: "active"}, Depth: 2, Parent: root}
	root.Children = []*TreeNode{child}

	got := serializeTreeForReview(root)

	for _, want := range []string{"[id:1] 政治人物", "描述: 人物分组", "└── [id:2] 伊朗政治人物", "描述: 伊朗相关政治人物"} {
		if !strings.Contains(got, want) {
			t.Fatalf("serialized tree missing %q in:\n%s", want, got)
		}
	}
}

func TestSerializeTreeForReviewIncludesPersonMetadata(t *testing.T) {
	root := &TreeNode{Tag: &models.TopicTag{ID: 1, Label: "体育人物", Category: "person", Status: "active"}, Depth: 1}
	child := &TreeNode{Tag: &models.TopicTag{
		ID:          2,
		Label:       "李宗伟",
		Category:    "person",
		Status:      "active",
		Description: "马来西亚羽毛球运动员",
		Metadata: models.MetadataMap{
			"country": "马来西亚",
			"role":    "羽毛球运动员",
			"domains": []any{"羽毛球"},
		},
	}, Depth: 2, Parent: root}
	root.Children = []*TreeNode{child}

	got := serializeTreeForReview(root)

	for _, want := range []string{"[id:2] 李宗伟", "国籍/地区: 马来西亚", "身份/职务: 羽毛球运动员", "领域: 羽毛球"} {
		if !strings.Contains(got, want) {
			t.Fatalf("serialized tree missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildTreeReviewPromptIncludesRulesAndTree(t *testing.T) {
	tree := "[id:1] 政治人物\n  └── [id:2] 伊朗政治人物\n"

	got := buildTreeReviewPrompt(tree, "person")

	for _, want := range []string{"person 类别", tree, "to_parent=0", "new_abstracts", "merges", "source_id", "target_id", "返回 JSON"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildTagForestMinDepth(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	tags := []models.TopicTag{
		{Label: "root2", Slug: "root2", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "child2", Slug: "child2", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "root3", Slug: "root3", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "child3", Slug: "child3", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "grandchild3", Slug: "grandchild3", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
	}
	if err := db.Create(&tags).Error; err != nil {
		t.Fatalf("create tags: %v", err)
	}
	relations := []models.TopicTagRelation{
		{ParentID: tags[0].ID, ChildID: tags[1].ID, RelationType: "abstract"},
		{ParentID: tags[2].ID, ChildID: tags[3].ID, RelationType: "abstract"},
		{ParentID: tags[3].ID, ChildID: tags[4].ID, RelationType: "abstract"},
	}
	if err := db.Create(&relations).Error; err != nil {
		t.Fatalf("create relations: %v", err)
	}

	defaultForest, err := BuildTagForest("event")
	if err != nil {
		t.Fatalf("BuildTagForest default: %v", err)
	}
	if len(defaultForest) != 1 || calculateTreeDepth(defaultForest[0]) != 3 {
		t.Fatalf("default forest = %+v, want one depth-3 tree", defaultForest)
	}

	minDepth2Forest, err := BuildTagForest("event", 2)
	if err != nil {
		t.Fatalf("BuildTagForest minDepth 2: %v", err)
	}
	if len(minDepth2Forest) != 2 {
		t.Fatalf("minDepth 2 forest len = %d, want 2", len(minDepth2Forest))
	}

	minDepth4Forest, err := BuildTagForest("event", 4)
	if err != nil {
		t.Fatalf("BuildTagForest minDepth 4: %v", err)
	}
	if len(minDepth4Forest) != 0 {
		t.Fatalf("minDepth 4 forest len = %d, want 0", len(minDepth4Forest))
	}
}

func TestRootLevelReviewTreeOnlyIncludesDirectChildren(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	child := &TreeNode{Tag: makeTag(2, "child"), Depth: 2, Parent: root}
	grandchild := &TreeNode{Tag: makeTag(3, "grandchild"), Depth: 3, Parent: child}
	root.Children = []*TreeNode{child}
	child.Children = []*TreeNode{grandchild}

	got := rootLevelReviewTree(root)

	if countNodes(got) != 2 {
		t.Fatalf("root-level review tree nodes = %d, want 2", countNodes(got))
	}
	if got.Children[0].Parent != got {
		t.Fatal("direct child parent should point to cloned root")
	}
}

func TestFilterTreesWithRecentRelations(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	recentRoot := &TreeNode{Tag: makeTag(1, "recent-root"), Depth: 1}
	recentChild := &TreeNode{Tag: makeTag(2, "recent-child"), Depth: 2, Parent: recentRoot}
	oldRoot := &TreeNode{Tag: makeTag(3, "old-root"), Depth: 1}
	oldChild := &TreeNode{Tag: makeTag(4, "old-child"), Depth: 2, Parent: oldRoot}
	recentRoot.Children = []*TreeNode{recentChild}
	oldRoot.Children = []*TreeNode{oldChild}
	if err := db.Create(&models.TopicTagRelation{ParentID: recentRoot.Tag.ID, ChildID: recentChild.Tag.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create recent relation: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: oldRoot.Tag.ID, ChildID: oldChild.Tag.ID, RelationType: "abstract", CreatedAt: time.Now().AddDate(0, 0, -30)}).Error; err != nil {
		t.Fatalf("create old relation: %v", err)
	}

	got := filterTreesWithRecentRelations([]*TreeNode{recentRoot, oldRoot}, 14)

	if len(got) != 1 || got[0].Tag.ID != recentRoot.Tag.ID {
		t.Fatalf("filtered roots = %+v, want only recent root", got)
	}
}

func TestReviewHierarchyTreesAppliesLLMMove(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	oldParent := models.TopicTag{Label: "旧父", Slug: "old-parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	newParent := models.TopicTag{Label: "新父", Slug: "new-parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子", Slug: "child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&oldParent).Error; err != nil {
		t.Fatalf("create old parent: %v", err)
	}
	if err := db.Create(&newParent).Error; err != nil {
		t.Fatalf("create new parent: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: oldParent.ID, ChildID: newParent.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create sibling relation: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: oldParent.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create child relation: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{Moves: []treeReviewMove{{TagID: child.ID, ToParent: newParent.ID, Reason: "test"}}, Merges: nil}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14, nil)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}

	if result.TreesReviewed != 1 || result.MovesApplied != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertAbstractRelationMissing(t, db, oldParent.ID, child.ID)
	assertAbstractRelationExists(t, db, newParent.ID, child.ID)
}

func TestReviewOneTreeRejectsReviewRootDetach(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	parent := models.TopicTag{Label: "父", Slug: "parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	reviewRoot := models.TopicTag{Label: "审查根", Slug: "review-root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子", Slug: "child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&parent, &reviewRoot, &child} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag %s: %v", tag.Label, err)
		}
	}
	for _, relation := range []models.TopicTagRelation{
		{ParentID: parent.ID, ChildID: reviewRoot.ID, RelationType: "abstract", CreatedAt: time.Now()},
		{ParentID: reviewRoot.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Moves: []treeReviewMove{{TagID: reviewRoot.ID, ToParent: 0, Reason: "should be rejected"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	tree := &TreeNode{Tag: &reviewRoot, Depth: 2}
	tree.Children = []*TreeNode{{Tag: &child, Depth: 3, Parent: tree}}
	result := &TreeReviewResult{}
	reviewOneTree(tree, "event", result)

	if result.MovesApplied != 0 {
		t.Fatalf("review root detach should be rejected, but move was applied")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error for review root detach attempt")
	}
	assertAbstractRelationExists(t, db, parent.ID, reviewRoot.ID)
}

func TestValidateAndCreateReviewAbstract_RejectsExistingDescendantReuseCycle(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	ancestor := models.TopicTag{Label: "祖先", Slug: "ancestor", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	descendant := models.TopicTag{Label: "后代", Slug: "descendant", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	peer := models.TopicTag{Label: "同级", Slug: "peer", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&ancestor).Error; err != nil {
		t.Fatalf("create ancestor: %v", err)
	}
	if err := db.Create(&descendant).Error; err != nil {
		t.Fatalf("create descendant: %v", err)
	}
	if err := db.Create(&peer).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: ancestor.ID, ChildID: descendant.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create existing relation: %v", err)
	}

	originalFinder := findSimilarExistingAbstractFn
	findSimilarExistingAbstractFn = func(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
		return &descendant
	}
	t.Cleanup(func() { findSimilarExistingAbstractFn = originalFinder })
	tagMap := map[uint]*TreeNode{
		ancestor.ID:   {Tag: &ancestor, Depth: 1},
		descendant.ID: {Tag: &descendant, Depth: 2},
		peer.ID:       {Tag: &peer, Depth: 2},
	}

	created, err := validateAndCreateReviewAbstract(treeReviewAbstract{Name: "后代", Description: "", ChildrenIDs: []uint{ancestor.ID, peer.ID}}, tagMap, "event")
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if created {
		t.Fatal("cycle rejection should not report created")
	}
	assertAbstractRelationMissing(t, db, descendant.ID, ancestor.ID)
}

func TestValidateAndCreateReviewAbstract_SlugReuseCountsReused(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	existing := models.TopicTag{Label: "已有分组", Slug: "existing group", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child1 := models.TopicTag{Label: "子一", Slug: "child-one", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child2 := models.TopicTag{Label: "子二", Slug: "child-two", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing: %v", err)
	}
	if err := db.Create(&child1).Error; err != nil {
		t.Fatalf("create child1: %v", err)
	}
	if err := db.Create(&child2).Error; err != nil {
		t.Fatalf("create child2: %v", err)
	}
	originalFinder := findSimilarExistingAbstractFn
	findSimilarExistingAbstractFn = func(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
		return nil
	}
	t.Cleanup(func() { findSimilarExistingAbstractFn = originalFinder })
	tagMap := map[uint]*TreeNode{
		child1.ID: {Tag: &child1, Depth: 1},
		child2.ID: {Tag: &child2, Depth: 1},
	}

	created, err := validateAndCreateReviewAbstract(treeReviewAbstract{Name: "existing group", Description: "", ChildrenIDs: []uint{child1.ID, child2.ID}}, tagMap, "event")
	if err != nil {
		t.Fatalf("validateAndCreateReviewAbstract: %v", err)
	}
	if created {
		t.Fatal("slug-matched existing abstract should count as reused")
	}
	assertAbstractRelationExists(t, db, existing.ID, child1.ID)
	assertAbstractRelationExists(t, db, existing.ID, child2.ID)
}

func TestSplitReviewTreesIncludesNestedLargeRootLevel(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	largeChild := &TreeNode{Tag: makeTag(2, "large-child"), Depth: 2, Parent: root}
	root.Children = []*TreeNode{largeChild}
	for i := 0; i < 51; i++ {
		largeChild.Children = append(largeChild.Children, &TreeNode{Tag: makeTag(uint(100+i), "leaf"), Depth: 3, Parent: largeChild})
	}

	parts := splitReviewTrees(root, 50)

	foundLargeChildRootLevel := false
	for _, part := range parts {
		if part.Tag.ID == largeChild.Tag.ID && countNodes(part) == len(largeChild.Children)+1 {
			foundLargeChildRootLevel = true
			break
		}
	}
	if !foundLargeChildRootLevel {
		t.Fatalf("expected nested large child root-level review in %d parts", len(parts))
	}
}

func TestCreateAbstractTagDirectly_SkipsDepthOverflowChild(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	deep := models.TopicTag{Label: "deep", Slug: "deep", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	mid := models.TopicTag{Label: "mid", Slug: "mid", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	leaf := models.TopicTag{Label: "leaf", Slug: "leaf", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	candidate := models.TopicTag{Label: "candidate", Slug: "candidate", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&deep).Error; err != nil {
		t.Fatalf("create deep: %v", err)
	}
	if err := db.Create(&mid).Error; err != nil {
		t.Fatalf("create mid: %v", err)
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	if err := db.Create(&candidate).Error; err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: deep.ID, ChildID: mid.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create deep->mid: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: mid.ID, ChildID: leaf.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create mid->leaf: %v", err)
	}

	originalFinder := findSimilarExistingAbstractFn
	findSimilarExistingAbstractFn = func(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
		return &deep
	}
	t.Cleanup(func() { findSimilarExistingAbstractFn = originalFinder })
	tagMap := map[uint]*TreeNode{
		leaf.ID:      {Tag: &leaf, Depth: 4},
		candidate.ID: {Tag: &candidate, Depth: 1},
	}

	err := createAbstractTagDirectly(treeCleanupAbstract{
		Name:        "deep",
		Description: "",
		ChildrenIDs: []uint{leaf.ID, candidate.ID},
		Reason:      "test",
	}, tagMap, "event")
	if err != nil {
		t.Fatalf("createAbstractTagDirectly: %v", err)
	}
	assertAbstractRelationExists(t, db, deep.ID, candidate.ID)
	assertAbstractRelationMissing(t, db, deep.ID, leaf.ID)
}

func makeTagWithStatus(id uint, label string, status string) *models.TopicTag {
	return &models.TopicTag{ID: id, Label: label, Status: status, Category: "event"}
}

func TestCalculateTreeDepth_SingleNode(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	if d := calculateTreeDepth(root); d != 1 {
		t.Errorf("expected 1, got %d", d)
	}
}

func TestCalculateTreeDepth_ThreeLevels(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	child := &TreeNode{Tag: makeTag(2, "child"), Depth: 2, Parent: root}
	grandchild := &TreeNode{Tag: makeTag(3, "grandchild"), Depth: 3, Parent: child}
	root.Children = []*TreeNode{child}
	child.Children = []*TreeNode{grandchild}

	if d := calculateTreeDepth(root); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
}

func TestCalculateTreeDepth_FiveLevels(t *testing.T) {
	n1 := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	n2 := &TreeNode{Tag: makeTag(2, "b"), Depth: 2, Parent: n1}
	n3 := &TreeNode{Tag: makeTag(3, "c"), Depth: 3, Parent: n2}
	n4 := &TreeNode{Tag: makeTag(4, "d"), Depth: 4, Parent: n3}
	n5 := &TreeNode{Tag: makeTag(5, "e"), Depth: 5, Parent: n4}
	n1.Children = []*TreeNode{n2}
	n2.Children = []*TreeNode{n3}
	n3.Children = []*TreeNode{n4}
	n4.Children = []*TreeNode{n5}

	if d := calculateTreeDepth(n1); d != 5 {
		t.Errorf("expected 5, got %d", d)
	}
}

func TestCountNodes(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	c1 := &TreeNode{Tag: makeTag(2, "c1"), Depth: 2, Parent: root}
	c2 := &TreeNode{Tag: makeTag(3, "c2"), Depth: 2, Parent: root}
	root.Children = []*TreeNode{c1, c2}

	if n := countNodes(root); n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func TestCollectAllTags(t *testing.T) {
	root := &TreeNode{Tag: makeTag(1, "root"), Depth: 1}
	c1 := &TreeNode{Tag: makeTag(2, "c1"), Depth: 2, Parent: root}
	c2 := &TreeNode{Tag: makeTag(3, "c2"), Depth: 2, Parent: root}
	root.Children = []*TreeNode{c1, c2}

	tags := collectAllTags(root)
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}
}

func TestValidateTreeReviewMove_TagNotActive(t *testing.T) {
	node := &TreeNode{Tag: makeTagWithStatus(1, "a", "inactive"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for inactive tag")
	}
}

func TestValidateTreeReviewMove_TagNotInTree(t *testing.T) {
	tagMap := map[uint]*TreeNode{}
	move := treeReviewMove{TagID: 999, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for tag not in tree")
	}
}

func TestValidateTreeReviewMove_SelfParent(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 1}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for self-parent")
	}
}

func TestValidateTreeReviewMove_ValidDetach(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err != nil {
		t.Errorf("expected no error for valid detach, got: %v", err)
	}
}

func TestValidateTreeReviewMove_InvalidTarget(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 999}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for non-existent target")
	}
}

func TestValidateTreeReviewMove_RejectsCycle(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root := models.TopicTag{Label: "root", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "child", Slug: "child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create root relation: %v", err)
	}

	tagMap := map[uint]*TreeNode{
		root.ID:  {Tag: &root, Depth: 1},
		child.ID: {Tag: &child, Depth: 2},
	}
	err := validateTreeReviewMove(treeReviewMove{TagID: root.ID, ToParent: child.ID}, tagMap)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestValidateTreeReviewMove_RejectsDepthOverflow(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tags := []models.TopicTag{
		{Label: "l1", Slug: "l1", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "l2", Slug: "l2", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "l3", Slug: "l3", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "l4", Slug: "l4", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "l5", Slug: "l5", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
		{Label: "moving", Slug: "moving", Category: "event", Kind: "event", Source: "abstract", Status: "active"},
	}
	if err := db.Create(&tags).Error; err != nil {
		t.Fatalf("create tags: %v", err)
	}
	for i := 0; i < 4; i++ {
		if err := db.Create(&models.TopicTagRelation{ParentID: tags[i].ID, ChildID: tags[i+1].ID, RelationType: "abstract"}).Error; err != nil {
			t.Fatalf("create relation %d: %v", i, err)
		}
	}

	tagMap := map[uint]*TreeNode{
		tags[4].ID: {Tag: &tags[4], Depth: 5},
		tags[5].ID: {Tag: &tags[5], Depth: 1},
	}
	err := validateTreeReviewMove(treeReviewMove{TagID: tags[5].ID, ToParent: tags[4].ID}, tagMap)
	if err == nil {
		t.Fatal("expected depth overflow error")
	}
}

func TestValidateTreeReviewMerge_SourceNotInTree(t *testing.T) {
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{2: target}
	merge := treeReviewMerge{SourceID: 999, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for source not in tree")
	}
}

func TestValidateTreeReviewMerge_TargetNotInTree(t *testing.T) {
	source := &TreeNode{Tag: makeTag(1, "source"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source}
	merge := treeReviewMerge{SourceID: 1, TargetID: 999}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for target not in tree")
	}
}

func TestValidateTreeReviewMerge_SelfMerge(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	merge := treeReviewMerge{SourceID: 1, TargetID: 1}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for self-merge")
	}
}

func TestValidateTreeReviewMerge_SourceInactive(t *testing.T) {
	source := &TreeNode{Tag: makeTagWithStatus(1, "source", "inactive"), Depth: 1}
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source, 2: target}
	merge := treeReviewMerge{SourceID: 1, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for inactive source")
	}
}

func TestValidateTreeReviewMerge_ValidMerge(t *testing.T) {
	source := &TreeNode{Tag: makeTag(1, "source"), Depth: 1}
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source, 2: target}
	merge := treeReviewMerge{SourceID: 1, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err != nil {
		t.Errorf("expected no error for valid merge, got: %v", err)
	}
}

func TestValidateTreeReviewMerge_RejectsDepthOverflow(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root := models.TopicTag{Label: "root", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagA := models.TopicTag{Label: "A", Slug: "a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagB := models.TopicTag{Label: "B", Slug: "b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagC := models.TopicTag{Label: "C", Slug: "c", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagD := models.TopicTag{Label: "D", Slug: "d", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagE := models.TopicTag{Label: "E", Slug: "e", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&root, &tagA, &tagB, &tagC, &tagD, &tagE} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag %s: %v", tag.Label, err)
		}
	}
	for _, relation := range []models.TopicTagRelation{
		{ParentID: root.ID, ChildID: tagA.ID, RelationType: "abstract"},
		{ParentID: tagA.ID, ChildID: tagB.ID, RelationType: "abstract"},
		{ParentID: root.ID, ChildID: tagC.ID, RelationType: "abstract"},
		{ParentID: tagC.ID, ChildID: tagD.ID, RelationType: "abstract"},
		{ParentID: tagD.ID, ChildID: tagE.ID, RelationType: "abstract"},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	tagMap := map[uint]*TreeNode{
		root.ID: {Tag: &root, Depth: 1},
		tagA.ID: {Tag: &tagA, Depth: 2},
		tagB.ID: {Tag: &tagB, Depth: 3},
		tagC.ID: {Tag: &tagC, Depth: 2},
		tagD.ID: {Tag: &tagD, Depth: 3},
		tagE.ID: {Tag: &tagE, Depth: 4},
	}

	err := validateTreeReviewMerge(treeReviewMerge{SourceID: tagC.ID, TargetID: tagB.ID}, tagMap)
	if err == nil {
		t.Fatal("expected depth overflow error")
	}
}

func TestValidateTreeReviewMerge_RejectsTargetSubtreeDepthOverflowUnderSourceParent(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root := models.TopicTag{Label: "root", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagA := models.TopicTag{Label: "A", Slug: "a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	parent := models.TopicTag{Label: "parent", Slug: "parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	source := models.TopicTag{Label: "source", Slug: "source", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	target := models.TopicTag{Label: "target", Slug: "target", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	targetChild := models.TopicTag{Label: "target child", Slug: "target-child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	targetGrandchild := models.TopicTag{Label: "target grandchild", Slug: "target-grandchild", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&root, &tagA, &parent, &source, &target, &targetChild, &targetGrandchild} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag %s: %v", tag.Label, err)
		}
	}
	for _, relation := range []models.TopicTagRelation{
		{ParentID: root.ID, ChildID: tagA.ID, RelationType: "abstract"},
		{ParentID: tagA.ID, ChildID: parent.ID, RelationType: "abstract"},
		{ParentID: parent.ID, ChildID: source.ID, RelationType: "abstract"},
		{ParentID: root.ID, ChildID: target.ID, RelationType: "abstract"},
		{ParentID: target.ID, ChildID: targetChild.ID, RelationType: "abstract"},
		{ParentID: targetChild.ID, ChildID: targetGrandchild.ID, RelationType: "abstract"},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	tagMap := map[uint]*TreeNode{
		root.ID:             {Tag: &root, Depth: 1},
		tagA.ID:             {Tag: &tagA, Depth: 2},
		parent.ID:           {Tag: &parent, Depth: 3},
		source.ID:           {Tag: &source, Depth: 4},
		target.ID:           {Tag: &target, Depth: 2},
		targetChild.ID:      {Tag: &targetChild, Depth: 3},
		targetGrandchild.ID: {Tag: &targetGrandchild, Depth: 4},
	}

	err := validateTreeReviewMerge(treeReviewMerge{SourceID: source.ID, TargetID: target.ID}, tagMap)
	if err == nil {
		t.Fatal("expected target subtree depth overflow error")
	}
}

func TestValidateTreeReviewMerge_RejectsAncestorDescendantMerge(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	source := models.TopicTag{Label: "source", Slug: "source", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	target := models.TopicTag{Label: "target", Slug: "target", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: source.ID, ChildID: target.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	sourceNode := &TreeNode{Tag: &source, Depth: 1}
	targetNode := &TreeNode{Tag: &target, Depth: 2, Parent: sourceNode}
	sourceNode.Children = []*TreeNode{targetNode}
	tagMap := map[uint]*TreeNode{
		source.ID: sourceNode,
		target.ID: targetNode,
	}

	err := validateTreeReviewMerge(treeReviewMerge{SourceID: source.ID, TargetID: target.ID}, tagMap)
	if err == nil {
		t.Fatal("expected ancestor/descendant merge error")
	}
}

func TestExecuteTreeReviewMove_KeepsOldParentWhenNewLinkFails(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	oldParent := models.TopicTag{Label: "旧父", Slug: "old-parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子", Slug: "child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	nonAbstractParent := models.TopicTag{Label: "普通父", Slug: "normal-parent", Category: "event", Kind: "event", Source: "llm", Status: "active"}
	if err := db.Create(&oldParent).Error; err != nil {
		t.Fatalf("create old parent: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&nonAbstractParent).Error; err != nil {
		t.Fatalf("create non-abstract parent: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: oldParent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create old relation: %v", err)
	}

	err := executeTreeReviewMove(treeReviewMove{TagID: child.ID, ToParent: nonAbstractParent.ID, Reason: "test"})
	if err == nil {
		t.Fatal("expected link failure")
	}
	assertAbstractRelationExists(t, db, oldParent.ID, child.ID)
}

func TestExecuteTreeReviewMove_MovesChildAtomically(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	oldParent := models.TopicTag{Label: "旧父", Slug: "old-parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	newParent := models.TopicTag{Label: "新父", Slug: "new-parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子", Slug: "child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&oldParent).Error; err != nil {
		t.Fatalf("create old parent: %v", err)
	}
	if err := db.Create(&newParent).Error; err != nil {
		t.Fatalf("create new parent: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: oldParent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create old relation: %v", err)
	}

	if err := executeTreeReviewMove(treeReviewMove{TagID: child.ID, ToParent: newParent.ID, Reason: "test"}); err != nil {
		t.Fatalf("execute move: %v", err)
	}

	assertAbstractRelationMissing(t, db, oldParent.ID, child.ID)
	assertAbstractRelationExists(t, db, newParent.ID, child.ID)
}

func TestReviewHierarchyTreesAppliesLLMMerge(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	if err := db.AutoMigrate(&models.MergeReembeddingQueue{}); err != nil {
		t.Fatalf("migrate merge dependencies: %v", err)
	}
	root := models.TopicTag{Label: "根", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	dup1 := models.TopicTag{Label: "重复A", Slug: "dup-a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	dup2 := models.TopicTag{Label: "重复B", Slug: "dup-b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	other := models.TopicTag{Label: "其他", Slug: "other", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&root, &dup1, &dup2, &other} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag %s: %v", tag.Label, err)
		}
	}
	for _, relation := range []models.TopicTagRelation{
		{ParentID: root.ID, ChildID: dup1.ID, RelationType: "abstract", CreatedAt: time.Now()},
		{ParentID: root.ID, ChildID: dup2.ID, RelationType: "abstract", CreatedAt: time.Now()},
		{ParentID: root.ID, ChildID: other.ID, RelationType: "abstract", CreatedAt: time.Now()},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Merges: []treeReviewMerge{{SourceID: dup1.ID, TargetID: dup2.ID, Reason: "test merge"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14, nil)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.TreesReviewed != 1 || result.MergesApplied != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	var merged models.TopicTag
	if err := db.First(&merged, dup1.ID).Error; err != nil {
		t.Fatalf("load merged tag: %v", err)
	}
	if merged.Status != "merged" {
		t.Fatalf("source tag status = %q, want merged", merged.Status)
	}
	assertAbstractRelationMissing(t, db, root.ID, dup1.ID)
	assertAbstractRelationExists(t, db, root.ID, dup2.ID)
}

func TestReviewHierarchyTreesRejectsRootMergeSource(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根节点", Slug: "root-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子节点", Slug: "child-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Merges: []treeReviewMerge{{SourceID: root.ID, TargetID: child.ID, Reason: "should be rejected"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14, nil)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.MergesApplied != 0 {
		t.Fatalf("root merge source should be rejected, but merge was applied")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error for root merge source attempt")
	}
	var rootTag models.TopicTag
	if err := db.First(&rootTag, root.ID).Error; err != nil {
		t.Fatalf("load root: %v", err)
	}
	if rootTag.Status != "active" {
		t.Fatalf("root tag should remain active, got %q", rootTag.Status)
	}
}

func TestReviewHierarchyTreesRejectsRootDemotion(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根节点", Slug: "root-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子节点", Slug: "child-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Moves: []treeReviewMove{{TagID: root.ID, ToParent: child.ID, Reason: "should be rejected"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14, nil)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.MovesApplied != 0 {
		t.Fatalf("root demotion should be rejected, but move was applied")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error for root demotion attempt")
	}
	assertAbstractRelationExists(t, db, root.ID, child.ID)
}

func TestIsAbstractRoot(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根节点", Slug: "root-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子节点", Slug: "child-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	if !isAbstractRoot(db, root.ID) {
		t.Fatal("expected root tag to be abstract root")
	}
	if isAbstractRoot(db, child.ID) {
		t.Fatal("expected child tag not to be abstract root")
	}
}
