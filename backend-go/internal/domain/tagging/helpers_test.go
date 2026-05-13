package tagging

import (
	"testing"
)

func TestSlugify_WhitespaceNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple consecutive spaces collapse to one",
			input:    "foo   bar",
			expected: "foo bar",
		},
		{
			name:     "leading and trailing whitespace trimmed",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "single space preserved between words",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "label without whitespace stays same",
			input:    "DeepSeek首轮融资",
			expected: "deepseek首轮融资",
		},
		{
			name:     "label with single space in Chinese context",
			input:    "DeepSeek 首轮融资",
			expected: "deepseek 首轮融资",
		},
		{
			name:     "punctuation replaced with dash, spaces preserved",
			input:    "hello, world!",
			expected: "hello- world",
		},
		{
			name:     "mixed spaces and punctuation, consecutive punctuation merged",
			input:    "a  b,  c!",
			expected: "a b- c",
		},
		{
			name:     "tabs and newlines collapsed",
			input:    "foo\tbar\nbaz",
			expected: "foo bar baz",
		},
		{
			name:     "trailing punctuation trimmed",
			input:    "hello-",
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Slugify(tt.input)
			if result != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
