import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

/**
 * 导航加载反馈浮层（fix-spa-nav-loading-ux specs「路由切换加载反馈」）：
 * - visible 时渲染居中 spinner（role=status + aria-live=polite 播报）
 * - contains motion-reduce:animate-none（prefers-reduced-motion 下停用旋转）
 * - 不可见时渲染空（快导航 / 导航结束不残留）
 *
 * 组件内 useNavLoading 走 Nuxt auto-import（无显式 import，vi.mock 拦不到），
 * 按 vitest.setup.ts 同款手法 stub 全局；真实状态机行为由 useNavLoading.test.ts 覆盖。
 */

const navState = { visible: ref(false) }
;(globalThis as Record<string, unknown>).useNavLoading = () => ({
  visible: navState.visible,
  begin: vi.fn(),
  end: vi.fn(),
})

vi.mock('@iconify/vue', () => ({
  Icon: {
    name: 'Icon',
    // 不声明 props：icon 等 attrs 透传到 span，便于断言（仓库既有 Icon mock 手法）
    inheritAttrs: true,
    template: '<span class="icon-mock" />',
  },
}))

import NavLoadingOverlay from './NavLoadingOverlay.vue'

describe('NavLoadingOverlay', () => {
  it('visible=true 渲染 spinner：role=status + aria-live=polite + reduced-motion 降级类', () => {
    navState.visible.value = true
    const wrapper = mount(NavLoadingOverlay)

    const status = wrapper.find('[role="status"]')
    expect(status.exists()).toBe(true)
    expect(status.attributes('aria-live')).toBe('polite')

    const icon = wrapper.find('.icon-mock')
    expect(icon.exists()).toBe(true)
    expect(icon.classes()).toContain('animate-spin')
    expect(icon.classes()).toContain('motion-reduce:animate-none')
    expect(icon.attributes('icon')).toBe('mdi:loading')
  })

  it('visible=false 渲染空（不残留反馈节点）', () => {
    navState.visible.value = false
    const wrapper = mount(NavLoadingOverlay)

    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.find('.icon-mock').exists()).toBe(false)
  })
})
