package narrative

import (
	"encoding/json"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

func createBoardFromAbstractTree(tree AbstractTreeNode, date time.Time, categoryID uint) (*models.NarrativeBoard, error) {
	eventTagIDs := collectBoardEventTagIDs(tree)
	if len(eventTagIDs) == 0 {
		return nil, nil
	}

	abstractTagID := tree.ID
	prevBoardIDs := matchPreviousBoard(abstractTagID, date, categoryID)

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	eventIDsJSON, _ := json.Marshal(eventTagIDs)
	abstractIDsJSON, _ := json.Marshal([]uint{abstractTagID})
	prevIDsJSON, _ := json.Marshal(prevBoardIDs)

	board := &models.NarrativeBoard{
		PeriodDate:      startOfDay,
		Name:            tree.Label,
		Description:     tree.Description,
		ScopeType:       models.NarrativeScopeTypeFeedCategory,
		ScopeCategoryID: &categoryID,
		EventTagIDs:     string(eventIDsJSON),
		AbstractTagIDs:  string(abstractIDsJSON),
		PrevBoardIDs:    string(prevIDsJSON),
		AbstractTagID:   &abstractTagID,
		IsSystem:        true,
	}

	if err := database.DB.Create(board).Error; err != nil {
		return nil, fmt.Errorf("save board from abstract tree %d: %w", tree.ID, err)
	}

	logging.Infof("board-creation: created board %d (%s) from abstract tree %d with %d event tags",
		board.ID, board.Name, tree.ID, len(eventTagIDs))
	return board, nil
}

func collectBoardEventTagIDs(tree AbstractTreeNode) []uint {
	var eventIDs []uint
	collectEventIDs(tree, &eventIDs)
	return eventIDs
}

func collectEventIDs(node AbstractTreeNode, ids *[]uint) {
	if node.Category == "event" {
		*ids = append(*ids, node.ID)
	}
	for _, child := range node.Children {
		collectEventIDs(child, ids)
	}
}

func matchPreviousBoard(abstractTagID uint, date time.Time, categoryID uint) []uint {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	var boards []models.NarrativeBoard
	database.DB.Where("abstract_tag_id = ? AND period_date >= ? AND period_date < ? AND scope_category_id = ?",
		abstractTagID, startOfYesterday, endOfYesterday, categoryID).
		Find(&boards)

	if len(boards) == 0 {
		return nil
	}

	var ids []uint
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	return ids
}
