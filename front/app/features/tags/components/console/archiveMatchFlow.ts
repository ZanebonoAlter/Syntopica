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
  state: 'matched' | 'failed' | 'compute' | 'pending' | 'skipped'
  /** 编号（⓪→⑤），来自 RULE_COPY 单一常量源。 */
  no: string
  /** 规则名（不含编号），来自 RULE_COPY。 */
  name: string
  /** 人话问法（不含阈值，阈值由消费方按真实 config 代入），来自 RULE_COPY。 */
  question: string
  /** 是否走到本步（matched/failed/compute=true，skipped=false；generic 说明步一律 false）。 */
  reached: boolean
}

/** 规则元数据单一常量源（D5）：popover 判定过程、工具行「匹配流程」弹窗、入板阶段名共用，编号/规则名/问法/短名不得在别处另写一套。 */
export interface RuleCopyEntry {
  id: string
  /** 编号（⓪→⑤）。 */
  no: string
  /** 规则名（不含编号）。 */
  name: string
  /** 人话问法（阈值由消费方按真实 config 代入）。 */
  question: string
  /** 短名（入板过程五阶段、列表 chip 用）。 */
  short: string
}

/** 规则元数据常量（顺序 ⓪→①→②→③→④→⑤，② 为中间计算步）。 */
export const RULE_COPY: readonly RuleCopyEntry[] = [
  { id: 'composite_hit', no: '⓪', name: '组合命中', question: '事件与版块挂载了同一组合标签？', short: '组合命中' },
  { id: 'direct_hit', no: '①', name: '精确匹配', question: '事件和版块有多少个辅助标签完全相同？', short: '精确匹配' },
  { id: 'similarity', no: '②', name: '算余弦相似度', question: '把事件的辅助标签和版块标签两两比较「有多像」', short: '算余弦相似度' },
  { id: 'hit_rate', no: '③', name: '命中率规则', question: '「几分之几」的标签像版块的？', short: '命中率' },
  { id: 'max_sim', no: '④', name: '最高相似度规则', question: '最像的那一对有多像？需同时满足三个条件（Smax / 命中数 / R）', short: '最高相似度' },
  { id: 'weighted', no: '⑤', name: '综合加权', question: '整体看像不像？把相似度和命中率加权', short: '综合加权' },
]

/** 按 id 取规则元数据（RULE_COPY 的唯一查询口）。 */
export function ruleOf(id: string): RuleCopyEntry | undefined {
  return RULE_COPY.find(rule => rule.id === id)
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

/** ① 交集数（口径=direct_hit_auxiliaries 的 tag_auxiliary_id 去重计数，唯一实现，组件/结论行共用）。 */
export function overlapOf(detail: MatchDetailResponse | null): number {
  if (!detail) return 0
  return new Set(detail.direct_hit_auxiliaries?.map(d => d.tag_auxiliary_id) ?? []).size
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
 * 规则名 / 编号 / 问法一律取自 RULE_COPY（与 popover 判定过程同源，D5）。
 */
export function buildGenericFlowSteps(config: MatchFlowConfig): MatchFlowStep[] {
  const by = (id: string, descExtra: string, result: string, state: MatchFlowStep['state'] = 'pending'): MatchFlowStep => {
    const rule = ruleOf(id)!
    return {
      id,
      title: `${rule.no} ${rule.name}`,
      desc: rule.question + descExtra,
      result,
      state,
      no: rule.no,
      name: rule.name,
      question: rule.question,
      reached: false,
    }
  }
  return [
    by('composite_hit', '', '组合标签交集非空 → 直接命中（免方向校验）'),
    by('direct_hit', '', `交集 ≥ ${config.directHitMinOverlap} 个 → 命中`),
    by('similarity', `，相似度 ≥ ${formatScore(config.simThreshold)} 算命中`, `中间计算步：产出命中数 / 标签总数 → 命中率 R、最高相似度 Smax（最小样本量 ${config.minEffectiveSample}）`, 'compute'),
    by('hit_rate', `，超过 ${(config.directHitRate * 100).toFixed(0)}% 就挂载`, `R > ${formatScore(config.directHitRate)} → 命中`),
    by('max_sim', '', `Smax ≥ ${formatScore(config.directMaxSim)} 且命中 ≥ ${config.directMaxSimMinHits} 且 R ≥ ${formatScore(config.directMaxSimMinHitRate)} → 命中`),
    by('weighted', '', `${formatScore(config.weightSim)}×Smax + ${formatScore(config.weightDensity)}×R ≥ ${formatScore(config.weightedThreshold)} → 命中`),
  ]
}

/** 方向校验说明（通用流程末尾的一行真话）。 */
export function directionCheckNote(config: MatchFlowConfig): string {
  return `方向校验：标签与版块的方向相似度需 ≥ ${formatScore(config.directionSimThreshold)}；未通过的匹配默认隐藏、不计入日报。`
}

/** 命中即停说明（通用流程末尾的一行真话）。 */
export const HIT_AND_STOP_NOTE = '命中即停：按 ⓪→①→②→③→④→⑤ 顺序判定，首个满足的规则决定结果，后面的规则不再看。'

/**
 * 逐标签判定过程（供 popover「判定过程」块逐条消费，D1/D2/A2）。
 *
 * 全集输出：走过的步给真实数值（matched/failed/compute），命中步之后的步标
 * skipped（reached=false，组件合并成一排「未走到」）；⓪ 仅 composite_hit 时产出。
 * direct_hit 命中时仍补产 ② compute 参照步（指标行需要 R/Smax 参照）；
 * composite_hit 命中不产 ②（组合交集即命中、免相似度计算）。
 * 真实数值不可得时（match-detail 缺失）给规则 + 阈值 + 「需实时拉取」，不伪造。
 * 口径：① 交集标「（完全相同）」，② 参照行标「相似达标 x/N」——与收编口径分家。
 */
export function buildTagFlowSteps(
  reason: string,
  config: MatchFlowConfig,
  detail: MatchDetailResponse | null,
  tagAuxCount: number,
): MatchFlowStep[] {
  const needLive = '（该标签命中数值需实时拉取）'
  const real = detail !== null
  const N = real ? detail.tag_auxiliary_count : tagAuxCount
  const overlap = overlapOf(detail)

  const descOf = (id: string): string => {
    const q = ruleOf(id)!.question
    if (id === 'similarity') return `把事件的 ${N} 个辅助标签和版块标签两两比较「有多像」，相似度 ≥ ${formatScore(config.simThreshold)} 算命中`
    if (id === 'hit_rate') return `${q}，超过 ${(config.directHitRate * 100).toFixed(0)}% 就挂载`
    return q
  }
  const step = (id: string, result: string, state: MatchFlowStep['state']): MatchFlowStep => {
    const rule = ruleOf(id)!
    return { id, title: `${rule.no} ${rule.name}`, desc: descOf(id), result, state, no: rule.no, name: rule.name, question: rule.question, reached: state !== 'skipped' }
  }
  const skippedFrom = (id: string): MatchFlowStep => step(id, '未走到', 'skipped')

  if (reason === 'composite_hit') {
    const chain = real && (detail.composite_hits?.length ?? 0) > 0
      ? `${detail.composite_hits!.map(h => h.label).join('、')} → 直接命中（免方向校验）`
      : `组合标签交集非空 → 直接命中（免方向校验）${real ? '' : needLive}`
    return [step('composite_hit', chain, 'matched'), skippedFrom('direct_hit'), skippedFrom('similarity'), skippedFrom('hit_rate'), skippedFrom('max_sim'), skippedFrom('weighted')]
  }

  const steps: MatchFlowStep[] = []

  // ① 精确匹配（口径：交集 = 辅助标签 ID 完全相同，与收编「相似达标」分家）
  steps.push(reason === 'direct_hit'
    ? step('direct_hit', real
      ? `交集 ${overlap} 个（完全相同）≥ ${config.directHitMinOverlap} → 满足`
      : `需交集 ≥ ${config.directHitMinOverlap} 个（完全相同）${needLive}`, 'matched')
    : step('direct_hit', real
      ? `交集 ${overlap} 个（完全相同）< ${config.directHitMinOverlap}，不够 → 继续`
      : `需交集 ≥ ${config.directHitMinOverlap} 个（完全相同）${needLive}`, 'failed'))

  // ② 算余弦相似度（中间计算步：走到 ③ 及之后必需；direct_hit 命中时作指标参照保留）
  steps.push(step('similarity', real
    ? `相似达标 ${detail.hits}/${N} · R ${formatScore(detail.hit_rate)} · Smax ${formatScore(detail.max_similarity)}`
    : `相似达标数 / R / Smax${needLive}`, 'compute'))

  if (reason === 'direct_hit') {
    steps.push(skippedFrom('hit_rate'), skippedFrom('max_sim'), skippedFrom('weighted'))
    return steps
  }

  if (reason === 'hit_rate') {
    steps.push(step('hit_rate', real
      ? `R=${formatScore(detail.hit_rate, 3)} > ${formatScore(config.directHitRate)} → 满足`
      : `需 R > ${formatScore(config.directHitRate)}${needLive}`, 'matched'))
    steps.push(skippedFrom('max_sim'), skippedFrom('weighted'))
    return steps
  }

  steps.push(step('hit_rate', real
    ? `R=${formatScore(detail.hit_rate, 3)} ≤ ${formatScore(config.directHitRate)} → 不够，继续`
    : `需 R > ${formatScore(config.directHitRate)}${needLive}`, 'failed'))

  const minHits = Math.min(config.directMaxSimMinHits, N || 1)
  if (reason === 'max_sim') {
    steps.push(step('max_sim', real
      ? `Smax=${formatScore(detail.max_similarity, 3)} ≥ ${formatScore(config.directMaxSim)}，${detail.hits} ≥ ${minHits}，R=${formatScore(detail.hit_rate, 3)} → 满足`
      : `需 Smax ≥ ${formatScore(config.directMaxSim)} 且命中 ≥ ${minHits} 且 R ≥ ${formatScore(config.directMaxSimMinHitRate)}${needLive}`, 'matched'))
    steps.push(skippedFrom('weighted'))
    return steps
  }

  steps.push(step('max_sim', real
    ? `Smax=${formatScore(detail.max_similarity, 3)}、${detail.hits}/${minHits}、R=${formatScore(detail.hit_rate, 3)} → 未全满足，继续`
    : `需三条件全满足${needLive}`, 'failed'))

  if (reason === 'weighted') {
    steps.push(step('weighted', real
      ? `${formatScore(config.weightSim)}×${formatScore(detail.max_similarity, 3)}+${formatScore(config.weightDensity)}×${formatScore(detail.hit_rate, 3)}=${formatScore(detail.score, 3)} ≥ ${formatScore(config.weightedThreshold)} → 满足`
      : `需加权分 ≥ ${formatScore(config.weightedThreshold)}${needLive}`, 'matched'))
  } else {
    // 未知 reason：如实标未匹配（不伪造 skipped / 命中态）
    steps.push(step('weighted', real
      ? `加权分 ${formatScore(detail.score, 3)} < ${formatScore(config.weightedThreshold)} → 不匹配`
      : `需加权分 ≥ ${formatScore(config.weightedThreshold)}${needLive}`, 'failed'))
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
