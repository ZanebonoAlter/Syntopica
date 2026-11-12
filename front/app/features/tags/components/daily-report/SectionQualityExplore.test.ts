import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SectionQualityExplore from './SectionQualityExplore.vue'
import type { DailyReportQualityEntry } from '~/api/dailyReports'

function entry(over: Partial<DailyReportQualityEntry> = {}): DailyReportQualityEntry {
  return {
    tag_id: 1,
    label: 'AI芯片',
    match_reason: 'direct_hit',
    score: 1.0,
    ...over,
  }
}

describe('SectionQualityExplore', () => {
  it('renders one chip per breakdown entry with the reason colour and score', () => {
    const wrapper = mount(SectionQualityExplore, {
      props: {
        breakdown: [
          entry({ tag_id: 1, label: 'AI芯片', match_reason: 'direct_hit', score: 1.0 }),
          entry({ tag_id: 2, label: 'GPT-5发布', match_reason: 'max_sim', score: 0.85 }),
          entry({ tag_id: 3, label: 'AI竞赛', match_reason: 'weighted', score: 0.59 }),
        ],
      },
    })

    const chips = wrapper.findAll('.section-quality-explore__chip')
    expect(chips).toHaveLength(3)
    expect(chips[0]!.attributes('style')).toContain('var(--color-match-direct-hit)')
    expect(chips[1]!.attributes('style')).toContain('var(--color-match-max-sim)')
    expect(chips[2]!.attributes('style')).toContain('var(--color-match-weighted)')

    expect(wrapper.text()).toContain('AI芯片')
    expect(wrapper.text()).toContain('GPT-5发布')
    expect(wrapper.text()).toContain('1.00')
    expect(wrapper.text()).toContain('0.85')
  })

  it('renders no downgrade marker (retired 2026-11-12)', () => {
    const wrapper = mount(SectionQualityExplore, {
      props: {
        breakdown: [entry({ tag_id: 9, label: 'X', match_reason: 'max_sim', score: 0.82 })],
      },
    })

    const chip = wrapper.find('.section-quality-explore__chip')
    expect(chip.classes()).not.toContain('section-quality-explore__chip--downgraded')
    expect(wrapper.text()).not.toContain('↓')
  })

  it('shows the placeholder when breakdown is null (historical section)', () => {
    const wrapper = mount(SectionQualityExplore, { props: { breakdown: null } })

    expect(wrapper.text()).toContain('无质量明细')
    expect(wrapper.findAll('.section-quality-explore__chip')).toHaveLength(0)
  })

  it('shows the placeholder when breakdown is an empty array', () => {
    const wrapper = mount(SectionQualityExplore, { props: { breakdown: [] } })

    expect(wrapper.text()).toContain('无质量明细')
    expect(wrapper.findAll('.section-quality-explore__chip')).toHaveLength(0)
  })

  it('keeps the tag id stable as the chip key', () => {
    const wrapper = mount(SectionQualityExplore, {
      props: { breakdown: [entry({ tag_id: 42 })] },
    })

    expect(wrapper.find('.section-quality-explore__chip').attributes('data-tag-id')).toBe('42')
  })
})

describe('SectionQualityExplore — topic anchor line', () => {
  it('renders the anchor line above breakdown when anchor data present', () => {
    const w = mount(SectionQualityExplore, {
      props: {
        breakdown: [entry()],
        topicLabel: '霍尔木兹海峡',
        topicDistance: 0.03,
        topicConfidence: 'anchor_hit',
      },
    })
    const anchor = w.find('.section-quality-explore__anchor')
    expect(anchor.exists()).toBe(true)
    // 锚定行在 chip 列表之前（DOM 顺序）
    expect(w.element.querySelector('.section-quality-explore__anchor + .section-quality-explore__list')).toBeTruthy()
    expect(anchor.text()).toContain('霍尔木兹海峡')
    expect(anchor.text()).toContain('0.03')
    expect(anchor.text()).toContain('极紧锚定')
  })

  it('distinguishes 稳锚 / 松锚 labels', () => {
    const steady = mount(SectionQualityExplore, {
      props: { topicDistance: 0.1, topicConfidence: 'anchor_hit' },
    })
    expect(steady.find('.section-quality-explore__anchor').text()).toContain('稳锚定')
    const loose = mount(SectionQualityExplore, {
      props: { topicDistance: 0.27, topicConfidence: 'anchor_hit' },
    })
    expect(loose.find('.section-quality-explore__anchor').text()).toContain('松锚定')
  })

  it('shows 新话题候选 with distance for auto_new', () => {
    const w = mount(SectionQualityExplore, {
      props: { topicDistance: 0.2, topicConfidence: 'auto_new' },
    })
    const t = w.find('.section-quality-explore__anchor').text()
    expect(t).toContain('新话题候选')
    expect(t).toContain('0.20')
  })

  it('does not render the anchor line for unmatched', () => {
    const w = mount(SectionQualityExplore, {
      props: { breakdown: [entry()], topicDistance: 0.1, topicConfidence: 'unmatched' },
    })
    expect(w.find('.section-quality-explore__anchor').exists()).toBe(false)
    expect(w.text()).toContain('AI芯片') // breakdown 仍正常
  })

  it('does not render the anchor line when distance missing/zero', () => {
    const a = mount(SectionQualityExplore, { props: { topicConfidence: 'anchor_hit' } })
    expect(a.find('.section-quality-explore__anchor').exists()).toBe(false)
    const b = mount(SectionQualityExplore, { props: { topicDistance: 0, topicConfidence: 'anchor_hit' } })
    expect(b.find('.section-quality-explore__anchor').exists()).toBe(false)
  })

  it('falls back to 未命名话题 when topicLabel missing', () => {
    const w = mount(SectionQualityExplore, {
      props: { topicDistance: 0.1, topicConfidence: 'anchor_hit' },
    })
    expect(w.find('.section-quality-explore__anchor').text()).toContain('未命名话题')
  })

  it('historical section (no breakdown, no anchor) shows 无质量明细 and no anchor line', () => {
    const w = mount(SectionQualityExplore, { props: { breakdown: null } })
    expect(w.find('.section-quality-explore__anchor').exists()).toBe(false)
    expect(w.text()).toContain('无质量明细')
  })
})
