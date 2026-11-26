import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ArchiveMatchPopover from './ArchiveMatchPopover.vue'
import type { BoardArticleTag, RelatedEventsResponse } from '~/api/semanticBoards'
import type { ArchiveArticle } from '../../composables/useBoardArchive'

// 用例来源：openspec/changes/enable-story-cross-day-continuation/test-cases.md
// 故事 S1 步 3/5 + 变体「可用性」：按需展开、接口排序信后端、描述可展开、
// 来源走既有阅读链路、空/部分/未就绪/失败可分辨、迟到响应丢弃、非 event 不请求。

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const { sbApi, articlesApi } = vi.hoisted(() => ({
  sbApi: {
    getMatchDetail: vi.fn(),
    getTagAuxiliaryLabels: vi.fn(),
    getMatchingConfig: vi.fn(),
    getComposition: vi.fn(),
    getRelatedEvents: vi.fn(),
  },
  articlesApi: {
    getArticle: vi.fn(),
  },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => sbApi }
})

vi.mock('~/api/articles', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/articles')>()
  return { ...original, useArticlesApi: () => articlesApi }
})

function tag(overrides: Partial<BoardArticleTag> = {}): BoardArticleTag {
  return {
    id: 7,
    label: '美伊关系',
    category: 'event',
    description: '伊朗与美国之间的双边关系与对抗线。',
    match_reason: 'direct_hit',
    score: 0.92,
    direction_mismatch: false,
    ...overrides,
  }
}

function article(): ArchiveArticle {
  return {
    id: 11,
    title: '伊朗与美国的对抗线',
    url: 'https://example.com/11',
    link: 'https://example.com/11',
    pub_date: '2026-10-23 08:00:00',
    feed_id: 1,
    feed_name: '测试来源',
    filtered_tags: [tag()],
    in_report: false,
    weak_evidence: false,
  }
}

function relatedResponse(overrides: Partial<RelatedEventsResponse> = {}): RelatedEventsResponse {
  return {
    board_id: 1974,
    tag_id: 7,
    status: 'ready',
    threshold: 0.82,
    window_days: 30,
    window_start: '2026-10-11T00:00:00Z',
    window_end: '2026-11-10T00:00:00Z',
    candidate_count: 3,
    indexed_count: 3,
    representation_version: 'related-event-v1',
    items: [
      {
        tag_id: 21,
        label: '美伊核谈判重启',
        description: '双方在阿曼重启核谈判的进程。',
        similarity: 0.93,
        source_article: { id: 501, title: '美伊重启核谈判' },
      },
      {
        tag_id: 22,
        label: '霍尔木兹海峡局势',
        description: '海峡通航与军事对峙。',
        similarity: 0.88,
        source_article: null,
      },
    ],
    ...overrides,
  }
}

function mountPopover(props: Record<string, unknown> = {}) {
  return mount(ArchiveMatchPopover, {
    props: {
      boardId: 1974,
      mode: 'tag',
      article: article(),
      tag: tag(),
      anchor: document.createElement('button'),
      ...props,
    },
    attachTo: document.body,
  })
}

const popEl = () => document.body.querySelector('[data-testid="archive-match-popover"]') as HTMLElement | null
const q = (testId: string) => popEl()!.querySelector(`[data-testid="${testId}"]`) as HTMLElement | null
const qa = (testId: string) => Array.from(popEl()!.querySelectorAll(`[data-testid="${testId}"]`)) as HTMLElement[]

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  sbApi.getMatchDetail.mockResolvedValue({ success: true, data: null })
  sbApi.getTagAuxiliaryLabels.mockResolvedValue({ success: true, data: { total: 0, items: [] } })
  sbApi.getMatchingConfig.mockResolvedValue({ success: true, data: { semantic_board_match_sim_threshold: 0.72 } })
  sbApi.getComposition.mockResolvedValue({ success: true, data: { total: 0, items: [] } })
  sbApi.getRelatedEvents.mockResolvedValue({ success: true, data: relatedResponse() })
  articlesApi.getArticle.mockResolvedValue({ success: true, data: { tags: [] } })
})

describe('ArchiveMatchPopover — 相关事件（S1）', () => {
  it('默认不请求；展开才查一次，且按接口顺序展示名称与相似度', async () => {
    mountPopover()
    await flushPromises()
    expect(sbApi.getRelatedEvents).not.toHaveBeenCalled()
    expect(q('archive-pop-related-toggle')).not.toBeNull()

    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(sbApi.getRelatedEvents).toHaveBeenCalledTimes(1)
    expect(sbApi.getRelatedEvents).toHaveBeenCalledWith(1974, 7)
    const items = qa('archive-pop-related-item')
    expect(items).toHaveLength(2)
    expect(items[0]!.textContent).toContain('美伊核谈判重启')
    expect(items[0]!.textContent).toContain('0.93')
    expect(items[1]!.textContent).toContain('霍尔木兹海峡局势')
    expect(items[1]!.textContent).toContain('0.88')
    expect(q('archive-pop-related-meta')!.textContent).toContain('阈值 0.82')
  })

  it('再次点击收起，不重复请求', async () => {
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()
    expect(q('archive-pop-related-item')).toBeNull()
    expect(sbApi.getRelatedEvents).toHaveBeenCalledTimes(1)
  })

  it('条目可展开完整描述', async () => {
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(q('archive-pop-related-desc')).toBeNull()
    qa('archive-pop-related-item')[0]!.querySelector('.amp-related-name')!.dispatchEvent(new Event('click'))
    await flushPromises()
    expect(q('archive-pop-related-desc')!.textContent).toContain('双方在阿曼重启核谈判的进程')
  })

  it('查看来源 → emit open-article（复用既有阅读链路）；无来源禁用且不伪造 ID', async () => {
    const wrapper = mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    const sources = qa('archive-pop-related-source')
    expect(sources[0]!.textContent).toContain('查看来源')
    expect(sources[1]!.textContent).toContain('暂无可读来源')
    expect((sources[1] as HTMLButtonElement).disabled).toBe(true)

    sources[0]!.click()
    await flushPromises()
    expect(wrapper.emitted('open-article')).toEqual([[501]])
  })

  it('非 event 标签卡不显示入口、不发请求', async () => {
    mountPopover({ tag: tag({ category: 'person', label: '某人物' }) })
    await flushPromises()
    expect(q('archive-pop-related-toggle')).toBeNull()
    expect(sbApi.getRelatedEvents).not.toHaveBeenCalled()
  })

  it('入板过程模式不发相关事件请求', async () => {
    mountPopover({ mode: 'article', tag: null })
    await flushPromises()
    expect(q('archive-pop-related-toggle')).toBeNull()
    expect(sbApi.getRelatedEvents).not.toHaveBeenCalled()
  })

  it('ready 空集给出可解释空态', async () => {
    sbApi.getRelatedEvents.mockResolvedValue({
      success: true,
      data: relatedResponse({ items: [], candidate_count: 0, indexed_count: 0 }),
    })
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(q('archive-pop-related-empty')!.textContent).toContain('暂无相关事件')
    expect(q('archive-pop-related-item')).toBeNull()
  })

  it('partial 列出现有结果并标注覆盖数', async () => {
    sbApi.getRelatedEvents.mockResolvedValue({
      success: true,
      data: relatedResponse({ status: 'partial', candidate_count: 5, indexed_count: 3 }),
    })
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(q('archive-pop-related-partial')!.textContent).toContain('部分事件尚未索引')
    expect(q('archive-pop-related-partial')!.textContent).toContain('3/5')
    expect(qa('archive-pop-related-item')).toHaveLength(2)
  })

  it('not_ready 明示索引未就绪，不冒充空集', async () => {
    sbApi.getRelatedEvents.mockResolvedValue({
      success: true,
      data: relatedResponse({ status: 'not_ready', reason: 'source_not_indexed', items: [], candidate_count: 0, indexed_count: 0 }),
    })
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(q('archive-pop-related-not-ready')!.textContent).toContain('索引尚未就绪')
    expect(q('archive-pop-related-empty')).toBeNull()
  })

  it('查询失败 → 局部错误 + 重试成功后可看列表', async () => {
    sbApi.getRelatedEvents.mockRejectedValueOnce(new Error('boom'))
    mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    expect(q('archive-pop-related-error')!.textContent).toContain('相关事件查询失败')
    expect(q('archive-pop-related-retry')).not.toBeNull()

    q('archive-pop-related-retry')!.click()
    await flushPromises()
    expect(qa('archive-pop-related-item')).toHaveLength(2)
  })

  it('上层阅读弹窗打开时 Esc 只关预览，不连带关掉标签卡', async () => {
    const wrapper = mountPopover()
    await flushPromises()

    const preview = document.createElement('div')
    preview.setAttribute('aria-modal', 'true')
    document.body.appendChild(preview)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()

    preview.remove()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('切换标签后迟到响应不覆盖新标签', async () => {
    let resolveFirst: (value: unknown) => void = () => {}
    sbApi.getRelatedEvents.mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
    sbApi.getRelatedEvents.mockResolvedValueOnce({
      success: true,
      data: relatedResponse({
        tag_id: 8,
        items: [{ tag_id: 31, label: '新标签的相关事件', description: '', similarity: 0.9, source_article: null }],
      }),
    })

    const wrapper = mountPopover()
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()

    await wrapper.setProps({ tag: tag({ id: 8, label: '另一事件' }) })
    await flushPromises()
    q('archive-pop-related-toggle')!.click()
    await flushPromises()
    expect(popEl()!.textContent).toContain('新标签的相关事件')

    resolveFirst({ success: true, data: relatedResponse() })
    await flushPromises()
    expect(popEl()!.textContent).toContain('新标签的相关事件')
    expect(popEl()!.textContent).not.toContain('美伊核谈判重启')
  })
})
