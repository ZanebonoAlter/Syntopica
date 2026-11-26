/**
 * 就地编排态纯逻辑 composable（inline-compose-lane 切片①）.
 *
 * 「新建泳道」从独立全屏视图改为 lanes 主视图就地叠加（composeMode 布尔态）。
 * 本 composable 负责编排态的全部纯逻辑与网络协调，视图层（D2/D3）只管渲染：
 *  - 候选池加载（getComposeCandidates）
 *  - 就地勾选 → 实时聚合锚点（aggregatePreview mean pooling）→ 全员
 *    cosineDistance / distanceTier / 离群标黄（outlierFlags：distance > threshold×1.3
 *    标黄但保持勾选，不自动删）
 *  - active 泳道 section 可勾走（moveOut 分类 + 二次确认）
 *  - 聚类质量单卡（成员数 / 平均距离 / 离群数）
 *  - 语义搜索冷启动（embedQuery → queryVec），勾选后 anchor 接管主信号
 *  - 候选话题侧边栏（采纳预填名+预勾 / 一键激活）
 *  - 相似 section 推荐（按已选聚合向量推荐未勾选 section，分 unassigned/active 两组）
 *  - 保存（createManualLane，含移出二次确认）
 *
 * 纯逻辑全部复用 composeReport.ts（cosineDistance / aggregatePreview / outlierFlags
 * / distanceTier / rankCandidates），禁止重复实现。后端零改动，API 全复用。
 *
 * 设计依据：section-lifecycle spec 三 Requirement + design §4.2/§4.3/§4.4。
 */
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import { usePersistentTopicsApi } from '~/api/persistentTopics'
import {
  aggregatePreview,
  cosineDistance,
  distanceTier,
  outlierFlags,
  rankCandidates,
} from '~/features/tags/components/composeReport'
import type { ComposeCandidate, ManualTopicResult } from '~/api/persistentTopics'
import type { BoardTopicListItem } from '~/api/dailyReports'
import type { DistanceTier } from '~/features/tags/components/composeReport'

/** 单个 pool 节点的编排态派生信息（按 section id 索引）。 */
export interface NodeTierInfo {
  /** 到 anchor（或冷启动 queryVec）的 cosineDistance；无 active 信号=Infinity。 */
  distance: number
  /** good | boundary | outlier | far。 */
  tier: DistanceTier
  /** distance > matchThreshold×1.3（与 outlierFlags 同门）；标黄不自动取消勾选。 */
  isOutlier: boolean
  /** 当前是否在选中集。 */
  selected: boolean
  /** 该 section 有 persistentTopicId（来自 active 泳道）→ 将移出。 */
  moveOut: boolean
  /** 若 moveOut，取 candidate.persistentTopic?.label ?? '现有泳道'。 */
  originLabel: string | null
}

/** 顶部「聚类质量」单卡数据。 */
export interface QualityCard {
  memberCount: number
  meanDistance: number
  outlierCount: number
}

/** 保存前移出二次确认用的单条 section 项。 */
export interface MoveOutItem {
  label: string
  origin: string
}

/** 右侧候选话题侧边栏单条。 */
export interface SidebarCandidateItem {
  topic: BoardTopicListItem
  /** topic.can_activate（已达 upgrade_threshold）。 */
  activatable: boolean
  /** topic.consecutive_hits === 0（近期未命中）。 */
  brokenStreak: boolean
}

/** 相似 section 推荐单条（按已选聚合向量匹配未勾选 pool 节点）。 */
export interface RecommendedSection {
  id: string
  clusterLabel: string
  /** active 来源项的原泳道名；unassigned 项为 null。 */
  originLabel: string | null
  /** 到已选聚合向量的 cosineDistance（≤ matchThreshold 才入推荐）。 */
  distance: number
}

/** 相似 section 推荐分组（主组=待确认来源，次组=现有泳道来源，弱化）。 */
export interface Recommendations {
  unassigned: RecommendedSection[]
  active: RecommendedSection[]
}

export interface UseInlineComposeOptions {
  boardId: Ref<number | string>
  /** host 传入的 candidate 话题（已 status==='candidate'）。 */
  candidateTopics: Ref<BoardTopicListItem[]>
  /** 保存/启用成功后 host 刷新总览；手动建泳道保存时带新建话题（矩阵高亮用）。 */
  onSaved: (topic?: ManualTopicResult) => void | Promise<void>
  /** host 弹 AppDialog 列出移出项；true=用户确认。 */
  requestMoveOutConfirm: (items: MoveOutItem[]) => Promise<boolean>
  /** getComposeCandidates 的 days，默认 30。 */
  embedDays?: number
  /** 采纳预勾上限，默认 20。 */
  adoptPreselectCap?: number
}

const DEFAULT_EMBED_DAYS = 30
const DEFAULT_ADOPT_CAP = 20
const RECOMMEND_UNASSIGNED_CAP = 5
const RECOMMEND_ACTIVE_CAP = 3
const SEARCH_DEBOUNCE_MS = 300
const FALLBACK_ORIGIN = '现有泳道'

export function useInlineCompose(opts: UseInlineComposeOptions) {
  const { getComposeCandidates, createManualLane, embedQuery } = usePersistentTopicsApi()
  const { updateTopic } = useDailyReportsApi()

  const embedDays = opts.embedDays ?? DEFAULT_EMBED_DAYS
  const adoptPreselectCap = opts.adoptPreselectCap ?? DEFAULT_ADOPT_CAP

  // ── state ───────────────────────────────────────────────────────────────
  const active = ref(false)
  const pool = ref<ComposeCandidate[]>([])
  const matchThreshold = ref(0.3)
  const selectedIds = ref<Set<string>>(new Set())
  const laneName = ref('')
  const queryText = ref('')
  const queryVec = ref<number[] | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)
  const searchError = ref<string | null>(null)

  let searchTimer: ReturnType<typeof setTimeout> | null = null

  // ── computed ────────────────────────────────────────────────────────────

  /** pool ∩ selectedIds，保持 pool 顺序。 */
  const selectedCandidates: ComputedRef<ComposeCandidate[]> = computed(() =>
    pool.value.filter(c => selectedIds.value.has(c.id)),
  )

  /** 选中 section embedding 的 mean（aggregatePreview）；无可用= null。 */
  const anchor: ComputedRef<number[] | null> = computed(() =>
    aggregatePreview(selectedCandidates.value.map(c => c.embedding)).mean,
  )

  /** rankCandidates 用的主信号：anchor 优先，否则 queryVec（冷启动）。 */
  const activeSignal: ComputedRef<number[] | null> = computed(() => anchor.value ?? queryVec.value)

  /**
   * 按 section id 索引每个 pool 节点的编排态派生信息。
   * distance 用 anchor（无勾选时退回 queryVec，都无则 Infinity 默认分层）。
   */
  const nodeInfo: ComputedRef<Record<string, NodeTierInfo>> = computed(() => {
    const ref1 = activeSignal.value
    const out: Record<string, NodeTierInfo> = {}
    for (const c of pool.value) {
      const distance = ref1 ? cosineDistance(c.embedding, ref1) : Number.POSITIVE_INFINITY
      const isMoveOut = c.persistentTopic?.status === 'active'
      out[c.id] = {
        distance,
        tier: distanceTier(distance, matchThreshold.value),
        isOutlier: distance > matchThreshold.value * 1.3,
        selected: selectedIds.value.has(c.id),
        moveOut: isMoveOut,
        originLabel: isMoveOut
          ? (c.persistentTopic?.label ?? FALLBACK_ORIGIN)
          : null,
      }
    }
    return out
  })

  /** 聚类质量单卡（实时）。 */
  const quality: ComputedRef<QualityCard> = computed(() => {
    const sel = selectedCandidates.value
    const center = anchor.value
    if (sel.length === 0 || !center) {
      return { memberCount: sel.length, meanDistance: 0, outlierCount: 0 }
    }
    const distances = sel.map(c => cosineDistance(c.embedding, center))
    const meanDistance = distances.reduce((s, d) => s + d, 0) / distances.length
    const flags = outlierFlags(distances, matchThreshold.value)
    const outlierCount = flags ? flags.filter(Boolean).length : 0
    return { memberCount: sel.length, meanDistance, outlierCount }
  })

  /**
   * 保存前移出二次确认用的移出项：仅 active 归属（与 lanes 视图 sectionLaneKey 同口径——
   * candidate/archived 归属在 lanes 显示为未分类，不算移出）。originLabel 取原 active 泳道名。
   */
  const moveOutItems: ComputedRef<MoveOutItem[]> = computed(() => {
    const items: MoveOutItem[] = []
    for (const c of selectedCandidates.value) {
      if (c.persistentTopic?.status !== 'active') continue
      items.push({ label: c.clusterLabel, origin: c.persistentTopic.label ?? FALLBACK_ORIGIN })
    }
    return items
  })

  /** 选中里来源=unassigned vs 来源=active 泳道的计数。 */
  const counts: ComputedRef<{ unassigned: number, moveOut: number }> = computed(() => {
    let unassigned = 0
    let moveOut = 0
    for (const c of selectedCandidates.value) {
      if (c.persistentTopic?.status === 'active') moveOut++
      else unassigned++
    }
    return { unassigned, moveOut }
  })

  /** 候选话题侧边栏：activatable 置顶，再按 consecutive_hits desc。 */
  const sidebarItems: ComputedRef<SidebarCandidateItem[]> = computed(() =>
    [...opts.candidateTopics.value]
      .sort((a, b) => (Number(b.can_activate) - Number(a.can_activate))
        || (b.consecutive_hits - a.consecutive_hits))
      .map(topic => ({
        topic,
        activatable: topic.can_activate,
        brokenStreak: topic.consecutive_hits === 0,
      })),
  )

  /**
   * 相似 section 推荐：按当前主信号（activeSignal = anchor ?? queryVec）匹配未勾选 pool 节点，
   * 与主视图 nodeInfo 同信号源——主视图标注的 good(贴合,d≤threshold) 节点必然入选推荐。
   * distance≤matchThreshold 入选；分两组——unassigned 主组(top5) / active 次组(top3)，各按距离升序。
   * activeSignal=null（未勾选且未搜索）→ 两组皆空（侧边栏该区隐藏）。
   * active 项 originLabel 取原泳道名；点击即 toggle（active 项等同勾走移出，复用现有移出提示）。
   */
  const recommendations: ComputedRef<Recommendations> = computed(() => {
    const center = activeSignal.value
    if (!center) return { unassigned: [], active: [] }
    const threshold = matchThreshold.value
    const unassigned: RecommendedSection[] = []
    const active: RecommendedSection[] = []
    for (const c of pool.value) {
      if (selectedIds.value.has(c.id)) continue
      const d = cosineDistance(c.embedding, center)
      if (d > threshold) continue
      const isMoveOut = c.persistentTopic?.status === 'active'
      const item: RecommendedSection = {
        id: c.id,
        clusterLabel: c.clusterLabel,
        originLabel: isMoveOut ? (c.persistentTopic?.label ?? FALLBACK_ORIGIN) : null,
        distance: d,
      }
      if (isMoveOut) active.push(item)
      else unassigned.push(item)
    }
    unassigned.sort((a, b) => a.distance - b.distance)
    active.sort((a, b) => a.distance - b.distance)
    return {
      unassigned: unassigned.slice(0, RECOMMEND_UNASSIGNED_CAP),
      active: active.slice(0, RECOMMEND_ACTIVE_CAP),
    }
  })

  /** 推荐区标题：已选优先（anchor），否则搜索词（queryVec）；都无则空（区隐藏时不显示）。 */
  const recommendationTitle: ComputedRef<string> = computed(() => {
    if (anchor.value) return '与你已选最相近'
    if (queryVec.value) return '与搜索词最相近'
    return ''
  })

  /** 候选池语义排序：anchor 优先，否则 queryVec，都无则原序。 */
  const rankedPool: ComputedRef<ComposeCandidate[]> = computed(() =>
    rankCandidates(pool.value, anchor.value, queryVec.value),
  )

  // ── actions ─────────────────────────────────────────────────────────────

  /** getComposeCandidates → pool/matchThreshold；失败置 error。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await getComposeCandidates(opts.boardId.value, embedDays)
      if (res.success && res.data) {
        pool.value = res.data.sections
        matchThreshold.value = res.data.matchThreshold
      } else {
        error.value = res.error || '加载候选失败'
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载候选失败'
    } finally {
      loading.value = false
    }
  }

  /** active=true + load()。 */
  async function enter(): Promise<void> {
    active.value = true
    await load()
  }

  /** 清空编排态数据。 */
  function reset(): void {
    pool.value = []
    selectedIds.value = new Set()
    laneName.value = ''
    queryText.value = ''
    queryVec.value = null
    error.value = null
    searchError.value = null
    loading.value = false
    saving.value = false
    if (searchTimer) {
      clearTimeout(searchTimer)
      searchTimer = null
    }
  }

  /** reset + active=false。 */
  function exit(): void {
    reset()
    active.value = false
  }

  /** 增/删 selectedIds。 */
  function toggle(id: string): void {
    const next = new Set(selectedIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds.value = next
  }

  function isSelected(id: string): boolean {
    return selectedIds.value.has(id)
  }

  /**
   * 语义搜索：debounce 300ms → embedQuery → queryVec。
   * 空文本→clearSearch 语义；失败降级（queryVec=null + searchError），不抛、不阻断。
   */
  function runSearch(text: string): void {
    queryText.value = text
    if (searchTimer) {
      clearTimeout(searchTimer)
      searchTimer = null
    }
    const trimmed = text.trim()
    if (trimmed === '') {
      queryVec.value = null
      searchError.value = null
      return
    }
    const boardIdVal = opts.boardId.value
    searchTimer = setTimeout(async () => {
      try {
        const res = await embedQuery(boardIdVal, trimmed)
        if (res.success && res.data && res.data.embedding.length > 0) {
          queryVec.value = res.data.embedding
          searchError.value = null
        } else {
          queryVec.value = null
          searchError.value = res.error || '搜索向量生成失败'
        }
      } catch (e) {
        queryVec.value = null
        searchError.value = e instanceof Error ? e.message : '搜索向量生成失败'
      }
    }, SEARCH_DEBOUNCE_MS)
  }

  /** 清空搜索文本/向量/错误，取消挂起请求。 */
  function clearSearch(): void {
    if (searchTimer) {
      clearTimeout(searchTimer)
      searchTimer = null
    }
    queryText.value = ''
    queryVec.value = null
    searchError.value = null
  }

  /**
   * 采纳（并入新泳道）：预填 laneName=topic.label + 预勾该话题全部 owned section，
   * 再补相似待确认（cosineDistance≤matchThreshold 升序），总上限 adoptPreselectCap。
   * centroid=pool 中 persistentTopicId===topic.id 的 embedding（aggregatePreview.mean）；
   * 无 centroid 则跳过预勾。纯前端，无 API；不替换已有勾选。
   */
  async function adopt(item: SidebarCandidateItem): Promise<void> {
    laneName.value = item.topic.label
    const topicId = String(item.topic.id)
    const owned = pool.value.filter(c => c.persistentTopicId === topicId)
    const centroid = aggregatePreview(owned.map(c => c.embedding)).mean
    if (!centroid) return
    const ownedPicks = owned
      .filter(c => !selectedIds.value.has(c.id))
      .map(c => c.id)
      .slice(0, adoptPreselectCap)
    const similarPicks = pool.value
      .filter(c => c.persistentTopicId == null && !selectedIds.value.has(c.id))
      .map(c => ({ id: c.id, d: cosineDistance(c.embedding, centroid) }))
      .filter(x => x.d <= matchThreshold.value)
      .sort((a, b) => a.d - b.d)
      .map(x => x.id)
    const picks = [...new Set([...ownedPicks, ...similarPicks])].slice(0, adoptPreselectCap)
    if (picks.length === 0) return
    const next = new Set(selectedIds.value)
    for (const id of picks) next.add(id)
    selectedIds.value = next
  }

  /** 确认启用：updateTopic(status='active') → 成功 onSaved；失败置 error。 */
  async function activate(item: SidebarCandidateItem): Promise<void> {
    if (!item.activatable) return
    error.value = null
    try {
      const res = await updateTopic(item.topic.id, { status: 'active' })
      if (res.success) {
        await opts.onSaved()
      } else {
        error.value = res.error || '启用失败'
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '启用失败'
    }
  }

  /**
   * 保存：校验 laneName 非空 + selectedIds 非空；
   * moveOutItems.length>0 → requestMoveOutConfirm，!ok 中止；
   * createManualLane(boardId, laneName, [...selectedIds]) → 成功 onSaved + exit；失败置 error。
   */
  async function save(): Promise<void> {
    if (laneName.value.trim() === '') {
      error.value = '请输入泳道名'
      return
    }
    if (selectedIds.value.size === 0) {
      error.value = '请至少选择一条 section'
      return
    }
    if (moveOutItems.value.length > 0) {
      const ok = await opts.requestMoveOutConfirm(moveOutItems.value)
      if (!ok) return
    }
    saving.value = true
    error.value = null
    try {
      const res = await createManualLane(
        opts.boardId.value,
        laneName.value.trim(),
        [...selectedIds.value],
      )
      if (res.success) {
        await opts.onSaved(res.data?.topic)
        exit()
      } else {
        error.value = res.error || '保存失败'
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '保存失败'
    } finally {
      saving.value = false
    }
  }

  /** reset + exit（无副作用，不发 API）。 */
  function cancel(): void {
    exit()
  }

  return {
    active,
    pool,
    matchThreshold,
    selectedIds,
    laneName,
    queryText,
    queryVec,
    loading,
    saving,
    error,
    searchError,
    selectedCandidates,
    anchor,
    activeSignal,
    nodeInfo,
    quality,
    moveOutItems,
    counts,
    sidebarItems,
    recommendations,
    recommendationTitle,
    rankedPool,
    enter,
    exit,
    load,
    toggle,
    isSelected,
    runSearch,
    clearSearch,
    adopt,
    activate,
    save,
    cancel,
  }
}
