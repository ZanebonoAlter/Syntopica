import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DailyReportTopicSection from './DailyReportTopicSection.vue'
import type {
  DailyReport,
  DailyReportSection,
  DailyReportThread,
} from '~/api/dailyReports'
import type { QualityZone, RequestCacheEntry } from './dailyReportMagazine'
import type { TopicLifelineData } from '~/features/tags/composables/useDailyReportReader'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import type { ContextRow } from '~/api/boardEnrichment'

// Icon stub — keeps the suite offline (no iconify CDN fetch in happy-dom).
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// Child components are stubbed so the suite stays focused on THIS component's
// thread-soft-degrade rendering (their internals are covered by their own specs).
const stubs = {
  DailyReportMiniLifeline: {
    name: 'DailyReportMiniLifeline',
    template: '<div class="lifeline-stub"><slot name="details" /></div>',
  },
  SectionTierBadge: { name: 'SectionTierBadge', template: '<span class="tier-stub" />' },
  SectionAnchorBadge: { name: 'SectionAnchorBadge', template: '<span class="anchor-stub" />' },
  SectionQualityExplore: { name: 'SectionQualityExplore', template: '<span class="explore-stub" />' },
  // LaneTrendOverview 故意不 stub：本机环境 VTU stubs 不生效（域外预存），
  // 统一按真实子组件断言（findComponent + 真实 testid），对环境修复前后行为一致。
}

function makeThread(id: number, over: Partial<DailyReportThread> = {}): DailyReportThread {
  return {
    id,
    report_id: 1,
    section_id: 100,
    title: `线索${id}`,
    summary: '',
    tag_ids: [],
    confidence: 0.9,
    related_article_ids: [],
    created_at: '2026-06-26T12:00:00Z',
    ...over,
  }
}

function makeSection(id: number, threads: DailyReportThread[], over: Partial<DailyReportSection> = {}): DailyReportSection {
  return {
    id,
    cluster_index: 0,
    cluster_label: `板块${id}`,
    cluster_tag_ids: [],
    threads,
    article_count: threads.reduce((n, t) => n + (t.related_article_ids?.length ?? 0), 0),
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
    ...over,
  }
}

function activeZone(sections: DailyReportSection[]): QualityZone {
  return { key: 'active', label: '关心的话题', eyebrow: 'Following', sections }
}

function briefsZone(sections: DailyReportSection[]): QualityZone {
  return { key: 'briefs', label: '其他动态', eyebrow: 'Briefs', sections }
}

function mountSection(over: {
  zone?: QualityZone
  reportDate?: string
  lifelineEntries?: Map<number, RequestCacheEntry<TopicLifelineData>>
  articleEntries?: Map<number, { status: 'success'; data: { title: string } }>
  reportDetails?: Map<number, DailyReport>
  laneDynamicsEntry?: RequestCacheEntry<LaneDynamicsResponse>
  contextEntries?: Map<string, RequestCacheEntry<ContextRow[]>>
  onEnsureLaneDynamics?: (retry?: boolean) => void
  onEnsureContext?: (topicId: number, granularity: 'month' | 'year', retry?: boolean) => void
} = {}) {
  return mount(DailyReportTopicSection, {
    props: {
      zone: over.zone ?? activeZone([makeSection(100, [makeThread(1)])]),
      reportDate: over.reportDate ?? '2026-06-26T12:00:00Z',
      lifelineEntries: over.lifelineEntries ?? new Map(),
      articleEntries: over.articleEntries ?? new Map(),
      reportDetails: over.reportDetails ?? new Map(),
      laneDynamicsEntry: over.laneDynamicsEntry ?? { status: 'idle' },
      contextEntries: over.contextEntries ?? new Map(),
      ...(over.onEnsureLaneDynamics ? { onEnsureLaneDynamics: over.onEnsureLaneDynamics } : {}),
      ...(over.onEnsureContext ? { onEnsureContext: over.onEnsureContext } : {}),
    },
    global: { stubs },
  })
}

describe('DailyReportTopicSection — watch section anchors', () => {
  it('adds a stable report-section anchor with scroll margin to each section article', () => {
    const wrapper = mountSection({ zone: activeZone([makeSection(100, [makeThread(1)])]) })
    const section = wrapper.find('#report-section-100')
    expect(section.exists()).toBe(true)
    expect(section.classes()).toContain('drm-section-card')
    expect(section.attributes('id')).toBe('report-section-100')
  })

  it('expands and locates a requested section even when its topic starts collapsed', async () => {
    const wrapper = mountSection({
      zone: briefsZone([makeSection(100, [makeThread(1)])]),
    })
    expect(wrapper.find('#report-section-100').exists()).toBe(false)

    await wrapper.setProps({ focusSectionId: 100 })
    await nextTick()
    expect(wrapper.find('#report-section-100').exists()).toBe(true)
  })
})

describe('DailyReportTopicSection — thread-fit soft-degrade (current loop)', () => {
  it('demotes a thread whose fit_distance exceeds the threshold (adds --demoted class)', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题线索', fit_distance: 0.4 }),
      ])]),
    })
    const thread = wrapper.find('.drm-section-card .drm-thread')
    expect(thread.classes()).toContain('drm-thread--demoted')
  })

  it('does NOT demote a thread that fits its section', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '贴合线索', fit_distance: 0.1 }),
      ])]),
    })
    const thread = wrapper.find('.drm-section-card .drm-thread')
    expect(thread.classes()).not.toContain('drm-thread--demoted')
  })

  it('treats the threshold boundary itself as non-demoted (0.28)', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '边界线索', fit_distance: 0.28 }),
      ])]),
    })
    const thread = wrapper.find('.drm-section-card .drm-thread')
    expect(thread.classes()).not.toContain('drm-thread--demoted')
  })

  it('does NOT demote a thread with a missing fit_distance (historical-style)', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '无信号线索' /* fit_distance undefined */ }),
      ])]),
    })
    const thread = wrapper.find('.drm-section-card .drm-thread')
    expect(thread.classes()).not.toContain('drm-thread--demoted')
  })

  it('keeps a demoted thread collapsed by default (no expanded articles)', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题线索', fit_distance: 0.4, related_article_ids: [101] }),
      ])]),
    })
    expect(wrapper.find('.drm-articles').exists()).toBe(false)
  })

  it('renders the section-bottom hint row counting demoted threads', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题A', fit_distance: 0.31 }),
        makeThread(2, { title: '跑题B', fit_distance: 0.45 }),
        makeThread(3, { title: '贴合', fit_distance: 0.05 }),
      ])]),
    })
    const hint = wrapper.find('.drm-thread__hint')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toContain('2')
    expect(hint.text()).toContain('可能跑题')
  })

  it('hides the hint row when no thread is demoted', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '贴合', fit_distance: 0.05 }),
      ])]),
    })
    expect(wrapper.find('.drm-thread__hint').exists()).toBe(false)
  })

  it('tolerates a section payload missing the threads field (degraded backend data)', () => {
    // Regression: sections saved without thread rows used to arrive with the
    // threads field omitted entirely; demotedCount must not throw.
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [], { threads: undefined as unknown as DailyReportThread[] })]),
    })
    expect(wrapper.find('.drm-thread__hint').exists()).toBe(false)
    expect(wrapper.text()).toContain('板块100')
  })

  it('renders the hint row as a non-interactive status note (clicking it does not expand threads)', async () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题A', fit_distance: 0.31, related_article_ids: [101] }),
        makeThread(2, { title: '跑题B', fit_distance: 0.45, related_article_ids: [102] }),
      ])]),
    })
    const hint = wrapper.find('.drm-thread__hint')
    // status note is a plain <p>, not an actionable <button>
    expect(hint.element.tagName).toBe('P')
    expect(hint.text()).toContain('可能跑题的线索')
    // clicking it must NOT expand any thread (no batch toggle anymore)
    await hint.trigger('click')
    expect(wrapper.findAll('.drm-articles')).toHaveLength(0)
  })

  it('flags a demoted thread with a left-aligned icon before its title', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题线索', fit_distance: 0.4 }),
      ])]),
    })
    const title = wrapper.find('.drm-thread__title')
    expect(title.exists()).toBe(true)
    // icon sits before the <strong> title in DOM order (left-aligned, not buried in the right meta)
    const iconNode = title.find('.drm-thread__flag').element
    const strongNode = title.find('strong').element
    const children = Array.from(title.element.childNodes)
    expect(children.indexOf(iconNode)).toBeLessThan(children.indexOf(strongNode))
    // icon carries the status label for assistive tech
    expect(wrapper.find('.drm-thread__flag').attributes('aria-label')).toBe('可能跑题的线索')
  })

  it('shows the fit-distance probe (number + label) only after a thread is expanded', async () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '跑题线索', fit_distance: 0.4, related_article_ids: [101] }),
      ])]),
      articleEntries: new Map([[101, { status: 'success', data: { title: '某文章' } }]]),
    })
    // collapsed: no probe, no number leaks into the header
    expect(wrapper.find('.drm-thread__fit-probe').exists()).toBe(false)
    const header = wrapper.find('.drm-thread__header')
    expect(header.text()).not.toContain('0.40')

    // expand
    await wrapper.find('.drm-thread__header').trigger('click')
    const probe = wrapper.find('.drm-thread__fit-probe')
    expect(probe.exists()).toBe(true)
    expect(probe.text()).toContain('0.40')
    expect(probe.text()).toContain('可能跑题')
  })

  it('labels a fitting thread in the probe without demoting it', async () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [
        makeThread(1, { title: '贴合线索', fit_distance: 0.12, related_article_ids: [101] }),
      ])]),
      articleEntries: new Map([[101, { status: 'success', data: { title: '某文章' } }]]),
    })
    expect(wrapper.find('.drm-thread').classes()).not.toContain('drm-thread--demoted')
    await wrapper.find('.drm-thread__header').trigger('click')
    const probe = wrapper.find('.drm-thread__fit-probe')
    expect(probe.text()).toContain('0.12')
    expect(probe.text()).toContain('贴合')
  })
})

describe('DailyReportTopicSection — candidate badge removal (two-zone model)', () => {
  it('does not render "突发" or candidate badge in briefs zone', () => {
    const wrapper = mountSection({
      zone: briefsZone([makeSection(100, [makeThread(1)], {
        persistent_topic: {
          id: 5, label: '候选话题', status: 'candidate',
          color: '#b44f45', consecutive_hits: 3, can_activate: false,
        },
      })]),
    })
    expect(wrapper.text()).not.toContain('突发')
    expect(wrapper.text()).not.toContain('Developing')
    expect(wrapper.text()).not.toContain('观察中')
    // 正向硬约束：briefs zone 的状态文案必须是"其他动态"，绝非旧 candidate 文案
    expect(wrapper.find('.drm-topic__status').text()).toBe('其他动态')
  })

  it('renders active zone status label as "关心 · 持续追踪" and not old candidate text', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [makeThread(1)], {
        persistent_topic: {
          id: 5, label: '测试话题', status: 'active',
          color: '#b44f45', consecutive_hits: 3, can_activate: false,
        },
      })]),
    })
    expect(wrapper.find('.drm-topic__status').text()).toBe('关心 · 持续追踪')
    expect(wrapper.text()).not.toContain('candidate')
    expect(wrapper.text()).not.toContain('观察中')
  })

  it('shows candidate topic label in briefs zone (still readable)', () => {
    const wrapper = mountSection({
      zone: briefsZone([makeSection(100, [makeThread(1)], {
        persistent_topic: {
          id: 5, label: '候选话题', status: 'candidate',
          color: '#b44f45', consecutive_hits: 3, can_activate: false,
        },
      })]),
    })
    // 双轨命名：大标题用当天 cluster_label，规范名作锚点小字。
    expect(wrapper.text()).toContain('板块100')
    expect(wrapper.text()).toContain('候选话题')
    expect(wrapper.text()).toContain('其他动态')
  })

  it('does not auto-expand topics in briefs zone', async () => {
    const wrapper = mountSection({
      zone: briefsZone([makeSection(100, [makeThread(1)], {
        persistent_topic: {
          id: 5, label: '候选话题', status: 'candidate',
          color: '#b44f45', consecutive_hits: 3, can_activate: false,
        },
      })]),
    })
    // briefs zone should not auto-expand topics (no lifeline stub visible)
    expect(wrapper.find('.lifeline-stub').exists()).toBe(false)
  })
})

describe('DailyReportTopicSection — thread-fit soft-degrade (history loop)', () => {
  function historyFixtures(threads: DailyReportThread[]) {
    const reportId = 60
    const sectionId = 366
    const dayKey = '2026-06-20'
    const report: DailyReport = {
      id: reportId,
      semantic_board_id: 1974,
      period_date: `${dayKey}T12:00:00Z`,
      title: '历史日报',
      summary: '',
      status: 'done',
      cluster_count: 1,
      article_count: threads.reduce((n, t) => n + (t.related_article_ids?.length ?? 0), 0),
      event_tag_count: 0,
      highlights: [],
      dynamics: '',
      sections: [makeSection(sectionId, threads, { report_id: undefined } as never)],
      created_at: `${dayKey}T12:00:00Z`,
    }
    const lifelineEntries = new Map<number, RequestCacheEntry<TopicLifelineData>>([
      [5, {
        status: 'success',
        data: {
          sections: [{ id: sectionId, report_id: reportId, period_date: `${dayKey}T12:00:00Z` }],
          relations: [],
        } as never,
      }],
    ])
    const reportDetails = new Map<number, DailyReport>([[reportId, report]])
    return { reportDetails, lifelineEntries, dayKey }
  }

  async function mountHistory(threads: DailyReportThread[]) {
    const { reportDetails, lifelineEntries, dayKey } = historyFixtures(threads)
    const wrapper = mountSection({ reportDetails, lifelineEntries })
    // drive the mini-lifeline stub to select the day → reveals the history section
    const lifeline = wrapper.findComponent({ name: 'DailyReportMiniLifeline' })
    lifeline.vm.$emit('selectDay', dayKey)
    await nextTick()
    return wrapper
  }

  it('demotes a signalled off-topic history thread, keeps a signal-less one normal', async () => {
    const wrapper = await mountHistory([
      makeThread(10, { title: '历史跑题', fit_distance: 0.5 }),
      makeThread(11, { title: '历史无信号' /* fit_distance undefined */ }),
    ])
    const threads = wrapper.findAll('.drm-history__section .drm-thread')
    expect(threads).toHaveLength(2)
    const demoted = threads.find(t => t.text().includes('历史跑题'))!
    const signalless = threads.find(t => t.text().includes('历史无信号'))!
    expect(demoted.classes()).toContain('drm-thread--demoted')
    expect(signalless.classes()).not.toContain('drm-thread--demoted')
  })

  it('renders the history hint row counting demoted threads', async () => {
    const wrapper = await mountHistory([
      makeThread(10, { title: '历史跑题A', fit_distance: 0.33 }),
      makeThread(11, { title: '历史跑题B', fit_distance: 0.6 }),
    ])
    const hint = wrapper.find('.drm-history__section .drm-thread__hint')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toContain('2')
  })
})

describe('DailyReportTopicSection — 泳道趋势区挂载（lane-trend-overview）', () => {
  it('FD-16: active 话题泳道展开时，趋势区渲染于当日明细与节点图之前，props 传递正确', () => {
    const wrapper = mountSection()
    const body = wrapper.find('.drm-topic__body')
    expect(body.exists()).toBe(true)

    // 真实子组件断言（VTU stubs 本机不生效，findComponent 对真实/改名组件均可靠）
    const trend = wrapper.find('[data-testid="lane-trend-overview"]')
    expect(trend.exists()).toBe(true)

    // DOM 顺序：趋势区第 1 → 当日 section 明细第 2 → 节点图（MiniLifeline）第 3
    const nodes = Array.from(body.element.children)
    expect(nodes[0]).toBe(trend.element)
    const sectionsIndex = nodes.findIndex(node => node.classList.contains('drm-topic__sections'))
    const lifelineElement = wrapper.findComponent({ name: 'DailyReportMiniLifeline' }).element
    const lifelineIndex = nodes.indexOf(lifelineElement)
    expect(sectionsIndex).toBeGreaterThan(0)
    expect(lifelineIndex).toBeGreaterThan(sectionsIndex)

    const trendComponent = wrapper.findComponent({ name: 'LaneTrendOverview' })
    expect(trendComponent.props('topicId')).toBe(5)
    expect(trendComponent.props('topicColor')).toBe('#b44f45')
    // laneDynamicsEntry idle → 泳道未命中聚合数据
    expect(trendComponent.props('lane')).toBeNull()
  })

  it('FD-16: 泳道展开信号上抛 ensureLaneDynamics，切档信号带 topicId 上抛', async () => {
    const onEnsureLaneDynamics = vi.fn()
    const onEnsureContext = vi.fn()
    const wrapper = mountSection({ onEnsureLaneDynamics, onEnsureContext })

    // 首个泳道自动展开即发取数信号（无 retry 参数）
    expect(onEnsureLaneDynamics).toHaveBeenCalledTimes(1)

    // 趋势区切月档 → ensureContext(topicId, 'month') 上抛
    const trend = wrapper.findComponent({ name: 'LaneTrendOverview' })
    trend.vm.$emit('ensureContext', 'month')
    await nextTick()
    expect(onEnsureContext).toHaveBeenCalledWith(5, 'month', undefined)
  })

  it('FD-13: briefs zone 泳道展开也不渲染趋势区，既有内容完整', async () => {
    const wrapper = mountSection({ zone: briefsZone([makeSection(100, [makeThread(1)])]) })
    expect(wrapper.find('[data-testid="lane-trend-overview"]').exists()).toBe(false)

    await wrapper.find('.drm-topic__header').trigger('click')
    expect(wrapper.find('.drm-topic__body').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-trend-overview"]').exists()).toBe(false)
    expect(wrapper.find('.drm-section-card').exists()).toBe(true)
  })

  it('FD-13: 无话题 id 的分组不渲染趋势区（也未挂节点图）', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [makeThread(1)], { persistent_topic: undefined })]),
    })
    // 首组自动展开，但无 topicId → 无趋势区
    expect(wrapper.find('.drm-topic__body').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-trend-overview"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'DailyReportMiniLifeline' }).exists()).toBe(false)
    // 当日明细照常
    expect(wrapper.find('.drm-section-card').exists()).toBe(true)
  })
})
