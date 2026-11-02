/**
 * laneDayRelatedArticles — 相关文章按需读取的条目状态（G4 CA-6/CA-8 + R2）。
 *
 * 从 `useLaneDayReading` 按内聚职责抽出（code-style §8），由父 store 创建一次并委托；
 * 文章缓存本体（≤20 FIFO、在途合乘、失败不入成功缓存）仍由父 store 注入的同一份缓存持有，
 * 本模块只管「articleId → 条目状态」的发布与晚到隔离。合同逐条继承：
 *  - 单项状态 idle/loading/success/error，失败单项可重试，互不影响且总数语义留在组件层；
 *  - **跨板块/卸载晚到**：发布前按 board 与板块代际双道判定，晚到的成功不发布、晚到的失败
 *    不冒成新板块错误（缓存失效代由 `res.stale` 标记）；
 *  - 加载标题不标已读、不动 reading_behaviors；已缓存 payload 可被站内阅读复用（`getArticlePayload`）。
 *
 * 不拥有：网络/归一化（父 store 注入的缓存与 fetcher）、UI 展开组、文章缓存容量策略。
 */
import { ref, type Ref } from 'vue'
import type { LaneDayArticleMeta } from '~/api/normalizers/laneDayReading'
import type { BoundedRequestCache } from './laneDayReadingCache'

/** 相关文章条目状态（按需；成功/失败分层、单项可重试，展示总数始终真实）。 */
export type RelatedArticleEntry =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success', meta: LaneDayArticleMeta }
  | { status: 'error', error: string }

export interface LaneDayRelatedArticlesDeps {
  /** 板块 ID（晚到响应与请求时的板块比对）。 */
  boardId: () => number
  /** 板块代际（父 store 的切板块/卸载递增）；晚到成功/失败都不发布。 */
  boardEpoch: () => number
  /** 同一份文章缓存（≤20 FIFO + 在途合乘；本模块不建第二份）。 */
  articleCache: BoundedRequestCache<string, LaneDayArticleMeta>
}

export interface LaneDayRelatedArticlesReader {
  /** 相关文章条目（articleId → 状态；同文章跨 thread 复用，板块作用域、独立于缓存逐出）。 */
  relatedEntries: Ref<Map<string, RelatedArticleEntry>>
  /** 单条目状态（组件渲染入口；未触达的 id = idle）。 */
  readRelated: (articleId: string) => RelatedArticleEntry
  /** 按需展开一批相关文章（同文章在途去重由缓存层合乘）。 */
  ensureRelatedArticles: (articleIds: string[]) => Promise<void>
  /** 失败单项重试（不影响其他条目与总数）。 */
  retryRelatedArticle: (articleId: string) => Promise<void>
  /** G5 站内阅读复用：因取标题已缓存的文章完整 payload（未缓存返回 null，不发请求）。 */
  getArticlePayload: (articleId: string) => LaneDayArticleMeta | null
  /** 诊断：文章缓存条目数。 */
  cacheSize: () => number
  /** 切板块/卸载：清条目与文章缓存（同一生命周期）。 */
  reset: () => void
}

export function createLaneDayRelatedArticles(deps: LaneDayRelatedArticlesDeps): LaneDayRelatedArticlesReader {
  const relatedEntries = ref(new Map<string, RelatedArticleEntry>())

  function publishRelated(articleId: string, entry: RelatedArticleEntry): void {
    const next = new Map(relatedEntries.value)
    next.set(articleId, entry)
    relatedEntries.value = next
  }

  /** 单条目状态（组件渲染入口；未触达的 id = idle）。 */
  function readRelated(articleId: string): RelatedArticleEntry {
    return relatedEntries.value.get(articleId) ?? { status: 'idle' }
  }

  async function ensureRelatedArticle(articleId: string, force = false): Promise<void> {
    const boardId = deps.boardId()
    const epoch = deps.boardEpoch()
    publishRelated(articleId, { status: 'loading' })
    const res = await deps.articleCache.load(articleId, { force })
    // 跨板块/卸载晚到：不发布成功，也不把缓存失效结果冒成新板块的错误
    if (epoch !== deps.boardEpoch() || deps.boardId() !== boardId || res.stale) return
    if (res.ok && res.data) {
      publishRelated(articleId, { status: 'success', meta: res.data })
    } else {
      publishRelated(articleId, { status: 'error', error: res.error || '加载失败' })
    }
  }

  /** 按需展开一批相关文章（同文章在途去重由缓存层合乘）。 */
  async function ensureRelatedArticles(articleIds: string[]): Promise<void> {
    const unique = [...new Set(articleIds)]
    await Promise.all(unique.map(id => ensureRelatedArticle(id)))
  }

  /** 失败单项重试（不影响其他条目与总数）。 */
  function retryRelatedArticle(articleId: string): Promise<void> {
    return ensureRelatedArticle(articleId, true)
  }

  /** G5 站内阅读复用：因取标题已缓存的文章完整 payload（未缓存返回 null，不发请求）。 */
  function getArticlePayload(articleId: string): LaneDayArticleMeta | null {
    return deps.articleCache.peek(articleId)
  }

  function reset(): void {
    relatedEntries.value = new Map()
    deps.articleCache.clear()
  }

  return {
    relatedEntries,
    readRelated,
    ensureRelatedArticles,
    retryRelatedArticle,
    getArticlePayload,
    cacheSize: () => deps.articleCache.size(),
    reset,
  }
}
