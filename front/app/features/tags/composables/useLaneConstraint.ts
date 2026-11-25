import { computed, ref } from 'vue'
import {
  useDailyReportsApi,
  type TopicLaneConstraint,
  type TopicLaneConstraintInput,
} from '~/api/dailyReports'

/**
 * 泳道修正工作面 · 辅助标签约束（topic-lane-auxiliary-label-constraint 第 6 组）。
 *
 * 契约（见 change 的 ui-design.md）：
 * - 约束是**话题级持久设置**，逐条即时落库（与「加入/移出/定首义/改名」同口径），
 *   **没有全局保存按钮**；生效时机由块头徽标「次日日报生效」承担。
 * - 后端 PUT 是**全量覆盖**语义，所以每次增删都要把完整集合提交一次；
 *   幂等由后端保证（相同请求重提结果相同）。
 * - 锚向量计算失败时后端**整体不落库**：此时保留用户输入、给出可重试错误，
 *   **不静默丢弃**（用户重打一遍短词的代价最高）。
 * - 短词单话题内唯一：重复时在前端就拦下并提示，不发请求（后端同样会 400）。
 */

export type LaneConstraintPhase = 'idle' | 'loading' | 'ready' | 'error'

export interface LaneConstraintRow {
  term: string
  mode: 'required' | 'preferred'
  /** 计算中的临时行（后有 anchor 返回前不可再操作）。 */
  computing: boolean
  /** 该行最近一次失败原因（保留 term 供重试）。 */
  error: string | null
}

const TERM_MAX_LENGTH = 100

export function useLaneConstraint() {
  const api = useDailyReportsApi()

  const topicId = ref<number | null>(null)
  const phase = ref<LaneConstraintPhase>('idle')
  const loadError = ref<string | null>(null)
  const rows = ref<LaneConstraintRow[]>([])
  /** 块内列表展开态（默认折叠：左栏已有 4 块，常态不再加高）。 */
  const expanded = ref(false)
  /** 提交在途（防重复提交，按钮 loading）。 */
  const saving = ref(false)
  /** 最近一次操作级反馈（重复短词 / 保存失败）。 */
  const actionError = ref<string | null>(null)

  const count = computed(() => rows.value.length)
  const isEmpty = computed(() => phase.value === 'ready' && rows.value.length === 0)
  const hasComputing = computed(() => rows.value.some(row => row.computing))

  /**
   * 草稿模式（新建泳道）：不拉数据、不落库，行集由父层在保存时随建话题提交。
   * 打开后 `phase` 直接进 ready，让空态/列表正常渲染。
   */
  const draft = ref(false)
  function enableDraft(initial: LaneConstraintRow[] = []) {
    draft.value = true
    rows.value = initial
    phase.value = 'ready'
    loadError.value = null
  }

  function toRows(list: TopicLaneConstraint[]): LaneConstraintRow[] {
    return list.map(item => ({
      term: item.term,
      mode: item.mode,
      computing: false,
      error: null,
    }))
  }

  /** 把当前行集还原成 PUT 载荷（computing/error 行不带 anchor，交给后端重算）。 */
  function toPayload(list: LaneConstraintRow[]): TopicLaneConstraintInput[] {
    return list.map(row => ({ term: row.term, mode: row.mode }))
  }

  async function load(id: number) {
    if (draft.value) return
    topicId.value = id
    phase.value = 'loading'
    loadError.value = null
    try {
      const res = await api.getTopicLaneConstraints(id)
      rows.value = toRows(res.data?.constraints ?? [])
      phase.value = 'ready'
    }
    catch (err) {
      phase.value = 'error'
      loadError.value = err instanceof Error ? err.message : String(err)
    }
  }

  async function retryLoad() {
    if (topicId.value == null) return
    await load(topicId.value)
  }

  /**
   * 全量提交一次。`next` 是提交后的目标行集；失败时回滚到提交前。
   * 成功时用服务端返回的行集覆盖本地（锚向量归一后的权威结果）。
   */
  async function commit(next: LaneConstraintRow[]): Promise<boolean> {
    // 草稿模式：只改本地行集（建话题时一并提交），不发请求也不模拟锚计算。
    if (draft.value) {
      actionError.value = null
      rows.value = next.map(row => ({ ...row, computing: false, error: null }))
      return true
    }
    if (topicId.value == null) return false
    const previous = rows.value
    saving.value = true
    actionError.value = null
    rows.value = next
    try {
      const res = await api.putTopicLaneConstraints(topicId.value, toPayload(next))
      rows.value = toRows(res.data?.constraints ?? [])
      return true
    }
    catch (err) {
      rows.value = previous
      actionError.value = err instanceof Error ? err.message : String(err)
      return false
    }
    finally {
      saving.value = false
    }
  }

  /** 新增一条约束（短词 trim + 唯一性 + 长度校验都在这里拦）。 */
  async function add(rawTerm: string, mode: 'required' | 'preferred' = 'required'): Promise<boolean> {
    const term = rawTerm.trim().slice(0, TERM_MAX_LENGTH)
    if (!term) return false
    if (rows.value.some(row => row.term === term)) {
      actionError.value = '该辅助标签已存在'
      return false
    }
    return commit([...rows.value, { term, mode, computing: true, error: null }])
  }

  /** 删除一条约束（省略即删除）。 */
  async function remove(term: string): Promise<boolean> {
    const next = rows.value.filter(row => row.term !== term)
    if (next.length === rows.value.length) return false
    return commit(next)
  }

  /** 切换某条的 required / preferred（不重算锚向量：term 未变，后端复用已落库锚）。 */
  async function setMode(term: string, mode: 'required' | 'preferred'): Promise<boolean> {
    let changed = false
    const next = rows.value.map((row) => {
      if (row.term !== term || row.mode === mode) return row
      changed = true
      return { ...row, mode }
    })
    if (!changed) return false
    return commit(next)
  }

  function toggleExpanded() {
    expanded.value = !expanded.value
  }

  function reset() {
    topicId.value = null
    phase.value = 'idle'
    loadError.value = null
    rows.value = []
    expanded.value = false
    saving.value = false
    actionError.value = null
  }

  return {
    topicId,
    phase,
    loadError,
    rows,
    count,
    isEmpty,
    expanded,
    saving,
    hasComputing,
    actionError,
    draft,
    enableDraft,
    load,
    retryLoad,
    add,
    remove,
    setMode,
    toggleExpanded,
    reset,
  }
}
