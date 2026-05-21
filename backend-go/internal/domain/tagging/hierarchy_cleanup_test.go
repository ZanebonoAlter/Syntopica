package tagging

import (
	"testing"

	"my-robot-backend/internal/domain/models"
)

func makeTag(id uint, label string) *models.TopicTag {
	return &models.TopicTag{ID: id, Label: label, Status: "active", Category: "event"}
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
