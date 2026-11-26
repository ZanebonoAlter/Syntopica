// 报告呈现纯逻辑（redesign-enrichment-research-workbench 3.2 / design D5）。
// 阅读短格式与原值分层 / 机械分源时效 / 证据表行渲染 / 图表真实日期布局。
// 只消费已入库 payload——不改源单位、不写回观测、不猜数值；无法格式化时
// 保留安全原值与说明，绝不把非零显示成 0。
import type {
  SignalAppendixCall,
  SignalCalculation,
  SignalReportBlock,
  SignalReportEvidenceRow,
  SignalReportPayload,
} from "~/api/boardSignals";
import {
  appendixAnchorId,
  formatReadingValue,
  resolveCalcRef,
  resolveDataRef,
  type SignalChartPoint,
} from "./signalReport";

export { formatReadingValue } from "./signalReport";

// ── 机械分源时效（D5：schema3 直接读 data_freshness；schema2 从 appendix
// 按工具派生「各源最大期」并标注派生口径——不改旧 facts 文字）。────────────

export interface SourceFreshnessView {
  tool: string;
  latest: string;
}

export interface SourceFreshnessSummary {
  sources: SourceFreshnessView[];
  /** 研究时点（材料边界）。 */
  cutoff: string;
  /** 报告生成时间（schema3 才有；schema2 为 null 不冒充）。 */
  generatedAt: string | null;
  /** true = schema2 从 appendix 派生的「各源最大期」口径（非逐组声明）。 */
  derived: boolean;
  hasObservations: boolean;
}

/** 工具名升序，注入行稳定可测。 */
function toolNameOrder(a: SourceFreshnessView, b: SourceFreshnessView): number {
  return a.tool < b.tool ? -1 : a.tool > b.tool ? 1 : 0;
}

/** appendix 按工具聚合的最新观测期（schema2 兜底口径；全无可解析期 → 空）。 */
function derivePerToolLatest(calls: SignalAppendixCall[]): SourceFreshnessView[] {
  const rankOf = (period: string): number | null => {
    const p = period.trim();
    const asInt = (s: string) => (/^\d+$/.test(s) ? Number(s) : NaN);
    let year = NaN;
    let month = 1;
    let day = 0;
    if (/^\d{4}$/.test(p)) year = asInt(p);
    else if (/^\d{6}$/.test(p)) {
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
  };
  const bestRank = new Map<string, number>();
  const bestRaw = new Map<string, string>();
  for (const call of calls ?? []) {
    for (const obs of call.observations ?? []) {
      const raw =
        typeof obs.period === "string" && obs.period.trim() !== "" ? obs.period : "";
      if (!raw) continue;
      const rank = rankOf(raw);
      if (rank === null) continue;
      const prev = bestRank.get(call.tool);
      if (prev === undefined || rank > prev) {
        bestRank.set(call.tool, rank);
        bestRaw.set(call.tool, raw);
      }
    }
  }
  const out: SourceFreshnessView[] = [];
  for (const [tool, latest] of bestRaw) out.push({ tool, latest });
  return out.sort(toolNameOrder);
}

/**
 * 机械分源时效汇总。schema3 读 data_freshness（工具名升序稳定化）；schema2
 * 从 appendix 派生并标 derived（读取层注明「各源最大期」）。零观测如实返回
 * 空列表，不用 cutoff 或今天日期冒充数据期。
 */
export function buildSourceFreshness(payload: SignalReportPayload): SourceFreshnessSummary {
  const calls = payload?.appendix?.calls ?? [];
  const cutoff = (payload?.generation_meta?.cutoff ?? payload?.signal_snapshot?.cutoff ?? "").slice(0, 10);
  if (payload?.schema_version >= 3 && payload.data_freshness) {
    const sources = (payload.data_freshness.sources ?? [])
      .map((s) => ({ tool: s.tool, latest: s.latest ?? "" }))
      .sort(toolNameOrder);
    return {
      sources,
      cutoff,
      generatedAt: payload.data_freshness.generated_at?.slice(0, 10) ?? null,
      derived: false,
      hasObservations: payload.data_freshness.has_observations ?? sources.some((s) => s.latest !== ""),
    };
  }
  const sources = derivePerToolLatest(calls);
  return {
    sources,
    cutoff,
    generatedAt: null,
    derived: true,
    hasObservations: sources.length > 0,
  };
}

// ── 证据表行渲染（schema3 evidence_table：refs 解析为阅读值 + 原值锚点；
// 行内缺失/悬空如实标注，不伪造数值）。──────────────────────────────────

export interface EvidenceCellView {
  ref: string;
  /** 阅读短格式（formatReadingValue）；缺失显示「缺失」。 */
  display: string;
  unit: string;
  title: string;
  anchor: string;
  clickable: boolean;
}

export interface EvidenceRowView {
  label: string;
  cells: EvidenceCellView[];
  /** 行内存在无法解析的引用时如实提示（不伪造数值）。 */
  hasMissing: boolean;
}

/** refs → 单元格视图：data/calc 查 appendix；news/未知进表如实标缺失。 */
function evidenceCell(ref: string, calls: SignalAppendixCall[], calculations: SignalCalculation[]): EvidenceCellView {
  if (ref.startsWith("k")) {
    const calc = resolveCalcRef(ref, calculations);
    if (!calc || calc.notOkReason) {
      return {
        ref,
        display: "缺失",
        unit: "",
        title: calc?.notOkReason ?? "计算引用无法解析",
        anchor: appendixAnchorId(ref),
        clickable: false,
      };
    }
    return {
      ref,
      display: formatReadingValue(calc.display),
      unit: calc.unit,
      title: `${calc.op}：${calc.expression}（代码计算，最多 ${calc.precision} 位小数）`,
      anchor: appendixAnchorId(ref),
      clickable: true,
    };
  }
  const obs = resolveDataRef(ref, calls);
  if (!obs) {
    return {
      ref,
      display: "缺失",
      unit: "",
      title: "观测引用无法解析（数据缺失）",
      anchor: appendixAnchorId(ref),
      clickable: false,
    };
  }
  return {
    ref,
    display: obs.missing ? "缺失" : formatReadingValue(obs.display),
    unit: obs.unit,
    title: `${obs.seriesLabel} · ${obs.period}${obs.missing ? " · 值缺失" : ""}`,
    anchor: appendixAnchorId(ref),
    clickable: true,
  };
}

/** schema3 证据表块 → 行渲染视图；非 evidence_table 块返回 null。 */
export function buildEvidenceTableRows(
  block: SignalReportBlock,
  calls: SignalAppendixCall[],
  calculations: SignalCalculation[],
): EvidenceRowView[] | null {
  if (block.type !== "evidence_table") return null;
  return (block.rows ?? []).map((row: SignalReportEvidenceRow) => {
    const cells = (row.refs ?? []).map((ref) => evidenceCell(ref, calls, calculations));
    return {
      label: row.label ?? "",
      cells,
      hasMissing: cells.some((c) => c.display === "缺失"),
    };
  });
}

// ── 图表真实日期布局（D5：日期按实际时间距离；null 断线不补零；连续性
// 无法证明降离散点并标观测数；comparison 柱从 0）。────────────────────────

/** period（YYYY-MM-DD / YYYY-MM / YYYY）→ ms 时间戳；不可解析 → null。 */
export function periodTimestamp(period: string): number | null {
  const p = period.trim();
  let m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(p);
  if (m) return Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  m = /^(\d{4})-(\d{2})$/.exec(p);
  if (m) return Date.UTC(Number(m[1]), Number(m[2]) - 1, 15);
  m = /^(\d{4})$/.exec(p);
  if (m) return Date.UTC(Number(m[1]), 6, 1);
  return null;
}

export interface ChartLayout {
  /** line = 时间连续可连线；discrete = 连续性不明，只画点/柱并标观测数。 */
  mode: "line" | "discrete";
  /** 每个点的归一化 x（0..1，按真实时间距离）；不可解析期间按序插入。 */
  x: number[];
  /** y 归一化（0..1，1=域顶）：line 按域内 min/max 加 12% 边距；柱从 0。 */
  y: (number | null)[];
  yMin: number;
  yMax: number;
  /** 非空观测数（离散态展示用）。 */
  observationCount: number;
}

/**
 * 数据点 → 布局。x 按真实时间距离（三天间隔 1 天与 6 天距离 6 天），不可解析
 * 期间的点按序均插（如实退化为序列位置）。相邻非空点间隔极不均匀（最大者
 * 超过最小者 10 倍）→ discrete（连续性不明不连线）。y：comparison 从 0；
 * line 取非空值域加边距。
 */
export function buildChartLayout(points: SignalChartPoint[], kind: "line" | "comparison"): ChartLayout {
  const n = points.length;
  let forcedDiscrete = false;
  const times = points.map((p) => periodTimestamp(p.period));
  // x：可解析的按真实时间；不可解析的 null 占位后按序插入。
  const known = times.filter((t): t is number => t !== null);
  let x: number[];
  if (known.length === n && n >= 2) {
    const min = Math.min(...known);
    const max = Math.max(...known);
    const span = max - min;
    x = times.map((t) => (span > 0 ? ((t as number) - min) / span : 0.5));
  } else {
    x = points.map((_, i) => (n <= 1 ? 0.5 : i / (n - 1)));
    // R-4：期间不可解析 → 连续性无法证明，强制离散（禁止按数组位置假装等时距连线）。
    if (kind !== "comparison") forcedDiscrete = true;
  }
  const values = points.map((p) => p.value);
  const nonNull = values.filter((v): v is number => v !== null);
  let yMin = 0;
  let yMax = 1;
  if (nonNull.length > 0) {
    if (kind === "comparison") {
      yMin = 0;
      yMax = Math.max(...nonNull);
      if (yMax === 0) yMax = 1;
    } else {
      let min = Math.min(...nonNull);
      let max = Math.max(...nonNull);
      if (min === max) {
        min -= 1;
        max += 1;
      } else {
        const pad = (max - min) * 0.12;
        min -= pad;
        max += pad;
      }
      yMin = min;
      yMax = max;
    }
  }
  const yOf = (v: number | null) => (v === null ? null : (v - yMin) / (yMax - yMin));
  // 连续性：相邻非空点的时间间隔最大者超过最小者 10 倍 → 视为序列断档
  // （连续性不明，降离散点；1:6 这类真实不等距仍保持连线，由 x 轴真实距离
  // 表达）。null 断点由渲染层切段负责，这里默认 line。
  let mode: "line" | "discrete" = "line";
  if (kind !== "comparison" && known.length === n && n >= 2) {
    const gaps: number[] = [];
    for (let i = 1; i < n; i++) {
      if (values[i] === null || values[i - 1] === null) continue;
      gaps.push((times[i] as number) - (times[i - 1] as number));
    }
    if (gaps.length > 0) {
      const minGap = Math.min(...gaps);
      const maxGap = Math.max(...gaps);
      if (minGap > 0 && maxGap > minGap * 10) mode = "discrete";
    }
  }
  if (forcedDiscrete) mode = "discrete";
  return {
    mode,
    x,
    y: values.map(yOf),
    yMin,
    yMax,
    observationCount: nonNull.length,
  };
}
