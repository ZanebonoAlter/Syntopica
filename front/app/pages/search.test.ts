import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { Article, SearchHitReason, SearchResponseData } from '~/types'
import searchPageSource from './search.vue?raw'

/** navigateTo 是 Nuxt 自动导入全局（AppHeaderView 同款用法）；单测里 stubGlobal 后裸标识符才可解析 */
const navigateToSpy = vi.fn()

/**
 * /search 页状态矩阵与交互（add-article-search test-cases 故事 S3 组件层落点）：
 * 初始空态/首查骨架屏/空态引导扩展一键重查/错误条重试保留输入/语义降级提示条/
 * 结果分区（命中 chip + 归档角标 + 扩展分隔条）/预览弹窗打开与关闭回原位。
 * 幂等与分页细节在 useSearch.test.ts（页面只断言透传）。
 */

const api = vi.hoisted(() => ({
  search: vi.fn(),
  getArticle: vi.fn(),
  updateArticle: vi.fn(),
}))

vi.mock('~/api/search', () => ({
  useSearchApi: () => ({ search: api.search }),
}))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticle: api.getArticle, updateArticle: api.updateArticle }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// ThemeToggle → useTheme → #imports.useHead（vitest stub 未导出）：页面测试替换为轻桩
vi.mock('~/components/ui/ThemeToggle.vue', () => ({
  default: { name: 'ThemeToggle', template: '<button data-test="theme-toggle-stub" />' },
}))

// articles/public 部分替换：ArticleCardView 用真实实现（断言命中 chip/归档角标/卡片点击），
// ArticleContentView 换轻桩回显正文文本（供高亮断言）。真模块链上 ArticleContentView
// 引 `#app`（vitest 不可解析），故不能用 importOriginal 混入真实现。
vi.mock('~/features/articles/public', async () => {
  const { default: ArticleCardView } = await import('~/features/articles/components/ArticleCardView.vue')
  const { getArticlePipelineState, getPipelineStateMeta } = await import('~/features/articles/composables/useArticleProcessingStatus')
  const { defineComponent, h } = await import('vue')
  return {
    ArticleCardView,
    getArticlePipelineState,
    getPipelineStateMeta,
    ArticleContentView: defineComponent({
      name: 'ArticleContentView',
      props: ['article', 'articles'],
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setup(props: any) {
        return () => h('div', { 'data-test': 'article-content-stub' }, `${props.article?.title ?? ''} ${props.article?.content ?? ''}`)
      },
    }),
  }
})

// ArticleCardView 内的 useFeedsStore 是 Nuxt auto-import（SFC 裸引用）——挂 globalThis
// eslint-disable-next-line @typescript-eslint/no-explicit-any
;(globalThis as any).useFeedsStore = () => ({ feeds: [] })

import SearchPage from './search.vue'

// AppDialog 是 Nuxt 自动导入组件，vitest 管线不解析——按 ArticlePreviewModal.test.ts 先例
// 顶名注册（保留 v-if(modelValue) 开关语义、插槽与关闭触发）。
const AppDialogStub = {
  name: 'AppDialog',
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  template: `<div v-if="modelValue" data-test="app-dialog-stub"><div data-test="app-dialog-header"><slot name="header" /></div><button data-test="app-dialog-close" @click="$emit('update:modelValue', false)">×</button><div data-test="app-dialog-body"><slot /></div></div>`,
}

const NuxtLinkStub = {
  name: 'NuxtLink',
  props: { to: { type: String, default: '' } },
  template: '<a :href="to" data-test="nuxt-link-stub"><slot /></a>',
}

function hit(id: string, over: { article?: Partial<Article>; archived?: boolean; hitReasons?: SearchHitReason[] } = {}) {
  const article: Article = {
    id, feedId: 'f1', title: `文章${id}`, description: '', content: '',
    link: 'https://example.com', pubDate: '2026-10-01T00:00:00Z', category: '',
    firecrawlStatus: 'completed', summaryStatus: 'complete', tagCount: 1,
  }
  return {
    article: { ...article, ...over.article },
    archived: over.archived ?? false,
    hitReasons: over.hitReasons ?? [{ type: 'keyword' as const, text: '特斯拉' }],
  }
}

function searchResponse(over: Partial<SearchResponseData> = {}): SearchResponseData {
  return {
    hits: [hit('101', { archived: true, hitReasons: [{ type: 'keyword', text: '特斯拉', score: 0.24 }] })],
    total: 1,
    page: 1,
    perPage: 20,
    tookMs: 312,
    semanticSkipped: false,
    semanticTags: [{ label: '新能源车', score: 0.87 }],
    expandWords: [],
    ...over,
  }
}

function mountPage() {
  return mount(SearchPage, {
    global: {
      components: { AppDialog: AppDialogStub, NuxtLink: NuxtLinkStub },
      mocks: { $dayjs: () => ({ fromNow: () => '2小时前' }) },
    },
  })
}

async function runSearchAndGetResults(w: ReturnType<typeof mountPage>) {
  api.search.mockResolvedValue({ success: true, data: searchResponse() })
  await w.find('input').setValue('特斯拉')
  await w.find('form.search-bar').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  api.search.mockReset()
  api.getArticle.mockReset()
  api.updateArticle.mockReset()
  navigateToSpy.mockReset()
  vi.stubGlobal('navigateTo', navigateToSpy)
})

afterEach(() => {
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

describe('/search — 状态矩阵', () => {
  it('初始空态：未搜索引导文案，无骨架/无结果列表、不发请求', () => {
    const w = mountPage()
    expect(w.find('[data-test="search-initial-empty"]').exists()).toBe(true)
    expect(w.text()).toContain('输入关键词，搜全站文章')
    expect(w.find('[data-test="search-skeletons"]').exists()).toBe(false)
    expect(w.find('.search-results').exists()).toBe(false)
    expect(api.search).not.toHaveBeenCalled()
  })

  it('提交→骨架屏→结果渲染：命中 chip 与归档角标存在（真实 ArticleCardView）', async () => {
    let release: (value: unknown) => void = () => {}
    api.search.mockImplementationOnce(() => new Promise((resolve) => { release = resolve }))
    const w = mountPage()
    await w.find('input').setValue('特斯拉')
    await w.find('form.search-bar').trigger('submit')

    // 首查骨架屏 ×3
    const skeletons = w.findAll('[data-test="search-skeletons"] .skeleton-row')
    expect(skeletons.length).toBe(3)

    release({ success: true, data: searchResponse() })
    await flushPromises()

    // 结果卡片：归档角标 + 关键词命中 chip
    expect(w.findAll('.article-row').length).toBe(1)
    expect(w.find('.archived-badge').text()).toBe('已归档')
    expect(w.find('.hit-chip.hit-keyword').text()).toContain('命中「特斯拉」')
    // 结果摘要行
    expect(w.find('.search-summary').text()).toContain('1 条结果')
    expect(w.find('.search-summary').text()).toContain('语义命中 1 个标签')
    expect(w.find('.search-summary').text()).toContain('0.31s')
  })

  it('空结果→「开启 LLM 扩展搜索再试」一键开启并自动重查（expand=on）', async () => {
    api.search.mockResolvedValueOnce({ success: true, data: searchResponse({ hits: [], total: 0, semanticTags: [] }) })
    const w = mountPage()
    await w.find('input').setValue('不存在词')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()

    expect(w.find('[data-test="search-no-results"]').exists()).toBe(true)
    const cta = w.find('[data-test="enable-expand-retry"]')
    expect(cta.exists()).toBe(true)

    api.search.mockResolvedValueOnce({ success: true, data: searchResponse() })
    await cta.trigger('click')
    await flushPromises()
    // 一键开启扩展并自动重查（spec「无结果引导扩展」Scenario）
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ expand: 'on', q: '不存在词' }))
    expect(w.findAll('.article-row').length).toBe(1)
  })

  it('错误条 + 重试保留输入：重试成功后恢复结果', async () => {
    api.search.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const w = mountPage()
    await w.find('input').setValue('特斯拉')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()

    const errorBar = w.find('.search-error')
    expect(errorBar.exists()).toBe(true)
    expect(errorBar.text()).toContain('网络错误')
    expect((w.find('input').element as HTMLInputElement).value).toBe('特斯拉')

    api.search.mockResolvedValueOnce({ success: true, data: searchResponse() })
    await w.find('[data-test="search-retry"]').trigger('click')
    await flushPromises()
    expect(w.findAll('.article-row').length).toBe(1)
  })

  it('语义降级：响应 semanticSkipped=true → 黄色提示条出现、关键词结果正常展示、无错误弹窗', async () => {
    const w = mountPage()
    api.search.mockResolvedValue({ success: true, data: searchResponse({ semanticSkipped: true, semanticTags: [] }) })
    await w.find('input').setValue('特斯拉')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()

    const notice = w.find('.search-notice')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('已降级为纯关键词结果')
    expect(w.findAll('.article-row').length).toBe(1)
    expect(w.find('.search-error').exists()).toBe(false)
  })

  it('扩展结果区隔：hitReasons 含 type=expand 的结果走分隔条之后，扩展词回显', async () => {
    const w = mountPage()
    api.search.mockResolvedValue({
      success: true,
      data: searchResponse({
        hits: [
          hit('101'),
          hit('102', { hitReasons: [{ type: 'expand', text: '起火' }] }),
          hit('103', { hitReasons: [{ type: 'expand', text: '热失控' }] }),
        ],
        total: 3,
        expandWords: ['起火', '热失控', '动力电池召回'],
      }),
    })
    await w.find('input').setValue('特斯拉电池自燃')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()

    const divider = w.find('[data-test="expand-divider"]')
    expect(divider.text()).toContain('以下 2 条来自扩展搜索')
    expect(w.find('.expand-hint').text()).toContain('起火 · 热失控 · 动力电池召回')
    // 扩展区卡片的扩展命中 chip
    const expandChips = w.findAll('.hit-chip.hit-expand')
    expect(expandChips.length).toBe(2)
    expect(expandChips[0]!.text()).toContain('扩展词「起火」')
  })

  it('R-5 双命中归属：keyword+expand 双理由卡留在主区，扩展区只放扩展独占命中', async () => {
    const w = mountPage()
    api.search.mockResolvedValue({
      success: true,
      data: searchResponse({
        hits: [
          hit('101', { hitReasons: [{ type: 'keyword', text: '特斯拉', score: 0.24 }] }),
          hit('102', { hitReasons: [
            { type: 'keyword', text: '特斯拉', score: 0.24 },
            { type: 'expand', text: '起火' },
          ] }),
          hit('103', { hitReasons: [{ type: 'expand', text: '起火' }] }),
        ],
        total: 3,
        expandWords: ['起火'],
      }),
    })
    await w.find('input').setValue('特斯拉')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()

    // 主区 = 主路命中全集（101 纯关键词 + 102 双命中）；扩展区 = 扩展独占（103）
    const lists = w.findAll('.search-results')
    expect(lists.length).toBe(2)
    const mainCards = lists[0]!.findAll('.article-row')
    const expandCards = lists[1]!.findAll('.article-row')
    expect(mainCards.map(c => c.text()).some(t => t.includes('文章102'))).toBe(true)
    expect(mainCards.length).toBe(2)
    expect(expandCards.length).toBe(1)
    expect(expandCards[0]!.text()).toContain('文章103')
    expect(expandCards[0]!.text()).not.toContain('文章102')
    expect(w.find('[data-test="expand-divider"]').text()).toContain('以下 1 条来自扩展搜索')

    // 双命中卡 chip 同时展示两类命中（不因双命中丢失任一解释）
    const dual = mainCards.find(c => c.text().includes('文章102'))!
    expect(dual.find('.hit-chip.hit-keyword').exists()).toBe(true)
    expect(dual.find('.hit-chip.hit-expand').exists()).toBe(true)
  })
})

describe('/search — 预览弹窗与过滤交互', () => {
  it('点卡片开预览（URL 不变、非路由跳转）：正文详情加载 + 命中词高亮；关闭回原位恢复滚动、列表保留', async () => {
    const w = mountPage()
    await runSearchAndGetResults(w)

    // 用户先把列表滚到 222 → 点卡片开弹窗（spec「关闭返回原位」的前置）
    const container = w.find('.search-main').element as HTMLElement
    container.scrollTop = 222

    // 详情接口返回含命中词的全文（snake_case → normalizeArticle）
    api.getArticle.mockResolvedValue({
      success: true,
      data: {
        id: 101, feed_id: 1, title: '特斯拉Model Y地下车库自燃', content: '<p>事故原因初步指向电池管线老化。</p>',
        link: '', pub_date: '2026-10-01T00:00:00Z', created_at: '2026-10-01T00:00:00Z',
        read: false, favorite: false,
      },
    })
    await w.find('.article-row').trigger('click')
    await flushPromises()

    // 弹窗开、路由桩未被触发（非路由跳转）
    expect(w.find('[data-test="app-dialog-stub"]').exists()).toBe(true)
    expect(w.findAll('[data-test="nuxt-link-stub"]').length).toBe(1) // 仅顶栏返回首页

    // 正文命中词高亮（关键词 hit 的 text=特斯拉）
    const marks = w.findAll('mark.apm-hit')
    expect(marks.length).toBeGreaterThan(0)
    expect(marks.some(m => m.text() === '特斯拉')).toBe(true)

    container.scrollTop = 0
    await w.find('[data-test="app-dialog-close"]').trigger('click')
    await nextTick()
    await nextTick()
    expect(container.scrollTop).toBe(222)
    expect(w.findAll('.article-row').length).toBe(1)
    expect((w.find('input').element as HTMLInputElement).value).toBe('特斯拉')
    expect(w.find('[data-test="app-dialog-stub"]').exists()).toBe(false)
  })

  it('「阅读全文」（spec「阅读全文跳转」）：关闭弹窗并深链 /?article=<id> 进首页阅读路径', async () => {
    const w = mountPage()
    await runSearchAndGetResults(w)

    api.getArticle.mockResolvedValue({
      success: true,
      data: {
        id: 101, feed_id: 1, title: '特斯拉Model Y地下车库自燃', content: '<p>事故原因初步指向电池管线老化。</p>',
        link: '', pub_date: '2026-10-01T00:00:00Z', created_at: '2026-10-01T00:00:00Z',
        read: false, favorite: false,
      },
    })
    await w.find('.article-row').trigger('click')
    await flushPromises()

    // 详情就绪后 header-actions 出现「阅读全文」
    const btn = w.find('[data-test="preview-read-full"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('阅读全文')

    await btn.trigger('click')
    await flushPromises()

    // 弹窗关闭 + 深链带字符串化 id（id 转换只在 API 归一层，此处透传 article.id）
    expect(w.find('[data-test="app-dialog-stub"]').exists()).toBe(false)
    expect(navigateToSpy).toHaveBeenCalledTimes(1)
    expect(navigateToSpy).toHaveBeenCalledWith({ path: '/', query: { article: '101' } })
  })

  it('含归档切换：点击 toggle 立即按 active-only 重查', async () => {
    const w = mountPage()
    await runSearchAndGetResults(w)

    api.search.mockResolvedValueOnce({ success: true, data: searchResponse() })
    await w.find('.app-toggle__track').trigger('click') // 第一个 toggle：含归档
    await flushPromises()
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ archived: 'active-only' }))
  })

  it('加载更多按钮：透传 loadMore（追加页不重置）', async () => {
    const w = mountPage()
    api.search.mockResolvedValueOnce({
      success: true,
      data: searchResponse({ hits: [hit('101'), hit('102')], total: 3 }),
    })
    await w.find('input').setValue('特斯拉')
    await w.find('form.search-bar').trigger('submit')
    await flushPromises()
    expect(w.find('[data-test="search-load-more"]').exists()).toBe(true)

    api.search.mockResolvedValueOnce({
      success: true,
      data: searchResponse({ hits: [hit('103')], total: 3, page: 2 }),
    })
    await w.find('[data-test="search-load-more"]').trigger('click')
    await flushPromises()
    expect(w.findAll('.article-row').length).toBe(3)
    // 加载完 → 尾部提示替代按钮
    expect(w.find('[data-test="search-load-more"]').exists()).toBe(false)
    expect(w.find('.search-end-hint').text()).toContain('已显示全部 3 条')
  })
})

describe('/search — 双主题零特判（源码锚）', () => {
  it('样式全走语义 token：无 data-theme 分支、无主题色字面量', () => {
    const style = searchPageSource.slice(searchPageSource.indexOf('<style'))
    expect(style).not.toContain('data-theme')
    expect(style).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
    expect(style).toContain('var(--color-bg-base)')
    expect(style).toContain('var(--color-warning-subtle)')
  })
})
