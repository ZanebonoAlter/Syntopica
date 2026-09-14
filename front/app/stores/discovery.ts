/**
 * 订阅源发现 store — 推荐卡片 + 目录状态的服务端状态与写操作。
 *
 * 写操作在此通知（失败只此一层 toast）；卡片列表仅服务发现页，
 * 与 useApiStore 的 feeds 数据隔离（接受成功后由页面层触发 feeds 重拉）。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useDiscoveryApi } from '~/api/discovery'
import { useFeedsApi } from '~/api/feeds'
import { useNotify } from '~/composables/useNotify'
import type {
  AskStatus,
  CandidateFilters,
  CandidateFormInput,
  CatalogStatus,
  DiscoveryCandidate,
  DiscoveryHistoryItem,
  DiscoveryInterest,
  DiscoveryRecommendation,
  DiscoveryRun,
  SaveCandidateResult,
} from '~/types/discovery'

/** 候选订阅提交结果：成功 | 已存在（409，视为已订阅不重复建） | 失败（保留输入可重试） */
export type SubscribeResult =
  | { status: 'subscribed', title: string }
  | { status: 'already', title?: string }
  | { status: 'error', error: string }

export const useDiscoveryStore = defineStore('discovery', () => {
  const api = useDiscoveryApi()
  const notify = useNotify()

  const cards = ref<DiscoveryRecommendation[]>([])
  const catalogStatus = ref<CatalogStatus | null>(null)
  const loading = ref(false)
  const refreshing = ref(false)
  const syncingCatalog = ref(false)
  /** 接受/拒绝进行中的卡片 id（防重复点击） */
  const actingIds = ref<string[]>([])

  async function loadRecommendations() {
    loading.value = true
    const res = await api.getRecommendations('pending')
    loading.value = false
    if (res.success && res.data) {
      cards.value = res.data
    } else {
      notify.error(res.error || '加载推荐失败')
    }
  }

  async function loadCatalogStatus() {
    const res = await api.getCatalogStatus()
    if (res.success && res.data) {
      catalogStatus.value = res.data
    }
  }

  async function refresh() {
    refreshing.value = true
    const res = await api.refreshRecommendations()
    refreshing.value = false
    if (res.success && res.data) {
      const s = res.data
      if (s.inserted > 0) {
        notify.success(`换了一批新推荐（新增 ${s.inserted} 条）`)
      } else if (s.candidates === 0) {
        notify.warn('暂无可推荐的内容：先同步目录，或用问答告诉我你想看什么')
      } else {
        notify.warn('没有新的推荐了，过几天再试试')
      }
      await loadRecommendations()
    } else {
      notify.error(res.error || '刷新推荐失败')
    }
  }

  /* —————————— 手动查询独立 run（design D2/D9；test-cases S1） ——————————
   * 查询结果独立展示（askRun），不写入 cards、不重拉个性化推荐；
   * 失败保留 cards 与输入，由组件层呈现「未更新 + 重试」。 */

  /** run 轮询节奏：间隔 2s、上限 60 次（≈2 分钟）——对应 design D2 的 run 轮询约定；超时转失败并提示手动刷新。 */
  const RUN_POLL_INTERVAL_MS = 2000
  const RUN_POLL_MAX_ATTEMPTS = 60

  const askStatus = ref<AskStatus>('idle')
  /** 最近一次查询的 run（成功后结果数据源；执行中为 null） */
  const askRun = ref<DiscoveryRun | null>(null)
  const askError = ref<string | null>(null)
  /** 最近一次查询输入（失败重试复用同一输入） */
  const lastQuery = ref('')
  const lastRunId = ref<string | null>(null)
  /** 独立结果视图开关（存 store：切页签再回来不丢） */
  const askViewOpen = ref(false)

  function sleep(ms: number) {
    return new Promise<void>(resolve => setTimeout(resolve, ms))
  }

  /**
   * 发起手动查询：POST ask 启动 run → 轮询 runs/:id 至终态（succeeded/failed/超时）。
   * 执行中重复提交直接拒绝（S1 防重复）；成功后静默刷新兴趣列表（成功查询形成独立兴趣记录）。
   */
  async function submitQuery(question: string): Promise<boolean> {
    const q = question.trim()
    if (!q || askStatus.value === 'running') return false
    askStatus.value = 'running'
    askViewOpen.value = true
    askError.value = null
    askRun.value = null
    lastQuery.value = q
    const started = await api.askDiscovery(q)
    if (!started.success || !started.data) {
      askStatus.value = 'failed'
      askError.value = started.error || '查询没有发起成功'
      return false
    }
    lastRunId.value = started.data.runId
    for (let attempt = 1; attempt <= RUN_POLL_MAX_ATTEMPTS; attempt++) {
      const res = await api.getRun(started.data.runId)
      if (res.success && res.data) {
        const run = res.data
        if (run.status === 'succeeded') {
          askRun.value = run
          askStatus.value = 'succeeded'
          // 成功查询形成独立兴趣记录：后台静默刷新兴趣列表（不阻塞结果展示）
          if (interestsLoaded.value) void loadInterests()
          return true
        }
        if (run.status === 'failed') {
          askStatus.value = 'failed'
          askError.value = '本次查询执行失败，没有产生新的推荐。'
          return false
        }
        // running / 未识别状态：继续轮询
      }
      // 单次轮询失败（网络抖动等）不立刻判死，计入次数上限内继续
      if (attempt < RUN_POLL_MAX_ATTEMPTS) await sleep(RUN_POLL_INTERVAL_MS)
    }
    askStatus.value = 'failed'
    askError.value = '查询超时未返回结果。可以重试，或稍后在「为你推荐」手动刷新。'
    return false
  }

  /** 关闭独立结果视图，回到个性化推荐（查询结果与状态保留，不清空） */
  function closeRunView() {
    askViewOpen.value = false
  }

  /* —————————— 兴趣记录（R3：逐条独立，不合成平均画像） —————————— */

  const interests = ref<DiscoveryInterest[]>([])
  const interestsLoading = ref(false)
  /** 请求失败信息；非 null = 错误态（可重试，不冒称空） */
  const interestsError = ref<string | null>(null)
  const interestsLoaded = ref(false)

  /** GET /api/discovery/interests — 失败保留旧数据 + 错误态，不触发 toast（组件层内联展示）。 */
  async function loadInterests(): Promise<boolean> {
    interestsLoading.value = true
    const res = await api.getInterests()
    interestsLoading.value = false
    if (res.success && res.data) {
      interests.value = res.data
      interestsLoaded.value = true
      interestsError.value = null
      return true
    }
    interestsError.value = res.error || '兴趣记录加载失败'
    return false
  }

  /* —————————— 推荐历史（R5：自动过期 ≠ 拒绝；恢复仅恢复资格） —————————— */

  const history = ref<DiscoveryHistoryItem[]>([])
  const historyLoading = ref(false)
  const historyError = ref<string | null>(null)
  const historyLoaded = ref(false)
  /** 恢复提交中的条目 id（防连点） */
  const restoringIds = ref<string[]>([])

  async function loadHistory(): Promise<boolean> {
    historyLoading.value = true
    const res = await api.getRecommendationHistory()
    historyLoading.value = false
    if (res.success && res.data) {
      history.value = res.data
      historyLoaded.value = true
      historyError.value = null
      return true
    }
    historyError.value = res.error || '推荐历史加载失败'
    return false
  }

  /** 恢复长期排除：仅恢复推荐资格，不自动订阅、不立即出卡（R5「恢复不是订阅」）。 */
  async function restoreRecommendation(id: string): Promise<boolean> {
    if (restoringIds.value.includes(id)) return false
    const target = history.value.find(h => h.id === id)
    if (!target) return false
    restoringIds.value = [...restoringIds.value, id]
    const res = await api.restoreRecommendation(id)
    restoringIds.value = restoringIds.value.filter(x => x !== id)
    if (res.success) {
      target.status = 'restored'
      notify.success('已恢复推荐资格：是否再次出现由后续筛选决定，不会自动订阅。')
      return true
    }
    notify.error(res.error || '恢复失败，请稍后重试')
    return false
  }

  /* —————————— 接受 / 拒绝 / 目录同步 —————————— */

  async function accept(id: string, opts: { categoryId?: string, parameters?: Record<string, string> } = {}): Promise<boolean> {
    if (actingIds.value.includes(id)) return false
    actingIds.value = [...actingIds.value, id]
    const res = await api.acceptRecommendation(id, opts)
    actingIds.value = actingIds.value.filter(x => x !== id)
    if (res.success && res.data) {
      cards.value = cards.value.filter(c => c.id !== id)
      notify.success(`已订阅「${res.data.title || res.data.url}」`)
      return true
    }
    notify.error(res.error || '订阅失败')
    return false
  }

  async function dismiss(id: string): Promise<boolean> {
    if (actingIds.value.includes(id)) return false
    actingIds.value = [...actingIds.value, id]
    const res = await api.dismissRecommendation(id)
    actingIds.value = actingIds.value.filter(x => x !== id)
    if (res.success) {
      cards.value = cards.value.filter(c => c.id !== id)
      return true
    }
    notify.error(res.error || '操作失败')
    return false
  }

  async function syncCatalog(): Promise<boolean> {
    syncingCatalog.value = true
    const res = await api.syncCatalog()
    syncingCatalog.value = false
    if (res.success && res.data) {
      const s = res.data
      if (s.Total > 0) {
        notify.success(`目录同步完成，共 ${s.Total} 条路由（新增 ${s.Inserted} / 更新 ${s.Updated}）`)
      } else {
        notify.warn('目录源暂时不可达，稍后再试')
      }
      await loadCatalogStatus()
      return s.Total > 0
    }
    notify.error(res.error || '目录同步失败')
    return false
  }

  /* —————————— 候选源库（feed-candidate-catalog） ——————————
   * 保存/启停均不触订阅（C7）；请求失败 ≠ 空库，错误态与空库态分开。 */

  const candidates = ref<DiscoveryCandidate[]>([])
  const candidatesTotal = ref(0)
  const candidatesLoading = ref(false)
  /** 请求失败信息；非 null = 错误态（不冒称空库），旧数据保留 */
  const candidatesError = ref<string | null>(null)
  /** 至少成功加载过一次（空库判定前提） */
  const candidatesLoaded = ref(false)
  const candidateFilters = ref<CandidateFilters>({ query: '', kind: 'all', participation: 'all' })
  /** 保存中（弹窗提交防连点） */
  const candidateSaving = ref(false)
  /** 启停提交中的候选 id（防连点；失败回滚原值） */
  const candidateTogglingIds = ref<string[]>([])

  async function loadCandidates(): Promise<boolean> {
    candidatesLoading.value = true
    const res = await api.getCandidates({ ...candidateFilters.value })
    candidatesLoading.value = false
    if (res.success && res.data) {
      candidates.value = res.data
      candidatesTotal.value = res.pagination?.total ?? res.data.length
      candidatesLoaded.value = true
      candidatesError.value = null
      return true
    }
    // 失败保留旧数据、置错误态：不展示虚构零计数，也不当作空库
    candidatesError.value = res.error || '候选源库加载失败'
    return false
  }

  function setCandidateFilters(patch: Partial<CandidateFilters>) {
    candidateFilters.value = { ...candidateFilters.value, ...patch }
    void loadCandidates()
  }

  function clearCandidateFilters() {
    candidateFilters.value = { query: '', kind: 'all', participation: 'all' }
    void loadCandidates()
  }

  const candidateFiltersActive = computed(() => {
    const f = candidateFilters.value
    return f.query.trim() !== '' || f.kind !== 'all' || f.participation !== 'all'
  })

  /** 新增/编辑候选：仅入库不订阅；重复地址返回已有条目，不静默新建。 */
  async function saveCandidate(input: CandidateFormInput, id?: string): Promise<SaveCandidateResult> {
    if (candidateSaving.value) return { status: 'error', error: '正在保存，请稍候' }
    candidateSaving.value = true
    const res = id ? await api.updateCandidate(id, input) : await api.createCandidate(input)
    candidateSaving.value = false
    if (res.success && res.data) {
      // 保存到候选库 ≠ 订阅：文案明示「尚未订阅」（C7）
      notify.success(`已保存到候选库（${res.data.name}），尚未订阅`)
      await loadCandidates()
      return { status: 'saved', candidate: res.data }
    }
    if (res.status === 409) {
      return { status: 'duplicate', existing: res.data ?? null, error: res.error }
    }
    return { status: 'error', error: res.error || '保存失败，请稍后重试' }
  }

  /** 推荐启停：仅控制推荐资格，不政订阅；乐观更新 + 失败恢复原值。 */
  async function setCandidateEnabled(id: string, enabled: boolean): Promise<boolean> {
    if (candidateTogglingIds.value.includes(id)) return false
    const target = candidates.value.find(c => c.id === id)
    if (!target) return false
    const original = target.recommendationEnabled
    candidateTogglingIds.value = [...candidateTogglingIds.value, id]
    target.recommendationEnabled = enabled
    const res = await api.updateCandidate(id, { recommendationEnabled: enabled })
    candidateTogglingIds.value = candidateTogglingIds.value.filter(x => x !== id)
    if (res.success) {
      notify.success(enabled ? '已恢复参与推荐，不影响已有订阅' : '已暂停参与推荐，已有订阅不受影响')
      return true
    }
    // 失败恢复原值 + 重试提示（C7 停用与订阅独立）；恢复说明始终随错误给出
    target.recommendationEnabled = original
    notify.error(`${res.error || '操作失败'}，已恢复原状态，请重试`)
    return false
  }

  /* —————————— 候选订阅（5.3，R6：原生确认即建 / RSSHub 需填参验证后建） ——————
   * 候选库与查询 run 条目不是推荐卡片（无 recommendation id），订阅走通用建源端点
   * POST /api/feeds（按 URL 精确去重，409 = 已存在）；需参数路由先 POST /feeds/fetch
   * 验证可解析再建（「验证成功后才创建」）。成功后本地标记 subscribed 防重复提交。 */

  const feedsApi = useFeedsApi()
  /** 订阅提交中的候选 id（防连点） */
  const subscribingIds = ref<string[]>([])
  /** 查询 run 条目订阅成功的本地标记（run 响应无 subscribed 字段，会话内防重复） */
  const subscribedRunIds = ref<string[]>([])

  /**
   * 提交候选订阅：verify=true 时先 fetch 验证地址可解析（需参数路由），通过后建源。
   * 409 = 地址已在订阅列表 → already（不冒称失败，也不重复创建）。
   */
  async function subscribeFeed(input: {
    candidateId: string
    url: string
    title?: string
    categoryId?: string
    verify?: boolean
  }): Promise<SubscribeResult> {
    if (subscribingIds.value.includes(input.candidateId)) {
      return { status: 'error', error: '正在提交，请稍候' }
    }
    if (!input.url) return { status: 'error', error: '订阅地址为空，无法提交' }
    subscribingIds.value = [...subscribingIds.value, input.candidateId]
    try {
      if (input.verify) {
        const check = await feedsApi.fetchFeed(input.url)
        if (!check.success) {
          return { status: 'error', error: `地址验证未通过：${check.error || '无法解析为 RSS/Atom'}` }
        }
      }
      const created = await feedsApi.createFeed({
        url: input.url,
        title: input.title || undefined,
        category_id: input.categoryId ? Number(input.categoryId) : undefined,
      })
      if (created.success && created.data) {
        notify.success(`已订阅「${created.data.title || input.url}」`)
        return { status: 'subscribed', title: created.data.title || '' }
      }
      if (created.status === 409) {
        // 相同有效地址重复确认不创建重复订阅（R6）：视为已订阅
        notify.warn('该地址已在订阅列表中，不会重复创建')
        return { status: 'already' }
      }
      return { status: 'error', error: created.error || '订阅失败，请稍后重试' }
    } finally {
      subscribingIds.value = subscribingIds.value.filter(x => x !== input.candidateId)
    }
  }

  /** 订阅成功后的本地标记：候选库行 + run 条目都置为已订阅 */
  function markSubscribed(candidateId: string) {
    const row = candidates.value.find(c => c.id === candidateId)
    if (row && !row.subscribed) row.subscribed = true
    if (!subscribedRunIds.value.includes(candidateId)) {
      subscribedRunIds.value = [...subscribedRunIds.value, candidateId]
    }
  }

  return {
    cards,
    catalogStatus,
    loading,
    refreshing,
    syncingCatalog,
    actingIds,
    loadRecommendations,
    loadCatalogStatus,
    refresh,
    submitQuery,
    closeRunView,
    askStatus,
    askRun,
    askError,
    lastQuery,
    lastRunId,
    askViewOpen,
    interests,
    interestsLoading,
    interestsError,
    interestsLoaded,
    loadInterests,
    history,
    historyLoading,
    historyError,
    historyLoaded,
    restoringIds,
    loadHistory,
    restoreRecommendation,
    accept,
    dismiss,
    syncCatalog,
    candidates,
    candidatesTotal,
    candidatesLoading,
    candidatesError,
    candidatesLoaded,
    candidateFilters,
    candidateFiltersActive,
    candidateSaving,
    candidateTogglingIds,
    loadCandidates,
    setCandidateFilters,
    clearCandidateFilters,
    saveCandidate,
    setCandidateEnabled,
    subscribingIds,
    subscribedRunIds,
    subscribeFeed,
    markSubscribed,
  }
})
