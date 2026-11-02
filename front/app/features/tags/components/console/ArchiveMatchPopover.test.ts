import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import ArchiveMatchPopover from './ArchiveMatchPopover.vue'
import type { BoardArticleTag, MatchDetailResponse } from '~/api/semanticBoards'
import type { ArchiveArticle } from '../../composables/useBoardArchive'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const { sbApi, articlesApi } = vi.hoisted(() => ({
  sbApi: {
    getMatchDetail: vi.fn(),
    getTagAuxiliaryLabels: vi.fn(),
    getMatchingConfig: vi.fn(),
    getComposition: vi.fn(),
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
    downgraded: false,
    direction_mismatch: false,
    ...overrides,
  }
}

function detail(overrides: Partial<MatchDetailResponse> = {}): MatchDetailResponse {
  return {
    topic_tag_id: 7,
    topic_tag_label: '美伊关系',
    semantic_board_id: 1974,
    match_reason: 'direct_hit',
    score: 0.92,
    downgraded: false,
    direction_mismatch: false,
    direction_sim: 0.8,
    effective_min_hits: 2,
    config: {
      sim_threshold: 0.72,
      hit_rate_sim_blend: 0.7,
      min_effective_sample: 3,
      direct_hit_rate: 0.5,
      direct_max_sim: 0.8,
      direct_max_sim_min_hits: 2,
      direct_max_sim_min_hit_rate: 0.3,
      direct_hit_min_overlap: 2,
      direction_sim_threshold: 0.5,
      direct_hit_score_factor: 0.7,
      weight_sim: 0.6,
      weight_density: 0.4,
      weighted_threshold: 0.6,
    },
    direct_hit_auxiliaries: [
      { tag_auxiliary_id: 101, tag_label: '伊朗', board_auxiliary_id: 201, board_label: '伊朗' },
    ],
    composite_hits: undefined,
    tag_auxiliary_count: 3,
    hits: 2,
    hit_rate: 0.667,
    max_similarity: 0.84,
    pairs: [
      {
        tag_auxiliary_id: 101,
        tag_auxiliary_label: '伊朗',
        board_auxiliary_id: 201,
        board_auxiliary_label: '伊朗',
        similarity: 0.84,
        is_hit: true,
      },
      {
        tag_auxiliary_id: 102,
        tag_auxiliary_label: '抵抗之弧',
        board_auxiliary_id: 202,
        board_auxiliary_label: '中东战争',
        similarity: 0.4,
        is_hit: false,
      },
    ],
    ...overrides,
  }
}

function article(overrides: Partial<ArchiveArticle> = {}): ArchiveArticle {
  return {
    id: 11,
    title: '伊朗与美国的对抗线',
    url: '',
    link: 'https://example.com/11',
    pub_date: '2026-10-23 08:00:00',
    feed_id: 1,
    feed_name: '测试源',
    filtered_tags: [tag()],
    in_report: true,
    weak_evidence: false,
    ...overrides,
  }
}

function articlePayload(tags: Array<{ id: number, slug: string, label: string, category: string }>) {
  return {
    id: 11,
    feed_id: 1,
    title: '伊朗与美国的对抗线',
    link: 'https://example.com/11',
    pub_date: '2026-10-23 08:00:00',
    created_at: '2026-10-23 08:00:00',
    read: false,
    favorite: false,
    tags,
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

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  sbApi.getMatchDetail.mockResolvedValue({ success: true, data: detail() })
  sbApi.getTagAuxiliaryLabels.mockResolvedValue({
    success: true,
    data: {
      total: 2,
      items: [
        { id: 101, label: '伊朗', slug: 'iran', aliases: [], ref_count: 3, description: '', display_order: 0, source: 'llm', status: 'active', protected: false },
        { id: 102, label: '抵抗之弧', slug: 'arc', aliases: [], ref_count: 1, description: '', display_order: 0, source: 'llm', status: 'active', protected: false },
      ],
    },
  })
  sbApi.getMatchingConfig.mockResolvedValue({ success: true, data: { semantic_board_match_sim_threshold: 0.72 } })
  sbApi.getComposition.mockResolvedValue({ success: true, data: { total: 1, items: [{ id: 101, label: '伊朗', slug: 'iran', aliases: [], ref_count: 1, description: '', display_order: 0, source: 'llm', status: 'active', protected: false }] } })
  articlesApi.getArticle.mockResolvedValue({
    success: true,
    data: articlePayload([
      { id: 7, slug: 'us-iran', label: '美伊关系', category: 'event' },
      { id: 9, slug: 'saudi', label: '沙特', category: 'keyword' },
    ]),
  })
})

describe('ArchiveMatchPopover — 标签卡模式', () => {
  it('头区显示标签名、类别、规则与分数；描述区标题随类别自适应', async () => {
    mountPopover()
    await flushPromises()
    const head = popEl()!.querySelector('[data-testid="archive-pop-head"]')!
    expect(head.textContent).toContain('美伊关系')
    expect(head.textContent).toContain('事件')
    expect(head.textContent).toContain('直接命中')
    expect(head.textContent).toContain('0.92')
    const desc = popEl()!.querySelector('[data-testid="archive-pop-desc"]')!
    expect(desc.textContent).toContain('伊朗与美国之间的双边关系')
    expect(popEl()!.textContent).toContain('事件描述')
  })

  it('类别是关键词时，描述区标题跟随类别', async () => {
    mountPopover({ tag: tag({ category: 'keyword', label: '沙特' }) })
    await flushPromises()
    expect(popEl()!.textContent).toContain('关键词描述')
  })

  it('辅助标签×版块收编：逐对标 ✓/✗ 并给最相似对与收编 M/N', async () => {
    mountPopover()
    await flushPromises()
    const rows = [...popEl()!.querySelectorAll('[data-testid="archive-pop-aux-row"]')] as HTMLElement[]
    expect(rows).toHaveLength(2)
    expect(rows[0]?.textContent).toContain('伊朗')
    expect(rows[0]?.textContent).toContain('✓')
    expect(rows[0]?.textContent).toContain('0.84')
    expect(rows[1]?.textContent).toContain('抵抗之弧')
    expect(rows[1]?.textContent).toContain('✗')
    expect(popEl()!.querySelector('[data-testid="archive-pop-aux-head"]')!.textContent).toContain('版块收编 1/2')
  })

  it('私有判定行标注第几步即命中，并指向通用流程入口', async () => {
    mountPopover()
    await flushPromises()
    const verdict = popEl()!.querySelector('[data-testid="archive-pop-verdict"]')!.textContent ?? ''
    expect(verdict).toContain('第 ① 步即命中')
    expect(verdict).toContain('直接命中 0.92')
    expect(verdict).toContain('匹配流程')
  })

  it('规则代入式默认收起，点开后就地展开（不渲染到 popover 外）', async () => {
    mountPopover()
    await flushPromises()
    expect(popEl()!.querySelector('[data-testid="archive-pop-calc"]')).toBeNull()
    const toggle = popEl()!.querySelector('[data-testid="archive-pop-calc-toggle"]') as HTMLElement
    toggle.click()
    await flushPromises()
    expect(popEl()!.querySelector('[data-testid="archive-pop-calc"]')).not.toBeNull()
    expect(popEl()!.querySelector('[data-testid="archive-pop-calc"]')!.textContent).toContain('score')
  })

  it('代入式与私有判定行标注实时重算 / 冻结 score 的口径', async () => {
    mountPopover()
    await flushPromises()
    const toggle = popEl()!.querySelector('[data-testid="archive-pop-calc-toggle"]') as HTMLElement
    toggle.click()
    await flushPromises()
    const calc = popEl()!.querySelector('[data-testid="archive-pop-calc"]')!.textContent ?? ''
    expect(calc).toContain('实时计算')
    expect(calc).toContain('冻结')
    const verdict = popEl()!.querySelector('[data-testid="archive-pop-verdict"]')!.textContent ?? ''
    expect(verdict).toContain('实时计算')
    expect(verdict).toContain('冻结')
  })

  it('实时重算值与冻结 score 不一致时如实提示', async () => {
    sbApi.getMatchDetail.mockResolvedValue({
      success: true,
      data: detail({ match_reason: 'weighted', score: 0.92, max_similarity: 0.84, hit_rate: 0.667 }),
    })
    mountPopover({ tag: tag({ match_reason: 'weighted', score: 0.92 }) })
    await flushPromises()
    const text = popEl()!.textContent ?? ''
    expect(text).toContain('重算值与冻结 score 不一致')
  })

  it('本文标签区展示文章其余标签（category 如实标注；当前标签在头部，不重复）', async () => {
    mountPopover()
    await flushPromises()
    const tags = popEl()!.querySelector('[data-testid="archive-pop-article-tags"]')!
    expect(tags.textContent).toContain('沙特')
    expect(tags.textContent).toContain('keyword')
    expect(tags.textContent).not.toContain('美伊关系')
    expect(popEl()!.querySelector('[data-testid="archive-pop-head"]')!.textContent).toContain('美伊关系')
  })

  it('composite_hit 标签展示真实组合链', async () => {
    sbApi.getMatchDetail.mockResolvedValue({
      success: true,
      data: detail({
        match_reason: 'composite_hit',
        score: 1,
        composite_hits: [
          {
            id: 9,
            label: '伊美关系',
            components: [
              { id: 11, label: '伊朗', position: 2 },
              { id: 12, label: '美国', position: 1 },
            ],
          },
        ],
      }),
    })
    mountPopover({ tag: tag({ match_reason: 'composite_hit', score: 1 }) })
    await flushPromises()
    const chain = popEl()!.querySelector('[data-testid="archive-pop-composite"]')!.textContent ?? ''
    expect(chain).toContain('伊美关系 = 美国 × 伊朗')
  })

  it('match-detail 不可得时如实标注缺失，不伪造逐对判定', async () => {
    sbApi.getMatchDetail.mockResolvedValue({ success: false, error: '挂了' })
    mountPopover()
    await flushPromises()
    const text = popEl()!.textContent ?? ''
    expect(text).toContain('未取到')
    expect(text).toContain('实时计算')
  })

  it('match-detail 不可得时：id 与版块构成直接重合的辅助标签标 ✓，其余标 ·', async () => {
    sbApi.getMatchDetail.mockResolvedValue({ success: false, error: '挂了' })
    mountPopover()
    await flushPromises()
    const rows = [...popEl()!.querySelectorAll('[data-testid="archive-pop-aux-row"]')] as HTMLElement[]
    expect(rows[0]?.textContent).toContain('伊朗')
    expect(rows[0]?.textContent).toContain('✓')
    expect(rows[1]?.textContent).toContain('抵抗之弧')
    expect(rows[1]?.textContent).toContain('·')
  })
})

describe('ArchiveMatchPopover — 入板过程模式', () => {
  const flowArticle = article({
    filtered_tags: [
      tag({ id: 9, label: '沙特', match_reason: 'hit_rate', score: 0.78 }),
      tag({ id: 10, label: '霍尔木兹', match_reason: 'max_sim', score: 0.5 }),
    ],
  })

  it('五阶段分组列出各标签得分，最强标签标「入板依据」', async () => {
    mountPopover({ mode: 'article', tag: null, article: flowArticle })
    await flushPromises()
    const stages = [...popEl()!.querySelectorAll('[data-testid^="archive-pop-stage-"]')] as HTMLElement[]
    const rateStage = stages.find(node => node.getAttribute('data-testid') === 'archive-pop-stage-hit_rate')!
    expect(rateStage.textContent).toContain('沙特')
    expect(rateStage.textContent).toContain('入板依据')
    expect(rateStage.textContent).toContain('0.78')
  })

  it('命中步之前的空阶段给未命中原因（真实阈值 + 未达），命中步之后标「未走到」', async () => {
    sbApi.getMatchDetail.mockResolvedValue({ success: false, error: '挂了' })
    mountPopover({ mode: 'article', tag: null, article: flowArticle })
    await flushPromises()
    const directStage = popEl()!.querySelector('[data-testid="archive-pop-stage-direct_hit"]')!
    const directWhy = directStage.querySelector('[data-testid="archive-pop-stage-why"]')!.textContent ?? ''
    expect(directWhy).toContain('需辅助标签交集 ≥ 2')
    expect(directWhy).toContain('未达')
    const skipped = [...popEl()!.querySelectorAll('[data-testid="archive-pop-stage-skipped"]')] as HTMLElement[]
    expect(skipped.length).toBeGreaterThan(0)
    expect(skipped[0]?.textContent).toContain('未走到（第 ③ 步命中即停）')
  })

  it('match-detail 可得时，空阶段给真实差距数值而非阈值模板', async () => {
    mountPopover({ mode: 'article', tag: null, article: flowArticle })
    await flushPromises()
    const directStage = popEl()!.querySelector('[data-testid="archive-pop-stage-direct_hit"]')!
    const directWhy = directStage.querySelector('[data-testid="archive-pop-stage-why"]')!.textContent ?? ''
    expect(directWhy).toContain('交集 1 个 < 2')
  })

  it('末行列出本文全部标签 chips', async () => {
    mountPopover({ mode: 'article', tag: null, article: flowArticle })
    await flushPromises()
    const tags = popEl()!.querySelector('[data-testid="archive-pop-article-tags"]')!
    expect(tags.textContent).toContain('美伊关系')
    expect(tags.textContent).toContain('沙特')
  })

  it('弱证据文章：头区标「仅弱证据」，徽标为「弱证据命中」且不给「入板依据」', async () => {
    const weakArticle = article({
      weak_evidence: true,
      filtered_tags: [tag({ id: 9, label: '沙特', match_reason: 'max_sim', score: 0.5, downgraded: true })],
    })
    mountPopover({ mode: 'article', tag: null, article: weakArticle })
    await flushPromises()
    const head = popEl()!.querySelector('[data-testid="archive-pop-head"]')!.textContent ?? ''
    expect(head).toContain('仅弱证据')
    const text = popEl()!.textContent ?? ''
    expect(text).toContain('弱证据命中（不计入强证据）')
    expect(text).not.toContain('入板依据')
  })

  it('无命中标签的文章：弹窗说明这类文章不会进入日报', async () => {
    // 本文一个 topic_tag 都没有 → 弹窗走「本文未打标签」分支（并说明不进日报口径）
    articlesApi.getArticle.mockResolvedValue({ success: true, data: articlePayload([]) })
    mountPopover({ mode: 'article', tag: null, article: article({ filtered_tags: [] }) })
    await flushPromises()
    const text = popEl()!.textContent ?? ''
    expect(text).toContain('本文未打标签')
    expect(text).toContain('无命中标签的文章不会进入日报')
  })
})

describe('ArchiveMatchPopover — 关闭与按需请求', () => {
  it('Esc 关闭并向外 emit close', async () => {
    const wrapper = mountPopover()
    await flushPromises()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('点遮罩关闭', async () => {
    const wrapper = mountPopover()
    await flushPromises()
    const mask = document.body.querySelector('[data-testid="archive-pop-mask"]') as HTMLElement
    mask.click()
    await flushPromises()
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('打开才请求：挂载后拉取文章/辅助标签/match-detail', async () => {
    mountPopover()
    await flushPromises()
    expect(articlesApi.getArticle).toHaveBeenCalledWith(11)
    expect(sbApi.getTagAuxiliaryLabels).toHaveBeenCalledWith(7)
    expect(sbApi.getMatchDetail).toHaveBeenCalledWith(1974, 7)
  })

  it('竞态保护：后发请求先到时，旧的慢响应不覆盖新内容', async () => {
    let resolveFirst: (value: unknown) => void = () => {}
    sbApi.getTagAuxiliaryLabels
      .mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockResolvedValueOnce({
        success: true,
        data: { total: 1, items: [{ id: 201, label: '新标签', slug: 'new', aliases: [], ref_count: 0, description: '', display_order: 0, source: 'llm', status: 'active', protected: false }] },
      })

    const wrapper = mountPopover()
    await flushPromises()
    await wrapper.setProps({ tag: tag({ id: 8, label: '新事件' }) })
    await flushPromises()
    expect(popEl()!.textContent).toContain('新标签')

    // 旧请求此刻才返回：不得覆盖已渲染的新内容
    resolveFirst({
      success: true,
      data: { total: 1, items: [{ id: 101, label: '旧标签', slug: 'old', aliases: [], ref_count: 0, description: '', display_order: 0, source: 'llm', status: 'active', protected: false }] },
    })
    await flushPromises()
    expect(popEl()!.textContent).not.toContain('旧标签')
  })
})

describe('ArchiveMatchPopover — 浮层展示合理性锚', () => {
  it('遮罩是 fixed + inset 0 的全屏层，且 Teleport 挂到 body', async () => {
    const css = readFileSync(`${import.meta.dirname}/archiveMatchPopover.css`, 'utf8')
    expect(css).toMatch(/\.amp-mask\s*\{[^}]*position:\s*fixed/s)
    expect(css).toMatch(/\.amp-mask\s*\{[^}]*inset:\s*0/s)

    mountPopover()
    await flushPromises()
    expect(document.body.querySelector(':scope > [data-testid="archive-pop-mask"]')).not.toBeNull()
    expect(document.body.querySelector(':scope > [data-testid="archive-match-popover"]')).not.toBeNull()
  })
})
