import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SectionWatchBadge from './SectionWatchBadge.vue'

describe('SectionWatchBadge', () => {
  it('renders a bare dot with the type-only title when no watch name is given', () => {
    const wrapper = mount(SectionWatchBadge, { props: { laneTier: 'watch_keyword' } })
    const badge = wrapper.find('.section-watch-badge')

    expect(badge.attributes('data-lane-tier')).toBe('watch_keyword')
    expect(badge.attributes('title')).toBe('关键字物化板块')
    expect(badge.attributes('aria-label')).toBe('关键字物化板块')
    expect(wrapper.find('.section-watch-badge__name').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })

  it('decorates the keyword badge with the watch name', () => {
    const wrapper = mount(SectionWatchBadge, { props: { laneTier: 'watch_keyword', watchLabel: 'harness' } })

    expect(wrapper.get('.section-watch-badge__name').text()).toBe('harness')
    expect(wrapper.find('.section-watch-badge').attributes('title')).toBe('关键字物化板块 · harness')
    expect(wrapper.find('.section-watch-badge').attributes('aria-label')).toBe('关键字物化板块 · harness')
  })

  it('decorates the sentence badge with the watch name', () => {
    const wrapper = mount(SectionWatchBadge, { props: { laneTier: 'watch_sentence', watchLabel: '美伊形势对市场影响' } })

    expect(wrapper.get('.section-watch-badge__name').text()).toBe('美伊形势对市场影响')
    expect(wrapper.find('.section-watch-badge').attributes('title')).toBe('一句话物化话题 · 美伊形势对市场影响')
  })

  it('treats a blank watch name as absent (fallback to bare dot)', () => {
    const wrapper = mount(SectionWatchBadge, { props: { laneTier: 'watch_sentence', watchLabel: '   ' } })

    expect(wrapper.find('.section-watch-badge__name').exists()).toBe(false)
    expect(wrapper.find('.section-watch-badge').attributes('title')).toBe('一句话物化话题')
  })

  it('falls back to keyword meta for an unknown lane tier', () => {
    const wrapper = mount(SectionWatchBadge, { props: { laneTier: 'watch_other' } })

    expect(wrapper.find('.section-watch-badge').attributes('title')).toBe('关键字物化板块')
  })
})
