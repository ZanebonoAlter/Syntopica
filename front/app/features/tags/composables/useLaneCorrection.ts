/**
 * 泳道修正工作面 · 状态机（lane-manual-correction-loop 5.1–5.10）。
 *
 * 契约来源：delta spec `specs/lane-correction-surface/spec.md`（工作面主契约）+
 * `specs/workshop-lane-compose/spec.md`（修正模式与新建模式草稿隔离）+
 * ui-design「Layout Contract / Interaction Contract / State Matrix / 展示字段溯源」。
 *
 * 口径与红线（本文件是唯一口径，组件层只渲染）：
 * - **查看与选入分离**（5.5）：点画布节点只改 `focusId`（选中态），成员数不变；
 *   成员集合只由左栏的取消勾选 / 加入 / 加回来改。
 * - **移除 = 直接解除归属**：`PATCH /daily-reports/sections/:id/assignment` body `{topic_id: null}`
 *   ——不弹去处选择器；内容仍在，仍可再参与候选（D4）。
 * - **加回 / 加入 = 归回本泳道**：body `{topic_id: 本泳道}`；只对「当前无归属」的节点有效。
 * - **首义 = 人工选定的节点范围**：`PATCH /daily-reports/topics/:id/anchor` body `{section_ids}`；
 *   写事实、**次日**日报生效（派生重心由算法次日重算，前端不做立即重算，D1）。
 * - **接近度 = 1 − 余弦距离**（两把尺子，来源不同、必须可区分）：
 *   成员取 `daily_report_sections.topic_match_distance` 换算（**归属快照**，日报
 *   生成时冻结）；候选取对「本泳道成员向量均值」的实时距离换算（**实时估算**，与
 *   编排画布同源：`laneComposeLogic.centroidRecommendations`）。展示层按
 *   `proximityKind` 给不同标签（fix-lane-metric-provenance）。
 * - **自动首义无节点集**：没有人工改写记录时不存在节点列表，界面不得虚构（A-12 / 溯源表）。
 * - **数据全量、展示分层**：后端按话题返回**全部成员**，窗口内的在 `sections`，
 *   窗口外的在 `early_members`；画布只画窗口内，左栏「更早成员」分组展示窗口外。
 * - **首义选择池 = 窗口内本泳道节点 + 全部更早成员**（都是本话题成员）：候选
 *   （未属本话题）不入池；已被移出（不再属本泳道）的节点也不在池内——服务端只
 *   接受**属本话题**的节点。
 */
import { computed, ref, type Ref } from 'vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import { usePersistentTopicsApi } from '~/api/persistentTopics'
import type { ComposeCandidate, ComposeEarlyMember, ComposeRelation } from '~/api/persistentTopics'
import { useLaneDynamicsWindowApi } from '~/api/laneDynamicsWindow'
import { useNotify } from '~/composables/useNotify'
import {
  centroidRecommendations,
  laneSeriesFromDynamics,
  validateWorkbenchRange,
  type WorkbenchTrendPoint,
} from './laneComposeLogic'
import type { DateBounds, ObservationRange } from '../components/console/observationRange'

/** 工作面节点（画布与左栏共用的最小形态；字段全部有来源，不留占位）。 */
export interface LaneCorrectionNode {
  /** section id（字符串，与编排候选同口径）。 */
  id: string
  /** 报告期日期（YYYY-MM-DD）。 */
  date: string
  /** 节点标题（section 的 cluster_label）。 */
  title: string
  /** 当前归属泳道 id（'' = 无归属）。 */
  topicId: string
  /** 归属泳道名（其它泳道只读参照用）。 */
  topicLabel: string
  /**
   * 归属泳道的状态（'active' / 'candidate' / 'archived' / '' = 无归属）。
   *
   * 候选口径用：属 **candidate（动态）** 泳道的节点可作为候选（改挂进本泳道，
   * 对方尚未转正、素材仍在流动）；属 **active** 泳道的节点只作只读参照。
   * 拿不到状态时（后端未升级）为 '' → 按只读参照处理（不虚构成候选）。
   */
  topicStatus?: string
  /** 接近度 = 1 − 余弦距离；没有距离口径时为 null（不伪造）。 */
  proximity: number | null
  /**
   * 接近度来源（fix-lane-metric-provenance）：两把尺子必须可区分——
   *  - `snapshot` = 归属快照距离换算（`1 − topic_match_distance`，日报生成时冻结）；
   *  - `estimate` = 对当前成员质心的实时估算（关联推荐行）。
   * 展示层据此给不同标签，MUST NOT 同名同形。
   */
  proximityKind: 'snapshot' | 'estimate'
  /** 是否落在当前取数窗口之外（更早折叠带用；按日期判断，不写死）。 */
  outsideWindow: boolean
}

/** 空态种类（各自身份，不共用一句话）。 */
export type LaneCorrectionEmpty = 'no-members' | 'no-nodes' | null

/** 取数窗口档位（unify-lane-workbench OQ-2 后 365 可用）。 */
export type LaneCorrectionWindow = 7 | 14 | 30 | 365

const DAY_MS = 86_400_000
/** 可编排窗口上限（含首末天；与后端三处同源）。 */
const CORRECTION_WINDOW_MAX_DAYS = 365

function parseUtcDay(value: string): number {
  return Date.parse(`${value}T00:00:00Z`)
}

function formatUtcDay(ts: number): string {
  return new Date(ts).toISOString().slice(0, 10)
}

/** 折叠带最多显示的节点数（超出走「+M」溢出标记，全量在左栏「更早」分组）。 */
export const LANE_FOLD_MAX = 6

export interface UseLaneCorrectionOptions {
  boardId: Ref<number>
  /** 目标泳道（持久话题）id。 */
  topicId: Ref<number>
  /** 泳道显示名（工作面标题；可就地编辑）。 */
  laneLabel: Ref<string>
  /** 改名成功后通知宿主（外壳据此刷新列表）。 */
  onRenamed?: (label: string) => void
}

export function useLaneCorrection(opts: UseLaneCorrectionOptions) {
  const { getComposeCandidates } = usePersistentTopicsApi()
  const { getTopicLifeline, getTopicAnchor, setSectionAssignment, updateTopicAnchor, updateTopic } = useDailyReportsApi()
  const { loadLaneDynamics } = useLaneDynamicsWindowApi()
  const { success: notifySuccess, error: notifyError } = useNotify()

  // ── 状态矩阵 ─────────────────────────────────────────────────────────────
  /** 首次加载中（骨架；失败转 error，不闪空态）。 */
  const loading = ref(false)
  /** 加载失败（行内错误 + 重试；保留本轮未提交改动）。 */
  const loadError = ref<string | null>(null)
  /** 写操作失败（行内错误 + 重试；改动留在画布上不丢）。 */
  const actionError = ref<string | null>(null)
  /** 正在提交的节点 id（按钮 loading，避免重复提交）。 */
  const pendingIds = ref<Set<string>>(new Set())
  /** 改名失败信息（就地编辑的输入框下方）。 */
  const renameError = ref<string | null>(null)

  // ── 取数 ─────────────────────────────────────────────────────────────────
  const windowDays = ref<LaneCorrectionWindow>(7)
  const sections = ref<ComposeCandidate[]>([])
  /** 窗口内两端可见的原关系（unify-lane-workbench 5.1：相关高亮的共享入口输入）。 */
  const relations = ref<ComposeRelation[]>([])
  const windowLabel = ref('')
  const windowFrom = ref('')
  const windowTo = ref('')
  /** 服务端回显的最新已完成日报日（自选下界校验用；null = 未知，跳过下界检查）。 */
  const windowLatest = ref<string | null>(null)
  /** 控件展示的观察范围（preview 直写；commit 成功后由服务端回显接管，模态本地）。 */
  const windowRange = ref<ObservationRange | null>(null)
  /** 自选范围非法的行内错误（非法保旧，不静默截断）。 */
  const windowError = ref<string | null>(null)
  const dates = ref<string[]>([])
  const matchThreshold = ref(0.3)
  /** 请求序号：只有最新一次取数能提交（迟到响应丢弃，B9）。 */
  let loadSeq = 0

  // ── 365 日本泳道曲线（6.3：lane-dynamics 目标泳道，独立于候选窗口） ──────────
  /** 目标泳道逐日线索曲线；null = 未取到/目标未返回（未知，不造全 0）。 */
  const curveSeries = ref<WorkbenchTrendPoint[] | null>(null)
  const curveLoading = ref(false)
  const curveError = ref<string | null>(null)
  /** 曲线日期轴（锚点日 − 364 ~ 锚点日；来自 lane-dynamics 回显锚点）。 */
  const curveBounds = ref<DateBounds | null>(null)
  /** 目标泳道是否出现在 lane-dynamics 响应里（false = 曲线未知）。 */
  const curveTargetKnown = ref(false)
  let curveSeq = 0
  /**
   * 更早成员（窗口外的本话题成员；数据全量、展示分层）。
   *
   * null = 后端未提供全量口径（旧后端 / 编排画布）→ 计数回退只显示窗口数。
   */
  const earlyMembers = ref<LaneCorrectionNode[] | null>(null)
  /** 全量泳道成员（任意日期；窗口外的成员只用于补全首义节点信息）。 */
  const laneHistory = ref<Map<string, LaneCorrectionNode>>(new Map())

  /** 当前首义来源：auto = 创建话题时写入（无节点集）；manual = 人工选定节点范围。 */
  const anchorSource = ref<'auto' | 'manual'>('auto')
  /** 当前首义节点集（人工改写的选定节点；自动首义恒为空）。 */
  const anchorSectionIds = ref<number[]>([])

  // ── 会话内交互态 ─────────────────────────────────────────────────────────
  /** 本泳道成员（窗口内）。 */
  const memberIds = ref<Set<string>>(new Set())
  /** 本次移出的节点（本轮会话内；可加回来）。 */
  const removedIds = ref<Set<string>>(new Set())
  /** 画布选中（只影响选中态，不改成员数）。 */
  const focusId = ref<string | null>(null)
  /** 首义范围勾选（本地选择集，提交前不落库）。 */
  const anchorSelection = ref<Set<string>>(new Set())
  /** 覆盖已有首义时的画布内确认条（不叠弹窗）。 */
  const anchorConfirmOpen = ref(false)

  // ── 派生 ─────────────────────────────────────────────────────────────────
  /** 成员的接近度（现库 `topic_match_distance` 换算；读回泳道全量成员时写入）。 */
  const memberProximity = ref<Map<string, number>>(new Map())

  /** 窗口内节点（画布散点与左栏清单的共同来源）。 */
  const windowNodes = computed<LaneCorrectionNode[]>(() => {
    const out: LaneCorrectionNode[] = []
    for (const section of sections.value) {
      out.push({
        id: section.id,
        date: section.periodDate.slice(0, 10),
        title: section.clusterLabel,
        topicId: section.persistentTopicId ?? '',
        topicLabel: section.persistentTopic?.label ?? '',
        topicStatus: section.persistentTopic?.status ?? '',
        proximity: memberProximity.value.get(section.id) ?? null,
        // 窗口内节点的接近度来自归属快照（topic_match_distance）——实时估算只用于
        // 关联推荐行（recommendations 覆盖为 estimate）。
        proximityKind: 'snapshot',
        outsideWindow: false,
      })
    }
    return out
  })

  /** 本泳道成员（窗口内；计数与左栏清单同源）。 */
  const memberNodes = computed<LaneCorrectionNode[]>(() =>
    windowNodes.value.filter(node => memberIds.value.has(node.id)),
  )

  /** 更早成员（窗口外；左栏「更早成员」分组，按月分堆）。 */
  const earlierMemberNodes = computed<LaneCorrectionNode[]>(() => earlyMembers.value ?? [])

  /**
   * 全量成员数（= 本话题成员 section 总数：窗口内 + 窗口外；左栏计数徽标
   * 「窗口 N · 全部 M」的 M，也是首义范围面板「全部 M」的唯一来源）。
   *
   * 定义唯一（fix-lane-metric-provenance）：同一把尺子 = 成员 section 数；
   * 左栏徽标与首义范围面板必须同数——不得一个用「窗口内 + 窗口外」加法、另一个
   * 用「去重并集」两套算法。
   * null = 后端未提供全量口径（旧后端 / 编排画布）→ 徽标回退只显示窗口数。
   */
  const totalMemberCount = computed<number | null>(() =>
    earlyMembers.value === null ? null : memberNodes.value.length + earlierMemberNodes.value.length,
  )

  /** 本次移出的节点（窗口内）。 */
  const removedNodes = computed<LaneCorrectionNode[]>(() =>
    windowNodes.value.filter(node => removedIds.value.has(node.id)),
  )

  /**
   * 只读参照（属其它 **active** 泳道的节点）：不可选入、不可移出。
   *
   * 属 candidate（动态）泳道的节点不进只读参照 —— 它们是候选（可改挂进本泳道）。
   */
  const readonlyNodes = computed<LaneCorrectionNode[]>(() =>
    windowNodes.value.filter(node =>
      Boolean(node.topicId)
      && node.topicStatus !== 'candidate'
      && !memberIds.value.has(node.id)
      && !removedIds.value.has(node.id)),
  )

  /**
   * 关联推荐（成员质心）：候选 = 窗口内**无归属**或属 **candidate（动态）泳道**
   * 且未选入的节点，按与质心的距离排序。
   *
   * 口径与编排画布一致（`centroidRecommendations`：质心 = 有效成员向量均值，
   * 阈值内才算推荐）；与编排画布的差别只在于「属 active 泳道的节点不作候选」
   * —— 那属于已转正的其它泳道，只作只读参照。
   */
  const recommendations = computed<LaneCorrectionNode[]>(() => {
    const chosen = new Set(memberNodes.value.map(node => node.id))
    // 质心池扩容：窗口内成员 + 更早成员（向量可用者）——否则窗口内成员为 0 时
    // 质心为空、候选区全空（早成员已是本话题成员，入 chosen 不参与候选排序）。
    const centroidPool = [...sections.value]
    for (const node of earlierMemberNodes.value) {
      const embedding = earlyEmbeddings.get(node.id)
      if (!embedding) continue
      chosen.add(node.id)
      // 显式构造最小对象，不借道 `sections.value[0]`（空数组时会展开成 {}）。
      centroidPool.push({
        id: node.id,
        embedding,
        persistentTopicId: String(opts.topicId.value),
        persistentTopic: undefined,
        periodDate: node.date,
        clusterLabel: node.title,
      })
    }
    const result = centroidRecommendations(centroidPool, chosen, matchThreshold.value)
    const out: LaneCorrectionNode[] = []
    for (const item of result.unassigned) {
      const section = item.section
      // 候选口径：无归属，或属 candidate（动态）泳道（可改挂进本泳道）。
      // 属 active 泳道的节点不作候选（已转正 → 只读参照）。
      const ownerStatus = section.persistentTopic?.status ?? ''
      if (section.persistentTopicId && ownerStatus !== 'candidate') continue
      if (chosen.has(section.id)) continue
      if (removedIds.value.has(section.id)) continue
      const node = windowNodes.value.find(entry => entry.id === section.id)
      if (!node) continue
      // 关联推荐的接近度 = 对当前成员质心的**实时估算**（不是归属快照）——
      // 与成员行必须可区分（fix-lane-metric-provenance）。
      out.push({ ...node, proximity: 1 - item.distance, proximityKind: 'estimate' })
    }
    return out
  })

  /** 候选（无归属、窗口内、未选入）——画布「候选区」的节点。 */
  const candidateNodes = computed<LaneCorrectionNode[]>(() => recommendations.value)

  /**
   * 当前首义节点集（人工改写才有节点集；自动首义恒为空 —— 不虚构）。
   * 节点信息来自窗口内候选或泳道全量成员（窗口外的首义节点据此展示标题与日期）。
   *
   * 解析顺序：窗口内节点 → 更早成员（后端全量口径，距离缺失时不伪造）→ 泳道全量
   * 成员（lifeline 兜底：更早成员里没有的历史节点）。
   */
  const anchorNodes = computed<LaneCorrectionNode[]>(() => {
    if (anchorSource.value !== 'manual') return []
    const out: LaneCorrectionNode[] = []
    for (const rawId of anchorSectionIds.value) {
      const id = String(rawId)
      const inWindow = windowNodes.value.find(node => node.id === id)
      const node = inWindow
        ?? earlierMemberNodes.value.find(entry => entry.id === id)
        ?? laneHistory.value.get(id)
      if (!node) continue
      out.push({ ...node, outsideWindow: !inWindow })
    }
    return out
  })

  /** 「更早」节点：当前首义集里落在窗口之外的节点（按日期判断）。 */
  const earlierAnchorNodes = computed<LaneCorrectionNode[]>(() => anchorNodes.value.filter(node => node.outsideWindow))

  /**
   * 首义选择池：窗口内本泳道节点 + **全部更早成员**（都是本话题成员）。
   * 未属本话题的候选不入池；已移出的节点不在池内（服务端只接受属本话题的节点）。
   */
  const anchorPool = computed<LaneCorrectionNode[]>(() => {
    const out: LaneCorrectionNode[] = []
    const seen = new Set<string>()
    const push = (node: LaneCorrectionNode): void => {
      if (seen.has(node.id)) return
      seen.add(node.id)
      out.push(node)
    }
    // 当前首义集优先（属当前首义集的节点带首义标记）。
    for (const node of anchorNodes.value) push(node)
    // 窗口内本泳道成员。
    for (const node of memberNodes.value) push(node)
    // 全部更早成员（窗口外的本话题成员）。
    for (const node of earlierMemberNodes.value) push(node)
    return out
  })

  /** 当前首义来源的展示结论（自动 / 人工 + 节点数）。 */
  const anchorSourceLabel = computed(() =>
    anchorSource.value === 'manual'
      ? `人工（选定的 ${anchorSectionIds.value.length} 个节点）`
      : '自动（创建话题时写入）',
  )

  /** 空态（首次加载失败时恒为 null —— 失败不呈现业务空态）。 */
  const emptyKind = computed<LaneCorrectionEmpty>(() => {
    if (loading.value || loadError.value) return null
    if (!windowNodes.value.length) return 'no-nodes'
    if (!memberNodes.value.length) return 'no-members'
    return null
  })

  // ── 取数 ─────────────────────────────────────────────────────────────────
  function buildDates(from: string, to: string): string[] {
    if (!from || !to) return []
    const start = Date.parse(`${from}T00:00:00Z`)
    const end = Date.parse(`${to}T00:00:00Z`)
    if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return []
    const out: string[] = []
    for (let ts = start; ts <= end && out.length <= 400; ts += DAY_MS) {
      out.push(new Date(ts).toISOString().slice(0, 10))
    }
    return out
  }

  /** 泳道全量成员 → 节点映射（窗口外的首义节点据此展示标题 / 日期 / 接近度）。 */
  function buildLaneHistory(nodes: { id: number, period_date: string, cluster_label: string, topic_match_distance?: number }[]): Map<string, LaneCorrectionNode> {
    const map = new Map<string, LaneCorrectionNode>()
    for (const node of nodes) {
      const distance = node.topic_match_distance
      map.set(String(node.id), {
        id: String(node.id),
        date: String(node.period_date ?? '').slice(0, 10),
        title: node.cluster_label ?? '',
        topicId: String(opts.topicId.value),
        topicLabel: opts.laneLabel.value,
        topicStatus: 'active',
        proximity: typeof distance === 'number' && Number.isFinite(distance) ? 1 - distance : null,
        proximityKind: 'snapshot',
        outsideWindow: true,
      })
    }
    return map
  }

  /**
   * 更早成员行 → 节点（后端 `early_members`；接近度 = 1 − 锚定距离，
   * 无距离口径时为 null —— 不伪造数字）。
   */
  /** 早成员向量（id → embedding）：关联推荐的质心需要窗口外成员参与。 */
  const earlyEmbeddings = new Map<string, number[]>()

  function buildEarlyMembers(rows: ComposeEarlyMember[]): LaneCorrectionNode[] {
    earlyEmbeddings.clear()
    for (const row of rows) {
      if (row.embedding && row.embedding.length > 0) earlyEmbeddings.set(row.id, row.embedding)
    }
    return rows.map(row => ({
      id: row.id,
      date: row.periodDate.slice(0, 10),
      title: row.clusterLabel,
      topicId: String(opts.topicId.value),
      topicLabel: opts.laneLabel.value,
      topicStatus: 'active',
      proximity: typeof row.topicMatchDistance === 'number' && Number.isFinite(row.topicMatchDistance)
        ? 1 - row.topicMatchDistance
        : null,
      // 更早成员的接近度同样来自归属快照（topic_match_distance）。
      proximityKind: 'snapshot',
      outsideWindow: true,
    }))
  }

  /** 装载工作面数据（候选窗口 + 泳道全量成员 + 当前首义）。 */
  async function load(): Promise<boolean> {
    const seq = ++loadSeq
    loading.value = true
    loadError.value = null
    try {
      const [candidates, lifeline, anchor] = await Promise.all([
        getComposeCandidates(opts.boardId.value, { days: windowDays.value, allMembersTopicId: opts.topicId.value }),
        getTopicLifeline(opts.topicId.value),
        getTopicAnchor(opts.topicId.value),
      ])
      if (seq !== loadSeq) return false // 迟到响应丢弃：不改清单/范围/曲线
      if (!candidates.success || !candidates.data) {
        loadError.value = candidates.error || '加载泳道成员失败'
        return false
      }
      if (!anchor.success || !anchor.data) {
        loadError.value = anchor.error || '读取当前首义失败'
        return false
      }

      sections.value = candidates.data.sections
      relations.value = candidates.data.relations
      matchThreshold.value = candidates.data.matchThreshold
      const from = candidates.data.window?.from ?? ''
      const to = candidates.data.window?.to ?? ''
      windowFrom.value = from
      windowTo.value = to
      windowLatest.value = candidates.data.window?.latestCompletedDate ?? null
      windowLabel.value = from && to ? `${from} — ${to}` : ''
      windowRange.value = from && to ? { start: from, end: to } : null
      dates.value = buildDates(from, to)

      const topicId = String(opts.topicId.value)
      memberIds.value = new Set(
        candidates.data.sections.filter(section => section.persistentTopicId === topicId).map(section => section.id),
      )
      removedIds.value = new Set()

      // 成员的接近度走现库距离（1 − topic_match_distance）；窗口外节点用泳道全量成员补全。
      const distanceOf = new Map<string, number>()
      const historyNodes = lifeline.success && lifeline.data ? lifeline.data.sections : []
      for (const node of historyNodes) {
        if (typeof node.topic_match_distance === 'number' && Number.isFinite(node.topic_match_distance)) {
          distanceOf.set(String(node.id), 1 - node.topic_match_distance)
        }
      }
      memberProximity.value = distanceOf
      laneHistory.value = buildLaneHistory(historyNodes)

      // 更早成员（窗口外的本话题成员）：null = 后端未提供全量口径 → 徽标回退
      // 只显示窗口数（兼容旧后端）。
      earlyMembers.value = candidates.data.earlyMembers === null || candidates.data.earlyMembers === undefined
        ? null
        : buildEarlyMembers(candidates.data.earlyMembers)

      anchorSource.value = anchor.data.source === 'manual' ? 'manual' : 'auto'
      anchorSectionIds.value = [...(anchor.data.section_ids ?? [])]
      anchorSelection.value = new Set()
      anchorConfirmOpen.value = false
      focusId.value = null
      actionError.value = null
      return true
    } catch (e) {
      if (seq !== loadSeq) return false
      loadError.value = e instanceof Error ? e.message : '加载泳道成员失败'
      return false
    } finally {
      if (seq === loadSeq) loading.value = false
    }
  }

  /**
   * 切窗口（按日期重新判定「更早」；失败保旧——档位/范围回退到生效值，迟到丢弃）。
   */
  async function setWindow(days: LaneCorrectionWindow): Promise<boolean> {
    if (days === windowDays.value) return true
    const seqBefore = loadSeq
    const previous = windowDays.value
    windowDays.value = days
    const ok = await load()
    if (!ok && loadSeq === seqBefore + 1) {
      // 失败且没有更新的取数接管 → 档位保旧
      windowDays.value = previous
      windowRange.value = windowFrom.value && windowTo.value ? { start: windowFrom.value, end: windowTo.value } : null
    }
    return ok
  }

  /** 控件档位（档按钮激活态；非 7/14/30/365 时无激活档）。 */
  const tierDays = computed<number>(() =>
    [7, 14, 30, 365].includes(windowDays.value) ? windowDays.value : 0)

  /** 手势/键盘预览：只更新选区显示，不请求（松手 commit 才取候选）。 */
  function previewWindow(range: ObservationRange): void {
    windowRange.value = range
  }

  /** 自选起止 commit：校验（非法保旧）后一次取数（带 from/to/allMembersTopicId）。 */
  async function commitWindow(range: ObservationRange): Promise<boolean> {
    return applyWindowRange(range.start, range.end)
  }

  /** 应用自选起止（同值不重取，J1；非法 → 行内报错 + 保旧）。 */
  async function applyWindowRange(from: string, to: string): Promise<boolean> {
    windowError.value = null
    const problem = validateWorkbenchRange(from, to, CORRECTION_WINDOW_MAX_DAYS, windowLatest.value)
    if (problem) {
      windowError.value = problem
      windowRange.value = windowFrom.value && windowTo.value ? { start: windowFrom.value, end: windowTo.value } : null
      return false
    }
    if (from === windowFrom.value && to === windowTo.value) return true
    const prevFrom = windowFrom.value
    const prevTo = windowTo.value
    const seq = ++loadSeq
    loading.value = true
    try {
      const res = await getComposeCandidates(opts.boardId.value, { from, to, allMembersTopicId: opts.topicId.value })
      if (seq !== loadSeq) return false // 迟到响应丢弃：不改清单/范围
      if (!res.success || !res.data) {
        windowError.value = res.error || '切换日期范围失败'
        windowRange.value = prevFrom && prevTo ? { start: prevFrom, end: prevTo } : null
        return false
      }
      // 成功：原子替换清单/范围/回显（不复用 load()——它按快捷档取数）
      sections.value = res.data.sections
      relations.value = res.data.relations
      matchThreshold.value = res.data.matchThreshold
      const newFrom = res.data.window?.from ?? from
      const newTo = res.data.window?.to ?? to
      windowFrom.value = newFrom
      windowTo.value = newTo
      windowLatest.value = res.data.window?.latestCompletedDate ?? windowLatest.value
      windowLabel.value = newFrom && newTo ? `${newFrom} — ${newTo}` : ''
      windowRange.value = newFrom && newTo ? { start: newFrom, end: newTo } : null
      dates.value = buildDates(newFrom, newTo)
      const topicId = String(opts.topicId.value)
      memberIds.value = new Set(res.data.sections.filter(section => section.persistentTopicId === topicId).map(section => section.id))
      removedIds.value = new Set()
      anchorSelection.value = new Set()
      focusId.value = null
      return true
    } catch (e) {
      windowError.value = e instanceof Error ? e.message : '切换日期范围失败'
      windowRange.value = prevFrom && prevTo ? { start: prevFrom, end: prevTo } : null
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * 365 日本泳道曲线（6.3）：lane-dynamics 目标泳道逐日 clue_count，独立于候选窗口。
   *
   * - 一次打开只保留一份会话 payload；切窗/关闭用请求序号隔离迟到响应；
   * - 目标泳道未出现在响应里 → 曲线未知（不造全 0）；轴上断档日 = 未知（不补 0）；
   * - 失败保旧曲线并标记错误，可单独重试；写操作成功后重取。
   */
  async function reloadCurve(): Promise<boolean> {
    const seq = ++curveSeq
    curveLoading.value = true
    curveError.value = null
    try {
      const res = await loadLaneDynamics(Number(opts.boardId.value), 365)
      if (seq !== curveSeq) return false
      if (!res.success) {
        curveError.value = res.error || res.message || '本泳道曲线加载失败'
        return false
      }
      if (!res.data) {
        curveError.value = res.message || '本泳道曲线数据为空'
        return false
      }
      const anchor = res.data.window.anchorDate
      if (!anchor) {
        curveError.value = '尚无已完成日报，取不到曲线锚点'
        return false
      }
      const endTs = parseUtcDay(anchor)
      const start = formatUtcDay(endTs - (CORRECTION_WINDOW_MAX_DAYS - 1) * DAY_MS)
      curveBounds.value = { start, end: anchor }
      const series = laneSeriesFromDynamics(res.data.window.lanes, String(opts.topicId.value))
      curveTargetKnown.value = series !== null
      if (!series) {
        curveSeries.value = null // 目标泳道未返回：未知，不造全 0
        return true
      }
      const valueByDate = new Map(series.map(point => [point.date, point.value]))
      const out: WorkbenchTrendPoint[] = []
      for (let ts = parseUtcDay(start); ts <= endTs && out.length <= 400; ts += DAY_MS) {
        const date = formatUtcDay(ts)
        out.push({ date, value: valueByDate.has(date) ? valueByDate.get(date)! ?? null : null })
      }
      curveSeries.value = out
      return true
    } catch (e) {
      if (seq !== curveSeq) return false
      curveError.value = e instanceof Error ? e.message : '本泳道曲线加载失败'
      return false
    } finally {
      if (seq === curveSeq) curveLoading.value = false
    }
  }

  // ── 归属变更 ─────────────────────────────────────────────────────────────
  function markPending(id: string, pending: boolean): void {
    const next = new Set(pendingIds.value)
    if (pending) next.add(id)
    else next.delete(id)
    pendingIds.value = next
  }

  /** 移除成员（取消勾选即移除）：直接解除归属，不弹去处选择器。 */
  async function removeMember(id: string): Promise<boolean> {
    const sectionId = Number(id)
    if (!Number.isSafeInteger(sectionId)) return false
    markPending(id, true)
    actionError.value = null
    try {
      const res = await setSectionAssignment(sectionId, null)
      if (!res.success) {
        actionError.value = res.error || '移除失败'
        notifyError(actionError.value)
        return false
      }
      const members = new Set(memberIds.value)
      members.delete(id)
      memberIds.value = members
      const removed = new Set(removedIds.value)
      removed.add(id)
      removedIds.value = removed
      // 首义选择集同步剔除：已移出的节点不在选择池内（服务端只接受属本话题的节点）——
      // 不剔除会让「用选中的 N 个节点定首义」的计数偏大，并带上非本话题的节点。
      const selection = new Set(anchorSelection.value)
      if (selection.delete(id)) anchorSelection.value = selection
      void reloadCurve() // 写后重取本泳道曲线（失败不影响写结果）
      notifySuccess('已移除，该内容不再归属任何泳道，仍可再参与候选；归属立即生效，重心次日日报生效')
      return true
    } catch (e) {
      actionError.value = e instanceof Error ? e.message : '移除失败'
      notifyError(actionError.value)
      return false
    } finally {
      markPending(id, false)
    }
  }

  /** 归回本泳道（加入 / 加回共用）：写 `{topic_id: 本泳道}`，成功后进成员列表。 */
  async function attachMember(id: string, successMessage: string): Promise<boolean> {
    const sectionId = Number(id)
    if (!Number.isSafeInteger(sectionId)) return false
    markPending(id, true)
    actionError.value = null
    try {
      const res = await setSectionAssignment(sectionId, opts.topicId.value)
      if (!res.success) {
        actionError.value = res.error || '加入失败'
        notifyError(actionError.value)
        return false
      }
      const members = new Set(memberIds.value)
      members.add(id)
      memberIds.value = members
      const removed = new Set(removedIds.value)
      removed.delete(id)
      removedIds.value = removed
      // 改挂：节点原属 candidate（动态）泳道 → 反馈说清“从哪个泳道挪来”。
      // 加回（本次移出的节点）沿用原反馈文案 —— 那不是改挂。
      const source = windowNodes.value.find(node => node.id === id)?.topicLabel ?? ''
      const wasOtherLane = source !== '' && source !== opts.laneLabel.value
      void reloadCurve() // 写后重取本泳道曲线（失败不影响写结果）
      notifySuccess(wasOtherLane ? `已从「${source}」移入` : successMessage)
      return true
    } catch (e) {
      actionError.value = e instanceof Error ? e.message : '加入失败'
      notifyError(actionError.value)
      return false
    } finally {
      markPending(id, false)
    }
  }

  /** 加入候选节点（勾选候选成员 / 关联推荐「加入」）：归回本泳道。 */
  async function addMember(id: string): Promise<boolean> {
    return attachMember(id, '已加入')
  }

  /** 加回本次移出的节点（与「加入」同一写路径，反馈文案不同）。 */
  async function restoreMember(id: string): Promise<boolean> {
    return attachMember(id, '已加回')
  }

  // ── 首义范围 ─────────────────────────────────────────────────────────────
  /** 勾选 / 取消勾选首义范围节点（本地选择，不落库）。 */
  function toggleAnchorNode(id: string): void {
    const next = new Set(anchorSelection.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    anchorSelection.value = next
  }

  /** 发起定首义：已有首义时先出画布内确认条（覆盖已有首义要确认）。 */
  function requestAnchorApply(): void {
    if (!anchorSelection.value.size) return
    if (anchorSource.value === 'manual' && anchorSectionIds.value.length) {
      anchorConfirmOpen.value = true
      return
    }
    void applyAnchor()
  }

  /** 确认覆盖首义 → 落库。 */
  async function applyAnchor(): Promise<boolean> {
    const ids = [...anchorSelection.value].map(id => Number(id)).filter(id => Number.isSafeInteger(id))
    if (!ids.length) {
      actionError.value = '先勾选至少一个节点'
      return false
    }
    actionError.value = null
    try {
      const res = await updateTopicAnchor(opts.topicId.value, ids)
      if (!res.success) {
        actionError.value = res.error || '重设首义失败'
        notifyError(actionError.value)
        return false
      }
      anchorSource.value = 'manual'
      anchorSectionIds.value = ids
      anchorConfirmOpen.value = false
      notifySuccess('首义已更新，次日日报生效')
      return true
    } catch (e) {
      actionError.value = e instanceof Error ? e.message : '重设首义失败'
      notifyError(actionError.value)
      return false
    }
  }

  /** 取消确认条（不落库）。 */
  function cancelAnchorApply(): void {
    anchorConfirmOpen.value = false
  }

  // ── 泳道名就地编辑 ───────────────────────────────────────────────────────
  /** 提交改名（Enter / 失焦都走这里；空白拒绝）。 */
  async function commitRename(label: string): Promise<boolean> {
    renameError.value = null
    const trimmed = label.trim()
    if (!trimmed) {
      renameError.value = '泳道名不能为空'
      return false
    }
    if (trimmed === opts.laneLabel.value) return true
    try {
      const res = await updateTopic(opts.topicId.value, { label: trimmed })
      if (!res.success) {
        renameError.value = res.error || '改名失败'
        notifyError(renameError.value)
        return false
      }
      opts.laneLabel.value = trimmed
      opts.onRenamed?.(trimmed)
      notifySuccess('泳道名已更新')
      return true
    } catch (e) {
      renameError.value = e instanceof Error ? e.message : '改名失败'
      notifyError(renameError.value)
      return false
    }
  }

  // ── 画布选中（只改选中态，不改成员数） ───────────────────────────────────
  function setFocus(id: string | null): void {
    focusId.value = focusId.value === id ? null : id
  }

  function clearFocus(): void {
    focusId.value = null
  }

  return {
    // 状态矩阵
    loading,
    loadError,
    actionError,
    renameError,
    pendingIds,
    // 取数
    windowDays,
    windowLabel,
    windowFrom,
    windowTo,
    windowRange,
    windowError,
    tierDays,
    relations,
    dates,
    matchThreshold,
    earlyMembers,
    // 365 日本泳道曲线
    curveSeries,
    curveLoading,
    curveError,
    curveBounds,
    curveTargetKnown,
    reloadCurve,
    // 会话态
    memberIds,
    removedIds,
    focusId,
    anchorSelection,
    anchorConfirmOpen,
    // 首义
    anchorSource,
    anchorSectionIds,
    anchorSourceLabel,
    anchorNodes,
    earlierAnchorNodes,
    anchorPool,
    // 派生
    memberNodes,
    earlierMemberNodes,
    totalMemberCount,
    removedNodes,
    readonlyNodes,
    candidateNodes,
    recommendations,
    emptyKind,
    // 动作
    load,
    setWindow,
    previewWindow,
    commitWindow,
    applyWindowRange,
    removeMember,
    addMember,
    restoreMember,
    toggleAnchorNode,
    requestAnchorApply,
    applyAnchor,
    cancelAnchorApply,
    commitRename,
    setFocus,
    clearFocus,
  }
}
