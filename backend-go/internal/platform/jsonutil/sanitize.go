package jsonutil

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

func SanitizeLLMJSON(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if !utf8.ValidString(content) {
		content = strings.ToValidUTF8(content, "�")
	}

	if !json.Valid([]byte(content)) {
		content = extractJSON(content)
	}

	if !json.Valid([]byte(content)) {
		content = fixUnescapedQuotes(content)
	}

	if !json.Valid([]byte(content)) {
		content = fixTruncatedJSONArray(content)
	}

	return content
}

func extractJSON(content string) string {
	if strings.HasPrefix(content, "{") {
		return content
	}
	if strings.HasPrefix(content, "[") {
		return content
	}

	arrStart := strings.Index(content, "[")
	arrEnd := strings.LastIndex(content, "]")
	objStart := strings.Index(content, "{")
	objEnd := strings.LastIndex(content, "}")

	if arrStart >= 0 && arrEnd > arrStart {
		return strings.TrimSpace(content[arrStart : arrEnd+1])
	}
	if objStart >= 0 && objEnd > objStart {
		return strings.TrimSpace(content[objStart : objEnd+1])
	}

	return content
}

func fixUnescapedQuotes(content string) string {
	for attempt := 0; attempt < 10; attempt++ {
		if json.Valid([]byte(content)) {
			return content
		}
		fixed := escapeInnerQuotes(content)
		if fixed == content {
			break
		}
		content = fixed
	}
	return content
}

func escapeInnerQuotes(content string) string {
	var sb strings.Builder
	sb.Grow(len(content) + len(content)/10)

	inString := false
	i := 0

	for i < len(content) {
		c := content[i]

		if !inString {
			sb.WriteByte(c)
			if c == '"' {
				inString = true
			}
			i++
			continue
		}

		if c == '\\' && i+1 < len(content) {
			sb.WriteByte(c)
			sb.WriteByte(content[i+1])
			i += 2
			continue
		}

		if c == '"' {
			rest := content[i+1:]
			trimmed := strings.TrimLeft(rest, " \t\n\r")

			if len(trimmed) == 0 {
				sb.WriteByte(c)
				i++
				continue
			}

			switch trimmed[0] {
			case ':', ',', '}', ']':
				sb.WriteByte(c)
				inString = false
				i++
				continue
			}

			sb.WriteByte('\\')
			sb.WriteByte('"')
			i++
			continue
		}

		sb.WriteByte(c)
		i++
	}

	return sb.String()
}

func fixTruncatedJSONArray(content string) string {
	if !strings.HasPrefix(content, "[") {
		return content
	}
	if strings.HasSuffix(strings.TrimSpace(content), "]") {
		return content
	}

	lastBrace := strings.LastIndex(content, "}")
	if lastBrace < 0 {
		return content
	}

	truncated := strings.TrimSpace(content[:lastBrace+1])

	openBrackets := 0
	for _, c := range truncated {
		switch c {
		case '[':
			openBrackets++
		case ']':
			openBrackets--
		}
	}
	for i := 0; i < openBrackets; i++ {
		truncated += "]"
	}

	return truncated
}
