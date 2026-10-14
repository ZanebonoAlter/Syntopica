/**
 * 站内文章搜索类型（add-article-search）
 * 前端侧已归一化的 camelCase 形态；snake_case → camelCase 与 id 字符串化
 * 只发生在 api/search.ts 归一层（红线：组件层不得散落映射）。
 */

import type { Article } from './article'

/** 命中解释三型：关键词 / 语义标签 / LLM 扩展词 */
export type SearchHitReasonType = 'keyword' | 'semantic' | 'expand'

export interface SearchHitReason {
  type: SearchHitReasonType
  /** 命中文本（关键词原文 / 标签名 / 扩展词） */
  text: string
  /** 语义 cosine 分数或关键词相似度（可选） */
  score?: number
}

/** 一条搜索结果：列表投影 Article + 归档标记 + 命中解释 */
export interface SearchHit {
  article: Article
  archived: boolean
  hitReasons: SearchHitReason[]
}

/** 语义路径命中标签（cosine 分数） */
export interface SemanticTagHit {
  label: string
  score: number
}

/** 归一化后的 /api/search 响应数据 */
export interface SearchResponseData {
  hits: SearchHit[]
  total: number
  page: number
  perPage: number
  tookMs: number | null
  /** embedding 不可用 → 静默跳过语义路径（降级提示条，不阻塞） */
  semanticSkipped: boolean
  semanticTags: SemanticTagHit[]
  /** expand=off 或 LLM 失败时为空数组 */
  expandWords: string[]
}
