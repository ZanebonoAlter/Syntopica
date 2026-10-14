import { computed, ref } from 'vue'
import { useSearchApi } from '~/api/search'
import type { SearchHit, SemanticTagHit } from '~/types'

/**
 * 站内搜索页数据编排（add-article-search）：fetch + 状态机 + 过滤开关 + offset 分页。
 *
 * 状态机：initial（未搜索）→ loading（首查骨架屏）→ success | empty | error；
 * 追加分页用独立 loadingMore（不重置已有结果）；semanticSkipped 为语义路径
 * 静默降级标记（黄色提示条，不阻塞结果）。幂等防护两条：
 * - 输入不变（且过滤器不变）不重查（submit 按 q+archived+expand 指纹比对）；
 * - 进行中重复提交直接忽略（loading/loadingMore 期间守卫）。
 */

export type SearchStatus = 'initial' | 'loading' | 'empty' | 'error' | 'success'

const DEFAULT_PER_PAGE = 20

export function useSearch() {
  const { search } = useSearchApi()

  // ---- 输入与过滤器 ----
  const query = ref('')
  /** 含归档切换（默认开：站内搜索默认覆盖全部文章） */
  const includeArchived = ref(true)
  /** LLM 扩展搜索（默认关：用户手动开启） */
  const expandEnabled = ref(false)

  // ---- 状态与结果 ----
  const status = ref<SearchStatus>('initial')
  const loadingMore = ref(false)
  const hits = ref<SearchHit[]>([])
  const total = ref(0)
  const page = ref(1)
  const tookMs = ref<number | null>(null)
  /** embedding 不可用 → 后端静默跳过语义路径（降级提示条，不报错） */
  const semanticSkipped = ref(false)
  const semanticTags = ref<SemanticTagHit[]>([])
  const expandWords = ref<string[]>([])
  const errorMessage = ref('')

  // 最后一次已提交查询（过滤器变更/重试以此为准，而非当前输入框内容）
  let lastQuery = ''
  // 已执行查询指纹（q + archived + expand）：submit 幂等判据
  let executedKey = ''
  // 响应序号：新查询使旧响应（含进行中的 loadMore）过期丢弃
  let seq = 0

  const loading = computed(() => status.value === 'loading')
  const hasMore = computed(() => hits.value.length < total.value)

  function keyOf(q: string): string {
    return [
      q,
      includeArchived.value ? 'include' : 'active-only',
      expandEnabled.value ? 'on' : 'off',
    ].join('\u0000')
  }

  function applyResponse(data: {
    hits: SearchHit[]
    total: number
    page: number
    tookMs: number | null
    semanticSkipped: boolean
    semanticTags: SemanticTagHit[]
    expandWords: string[]
  }, append: boolean): void {
    hits.value = append ? [...hits.value, ...data.hits] : data.hits
    total.value = data.total
    page.value = data.page
    tookMs.value = data.tookMs
    semanticSkipped.value = data.semanticSkipped
    semanticTags.value = data.semanticTags
    expandWords.value = data.expandWords
  }

  /** 首查/重查：整页替换结果（q 来自已提交查询）；可打断进行中的 loadMore（seq 使旧响应过期） */
  async function runSearch(q: string): Promise<void> {
    loadingMore.value = false
    status.value = 'loading'
    errorMessage.value = ''
    const mySeq = ++seq
    const res = await search({
      q,
      archived: includeArchived.value ? 'include' : 'active-only',
      expand: expandEnabled.value ? 'on' : 'off',
      page: 1,
      perPage: DEFAULT_PER_PAGE,
    })
    if (mySeq !== seq) return // 期间有新查询发起，本响应过期
    if (!res.success || !res.data) {
      status.value = 'error'
      errorMessage.value = res.error || '搜索失败，请稍后重试'
      return
    }
    applyResponse(res.data, false)
    status.value = res.data.hits.length === 0 ? 'empty' : 'success'
  }

  /** 提交搜索（输入框回车/按钮）：空查询忽略；首查进行中忽略（重复提交防护）；
   *  输入不变不重查。loadMore 进行中不阻塞——新查询优先，旧追加响应经 seq 丢弃 */
  function submit(): void {
    const q = query.value.trim()
    if (!q) return
    if (loading.value) return
    const key = keyOf(q)
    if (key === executedKey) return
    executedKey = key
    lastQuery = q
    void runSearch(q)
  }

  /** 错误重试：强制重跑最后一次已提交查询（绕过幂等判据，保留输入） */
  function retry(): void {
    if (!lastQuery || loading.value) return
    void runSearch(lastQuery)
  }

  /** 追加下一页（offset 分页）：不重置已有结果；追加失败保持现状 */
  async function loadMore(): Promise<void> {
    if (!lastQuery || loading.value || loadingMore.value || !hasMore.value) return
    loadingMore.value = true
    const mySeq = ++seq
    const res = await search({
      q: lastQuery,
      archived: includeArchived.value ? 'include' : 'active-only',
      expand: expandEnabled.value ? 'on' : 'off',
      page: page.value + 1,
      perPage: DEFAULT_PER_PAGE,
    })
    loadingMore.value = false
    if (mySeq !== seq) return // 期间有新查询发起，本响应过期
    if (!res.success || !res.data) return
    applyResponse(res.data, true)
  }

  /** 过滤器变更后立即重查（以最后已提交查询为基准）；空态引导「开启扩展再试」亦经此路径。
   *  仅首查进行中跳过（重复提交防护）；loadMore 进行中不阻塞 */
  function requeryIfExecuted(): void {
    if (!lastQuery || loading.value) return
    executedKey = keyOf(lastQuery)
    void runSearch(lastQuery)
  }

  function setIncludeArchived(value: boolean): void {
    if (includeArchived.value === value) return
    includeArchived.value = value
    requeryIfExecuted()
  }

  function setExpandEnabled(value: boolean): void {
    if (expandEnabled.value === value) return
    expandEnabled.value = value
    requeryIfExecuted()
  }

  return {
    query,
    includeArchived,
    expandEnabled,
    status,
    loading,
    loadingMore,
    hits,
    total,
    page,
    tookMs,
    semanticSkipped,
    semanticTags,
    expandWords,
    errorMessage,
    hasMore,
    submit,
    retry,
    loadMore,
    setIncludeArchived,
    setExpandEnabled,
  }
}
