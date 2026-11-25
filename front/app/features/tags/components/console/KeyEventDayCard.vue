<script setup lang="ts">
/**
 * 日聚焦卡片（key-event-day-focus-cards 3.1 / design D7）。
 *
 * 泳道行内的一张小卡：首行「MM/DD 周X · 所属话题」→ 标题（**完整可读、可折行、
 * MUST NOT 省略号截断**）→ 摘要 1 行节选（点开就地展开全文）→ 「关联文章 N 篇 ›」
 * 文字链（点开渲染真实文章列表，展开后文案切「收起文章」——ui-design Interaction
 * Contract「按钮变『收起文章』」）。单卡高约 116px（紧凑形态，用户 2026-11-23 定）。
 *
 * 只读：卡片内不出现注记写入口（确认 / 取消 / 编辑一律没有）、不出现 suggested /
 * dismissed，也不出现教学语。
 *
 * 展开态**逐卡独立**（不做全局单开手风琴）；两处展开控件都是 `<button>` +
 * `aria-expanded`（与 `LaneRelatedArticles.vue` 的 `.lra-toggle` 同口径）——键盘
 * 可达性由本卡承接。
 *
 * 关联文章走既有 `LaneRelatedArticles.vue` 与既有 `reading` store（`laneDayRelatedArticles`
 * 缓存：≤20 FIFO / 在途合乘 / 晚到隔离），**不新建第二份阅读器与第二份缓存**；点文章
 * `emit('openArticle', id)` → 壳层既有 `ArticlePreviewModal` 打开全文。
 */
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import LaneRelatedArticles from './LaneRelatedArticles.vue'

const props = defineProps<{
  /** 事件（确认时快照：标题 / 话题 / 摘要 / 关联文章）。 */
  event: BoardKeyEventItem
  /** 壳层共享读取实例（文章元信息按需读；不自建缓存）。 */
  reading: LaneDayReadingStore
}>()

const emit = defineEmits<{
  openArticle: [articleId: string]
}>()

/** 摘要是否就地展开全文（逐卡独立）。 */
const summaryExpanded = ref(false)

/** 关联文章列表是否展开（逐卡独立）。 */
const articlesExpanded = ref(false)

/** 周几（date-only → 中文周几；按 UTC 解析避免时区偏移）。 */
const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

/** 首行日期标签：「MM/DD 周X」（无日期时为空）。 */
const dateLabel = computed(() => {
  const date = props.event.sourcePeriodDate
  if (!date) return ''
  const parsed = new Date(`${date}T00:00:00Z`)
  const weekday = Number.isNaN(parsed.getTime()) ? '' : (WEEKDAYS[parsed.getUTCDay()] ?? '')
  const md = date.slice(5).replace('-', '/')
  return weekday ? `${md} ${weekday}` : md
})

/** 所属话题（快照；空则不渲染该段）。 */
const topicLabel = computed(() => props.event.topicLabelSnapshot.trim())

function toggleSummary(): void {
  summaryExpanded.value = !summaryExpanded.value
}

function toggleArticles(): void {
  articlesExpanded.value = !articlesExpanded.value
}
</script>

<template>
  <article class="kdc" :data-testid="`key-event-day-card-${event.id}`">
    <p class="kdc-meta">
      <span class="kdc-date" :data-testid="`key-event-day-card-date-${event.id}`">{{ dateLabel }}</span>
      <template v-if="topicLabel">
        <span class="kdc-sep" aria-hidden="true">·</span>
        <span class="kdc-topic" :data-testid="`key-event-day-card-topic-${event.id}`">{{ topicLabel }}</span>
      </template>
    </p>

    <!-- 标题：完整显示、可折行，MUST NOT 省略号截断（delta spec「卡片标题完整不截断」） -->
    <h4 class="kdc-title" :data-testid="`key-event-day-card-title-${event.id}`">{{ event.titleSnapshot }}</h4>

    <!-- 摘要：1 行节选 → 点开就地展开全文（再点收起） -->
    <button
      v-if="event.summarySnapshot"
      type="button"
      class="kdc-summary-btn"
      :class="{ 'is-expanded': summaryExpanded }"
      :aria-expanded="summaryExpanded"
      :data-testid="`key-event-day-card-summary-${event.id}`"
      @click="toggleSummary"
    >
      <span class="kdc-summary">{{ event.summarySnapshot }}</span>
    </button>

    <!-- 关联文章：无文章不显示入口（MUST NOT 空列表占位） -->
    <button
      v-if="event.articleIds.length > 0"
      type="button"
      class="kdc-articles-btn"
      :aria-expanded="articlesExpanded"
      :aria-label="articlesExpanded ? `收起文章（${event.articleIds.length} 篇）` : `关联文章 ${event.articleIds.length} 篇`"
      :data-testid="`key-event-day-card-articles-${event.id}`"
      @click="toggleArticles"
    >
      <span>{{ articlesExpanded ? '收起文章' : `关联文章 ${event.articleIds.length} 篇` }}</span>
      <Icon :icon="articlesExpanded ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="13" />
    </button>

    <div v-if="articlesExpanded" class="kdc-articles">
      <!-- 复用既有文章列表组件与同一份 reading store；showToggle=false：卡片自带入口，
           不出现第二个内部开关（design D7）。 -->
      <LaneRelatedArticles
        :reading="reading"
        :article-ids="event.articleIds"
        :thread-id="`key-event-card-${event.id}`"
        :show-toggle="false"
        @open-article="id => emit('openArticle', id)"
      />
    </div>
  </article>
</template>

<style scoped>
.kdc {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  width: 100%;
  padding: 0.4rem 0.55rem 0.45rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
}

.kdc-meta {
  display: flex;
  align-items: baseline;
  gap: 0.3rem;
  margin: 0;
  min-width: 0;
  font-size: 0.68rem;
  line-height: 1.4;
}

/* 日期：事件语义色（琥珀 = 事件；design D12）。 */
.kdc-date {
  flex: 0 0 auto;
  color: var(--color-event);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

/* 话题：弱化（同一行，不抢标题）。 */
.kdc-topic {
  min-width: 0;
  color: var(--color-text-muted);
  overflow-wrap: anywhere;
}

.kdc-sep {
  flex: 0 0 auto;
  color: var(--color-text-muted);
}

/* 标题：完整可读（不截断、不省略号）；允许折行。 */
.kdc-title {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
  line-height: 1.5;
  color: var(--color-text-primary);
  white-space: normal;
  overflow-wrap: anywhere;
}

.kdc-summary-btn {
  display: block;
  width: 100%;
  padding: 0;
  border: none;
  background: none;
  text-align: left;
  cursor: pointer;
}

.kdc-summary-btn:hover .kdc-summary,
.kdc-summary-btn:focus-visible .kdc-summary {
  color: var(--color-accent);
}

.kdc-summary-btn:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--color-accent);
}

/* 摘要：默认 1 行节选；展开后全文换行显示（标题不受此规则约束）。 */
.kdc-summary {
  display: block;
  font-size: 0.74rem;
  line-height: 1.6;
  color: var(--color-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kdc-summary-btn.is-expanded .kdc-summary {
  white-space: pre-line;
  overflow: visible;
  text-overflow: clip;
}

/* 关联文章入口：文字链（动作标签），非教学语。 */
.kdc-articles-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  align-self: flex-start;
  margin-top: 0.1rem;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.72rem;
  line-height: 1.4;
  cursor: pointer;
}

.kdc-articles-btn:hover {
  text-decoration: underline;
  text-underline-offset: 2px;
}

.kdc-articles-btn:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--color-accent);
}

.kdc-articles {
  margin-top: 0.25rem;
  padding-top: 0.3rem;
  border-top: 1px solid var(--color-border-subtle);
}
</style>
