import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import DiscoveryPanel from './DiscoveryPanel.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { DiscoveryHistoryItem, DiscoveryRun } from '~/types/discovery'

/**
 * S1 手动查询独立于个性化推荐（R1）+ 历史区 R5 —— DiscoveryPanel 查询流状态测试。
 * mock useDiscovery composable（真 store + 固定 groups）隔离 apiStore；store 走真实现 +
 * mock api（沿用 5.1 CandidateLibrary.test.ts「真 store + mock api」风格）。
 * 覆盖：空白不请求 / 执行中一次 / 失败保留输入+未更新+重试复用输入 / 成功零条≠失败 /
 * 返回恢复原列表 / 轮询超时转失败提示手动刷新（fake timers）/ 历史四状态与恢复按钮。
 */

const askDiscoveryMock = vi.fn()
const getRunMock = vi.fn()
const getRecommendationsMock = vi.fn()
const getRecommendationHistoryMock = vi.fn()
const restoreRecommendationMock = vi.fn()
const notifyErrorMock = vi.fn()
const notifySuccessMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    getRecommendations: getRecommendationsMock,
    askDiscovery: askDiscoveryMock,
    getRun: getRunMock,
    getRecommendationHistory: getRecommendationHistoryMock,
    restoreRecommendation: restoreRecommendationMock,
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ error: notifyErrorMock, success: notifySuccessMock, warn: vi.fn() }),
}))

vi.mock('~/api/rsshub', () => ({
  useRsshubApi: () => ({ getStatus: () => Promise.resolve({ success: false }) }),
}))

// 候选订阅弹窗（5.3）依赖真 apiStore（模块级 defineStore 依赖 Nuxt auto-import，测试环境无）
vi.mock('~/stores/api', () => ({
  useApiStore: () => ({ categories: [] }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

vi.mock('../composables/useDiscovery', async () => {
  const { computed } = await import('vue')
  const { useDiscoveryStore } = await import('~/stores/discovery')
  return {
    useDiscovery: () => ({
      store: useDiscoveryStore(),
      groups: computed(() => [
        { label: '全局推荐', cards: [{ id: 'card-1' }] },
      ]),
      catalogEmpty: computed(() => false),
    }),
  }
})

vi.mock('./DiscoveryCard.vue', () => ({
  default: { name: 'DiscoveryCard', props: ['card', 'docBase'], template: '<div data-testid="stub-recommend-card" />' },
}))

vi.mock('./DiscoveryRunCard.vue', () => ({
  default: { name: 'DiscoveryRunCard', props: ['item'], template: '<div :data-testid="`stub-run-card-${item.candidateId}`" />' },
}))

function runPayload(over: Partial<DiscoveryRun> = {}): DiscoveryRun {
  return {
    id: '7',
    kind: 'qa',
    query: '量子计算',
    status: 'succeeded',
    startedAt: '2026-09-12T00:00:00Z',
    finishedAt: '2026-09-12T00:00:05Z',
    items: [
      { candidateId: 'a1', name: '量子期刊', description: '', reason: '', recallOrigins: [], availability: 'unknown' },
      { candidateId: 'a2', name: '量子博客', description: '', reason: '', recallOrigins: [], availability: 'unknown' },
    ],
    ...over,
  }
}

function mountPanel() {
  return mount(DiscoveryPanel)
}

async function submitQuery(wrapper: ReturnType<typeof mountPanel>, text: string) {
  await wrapper.find('[data-testid="discovery-query-input"]').find('input').setValue(text)
  await wrapper.find('[data-testid="discovery-query-form"]').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('DiscoveryPanel — 常驻查询区（R1）', () => {
  it('查询框在推荐列表之前常驻渲染（不藏在兴趣页签/弹窗）', () => {
    const wrapper = mountPanel()
    const form = wrapper.find('[data-testid="discovery-query-form"]')
    expect(form.exists()).toBe(true)
    expect(form.text()).toContain('你想看什么内容？')
    expect(wrapper.find('[data-testid="discovery-query-submit"]').exists()).toBe(true)
    // 表单在推荐工具行之前（DOM 顺序）
    const formIdx = (form.element as HTMLElement).compareDocumentPosition(
      wrapper.find('.discovery-toolbar').element as HTMLElement,
    )
    expect(formIdx & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    wrapper.unmount()
  })

  it('空/纯空白提交：就地提示且不发起请求', async () => {
    const wrapper = mountPanel()
    await wrapper.find('[data-testid="discovery-query-form"]').trigger('submit')
    expect(wrapper.find('.app-input-error').text()).toContain('先输入想看的内容')
    expect(askDiscoveryMock).not.toHaveBeenCalled()

    await submitQuery(wrapper, '   \t ')
    expect(wrapper.find('.app-input-error').exists()).toBe(true)
    expect(askDiscoveryMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('超 500 rune 输入被截断并提示', async () => {
    const wrapper = mountPanel()
    const input = wrapper.find('[data-testid="discovery-query-input"]').find('input')
    await input.setValue('长'.repeat(501))
    expect((input.element as HTMLInputElement).value).toHaveLength(500)
    expect(wrapper.find('[data-testid="discovery-query-limit-hint"]').text()).toContain('已达上限')
    wrapper.unmount()
  })
})

describe('DiscoveryPanel — 查询流状态机（S1）', () => {
  it('执行中防重复：按钮禁用且二次提交不再发请求', async () => {
    askDiscoveryMock.mockReturnValue(new Promise(() => {}))
    getRunMock.mockReturnValue(new Promise(() => {}))
    const wrapper = mountPanel()
    await submitQuery(wrapper, '量子计算')
    expect(askDiscoveryMock).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="discovery-run-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="discovery-query-submit"]').attributes('disabled')).toBeDefined()

    // 绕过按钮 disabled 直接再触发 form submit：onSubmit 的 running guard 拦截
    await wrapper.find('[data-testid="discovery-query-form"]').trigger('submit')
    expect(askDiscoveryMock).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('成功：独立结果区「关于××的结果」+ 返回恢复原推荐列表', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({ success: true, data: runPayload() })
    const wrapper = mountPanel()
    await submitQuery(wrapper, '量子计算')

    const runView = wrapper.find('[data-testid="discovery-run-view"]')
    expect(runView.exists()).toBe(true)
    expect(runView.text()).toContain('关于「量子计算」的结果')
    expect(wrapper.findAll('[data-testid^="stub-run-card-"]')).toHaveLength(2)
    // 打开结果视图时推荐工具行隐藏（不混入）
    expect(wrapper.find('.discovery-toolbar').exists()).toBe(false)

    await wrapper.find('[data-testid="discovery-run-back"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="discovery-run-view"]').exists()).toBe(false)
    expect(wrapper.find('.discovery-toolbar').exists()).toBe(true)
    expect(wrapper.find('[data-testid="stub-recommend-card"]').exists()).toBe(true) // 原列表原样恢复
    wrapper.unmount()
  })

  it('成功零条：空态文案明确区分于失败态', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({ success: true, data: runPayload({ items: [] }) })
    const wrapper = mountPanel()
    await submitQuery(wrapper, '量子计算')
    const empty = wrapper.find('[data-testid="discovery-run-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('没有找到')
    expect(empty.text()).toContain('不是服务故障') // 零条 ≠ 失败
    expect(wrapper.find('[data-testid="discovery-run-failed"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('失败：保留输入 + 旧结果标未更新 + 重试复用同一输入', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({ success: true, data: runPayload({ status: 'failed', items: [] }) })
    const wrapper = mountPanel()
    await submitQuery(wrapper, '量子计算')

    const failed = wrapper.find('[data-testid="discovery-run-failed"]')
    expect(failed.exists()).toBe(true)
    expect(failed.text()).toContain('未更新') // 旧推荐未更新明示
    expect(failed.text()).toContain('重试')
    // 输入保留（失败不清空 query）
    const input = wrapper.find('[data-testid="discovery-query-input"]').find('input')
    expect((input.element as HTMLInputElement).value).toBe('量子计算')

    // 重试：复用同一输入再次发起
    askDiscoveryMock.mockClear()
    getRunMock.mockResolvedValue({ success: true, data: runPayload() })
    await wrapper.find('[data-testid="discovery-run-retry"]').trigger('click')
    await flushPromises()
    expect(askDiscoveryMock).toHaveBeenCalledWith('量子计算')
    expect(wrapper.find('[data-testid="discovery-run-list"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('ask 启动即失败：显示失败态与可定位错误', async () => {
    askDiscoveryMock.mockResolvedValue({ success: false, error: '服务不可用' })
    const wrapper = mountPanel()
    await submitQuery(wrapper, '量子计算')
    const failed = wrapper.find('[data-testid="discovery-run-failed"]')
    expect(failed.exists()).toBe(true)
    expect(failed.text()).toContain('服务不可用')
    expect(wrapper.find('[data-testid="discovery-run-loading"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('轮询超时（2s×60）转失败并提示手动刷新（design D2）', async () => {
    vi.useFakeTimers()
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '9' } })
    getRunMock.mockResolvedValue({ success: true, data: runPayload({ id: '9', status: 'running', finishedAt: null }) })
    const wrapper = mountPanel()

    await wrapper.find('[data-testid="discovery-query-input"]').find('input').setValue('量子计算')
    wrapper.find('[data-testid="discovery-query-form"]').trigger('submit') // 不 await：轮询挂着
    await vi.advanceTimersByTimeAsync(2000 * 60 + 2000)
    await flushPromises()

    const failed = wrapper.find('[data-testid="discovery-run-failed"]')
    expect(failed.exists()).toBe(true)
    expect(failed.text()).toContain('手动刷新') // 超时提示手动刷新
    expect(getRunMock).toHaveBeenCalledTimes(60) // 上限 60 次不无限轮询
    vi.useRealTimers()
    wrapper.unmount()
  })
})

describe('DiscoveryPanel — 历史区（R5）', () => {
  function historyItem(over: Partial<DiscoveryHistoryItem> = {}): DiscoveryHistoryItem {
    return {
      id: '1',
      name: '某推荐源',
      status: 'expired',
      snoozedUntil: null,
      lastSelectedAt: '2026-09-01T00:00:00Z',
      reason: '',
      ...over,
    }
  }

  function seedHistory(items: DiscoveryHistoryItem[]) {
    const store = useDiscoveryStore()
    store.history = items
    store.historyLoaded = true
    store.historyError = null
    return store
  }

  async function openHistory(wrapper: ReturnType<typeof mountPanel>) {
    await wrapper.find('[data-testid="discovery-mode-history"]').trigger('click')
    await nextTick()
    await flushPromises()
  }

  it('四状态文案区分；自动过期不称「不感兴趣」；暂时不看显示到期时间', async () => {
    seedHistory([
      historyItem({ id: '1', status: 'accepted' }),
      historyItem({ id: '2', status: 'expired' }),
      historyItem({ id: '3', status: 'snoozed', snoozedUntil: '2026-10-12T00:00:00Z' }),
      historyItem({ id: '4', status: 'excluded' }),
    ])
    const wrapper = mountPanel()
    await openHistory(wrapper)

    const badges = wrapper.findAll('.discovery-history__badge')
    expect(badges.map(b => b.text())).toEqual(['已订阅', '自动过期', '暂时不看', '长期排除'])
    const notes = wrapper.findAll('.discovery-history__note')
    expect(notes[1]!.text()).toContain('不是你拒绝过') // 自动过期 ≠ 拒绝
    expect(notes[1]!.text()).not.toContain('不感兴趣')
    expect(notes[2]!.text()).toContain('2026-10-12T00:00:00Z') // 冷却到期时间
    // 仅长期排除带恢复按钮
    expect(wrapper.find('[data-testid="discovery-history-restore-4"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="discovery-history-restore-1"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('恢复按钮调用 store.restoreRecommendation；成功后条目转「已恢复资格」', async () => {
    seedHistory([historyItem({ id: '4', status: 'excluded' })])
    restoreRecommendationMock.mockResolvedValue({ success: true })
    const wrapper = mountPanel()
    await openHistory(wrapper)

    await wrapper.find('[data-testid="discovery-history-restore-4"]').trigger('click')
    await flushPromises()
    expect(restoreRecommendationMock).toHaveBeenCalledWith('4')
    // 文案仅提示恢复资格，不承诺出卡、不自动订阅
    expect(notifySuccessMock).toHaveBeenCalledWith(expect.stringContaining('不会自动订阅'))
    expect(wrapper.find('.discovery-history__badge').text()).toBe('已恢复资格')
    // 恢复后按钮消失，不冒充可再次恢复
    expect(wrapper.find('[data-testid="discovery-history-restore-4"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('历史加载失败显示错误态与重试，不冒称空', async () => {
    const store = useDiscoveryStore()
    store.historyLoaded = false
    getRecommendationHistoryMock.mockResolvedValue({ success: false, error: '网关超时' })
    const wrapper = mountPanel()
    await openHistory(wrapper)
    const error = wrapper.find('[data-testid="discovery-history-error"]')
    expect(error.exists()).toBe(true)
    expect(error.text()).toContain('推荐历史加载失败')
    expect(wrapper.find('[data-testid="discovery-history-empty"]').exists()).toBe(false)

    getRecommendationHistoryMock.mockResolvedValue({
      success: true,
      data: [historyItem({ status: 'expired' })],
    })
    await wrapper.find('[data-testid="discovery-history-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="discovery-history-list"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('空历史显示空态文案', async () => {
    seedHistory([])
    const wrapper = mountPanel()
    await openHistory(wrapper)
    const empty = wrapper.find('[data-testid="discovery-history-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('还没有历史记录')
    wrapper.unmount()
  })
})
