package tagging

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type HierarchyTemplateManager struct {
	mu        sync.RWMutex
	templates map[string]*CategoryHierarchyTemplate
	configID  uint
	version   int
}

var hierarchyManagerInstance *HierarchyTemplateManager
var hierarchyManagerOnce sync.Once

func GetHierarchyManager() *HierarchyTemplateManager {
	hierarchyManagerOnce.Do(func() {
		hierarchyManagerInstance = &HierarchyTemplateManager{
			templates: make(map[string]*CategoryHierarchyTemplate),
		}
	})
	return hierarchyManagerInstance
}

func (m *HierarchyTemplateManager) loadSystemDefaultsLocked() {
	defaults := BuildAllDefaultTemplates()
	for _, t := range defaults {
		m.templates[t.TemplateKey()] = t
	}
	logging.Infof("Loaded %d default hierarchy templates", len(defaults))
}

func (m *HierarchyTemplateManager) LoadSystemDefaults() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadSystemDefaultsLocked()
}

func (m *HierarchyTemplateManager) loadFromDBLocked() error {
	if database.DB == nil {
		logging.Warnf("Database not available, using system defaults only")
		return nil
	}

	var config models.HierarchyConfig
	if err := database.DB.Order("version DESC").First(&config).Error; err != nil {
		logging.Warnf("No hierarchy config found in DB, system defaults will be saved on first update: %v", err)
		return nil
	}

	if len(config.Templates) == 0 {
		logging.Infoln("Hierarchy config exists but templates are empty, using system defaults")
		return nil
	}

	m.configID = config.ID
	m.version = config.Version

	for key, raw := range config.Templates {
		var t CategoryHierarchyTemplate
		if err := json.Unmarshal(raw, &t); err != nil {
			logging.Warnf("Failed to parse template %s from DB: %v, using default", key, err)
			continue
		}
		m.templates[key] = &t
	}

	logging.Infof("Loaded hierarchy config version %d with %d templates", m.version, len(config.Templates))
	return nil
}

func (m *HierarchyTemplateManager) LoadFromDB() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadSystemDefaultsLocked()
	return m.loadFromDBLocked()
}

func (m *HierarchyTemplateManager) GetTemplate(category, subType string) *CategoryHierarchyTemplate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if subType != "" {
		key := category + ":" + subType
		if t, ok := m.templates[key]; ok {
			return t
		}
	}

	if t, ok := m.templates[category]; ok {
		return t
	}

	return nil
}

func (m *HierarchyTemplateManager) GetTemplateByKey(key string) *CategoryHierarchyTemplate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates[key]
}

func (m *HierarchyTemplateManager) AllTemplates() []*CategoryHierarchyTemplate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*CategoryHierarchyTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		result = append(result, t)
	}
	return result
}

func (m *HierarchyTemplateManager) GetVersion() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *HierarchyTemplateManager) SaveConfig(templatesJSON map[string]json.RawMessage, changeLog string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if database.DB == nil {
		return fmt.Errorf("database not available")
	}

	newVersion := m.version + 1

	config := models.HierarchyConfig{
		Templates: models.HierarchyTemplatesJSON(templatesJSON),
		Version:   newVersion,
	}

	if err := database.DB.Create(&config).Error; err != nil {
		return fmt.Errorf("save hierarchy config: %w", err)
	}

	oldTemplates := models.HierarchyTemplatesJSON{}
	if m.configID > 0 {
		var oldConfig models.HierarchyConfig
		if err := database.DB.First(&oldConfig, m.configID).Error; err == nil {
			oldTemplates = oldConfig.Templates
		}
	}

	versionRecord := models.HierarchyConfigVersion{
		ConfigID:  config.ID,
		Version:   newVersion,
		Templates: oldTemplates,
		ChangeLog: changeLog,
	}
	if err := database.DB.Create(&versionRecord).Error; err != nil {
		logging.Warnf("Failed to save hierarchy config version record: %v", err)
	}

	for key, raw := range templatesJSON {
		var t CategoryHierarchyTemplate
		if err := json.Unmarshal(raw, &t); err != nil {
			logging.Warnf("Failed to parse template %s after save: %v", key, err)
			continue
		}
		m.templates[key] = &t
	}

	m.configID = config.ID
	m.version = newVersion

	logging.Infof("Hierarchy config saved: version %d, %d templates, change: %s", newVersion, len(templatesJSON), changeLog)
	return nil
}

func (m *HierarchyTemplateManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.templates = make(map[string]*CategoryHierarchyTemplate)
	m.loadSystemDefaultsLocked()
	return m.loadFromDBLocked()
}

type ConfigImpact struct {
	TotalTags                       int                  `json:"total_tags"`
	AffectedTagCount                int                  `json:"affected_tag_count"`
	EstimatedRebuildDurationSeconds int                  `json:"estimated_rebuild_duration_seconds"`
	DepthExceeded                   int                  `json:"depth_exceeded"`
	LevelMismatch                   int                  `json:"level_mismatch"`
	CrossCategory                   int                  `json:"cross_category"`
	NewLeafViolations               int                  `json:"new_leaf_violations"`
	ViolationSummary                map[string]int       `json:"violation_summary,omitempty"`
	Details                         []ConfigImpactDetail `json:"details,omitempty"`
}

type ConfigImpactDetail struct {
	TagID    uint   `json:"tag_id"`
	TagLabel string `json:"tag_label"`
	Category string `json:"category"`
	Issue    string `json:"issue"`
	Depth    int    `json:"depth"`
	ParentID *uint  `json:"parent_id,omitempty"`
}

func previewConfigImpact(newTemplates *[]CategoryHierarchyTemplate) (*ConfigImpact, error) {
	impact := &ConfigImpact{}

	tmplByKey := make(map[string]*CategoryHierarchyTemplate)
	for i := range *newTemplates {
		t := &(*newTemplates)[i]
		tmplByKey[t.TemplateKey()] = t
	}

	var allTags []models.TopicTag
	if err := database.DB.Where("status = 'active'").Find(&allTags).Error; err != nil {
		return nil, fmt.Errorf("load all tags: %w", err)
	}
	impact.TotalTags = len(allTags)

	for _, tag := range allTags {
		tmpl, ok := tmplByKey[tag.Category]
		if !ok {
			continue
		}

		depth := getTagDepthFromRoot(tag.ID)

		if depth+1 > tmpl.MaxLevel {
			impact.DepthExceeded++
			impact.Details = append(impact.Details, ConfigImpactDetail{
				TagID: tag.ID, TagLabel: tag.Label, Category: tag.Category,
				Issue: "depth_exceeded", Depth: depth,
			})
		}

		leafLevel := tmpl.GetLeafLevel()
		if depth+1 == leafLevel && tmpl.IsLeafLevel(leafLevel) {
			var childCount int64
			database.DB.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND relation_type = 'abstract'", tag.ID).Count(&childCount)
			if childCount > 0 {
				impact.NewLeafViolations++
				impact.Details = append(impact.Details, ConfigImpactDetail{
					TagID: tag.ID, TagLabel: tag.Label, Category: tag.Category,
					Issue: "leaf_with_children", Depth: depth,
				})
			}
		}

		var crossRelation models.TopicTagRelation
		if err := database.DB.Where("child_id = ? AND relation_type = 'abstract'", tag.ID).
			Joins("JOIN topic_tags on topic_tags.id = topic_tag_relations.parent_id").
			Where("topic_tags.category != ?", tag.Category).
			First(&crossRelation).Error; err == nil {
			impact.CrossCategory++
			impact.Details = append(impact.Details, ConfigImpactDetail{
				TagID: tag.ID, TagLabel: tag.Label, Category: tag.Category,
				Issue: "cross_category", ParentID: &crossRelation.ParentID,
			})
		}
	}

	impact.finalize()
	return impact, nil
}

func (i *ConfigImpact) finalize() {
	if i.ViolationSummary == nil {
		i.ViolationSummary = map[string]int{}
	}
	for _, d := range i.Details {
		i.ViolationSummary[d.Issue]++
	}

	seen := make(map[uint]bool)
	for _, d := range i.Details {
		if seen[d.TagID] {
			continue
		}
		seen[d.TagID] = true
		i.AffectedTagCount++
	}
	estimated := time.Duration(i.AffectedTagCount) * defaultAvgPlacementTime
	i.EstimatedRebuildDurationSeconds = int(estimated.Seconds())
}

func generatePendingChanges(impact *ConfigImpact) error {
	for _, d := range impact.Details {
		change := models.HierarchyPendingChange{
			TagID:      d.TagID,
			TagLabel:   d.TagLabel,
			ChangeType: d.Issue,
			Reason:     fmt.Sprintf("Config impact: %s at depth=%d", d.Issue, d.Depth),
			Status:     "pending",
		}
		if d.ParentID != nil {
			change.CurrentParentID = d.ParentID
			var parent models.TopicTag
			if err := database.DB.First(&parent, *d.ParentID).Error; err == nil {
				change.CurrentParentLabel = parent.Label
			}
		}
		if err := database.DB.Create(&change).Error; err != nil {
			logging.Warnf("Failed to create pending change for tag %d: %v", d.TagID, err)
		}
	}
	return nil
}
