package topicanalysis

import (
	"testing"
)

func TestBuildAllDefaultTemplates(t *testing.T) {
	defaults := BuildAllDefaultTemplates()
	if len(defaults) != 5 {
		t.Fatalf("expected 5 default templates, got %d", len(defaults))
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

func TestKeywordSubTypeTemplates(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	if tmpl := mgr.GetTemplate("keyword", "technology"); tmpl == nil {
		t.Fatal("keyword:technology template not found")
	}
	if tmpl := mgr.GetTemplate("keyword", "company_business"); tmpl == nil {
		t.Fatal("keyword:company_business template not found")
	}
	if tmpl := mgr.GetTemplate("keyword", "concept"); tmpl == nil {
		t.Fatal("keyword:concept template not found")
	}
}

func TestGetTemplateFallback(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	techTmpl := mgr.GetTemplate("keyword", "technology")
	if techTmpl == nil || techTmpl.SubType != "technology" {
		t.Fatal("keyword:technology should return specific subtype template")
	}

	unknownTmpl := mgr.GetTemplate("keyword", "nonexistent")
	if unknownTmpl != nil {
		t.Fatal("keyword:nonexistent should return nil, got a template")
	}
}

func TestResolveLevelFromDepth(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	tests := []struct {
		category string
		depth    int
		expected int
	}{
		{"event", 0, 1},
		{"event", 1, 2},
		{"event", 2, 3},
		{"event", 5, 3},
		{"person", 0, 1},
		{"person", 1, 2},
		{"person", 3, 2},
		{"nonexistent", 0, 1},
		{"nonexistent", 3, 4},
		{"nonexistent", 5, 4},
	}

	for _, tt := range tests {
		got := ResolveLevelFromDepth(tt.category, tt.depth)
		if got != tt.expected {
			t.Errorf("ResolveLevelFromDepth(%s, %d) = %d, want %d", tt.category, tt.depth, got, tt.expected)
		}
	}
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
