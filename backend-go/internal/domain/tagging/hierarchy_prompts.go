package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/logging"
)

func buildMatchPrompt(child *models.TopicTag, candidates []TagCandidate, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, `You are placing a tag "%s" (category: %s) into a hierarchy under a %s parent.

The %s level represents: %s
`, child.Label, child.Category, levelDef.Name, levelDef.Name, levelDef.Description)

	sb.WriteString("=== CANDIDATE PARENT TAGS ===\n")
	for i, c := range candidates {
		fmt.Fprintf(&sb, "%d. %s (similarity: %.4f, description: %s)\n",
			i+1, c.Tag.Label, c.Similarity, truncateDesc(c.Tag.Description, 200))
	}

	fmt.Fprintf(&sb, `
=== DECISION RULES ===
- Choose the SINGLE best parent from the candidates, OR indicate "create_new" if no candidate is suitable
- Similarity >= 0.85: strong signal for match
- Similarity < 0.65: weak signal, prefer create_new unless the semantic match is obvious
- Return JSON: {"action": "select"|"create_new", "target": "<candidate_name>", "reason": "<brief>"}
`)

	return sb.String()
}

func buildCreationPrompt(child *models.TopicTag, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel) string {
	return fmt.Sprintf(`Create a new %s tag for the hierarchy template "%s" (max %d levels).

The child tag is: "%s" (category: %s, description: %s)

%s Level Definition:
- Name: %s
- Description: %s

Create a concise, specific %s name (1-160 chars) and a brief description.
The %s should represent the appropriate grouping of the child tag.

Return JSON: {"name": "<name>", "description": "<description>"}`,
		levelDef.Name, tmpl.TemplateKey(), tmpl.MaxLevel,
		child.Label, child.Category, truncateDesc(child.Description, 300),
		levelDef.Name, levelDef.Name, levelDef.Description,
		levelDef.Name, levelDef.Name)
}

func truncateDesc(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen] + "..."
}

type matchResponse struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type creationResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type anchorVoteResponse struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

func callLLMForMatch(ctx context.Context, child *models.TopicTag, candidates []TagCandidate, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel, targetDepth int) (*models.TopicTag, error) {
	prompt := buildMatchPrompt(child, candidates, tmpl, levelDef)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a hierarchy placement expert. Select the best parent."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "hierarchy_match",
			"tag_id":       child.ID,
			"tag_label":    child.Label,
			"candidates":   len(candidates),
			"target_depth": targetDepth,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM match: %w", err)
	}

	var resp matchResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return nil, fmt.Errorf("parse match response: %w", err)
	}

	if resp.Action == "create_new" || resp.Target == "" {
		return nil, nil
	}

	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Label == resp.Target {
			return c.Tag, nil
		}
	}

	logging.Warnf("LLM selected target '%s' not found in candidates, selecting top candidate", resp.Target)
	if len(candidates) > 0 {
		return candidates[0].Tag, nil
	}
	return nil, nil
}

func callLLMForCreation(ctx context.Context, child *models.TopicTag, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel) (label, description string, err error) {
	prompt := buildCreationPrompt(child, tmpl, levelDef)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You create concise, accurate hierarchy tag names."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":   "hierarchy_create",
			"child_label": child.Label,
			"category":    child.Category,
			"level_name":  levelDef.Name,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return child.Label, child.Description, fmt.Errorf("LLM create: %w", err)
	}

	var resp creationResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return child.Label, child.Description, fmt.Errorf("parse create response: %w", err)
	}

	if resp.Name == "" {
		return child.Label, child.Description, nil
	}
	return resp.Name, resp.Description, nil
}

func callLLMForAnchorVote(ctx context.Context, tag *models.TopicTag, anchors []Anchor, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel) *Anchor {
	var sb strings.Builder
	fmt.Fprintf(&sb, `A tag "%s" (category: %s) has multiple possible %s parents. Select the best one.

=== ANCHOR OPTIONS ===
`, tag.Label, tag.Category, levelDef.Name)

	for i, a := range anchors {
		fmt.Fprintf(&sb, "%d. %s (similarity: %.4f, source: %s)\n",
			i+1, a.ParentLabel, a.Similarity, a.Source)
	}

	sb.WriteString(`
Return JSON: {"target": "<parent_label>", "reason": "<brief>"}`)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a hierarchy placement expert. Vote for the best parent."},
			{Role: "user", Content: sb.String()},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "anchor_vote",
			"tag_id":       tag.ID,
			"tag_label":    tag.Label,
			"anchor_count": len(anchors),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		logging.Warnf("callLLMForAnchorVote: LLM call failed for tag %d: %v", tag.ID, err)
		return nil
	}

	var resp anchorVoteResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		logging.Warnf("callLLMForAnchorVote: parse response failed for tag %d: %v", tag.ID, err)
		return nil
	}

	for _, a := range anchors {
		if a.ParentLabel == resp.Target {
			return &a
		}
	}

	logging.Warnf("callLLMForAnchorVote: selected target '%s' not in anchors", resp.Target)
	return nil
}
