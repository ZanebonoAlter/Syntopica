<script setup lang="ts">
/**
 * 右栏 · 关联推荐（unify-lane-workbench 3.1 / D5，从 `LaneMemberList.vue` 迁出）。
 *
 * 待选侧唯一承载：推荐行（标题 / 来源泳道 / 估算接近度）＋「＋加入」「定位」。
 * 左栏 SHALL NOT 再出现关联推荐分组（两侧内容不重复——左已选、右待选）。
 *
 * 契约：
 * - **估算接近度**：对当前有效成员质心的实时余弦距离换算（`1 − 距离`），
 *   与成员行的归属快照**不同名不同形**（fix-lane-metric-provenance 两把尺子）；
 *   接近度不可算时不显示数字（不伪造）。
 * - **来源泳道名**：属 candidate（动态）泳道的推荐行标注改挂来源；
 *   无来源不显示装饰性标签。
 * - **pending 同步禁用**：与节点卡/左栏共享同一份 pendingIds，防重复提交。
 * - 组件只收 props/emit，不调 API、不存成员集合（写操作统一走 useLaneCorrection）。
 */
import { computed } from 'vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

const props = defineProps<{
  /** 关联推荐（成员质心口径的候选；按接近度降序在 composable 层已排序）。 */
  recommendations: LaneCorrectionNode[]
  /** 正在提交的节点 id（按钮 loading，防重复提交；与卡片/左栏同一份）。 */
  pendingIds: string[]
  /** 推荐派生失败（候选取数失败）时的行内错误；null = 正常。 */
  error?: string | null
}>()

const emit = defineEmits<{
  /** 「＋ 加入」→ 归回本泳道（与节点卡「加入」同一语义、同一份状态）。 */
  add: [id: string]
  /** 「定位」→ 画布聚焦该节点（只改选中态）。 */
  focus: [id: string]
}>()

/** 标题行计数（与推荐行同源；active 只读不在列表里）。 */
const countText = computed(() => `待选 · ${props.recommendations.length}`)

function isPending(id: string): boolean {
  return props.pendingIds.includes(id)
}

/** 估算接近度（两把尺子：推荐 = 实时估算，成员 = 归属快照——不同名不同形）。 */
function proximityText(node: LaneCorrectionNode): string {
  if (node.proximity === null) return ''
  return `估算接近度 ${node.proximity.toFixed(2)}`
}
</script>

<template>
  <section class="lrl-panel" data-testid="lane-recommend-panel" aria-label="关联推荐">
    <header class="lrl-head">
      <h3 class="lrl-title">关联推荐</h3>
      <span class="lrl-count" data-testid="lane-recommend-count">{{ countText }}</span>
      <span class="lrl-timing">重心 · 次日重算</span>
    </header>

    <div class="lrl-body">
      <p v-if="error" class="lrl-error" role="alert" data-testid="lane-recommend-error">{{ error }}</p>
      <ul v-else-if="recommendations.length" class="lrl-list" data-testid="lane-recommend-list">
        <li
          v-for="node in recommendations"
          :key="node.id"
          class="lrl-row"
          :data-testid="`lane-recommend-${node.id}`"
        >
          <span class="lrl-date">{{ node.date.slice(5) }}</span>
          <span class="lrl-name" :title="node.title">{{ node.title }}</span>
          <span v-if="node.topicLabel" class="lrl-tag" data-testid="lane-recommend-source">来自 {{ node.topicLabel }}</span>
          <span v-if="proximityText(node)" class="lrl-prox" :data-testid="`lane-recommend-prox-${node.id}`">{{ proximityText(node) }}</span>
          <button
            type="button"
            class="lrl-peek"
            :aria-label="`在画布上定位 ${node.title}`"
            :data-testid="`lane-recommend-peek-${node.id}`"
            @click="emit('focus', node.id)"
          >定位</button>
          <button
            type="button"
            class="lrl-add"
            :disabled="isPending(node.id)"
            :aria-label="`把「${node.title}」加入本泳道`"
            :data-testid="`lane-recommend-add-${node.id}`"
            @click="emit('add', node.id)"
          >＋ 加入</button>
        </li>
      </ul>
      <p v-else class="lrl-empty" data-testid="lane-recommend-empty">没有新的关联推荐。</p>
    </div>
  </section>
</template>

<style scoped>
.lrl-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  border-left: 1px solid var(--color-border-medium);
}

.lrl-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px 6px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lrl-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.lrl-count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.lrl-timing {
  margin-left: auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px dashed var(--color-border-medium);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.lrl-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 0 12px 10px;
}

.lrl-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.lrl-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 4px 0;
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: 12px;
}

.lrl-row:last-child {
  border-bottom: 0;
}

.lrl-date {
  flex: none;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lrl-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.lrl-tag {
  flex: none;
  font-size: 10px;
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  padding: 0 4px;
}

.lrl-prox {
  flex: none;
  font-size: 10px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lrl-peek,
.lrl-add {
  flex: none;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 11px;
  padding: 1px 6px;
  cursor: pointer;
}

.lrl-add {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lrl-peek:hover,
.lrl-add:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lrl-add:disabled {
  opacity: 0.55;
  cursor: wait;
}

.lrl-empty {
  margin: 4px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.lrl-error {
  margin: 4px 0;
  font-size: 12px;
  color: var(--color-error);
}
</style>
