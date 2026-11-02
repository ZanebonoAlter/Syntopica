/**
 * Shared match-quality helpers for the match-score surface (archive view) and
 * the daily-report quality explore panel.
 *
 * Colours are derived from theme tokens so they follow the editorial/dark
 * themes. A downgraded match halves the token's opacity via `color-mix` — this
 * preserves the existing 50%-opacity visual (previously `color + '80'` hex
 * alpha) without hard-coded hex, since alpha can no longer be appended to a
 * `var()` token.
 */

/** Structural shape for a matched tag. Accepts both {@link BoardArticleTag}
 *  (tags surface) and a `quality_breakdown` entry (daily-report explore panel). */
export interface MatchInfoEntry {
  match_reason: string
  score: number
  downgraded?: boolean
}

const MATCH_REASON_TOKENS: Record<string, string> = {
  direct_hit: 'var(--color-match-direct-hit)',
  hit_rate: 'var(--color-match-hit-rate)',
  max_sim: 'var(--color-match-max-sim)',
  weighted: 'var(--color-match-weighted)',
  composite_hit: 'var(--color-match-composite)',
}

/** Theme-token colour for a match reason. Pass `downgraded` for a 50%-opacity
 *  variant (used for downgraded chip borders / labels). */
export function matchReasonColor(reason: string, downgraded?: boolean): string {
  const token = MATCH_REASON_TOKENS[reason] ?? 'var(--color-match-weighted)'
  return downgraded
    ? `color-mix(in srgb, ${token} 50%, transparent)`
    : token
}

/** Human-readable label + score for a matched tag, e.g. "相似度 0.85↓". */
export function matchInfoLabel(tag: MatchInfoEntry): string {
  const labels: Record<string, string> = {
    direct_hit: '直接命中',
    hit_rate: '命中率',
    max_sim: '相似度',
    weighted: '综合',
    composite_hit: '组合命中',
  }
  return `${labels[tag.match_reason] || tag.match_reason} ${tag.score.toFixed(2)}${tag.downgraded ? '↓' : ''}`
}

/**
 * Matching-flow step numbers (archive rework, 2026-10-31).
 *
 * The generic pipeline is ⓪组合命中 → ①精确匹配 → ②余弦相似度 → ③命中率 →
 * ④最高相似度 → ⑤综合加权, and a match stops at the first step that hits.
 * The step number tells "靠第几步进入版块"; ② has no match reason of its own
 * (cosine similarity feeds ③/④), hence the gaps in the numbering.
 */
const STEP_NO: Record<string, string> = {
  composite_hit: '⓪',
  direct_hit: '①',
  hit_rate: '③',
  max_sim: '④',
  weighted: '⑤',
}

/** Short labels of the same steps, used next to the step number. */
const STEP_SHORT: Record<string, string> = {
  composite_hit: '组合',
  direct_hit: '精确',
  hit_rate: '命中率',
  max_sim: '相似度',
  weighted: '加权',
}

/** Step number of a match reason (unknown reason → '·'). */
export function stepNoOf(reason: string): string {
  return STEP_NO[reason] ?? '·'
}

/** Short step label of a match reason (unknown reason → the raw reason). */
export function stepShortOf(reason: string): string {
  return STEP_SHORT[reason] ?? reason
}

/**
 * Tag category labels (archive rework v10: composition tags are not limited to
 * events — keyword / person / org are labelled truthfully as well).
 */
export const CAT_LABEL: Record<string, string> = {
  event: '事件',
  keyword: '关键词',
  person: '人物',
  org: '组织',
}

/** Category label, falling back to the raw category (empty → '标签'). */
export function catLabel(category: string): string {
  return CAT_LABEL[category] || category || '标签'
}
