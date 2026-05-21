package tagging

import (
	"os"
	"strings"
	"testing"
)

func TestHierarchyOrchestrationServiceDefinesClosureFlowSurface(t *testing.T) {
	content, err := os.ReadFile("hierarchy_orchestration.go")
	if err != nil {
		t.Fatalf("read hierarchy_orchestration.go: %v", err)
	}
	source := string(content)

	for _, marker := range []string{
		"type HierarchyClosureStatus struct",
		"ActiveSectorCount",
		"UnplacedTagCount",
		"PendingChangeCount",
		"ActiveRebuildJob",
		"BlockerCounts",
		"func (s *HierarchyOrchestrationService) InspectCategoryClosureStatus",
		"func (s *HierarchyOrchestrationService) BootstrapCategory",
		"func (s *HierarchyOrchestrationService) RunCategoryClosureFlow",
		"func (s *HierarchyOrchestrationService) RefreshSummary",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("expected hierarchy orchestration surface to contain %s", marker)
		}
	}
}

func TestPlacementSchedulerUsesHierarchyOrchestrationFlow(t *testing.T) {
	content, err := os.ReadFile("../../jobs/tag_hierarchy_placement.go")
	if err != nil {
		t.Fatalf("read tag_hierarchy_placement.go: %v", err)
	}
	body := extractSourceBetween(t, string(content), "func (s *TagHierarchyPlacementScheduler) executePlacementCycle", "func (s *TagHierarchyPlacementScheduler) updateSchedulerStatus")
	if !strings.Contains(body, "NewHierarchyOrchestrationService") || !strings.Contains(body, "RunCategoryClosureFlow") {
		t.Fatal("expected placement scheduler to run hierarchy orchestration flow")
	}
}

func TestRebuildServiceRunsHierarchyBootstrapPreflight(t *testing.T) {
	content, err := os.ReadFile("rebuild_service.go")
	if err != nil {
		t.Fatalf("read rebuild_service.go: %v", err)
	}
	body := extractSourceBetween(t, string(content), "func (s *RebuildService) ExecuteJob", "func (s *RebuildService) ResumeJob")
	if !strings.Contains(body, "BootstrapCategoryForRebuild") {
		t.Fatal("expected rebuild execution to run hierarchy bootstrap preflight")
	}
}
