<script setup lang="ts">
/**
 * 当日其他话题 · 阅读正文状态块（G3 内聚拆分，纯视图）。
 *
 * 只覆盖阅读正文的非 ready 状态：加载骨架 / 无日报 / 读取失败（受控重试意图）/ 详情已更新
 * 缺该分区（受控刷新意图）。文案各自诚实、不借别日/别分区；按钮不直连读取 store——
 * 重试/刷新只 emit，由 G4 统一编排「force 索引 → force 当日」。
 */
defineProps<{
  /** 'pending' | 'no-report' | 'error' | 'missing-section'（ready/idle 不进来）。 */
  state: string
  /** 当前状态标题（与页头同一份文案）。 */
  title: string
  /** 报告读取失败的技术文案（仅 error）。 */
  errorText: string | null
}>()

const emit = defineEmits<{
  /** 受控报告重试/刷新意图（父级带当前稳定 date/section 转发）。 */
  retryReport: []
}>()
</script>

<template>
  <div v-if="state === 'pending'" class="otr-skeleton" data-testid="other-reading-loading">
    <div class="otr-skeleton-line" />
    <div class="otr-skeleton-line is-short" />
  </div>
  <div v-else-if="state === 'no-report'" class="otr-state" data-testid="other-reading-no-report">
    所选日没有日报；不借别日或相似标题补摘要。
  </div>
  <div v-else-if="state === 'error'" class="otr-state otr-state--error" role="alert" data-testid="other-reading-error">
    <span>{{ title }}<template v-if="errorText">（{{ errorText }}）</template>；不借别日内容。</span>
    <button type="button" data-testid="other-reading-retry" @click="emit('retryReport')">重试</button>
  </div>
  <div v-else-if="state === 'missing-section'" class="otr-state" data-testid="other-reading-missing-section">
    <span>当日报告已更新，未找到该分区；不借别分区或相似标题。刷新会按稳定 ID 重读当日详情。</span>
    <button type="button" data-testid="other-reading-refresh" @click="emit('retryReport')">刷新</button>
  </div>
</template>

<style scoped>
.otr-state {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.6rem;
  flex-wrap: wrap;
  margin: 0;
  padding: 0.35rem 0;
  font-size: 0.78rem;
  line-height: 1.65;
  color: var(--color-text-muted);
  overflow-wrap: break-word;
}

.otr-state--error {
  color: var(--color-error);
}

.otr-state button {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
}

.otr-state button:hover {
  border-color: var(--color-accent);
}

.otr-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.otr-skeleton-line {
  height: 0.85rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
}

.otr-skeleton-line.is-short {
  width: 55%;
}
</style>
