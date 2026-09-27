import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import TagsPage from './TagsPage.vue'

const apiMocks = vi.hoisted(() => ({
  listWatches: vi.fn(),
}))

// 懒面板 vi.mock 替身（__esModule 标记：defineAsyncComponent 解包动态 import
// 命名空间时据此取 default；testid 供懒加载用例断言渲染归属）

vi.mock('~/api/topicWatches', () => ({
  useTopicWatchesApi: () => ({ listWatches: apiMocks.listWatches }),
}))

vi.mock('~/composables/useOnboarding', () => ({
  useOnboarding: () => ({ isTagsFirstRun: ref(false), startTagsTour: vi.fn() }),
}))

vi.mock('~/features/tags/composables/useTagsPage', () => ({
  useTagsPage: () => new Proxy({
    boards: ref([{ id: 1974, label: '地缘政治' }]),
    selectedBoardId: ref(1974),
    contentTab: ref('composition'),
    boardsLoading: ref(false),
    boardsError: ref(null),
    compositionLabels: ref([]),
    compositionLoading: ref(false),
  }, {
    get: (target, key) => key in target ? target[key as keyof typeof target] : ref(null),
  }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span />' },
}))

// TagsPage 的主题开关依赖 Nuxt `#imports`；本组件测试只验证 tab 栏入口，
// 以轻量 stub 隔离主题运行时。
vi.mock('~/components/ui/ThemeToggle.vue', () => ({
  default: { name: 'ThemeToggle', template: '<span />' },
}))
vi.mock('./BoardThreadBrowser.vue', () => {
  // __esModule 标记：defineAsyncComponent 解包动态 import 命名空间时据此取 default
  return { __esModule: true, default: { name: 'BoardThreadBrowser', template: '<div data-testid="lazy-panel-topic-overview" />' } }
})
vi.mock('./AddSemanticBoardDialog.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./BoardCompositionPanel.vue', () => ({ default: { name: 'BoardCompositionPanel', template: '<div data-testid="eager-panel-composition" />' } }))
vi.mock('./AuxiliaryLabelPool.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./UpgradeSuggestionPanel.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./BackfillProgress.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./MatchingConfigDialog.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./DailyReportGenerateDialog.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./BoardDailyReportTimeline.vue', () => {
  return { __esModule: true, default: { name: 'BoardDailyReportTimeline', template: '<div data-testid="lazy-panel-daily-reports" />' } }
})
vi.mock('./TopicDetectiveWall.client.vue', () => {
  return { __esModule: true, default: { name: 'TopicDetectiveWall', template: '<div data-testid="lazy-panel-detective-wall" />' } }
})
vi.mock('./TagMergePreview.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./BoardListSidebar.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./BoardTimelinePanel.vue', () => {
  return { __esModule: true, default: { name: 'BoardTimelinePanel', template: '<div data-testid="lazy-panel-articles" />' } }
})
vi.mock('./BoardEnrichmentPanel.vue', () => {
  return { __esModule: true, default: { name: 'BoardEnrichmentPanel', template: '<div data-testid="lazy-panel-enrichment" />' } }
})
vi.mock('./BoardEditDialog.vue', () => ({ default: { template: '<span />' } }))
vi.mock('./ArticlePreviewModal.vue', () => ({ default: { template: '<span />' } }))

const WatchManagePanelStub = {
  name: 'WatchManagePanel',
  props: ['modelValue'],
  template: '<div data-testid="watch-manage-panel-stub" :data-open="String(modelValue)" />',
}

describe('TagsPage — 版块级关注入口', () => {
  it('在五个平级内容 tab 下常驻，计数含 active+paused，并可打开管理面板', async () => {
    apiMocks.listWatches.mockResolvedValue({
      success: true,
      data: [
        { id: '1', status: 'active' },
        { id: '2', status: 'paused' },
      ],
    })

    const wrapper = mount(TagsPage, {
      shallow: true,
      global: { stubs: { WatchManagePanel: WatchManagePanelStub } },
    })
    await flushPromises()

    const tabs = wrapper.findAll('.tags-content-tab')
    expect(tabs).toHaveLength(5)
    const chip = wrapper.find('[data-testid="watch-panel-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('我在追踪')
    expect(chip.text()).toContain('(2)')
    expect(chip.classes()).toContain('tags-watch-chip--nowrap')
    expect(chip.attributes('style') || '').not.toContain('white-space: normal')

    for (const tab of tabs) {
      await tab.trigger('click')
      expect(wrapper.find('[data-testid="watch-panel-chip"]').exists()).toBe(true)
    }

    await chip.trigger('click')
    expect(wrapper.find('[data-testid="watch-manage-panel-stub"]').attributes('data-open')).toBe('true')
  })
})

describe('TagsPage — tab 面板懒加载（fix-spa-nav-loading-ux D3）', () => {
  function mountTagsPage() {
    // 非 shallow：懒面板用动态 import + vi.mock 替身渲染，占位/面板真实性依赖完整渲染链。
    // 注：vitest 会在模块图就绪时预执行 vi.mock 工厂，「模块是否加载」无法在此层断言，
    // 懒加载的 chunk 级证据由任务 3.3（pnpm build 产物分 chunk）验收。
    return mount(TagsPage, {
      global: { stubs: { WatchManagePanel: WatchManagePanelStub } },
    })
  }

  it('默认「板块内容」tab 仅渲染默认面板，无任何懒面板渲染', async () => {
    apiMocks.listWatches.mockResolvedValue({ success: true, data: [] })
    const wrapper = mountTagsPage()
    await flushPromises()

    expect(wrapper.find('[data-testid="eager-panel-composition"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lazy-panel-topic-overview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lazy-panel-daily-reports"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lazy-panel-articles"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lazy-panel-enrichment"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lazy-panel-detective-wall"]').exists()).toBe(false)
  })

  it('切到懒 tab 后对应面板按需渲染，其余懒面板仍不渲染', async () => {
    apiMocks.listWatches.mockResolvedValue({ success: true, data: [] })
    const wrapper = mountTagsPage()
    await flushPromises()

    const tabs = wrapper.findAll('.tags-content-tab')
    await tabs[1]!.trigger('click') // 话题总览
    await flushPromises()

    expect(wrapper.find('[data-testid="lazy-panel-topic-overview"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lazy-panel-articles"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lazy-panel-detective-wall"]').exists()).toBe(false)
  })
})
