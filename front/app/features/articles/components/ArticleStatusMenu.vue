<script lang="ts">
import type { Article } from '~/types'

/** 工具栏状态图标 + 处理详情浮层 + ⋯ 更多菜单的 props（design D3，数据从 ArticleContentView 直取） */
export interface ArticleStatusMenuProps {
  /** 合并实时处理状态的文章（mergedArticle） */
  article: Article | null
  /** feed 开启 Firecrawl 时出现「手动抓取全文」 */
  showManualFirecrawlAction: boolean
  /** feed 开启 AI 总结时出现「生成 AI 总结」 */
  showManualSummaryAction: boolean
  /** AI 能力开启时出现「手动打标签」 */
  showManualTaggingAction: boolean
  /** 任一抓取/总结操作进行中（同一时刻至多一个手动操作） */
  actionBusy: boolean
  manualFirecrawlLoading: boolean
  manualSummaryLoading: boolean
  manualTaggingLoading: boolean
  manualFirecrawlLabel: string
  manualSummaryLabel: string
  manualTaggingLabel: string
  /** 手动抓取/总结的行内错误（不再占正文上方横幅） */
  manualActionError: string | null
  /** 手动打标签的行内错误 */
  taggingError: string | null
  /** 抓取/总结时间与尝试次数明细（原 detailLines） */
  detailLines: string[]
  /** 双源可用时出现内容源切换分段控件 */
  showContentSourceToggle: boolean
  activeContentSource: string | null
}
</script>

<script setup lang="ts">
import { Icon } from '@iconify/vue'
import {
  getArticlePipelineState,
  getFirecrawlStatusMeta,
  getPipelineStateMeta,
  getSummaryStatusMeta,
  type StatusTone,
} from '~/features/articles/composables/useArticleProcessingStatus'

const props = withDefaults(defineProps<ArticleStatusMenuProps>(), {
  article: null,
  showManualFirecrawlAction: false,
  showManualSummaryAction: false,
  showManualTaggingAction: false,
  actionBusy: false,
  manualFirecrawlLoading: false,
  manualSummaryLoading: false,
  manualTaggingLoading: false,
  manualFirecrawlLabel: '手动抓取全文',
  manualSummaryLabel: '手动生成总结',
  manualTaggingLabel: '手动打标签',
  manualActionError: null,
  taggingError: null,
  detailLines: () => [],
  showContentSourceToggle: false,
  activeContentSource: null,
})

const emit = defineEmits<{
  'manual-firecrawl': []
  'manual-summary': []
  'manual-tagging': []
  'update:activeContentSource': [source: string]
}>()

// ---- 状态图标四态（语义与 reading-list-panel 一致：琥珀时钟/info 旋转/error 警示/淡灰完成） ----
const pipelineState = computed(() => (props.article ? getArticlePipelineState(props.article) : 'done'))
const stateMeta = computed(() => getPipelineStateMeta(pipelineState.value))

const hasStatusSignal = computed(() => {
  if (!props.article) return false
  return Boolean(
    props.showManualFirecrawlAction
    || props.showManualSummaryAction
    || props.detailLines.length > 0
    || (props.article.firecrawlStatus && props.article.firecrawlStatus !== 'pending')
    || (props.article.summaryStatus && props.article.summaryStatus !== 'incomplete'),
  )
})

const stateColor = computed(() => {
  switch (stateMeta.value.colorToken) {
    case 'warning': return 'var(--color-warning)'
    case 'info': return 'var(--color-info)'
    case 'error': return 'var(--color-error)'
    default: return 'var(--color-text-muted)'
  }
})

// ---- 处理详情浮层：抓取/总结/标签三行 + 明细 + 失败错误文案（超长滚动） ----
interface DetailRow {
  label: string
  value: string
  tone: StatusTone
}

const detailRows = computed<DetailRow[]>(() => {
  if (!props.article) return []
  const firecrawl = getFirecrawlStatusMeta(props.article)
  const summary = getSummaryStatusMeta(props.article)
  return [
    { label: '全文抓取', value: firecrawl.label, tone: firecrawl.tone },
    { label: 'AI 总结', value: summary.label, tone: summary.tone },
    {
      label: '标签',
      value: props.article.tagCount ? `已标记 ${props.article.tagCount}` : '待打标签',
      tone: props.article.tagCount ? 'success' : 'neutral',
    },
  ]
})

const detailError = computed(() => {
  if (!props.article) return ''
  return props.article.completionError || props.article.firecrawlError || ''
})

const detailTitle = computed(() => {
  if (pipelineState.value !== 'failed' || !props.article) return '处理详情'
  return props.article.firecrawlError ? '抓取失败' : '总结失败'
})

function toneColor(tone: StatusTone): string {
  switch (tone) {
    case 'success': return 'var(--color-success)'
    case 'danger': return 'var(--color-error)'
    case 'warning': return 'var(--color-warning)'
    case 'info': return 'var(--color-info)'
    default: return 'var(--color-text-secondary)'
  }
}

// ---- 浮层开合：覆盖式定位，点击外部/再点/Esc 关闭，同一时刻至多一个浮层 ----
const statusOpen = ref(false)
const menuOpen = ref(false)
const rootRef = ref<HTMLElement | null>(null)

const hasMenuContent = computed(() =>
  props.showManualFirecrawlAction
  || props.showManualSummaryAction
  || props.showManualTaggingAction
  || props.showContentSourceToggle,
)

function toggleStatus() {
  statusOpen.value = !statusOpen.value
  if (statusOpen.value) menuOpen.value = false
}

function toggleMenu() {
  menuOpen.value = !menuOpen.value
  if (menuOpen.value) statusOpen.value = false
}

function closeAllPopovers() {
  statusOpen.value = false
  menuOpen.value = false
}

function onDocMouseDown(event: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(event.target as Node)) closeAllPopovers()
}

function onDocKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeAllPopovers()
}

watch(
  () => statusOpen.value || menuOpen.value,
  (open) => {
    if (open) {
      document.addEventListener('mousedown', onDocMouseDown)
      document.addEventListener('keydown', onDocKeyDown)
    } else {
      document.removeEventListener('mousedown', onDocMouseDown)
      document.removeEventListener('keydown', onDocKeyDown)
    }
  },
)

// 切换文章时收起浮层，避免详情串文
watch(() => props.article?.id, () => closeAllPopovers())

onUnmounted(() => {
  document.removeEventListener('mousedown', onDocMouseDown)
  document.removeEventListener('keydown', onDocKeyDown)
})
</script>

<template>
  <div ref="rootRef" class="status-menu">
    <!-- 处理状态图标（四态） + 处理详情浮层 -->
    <button
      v-if="hasStatusSignal"
      class="action-btn status-btn"
      :class="{ 'status-btn-active': statusOpen }"
      :title="stateMeta.title"
      :aria-label="stateMeta.title"
      :aria-expanded="statusOpen"
      @click="toggleStatus"
    >
      <Icon
        :icon="stateMeta.icon"
        width="18"
        height="18"
        :class="{ 'animate-spin': stateMeta.spinning }"
        :style="{ color: stateColor }"
      />
    </button>

    <div v-if="statusOpen" class="asm-pop asm-status-pop" role="dialog" aria-label="处理详情">
      <h4 class="asm-pop-title" :class="{ 'asm-pop-title-error': pipelineState === 'failed' }">
        <Icon :icon="pipelineState === 'failed' ? 'mdi:alert-circle' : 'mdi:information-slab-circle'" width="13" height="13" />
        <span>{{ detailTitle }}</span>
      </h4>
      <div v-for="row in detailRows" :key="row.label" class="asm-row">
        <span>{{ row.label }}</span>
        <span class="asm-row-value" :style="{ color: toneColor(row.tone) }">{{ row.value }}</span>
      </div>
      <div v-if="detailLines.length" class="asm-meta">
        <div v-for="line in detailLines" :key="line">{{ line }}</div>
      </div>
      <div v-if="detailError" class="asm-error">{{ detailError }}</div>
    </div>

    <!-- ⋯ 更多操作菜单 -->
    <button
      v-if="hasMenuContent"
      class="action-btn"
      :class="{ active: menuOpen }"
      title="更多操作"
      aria-label="更多操作"
      :aria-expanded="menuOpen"
      @click="toggleMenu"
    >
      <Icon icon="mdi:dots-vertical" width="20" height="20" />
    </button>

    <div v-if="menuOpen" class="asm-pop asm-menu-pop" role="menu" aria-label="更多操作">
      <div class="asm-menu-label">手动操作</div>
      <button
        v-if="showManualFirecrawlAction"
        class="asm-menu-item"
        role="menuitem"
        :disabled="actionBusy"
        @click="emit('manual-firecrawl')"
      >
        <Icon icon="mdi:web-sync" width="15" height="15" :class="{ 'animate-spin': manualFirecrawlLoading }" />
        <span>{{ manualFirecrawlLabel }}</span>
      </button>
      <button
        v-if="showManualSummaryAction"
        class="asm-menu-item"
        role="menuitem"
        :disabled="actionBusy"
        @click="emit('manual-summary')"
      >
        <Icon icon="mdi:brain" width="15" height="15" :class="{ 'animate-spin': manualSummaryLoading }" />
        <span>{{ manualSummaryLabel }}</span>
      </button>
      <button
        v-if="showManualTaggingAction"
        class="asm-menu-item"
        role="menuitem"
        :disabled="actionBusy || manualTaggingLoading"
        @click="emit('manual-tagging')"
      >
        <Icon icon="mdi:tag-plus-outline" width="15" height="15" :class="{ 'animate-spin': manualTaggingLoading }" />
        <span>{{ manualTaggingLabel }}</span>
      </button>

      <!-- 手动操作行内错误反馈（spec：不恢复正文上方错误横幅） -->
      <div v-if="manualActionError" class="asm-menu-error">{{ manualActionError }}</div>
      <div v-if="taggingError" class="asm-menu-error">{{ taggingError }}</div>

      <template v-if="showContentSourceToggle">
        <div class="asm-menu-sep" />
        <div class="asm-menu-source">
          <span>内容源</span>
          <div class="asm-seg" role="group" aria-label="内容源切换">
            <button
              :class="{ 'asm-seg-on': activeContentSource === 'original' }"
              @click="emit('update:activeContentSource', 'original')"
            >
              原始内容
            </button>
            <button
              :class="{ 'asm-seg-on': activeContentSource === 'firecrawl' }"
              @click="emit('update:activeContentSource', 'firecrawl')"
            >
              Firecrawl 全文
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.status-menu {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 0.1rem;
}

.status-btn-active {
  color: var(--color-text-primary);
}

/* 浮层：覆盖式定位（锚定工具栏，不挤动阅读流） */
.asm-pop {
  position: absolute;
  top: calc(100% + 8px);
  z-index: 30;
  border: 1px solid var(--color-border-subtle);
  border-radius: 0.625rem;
  background: var(--color-bg-hover);
  box-shadow: 0 10px 32px rgba(26, 26, 26, 0.13);
}

.asm-status-pop {
  left: 0;
  width: 272px;
  padding: 12px 14px;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
}

.asm-pop-title {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 8px;
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  color: var(--color-text-muted);
}

.asm-pop-title-error {
  color: var(--color-error);
}

.asm-row {
  display: flex;
  justify-content: space-between;
  padding: 3px 0;
}

.asm-row-value {
  font-weight: 600;
}

.asm-meta {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--color-border-subtle);
  font-size: 0.6875rem;
  color: var(--color-text-muted);
}

.asm-error {
  margin-top: 8px;
  max-height: 120px;
  overflow-y: auto;
  padding: 6px 8px;
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--color-error) 8%, transparent);
  color: var(--color-error);
  font-size: 0.6875rem;
  line-height: 1.5;
  word-break: break-word;
}

.asm-menu-pop {
  right: 0;
  min-width: 208px;
  /* 宽度随内容自适应：内容源分段控件（原始内容/Firecrawl 全文）不得溢出被裁 */
  width: max-content;
  max-width: min(340px, calc(100vw - 2rem));
  padding: 6px;
}

.asm-menu-label {
  padding: 6px 10px 4px;
  font-size: 0.625rem;
  letter-spacing: 0.1em;
  color: var(--color-text-muted);
}

.asm-menu-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: 0.4375rem;
  background: transparent;
  cursor: pointer;
  text-align: left;
  font-size: 0.8125rem;
  color: var(--color-text-primary);
}

.asm-menu-item:hover:not(:disabled) {
  background: var(--color-bg-active);
}

.asm-menu-item:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.asm-menu-item svg {
  flex-shrink: 0;
  color: var(--color-text-secondary);
}

.asm-menu-error {
  margin: 4px 8px;
  max-height: 96px;
  overflow-y: auto;
  padding: 6px 8px;
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--color-error) 8%, transparent);
  color: var(--color-error);
  font-size: 0.6875rem;
  line-height: 1.5;
  word-break: break-word;
}

.asm-menu-sep {
  height: 1px;
  margin: 5px 8px;
  background: var(--color-border-subtle);
}

.asm-menu-source {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 7px 10px;
  font-size: 0.8125rem;
  color: var(--color-text-primary);
  flex-wrap: nowrap;
}

.asm-menu-source > span {
  white-space: nowrap;
  flex-shrink: 0;
}

.asm-seg {
  display: inline-flex;
  border: 1px solid var(--color-border-subtle);
  border-radius: 0.4375rem;
  overflow: hidden;
}

.asm-seg button {
  padding: 3px 9px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  font-size: 0.6875rem;
  white-space: nowrap;
}

.asm-seg button.asm-seg-on {
  background: var(--color-bg-sunken);
  color: var(--color-text-primary);
  font-weight: 600;
}
</style>
