package tagging

import (
	"context"
	"errors"
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"

	"gorm.io/gorm"
)

const DefaultAutoSectorThreshold = 15

const (
	ClosureBlockerNoActiveSector   = "no_active_sector"
	ClosureBlockerNoMatchingSector = "no_matching_sector"
	ClosureBlockerPendingChanges   = "pending_changes"
	ClosureBlockerRebuildRunning   = "rebuild_running"
)

type HierarchyClosureStatus struct {
	Category           string             `json:"category"`
	ActiveSectorCount  int                `json:"active_sector_count"`
	UnplacedTagCount   int                `json:"unplaced_tag_count"`
	PendingChangeCount int                `json:"pending_change_count"`
	ActiveRebuildJob   *models.RebuildJob `json:"active_rebuild_job,omitempty"`
	BlockerCounts      map[string]int     `json:"blocker_counts"`
	TopBlocker         string             `json:"top_blocker,omitempty"`
}

type HierarchyClosureSummary struct {
	Category      string                  `json:"category"`
	InitialStatus *HierarchyClosureStatus `json:"initial_status,omitempty"`
	FinalStatus   *HierarchyClosureStatus `json:"final_status,omitempty"`
	Bootstrapped  bool                    `json:"bootstrapped"`
	PlacedCount   int                     `json:"placed_count"`
}

type HierarchyOrchestrationService struct {
	db                  *gorm.DB
	autoSectorThreshold int
}

func GetHierarchyOrchestrationService() *HierarchyOrchestrationService {
	return NewHierarchyOrchestrationService(database.DB)
}

func NewHierarchyOrchestrationService(db *gorm.DB) *HierarchyOrchestrationService {
	return &HierarchyOrchestrationService{db: db, autoSectorThreshold: DefaultAutoSectorThreshold}
}

func (s *HierarchyOrchestrationService) InspectCategoryClosureStatus(ctx context.Context, category string) (*HierarchyClosureStatus, error) {
	if s.db == nil {
		return nil, fmt.Errorf("inspect hierarchy closure status: database is nil")
	}
	status := &HierarchyClosureStatus{
		Category:      category,
		BlockerCounts: make(map[string]int),
	}

	var activeSectors int64
	if err := s.db.WithContext(ctx).Model(&models.BoardConcept{}).
		Where("category = ? AND status = ?", category, "active").
		Count(&activeSectors).Error; err != nil {
		return nil, fmt.Errorf("count active sectors: %w", err)
	}
	status.ActiveSectorCount = int(activeSectors)

	var unplacedTags int64
	if err := s.db.WithContext(ctx).Model(&models.TopicTag{}).
		Where("category = ? AND status = ? AND source <> ? AND concept_id IS NULL", category, "active", "abstract").
		Count(&unplacedTags).Error; err != nil {
		return nil, fmt.Errorf("count unplaced tags: %w", err)
	}
	status.UnplacedTagCount = int(unplacedTags)

	var pendingChanges int64
	if err := s.db.WithContext(ctx).Model(&models.HierarchyPendingChange{}).
		Joins("JOIN topic_tags ON topic_tags.id = hierarchy_pending_changes.tag_id").
		Where("hierarchy_pending_changes.status = ? AND topic_tags.category = ?", "pending", category).
		Count(&pendingChanges).Error; err != nil {
		return nil, fmt.Errorf("count pending changes: %w", err)
	}
	status.PendingChangeCount = int(pendingChanges)

	activeJob, err := s.findActiveRebuildJob(ctx, category, 0)
	if err != nil {
		return nil, err
	}
	status.ActiveRebuildJob = activeJob
	status.populateBlockers()
	return status, nil
}

func (s *HierarchyOrchestrationService) BootstrapCategory(ctx context.Context, category string) (bool, error) {
	return s.bootstrapCategory(ctx, category, 0)
}

func (s *HierarchyOrchestrationService) BootstrapCategoryForRebuild(ctx context.Context, category string, rebuildJobID uint) (bool, error) {
	return s.bootstrapCategory(ctx, category, rebuildJobID)
}

func (s *HierarchyOrchestrationService) RunCategoryClosureFlow(ctx context.Context, category string) (*HierarchyClosureSummary, error) {
	initial, err := s.InspectCategoryClosureStatus(ctx, category)
	if err != nil {
		return nil, err
	}

	bootstrapped, err := s.BootstrapCategory(ctx, category)
	if err != nil {
		return nil, err
	}

	placed, err := RetryOrphanPlacementsForCategory(ctx, category)
	if err != nil {
		return nil, err
	}

	final, err := s.RefreshSummary(ctx, category)
	if err != nil {
		return nil, err
	}

	return &HierarchyClosureSummary{
		Category:      category,
		InitialStatus: initial,
		FinalStatus:   final,
		Bootstrapped:  bootstrapped,
		PlacedCount:   placed,
	}, nil
}

func (s *HierarchyOrchestrationService) RefreshSummary(ctx context.Context, category string) (*HierarchyClosureStatus, error) {
	return s.InspectCategoryClosureStatus(ctx, category)
}

func (s *HierarchyOrchestrationService) bootstrapCategory(ctx context.Context, category string, ignoreRebuildJobID uint) (bool, error) {
	status, err := s.inspectCategoryClosureStatus(ctx, category, ignoreRebuildJobID)
	if err != nil {
		return false, err
	}
	if status.ActiveRebuildJob != nil {
		return false, nil
	}
	if status.UnplacedTagCount < s.autoSectorThreshold {
		return false, nil
	}
	if status.ActiveSectorCount > 0 {
		return false, nil
	}
	if err := AutoGenerateSectors(ctx, s.db, category, s.autoSectorThreshold); err != nil {
		return false, err
	}
	return true, nil
}

func (s *HierarchyOrchestrationService) inspectCategoryClosureStatus(ctx context.Context, category string, ignoreRebuildJobID uint) (*HierarchyClosureStatus, error) {
	status, err := s.InspectCategoryClosureStatus(ctx, category)
	if err != nil {
		return nil, err
	}
	if ignoreRebuildJobID == 0 || status.ActiveRebuildJob == nil || status.ActiveRebuildJob.ID != ignoreRebuildJobID {
		return status, nil
	}
	status.ActiveRebuildJob = nil
	status.populateBlockers()
	return status, nil
}

func (s *HierarchyOrchestrationService) findActiveRebuildJob(ctx context.Context, category string, ignoreRebuildJobID uint) (*models.RebuildJob, error) {
	var job models.RebuildJob
	query := s.db.WithContext(ctx).
		Where("category = ? AND status IN ?", category, []string{
			models.RebuildJobStatusPending,
			models.RebuildJobStatusRunning,
			models.RebuildJobStatusPaused,
		}).
		Order("created_at DESC")
	if ignoreRebuildJobID != 0 {
		query = query.Where("id <> ?", ignoreRebuildJobID)
	}
	if err := query.First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find active rebuild job: %w", err)
	}
	return &job, nil
}

func (s *HierarchyClosureStatus) populateBlockers() {
	s.BlockerCounts = make(map[string]int)
	if s.ActiveRebuildJob != nil {
		s.BlockerCounts[ClosureBlockerRebuildRunning] = 1
	}
	if s.UnplacedTagCount > 0 && s.ActiveSectorCount == 0 {
		s.BlockerCounts[ClosureBlockerNoActiveSector] = s.UnplacedTagCount
	}
	if s.UnplacedTagCount > 0 && s.ActiveSectorCount > 0 {
		s.BlockerCounts[ClosureBlockerNoMatchingSector] = s.UnplacedTagCount
	}
	if s.PendingChangeCount > 0 {
		s.BlockerCounts[ClosureBlockerPendingChanges] = s.PendingChangeCount
	}
	s.TopBlocker = topClosureBlocker(s.BlockerCounts)
}

func topClosureBlocker(counts map[string]int) string {
	for _, key := range []string{
		ClosureBlockerRebuildRunning,
		ClosureBlockerNoActiveSector,
		ClosureBlockerPendingChanges,
		ClosureBlockerNoMatchingSector,
	} {
		if counts[key] > 0 {
			return key
		}
	}
	return ""
}
