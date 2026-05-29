package airouter

import "testing"

func TestStripThinkTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no think tags",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "simple think tags",
			input: "<think\n>\nLet me think about this\n</think\n>\nThe answer is 42",
			want:  "The answer is 42",
		},
		{
			name:  "think tags with multiline content",
			input: "<think\n>\nStep 1: analyze\nStep 2: reason\nStep 3: conclude\n</think\n>\nFinal answer",
			want:  "Final answer",
		},
		{
			name:  "only think content no trailing text",
			input: "<think\n>\nJust thinking\n</think\n>",
			want:  "",
		},
		{
			name:  "think tags with extra whitespace in tags",
			input: "<think  >\nreasoning\n</think  >\nresult",
			want:  "result",
		},
		{
			name:  "multiple think blocks",
			input: "<think\n>first</think\n>middle<think\n>second</think\n>end",
			want:  "middleend",
		},
		{
			name:  "empty think tags",
			input: "<think\n></think\n>content",
			want:  "content",
		},
		{
			name:  "think with code blocks inside",
			input: "<think\n>\n```go\nfmt.Println(\"hi\")\n```\n</think\n>\nHere is the answer",
			want:  "Here is the answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkTags(tt.input)
			if got != tt.want {
				t.Errorf("stripThinkTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
