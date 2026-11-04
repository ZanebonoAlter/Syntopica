import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DailyReportMasthead from './DailyReportMasthead.vue'
import type { DailyReport, DailyReportHighlight } from '~/api/dailyReports'

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

function mountMasthead(over: ReportOverrides = {}) {
  return mount(DailyReportMasthead, { props: { report: makeReport(over) } })
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
