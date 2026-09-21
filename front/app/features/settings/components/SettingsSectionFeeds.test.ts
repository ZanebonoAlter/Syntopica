import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

// useGlobalSettings / useApiStore 在组件里是显式 import，mock 其模块即可。
// 返回值给真 ref（模板依赖顶层自动解包），其余方法给 vi.fn()。
// feedsByCategoryRef 经 hoisted 容器暴露：mock 工厂首次执行时创建，
// 深链测试 mount 后往里填数据触发 watch。
const mocks = vi.hoisted(() => ({
  feedsByCategoryRef: undefined as unknown as ReturnType<typeof ref<Record<string, unknown[]>>>,
  routeQueryRef: undefined as unknown as ReturnType<typeof ref<Record<string, string>>>,
}))
vi.mock('~/composables/useGlobalSettings', async () => {
  const { ref } = await import('vue')
  mocks.feedsByCategoryRef = ref<Record<string, unknown[]>>({})
  return {
    useGlobalSettings: () => ({
      collapsedCategories: ref({}),
      loading: ref(false),
      error: ref<string | null>(null),
      success: ref<string | null>(null),
      feedsByCategory: mocks.feedsByCategoryRef,
      categories: ref([]),
      refreshOptions: [],
      maxArticlesOptions: [],
      updateFeedSetting: vi.fn(),
      refreshFeed: vi.fn(),
      createCategoryAndAssign: vi.fn(),
      deleteFeed: vi.fn(),
    }),
  }
})

const fetchFeedsMock = vi.fn().mockResolvedValue(undefined)
vi.mock('~/stores/api', () => ({
  useApiStore: () => ({
    fetchFeeds: fetchFeedsMock,
    exportOpml: vi.fn(),
  }),
}))

// 深链测试需要 useRoute 返回可控 query；默认无 feed 参数（mock 工厂首次执行时初始化）
vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  const { ref } = await import('vue')
  mocks.routeQueryRef = ref<Record<string, string>>({})
  return {
    ...actual,
    useRoute: () => ({ query: mocks.routeQueryRef.value }),
  }
})

import SettingsSectionFeeds from './SettingsSectionFeeds.vue'

function mountSection(options: { attach?: boolean } = {}) {
  return mount(SettingsSectionFeeds, {
    attachTo: options.attach ? document.body : undefined,
    global: {
      stubs: {
        Icon: { template: '<i />' },
        FeedMasterList: { template: '<div class="stub-feed-master-list"><div class="feed-master__item--active" /></div>' },
        FeedDetailEditor: { template: '<div class="stub-feed-detail-editor" />' },
        AddFeedDialog: { template: '<div class="stub-add-feed-dialog" />' },
        AddCategoryDialog: { template: '<div class="stub-add-category-dialog" />' },
        ImportOpmlDialog: { template: '<div class="stub-import-opml-dialog" />' },
      },
    },
  })
}

describe('SettingsSectionFeeds 订阅源管理工具条', () => {
  beforeEach(() => {
    fetchFeedsMock.mockClear()
    mocks.feedsByCategoryRef.value = {}
    mocks.routeQueryRef.value = {}
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('渲染 4 个管理入口：添加订阅源 / 添加分类 / 导入 / 导出', () => {
    const wrapper = mountSection()
    const buttons = wrapper.findAll('.feeds-toolbar__btn')
    expect(buttons).toHaveLength(4)
    const labels = buttons.map(b => b.text())
    expect(labels).toContain('添加订阅源')
    expect(labels).toContain('添加分类')
    expect(labels).toContain('导入')
    expect(labels).toContain('导出')
  })

  it('点击「添加订阅源」打开 AddFeedDialog', async () => {
    const wrapper = mountSection()
    expect(wrapper.find('.stub-add-feed-dialog').exists()).toBe(false)

    const btn = wrapper
      .findAll('.feeds-toolbar__btn')
      .find(b => b.text().includes('添加订阅源'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')

    expect(wrapper.find('.stub-add-feed-dialog').exists()).toBe(true)
  })

  it('点击「添加分类」打开 AddCategoryDialog', async () => {
    const wrapper = mountSection()
    const btn = wrapper
      .findAll('.feeds-toolbar__btn')
      .find(b => b.text().includes('添加分类'))
    await btn!.trigger('click')
    expect(wrapper.find('.stub-add-category-dialog').exists()).toBe(true)
  })

  it('点击「导入」打开 ImportOpmlDialog', async () => {
    const wrapper = mountSection()
    const btn = wrapper
      .findAll('.feeds-toolbar__btn')
      .find(b => b.text().includes('导入'))
    await btn!.trigger('click')
    expect(wrapper.find('.stub-import-opml-dialog').exists()).toBe(true)
  })
})

describe('SettingsSectionFeeds 深链定位（unify-feed-summary-toggles DL-1/DL-2）', () => {
  beforeEach(() => {
    fetchFeedsMock.mockClear()
    mocks.feedsByCategoryRef.value = {}
    mocks.routeQueryRef.value = {}
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('DL-1: ?feed=<存在id> 列表就绪后选中定位并滚动', async () => {
    mocks.routeQueryRef.value = { feed: 'f2', section: 'feeds' }
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView

    const wrapper = mountSection({ attach: true })
    // 列表尚未就绪：默认空态
    expect(wrapper.find('.feeds-detail__empty').exists()).toBe(true)

    // feeds 就绪后触发深链 watch（FeedDetailEditor 被 stub，选中后由空态切换为编辑器 stub）
    mocks.feedsByCategoryRef.value = {
      分类A: [
        { id: 'f1', title: 'F1', url: 'https://example.com/1', category: '', description: '' },
        { id: 'f2', title: 'F2', url: 'https://example.com/2', category: '', description: '' },
      ],
    }
    await vi.dynamicImportSettled()
    await new Promise(r => setTimeout(r, 0))

    expect(wrapper.find('.stub-feed-detail-editor').exists()).toBe(true)
    expect(scrollIntoView).toHaveBeenCalled()
  })

  it('DL-2: ?feed=<不存在id> 优雅降级到默认视图，不报错', async () => {
    mocks.routeQueryRef.value = { feed: 'gone', section: 'feeds' }
    Element.prototype.scrollIntoView = vi.fn()

    const wrapper = mountSection()
    mocks.feedsByCategoryRef.value = {
      分类A: [{ id: 'f1', title: 'F1', url: 'https://example.com/1', category: '', description: '' }],
    }
    await vi.dynamicImportSettled()
    await new Promise(r => setTimeout(r, 0))

    // 目标不存在：落默认视图（无选中 → 空态），不报错
    expect(wrapper.find('.feeds-detail__empty').exists()).toBe(true)
    expect(wrapper.find('.stub-feed-detail-editor').exists()).toBe(false)
  })
})
