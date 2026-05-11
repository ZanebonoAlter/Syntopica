package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

type CategoryBoardBrief struct {
	BoardID        uint   `json:"board_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	CategoryID     uint   `json:"category_id"`
	EventTagIDs    []uint `json:"event_tag_ids"`
	AbstractTagIDs []uint `json:"abstract_tag_ids"`
}

func CollectAllCategoryBoards(date time.Time) ([]CategoryBoardBrief, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var boards []models.NarrativeBoard
	if err := database.DB.
		Where("scope_type = ? AND period_date >= ? AND period_date < ?",
			models.NarrativeScopeTypeFeedCategory, startOfDay, endOfDay).
		Order("id ASC").
		Find(&boards).Error; err != nil {
		return nil, fmt.Errorf("collect all category boards for %s: %w", date.Format("2006-01-02"), err)
	}

	if len(boards) == 0 {
		return nil, nil
	}

	result := make([]CategoryBoardBrief, 0, len(boards))
	for _, b := range boards {
		var eventIDs []uint
		if b.EventTagIDs != "" {
			_ = json.Unmarshal([]byte(b.EventTagIDs), &eventIDs)
		}
		var abstractIDs []uint
		if b.AbstractTagIDs != "" {
			_ = json.Unmarshal([]byte(b.AbstractTagIDs), &abstractIDs)
		}

		catID := uint(0)
		if b.ScopeCategoryID != nil {
			catID = *b.ScopeCategoryID
		}

		result = append(result, CategoryBoardBrief{
			BoardID:        b.ID,
			Name:           b.Name,
			Description:    b.Description,
			CategoryID:     catID,
			EventTagIDs:    eventIDs,
			AbstractTagIDs: abstractIDs,
		})
	}

	logging.Infof("board-merge: collected %d category boards for %s", len(result), date.Format("2006-01-02"))
	return result, nil
}

const globalMergeSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了各分类频道独立生成的叙事看板列表。

你的任务是判断哪些看板实际上在讨论同一主题（跨分类的同一事件），应该被合并为一个全局看板。

规则：
1. 只有当两个或多个看板确实在讨论同一核心事件/主题时，才应该合并
2. 不要强行合并不相关的看板
3. 每个合并组至少包含 2 个看板
4. 为合并后的全局看板提供新的名称和描述
5. 独立看板（不需要合并的）不需要出现在输出中

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：merge_groups
2. merge_groups 必须是 JSON 数组；没有需要合并的看板时，返回 {"merge_groups":[]}
3. 每个合并组包含 merged_board_ids（要合并的看板ID列表）、name（新名称）、description（新描述）
4. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`

type MergeGroup struct {
	MergedBoardIDs []uint `json:"merged_board_ids"`
	Name           string `json:"name"`
	Description    string `json:"description"`
}

func buildGlobalMergePrompt(boards []CategoryBoardBrief) string {
	var sb strings.Builder

	sb.WriteString("## 各分类频道看板列表\n\n")

	catNames := make(map[uint]string)
	var categories []models.Category
	boardCatIDs := make(map[uint]bool)
	for _, b := range boards {
		boardCatIDs[b.CategoryID] = true
	}
	catIDList := make([]uint, 0, len(boardCatIDs))
	for id := range boardCatIDs {
		catIDList = append(catIDList, id)
	}
	if len(catIDList) > 0 {
		database.DB.Where("id IN ?", catIDList).Find(&categories)
		for _, c := range categories {
			catNames[c.ID] = c.Name
		}
	}

	for _, b := range boards {
		catName := catNames[b.CategoryID]
		if catName == "" {
			catName = fmt.Sprintf("分类%d", b.CategoryID)
		}
		fmt.Fprintf(&sb, "### [BoardID:%d] %s (分类:%s)\n", b.BoardID, b.Name, catName)
		fmt.Fprintf(&sb, "描述: %s\n", b.Description)

		var narrativesForBoard []models.NarrativeSummary
		database.DB.Where("board_id = ?", b.BoardID).
			Order("id ASC").
			Find(&narrativesForBoard)

		if len(narrativesForBoard) > 0 {
			sb.WriteString("叙事:\n")
			for _, n := range narrativesForBoard {
				fmt.Fprintf(&sb, "  - [叙事ID:%d] %s (状态:%s)\n    摘要: %s\n",
					n.ID, n.Title, n.Status, n.Summary)
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n请判断哪些看板在讨论同一核心主题，应该被合并为全局看板。\n")
	return sb.String()
}

func parseGlobalMergeResponse(content string, validBoardIDs map[uint]bool) ([]MergeGroup, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		MergeGroups []MergeGroup `json:"merge_groups"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse global merge JSON: %w", err)
	}

	var valid []MergeGroup
	for i, g := range raw.MergeGroups {
		if len(g.MergedBoardIDs) < 2 {
			logging.Warnf("board-merge: skipping merge group %d — fewer than 2 boards", i)
			continue
		}

		if strings.TrimSpace(g.Name) == "" {
			logging.Warnf("board-merge: skipping merge group %d — empty name", i)
			continue
		}

		var filteredIDs []uint
		for _, id := range g.MergedBoardIDs {
			if validBoardIDs[id] {
				filteredIDs = append(filteredIDs, id)
			} else {
				logging.Warnf("board-merge: dropping invalid board_id %d in merge group '%s'", id, g.Name)
			}
		}

		if len(filteredIDs) < 2 {
			logging.Warnf("board-merge: skipping merge group %d ('%s') — fewer than 2 valid boards after filtering", i, g.Name)
			continue
		}

		seen := make(map[uint]bool)
		var deduped []uint
		for _, id := range filteredIDs {
			if !seen[id] {
				seen[id] = true
				deduped = append(deduped, id)
			}
		}

		valid = append(valid, MergeGroup{
			MergedBoardIDs: deduped,
			Name:           g.Name,
			Description:    g.Description,
		})
	}

	return valid, nil
}

func MergeGlobalBoards(ctx context.Context, date time.Time, boards []CategoryBoardBrief) error {
	if len(boards) == 0 {
		return nil
	}

	prompt := buildGlobalMergePrompt(boards)

	temperature := 0.3
	maxTokens := 4000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: globalMergeSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"merge_groups": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"merged_board_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "要合并的看板ID列表"},
							"name":             {Type: "string", Description: "合并后的全局看板名称"},
							"description":      {Type: "string", Description: "合并后的全局看板描述"},
						},
						Required: []string{"merged_board_ids", "name", "description"},
					},
				},
			},
			Required: []string{"merge_groups"},
		},
		Metadata: map[string]any{
			"operation":   "global_board_merge",
			"board_count": len(boards),
		},
	})
	if err != nil {
		return fmt.Errorf("global merge AI call failed: %w", err)
	}

	logging.Infof("board-merge: raw LLM response length=%d, first_500=%s", len(result.Content), truncateStr(result.Content, 500))

	validBoardIDs := make(map[uint]bool, len(boards))
	for _, b := range boards {
		validBoardIDs[b.BoardID] = true
	}

	mergeGroups, err := parseGlobalMergeResponse(result.Content, validBoardIDs)
	if err != nil {
		return fmt.Errorf("parse global merge response: %w", err)
	}

	mergedBoardIDs := make(map[uint]bool)
	for _, group := range mergeGroups {
		startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

		var allEventIDs []uint
		var allAbstractIDs []uint
		eventSet := make(map[uint]bool)
		abstractSet := make(map[uint]bool)
		catSet := make(map[uint]bool)

		for _, id := range group.MergedBoardIDs {
			mergedBoardIDs[id] = true
			for _, b := range boards {
				if b.BoardID == id {
					catSet[b.CategoryID] = true
					for _, eid := range b.EventTagIDs {
						if !eventSet[eid] {
							eventSet[eid] = true
							allEventIDs = append(allEventIDs, eid)
						}
					}
					for _, aid := range b.AbstractTagIDs {
						if !abstractSet[aid] {
							abstractSet[aid] = true
							allAbstractIDs = append(allAbstractIDs, aid)
						}
					}
				}
			}
		}

		eventIDsJSON, _ := json.Marshal(allEventIDs)
		abstractIDsJSON, _ := json.Marshal(allAbstractIDs)

		prevIDs := group.MergedBoardIDs
		prevIDsJSON, _ := json.Marshal(prevIDs)

		globalBoard := models.NarrativeBoard{
			PeriodDate:     startOfDay,
			Name:           group.Name,
			Description:    group.Description,
			ScopeType:      models.NarrativeScopeTypeGlobal,
			EventTagIDs:    string(eventIDsJSON),
			AbstractTagIDs: string(abstractIDsJSON),
			PrevBoardIDs:   string(prevIDsJSON),
		}

		if len(catSet) == 1 {
			for catID := range catSet {
				globalBoard.ScopeCategoryID = &catID
			}
		}

		if err := database.DB.Create(&globalBoard).Error; err != nil {
			logging.Warnf("board-merge: failed to create global board '%s': %v", group.Name, err)
			continue
		}

		if err := database.DB.Model(&models.NarrativeSummary{}).
			Where("board_id IN ?", group.MergedBoardIDs).
			Update("board_id", globalBoard.ID).Error; err != nil {
			logging.Warnf("board-merge: failed to reassign narratives to global board %d: %v", globalBoard.ID, err)
		}

		logging.Infof("board-merge: created global board %d ('%s') from %d category boards",
			globalBoard.ID, group.Name, len(group.MergedBoardIDs))
	}

	return nil
}
