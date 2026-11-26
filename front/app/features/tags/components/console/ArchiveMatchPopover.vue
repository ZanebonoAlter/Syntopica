<script setup lang="ts">
/**
 * 归档匹配 popover（rework-workshop-into-radar-and-desk 8G.3c，2026-10-31 归档重做）。
 *
 * 一个组件两种模式（原型 ui-prototype/revision-2026-10-31-archive-rework）：
 *  - tag 模式：点行内标签 chip 打开的「标签卡」——标签名 / 类别 / 规则 / 分数 /
 *    {类别}描述 / 本文标签层 / 组合链 / 辅助标签×版块收编 ✓✗ / 私有判定行 /
 *    规则代入式（KaTeX，公式构造来源见 archiveMatchFlow.ts 头注释）；
 *  - article 模式：点行内「入板」chip 打开的「入板过程」——⓪①③④⑤ 五阶段分组得分 /
 *    命中步之前的未命中原因 / 命中步之后的「未走到」/ 末行本文全部标签。
 *
 * 锚定形态：fixed 定位贴 chip（右侧优先、下半屏上翻、视口 clamp），Esc 与点外关闭，
 * 窄屏（<768px）改全宽底部档。数据按需拉（打开才请求），请求竞态用序号保护，
 * 缺数据如实标注、不伪造。
 */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import KaTeXRender from '~/components/KaTeXRender.vue'
import { useArticlesApi } from '~/api/articles'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import type { ArticleTag } from '~/types'
import type { AuxiliaryLabelItem, BoardArticleTag, MatchDetailResponse, RelatedEventsResponse } from '~/api/semanticBoards'
import { catLabel, matchReasonColor } from '~/utils/matchQuality'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import { Z_INDEX, nestedOverlayZ } from '~/components/ui/layout-contract'
import { isWeakArticle, type ArchiveArticle } from '../../composables/useBoardArchive'
import {
  buildTagFlowSteps,
  compositeChainLines,
  flowConfigFromApi,
  flowConfigFromDetail,
  formatScore,
  LIVE_CALC_NOTE,
  overlapOf,
  primaryFormula,
  rateFormula,
  ruleOf,
  scoreDriftNote,
  substitutionFormula,
  whyMissText,
  type MatchFlowConfig,
  type MatchFlowStep,
} from './archiveMatchFlow'

const props = defineProps<{
  boardId: number
  /** tag = 标签卡；article = 入板过程。 */
  mode: 'tag' | 'article'
  article: ArchiveArticle | null
  /** 标签卡模式下的命中标签（入板模式传 null）。 */
  tag: BoardArticleTag | null
  /** 锚 chip 元素（定位用）。 */
  anchor: HTMLElement | null
}>()

const emit = defineEmits<{
  close: []
  /** 相关事件「查看来源」：交给宿主既有阅读链路（ArticlePreviewModal），不在此另开弹窗。 */
  'open-article': [articleId: number]
}>()

const sbApi = useSemanticBoardsApi()
const articlesApi = useArticlesApi()

const articleTags = ref<ArticleTag[]>([])
const auxLabels = ref<AuxiliaryLabelItem[]>([])
/** 版块辅助池 id（match-detail 不可得时，「id 直接重合」标 ✓ 的依据）。 */
const boardPoolIds = ref<Set<number>>(new Set())
const detail = ref<MatchDetailResponse | null>(null)
const flowConfig = ref<MatchFlowConfig | null>(null)
const loading = ref(false)
const calcOpen = ref(false)
/** 未取到的数据块（如实标注，不伪造）。 */
const missingParts = ref<string[]>([])

const isNarrow = useIsNarrowViewport()
const popRef = ref<HTMLElement | null>(null)
const position = ref({ left: 0, top: 0 })

const MASK_Z = Z_INDEX.dialog
const POP_Z = nestedOverlayZ(Z_INDEX.dialog)

/**
 * 本文最强判定标签 = `filtered_tags[0]`。
 *
 * 后端已按归档证据层级（ArchiveEvidenceRank）→ 分数 → id 排好序，首条即本文
 * 最强证据标签；前端**信后端序**，不按最高分另推一套口径。
 */
const strongestTag = computed<BoardArticleTag | null>(() => {
  const tags = props.article?.filtered_tags ?? []
  return tags[0] ?? null
})

/** 本文是否弱证据（后端派生字段优先）。 */
const weakArticle = computed(() => (props.article ? isWeakArticle(props.article) : false))

/** 最强标签徽标文案：弱证据文章如实标「弱证据命中」，强证据才叫「入板依据」。 */
const bestTagBadge = computed(() => (weakArticle.value ? '弱证据命中（不计入强证据）' : '入板依据'))

/** 代入式/判定过程的数值口径标注（实时重算 vs 冻结 score）——全 popover 仅渲染这一处（2.5 收敛）。 */
const liveCalcNote = computed(() => (detail.value ? LIVE_CALC_NOTE : ''))
const driftNote = computed(() => scoreDriftNote(detail.value))

const ruleEntry = computed(() => (props.tag ? ruleOf(props.tag.match_reason) : undefined))
const ruleColor = computed(() => (props.tag ? matchReasonColor(props.tag.match_reason) : 'var(--color-text-muted)'))
const scoreText = computed(() => (props.tag ? formatScore(props.tag.score) : '--'))

/** 头区规则名：第①步 精确匹配 命中（同源 RULE_COPY，不用旧「直接命中」标签）。 */
const headStepText = computed(() => (ruleEntry.value ? `第${ruleEntry.value.no}步 ${ruleEntry.value.name} 命中` : '·'))

/** 指标行（口径=相似 ≥ 阈值的标签对数 hits，与 ① 交集「完全相同」分家；缺数值给 —，不伪造 0.00）。 */
const metricText = computed(() => {
  const det = detail.value
  const isComposite = props.tag?.match_reason === 'composite_hit'
  if (!det || isComposite) return '相似达标 —/— · R — · Smax —'
  const N = det.tag_auxiliary_count
  return `相似达标 ${det.hits}/${N} · R ${formatScore(det.hit_rate)} · Smax ${formatScore(det.max_similarity)}`
})

/** 组合链（composite_hit 时才有真实推导链）。 */
const compositeChains = computed(() => compositeChainLines(detail.value))

interface AuxRow {
  id: number
  label: string
  state: 'hit' | 'miss' | 'unknown'
  pairLabel?: string
  similarity?: number
}

/**
 * 辅助标签 × 版块收编层。
 *
 * match-detail 可得 → 用真实逐对（取该辅助标签的最相似对，sim ≥ 阈值算 ✓）；
 * 不可得 → 只把「id 与版块直接命中辅助标签重合」的标 ✓，其余标 ·（需实时计算）。
 */
const auxRows = computed<AuxRow[]>(() => {
  const pairs = detail.value?.pairs ?? []
  const directHits = detail.value?.direct_hit_auxiliaries ?? []
  const threshold = detail.value?.config?.sim_threshold
  return auxLabels.value.map((item) => {
    const candidates = pairs.filter(p => p.tag_auxiliary_id === item.id)
    if (candidates.length > 0) {
      const best = candidates.reduce((a, b) => (b.similarity > a.similarity ? b : a))
      const hit = typeof threshold === 'number' ? best.similarity >= threshold : best.is_hit
      return {
        id: item.id,
        label: item.label,
        state: hit ? 'hit' : 'miss',
        pairLabel: best.board_auxiliary_label,
        similarity: best.similarity,
      }
    }
    const direct = directHits.find(d => d.tag_auxiliary_id === item.id)
    if (direct) {
      return { id: item.id, label: item.label, state: 'hit', pairLabel: direct.board_label }
    }
    // 无逐对数据：id 与版块辅助池直接重合标 ✓（真话），其余标 ·（需实时计算）
    if (boardPoolIds.value.has(item.id)) {
      return { id: item.id, label: item.label, state: 'hit' }
    }
    return { id: item.id, label: item.label, state: 'unknown' }
  })
})

const auxHitCount = computed(() => auxRows.value.filter(row => row.state === 'hit').length)
const auxSummary = computed(() => (detail.value ? `${auxHitCount.value}/${auxRows.value.length}` : '—'))

/** 判定过程（D1/D2/A2）：全集输出里走到/命中的步逐条列，命中步之后的步合并一排。 */
const verdictSteps = computed<MatchFlowStep[]>(() => {
  const tag = props.tag
  if (!tag || !flowConfig.value || loading.value) return []
  return buildTagFlowSteps(tag.match_reason, flowConfig.value, detail.value, auxLabels.value.length)
})
const walkedSteps = computed(() => verdictSteps.value.filter(step => step.state !== 'skipped'))
const skippedSteps = computed(() => verdictSteps.value.filter(step => step.state === 'skipped'))
const matchedStep = computed(() => verdictSteps.value.find(step => step.state === 'matched') ?? null)

/** 结论行（2.3）：一句话因果，规则名与得分同源；无数值时如实写需实时拉取。 */
const verdictLine = computed(() => {
  const tag = props.tag
  if (!tag || !ruleEntry.value) return ''
  const no = ruleEntry.value.no
  const name = ruleEntry.value.name
  if (!detail.value || !flowConfig.value) {
    return `本文判定走 ${no}${name}，该步命中数值需实时拉取`
  }
  const det = detail.value
  const N = det.tag_auxiliary_count
  const score = formatScore(tag.score)
  if (tag.match_reason === 'composite_hit') {
    const chains = compositeChainLines(det)
    const chainText = chains.length ? `「${chains[0]}」` : '（组合标签交集非空）'
    return `本文与版块共挂组合标签${chainText} → ${no}${name}即停，得分 ${score}`
  }
  if (tag.match_reason === 'direct_hit') {
    const overlap = overlapOf(det)
    return `本文 ${N} 个辅助标签里 ${overlap} 个与版块完全相同 → ${no}${name}（≥ ${flowConfig.value.directHitMinOverlap}）命中，得分 ${score}`
  }
  if (tag.match_reason === 'hit_rate') {
    return `本文 ${N} 个辅助标签里相似达标 ${det.hits} 个（R ${formatScore(det.hit_rate)} > ${formatScore(flowConfig.value.directHitRate)}）→ ${no}${name}命中，得分 ${score}`
  }
  if (tag.match_reason === 'max_sim') {
    return `本文最相似一对 Smax ${formatScore(det.max_similarity)} ≥ ${formatScore(flowConfig.value.directMaxSim)} → ${no}${name}命中，得分 ${score}`
  }
  return `本文加权分 ${formatScore(det.score)} ≥ ${formatScore(flowConfig.value.weightedThreshold)} → ${no}${name}命中，得分 ${score}`
})

interface StageView {
  reason: string
  items: BoardArticleTag[]
  skipped: boolean
  why: string
}

/** 入板过程五阶段（⓪①③④⑤）。命中步之前空阶段给未命中原因，之后标「未走到」。 */
const stages = computed<StageView[]>(() => {
  const tags = props.article?.filtered_tags ?? []
  const best = strongestTag.value
  const order = ['composite_hit', 'direct_hit', 'hit_rate', 'max_sim', 'weighted']
  const bestIdx = best ? order.indexOf(best.match_reason) : -1
  return order.map((reason, index) => ({
    reason,
    items: tags.filter(tag => tag.match_reason === reason),
    skipped: bestIdx >= 0 && index > bestIdx,
    why: whyMissText(reason, flowConfig.value, detail.value),
  }))
})

const bestTagId = computed(() => strongestTag.value?.id ?? null)
/** 入板阶段名同源（D5）：编号/短名一律取 RULE_COPY，不再各写一套。 */
const bestStepNo = computed(() => (strongestTag.value ? ruleOf(strongestTag.value.match_reason)?.no ?? '·' : '·'))

/** 通用流程说明（入板模式纯数据，不重复流程叙述——通用流程在工具行「匹配流程」）。 */
/** 收编标题的口径词阈值（相似 ≥ x.xx，非完全相同；缺配置给 —）。 */
const simThresholdText = computed(() => (flowConfig.value ? formatScore(flowConfig.value.simThreshold) : '—'))

/** 当前匹配参数 12 项（沿用旧版 MatchDetailPanel 的文案，来源 flowConfig 同字段）。 */
const configRows = computed(() => {
  const cfg = flowConfig.value
  if (!cfg) return []
  return [
    { label: 'sim_threshold（相似度命中线）', value: formatScore(cfg.simThreshold) },
    { label: 'direct_hit_min_overlap（精确匹配最少交集）', value: `${cfg.directHitMinOverlap}` },
    { label: 'direct_hit_rate（命中率挂载线）', value: formatScore(cfg.directHitRate) },
    { label: 'hit_rate_sim_blend（命中率混相似度权重）', value: formatScore(cfg.hitRateSimBlend) },
    { label: 'min_effective_sample（最小样本量）', value: `${cfg.minEffectiveSample}` },
    { label: 'direct_max_sim（最高相似度挂载线）', value: formatScore(cfg.directMaxSim) },
    { label: 'direct_max_sim_min_hits（最高相似度最少命中）', value: `${cfg.directMaxSimMinHits}` },
    { label: 'direct_max_sim_min_hit_rate（最高相似度最低命中率）', value: formatScore(cfg.directMaxSimMinHitRate) },
    { label: 'direction_sim_threshold（方向校验阈值）', value: formatScore(cfg.directionSimThreshold) },
    { label: 'weight_sim（加权相似度比重）', value: formatScore(cfg.weightSim) },
    { label: 'weight_density（加权命中率比重）', value: formatScore(cfg.weightDensity) },
    { label: 'weighted_threshold（加权挂载线）', value: formatScore(cfg.weightedThreshold) },
  ]
})

/** 本文标签 chips（构成标签 = filtered_tags 命中的那些，高亮）。 */
const articleTagViews = computed(() => {
  const hitIds = new Set((props.article?.filtered_tags ?? []).map(tag => tag.id))
  const currentId = props.mode === 'tag' ? props.tag?.id : undefined
  return articleTags.value
    .filter(tag => currentId === undefined || tag.id !== currentId)
    .map(tag => ({
      key: tag.id ?? tag.slug,
      label: tag.label,
      category: tag.category,
      matched: typeof tag.id === 'number' ? hitIds.has(tag.id) : false,
    }))
})

// ── 相关事件（只有 event 标签卡才请求；按需展开、迟到响应丢弃）───────────────
//
// 契约（enable-story-cross-day-continuation ui-design.md）：
//  - 非 event 标签 / 入板过程模式 → 一个请求都不发；
//  - 展开才查询，一次展开一次请求；关闭或切换标签后迟到响应不得覆盖新状态；
//  - ready 空集 / partial / not_ready / 查询失败分别有可区分的文案，缺索引不冒充空集；
//  - 条目只用接口给的顺序，展示原名称/描述/相似度；来源走既有文章预览，无来源明示禁用。

const relatedOpen = ref(false)
const relatedLoading = ref(false)
const relatedError = ref<string | null>(null)
const relatedData = ref<RelatedEventsResponse | null>(null)
const expandedRelatedIds = ref<Set<number>>(new Set())
let relatedSeq = 0

const isEventTagCard = computed(() => props.mode === 'tag' && props.tag?.category === 'event')

const relatedItems = computed(() => relatedData.value?.items ?? [])

const relatedCoverageText = computed(() => {
  const data = relatedData.value
  if (!data) return ''
  return `候选 ${data.candidate_count} · 已索引 ${data.indexed_count} · 阈值 ${formatScore(data.threshold)}`
})

function resetRelated(): void {
  relatedSeq++
  relatedOpen.value = false
  relatedLoading.value = false
  relatedError.value = null
  relatedData.value = null
  expandedRelatedIds.value = new Set()
}

async function toggleRelated(): Promise<void> {
  if (!isEventTagCard.value || !props.tag) return
  if (relatedOpen.value) {
    relatedOpen.value = false
    return
  }
  relatedOpen.value = true
  await loadRelated()
}

async function loadRelated(): Promise<void> {
  const tag = props.tag
  if (!isEventTagCard.value || !tag) return
  const seq = ++relatedSeq
  relatedLoading.value = true
  relatedError.value = null
  try {
    const res = await sbApi.getRelatedEvents(props.boardId, tag.id)
    if (seq !== relatedSeq) return
    if (!res?.success || !res.data) {
      // 成败都用可读中文（底层 fetch 的 "Failed to fetch" 不直接给用户看）。
      relatedData.value = null
      relatedError.value = '相关事件查询失败，请重试'
      return
    }
    relatedData.value = res.data
  } catch {
    if (seq !== relatedSeq) return
    relatedData.value = null
    relatedError.value = '相关事件查询失败，请重试'
  } finally {
    if (seq === relatedSeq) relatedLoading.value = false
  }
}

function toggleRelatedDescription(tagId: number): void {
  const next = new Set(expandedRelatedIds.value)
  if (next.has(tagId)) next.delete(tagId)
  else next.add(tagId)
  expandedRelatedIds.value = next
}

function openRelatedSource(articleId: number | undefined): void {
  if (typeof articleId !== 'number') return
  emit('open-article', articleId)
}

// ── 数据按需拉取（打开才请求；序号保护竞态）────────────────────────────────

let requestSeq = 0

async function load(): Promise<void> {
  const seq = ++requestSeq
  const articleId = props.article?.id
  const tag = props.tag
  if (!articleId) {
    loading.value = false
    return
  }

  loading.value = true
  calcOpen.value = false
  articleTags.value = []
  auxLabels.value = []
  boardPoolIds.value = new Set()
  detail.value = null
  flowConfig.value = null
  missingParts.value = []
  resetRelated()

  const missing: string[] = []
  const articleTask = articlesApi.getArticle(articleId)
    .then((res) => {
      if (!res?.success || !res.data) {
        missing.push('本文标签')
        return [] as ArticleTag[]
      }
      return normalizeArticle(res.data as unknown as ArticlePayload).tags ?? []
    })
    .catch(() => {
      missing.push('本文标签')
      return [] as ArticleTag[]
    })

  let auxTask: Promise<AuxiliaryLabelItem[]> = Promise.resolve([])
  let detailTask: Promise<MatchDetailResponse | null> = Promise.resolve(null)

  if (props.mode === 'tag' && tag) {
    auxTask = sbApi.getTagAuxiliaryLabels(tag.id)
      .then((res) => {
        if (!res?.success) {
          missing.push('辅助标签')
          return [] as AuxiliaryLabelItem[]
        }
        return res.data?.items ?? []
      })
      .catch(() => {
        missing.push('辅助标签')
        return [] as AuxiliaryLabelItem[]
      })
    detailTask = sbApi.getMatchDetail(props.boardId, tag.id)
      .then((res) => (res?.success && res.data ? res.data : null))
      .catch(() => null)
  } else {
    const best = strongestTag.value
    if (best) {
      detailTask = sbApi.getMatchDetail(props.boardId, best.id)
        .then((res) => (res?.success && res.data ? res.data : null))
        .catch(() => null)
    }
  }

  const [tags, aux, det] = await Promise.all([articleTask, auxTask, detailTask])
  if (seq !== requestSeq) return

  articleTags.value = tags
  auxLabels.value = aux
  detail.value = det
  flowConfig.value = det ? flowConfigFromDetail(det.config) : null

  if (!det) {
    // 逐对明细不可得时：阈值口径退回全局匹配配置（真实值）；两者都不可得则如实标缺失。
    const global = await sbApi.getMatchingConfig()
      .then(res => (res?.success ? res.data : null))
      .catch(() => null)
    if (seq !== requestSeq) return
    if (global) flowConfig.value = flowConfigFromApi(global)
    else missing.push('匹配阈值')
    missing.push('逐对匹配明细')
    // 「id 直接重合标 ✓」需要版块辅助池：拉版块构成（既有端点，不新造）。
    if (props.mode === 'tag' && tag) {
      try {
        const composition = await sbApi.getComposition(props.boardId)
          .then(res => (res?.success ? res.data : null))
          .catch(() => null)
        if (seq !== requestSeq) return
        boardPoolIds.value = new Set((composition?.items ?? []).map(item => item.id))
      } catch {
        // 构成拉不到就如实标 ·（不伪造收编关系）
      }
    }
  }

  missingParts.value = missing
  loading.value = false
  await syncPosition()
}

// ── 定位（fixed：右侧优先 / 下半屏上翻 / 视口 clamp）────────────────────────

const popStyle = computed(() => {
  if (isNarrow.value) return { zIndex: POP_Z }
  return { zIndex: POP_Z, left: `${position.value.left}px`, top: `${position.value.top}px` }
})

function reposition(): void {
  if (isNarrow.value) return
  const el = popRef.value
  const anchor = props.anchor
  if (!el || !anchor) return
  const rect = anchor.getBoundingClientRect()
  const pw = el.offsetWidth || 320
  const ph = el.offsetHeight || 320
  const vw = window.innerWidth || 1024
  const vh = window.innerHeight || 768
  const m = 8
  const x = Math.min(Math.max(rect.right + m, m), Math.max(vw - pw - m, m))
  let y = rect.bottom + m + ph > vh - m ? Math.max(rect.top - ph - m, m) : rect.top
  y = Math.min(Math.max(y, m), Math.max(vh - ph - m, m))
  position.value = { left: x, top: y }
}

async function syncPosition(): Promise<void> {
  await nextTick()
  reposition()
  if (typeof window.requestAnimationFrame === 'function') {
    window.requestAnimationFrame(() => reposition())
  }
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  // 上层阅读弹窗（ArticlePreviewModal，aria-modal=true）打开时，Esc 属于它：
  // 关闭预览后回到本卡（ui-design 合同），不能被同一个 Esc 连带关掉卡片。
  if (document.querySelector('[aria-modal="true"]')) return
  emit('close')
}

watch([() => props.mode, () => props.tag?.id, () => props.article?.id], () => {
  void load()
}, { immediate: true })

watch(isNarrow, () => { void syncPosition() })

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', reposition)
  window.addEventListener('scroll', reposition, true)
  void syncPosition()
})

onUnmounted(() => {
  relatedSeq++
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', reposition)
  window.removeEventListener('scroll', reposition, true)
})
</script>

<template>
  <Teleport to="body">
    <div class="amp-mask" :style="{ zIndex: MASK_Z }" data-testid="archive-pop-mask" @click="emit('close')" />
    <aside
      ref="popRef"
      class="amp-pop"
      :class="{ 'is-narrow': isNarrow }"
      :style="popStyle"
      role="dialog"
      aria-label="归档匹配详情"
      data-testid="archive-match-popover"
    >
      <!-- 标签卡模式 -->
      <template v-if="mode === 'tag' && tag">
        <header class="amp-head" :style="{ borderLeftColor: ruleColor }" data-testid="archive-pop-head">
          <span class="amp-name">{{ tag.label }}</span>
          <span class="amp-cat">{{ catLabel(tag.category) }}</span>
          <span class="amp-rule" :style="{ color: ruleColor, borderColor: ruleColor }" data-testid="archive-pop-head-step">{{ headStepText }}</span>
          <span class="amp-score" :style="{ color: ruleColor }">{{ scoreText }}</span>
          <span v-if="tag.direction_mismatch" class="amp-flag is-error">方向校验未通过 ⊘</span>
          <button type="button" class="amp-close" data-testid="archive-pop-close" @click="emit('close')">关闭 Esc</button>
          <p class="amp-metric" data-testid="archive-pop-metric">{{ metricText }}</p>
        </header>

        <div class="amp-body">
          <p v-if="loading" class="amp-note" data-testid="archive-pop-loading">正在加载匹配证据…</p>
          <p v-if="missingParts.length" class="amp-missing" data-testid="archive-pop-missing">
            未取到：{{ missingParts.join('、') }}（如实标注，不伪造）。
          </p>

          <!-- 结论（一句话因果，规则名与得分同源；无数值如实写需实时拉取） -->
          <p v-if="!loading && verdictLine" class="amp-verdict-line" data-testid="archive-pop-verdict">
            {{ verdictLine }}
          </p>

          <!-- 判定过程（命中即停）：走过的步逐条给差距，命中步之后的合并一排 -->
          <section v-if="!loading && verdictSteps.length" class="amp-sec" data-testid="archive-pop-verdict-steps">
            <span class="amp-k">判定过程（命中即停）</span>
            <div
              v-for="step in walkedSteps"
              :key="step.id"
              class="amp-flow-step"
              :class="`is-${step.state}`"
              :data-testid="`archive-pop-step-${step.id}`"
            >
              <div class="amp-flow-head">
                <span class="amp-flow-mark">{{ step.state === 'matched' ? '✓' : step.state === 'failed' ? '✗' : '·' }}</span>
                <span class="amp-flow-no" :style="{ color: step.state === 'matched' ? ruleColor : undefined }">{{ step.no }}</span>
                <span class="amp-flow-name">{{ step.name }}</span>
                <span v-if="step.state === 'matched'" class="amp-flow-state is-hit">命中</span>
                <span v-else-if="step.state === 'failed'" class="amp-flow-state is-miss">未通过</span>
                <span v-else class="amp-flow-state is-ref">参照</span>
              </div>
              <p class="amp-flow-q">{{ step.desc }}</p>
              <p v-if="step.result" class="amp-flow-result" data-testid="archive-pop-step-result">{{ step.result }}</p>
              <!-- 命中步就地展开公式（2.4：默认收起，缺 match-detail 不渲染入口） -->
              <template v-if="step.state === 'matched' && detail">
                <button type="button" class="amp-calc-btn" data-testid="archive-pop-calc-toggle" @click="calcOpen = !calcOpen">
                  {{ calcOpen ? '收起计算明细' : '这一步怎么算的 ▸' }}
                </button>
                <div v-if="calcOpen" class="amp-calc" data-testid="archive-pop-calc">
                  <KaTeXRender v-if="primaryFormula(detail)" :latex="primaryFormula(detail)" display />
                  <KaTeXRender v-if="substitutionFormula(detail)" :latex="substitutionFormula(detail)" display />
                  <KaTeXRender v-if="tag.match_reason !== 'composite_hit'" :latex="rateFormula(detail)" display />
                </div>
              </template>
            </div>
            <p v-if="skippedSteps.length" class="amp-flow-skipped" data-testid="archive-pop-steps-skipped">
              {{ skippedSteps.map(step => step.no).join('') }} 未走到（{{ matchedStep?.no }}已命中即停）
            </p>
            <p v-if="liveCalcNote" class="amp-verdict-note" data-testid="archive-pop-live-note">{{ liveCalcNote }}</p>
            <p v-if="driftNote" class="amp-verdict-note is-warn" data-testid="archive-pop-drift-note">{{ driftNote }}</p>
          </section>

          <section v-if="compositeChains.length" class="amp-sec" data-testid="archive-pop-composite">
            <span class="amp-k">组合命中（挂齐组合标签全部组件 → 推导挂载）</span>
            <div v-for="chain in compositeChains" :key="chain" class="amp-chain">
              <Icon icon="mdi:lightning-bolt" width="12" />
              <span>{{ chain }}</span>
            </div>
          </section>

          <section class="amp-sec">
            <span class="amp-k" data-testid="archive-pop-aux-head">
              辅助标签 ×{{ auxRows.length }} · 版块收编 {{ auxSummary }}（相似 ≥ {{ simThresholdText }}，非完全相同）
            </span>
            <div
              v-for="row in auxRows"
              :key="row.id"
              class="amp-aux"
              :class="`is-${row.state}`"
              data-testid="archive-pop-aux-row"
            >
              <span class="amp-aux-mark">{{ row.state === 'hit' ? '✓' : row.state === 'miss' ? '✗' : '·' }}</span>
              <span class="amp-aux-label">{{ row.label }}</span>
              <span v-if="row.pairLabel" class="amp-aux-pair">
                {{ row.pairLabel }}<template v-if="row.similarity != null"> {{ formatScore(row.similarity) }}</template>
              </span>
            </div>
            <p v-if="!auxRows.length" class="amp-note">该标签无辅助标签。</p>
            <p v-if="!detail" class="amp-note">
              ✓ = 与版块辅助池直接重合；逐对相似判定需 getMatchDetail 实时计算，当前未取到逐对数据（相似度阈值 {{ simThresholdText }}）。
            </p>
          </section>

          <section class="amp-sec">
            <span class="amp-k">{{ catLabel(tag.category) }}描述</span>
            <p class="amp-desc" data-testid="archive-pop-desc">
              {{ tag.description || '（该标签暂无描述——真实数据来自 topic_tags.description，后端未返回）' }}
            </p>
          </section>

          <section v-if="isEventTagCard" class="amp-sec" data-testid="archive-pop-related">
            <button
              type="button"
              class="amp-related-toggle"
              data-testid="archive-pop-related-toggle"
              :aria-expanded="relatedOpen"
              @click="toggleRelated"
            >
              相关事件 {{ relatedOpen ? '▾' : '▸' }}
            </button>
            <p class="amp-note amp-related-hint">按名称、描述与辅助标签的语义相似度，找同版块值得一起看的事件；只是相关推荐，不合并事件。</p>

            <template v-if="relatedOpen">
              <p v-if="relatedLoading" class="amp-note" data-testid="archive-pop-related-loading">正在查询相关事件…</p>
              <template v-else-if="relatedError">
                <p class="amp-missing" data-testid="archive-pop-related-error">{{ relatedError }}</p>
                <button type="button" class="amp-related-retry" data-testid="archive-pop-related-retry" @click="loadRelated">重试</button>
              </template>
              <template v-else-if="relatedData">
                <p class="amp-related-meta" data-testid="archive-pop-related-meta">{{ relatedCoverageText }}</p>
                <p v-if="relatedData.status === 'not_ready'" class="amp-missing" data-testid="archive-pop-related-not-ready">
                  相关事件索引尚未就绪（{{ relatedData.reason === 'source_not_indexed' ? '本条事件还没有向量' : '当前模型空间没有可用索引' }}），刷新索引后可再查。
                </p>
                <p v-else-if="relatedData.status === 'partial'" class="amp-note" data-testid="archive-pop-related-partial">
                  部分事件尚未索引（已就绪 {{ relatedData.indexed_count }}/{{ relatedData.candidate_count }}），下面是当前能算出的结果。
                </p>
                <p v-if="relatedData.status === 'ready' && relatedItems.length === 0" class="amp-note" data-testid="archive-pop-related-empty">
                  当前阈值与近 30 天范围内暂无相关事件。
                </p>
                <div
                  v-for="item in relatedItems"
                  :key="item.tag_id"
                  class="amp-related-item"
                  data-testid="archive-pop-related-item"
                >
                  <button type="button" class="amp-related-name" :title="item.label" @click="toggleRelatedDescription(item.tag_id)">
                    <span class="amp-related-label">{{ item.label }}</span>
                    <span class="amp-related-sim" data-testid="archive-pop-related-sim">{{ formatScore(item.similarity) }}</span>
                  </button>
                  <p v-if="expandedRelatedIds.has(item.tag_id)" class="amp-related-desc" data-testid="archive-pop-related-desc">
                    {{ item.description || '（该事件暂无描述）' }}
                  </p>
                  <div class="amp-related-actions">
                    <button
                      type="button"
                      class="amp-related-source"
                      data-testid="archive-pop-related-source"
                      :disabled="!item.source_article"
                      @click="openRelatedSource(item.source_article?.id)"
                    >
                      {{ item.source_article ? '查看来源' : '暂无可读来源' }}
                    </button>
                    <span v-if="item.source_article" class="amp-related-source-title" :title="item.source_article.title">{{ item.source_article.title }}</span>
                  </div>
                </div>
              </template>
            </template>
          </section>

          <section class="amp-sec">
            <span class="amp-k">本文标签</span>
            <div v-if="articleTagViews.length" class="amp-tags" data-testid="archive-pop-article-tags">
              <span v-for="item in articleTagViews" :key="item.key" class="amp-mini-tag">
                {{ item.label }}<i>{{ item.category }}</i>
              </span>
            </div>
            <p v-else class="amp-note">本文未打其他标签。</p>
          </section>

          <details v-if="flowConfig" class="amp-config" data-testid="archive-pop-config">
            <summary>当前匹配参数</summary>
            <dl>
              <div v-for="row in configRows" :key="row.label"><dt>{{ row.label }}</dt><dd>{{ row.value }}</dd></div>
            </dl>
          </details>
        </div>
      </template>

      <!-- 入板过程模式 -->
      <template v-else>
        <header class="amp-head" data-testid="archive-pop-head">
          <span class="amp-name">本文入板过程</span>
          <span v-if="weakArticle" class="amp-flag" data-testid="archive-pop-weak-flag">仅弱证据</span>
          <button type="button" class="amp-close" data-testid="archive-pop-close" @click="emit('close')">关闭 Esc</button>
        </header>

        <div class="amp-body">
          <p v-if="loading" class="amp-note" data-testid="archive-pop-loading">正在加载匹配证据…</p>
          <p v-if="missingParts.length" class="amp-missing" data-testid="archive-pop-missing">
            未取到：{{ missingParts.join('、') }}（如实标注，不伪造）。
          </p>

          <section class="amp-sec" data-testid="archive-pop-stages">
            <div v-for="stage in stages" :key="stage.reason" class="amp-stage" :data-testid="`archive-pop-stage-${stage.reason}`">
              <div class="amp-stage-head">
                <span class="amp-stage-no" :style="{ color: matchReasonColor(stage.reason) }">{{ ruleOf(stage.reason)?.no ?? '·' }}</span>
                <span class="amp-stage-name">{{ ruleOf(stage.reason)?.short ?? stage.reason }}</span>
                <span class="amp-stage-cnt">{{ stage.items.length || '' }}</span>
              </div>
              <div
                v-for="item in stage.items"
                :key="item.id"
                class="amp-stage-item"
                data-testid="archive-pop-stage-item"
              >
                <span class="amp-stage-label">{{ item.label }}</span>
                <span v-if="item.id === bestTagId" class="amp-win" :class="{ 'is-weak': weakArticle }">{{ bestTagBadge }}</span>
                <span class="amp-stage-score" :style="{ color: matchReasonColor(stage.reason) }">{{ formatScore(item.score) }}</span>
              </div>
              <p v-if="stage.items.length === 0 && stage.skipped" class="amp-stage-empty is-skipped" data-testid="archive-pop-stage-skipped">
                未走到（第 {{ bestStepNo }} 步命中即停）
              </p>
              <p v-else-if="stage.items.length === 0" class="amp-stage-why" data-testid="archive-pop-stage-why">
                ✗ {{ stage.why }}
              </p>
            </div>
          </section>

          <section class="amp-sec">
            <span class="amp-k">本文标签</span>
            <div v-if="articleTagViews.length" class="amp-tags" data-testid="archive-pop-article-tags">
              <span
                v-for="item in articleTagViews"
                :key="item.key"
                class="amp-mini-tag"
                :class="{ 'is-matched': item.matched }"
              >{{ item.label }}</span>
            </div>
            <p v-else class="amp-note">本文未打标签。——无命中标签的文章不会进入日报</p>
          </section>
        </div>
      </template>
    </aside>
  </Teleport>
</template>

<style scoped src="./archiveMatchPopover.css" />
