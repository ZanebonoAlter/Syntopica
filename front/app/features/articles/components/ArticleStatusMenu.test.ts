import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Article } from '~/types'

/**
 * ArticleStatusMenu（redesign-reading-pane task 2.2）：
 * 状态图标四态（语义同 reading-list-panel）、处理详情浮层开合（点开/再点/点外/Esc）、
 * ⋯ 菜单项按 feed 能力显隐、busy 禁用、双源切换控件条件渲染、emit 事件断言。
 *
 * emit 断言用 attrs 事件 spy——emitted() 捕获链在 vue3.5+VTU 组合下失效
 * （同 ArticleCardView.test.ts 已验证的口径，走真实 Vue 事件传播）。
 */

vi.mock('@iconify/vue', () => ({
  Icon: {
    name: 'Icon',
    inheritAttrs: true,
    props: ['icon'],
    template: '<span class="icon-stub" :data-icon="icon" aria-hidden="true" />',
  },
}))

import ArticleStatusMenu from './ArticleStatusMenu.vue'

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
    tagCount: 3,
    ...over,
  }
}

function baseProps(over: Record<string, unknown> = {}) {
  return {
    article: article(),
    showManualFirecrawlAction: true,
    showManualSummaryAction: true,
    showManualTaggingAction: true,
    actionBusy: false,
    manualFirecrawlLoading: false,
    manualSummaryLoading: false,
    manualTaggingLoading: false,
    manualFirecrawlLabel: '手动抓取全文',
    manualSummaryLabel: '手动生成总结',
    manualTaggingLabel: '手动打标签',
    manualActionError: null,
    taggingError: null,
    detailLines: ['抓取时间：2026-09-18 10:00', '总结时间：2026-09-18 10:05'],
    showContentSourceToggle: false,
    activeContentSource: 'firecrawl',
    ...over,
  }
}

describe('ArticleStatusMenu', () => {
  it('renders the four pipeline states with matching icon shape and color', () => {
    const cases = [
      { over: { firecrawlStatus: 'pending', summaryStatus: 'incomplete' }, title: '处理状态：排队中', icon: 'mdi:clock-outline', color: 'var(--color-warning)' },
      { over: { firecrawlStatus: 'processing' }, title: '处理状态：进行中', icon: 'mdi:loading', color: 'var(--color-info)', spin: true },
      { over: { firecrawlStatus: 'failed' }, title: '处理状态：失败', icon: 'mdi:alert-circle', color: 'var(--color-error)' },
      { over: { firecrawlStatus: 'completed', summaryStatus: 'complete' }, title: '处理状态：完成', icon: 'mdi:check-circle', color: 'var(--color-text-muted)' },
    ] as const

    for (const c of cases) {
      const wrapper = mount(ArticleStatusMenu, { props: baseProps({ article: article({ ...c.over }) }) })
      const btn = wrapper.find('.status-btn')
      expect(btn.exists(), c.title).toBe(true)
      expect(btn.attributes('title')).toBe(c.title)
      const icon = btn.find('.icon-stub')
      expect(icon.attributes('data-icon')).toBe(c.icon)
      expect(icon.attributes('style')).toContain(c.color)
      if ('spin' in c && c.spin) {
        expect(icon.classes()).toContain('animate-spin')
      } else {
        expect(icon.classes()).not.toContain('animate-spin')
      }
      wrapper.unmount()
    }
  })

  it('hides the status icon when there is no processing signal at all', () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({
        article: article({ firecrawlStatus: undefined, summaryStatus: undefined, tagCount: 0 }),
        showManualFirecrawlAction: false,
        showManualSummaryAction: false,
        detailLines: [],
      }),
    })
    expect(wrapper.find('.status-btn').exists()).toBe(false)
  })

  it('opens the detail popover with three rows and meta lines, closes on second click / outside / Esc', async () => {
    const wrapper = mount(ArticleStatusMenu, { props: baseProps() })

    await wrapper.find('.status-btn').trigger('click')
    const pop = wrapper.find('.asm-status-pop')
    expect(pop.exists()).toBe(true)
    expect(pop.text()).toContain('处理详情')
    expect(pop.text()).toContain('全文抓取')
    expect(pop.text()).toContain('AI 总结')
    expect(pop.text()).toContain('标签')
    expect(pop.text()).toContain('已标记 3')
    expect(pop.text()).toContain('抓取时间：2026-09-18 10:00')

    // 再点图标 → 关闭
    await wrapper.find('.status-btn').trigger('click')
    expect(wrapper.find('.asm-status-pop').exists()).toBe(false)

    // 点开 → 点外部 → 关闭
    await wrapper.find('.status-btn').trigger('click')
    expect(wrapper.find('.asm-status-pop').exists()).toBe(true)
    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.asm-status-pop').exists()).toBe(false)

    // 点开 → Esc → 关闭
    await wrapper.find('.status-btn').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.asm-status-pop').exists()).toBe(false)
  })

  it('shows the full error text in the popover when processing failed', async () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ article: article({ firecrawlStatus: 'failed', firecrawlError: '抓取超时：上游服务 504' }) }),
    })
    await wrapper.find('.status-btn').trigger('click')
    const pop = wrapper.find('.asm-status-pop')
    expect(pop.text()).toContain('抓取失败')
    expect(pop.find('.asm-error').text()).toContain('抓取超时：上游服务 504')
  })

  it('opens the ⋯ menu with items by capability and closes it via outside click', async () => {
    const wrapper = mount(ArticleStatusMenu, { props: baseProps() })
    const moreBtn = wrapper.find('[aria-label="更多操作"]')
    expect(moreBtn.exists()).toBe(true)

    await moreBtn.trigger('click')
    const menu = wrapper.find('.asm-menu-pop')
    expect(menu.exists()).toBe(true)
    expect(menu.text()).toContain('手动抓取全文')
    expect(menu.text()).toContain('手动生成总结')
    expect(menu.text()).toContain('手动打标签')
    // 无双源时切换控件不出现
    expect(menu.find('.asm-seg').exists()).toBe(false)

    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.asm-menu-pop').exists()).toBe(false)
  })

  it('hides the ⋯ button entirely when no manual action and no source toggle is available', () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showManualFirecrawlAction: false, showManualSummaryAction: false, showManualTaggingAction: false }),
    })
    expect(wrapper.find('[aria-label="更多操作"]').exists()).toBe(false)
  })

  it('disables all manual items while a manual action is busy', async () => {
    const wrapper = mount(ArticleStatusMenu, { props: baseProps({ actionBusy: true }) })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const items = wrapper.findAll('.asm-menu-item')
    expect(items.length).toBe(3)
    for (const item of items) expect(item.attributes('disabled')).toBeDefined()
  })

  it('renders the source segmented control only when both sources exist, and emits source change', async () => {
    const onSourceChange = vi.fn()
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showContentSourceToggle: true, activeContentSource: 'original' }),
      attrs: { 'onUpdate:activeContentSource': onSourceChange },
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const seg = wrapper.find('.asm-seg')
    expect(seg.exists()).toBe(true)
    expect(seg.text()).toContain('原始内容')
    expect(seg.text()).toContain('Firecrawl 全文')

    const buttons = seg.findAll('button')
    await buttons[1]!.trigger('click')
    expect(onSourceChange).toHaveBeenCalledWith('firecrawl')
  })

  it('emits manual-firecrawl / manual-summary / manual-tagging on item click', async () => {
    const onFirecrawl = vi.fn()
    const onSummary = vi.fn()
    const onTagging = vi.fn()
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps(),
      attrs: { onManualFirecrawl: onFirecrawl, onManualSummary: onSummary, onManualTagging: onTagging },
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const items = wrapper.findAll('.asm-menu-item')

    await items[0]!.trigger('click')
    expect(onFirecrawl).toHaveBeenCalledTimes(1)
    await items[1]!.trigger('click')
    expect(onSummary).toHaveBeenCalledTimes(1)
    await items[2]!.trigger('click')
    expect(onTagging).toHaveBeenCalledTimes(1)
  })

  it('shows inline manual action errors inside the menu popover', async () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ manualActionError: '手动抓取失败', taggingError: '提交标签任务失败' }),
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const errors = wrapper.findAll('.asm-menu-error')
    expect(errors.length).toBe(2)
    expect(errors[0]!.text()).toBe('手动抓取失败')
    expect(errors[1]!.text()).toBe('提交标签任务失败')
  })

  it('shows only one popover at a time (opening menu closes status popover)', async () => {
    const wrapper = mount(ArticleStatusMenu, { props: baseProps() })
    await wrapper.find('.status-btn').trigger('click')
    expect(wrapper.find('.asm-status-pop').exists()).toBe(true)

    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    expect(wrapper.find('.asm-status-pop').exists()).toBe(false)
    expect(wrapper.find('.asm-menu-pop').exists()).toBe(true)
  })

  // ===== 人工标记疑似 AI（add-ai-generated-article-detection）=====

  it('手动操作组含「标记为疑似 AI 生成」与「取消疑似 AI 标记」两项，点击分别发出 suspect / not_ai', async () => {
    const onDetection = vi.fn()
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showManualDetectionAction: true }),
      attrs: { onManualAiDetection: onDetection },
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const menu = wrapper.find('.asm-menu-pop')
    expect(menu.text()).toContain('标记为疑似 AI 生成')
    expect(menu.text()).toContain('取消疑似 AI 标记')

    const items = wrapper.findAll('.asm-menu-item')
    // 菜单尾三顶：标记 / 取消 / 重判（review R-6 新增第三项）
    const mark = items[items.length - 3]!
    const unmark = items[items.length - 2]!
    expect(mark.find('.icon-stub').attributes('data-icon')).toBe('mdi:robot-outline')
    expect(unmark.find('.icon-stub').attributes('data-icon')).toBe('mdi:check-circle-outline')

    await mark.trigger('click')
    expect(onDetection).toHaveBeenCalledTimes(1)
    expect(onDetection).toHaveBeenCalledWith('suspect')

    await unmark.trigger('click')
    expect(onDetection).toHaveBeenCalledTimes(2)
    expect(onDetection).toHaveBeenCalledWith('not_ai')
  })

  it('人工标记失败走菜单内联错误位（不弹窗、不做整页错误）', async () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showManualDetectionAction: true, manualActionError: '标记失败：服务不可用' }),
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const errors = wrapper.findAll('.asm-menu-error')
    expect(errors.length).toBe(1)
    expect(errors[0]!.text()).toContain('标记失败：服务不可用')
  })

  it('人工标记项默认不出现（宿主显式开才渲染）', async () => {
    const wrapper = mount(ArticleStatusMenu, { props: baseProps() })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    expect(wrapper.find('.asm-menu-pop').text()).not.toContain('疑似 AI')
  })

  // ===== 显式重新判定（review R-6）=====

  it('第三项「交由模型重新判定」：渲染 + 点击发出 recheck 事件（不直接改判定字段）', async () => {
    const onRecheck = vi.fn()
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showManualDetectionAction: true }),
      attrs: { onRecheckAiDetection: onRecheck },
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const items = wrapper.findAll('.asm-menu-item')
    const recheck = items[items.length - 1]!
    expect(recheck.text()).toBe('交由模型重新判定')
    expect(recheck.find('.icon-stub').attributes('data-icon')).toBe('mdi:refresh')

    await recheck.trigger('click')
    expect(onRecheck).toHaveBeenCalledTimes(1)
  })

  it('重判 pending 时按钮置灰（不重复入队）', async () => {
    const wrapper = mount(ArticleStatusMenu, {
      props: baseProps({ showManualDetectionAction: true, manualRecheckLoading: true }),
    })
    await wrapper.find('[aria-label="更多操作"]').trigger('click')
    const items = wrapper.findAll('.asm-menu-item')
    const recheck = items[items.length - 1]!
    expect(recheck.text()).toBe('交由模型重新判定')
    expect(recheck.attributes('disabled')).toBeDefined()
  })
})
