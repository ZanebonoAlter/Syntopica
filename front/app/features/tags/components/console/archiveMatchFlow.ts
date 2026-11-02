/**
 * 归档匹配的纯计算助手（rework-workshop-into-radar-and-desk 8G.3c，2026-10-31 归档重做）。
 *
 * 口径与来源：
 *  - 步骤顺序与「命中即停」语义沿用 `features/tags/components/MatchDetailPanel.vue`
 *    的 flowSteps（⓪组合命中 → ①精确匹配 → ②余弦相似度 → ③命中率 → ④最高相似度 → ⑤综合加权）；
 *  - 公式构造（primaryFormula / substitutionFormula / rateFormula）逐字沿用
 *    MatchDetailPanel 的实现（KaTeX 渲染）；**MatchDetailPanel 本体不改动**
 *    （旧页面仍在用），此处只是把同一套构造抽成可复用纯函数并注明来源；
 *  - 阈值来自两处同源接口：全局匹配配置（GET /semantic-boards/matching-config）
 *    与 match-detail 响应里的 config（字段语义一致）。
 *
 * 归档只读：本模块不产生任何写操作，也不复制后端的判定口径
 * （in_report / weak_evidence / quality 序一律后端派生）。
 * 实时重算 vs 冻结 score 的对照（recomputedScore / scoreDriftNote）只用于如实标注
 * 「代入式里的数是实时值、score 是冻结值」，不参与任何判定。
 */
import type { MatchDetailConfig, MatchDetailResponse, MatchingConfig } from '~/api/semanticBoards'

/** 匹配流程的一个步骤（⓪→⑤，命中即停）。 */
export interface MatchFlowStep {
  id: string
  title: string
  desc: string
  result: string
  state: 'matched' | 'failed' | 'compute' | 'pending'
}

/** 五个命中阶段的规则顺序（②余弦相似度是中间计算步，不产出判定规则）。 */
export const FLOW_STAGE_ORDER = ['composite_hit', 'direct_hit', 'hit_rate', 'max_sim', 'weighted'] as const

export type FlowStageReason = typeof FLOW_STAGE_ORDER[number]

/** 阈值口径（归一后的 camelCase，避免组件里散落后端字段名）。 */
export interface MatchFlowConfig {
  simThreshold: number
  directHitRate: number
  directMaxSim: number
  directMaxSimMinHits: number
  directMaxSimMinHitRate: number
  minEffectiveSample: number
  hitRateSimBlend: number
  weightSim: number
  weightDensity: number
  weightedThreshold: number
  directHitMinOverlap: number
  directionSimThreshold: number
}

function num(value: number | undefined | null, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

/** 分数格式化（非有限值回退 '--'，不伪造 0.00）。 */
export function formatScore(value: number | undefined | null, digits = 2): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return value.toFixed(digits)
}

/** 全局匹配配置（GET /semantic-boards/matching-config）→ 归一阈值口径。 */
export function flowConfigFromApi(config: MatchingConfig | null | undefined): MatchFlowConfig {
  return {
    simThreshold: num(config?.semantic_board_match_sim_threshold, 0.6),
    directHitRate: num(config?.semantic_board_match_direct_hit_rate, 0.5),
    directMaxSim: num(config?.semantic_board_match_direct_max_sim, 0.8),
    directMaxSimMinHits: num(config?.semantic_board_match_direct_max_sim_min_hits, 2),
    directMaxSimMinHitRate: num(config?.semantic_board_match_direct_max_sim_min_hit_rate, 0.3),
    minEffectiveSample: num(config?.semantic_board_match_min_effective_sample, 3),
    hitRateSimBlend: num(config?.semantic_board_match_hit_rate_sim_blend, 0.5),
    weightSim: num(config?.semantic_board_match_weight_sim, 0.7),
    weightDensity: num(config?.semantic_board_match_weight_density, 0.3),
    weightedThreshold: num(config?.semantic_board_match_weighted_threshold, 0.6),
    directHitMinOverlap: num(config?.semantic_board_match_direct_hit_min_overlap, 2),
    directionSimThreshold: num(config?.semantic_board_match_direction_sim_threshold, 0.5),
  }
}

/** match-detail 响应里的 config → 归一阈值口径（与全局配置同源同字段）。 */
export function flowConfigFromDetail(config: MatchDetailConfig | null | undefined): MatchFlowConfig {
  return {
    simThreshold: num(config?.sim_threshold, 0.6),
    directHitRate: num(config?.direct_hit_rate, 0.5),
    directMaxSim: num(config?.direct_max_sim, 0.8),
    directMaxSimMinHits: num(config?.direct_max_sim_min_hits, 2),
    directMaxSimMinHitRate: num(config?.direct_max_sim_min_hit_rate, 0.3),
    minEffectiveSample: num(config?.min_effective_sample, 3),
    hitRateSimBlend: num(config?.hit_rate_sim_blend, 0.5),
    weightSim: num(config?.weight_sim, 0.7),
    weightDensity: num(config?.weight_density, 0.3),
    weightedThreshold: num(config?.weighted_threshold, 0.6),
    directHitMinOverlap: num(config?.direct_hit_min_overlap, 2),
    directionSimThreshold: num(config?.direction_sim_threshold, 0.5),
  }
}

/**
 * 通用匹配流程说明（工具行「匹配流程」弹窗）。
 *
 * 对所有构成标签一致：每步「做了什么 / 阈值（全局配置真实值）」；命中即停。
 * state 一律 pending/compute——这是「说明」不是「某次判定结果」，不冒充命中态。
 */
export function buildGenericFlowSteps(config: MatchFlowConfig): MatchFlowStep[] {
  return [
    {
      id: 'composite_hit',
      title: '⓪ 组合命中',
      desc: '事件与版块挂载了同一组合标签？',
      result: '组合标签交集非空 → 直接命中（免方向校验）',
      state: 'pending',
    },
    {
      id: 'direct_hit',
      title: '① 精确匹配',
      desc: '事件和版块有多少个辅助标签完全相同？',
      result: `交集 ≥ ${config.directHitMinOverlap} 个 → 命中`,
      state: 'pending',
    },
    {
      id: 'similarity',
      title: '② 算余弦相似度',
      desc: `把事件的辅助标签和版块标签两两比较「有多像」，相似度 ≥ ${formatScore(config.simThreshold)} 算命中`,
      result: `中间计算步：产出命中数 / 标签总数 → 命中率 R、最高相似度 Smax（最小样本量 ${config.minEffectiveSample}）`,
      state: 'compute',
    },
    {
      id: 'hit_rate',
      title: '③ 命中率规则',
      desc: `「几分之几」的标签像版块的？超过 ${(config.directHitRate * 100).toFixed(0)}% 就挂载`,
      result: `R > ${formatScore(config.directHitRate)} → 命中`,
      state: 'pending',
    },
    {
      id: 'max_sim',
      title: '④ 最高相似度规则',
      desc: '最像的那一对有多像？需同时满足三个条件（Smax / 命中数 / R）',
      result: `Smax ≥ ${formatScore(config.directMaxSim)} 且命中 ≥ ${config.directMaxSimMinHits} 且 R ≥ ${formatScore(config.directMaxSimMinHitRate)} → 命中`,
      state: 'pending',
    },
    {
      id: 'weighted',
      title: '⑤ 综合加权',
      desc: '整体看像不像？把相似度和命中率加权',
      result: `${formatScore(config.weightSim)}×Smax + ${formatScore(config.weightDensity)}×R ≥ ${formatScore(config.weightedThreshold)} → 命中`,
      state: 'pending',
    },
  ]
}

/** 方向校验说明（通用流程末尾的一行真话）。 */
export function directionCheckNote(config: MatchFlowConfig): string {
  return `方向校验：标签与版块的方向相似度需 ≥ ${formatScore(config.directionSimThreshold)}；未通过的匹配默认隐藏、不计入日报。`
}

/** 命中即停说明（通用流程末尾的一行真话）。 */
export const HIT_AND_STOP_NOTE = '命中即停：按 ⓪→①→②→③→④→⑤ 顺序判定，首个满足的规则决定结果，后面的规则不再看。'

/** 逐标签的匹配流程步骤（真实数值可得时给数值，否则如实标「需实时计算」）。 */
export function buildTagFlowSteps(
  reason: string,
  config: MatchFlowConfig,
  detail: MatchDetailResponse | null,
  tagAuxCount: number,
): MatchFlowStep[] {
  const needLive = '（该标签命中数值需 getMatchDetail 实时计算）'
  const real = detail !== null
  const N = real ? detail.tag_auxiliary_count : tagAuxCount
  const overlap = real ? (detail.direct_hit_auxiliaries?.length ?? 0) : 0
  const steps: MatchFlowStep[] = []

  if (reason === 'composite_hit') {
    steps.push({
      id: 'composite_hit',
      title: '⓪ 组合命中',
      desc: '事件与版块挂载了同一组合标签？',
      result: real && (detail.composite_hits?.length ?? 0) > 0
        ? `${detail.composite_hits!.map(h => h.label).join('、')} → 直接命中（免方向校验）`
        : '组合标签交集非空 → 直接命中（免方向校验）',
      state: 'matched',
    })
    return steps
  }

  if (reason === 'direct_hit') {
    steps.push({
      id: 'direct_hit',
      title: '① 精确匹配',
      desc: '事件和版块有多少个辅助标签完全相同？',
      result: real
        ? `交集 ${overlap} 个 ≥ ${config.directHitMinOverlap} → 满足！score=${formatScore(detail.score)}`
        : `需交集 ≥ ${config.directHitMinOverlap}${needLive}`,
      state: 'matched',
    })
    return steps
  }

  steps.push({
    id: 'direct_hit',
    title: '① 精确匹配',
    desc: '事件和版块有多少个辅助标签完全相同？',
    result: real
      ? `交集 ${overlap} 个 < ${config.directHitMinOverlap}，不够 → 继续`
      : `交集未达 ${config.directHitMinOverlap}${needLive}`,
    state: 'failed',
  })

  steps.push({
    id: 'similarity',
    title: '② 算余弦相似度',
    desc: `把事件的 ${N} 个辅助标签和版块标签两两比较「有多像」，相似度 ≥ ${formatScore(config.simThreshold)} 算命中`,
    result: real
      ? `命中 ${detail.hits}/${N}，R=${formatScore(detail.hit_rate, 3)}，Smax=${formatScore(detail.max_similarity, 3)}`
      : needLive,
    state: 'compute',
  })

  if (reason === 'hit_rate') {
    steps.push({
      id: 'hit_rate',
      title: '③ 命中率规则',
      desc: `「几分之几」的标签像版块的？超过 ${(config.directHitRate * 100).toFixed(0)}% 就挂载`,
      result: real
        ? `R=${formatScore(detail.hit_rate, 3)} > ${formatScore(config.directHitRate)} → 满足！score=${formatScore(detail.score)}`
        : needLive,
      state: 'matched',
    })
    return steps
  }

  steps.push({
    id: 'hit_rate',
    title: '③ 命中率规则',
    desc: `「几分之几」的标签像版块的？超过 ${(config.directHitRate * 100).toFixed(0)}% 就挂载`,
    result: real
      ? `R=${formatScore(detail.hit_rate, 3)} ≤ ${formatScore(config.directHitRate)} → 不够`
      : needLive,
    state: 'failed',
  })

  const minHits = Math.min(config.directMaxSimMinHits, N || 1)
  if (reason === 'max_sim') {
    steps.push({
      id: 'max_sim',
      title: '④ 最高相似度规则',
      desc: '最像的那一对有多像？需同时满足三个条件（Smax / 命中数 / R）',
      result: real
        ? `Smax=${formatScore(detail.max_similarity, 3)} ≥ ${formatScore(config.directMaxSim)}，${detail.hits}≥${minHits}，R=${formatScore(detail.hit_rate, 3)} → 满足！${detail.downgraded ? ' ⚠降级匹配' : ''}`
        : needLive,
      state: 'matched',
    })
    return steps
  }

  steps.push({
    id: 'max_sim',
    title: '④ 最高相似度规则',
    desc: '最像的那一对有多像？需同时满足三个条件（Smax / 命中数 / R）',
    result: real
      ? `Smax=${formatScore(detail.max_similarity, 3)}、${detail.hits}/${minHits}、R=${formatScore(detail.hit_rate, 3)} → 未全满足，继续`
      : needLive,
    state: 'failed',
  })

  if (reason === 'weighted') {
    steps.push({
      id: 'weighted',
      title: '⑤ 综合加权',
      desc: '整体看像不像？相似度和命中率加权',
      result: real
        ? `${formatScore(config.weightSim)}×${formatScore(detail.max_similarity, 3)}+${formatScore(config.weightDensity)}×${formatScore(detail.hit_rate, 3)}=${formatScore(detail.score, 3)} ≥ ${formatScore(config.weightedThreshold)} → 满足！`
        : needLive,
      state: 'matched',
    })
  } else {
    steps.push({
      id: 'weighted',
      title: '⑤ 综合加权',
      desc: '整体看像不像？相似度和命中率加权',
      result: real
        ? `加权分 ${formatScore(detail.score, 3)} < ${formatScore(config.weightedThreshold)} → 不匹配`
        : needLive,
      state: 'failed',
    })
  }

  return steps
}

/**
 * 「为什么没命中」的一句话（入板过程 popover 的空阶段用）。
 *
 * 有 match-detail 真实数值 → 给差距数值；没有 → 给该阶段真实阈值 + 未达结论。
 */
export function whyMissText(
  reason: string,
  config: MatchFlowConfig | null,
  detail: MatchDetailResponse | null,
): string {
  if (!config) return '匹配阈值与命中数值均未取到（匹配配置与 match-detail 不可得）'
  if (reason === 'composite_hit') return '本文标签未挂本版块组合标签'
  if (reason === 'direct_hit') {
    return detail
      ? `最强标签交集 ${detail.direct_hit_auxiliaries?.length ?? 0} 个 < ${config.directHitMinOverlap}`
      : `需辅助标签交集 ≥ ${config.directHitMinOverlap}，本文标签未达`
  }
  if (reason === 'hit_rate') {
    return detail
      ? `最强标签 R=${formatScore(detail.hit_rate, 3)} ≤ ${formatScore(config.directHitRate)}`
      : `需命中率 R > ${formatScore(config.directHitRate)}，本文标签未达`
  }
  if (reason === 'max_sim') {
    return detail
      ? `最强标签 Smax=${formatScore(detail.max_similarity, 3)}、${detail.hits} 命中、R=${formatScore(detail.hit_rate, 3)}，三条件未全满足`
      : `需 Smax≥${formatScore(config.directMaxSim)} 且命中≥${config.directMaxSimMinHits} 且 R≥${formatScore(config.directMaxSimMinHitRate)}，未全达`
  }
  return detail
    ? `最强标签加权分 ${formatScore(detail.score, 3)} < ${formatScore(config.weightedThreshold)}`
    : `需加权分 ≥ ${formatScore(config.weightedThreshold)}，本文标签未达`
}

/** 组合命中链（如「伊美关系 = 伊朗 × 美国」）：label + 有序组件序列。 */
export function compositeChainLines(detail: MatchDetailResponse | null): string[] {
  return (detail?.composite_hits ?? []).map((hit) => {
    const chain = [...hit.components]
      .sort((a, b) => a.position - b.position)
      .map(c => c.label)
      .join(' × ')
    return `${hit.label} = ${chain}`
  })
}

// ── KaTeX 公式构造（来源：features/tags/components/MatchDetailPanel.vue）────────

/** 主公式（按判定规则分形态）。 */
export function primaryFormula(item: MatchDetailResponse): string {
  if (item.match_reason === 'composite_hit') return '\\text{score}=1.0\\ (\\text{组合交集}\\neq\\varnothing)'
  if (item.match_reason === 'direct_hit') return `\\text{score}=f_{\\text{direct}}\\times 1=${formatScore(item.config.direct_hit_score_factor, 2)}`
  if (item.match_reason === 'hit_rate') return '\\text{score}=\\alpha\\cdot S_{\\max}+(1-\\alpha)\\cdot R'
  if (item.match_reason === 'max_sim') return '\\text{score}=S_{\\max}'
  if (item.match_reason === 'weighted') return '\\text{score}=w_{\\text{sim}}\\cdot S_{\\max}+w_{\\text{density}}\\cdot R'
  return ''
}

/** 代入式（把真实参数代进公式）。 */
export function substitutionFormula(item: MatchDetailResponse): string {
  const score = formatScore(item.score, 3)
  const maxSim = formatScore(item.max_similarity, 3)
  const rate = formatScore(item.hit_rate, 3)
  if (item.match_reason === 'hit_rate') {
    const alpha = formatScore(item.config.hit_rate_sim_blend, 2)
    return `\\text{score}=${alpha}\\times ${maxSim}+${formatScore(1 - item.config.hit_rate_sim_blend, 2)}\\times ${rate}=${score}`
  }
  if (item.match_reason === 'max_sim') {
    return `S_{\\max}=${maxSim}\\ge ${formatScore(item.config.direct_max_sim, 2)},\\ hits\\ge ${Math.min(item.config.direct_max_sim_min_hits, item.tag_auxiliary_count)},\\ R\\ge ${formatScore(item.config.direct_max_sim_min_hit_rate, 2)}`
  }
  if (item.match_reason === 'weighted') {
    return `\\text{score}=${formatScore(item.config.weight_sim, 2)}\\times ${maxSim}+${formatScore(item.config.weight_density, 2)}\\times ${rate}=${score}`
  }
  return ''
}

/** 命中率公式（R = hits / max(N, s)）。 */
export function rateFormula(item: MatchDetailResponse): string {
  return `R=\\frac{\\text{hits}}{\\max(N,s)}=\\frac{${item.hits}}{\\max(${item.tag_auxiliary_count},${item.config.min_effective_sample})}=${formatScore(item.hit_rate, 3)}`
}

// ── 实时重算 vs 冻结 score（R-05：不得拿当前规则的实时重算冒充历史明细）──────

/** 代入式里的数值口径标注（如实说明哪些是实时值、哪个是冻结值）。 */
export const LIVE_CALC_NOTE = 'S_max / R / 交集 为按当前匹配配置实时计算；score 为匹配时冻结值'

/** 实时重算值与冻结 score 的差异阈值（超过即视为不一致）。 */
const SCORE_DRIFT_EPSILON = 0.001

/**
 * 按**当前**配置把实时值代入公式算出的分数。
 *
 * match-detail 的 hits / hit_rate / max_similarity 是按当前 auxiliaries + 当前
 * config **现场重算**的，而 score 是匹配时的**冻结值**——两者可能对不上。
 * 只覆盖代入式会写成等式的乘加类公式（hit_rate / weighted）；不可算 → null。
 */
export function recomputedScore(item: MatchDetailResponse): number | null {
  const smax = item.max_similarity
  const rate = item.hit_rate
  if (!Number.isFinite(smax) || !Number.isFinite(rate)) return null
  if (item.match_reason === 'hit_rate') {
    const alpha = num(item.config.hit_rate_sim_blend, 0.5)
    return alpha * smax + (1 - alpha) * rate
  }
  if (item.match_reason === 'weighted') {
    return num(item.config.weight_sim, 0.7) * smax + num(item.config.weight_density, 0.3) * rate
  }
  return null
}

/**
 * 一致性提示（重算值与冻结 score 不一致时给，否则空串）。
 *
 * 不一致意味着页面上那条等式不成立（阈值或辅助标签变过）——必须如实标出来。
 */
export function scoreDriftNote(item: MatchDetailResponse | null | undefined): string {
  if (!item) return ''
  const computed = recomputedScore(item)
  if (computed === null || !Number.isFinite(item.score)) return ''
  if (Math.abs(computed - item.score) <= SCORE_DRIFT_EPSILON) return ''
  return '⚠ 重算值与冻结 score 不一致（配置或标签已变动）'
}
