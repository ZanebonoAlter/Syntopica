<script setup lang="ts">
/**
 * BoardHealthPanel — /tags 板块页「板块内容」tab 治理主面板
 * （add-board-health-checkup ui-design 形态 A：吸收构成管理，BoardCompositionPanel 退役）。
 *
 * 职能：体检评价（评分徽章/评语/指标 chips/质量表/趋势/未收编候选）+ 构成管理
 * （picker 添加双模式/行内移除/组合创建与挂载移除）。体检数据无快照或失败时构成
 * 管理照常可用（面板不依赖体检数据）。
 *
 * 硬约束：颜色仅 Layer 2 语义 token（评分色档从 success/warning/error 语义档派生），
 * 统一组件 AppButton/AppSectionHeader；长列表折叠（质量表/组合 top 10、候选 top 5）。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import {
  useSemanticBoardsApi,
  type AuxiliaryLabelItem,
  type BoardCompositeMount,
  type BoardHealthCandidate,
  type BoardHealthLabelMetric,
  type BoardHealthSnapshotDTO,
  type BoardHealthTrendPoint,
} from '~/api/semanticBoards'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import AuxiliaryLabelPicker from './AuxiliaryLabelPicker.vue'
import CompositeLabelEditDialog from './CompositeLabelEditDialog.vue'

const props = defineProps<{
  boardId: number
  labels: AuxiliaryLabelItem[]
  loading: boolean
  /** 本版块挂载的组合标签（add-composite-labels：版块维度的组合管理） */
  composites?: BoardCompositeMount[]
  boardLabel?: string
}>()

const emit = defineEmits<{
  remove: [auxiliaryLabelId: number]
  refresh: []
  /** 体检数据加载结果上报（TagsPage 顶部警告条消费；无快照/低分语义） */
  healthLoaded: [payload: { hasSnapshot: boolean; score: number | null }]
}>()

const { success: notifySuccess } = useNotify()
const api = useSemanticBoardsApi()

// ── 体检数据 ──────────────────────────────────────────────────────────────
const snapshot = ref<BoardHealthSnapshotDTO | null>(null)
const trend = ref<BoardHealthTrendPoint[]>([])
const healthLoading = ref(false)
const healthError = ref<string | null>(null)

let requestSeq = 0

async function loadHealth(): Promise<void> {
  const seq = ++requestSeq
  healthLoading.value = true
  healthError.value = null
  const res = await api.getBoardHealthCheckup(props.boardId)
  if (seq !== requestSeq) return
  if (res.success && res.data) {
    snapshot.value = res.data.latest
    trend.value = res.data.trend ?? []
    emit('healthLoaded', {
      hasSnapshot: res.data.latest !== null,
      score: res.data.latest?.llm_score ?? null,
    })
  } else {
    healthError.value = res.error || '体检数据加载失败'
    snapshot.value = null
    trend.value = []
    emit('healthLoaded', { hasSnapshot: false, score: null })
  }
  healthLoading.value = false
}

watch(() => props.boardId, () => { void loadHealth() })
onMounted(() => { void loadHealth() })

const ALERT_THRESHOLD = 60

function scoreTier(score: number): 'good' | 'mid' | 'low' {
  if (score >= 70) return 'good'
  if (score >= ALERT_THRESHOLD) return 'mid'
  return 'low'
}

const scoreTierClass = computed(() =>
  snapshot.value?.llm_score != null ? `bhp-badge--${scoreTier(snapshot.value.llm_score)}` : '',
)
const isLowScore = computed(
  () => snapshot.value?.llm_score != null && snapshot.value.llm_score < ALERT_THRESHOLD,
)

const checkedAtText = computed(() => {
  const at = snapshot.value?.checked_at
  if (!at) return ''
  const d = new Date(at)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
})

const boardMetrics = computed(() => snapshot.value?.metrics.board)

/** 质量表行：构成标签（props.labels）联查体检归因（按 aux_id）。 */
interface QualityRow {
  id: number
  label: string
  metric: BoardHealthLabelMetric | null
}

const qualityRows = computed<QualityRow[]>(() => {
  const byAux = new Map<number, BoardHealthLabelMetric>()
  for (const m of snapshot.value?.metrics.labels ?? []) byAux.set(m.aux_id, m)
  const rows: QualityRow[] = props.labels.map((l) => ({
    id: l.id,
    label: l.label,
    metric: byAux.get(l.id) ?? null,
  }))
  // 样本数降序（无体检数据保持构成原序），稳定可复现。
  rows.sort((a, b) => (b.metric?.samples ?? 0) - (a.metric?.samples ?? 0))
  return rows
})

// ── 长列表折叠（质量表/组合 top 10、候选 top 5） ─────────────────────────
const FOLD_LABELS = 10
const FOLD_COMPOSITES = 10
const FOLD_CANDIDATES = 5

const labelsExpanded = ref(false)
const compositesExpanded = ref(false)
const candidatesExpanded = ref(false)

watch(() => props.boardId, () => {
  labelsExpanded.value = false
  compositesExpanded.value = false
  candidatesExpanded.value = false
})

const visibleQualityRows = computed(() =>
  labelsExpanded.value ? qualityRows.value : qualityRows.value.slice(0, FOLD_LABELS),
)
const visibleComposites = computed(() => {
  const list = props.composites ?? []
  return compositesExpanded.value ? list : list.slice(0, FOLD_COMPOSITES)
})
const visibleCandidates = computed(() => {
  const list = snapshot.value?.uncaptured_candidates ?? []
  return candidatesExpanded.value ? list : list.slice(0, FOLD_CANDIDATES)
})

// ── 趋势 sparkline（纯展示 SVG polyline，0-100） ─────────────────────────
const trendPolyline = computed(() => {
  const points = trend.value.filter((p) => p.llm_score != null) as Array<{ checked_at: string; llm_score: number }>
  if (points.length < 2) return ''
  const W = 220
  const H = 44
  const step = W / (points.length - 1)
  return points
    .map((p, i) => `${(i * step).toFixed(1)},${(H - (p.llm_score / 100) * H).toFixed(1)}`)
    .join(' ')
})
const trendSummary = computed(() => {
  const points = trend.value.filter((p) => p.llm_score != null)
  if (points.length === 0) return '暂无评分'
  const scores = points.map((p) => p.llm_score as number)
  const first = scores[0] ?? 0
  const last = scores[scores.length - 1] ?? 0
  const delta = Math.round(last - first)
  return `${points.length} 轮 · 首轮 ${Math.round(first)} → 最新 ${Math.round(last)}（${delta >= 0 ? '+' : ''}${delta}）`
})

// ── 构成管理：picker（双模式）+ 移除 + 组合 ──────────────────────────────
const showPicker = ref(false)
const pickerMode = ref<'edit' | 'create'>('edit')
const pendingIds = ref<number[]>([])
const adding = ref(false)
const notice = ref('')
const showCompositeDialog = ref(false)

function togglePicker(): void {
  showPicker.value = !showPicker.value
  if (!showPicker.value) {
    pendingIds.value = []
    pickerMode.value = 'edit'
  }
}

function handleRemove(id: number, label: string): void {
  if (!confirm(`从板块中移除辅助标签 "${label}"？\n注意：不会自动回填历史数据。`)) return
  emit('remove', id)
}

async function handleConfirmAdd(): Promise<void> {
  if (pendingIds.value.length === 0) {
    showPicker.value = false
    return
  }
  adding.value = true
  try {
    for (const id of pendingIds.value) {
      const res = await api.addComposition(props.boardId, id)
      if (!res.success) {
        console.error('Failed to add composition:', res.error)
      }
    }
    showPicker.value = false
    pendingIds.value = []
    notifySuccess('已添加构成标签')
    notice.value = '已添加构成标签。历史标签归属不会自动回填，可手动触发 board 回填。'
    emit('refresh')
  } finally {
    adding.value = false
  }
}

/** 移除组合挂载（复用 composition 删除接口，auxiliary_label_id 列复用）。 */
function handleRemoveCompositeMount(item: BoardCompositeMount): void {
  if (!confirm(`从板块中移除组合标签 "${item.label}"（${item.components.join(' × ')}）？\n注意：不会自动回填历史归属。`)) return
  api.removeFromComposition(props.boardId, item.id).then(() => {
    emit('refresh')
  }).catch(() => {
    notice.value = '移除组合挂载失败，请重试'
  })
}

// ── 收编快捷动作（手动挂载特权，不走 LLM 建议管道） ──────────────────────
const capturingId = ref<number | null>(null)

async function handleCapture(c: BoardHealthCandidate): Promise<void> {
  if (capturingId.value !== null) return
  capturingId.value = c.aux_id
  try {
    const res = await api.addComposition(props.boardId, c.aux_id)
    if (res.success) {
      notifySuccess(`已收编「${c.label}」，历史归属不自动回填`)
      // 本地剔除收编行 + 刷新构成（体检数据下轮刷新候选）
      if (snapshot.value) {
        snapshot.value = {
          ...snapshot.value,
          uncaptured_candidates: snapshot.value.uncaptured_candidates.filter((x) => x.aux_id !== c.aux_id),
        }
      }
      emit('refresh')
    } else {
      notice.value = `收编「${c.label}」失败：${res.error ?? '请重试'}`
    }
  } finally {
    capturingId.value = null
  }
}

function fmtPct(v: number | null | undefined): string {
  return v == null ? '—' : `${Math.round(v * 100)}%`
}
function fmtNum(v: number | null | undefined, digits = 2): string {
  return v == null ? '—' : v.toFixed(digits)
}
</script>

<template>
  <div class="bhp-panel" data-testid="board-health-panel">
    <div class="bhp-header">
      <Icon icon="mdi:heart-pulse" width="15" class="bhp-header-icon" />
      <span class="bhp-title">版块体检</span>
      <!-- 评分徽章：好/中/差三档语义色；无快照「等待首轮体检」占位 -->
      <span
        v-if="snapshot?.llm_score != null"
        class="bhp-badge"
        :class="scoreTierClass"
        data-testid="board-health-score"
      >{{ Math.round(snapshot.llm_score) }}</span>
      <span v-else-if="snapshot" class="bhp-badge bhp-badge--pending">待评分</span>
      <span v-else class="bhp-badge bhp-badge--pending">等待首轮体检</span>
      <span v-if="checkedAtText" class="bhp-checked-at">上次体检 {{ checkedAtText }}</span>
      <div class="bhp-spacer" />
      <AppButton variant="secondary" size="sm" data-testid="board-health-add" @click="togglePicker">
        <Icon :icon="showPicker ? 'mdi:close' : 'mdi:plus'" width="14" />
        {{ showPicker ? '取消' : '添加构成' }}
      </AppButton>
    </div>

    <!-- 体检区：loading / error / empty / success 四态独立于构成管理 -->
    <div v-if="healthLoading" class="bhp-health-loading" data-testid="board-health-loading">
      体检数据加载中…
    </div>
    <div v-else-if="healthError" class="bhp-health-error" data-testid="board-health-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ healthError }}</span>
      <button type="button" class="bhp-retry" @click="loadHealth()">重试</button>
    </div>
    <div v-else-if="!snapshot" class="bhp-health-empty" data-testid="board-health-empty">
      等待首轮体检——体检在每日日报生成后自动执行，届时此处展示评分、构成质量与未收编候选。
      构成管理不受影响，可照常使用。
    </div>
    <template v-else>
      <!-- LLM 评语条 -->
      <div v-if="snapshot.llm_comment" class="bhp-comment" data-testid="board-health-comment">
        <Icon icon="mdi:comment-quote-outline" width="14" />
        <span>{{ snapshot.llm_comment }}</span>
      </div>

      <!-- 指标 chips -->
      <div class="bhp-chips" data-testid="board-health-chips">
        <span class="bhp-chip">挂载 tags <b>{{ boardMetrics?.matched_tags ?? 0 }}</b></span>
        <span class="bhp-chip" :class="{ 'bhp-chip--warn': (boardMetrics?.ghost_matches ?? 0) > 0 }">
          幽灵匹配 <b>{{ boardMetrics?.ghost_matches ?? 0 }}</b>
        </span>
        <span class="bhp-chip">向量缺失 <b>{{ boardMetrics?.vector_missing ?? 0 }}</b></span>
      </div>
    </template>

    <div v-if="notice" class="bhp-notice" data-testid="board-health-notice">
      <Icon icon="mdi:information-outline" width="14" />
      <span>{{ notice }}</span>
    </div>

    <!-- picker：版块推荐 / 手动指定双模式（搜索/分页由复用组件自带） -->
    <div v-if="showPicker" class="bhp-picker">
      <div class="bhp-picker-mode">
        <button
          type="button"
          class="bhp-mode-btn"
          :class="{ 'bhp-mode-btn--on': pickerMode === 'edit' }"
          @click="pickerMode = 'edit'"
        >版块推荐</button>
        <button
          type="button"
          class="bhp-mode-btn"
          :class="{ 'bhp-mode-btn--on': pickerMode === 'create' }"
          @click="pickerMode = 'create'"
        >手动指定</button>
      </div>
      <AuxiliaryLabelPicker
        :key="pickerMode"
        :mode="pickerMode"
        :board-id="boardId"
        :selected-ids="pendingIds"
        @update:selected-ids="pendingIds = $event"
      />
      <div v-if="pendingIds.length > 0" class="bhp-confirm">
        <AppButton
          size="sm"
          :loading="adding"
          :disabled="adding"
          data-testid="board-health-confirm-add"
          @click="handleConfirmAdd"
        >
          {{ adding ? '添加中...' : `确认添加 ${pendingIds.length} 个标签` }}
        </AppButton>
      </div>
    </div>

    <!-- 构成标签质量表（行内移除；无快照时质量列「待体检」） -->
    <div class="bhp-section">
      <AppSectionHeader title="构成标签质量" icon-name="mdi:puzzle-outline" />
      <div v-if="loading" class="bhp-loading" data-testid="board-health-labels-loading">
        构成标签加载中…
      </div>
      <div v-else-if="labels.length === 0" class="bhp-empty-hint">
        <Icon icon="mdi:tag-off-outline" width="20" />
        <span>暂无构成标签</span>
        <button type="button" class="bhp-empty-add" @click="showPicker = true">点击添加</button>
      </div>
      <table v-else class="bhp-table" data-testid="board-health-quality-table">
        <thead>
          <tr>
            <th>标签</th>
            <th class="bhp-num-col">样本</th>
            <th class="bhp-num-col">均值</th>
            <th class="bhp-num-col">中位数</th>
            <th class="bhp-num-col">低分占比</th>
            <th class="bhp-op-col" />
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visibleQualityRows" :key="row.id" data-testid="board-health-quality-row">
            <td class="bhp-label-cell">{{ row.label }}</td>
            <td class="bhp-num">{{ row.metric ? row.metric.samples : '待体检' }}</td>
            <td class="bhp-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">—</template>
              <template v-else>{{ fmtNum(row.metric.cosine_mean) }}</template>
            </td>
            <td class="bhp-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">—</template>
              <template v-else>{{ fmtNum(row.metric.cosine_median) }}</template>
            </td>
            <td class="bhp-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">数据不足</template>
              <template v-else>
                <span :class="{ 'bhp-low-sim': (row.metric.low_sim_ratio ?? 0) >= 0.3 }">
                  {{ fmtPct(row.metric.low_sim_ratio) }}
                </span>
              </template>
            </td>
            <td class="bhp-op-col">
              <button
                type="button"
                class="bhp-remove"
                title="移除"
                :data-testid="`board-health-remove-${row.id}`"
                @click="handleRemove(row.id, row.label)"
              >
                <Icon icon="mdi:close" width="10" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <button
        v-if="qualityRows.length > FOLD_LABELS"
        type="button"
        class="bhp-fold"
        data-testid="board-health-labels-fold"
        @click="labelsExpanded = !labelsExpanded"
      >
        {{ labelsExpanded ? '收起' : `展开全部 (${qualityRows.length})` }}
      </button>
    </div>

    <!-- 组合标签区（创建入口 + 挂载移除，平移自 BoardCompositionPanel） -->
    <div class="bhp-section" data-testid="board-health-composite-section">
      <div class="bhp-sub-header">
        <AppSectionHeader title="组合标签" icon-name="mdi:link-variant" />
        <AppButton variant="secondary" size="sm" data-testid="board-composite-create" @click="showCompositeDialog = true">
          <Icon icon="mdi:plus" width="14" />
          创建并挂载
        </AppButton>
      </div>
      <div v-if="!composites || composites.length === 0" class="bhp-comp-empty">
        暂未挂载组合标签——组合标签是最强匹配信号（composite_hit），适合「美国国债×收益率」这类指向性主题
      </div>
      <div v-else class="bhp-comp-list">
        <div
          v-for="item in visibleComposites"
          :key="item.id"
          class="bhp-comp-item"
          :class="{ 'bhp-comp-item--disabled': item.status !== 'active' }"
          :data-testid="`board-composite-item-${item.id}`"
        >
          <span class="bhp-comp-name">{{ item.label }}</span>
          <span class="bhp-comp-chain">= {{ item.components.join(' × ') }}</span>
          <button type="button" class="bhp-remove" title="移除挂载" @click="handleRemoveCompositeMount(item)">
            <Icon icon="mdi:close" width="10" />
          </button>
        </div>
      </div>
      <button
        v-if="(composites?.length ?? 0) > FOLD_COMPOSITES"
        type="button"
        class="bhp-fold"
        data-testid="board-health-composites-fold"
        @click="compositesExpanded = !compositesExpanded"
      >
        {{ compositesExpanded ? '收起' : `展开全部 (${composites?.length})` }}
      </button>
      <CompositeLabelEditDialog
        :visible="showCompositeDialog"
        :board-id="boardId"
        :board-label="boardLabel"
        @confirm="emit('refresh'); showCompositeDialog = false"
        @cancel="showCompositeDialog = false"
      />
    </div>

    <!-- two-col：趋势 | 未收编候选（窄容器 flex-wrap 折行单列） -->
    <div v-if="snapshot" class="bhp-two-col" data-testid="board-health-two-col">
      <div class="bhp-col">
        <AppSectionHeader title="评分趋势" icon-name="mdi:chart-line" />
        <svg v-if="trendPolyline" class="bhp-trend" viewBox="0 0 220 44" preserveAspectRatio="none" data-testid="board-health-trend">
          <polyline :points="trendPolyline" fill="none" stroke="var(--color-accent)" stroke-width="2" />
        </svg>
        <div v-else class="bhp-trend-empty">评分点不足，暂无趋势线</div>
        <div class="bhp-trend-summary">{{ trendSummary }}</div>
      </div>
      <div class="bhp-col">
        <AppSectionHeader title="未收编候选" icon-name="mdi:tag-plus-outline" description="成员 tags 高频携带、但不在版块构成中的辅助标签" />
        <div v-if="visibleCandidates.length === 0" class="bhp-cand-empty">暂无未收编候选</div>
        <div v-else class="bhp-cand-list" data-testid="board-health-candidates">
          <div
            v-for="c in visibleCandidates"
            :key="c.aux_id"
            class="bhp-cand-item"
            :data-testid="`board-health-candidate-${c.aux_id}`"
          >
            <div class="bhp-cand-main">
              <span class="bhp-cand-label">{{ c.label }}</span>
              <span class="bhp-cand-evidence">{{ c.member_tags }} tags · {{ c.member_articles }} 篇</span>
            </div>
            <AppButton
              variant="secondary"
              size="sm"
              :loading="capturingId === c.aux_id"
              :disabled="capturingId !== null"
              :data-testid="`board-health-capture-${c.aux_id}`"
              @click="handleCapture(c)"
            >
              收编
            </AppButton>
          </div>
        </div>
        <button
          v-if="(snapshot.uncaptured_candidates?.length ?? 0) > FOLD_CANDIDATES"
          type="button"
          class="bhp-fold"
          data-testid="board-health-candidates-fold"
          @click="candidatesExpanded = !candidatesExpanded"
        >
          {{ candidatesExpanded ? '收起' : `展开全部 (${snapshot.uncaptured_candidates?.length})` }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bhp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
}

.bhp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bhp-header-icon {
  color: var(--color-text-muted);
}

.bhp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.bhp-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 20px;
  padding: 0 8px;
  border-radius: 10px;
  border: 1px solid transparent;
  font-size: 11px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.bhp-badge--good {
  color: var(--color-success);
  background: var(--color-success-subtle);
  border-color: var(--color-success-subtle);
}

.bhp-badge--mid {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
  border-color: var(--color-warning-subtle);
}

.bhp-badge--low {
  color: var(--color-error);
  background: var(--color-error-subtle);
  border-color: var(--color-error-subtle);
}

.bhp-badge--pending {
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-color: var(--color-border-subtle);
  font-weight: 500;
}

.bhp-checked-at {
  font-size: 11px;
  color: var(--color-text-muted);
}

.bhp-spacer {
  flex: 1;
}

.bhp-health-loading,
.bhp-loading {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bhp-health-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  border: 1px solid var(--color-error-subtle);
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 0.74rem;
}

.bhp-retry {
  margin-left: auto;
  border: 1px solid var(--color-error);
  background: none;
  color: var(--color-error);
  border-radius: 6px;
  padding: 0.1rem 0.5rem;
  font-size: 0.7rem;
  cursor: pointer;
}

.bhp-health-empty {
  padding: 0.6rem 0.7rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  border: 1px dashed var(--color-border-medium);
  font-size: 0.74rem;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.bhp-comment {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  border-left: 3px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-text-primary);
  font-size: 0.74rem;
  line-height: 1.5;
}

.bhp-comment span {
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.bhp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.bhp-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.18rem 0.5rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-base);
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.bhp-chip b {
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.bhp-chip--warn {
  border-color: var(--color-warning-subtle);
}

.bhp-chip--warn b {
  color: var(--color-warning);
}

.bhp-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  border: 1px solid var(--color-secondary);
  background: var(--color-secondary);
  color: var(--color-text-inverted);
  font-size: 0.68rem;
  line-height: 1.5;
}

.bhp-picker {
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-input-bg);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.bhp-picker-mode {
  display: inline-flex;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  overflow: hidden;
  width: fit-content;
}

.bhp-mode-btn {
  border: none;
  background: var(--color-bg-base);
  padding: 0.25rem 0.7rem;
  font-size: 0.7rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bhp-mode-btn + .bhp-mode-btn {
  border-left: 1px solid var(--color-border-subtle);
}

.bhp-mode-btn--on {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-weight: 600;
}

.bhp-confirm {
  display: flex;
  justify-content: flex-end;
}

.bhp-section {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.bhp-empty-hint {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.8rem 0;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.bhp-empty-add {
  font-size: 0.68rem;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  border: 1px solid var(--color-secondary);
  background: none;
  color: var(--color-secondary);
  cursor: pointer;
}

.bhp-empty-add:hover {
  background: var(--color-secondary);
  color: var(--color-text-inverted);
}

.bhp-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.bhp-table th {
  text-align: left;
  font-weight: 500;
  color: var(--color-text-muted);
  font-size: 11px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-border-medium);
}

.bhp-table td {
  padding: 7px 8px;
  border-bottom: 1px solid var(--color-border-subtle);
  vertical-align: middle;
  color: var(--color-text-primary);
}

.bhp-num-col {
  width: 84px;
}

.bhp-op-col {
  width: 30px;
}

.bhp-label-cell {
  font-weight: 500;
}

.bhp-num {
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.bhp-low-sim {
  color: var(--color-warning);
  font-weight: 600;
}

.bhp-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border: none;
  border-radius: 4px;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
  opacity: 0.6;
  transition: all 0.12s ease;
}

.bhp-remove:hover {
  opacity: 1;
  color: var(--color-error);
  background: var(--color-error-subtle);
}

.bhp-fold {
  align-self: flex-start;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  border-radius: 7px;
  padding: 0.2rem 0.6rem;
  font-size: 0.7rem;
  cursor: pointer;
}

.bhp-fold:hover {
  background: var(--color-bg-hover);
}

.bhp-sub-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
}

.bhp-sub-header > :first-child {
  flex: 1;
}

.bhp-comp-empty {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bhp-comp-list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bhp-comp-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
}

.bhp-comp-item--disabled {
  opacity: 0.55;
}

.bhp-comp-name {
  font-weight: 600;
  font-size: 0.78rem;
  color: var(--color-text-primary);
}

.bhp-comp-chain {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.bhp-two-col {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  border-top: 1px dashed var(--color-border-subtle);
  padding-top: 0.6rem;
}

.bhp-col {
  flex: 1;
  min-width: 300px;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bhp-trend {
  width: 100%;
  height: 44px;
}

.bhp-trend-empty,
.bhp-trend-summary,
.bhp-cand-empty {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.bhp-trend-summary {
  font-variant-numeric: tabular-nums;
}

.bhp-cand-list {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.bhp-cand-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
}

.bhp-cand-main {
  flex: 1;
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.bhp-cand-label {
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.bhp-cand-evidence {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
