import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import type { Article } from '~/types'
import ArticlePreviewModal from './ArticlePreviewModal.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// ArticleContentView 是重组件（iframe/工具栏全家桶）——模块级 mock 成轻桩，
// 只保留「渲染于 .preview-body 内」的可定位性（FeedLayoutShell.test.ts 同款先例）。
// 桩内回显 article.title/content 文本，供命中词高亮（add-article-search）断言 mark 包裹。
vi.mock('~/features/articles/public', async () => {
  const { h } = await import('vue')
  return {
    ArticleContentView: {
      name: 'ArticleContentView',
      props: ['article', 'articles', 'fullscreenZIndex'],
      // R-6（review）：正文经 innerHTML 回显——等价真组件 v-html 的重渲染语义
      // （article 引用替换触发重渲染时，Vue 对 innerHTML 无条件重设、注入 mark 被
      // 丢弃），使「响应式更新后 mark 自动重施」回归测试可复现真实丢弃路径
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setup(props: any) {
        return () => h('div', {
          'data-test': 'article-content-stub',
          innerHTML: `${props.article?.title ?? ''} ${props.article?.content ?? ''}`,
        })
      },
    },
  }
})

// AppDialog 是 Nuxt 自动导入组件（本 SFC 无显式 import），vitest 管线不解析——
// global.stubs 会把未解析 vnode 换成不转发插槽的空桩（实测空 HTML），
// 须走 global.components 顶名注册（resolveComponent 按 app 级解析），
// 保留 v-if(modelValue) 开关语义与两个插槽。
const AppDialogStub = {
  name: 'AppDialog',
  props: {
    modelValue: { type: Boolean, default: false },
    semanticDialog: { type: Boolean, default: false },
    zIndex: { type: Number, default: 1000 },
  },
  emits: ['update:modelValue'],
  template: `<div v-if="modelValue" data-test="app-dialog-stub" :data-semantic="String(semanticDialog)"><div data-test="app-dialog-header"><slot name="header" /></div><button data-test="app-dialog-close" @click="$emit('update:modelValue', false)">×</button><div data-test="app-dialog-body"><slot /></div></div>`,
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

describe('ArticlePreviewModal — 命中词高亮（add-article-search）', () => {
  it('highlightTerms 传入：正文命中词被 mark.apm-hit 包裹，未命中文本不受影响', async () => {
    const w = mountModal({
      selectedPreviewArticle: articleFixture({
        title: '特斯拉Model Y地下车库自燃',
        content: '事故原因初步指向电池管线老化，官方已介入调查。',
      }),
      highlightTerms: ['自燃', '电池'],
    })
    await nextTick()
    const marks = w.findAll('mark.apm-hit')
    const texts = marks.map(m => m.text())
    expect(texts).toContain('自燃')
    expect(texts).toContain('电池')
    // 未命中片段保持普通文本（stub 回显 title+content）
    expect(w.find('[data-test="article-content-stub"]').text()).toContain('事故原因')
  })

  it('长词优先：术语不被更短的词拆碎嵌套', async () => {
    const w = mountModal({
      selectedPreviewArticle: articleFixture({ title: '', content: '特斯拉电池自燃复盘' }),
      highlightTerms: ['特斯拉电池', '电池'],
    })
    await nextTick()
    const marks = w.findAll('mark.apm-hit')
    expect(marks.length).toBe(1)
    expect(marks[0]!.text()).toBe('特斯拉电池')
    expect(marks[0]!.element.querySelector('mark')).toBe(null)
  })

  it('重复高亮防护：同一 DOM 二次应用不产生嵌套 mark', async () => {
    const w = mountModal({
      selectedPreviewArticle: articleFixture({ title: '', content: '关键词出现两次：关键词' }),
      highlightTerms: ['关键词'],
    })
    await nextTick()
    // 弹窗关闭再打开（visible 抖动）→ 重放高亮，已有 mark 的祖先链被跳过
    await w.setProps({ visible: false })
    await w.setProps({ visible: true })
    await nextTick()
    const marks = w.findAll('mark.apm-hit')
    expect(marks.length).toBe(2)
    for (const m of marks) expect(m.element.parentElement?.tagName).not.toBe('MARK')
  })

  it('R-6：响应式更新丢弃后自动重施——selectedPreviewArticle 同 id 引用替换（弹窗内收藏路径）→ mark 重现', async () => {
    const article = articleFixture({
      title: '特斯拉Model Y地下车库自燃',
      content: '事故原因初步指向电池管线老化。',
    })
    const w = mountModal({ selectedPreviewArticle: article, highlightTerms: ['特斯拉'] })
    await nextTick()
    expect(w.findAll('mark.apm-hit').length).toBe(1)

    // 弹窗内收藏（search.vue handleFavorite）：同 id 新对象替换引用 → 子树重渲染
    // 重设 innerHTML，注入 mark 被丢弃（无修复时归零且不重施）→ watch 重施后仍在
    await w.setProps({ selectedPreviewArticle: { ...article, favorite: true } })
    await nextTick()
    await nextTick()
    const marks = w.findAll('mark.apm-hit')
    expect(marks.length).toBe(1)
    expect(marks[0]!.text()).toBe('特斯拉')
  })

  it('R-6：MutationObserver 兜底——不经 props 的内部重渲染丢弃 DOM（v-html 区重设）后 mark 自动重施', async () => {
    const w = mountModal({
      selectedPreviewArticle: articleFixture({ title: '特斯拉Model Y地下车库自燃', content: '正文' }),
      highlightTerms: ['特斯拉'],
    })
    await nextTick()
    expect(w.findAll('mark.apm-hit').length).toBe(1)

    // 模拟 ArticleContentView 内部状态重渲染（视图模式/正文源切换等，不经本组件
    // props）：直接重写 .preview-body 内子树 innerHTML，丢弃已注入 mark
    const stub = w.find('[data-test="article-content-stub"]').element as HTMLElement
    stub.innerHTML = '特斯拉Model Y地下车库自燃 正文'
    expect(w.findAll('mark.apm-hit').length).toBe(0) // 丢弃瞬间
    // MutationObserver 回调异步派发：等微任务检查点之后重施
    await new Promise(resolve => setTimeout(resolve, 0))
    const marks = w.findAll('mark.apm-hit')
    expect(marks.length).toBe(1)
    expect(marks[0]!.text()).toBe('特斯拉')
  })

  it('缺省（不传 highlightTerms）：零 mark，行为与 tags 域既有用法一致', async () => {
    const w = mountModal({
      selectedPreviewArticle: articleFixture({ title: '', content: '正文含自燃字样' }),
    })
    await nextTick()
    expect(w.findAll('mark.apm-hit').length).toBe(0)
    expect(w.find('[data-test="article-content-stub"]').text()).toContain('自燃')
  })

  it('关闭：经 AppDialog update:modelValue=false 触发 close 事件（父层据此关弹窗回列表原位）', async () => {
    const onClose = vi.fn()
    const w = mount(ArticlePreviewModal, {
      props: {
        visible: true,
        selectedPreviewArticle: articleFixture({ content: '正文' }),
        previewArticles: [],
        loadingPreviewArticle: false,
        highlightTerms: ['正文'],
        onClose,
      },
      global: { components: { AppDialog: AppDialogStub } },
    })
    await nextTick()
    expect(w.findAll('mark.apm-hit').length).toBe(1)
    await w.find('[data-test="app-dialog-close"]').trigger('click')
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

// ===== header-actions 可选插槽（add-article-search D1）：不传时 header 与现状一致（tags 域零变化红线） =====

describe('ArticlePreviewModal — header-actions 插槽（add-article-search）', () => {
  it('插槽传入：内容渲染于 header 内，DOM 顺序为 标题 → 动作 → 关闭钮（标题行右侧、关闭钮之前）', () => {
    const w = mount(ArticlePreviewModal, {
      props: {
        visible: true,
        selectedPreviewArticle: articleFixture(),
        previewArticles: [],
        loadingPreviewArticle: false,
      },
      slots: { 'header-actions': '<button data-test="header-action-btn">阅读全文</button>' },
      global: { components: { AppDialog: AppDialogStub } },
    })
    const headerHtml = w.find('[data-test="app-dialog-stub"]').html()
    expect(w.find('[data-test="header-action-btn"]').exists()).toBe(true)
    expect(headerHtml.indexOf('preview-header-text')).toBeLessThan(headerHtml.indexOf('header-action-btn'))
    // 关闭钮与动作同层级（AppDialog header flex 行的末尾）：标题 → 动作 → 关闭钮
    expect(headerHtml.indexOf('header-action-btn')).toBeLessThan(headerHtml.indexOf('app-dialog-close'))
  })

  it('不传插槽：header 仅标题文本，无 .preview-header-actions 节点（与现状一致）', () => {
    const w = mountModal()
    const header = w.find('[data-test="app-dialog-header"]')
    expect(header.find('.preview-header-actions').exists()).toBe(false)
    expect(header.findAll('p.preview-header-text').length).toBe(1)
    expect(header.text()).toContain('文章预览')
  })
})

// ===== 相关线索阅读上下文（replace-lane-lifeline-with-topic-events review R-1）=====
// 从线索 chip 打开时要带「档位 + 分数 + 候选日期」；普通文章行不传 → 整行不渲染。
describe('ArticlePreviewModal — 相关线索上下文行', () => {
  it('传 relatedContext：标题下方渲染一行「相关线索 · 强关联 0.89 · 日期」', () => {
    const w = mountModal({ relatedContext: { score: 0.89, level: 'strong', periodDate: '2026-11-02' } })
    const row = w.find('[data-testid="preview-related-context"]')
    expect(row.exists()).toBe(true)
    expect(row.text()).toBe('相关线索 · 强关联 0.89 · 2026-11-02')
    // 仍在 header 内，且标题行不受影响（header 不再多出第二个 .preview-header-text）
    expect(w.find('[data-test="app-dialog-header"]').element.contains(row.element)).toBe(true)
    expect(w.findAll('p.preview-header-text')).toHaveLength(1)
  })

  it('related 档显示「相关」，分数向零截断不跨档（0.8499 → 0.84）', () => {
    const w = mountModal({ relatedContext: { score: 0.8499, level: 'related', periodDate: '2026-10-30' } })
    expect(w.find('[data-testid="preview-related-context"]').text()).toBe('相关线索 · 相关 0.84 · 2026-10-30')
  })

  it('不传 relatedContext（普通文章行）：不渲染上下文行（零行为变化）', () => {
    const w = mountModal()
    expect(w.find('[data-testid="preview-related-context"]').exists()).toBe(false)
    expect(w.text()).not.toContain('相关线索')
  })
})


// ===== 全屏层级契约（2026-10-28 用户反馈修复）：弹窗内点「全屏」不能被遮罩盖住 =====

describe('ArticlePreviewModal — 全屏层级（fix-preview-fullscreen-zindex）', () => {
  it('弹窗遮罩层级 9100；全屏层层级高于遮罩（否则全屏被盖住，点击像没反应）', () => {
    const w = mountModal()
    const dialogZ = Number(w.findComponent({ name: 'AppDialog' }).props('zIndex'))
    const fullscreenZ = Number(w.findComponent({ name: 'ArticleContentView' }).props('fullscreenZIndex'))
    expect(dialogZ).toBe(9100)
    expect(fullscreenZ).toBeGreaterThan(dialogZ)
  })

  it('全屏层层级仍低于全局 toast（9999）——全屏中的失败提示要能看到', () => {
    const w = mountModal()
    const fullscreenZ = Number(w.findComponent({ name: 'ArticleContentView' }).props('fullscreenZIndex'))
    expect(fullscreenZ).toBeLessThan(9999)
  })
})

describe('ArticlePreviewModal — R38 matrix 语义 opt-in', () => {
  it('默认不启用；matrix 显式 dialogSemantics=true 才转发给 AppDialog', () => {
    const normal = mountModal()
    expect(normal.find('[data-test="app-dialog-stub"]').attributes('data-semantic')).toBe('false')
    normal.unmount()

    const matrix = mountModal({ dialogSemantics: true })
    expect(matrix.find('[data-test="app-dialog-stub"]').attributes('data-semantic')).toBe('true')
    matrix.unmount()
  })
})
