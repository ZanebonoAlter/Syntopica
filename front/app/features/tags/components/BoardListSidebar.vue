<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { SemanticBoard } from '~/api/semanticBoards'

const props = defineProps<{
  boards: SemanticBoard[]
  selectedBoardId: number | null
  boardsLoading: boolean
  boardsError: string | null
}>()

const emit = defineEmits<{
  select: [id: number | null]
  delete: [id: number]
  edit: [board: SemanticBoard]
  'add-board': []
  'upgrade-suggest': []
  'trigger-backfill': []
  'open-merge-preview': []
  'open-matching-config': []
  'open-generate': []
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
  <aside class="tags-sidebar">
    <div v-if="boardsError" class="tags-sidebar-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ boardsError }}</span>
    </div>
    <div class="sb-list" data-onboarding="tags-board-list">
      <div class="sb-list-header">
        <span class="sb-list-title">语义板块</span>
        <span class="sb-list-count">{{ boards.length }}</span>
      </div>

      <div
        class="sb-item"
        :class="{ 'sb-item--active': selectedBoardId === null }"
        @click="emit('select', null)"
      >
        <Icon icon="mdi:view-grid" width="14" class="sb-item-icon" />
        <span class="sb-item-label">全部</span>
        <span class="sb-item-badge">{{ boards.reduce((s, x) => s + x.tag_count, 0) }}</span>
      </div>

      <div v-if="boardsLoading" class="sb-loading">
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
            'sb-item--active': selectedBoardId === board.id,
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
            class="sb-icon-btn sb-edit-btn"
            title="编辑板块"
            @click.stop="emit('edit', board)"
          >
            <Icon icon="mdi:pencil" width="12" />
          </button>
          <button
            type="button"
            class="sb-icon-btn sb-delete-btn"
            title="删除板块"
            @click.stop="emit('delete', board.id)"
          >
            <Icon icon="mdi:close" width="12" />
          </button>
        </div>
      </div>

      <div class="sb-actions" data-onboarding="tags-board-actions">
        <button type="button" class="sb-action-btn sb-action-btn--primary" data-onboarding="tags-add-board" @click="emit('add-board')">
          <Icon icon="mdi:plus" width="14" />
          添加板块
        </button>
        <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="emit('upgrade-suggest')">
          <Icon icon="mdi:auto-fix" width="14" />
          升级建议
        </button>
        <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="emit('trigger-backfill')">
          <Icon icon="mdi:backup-restore" width="14" />
          匹配回填
        </button>
        <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="emit('open-merge-preview')">
          <Icon icon="mdi:call-merge" width="14" />
          标签合并
        </button>
        <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="emit('open-matching-config')">
          <Icon icon="mdi:tune" width="14" />
          匹配参数
        </button>
        <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="emit('open-generate')">
          <Icon icon="mdi:auto-fix" width="14" />
          生成日报
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.tags-sidebar {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid var(--color-border-subtle);
  padding: 1rem;
  overflow-y: auto;
}

/* 窄屏（<768px）：随父容器纵向堆叠，占满宽度并限高自滚；动作按钮双列防过长（mobile-viewport-stage1 任务 4.1） */
@media (max-width: 767.98px) {
  .tags-sidebar {
    width: 100%;
    max-height: 40vh;
    border-right: none;
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .sb-actions {
    display: grid;
    grid-template-columns: 1fr 1fr;
  }
}

.tags-sidebar-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.8rem;
  margin-bottom: 0.75rem;
  border-radius: 10px;
  border: 1px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.75rem;
}

.sb-list { display: flex; flex-direction: column; gap: 0.5rem; }
.sb-list-header { display: flex; align-items: center; justify-content: space-between; padding: 0 0.25rem; margin-bottom: 0.25rem; }
.sb-list-title { font-size: 0.7rem; letter-spacing: 0.18em; text-transform: uppercase; color: var(--color-text-muted); }
.sb-list-count { font-size: 0.65rem; color: var(--color-text-muted); padding: 0.1rem 0.45rem; border-radius: 999px; background: var(--color-bg-hover); }

.sb-item { display: flex; align-items: center; gap: 0.4rem; padding: 0.45rem 0.6rem; border-radius: 10px; cursor: pointer; transition: all 0.12s ease; position: relative; }
.sb-item:hover { background: var(--color-bg-hover); }
.sb-item--active { background: var(--color-accent-subtle); border: 1px solid var(--color-accent); }
.sb-item--protected .sb-source-icon { color: var(--color-accent); }

.sb-item-icon, .sb-source-icon { color: var(--color-text-muted); flex-shrink: 0; }
.sb-item-label { flex: 1; font-size: 0.8rem; color: var(--color-text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sb-item--active .sb-item-label { color: var(--color-accent); }
.sb-item-badge { font-size: 0.6rem; color: var(--color-text-muted); padding: 0.1rem 0.4rem; border-radius: 999px; background: var(--color-bg-hover); flex-shrink: 0; }
.sb-icon-btn { display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; border: none; border-radius: 6px; background: none; color: var(--color-text-muted); cursor: pointer; opacity: 0; transition: all 0.12s ease; flex-shrink: 0; }
.sb-item:hover .sb-icon-btn { opacity: 1; }
.sb-edit-btn:hover { color: var(--color-text-inverted); background: var(--color-secondary); }
.sb-delete-btn:hover { color: rgba(252, 165, 165, 0.9); background: rgba(239, 68, 68, 0.12); }

.sb-loading { display: flex; flex-direction: column; gap: 0.5rem; }
.sb-skeleton { height: 32px; border-radius: 10px; background: var(--color-bg-hover); animation: sbPulse 1.5s ease-in-out infinite; }
@keyframes sbPulse { 0%, 100% { opacity: 0.4; } 50% { opacity: 0.8; } }

.sb-empty { display: flex; flex-direction: column; align-items: center; gap: 0.4rem; padding: 2rem 0; color: var(--color-text-muted); font-size: 0.75rem; }
.sb-items { display: flex; flex-direction: column; gap: 1px; }

.sb-actions { display: flex; flex-direction: column; gap: 0.4rem; margin-top: 0.75rem; padding-top: 0.75rem; border-top: 1px solid var(--color-border-subtle); }
.sb-action-btn { display: flex; align-items: center; justify-content: center; gap: 0.4rem; width: 100%; padding: 0.5rem; border-radius: 10px; border: 1px solid var(--color-border-medium); background: none; font-size: 0.75rem; cursor: pointer; transition: all 0.12s ease; }
.sb-action-btn--primary { color: var(--color-accent); border-color: var(--color-accent); }
.sb-action-btn--primary:hover { background: var(--color-accent-subtle); border-color: var(--color-accent); color: var(--color-accent-hover); }
.sb-action-btn--secondary { color: var(--color-secondary); border-color: var(--color-secondary); }
.sb-action-btn--secondary:hover { background: var(--color-secondary); border-color: var(--color-secondary); color: var(--color-text-inverted); }
.sb-action-btn--ghost { color: var(--color-text-muted); border-color: var(--color-border-subtle); }
.sb-action-btn--ghost:hover { background: var(--color-bg-hover); color: var(--color-text-secondary); }
</style>
