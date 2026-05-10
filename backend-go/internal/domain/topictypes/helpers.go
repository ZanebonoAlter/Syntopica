package topictypes

import (
	"regexp"
	"strings"
)

var punctuationPattern = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
var spacePattern = regexp.MustCompile(`\s+`)

func Slugify(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = spacePattern.ReplaceAllString(clean, " ")
	clean = punctuationPattern.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")
	return clean
}

func NormalizeDisplayCategory(kind string, fallback string) string {
	switch kind {
	case "topic":
		return "event"
	case "entity":
		return "person"
	case "keyword":
		return "keyword"
	}

	switch fallback {
	case "topic":
		return "event"
	case "entity":
		return "person"
	case "event", "person", "keyword":
		return fallback
	default:
		return "keyword"
	}
}

func NormalizeTopicKind(kind string, category string) string {
	switch kind {
	case "topic", "entity", "keyword":
		return kind
	}

	switch category {
	case "event":
		return "topic"
	case "person":
		return "entity"
	default:
		return "keyword"
	}
}

func DedupeTagsWithCategory(items []TopicTag) []TopicTag {
	best := make(map[string]TopicTag)
	for _, item := range items {
		if item.Slug == "" {
			item.Slug = Slugify(item.Label)
		}
		if item.Category == "" {
			item.Category = "keyword"
		}
		key := item.Category + ":" + item.Slug
		current, exists := best[key]
		if !exists || current.Score < item.Score {
			best[key] = item
		}
	}

	result := make([]TopicTag, 0, len(best))
	for _, item := range best {
		result = append(result, item)
	}

	return result
}

func DedupeTopics(items []TopicTag) []TopicTag {
	return DedupeTagsWithCategory(items)
}
