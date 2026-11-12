import { describe, expect, it } from 'vitest'
import { CAT_LABEL, catLabel, matchInfoLabel, matchReasonColor, stepNoOf, stepShortOf } from './matchQuality'

describe('matchReasonColor', () => {
  it('maps each known reason to its theme token', () => {
    expect(matchReasonColor('direct_hit')).toBe('var(--color-match-direct-hit)')
    expect(matchReasonColor('hit_rate')).toBe('var(--color-match-hit-rate)')
    expect(matchReasonColor('max_sim')).toBe('var(--color-match-max-sim)')
    expect(matchReasonColor('weighted')).toBe('var(--color-match-weighted)')
    expect(matchReasonColor('composite_hit')).toBe('var(--color-match-composite)')
  })

  it('falls back to the weighted token for an unknown reason', () => {
    expect(matchReasonColor('unknown')).toBe('var(--color-match-weighted)')
    expect(matchReasonColor('')).toBe('var(--color-match-weighted)')
  })

  // downgraded 淡化色已随标记退役（2026-11-12）：matchReasonColor 单参、恒返回裸 token。
  it('returns the bare token for every reason (downgraded retired)', () => {
    expect(matchReasonColor('max_sim')).toBe('var(--color-match-max-sim)')
  })
})

describe('matchInfoLabel', () => {
  it('builds a chinese reason word + score', () => {
    expect(matchInfoLabel({ match_reason: 'direct_hit', score: 1 })).toBe('直接命中 1.00')
    expect(matchInfoLabel({ match_reason: 'hit_rate', score: 0.42 })).toBe('命中率 0.42')
    expect(matchInfoLabel({ match_reason: 'max_sim', score: 0.85 })).toBe('相似度 0.85')
    expect(matchInfoLabel({ match_reason: 'weighted', score: 0.59 })).toBe('综合 0.59')
    expect(matchInfoLabel({ match_reason: 'composite_hit', score: 1 })).toBe('组合命中 1.00')
  })

  it('never appends the downgrade marker (retired 2026-11-12)', () => {
    expect(matchInfoLabel({ match_reason: 'max_sim', score: 0.82 })).toBe('相似度 0.82')
  })

  it('falls back to the raw reason for unknown values', () => {
    expect(matchInfoLabel({ match_reason: 'mystery', score: 0.1 })).toBe('mystery 0.10')
  })
})

describe('匹配流程步号', () => {
  it('映射到通用流程的步号（组合⓪/精确①/命中率③/相似度④/加权⑤）', () => {
    expect(stepNoOf('composite_hit')).toBe('⓪')
    expect(stepNoOf('direct_hit')).toBe('①')
    expect(stepNoOf('hit_rate')).toBe('③')
    expect(stepNoOf('max_sim')).toBe('④')
    expect(stepNoOf('weighted')).toBe('⑤')
  })

  it('未知规则回退占位步号与原规则名', () => {
    expect(stepNoOf('mystery')).toBe('·')
    expect(stepShortOf('mystery')).toBe('mystery')
  })

  it('短标签：组合/精确/命中率/相似度/加权', () => {
    expect(stepShortOf('composite_hit')).toBe('组合')
    expect(stepShortOf('direct_hit')).toBe('精确')
    expect(stepShortOf('hit_rate')).toBe('命中率')
    expect(stepShortOf('max_sim')).toBe('相似度')
    expect(stepShortOf('weighted')).toBe('加权')
  })
})

describe('标签类别', () => {
  it('事件/关键词/人物/组织四类如实标注', () => {
    expect(CAT_LABEL).toEqual({ event: '事件', keyword: '关键词', person: '人物', org: '组织' })
    expect(catLabel('event')).toBe('事件')
    expect(catLabel('keyword')).toBe('关键词')
    expect(catLabel('person')).toBe('人物')
    expect(catLabel('org')).toBe('组织')
  })

  it('未知类别回退原值，空值回退「标签」', () => {
    expect(catLabel('concept')).toBe('concept')
    expect(catLabel('')).toBe('标签')
  })
})
