/**
 * 新建泳道编排态 · 状态机（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 职责：取数窗口（快捷 7/14/30/90 + 自选起止）、候选链（服务端唯一实现，前端只渲染）、
 * 本次已选集合、查看焦点、质心推荐（每次增删重算）、命名（自动名跟随来源组 / 人工名永不被覆盖）、
 * 草稿确认（有成员或名字时切窗口先确认）、日期非法保留旧范围与草稿。
 *
 * 口径来源：`lane-compose-implementation-contract.md`（唯一口径）+ delta spec
 * `specs/workshop-lane-compose/spec.md`。纯计算在 `laneComposeLogic.ts`（原型逐字移植）。
 *
 * 纪律（红线）：
 * - 不自动改话题状态、不自动归档、不伪造关系边；推荐不是置信度；
 * - 质心是**前端草稿计算**，不落库（人工建话题的初始向量由服务端按所选成员均值写入）；
 * - 归档来源只显示不复活。
 *
 * 历史待启用话题（管理名单）：名单来自既有端点 `GET /semantic-boards/:id/topics`
 * （服务端按累计命中门槛过滤可见性，前端**不放宽**）；「带入草稿」只带该话题在当前窗口内
 * 的 owned 成员（不补相似、不截条）；「直接启用原话题」资格沿服务端 `can_activate`，
 * 走既有 `updateTopic(status='active')`——**不重算质心、不改命中计数与状态机规则**。
 *
 * 与旧 `useInlineCompose` 的关系：旧 composable 的「相似补选 + 20 条上限」语义**保留原处**
 * （线程浏览器仍在用），编排态不复用；本文件是编排态的**另建**状态机。
 */
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import type { BoardTopicListItem } from '~/api/dailyReports'
import { usePersistentTopicsApi } from '~/api/persistentTopics'
import type {
  ComposeCandidate,
  ComposeCandidateChain,
  ComposeCandidatesQuery,
  ComposeRelation,
  ComposeWindow,
  ManualTopicResult,
} from '~/api/persistentTopics'
import {
  candidateSelection,
  centroidRecommendations,
  laneSelectionState,
  type LaneRecommendations,
  type LaneSelectionState,
} from './laneComposeLogic'

/** 取数窗口档位：快捷 7/14/30/90 或自选起止。 */
export type LaneWindowMode = 7 | 14 | 30 | 90 | 'custom'

/** 生效中的窗口（服务端回显为准，前端不自推末日）。 */
export interface LaneWindowState {
  mode: LaneWindowMode
  from: string
  to: string
  days: number
}

/** 草稿确认请求（宿主用弹窗承接，返回 true = 确认执行）。 */
export interface LaneComposeConfirmRequest {
  kind: 'window' | 'discard'
  title: string
  message: string
  confirmText: string
  cancelText: string
}

/**
 * 历史管理名单条目（服务端 snake_case 已在**本层**归一为 camelCase，组件层不再映射）。
 *
 * 可见性沿服务端：名单端点已按累计命中门槛过滤「未达门槛的 candidate」，
 * 前端不得放宽；`canActivate` 是服务端给的启用资格，前端不得自造门槛。
 */
export interface LaneHistoryTopic {
  /** 字符串化 id（与 sections 的 persistentTopicId 同口径，便于比对）。 */
  id: string
  label: string
  status: string
  /** 累计命中次数（服务端 hit_count）。 */
  hitCount: number
  /** 连续命中次数（服务端 consecutive_hits）。 */
  consecutiveHits: number
  /** 最近出现日期（服务端 last_seen_date）。 */
  lastSeenDate: string
  /** 关联 section 数（服务端 section_count）。 */
  sectionCount: number
  /** 服务端给的启用资格（沿服务端 can_activate）。 */
  canActivate: boolean
  /** 不可直接启用的原因（null = 可启用）；文案按服务端事实给，不自造门槛。 */
  activateBlockReason: string | null
}

/** 「带入草稿」的结果（供宿主给明确反馈）。 */
export interface LaneHistoryAdoptOutcome {
  topicId: string
  topicLabel: string
  /** 窗口内该话题的 owned 成员数。 */
  ownedCount: number
  /** 本次新加入草稿的成员数。 */
  addedCount: number
  /** 用户手写过名字 → 保留手写名、未覆盖。 */
  keptManualName: boolean
  /** 本次是否预填了自动名。 */
  prefilledName: boolean
}

/** 「直接启用原话题」的结果。 */
export interface LaneHistoryActivateOutcome {
  ok: boolean
  message: string
}

export interface UseLaneComposeOptions {
  boardId: Ref<number | string>
  /** 草稿确认（切窗口 / 取消本次）；返回 true = 确认执行，false = 取消并恢复原控件与草稿。 */
  requestConfirm: (request: LaneComposeConfirmRequest) => Promise<boolean>
  /** 保存成功回调（宿主负责 emit saved / close）。 */
  onSaved?: (topic?: ManualTopicResult) => void | Promise<void>
}

/** 契约 §1.1：自选范围最多 90 天（含首末）→ 首末日差 ≤ 89。 */
const MAX_WINDOW_SPAN_DAYS = 89
/** 契约 §1.1：快捷档默认 30 天。 */
const DEFAULT_WINDOW_DAYS = 30
/** 服务端未回传 match_threshold 时的兜底阈值。 */
const DEFAULT_MATCH_THRESHOLD = 0.3
const DAY_MS = 86_400_000

function parseUtcDay(value: string): number {
  return Date.parse(`${value}T00:00:00Z`)
}

function formatUtcDay(ts: number): string {
  return new Date(ts).toISOString().slice(0, 10)
}

export function useLaneCompose(opts: UseLaneComposeOptions) {
  const { getComposeCandidates, createManualLane } = usePersistentTopicsApi()
  const { listBoardTopics, updateTopic } = useDailyReportsApi()

  // ── 取数状态 ─────────────────────────────────────────────────────────────
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)
  const dateError = ref<string | null>(null)
  const nameError = ref<string | null>(null)

  const sections = ref<ComposeCandidate[]>([])
  const relations = ref<ComposeRelation[]>([])
  const candidateChains = ref<ComposeCandidateChain[]>([])
  const matchThreshold = ref(DEFAULT_MATCH_THRESHOLD)
  const serverWindow = ref<ComposeWindow | null>(null)

  // ── 窗口控件态（与生效窗口分离，取消时回滚控件） ─────────────────────────
  const appliedWindow = ref<LaneWindowState>({ mode: DEFAULT_WINDOW_DAYS, from: '', to: '', days: DEFAULT_WINDOW_DAYS })
  const controlMode = ref<LaneWindowMode>(DEFAULT_WINDOW_DAYS)
  const controlFrom = ref('')
  const controlTo = ref('')

  // ── 草稿状态 ─────────────────────────────────────────────────────────────
  const selectedIds = ref<Set<string>>(new Set())
  const focusId = ref<string | null>(null)
  const laneName = ref('')
  const nameManuallyEdited = ref(false)
  /** 自动预填名的来源键：`lane:{chainKey}`（候选链来源）/ `history:{topicId}`（历史名单带入来源）。 */
  const autoNameKey = ref<string | null>(null)
  /** 已有 active 泳道只读参照开关（默认隐藏；仅用户显式开启才显示）。 */
  const showAssigned = ref(false)

  // ── 历史管理名单（待启用话题） ───────────────────────────────────────────
  const historyTopics = ref<LaneHistoryTopic[]>([])
  const historyLoading = ref(false)
  const historyError = ref<string | null>(null)
  const historyQuery = ref('')
  /** 正在「直接启用」的 topicId（按钮 loading 用）。 */
  const activatingTopicId = ref<string | null>(null)

  // ── 派生 ─────────────────────────────────────────────────────────────────
  const sectionsById = computed(() => {
    const map = new Map<string, ComposeCandidate>()
    for (const section of sections.value) map.set(section.id, section)
    return map
  })

  /** 候选链成员（按链内顺序解析；窗口外的成员忽略）。 */
  function chainMembers(chain: ComposeCandidateChain): ComposeCandidate[] {
    const out: ComposeCandidate[] = []
    for (const id of chain.sectionIds) {
      const section = sectionsById.value.get(id)
      if (section) out.push(section)
    }
    return out
  }

  /** 每条候选链的选择三态（unchecked / indeterminate / checked）。 */
  const chainStates = computed<Record<string, LaneSelectionState>>(() => {
    const out: Record<string, LaneSelectionState> = {}
    for (const chain of candidateChains.value) {
      out[chain.key] = laneSelectionState(selectedIds.value, chain.sectionIds)
    }
    return out
  })

  /** 本次已选成员（按窗口内原始顺序）。 */
  const selectedSections = computed(() => sections.value.filter(s => selectedIds.value.has(s.id)))
  const memberCount = computed(() => selectedIds.value.size)
  const hasDraft = computed(() => selectedIds.value.size > 0 || laneName.value.trim() !== '')

  /** 质心推荐（每次增删自动重算；焦点不参与质心）。 */
  const recommendations: ComputedRef<LaneRecommendations<ComposeCandidate>> = computed(() =>
    centroidRecommendations(sections.value, selectedIds.value, matchThreshold.value),
  )

  /** 焦点 section（只读线索用；右栏不重复放选入按钮）。 */
  const focusSection = computed<ComposeCandidate | null>(() =>
    focusId.value ? sectionsById.value.get(focusId.value) ?? null : null,
  )

  /** 历史名单按话题名过滤（纯前端，不调 AI、不调 embed）。 */
  const filteredHistoryTopics = computed<LaneHistoryTopic[]>(() => {
    const q = historyQuery.value.trim().toLowerCase()
    if (!q) return historyTopics.value
    return historyTopics.value.filter(t => t.label.toLowerCase().includes(q))
  })

  /** 窗口内日历日（画布列）。 */
  const dates = computed<string[]>(() => {
    const from = serverWindow.value?.from
    const to = serverWindow.value?.to
    if (!from || !to) return []
    const start = parseUtcDay(from)
    const end = parseUtcDay(to)
    if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return []
    const out: string[] = []
    for (let ts = start; ts <= end && out.length <= 400; ts += DAY_MS) out.push(formatUtcDay(ts))
    return out
  })

  const windowLabel = computed(() => {
    const w = serverWindow.value
    if (!w?.from || !w?.to) return ''
    return `${w.from} — ${w.to}`
  })

  // ── 取数 ─────────────────────────────────────────────────────────────────
  async function loadWindow(query: ComposeCandidatesQuery & { mode: LaneWindowMode }): Promise<boolean> {
    loading.value = true
    error.value = null
    try {
      const res = await getComposeCandidates(opts.boardId.value, query)
      if (!res.success || !res.data) {
        error.value = res.error || '加载候选失败'
        return false
      }
      sections.value = res.data.sections
      relations.value = res.data.relations
      candidateChains.value = res.data.candidateChains
      matchThreshold.value = res.data.matchThreshold
      serverWindow.value = res.data.window
      appliedWindow.value = {
        mode: query.mode,
        from: res.data.window?.from ?? '',
        to: res.data.window?.to ?? '',
        days: res.data.window?.days ?? query.days ?? DEFAULT_WINDOW_DAYS,
      }
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载候选失败'
      return false
    } finally {
      loading.value = false
    }
  }

  /** 初始装载：近 30 天。 */
  async function load(): Promise<void> {
    const ok = await loadWindow({ mode: DEFAULT_WINDOW_DAYS, days: DEFAULT_WINDOW_DAYS })
    if (ok) {
      controlMode.value = DEFAULT_WINDOW_DAYS
      controlFrom.value = appliedWindow.value.from
      controlTo.value = appliedWindow.value.to
    }
  }

  // ── 窗口切换 ─────────────────────────────────────────────────────────────
  function restoreDateControls(): void {
    controlFrom.value = appliedWindow.value.from
    controlTo.value = appliedWindow.value.to
  }

  /** 控件回滚：快捷档回滚档位与日期；自选档只回滚日期（用户仍在自选模式里改）。 */
  function restoreControls(mode: LaneWindowMode): void {
    if (mode !== 'custom') controlMode.value = appliedWindow.value.mode
    restoreDateControls()
  }

  /** 自选起止校验：与契约 §1.1 同口径（非法一律报错，不静默纠正、不截断）。 */
  function validateCustomRange(from: string, to: string): string | null {
    const fmt = /^\d{4}-\d{2}-\d{2}$/
    if (!fmt.test(from) || !fmt.test(to)) return '请选择有效的起止日期'
    const fromTs = parseUtcDay(from)
    const toTs = parseUtcDay(to)
    if (!Number.isFinite(fromTs) || !Number.isFinite(toTs)) return '请选择有效的起止日期'
    if (formatUtcDay(fromTs) !== from || formatUtcDay(toTs) !== to) return '请选择有效的起止日期'
    if (from > to) return '开始日期不能晚于结束日期'
    if (Math.round((toTs - fromTs) / DAY_MS) > MAX_WINDOW_SPAN_DAYS) {
      return `日期范围不能超过 ${MAX_WINDOW_SPAN_DAYS + 1} 天`
    }
    const latest = serverWindow.value?.latestCompletedDate
    if (latest) {
      const latestTs = parseUtcDay(latest)
      if (Number.isFinite(latestTs)) {
        const earliest = latestTs - MAX_WINDOW_SPAN_DAYS * DAY_MS
        if (fromTs < earliest) {
          return `开始日期早于支持的取数窗（${formatUtcDay(earliest)} 起）`
        }
      }
    }
    return null
  }

  /** 清草稿：成员、名称、焦点全清（切窗口确认后才走这里）。 */
  function clearDraft(): void {
    selectedIds.value = new Set()
    laneName.value = ''
    nameManuallyEdited.value = false
    autoNameKey.value = null
    focusId.value = null
  }

  /** 切窗口（有草稿先确认；失败保留旧范围与草稿）。 */
  async function switchWindow(query: ComposeCandidatesQuery & { mode: LaneWindowMode }): Promise<boolean> {
    dateError.value = null
    if (hasDraft.value) {
      const ok = await opts.requestConfirm({
        kind: 'window',
        title: '切换日期范围',
        message: '切换会清空本次选择和名称，按新范围重算候选与质心推荐；不会修改任何历史数据。',
        confirmText: '清空草稿并切换',
        cancelText: '保留当前范围',
      })
      if (!ok) {
        restoreControls(query.mode)
        return false
      }
    }
    const loaded = await loadWindow(query)
    if (!loaded) {
      // 取数失败：保留旧范围与草稿，控件恢复原值
      restoreControls(query.mode)
      return false
    }
    clearDraft()
    controlMode.value = query.mode
    controlFrom.value = appliedWindow.value.from
    controlTo.value = appliedWindow.value.to
    return true
  }

  /** 快捷档切换（7/14/30/90）。 */
  async function setQuickDays(days: 7 | 14 | 30 | 90): Promise<void> {
    controlMode.value = days
    await switchWindow({ mode: days, days })
  }

  /** 切到自选起止：把当前生效范围填进控件，等用户改完点「应用」。 */
  function startCustomRange(): void {
    controlMode.value = 'custom'
    controlFrom.value = appliedWindow.value.from
    controlTo.value = appliedWindow.value.to
  }

  /** 应用自选起止（非法 → 报错 + 保留旧范围与草稿 + 控件恢复原值）。 */
  async function applyCustomRange(): Promise<void> {
    dateError.value = null
    const from = controlFrom.value.trim()
    const to = controlTo.value.trim()
    const problem = validateCustomRange(from, to)
    if (problem) {
      dateError.value = problem
      restoreControls('custom')
      return
    }
    if (from === appliedWindow.value.from && to === appliedWindow.value.to) {
      appliedWindow.value = { ...appliedWindow.value, mode: 'custom', from, to }
      return
    }
    await switchWindow({ mode: 'custom', from, to })
  }

  // ── 选择 ─────────────────────────────────────────────────────────────────
  /** 自动预填名跟随来源组：该组被取消且名字未被人工改写时一并清空。 */
  function syncAutoName(): void {
    if (!autoNameKey.value || nameManuallyEdited.value) return
    const key = autoNameKey.value
    if (key.startsWith('lane:')) {
      const chain = candidateChains.value.find(c => c.key === key.slice(5))
      const stillHas = chain ? chain.sectionIds.some(id => selectedIds.value.has(id)) : false
      if (!chain || !stillHas) {
        laneName.value = ''
        autoNameKey.value = null
      }
      return
    }
    if (key.startsWith('history:')) {
      // 历史来源：该话题在窗口内的 owned 成员被全部取消 → 自动名一并清空（人工改写过则保留）
      const topicId = key.slice('history:'.length)
      const stillHas = sections.value.some(s => s.persistentTopicId === topicId && selectedIds.value.has(s.id))
      if (!stillHas) {
        laneName.value = ''
        autoNameKey.value = null
      }
    }
  }

  /** 整组勾选 / 取消：追加去重、不截条数、不自动补相似、不吞组外人工补选。 */
  function setChainSelected(key: string, checked: boolean): void {
    const chain = candidateChains.value.find(c => c.key === key)
    if (!chain) return
    selectedIds.value = candidateSelection(selectedIds.value, chain.sectionIds, checked)
    if (checked && !laneName.value.trim()) {
      laneName.value = chain.topicLabel
      nameManuallyEdited.value = false
      autoNameKey.value = `lane:${key}`
    }
    syncAutoName()
  }

  /** 点候选标题：未全选 → 一次补齐该片段全部成员；已全选 → 取消该组。 */
  function toggleChain(key: string): void {
    const state = chainStates.value[key]
    if (!state) return
    setChainSelected(key, !state.checked)
  }

  /** 取消整组（只撤该组，组外人工补选保留）。 */
  function cancelChain(key: string): void {
    setChainSelected(key, false)
  }

  /** 单成员勾选 / 取消。 */
  function toggleMember(id: string, checked: boolean): void {
    const next = new Set(selectedIds.value)
    if (checked) next.add(id)
    else next.delete(id)
    selectedIds.value = next
    if (next.size === 0 && !nameManuallyEdited.value) {
      laneName.value = ''
      autoNameKey.value = null
    } else {
      syncAutoName()
    }
  }

  /** 清空已选：成员全清，**手写名保留**。 */
  function clearSelection(): void {
    selectedIds.value = new Set()
    if (!nameManuallyEdited.value) {
      laneName.value = ''
      autoNameKey.value = null
    }
    focusId.value = null
  }

  // ── 焦点 ─────────────────────────────────────────────────────────────────
  function setFocus(id: string | null): void {
    focusId.value = id
  }

  /** 再点同一节点 = 取消选中（焦点清空）。 */
  function toggleFocus(id: string): void {
    focusId.value = focusId.value === id ? null : id
  }

  function clearFocus(): void {
    focusId.value = null
  }

  // ── 命名 ─────────────────────────────────────────────────────────────────
  /** 人工改名：此后任何预填都不得覆盖。 */
  function setLaneName(value: string): void {
    laneName.value = value
    nameManuallyEdited.value = true
    nameError.value = null
  }

  // ── 历史管理名单（待启用话题） ───────────────────────────────────────────
  /**
   * 服务端名单条目 → 前端 camelCase 结构（**转换只在本层做**，组件层不再映射）。
   *
   * 启用资格沿服务端 `can_activate`；前端**不自造门槛、不放宽可见性**
   * （未达门槛的 candidate 由服务端过滤掉，不在本界面出现）。
   */
  function toHistoryTopic(item: BoardTopicListItem): LaneHistoryTopic {
    const canActivate = Boolean(item.can_activate)
    return {
      id: String(item.id),
      label: item.label,
      status: item.status,
      hitCount: item.hit_count ?? 0,
      consecutiveHits: item.consecutive_hits ?? 0,
      lastSeenDate: item.last_seen_date ?? '',
      sectionCount: item.section_count ?? 0,
      canActivate,
      activateBlockReason: canActivate
        ? null
        : item.status === 'active'
          ? '已是启用状态，无需再次启用'
          : item.status === 'archived'
            ? '已归档，不能直接启用'
            : '累计命中未达管理门槛，暂不能直接启用',
    }
  }

  /** 拉取历史管理名单（打开入口时调用；失败保留旧列表 + 报错）。 */
  async function loadHistoryTopics(): Promise<boolean> {
    historyLoading.value = true
    historyError.value = null
    try {
      const res = await listBoardTopics(Number(opts.boardId.value))
      if (!res.success || !res.data) {
        historyError.value = res.error || '加载历史待启用话题失败'
        return false
      }
      historyTopics.value = (res.data.topics ?? []).map(toHistoryTopic)
      return true
    } catch (e) {
      historyError.value = e instanceof Error ? e.message : '加载历史待启用话题失败'
      return false
    } finally {
      historyLoading.value = false
    }
  }

  /**
   * 「带入草稿」：把该话题在**当前窗口内**的 owned 成员加入本次草稿。
   *
   * 口径（契约 §2.4，与候选整组一致）：
   * - 成员 = sections 里 `persistent_topic_id === 该话题 id` 的项（**只带 owned**）：
   *   不自动补相似推荐、**不截 20 条**（旧 `useInlineCompose.adopt` 的相似补选语义保留在原处，本编排态不复用）；
   * - 名称：用户**没手写过** → 预填话题名并把自动名绑定到 `history:<topicId>`；
   *   用户手写过 → **保留手写名、不覆盖**；
   * - 后续该来源的 owned 成员被全部取消且名字未被人工改写时，自动名一并清空。
   */
  function adoptHistoryTopic(topicId: string): LaneHistoryAdoptOutcome | null {
    const topic = historyTopics.value.find(t => t.id === topicId)
    if (!topic) return null
    const owned = sections.value.filter(s => s.persistentTopicId === topic.id)
    const added = owned.filter(s => !selectedIds.value.has(s.id)).map(s => s.id)
    if (added.length > 0) {
      const next = new Set(selectedIds.value)
      for (const id of added) next.add(id)
      selectedIds.value = next
    }
    const keptManualName = nameManuallyEdited.value
    const prefilledName = !keptManualName && owned.length > 0
    if (prefilledName) {
      laneName.value = topic.label
      nameManuallyEdited.value = false
      autoNameKey.value = `history:${topic.id}`
    }
    syncAutoName()
    return {
      topicId: topic.id,
      topicLabel: topic.label,
      ownedCount: owned.length,
      addedCount: added.length,
      keptManualName,
      prefilledName,
    }
  }

  /**
   * 「直接启用原话题」：改的是**原话题的状态**，与本次草稿的「人工新建 active/manual」是两件事。
   *
   * - 资格沿服务端 `can_activate`：为假直接拒绝、**不发请求**；
   * - 走既有 `updateTopic(status='active')`：不重算质心、不改全局命中计数与状态机规则；
   * - 失败保留当前状态（不假装成功）。
   */
  async function activateHistoryTopic(topicId: string): Promise<LaneHistoryActivateOutcome> {
    const topic = historyTopics.value.find(t => t.id === topicId)
    if (!topic) return { ok: false, message: '话题不存在或已从名单移除' }
    if (!topic.canActivate) {
      const message = topic.activateBlockReason || '暂不能直接启用'
      historyError.value = message
      return { ok: false, message }
    }
    activatingTopicId.value = topic.id
    try {
      const res = await updateTopic(Number(topic.id), { status: 'active' })
      if (!res.success) {
        const message = res.error || '启用失败'
        historyError.value = message
        return { ok: false, message }
      }
      // 启用后刷新名单（状态变了，资格随之变化）；刷新失败不影响已成功的启用结果
      await loadHistoryTopics()
      return { ok: true, message: `已启用原话题「${topic.label}」` }
    } catch (e) {
      const message = e instanceof Error ? e.message : '启用失败'
      historyError.value = message
      return { ok: false, message }
    } finally {
      activatingTopicId.value = null
    }
  }

  // ── 保存 ─────────────────────────────────────────────────────────────────
  /** 保存：名称空白拒绝并保留成员；成功后走宿主回调（emit saved + close）。 */
  async function save(name?: string): Promise<boolean> {
    const label = (name ?? laneName.value).trim()
    nameError.value = null
    if (!label) {
      nameError.value = '给新泳道起个名字'
      return false
    }
    if (selectedIds.value.size === 0) {
      nameError.value = '请至少选择一个事件'
      return false
    }
    saving.value = true
    error.value = null
    try {
      const res = await createManualLane(opts.boardId.value, label, [...selectedIds.value])
      if (!res.success) {
        error.value = res.error || '保存失败'
        return false
      }
      await opts.onSaved?.(res.data?.topic)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : '保存失败'
      return false
    } finally {
      saving.value = false
    }
  }

  /** 取消本次（有草稿先确认；确认才清本地草稿）。 */
  async function cancel(): Promise<boolean> {
    if (hasDraft.value) {
      const ok = await opts.requestConfirm({
        kind: 'discard',
        title: '返回板块',
        message: '返回会丢弃未保存的选择和名称，已有泳道不受影响。',
        confirmText: '确认返回',
        cancelText: '继续编排',
      })
      if (!ok) return false
    }
    clearDraft()
    showAssigned.value = false
    return true
  }

  return {
    // 取数
    loading,
    saving,
    error,
    dateError,
    nameError,
    sections,
    relations,
    candidateChains,
    matchThreshold,
    serverWindow,
    sectionsById,
    dates,
    windowLabel,
    // 窗口
    appliedWindow,
    controlMode,
    controlFrom,
    controlTo,
    load,
    setQuickDays,
    startCustomRange,
    applyCustomRange,
    // 草稿
    selectedIds,
    selectedSections,
    memberCount,
    hasDraft,
    laneName,
    nameManuallyEdited,
    chainMembers,
    chainStates,
    recommendations,
    focusId,
    focusSection,
    showAssigned,
    // 历史管理名单
    historyTopics,
    historyLoading,
    historyError,
    historyQuery,
    filteredHistoryTopics,
    activatingTopicId,
    loadHistoryTopics,
    adoptHistoryTopic,
    activateHistoryTopic,
    // 动作
    setChainSelected,
    toggleChain,
    cancelChain,
    toggleMember,
    clearSelection,
    setFocus,
    toggleFocus,
    clearFocus,
    setLaneName,
    save,
    cancel,
  }
}
