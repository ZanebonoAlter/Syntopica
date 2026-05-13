package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
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
	ClearUnclassifiedBucket()

	deleted, err := s.DeleteByDate(date, "", nil)
	if err != nil {
		return 0, err
	}
	logging.Infof("narrative: deleted %d old narratives before regenerating for %s", deleted, date.Format("2006-01-02"))

	return s.GenerateAndSave(date)
}

func (s *NarrativeService) RegenerateAndSaveForCategory(date time.Time, categoryID uint) (int, error) {
	ClearUnclassifiedBucket()

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
	catSaved, err := s.GenerateAndSaveForAllCategories(date)
	if err != nil {
		logging.Warnf("narrative: category generation had errors: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	saved, gErr := s.GenerateAndSaveGlobal(ctx, date)
	if gErr != nil {
		logging.Warnf("narrative: global generation failed: %v", gErr)
	}

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

	totalSaved := catSaved + saved
	logging.Infof("narrative: GenerateAndSave complete — %d narratives saved for %s",
		totalSaved, date.Format("2006-01-02"))
	return totalSaved, nil
}

func (s *NarrativeService) GenerateAndSaveGlobal(ctx context.Context, date time.Time) (int, error) {
	tagInputs, err := CollectTagInputs(date)
	if err != nil {
		return 0, fmt.Errorf("collect global tag inputs: %w", err)
	}

	if len(tagInputs) == 0 {
		return 0, nil
	}

	var globalBoards []models.NarrativeBoard

	matchingTagsByConcept := make(map[uint][]TagInput)
	for _, tag := range tagInputs {
		conceptMatch, mErr := MatchTagToConcept(ctx, TagInput{
			ID:          tag.ID,
			Label:       tag.Label,
			Description: tag.Description,
		})
		if mErr != nil {
			logging.Warnf("narrative: global match tag %d failed: %v", tag.ID, mErr)
			AddToUnclassifiedBucket(tag)
			continue
		}
		if conceptMatch != nil {
			matchingTagsByConcept[conceptMatch.ConceptID] = append(matchingTagsByConcept[conceptMatch.ConceptID], tag)
		} else {
			AddToUnclassifiedBucket(tag)
		}
	}

	for conceptID, tags := range matchingTagsByConcept {
		if len(tags) == 0 {
			continue
		}

		var concept models.BoardConcept
		if err := database.DB.Where("id = ?", conceptID).First(&concept).Error; err != nil {
			logging.Warnf("narrative: global concept %d not found", conceptID)
			for _, t := range tags {
				AddToUnclassifiedBucket(t)
			}
			continue
		}

		board, bErr := BuildBoardFromMatchedTags(conceptID, concept.Name, tags, date, nil)
		if bErr != nil {
			logging.Warnf("narrative: global create concept board failed for concept %d: %v", conceptID, bErr)
			continue
		}
		if board != nil {
			globalBoards = append(globalBoards, *board)
		}
	}

	totalSaved := 0
	for _, board := range globalBoards {
		eventTags, lErr := LoadBoardEventTags(board)
		if lErr != nil {
			continue
		}
		if len(eventTags) == 0 {
			continue
		}

		var prevBoardIDs []uint
		if board.PrevBoardIDs != "" {
			_ = json.Unmarshal([]byte(board.PrevBoardIDs), &prevBoardIDs)
		}

		var prevNarrs []PreviousNarrative
		if len(prevBoardIDs) > 0 {
			var prevSummaries []models.NarrativeSummary
			database.DB.Where("board_id IN ?", prevBoardIDs).Order("id ASC").Find(&prevSummaries)
			for _, ps := range prevSummaries {
				prevNarrs = append(prevNarrs, PreviousNarrative{
					ID:         uint64(ps.ID),
					Title:      ps.Title,
					Summary:    ps.Summary,
					Status:     ps.Status,
					Generation: ps.Generation,
				})
			}
		}

		boardCtx := BoardNarrativeContext{
			Board:          board,
			EventTags:      eventTags,
			PrevNarratives: prevNarrs,
		}

		if board.BoardConceptID != nil {
			var concept models.BoardConcept
			if err := database.DB.Where("id = ?", *board.BoardConceptID).First(&concept).Error; err == nil {
				boardCtx.ConceptName = concept.Name
				boardCtx.ConceptDescription = concept.Description
			}
		}

		outputs, gErr := GenerateNarrativesForBoard(ctx, boardCtx)
		if gErr != nil {
			continue
		}

		scopeOpts := &ScopeSaveOpts{
			ScopeType:  models.NarrativeScopeTypeGlobal,
			CategoryID: nil,
			Label:      "",
		}
		saved, sErr := saveNarrativesWithBoard(outputs, board, date, scopeOpts)
		if sErr != nil {
			continue
		}
		totalSaved += saved
	}

	logging.Infof("narrative: global generation saved %d narratives across %d concept boards",
		totalSaved, len(globalBoards))
	return totalSaved, nil
}

func (s *NarrativeService) GenerateAndSaveForCategory(date time.Time, categoryID uint, categoryLabel string) (int, error) {
	abstractTrees, err := CollectAbstractTreeInputsByCategory(date, categoryID)
	if err != nil {
		return 0, fmt.Errorf("collect abstract trees for category %d: %w", categoryID, err)
	}

	events, err := CollectUnclassifiedEventTagsByCategory(date, categoryID)
	if err != nil {
		return 0, fmt.Errorf("collect event tags for category %d: %w", categoryID, err)
	}

	if len(abstractTrees) == 0 && len(events) == 0 {
		logging.Infof("narrative: no abstract trees or event tags for category %d on %s, skipping", categoryID, date.Format("2006-01-02"))
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	hotspotThreshold := getHotspotThreshold()

	var hotspotTrees []AbstractTreeNode
	var matchingTrees []AbstractTreeNode

	for _, tree := range abstractTrees {
		nodeCount := CountTreeNodes(tree)
		if nodeCount >= hotspotThreshold {
			hotspotTrees = append(hotspotTrees, tree)
		} else {
			matchingTrees = append(matchingTrees, tree)
		}
	}

	var allBoards []models.NarrativeBoard

	for _, tree := range hotspotTrees {
		board, bErr := createBoardFromAbstractTree(tree, date, categoryID)
		if bErr != nil {
			logging.Warnf("narrative: failed to create hotspot board from abstract tree %d: %v", tree.ID, bErr)
			continue
		}
		if board != nil {
			allBoards = append(allBoards, *board)
		}
	}

	matchingTagsByConcept := make(map[uint][]TagInput)
	var unclassifiedForBucket []TagInput

	matcherCtx, matcherCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer matcherCancel()

	for _, tree := range matchingTrees {
		conceptMatch, mErr := MatchTagToConcept(matcherCtx, TagInput{
			ID:          tree.ID,
			Label:       tree.Label,
			Description: tree.Description,
		})
		if mErr != nil {
			logging.Warnf("narrative: match small tree %d to concept failed: %v", tree.ID, mErr)
			board, bErr := createBoardFromAbstractTree(tree, date, categoryID)
			if bErr == nil && board != nil {
				allBoards = append(allBoards, *board)
			}
			continue
		}
		if conceptMatch != nil {
			childTags := collectAllEventTags(tree)
			matchingTagsByConcept[conceptMatch.ConceptID] = append(matchingTagsByConcept[conceptMatch.ConceptID], childTags...)
			logging.Infof("narrative: small tree %s (%d) matched to concept %s (sim=%.3f)",
				tree.Label, tree.ID, conceptMatch.Name, conceptMatch.Similarity)
		} else {
			board, bErr := createBoardFromAbstractTree(tree, date, categoryID)
			if bErr == nil && board != nil {
				allBoards = append(allBoards, *board)
				logging.Infof("narrative: small tree %s (%d) no concept match, created standalone board %d",
					tree.Label, tree.ID, board.ID)
			} else {
				childTags := collectAllEventTags(tree)
				for _, t := range childTags {
					AddToUnclassifiedBucket(t)
				}
			}
		}
	}

	for _, tag := range events {
		conceptMatch, mErr := MatchTagToConcept(matcherCtx, TagInput{
			ID:          tag.ID,
			Label:       tag.Label,
			Description: tag.Description,
		})
		if mErr != nil {
			logging.Warnf("narrative: match event tag %d to concept failed: %v", tag.ID, mErr)
			unclassifiedForBucket = append(unclassifiedForBucket, tag)
			continue
		}
		if conceptMatch != nil {
			matchingTagsByConcept[conceptMatch.ConceptID] = append(matchingTagsByConcept[conceptMatch.ConceptID], tag)
		} else {
			unclassifiedForBucket = append(unclassifiedForBucket, tag)
		}
	}

	for _, tag := range unclassifiedForBucket {
		AddToUnclassifiedBucket(tag)
	}

	for conceptID, tags := range matchingTagsByConcept {
		if len(tags) == 0 {
			continue
		}

		var concept models.BoardConcept
		if err := database.DB.Where("id = ?", conceptID).First(&concept).Error; err != nil {
			logging.Warnf("narrative: concept %d not found, skipping %d tags", conceptID, len(tags))
			for _, t := range tags {
				AddToUnclassifiedBucket(t)
			}
			continue
		}

		board, bErr := BuildBoardFromMatchedTags(conceptID, concept.Name, tags, date, &categoryID)
		if bErr != nil {
			logging.Warnf("narrative: failed to create concept board for concept %d: %v", conceptID, bErr)
			for _, t := range tags {
				AddToUnclassifiedBucket(t)
			}
			continue
		}
		if board != nil {
			allBoards = append(allBoards, *board)
		}
	}

	if len(allBoards) == 0 {
		logging.Infof("narrative: no boards created for category %d on %s", categoryID, date.Format("2006-01-02"))
		return 0, nil
	}

	totalSaved := 0
	for _, board := range allBoards {
		eventTags, lErr := LoadBoardEventTags(board)
		if lErr != nil {
			logging.Warnf("narrative: failed to load event tags for board %d: %v", board.ID, lErr)
			continue
		}
		if len(eventTags) == 0 {
			continue
		}

		var prevBoardIDs []uint
		if board.PrevBoardIDs != "" {
			_ = json.Unmarshal([]byte(board.PrevBoardIDs), &prevBoardIDs)
		}

		var prevNarrs []PreviousNarrative
		if len(prevBoardIDs) > 0 {
			var prevSummaries []models.NarrativeSummary
			database.DB.Where("board_id IN ?", prevBoardIDs).Order("id ASC").Find(&prevSummaries)
			for _, ps := range prevSummaries {
				prevNarrs = append(prevNarrs, PreviousNarrative{
					ID:         uint64(ps.ID),
					Title:      ps.Title,
					Summary:    ps.Summary,
					Status:     ps.Status,
					Generation: ps.Generation,
				})
			}
		}

		boardCtx := BoardNarrativeContext{
			Board:          board,
			EventTags:      eventTags,
			PrevNarratives: prevNarrs,
		}

		if board.BoardConceptID != nil {
			var concept models.BoardConcept
			if err := database.DB.Where("id = ?", *board.BoardConceptID).First(&concept).Error; err == nil {
				boardCtx.ConceptName = concept.Name
				boardCtx.ConceptDescription = concept.Description
			}
		}

		outputs, gErr := GenerateNarrativesForBoard(ctx, boardCtx)
		if gErr != nil {
			logging.Warnf("narrative: failed to generate narratives for board %d: %v", board.ID, gErr)
			continue
		}

		saved, sErr := SaveNarrativesForBoard(outputs, board, date, categoryID)
		if sErr != nil {
			logging.Warnf("narrative: failed to save narratives for board %d: %v", board.ID, sErr)
			continue
		}
		totalSaved += saved
	}

	logging.Infof("narrative: saved %d narratives across %d boards for category %d (%s) on %s (hotspot=%d, concept=%d)",
		totalSaved, len(allBoards), categoryID, categoryLabel, date.Format("2006-01-02"),
		len(hotspotTrees), len(matchingTrees))

	cleanEmptyBoards(date, &categoryID)

	return totalSaved, nil
}

func collectAllEventTags(tree AbstractTreeNode) []TagInput {
	var result []TagInput
	if tree.Category == "event" {
		result = append(result, TagInput{
			ID:          tree.ID,
			Label:       tree.Label,
			Description: tree.Description,
		})
	}
	for _, child := range tree.Children {
		result = append(result, collectAllEventTags(child)...)
	}
	return result
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

func resolveArticleIDs(tagIDs []uint, date time.Time) []uint64 {
	if len(tagIDs) == 0 {
		return nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var articleIDs []uint64
	if err := database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.article_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagIDs, startOfDay, endOfDay).
		Pluck("article_topic_tags.article_id", &articleIDs).Error; err != nil {
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
	AbstractTagID   *uint               `json:"abstract_tag_id,omitempty"`
	AbstractTagSlug string              `json:"abstract_tag_slug,omitempty"`
	BoardConceptID  *uint               `json:"board_concept_id,omitempty"`
	ConceptName     string              `json:"concept_name,omitempty"`
	IsSystem        bool                `json:"is_system"`
	CreatedAt       string              `json:"created_at"`
	EventTags       []TagBrief          `json:"event_tags"`
	AbstractTags    []TagBrief          `json:"abstract_tags"`
}

type BoardTimelineDay struct {
	Date   string             `json:"date"`
	Boards []BoardSummaryItem `json:"boards"`
}

type BoardDetailResponse struct {
	Board        models.NarrativeBoard `json:"board"`
	Narratives   []NarrativeListItem   `json:"narratives"`
	EventTags    []TagBrief            `json:"event_tags"`
	AbstractTags []TagBrief            `json:"abstract_tags"`
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

	abstractTagIDs := make(map[uint]bool)
	for _, b := range boards {
		if b.AbstractTagID != nil {
			abstractTagIDs[*b.AbstractTagID] = true
		}
	}
	abstractTagMap := make(map[uint]models.TopicTag)
	if len(abstractTagIDs) > 0 {
		tagIDs := make([]uint, 0, len(abstractTagIDs))
		for id := range abstractTagIDs {
			tagIDs = append(tagIDs, id)
		}
		var tags []models.TopicTag
		database.DB.Where("id IN ?", tagIDs).Find(&tags)
		for _, t := range tags {
			abstractTagMap[t.ID] = t
		}
	}

	conceptIDs := make(map[uint]bool)
	for _, b := range boards {
		if b.BoardConceptID != nil {
			conceptIDs[*b.BoardConceptID] = true
		}
	}
	conceptMap := make(map[uint]models.BoardConcept)
	if len(conceptIDs) > 0 {
		cIDs := make([]uint, 0, len(conceptIDs))
		for id := range conceptIDs {
			cIDs = append(cIDs, id)
		}
		var concepts []models.BoardConcept
		database.DB.Where("id IN ?", cIDs).Find(&concepts)
		for _, c := range concepts {
			conceptMap[c.ID] = c
		}
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

				conceptName := ""
				if b.BoardConceptID != nil {
					if c, ok := conceptMap[*b.BoardConceptID]; ok {
						conceptName = c.Name
					}
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
					AbstractTagID:   b.AbstractTagID,
					BoardConceptID:  b.BoardConceptID,
					ConceptName:     conceptName,
					IsSystem:        b.IsSystem,
					AbstractTagSlug: func() string {
						if b.AbstractTagID != nil {
							if tag, ok := abstractTagMap[*b.AbstractTagID]; ok {
								return tag.Slug
							}
						}
						return ""
					}(),
					CreatedAt:    b.CreatedAt.Format("2006-01-02T15:04:05Z"),
					EventTags:    resolveTagIDsToBriefs(b.EventTagIDs),
					AbstractTags: resolveTagIDsToBriefs(b.AbstractTagIDs),
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
		Board:        board,
		Narratives:   toListItems(narratives),
		EventTags:    resolveTagIDsToBriefs(board.EventTagIDs),
		AbstractTags: resolveTagIDsToBriefs(board.AbstractTagIDs),
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
