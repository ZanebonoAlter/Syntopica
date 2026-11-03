package boardhealth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
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

// TestSystemPromptTriageAnchors 断言分诊层锚定（board-health-triage-advice
// spec「LLM 综合评分与降级」四类判据 + implementation-contract §3）：摘除只认
// 磁铁证据、短名不判摘、多归属保护、rewrite 须 suggestion、证据不得虚构、
// 覆盖全部标签、同一次调用产出。
func TestSystemPromptTriageAnchors(t *testing.T) {
	needles := []string{
		"magnet.suspicious=true",
		"仅凭 low_sim_ratio 相对位次、低 cosine_mean",
		"禁止判 remove（归 keep）", // 短实体保护
		"多归属不是摘除的充分依据",       // 多归属保护
		"rewrite",
		"必须给出 suggestion=新的百科式描述文案",
		`"triage"`, // 输出结构含 triage
		"不得遗漏、不得重复、不得虚构不存在的 aux_id", // 覆盖与引用合法性
		"禁止编造未提供的数字/事件/因果",          // 证据不虚构（分诊层）
		"证据不足下轮再判",
		"同一次调用产出", // D3 单调用四产出
	}
	for _, needle := range needles {
		if !strings.Contains(boardHealthSystemPrompt, needle) {
			t.Errorf("system prompt missing triage anchor %q", needle)
		}
	}
}

// signalFixtureLabels 建一个含三类嫌疑样本的标签清单（磁铁嫌疑/预筛命中/
// 低分/健康），供提示词裁剪测试用。
func signalFixtureLabels(n int) []tagmodels.BoardHealthLabelMetrics {
	labels := make([]tagmodels.BoardHealthLabelMetrics, 0, n)
	for i := 0; i < n; i++ {
		label := tagmodels.BoardHealthLabelMetrics{
			AuxID: uint(1000 + i), Label: fmt.Sprintf("标签%d", i),
			Samples: 50, Insufficient: false,
		}
		mean, ratio := 0.7, 0.1
		label.CosineMean, label.LowSimRatio = &mean, &ratio
		labels = append(labels, label)
	}
	return labels
}

// TestBuildScorePromptEvidenceTruncation —— D6 护栏：证据块截 60，但完整
// 标签清单保留（不能截掉标签导致漏裁决）。白盒「LLM：60/61 标签截断」。
func TestBuildScorePromptEvidenceTruncation(t *testing.T) {
	board := &models.SemanticLabel{Label: "裁剪板", LabelType: "board", Status: "active"}
	newPrompt := func(labels []tagmodels.BoardHealthLabelMetrics) string {
		metrics := &tagmodels.BoardHealthMetrics{
			Board:  tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}},
			Labels: labels,
		}
		return buildScorePrompt(board, metrics, nil)
	}
	countBlocks := func(prompt string) int {
		return strings.Count(prompt, "  磁铁: ")
	}

	// 60 个：不截断。
	prompt60 := newPrompt(signalFixtureLabels(60))
	require.Equal(t, 60, countBlocks(prompt60), "60 labels: no truncation")
	require.NotContains(t, prompt60, "证据块按磁铁嫌疑")

	// 61 个：证据块 60，但完整清单 61 个都在，末尾覆盖指令声明 61。
	labels61 := signalFixtureLabels(61)
	prompt61 := newPrompt(labels61)
	require.Equal(t, 60, countBlocks(prompt61), "61 labels: evidence block capped at 60")
	require.Contains(t, prompt61, "全部 61 个构成标签")
	// 完整清单：61 个 aux_id 都进 JSON。
	for i := range labels61 {
		require.Contains(t, prompt61, fmt.Sprintf(`"aux_id":%d`, labels61[i].AuxID))
	}

	// 优先级：末位标签（位次 61）带磁铁嫌疑 → 必入选证捷块，挤掉一个
	// 无嫌疑标签；被挤掉的标签仍在完整清单里。
	labels61[60].Magnet = &tagmodels.BoardHealthMagnetSignal{
		AllSamples: 61, FailedSamples: 40, Suspicious: true,
		AllMean: &[]float64{0.68}[0], FailedMean: &[]float64{0.52}[0],
	}
	promptPrio := newPrompt(labels61)
	require.Equal(t, 60, countBlocks(promptPrio))
	require.Contains(t, promptPrio, "磁铁: 嫌疑成立 all=0.680 failed=0.520 failed_n=40")
	require.Contains(t, promptPrio, fmt.Sprintf("- aux #%d %s", labels61[60].AuxID, labels61[60].Label))
	// 位次末尾的无嫌疑标签 0 仍出现在完整清单 JSON（未被截掉）。
	require.Contains(t, promptPrio, fmt.Sprintf(`"aux_id":%d`, labels61[0].AuxID))
}

// TestBuildScorePromptEvidenceRendering —— 证据四行渲染：磁铁两侧均值/不
// 足、近邻或显式不足、desc 预筛命中/未命中/空、流向未知显式未知。
func TestBuildScorePromptEvidenceRendering(t *testing.T) {
	allMean, failedMean := 0.68, 0.52
	lowRatio := 0.1
	labels := []tagmodels.BoardHealthLabelMetrics{
		{
			AuxID: 1, Label: "上海", Samples: 400, CosineMean: &allMean, LowSimRatio: &lowRatio,
			Description: "上海知名商业综合体",
			Magnet: &tagmodels.BoardHealthMagnetSignal{
				AllSamples: 400, FailedSamples: 40, Suspicious: true,
				AllMean: &allMean, FailedMean: &failedMean,
			},
			Neighbors: []tagmodels.BoardHealthNeighbor{{AuxID: 9, Label: "举办地点", Cosine: 0.796}},
			DescFlags: []string{"此次"},
		},
		{
			AuxID: 2, Label: "无向量", Insufficient: true, // 缺自身向量：磁铁/近邻显式不足
			Magnet: &tagmodels.BoardHealthMagnetSignal{Insufficient: true},
		},
	}
	metrics := &tagmodels.BoardHealthMetrics{
		Board: tagmodels.BoardHealthBoardMetrics{
			ReasonDist: map[string]int{},
			MagnetFlow: &tagmodels.BoardHealthMagnetFlow{FailedTags: 40, FlowTags: 0}, // 无可靠流向
		},
		Labels: labels,
	}
	board := &models.SemanticLabel{Label: "渲染板", LabelType: "board", Status: "active"}
	prompt := buildScorePrompt(board, metrics, nil)

	require.Contains(t, prompt, "磁铁: 嫌疑成立 all=0.680 failed=0.520 failed_n=40")
	require.Contains(t, prompt, "近邻: 举办地点(0.796)")
	require.Contains(t, prompt, "desc: 「上海知名商业综合体」 预筛命中:此次")
	// 缺向量标签：磁铁/近邻显式不足，不臆造。
	require.Contains(t, prompt, "磁铁: 不足（all_n=0, failed_n=0；样本不足或向量缺失，不硬凑）")
	require.Contains(t, prompt, "近邻: 信号不足（缺自身向量）")
	require.Contains(t, prompt, "desc: （空） 预筛未命中")
	// 流向未知显式未知（magnet_flow JSON 里 top_board=null）。
	require.Contains(t, prompt, `"top_board":null`)
	// 覆盖指令。
	require.Contains(t, prompt, "全部 2 个构成标签")
	// 项目结构保留：体检统计数据段与口径提醒仍在。
	require.Contains(t, prompt, "体检统计数据")
	require.Contains(t, prompt, "口径提醒")
}

// TestValidateTriage —— 合法性校验闭集：非法类别/未知 aux/重复/漏覆盖/
// 缺证据/rewrite 缺 suggestion 都不可变成可执行建议（整体裁决降级）。
func TestValidateTriage(t *testing.T) {
	labels := []tagmodels.BoardHealthLabelMetrics{{AuxID: 1, Label: "A"}, {AuxID: 2, Label: "B"}}
	valid := []TriageItem{
		{AuxID: 1, Category: "remove", Evidence: "magnet failed=0.52 all=0.68 failed_n=40"},
		{AuxID: 2, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "百科式定义"},
	}
	require.NoError(t, validateTriage(valid, labels))

	cases := []struct {
		name   string
		triage []TriageItem
	}{
		{"非法类别", []TriageItem{{AuxID: 1, Category: "quarantine", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"未知 aux id", []TriageItem{{AuxID: 999, Category: "keep", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"重复 aux id", []TriageItem{{AuxID: 1, Category: "keep", Evidence: "x"}, {AuxID: 1, Category: "keep", Evidence: "y"}}},
		{"漏覆盖（少一条）", []TriageItem{{AuxID: 1, Category: "keep", Evidence: "x"}}},
		{"多余条目（多一条）", []TriageItem{valid[0], valid[1], {AuxID: 3, Category: "keep", Evidence: "z"}}},
		{"缺证据", []TriageItem{{AuxID: 1, Category: "keep", Evidence: " "}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"rewrite 无 suggestion", []TriageItem{{AuxID: 1, Category: "rewrite", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, validateTriage(tc.triage, labels), "不可把非法清单透成可执行建议")
		})
	}

	// 空构成标签 → 空清单合法；非空清单 → 非法。
	require.NoError(t, validateTriage(nil, nil))
	require.Error(t, validateTriage(valid[:1], nil))
}
