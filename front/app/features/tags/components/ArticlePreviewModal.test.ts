import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Article } from '~/types'
import ArticlePreviewModal from './ArticlePreviewModal.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// ArticleContentView 是重组件（iframe/工具栏全家桶）——模块级 mock 成轻桩，
// 只保留「渲染于 .preview-body 内」的可定位性（FeedLayoutShell.test.ts 同款先例）。
vi.mock('~/features/articles/public', () => ({
  ArticleContentView: {
    name: 'ArticleContentView',
    props: ['article', 'articles'],
    template: '<div data-test="article-content-stub" />',
  },
}))

// AppDialog 是 Nuxt 自动导入组件（本 SFC 无显式 import），vitest 管线不解析——
// global.stubs 会把未解析 vnode 换成不转发插槽的空桩（实测空 HTML），
// 须走 global.components 顶名注册（resolveComponent 按 app 级解析），
// 保留 v-if(modelValue) 开关语义与两个插槽。
const AppDialogStub = {
  name: 'AppDialog',
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  template: `<div v-if="modelValue" data-test="app-dialog-stub"><div data-test="app-dialog-header"><slot name="header" /></div><div data-test="app-dialog-body"><slot /></div></div>`,
}

function articleFixture(over: Partial<Article> = {}): Article {
  return {
    id: 'a1',
    feedId: 'f1',
    title: '测试文章',
    description: '',
    content: '<p>正文</p>',
    link: 'https://example.com/a1',
    pubDate: '2026-01-01T00:00:00Z',
    category: 'tech',
    ...over,
  }
}

function mountModal(extra: Record<string, unknown> = {}) {
  return mount(ArticlePreviewModal, {
    props: {
      visible: true,
      selectedPreviewArticle: articleFixture(),
      previewArticles: [],
      loadingPreviewArticle: false,
      ...extra,
    },
    global: { components: { AppDialog: AppDialogStub } },
  })
}

describe('ArticlePreviewModal — 内容区确定高度契约（fix-preview-dialog-iframe-height）', () => {
  it('visible=true 且选中文章存在 → 渲染 .preview-body 且携带显式 height 声明（锚定 85vh 上限）', () => {
    const w = mountModal()
    const body = w.find('.preview-body')
    expect(body.exists()).toBe(true)
    // happy-dom 不算布局：显式内联高度是可断言的结构契约
    // （flex:1 从非 flex 宿主解析不出高度，iframe 链条会塌陷——见 change design D1）
    const inlineHeight = (body.element as HTMLElement).style.height
    expect(inlineHeight).not.toBe('')
    expect(inlineHeight).toContain('85vh')
  })

  it('ArticleContentView（stub）渲染于 .preview-body 内', () => {
    const w = mountModal()
    const body = w.find('.preview-body')
    expect(body.find('[data-test="article-content-stub"]').exists()).toBe(true)
  })

  it('弹窗经 AppDialog 宿主打开：visible=false 时不渲染内容区', () => {
    const w = mountModal({ visible: false })
    expect(w.find('[data-test="app-dialog-stub"]').exists()).toBe(false)
    expect(w.find('.preview-body').exists()).toBe(false)
  })
})
