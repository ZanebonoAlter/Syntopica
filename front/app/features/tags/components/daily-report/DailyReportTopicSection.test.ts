import { readFileSync } from 'node:fs'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DailyReportTopicSection from './DailyReportTopicSection.vue'
import type {
  DailyReportSection,
  DailyReportThread,
  ThreadRelationsResponse,
} from '~/api/dailyReports'
import type { QualityZone, RequestCacheEntry } from './dailyReportMagazine'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { useConfirm } from '~/composables/useConfirm'
import type { ContextRow } from '~/api/boardEnrichment'

// 线索相关推荐走模块级单例缓存（useThreadRelatedEvents）→ 取数必须离线 mock，
// 否则 happy-dom 会去连真后端。默认每个报告都「没有候选」（chip 行不渲染）。
const relations = vi.hoisted(() => ({
  byReport: new Map<number, ThreadRelationsResponse>(),
  getThreadRelations: vi.fn(),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ getThreadRelations: relations.getThreadRelations }),
}))

// Icon stub — keeps the suite offline (no iconify CDN fetch in happy-dom).
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// Child components are stubbed so the suite stays focused on THIS component's
// thread-soft-degrade rendering (their internals are covered by their own specs).
const stubs = {
  SectionTierBadge: { name: 'SectionTierBadge', template: '<span class="tier-stub" />' },
  SectionAnchorBadge: { name: 'SectionAnchorBadge', template: '<span class="anchor-stub" />' },
  SectionQualityExplore: { name: 'SectionQualityExplore', template: '<span class="explore-stub" />' },
  // LaneTrendOverview 故意不 stub：本机环境 VTU stubs 不生效（域外预存），
  // 统一按真实子组件断言（findComponent + 真实 testid），对环境修复前后行为一致。
  KeyEventMarkerActions: {
    name: 'KeyEventMarkerActions',
    props: ['mode', 'threadId', 'topicId', 'view'],
    emits: ['mark', 'dismiss', 'unmark'],
    template: `<div class="ke-stub" :data-thread-id="threadId" :data-ke-mode="mode" :data-ke-state="view?.marker?.state ?? 'none'" @click.stop="$emit('mark')" />`,
  },
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
  reportId?: number
  reportDate?: string
  articleEntries?: Map<number, { status: 'success'; data: { title: string } }>
  laneDynamicsEntry?: RequestCacheEntry<LaneDynamicsResponse>
  contextEntries?: Map<string, RequestCacheEntry<ContextRow[]>>
  onEnsureLaneDynamics?: (retry?: boolean) => void
  onEnsureContext?: (topicId: number, granularity: 'month' | 'year', retry?: boolean) => void
  rebucketSectionId?: number | null
  sectionAliases?: Map<number, number>
} = {}) {
  return mount(DailyReportTopicSection, {
    props: {
      zone: over.zone ?? activeZone([makeSection(100, [makeThread(1)])]),
      reportId: over.reportId ?? 1,
      reportDate: over.reportDate ?? '2026-06-26T12:00:00Z',
      articleEntries: over.articleEntries ?? new Map(),
      laneDynamicsEntry: over.laneDynamicsEntry ?? { status: 'idle' },
      contextEntries: over.contextEntries ?? new Map(),
      ...(over.onEnsureLaneDynamics ? { onEnsureLaneDynamics: over.onEnsureLaneDynamics } : {}),
      ...(over.onEnsureContext ? { onEnsureContext: over.onEnsureContext } : {}),
      ...(over.rebucketSectionId !== undefined ? { rebucketSectionId: over.rebucketSectionId } : {}),
      ...(over.sectionAliases ? { sectionAliases: over.sectionAliases } : {}),
    },
    global: { stubs },
  })
}

// 每个用例默认「没有候选」：取数 mock 从这里拿空结果，测试内按报告 id 注册数据。
beforeEach(() => {
  relations.getThreadRelations.mockReset()
  relations.getThreadRelations.mockImplementation(async (reportId: number) => {
    const registered = relations.byReport.get(reportId)
    return {
      success: true,
      data: registered ?? {
        report_id: reportId,
        days: 7,
        window_start: '2026-06-19',
        window_end: '2026-06-25',
        status: 'empty',
        threads: {},
        total: 0,
        skipped_no_embedding: 0,
      },
    }
  })
})

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
    // briefs zone 不自动展开话题：正文体整体不渲染
    expect(wrapper.find('.drm-topic__body').exists()).toBe(false)
    expect(wrapper.find('.drm-section-card').exists()).toBe(false)
  })
})

describe('DailyReportTopicSection — 泳道趋势区挂载（lane-trend-overview）', () => {
  it('FD-16: active 话题泳道展开时，趋势区渲染于当日明细之前，props 传递正确', () => {
    const wrapper = mountSection()
    const body = wrapper.find('.drm-topic__body')
    expect(body.exists()).toBe(true)

    // 真实子组件断言（VTU stubs 本机不生效，findComponent 对真实/改名组件均可靠）
    const trend = wrapper.find('[data-testid="lane-trend-overview"]')
    expect(trend.exists()).toBe(true)

    // DOM 顺序：趋势区第 1 → 当日 section 明细第 2（折线/节点图已整体退役，正文是最后一块）
    const nodes = Array.from(body.element.children)
    expect(nodes[0]).toBe(trend.element)
    const sectionsIndex = nodes.findIndex(node => node.classList.contains('drm-topic__sections'))
    expect(sectionsIndex).toBeGreaterThan(0)
    expect(sectionsIndex).toBe(nodes.length - 1)

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

  it('FD-13: 无话题 id 的分组不渲染趋势区', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [makeThread(1)], { persistent_topic: undefined })]),
    })
    // 首组自动展开，但无 topicId → 无趋势区
    expect(wrapper.find('.drm-topic__body').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-trend-overview"]').exists()).toBe(false)
    // 当日明细照常
    expect(wrapper.find('.drm-section-card').exists()).toBe(true)
  })
})


// ── 关键事件标记（add-topic-key-event-markers）───────────────────────────────
// 集成红线：标记按钮是 thread header 的兄弟节点（零嵌套按钮）、点击标记不误
// 展开文章列表、A/B 条目独立（只变被选者）、锚点 ID 稳定不受标记影响。
describe('DailyReportTopicSection — key-event marker actions', () => {
  it('renders review-mode actions for current-date threads and history-mode for historical ones', () => {
    const wrapper = mountSection()
    const keStubs = wrapper.findAll('.ke-stub')
    expect(keStubs.length).toBeGreaterThan(0)
    expect(keStubs.some(el => el.attributes('data-ke-mode') === 'review')).toBe(true)
  })

  it('keying a marker does NOT toggle the thread article expansion (点击标记只发动作)', async () => {
    const wrapper = mountSection({ zone: activeZone([makeSection(100, [makeThread(1)])]) })
    expect(wrapper.find('.drm-thread').exists()).toBe(true)
    expect(wrapper.find('.drm-articles').exists()).toBe(false)

    // 标记动作经由 stub 冒泡为 mark 事件（.stop 防扩散）。
    const keStub = wrapper.find('.ke-stub')
    await keStub.trigger('click')
    expect(wrapper.find('.drm-articles').exists()).toBe(false) // 标记点击不得展开文章
    expect(wrapper.emitted('open-article')).toBeUndefined()
  })

  it('thread anchors stay stable regardless of marker state (t:thread.id 锚不变)', () => {
    const wrapper = mountSection({ zone: activeZone([makeSection(100, [makeThread(7, { summary: '摘要文本' })])]) })
    // 页边注锚容器精确断言：t:thread.id 与标记状态互不影响。
    expect(wrapper.find('[data-mn-anchor="t:7"]').exists()).toBe(true)
  })

  it('disabled-thread logic stays independent of marker presence (无文章不误禁标记)', () => {
    const wrapper = mountSection({
      zone: activeZone([makeSection(100, [makeThread(1, { related_article_ids: [] })])]),
    })
    const keStub = wrapper.find('.ke-stub')
    expect(keStub.exists()).toBe(true)
  })
})

describe('DailyReportTopicSection — 泳道内主题分桶（add-intra-lane-theme-bucketing）', () => {
  function bucketedSection(id: number, over: Partial<DailyReportSection> = {}): DailyReportSection {
    return makeSection(id, [makeThread(id * 10)], {
      cluster_index: 3,
      cluster_label: `章节-${id}`,
      bucket_title: '工具调用',
      bucket_index: 0,
      lane_tier: 'l1_direct',
      ...over,
    })
  }

  it('bucket_title 非空时渲染方向徽标', () => {
    const wrapper = mountSection({ zone: activeZone([bucketedSection(100)]) })
    const badge = wrapper.find('.drm-section-bucket')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('工具调用')
    expect(badge.attributes('title')).toBe('工具调用')
  })

  it('徽标宽度上限用 em（汉字宽）而非 ch——否则 ≤12 字的方向名会被整条截断', () => {
    // 12ch 只够 6 个汉字（ch = 数字宽 ≈ 汉字一半），2026-10-24 真机实测 6/6 徽标全被截断。
    const source = readFileSync(`${import.meta.dirname}/DailyReportTopicSection.vue`, 'utf8')
    const start = source.indexOf('.drm-section-bucket {')
    const block = source.slice(start, source.indexOf('.drm-section-card__rebucket {', start))
    expect(block).toContain('max-width: 14em')
    expect(block).not.toMatch(/max-width:\s*\d+ch/)
  })

  it('bucket_title 为空时不渲染徽标', () => {
    const wrapper = mountSection({ zone: activeZone([bucketedSection(100, { bucket_title: null })]) })
    expect(wrapper.find('.drm-section-bucket').exists()).toBe(false)
  })

  it('组内只有一条 section 但带桶名时仍渲染章节标题', () => {
    const wrapper = mountSection({ zone: activeZone([bucketedSection(100)]) })
    const title = wrapper.find('.drm-section-card__title')
    expect(title.exists()).toBe(true)
    expect(title.text()).toBe('章节-100')
  })

  it('单条未分桶 section 不渲染章节标题（历史行为不变）', () => {
    const wrapper = mountSection({
      zone: activeZone([bucketedSection(100, { bucket_title: null, bucket_index: 0 })]),
    })
    expect(wrapper.find('.drm-section-card__title').exists()).toBe(false)
  })

  it('算法桶显示重新分桶按钮，watch 物化组隐藏', () => {
    const algo = mountSection({ zone: activeZone([bucketedSection(100)]) })
    expect(algo.find('.drm-section-card__rebucket').exists()).toBe(true)

    for (const tier of ['watch_keyword', 'watch_sentence']) {
      const watch = mountSection({
        zone: briefsZone([bucketedSection(100, { lane_tier: tier, bucket_title: null })]),
      })
      expect(watch.find('.drm-section-card__rebucket').exists()).toBe(false)
    }
  })

  it('重分桶进行中按钮置灰且 aria-busy（重复提交保护）', () => {
    const wrapper = mountSection({ zone: activeZone([bucketedSection(100)]), rebucketSectionId: 100 })
    const button = wrapper.find('.drm-section-card__rebucket')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('aria-busy')).toBe('true')
  })

  it('点按钮先弹二次确认，确认后才 emit rebucket', async () => {
    const wrapper = mountSection({ zone: activeZone([bucketedSection(100)]) })
    await wrapper.find('.drm-section-card__rebucket').trigger('click')
    await nextTick()

    const { state, settle } = useConfirm()
    expect(state.value?.open).toBe(true)
    expect(state.value?.message).toContain('替换该天该泳道的主题分桶')
    expect(wrapper.emitted('rebucket')).toBeUndefined()

    settle(state.value!.id, false)
    await nextTick()
    expect(wrapper.emitted('rebucket')).toBeUndefined()

    await wrapper.find('.drm-section-card__rebucket').trigger('click')
    await nextTick()
    settle(useConfirm().state.value!.id, true)
    await flushPromises()
    expect(wrapper.emitted('rebucket')).toEqual([[100]])
  })

  it('焦点锚点失效时回退到别名指向的组头 section', async () => {
    const wrapper = mountSection({
      zone: activeZone([bucketedSection(201)]),
      sectionAliases: new Map([[100, 201]]),
    })
    // 组件用 document.getElementById 定位锚点 → 必须真正挂进 document。
    document.body.appendChild(wrapper.element)
    const scrollIntoView = vi.fn()
    const target = wrapper.find('#report-section-201').element as HTMLElement
    target.scrollIntoView = scrollIntoView

    await wrapper.setProps({ focusSectionId: 100 })
    await flushPromises()
    expect(scrollIntoView).toHaveBeenCalled()
    wrapper.unmount()
  })
})

// ── 线索相关推荐 chip 行（replace-lane-lifeline-with-topic-events 4.5/7.3）──
// 插入位置红线：整宽一行，紧跟 .drm-thread-row 之后、文章列表之前（不塞进两列 grid）。
describe('DailyReportTopicSection — 线索相关推荐 chip 行', () => {
  function relatedResponse(reportId: number, threadId: number): ThreadRelationsResponse {
    return {
      report_id: reportId,
      days: 7,
      window_start: '2026-06-19',
      window_end: '2026-06-25',
      status: 'ready',
      threads: {
        [String(threadId)]: [{
          thread_id: 9001,
          title: '早前铺垫过的同一条线',
          summary: '历史摘要',
          period_date: '2026-06-20',
          score: 0.91,
          level: 'strong',
          confirmed: true,
          source_article: { id: 555, title: '来源文章', feed: 'feed', published_at: null },
        }],
      },
      total: 1,
      skipped_no_embedding: 0,
    }
  }

  it('chip 行位于 .drm-thread-row 之后、文章列表之前，且全宽（不进两列 grid）', async () => {
    relations.byReport.set(70, relatedResponse(70, 1))
    const wrapper = mountSection({ reportId: 70, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()

    const thread = wrapper.find('.drm-thread')
    const row = thread.find('.drm-thread-row')
    const chipRow = thread.find('[data-testid="thread-related-1"]')
    expect(row.exists()).toBe(true)
    expect(chipRow.exists()).toBe(true)
    // 兄弟关系：row 的 nextElementSibling 就是 chip 行 → 说明它在两列 grid 之外
    expect(row.element.nextElementSibling).toBe(chipRow.element)
    expect(row.element.contains(chipRow.element)).toBe(false)
    expect(thread.element.contains(chipRow.element)).toBe(true)
  })

  it('无相关事件的线索不渲染 chip 行（不占位、不写空态文案）', async () => {
    const wrapper = mountSection({ reportId: 71, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('暂无相关')
    // 线索本体不受影响
    expect(wrapper.find('.drm-thread__header').text()).toContain('线索1')
  })

  it('展开话题卡片即按 (reportId, days) 请求一次（覆盖整份日报）', async () => {
    relations.getThreadRelations.mockClear()
    mountSection({ reportId: 72, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(1)
    expect(relations.getThreadRelations).toHaveBeenCalledWith(72, { days: 7 })
  })
})

// ── 分区级档位工具条（review R-3）：档位不在线索行里，否则无候选的线索行会留下空行 ──
describe('DailyReportTopicSection — 相关线索档位工具条', () => {
  function emptyEntry(reportId: number, days: number) {
    return {
      success: true as const,
      data: {
        report_id: reportId,
        days,
        window_start: '2026-06-19',
        window_end: '2026-06-25',
        status: 'empty' as const,
        threads: {},
        total: 0,
        skipped_no_embedding: 0,
      },
    }
  }

  it('分区标题行渲染 7/30 档位控件，位于话题卡片之外（每条分区一条，不是每条线索一条）', async () => {
    const wrapper = mountSection({
      reportId: 73,
      zone: activeZone([makeSection(100, [makeThread(1), makeThread(2)])]),
    })
    await flushPromises()

    const header = wrapper.find('.drm-zone__header')
    const seven = header.find('[data-testid="zone-related-days-7"]')
    const thirty = header.find('[data-testid="zone-related-days-30"]')
    expect(seven.exists()).toBe(true)
    expect(thirty.exists()).toBe(true)
    expect(seven.attributes('aria-pressed')).toBe('true')
    expect(thirty.attributes('aria-pressed')).toBe('false')
    // 不在话题卡片/线索行里，且整分区只有一个控件
    expect(wrapper.find('.drm-topic').element.contains(seven.element)).toBe(false)
    expect(wrapper.findAll('[data-testid="zone-related-days-7"]')).toHaveLength(1)
    // 线索行不再携带档位控件
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
  })

  it('切到 30 天按新档位重取；切回 7 天命中缓存不再请求', async () => {
    relations.getThreadRelations.mockImplementation(async (reportId: number, params?: { days?: number }) =>
      emptyEntry(reportId, params?.days ?? 7))
    const wrapper = mountSection({ reportId: 74, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-testid="zone-related-days-30"]').trigger('click')
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(2)
    expect(relations.getThreadRelations).toHaveBeenLastCalledWith(74, { days: 30 })
    expect(wrapper.find('[data-testid="zone-related-days-30"]').attributes('aria-pressed')).toBe('true')

    // 切回 7 天：该档已成功加载过 → 命中缓存（spec「7/30 档位切换与缓存」）
    await wrapper.find('[data-testid="zone-related-days-7"]').trigger('click')
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(2)
  })

  it('失败档再切会强制重发（工具条常驻，切档是失败后的唯一重试入口）', async () => {
    relations.getThreadRelations.mockImplementation(async (reportId: number, params?: { days?: number }) =>
      params?.days === 30 ? { success: false as const, error: 'boom' } : emptyEntry(reportId, 7))
    const wrapper = mountSection({ reportId: 75, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()

    await wrapper.find('[data-testid="zone-related-days-30"]').trigger('click')
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(2)

    await wrapper.find('[data-testid="zone-related-days-7"]').trigger('click')
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(2) // 7 天成功档命中缓存

    await wrapper.find('[data-testid="zone-related-days-30"]').trigger('click')
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(3) // 错误档重发
  })

  it('报告切换时清掉旧报告的推荐缓存（resetLocal 接到 reportId 变化）', async () => {
    relations.getThreadRelations.mockImplementation(async (reportId: number, params?: { days?: number }) =>
      emptyEntry(reportId, params?.days ?? 7))
    const wrapper = mountSection({ reportId: 76, zone: activeZone([makeSection(100, [makeThread(1)])]) })
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(1)

    // 同一组件实例换报告（reader 同时下发新 reportId + period_date）：缓存被清、重取新报告
    await wrapper.setProps({ reportId: 77, reportDate: '2026-06-27T12:00:00Z' })
    await flushPromises()
    expect(relations.getThreadRelations).toHaveBeenCalledTimes(2)
    expect(relations.getThreadRelations).toHaveBeenLastCalledWith(77, { days: 7 })
  })
})

// ── retire-story-adopt-event-metrics 3.3：聚簇定位锚点 / 展开定位 / 2s 单高亮 ──

describe('DailyReportTopicSection — 聚簇定位锚点与高亮（D14）', () => {
  it('当前正文分支：thread 容器带稳定锚点 report-thread-{id} 与程序聚焦 tabindex；事件线标识已退役', async () => {
    const wrapper = mountSection({ zone: activeZone([makeSection(100, [makeThread(1, { tag_ids: [55] })])]) })
    await nextTick() // active 首组挂载即自动展开
    const anchor = wrapper.find('#report-thread-1')
    expect(anchor.exists()).toBe(true)
    expect(anchor.classes()).toContain('drm-thread')
    expect(anchor.attributes('tabindex')).toBe('-1')
    // 事件线身份不再外透（D14）：thread 容器无 data-story-id
    expect(anchor.attributes('data-story-id')).toBeUndefined()
  })

  it('locateThread：折叠组先展开，滚到/聚焦/点亮 DOM 首条属于该聚簇的线程（非最小 thread id）；2s 边界清理', async () => {
    vi.useFakeTimers()
    const scrollIntoView = vi.fn()
    const focus = vi.spyOn(HTMLElement.prototype, 'focus')
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView as never
    let wrapper: ReturnType<typeof mountSection> | undefined
    try {
      // briefs 分区无自动展开；同簇两条线程 DOM 序 30 → 20，定位 30 而非最小 id 20
      const zone = briefsZone([makeSection(100, [makeThread(30, { tag_ids: [55] }), makeThread(20, { tag_ids: [55] })])])
      wrapper = mountSection({ zone })
      // 组件用 document.getElementById 定位锚点 → 必须真正挂进 document（同 focusSectionId 用例）。
      document.body.appendChild(wrapper.element)
      expect(wrapper.find('#report-thread-30').exists()).toBe(false)

      await wrapper.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
      await nextTick()
      await nextTick()
      expect(wrapper.find('#report-thread-30').exists()).toBe(true)
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(focus).toHaveBeenCalledWith(expect.objectContaining({ preventScroll: true }))
      expect(wrapper.find('#report-thread-30').classes()).toContain('drm-thread--lit')
      expect(wrapper.find('#report-thread-20').classes()).not.toContain('drm-thread--lit')

      vi.advanceTimersByTime(1999)
      expect(wrapper.find('#report-thread-30').classes()).toContain('drm-thread--lit')
      vi.advanceTimersByTime(1)
      await nextTick()
      expect(wrapper.find('#report-thread-30').classes()).not.toContain('drm-thread--lit')
    } finally {
      wrapper?.unmount()
      if (originalScroll) Element.prototype.scrollIntoView = originalScroll
      else delete (Element.prototype as never as Record<string, unknown>).scrollIntoView
      focus.mockRestore()
      vi.useRealTimers()
    }
  })

  it('重复定位：后一次替换高亮，始终至多一个点亮目标', async () => {
    const zone = briefsZone([makeSection(100, [makeThread(30, { tag_ids: [55] }), makeThread(31, { tag_ids: [66] })])])
    const wrapper = mountSection({ zone })
    document.body.appendChild(wrapper.element)
    await wrapper.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
    await nextTick()
    await nextTick()
    expect(wrapper.find('#report-thread-30').classes()).toContain('drm-thread--lit')

    await wrapper.setProps({ locateThread: { clusterId: 66, nonce: 2 } })
    await nextTick()
    await nextTick()
    expect(wrapper.find('#report-thread-31').classes()).toContain('drm-thread--lit')
    expect(wrapper.find('#report-thread-30').classes()).not.toContain('drm-thread--lit')
    wrapper.unmount()
  })

  it('切 report：清理高亮与计时器（无旧报回跳）', async () => {
    const zone = briefsZone([makeSection(100, [makeThread(30, { tag_ids: [55] })])])
    const wrapper = mountSection({ zone })
    document.body.appendChild(wrapper.element)
    await wrapper.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
    await nextTick()
    await nextTick()
    expect(wrapper.find('#report-thread-30').classes()).toContain('drm-thread--lit')

    await wrapper.setProps({ reportDate: '2026-06-27T12:00:00Z' })
    await nextTick()
    expect(wrapper.find('#report-thread-30').classes()).not.toContain('drm-thread--lit')
    wrapper.unmount()
  })

  it('本分区无对应簇（目标在其它 zone）：静默不动作，不展开不滚动', async () => {
    const scrollIntoView = vi.fn()
    const originalScroll = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView as never
    try {
      const zone = briefsZone([makeSection(100, [makeThread(30, { tag_ids: [55] })])])
      const wrapper = mountSection({ zone })
      await wrapper.setProps({ locateThread: { clusterId: 999, nonce: 1 } })
      await nextTick()
      await nextTick()
      expect(wrapper.find('#report-thread-30').exists()).toBe(false) // 仍折叠
      expect(scrollIntoView).not.toHaveBeenCalled()
      expect(wrapper.findAll('.drm-thread--lit')).toHaveLength(0)
    } finally {
      if (originalScroll) Element.prototype.scrollIntoView = originalScroll
      else delete (Element.prototype as never as Record<string, unknown>).scrollIntoView
    }
  })

  it('卸载清理计时器（不留迟到高亮回调）', async () => {
    vi.useFakeTimers()
    try {
      const zone = briefsZone([makeSection(100, [makeThread(30, { tag_ids: [55] })])])
      const wrapper = mountSection({ zone })
      await wrapper.setProps({ locateThread: { clusterId: 55, nonce: 1 } })
      await nextTick()
      await nextTick()
      wrapper.unmount()
      expect(() => vi.advanceTimersByTime(3000)).not.toThrow()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })
})
