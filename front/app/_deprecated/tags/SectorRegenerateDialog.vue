<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { SectorDiff } from '~/api/boardConcepts'

const props = defineProps<{
  diff: SectorDiff
  loading: boolean
}>()

const keep = computed(() => props.diff.keep ?? [])
const add = computed(() => props.diff.add ?? [])
const merge = computed(() => props.diff.merge ?? [])
const split = computed(() => props.diff.split ?? [])

const emit = defineEmits<{
  confirm: [diff: SectorDiff]
  cancel: []
}>()
</script>

<template>
  <Teleport to="body">
    <div class="dialog-overlay" @click.self="emit('cancel')">
      <div class="dialog-card">
        <div class="dialog-header">
          <h3 class="dialog-title">LLM 重新生成预览</h3>
          <button type="button" class="dialog-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div class="dialog-body">
          <div class="diff-section diff-section--keep">
            <div class="diff-section-header">
              <Icon icon="mdi:check-circle-outline" width="14" class="text-green-400/70" />
              <span class="diff-section-title">保留</span>
              <span class="diff-section-count">{{ keep.length }}</span>
            </div>
            <div v-if="keep.length > 0" class="diff-items">
              <div v-for="item in keep" :key="item.id" class="diff-item diff-item--keep">
                {{ item.name }}
              </div>
            </div>
            <div v-else class="diff-empty">无保留项</div>
          </div>

          <div class="diff-section diff-section--add">
            <div class="diff-section-header">
              <Icon icon="mdi:plus-circle-outline" width="14" class="text-blue-400/70" />
              <span class="diff-section-title">新增</span>
              <span class="diff-section-count">{{ add.length }}</span>
            </div>
            <div v-if="add.length > 0" class="diff-items">
              <div v-for="(item, i) in add" :key="i" class="diff-item diff-item--add">
                <span class="diff-item-name">{{ item.name }}</span>
                <span v-if="item.description" class="diff-item-desc">{{ item.description }}</span>
              </div>
            </div>
            <div v-else class="diff-empty">无新增项</div>
          </div>

          <div class="diff-section diff-section--merge">
            <div class="diff-section-header">
              <Icon icon="mdi:merge" width="14" class="text-purple-400/70" />
              <span class="diff-section-title">合并</span>
              <span class="diff-section-count">{{ merge.length }}</span>
            </div>
            <div v-if="merge.length > 0" class="diff-items">
              <div v-for="(item, i) in merge" :key="i" class="diff-item diff-item--merge">
                <span class="diff-item-name">{{ item.name }}</span>
                <span class="diff-item-detail">
                  ← 合并 {{ item.source_ids.length }} 个源 → 目标 #{{ item.target_id }}
                </span>
              </div>
            </div>
            <div v-else class="diff-empty">无合并项</div>
          </div>

          <div class="diff-section diff-section--split">
            <div class="diff-section-header">
              <Icon icon="mdi:call-split" width="14" class="text-amber-400/70" />
              <span class="diff-section-title">拆分</span>
              <span class="diff-section-count">{{ split.length }}</span>
            </div>
            <div v-if="split.length > 0" class="diff-items">
              <div v-for="(item, i) in split" :key="i" class="diff-item diff-item--split">
                <span class="diff-item-name">源 #{{ item.source_id }}</span>
                <div class="diff-split-items">
                  <span v-for="(ni, j) in item.new_items" :key="j" class="diff-split-item">
                    {{ ni.name }}
                  </span>
                </div>
              </div>
            </div>
            <div v-else class="diff-empty">无拆分项</div>
          </div>

          <div v-if="diff.affected_tag_count > 0" class="diff-affected">
            <Icon icon="mdi:tag-outline" width="13" />
            <span>影响 {{ diff.affected_tag_count }} 个标签</span>
          </div>
        </div>

        <div class="dialog-footer">
          <button type="button" class="dialog-btn dialog-btn--ghost" @click="emit('cancel')">
            取消
          </button>
          <button
            type="button"
            class="dialog-btn dialog-btn--primary"
            :disabled="loading"
            @click="emit('confirm', diff)"
          >
            <Icon v-if="loading" icon="mdi:loading" width="14" class="animate-spin mr-1" />
            确认执行
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
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
  width: min(520px, 92%);
  max-height: 85vh;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1rem;
  flex-shrink: 0;
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
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-right: 0.25rem;
}

.diff-section {
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 10px;
  padding: 0.65rem 0.75rem;
  background: rgba(0, 0, 0, 0.12);
}

.diff-section-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.4rem;
}

.diff-section-title {
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.7);
  font-weight: 500;
}

.diff-section-count {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.diff-items {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.diff-item {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  font-size: 0.78rem;
}

.diff-item--keep {
  color: rgba(134, 239, 172, 0.8);
}

.diff-item--add {
  color: rgba(147, 197, 253, 0.85);
}

.diff-item--merge {
  color: rgba(196, 181, 253, 0.85);
}

.diff-item--split {
  color: rgba(252, 211, 77, 0.85);
}

.diff-item-name {
  font-weight: 500;
}

.diff-item-desc {
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.35);
}

.diff-item-detail {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
}

.diff-split-items {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-top: 0.2rem;
}

.diff-split-item {
  font-size: 0.65rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: rgba(252, 211, 77, 0.1);
  border: 1px solid rgba(252, 211, 77, 0.15);
  color: rgba(252, 211, 77, 0.7);
}

.diff-empty {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.25);
  text-align: center;
  padding: 0.3rem;
}

.diff-affected {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
  color: rgba(255, 200, 180, 0.8);
  font-size: 0.72rem;
}

.dialog-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1rem;
  flex-shrink: 0;
}

.dialog-btn {
  display: flex;
  align-items: center;
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
