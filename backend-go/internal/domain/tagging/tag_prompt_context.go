package tagging

import (
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
)

func formatTagPromptContext(tag *models.TopicTag) string {
	if tag == nil {
		return ""
	}

	parts := make([]string, 0, 2)
	if tag.Description != "" {
		parts = append(parts, fmt.Sprintf("描述: %s", TruncateStr(tag.Description, 200)))
	}
	if attrs := formatPersonAttrs(tag.Metadata); tag.Category == models.TagCategoryPerson && attrs != "" {
		parts = append(parts, attrs)
	}

	return strings.Join(parts, "; ")
}

func formatPersonAttrs(metadata models.MetadataMap) string {
	if len(metadata) == 0 {
		return ""
	}

	attrs := make([]string, 0, 4)
	if value := metadataString(metadata, "country"); value != "" {
		attrs = append(attrs, "国籍/地区: "+value)
	}
	if value := metadataString(metadata, "organization"); value != "" {
		attrs = append(attrs, "组织: "+value)
	}
	if value := metadataString(metadata, "role"); value != "" {
		attrs = append(attrs, "身份/职务: "+value)
	}
	if domains := metadataStringList(metadata, "domains"); len(domains) > 0 {
		attrs = append(attrs, "领域: "+strings.Join(domains, ", "))
	}
	if len(attrs) == 0 {
		return ""
	}

	return "属性: " + strings.Join(attrs, " | ")
}

func metadataString(metadata models.MetadataMap, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func metadataStringList(metadata models.MetadataMap, key string) []string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return nil
	}

	var values []string
	switch typed := value.(type) {
	case []string:
		values = typed
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
	}

	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
