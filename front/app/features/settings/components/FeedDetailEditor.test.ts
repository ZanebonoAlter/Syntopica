/**
 * FeedDetailEditor — unify-feed-summary-toggles FE-1..FE-5。
 *
 * 词汇表统一渲染（四 toggle 名称/说明、无「内容补全」旧名）、切换 toggle 走
 * update-feed 单字段 PATCH 事件、总结重试上限 select、RSS 地址编辑保存。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedDetailEditor from './FeedDetailEditor.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import type { RssFeed } from '~/types'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))
vi.mock('~/components/feed/FeedIcon.vue', () => ({
  default: { name: 'FeedIcon', template: '<i />' },
}))
vi.mock('./FeedSourceQualityBlock.vue', () => ({
  default: { name: 'FeedSourceQualityBlock', template: '<div />' },
}))

function createFeed(overrides: Partial<RssFeed> = {}): RssFeed {
  return {
    id: '7',
    title: 'Test Feed',
    description: '',
    url: 'https://example.com/feed.xml',
    category: '',
    articleCount: 3,
    unreadCount: 0,
    articleSummaryEnabled: true,
    completionOnRefresh: false,
    maxCompletionRetries: 3,
    firecrawlEnabled: true,
    taggingEnabled: true,
    ...overrides,
  } as RssFeed
}

function mountEditor(feedOverrides: Partial<RssFeed> = {}) {
  return mount(FeedDetailEditor, {
    props: {
      feed: createFeed(feedOverrides),
      categories: [],
      refreshOptions: [{ label: '关闭', value: 0 }],
      maxArticlesOptions: [{ label: '100 篇', value: 100 }],
      loading: false,
    },
    global: {
      components: { AppToggle },
    },
  })
}

describe('FeedDetailEditor 词汇表与字段（unify-feed-summary-toggles）', () => {
  it('FE-1: 渲染词汇表四 toggle 名称与说明文案，无旧名「内容补全」', () => {
    const wrapper = mountEditor()
    const text = wrapper.text()

    expect(text).toContain('AI 总结')
    expect(text).toContain('为文章生成 AI 整理稿；开启后文章页可手动生成')
    expect(text).toContain('刷新后自动总结')
    expect(text).toContain('新文章自动排队总结；关闭后仅手动生成')
    expect(text).toContain('全文抓取')
    expect(text).toContain('用 Firecrawl 抓取完整正文供阅读，与总结独立')
    expect(text).toContain('AI 打标签')
    expect(text).toContain('自动为文章生成主题标签')

    // 旧名不得再出现
    expect(text).not.toContain('内容补全')
    expect(text).not.toContain('刷新时自动补全')
    expect(wrapper.findAll('.feed-detail__toggle-row')).toHaveLength(4)
  })

  it('FE-2: 切换「AI 总结」toggle → update-feed(article_summary_enabled)', async () => {
    const wrapper = mountEditor()
    const toggles = wrapper.findAllComponents(AppToggle)
    // toggle 顺序与模板一致：AI 总结 → 刷新后自动总结 → 全文抓取 → AI 打标签
    await toggles[0]!.find('.app-toggle__track').trigger('click')

    expect(wrapper.emitted('update-feed')).toBeTruthy()
    expect(wrapper.emitted('update-feed')![0]).toEqual(['7', 'article_summary_enabled', false])
  })

  it('FE-3: 切换「刷新后自动总结」toggle → update-feed(completion_on_refresh)', async () => {
    const wrapper = mountEditor({ completionOnRefresh: false })
    const toggles = wrapper.findAllComponents(AppToggle)
    await toggles[1]!.find('.app-toggle__track').trigger('click')

    expect(wrapper.emitted('update-feed')![0]).toEqual(['7', 'completion_on_refresh', true])
  })

  it('FE-4: 总结重试上限 select 变更 → update-feed(max_completion_retries)', async () => {
    const wrapper = mountEditor({ maxCompletionRetries: 3 })
    const selects = wrapper.findAll('select')
    // 表单 select 顺序：分类 → 自动刷新 → 最大文章数 → 总结重试上限
    const retrySelect = selects[3]!
    await retrySelect.setValue('5')

    expect(wrapper.emitted('update-feed')![0]).toEqual(['7', 'max_completion_retries', 5])
  })

  it('FE-5: RSS 地址编辑后保存 → update-feed(url)；未修改时保存按钮禁用', async () => {
    const wrapper = mountEditor()
    const input = wrapper.find('input[type="url"]')
    expect((input.element as HTMLInputElement).value).toBe('https://example.com/feed.xml')

    // 未修改：保存按钮禁用
    const saveBtn = wrapper.findAll('button').find(b => b.text() === '保存')!
    expect(saveBtn.attributes('disabled')).toBeDefined()

    await input.setValue('https://example.com/renamed.xml')
    await saveBtn.trigger('click')

    expect(wrapper.emitted('update-feed')![0]).toEqual(['7', 'url', 'https://example.com/renamed.xml'])
  })
})
