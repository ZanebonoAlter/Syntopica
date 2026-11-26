<script setup lang="ts">
/**
 * 泳道态势面板（rework-workshop-into-radar-and-desk 矩阵阶段 G3，ObservationLaneEvolution 子块）。
 *
 * 最近滚动快照逐字展示：默认折叠的 <details>（标题带 asOf 与「非本视窗总结」标注），
 * 正文 = summary 逐字；旧长版 detail 存在时给「展开全文」切换（逐字、不编造）。
 * snapshot=null = 待结算占位（合法态，不挡时间线）。不调用 AI、不写任何状态。
 */
import { ref, watch } from 'vue'
import type { WindowLaneSnapshot } from '~/api/normalizers/laneDynamicsWindow'

const props = defineProps<{
  /** 最新滚动快照；null = 待结算（合法态）。 */
  snapshot: WindowLaneSnapshot | null
}>()

const detailExpanded = ref(false)
// 换泳道（快照对象身份变化）时收起长版展开态
watch(() => props.snapshot, () => {
  detailExpanded.value = false
})
</script>

<template>
  <details v-if="snapshot" class="stp" data-testid="lane-evolution-stance">
    <summary>
      最近结算态势 · <span data-testid="lane-evolution-asof">{{ snapshot.asOf }}</span>（非本视窗总结）
    </summary>
    <p class="stp-text" data-testid="lane-evolution-stance-text">{{ snapshot.summary }}</p>
    <template v-if="snapshot.detail">
      <button
        type="button"
        class="stp-detail-toggle"
        data-testid="lane-evolution-detail-toggle"
        @click="detailExpanded = !detailExpanded"
      >{{ detailExpanded ? '收起全文' : '展开全文' }}</button>
      <p v-if="detailExpanded" class="stp-text is-detail" data-testid="lane-evolution-detail">{{ snapshot.detail }}</p>
    </template>
  </details>
  <div v-else class="stp-pending" data-testid="lane-evolution-pending">
    态势待结算 · 时间线照常展示
  </div>
</template>

<style scoped>
.stp {
  border-left: 3px solid var(--color-accent);
  border-radius: 0 8px 8px 0;
  background: var(--color-accent-subtle);
  padding: 0.45rem 0.65rem;
}

.stp summary {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}

.stp summary:hover {
  color: var(--color-text-primary);
}

.stp-text {
  margin: 0.4rem 0 0;
  font-size: 0.8rem;
  line-height: 1.6;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
}

.stp-text.is-detail {
  border-top: 1px dashed var(--color-border-subtle);
  padding-top: 0.4rem;
}

.stp-detail-toggle {
  margin-top: 0.35rem;
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
  padding: 0;
}

.stp-detail-toggle:hover {
  text-decoration: underline;
}

.stp-pending {
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}
</style>
