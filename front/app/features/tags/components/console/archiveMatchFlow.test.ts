import { describe, expect, it } from 'vitest'
import {
  buildGenericFlowSteps,
  buildTagFlowSteps,
  rateFormula,
  RULE_COPY,
  ruleOf,
  type MatchFlowConfig,
} from './archiveMatchFlow'
import type { MatchDetailResponse } from '~/api/semanticBoards'

/**
 * clarify-archive-match-rules §1 纯函数用例：
 *  - 规则元数据单一常量源（RULE_COPY：编号/规则名/问法/短名，顺序 ⓪→⑤）；
 *  - buildGenericFlowSteps / buildTagFlowSteps 全部从常量渲染（同源，D5）；
 *  - buildTagFlowSteps 全集输出：走过的步给真实数值、命中步之后标 skipped（A2）、
 *    direct_hit 命中补产 ② compute 参照步（A2）、composite_hit 仅 ⓪（免相似度计算）；
 *  - 口径分家：① 交集标「（完全相同）」，② 参照行标「相似达标 x/N」；
 *  - 缺 match-detail 时给规则 + 阈值 + 「需实时拉取」，不伪造。
 */

const config: MatchFlowConfig = {
  simThreshold: 0.72,
  directHitRate: 0.5,
  directMaxSim: 0.8,
  directMaxSimMinHits: 2,
  directMaxSimMinHitRate: 0.3,
  minEffectiveSample: 3,
  hitRateSimBlend: 0.7,
  weightSim: 0.6,
  weightDensity: 0.4,
  weightedThreshold: 0.6,
  directHitMinOverlap: 2,
  directionSimThreshold: 0.5,
}

function detail(overrides: Partial<MatchDetailResponse> = {}): MatchDetailResponse {
  return {
    topic_tag_id: 7,
    topic_tag_label: '美伊关系',
    semantic_board_id: 1974,
    match_reason: 'direct_hit',
    score: 0.7,
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
      { tag_auxiliary_id: 102, tag_label: '抵抗之弧', board_auxiliary_id: 202, board_label: '中东战争' },
      { tag_auxiliary_id: 103, tag_label: '霍尔木兹', board_auxiliary_id: 203, board_label: '霍尔木兹' },
    ],
    composite_hits: undefined,
    tag_auxiliary_count: 5,
    hits: 5,
    hit_rate: 1,
    max_similarity: 0.86,
    pairs: [],
    ...overrides,
  }
}

describe('RULE_COPY — 规则元数据单一常量源', () => {
  it('顺序 ⓪→①→②→③→④→⑤，编号唯一且问法/短名非空', () => {
    expect(RULE_COPY.map(rule => rule.no)).toEqual(['⓪', '①', '②', '③', '④', '⑤'])
    for (const rule of RULE_COPY) {
      expect(rule.name.length).toBeGreaterThan(0)
      expect(rule.question.length).toBeGreaterThan(0)
      expect(rule.short.length).toBeGreaterThan(0)
    }
  })

  it('五个判定规则（⓪①③④⑤）+ ② 中间计算步；ruleOf 按 id 查询', () => {
    expect(ruleOf('composite_hit')!.name).toBe('组合命中')
    expect(ruleOf('direct_hit')!.name).toBe('精确匹配')
    expect(ruleOf('similarity')!.name).toBe('算余弦相似度')
    expect(ruleOf('hit_rate')!.name).toBe('命中率规则')
    expect(ruleOf('max_sim')!.name).toBe('最高相似度规则')
    expect(ruleOf('weighted')!.name).toBe('综合加权')
    expect(ruleOf('nonexistent')).toBeUndefined()
  })
})

describe('buildGenericFlowSteps — 通用说明从常量渲染', () => {
  it('六步的编号、规则名、问法全部来自 RULE_COPY（同源）', () => {
    const steps = buildGenericFlowSteps(config)
    expect(steps).toHaveLength(6)
    steps.forEach((step, index) => {
      const rule = RULE_COPY[index]!
      expect(step.title).toBe(`${rule.no} ${rule.name}`)
      expect(step.desc.startsWith(rule.question)).toBe(true)
      expect(step.no).toBe(rule.no)
      expect(step.name).toBe(rule.name)
      expect(step.question).toBe(rule.question)
    })
  })

  it('② 是中间计算步（compute），其余为 pending；阈值取真实配置值', () => {
    const steps = buildGenericFlowSteps(config)
    expect(steps.find(step => step.id === 'similarity')!.state).toBe('compute')
    expect(steps.find(step => step.id === 'direct_hit')!.result).toContain('交集 ≥ 2')
    expect(steps.find(step => step.id === 'similarity')!.desc).toContain('0.72')
  })
})

describe('buildTagFlowSteps — 逐标签判定过程（全集输出）', () => {
  it('direct_hit 命中：① 命中（交集标完全相同）+ ② compute 参照步（相似达标口径）+ ③④⑤ skipped，无 ⓪', () => {
    const steps = buildTagFlowSteps('direct_hit', config, detail(), 5)
    expect(steps.map(step => step.id)).toEqual(['direct_hit', 'similarity', 'hit_rate', 'max_sim', 'weighted'])
    const direct = steps[0]!
    expect(direct.state).toBe('matched')
    expect(direct.reached).toBe(true)
    expect(direct.result).toContain('交集 3 个（完全相同）≥ 2')
    const similarity = steps[1]!
    expect(similarity.state).toBe('compute')
    expect(similarity.result).toContain('相似达标 5/5')
    expect(similarity.result).toContain('R 1.00')
    expect(similarity.result).toContain('Smax 0.86')
    expect(steps.slice(2).every(step => step.state === 'skipped' && step.reached === false)).toBe(true)
    // ① 与 ② 两个口径分家：交集用「完全相同」，相似达标不带该词
    expect(similarity.result).not.toContain('完全相同')
  })

  it('hit_rate 命中：① 未通过给真实差距 + ② compute + ③ 命中 + ④⑤ skipped', () => {
    const steps = buildTagFlowSteps('hit_rate', config, detail({
      match_reason: 'hit_rate',
      hits: 3,
      hit_rate: 0.6,
      direct_hit_auxiliaries: [{ tag_auxiliary_id: 101, tag_label: '伊朗', board_auxiliary_id: 201, board_label: '伊朗' }],
    }), 5)
    expect(steps[0]!.state).toBe('failed')
    expect(steps[0]!.result).toContain('交集 1 个（完全相同）< 2')
    expect(steps[1]!.state).toBe('compute')
    expect(steps[1]!.result).toContain('相似达标 3/5')
    const rate = steps[2]!
    expect(rate.state).toBe('matched')
    expect(rate.result).toContain('R=0.600 > 0.50')
    expect(steps.slice(3).map(step => step.state)).toEqual(['skipped', 'skipped'])
  })

  it('weighted 兜底命中（①-④ 全落空）：无 skipped，⑤ matched 给代入数值', () => {
    const steps = buildTagFlowSteps('weighted', config, detail({
      match_reason: 'weighted',
      hits: 1,
      hit_rate: 0.2,
      max_similarity: 0.75,
      direct_hit_auxiliaries: [],
    }), 5)
    expect(steps.map(step => step.state)).toEqual(['failed', 'compute', 'failed', 'failed', 'matched'])
    expect(steps.every(step => step.reached)).toBe(true)
    expect(steps[4]!.result).toContain('0.700')
    expect(steps[4]!.result).toContain('≥ 0.60')
  })

  it('composite_hit 命中：仅 ⓪ 走到（组合链 + 免方向校验），①-⑤ 全 skipped，无 ② compute', () => {
    const steps = buildTagFlowSteps('composite_hit', config, detail({
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
    }), 5)
    expect(steps).toHaveLength(6)
    expect(steps[0]!.id).toBe('composite_hit')
    expect(steps[0]!.state).toBe('matched')
    expect(steps[0]!.result).toContain('伊美关系')
    expect(steps[0]!.result).toContain('免方向校验')
    expect(steps[1]!.id).toBe('direct_hit')
    expect(steps[1]!.state).toBe('skipped')
    expect(steps.find(step => step.id === 'similarity')!.state).toBe('skipped')
    expect(steps.slice(1).every(step => step.reached === false)).toBe(true)
  })

  it('match-detail 缺失：命中步标「需实时拉取」，① 仍给真实阈值与口径词，不伪造数值', () => {
    const steps = buildTagFlowSteps('direct_hit', config, null, 5)
    const direct = steps[0]!
    expect(direct.state).toBe('matched')
    expect(direct.result).toContain('需交集 ≥ 2 个（完全相同）')
    expect(direct.result).toContain('需实时拉取')
    expect(steps[1]!.result).toContain('需实时拉取')
  })

  it('direct_hit_auxiliaries 重复 id（脏数据）：交集按去重计数', () => {
    const dup = detail({
      direct_hit_auxiliaries: [
        { tag_auxiliary_id: 101, tag_label: '伊朗', board_auxiliary_id: 201, board_label: '伊朗' },
        { tag_auxiliary_id: 101, tag_label: '伊朗', board_auxiliary_id: 201, board_label: '伊朗' },
        { tag_auxiliary_id: 102, tag_label: '抵抗之弧', board_auxiliary_id: 202, board_label: '中东战争' },
      ],
    })
    const steps = buildTagFlowSteps('direct_hit', config, dup, 5)
    expect(steps[0]!.result).toContain('交集 2 个（完全相同）')
  })
})

describe('rateFormula — 命中率公式（最小样本量兜底）', () => {
  it('单元素 N=1：R 分母取 max(N, s)=3 而非 1', () => {
    const item = detail({ tag_auxiliary_count: 1, hits: 1, hit_rate: 0.333 })
    const latex = rateFormula(item)
    expect(latex).toContain('\\max(1,3)')
    expect(latex).toContain('0.333')
  })
})
