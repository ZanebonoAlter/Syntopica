<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import KaTeXRender from '~/components/KaTeXRender.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardArticleTag, MatchDetailPair, MatchDetailResponse } from '~/api/semanticBoards'

const props = defineProps<{
  boardId: number
  tag: BoardArticleTag | null
}>()

const emit = defineEmits<{ close: [] }>()

const sbApi = useSemanticBoardsApi()
const detail = ref<MatchDetailResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
let requestSeq = 0

const selectedTagLabel = computed(() => detail.value?.topic_tag_label || props.tag?.label || '匹配详情')
const sortedPairs = computed(() => [...(detail.value?.pairs || [])].sort((a, b) => b.similarity - a.similarity))

watch([() => props.boardId, () => props.tag?.id], async ([boardId, tagId]) => {
  const seq = ++requestSeq
  if (!tagId) {
    detail.value = null
    error.value = null
    loading.value = false
    return
  }

  loading.value = true
  error.value = null
  detail.value = null
  try {
    const res = await sbApi.getMatchDetail(boardId, tagId)
    if (seq !== requestSeq) return
    if (res.success && res.data) {
      detail.value = res.data
    } else {
      error.value = res.error || '加载匹配详情失败'
    }
  } catch {
    if (seq === requestSeq) {
      error.value = '加载匹配详情失败'
    }
  } finally {
    if (seq === requestSeq) {
      loading.value = false
    }
  }
}, { immediate: true })

function formatScore(value: number | undefined, digits = 2): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(digits) : '0.00'
}

function reasonLabel(reason: string): string {
  const labels: Record<string, string> = {
    direct_hit: '直接命中',
    hit_rate: '命中率',
    max_sim: '最高相似度',
    weighted: '综合加权',
  }
  return labels[reason] || reason
}

function reasonColor(reason: string): string {
  const colors: Record<string, string> = {
    direct_hit: '#22c55e',
    hit_rate: '#3b82f6',
    max_sim: '#f59e0b',
    weighted: '#94a3b8',
  }
  return colors[reason] || '#94a3b8'
}

function primaryFormula(item: MatchDetailResponse): string {
  if (item.match_reason === 'hit_rate') return '\\text{score}=\\alpha\\cdot S_{\\max}+(1-\\alpha)\\cdot R'
  if (item.match_reason === 'max_sim') return '\\text{score}=S_{\\max}'
  if (item.match_reason === 'weighted') return '\\text{score}=w_{\\text{sim}}\\cdot S_{\\max}+w_{\\text{density}}\\cdot R'
  return ''
}

function substitutionFormula(item: MatchDetailResponse): string {
  const score = formatScore(item.score, 3)
  const maxSim = formatScore(item.max_similarity, 3)
  const rate = formatScore(item.hit_rate, 3)
  if (item.match_reason === 'hit_rate') {
    const alpha = formatScore(item.config.hit_rate_sim_blend, 2)
    return `\\text{score}=${alpha}\\times ${maxSim}+${formatScore(1 - item.config.hit_rate_sim_blend, 2)}\\times ${rate}=${score}`
  }
  if (item.match_reason === 'max_sim') return `S_{\\max}=${maxSim}\\ge ${formatScore(item.config.direct_max_sim, 2)},\\ hits\\ge ${Math.min(item.config.direct_max_sim_min_hits, item.tag_auxiliary_count)},\\ R\\ge ${formatScore(item.config.direct_max_sim_min_hit_rate, 2)}`
  if (item.match_reason === 'weighted') return `\\text{score}=${formatScore(item.config.weight_sim, 2)}\\times ${maxSim}+${formatScore(item.config.weight_density, 2)}\\times ${rate}=${score}`
  return ''
}

function rateFormula(item: MatchDetailResponse): string {
  return `R=\\frac{\\text{hits}}{\\max(N,s)}=\\frac{${item.hits}}{\\max(${item.tag_auxiliary_count},${item.config.min_effective_sample})}=${formatScore(item.hit_rate, 3)}`
}

function pairKey(pair: MatchDetailPair): string {
  return `${pair.tag_auxiliary_id}-${pair.board_auxiliary_id}`
}

function directionCheckText(item: MatchDetailResponse): string {
  const dirSim = item.direction_sim
  const threshold = item.config.direction_sim_threshold
  if (dirSim == null) return ' 方向校验跳过（无数据）'
  if (dirSim >= threshold) return ` 方向校验✓${formatScore(dirSim)}≥${formatScore(threshold)}`
  return ` ⚠方向不符 ${formatScore(dirSim)}<${formatScore(threshold)}`
}

interface FlowStep {
  id: string
  title: string
  desc: string
  result: string
  state: 'matched' | 'failed' | 'compute'
}

const overlapCount = computed(() => detail.value?.direct_hit_auxiliaries?.length ?? 0)

const flowSteps = computed<FlowStep[]>(() => {
  if (!detail.value) return []
  const d = detail.value
  const c = d.config
  const reason = d.match_reason
  const overlap = overlapCount.value
  const N = d.tag_auxiliary_count
  const steps: FlowStep[] = []

  // ① Direct Hit
  if (reason === 'direct_hit') {
    steps.push({
      id: 'direct_hit', title: '① 精确匹配',
      desc: '事件和板块有多少个辅助标签完全相同？',
      result: `交集 ${overlap} 个 ≥ ${c.direct_hit_min_overlap} → 直接命中！`,
      state: 'matched',
    })
    return steps
  }
  steps.push({
    id: 'direct_hit', title: '① 精确匹配',
    desc: '事件和板块有多少个辅助标签完全相同？',
    result: `交集 ${overlap} 个 < ${c.direct_hit_min_overlap}，不够 → 继续`,
    state: 'failed',
  })

  // ② Similarity
  steps.push({
    id: 'similarity', title: '② 算余弦相似度',
    desc: `把事件的 ${N} 个标签和板块标签两两比较「有多像」，相似度 ≥ ${formatScore(c.sim_threshold)} 算命中`,
    result: `命中 ${d.hits}/${N}，R=${formatScore(d.hit_rate)}，Smax=${formatScore(d.max_similarity)}`,
    state: 'compute',
  })

  // ③ Hit Rate
  if (reason === 'hit_rate') {
    steps.push({
      id: 'hit_rate', title: '③ 命中率规则',
      desc: `「几分之几」的标签像板块的？超过 ${(c.direct_hit_rate * 100).toFixed(0)}% 就挂载`,
      result: `R=${formatScore(d.hit_rate)} > ${formatScore(c.direct_hit_rate)} → 满足！`,
      state: 'matched',
    })
    return steps
  }
  steps.push({
    id: 'hit_rate', title: '③ 命中率规则',
    desc: `「几分之几」的标签像板块的？超过 ${(c.direct_hit_rate * 100).toFixed(0)}% 就挂载`,
    result: `R=${formatScore(d.hit_rate)} ≤ ${formatScore(c.direct_hit_rate)} → 不够`,
    state: 'failed',
  })

  // ④ Max Sim
  const minHits = Math.min(c.direct_max_sim_min_hits, N)
  const simOk = d.max_similarity >= c.direct_max_sim
  const hitsOk = d.hits >= minHits
  const rateOk = d.hit_rate >= c.direct_max_sim_min_hit_rate
  if (reason === 'max_sim') {
    steps.push({
      id: 'max_sim', title: '④ 最高相似度规则',
      desc: '最像的那一对有多像？需同时满足三个条件',
      result: `✓Smax=${formatScore(d.max_similarity)} ✓${d.hits}≥${minHits}命中 ✓R=${formatScore(d.hit_rate)} → 满足！${d.downgraded ? ` ⚠降级匹配（原阈值${c.direct_max_sim_min_hits}，因仅有${d.tag_auxiliary_count}个辅助标签降为${d.effective_min_hits}）` : ''}${directionCheckText(d)}`,
      state: 'matched',
    })
    return steps
  }
  steps.push({
    id: 'max_sim', title: '④ 最高相似度规则',
    desc: '最像的那一对有多像？需同时满足三个条件',
    result: `${simOk ? '✓' : '✗'}Smax${formatScore(d.max_similarity)} ${hitsOk ? '✓' : '✗'}${d.hits}/${minHits}命中 ${rateOk ? '✓' : '✗'}R=${formatScore(d.hit_rate)} → 继续`,
    state: 'failed',
  })

  // ⑤ Weighted
  const w = c.weight_sim * d.max_similarity + c.weight_density * d.hit_rate
  steps.push({
    id: 'weighted', title: '⑤ 综合加权',
    desc: '整体看像不像？把相似度和命中率综合打分',
    result: reason === 'weighted'
      ? `${formatScore(c.weight_sim)}×${formatScore(d.max_similarity)}+${formatScore(c.weight_density)}×${formatScore(d.hit_rate)}=${formatScore(w)} ≥ ${formatScore(c.weighted_threshold)} → 满足！`
      : `${formatScore(w)} < ${formatScore(c.weighted_threshold)} → 不匹配`,
    state: reason === 'weighted' ? 'matched' : 'failed',
  })

  return steps
})
</script>

<template>
  <aside class="match-detail-panel">
    <header class="match-detail-header">
      <div>
        <p class="match-detail-eyebrow">匹配详情</p>
        <h3>{{ selectedTagLabel }}</h3>
      </div>
      <button type="button" class="match-detail-close" aria-label="关闭匹配详情" @click="emit('close')">
        <Icon icon="mdi:close" width="16" />
      </button>
    </header>

    <div v-if="loading" class="match-detail-loading">
      <div v-for="i in 4" :key="i" class="match-detail-skeleton" />
    </div>

    <div v-else-if="error" class="match-detail-error">
      <Icon icon="mdi:alert-circle-outline" width="18" />
      <span>{{ error }}</span>
    </div>

    <div v-else-if="detail" class="match-detail-body">
      <section class="match-detail-summary">
        <span class="match-detail-badge" :style="{ color: reasonColor(detail.match_reason), borderColor: reasonColor(detail.match_reason) }">
          {{ reasonLabel(detail.match_reason) }} {{ formatScore(detail.score) }}
        </span>
        <div class="match-detail-metrics">
          <span>命中 {{ detail.hits }}/{{ detail.tag_auxiliary_count }}</span>
          <span>R {{ formatScore(detail.hit_rate) }}</span>
          <span>Smax {{ formatScore(detail.max_similarity) }}</span>
        </div>
      </section>

      <section v-if="detail.direct_hit_auxiliaries.length" class="match-detail-section">
        <h4>直接命中辅助标签</h4>
        <div class="direct-hit-list">
          <div v-for="hit in detail.direct_hit_auxiliaries" :key="`${hit.tag_auxiliary_id}-${hit.board_auxiliary_id}`" class="direct-hit-item">
            <Icon icon="mdi:check-circle" width="14" />
            <span>{{ hit.tag_label }}</span>
            <span class="direct-hit-arrow">→</span>
            <span>{{ hit.board_label }}</span>
          </div>
        </div>
      </section>

      <section v-if="!detail.direct_hit_auxiliaries.length" class="match-detail-section">
        <h4>得分公式</h4>
        <div class="formula-card">
          <KaTeXRender v-if="primaryFormula(detail)" :latex="primaryFormula(detail)" display />
          <KaTeXRender v-if="substitutionFormula(detail)" :latex="substitutionFormula(detail)" display />
          <KaTeXRender :latex="rateFormula(detail)" display />
        </div>
      </section>

      <section class="match-detail-section">
        <h4>逐对匹配</h4>
        <div v-if="sortedPairs.length" class="pair-list">
          <div v-for="pair in sortedPairs" :key="pairKey(pair)" class="pair-row" :class="{ 'pair-row--hit': pair.is_hit }">
            <Icon :icon="pair.is_hit ? 'mdi:check-circle' : 'mdi:circle-outline'" width="14" />
            <div class="pair-labels">
              <span>{{ pair.tag_auxiliary_label }}</span>
              <small>→ {{ pair.board_auxiliary_label }}</small>
            </div>
            <strong>{{ formatScore(pair.similarity) }}</strong>
          </div>
        </div>
        <p v-else class="match-detail-empty">无逐对相似度明细</p>
      </section>

      <section v-if="flowSteps.length" class="match-detail-section">
        <h4>匹配流程</h4>
        <p class="flow-hint">系统按顺序检查，首个满足的规则决定匹配结果</p>
        <div class="flow-steps">
          <template v-for="(step, idx) in flowSteps" :key="step.id">
            <div v-if="idx > 0" class="flow-connector"><div class="flow-connector-line" /></div>
            <div class="flow-node" :class="[`flow-node--${step.state}`]">
              <div class="flow-node-head">
                <strong>{{ step.title }}</strong>
                <span v-if="step.state === 'matched'" class="flow-badge">命中</span>
              </div>
              <div class="flow-node-desc">{{ step.desc }}</div>
              <div class="flow-node-result" :class="[`flow-node-result--${step.state}`]">
                <Icon :icon="step.state === 'matched' ? 'mdi:check-circle' : step.state === 'compute' ? 'mdi:calculator' : 'mdi:arrow-down-circle-outline'" width="13" />
                <span>{{ step.result }}</span>
              </div>
            </div>
          </template>
        </div>
      </section>

      <details class="match-detail-config">
        <summary>当前匹配参数</summary>
        <dl>
          <div><dt>sim_threshold（相似度命中线）</dt><dd>{{ formatScore(detail.config.sim_threshold) }}</dd></div>
          <div><dt>direct_hit_min_overlap（精确匹配最少交集）</dt><dd>{{ detail.config.direct_hit_min_overlap }}</dd></div>
          <div><dt>direct_hit_rate（命中率挂载线）</dt><dd>{{ formatScore(detail.config.direct_hit_rate) }}</dd></div>
          <div><dt>hit_rate_sim_blend（命中率混相似度权重）</dt><dd>{{ formatScore(detail.config.hit_rate_sim_blend) }}</dd></div>
          <div><dt>min_effective_sample（最小样本量）</dt><dd>{{ detail.config.min_effective_sample }}</dd></div>
          <div><dt>direct_max_sim（最高相似度挂载线）</dt><dd>{{ formatScore(detail.config.direct_max_sim) }}</dd></div>
          <div><dt>direct_max_sim_min_hits（最高相似度最少命中）</dt><dd>{{ detail.config.direct_max_sim_min_hits }}</dd></div>
          <div><dt>direct_max_sim_min_hit_rate（最高相似度最低命中率）</dt><dd>{{ formatScore(detail.config.direct_max_sim_min_hit_rate) }}</dd></div>
          <div><dt>direction_sim_threshold（方向校验阈值）</dt><dd>{{ formatScore(detail.config.direction_sim_threshold) }}</dd></div>
          <div><dt>weight_sim（加权相似度比重）</dt><dd>{{ formatScore(detail.config.weight_sim) }}</dd></div>
          <div><dt>weight_density（加权命中率比重）</dt><dd>{{ formatScore(detail.config.weight_density) }}</dd></div>
          <div><dt>weighted_threshold（加权挂载线）</dt><dd>{{ formatScore(detail.config.weighted_threshold) }}</dd></div>
        </dl>
      </details>
    </div>
  </aside>
</template>

<style scoped>
.match-detail-panel {
  height: 100%;
  min-height: 420px;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 18px;
  background: rgba(12, 18, 27, 0.86);
  color: rgba(255, 255, 255, 0.76);
  overflow: hidden;
  backdrop-filter: blur(18px);
}

.match-detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.match-detail-eyebrow {
  margin: 0 0 0.2rem;
  font-size: 0.65rem;
  color: rgba(240, 138, 75, 0.75);
}

.match-detail-header h3 {
  margin: 0;
  font-family: serif;
  font-size: 1rem;
  color: rgba(255, 255, 255, 0.88);
}

.match-detail-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.45);
  cursor: pointer;
}

.match-detail-body,
.match-detail-loading,
.match-detail-error {
  padding: 1rem;
}

.match-detail-loading {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.match-detail-skeleton {
  height: 42px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.05);
  animation: pulse 1.4s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.45; }
  50% { opacity: 0.9; }
}

.match-detail-error {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  color: rgba(255, 170, 140, 0.86);
  font-size: 0.78rem;
}

.match-detail-summary {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.match-detail-badge {
  align-self: flex-start;
  padding: 0.2rem 0.55rem;
  border: 1px solid;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  font-size: 0.72rem;
  font-weight: 600;
}

.match-detail-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.42);
}

.match-detail-section {
  margin-top: 1rem;
}

.match-detail-section h4 {
  margin: 0 0 0.55rem;
  font-size: 0.72rem;
  color: rgba(255, 220, 200, 0.78);
}

.formula-card {
  padding: 0.65rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.035);
  font-size: 0.78rem;
}

.direct-hit-list,
.pair-list {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.direct-hit-item,
.pair-row {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.45rem 0.5rem;
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.035);
  font-size: 0.72rem;
}

.direct-hit-item,
.pair-row--hit {
  color: rgba(165, 243, 190, 0.9);
}

.direct-hit-arrow {
  color: rgba(255, 255, 255, 0.28);
}

.pair-labels {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 0.1rem;
}

.pair-labels span,
.pair-labels small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pair-labels small {
  color: rgba(255, 255, 255, 0.38);
}

.pair-row strong {
  color: rgba(255, 255, 255, 0.75);
  font-size: 0.72rem;
}

.match-detail-empty {
  margin: 0;
  color: rgba(255, 255, 255, 0.35);
  font-size: 0.72rem;
}

.match-detail-config {
  margin-top: 1rem;
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.45);
}

.match-detail-config summary {
  cursor: pointer;
}

.match-detail-config dl {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin: 0.6rem 0 0;
}

.match-detail-config div {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
}

.match-detail-config dt,
.match-detail-config dd {
  margin: 0;
}

/* ---- Flow ---- */
.flow-hint {
  margin: 0 0 0.6rem;
  font-size: 0.63rem;
  color: rgba(255, 255, 255, 0.32);
}

.flow-steps {
  display: flex;
  flex-direction: column;
}

.flow-connector {
  display: flex;
  justify-content: center;
  height: 14px;
}

.flow-connector-line {
  width: 1px;
  height: 100%;
  background: rgba(255, 255, 255, 0.1);
}

.flow-node {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

.flow-node--matched {
  border-color: rgba(34, 197, 94, 0.28);
  background: rgba(34, 197, 94, 0.055);
}

.flow-node--compute {
  border-color: rgba(59, 130, 246, 0.18);
  background: rgba(59, 130, 246, 0.035);
}

.flow-node-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.2rem;
}

.flow-node-head strong {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.8);
}

.flow-badge {
  padding: 0.08rem 0.32rem;
  border-radius: 4px;
  background: rgba(34, 197, 94, 0.15);
  color: rgba(74, 222, 128, 0.92);
  font-size: 0.58rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.flow-node-desc {
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.63rem;
  line-height: 1.45;
  margin: 0 0 0.25rem;
}

.flow-node-result {
  display: flex;
  align-items: flex-start;
  gap: 0.28rem;
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.52);
  line-height: 1.4;
}

.flow-node-result--matched {
  color: rgba(74, 222, 128, 0.88);
}

.flow-node-result--compute {
  color: rgba(96, 165, 250, 0.78);
}

.flow-node-result--failed {
  color: rgba(255, 255, 255, 0.45);
}
</style>
