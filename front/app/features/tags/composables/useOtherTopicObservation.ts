/**
 * 同归属跨日观察只读依赖（rework-workshop-into-radar-and-desk 矩阵阶段 8C G3）。
 *
 * 职责（合同 §4.3 获批跨日观察带）：
 *  - 只读复用 section-timeline（经 `api/otherTopicObservation` 边界归一），**不建**第二套日报
 *    index/detail 读取器：日报报日索引/详情仍全部由 `useLaneDayReading` 单 store 服务。
 *  - **板块作用域单份成功数据**：一次 GET（days=0 = 板块全部历史）后，任意其他话题/窗口都
 *    本地投影——拖观察范围/预览只重算 `nodes`，不按像素发请求；换话题命中同一份数据。
 *  - **晚到隔离**：板块代际（epoch）+ 请求序号（seq）；切板块/卸载后旧响应不写数据/错误/
 *    loading；同板块强刷时旧响应不得覆盖更新的强刷结果；在途请求合乘。失败保留旧成功数据并
 *    标 `stale`（明确陈旧），首次失败才给 error；失败不入成功缓存，重试真的再发 GET。
 *  - **投影口径**：只取 `persistentTopicId` 与请求 topicId 完全一致、且日期落在当前连续观察轴
 *    内的**真实分区记录日**；同日多分区按接口原序保留，节点计数 = 同日 `threadCount` 峰值
 *    （任一分区缺失则整日未知 null，不冒 0、不求和）；节点携带同一天首个分区的稳定
 *    report/section 目标供 G4 原子切日。
 *  - **刷新语义**：已有成功数据时强刷在途，`view.status` 仍 `ready`（内容可读）但 `refreshing=true`
 *    ——消费方（阅读终态信号）不得把它当本轮终态；`invalidate()` 是生成完成后的**懒失效**：
 *    不发请求、丢掉成功数据并作废在途响应，下次 `ensureObservation` 才真的重读。
 *  - 无可靠 topicId 时**不发请求**，状态 `missing-id`（缺身份不是无记录）。
 *  - 不把嵌套当前 `persistent_topic`（label/status/hit_count）当历史——归一阶段已丢弃。
 *
 * 不拥有：selectedDate/阅读槽/组展开态/GUI 日期（G4 壳层持有）；本 composable 只服务
 * 观察带的读取与投影。
 */
import { computed, getCurrentScope, onScopeDispose, ref, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import {
  useOtherTopicObservationApi,
  type ObservationResponse,
  type ObservationSectionRecord,
  type ObservationTimelineData,
} from '~/api/otherTopicObservation'

/** 单个真实记录日节点（同一天同归属可有多条分区记录）。 */
export interface ObservedDayNode {
  date: string
  /** 同一天记录的首个日报 ID（同一天同板块同一份报告）。 */
  reportId: string
  /** 稳定目标分区（同日报告原序首个）；G4 切日后必须按此 ID 核对详情，缺失不借标题补。 */
  sectionId: string
  /** 同一天同归属全部分区（接口原序，含目标本身）。 */
  sectionIds: string[]
  /** 各分区历史原题（接口原序）；只用于关联日提示，不合并正文。 */
  sectionTitles: string[]
  /** 同日分区 threadCount 峰值；任一分区计数缺失 = null（未知，不冒 0）。 */
  threadCount: number | null
}

export type ObservationStatus = 'missing-id' | 'idle' | 'loading' | 'ready' | 'error'

/** 观察带展示视图（组件只读，不含网络/状态机）。 */
export interface OtherTopicObservationView {
  topicId: string | null
  status: ObservationStatus
  /** 失败/陈旧的技术文案（ready+stale 时也保留，用于明确陈旧）。 */
  error: string | null
  /** 刷新失败但保留上次成功数据（内容仍可读）。 */
  stale: boolean
  /** 已有成功数据时强刷在途：status 仍 `ready`，但本轮不能当终态（布局还会变）。 */
  refreshing: boolean
  /** 覆盖口径：'all' = 板块全部历史（本观察带唯一口径）；null = 尚未取到。 */
  coverage: 'all' | null
  /** 当前观察轴内、该 topicId 的真实记录日节点（升序，来自 observation axis 顺序）。 */
  nodes: ObservedDayNode[]
}

export interface OtherTopicObservationFetchers {
  /** 产品读取签名只有 boardId（恒定 days=0 全部历史，不开放窗口 days）。 */
  timeline: (boardId: number) => Promise<ObservationResponse<ObservationTimelineData>>
}

export interface UseOtherTopicObservationDeps {
  boardId: Ref<number>
  /** 当前可见观察轴（升序连续）；只用于本地裁投影，变化**不触发请求**。 */
  axis: () => readonly string[]
  fetchers?: OtherTopicObservationFetchers
}

/**
 * 把板块级 section-timeline 记录投影为某 topicId 在当前观察轴内的记录日节点。
 * 纯函数：同一天多分区保持输入原序；峰值缺失即 null；窗外/其他 ID/无 ID 一律不进入。
 */
export function projectObservationDays(
  records: readonly ObservationSectionRecord[] | null | undefined,
  options: { topicId: string | null, axis: readonly string[] },
): ObservedDayNode[] {
  const topicId = options.topicId
  if (topicId === null || records == null || records.length === 0) return []
  const byDate = new Map<string, ObservationSectionRecord[]>()
  for (const record of records) {
    if (record.persistentTopicId !== topicId) continue
    if (record.date === '') continue
    const bucket = byDate.get(record.date)
    if (bucket) bucket.push(record)
    else byDate.set(record.date, [record])
  }
  if (byDate.size === 0) return []

  const nodes: ObservedDayNode[] = []
  for (const date of options.axis) {
    const dayRecords = byDate.get(date)
    if (!dayRecords || dayRecords.length === 0) continue
    let peak: number | null = 0
    for (const record of dayRecords) {
      if (record.threadCount === null) {
        peak = null
        break
      }
      if (peak !== null && record.threadCount > peak) peak = record.threadCount
    }
    const first = dayRecords[0]!
    nodes.push({
      date,
      reportId: first.reportId,
      sectionId: first.sectionId,
      sectionIds: dayRecords.map(record => record.sectionId),
      sectionTitles: dayRecords.map(record => record.clusterLabel),
      threadCount: peak,
    })
  }
  return nodes
}

export function useOtherTopicObservation(deps: UseOtherTopicObservationDeps) {
  const defaultApi = useOtherTopicObservationApi()
  const fetchers: OtherTopicObservationFetchers = deps.fetchers ?? {
    timeline: defaultApi.loadBoardSectionTimeline,
  }

  /** 板块级单份成功数据（含全部话题记录；换话题只换投影）。 */
  const dataset = shallowRef<ObservationTimelineData | null>(null)
  /** 当前观察目标（由 OtherTopicReading 在选择其他行时写入）。 */
  const topicId = ref<string | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const stale = ref(false)

  let boardEpoch = 0
  let requestSeq = 0
  /** 在途请求（同板块合乘；force 会用新序号超越旧请求）。 */
  let inflight: { epoch: number, seq: number, promise: Promise<void> } | null = null

  const nodes = computed<ObservedDayNode[]>(() => projectObservationDays(dataset.value?.records, {
    topicId: topicId.value,
    axis: deps.axis(),
  }))

  const view = computed<OtherTopicObservationView>(() => {
    const current = topicId.value
    if (current === null) {
      return { topicId: null, status: 'missing-id', error: null, stale: false, refreshing: false, coverage: null, nodes: [] }
    }
    if (dataset.value !== null) {
      return {
        topicId: current,
        status: 'ready',
        error: error.value,
        stale: stale.value,
        refreshing: loading.value,
        coverage: 'all',
        nodes: nodes.value,
      }
    }
    if (loading.value) {
      return { topicId: current, status: 'loading', error: null, stale: false, refreshing: false, coverage: null, nodes: [] }
    }
    if (error.value !== null) {
      return { topicId: current, status: 'error', error: error.value, stale: false, refreshing: false, coverage: null, nodes: [] }
    }
    return { topicId: current, status: 'idle', error: null, stale: false, refreshing: false, coverage: null, nodes: [] }
  })

  async function runRequest(boardId: number, epoch: number, seq: number): Promise<void> {
    // days=0 = 板块全部历史：正数会被后端钳到 90 天，覆盖不了 365 观察池（见 API 边界头注释）。
    const res = await fetchers.timeline(boardId)
    // 切板块/卸载晚到：不写数据/错误/loading（loading 归最新请求所有）。
    if (epoch !== boardEpoch || deps.boardId.value !== boardId || seq !== requestSeq) return
    if (!res.success || !res.data) {
      error.value = res.error || '同归属跨日记录加载失败'
      // 旧成功数据保留可读，并明确标注陈旧；失败不写入成功缓存。
      stale.value = dataset.value !== null
      return
    }
    dataset.value = res.data
    error.value = null
    stale.value = false
  }

  function startRequest(boardId: number): Promise<void> {
    const epoch = boardEpoch
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    const entry: { epoch: number, seq: number, promise: Promise<void> } = {
      epoch,
      seq,
      promise: Promise.resolve(),
    }
    entry.promise = runRequest(boardId, epoch, seq).finally(() => {
      if (inflight === entry) {
        inflight = null
        loading.value = false
      }
    })
    inflight = entry
    return entry.promise
  }

  /**
   * 懒读取入口：topicId 为 null（当次未归属/未选）时不发请求；已有板块级成功数据时本地换投影，
   * 不重复 GET。`force` = 生成完成后刷新/失败重试（必发新请求，旧成功数据在失败时保留）。
   */
  async function ensureObservation(nextTopicId: string | null, options: { force?: boolean } = {}): Promise<void> {
    topicId.value = nextTopicId
    if (nextTopicId === null) return
    const boardId = deps.boardId.value
    if (!options.force && dataset.value !== null) return
    if (!options.force) {
      const shared = inflight
      if (shared !== null && shared.epoch === boardEpoch) {
        await shared.promise
        return
      }
    }
    await startRequest(boardId)
  }

  /** 强制重读当前观察目标（失败重试/生成后刷新）；无目标时 no-op。 */
  async function retry(): Promise<void> {
    await ensureObservation(topicId.value, { force: true })
  }

  /**
   * 懒失效（生成完成/日报重建后调用）：**不发请求**，只丢成功缓存并作废在途响应，让下一次
   * `ensureObservation` 真的重读。没选其他行时调用也不会沿旧 topic 发无谓 GET。
   */
  function invalidate(): void {
    dataset.value = null
    error.value = null
    stale.value = false
    // 在途旧响应一律作废（seq 已变），loading 归后续新请求所有。
    requestSeq += 1
    inflight = null
    loading.value = false
  }

  function resetForBoard(): void {
    boardEpoch += 1
    requestSeq += 1
    inflight = null
    dataset.value = null
    topicId.value = null
    loading.value = false
    error.value = null
    stale.value = false
  }

  watch(deps.boardId, () => {
    resetForBoard()
  })

  if (getCurrentScope()) {
    onScopeDispose(() => {
      resetForBoard()
    })
  }

  return {
    /** 观察带视图（组件渲染入口）。 */
    view: view as ComputedRef<OtherTopicObservationView>,
    /** 懒读取/本地换投影入口。 */
    ensureObservation,
    /** 失败重试/强刷当前目标。 */
    retry,
    /** 生成完成后的懒失效（不 GET；下次 ensure 才重读）。 */
    invalidate,
    /** 切板块/卸载复位（boardId watch 已自动触发，供测试/显式复位）。 */
    resetForBoard,
    /** 诊断：当前板块成功数据是否已取到（不参与渲染口径）。 */
    hasDataset: () => dataset.value !== null,
  }
}

export type OtherTopicObservationStore = ReturnType<typeof useOtherTopicObservation>
