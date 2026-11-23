<script setup lang="ts">
/**
 * 当日关键事件列表（add-key-event-matrix-view，ui-design IA「下方阅读区」）。
 *
 * 选中日驱动（与矩阵共用 selectedDate）：展示该日全部 confirmed 事件——标题 +
 * 所属话题（确认时快照）+ 摘要内联展开（单开手风琴，其余收束）+ 关联文章。
 *
 * 事件 → 文章 → 阅读闭环（design D12）：关联文章直接复用 `LaneRelatedArticles`
 * （同一份 laneDayReading 文章缓存，≤20 FIFO、在途合乘、晚到隔离——不新建第二份）；
 * 点文章 emit open-article，由壳层已挂载的 ArticlePreviewModal 打开全文。
 * 本面板只读：不出现注记确认/取消/编辑（那是日报阅读台 KeyEventMarkerActions 的职责）。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import LaneRelatedArticles from './LaneRelatedArticles.vue'

const props = defineProps<{
  selectedDate: string | null
  /** 日期 → 当日全部 confirmed 事件（接口序）。 */
  eventsByDate: ReadonlyMap<string, BoardKeyEventItem[]>
  /** 壳层共享读取实例（文章元信息按需读，不自建缓存）。 */
  reading: LaneDayReadingStore
}>()

const emit = defineEmits<{
  selectDate: [date: string]
  openArticle: [articleId: string]
}>()

const dayEvents = computed(() =>
  props.selectedDate ? props.eventsByDate.get(props.selectedDate) ?? [] : [])

/** 摘要手风琴：选中日内联展开一条，其余收束（ui-design Interaction Contract）。 */
const expandedId = ref<string | null>(null)

watch(() => props.selectedDate, () => {
  expandedId.value = null
})

function toggleCard(event: BoardKeyEventItem): void {
  expandedId.value = expandedId.value === event.id ? null : event.id
}

function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}
</script>

<template>
  <section class="kdp" data-testid="key-event-day-panel">
    <p v-if="!selectedDate" class="kdp-hint" data-testid="key-event-day-empty-selection">
      点时间轴上的事件节点选择日期，这里显示当天全部确认的关键事件。
    </p>
    <template v-else>
      <div class="kdp-head">
        <span class="kdp-title" data-testid="key-event-day-title">{{ shortDate(selectedDate) }} 关键事件</span>
        <span class="kdp-count">{{ dayEvents.length }} 条</span>
      </div>
      <p v-if="dayEvents.length === 0" class="kdp-hint" data-testid="key-event-day-none">
        这一天没有确认的关键事件 · 在日报阅读台标记
      </p>
      <article
        v-for="event in dayEvents"
        v-else
        :key="event.id"
        class="kdp-card"
        :class="{ 'is-open': expandedId === event.id }"
        :data-testid="`key-event-card-${event.id}`"
      >
        <button
          type="button"
          class="kdp-card-head"
          :aria-expanded="expandedId === event.id"
          :title="event.titleSnapshot"
          @click="toggleCard(event)"
        >
          <Icon icon="mdi:star" width="13" class="kdp-star" />
          <span class="kdp-card-title">{{ event.titleSnapshot }}</span>
          <span class="kdp-card-topic">{{ event.topicLabelSnapshot }}</span>
          <Icon :icon="expandedId === event.id ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="14" class="kdp-chev" />
        </button>
        <div v-if="expandedId === event.id" class="kdp-card-body">
          <p v-if="event.summarySnapshot" class="kdp-summary">{{ event.summarySnapshot }}</p>
          <!-- 无关联文章不显示入口（spec：MUST NOT 空列表占位） -->
          <LaneRelatedArticles
            v-if="event.articleIds.length > 0"
            :reading="reading"
            :article-ids="event.articleIds"
            :thread-id="`key-event-${event.id}`"
            @open-article="id => emit('openArticle', id)"
          />
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.kdp {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  scroll-margin-top: 0.8rem;
}

.kdp-head {
  display: flex;
  align-items: baseline;
  gap: 0.45rem;
}

.kdp-title {
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.kdp-count {
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.kdp-hint {
  margin: 0;
  padding: 0.55rem 0.7rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  color: var(--color-text-muted);
  font-size: 0.75rem;
  line-height: 1.6;
}

.kdp-card {
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
}

.kdp-card.is-open {
  border-color: var(--color-border-medium);
}

.kdp-card-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  width: 100%;
  padding: 0.5rem 0.65rem;
  border: none;
  border-radius: 8px;
  background: none;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.kdp-card-head:hover .kdp-card-title,
.kdp-card-head:focus-visible .kdp-card-title {
  color: var(--color-accent);
}

.kdp-card-head:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--color-accent);
}

.kdp-star {
  flex: 0 0 auto;
  color: var(--color-accent);
}

.kdp-card-title {
  flex: 0 1 auto;
  min-width: 0;
  font-size: 0.8rem;
  font-weight: 600;
  line-height: 1.55;
  /* 标题必须**完整可读**（review R-3）：层数超限的行轴上已无名称（design D7），
     名称由本阅读区承担；此处若再单行省略号截断，事件名就彻底不可读了。
     delta spec「MUST NOT 用省略号截断」→ 允许换行，不做 ellipsis。 */
  white-space: normal;
  overflow-wrap: anywhere;
}

.kdp-card-topic {
  flex: 0 0 auto;
  margin-left: auto;
  max-width: 30%;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kdp-chev {
  flex: 0 0 auto;
  color: var(--color-text-muted);
}

.kdp-card-body {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  padding: 0 0.65rem 0.55rem;
}

.kdp-summary {
  margin: 0;
  font-size: 0.76rem;
  line-height: 1.7;
  color: var(--color-text-secondary);
  white-space: pre-line;
}
</style>
