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
    expect(cover.find('img').attributes('src')).toBe(
      `/api/image-proxy?url=${encodeURIComponent('https://example.com/cover.jpg')}`,
    )

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

  // ===== 搜索命中 chip 与归档角标（add-article-search）：缺省零渲染，传入才生效 =====

  it('搜索三型命中 chip：🔍关键词 info / 🏷语义+分数 success / ✨扩展 warning（内联 meta 行）', () => {
    const w = mountCard(article(), {
      hitReasons: [
        { type: 'keyword', text: '特斯拉', score: 0.24 },
        { type: 'semantic', text: '新能源车', score: 0.83 },
        { type: 'expand', text: '起火' },
      ],
    })
    const chips = w.findAll('.hit-chip')
    expect(chips.length).toBe(3)
    expect(chips[0]!.classes()).toContain('hit-keyword')
    expect(chips[0]!.text()).toContain('🔍')
    expect(chips[0]!.text()).toContain('命中「特斯拉」')
    expect(chips[1]!.classes()).toContain('hit-semantic')
    expect(chips[1]!.text()).toContain('🏷')
    expect(chips[1]!.text()).toContain('标签「新能源车」')
    // 语义分数保留两位小数回显
    expect(chips[1]!.find('.hit-chip-score').text()).toBe('0.83')
    expect(chips[2]!.classes()).toContain('hit-expand')
    expect(chips[2]!.text()).toContain('✨')
    expect(chips[2]!.text()).toContain('扩展词「起火」')
    // chip 落在 meta 行内（不新增行，行高不变）
    expect(w.find('.row-meta').find('.hit-chip').exists()).toBe(true)
  })

  it('归档角标：标题尾「已归档」灰描边小标签 + 行降饱和类；语义三色走既有 token 不新造色', () => {
    const w = mountCard(article(), { archived: true })
    expect(w.find('article').classes()).toContain('article-row-archived')
    const badge = w.find('.archived-badge')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('已归档')
    expect(w.find('.row-title').text()).toContain('已归档')

    // 样式锚（防样式重构丢失）：三型 chip 色 + subtle 变体 + 归档降饱和限定封面/文字（不碰浮层）
    const css = readCss()
    expect(css).toContain('.article-row .hit-keyword')
    expect(css).toContain('color-mix(in srgb, var(--color-info) 12%, transparent)')
    expect(css).toContain('.article-row .hit-semantic')
    expect(css).toContain('var(--color-success-subtle)')
    expect(css).toContain('.article-row .hit-expand')
    expect(css).toContain('var(--color-warning-subtle)')
    expect(css).toContain('.article-row.article-row-archived .row-cover img')
    expect(css).not.toMatch(/\.article-row\.article-row-archived\s*\{[^}]*filter/)
  })

  it('缺省 props（列表既有用法）：不渲染 chip/角标，模板与现列表一致', () => {
    const w = mountCard(article())
    expect(w.findAll('.hit-chip').length).toBe(0)
    expect(w.find('.archived-badge').exists()).toBe(false)
    expect(w.find('article').classes()).not.toContain('article-row-archived')
  })

  // ===== 疑似 AI 生成角标（add-ai-generated-article-detection）=====

  it('疑似 AI 生成角标：suspect 压封面右上角（不占标题宽、不进 meta 行），未判定/非 AI/缺省一律不渲染也不占位', () => {
    const suspect = mountCard(article({ aiDetectionStatus: 'suspect' }))
    const badge = suspect.find('.suspect-badge')
    expect(badge.exists()).toBe(true)
    // 紧凑短标 + 完整文案走无障碍名（修订 C：165px meta 行装不下全称角标）
    expect(badge.text()).toBe('AI')
    expect(badge.attributes('aria-label')).toBe('疑似 AI 生成')
    expect(badge.attributes('title')).toBe('疑似 AI 生成')
    // 结构断言：角标在封面槽内——既不在标题链（R-5 会被 clamp 盒整块裁掉），
    // 也不在 meta 行（R-13 后实测会裁掉右侧 4px 并把来源名压到 0 宽）
    expect(suspect.find('.row-cover').find('.suspect-badge').exists()).toBe(true)
    expect(suspect.find('.row-title').find('.suspect-badge').exists()).toBe(false)
    expect(suspect.find('.row-heading').find('.suspect-badge').exists()).toBe(false)
    expect(suspect.find('.row-meta').find('.suspect-badge').exists()).toBe(false)
    // meta 行内容不变：时间 / 来源名 / 处理状态图标都还在
    expect(suspect.find('.row-meta > .row-time').exists()).toBe(true)
    expect(suspect.find('.row-meta > .row-identity').exists()).toBe(true)
    expect(suspect.find('.row-meta > .row-state-icon, .row-meta > .row-state-spin').exists()).toBe(true)

    const unknown = mountCard(article({ aiDetectionStatus: 'unknown' }))
    expect(unknown.find('.suspect-badge').exists()).toBe(false)
    expect(unknown.text()).not.toContain('疑似 AI 生成')

    expect(mountCard(article({ aiDetectionStatus: 'not_ai' })).find('.suspect-badge').exists()).toBe(false)
    expect(mountCard(article()).find('.suspect-badge').exists()).toBe(false)
  })

  it('clamp 盒仍只包标题：.row-title 保留两行截断，角标在盒外 → 长标题截断时角标仍可见', () => {
    const css = readCss()
    const titleStart = css.indexOf('.article-row .row-title {')
    expect(titleStart).toBeGreaterThan(-1)
    const titleBlock = css.slice(titleStart, css.indexOf('}', titleStart))
    expect(titleBlock).toContain('-webkit-line-clamp: 2')
    expect(titleBlock).toContain('overflow: hidden')
    // 角标已离开标题行与 meta 行（修订 C）：只挂在封面槽上，不受 clamp / meta nowrap 影响
    expect(css).not.toContain('.article-row .row-heading .suspect-badge {')
    expect(css).not.toContain('.article-row .row-meta .suspect-badge {')
  })

  it('疑似 AI 角标不改行高：压封面绝对定位、实底 pill，字色用 text-secondary（不用 muted）', () => {
    const css = readCss()
    const start = css.indexOf('.article-row .suspect-badge {')
    expect(start).toBeGreaterThan(-1)
    const block = css.slice(start, css.indexOf('}', start))
    expect(block).toContain('position: absolute')
    expect(block).toContain('top: 3px')
    expect(block).toContain('right: 3px')
    expect(block).toContain('font-size: 10px')
    expect(block).toContain('font-weight: 700')
    expect(block).toContain('padding: 0 4px')
    expect(block).toContain('border-radius: 4px')
    expect(block).toContain('border: 1px solid var(--color-border-medium)')
    expect(block).toContain('background: var(--color-bg-elevated)')
    expect(block).toContain('box-shadow: 0 1px 2px rgba(0, 0, 0, 0.16)')
    expect(block).toContain('color: var(--color-text-secondary)')
    expect(block).not.toContain('--color-text-muted')
    // 定位基准：封面槽必须是定位容器，否则绝对定位会跑到整行右上（压住收藏星/状态图标）
    const coverStart = css.indexOf('.article-row .row-cover {')
    expect(coverStart).toBeGreaterThan(-1)
    const coverBlock = css.slice(coverStart, css.indexOf('}', coverStart))
    expect(coverBlock).toContain('position: relative')
    // 虚拟列表行高假设不变：行仍是固定 100px
    expect(css).toContain('height: 100px')
  })
})
