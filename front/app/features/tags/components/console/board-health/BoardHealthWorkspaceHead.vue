<script setup lang="ts">
/**
 * 工作面头部与状态块（board-health-triage-advice seam C2 / task 3.2）。
 *
 * 承载四类内容：
 *  1. 顶部一行：本轮待核对数 + 当前项真实进展（**不做评分大卡**）；
 *  2. 非正常轮次的说明（旧快照 / 空清单 / 降级 / 读取失败 / 等待首轮）——**不猜诊断、不说全部健康**；
 *  3. 两行状态提示：stale（刷新失败保留上次快照）、分诊有丢弃项（数据不完整）；
 *  4. 一行快照口径：评分/评语取最新体检快照，不随观察日期回溯（右轨只留结论，口径归详情）；
 *  5. 折叠区「本轮体检依据」：**复用既有 `BoardHealthSignal`**（评分/评语/挂载统计/趋势，零额外请求）。
 */
import { Icon } from '@iconify/vue'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import BoardHealthSignal from '../BoardHealthSignal.vue'

const props = defineProps<{
  /** 本轮需核对的项数（正常轮次）。 */
  issueCount: number
  /** 当前项的真实进展（无任务时为空串）。 */
  progressText: string
  /** 非正常轮次的标题（正常轮次为空串）。 */
  stateTitle: string
  /** 非正常轮次的说明（正常轮次为空串）。 */
  stateText: string
  /** 是否处于非正常轮次（决定是否显示状态块）。 */
  abnormal: boolean
  /** 读取失败（可重试）。 */
  retryable: boolean
  /** 刷新失败但保留上次快照。 */
  stale: boolean
  /** 本轮被剔除/不可执行的分诊条数（>0 时提示数据不完整）。 */
  dropped: number
  /** 是否展示「原统计与评分趋势」折叠区（旧快照/降级/空清单时）。 */
  showBasis: boolean
  snapshot: BoardHealthSnapshotDTO | null
  score: number | null
  tier: 'good' | 'mid' | 'low' | null
  checkedAtText: string
  trendPolyline: string
  trendSummary: string
}>()

const emit = defineEmits<{
  retry: []
}>()
</script>

<template>
  <div class="bhw-head-block">
    <header class="bhw-head">
      <span class="bhw-head-title">本轮体检</span>
      <span class="bhw-head-count" data-testid="bhw-issue-count">
        <template v-if="!abnormal">本轮需核对 {{ issueCount }} 项</template>
        <template v-else>{{ stateTitle }}</template>
      </span>
      <span v-if="progressText" class="bhw-head-progress" data-testid="bhw-progress-summary">{{ progressText }}</span>
    </header>

    <!-- 状态说明行（纯说明，不可交互；不伪装成动作） -->
    <p v-if="abnormal" class="bhw-state" data-testid="bhw-state">
      <strong>{{ stateTitle }}</strong>
      <span>{{ stateText }}</span>
      <button v-if="retryable" type="button" class="bhw-link" data-testid="bhw-retry-load" @click="emit('retry')">
        重试
      </button>
    </p>

    <!-- 刷新失败但保留上次快照（stale） -->
    <p v-if="stale" class="bhw-hint" data-testid="bhw-stale-hint">
      刷新失败，显示上次快照；提交修复时仍须核对实时版本。
      <button type="button" class="bhw-link" @click="emit('retry')">重试</button>
    </p>

    <!-- 分诊有丢弃项：一行说明数据不完整（不加信息卡） -->
    <p v-if="dropped > 0" class="bhw-hint" data-testid="bhw-dropped-hint">
      本轮分诊有 {{ dropped }} 条记录不可执行（已剔除），下面的清单不完整。
    </p>

    <!-- 体检快照口径（右轨不再重复，口径归详情）：最新一轮体检，不是所选日期的评分 -->
    <p v-if="snapshot" class="bhw-hint" data-testid="bhw-latest-snapshot-hint">
      评分与评语取最新体检快照，不随观察日期回溯为历史评分。
    </p>

    <details v-if="showBasis" class="bhw-basis" data-testid="bhw-basis">
      <summary>原统计与评分趋势</summary>
      <BoardHealthSignal
        :snapshot="snapshot"
        :loading="false"
        :error="null"
        :score="score"
        :tier="tier"
        :checked-at-text="checkedAtText"
        :trend-polyline="trendPolyline"
        :trend-summary="trendSummary"
        :show-candidates="false"
        :show-detail-entry="false"
      />
    </details>
  </div>
</template>

<style scoped>
.bhw-head-block {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  min-width: 0;
}

.bhw-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem;
  padding-bottom: 0.45rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.bhw-head-title {
  font-size: 0.66rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.bhw-head-count {
  font-size: 0.8rem;
  color: var(--color-text-primary);
}

.bhw-head-progress {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.bhw-state {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.4rem;
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.bhw-state strong {
  color: var(--color-text-primary);
}

.bhw-hint {
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-text-muted);
  line-height: 1.55;
}

.bhw-link {
  border: none;
  background: none;
  padding: 0;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
}

.bhw-basis {
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  padding: 0.45rem 0.6rem;
  background: var(--color-bg-sunken);
}

.bhw-basis summary {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}
</style>
