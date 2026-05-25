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
  generate: []
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
      <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="emit('generate')">
        <Icon icon="mdi:auto-fix" width="14" />
        整理叙事
      </button>
    </div>
  </div>
</template>

<style scoped>
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
