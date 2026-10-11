import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { DegradationEvent, DegradationReport } from '~/types'

const apiMocks = vi.hoisted(() => ({
  getDegradations: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import DegradationPanel from './DegradationPanel.vue'

const stubs = { Icon: true }

function evt(partial: Partial<DegradationEvent> & Pick<DegradationEvent, 'capability' | 'type'>): DegradationEvent {
  return {
    occurred_at: '2026-09-28T02:14:33Z',
    from: null,
    to: null,
    model: 'qwythos',
    code: 'timeout',
    ...partial,
  }
}

function reportFixture(partial: Partial<DegradationReport> = {}): DegradationReport {
  return {
    window: '24h',
    stats: { calls: 1842, degradations: 12, failures: 3 },
    events: [
      evt({ capability: 'topic_tagging', type: 'fallback', from: 0, to: 1, model: 'qwythos', code: 'empty_response' }),
      evt({ capability: 'digest_polish', type: 'structured', model: 'deepseek-flash', code: 'json_schema→json_object' }),
      evt({ capability: 'summary', type: 'fallback', model: 'deepseek-flash', code: 'timeout' }),
    ],
    ...partial,
  }
}

async function mountPanel() {
  const wrapper = mount(DegradationPanel, { global: { stubs } })
  await flushPromises()
  return wrapper
}

describe('DegradationPanel（U6）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getDegradations.mockResolvedValue({ success: true, data: reportFixture() })
  })

  it('挂载按默认 24h 请求；统计行渲染 stats 三值；切换 7d 即按新窗口重新请求', async () => {
    const wrapper = await mountPanel()

    expect(apiMocks.getDegradations).toHaveBeenCalledWith('24h')
    const stats = wrapper.find('.obs-head__stats')
    expect(stats.text()).toContain('1842')
    expect(stats.text()).toContain('调用')
    expect(stats.text()).toContain('12')
    expect(stats.text()).toContain('降级')
    expect(stats.text()).toContain('3')
    expect(stats.text()).toContain('失败')

    await wrapper.findAll('.obs-seg__btn').find(b => b.text() === '7d')!.trigger('click')
    await flushPromises()

    expect(apiMocks.getDegradations).toHaveBeenCalledWith('7d')
    expect(apiMocks.getDegradations).toHaveBeenCalledTimes(2)
    // 选中态跟随窗口
    expect(wrapper.findAll('.obs-seg__btn').find(b => b.text() === '7d')!.classes()).toContain('obs-seg__btn--on')
  })

  it('事件行渲染：时间形态/能力/顺位迁移（from→to）全链失败标记/模型/错误码 + title', async () => {
    const wrapper = await mountPanel()
    const rows = wrapper.findAll('.evt-row')
    expect(rows).toHaveLength(3)

    // fallback 0→1 → 顺位1→2（1-based 展示）
    expect(rows[0]!.find('.evt-row__move').text()).toBe('顺位1→2')
    expect(rows[0]!.find('.evt-row__move--fail').exists()).toBe(false)
    expect(rows[0]!.text()).toContain('主题提取')
    expect(rows[0]!.find('.evt-row__model').text()).toBe('qwythos')
    expect(rows[0]!.find('.evt-row__code').text()).toBe('empty_response')
    // 时间为 MM-DD HH:mm 形态（时区相关，不断言具体值）
    expect(rows[0]!.find('.evt-row__time').text()).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/)

    // structured 事件无顺位概念（—），类型标 accent 色
    expect(rows[1]!.find('.evt-row__move').text()).toBe('—')
    expect(rows[1]!.find('.evt-row__type--structured').text()).toBe('结构化降级')
    expect(rows[1]!.find('.evt-row__code').attributes('title')).toBe('json_schema→json_object')

    // 全链失败：fallback 且 from/to 为空
    expect(rows[2]!.find('.evt-row__move').text()).toBe('全链失败')
    expect(rows[2]!.find('.evt-row__move--fail').exists()).toBe(true)
    expect(rows[2]!.find('.evt-row__model').attributes('title')).toBe('deepseek-flash')
  })

  it('类型标两色：fallback=供应商失败（warning 形态 class）', async () => {
    const wrapper = await mountPanel()
    const rows = wrapper.findAll('.evt-row')
    expect(rows[0]!.find('.evt-row__type--fallback').text()).toBe('供应商失败')
    expect(rows[0]!.find('.evt-row__type--structured').exists()).toBe(false)
  })

  it('「定位」emit locate 事件并携带事件 capability', async () => {
    const wrapper = await mountPanel()
    const rows = wrapper.findAll('.evt-row')

    await rows[1]!.find('.evt-row__locate').trigger('click')
    await rows[2]!.find('.evt-row__locate').trigger('click')

    expect(wrapper.emitted('locate')).toHaveLength(2)
    expect(wrapper.emitted('locate')![0]).toEqual(['digest_polish'])
    expect(wrapper.emitted('locate')![1]).toEqual(['summary'])
  })

  it('空窗口态：events 为空显「窗口内无记录」，stats 计数照常返回', async () => {
    apiMocks.getDegradations.mockResolvedValue({
      success: true,
      data: reportFixture({ events: [], stats: { calls: 5, degradations: 0, failures: 0 } }),
    })
    const wrapper = await mountPanel()

    expect(wrapper.find('.obs-empty').text()).toBe('窗口内无记录')
    expect(wrapper.find('.evt-row').exists()).toBe(false)
    expect(wrapper.find('.obs-head__stats').text()).toContain('5 调用')
  })

  it('loading：首载未返回时渲染骨架行；error：内联条原文 + 重试重发同窗口请求', async () => {
    let resolve!: (v: unknown) => void
    apiMocks.getDegradations.mockImplementationOnce(() => new Promise((r) => { resolve = r }))
    const wrapper = mount(DegradationPanel, { global: { stubs } })

    // 挂载后请求未返回：骨架行可见、无事件行
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.obs-skeleton').exists()).toBe(true)
    expect(wrapper.find('.evt-row').exists()).toBe(false)

    resolve({ success: true, data: reportFixture() })
    await flushPromises()
    expect(wrapper.find('.obs-skeleton').exists()).toBe(false)

    apiMocks.getDegradations.mockResolvedValueOnce({ success: false, error: 'window must be 24h or 7d' })
    await wrapper.findAll('.obs-seg__btn').find(b => b.text() === '7d')!.trigger('click')
    await flushPromises()

    const errorBar = wrapper.find('.obs-error')
    expect(errorBar.text()).toContain('window must be 24h or 7d')
    // State Matrix「错误恢复」：保留上次结果
    expect(wrapper.find('.evt-row').exists()).toBe(true)

    apiMocks.getDegradations.mockResolvedValueOnce({ success: true, data: reportFixture({ window: '7d' }) })
    await errorBar.find('.obs-error__retry').trigger('click')
    await flushPromises()
    expect(apiMocks.getDegradations).toHaveBeenLastCalledWith('7d')
    expect(wrapper.find('.obs-error').exists()).toBe(false)
  })
})
