import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DailyReportMasthead from './DailyReportMasthead.vue'
import type { DailyReport, DailyReportHighlight, DailyReportSection } from '~/api/dailyReports'

// highlights 的非数组形态（null / 字段缺失）来自历史坏数据与接口异常，
// 前端契约层必须兜底成空态占位而不是整块隐藏。
type ReportOverrides = Partial<Omit<DailyReport, 'highlights'>> & {
  highlights?: DailyReportHighlight[] | null
}

function makeReport(over: ReportOverrides = {}): DailyReport {
  return {
    id: 1303,
    semantic_board_id: 1980,
    period_date: '2026-11-03T00:00:00Z',
    title: '11 月 3 日日报',
    summary: '本期概要。',
    status: 'done',
    cluster_count: 1,
    article_count: 3,
    event_tag_count: 2,
    highlights: [],
    dynamics: '',
    sections: [],
    created_at: '2026-11-03T00:00:00Z',
    ...over,
  } as unknown as DailyReport
}

function makeSection(id: number, threads: { id: number, tag_ids?: number[] }[]): DailyReportSection {
  return {
    id,
    cluster_index: 0,
    cluster_label: `分区${id}`,
    cluster_tag_ids: [],
    article_count: threads.length,
    best_tier: 0,
    avg_score: 0.9,
    threads: threads.map(({ id: threadId, tag_ids: tagIds }) => ({
      id: threadId,
      report_id: 1303,
      section_id: id,
      title: `线索${threadId}`,
      summary: '',
      tag_ids: tagIds ?? [],
      confidence: 0.9,
      related_article_ids: [],
      created_at: '2026-11-03T12:00:00Z',
    })),
  } as DailyReportSection
}

function mountMasthead(over: ReportOverrides = {}) {
  return mount(DailyReportMasthead, { props: { report: makeReport(over) } })
}

function clusterHighlight(over: Partial<DailyReportHighlight> = {}): DailyReportHighlight {
  return {
    title: '要闻',
    reason: '理由',
    tag_ids: [55],
    cluster_id: 55,
    article_count: 6,
    cluster_mode: '向量聚簇',
    data_as_of: '2026-11-03',
    research_at: '2026-11-03T21:30:00+08:00',
    ...over,
  } as DailyReportHighlight
}

describe('DailyReportMasthead — 今日重点空态可见性', () => {
  // B18：正常态渲染要闻，且不出现空态占位
  it('renders highlights without empty state', () => {
    const wrapper = mountMasthead({
      highlights: [
        { title: '要闻一', reason: '理由一', tag_ids: [] },
        { title: '要闻二', reason: '理由二', tag_ids: [] },
        { title: '要闻三', reason: '理由三', tag_ids: [] },
      ],
    })

    expect(wrapper.findAll('.drm-highlight')).toHaveLength(3)
    expect(wrapper.find('[data-testid="highlights-empty"]').exists()).toBe(false)
    // 正常态保留原锚点，且 id 唯一（空态与正常态互斥渲染）
    expect(wrapper.find('#report-highlights').exists()).toBe(true)
    expect(wrapper.findAll('#report-highlights')).toHaveLength(1)
  })

  // B16：highlights 为空数组 → 渲染空态占位，不渲染要闻条目
  it('renders empty state when highlights empty', () => {
    const wrapper = mountMasthead({ highlights: [] })

    const empty = wrapper.find('[data-testid="highlights-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('本期暂无重点')
    expect(wrapper.findAll('.drm-highlight')).toHaveLength(0)
    expect(wrapper.findAll('#report-highlights')).toHaveLength(1)
  })

  // B17：highlights 为 null（历史坏数据 / 接口异常）→ 同空态占位，且不抛错
  it('renders empty state when highlights null', () => {
    const wrapper = mountMasthead({ highlights: null })

    const empty = wrapper.find('[data-testid="highlights-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('本期暂无重点')
    expect(wrapper.findAll('.drm-highlight')).toHaveLength(0)
    expect(wrapper.findAll('#report-highlights')).toHaveLength(1)
  })
})

// ── retire-story-adopt-event-metrics 3.3：聚簇快照依据与正文定位 ──

describe('DailyReportMasthead — 重点依据（聚簇快照）与正文定位', () => {
  it('依据行 = 「N 篇来源 · 聚簇口径 · 数据截至日 · 研究时点」；真实 0 篇也显示', () => {
    const wrapper = mountMasthead({ highlights: [clusterHighlight()], sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])] })
    const meta = wrapper.find('[data-testid="highlight-meta-0"]')
    expect(meta.text()).toContain('6 篇来源')
    expect(meta.text()).toContain('聚簇口径 向量聚簇')
    expect(meta.text()).toContain('数据截至 2026-11-03')
    expect(meta.text()).toContain('研究时点 11/03 21:30')

    const zero = mountMasthead({
      highlights: [clusterHighlight({ article_count: 0 })],
      sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])],
    })
    expect(zero.find('[data-testid="highlight-meta-0"]').text()).toContain('0 篇来源')
  })

  it('缺字段整体省略（不显示 0/空括号/undefined）；非法 research_at 省略', () => {
    const wrapper = mountMasthead({
      highlights: [{
        title: '旧重点',
        reason: '理由',
        tag_ids: [],
        cluster_id: 55,
        research_at: 'not-a-date',
      } as DailyReportHighlight],
      sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])],
    })
    // 全部字段缺失 → 依据行整体不渲染（不是空段落、不是 0/undefined 占位）
    expect(wrapper.find('[data-testid="highlight-meta-0"]').exists()).toBe(false)
    const article = wrapper.find('.drm-highlight')
    expect(article.text()).not.toContain('篇来源')
    expect(article.text()).not.toContain('undefined')
    expect(article.text()).not.toContain('聚簇口径')
    expect(article.text()).not.toContain('数据截至')
    expect(article.text()).not.toContain('研究时点')
  })

  it('点击定位：有 cluster_id 且正文有对应线索 → 键盘可达按钮，点击 emit locate(clusterId)', async () => {
    const wrapper = mountMasthead({
      highlights: [clusterHighlight()],
      sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])],
    })
    const button = wrapper.find('[data-testid="highlight-locate-0"]')
    expect(button.element.tagName).toBe('BUTTON')
    expect(button.attributes('title')).toContain('聚簇')
    await button.trigger('click')
    expect(wrapper.emitted('locate')).toEqual([[55]])
  })

  it('划选非折叠选区时点击不定位（只当文本操作）', async () => {
    const wrapper = mountMasthead({
      highlights: [clusterHighlight()],
      sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])],
    })
    const selectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false } as Selection)
    await wrapper.find('[data-testid="highlight-locate-0"]').trigger('click')
    expect(wrapper.emitted('locate')).toBeUndefined()
    selectionSpy.mockRestore()
  })

  it('cluster_id 有但正文无对应线索 → 不渲染按钮，明示「本期正文暂无对应线索」（不跳假位置）', () => {
    const wrapper = mountMasthead({
      highlights: [clusterHighlight({ cluster_id: 77 })],
      sections: [makeSection(366, [{ id: 10, tag_ids: [55] }])],
    })
    expect(wrapper.find('[data-testid="highlight-locate-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="highlight-meta-0"]').text()).toContain('本期正文暂无对应线索')
  })

  it('旧数据兼容：无 cluster_id（标签回退模式）→ 无按钮、无依据行，外观与现状一致', () => {
    const wrapper = mountMasthead({
      highlights: [{ title: '旧重点', reason: '理由', tag_ids: [] }],
      sections: [makeSection(366, [{ id: 10 }])],
    })
    const article = wrapper.find('.drm-highlight')
    expect(article.find('button').exists()).toBe(false)
    expect(wrapper.find('[data-testid="highlight-meta-0"]').exists()).toBe(false)
    expect(article.text()).not.toContain('篇来源')
    expect(article.text()).not.toContain('聚簇口径')
    expect(article.text()).not.toContain('进展')
    expect(article.text()).not.toContain('暂无对应线索')
    expect(article.find('h3').text()).toBe('旧重点')
  })
})
