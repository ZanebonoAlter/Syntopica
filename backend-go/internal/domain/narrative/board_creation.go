package narrative

import (
	"encoding/json"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

func createBoardFromSemanticBoard(input SemanticBoardNarrativeInput, date time.Time, scopeOpts ScopeSaveOpts) (*models.NarrativeBoard, error) {
	if len(input.EventTags) == 0 {
		return nil, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	eventTagIDs := make([]uint, 0, len(input.EventTags))
	for _, tag := range input.EventTags {
		eventTagIDs = append(eventTagIDs, tag.ID)
	}

	eventIDsJSON, _ := json.Marshal(eventTagIDs)
	prevIDsJSON, _ := json.Marshal(input.PrevBoardIDs)
	semanticBoardID := input.Board.ID

	board := &models.NarrativeBoard{
		PeriodDate:      startOfDay,
		Name:            input.Board.Label,
		Description:     input.Board.Description,
		ScopeType:       scopeOpts.ScopeType,
		ScopeCategoryID: scopeOpts.CategoryID,
		ScopeLabel:      scopeOpts.Label,
		EventTagIDs:     string(eventIDsJSON),
		PrevBoardIDs:    string(prevIDsJSON),
		SemanticBoardID: &semanticBoardID,
		IsSystem:        true,
	}

	if err := database.DB.Create(board).Error; err != nil {
		return nil, fmt.Errorf("save narrative board from semantic board %d: %w", input.Board.ID, err)
	}

	logging.Infof("board-creation: created narrative board %d from semantic board %d with %d event tags",
		board.ID, input.Board.ID, len(eventTagIDs))
	return board, nil
}

func matchPreviousSemanticBoard(semanticBoardID uint, date time.Time, scopeType string, categoryID *uint) []uint {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	query := database.DB.Where("semantic_board_id = ? AND scope_type = ? AND period_date >= ? AND period_date < ?",
		semanticBoardID, scopeType, startOfYesterday, endOfYesterday)
	if categoryID != nil {
		query = query.Where("scope_category_id = ?", *categoryID)
	} else {
		query = query.Where("scope_category_id IS NULL")
	}

	var boards []models.NarrativeBoard
	query.Order("id ASC").Find(&boards)
	if len(boards) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(boards))
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	return ids
}
