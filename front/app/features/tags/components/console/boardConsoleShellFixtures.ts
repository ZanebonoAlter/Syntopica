/**
 * BoardConsoleShell 测试 fixture（8C G4b 从 1094 行巨型壳测试按 §8 抽出）。
 *
 * 只放纯数据构造（A 档响应/365 池/日报元信息/日报列表与详情）；模块 mock、路由桩与挂载
 * 装配在 `boardConsoleShellTestHarness.ts`。不得以 `*.test.ts` 命名（会被 Vitest 收集）。
 */
import type { LaneDynamicsResponse } from '~/api/laneDynamics'

export const TODAY = '2026-10-23'
export const BOARD_LABEL = '中东地缘政治'

export function lane(topicId: number, label: string, days: Record<string, number>) {
  return {
    topic_id: topicId,
    label,
    watch_linked: topicId === 10,
    section_count_14d: 0,
    snapshot: null,
    timeline: Object.entries(days).map(([date, count]) => ({
      date,
      sections: Array.from({ length: count }, (_, i) => ({
        section_id: topicId * 100 + i,
        label: `${label} ${date} 分区${i}`,
        thread_count: i + 1,
        events: [`${label} 事件${i}`],
      })),
    })),
  }
}

export function response(): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    lanes: [
      lane(10, '主线 A', { [TODAY]: 3 }),
      lane(11, '主线 B', { [TODAY]: 3 }),
      lane(12, '休眠线 C', { '2026-10-20': 2 }),
    ],
    candidates: [{ topic_id: 30, label: '候选话题', last_seen_date: TODAY, recent_hint: '连续命中' }],
  }
}

export function emptyResponse(): LaneDynamicsResponse {
  return { window_days: 14, has_reports: true, lanes: [], candidates: [] }
}

export function poolResponse(): LaneDynamicsResponse {
  return {
    window_days: 365,
    has_reports: true,
    lanes: [{
      ...lane(10, '主线 A', {}),
      timeline: [{
        date: '2026-06-15',
        sections: [{ section_id: 9999, label: '历史桶', thread_count: 2, events: ['历史事件'] }],
      }],
    }],
    candidates: [],
  }
}

export function overview(reportDate: string | null = TODAY) {
  return {
    success: true,
    data: {
      date: TODAY,
      boards: [
        {
          boardId: '1974',
          label: BOARD_LABEL,
          tagCount: 128,
          reportDate: reportDate ?? '',
          reportTitle: reportDate ? '已有日报' : '',
          reportArticleCount: 2,
          reportEventCount: 2,
          anchorDate: reportDate ?? undefined,
          todayClues: 0,
          last3Clues: 0,
          prev3Clues: 0,
          attention: 0,
          dataDelayed: false,
          clueDays: [],
          lanes: [],
        },
        {
          boardId: '1975',
          label: 'AI 与机器学习',
          tagCount: 96,
          reportDate: '2026-10-22',
          reportTitle: 'AI 日报',
          reportArticleCount: 1,
          reportEventCount: 1,
          anchorDate: '2026-10-22',
          todayClues: 0,
          last3Clues: 0,
          prev3Clues: 0,
          attention: 0,
          dataDelayed: false,
          clueDays: [],
          lanes: [],
        },
      ],
    },
  }
}

export function reportList() {
  return {
    success: true,
    data: {
      reports: [
        { id: 60, semantic_board_id: 1974, period_date: TODAY, title: '已有日报', created_at: '' },
      ],
    },
  }
}

export function reportDetail() {
  return {
    success: true,
    data: {
      report: {
        id: 60,
        semantic_board_id: 1974,
        period_date: TODAY,
        title: '已有日报',
        summary: '',
        status: 'published',
        cluster_count: 1,
        article_count: 2,
        event_tag_count: 2,
        highlights: [],
        dynamics: '',
        sections: [
          {
            id: 1000,
            cluster_index: 0,
            cluster_label: '主线 A',
            cluster_tag_ids: [],
            article_count: 2,
            best_tier: 1,
            avg_score: 1,
            threads: [
              {
                id: 1,
                report_id: 60,
                section_id: 1000,
                title: '完整线索标题',
                summary: '真实摘要逐字内容',
                tag_ids: [],
                confidence: 1,
                related_article_ids: [42],
                created_at: '',
              },
            ],
          },
        ],
        created_at: '',
      },
    },
  }
}
