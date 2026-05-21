package tagging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"

	"my-robot-backend/internal/platform/database"
)

func TestPlaceTagInHierarchy_NoTemplate(t *testing.T) {
	tag := &models.TopicTag{ID: 999, Category: "nonexistent", Label: "test"}
	result, err := PlaceTagInHierarchy(context.Background(), tag)
	if err == nil {
		t.Fatal("expected error for category with no template")
	}
	if result != nil {
		t.Fatal("expected nil result on error")
	}
}

func TestPlaceTagAtLevelNoParentPathCallsNodeCreationDecision(t *testing.T) {
	content, err := os.ReadFile("hierarchy_placement.go")
	if err != nil {
		t.Fatalf("read hierarchy_placement.go: %v", err)
	}

	body := extractSourceBetween(t, string(content), "func placeTagAtLevel", "// Anchor represents")
	if !strings.Contains(body, "decideNodeCreation(") {
		t.Fatal("expected no-parent path to call node creation decision")
	}
	if strings.Contains(body, `result.Action = "unplaced"`) {
		t.Fatal("expected no-parent path not to silently leave tag unplaced")
	}
}

func TestPlaceTagPlacementResultIncludesBlockerMetadata(t *testing.T) {
	result := markPlacementBlocker(&PlacementResult{TagID: 1}, "unplaced", "insufficient_siblings", "wait_for_more_siblings")

	if result.BlockerReason != "insufficient_siblings" {
		t.Fatalf("expected blocker reason, got %q", result.BlockerReason)
	}
	if result.DiagnosticAction != "wait_for_more_siblings" {
		t.Fatalf("expected diagnostic action, got %q", result.DiagnosticAction)
	}
}

func TestPlaceTagValidateNodeCreationContextBlockers(t *testing.T) {
	tests := []struct {
		name           string
		creationCtx    nodeCreationContext
		targetDepth    int
		anchorCount    int
		candidateCount int
		wantReason     string
	}{
		{
			name:           "no anchor context",
			creationCtx:    nodeCreationContext{CandidateChildIDs: []uint{1, 2}},
			targetDepth:    1,
			anchorCount:    0,
			candidateCount: 0,
			wantReason:     "no_anchor_context",
		},
		{
			name:           "insufficient siblings",
			creationCtx:    nodeCreationContext{CandidateChildIDs: []uint{1}},
			targetDepth:    1,
			anchorCount:    1,
			candidateCount: 0,
			wantReason:     "insufficient_siblings",
		},
		{
			name:           "high article overlap",
			creationCtx:    nodeCreationContext{CandidateChildIDs: []uint{1, 2}, MaxArticleJaccard: 0.71},
			targetDepth:    1,
			anchorCount:    1,
			candidateCount: 0,
			wantReason:     "low_information_gain",
		},
		{
			name:           "low leaf to depth ratio",
			creationCtx:    nodeCreationContext{CandidateChildIDs: []uint{1, 2}, MaxArticleJaccard: 0.2},
			targetDepth:    2,
			anchorCount:    1,
			candidateCount: 0,
			wantReason:     "low_information_gain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, _ := validateNodeCreationContext(tt.creationCtx, tt.targetDepth, tt.anchorCount, tt.candidateCount)
			if reason != tt.wantReason {
				t.Fatalf("expected reason %q, got %q", tt.wantReason, reason)
			}
		})
	}
}

func TestPlaceTagDecideNodeCreationSuccessReturnsCreatedNodeMetadata(t *testing.T) {
	setupPlacementTestDB(t)

	conceptID := uint(42)
	trigger := models.TopicTag{Label: "Go", Category: "keyword", Status: "active", Source: "llm", ConceptID: &conceptID}
	anchorParent := models.TopicTag{Label: "Programming", Category: "keyword", Status: "active", Source: "abstract", ConceptID: &conceptID}
	siblingOne := models.TopicTag{Label: "Rust", Category: "keyword", Status: "active", Source: "llm", ConceptID: &conceptID}
	siblingTwo := models.TopicTag{Label: "Python", Category: "keyword", Status: "active", Source: "llm", ConceptID: &conceptID}
	if err := database.DB.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := database.DB.Create(&anchorParent).Error; err != nil {
		t.Fatalf("create anchor parent: %v", err)
	}
	if err := database.DB.Create(&siblingOne).Error; err != nil {
		t.Fatalf("create sibling one: %v", err)
	}
	if err := database.DB.Create(&siblingTwo).Error; err != nil {
		t.Fatalf("create sibling two: %v", err)
	}
	if err := database.DB.Create(&[]models.TopicTagRelation{
		{ParentID: anchorParent.ID, ChildID: siblingOne.ID, RelationType: "abstract"},
		{ParentID: anchorParent.ID, ChildID: siblingTwo.ID, RelationType: "abstract"},
	}).Error; err != nil {
		t.Fatalf("create sibling relations: %v", err)
	}

	originalCreate := createAbstractAtLevelFn
	createAbstractAtLevelFn = func(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel, conceptID uint) (uint, string, error) {
		return uint(777), "Language Group", nil
	}
	t.Cleanup(func() {
		createAbstractAtLevelFn = originalCreate
	})

	tmpl := &CategoryHierarchyTemplate{Category: "keyword", MaxLevel: 2, Levels: []AbstractionLevel{{Level: 1, Name: "主题领域"}, {Level: 2, Name: "具体概念", IsLeaf: true}}}
	result, err := decideNodeCreation(context.Background(), &trigger, tmpl, &tmpl.Levels[0], 1, conceptID, []Anchor{{ParentID: anchorParent.ID}}, nil, &PlacementResult{TagID: trigger.ID, ConceptID: &conceptID})
	if err != nil {
		t.Fatalf("decide node creation: %v", err)
	}

	if result.Action != "created_node" {
		t.Fatalf("expected created_node action, got %q", result.Action)
	}
	if result.ParentID == nil || *result.ParentID != 777 {
		t.Fatalf("expected parent id 777, got %v", result.ParentID)
	}
	if result.ParentLabel != "Language Group" {
		t.Fatalf("expected parent label, got %q", result.ParentLabel)
	}
	if len(result.CreatedParents) != 1 || result.CreatedParents[0] != 777 {
		t.Fatalf("expected created parent metadata, got %#v", result.CreatedParents)
	}
}

func TestSearchAnchorsUsesActiveAnchorSignals(t *testing.T) {
	setupPlacementTestDB(t)

	conceptID := uint(42)
	trigger := models.TopicTag{Label: "Go", Category: "keyword", Status: "active", Source: "llm", ConceptID: nil}
	parent := models.TopicTag{Label: "Programming", Category: "keyword", Status: "active", Source: "abstract", ConceptID: &conceptID}
	parentTwo := models.TopicTag{Label: "Systems", Category: "keyword", Status: "active", Source: "abstract", ConceptID: &conceptID}
	sibling := models.TopicTag{Label: "Rust", Category: "keyword", Status: "active", Source: "llm", ConceptID: &conceptID}
	siblingTwo := models.TopicTag{Label: "Zig", Category: "keyword", Status: "active", Source: "llm", ConceptID: &conceptID}
	if err := database.DB.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := database.DB.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := database.DB.Create(&parentTwo).Error; err != nil {
		t.Fatalf("create second parent: %v", err)
	}
	if err := database.DB.Create(&sibling).Error; err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	if err := database.DB.Create(&siblingTwo).Error; err != nil {
		t.Fatalf("create second sibling: %v", err)
	}
	if err := database.DB.Create(&models.TopicTagRelation{ParentID: parent.ID, ChildID: sibling.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create sibling relation: %v", err)
	}
	if err := database.DB.Create(&models.TopicTagRelation{ParentID: parentTwo.ID, ChildID: siblingTwo.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create second sibling relation: %v", err)
	}
	if err := database.DB.Create(&models.HierarchyAnchorSignal{
		Category:     "keyword",
		CenterTagID:  trigger.ID,
		MemberTagIDs: []uint{trigger.ID, sibling.ID, siblingTwo.ID},
		ExpiresAt:    time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create anchor signal: %v", err)
	}

	anchors, err := searchAnchors(context.Background(), &trigger, conceptID, 1)
	if err != nil {
		t.Fatalf("search anchors: %v", err)
	}
	if len(anchors) == 0 {
		t.Fatal("expected anchor signal anchor")
	}
	if anchors[0].Source != "anchor_signal" {
		t.Fatalf("expected anchor_signal source, got %q", anchors[0].Source)
	}
	if anchors[0].ParentID != parent.ID {
		t.Fatalf("expected parent %d, got %d", parent.ID, anchors[0].ParentID)
	}
}

func TestAggregateToUpperLevelDoesNotCreateAbstractNode(t *testing.T) {
	content, err := os.ReadFile("hierarchy_aggregation.go")
	if err != nil {
		t.Fatalf("read hierarchy_aggregation.go: %v", err)
	}

	source := string(content)
	if strings.Contains(source, "createAbstractAtLevel") {
		t.Fatal("expected aggregateToUpperLevel not to create abstract nodes")
	}
	if !strings.Contains(source, "PlaceTagInHierarchy owns node creation") {
		t.Fatal("expected structured skip log")
	}
}

func extractSourceBetween(t *testing.T, source, startMarker, endMarker string) string {
	t.Helper()

	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("missing marker %q", startMarker)
	}
	end := strings.Index(source[start:], endMarker)
	if end < 0 {
		t.Fatalf("missing marker %q", endMarker)
	}
	return source[start : start+end]
}

func setupPlacementTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})

	if err := db.AutoMigrate(
		&models.TopicTag{},
		&models.TopicTagRelation{},
		&models.TopicTagEmbedding{},
		&models.ArticleTopicTag{},
		&models.EmbeddingConfig{},
		&models.HierarchyConfig{},
		&models.HierarchyPendingChange{},
		&models.HierarchyConfigVersion{},
		&models.HierarchyAnchorSignal{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}
