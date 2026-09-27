import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, onBeforeMount, type Ref } from 'vue'

import {
  NARROW_VIEWPORT_QUERY,
  useIsNarrowViewport,
  useMediaQuery,
  __mediaQueryClient,
} from './useMediaQuery'

/**
 * useMediaQuery 单测（mobile-viewport-stage1 任务 1.1）：
 * - 断点边界：767px 命中 / 768px 不命中（<768 窄屏，design.md D1）
 * - change 监听：mounted 注册、unmount 清理、跨断点翻转
 * - SSR 默认值：守卫关闭时恒 false 且不触 matchMedia；真实值挂载后才写入
 *
 * happy-dom 不具备可求值的 matchMedia（不解析媒体查询、不发 change），这里替换为
 * 可控桩：按当前视口宽对 `(max-width: Npx)` 真实求值并播发 change——断点常量因此
 * 被真实走到，而非比对字符串了事。
 */

type ChangeListener = (event: MediaQueryListEvent) => void

type MockMediaQueryList = {
  media: string
  readonly matches: boolean
  addEventListener: (type: string, listener: ChangeListener) => void
  removeEventListener: (type: string, listener: ChangeListener) => void
  /** 测试断言用：当前注册的 change 监听器集合 */
  listeners: Set<ChangeListener>
}

function createMatchMediaStub() {
  let viewportWidth = 1280

  /** 按当前视口宽求值（只支持本仓库用到的 max-width 形态，其余一律 false）。 */
  function evaluate(query: string): boolean {
    const hit = /^\(max-width:\s*([\d.]+)px\)$/.exec(query)
    return hit !== null && viewportWidth <= Number.parseFloat(hit[1] ?? '')
  }

  const created: MockMediaQueryList[] = []

  const matchMedia = vi.fn((query: string): MockMediaQueryList => {
    const listeners = new Set<ChangeListener>()
    const list: MockMediaQueryList = {
      media: query,
      get matches() {
        return evaluate(query)
      },
      addEventListener(type, listener) {
        if (type === 'change') listeners.add(listener)
      },
      removeEventListener(type, listener) {
        if (type === 'change') listeners.delete(listener)
      },
      listeners,
    }
    created.push(list)
    return list
  })

  /** 模拟视口宽度变化：更新求值基准，并向所有存活监听器播发 change。 */
  function resizeTo(width: number): void {
    viewportWidth = width
    for (const list of created) {
      for (const listener of [...list.listeners]) {
        listener(new MediaQueryListEvent('change', { matches: list.matches, media: list.media }))
      }
    }
  }

  return { matchMedia, resizeTo, created }
}

type MatchMediaStub = ReturnType<typeof createMatchMediaStub>

/** 挂载一个消费 composable 的宿主组件（生命周期钩子需组件实例才有意义）。 */
function mountProbe(use: () => Ref<boolean>) {
  let matches!: Ref<boolean>
  const Probe = defineComponent({
    setup() {
      matches = use()
      return () => h('div')
    },
  })
  const wrapper = mount(Probe)
  return { wrapper, matches }
}

describe('useMediaQuery', () => {
  let stub: MatchMediaStub
  const originalMatchMedia = window.matchMedia

  beforeEach(() => {
    // Vitest 下 import.meta.client 解析为 undefined（视为 server），客户端用例显式开启
    __mediaQueryClient.value = true
    stub = createMatchMediaStub()
    window.matchMedia = stub.matchMedia as unknown as typeof window.matchMedia
  })

  afterEach(() => {
    window.matchMedia = originalMatchMedia
    __mediaQueryClient.value = false
  })

  describe('断点边界（useIsNarrowViewport，<768px）', () => {
    it('767px 视口命中窄屏断点', () => {
      stub.resizeTo(767)
      const { matches } = mountProbe(() => useIsNarrowViewport())
      expect(matches.value).toBe(true)
    })

    it('768px 视口不命中（宽屏零变化边界）', () => {
      stub.resizeTo(768)
      const { matches } = mountProbe(() => useIsNarrowViewport())
      expect(matches.value).toBe(false)
    })

    it('以约定的断点查询串调用 matchMedia', () => {
      mountProbe(() => useIsNarrowViewport())
      expect(stub.matchMedia).toHaveBeenCalledWith(NARROW_VIEWPORT_QUERY)
    })
  })

  describe('change 监听', () => {
    it('跨断点翻转：767 → 768 → 767', () => {
      stub.resizeTo(767)
      const { matches } = mountProbe(() => useIsNarrowViewport())
      expect(matches.value).toBe(true)

      stub.resizeTo(768)
      expect(matches.value).toBe(false)

      stub.resizeTo(767)
      expect(matches.value).toBe(true)
    })

    it('自定义查询同样随 change 更新', () => {
      const { matches } = mountProbe(() => useMediaQuery('(max-width: 500px)'))
      expect(matches.value).toBe(false) // 初始视口 1280

      stub.resizeTo(400)
      expect(matches.value).toBe(true)

      stub.resizeTo(600)
      expect(matches.value).toBe(false)
    })

    it('mounted 注册 change 监听，unmount 移除且引用不再更新', () => {
      stub.resizeTo(767)
      const { wrapper, matches } = mountProbe(() => useIsNarrowViewport())
      const list = stub.created[0]!
      expect(list.listeners.size).toBe(1)

      wrapper.unmount()
      expect(list.listeners.size).toBe(0)

      stub.resizeTo(768)
      expect(matches.value).toBe(true) // 卸载后保持最后已知值
    })
  })

  describe('挂载时序（真实值仅 mounted 后写入）', () => {
    it('beforeMount 时仍为默认 false 且未触 matchMedia', () => {
      stub.resizeTo(375)
      let matches!: Ref<boolean>
      let beforeMountMatches: boolean | null = null
      let beforeMountCalls = 0

      const Probe = defineComponent({
        setup() {
          matches = useIsNarrowViewport()
          onBeforeMount(() => {
            beforeMountMatches = matches.value
            beforeMountCalls = stub.matchMedia.mock.calls.length
          })
          return () => h('div')
        },
      })

      mount(Probe)
      expect(beforeMountMatches).toBe(false)
      expect(beforeMountCalls).toBe(0)
      expect(matches.value).toBe(true) // mounted 后已置真实值
    })
  })

  describe('SSR（__mediaQueryClient = false）', () => {
    it('useMediaQuery 返回默认 false 的 ref，不触 matchMedia，也无需组件实例', () => {
      __mediaQueryClient.value = false
      const matches = useMediaQuery(NARROW_VIEWPORT_QUERY)
      expect(matches.value).toBe(false)
      expect(stub.matchMedia).not.toHaveBeenCalled()
    })

    it('useIsNarrowViewport 同样短路', () => {
      __mediaQueryClient.value = false
      const isNarrow = useIsNarrowViewport()
      expect(isNarrow.value).toBe(false)
      expect(stub.matchMedia).not.toHaveBeenCalled()
    })
  })
})
