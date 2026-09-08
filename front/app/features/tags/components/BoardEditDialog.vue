<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'

const props = defineProps<{
  editingBoard: boolean
  editLabel: string
  editDescription: string
  editEnrichmentEnabled: boolean
  editRelationAutoDiscoveryEnabled: boolean
  editWindowDays: number
  editContextLayers: string[]
  editSaving: boolean
  editError: string | null
}>()

const emit = defineEmits<{
  close: []
  save: []
  'update:edit-label': [val: string]
  'update:edit-description': [val: string]
  'update:edit-enrichment-enabled': [val: boolean]
  'update:edit-relation-auto-discovery-enabled': [val: boolean]
  'update:edit-window-days': [val: number]
  'update:edit-context-layers': [val: string[]]
}>()

const show = computed({
  get: () => props.editingBoard,
  set: (val: boolean) => { if (!val) emit('close') },
})

const LAYER_OPTIONS = ['week', 'month', 'year', 'all'] as const
const LAYER_LABEL: Record<string, string> = { week: '周', month: '月', year: '年', all: '总' }

function toggleLayer(layer: string) {
  const current = props.editContextLayers
  const next = current.includes(layer)
    ? current.filter(l => l !== layer)
    : [...current, layer]
  // 保持 week/month/year/all 的固定顺序，便于后端消费
  const ordered = LAYER_OPTIONS.filter(l => next.includes(l))
  emit('update:edit-context-layers', ordered)
}
</script>

<template>
  <AppDialog v-model="show" title="编辑板块" width="520px">
    <form class="board-form" @submit.prevent="emit('save')">
      <label class="form-field">
        <span class="form-label">名称 <span class="required-mark">*</span></span>
        <AppInput
          :model-value="editLabel"
          placeholder="板块名称"
          @update:model-value="emit('update:edit-label', $event as string)"
        />
      </label>
      <label class="form-field">
        <span class="form-label">描述</span>
        <textarea
          :value="editDescription"
          class="native-textarea"
          placeholder="可选描述"
          maxlength="500"
          rows="4"
          @input="emit('update:edit-description', ($event.target as HTMLTextAreaElement).value)"
        />
      </label>

      <div class="form-field">
        <span class="form-label">分析配置</span>
        <div class="enrichment-config">
          <label class="enrichment-toggle">
            <AppToggle
              :model-value="editEnrichmentEnabled"
              @update:model-value="emit('update:edit-enrichment-enabled', $event as boolean)"
            />
            <span class="enrichment-toggle-text">
              <span class="enrichment-toggle-label">开启数据增强</span>
              <span class="enrichment-toggle-hint">关闭时不能触发循环 B 分析（聚焦/板块级），可在工作台面板一键开启</span>
            </span>
          </label>

          <label class="enrichment-toggle" :class="{ disabled: !editEnrichmentEnabled }" data-test="relation-auto-toggle-row">
            <AppToggle
              :model-value="editRelationAutoDiscoveryEnabled && editEnrichmentEnabled"
              :disabled="!editEnrichmentEnabled"
              @update:model-value="emit('update:edit-relation-auto-discovery-enabled', $event as boolean)"
            />
            <span class="enrichment-toggle-text">
              <span class="enrichment-toggle-label">跨版块关系自动发现</span>
              <span class="enrichment-toggle-hint">默认关闭。开启后每份新简报按预算自动从观察发起外部检索，只生成待裁决建议、绝不自动确认；需先开启数据增强。全局预算与冷却在服务配置管理</span>
            </span>
          </label>

          <label class="form-subfield">
            <span class="form-sublabel">实时详情窗口（window_days）</span>
            <AppInput
              :model-value="editWindowDays"
              type="number"
              min="1"
              max="90"
              @update:model-value="emit('update:edit-window-days', ($event as number) || 14)"
            />
          </label>

          <div class="form-subfield">
            <span class="form-sublabel">解读员读取的上下文层（context_layers）</span>
            <div class="layer-chips">
              <button
                v-for="layer in LAYER_OPTIONS"
                :key="layer"
                type="button"
                class="layer-chip"
                :class="{ 'layer-chip--on': editContextLayers.includes(layer) }"
                @click="toggleLayer(layer)"
              >
                <Icon :icon="editContextLayers.includes(layer) ? 'mdi:check' : 'mdi:plus'" width="11" />
                {{ LAYER_LABEL[layer] }}（{{ layer }}）
              </button>
            </div>
            <p class="layer-hint">去掉某层后解读员不再读它（可省 token）；year/all 未生成会自动跳过。</p>
          </div>
        </div>
      </div>

      <p v-if="editError" class="error-text">{{ editError }}</p>
    </form>

    <template #footer>
      <AppButton variant="ghost" size="sm" :disabled="editSaving" @click="emit('close')">取消</AppButton>
      <AppButton
        variant="primary"
        size="sm"
        :disabled="editSaving || !editLabel.trim()"
        @click="emit('save')"
      >
        {{ editSaving ? '保存中...' : '保存' }}
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.board-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.form-label {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  letter-spacing: 0.02em;
}

.required-mark {
  color: var(--color-accent);
}

.native-textarea {
  width: 100%;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font-size: 0.82rem;
  padding: 0.55rem 0.85rem;
  outline: none;
  resize: vertical;
  box-sizing: border-box;
  font-family: inherit;
}

.native-textarea::placeholder {
  color: var(--color-text-muted);
}

.native-textarea:focus {
  border-color: var(--color-input-focus);
}

.error-text {
  font-size: 0.72rem;
  color: var(--color-accent);
}

/* enrichment config */
.enrichment-config {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  padding: 0.7rem 0.8rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-hover);
}

.enrichment-toggle.disabled { opacity: 0.6; }
.enrichment-toggle {
  display: flex;
  align-items: flex-start;
  gap: 0.55rem;
  cursor: pointer;
}

.enrichment-toggle-text {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}

.enrichment-toggle-label {
  font-size: 0.78rem;
  color: var(--color-text-primary);
}

.enrichment-toggle-hint {
  font-size: 0.64rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.form-subfield {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.form-sublabel {
  font-size: 0.68rem;
  color: var(--color-text-secondary);
}

.layer-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.layer-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.2rem 0.5rem;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  font-size: 0.68rem;
  cursor: pointer;
  transition: all 0.12s ease;
  font-family: inherit;
}

.layer-chip:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text-secondary);
}

.layer-chip--on {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.layer-hint {
  font-size: 0.62rem;
  color: var(--color-text-muted);
  margin: 0.15rem 0 0;
  line-height: 1.4;
}
</style>
