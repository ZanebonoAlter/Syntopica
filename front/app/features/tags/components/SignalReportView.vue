<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import type { SignalReportDetail } from '~/api/boardSignals'
import {
  buildAppendixCalcRows,
  buildAppendixObsRows,
  buildParagraphViews,
  buildSignalChartData,
  directionLabel,
  findSection,
  isRetrospectiveReport,
  resolveDataRef,
  splitLineSegments,
  type AppendixCalcRow,
  type AppendixObsRow,
  type SignalCalculation,
  type SignalChartData,
  type SignalChartPoint,
  type SignalReportPayload,
  type TextSegmentView,
} from './signalReport'
import {
  buildChartLayout,
  formatReadingValue,
  buildEvidenceTableRows,
  buildSourceFreshness,
  type ChartLayout,
  type EvidenceCellView,
  type EvidenceRowView,
  type SourceFreshnessSummary,
} from './reportPresentation'
import {
  answerStatusLabel,
  isSafeHttpUrl,
  sourceAnchorId,
  sourceTemporalLine,
  type SignalAppendixSource,
  type SignalReportAnswer,
} from './signalReport'

/**
 * 信号解读报告阅读视图（board-signal-reports 5.3，FE-6~9）。
 *
 * AppPageShell reader≤760 阅读列；四段按序（thesis/facts/causal/implication，
 * implication 展示结构化字段 verdict/direction/horizon/trigger_condition/
 * self_doubt）。正文引用 token（[[data:cN:oM]]/[[calc:kN]]/[[news:]]）由前端
 * 从 appendix 查表渲染数值与原生单位——模型永不供数；点击展开附录并定位。
 * 图表 refs → 轻量 SVG（null 断点不补零不连线，来源标 原值/代码计算），0 图合法。
 *
 * 安全渲染：LLM 文本一律走插值（{{ }}）与 DOM textContent，不经 v-html。
 * 可测纯逻辑（引用解析/图表变换/附录行/as-of 最大期）全部在 ./signalReport。
 */
const props = defineProps<{
  report: SignalReportDetail | null
  loading: boolean
  error: string | null
  /** 重新研究任务在跑（从报告页发起或列表恢复）：显示内联进度。 */
  regenerating: boolean
  /** 重新研究任务的实时阶段（research/compose）。 */
  regeneratePhase?: string | null
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'regenerate', candidateId: number): void
}>()

const payload = computed<SignalReportPayload | null>(() => props.report?.sectors ?? null)
const snapshot = computed(() => payload.value?.signal_snapshot ?? null)
const meta = computed(() => payload.value?.generation_meta ?? null)

const EMPTY_PAYLOAD = {
  report: { title: '', sections: [], charts: [] },
  appendix: { calls: [], calculations: [], gaps: [] },
  signal_snapshot: null,
  generation_meta: null,
} as unknown as SignalReportPayload

const sections = computed(() => ({
  thesis: findSection(payload.value ?? EMPTY_PAYLOAD, 'thesis'),
  facts: findSection(payload.value ?? EMPTY_PAYLOAD, 'facts'),
  causal: findSection(payload.value ?? EMPTY_PAYLOAD, 'causal'),
  implication: findSection(payload.value ?? EMPTY_PAYLOAD, 'implication'),
}))

const retrospective = computed(() => (payload.value ? isRetrospectiveReport(payload.value) : false))
const budgetExhausted = computed(() => meta.value?.stop_reason === 'budget_exhausted')
const cutoffLabel = computed(() => (snapshot.value?.cutoff ? snapshot.value.cutoff.slice(0, 10) : '—'))

// ── 引用渲染：正文 token → appendix 查表（悬空引用显示占位不崩）────────────

const calls = computed(() => payload.value?.appendix?.calls ?? [])
const calculations = computed<SignalCalculation[]>(() => payload.value?.appendix?.calculations ?? [])
// question-v1：已读文字证据（appendix.sources；旧载荷缺省=空）。
const sources = computed<SignalAppendixSource[]>(() => payload.value?.appendix?.sources ?? [])
const answer = computed<SignalReportAnswer | null>(() => payload.value?.answer ?? null)
const answerLabel = computed(() => (answer.value ? answerStatusLabel(answer.value.status) : ''))

/** 段文本 → 段落渲染模型（引用查表；纯函数委托 ./signalReport）。 */
function paragraphs(text: string): TextSegmentView[][] {
  return buildParagraphViews(text, calls.value, calculations.value, sources.value)
}

// ── schema3 受控块渲染（3.2）：facts/causal 支持 paragraph/evidence_table/
// chart 三类块；thesis/implication 恒为单段落，继续走兼容 text 模板分支。────

interface SectionBlockView {
  paragraph?: TextSegmentView[][]
  table?: { title: string; rows: EvidenceRowView[] }
  chartId?: string
}

function sectionBlockViews(kind: 'facts' | 'causal'): SectionBlockView[] | null {
  const section = sections.value[kind]
  const blocks = section?.blocks
  if (!blocks || blocks.length === 0) return null
  const out: SectionBlockView[] = []
  for (const b of blocks) {
    if (b.type === 'paragraph') {
      out.push({ paragraph: paragraphs(b.text ?? '') })
    } else if (b.type === 'evidence_table') {
      out.push({
        table: {
          title: b.title ?? '',
          rows: buildEvidenceTableRows(b, calls.value, calculations.value) ?? [],
        },
      })
    } else if (b.type === 'chart' && b.chart_id) {
      out.push({ chartId: b.chart_id })
    }
    // 未知块类型：后端已拒绝；防御性跳过（兼容 text 分支仍可读）。
  }
  return out
}

const factsBlockViews = computed(() => sectionBlockViews('facts'))
const causalBlockViews = computed(() => sectionBlockViews('causal'))

function onCellClick(cell: EvidenceCellView) {
  void focusRef({ anchor: cell.anchor, clickable: cell.clickable, token: { ref: cell.ref } })
}

// ── as-of 机械行（3.2 D5）：分源时效（schema3 直读 data_freshness；schema2 从
// appendix 派生「各源最大期」并标注）；研究时点 = cutoff，不混生成时间 ──

const freshness = computed<SourceFreshnessSummary | null>(() =>
  payload.value ? buildSourceFreshness(payload.value) : null,
)
const freshnessLabel = computed(() => {
  const f = freshness.value
  if (!f || f.sources.length === 0) return '无可用数据期'
  return f.sources.map((s) => `${s.tool} ${s.latest || '无观测'}`).join(' · ')
})
const researchCutoff = computed(() => (meta.value?.cutoff ?? snapshot.value?.cutoff ?? '').slice(0, 10) || '—')

// ── 附录定位（点击引用 → 展开附录 + 高亮行）─────────────────────────────

const appendixOpen = ref(false)
const highlightRef = ref<string | null>(null)

async function focusRef(view: { anchor: string | null; clickable: boolean; token: { ref: string } } | null) {
  if (!view?.clickable || !view.anchor) return
  highlightRef.value = view.token.ref
  appendixOpen.value = true
  await nextTick()
  document.getElementById(view.anchor)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

function onAppendixToggle(event: Event) {
  appendixOpen.value = (event.target as HTMLDetailsElement).open
  if (!appendixOpen.value) highlightRef.value = null
}

// ── 附录表行（观测全集 + 计算全集 + 缺口）─────────────────────────────

const appendixObsRows = computed<AppendixObsRow[]>(() => buildAppendixObsRows(calls.value))
const appendixCalcRows = computed<AppendixCalcRow[]>(() => buildAppendixCalcRows(calculations.value))
const appendixGaps = computed(() => payload.value?.appendix?.gaps ?? [])

// ── 图表：refs → SVG 几何（null 断点不补零不连线；0 图合法）─────────────

const chartDatas = computed<SignalChartData[]>(() =>
  (payload.value?.report?.charts ?? []).map((ch) =>
    buildSignalChartData(ch, calls.value, calculations.value),
  ),
)

interface ChartSvgGeometry {
  width: number
  height: number
  bottom: number
  kind: 'line' | 'comparison'
  /** discrete = 连续性不明：只画点/柱不连线（figcaption 标观测数）。 */
  mode: 'line' | 'discrete'
  /** 折线分段 path（null 断点切分；段内 ≥2 点才连线，非空点画圆点）。 */
  paths: string[]
  dots: { x: number; y: number; display: string }[]
  valueLabels: { x: number; y: number; text: string }[]
  xLabels: { x: number; text: string }[]
  yLabels: { x: number; y: number; text: string }[]
  bars: { x: number; y: number; width: number; height: number; display: string; xLabel: string }[]
}

const W = 320
const H = 175
const PAD = { left: 46, right: 20, top: 26, bottom: 38 }

function shortPeriod(period: string): string {
  const m = /\d{4}-(\d{2})-(\d{2})/.exec(period)
  if (m) return `${m[1]}-${m[2]}`
  return period
}

function fmtAxis(v: number): string {
  // R-3：轴刻度走阅读短格式——小量级（0.001~0.002）不得显示成 0（D5）。
  return formatReadingValue(String(v))
}

/**
 * 数据点 → SVG 几何。x 用真实日期距离（buildChartLayout）；y 域：折线看变化
 * （域内 min/max 加边距），对比柱从 0 起。离散模式不产 path（连续性不明不连线）。
 */
function chartGeometry(data: SignalChartData): ChartSvgGeometry | null {
  const points: SignalChartPoint[] = data.points
  const valid = points.filter((p): p is SignalChartPoint & { value: number } => p.value !== null)
  if (valid.length === 0) return null
  const layout: ChartLayout = buildChartLayout(points, data.kind)
  const plotW = W - PAD.left - PAD.right
  const plotH = H - PAD.top - PAD.bottom
  const xOf = (i: number) => PAD.left + layout.x[i]! * plotW
  const yOfNorm = (norm: number) => PAD.top + (1 - norm) * plotH
  const yLabels = [
    { x: 4, y: PAD.top + 4, text: fmtAxis(layout.yMax) },
    { x: 4, y: H - PAD.bottom + 4, text: fmtAxis(layout.yMin) },
  ]
  const isComparison = data.kind === 'comparison'

  if (isComparison) {
    const slot = plotW / Math.max(points.length, 1)
    const barW = Math.min(54, slot * 0.55)
    return {
      width: W,
      height: H,
      bottom: H - PAD.bottom,
      kind: 'comparison',
      mode: 'line',
      paths: [],
      dots: [],
      valueLabels: [],
      xLabels: [],
      yLabels,
      bars: points.map((p, i) => {
        const norm = layout.y[i] ?? 0
        const y = yOfNorm(norm)
        return {
          x: PAD.left + slot * i + (slot - barW) / 2,
          y,
          width: barW,
          height: Math.max(H - PAD.bottom - y, 0),
          display: p.display,
          xLabel: shortPeriod(p.period),
        }
      }),
    }
  }

  // 折线：null 断点分段，段内连续点连线；离散模式只画点不产 path。
  const segments = splitLineSegments(points)
  const indexOf = new Map(points.map((p, i) => [p, i]))
  const paths =
    layout.mode === 'line'
      ? segments
          .filter((seg) => seg.length >= 2)
          .map((seg) =>
            seg
              .map((p, i) => {
                const cmd = i === 0 ? 'M' : 'L'
                const idx = indexOf.get(p) ?? 0
                return `${cmd}${xOf(idx).toFixed(1)} ${yOfNorm(layout.y[idx] ?? 0).toFixed(1)}`
              })
              .join(' '),
          )
      : []
  const dots = valid.map((p) => {
    const idx = indexOf.get(p) ?? 0
    return { x: xOf(idx), y: yOfNorm(layout.y[idx] ?? 0), display: p.display }
  })
  return {
    width: W,
    height: H,
    bottom: H - PAD.bottom,
    kind: 'line',
    mode: layout.mode,
    paths,
    dots,
    valueLabels: dots.map((d) => ({ x: d.x, y: d.y - 8, text: d.display })),
    xLabels: points.map((p, i) => ({ x: xOf(i), text: shortPeriod(p.period) })),
    yLabels,
    bars: [],
  }
}

const chartGeometries = computed<(ChartSvgGeometry & { data: SignalChartData })[]>(() =>
  chartDatas.value
    .map((data) => {
      const geo = chartGeometry(data)
      return geo ? { ...geo, data } : null
    })
    .filter((g): g is ChartSvgGeometry & { data: SignalChartData } => g !== null),
)

function onRegenerate() {
  const candidateId = snapshot.value?.candidate_id
  if (candidateId) emit('regenerate', candidateId)
}

function onRefClick(view: Parameters<typeof focusRef>[0]) {
  void focusRef(view)
}
</script>

<template>
  <AppPageShell mode="reader" as="div" class="signal-report" data-testid="signal-report-view">
    <!-- 顶栏：返回列表（周期/任务状态保留在 composable，返回不丢） -->
    <div class="sr-topbar">
      <AppButton variant="ghost" size="sm" data-testid="signal-report-back" @click="emit('back')">
        ← 返回列表
      </AppButton>
      <span class="sr-topmeta muted">报告生成即可阅读，无后续步骤</span>
    </div>

    <div v-if="loading" class="sr-state">
      <Icon icon="mdi:loading" width="16" class="spin" />
      报告加载中…
    </div>
    <div v-else-if="error" class="sr-state sr-state--error" role="alert" data-testid="signal-report-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <span class="muted">报告没有加载成功，并非报告不存在。</span>
    </div>
    <div v-else-if="!payload || !snapshot" class="sr-state sr-state--error" role="alert">
      <Icon icon="mdi:file-alert-outline" width="16" />
      <span>报告数据格式异常（缺少正文载荷）。</span>
    </div>

    <template v-else>
      <p class="sr-kicker">信号解读报告</p>
      <h1 class="sr-title">{{ payload.report.title }}</h1>
      <p class="sr-asof" data-testid="signal-asof">
        数据截至 {{ freshnessLabel }} · 研究时点 {{ researchCutoff }}<template v-if="freshness?.derived && freshness.sources.length > 0">（各源最大期）</template><template v-if="freshness?.generatedAt"> · 报告生成 {{ freshness.generatedAt }}</template>
      </p>
      <div class="sr-meta">
        <span>目标周期 {{ snapshot.period }}</span>
        <span>数据截止 {{ cutoffLabel }}</span>
        <span v-if="meta">取数 {{ meta.source_calls }} 次 · 计算 {{ meta.calculation_calls }} 次 · 决策 {{ meta.decisions }}/40 轮</span>
        <span v-if="retrospective" class="sr-retro-tag" data-testid="signal-retrospective">事后回顾 · 本次数据版本</span>
      </div>
      <div v-if="budgetExhausted" class="sr-budget" role="note" data-testid="signal-budget-notice">
        <Icon icon="mdi:flash-alert-outline" width="14" />
        研究预算已用尽（最多 40 轮决策）。以下判断基于已取得的证据；影响结论的缺口在正文与附录中如实列出，不代表已验证所有问题。
      </div>
      <div v-if="regenerating" class="sr-regen" role="status" aria-live="polite" data-testid="signal-regen-progress">
        <Icon icon="mdi:loading" width="14" class="spin" />
        重新研究进行中（{{ regeneratePhase === 'compose' ? '成文' : '研究' }}阶段）· 决策上限 40 轮 · 可返回列表查看候选
      </div>
      <hr class="sr-sep" />

      <!-- question-v1 答题范围（非可信度、非任务状态；旧报告缺省不显示） -->
      <div v-if="answer" class="sr-answer" :data-answer="answer.status" data-testid="signal-answer">
        <span class="sr-answer-badge">{{ answerLabel }}</span>
        <span class="sr-answer-reason">{{ answer.reason }}</span>
        <ul v-if="answer.unanswered?.length" class="sr-answer-unanswered" data-testid="signal-answer-unanswered">
          <li v-for="(u, ui) in answer.unanswered" :key="`u${ui}`">{{ u }}</li>
        </ul>
      </div>

      <!-- 先说结论（thesis，lede 版式） -->
      <p v-for="(para, pi) in paragraphs(sections.thesis?.text ?? '')" :key="`t${pi}`" class="sr-lede">
        <template v-for="(seg, si) in para" :key="`t${pi}-${si}`">
          <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
          <button
            v-else-if="seg.view?.clickable"
            type="button"
            class="sr-ref"
            :title="seg.view.title"
            @click="onRefClick(seg.view)"
          >{{ seg.view.label }}</button>
          <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
          <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
        </template>
      </p>

      <!-- 发生了什么（facts）：schema3 块分支；无块回退兼容 text 段落 -->
      <section class="sr-section">
        <h2 class="sr-h2">发生了什么</h2>
        <template v-if="factsBlockViews">
          <template v-for="(bv, bi) in factsBlockViews" :key="`fb${bi}`">
            <p v-if="bv.paragraph" v-for="(para, pi) in bv.paragraph" :key="`fbp${bi}-${pi}`" class="sr-para">
              <template v-for="(seg, si) in para" :key="`fbp${bi}-${pi}-${si}`">
                <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
                <button
                  v-else-if="seg.view?.clickable"
                  type="button"
                  class="sr-ref"
                  :title="seg.view.title"
                  @click="onRefClick(seg.view)"
                >{{ seg.view.label }}</button>
                <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
                <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
              </template>
            </p>
            <div v-else-if="bv.table" class="sr-etable" data-testid="signal-evidence-table">
              <p class="sr-etable-title">{{ bv.table.title }}</p>
              <table class="sr-etable-table">
                <tbody>
                  <tr v-for="(row, ri) in bv.table.rows" :key="`fr${bi}-${ri}`">
                    <th scope="row">{{ row.label }}</th>
                    <td>
                      <template v-for="(cell, ci) in row.cells" :key="`fc${bi}-${ri}-${ci}`">
                        <button
                          v-if="cell.clickable"
                          type="button"
                          class="sr-ref"
                          :title="cell.title"
                          @click="onCellClick(cell)"
                        >{{ cell.display }}<template v-if="cell.unit"> {{ cell.unit }}</template></button>
                        <span v-else class="sr-ref sr-ref--missing" :title="cell.title">{{ cell.display }}</span>
                      </template>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else-if="bv.chartId" class="sr-chart-anchor">📊 见图 {{ bv.chartId }}（下方图表）</p>
          </template>
        </template>
        <template v-else>
          <p v-for="(para, pi) in paragraphs(sections.facts?.text ?? '')" :key="`f${pi}`" class="sr-para">
            <template v-for="(seg, si) in para" :key="`f${pi}-${si}`">
              <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
              <button
                v-else-if="seg.view?.clickable"
                type="button"
                class="sr-ref"
                :title="seg.view.title"
                @click="onRefClick(seg.view)"
              >{{ seg.view.label }}</button>
              <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
              <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
            </template>
          </p>
        </template>

        <!-- 图表（轻量 SVG；0 图合法不渲染区块） -->
        <div v-if="chartGeometries.length" class="sr-charts" data-testid="signal-charts">
          <figure v-for="(chart, ci) in chartGeometries" :key="chart.data.chartId" class="sr-chart">
            <svg
              :viewBox="`0 0 ${chart.width} ${chart.height}`"
              role="img"
              :aria-label="chart.data.claim"
            >
              <line class="sr-chart-axis" x1="46" y1="26" x2="46" :y2="chart.bottom" />
              <line class="sr-chart-axis" x1="46" :y1="chart.bottom" :x2="chart.width - 20" :y2="chart.bottom" />
              <text v-for="(l, li) in chart.yLabels" :key="`yl${ci}-${li}`" class="sr-chart-text" :x="l.x" :y="l.y">{{ l.text }}</text>
              <template v-if="chart.kind === 'line'">
                <path v-for="(d, pi2) in chart.paths" :key="`p${ci}-${pi2}`" class="sr-chart-line" :d="d" />
                <circle v-for="(d, di) in chart.dots" :key="`d${ci}-${di}`" class="sr-chart-dot" :cx="d.x" :cy="d.y" r="3" />
                <text v-for="(l, vi) in chart.valueLabels" :key="`vl${ci}-${vi}`" class="sr-chart-text" :x="l.x" :y="l.y" text-anchor="middle">{{ l.text }}</text>
                <text v-for="(l, xi) in chart.xLabels" :key="`xl${ci}-${xi}`" class="sr-chart-text" :x="l.x" :y="chart.bottom + 16" text-anchor="middle">{{ l.text }}</text>
              </template>
              <template v-else>
                <rect
                  v-for="(b, bi) in chart.bars"
                  :key="`b${ci}-${bi}`"
                  class="sr-chart-bar"
                  :x="b.x"
                  :y="b.y"
                  :width="b.width"
                  :height="b.height"
                />
                <text v-for="(b, bi2) in chart.bars" :key="`bv${ci}-${bi2}`" class="sr-chart-text" :x="b.x + b.width / 2" :y="b.y - 6" text-anchor="middle">{{ b.display }}</text>
                <text v-for="(b, bi3) in chart.bars" :key="`bx${ci}-${bi3}`" class="sr-chart-text" :x="b.x + b.width / 2" :y="chart.bottom + 16" text-anchor="middle">{{ b.xLabel }}</text>
              </template>
            </svg>
            <figcaption>
              图{{ ci + 1 }} · {{ chart.data.claim }}<template v-if="chart.data.unit">；单位：{{ chart.data.unit }}</template><template v-if="chart.data.points.some((p) => p.origin === 'calc')">；数值由代码计算</template><template v-if="chart.data.points.some((p) => p.value === null)">；缺失期间断开显示</template><template v-if="chart.mode === 'discrete'">；连续性不明，按 {{ chart.dots.length }} 个观测点展示不连线</template>。
            </figcaption>
          </figure>
        </div>
      </section>

      <!-- 这意味着什么（causal）：schema3 块分支；无块回退兼容 text 段落 -->
      <section class="sr-section">
        <h2 class="sr-h2">这意味着什么</h2>
        <template v-if="causalBlockViews">
          <template v-for="(bv, bi) in causalBlockViews" :key="`cb${bi}`">
            <p v-if="bv.paragraph" v-for="(para, pi) in bv.paragraph" :key="`cbp${bi}-${pi}`" class="sr-para">
              <template v-for="(seg, si) in para" :key="`cbp${bi}-${pi}-${si}`">
                <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
                <button
                  v-else-if="seg.view?.clickable"
                  type="button"
                  class="sr-ref"
                  :title="seg.view.title"
                  @click="onRefClick(seg.view)"
                >{{ seg.view.label }}</button>
                <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
                <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
              </template>
            </p>
            <div v-else-if="bv.table" class="sr-etable" data-testid="signal-evidence-table">
              <p class="sr-etable-title">{{ bv.table.title }}</p>
              <table class="sr-etable-table">
                <tbody>
                  <tr v-for="(row, ri) in bv.table.rows" :key="`cr${bi}-${ri}`">
                    <th scope="row">{{ row.label }}</th>
                    <td>
                      <template v-for="(cell, ci) in row.cells" :key="`cc${bi}-${ri}-${ci}`">
                        <button
                          v-if="cell.clickable"
                          type="button"
                          class="sr-ref"
                          :title="cell.title"
                          @click="onCellClick(cell)"
                        >{{ cell.display }}<template v-if="cell.unit"> {{ cell.unit }}</template></button>
                        <span v-else class="sr-ref sr-ref--missing" :title="cell.title">{{ cell.display }}</span>
                      </template>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else-if="bv.chartId" class="sr-chart-anchor">📊 见图 {{ bv.chartId }}（上方图表）</p>
          </template>
        </template>
        <template v-else>
          <p v-for="(para, pi) in paragraphs(sections.causal?.text ?? '')" :key="`c${pi}`" class="sr-para">
            <template v-for="(seg, si) in para" :key="`c${pi}-${si}`">
              <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
              <button
                v-else-if="seg.view?.clickable"
                type="button"
                class="sr-ref"
                :title="seg.view.title"
                @click="onRefClick(seg.view)"
              >{{ seg.view.label }}</button>
              <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
              <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
            </template>
          </p>
        </template>
      </section>

      <!-- 接下来怎么看（implication + 结构化字段） -->
      <section class="sr-judgment" data-testid="signal-implication">
        <h2 class="sr-h2">接下来怎么看</h2>
        <p v-if="sections.implication?.verdict" class="sr-verdict">{{ sections.implication.verdict }}</p>
        <p v-for="(para, pi) in paragraphs(sections.implication?.text ?? '')" :key="`i${pi}`" class="sr-para">
          <template v-for="(seg, si) in para" :key="`i${pi}-${si}`">
            <span v-if="seg.kind === 'text'">{{ seg.text }}</span>
            <button
              v-else-if="seg.view?.clickable"
              type="button"
              class="sr-ref"
              :title="seg.view.title"
              @click="onRefClick(seg.view)"
            >{{ seg.view.label }}</button>
            <span v-else-if="seg.view" class="sr-ref sr-ref--news" :title="seg.view.title">{{ seg.view.label }}</span>
            <span v-else class="sr-ref sr-ref--missing" title="引用无法解析（数据缺失）">数据引用缺失</span>
          </template>
        </p>
        <dl v-if="sections.implication" class="sr-fields">
          <div class="sr-field">
            <dt>判断方向</dt>
            <dd data-testid="signal-direction">{{ directionLabel(sections.implication.direction) }}</dd>
          </div>
          <div class="sr-field">
            <dt>时间范围</dt>
            <dd>{{ sections.implication.horizon }}</dd>
          </div>
          <div class="sr-field">
            <dt>触发条件</dt>
            <dd data-testid="signal-trigger">{{ sections.implication.trigger_condition }}</dd>
          </div>
          <div class="sr-field">
            <dt>自我质疑</dt>
            <dd data-testid="signal-self-doubt">{{ sections.implication.self_doubt }}</dd>
          </div>
        </dl>
      </section>

      <!-- 附录（代码生成；点击正文引用展开并定位；局部横滚不撑宽整页） -->
      <details class="sr-appendix" :open="appendixOpen" data-testid="signal-appendix" @toggle="onAppendixToggle">
        <summary>想核对数字？展开数据和计算过程</summary>
        <div class="sr-appendix-body">
          <template v-if="appendixGaps.length">
            <h3 class="sr-appendix-h">缺口</h3>
            <ul class="sr-gaps">
              <li v-for="(gap, gi) in appendixGaps" :key="`g${gi}`">
                <template v-if="gap.tool">[{{ gap.tool }}] </template>{{ gap.reason }}
              </li>
            </ul>
          </template>

          <!-- question-v1 文字来源核查区：标题/URL/时间/使用片段（旧报告无此节） -->
          <template v-if="sources.length">
            <h3 class="sr-appendix-h">文字来源（本次研究实际读取的原文片段）</h3>
            <ul class="sr-sources" data-testid="signal-appendix-sources">
              <li
                v-for="src in sources"
                :id="sourceAnchorId(src.id)"
                :key="src.id"
                :class="{ 'sr-row-target': highlightRef === src.id }"
                class="sr-source"
              >
                <div class="sr-source-head">
                  <strong>{{ src.id }} · {{ src.title || '（无标题）' }}</strong>
                  <span class="sr-source-meta">{{ sourceTemporalLine(src) }}<template v-if="src.truncated"> · 片段 {{ src.original_runes }} 字符已截取</template></span>
                </div>
                <a
                  v-if="isSafeHttpUrl(src.final_url || src.url)"
                  :href="src.final_url || src.url"
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  class="sr-source-url"
                  data-testid="signal-source-link"
                >{{ src.final_url || src.url }}</a>
                <span v-else-if="src.url || src.final_url" class="sr-source-url sr-source-url--plain">{{ src.final_url || src.url }}</span>
                <p class="sr-source-text" data-testid="signal-source-text">{{ src.text }}</p>
                <p v-if="src.temporal_note" class="sr-source-note">{{ src.temporal_note }}</p>
              </li>
            </ul>
          </template>

          <h3 class="sr-appendix-h">调用账本</h3>
          <div class="sr-table-scroll">
            <table class="sr-table">
              <thead>
                <tr><th>调用</th><th>查询问题</th><th>工具</th><th>状态</th><th>来源元信息</th></tr>
              </thead>
              <tbody>
                <tr v-for="call in calls" :key="call.call_id">
                  <td>{{ call.call_id }}</td>
                  <td>{{ call.question }}</td>
                  <td>{{ call.tool }}</td>
                  <td>{{ call.status }}<template v-if="call.error">（{{ call.error }}）</template></td>
                  <td>
                    <template v-if="call.retrieved_at || call.last_modified || call.source_sha256">
                      retrieved_at {{ call.retrieved_at || '—' }} · last_modified {{ call.last_modified || '—' }} · sha256 {{ call.source_sha256 || '—' }}
                    </template>
                    <template v-else>—</template>
                  </td>
                </tr>
                <tr v-if="!calls.length"><td colspan="5">无取数调用。</td></tr>
              </tbody>
            </table>
          </div>

          <h3 class="sr-appendix-h">观测</h3>
          <div class="sr-table-scroll">
            <table class="sr-table">
              <thead>
                <tr><th>引用</th><th>指标</th><th>期间</th><th>原值</th><th>单位</th><th>缺失说明</th></tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in appendixObsRows"
                  :id="row.anchor"
                  :key="row.ref"
                  :class="{ 'sr-row-target': highlightRef === row.ref }"
                >
                  <td>{{ row.ref }}</td>
                  <td>{{ row.label }}</td>
                  <td>{{ row.period }}</td>
                  <td>{{ row.display }}</td>
                  <td>{{ row.unit }}</td>
                  <td>{{ row.missingReason || '—' }}</td>
                </tr>
                <tr v-if="!appendixObsRows.length"><td colspan="6">无观测。</td></tr>
              </tbody>
            </table>
          </div>

          <h3 class="sr-appendix-h">代码计算账本</h3>
          <div class="sr-table-scroll">
            <table class="sr-table">
              <thead>
                <tr><th>计算</th><th>操作 / 公式</th><th>输入</th><th>结果</th><th>精度</th><th>状态</th></tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in appendixCalcRows"
                  :id="row.anchor"
                  :key="row.ref"
                  :class="{ 'sr-row-target': highlightRef === row.ref }"
                >
                  <td>{{ row.ref }}</td>
                  <td>{{ row.op }}：{{ row.expression }}</td>
                  <td>{{ row.inputs.join('、') }}</td>
                  <td>{{ row.display }}<template v-if="row.unit !== '—'"> {{ row.unit }}</template></td>
                  <td>{{ row.precision }}</td>
                  <td>{{ row.status }}<template v-if="row.reason">（{{ row.reason }}）</template></td>
                </tr>
                <tr v-if="!appendixCalcRows.length"><td colspan="6">无计算。</td></tr>
              </tbody>
            </table>
          </div>
          <p class="sr-appendix-note">原值保留源精度；派生值由代码按 half-up 最多 4 位小数计算，非模型给出。</p>
        </div>
      </details>

      <!-- 显式重新研究（唯一重做入口；regenerate=true 由父级确认预算） -->
      <div class="sr-footer">
        <AppButton variant="secondary" size="sm" :disabled="regenerating" data-testid="signal-report-regenerate" @click="onRegenerate">
          重新研究（新任务）
        </AppButton>
        <span class="muted sr-footer-note">显式点击才重新取数，可能消耗预算；成功追加新版本报告，不覆盖原文。</span>
      </div>
    </template>
  </AppPageShell>
</template>

<style scoped>
.muted { color: var(--color-text-muted); }
.signal-report {
  --font-serif-display: 'Noto Serif SC', 'Source Han Serif SC', 'Songti SC', 'SimSun', serif;
  padding-top: 0.5rem;
}
.sr-topbar { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; }
.sr-topmeta { font-size: 0.75rem; }
.sr-state { display: flex; align-items: center; gap: 0.5rem; padding: 2rem 0; color: var(--color-text-muted); font-size: 0.9rem; }
.sr-state--error { color: var(--color-error, var(--color-accent)); flex-wrap: wrap; }
.sr-kicker {
  margin: 1.6rem 0 0; font-size: 0.72rem; font-weight: 700;
  letter-spacing: 0.22em; color: var(--color-accent);
}
.sr-title {
  font-family: var(--font-serif-display);
  font-size: clamp(1.55rem, 3.4vw, 2.1rem);
  font-weight: 700; line-height: 1.38; margin: 0.6rem 0 0.8rem;
  color: var(--color-text-primary); text-wrap: balance;
}
.sr-meta { display: flex; gap: 0.4rem 1.1rem; flex-wrap: wrap; font-size: 0.75rem; color: var(--color-text-muted); }
.sr-asof { margin: -0.2rem 0 0.8rem; font-size: 0.78rem; color: var(--color-text-secondary); }
.sr-retro-tag {
  font-size: 0.7rem; font-weight: 600; padding: 0.08rem 0.5rem; border-radius: 999px;
  color: var(--color-warning); background: var(--color-warning-subtle);
}
.sr-budget, .sr-regen {
  display: flex; align-items: baseline; gap: 0.4rem; flex-wrap: wrap;
  margin-top: 0.7rem; padding: 0.5rem 0.7rem; font-size: 0.78rem; line-height: 1.6;
  border-radius: 8px;
}
.sr-budget { border: 1px solid var(--color-warning); background: var(--color-warning-subtle); color: var(--color-warning); }
.sr-regen { border: 1px dashed var(--color-accent); background: var(--color-accent-subtle); color: var(--color-accent); }
.sr-sep {
  border: 0; background: var(--color-accent); opacity: 0.8;
  width: 34px; height: 3px; border-radius: 2px; margin: 1.4rem 0 1.5rem;
}
.sr-lede {
  margin: 0 0 1.4rem; padding-left: 16px;
  border-left: 2px solid var(--color-border-medium);
  font-family: var(--font-serif-display);
  font-size: 0.96rem; line-height: 1.95; color: var(--color-text-secondary);
}
.sr-section { margin: 0 0 1.6rem; }
.sr-h2 {
  font-size: 0.78rem; font-weight: 700; letter-spacing: 0.13em;
  margin: 0 0 0.7rem; color: var(--color-text-primary);
}
.sr-para { font-size: 0.9rem; line-height: 1.95; margin: 0.7rem 0; color: var(--color-text-primary); overflow-wrap: anywhere; }
.sr-ref {
  display: inline; padding: 0 1px; border: none; background: transparent;
  font: inherit; color: inherit; cursor: pointer;
  font-weight: 650;
  border-bottom: 1px dashed rgba(217, 74, 74, 0.45);
}
.sr-ref:hover { background: var(--color-accent-subtle); color: var(--color-accent); }
.sr-ref--news { cursor: help; font-weight: 500; color: var(--color-text-secondary); }
.sr-ref--missing { cursor: default; font-weight: 400; color: var(--color-text-muted); border-bottom: 1px dashed var(--color-border-medium); }

/* 小倍图（flex wrap，无边框无卡片） */
.sr-charts { display: flex; flex-wrap: wrap; gap: 1.4rem; margin: 1.4rem 0; }
.sr-chart { flex: 1 1 240px; margin: 0; min-width: 0; max-width: 420px; }
.sr-chart svg { width: 100%; height: auto; display: block; }
.sr-chart-axis { stroke: var(--color-border-medium); stroke-width: 1; }
.sr-chart-line { stroke: var(--color-accent); stroke-width: 1.8; fill: none; }
.sr-chart-dot { fill: var(--color-accent); }
.sr-chart-bar { fill: var(--color-text-muted); opacity: 0.75; }
.sr-etable { margin: 1rem 0; }
.sr-etable-title { font-size: 0.9rem; font-weight: 600; color: var(--color-text-muted); margin: 0 0 0.35rem; }
.sr-etable-table { width: 100%; border-collapse: collapse; font-size: 0.92rem; }
.sr-etable-table th { text-align: left; font-weight: 500; color: var(--color-text-muted); padding: 0.3rem 0.6rem 0.3rem 0; vertical-align: top; white-space: nowrap; }
.sr-etable-table td { padding: 0.3rem 0; vertical-align: top; }
.sr-chart-anchor { color: var(--color-text-muted); font-size: 0.88rem; margin: 0.4rem 0; }
.sr-chart-bar:last-of-type { fill: var(--color-accent); opacity: 1; }
.sr-chart-text { font-size: 10px; fill: var(--color-text-muted); }
.sr-chart figcaption { font-size: 0.72rem; color: var(--color-text-secondary); line-height: 1.6; margin-top: 0.4rem; }

/* 后果评估：3px accent 左线段落块，无底色/圆角/阴影 */
.sr-judgment { border-left: 3px solid var(--color-accent); padding: 0.1rem 0 0.1rem 16px; margin: 0 0 1.6rem; }
.sr-judgment .sr-h2 { color: var(--color-accent); }
.sr-verdict {
  font-family: var(--font-serif-display);
  font-size: 1.12rem; font-weight: 700; line-height: 1.6;
  margin: 0 0 0.6rem; color: var(--color-text-primary);
}
.sr-fields { display: flex; flex-direction: column; gap: 0.45rem; margin: 0.9rem 0 0; }
.sr-field { display: flex; gap: 0.6rem; font-size: 0.8rem; line-height: 1.7; }
.sr-field dt { flex: 0 0 auto; color: var(--color-text-muted); font-weight: 600; }
.sr-field dd { margin: 0; color: var(--color-text-secondary); overflow-wrap: anywhere; }

/* 附录：去外框，行 hairline；局部横滚不撑宽整页 */
.sr-appendix { border-top: 1px solid var(--color-border-subtle); margin-top: 1.4rem; padding-top: 0.8rem; }
.sr-appendix summary { cursor: pointer; font-size: 0.82rem; color: var(--color-text-secondary); }
.sr-appendix-body { padding-top: 0.6rem; font-size: 0.8rem; color: var(--color-text-secondary); }
.sr-appendix-h { font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; color: var(--color-text-muted); margin: 1rem 0 0.4rem; }
.sr-gaps { margin: 0; padding-left: 1.1rem; line-height: 1.8; }
.sr-table-scroll { max-width: 100%; overflow-x: auto; }
.sr-table { border-collapse: collapse; width: 100%; font-size: 0.78rem; margin: 0.3rem 0 0.8rem; }
.sr-table th, .sr-table td { text-align: left; padding: 0.4rem 0.55rem; border-bottom: 1px solid var(--color-border-subtle); vertical-align: top; overflow-wrap: anywhere; }
.sr-table th { color: var(--color-text-muted); border-bottom: 1px solid var(--color-border-medium); font-weight: 600; white-space: nowrap; }
.sr-row-target td { background: var(--color-accent-subtle); }
.sr-appendix-note { font-size: 0.72rem; color: var(--color-text-muted); }

.sr-footer {
  display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;
  border-top: 1px solid var(--color-border-subtle);
  margin-top: 1.6rem; padding-top: 1rem;
}
.sr-footer-note { font-size: 0.75rem; }
.spin { animation: sr-spin 1s linear infinite; }
@keyframes sr-spin { to { transform: rotate(360deg); } }

@media (max-width: 600px) {
  .sr-lede { font-size: 0.92rem; }
  .sr-para { font-size: 0.88rem; }
  .sr-field { flex-direction: column; gap: 0.1rem; }
}

/* ── question-v1 答题范围与文字来源核查区（add-question-driven-news-research）── */
.sr-answer {
  margin: 0 0 14px;
  padding: 10px 12px;
  border-left: 3px solid var(--tone-accent, #8a7a4f);
  background: var(--tone-surface-muted, rgba(0, 0, 0, 0.04));
  display: grid;
  gap: 6px;
}
.sr-answer-badge {
  font-weight: 600;
  font-size: 13px;
}
.sr-answer-reason {
  font-size: 13px;
  opacity: 0.9;
}
.sr-answer-unanswered {
  margin: 0;
  padding-left: 18px;
  font-size: 12.5px;
  opacity: 0.85;
}
.sr-sources {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
  display: grid;
  gap: 10px;
}
.sr-source {
  padding: 8px 10px;
  border: 1px solid var(--tone-border, rgba(0, 0, 0, 0.12));
  border-radius: 6px;
  display: grid;
  gap: 4px;
}
.sr-source-head {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
  justify-content: space-between;
}
.sr-source-meta {
  font-size: 12px;
  opacity: 0.75;
}
.sr-source-url {
  font-size: 12px;
  word-break: break-all;
  overflow-wrap: anywhere;
  color: var(--tone-link, #4a6fa5);
}
.sr-source-url--plain {
  color: inherit;
  opacity: 0.8;
}
.sr-source-text {
  margin: 0;
  font-size: 12.5px;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 220px;
  overflow-y: auto;
}
.sr-source-note {
  margin: 0;
  font-size: 11.5px;
  opacity: 0.7;
}
</style>
