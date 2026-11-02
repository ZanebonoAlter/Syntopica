/**
 * BoardConsoleShell 集成用例 fixture（8C G4b）：当日详情/其他分区/报日索引/观察带构造。
 *
 * 只放纯数据与 mock 装配（不含断言）；故事与断言在 `BoardConsoleShell.integration.test.ts`。
 * 独立于旧壳 fixture（`boardConsoleShellFixtures.ts`）——旧的 38 例资产保持原样。
 */
import { flushPromises } from '@vue/test-utils'
import {
  getLaneDynamics,
  getBoardDailyReports,
  getDailyReportDetail,
  mountShell,
  response,
  TODAY,
} from './boardConsoleShellTestHarness'

export const EARLIER = '2026-10-21'

export function thread(sectionId: number, id: number, title: string, summary: string) {
  return {
    id,
    report_id: 60,
    section_id: sectionId,
    title,
    summary,
    tag_ids: [],
    confidence: 1,
    related_article_ids: [],
    created_at: '',
  }
}

export function section(options: {
  id: number
  clusterIndex: number
  label: string
  threads: ReturnType<typeof thread>[]
  persistentTopicId?: number | null
  statusAtReport?: string | null
}) {
  return {
    id: options.id,
    cluster_index: options.clusterIndex,
    cluster_label: options.label,
    cluster_tag_ids: [],
    article_count: 1,
    best_tier: 1,
    avg_score: 1,
    persistent_topic_id: options.persistentTopicId ?? null,
    topic_status_at_report: options.statusAtReport ?? null,
    threads: options.threads,
  }
}

export function detail(reportId: number, date: string, sections: unknown[]) {
  return {
    success: true,
    data: {
      report: {
        id: reportId,
        semantic_board_id: 1974,
        period_date: date,
        title: `${date} 日报`,
        summary: '',
        status: 'published',
        cluster_count: sections.length,
        article_count: 2,
        event_tag_count: 2,
        highlights: [],
        dynamics: '',
        sections,
        created_at: '',
      },
    },
  }
}

/** 当日详情：1000 = 正式泳道 10 的桶（不进其他组），1100/1101 = 其他话题行。 */
export function laneSection(laneTitle = '完整线索标题') {
  return section({
    id: 1000, clusterIndex: 0, label: '主线 A', persistentTopicId: 10, statusAtReport: 'active',
    threads: [thread(1000, 1, laneTitle, '真实摘要逐字内容')],
  })
}

export function otherSectionA() {
  return section({
    id: 1200, clusterIndex: 1, label: '别的分区甲',
    threads: [
      thread(1200, 11, '其他线索一', '其他摘要一'),
      thread(1200, 12, '其他线索二', '其他摘要二'),
    ],
  })
}

export function otherSectionB() {
  return section({
    id: 1201, clusterIndex: 2, label: '别的分区乙', persistentTopicId: 77, statusAtReport: 'archived',
    threads: [thread(1201, 21, '观察线索', '观察摘要')],
  })
}

export function reportIndex() {
  return {
    success: true,
    data: {
      reports: [
        { id: 59, semantic_board_id: 1974, period_date: EARLIER, title: '前一日报', created_at: '' },
        { id: 60, semantic_board_id: 1974, period_date: TODAY, title: '已有日报', created_at: '' },
      ],
    },
  }
}

/** 详情按 report id 分支：59 = 前一日报（观察节点目标分区 2100），60 = 今日。 */
export function mockDetails(options: { laneTitle?: string, withOtherA?: boolean } = {}) {
  getDailyReportDetail.mockImplementation((id: number) => Promise.resolve(
    id === 59
      ? detail(59, EARLIER, [
          section({
            id: 2100, clusterIndex: 0, label: '观察甲', persistentTopicId: 77, statusAtReport: 'active',
            threads: [thread(2100, 31, '前日线索', '前日摘要')],
          }),
        ])
      : detail(60, TODAY, [
          laneSection(options.laneTitle),
          ...(options.withOtherA === false ? [] : [otherSectionA()]),
          otherSectionB(),
        ]),
  ))
}

export function mature() {
  getLaneDynamics.mockResolvedValue({ success: true, data: response() })
  getBoardDailyReports.mockResolvedValue(reportIndex())
  mockDetails()
}

export async function mountReady(): Promise<ReturnType<typeof mountShell>> {
  const wrapper = mountShell()
  await flushPromises()
  await flushPromises()
  return wrapper
}
