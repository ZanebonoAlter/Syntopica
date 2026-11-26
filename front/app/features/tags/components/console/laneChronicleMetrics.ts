/**
 * 编年当日计数口径文案（8C G4b 从 `LaneChronicle` 按内聚行为抽出）。
 *
 * 口径不变（R26）：未知不等于 0——全未知标 `?`，部分未知标 `≥已知部分`；当日线索合计
 * 已知时（`clueCount` 非 null，后端权威值）直接用该值，不被「部分桶未知」拉成降级态。
 * 桶峰值/桶数/综合稿差额/未知提示进可展开的计数说明。纯派生，不发请求、不改数据。
 */
import { computed, type ComputedRef } from 'vue'
import { dayCellMetrics, type WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'

export function useLaneChronicleMetrics(currentDay: ComputedRef<WindowLaneDay | null>) {
  const metrics = computed(() => dayCellMetrics(currentDay.value))

  /** R26：未知不能冒完整总量。
   *  口径（fix-lane-metric-provenance）：当日线索合计 = 后端 `clue_count` 时（来源
   *  `backend`）那是权威完整值，直接原样显示（哪怕各桶明细有未知）；旧响应回退各桶
   *  合计且有桶未知时（`buckets` + hasUnknown），已知部分是**下界** → 写 `≥N`；
   *  全未知分不了合计 → `?`。
   */
  function formatDayTotal(): string {
    const m = metrics.value
    if (m.clueCount === null) return '?'
    if (m.isLowerBound) return `≥${m.clueCount}`
    return String(m.clueCount)
  }

  /** 默认简短真实合计（获批标题优先稿），桶峰值/桶数/口径进可展开计数说明。 */
  const metricsSummary = computed(() => {
    if (currentDay.value === null) return null
    return `当日合计 ${formatDayTotal()} 条`
  })

  const metricsDetail = computed(() => {
    if (currentDay.value === null) return null
    const m = metrics.value
    const peak = m.peak === null ? '?' : String(m.peak)
    const unknownNote = m.hasUnknown ? '（部分桶线索数未知，不补 0）' : ''
    const compositeNote = m.compositeCount !== null && m.compositeCount > 0
      ? ` · 其中 ${m.compositeCount} 条综合稿未计入`
      : ''
    return `桶峰值 ${peak} 条 · 当日合计 ${formatDayTotal()} 条 · 主题桶数 ${m.sectionCount} 个${compositeNote}${unknownNote}`
  })

  return { metrics, metricsSummary, metricsDetail }
}
