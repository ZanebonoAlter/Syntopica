/**
 * 编年当日计数口径文案（8C G4b 从 `LaneChronicle` 按内聚行为抽出）。
 *
 * 口径不变（R26）：未知不等于 0——全未知标 `?`，部分未知标 `≥已知部分`；默认给简短真实合计，
 * 桶峰值/桶数/未知提示进可展开的计数说明。纯派生，不发请求、不改数据。
 */
import { computed, type ComputedRef } from 'vue'
import { dayCellMetrics, type WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'

export function useLaneChronicleMetrics(currentDay: ComputedRef<WindowLaneDay | null>) {
  const metrics = computed(() => dayCellMetrics(currentDay.value))

  /** R26：未知不能冒完整总量。 */
  function formatDayTotal(): string {
    const m = metrics.value
    if (m.hasUnknown) return m.peak === null ? '?' : `≥${m.total}`
    return String(m.total)
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
    return `桶峰值 ${peak} 条 · 当日合计 ${formatDayTotal()} 条 · 主题桶数 ${m.sectionCount} 个${unknownNote}`
  })

  return { metrics, metricsSummary, metricsDetail }
}
