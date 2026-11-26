package main

import (
	"strings"
	"testing"
)

func TestRedoBroadFlagRemoved(t *testing.T) {
	if _, err := parseOptions([]string{"--redo-broad", "--dry-run"}); err == nil {
		t.Fatal("removed flag accepted")
	}
}

func TestSubjectKeyPromptEntityContract(t *testing.T) {
	prompt := fingerprintSystemPrompt()
	for _, text := range []string{"必须是单一实体", "最短可识别形式", "正例：", "反例："} {
		if !strings.Contains(prompt, text) {
			t.Errorf("missing entity contract %q", text)
		}
	}
	for _, text := range []string{"不超过 12", "美国中期选举", "美股科技股", "三方谈判", "AI竞争", "数据中心"} {
		if strings.Contains(prompt, text) {
			t.Errorf("prompt contains rejected heuristic %q", text)
		}
	}
}
