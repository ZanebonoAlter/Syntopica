<script setup lang="ts">
import { computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { UpgradeSuggestionRow, SemanticBoard } from '~/api/semanticBoards'

const props = defineProps<{
  visible: boolean
  backfillNotice: boolean
  /** 持久化建议（唯一数据源，GET /upgrade-suggestions）。 */
  persistedSuggestions: UpgradeSuggestionRow[]
  persistedLoading: boolean
  persistedGenerating: boolean
  /** 生成失败信息（入口区行内提示，空=无错误）。 */
  generateError?: string
  /** 全量 active 板块（扩充方向的锁定版块单选下拉数据源）。 */
  boards: SemanticBoard[]
}>()

const emit = defineEmits<{
  cancel: []
  loadPersisted: [decision: string]
  generate: [params: { direction: 'create' | 'expand'; source: 'aux' | 'composite'; target_board_id?: number; days?: number }]
  dismissRow: [id: number]
  confirmRow: [row: UpgradeSuggestionRow]
}>()

// ---- 生成入口：方向 × 来源 两步选择 + 扩充锁定版块（spec: 生成入口模式选择）----
type GenDirection = 'create' | 'expand'
type GenSource = 'aux' | 'composite'
const genDirection = ref<GenDirection>('create')
const genSource = ref<GenSource>('aux')
const genTargetBoardId = ref<number | null>(null)
const genDays = ref(1)
const boardSearch = ref('')
// 本会话是否已生成过（区分空态：「未生成过 → 引导」vs「生成过无建议 → 覆盖提示」）。
const hasGenerated = ref(false)

const dayOptions = [
  { value: 1, label: '今天' },
  { value: 3, label: '最近3天' },
  { value: 7, label: '最近7天' },
  { value: 30, label: '最近30天' },
  { value: 0, label: '全部' },
]

const filteredBoards = computed(() => {
  const kw = boardSearch.value.trim().toLowerCase()
  if (!kw) return props.boards
  return props.boards.filter((b) =>
    b.label.toLowerCase().includes(kw)
    || (b.aliases ?? []).some((a) => a.toLowerCase().includes(kw)))
})

// 扩充方向必须选定版块才能生成（spec: 扩充方向必须选定版块）。
const canGenerate = computed(() =>
  genDirection.value === 'create' || (genTargetBoardId.value != null && genTargetBoardId.value > 0))

function handleGenerate() {
  if (!canGenerate.value || props.persistedGenerating) return
  hasGenerated.value = true
  emit('generate', {
    direction: genDirection.value,
    source: genSource.value,
    ...(genDirection.value === 'expand' && genTargetBoardId.value
      ? { target_board_id: genTargetBoardId.value }
      : {}),
    ...(genDirection.value === 'create' && genSource.value === 'aux' ? { days: genDays.value } : {}),
  })
}

// 每行勾选的辅助标签 id 集合（row.id → Set<auxId>）。默认全选；emit 时只带勾选子集。
const selectedAuxByRow = ref<Record<number, Set<number>>>({})

// ---- 持久化建议过滤（唯一数据源） ----
type PersistedFilter = 'all' | 'merge_into_existing' | 'create_new' | 'compose'
const persistedFilter = ref<PersistedFilter>('all')
const filterTabs: { key: PersistedFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'merge_into_existing', label: '合并' },
  { key: 'create_new', label: '新建' },
  { key: 'compose', label: '组合' },
]

// all → decision=""（后端默认列表）；其余原样传
function decisionParam(tab: PersistedFilter): string {
  return tab === 'all' ? '' : tab
}

watch(persistedFilter, (tab) => emit('loadPersisted', decisionParam(tab)))
watch(() => props.visible, (v) => {
  if (v) emit('loadPersisted', decisionParam(persistedFilter.value))
})
// 切换方向时重置版块选择（create 不需要 target）。
watch(genDirection, () => {
  genTargetBoardId.value = null
  boardSearch.value = ''
})

// ---- evidence 安全读取（存量建议的 shortlist/泳道/共现快照，缺 key 降级不崩） ----
function laneBriefs(row: UpgradeSuggestionRow): string[] {
  const v = row.evidence?.lane_briefs
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}
function cotagEvents(row: UpgradeSuggestionRow): string[] {
  const v = row.evidence?.cotag_events
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}
interface ShortlistItem {
  board_id?: number
  board_label?: string
  composition_dist?: number
  lane_dist?: number
}
function shortlist(row: UpgradeSuggestionRow): ShortlistItem[] {
  const v = row.evidence?.shortlist
  return Array.isArray(v) ? v.filter((x): x is ShortlistItem => x != null && typeof x === 'object') : []
}
function rowAuxLabels(row: UpgradeSuggestionRow): { id: number; label: string; status?: string }[] {
  return row.auxiliary_labels && row.auxiliary_labels.length > 0
    ? row.auxiliary_labels
    : row.auxiliary_label_ids.map((id) => ({ id, label: `标签 #${id}` }))
}

// ---- 辅助标签勾选（不必全要，emit 时只带勾选子集）----
watch(() => props.persistedSuggestions, (rows) => {
  const next: Record<number, Set<number>> = {}
  for (const row of rows) {
    next[row.id] = new Set(rowAuxLabels(row).filter((al) => !isAuxDisabled(al)).map((al) => al.id))
  }
  selectedAuxByRow.value = next
}, { immediate: true, deep: false })

function isAuxDisabled(al: { status?: string }): boolean {
  return !!al.status && al.status !== 'active'
}
function isAuxSelected(rowId: number, auxId: number): boolean {
  return selectedAuxByRow.value[rowId]?.has(auxId) ?? true
}
function toggleAux(rowId: number, auxId: number): void {
  const set = selectedAuxByRow.value[rowId] ?? new Set<number>()
  if (set.has(auxId)) set.delete(auxId)
  else set.add(auxId)
  selectedAuxByRow.value[rowId] = set
}
function selectAllAux(row: UpgradeSuggestionRow): void {
  selectedAuxByRow.value[row.id] = new Set(rowAuxLabels(row).map((al) => al.id))
}
function clearAllAux(row: UpgradeSuggestionRow): void {
  selectedAuxByRow.value[row.id] = new Set<number>()
}
function selectedAuxCount(row: UpgradeSuggestionRow): number {
  const all = rowAuxLabels(row)
  return all.filter((al) => isAuxSelected(row.id, al.id)).length
}
function selectedAuxIds(row: UpgradeSuggestionRow): number[] {
  return rowAuxLabels(row).filter((al) => isAuxSelected(row.id, al.id)).map((al) => al.id)
}
function rowTargetLabel(row: UpgradeSuggestionRow): string {
  if (row.target_board_label) return row.target_board_label
  return row.target_board_id ? `板块 #${row.target_board_id}` : ''
}

// ---- compose 建议证据（安全读取，缺 key 降级） ----
function composeCooccurrence(row: UpgradeSuggestionRow): number | null {
  const v = row.evidence?.compose_cooccurrence
  return typeof v === 'number' ? v : null
}
function composeWindowDays(row: UpgradeSuggestionRow): number | null {
  const v = row.evidence?.compose_window_days
  return typeof v === 'number' ? v : null
}
function composeRepresentTitles(row: UpgradeSuggestionRow): string[] {
  const v = row.evidence?.compose_representative_titles
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}

// 确认动作（单一入口，带勾选子集；target 来自建议本身——扩充建议的锁定版块）。
function handleConfirmRow(row: UpgradeSuggestionRow) {
  emit('confirmRow', { ...row, auxiliary_label_ids: selectedAuxIds(row) })
}

function decisionLabel(d: string): string {
  switch (d) {
    case 'create_new': return '创建新板块'
    case 'merge_into_existing': return '合并到已有板块'
    case 'compose': return '创建组合标签'
    default: return d
  }
}

function decisionStyle(d: string): { border: string; bg: string; color: string } {
  switch (d) {
    case 'create_new': return { border: 'var(--color-success-border, rgba(61,138,74,0.3))', bg: 'var(--color-success-bg, rgba(61,138,74,0.08))', color: 'var(--color-success)' }
    case 'merge_into_existing': return { border: 'var(--color-link-border)', bg: 'var(--color-link-subtle)', color: 'var(--color-link)' }
    case 'compose': return { border: 'var(--color-accent-border, rgba(120,80,200,0.3))', bg: 'var(--color-accent-subtle)', color: 'var(--color-accent)' }
    default: return { border: 'var(--color-border-subtle)', bg: 'var(--color-bg-hover)', color: 'var(--color-text-secondary)' }
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="usp-overlay" @click.self="emit('cancel')">
      <div class="usp-card">
        <div class="usp-header">
          <div>
            <h3 class="usp-title">板块升级建议</h3>
            <p class="usp-subtitle">创建 / 扩充分开裁决，每轮 LLM 只做一种判断</p>
          </div>
          <button type="button" class="usp-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <!-- 生成入口：方向 → 来源 →（扩充）锁定版块（spec: 生成入口模式选择） -->
        <section class="usp-gen" data-testid="upgrade-gen-entry">
          <div class="usp-gen-row">
            <div class="usp-mode-selector usp-gen-group">
              <label class="usp-mode-option">
                <input v-model="genDirection" type="radio" value="create" :disabled="persistedGenerating" data-testid="gen-direction-create" />
                <span>创建版块</span>
              </label>
              <label class="usp-mode-option">
                <input v-model="genDirection" type="radio" value="expand" :disabled="persistedGenerating" data-testid="gen-direction-expand" />
                <span>版块扩充</span>
              </label>
            </div>
            <div class="usp-mode-selector usp-gen-group">
              <label class="usp-mode-option">
                <input v-model="genSource" type="radio" value="aux" :disabled="persistedGenerating" data-testid="gen-source-aux" />
                <span>单标签</span>
              </label>
              <label class="usp-mode-option">
                <input v-model="genSource" type="radio" value="composite" :disabled="persistedGenerating" data-testid="gen-source-composite" />
                <span>组合标签</span>
              </label>
            </div>
          </div>
          <div v-if="genDirection === 'expand'" class="usp-gen-row">
            <div class="usp-merge-dropdown usp-merge-dropdown--search usp-gen-board-picker">
              <input
                class="usp-merge-search"
                type="text"
                placeholder="选择要扩充的版块（仅本轮目标）…"
                :value="boardSearch"
                data-testid="gen-board-search"
                @input="boardSearch = ($event.target as HTMLInputElement).value"
              >
              <div class="usp-merge-list">
                <button
                  v-for="b in filteredBoards"
                  :key="b.id"
                  type="button"
                  class="usp-merge-option"
                  :class="{ 'usp-merge-option--recommended': genTargetBoardId === b.id }"
                  :data-testid="`gen-board-option-${b.id}`"
                  @click="genTargetBoardId = b.id"
                >
                  <span class="usp-merge-option-name">{{ b.label }}</span>
                  <span v-if="genTargetBoardId === b.id" class="usp-merge-option-tag">已选</span>
                </button>
                <span v-if="filteredBoards.length === 0" class="usp-merge-empty">无匹配板块</span>
              </div>
            </div>
          </div>
          <div v-if="genDirection === 'create' && genSource === 'aux'" class="usp-gen-row usp-gen-row--days">
            <label class="usp-gen-days-label">候选时间窗</label>
            <select v-model.number="genDays" class="usp-gen-days" :disabled="persistedGenerating" data-testid="gen-days">
              <option v-for="o in dayOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div class="usp-gen-row usp-gen-row--action">
            <button
              type="button"
              class="usp-suggest-btn usp-suggest-btn--small"
              :disabled="persistedGenerating || !canGenerate"
              :title="!canGenerate ? '版块扩充需先选定一个目标版块' : undefined"
              data-testid="gen-submit"
              @click="handleGenerate()"
            >
              <Icon v-if="persistedGenerating" icon="mdi:loading" width="13" class="animate-spin" />
              <Icon v-else icon="mdi:auto-fix" width="13" />
              {{ persistedGenerating ? '生成中...' : '生成建议' }}
            </button>
            <span v-if="genDirection === 'expand' && !canGenerate" class="usp-gen-hint">请先选定要扩充的版块</span>
            <span v-if="generateError" class="usp-gen-error" data-testid="gen-error">{{ generateError }}</span>
          </div>
        </section>

        <!-- 持久化建议（唯一数据源） -->
        <section class="usp-persisted">
          <div class="usp-persisted-toolbar">
            <div class="usp-filter-tabs">
              <button
                v-for="t in filterTabs"
                :key="t.key"
                type="button"
                class="usp-filter-tab"
                :class="{ 'is-active': persistedFilter === t.key }"
                @click="persistedFilter = t.key"
              >
                {{ t.label }}
              </button>
            </div>
          </div>

          <div v-if="backfillNotice" class="usp-notice">
            <Icon icon="mdi:information-outline" width="14" />
            <span>已执行升级建议。历史标签归属不会自动回填，可手动触发匹配回填让新构成生效。</span>
          </div>

          <div v-if="persistedLoading" class="usp-loading">
            <Icon icon="mdi:loading" width="20" class="animate-spin text-white/30" />
            <span>加载建议...</span>
          </div>
          <div v-else-if="persistedSuggestions.length === 0" class="usp-persisted-empty">
            <Icon :icon="hasGenerated ? 'mdi:check-circle-outline' : 'mdi:lightbulb-on-outline'" width="16" />
            <span v-if="hasGenerated">{{ genDirection === 'expand' ? '本轮无建议——该版块可能已充分覆盖' : '本轮无建议' }}</span>
            <span v-else>选择方向与来源后生成建议</span>
          </div>
          <div v-else class="usp-persisted-list">
            <div
              v-for="row in persistedSuggestions"
              :key="row.id"
              class="usp-item usp-row"
              :class="{ 'usp-row--high': row.confidence === 'high' }"
              :data-confidence="row.confidence"
              :data-decision="row.decision"
              :style="{ borderColor: decisionStyle(row.decision).border, background: decisionStyle(row.decision).bg }"
            >
              <div class="usp-item-header">
                <span class="usp-item-decision" :style="{ color: decisionStyle(row.decision).color }">
                  {{ decisionLabel(row.decision) }}
                </span>
                <span v-if="row.confidence === 'high'" class="usp-confidence-badge" data-confidence="high">高置信</span>
                <span v-if="rowTargetLabel(row)" class="usp-item-board" data-testid="row-target-badge">→ {{ rowTargetLabel(row) }}</span>
                <span v-else-if="row.board_label" class="usp-item-board">{{ row.board_label }}</span>
              </div>
              <p v-if="row.description" class="usp-item-desc">{{ row.description }}</p>
              <div class="usp-aux-toolbar">
                <span class="usp-aux-count">{{ row.decision === 'compose' ? '组件' : '辅助标签' }} {{ selectedAuxCount(row) }}/{{ rowAuxLabels(row).length }}</span>
                <button type="button" class="usp-aux-toggle" @click="selectAllAux(row)">全选</button>
                <button type="button" class="usp-aux-toggle" @click="clearAllAux(row)">清空</button>
              </div>
              <div class="usp-item-tags">
                <button
                  v-for="al in rowAuxLabels(row)"
                  :key="al.id"
                  type="button"
                  class="usp-item-tag usp-aux-chip"
                  :class="{ 'usp-aux-chip--off': !isAuxSelected(row.id, al.id), 'usp-aux-chip--disabled': isAuxDisabled(al) }"
                  :disabled="isAuxDisabled(al)"
                  :title="isAuxDisabled(al) ? ((al.label || ('标签 #' + al.id)) + '（已失效）') : (al.label || ('标签 #' + al.id))"
                  @click="toggleAux(row.id, al.id)"
                >
                  <Icon
                    :icon="isAuxSelected(row.id, al.id) ? 'mdi:checkbox-marked' : 'mdi:checkbox-blank-outline'"
                    width="12"
                  />
                  {{ al.label || ('标签 #' + al.id) }}
                  <span v-if="isAuxDisabled(al)" class="usp-aux-chip-status">已失效</span>
                </button>
              </div>
              <div v-if="laneBriefs(row).length > 0" class="usp-evidence">
                <span class="usp-evidence-label">泳道：</span>
                <span v-for="(b, bi) in laneBriefs(row)" :key="'lb' + bi" class="usp-evidence-chip">{{ b }}</span>
              </div>
              <div v-if="cotagEvents(row).length > 0" class="usp-evidence">
                <span class="usp-evidence-label">共现：</span>
                <span v-for="(e, ei) in cotagEvents(row)" :key="'ce' + ei" class="usp-evidence-chip">{{ e }}</span>
              </div>
              <div v-if="row.decision === 'compose' && (composeCooccurrence(row) !== null || composeRepresentTitles(row).length > 0)" class="usp-evidence" data-testid="compose-evidence">
                <span class="usp-evidence-label">组合证据：</span>
                <span v-if="composeCooccurrence(row) !== null" class="usp-evidence-chip" data-testid="compose-cooccurrence">
                  共现 {{ composeCooccurrence(row) }} 篇<template v-if="composeWindowDays(row)"> / {{ composeWindowDays(row) }} 天窗口</template>
                </span>
                <span v-for="(title, ti) in composeRepresentTitles(row)" :key="'rt' + ti" class="usp-evidence-chip" :title="title">{{ title }}</span>
              </div>
              <div v-if="shortlist(row).length > 0" class="usp-evidence">
                <span class="usp-evidence-label">候选版块：</span>
                <span v-for="(s, si) in shortlist(row)" :key="'sl' + si" class="usp-evidence-chip">{{ s.board_label || ('板块 #' + s.board_id) }}</span>
              </div>
              <div class="usp-item-actions">
                <button
                  v-if="row.decision === 'merge_into_existing'"
                  type="button"
                  class="usp-item-btn usp-item-btn--primary"
                  :disabled="selectedAuxCount(row) === 0 || !row.target_board_id"
                  :title="!row.target_board_id ? '建议缺少目标版块（历史数据）' : (selectedAuxCount(row) === 0 ? '至少勾选一个辅助标签' : ('合并进 ' + rowTargetLabel(row)))"
                  data-testid="merge-confirm"
                  @click="handleConfirmRow(row)"
                >
                  <Icon icon="mdi:check" width="12" />
                  合并进{{ rowTargetLabel(row) ? `「${rowTargetLabel(row)}」` : '' }}
                </button>
                <button
                  v-if="row.decision === 'create_new'"
                  type="button"
                  class="usp-item-btn usp-item-btn--primary"
                  :disabled="selectedAuxCount(row) === 0"
                  :title="selectedAuxCount(row) === 0 ? '至少勾选一个辅助标签' : undefined"
                  @click="handleConfirmRow(row)"
                >
                  <Icon icon="mdi:check" width="12" />
                  创建新版块
                </button>
                <button
                  v-if="row.decision === 'compose'"
                  type="button"
                  class="usp-item-btn usp-item-btn--primary"
                  :disabled="selectedAuxCount(row) < 2"
                  :title="selectedAuxCount(row) < 2 ? '组合标签至少需要 2 个组件' : (rowTargetLabel(row) ? '创建组合并挂载进「' + rowTargetLabel(row) + '」' : undefined)"
                  data-testid="compose-confirm"
                  @click="handleConfirmRow(row)"
                >
                  <Icon icon="mdi:check" width="12" />
                  {{ rowTargetLabel(row) ? '创建并挂载' : '创建组合' }}
                </button>
                <button
                  type="button"
                  class="usp-item-btn usp-item-btn--dismiss"
                  @click="emit('dismissRow', row.id)"
                >
                  <Icon icon="mdi:close" width="12" />
                  忽略
                </button>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.usp-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-overlay);
  backdrop-filter: blur(8px);
  padding: 1rem;
}

.usp-card {
  width: min(560px, 95vw);
  max-height: 80vh;
  overflow-y: auto;
  border-radius: 1.25rem;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-elevated);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.usp-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.usp-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.usp-subtitle {
  margin-top: 0.25rem;
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.usp-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-close:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.usp-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.usp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 2rem 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.usp-mode-selector {
  display: flex;
  gap: 1rem;
  justify-content: center;
}

.usp-mode-option {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.usp-mode-option input[type="radio"] {
  accent-color: var(--color-accent);
  width: 14px;
  height: 14px;
}

.usp-suggest-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 1rem;
  border-radius: 10px;
  border: 1px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent-hover);
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-suggest-btn:hover:not(:disabled) {
  background: var(--color-accent-subtle);
}

.usp-suggest-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.usp-suggest-btn--small {
  padding: 0.35rem 0.65rem;
  font-size: 0.72rem;
}

.usp-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.usp-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.usp-toolbar-text {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.usp-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.45rem;
  padding: 0.65rem 0.75rem;
  border-radius: 10px;
  border: 1px solid var(--color-info-bg, rgba(61,122,138,0.25));
  background: var(--color-info-bg, rgba(61,122,138,0.08));
  color: var(--color-info);
  font-size: 0.72rem;
  line-height: 1.5;
}

.usp-item {
  padding: 0.85rem;
  border-radius: 12px;
  border: 1px solid;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.usp-item-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.usp-item-decision {
  font-size: 0.72rem;
  font-weight: 600;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  background: var(--color-input-bg);
}

.usp-item-board {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--color-text-primary);
}

.usp-item-desc {
  font-size: 0.75rem;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.usp-item-reason {
  font-size: 0.72rem;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.usp-item-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
}

.usp-item-tag {
  font-size: 0.65rem;
  color: var(--color-text-muted);
  padding: 0.1rem 0.35rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
}

/* 辅助标签勾选 chip */
.usp-aux-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.15rem;
}

.usp-aux-count {
  font-size: 0.64rem;
  color: var(--color-text-muted);
}

.usp-aux-toggle {
  border: none;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  cursor: pointer;
  padding: 0.05rem 0.2rem;
  border-radius: 4px;
  transition: all 0.1s ease;
}

.usp-aux-toggle:hover {
  color: var(--color-text-secondary);
  background: var(--color-bg-hover);
}

.usp-aux-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  border: 1px solid transparent;
  cursor: pointer;
  transition: all 0.1s ease;
  user-select: none;
}

.usp-aux-chip:hover {
  background: var(--color-bg-sunken);
}

/* 选中态：用 link 色高亮 */
.usp-aux-chip:not(.usp-aux-chip--off) {
  color: var(--color-link);
  background: var(--color-link-subtle);
}

/* 取消态：灰化 + 删除线 */
.usp-aux-chip--off {
  color: var(--color-text-muted);
  background: transparent;
  border-color: var(--color-border-subtle);
  text-decoration: line-through;
  opacity: 0.65;
}

/* 失效标签（建议生成后被禁用）：在 --off 基础上锁死不可交互 */
.usp-aux-chip--disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.usp-aux-chip--disabled:hover {
  background: transparent;
}

.usp-aux-chip-status {
  margin-left: 0.2rem;
  padding: 0.02rem 0.2rem;
  border-radius: 3px;
  background: var(--color-warning-bg, rgba(180, 140, 40, 0.18));
  color: var(--color-warning, #b48c28);
  font-size: 0.58rem;
  font-weight: 600;
}

.usp-item-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 0.25rem;
}

.usp-item-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.35rem 0.7rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-medium);
  background: none;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-item-btn--primary {
  border-color: var(--color-success-border, rgba(61,138,74,0.3));
  background: var(--color-success-bg, rgba(61,138,74,0.1));
  color: var(--color-success);
}

.usp-item-btn--primary:hover {
  background: var(--color-success-bg, rgba(61,138,74,0.18));
}

.usp-item-affinities {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.usp-item-affinities-label {
  color: var(--color-text-muted);
}

.usp-item-affinity {
  padding: 0.1rem 0.3rem;
  border-radius: 4px;
  background: var(--color-link-subtle);
  color: var(--color-link);
}

.usp-item-affinity-detail {
  color: var(--color-text-muted);
}

.usp-merge-wrapper {
  position: relative;
}

.usp-item-btn--merge {
  border-color: var(--color-link-border);
  background: var(--color-link-subtle);
  color: var(--color-link);
}

.usp-item-btn--merge:hover {
  background: var(--color-link-border);
}

.usp-merge-dropdown {
  position: absolute;
  right: 0;
  bottom: 100%;
  margin-bottom: 4px;
  min-width: 200px;
  border-radius: 8px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-elevated);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  z-index: 10;
  overflow: hidden;
}

.usp-merge-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0.45rem 0.65rem;
  border: none;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
  transition: background 0.1s ease;
}

.usp-merge-option:hover {
  background: var(--color-bg-hover);
}

.usp-merge-option-detail {
  color: var(--color-text-muted);
  font-size: 0.65rem;
}

/* 带 search 的下拉：加宽 + 内容区滚动 */
.usp-merge-dropdown--search {
  min-width: 260px;
  max-height: 320px;
  display: flex;
  flex-direction: column;
}

.usp-merge-search {
  width: 100%;
  padding: 0.45rem 0.65rem;
  border: none;
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font-size: 0.74rem;
  outline: none;
}

.usp-merge-search::placeholder {
  color: var(--color-text-muted);
}

.usp-merge-list {
  overflow-y: auto;
  max-height: 260px;
}

.usp-merge-group-label {
  padding: 0.3rem 0.65rem 0.15rem;
  font-size: 0.62rem;
  font-weight: 600;
  color: var(--color-text-muted);
  letter-spacing: 0.02em;
  background: var(--color-bg-sunken);
}

/* LLM/算法候选区高亮 */
.usp-merge-option--candidate {
  background: var(--color-link-subtle);
}

.usp-merge-option--candidate:hover {
  background: var(--color-link-border);
}

.usp-merge-option-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* LLM/high 已推荐的目标，在全量列表里标记 */
.usp-merge-option--recommended {
  background: var(--color-success-bg, rgba(61, 138, 74, 0.08));
}

.usp-merge-option--recommended:hover {
  background: var(--color-success-bg, rgba(61, 138, 74, 0.18));
}

.usp-merge-option-tag {
  margin-left: 0.4rem;
  padding: 0.05rem 0.3rem;
  border-radius: 4px;
  background: var(--color-success-bg, rgba(61, 138, 74, 0.2));
  color: var(--color-success);
  font-size: 0.6rem;
  font-weight: 600;
}

.usp-persisted {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.usp-persisted-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.usp-filter-tabs {
  display: flex;
  gap: 0.2rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  padding: 0.15rem;
}

.usp-filter-tab {
  padding: 0.25rem 0.55rem;
  border: none;
  border-radius: 6px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-filter-tab:hover {
  color: var(--color-text-secondary);
}

.usp-filter-tab.is-active {
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
}

.usp-persisted-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  padding: 1.2rem 0;
  color: var(--color-text-muted);
  font-size: 0.78rem;
}

.usp-persisted-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.usp-row--high {
  box-shadow: inset 3px 0 0 var(--color-success, #3d8a4a);
}

.usp-confidence-badge {
  font-size: 0.62rem;
  font-weight: 600;
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  background: var(--color-success-bg, rgba(61, 138, 74, 0.15));
  color: var(--color-success);
}

/* target_off_shortlist 警示徽标（board-upgrade spec 方案 B） */
.usp-off-shortlist-badge {
  background: var(--color-warning-bg, rgba(180, 140, 40, 0.15));
  color: var(--color-warning, #b48c28);
}

.usp-merge-empty {
  display: block;
  padding: 0.45rem 0.65rem;
  color: var(--color-text-muted);
  font-size: 0.7rem;
}

.usp-evidence {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.usp-evidence-label {
  color: var(--color-text-muted);
}

.usp-evidence-chip {
  padding: 0.08rem 0.3rem;
  border-radius: 4px;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.usp-item-btn--dismiss {
  border-color: var(--color-border-medium);
  color: var(--color-text-muted);
}

.usp-item-btn--dismiss:hover {
  background: var(--color-bg-hover);
  color: var(--color-danger, #c05050);
}

.usp-divider {
  height: 1px;
  background: var(--color-border-subtle);
  margin: 0.15rem 0;
}

.usp-manual-title {
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  letter-spacing: 0.02em;
}
.usp-gen {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 0.75rem;
  margin-bottom: 0.75rem;
  background: var(--color-bg-sunken, rgba(255, 255, 255, 0.02));
}

.usp-gen-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.usp-gen-row--days {
  gap: 0.5rem;
}

.usp-gen-group {
  margin: 0;
}

.usp-gen-days-label {
  font-size: 0.75rem;
  color: var(--color-text-secondary);
}

.usp-gen-days {
  background: var(--color-bg-input, rgba(255, 255, 255, 0.05));
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-medium);
  border-radius: 0.375rem;
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
}

.usp-gen-board-picker {
  position: static;
  width: 100%;
}

.usp-gen-hint {
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.usp-gen-error {
  font-size: 0.75rem;
  color: var(--color-danger, #d95c5c);
}

</style>
