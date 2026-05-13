package tagging

import (
	"strings"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
)

func TestCollectTreeBridgePairs_GlobalDedup(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	_ = db

	tag1 := models.TopicTag{Label: "机器学习", Slug: "machine-learning", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tag2 := models.TopicTag{Label: "深度学习", Slug: "deep-learning", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tag3 := models.TopicTag{Label: "NLP", Slug: "nlp", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&tag1).Error; err != nil {
		t.Fatalf("create tag1: %v", err)
	}
	if err := db.Create(&tag2).Error; err != nil {
		t.Fatalf("create tag2: %v", err)
	}
	if err := db.Create(&tag3).Error; err != nil {
		t.Fatalf("create tag3: %v", err)
	}

	originalFinder := findTreeBridgeSimilarFn
	callCount := 0
	findTreeBridgeSimilarFn = func(rootID uint, category string, limit int) ([]TagCandidate, error) {
		callCount++
		switch rootID {
		case tag1.ID:
			return []TagCandidate{{Tag: &tag2, Similarity: 0.85}}, nil
		case tag2.ID:
			return []TagCandidate{{Tag: &tag1, Similarity: 0.85}}, nil
		case tag3.ID:
			return []TagCandidate{{Tag: &tag1, Similarity: 0.90}}, nil
		default:
			return nil, nil
		}
	}
	t.Cleanup(func() { findTreeBridgeSimilarFn = originalFinder })

	pairs, err := collectTreeBridgePairs("event")
	if err != nil {
		t.Fatalf("collectTreeBridgePairs: %v", err)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 calls to FindSimilarAbstractTags, got %d", callCount)
	}

	foundPairA := false
	foundPairC := false
	for _, p := range pairs {
		if (p.TagA == tag1.ID && p.TagB == tag2.ID) || (p.TagA == tag2.ID && p.TagB == tag1.ID) {
			foundPairA = true
		}
		if (p.TagA == tag1.ID && p.TagB == tag3.ID) || (p.TagA == tag3.ID && p.TagB == tag1.ID) {
			foundPairC = true
		}
	}
	if !foundPairA {
		t.Error("expected tag1-tag2 pair (dedup across A→B and B→A)")
	}
	if !foundPairC {
		t.Error("expected tag1-tag3 pair")
	}

	countPairA := 0
	for _, p := range pairs {
		if (p.TagA == tag1.ID && p.TagB == tag2.ID) || (p.TagA == tag2.ID && p.TagB == tag1.ID) {
			countPairA++
		}
	}
	if countPairA != 1 {
		t.Errorf("tag1-tag2 pair should appear exactly once (dedup), got %d", countPairA)
	}
}

func TestBuildTreeBridgePrompt_Format(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root1 := models.TopicTag{Label: "机器学习", Slug: "ml", Category: "event", Kind: "event", Source: "abstract", Status: "active", Description: "机器学习相关话题"}
	root2 := models.TopicTag{Label: "深度学习", Slug: "dl", Category: "event", Kind: "event", Source: "abstract", Status: "active", Description: "深度学习相关话题"}
	if err := db.Create(&root1).Error; err != nil {
		t.Fatalf("create root1: %v", err)
	}
	if err := db.Create(&root2).Error; err != nil {
		t.Fatalf("create root2: %v", err)
	}

	child1 := models.TopicTag{Label: "监督学习", Slug: "supervised", Category: "event", Kind: "event", Source: "abstract", Status: "active", CreatedAt: time.Now()}
	child2 := models.TopicTag{Label: "无监督学习", Slug: "unsupervised", Category: "event", Kind: "event", Source: "abstract", Status: "active", CreatedAt: time.Now().Add(time.Second)}
	child3 := models.TopicTag{Label: "CNN", Slug: "cnn", Category: "event", Kind: "event", Source: "abstract", Status: "active", CreatedAt: time.Now()}
	if err := db.Create(&child1).Error; err != nil {
		t.Fatalf("create child1: %v", err)
	}
	if err := db.Create(&child2).Error; err != nil {
		t.Fatalf("create child2: %v", err)
	}
	if err := db.Create(&child3).Error; err != nil {
		t.Fatalf("create child3: %v", err)
	}

	for _, rel := range []models.TopicTagRelation{
		{ParentID: root1.ID, ChildID: child1.ID, RelationType: "abstract"},
		{ParentID: root1.ID, ChildID: child2.ID, RelationType: "abstract"},
		{ParentID: root2.ID, ChildID: child3.ID, RelationType: "abstract"},
	} {
		if err := db.Create(&rel).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	var article models.Article
	db.Create(&article)
	db.Create(&models.ArticleTopicTag{TopicTagID: root1.ID, ArticleID: article.ID})
	db.Create(&models.ArticleTopicTag{TopicTagID: root2.ID, ArticleID: article.ID})

	pairs := []treeBridgePair{
		{TagA: root1.ID, TagB: root2.ID, Similarity: 0.86},
	}

	prompt := buildTreeBridgePrompt(pairs)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}

	for _, want := range []string{
		"1. 树根 A:",
		"机器学习",
		"深度学习",
		"监督学习",
		"无监督学习",
		"CNN",
		"0.86",
		"merge",
		"parent_A",
		"parent_B",
		"skip",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestExecuteTreeBridgePairs_MergeThenSkipParent(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	if err := db.AutoMigrate(&models.MergeReembeddingQueue{}); err != nil {
		t.Fatalf("migrate merge dependencies: %v", err)
	}

	tagA := models.TopicTag{Label: "标签A", Slug: "tag-a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagB := models.TopicTag{Label: "标签B", Slug: "tag-b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&tagA).Error; err != nil {
		t.Fatalf("create tagA: %v", err)
	}
	if err := db.Create(&tagB).Error; err != nil {
		t.Fatalf("create tagB: %v", err)
	}

	pairs := []treeBridgePair{
		{TagA: tagA.ID, TagB: tagB.ID, Similarity: 0.90},
	}

	judgment := &treeBridgeJudgment{
		Pairs: []treeBridgeJudgmentPair{
			{Index: 1, Action: "merge", Reason: "same concept"},
			{Index: 1, Action: "parent_A", Reason: "B is narrower than A"},
		},
	}

	merges, links, errors, err := executeTreeBridgePairs(pairs, judgment, "event")
	if err != nil {
		t.Fatalf("executeTreeBridgePairs: %v", err)
	}

	if merges != 1 {
		t.Errorf("expected 1 merge, got %d", merges)
	}
	if links != 0 {
		t.Errorf("expected 0 links (tagA was merged, skipSet should prevent parent link), got %d", links)
	}
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}

	var merged models.TopicTag
	if err := db.First(&merged, tagA.ID).Error; err != nil {
		t.Fatalf("load merged tag: %v", err)
	}
	if merged.Status != "merged" {
		t.Errorf("tagA status = %q, want merged", merged.Status)
	}

	var kept models.TopicTag
	if err := db.First(&kept, tagB.ID).Error; err != nil {
		t.Fatalf("load kept tag: %v", err)
	}
	if kept.Status != "active" {
		t.Errorf("tagB status = %q, want active", kept.Status)
	}

	time.Sleep(50 * time.Millisecond)
}

func TestExecuteTreeBridgePairs_ParentLinks(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tagA := models.TopicTag{Label: "标签A", Slug: "tag-a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagB := models.TopicTag{Label: "标签B", Slug: "tag-b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&tagA).Error; err != nil {
		t.Fatalf("create tagA: %v", err)
	}
	if err := db.Create(&tagB).Error; err != nil {
		t.Fatalf("create tagB: %v", err)
	}

	pairs := []treeBridgePair{
		{TagA: tagA.ID, TagB: tagB.ID, Similarity: 0.85},
	}

	judgment := &treeBridgeJudgment{
		Pairs: []treeBridgeJudgmentPair{
			{Index: 1, Action: "parent_A", Reason: "B is narrower than A"},
		},
	}

	merges, links, errors, err := executeTreeBridgePairs(pairs, judgment, "event")
	if err != nil {
		t.Fatalf("executeTreeBridgePairs: %v", err)
	}
	if merges != 0 {
		t.Errorf("expected 0 merges, got %d", merges)
	}
	if links != 1 {
		t.Errorf("expected 1 link, got %d", links)
	}
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}

	assertAbstractRelationExists(t, db, tagA.ID, tagB.ID)

	time.Sleep(50 * time.Millisecond)
}

func TestDetermineTreeBridgeMergeDirection(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tagA := models.TopicTag{Label: "多子标签", Slug: "many-kids", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagB := models.TopicTag{Label: "少子标签", Slug: "few-kids", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&tagA).Error; err != nil {
		t.Fatalf("create tagA: %v", err)
	}
	if err := db.Create(&tagB).Error; err != nil {
		t.Fatalf("create tagB: %v", err)
	}

	childForA := models.TopicTag{Label: "子1", Slug: "child-a", Category: "event", Source: "abstract", Status: "active"}
	childForA2 := models.TopicTag{Label: "子2", Slug: "child-a2", Category: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&childForA).Error; err != nil {
		t.Fatalf("create childA: %v", err)
	}
	if err := db.Create(&childForA2).Error; err != nil {
		t.Fatalf("create childA2: %v", err)
	}

	childForB := models.TopicTag{Label: "子B", Slug: "child-b", Category: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&childForB).Error; err != nil {
		t.Fatalf("create childB: %v", err)
	}

	for _, rel := range []models.TopicTagRelation{
		{ParentID: tagA.ID, ChildID: childForA.ID, RelationType: "abstract"},
		{ParentID: tagA.ID, ChildID: childForA2.ID, RelationType: "abstract"},
		{ParentID: tagB.ID, ChildID: childForB.ID, RelationType: "abstract"},
	} {
		if err := db.Create(&rel).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	source, target := determineTreeBridgeMergeDirection(treeBridgePair{TagA: tagA.ID, TagB: tagB.ID})
	if source != tagB.ID || target != tagA.ID {
		t.Errorf("expected source=%d (fewer children) target=%d, got source=%d target=%d", tagB.ID, tagA.ID, source, target)
	}
}

func TestExecuteTreeBridge_BudgetAware(t *testing.T) {
	_ = setupAbstractTagServiceTestDB(t)

	originalFinder := findTreeBridgeSimilarFn
	findTreeBridgeSimilarFn = func(rootID uint, category string, limit int) ([]TagCandidate, error) {
		return nil, nil
	}
	t.Cleanup(func() { findTreeBridgeSimilarFn = originalFinder })

	result, err := ExecuteTreeBridge("event", nil)
	if err != nil {
		t.Fatalf("ExecuteTreeBridge with nil budget: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
