import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import KeyEventLabel from './KeyEventLabel.vue'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'

function event(overrides: Partial<BoardKeyEventItem> = {}): BoardKeyEventItem {
  return {
    id: '31', topicId: '93', topicLabelSnapshot: 'AI 模型发布',
    sourcePeriodDate: '2026-11-12', titleSnapshot: '婚外情曝光触发董事会调查与罢免程序',
    summarySnapshot: '', articleIds: [], ...overrides,
  }
}

describe('KeyEventLabel — 标签不截断', () => {
  it('完整显示标题全文（长标题不省略）', () => {
    const wrapper = mount(KeyEventLabel, {
      props: { event: event(), left: 100, top: 46 },
    })
    const label = wrapper.find('[data-testid="key-event-label-31"]')
    expect(label.text()).toBe('婚外情曝光触发董事会调查与罢免程序')
  })

  it('自然宽度渲染：不换行、无 text-overflow（源码契约，happy-dom 不解析 scoped CSS 改读源码）', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const source = readFileSync(resolve(process.cwd(), 'app/features/tags/components/console/KeyEventLabel.vue'), 'utf-8')
    expect(source).toContain('white-space: nowrap')
    expect(source).not.toContain('text-overflow:')
  })

  it('点击转发 select（选日）；测量态保持隐藏', async () => {
    const wrapper = mount(KeyEventLabel, {
      props: { event: event(), left: 0, top: 0, measuring: true },
    })
    expect(wrapper.find('.kev-label').classes()).toContain('is-measuring')
    await wrapper.find('[data-testid="key-event-label-31"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['2026-11-12']])
  })

  it('定位由父注入（left/top 绝对定位）', () => {
    const wrapper = mount(KeyEventLabel, {
      props: { event: event(), left: 234, top: 90 },
    })
    const style = (wrapper.find('.kev-label').element as HTMLElement).style
    expect(style.left).toBe('234px')
    expect(style.top).toBe('90px')
  })

  it('title 携带日期（节点错位时可辨）', () => {
    const wrapper = mount(KeyEventLabel, {
      props: { event: event(), left: 0, top: 0 },
    })
    expect(wrapper.find('[data-testid="key-event-label-31"]').attributes('title')).toContain('2026-11-12')
    expect(vi.fn()).toBeDefined()
  })
})
