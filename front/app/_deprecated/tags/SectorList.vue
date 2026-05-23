<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { SectorItem } from '~/api/boardConcepts'

defineProps<{
  sectors: SectorItem[]
  selectedId: number | null
  loading: boolean
}>()

const emit = defineEmits<{
  select: [id: number | null]
  add: []
  regenerate: []
  delete: [id: number]
}>()

function sourceIcon(source: string): string {
  switch (source) {
    case 'manual': return 'mdi:lock'
    case 'llm': return 'mdi:robot'
    default: return 'mdi:lightning-bolt'
  }
}

function sourceTitle(source: string): string {
  switch (source) {
    case 'manual': return '手动创建'
    case 'llm': return 'LLM 生成'
    default: return '自动生成'
  }
}
</script>

<template>
  <div class="sector-list">
    <div class="sector-list-header">
      <span class="sector-list-title">板块</span>
      <span class="sector-list-count">{{ sectors.length }}</span>
    </div>

    <div
      class="sector-item"
      :class="{ 'sector-item--active': selectedId === null }"
      @click="emit('select', null)"
    >
      <Icon icon="mdi:view-grid" width="14" class="sector-item-icon" />
      <span class="sector-item-label">全部</span>
      <span class="sector-item-badge">{{ sectors.reduce((s, x) => s + x.tag_count, 0) }}</span>
    </div>

    <div v-if="loading" class="sector-loading">
      <div v-for="i in 3" :key="i" class="sector-skeleton" />
    </div>

    <div v-else-if="sectors.length === 0" class="sector-empty">
      <Icon icon="mdi:folder-outline" width="24" class="text-white/15" />
      <p>暂无板块</p>
    </div>

    <div v-else class="sector-items">
      <div
        v-for="sector in sectors"
        :key="sector.id"
        class="sector-item"
        :class="{
          'sector-item--active': selectedId === sector.id,
          'sector-item--declining': sector.declining,
          'sector-item--protected': sector.protected,
        }"
        @click="emit('select', sector.id)"
      >
        <Icon
          :icon="sourceIcon(sector.source)"
          width="13"
          class="sector-source-icon"
          :title="sourceTitle(sector.source)"
        />
        <span class="sector-item-label">{{ sector.name }}</span>
        <span v-if="sector.tag_count > 0" class="sector-item-badge">{{ sector.tag_count }}</span>
        <button
          type="button"
          class="sector-delete-btn"
          title="删除板块"
          @click.stop="emit('delete', sector.id)"
        >
          <Icon icon="mdi:close" width="12" />
        </button>
      </div>
    </div>

    <div class="sector-actions">
      <button
        type="button"
        class="sector-action-btn sector-action-btn--primary"
        @click.stop="emit('add')"
      >
        <Icon icon="mdi:plus" width="14" />
        添加板块
      </button>
      <button
        type="button"
        class="sector-action-btn sector-action-btn--secondary"
        @click="emit('regenerate')"
      >
        <Icon icon="mdi:auto-fix" width="14" />
        LLM 重新生成
      </button>
    </div>
  </div>
</template>

<style scoped>
.sector-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sector-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 0.25rem;
  margin-bottom: 0.25rem;
}

.sector-list-title {
  font-size: 0.7rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
}

.sector-list-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.sector-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.6rem;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.12s ease;
  position: relative;
}

.sector-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.sector-item--active {
  background: rgba(240, 138, 75, 0.1);
  border: 1px solid rgba(240, 138, 75, 0.2);
}

.sector-item--declining {
  opacity: 0.6;
}

.sector-item--protected .sector-source-icon {
  color: rgba(240, 138, 75, 0.6);
}

.sector-item-icon {
  color: rgba(255, 255, 255, 0.35);
  flex-shrink: 0;
}

.sector-source-icon {
  color: rgba(255, 255, 255, 0.3);
  flex-shrink: 0;
}

.sector-item-label {
  flex: 1;
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.75);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sector-item--active .sector-item-label {
  color: rgba(255, 220, 200, 0.9);
}

.sector-item-badge {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.sector-delete-btn {
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

.sector-item:hover .sector-delete-btn {
  opacity: 1;
}

.sector-delete-btn:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}

.sector-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sector-skeleton {
  height: 32px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: sectorPulse 1.5s ease-in-out infinite;
}

@keyframes sectorPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.sector-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.75rem;
}

.sector-items {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.sector-actions {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.sector-action-btn {
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

.sector-action-btn--primary {
  color: rgba(255, 220, 200, 0.7);
  border-color: rgba(240, 138, 75, 0.2);
}

.sector-action-btn--primary:hover {
  background: rgba(240, 138, 75, 0.1);
  border-color: rgba(240, 138, 75, 0.35);
  color: rgba(255, 220, 200, 0.9);
}

.sector-action-btn--secondary {
  color: rgba(147, 197, 253, 0.6);
  border-color: rgba(99, 179, 237, 0.2);
}

.sector-action-btn--secondary:hover {
  background: rgba(99, 179, 237, 0.08);
  border-color: rgba(99, 179, 237, 0.35);
  color: rgba(147, 197, 253, 0.9);
}
</style>
