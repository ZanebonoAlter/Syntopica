import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import MarginNoteCard from './MarginNoteCard.vue'
import type { MarginNoteAnnotation } from '~/api/marginNotes'
import type { MarginNoteAskState } from '~/features/tags/composables/useMarginNotes'

// Icon stub — keeps the suite offline
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeNote(over: Partial<MarginNoteAnnotation> = {}): MarginNoteAnnotation {
  return {
    id: 5,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '以利率招标方式开展 3000 亿元 7 天期逆回购操作，中标利率维持 1.80% 不变。因今日有 1000 亿元逆回购到期，实现净投放 2000 亿元。本周累计净投放已超 8000 亿元，为近三个月单周最高。',
    anchor_offset_start: null,
    anchor_offset_end: null,
    created_at: '2026-09-22T00:00:00Z',
    qas: [
      {
        id: 9,
        annotation_id: 5,
        question: '逆回购是什么意思？',
        answer: '逆回购是央行向市场短期注入流动性的操作。',
        cited_article_ids: [101, 102, 103],
        cited_web_sources: [
          { title: '逆回购_财经百科', url: 'https://money.163.com/baike/x' },
        ],
        extracted_terms: [{ term: '逆回购', is_new: true }, { term: '流动性对冲', is_new: false }],
        created_at: '2026-09-22T01:00:00Z',
      },
    ],
    ...over,
  }
}

const idleAskState: MarginNoteAskState = { pendingQuestion: null, failedQuestion: null, error: null }

function mountCard(over: Partial<MarginNoteAnnotation> = {}, askState: MarginNoteAskState = idleAskState) {
  const on = {
    jump: vi.fn(),
    removeRequest: vi.fn(),
    ask: vi.fn(),
    retryAsk: vi.fn(),
    openCited: vi.fn(),
  }
  const wrapper = mount(MarginNoteCard, {
    props: {
      note: makeNote(over),
      askState,
      onJump: on.jump,
      onRemoveRequest: on.removeRequest,
      onAsk: on.ask,
      onRetryAsk: on.retryAsk,
      onOpenCited: on.openCited,
    },
  })
  return { wrapper, on }
}

describe('MarginNoteCard（锚点卡交互契约）', () => {
  it('MC-1：引用摘要两行截断 + 独立常显「⌄ 展开全文」提示行（不藏在截断盒内）', () => {
    const { wrapper } = mountCard()
    const quote = wrapper.find('.mn-card__quote')
    expect(quote.classes()).not.toContain('mn-card__quote--expanded')
    const toggle = wrapper.find('[data-testid="mn-quote-toggle"]')
    expect(toggle.exists()).toBe(true)
    // 提示行是 quote 的兄弟节点（独立于截断盒），不是盒内 ::after
    expect(quote.find('[data-testid="mn-quote-toggle"]').exists()).toBe(false)
    expect(toggle.text()).toContain('展开全文')
  })

  it('MC-2：点提示行展开全文并变「⌃ 收起」，再点收起；不触发 jump/删除', async () => {
    const { wrapper, on } = mountCard()
    const toggle = wrapper.find('[data-testid="mn-quote-toggle"]')
    await toggle.trigger('click')
    expect(wrapper.find('.mn-card__quote').classes()).toContain('mn-card__quote--expanded')
    expect(wrapper.find('[data-testid="mn-quote-toggle"]').text()).toContain('收起')
    await wrapper.find('[data-testid="mn-quote-toggle"]').trigger('click')
    expect(wrapper.find('.mn-card__quote').classes()).not.toContain('mn-card__quote--expanded')
    expect(on.jump).not.toHaveBeenCalled()
    expect(on.removeRequest).not.toHaveBeenCalled()
  })

  it('MC-3/MC-4：↗ 跳回原文与 ✕ 删除按钮常显且各自 emit', async () => {
    const { wrapper, on } = mountCard()
    expect(wrapper.find('[data-testid="mn-jump"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="mn-delete"]').exists()).toBe(true)
    await wrapper.find('[data-testid="mn-jump"]').trigger('click')
    await wrapper.find('[data-testid="mn-delete"]').trigger('click')
    expect(on.jump).toHaveBeenCalledTimes(1)
    expect(on.removeRequest).toHaveBeenCalledTimes(1)
  })

  it('QA 轮渲染：问题/回答/引用 chips/术语 chips；新词带入库角标；纯模型知识标注（PR-1）', async () => {
    const { wrapper } = mountCard()
    expect(wrapper.findAll('.mn-qa')).toHaveLength(1)
    expect(wrapper.text()).toContain('逆回购是什么意思？')
    expect(wrapper.findAll('[data-cited-article]')).toHaveLength(3)
    // 引用非空时不显示纯模型知识标注
    expect(wrapper.find('[data-testid="mn-pure-model"]').exists()).toBe(false)
    const newChip = wrapper.find('.mn-term-chip--new')
    expect(newChip.text()).toContain('逆回购')
    expect(newChip.text()).toContain('入库')
    expect(wrapper.findAll('.mn-term-chip:not(.mn-term-chip--new)')[0]?.text()).toContain('流动性对冲')

    // cited 为空数组 = 纯模型知识
    const { wrapper: pureWrapper } = mountCard({
      qas: [{ ...makeNote().qas[0]!, cited_article_ids: [], cited_web_sources: [], extracted_terms: [] }],
    })
    expect(pureWrapper.find('[data-testid="mn-pure-model"]').exists()).toBe(true)
    expect(pureWrapper.text()).toContain('纯模型知识')
  })

  it('D7 网络来源：chips 渲染（外链新窗口）；仅有网络来源时不标纯模型知识；均空才标（WS-4/7）', () => {
    const { wrapper } = mountCard()
    const webs = wrapper.find('[data-testid="mn-web-sources"]')
    expect(webs.exists()).toBe(true)
    const chip = webs.find('a.mn-web-chip')
    expect(chip.attributes('href')).toBe('https://money.163.com/baike/x')
    expect(chip.attributes('target')).toBe('_blank')
    expect(chip.attributes('rel')).toBe('noopener')
    expect(chip.text()).toContain('逆回购_财经百科')
    // 有网络来源（即便无文章引用）→ 不标纯模型知识。
    const { wrapper: webOnly } = mountCard({
      qas: [{
        ...makeNote().qas[0]!,
        cited_article_ids: [],
        cited_web_sources: [{ title: '', url: 'https://x.example.com' }],
        extracted_terms: [],
      }],
    })
    expect(webOnly.find('[data-testid="mn-web-sources"]').exists()).toBe(true)
    expect(webOnly.find('[data-testid="mn-pure-model"]').exists()).toBe(false)
    // 无任何来源 → 标注出现（且来源行不渲染）。
    const { wrapper: none } = mountCard({
      qas: [{ ...makeNote().qas[0]!, cited_article_ids: [], cited_web_sources: [], extracted_terms: [] }],
    })
    expect(none.find('[data-testid="mn-web-sources"]').exists()).toBe(false)
    expect(none.find('[data-testid="mn-pure-model"]').exists()).toBe(true)
  })

  it('生成中：骨架行展示 pending 问题；输入锁定（ID-2）', async () => {
    const { wrapper } = mountCard({}, { pendingQuestion: '那到期不续做会怎样？', failedQuestion: null, error: null })
    const pending = wrapper.find('[data-testid="mn-qa-pending"]')
    expect(pending.exists()).toBe(true)
    expect(pending.text()).toContain('那到期不续做会怎样？')
    expect(wrapper.find('[data-testid="mn-ask-submit"]').attributes('disabled')).toBeDefined()
  })

  it('AV-1：失败行内错误 + 重试按钮，问题文本保留在错误行内', async () => {
    const { wrapper, on } = mountCard({}, { pendingQuestion: null, failedQuestion: '逆回购是什么', error: '模型超时' })
    const errorRow = wrapper.find('[data-testid="mn-qa-error"]')
    expect(errorRow.exists()).toBe(true)
    expect(errorRow.text()).toContain('模型超时')
    expect(errorRow.text()).toContain('逆回购是什么')
    await wrapper.find('[data-testid="mn-qa-retry"]').trigger('click')
    expect(on.retryAsk).toHaveBeenCalledTimes(1)
  })

  it('IN-3：空问题提交不发 ask；非空提交清空输入框（ID-2 不重复提交由 askState 锁定）', async () => {
    const { wrapper, on } = mountCard()
    await wrapper.find('[data-testid="mn-ask-form"]').trigger('submit')
    expect(on.ask).not.toHaveBeenCalled()

    await wrapper.find('input[type="text"]').setValue('追问：到期不续做会怎样？')
    await wrapper.find('[data-testid="mn-ask-form"]').trigger('submit')
    expect(on.ask).toHaveBeenCalledWith('追问：到期不续做会怎样？')
    expect((wrapper.find('input[type="text"]').element as HTMLInputElement).value).toBe('')
  })

  it('PR-3：changed 降级态显示「原文已变更」，跳回原文按钮隐藏', () => {
    const { wrapper } = mountCard()
    wrapper.setProps({ changed: true })
    // setProps 后需要重新取
    return nextTick().then(() => {
      expect(wrapper.find('[data-testid="mn-changed"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="mn-jump"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="mn-delete"]').exists()).toBe(true)
    })
  })

  it('引用 chip 点击 → openCited 携带文章 id（与 thread 溯源同链路）', async () => {
    const { wrapper, on } = mountCard()
    await wrapper.find('[data-cited-article="102"]').trigger('click')
    expect(on.openCited).toHaveBeenCalledWith(102)
  })
})
