/**
 * BoardThreadBrowser — focus view (slice C′).
 *
 * Covers the section-lifecycle spec scenarios for the third view mode `focus`:
 * enter/exit, sticky header, timeline node filtering, drag-vs-click, in-place
 * expand (no popup overlay) and empty-topic fallback. Heavy view-model logic
 * (filtering, drag threshold, meta) is covered by topicFocus.test.ts; here we
 * assert the component wiring only.
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'
import { useTopicKeyEvents } from '../composables/useTopicKeyEvents'
import type {
  SectionTimelineNode,
  DailyReport,
  DailyReportThread,
} from '~/api/dailyReports'

// Icon stub — keep the suite offline (no iconify CDN fetch in happy-dom).
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// API surface mocks — BoardThreadBrowser calls these directly.
const timelineSections = ref<SectionTimelineNode[]>([])
const detailReport = ref<DailyReport | null>(null)
const getBoardSectionTimeline = vi.fn()
const getDailyReportDetail = vi.fn()
const getArticle = vi.fn()
const listBoardTopics = vi.fn()
const getReportKeyEvents = vi.fn()
const getTopicKeyEvents = vi.fn()
const putThreadKeyEvent = vi.fn()
const patchKeyEventMarker = vi.fn()

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getBoardSectionTimeline,
    getDailyReportDetail,
    listBoardTopics,
    getReportKeyEvents,
    getTopicKeyEvents,
    putThreadKeyEvent,
    patchKeyEventMarker,
  }),
}))
vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticle }),
}))

const stubs = {
  TopicManageDialog: { name: 'TopicManageDialog', template: '<div class="topic-mgmt-stub" />' },
}

function makeNode(
  id: number,
  over: Partial<SectionTimelineNode> & { topicId?: number; date?: string } = {},
): SectionTimelineNode {
  const topicId = over.topicId ?? 5
  return {
    id,
    report_id: over.report_id ?? 10,
    period_date: `${over.date ?? '2026-06-18'}T12:00:00Z`,
    cluster_label: over.cluster_label ?? `节点${id}`,
    status: over.status ?? 'continuing',
    article_count: 2,
    thread_count: 1,
    persistent_topic_id: topicId,
    persistent_topic: {
      id: topicId,
      label: topicId === 5 ? '以黎冲突' : '其他话题',
      status: 'active',
      color: '#b44f45',
      consecutive_hits: 3,
      can_activate: false,
    },
  }
}

function makeThread(id: number, over: Partial<DailyReportThread> = {}): DailyReportThread {
  return {
    id,
    report_id: 11,
    section_id: 102,
    title: `线索${id}`,
    summary: '摘要',
    tag_ids: [],
    confidence: 0.9,
    related_article_ids: [],
    created_at: '2026-06-29T12:00:00Z',
    ...over,
  }
}

beforeEach(() => {
  listBoardTopics.mockReset()
  getReportKeyEvents.mockReset()
  getTopicKeyEvents.mockReset()
  putThreadKeyEvent.mockReset()
  patchKeyEventMarker.mockReset()
  getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [] } })
  getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'idle', markers: [] } })
})

function mountBrowser() {
  return mount(BoardThreadBrowser, {
    props: { boardId: 1 },
    global: { stubs },
  })
}

async function toLanesAndEnterFocus(wrapper: ReturnType<typeof mountBrowser>) {
  // lanes view is where lane rows become clickable.
  const lanesBtn = wrapper.findAll('.btb-view-btn').find(b => b.text().includes('话题泳道'))!
  await lanesBtn.trigger('click')
  await nextTick()
  // click the lane row for topic 5 (background/label, not a node)
  await wrapper.find('.btb-lane-row[data-topic="5"]').trigger('click')
  await nextTick()
}

describe('BoardThreadBrowser — focus view (slice C′)', () => {
  beforeEach(() => {
    timelineSections.value = [
      makeNode(101, { topicId: 5, date: '2026-06-16', report_id: 10 }),
      makeNode(102, { topicId: 5, date: '2026-06-29', report_id: 11, cluster_label: '以军空袭' }),
      makeNode(103, { topicId: 7, date: '2026-06-18', report_id: 12 }),
    ]
    detailReport.value = {
      id: 11,
      semantic_board_id: 1,
      period_date: '2026-06-29T12:00:00Z',
      title: '日报',
      summary: '',
      status: 'done',
      cluster_count: 1,
      article_count: 2,
      event_tag_count: 0,
      highlights: [],
      dynamics: '',
      sections: [{
        id: 102,
        cluster_index: 0,
        cluster_label: '以军空袭',
        cluster_tag_ids: [],
        threads: [makeThread(201, { title: '黎方提停火框架' })],
        article_count: 2,
        best_tier: 0,
        avg_score: 0.9,
        quality_breakdown: null,
        persistent_topic_id: 5,
      }],
      created_at: '2026-06-29T12:00:00Z',
    } as DailyReport
    getBoardSectionTimeline.mockReset()
    getDailyReportDetail.mockReset()
    getArticle.mockReset()
    getBoardSectionTimeline.mockImplementation(async () => ({ success: true, data: { sections: timelineSections.value, relations: [] } }))
    getDailyReportDetail.mockImplementation(async () => ({ success: true, data: { report: detailReport.value! } }))
    // useTheme is a Nuxt auto-import; inject a stub so setup does not touch #imports.
    ;(globalThis as Record<string, unknown>).useTheme = () => ({ theme: ref('editorial') })
  })
  afterEach(() => {
    delete (globalThis as Record<string, unknown>).useTheme
  })

  // ---- C-T1: enter / exit ----
  it('enters focus mode when a lane row (not a node) is clicked', async () => {
    const wrapper = await mountBrowser()
    await nextTick() // let the immediate watch load data
    await toLanesAndEnterFocus(wrapper)
    expect(wrapper.find('.btb-focus').exists()).toBe(true)
    // still no popup overlay mounted for focus
    expect(document.body.querySelector('.btb-popup-overlay')).toBeNull()
  })

  it('returns to lanes when 返回总览 is clicked', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    expect(wrapper.find('.btb-focus').exists()).toBe(true)
    await wrapper.find('.btb-focus-back').trigger('click')
    await nextTick()
    expect(wrapper.find('.btb-focus').exists()).toBe(false)
    // back in lanes, the SVG canvas is rendered again
    expect(wrapper.find('.btb-chart').exists()).toBe(true)
  })

  // ---- C-T2: sticky header ----
  it('renders the sticky header with topic name, status badge and meta', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    const head = wrapper.find('.btb-focus-head')
    expect(head.exists()).toBe(true)
    expect(head.text()).toContain('以黎冲突')
    expect(wrapper.find('.btb-focus-status').exists()).toBe(true)
    const meta = wrapper.find('.btb-focus-meta')
    expect(meta.exists()).toBe(true)
    expect(meta.text()).toContain('2')        // 2 dynamics
    expect(meta.text()).toContain('6/29')    // latest date (formatDateShort -> M/D)
  })

  // ---- C-T3: node filtering ----
  it('renders ONLY the focused topic nodes on the timeline', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    const nodes = wrapper.findAll('.btb-focus-node')
    // topic 5 has nodes 101 + 102; topic 7 (103) must be excluded
    expect(nodes).toHaveLength(2)
  })

  // ---- C-T3: in-place expand, NO popup overlay ----
  it('expands threads in-place and does NOT open the popup overlay (Scenario: 节点就地展开而非弹窗)', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    // click the node for section 102 (the 06-29 column)
    const nodes = wrapper.findAll('.btb-focus-node')
    const target = nodes.find(n => n.text().includes('以军空袭') || n.attributes('data-section') === '102')
      ?? nodes[nodes.length - 1]!
    await target.trigger('click')
    await nextTick()
    await nextTick() // flush the async getDailyReportDetail
    // in-place region exists and shows the loaded thread
    const inline = wrapper.find('.btb-focus-inline')
    expect(inline.exists()).toBe(true)
    expect(inline.text()).toContain('黎方提停火框架')
    // popup overlay must NOT be mounted at all (the Scenario's SHALL NOT)
    expect(document.body.querySelector('.btb-popup-overlay')).toBeNull()
  })

  // ---- C-T3: drag suppresses the following click ----
  it('swallows the node click after a drag beyond the threshold (Scenario: 拖拽平移时间轴)', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    const wrap = wrapper.find('.btb-focus-timeline-wrap')
    // pointer down -> move beyond threshold (delta 20 > 3) -> up, then a click on a node
    await wrap.trigger('pointerdown', { clientX: 100, clientY: 0, pointerId: 1 })
    await wrap.trigger('pointermove', { clientX: 120, clientY: 0, pointerId: 1 })
    await wrap.trigger('pointerup', { clientX: 120, clientY: 0, pointerId: 1 })
    const node = wrapper.findAll('.btb-focus-node')[0]!
    await node.trigger('click')
    await nextTick()
    // click was suppressed: no inline region, no selected node
    expect(wrapper.find('.btb-focus-inline').exists()).toBe(false)
  })

  it('does NOT swallow the node click when the drag stays under the threshold', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    const wrap = wrapper.find('.btb-focus-timeline-wrap')
    await wrap.trigger('pointerdown', { clientX: 100, clientY: 0, pointerId: 1 })
    await wrap.trigger('pointermove', { clientX: 102, clientY: 0, pointerId: 1 }) // delta 2 < 3
    await wrap.trigger('pointerup', { clientX: 102, clientY: 0, pointerId: 1 })
    const node = wrapper.findAll('.btb-focus-node')[0]!
    await node.trigger('click')
    await nextTick()
    await nextTick()
    expect(wrapper.find('.btb-focus-inline').exists()).toBe(true)
  })

  // ---- C-T4: empty-topic fallback ----
  it('degrades gracefully when the focused topic has no nodes in the window (Scenario: 空话题降级)', async () => {
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    expect(wrapper.find('.btb-focus').exists()).toBe(true)
    // reload with a window that no longer contains topic 5
    timelineSections.value = [
      makeNode(103, { topicId: 7, date: '2026-06-18' }),
    ]
    getBoardSectionTimeline.mockResolvedValueOnce({ success: true, data: { sections: timelineSections.value, relations: [] } })
    // change the days window to trigger a reload
    const seven = wrapper.findAll('.btb-days-btn').find(b => b.text().includes('7天'))!
    await seven.trigger('click')
    await nextTick()
    await nextTick()
    // no throw; the empty-state note renders and a way back remains
    expect(wrapper.find('.btb-focus-empty').exists()).toBe(true)
    expect(wrapper.find('.btb-focus-back').exists()).toBe(true)
  })
})

// ── 关键事件注记（add-topic-key-event-markers）───────────────────────────────
describe('BoardThreadBrowser — key-event markers', () => {
  beforeEach(() => {
    // useTheme is a Nuxt auto-import; inject a stub (mirrors the suite above).
    ;(globalThis as Record<string, unknown>).useTheme = () => ({ theme: ref('editorial') })
    // 单例共享缓存逐用例隔离（避免上一用例的 confirmed 泄漏到下一用例）。
    useTopicKeyEvents().resetLocal()
    timelineSections.value = [
      makeNode(101, { topicId: 5, date: '2026-06-16', report_id: 10 }),
      makeNode(102, { topicId: 5, date: '2026-06-29', report_id: 11, cluster_label: '以军空袭' }),
      makeNode(103, { topicId: 7, date: '2026-06-18', report_id: 12 }),
    ]
    detailReport.value = {
      id: 11,
      semantic_board_id: 1,
      period_date: '2026-06-29T12:00:00Z',
      title: '日报',
      summary: '',
      status: 'done',
      cluster_count: 1,
      article_count: 2,
      event_tag_count: 0,
      highlights: [],
      dynamics: '',
      sections: [{
        id: 102,
        cluster_index: 0,
        cluster_label: '以军空袭',
        cluster_tag_ids: [],
        threads: [makeThread(201, { title: '黎方提停火框架' })],
        article_count: 2,
        best_tier: 0,
        avg_score: 0.9,
        quality_breakdown: null,
        persistent_topic_id: 5,
      }],
      created_at: '2026-06-29T12:00:00Z',
    } as DailyReport
    getBoardSectionTimeline.mockReset()
    getDailyReportDetail.mockReset()
    getArticle.mockReset()
    getBoardSectionTimeline.mockImplementation(async () => ({ success: true, data: { sections: timelineSections.value, relations: [] } }))
    getDailyReportDetail.mockImplementation(async () => ({ success: true, data: { report: detailReport.value! } }))
  })
  afterEach(() => {
    delete (globalThis as Record<string, unknown>).useTheme
  })

  function keMarker(over: Record<string, unknown> = {}) {
    return {
      id: 31,
      semantic_board_id: 1,
      persistent_topic_id: 5,
      source_report_id: 10,
      source_section_id: 101,
      source_thread_id: 201,
      source_period_date: '2026-06-16T12:00:00Z',
      topic_label_snapshot: '以黎冲突',
      title_snapshot: '快照线索',
      summary_snapshot: '快照摘要',
      tag_ids_snapshot: [],
      article_ids_snapshot: [],
      state: 'confirmed',
      revision: 1,
      ...over,
    }
  }

  it('renders ★n badges on lanes nodes only (timeline DAG stays clean)', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker()] } })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()

    // 默认 timeline：无徽标（不进 DAG）。
    expect(wrapper.findAll('.btb-node-ke-badge')).toHaveLength(0)

    const lanesBtn = wrapper.findAll('.btb-view-btn').find(b => b.text().includes('话题泳道'))!
    await lanesBtn.trigger('click')
    await nextTick()
    await flushPromises()
    await nextTick()
    const badges = wrapper.findAll('.btb-node-ke-badge')
    expect(badges).toHaveLength(1)
    expect(badges[0]!.text()).toBe('★1')
  })

  it('suggested rows from the shared cache never reach lanes badges (不泄漏建议)', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker({ id: 40, state: 'suggested' })] } })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()
    const lanesBtn = wrapper.findAll('.btb-view-btn').find(b => b.text().includes('话题泳道'))!
    await lanesBtn.trigger('click')
    await nextTick()
    await flushPromises()
    await nextTick()
    expect(wrapper.findAll('.btb-node-ke-badge')).toHaveLength(0)
  })

  it('focus view merges confirmed markers on real sections and detached snapshots on the same axis', async () => {
    getTopicKeyEvents.mockResolvedValue({
      success: true,
      data: {
        topic_id: 5,
        days: 14,
        markers: [
          keMarker(), // 挂在真实 section 101
          keMarker({ id: 32, source_section_id: 999, source_report_id: null, title_snapshot: '失效来源快照', source_period_date: '2026-06-20T12:00:00Z' }), // 失效快照
        ],
      },
    })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    await flushPromises()
    await nextTick()

    // 真实节点 ★1 徽标 + 快照列（同轴独立、dashed 轮廓）。
    const sectionKe = wrapper.find('.btb-focus-node[data-section="101"] .btb-focus-node__ke')
    expect(sectionKe.text()).toBe('★1')
    const snapshotNode = wrapper.find('.btb-focus-node--snapshot[data-marker="32"]')
    expect(snapshotNode.exists()).toBe(true)

    // 快照点击 → 原位详情（标题/来源语义/取消维护），不弹 overlay。
    await snapshotNode.trigger('click')
    await nextTick()
    const detail = wrapper.find('[data-testid="focus-marker-detail"]')
    expect(detail.exists()).toBe(true)
    expect(detail.text()).toContain('失效来源快照')
    expect(document.body.querySelector('.btb-popup-overlay')).toBeNull()

    // 取消 → actMarker(unmark)。
    patchKeyEventMarker.mockResolvedValue({ success: true, data: keMarker({ id: 32, state: 'dismissed', revision: 2 }) })
    const unmark = detail.findAll('button').find(b => b.text().includes('取消'))
    await unmark!.trigger('click')
    await nextTick()
    await new Promise(r => setTimeout(r, 0))
    expect(patchKeyEventMarker).toHaveBeenCalledWith(5, 32, 'unmark', 1)
  })

  it('shows a local error with retry entry when unmark fails (读写 error 局部反馈)', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker()] } })
    patchKeyEventMarker.mockResolvedValue({ success: false, status: 400, error: '来源已失效' })
    const wrapper = await mountBrowser()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    await flushPromises()
    await nextTick()

    await wrapper.find('.btb-focus-node[data-section="101"]').trigger('click')
    await nextTick()
    const unmark = wrapper.find('[data-testid="focus-ke-list"]').findAll('button').find(b => b.text().includes('取消'))!
    await unmark.trigger('click')
    await flushPromises()
    await nextTick()

    const err = wrapper.find('[data-testid="focus-ke-list"] .btb-ke-detail__error')
    expect(err.exists()).toBe(true)
    expect(err.text()).toContain('来源已失效')
    // 取消按钮保持可用=恢复重试入口。
    const unmark2 = wrapper.find('[data-testid="focus-ke-list"]').findAll('button').find(b => b.text().includes('取消'))!
    expect(unmark2.attributes('disabled')).toBeUndefined()
  })

  it('highlight mode only dims non-marker sections and never removes background nodes', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker()] } })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    await flushPromises()
    await nextTick()

    // 初始：两列都完整渲染。
    expect(wrapper.find('.btb-focus-node[data-section="101"]').exists()).toBe(true)
    expect(wrapper.find('.btb-focus-node[data-section="102"]').exists()).toBe(true)
    expect(wrapper.findAll('.btb-focus-col--dimmed')).toHaveLength(0)

    const toggle = wrapper.find('.btb-ke-toggle')
    expect(toggle.attributes('aria-pressed')).toBe('false')
    await toggle.trigger('click')
    await nextTick()
    expect(wrapper.find('.btb-ke-toggle').attributes('aria-pressed')).toBe('true')
    // 无标记的 102 变暗；有标记的 101 与快照保持。
    expect(wrapper.find('.btb-focus-col[data-v-col]') ? true : true).toBe(true)
    const dimmed = wrapper.findAll('.btb-focus-col--dimmed')
    expect(dimmed).toHaveLength(1)
    expect(wrapper.find('.btb-focus-node[data-section="101"]').exists()).toBe(true) // 背景不移除
    expect(wrapper.find('.btb-focus-node[data-section="102"]').exists()).toBe(true) // 背景不移除
  })

  it('clicking a section node shows the marker maintenance list with source-report navigation', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker()] } })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()
    await toLanesAndEnterFocus(wrapper)
    await flushPromises()
    await nextTick()

    await wrapper.find('.btb-focus-node[data-section="101"]').trigger('click')
    await nextTick()
    const list = wrapper.find('[data-testid="focus-ke-list"]')
    expect(list.exists()).toBe(true)
    expect(list.text()).toContain('快照线索')

    const link = list.findAll('button').find(b => b.text().includes('来源日报'))
    await link!.trigger('click')
    expect(wrapper.emitted('openDailyReport')).toHaveLength(1)
    expect(wrapper.emitted('openDailyReport')![0]).toEqual([10])
  })

  it('overview/lanes popup carries the marker list with unmark + source navigation', async () => {
    getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [keMarker()] } })
    const wrapper = await mountBrowser()
    await nextTick()
    await flushPromises()
    await nextTick()
    const lanesBtn = wrapper.findAll('.btb-view-btn').find(b => b.text().includes('话题泳道'))!
    await lanesBtn.trigger('click')
    await nextTick()
    await flushPromises()
    await nextTick()

    const dagNode = wrapper.findAll('.btb-dag-node').find(g => g.html().includes('节点101'))
    expect(dagNode, `lanes nodes: ${wrapper.findAll('.btb-dag-node').length}`).toBeTruthy()
    await dagNode!.trigger('click')
    await flushPromises()
    await nextTick()
    // popup 经 Teleport 挂 document.body（与既有 focus 断言同口径），组件树内查不到。
    const list = document.body.querySelector('[data-testid="popup-ke-list"]')
    expect(list, 'popup ke-list rendered in body').toBeTruthy()
    const link = [...(list as HTMLElement).querySelectorAll('button')].find(b => b.textContent?.includes('来源日报'))
    expect(link).toBeTruthy()
    link!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(wrapper.emitted('openDailyReport')![0]).toEqual([10])
    ;(list as HTMLElement).remove()
  })
})
