package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

func TestFingerprintRequestPromptAndParameters(t *testing.T) {
	req, err := fingerprintRequest("test-session", models.TopicTag{})
	if err != nil {
		t.Fatal(err)
	}
	if req.Operation != "topicgraph.backfill_fingerprint" || req.SessionID != "test-session" || req.Capability != airouter.CapabilityTopicTagging {
		t.Fatalf("unexpected routing: %+v", req)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 1024 || req.Temperature == nil || *req.Temperature != 0.2 || !req.JSONMode || req.JSONSchema != nil {
		t.Fatalf("unexpected parameters: %+v", req)
	}
	if req.Metadata["prompt_version"] != "4" || len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("unexpected prompt contract: %+v", req)
	}
	prompt := req.Messages[0].Content
	if !strings.HasPrefix(prompt, "直接输出 JSON 对象") {
		t.Fatalf("prompt must be direct: %s", prompt)
	}
	for _, text := range []string{"不要推理过程", "不要解释", "不要 markdown 代码块", "subject：", "subject_key：", "action：", "object：", "occurred_at：", "person/company/org/product/other", "无明确日期填 \"\"", "不得使用当前日期", "只输出这个 JSON 对象，字段名不得更改", "只依据输入 label/description/event_keywords/aliases", "不得编造", "拿不准的字段填空字符串，不要为了完整而猜测"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("prompt missing %q", text)
		}
	}
}

func TestFingerprintSystemPromptJSONExample(t *testing.T) {
	prompt := fingerprintSystemPrompt()
	for _, prefix := range []string{"示例输入：", "示例输出："} {
		_, rest, found := strings.Cut(prompt, prefix)
		if !found {
			t.Fatalf("prompt missing %q", prefix)
		}
		example, _, _ := strings.Cut(rest, "\n")
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(example), &fields); err != nil {
			t.Fatalf("invalid JSON for %s: %v", prefix, err)
		}
		if prefix == "示例输出：" {
			if len(fields) != 5 {
				t.Fatalf("expected five output fields: %s", example)
			}
			for _, name := range []string{"subject", "subject_key", "action", "object", "occurred_at"} {
				var value string
				if err := json.Unmarshal(fields[name], &value); err != nil || value == "" {
					t.Fatalf("invalid example field %s: %s", name, example)
				}
				if name == "subject_key" && value != "person:特朗普" {
					t.Fatalf("subject_key must include type and canonical name: %q", value)
				}
			}
		}
	}
}

func TestFingerprintRequestClipsInput(t *testing.T) {
	for _, count := range []int{0, 2, 10, 20, 25} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			keywords, aliases := make([]string, count), make([]string, count)
			for i := range count {
				keywords[i], aliases[i] = fmt.Sprintf("关键词%d", i), fmt.Sprintf("别名%d", i)
			}
			aliasJSON, err := json.Marshal(aliases)
			if err != nil {
				t.Fatal(err)
			}
			description := strings.Repeat("文🙂", count*5)
			tag := models.TopicTag{Label: "主体发布产品", Description: description, Aliases: string(aliasJSON), Metadata: models.MetadataMap{"event_keywords": keywords, "private": "DO_NOT_SEND"}}
			req, err := fingerprintRequest("test", tag)
			if err != nil {
				t.Fatal(err)
			}
			var input map[string]json.RawMessage
			if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
				t.Fatal(err)
			}
			if len(input) != 4 || strings.Contains(req.Messages[1].Content, "DO_NOT_SEND") {
				t.Fatalf("unexpected fields: %s", req.Messages[1].Content)
			}
			var gotLabel, gotDescription string
			var gotKeywords, gotAliases []string
			for name, dest := range map[string]any{"label": &gotLabel, "description": &gotDescription, "event_keywords": &gotKeywords, "aliases": &gotAliases} {
				if err := json.Unmarshal(input[name], dest); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			}
			wantDescription := []rune(description)
			if gotLabel != tag.Label || gotDescription != string(wantDescription[:min(len(wantDescription), 200)]) || !utf8.ValidString(gotDescription) {
				t.Fatalf("unexpected label/description: %q %q", gotLabel, gotDescription)
			}
			if len(gotKeywords) != min(count, 20) || len(gotAliases) != min(count, 10) {
				t.Fatalf("unexpected list lengths: %d %d", len(gotKeywords), len(gotAliases))
			}
			if count > 0 && (!reflect.DeepEqual(gotKeywords, keywords[:min(count, 20)]) || !reflect.DeepEqual(gotAliases, aliases[:min(count, 10)])) {
				t.Fatal("list contents/order changed")
			}
			if tag.Description != description || len(tag.Metadata["event_keywords"].([]string)) != count {
				t.Fatal("input tag mutated")
			}
		})
	}
}
