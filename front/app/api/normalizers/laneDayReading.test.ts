/**
 * laneDayReading normalizer 测试（矩阵阶段 G4）。
 * 覆盖：字符串 ID/camelCase 归一、join 命中/未命中/零线索合法、fallback 模型不借别日、
 * 文章元信息子集（不造来源名）。
 */
import { describe, expect, it } from 'vitest'
import {
  buildLaneDayFallbackModel,
  buildLaneDayJoinedModel,
  normalizeArticleMeta,
  normalizeReportDetail,
  normalizeReportIndexItem,
} from './laneDayReading'
import type { DailyReport, DailyReportListItem } from '../dailyReports'
import type { WindowLaneDay } from './laneDynamicsWindow'

function day(sections: Partial<WindowLaneDay['sections'][number]>[]): WindowLaneDay {
  return {
    date: '2026-10-22',
    sections: sections.map((section, index) => ({
      sectionId: section.sectionId ?? String(index + 1),
      label: section.label ?? `桶${index + 1}`,
      threadCount: section.threadCount ?? null,
      fallbackClueCount: section.fallbackClueCount ?? null,
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? [],
    })),
  }
}

const REPORT: DailyReport = {
  id: 1211,
  semantic_board_id: 1974,
  period_date: '2026-10-22',
  title: '日报',
  summary: '',
  status: 'completed',
  cluster_count: 5,
  article_count: 8,
  event_tag_count: 3,
  highlights: [],
  dynamics: '',
  created_at: '2026-10-23T04:00:00Z',
  sections: [
    {
      id: 501,
      cluster_index: 0,
      cluster_label: '海峡局势',
      cluster_tag_ids: [],
      threads: [
        { id: 1, report_id: 1211, section_id: 501, title: '演习升级', summary: '摘要甲', tag_ids: [], confidence: 0.82, related_article_ids: [11, 12], created_at: '' },
        { id: 2, report_id: 1211, section_id: 501, title: '演习升级', summary: '同名不同 ID 的独立摘要', tag_ids: [], confidence: 0.4, related_article_ids: [], created_at: '', fit_distance: 1.2 },
      ],
      article_count: 3,
      best_tier: 1,
      avg_score: 0.8,
    },
    { id: 502, cluster_index: 1, cluster_label: '油价', cluster_tag_ids: [], threads: [], article_count: 0, best_tier: 2, avg_score: 0.5, bucket_title: '能源方向' },
  ],
}

describe('normalize — 历史快照字段（8C G1）', () => {
  it('section 带 clusterIndex/topicStatusAtReport/persistentTopicId（字符串 ID、原值保留）', () => {
    const detail = normalizeReportDetail({
      ...REPORT,
      sections: [
        {
          ...REPORT.sections[0]!,
          cluster_index: 2,
          topic_status_at_report: 'candidate',
          persistent_topic_id: 1204,
        },
        { ...REPORT.sections[1]!, cluster_index: 0, topic_status_at_report: 'ACTIVE' },
      ],
    })
    expect(detail.sections[0]!.clusterIndex).toBe(2)
    expect(detail.sections[0]!.topicStatusAtReport).toBe('candidate')
    expect(detail.sections[0]!.persistentTopicId).toBe('1204')
    expect(detail.sections[0]).not.toHaveProperty('persistent_topic_id')
    // 未知/大小写异常原值透传（降级由纯模型决定，边界不猜）
    expect(detail.sections[1]!.topicStatusAtReport).toBe('ACTIVE')
    expect(detail.sections[1]!.persistentTopicId).toBeNull()
  })

  it('旧日报缺字段：clusterIndex/topicStatusAtReport/persistentTopicId 全为 null，不补造', () => {
    const legacy = normalizeReportDetail({
      ...REPORT,
      sections: [{
        id: 600,
        cluster_label: '旧分区',
        cluster_tag_ids: [],
        threads: [],
        article_count: 0,
        best_tier: 0,
        avg_score: 0,
      } as unknown as DailyReport['sections'][number]],
    })
    const section = legacy.sections[0]!
    expect(section.clusterIndex).toBeNull()
    expect(section.topicStatusAtReport).toBeNull()
    expect(section.persistentTopicId).toBeNull()
    expect(section.clusterLabel).toBe('旧分区')
    expect(section.threads).toEqual([])
  })
})

describe('normalize — 字符串 ID/camelCase 归一', () => {
  it('报告列表条目：数字 id → 字符串 reportId，periodDate 透传', () => {
    const item: DailyReportListItem = {
      id: 1211, semantic_board_id: 1974, period_date: '2026-10-22', title: '日报', summary: '',
      status: 'completed', cluster_count: 5, article_count: 8, event_tag_count: 3, created_at: '2026-10-23T04:00:00Z',
    }
    const normalized = normalizeReportIndexItem(item)
    expect(normalized.reportId).toBe('1211')
    expect(normalized.periodDate).toBe('2026-10-22')
    expect(normalized).not.toHaveProperty('period_date')
  })

  it('报告详情：thread/section ID 字符串化、fitDistance 缺省为 null、bucketTitle 保留', () => {
    const detail = normalizeReportDetail(REPORT)
    expect(detail.reportId).toBe('1211')
    expect(detail.boardId).toBe('1974')
    const first = detail.sections[0]!
    expect(first.sectionId).toBe('501')
    expect(first.threads[0]!.relatedArticleIds).toEqual(['11', '12'])
    expect(first.threads[0]!.fitDistance).toBeNull()
    expect(first.threads[1]!.fitDistance).toBe(1.2)
    expect(detail.sections[1]!.bucketTitle).toBe('能源方向')
  })
})

describe('join — 稳定 sectionId 命中', () => {
  it('全部命中 → ready；同名不同 threadId 各自保留；0 线索桶合法', () => {
    const model = buildLaneDayJoinedModel(day([{ sectionId: '501' }, { sectionId: '502' }]), {
      reportId: '1211',
      detail: normalizeReportDetail(REPORT),
    })
    expect(model.status).toBe('ready')
    expect(model.missingSectionCount).toBe(0)
    expect(model.sections[0]!.threads).toHaveLength(2)
    expect(model.sections[0]!.threads[0]!.summary).toBe('摘要甲')
    expect(model.sections[0]!.threads[1]!.summary).toBe('同名不同 ID 的独立摘要')
    expect(model.sections[1]!.threads).toHaveLength(0)
    expect(model.sections[1]!.matched).toBe(true)
  })

  it('部分未命中 → degraded；未命中桶保留 base 标题与折叠数', () => {
    const model = buildLaneDayJoinedModel(
      day([
        { sectionId: '501' },
        { sectionId: '999', eventTitles: ['旧标题一', '旧标题二'], foldedCount: 3 },
      ]),
      { reportId: '1211', detail: normalizeReportDetail(REPORT) },
    )
    expect(model.status).toBe('degraded')
    expect(model.missingSectionCount).toBe(1)
    expect(model.foldedTotal).toBe(3)
    const missing = model.sections[1]!
    expect(missing.matched).toBe(false)
    expect(missing.fallbackTitles).toEqual(['旧标题一', '旧标题二'])
  })

  it('fallback（no-report/error）：sections 全部按 base 降级、不借别日', () => {
    const model = buildLaneDayFallbackModel(day([{ sectionId: '501', eventTitles: ['仅有的标题'], foldedCount: 2 }]), {
      status: 'no-report',
    })
    expect(model.status).toBe('no-report')
    expect(model.reportId).toBeNull()
    expect(model.sections[0]!.matched).toBe(false)
    expect(model.sections[0]!.fallbackTitles).toEqual(['仅有的标题'])
    expect(model.foldedTotal).toBe(2)
  })
})

describe('normalizeArticleMeta — 元信息子集', () => {
  it('标题/时间/完整 payload（G5 复用）；不造来源名字段', () => {
    const meta = normalizeArticleMeta({
      id: 11, feed_id: 3, title: '伊朗宣布演习', link: 'https://example.com/a', pub_date: '2026-10-22T09:00:00+08:00',
      created_at: '2026-10-22T10:00:00+08:00', read: false, favorite: false,
    })
    expect(meta.articleId).toBe('11')
    expect(meta.title).toBe('伊朗宣布演习')
    expect(meta.pubDate).toBe('2026-10-22T09:00:00+08:00')
    expect(meta.article.id).toBe('11')
    expect(meta.article.content).toBe('')
    expect(meta).not.toHaveProperty('sourceName')
  })

  it('空标题回退（无标题）', () => {
    const meta = normalizeArticleMeta({
      id: 12, feed_id: 3, title: '', link: '', pub_date: '', created_at: '', read: false, favorite: false,
    })
    expect(meta.title).toBe('(无标题)')
  })
})
