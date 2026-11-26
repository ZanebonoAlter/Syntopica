/**
 * 控制台右轨 normalizer（矩阵阶段 G5）：health/source 既有 snake_case wire DTO →
 * camelCase + 字符串 ID 读模型；空快照/零值不补造。
 */
import { describe, expect, it } from 'vitest'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import type { BoardSourceBreakdown } from '~/types'
import { normalizeRailHealth, normalizeRailSourceBreakdown } from './consoleRail'

function snapshot(): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-23T23:00:00+08:00',
    metrics: {
      board: { matched_tags: 128, reason_dist: {}, ghost_matches: 9, vector_missing: 2 },
      labels: [],
    },
    uncaptured_candidates: [
      { aux_id: 7, label: '候选标签', member_tags: 20, member_articles: 30 },
    ],
    llm_score: 85,
    llm_comment: '评语。',
    created_at: '',
  }
}

function breakdown(): BoardSourceBreakdown {
  return {
    total_articles: 65,
    source_count: 1,
    sources: [
      { feed_id: 12, title: '通讯社', articles: 30, share: 30 / 65, feed_articles: 60, feed_hit_rate: 0.5 },
    ],
  }
}

describe('consoleRail normalizer', () => {
  it('health：camelCase + 字符串 auxId，候选/指标逐字段保留', () => {
    expect(normalizeRailHealth(snapshot())).toEqual({
      score: 85,
      checkedAt: '2026-10-23T23:00:00+08:00',
      comment: '评语。',
      matchedTags: 128,
      ghostMatches: 9,
      vectorMissing: 2,
      candidates: [{ auxId: '7', label: '候选标签', memberTags: 20, memberArticles: 30 }],
    })
  })

  it('health：无快照返回 null（等待首轮，不伪 0）', () => {
    expect(normalizeRailHealth(null)).toBeNull()
  })

  it('sources：字符串 feedId + camelCase；总量/来源数不重算', () => {
    expect(normalizeRailSourceBreakdown(breakdown())).toEqual({
      totalArticles: 65,
      sourceCount: 1,
      sources: [
        { feedId: '12', title: '通讯社', articles: 30, share: 30 / 65, feedArticles: 60, feedHitRate: 0.5 },
      ],
    })
    expect(normalizeRailSourceBreakdown(null)).toBeNull()
  })
})
