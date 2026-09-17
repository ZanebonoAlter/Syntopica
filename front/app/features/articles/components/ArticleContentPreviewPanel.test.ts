import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Article } from '~/types'

/**
 * ArticleContentPreviewPanel（redesign-reading-pane task 5.1）：
 * 无摘要不渲染整理稿区块、无 description 不渲染导语、有导语无边框、
 * 状态横幅/手动按钮行/内容源切换卡不再渲染（已迁入 ArticleStatusMenu）。
 */

vi.mock('@iconify/vue', () => ({
  Icon: {
    name: 'Icon',
    inheritAttrs: true,
    props: ['icon'],
    template: '<span class="icon-stub" :data-icon="icon" aria-hidden="true" />',
  },
}))

import ArticleContentPreviewPanel from './ArticleContentPreviewPanel.vue'

function article(over: Partial<Article> = {}): Article {
  return {
    id: 'a1',
    feedId: 'f1',
    title: '测试文章标题',
    description: '<p>这是导语文本。</p>',
    content: '<p>正文。</p>',
    link: '',
    pubDate: '2026-09-18T00:00:00Z',
    category: '',
    ...over,
  }
}

function mountPanel(over: Partial<Article> = {}, props: Record<string, unknown> = {}) {
  return mount(ArticleContentPreviewPanel, {
    props: {
      article: article(over),
      highlightedTagSlugs: [],
      aiEnabled: false,
      kickerLabel: '掘金 · 人工智能',
      renderedStoredSummary: '',
      manualTaggingLoading: false,
      manualTaggingLabel: '手动打标签',
      taggingError: null,
      actionBusy: false,
      showDescription: false,
      displayContent: '<p>正文渲染内容</p>',
      articleImageUrl: null,
      articlePubDate: '2026-09-18T00:00:00Z',
      articleAuthor: null,
      articleRead: false,
      articleTitleFull: '测试文章标题',
      handleManualTagging: vi.fn(),
      handleTagWatchToggle: vi.fn(),
      openOriginal: vi.fn(),
      ...props,
    },
    global: {
      components: {
        // vitest 环境无 Nuxt auto-import，显式注册 AppButton 替身（未注册组件 VTU 无法 stub）
        AppButton: {
          name: 'AppButton',
          props: ['variant', 'disabled'],
          template: '<button type="button" class="app-btn-stub"><slot /></button>',
        },
      },
    },
  })
}

describe('ArticleContentPreviewPanel', () => {
  it('renders reading col with kicker / meta / title / body aligned', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.reading-col').exists()).toBe(true)
    expect(wrapper.find('.kicker').exists()).toBe(true)
    expect(wrapper.find('.kicker-label').text()).toBe('掘金 · 人工智能')
    expect(wrapper.find('.article-title-full').text()).toBe('测试文章标题')
    expect(wrapper.find('.markdown-article').exists()).toBe(true)
    expect(wrapper.find('.section-sep').exists()).toBe(true)
  })

  it('does not render AI summary block when there is no summary', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.ai-block').exists()).toBe(false)
  })

  it('renders AI summary as frameless section (no card surface) when summary exists', () => {
    const wrapper = mountPanel({}, { renderedStoredSummary: '<p>摘要内容</p>' })
    const block = wrapper.find('.ai-block')
    expect(block.exists()).toBe(true)
    expect(block.text()).toContain('AI 整理稿')
    expect(block.find('.markdown-summary').exists()).toBe(true)
    // 去卡片化：不再有 summary-surface 卡框容器
    expect(block.find('.summary-surface').exists()).toBe(false)
  })

  it('does not render the lede when description guard hides it', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.lede').exists()).toBe(false)
    // 不再有旧版描述卡占位
    expect(wrapper.find('.article-description').exists()).toBe(false)
  })

  it('renders the lede as frameless lead paragraph (no card border) when description shows', () => {
    const wrapper = mountPanel({}, { showDescription: true })
    const lede = wrapper.find('.lede')
    expect(lede.exists()).toBe(true)
    expect(lede.classes()).not.toContain('article-description')
    expect(wrapper.find('.article-description').exists()).toBe(false)
  })

  it('no longer renders processing banner / manual action row / content source toggle', () => {
    const wrapper = mountPanel()
    const text = wrapper.text()
    expect(wrapper.find('.summary-surface').exists()).toBe(false)
    expect(text).not.toContain('全文已抓取')
    expect(text).not.toContain('正在抓取全文')
    expect(text).not.toContain('手动抓取全文')
    expect(text).not.toContain('手动生成总结')
    expect(text).not.toContain('Firecrawl 全文')
  })

  it('renders empty-content fallback with open-original button when body is empty', () => {
    const wrapper = mountPanel({}, { displayContent: '' })
    expect(wrapper.find('.markdown-article').exists()).toBe(false)
    const empty = wrapper.find('.empty-content')
    expect(empty.exists()).toBe(true)
    expect(empty.find('.app-btn-stub').text()).toContain('前往原文阅读')
  })
})
