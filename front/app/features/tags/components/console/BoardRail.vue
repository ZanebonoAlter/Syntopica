<script setup lang="ts">
/**
 * 版块栏（rework-workshop-into-radar-and-desk 1.2）：态势页左栏。
 *
 * 只做导航：列出全部版块，高亮当前版块，点其它版块切路由。
 * 「全部 / 标签池」入口退役（雷达是工坊的家，回雷达走面包屑）。
 */
import { Icon } from '@iconify/vue'
import type { SemanticBoard } from '~/api/semanticBoards'

defineProps<{
  boards: SemanticBoard[]
  selectedBoardId: number
  loading: boolean
  error: string | null
  /** 新建版块唯一入口已迁回总览（BoardRadarGovernance）；本版块栏默认不再重复。 */
  allowAddBoard?: boolean
}>()

const emit = defineEmits<{
  select: [boardId: number]
  retry: []
  'add-board': []
}>()
</script>

<template>
  <aside class="rail" data-testid="board-rail">
    <div class="rail-head">
      <span class="rail-title">语义板块</span>
      <span class="rail-count">{{ boards.length }}</span>
    </div>

    <div v-if="error" class="rail-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ error }}</span>
      <button type="button" class="rail-retry" data-testid="board-rail-retry" @click="emit('retry')">重试</button>
    </div>

    <div v-if="loading" class="rail-skeleton">
      <div v-for="i in 4" :key="i" class="rail-skeleton-row" />
    </div>

    <div v-else-if="boards.length === 0" class="rail-empty">
      <Icon icon="mdi:folder-outline" width="20" />
      <span>暂无板块</span>
    </div>

    <div v-else class="rail-list">
      <button
        v-for="board in boards"
        :key="board.id"
        type="button"
        class="rail-item"
        :class="{ 'is-active': board.id === selectedBoardId }"
        :title="board.label"
        :data-testid="`board-rail-item-${board.id}`"
        @click="emit('select', board.id)"
      >
        <span class="rail-item-name">{{ board.label }}</span>
        <span class="rail-item-count">{{ board.tag_count }}</span>
      </button>
    </div>

    <button
      v-if="allowAddBoard"
      type="button"
      class="rail-add"
      data-testid="board-rail-add"
      @click="emit('add-board')"
    >
      <Icon icon="mdi:plus" width="14" />
      新建版块
    </button>
  </aside>
</template>

<style scoped>
.rail {
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow-y: auto;
  border-right: 1px solid var(--color-border-subtle);
  padding: 0.9rem 0.7rem 1rem 0.9rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.rail-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 0 0.4rem 0.6rem;
}

.rail-title {
  font-size: 0.66rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.rail-count {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rail-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.rail-error {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.5rem;
  border-radius: 6px;
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 0.7rem;
}

.rail-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: 0.2rem;
}

.rail-skeleton-row {
  height: 1.9rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: rail-pulse 1.4s ease-in-out infinite;
}

@keyframes rail-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

.rail-empty {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.8rem 0.5rem;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.rail-list {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.rail-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.42rem 0.45rem;
  border: none;
  border-radius: 6px;
  background: none;
  color: var(--color-text-secondary);
  text-align: left;
  cursor: pointer;
}

.rail-item:hover {
  background: var(--color-bg-hover);
}

.rail-item.is-active {
  background: var(--color-accent-subtle);
}

.rail-item-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.82rem;
}

.rail-item.is-active .rail-item-name {
  color: var(--color-accent);
  font-weight: 600;
}

.rail-item-count {
  flex: 0 0 auto;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rail-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.3rem;
  width: 100%;
  margin-top: auto;
  padding: 0.45rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 5px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}

.rail-add:hover,
.rail-add:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}
</style>
