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

type BoardPartition struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	EventTagIDs    []uint `json:"event_tag_ids"`
	AbstractTagIDs []uint `json:"abstract_tag_ids"`
	PrevBoardIDs   []uint `json:"prev_board_ids"`
}

const boardPartitionSystemPrompt = `你是一名专业的新闻叙事看板分析师。你的任务是将当天的事件标签和抽象标签分组到叙事看板（board）中。

每个看板代表一个独立的故事线或主题领域。规则：
1. 每个事件标签只能出现在一个看板中（不可重复分配）
2. 抽象标签可以分配到多个看板（如果一个抽象标签涵盖的子标签分散在多个看板中）。abstract_tag_ids 必须使用下方抽象树中的 AbstractTagID 数字，不要使用列表序号
3. 为每个看板提供简洁的名称（中文，不超过20字）和描述（中文，50-100字，说明看板涵盖的故事范围）
4. 如果某个事件标签与前一天的某个看板延续，标注 prev_board_ids
5. 每个看板必须至少包含一个事件标签
6. 不要为了凑数而强行合并不相关的事件
7. 数量不固定，有几条写几条，没有就返回空数组

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：boards
2. boards 必须是 JSON 数组；没有符合条件的看板时，返回 {"boards":[]}
3. boards 数组中的每个元素都必须包含 name、description、event_tag_ids、abstract_tag_ids、prev_board_ids 字段
4. event_tag_ids 和 prev_board_ids 必须始终输出数组，即使为空也要输出 []
5. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`

func buildBoardPartitionPrompt(
	events []TagInput,
	abstractTrees []AbstractTreeNode,
	prevBoards []PreviousBoardBrief,
	prevNarratives []BoardNarrativeBrief,
) string {
	var sb strings.Builder

	sb.WriteString("## 今日事件标签\n\n")
	for _, t := range events {
		fmt.Fprintf(&sb, "- [EventTagID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount)
		if t.Description != "" {
			fmt.Fprintf(&sb, ", 描述:%s", t.Description)
		}
		sb.WriteString(")\n")
	}

	sb.WriteString("\n## 今日抽象标签\n\n")
	for _, tree := range abstractTrees {
		writeTreeNode(&sb, fmt.Sprintf("#### 抽象标签 [AbstractTagID:%d]", tree.ID), tree, 0)
		sb.WriteString("\n")
	}

	if len(prevBoards) > 0 {
		sb.WriteString("\n## 昨日看板（供延续参考）\n\n")
		for _, b := range prevBoards {
			fmt.Fprintf(&sb, "- [PrevBoardID:%d] %s: %s\n", b.ID, b.Name, b.Description)
		}
	}

	if len(prevNarratives) > 0 {
		sb.WriteString("\n## 昨日看板中的叙事（供上下文参考）\n\n")
		for _, n := range prevNarratives {
			fmt.Fprintf(&sb, "- [叙事ID:%d, 看板ID:%d] %s (状态:%s)\n  摘要: %s\n",
				n.ID, n.BoardID, n.Title, n.Status, n.Summary)
		}
	}

	sb.WriteString("\n请将以上事件标签和抽象标签分组到叙事看板中。确保每个事件标签只出现在一个看板中。\n")
	return sb.String()
}

func parseBoardPartitionResponse(content string, validEventIDs map[uint]bool, validAbstractIDs map[uint]bool, validPrevBoardIDs map[uint]bool) ([]BoardPartition, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Boards []BoardPartition `json:"boards"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse board partition JSON: %w", err)
	}

	eventAssignment := make(map[uint]int)
	var valid []BoardPartition
	for i, b := range raw.Boards {
		if strings.TrimSpace(b.Name) == "" {
			logging.Warnf("board-generator: skipping board %d with empty name", i)
			continue
		}

		var filteredEventIDs []uint
		for _, id := range b.EventTagIDs {
			if !validEventIDs[id] {
				logging.Warnf("board-generator: dropping invalid event_tag_id %d in board '%s'", id, b.Name)
				continue
			}
			if prevBoard, exists := eventAssignment[id]; exists {
				logging.Warnf("board-generator: event tag %d already assigned to board index %d, removing from board '%s'",
					id, prevBoard, b.Name)
				continue
			}
			filteredEventIDs = append(filteredEventIDs, id)
		}

		if len(filteredEventIDs) == 0 {
			logging.Warnf("board-generator: skipping board '%s' — no valid event tags after filtering", b.Name)
			continue
		}

		for _, id := range filteredEventIDs {
			eventAssignment[id] = i
		}

		var filteredAbstractIDs []uint
		for _, id := range b.AbstractTagIDs {
			if validAbstractIDs[id] {
				filteredAbstractIDs = append(filteredAbstractIDs, id)
			} else {
				logging.Warnf("board-generator: dropping invalid abstract_tag_id %d in board '%s'", id, b.Name)
			}
		}

		var filteredPrevBoardIDs []uint
		for _, id := range b.PrevBoardIDs {
			if validPrevBoardIDs[id] {
				filteredPrevBoardIDs = append(filteredPrevBoardIDs, id)
			} else {
				logging.Warnf("board-generator: dropping invalid prev_board_id %d in board '%s'", id, b.Name)
			}
		}

		if filteredAbstractIDs == nil {
			filteredAbstractIDs = []uint{}
		}
		if filteredPrevBoardIDs == nil {
			filteredPrevBoardIDs = []uint{}
		}
		if filteredEventIDs == nil {
			filteredEventIDs = []uint{}
		}

		valid = append(valid, BoardPartition{
			Name:           b.Name,
			Description:    b.Description,
			EventTagIDs:    filteredEventIDs,
			AbstractTagIDs: filteredAbstractIDs,
			PrevBoardIDs:   filteredPrevBoardIDs,
		})
	}

	return valid, nil
}

func GenerateBoardsForCategory(ctx context.Context, date time.Time, categoryID uint, categoryLabel string) ([]models.NarrativeBoard, error) {
	events, err := CollectUnclassifiedEventTagsByCategory(date, categoryID)
	if err != nil {
		return nil, fmt.Errorf("collect event tags for category %d: %w", categoryID, err)
	}
	if len(events) == 0 {
		logging.Infof("board-generator: no event tags for category %d on %s, skipping", categoryID, date.Format("2006-01-02"))
		return nil, nil
	}

	abstractTrees, err := CollectAbstractTreeInputsByCategory(date, categoryID)
	if err != nil {
		return nil, fmt.Errorf("collect abstract trees for category %d: %w", categoryID, err)
	}

	prevBoards, err := CollectPreviousDayBoards(date, models.NarrativeScopeTypeFeedCategory, &categoryID)
	if err != nil {
		logging.Warnf("board-generator: failed to collect previous day boards for category %d: %v", categoryID, err)
	}

	var prevBoardIDs []uint
	prevBoardIDSet := make(map[uint]bool)
	for _, b := range prevBoards {
		prevBoardIDs = append(prevBoardIDs, b.ID)
		prevBoardIDSet[b.ID] = true
	}

	var prevNarratives []BoardNarrativeBrief
	if len(prevBoardIDs) > 0 {
		prevNarratives, err = CollectPreviousBoardNarratives(prevBoardIDs)
		if err != nil {
			logging.Warnf("board-generator: failed to collect previous board narratives: %v", err)
		}
	}

	prompt := buildBoardPartitionPrompt(events, abstractTrees, prevBoards, prevNarratives)

	temperature := 0.4
	maxTokens := 6000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: boardPartitionSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"boards": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":             {Type: "string", Description: "看板名称，不超过20字"},
							"description":      {Type: "string", Description: "看板描述，50-100字"},
							"event_tag_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "分配到此看板的事件标签ID"},
							"abstract_tag_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "分配到此看板的抽象标签ID"},
							"prev_board_ids":   {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "延续的昨日看板ID"},
						},
						Required: []string{"name", "description", "event_tag_ids", "abstract_tag_ids", "prev_board_ids"},
					},
				},
			},
			Required: []string{"boards"},
		},
		Metadata: map[string]any{
			"operation":   "board_partition",
			"category_id": categoryID,
			"event_count": len(events),
			"tree_count":  len(abstractTrees),
			"prev_boards": len(prevBoards),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("board partition AI call failed: %w", err)
	}

	logging.Infof("board-generator: raw LLM response length=%d, first_500=%s", len(result.Content), truncateStr(result.Content, 500))

	validEventIDs := make(map[uint]bool, len(events))
	for _, e := range events {
		validEventIDs[e.ID] = true
	}

	validAbstractIDs := make(map[uint]bool)
	collectAbstractTagIDsFromTrees(abstractTrees, validAbstractIDs)

	validPrevBoardIDs := prevBoardIDSet

	partitions, err := parseBoardPartitionResponse(result.Content, validEventIDs, validAbstractIDs, validPrevBoardIDs)
	if err != nil {
		return nil, fmt.Errorf("parse board partition response: %w", err)
	}

	if len(partitions) == 0 {
		logging.Infof("board-generator: no boards generated for category %d on %s", categoryID, date.Format("2006-01-02"))
		return nil, nil
	}

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	var boards []models.NarrativeBoard
	for _, p := range partitions {
		eventIDsJSON, _ := json.Marshal(p.EventTagIDs)
		abstractIDsJSON, _ := json.Marshal(p.AbstractTagIDs)
		prevIDsJSON, _ := json.Marshal(p.PrevBoardIDs)

		board := models.NarrativeBoard{
			PeriodDate:      startOfDay,
			Name:            p.Name,
			Description:     p.Description,
			ScopeType:       models.NarrativeScopeTypeFeedCategory,
			ScopeCategoryID: &categoryID,
			EventTagIDs:     string(eventIDsJSON),
			AbstractTagIDs:  string(abstractIDsJSON),
			PrevBoardIDs:    string(prevIDsJSON),
		}
		boards = append(boards, board)
	}

	if err := database.DB.CreateInBatches(boards, 20).Error; err != nil {
		return nil, fmt.Errorf("save boards for category %d: %w", categoryID, err)
	}

	logging.Infof("board-generator: saved %d boards for category %d (%s) on %s",
		len(boards), categoryID, categoryLabel, date.Format("2006-01-02"))
	return boards, nil
}

func collectAbstractTagIDsFromTrees(trees []AbstractTreeNode, idSet map[uint]bool) {
	for _, t := range trees {
		collectAbstractTagIDsFromNode(t, idSet)
	}
}

func collectAbstractTagIDsFromNode(node AbstractTreeNode, idSet map[uint]bool) {
	if node.IsAbstract {
		idSet[node.ID] = true
	}
	for _, child := range node.Children {
		collectAbstractTagIDsFromNode(child, idSet)
	}
}

func writeTreeNode(sb *strings.Builder, prefix string, node AbstractTreeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(sb, "%s%s: %s (分类:%s, 文章数:%d", prefix, indent, node.Label, node.Category, node.ArticleCount)
	if node.IsAbstract {
		sb.WriteString(", 抽象标签")
	}
	if node.Description != "" {
		fmt.Fprintf(sb, ", 描述:%s", node.Description)
	}
	sb.WriteString(")\n")

	for _, child := range node.Children {
		writeTreeNode(sb, "-", child, depth+1)
	}
}
