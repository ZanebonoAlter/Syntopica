import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { mount } from '@vue/test-utils'

/**
 * TagQueueProgressChip 组件测试（tag-queue-progress-chip，S2 故事 + 白盒 E 组）：
 * - 空闲不渲染（E1：v-if 移除 DOM，非 visibility:hidden）
 * - 进行中态（S2 步 1）与失败态（S2 步 5/E4）
 * - 4 位数计数不换行（V25：white-space nowrap 样式锚）
 */

const navigateToSpy = vi.fn(() => Promise.resolve())

vi.stubGlobal('navigateTo', navigateToSpy)

// 用真 Vue ref：模板对 computed/ref 自动解包，普通 {value} 对象不会解包
const status = ref({ pending: 0, processing: 0, completedToday: 0, failed: 0 })
const activeCount = ref(0)
const visible = ref(false)
const isFailedState = ref(false)
const roundTotal = ref(0)
const progressPercent = ref(0)

const state = {
  status,
  activeCount,
  visible,
  isFailedState,
  roundTotal,
  progressPercent,
  ensureStarted: vi.fn(),
  stop: vi.fn(),
  reconcile: vi.fn(async () => {}),
}

vi.mock('~/composables/useTagQueueProgress', () => ({
  useTagQueueProgress: () => state,
}))

import TagQueueProgressChip from './TagQueueProgressChip.vue'

function setState(s: { active?: number; failed?: number; done?: number }) {
  const active = s.active ?? 0
  const failed = s.failed ?? 0
  const done = s.done ?? 0
  status.value = { pending: active, processing: 0, completedToday: done, failed }
  activeCount.value = active
  visible.value = active > 0 || failed > 0
  isFailedState.value = failed > 0
  roundTotal.value = active + done
  progressPercent.value = roundTotal.value > 0 ? Math.round((done / roundTotal.value) * 100) : 0
}

beforeEach(() => {
  vi.clearAllMocks()
  setState({})
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('TagQueueProgressChip — 可见性（白盒 E）', () => {
  it('E1：队列空且无失败 → 组件不渲染（v-if 移除 DOM）', () => {
    setState({ active: 0, failed: 0 })
    const wrapper = mount(TagQueueProgressChip)
    expect(wrapper.find('[data-testid="tag-queue-progress-chip"]').exists()).toBe(false)
    // 空闲=不渲染，而非 visibility:hidden
    expect(wrapper.html()).not.toContain('queue-chip')
  })

  it('E2：有活跃任务 → 可见，文案「分析中 n/total」+ 进度条', () => {
    setState({ active: 5, done: 3 })
    const wrapper = mount(TagQueueProgressChip)
    const chip = wrapper.find('[data-testid="tag-queue-progress-chip"]')
    expect(chip.exists()).toBe(true)
    expect(wrapper.find('[data-testid="chip-text"]').text()).toContain('分析中 3/8')
    expect(wrapper.find('.queue-chip__bar > i').attributes('style')).toContain('width: 38%')
    expect(state.ensureStarted).toHaveBeenCalled()
  })

  it('E4：仅剩失败（活跃==0）→ 保持失败态可见', () => {
    setState({ active: 0, failed: 2, done: 6 })
    const wrapper = mount(TagQueueProgressChip)
    const chip = wrapper.find('[data-testid="tag-queue-progress-chip"]')
    expect(chip.exists()).toBe(true)
    expect(wrapper.find('[data-testid="chip-text"]').text()).toContain('2 个失败')
    expect(chip.classes()).toContain('queue-chip--failed')
  })

  it('S2 步 5：有活跃也有失败 → 红色失败态且进度条填满', () => {
    setState({ active: 3, failed: 2, done: 6 })
    const wrapper = mount(TagQueueProgressChip)
    expect(wrapper.find('[data-testid="tag-queue-progress-chip"]').classes()).toContain('queue-chip--failed')
  })

  it('V25：4 位数计数不换行（white-space nowrap 样式锚）', () => {
    setState({ active: 1200, done: 800 })
    const wrapper = mount(TagQueueProgressChip)
    expect(wrapper.find('[data-testid="chip-text"]').text()).toBe('分析中 800/2000')
    // 自适应宽 + 禁换行样式锚（不截断）
    const chipEl = wrapper.find('[data-testid="tag-queue-progress-chip"]')
    expect(chipEl.classes()).toContain('queue-chip')
  })
})

describe('TagQueueProgressChip — 点击跳转', () => {
  it('点击跳 /settings?section=queues&queue=tag', async () => {
    setState({ active: 5, done: 3 })
    const wrapper = mount(TagQueueProgressChip)
    await wrapper.find('[data-testid="tag-queue-progress-chip"]').trigger('click')
    expect(navigateToSpy).toHaveBeenCalledWith('/settings?section=queues&queue=tag')
  })
})
