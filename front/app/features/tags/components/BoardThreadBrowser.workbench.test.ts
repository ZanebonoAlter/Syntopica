/**
 * BoardThreadBrowser — 话题总览工作台化（切片② tasks 2.1-2.5）.
 *
 * Covers section-lifecycle spec scenarios:
 *  - 工具条控件存在（时间范围 7/14/30/全部 + 视图分段 + 回刷/合并/新建）
 *  - 时间范围切换触发重载（含"全部" → days=0）
 *  - 泳道 hover 操作菜单出现（重命名/归档/删除），且不使用 window.*
 *  - TopicManageDialog 已解耦（不再渲染）
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'
import type { SectionTimelineNode } from '~/api/dailyReports'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const timelineSections = ref<SectionTimelineNode[]>([])
const getBoardSectionTimeline = vi.fn()
const getDailyReportDetail = vi.fn()
const getArticle = vi.fn()
const updateTopic = vi.fn()
const deleteTopic = vi.fn()
const mergeTopics = vi.fn()
const backfillPersistentTopics = vi.fn()
const listBoardTopics = vi.fn()
const getReportKeyEvents = vi.fn().mockResolvedValue({ success: true, data: { status: 'idle', markers: [] } })
const getTopicKeyEvents = vi.fn().mockResolvedValue({ success: true, data: { topic_id: 1, days: 14, markers: [] } })
const putThreadKeyEvent = vi.fn()
const patchKeyEventMarker = vi.fn()

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getBoardSectionTimeline,
    getDailyReportDetail,
    updateTopic,
    deleteTopic,
    mergeTopics,
    backfillPersistentTopics,
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
// 编排态 API（切片③）：工作台测试不验证编排态内部，静默认 stub 即可。
vi.mock('~/api/persistentTopics', () => ({
  usePersistentTopicsApi: () => ({
    getComposeCandidates: vi.fn().mockResolvedValue({ success: true, data: { sections: [], matchThreshold: 0.3 } }),
    createManualLane: vi.fn().mockResolvedValue({ success: true, data: { topic: { id: '1', label: 'x', status: 'active', source: 'manual' }, skipped: [] } }),
  }),
}))

// 统一组件库 stub（AppDialog/AppButton/AppInput）+ 编排态浮层 stub——保持离线、可控交互。
const stubs = {
  ComposeInlineToolbar: {
    name: 'ComposeInlineToolbar',
    props: ['laneName', 'canSave'],
    template: '<div class="compose-inline-toolbar-stub" data-testid="compose-toolbar">编排工具条 stub</div>',
  },
  ComposeSidebar: {
    name: 'ComposeSidebar',
    props: ['items', 'queryText'],
    template: '<div class="compose-sidebar-stub" data-testid="compose-sidebar">候选侧边栏 stub</div>',
  },
  AppDialog: {
    name: 'AppDialog',
    props: ['modelValue', 'title', 'width'],
    template: `<div v-if="modelValue" class="app-dialog-stub" :data-title="title">
      <div class="app-dialog-body"><slot /></div>
      <div class="app-dialog-footer"><slot name="footer" /></div>
    </div>`,
  },
  AppButton: {
    name: 'AppButton',
    props: ['variant', 'size', 'disabled', 'loading'],
    template: '<button class="app-button-stub" :disabled="disabled || loading"><slot /></button>',
  },
  AppInput: {
    name: 'AppInput',
    props: ['modelValue', 'type', 'placeholder'],
    emits: ['update:modelValue'],
    template: '<input class="app-input-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}

function makeNode(id: number, topicId: number, date: string, label = `话题${topicId}`, confidence?: string): SectionTimelineNode {
  return {
    id,
    report_id: 10,
    period_date: `${date}T12:00:00Z`,
    cluster_label: `节点${id}`,
    status: 'continuing',
    article_count: 2,
    thread_count: 1,
    persistent_topic_id: topicId,
    topic_match_confidence: confidence,
    persistent_topic: {
      id: topicId,
      label,
      status: 'active',
      color: '#b44f45',
      consecutive_hits: 3,
      can_activate: false,
    },
  }
}

async function mountBrowser() {
  const wrapper = mount(BoardThreadBrowser, { props: { boardId: 1 }, global: { stubs } })
  await nextTick() // flush immediate watch → loadData
  return wrapper
}

async function switchToLanes(wrapper: VueWrapper) {
  const lanesBtn = wrapper.findAll('.btb-view-btn').find(b => b.text().includes('话题泳道'))!
  await lanesBtn.trigger('click')
  await nextTick()
}

describe('BoardThreadBrowser — 话题总览工作台化（切片②）', () => {
  beforeEach(() => {
    timelineSections.value = [
      makeNode(101, 5, '2026-06-16', '以黎冲突'),
      makeNode(102, 5, '2026-06-29', '以黎冲突'),
      makeNode(103, 7, '2026-06-18', '中东局势'),
    ]
    getBoardSectionTimeline.mockReset()
    getDailyReportDetail.mockReset()
    getArticle.mockReset()
    updateTopic.mockReset()
    deleteTopic.mockReset()
    mergeTopics.mockReset()
    backfillPersistentTopics.mockReset()
    listBoardTopics.mockReset()
    getBoardSectionTimeline.mockImplementation(async () => ({ success: true, data: { sections: timelineSections.value, relations: [] } }))
    updateTopic.mockResolvedValue({ success: true, data: {} })
    listBoardTopics.mockResolvedValue({ success: true, data: { topics: [] } })
    ;(globalThis as Record<string, unknown>).useTheme = () => ({ theme: ref('editorial') })
  })
  afterEach(() => {
    delete (globalThis as Record<string, unknown>).useTheme
  })

  // ---- 2.2: 工具条控件存在 ----
  it('renders the workbench toolbar controls (回刷/合并/新建 + time range + view seg)', async () => {
    const wrapper = await mountBrowser()
    const text = wrapper.text()
    expect(text).toContain('回刷归属')
    expect(text).toContain('合并预览')
    expect(text).toContain('新建泳道')
    // 时间范围四档
    for (const label of ['7天', '14天', '30天', '全部']) expect(text).toContain(label)
    // 视图模式分段
    expect(text).toContain('时间线')
    expect(text).toContain('话题泳道')
  })

  // ---- 2.2: 时间范围切换触发重载 ----
  it('reloads with the selected day window when the time range changes', async () => {
    const wrapper = await mountBrowser()
    getBoardSectionTimeline.mockClear()
    const thirty = wrapper.findAll('.btb-days-btn').find(b => b.text().includes('30天'))!
    await thirty.trigger('click')
    await nextTick()
    expect(getBoardSectionTimeline).toHaveBeenCalledWith(1, 30)
  })

  it('reloads with days=0 for 全部 (all history)', async () => {
    const wrapper = await mountBrowser()
    getBoardSectionTimeline.mockClear()
    const all = wrapper.findAll('.btb-days-btn').find(b => b.text().includes('全部'))!
    await all.trigger('click')
    await nextTick()
    expect(getBoardSectionTimeline).toHaveBeenCalledWith(1, 0)
  })

  // ---- 2.3: 泳道 hover 操作菜单出现 ----
  it('renders rename / archive / delete ops on enterable lane rows', async () => {
    const wrapper = await mountBrowser()
    await switchToLanes(wrapper)
    const ops = wrapper.findAll('.btb-lane-op')
    expect(ops.length).toBeGreaterThan(0)
    expect(wrapper.find('.btb-lane-op[aria-label="重命名"]').exists()).toBe(true)
    expect(wrapper.find('.btb-lane-op[aria-label="归档"]').exists()).toBe(true)
    expect(wrapper.find('.btb-lane-op[aria-label="删除"]').exists()).toBe(true)
  })

  // ---- 2.3 + 2.4: 重命名走 AppDialog（不弹原生 prompt），调 updateTopic ----
  it('opens an AppDialog on rename and calls updateTopic (no native prompt)', async () => {
    const promptSpy = vi.spyOn(window, 'prompt').mockImplementation(() => null)
    const wrapper = await mountBrowser()
    await switchToLanes(wrapper)
    await wrapper.find('.btb-lane-op[aria-label="重命名"]').trigger('click')
    await nextTick()
    // AppDialog 出现
    expect(wrapper.find('.app-dialog-stub').exists()).toBe(true)
    // 输入新名 → 保存
    await wrapper.find('.app-input-stub').setValue('以黎冲突升级')
    const save = wrapper.findAll('.app-button-stub').find(b => b.text().includes('保存'))!
    await save.trigger('click')
    await nextTick()
    await nextTick()
    expect(updateTopic).toHaveBeenCalledWith(5, { label: '以黎冲突升级' })
    expect(promptSpy).not.toHaveBeenCalled()
    promptSpy.mockRestore()
  })

  // ---- 2.3 + 2.4: 删除走 AppDialog（不弹原生 confirm）----
  it('opens an AppDialog on delete (no native confirm)', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockImplementation(() => false)
    const wrapper = await mountBrowser()
    await switchToLanes(wrapper)
    await wrapper.find('.btb-lane-op[aria-label="删除"]').trigger('click')
    await nextTick()
    expect(wrapper.find('.app-dialog-stub[data-title="删除话题"]').exists()).toBe(true)
    expect(confirmSpy).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  // ---- 2.4: TopicManageDialog 已解耦（不再渲染）----
  it('does NOT render TopicManageDialog anymore', async () => {
    const wrapper = await mountBrowser()
    // 既无 stub 也无真实组件节点
    expect(wrapper.findComponent({ name: 'TopicManageDialog' }).exists()).toBe(false)
    expect(wrapper.find('.topic-mgmt-stub').exists()).toBe(false)
  })

  // ---- 3.1: 新建泳道按钮进入编排态（composeMode 叠加，viewMode 保持 lanes）----
  it('enters compose mode (renders inline overlay, keeps lanes) when 新建泳道 is clicked', async () => {
    const wrapper = await mountBrowser()
    // 初始无编排态浮层
    expect(wrapper.find('.compose-inline-toolbar-stub').exists()).toBe(false)
    const btn = wrapper.findAll('button').find(b => b.text().includes('新建泳道'))!
    await btn.trigger('click')
    await nextTick()
    // 进入编排态：浮工具条渲染、总览工具条隐藏、lanes 视图保留（叠加而非全屏替换）
    expect(wrapper.find('.compose-inline-toolbar-stub').exists()).toBe(true)
    expect(wrapper.find('.compose-sidebar-stub').exists()).toBe(true)
    expect(wrapper.find('.btb-controls').exists()).toBe(false)
    expect(wrapper.find('.btb-chart').exists()).toBe(true)
  })

  // ---- 3.9: manual confidence 节点双环样式 ----
  it('renders a double-ring style for manual-confidence nodes in lanes', async () => {
    timelineSections.value = [
      makeNode(101, 5, '2026-06-16', '手动泳道', 'manual'),
      makeNode(102, 7, '2026-06-18', '算法话题'), // 无 confidence（算法三态）
    ]
    const wrapper = await mountBrowser()
    await switchToLanes(wrapper)
    const manualNode = wrapper.find('.btb-dag-node--manual')
    expect(manualNode.exists()).toBe(true)
    // 双环：ring + core 两个圆
    expect(manualNode.find('.btb-manual-ring').exists()).toBe(true)
    expect(manualNode.find('.btb-manual-core').exists()).toBe(true)
    // hover title 提示"人工归属"
    expect(manualNode.find('title').text()).toContain('人工归属')
    // 算法节点不套 manual 样式
    const allNodes = wrapper.findAll('.btb-dag-node')
    expect(allNodes.filter(n => n.classes().includes('btb-dag-node--manual')).length).toBe(1)
  })
})
