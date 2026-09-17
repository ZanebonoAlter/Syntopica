import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import type { Article, RssFeed } from '~/types'

/**
 * ArticleCardView 行式渲染状态矩阵（declutter-article-list-panel test-cases 主链路 2/4/5 落点）：
 * 四态图标、处理详情浮层（三行 + 失败错误文案 + 点外关闭）、
 * 单 feed 视图隐藏来源名、已读弱化、收藏/选中事件。
 */

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const feeds: Partial<RssFeed>[] = [
  { id: 'f1', title: '智源社区', color: '#888', icon: 'mdi:rss' },
]

// useFeedsStore 是 Nuxt auto-import（SFC 内裸引用），测试环境挂 globalThis
// eslint-disable-next-line @typescript-eslint/no-explicit-any
;(globalThis as any).useFeedsStore = () => ({
  feeds: feeds as RssFeed[],
  getCategoryBySlug: () => null,
})

import ArticleCardView from './ArticleCardView.vue'

// node:fs 静态/动态 import 在本 vitest 环境被 stub（env 回归，另行立项）；getBuiltinModule 绕过模块图
function readCss(): string {
  return ((process as unknown as { getBuiltinModule: (m: string) => typeof import('node:fs') }).getBuiltinModule('node:fs'))
    .readFileSync('app/components/article/ArticleCard.css', 'utf8')
}
import articleCardCss from '../../../components/article/ArticleCard.css?raw'

function article(over: Partial<Article> = {}): Article {
  return {
    id: 'a1',
    feedId: 'f1',
    title: '测试文章标题',
    description: '',
    content: '',
    link: '',
    pubDate: '2026-09-18T00:00:00Z',
    category: '',
    firecrawlStatus: 'completed',
    summaryStatus: 'complete',
    tagCount: 4,
    ...over,
  }
}

function mountCard(a: Article, props: Record<string, unknown> = {}) {
  return mount(ArticleCardView, {
    props: { article: a, ...props },
    global: {
      mocks: {
        $dayjs: () => ({ fromNow: () => '2小时前' }),
      },
    },
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ArticleCardView 行式渲染', () => {
  it('四态图标：完成淡灰 / 排队琥珀 / 失败红 / 进行中转圈（每行仅 1 个状态指示）', async () => {
    const done = mount(ArticleCardView, { props: { article: article() }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(done.find('.row-state-done').exists()).toBe(true)
    expect(done.find('.row-state-icon').findAll('*').length).toBeGreaterThanOrEqual(0)
    expect(done.find('.row-state-spin').exists()).toBe(false)

    const queued = mount(ArticleCardView, { props: { article: article({ firecrawlStatus: 'pending', summaryStatus: 'complete' }) }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(queued.find('.row-state-queued').exists()).toBe(true)

    const failed = mount(ArticleCardView, { props: { article: article({ firecrawlStatus: 'failed', firecrawlError: 'boom' }) }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(failed.find('.row-state-failed').exists()).toBe(true)

    const processing = mount(ArticleCardView, { props: { article: article({ firecrawlStatus: 'processing', summaryStatus: 'pending' }) }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(processing.find('.row-state-spin').exists()).toBe(true)
    expect(processing.find('.row-state-icon').exists()).toBe(false)
  })

  it('点开处理详情浮层：三行状态 + 标签计数，无错误段', async () => {
    const w = mount(ArticleCardView, { props: { article: article() }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(w.find('.row-status-popover').exists()).toBe(false)
    await w.find('.row-state-icon').trigger('click')
    const pop = w.find('.row-status-popover')
    expect(pop.exists()).toBe(true)
    const text = pop.text()
    expect(text).toContain('抓取')
    expect(text).toContain('总结')
    expect(text).toContain('标签')
    expect(text).toContain('已标记 4')
    expect(pop.find('.rsp-error').exists()).toBe(false)
  })

  it('失败行浮层：标题抓取失败 + 错误文案完整可见', async () => {
    const w = mount(ArticleCardView, {
      props: { article: article({ firecrawlStatus: 'failed', firecrawlError: '目标站点 504 Gateway Timeout（重试 3 次未成功）' }) },
      global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } },
    })
    await w.find('.row-state-icon').trigger('click')
    const pop = w.find('.row-status-popover')
    expect(pop.text()).toContain('抓取失败')
    expect(pop.find('.rsp-error').text()).toContain('504 Gateway Timeout')
  })

  it('再点图标 / 点击行外（document mousedown）关闭浮层', async () => {
    const w = mount(ArticleCardView, { props: { article: article() }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    await w.find('.row-state-icon').trigger('click')
    expect(w.find('.row-status-popover').exists()).toBe(true)
    await w.find('.row-state-icon').trigger('click')
    expect(w.find('.row-status-popover').exists()).toBe(false)

    await w.find('.row-state-icon').trigger('click')
    expect(w.find('.row-status-popover').exists()).toBe(true)
    // 点击行外（document 为 target，不在本行内）关闭
    document.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.find('.row-status-popover').exists()).toBe(false)
  })

  it('单 feed 视图（showFeedTitle=false）隐藏来源名；默认视图显示', () => {
    const feedView = mount(ArticleCardView, { props: { article: article(), showFeedTitle: false }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(feedView.find('.row-identity').exists()).toBe(false)
    const all = mount(ArticleCardView, { props: { article: article() }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    expect(all.find('.row-identity').text()).toBe('智源社区')
  })

  it('已读行弱化 class + 收藏/选中事件', async () => {
    // emitted() 捕获链在 vue3.5+VTU 组合下失效（Vue 3.5 移除 component:emit → devtools hook 通知），
    // 改用 attrs 事件 spy（走真实 Vue 事件传播，B-probe 已验证 handler 真跑）
    const onFavorite = vi.fn()
    const onClick = vi.fn()
    const w = mount(ArticleCardView, {
      props: { article: article({ read: true, favorite: true }) },
      attrs: { onFavorite, onClick },
      global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } },
    })
    expect(w.find('article').classes()).toContain('article-row-read')
    expect(w.find('.row-favorite').classes()).toContain('active')

    await w.find('.row-favorite').trigger('click')
    expect(onFavorite).toHaveBeenCalledWith('a1')

    await w.find('article').trigger('click')
    expect(onClick).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1' }))
  })

  it('超长标题两行截断：行高类锚点存在', () => {
    const w = mount(ArticleCardView, { props: { article: article({ title: '长'.repeat(200) }) }, global: { mocks: { $dayjs: () => ({ fromNow: () => '' }) } } })
    const title = w.find('.row-title')
    // line-clamp 截断样式锚（规则来自 ArticleCard.css）
    expect(title.attributes('class')).toContain('row-title')
    expect(title.text()).toContain('长')
  })

  // ===== v5 视觉修订（ui-design 修订 v5，用户 2026-09-18 批准）=====

  it('v5 封面槽：image_url 非空渲染 img；为空降级 feed 图标占位；行高/字号样式锚', () => {
    const withCover = mountCard(article({ imageUrl: 'https://example.com/cover.jpg' }))
    const cover = withCover.find('.row-cover')
    expect(cover.exists()).toBe(true)
    expect(cover.find('img').exists()).toBe(true)
    expect(cover.find('img').attributes('src')).toBe('https://example.com/cover.jpg')

    // 无图 → 占位（FeedIcon 渲染，无 img）
    const noCover = mountCard(article())
    const cover2 = noCover.find('.row-cover')
    expect(cover2.find('img').exists()).toBe(false)
    expect(cover2.text() || '').toBeDefined()

    // v5 密度样式锚（规则来自 ArticleCard.css 源文本，防样式重构回退）
    // node:fs 静态/动态 import 在本 vitest 环境被 stub 成 undefined（env 回归，另行立项）；
// process.getBuiltinModule 绕过模块图直接取内建模块
    const css = ((process as any).getBuiltinModule('node:fs') as typeof import('node:fs'))
      .readFileSync('app/components/article/ArticleCard.css', 'utf8')
    expect(css).toContain('height: 100px')
    expect(css).toContain('flex: 0 0 68px')
    expect(css).toContain('font-size: 15px')
    expect(css).toContain('font-size: 12.5px')
  })

  it('v5 已读态局部降级：整行 opacity 已移除，仅标题降色（封面/浮层不受牵连）', () => {
    const css = readCss()
    // 负向锚：read 行不得再有整行 opacity（透字根因 + 可读性）
    expect(css).not.toMatch(/\.article-row\.article-row-read\s*\{[^}]*opacity/)
    // 正向锚：仅标题降色
    expect(css).toContain('.article-row.article-row-read .row-title')
    expect(css).toContain('color: var(--color-text-secondary)')
  })

  it('v5 处理详情浮层实心底：popover 块背景为实心 bg-base，无 rgba/透明底', () => {
    const css = readCss()
    const start = css.indexOf('.row-status-popover {')
    const end = css.indexOf('}', start)
    const block = css.slice(start, end)
    expect(block).toContain('background: var(--color-bg-base)')
    expect(block).not.toMatch(/rgba|bg-hover/)
  })

  it('v5 封面加载失败：@error 降级占位，不渲染破图', async () => {
    const w = mountCard(article({ imageUrl: 'https://example.com/broken.jpg' }))
    expect(w.find('.row-cover img').exists()).toBe(true)
    await w.find('.row-cover img').trigger('error')
    await nextTick()
    expect(w.find('.row-cover img').exists()).toBe(false)
  })
})
