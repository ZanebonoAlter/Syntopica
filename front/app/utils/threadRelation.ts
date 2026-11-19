/**
 * 线索相关推荐（thread-relations）的展示口径：档位文案与分数格式。
 *
 * 分数是余弦相似度（服务端不减权、不取整）；`level` 由服务端按 `score >= 0.85`
 * 判 strong，前端只负责显示。**显示分数 MUST NOT 向上舍入**——0.8499 四舍五入成
 * `0.85` 会被读成「刚好达标」，与旁边的「相关」矛盾（0.85 恰是 strong 边界）。
 * 所以统一向零截断到两位小数（见 {@link formatRelationScore}）。
 *
 * 纯函数，无副作用、无 DOM 访问。
 */

/** 候选线索的相关度档位（服务端口径：`score >= 0.85` → strong）。 */
export type ThreadRelationLevel = 'strong' | 'related'

/** 相关线索窗口的两档天数（端点只认 7/30，其它值回落 7）。 */
export const RELATED_WINDOW_OPTIONS = [7, 30] as const

/** 默认窗口档位（分类工具条默认停在 7 天）。 */
export const RELATED_WINDOW_DEFAULT = 7

/** 档位中文文案（浮层与阅读预览弹窗共用，避免两处各写一套）。 */
export function relationLevelLabel(level: ThreadRelationLevel): string {
  return level === 'strong' ? '强关联' : '相关'
}

/**
 * 相关度分数显示：向零截断到两位小数，**绝不四舍五入**。
 *
 * 截断而不是 `toFixed(2)`：0.8499 舍入后是 `0.85`，恰好是 strong 边界，会与
 * 「相关」档位打架。`1e-9` 只用来抵消 `score * 100` 的浮点误差（如 0.86 存成
 * 85.99999999999999），量级远小于任何真实分数差。
 */
export function formatRelationScore(score: number): string {
  if (!Number.isFinite(score)) return '—'
  const clamped = Math.min(1, Math.max(0, score))
  return (Math.trunc(clamped * 100 + 1e-9) / 100).toFixed(2)
}

/** 阅读预览弹窗要带的相关度上下文（点击线索 chip 时随 openArticle 一起传）。 */
export interface RelatedPreviewContext {
  score: number
  level: ThreadRelationLevel
  /** 该候选所属日报日期（YYYY-MM-DD）。 */
  periodDate: string
}

/** openArticle 的入参：数字 = 普通文章行（无相关度上下文）；对象 = 带上下文的 chip 点击。 */
export type ArticleOpenTarget = number | (RelatedPreviewContext & { articleId: number })
