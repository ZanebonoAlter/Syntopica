package tagging

import (
	"testing"
)

func TestBuildAllDefaultTemplates(t *testing.T) {
	defaults := BuildAllDefaultTemplates()
	if len(defaults) != 3 {
		t.Fatalf("expected 3 default templates, got %d", len(defaults))
	}
}

func TestEventTemplate(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	tmpl := mgr.GetTemplate("event", "")
	if tmpl == nil {
		t.Fatal("event template not found")
	}
	if tmpl.MaxLevel != 3 {
		t.Fatalf("event max level expected 3, got %d", tmpl.MaxLevel)
	}
	if tmpl.GetLeafLevel() != 3 {
		t.Fatalf("event leaf level expected 3, got %d", tmpl.GetLeafLevel())
	}
	if tmpl.TemplateKey() != "event" {
		t.Fatalf("event template key expected 'event', got %s", tmpl.TemplateKey())
	}
	if !tmpl.IsLeafLevel(3) {
		t.Fatal("event L3 should be leaf")
	}
	if tmpl.IsLeafLevel(1) {
		t.Fatal("event L1 should not be leaf")
	}
}

func TestPersonTemplate(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	tmpl := mgr.GetTemplate("person", "")
	if tmpl == nil {
		t.Fatal("person template not found")
	}
	if tmpl.MaxLevel != 2 {
		t.Fatalf("person max level expected 2, got %d", tmpl.MaxLevel)
	}
	if tmpl.GetLeafLevel() != 2 {
		t.Fatalf("person leaf level expected 2, got %d", tmpl.GetLeafLevel())
	}
}

func TestKeywordTemplate(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	tmpl := mgr.GetTemplate("keyword", "")
	if tmpl == nil {
		t.Fatal("keyword template not found")
	}
	if tmpl.SubType != "" {
		t.Fatalf("keyword template should have empty SubType, got %q", tmpl.SubType)
	}
	if tmpl.MaxLevel != 3 {
		t.Fatalf("keyword max level expected 3, got %d", tmpl.MaxLevel)
	}
	if tmpl.GetLeafLevel() != 3 {
		t.Fatalf("keyword leaf level expected 3, got %d", tmpl.GetLeafLevel())
	}
	if tmpl.TemplateKey() != "keyword" {
		t.Fatalf("keyword template key expected 'keyword', got %s", tmpl.TemplateKey())
	}
	if tmpl.Levels[0].Name != "主题领域" {
		t.Fatalf("keyword L1 name expected '主题领域', got %q", tmpl.Levels[0].Name)
	}
}

func TestKeywordSubTypeReturnsNil(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	if tmpl := mgr.GetTemplateByKey("keyword:technology"); tmpl != nil {
		t.Fatal("keyword:technology key should not exist after removing subtypes")
	}
}

func TestGetTemplateFallback(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	keywordTmpl := mgr.GetTemplate("keyword", "")
	if keywordTmpl == nil {
		t.Fatal("keyword base template should exist")
	}

	unknownTmpl := mgr.GetTemplate("nonexistent", "")
	if unknownTmpl != nil {
		t.Fatal("nonexistent category should return nil")
	}
}

func TestResolveLevelFromDepth(t *testing.T) {
	t.Skip("ResolveLevelFromDepth removed — depth is now canonical")
}

func TestTemplateKey(t *testing.T) {
	tests := []struct {
		category string
		subType  string
		expected string
	}{
		{"event", "", "event"},
		{"keyword", "technology", "keyword:technology"},
		{"person", "", "person"},
	}

	for _, tt := range tests {
		tmpl := &CategoryHierarchyTemplate{Category: tt.category, SubType: tt.subType}
		if got := tmpl.TemplateKey(); got != tt.expected {
			t.Errorf("TemplateKey() = %q, want %q", got, tt.expected)
		}
	}
}

func TestGetLevelName(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	tmpl := mgr.GetTemplate("event", "")
	if tmpl == nil {
		t.Fatal("event template not found")
	}

	if name := tmpl.GetLevelName(1); name != "事件类型" {
		t.Errorf("event L1 name = %q, want '事件类型'", name)
	}
	if name := tmpl.GetLevelName(99); name != "L99" {
		t.Errorf("invalid level name = %q, want 'L99'", name)
	}
}
