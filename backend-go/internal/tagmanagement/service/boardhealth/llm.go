package boardhealth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// BoardHealthLLM adjudicates one board's checkup statistics: a single call
// producing score + comment + notify together (design D5 — one call, one
// decision space). Seam for unit tests; production wiring is
// NewDefaultBoardHealthLLM.
type BoardHealthLLM interface {
	ScoreBoardHealth(ctx context.Context, prompt string) (*BoardHealthLLMResult, error)
}

// BoardHealthLLMResult is the merged adjudication output (0-100 score, one
// sentence comment, notify verdict, and — board-health-triage-advice D3 —
// the per-label triage list produced by the SAME single call). Triage is
// validated against the composition labels before persistence
// (validateTriage); an invalid list degrades the whole verdict.
type BoardHealthLLMResult struct {
	Score   int          `json:"score"`
	Comment string       `json:"comment"`
	Notify  bool         `json:"notify"`
	Triage  []TriageItem `json:"triage"`
}

// TriageItem is one label-level triage verdict inside the LLM result
// (design D3). Category is the closed set remove|rewrite|keep|insufficient;
// converted to models.BoardHealthTriageItem after validation.
type TriageItem struct {
	AuxID      uint   `json:"aux_id"`
	Category   string `json:"category"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion"`
}

// defaultBoardHealthLLM is the airouter-backed production implementation,
// mirroring board.defaultSemanticBoardUpgradeLLM.
type defaultBoardHealthLLM struct{}

// NewDefaultBoardHealthLLM returns the production airouter-backed LLM.
func NewDefaultBoardHealthLLM() BoardHealthLLM {
	return defaultBoardHealthLLM{}
}

// boardHealthSystemPrompt anchors the scoring on the given metrics (risk
// mitigation for inter-round drift: the same metrics must earn the same
// score band). Affinity distributions are BOARD-RELATIVE (label embedding ×
// mounted-tag embedding, low-sim line = in-board P25): absolute cosine
// values are not comparable across boards, only within one board's label
// ranking. An insufficient label still means "evidence unavailable" (tiny
// board / missing vectors), NOT "label broken" — the anchor from the
// 2026-10-03 finding (all-insufficient misread as dead content → tanked
// score) survives the 2026-10-08口径 switch (see explore-findings).
const boardHealthSystemPrompt = `你是一个语义板块治理助手，任务是基于给定的体检统计数据与三病分诊信号，对单个板块的整体健康度做综合评分，并对每个构成标签给出分诊裁决。

统计口径（必须先理解，防止误读）：
- board.matched_tags 是该版块当前全部匹配 tag 行数（全量现状，不限当日），它反映板块挂载规模
- labels[].samples 是该构成标签贴合度分布的样本数（= 参与比对的挂载 tag 数）；cosine_mean 等分布指标是「该构成标签语义与版块实际内容的余弦贴合度」，只在板块内各构成标签互比才有意义（跨板块绝对值不可比）
- low_sim_ratio 按板块内相对低分线（达标标签 cosine_mean 的 P25）计算：它只说明该标签在板块内的相对位次，P25 相对线的固有性质决定相当比例标签天然在线下；单独的高 low_sim_ratio 不是跑题证据，须结合 cosine_mean 绝对值判断（< 0.6 才视为不健康）
- 短实体标签（人名/地名/技术名词等 2-4 字短语）语义聚焦，其 cosine_mean 天然低于泛概念标签 0.05-0.10（identity 事件句 × 短名词标签的 cosine 天然带 0.51-0.76），不构成扣分或「跑题」评语依据
- insufficient=true 表示样本量不足以评估（板块太小或向量缺失，不硬凑分布），不表示标签失效、更不表示板块内容匮乏；全部标签 insufficient 的板块按剩余真实指标（ghost 占比、matched_tags 规模、reason_dist）定档，证据缺失本身不得进入评语或作为降档理由
- labels[].magnet 是磁铁信号：all_mean/failed_mean 为该标签对全部挂载与方向失败子集的贴合均值，failed_samples 为失败子集样本数；suspicious=true 才是磁铁嫌疑成立；insufficient=true 表示样本不足或向量缺失，不硬凑，不作为判病依据
- labels[].neighbors 是最近邻证据：全库 active 辅助标签向量空间 top-3（排除自身）；null 表示该标签缺自身向量（信号不足）
- labels[].desc_flags 是 description 文章式词根预筛命中（事件性嫌疑）；空或缺失表示未命中
- board.magnet_flow 是失败挂载的主要流向：failed_tags 个方向失败挂载中 flow_tags 个在其他版块有有效挂载，top_board 为 top1 流向版块；null 表示无可靠流向（未知）

评分锚定（必须遵守）：
- 只基于给定指标打分，不要虚构未提供的信号；同一组指标应得到同一档分数
- score 是 0-100 整数：90+ 构成清晰且质量证据充分；70-89 总体健康有小瑕疵；60-69 存在值得关注的弱项；<60 明显不健康，判据仅限真实病征：幽灵匹配占比高（ghost_matches/matched_tags 偏高）、板块内贴合度分化明显（双条件须同时满足：多数达标标签 cosine_mean < 0.6 且 low_sim_ratio 大面积偏高；cosine_mean ≥ 0.6 时 low_sim_ratio 高只是板内相对位次，不是分化病征）、matched_tags 相对构成规模明显萎缩、reason_dist 被弱命中路径（hit_rate/max_sim/weighted）主导且 ghost 占比同时偏高（composite_hit 仅在板块挂载组合标签时可能产生，全库仅少数板有；无组合挂载的板 reason_dist 全为单标签路径是常态而非病征，不得单独作为降档依据）
- insufficient 标签数量本身不是扣分项：它只说明质量证据缺失，不影响分数；不要因「大量标签证据不足」而降档，也不要把证据不足误述为内容匮乏
- comment 是一句话评语（60 字以内），点出最主要的健康或风险信号；达标标签的板块内贴合度分化才是质量评语依据，禁止编造指标里没有的数字/事件/因果
- notify 是打扰裁决：只有当分数低且用户应当尽快处理（如大面积方向不匹配、幽灵匹配占比明显偏高）时才 true；轻微瑕疵一律 false

标签分诊（triage，与 score 同一次调用产出）：
- triage 数组对体检数据中的每一个构成标签恰好给出一条裁决（aux_id 与 labels 清单一一对齐，不得遗漏、不得重复、不得虚构不存在的 aux_id）
- category 四值判据：
  - "remove"（建议摘除）：必须有磁铁信号成立（magnet.suspicious=true：failed 均值显著低于全体 + failed_samples 达标）或等价强证据，可结合 board.magnet_flow 的主要流向佐证；仅凭 low_sim_ratio 相对位次、低 cosine_mean、或弱命中路径占比都不得判 remove；短实体标签（人名/地名/技术名词等）即使 cosine_mean 低也禁止判 remove（归 keep）
  - "rewrite"（建议重写描述）：须有最近邻漂移/窄化证据（neighbors 显示最近邻是实例/机构/地点类标签而非同域概念标签）或 desc_flags 事件性词根命中；必须给出 suggestion=新的百科式描述文案（定义式、时间中性，不含事件叙述）
  - "keep"（保留澄清）：cosine 天然低但贴合分布正常（短名天花板）、或绝对健康（cosine_mean ≥ 0.6）、或合理多归属：失败挂载集中流向语义重叠的对手版块属边界重叠，是版块定义问题而非标签污染，应归 keep（必要时 rewrite 版块定义），多归属不是摘除的充分依据
  - "insufficient"（证据不足下轮再判）：magnet/neighbors 信号 insufficient、样本低于最小样本量、或必要证据缺失；证据缺失不得虚构结论
- evidence 必须引用给定信号数值或统计事实（如 magnet failed=0.52 all=0.68 failed_n=40、最近邻 cosine 0.796、desc_flags 命中此次），禁止编造未提供的数字/事件/因果
- 评分判据的防误读条款（分化双条件/短实体不扣分/composite 常态保护）在分诊层同步生效

只返回 JSON：{"score": <0-100>, "comment": "<一句话评语>", "notify": <true|false>, "triage": [{"aux_id": <构成标签id>, "category": "remove|rewrite|keep|insufficient", "evidence": "<证据摘要，引用真实数值>", "suggestion": "<新描述文案，仅 category=rewrite 时必填，其余可为空串>"}]}`

// ScoreBoardHealth calls the airouter and parses the merged JSON verdict. A
// malformed/timed-out response returns an error so the caller degrades
// (score NULL, no retry) — no partial result is synthesized.
func (defaultBoardHealthLLM) ScoreBoardHealth(ctx context.Context, prompt string) (*BoardHealthLLMResult, error) {
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "tagmanagement.board_health_checkup",
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: boardHealthSystemPrompt},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		Metadata: map[string]any{"operation": "board_health_checkup"},
	})
	if err != nil {
		return nil, err
	}
	sanitized := jsonutil.SanitizeLLMJSON(result.Content)
	var parsed BoardHealthLLMResult
	if err := json.Unmarshal([]byte(sanitized), &parsed); err != nil {
		preview := sanitized
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		logging.Warnf("[board-health] LLM JSON parse failed: %v, preview=%s", err, preview)
		return nil, err
	}
	if parsed.Score < 0 || parsed.Score > 100 {
		return nil, fmt.Errorf("llm score out of range: %d", parsed.Score)
	}
	return &parsed, nil
}

// promptMetrics mirrors BoardHealthMetrics minus the per-label triage-signal
// fields: the FULL label list keeps its distribution fields (完整清单 — the
// triage coverage anchor), while magnet/neighbors/desc_flags render as the
// separately truncated evidence block below, so the 60-label guard (D6)
// bounds prompt size without ever dropping a label from adjudication
// coverage (不能截掉标签导致漏裁决).
type promptMetrics struct {
	Board  promptBoardMetrics   `json:"board"`
	Labels []promptLabelMetrics `json:"labels"`
}

type promptBoardMetrics struct {
	MatchedTags   int                              `json:"matched_tags"`
	ReasonDist    map[string]int                   `json:"reason_dist"`
	GhostMatches  int                              `json:"ghost_matches"`
	VectorMissing int                              `json:"vector_missing"`
	MagnetFlow    *tagmodels.BoardHealthMagnetFlow `json:"magnet_flow"`
}

type promptLabelMetrics struct {
	AuxID        uint     `json:"aux_id"`
	Label        string   `json:"label"`
	Samples      int      `json:"samples"`
	CosineMean   *float64 `json:"cosine_mean"`
	CosineMedian *float64 `json:"cosine_median"`
	CosineP25    *float64 `json:"cosine_p25"`
	CosineP75    *float64 `json:"cosine_p75"`
	LowSimRatio  *float64 `json:"low_sim_ratio"`
	Insufficient bool     `json:"insufficient"`
}

// maxEvidenceLabels caps the per-label signal evidence block (design D6:
// 提示词体积护栏，~60 标签 × 4 行).
const maxEvidenceLabels = 60

// buildScorePrompt renders the user prompt: board name/description, board +
// label-level metrics (complete label list, distributions only), the
// triage-signal evidence block (truncated to maxEvidenceLabels suspects
// first), and the top uncaptured candidates.
func buildScorePrompt(board *models.SemanticLabel, metrics *tagmodels.BoardHealthMetrics, candidates []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "板块：%s\n", board.Label)
	if desc := strings.TrimSpace(board.Description); desc != "" {
		fmt.Fprintf(&b, "描述：%s\n", desc)
	}

	projection := promptMetrics{Board: promptBoardMetrics{
		MatchedTags:   metrics.Board.MatchedTags,
		ReasonDist:    metrics.Board.ReasonDist,
		GhostMatches:  metrics.Board.GhostMatches,
		VectorMissing: metrics.Board.VectorMissing,
		MagnetFlow:    metrics.Board.MagnetFlow,
	}}
	for i := range metrics.Labels {
		l := &metrics.Labels[i]
		projection.Labels = append(projection.Labels, promptLabelMetrics{
			AuxID: l.AuxID, Label: l.Label, Samples: l.Samples,
			CosineMean: l.CosineMean, CosineMedian: l.CosineMedian,
			CosineP25: l.CosineP25, CosineP75: l.CosineP75,
			LowSimRatio: l.LowSimRatio, Insufficient: l.Insufficient,
		})
	}
	metricsJSON, err := json.Marshal(projection)
	if err != nil {
		metricsJSON = []byte("{}")
	}
	b.WriteString("\n体检统计数据（JSON，构成标签完整清单）：\n")
	b.Write(metricsJSON)
	b.WriteString("\n")

	renderEvidenceBlock(&b, metrics)

	// Coverage explainer: without it the model reads all-insufficient labels as
	// dead content (口径②覆盖面 ≠ 质量，见 system prompt 口径节).
	b.WriteString("\n口径提醒：matched_tags 为全量匹配现状（不限当日）；insufficient=true 只表示该标签无可归因纯净样本（归因覆盖面窄），不是标签失效或内容匮乏。\n")
	if len(candidates) > 0 {
		b.WriteString("\n未收编候选（成员 tags 高频携带、但不在版块构成中的辅助标签，按频次降序）：\n")
		for _, c := range candidates {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	} else {
		b.WriteString("\n未收编候选：无\n")
	}
	fmt.Fprintf(&b, "\n请基于以上数据给出 score/comment/notify，并对体检数据中全部 %d 个构成标签给出 triage 分诊（aux_id 逐一对齐，不得遗漏）。\n", len(metrics.Labels))
	return b.String()
}

// renderEvidenceBlock writes the per-label triage-signal evidence (贴合分布
// 摘要 + 磁铁两侧均值 + 最近邻 top-3 + desc 预筛标记，design D6 四行),
// suspect-first, capped at maxEvidenceLabels. Missing sides render as
// explicit 不足/未知 — never as a silent pass (描述/邻居缺失需显式不足).
func renderEvidenceBlock(b *strings.Builder, metrics *tagmodels.BoardHealthMetrics) {
	b.WriteString("\n三病信号证据（每标签：贴合/磁铁/最近邻/描述预筛）：\n")
	if len(metrics.Labels) == 0 {
		b.WriteString("（该版块暂无构成标签）\n")
		return
	}
	for _, idx := range evidenceLabelOrder(metrics.Labels) {
		l := &metrics.Labels[idx]
		fmt.Fprintf(b, "- aux #%d %s\n", l.AuxID, l.Label)

		// 贴合分布摘要
		if l.Insufficient {
			b.WriteString("  贴合: insufficient（样本不足或向量缺失，不硬凑）\n")
		} else {
			fmt.Fprintf(b, "  贴合: samples=%d mean=%s p25=%s p75=%s low_sim=%s\n",
				l.Samples, formatOptFloat(l.CosineMean), formatOptFloat(l.CosineP25),
				formatOptFloat(l.CosineP75), formatOptFloat(l.LowSimRatio))
		}

		// 磁铁两侧均值
		switch {
		case l.Magnet == nil:
			b.WriteString("  磁铁: 无信号\n")
		case l.Magnet.Insufficient:
			fmt.Fprintf(b, "  磁铁: 不足（all_n=%d, failed_n=%d；样本不足或向量缺失，不硬凑）\n",
				l.Magnet.AllSamples, l.Magnet.FailedSamples)
		case l.Magnet.Suspicious:
			fmt.Fprintf(b, "  磁铁: 嫌疑成立 all=%s failed=%s failed_n=%d\n",
				formatOptFloat(l.Magnet.AllMean), formatOptFloat(l.Magnet.FailedMean), l.Magnet.FailedSamples)
		default:
			fmt.Fprintf(b, "  磁铁: 无嫌疑 all=%s failed=%s failed_n=%d\n",
				formatOptFloat(l.Magnet.AllMean), formatOptFloat(l.Magnet.FailedMean), l.Magnet.FailedSamples)
		}

		// 最近邻 top-3
		switch {
		case l.Neighbors == nil:
			b.WriteString("  近邻: 信号不足（缺自身向量）\n")
		case len(l.Neighbors) == 0:
			b.WriteString("  近邻: 无（active 辅助空间内无其他标签）\n")
		default:
			parts := make([]string, 0, len(l.Neighbors))
			for _, n := range l.Neighbors {
				parts = append(parts, fmt.Sprintf("%s(%.3f)", n.Label, n.Cosine))
			}
			fmt.Fprintf(b, "  近邻: %s\n", strings.Join(parts, ", "))
		}

		// desc 预筛标记
		desc := "（空）"
		if l.Description != "" {
			desc = fmt.Sprintf("「%s」", l.Description)
		}
		if len(l.DescFlags) > 0 {
			fmt.Fprintf(b, "  desc: %s 预筛命中:%s\n", desc, strings.Join(l.DescFlags, ","))
		} else {
			fmt.Fprintf(b, "  desc: %s 预筛未命中\n", desc)
		}
	}
	if len(metrics.Labels) > maxEvidenceLabels {
		fmt.Fprintf(b, "（构成标签 %d 个，证据块按磁铁嫌疑→预筛命中→低分位次→标签顺序截断至 %d 个；上方完整清单中的其余标签仍必须分诊，不得因缺证据块而跳过）\n",
			len(metrics.Labels), maxEvidenceLabels)
	}
}

// evidenceLabelOrder picks which labels render an evidence block when the
// composition exceeds maxEvidenceLabels (D6 priority: 全部 🔴/🟡 候选 +
// 低分/磁铁命中 + 按位次补齐): tier 1 magnet suspects, tier 2 desc-screen
// hits, tier 3 low-sim labels (relative ratio ≥ 0.5), tier 4 board order
// (labels arrive id ASC from loadCompositionLabels). Within a tier the
// board order is preserved.
func evidenceLabelOrder(labels []tagmodels.BoardHealthLabelMetrics) []int {
	tiers := make([]int, len(labels))
	for i := range labels {
		switch {
		case labels[i].Magnet != nil && labels[i].Magnet.Suspicious:
			tiers[i] = 1
		case len(labels[i].DescFlags) > 0:
			tiers[i] = 2
		case labels[i].LowSimRatio != nil && *labels[i].LowSimRatio >= 0.5:
			tiers[i] = 3
		default:
			tiers[i] = 4
		}
	}
	order := make([]int, 0, len(labels))
	for tier := 1; tier <= 4; tier++ {
		for i := range labels {
			if tiers[i] == tier {
				order = append(order, i)
			}
		}
	}
	if len(order) > maxEvidenceLabels {
		order = order[:maxEvidenceLabels]
	}
	return order
}

// formatOptFloat renders a nil-able float as .3f, "-" when absent.
func formatOptFloat(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.3f", *v)
}

// validateTriage enforces the triage contract (implementation-contract §3)
// before anything is persisted: closed category set, evidence required on
// every item, rewrite requires a suggestion, aux ids must reference real
// composition labels exactly once, and every composition label must be
// covered. Any violation degrades the WHOLE verdict (task 1.3: 优先整个
// 裁决降级，不伪造补齐) — a bad item must never surface as actionable
// advice.
func validateTriage(triage []TriageItem, labels []tagmodels.BoardHealthLabelMetrics) error {
	if len(triage) != len(labels) {
		return fmt.Errorf("triage covers %d items but board has %d composition labels", len(triage), len(labels))
	}
	known := make(map[uint]bool, len(labels))
	for i := range labels {
		known[labels[i].AuxID] = true
	}
	seen := make(map[uint]bool, len(triage))
	for _, item := range triage {
		switch item.Category {
		case "remove", "rewrite", "keep", "insufficient":
		default:
			return fmt.Errorf("triage aux %d: invalid category %q", item.AuxID, item.Category)
		}
		if !known[item.AuxID] {
			return fmt.Errorf("triage references unknown aux id %d", item.AuxID)
		}
		if seen[item.AuxID] {
			return fmt.Errorf("triage duplicate aux id %d", item.AuxID)
		}
		seen[item.AuxID] = true
		if strings.TrimSpace(item.Evidence) == "" {
			return fmt.Errorf("triage aux %d (%s): evidence required", item.AuxID, item.Category)
		}
		if item.Category == "rewrite" && strings.TrimSpace(item.Suggestion) == "" {
			return fmt.Errorf("triage aux %d: rewrite requires a suggestion", item.AuxID)
		}
	}
	return nil
}
