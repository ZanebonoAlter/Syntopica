import { describe, expect, it } from 'vitest'
import { normalizeBoardKeyEvent, normalizeBoardKeyEvents } from './normalizers/boardKeyEvents'

describe('normalizeBoardKeyEvents — 版块级关键事件归一', () => {
  it('空数组与 null 容器归一为空数组（不冒充失败）', () => {
    expect(normalizeBoardKeyEvents([])).toEqual([])
    expect(normalizeBoardKeyEvents(null)).toEqual([])
    expect(normalizeBoardKeyEvents(undefined)).toEqual([])
  })

  it('单条：camelCase、date-only、ID 字符串化、null 快照字段兑底', () => {
    const item = normalizeBoardKeyEvent({
      id: 12,
      persistent_topic_id: 93,
      source_period_date: '2026-11-18T00:00:00Z',
      topic_label_snapshot: 'AI 模型发布',
      title_snapshot: 'GPT-6.1 Terra 发布',
      summary_snapshot: '主打电脑操作与降价。',
      article_ids_snapshot: [101, 102],
      state: 'confirmed',
    })
    expect(item).toEqual({
      id: '12',
      topicId: '93',
      topicLabelSnapshot: 'AI 模型发布',
      sourcePeriodDate: '2026-11-18',
      titleSnapshot: 'GPT-6.1 Terra 发布',
      summarySnapshot: '主打电脑操作与降价。',
      articleIds: ['101', '102'],
    })
  })

  it('null 快照：无话题归属与无文章都是合法态（空数组/空串，不造 0）', () => {
    const item = normalizeBoardKeyEvent({
      id: 13,
      persistent_topic_id: null,
      source_period_date: '2026-11-19T00:00:00Z',
      topic_label_snapshot: '',
      title_snapshot: '来源被重建替换的注记',
      summary_snapshot: '',
      article_ids_snapshot: null,
      state: 'confirmed',
    })
    expect(item.topicId).toBeNull()
    expect(item.articleIds).toEqual([])
    expect(item.sourcePeriodDate).toBe('2026-11-19')
  })

  it('多话题同日三条：保序归一（同日多事件顺序 = 接口序）', () => {
    const items = normalizeBoardKeyEvents([
      { id: 3, persistent_topic_id: 7, source_period_date: '2026-11-12T00:00:00Z', topic_label_snapshot: 'A', title_snapshot: '三', summary_snapshot: '', article_ids_snapshot: null, state: 'confirmed' },
      { id: 1, persistent_topic_id: 9, source_period_date: '2026-11-12T00:00:00Z', topic_label_snapshot: 'B', title_snapshot: '一', summary_snapshot: '', article_ids_snapshot: [5], state: 'confirmed' },
      { id: 2, persistent_topic_id: 7, source_period_date: '2026-11-12T00:00:00Z', topic_label_snapshot: 'A', title_snapshot: '二', summary_snapshot: '', article_ids_snapshot: null, state: 'confirmed' },
    ])
    expect(items.map(item => item.id)).toEqual(['3', '1', '2'])
    expect(items[1]!.articleIds).toEqual(['5'])
  })
})
