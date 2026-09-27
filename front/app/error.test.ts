import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { mount } from '@vue/test-utils'
import ErrorPage from './error.vue'

/**
 * 全局错误兜底页（spa-loading-ux specs）：
 * - chunk 加载失败 → 网络提示文案 + 重新加载入口
 * - 应用内错误 → 展示错误信息，重试走 clearError 回首页
 * 结构契约（居中 fallback / 复用 AppButton）见 ui-design.md，此处锁行为。
 */

const clearErrorMock = vi.fn()

function setError(value: { statusCode?: number, message?: string } | null) {
  // Nuxt useError 返回共享 Ref；测试里每次 mount 前注入当前错误
  ;(globalThis as Record<string, unknown>).useError = () => ref(value)
  ;(globalThis as Record<string, unknown>).clearError = clearErrorMock
}

function mountError() {
  return mount(ErrorPage, {
    global: {
      // 纯 vue 环境无 Nuxt 自动注册：显式注入两个依赖。
      // Icon 必须 stub——本地 iconify 集合未注册时会向 api.iconify.design
      // 发请求，无外网环境下挂起直至超时。
      components: {
        AppButton: { template: '<button data-test="retry"><slot /></button>' },
        Icon: { template: '<i data-test="icon" />' },
      },
    },
  })
}

afterEach(() => {
  clearErrorMock.mockClear()
})

describe('全局错误兜底页', () => {
  it('chunk 加载失败：显示网络提示文案与重新加载入口', () => {
    setError({ statusCode: undefined, message: 'Failed to fetch dynamically imported module: /_nuxt/pages-tags.js' })
    const wrapper = mountError()
    expect(wrapper.text()).toContain('网络不稳定，资源加载失败')
    expect(wrapper.find('[data-test="retry"]').text()).toBe('重新加载')
  })

  it('应用内错误：展示错误信息，点击重试调用 clearError 回首页', async () => {
    setError({ statusCode: 500, message: '自定义运行时错误' })
    const wrapper = mountError()
    expect(wrapper.text()).toContain('自定义运行时错误')
    await wrapper.find('[data-test="retry"]').trigger('click')
    expect(clearErrorMock).toHaveBeenCalledWith({ redirect: '/' })
  })

  it('chunk 错误点击重试刷新页面（走首屏加载拉新 chunk）', () => {
    setError({ statusCode: undefined, message: 'Failed to fetch dynamically imported module: /_nuxt/x.js' })
    const wrapper = mountError()
    const reload = vi.fn()
    Object.defineProperty(window, 'location', { value: { reload }, writable: true })
    wrapper.find('[data-test="retry"]').trigger('click')
    expect(reload).toHaveBeenCalled()
    expect(clearErrorMock).not.toHaveBeenCalled()
  })

  it('无消息错误回退未知错误文案', () => {
    setError({ statusCode: 500 })
    const wrapper = mountError()
    expect(wrapper.text()).toContain('发生未知错误')
  })
})
