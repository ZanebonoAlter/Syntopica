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
// normalized against the composition labels before persistence
// (normalizeTriage): 结构性非法项整轮降级，漏判项补「证据不足」。
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
- labels[].magnet 是磁铁信号：good_mean/failed_mean 为该标签对健康挂载子集（direction_mismatch=false）与方向失败子集（direction_mismatch=true）的贴合均值，good_samples/failed_samples 为两侧样本数；只有 failed_mean 显著高于 good_mean（差值达阈值）才是磁铁嫌疑成立——语义是「磁铁把跑题的挂载吸过来」；all_mean/all_samples 是全体挂载均值（**含**失败行，仅供历史口径对比展示，不参与判定，且 all_mean 与 cosine_mean 恒等）；insufficient=true 表示任一侧样本不足或向量缺失，不硬凑，不作为判病依据
- labels[].neighbors 是最近邻证据：全库 active 辅助标签向量空间 top-3（排除自身）；null 表示该标签缺自身向量（信号不足）
- labels[].desc_flags 是 description 文章式词根预筛命中（事件性嫌疑）；空或缺失表示未命中
- labels[].core_subject=true 表示该标签是本版块的核心主体（标签名出现在版块描述/别名中，或仅出现在标题中且标签名 ≥ 3 字符；2 字短词不据标题推断），它就是版块自身要谈论的对象：一律禁止判 remove 或 rewrite，必须归 keep
- board.magnet_flow 是失败挂载的主要流向：failed_tags 个方向失败挂载中 flow_tags 个在其他版块有有效挂载，top_board 为 top1 流向版块；null 表示无可靠流向（未知）；它是板级证据，只能佐证，不得充当单个标签的摘除证据

评分锚定（必须遵守）：
- 只基于给定指标打分，不要虚构未提供的信号；同一组指标应得到同一档分数
- score 是 0-100 整数：90+ 构成清晰且质量证据充分；70-89 总体健康有小瑕疵；60-69 存在值得关注的弱项；<60 明显不健康，判据仅限真实病征：幽灵匹配占比高（ghost_matches/matched_tags 偏高）、板块内贴合度分化明显（双条件须同时满足：多数达标标签 cosine_mean < 0.6 且 low_sim_ratio 大面积偏高；cosine_mean ≥ 0.6 时 low_sim_ratio 高只是板内相对位次，不是分化病征）、matched_tags 相对构成规模明显萎缩、reason_dist 被弱命中路径（hit_rate/max_sim/weighted）主导且 ghost 占比同时偏高（composite_hit 仅在板块挂载组合标签时可能产生，全库仅少数板有；无组合挂载的板 reason_dist 全为单标签路径是常态而非病征，不得单独作为降档依据）
- insufficient 标签数量本身不是扣分项：它只说明质量证据缺失，不影响分数；不要因「大量标签证据不足」而降档，也不要把证据不足误述为内容匮乏
- comment 是一句话评语（60 字以内），点出最主要的健康或风险信号；达标标签的板块内贴合度分化才是质量评语依据，禁止编造指标里没有的数字/事件/因果
- notify 是打扰裁决：只有当分数低且用户应当尽快处理（如大面积方向不匹配、幽灵匹配占比明显偏高）时才 true；轻微瑕疵一律 false

标签分诊（triage，与 score 同一次调用产出）：
- triage 数组对体检数据中的每一个构成标签恰好给出一条裁决（aux_id 与 labels 清单一一对齐，不得遗漏、不得重复、不得虚构不存在的 aux_id）
- 核心主体保护：core_subject=true 的标签是版块自身要谈论的对象，MUST NOT 判 remove 或 rewrite，只能归 keep（判据见下方 category 说明）
- 板级证据只可佐证、不得当单标签证据：board.magnet_flow 的失败挂载流向与版块级共享的失败子集对每一个标签都成立（同一组行、只有余弦值不同），不构成标签级归因，MUST NOT 当作某个标签判 remove/rewrite 的必要证据
- category 四值判据：
  - "remove"（建议摘除）：必须有磁铁信号成立（magnet.suspicious=true：失败子集均值显著高于健康子集均值 good_mean，且 good_samples 与 failed_samples 各自达标）或等价强证据；仅凭 low_sim_ratio 相对位次、低 cosine_mean、或弱命中路径占比都不得判 remove；短实体标签（人名/地名/技术名词等）即使 cosine_mean 低也禁止判 remove（归 keep）；core_subject=true 的标签禁止判 remove
  - "rewrite"（建议重写描述）：须有最近邻漂移/窄化证据（neighbors 显示最近邻是实例/机构/地点类标签而非同域概念标签）或 desc_flags 事件性词根命中；必须给出 suggestion=新的百科式描述文案（定义式、时间中性，不含事件叙述）；国家/地区/城市/机构/人物等实体类标签的同类最近邻（如「伊朗」的最近邻是「中东地区」「中东国家」）是健康形态，MUST NOT 记为漂移/窄化证据、不得据此判 rewrite；core_subject=true 的标签禁止判 rewrite
  - "keep"（保留澄清）：cosine 天然低但贴合分布正常（短名天花板）、或绝对健康（cosine_mean ≥ 0.6）、或该标签被标记为核心主体（core_subject=true）、或合理多归属：失败挂载集中流向语义重叠的对手版块属边界重叠，是版块定义问题而非标签污染，应归 keep（必要时 rewrite 版块定义），多归属不是摘除的充分依据
  - "insufficient"（证据不足下轮再判）：magnet/neighbors 信号 insufficient、样本低于最小样本量、或必要证据缺失；证据缺失不得虚构结论
- evidence 必须引用给定信号数值或统计事实（如 magnet good=0.52 failed=0.72 good_n=284 failed_n=40、最近邻 cosine 0.796、desc_flags 命中此次），禁止编造未提供的数字/事件/因果
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
	// CoreSubject 也进完整清单：证据块只渲染前 60 个标签，尾部标签若不带
	// 标记，「核心主体不得判 remove/rewrite」这条硬约束就管不到它们。
	CoreSubject bool `json:"core_subject"`
}

// maxEvidenceLabels caps the per-label signal evidence block (design D6:
// 提示词体积护栏，~60 标签 × 4 行).
const maxEvidenceLabels = 60

// buildScorePrompt renders the user prompt: board name/description, board +
// label-level metrics (complete label list, distributions only), the
// triage-signal evidence block (truncated to maxEvidenceLabels suspects
// first), and the top uncaptured candidates.
//
// magnetGap 是磁铁判定阈值（config.MagnetGap）——模型要引用证据就得知道
// 差值门槛。
func buildScorePrompt(board *models.SemanticLabel, metrics *tagmodels.BoardHealthMetrics, candidates []string, magnetGap float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "板块：%s\n", board.Label)
	if desc := strings.TrimSpace(board.Description); desc != "" {
		fmt.Fprintf(&b, "描述：%s\n", desc)
	} else {
		// 板块无描述文本时显式声明：核心主体只能依据 core_subject 标记判定，
		// 不让模型自行臆断板块范围（spec「LLM 综合评分与降级」）。
		b.WriteString("描述：（本板无描述文本，核心主体只能依据 core_subject 标记判定）\n")
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
			CoreSubject: l.CoreSubject,
		})
	}
	metricsJSON, err := json.Marshal(projection)
	if err != nil {
		metricsJSON = []byte("{}")
	}
	b.WriteString("\n体检统计数据（JSON，构成标签完整清单）：\n")
	b.Write(metricsJSON)
	b.WriteString("\n")

	renderEvidenceBlock(&b, metrics, magnetGap)

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
// 摘要 + 磁铁两侧均值 + 最近邻 top-3 + desc 预筛标记 + 核心主体标记，
// design D6 四行 + core_subject 行), suspect-first, capped at
// maxEvidenceLabels. Missing sides render as explicit 不足/未知 — never as a
// silent pass (描述/邻居缺失需显式不足).
//
// magnetGap 是磁铁判定阈值，写进证据行让模型能自己判断差值是否达门槛。
func renderEvidenceBlock(b *strings.Builder, metrics *tagmodels.BoardHealthMetrics, magnetGap float64) {
	b.WriteString("\n三病信号证据（每标签：贴合/磁铁/最近邻/描述预筛/核心主体）：\n")
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

		// 磁铁两侧均值（新口径：失败子集 vs 健康子集）
		switch {
		case l.Magnet == nil:
			b.WriteString("  磁铁: 无信号\n")
		case l.Magnet.Insufficient:
			fmt.Fprintf(b, "  磁铁: 不足（good_n=%d, failed_n=%d；样本不足或向量缺失，不硬凑）\n",
				l.Magnet.GoodSamples, l.Magnet.FailedSamples)
		case l.Magnet.Suspicious:
			fmt.Fprintf(b, "  磁铁: 嫌疑成立 good=%s failed=%s good_n=%d failed_n=%d 阈值=%.3f\n",
				formatOptFloat(l.Magnet.GoodMean), formatOptFloat(l.Magnet.FailedMean),
				l.Magnet.GoodSamples, l.Magnet.FailedSamples, magnetGap)
		default:
			fmt.Fprintf(b, "  磁铁: 无嫌疑 good=%s failed=%s good_n=%d failed_n=%d 阈值=%.3f\n",
				formatOptFloat(l.Magnet.GoodMean), formatOptFloat(l.Magnet.FailedMean),
				l.Magnet.GoodSamples, l.Magnet.FailedSamples, magnetGap)
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

		// 核心主体标记（判据硬约束的输入）：是 → 禁止判 remove/rewrite。
		if l.CoreSubject {
			b.WriteString("  core_subject: 是（核心主体：出现在版块描述/别名中，或仅出现在标题中且标签名 ≥ 3 字符；禁止判 remove/rewrite，只能归 keep）\n")
		} else {
			b.WriteString("  core_subject: 否\n")
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

// minTriageCoverage 是分诊清单的覆盖率下限：低于该值视为「模型输出崩了」
// （截断 / 格式坏），整轮降级。
//
// 实证依据（2026-11-03 体检）：本地 LLM 在 47/50/54/58/96/98 标签的板块上
// 分别只回 45/40/51/48/≥68/≥72 条 —— 漏判 2–28 条，实测覆盖率 71%–96%。
// 0.5 的下限用来挡「只回几条」的崩溃输出（那种情况补齐会变成满屏「证据
// 不足」的假象），同时把正常的漏判（≥50%）留给补齐路径处理。
const minTriageCoverage = 0.5

// normalizeTriage 校验并归一分诊清单（fix-board-health-triage-degrade D1）。
//
// 致命集（返回 error → 整轮裁决降级，score/comment/triage 全空）：
//   - category 不在闭集 {remove,rewrite,keep,insufficient}；
//   - aux_id 未知（不指向真实构成标签）或重复；
//   - evidence 去空白后为空（无证据的建议比没有建议更危险）；
//   - category=rewrite 但缺 suggestion（界面上是空白可编辑框）。
//
// 可补齐：合法但漏判的构成标签补一条 category=insufficient 的「证据不足」
// 条目 —— MUST NOT 猜 keep/remove/rewrite（补「证据不足」是承认本轮没
// 证据，不是伪造可执行建议）。覆盖率低于 minTriageCoverage 时整轮降级。
//
// 返回顺序：先保留 LLM 给出的条目顺序，再按 labels 顺序追加缺失项。
func normalizeTriage(triage []TriageItem, labels []tagmodels.BoardHealthLabelMetrics) ([]TriageItem, int, error) {
	known := make(map[uint]bool, len(labels))
	for i := range labels {
		known[labels[i].AuxID] = true
	}
	seen := make(map[uint]bool, len(triage))
	for _, item := range triage {
		switch item.Category {
		case "remove", "rewrite", "keep", "insufficient":
		default:
			return nil, 0, fmt.Errorf("triage aux %d: invalid category %q", item.AuxID, item.Category)
		}
		if !known[item.AuxID] {
			return nil, 0, fmt.Errorf("triage references unknown aux id %d", item.AuxID)
		}
		if seen[item.AuxID] {
			return nil, 0, fmt.Errorf("triage duplicate aux id %d", item.AuxID)
		}
		seen[item.AuxID] = true
		if strings.TrimSpace(item.Evidence) == "" {
			return nil, 0, fmt.Errorf("triage aux %d (%s): evidence required", item.AuxID, item.Category)
		}
		if item.Category == "rewrite" && strings.TrimSpace(item.Suggestion) == "" {
			return nil, 0, fmt.Errorf("triage aux %d: rewrite requires a suggestion", item.AuxID)
		}
	}

	// 覆盖率 = 合法唯一条目数 / 构成标签数。labels 为空时 triage 必须为空
	// （否则上面的未知 aux 判定已经报错）。
	m := len(labels)
	if m == 0 {
		return nil, 0, nil
	}
	covered := len(seen)
	coverage := float64(covered) / float64(m)
	if coverage < minTriageCoverage {
		// 覆盖率不足也要回传真实 covered —— 否则降级板的轮次日志只能打
		// covered=0，与同一行 reason 里的 covered=68/96 自相矛盾（review M-2）。
		return nil, covered, fmt.Errorf("triage covers %d/%d (%.1f%%) below min coverage %.0f%%",
			covered, m, coverage*100, minTriageCoverage*100)
	}

	// 补齐缺失标签：category 恒为 insufficient，suggestion 留空。
	filled := make([]TriageItem, 0, m)
	filled = append(filled, triage...)
	for i := range labels {
		if seen[labels[i].AuxID] {
			continue
		}
		filled = append(filled, TriageItem{
			AuxID:    labels[i].AuxID,
			Category: "insufficient",
			Evidence: fmt.Sprintf("本轮 LLM 未覆盖该标签（返回 %d/%d 条），下轮再判", covered, m),
		})
	}
	return filled, covered, nil
}

// ── 分诊合理性闸门（fix-board-health-magnet-signal design D4/D5/D6）───────
//
// 三道闸门顺序执行、彼此独立，且只依据确定性信号与版块文本判定——MUST NOT
// 依赖 LLM 自觉（提示词层已被实证会翻转：同一证据 11-06 判 keep、11-09 判
// rewrite）。
//
//	G1 核心主体拦截：core_subject 标签被判 remove/rewrite → 改判 keep，
//	   evidence 标注「核心主体保护」+ 保留原判，suggestion 清空；
//	G2 摘除证据校验：remove 必须有成立的磁铁信号（新口径），不成立或信号
//	   不足 → 降级 insufficient + 注明原因；
//	G3 占比闸门：可执行占比（remove+rewrite）/总数 超过阈值 → 本轮全部可
//	   执行项降级 insufficient + 注明原因，并在快照记录闸门结论。
//
// 闸门 MUST NOT 改写评分与评语（它们由调用方单独持有），也不丢弃分诊证据
// 本身——降级只改类别并在证据里追加原因。
//
// 返回的闸门记录为 nil = 本轮无闸门结论（分诊清单为空）；非 nil 时
// Actionable/Ratio 取自 **LLM 原始裁决**（G1/G2 之前的状态）——那是触发闸门
// 的证据（若取降级后状态，remove 主导轮次会被 G2 清零、触发原因被抹掉）。
func applyTriageGuards(items []TriageItem, labels []tagmodels.BoardHealthLabelMetrics, config Config) ([]TriageItem, *tagmodels.BoardHealthTriageGuard) {
	if len(items) == 0 {
		return items, nil // 无分诊可降级 → 不写闸门记录
	}
	byAux := make(map[uint]*tagmodels.BoardHealthLabelMetrics, len(labels))
	for i := range labels {
		byAux[labels[i].AuxID] = &labels[i]
	}
	guarded := make([]TriageItem, len(items))
	copy(guarded, items) // 输入切片不被就地改写（调用方的 LLM 结果保持原值）

	// G1 核心主体拦截：改判 keep（证据其实充分，只是「不允许动核心主体」——
	// 判 insufficient 会让下轮再来一遍同样的拦截）。
	for i := range guarded {
		item := &guarded[i]
		if item.Category != "remove" && item.Category != "rewrite" {
			continue
		}
		label := byAux[item.AuxID]
		if label == nil || !label.CoreSubject {
			continue
		}
		original := item.Category
		item.Category = "keep"
		item.Suggestion = ""
		item.Evidence = fmt.Sprintf("核心主体保护：该标签出现在板块描述/别名中（或标题中且标签名 ≥ 3 字），不作为摘除或重写对象（原判：%s，原证据：%s）",
			original, item.Evidence)
	}

	// G2 摘除证据校验：摘除是破坏性建议，必须有成立的磁铁信号（新口径：
	// 失败子集均值显著高于健康子集均值）。rewrite 的证据形态多样，规则层
	// 难以可靠机器化，不做同等硬校验（门槛可低一档）。
	for i := range guarded {
		item := &guarded[i]
		if item.Category != "remove" {
			continue
		}
		label := byAux[item.AuxID]
		switch {
		case label == nil || label.Magnet == nil:
			item.Category = "insufficient"
			item.Suggestion = ""
			item.Evidence = fmt.Sprintf("%s；磁铁信号缺失，摘除建议降级为证据不足", item.Evidence)
		case label.Magnet.Insufficient:
			item.Category = "insufficient"
			item.Suggestion = ""
			item.Evidence = fmt.Sprintf("%s；磁铁信号不足（good_n=%d, failed_n=%d，样本不足或向量缺失），摘除建议降级为证据不足",
				item.Evidence, label.Magnet.GoodSamples, label.Magnet.FailedSamples)
		case !label.Magnet.Suspicious:
			item.Category = "insufficient"
			item.Suggestion = ""
			item.Evidence = fmt.Sprintf("%s；磁铁信号不成立（failed 均值未显著高于健康子集均值），摘除建议降级为证据不足",
				item.Evidence)
		}
	}

	// G3 占比闸门：可执行占比超限 → 本轮全部可执行项降级（47/47 的坏轮次
	// 会被拦住；已知正确轮次最高 72%，阈值 0.8 留余量）。
	//
	// 占比取 **LLM 原始裁决**（G1/G2 之前统计，review R-5）：如实反映「本轮
	// 模型有多激进」。若取降级后的状态，remove 主导轮次里 G2 先把 47 条摘除
	// 降为证据不足，G3 再数 actionable 会得到 0 —— 闸门痕迹被抹掉、记录失真。
	// 降级动作仍作用于当前（已被 G1/G2 改过的）清单。
	total := len(items)
	actionable := 0
	for i := range items {
		if items[i].Category == "remove" || items[i].Category == "rewrite" {
			actionable++
		}
	}
	threshold := config.MaxActionableRatio
	if threshold <= 0 {
		// 零值 Config{}（未显式赋值 / 老调用方）按默认阈值——0 阈值会把任何
		// 可执行项都降级，不是「缺省值有固定默认」的语义。
		threshold = DefaultMaxActionableRatio
	}
	ratio := float64(actionable) / float64(total)
	guard := &tagmodels.BoardHealthTriageGuard{
		Actionable: actionable,
		Total:      total,
		Ratio:      ratio,
		Threshold:  threshold,
	}
	if ratio <= threshold {
		return guarded, guard // 未超限：不降级（占比恰等于阈值也不降级）
	}
	reason := fmt.Sprintf("可执行占比 %d/%d（%.1f%%）超过阈值 %.0f%%，本轮信号可能不可用",
		actionable, total, ratio*100, threshold*100)
	for i := range guarded {
		item := &guarded[i]
		if item.Category != "remove" && item.Category != "rewrite" {
			continue
		}
		item.Category = "insufficient"
		item.Suggestion = ""
		item.Evidence = fmt.Sprintf("%s；%s", item.Evidence, reason)
	}
	guard.Degraded = true
	guard.Reason = reason
	return guarded, guard
}
