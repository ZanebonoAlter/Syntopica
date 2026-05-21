package tagging

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"my-robot-backend/internal/domain/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/platform/database"
)

func TestHierarchyTemplateManager_LoadSystemDefaults(t *testing.T) {
	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	mgr.LoadSystemDefaults()

	if len(mgr.templates) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(mgr.templates))
	}

	expectedKeys := []string{"event", "person", "keyword"}
	for _, key := range expectedKeys {
		if _, ok := mgr.templates[key]; !ok {
			t.Errorf("missing template key: %s", key)
		}
	}

	if mgr.GetVersion() != 0 {
		t.Errorf("expected version 0 before DB load, got %d", mgr.GetVersion())
	}
}

func TestHierarchyTemplateManager_LoadFromDB(t *testing.T) {
	db := setupConfigTestDB(t)

	tmplJSON, err := json.Marshal(CategoryHierarchyTemplate{
		Category: "event",
		SubType:  "",
		MaxLevel: 2,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件大类", Description: "大分类", IsLeaf: false},
			{Level: 2, Name: "具体事件", Description: "具体实例", IsLeaf: true},
		},
	})
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}

	config := models.HierarchyConfig{
		Templates: models.HierarchyTemplatesJSON{"event": tmplJSON},
		Version:   1,
	}
	db.Create(&config)

	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	err = mgr.LoadFromDB()
	if err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	if mgr.GetVersion() != 1 {
		t.Errorf("expected version 1, got %d", mgr.GetVersion())
	}

	tmpl := mgr.GetTemplate("event", "")
	if tmpl == nil {
		t.Fatal("event template not loaded from DB")
	}
	if tmpl.MaxLevel != 2 {
		t.Errorf("expected MaxLevel=2 from DB, got %d", tmpl.MaxLevel)
	}
	if tmpl.Levels[0].Name != "事件大类" {
		t.Errorf("expected L1 name '事件大类', got %q", tmpl.Levels[0].Name)
	}
}

func TestHierarchyTemplateManager_LoadFromDBDefaultFallback(t *testing.T) {
	_ = setupConfigTestDB(t)

	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	err := mgr.LoadFromDB()
	if err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}

	if len(mgr.templates) != 3 {
		t.Fatalf("expected 3 default templates when DB has no config, got %d", len(mgr.templates))
	}
}

func TestHierarchyTemplateManager_GetTemplate(t *testing.T) {
	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	mgr.LoadSystemDefaults()

	if tmpl := mgr.GetTemplate("keyword", ""); tmpl == nil {
		t.Fatal("keyword template not found")
	}

	if tmpl := mgr.GetTemplate("nonexistent", ""); tmpl != nil {
		t.Fatal("nonexistent category should return nil")
	}

	if tmpl := mgr.GetTemplate("nonexistent", ""); tmpl != nil {
		t.Fatal("nonexistent category should return nil")
	}
}

func TestHierarchyTemplateManager_SaveConfig(t *testing.T) {
	db := setupConfigTestDB(t)
	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	mgr.LoadSystemDefaults()

	tmplJSON, err := json.Marshal(CategoryHierarchyTemplate{
		Category: "event",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件类型", Description: "大类", IsLeaf: false},
			{Level: 2, Name: "事件主体", Description: "核心实体", IsLeaf: false},
			{Level: 3, Name: "具体事件", Description: "实例", IsLeaf: true},
		},
	})
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}

	err = mgr.SaveConfig(map[string]json.RawMessage{"event": tmplJSON}, "test save")
	if err != nil {
		t.Fatalf("SaveConfig v1: %v", err)
	}

	if mgr.GetVersion() != 1 {
		t.Errorf("expected version 1 after first save, got %d", mgr.GetVersion())
	}

	err = mgr.SaveConfig(map[string]json.RawMessage{"event": tmplJSON}, "second save")
	if err != nil {
		t.Fatalf("SaveConfig v2: %v", err)
	}

	if mgr.GetVersion() != 2 {
		t.Errorf("expected version 2 after second save, got %d", mgr.GetVersion())
	}

	var configs []models.HierarchyConfig
	db.Order("version ASC").Find(&configs)
	if len(configs) != 2 {
		t.Fatalf("expected 2 config records, got %d", len(configs))
	}
	if configs[0].Version != 1 {
		t.Errorf("config 0 version = %d, want 1", configs[0].Version)
	}
	if configs[1].Version != 2 {
		t.Errorf("config 1 version = %d, want 2", configs[1].Version)
	}

	var vRecords []models.HierarchyConfigVersion
	db.Order("id ASC").Find(&vRecords)
	if len(vRecords) != 2 {
		t.Fatalf("expected 2 version records (one per save), got %d", len(vRecords))
	}
	if vRecords[0].ChangeLog != "test save" {
		t.Errorf("expected first record change_log='test save', got %q", vRecords[0].ChangeLog)
	}
	if vRecords[1].ChangeLog != "second save" {
		t.Errorf("expected second record change_log='second save', got %q", vRecords[1].ChangeLog)
	}
}

func TestPreviewConfigImpact(t *testing.T) {
	db := setupConfigTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	depth1 := models.TopicTag{Label: "L1", Slug: "l1", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	depth2 := models.TopicTag{Label: "L2", Slug: "l2", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	depth3 := models.TopicTag{Label: "L3", Slug: "l3", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	depth4 := models.TopicTag{Label: "L4", Slug: "l4", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	db.Create(&depth1)
	db.Create(&depth2)
	db.Create(&depth3)
	db.Create(&depth4)
	db.Create(&models.TopicTagRelation{ParentID: depth1.ID, ChildID: depth2.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: depth2.ID, ChildID: depth3.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: depth3.ID, ChildID: depth4.ID, RelationType: "abstract"})

	newTmpl := []CategoryHierarchyTemplate{{
		Category: "event",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件类型", IsLeaf: false},
			{Level: 2, Name: "事件主体", IsLeaf: false},
			{Level: 3, Name: "具体事件", IsLeaf: true},
		},
	}}

	impact, err := previewConfigImpact(&newTmpl)
	if err != nil {
		t.Fatalf("previewConfigImpact: %v", err)
	}
	if impact.TotalTags == 0 {
		t.Error("expected some tags in impact")
	}
	if impact.DepthExceeded != 1 {
		t.Errorf("expected 1 depth exceeded (tag at depth 3+1=4 > 3), got %d", impact.DepthExceeded)
	}
}

func TestHierarchyConfigHandlerSeparatesPreviewAndApply(t *testing.T) {
	content, err := os.ReadFile("hierarchy_handler.go")
	if err != nil {
		t.Fatalf("read hierarchy_handler.go: %v", err)
	}
	source := string(content)

	body := extractSourceBetween(t, source, "func UpdateHierarchyConfig", "func PreviewHierarchyConfig")
	if strings.Contains(body, "TriggerTemplateRebuild") || strings.Contains(body, "SaveConfig") {
		t.Fatal("UpdateHierarchyConfig should route preview/apply instead of directly saving or rebuilding")
	}
	for _, marker := range []string{
		"PreviewHierarchyConfig",
		"previewConfigImpact",
		"applyHierarchyConfig",
		"preview_only",
		"RegisterHierarchyRoutes",
		`POST("/config/preview"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("expected hierarchy config handler source to contain %s", marker)
		}
	}

	applyBody := extractSourceBetween(t, source, "func applyHierarchyConfig", "func GetHierarchyPending")
	for _, marker := range []string{"changedTemplateCategories", "hasActiveRebuildJob", "SaveConfig", "TriggerTemplateRebuild", "ExecuteJob"} {
		if !strings.Contains(applyBody, marker) {
			t.Fatalf("expected applyHierarchyConfig to contain %s", marker)
		}
	}
}

func TestConfigImpactIncludesRebuildEstimateAndViolationSummary(t *testing.T) {
	content, err := os.ReadFile("hierarchy_config.go")
	if err != nil {
		t.Fatalf("read hierarchy_config.go: %v", err)
	}
	source := string(content)
	for _, marker := range []string{"AffectedTagCount", "EstimatedRebuildDurationSeconds", "ViolationSummary"} {
		if !strings.Contains(source, marker) {
			t.Fatalf("expected ConfigImpact to contain %s", marker)
		}
	}
}

func TestPreviewConfigImpact_CrossCategory(t *testing.T) {
	db := setupConfigTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	eventTag := models.TopicTag{Label: "event-child", Slug: "ev-c", Category: "event", Status: "active"}
	kwTag := models.TopicTag{Label: "kw-parent", Slug: "kw-p", Category: "keyword", Source: "abstract", Status: "active"}
	db.Create(&eventTag)
	db.Create(&kwTag)
	db.Create(&models.TopicTagRelation{ParentID: kwTag.ID, ChildID: eventTag.ID, RelationType: "abstract"})

	newTmpl := []CategoryHierarchyTemplate{{
		Category: "keyword",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "领域", IsLeaf: false},
			{Level: 2, Name: "子域", IsLeaf: false},
			{Level: 3, Name: "概念", IsLeaf: true},
		},
	}, {
		Category: "event",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件类型", IsLeaf: false},
			{Level: 2, Name: "事件主体", IsLeaf: false},
			{Level: 3, Name: "具体事件", IsLeaf: true},
		},
	}}

	impact, err := previewConfigImpact(&newTmpl)
	if err != nil {
		t.Fatalf("previewConfigImpact: %v", err)
	}
	if impact.CrossCategory != 1 {
		t.Errorf("expected 1 cross-category violation, got %d", impact.CrossCategory)
	}

	found := false
	for _, d := range impact.Details {
		if d.Issue == "cross_category" && d.TagID == eventTag.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cross_category detail for event tag")
	}
}

func TestPreviewConfigImpact_LeafWithChildren(t *testing.T) {
	db := setupConfigTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	l1Tag := models.TopicTag{Label: "event-type", Slug: "et", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	l2Tag := models.TopicTag{Label: "event-body", Slug: "eb", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	l3Tag := models.TopicTag{Label: "leaf-with-kids", Slug: "lwk", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	childTag := models.TopicTag{Label: "child-of-leaf", Slug: "col", Category: "event", Status: "active"}
	db.Create(&l1Tag)
	db.Create(&l2Tag)
	db.Create(&l3Tag)
	db.Create(&childTag)
	db.Create(&models.TopicTagRelation{ParentID: l1Tag.ID, ChildID: l2Tag.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: l2Tag.ID, ChildID: l3Tag.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: l3Tag.ID, ChildID: childTag.ID, RelationType: "abstract"})

	newTmpl := []CategoryHierarchyTemplate{{
		Category: "event",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件类型", IsLeaf: false},
			{Level: 2, Name: "事件主体", IsLeaf: false},
			{Level: 3, Name: "具体事件", IsLeaf: true},
		},
	}}

	impact, err := previewConfigImpact(&newTmpl)
	if err != nil {
		t.Fatalf("previewConfigImpact: %v", err)
	}
	if impact.NewLeafViolations != 1 {
		t.Errorf("expected 1 leaf violation, got %d", impact.NewLeafViolations)
	}
}

func TestGeneratePendingChanges(t *testing.T) {
	db := setupConfigTestDB(t)

	impact := &ConfigImpact{
		Details: []ConfigImpactDetail{
			{TagID: 100, TagLabel: "test-tag", Category: "event", Issue: "depth_exceeded", Depth: 5},
			{TagID: 200, TagLabel: "cross-tag", Category: "event", Issue: "cross_category", Depth: 2, ParentID: ptr(uint(150))},
		},
	}

	if err := generatePendingChanges(impact); err != nil {
		t.Fatalf("generatePendingChanges: %v", err)
	}

	var changes []models.HierarchyPendingChange
	db.Find(&changes)
	if len(changes) != 2 {
		t.Fatalf("expected 2 pending changes, got %d", len(changes))
	}

	if changes[0].Status != "pending" {
		t.Errorf("expected status 'pending', got %q", changes[0].Status)
	}
}

func TestGetHierarchyManager_Singleton(t *testing.T) {
	mgr1 := GetHierarchyManager()
	mgr2 := GetHierarchyManager()
	if mgr1 != mgr2 {
		t.Fatal("GetHierarchyManager should return singleton")
	}
}

func TestHierarchyTemplateManager_AllTemplates(t *testing.T) {
	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	mgr.LoadSystemDefaults()

	all := mgr.AllTemplates()
	if len(all) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(all))
	}
}

func TestHierarchyTemplateManager_GetTemplateByKey(t *testing.T) {
	mgr := &HierarchyTemplateManager{templates: make(map[string]*CategoryHierarchyTemplate)}
	mgr.LoadSystemDefaults()

	if tmpl := mgr.GetTemplateByKey("keyword"); tmpl == nil {
		t.Fatal("GetTemplateByKey('keyword') returned nil")
	}
	if tmpl := mgr.GetTemplateByKey("nonexistent:key"); tmpl != nil {
		t.Fatal("GetTemplateByKey('nonexistent:key') should return nil")
	}
}

func TestTemplateViolationResult(t *testing.T) {
	db := setupConfigTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	parentEvent := models.TopicTag{Label: "p-event", Slug: "p-ev", Category: "event", Source: "abstract", Status: "active"}
	childKeyword := models.TopicTag{Label: "c-kw", Slug: "c-kw", Category: "keyword", Status: "active"}
	db.Create(&parentEvent)
	db.Create(&childKeyword)
	db.Create(&models.TopicTagRelation{ParentID: parentEvent.ID, ChildID: childKeyword.ID, RelationType: "abstract"})

	result, err := CleanupTemplateViolations()
	if err != nil {
		t.Fatalf("CleanupTemplateViolations: %v", err)
	}
	if result.CrossCategory < 1 {
		t.Errorf("expected at least 1 cross-category, got %d", result.CrossCategory)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func setupConfigTestDB(t *testing.T) *gorm.DB {
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
		&models.HierarchyConfig{},
		&models.HierarchyPendingChange{},
		&models.HierarchyConfigVersion{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}
