package topicanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/logging"
)

func buildL2MatchPrompt(tag *models.TopicTag, candidates []TagCandidate) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are placing a leaf tag "%s" (category: %s) into a hierarchy under an L2 parent.

The L2 level represents the main entity/subject the leaf tag belongs to.
`, tag.Label, tag.Category))

	sb.WriteString("=== L2 CANDIDATE PARENT TAGS ===\n")
	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("%d. %s (similarity: %.4f, description: %s)\n",
			i+1, c.Tag.Label, c.Similarity, truncateDesc(c.Tag.Description, 200)))
	}

	sb.WriteString(fmt.Sprintf(`
=== DECISION RULES ===
- Choose the SINGLE best L2 parent from the candidates, OR indicate "create_new" if no candidate is suitable
- The L2 should be the entity/company/product/organization the leaf tag belongs to
- Similarity >= %.2f: strong signal for match
- Similarity < %.2f: weak signal, prefer create_new unless the semantic match is obvious
- Return JSON: {"action": "select"|"create_new", "target": "<candidate_name_or_empty>", "reason": "<brief>"}
`, PlacementL2HighThreshold, PlacementL2LowThreshold))

	return sb.String()
}

func buildL1MatchPrompt(l2Tag *models.TopicTag, candidates []TagCandidate, existingL1s []*models.TopicTag) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are placing an L2 tag "%s" (category: %s) under an L1 event type / category.

The L1 level represents the highest-level categorization (e.g., "产品发布", "融资并购", "政策法规").
`, l2Tag.Label, l2Tag.Category))

	if len(existingL1s) > 0 {
		sb.WriteString("\n=== EXISTING L1 TYPES (for reference) ===\n")
		for i, l1 := range existingL1s {
			sb.WriteString(fmt.Sprintf("%d. %s (description: %s)\n", i+1, l1.Label, truncateDesc(l1.Description, 100)))
		}
		sb.WriteString("Prefer reusing an existing L1 type if semantically appropriate.\n")
	}

	if len(candidates) > 0 {
		sb.WriteString("\n=== L1 CANDIDATE PARENT TAGS (embedding matches) ===\n")
		for i, c := range candidates {
			sb.WriteString(fmt.Sprintf("%d. %s (similarity: %.4f, description: %s)\n",
				i+1, c.Tag.Label, c.Similarity, truncateDesc(c.Tag.Description, 200)))
		}
	}

	sb.WriteString(fmt.Sprintf(`
=== DECISION RULES ===
- If an existing L1 type fits: return {"action": "select_existing", "target": "<type_name>", "reason": "..."}
- If a candidate is suitable: return {"action": "select", "target": "<candidate_name>", "reason": "..."}
- If a new type is needed: return {"action": "create_new", "target": "", "new_name": "<suggested_name>", "new_description": "<brief>", "reason": "..."}
- Similarity >= %.2f: strong, prefer select
- Similarity < %.2f: weak, prefer create_new
`, PlacementL1HighThreshold, PlacementL1LowThreshold))

	return sb.String()
}

func buildL2CreationPrompt(childTag *models.TopicTag, tmpl *CategoryHierarchyTemplate) string {
	l2Def := tmpl.Levels[1]

	return fmt.Sprintf(`Create a new L2 tag for the hierarchy template "%s" (max %d levels).

The child tag is: "%s" (category: %s, description: %s)

L2 Level Definition:
- Name: %s
- Description: %s

The L2 tag should represent the entity/company/product/organization that the child belongs to.
Create a concise, specific L2 tag name (1-160 chars) and a brief description.

Return JSON: {"name": "<name>", "description": "<description>"}`,
		tmpl.TemplateKey(), tmpl.MaxLevel,
		childTag.Label, childTag.Category, truncateDesc(childTag.Description, 300),
		l2Def.Name, l2Def.Description)
}

func buildL1CreationPrompt(l2Tag *models.TopicTag, existingL1s []*models.TopicTag, tmpl *CategoryHierarchyTemplate) string {
	l1Def := tmpl.Levels[0]

	var existingList strings.Builder
	if len(existingL1s) > 0 {
		existingList.WriteString("\nExisting L1 types (use as few-shot reference, prefer reusing if semantically appropriate):\n")
		for i, l1 := range existingL1s {
			existingList.WriteString(fmt.Sprintf("%d. %s\n", i+1, l1.Label))
		}
	}

	return fmt.Sprintf(`Create a new L1 event type / category for the hierarchy template "%s" (max %d levels).

The L2 tag is: "%s" (category: %s, description: %s)

L1 Level Definition:
- Name: %s
- Description: %s
%s

Create a concise, specific L1 type name (1-160 chars) and a brief description.
The L1 should be the highest-level categorization — broad enough to potentially contain multiple L2 tags.

Return JSON: {"name": "<name>", "description": "<description>"}`,
		tmpl.TemplateKey(), tmpl.MaxLevel,
		l2Tag.Label, l2Tag.Category, truncateDesc(l2Tag.Description, 300),
		l1Def.Name, l1Def.Description, existingList.String())
}

func buildL1DedupPrompt(tag1 *models.TopicTag, tag2 *models.TopicTag) string {
	return fmt.Sprintf(`Determine whether these two L1 event type tags represent the same concept and should be merged.

Tag 1: "%s" (description: %s)
Tag 2: "%s" (description: %s)

Return JSON: {"should_merge": true|false, "merged_name": "<preferred_name>", "reason": "<brief>"}`, 
		tag1.Label, truncateDesc(tag1.Description, 200),
		tag2.Label, truncateDesc(tag2.Description, 200))
}

func truncateDesc(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen] + "..."
}

type l2MatchResponse struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type l1MatchResponse struct {
	Action         string `json:"action"`
	Target         string `json:"target"`
	NewName        string `json:"new_name"`
	NewDescription string `json:"new_description"`
	Reason         string `json:"reason"`
}

type tagCreationResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type l1DedupResponse struct {
	ShouldMerge bool   `json:"should_merge"`
	MergedName  string `json:"merged_name"`
	Reason      string `json:"reason"`
}

func callLLMForL2Match(ctx context.Context, tag *models.TopicTag, candidates []TagCandidate) (*models.TopicTag, error) {
	prompt := buildL2MatchPrompt(tag, candidates)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a hierarchy placement expert. Select the best L2 parent."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":  "l2_match",
			"tag_id":     tag.ID,
			"tag_label":  tag.Label,
			"candidates": len(candidates),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM L2 match: %w", err)
	}

	var resp l2MatchResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return nil, fmt.Errorf("parse L2 match response: %w", err)
	}

	if resp.Action == "create_new" || resp.Target == "" {
		return nil, nil
	}

	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Label == resp.Target {
			return c.Tag, nil
		}
	}

	logging.Warnf("LLM selected L2 target '%s' not found in candidates, selecting top candidate", resp.Target)
	if len(candidates) > 0 {
		return candidates[0].Tag, nil
	}
	return nil, nil
}

func callLLMForL1Match(ctx context.Context, l2Tag *models.TopicTag, candidates []TagCandidate, existingL1s []*models.TopicTag, tmpl *CategoryHierarchyTemplate) (*models.TopicTag, error) {
	prompt := buildL1MatchPrompt(l2Tag, candidates, existingL1s)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a hierarchy placement expert. Select or create L1 event types."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "l1_match",
			"tag_id":       l2Tag.ID,
			"tag_label":    l2Tag.Label,
			"candidates":   len(candidates),
			"existing_l1s": len(existingL1s),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM L1 match: %w", err)
	}

	var resp l1MatchResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return nil, fmt.Errorf("parse L1 match response: %w", err)
	}

	switch resp.Action {
	case "select_existing":
		for _, l1 := range existingL1s {
			if l1.Label == resp.Target {
				return l1, nil
			}
		}
	case "select":
		for _, c := range candidates {
			if c.Tag != nil && c.Tag.Label == resp.Target {
				return c.Tag, nil
			}
		}
	}

	return nil, nil
}

func callLLMForL2Creation(ctx context.Context, childTag *models.TopicTag, tmpl *CategoryHierarchyTemplate) (label, description string, err error) {
	prompt := buildL2CreationPrompt(childTag, tmpl)

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
			"operation":   "l2_create",
			"child_label": childTag.Label,
			"category":    childTag.Category,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return childTag.Label, childTag.Description, fmt.Errorf("LLM L2 create: %w", err)
	}

	var resp tagCreationResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return childTag.Label, childTag.Description, fmt.Errorf("parse L2 create response: %w", err)
	}

	if resp.Name == "" {
		return childTag.Label, childTag.Description, nil
	}
	return resp.Name, resp.Description, nil
}

func callLLMForL1Creation(ctx context.Context, l2Tag *models.TopicTag, existingL1s []*models.TopicTag, tmpl *CategoryHierarchyTemplate) (label, description string, err error) {
	prompt := buildL1CreationPrompt(l2Tag, existingL1s, tmpl)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You create concise, accurate hierarchy tag type names."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":    "l1_create",
			"l2_label":     l2Tag.Label,
			"category":     l2Tag.Category,
			"existing_l1s": len(existingL1s),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return l2Tag.Label, l2Tag.Description, fmt.Errorf("LLM L1 create: %w", err)
	}

	var resp tagCreationResponse
	if err := json.Unmarshal([]byte(result.Content), &resp); err != nil {
		return l2Tag.Label, l2Tag.Description, fmt.Errorf("parse L1 create response: %w", err)
	}

	if resp.Name == "" {
		return l2Tag.Label, l2Tag.Description, nil
	}
	return resp.Name, resp.Description, nil
}
