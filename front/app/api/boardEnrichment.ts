import { apiClient } from "./client";
import type { ApiResponse } from "~/types";

/**
 * 数据增强（data-enrichment）API client。
 *
 * 三表认知闭环 + 板块数据源绑定 + 个股辩论（FinGenius），对应后端已实现的 16 条 REST endpoint。
 * 数据契约设计为侦探墙（TopicDetectiveWall）后续重构可直接复用：
 *  - ContextRow / ResultSummaryRow / ReviewRow / DataSourceRow 顶层行类型稳定；
 *  - sectors / tool_calls / input_snapshot 这类 LLM 产物保留宽口子（unknown / 联合），
 *    侦探墙渲染时再按需 narrow。
 *
 * 路由分两个维度：
 *  - Topic 维度（表 1/2/3）：/persistent-topics/:topicId/enrichment/...
 *  - Board 维度（数据源绑定）：/semantic-boards/:id/data-sources
 *
 * 注意：后端 getResult / applyReview / updateReviewDeviation 虽然实现里只用 :id，
 * 但路由前缀含 :topicId，URL 仍须带上（后端解析忽略），所以函数签名是 (topicId, id)。
 */

// ── Shared scalars ──────────────────────────────────────────────────────────

export type ContextGranularity = "week" | "month" | "year" | "all";

export type EnrichmentSource = "manual" | "llm_assisted" | string;

// ── Table 1: topic_lifeline_context ─────────────────────────────────────────

/** 单条分层新闻汇总上下文（period-archival：granularity+period 唯一）。 */
export interface ContextRow {
	id: number;
	persistent_topic_id?: number;
	granularity: ContextGranularity;
	/** 周期键：week=2026-W27 / month=2026-06 / year=2026 / all=all。 */
	period: string;
	content: string;
	/** 汇总截止日（时效判断 + 检查自愈依据）。 */
	as_of_date: string;
	source: EnrichmentSource;
	created_at?: string;
	updated_at?: string;
}

// ── Table 2: topic_enrichment_result ────────────────────────────────────────

/** 方向（涨/跌/横盘）。debate.verdict / ResultSector.direction 复用。 */
export type Direction = "up" | "down" | "flat";

/** 板块下代表标的（design §3.4 symbols）。 */
export interface ResultSectorSymbol {
	code: string;
	name?: string;
	/** etf=ETF基金，leader_stock=龙头股。 */
	kind?: "etf" | "leader_stock" | string;
	[key: string]: unknown;
}

/** 板块证据条目（design §3.4 evidence）。 */
export interface ResultSectorEvidence {
	context_id?: number | string;
	period?: string;
	text?: string;
	[key: string]: unknown;
}

/**
 * 分析员结论中的一个产业切片（design §3.3/§3.4 新 schema）。
 *
 * 板块名以后端 handler.extractSymbolsFromSectors 读取的字段为准（sector），
 * 同时兼容 design schema 的 name；渲染时取 sector || name。
 * LLM 产物，字段保留宽口子。
 */
export interface ResultSector {
	/** 后端辩论提取读此字段作为 sector 名（权威）。 */
	sector?: string;
	/** design schema 的板块名字段（兼容回退）。 */
	name?: string;
	direction?: Direction | string;
	confidence?: number;
	horizon?: "short" | "mid" | "long" | string;
	reasoning?: string;
	evidence?: ResultSectorEvidence[];
	trigger_up?: string;
	trigger_down?: string;
	symbols?: ResultSectorSymbol[];
	[key: string]: unknown;
}

// ── （causal-analysis-agent 阶段2/3b：result.sectors 已迁移为 AnalyzeOutput
//     {form,lens,analysis} 按形态多态；review.verdict 迁移为 ReviewVerdict。
//     旧的 EvolutionAnalysis / EvolutionPosition / EvolutionSignal /
//     EvolutionEvidence / FinancialView / PositionChange 类型已随消费端
//     EvolutionReport → CausalAnalysisReport 重构一并移除。ResultSector 仍保留
//     给 DebateSection 的金融辩论板块切片用。）───────────────────────────────

/** 表2 列表项（slim 版，后端 listResults 返回）。 */
export interface ResultSummaryRow {
	id: number;
	evolution_assessment: string;
	/** 字段名不变；内容为 AnalyzeOutput（按 form 多态）。 */
	sectors: AnalyzeOutput | null;
	tool_calls_count: number;
	session_id: string;
	created_at: string;
}

/** 表2 详情（后端 getResult 返回，含 tool_calls / input_snapshot / causal_chain）。 */
export interface ResultDetailRow {
	id: number;
	persistent_topic_id?: number;
	evolution_assessment: string;
	/** 字段名不变；内容为 AnalyzeOutput（按 form 多态）。 */
	sectors: AnalyzeOutput | null;
	causal_chain: string | null;
	/** 工具调用记录（名/参数/返回摘要/耗时），LLM 产物，结构未冻结。 */
	tool_calls: unknown;
	/** 编排元数据（读了哪些 context 层 + as_of + section 范围 + 引用 review id）。 */
	input_snapshot: unknown;
	session_id: string;
	created_at: string;
}

/** POST .../results/trigger 的异步响应（fix-board-analysis-material：
 * 触发立即返回，分析后台跑，轮询 getAnalysisStatus 拿结果）。 */
export interface TriggerStartedResponse {
	status: "started";
	scope: "board" | "topic" | string;
	target_id: number;
	/** 唯一任务身份（board-level-deep-analysis 5.x：202 返回，供按 job_id 精确轮询）。 */
	job_id?: string;
	/** 任务种类：board_brief | board_investigation | topic_analysis。 */
	job_kind?: AnalysisJobKind | string;
	/** 仅 409 冲突体的 data 里出现（当前任务仍在跑）。 */
	running?: boolean;
}

/** 结果种类（repository.EffectiveResultKind）：board 档三选一。 */
export type ResultKind =
	| "board_brief"
	| "board_investigation"
	| "legacy_board_analysis"
	| string;

/** 异步任务种类（handler.AnalysisJobKind 常量）。 */
export type AnalysisJobKind =
	| "board_brief"
	| "board_investigation"
	| "topic_analysis"
	| string;

/** GET /enrichment/analysis-status?job_id= 的响应（handler.AnalysisStatus 镜像）。 */
export interface AnalysisJobStatus {
	job_id: string;
	job_kind: AnalysisJobKind;
	scope: "board" | "topic" | string;
	target_id: number;
	running: boolean;
	started_at?: string;
	finished?: boolean;
	error?: string;
	result_id?: number;
}

/** GET /enrichment/analysis-status 的响应（scope=board|topic，当前/最近任务；
 * 无任务时后端返 idle 骨架，无 job_id/job_kind）。 */
export interface AnalysisStatusRow {
	scope: "board" | "topic";
	target_id: number;
	running: boolean;
	/** 任务身份：存在历史/在跑任务时才有（重进恢复用它转按 job_id 精确轮询）。 */
	job_id?: string;
	job_kind?: AnalysisJobKind;
	started_at?: string;
	finished?: boolean;
	error?: string;
	result_id?: number;
}

// ── Table 3: topic_enrichment_review ────────────────────────────────────────

/** review verdict 兑现度结算的单条（jsonb 数组元素，review judge 输出）。 */
export type VerdictMark = "hit" | "part" | "miss";

export interface ReviewVerdictItem {
	sector?: string;
	predicted_dir?: Direction | string;
	actual?: Direction | string;
	mark?: VerdictMark | string;
	[key: string]: unknown;
}

/** 两次 result 之间的认知增量（反思）。 */
export interface ReviewRow {
	id: number;
	persistent_topic_id?: number;
	prev_result_id: number | null;
	curr_result_id: number;
	/** 反思结算（causal-analysis-agent：should_review + new_findings + 确定性漂移）。 */
	verdict?: ReviewVerdict | null;
	deviation_summary: string;
	affected_context: ContextGranularity | null;
	confidence: number | null;
	applied: boolean;
	source: EnrichmentSource;
	created_at: string;
	updated_at?: string;
}

// ── Board data sources ──────────────────────────────────────────────────────

/** 板块与数据源的绑定行。 */
export interface DataSourceRow {
	id: number;
	semantic_board_id: number;
	source_type: string;
	/** 板块级参数，schema 由 source_type 决定。 */
	config: Record<string, unknown>;
	enabled: boolean;
	created_at?: string;
	updated_at?: string;
}

// ── Stock debate (FinGenius) ────────────────────────────────────────────────

/** 个股多角色辩论结果（design §4.2b，append-only by result+sector+code）。 */
export interface StockDebateResult {
	id: number;
	topic_enrichment_result_id: number;
	persistent_topic_id?: number;
	sector: string;
	code: string;
	name?: string;
	/** 多角色辩论最终结论方向。 */
	verdict?: Direction | string;
	/** 共识强度（如 majority/unanimous/split）。 */
	consensus?: string;
	/** 参与方（jsonb，结构未冻结）。 */
	agents?: unknown;
	/** 投票（jsonb，结构未冻结）。 */
	votes?: unknown;
	/** FinGenius 原始研究报告（jsonb）。 */
	fingenius_research?: unknown;
	/** FinGenius 原始辩论记录（jsonb）。 */
	fingenius_battle?: unknown;
	fingenius_task_id?: string;
	/** done=已提炼，failed=提炼失败（降级展示原文），running=进行中。 */
	distill_status?: "done" | "failed" | "running" | string;
	html_content?: string;
	created_at?: string;
	[key: string]: unknown;
}

// ── Causal analysis schema（causal-analysis-agent 阶段2 新 result.sectors）──
// 后端 result.sectors 形状为 {form,lens,analysis}（按 form 多态）。本节是该形状的
// TS 镜像；3b 已把 ResultDetailRow/ResultSummaryRow/TriggerEnrichmentResponse.sectors
// 与 ReviewRow.verdict 迁移到 AnalyzeOutput / ReviewVerdict，旧 Evolution* 类型已移除。

/** 因果叙事形态：event_chain=事件链 / theme_vein=主题脉络 / single_point=单点深入 / structural=结构演化 / sparse=稀疏。 */
export type AnalyzeForm =
	| "event_chain"
	| "theme_vein"
	| "single_point"
	| "structural"
	| "sparse";

/** 确定性档位：high=高 / medium=中 / low=低 / question=存疑。 */
export type AnalyzeCert = "high" | "medium" | "low" | "question";

/**
 * 引用来源：news=订阅源新闻报道 / tool=agent 工具查证 / web=web_search 网页 / page=fetch_page 正文。
 * （web/page 为深度层 evidence_chain 的可核查外部证据；既有事实层仍只用 news/tool。）
 * 加 Analyze 前缀避免与 Vue 的 Ref<T>（本文件大量使用）混淆。
 */
export interface AnalyzeRef {
	source_type: "news" | "tool" | "web" | "page";
	ref: string;
	quote?: string;
	[key: string]: unknown;
}

/** 洞察条目（确定性 + 标题 + 逻辑链 + 证据，可选 web 核验引用）。 */
export interface AnalyzeInsight {
	cert: AnalyzeCert | string;
	title: string;
	logic: string;
	evidence: AnalyzeRef[];
	web_verified?: AnalyzeRef[];
	[key: string]: unknown;
}

// ── 深度层 depth（data-enrichment-structural-depth：非 sparse 形态强制产出）──
/** 多层机制拆解的单层（子机制名 + 深层逻辑 + 依据）。 */
export interface MechanismLayer {
	layer: string;
	deep_logic: string;
	basis: string;
	[key: string]: unknown;
}

/** 历史类比（案例 + 机制类比 + 何处不同）。 */
export interface HistoricalAnalogy {
	case: string;
	mechanism: string;
	diff: string;
	[key: string]: unknown;
}

/** 范式转折判断（无迹象则 depth.regime_shift 为 null）。 */
export interface RegimeShift {
	judgment: string;
	evidence: string;
	[key: string]: unknown;
}

/** 可核查证据链条目：news=分层新闻 / web=web_search 网页 / page=fetch_page 正文 / lane=板块内泳道。web/page 带 url+quote+institution+date；lane 带 ref=lane_id。 */
export interface EvidenceChainItem {
	source_type: "news" | "web" | "page" | "lane" | string;
	ref?: string;
	url?: string;
	/** 原文摘录（非 AI 转述）。 */
	quote?: string;
	institution?: string;
	date?: string;
	/** 可选两级分类：quote=原文摘录 / series=数据序列 / chart=图表。 */
	kind?: string;
	/** lane 证据的论据说明（该泳道贡献了什么）。 */
	lane_note?: string;
	[key: string]: unknown;
}

/**
 * 分析深度层（"内部看美国"分析基因映射）。
 * 非 sparse 形态由后端强制产出；前端按可选消费——旧结果无 depth 则降级不渲染、不报错。
 */
export interface AnalyzeDepth {
	/** 系统重定位：一句话把话题放进哪个大系统讲。 */
	system_reframe: string;
	mechanism_layers: MechanismLayer[];
	historical_analogy: HistoricalAnalogy[];
	regime_shift?: RegimeShift | null;
	/** 反过度解读边界：明确"目前还不能下结论"的范围（后端校验非空）。 */
	boundary: string;
	evidence_chain: EvidenceChainItem[];
	[key: string]: unknown;
}

// ── Board-level analysis（board-level-deep-analysis D8：scope=board 五字段）──

/** board_interpret 候选命题：钩子（近期真实异常）升维成结构命题。 */
export interface BoardCandidate {
	thesis: string;
	hook: string;
	angle: string;
	[key: string]: unknown;
}

/** 论证层（层级递进机制层的一层：表层现象逐层向下挖到结构性根因）。 */
export interface BoardArgumentLayer {
	layer: string;
	deep_logic: string;
	basis: string;
	[key: string]: unknown;
}

/** 论文式论证骨架：intro 定调 → layers 层级递进 → boundary 边界 → conclusion 收束。 */
export interface BoardArgument {
	intro: string;
	layers: BoardArgumentLayer[];
	boundary: string;
	conclusion: { cert: string; judgment: string; [key: string]: unknown };
	[key: string]: unknown;
}

/** 版块档 sectors：{scope, thesis, candidates, argument, depth, lane_refs} + interpret 元信息。 */
export interface BoardSectors {
	scope: "board";
	/** board=正常结构分析 / sparse=素材不足诚实降级。 */
	form: "board" | "sparse" | string;
	thesis: string;
	angle?: string;
	candidates: BoardCandidate[];
	chosen_index: number;
	reason?: string;
	/** 论证骨架（sparse 档可能缺席，前端降级渲染）。 */
	argument?: BoardArgument;
	/** 深度层（旧结果无此字段则降级不渲染）。 */
	depth?: AnalyzeDepth;
	lane_refs: Array<{ lane_id: number; note?: string; board_id?: number; [key: string]: unknown }>;
	/** interpret 元信息：degraded=LLM 不可用机械降级 / all_sparse=全部泳道素材不足。 */
	interpret_meta?: {
		degraded?: boolean;
		degraded_why?: string;
		all_sparse?: boolean;
		[key: string]: unknown;
	};
	[key: string]: unknown;
}

// ── Board brief（board-level-deep-analysis 5.x：result_kind=board_brief 的 sectors）──

/** 泳道关系类型枚举（D3）：同期发生不等于因果，possible_causal 置信最多 medium。 */
export type BoardRelationType =
	| "common_driver"
	| "possible_causal"
	| "divergent"
	| "context_only"
	| "unclear"
	| string;

/** 关键观察：内部新闻记忆读数，非外部核查事实；basis 指向泳道素材。 */
export interface BoardBriefObservation {
	id: string;
	/** 针对的活跃泳道（幽灵 lane 已被后端 sanitizeEvidenceChain 清理）。 */
	lane_id: number;
	statement: string;
	/** 依据说明（新闻记忆读数来源）。 */
	basis: string;
	/** 截止日期 YYYY-MM-DD。 */
	as_of_date: string;
	[key: string]: unknown;
}

/** 跨泳道关系：≥2 条活跃 lane + 类型 + 解释 + 置信 + 观察依据（o-id 引用）。 */
export interface BoardBriefRelationship {
	lane_ids: number[];
	type: BoardRelationType;
	explanation: string;
	confidence: "low" | "medium" | "high" | string;
	/** 引用的 observation id 列表（悬空引用已被后端剔除）。 */
	evidence_refs: string[];
	[key: string]: unknown;
}

/**
 * 已确认跨版块关系（服务端机械装配，LLM 不生成；add-evidence-backed-cross-board-relations 5.3）。
 * direction: outgoing=本版块是 source / incoming=本版块是 target。
 */
export interface BoardBriefCrossRelation {
	relation_id: number;
	other_board_id: number;
	direction: "outgoing" | "incoming" | string;
	relation_type: string;
	claim: string;
	quality_grade: "high" | "medium" | "low" | "none" | string;
	confirmed_at?: string;
	expires_at?: string;
	evidence_url?: string;
	evidence_quote?: string;
	[key: string]: unknown;
}

/** 未知项：问题 + 为什么不确定 + 需要什么证据。 */
export interface BoardBriefUncertainty {
	question: string;
	why_uncertain: string;
	needed_evidence: string;
	[key: string]: unknown;
}

/** 研究问题候选（0-4）：具体、可被证据支持或削弱；空数组是正常态非失败。 */
export interface BoardBriefQuestion {
	id: string;
	question: string;
	rationale: string;
	related_lane_ids: number[];
	[key: string]: unknown;
}

/** 简报 sectors（D3 schema）：无 thesis、无固定机制层、无历史类比要求。 */
export interface BoardBriefSectors {
	scope: "board";
	result_kind: "board_brief" | string;
	/** 1-3 句人话概览。 */
	summary: string;
	observations: BoardBriefObservation[];
	relationships: BoardBriefRelationship[];
	uncertainties: BoardBriefUncertainty[];
	research_questions: BoardBriefQuestion[];
	lane_refs: Array<{ lane_id: number; note?: string; board_id?: number; [key: string]: unknown }>;
	/** 已确认跨版块关系（机械字段，旧简报缺失时降级为空）。 */
	cross_board_relations?: BoardBriefCrossRelation[];
	/** 机械降级（LLM 失败后只产观察不造关系）时为 true。 */
	degraded?: boolean;
	degraded_why?: string;
	/** 全部泳道素材稀薄：observations/relationships/questions 均为空。 */
	all_sparse?: boolean;
	retry_reason?: string;
	review_digest?: string;
	[key: string]: unknown;
}

// ── Board investigation（board-level-deep-analysis 5.6：result_kind=board_investigation 的 sectors）──

/** 调查问题（D5）：generated=简报候选问题（文本以父简报为准）/ custom=用户自填。 */
export interface BoardInvestigationQuestion {
	/** 显示用 id（generated 取简报候选 id；custom 可空）。 */
	id?: string;
	text: string;
	source: "generated" | "custom" | string;
	[key: string]: unknown;
}

/** 假设终评五态（D4）：supported|plausible|insufficient|weakened|refuted，允许 H0 最可信、允许全 insufficient。 */
export type InvestigationAssessment =
	| "supported"
	| "plausible"
	| "insufficient"
	| "weakened"
	| "refuted"
	| string;

/** 置信三档（与 conclusion.confidence 共用）。 */
export type InvestigationConfidence = "low" | "medium" | "high" | string;

/** 最终竞争假设（含评估）：derived_from 追溯初始假设 id；support/counter 是证据 id 引用。 */
export interface BoardInvestigationHypothesis {
	id: string;
	label: string;
	/** 零假设（H0）："变化之间没有可下结论的关联"。 */
	is_null?: boolean;
	derived_from?: string[];
	assessment: InvestigationAssessment;
	confidence: InvestigationConfidence;
	scope?: string;
	support_evidence: string[];
	counter_evidence: string[];
	/** 诚实缺口（非 null，空数组是正常态）。 */
	gaps: string[];
	[key: string]: unknown;
}

/** 有界终局结论（D4）：summary/confidence/scope/boundary 四字段，无固定机制层配额。 */
export interface BoardInvestigationConclusion {
	summary: string;
	confidence: InvestigationConfidence;
	scope?: string;
	boundary?: string;
	[key: string]: unknown;
}

/** 调查证据条目（显式极性 supports/counters 携假设 id）；web/page 带 url+quote+institution+date，lane 的 ref=泳道编号（字符串）。 */
export interface BoardInvestigationEvidence {
	id: string;
	source_type: "news" | "web" | "page" | "lane" | string;
	ref?: string;
	url?: string;
	/** 原文逐字摘录（非 AI 转述）。 */
	quote?: string;
	institution?: string;
	date?: string;
	/** 可选两级分类：quote=原文摘录 / series=数据序列 / chart=图表。 */
	kind?: string;
	/** lane 证据的论据说明（该泳道贡献了什么）。 */
	lane_note?: string;
	supports?: string[];
	counters?: string[];
	[key: string]: unknown;
}

/** 注入方法卡留痕（机械写入，绝不采信 LLM 输出）：id+title+content_hash。 */
export interface BoardInvestigationMethodRef {
	id: number;
	title?: string;
	content_hash?: string;
	[key: string]: unknown;
}

/** 调查 sectors（D4 终局 schema）：question + hypotheses + conclusion + evidence_chain + lane_refs。 */
export interface BoardInvestigationSectors {
	scope: "board";
	result_kind: "board_investigation" | string;
	parent_briefing_id: number;
	question: BoardInvestigationQuestion;
	hypotheses: BoardInvestigationHypothesis[];
	conclusion: BoardInvestigationConclusion;
	evidence_chain: BoardInvestigationEvidence[];
	lane_refs: Array<{ lane_id: number; note?: string; board_id?: number; [key: string]: unknown }>;
	method_refs?: BoardInvestigationMethodRef[];
	retry_reason?: string;
	[key: string]: unknown;
}

/** 版块档 result 行（列表/详情同构，后端 serializeBoardResult）。
 * 5.x 起新增 result_kind / parent_result_id / question_key（board 档三 kind：
 * board_brief / board_investigation / legacy_board_analysis）；旧行由后端
 * EffectiveResultKind 兜底为 legacy，前端按 kind 分派视图。 */
export interface BoardAnalysisResultRow {
	id: number;
	analysis_scope: "board";
	result_kind?: ResultKind;
	parent_result_id?: number | null;
	question_key?: string | null;
	sectors: BoardSectors | BoardBriefSectors | BoardInvestigationSectors | null;
	tool_calls?: unknown;
	input_snapshot?: unknown;
	session_id?: string;
	created_at?: string;
	[key: string]: unknown;
}

// ── analysis body（按 form 多态：每个 variant 形状不同）────────────────────
/** event_chain 的 analysis 体：事实层 + 时间线 + 洞察层（+ 可选深度层）。 */
export interface AnalysisEventChain {
	fact_layer: Array<{
		claim: string;
		evidence: AnalyzeRef[];
		verified: boolean;
	}>;
	timeline: Array<{
		date: string;
		event: string;
		ref?: AnalyzeRef;
	}>;
	insight_layer: AnalyzeInsight[];
	/** 深度层（可选：旧结果无此字段，前端降级不渲染）。 */
	depth?: AnalyzeDepth;
}

/** theme_vein 的 analysis 体：脉络切片 + 跨脉络洞察（+ 可选深度层）。 */
export interface AnalysisThemeVein {
	veins: Array<{
		name: string;
		desc: string;
		evidence: AnalyzeRef[];
	}>;
	cross_insight: AnalyzeInsight[];
	/** 深度层（可选：旧结果无此字段，前端降级不渲染）。 */
	depth?: AnalyzeDepth;
}

/** single_point 的 analysis 体：单点深入（影响 + 证据，+ 可选深度层）。 */
export interface AnalysisSinglePoint {
	impact: {
		implication: string;
		ripple: string;
		benchmark: string;
	};
	evidence: AnalyzeRef[];
	/** 深度层（可选：旧结果无此字段，前端降级不渲染）。 */
	depth?: AnalyzeDepth;
}

/** structural 的 analysis 体：结构演化叙述 + 关键阶段（+ 可选深度层）。 */
export interface AnalysisStructural {
	evolution_narrative: string;
	phases: Array<{
		period: string;
		event: string;
		ref?: AnalyzeRef;
	}>;
	/** 深度层（可选：旧结果无此字段，前端降级不渲染）。 */
	depth?: AnalyzeDepth;
}

/** sparse 的 analysis 体：稀疏（提示 + 摘要）。 */
export interface AnalysisSparse {
	notice: string;
	summary: string;
}

/**
 * analysis 内容体联合。各 variant 无独立判别字段，窄化须依赖父 AnalyzeOutput.form
 * （switch output.form 后即可安全访问对应 analysis 专属结构）。
 */
export type AnalysisBody =
	| AnalysisEventChain
	| AnalysisThemeVein
	| AnalysisSinglePoint
	| AnalysisStructural
	| AnalysisSparse;

/**
 * result.sectors 新形状 {form,lens,analysis}（判别联合）。
 * form 为判别字段：switch (out.form) 后 TS 自动窄化到对应 variant，
 * 进而安全读取 out.analysis 的专属结构。lens 为分析视角标签（LLM 产物）。
 */
export interface AnalyzeEventChainOutput {
	form: "event_chain";
	lens: string;
	analysis: AnalysisEventChain;
}

export interface AnalyzeThemeVeinOutput {
	form: "theme_vein";
	lens: string;
	analysis: AnalysisThemeVein;
}

export interface AnalyzeSinglePointOutput {
	form: "single_point";
	lens: string;
	analysis: AnalysisSinglePoint;
}

export interface AnalyzeStructuralOutput {
	form: "structural";
	lens: string;
	analysis: AnalysisStructural;
}

export interface AnalyzeSparseOutput {
	form: "sparse";
	lens: string;
	analysis: AnalysisSparse;
}

export type AnalyzeOutput =
	| AnalyzeEventChainOutput
	| AnalyzeThemeVeinOutput
	| AnalyzeSinglePointOutput
	| AnalyzeStructuralOutput
	| AnalyzeSparseOutput;

/**
 * review.verdict 新形状（causal-analysis-agent 阶段2）。
 * 反思结算：是否需要重审 + 原因 + 新发现/被推翻的洞察 + 确定性漂移。
 * ReviewRow.verdict 已在 3b 迁移为本类型。
 */
export interface ReviewVerdict {
	should_review: boolean;
	reason?: string;
	new_findings?: string[];
	overturned?: string[];
	confidence_shift?: Array<{
		insight: string;
		from: AnalyzeCert | string;
		to: AnalyzeCert | string;
	}>;
	affected_context?: string | null;
	confidence?: number | null;
	[key: string]: unknown;
}

/**
 * 报告追问 QA 条目（append-only，归属一个不可变 result 快照；可 sediment 沉淀）。
 * 对齐后端 TopicEnrichmentQA（table topic_enrichment_qa）。
 */
export interface TopicEnrichmentQA {
	id: number;
	topic_enrichment_result_id: number;
	question: string;
	answer: string;
	/** 工具调用记录（jsonb，结构未冻结）。 */
	tool_calls: unknown;
	source: EnrichmentSource;
	sedimented: boolean;
	created_at: string;
}

// ── Request body types ──────────────────────────────────────────────────────

export interface CreateReviewBody {
	curr_result_id: number;
	deviation_summary: string;
	prev_result_id?: number;
}

export interface UpsertDataSourceBody {
	source_type: string;
	config?: Record<string, unknown>;
	enabled?: boolean;
}

/** POST .../analysis/investigations/trigger 的 body（5.x）。
 * briefing_result_id 必填；question_id 非空 → generated（文本以父简报为准），
 * 否则 question 文本 → custom。二者全空后端 400。 */
/** POST .../relations/discover 的 body（source 文本永远不出现在客户端——服务端从父简报重取）。 */
export interface RelationDiscoverBody {
	briefing_result_id: number;
	source_kind: "observation" | "question" | string;
	source_key: string;
}

/** 关系证据（RelationEvidence 镜像）。 */
export interface BoardRelationEvidence {
	ref?: string;
	tool?: string;
	url?: string;
	title?: string;
	quote?: string;
	institution?: string;
	date?: string;
	use?: "support" | "counter" | string;
	verified?: boolean;
	[key: string]: unknown;
}

/** 跨版块关系行（serializeCrossBoardRelation 镜像；status 生命周期
 * unresolved/proposed/confirmed/dismissed/expired）。 */
export interface BoardRelationRow {
	id: number;
	run_id?: number | null;
	source_board_id: number;
	target_board_id?: number | null;
	target_lane_id?: number | null;
	target_concept?: string | null;
	relation_type: string;
	claim: string;
	mechanism?: string | null;
	verification_verdict: "supported" | "contested" | "insufficient" | "rejected" | string;
	quality_grade: "high" | "medium" | "low" | "none" | string;
	evidence?: BoardRelationEvidence[];
	counterevidence?: BoardRelationEvidence[];
	status: "unresolved" | "proposed" | "confirmed" | "dismissed" | "expired" | string;
	suggestion_hash?: string;
	evidence_version?: string;
	expires_at?: string | null;
	confirmed_at?: string | null;
	dismissed_at?: string | null;
	expired_at?: string | null;
	dismiss_reason?: string | null;
	created_at?: string;
	updated_at?: string;
}

/** 关系详情：行 + 映射快照 / gap / run 审计。 */
export interface BoardRelationDetail extends BoardRelationRow {
	mapping_snapshot?: unknown;
	gaps?: unknown;
	run?: {
		id: number;
		status: string;
		trigger_kind: string;
		source_kind: string;
		source_key: string;
		budget_snapshot?: unknown;
		gaps?: unknown;
		error?: string;
		created_at?: string;
	} | null;
}

/** re-resolve 响应（RelationReResolveOutput 镜像）。 */
export interface RelationReResolveResult {
	relation_id?: number;
	new_status?: string;
	outcome?: string;
	[key: string]: unknown;
}

export interface TriggerInvestigationBody {
	briefing_result_id: number;
	question_id?: string;
	question?: string;
}

/** POST .../results/:id/qa 的响应（对齐后端 service.QAAnswer）。 */
export interface AskQAResponse {
	answer: string;
	/** 工具调用记录（结构未冻结，LLM 产物）。 */
	tool_calls: unknown;
	/** 双类引用：news=报告上下文 / tool=工具查证结果。 */
	refs: AnalyzeRef[];
}

// ── API factory ─────────────────────────────────────────────────────────────

export function useBoardEnrichmentApi() {
	// ── Table 1: contexts (period-archival: keyed by granularity+period) ────
	/** 拉所有 contexts；传 granularity 则按粒度过滤（用于周期筛选器翻历史）。 */
	async function listContexts(
		topicId: number,
		granularity?: ContextGranularity,
	): Promise<ApiResponse<ContextRow[]>> {
		const qs = granularity ? `?granularity=${granularity}` : "";
		return apiClient.get(
			`/persistent-topics/${topicId}/enrichment/contexts${qs}`,
		);
	}

	/** 取具体周期：GET /contexts/:granularity/:period（period 如 2026-W27）。 */
	async function getContext(
		topicId: number,
		granularity: ContextGranularity,
		period: string,
	): Promise<ApiResponse<ContextRow>> {
		return apiClient.get(
			`/persistent-topics/${topicId}/enrichment/contexts/${granularity}/${period}`,
		);
	}

	/** 编辑具体周期：PUT /contexts/:granularity/:period。 */
	async function updateContext(
		topicId: number,
		granularity: ContextGranularity,
		period: string,
		body: { content: string },
	): Promise<ApiResponse<ContextRow>> {
		return apiClient.put(
			`/persistent-topics/${topicId}/enrichment/contexts/${granularity}/${period}`,
			body,
		);
	}

	/** 重生成：POST /contexts/:granularity/regenerate?period=…（period 省略=当前周期）。 */
	async function regenerateContext(
		topicId: number,
		granularity: ContextGranularity,
		period?: string,
	): Promise<ApiResponse<ContextRow>> {
		const qs = period ? `?period=${period}` : "";
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/contexts/${granularity}/regenerate${qs}`,
		);
	}

	// ── Table 2: results ────────────────────────────────────────────────────
	async function listResults(
		topicId: number,
	): Promise<ApiResponse<ResultSummaryRow[]>> {
		return apiClient.get(`/persistent-topics/${topicId}/enrichment/results`);
	}

	async function getResult(
		topicId: number,
		id: number,
	): Promise<ApiResponse<ResultDetailRow>> {
		return apiClient.get(
			`/persistent-topics/${topicId}/enrichment/results/${id}`,
		);
	}

	async function triggerEnrichment(
		topicId: number,
		/** D8 下钻：版块报告 lane 引用点击后预填视角（可选）。 */
		prefillLens?: string,
	): Promise<ApiResponse<TriggerStartedResponse>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/results/trigger`,
			prefillLens ? { prefill_lens: prefillLens } : undefined,
		);
	}

	// ── Board-level analysis（board-level-deep-analysis D8 + 5.x）───────
	/** 触发版块简报（原「分析板块」，5.3 起后端默认产 board_brief）。
	 * 202 → data {status,job_id,job_kind:'board_brief',scope,target_id}；
	 * 409 → success=false 但 data 携当前在跑任务身份（按其 job_id 恢复轮询）。 */
	async function triggerBoardAnalysis(
		boardId: number,
	): Promise<ApiResponse<TriggerStartedResponse>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/trigger`,
		);
	}

	/** 按唯一 job_id 精确查一个任务（含已完成；未知 job_id → 404）。 */
	async function getAnalysisStatusByJobId(
		jobId: string,
	): Promise<ApiResponse<AnalysisJobStatus>> {
		return apiClient.get(`/enrichment/analysis-status?job_id=${encodeURIComponent(jobId)}`);
	}

	/** 按 scope+id 查当前/最近任务（重进恢复入口；无任务 = idle 骨架）。 */
	async function getAnalysisStatus(
		scope: "board" | "topic",
		id: number,
	): Promise<ApiResponse<AnalysisStatusRow>> {
		return apiClient.get(`/enrichment/analysis-status?scope=${scope}&id=${id}`);
	}

	/** 触发问题调查（board_investigation）。generated 问题传 question_id
	 * （文本以父简报为准）；custom 问题传 question 文本。
	 * 5.4/5.5 切片只定义契约供组件 emit 复用，不自动调用（5.6/5.7 接线）。 */
	async function triggerBoardInvestigation(
		boardId: number,
		payload: TriggerInvestigationBody,
	): Promise<ApiResponse<TriggerStartedResponse>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/investigations/trigger`,
		payload,
		);
	}

	// ── 跨版块关系发现（add-evidence-backed-cross-board-relations 4.x/6.x）──

	/** 从简报观察/研究问题手动发现跨版块关系。
	 * 202 → data {status,job_id,job_kind:'relation_discovery',scope:'relation',target_id}；
	 * 409 → success=false 但 data 携当前在跑任务身份（按其 job_id 恢复轮询）。 */
	async function triggerRelationDiscovery(
		boardId: number,
		payload: RelationDiscoverBody,
	): Promise<ApiResponse<TriggerStartedResponse>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/relations/discover`,
			payload,
		);
	}

	/** 关系列表（双侧匹配；status 逗号分隔过滤 unresolved/proposed/confirmed/dismissed/expired）。 */
	async function listBoardRelations(
		boardId: number,
		status?: string,
		limit?: number,
	): Promise<ApiResponse<BoardRelationRow[]>> {
		const params = new URLSearchParams();
		if (status) params.set("status", status);
		if (limit) params.set("limit", String(limit));
		const qs = params.toString() ? `?${params.toString()}` : "";
		return apiClient.get(
			`/semantic-boards/${boardId}/enrichment/analysis/relations${qs}`,
		);
	}

	/** 关系详情（列表字段 + mapping_snapshot / gaps / run 审计链接）。 */
	async function getBoardRelation(
		boardId: number,
		relationId: number,
	): Promise<ApiResponse<BoardRelationDetail>> {
		return apiClient.get(
			`/semantic-boards/${boardId}/enrichment/analysis/relations/${relationId}`,
		);
	}

	/** 确认 proposed 关系（TTL 内生效；非 proposed → 409）。 */
	async function confirmBoardRelation(
		boardId: number,
		relationId: number,
	): Promise<ApiResponse<BoardRelationRow>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/relations/${relationId}/confirm`,
		);
	}

	/** 驳回 proposed/unresolved 关系（reason 必填；进入冷却）。 */
	async function dismissBoardRelation(
		boardId: number,
		relationId: number,
		reason: string,
	): Promise<ApiResponse<BoardRelationRow>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/relations/${relationId}/dismiss`,
			{ reason },
		);
	}

	/** 重解析 unresolved 关系（目标板块可能已出现）。 */
	async function reResolveBoardRelation(
		boardId: number,
		relationId: number,
	): Promise<ApiResponse<RelationReResolveResult>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/relations/${relationId}/re-resolve`,
		);
	}

	async function listBoardAnalysisResults(
		boardId: number,
		/** 可选 kind 过滤：board_brief | board_investigation | legacy_board_analysis；缺省 = 全部。 */
		kind?: "board_brief" | "board_investigation" | "legacy_board_analysis",
	): Promise<ApiResponse<BoardAnalysisResultRow[]>> {
		const qs = kind ? `?kind=${kind}` : "";
		return apiClient.get(
			`/semantic-boards/${boardId}/enrichment/analysis/results${qs}`,
		);
	}

	async function getBoardAnalysisResult(
		boardId: number,
		id: number,
	): Promise<ApiResponse<BoardAnalysisResultRow>> {
		return apiClient.get(
			`/semantic-boards/${boardId}/enrichment/analysis/results/${id}`,
		);
	}

	// ── Table 3: reviews ────────────────────────────────────────────────────
	async function listReviews(
		topicId: number,
	): Promise<ApiResponse<ReviewRow[]>> {
		return apiClient.get(`/persistent-topics/${topicId}/enrichment/reviews`);
	}

	async function createReview(
		topicId: number,
		body: CreateReviewBody,
	): Promise<ApiResponse<ReviewRow>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/reviews`,
			body,
		);
	}

	async function updateReviewDeviation(
		topicId: number,
		id: number,
		body: { deviation_summary: string },
	): Promise<ApiResponse<ReviewRow>> {
		return apiClient.put(
			`/persistent-topics/${topicId}/enrichment/reviews/${id}`,
			body,
		);
	}

	async function applyReview(
		topicId: number,
		id: number,
	): Promise<ApiResponse<ReviewRow>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/reviews/${id}/apply`,
		);
	}

	// ── Stock debate (FinGenius) ────────────────────────────────────────────
	// 触发：不传 body，后端默认从 result.sectors 提取 symbols 自动跑。
	async function triggerDebate(
		topicId: number,
		resultId: number,
	): Promise<ApiResponse<{ debates: StockDebateResult[] }>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/results/${resultId}/debates`,
		);
	}

	async function listDebates(
		topicId: number,
		resultId: number,
	): Promise<ApiResponse<StockDebateResult[]>> {
		return apiClient.get(
			`/persistent-topics/${topicId}/enrichment/results/${resultId}/debates`,
		);
	}

	// ── Board data sources ──────────────────────────────────────────────────
	async function listDataSources(
		boardId: number,
	): Promise<ApiResponse<DataSourceRow[]>> {
		return apiClient.get(`/semantic-boards/${boardId}/data-sources`);
	}

	async function upsertDataSource(
		boardId: number,
		body: UpsertDataSourceBody,
	): Promise<ApiResponse<DataSourceRow>> {
		return apiClient.put(`/semantic-boards/${boardId}/data-sources`, body);
	}

	async function deleteDataSource(
		boardId: number,
		sourceType: string,
	): Promise<ApiResponse<{ deleted: boolean }>> {
		return apiClient.delete(
			`/semantic-boards/${boardId}/data-sources/${sourceType}`,
		);
	}

	// ── QA (causal-analysis-agent 阶段3：报告追问 + 沉淀) ─────────────────────
	/** 问一轮：POST .../results/:id/qa body {question} → {answer,tool_calls,refs}。 */
	async function askQA(
		topicId: number,
		resultId: number,
		question: string,
	): Promise<ApiResponse<AskQAResponse>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/results/${resultId}/qa`,
			{ question },
		);
	}

	/** 多轮历史：GET .../results/:id/qa → TopicEnrichmentQA[]（oldest first）。 */
	async function listQA(
		topicId: number,
		resultId: number,
	): Promise<ApiResponse<TopicEnrichmentQA[]>> {
		return apiClient.get(
			`/persistent-topics/${topicId}/enrichment/results/${resultId}/qa`,
		);
	}

	/** 沉淀一轮：POST .../qa/:id/sediment → 更新后的 TopicEnrichmentQA（sedimented=true）。 */
	async function sedimentQA(
		topicId: number,
		qaId: number,
	): Promise<ApiResponse<TopicEnrichmentQA>> {
		return apiClient.post(
			`/persistent-topics/${topicId}/enrichment/qa/${qaId}/sediment`,
		);
	}

	// ── Board QA（board-level-deep-analysis 6.2：版块报告追问，design D5）─────
	// QA 按 result id 工作：三种 board kind（brief/investigation/legacy）均可
	// 追问——“只读”指 result 行不可变，QA 是独立 append-only 行，走 board 路由。
	/** 问一轮（版块档）：POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa。 */
	async function askBoardQA(
		boardId: number,
		resultId: number,
		question: string,
	): Promise<ApiResponse<AskQAResponse>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/results/${resultId}/qa`,
			{ question },
		);
	}

	/** 多轮历史（版块档）：GET 同路径 → TopicEnrichmentQA[]（oldest first）。 */
	async function listBoardQA(
		boardId: number,
		resultId: number,
	): Promise<ApiResponse<TopicEnrichmentQA[]>> {
		return apiClient.get(
			`/semantic-boards/${boardId}/enrichment/analysis/results/${resultId}/qa`,
		);
	}

	/** 沉淀一轮（版块档）：POST .../results/:rid/qa/:qid/sediment（sediment 路由按 result 定位）。 */
	async function sedimentBoardQA(
		boardId: number,
		resultId: number,
		qaId: number,
	): Promise<ApiResponse<TopicEnrichmentQA>> {
		return apiClient.post(
			`/semantic-boards/${boardId}/enrichment/analysis/results/${resultId}/qa/${qaId}/sediment`,
		);
	}

	return {
		listContexts,
		getContext,
		updateContext,
		regenerateContext,
		listResults,
		getResult,
		triggerEnrichment,
		// board-level analysis (board-level-deep-analysis)
		triggerBoardAnalysis,
		getAnalysisStatus,
		getAnalysisStatusByJobId,
		triggerBoardInvestigation,
		triggerRelationDiscovery,
		listBoardRelations,
		getBoardRelation,
		confirmBoardRelation,
		dismissBoardRelation,
		reResolveBoardRelation,
		listBoardAnalysisResults,
		getBoardAnalysisResult,
		listReviews,
		createReview,
		updateReviewDeviation,
		applyReview,
		triggerDebate,
		listDebates,
		listDataSources,
		upsertDataSource,
		deleteDataSource,
		// qa (causal-analysis-agent 阶段3)
		askQA,
		listQA,
		sedimentQA,
		// board qa (board-level-deep-analysis 6.2)
		askBoardQA,
		listBoardQA,
		sedimentBoardQA,
	};
}
