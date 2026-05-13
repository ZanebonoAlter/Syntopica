package concept

import (
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

// ListActiveConcepts returns concepts WHERE status='active' AND category=?.
func ListActiveConcepts(category string) ([]models.BoardConcept, error) {
	var concepts []models.BoardConcept
	if err := database.DB.Where("status = ? AND category = ?", "active", category).
		Order("display_order ASC, id ASC").
		Find(&concepts).Error; err != nil {
		return nil, fmt.Errorf("list active concepts: %w", err)
	}
	return concepts, nil
}

// ListConcepts returns ALL concepts (including pending) WHERE category=?.
func ListConcepts(category string) ([]models.BoardConcept, error) {
	var concepts []models.BoardConcept
	if err := database.DB.Where("category = ?", category).
		Order("display_order ASC, id ASC").
		Find(&concepts).Error; err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	return concepts, nil
}

// GetConceptByID returns a single concept by its ID.
func GetConceptByID(id uint) (*models.BoardConcept, error) {
	var concept models.BoardConcept
	if err := database.DB.Where("id = ?", id).First(&concept).Error; err != nil {
		return nil, fmt.Errorf("get concept %d: %w", id, err)
	}
	return &concept, nil
}

// CreateConcept creates a new BoardConcept with status='active'.
func CreateConcept(name, description, category string) (*models.BoardConcept, error) {
	concept := &models.BoardConcept{
		Name:        name,
		Description: description,
		Category:    category,
		Status:      "active",
		ScopeType:   "global",
		IsSystem:    false,
	}

	if err := database.DB.Create(concept).Error; err != nil {
		return nil, fmt.Errorf("create concept: %w", err)
	}
	return concept, nil
}

// UpdateConcept updates the name and description of a concept.
func UpdateConcept(id uint, name, description string) (*models.BoardConcept, error) {
	concept, err := GetConceptByID(id)
	if err != nil {
		return nil, err
	}

	if err := database.DB.Model(concept).Updates(map[string]interface{}{
		"name":        name,
		"description": description,
	}).Error; err != nil {
		return nil, fmt.Errorf("update concept %d: %w", id, err)
	}
	concept.Name = name
	concept.Description = description
	return concept, nil
}

// DeactivateConcept sets a concept's status to 'inactive'.
func DeactivateConcept(id uint) error {
	result := database.DB.Model(&models.BoardConcept{}).
		Where("id = ?", id).
		Update("status", "inactive")
	if result.Error != nil {
		return fmt.Errorf("deactivate concept %d: %w", id, result.Error)
	}
	return nil
}

// ConfirmConcept sets a concept's status from 'pending' to 'active'.
func ConfirmConcept(id uint) error {
	result := database.DB.Model(&models.BoardConcept{}).
		Where("id = ? AND status = ?", id, "pending").
		Update("status", "active")
	if result.Error != nil {
		return fmt.Errorf("confirm concept %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("concept %d not found or not pending", id)
	}
	return nil
}

// MergeConcept sets the source concept status to 'merged' and moves its children
// to the target concept. The "children" are the rows in board_concepts where
// scope_category_id points to the source; those get reassigned to the target.
func MergeConcept(sourceID, targetID uint) error {
	if sourceID == targetID {
		return fmt.Errorf("cannot merge concept %d into itself", sourceID)
	}

	_, err := GetConceptByID(sourceID)
	if err != nil {
		return fmt.Errorf("merge source: %w", err)
	}
	_, err = GetConceptByID(targetID)
	if err != nil {
		return fmt.Errorf("merge target: %w", err)
	}

	tx := database.DB.Begin()

	if err := tx.Model(&models.BoardConcept{}).
		Where("scope_category_id = ?", sourceID).
		Update("scope_category_id", targetID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("reassign children from %d to %d: %w", sourceID, targetID, err)
	}

	if err := tx.Model(&models.BoardConcept{}).
		Where("id = ?", sourceID).
		Update("status", "merged").Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("mark source %d as merged: %w", sourceID, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit merge: %w", err)
	}
	return nil
}
