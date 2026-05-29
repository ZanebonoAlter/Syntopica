package tagging

import (
	"regexp"
	"sort"
	"strings"
)

type topicRule struct {
	Label    string
	Slug     string
	Kind     string
	Patterns []string
	Score    float64
}

var topicRules = []topicRule{
	{Label: "AI Agent", Slug: "ai-agent", Kind: "topic", Score: 1.3, Patterns: []string{"ai agent", "agents", "智能体", "agentic"}},
	{Label: "OpenAI", Slug: "openai", Kind: "entity", Score: 1.4, Patterns: []string{"openai", "open ai"}},
	{Label: "Anthropic", Slug: "anthropic", Kind: "entity", Score: 1.3, Patterns: []string{"anthropic", "claude"}},
	{Label: "Multimodal", Slug: "multimodal", Kind: "topic", Score: 1.1, Patterns: []string{"multimodal", "多模态"}},
	{Label: "Coding", Slug: "coding", Kind: "topic", Score: 0.9, Patterns: []string{"coding", "code", "编程", "开发"}},
	{Label: "Infra", Slug: "infra", Kind: "topic", Score: 0.8, Patterns: []string{"infra", "infrastructure", "算力", "基础设施"}},
	{Label: "NVIDIA", Slug: "nvidia", Kind: "entity", Score: 1.3, Patterns: []string{"nvidia"}},
}

var productTokenPattern = regexp.MustCompile(`\b(?:GPT-\d+(?:\.\d+)?|Claude\s?\d+(?:\.\d+)?|Gemini\s?[A-Z0-9.-]+|Llama\s?\d+(?:\.\d+)?)\b`)
var heuristicPunctPattern = regexp.MustCompile(`[\s\p{P}]+`)

func ExtractTopics(input ExtractionInput) []TopicTag {
	combined := strings.ToLower(strings.Join([]string{input.Title, input.Summary, input.FeedName, input.CategoryName}, " "))
	scores := make(map[string]TopicTag)

	for _, rule := range topicRules {
		matches := 0
		for _, pattern := range rule.Patterns {
			matches += strings.Count(combined, strings.ToLower(pattern))
		}
		if matches == 0 {
			continue
		}

		scores[rule.Slug] = TopicTag{
			Label: rule.Label,
			Slug:  rule.Slug,
			Kind:  rule.Kind,
			Score: rule.Score + float64(matches-1)*0.15,
		}
	}

	for _, match := range productTokenPattern.FindAllString(input.Title+" "+input.Summary, -1) {
		label := strings.TrimSpace(match)
		slug := slugify(label)
		if slug == "" {
			continue
		}
		current, ok := scores[slug]
		if !ok || current.Score < 1.15 {
			scores[slug] = TopicTag{Label: label, Slug: slug, Kind: "entity", Score: 1.15}
		}
	}

	feedTag := extractFeedEntity(input.FeedName)
	if feedTag != nil {
		current, exists := scores[feedTag.Slug]
		if !exists || current.Score < feedTag.Score {
			scores[feedTag.Slug] = *feedTag
		}
	}

	if category := strings.TrimSpace(input.CategoryName); category != "" {
		slug := slugify(category)
		if slug != "" {
			if _, exists := scores[slug]; !exists {
				scores[slug] = TopicTag{Label: category, Slug: slug, Kind: "topic", Score: 0.65}
			}
		}
	}

	result := make([]TopicTag, 0, len(scores))
	for _, topic := range scores {
		result = append(result, topic)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Label < result[j].Label
		}
		return result[i].Score > result[j].Score
	})

	if len(result) > 8 {
		return result[:8]
	}

	return result
}

func extractFeedEntity(feedName string) *TopicTag {
	clean := strings.TrimSpace(feedName)
	if clean == "" {
		return nil
	}

	upper := regexp.MustCompile(`\b[A-Z][A-Za-z0-9.-]{2,}\b`).FindString(clean)
	if upper == "" {
		return nil
	}

	return &TopicTag{
		Label: upper,
		Slug:  slugify(upper),
		Kind:  "entity",
		Score: 0.85,
	}
}

func slugify(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = heuristicPunctPattern.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")
	return clean
}
