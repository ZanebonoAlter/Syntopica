<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyTemplate, HierarchyLevel, ConfigImpact } from '~/api/hierarchyConfig'

const props = defineProps<{
  category: string
}>()

const emit = defineEmits<{
  saved: []
  cancel: []
}>()

const api = useHierarchyConfigApi()

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const templates = ref<HierarchyTemplate[]>([])
const activeTemplate = ref<HierarchyTemplate | null>(null)

const showImpact = ref(false)
const impactData = ref<ConfigImpact | null>(null)
const impactSaving = ref(false)
const previewTemplates = ref<HierarchyTemplate[] | null>(null)

const editLevels = ref<HierarchyLevel[]>([])

watch(() => props.category, () => {
  const t = templates.value.find(t => t.category === props.category)
  activeTemplate.value = t ?? null
  if (t) {
    editLevels.value = JSON.parse(JSON.stringify(t.levels))
  } else {
    editLevels.value = []
  }
}, { immediate: false })

async function loadConfig() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getConfig()
    if (res.success && res.data) {
      templates.value = res.data.templates
      const t = templates.value.find(t => t.category === props.category)
      activeTemplate.value = t ?? null
      if (t) {
        editLevels.value = JSON.parse(JSON.stringify(t.levels))
      }
    } else {
      error.value = res.error || '加载配置失败'
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '网络错误'
  } finally {
    loading.value = false
  }
}

loadConfig()

function addLevel() {
  const maxLevel = editLevels.value.reduce((m, l) => Math.max(m, l.level), 0)
  editLevels.value.push({
    level: maxLevel + 1,
    name: '',
    description: '',
    is_leaf: false,
    max_children: 20,
    forbidden_patterns: [],
  })
}

function removeLevel(index: number) {
  editLevels.value.splice(index, 1)
  editLevels.value.forEach((l, i) => {
    l.level = i + 1
  })
}

const hasChanges = computed(() => {
  if (!activeTemplate.value) return editLevels.value.length > 0
  const original = JSON.stringify(activeTemplate.value.levels)
  const current = JSON.stringify(editLevels.value)
  return original !== current
})

async function handleSave() {
  if (!hasChanges.value) return

  saving.value = true
  error.value = ''
  try {
    const updatedTemplates = templates.value.map(t => {
      if (t.category === props.category) {
        return { ...t, levels: JSON.parse(JSON.stringify(editLevels.value)), max_level: editLevels.value.length }
      }
      return t
    })

    const res = await api.previewConfig(updatedTemplates, `预览 ${props.category} 模板层级`)
    if (res.success && res.data) {
      impactData.value = res.data.impact
      previewTemplates.value = updatedTemplates
      showImpact.value = true
    } else {
      error.value = res.error || '预览失败'
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '网络错误'
  } finally {
    saving.value = false
  }
}

async function handleConfirmImpact() {
  if (!previewTemplates.value) return
  impactSaving.value = true
  error.value = ''
  try {
    const res = await api.applyConfig(previewTemplates.value, `更新 ${props.category} 模板层级`)
    if (!res.success) {
      error.value = res.error || '应用失败'
      return
    }
    showImpact.value = false
    emit('saved')
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '网络错误'
  } finally {
    impactSaving.value = false
  }
}

const affectedCount = computed(() => impactData.value?.total_tags ?? 0)
const estimatedMinutes = computed(() => {
  const total = affectedCount.value
  if (total === 0) return '< 1'
  return Math.max(1, Math.ceil(total * 0.5 / 60))
})
</script>

<template>
  <Teleport to="body">
    <!-- Impact Confirmation Overlay -->
    <div v-if="showImpact" class="dialog-overlay" style="z-index: 110">
      <div class="dialog-card" style="width: min(400px, 90%)">
        <div class="dialog-header">
          <h3 class="dialog-title">变更影响确认</h3>
        </div>

        <div class="dialog-body">
          <div class="impact-summary">
            <div class="impact-row">
              <Icon icon="mdi:tag-outline" width="16" class="impact-icon" />
              <span>受影响标签: <strong>{{ affectedCount }}</strong> 个</span>
            </div>
            <div class="impact-row">
              <Icon icon="mdi:clock-outline" width="16" class="impact-icon" />
              <span>预计重建耗时: <strong>{{ estimatedMinutes }}</strong> 分钟</span>
            </div>
          </div>
          <div v-if="impactData" class="impact-detail">
            <div v-if="impactData.depth_exceeded > 0" class="impact-detail-item">
              <span class="impact-detail-label">深度超限</span>
              <span class="impact-detail-value">{{ impactData.depth_exceeded }}</span>
            </div>
            <div v-if="impactData.level_mismatch > 0" class="impact-detail-item">
              <span class="impact-detail-label">层级不匹配</span>
              <span class="impact-detail-value">{{ impactData.level_mismatch }}</span>
            </div>
            <div v-if="impactData.new_leaf_violations > 0" class="impact-detail-item">
              <span class="impact-detail-label">叶子节点违规</span>
              <span class="impact-detail-value">{{ impactData.new_leaf_violations }}</span>
            </div>
          </div>
        </div>

        <div class="dialog-footer">
          <button type="button" class="dialog-btn dialog-btn--ghost" @click="showImpact = false">
            取消
          </button>
          <button
            type="button"
            class="dialog-btn dialog-btn--primary"
            :disabled="impactSaving"
            @click="handleConfirmImpact"
          >
            <Icon v-if="impactSaving" icon="mdi:loading" width="14" class="animate-spin" />
            确认重建
          </button>
        </div>
      </div>
    </div>

    <!-- Main Template Settings Dialog -->
    <div class="dialog-overlay" @click.self="emit('cancel')">
      <div class="dialog-card dialog-card--wide">
        <div class="dialog-header">
          <h3 class="dialog-title">
            <Icon icon="mdi:cog-outline" width="16" class="mr-1.5" />
            模板设置 — {{ category }}
          </h3>
          <button type="button" class="dialog-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div v-if="loading" class="dialog-loading">
          <Icon icon="mdi:loading" width="20" class="animate-spin" />
          <span>加载配置中...</span>
        </div>

        <div v-else-if="error" class="dialog-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ error }}</span>
        </div>

        <template v-else>
          <div class="dialog-body">
            <div class="level-list">
              <div
                v-for="(level, index) in editLevels"
                :key="index"
                class="level-card"
              >
                <div class="level-card-header">
                  <span class="level-badge">L{{ level.level }}</span>
                  <button
                    type="button"
                    class="level-remove-btn"
                    title="删除层级"
                    @click="removeLevel(index)"
                  >
                    <Icon icon="mdi:close" width="12" />
                  </button>
                </div>
                <div class="level-card-fields">
                  <label class="level-field">
                    <span class="level-field-label">名称</span>
                    <input
                      v-model="level.name"
                      type="text"
                      class="dialog-input"
                      placeholder="层级名称"
                    />
                  </label>
                  <label class="level-field level-field--small">
                    <span class="level-field-label">最大子节点数</span>
                    <input
                      v-model.number="level.max_children"
                      type="number"
                      class="dialog-input"
                      min="1"
                      max="100"
                    />
                  </label>
                  <label class="level-checkbox">
                    <input type="checkbox" v-model="level.is_leaf" />
                    <span class="level-checkbox-label">叶子节点</span>
                  </label>
                </div>
              </div>
            </div>

            <button type="button" class="add-level-btn" @click="addLevel">
              <Icon icon="mdi:plus" width="14" />
              添加层级
            </button>
          </div>

          <div class="dialog-footer">
            <button type="button" class="dialog-btn dialog-btn--ghost" @click="emit('cancel')">
              取消
            </button>
            <button
              type="button"
              class="dialog-btn dialog-btn--primary"
              :disabled="saving || !hasChanges"
              @click="handleSave"
            >
              <Icon v-if="saving" icon="mdi:loading" width="14" class="animate-spin" />
              预览影响
            </button>
          </div>
        </template>
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
  max-height: 85vh;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.dialog-card--wide {
  width: min(560px, 92%);
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1rem;
  flex-shrink: 0;
}

.dialog-title {
  display: flex;
  align-items: center;
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

.dialog-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 3rem 0;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.82rem;
}

.dialog-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.8rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(255, 200, 180, 0.85);
  font-size: 0.75rem;
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
  gap: 0.3rem;
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

.level-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.level-card {
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 10px;
  padding: 0.65rem 0.75rem;
  background: rgba(0, 0, 0, 0.12);
}

.level-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.level-badge {
  font-size: 0.7rem;
  font-weight: 600;
  color: rgba(240, 138, 75, 0.7);
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.1);
  border: 1px solid rgba(240, 138, 75, 0.15);
}

.level-remove-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.2);
  cursor: pointer;
  transition: all 0.12s ease;
}

.level-remove-btn:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}

.level-card-fields {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
}

.level-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  flex: 1;
}

.level-field--small {
  flex: 0 0 100px;
}

.level-field-label {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
  letter-spacing: 0.02em;
}

.level-checkbox {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding-bottom: 0.35rem;
  cursor: pointer;
  flex-shrink: 0;
}

.level-checkbox input[type="checkbox"] {
  accent-color: rgba(240, 138, 75, 0.7);
  width: 14px;
  height: 14px;
}

.level-checkbox-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.5);
  white-space: nowrap;
}

.add-level-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  width: 100%;
  padding: 0.5rem;
  border: 1px dashed rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.add-level-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.03);
}

.impact-summary {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  border-radius: 10px;
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
}

.impact-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.82rem;
  color: rgba(255, 200, 180, 0.85);
}

.impact-row strong {
  color: rgba(255, 220, 200, 0.95);
}

.impact-icon {
  color: rgba(240, 138, 75, 0.6);
  flex-shrink: 0;
}

.impact-detail {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-top: 0.5rem;
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.15);
}

.impact-detail-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.72rem;
}

.impact-detail-label {
  color: rgba(255, 255, 255, 0.5);
}

.impact-detail-value {
  color: rgba(252, 165, 165, 0.8);
  font-weight: 500;
}
</style>
