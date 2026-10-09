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
)

// BoardHealthLLM adjudicates one board's checkup statistics: a single call
// producing score + comment + notify together (design D5 — one call, one
// decision space). Seam for unit tests; production wiring is
// NewDefaultBoardHealthLLM.
type BoardHealthLLM interface {
	ScoreBoardHealth(ctx context.Context, prompt string) (*BoardHealthLLMResult, error)
}

// BoardHealthLLMResult is the merged adjudication output (0-100 score, one
// sentence comment, notify verdict).
type BoardHealthLLMResult struct {
	Score   int    `json:"score"`
	Comment string `json:"comment"`
	Notify  bool   `json:"notify"`
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
const boardHealthSystemPrompt = `你是一个语义板块治理助手，任务是基于给定的体检统计数据对单个板块的整体健康度做综合评分。

统计口径（必须先理解，防止误读）：
- board.matched_tags 是该版块当前全部匹配 tag 行数（全量现状，不限当日），它反映板块挂载规模
- labels[].samples 是该构成标签贴合度分布的样本数（= 参与比对的挂载 tag 数）；cosine_mean 等分布指标是「该构成标签语义与版块实际内容的余弦贴合度」，只在板块内各构成标签互比才有意义（跨板块绝对值不可比）
- low_sim_ratio 按板块内相对低分线（达标标签 cosine_mean 的 P25）计算：某标签该值高说明它在板块内贴合度垫底，挂载内容跑题风险高
- insufficient=true 表示样本量不足以评估（板块太小或向量缺失，不硬凑分布），不表示标签失效、更不表示板块内容匮乏

评分锚定（必须遵守）：
- 只基于给定指标打分，不要虚构未提供的信号；同一组指标应得到同一档分数
- score 是 0-100 整数：90+ 构成清晰且质量证据充分；70-89 总体健康有小瑕疵；60-69 存在值得关注的弱项；<60 明显不健康，判据仅限真实病征：幽灵匹配占比高（ghost_matches/matched_tags 偏高）、板块内贴合度分化明显（多数达标标签 cosine_mean 显著低于板块内高水平标签且 low_sim_ratio 大面积偏高）、matched_tags 相对构成规模明显萎缩、reason_dist 被弱命中路径（hit_rate/max_sim/weighted）主导且组合命中（composite_hit）几乎为零
- insufficient 标签数量本身不是扣分项：它只说明质量证据缺失，不影响分数；不要因「大量标签证据不足」而降档，也不要把证据不足误述为内容匮乏
- comment 是一句话评语（60 字以内），点出最主要的健康或风险信号；达标标签的板块内贴合度分化才是质量评语依据，禁止编造指标里没有的数字/事件/因果
- notify 是打扰裁决：只有当分数低且用户应当尽快处理（如大面积方向不匹配、幽灵匹配占比明显偏高）时才 true；轻微瑕疵一律 false

只返回 JSON：{"score": <0-100>, "comment": "<一句话评语>", "notify": <true|false>}`

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

// buildScorePrompt renders the user prompt: board name/description,
// composition list, board-level metrics, label-level distribution (with
// insufficient flags), and the top uncaptured candidates.
func buildScorePrompt(board *models.SemanticLabel, metricsJSON string, candidates []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "板块：%s\n", board.Label)
	if desc := strings.TrimSpace(board.Description); desc != "" {
		fmt.Fprintf(&b, "描述：%s\n", desc)
	}
	b.WriteString("\n体检统计数据（JSON）：\n")
	b.WriteString(metricsJSON)
	b.WriteString("\n")
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
	b.WriteString("\n请基于以上数据给出 score/comment/notify。\n")
	return b.String()
}
