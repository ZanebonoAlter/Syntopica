import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DiscoveryRunCard from './DiscoveryRunCard.vue'
import type { DiscoveryRunItem } from '~/types/discovery'

/**
 * 查询结果卡片（独立 run 视图条目；test-cases S6-4 组件面 / R6）：
 * - 展示名称 / 召回来源徽标 / 描述 / 推荐理由 / 可用性四态文案；
 * - unknown = 未验证：明示、不伪造时间或匹配百分比（全卡不出现 %）；
 * - 缺失字段降级（无名称 → 未知来源；无来源/描述/理由不渲染对应块）；
 * - 超长名称换行锚点（u-break-title）。
 */

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function item(over: Partial<DiscoveryRunItem> = {}): DiscoveryRunItem {
  return {
    candidateId: '42',
    name: '东洋经济周刊',
    description: '日本商业与经济深度报道',
    reason: '与你查询的「日本经济」方向一致',
    recallOrigins: ['版块召回', '行为画像'],
    availability: 'unknown',
    ...over,
  }
}

function mountCard(props: { item: DiscoveryRunItem }) {
  return mount(DiscoveryRunCard, { props })
}

describe('DiscoveryRunCard — 字段展示', () => {
  it('渲染名称/描述/理由与召回来源徽标', () => {
    const wrapper = mountCard({ item: item() })
    expect(wrapper.find('[data-testid="run-card"]').exists()).toBe(true)
    expect(wrapper.find('.run-card__name').text()).toBe('东洋经济周刊')
    expect(wrapper.find('.run-card__desc').text()).toContain('日本商业与经济深度报道')
    expect(wrapper.find('.run-card__reason').text()).toContain('日本经济')
    const origins = wrapper.findAll('.run-card__origin')
    expect(origins.map(o => o.text())).toEqual(['版块召回', '行为画像']) // 实际召回来源，可多个
    wrapper.unmount()
  })

  it('可用性四态文案：未验证/可用/失效/需填参验证', () => {
    const cases: Array<[DiscoveryRunItem['availability'], string]> = [
      ['unknown', '未验证'], // 无检查记录明示未验证，不冒称可用
      ['ok', '可用'],
      ['broken', '失效'],
      ['requires_parameters', '需填参验证'], // 需参数 ≠ 失效
    ]
    for (const [availability, text] of cases) {
      const wrapper = mountCard({ item: item({ availability }) })
      expect(wrapper.find('.run-card__availability').text()).toBe(text)
      wrapper.unmount()
    }
  })

  it('缺失字段降级：无名称显示未知来源；空来源/描述/理由不渲染对应块', () => {
    const wrapper = mountCard({ item: item({ name: '', description: '', reason: '', recallOrigins: [] }) })
    expect(wrapper.find('.run-card__name').text()).toBe('未知来源')
    expect(wrapper.find('.run-card__origins').exists()).toBe(false)
    expect(wrapper.find('.run-card__desc').exists()).toBe(false)
    expect(wrapper.find('.run-card__reason').exists()).toBe(false)
    wrapper.unmount()
  })

  it('不出现百分比（无检查记录不伪造匹配度）', () => {
    const wrapper = mountCard({ item: item() })
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.text()).not.toContain('％')
    wrapper.unmount()
  })

  it('超长名称带换行锚点（u-break-title）', () => {
    const wrapper = mountCard({ item: item({ name: '长'.repeat(150) }) })
    expect(wrapper.find('.run-card__name.u-break-title').exists()).toBe(true)
    wrapper.unmount()
  })
})
