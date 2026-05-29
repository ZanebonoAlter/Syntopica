package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

const boardNarrativeSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了一个叙事看板（board）的上下文信息，包括 SemanticBoard 标签和描述、该看板下的事件标签，以及前一天该看板相关叙事的延续信息。

你的任务是基于这个看板的上下文，生成叙事线索。每条叙事应该：
1. 有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句，不能是纯名词）
2. 有一段客观的摘要描述（中文，200-400字，包含关键事实和发展脉络）
3. 有一个状态标签：emerging（新出现）、continuing（持续发展）、splitting（分化）、merging（合并）、ending（趋于结束）
4. 关联到相关的标签ID
5. 如果是从已有叙事延续而来，标明父叙事ID
6. 给出置信度分数（0-1）
7. 按因果、影响、主题关联分组，不要按语义相似度归类
8. 不要为了凑数而强行合并不相关的标签
9. 数量不固定，有几条写几条，没有就返回空数组

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：narratives
2. narratives 必须是 JSON 数组；没有符合条件的叙事时，返回 {"narratives":[]}
3. narratives 数组中的每个元素都必须包含 title、summary、status、related_tag_ids、parent_ids、confidence_score 字段
4. status 只能是 emerging、continuing、splitting、merging、ending 之一
5. related_tag_ids 和 parent_ids 必须始终输出数组，即使为空也要输出 []
6. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`

type BoardNarrativeContext struct {
	Board              models.NarrativeBoard
	EventTags          []TagInput
	PrevNarratives     []PreviousNarrative
	SemanticBoardLabel string
	SemanticBoardDesc  string
	ConceptName        string
	ConceptDescription string
}

func buildBoardNarrativePrompt(ctx BoardNarrativeContext) string {
	var sb strings.Builder

	sb.WriteString("## 看板上下文\n\n")
	boardName := ctx.Board.Name
	if ctx.SemanticBoardLabel != "" {
		boardName = ctx.SemanticBoardLabel
	}
	boardDescription := ctx.Board.Description
	if ctx.SemanticBoardDesc != "" {
		boardDescription = ctx.SemanticBoardDesc
	}

	fmt.Fprintf(&sb, "- SemanticBoard: %s\n", boardName)
	fmt.Fprintf(&sb, "- SemanticBoard 描述: %s\n", boardDescription)

	if ctx.ConceptName != "" {
		fmt.Fprintf(&sb, "- 板块概念: %s\n", ctx.ConceptName)
		if ctx.ConceptDescription != "" {
			fmt.Fprintf(&sb, "- 概念描述: %s\n", ctx.ConceptDescription)
		}
	}

	sb.WriteString("\n## 看板事件标签\n\n")
	for _, t := range ctx.EventTags {
		fmt.Fprintf(&sb, "- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}

	if len(ctx.PrevNarratives) > 0 {
		sb.WriteString("\n## 昨日该看板相关叙事（供延续/对比参考）\n\n")
		for _, p := range ctx.PrevNarratives {
			fmt.Fprintf(&sb, "- [ID:%d] %s (状态:%s, 第%d代)\n  摘要: %s\n",
				p.ID, p.Title, p.Status, p.Generation, p.Summary)
		}
	}

	sb.WriteString("\n请基于以上看板上下文，生成本看板的叙事线索。注意发现标签之间的关联，识别新兴趋势，标注与昨日叙事的延续关系。\n")
	return sb.String()
}

func GenerateNarrativesForBoard(ctx context.Context, boardCtx BoardNarrativeContext) ([]NarrativeOutput, error) {
	if len(boardCtx.EventTags) == 0 {
		return nil, nil
	}

	prompt := buildBoardNarrativePrompt(boardCtx)

	temperature := 0.4
	maxTokens := 6000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: boardNarrativeSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"narratives": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":            {Type: "string", Description: "叙事标题，带判断的短句，不超过30字"},
							"summary":          {Type: "string", Description: "叙事摘要，200-400字"},
							"status":           {Type: "string", Description: "emerging/continuing/splitting/merging/ending"},
							"related_tag_ids":  {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"parent_ids":       {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence_score": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "status", "related_tag_ids", "parent_ids"},
					},
				},
			},
			Required: []string{"narratives"},
		},
		Metadata: map[string]any{
			"operation":            "board_narrative_generation",
			"board_id":             boardCtx.Board.ID,
			"board_name":           boardCtx.Board.Name,
			"event_tag_count":      len(boardCtx.EventTags),
			"prev_narrative_count": len(boardCtx.PrevNarratives),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("board narrative AI call failed for board %d: %w", boardCtx.Board.ID, err)
	}

	logging.Infof("board-narrative: raw LLM response for board %d, length=%d, first_500=%s",
		boardCtx.Board.ID, len(result.Content), truncateStr(result.Content, 500))

	outputs, err := parseNarrativeResponse(result.Content)
	if err != nil {
		logging.Warnf("board-narrative: parse error for board %d: %v, last_300=%s",
			boardCtx.Board.ID, err, truncateStr(result.Content, 300))
		return nil, fmt.Errorf("parse board narrative response for board %d: %w", boardCtx.Board.ID, err)
	}

	outputs = validateNarrativeOutputs(outputs, boardCtx.EventTags, boardCtx.PrevNarratives)

	logging.Infof("board-narrative: generated %d narratives for board %d (%s)",
		len(outputs), boardCtx.Board.ID, boardCtx.Board.Name)
	return outputs, nil
}

// Deprecated: Use saveNarrativesWithBoard instead. Kept for rollback safety.
func SaveNarrativesForBoard(outputs []NarrativeOutput, board models.NarrativeBoard, date time.Time, categoryID uint) (int, error) {
	if len(outputs) == 0 {
		return 0, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	boardID := board.ID
	catID := categoryID
	scopeOpts := &ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeFeedCategory, CategoryID: &catID}
	records := make([]models.NarrativeSummary, 0, len(outputs))
	for _, out := range outputs {
		parentIDsJSON, _ := json.Marshal(out.ParentIDs)
		tagIDsJSON, _ := json.Marshal(out.RelatedTagIDs)
		articleIDs := resolveArticleIDsForScope(out.RelatedTagIDs, date, scopeOpts)
		articleIDsJSON, _ := json.Marshal(articleIDs)

		generation := resolveGeneration(out, date)

		status := out.Status
		if status == "" {
			status = models.NarrativeStatusEmerging
		}

		record := models.NarrativeSummary{
			Title:             out.Title,
			Summary:           out.Summary,
			Status:            status,
			Period:            "daily",
			PeriodDate:        startOfDay,
			Generation:        generation,
			ParentIDs:         string(parentIDsJSON),
			RelatedTagIDs:     string(tagIDsJSON),
			RelatedArticleIDs: string(articleIDsJSON),
			Source:            "ai",
			ScopeType:         models.NarrativeScopeTypeFeedCategory,
			ScopeCategoryID:   &catID,
			BoardID:           &boardID,
		}
		records = append(records, record)
	}

	if err := database.DB.CreateInBatches(records, 20).Error; err != nil {
		logging.Warnf("board-narrative: batch save failed for board %d: %v, falling back to individual saves", boardID, err)
		saved := 0
		for _, record := range records {
			if err := database.DB.Create(&record).Error; err != nil {
				logging.Warnf("board-narrative: failed to save '%s': %v", record.Title, err)
				continue
			}
			saved++
		}
		return saved, nil
	}

	logging.Infof("board-narrative: saved %d narratives for board %d (%s)",
		len(records), boardID, board.Name)
	return len(records), nil
}

func saveNarrativesWithBoard(outputs []NarrativeOutput, board models.NarrativeBoard, date time.Time, scopeOpts *ScopeSaveOpts) (int, error) {
	if len(outputs) == 0 {
		return 0, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	boardID := board.ID

	records := make([]models.NarrativeSummary, 0, len(outputs))
	for _, out := range outputs {
		parentIDsJSON, _ := json.Marshal(out.ParentIDs)
		tagIDsJSON, _ := json.Marshal(out.RelatedTagIDs)
		articleIDs := resolveArticleIDsForScope(out.RelatedTagIDs, date, scopeOpts)
		articleIDsJSON, _ := json.Marshal(articleIDs)

		generation := 0
		if scopeOpts != nil && scopeOpts.ScopeType == models.NarrativeScopeTypeGlobal {
			generation = resolveGlobalGeneration(date)
		} else {
			generation = resolveGeneration(out, date)
		}

		status := out.Status
		if status == "" {
			status = models.NarrativeStatusEmerging
		}

		record := models.NarrativeSummary{
			Title:             out.Title,
			Summary:           out.Summary,
			Status:            status,
			Period:            "daily",
			PeriodDate:        startOfDay,
			Generation:        generation,
			ParentIDs:         string(parentIDsJSON),
			RelatedTagIDs:     string(tagIDsJSON),
			RelatedArticleIDs: string(articleIDsJSON),
			Source:            "ai",
			BoardID:           &boardID,
			ScopeType:         models.NarrativeScopeTypeGlobal,
		}
		if scopeOpts != nil {
			record.ScopeType = scopeOpts.ScopeType
			record.ScopeCategoryID = scopeOpts.CategoryID
			record.ScopeLabel = scopeOpts.Label
		}
		records = append(records, record)
	}

	if err := database.DB.CreateInBatches(records, 20).Error; err != nil {
		logging.Warnf("board-narrative: batch save failed for board %d: %v", boardID, err)
		saved := 0
		for _, record := range records {
			if err := database.DB.Create(&record).Error; err != nil {
				logging.Warnf("board-narrative: failed to save '%s': %v", record.Title, err)
				continue
			}
			saved++
		}
		return saved, nil
	}

	logging.Infof("board-narrative: saved %d narratives for board %d (%s)",
		len(records), boardID, board.Name)
	return len(records), nil
}

func LoadBoardEventTags(board models.NarrativeBoard) ([]TagInput, error) {
	var eventTagIDs []uint
	if board.EventTagIDs != "" {
		if err := json.Unmarshal([]byte(board.EventTagIDs), &eventTagIDs); err != nil {
			return nil, fmt.Errorf("parse event_tag_ids for board %d: %w", board.ID, err)
		}
	}
	if len(eventTagIDs) == 0 {
		return nil, nil
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ?", eventTagIDs, "active").Find(&tags)
	tagMap := make(map[uint]models.TopicTag, len(tags))
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	tagIDs := make([]uint, 0, len(tags))
	for _, t := range tags {
		tagIDs = append(tagIDs, t.ID)
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	if len(tagIDs) > 0 {
		database.DB.Model(&models.ArticleTopicTag{}).
			Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("article_topic_tags.topic_tag_id IN ?", tagIDs).
			Group("article_topic_tags.topic_tag_id").
			Scan(&counts)
	}

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, id := range eventTagIDs {
		tag, ok := tagMap[id]
		if !ok {
			continue
		}
		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			Source:       tag.Source,
		})
	}
	return inputs, nil
}
