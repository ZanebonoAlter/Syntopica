<script setup lang="ts">
/**
 * 治理明细区（rework-workshop-into-radar-and-desk task 0.3 / 3.2）：
 * 构成标签质量表 + 组合标签挂载 + 未收编候选全量 + 添加构成（picker 双模式）。
 *
 * 从 `BoardHealthPanel` 拆出的独立可渲染区块：只读展示 + 三类动作外抛
 * （remove/refresh/capture），自身只负责「添加构成」的本地交互（picker/notice）。
 * 数据 props 由宿主传入（面板用 `useBoardHealth`，抽屉后续复用同一份）。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import {
  useSemanticBoardsApi,
  type AuxiliaryLabelItem,
  type BoardCompositeMount,
  type BoardHealthCandidate,
  type BoardHealthLabelMetric,
  type BoardHealthSnapshotDTO,
} from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import AuxiliaryLabelPicker from '../AuxiliaryLabelPicker.vue'
import CompositeLabelEditDialog from '../CompositeLabelEditDialog.vue'

const props = withDefaults(defineProps<{
  boardId: number
  labels: AuxiliaryLabelItem[]
  loading: boolean
  snapshot: BoardHealthSnapshotDTO | null
  composites?: BoardCompositeMount[]
  boardLabel?: string
  capturingId?: number | null
  /**
   * 渲染范围（治理抽屉按页签拆开显示）：
   * - `all`（默认，旧面板 / 全量视图）
   * - `quality` 构成标签质量表；`composites` 组合标签；`candidates` 未收编候选
   */
  mode?: 'all' | 'quality' | 'composites' | 'candidates'
}>(), {
  mode: 'all',
})

const emit = defineEmits<{
  remove: [auxiliaryLabelId: number]
  refresh: []
  capture: [candidate: BoardHealthCandidate]
}>()

const api = useSemanticBoardsApi()
const { success: notifySuccess } = useNotify()

/** 长列表折叠（质量表/组合 top 10、候选 top 5）。 */
const FOLD_LABELS = 10
const FOLD_COMPOSITES = 10
const FOLD_CANDIDATES = 5

const labelsExpanded = ref(false)
const compositesExpanded = ref(false)
const candidatesExpanded = ref(false)
const showPicker = ref(false)
const pickerMode = ref<'edit' | 'create'>('edit')
const pendingIds = ref<number[]>([])
const adding = ref(false)
const notice = ref('')
const showCompositeDialog = ref(false)

watch(() => props.boardId, () => {
  labelsExpanded.value = false
  compositesExpanded.value = false
  candidatesExpanded.value = false
  showPicker.value = false
  pendingIds.value = []
  notice.value = ''
})

/** 质量表行：构成标签（props.labels）联查体检归因（按 aux_id）。 */
interface QualityRow {
  id: number
  label: string
  metric: BoardHealthLabelMetric | null
}

const qualityRows = computed<QualityRow[]>(() => {
  const byAux = new Map<number, BoardHealthLabelMetric>()
  for (const m of props.snapshot?.metrics.labels ?? []) byAux.set(m.aux_id, m)
  const rows: QualityRow[] = props.labels.map((l) => ({
    id: l.id,
    label: l.label,
    metric: byAux.get(l.id) ?? null,
  }))
  // 样本数降序（无体检数据保持构成原序），稳定可复现。
  rows.sort((a, b) => (b.metric?.samples ?? 0) - (a.metric?.samples ?? 0))
  return rows
})

const visibleQualityRows = computed(() =>
  labelsExpanded.value ? qualityRows.value : qualityRows.value.slice(0, FOLD_LABELS),
)
const visibleComposites = computed(() => {
  const list = props.composites ?? []
  return compositesExpanded.value ? list : list.slice(0, FOLD_COMPOSITES)
})
const visibleCandidates = computed(() => {
  const list = props.snapshot?.uncaptured_candidates ?? []
  return candidatesExpanded.value ? list : list.slice(0, FOLD_CANDIDATES)
})

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

function onCapture(c: BoardHealthCandidate) {
  if (props.capturingId !== null && props.capturingId !== undefined) return
  emit('capture', c)
}

function fmtPct(v: number | null | undefined): string {
  return v == null ? '—' : `${Math.round(v * 100)}%`
}
function fmtNum(v: number | null | undefined, digits = 2): string {
  return v == null ? '—' : v.toFixed(digits)
}
</script>

<template>
  <div class="bhd" data-testid="board-health-detail">
    <div v-if="notice" class="bhd-notice" data-testid="board-health-notice">
      <Icon icon="mdi:information-outline" width="14" />
      <span>{{ notice }}</span>
    </div>

    <!-- picker：版块推荐 / 手动指定双模式（搜索/分页由复用组件自带） -->
    <div v-if="showPicker" class="bhd-picker">
      <div class="bhd-picker-mode">
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
      <div v-if="(mode === 'all' || mode === 'quality') && pendingIds.length > 0" class="bhd-confirm">
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
    <div v-if="mode === 'all' || mode === 'quality'" class="bhd-section">
      <div class="bhd-sub-header">
        <AppSectionHeader title="构成标签质量" icon-name="mdi:puzzle-outline" />
        <AppButton variant="secondary" size="sm" data-testid="board-health-add" @click="togglePicker">
          <Icon :icon="showPicker ? 'mdi:close' : 'mdi:plus'" width="14" />
          {{ showPicker ? '取消' : '添加构成' }}
        </AppButton>
      </div>
      <div v-if="loading" class="bhd-loading" data-testid="board-health-labels-loading">
        构成标签加载中…
      </div>
      <div v-else-if="labels.length === 0" class="bhd-empty-hint">
        <Icon icon="mdi:tag-off-outline" width="20" />
        <span>暂无构成标签</span>
        <button type="button" class="bhd-empty-add" @click="showPicker = true">点击添加</button>
      </div>
      <table v-else class="bhd-table" data-testid="board-health-quality-table">
        <thead>
          <tr>
            <th>标签</th>
            <th class="bhd-num-col">样本</th>
            <th class="bhd-num-col">均值</th>
            <th class="bhd-num-col">中位数</th>
            <th class="bhd-num-col">低分占比</th>
            <th class="bhd-op-col" />
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visibleQualityRows" :key="row.id" data-testid="board-health-quality-row">
            <td class="bhd-label-cell">{{ row.label }}</td>
            <td class="bhd-num">{{ row.metric ? row.metric.samples : '待体检' }}</td>
            <td class="bhd-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">—</template>
              <template v-else>{{ fmtNum(row.metric.cosine_mean) }}</template>
            </td>
            <td class="bhd-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">—</template>
              <template v-else>{{ fmtNum(row.metric.cosine_median) }}</template>
            </td>
            <td class="bhd-num">
              <template v-if="!snapshot">待体检</template>
              <template v-else-if="!row.metric || row.metric.insufficient">数据不足</template>
              <template v-else>
                <span :class="{ 'bhd-low-sim': (row.metric.low_sim_ratio ?? 0) >= 0.3 }">
                  {{ fmtPct(row.metric.low_sim_ratio) }}
                </span>
              </template>
            </td>
            <td class="bhd-op-col">
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
        class="bhd-fold"
        data-testid="board-health-labels-fold"
        @click="labelsExpanded = !labelsExpanded"
      >
        {{ labelsExpanded ? '收起' : `展开全部 (${qualityRows.length})` }}
      </button>
    </div>

    <!-- 组合标签区（创建入口 + 挂载移除，平移自 BoardCompositionPanel） -->
    <div v-if="mode === 'all' || mode === 'composites'" class="bhd-section" data-testid="board-health-composite-section">
      <div class="bhd-sub-header">
        <AppSectionHeader title="组合标签" icon-name="mdi:link-variant" />
        <AppButton variant="secondary" size="sm" data-testid="board-composite-create" @click="showCompositeDialog = true">
          <Icon icon="mdi:plus" width="14" />
          创建并挂载
        </AppButton>
      </div>
      <div v-if="!composites || composites.length === 0" class="bhd-comp-empty">
        暂未挂载组合标签——组合标签是最强匹配信号（composite_hit），适合「美国国债×收益率」这类指向性主题
      </div>
      <div v-else class="bhd-comp-list">
        <div
          v-for="item in visibleComposites"
          :key="item.id"
          class="bhd-comp-item"
          :class="{ 'bhd-comp-item--disabled': item.status !== 'active' }"
          :data-testid="`board-composite-item-${item.id}`"
        >
          <span class="bhd-comp-name">{{ item.label }}</span>
          <span class="bhd-comp-chain">= {{ item.components.join(' × ') }}</span>
          <button type="button" class="bhp-remove" title="移除挂载" @click="handleRemoveCompositeMount(item)">
            <Icon icon="mdi:close" width="10" />
          </button>
        </div>
      </div>
      <button
        v-if="(composites?.length ?? 0) > FOLD_COMPOSITES"
        type="button"
        class="bhd-fold"
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

    <!-- 未收编候选全量（默认 top 5，可展开） -->
    <div v-if="mode === 'all' || mode === 'candidates'" class="bhd-section">
      <AppSectionHeader title="未收编候选" icon-name="mdi:tag-plus-outline" description="成员 tags 高频携带、但不在版块构成中的辅助标签" />
      <div v-if="visibleCandidates.length === 0" class="bhd-cand-empty">暂无未收编候选</div>
      <div v-else class="bhd-cand-list" data-testid="board-health-candidates">
        <div
          v-for="c in visibleCandidates"
          :key="c.aux_id"
          class="bhd-cand-item"
          :data-testid="`board-health-candidate-${c.aux_id}`"
        >
          <div class="bhd-cand-main">
            <span class="bhd-cand-label">{{ c.label }}</span>
            <span class="bhd-cand-evidence">{{ c.member_tags }} tags · {{ c.member_articles }} 篇</span>
          </div>
          <AppButton
            variant="secondary"
            size="sm"
            :loading="capturingId === c.aux_id"
            :disabled="capturingId !== null && capturingId !== undefined"
            :data-testid="`board-health-capture-${c.aux_id}`"
            @click="onCapture(c)"
          >
            收编
          </AppButton>
        </div>
      </div>
      <button
        v-if="(snapshot?.uncaptured_candidates?.length ?? 0) > FOLD_CANDIDATES"
        type="button"
        class="bhd-fold"
        data-testid="board-health-candidates-fold"
        @click="candidatesExpanded = !candidatesExpanded"
      >
        {{ candidatesExpanded ? '收起' : `展开全部 (${snapshot?.uncaptured_candidates?.length})` }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.bhd {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.bhd-notice {
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

.bhd-picker {
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-input-bg);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.bhd-picker-mode {
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

.bhd-confirm {
  display: flex;
  justify-content: flex-end;
}

.bhd-section {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.bhd-sub-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
}

.bhd-sub-header > :first-child {
  flex: 1;
}

.bhd-loading {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bhd-empty-hint {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.8rem 0;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.bhd-empty-add {
  font-size: 0.68rem;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  border: 1px solid var(--color-secondary);
  background: none;
  color: var(--color-secondary);
  cursor: pointer;
}

.bhd-empty-add:hover {
  background: var(--color-secondary);
  color: var(--color-text-inverted);
}

.bhd-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.bhd-table th {
  text-align: left;
  font-weight: 500;
  color: var(--color-text-muted);
  font-size: 11px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-border-medium);
}

.bhd-table td {
  padding: 7px 8px;
  border-bottom: 1px solid var(--color-border-subtle);
  vertical-align: middle;
  color: var(--color-text-primary);
}

.bhd-num-col {
  width: 84px;
}

.bhd-op-col {
  width: 30px;
}

.bhd-label-cell {
  font-weight: 500;
}

.bhd-num {
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.bhd-low-sim {
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

.bhd-fold {
  align-self: flex-start;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  border-radius: 7px;
  padding: 0.2rem 0.6rem;
  font-size: 0.7rem;
  cursor: pointer;
}

.bhd-fold:hover {
  background: var(--color-bg-hover);
}

.bhd-comp-empty {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bhd-comp-list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bhd-comp-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
}

.bhd-comp-item--disabled {
  opacity: 0.55;
}

.bhd-comp-name {
  font-weight: 600;
  font-size: 0.78rem;
  color: var(--color-text-primary);
}

.bhd-comp-chain {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.bhd-cand-empty {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.bhd-cand-list {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.bhd-cand-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
}

.bhd-cand-main {
  flex: 1;
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.bhd-cand-label {
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.bhd-cand-evidence {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
