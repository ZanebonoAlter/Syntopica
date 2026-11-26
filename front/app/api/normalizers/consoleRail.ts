/**
 * 控制台右轨读模型 normalizer（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 既有 health-checkup / source-breakdown 端点沿用 snake_case wire 协议（不改共享 API、
 * 不改旧管理面板 DTO）。新增右轨/单级宽工作面只在 API 边界消费归一后的 camelCase +
 * 字符串 ID 视图，组件/template 不再出现 `llm_score`/`aux_id`/`feed_id` 等 raw 字段。
 */
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import type { BoardSourceBreakdown } from '~/types'

/** 右轨构成候选（辅助标签；字符串 ID 与 camelCase）。 */
export interface RailHealthCandidateView {
  auxId: string
  label: string
  memberTags: number
  memberArticles: number
}

/** 右轨体检快照只读视图（不含标签级长表；全量在既有 BoardHealthDetail）。 */
export interface RailHealthSnapshotView {
  score: number | null
  checkedAt: string
  comment: string
  matchedTags: number
  ghostMatches: number
  vectorMissing: number
  candidates: RailHealthCandidateView[]
}

export function normalizeRailHealth(snapshot: BoardHealthSnapshotDTO | null): RailHealthSnapshotView | null {
  if (!snapshot) return null
  const board = snapshot.metrics.board
  return {
    score: snapshot.llm_score ?? null,
    checkedAt: snapshot.checked_at ?? '',
    comment: snapshot.llm_comment ?? '',
    matchedTags: board?.matched_tags ?? 0,
    ghostMatches: board?.ghost_matches ?? 0,
    vectorMissing: board?.vector_missing ?? 0,
    candidates: (snapshot.uncaptured_candidates ?? []).map(candidate => ({
      auxId: String(candidate.aux_id),
      label: candidate.label,
      memberTags: candidate.member_tags,
      memberArticles: candidate.member_articles,
    })),
  }
}

/** 来源行只读视图（feedId 字符串；统计口径仍由 sourcestats 唯一实现）。 */
export interface RailSourceView {
  feedId: string
  title: string
  articles: number
  share: number
  feedArticles: number
  feedHitRate: number
}

export interface RailSourceBreakdownView {
  totalArticles: number
  sourceCount: number
  sources: RailSourceView[]
}

export function normalizeRailSourceBreakdown(breakdown: BoardSourceBreakdown | null): RailSourceBreakdownView | null {
  if (!breakdown) return null
  return {
    totalArticles: breakdown.total_articles ?? 0,
    sourceCount: breakdown.source_count ?? 0,
    sources: (breakdown.sources ?? []).map(source => ({
      feedId: String(source.feed_id),
      title: source.title ?? '',
      articles: source.articles ?? 0,
      share: source.share ?? 0,
      feedArticles: source.feed_articles ?? 0,
      feedHitRate: source.feed_hit_rate ?? 0,
    })),
  }
}
