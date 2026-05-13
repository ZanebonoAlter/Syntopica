package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

type BatchTagJudgmentItem struct {
	Label       string
	Category    string
	Description string
	Candidates  []TagCandidate
}

type BatchTagJudgmentResult struct {
	Results map[string]*TagExtractionResult
}

func BatchCallLLMForTagJudgment(ctx context.Context, items []BatchTagJudgmentItem, narrativeContext string) (*BatchTagJudgmentResult, error) {
	if len(items) == 0 {
		return &BatchTagJudgmentResult{Results: make(map[string]*TagExtractionResult)}, nil
	}
	if len(items) == 1 {
		result, err := ExtractAbstractTag(ctx, items[0].Candidates, items[0].Label, items[0].Category, WithCaller("batch_single"))
		if err != nil {
			return nil, err
		}
		return &BatchTagJudgmentResult{Results: map[string]*TagExtractionResult{items[0].Label: result}}, nil
	}

	prompt := buildBatchTagJudgmentPrompt(items, narrativeContext)

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation": "batch_tag_judgment",
			"tag_count": len(items),
			"caller":    "batch",
		},
	}

	resp, err := airouter.NewRouter().Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch tag judgment LLM call failed: %w", err)
	}

	return parseBatchTagJudgmentResponse(ctx, resp.Content, items)
}

func buildBatchTagJudgmentPrompt(items []BatchTagJudgmentItem, narrativeContext string) string {
	var sb strings.Builder
	sb.WriteString("You are comparing MULTIPLE new tags against existing candidate tags.\n")
	sb.WriteString("For EACH new tag, decide if its candidates are the same concept (merge), related (abstract), or unrelated (none).\n\n")

	for i, item := range items {
		fmt.Fprintf(&sb, "### New Tag %d: %q (category: %s)\n", i+1, item.Label, item.Category)
		if item.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", item.Description)
		}
		sb.WriteString("Existing candidates:\n")
		sb.WriteString(buildCandidateList(item.Candidates))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Return a JSON object with a \"tags\" field. Each key is the new tag label, and the value has three arrays: merges, abstracts, none.\n")
	sb.WriteString("The rules for merge/abstract/none are the same as single-tag judgment.\n")
	sb.WriteString("EVERY candidate for EVERY tag must appear in exactly one of the three arrays.\n")

	if narrativeContext != "" {
		fmt.Fprintf(&sb, "\nAdditional context:\n%s\n", narrativeContext)
	}

	return sb.String()
}

func parseBatchTagJudgmentResponse(ctx context.Context, content string, items []BatchTagJudgmentItem) (*BatchTagJudgmentResult, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Tags map[string]json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse batch tag judgment response: %w", err)
	}

	result := &BatchTagJudgmentResult{Results: make(map[string]*TagExtractionResult)}
	for _, item := range items {
		rawJudgment, ok := raw.Tags[item.Label]
		if !ok {
			logging.Warnf("batch tag judgment: no result for tag %q, treating as no_action", item.Label)
			continue
		}
		judgment, err := parseTagJudgmentFromString(string(rawJudgment), item.Candidates)
		if err != nil {
			logging.Warnf("batch tag judgment: parse failed for %q: %v, treating as no_action", item.Label, err)
			continue
		}
		extractionResult, err := ProcessJudgment(ctx, judgment, item.Candidates, item.Label, item.Category)
		if err != nil {
			logging.Warnf("batch tag judgment: processJudgment failed for %q: %v", item.Label, err)
			continue
		}
		result.Results[item.Label] = extractionResult
	}

	return result, nil
}

func parseTagJudgmentFromString(content string, candidates []TagCandidate) (*tagJudgment, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Merges    []json.RawMessage `json:"merges"`
		Abstracts []json.RawMessage `json:"abstracts"`
		None      []string          `json:"none"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse tag judgment: %w", err)
	}

	candidateLabels := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			candidateLabels[c.Tag.Label] = true
		}
	}

	judgment := &tagJudgment{}
	usedLabels := make(map[string]bool)

	for _, item := range raw.Merges {
		var m struct {
			Target   string   `json:"target"`
			Label    string   `json:"label"`
			Children []string `json:"children"`
			Reason   string   `json:"reason"`
		}
		if err := json.Unmarshal(item, &m); err != nil || m.Target == "" {
			continue
		}
		merge := tagJudgmentMerge{
			Target: strings.TrimSpace(m.Target),
			Label:  strings.TrimSpace(m.Label),
			Reason: m.Reason,
		}
		if merge.Label == "" {
			merge.Label = merge.Target
		}
		for _, ch := range m.Children {
			ch = strings.TrimSpace(ch)
			if candidateLabels[ch] {
				merge.Children = append(merge.Children, ch)
			}
		}
		if usedLabels[merge.Target] {
			continue
		}
		judgment.Merges = append(judgment.Merges, merge)
		usedLabels[merge.Target] = true
		for _, ch := range merge.Children {
			usedLabels[ch] = true
		}
	}

	for _, item := range raw.Abstracts {
		var a struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Children    []string `json:"children"`
			Reason      string   `json:"reason"`
		}
		if err := json.Unmarshal(item, &a); err != nil || a.Name == "" {
			continue
		}
		abstractName := strings.TrimSpace(a.Name)
		if len(abstractName) > maxAbstractNameLen {
			abstractName = abstractName[:maxAbstractNameLen]
		}
		desc := strings.TrimSpace(a.Description)
		if len(desc) > 500 {
			desc = desc[:500]
		}
		abstract := tagJudgmentAbstract{
			Name:        abstractName,
			Description: desc,
			Reason:      a.Reason,
		}
		var dedupedChildren []string
		for _, ch := range a.Children {
			ch = strings.TrimSpace(ch)
			if candidateLabels[ch] && !usedLabels[ch] {
				dedupedChildren = append(dedupedChildren, ch)
				usedLabels[ch] = true
			}
		}
		if len(dedupedChildren) > 0 {
			abstract.Children = dedupedChildren
			judgment.Abstracts = append(judgment.Abstracts, abstract)
		}
	}

	for _, label := range raw.None {
		label = strings.TrimSpace(label)
		if label == "" || !candidateLabels[label] || usedLabels[label] {
			continue
		}
		judgment.None = append(judgment.None, label)
		usedLabels[label] = true
	}

	for _, c := range candidates {
		if c.Tag != nil && !usedLabels[c.Tag.Label] {
			judgment.None = append(judgment.None, c.Tag.Label)
		}
	}

	return judgment, nil
}
