package boardhealth

import (
	"strings"
	"testing"
)

// TestSystemPromptAnchors 断言评分判据的防误读锚定条款不回退
// （fix-board-health-llm-misread）：①分化双条件（相对垫底且绝对不健康）；
// ②短实体标签不扣分；③composite 常态保护；④既有 insufficient 不扣分条款
// 保留。字符串级防回归——LLM prompt 是行为契约（spec「LLM 综合评分与降级」
// 评分判据条款），措辞可调但锚定语义不可丢。
func TestSystemPromptAnchors(t *testing.T) {
	anchors := []struct {
		name    string
		needles []string
	}{
		{
			name:    "divergence requires both relative and absolute conditions",
			needles: []string{"双条件", "cosine_mean", "0.6", "相对位次"},
		},
		{
			name:    "low_sim_ratio explained as relative rank, not off-topic evidence",
			needles: []string{"相对位次", "不是跑题证据", "0.6", "不健康"},
		},
		{
			name:    "short entity labels not penalized",
			needles: []string{"短实体标签", "不构成扣分"},
		},
		{
			name:    "composite-absent boards protected on reason_dist",
			needles: []string{"常态而非病征", "不得单独作为降档依据"},
		},
		{
			name:    "insufficient labels still not a deduction (existing anchor survives + review H1 hardening)",
			needles: []string{"insufficient 标签数量本身不是扣分项", "证据缺失本身不得进入评语或作为降档理由"},
		},
	}
	for _, anchor := range anchors {
		t.Run(anchor.name, func(t *testing.T) {
			for _, needle := range anchor.needles {
				if !strings.Contains(boardHealthSystemPrompt, needle) {
					t.Errorf("system prompt missing anchor %q (regression of fix-board-health-llm-misread)", needle)
				}
			}
		})
	}
}
