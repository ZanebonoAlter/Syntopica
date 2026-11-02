/**
 * BoardRadarMasthead 测试（rework-workshop-into-radar-and-desk 8F / FB-13）。
 *
 * 覆盖：报头「主页」入口归位——落在右上角操作区、紧贴「全局治理」左侧，
 * 且与治理菜单同属一个操作按钮档（共享 rd-action-btn 类）；标题区不再挂返回入口。
 * 纯展示组件：无数据依赖，用例只断言结构与可点回主页的契约。
 */
import { describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import BoardRadarMasthead from './BoardRadarMasthead.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const NuxtLinkStub = {
  name: 'NuxtLink',
  props: ['to'],
  template: '<a :href="to"><slot /></a>',
}

function mountMasthead() {
  return mount(BoardRadarMasthead, {
    props: {
      dateText: '2026-10-29',
      headline: '4 个板块值得关注，暂未出现升温',
      sub: 'no-hot' as const,
      hotCount: 0,
      rowsCount: 12,
      warmingCount: 0,
      statsVisible: true,
      creatingBoard: false,
    },
    global: {
      components: { NuxtLink: NuxtLinkStub },
      stubs: { AppButton: { template: '<button><slot /></button>' } },
    },
  })
}

describe('BoardRadarMasthead — 主页入口归位到操作区（8F / FB-13）', () => {
  it('「主页」在右上角操作区内，且紧贴「全局治理」左侧', () => {
    const wrapper = mountMasthead()
    const actions = wrapper.find('.rd-issue-actions')
    const home = actions.find('[data-testid="radar-home"]')
    const governance = actions.find('[data-testid="radar-more"]')

    expect(home.exists()).toBe(true)
    expect(governance.exists()).toBe(true)
    // 紧贴：治理菜单的前一个兄弟元素就是主页入口
    expect(governance.element.previousElementSibling).toBe(home.element)
  })

  it('标题区不再挂返回入口（避免与右侧操作组分得太开）', () => {
    const wrapper = mountMasthead()
    expect(wrapper.find('.rd-heading [data-testid="radar-home"]').exists()).toBe(false)
  })

  it('入口仍回主页：链接指向 /、有可读文案与 aria 标签', () => {
    const wrapper = mountMasthead()
    const home = wrapper.find('[data-testid="radar-home"]')
    expect(home.attributes('href')).toBe('/')
    expect(home.attributes('aria-label')).toBe('返回主页')
    expect(home.text()).toContain('主页')
  })

  it('与「全局治理」同档：两者共用同一个操作按钮类', () => {
    const wrapper = mountMasthead()
    const home = wrapper.find('[data-testid="radar-home"]')
    const governance = wrapper.find('[data-testid="radar-more"] summary')
    expect(home.classes()).toContain('rd-action-btn')
    expect(governance.classes()).toContain('rd-action-btn')
  })

  it('同组成员仍在同一操作区：主页紧邻治理、治理之后是新建版块', () => {
    const wrapper = mountMasthead()
    const actions = wrapper.find('.rd-issue-actions')
    const home = actions.find('[data-testid="radar-home"]')
    const more = actions.find('[data-testid="radar-more"]')
    const cta = actions.find('[data-testid="radar-add-board"]')
    expect(cta.exists()).toBe(true)
    expect(home.element.nextElementSibling).toBe(more.element)
    expect(more.element.nextElementSibling).toBe(cta.element)
  })

  // 样式锚：仓库 eslint 未覆盖 .vue、happy-dom 也不应用 scoped CSS，
  // 模板与样式对 lint/单测天然失明 → 用源码正则锚住「同档」口径与治理菜单标记规则。
  it('样式锚：同档口径与治理菜单标记规则不得被改坏', () => {
    const src = readFileSync(`${import.meta.dirname}/BoardRadarMasthead.vue`, 'utf8')
    // 同档口径（用户 2026-10-30 确认）：min-height 2rem / 0.7rem 字 / border-medium / hover 变 accent
    expect(src).toMatch(/\.rd-action-btn\s*\{[^}]*min-height:\s*2rem/)
    expect(src).toMatch(/\.rd-action-btn\s*\{[^}]*font-size:\s*0\.7rem/)
    expect(src).toMatch(/\.rd-action-btn\s*\{[^}]*border:\s*1px solid var\(--color-border-medium\)/)
    expect(src).toMatch(/\.rd-action-btn:hover[\s\S]*?border-color:\s*var\(--color-accent\)/)
    // 治理菜单：展开标记与指针不得回归（否则复现三角箭头 / 失去手型）
    expect(src).toMatch(/\.rd-more summary\s*\{[^}]*cursor:\s*pointer/)
    expect(src).toMatch(/\.rd-more summary\s*\{[^}]*list-style:\s*none/)
    expect(src).toMatch(/\.rd-more summary::[a-z-]*webkit-details-marker\s*\{[^}]*display:\s*none/)
    expect(src).toMatch(/\.rd-more\[open\] summary\s*\{[^}]*var\(--color-accent\)/)
    // 「主页在治理左边」是用户诉求本身：容器不得改成 row-reverse / 用 order 调换视觉左右
    expect(src).not.toMatch(/\.rd-issue-actions\s*\{[^}]*flex-direction:\s*row-reverse/)
    expect(src).not.toMatch(/\.rd-issue-actions\s*\{[^}]*order:\s*-/)
  })
})
