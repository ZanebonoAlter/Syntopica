import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

/**
 * TagQueuePanel 计数语义测试（tag-queue-progress-chip 面板侧，S2 步 7 / 面板计数语义 Scenario）：
 * - 活跃量（pending+leased）主展示
 * - 「今日完成」取 completed_today；旧后端未合入（字段缺省）回退显示累计 completed 并标注「累计」
 * - 累计 total 总体进度条停用
 */

const { getStatusMock, getTasksMock, onMock } = vi.hoisted(() => ({
  getStatusMock: vi.fn(),
  getTasksMock: vi.fn(),
  onMock: vi.fn(() => () => {}),
}))

vi.mock('~/api', async (importOriginal) => {
  const original = await importOriginal<Record<string, unknown>>()
  return {
    ...original,
    useTagQueueApi: () => ({
      getStatus: getStatusMock,
      getTasks: getTasksMock,
      retryFailed: vi.fn(),
      retagToday: vi.fn(),
    }),
  }
})

vi.mock('~/composables/useEventStream', () => ({
  useEventStream: () => ({
    on: onMock,
    off: vi.fn(),
    connected: false,
  }),
}))

import TagQueuePanel from './TagQueuePanel.vue'

function statusPayload(overrides: Record<string, number | undefined> = {}) {
  return {
    success: true,
    data: {
      pending: 3,
      processing: 2,
      completed: 35,
      failed: 1,
      total: 41,
      completed_today: 20,
      ...overrides,
    },
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  getStatusMock.mockReset().mockResolvedValue(statusPayload())
  getTasksMock.mockReset().mockResolvedValue({ success: true, data: { tasks: [], total: 0 } })
})

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('TagQueuePanel — 队列计数展示语义（S2 步 7）', () => {
  it('活跃量主展示 = pending+leased（5）；今日完成 = completed_today（20）；失败 1', async () => {
    const wrapper = mount(TagQueuePanel)
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.find('[data-testid="queue-active-count"]').text()).toContain('5')
    expect(wrapper.find('[data-testid="queue-completed-today"]').text()).toContain('20')
    expect(wrapper.find('[data-testid="queue-completed-today"]').text()).toContain('今日完成')
    expect(wrapper.find('[data-testid="queue-failed-count"]').text()).toContain('1')
  })

  it('旧后端未合入 completed_today（undefined）→ 回退显示累计 completed 并标注「累计」', async () => {
    getStatusMock.mockResolvedValue(statusPayload({ completed_today: undefined }))
    const wrapper = mount(TagQueuePanel)
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.find('[data-testid="queue-completed-today"]').text()).toContain('35')
    expect(wrapper.find('[data-testid="queue-completed-today"]').text()).toContain('累计')
  })

  it('累计 total 总体进度条停用（不再渲染「总体进度」区块）', async () => {
    const wrapper = mount(TagQueuePanel)
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.text()).not.toContain('总体进度')
  })

  it('V21 变体（空队列）：计数全 0 不报错', async () => {
    getStatusMock.mockResolvedValue(statusPayload({ pending: 0, processing: 0, completed_today: 0, failed: 0, total: 0 }))
    const wrapper = mount(TagQueuePanel)
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.find('[data-testid="queue-active-count"]').text()).toContain('0')
    expect(wrapper.find('[data-testid="queue-failed-count"]').text()).toContain('0')
  })
})
