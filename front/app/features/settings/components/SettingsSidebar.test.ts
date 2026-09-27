import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingsSidebar from './SettingsSidebar.vue'
import type { SectionMeta } from './SettingsWorkspace.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const sections: SectionMeta[] = [
  { key: 'feeds', group: '内容管理', label: '订阅源', description: '', icon: 'mdi:rss', component: { name: 'FeedsStub' } },
  { key: 'preferences', group: '内容管理', label: '兴趣画像', description: '', icon: 'mdi:account-heart-outline', component: { name: 'PreferencesStub' } },
  { key: 'margin-notes', group: '内容管理', label: '页边注', description: '', icon: 'mdi:notebook-outline', component: { name: 'MarginNotesStub' } },
  { key: 'ai-providers', group: 'AI 配置', label: 'AI 模型', description: '', icon: 'mdi:brain', component: { name: 'AiProvidersStub' } },
  { key: 'capability-routes', group: 'AI 配置', label: '能力路由', description: '', icon: 'mdi:routes', component: { name: 'CapabilityRoutesStub' } },
  { key: 'datasources-network', group: '数据源与网络', label: '数据源与网络', description: '', icon: 'mdi:database-network-outline', component: { name: 'DatasourcesNetworkStub' } },
  { key: 'runtime-status', group: '运行状态', label: '运行状态', description: '', icon: 'mdi:monitor-dashboard', component: { name: 'RuntimeStatusStub' } },
]

const GROUP_TITLES = ['内容管理', 'AI 配置', '数据源与网络', '运行状态']

function mountSidebar(mobileOpen = false) {
  return mount(SettingsSidebar, {
    props: { sections, activeSection: 'feeds', mobileOpen },
    global: { stubs: { teleport: true } },
  })
}

describe('SettingsSidebar（分组导航渲染）', () => {
  it('渲染 4 个分组标题与全部 7 个导航项', () => {
    const wrapper = mountSidebar()

    const titles = wrapper.findAll('.settings-sidebar__group-title')
    expect(titles).toHaveLength(4)
    for (const title of GROUP_TITLES) {
      expect(titles.map(t => t.text())).toContain(title)
    }

    const items = wrapper.findAll('.settings-sidebar__item')
    expect(items).toHaveLength(7)
    // 每个导航项带 data-onboarding 锚（onboarding tour 依赖）
    expect(wrapper.find('[data-onboarding="settings-nav"]').exists()).toBe(true)
    expect(wrapper.find('[data-onboarding="settings-nav-runtime-status"]').exists()).toBe(true)
  })

  it('分组标题不可点击（纯展示元素，非 button）', () => {
    const wrapper = mountSidebar()
    const title = wrapper.find('.settings-sidebar__group-title')
    expect(title.element.tagName).toBe('P')
    expect(title.attributes('aria-hidden')).toBe('true')
    // 点击组标题不触发 select
    const emitted = () => wrapper.emitted('select')
    title.trigger('click')
    expect(emitted()).toBeUndefined()
  })

  it('点导航项发出 select 事件并带 section 键', async () => {
    const wrapper = mountSidebar()
    await wrapper.find('[data-onboarding="settings-nav-datasources-network"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['datasources-network']])
  })

  it('active 态只落在当前 section', () => {
    const wrapper = mountSidebar()
    const activeItems = wrapper.findAll('.settings-sidebar__item--active')
    expect(activeItems).toHaveLength(1)
    expect(activeItems[0]!.text()).toContain('订阅源')
  })

  it('抽屉模式分组结构同构（mobileOpen 时抽屉内同样 4 组 7 项）', () => {
    const wrapper = mountSidebar(true)
    const drawer = wrapper.find('.settings-sidebar-drawer')
    expect(drawer.exists()).toBe(true)
    expect(drawer.findAll('.settings-sidebar__group-title')).toHaveLength(4)
    expect(drawer.findAll('.settings-sidebar__item')).toHaveLength(7)
    // 桌面侧栏与抽屉并存时桌面侧栏仍渲染（媒体查询控制显示）
    expect(wrapper.findAll('.settings-sidebar .settings-sidebar__item')).toHaveLength(7)
  })
})
