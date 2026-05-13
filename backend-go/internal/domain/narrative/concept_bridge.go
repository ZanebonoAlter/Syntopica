package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"my-robot-backend/internal/domain/concept"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

var (
	unclassifiedMu      sync.Mutex
	pendingUnclassified []TagInput
)

func AddToUnclassifiedBucket(tag TagInput) {
	unclassifiedMu.Lock()
	defer unclassifiedMu.Unlock()
	pendingUnclassified = append(pendingUnclassified, tag)
}

func GetUnclassifiedBucket() []TagInput {
	unclassifiedMu.Lock()
	defer unclassifiedMu.Unlock()
	return pendingUnclassified
}

func ClearUnclassifiedBucket() {
	unclassifiedMu.Lock()
	defer unclassifiedMu.Unlock()
	pendingUnclassified = nil
}

func MatchTagToConcept(ctx context.Context, tag TagInput) (*concept.ConceptMatchResult, error) {
	return concept.MatchTagToConcept(ctx, tag.Label, tag.Description, tag.Category, tag.ID)
}

func BuildBoardFromMatchedTags(conceptID uint, conceptName string, matchedTags []TagInput, date time.Time, categoryID *uint) (*models.NarrativeBoard, error) {
	if len(matchedTags) == 0 {
		return nil, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	eventIDs := make([]uint, 0, len(matchedTags))
	for _, t := range matchedTags {
		eventIDs = append(eventIDs, t.ID)
	}

	eventIDsJSON, _ := json.Marshal(eventIDs)
	abstractIDsJSON, _ := json.Marshal([]uint{})

	prevBoardIDs := matchConceptPreviousBoard(conceptID, date, categoryID)
	prevIDsJSON, _ := json.Marshal(prevBoardIDs)

	scopeType := models.NarrativeScopeTypeFeedCategory
	if categoryID == nil {
		scopeType = models.NarrativeScopeTypeGlobal
	}

	board := &models.NarrativeBoard{
		PeriodDate:      startOfDay,
		Name:            conceptName,
		Description:     "",
		ScopeType:       scopeType,
		ScopeCategoryID: categoryID,
		EventTagIDs:     string(eventIDsJSON),
		AbstractTagIDs:  string(abstractIDsJSON),
		PrevBoardIDs:    string(prevIDsJSON),
		BoardConceptID:  &conceptID,
		IsSystem:        false,
	}

	if err := database.DB.Create(board).Error; err != nil {
		return nil, fmt.Errorf("create matched concept board: %w", err)
	}

	logging.Infof("narrative: created board %d (%s) from concept %d with %d tags",
		board.ID, board.Name, conceptID, len(matchedTags))
	return board, nil
}

const defaultHotspotThreshold = 3

func getHotspotThreshold() int {
	var setting models.AISettings
	if err := database.DB.Where("key = ?", "narrative_board_hotspot_threshold").First(&setting).Error; err != nil {
		return defaultHotspotThreshold
	}
	if val, err := strconv.Atoi(setting.Value); err == nil && val >= 2 {
		return val
	}
	return defaultHotspotThreshold
}

func matchConceptPreviousBoard(conceptID uint, date time.Time, categoryID *uint) []uint {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	query := database.DB.Where("board_concept_id = ? AND period_date >= ? AND period_date < ?",
		conceptID, startOfYesterday, endOfYesterday)
	if categoryID != nil {
		query = query.Where("scope_category_id = ?", *categoryID)
	}

	var boards []models.NarrativeBoard
	query.Find(&boards)

	if len(boards) == 0 {
		return nil
	}

	var ids []uint
	for _, b := range boards {
		ids = append(ids, b.ID)
	}
	return ids
}
