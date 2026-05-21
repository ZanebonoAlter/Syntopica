package tagging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

type tagJudgment struct {
	Merges    []tagJudgmentMerge    `json:"merges,omitempty"`
	Abstracts []tagJudgmentAbstract `json:"abstracts,omitempty"`
	None      []string              `json:"none,omitempty"`
}

type tagJudgmentMerge struct {
	Target   string   `json:"target"`
	Label    string   `json:"label"`
	Children []string `json:"children"`
	Reason   string   `json:"reason"`
}

type tagJudgmentAbstract struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Children    []string `json:"children"`
	Reason      string   `json:"reason"`
}

const mergeMinSimilarity = 0.85

func callLLMForTagJudgment(ctx context.Context, candidates []TagCandidate, newLabel string, category string, narrativeContext string, caller string) (*tagJudgment, error) {
	router := airouter.NewRouter()
	hasHighSim := shouldAllowMergeJudgment(candidates)
	maxCandidateDepth := computeMaxCandidateDepth(candidates)
	prompt := buildTagJudgmentPrompt(candidates, newLabel, category, narrativeContext, hasHighSim, maxCandidateDepth)

	// Get sub_type from candidates if available
	subType := ""
	if len(candidates) > 0 && candidates[0].Tag != nil {
		subType = candidates[0].Tag.SubType
	}
	prompt = injectHierarchyContext(prompt, category, subType)

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		JSONSchema:  buildTagJudgmentSchema(),
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":       "tag_judgment",
			"caller":          caller,
			"candidate_count": len(candidates),
			"new_label":       newLabel,
			"category":        category,
			"candidates":      buildCandidateSummary(candidates),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	logging.Infof("Tag judgment LLM response for %q: %s", newLabel, result.Content)

	return parseTagJudgmentResponse(result.Content, candidates)
}

func ensureNewLabelCandidateInAbstractJudgment(judgment *tagJudgment, candidates []TagCandidate, newLabel string) {
	if judgment == nil || len(judgment.Abstracts) == 0 {
		return
	}

	label := candidateLabelForNewLabel(candidates, newLabel)
	if label == "" {
		return
	}

	// Check if label is already in any abstract or merge
	for _, abstract := range judgment.Abstracts {
		if labelInSlice(abstract.Children, label) {
			return
		}
	}
	for _, merge := range judgment.Merges {
		if merge.Target == label || labelInSlice(merge.Children, label) {
			return
		}
	}

	// Add to the first abstract that has space
	if len(judgment.Abstracts) > 0 {
		judgment.Abstracts[0].Children = append(judgment.Abstracts[0].Children, label)
	}
}

func candidateLabelForNewLabel(candidates []TagCandidate, newLabel string) string {
	newSlug := Slugify(newLabel)
	if newSlug == "" {
		return ""
	}
	for _, c := range candidates {
		if c.Tag == nil {
			continue
		}
		if c.Tag.Slug == newSlug || Slugify(c.Tag.Label) == newSlug {
			return c.Tag.Label
		}
	}
	return ""
}

func labelInSlice(labels []string, target string) bool {
	for _, label := range labels {
		if label == target {
			return true
		}
	}
	return false
}

func selectMergeTarget(candidates []TagCandidate, mergeTarget string, mergeLabel string) *models.TopicTag {
	mergeTargetSlug := Slugify(mergeTarget)
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Slug == mergeTargetSlug {
			return c.Tag
		}
	}

	mergeLabelSlug := Slugify(mergeLabel)
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Slug == mergeLabelSlug {
			return c.Tag
		}
	}

	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Label == mergeTarget {
			return c.Tag
		}
	}

	if mergeTargetSlug != "" || mergeLabelSlug != "" {
		for _, c := range candidates {
			if c.Tag != nil && c.Tag.Source != "abstract" {
				cSlug := Slugify(c.Tag.Label)
				if mergeTargetSlug != "" && cSlug == mergeTargetSlug {
					return c.Tag
				}
				if mergeLabelSlug != "" && cSlug == mergeLabelSlug {
					return c.Tag
				}
			}
		}
	}

	logging.Warnf("selectMergeTarget: LLM target %q and label %q do not match any candidate, refusing merge", mergeTarget, mergeLabel)
	return nil
}

func parseTagJudgmentResponse(content string, candidates []TagCandidate) (*tagJudgment, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	// Support both single and array formats for backward compatibility
	var raw struct {
		// New array format
		Merges    []json.RawMessage `json:"merges"`
		Abstracts []json.RawMessage `json:"abstracts"`
		None      []string          `json:"none"`
		// Legacy single format (for backward compatibility)
		Merge    json.RawMessage `json:"merge"`
		Abstract json.RawMessage `json:"abstract"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse tag judgment response: %w", err)
	}

	candidateLabels := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			candidateLabels[c.Tag.Label] = true
		}
	}

	filterChildren := func(children []string) []string {
		var valid []string
		for _, ch := range children {
			ch = strings.TrimSpace(ch)
			if candidateLabels[ch] {
				valid = append(valid, ch)
			}
		}
		return valid
	}

	judgment := &tagJudgment{}
	usedLabels := make(map[string]bool)

	// Helper to parse a single merge item
	parseMerge := func(data json.RawMessage) *tagJudgmentMerge {
		var m struct {
			Target   string   `json:"target"`
			Label    string   `json:"label"`
			Children []string `json:"children"`
			Reason   string   `json:"reason"`
		}
		if err := json.Unmarshal(data, &m); err != nil || m.Target == "" {
			return nil
		}
		merge := &tagJudgmentMerge{
			Target:   strings.TrimSpace(m.Target),
			Label:    strings.TrimSpace(m.Label),
			Children: filterChildren(m.Children),
			Reason:   m.Reason,
		}
		if merge.Label == "" {
			merge.Label = merge.Target
		}
		return merge
	}

	// Helper to parse a single abstract item
	parseAbstract := func(data json.RawMessage) *tagJudgmentAbstract {
		var a struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Children    []string `json:"children"`
			Reason      string   `json:"reason"`
		}
		if err := json.Unmarshal(data, &a); err != nil || a.Name == "" {
			return nil
		}
		abstractName := strings.TrimSpace(a.Name)
		if len(abstractName) > maxAbstractNameLen {
			abstractName = abstractName[:maxAbstractNameLen]
		}
		desc := strings.TrimSpace(a.Description)
		if len(desc) > 500 {
			desc = desc[:500]
		}
		return &tagJudgmentAbstract{
			Name:        abstractName,
			Description: desc,
			Children:    filterChildren(a.Children),
			Reason:      a.Reason,
		}
	}

	// Parse merges (array format preferred, fallback to single)
	if len(raw.Merges) > 0 {
		for _, item := range raw.Merges {
			if merge := parseMerge(item); merge != nil {
				// Deduplicate: skip if target already used
				if usedLabels[merge.Target] {
					continue
				}
				judgment.Merges = append(judgment.Merges, *merge)
				usedLabels[merge.Target] = true
				for _, ch := range merge.Children {
					usedLabels[ch] = true
				}
			}
		}
	} else if len(raw.Merge) > 0 {
		if merge := parseMerge(raw.Merge); merge != nil {
			judgment.Merges = append(judgment.Merges, *merge)
			usedLabels[merge.Target] = true
			for _, ch := range merge.Children {
				usedLabels[ch] = true
			}
		}
	}

	// Parse abstracts (array format preferred, fallback to single)
	if len(raw.Abstracts) > 0 {
		for _, item := range raw.Abstracts {
			if abstract := parseAbstract(item); abstract != nil {
				// Deduplicate children across abstracts
				var dedupedChildren []string
				for _, ch := range abstract.Children {
					if !usedLabels[ch] {
						dedupedChildren = append(dedupedChildren, ch)
						usedLabels[ch] = true
					}
				}
				if len(dedupedChildren) > 0 {
					abstract.Children = dedupedChildren
					judgment.Abstracts = append(judgment.Abstracts, *abstract)
				}
			}
		}
	} else if len(raw.Abstract) > 0 {
		if abstract := parseAbstract(raw.Abstract); abstract != nil {
			var dedupedChildren []string
			for _, ch := range abstract.Children {
				if !usedLabels[ch] {
					dedupedChildren = append(dedupedChildren, ch)
					usedLabels[ch] = true
				}
			}
			if len(dedupedChildren) > 0 {
				abstract.Children = dedupedChildren
				judgment.Abstracts = append(judgment.Abstracts, *abstract)
			}
		}
	}

	// Parse none: only keep candidates that aren't already in merges or abstracts
	for _, label := range raw.None {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if !candidateLabels[label] {
			continue
		}
		if usedLabels[label] {
			continue
		}
		judgment.None = append(judgment.None, label)
		usedLabels[label] = true
	}

	// Cross-validation: any candidate not placed anywhere goes to none automatically
	for _, c := range candidates {
		if c.Tag != nil && !usedLabels[c.Tag.Label] {
			judgment.None = append(judgment.None, c.Tag.Label)
		}
	}

	return judgment, nil
}

func buildCandidateList(candidates []TagCandidate) string {
	var parts []string
	for _, c := range candidates {
		if c.Tag != nil {
			tagType := "normal"
			if c.Tag.Source == "abstract" {
				tagType = "abstract"
			}
			contextInfo := formatTagPromptContext(c.Tag)
			if c.DateRange != "" {
				if contextInfo != "" {
					contextInfo += "; "
				}
				contextInfo += c.DateRange
			}
			if contextInfo != "" {
				contextInfo = fmt.Sprintf(" (%s)", contextInfo)
			}
			pathInfo := ""
			if c.Tag.Source == "abstract" {
				depth := getTagDepthFromRoot(c.Tag.ID)
				if depth > 0 {
					pathStr := loadTagPathString(c.Tag.ID, 4)
					pathInfo = fmt.Sprintf(", depth=%d, path=%s", depth, pathStr)
				}
			}
			parts = append(parts, fmt.Sprintf("- %q (similarity: %.2f, type: %s, id=%d%s)%s", c.Tag.Label, c.Similarity, tagType, c.Tag.ID, pathInfo, contextInfo))
		}
	}
	return strings.Join(parts, "\n")
}

func shouldAllowMergeJudgment(candidates []TagCandidate) bool {
	for _, candidate := range candidates {
		if candidate.Tag != nil && candidate.Similarity >= mergeMinSimilarity {
			return true
		}
	}
	return false
}

func computeMaxCandidateDepth(candidates []TagCandidate) int {
	maxDepth := 0
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Source == "abstract" {
			d := getTagDepthFromRoot(c.Tag.ID)
			if d > maxDepth {
				maxDepth = d
			}
		}
	}
	return maxDepth
}

func buildTagJudgmentSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{Type: "object", Properties: map[string]airouter.SchemaProperty{
		"merges": {Type: "array", Description: "合并判断列表：仅当新标签与候选是完全同一概念时填写。父子、组织产品、生态、平台插件关系禁止 merge，应使用 abstracts。可以为空数组", Items: &airouter.SchemaProperty{
			Type: "object", Properties: map[string]airouter.SchemaProperty{
				"target":   {Type: "string", Description: "合并目标候选标签名称（必须是相似度≥0.85的候选）"},
				"label":    {Type: "string", Description: "合并后的统一名称"},
				"children": {Type: "array", Description: "与 target 完全同一概念的其他候选标签名称列表，不包括父子或生态相关标签", Items: &airouter.SchemaProperty{Type: "string"}},
				"reason":   {Type: "string", Description: "判断理由"},
			}, Required: []string{"target", "label", "children"},
		}},
		"abstracts": {Type: "array", Description: "抽象判断列表：需要为哪些候选创建抽象父标签。可以为空数组", Items: &airouter.SchemaProperty{
			Type: "object", Properties: map[string]airouter.SchemaProperty{
				"name":        {Type: "string", Description: "抽象标签名称（1-160字）"},
				"description": {Type: "string", Description: "抽象标签中文客观描述（500字以内）"},
				"children":    {Type: "array", Description: "应作为该抽象标签子标签的候选名称列表", Items: &airouter.SchemaProperty{Type: "string"}},
				"reason":      {Type: "string", Description: "判断理由"},
			}, Required: []string{"name", "description", "children"},
		}},
		"none": {Type: "array", Description: "与新标签无关或独立的候选标签名称列表。每个候选必须且只能出现在 merges、abstracts、none 其中一个数组中", Items: &airouter.SchemaProperty{Type: "string"}},
	}, Required: []string{"merges", "abstracts", "none"}}
}

func buildTagJudgmentPrompt(candidates []TagCandidate, newLabel string, category string, narrativeContext string, hasHighSimCandidate bool, maxCandidateDepth int) string {
	tagList := buildCandidateList(candidates)

	maxDepth := getMaxDepthForCategory(category)

	mergeRules := fmt.Sprintf(`
=== MERGES (very strict — same concept only) ===
- "target" MUST be the label of an EXISTING CANDIDATE TAG (from the list below). NEVER use the new tag's own label "%s" as target.
- "children" are other candidates that are ALSO the same concept (e.g. aliases, translations, different names for the same thing)
- Merge ONLY when the new tag and the target describe the EXACT SAME entity/event/person
- GOOD merges: "Tim Cook" ↔ "蒂姆·库克", "俄乌战争" ↔ "俄罗斯入侵乌克兰"
- NEVER merge parent/child, organization/product, ecosystem, platform/plugin, company/protocol, or brand/project relationships.
- For parent/child or ecosystem relationships, you MUST use abstracts or none; merge is always wrong.
- BAD merges (DO NOT): "GPT-5发布" with "DeepSeekV4发布" — different events even if same domain
- BAD merges (DO NOT): "Anthropic" with "Anthropic 协议" — organization/product relationship, not the same concept
- BAD merges (DO NOT): "伊朗核问题" with "美众议院法案" — completely different topics
- You may return MULTIPLE merges if there are multiple synonym groups, or empty array if no merges apply.`, newLabel)
	if hasHighSimCandidate {
		mergeRules += fmt.Sprintf(`
- The merge target MUST have similarity >= %.2f. Lower-similarity candidates should go to abstracts or be left out.`, mergeMinSimilarity)
	} else {
		mergeRules += fmt.Sprintf(`
- CAUTION: No candidate has similarity >= %.2f. Merge is very unlikely to be correct — only use it if you are absolutely certain the new tag and target are the exact same concept despite low embedding similarity.
- In most cases when no high-similarity candidate exists, abstracts or none is the correct answer.`, mergeMinSimilarity)
	}
	mergeShape := `  "merges": [ ... ],    // ONLY if the new tag IS THE SAME concept as candidates
  "abstracts": [ ... ], // ONLY if 2+ candidates are genuinely related but distinct
  "none": [ ... ]       // candidates that are unrelated or independent`

	abstractDepthWarning := ""
	if maxCandidateDepth >= 3 {
		abstractDepthWarning = fmt.Sprintf(`
- DEPTH LIMIT WARNING: one or more abstract candidates already have depth=%d (max allowed=%d).
  Do NOT create a new abstract that would nest deeper. If the candidates already belong to a deep hierarchy,
  prefer merge (if same concept) or use none. Creating a new abstract under deep candidates will be rejected.`,
			maxCandidateDepth, maxDepth)
	}

	rules := fmt.Sprintf(`
You are comparing a NEW tag against existing candidate tags. Decide if they are the same concept, related, or unrelated.

Return ONE JSON object with THREE arrays (all required, can be empty):
{
%s
}

Every candidate MUST appear in exactly ONE array. Do not omit any candidate.

%s

=== ABSTRACTS (related but distinct concepts — encouraged when appropriate) ===
- Create when the new tag + 1+ candidates share a DIRECT thematic connection
- This is DIFFERENT from merge: the concepts are related but NOT the same — they should be grouped under a broader parent
- "name" must be a concise specific category, NOT a vague label like "新闻"/"技术"/"人物"
- Good: "AI大模型" for GPT-5+Claude+Gemini, "航天产业" for SpaceX+Starlink, "中东局势" for 伊朗核问题+伊美谈判
- Bad: grouping unrelated events that merely happened around the same time
- When 2+ candidates are in the same domain/topic area and similarity >= 0.65, abstract is usually appropriate
- The hierarchy depth limit is %d levels. Do not create abstracts that would push beyond this limit.%s
- You may return MULTIPLE abstracts to group candidates into different categories (e.g., "航天AI技术" and "大模型基础设施" as separate groups)
- Each candidate should appear in at most ONE abstract group
- RECOMMENDED: When many candidates exist, create 2-4 focused abstract groups rather than one large group

=== NONE (unrelated or independent candidates) ===
- Candidates that have NO meaningful relationship with the new tag go here
- Candidates that are vaguely related but not enough for merge or abstract go here
- When in doubt, prefer none over forcing a weak relationship
- It is NORMAL and EXPECTED for many candidates to be in none — most candidates are not related
- Examples of none: different events in the same week, different people in the same broad industry, keywords from different domains

=== CRITICAL ===
- ALL three arrays (merges, abstracts, none) MUST be present in the response
- Every candidate label must appear in EXACTLY ONE array — no duplicates across arrays, no遗漏
- If candidates are clearly unrelated to the new tag AND to each other → all go in none
- Prefer multiple focused abstracts over one overly broad abstract
- Do NOT put candidates in merges just because they seem somewhat similar — merge requires SAME concept

Existing candidates:`, mergeShape, mergeRules, maxDepth, abstractDepthWarning)

	categoryRules := ""
	switch category {
	case "person":
		categoryRules = `
Person-specific rules:
- merge: ONLY when different names for the SAME person (e.g. "Tim Cook" and "蒂姆·库克")
- abstract: CREATE abstract tags for people who share affiliations, roles, domains, or national relevance
  - Examples: "中国科技领袖" for 雷军+余承东, "AI公司CEO" for Sam Altman+Satya Nadella, "美国政治人物" for Trump+Biden
  - People in the same industry, organization, or country's public sphere are related enough for abstract
  - Do NOT create overly vague tags like "人物" — add thematic specificity (e.g. "中国科技领袖" not "人物")`
	case "event":
		categoryRules = `
Event-specific rules:
- merge: ONLY when clearly the SAME event with different descriptions (same actors, same incident)
- abstract: CREATE abstract tags ONLY when events are causally connected or part of the SAME event chain
  - GOOD abstracts: "2024年大选" for multiple events in the same election, "中东冲突升级" for events in the same conflict
  - BAD abstracts (DO NOT create): grouping unrelated news events that merely happened around the same time
  - CRITICAL: "same time period" alone is NOT sufficient for grouping. Events must share a direct causal link or be clearly part of the same story arc
  - Do NOT group events that merely share a broad category like "international news" or "tech news"
  - If a candidate event involves completely different countries, organizations, or topics → put in none`
	default:
		categoryRules = `
Keyword-specific rules:
- merge: ONLY when tags are synonyms, translations, or different names for the EXACT SAME concept
- abstract: ONLY when keywords are directly related in a specific domain
  - Good: "前端框架" for React+Vue+Angular, "编程语言" for Python+Rust+Go
  - Bad: "技术" or "科技" as abstract for unrelated tech keywords
- Do NOT merge keywords that merely share a broad field (e.g. "AI" and "云计算" are different topics)
- When in doubt, put in none`
	}

	prompt := fmt.Sprintf(`%s
%s

>>> NEW TAG TO CLASSIFY: "%s" (category: %s)

%s

Respond with JSON now.`, rules, tagList, newLabel, category, categoryRules)

	if narrativeContext != "" {
		prompt += fmt.Sprintf("\n\nAdditional context from narrative analysis:\n%s\nUse this context to help determine relationships.", narrativeContext)
	}

	return prompt
}

func injectHierarchyContext(prompt string, category, subType string) string {
	mgr := GetHierarchyManager()
	if mgr == nil {
		return prompt
	}
	tmpl := mgr.GetTemplate(category, subType)
	if tmpl == nil {
		return prompt
	}

	var sb strings.Builder
	sb.WriteString(prompt)
	sb.WriteString("\n\n=== HIERARCHY LEVEL GUIDANCE ===\n")
	fmt.Fprintf(&sb, "Template: %s (%d levels)\n", tmpl.TemplateKey(), tmpl.MaxLevel)
	sb.WriteString("Level definitions:\n")
	for _, l := range tmpl.Levels {
		fmt.Fprintf(&sb, "  L%d: %s — %s", l.Level, l.Name, l.Description)
		if l.IsLeaf {
			sb.WriteString(" (LEAF)")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nWhen creating abstracts, ensure the abstract name fits the appropriate level's semantics.\n")
	fmt.Fprintf(&sb, "Abstract tags should be L2 or L1 level. Newly created child tags default to L%d (leaf).\n", tmpl.GetLeafLevel())

	return sb.String()
}

func findSimilarExistingAbstract(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
	es := NewEmbeddingService()
	thresholds := es.GetThresholds()
	probe := &models.TopicTag{
		Label:       name,
		Description: desc,
		Category:    category,
		Source:      "abstract",
	}
	similar, err := es.FindSimilarTags(ctx, probe, category, 8, EmbeddingTypeSemantic)
	if err != nil {
		logging.Warnf("findSimilarExistingAbstract: embedding search failed: %v", err)
		return nil
	}

	existingAbstracts := make([]models.TopicTag, 0, len(similar))
	for _, candidate := range similar {
		if candidate.Tag == nil || candidate.Tag.Source != "abstract" {
			continue
		}
		if candidate.Similarity < thresholds.LowSimilarity {
			continue
		}
		existingAbstracts = append(existingAbstracts, *candidate.Tag)
		if len(existingAbstracts) == 5 {
			break
		}
	}
	if len(existingAbstracts) == 0 {
		return nil
	}

	candidateLabels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Tag != nil {
			candidateLabels = append(candidateLabels, candidate.Tag.Label)
		}
	}

	router := airouter.NewRouter()
	abstractInfo := make([]string, 0, len(existingAbstracts))
	for _, existing := range existingAbstracts {
		children := loadAbstractChildLabels(existing.ID, 5)
		abstractInfo = append(abstractInfo,
			fmt.Sprintf("- ID %d: %q (%s, 子标签: %s)", existing.ID, existing.Label, formatTagPromptContext(&existing), formatChildLabels(children)))
	}

	prompt := fmt.Sprintf(`一个新的抽象标签即将被创建，请检查以下已有抽象标签中是否有描述同一概念的。

即将创建的抽象标签: %q (描述: %s)
其候选子标签: %s

已有同 category 抽象标签:
%s

规则:
- 只有当已有标签描述的核心概念与新标签完全相同时才返回
- 不要把宽泛程度不同的标签当作同一概念

返回 JSON: {"reuse_id": 0 或 已有标签的ID, "reason": "简要说明"}`,
		name, TruncateStr(desc, 200), formatChildLabels(candidateLabels), strings.Join(abstractInfo, "\n"))

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"reuse_id": {Type: "integer", Description: "复用的已有标签ID，0表示不匹配"},
				"reason":   {Type: "string", Description: "判断理由"},
			},
			Required: []string{"reuse_id", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "find_similar_existing_abstract",
			"new_name":     name,
			"category":     category,
			"candidates_n": len(candidates),
			"shortlist_n":  len(existingAbstracts),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		logging.Warnf("findSimilarExistingAbstract: LLM call failed: %v", err)
		return nil
	}

	var parsed struct {
		ReuseID uint   `json:"reuse_id"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		logging.Warnf("findSimilarExistingAbstract: parse failed: %v", err)
		return nil
	}
	if parsed.ReuseID == 0 {
		return nil
	}

	for i := range existingAbstracts {
		if existingAbstracts[i].ID == parsed.ReuseID {
			logging.Infof("findSimilarExistingAbstract: found match %d (%q) for new %q: %s",
				existingAbstracts[i].ID, existingAbstracts[i].Label, name, parsed.Reason)
			return &existingAbstracts[i]
		}
	}

	logging.Warnf("findSimilarExistingAbstract: reuse_id %d not found in shortlist", parsed.ReuseID)
	return nil
}

func batchJudgeNarrowerConcepts(ctx context.Context, parentTag *models.TopicTag, candidates []TagCandidate) ([]uint, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	parentChildren := loadAbstractChildLabels(parentTag.ID, 5)

	var entries []string
	for i, c := range candidates {
		childLabels := loadAbstractChildLabels(c.Tag.ID, 5)
		entries = append(entries, fmt.Sprintf("%d. %q (%s, 子标签: %s, 相似度: %.4f)",
			i+1, c.Tag.Label, formatTagPromptContext(c.Tag), formatChildLabels(childLabels), c.Similarity))
	}

	prompt := fmt.Sprintf(`判断以下候选标签中哪些是目标标签的更窄（更具体）概念，应该作为其子标签。

目标标签（潜在的父标签）: %q (%s)
目标标签的子标签: %s

候选标签:
%s

规则:
- 如果候选标签描述的是目标标签范围内的一个具体方面、子集或特定场景，则它是更窄概念
- 如果两者是同一层级或候选更宽泛，不选
- 如果候选的子标签与目标的子标签高度重叠，说明是同一概念，不选
- 可以选择零个、一个或多个

返回 JSON: {"narrower_ids": [选中候选的编号列表], "reasons": {"编号": "简要说明"}}`,
		parentTag.Label, formatTagPromptContext(parentTag), formatChildLabels(parentChildren),
		strings.Join(entries, "\n"))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"narrower_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}, Description: "选中为更窄概念的候选编号列表"},
				"reasons":      {Type: "object", Description: "每个选中候选的判断理由"},
			},
			Required: []string{"narrower_ids"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":       "adopt_narrower_abstract_batch",
			"parent_tag":      parentTag.ID,
			"candidate_count": len(candidates),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		NarrowerIDs []int             `json:"narrower_ids"`
		Reasons     map[string]string `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	logging.Infof("batchJudgeNarrowerConcepts: parent=%d, candidates=%d, selected_ids=%v", parentTag.ID, len(candidates), parsed.NarrowerIDs)

	var ids []uint
	for _, idx := range parsed.NarrowerIDs {
		if idx >= 1 && idx <= len(candidates) {
			ids = append(ids, candidates[idx-1].Tag.ID)
		}
	}
	return ids, nil
}

type abstractRelationJudgment struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type batchAbstractRelationResult struct {
	CandidateIndex int
	TagID          uint
	Action         string
	Target         string
	Reason         string
}

func normalizeAbstractRelationJudgment(judgment *abstractRelationJudgment) error {
	if judgment == nil {
		return fmt.Errorf("empty abstract relation judgment")
	}
	action := strings.ToLower(strings.TrimSpace(judgment.Action))
	judgment.Target = strings.ToUpper(strings.TrimSpace(judgment.Target))
	judgment.Reason = strings.TrimSpace(judgment.Reason)

	switch action {
	case "merge":
		judgment.Action = "merge"
		if judgment.Target != "A" && judgment.Target != "B" {
			return fmt.Errorf("invalid merge target %q", judgment.Target)
		}
	case "parent_a":
		judgment.Action = "parent_A"
		if judgment.Target != "A" && judgment.Target != "B" {
			judgment.Target = ""
		}
	case "parent_b":
		judgment.Action = "parent_B"
		if judgment.Target != "A" && judgment.Target != "B" {
			judgment.Target = ""
		}
	case "skip":
		judgment.Action = "skip"
		if judgment.Target != "A" && judgment.Target != "B" {
			judgment.Target = ""
		}
	default:
		return fmt.Errorf("invalid abstract relation action %q", strings.TrimSpace(judgment.Action))
	}
	return nil
}

func batchJudgeAbstractRelationships(ctx context.Context, tagA *models.TopicTag, candidates []TagCandidate) ([]batchAbstractRelationResult, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	childrenA := loadAbstractChildLabels(tagA.ID, 8)

	var entries []string
	for i, c := range candidates {
		childLabels := loadAbstractChildLabels(c.Tag.ID, 8)
		entries = append(entries, fmt.Sprintf("%d. %q (%s, 子标签: %s, 相似度: %.4f)",
			i+1, c.Tag.Label, formatTagPromptContext(c.Tag), formatChildLabels(childLabels), c.Similarity))
	}

	prompt := fmt.Sprintf(`给定抽象标签 A 和以下候选标签列表，判断每对 (A, 候选) 的关系。

标签 A: %q (%s)
标签 A 的子标签: %s

候选标签:
%s

对每个候选，从以下动作中选择一个:
- "merge": 两者描述完全相同的概念（同义词、翻译、不同措辞的同一概念）。在 target 中指定保留哪个（A 或候选编号）。
- "parent_A": 标签 A 是更宽泛的概念，该候选是 A 的具体子概念。
- "parent_B": 该候选是更宽泛的概念，标签 A 是候选的具体子概念。
- "skip": 两者看起来相似但不应该关联（不同领域、不同地区、不相关主题）。

规则:
- 使用子标签来理解每个抽象标签实际涵盖的范围
- "merge" 仅当两者确实是同一概念时使用——不仅仅是相关或重叠
- "skip" 当子标签显示它们涵盖完全不同的领域或地区时使用
- 对于 parent/child，父标签应该是更宽泛的概念
- 如果两者同样宽泛但相关，优先选择 "skip" 而不是强行建立关系

返回 JSON: {"judgments": [{"index": 候选编号, "action": "merge"|"parent_A"|"parent_B"|"skip", "target": "A"|"<候选编号>"|"", "reason": "简要说明"}]}`,
		tagA.Label, formatTagPromptContext(tagA), formatChildLabels(childrenA),
		strings.Join(entries, "\n"))

	prompt = injectHierarchyContext(prompt, tagA.Category, tagA.SubType)

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"judgments": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"index":  {Type: "integer", Description: "候选编号"},
							"action": {Type: "string", Description: "merge, parent_A, parent_B, 或 skip"},
							"target": {Type: "string", Description: "A 或候选编号，merge 时保留的标签；其他动作时也需要填写"},
							"reason": {Type: "string", Description: "判断理由"},
						},
						Required: []string{"index", "action", "target", "reason"},
					},
					Description: "每个候选的判断结果",
				},
			},
			Required: []string{"judgments"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":       "judge_abstract_relationship_batch",
			"tag_a":           tagA.ID,
			"candidate_count": len(candidates),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		Judgments []struct {
			Index  int    `json:"index"`
			Action string `json:"action"`
			Target string `json:"target"`
			Reason string `json:"reason"`
		} `json:"judgments"`
	}
	if err := json.Unmarshal([]byte(jsonutil.SanitizeLLMJSON(result.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	logging.Infof("batchJudgeAbstractRelationships: tagA=%d, candidates=%d, judgments=%d", tagA.ID, len(candidates), len(parsed.Judgments))

	var results []batchAbstractRelationResult
	for _, j := range parsed.Judgments {
		idx := j.Index - 1
		if idx < 0 || idx >= len(candidates) {
			logging.Warnf("batchJudgeAbstractRelationships: invalid index %d (max %d), skipping", j.Index, len(candidates))
			continue
		}
		normalized := abstractRelationJudgment{
			Action: j.Action,
			Target: j.Target,
			Reason: j.Reason,
		}
		if err := normalizeAbstractRelationJudgment(&normalized); err != nil {
			logging.Warnf("batchJudgeAbstractRelationships: invalid judgment for index %d: %v, treating as skip", j.Index, err)
			results = append(results, batchAbstractRelationResult{
				CandidateIndex: idx,
				TagID:          candidates[idx].Tag.ID,
				Action:         "skip",
				Reason:         fmt.Sprintf("invalid judgment normalized to skip: %v", err),
			})
			continue
		}
		results = append(results, batchAbstractRelationResult{
			CandidateIndex: idx,
			TagID:          candidates[idx].Tag.ID,
			Action:         normalized.Action,
			Target:         normalized.Target,
			Reason:         normalized.Reason,
		})
	}
	return results, nil
}

func judgeCrossLayerDuplicate(ctx context.Context, sourceID uint, candidateID uint) (bool, string, error) {
	if database.DB == nil {
		return false, "", fmt.Errorf("database not initialized")
	}

	var sourceTag models.TopicTag
	if err := database.DB.First(&sourceTag, sourceID).Error; err != nil {
		return false, "", err
	}
	var candidateTag models.TopicTag
	if err := database.DB.First(&candidateTag, candidateID).Error; err != nil {
		return false, "", err
	}

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`请判断以下两个标签是否描述同一概念，并且应该合并。

标签 A: %q
- 上下文: %s
- 层级路径: %s
- 子标签: %s

标签 B: %q
- 上下文: %s
- 层级路径: %s
- 子标签: %s

规则:
- 只回答是否应合并，不要讨论层级摆放
- 只有在两个标签的核心概念相同、保留为两个节点会造成重复时，才返回 should_merge=true

返回 JSON: {"should_merge": true/false, "reason": "简要说明"}`,
		sourceTag.Label, formatTagPromptContext(&sourceTag), loadTagPathString(sourceID, 6), formatChildLabels(loadAbstractChildLabels(sourceID, 5)),
		candidateTag.Label, formatTagPromptContext(&candidateTag), loadTagPathString(candidateID, 6), formatChildLabels(loadAbstractChildLabels(candidateID, 5)))

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"should_merge": {Type: "boolean", Description: "是否应合并"},
				"reason":       {Type: "string", Description: "判断理由"},
			},
			Required: []string{"should_merge", "reason"},
		},
		Temperature: func() *float64 { f := 0.1; return &f }(),
		Metadata: map[string]any{
			"operation":    "judge_cross_layer_duplicate",
			"source_id":    sourceID,
			"candidate_id": candidateID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return false, "", fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		ShouldMerge bool   `json:"should_merge"`
		Reason      string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return false, "", fmt.Errorf("parse response: %w", err)
	}

	return parsed.ShouldMerge, parsed.Reason, nil
}

func mergeOrLinkSimilarAbstract(ctx context.Context, tag1ID, tag2ID uint) error {
	var tag1, tag2 models.TopicTag
	if err := database.DB.First(&tag1, tag1ID).Error; err != nil {
		return fmt.Errorf("load tag %d: %w", tag1ID, err)
	}
	if err := database.DB.First(&tag2, tag2ID).Error; err != nil {
		return fmt.Errorf("load tag %d: %w", tag2ID, err)
	}

	children1 := loadAbstractChildLabels(tag1ID, 5)
	children2 := loadAbstractChildLabels(tag2ID, 5)

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`两个抽象标签非常相似，请判断它们的关系。

标签 A: %q (%s)
A 的子标签: %s

标签 B: %q (%s)
B 的子标签: %s

判断:
- 如果它们描述的是完全相同的概念（只是表述不同），返回 "merge"
- 如果 A 是 B 的上位概念（更宽泛），返回 "parent_A"
- 如果 B 是 A 的上位概念（更宽泛），返回 "parent_B"

返回 JSON: {"action": "merge"|"parent_A"|"parent_B", "target": "A"|"B", "reason": "简要说明"}`,
		tag1.Label, formatTagPromptContext(&tag1), formatChildLabels(children1),
		tag2.Label, formatTagPromptContext(&tag2), formatChildLabels(children2))

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"action": {Type: "string", Description: "merge, parent_A, 或 parent_B"},
				"target": {Type: "string", Description: "A 或 B，merge 时保留的标签"},
				"reason": {Type: "string", Description: "判断理由"},
			},
			Required: []string{"action", "target", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "merge_or_link_similar_abstract",
			"tag_a":     tag1ID,
			"tag_b":     tag2ID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		Action string `json:"action"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return fmt.Errorf("parse LLM response: %w", err)
	}

	switch parsed.Action {
	case "merge":
		sourceID, targetID := tag1ID, tag2ID
		sourceLabel, targetLabel := tag1.Label, tag2.Label
		if strings.EqualFold(parsed.Target, "A") {
			sourceID, targetID = tag2ID, tag1ID
			sourceLabel, targetLabel = tag2.Label, tag1.Label
		}
		logging.Infof("mergeOrLinkSimilarAbstract: merging %d (%s) into %d (%s), reason: %s",
			sourceID, sourceLabel, targetID, targetLabel, parsed.Reason)
		return MergeTags(sourceID, targetID)
	case "parent_A":
		return linkAbstractParentChild(tag2ID, tag1ID)
	case "parent_B":
		return linkAbstractParentChild(tag1ID, tag2ID)
	default:
		return fmt.Errorf("unknown action %q from LLM", parsed.Action)
	}
}

func reparentOrLinkAbstractChild(ctx context.Context, childID, newParentID uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		wouldCycle, err := wouldCreateCycle(tx, newParentID, childID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", newParentID, childID)
		}

		var count int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ? AND relation_type = ?", newParentID, childID, "abstract").
			Count(&count)
		if count > 0 {
			return nil
		}

		var oldParentRelation models.TopicTagRelation
		if err := tx.Where("child_id = ? AND relation_type = ?", childID, "abstract").First(&oldParentRelation).Error; err == nil {
			logging.Infof("reparentOrLinkAbstractChild: child %d already has parent %d, skipping (defer to scheduled cleanup)", childID, oldParentRelation.ParentID)
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find old parent: %w", err)
		}

		var targetTag, candidateTag models.TopicTag
		if err := tx.First(&targetTag, newParentID).Error; err != nil {
			return fmt.Errorf("load target tag %d: %w", newParentID, err)
		}
		if err := tx.First(&candidateTag, childID).Error; err != nil {
			return fmt.Errorf("load candidate tag %d: %w", childID, err)
		}

		childSubtreeDepth := getAbstractSubtreeDepth(tx, childID)
		parentAncestryDepth := getTagDepthFromRootDB(tx, newParentID)
		maxDepth := getMaxDepthForCategory(targetTag.Category)
		if childSubtreeDepth+parentAncestryDepth+1 > maxDepth {
			return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth %d", childSubtreeDepth, parentAncestryDepth, maxDepth)
		}

		targetLevel := getTagDepthFromRoot(targetTag.ID)
		candidateLevel := getTagDepthFromRoot(candidateTag.ID)
		if targetLevel != candidateLevel {
			logging.Infof("Adopt narrower: skip tag %d (L%d) adopting %d (L%d) — cross-level",
				targetTag.ID, targetLevel, candidateTag.ID, candidateLevel)
			return fmt.Errorf("cross-level skip: target L%d != candidate L%d", targetLevel, candidateLevel)
		}

		relation := models.TopicTagRelation{
			ParentID:     newParentID,
			ChildID:      childID,
			RelationType: "abstract",
		}
		return tx.Create(&relation).Error
	})
}
