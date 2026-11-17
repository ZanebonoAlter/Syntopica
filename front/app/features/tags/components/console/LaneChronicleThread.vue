<script setup lang="ts">
/**
 * LaneChronicleThread — 编年单条线索（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 标题默认收束按钮；展开 = 该 thread 真实 summary 逐字 + 次级区（置信/关联篇数，
 * interaction-conventions：分数只进探究区，不堆在标题行）+ 相关文章按需块
 * （LaneRelatedArticles）。同名不同 threadId 各自独立渲染（父按 threadId keyed）。
 *
 * 综合稿标记（fix-lane-metric-provenance）：`scope='composite'` 的线索（event
 * 标签数 ≥ 2）带只读徽标「综合稿」——它不计入当日线索合计，徽标文案不得暗示
 * 它被计入。
 */
import { Icon } from '@iconify/vue'
import type { LaneDayThread } from '~/api/normalizers/laneDayReading'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import LaneRelatedArticles from './LaneRelatedArticles.vue'

defineProps<{
  thread: LaneDayThread
  expanded: boolean
  /** 读取 store（相关文章按需；由 LaneChronicle 统一持有）。 */
  reading: LaneDayReadingStore
}>()

const emit = defineEmits<{
  toggle: [threadId: string]
  openArticle: [articleId: string]
}>()

/** 综合稿（多事件稿）：不计入线索合计，徽标只作身份标记。 */
const isComposite = (thread: { scope?: string | null }): boolean => thread.scope === 'composite'
</script>

<template>
  <div class="lct" :data-testid="`lane-chronicle-thread-${thread.threadId}`">
    <button
      type="button"
      class="lct-head"
      :aria-expanded="expanded"
      :data-testid="`lane-chronicle-thread-toggle-${thread.threadId}`"
      @click="emit('toggle', thread.threadId)"
    >
      <Icon :icon="expanded ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="13" class="lct-caret" />
      <span class="lct-title" :title="thread.title">{{ thread.title || '(无标题线索)' }}</span>
      <span
        v-if="isComposite(thread)"
        class="lct-composite"
        data-testid="lane-chronicle-composite-tag"
        title="综合稿（多个事件的同一篇稿）：不计入当日线索合计"
      >综合稿</span>
    </button>
    <div v-if="expanded" class="lct-body">
      <p v-if="thread.summary" class="lct-summary">{{ thread.summary }}</p>
      <p v-else class="lct-summary lct-summary--empty">暂无摘要记录</p>
      <p class="lct-meta">
        <span>置信 {{ thread.confidence.toFixed(2) }}</span>
        <span aria-hidden="true">·</span>
        <span>关联 {{ thread.relatedArticleIds.length }} 篇</span>
      </p>
      <LaneRelatedArticles
        :reading="reading"
        :article-ids="thread.relatedArticleIds"
        :thread-id="thread.threadId"
        @open-article="id => emit('openArticle', id)"
      />
    </div>
  </div>
</template>

<style scoped>
.lct {
  border-left: 2px solid var(--color-border-subtle);
  padding-left: 0.55rem;
}

.lct-head {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  width: 100%;
  border: none;
  background: none;
  padding: 0.18rem 0;
  text-align: left;
  cursor: pointer;
  color: var(--color-text-primary);
}

.lct-caret {
  flex: 0 0 auto;
  color: var(--color-text-muted);
  transform: translateY(2px);
}

.lct-title {
  font-size: 0.84rem;
  line-height: 1.55;
  overflow-wrap: break-word;
}

/* 综合稿徽标：只读身份标记（不暗示计入线索合计），复用泳道工作面「首义」徽标那一档。 */
.lct-composite {
  flex: none;
  font-size: 10px;
  color: var(--color-text-muted);
  border: 1px dashed var(--color-border-strong);
  border-radius: 3px;
  padding: 0 4px;
  white-space: nowrap;
}

.lct-body {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  padding: 0.35rem 0 0.45rem 1.05rem;
}

.lct-summary {
  margin: 0;
  font-size: 0.8rem;
  line-height: 1.75;
  color: var(--color-text-secondary);
  max-width: 44rem;
  overflow-wrap: break-word;
  white-space: pre-line;
}

.lct-summary--empty {
  color: var(--color-text-muted);
  font-style: italic;
}

.lct-meta {
  margin: 0;
  display: flex;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
