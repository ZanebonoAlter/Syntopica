/**
 * FeedSourceQualityPill — add-source-board-hit-rate T5 / B6-01..08。
 *
 * pill 三档配色（高 ≥60% / 中 30–60% / 低 <30%）、样本少（<5 篇）、
 * 无新文（0 篇）、打标关闭（不显示百分比）、加载中/失败「—」占位不抖动。
 * 分档边界（30% 取中档、60% 取高档）属 spec 三分取上界包含。
 */
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedSourceQualityPill from './FeedSourceQualityPill.vue'

// 样式规则锚（对照 docs/reference/standard/frontend/testing.md 机械锚思路）：
// 「—」占位与数字态同宽的固定宽度规则必须留在源码 style 中，防止样式重构丢失。
import pillSource from './FeedSourceQualityPill.vue?raw'

function mountPill(props: Record<string, unknown> = {}) {
  return mount(FeedSourceQualityPill, { props })
}

describe('FeedSourceQualityPill — 三档配色（B6-01..03）', () => {
  it('B6-01: 67 篇/命中 56 → 「84%」高指标档', () => {
    const wrapper = mountPill({ articles: 67, hitRate: 56 / 67, taggingEnabled: true })
    expect(wrapper.text()).toBe('84%')
    expect(wrapper.classes()).toContain('feed-source-pill--high')
    expect(wrapper.attributes('data-state')).toBe('high')
  })

  it('B6-02: 1662 篇/命中 437 → 「26%」低指标档', () => {
    const wrapper = mountPill({ articles: 1662, hitRate: 437 / 1662, taggingEnabled: true })
    expect(wrapper.text()).toBe('26%')
    expect(wrapper.classes()).toContain('feed-source-pill--low')
    expect(wrapper.attributes('data-state')).toBe('low')
  })

  it('B6-03: 499 篇/命中 160 → 「32%」中指标档（边界 30% 取中档、60% 取高档）', () => {
    const wrapper = mountPill({ articles: 499, hitRate: 160 / 499, taggingEnabled: true })
    expect(wrapper.text()).toBe('32%')
    expect(wrapper.classes()).toContain('feed-source-pill--mid')
    expect(wrapper.attributes('data-state')).toBe('mid')
  })

  it('分档边界：hit_rate=0.6 → 高；0.3 → 中；0.299 → 低', () => {
    expect(mountPill({ articles: 100, hitRate: 0.6 }).attributes('data-state')).toBe('high')
    expect(mountPill({ articles: 100, hitRate: 0.3 }).attributes('data-state')).toBe('mid')
    expect(mountPill({ articles: 100, hitRate: 0.299 }).attributes('data-state')).toBe('low')
  })
})

describe('FeedSourceQualityPill — 特殊态不显示百分比（B6-04..06）', () => {
  it('B6-04: 3 篇 → 「样本少」，DOM 内不含 %', () => {
    const wrapper = mountPill({ articles: 3, hitRate: 1 / 3, taggingEnabled: true })
    expect(wrapper.text()).toBe('样本少')
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.attributes('data-state')).toBe('low-sample')
  })

  it('B6-05: 0 篇 → 「无新文」', () => {
    const wrapper = mountPill({ articles: 0, hitRate: 0, taggingEnabled: true })
    expect(wrapper.text()).toBe('无新文')
    expect(wrapper.attributes('data-state')).toBe('empty')
  })

  it('B6-06: tagging_enabled=false → 「打标关闭」，不含百分比（配置结果而非内容质量）', () => {
    const wrapper = mountPill({ articles: 1, hitRate: 0, taggingEnabled: false })
    expect(wrapper.text()).toBe('打标关闭')
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.attributes('data-state')).toBe('off')
  })
})

describe('FeedSourceQualityPill — 「—」占位（B6-07..08）', () => {
  it('B6-07: 统计请求 pending → 「—」占位，源码保留固定 min-width 防抖动', () => {
    const wrapper = mountPill({ articles: 1662, hitRate: 0.263, taggingEnabled: true, loading: true })
    expect(wrapper.text()).toBe('—')
    expect(wrapper.attributes('data-state')).toBe('pending')
    expect(wrapper.classes()).toContain('feed-source-pill--pending')
    // 样式规则锚：固定宽度规则在源码中（scoped CSS 不进 happy-dom，锚在源码层）
    expect(pillSource).toContain('min-width:')
    expect(pillSource).toMatch(/\.feed-source-pill\s*\{[^}]*min-width/)
  })

  it('B6-08: 统计缺失（聚合失败无数据）→ 保持「—」不抛错', () => {
    const wrapper = mountPill({})
    expect(wrapper.text()).toBe('—')
    expect(wrapper.attributes('data-state')).toBe('pending')
  })
})
