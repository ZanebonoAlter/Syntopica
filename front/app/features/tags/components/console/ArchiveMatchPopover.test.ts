import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import ArchiveMatchPopover from './ArchiveMatchPopover.vue'
import { ruleOf } from './archiveMatchFlow'
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
  it('头区显示标签名、类别、第 N 步规则名与指标行；描述区标题随类别自适应', async () => {
    mountPopover()
    await flushPromises()
    const head = popEl()!.querySelector('[data-testid="archive-pop-head"]')!
    expect(head.textContent).toContain('美伊关系')
    expect(head.textContent).toContain('事件')
    expect(head.querySelector('[data-testid="archive-pop-head-step"]')!.textContent).toContain('第①步 精确匹配 命中')
    expect(head.textContent).toContain('0.92')
    expect(head.querySelector('[data-testid="archive-pop-metric"]')!.textContent).toContain('相似达标 2/3 · R 0.67 · Smax 0.84')
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

  /** M1/M2 场景：direct_hit 命中，交集 3、相似达标 5/5。 */
  function directHitDetail() {
    return detail({
      score: 0.7,
      tag_auxiliary_count: 5,
      hits: 5,
      hit_rate: 1,
      max_similarity: 0.86,
      direct_hit_auxiliaries: [
        { tag_auxiliary_id: 101, tag_label: '伊朗', board_auxiliary_id: 201, board_label: '伊朗' },
        { tag_auxiliary_id: 102, tag_label: '抵抗之弧', board_auxiliary_id: 202, board_label: '中东战争' },
        { tag_auxiliary_id: 103, tag_label: '霍尔木兹', board_auxiliary_id: 203, board_label: '霍尔木兹' },
      ],
    })
  }

  it('标签卡判定过程逐条呈现：结论行 + ① 命中 + ② 参照 + ③④⑤ 合并未走到，无空壳行', async () => {
    sbApi.getMatchDetail.mockResolvedValue({ success: true, data: directHitDetail() })
    mountPopover({ tag: tag({ score: 0.7 }) })
    await flushPromises()
    const pop = popEl()!
    // 结论行：一句话因果，不再有长判定句与「完整流程见工具行」前提
    const verdict = pop.querySelector('[data-testid="archive-pop-verdict"]')!.textContent ?? ''
    expect(verdict).toContain('本文 5 个辅助标签里 3 个与版块完全相同')
    expect(verdict).toContain('①精确匹配（≥ 2）命中，得分 0.70')
    expect(pop.textContent).not.toContain('完整流程')
    // ① 命中行：问法原文 + 交集口径词
    const direct = pop.querySelector('[data-testid="archive-pop-step-direct_hit"]')!
    expect(direct.textContent).toContain('命中')
    expect(direct.textContent).toContain('事件和版块有多少个辅助标签完全相同？')
    expect(direct.textContent).toContain('交集 3 个（完全相同）≥ 2')
    // ② 参照行：相似达标口径，与 ① 分家
    const similarity = pop.querySelector('[data-testid="archive-pop-step-similarity"]')!
    expect(similarity.textContent).toContain('参照')
    expect(similarity.textContent).toContain('相似达标 5/5')
    expect(similarity.textContent).toContain('R 1.00')
    expect(similarity.textContent).toContain('Smax 0.86')
    // ③④⑤ 合并一排「未走到」，无各步空壳行
    const skipped = pop.querySelector('[data-testid="archive-pop-steps-skipped"]')!
    expect(skipped.textContent).toContain('③④⑤ 未走到（①已命中即停）')
    expect(pop.querySelector('[data-testid="archive-pop-step-hit_rate"]')).toBeNull()
    expect(pop.querySelector('[data-testid="archive-pop-step-max_sim"]')).toBeNull()
    expect(pop.querySelector('[data-testid="archive-pop-step-weighted"]')).toBeNull()
  })

  it('命中步就地展开公式：按钮「这一步怎么算的」，KaTeX 在命中步容器内，收起不残留', async () => {
    mountPopover()
    await flushPromises()
    const pop = popEl()!
    expect(pop.querySelector('[data-testid="archive-pop-calc"]')).toBeNull()
    const toggle = pop.querySelector('[data-testid="archive-pop-calc-toggle"]') as HTMLElement
    expect(toggle.textContent).toContain('这一步怎么算的')
    toggle.click()
    await flushPromises()
    // 公式渲染在命中步（direct_hit）容器内，不在 popover 末尾或独立块
    const direct = pop.querySelector('[data-testid="archive-pop-step-direct_hit"]')!
    const calc = direct.querySelector('[data-testid="archive-pop-calc"]')
    expect(calc).not.toBeNull()
    expect(calc!.textContent).toContain('score')
    expect(calc!.textContent).toContain('R')
    toggle.click()
    await flushPromises()
    expect(pop.querySelector('[data-testid="archive-pop-calc"]')).toBeNull()
  })

  it('口径注释收敛为一处：实时重算 / 冻结 score 全 popover 仅一个注释节点', async () => {
    mountPopover()
    await flushPromises()
    const pop = popEl()!
    const notes = [...pop.querySelectorAll('[data-testid="archive-pop-live-note"]')] as HTMLElement[]
    expect(notes).toHaveLength(1)
    expect(notes[0]!.textContent).toContain('实时计算')
    expect(notes[0]!.textContent).toContain('冻结')
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

  it('收编与交集口径分家：收编标题标「相似 ≥ 阈值，非完全相同」，① 行标「（完全相同）」', async () => {
    mountPopover()
    await flushPromises()
    const auxHead = popEl()!.querySelector('[data-testid="archive-pop-aux-head"]')!.textContent ?? ''
    expect(auxHead).toContain('相似 ≥ 0.72，非完全相同')
    const direct = popEl()!.querySelector('[data-testid="archive-pop-step-direct_hit"]')!.textContent ?? ''
    expect(direct).toContain('（完全相同）')
    // 两个数字各自带口径词，不得被读作同一口径
    expect(auxHead).toContain('版块收编 1/2')
  })

  it('判定过程前置失败步给真实差距：① 未通过给「交集 1 个 < 2」，③ 命中给 R 实际数值', async () => {
    sbApi.getMatchDetail.mockResolvedValue({
      success: true,
      data: detail({ match_reason: 'hit_rate', hits: 3, hit_rate: 0.6, score: 0.66 }),
    })
    mountPopover({ tag: tag({ match_reason: 'hit_rate', score: 0.66 }) })
    await flushPromises()
    const pop = popEl()!
    const direct = pop.querySelector('[data-testid="archive-pop-step-direct_hit"]')!
    expect(direct.textContent).toContain('交集 1 个（完全相同）< 2')
    expect(direct.textContent).toContain('未通过')
    const rate = pop.querySelector('[data-testid="archive-pop-step-hit_rate"]')!
    expect(rate.textContent).toContain('R=0.600 > 0.50')
    const skipped = pop.querySelector('[data-testid="archive-pop-steps-skipped"]')!
    expect(skipped.textContent).toContain('④⑤ 未走到（③已命中即停）')
  })

  it('判定数值缺失：仍列规则名+问法+阈值，命中步标「需实时拉取」，不渲染公式入口', async () => {
    sbApi.getMatchDetail.mockResolvedValue({ success: false, error: '挂了' })
    mountPopover()
    await flushPromises()
    const pop = popEl()!
    const direct = pop.querySelector('[data-testid="archive-pop-step-direct_hit"]')!
    expect(direct.textContent).toContain('精确匹配')
    expect(direct.textContent).toContain('事件和版块有多少个辅助标签完全相同？')
    expect(direct.textContent).toContain('需交集 ≥ 2 个（完全相同）')
    expect(direct.textContent).toContain('需实时拉取')
    expect(pop.querySelector('[data-testid="archive-pop-calc-toggle"]')).toBeNull()
    expect(pop.textContent).toContain('未取到')
    // 结论行同步如实标注，不伪造数值
    expect(pop.querySelector('[data-testid="archive-pop-verdict"]')!.textContent).toContain('需实时拉取')
  })

  it('composite_hit 判定过程：⓪ 组合链命中 + ①②③④⑤ 合并未走到，无 ② 参照行，结论行不叠字', async () => {
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
    const pop = popEl()!
    const composite = pop.querySelector('[data-testid="archive-pop-step-composite_hit"]')!
    expect(composite.textContent).toContain('伊美关系')
    expect(composite.textContent).toContain('免方向校验')
    const skipped = pop.querySelector('[data-testid="archive-pop-steps-skipped"]')!
    expect(skipped.textContent).toContain('①②③④⑤ 未走到（⓪已命中即停）')
    expect(pop.querySelector('[data-testid="archive-pop-step-similarity"]')).toBeNull()
    // 头区指标行无相似度语义，数值如实给 —
    expect(pop.querySelector('[data-testid="archive-pop-metric"]')!.textContent).toContain('—')
    // 结论行给「⓪组合命中即停」，不与规则名尾部叠字（review R-1）
    const verdict = pop.querySelector('[data-testid="archive-pop-verdict"]')!.textContent ?? ''
    expect(verdict).toContain('「伊美关系 = 美国 × 伊朗」')
    expect(verdict).toContain('⓪组合命中即停，得分 1.00')
    expect(verdict).not.toContain('命中命中')
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

  it('空集：标签无辅助标签时收编区如实说明，不伪造行', async () => {
    sbApi.getTagAuxiliaryLabels.mockResolvedValue({ success: true, data: { total: 0, items: [] } })
    mountPopover()
    await flushPromises()
    const pop = popEl()!
    expect(pop.querySelector('[data-testid="archive-pop-aux-head"]')!.textContent).toContain('辅助标签 ×0')
    expect(pop.textContent).toContain('该标签无辅助标签')
    expect(pop.querySelectorAll('[data-testid="archive-pop-aux-row"]')).toHaveLength(0)
  })

  it('loading 时不渲染判定过程（数据到达前不出占位再跳变）', async () => {
    let resolveDetail: (value: unknown) => void = () => {}
    sbApi.getMatchDetail.mockImplementationOnce(() => new Promise(resolve => { resolveDetail = resolve }))
    mountPopover()
    await flushPromises()
    // 请求未返回：loading 态无判定过程/结论占位
    expect(popEl()!.querySelector('[data-testid="archive-pop-verdict-steps"]')).toBeNull()
    expect(popEl()!.querySelector('[data-testid="archive-pop-verdict"]')).toBeNull()
    expect(popEl()!.textContent).toContain('正在加载匹配证据')
    resolveDetail({ success: true, data: detail() })
    await flushPromises()
    expect(popEl()!.querySelector('[data-testid="archive-pop-verdict-steps"]')).not.toBeNull()
  })

  it('match-detail 失败后关闭重开：重新请求，成功则判定过程给真实数值', async () => {
    sbApi.getMatchDetail.mockResolvedValueOnce({ success: false, error: '挂了' })
    const first = mountPopover()
    await flushPromises()
    expect(popEl()!.textContent).toContain('需实时拉取')
    first.unmount()
    document.body.innerHTML = ''
    sbApi.getMatchDetail.mockResolvedValue({ success: true, data: detail({ hits: 5, tag_auxiliary_count: 5, hit_rate: 1, max_similarity: 0.86 }) })
    mountPopover()
    await flushPromises()
    const similarity = popEl()!.querySelector('[data-testid="archive-pop-step-similarity"]')!
    expect(similarity.textContent).toContain('相似达标 5/5 · R 1.00 · Smax 0.86')
  })
})

describe('ArchiveMatchPopover — 入板过程模式', () => {
  const flowArticle = article({
    filtered_tags: [
      tag({ id: 9, label: '沙特', match_reason: 'hit_rate', score: 0.78 }),
      tag({ id: 10, label: '霍尔木兹', match_reason: 'max_sim', score: 0.5 }),
    ],
  })

  it('五阶段分组列出各标签得分，阶段名与 RULE_COPY 常量源同源，最强标签标「入板依据」', async () => {
    mountPopover({ mode: 'article', tag: null, article: flowArticle })
    await flushPromises()
    const stages = [...popEl()!.querySelectorAll('[data-testid^="archive-pop-stage-"]')] as HTMLElement[]
    const rateStage = stages.find(node => node.getAttribute('data-testid') === 'archive-pop-stage-hit_rate')!
    expect(rateStage.textContent).toContain('沙特')
    expect(rateStage.textContent).toContain('入板依据')
    expect(rateStage.textContent).toContain('0.78')
    // 阶段名同源（spec「阶段命名 SHALL 与通用流程说明同源」）：编号+短名取 RULE_COPY
    const directStage = popEl()!.querySelector('[data-testid="archive-pop-stage-direct_hit"]')!
    expect(directStage.textContent).toContain(ruleOf('direct_hit')!.no)
    expect(directStage.textContent).toContain(ruleOf('direct_hit')!.short)
    const weightedStage = popEl()!.querySelector('[data-testid="archive-pop-stage-weighted"]')!
    expect(weightedStage.textContent).toContain(ruleOf('weighted')!.short)
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
