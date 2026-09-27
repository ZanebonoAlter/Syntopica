import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import SettingsWorkspace from './SettingsWorkspace.vue'

/**
 * SettingsWorkspace 旧键深链重定向（settings-workspace spec：旧 section 键深链重定向 / 非法 tab 值回退）。
 * 9 旧键（6 数据源类 + 3 运行状态类）命中即 router.replace 到复合新键携带 tab=<旧键>。
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

vi.mock('~/composables/useTheme', () => ({
  useTheme: () => ({ toggleTheme: vi.fn(), isDark: { value: false } }),
}))
vi.mock('~/composables/useOnboarding', () => ({
  useOnboarding: () => ({ startSettingsTour: vi.fn() }),
}))

// 全部 section 实体 stub（本测试只关心导航/重定向，不渲染面板内容）
vi.mock('./SettingsSectionFeeds.vue', () => ({ default: { name: 'FeedsStub', template: '<div data-testid="sec-feeds" />' } }))
vi.mock('./SettingsSectionPreferences.vue', () => ({ default: { name: 'PreferencesStub', template: '<div />' } }))
vi.mock('./SettingsSectionMarginNotes.vue', () => ({ default: { name: 'MarginNotesStub', template: '<div />' } }))
vi.mock('~/features/ai/components/AIProviderManagement.vue', () => ({ default: { name: 'AiProvidersStub', template: '<div />' } }))
vi.mock('./SettingsSectionCapabilityRoutes.vue', () => ({ default: { name: 'CapabilityRoutesStub', template: '<div />' } }))
vi.mock('./SettingsSectionDatasourcesNetwork.vue', () => ({ default: { name: 'DatasourcesNetworkStub', template: '<div data-testid="sec-datasources-network" />' } }))
vi.mock('./SettingsSectionRuntimeStatus.vue', () => ({ default: { name: 'RuntimeStatusStub', template: '<div data-testid="sec-runtime-status" />' } }))

function mountWorkspace() {
  return mount(SettingsWorkspace, {
    global: { stubs: { teleport: true } },
  })
}

const LEGACY_CASES: [string, string, string][] = [
  ['firecrawl', 'datasources-network', 'firecrawl'],
  ['bocha', 'datasources-network', 'bocha'],
  ['searxng', 'datasources-network', 'searxng'],
  ['rsshub', 'datasources-network', 'rsshub'],
  ['datasources', 'datasources-network', 'datasources'],
  ['proxy', 'datasources-network', 'proxy'],
  ['ai-health', 'runtime-status', 'ai-health'],
  ['queues', 'runtime-status', 'queues'],
  ['schedulers', 'runtime-status', 'schedulers'],
]

beforeEach(() => {
  replaceMock.mockClear()
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
})

describe('SettingsWorkspace — 旧键深链重定向', () => {
  for (const [legacyKey, newSection, tab] of LEGACY_CASES) {
    it(`?section=${legacyKey} → ${newSection}&tab=${tab}`, () => {
      routeQuery.section = legacyKey
      mountWorkspace()
      expect(replaceMock).toHaveBeenCalledWith({ query: { section: newSection, tab } })
    })
  }

  it('保留键（feeds 等 5 键）不触发重定向', () => {
    for (const key of ['feeds', 'preferences', 'margin-notes', 'ai-providers', 'capability-routes']) {
      for (const k of Object.keys(routeQuery)) delete routeQuery[k]
      replaceMock.mockClear()
      routeQuery.section = key
      mountWorkspace()
      expect(replaceMock, `保留键 ${key} 不应重定向`).not.toHaveBeenCalled()
    }
  })

  it('新复合键本身不重定向（幂等：重定向落点稳定）', () => {
    routeQuery.section = 'datasources-network'
    routeQuery.tab = 'proxy'
    mountWorkspace()
    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('未知键（含大小写不匹配 PROXY）不重定向，回退默认 section', () => {
    routeQuery.section = 'PROXY'
    const wrapper = mountWorkspace()
    expect(replaceMock).not.toHaveBeenCalled()
    // 回退默认：内容区渲染 feeds
    expect(wrapper.find('[data-testid="sec-feeds"]').exists()).toBe(true)
  })

  it('重定向保留其余 query 参数（如 ?section=proxy&foo=bar）', () => {
    routeQuery.section = 'proxy'
    routeQuery.foo = 'bar'
    mountWorkspace()
    expect(replaceMock).toHaveBeenCalledWith({ query: { section: 'datasources-network', tab: 'proxy', foo: 'bar' } })
  })

  it('无 section 参数时默认 feeds，不重定向', () => {
    const wrapper = mountWorkspace()
    expect(replaceMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="sec-feeds"]').exists()).toBe(true)
  })
})
