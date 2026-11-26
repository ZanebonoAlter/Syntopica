<script setup lang="ts">
/**
 * 线索行内的「相关线索」chip 行（replace-lane-lifeline-with-topic-events D7）。
 *
 * 形态红线（ui-design.md 已批准，不得改）：**线索行内 chip + 悬停/聚焦预览浮层 +
 * 点击直接开站内阅读预览**。不做按天分组的事件列表/时间轴（v1 已否决）。
 *
 * 渲染口径：
 * - 只读宿主的推荐缓存条目（取数由宿主 `useThreadRelatedEvents` 负责），本组件零 API 副作用；
 * - 最多 3 个 chip，其余折叠成「还有 N 条」（展开仍只是 chip，不跳转、不弹层）；
 * - **没有候选就整行不渲染**（不占位、不留最小高度、不写「暂无相关」）——没有相关是常态，
 *   不该每条线索都念一遍；错误态同理静默；
 * - chip 只写候选线索标题（不写分数、不用标签名），分数与档位只进浮层——裸小数会被读成
 *   置信度/重要度；
 * - 窗口档位（7/30 天）**不在本组件**：`7 天 / 30 天` 是分区级工具条（宿主
 *   DailyReportTopicSection 的 zone header），否则没有候选的线索行会留下一条只有档位
 *   控件的空行（review R-3）；
 * - 浮层打开时登记 overlayStack（review R-4）：阅读台的 Esc 闸门据此让位，同一击键
 *   只关浮层、不把用户踢出日报。
 */
import { computed, nextTick, onUnmounted, ref } from 'vue'
import { Z_INDEX } from '~/components/ui/layout-contract'
import { popOverlay, pushOverlay } from '~/components/ui/overlayStack'
import { formatRelationScore, relationLevelLabel } from '~/utils/threadRelation'
import type { ThreadRelationItem, ThreadRelationsResponse } from '~/api/dailyReports'
import type { ArticleOpenTarget } from '~/utils/threadRelation'
import type { RequestCacheEntry } from './dailyReportMagazine'

const props = defineProps<{
  threadId: number
  reportId: number
  entry: RequestCacheEntry<ThreadRelationsResponse>
}>()

const emit = defineEmits<{
  /** 数字入参 = 无相关度上下文的普通文章；对象入参 = chip 点击（带档位/分数/候选日期）。 */
  openArticle: [target: ArticleOpenTarget]
}>()

/** chip 折叠阈值（界面上最多 3 个，其余收进「还有 N 条」）。 */
const MAX_CHIPS = 3
/** 浮层与锚点/视口边缘的间距（px）。 */
const TIP_GAP = 8
const VIEWPORT_PAD = 12

// 层级取档位表（review R-12：不再手写 `readingDialog - 1` 的表外私有值）。
const TIP_Z_INDEX = Z_INDEX.readingTooltip

const expanded = ref(false)
const active = ref<ThreadRelationItem | null>(null)
const tipRef = ref<HTMLElement | null>(null)
const tipPosition = ref({ left: 0, top: 0 })
let hideTimer: ReturnType<typeof setTimeout> | null = null
/** 浮层是否已登记 overlayStack（成对 push/pop，多次 show 不重复计数）。 */
let overlayPushed = false
/** 是否已挂视口监听（滚动/resize 收起浮层，review R-13）。 */
let viewportBound = false

/** 该线索的候选（响应按当前期 thread id 分组；无推荐/未加载/失败都给空数组）。 */
const items = computed<ThreadRelationItem[]>(() => {
  if (props.entry.status !== 'success') return []
  return props.entry.data?.threads?.[String(props.threadId)] ?? []
})
const visibleItems = computed(() => (expanded.value ? items.value : items.value.slice(0, MAX_CHIPS)))
const hiddenCount = computed(() => Math.max(0, items.value.length - MAX_CHIPS))
/**
 * 切档到未缓存的窗口时 cache 里是新 key（`{status:'loading'}` 无 data），
 * 此时保留该行并给骨架 chip（ui-design State Matrix：2 个骨架，行高固定不跳动）——
 * 否则 chip 行会在切档瞬间消失又出现，读起来像「这条线索没相关」。
 */
const loading = computed(() => props.entry.status === 'loading')
/**
 * 无候选 / 失败 / 非加载态 → 整行不渲染（任何档位都如此）。档位控件在分区工具条，
 * 不随本行消失（review R-3）：切档后即使本条线索没有候选，用户也能在分区工具条切回。
 */
const showRow = computed(() => loading.value || items.value.length > 0)
const tipId = computed(() => `thread-related-tip-${props.threadId}`)

function barWidth(score: number): string {
  // 分数是启发式相似度不是概率：条宽只做相对强弱提示，保留一位小数的百分比。
  return `${Math.round(Math.min(1, Math.max(0, score)) * 100)}%`
}

function periodDate(item: ThreadRelationItem): string {
  return (item.period_date || '').slice(0, 10)
}

function clearHideTimer() {
  if (hideTimer) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
}

/** 视口边界翻转：先放下方，放不下翻到上方；左右不出视口。 */
function placeTooltip(anchor: DOMRect) {
  const tip = tipRef.value?.getBoundingClientRect()
  const width = tip?.width ?? 0
  const height = tip?.height ?? 0
  const winW = typeof window !== 'undefined' ? window.innerWidth : 0
  const winH = typeof window !== 'undefined' ? window.innerHeight : 0

  let left = anchor.left
  let top = anchor.bottom + TIP_GAP
  if (width > 0 && left + width > winW - VIEWPORT_PAD) left = winW - width - VIEWPORT_PAD
  if (left < VIEWPORT_PAD) left = VIEWPORT_PAD
  if (height > 0 && top + height > winH - VIEWPORT_PAD) top = anchor.top - height - TIP_GAP
  if (top < VIEWPORT_PAD) top = VIEWPORT_PAD

  tipPosition.value = { left, top }
}

async function show(item: ThreadRelationItem, event: Event) {
  clearHideTimer()
  const anchor = (event.currentTarget as HTMLElement | null)?.getBoundingClientRect()
  active.value = item
  // 浮层一出现就登记浮层栈：阅读台的 Esc 闸门（hasOpenOverlay）据此先让位。
  if (!overlayPushed) {
    pushOverlay()
    overlayPushed = true
  }
  bindViewportGuards()
  await nextTick()
  // 浮层尺寸要挂载后才量得到（宽度依赖内容换行），所以定位放在 nextTick 之后。
  if (anchor && active.value === item) placeTooltip(anchor)
}

function scheduleHide() {
  clearHideTimer()
  hideTimer = setTimeout(hide, 120)
}

function hide() {
  clearHideTimer()
  active.value = null
  releaseViewportGuards()
  if (overlayPushed) {
    popOverlay()
    overlayPushed = false
  }
}

/** 滚动/resize 会让 fixed 浮层脱位（锚点已不在原位）：直接收起，不追着算（review R-13）。 */
function onViewportChange() {
  if (active.value) hide()
}

function bindViewportGuards() {
  if (viewportBound || typeof window === 'undefined') return
  // capture：滚动可能发生在任意祖先滚动容器（阅读台正文列），冒泡到 window 只覆盖页面级。
  window.addEventListener('scroll', onViewportChange, true)
  window.addEventListener('resize', onViewportChange)
  viewportBound = true
}

function releaseViewportGuards() {
  if (!viewportBound || typeof window === 'undefined') return
  window.removeEventListener('scroll', onViewportChange, true)
  window.removeEventListener('resize', onViewportChange)
  viewportBound = false
}

function onChipClick(item: ThreadRelationItem) {
  // 没有来源文章就没有可打开的阅读对象：不弹空弹窗、不报错（chip 仍是 focusable 供浮层阅读）。
  const articleId = item.source_article?.id
  if (articleId == null) return
  // 带相关度上下文（review R-1）：阅读预览弹窗要显示档位/分数/候选日期作阅读上下文。
  emit('openArticle', { articleId, score: item.score, level: item.level, periodDate: periodDate(item) })
}

function expand() {
  expanded.value = true
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && active.value) hide()
}

if (typeof window !== 'undefined') window.addEventListener('keydown', onKeydown)
onUnmounted(() => {
  clearHideTimer()
  releaseViewportGuards()
  if (overlayPushed) {
    popOverlay()
    overlayPushed = false
  }
  if (typeof window !== 'undefined') window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div
    v-if="showRow"
    class="tre"
    :data-testid="`thread-related-${threadId}`"
    :aria-busy="loading || undefined"
  >
    <template v-if="loading">
      <span class="tre__chip tre__chip--skeleton" aria-hidden="true" />
      <span class="tre__chip tre__chip--skeleton tre__chip--skeleton-short" aria-hidden="true" />
    </template>
    <template v-else>
      <button
        v-for="item in visibleItems"
        :key="item.thread_id"
        type="button"
        class="tre__chip"
        :class="{ 'tre__chip--confirmed': item.confirmed, 'tre__chip--static': item.source_article == null }"
        :title="item.title"
        :aria-label="item.confirmed ? `${item.title}（已确认关键事件）` : item.title"
        :aria-describedby="active === item ? tipId : undefined"
        :data-thread-id="item.thread_id"
        @mouseenter="show(item, $event)"
        @mouseleave="scheduleHide"
        @focus="show(item, $event)"
        @blur="hide"
        @click="onChipClick(item)"
      >
        <span v-if="item.confirmed" class="tre__mark" aria-hidden="true">★</span>
        <span class="tre__label">{{ item.title }}</span>
      </button>

      <button
        v-if="hiddenCount > 0 && !expanded"
        type="button"
        class="tre__chip tre__chip--more"
        :aria-expanded="expanded"
        :data-testid="`thread-related-more-${threadId}`"
        @click="expand"
      >
        还有 {{ hiddenCount }} 条
      </button>
    </template>

    <Teleport to="body">
      <div
        v-if="active"
        :id="tipId"
        ref="tipRef"
        class="tre-tip"
        role="tooltip"
        :style="{ left: `${tipPosition.left}px`, top: `${tipPosition.top}px`, zIndex: TIP_Z_INDEX }"
        @mouseenter="clearHideTimer"
        @mouseleave="scheduleHide"
      >
        <p class="tre-tip__title">{{ active.title }}</p>
        <p v-if="active.summary" class="tre-tip__summary">{{ active.summary }}</p>
        <p v-if="active.source_article" class="tre-tip__source">来源：{{ active.source_article.title }}</p>
        <div class="tre-tip__meta">
          <span class="tre-tip__level">{{ relationLevelLabel(active.level) }}</span>
          <span class="tre-tip__score">{{ formatRelationScore(active.score) }}</span>
        </div>
        <div class="tre-tip__bar" aria-hidden="true">
          <span :style="{ width: barWidth(active.score) }" />
        </div>
        <p class="tre-tip__date">{{ periodDate(active) }}</p>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
/* 整宽一行（宿主放在 .drm-thread-row 之后，不塞进它的两列 grid） */
.tre {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.5rem;
}

.tre__chip {
  display: inline-flex;
  align-items: center;
  gap: 0.28rem;
  max-width: 18rem;
  padding: 1px 6px;
  border: 1px solid var(--color-border-medium);
  border-radius: 2px;
  background: transparent;
  color: var(--color-text-secondary);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 11px;
  line-height: 1.5;
  text-align: left;
  cursor: pointer;
  transition: border-color 120ms ease, color 120ms ease;
}

/* 切档骨架：与 chip 同高，静态灰块（无循环脉冲，V5） */
.tre__chip--skeleton {
  width: 7.5rem;
  height: 1.35rem;
  border-color: transparent;
  background: var(--color-border-subtle);
  pointer-events: none;
}

.tre__chip--skeleton-short {
  width: 5rem;
}

.tre__chip:hover,
.tre__chip:focus-visible {
  border-color: var(--color-border-strong);
  color: var(--color-text-primary);
}

.tre__chip:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

/* ★ = 人工已确认的关键事件（真实状态，不是装饰）；描边与星号同取 accent */
.tre__chip--confirmed {
  border-color: var(--color-accent);
}

.tre__mark {
  color: var(--color-accent);
}

/* 无来源文章的候选不可点开阅读：不给手型，避免点空 */
.tre__chip--static {
  cursor: default;
}

.tre__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tre__chip--more {
  border-style: dashed;
  color: var(--color-text-secondary);
}

/* 预览浮层：fixed + 视口边界翻转（不被线索行 overflow 裁剪）；阴影只给脱离平面的浮层 */
.tre-tip {
  position: fixed;
  width: min(24rem, calc(100vw - 2rem));
  padding: 0.6rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 2px;
  background: var(--color-bg-elevated);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.16);
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  line-height: 1.6;
  transition: opacity 120ms ease;
}

.tre-tip__title {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.82rem;
  font-weight: 700;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.tre-tip__summary {
  display: -webkit-box;
  margin: 0.3rem 0 0;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.tre-tip__source {
  margin: 0.35rem 0 0;
  overflow: hidden;
  color: var(--color-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tre-tip__meta {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  margin-top: 0.4rem;
  padding-top: 0.35rem;
  border-top: 1px solid var(--color-border-subtle);
}

.tre-tip__level {
  color: var(--color-text-primary);
  font-weight: 600;
}

.tre-tip__score {
  color: var(--color-text-secondary);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.68rem;
  font-variant-numeric: tabular-nums;
}

.tre-tip__bar {
  height: 2px;
  margin-top: 0.3rem;
  background: var(--color-border-subtle);
}

.tre-tip__bar > span {
  display: block;
  height: 100%;
  background: var(--color-accent);
}

.tre-tip__date {
  margin: 0.3rem 0 0;
  color: var(--color-text-secondary);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
}

@media (max-width: 767.98px) {
  /* 窄屏：chip 更早截断（不横向溢出） */
  .tre__chip {
    max-width: 11rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .tre__chip,
  .tre-tip {
    transition: none;
  }
}
</style>
