/**
 * 控制台泳道选择/观察日投影（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 名单/排序/休眠唯一基准 = A 档响应 × source 全轴（G3 `buildObservationRows`），
 * 观察裁切/B 池只影响日投影；本模块只做 computed 派生，不持有第二份 sourceDays/范围。
 * selectedDate 由壳层唯一持有（矩阵/演进/编年共用），默认/失效时回落到可见区间最后有分区日。
 * 报日索引（reportDates）是第一证据源；**已验证记录日**（verifiedDay，当前观察目标在观察带里
 * 有真实分区记录的日子）是更强的第二证据源：索引缺该日时也不把用户刚选定的它当普通无报日
 * 搬走（8E review R-41）。
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneTrendPoint } from './laneTrendSeries'
import type { DateBounds } from './observationRange'
import {
  buildConsoleLaneGroups,
  buildPoolSeries,
  findSelectedRow,
  reconcileSelectedDate,
} from './laneConsoleView'
import type { LaneDayResolver } from './observationLanes'

export interface ConsoleSelectionDeps {
  sourceWindow: Ref<LaneDynamicsWindowData | null>
  /** A 档自身全轴（不是可见裁切轴）。 */
  axis: ComputedRef<string[]>
  visibleAxis: ComputedRef<string[]>
  selectedLaneId: Ref<number | null>
  resolveLaneDay: LaneDayResolver
  poolBounds: ComputedRef<DateBounds | null>
  /** 报日集合（G4 接线前不传 = 未接线，选日保持旧口径；传 null = 索引未就绪/失败）。 */
  reportDates?: { readonly value: ReadonlySet<string> | null } | null
  /**
   * 已验证记录日（当前观察目标在观察带里有真实分区记录的日子）：该日不被普通回落搬走
   * （报日索引缺该日时黄点点击仍原子生效）；null = 当前无已验证目标 → 走普通回落。
   */
  verifiedDay?: { readonly value: string | null } | null
}

export function useConsoleSelection(deps: ConsoleSelectionDeps) {
  const laneGroups = computed(() => buildConsoleLaneGroups(deps.sourceWindow.value, deps.axis.value))
  const allLanes = computed(() => laneGroups.value.rows)
  const activeLanes = computed(() => laneGroups.value.active)
  const dormantLanes = computed(() => laneGroups.value.dormant)
  const selectedLaneRow = computed(() => findSelectedRow(allLanes.value, deps.selectedLaneId.value))
  const selectedLaneKey = computed(() => selectedLaneRow.value?.topicId ?? null)

  const selectedDate = ref<string | null>(null)
  /** 报日依赖三态：未接线（undefined）/ 已接线但未知（null）/ 已就绪集合。 */
  const reportDatesValue = computed<ReadonlySet<string> | null | undefined>(() => {
    const source = deps.reportDates
    return source === undefined || source === null ? undefined : source.value
  })
  const reconciledSelectedDate = computed(() => {
    const reportDates = reportDatesValue.value
    // A 名单未到达：先不定默认日（否则会把「还没名单」当成「0 正式泳道」选中最新报日，
    // 名单到达后该报日按规则 1 被保留，默认日就永远回不到「最后有分区日」）。
    if (reportDates !== undefined && selectedDate.value === null && deps.sourceWindow.value === null) return null
    return reconcileSelectedDate(
      selectedLaneRow.value,
      deps.visibleAxis.value,
      deps.resolveLaneDay,
      selectedDate.value,
      reportDates,
      deps.verifiedDay?.value ?? null,
    )
  })
  watch(reconciledSelectedDate, (next) => {
    if (next !== selectedDate.value) selectedDate.value = next
  }, { immediate: true })

  /** 视窗控件 series：365 池 A 名单同 ID 日合计；未知不补 0。 */
  const poolSeries = computed<LaneTrendPoint[] | null>(() => {
    const bounds = deps.poolBounds.value
    if (!bounds) return null
    return buildPoolSeries(bounds, allLanes.value.map(row => row.topicId), deps.resolveLaneDay)
  })

  return {
    laneGroups,
    allLanes,
    activeLanes,
    dormantLanes,
    selectedLaneRow,
    selectedLaneKey,
    selectedDate,
    poolSeries,
  }
}
