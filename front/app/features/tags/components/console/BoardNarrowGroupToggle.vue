<script setup lang="ts">
/**
 * 窄屏折叠组 summary（G3 内聚拆分，纯视图）。
 *
 * 正式「当日其他话题」组与「休眠泳道」组共用同一个可聚焦原生 button：箭头 + 标题 + 计数 +
 * 右侧状态说明；展开态/禁用/文案全由父级传入（两个组互不串联），本组件不持状态。
 */
import { Icon } from '@iconify/vue'

defineProps<{
  title: string
  /** 计数文案（null = 不显示计数）。 */
  count: string | null
  /** 计数的 testid（省略 = 无 testid）。 */
  countTestid?: string
  note: string
  /** 说明的 testid（省略 = 无 testid）。 */
  noteTestid?: string
  expanded: boolean
  disabled?: boolean
  /** 组 summary 的 testid。 */
  testid: string
}>()

const emit = defineEmits<{
  toggle: []
}>()
</script>

<template>
  <button
    type="button"
    class="bns-group-toggle"
    :data-testid="testid"
    :aria-expanded="expanded"
    :disabled="disabled"
    @click="emit('toggle')"
  >
    <Icon :icon="expanded ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="13" />
    <span class="bns-group-title">
      {{ title }}<span v-if="count !== null" class="bns-group-count" :data-testid="countTestid"> · {{ count }}</span>
    </span>
    <span class="bns-group-note" :data-testid="noteTestid">{{ note }}</span>
  </button>
</template>

<style scoped>
.bns-group-toggle {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  width: 100%;
  padding: 0.35rem 0.1rem;
  border: 0;
  background: none;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
  font-size: 0.76rem;
}

.bns-group-toggle:disabled {
  cursor: default;
  color: var(--color-text-muted);
}

.bns-group-title {
  font-weight: 600;
}

.bns-group-count {
  color: var(--color-text-secondary);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.bns-group-note {
  margin-left: auto;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  text-align: right;
}
</style>
