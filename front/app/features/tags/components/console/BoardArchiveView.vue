<script setup lang="ts">
/**
 * 归档视图（rework-workshop-into-radar-and-desk 8G.3c，2026-10-31 归档重做）。
 *
 * 产品定位（对齐 ui-design-archive-rework.md v10 与原型
 * ui-prototype/revision-2026-10-31-archive-rework/standalone.html）：
 *  - deck 行（v11 / FB-14）：`范围 · N 篇 ｜ 已进 X（P%）+ 比例条 ｜ 未进 Y ｜ 弱证据 Z（可与入报交叉）`
 *    + 口径两句直接可见；四格大数字卡片已退役（与列表设计语言分行，且与 chip 计数重复）；
 *  - 工具行（v11 收敛）：日期快捷 + 起止 ｜ 组内排序（质量/时间）｜ 全部/未纳入日报/弱证据
 *    （不带计数，计数只在 deck）｜ 右端 ⚙筛选（来源 + 构成标签 + 方向不符）+ ⓘ匹配流程；
 *  - 主体：按日期分组平铺文章行，四列「时间 | 主体 | 最强匹配 | 状态」；行内标签带
 *    按匹配规则四色编码（规则色边框 + 规则色分数、降级半透明、方向不符虚线边 + ⊘），
 *    超过 3 个折叠「+N」，标签带末尾挂「入板」chip（虚线边，标本文靠第几步入板）。
 *
 * 交互契约：
 *  - 行主点击 = emit('open-article')（控制台 Shell 已接 ArticlePreviewModal，本组件不开弹窗）；
 *  - 点标签 chip = 标签卡 popover；点「入板」chip = 入板过程 popover（均 stopPropagation）；
 *  - 归档只读：无补进日报 / 重跑 / 改匹配等写动作。
 *
 * 数据口径不变：in_report / weak_evidence / 组内 quality 序均由后端派生，前端只读不重排；
 * 日期分组走 groupedArticles（Map 聚合，避免 quality 序下逐条相邻切段的碎组）。
 */
import { computed, nextTick, ref, toRef, useId, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import ArchiveMatchPopover from './ArchiveMatchPopover.vue'
import ObservationRangeControls from './ObservationRangeControls.vue'
import ArchiveHistoryStrip from './ArchiveHistoryStrip.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardArticleTag } from '~/api/semanticBoards'
import { AI_SUSPECT_LABEL, isAiSuspect } from '~/utils/aiDetection'
import { matchReasonColor, stepNoOf, stepShortOf } from '~/utils/matchQuality'
import { todayInShanghai } from '../../composables/useBoardConsole'
import { useArchiveDailyCounts } from '../../composables/useArchiveDailyCounts'
import {
  shiftRangeWithViewport,
  sliceSeries,
  viewportFor,
} from './archiveTimeSeries'
import {
  clampRangeToBounds,
  daysBetweenInclusive,
  sameRange,
  shiftDate,
  type DateBounds,
  type ObservationRange,
} from './observationRange'
import {
  isWeakArticle,
  ruleMeta,
  useBoardArchive,
  type ArchiveArticle,
  type ArchiveFilter,
} from '../../composables/useBoardArchive'
import {
  buildGenericFlowSteps,
  directionCheckNote,
  flowConfigFromApi,
  HIT_AND_STOP_NOTE,
  type MatchFlowConfig,
} from './archiveMatchFlow'

const props = defineProps<{
  boardId: number
  /** 从历史格子「看归档」带入的日期；普通切换归档不传。 */
  initialDate?: string
  /** 当前版块构成标签，用于筛选覆盖面（不再混用 event tag id）。 */
  labels?: Array<{ id: number, label: string }>
}>()

const emit = defineEmits<{
  'open-article': [articleId: number]
}>()

const sbApi = useSemanticBoardsApi()

// ── 时间导航块状态（archive-time-chart-range） ──────────────────────────────
// 不变量：committedRange ⊆ viewport（主图按 bounds 下标高亮，区间越界会画歪）。
// 三处入口（主图刷选 / 导航条平移 / 日期表单）全部收敛到 commitRange 与
// onViewport* 三个函数，不各自改状态。

/** 基准日：initialDate 优先，否则日历今天（Asia/Shanghai）；旧版日报锚点不再作默认。 */
const baseDate = computed(() => props.initialDate || todayInShanghai())

/** 视窗右端（含）。 */
const anchorEnd = ref(baseDate.value)
/** 视窗宽度（天）：默认 90；表单提交了更宽的区间才放宽（只放宽不收紧，避免视窗跳变）。 */
const viewportDays = ref(90)
/** 已提交区间 = 列表请求范围。 */
const committedRange = ref<ObservationRange>({ start: baseDate.value, end: baseDate.value })
/** 手势 / 拖动中的临时区间（只喂主图，不发请求）。 */
const draftRange = ref<ObservationRange | null>(null)
/** 导航条拖动基线：手势开始时的视窗右端 / 视窗起点 / 已提交区间（Esc 回滚 + 同步平移的参照）。 */
interface ViewportDragBase {
  anchorEnd: string
  viewportStart: string
  range: ObservationRange
}

/** 导航条拖动基线（null = 不在拖动中）。delta 必须相对基线算，不能在已更新的视窗上算增量。 */
const viewportDrag = ref<ViewportDragBase | null>(null)

const {
  earliest: countsEarliest,
  latest: countsLatest,
  byDate: countsByDate,
  loading: countsLoading,
  error: countsError,
  reload: reloadCounts,
} = useArchiveDailyCounts(toRef(props, 'boardId'))

/** 视窗右端上限：max(今天, 最新命中日)；基准日兜底（initialDate 可能是未来日）。 */
const upperBound = computed(() => {
  const today = todayInShanghai()
  let bound = today
  if (countsLatest.value && countsLatest.value > bound) bound = countsLatest.value
  if (baseDate.value > bound) bound = baseDate.value
  return bound
})

/** 当前视窗（主图 bounds = 视窗区间）。 */
// earliest 直接交给 viewportFor：它已处理「anchor 早于 earliest」与「earliest 晚于上界」
// 两种边界（前者把视窗抬到能让区间完整可见的位置，后者退化为单点）——不能在这里
// 把 earliest 抹成 null，否则「计数到的历史晚于已提交区间」时视窗会丢掉左贴，
// committedRange ⊆ viewport 的不变量就失守（archive-time-chart-range R-3）。
const viewport = computed(() => viewportFor(
  anchorEnd.value,
  countsEarliest.value,
  upperBound.value,
  viewportDays.value,
))

/** 视窗实际宽度（贴齐最早日时变窄）。 */
const viewportSpan = computed(() => daysBetweenInclusive(viewport.value.start, viewport.value.end))

/** 主图曲线：视窗内每日篇数（缺日 = 已知 0，补基线）。 */
const viewportSeries = computed(() => sliceSeries(viewport.value, countsByDate.value))

/** 日期表单可选界：全历史（最早有文章日 → 今天）；计数未就绪时兜底为一窗。 */
const fullBounds = computed<DateBounds>(() => ({
  start: countsEarliest.value ?? shiftDate(upperBound.value, -(viewportSpan.value - 1)),
  end: upperBound.value,
}))

/** 主图反馈行文案（空态 / 失败态；成功态不占行）。 */
const chartNotice = computed<string | null>(() => {
  if (countsError.value) return '时间曲线加载失败，点「重试」重拉计数'
  if (!countsLoading.value && countsEarliest.value === null) return '本版块还没有命中文章，暂无时间曲线'
  return null
})

/** 区间是否落在视窗内（committedRange ⊆ viewport 的判定）。 */
function insideViewport(range: ObservationRange): boolean {
  return range.start >= viewport.value.start && range.end <= viewport.value.end
}

/** 提交区间：越界时先把视窗右端跟到区间末（区间完整可见），再写区间。 */
function commitRange(range: ObservationRange): void {
  const width = daysBetweenInclusive(range.start, range.end)
  if (width > viewportDays.value) viewportDays.value = width
  if (!insideViewport(range)) {
    anchorEnd.value = range.end
  }
  draftRange.value = null
  viewportDrag.value = null
  // 同值提交不重拉（拖动原位 / 点同一天时避免多一次列表请求）。
  if (sameRange(range, committedRange.value)) return
  committedRange.value = { start: range.start, end: range.end }
}

/** 主图拖动中：只写草稿（列表不重拉，deck 数字保持上次提交值）。 */
function onChartPreview(range: ObservationRange): void {
  draftRange.value = range
}

/** 主图松手 / 日期表单提交：写区间（列表重拉一次）。 */
function onChartCommit(range: ObservationRange): void {
  commitRange(range)
}

/** 取（或建立）本次拖动的基线：delta 以基线视窗起点为准，否则拖动中视窗已随 anchorEnd 移动，增量会算成 0。 */
function viewportDragBase(): ViewportDragBase {
  if (viewportDrag.value) return viewportDrag.value
  const base: ViewportDragBase = {
    anchorEnd: anchorEnd.value,
    viewportStart: viewport.value.start,
    range: { ...committedRange.value },
  }
  viewportDrag.value = base
  return base
}

/** 区间随视窗同步平移同偏移（相对拖动基线，不累加）。 */
function shiftedRangeFor(base: ViewportDragBase, next: ObservationRange): ObservationRange {
  const delta = daysBetweenInclusive(base.viewportStart, next.start) - 1
  return shiftRangeWithViewport(base.range, delta, next)
}

/** 导航条拖动中：视窗与已提交区间同步平移同偏移（只写草稿，不发请求）。 */
function onViewportPreview(next: ObservationRange): void {
  const base = viewportDragBase()
  anchorEnd.value = next.end
  draftRange.value = shiftedRangeFor(base, next)
}

/** 导航条松手 / 键盘步进：视窗与区间一起提交（一次列表请求）。 */
function onViewportCommit(next: ObservationRange): void {
  const base = viewportDragBase()
  anchorEnd.value = next.end
  const shifted = shiftedRangeFor(base, next)
  viewportDrag.value = null
  draftRange.value = null
  if (sameRange(shifted, committedRange.value)) return
  committedRange.value = shifted
}

/** 导航条取消（Esc / pointercancel）：回到拖动前的视窗与区间，不提交。 */
function onViewportCancel(): void {
  if (viewportDrag.value) anchorEnd.value = viewportDrag.value.anchorEnd
  viewportDrag.value = null
  draftRange.value = null
}

watch(baseDate, (next) => {
  anchorEnd.value = next
  committedRange.value = { start: next, end: next }
  draftRange.value = null
  viewportDrag.value = null
})

/**
 * 计数到达/变化后重建不变量 `committedRange ⊆ viewport`。
 *
 * 触发场景：进归档时计数未回 → 视窗先按「今天往前 90 天」兜底，用户此时用日期表单
 * 提交了更早的区间；计数回来后 `earliest` 把视窗左端抬到最早有文章日（视窗收窄），
 * 已提交区间就可能漏到视窗外——那一瞬间 `ObservationRangeControls` 会拿到负下标
 * 把选区画到轴外，后续手势还会从轴上取到空日期。
 */
watch([countsEarliest, upperBound], () => {
  if (insideViewport(committedRange.value)) {
    if (draftRange.value && !insideViewport(draftRange.value)) draftRange.value = null
    return
  }
  // 先试「把视窗右端跟到区间末」；仍越界（区间起点早于 earliest）再把区间钳进视窗。
  anchorEnd.value = committedRange.value.end
  if (insideViewport(committedRange.value)) return
  const clamped = clampRangeToBounds(committedRange.value, viewport.value)
  draftRange.value = null
  viewportDrag.value = null
  if (!sameRange(clamped, committedRange.value)) committedRange.value = clamped
})

/** 展开「+N」的文章 id 集合。 */
const expandedIds = ref<Set<number>>(new Set())
const moreFiltersOpen = ref(false)
const filterPanelId = useId()
const moreButton = ref<HTMLButtonElement | null>(null)
const flowDialogOpen = ref(false)
const flowConfig = ref<MatchFlowConfig | null>(null)
const flowLoading = ref(false)
const flowError = ref<string | null>(null)

const startDate = computed(() => committedRange.value.start)
const endDate = computed(() => committedRange.value.end)

const {
  visibleArticles,
  groupedArticles,
  stats,
  filter,
  sortMode,
  setSortMode,
  feedId,
  feedOptions,
  labelId,
  showDirectionMismatch,
  loading,
  error,
  truncated,
  range,
  reload,
} = useBoardArchive(toRef(props, 'boardId'), startDate, endDate)

const FILTERS: Array<{ key: ArchiveFilter, label: string }> = [
  { key: 'all', label: '全部' },
  { key: 'not_in_report', label: '未纳入日报' },
  { key: 'weak', label: '弱证据' },
]

const activeFilters = computed(() => {
  const items: Array<{ key: 'feed' | 'label', text: string }> = []
  if (feedId.value !== null) {
    const name = feedOptions.value.find(item => item.id === feedId.value)?.name
    items.push({ key: 'feed', text: name ? `来源：${name}` : '已选来源' })
  }
  if (labelId.value !== null) {
    const name = props.labels?.find(item => item.id === labelId.value)?.label
    items.push({ key: 'label', text: name ? `标签：${name}` : '已选标签' })
  }
  return items
})
const hasFilters = computed(() => filter.value !== 'all' || activeFilters.value.length > 0)

function changeSort(event: Event): void {
  setSortMode((event.target as HTMLSelectElement).value === 'time' ? 'time' : 'quality')
}

function focusFilterControl(): void {
  void nextTick(() => moreButton.value?.focus())
}

function removeFilter(key: 'feed' | 'label'): void {
  if (key === 'feed') feedId.value = null
  else labelId.value = null
  focusFilterControl()
}

function resetFilters(): void {
  filter.value = 'all'
  feedId.value = null
  labelId.value = null
  focusFilterControl()
}

// ── 行内容派生 ────────────────────────────────────────────────────────────

/** 行内标签 chip 的展示信息（规则色、分数、提示文案一次算好）。 */
interface TagView {
  id: number
  label: string
  score: string
  color: string
  title: string
  mismatch: boolean
  /** 原始标签对象（popover 入口要原样传下去）。 */
  raw: BoardArticleTag
}

/** 最强匹配列 + 入板 chip 需要的字段。 */
interface StrongestView {
  id: number
  ruleLabel: string
  score: string
  color: string
  stepNo: string
  stepShort: string
}

interface RowView {
  id: number
  article: ArchiveArticle
  time: string
  tags: TagView[]
  hiddenTags: number
  hasTags: boolean
  strongest: StrongestView | null
  inReport: boolean
  weak: boolean
  /** 疑似 AI 生成（add-ai-generated-article-detection）：仅 suspect 挂标记，未判定/非 AI 不占位。 */
  suspect: boolean
}

interface GroupView {
  date: string
  count: number
  weakCount: number
  rows: RowView[]
}

function articleTags(article: ArchiveArticle): BoardArticleTag[] {
  return article.filtered_tags ?? []
}

function tagColor(tag: BoardArticleTag): string {
  return matchReasonColor(tag.match_reason)
}

function tagScore(tag: BoardArticleTag): string {
  return Number.isFinite(tag.score) ? tag.score.toFixed(2) : '--'
}

/** chip hover 提示：规则 + 分数 + 描述摘要。 */
function tagTitle(tag: BoardArticleTag): string {
  const meta = ruleMeta(tag.match_reason)
  const summary = (tag.description || meta.semantics).slice(0, 60)
  return `${meta.label} ${tagScore(tag)} · ${summary}…（点击展开完整描述与计算明细）`
}

function tagView(tag: BoardArticleTag): TagView {
  return {
    id: tag.id,
    label: tag.label,
    score: tagScore(tag),
    color: tagColor(tag),
    title: tagTitle(tag),
    mismatch: tag.direction_mismatch,
    raw: tag,
  }
}

/**
 * 最强判定标签 = `filtered_tags[0]`。
 *
 * 后端已按归档证据层级（ArchiveEvidenceRank）→ 分数 → id 排好序，首条即本文
 * 最强证据标签（backend-go/.../board_match_handler.go 组装 filtered_tags 时的排序）。
 * 前端**信后端序**，不再按最高分另推一套口径（不复制 BestTier/BestScore）。
 */
function strongestTag(article: ArchiveArticle): BoardArticleTag | null {
  return articleTags(article)[0] ?? null
}

function strongestView(article: ArchiveArticle): StrongestView | null {
  const best = strongestTag(article)
  if (!best) return null
  return {
    id: best.id,
    ruleLabel: ruleMeta(best.match_reason).label,
    score: tagScore(best),
    color: tagColor(best),
    stepNo: stepNoOf(best.match_reason),
    stepShort: stepShortOf(best.match_reason),
  }
}

function timestamp(article: ArchiveArticle): string {
  return (article.pub_date ?? '').replace('T', ' ').slice(5, 16)
}

/** 展开 / 收起「+N」折叠的标签。 */
function toggleTags(articleId: number): void {
  const next = new Set(expandedIds.value)
  if (next.has(articleId)) next.delete(articleId)
  else next.add(articleId)
  expandedIds.value = next
}

/**
 * 分组行视图（模板只读派生结果，避免模板里重复算同一份东西）。
 *
 * 组内顺序沿用后端 quality 原序（前端不重排）；未展开时只露前 3 个标签，其余折叠「+N」。
 */
const groupViews = computed<GroupView[]>(() => groupedArticles.value.map(group => {
  const rows = group.articles.map((article): RowView => {
    const tags = articleTags(article)
    const shown = expandedIds.value.has(article.id) ? tags : tags.slice(0, 3)
    return {
      id: article.id,
      article,
      time: timestamp(article),
      tags: shown.map(tagView),
      hiddenTags: Math.max(0, tags.length - shown.length),
      hasTags: tags.length > 0,
      strongest: strongestView(article),
      inReport: !!article.in_report,
      weak: isWeakArticle(article),
      suspect: isAiSuspect(article.aiDetectionStatus),
    }
  })
  return {
    date: group.date,
    count: rows.length,
    weakCount: rows.filter(row => row.weak).length,
    rows,
  }
}))

// ── popover 入口 ─────────────────────────────────────────────────────────

type PopState =
  | { key: string, mode: 'tag', article: ArchiveArticle, tag: BoardArticleTag, anchor: HTMLElement | null }
  | { key: string, mode: 'article', article: ArchiveArticle, anchor: HTMLElement | null }

const pop = ref<PopState | null>(null)

function openTagPopover(article: ArchiveArticle, tag: BoardArticleTag, event: MouseEvent): void {
  pop.value = {
    key: `tag-${article.id}-${tag.id}`,
    mode: 'tag',
    article,
    tag,
    anchor: (event.currentTarget as HTMLElement | null) ?? null,
  }
}

function openFlowPopover(article: ArchiveArticle, event: MouseEvent): void {
  pop.value = {
    key: `flow-${article.id}`,
    mode: 'article',
    article,
    anchor: (event.currentTarget as HTMLElement | null) ?? null,
  }
}

// ── 匹配流程（通用说明，阈值取全局匹配配置真实值）─────────────────────────

const flowSteps = computed(() => (flowConfig.value ? buildGenericFlowSteps(flowConfig.value) : []))

async function openFlowDialog(): Promise<void> {
  flowDialogOpen.value = true
  if (flowConfig.value) return
  flowLoading.value = true
  flowError.value = null
  try {
    const res = await sbApi.getMatchingConfig()
    if (res?.success && res.data) flowConfig.value = flowConfigFromApi(res.data)
    else flowError.value = res?.error || '匹配参数加载失败'
  } catch {
    flowError.value = '匹配参数加载失败'
  } finally {
    flowLoading.value = false
  }
}

// ── 状态展示 ─────────────────────────────────────────────────────────────

const articlesEmpty = computed(() => stats.value.total === 0)

/** 已进日报占比（整数百分比）；窗口内 0 篇时返回 null，不显示 NaN%。 */
const inReportPct = computed<number | null>(() => {
  const total = stats.value.total
  if (!total) return null
  return Math.round((stats.value.inReport / total) * 100)
})

/** deck 行日期：单日显示当天，跨日显示「起点 → 终点」。 */
const rangeLabel = computed(() => (
  range.value.start === range.value.end
    ? range.value.start
    : `${range.value.start} → ${range.value.end}`
))

/** 筛选 chip 的计数只进 title（数字在 deck 出现一次，chip 只做动作）。 */
const filterCounts = computed<Record<ArchiveFilter, number>>(() => ({
  all: stats.value.total,
  not_in_report: stats.value.notInReport,
  weak: stats.value.weak,
}))

const listCount = computed(() => {
  const shown = visibleArticles.value.length
  if (truncated.value) return `显示 ${shown} 篇`
  if (!hasFilters.value) return `${shown} 篇`
  if (!activeFilters.value.length) return `显示 ${shown} / ${stats.value.total} 篇`
  if (countsLoading.value || countsError.value) return `显示 ${shown} 篇`
  let total = 0
  for (const [date, count] of countsByDate.value) {
    if (date >= range.value.start && date <= range.value.end) total += count
  }
  return total >= stats.value.total ? `显示 ${shown} / ${total} 篇` : `显示 ${shown} 篇`
})
</script>

<template>
  <section class="archive" data-testid="board-archive">
    <div class="ar-deck" data-testid="archive-window">
      <div class="ar-deck-nums">
        <span class="ar-deck-range">
          <span class="ar-deck-date">{{ rangeLabel }}</span>
          <span class="ar-deck-dot" aria-hidden="true">·</span>
          <span class="ar-deck-total" data-testid="archive-stat-total">{{ stats.total }} 篇</span>
        </span>

        <span class="ar-deck-rule" aria-hidden="true" />

        <span class="ar-deck-item" data-testid="archive-stat-in">
          <span class="ar-deck-label">已进日报</span>
          <b class="ar-deck-num">{{ stats.inReport }}</b>
          <span
            class="ar-deck-bar"
            role="img"
            :aria-label="inReportPct === null ? '窗口内无文章' : `已进日报占 ${inReportPct}%`"
          >
            <i :style="{ width: `${inReportPct ?? 0}%` }" />
          </span>
          <span class="ar-deck-pct" data-testid="archive-stat-in-pct">{{ inReportPct === null ? '—' : `${inReportPct}%` }}</span>
        </span>

        <span class="ar-deck-rule" aria-hidden="true" />

        <span class="ar-deck-item" data-testid="archive-stat-out">
          <span class="ar-deck-label">未进</span>
          <b class="ar-deck-num">{{ stats.notInReport }}</b>
        </span>

        <span class="ar-deck-rule" aria-hidden="true" />

        <span class="ar-deck-item" data-testid="archive-stat-weak">
          <span class="ar-deck-label">弱证据</span>
          <b class="ar-deck-num">{{ stats.weak }}</b>
          <span class="ar-deck-hint">可与入报交叉</span>
        </span>
      </div>
      <div class="ar-deck-meta">
        <p class="ar-deck-note">已入 = 出现在范围内任一日报分区线索；无命中标签的文章不会进入日报。</p>
        <AppToggle v-model="showDirectionMismatch" label="显示方向不符标签" data-testid="archive-direction-toggle" />
        <button type="button" class="ar-gear" data-testid="archive-flow-open" @click="openFlowDialog">
          <Icon icon="mdi:information-outline" width="14" />
          匹配流程
        </button>
      </div>
    </div>



    <!-- 时间导航块（archive-time-chart-range）：日期表单 + 90 天视窗主图 + 全历史导航条 -->
    <section class="ar-time-nav" data-testid="archive-time-nav">
      <div v-if="countsError" class="ar-time-error">
        <button
          type="button"
          class="ar-text-btn"
          data-testid="archive-time-retry"
          @click="reloadCounts"
        >重试</button>
      </div>

      <ObservationRangeControls
        :bounds="viewport"
        :range="draftRange ?? committedRange"
        :source-days="viewportSpan"
        :series="countsLoading || countsEarliest === null ? null : viewportSeries"
        :tiers="[]"
        :max-span-days="viewportSpan"
        :selectable-bounds="fullBounds"
        :loading="countsLoading"
        :notice="chartNotice"
        unit-label="篇"
        title="时间轴"
        default-collapsed
        collapse-storage-key="syntopica.archive.time.collapsed"
        :caption="[
          '每日进版块文章数（全版块命中）',
          '篇 · 命中本版块标签的文章数，非线索数；不随列表筛选变化',
        ]"
        @preview="onChartPreview"
        @commit="onChartCommit"
      >
        <template #footer>
          <ArchiveHistoryStrip
            :earliest="countsEarliest"
            :latest="upperBound"
            :by-date="countsByDate"
            :viewport="viewport"
            :disabled="countsLoading"
            @viewport-preview="onViewportPreview"
            @viewport-commit="onViewportCommit"
            @viewport-cancel="onViewportCancel"
          />
        </template>
      </ObservationRangeControls>
    </section>

    <div class="ar-list-head">
      <h2>文章列表</h2>
      <span v-if="!loading && !error" data-testid="archive-list-count" role="status">{{ listCount }}</span>
    </div>
    <div class="ar-toolbar" data-testid="archive-toolbar">
      <div class="ar-toolbar-leading">
      <div class="ar-filters" role="group" aria-label="归档筛选">
        <button
          v-for="item in FILTERS"
          :key="item.key"
          type="button"
          class="ar-chip"
          :class="{ 'is-active': filter === item.key }"
          :aria-pressed="filter === item.key"
          :title="`${item.label} ${filterCounts[item.key]} 篇`"
          :data-testid="`archive-filter-${item.key}`"
          @click="filter = item.key"
        >
          {{ item.label }}
        </button>
      </div>
        <button
          type="button"
          class="ar-gear"
          :aria-expanded="moreFiltersOpen"
          :aria-controls="filterPanelId"
          ref="moreButton"
          data-testid="archive-filter-open"
          @click="moreFiltersOpen = !moreFiltersOpen"
        >更多筛选{{ activeFilters.length ? ` · ${activeFilters.length}` : '' }}</button>
      </div>
      <label class="ar-select">
        组内排序
        <select data-testid="archive-sort" :value="sortMode" @change="changeSort">
          <option value="quality">强证据优先</option>
          <option value="time">发布时间 ↓</option>
        </select>
      </label>
    </div>
    <div v-show="moreFiltersOpen" :id="filterPanelId" class="ar-filter-form">
      <label class="ar-select">
        <span>来源</span>
        <select v-model="feedId" data-testid="archive-feed-filter">
          <option :value="null">全部来源</option>
          <option v-for="feed in feedOptions" :key="feed.id" :value="feed.id">{{ feed.name }}</option>
        </select>
      </label>
      <label class="ar-select">
        <span>构成标签</span>
        <select v-model="labelId" data-testid="archive-label-filter">
          <option :value="null">全部标签</option>
          <option v-for="label in labels ?? []" :key="label.id" :value="label.id">{{ label.label }}</option>
        </select>
      </label>
    </div>
    <div v-if="hasFilters" class="ar-active-filters" aria-label="已选筛选">
      <button
        v-for="item in activeFilters"
        :key="item.key"
        type="button"
        class="ar-gear"
        :aria-label="`移除筛选：${item.text}`"
        @click="removeFilter(item.key)"
      >{{ item.text }} <span aria-hidden="true">×</span></button>
      <button type="button" class="ar-text-btn" data-testid="archive-filter-reset" @click="resetFilters">清空筛选</button>
    </div>

    <div v-if="loading" class="ar-state" data-testid="archive-loading">
      <span class="ar-spinner" />
      正在加载归档…
    </div>

    <div v-else-if="error" class="ar-state is-error" data-testid="archive-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <button type="button" class="ar-text-btn" data-testid="archive-retry" @click="reload">重试</button>
    </div>

    <div v-else-if="articlesEmpty && !hasFilters" class="ar-empty" data-testid="archive-empty">
      <Icon icon="mdi:calendar-filter" width="24" />
      <p>{{ range.start === range.end ? `${range.start} 没有匹配到本版块的文章。` : '这个日期范围内没有匹配到本版块的文章。' }}</p>
      <small>换个日期，或确认文章是否已经打标。</small>
    </div>

    <div v-else-if="visibleArticles.length === 0" class="ar-state" data-testid="archive-filter-empty">
      当前筛选下没有文章。
      <button type="button" class="ar-text-btn" @click="resetFilters">清空筛选</button>
    </div>

    <div v-else class="ar-days" data-testid="archive-days">
      <section v-for="group in groupViews" :key="group.date" class="ar-day">
        <header class="ar-day-head">
          <h3 class="ar-day-date">{{ group.date }}</h3>
          <span class="ar-day-count">{{ group.count }} 篇</span>
          <span v-if="group.weakCount" class="ar-day-weak">弱证据 {{ group.weakCount }}</span>
        </header>

        <div
          v-for="row in group.rows"
          :key="row.id"
          class="ar-row"
          :data-testid="`archive-row-${row.id}`"
          @click="emit('open-article', row.id)"
        >
          <time class="ar-row-time">{{ row.time }}</time>

          <div class="ar-row-main">
            <button type="button" class="ar-row-title" :title="row.article.title">
              {{ row.article.title }}
            </button>
            <div class="ar-row-sub">
              <span class="ar-row-feed">{{ row.article.feed_name }}</span>
              <span class="ar-row-tags">
                <button
                  v-for="tag in row.tags"
                  :key="tag.id"
                  type="button"
                  class="ar-tag"
                  :class="{ 'is-mismatch': tag.mismatch }"
                  :style="{ borderColor: tag.color }"
                  :title="tag.title"
                  :data-testid="`archive-tag-${row.id}-${tag.id}`"
                  @click.stop="openTagPopover(row.article, tag.raw, $event)"
                >
                  <span class="ar-tag-label">{{ tag.label }}</span>
                  <span class="ar-tag-score" :style="{ color: tag.color }">{{ tag.score }}</span>
                  <span v-if="tag.mismatch" class="ar-tag-mark">⊘</span>
                </button>
                <button
                  v-if="row.hiddenTags > 0"
                  type="button"
                  class="ar-tag-more"
                  :data-testid="`archive-tag-more-${row.id}`"
                  @click.stop="toggleTags(row.id)"
                >
                  +{{ row.hiddenTags }}
                </button>
                <span v-if="!row.hasTags" class="ar-tag is-empty" title="无命中标签的文章不会进入日报">无标签</span>
                <button
                  v-if="row.strongest"
                  type="button"
                  class="ar-flow-chip"
                  :style="{ borderColor: row.strongest.color }"
                  :title="`本文靠第 ${row.strongest.stepNo} 步进入版块；点击看本文在各阶段的得分`"
                  :data-testid="`archive-flow-chip-${row.id}`"
                  @click.stop="openFlowPopover(row.article, $event)"
                >
                  <span class="ar-flow-label">入板</span>
                  <span class="ar-flow-no" :style="{ color: row.strongest.color }">{{ row.strongest.stepNo }}</span>
                  <span class="ar-flow-label">{{ row.strongest.stepShort }}</span>
                </button>
                <!-- 疑似 AI 生成：中性标记追加在标签带末尾，不占「入板」步号、不用规则色（spec: 归档视图文章行标注） -->
                <span
                  v-if="row.suspect"
                  class="ar-suspect-chip"
                  title="判定为疑似 AI 生成；可在文章详情的手动操作里纠正"
                  :data-testid="`archive-suspect-chip-${row.id}`"
                >{{ AI_SUSPECT_LABEL }}</span>
              </span>
            </div>
          </div>

          <div class="ar-row-strongest" data-testid="archive-strongest">
            <template v-if="row.strongest">
              <span class="ar-strongest-rule">最强匹配</span>
              <span class="ar-strongest-value" :style="{ color: row.strongest.color }">
                {{ row.strongest.ruleLabel }} {{ row.strongest.score }}
              </span>
            </template>
          </div>

          <div class="ar-row-status" :class="{ 'is-weak': row.weak }" data-testid="archive-status">
            <span v-if="row.inReport" class="is-in">已入</span>
            <span v-else class="is-out">未入日报</span>
            <span v-if="row.weak" class="is-weak-flag">弱证据</span>
          </div>
        </div>
      </section>
    </div>

    <p v-if="truncated" class="ar-truncated">结果超过 2000 篇，仅展示前 2000 篇；请缩小日期范围。</p>



    <!-- 匹配流程：对所有构成标签一致的通用说明（阈值取全局匹配配置真实值） -->
    <AppDialog v-model="flowDialogOpen" size="md" title="匹配流程 · 版块怎么判定标签归属（对所有构成标签通用）">
      <p v-if="flowLoading" class="ar-flow-note" data-testid="archive-flow-loading">正在加载匹配参数…</p>
      <p v-else-if="flowError" class="ar-flow-note is-error" data-testid="archive-flow-error">
        {{ flowError }}（阈值未取到，不展示猜测值）
      </p>
      <template v-else>
        <p class="ar-flow-note">{{ HIT_AND_STOP_NOTE }}</p>
        <div
          v-for="step in flowSteps"
          :key="step.id"
          class="ar-flow-step"
          data-testid="archive-flow-step"
        >
          <div class="ar-flow-step-head">
            <strong>{{ step.title }}</strong>
          </div>
          <p class="ar-flow-step-desc">{{ step.desc }}</p>
          <p class="ar-flow-step-result">{{ step.result }}</p>
        </div>
        <p v-if="flowConfig" class="ar-flow-note">{{ directionCheckNote(flowConfig) }}</p>
      </template>
    </AppDialog>

    <ArchiveMatchPopover
      v-if="pop"
      :key="pop.key"
      :board-id="boardId"
      :mode="pop.mode"
      :article="pop.article"
      :tag="pop.mode === 'tag' ? pop.tag : null"
      :anchor="pop.anchor"
      @close="pop = null"
      @open-article="id => emit('open-article', id)"
    />
  </section>
</template>

<style scoped src="./boardArchiveView.css" />
