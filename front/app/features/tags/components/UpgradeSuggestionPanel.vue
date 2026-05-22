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
