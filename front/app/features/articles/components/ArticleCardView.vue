<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { Article, RssFeed, SearchHitReason } from '~/types'
import FeedIcon from '~/components/feed/FeedIcon.vue'
import { proxiedImageUrl } from '~/utils/imageProxy'
import { AI_SUSPECT_LABEL, isAiSuspect } from '~/utils/aiDetection'
import RowStatusPopover from './RowStatusPopover.vue'
import {
  getArticlePipelineState,
  getFirecrawlStatusMeta,
  getPipelineStateMeta,
  getSummaryStatusMeta,
  type PipelineLine,
} from '~/features/articles/composables/useArticleProcessingStatus'

import '~/components/article/ArticleCard.css'

interface Props {
  article: Article
  compact?: boolean
  selected?: boolean
  /** 单 feed 视图传 false：行内省略来源名（头部已示） */
  showFeedTitle?: boolean
  /** 搜索命中解释（add-article-search）：传入时 meta 行渲染三型命中 chip；缺省零渲染，列表行为不变 */
  hitReasons?: SearchHitReason[]
  /** 搜索结果归档标记（add-article-search）：true 时标题尾渲染灰描边角标并降饱和；缺省不渲染 */
  archived?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  compact: false,
  selected: false,
  showFeedTitle: true,
  hitReasons: () => [],
  archived: false,
})

const emit = defineEmits<{
  click: [article: Article]
  favorite: [id: string]
}>()

const feedsStore = useFeedsStore()

const feed = computed(() => feedsStore.feeds.find((f: RssFeed) => f.id === props.article.feedId))

// v5 封面槽：image_url 非空渲染封面；为空或加载失败降级 feed 图标占位（行高不变）
const coverSrc = computed(() => proxiedImageUrl(props.article.imageUrl || ''))
const coverFailed = ref(false)
watch(
  () => props.article.imageUrl,
  () => {
    coverFailed.value = false
  },
)

const pipelineState = computed(() => getArticlePipelineState(props.article))
const stateMeta = computed(() => getPipelineStateMeta(pipelineState.value))
// 疑似 AI 生成标记（add-ai-generated-article-detection）：仅 suspect 渲染，
// 未判定 / 已确认非 AI 一律不渲染、不占位（行高与虚拟列表假设不变）
const showSuspectBadge = computed(() => isAiSuspect(props.article.aiDetectionStatus))
const popoverOpen = ref(false)
const rootRef = ref<HTMLElement | null>(null)

// 浮层互斥与点外关闭：本行浮层打开时，点击浮层/本行状态图标之外（含其他行）即关闭（spec 处理详情浮层）
function onDocMouseDown(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (rootRef.value?.contains(target)) {
    if (target.closest('.row-status-popover')) return
    if (target.closest('.row-state-icon, .row-state-spin')) return
    closePopover()
    return
  }
  closePopover()
}

watch(popoverOpen, (open) => {
  if (open) {
    document.addEventListener('mousedown', onDocMouseDown)
  } else {
    document.removeEventListener('mousedown', onDocMouseDown)
  }
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onDocMouseDown)
})

const popoverTitle = computed(() => {
  if (pipelineState.value === 'failed') {
    return props.article.firecrawlError ? '抓取失败' : '总结失败'
  }
  return '处理详情'
})
const popoverError = computed(() => {
  if (pipelineState.value !== 'failed') return ''
  return props.article.completionError || props.article.firecrawlError || '处理失败，原因未知'
})
const popoverHint = computed(() => {
  switch (pipelineState.value) {
    case 'queued':
      return '排队中：等待抓取/总结链路处理'
    case 'processing':
      return '完成后自动进入后续链路'
    case 'failed':
      return '稍后将随下次刷新自动重试'
    default:
      return props.article.tagCount ? '标签详情可在「叙事工坊 / 语义版块」按本文查看' : ''
  }
})
const popoverLines = computed<PipelineLine[]>(() => [
  { label: '抓取', value: getFirecrawlStatusMeta(props.article).label, tone: getFirecrawlStatusMeta(props.article).tone },
  { label: '总结', value: getSummaryStatusMeta(props.article).label, tone: getSummaryStatusMeta(props.article).tone },
  { label: '标签', value: props.article.tagCount ? `已标记 ${props.article.tagCount}` : '待打标签', tone: props.article.tagCount ? 'success' : 'neutral' },
])

function togglePopover() {
  popoverOpen.value = !popoverOpen.value
}
function closePopover() {
  popoverOpen.value = false
}

// 命中 chip 文案：关键词「命中词」/ 语义「标签名 + cosine」/ 扩展「扩展词」（原型 hit-chip 三型）
function hitChipLabel(reason: SearchHitReason): string {
  if (reason.type === 'semantic') return `标签「${reason.text}」`
  if (reason.type === 'expand') return `扩展词「${reason.text}」`
  return `命中「${reason.text}」`
}
</script>

<template>
  <article
    ref="rootRef"
    class="article-row group"
    :class="{ 'article-row-read': article.read, selected, 'article-row-archived': archived }"
    @click="emit('click', article)"
  >
    <div class="row-cover">
      <img
        v-if="coverSrc && !coverFailed"
        :src="coverSrc"
        alt=""
        loading="lazy"
        @error="coverFailed = true"
      />
      <FeedIcon v-else :icon="feed?.icon" :feed-id="feed?.id" :color="feed?.color" :size="26" />
      <!-- 疑似 AI 角标压封面右上角（2026-11-25 修订 C）：不进 .row-title 的两行 clamp 盒
           （R-5 被整块裁掉），也不占 .row-meta 的 flex 宽度（R-13 后实测 meta 内容宽恒 165px，
           时间 49 + 状态图标 19 + 全称角标 77 + gap 24 = 169 → 角标右侧被裁 4px、来源名被压到 0 宽）。
           短标「AI」+ 实底保证可读与醒目，全称走 title / aria-label；未判定不渲染、行高由固定 100px 兜住 -->
      <span
        v-if="showSuspectBadge"
        class="suspect-badge"
        role="img"
        :title="AI_SUSPECT_LABEL"
        :aria-label="AI_SUSPECT_LABEL"
      >AI</span>
    </div>
    <div class="row-main">
      <div class="row-heading">
        <h3 class="row-title">{{ article.title }}<span v-if="archived" class="archived-badge">已归档</span></h3>
      </div>

      <div class="row-meta">
        <span class="row-time">{{ $dayjs(article.pubDate).fromNow() }}</span>
        <span v-if="showFeedTitle && feed" class="row-identity">{{ feed.title }}</span>
        <span v-else-if="article.author" class="row-identity">{{ article.author }}</span>
        <span
          v-for="(reason, i) in hitReasons"
          :key="`${reason.type}-${i}`"
          class="hit-chip"
          :class="`hit-${reason.type}`"
        >
          <span aria-hidden="true">{{ reason.type === 'keyword' ? '🔍' : reason.type === 'semantic' ? '🏷' : '✨' }}</span>
          {{ hitChipLabel(reason) }}
          <b v-if="reason.type === 'semantic' && reason.score != null" class="hit-chip-score">{{ reason.score.toFixed(2) }}</b>
        </span>
        <span class="row-meta-grow"></span>
        <span
          v-if="stateMeta.spinning"
          class="row-state-spin"
          role="status"
          :title="stateMeta.title"
          @click.stop="togglePopover"
        ></span>
        <button
          v-else
          class="row-state-icon"
          :class="`row-state-${pipelineState}`"
          :title="stateMeta.title"
          :aria-label="stateMeta.title"
          @click.stop="togglePopover"
        >
          <Icon :icon="stateMeta.icon" width="15" height="15" />
        </button>
      </div>
    </div>

    <button
      class="row-favorite"
      :class="{ active: article.favorite }"
      :aria-label="article.favorite ? '取消收藏' : '收藏'"
      @click.stop="emit('favorite', article.id)"
    >
      <Icon :icon="article.favorite ? 'mdi:star' : 'mdi:star-outline'" width="17" height="17" />
    </button>

    <RowStatusPopover
      v-if="popoverOpen"
      :title="popoverTitle"
      :title-tone="pipelineState === 'failed' ? 'error' : 'neutral'"
      :lines="popoverLines"
      :error="popoverError"
      :hint="popoverHint"
      @close="closePopover"
    />
  </article>
</template>
