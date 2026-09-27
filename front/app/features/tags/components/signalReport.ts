// 可测纯逻辑（6.T3 前置）：引用解析 / appendix 查表 / 图表数据变换 /
// 状态派生全部在此导出，SignalReportView / SignalCandidateList 只做渲染。
import type {
  SignalAppendixCall,
  SignalAppendixObservation,
  SignalCandidateStatus,
  SignalCalculation,
  SignalGranularity,
  SignalReportChart,
  SignalReportPayload,
} from "~/api/boardSignals";

export type {
  SignalAppendixCall,
  SignalAppendixObservation,
  SignalCandidateStatus,
  SignalCalculation,
  SignalGranularity,
  SignalReportChart,
  SignalReportPayload,
} from "~/api/boardSignals";

// ── 正文引用 token 解析（6.T3 前置：可测纯逻辑）────────────────────────────
//
// 正文/图题中的引用原样存储：[[data:c1:o2]]（原值观测）、[[calc:k1]]（代码
// 计算）、[[news:<切片ID>]]（候选新闻依据）。展示数值由前端从 appendix
// 查表渲染——模型永远不提供展示数值。

export type SignalRefToken =
  | { type: "data"; ref: string }
  | { type: "calc"; ref: string }
  | { type: "news"; ref: string };

export type SignalTextSegment =
  | { kind: "text"; text: string }
  | { kind: "ref"; token: SignalRefToken };

const REF_TOKEN_RE = /\[\[(data|calc|news):([^\]]+)\]\]/g;

/** 把一段含引用 token 的正文切成纯文本段与引用段（无 token 时返回单文本段）。 */
export function parseSignalReferenceTokens(text: string): SignalTextSegment[] {
  const segments: SignalTextSegment[] = [];
  let cursor = 0;
  for (const match of text.matchAll(REF_TOKEN_RE)) {
    const index = match.index ?? 0;
    if (index > cursor) {
      segments.push({ kind: "text", text: text.slice(cursor, index) });
    }
    const kind = match[1];
    const ref = (match[2] ?? "").trim();
    if (kind === "data") segments.push({ kind: "ref", token: { type: "data", ref } });
    else if (kind === "calc") segments.push({ kind: "ref", token: { type: "calc", ref } });
    else segments.push({ kind: "ref", token: { type: "news", ref } });
    cursor = index + match[0].length;
  }
  if (cursor < text.length) {
    segments.push({ kind: "text", text: text.slice(cursor) });
  }
  return segments;
}

// ── appendix 查表（悬空引用兜底为 null，渲染层显示占位不崩）────────────────

export interface SignalDataRefView {
  ref: string;
  /** 原值按源精度显示（raw_value 优先），缺失显示标记不转 0。 */
  display: string;
  unit: string;
  seriesLabel: string;
  period: string;
  question: string;
  missing: boolean;
}

export interface SignalCalcRefView {
  ref: string;
  display: string;
  unit: string;
  expression: string;
  inputs: string[];
  precision: number;
  op: string;
  /** missing/rejected 的计算不可引用（后端已拒），兜底仍给出原因。 */
  notOkReason: string | null;
}

function observationLabel(obs: SignalAppendixObservation): string {
  if (typeof obs.label === "string" && obs.label) return obs.label;
  const parts = [obs.geo, obs.product, obs.flow].filter(
    (v): v is string => typeof v === "string" && v !== "",
  );
  if (parts.length > 0) return parts.join(" · ");
  return "观测";
}

/** 原值观测展示：raw_value（源精度）优先，缺失保留标记，绝不转 0。 */
export function formatObservationValue(obs: SignalAppendixObservation): string {
  if (typeof obs.raw_value === "string" && obs.raw_value.trim() !== "") {
    return obs.raw_value.trim();
  }
  if (obs.value === null || obs.value === undefined) return "缺失";
  return String(obs.value);
}

/** 查 [[data:cN:oM]]：在 appendix.calls 的观测全集里找观测行。 */
export function resolveDataRef(
  ref: string,
  calls: SignalAppendixCall[],
): SignalDataRefView | null {
  for (const call of calls) {
    for (const obs of call.observations ?? []) {
      if (obs.observation_id !== ref) continue;
      const missing = obs.value === null || obs.value === undefined;
      return {
        ref,
        display: formatObservationValue(obs),
        unit: typeof obs.unit === "string" ? obs.unit : "",
        seriesLabel: observationLabel(obs),
        period: typeof obs.period === "string" ? obs.period : "",
        question: call.question,
        missing,
      };
    }
  }
  return null;
}

/** 查 [[calc:kN]]：appendix.calculations 里找计算行（status=ok 才有展示值）。 */
export function resolveCalcRef(
  ref: string,
  calculations: SignalCalculation[],
): SignalCalcRefView | null {
  for (const calc of calculations) {
    if (calc.calc_id !== ref) continue;
    const ok = calc.status === "ok" && typeof calc.value === "string" && calc.value !== "";
    return {
      ref,
      display: ok ? (calc.value ?? "") : "缺失",
      unit: calc.unit ?? "",
      expression: calc.expression,
      inputs: calc.inputs ?? [],
      precision: calc.precision,
      op: calc.op,
      notOkReason: ok ? null : (calc.reason ?? `计算状态 ${calc.status}`),
    };
  }
  return null;
}

/** 附录锚点 DOM id（ref 里的冒号换成连字符，保证选择器安全）。 */
export function appendixAnchorId(ref: string): string {
  return `signal-app-${ref.replace(/:/g, "-").replace(/[^a-zA-Z0-9_-]/g, "")}`;
}

// ── 图表数据变换（null 断点不补零不连线；来源标记 原值/代码计算）──────────

export interface SignalChartPoint {
  ref: string;
  period: string;
  value: number | null;
  /** 源精度展示串（raw_value 优先）：图表数值标签用它，不经 Number 往返。 */
  display: string;
  unit: string;
  /** data=原值观测 / calc=代码计算。 */
  origin: "data" | "calc";
  seriesLabel: string;
}

export interface SignalChartData {
  chartId: string;
  kind: "line" | "comparison";
  claim: string;
  points: SignalChartPoint[];
  unit: string;
  /** 单位/系列是否一致（不一致后端已拒绝；前端兜底展示原样不画轴）。 */
  mixed: boolean;
}

/** 把图表 refs 解析成数据点（观测值或计算值；解析不到的 ref 跳过）。 */
export function buildSignalChartData(
  chart: SignalReportChart,
  calls: SignalAppendixCall[],
  calculations: SignalCalculation[],
): SignalChartData {
  const points: SignalChartPoint[] = [];
  for (const ref of chart.refs ?? []) {
    if (ref.startsWith("k")) {
      const calc = resolveCalcRef(ref, calculations);
      if (calc && calc.display !== "缺失") {
        const value = Number(calc.display);
        points.push({
          ref,
          period: calcPointPeriod(calc.inputs, calls, calculations),
          value: Number.isFinite(value) ? value : null,
          display: calc.display,
          unit: calc.unit,
          origin: "calc",
          seriesLabel: calc.expression,
        });
      }
      continue;
    }
    const obs = resolveDataRef(ref, calls);
    if (obs) {
      const value = obs.missing ? null : Number(obs.display);
      points.push({
        ref,
        period: obs.period,
        value: Number.isFinite(value as number) ? (value as number) : null,
        display: obs.display,
        unit: obs.unit,
        origin: "data",
        seriesLabel: obs.seriesLabel,
      });
    }
  }
  // 折线按期间升序（后端已排序，防御性重排；null 断点保留不补零）
  points.sort((a, b) => a.period.localeCompare(b.period));
  const units = new Set(points.map((p) => `${p.unit}`));
  return {
    chartId: chart.chart_id,
    kind: chart.kind === "comparison" ? "comparison" : "line",
    claim: chart.claim,
    points,
    unit: points[0]?.unit ?? "",
    mixed: units.size > 1,
  };
}

/** 计算点的展示期间：取输入观测的最大期间（与后端排序口径一致）。 */
function calcPointPeriod(
  inputs: string[],
  calls: SignalAppendixCall[],
  calculations: SignalCalculation[],
): string {
  let max = "";
  for (const input of inputs ?? []) {
    const obs = resolveDataRef(input, calls);
    if (obs && obs.period > max) max = obs.period;
    else {
      const nested = resolveCalcRef(input, calculations);
      if (nested) {
        for (const nestedInput of nested.inputs) {
          const inner = resolveDataRef(nestedInput, calls);
          if (inner && inner.period > max) max = inner.period;
        }
      }
    }
  }
  return max;
}

/** 折线分段：null 断点切开，返回若干段坐标序列（不补零不跨段连线）。 */
export function splitLineSegments(points: SignalChartPoint[]): SignalChartPoint[][] {
  const segments: SignalChartPoint[][] = [];
  let current: SignalChartPoint[] = [];
  for (const p of points) {
    if (p.value === null) {
      if (current.length > 0) segments.push(current);
      current = [];
      continue;
    }
    current.push(p);
  }
  if (current.length > 0) segments.push(current);
  return segments;
}

// ── 展示派生（状态徽标 / 周期校验 / 事后回顾）─────────────────────────────

/** 候选派生状态徽标（服务端只读，前端不做本地推断）。 */
export function candidateStatusLabel(status: SignalCandidateStatus): string {
  switch (status) {
    case "researching":
      return "研究中";
    case "reported":
      return "已有报告";
    default:
      return "待研究";
  }
}

/** 周期格式校验（仅形状；未来周期由服务端按业务时区判，前端不复制该逻辑）。 */
export function validateSignalPeriodFormat(
  granularity: SignalGranularity,
  period: string,
): boolean {
  return granularity === "year"
    ? /^\d{4}$/.test(period)
    : /^\d{4}-(0[1-9]|1[0-2])$/.test(period);
}

/** 历史报告标记：analysis_mode=retrospective → 「事后回顾 · 本次数据版本」。 */
export function isRetrospectiveReport(payload: SignalReportPayload): boolean {
  return (
    payload?.generation_meta?.analysis_mode === "retrospective" ||
    payload?.signal_snapshot?.analysis_mode === "retrospective"
  );
}

/** direction 枚举的中文小标（仅 implication 结构化字段展示用）。 */
export function directionLabel(direction?: string): string {
  switch (direction) {
    case "up":
      return "向上";
    case "down":
      return "向下";
    case "diverge":
      return "分化";
    case "conditional":
      return "看条件";
    default:
      return direction ?? "—";
  }
}

// ── 报告 as-of 机械行（5.5：数据截至 · 研究时点；数据只取已入库 payload）────

/**
 * 混杂 period 形态（YYYY-MM-DD / YYYY-MM / YYYY / YYYYMM）→ 可比较序数
 * （(年×12+月)×32+日：月为主序、日破同月平局，EIA 周度期同月内也能取到
 * 真正最大）；不可解析返回 null。直接字符串比较会把 "2026-07" 排在
 * "202601" 之后，取全局最大前必须先按形态规范化。
 */
function signalPeriodRank(period: string): number | null {
  const p = period.trim();
  const asInt = (s: string) => (/^\d+$/.test(s) ? Number(s) : NaN);
  let year = NaN;
  let month = 1;
  let day = 0;
  if (/^\d{4}$/.test(p)) {
    year = asInt(p);
  } else if (/^\d{6}$/.test(p)) {
    year = asInt(p.slice(0, 4));
    month = asInt(p.slice(4));
  } else if (/^\d{4}-\d{2}$/.test(p)) {
    year = asInt(p.slice(0, 4));
    month = asInt(p.slice(5));
  } else if (/^\d{4}-\d{2}-\d{2}$/.test(p)) {
    year = asInt(p.slice(0, 4));
    month = asInt(p.slice(5, 7));
    day = asInt(p.slice(8));
  }
  if (!Number.isInteger(year) || year <= 0) return null;
  if (!Number.isInteger(month) || month < 1 || month > 12) return null;
  if (!Number.isInteger(day) || day < 0 || day > 31) return null;
  return (year * 12 + month) * 32 + day;
}

/**
 * appendix 全部观测 period 的全局最大值（按形态规范化比较；WDI 观测的期间
 * 键是 year——period 缺失时回退，源格式以 payload 实际为准）。无任何可解析
 * 期间返回 null（渲染层如实显示「无可用数据期」）；数据只取已入库 payload，
 * 不做任何推断。
 */
export function latestObservationPeriod(calls: SignalAppendixCall[]): string | null {
  let best: string | null = null;
  let bestRank = 0;
  for (const call of calls ?? []) {
    for (const obs of call.observations ?? []) {
      const raw =
        typeof obs.period === "string" && obs.period.trim() !== ""
          ? obs.period
          : typeof obs.year === "string" && obs.year.trim() !== ""
            ? obs.year
            : "";
      if (!raw) continue;
      const rank = signalPeriodRank(raw);
      if (rank === null) continue;
      if (best === null || rank > bestRank) {
        best = raw;
        bestRank = rank;
      }
    }
  }
  return best;
}

/** 四段固定顺序与标题（thesis/facts/causal/implication）。 */
export const SIGNAL_SECTION_ORDER = [
  { kind: "thesis", title: "先说结论" },
  { kind: "facts", title: "发生了什么" },
  { kind: "causal", title: "这意味着什么" },
  { kind: "implication", title: "接下来怎么看" },
] as const;

/** 按 kind 取段（后端保证恰四段唯一序；前端按序取，缺段不崩）。 */
export function findSection(
  payload: SignalReportPayload,
  kind: (typeof SIGNAL_SECTION_ORDER)[number]["kind"],
): SignalReportPayload["report"]["sections"][number] | null {
  return payload?.report?.sections?.find((s) => s.kind === kind) ?? null;
}

// ── 正文段落渲染模型（SignalReportView 消费；纯函数可测）─────────────────

export interface RefLinkView {
  token: SignalRefToken;
  label: string;
  title: string;
  /** 附录行锚点 id；news 引用不可点击（原地展示不跳转）。 */
  anchor: string | null;
  clickable: boolean;
}

export type TextSegmentView =
  | { kind: "text"; text: string }
  | { kind: "ref"; view: RefLinkView | null };

/**
 * 一段正文 → 渲染段：引用 token 从 appendix 查表生成展示值；悬空 data/calc
 * 引用 view=null（渲染层显示占位不崩）。
 */
export function buildSegmentViews(
  text: string,
  calls: SignalAppendixCall[],
  calculations: SignalCalculation[],
): TextSegmentView[] {
  return parseSignalReferenceTokens(text).map((seg) => {
    if (seg.kind === "text") return seg;
    const token = seg.token;
    if (token.type === "data") {
      const obs = resolveDataRef(token.ref, calls);
      if (!obs) return { kind: "ref", view: null };
      return {
        kind: "ref",
        view: {
          token,
          label: obs.unit ? `${obs.display} ${obs.unit}` : obs.display,
          title: `${obs.seriesLabel} · ${obs.period}${obs.missing ? " · 值缺失" : ""}${obs.question ? ` · 查询问题：${obs.question}` : ""}`,
          anchor: appendixAnchorId(token.ref),
          clickable: true,
        },
      };
    }
    if (token.type === "calc") {
      const calc = resolveCalcRef(token.ref, calculations);
      if (!calc) return { kind: "ref", view: null };
      return {
        kind: "ref",
        view: {
          token,
          label: calc.unit ? `${calc.display} ${calc.unit}` : calc.display,
          title: `${calc.op}：${calc.expression}（代码计算，最多 ${calc.precision} 位小数）${calc.notOkReason ? ` · ${calc.notOkReason}` : ""}`,
          anchor: appendixAnchorId(token.ref),
          clickable: true,
        },
      };
    }
    // news：候选新闻依据切片（内容不在报告 payload，原地展示不跳转）
    return {
      kind: "ref",
      view: {
        token,
        label: `新闻依据 #${token.ref}`,
        title: "候选新闻依据切片（编号可在候选列表「新闻依据」核对）",
        anchor: null,
        clickable: false,
      },
    };
  });
}

/** 段文本按空行/换行拆段，段内再解析引用 token。 */
export function buildParagraphViews(
  text: string,
  calls: SignalAppendixCall[],
  calculations: SignalCalculation[],
): TextSegmentView[][] {
  return text
    .split(/\n+/)
    .map((p) => p.trim())
    .filter((p) => p !== "")
    .map((p) => buildSegmentViews(p, calls, calculations));
}

// ── 附录表行（观测全集 / 计算全集）─────────────────────────────────────

export interface AppendixObsRow {
  anchor: string;
  ref: string;
  label: string;
  period: string;
  display: string;
  unit: string;
  missingReason: string;
  question: string;
}

export function buildAppendixObsRows(calls: SignalAppendixCall[]): AppendixObsRow[] {
  const rows: AppendixObsRow[] = [];
  for (const call of calls) {
    for (const obs of call.observations ?? []) {
      const resolved = resolveDataRef(obs.observation_id, calls);
      if (!resolved) continue;
      rows.push({
        anchor: appendixAnchorId(obs.observation_id),
        ref: obs.observation_id,
        label: resolved.seriesLabel,
        period: resolved.period || "—",
        display: resolved.display,
        unit: resolved.unit || "—",
        missingReason: typeof obs.missing_reason === "string" ? obs.missing_reason : "",
        question: call.question,
      });
    }
  }
  return rows;
}

export interface AppendixCalcRow {
  anchor: string;
  ref: string;
  op: string;
  expression: string;
  inputs: string[];
  display: string;
  unit: string;
  precision: number;
  status: string;
  reason: string;
}

export function buildAppendixCalcRows(calculations: SignalCalculation[]): AppendixCalcRow[] {
  return calculations.map((calc) => ({
    anchor: appendixAnchorId(calc.calc_id),
    ref: calc.calc_id,
    op: calc.op,
    expression: calc.expression,
    inputs: calc.inputs ?? [],
    display: calc.status === "ok" ? (calc.value ?? "—") : `缺失（${calc.reason || calc.status}）`,
    unit: calc.unit || "—",
    precision: calc.precision,
    status: calc.status,
    reason: calc.reason ?? "",
  }));
}
