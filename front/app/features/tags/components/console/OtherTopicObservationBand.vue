<script setup lang="ts">
/**
 * 同归属读取状态（沿用旧文件名）：仅说明/重试，不再重复关联日期导航。
 * 真实日期导航由矩阵列头/窄屏同源日期条承接；本组件不持有网络、日期或选择状态。
 */
import { computed } from 'vue'
import type { ObservedDayNode, ObservationStatus } from '~/features/tags/composables/useOtherTopicObservation'

const props = defineProps<{
  /** 观察依赖状态（missing-id/idle/loading/ready/error）。 */
  status: ObservationStatus
  /** 刷新失败但保留上次成功数据（内容仍可读，明确陈旧）。 */
  stale: boolean
  /** 失败/陈旧的技术文案（ready+stale 时也保留）。 */
  error: string | null
  /** 当前观察轴内该归属的真实记录日节点（升序）。 */
  nodes: ObservedDayNode[]
}>()

const emit = defineEmits<{
  retry: []
}>()

const showRetry = computed(() => props.status === 'error' || props.stale)

function peakText(node: ObservedDayNode): string {
  return node.threadCount === null ? '?' : String(node.threadCount)
}

function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}

/** 说明：缺身份/读取中/失败/无记录/单日/多日各自诚实，不补 0、不称同一事件。 */
const note = computed<string>(() => {
  if (props.status === 'missing-id') return '该分区当次未归属，无法按稳定 ID 跨日核对。'
  if (props.status === 'idle' || props.status === 'loading') return '正在核对同归属跨日记录…'
  if (props.status === 'error') return '同归属跨日记录读取失败；当前日报正文仍可读。'
  const nodes = props.nodes
  if (nodes.length === 0) return '本窗没有该归属的其他记录日（不补 0、不连线）。'
  if (nodes.length === 1) {
    const node = nodes[0]!
    return `本窗仅 1 个同归属记录日（${shortDate(node.date)} · ${peakText(node)} 条线索）：关联日已标在日期表头。`
  }
  return `同归属记录日 · ${nodes.length} 天（仅按稳定 ID 关联，不称同一事件/正式泳道；关联日已标在日期表头）`
})
</script>

<template>
  <section class="otr-observe" data-testid="other-observe-band" aria-label="同归属跨日观察">
    <div class="otr-observe-head">
      <span class="otr-observe-kicker">同归属记录</span>
      <button
        v-if="showRetry"
        type="button"
        class="otr-observe-retry"
        data-testid="other-observe-retry"
        @click="emit('retry')"
      >重试</button>
    </div>
    <p class="otr-observe-note" data-testid="other-observe-note">{{ note }}</p>
    <p v-if="stale" class="otr-observe-stale" data-testid="other-observe-stale">
      保留上次成功记录（{{ error }}）
    </p>

  </section>
</template>

<style scoped>
.otr-observe {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.35rem 0;
}

.otr-observe-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.6rem;
}

.otr-observe-kicker {
  color: var(--color-text-muted);
  font-size: 0.6rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.otr-observe-note,
.otr-observe-stale {
  margin: 0;
  font-size: 0.72rem;
  line-height: 1.6;
  color: var(--color-text-secondary);
  overflow-wrap: break-word;
}

.otr-observe-stale {
  color: var(--color-warning, var(--color-text-muted));
}

.otr-observe-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
}

.otr-observe-retry:hover {
  border-color: var(--color-accent);
}

</style>
