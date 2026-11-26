<script setup lang="ts">
/**
 * 右轨「当日其他话题」子块（add-other-topics-to-context-rail design D4/D5/D6）。
 *
 * 纯受控：行/计数/状态词全部来自壳层用 `buildOtherTopics` 派生后经 props 下发的
 * `OtherTopicsView`——本组件**不自建网络/日期/身份排除/行序/计数**，也不重算排除口径
 * （唯一实现点 = `dayOtherTopics.ts`；矩阵「当日其他话题」组同源同值）。
 *
 * 与「当日日报」块内泳道层**显式分家**（两套量纲，不可并排成同一列）：
 *  - 泳道层「N 条」= lane-dynamics 该泳道当日合计（与矩阵日格同源同值）；
 *  - 本块「N 条线索」= 该分区非综合稿 thread 数（与矩阵「当日其他话题」组同源同值）。
 *  故本块自带标题行 + 上分隔线，单位后缀写「条线索」。
 *
 * 状态文案全部复用 `dayOtherTopics.ts` 既有常量（模型已给 `statusText`/`note`），
 * 不新写一套空态/错误文案；失败态的「重试」沿用右轨既有受控重试（只重取当日详情）。
 *
 * 点行只发事件（`sectionId` + `topicId`），不改选择态、不开浮层：日期唯一源是矩阵选中日，
 * 由宿主（`BoardContextRail`）补上 `date` 后交给既有 `selectOther` 选行链。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { OtherTopicRow, OtherTopicsView } from './dayOtherTopics'

const props = defineProps<{
  /** 「当日其他话题」组视图模型（矩阵同源派生；本组件只渲染）。 */
  view: OtherTopicsView
}>()

const emit = defineEmits<{
  /** 点某行 → 选中该分区（`date` 由宿主补：唯一日期源 = 矩阵选中日）。 */
  selectOther: [payload: { sectionId: string, topicId: string | null }]
  /** 读取失败 → 受控重试（只重取当日详情，不重建索引、不改选中日）。 */
  retry: []
}>()

/** 计数徽标只在 ready 给数字（0 是真实结论）；其余态不冒 0。 */
const countText = computed(() => (
  props.view.state === 'ready' && typeof props.view.clueCount === 'number'
    ? `${props.view.clueCount} 条线索`
    : null
))

/** 非 ready 没有可读行（不渲染半截列表）。 */
const rows = computed<OtherTopicRow[]>(() => (props.view.state === 'ready' ? props.view.rows : []))

/** 失败态（报日索引失败 / 当日报告读取失败）才给重试入口。 */
const failed = computed(() => props.view.state === 'error' || props.view.state === 'index-error')

function selectRow(row: OtherTopicRow): void {
  emit('selectOther', { sectionId: row.sectionId, topicId: row.persistentTopicId })
}
</script>

<template>
  <section class="rob" data-testid="rail-other-topics">
    <div class="rob-head">
      <span class="rob-title">
        <Icon icon="mdi:thought-bubble-outline" width="13" />
        当日其他话题
      </span>
      <span v-if="countText !== null" class="rob-count" data-testid="rail-other-topics-count">{{ countText }}</span>
    </div>

    <ul v-if="rows.length > 0" class="rob-rows" data-testid="rail-other-topics-rows">
      <li
        v-for="row in rows"
        :key="row.sectionId"
        class="rob-row"
        :data-testid="`rail-other-topic-item-${row.sectionId}`"
      >
        <button
          type="button"
          class="rob-row-btn"
          data-testid="rail-other-topic-row"
          :title="`${row.label} · ${row.statusText}`"
          @click="selectRow(row)"
        >
          <span class="rob-row-label" data-testid="rail-other-topic-label">{{ row.label }}</span>
          <span class="rob-row-count" data-testid="rail-other-topic-count">{{ row.threadCount }} 条线索</span>
          <span class="rob-row-status" data-testid="rail-other-topic-status">{{ row.statusText }}</span>
        </button>
      </li>
    </ul>

    <div v-else-if="failed" class="rob-error" data-testid="rail-other-topics-error">
      <Icon icon="mdi:alert-circle-outline" width="13" />
      <span>{{ view.statusText }}</span>
      <button
        v-if="view.retryable"
        type="button"
        class="rob-link"
        data-testid="rail-other-topics-retry"
        @click="emit('retry')"
      >重试</button>
    </div>

    <p v-else class="rob-hint" data-testid="rail-other-topics-hint">{{ view.statusText }}</p>

    <p v-if="view.note" class="rob-note" data-testid="rail-other-topics-note">{{ view.note }}</p>
  </section>
</template>

<style scoped>
.rob {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 0;
  margin-top: 0.35rem;
  padding-top: 0.55rem;
  border-top: 1px solid var(--color-border-subtle);
}

.rob-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.45rem;
  min-width: 0;
}

.rob-title {
  display: inline-flex;
  align-items: baseline;
  gap: 0.3rem;
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  min-width: 0;
  white-space: nowrap;
}

.rob-count {
  flex: 0 0 auto;
  font-size: 0.68rem;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.rob-rows {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.rob-row-btn {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.15rem 0.45rem;
  align-items: baseline;
  width: 100%;
  min-width: 0;
  padding: 0;
  border: 0;
  background: none;
  text-align: left;
  cursor: pointer;
}

.rob-row-label {
  grid-row: 1;
  grid-column: 1;
  min-width: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  font-size: 0.76rem;
  line-height: 1.4;
  color: var(--color-text-primary);
}

.rob-row-btn:hover .rob-row-label {
  color: var(--color-accent);
}

.rob-row-count {
  grid-row: 1;
  grid-column: 2;
  font-size: 0.68rem;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.rob-row-status {
  grid-row: 2;
  grid-column: 1 / -1;
  min-width: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.rob-hint {
  margin: 0;
  font-size: 0.7rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.rob-error {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.7rem;
  color: var(--color-error);
}

.rob-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.rob-note {
  margin: 0;
  font-size: 0.66rem;
  line-height: 1.5;
  color: var(--color-text-muted);
}
</style>
