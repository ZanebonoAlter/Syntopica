package service

import (
	"context"
	"fmt"
)

// ── 版块形态 prompt 分支（design D3，tasks 3.3）───────────────────────────────
//
// 版块级探索/分析复用既有 tool_use/analyze Operation，但 prompt 按版块形态分支：
// 论证骨架=层级递进机制层；lane 论据织入主路径（内部工具优先级 ≥ 外部检索）；
// 跨泳道素材织入同一论证链，禁止并列罗列。max_loops 与去重防御完全复用
// runToolLoop（不另造 loop）。

// generateBoardSessionID: data_enrichment_board_{boardID}_{uuid8}.
func generateBoardSessionID(boardID uint) string {
	return fmt.Sprintf("data_enrichment_board_%d_%s", boardID, RandomHex(8))
}

// boardAgentLoopSystemPrompt is the board-form explorer prompt (角色② 版块分
// 支). Differences from the single-lane prompt: the "lens" is the board
// proposition (thesis); internal lane tools come FIRST in the workflow —
// lane 论据织入主路径要求内部脉络优先于外部检索.
const boardAgentLoopSystemPrompt = `你是一位研究助理 / 事实核查员，正在为一份【版块级深度分析】搜集论据。背景：这是一个新闻板块的多条泳道（持久话题）的态势卡（见下方），本次分析命题是「%s」。你需要为结构化分析师搜集能串起多条泳道的论据、历史 precedents 与一手原文。

可用工具：
%s

工作流程（版块分析专用，重要）：
1. 优先用内部工具：list_lanes 看板块泳道全景，get_lane_detail 下钻与命题相关的泳道（态势卡里质量高/事实相关的泳道），把本系统已沉淀的演进脉络吃透——内部论据是主路径
2. 内部素材不足以支撑论证时，再用 web_search 检索外部背景、历史 precedents 与专家分析（换 2-3 个角度/关键词），对关键命中用 fetch_page 取可核查原文摘录（不是 AI 转述）
3. 论据要服务命题论证（层级递进机制层），不是逐泳道罗列——跨泳道素材要能织进同一传导链
4. 拿到足以支撑论证的素材后，立即宣布完成

关键纪律（违反会导致死循环）：
- 工具返回的数据是完整的，不要因为"看起来不全"重查同一个查询
- 绝对不要用相同参数重复调用同一个工具
- 单条泳道下钻最多 2 次，把预算留给跨泳道论证

每一轮输出严格 JSON，二选一：
- 继续调工具：{"action": "call_tool", "thought": "...", "tool": "工具名", "args": {...}}
- 宣布完成：{"action": "finish", "thought": "...", "summary": "给分析师的论据汇总（标注哪些来自内部泳道、哪些来自外部检索，含可核查 URL 与原文摘录）"}

不要输出 JSON 以外的任何内容。

板块泳道态势卡（背景）：
%s`

// runBoardAgentLoop executes the agent loop for one board research direction.
// Same loop machinery as runAgentLoop (maxLoops/dedup/full-results defenses);
// only the system prompt branches to the board form.
func (o *OrchestratorService) runBoardAgentLoop(ctx context.Context, sessionID, direction, thesis, cardsMD string, allowedTools []string) (*AgentLoopResult, error) {
	toolsDesc := buildToolsDesc(o.toolRegistry, allowedTools)
	system := fmt.Sprintf(boardAgentLoopSystemPrompt, thesis, toolsDesc, cardsMD)
	return runToolLoop(ctx, o.airouter, o.toolRegistry, o.capability, toolLoopParams{
		sessionID:    sessionID,
		systemPrompt: system,
		taskLine:     "当前研究方向: " + direction,
		operation:    "data_enrichment.tool_use",
		allowedTools: allowedTools,
		maxLoops:     maxAgentLoops,
		resultTopic:  direction,
	})
}

// boardAnalyzePrompt is the board-form analyst prompt (角色③ 版块分支).
// Argument skeleton = 层级递进机制层; output JSON contract matches the
// board sectors shape {scope, thesis, candidates, argument, depth, lane_refs}.
const boardAnalyzePrompt = `你是一位资深产业分析师，正在为新闻板块写一份【版块级深度分析】。命题（thesis）已定：「%s」，切角：「%s」。

下面提供：
1) 板块各泳道态势卡（内部背景）
2) 各研究方向的论据素材（agent 检索的内部泳道细节 + 外部背景/原文）

你的任务：围绕命题产出论文式论证——不是逐泳道罗列，而是把跨泳道素材织入同一论证链。

【论证骨架：层级递进机制层】
argument 按层级递进组织（第一层是表层现象，逐层向下挖到结构性根因）：
- intro：开篇定调——钩子（近期真实异常）如何升维到命题（结构判断），2-3 句
- layers：3-5 层机制层，每层 {layer: 机制名, deep_logic: 深层逻辑, basis: 依据（引用素材）}；层与层之间要有递进关系（上一层解释不了，才需要下一层）
- boundary：反过度解读边界——明确写"目前还不能下结论的边界"，不可空泛
- conclusion：收束——回扣开篇钩子，落在命题上的最终判断（含确定性分级 cert: high|medium|low|question）

【跨泳道织入纪律】
- 论据可以来自多条泳道，但必须服务同一传导链；"泳道A怎样、泳道B怎样"的并列罗列是失败的
- 涉及具体泳道处用 lane 引用（source_type="lane", ref=lane_id, note=该泳道贡献了什么论据）

【深度层 depth（强制）】
- system_reframe：这个板块命题该放进哪个更大的系统来讲
- mechanism_layers：与 argument.layers 呼应的多层机制拆解（可复用，但要补 basis）
- historical_analogy：历史类比 {case, mechanism, diff}
- regime_shift：范式转折判断（确实有才填，无则 null）
- boundary：与 argument.boundary 呼应的边界声明
- evidence_chain：可核查证据链，source_type ∈ news|web|page|lane；web/page 必须带 url + quote(原文摘录，非转述) + institution + date；lane 必须带 lane_id + note

【证据多样性纪律】
evidence_chain 尽量覆盖 ≥3 类证据：数据序列（机构统计/长序列数字）、报告文献（一手报告/官方文件）、历史对照（可核查历史事件）、新闻网页（时效性事件）。检索引导：优先一手源（机构名 + 数据关键词组合，而非事件转述关键词）。某类确实检索不到时，在 boundary 里诚实标注，不要编造。

【引用格式】source_type ∈ news（分层新闻）| web（web_search 网页）| page（fetch_page 正文）| lane（板块内泳道，ref=lane_id）；quote 直接摘录原话/原文

输出严格 JSON（不要 markdown 包裹）：
{"scope":"board","form":"board","thesis":"%s","lens":"%s","candidates":[],"argument":{"intro":"...","layers":[...],"boundary":"...","conclusion":{"cert":"...","judgment":"..."}},"depth":{"system_reframe":"...","mechanism_layers":[...],"historical_analogy":{...},"regime_shift":null,"boundary":"...","evidence_chain":[...]},"lane_refs":[{"lane_id":0,"note":"..."}]}`

// assembleBoardAnalyzePrompt builds the board analyze prompt (single assembly
// point so the board branch is unit-testable without an LLM).
func (o *OrchestratorService) assembleBoardAnalyzePrompt(_ context.Context, thesis, angle, cardsMD string, topicsData []map[string]any) string {
	topicsBlock := ""
	for _, td := range topicsData {
		topic, _ := td["topic"].(string)
		data, _ := td["data"].(string)
		if data == "" {
			data = "(无数据)"
		}
		topicsBlock += fmt.Sprintf("【%s】\n查询数据:\n%s\n\n", topic, data)
	}

	prompt := fmt.Sprintf(boardAnalyzePrompt, thesis, angle, thesis, angle)
	prompt += "\n\n---\n板块泳道态势卡:\n" + cardsMD + "\n\n各研究方向论据:\n" + topicsBlock
	return prompt
}

// AssembleBoardAnalyzePromptForTest exposes the prompt assembler for tests.
func (o *OrchestratorService) AssembleBoardAnalyzePromptForTest(ctx context.Context, thesis, angle, cardsMD string, topicsData []map[string]any) string {
	return o.assembleBoardAnalyzePrompt(ctx, thesis, angle, cardsMD, topicsData)
}

// GenerateBoardSessionIDForTest exposes the board session ID generator.
func GenerateBoardSessionIDForTest(boardID uint) string { return generateBoardSessionID(boardID) }

// RunBoardAgentLoopForTest exposes the board agent loop for tests.
func (o *OrchestratorService) RunBoardAgentLoopForTest(ctx context.Context, sessionID, direction, thesis, cardsMD string, allowedTools []string) (*AgentLoopResult, error) {
	return o.runBoardAgentLoop(ctx, sessionID, direction, thesis, cardsMD, allowedTools)
}

// MaxAgentLoopsForTest exposes the shared loop cap constant.
func MaxAgentLoopsForTest() int { return maxAgentLoops }

// AssembleSingleLaneAnalyzePromptForTest exposes the single-lane analyze prompt
// assembly (D10 applies to both scopes).
func (o *OrchestratorService) AssembleSingleLaneAnalyzePromptForTest(_ context.Context, form, lens, lifelineText, contextText string, topicsData []map[string]any) string {
	// Mirrors analyze()'s assembly without the LLM call.
	prompt := fmt.Sprintf(analyzePrompt, form, lens, form, lens)
	if contextText != "" {
		prompt += "\n\n---\n分层新闻上下文:\n" + contextText
	}
	prompt += "\n\n话题演进脉络:\n" + lifelineText
	return prompt
}
