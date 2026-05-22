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
