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
import type { AuxiliaryLabelItem, BoardArticleTag, MatchDetailResponse } from '~/api/semanticBoards'
import { catLabel, matchReasonColor, stepNoOf } from '~/utils/matchQuality'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import { Z_INDEX, nestedOverlayZ } from '~/components/ui/layout-contract'
import { ruleMeta, isWeakArticle, type ArchiveArticle } from '../../composables/useBoardArchive'
import {
  buildTagFlowSteps,
  compositeChainLines,
  flowConfigFromApi,
  flowConfigFromDetail,
  formatScore,
  LIVE_CALC_NOTE,
  primaryFormula,
  rateFormula,
  scoreDriftNote,
  substitutionFormula,
  whyMissText,
  type MatchFlowConfig,
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

const emit = defineEmits<{ close: [] }>()

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

/** 代入式/判定行的数值口径标注（实时重算 vs 冻结 score）。 */
const liveCalcNote = computed(() => (detail.value ? LIVE_CALC_NOTE : ''))
const driftNote = computed(() => scoreDriftNote(detail.value))

const ruleLabel = computed(() => (props.tag ? ruleMeta(props.tag.match_reason).label : ''))
const ruleColor = computed(() => (props.tag ? matchReasonColor(props.tag.match_reason, props.tag.downgraded) : 'var(--color-text-muted)'))
const scoreText = computed(() => (props.tag ? formatScore(props.tag.score) : '--'))

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

/** 私有判定行：第 N 步即命中（命中数值可得时给真实数值，否则如实标需实时拉取）。 */
const verdictStep = computed(() => (props.tag ? stepNoOf(props.tag.match_reason) : '·'))
const verdictDetail = computed(() => {
  const tag = props.tag
  if (!tag || !detail.value || !flowConfig.value) return '该步命中数值需实时拉取'
  const matched = buildTagFlowSteps(tag.match_reason, flowConfig.value, detail.value, auxLabels.value.length)
    .find(step => step.state === 'matched')
  return matched ? matched.result : '该步命中数值需实时拉取'
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
const bestStepNo = computed(() => (strongestTag.value ? stepNoOf(strongestTag.value.match_reason) : '·'))

/** 通用流程说明（入板模式末尾指向它）。 */
/** 通用流程说明（入板模式纯数据，不重复流程叙述——通用流程在工具行「匹配流程」）。 */
const simThresholdText = computed(() => (flowConfig.value ? formatScore(flowConfig.value.simThreshold) : '—'))

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
  if (event.key === 'Escape') emit('close')
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
          <span class="amp-rule" :style="{ color: ruleColor, borderColor: ruleColor }">{{ ruleLabel }}</span>
          <span class="amp-score" :style="{ color: ruleColor }">{{ scoreText }}</span>
          <span v-if="tag.downgraded" class="amp-flag">已降级 ↓</span>
          <span v-if="tag.direction_mismatch" class="amp-flag is-error">方向校验未通过 ⊘</span>
          <button type="button" class="amp-close" data-testid="archive-pop-close" @click="emit('close')">关闭 Esc</button>
        </header>

        <div class="amp-body">
          <p v-if="loading" class="amp-note" data-testid="archive-pop-loading">正在加载匹配证据…</p>
          <p v-if="missingParts.length" class="amp-missing" data-testid="archive-pop-missing">
            未取到：{{ missingParts.join('、') }}（如实标注，不伪造）。
          </p>

          <section class="amp-sec">
            <span class="amp-k">{{ catLabel(tag.category) }}描述</span>
            <p class="amp-desc" data-testid="archive-pop-desc">
              {{ tag.description || '（该标签暂无描述——真实数据来自 topic_tags.description，后端未返回）' }}
            </p>
          </section>

          <section v-if="compositeChains.length" class="amp-sec" data-testid="archive-pop-composite">
            <span class="amp-k">组合命中（挂齐组合标签全部组件 → 推导挂载）</span>
            <div v-for="chain in compositeChains" :key="chain" class="amp-chain">
              <Icon icon="mdi:lightning-bolt" width="12" />
              <span>{{ chain }}</span>
            </div>
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

          <section class="amp-sec">
            <span class="amp-k" data-testid="archive-pop-aux-head">
              辅助标签 ×{{ auxRows.length }} · 版块收编 {{ auxSummary }}
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
            <p v-if="!detail" class="amp-note">
              ✓ = 与版块辅助池直接重合；逐对相似判定需 getMatchDetail 实时计算，当前未取到逐对数据（相似度阈值 {{ simThresholdText }}）。
            </p>
          </section>

          <p class="amp-verdict" data-testid="archive-pop-verdict">
            判定：按 ⓪→①→②→③→④→⑤ 顺序判定，本事件
            <b :style="{ color: ruleColor }">第 {{ verdictStep }} 步即命中 · {{ ruleLabel }} {{ scoreText }}</b>
            （{{ verdictDetail }}）完整流程与阈值见工具行「匹配流程」。
            <span v-if="liveCalcNote" class="amp-verdict-note" data-testid="archive-pop-live-note">{{ liveCalcNote }}</span>
            <span v-if="driftNote" class="amp-verdict-note is-warn" data-testid="archive-pop-drift-note">{{ driftNote }}</span>
          </p>

          <button
            type="button"
            class="amp-calc-btn"
            data-testid="archive-pop-calc-toggle"
            @click="calcOpen = !calcOpen"
          >
            {{ calcOpen ? '收起规则代入式' : '规则代入式（各步参数代入）' }}
          </button>
          <div v-if="calcOpen" class="amp-calc" data-testid="archive-pop-calc">
            <template v-if="detail">
              <KaTeXRender v-if="primaryFormula(detail)" :latex="primaryFormula(detail)" display />
              <KaTeXRender v-if="substitutionFormula(detail)" :latex="substitutionFormula(detail)" display />
              <KaTeXRender :latex="rateFormula(detail)" display />
              <p v-if="liveCalcNote" class="amp-note" data-testid="archive-pop-calc-note">{{ liveCalcNote }}</p>
              <p v-if="driftNote" class="amp-note is-warn" data-testid="archive-pop-calc-drift">{{ driftNote }}</p>
            </template>
            <p v-else class="amp-note">
              该标签的历史计算明细不可得（未取到 match-detail），不拿当前规则伪造结果。
            </p>
          </div>
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
                <span class="amp-stage-no" :style="{ color: matchReasonColor(stage.reason) }">{{ stepNoOf(stage.reason) }}</span>
                <span class="amp-stage-name">{{ ruleMeta(stage.reason).label }}</span>
                <span class="amp-stage-cnt">{{ stage.items.length || '' }}</span>
              </div>
              <div
                v-for="item in stage.items"
                :key="item.id"
                class="amp-stage-item"
                data-testid="archive-pop-stage-item"
              >
                <span class="amp-stage-label">{{ item.label }}</span>
                <span v-if="item.downgraded" class="amp-flag">↓</span>
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
