import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppPageShell from './AppPageShell.vue'
import { effectiveContentWidth } from './layout-contract'

/**
 * 布局契约单元锚点（standard/frontend/layout.md）：
 * - 视觉/构图行为在 major UI change 验收时由 opencli + 视觉子代理按 1440×900/1920×1080 检查，
 *   这里只锁结构契约（模式 class/CSS var/语义标签）与宽度数值边界。
 */

function shellVars(wrapper: ReturnType<typeof mount>): Record<string, string> {
  const el = wrapper.find('.app-page-shell').element as HTMLElement
  return {
    max: el.style.getPropertyValue('--shell-max'),
    gutter: el.style.getPropertyValue('--shell-gutter'),
    asideMin: el.style.getPropertyValue('--aside-min'),
  }
}

describe('AppPageShell 四模式', () => {
  it.each([
    ['reader', '760px'],
    ['contained', '1120px'],
    ['workspace', 'none'],
    ['split', 'none'],
  ] as const)('%s 模式：data-shell-mode + --shell-max=%s', (mode, max) => {
    const wrapper = mount(AppPageShell, { props: { mode } })
    expect(wrapper.find('.app-page-shell').attributes('data-shell-mode')).toBe(mode)
    expect(shellVars(wrapper).max).toBe(max)
    expect(wrapper.find('.app-page-shell__inner').classes()).toContain(`is-${mode}`)
  })

  it('默认 mode=contained', () => {
    const wrapper = mount(AppPageShell)
    expect(wrapper.find('.app-page-shell').attributes('data-shell-mode')).toBe('contained')
    expect(shellVars(wrapper).max).toBe('1120px')
  })

  it('gutter 透传为 --shell-gutter', () => {
    const wrapper = mount(AppPageShell, { props: { gutter: 32 } })
    expect(shellVars(wrapper).gutter).toBe('32px')
  })
})

describe('AppPageShell 可访问 DOM 语义', () => {
  it('主内容默认渲染为 <main>，as 可覆盖', () => {
    const wrapper = mount(AppPageShell)
    expect(wrapper.find('main.app-page-shell__main').exists()).toBe(true)
    const asDiv = mount(AppPageShell, { props: { as: 'div' } })
    expect(asDiv.find('div.app-page-shell__main').exists()).toBe(true)
    expect(asDiv.find('main').exists()).toBe(false)
  })

  it('split + aside 插槽渲染 <aside> 侧栏并携带栏宽契约', () => {
    const wrapper = mount(AppPageShell, {
      props: { mode: 'split', asideMin: '320px', asideSide: 'left' },
      slots: { default: '<p>main</p>', aside: '<nav>list</nav>' },
    })
    const aside = wrapper.find('aside.app-page-shell__aside')
    expect(aside.exists()).toBe(true)
    expect(aside.text()).toContain('list')
    expect(shellVars(wrapper).asideMin).toBe('320px')
    expect(wrapper.find('.app-page-shell__inner').classes()).toContain('aside-left')
    // 溢出契约：主栏可收缩（min-width:0 类名锚点），侧栏自身滚动
    expect(wrapper.find('.app-page-shell__main').exists()).toBe(true)
    expect(aside.classes()).toContain('app-page-shell__aside')
  })

  it('split 无 aside 插槽时不渲染空侧栏', () => {
    const wrapper = mount(AppPageShell, { props: { mode: 'split' }, slots: { default: '<p>x</p>' } })
    expect(wrapper.find('aside').exists()).toBe(false)
  })
})

describe('布局宽度边界值（effectiveContentWidth，gutter=24）', () => {
  it('治理列表选择 contained：宽屏（1440/1920）不超过 1120 且小屏退化为 100%−gutter', () => {
    expect(effectiveContentWidth(1440, 'contained')).toBe(1120)
    expect(effectiveContentWidth(1920, 'contained')).toBe(1120)
    expect(effectiveContentWidth(1000, 'contained')).toBe(1000 - 48)
  })

  it('工作台选择 workspace 或 split：使用可用宽度不套上限', () => {
    expect(effectiveContentWidth(1920, 'workspace')).toBe(1920 - 48)
    expect(effectiveContentWidth(1280, 'split')).toBe(1280 - 48)
  })

  it('reader 窄于/等于/宽于 760 的边界', () => {
    expect(effectiveContentWidth(500, 'reader')).toBe(500 - 48) // 窄于上限：100%−gutter
    expect(effectiveContentWidth(808, 'reader')).toBe(760) // 视口=上限+2gutter：恰好 760
    expect(effectiveContentWidth(1920, 'reader')).toBe(760)
  })
})
