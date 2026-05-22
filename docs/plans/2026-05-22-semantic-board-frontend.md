# Semantic Board 前端改造 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将现有基于 sector + 层级树的"标签管理"页面，改造为基于 SemanticBoard + AuxiliaryLabel 的"语义板块管理"体系，并同步更新 NarrativeBoard 展示。

**Architecture:** 保留 TagsPage 的左右分栏布局（左侧 sidebar 260px + 右侧 content），但将左侧列表从 Sector 替换为 SemanticBoard，右侧内容从层级树替换为 Board Composition / Auxiliary Label Pool。新增升级建议、回填进度、匹配参数配置三个功能面板，均以 Dialog 形式嵌入。NarrativePanel 移除 abstract_tag 相关展示，增加 semantic_board 来源标识。

**Tech Stack:** Nuxt 4, Vue 3 Composition API (`<script setup lang="ts">`), Tailwind CSS + scoped CSS, `@iconify/vue`, Pinia (optional), `ApiClient` in `app/api/client.ts`.

**Reference Docs:**
- API: `docs/reference/api/semantic-boards.md`
- Design: `openspec/changes/semantic-label-board-system/design.md`

---

## Task 1: 新建 API Clients

**Files:**
- Create: `front/app/api/semanticBoards.ts`
- Create: `front/app/api/auxiliaryLabels.ts`

**Step 1: 创建 semanticBoards API client**

```typescript
import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface SemanticBoard {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  tag_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
  created_at: string
  updated_at: string
}

export interface AuxiliaryLabelItem {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
}

export interface BoardCompositionResponse {
  items: AuxiliaryLabelItem[]
  total: number
}

export interface UpgradeCandidate {
  id: number
  label: string
  slug: string
  ref_count: number
}

export interface UpgradeCluster {
  candidates: UpgradeCandidate[]
  existing_board_id: number | null
  existing_board_label: string
  existing_board_description: string
  existing_board_auxiliary_labels: number[]
}

export interface UpgradeConfig {
  semantic_board_upgrade_ref_count_threshold: number
  semantic_board_upgrade_cluster_distance_threshold: number
  semantic_board_upgrade_cotag_window_days: number
  semantic_board_upgrade_cotag_top_n: number
  semantic_board_upgrade_cotag_dedupe_sim_threshold: number
  semantic_board_upgrade_cotag_hard_limit: number
}

export interface UpgradeCandidatesResponse {
  candidates: UpgradeCandidate[]
  clusters: UpgradeCluster[]
  config: UpgradeConfig
}

export interface UpgradeSuggestion {
  decision: 'create_new' | 'merge_into_existing' | 'skip'
  board_label?: string
  description?: string
  target_board_id?: number
  auxiliary_label_ids: number[]
  reason: string
}

export interface UpgradeSuggestResponse {
  suggestions: UpgradeSuggestion[]
}

export interface BackfillTask {
  id: string
  mode: string
  board_id?: number
  total: number
  processed: number
  failed: number
  status: 'pending' | 'running' | 'completed' | 'failed'
  failures: string[]
  created_at: string
}

export interface MatchingConfig {
  semantic_board_match_sim_threshold: number
  semantic_board_match_direct_hit_rate: number
  semantic_board_match_direct_max_sim: number
  semantic_board_match_weight_sim: number
  semantic_board_match_weight_density: number
  semantic_board_match_weighted_threshold: number
  semantic_board_match_max_boards: number
}

export function useSemanticBoardsApi() {
  async function getBoards(params?: { search?: string; status?: string }): Promise<ApiResponse<{ items: SemanticBoard[]; total: number }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards${query ? `?${query}` : ''}`)
  }

  async function getBoard(id: number): Promise<ApiResponse<SemanticBoard>> {
    return apiClient.get(`/semantic-boards/${id}`)
  }

  async function createBoard(data: {
    label: string
    description?: string
    display_order?: number
    protected?: boolean
    auxiliary_labels?: number[]
  }): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post('/semantic-boards', data)
  }

  async function updateBoard(id: number, data: {
    label?: string
    description?: string
    display_order?: number
    protected?: boolean
    status?: string
  }): Promise<ApiResponse<{ id: number }>> {
    return apiClient.put(`/semantic-boards/${id}`, data)
  }

  async function deleteBoard(id: number): Promise<ApiResponse<{ id: number }>> {
    return apiClient.delete(`/semantic-boards/${id}`)
  }

  async function getComposition(id: number): Promise<ApiResponse<BoardCompositionResponse>> {
    return apiClient.get(`/semantic-boards/${id}/composition`)
  }

  async function removeFromComposition(boardId: number, auxiliaryLabelId: number): Promise<ApiResponse<{ board_id: number; auxiliary_label_id: number }>> {
    return apiClient.delete(`/semantic-boards/${boardId}/composition/${auxiliaryLabelId}`)
  }

  async function getUpgradeCandidates(): Promise<ApiResponse<UpgradeCandidatesResponse>> {
    return apiClient.get('/semantic-boards/upgrade-candidates')
  }

  async function suggestUpgrade(): Promise<ApiResponse<UpgradeSuggestResponse>> {
    return apiClient.post('/semantic-boards/upgrade-suggest')
  }

  async function executeUpgrade(data: {
    decision: 'create_new' | 'merge_into_existing'
    board_label?: string
    description?: string
    target_board_id?: number
    auxiliary_label_ids: number[]
  }): Promise<ApiResponse<{ semantic_board_id: number; auxiliary_label_ids: number[] }>> {
    return apiClient.post('/semantic-boards/upgrade-execute', data)
  }

  async function triggerBackfill(data: { mode: string; board_id?: number }): Promise<ApiResponse<BackfillTask>> {
    return apiClient.post('/semantic-boards/backfill', data)
  }

  async function getBackfillStatus(id: string): Promise<ApiResponse<BackfillTask>> {
    return apiClient.get(`/semantic-boards/backfill/${id}`)
  }

  async function getMatchingConfig(): Promise<ApiResponse<MatchingConfig>> {
    return apiClient.get('/semantic-boards/matching-config')
  }

  async function updateMatchingConfig(data: Partial<MatchingConfig>): Promise<ApiResponse<MatchingConfig>> {
    return apiClient.put('/semantic-boards/matching-config', data)
  }

  return {
    getBoards,
    getBoard,
    createBoard,
    updateBoard,
    deleteBoard,
    getComposition,
    removeFromComposition,
    getUpgradeCandidates,
    suggestUpgrade,
    executeUpgrade,
    triggerBackfill,
    getBackfillStatus,
    getMatchingConfig,
    updateMatchingConfig,
  }
}
```

**Step 2: 创建 auxiliaryLabels API client**

```typescript
import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface AuxiliaryLabel {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
}

export function useAuxiliaryLabelsApi() {
  async function getLabels(params?: { search?: string; status?: string }): Promise<ApiResponse<{ items: AuxiliaryLabel[]; total: number }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/auxiliary-labels${query ? `?${query}` : ''}`)
  }

  async function disableLabel(id: number): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post(`/auxiliary-labels/${id}/disable`)
  }

  async function mergeAlias(sourceId: number, targetId: number): Promise<ApiResponse<{ source_id: number; target_id: number }>> {
    return apiClient.post('/auxiliary-labels/merge-alias', { source_id: sourceId, target_id: targetId })
  }

  return {
    getLabels,
    disableLabel,
    mergeAlias,
  }
}
```

**Step 3: 运行 typecheck 验证新文件无类型错误**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: PASS（新文件无错误，旧文件可能有已知错误）

**Step 4: Commit**

```bash
git add front/app/api/semanticBoards.ts front/app/api/auxiliaryLabels.ts
git commit -m "feat(frontend): add semantic boards and auxiliary labels API clients"
```

---

## Task 2: 新建 SemanticBoardList 组件

**Files:**
- Create: `front/app/features/tags/components/SemanticBoardList.vue`
- Reference: `front/app/features/tags/components/SectorList.vue`（复制风格）

**Step 1: 编写 SemanticBoardList 组件**

参照 SectorList 的暗色风格（圆角 item、hover 背景、active 橙色边框、badge、删除按钮）。
Props: `boards`, `selectedId`, `loading`, `searchQuery`。
Emits: `select`, `add`, `upgrade`, `backfill`, `config`, `delete`。

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { SemanticBoard } from '~/api/semanticBoards'

defineProps<{
  boards: SemanticBoard[]
  selectedId: number | null
  loading: boolean
  searchQuery: string
}>()

const emit = defineEmits<{
  select: [id: number | null]
  add: []
  upgrade: []
  backfill: []
  config: []
  delete: [id: number]
}>()

function sourceIcon(source: string): string {
  switch (source) {
    case 'manual': return 'mdi:lock'
    case 'llm_extract': return 'mdi:robot'
    default: return 'mdi:lightning-bolt'
  }
}

function sourceTitle(source: string): string {
  switch (source) {
    case 'manual': return '手动创建'
    case 'llm_extract': return 'LLM 生成'
    default: return '自动生成'
  }
}
</script>

<template>
  <div class="sb-list">
    <div class="sb-list-header">
      <span class="sb-list-title">语义板块</span>
      <span class="sb-list-count">{{ boards.length }}</span>
    </div>

    <div
      class="sb-item"
      :class="{ 'sb-item--active': selectedId === null }"
      @click="emit('select', null)"
    >
      <Icon icon="mdi:view-grid" width="14" class="sb-item-icon" />
      <span class="sb-item-label">全部</span>
      <span class="sb-item-badge">{{ boards.reduce((s, x) => s + x.tag_count, 0) }}</span>
    </div>

    <div v-if="loading" class="sb-loading">
      <div v-for="i in 3" :key="i" class="sb-skeleton" />
    </div>

    <div v-else-if="boards.length === 0" class="sb-empty">
      <Icon icon="mdi:folder-outline" width="24" class="text-white/15" />
      <p>暂无板块</p>
    </div>

    <div v-else class="sb-items">
      <div
        v-for="board in boards"
        :key="board.id"
        class="sb-item"
        :class="{
          'sb-item--active': selectedId === board.id,
          'sb-item--protected': board.protected,
        }"
        @click="emit('select', board.id)"
      >
        <Icon
          :icon="sourceIcon(board.source)"
          width="13"
          class="sb-source-icon"
          :title="sourceTitle(board.source)"
        />
        <span class="sb-item-label">{{ board.label }}</span>
        <span v-if="board.tag_count > 0" class="sb-item-badge">{{ board.tag_count }}</span>
        <button
          type="button"
          class="sb-delete-btn"
          title="删除板块"
          @click.stop="emit('delete', board.id)"
        >
          <Icon icon="mdi:close" width="12" />
        </button>
      </div>
    </div>

    <div class="sb-actions">
      <button type="button" class="sb-action-btn sb-action-btn--primary" @click="emit('add')">
        <Icon icon="mdi:plus" width="14" />
        添加板块
      </button>
      <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="emit('upgrade')">
        <Icon icon="mdi:auto-fix" width="14" />
        升级建议
      </button>
      <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="emit('backfill')">
        <Icon icon="mdi:backup-restore" width="14" />
        匹配回填
      </button>
      <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="emit('config')">
        <Icon icon="mdi:tune" width="14" />
        匹配参数
      </button>
    </div>
  </div>
</template>

<style scoped>
/* 完全参照 SectorList.vue 的样式，类名改为 sb- 前缀 */
.sb-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sb-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 0.25rem;
  margin-bottom: 0.25rem;
}

.sb-list-title {
  font-size: 0.7rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
}

.sb-list-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.sb-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.6rem;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.12s ease;
  position: relative;
}

.sb-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.sb-item--active {
  background: rgba(240, 138, 75, 0.1);
  border: 1px solid rgba(240, 138, 75, 0.2);
}

.sb-item--protected .sb-source-icon {
  color: rgba(240, 138, 75, 0.6);
}

.sb-item-icon {
  color: rgba(255, 255, 255, 0.35);
  flex-shrink: 0;
}

.sb-source-icon {
  color: rgba(255, 255, 255, 0.3);
  flex-shrink: 0;
}

.sb-item-label {
  flex: 1;
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.75);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sb-item--active .sb-item-label {
  color: rgba(255, 220, 200, 0.9);
}

.sb-item-badge {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.sb-delete-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.15);
  cursor: pointer;
  opacity: 0;
  transition: all 0.12s ease;
  flex-shrink: 0;
}

.sb-item:hover .sb-delete-btn {
  opacity: 1;
}

.sb-delete-btn:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}

.sb-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sb-skeleton {
  height: 32px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: sbPulse 1.5s ease-in-out infinite;
}

@keyframes sbPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.sb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.75rem;
}

.sb-items {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.sb-actions {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.sb-action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  width: 100%;
  padding: 0.5rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.sb-action-btn--primary {
  color: rgba(255, 220, 200, 0.7);
  border-color: rgba(240, 138, 75, 0.2);
}

.sb-action-btn--primary:hover {
  background: rgba(240, 138, 75, 0.1);
  border-color: rgba(240, 138, 75, 0.35);
  color: rgba(255, 220, 200, 0.9);
}

.sb-action-btn--secondary {
  color: rgba(147, 197, 253, 0.6);
  border-color: rgba(99, 179, 237, 0.2);
}

.sb-action-btn--secondary:hover {
  background: rgba(99, 179, 237, 0.08);
  border-color: rgba(99, 179, 237, 0.35);
  color: rgba(147, 197, 253, 0.9);
}

.sb-action-btn--ghost {
  color: rgba(255, 255, 255, 0.4);
  border-color: rgba(255, 255, 255, 0.06);
}

.sb-action-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.7);
}
</style>
```

**Step 2: 运行 lint + typecheck**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

**Step 3: Commit**

```bash
git add front/app/features/tags/components/SemanticBoardList.vue
git commit -m "feat(frontend): add SemanticBoardList component"
```

---

## Task 3: 新建 AddSemanticBoardDialog 组件

**Files:**
- Create: `front/app/features/tags/components/AddSemanticBoardDialog.vue`
- Reference: `front/app/features/tags/components/AddSectorDialog.vue`

**Step 1: 编写组件**

基于 AddSectorDialog 改造，增加 `description`, `display_order`, `protected` 字段，以及辅助标签多选（可选，后续 Task 4 再完善）。

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'

const props = defineProps<{
  visible: boolean
  editMode?: boolean
  initialData?: {
    label: string
    description: string
    display_order: number
    protected: boolean
  }
}>()

const emit = defineEmits<{
  confirm: [data: { label: string; description: string; display_order: number; protected: boolean }]
  cancel: []
}>()

const label = ref('')
const description = ref('')
const displayOrder = ref(0)
const isProtected = ref(false)

watch(() => props.visible, (v) => {
  if (v) {
    label.value = props.initialData?.label ?? ''
    description.value = props.initialData?.description ?? ''
    displayOrder.value = props.initialData?.display_order ?? 0
    isProtected.value = props.initialData?.protected ?? false
  }
})

function handleSubmit() {
  const trimmed = label.value.trim()
  if (!trimmed) return
  emit('confirm', {
    label: trimmed,
    description: description.value.trim(),
    display_order: displayOrder.value,
    protected: isProtected.value,
  })
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="dialog-overlay" @click.self="emit('cancel')" @keydown.escape="emit('cancel')">
      <div class="dialog-card">
        <div class="dialog-header">
          <h3 class="dialog-title">{{ editMode ? '编辑板块' : '添加板块' }}</h3>
          <button type="button" class="dialog-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div class="dialog-body">
          <label class="dialog-field">
            <span class="dialog-label">名称 <span class="dialog-required">*</span></span>
            <input v-model="label" type="text" class="dialog-input" placeholder="板块名称" maxlength="100" autofocus @keyup.enter="handleSubmit" />
          </label>
          <label class="dialog-field">
            <span class="dialog-label">描述</span>
            <input v-model="description" type="text" class="dialog-input" placeholder="可选描述" maxlength="500" @keyup.enter="handleSubmit" />
          </label>
          <label class="dialog-field">
            <span class="dialog-label">排序</span>
            <input v-model.number="displayOrder" type="number" class="dialog-input" placeholder="0" />
          </label>
          <label class="dialog-field dialog-field--row">
            <input v-model="isProtected" type="checkbox" class="dialog-checkbox" />
            <span class="dialog-label">受保护（禁止自动删除）</span>
          </label>
        </div>

        <div class="dialog-footer">
          <button type="button" class="dialog-btn dialog-btn--ghost" @click="emit('cancel')">取消</button>
          <button type="button" class="dialog-btn dialog-btn--primary" :disabled="!label.trim()" @click="handleSubmit">确认</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* 完全复制 AddSectorDialog.vue 的样式，增加 .dialog-field--row 和 .dialog-checkbox */
.dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
}

.dialog-card {
  width: min(420px, 90%);
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.25rem;
}

.dialog-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.dialog-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.dialog-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.dialog-body {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.dialog-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.dialog-field--row {
  flex-direction: row;
  align-items: center;
  gap: 0.5rem;
}

.dialog-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: 0.02em;
}

.dialog-required {
  color: rgba(240, 138, 75, 0.8);
}

.dialog-input {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.25);
  color: rgba(255, 255, 255, 0.88);
  font-size: 0.82rem;
  padding: 0.55rem 0.85rem;
  outline: none;
  transition: border-color 0.12s ease;
  box-sizing: border-box;
}

.dialog-input::placeholder {
  color: rgba(255, 255, 255, 0.2);
}

.dialog-input:focus {
  border-color: rgba(240, 138, 75, 0.45);
}

.dialog-checkbox {
  width: 16px;
  height: 16px;
  accent-color: rgba(240, 138, 75, 0.8);
}

.dialog-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1.25rem;
}

.dialog-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.45rem 1.1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.dialog-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.06);
}

.dialog-btn--primary {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.12);
}

.dialog-btn--primary:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.6);
}

.dialog-btn--primary:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
```

**Step 2: lint + typecheck + commit**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

```bash
git add front/app/features/tags/components/AddSemanticBoardDialog.vue
git commit -m "feat(frontend): add AddSemanticBoardDialog component"
```

---

## Task 4: 新建 BoardCompositionPanel 组件

**Files:**
- Create: `front/app/features/tags/components/BoardCompositionPanel.vue`

**Step 1: 编写组件**

展示 board 的 composition 辅助标签，以 chip 形式排列，每个 chip 可点击移除。
Props: `boardId`, `labels` (AuxiliaryLabelItem[]), `loading`。
Emits: `remove`, `refresh`。

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { AuxiliaryLabelItem } from '~/api/semanticBoards'

const props = defineProps<{
  boardId: number
  labels: AuxiliaryLabelItem[]
  loading: boolean
}>()

const emit = defineEmits<{
  remove: [auxiliaryLabelId: number]
  refresh: []
}>()

function handleRemove(id: number, label: string) {
  if (!confirm(`从板块中移除辅助标签 "${label}"？\n注意：不会自动回填历史数据。`)) return
  emit('remove', id)
}
</script>

<template>
  <div class="bcp-panel">
    <div class="bcp-header">
      <Icon icon="mdi:puzzle-outline" width="15" class="text-white/50" />
      <span class="bcp-title">构成标签</span>
      <span class="bcp-count">{{ labels.length }}</span>
    </div>

    <div v-if="loading" class="bcp-loading">
      <div v-for="i in 3" :key="i" class="bcp-skeleton-chip" />
    </div>

    <div v-else-if="labels.length === 0" class="bcp-empty">
      <Icon icon="mdi:tag-off-outline" width="20" class="text-white/15" />
      <span>暂无构成标签</span>
    </div>

    <div v-else class="bcp-chips">
      <div
        v-for="label in labels"
        :key="label.id"
        class="bcp-chip"
        :class="{ 'bcp-chip--disabled': label.status === 'disabled' }"
      >
        <span class="bcp-chip-label">{{ label.label }}</span>
        <span v-if="label.ref_count > 0" class="bcp-chip-ref">{{ label.ref_count }}</span>
        <button
          type="button"
          class="bcp-chip-remove"
          title="移除"
          @click="handleRemove(label.id, label.label)"
        >
          <Icon icon="mdi:close" width="10" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bcp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

.bcp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bcp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.bcp-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.bcp-loading {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.bcp-skeleton-chip {
  width: 60px;
  height: 26px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
  animation: bcpPulse 1.5s ease-in-out infinite;
}

@keyframes bcpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.bcp-empty {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 1rem 0;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.75rem;
}

.bcp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.bcp-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.05);
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.75);
  transition: all 0.12s ease;
}

.bcp-chip:hover {
  background: rgba(255, 255, 255, 0.08);
}

.bcp-chip--disabled {
  opacity: 0.5;
  border-style: dashed;
}

.bcp-chip-ref {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0 0.25rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
}

.bcp-chip-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 4px;
  background: none;
  color: rgba(255, 255, 255, 0.2);
  cursor: pointer;
  opacity: 0;
  transition: all 0.12s ease;
}

.bcp-chip:hover .bcp-chip-remove {
  opacity: 1;
}

.bcp-chip-remove:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}
</style>
```

**Step 2: lint + typecheck + commit**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

```bash
git add front/app/features/tags/components/BoardCompositionPanel.vue
git commit -m "feat(frontend): add BoardCompositionPanel component"
```

---

## Task 5: 新建 AuxiliaryLabelPool 组件

**Files:**
- Create: `front/app/features/tags/components/AuxiliaryLabelPool.vue`

**Step 1: 编写组件**

展示辅助标签池，支持搜索、按状态筛选、禁用、合并 alias。
Props: `labels`, `loading`, `searchQuery`, `statusFilter`。
Emits: `update:searchQuery`, `update:statusFilter`, `disable`, `merge`, `refresh`。

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { AuxiliaryLabel } from '~/api/auxiliaryLabels'

const props = defineProps<{
  labels: AuxiliaryLabel[]
  loading: boolean
  searchQuery: string
  statusFilter: string
}>()

const emit = defineEmits<{
  'update:searchQuery': [v: string]
  'update:statusFilter': [v: string]
  disable: [id: number, label: string]
  merge: [sourceId: number, targetId: number]
  refresh: []
}>()

const mergeSourceId = ref<number | null>(null)

function handleDisable(id: number, label: string) {
  if (!confirm(`禁用辅助标签 "${label}"？\n禁用后不再参与 board 匹配和升级候选。`)) return
  emit('disable', id, label)
}

function startMerge(sourceId: number) {
  mergeSourceId.value = sourceId
}

function cancelMerge() {
  mergeSourceId.value = null
}

function confirmMerge(targetId: number, targetLabel: string) {
  if (mergeSourceId.value === null) return
  if (!confirm(`将标签合并为 "${targetLabel}" 的 alias？`)) return
  emit('merge', mergeSourceId.value, targetId)
  mergeSourceId.value = null
}
</script>

<template>
  <div class="alp-panel">
    <div class="alp-header">
      <Icon icon="mdi:tag-multiple-outline" width="15" class="text-white/50" />
      <span class="alp-title">辅助标签池</span>
      <span class="alp-count">{{ labels.length }}</span>
    </div>

    <div class="alp-filters">
      <input
        :value="searchQuery"
        type="text"
        class="alp-search"
        placeholder="搜索标签..."
        @input="emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
      />
      <select
        :value="statusFilter"
        class="alp-select"
        @change="emit('update:statusFilter', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部状态</option>
        <option value="active">活跃</option>
        <option value="disabled">已禁用</option>
      </select>
    </div>

    <div v-if="loading" class="alp-loading">
      <div v-for="i in 4" :key="i" class="alp-skeleton-row" />
    </div>

    <div v-else-if="labels.length === 0" class="alp-empty">
      <Icon icon="mdi:tag-off-outline" width="24" class="text-white/15" />
      <span>暂无辅助标签</span>
    </div>

    <div v-else class="alp-list">
      <div
        v-for="label in labels"
        :key="label.id"
        class="alp-row"
        :class="{ 'alp-row--disabled': label.status === 'disabled', 'alp-row--merge-target': mergeSourceId !== null && mergeSourceId !== label.id }"
      >
        <div class="alp-row-main">
          <span class="alp-row-label">{{ label.label }}</span>
          <span v-if="label.aliases.length" class="alp-row-aliases">aka {{ label.aliases.join(', ') }}</span>
          <span class="alp-row-ref">{{ label.ref_count }} 引用</span>
        </div>
        <div class="alp-row-actions">
          <template v-if="mergeSourceId === null">
            <button
              v-if="label.status === 'active'"
              type="button"
              class="alp-row-btn"
              title="禁用"
              @click="handleDisable(label.id, label.label)"
            >
              <Icon icon="mdi:eye-off-outline" width="12" />
            </button>
            <button
              type="button"
              class="alp-row-btn"
              title="合并为其他标签的 alias"
              @click="startMerge(label.id)"
            >
              <Icon icon="mdi:merge" width="12" />
            </button>
          </template>
          <template v-else-if="mergeSourceId !== label.id">
            <button
              type="button"
              class="alp-row-btn alp-row-btn--primary"
              @click="confirmMerge(label.id, label.label)"
            >
              合并到此
            </button>
          </template>
          <span v-else class="alp-row-self">源标签</span>
        </div>
      </div>
      <div v-if="mergeSourceId !== null" class="alp-merge-hint">
        <span>选择目标标签进行合并</span>
        <button type="button" class="alp-row-btn" @click="cancelMerge">取消</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.alp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.alp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.alp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.alp-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.alp-filters {
  display: flex;
  gap: 0.5rem;
}

.alp-search {
  flex: 1;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.2);
  color: rgba(255, 255, 255, 0.8);
  font-size: 0.75rem;
  padding: 0.4rem 0.7rem;
  outline: none;
}

.alp-search::placeholder {
  color: rgba(255, 255, 255, 0.25);
}

.alp-search:focus {
  border-color: rgba(240, 138, 75, 0.4);
}

.alp-select {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.2);
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.75rem;
  padding: 0.4rem 0.6rem;
  outline: none;
}

.alp-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.alp-skeleton-row {
  height: 36px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: alpPulse 1.5s ease-in-out infinite;
}

@keyframes alpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.alp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.75rem;
}

.alp-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.alp-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.5rem 0.6rem;
  border-radius: 10px;
  transition: all 0.12s ease;
}

.alp-row:hover {
  background: rgba(255, 255, 255, 0.04);
}

.alp-row--disabled {
  opacity: 0.5;
}

.alp-row--merge-target {
  cursor: pointer;
}

.alp-row-main {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 0;
  flex-wrap: wrap;
}

.alp-row-label {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.8);
}

.alp-row-aliases {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
}

.alp-row-ref {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.35rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.alp-row-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  flex-shrink: 0;
}

.alp-row-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.2rem;
  padding: 0.25rem 0.4rem;
  border-radius: 6px;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  font-size: 0.65rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.alp-row-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.alp-row-btn--primary {
  color: rgba(240, 138, 75, 0.8);
}

.alp-row-btn--primary:hover {
  background: rgba(240, 138, 75, 0.12);
}

.alp-row-self {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.25);
  padding: 0.2rem 0.4rem;
}

.alp-merge-hint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.6rem;
  border-radius: 10px;
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
  font-size: 0.72rem;
  color: rgba(255, 220, 200, 0.8);
}
</style>
```

**Step 2: lint + typecheck + commit**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

```bash
git add front/app/features/tags/components/AuxiliaryLabelPool.vue
git commit -m "feat(frontend): add AuxiliaryLabelPool component"
```

---

## Task 6: 新建 UpgradeSuggestionPanel 组件

**Files:**
- Create: `front/app/features/tags/components/UpgradeSuggestionPanel.vue`
- Reference: `front/app/features/tags/components/SectorApprovalPanel.vue`

**Step 1: 编写组件**

Dialog 形式，展示升级候选簇和 LLM 建议，支持确认/拒绝。
Props: `visible`, `candidates`, `clusters`, `suggestions`, `loading`。
Emits: `suggest`, `execute`, `cancel`。

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { UpgradeCandidate, UpgradeCluster, UpgradeSuggestion } from '~/api/semanticBoards'

const props = defineProps<{
  visible: boolean
  candidates: UpgradeCandidate[]
  clusters: UpgradeCluster[]
  suggestions: UpgradeSuggestion[]
  loading: boolean
  suggesting: boolean
}>()

const emit = defineEmits<{
  suggest: []
  execute: [suggestion: UpgradeSuggestion]
  cancel: []
}>()

function decisionLabel(d: string): string {
  switch (d) {
    case 'create_new': return '创建新板块'
    case 'merge_into_existing': return '合并到已有板块'
    case 'skip': return '跳过'
    default: return d
  }
}

function decisionStyle(d: string): { border: string; bg: string; color: string } {
  switch (d) {
    case 'create_new': return { border: 'rgba(52,211,153,0.3)', bg: 'rgba(52,211,153,0.08)', color: 'rgba(134,239,172,0.9)' }
    case 'merge_into_existing': return { border: 'rgba(96,165,250,0.3)', bg: 'rgba(96,165,250,0.08)', color: 'rgba(147,197,253,0.9)' }
    case 'skip': return { border: 'rgba(107,114,128,0.3)', bg: 'rgba(107,114,128,0.08)', color: 'rgba(209,213,219,0.7)' }
    default: return { border: 'rgba(255,255,255,0.1)', bg: 'rgba(255,255,255,0.04)', color: 'rgba(255,255,255,0.6)' }
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
            <p class="usp-subtitle">
              候选标签 {{ candidates.length }} 个 · 聚类 {{ clusters.length }} 个
            </p>
          </div>
          <button type="button" class="usp-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div v-if="loading" class="usp-loading">
          <Icon icon="mdi:loading" width="20" class="animate-spin text-white/30" />
          <span>加载候选...</span>
        </div>

        <div v-else-if="suggestions.length === 0" class="usp-empty">
          <p v-if="candidates.length === 0">暂无满足条件的升级候选</p>
          <button
            v-else
            type="button"
            class="usp-suggest-btn"
            :disabled="suggesting"
            @click="emit('suggest')"
          >
            <Icon v-if="suggesting" icon="mdi:loading" width="14" class="animate-spin" />
            <Icon v-else icon="mdi:brain" width="14" />
            {{ suggesting ? 'LLM 分析中...' : '获取 LLM 建议' }}
          </button>
        </div>

        <div v-else class="usp-list">
          <div
            v-for="(s, i) in suggestions"
            :key="i"
            class="usp-item"
            :style="{ borderColor: decisionStyle(s.decision).border, background: decisionStyle(s.decision).bg }"
          >
            <div class="usp-item-header">
              <span class="usp-item-decision" :style="{ color: decisionStyle(s.decision).color }">
                {{ decisionLabel(s.decision) }}
              </span>
              <span v-if="s.board_label" class="usp-item-board">{{ s.board_label }}</span>
              <span v-else-if="s.target_board_id" class="usp-item-board">板块 #{{ s.target_board_id }}</span>
            </div>
            <p v-if="s.description" class="usp-item-desc">{{ s.description }}</p>
            <p class="usp-item-reason">{{ s.reason }}</p>
            <div class="usp-item-tags">
              <span v-for="id in s.auxiliary_label_ids" :key="id" class="usp-item-tag">标签 #{{ id }}</span>
            </div>
            <div v-if="s.decision !== 'skip'" class="usp-item-actions">
              <button
                type="button"
                class="usp-item-btn usp-item-btn--primary"
                @click="emit('execute', s)"
              >
                <Icon icon="mdi:check" width="12" />
                确认执行
              </button>
            </div>
          </div>
        </div>
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
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
  padding: 1rem;
}

.usp-card {
  width: min(560px, 95vw);
  max-height: 80vh;
  overflow-y: auto;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
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
  color: rgba(255, 255, 255, 0.9);
}

.usp-subtitle {
  margin-top: 0.25rem;
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.4);
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
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.usp-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.8rem;
}

.usp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.8rem;
}

.usp-suggest-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 1rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.3);
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 220, 200, 0.9);
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-suggest-btn:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.18);
}

.usp-suggest-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.usp-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
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
  background: rgba(0, 0, 0, 0.2);
}

.usp-item-board {
  font-size: 0.8rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.85);
}

.usp-item-desc {
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.55);
  line-height: 1.5;
}

.usp-item-reason {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.5;
}

.usp-item-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
}

.usp-item-tag {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.45);
  padding: 0.1rem 0.35rem;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.06);
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
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.usp-item-btn--primary {
  border-color: rgba(52, 211, 153, 0.3);
  background: rgba(52, 211, 153, 0.1);
  color: rgba(134, 239, 172, 0.9);
}

.usp-item-btn--primary:hover {
  background: rgba(52, 211, 153, 0.18);
}
</style>
```

**Step 2: lint + typecheck + commit**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

```bash
git add front/app/features/tags/components/UpgradeSuggestionPanel.vue
git commit -m "feat(frontend): add UpgradeSuggestionPanel component"
```

---

## Task 7: 新建 BackfillProgress 和 MatchingConfigDialog 组件

**Files:**
- Create: `front/app/features/tags/components/BackfillProgress.vue`
- Create: `front/app/features/tags/components/MatchingConfigDialog.vue`

**Step 1: 编写 BackfillProgress**

底部进度条组件，展示回填任务状态。
Props: `task` (BackfillTask | null)。

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { BackfillTask } from '~/api/semanticBoards'

const props = defineProps<{
  task: BackfillTask | null
}>()

const percent = computed(() => {
  if (!props.task || props.task.total === 0) return 0
  return Math.round((props.task.processed / props.task.total) * 100)
})

const isRunning = computed(() => props.task?.status === 'pending' || props.task?.status === 'running')
</script>

<template>
  <div v-if="task" class="bf-progress">
    <div class="bf-bar-track">
      <div class="bf-bar-fill" :style="{ width: `${percent}%` }" />
    </div>
    <div class="bf-info">
      <Icon v-if="isRunning" icon="mdi:loading" width="13" class="animate-spin text-blue-400/60" />
      <Icon v-else-if="task.status === 'completed'" icon="mdi:check-circle-outline" width="13" class="text-green-400/70" />
      <Icon v-else icon="mdi:alert-circle-outline" width="13" class="text-red-400/70" />
      <span>回填: {{ task.processed }}/{{ task.total }}</span>
      <span v-if="task.failed > 0" class="bf-failed">失败 {{ task.failed }}</span>
    </div>
  </div>
</template>

<style scoped>
.bf-progress {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  max-width: 400px;
}

.bf-bar-track {
  flex: 1;
  height: 3px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.bf-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(99, 179, 237, 0.7), rgba(147, 197, 253, 0.9));
  transition: width 0.3s ease;
}

.bf-info {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.45);
  white-space: nowrap;
}

.bf-failed {
  color: rgba(252, 165, 165, 0.8);
}
</style>
```

**Step 2: 编写 MatchingConfigDialog**

表单 Dialog，展示和编辑匹配参数。
Props: `visible`, `config` (MatchingConfig | null), `loading`。
Emits: `save`, `cancel`。

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { MatchingConfig } from '~/api/semanticBoards'

const props = defineProps<{
  visible: boolean
  config: MatchingConfig | null
  loading: boolean
}>()

const emit = defineEmits<{
  save: [data: Partial<MatchingConfig>]
  cancel: []
}>()

const form = ref<Partial<MatchingConfig>>({})

watch(() => props.visible, (v) => {
  if (v && props.config) {
    form.value = { ...props.config }
  }
})

function handleSave() {
  emit('save', form.value)
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="mc-overlay" @click.self="emit('cancel')">
      <div class="mc-card">
        <div class="mc-header">
          <h3 class="mc-title">匹配参数配置</h3>
          <button type="button" class="mc-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div v-if="loading" class="mc-loading">加载中...</div>

        <div v-else class="mc-body">
          <label class="mc-field">
            <span class="mc-label">相似度阈值</span>
            <input v-model.number="form.semantic_board_match_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">直接命中率阈值</span>
            <input v-model.number="form.semantic_board_match_direct_hit_rate" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">直接匹配最大相似度</span>
            <input v-model.number="form.semantic_board_match_direct_max_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">相似度权重</span>
            <input v-model.number="form.semantic_board_match_weight_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">密度权重</span>
            <input v-model.number="form.semantic_board_match_weight_density" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">加权综合阈值</span>
            <input v-model.number="form.semantic_board_match_weighted_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
          </label>
          <label class="mc-field">
            <span class="mc-label">最大归属板块数</span>
            <input v-model.number="form.semantic_board_match_max_boards" type="number" min="1" max="10" class="mc-input" />
          </label>
        </div>

        <div class="mc-footer">
          <button type="button" class="mc-btn mc-btn--ghost" @click="emit('cancel')">取消</button>
          <button type="button" class="mc-btn mc-btn--primary" @click="handleSave">保存</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* 复用 AddSemanticBoardDialog 的 dialog 样式，类名改为 mc- */
.mc-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
}

.mc-card {
  width: min(420px, 90%);
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

.mc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.25rem;
}

.mc-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.mc-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.mc-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.mc-loading {
  text-align: center;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.8rem;
}

.mc-body {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.mc-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.mc-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: 0.02em;
}

.mc-input {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.25);
  color: rgba(255, 255, 255, 0.88);
  font-size: 0.82rem;
  padding: 0.55rem 0.85rem;
  outline: none;
  transition: border-color 0.12s ease;
  box-sizing: border-box;
}

.mc-input:focus {
  border-color: rgba(240, 138, 75, 0.45);
}

.mc-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1.25rem;
}

.mc-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.45rem 1.1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.mc-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.06);
}

.mc-btn--primary {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.12);
}

.mc-btn--primary:hover {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.6);
}
</style>
```

**Step 3: lint + typecheck + commit**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

```bash
git add front/app/features/tags/components/BackfillProgress.vue front/app/features/tags/components/MatchingConfigDialog.vue
git commit -m "feat(frontend): add BackfillProgress and MatchingConfigDialog components"
```

---

## Task 8: 改造 TagsPage.vue

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`
- Delete references: SectorList, AddSectorDialog, SectorApprovalPanel, TemplateSettingsDialog, PendingChangePanel, TagHierarchy

**Step 1: 替换 imports 和 API**

移除 `useBoardConceptsApi`, `useHierarchyConfigApi`, `useWebSocketRebuild` 及所有旧组件 import。
新增 `useSemanticBoardsApi`, `useAuxiliaryLabelsApi` 及新组件 import。

**Step 2: 重写 state 和逻辑**

保留：顶部栏布局、左侧 sidebar 布局、文章时间线展示、底部栏布局。
替换：
- `sectors` → `boards` (SemanticBoard[])
- `selectedSectorId` → `selectedBoardId`
- `selectedCategory` → 删除（新体系无 category 区分）
- `categories` tabs → 删除
- `closureStatus` / `pendingCount` / `rebuildStatus` 等层级相关 state → 删除
- 新增：`compositionLabels`, `auxiliaryLabels`, `upgradeCandidates`, `upgradeClusters`, `upgradeSuggestions`, `backfillTask`, `matchingConfig`, 各 dialog 显隐 state

**Step 3: 重写 load 函数**

```typescript
async function loadBoards() {
  boardsLoading.value = true
  const res = await sbApi.getBoards({ search: searchQuery.value || undefined, status: statusFilter.value || undefined })
  if (res.success && res.data) {
    boards.value = res.data.items
  } else {
    boardsError.value = res.error || '加载失败'
  }
  boardsLoading.value = false
}

async function loadComposition(boardId: number) {
  compositionLoading.value = true
  const res = await sbApi.getComposition(boardId)
  if (res.success && res.data) {
    compositionLabels.value = res.data.items
  }
  compositionLoading.value = false
}

async function loadAuxiliaryLabels() {
  auxLoading.value = true
  const res = await auxApi.getLabels({ search: auxSearchQuery.value || undefined, status: auxStatusFilter.value || undefined })
  if (res.success && res.data) {
    auxiliaryLabels.value = res.data.items
  }
  auxLoading.value = false
}
```

**Step 4: 重写事件处理**

- `handleSelectBoard` → 加载 composition 和时间线
- `handleAddBoard` → 调用 createBoard，刷新列表
- `handleDeleteBoard` → 调用 deleteBoard（软删除），刷新列表
- `handleRemoveComposition` → 调用 removeFromComposition，刷新 composition
- `handleUpgradeSuggest` → 先 getUpgradeCandidates，再 suggestUpgrade
- `handleExecuteUpgrade` → 调用 executeUpgrade，刷新列表
- `handleTriggerBackfill` → 调用 triggerBackfill，启动轮询
- `handleSaveMatchingConfig` → 调用 updateMatchingConfig

**Step 5: 重写 template**

- 顶部栏：删除 category tabs，保留返回按钮和标题（改为"语义板块管理"）
- 左侧 sidebar：`<SemanticBoardList />` 替代 `<SectorList />`
- 右侧 content：
  - 选中 board：`<BoardCompositionPanel />` + 文章时间线
  - 未选中：`<AuxiliaryLabelPool />`
- 底部栏：`<BackfillProgress />` 替代 rebuild progress
- Dialogs：`<AddSemanticBoardDialog />`, `<UpgradeSuggestionPanel />`, `<MatchingConfigDialog />`

**Step 6: lint + typecheck + build**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`
Expected: PASS

**Step 7: Commit**

```bash
git add front/app/features/tags/components/TagsPage.vue
git commit -m "feat(frontend): refactor TagsPage to semantic board management"
```

---

## Task 9: 改造 NarrativePanel / NarrativeDetailCard

**Files:**
- Modify: `front/app/features/topic-graph/components/NarrativePanel.vue`
- Modify: `front/app/features/topic-graph/components/NarrativeDetailCard.vue`

**Step 1: NarrativePanel 移除 abstract_tag 逻辑**

- 删除 `abstractTagIds` computed 及相关逻辑
- 删除 `abstract_tags` 相关展示（boardTags 中 abstractTags 部分）
- 在 board 标签组 header 中增加 `semantic_board_label` 展示（如果 API 已返回）
- 删除 `unclassifiedTags` 中的旧概念提示

**Step 2: NarrativeDetailCard 移除 abstract 标识**

- 删除 `isAbstract` computed 和 `narrative-detail--abstract` class
- 删除 abstract 标签特殊样式（如虚线边框）
- 保留其他功能不变

**Step 3: lint + typecheck + build**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`
Expected: PASS

**Step 4: Commit**

```bash
git add front/app/features/topic-graph/components/NarrativePanel.vue front/app/features/topic-graph/components/NarrativeDetailCard.vue
git commit -m "feat(frontend): update NarrativePanel for semantic board system"
```

---

## Task 10: 清理废弃代码

**Files:**
- Delete (or unreference): `front/app/features/tags/components/SectorList.vue`
- Delete (or unreference): `front/app/features/tags/components/SectorApprovalPanel.vue`
- Delete (or unreference): `front/app/features/tags/components/AddSectorDialog.vue`
- Delete (or unreference): `front/app/features/tags/components/TemplateSettingsDialog.vue`
- Delete (or unreference): `front/app/features/tags/components/PendingChangePanel.vue`
- Delete (or unreference): `front/app/api/boardConcepts.ts`
- Modify: `front/app/features/topic-graph/components/TagHierarchy.vue`（确认在其他页面是否仍在使用，如果在 topics.vue 使用则保留，只在 TagsPage 中移除引用）

**Step 1: 确认 TagHierarchy 在其他页面的使用情况**

Run: `grep -r "TagHierarchy" front/app/pages/ front/app/features/`
If only used in TagsPage → safe to remove from TagsPage only.
If also used in topics.vue → keep file, just remove from TagsPage.

**Step 2: 删除或归档废弃文件**

推荐做法：将废弃文件移入 `front/app/_deprecated/` 目录，而非直接删除，便于后续参考。

```bash
mkdir -p front/app/_deprecated/tags
mv front/app/features/tags/components/SectorList.vue front/app/_deprecated/tags/
mv front/app/features/tags/components/SectorApprovalPanel.vue front/app/_deprecated/tags/
mv front/app/features/tags/components/AddSectorDialog.vue front/app/_deprecated/tags/
mv front/app/features/tags/components/TemplateSettingsDialog.vue front/app/_deprecated/tags/
mv front/app/features/tags/components/PendingChangePanel.vue front/app/_deprecated/tags/
mv front/app/api/boardConcepts.ts front/app/_deprecated/
```

**Step 3: 确认没有残留 import**

Run: `cd front && pnpm exec nuxi typecheck`
Expected: PASS（无未使用的 import 错误；如果有，修复它们）

**Step 4: Commit**

```bash
git add front/app/_deprecated/
git commit -m "chore(frontend): move deprecated tag hierarchy components to _deprecated"
```

---

## Task 11: 端到端验证

**Step 1: 前端构建验证**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`
Expected: PASS

**Step 2: 后端构建验证**

Run: `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...`
Expected: PASS（确保后端 API 与前端期望一致）

**Step 3: 手动功能验证**

启动前后端，访问 `/tags` 页面，验证：
1. SemanticBoard 列表加载正常
2. 添加/编辑/删除 board 正常
3. Board composition 展示和移除正常
4. 辅助标签池搜索/禁用/合并正常
5. 升级建议流程（获取候选 → LLM 建议 → 确认执行）正常
6. 回填触发和进度展示正常
7. 匹配参数保存正常
8. NarrativePanel 无 abstract tag 展示

**Step 4: Commit**

```bash
git commit --allow-empty -m "feat(frontend): complete semantic board frontend migration (closes phase 10)"
```

---

## Summary

| Task | Component / File | Verdict |
|------|------------------|---------|
| 1 | API Clients | Create `semanticBoards.ts`, `auxiliaryLabels.ts` |
| 2 | SemanticBoardList | Replace SectorList |
| 3 | AddSemanticBoardDialog | Replace AddSectorDialog |
| 4 | BoardCompositionPanel | New |
| 5 | AuxiliaryLabelPool | New |
| 6 | UpgradeSuggestionPanel | Replace SectorApprovalPanel |
| 7 | BackfillProgress + MatchingConfigDialog | New |
| 8 | TagsPage.vue | Major refactor |
| 9 | NarrativePanel / NarrativeDetailCard | Minor refactor |
| 10 | Cleanup | Move deprecated files |
| 11 | E2E verification | lint, typecheck, build, manual test |

**Plan saved to:** `docs/plans/2026-05-22-semantic-board-frontend.md`
