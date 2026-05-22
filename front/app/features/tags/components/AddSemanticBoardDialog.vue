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
