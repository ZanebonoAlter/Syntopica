package service

import (
	"context"
	"fmt"
	"strings"

	"syntopica-backend/internal/platform/airouter"
)

// ── board_interpret：版块级命题生成（design D3，tasks 3.2 / M2）──────────────
//
// 输入：全板块态势卡集合 + 参考角色文档 + 板块级 applied review。输出候选命题
// （钩子 × 切角公式）+ chosen + reason。素材稀薄走 sparse 诚实降级；LLM 产出
// 不可解析时重试一次，仍烂则从最高质态势卡机械提炼单候选降级（不静默失败）。

// boardInterpretFormBoard / boardInterpretFormSparse are the board-level
// "forms": a board either sustains proposition generation or honestly reports
// thin material.
const (
	boardInterpretFormBoard  = "board"
	boardInterpretFormSparse = "sparse"
)

// boardCandidate is one 命题候选: hook（近期异常/新涌现素材）× angle（切角，
// 方法论模式）→ thesis（结构命题）。
type boardCandidate struct {
	Thesis string `json:"thesis"`
	Hook   string `json:"hook"`
	Angle  string `json:"angle"`
}

// boardInterpretOutput is the parsed board_interpret LLM response.
type boardInterpretOutput struct {
	Form        string           `json:"form"`
	Candidates  []boardCandidate `json:"candidates"`
	ChosenIndex int              `json:"chosen_index"`
	Reason      string           `json:"reason"`
	AllSparse   bool             `json:"all_sparse"` // mechanically detected upstream
	Degraded    bool             `json:"degraded"`   // true = mechanical fallback (LLM unusable)
	DegradedWhy string           `json:"degraded_why,omitempty"`
}

type boardInterpretInput struct {
	SessionID string
	CardsMD   string             // rendered situation cards (may be "" only when AllSparse)
	ReviewTxt string             // board-level applied review digest
	AllSparse bool               // every lane card signalled sparse/no material
	TopCard   *LaneSituationCard // highest-quality card (degraded fallback source)
}

const boardInterpretPrompt = `你是一位资深产业分析编辑，要为一个新闻板块生成「版块级结构命题」——不是罗列新闻，而是找到能把多条泳道串起来的底层命题。

方法论公式：命题 = 钩子 × 切角。
- 钩子：板块近期真实的异常/新涌现素材（从态势卡的事实里找，必须是卡里出现过的现象，不许编造）
- 切角：分析视角/结构切面（参考「分析方法参考」里的命题生成模式，如概念重命名、机制保质期、层级声明等）
- 命题（thesis）：一句「X 不是 A，而是 B」式的可论证结构判断，跨泳道、非单事件

先判断素材是否支撑：
- 若态势卡显示多数泳道无事实、命中稀薄，输出 form=sparse，thesis 明示素材不足，不要硬编命题
- 否则 form=board，生成 2-3 个候选命题，选定一个（chosen_index）并说明理由（reason 里说明为什么这个钩子×切角最值得写）

输出严格 JSON（不要其他内容）：
{"form": "board|sparse", "candidates": [{"thesis": "...", "hook": "...", "angle": "..."}], "chosen_index": 0, "reason": "选定理由"}`
const boardInterpretSparseNote = "\n\n注意：该板块所有泳道的素材信号都很稀薄（无事实摘要或稀疏历史）。如果确实找不到可论证的钩子，诚实输出 form=sparse。"

// boardInterpret runs the D3 board interpret step: one LLM call over the
// situation cards. Parse failure retries once; a second failure falls back to
// a mechanical single candidate from the top card (degraded, never silent).
func (o *OrchestratorService) boardInterpret(ctx context.Context, bctx boardInterpretInput) (*boardInterpretOutput, error) {
	prompt := boardInterpretPrompt
	if bctx.ReviewTxt != "" {
		prompt += "\n\n历史认知记录(避免重蹈已知偏差):\n" + bctx.ReviewTxt + "\n"
	}
	if bctx.AllSparse {
		prompt += boardInterpretSparseNote
	}
	prompt += "\n\n---\n态势卡集合:\n" + bctx.CardsMD

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.airouter.Chat(ctx, airouter.ChatRequest{
			Capability:  o.capability,
			Operation:   "data_enrichment.board_interpret",
			SessionID:   bctx.SessionID,
			Messages:    []airouter.Message{{Role: "user", Content: prompt}},
			Temperature: floatPtr(0.3),
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("board interpret chat: %w", err)
			continue
		}
		parsed, err := ParseJSONResponse(resp.Content)
		if err != nil {
			lastErr = fmt.Errorf("board interpret parse: %w", err)
			continue
		}
		if out, ok := validateBoardInterpret(parsed); ok {
			out.AllSparse = bctx.AllSparse
			return out, nil
		}
		lastErr = fmt.Errorf("board interpret: invalid payload (form/candidates/chosen mismatch)")
	}

	// Degraded fallback: mechanical single candidate from the top card.
	return degradedBoardInterpret(bctx, lastErr), nil
}

// validateBoardInterpret enforces the M2 contract: form board|sparse; board
// requires 1-3 non-empty candidates and chosen_index inside range; sparse
// requires an honest (possibly empty) candidate set with chosen 0.
func validateBoardInterpret(parsed map[string]any) (*boardInterpretOutput, bool) {
	form, _ := parsed["form"].(string)
	switch form {
	case boardInterpretFormSparse:
		out := &boardInterpretOutput{Form: boardInterpretFormSparse, ChosenIndex: 0}
		out.Reason, _ = parsed["reason"].(string)
		// Sparse may still carry a thesis explaining the shortfall.
		if cands, ok := parseBoardCandidates(parsed); ok && len(cands) > 0 {
			out.Candidates = cands[:1]
		}
		return out, true
	case boardInterpretFormBoard:
		cands, ok := parseBoardCandidates(parsed)
		if !ok || len(cands) < 1 || len(cands) > 3 {
			return nil, false
		}
		idx, ok := parsed["chosen_index"].(float64)
		if !ok || int(idx) < 0 || int(idx) >= len(cands) {
			return nil, false // dangling chosen → parse failure (M2.3)
		}
		out := &boardInterpretOutput{
			Form:        boardInterpretFormBoard,
			Candidates:  cands,
			ChosenIndex: int(idx),
		}
		out.Reason, _ = parsed["reason"].(string)
		return out, true
	}
	return nil, false
}

// parseBoardCandidates extracts non-empty candidates; false when the field is
// missing or no candidate has a thesis.
func parseBoardCandidates(parsed map[string]any) ([]boardCandidate, bool) {
	raw, ok := parsed["candidates"].([]any)
	if !ok {
		return nil, false
	}
	cands := make([]boardCandidate, 0, len(raw))
	for _, c := range raw {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		bc := boardCandidate{}
		bc.Thesis, _ = cm["thesis"].(string)
		bc.Hook, _ = cm["hook"].(string)
		bc.Angle, _ = cm["angle"].(string)
		if strings.TrimSpace(bc.Thesis) != "" {
			cands = append(cands, bc)
		}
	}
	return cands, len(cands) > 0
}

// degradedBoardInterpret synthesizes the M2.2 fallback from the top card.
func degradedBoardInterpret(bctx boardInterpretInput, cause error) *boardInterpretOutput {
	why := ""
	if cause != nil {
		why = cause.Error()
	}
	if bctx.TopCard == nil {
		return &boardInterpretOutput{
			Form: boardInterpretFormSparse, ChosenIndex: 0,
			Reason:   "素材不足且命题生成不可用，诚实降级",
			Degraded: true, DegradedWhy: why,
			AllSparse: bctx.AllSparse,
		}
	}
	tc := bctx.TopCard
	return &boardInterpretOutput{
		Form: boardInterpretFormBoard,
		Candidates: []boardCandidate{{
			Thesis: fmt.Sprintf("《%s》板块动向观察（降级命题：自动生成不可用，待人工复核）", tc.Label),
			Hook:   tc.FactsDigest,
			Angle:  "无（降级生成）",
		}},
		ChosenIndex: 0,
		Reason:      fmt.Sprintf("命题生成两次不可解析，从最高质态势卡（泳道#%d）机械提炼", tc.LaneID),
		Degraded:    true,
		DegradedWhy: why,
		AllSparse:   bctx.AllSparse,
	}
}

// BoardInterpretInputForTest / BoardInterpretForTest expose the board
// interpret step to the external test package.
type BoardInterpretInputForTest = boardInterpretInput

func (o *OrchestratorService) BoardInterpretForTest(ctx context.Context, bctx boardInterpretInput) (*boardInterpretOutput, error) {
	// M2.5: empty card set (no lanes) → reject before any LLM call.
	if strings.TrimSpace(bctx.CardsMD) == "" && !bctx.AllSparse {
		return nil, fmt.Errorf("board interpret: no lanes to analyze")
	}
	return o.boardInterpret(ctx, bctx)
}
