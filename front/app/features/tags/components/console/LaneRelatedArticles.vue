<script setup lang="ts">
/**
 * LaneRelatedArticles — 线索相关文章（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 按需展开：首次点开才按真实 related_article_ids 读取文章详情（元信息：标题/时间；
 * /articles/:id 不带来源名，不造）。展示总数始终 = articleIds.length——单项失败不改
 * 总数、不说「无文章」，失败项局部重试不阻塞其他条目；文章主点击 emit 给 G5 站内
 * 读原文（不加「先预览导语再点正文」中间层）。加载标题不标已读、不动 reading_behaviors。
 *
 * `showToggle=false`（key-event-day-focus-cards 日聚焦卡片）：调用方自带入口驱动
 * 展开 → 本组件不渲染内部开关，列表随挂载展开、由调用方挂载/卸载开合（避免双开关）。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'

const props = withDefaults(defineProps<{
  reading: LaneDayReadingStore
  articleIds: string[]
  threadId: string
  /**
   * 内部「相关文章 · N 篇」开关是否渲染（默认 `true` = 保留既有行为，既有消费方
   * `LaneChronicleThread.vue` 零变化）。
   *
   * `false` = 调用方自带入口（key-event-day-focus-cards 的日聚焦卡片用「关联文章
   * N 篇 ›」文字链）：本组件**不渲染内部开关**，列表在挂载期间恒展开，开合由调用方
   * 挂载 / 卸载控制——避免双开关与两套文案（design D7）。
   */
  showToggle?: boolean
}>(), { showToggle: true })

const emit = defineEmits<{
  openArticle: [articleId: string]
}>()

/** 列表是否展开：默认由内部开关控制；`showToggle=false` 时随挂载即展开。 */
const visible = ref(props.showToggle === false)

watch(() => props.threadId, () => {
  // 外部驱动模式（showToggle=false）不由 threadId 复位：开合权在调用方手里。
  if (props.showToggle) visible.value = false
})

onMounted(() => {
  // 外部驱动模式：列表随挂载展开 → 按需读文章元信息（同内部开关展开时的行为）。
  if (visible.value) void props.reading.ensureRelatedArticles(props.articleIds)
})

/** 逐条目视图模型（同一局部变量侚 v-if 窄化，避免重复谓词调用）。 */
const entries = computed(() => props.articleIds.map(articleId => ({
  articleId,
  entry: props.reading.readRelated(articleId),
})))

function toggle(): void {
  visible.value = !visible.value
  if (visible.value) void props.reading.ensureRelatedArticles(props.articleIds)
}

function retry(articleId: string): void {
  void props.reading.retryRelatedArticle(articleId)
}

function shortDate(pubDate: string): string {
  return pubDate ? pubDate.slice(0, 10) : ''
}
</script>

<template>
  <div v-if="articleIds.length === 0" class="lra-empty">相关文章列表为空</div>
  <div v-else class="lra" :data-testid="`lane-chronicle-related-${threadId}`">
    <button
      v-if="showToggle"
      type="button" class="lra-toggle" :aria-expanded="visible"
      :data-testid="`lane-chronicle-related-toggle-${threadId}`" @click="toggle"
    >
      <Icon :icon="visible ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="13" />
      相关文章 · {{ articleIds.length }} 篇
    </button>
    <ul v-if="visible" class="lra-list">
      <li v-for="{ articleId, entry } in entries" :key="articleId" class="lra-item">
        <button
          v-if="entry.status === 'success'"
          type="button" class="lra-article"
          :data-testid="`lane-chronicle-article-${articleId}`"
          :title="entry.meta.title"
          @click="emit('openArticle', articleId)"
        >
          <span class="lra-article-title">{{ entry.meta.title }}</span>
          <span class="lra-article-date">{{ shortDate(entry.meta.pubDate) }}</span>
        </button>
        <div v-else-if="entry.status === 'error'" class="lra-error">
          <span>文章 #{{ articleId }} 加载失败（{{ entry.error }}）</span>
          <button
            type="button" class="lra-retry"
            :data-testid="`lane-chronicle-article-retry-${articleId}`"
            @click="retry(articleId)"
          >重试</button>
        </div>
        <div v-else class="lra-loading" :data-testid="`lane-chronicle-article-loading-${articleId}`">加载中…</div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.lra-empty {
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.lra {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.lra-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  align-self: flex-start;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  padding: 0.16rem 0.45rem;
  cursor: pointer;
}

.lra-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.lra-item {
  display: flex;
}

.lra-article {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  padding: 0.14rem 0;
  text-align: left;
  cursor: pointer;
}

.lra-article-title {
  font-size: 0.76rem;
  line-height: 1.55;
  color: var(--color-text-secondary);
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, currentColor 35%, transparent);
  text-underline-offset: 3px;
  overflow-wrap: break-word;
}

.lra-article:hover .lra-article-title {
  color: var(--color-accent);
}

.lra-article-date {
  flex: 0 0 auto;
  font-size: 0.64rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lra-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  overflow-wrap: break-word;
}

.lra-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.05rem 0.35rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.lra-loading {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}
</style>
