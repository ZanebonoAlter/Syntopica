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
import { computed, ref, toRef, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import ArchiveMatchPopover from './ArchiveMatchPopover.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardArticleTag } from '~/api/semanticBoards'
import { matchReasonColor, stepNoOf, stepShortOf } from '~/utils/matchQuality'
import { todayInShanghai } from '../../composables/useBoardConsole'
import {
  isWeakArticle,
  ruleMeta,
  useBoardArchive,
  type ArchiveArticle,
  type ArchiveFilter,
  type ArchiveSortMode,
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
  /** 矩阵同源的最新日报日期（YYYY-MM-DD）；归档默认落在这一天。 */
  asOf: string | null
  /** 从历史格子「看归档」带入的日期；普通切换归档不传。 */
  initialDate?: string
  /** 当前版块构成标签，用于筛选覆盖面（不再混用 event tag id）。 */
  labels?: Array<{ id: number, label: string }>
}>()

const emit = defineEmits<{
  'open-article': [articleId: number]
}>()

const sbApi = useSemanticBoardsApi()

const today = computed(() => props.initialDate || props.asOf || todayInShanghai())
const startDate = ref(today.value)
const endDate = ref(today.value)
/** 展开「+N」的文章 id 集合。 */
const expandedIds = ref<Set<number>>(new Set())
/** 筛选覆盖面 / 匹配流程 两个弹窗的开关。 */
const filterDialogOpen = ref(false)
const flowDialogOpen = ref(false)
const flowConfig = ref<MatchFlowConfig | null>(null)
const flowLoading = ref(false)
const flowError = ref<string | null>(null)

watch(today, (next) => {
  startDate.value = next
  endDate.value = next
}, { immediate: true })

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

const SORTS: Array<{ key: ArchiveSortMode, label: string }> = [
  { key: 'quality', label: '质量' },
  { key: 'time', label: '时间' },
]

const PRESETS = [
  { days: 1, label: '数据日' },
  { days: 7, label: '近 7 天' },
  { days: 14, label: '近 14 天' },
  { days: 30, label: '近 30 天' },
]

function addDays(date: string, delta: number): string {
  const base = new Date(`${date}T00:00:00Z`)
  base.setUTCDate(base.getUTCDate() + delta)
  return base.toISOString().slice(0, 10)
}

function applyPreset(days: number): void {
  endDate.value = today.value
  startDate.value = days <= 1 ? today.value : addDays(today.value, -(days - 1))
}

function isPreset(days: number): boolean {
  const expected = days <= 1 ? today.value : addDays(today.value, -(days - 1))
  return startDate.value === expected && endDate.value === today.value
}

function onStartDate(value: string | number): void {
  startDate.value = String(value)
}

function onEndDate(value: string | number): void {
  endDate.value = String(value)
}

function resetFilters(): void {
  feedId.value = null
  labelId.value = null
  showDirectionMismatch.value = false
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

/** 筛选覆盖面是否有生效项（决定「筛选」按钮上的提示点）。 */
const advancedActive = computed(() => feedId.value !== null || labelId.value !== null || showDirectionMismatch.value)
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
      <p class="ar-deck-note">
        已入 = 出现在范围内任一日报分区线索（后端派生）；无命中标签的文章不会进入日报。
      </p>
    </div>

    <div class="ar-toolbar" data-testid="archive-toolbar">
      <div class="ar-group">
        <div class="ar-presets" role="group" aria-label="日期快捷筛选">
          <button
            v-for="preset in PRESETS"
            :key="preset.days"
            type="button"
            class="ar-chip"
            :class="{ 'is-active': isPreset(preset.days) }"
            :data-testid="`archive-preset-${preset.days}`"
            @click="applyPreset(preset.days)"
          >
            {{ preset.label }}
          </button>
        </div>

        <label class="ar-date-field">
          <span>从</span>
          <AppInput
            :model-value="startDate"
            type="date"
            :max="endDate"
            data-testid="archive-start-date"
            @update:model-value="onStartDate"
          />
        </label>
        <label class="ar-date-field">
          <span>到</span>
          <AppInput
            :model-value="endDate"
            type="date"
            :min="startDate"
            data-testid="archive-end-date"
            @update:model-value="onEndDate"
          />
        </label>
      </div>

      <div class="ar-seg" role="group" aria-label="组内排序">
        <button
          v-for="item in SORTS"
          :key="item.key"
          type="button"
          class="ar-seg-btn"
          :class="{ 'is-active': sortMode === item.key }"
          :aria-pressed="sortMode === item.key"
          :data-testid="`archive-sort-${item.key}`"
          @click="setSortMode(item.key)"
        >
          {{ item.label }}
        </button>
      </div>

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

      <div class="ar-toolbar-tail">
        <button
          type="button"
          class="ar-gear"
          :class="{ 'is-open': advancedActive }"
          data-testid="archive-filter-open"
          @click="filterDialogOpen = true"
        >
          <Icon icon="mdi:cog" width="14" />
          筛选
          <span v-if="advancedActive" class="ar-dot" />
        </button>
        <button type="button" class="ar-gear" data-testid="archive-flow-open" @click="openFlowDialog">
          <Icon icon="mdi:information-outline" width="14" />
          匹配流程
        </button>
      </div>
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

    <div v-else-if="articlesEmpty" class="ar-empty" data-testid="archive-empty">
      <Icon icon="mdi:calendar-filter" width="24" />
      <p>{{ range.start === range.end ? `${range.start} 没有匹配到本版块的文章。` : '这个日期范围内没有匹配到本版块的文章。' }}</p>
      <small>换个日期，或确认文章是否已经打标。</small>
    </div>

    <div v-else-if="visibleArticles.length === 0" class="ar-state" data-testid="archive-filter-empty">
      当前筛选下没有文章，换个筛选看看。
    </div>

    <div v-else class="ar-days" data-testid="archive-days">
      <section v-for="group in groupViews" :key="group.date" class="ar-day">
        <header class="ar-day-head">
          <h3 class="ar-day-date">{{ group.date }}</h3>
          <span class="ar-day-count">{{ group.count }} 篇</span>
          <span v-if="group.weakCount" class="ar-day-weak">弱证据 {{ group.weakCount }}</span>
          <span class="ar-day-sort">组内：{{ sortMode === 'quality' ? '质量（强证据在前）' : '发布时间' }}</span>
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

    <!-- 筛选覆盖面：来源 / 构成标签 / 方向不符（工具行降噪，不再常年占一行原生控件） -->
    <AppDialog v-model="filterDialogOpen" size="md" title="筛选">
      <div class="ar-filter-form">
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
        <AppToggle
          v-model="showDirectionMismatch"
          label="显示方向不符标签"
          data-testid="archive-direction-toggle"
        />
      </div>
      <template #footer>
        <button type="button" class="ar-text-btn" data-testid="archive-filter-reset" @click="resetFilters">
          清空筛选
        </button>
      </template>
    </AppDialog>

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
