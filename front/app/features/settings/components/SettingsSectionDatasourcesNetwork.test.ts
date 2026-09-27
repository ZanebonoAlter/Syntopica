import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import SettingsSectionDatasourcesNetwork from './SettingsSectionDatasourcesNetwork.vue'

/**
 * 复合 section「数据源与网络」（settings-workspace spec：复合 section 子 tab 切换与 URL 承载）。
 * 子面板全部 stub 隔离（不拉真实 API）；routeQuery/replaceMock 驱动 URL tab 参数行为。
 */

const routeQuery = reactive<Record<string, string>>({})
const replaceMock = vi.fn((to: { query?: Record<string, unknown> }) => {
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  if (to.query) {
    for (const [k, v] of Object.entries(to.query)) {
      if (v !== undefined) routeQuery[k] = String(v)
    }
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ replace: replaceMock }),
}))

vi.mock('~/components/dialog/FirecrawlConfigPanel.vue', () => ({
  default: { name: 'FirecrawlStub', template: '<div data-testid="panel-firecrawl" />' },
}))
vi.mock('~/components/dialog/BochaConfigPanel.vue', () => ({
  default: { name: 'BochaStub', template: '<div data-testid="panel-bocha" />' },
}))
vi.mock('~/components/dialog/SearxngConfigPanel.vue', () => ({
  default: { name: 'SearxngStub', template: '<div data-testid="panel-searxng" />' },
}))
vi.mock('./SettingsSectionRsshub.vue', () => ({
  default: { name: 'RsshubStub', template: '<div data-testid="panel-rsshub" />' },
}))
vi.mock('./SettingsSectionDatasources.vue', () => ({
  default: { name: 'DatasourcesStub', template: '<div data-testid="panel-datasources" />' },
}))
vi.mock('./SettingsSectionProxy.vue', () => ({
  default: { name: 'ProxyStub', template: '<div data-testid="panel-proxy" />' },
}))

const SUB_TABS = ['firecrawl', 'bocha', 'searxng', 'rsshub', 'datasources', 'proxy']

function mountSection() {
  return mount(SettingsSectionDatasourcesNetwork)
}

beforeEach(() => {
  replaceMock.mockClear()
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  routeQuery.section = 'datasources-network'
})

describe('SettingsSectionDatasourcesNetwork（复合 section 子 tab）', () => {
  it('渲染 6 个子 tab，默认落 Firecrawl', () => {
    const wrapper = mountSection()
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map(t => t.text())).toEqual(['Firecrawl', '博查', 'SearXNG', 'RSSHub', '研究数据源', '出站代理'])
    expect(tabs[0]!.attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="panel-firecrawl"]').exists()).toBe(true)
  })

  it('无 tab 参数时不发 replace（默认即合法态）', () => {
    mountSection()
    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('切到出站代理：router.replace 携带 tab=proxy，面板切换', async () => {
    const wrapper = mountSection()
    const proxyTab = wrapper.findAll('[role="tab"]').find(t => t.text() === '出站代理')!
    await proxyTab.trigger('click')

    expect(replaceMock).toHaveBeenCalledWith({ query: { section: 'datasources-network', tab: 'proxy' } })
    expect(routeQuery.tab).toBe('proxy')
    await nextTick()
    expect(wrapper.find('[data-testid="panel-proxy"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="panel-firecrawl"]').exists()).toBe(false)
  })

  it('URL tab 参数驱动初始子 tab（?tab=searxng 直达 SearXNG）', () => {
    routeQuery.tab = 'searxng'
    const wrapper = mountSection()
    expect(wrapper.find('[data-testid="panel-searxng"]').exists()).toBe(true)
  })

  it('非法 tab 值回默认并清参数（回退到 Firecrawl）', async () => {
    routeQuery.tab = '不存在值'
    const wrapper = mountSection()
    await nextTick()

    expect(replaceMock).toHaveBeenCalledWith({ query: { section: 'datasources-network', tab: undefined } })
    expect(wrapper.find('[data-testid="panel-firecrawl"]').exists()).toBe(true)
  })

  it('空 tab 参数（tab=）同样回默认并清参数', async () => {
    routeQuery.tab = ''
    const wrapper = mountSection()
    await nextTick()

    expect(replaceMock).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="panel-firecrawl"]').exists()).toBe(true)
  })

  it('子 tab 键沿用旧 section 键（旧键重定向目标合法）', () => {
    // settings-workspace spec：旧键深链重定向 → tab=<旧键>；容器必须接受全部 6 旧键
    for (const legacyKey of SUB_TABS) {
      for (const k of Object.keys(routeQuery)) delete routeQuery[k]
      routeQuery.section = 'datasources-network'
      routeQuery.tab = legacyKey
      const wrapper = mountSection()
      const selected = wrapper.findAll('[role="tab"]').find(t => t.attributes('aria-selected') === 'true')
      expect(selected, `旧键 ${legacyKey} 应命中子 tab`).toBeDefined()
      wrapper.unmount()
    }
  })

  it('tab 条 sticky 贴顶且横向滚动（形态断言：role=tablist 容器类存在）', () => {
    const wrapper = mountSection()
    expect(wrapper.find('.settings-tabs-nav').exists()).toBe(true)
    expect(wrapper.find('.settings-tabs-nav__track[role="tablist"]').exists()).toBe(true)
  })
})
