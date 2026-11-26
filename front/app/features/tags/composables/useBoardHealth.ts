/**
 * 版块体检数据源（rework-workshop-into-radar-and-desk task 0.3）。
 *
 * 面板（旧 tab 结构）与治理右轨（新壳）共用同一套取数/派生/收编逻辑：
 *  - 取数：GET /semantic-boards/:id/health-checkup（快照 + 趋势）。
 *  - 派生：评分分档、checked_at 文案、趋势折线/摘要（纯展示派生，与后端口径无关）。
 *  - 收编：addComposition + toast + 本地剔除候选 + 回调刷新构成。
 *  - 竞态：按请求序号丢弃旧版块/旧请求的迟到响应（切版块时不得覆盖）。
 *
 * 组件只做渲染：`BoardHealthSignal`（信号区）/`BoardHealthDetail`（明细区）/
 * `BoardHealthPanel` 都消费本 composable 的同一份状态。
 */
import { computed, ref, watch, type Ref } from 'vue'
import {
  useSemanticBoardsApi,
  type BoardHealthCandidate,
  type BoardHealthSnapshotDTO,
  type BoardHealthTrendPoint,
} from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'

const ALERT_THRESHOLD = 60
/** 趋势迷你条尺寸（与旧面板一致，纯展示常量）。 */
const TREND_W = 220
const TREND_H = 44

export interface BoardHealthCaptureResult {
  ok: boolean
  error?: string
}

export interface BoardHealthOptions {
  /** 收编成功后的刷新回调（面板刷新构成；右轨可重取体检）。 */
  onCaptured?: () => void
}

export function useBoardHealth(boardId: Ref<number>, options: BoardHealthOptions = {}) {
  const api = useSemanticBoardsApi()
  const { success: notifySuccess } = useNotify()

  const snapshot = ref<BoardHealthSnapshotDTO | null>(null)
  const trend = ref<BoardHealthTrendPoint[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** 刷新失败但保留上次成功快照（≠ 无数据；首错仍为 error+空态）。 */
  const staleNotice = ref(false)
  /**
   * 最近一次失败的 HTTP 状态码（成功/无失败时为 null）。
   * 消费方据此区分「刷新失败（网络/5xx，仍是 stale）」与「资源不存在/无权（404/403）」——
   * 后者不是"刷新失败"，旧快照可能已失效（版块被删/权限变化），不能当作仍可操作的清单。
   */
  const errorStatus = ref<number | null>(null)
  const capturingId = ref<number | null>(null)

  let requestSeq = 0

  async function load(): Promise<void> {
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    errorStatus.value = null
    staleNotice.value = false
    const res = await api.getBoardHealthCheckup(boardId.value)
    if (seq !== requestSeq) return
    if (res.success && res.data) {
      snapshot.value = res.data.latest
      trend.value = res.data.trend ?? []
    } else {
      error.value = res.error || '体检数据加载失败'
      errorStatus.value = res.status ?? null
      // 刷新失败保留旧快照（stale 标注）；首错才清空为无数据态，不伪 0。
      if (snapshot.value === null) trend.value = []
      else staleNotice.value = true
    }
    loading.value = false
  }

  watch(boardId, () => {
    snapshot.value = null
    trend.value = []
    error.value = null
    errorStatus.value = null
    staleNotice.value = false
    void load()
  }, { immediate: true })

  const score = computed(() => snapshot.value?.llm_score ?? null)

  const tier = computed<'good' | 'mid' | 'low' | null>(() => {
    const s = score.value
    if (s == null) return null
    if (s >= 70) return 'good'
    if (s >= ALERT_THRESHOLD) return 'mid'
    return 'low'
  })

  const isLowScore = computed(() => score.value != null && score.value < ALERT_THRESHOLD)

  const checkedAtText = computed(() => {
    const at = snapshot.value?.checked_at
    if (!at) return ''
    const d = new Date(at)
    return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  })

  /** 趋势折线（0-100 归一到 SVG 视口；<2 个评分点返回空串）。 */
  const trendPolyline = computed(() => {
    const points = trend.value.filter((p) => p.llm_score != null) as Array<{ checked_at: string; llm_score: number }>
    if (points.length < 2) return ''
    const step = TREND_W / (points.length - 1)
    return points
      .map((p, i) => `${(i * step).toFixed(1)},${(TREND_H - (p.llm_score / 100) * TREND_H).toFixed(1)}`)
      .join(' ')
  })

  const trendSummary = computed(() => {
    const points = trend.value.filter((p) => p.llm_score != null)
    if (points.length === 0) return '暂无评分'
    const scores = points.map((p) => p.llm_score as number)
    const first = scores[0] ?? 0
    const last = scores[scores.length - 1] ?? 0
    const delta = Math.round(last - first)
    return `${points.length} 轮 · 首轮 ${Math.round(first)} → 最新 ${Math.round(last)}（${delta >= 0 ? '+' : ''}${delta}）`
  })

  /**
   * 收编候选（手动挂载特权，不走 LLM 建议管道）：
   * 成功 → toast + 本地剔除候选行 + onCaptured 回调；失败 → 返回 error 供宿主行内提示。
   */
  async function capture(candidate: BoardHealthCandidate): Promise<BoardHealthCaptureResult> {
    if (capturingId.value !== null) return { ok: false }
    capturingId.value = candidate.aux_id
    try {
      const res = await api.addComposition(boardId.value, candidate.aux_id)
      if (!res.success) return { ok: false, error: res.error || '请重试' }
      notifySuccess(`已收编「${candidate.label}」，历史归属不自动回填`)
      if (snapshot.value) {
        snapshot.value = {
          ...snapshot.value,
          uncaptured_candidates: snapshot.value.uncaptured_candidates.filter((x) => x.aux_id !== candidate.aux_id),
        }
      }
      options.onCaptured?.()
      return { ok: true }
    } finally {
      capturingId.value = null
    }
  }

  return {
    snapshot,
    trend,
    loading,
    error,
    errorStatus,
    staleNotice,
    capturingId,
    score,
    tier,
    isLowScore,
    checkedAtText,
    trendPolyline,
    trendSummary,
    load,
    capture,
    ALERT_THRESHOLD,
    TREND_HEIGHT: TREND_H,
  }
}
