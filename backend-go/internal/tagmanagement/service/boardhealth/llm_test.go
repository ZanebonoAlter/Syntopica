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

// TestSystemPromptMagnetAndCoreSubjectAnchors —— fix-board-health-magnet-signal
// D7 的四条硬约束防回退（字符串级断言，风格对齐既有锚点测试）：① 磁铁口径
// 改为「失败子集相对健康子集」；② 核心主体不得判 remove/rewrite；③ 实体类
// 标签的同类最近邻不算漂移证据；④ 板级失败子集/流向只可佐证、不作单标签
// 必要证据。
func TestSystemPromptMagnetAndCoreSubjectAnchors(t *testing.T) {
	anchors := []struct {
		name    string
		needles []string
	}{
		{
			name:    "magnet verdict uses failed vs healthy subset",
			needles: []string{"good_mean/failed_mean", "失败子集均值显著高于健康子集均值", "不参与判定"},
		},
		{
			name:    "core subject must not be removed or rewritten",
			needles: []string{"core_subject=true 的标签禁止判 remove", "core_subject=true 的标签禁止判 rewrite"},
		},
		{
			name:    "core subject caliber follows the tightened rule (review R-6)",
			needles: []string{"标签名出现在版块描述/别名中，或仅出现在标题中且标签名 ≥ 3 字符"},
		},
		{
			name:    "same-kind neighbors of entity labels are not drift evidence",
			needles: []string{"同类最近邻", "MUST NOT 记为漂移/窄化证据"},
		},
		{
			name:    "board-level evidence only corroborates",
			needles: []string{"板级证据只可佐证", "MUST NOT 当作某个标签判 remove/rewrite 的必要证据"},
		},
	}
	for _, anchor := range anchors {
		t.Run(anchor.name, func(t *testing.T) {
			for _, needle := range anchor.needles {
				if !strings.Contains(boardHealthSystemPrompt, needle) {
					t.Errorf("system prompt missing anchor %q (regression of fix-board-health-magnet-signal D7)", needle)
				}
			}
		})
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
		return buildScorePrompt(board, metrics, nil, 0.08)
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
		AllSamples: 61, GoodSamples: 284, FailedSamples: 40, Suspicious: true,
		GoodMean: &[]float64{0.52}[0], FailedMean: &[]float64{0.72}[0],
	}
	promptPrio := newPrompt(labels61)
	require.Equal(t, 60, countBlocks(promptPrio))
	require.Contains(t, promptPrio, "磁铁: 嫌疑成立 good=0.520 failed=0.720 good_n=284 failed_n=40 阈值=0.080")
	require.Contains(t, promptPrio, fmt.Sprintf("- aux #%d %s", labels61[60].AuxID, labels61[60].Label))
	// 位次末尾的无嫌疑标签 0 仍出现在完整清单 JSON（未被截掉）。
	require.Contains(t, promptPrio, fmt.Sprintf(`"aux_id":%d`, labels61[0].AuxID))
}

// TestBuildScorePromptEvidenceRendering —— 证据四行渲染：磁铁两侧均值/不
// 足、近邻或显式不足、desc 预筛命中/未命中/空、流向未知显式未知。
func TestBuildScorePromptEvidenceRendering(t *testing.T) {
	goodMean, failedMean := 0.52, 0.72
	lowRatio := 0.1
	labels := []tagmodels.BoardHealthLabelMetrics{
		{
			AuxID: 1, Label: "上海", Samples: 400, CosineMean: &goodMean, LowSimRatio: &lowRatio,
			Description: "上海知名商业综合体",
			CoreSubject: true,
			Magnet: &tagmodels.BoardHealthMagnetSignal{
				AllSamples: 400, GoodSamples: 284, FailedSamples: 40, Suspicious: true,
				GoodMean: &goodMean, FailedMean: &failedMean,
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
	prompt := buildScorePrompt(board, metrics, nil, 0.08)

	require.Contains(t, prompt, "磁铁: 嫌疑成立 good=0.520 failed=0.720 good_n=284 failed_n=40 阈值=0.080")
	require.Contains(t, prompt, "近邻: 举办地点(0.796)")
	require.Contains(t, prompt, "desc: 「上海知名商业综合体」 预筛命中:此次")
	// 核心主体标记逐标签透出（判据硬约束的输入）。
	require.Contains(t, prompt, "core_subject: 是")
	require.Contains(t, prompt, "core_subject: 否")
	// 缺向量标签：磁铁/近邻显式不足，不臆造。
	require.Contains(t, prompt, "磁铁: 不足（good_n=0, failed_n=0；样本不足或向量缺失，不硬凑）")
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

// TestBuildScorePromptCoreSubjectAndNoDescription —— 板块 description 为空时
// 显式告知模型「核心主体只能依据 core_subject 标记判定」（不臆断板块范围），
// 且核心主体标记进完整标签清单（证据块只渲染前 60 个，尾部标签也得带上）。
func TestBuildScorePromptCoreSubjectAndNoDescription(t *testing.T) {
	metrics := &tagmodels.BoardHealthMetrics{
		Board:  tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}},
		Labels: []tagmodels.BoardHealthLabelMetrics{{AuxID: 262, Label: "伊朗", CoreSubject: true}},
	}
	board := &models.SemanticLabel{Label: "中东地缘政治与美伊关系", LabelType: "board", Status: "active"}

	prompt := buildScorePrompt(board, metrics, nil, 0.08)
	require.Contains(t, prompt, "本板无描述文本，核心主体只能依据 core_subject 标记判定")
	require.Contains(t, prompt, `"core_subject":true`)
	require.Contains(t, prompt, "core_subject: 是（核心主体：出现在版块描述/别名中，或仅出现在标题中且标签名 ≥ 3 字符；禁止判 remove/rewrite，只能归 keep）")

	// 有描述时不出「本板无描述文本」声明。
	board.Description = "本板覆盖中东地缘政治与美伊关系"
	prompt = buildScorePrompt(board, metrics, nil, 0.08)
	require.Contains(t, prompt, "描述：本板覆盖中东地缘政治与美伊关系")
	require.NotContains(t, prompt, "本板无描述文本")
}

// normalizeTriageLabels 构造 n 个构成标签（aux id 从 1 递增），供归一化用例复用。
func normalizeTriageLabels(n int) []tagmodels.BoardHealthLabelMetrics {
	labels := make([]tagmodels.BoardHealthLabelMetrics, 0, n)
	for i := 0; i < n; i++ {
		labels = append(labels, tagmodels.BoardHealthLabelMetrics{
			AuxID: uint(i + 1),
			Label: fmt.Sprintf("标签%d", i+1),
		})
	}
	return labels
}

// normalizeTriageKeepItems 造 n 条合法分诊条目（aux id 1..n）。
func normalizeTriageKeepItems(n int) []TriageItem {
	items := make([]TriageItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, TriageItem{
			AuxID:    uint(i + 1),
			Category: "keep",
			Evidence: fmt.Sprintf("cosine_mean=0.71 且无磁铁嫌疑 #%d", i+1),
		})
	}
	return items
}

// TestNormalizeTriageFatalSet —— 致命集仍整轮降级：非法类别 / 未知 aux /
// 重复 aux / 缺证据（含全角空格）/ rewrite 缺 suggestion / 空板却给建议。
// 这些是结构性非法，保留会污染清单或透出无证据的可执行建议。
func TestNormalizeTriageFatalSet(t *testing.T) {
	labels := normalizeTriageLabels(2)
	valid := []TriageItem{
		{AuxID: 1, Category: "remove", Evidence: "magnet failed=0.72 good=0.52 failed_n=40"},
		{AuxID: 2, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "百科式定义"},
	}
	normalized, covered, err := normalizeTriage(valid, labels)
	require.NoError(t, err)
	require.Equal(t, valid, normalized, "全量覆盖时原样返回（filled=0）")
	require.Equal(t, 2, covered, "全量覆盖时 covered == 构成标签数")

	cases := []struct {
		name   string
		triage []TriageItem
	}{
		{"非法类别", []TriageItem{{AuxID: 1, Category: "quarantine", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"未知 aux id", []TriageItem{{AuxID: 999, Category: "keep", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"重复 aux id", []TriageItem{{AuxID: 1, Category: "keep", Evidence: "x"}, {AuxID: 1, Category: "keep", Evidence: "y"}}},
		{"缺证据", []TriageItem{{AuxID: 1, Category: "keep", Evidence: " "}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"缺证据（全角空格）", []TriageItem{{AuxID: 1, Category: "keep", Evidence: "　"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"rewrite 无 suggestion", []TriageItem{{AuxID: 1, Category: "rewrite", Evidence: "x"}, {AuxID: 2, Category: "keep", Evidence: "y"}}},
		{"多余条目（多一条）", []TriageItem{valid[0], valid[1], {AuxID: 3, Category: "keep", Evidence: "z"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := normalizeTriage(tc.triage, labels)
			require.Error(t, err, "结构性非法必须整轮降级")
		})
	}

	// 空构成标签 → 空清单合法；非空清单 → 未知 aux → 降级。
	normalized, _, err = normalizeTriage(nil, nil)
	require.NoError(t, err)
	require.Empty(t, normalized)
	_, _, err = normalizeTriage(valid[:1], nil)
	require.Error(t, err)
}

// TestNormalizeTriageFillsMissingLabels —— 漏判补齐（design D1）：合法但漏
// 判的构成标签补一条 insufficient「证据不足」，已给条目原样保留且在前，
// 缺失项按 labels 顺序追加在后。MUST NOT 猜 keep/remove/rewrite。
func TestNormalizeTriageFillsMissingLabels(t *testing.T) {
	labels := normalizeTriageLabels(3)
	triage := []TriageItem{
		{AuxID: 1, Category: "keep", Evidence: "cosine_mean=0.71"},
		{AuxID: 3, Category: "remove", Evidence: "magnet failed=0.72 good=0.52"},
	}
	normalized, _, err := normalizeTriage(triage, labels)
	require.NoError(t, err)
	require.Len(t, normalized, 3, "补齐后清单长度 == 构成标签数")
	require.Equal(t, triage[0], normalized[0], "LLM 给的条目原样保留")
	require.Equal(t, triage[1], normalized[1])
	require.Equal(t, uint(2), normalized[2].AuxID, "缺失项按 labels 顺序追加")
	require.Equal(t, "insufficient", normalized[2].Category, "补齐项只能是证据不足")
	require.Contains(t, normalized[2].Evidence, "本轮 LLM 未覆盖该标签")
	require.Contains(t, normalized[2].Evidence, "返回 2/3 条")
	require.Contains(t, normalized[2].Evidence, "下轮再判")
	require.Empty(t, normalized[2].Suggestion, "补齐项不得携带建议文案")
	require.Len(t, triage, 2, "输入切片不被就地修改")
}

// TestNormalizeTriageCoverage —— 覆盖率分支表（test-cases.md §4）：
// 覆盖率下限 0.5 —— 低于下限整轮降级，达到下限走补齐。
func TestNormalizeTriageCoverage(t *testing.T) {
	cases := []struct {
		name       string
		labels     int
		covered    int
		wantErr    bool
		wantFilled int
	}{
		{"0% 覆盖降级", 10, 0, true, 0},
		{"49.9% 覆盖降级", 1000, 499, true, 0},
		{"50% 覆盖通过", 10, 5, false, 5},
		{"96% 覆盖补齐 1 条", 25, 24, false, 1},
		{"100% 覆盖原样", 10, 10, false, 0},
		{"m=1 且 n=0 降级", 1, 0, true, 0},
		{"m=0 且 n=0 合法", 0, 0, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labels := normalizeTriageLabels(tc.labels)
			triage := normalizeTriageKeepItems(tc.covered)
			normalized, covered, err := normalizeTriage(triage, labels)
			// covered 即使降级也要回传真实值（review M-2：轮次日志不能打 covered=0）。
			if tc.labels > 0 {
				require.Equal(t, tc.covered, covered, "降级路径也要回传真实覆盖数")
			}
			if tc.wantErr {
				require.Error(t, err, "覆盖率低于下限必须整轮降级")
				if tc.labels > 0 {
					require.Contains(t, err.Error(), "below min coverage")
					require.Contains(t, err.Error(), fmt.Sprintf("%d/%d", tc.covered, tc.labels))
					require.Contains(t, err.Error(), "below min coverage 50%", "reason 要带阈值数值")
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, normalized, tc.labels)
			filled := 0
			for _, item := range normalized {
				if item.Category == "insufficient" && strings.Contains(item.Evidence, "本轮 LLM 未覆盖该标签") {
					filled++
				}
			}
			require.Equal(t, tc.wantFilled, filled, "补齐条数 == 漏判条数")
		})
	}
}

// ── 分诊合理性闸门（fix-board-health-magnet-signal design D4/D5/D6，
// test-cases.md §7.2 分支表）────────────────────────────────

// magnetSuspect 造一个成立的磁铁信号（失败侧高于健康侧）。
func magnetSuspect() *tagmodels.BoardHealthMagnetSignal {
	good, failed := 0.52, 0.72
	return &tagmodels.BoardHealthMagnetSignal{
		GoodMean: &good, FailedMean: &failed,
		GoodSamples: 284, FailedSamples: 40, Suspicious: true,
	}
}

// magnetNotSuspect 造一个不成立的磁铁信号（失败侧低于健康侧——核心主体形态）。
func magnetNotSuspect() *tagmodels.BoardHealthMagnetSignal {
	good, failed := 0.70, 0.54
	return &tagmodels.BoardHealthMagnetSignal{
		GoodMean: &good, FailedMean: &failed,
		GoodSamples: 284, FailedSamples: 19,
	}
}

// magnetInsufficient 造一个样本不足的磁铁信号。
func magnetInsufficient() *tagmodels.BoardHealthMagnetSignal {
	return &tagmodels.BoardHealthMagnetSignal{GoodSamples: 3, FailedSamples: 20, Insufficient: true}
}

// TestApplyTriageGuardsCoreSubject —— G1 核心主体拦截：被判 remove/rewrite
// 的核心主体改判 keep，evidence 标注「核心主体保护」+ 保留原判与原证据，
// suggestion 清空；非核心主体与 keep/insufficient 不动。
func TestApplyTriageGuardsCoreSubject(t *testing.T) {
	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: 1, Label: "伊朗", CoreSubject: true},
		{AuxID: 2, Label: "卡塔尔", CoreSubject: false},
		{AuxID: 3, Label: "德黑兰", CoreSubject: true},
	}
	items := []TriageItem{
		{AuxID: 1, Category: "rewrite", Evidence: "近邻窄化", Suggestion: "伊朗，中东国家"},
		{AuxID: 2, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "卡塔尔，君主制国家"},
		{AuxID: 3, Category: "keep", Evidence: "cosine_mean=0.71"},
	}
	guarded, guard := applyTriageGuards(items, labels, Config{MaxActionableRatio: 0.8})

	require.Equal(t, "keep", guarded[0].Category, "核心主体判 rewrite → 改判 keep")
	require.Contains(t, guarded[0].Evidence, "核心主体保护")
	require.Contains(t, guarded[0].Evidence, "出现在板块描述/别名中（或标题中且标签名 ≥ 3 字）", "证据里的命中口径与新规则一致")
	require.Contains(t, guarded[0].Evidence, "rewrite", "原判留痕")
	require.Contains(t, guarded[0].Evidence, "近邻窄化", "原证据留痕")
	require.Empty(t, guarded[0].Suggestion, "改判 keep 不得携带重写文案")

	require.Equal(t, "rewrite", guarded[1].Category, "非核心主体不动")
	require.Equal(t, "卡塔尔，君主制国家", guarded[1].Suggestion)
	require.Equal(t, "keep", guarded[2].Category, "核心主体判 keep 不动")
	require.Equal(t, "cosine_mean=0.71", guarded[2].Evidence)

	// 闸门记录：可执行取 **LLM 原始裁决**（G1/G2 之前）：原始 2 条 rewrite
	// / 总数 3 → 2/3 ≤ 0.8 → 未降级（G1 把其中一条改判 keep 不影响占比）。
	require.NotNil(t, guard)
	require.False(t, guard.Degraded)
	require.Equal(t, 2, guard.Actionable)
	require.Equal(t, 3, guard.Total)
	require.InDelta(t, 2.0/3.0, guard.Ratio, 1e-9)
	require.InDelta(t, 0.8, guard.Threshold, 1e-9)

	// 输入切片不被就地改写。
	require.Equal(t, "rewrite", items[0].Category)
	require.Equal(t, "伊朗，中东国家", items[0].Suggestion)
}

// TestApplyTriageGuardsCoreSubjectRemove —— G1 对 remove 同样生效。
func TestApplyTriageGuardsCoreSubjectRemove(t *testing.T) {
	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: 1, Label: "伊朗", CoreSubject: true},
		{AuxID: 2, Label: "卡塔尔", CoreSubject: false, Magnet: magnetSuspect()},
	}
	items := []TriageItem{
		{AuxID: 1, Category: "remove", Evidence: "磁铁 failed=0.72 good=0.52"},
		{AuxID: 2, Category: "remove", Evidence: "磁铁 failed=0.72 good=0.52"},
	}
	// 阈值配 1.0：原始占比 2/2 = 1.0 不超限 → 隔离 G1 行为（G3 另有用例）。
	guarded, _ := applyTriageGuards(items, labels, Config{MaxActionableRatio: 1.0})
	require.Equal(t, "keep", guarded[0].Category, "核心主体判 remove → 改判 keep")
	require.Contains(t, guarded[0].Evidence, "核心主体保护")
	require.Contains(t, guarded[0].Evidence, "remove")
	require.Empty(t, guarded[0].Suggestion)
	// 非核心主体的 remove 不受 G1 影响（后续由 G2 校验磁铁证据）。
	require.Equal(t, "remove", guarded[1].Category)
}

// TestApplyTriageGuardsRemoveEvidence —— G2 摘除证据校验：磁铁成立才留
// remove，不成立/不足一律降级为证据不足；非 remove 类别不动。
func TestApplyTriageGuardsRemoveEvidence(t *testing.T) {
	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: 1, Label: "甲", Magnet: magnetSuspect()},
		{AuxID: 2, Label: "乙", Magnet: magnetNotSuspect()},
		{AuxID: 3, Label: "丙", Magnet: magnetInsufficient()},
		{AuxID: 4, Label: "丁", Magnet: magnetSuspect()},
	}
	items := []TriageItem{
		{AuxID: 1, Category: "remove", Evidence: "磁铁成立 failed=0.72 good=0.52"},
		{AuxID: 2, Category: "remove", Evidence: "磁铁不成立 failed=0.54 good=0.70"},
		{AuxID: 3, Category: "remove", Evidence: "磁铁样本不足"},
		{AuxID: 4, Category: "rewrite", Evidence: "desc 预筛命中此次"},
	}
	guarded, guard := applyTriageGuards(items, labels, Config{MaxActionableRatio: 1.0})

	require.Equal(t, "remove", guarded[0].Category, "磁铁成立 → 保持 remove")
	require.Equal(t, "磁铁成立 failed=0.72 good=0.52", guarded[0].Evidence)

	require.Equal(t, "insufficient", guarded[1].Category, "磁铁不成立 → 降级")
	require.Contains(t, guarded[1].Evidence, "磁铁信号不成立")
	require.Contains(t, guarded[1].Evidence, "磁铁不成立 failed=0.54 good=0.70", "原证据保留")

	require.Equal(t, "insufficient", guarded[2].Category, "磁铁不足 → 降级")
	require.Contains(t, guarded[2].Evidence, "磁铁信号不足")

	require.Equal(t, "rewrite", guarded[3].Category, "非 remove 类别不动")

	// 闸门记录：占比取 LLM 原始裁决（4/4 = 100%）；阈值配 1.0 → 不超限，
	// 避免 G3 叠加降级干扰本用例对 G2 的观察。
	require.NotNil(t, guard)
	require.False(t, guard.Degraded, "阈值 1.0 时原始占比 1.0 不超限")
	require.Equal(t, 4, guard.Actionable)
	require.Equal(t, 4, guard.Total)
	require.InDelta(t, 1.0, guard.Ratio, 1e-9)
}

// TestApplyTriageGuardsActionableRatio —— G3 占比闸门：47/47 全 rewrite
// （占比 1.0 > 0.8）→ 全部降级为证据不足，闸门记录占比/阈值/原因；分数与
// 评语不在闸门职责内（由调用方单独持有）。
func TestApplyTriageGuardsActionableRatio(t *testing.T) {
	labels := make([]tagmodels.BoardHealthLabelMetrics, 0, 47)
	items := make([]TriageItem, 0, 47)
	for i := 1; i <= 47; i++ {
		labels = append(labels, tagmodels.BoardHealthLabelMetrics{AuxID: uint(i), Label: fmt.Sprintf("标签%d", i)})
		items = append(items, TriageItem{
			AuxID: uint(i), Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "百科式新描述",
		})
	}
	guarded, guard := applyTriageGuards(items, labels, Config{MaxActionableRatio: 0.8})

	require.NotNil(t, guard)
	require.True(t, guard.Degraded)
	require.Equal(t, 47, guard.Actionable)
	require.Equal(t, 47, guard.Total)
	require.InDelta(t, 1.0, guard.Ratio, 1e-9)
	require.InDelta(t, 0.8, guard.Threshold, 1e-9)
	require.Contains(t, guard.Reason, "超过阈值")
	for _, item := range guarded {
		require.Equal(t, "insufficient", item.Category, "可执行项全部降级")
		require.Empty(t, item.Suggestion, "降级项不得携带建议文案")
		require.Contains(t, item.Evidence, "超过阈值")
	}
	require.Equal(t, "rewrite", items[0].Category, "输入切片不被就地改写")
}

// TestApplyTriageGuardsRatioFromOriginalVerdict —— G3 占比取 **LLM 原始裁决**
// （review R-5 / spec Scenario「闸门占比取 LLM 原始裁决而非降级后状态」）：
// 47 条全 remove，但它们的磁铁信号都不成立 → G2 先把 47 条降为证据不足。
// 若占比按降级后的状态统计就是 0/47（闸门痕迹被抹掉、记录失真），实际必须
// 仍是原始的 1.0 → 超阈值 → degraded=true。
func TestApplyTriageGuardsRatioFromOriginalVerdict(t *testing.T) {
	labels := make([]tagmodels.BoardHealthLabelMetrics, 0, 47)
	items := make([]TriageItem, 0, 47)
	for i := 1; i <= 47; i++ {
		labels = append(labels, tagmodels.BoardHealthLabelMetrics{
			AuxID: uint(i), Label: fmt.Sprintf("标签%d", i), Magnet: magnetNotSuspect(),
		})
		items = append(items, TriageItem{
			AuxID: uint(i), Category: "remove", Evidence: "磁铁 failed=0.54 good=0.70",
		})
	}
	guarded, guard := applyTriageGuards(items, labels, Config{MaxActionableRatio: 0.8})

	require.NotNil(t, guard)
	require.Equal(t, 47, guard.Actionable, "占比取 LLM 原始裁决（G1/G2 之前）")
	require.Equal(t, 47, guard.Total)
	require.InDelta(t, 1.0, guard.Ratio, 1e-9, "G2 降级不得把闸门占比抹成 0")
	require.True(t, guard.Degraded, "原始占比 1.0 > 0.8 → 降级")
	require.Contains(t, guard.Reason, "超过阈值")
	for _, item := range guarded {
		require.Equal(t, "insufficient", item.Category, "磁铁不成立的摘除被 G2 降级")
		require.Contains(t, item.Evidence, "磁铁信号不成立")
	}
}

// TestApplyTriageGuardsRatioBoundary —— 占比边界（test-cases.md §7.3）：恰等
// 于阈值不降级（> 语义）；阈值配成 1.0 时占比 1.0 不超限；零值 Config 按
// 默认阈值 0.8 处理（不做「0 阈值全降级」）。
func TestApplyTriageGuardsRatioBoundary(t *testing.T) {
	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: 1, Label: "甲", Magnet: magnetSuspect()},
		{AuxID: 2, Label: "乙", Magnet: magnetSuspect()},
		{AuxID: 3, Label: "丙", Magnet: magnetSuspect()},
		{AuxID: 4, Label: "丁", Magnet: magnetSuspect()},
	}
	items := []TriageItem{
		{AuxID: 1, Category: "rewrite", Evidence: "e1", Suggestion: "s1"},
		{AuxID: 2, Category: "remove", Evidence: "e2"},
		{AuxID: 3, Category: "rewrite", Evidence: "e3", Suggestion: "s3"},
		{AuxID: 4, Category: "keep", Evidence: "e4"},
	} // actionable = 3/4 = 0.75

	guarded, guard := applyTriageGuards(items, labels, Config{MaxActionableRatio: 0.75})
	require.False(t, guard.Degraded, "占比恰等于阈值 → 不降级")
	require.Equal(t, 3, guard.Actionable)
	require.Equal(t, "rewrite", guarded[0].Category)

	guarded, guard = applyTriageGuards(items, labels, Config{MaxActionableRatio: 0.74})
	require.True(t, guard.Degraded, "超限即降级")
	require.Equal(t, "insufficient", guarded[0].Category)
	require.Equal(t, "insufficient", guarded[1].Category)
	require.Equal(t, "keep", guarded[3].Category, "非可执行项不动")

	// 阈值配成 1.0：占比 1.0 不超限（1.0 > 1.0 为假）→ 不降级。
	allActionable := []TriageItem{
		{AuxID: 1, Category: "rewrite", Evidence: "e1", Suggestion: "s1"},
		{AuxID: 2, Category: "remove", Evidence: "e2"},
		{AuxID: 3, Category: "rewrite", Evidence: "e3", Suggestion: "s3"},
		{AuxID: 4, Category: "rewrite", Evidence: "e4", Suggestion: "s4"},
	}
	_, guard = applyTriageGuards(allActionable, labels, Config{MaxActionableRatio: 1.0})
	require.False(t, guard.Degraded, "阈值 1.0 时占比 1.0 不超限")

	// 零值 Config{} → 默认阈值 0.8（不是 0）。
	_, guard = applyTriageGuards(items, labels, Config{})
	require.InDelta(t, DefaultMaxActionableRatio, guard.Threshold, 1e-9)
	require.False(t, guard.Degraded, "0.75 ≤ 0.8 → 不降级")
}

// TestApplyTriageGuardsEmptyTriage —— 分诊清单为空 → 不触发闸门、不写记录。
func TestApplyTriageGuardsEmptyTriage(t *testing.T) {
	guarded, guard := applyTriageGuards(nil, []tagmodels.BoardHealthLabelMetrics{{AuxID: 1}}, Config{})
	require.Nil(t, guard)
	require.Empty(t, guarded)
}
