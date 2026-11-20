import { describe, expect, it, vi } from 'vitest'
// Vite 的 ?raw 导入：测试环境不经过 Nuxt 管线，?raw 拿源码文本最直接
import source from './ArticleContentView.vue?raw'
import previewPanelSource from './ArticleContentPreviewPanel.vue?raw'
import contentViewSource from '../composables/useArticleContentView.ts?raw'
// 原文源加工链导出的纯函数：行为断言直接打在它上面（不拉 store）
import { buildOriginalContentHtml } from '~/utils/articleContentHtml'

/**
 * ArticleContentView — 全屏层 z-index 源码锚（2026-10-28 用户反馈修复）。
 *
 * 缺陷：`fullscreen-article` 层硬编码 `z-[100]`，而它在弹窗内（AppDialog 遮罩 9100）
 * 之下 → 全屏被遮罩盖住，点「全屏」看起来毫无反应。修法：层级由宿主经
 * `fullscreenZIndex` prop 传入（弹窗宿主传 9200）。
 *
 * 真组件挂载成本高（pinia stores + Nuxt runtime + 多个 API composable），
 * 故用源码锚守住「层级取自 prop、不得回迁硬编码」这条不变量
 * （同 change 源码守卫先例：防规则回迁）。
 */
const sourceText: string = source

describe('ArticleContentView — 全屏层 z-index 锚', () => {
  it('全屏层层级取自 fullscreenZIndex prop', () => {
    expect(sourceText).toMatch(/fullscreenZIndex\?: number/)
    expect(sourceText).toMatch(/class="fullscreen-article[^"]*"[\s\S]{0,200}zIndex: fullscreenZIndex/)
  })

  it('不得回迁硬编码 z-[100]（会再次被弹窗遮罩盖住）', () => {
    expect(sourceText).not.toContain('z-[100]')
  })

  it('prop 缺省值 100（阅读页原地全屏行为不变）', () => {
    expect(sourceText).toMatch(/fullscreenZIndex: 100/)
  })
})

/**
 * 导语段纯文本渲染（fix-reader-lede-blank-space task 4.2）。
 *
 * 事故形态：部分订阅源把整篇文章的 HTML 塞进 description，公众号根节点带
 * `style="visibility: visible; opacity: 0;"` —— 内容不可见却照样占位（实测单篇
 * 导语段高 8046 px），且 `<img onerror=...>` 经 v-html 直出会真实执行。
 * 契约：导语 MUST 以去标签纯文本渲染、长度 ≤300、不得把来源 HTML 注入 DOM。
 */
vi.mock('@iconify/vue', () => ({
  Icon: {
    name: 'Icon',
    inheritAttrs: true,
    props: ['icon'],
    template: '<span class="icon-stub" :data-icon="icon" aria-hidden="true" />',
  },
}))

vi.mock('mermaid', () => ({
  default: { initialize: vi.fn(), render: vi.fn() },
}))

import { mount } from '@vue/test-utils'
import type { Article } from '~/types'
import ArticleContentPreviewPanel from './ArticleContentPreviewPanel.vue'

function mountPreviewPanel(article: Partial<Article> = {}) {
  const full: Article = {
    id: 'a1',
    feedId: 'f1',
    title: '测试文章标题',
    description: '',
    content: '<p>正文。</p>',
    link: '',
    pubDate: '2026-09-18T00:00:00Z',
    category: '',
    ...article,
  }
  return mount(ArticleContentPreviewPanel, {
    props: {
      article: full,
      highlightedTagSlugs: [],
      aiEnabled: false,
      kickerLabel: '智源社区',
      renderedStoredSummary: '',
      manualTaggingLoading: false,
      manualTaggingLabel: '手动打标签',
      taggingError: null,
      actionBusy: false,
      showDescription: true,
      displayContent: '<p>正文渲染内容</p>',
      articleImageUrl: null,
      articlePubDate: '2026-09-18T00:00:00Z',
      articleAuthor: null,
      articleRead: false,
      articleTitleFull: '测试文章标题',
      handleManualTagging: vi.fn(),
      handleTagWatchToggle: vi.fn(),
      openOriginal: vi.fn(),
    },
    global: {
      components: {
        AppButton: {
          name: 'AppButton',
          props: ['variant', 'disabled'],
          template: '<button type="button" class="app-btn-stub"><slot /></button>',
        },
      },
    },
  })
}

describe('导语段 — 纯文本渲染（fix-reader-lede-blank-space）', () => {
  it('description 含 img onerror 时，导语节点不含任何元素子节点、不绑定事件属性', () => {
    const wrapper = mountPreviewPanel({
      description: '<p>可见导语</p><img src="https://example.com/a.png" onerror="alert(1)">',
    })
    const lede = wrapper.find('.lede')
    expect(lede.exists()).toBe(true)
    // 纯文本节点：导语内不得有任何元素
    expect(lede.element.querySelectorAll('*').length).toBe(0)
    expect(lede.html()).not.toContain('onerror')
    expect(lede.text()).toContain('可见导语')
  })

  it('description 被 opacity:0 包裹时，导语段不渲染、不留空白占位', () => {
    const wrapper = mountPreviewPanel({
      description: '<div style="visibility: visible; opacity: 0;">整篇隐藏正文</div>',
    })
    expect(wrapper.find('.lede').exists()).toBe(false)
  })

  it('导语文本长度不超过 300 字符', () => {
    const wrapper = mountPreviewPanel({ description: `<p>${'长'.repeat(500)}</p>` })
    const lede = wrapper.find('.lede')
    expect(lede.exists()).toBe(true)
    expect(lede.text().length).toBeLessThanOrEqual(300)
  })

  it('导语段不得再 v-html 直出第三方 HTML（源码锚，防规则回迁）', () => {
    expect(previewPanelSource).not.toContain('v-html="article.description"')
    expect(previewPanelSource).toMatch(/v-text="ledePlainText"/)
    expect(previewPanelSource).toMatch(/toPlainText\(props\.article\?\.description \?\? '', LEDE_MAX_RUNES\)/)
  })
})

describe('原文源渲染前清洗（fix-reader-lede-blank-space task 4.3）', () => {
  it('原文源分支施加 sanitizeHtml，且清洗在图片代理与 Base64 剔除之前', () => {
    expect(contentViewSource).toMatch(/import \{ buildOriginalContentHtml \} from '~\/utils\/articleContentHtml'/)
    expect(contentViewSource).toMatch(/return buildOriginalContentHtml\(resolvedContent\)/)
    // 既有链路不变：Base64 图片剔除仍是 proxyImagesInHtml 的入参
    expect(contentViewSource).toMatch(/proxyImagesInHtml\(content\.replace\(/)
    expect(contentViewSource).toContain('Base64-Image-Removed')
  })

  it('firecrawl 源的 markdown 渲染链不变', () => {
    expect(contentViewSource).toMatch(/content = marked\.parse\(resolvedContent\) as string/)
  })
})

describe('原文源渲染前清洗 — 行为断言（task 7.8）', () => {
  it('视觉隐藏的正文清洗后仍可见（剥声明而非删子树）', () => {
    const html = buildOriginalContentHtml('<div style="visibility: visible; opacity: 0;"><p>正文第一段</p></div>')
    expect(html).toContain('正文第一段')
    expect(html).not.toContain('opacity')
  })

  it('语义不展示的块连同子树被移除', () => {
    const html = buildOriginalContentHtml('<p style="display: none">藏块</p><p>正文</p>')
    expect(html).not.toContain('藏块')
    expect(html).toContain('正文')
  })

  it('事件属性不被保留', () => {
    const html = buildOriginalContentHtml('<p onclick="alert(1)">正文</p><img src="https://e.com/a.png" onerror="alert(1)">')
    expect(html).not.toMatch(/onclick|onerror/i)
    expect(html).toContain('正文')
  })

  it('脚本与可执行嵌入被剔除', () => {
    const html = buildOriginalContentHtml('<script>alert(1)</script><iframe srcdoc="<script>x</script>"></iframe><p>正文</p>')
    expect(html).not.toContain('<script')
    expect(html).not.toContain('<iframe')
    expect(html).toContain('正文')
  })

  it('危险 URL 方案被移除、外链图片仍走代理（顺序契约）', () => {
    const html = buildOriginalContentHtml('<a href="javascript:alert(1)">链接</a><img src="https://cdn.example.com/a.png">')
    expect(html).not.toContain('javascript:')
    expect(html).toContain('/api/image-proxy?url=')
  })
})

/**
 * displayContent 分支行为（review M4 / task 7.8 的补强）。
 *
 * 上一条 describe 打在 `buildOriginalContentHtml`（原文源加工链的纯函数，
 * displayContent 原文分支的输入产出者）；这里再走一遍真 composable，守住
 * 「哪个源走哪条链」这层运行时行为——分支选错在纯函数测试里看不出来。
 * 只替换会真开 IO 的依赖（事件流 WebSocket、详情状态请求、pinia store）。
 */
describe('displayContent 分支行为（composable 挂载）', () => {
  it('原文源走清洗链：隐藏声明剥掉、正文可见、事件属性与嵌入元素不残留', async () => {
    // 本用例自行控制模块图（末尾动态 import），不污染同文件其它用例
    vi.resetModules()
    vi.doMock('~/features/articles/composables/useTagWebSocket', () => ({
      useTagWebSocket: () => ({
        onResult: vi.fn(),
        onError: vi.fn(),
        watchArticle: vi.fn(),
        clearWatch: vi.fn(),
      }),
    }))
    vi.doMock('~/features/articles/composables/useContentCompletion', () => ({
      useContentCompletion: () => ({
        getCompletionStatus: vi.fn().mockResolvedValue(null),
        completeArticle: vi.fn(),
      }),
    }))
    vi.doMock('~/stores/feeds', () => ({ useFeedsStore: () => ({ feeds: [] }) }))
    vi.doMock('~/stores/articles', () => ({ useArticlesStore: () => ({ articles: [] }) }))

    const { defineComponent, h } = await import('vue')
    const { useArticleContentView } = await import('../composables/useArticleContentView')
    const Harness = defineComponent({
      props: { article: { type: Object, required: true } },
      setup(props) {
        const view = useArticleContentView({ article: props.article as Article })
        return () => h('div', { class: 'harness-content', innerHTML: view.displayContent.value })
      },
    })
    const article: Article = {
      id: 'a1',
      feedId: 'f1',
      title: '测试文章标题',
      description: '',
      content: '<div style="visibility: visible; opacity: 0;"><p>被藏起来的正文</p></div>'
        + '<p style="display: none;">藏块</p>'
        + '<img src="x" onerror="alert(1)">'
        + '<iframe srcdoc="<script>alert(2)</script>"></iframe>',
      link: '',
      pubDate: '2026-09-18T00:00:00Z',
      category: '',
    }
    const wrapper = mount(Harness, { props: { article } })

    // 行为：清洗链真的跑过（不是源码字符串断言）
    expect(wrapper.text()).toContain('被藏起来的正文')
    expect(wrapper.text()).not.toContain('藏块')
    expect(wrapper.html()).not.toContain('opacity')
    expect(wrapper.html()).not.toContain('onerror')
    expect(wrapper.element.querySelector('iframe')).toBeNull()
  })
})
