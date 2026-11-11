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

function makeSection(id: number, threads: { id: number, story_id?: number | null }[]): DailyReportSection {
  return {
    id,
    cluster_index: 0,
    cluster_label: `分区${id}`,
    cluster_tag_ids: [],
    article_count: threads.length,
    best_tier: 0,
    avg_score: 0.9,
    threads: threads.map(({ id: threadId, story_id: storyId }) => ({
      id: threadId,
      report_id: 1303,
      section_id: id,
      title: `线索${threadId}`,
      summary: '',
      tag_ids: [],
      confidence: 0.9,
      related_article_ids: [],
      created_at: '2026-11-03T12:00:00Z',
      ...(storyId !== undefined ? { story_id: storyId } : {}),
    })),
  } as DailyReportSection
}

function mountMasthead(over: ReportOverrides = {}) {
  return mount(DailyReportMasthead, { props: { report: makeReport(over) } })
}

function storyHighlight(over: Partial<DailyReportHighlight> = {}): DailyReportHighlight {
  return {
    title: '要闻',
    reason: '理由',
    tag_ids: [],
    story_id: 55,
    participant_count: 6,
    story_heat: 2.4,
    heat_rule_version: 'heat-v1-48h-hl24h',
    heat_as_of: '2026-11-03T23:59:59+08:00',
    heat_incomplete: false,
    is_progress: false,
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

// ── surface-story-heat-in-report-top：来源依据 / 进展徽标 / 点击定位 ──

describe('DailyReportMasthead — 重点依据与正文定位', () => {
  it('来源数显示：participant_count=6 → 「6 个来源」；真实 0 也显示；快照 title 附热度/截止', () => {
    const wrapper = mountMasthead({ highlights: [storyHighlight()] })
    const meta = wrapper.find('[data-testid="highlight-meta-0"]')
    expect(meta.exists()).toBe(true)
    expect(meta.text()).toContain('6 个来源')
    expect(meta.attributes('title')).toContain('48h 热度 2.4')
    expect(meta.attributes('title')).toContain('截止 2026-11-03')

    const zero = mountMasthead({ highlights: [storyHighlight({ participant_count: 0 })] })
    expect(zero.find('[data-testid="highlight-meta-0"]').text()).toContain('0 个来源')
  })

  it('快照缺失不显示数字（不伪造 0/undefined）；heat_incomplete 附提示', () => {
    const wrapper = mountMasthead({
      highlights: [{ title: '旧重点', reason: '理由', tag_ids: [], story_id: 55 } as DailyReportHighlight],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    expect(wrapper.find('[data-testid="highlight-meta-0"]').exists()).toBe(false)

    const incomplete = mountMasthead({
      highlights: [storyHighlight({ heat_incomplete: true })],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    expect(incomplete.find('[data-testid="highlight-meta-0"]').attributes('title')).toContain('热度证据不全')
  })

  it('进展徽标：is_progress=true → 标题旁「进展」+「昨日重点再次入选」；false/缺省不显示', () => {
    const progress = mountMasthead({
      highlights: [storyHighlight({ is_progress: true })],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    const badge = progress.find('[data-testid="highlight-progress-0"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('进展')
    expect(badge.attributes('title')).toContain('昨日重点再次入选')

    const plain = mountMasthead({
      highlights: [storyHighlight({ is_progress: false })],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    expect(plain.find('[data-testid="highlight-progress-0"]').exists()).toBe(false)
    const missing = mountMasthead({
      highlights: [storyHighlight({ is_progress: undefined })],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    expect(missing.find('[data-testid="highlight-progress-0"]').exists()).toBe(false)
  })

  it('点击定位：有 story_id 且正文有线程 → 键盘可达按钮，点击 emit locate(storyId)', async () => {
    const wrapper = mountMasthead({
      highlights: [storyHighlight()],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    const button = wrapper.find('[data-testid="highlight-locate-0"]')
    expect(button.exists()).toBe(true)
    expect(button.element.tagName).toBe('BUTTON')
    expect(button.attributes('aria-label')).toContain('要闻')
    await button.trigger('click')
    expect(wrapper.emitted('locate')).toEqual([[55]])
  })

  it('划选文案不触发定位（selection guard：非折叠选区点击不 emit）', async () => {
    const wrapper = mountMasthead({
      highlights: [storyHighlight()],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    const getSelection = vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false } as Selection)
    await wrapper.find('[data-testid="highlight-locate-0"]').trigger('click')
    expect(wrapper.emitted('locate')).toBeUndefined()
    getSelection.mockRestore()
  })

  it('story_id 有但正文无对应线程 → 不渲染按钮，显示「本期正文暂无对应事件」（不跳假位置）', () => {
    const wrapper = mountMasthead({
      highlights: [storyHighlight({ story_id: 77 })],
      sections: [makeSection(366, [{ id: 10, story_id: 55 }])],
    })
    expect(wrapper.find('[data-testid="highlight-locate-0"]').exists()).toBe(false)
    expect(wrapper.find('h3 button').exists()).toBe(false)
    const meta = wrapper.find('[data-testid="highlight-meta-0"]')
    expect(meta.exists()).toBe(true)
    expect(meta.text()).toContain('本期正文暂无对应事件')
  })

  it('旧数据兼容：无 story_id（标签回退模式）→ 三项都不显示，外观与现状一致', () => {
    const wrapper = mountMasthead({
      highlights: [{ title: '旧重点', reason: '理由', tag_ids: [] }],
      sections: [makeSection(366, [{ id: 10 }])],
    })
    const article = wrapper.find('.drm-highlight')
    expect(article.find('button').exists()).toBe(false)
    expect(wrapper.find('[data-testid="highlight-progress-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="highlight-meta-0"]').exists()).toBe(false)
    expect(article.text()).not.toContain('个来源')
    expect(article.text()).not.toContain('进展')
    expect(article.text()).not.toContain('暂无对应事件')
    expect(article.find('h3').text()).toBe('旧重点')
  })
})
