package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
)

func fallbackNarrativeAssociation(ctx context.Context, narrative models.NarrativeSummary, allPrev []PreviousNarrative) ([]uint, error) {
	if len(allPrev) == 0 {
		return nil, nil
	}

	var currentParentIDs []uint
	if narrative.ParentIDs != "" {
		_ = json.Unmarshal([]byte(narrative.ParentIDs), &currentParentIDs)
	}

	validPrevIDs := make(map[uint64]bool, len(allPrev))
	for _, p := range allPrev {
		validPrevIDs[p.ID] = true
	}

	var unresolved []uint
	for _, pid := range currentParentIDs {
		if !validPrevIDs[uint64(pid)] {
			unresolved = append(unresolved, pid)
		}
	}

	if len(unresolved) == 0 {
		return currentParentIDs, nil
	}

	var resolved []uint
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		resolved, lastErr = attemptFallbackResolution(ctx, narrative, allPrev)
		if lastErr == nil {
			return resolved, nil
		}
		logging.Warnf("board-postprocess: fallback retry %d failed for narrative %d: %v", retry+1, narrative.ID, lastErr)
	}

	if len(resolved) > 0 {
		return resolved, nil
	}
	return nil, fmt.Errorf("fallback narrative association failed after 3 retries for narrative %d: %w", narrative.ID, lastErr)
}

func attemptFallbackResolution(ctx context.Context, narrative models.NarrativeSummary, allPrev []PreviousNarrative) ([]uint, error) {
	prompt := buildFallbackPrompt(narrative, allPrev)

	temperature := 0.3
	maxTokens := 1000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: fallbackAssociationSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"parent_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "匹配的父叙事ID列表"},
			},
			Required: []string{"parent_ids"},
		},
		Metadata: map[string]any{
			"operation":    "fallback_narrative_association",
			"narrative_id": narrative.ID,
			"prev_count":   len(allPrev),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("fallback association AI call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var parsed struct {
		ParentIDs []uint `json:"parent_ids"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("parse fallback response: %w", err)
	}

	validPrevIDs := make(map[uint64]bool, len(allPrev))
	for _, p := range allPrev {
		validPrevIDs[p.ID] = true
	}

	var valid []uint
	for _, id := range parsed.ParentIDs {
		if validPrevIDs[uint64(id)] {
			valid = append(valid, id)
		}
	}

	return valid, nil
}

const fallbackAssociationSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了一条叙事和所有可用的昨日叙事列表。

你的任务是判断这条叙事最可能是从哪些昨日叙事延续而来的。根据标题和摘要的语义关联来匹配。

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：parent_ids
2. parent_ids 必须是 JSON 数组，包含匹配的昨日叙事ID
3. 如果没有找到合理的匹配，返回 {"parent_ids":[]}
4. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀`

func buildFallbackPrompt(narrative models.NarrativeSummary, allPrev []PreviousNarrative) string {
	var sb strings.Builder

	sb.WriteString("## 待匹配的叙事\n\n")
	fmt.Fprintf(&sb, "- 标题: %s\n  摘要: %s\n  状态: %s\n",
		narrative.Title, narrative.Summary, narrative.Status)

	sb.WriteString("\n## 可用的昨日叙事列表\n\n")
	for _, p := range allPrev {
		fmt.Fprintf(&sb, "- [ID:%d] %s (状态:%s, 第%d代)\n  摘要: %s\n",
			p.ID, p.Title, p.Status, p.Generation, p.Summary)
	}

	sb.WriteString("\n请判断待匹配叙事最可能从哪些昨日叙事延续而来，返回匹配的 ID 列表。\n")
	return sb.String()
}

type BoardConnection struct {
	FromBoardID uint `json:"from_board_id"`
	ToBoardID   uint `json:"to_board_id"`
}

func DeriveBoardConnections() ([]BoardConnection, error) {
	var narratives []models.NarrativeSummary
	database.DB.Where("board_id IS NOT NULL").
		Order("id ASC").
		Find(&narratives)

	if len(narratives) == 0 {
		return nil, nil
	}

	narrativeBoardMap := make(map[uint64]uint)
	for _, n := range narratives {
		if n.BoardID != nil {
			narrativeBoardMap[n.ID] = *n.BoardID
		}
	}

	allPrevIDs := make([]uint64, 0)
	for _, n := range narratives {
		var parentIDs []uint64
		if n.ParentIDs != "" {
			_ = json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		}
		allPrevIDs = append(allPrevIDs, parentIDs...)
	}

	if len(allPrevIDs) > 0 {
		var prevNarratives []models.NarrativeSummary
		database.DB.Where("id IN ?", allPrevIDs).Find(&prevNarratives)
		for _, pn := range prevNarratives {
			if pn.BoardID != nil {
				narrativeBoardMap[pn.ID] = *pn.BoardID
			}
		}
	}

	connectionSet := make(map[string]bool)
	var connections []BoardConnection

	for _, n := range narratives {
		if n.BoardID == nil {
			continue
		}

		var parentIDs []uint64
		if n.ParentIDs != "" {
			_ = json.Unmarshal([]byte(n.ParentIDs), &parentIDs)
		}

		for _, pid := range parentIDs {
			parentBoardID, ok := narrativeBoardMap[pid]
			if !ok || parentBoardID == *n.BoardID {
				continue
			}

			key := fmt.Sprintf("%d->%d", parentBoardID, *n.BoardID)
			if connectionSet[key] {
				continue
			}
			connectionSet[key] = true

			connections = append(connections, BoardConnection{
				FromBoardID: parentBoardID,
				ToBoardID:   *n.BoardID,
			})
		}
	}

	logging.Infof("board-postprocess: derived %d board connections from %d narratives", len(connections), len(narratives))
	return connections, nil
}

func FeedbackNarrativesToTagsWithBoard(outputs []NarrativeOutput) {
}
