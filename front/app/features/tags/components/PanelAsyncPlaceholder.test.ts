import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

/**
 * 懒加载面板占位（fix-spa-nav-loading-ux D3 / test-cases 主链路步 8）：
 * 极简居中 spinner，role=status + aria-live=polite，reduced-motion 停用旋转。
 */

vi.mock('@iconify/vue', () => ({
  Icon: {
    name: 'Icon',
    // 不声明 props：icon 等 attrs 透传到 span，便于断言（仓库既有 Icon mock 手法）
    inheritAttrs: true,
    template: '<span class="icon-mock" />',
  },
}))

import PanelAsyncPlaceholder from './PanelAsyncPlaceholder.vue'

describe('PanelAsyncPlaceholder', () => {
  it('渲染居中 spinner 占位：role=status + reduced-motion 降级类', () => {
    const wrapper = mount(PanelAsyncPlaceholder)

    const status = wrapper.find('[role="status"]')
    expect(status.exists()).toBe(true)
    expect(status.attributes('aria-live')).toBe('polite')

    const icon = wrapper.find('.icon-mock')
    expect(icon.exists()).toBe(true)
    expect(icon.classes()).toContain('animate-spin')
    expect(icon.classes()).toContain('motion-reduce:animate-none')
    expect(icon.attributes('icon')).toBe('mdi:loading')
  })
})
