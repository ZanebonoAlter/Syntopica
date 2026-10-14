import type { LocationQuery } from 'vue-router'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import type { ApiResponse } from '~/types'
import type { Article } from '~/types/article'

export interface ArticleDeepLinkDeps {
  /** 详情直取（articlesApi.getArticle）——按 id 取，不依赖列表成员资格（搜索结果多为归档文章，不在首页默认列表） */
  getArticle: (id: number) => Promise<ApiResponse<unknown>>
  /** 深链文章落地回调（FeedLayoutShell.handleArticleClick：窄屏进阅读态/hydrate/标已读全复用） */
  onArticle: (article: Article) => void | Promise<void>
}

/** 合法正整数字符串（含前导零形式，Number 后须 > 0） */
const POSITIVE_INT_RE = /^\d+$/

/**
 * `/?article=<id>` 深链编排（add-article-search D1）：
 * 在首页初始数据加载完成后调用一次。query.article 为合法正整数字符串时按 id
 * 详情直取，normalize（id 字符串化/camelCase）后交给 onArticle 进入现有阅读路径；
 * 缺参/非法/详情失败（404/网络）一律静默忽略，普通进首页零变化。
 *
 * query 容忍 undefined：useRoute 在无 router 上下文（单测直挂组件）时返回 undefined。
 */
export async function applyArticleDeepLink(
  query: LocationQuery | undefined,
  deps: ArticleDeepLinkDeps,
): Promise<void> {
  const raw = query?.article
  if (typeof raw !== 'string' || !POSITIVE_INT_RE.test(raw) || Number(raw) === 0) return
  try {
    const response = await deps.getArticle(Number(raw))
    if (!response.success || !response.data) return
    await deps.onArticle(normalizeArticle(response.data as unknown as ArticlePayload))
  } catch {
    // 详情失败（404/网络）：静默忽略，不打断首页正常进入
  }
}
