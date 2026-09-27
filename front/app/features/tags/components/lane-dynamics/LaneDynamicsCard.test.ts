/**
 * LaneDynamicsCard 组件测试（overview-lane-dynamics tasks 3.3）。
 *
 * 覆盖 spec「泳道卡片构成」+「单日多事件」+ ui-design 折叠交互：
 *  - 待结算降级：snapshot=null → 占位条 + 时间线照常渲染（不置灰）
 *  - 态势句 + as_of 标注（"2026-09-09" → "9/9"）
 *  - watch 角标：watch_linked 才渲染「追踪中」
 *  - 折叠展开：单日超 5 条 → 前 5 条 + 「还有 N 条」；点击就地展开、不触发卡片 select
 *  - 后端截断标注：folded_count → 「另有 N 条未载入」（不可展开）
 *  - 日期内多 section 分组渲染
 *  - 整卡点击 emit select(topic_id)
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneDynamicsCard from './LaneDynamicsCard.vue'
import type { LaneDynamicsLane } from '~/api/laneDynamics'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeLane(overrides: Partial<LaneDynamicsLane> = {}): LaneDynamicsLane {
  return {
    topic_id: 101,
    label: '美联储利率路径',
    watch_linked: false,
    section_count_14d: 3,
    snapshot: {
      summary: '近两周围绕降息节奏反复拉锯。',
      as_of: '2026-09-09',
    },
    timeline: [
      {
        date: '2026-09-08',
        sections: [
          { section_id: 1, label: '8月非农不及预期', events: ['非农低于预期', '降息概率升'] },
        ],
      },
      {
        date: '2026-09-05',
        sections: [
          { section_id: 2, label: 'ADP 数据平淡', events: ['ADP 符合预期'] },
        ],
      },
    ],
    ...overrides,
  }
}

function mountCard(lane: LaneDynamicsLane) {
  return mount(LaneDynamicsCard, {
    props: { lane, windowDays: 14 },
  })
}

describe('LaneDynamicsCard', () => {
  it('渲染态势句与汇总截止标注（as_of 格式化为 M/D）', () => {
    const wrapper = mountCard(makeLane())
    expect(wrapper.find('[data-testid="lane-stance"]').text()).toContain('反复拉锯')
    expect(wrapper.find('[data-testid="lane-asof"]').text()).toBe('汇总截止 9/9')
    expect(wrapper.find('[data-testid="lane-pending"]').exists()).toBe(false)
  })

  it('待结算降级：snapshot=null → 占位条 + 时间线照常渲染', () => {
    const wrapper = mountCard(makeLane({ snapshot: null }))
    expect(wrapper.find('[data-testid="lane-pending"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-pending"]').text()).toContain('态势待结算')
    expect(wrapper.find('[data-testid="lane-stance"]').exists()).toBe(false)
    // 时间线照常：两天节点 + 全部事件可见
    const tl = wrapper.find('[data-testid="lane-timeline"]')
    expect(tl.exists()).toBe(true)
    expect(wrapper.findAll('.ldc-event').length).toBe(3)
    expect(wrapper.find('[data-testid="lane-day-2026-09-08"]').exists()).toBe(true)
  })

  it('watch 角标：watch_linked=true 渲染「追踪中」，false 不渲染', () => {
    const withBadge = mountCard(makeLane({ watch_linked: true }))
    expect(withBadge.find('[data-testid="lane-watch-badge"]').exists()).toBe(true)
    expect(withBadge.find('[data-testid="lane-watch-badge"]').text()).toContain('追踪中')

    const withoutBadge = mountCard(makeLane({ watch_linked: false }))
    expect(withoutBadge.find('[data-testid="lane-watch-badge"]').exists()).toBe(false)
  })

  it('日期内多 section 分组：按 section 归组渲染标题与事件', () => {
    const wrapper = mountCard(makeLane({
      timeline: [
        {
          date: '2026-09-08',
          sections: [
            { section_id: 1, label: '非农数据', events: ['e1', 'e2'] },
            { section_id: 2, label: '美联储表态', events: ['e3'] },
          ],
        },
      ],
    }))
    const labels = wrapper.findAll('.ldc-section-label').map(n => n.text())
    expect(labels).toEqual(['非农数据', '美联储表态'])
    expect(wrapper.findAll('.ldc-event').length).toBe(3)
  })

  it('单日超 5 条折叠「还有 N 条」，点击就地展开', async () => {
    const events = Array.from({ length: 7 }, (_, i) => `事件${i + 1}`)
    const wrapper = mountCard(makeLane({
      timeline: [{ date: '2026-09-08', sections: [{ section_id: 1, label: '大新闻', events }] }],
    }))

    // 折叠态：前 5 条 + 「还有 2 条」
    expect(wrapper.findAll('.ldc-event').length).toBe(5)
    const more = wrapper.find('[data-testid="lane-day-more-2026-09-08"]')
    expect(more.exists()).toBe(true)
    expect(more.text()).toBe('还有 2 条')

    // 展开后：全部 7 条，折叠链接消失
    await more.trigger('click')
    expect(wrapper.findAll('.ldc-event').length).toBe(7)
    expect(wrapper.find('[data-testid="lane-day-more-2026-09-08"]').exists()).toBe(false)

    // 折叠按钮的点击不得触发卡片 select（stopPropagation）
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('后端截断标注：folded_count > 0 → 「另有 N 条未载入」', () => {
    const wrapper = mountCard(makeLane({
      timeline: [
        {
          date: '2026-09-08',
          sections: [{ section_id: 1, label: '长线索', events: ['e1', 'e2'], folded_count: 4 }],
        },
      ],
    }))
    // 3 条以内无前端折叠，直接显示后端截断标注
    expect(wrapper.findAll('.ldc-event').length).toBe(2)
    const note = wrapper.find('.ldc-folded-note')
    expect(note.exists()).toBe(true)
    expect(note.text()).toBe('另有 4 条未载入')
    expect(wrapper.find('[data-testid="lane-day-more-2026-09-08"]').exists()).toBe(false)
  })

  it('整卡可点：点击卡片 emit select(topic_id)', async () => {
    const wrapper = mountCard(makeLane())
    await wrapper.find('[data-testid="lane-card"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[101]])
  })

  it('键盘可达：Enter 触发 select', async () => {
    const wrapper = mountCard(makeLane())
    await wrapper.find('[data-testid="lane-card"]').trigger('keydown.enter')
    expect(wrapper.emitted('select')).toEqual([[101]])
  })

  it('计数小字：windowDays 与 section_count_14d', () => {
    const wrapper = mountCard(makeLane({ section_count_14d: 12 }))
    expect(wrapper.find('.ldc-count').text()).toBe('14天 · 12 section')
  })
})
