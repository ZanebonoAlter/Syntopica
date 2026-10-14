/**
 * 站内文章搜索 API（add-article-search）—— GET /api/search 唯一前端边界。
 *
 * 后端契约：q / archived=include|active-only / expand=off|on / page / per_page。
 * 响应体按 change 契约为顶层 camelCase（results/semantic/expand/total/perPage/tookMs），
 * 结果项复用 articles 列表窄投影字段。因后端与前端并行开发、且既有 articles 列表在
 * wire 上是 snake_case + success/data 包裹，本层对两种形态与两种命名宽容归一：
 * snake_case → camelCase、数字 id → string 只发生在这里（红线：组件层零映射）。
 */

import { apiClient } from './client'
import { normalizeArticle, type ArticlePayload } from './normalizers/article'
import type { ApiResponse, SearchHit, SearchHitReason, SearchResponseData, SemanticTagHit } from '~/types'

export interface SearchQueryParams {
  q: string
  archived?: 'include' | 'active-only'
  expand?: 'off' | 'on'
  page?: number
  perPage?: number
}

type WireRecord = Record<string, unknown>

function isRecord(value: unknown): value is WireRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function toArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function toInt(value: unknown): number | null {
  const n = typeof value === 'string' ? Number(value) : value
  return typeof n === 'number' && Number.isFinite(n) ? n : null
}

/** 双命名取值：wire 字段可能 camelCase（契约形态）或 snake_case（Go 既有惯例） */
function pick(raw: WireRecord, camel: string, snake: string): unknown {
  const v1 = raw[camel]
  if (v1 !== undefined && v1 !== null) return v1
  const v2 = raw[snake]
  return v2
}

function pickString(raw: WireRecord, camel: string, snake: string): string {
  const v = pick(raw, camel, snake)
  return typeof v === 'string' ? v : ''
}

/** 结果项 → ArticlePayload（复用既有 normalizeArticle 完成 id 字符串化 + camelCase） */
function toArticlePayload(raw: WireRecord): ArticlePayload {
  return {
    id: toInt(pick(raw, 'id', 'id')) ?? 0,
    feed_id: toInt(pick(raw, 'feedId', 'feed_id')) ?? 0,
    title: pickString(raw, 'title', 'title'),
    description: pickString(raw, 'description', 'description'),
    content: pickString(raw, 'content', 'content'),
    excerpt: pickString(raw, 'excerpt', 'excerpt'),
    link: pickString(raw, 'link', 'link'),
    pub_date: pickString(raw, 'pubDate', 'pub_date'),
    created_at: pickString(raw, 'createdAt', 'created_at'),
    author: (pick(raw, 'author', 'author') as string | undefined) ?? undefined,
    category_id: (toInt(pick(raw, 'categoryId', 'category_id')) ?? undefined) as number | undefined,
    read: pick(raw, 'read', 'read') === true,
    favorite: pick(raw, 'favorite', 'favorite') === true,
    summary_status: (pick(raw, 'summaryStatus', 'summary_status') as string | undefined) ?? undefined,
    summary_generated_at: (pick(raw, 'summaryGeneratedAt', 'summary_generated_at') as string | undefined) ?? undefined,
    firecrawl_status: (pick(raw, 'firecrawlStatus', 'firecrawl_status') as string | undefined) ?? undefined,
    firecrawl_error: (pick(raw, 'firecrawlError', 'firecrawl_error') as string | undefined) ?? undefined,
    firecrawl_crawled_at: (pick(raw, 'firecrawlCrawledAt', 'firecrawl_crawled_at') as string | undefined) ?? undefined,
    image_url: (pick(raw, 'imageUrl', 'image_url') as string | undefined) ?? undefined,
    tag_count: (toInt(pick(raw, 'tagCount', 'tag_count')) ?? undefined) as number | undefined,
  }
}

const HIT_REASON_TYPES: ReadonlySet<string> = new Set(['keyword', 'semantic', 'expand'])

function toHitReasons(raw: unknown): SearchHitReason[] {
  return toArray(raw)
    .filter(isRecord)
    .map(item => ({
      type: item.type as SearchHitReason['type'],
      text: typeof item.text === 'string' ? item.text : '',
      score: typeof item.score === 'number' ? item.score : undefined,
    }))
    .filter(item => HIT_REASON_TYPES.has(item.type) && item.text !== '')
}

function toSearchHit(raw: unknown): SearchHit {
  if (!isRecord(raw)) {
    return { article: normalizeArticle(toArticlePayload({})), archived: false, hitReasons: [] }
  }
  return {
    article: normalizeArticle(toArticlePayload(raw)),
    archived: pick(raw, 'archived', 'archived') === true,
    hitReasons: toHitReasons(pick(raw, 'hitReasons', 'hit_reasons')),
  }
}

function toSemanticTags(raw: unknown): SemanticTagHit[] {
  return toArray(raw)
    .filter(isRecord)
    .map(tag => ({ label: typeof tag.label === 'string' ? tag.label : '', score: toInt(tag.score) ?? 0 }))
    .filter(tag => tag.label !== '')
}

export function normalizeSearchResponse(raw: unknown): SearchResponseData {
  const body: WireRecord = isRecord(raw) ? raw : {}
  const semantic = pick(body, 'semantic', 'semantic')
  const semanticBlock: WireRecord = isRecord(semantic) ? semantic : {}
  const expand = pick(body, 'expand', 'expand')
  const expandBlock: WireRecord = isRecord(expand) ? expand : {}
  const hits = toArray(pick(body, 'results', 'results')).map(toSearchHit)

  return {
    hits,
    total: toInt(pick(body, 'total', 'total')) ?? hits.length,
    page: toInt(pick(body, 'page', 'page')) ?? 1,
    perPage: toInt(pick(body, 'perPage', 'per_page')) ?? hits.length,
    tookMs: toInt(pick(body, 'tookMs', 'took_ms')),
    semanticSkipped: semanticBlock.skipped === true,
    semanticTags: semanticBlock.skipped === true ? [] : toSemanticTags(semanticBlock.tags),
    expandWords: toArray(expandBlock.words).filter((w): w is string => typeof w === 'string'),
  }
}

export function useSearchApi() {
  async function search(params: SearchQueryParams): Promise<ApiResponse<SearchResponseData>> {
    const query = apiClient.buildQueryParams({
      q: params.q,
      archived: params.archived,
      expand: params.expand,
      page: params.page,
      per_page: params.perPage,
    })
    const res = await apiClient.get<SearchResponseData>(`/search?${query}`)
    if (!res.success) {
      return { success: false, error: res.error, message: res.message, status: res.status }
    }
    // 顶层平铺契约形态（无 data 包裹）经 client extras 透传到响应顶层：res.data 缺省时回退整包。
    const payload = res.data ?? (res as unknown as WireRecord)
    return { success: true, data: normalizeSearchResponse(payload) }
  }

  return { search }
}
