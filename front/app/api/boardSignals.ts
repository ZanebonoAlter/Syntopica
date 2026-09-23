import { apiClient } from "./client";
import type { ApiResponse } from "~/types";

/**
 * 板块信号解读报告（board-signal-reports）API client。
 *
 * 两阶段人工流程：手动「发现信号」只保存候选（discovery job）；
 * 用户逐条点「深入分析」才启动研究（research job，≤40 轮），成功后
 * 产出不可变 signal_report。全程无 review/judge 字段。
 *
 * 契约源：openspec/changes/board-signal-reports/phase-2a-report.md ②
 * 与 phase-2b-report.md ②（后端 handler/signal_discovery.go、
 * signal_research.go 逐字段对齐）。刻意不复用旧 brief 的
 * boardEnrichment.ts 类型——新链形状互斥，旧 API 模块零改动。
 *
 * 路由（挂现有 board 分析组，与旧 brief/investigation 共享 202/409 互斥）：
 *  POST /semantic-boards/:id/enrichment/analysis/signal-discoveries
 *  GET  /semantic-boards/:id/enrichment/analysis/signals
 *  POST /semantic-boards/:id/enrichment/analysis/signals/:candidateId/research
 *  GET  /semantic-boards/:id/enrichment/analysis/signal-reports
 *  GET  /semantic-boards/:id/enrichment/analysis/signal-reports/:rid
 *  job 轮询沿用 GET /enrichment/analysis-status?job_id=
 */

// ── Shared scalars ──────────────────────────────────────────────────────────

/** 信号发现的粒度：仅月/年（week 不支持）。 */
export type SignalGranularity = "month" | "year";

/** 服务端派生的候选状态（前端只读，勿自行推断；重启不会卡 researching）。 */
export type SignalCandidateStatus = "pending" | "researching" | "reported";

/** 研究进展行状态（tasks 4.7）：running=滚动更新中；abandoned=超时/失败保留；
 * superseded=成功落库报告后归档。 */
export type SignalResearchProgressStatus = "running" | "abandoned" | "superseded";

/** signal 专用 job 种类（旧 kind board_brief/board_investigation 等不在新链）。 */
export type SignalJobKind = "board_signal_discovery" | "board_signal_report";

/** discovery job 终态：discovered=候选≥1 / no_signal=0 条（正常完成）/ failed。 */
export type SignalDiscoveryOutcome =
  | "discovered"
  | "no_signal"
  | "failed";

/** research job 终态：预算耗尽**不是** failed（stop_reason 在 generation_meta）。 */
export type SignalResearchOutcome = "succeeded" | "failed";

/** job 运行阶段：discovery 走 prepare→detect；research 走 research→compose。 */
export type SignalJobPhase =
  | "prepare"
  | "detect"
  | "research"
  | "compose";

// ── Trigger 请求/响应 ───────────────────────────────────────────────────────

/** POST /signal-discoveries 请求体。 */
export interface SignalDiscoveryBody {
  granularity: SignalGranularity;
  period: string; // month=YYYY-MM / year=YYYY；非法/未来 → 400 无 job
}

/** POST /signals/:candidateId/research 请求体（仅此一个语义开关；
 * 其余浏览器自带字段一律被服务端忽略——候选快照权威）。 */
export interface SignalResearchBody {
  regenerate?: boolean;
}

/** 202 帧共用形状（discovery 与 research 字段差异见各自接口）。 */
export interface SignalTriggerStarted {
  status: "started";
  job_id: string;
  job_kind: SignalJobKind;
  scope: "board";
  target_id: number;
  granularity: SignalGranularity;
  period: string;
  /** 仅 research 202 帧出现。 */
  candidate_id?: number;
}

/** research 幂等复用 200 帧：该候选已有成功报告且未显式 regenerate，零新 LLM。 */
export interface SignalAlreadyReported {
  status: "already_reported";
  result_id: number;
}

// ── Job 状态（GET /enrichment/analysis-status?job_id= 的 signal 扩展）───────

/**
 * signal job 状态：在旧 AnalysisStatus 骨架上新增 signal 专用字段
 * （phase/outcome/error_stage/…，后端全 omitempty——旧 kind 输出零变化，
 * 因此这些键运行中/终态才出现，类型全部可选）。
 * - result_id：仅 research job succeeded 存在；discovery 永不出现。
 * - candidate_count：no_signal 时为 0，omitempty 下键省略。
 */
export interface SignalAnalysisJobStatus {
  job_id: string;
  job_kind: string; // board_signal_discovery | board_signal_report | board_brief …
  scope: "board" | string;
  target_id: number;
  running: boolean;
  started_at?: string;
  finished?: boolean;
  error?: string;
  result_id?: number;
  // ── signal 专用扩展（2a/2b 交付）──
  phase?: SignalJobPhase;
  outcome?: SignalDiscoveryOutcome | SignalResearchOutcome;
  /** 仅 failed 且阶段可知：prepare/detect/save（discovery）或 research/compose/save（research）。 */
  error_stage?: SignalJobPhase | "save";
  granularity?: SignalGranularity;
  period?: string;
  discovery_id?: number;
  candidate_id?: number;
  candidate_count?: number;
}

// ── 候选（GET /signals 行）──────────────────────────────────────────────────

export interface SignalCandidateRow {
  id: number;
  discovery_id: number;
  granularity: SignalGranularity;
  period: string;
  /** 异常是什么（detect 校验后输出）。 */
  signal: string;
  /** 值得查的原因。 */
  why_it_matters: string;
  /** 研究问题。 */
  research_question: string;
  /** 新闻依据＝候选白名单里的新闻切片 ID（展示用，不跳转）。 */
  evidence_refs: string[];
  /** 1~10 内部阈值分：不展示为可信度/收益预测。 */
  score: number;
  rationale: string;
  /** 发现时间来自批次。 */
  discovery_created_at: string;
  status: SignalCandidateStatus;
  /** 仅 reported 有值，其余 null。 */
  latest_result_id: number | null;
  /** 上次研究进展摘要（tasks 4.7「断了不能白跑」）：候选从未研究过 → null。
   * 研究失败错误提示从这里读「已保留 N 轮进展」。 */
  last_research_progress: SignalResearchProgressSummary | null;
}

/** 候选行/列表内的进展摘要（不含 ledger 全量账本）。 */
export interface SignalResearchProgressSummary {
  rounds_done: number;
  source_calls: number;
  calculation_calls: number;
  status: SignalResearchProgressStatus;
  /** 失败原因：timeout | error_stage 值；成功归档行无 → null。 */
  stop_reason: string | null;
  updated_at: string;
}

/** GET /signals/:candidateId/research-progress 全量进展（含 ledger jsonb）。 */
export interface SignalResearchProgress extends SignalResearchProgressSummary {
  job_id: string;
  candidate_id: number;
  semantic_board_id: number;
  granularity: SignalGranularity;
  period: string;
  /** 全量研究账本（calls/calculations/gaps，与报告 appendix 同源结构）。 */
  ledger: SignalReportPayload["appendix"];
  error: string | null;
  created_at: string;
}

// ── 报告 payload（sectors，schema_version=2）───────────────────────────────

/** 冻结的候选快照（signal_compose.go snapshot map 逐字段对齐）。 */
export interface SignalSnapshot {
  candidate_id: number;
  discovery_id: number;
  semantic_board_id: number;
  granularity: SignalGranularity;
  period: string;
  signal: string;
  why_it_matters: string;
  research_question: string;
  evidence_refs: string[];
  score: number;
  rationale: string;
  /** retrospective=历史周期（事后回顾，不声称 point-in-time 回测）。 */
  analysis_mode: string;
  cutoff: string;
}

/** 报告段：thesis/facts/causal 三段纯文本；implication 带结构化字段。 */
export interface SignalReportSection {
  kind: "thesis" | "facts" | "causal" | "implication";
  text: string;
  /** ── 仅 implication ── */
  verdict?: string;
  /** up|down|diverge|conditional。 */
  direction?: string;
  horizon?: string;
  trigger_condition?: string;
  self_doubt?: string;
}

export interface SignalReportChart {
  chart_id: string;
  /** line=折线（同系列≥2 非空点，已按期间升序）；comparison=对比柱。 */
  kind: "line" | "comparison";
  claim: string;
  /** 原值观测（cN:oM）或已成功计算（kN）。 */
  refs: string[];
}

/** 附录观测行：源维度字段随源不同（label/geo/flow/product…），保留宽口子。 */
export interface SignalAppendixObservation {
  observation_id: string; // "c1:o2"
  period?: string;
  value?: number | null;
  unit?: string;
  raw_value?: string;
  missing_reason?: string;
  [key: string]: unknown;
}

/** 附录调用行（真实来源时间/hash，完整筛选观测；filter_meta 见 cutoff 过滤层）。 */
export interface SignalAppendixCall {
  call_id: string; // "c1"
  question: string;
  tool: string;
  args?: Record<string, unknown>;
  status: string; // ok | error
  error?: string;
  retrieved_at?: string;
  last_modified?: string;
  source_sha256?: string;
  documents?: unknown;
  observations: SignalAppendixObservation[];
  filter_meta?: Record<string, unknown>;
}

/** 代码计算行（模型永不供数；value 为规范十进制字符串，half-up 最多 4 位）。 */
export interface SignalCalculation {
  calc_id: string; // "k1"
  op: "difference" | "percent_change" | "mean" | string;
  inputs: string[];
  expression: string;
  value?: string;
  unit?: string;
  precision: number;
  status: "ok" | "missing" | "rejected" | string;
  reason?: string;
  computed_at: string;
}

/** 缺口行（取数失败/无覆盖/材料 gap；如实记录不编造）。 */
export interface SignalAppendixGap {
  call_id?: string;
  tool?: string;
  reason: string;
}

export interface SignalReportGenerationMeta {
  session_id: string;
  attempts: number;
  retries: number;
  decisions: number;
  /** 真实取数次数（界面计数从这里读，不信模型自报）。 */
  source_calls: number;
  calculation_calls: number;
  /** finished | budget_exhausted（耗尽须展示缺口提示）。 */
  stop_reason: "finished" | "budget_exhausted" | string;
  analysis_mode: string;
  cutoff: string;
}

/** sectors payload（schema_version=2）：报告本体 + 代码生成附录 + 真实计数。 */
export interface SignalReportPayload {
  schema_version: number; // 2
  signal_snapshot: SignalSnapshot;
  report: {
    title: string;
    sections: SignalReportSection[];
    charts: SignalReportChart[];
  };
  appendix: {
    calls: SignalAppendixCall[];
    calculations: SignalCalculation[];
    gaps: SignalAppendixGap[];
  };
  generation_meta: SignalReportGenerationMeta;
}

// ── 报告列表/详情行 ─────────────────────────────────────────────────────────

/** GET /signal-reports 列表行（含完整 sectors；不含 tool_calls/input_snapshot）。 */
export interface SignalReportRow {
  id: number;
  analysis_scope: string; // board
  result_kind: "signal_report" | string;
  semantic_board_id: number | null;
  granularity: SignalGranularity;
  period: string;
  source_signal_id: number;
  sectors: SignalReportPayload;
  session_id: string;
  created_at: string;
}

/** 详情 = 列表行 + 完整工具日志与输入快照；整个响应无任何 review 字段。 */
export interface SignalReportDetail extends SignalReportRow {
  tool_calls?: unknown;
  input_snapshot?: unknown;
}

/** 列表查询参数（before_id 游标排他；limit 默认 20 最大 100）。 */
export interface SignalListParams {
  granularity: SignalGranularity;
  period: string;
  before_id?: number;
  limit?: number;
}

// ── API factory ─────────────────────────────────────────────────────────────

export function useBoardSignalsApi() {
  const boardBase = (boardId: number) =>
    `/semantic-boards/${boardId}/enrichment/analysis`;

  /** 触发信号发现。202→started；400 周期非法/未来或增强未开启（无 job）；
   * 409 同板块任一任务在跑（data 携 running job 身份，job_kind 可区分是谁）。 */
  async function triggerSignalDiscovery(
    boardId: number,
    body: SignalDiscoveryBody,
  ): Promise<ApiResponse<SignalTriggerStarted>> {
    return apiClient.post(`${boardBase(boardId)}/signal-discoveries`, body);
  }

  /** 候选列表（按周期，id 倒序；空数组=空态；空新批次不清旧记录）。 */
  async function listSignalCandidates(
    boardId: number,
    params: SignalListParams,
  ): Promise<ApiResponse<SignalCandidateRow[]>> {
    const qs = buildSignalQuery(params);
    return apiClient.get(`${boardBase(boardId)}/signals${qs}`);
  }

  /** 深入研究某条候选。404 候选不存在/跨板块；400 开关未开启；
   * 409 running（含自身重复点击，data 携 running 身份可恢复轮询）；
   * 200 already_reported（零新 LLM）；202 新 research job。 */
  async function triggerSignalResearch(
    boardId: number,
    candidateId: number,
    body: SignalResearchBody = {},
  ): Promise<ApiResponse<SignalTriggerStarted | SignalAlreadyReported>> {
    return apiClient.post(
      `${boardBase(boardId)}/signals/${candidateId}/research`,
      body,
    );
  }

  /** board 级当前/最近分析任务（重进恢复）：running 时携 job_id/kind/candidate_id，
   * 进程重启或无任务 → running=false（内存真相，天然排除死行残留）。 */
  async function getBoardAnalysisStatus(
    boardId: number,
  ): Promise<ApiResponse<SignalAnalysisJobStatus>> {
    return apiClient.get(`${boardBase(boardId)}/enrichment/analysis-status`);
  }

  /** 候选最近研究进展全量（tasks 4.7）。候选不存在/跨板块 → 404；
   * 候选从未研究 → 200 data=null。 */
  async function getSignalResearchProgress(
    boardId: number,
    candidateId: number,
  ): Promise<ApiResponse<SignalResearchProgress | null>> {
    return apiClient.get(
      `${boardBase(boardId)}/signals/${candidateId}/research-progress`,
    );
  }

  /** 报告列表（仅成功报告，id 倒序；source_signal_id 过滤候选版本序列）。 */
  async function listSignalReports(
    boardId: number,
    params: SignalListParams & { source_signal_id?: number },
  ): Promise<ApiResponse<SignalReportRow[]>> {
    const qs = buildSignalQuery(params);
    return apiClient.get(`${boardBase(boardId)}/signal-reports${qs}`);
  }

  /** 报告详情（不可变 payload；owner/kind 不匹配 404；无 review 字段）。 */
  async function getSignalReport(
    boardId: number,
    resultId: number,
  ): Promise<ApiResponse<SignalReportDetail>> {
    return apiClient.get(`${boardBase(boardId)}/signal-reports/${resultId}`);
  }

  /** 按 job_id 精确轮询（discovery/research 共用；未知 job_id → 404=后端重启丢内存任务）。 */
  async function getSignalJobStatus(
    jobId: string,
  ): Promise<ApiResponse<SignalAnalysisJobStatus>> {
    return apiClient.get(
      `/enrichment/analysis-status?job_id=${encodeURIComponent(jobId)}`,
    );
  }

  return {
    triggerSignalDiscovery,
    listSignalCandidates,
    triggerSignalResearch,
    getSignalResearchProgress,
    getBoardAnalysisStatus,
    listSignalReports,
    getSignalReport,
    getSignalJobStatus,
  };
}

/** 列表参数 → query string（跳过空值；无参数返回空串）。 */
function buildSignalQuery(params: object): string {
  const sp = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    sp.set(key, String(value));
  }
  const qs = sp.toString();
  return qs ? `?${qs}` : "";
}
