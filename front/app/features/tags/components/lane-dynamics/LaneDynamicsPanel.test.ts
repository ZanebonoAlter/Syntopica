/**
 * LaneDynamicsPanel 组件测试（overview-lane-dynamics tasks 3.2/3.4）。
 *
 * 覆盖 spec「泳道动态视图入口」状态矩阵 + 空态处理 + 候选栏：
 *  - loading → success：骨架（shimmer 占位）出现后消失，卡片网格渲染
 *  - error：无旧数据 → 内联错误条 + 重试；重试成功恢复内容
 *  - 刷新失败但保留旧数据 → 顶部刷新提示条 + 旧内容仍在
 *  - 空态①（无日报）：「生成日报」引导 → 触发端点 → WS done 后自动刷新
 *  - 空态②（有日报无活跃泳道）：文案，无卡片无候选栏
 *  - 候选栏：无候选不渲染；有条目点击 emit selectTopic；卡片点击同 emit
 *
 * API composables / WS 进度 / 通知均 mock，无网络调用。
 */
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import LaneDynamicsPanel from './LaneDynamicsPanel.vue'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const getLaneDynamics = vi.fn()
const generateDailyReport = vi.fn()
const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

// useDailyReportProgress mock：done/progress 为可写 ref，测试里直接驱动 WS 完成信号
const progressRef = ref(new Map())
const doneRef = ref(false)
const totalSavedRef = ref(0)

vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics }),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ generateDailyReport }),
}))

vi.mock('~/composables/useDailyReportProgress', () => ({
  useDailyReportProgress: () => ({
    progress: progressRef,
    done: doneRef,
    jobId: ref(null),
    totalSaved: totalSavedRef,
    totalBoards: ref(0),
    reset: vi.fn(),
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))

function makeData(overrides: Partial<LaneDynamicsResponse> = {}): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    lanes: [
      {
        topic_id: 101,
        label: '美联储利率路径',
        watch_linked: true,
        section_count_14d: 12,
        snapshot: { summary: '降息预期反复拉锯。', as_of: '2026-09-09' },
        timeline: [
          {
            date: '2026-09-08',
            sections: [{ section_id: 1, label: '非农数据', events: ['非农低于预期'] }],
          },
        ],
      },
      {
        topic_id: 102,
        label: '日元套息交易逆转',
        watch_linked: false,
        section_count_14d: 5,
        snapshot: null,
        timeline: [],
      },
    ],
    candidates: [
      { topic_id: 201, label: '中东局势与油价', last_seen_date: '2026-09-08', recent_hint: '停火谈判再现僵局' },
    ],
    ...overrides,
  }
}

// vitest 未启 auto-unmount：共享 done/progress ref 上挂着的 watch(done) 若组件不卸载，
// 会在下个 beforeEach 重置 ref 时僵尸触发 load（打向已 reset 的 mock）→ 显式卸载。
let activeWrapper: VueWrapper | null = null

function mountPanel(boardId = 7) {
  activeWrapper = mount(LaneDynamicsPanel, { props: { boardId } })
  return activeWrapper
}

beforeEach(() => {
  getLaneDynamics.mockReset()
  generateDailyReport.mockReset()
  notifyMocks.success.mockClear()
  notifyMocks.error.mockClear()
  progressRef.value = new Map()
  doneRef.value = false
  totalSavedRef.value = 0
})

afterEach(() => {
  activeWrapper?.unmount()
  activeWrapper = null
})

describe('LaneDynamicsPanel — loading / success', () => {
  it('loading 显示卡片骨架占位，完成后渲染卡片网格与候选栏', async () => {
    let resolveFetch: (v: { success: boolean, data?: LaneDynamicsResponse }) => void = () => {}
    getLaneDynamics.mockReturnValue(new Promise(resolve => { resolveFetch = resolve }))

    const wrapper = mountPanel()
    expect(getLaneDynamics).toHaveBeenCalledWith(7, 14)
    // loading：骨架 shimmer（卡片占位，非转圈盖层）
    expect(wrapper.find('[data-testid="lane-skeleton"]').exists()).toBe(true)
    expect(wrapper.findAll('.ldp-skeleton').length).toBe(4)

    resolveFetch({ success: true, data: makeData() })
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-skeleton"]').exists()).toBe(false)
    const cards = wrapper.findAll('[data-testid="lane-card"]')
    expect(cards.length).toBe(2)
    expect(wrapper.text()).toContain('美联储利率路径')
    expect(wrapper.text()).toContain('降息预期反复拉锯。')
    // 候选栏渲染
    expect(wrapper.find('[data-testid="lane-candidate-bar"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('中东局势与油价')
  })

  it('卡片点击 → emit selectTopic（联动 TagsPage 切话题总览 focus）', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: makeData() })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="lane-card"]').trigger('click')
    expect(wrapper.emitted('selectTopic')).toEqual([[101]])
  })
})

describe('LaneDynamicsPanel — error / retry', () => {
  it('首次加载失败：内联错误条 + 重试；重试成功恢复内容', async () => {
    getLaneDynamics.mockResolvedValueOnce({ success: false, error: 'boom' })
    const wrapper = mountPanel()
    await flushPromises()

    const errBar = wrapper.find('[data-testid="lane-error"]')
    expect(errBar.exists()).toBe(true)
    expect(errBar.text()).toContain('boom')
    expect(wrapper.find('[data-testid="lane-card"]').exists()).toBe(false)

    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeData() })
    await wrapper.find('[data-testid="lane-retry"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="lane-card"]').length).toBe(2)
    expect(getLaneDynamics).toHaveBeenCalledTimes(2)
  })

  it('刷新失败但保留旧数据：顶部刷新提示条 + 旧内容仍在', async () => {
    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeData() })
    const wrapper = mountPanel()
    await flushPromises()

    getLaneDynamics.mockResolvedValueOnce({ success: false, error: '断网' })
    await wrapper.find('[data-testid="lane-refresh"]').trigger('click')
    await flushPromises()

    // 旧数据保留 + 顶部刷新提示条（非清空式错误）
    expect(wrapper.findAll('[data-testid="lane-card"]').length).toBe(2)
    const stale = wrapper.find('[data-testid="lane-stale-notice"]')
    expect(stale.exists()).toBe(true)
    expect(stale.text()).toContain('断网')
    expect(wrapper.find('[data-testid="lane-error"]').exists()).toBe(false)
  })
})

describe('LaneDynamicsPanel — 空态', () => {
  it('空态① 无日报：生成日报引导 → WS done 后自动刷新', async () => {
    // 初始：无日报
    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeData({ has_reports: false, lanes: [], candidates: [] }) })
    const wrapper = mountPanel()
    await flushPromises()

    const empty = wrapper.find('[data-testid="lane-empty-no-reports"]')
    expect(empty.exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-generate-btn"]').exists()).toBe(true)

    // 点击生成日报 → 触发既有端点（board_id 归属当前板块）
    generateDailyReport.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'queued' } })
    await wrapper.find('[data-testid="lane-generate-btn"]').trigger('click')
    await flushPromises()
    expect(generateDailyReport).toHaveBeenCalledWith({ date: expect.any(String), board_id: 7 })
    // 生成中：按钮禁用态文案
    expect(wrapper.find('[data-testid="lane-generate-btn"]').text()).toContain('生成中')

    // WS done（useDailyReportProgress）→ 通知 + 自动刷新（spec「生成后刷新」）
    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeData() })
    totalSavedRef.value = 5
    doneRef.value = true
    await flushPromises()

    expect(notifyMocks.success).toHaveBeenCalledWith('日报已生成（共 5 篇）')
    expect(getLaneDynamics).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[data-testid="lane-card"]').length).toBe(2)
    expect(wrapper.emitted('generated')).toBeTruthy()
  })

  it('生成触达失败：错误通知 + 退出 generating 态', async () => {
    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeData({ has_reports: false, lanes: [], candidates: [] }) })
    const wrapper = mountPanel()
    await flushPromises()

    generateDailyReport.mockResolvedValue({ success: false, error: '服务器开小差' })
    await wrapper.find('[data-testid="lane-generate-btn"]').trigger('click')
    await flushPromises()

    expect(notifyMocks.error).toHaveBeenCalledWith('服务器开小差')
    expect(wrapper.find('[data-testid="lane-generate-btn"]').text()).toContain('生成日报')
  })

  it('空态② 有日报无活跃泳道：文案，无卡片无候选栏', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: makeData({ lanes: [], candidates: [] }) })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-empty-no-lanes"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('暂无活跃泳道')
    expect(wrapper.find('[data-testid="lane-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-candidate-bar"]').exists()).toBe(false)
  })
})

describe('LaneDynamicsPanel — 候选栏（tasks 3.4）', () => {
  it('无候选不渲染候选栏', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: makeData({ candidates: [] }) })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-candidate-bar"]').exists()).toBe(false)
  })

  it('候选条目点击 → emit selectTopic（跳话题总览 focus）', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: makeData() })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="lane-candidate-201"]').trigger('click')
    expect(wrapper.emitted('selectTopic')).toEqual([[201]])
  })
})
