<script setup lang="ts">
import { ref } from 'vue'
import { useNotify } from '~/composables/useNotify'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type AuxiliaryLabelItem, type BoardCompositeMount } from '~/api/semanticBoards'
import AuxiliaryLabelPicker from './AuxiliaryLabelPicker.vue'
import TopicLandscapePanel from './topic-landscape/TopicLandscapePanel.vue'
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
  selectTopic: [topicId: number]
}>()

const { success: notifySuccess } = useNotify()
const api = useSemanticBoardsApi()
const showPicker = ref(false)
const pendingIds = ref<number[]>([])
const adding = ref(false)
const notice = ref('')
const showCompositeDialog = ref(false)

function handleRemove(id: number, label: string) {
  if (!confirm(`从板块中移除辅助标签 "${label}"？\n注意：不会自动回填历史数据。`)) return
  emit('remove', id)
}

/** 移除组合挂载（复用 composition 删除接口，auxiliary_label_id 列复用）。 */
function handleRemoveCompositeMount(item: BoardCompositeMount) {
  if (!confirm(`从板块中移除组合标签 "${item.label}"（${item.components.join(' × ')}）？\n注意：不会自动回填历史归属。`)) return
  api.removeFromComposition(props.boardId, item.id).then(() => {
    emit('refresh')
  }).catch(() => {
    notice.value = '移除组合挂载失败，请重试'
  })
}

async function handleConfirmAdd() {
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
</script>

<template>
  <div class="bcp-panel">
    <div class="bcp-header">
      <Icon icon="mdi:puzzle-outline" width="15" class="text-white/50" />
      <span class="bcp-title">构成标签</span>
      <span class="bcp-count">{{ labels.length }}</span>
      <div class="bcp-spacer" />
      <button type="button" class="bcp-add-btn" @click="showPicker = !showPicker">
        <Icon :icon="showPicker ? 'mdi:close' : 'mdi:plus'" width="14" />
        {{ showPicker ? '取消' : '添加' }}
      </button>
    </div>

    <div v-if="notice" class="bcp-notice">
      <Icon icon="mdi:information-outline" width="14" />
      <span>{{ notice }}</span>
    </div>

    <!-- Add auxiliary picker -->
    <div v-if="showPicker" class="bcp-picker">
      <AuxiliaryLabelPicker
        mode="edit"
        :board-id="boardId"
        :selected-ids="pendingIds"
        @update:selected-ids="pendingIds = $event"
      />
      <div v-if="pendingIds.length > 0" class="bcp-confirm">
        <button type="button" class="bcp-confirm-btn" :disabled="adding" @click="handleConfirmAdd">
          <Icon icon="mdi:check" width="14" />
          {{ adding ? '添加中...' : `确认添加 ${pendingIds.length} 个标签` }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="bcp-loading">
      <div v-for="i in 3" :key="i" class="bcp-skeleton-chip" />
    </div>

    <div v-else-if="labels.length === 0 && !showPicker" class="bcp-empty">
      <Icon icon="mdi:tag-off-outline" width="20" class="text-white/15" />
      <span>暂无构成标签</span>
      <button type="button" class="bcp-empty-add" @click="showPicker = true">点击添加</button>
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

    <!-- 组合标签（add-composite-labels：版块维度的组合挂载管理 + 创建入口） -->
    <div class="bcp-comp-section" data-testid="board-composite-section">
      <div class="bcp-header">
        <Icon icon="mdi:link-variant" width="15" class="text-white/50" />
        <span class="bcp-title">组合标签</span>
        <span class="bcp-count">{{ composites?.length ?? 0 }}</span>
        <div class="bcp-spacer" />
        <button type="button" class="bcp-add-btn" data-testid="board-composite-create" @click="showCompositeDialog = true">
          <Icon icon="mdi:plus" width="14" />
          创建并挂载
        </button>
      </div>
      <div v-if="!composites || composites.length === 0" class="bcp-comp-empty">
        暂未挂载组合标签——组合标签是最强匹配信号（composite_hit），适合「美国国债×收益率」这类指向性主题
      </div>
      <div v-else class="bcp-comp-list">
        <div
          v-for="item in composites"
          :key="item.id"
          class="bcp-comp-item"
          :class="{ 'bcp-comp-item--disabled': item.status !== 'active' }"
          :data-testid="`board-composite-item-${item.id}`"
        >
          <span class="bcp-comp-name">{{ item.label }}</span>
          <span class="bcp-comp-chain">= {{ item.components.join(' × ') }}</span>
          <span v-if="item.ref_count > 0" class="bcp-chip-ref">{{ item.ref_count }}</span>
          <button type="button" class="bcp-chip-remove" title="移除挂载" @click="handleRemoveCompositeMount(item)">
            <Icon icon="mdi:close" width="10" />
          </button>
        </div>
      </div>
      <CompositeLabelEditDialog
        :visible="showCompositeDialog"
        :board-id="boardId"
        :board-label="boardLabel"
        @confirm="emit('refresh'); showCompositeDialog = false"
        @cancel="showCompositeDialog = false"
      />
    </div>

    <!-- 视觉分隔 + 话题态势版图（design §5：构成标签管理区下方） -->
    <hr class="bcp-divider">
    <TopicLandscapePanel :board-id="boardId" @select-topic="(id) => emit('selectTopic', id)" />
  </div>
</template>

<style scoped>
.bcp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
}

.bcp-comp-section { display: flex; flex-direction: column; gap: 0.5rem; padding-top: 0.6rem; border-top: 1px dashed var(--color-border-subtle); }
.bcp-comp-empty { padding: 0.5rem 0.6rem; border-radius: 8px; background: var(--color-bg-sunken); font-size: 0.74rem; color: var(--color-text-muted); }
.bcp-comp-list { display: flex; flex-direction: column; gap: 0.35rem; }
.bcp-comp-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.4rem 0.6rem; border: 1px solid var(--color-border-subtle); border-radius: 8px; background: var(--color-bg-base); }
.bcp-comp-item--disabled { opacity: 0.55; }
.bcp-comp-name { font-weight: 600; font-size: 0.78rem; color: var(--color-text-primary); }
.bcp-comp-chain { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.72rem; color: var(--color-text-secondary); }
.bcp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bcp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.bcp-count {
  font-size: 0.65rem;
  color: var(--color-text-muted);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: var(--color-bg-hover);
}

.bcp-spacer {
  flex: 1;
}

.bcp-add-btn {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-primary);
  font-size: 0.68rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.bcp-add-btn:hover {
  background: var(--color-bg-hover);
  border-color: var(--color-border-strong);
}

.bcp-notice {
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

.bcp-picker {
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-input-bg);
}

.bcp-confirm {
  display: flex;
  justify-content: flex-end;
  padding-top: 0.4rem;
}

.bcp-confirm-btn {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.35rem 0.8rem;
  border-radius: 8px;
  border: 1px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.bcp-confirm-btn:hover:not(:disabled) {
  background: var(--color-accent-subtle);
  border-color: var(--color-accent-hover);
}

.bcp-confirm-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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
  background: var(--color-bg-hover);
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
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.bcp-empty-add {
  font-size: 0.68rem;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  border: 1px solid var(--color-secondary);
  background: none;
  color: var(--color-secondary);
  cursor: pointer;
  transition: all 0.1s;
}

.bcp-empty-add:hover {
  background: var(--color-secondary);
  color: var(--color-text-inverted);
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
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-hover);
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  transition: all 0.12s ease;
}

.bcp-chip:hover {
  background: var(--color-bg-hover);
}

.bcp-chip--disabled {
  opacity: 0.5;
  border-style: dashed;
}

.bcp-chip-ref {
  font-size: 0.6rem;
  color: var(--color-text-muted);
  padding: 0 0.25rem;
  border-radius: 4px;
  background: var(--color-bg-hover);
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
  color: var(--color-text-muted);
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

.bcp-divider {
  border: none;
  border-top: 1px solid var(--color-border-subtle);
  margin: 0.25rem 0;
}
</style>
