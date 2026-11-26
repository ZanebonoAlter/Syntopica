import { computed, ref, shallowRef } from 'vue'
import { useDailyReportsApi, type KeyEventState, type TopicKeyEventMarker } from '~/api/dailyReports'

// ── 话题关键事件注记共享状态（add-topic-key-event-markers design D5）──────────
//
// 模块级单例缓存：日报阅读器（review/history 挑选）与话题总览（confirmed 回看）
// 共享同一份注记状态，任一入口的写动作即时同步到所有入口（跨入口缓存同步）。
// 缓存按 reportId/topicId 分键——迟到响应只写自己的键，不会串到别的 owner
// （切换泳道/版块不串状态的结构性保证）。
//
// 写动作语义：
// - 每条目请求锁：同一条 thread/marker 同时只允许一个在途动作；
// - revision：已有行变更状态时携带已知最新 revision，409 时采纳服务端返回的
//   最新状态并提示重读重试（不静默改写）；
// - 乐观待保存：动作在途时 UI 显示"待保存"目标态；失败回滚到已知状态；
// - 结果不明（网络异常/5xx）：先重读所属报告注记，再让用户幂等重试。

export type KeyEventPendingAction = 'mark' | 'dismiss' | 'unmark'

export interface ReportKeyEventEntry {
  status: 'idle' | 'running' | 'done' | 'failed'
  markers: TopicKeyEventMarker[]
  loading: boolean
  error: string
}

export interface TopicKeyEventEntry {
  markers: TopicKeyEventMarker[]
  loading: boolean
  error: string
  /** 已拉取的窗口（days=0 表示不限窗，覆盖任意窗口）；窗口变化时重拉。 */
  days: number
}

export interface ThreadKeyEventView {
  /** 该 thread 当前已知注记（null = 无行；含 suggested/confirmed/dismissed）。 */
  marker: TopicKeyEventMarker | null
  /** 在途动作（乐观"待保存"目标态）。 */
  pending: KeyEventPendingAction | null
  /** 本条最近一次动作错误（保存失败/冲突/结果不明提示）。 */
  error: string | null
}

// ── 模块级共享状态（跨入口单例）────────────────────────────────────────────

const reportEntries = ref(new Map<number, ReportKeyEventEntry>())
const topicEntries = ref(new Map<number, TopicKeyEventEntry>())
const pendingByThread = ref(new Map<number, KeyEventPendingAction>())
const pendingByMarker = ref(new Map<number, KeyEventPendingAction>())
const errorsByThread = ref(new Map<number, string>())

/** markers 索引：threadId → 最新注记行（扫描报告缓存构建，写动作即时 upsert）。 */
const markersByThread = computed(() => {
  const index = new Map<number, TopicKeyEventMarker>()
  for (const entry of reportEntries.value.values()) {
    for (const marker of entry.markers) {
      if (marker.source_thread_id != null) index.set(marker.source_thread_id, marker)
    }
  }
  return index
})

const markersById = computed(() => {
  const index = new Map<number, TopicKeyEventMarker>()
  for (const entry of reportEntries.value.values()) {
    for (const marker of entry.markers) index.set(marker.id, marker)
  }
  for (const entry of topicEntries.value.values()) {
    for (const marker of entry.markers) if (!index.has(marker.id)) index.set(marker.id, marker)
  }
  return index
})

function upsertMarker(marker: TopicKeyEventMarker) {
  // 报告键索引：写动作与读取共用同一份列表；无既有条目时就地建档，
  // 避免「先标记后拉取」的顺序下写动作丢失（本地幂等短路依赖这份缓存）。
  if (marker.source_report_id != null) {
    const reportId = marker.source_report_id
    let entry = reportEntries.value.get(reportId)
    if (!entry) {
      entry = { status: 'idle', markers: [], loading: false, error: '' }
      reportEntries.value.set(reportId, entry)
    }
    const idx = entry.markers.findIndex(m => m.id === marker.id)
    if (idx >= 0) entry.markers.splice(idx, 1, marker)
    else entry.markers.push(marker)
    reportEntries.value = new Map(reportEntries.value)
  }
  // 话题键（confirmed 总览投影）：dismissed 的节点退出 confirmed 列表。
  if (marker.persistent_topic_id != null) {
    const topicEntry = topicEntries.value.get(marker.persistent_topic_id)
    if (topicEntry) {
      const idx = topicEntry.markers.findIndex(m => m.id === marker.id)
      if (marker.state === 'confirmed') {
        if (idx >= 0) topicEntry.markers.splice(idx, 1, marker)
        else topicEntry.markers.push(marker)
      } else if (idx >= 0) {
        topicEntry.markers.splice(idx, 1)
      }
      topicEntries.value = new Map(topicEntries.value)
    }
  }
}

function reportIdForThread(threadId: number): number | null {
  const marker = markersByThread.value.get(threadId)
  return marker?.source_report_id ?? null
}

export function useTopicKeyEvents() {
  const { getReportKeyEvents, getTopicKeyEvents, putThreadKeyEvent, patchKeyEventMarker } = useDailyReportsApi()

  /** 日报读端：当期来源映射及推荐状态（打开页面只读，不触发推荐）。 */
  function reportEntry(reportId: number): ReportKeyEventEntry {
    return reportEntries.value.get(reportId) ?? { status: 'idle', markers: [], loading: false, error: '' }
  }

  /** 总览读端：话题已确认节点（topic 读端只拿 confirmed；未确认不进此缓存）。 */
  function topicEntry(topicId: number): TopicKeyEventEntry {
    return topicEntries.value.get(topicId) ?? { markers: [], loading: false, error: '', days: -1 }
  }

  async function ensureReportKeyEvents(reportId: number, retry = false) {
    const existing = reportEntries.value.get(reportId)
    if (existing && existing.loading) return
    if (existing && !retry && (existing.markers.length > 0 || existing.status !== 'idle')) return
    const entry: ReportKeyEventEntry = { ...(existing ?? { status: 'idle', markers: [], error: '' }), loading: true, error: '' }
    reportEntries.value.set(reportId, entry)
    reportEntries.value = new Map(reportEntries.value)
    const response = await Promise.resolve(getReportKeyEvents(reportId)).catch(() => null)
    // 迟到守卫：仅当该 reportId 仍是当前键（未被清除）时写回。
    const current = reportEntries.value.get(reportId)
    if (!current || !current.loading) return
    if (response?.success && response.data) {
      current.status = response.data.status ?? 'idle'
      current.markers = response.data.markers ?? []
    } else {
      current.error = response?.error || '关键事件加载失败'
    }
    current.loading = false
    reportEntries.value = new Map(reportEntries.value)
  }

  async function ensureTopicKeyEvents(topicId: number, days = 14, retry = false) {
    const existing = topicEntries.value.get(topicId)
    if (existing && existing.loading) return
    // 窗口语义：days=0（不限窗）覆盖一切窗口；同窗口且无错误才短路径，
    // 切换 days（如 7→30）必须重拉，旧窗口外的 confirmed 不能缺。
    if (existing && !retry && existing.days === 0) return
    if (existing && !retry && existing.days === days && existing.error === '') return
    const entry: TopicKeyEventEntry = { markers: existing?.markers ?? [], loading: true, error: '', days }
    topicEntries.value.set(topicId, entry)
    topicEntries.value = new Map(topicEntries.value)
    const response = await Promise.resolve(getTopicKeyEvents(topicId, days)).catch(() => null)
    const current = topicEntries.value.get(topicId)
    if (!current || !current.loading) return
    if (response?.success && response.data) {
      // 话题读端只留 confirmed：服务端契约即 confirmed-only，前端再兜底一层，
      // 确保建议态永远不会泄漏进总览/焦点回看（spec：未确认按原新闻）。
      current.markers = (response.data.markers ?? []).filter(m => m.state === 'confirmed')
    } else {
      current.error = response?.error || '已确认节点加载失败'
    }
    current.loading = false
    topicEntries.value = new Map(topicEntries.value)
  }

  function threadView(threadId: number): ThreadKeyEventView {
    return {
      marker: markersByThread.value.get(threadId) ?? null,
      pending: pendingByThread.value.get(threadId) ?? null,
      error: errorsByThread.value.get(threadId) ?? null,
    }
  }

  function markerView(markerId: number): ThreadKeyEventView {
    const marker = markersById.value.get(markerId) ?? null
    const threadId = marker?.source_thread_id
    // detached 快照（thread 已断）无 thread 键：错误挂在 -markerId 负键上。
    const errorKey = threadId ?? -markerId
    return {
      marker,
      pending: marker ? pendingByMarker.value.get(marker.id) ?? (threadId != null ? pendingByThread.value.get(threadId) ?? null : null) : null,
      error: errorsByThread.value.get(errorKey) ?? null,
    }
  }

  function setThreadError(threadId: number, message: string | null) {
    if (message == null) errorsByThread.value.delete(threadId)
    else errorsByThread.value.set(threadId, message)
    errorsByThread.value = new Map(errorsByThread.value)
  }

  /** 结果不明时重读：恢复该 thread 所属报告的服务端真状态。 */
  async function rereadAfterUnknown(threadId: number) {
    const reportId = reportIdForThread(threadId)
    if (reportId != null) await ensureReportKeyEvents(reportId, true)
  }

  /** thread 寻址动作（PUT）：确认建议/手动标记/不采纳。 */
  async function actThread(topicId: number, threadId: number, action: 'mark' | 'dismiss') {
    if (pendingByThread.value.has(threadId)) return // 每条请求锁
    const known = markersByThread.value.get(threadId) ?? null
    const target: KeyEventState = action === 'mark' ? 'confirmed' : 'dismissed'
    // 同目标重复提交：本地直接采纳已知终态（幂等，服务端同语义；省一次往返）。
    if (known != null && known.state === target) return
    const needsRevision = known != null && known.state !== target
    setThreadError(threadId, null)
    pendingByThread.value.set(threadId, action)
    pendingByThread.value = new Map(pendingByThread.value)
    try {
      const response = await putThreadKeyEvent(topicId, threadId, action, needsRevision ? known?.revision : undefined)
      if (response.success && response.data) {
        upsertMarker(response.data)
      } else if (response.status === 409 && response.data) {
        // 采纳服务端最新状态（幂等重试的基线），提示用户内容已更新。
        upsertMarker(response.data as TopicKeyEventMarker)
        setThreadError(threadId, '该条内容已被更新，已同步最新状态，请重试')
      } else if (response.status == null || response.status >= 500) {
        // 结果不明：重读真状态，已知状态保持（回滚乐观态）。
        await rereadAfterUnknown(threadId)
        setThreadError(threadId, '保存结果未知，已重新读取最新状态，请重试')
      } else {
        setThreadError(threadId, response.error || '保存失败，请重试')
      }
    } catch {
      await rereadAfterUnknown(threadId)
      setThreadError(threadId, '保存结果未知，已重新读取最新状态，请重试')
    } finally {
      pendingByThread.value.delete(threadId)
      pendingByThread.value = new Map(pendingByThread.value)
    }
  }

  /** marker 寻址维护（PATCH）：取消已确认（含失效快照）/ 重新标记。 */
  async function actMarker(topicId: number, markerId: number, action: 'mark' | 'dismiss' | 'unmark') {
    if (pendingByMarker.value.has(markerId)) return
    const known = markersById.value.get(markerId)
    if (!known) return
    const target: KeyEventState = action === 'mark' ? 'confirmed' : 'dismissed'
    if (known.state === target) return // 同目标幂等
    const threadId = known.source_thread_id
    const errorKey = threadId ?? -markerId
    setThreadError(errorKey, null)
    pendingByMarker.value.set(markerId, action)
    pendingByMarker.value = new Map(pendingByMarker.value)
    try {
      const response = await patchKeyEventMarker(topicId, markerId, action, known.revision)
      if (response.success && response.data) {
        upsertMarker(response.data)
      } else if (response.status === 409 && response.data) {
        upsertMarker(response.data as TopicKeyEventMarker)
        setThreadError(errorKey, '该条内容已被更新，已同步最新状态，请重试')
      } else if (response.status == null || response.status >= 500) {
        await rereadAfterUnknown(errorKey > 0 ? errorKey : -1)
        setThreadError(errorKey, '保存结果未知，已重新读取最新状态，请重试')
      } else {
        setThreadError(errorKey, response.error || '保存失败，请重试')
      }
    } catch {
      await rereadAfterUnknown(threadId ?? -1)
      setThreadError(errorKey, '保存结果未知，已重新读取最新状态，请重试')
    } finally {
      pendingByMarker.value.delete(markerId)
      pendingByMarker.value = new Map(pendingByMarker.value)
    }
  }

  /** 版块/泳道切换：清局部错误与在途标记的展示态（缓存数据按 owner 分键保留，
   *  合法在途人工写仍按发起 owner 完成并落缓存）。 */
  /** 版块切换/隔离场景：清空全部本地缓存（后续 ensure 重新拉取，保证新鲜度）。 */
  function resetLocal() {
    errorsByThread.value = new Map()
    pendingByThread.value = new Map()
    pendingByMarker.value = new Map()
    reportEntries.value = new Map()
    topicEntries.value = new Map()
  }

  return {
    reportEntries,
    topicEntries,
    reportEntry,
    topicEntry,
    ensureReportKeyEvents,
    ensureTopicKeyEvents,
    threadView,
    markerView,
    actThread,
    actMarker,
    resetLocal,
    // 测试/调试内省口（只读）。
    _state: shallowRef({ markersByThread, markersById }),
  }
}
