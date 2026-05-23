package narrative

import (
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type PreviousBoardBrief struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func CollectPreviousDayBoards(date time.Time, scopeType string, categoryID *uint) ([]PreviousBoardBrief, error) {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	query := database.DB.Model(&models.NarrativeBoard{}).
		Where("period_date >= ? AND period_date < ?", startOfYesterday, endOfYesterday)

	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
	}
	if categoryID != nil {
		query = query.Where("scope_category_id = ?", *categoryID)
	}

	var boards []models.NarrativeBoard
	if err := query.Order("id ASC").Find(&boards).Error; err != nil {
		return nil, fmt.Errorf("collect previous day boards: %w", err)
	}

	if len(boards) == 0 {
		return nil, nil
	}

	result := make([]PreviousBoardBrief, 0, len(boards))
	for _, b := range boards {
		result = append(result, PreviousBoardBrief{
			ID:          b.ID,
			Name:        b.Name,
			Description: b.Description,
		})
	}

	logging.Infof("board-collector: found %d previous day boards for %s (scope=%s, category=%v)",
		len(result), yesterday.Format("2006-01-02"), scopeType, categoryID)
	return result, nil
}

type BoardNarrativeBrief struct {
	ID      uint64 `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
	BoardID uint   `json:"board_id"`
}

func CollectPreviousBoardNarratives(prevBoardIDs []uint) ([]BoardNarrativeBrief, error) {
	if len(prevBoardIDs) == 0 {
		return nil, nil
	}

	var narratives []models.NarrativeSummary
	if err := database.DB.Where("board_id IN ?", prevBoardIDs).
		Order("id ASC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("collect previous board narratives: %w", err)
	}

	if len(narratives) == 0 {
		return nil, nil
	}

	result := make([]BoardNarrativeBrief, 0, len(narratives))
	for _, n := range narratives {
		boardID := uint(0)
		if n.BoardID != nil {
			boardID = *n.BoardID
		}
		result = append(result, BoardNarrativeBrief{
			ID:      n.ID,
			Title:   n.Title,
			Summary: n.Summary,
			Status:  n.Status,
			BoardID: boardID,
		})
	}

	logging.Infof("board-collector: found %d narratives from %d previous boards", len(result), len(prevBoardIDs))
	return result, nil
}
