package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

type NarrativeService struct{}

func NewNarrativeService() *NarrativeService {
	return &NarrativeService{}
}

func (s *NarrativeService) DeleteByDate(date time.Time, scopeType string, categoryID *uint) (int, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := database.DB.Where("period = ? AND period_date >= ? AND period_date < ?", "daily", startOfDay, endOfDay)

	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			query = query.Where("scope_category_id = ?", *categoryID)
		}
	}

	result := query.Delete(&models.NarrativeSummary{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete narratives for %s: %w", date.Format("2006-01-02"), result.Error)
	}

	boardQuery := database.DB.Where("period_date >= ? AND period_date < ?", startOfDay, endOfDay)
	if scopeType != "" {
		boardQuery = boardQuery.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			boardQuery = boardQuery.Where("scope_category_id = ?", *categoryID)
		}
	}
	boardResult := boardQuery.Delete(&models.NarrativeBoard{})
	if boardResult.Error != nil {
		logging.Warnf("narrative: failed to delete boards for %s: %v", date.Format("2006-01-02"), boardResult.Error)
	}

	logging.Infof("narrative: deleted %d existing narratives and %d boards for %s (scope=%s)",
		result.RowsAffected, boardResult.RowsAffected, date.Format("2006-01-02"), scopeType)
	return int(result.RowsAffected), nil
}

func (s *NarrativeService) RegenerateAndSave(date time.Time) (int, error) {
	deleted, err := s.DeleteByDate(date, "", nil)
	if err != nil {
		return 0, err
	}
	logging.Infof("narrative: deleted %d old narratives before regenerating for %s", deleted, date.Format("2006-01-02"))

	return s.GenerateAndSave(date)
}

func (s *NarrativeService) RegenerateAndSaveForCategory(date time.Time, categoryID uint) (int, error) {
	deleted, err := s.DeleteByDate(date, models.NarrativeScopeTypeFeedCategory, &categoryID)
	if err != nil {
		return 0, err
	}
	logging.Infof("narrative: deleted %d old category narratives for category %d before regenerating", deleted, categoryID)

	var cat models.Category
	if err := database.DB.Where("id = ?", categoryID).First(&cat).Error; err != nil {
		return 0, fmt.Errorf("category %d not found: %w", categoryID, err)
	}

	return s.GenerateAndSaveForCategory(date, categoryID, cat.Name)
}

type ScopeSaveOpts struct {
	ScopeType  string
	CategoryID *uint
	Label      string
}

func (s *NarrativeService) GenerateAndSave(date time.Time) (int, error) {
	return s.GenerateAndSaveForAllBoards(date)
}

func (s *NarrativeService) GenerateAndSaveForBoard(semanticBoardID uint, date time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Collect all board inputs
	allInputs, err := CollectSemanticBoardNarrativeInputs(date)
	if err != nil {
		return 0, fmt.Errorf("collect semantic board inputs: %w", err)
	}

	// Filter to target board
	var targetInput *SemanticBoardNarrativeInput
	for i := range allInputs {
		if allInputs[i].Board.ID == semanticBoardID {
			targetInput = &allInputs[i]
			break
		}
	}
	if targetInput == nil {
		return 0, fmt.Errorf("semantic board %d not found or has no event tags for %s", semanticBoardID, date.Format("2006-01-02"))
	}

	scopeOpts := ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeBoard}
	board, bErr := createBoardFromSemanticBoard(*targetInput, date, scopeOpts)
	if bErr != nil {
		return 0, fmt.Errorf("create narrative board: %w", bErr)
	}
	if board == nil {
		return 0, nil
	}

	eventTags := targetInput.EventTags
	if len(eventTags) == 0 {
		return 0, nil
	}

	prevNarrs := collectPreviousNarrativesForBoards(targetInput.PrevBoardIDs)
	boardCtx := BoardNarrativeContext{
		Board:              *board,
		EventTags:          eventTags,
		PrevNarratives:     prevNarrs,
		SemanticBoardLabel: targetInput.Board.Label,
		SemanticBoardDesc:  targetInput.Board.Description,
	}

	outputs, gErr := GenerateNarrativesForBoard(ctx, boardCtx)
	if gErr != nil {
		return 0, fmt.Errorf("generate narratives: %w", gErr)
	}

	saved, sErr := saveNarrativesWithBoard(outputs, *board, date, &scopeOpts)
	if sErr != nil {
		return 0, fmt.Errorf("save narratives: %w", sErr)
	}

	logging.Infof("narrative: GenerateAndSaveForBoard complete — %d narratives saved for board %d on %s",
		saved, semanticBoardID, date.Format("2006-01-02"))
	return saved, nil
}

func (s *NarrativeService) RegenerateAndSaveForBoard(semanticBoardID uint, date time.Time) (int, error) {
	// Delete existing narratives for this board on this date
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var boards []models.NarrativeBoard
	database.DB.Where("semantic_board_id = ? AND period_date >= ? AND period_date < ?", semanticBoardID, startOfDay, endOfDay).Find(&boards)
	for _, nb := range boards {
		database.DB.Where("board_id = ?", nb.ID).Delete(&models.NarrativeSummary{})
		database.DB.Delete(&nb)
	}

	return s.GenerateAndSaveForBoard(semanticBoardID, date)
}

func (s *NarrativeService) GenerateAndSaveForAllBoards(date time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	input, err := CollectSemanticBoardNarrativeInputs(date)
	if err != nil {
		return 0, fmt.Errorf("collect semantic board inputs: %w", err)
	}
	if len(input) == 0 {
		logging.Infof("narrative: no semantic board event tags for %s", date.Format("2006-01-02"))
		return 0, nil
	}

	totalSaved := 0
	for _, inp := range input {
		scopeOpts := ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeBoard}
		board, bErr := createBoardFromSemanticBoard(inp, date, scopeOpts)
		if bErr != nil {
			logging.Warnf("narrative: failed to create narrative board from semantic board %d: %v", inp.Board.ID, bErr)
			continue
		}
		if board == nil {
			continue
		}
		eventTags := inp.EventTags
		if len(eventTags) == 0 {
			continue
		}

		prevNarrs := collectPreviousNarrativesForBoards(inp.PrevBoardIDs)
		boardCtx := BoardNarrativeContext{
			Board:              *board,
			EventTags:          eventTags,
			PrevNarratives:     prevNarrs,
			SemanticBoardLabel: inp.Board.Label,
			SemanticBoardDesc:  inp.Board.Description,
		}

		outputs, gErr := GenerateNarrativesForBoard(ctx, boardCtx)
		if gErr != nil {
			logging.Warnf("narrative: failed to generate narratives for board %d: %v", board.ID, gErr)
			continue
		}

		saved, sErr := saveNarrativesWithBoard(outputs, *board, date, &scopeOpts)
		if sErr != nil {
			logging.Warnf("narrative: failed to save narratives for board %d: %v", board.ID, sErr)
			continue
		}
		totalSaved += saved
	}

	// Post-generation steps
	allPrev, pErr := CollectPreviousNarratives(date, "", nil)
	if pErr != nil {
		logging.Warnf("narrative: failed to collect previous narratives for fallback: %v", pErr)
	} else if len(allPrev) > 0 {
		s.runFallbackAssociations(ctx, date, allPrev)
	}

	if _, cErr := DeriveBoardConnections(); cErr != nil {
		logging.Warnf("narrative: failed to derive board connections: %v", cErr)
	}

	s.runFeedbackFromTodayNarratives(date)

	cleanEmptyBoards(date, nil)

	logging.Infof("narrative: GenerateAndSaveForAllBoards complete — %d narratives saved for %s",
		totalSaved, date.Format("2006-01-02"))
	return totalSaved, nil
}

// Deprecated: Use GenerateAndSaveForAllBoards instead. Kept for rollback safety.
func (s *NarrativeService) GenerateAndSaveGlobal(ctx context.Context, date time.Time) (int, error) {
	scopeOpts := ScopeSaveOpts{
		ScopeType:  models.NarrativeScopeTypeGlobal,
		CategoryID: nil,
		Label:      "",
	}
	return s.generateAndSaveSemanticBoardScope(ctx, date, scopeOpts)
}

// Deprecated: Use GenerateAndSaveForAllBoards instead. Kept for rollback safety.
func (s *NarrativeService) GenerateAndSaveForCategory(date time.Time, categoryID uint, categoryLabel string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	scopeOpts := ScopeSaveOpts{
		ScopeType:  models.NarrativeScopeTypeFeedCategory,
		CategoryID: &categoryID,
		Label:      categoryLabel,
	}
	saved, err := s.generateAndSaveSemanticBoardScope(ctx, date, scopeOpts)
	if err != nil {
		return 0, fmt.Errorf("generate semantic boards for category %d: %w", categoryID, err)
	}

	logging.Infof("narrative: saved %d semantic-board narratives for category %d (%s) on %s",
		saved, categoryID, categoryLabel, date.Format("2006-01-02"))
	cleanEmptyBoards(date, &categoryID)
	return saved, nil
}

// Deprecated: Use GenerateAndSaveForAllBoards instead. Kept for rollback safety.
func (s *NarrativeService) generateAndSaveSemanticBoardScope(ctx context.Context, date time.Time, scopeOpts ScopeSaveOpts) (int, error) {
	inputs, err := CollectSemanticBoardNarrativeInputs(date)
	if err != nil {
		return 0, err
	}
	if len(inputs) == 0 {
		logging.Infof("narrative: no semantic board event tags for %s (scope=%s, category=%v)", date.Format("2006-01-02"), scopeOpts.ScopeType, scopeOpts.CategoryID)
		return 0, nil
	}

	totalSaved := 0
	for _, input := range inputs {
		board, bErr := createBoardFromSemanticBoard(input, date, scopeOpts)
		if bErr != nil {
			logging.Warnf("narrative: failed to create narrative board from semantic board %d: %v", input.Board.ID, bErr)
			continue
		}
		if board != nil {
			eventTags := input.EventTags
			if len(eventTags) == 0 {
				continue
			}

			prevNarrs := collectPreviousNarrativesForBoards(input.PrevBoardIDs)
			boardCtx := BoardNarrativeContext{
				Board:              *board,
				EventTags:          eventTags,
				PrevNarratives:     prevNarrs,
				SemanticBoardLabel: input.Board.Label,
				SemanticBoardDesc:  input.Board.Description,
			}

			outputs, gErr := GenerateNarrativesForBoard(ctx, boardCtx)
			if gErr != nil {
				logging.Warnf("narrative: failed to generate narratives for board %d: %v", board.ID, gErr)
				continue
			}

			saved, sErr := saveNarrativesWithBoard(outputs, *board, date, &scopeOpts)
			if sErr != nil {
				logging.Warnf("narrative: failed to save narratives for board %d: %v", board.ID, sErr)
				continue
			}
			totalSaved += saved
		}
	}

	logging.Infof("narrative: saved %d narratives across %d semantic boards for %s (scope=%s, category=%v)",
		totalSaved, len(inputs), date.Format("2006-01-02"), scopeOpts.ScopeType, scopeOpts.CategoryID)
	return totalSaved, nil
}

func collectPreviousNarrativesForBoards(prevBoardIDs []uint) []PreviousNarrative {
	if len(prevBoardIDs) == 0 {
		return nil
	}

	var prevSummaries []models.NarrativeSummary
	database.DB.Where("board_id IN ?", prevBoardIDs).Order("id ASC").Find(&prevSummaries)
	prevNarrs := make([]PreviousNarrative, 0, len(prevSummaries))
	for _, ps := range prevSummaries {
		prevNarrs = append(prevNarrs, PreviousNarrative{
			ID:         uint64(ps.ID),
			Title:      ps.Title,
			Summary:    ps.Summary,
			Status:     ps.Status,
			Generation: ps.Generation,
		})
	}
	return prevNarrs
}

func (s *NarrativeService) runFallbackAssociations(ctx context.Context, date time.Time, allPrev []PreviousNarrative) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var todayNarratives []models.NarrativeSummary
	database.DB.Where("period_date >= ? AND period_date < ? AND parent_ids != '' AND parent_ids != '[]'",
		startOfDay, endOfDay).
		Order("id ASC").
		Find(&todayNarratives)

	resolved := 0
	for _, n := range todayNarratives {
		if resolved >= 10 {
			break
		}

		newParentIDs, err := fallbackNarrativeAssociation(ctx, n, allPrev)
		if err != nil {
			logging.Warnf("narrative: fallback association failed for narrative %d: %v", n.ID, err)
			continue
		}
		if newParentIDs != nil {
			parentIDsJSON, _ := json.Marshal(newParentIDs)
			database.DB.Model(&models.NarrativeSummary{}).Where("id = ?", n.ID).Update("parent_ids", string(parentIDsJSON))
			resolved++
		}
	}

	if resolved > 0 {
		logging.Infof("narrative: resolved %d narrative parent associations via fallback", resolved)
	}
}

func (s *NarrativeService) runFeedbackFromTodayNarratives(date time.Time) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var todayNarratives []models.NarrativeSummary
	database.DB.Where("period_date >= ? AND period_date < ? AND source = ?",
		startOfDay, endOfDay, "ai").Find(&todayNarratives)

	if len(todayNarratives) == 0 {
		return
	}

	var feedbackOutputs []NarrativeOutput
	for _, n := range todayNarratives {
		var tagIDs []uint
		if n.RelatedTagIDs != "" {
			_ = json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
		}
		var parentIDs []uint
		if n.ParentIDs != "" {
			_ = json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		}
		feedbackOutputs = append(feedbackOutputs, NarrativeOutput{
			Title:         n.Title,
			Summary:       n.Summary,
			Status:        n.Status,
			RelatedTagIDs: tagIDs,
			ParentIDs:     parentIDs,
		})
	}

	go FeedbackNarrativesToTagsWithBoard(feedbackOutputs)
}

func cleanEmptyBoards(date time.Time, categoryID *uint) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	subQuery := database.DB.Model(&models.NarrativeSummary{}).
		Select("DISTINCT board_id").
		Where("board_id IS NOT NULL AND period_date >= ? AND period_date < ?", startOfDay, endOfDay)

	boardQuery := database.DB.Where("period_date >= ? AND period_date < ?", startOfDay, endOfDay).
		Where("id NOT IN (?)", subQuery)
	if categoryID != nil {
		boardQuery = boardQuery.Where("scope_category_id = ?", *categoryID)
	}

	result := boardQuery.Delete(&models.NarrativeBoard{})
	if result.Error != nil {
		logging.Warnf("narrative: cleanEmptyBoards failed for %s: %v", date.Format("2006-01-02"), result.Error)
		return
	}
	if result.RowsAffected > 0 {
		logging.Infof("narrative: cleaned %d empty boards for %s", result.RowsAffected, date.Format("2006-01-02"))
	}
}

// Deprecated: Use GenerateAndSaveForAllBoards instead. Kept for rollback safety.
func (s *NarrativeService) GenerateAndSaveForAllCategories(date time.Time) (int, error) {
	categories, err := CollectActiveCategories(date)
	if err != nil {
		return 0, fmt.Errorf("collect active categories: %w", err)
	}
	if len(categories) == 0 {
		logging.Infof("narrative: no active categories for %s", date.Format("2006-01-02"))
		return 0, nil
	}

	totalSaved := 0
	for _, cat := range categories {
		saved, err := s.GenerateAndSaveForCategory(date, cat.ID, cat.Name)
		if err != nil {
			logging.Warnf("narrative: failed to generate for category %d (%s): %v", cat.ID, cat.Name, err)
			continue
		}
		totalSaved += saved
	}

	logging.Infof("narrative: saved %d category narratives across %d categories for %s", totalSaved, len(categories), date.Format("2006-01-02"))
	return totalSaved, nil
}

func resolveGeneration(out NarrativeOutput, date time.Time) int {
	if len(out.ParentIDs) == 0 {
		return 0
	}

	var prevNarratives []models.NarrativeSummary
	database.DB.Where("id IN ?", out.ParentIDs).Find(&prevNarratives)

	maxGen := -1
	for _, n := range prevNarratives {
		if n.Generation > maxGen {
			maxGen = n.Generation
		}
	}
	if maxGen < 0 {
		return 0
	}
	return maxGen + 1
}

func resolveGlobalGeneration(date time.Time) int {
	yesterday := date.AddDate(0, 0, -1)
	startOfYesterday := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfYesterday := startOfYesterday.Add(24 * time.Hour)

	var maxGen int
	database.DB.Model(&models.NarrativeSummary{}).
		Where("scope_type = ? AND period = ? AND period_date >= ? AND period_date < ?",
			models.NarrativeScopeTypeGlobal, "daily", startOfYesterday, endOfYesterday).
		Select("COALESCE(MAX(generation), -1)").
		Scan(&maxGen)

	if maxGen < 0 {
		return 0
	}
	return maxGen + 1
}

func resolveArticleIDsForScope(tagIDs []uint, date time.Time, scopeOpts *ScopeSaveOpts) []uint64 {
	if len(tagIDs) == 0 {
		return nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var articleIDs []uint64
	query := database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.article_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagIDs, startOfDay, endOfDay)

	if scopeOpts != nil && scopeOpts.ScopeType == models.NarrativeScopeTypeFeedCategory && scopeOpts.CategoryID != nil {
		query = query.Joins("JOIN feeds ON feeds.id = articles.feed_id").Where("feeds.category_id = ?", *scopeOpts.CategoryID)
	}

	if err := query.Pluck("article_topic_tags.article_id", &articleIDs).Error; err != nil {
		logging.Warnf("narrative: resolveArticleIDs failed: %v", err)
	}

	return articleIDs
}

type NarrativeListItem struct {
	ID          uint64     `json:"id"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Period      string     `json:"period"`
	PeriodDate  string     `json:"period_date"`
	Generation  int        `json:"generation"`
	ParentIDs   []uint64   `json:"parent_ids"`
	RelatedTags []TagBrief `json:"related_tags"`
	ChildIDs    []uint64   `json:"child_ids"`
	BoardID     *uint      `json:"board_id,omitempty"`
}

type TagBrief struct {
	ID       uint   `json:"id"`
	Slug     string `json:"slug"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Kind     string `json:"kind,omitempty"`
}

func resolveTagIDsToBriefs(tagIDsJSON string) []TagBrief {
	if tagIDsJSON == "" || tagIDsJSON == "[]" {
		return []TagBrief{}
	}
	var ids []uint
	if err := json.Unmarshal([]byte(tagIDsJSON), &ids); err != nil || len(ids) == 0 {
		return []TagBrief{}
	}
	var tags []models.TopicTag
	database.DB.Where("id IN ?", ids).Find(&tags)
	tagMap := make(map[uint]models.TopicTag, len(tags))
	for _, t := range tags {
		tagMap[t.ID] = t
	}
	result := make([]TagBrief, 0, len(ids))
	for _, id := range ids {
		if t, ok := tagMap[id]; ok {
			result = append(result, TagBrief{
				ID:       t.ID,
				Slug:     t.Slug,
				Label:    t.Label,
				Category: t.Category,
				Kind:     t.Kind,
			})
		}
	}
	return result
}

type TimelineDay struct {
	Date       string                `json:"date"`
	Narratives []NarrativeListItem   `json:"narratives"`
	Boards     []BoardNarrativeGroup `json:"boards,omitempty"`
}

type BoardNarrativeGroup struct {
	ID          uint                `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Status      string              `json:"status"`
	Narratives  []NarrativeListItem `json:"narratives"`
}

type BoardSummaryItem struct {
	ID              uint                `json:"id"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	NarrativeCount  int                 `json:"narrative_count"`
	AggregateStatus string              `json:"aggregate_status"`
	ScopeType       string              `json:"scope_type"`
	ScopeCategoryID *uint               `json:"scope_category_id,omitempty"`
	Narratives      []NarrativeListItem `json:"narratives"`
	PrevBoardIDs    []uint              `json:"prev_board_ids"`
	IsSystem        bool                `json:"is_system"`
	CreatedAt       string              `json:"created_at"`
	EventTags       []TagBrief          `json:"event_tags"`
}

type BoardTimelineDay struct {
	Date   string             `json:"date"`
	Boards []BoardSummaryItem `json:"boards"`
}

type BoardDetailResponse struct {
	Board      models.NarrativeBoard `json:"board"`
	Narratives []NarrativeListItem   `json:"narratives"`
	EventTags  []TagBrief            `json:"event_tags"`
}

func (s *NarrativeService) GetTimeline(anchorDate time.Time, days int, scopeType string, categoryID *uint) ([]TimelineDay, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	startOfAnchor := time.Date(anchorDate.Year(), anchorDate.Month(), anchorDate.Day(), 0, 0, 0, 0, anchorDate.Location())
	rangeStart := startOfAnchor.AddDate(0, 0, -(days - 1))
	rangeEnd := startOfAnchor.Add(24 * time.Hour)

	query := database.DB.
		Where("period = ? AND period_date >= ? AND period_date < ?", "daily", rangeStart, rangeEnd)

	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			query = query.Where("scope_category_id = ?", *categoryID)
		}
	} else {
		query = query.Where("scope_type = ?", models.NarrativeScopeTypeGlobal)
	}

	var narratives []models.NarrativeSummary
	if err := query.
		Order("period_date ASC, generation ASC, id ASC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("query narrative timeline: %w", err)
	}

	grouped := make(map[string][]models.NarrativeSummary)
	for _, n := range narratives {
		key := n.PeriodDate.Format("2006-01-02")
		grouped[key] = append(grouped[key], n)
	}

	allItems := toListItems(narratives)
	itemByID := make(map[uint64]NarrativeListItem)
	for _, item := range allItems {
		itemByID[item.ID] = item
	}

	var boardsInRange []models.NarrativeBoard
	boardQuery := database.DB.Where("period_date >= ? AND period_date < ?", rangeStart, rangeEnd)
	if scopeType != "" {
		boardQuery = boardQuery.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			boardQuery = boardQuery.Where("scope_category_id = ?", *categoryID)
		}
	} else {
		boardQuery = boardQuery.Where("scope_type = ?", models.NarrativeScopeTypeGlobal)
	}
	boardQuery.Order("id ASC").Find(&boardsInRange)

	boardsByDate := make(map[string][]models.NarrativeBoard)
	for _, b := range boardsInRange {
		key := b.PeriodDate.Format("2006-01-02")
		boardsByDate[key] = append(boardsByDate[key], b)
	}

	var result []TimelineDay
	for d := rangeStart; d.Before(rangeEnd); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		dayItems := make([]NarrativeListItem, 0)
		if ns, ok := grouped[key]; ok {
			for _, n := range ns {
				if item, found := itemByID[n.ID]; found {
					dayItems = append(dayItems, item)
				}
			}
		}

		var boardGroups []BoardNarrativeGroup
		if dayBoards, ok := boardsByDate[key]; ok {
			for _, b := range dayBoards {
				var boardNarItems = make([]NarrativeListItem, 0)
				statusMap := make(map[string]int)
				for _, item := range dayItems {
					if item.BoardID != nil && *item.BoardID == b.ID {
						boardNarItems = append(boardNarItems, item)
						statusMap[item.Status]++
					}
				}
				boardGroups = append(boardGroups, BoardNarrativeGroup{
					ID:          b.ID,
					Name:        b.Name,
					Description: b.Description,
					Status:      aggregateBoardStatus(statusMap),
					Narratives:  boardNarItems,
				})
			}
		}

		if len(boardGroups) > 0 {
			var ungrouped []NarrativeListItem
			for _, item := range dayItems {
				if item.BoardID == nil {
					ungrouped = append(ungrouped, item)
				}
			}
			dayItems = ungrouped
		}

		day := TimelineDay{
			Date:       key,
			Narratives: dayItems,
		}
		if len(boardGroups) > 0 {
			day.Boards = boardGroups
		}

		result = append(result, day)
	}

	return result, nil
}

func (s *NarrativeService) GetByDate(date time.Time, scopeType string, categoryID *uint) ([]NarrativeListItem, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := database.DB.
		Where("period = ? AND period_date >= ? AND period_date < ?", "daily", startOfDay, endOfDay)

	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			query = query.Where("scope_category_id = ?", *categoryID)
		}
	} else {
		query = query.Where("scope_type = ?", models.NarrativeScopeTypeGlobal)
	}

	var narratives []models.NarrativeSummary
	if err := query.
		Order("generation ASC, id ASC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("query narratives by date: %w", err)
	}

	return toListItems(narratives), nil
}

func (s *NarrativeService) GetByBoardID(boardID uint) ([]NarrativeListItem, error) {
	var narratives []models.NarrativeSummary
	if err := database.DB.Where("board_id = ?", boardID).
		Order("source ASC, generation ASC, id ASC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("query narratives for board %d: %w", boardID, err)
	}

	return toListItems(narratives), nil
}

func aggregateBoardStatus(statusMap map[string]int) string {
	if len(statusMap) == 0 {
		return ""
	}
	if statusMap[models.NarrativeStatusEmerging] > 0 {
		return models.NarrativeStatusEmerging
	}
	if statusMap[models.NarrativeStatusContinuing] > 0 {
		return models.NarrativeStatusContinuing
	}
	if statusMap[models.NarrativeStatusSplitting] > 0 {
		return models.NarrativeStatusSplitting
	}
	if statusMap[models.NarrativeStatusMerging] > 0 {
		return models.NarrativeStatusMerging
	}
	return models.NarrativeStatusEnding
}

func (s *NarrativeService) GetBoardTimeline(startDate, endDate time.Time, scopeType string, categoryID *uint) ([]BoardTimelineDay, error) {
	query := database.DB.Where("period_date >= ? AND period_date < ?", startDate, endDate)

	if scopeType != "" && scopeType != "all" {
		if categoryID != nil {
			query = query.Where("scope_category_id = ?", *categoryID)
		} else {
			query = query.Where("scope_type = ?", scopeType)
		}
	}

	var boards []models.NarrativeBoard
	if err := query.Order("period_date ASC, id ASC").Find(&boards).Error; err != nil {
		return nil, fmt.Errorf("query board timeline: %w", err)
	}

	if len(boards) == 0 {
		return []BoardTimelineDay{}, nil
	}

	boardIDs := make([]uint, 0, len(boards))
	for _, b := range boards {
		boardIDs = append(boardIDs, b.ID)
	}

	var narratives []models.NarrativeSummary
	database.DB.Where("board_id IN ?", boardIDs).
		Order("source ASC, generation ASC, id ASC").
		Find(&narratives)

	narrativeItems := toListItems(narratives)
	if narrativeItems == nil {
		narrativeItems = []NarrativeListItem{}
	}

	narrativesByBoard := make(map[uint][]NarrativeListItem)
	for _, item := range narrativeItems {
		if item.BoardID != nil {
			narrativesByBoard[*item.BoardID] = append(narrativesByBoard[*item.BoardID], item)
		}
	}

	boardStatuses := make(map[uint]map[string]int)
	for _, item := range narrativeItems {
		if item.BoardID != nil {
			if boardStatuses[*item.BoardID] == nil {
				boardStatuses[*item.BoardID] = make(map[string]int)
			}
			boardStatuses[*item.BoardID][item.Status]++
		}
	}

	grouped := make(map[string][]models.NarrativeBoard)
	for _, b := range boards {
		key := b.PeriodDate.In(time.Local).Format("2006-01-02")
		grouped[key] = append(grouped[key], b)
	}

	var result []BoardTimelineDay
	for d := startDate; d.Before(endDate); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		day := BoardTimelineDay{Date: key}
		if bs, ok := grouped[key]; ok {
			for _, b := range bs {
				var prevBoardIDs []uint
				if b.PrevBoardIDs != "" {
					_ = json.Unmarshal([]byte(b.PrevBoardIDs), &prevBoardIDs)
				}
				if prevBoardIDs == nil {
					prevBoardIDs = []uint{}
				}

				boardNarItems := narrativesByBoard[b.ID]
				if boardNarItems == nil {
					boardNarItems = []NarrativeListItem{}
				}

				day.Boards = append(day.Boards, BoardSummaryItem{
					ID:              b.ID,
					Name:            b.Name,
					Description:     b.Description,
					NarrativeCount:  len(boardNarItems),
					AggregateStatus: aggregateBoardStatus(boardStatuses[b.ID]),
					ScopeType:       b.ScopeType,
					ScopeCategoryID: b.ScopeCategoryID,
					Narratives:      boardNarItems,
					PrevBoardIDs:    prevBoardIDs,
					IsSystem:        b.IsSystem,
					CreatedAt:       b.CreatedAt.Format("2006-01-02T15:04:05Z"),
					EventTags:       resolveTagIDsToBriefs(b.EventTagIDs),
				})
			}
		}
		result = append(result, day)
	}

	return result, nil
}

func (s *NarrativeService) GetBoardDetail(boardID uint) (*BoardDetailResponse, error) {
	var board models.NarrativeBoard
	if err := database.DB.Where("id = ?", boardID).First(&board).Error; err != nil {
		return nil, fmt.Errorf("board %d not found: %w", boardID, err)
	}

	var narratives []models.NarrativeSummary
	if err := database.DB.Where("board_id = ?", boardID).
		Order("source ASC, generation ASC, id ASC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("query narratives for board %d: %w", boardID, err)
	}

	return &BoardDetailResponse{
		Board:      board,
		Narratives: toListItems(narratives),
		EventTags:  resolveTagIDsToBriefs(board.EventTagIDs),
	}, nil
}

type NarrativeScopeItem struct {
	CategoryID      uint   `json:"category_id"`
	CategoryName    string `json:"category_name"`
	CategoryIcon    string `json:"category_icon"`
	CategoryColor   string `json:"category_color"`
	BoardCount      int    `json:"board_count"`
	LastGeneratedAt string `json:"last_generated_at"`
}

type NarrativeScopesResponse struct {
	Date        string               `json:"date"`
	GlobalCount int                  `json:"global_count"`
	Categories  []NarrativeScopeItem `json:"categories"`
}

func (s *NarrativeService) GetScopes(date time.Time, days int) (*NarrativeScopesResponse, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	startOfAnchor := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	rangeStart := startOfAnchor.AddDate(0, 0, -(days - 1))
	rangeEnd := startOfAnchor.Add(24 * time.Hour)

	var globalCount int64
	database.DB.Model(&models.NarrativeBoard{}).
		Where("scope_type = ? AND period_date >= ? AND period_date < ?", models.NarrativeScopeTypeGlobal, rangeStart, rangeEnd).
		Count(&globalCount)

	type catRow struct {
		ScopeCategoryID uint   `json:"scope_category_id"`
		ScopeLabel      string `json:"scope_label"`
		Cnt             int    `json:"cnt"`
	}
	var catRows []catRow
	database.DB.Model(&models.NarrativeBoard{}).
		Select("scope_category_id, scope_label, COUNT(*) as cnt").
		Where("scope_type = ? AND period_date >= ? AND period_date < ?", models.NarrativeScopeTypeFeedCategory, rangeStart, rangeEnd).
		Group("scope_category_id, scope_label").
		Scan(&catRows)

	var items []NarrativeScopeItem
	if len(catRows) > 0 {
		catIDs := make([]uint, 0, len(catRows))
		for _, row := range catRows {
			catIDs = append(catIDs, row.ScopeCategoryID)
		}
		var categories []models.Category
		database.DB.Where("id IN ?", catIDs).Find(&categories)
		catMap := make(map[uint]models.Category, len(categories))
		for _, c := range categories {
			catMap[c.ID] = c
		}

		type lastRow struct {
			ScopeCategoryID uint   `json:"scope_category_id"`
			CreatedAt       string `json:"created_at"`
		}
		var lastRows []lastRow
		database.DB.Model(&models.NarrativeBoard{}).
			Select("scope_category_id, MAX(created_at) as created_at").
			Where("scope_type = ? AND scope_category_id IN ? AND period_date >= ? AND period_date < ?",
				models.NarrativeScopeTypeFeedCategory, catIDs, rangeStart, rangeEnd).
			Group("scope_category_id").
			Scan(&lastRows)
		lastMap := make(map[uint]string, len(lastRows))
		for _, lr := range lastRows {
			lastMap[lr.ScopeCategoryID] = lr.CreatedAt
		}

		for _, row := range catRows {
			cat, ok := catMap[row.ScopeCategoryID]
			if ok {
				items = append(items, NarrativeScopeItem{
					CategoryID:      cat.ID,
					CategoryName:    cat.Name,
					CategoryIcon:    cat.Icon,
					CategoryColor:   cat.Color,
					BoardCount:      row.Cnt,
					LastGeneratedAt: lastMap[cat.ID],
				})
			} else {
				items = append(items, NarrativeScopeItem{
					CategoryID:      row.ScopeCategoryID,
					CategoryName:    row.ScopeLabel,
					BoardCount:      row.Cnt,
					LastGeneratedAt: lastMap[row.ScopeCategoryID],
				})
			}
		}
	}

	return &NarrativeScopesResponse{
		Date:        startOfAnchor.Format("2006-01-02"),
		GlobalCount: int(globalCount),
		Categories:  items,
	}, nil
}

func (s *NarrativeService) GetNarrativeTree(narrativeID uint64) (*NarrativeListItem, error) {
	var narrative models.NarrativeSummary
	if err := database.DB.Where("id = ?", narrativeID).First(&narrative).Error; err != nil {
		return nil, fmt.Errorf("query narrative %d: %w", narrativeID, err)
	}

	items := toListItems([]models.NarrativeSummary{narrative})
	if len(items) == 0 {
		return nil, fmt.Errorf("failed to build list item for narrative %d", narrativeID)
	}
	return &items[0], nil
}

func (s *NarrativeService) GetNarrativeHistory(narrativeID uint64) ([]NarrativeListItem, error) {
	var narrative models.NarrativeSummary
	if err := database.DB.Where("id = ?", narrativeID).First(&narrative).Error; err != nil {
		return nil, fmt.Errorf("query narrative %d: %w", narrativeID, err)
	}

	var history []models.NarrativeSummary
	visited := make(map[uint64]bool)
	walkHistory(narrativeID, &history, visited)

	return toListItems(history), nil
}

func walkHistory(id uint64, history *[]models.NarrativeSummary, visited map[uint64]bool) {
	walkHistoryDepth(id, history, visited, 0, 30)
}

func walkHistoryDepth(id uint64, history *[]models.NarrativeSummary, visited map[uint64]bool, depth, maxDepth int) {
	if depth > maxDepth || visited[id] {
		return
	}
	visited[id] = true

	var narrative models.NarrativeSummary
	if err := database.DB.Where("id = ?", id).First(&narrative).Error; err != nil {
		return
	}

	var parentIDs []uint64
	if narrative.ParentIDs != "" {
		_ = json.Unmarshal([]byte(narrative.ParentIDs), &parentIDs)
	}

	for _, pid := range parentIDs {
		walkHistoryDepth(pid, history, visited, depth+1, maxDepth)
	}

	*history = append(*history, narrative)
}

func toListItems(narratives []models.NarrativeSummary) []NarrativeListItem {
	if len(narratives) == 0 {
		return nil
	}

	tagIDSet := make(map[uint]bool)
	for _, n := range narratives {
		var tagIDs []uint
		if n.RelatedTagIDs != "" {
			_ = json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
		}
		for _, id := range tagIDs {
			tagIDSet[id] = true
		}
	}

	tagBriefMap := make(map[uint]TagBrief)
	if len(tagIDSet) > 0 {
		tagIDs := make([]uint, 0, len(tagIDSet))
		for id := range tagIDSet {
			tagIDs = append(tagIDs, id)
		}
		var tags []models.TopicTag
		database.DB.Where("id IN ?", tagIDs).Find(&tags)
		for _, t := range tags {
			tagBriefMap[t.ID] = TagBrief{ID: t.ID, Slug: t.Slug, Label: t.Label, Category: t.Category, Kind: t.Kind}
		}
	}

	childMap := make(map[uint64][]uint64)
	for _, n := range narratives {
		var parentIDs []uint64
		if n.ParentIDs != "" {
			_ = json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		}
		for _, pid := range parentIDs {
			childMap[pid] = append(childMap[pid], n.ID)
		}
	}

	items := make([]NarrativeListItem, 0, len(narratives))
	for _, n := range narratives {
		var parentIDs []uint64
		if n.ParentIDs != "" {
			_ = json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		}

		var tagIDs []uint
		if n.RelatedTagIDs != "" {
			_ = json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
		}

		tagBriefs := make([]TagBrief, 0, len(tagIDs))
		for _, tid := range tagIDs {
			if brief, ok := tagBriefMap[tid]; ok {
				tagBriefs = append(tagBriefs, brief)
			}
		}

		childIDs := childMap[n.ID]
		if childIDs == nil {
			childIDs = []uint64{}
		}

		items = append(items, NarrativeListItem{
			ID:          n.ID,
			Title:       n.Title,
			Summary:     n.Summary,
			Status:      n.Status,
			Source:      n.Source,
			Period:      n.Period,
			PeriodDate:  n.PeriodDate.Format("2006-01-02"),
			Generation:  n.Generation,
			ParentIDs:   parentIDs,
			RelatedTags: tagBriefs,
			ChildIDs:    childIDs,
			BoardID:     n.BoardID,
		})
	}

	return items
}
