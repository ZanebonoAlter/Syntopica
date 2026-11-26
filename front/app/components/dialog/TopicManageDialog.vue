<script setup lang="ts">
/**
 * 泳道管理弹窗（原「话题管理」；change `lane-manual-correction-loop` 6.4）。
 *
 * 更名理由（契约 `lane-correction-surface`「入口接线」）：本面板管理的是**泳道实体**，
 * 候选人工激活只是其中一个子能力——入口与标题统一叫「泳道管理」。
 *
 * 行内「编辑」入口 → `editLane` 事件（上抛给壳层）→ 进泳道修正工作面（画布）。
 * 窄屏（<768px）该入口**禁用**并说明原因（画布 SHALL NOT 画残缺两栏）。
 */
import { Icon } from '@iconify/vue'
import { useDailyReportsApi, type BoardTopicListItem } from '~/api/dailyReports'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'

const props = defineProps<{
  modelValue: boolean
  boardId: number
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  changed: []
  /** 「编辑泳道」→ 泳道修正工作面（画布）。 */
  editLane: [topicId: number]
}>()

const {
  listBoardTopics,
  updateTopic,
  deleteTopic,
  mergeTopics,
  backfillPersistentTopics,
} = useDailyReportsApi()

// --- List state ---
const topics = ref<BoardTopicListItem[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const filter = ref<'all' | 'active' | 'candidate' | 'archived'>('all')
const search = ref('')
/** 窄屏（<768px）：泳道修正画布入口禁用（只提示宽屏/横屏）。 */
const isNarrow = useIsNarrowViewport()

// --- In-flight op guard (shared across topic rows) ---
const busyId = ref<number | null>(null)

const STATUS_LABEL: Record<string, string> = {
  candidate: '候选',
  active: '活跃',
  archived: '已归档',
}

const stats = computed(() => {
  let active = 0, candidate = 0, archived = 0
  for (const t of topics.value) {
    if (t.status === 'active') active++
    else if (t.status === 'candidate') candidate++
    else if (t.status === 'archived') archived++
  }
  return { active, candidate, archived, total: topics.value.length }
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return topics.value.filter(t => {
    if (filter.value !== 'all' && t.status !== filter.value) return false
    if (q && !t.label.toLowerCase().includes(q)) return false
    return true
  })
})

async function load() {
  loading.value = true
  error.value = null
  const res = await listBoardTopics(props.boardId)
  loading.value = false
  if (res.success && res.data) {
    topics.value = res.data.topics ?? []
  } else {
    error.value = res.error || '加载话题失败'
  }
}

// Reload whenever the dialog opens for this board.
watch(() => props.modelValue, (open) => {
  if (open) load()
})

watch(() => props.boardId, () => {
  if (props.modelValue) load()
})

// --- Rename sub-dialog ---
const renameTarget = ref<BoardTopicListItem | null>(null)
const renameLabel = ref('')
const renameOpen = computed({
  get: () => renameTarget.value !== null,
  set: (v) => { if (!v) renameTarget.value = null },
})

function startRename(t: BoardTopicListItem) {
  renameTarget.value = t
  renameLabel.value = t.label
}

async function confirmRename() {
  const t = renameTarget.value
  if (!t) return
  const label = renameLabel.value.trim()
  if (!label || label === t.label) { renameTarget.value = null; return }
  busyId.value = t.id
  const res = await updateTopic(t.id, { label })
  busyId.value = null
  if (res.success) {
    renameTarget.value = null
    await load()
    emit('changed')
  } else {
    error.value = res.error || '重命名失败'
  }
}

// --- Merge sub-dialog ---
const mergeTarget = ref<BoardTopicListItem | null>(null)
const mergeSelected = ref<number | null>(null)
const mergeOpen = computed({
  get: () => mergeTarget.value !== null,
  set: (v) => { if (!v) { mergeTarget.value = null; mergeSelected.value = null } },
})

function startMerge(t: BoardTopicListItem) {
  mergeTarget.value = t
  mergeSelected.value = null
}

const mergeCandidates = computed(() =>
  topics.value.filter(t =>
    mergeTarget.value
    && t.id !== mergeTarget.value.id
    && t.status !== 'archived',
  ),
)

async function confirmMerge() {
  const t = mergeTarget.value
  const targetId = mergeSelected.value
  if (!t || targetId === null) return
  const target = topics.value.find(o => o.id === targetId)
  if (!target) return
  busyId.value = t.id
  const res = await mergeTopics(targetId, [t.id])
  busyId.value = null
  if (res.success) {
    mergeTarget.value = null
    mergeSelected.value = null
    await load()
    emit('changed')
  } else {
    error.value = res.error || '合并失败'
  }
}

// --- Archive (confirm) ---
const archiveTarget = ref<BoardTopicListItem | null>(null)
const archiveOpen = computed({
  get: () => archiveTarget.value !== null,
  set: (v) => { if (!v) archiveTarget.value = null },
})

async function confirmArchive() {
  const t = archiveTarget.value
  if (!t) return
  busyId.value = t.id
  const res = await updateTopic(t.id, { status: 'archived' })
  busyId.value = null
  if (res.success) {
    archiveTarget.value = null
    await load()
    emit('changed')
  } else {
    error.value = res.error || '归档失败'
  }
}

// --- Activate candidate (confirm) ---
const activateTarget = ref<BoardTopicListItem | null>(null)
const activateOpen = computed({
  get: () => activateTarget.value !== null,
  set: (v) => { if (!v) activateTarget.value = null },
})

async function confirmActivate() {
  const t = activateTarget.value
  if (!t) return
  busyId.value = t.id
  const res = await updateTopic(t.id, { status: 'active' })
  busyId.value = null
  if (res.success) {
    activateTarget.value = null
    await load()
    emit('changed')
  } else {
    error.value = res.error || '启用失败'
  }
}

// --- Delete (confirm; irreversible) ---
const deleteTarget = ref<BoardTopicListItem | null>(null)
const deleteOpen = computed({
  get: () => deleteTarget.value !== null,
  set: (v) => { if (!v) deleteTarget.value = null },
})
const deleteConfirmText = ref('')
const deleteCanConfirm = computed(() => {
  const t = deleteTarget.value
  if (!t) return false
  return deleteConfirmText.value.trim() === t.label.trim()
})

async function confirmDelete() {
  const t = deleteTarget.value
  if (!t || !deleteCanConfirm.value) return
  busyId.value = t.id
  const res = await deleteTopic(t.id)
  busyId.value = null
  if (res.success) {
    deleteTarget.value = null
    deleteConfirmText.value = ''
    await load()
    emit('changed')
  } else {
    error.value = res.error || '删除失败'
  }
}

// --- Backfill ---
const backfilling = ref(false)
async function runBackfill() {
  if (backfilling.value) return
  backfilling.value = true
  error.value = null
  const res = await backfillPersistentTopics(props.boardId)
  backfilling.value = false
  if (res.success) {
    // Backend rebuilds async; refresh shortly after.
    setTimeout(() => { load() }, 4000)
  } else {
    error.value = res.error || '回刷失败'
  }
}

function fmtDate(d: string) {
  return d ? d.slice(0, 10) : ''
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    title="泳道管理"
    width="720px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <!-- Stats + actions bar -->
    <div class="tm-bar">
      <div class="tm-stats">
        <span><b>{{ stats.active }}</b> 活跃</span>
        <span><b>{{ stats.candidate }}</b> 候选</span>
        <span><b>{{ stats.archived }}</b> 已归档</span>
      </div>
      <AppButton variant="ghost" size="sm" :loading="backfilling" @click="runBackfill">
        <Icon icon="mdi:database-refresh-outline" width="14" />
        <span>{{ backfilling ? '提交中…' : '回刷历史泳道' }}</span>
      </AppButton>
    </div>

    <!-- Filter + search -->
    <div class="tm-controls">
      <div class="tm-tabs">
        <button
          v-for="opt in ([
            ['all', '全部'],
            ['active', '活跃'],
            ['candidate', '候选'],
            ['archived', '已归档'],
          ] as const)"
          :key="opt[0]"
          class="tm-tab"
          :class="{ 'tm-tab--active': filter === opt[0] }"
          @click="filter = opt[0]"
        >{{ opt[1] }}</button>
      </div>
      <AppInput v-model="search" type="text" placeholder="搜索泳道名…" class="tm-search" />
    </div>

    <!-- Error banner -->
    <Transition name="fade">
      <div v-if="error" class="tm-error">
        <Icon icon="mdi:alert-circle" width="16" />
        <span>{{ error }}</span>
        <button class="tm-error-close" @click="error = null">✕</button>
      </div>
    </Transition>

    <!-- List -->
    <div class="tm-list">
      <p v-if="isNarrow" class="tm-narrow-note" data-testid="topic-edit-narrow-note">泳道修正需要宽屏或横屏</p>
      <div v-if="loading" class="tm-empty">加载中…</div>
      <div v-else-if="filtered.length === 0" class="tm-empty">
        {{ topics.length === 0 ? '本板块尚无泳道。' : '无匹配泳道。' }}
      </div>
      <div
        v-for="t in filtered"
        :key="t.id"
        class="tm-row"
        :class="{ 'tm-row--archived': t.status === 'archived' }"
      >
        <span class="tm-color" :style="{ background: t.color }" />
        <div class="tm-main">
          <span class="tm-label">{{ t.label }}</span>
          <span class="tm-meta">
            <span class="tm-status" :class="`tm-status--${t.status}`">{{ STATUS_LABEL[t.status] || t.status }}</span>
            · {{ t.section_count }} 条
            <template v-if="t.status === 'candidate'"> · 连续 {{ t.consecutive_hits }} 天</template>
            · {{ fmtDate(t.first_seen_date) }}→{{ fmtDate(t.last_seen_date) }}
          </span>
        </div>
        <div class="tm-ops">
          <AppButton
            v-if="t.status === 'candidate'"
            variant="primary"
            size="sm"
            :disabled="busyId !== null || !t.can_activate"
            :title="t.can_activate ? '人工确认后进入持久话题泳道' : '需先满足连续多天出现条件'"
            @click="activateTarget = t"
          >确认启用</AppButton>
          <AppButton variant="ghost" size="sm" :disabled="busyId !== null" @click="startRename(t)">重命名</AppButton>
          <AppButton
            v-if="t.status !== 'archived'"
            variant="ghost"
            size="sm"
            :disabled="busyId !== null || isNarrow"
            :title="isNarrow ? '泳道修正需要宽屏或横屏' : undefined"
            data-testid="topic-edit-lane"
            @click="emit('editLane', t.id)"
          >编辑</AppButton>
          <AppButton variant="ghost" size="sm" :disabled="busyId !== null" @click="startMerge(t)">合并</AppButton>
          <AppButton
            v-if="t.status !== 'archived'"
            variant="ghost"
            size="sm"
            :disabled="busyId !== null"
            @click="archiveTarget = t"
          >归档</AppButton>
          <AppButton
            variant="ghost"
            size="sm"
            :disabled="busyId !== null"
            class="tm-delete-btn"
            @click="deleteTarget = t; deleteConfirmText = ''"
          >删除</AppButton>
        </div>
      </div>
    </div>

    <template #footer>
      <AppButton variant="secondary" @click="close">关闭</AppButton>
    </template>
  </AppDialog>

  <!-- Rename sub-dialog（次要路径；主路径是修正工作面标题就地编辑） -->
  <AppDialog
    :model-value="renameOpen"
    title="重命名泳道"
    size="sm"
    @update:model-value="renameOpen = $event"
  >
    <div class="tm-sub-body">
      <label class="tm-field-label">泳道名称</label>
      <AppInput
        v-model="renameLabel"
        type="text"
        placeholder="输入新名称"
      />
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="renameTarget = null">取消</AppButton>
      <AppButton
        variant="primary"
        :loading="busyId === renameTarget?.id"
        @click="confirmRename"
      >保存</AppButton>
    </template>
  </AppDialog>

  <!-- Merge sub-dialog -->
  <AppDialog
    :model-value="mergeOpen"
    title="合并泳道"
    width="460px"
    @update:model-value="mergeOpen = $event"
  >
    <div class="tm-sub-body">
      <p class="tm-hint">
        将「{{ mergeTarget?.label }}」合并进以下哪个泳道？源泳道将被归档，其 section 全部改指目标。
      </p>
      <div class="tm-merge-list">
        <button
          v-for="c in mergeCandidates"
          :key="c.id"
          class="tm-merge-cand"
          :class="{ 'tm-merge-cand--sel': mergeSelected === c.id }"
          @click="mergeSelected = c.id"
        >
          <span class="tm-color" :style="{ background: c.color }" />
          <span class="tm-merge-cand-label">{{ c.label }}</span>
          <span class="tm-merge-cand-count">{{ c.section_count }} 条</span>
        </button>
        <div v-if="mergeCandidates.length === 0" class="tm-empty">无其他可合并话题</div>
      </div>
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="mergeTarget = null">取消</AppButton>
      <AppButton
        variant="primary"
        :disabled="mergeSelected === null"
        :loading="busyId === mergeTarget?.id"
        @click="confirmMerge"
      >确认合并</AppButton>
    </template>
  </AppDialog>

  <!-- Archive confirm -->
  <AppDialog
    :model-value="archiveOpen"
    title="归档泳道"
    width="420px"
    @update:model-value="archiveOpen = $event"
  >
    <p class="tm-confirm-body">
      确定归档「{{ archiveTarget?.label }}」？归档后不再参与新归属，可在列表中重新启用。
    </p>
    <template #footer>
      <AppButton variant="secondary" @click="archiveTarget = null">取消</AppButton>
      <AppButton variant="primary" :loading="busyId === archiveTarget?.id" @click="confirmArchive">归档</AppButton>
    </template>
  </AppDialog>

  <!-- Activate confirm -->
  <AppDialog
    :model-value="activateOpen"
    title="启用为泳道"
    width="420px"
    @update:model-value="activateOpen = $event"
  >
    <p class="tm-confirm-body">
      确定将「{{ activateTarget?.label }}」启用为泳道？启用后参与话题泳道与新归属。
    </p>
    <template #footer>
      <AppButton variant="secondary" @click="activateTarget = null">取消</AppButton>
      <AppButton variant="primary" :loading="busyId === activateTarget?.id" @click="confirmActivate">启用</AppButton>
    </template>
  </AppDialog>

  <!-- Delete confirm (type-to-confirm) -->
  <AppDialog
    :model-value="deleteOpen"
    title="删除泳道"
    width="440px"
    @update:model-value="deleteOpen = $event"
  >
    <div class="tm-sub-body">
      <div class="tm-danger-banner">
        <Icon icon="mdi:alert" width="18" />
        <span>硬删除不可恢复。泳道「{{ deleteTarget?.label }}」下的 {{ deleteTarget?.section_count }} 条 section 将解除归属但保留内容。</span>
      </div>
      <label class="tm-field-label">输入泳道名称「{{ deleteTarget?.label }}」以确认</label>
      <AppInput v-model="deleteConfirmText" type="text" placeholder="话题名称" />
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="deleteTarget = null">取消</AppButton>
      <AppButton
        variant="danger"
        :disabled="!deleteCanConfirm"
        :loading="busyId === deleteTarget?.id"
        @click="confirmDelete"
      >删除</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.tm-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.tm-stats {
  display: flex;
  gap: 16px;
  font-size: 13px;
  color: var(--color-text-secondary);
}
.tm-stats b {
  color: var(--color-text-primary);
  font-weight: 600;
}
.tm-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.tm-tabs {
  display: flex;
  gap: 4px;
}
.tm-tab {
  padding: 4px 12px;
  font-size: 13px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-radius: 6px;
}
.tm-tab:hover { background: var(--color-bg-hover); }
.tm-tab--active {
  background: var(--color-bg-active);
  color: var(--color-text-primary);
  font-weight: 600;
}
.tm-search { flex: 1; }

.tm-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
  background: var(--color-error-bg, rgba(193, 47, 47, 0.1));
  border: 1px solid var(--color-error, #c12f2f);
  border-radius: 8px;
  color: var(--color-error, #c12f2f);
  font-size: 13px;
}
.tm-error-close {
  margin-left: auto;
  border: none;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.tm-list {
  max-height: 46vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tm-narrow-note {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}
.tm-empty {
  padding: 24px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 13px;
}
.tm-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--color-bg-secondary, transparent);
}
.tm-row:hover { background: var(--color-bg-hover); }
.tm-row--archived { opacity: 0.55; }
.tm-color {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
.tm-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tm-label {
  font-size: 14px;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tm-meta {
  font-size: 12px;
  color: var(--color-text-muted);
}
.tm-status {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}
.tm-status--active { background: rgba(45, 138, 122, 0.15); color: #2d8a7a; }
.tm-status--candidate { background: rgba(212, 136, 60, 0.15); color: #d4883c; }
.tm-status--archived { background: var(--color-bg-active); color: var(--color-text-muted); }
.tm-ops {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.tm-delete-btn:hover { color: var(--color-error, #c12f2f); }

.tm-sub-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.tm-field-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary);
}
.tm-hint {
  font-size: 13px;
  color: var(--color-text-secondary);
  margin: 0 0 4px;
  line-height: 1.5;
}
.tm-merge-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 40vh;
  overflow-y: auto;
}
.tm-merge-cand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  text-align: left;
}
.tm-merge-cand:hover { background: var(--color-bg-hover); }
.tm-merge-cand--sel {
  border-color: var(--color-border-strong);
  background: var(--color-bg-hover);
}
.tm-merge-cand-label {
  flex: 1;
  font-size: 13px;
  color: var(--color-text-primary);
}
.tm-merge-cand-count {
  font-size: 12px;
  color: var(--color-text-muted);
}
.tm-confirm-body {
  font-size: 14px;
  color: var(--color-text-primary);
  line-height: 1.5;
  margin: 0;
}
.tm-danger-banner {
  display: flex;
  gap: 8px;
  padding: 10px 12px;
  background: var(--color-error-bg, rgba(193, 47, 47, 0.1));
  border: 1px solid var(--color-error, #c12f2f);
  border-radius: 8px;
  color: var(--color-error, #c12f2f);
  font-size: 13px;
  line-height: 1.5;
}

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
