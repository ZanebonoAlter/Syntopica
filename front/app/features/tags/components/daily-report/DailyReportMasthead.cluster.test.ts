/**
 * DailyReportMasthead × 正文分区 —— 聚簇定位主链路测试（retire-story-adopt-event-metrics 3.3 / D14）。
 *
 * 覆盖 test-cases.md / delta spec 的三个 Scenario：
 *  - 点击第一条可见正文线程：`cluster_id` 只定位 DOM 顺序第一条 `tag_ids` 含该 id 的 thread，
 *    不是最小 thread id、不按事件线身份；
 *  - 失去目标与旧重点：目标消失时不渲染按钮并明示「本期正文暂无对应线索」；
 *    旧标签回退重点（无 cluster_id）保持静态标题、不显示任何快照；
 *  - 重复定位与减弱动画：重复点击同簇重新点亮（nonce 递增），高亮 2s 后清理，
 *    prefers-reduced-motion 下 scrollIntoView 用 `auto`（不平滑滚动）。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DailyReportMasthead from './DailyReportMasthead.vue'
import DailyReportTopicSection from './DailyReportTopicSection.vue'
import type { DailyReport, DailyReportHighlight, DailyReportSection, DailyReportThread } from '~/api/dailyReports'
import type { QualityZone } from './dailyReportMagazine'

const reducedMotion = { value: false }
vi.mock('~/composables/useMediaQuery', () => ({
  useMediaQuery: () => ({ get value() { return reducedMotion.value } }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const stubs = {
  DailyReportMiniLifeline: {
    name: 'DailyReportMiniLifeline',
    template: '<div class="lifeline-stub"><slot name="details" /></div>',
  },
  SectionTierBadge: { name: 'SectionTierBadge', template: '<span class="tier-stub" />' },
  SectionAnchorBadge: { name: 'SectionAnchorBadge', template: '<span class="anchor-stub" />' },
  SectionQualityExplore: { name: 'SectionQualityExplore', template: '<span class="explore-stub" />' },
  KeyEventMarkerActions: { name: 'KeyEventMarkerActions', template: '<span class="ke-stub" />' },
}

function makeThread(id: number, tagIds: number[]): DailyReportThread {
  return {
    id,
    report_id: 1303,
    section_id: 366,
    title: `线索${id}`,
    summary: '',
    tag_ids: tagIds,
    confidence: 0.9,
    related_article_ids: [],
    created_at: '2026-11-03T12:00:00Z',
  }
}

function makeSection(id: number, threads: DailyReportThread[]): DailyReportSection {
  return {
    id,
    cluster_index: 0,
    cluster_label: `分区${id}`,
    cluster_tag_ids: [],
    threads,
    article_count: threads.length,
    best_tier: 0,
    avg_score: 0.9,
    quality_breakdown: null,
    persistent_topic_id: 5,
    persistent_topic: {
      id: 5,
      label: '测试话题',
      status: 'active',
      color: '#b44f45',
      consecutive_hits: 3,
      can_activate: false,
    },
  } as DailyReportSection
}

function makeReport(over: Partial<DailyReport> = {}): DailyReport {
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
  } as DailyReport
}

function highlight(over: Partial<DailyReportHighlight> = {}): DailyReportHighlight {
  return {
    title: '要闻',
    reason: '理由',
    tag_ids: [55],
    cluster_id: 55,
    article_count: 4,
    cluster_mode: '向量聚簇',
    data_as_of: '2026-11-03',
    research_at: '2026-11-03T21:30:00+08:00',
    ...over,
  }
}

function briefsZone(sections: DailyReportSection[]): QualityZone {
  return { key: 'briefs', label: '其他动态', eyebrow: 'Briefs', sections }
}

function mountSection(zone: QualityZone) {
  return mount(DailyReportTopicSection, {
    props: {
      zone,
      reportDate: '2026-11-03T12:00:00Z',
      lifelineEntries: new Map(),
      articleEntries: new Map(),
      reportDetails: new Map(),
      laneDynamicsEntry: { status: 'idle' as const },
      contextEntries: new Map(),
    },
    global: { stubs },
  })
}

beforeEach(() => {
  reducedMotion.value = false
})

describe('DailyReportMasthead × 正文 — 聚簇定位主链路', () => {
  it('点击第一条可见正文线程：定位 DOM 顺序第一条 tag_ids 含 cluster_id 的线程（非最小 id）', async () => {
    const sections = [makeSection(366, [makeThread(30, [55, 7]), makeThread(20, [55])])]
    const report = makeReport({ sections, highlights: [highlight()] })
    const masthead = mount(DailyReportMasthead, { props: { report } })
    const button = masthead.find('[data-testid="highlight-locate-0"]')
    expect(button.exists()).toBe(true)
    await button.trigger('click')
    expect(masthead.emitted('locate')).toEqual([[55]])

    // 阅读容器把定位意图转给正文分区：DOM 序 30 → 20，点亮 30
    const scrollIntoView = vi.fn()
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView as never
    const section = mountSection(briefsZone(sections))
    document.body.appendChild(section.element)
    try {
      await section.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
      await nextTick()
      await nextTick()
      expect(section.find('#report-thread-30').classes()).toContain('drm-thread--lit')
      expect(section.find('#report-thread-20').classes()).not.toContain('drm-thread--lit')
      expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'center' })
    } finally {
      section.unmount()
      if (originalScroll) Element.prototype.scrollIntoView = originalScroll
      else delete (Element.prototype as never as Record<string, unknown>).scrollIntoView
    }
  })

  it('失去目标：cluster_id 有但正文无线索 → 不渲染按钮、明示「本期正文暂无对应线索」', () => {
    const masthead = mount(DailyReportMasthead, {
      props: { report: makeReport({ sections: [makeSection(366, [makeThread(10, [7])])], highlights: [highlight()] }) },
    })
    expect(masthead.find('[data-testid="highlight-locate-0"]').exists()).toBe(false)
    expect(masthead.find('[data-testid="highlight-meta-0"]').text()).toContain('本期正文暂无对应线索')
    // 依据快照仍如实展示（不因失去定位目标而丢口径）
    expect(masthead.find('[data-testid="highlight-meta-0"]').text()).toContain('4 篇来源')
    expect(masthead.find('[data-testid="highlight-meta-0"]').text()).toContain('聚簇口径 向量聚簇')
  })

  it('旧重点（无 cluster_id / 标签回退）：静态标题、无按钮、无依据行，不伪造快照', () => {
    const masthead = mount(DailyReportMasthead, {
      props: {
        report: makeReport({
          sections: [makeSection(366, [makeThread(10, [55])])],
          highlights: [{ title: '旧重点', reason: '理由', tag_ids: [] }],
        }),
      },
    })
    expect(masthead.find('[data-testid="highlight-locate-0"]').exists()).toBe(false)
    expect(masthead.find('[data-testid="highlight-meta-0"]').exists()).toBe(false)
    expect(masthead.find('.drm-highlight h3').text()).toBe('旧重点')
  })

  it('重复定位与减弱动画：同簇重复请求重新点亮（2s 清理），reduced-motion 不平滑滚动', async () => {
    vi.useFakeTimers()
    reducedMotion.value = true
    const scrollIntoView = vi.fn()
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView as never
    const section = mountSection(briefsZone([makeSection(366, [makeThread(30, [55])])]))
    document.body.appendChild(section.element)
    try {
      await section.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
      await nextTick()
      await nextTick()
      expect(section.find('#report-thread-30').classes()).toContain('drm-thread--lit')
      expect(scrollIntoView).toHaveBeenLastCalledWith({ behavior: 'auto', block: 'center' })

      vi.advanceTimersByTime(2000)
      await nextTick()
      expect(section.find('#report-thread-30').classes()).not.toContain('drm-thread--lit')

      // 重复点击同簇：nonce 递增 → 重新点亮
      await section.setProps({ locateThread: { clusterId: 55, nonce: 2 } })
      await nextTick()
      await nextTick()
      expect(section.find('#report-thread-30').classes()).toContain('drm-thread--lit')
      expect(scrollIntoView).toHaveBeenCalledTimes(2)
    } finally {
      section.unmount()
      if (originalScroll) Element.prototype.scrollIntoView = originalScroll
      else delete (Element.prototype as never as Record<string, unknown>).scrollIntoView
      vi.useRealTimers()
    }
  })
})

afterEach(() => {
  vi.useRealTimers()
  flushPromises()
})
