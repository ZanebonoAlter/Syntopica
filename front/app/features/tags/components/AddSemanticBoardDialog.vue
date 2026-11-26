<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { Icon } from '@iconify/vue'
import AuxiliaryLabelPicker from './AuxiliaryLabelPicker.vue'

const props = defineProps<{
  visible: boolean
  initialData?: {
    label: string
    description: string
    display_order: number
    protected: boolean
  }
}>()

const emit = defineEmits<{
  confirm: [data: { label: string; description: string; display_order: number; protected: boolean; auxiliary_labels: number[] }]
  cancel: []
}>()

const show = computed({
  get: () => props.visible,
  set: (val: boolean) => { if (!val) emit('cancel') }
})

const label = ref('')
const description = ref('')
const displayOrder = ref(0)
const isProtected = ref(false)
const selectedAuxiliaryIds = ref<number[]>([])
const step = ref<'form' | 'picker'>('form')

watch(() => props.visible, (v) => {
  if (v) {
    label.value = props.initialData?.label ?? ''
    description.value = props.initialData?.description ?? ''
    displayOrder.value = props.initialData?.display_order ?? 0
    isProtected.value = props.initialData?.protected ?? false
    selectedAuxiliaryIds.value = []
    step.value = 'form'
  }
})

function nextStep() {
  const trimmed = label.value.trim()
  if (!trimmed) return
  step.value = 'picker'
}

function handleSubmit() {
  const trimmed = label.value.trim()
  if (!trimmed) return
  emit('confirm', {
    label: trimmed,
    description: description.value.trim(),
    display_order: displayOrder.value,
    protected: isProtected.value,
    auxiliary_labels: selectedAuxiliaryIds.value,
  })
}
</script>

<template>
  <AppDialog v-model="show" title="添加板块" width="520px">
    <!-- Step 1: Basic info -->
    <div v-if="step === 'form'" class="form-fields">
      <label class="form-field">
        <span class="form-label">名称 <span class="required-mark">*</span></span>
        <AppInput v-model="label" placeholder="板块名称" />
      </label>
      <label class="form-field">
        <span class="form-label">描述</span>
        <AppInput v-model="description" placeholder="可选描述" />
      </label>
      <label class="form-field">
        <span class="form-label">排序</span>
        <AppInput v-model="displayOrder" type="number" placeholder="0" />
      </label>
      <div class="form-field-row">
        <AppToggle v-model="isProtected" label="受保护（禁止自动删除）" />
      </div>
    </div>

    <!-- Step 2: Auxiliary label picker -->
    <div v-else class="form-fields">
      <div class="step-info">
        <span class="step-badge">2/2</span>
        <span class="step-text">选择构成标签（推荐基于语义相似度，可跳过）</span>
      </div>
      <AuxiliaryLabelPicker
        mode="create"
        :initial-label="label"
        :initial-description="description"
        :selected-ids="selectedAuxiliaryIds"
        @update:selected-ids="selectedAuxiliaryIds = $event"
      />
    </div>

    <template #footer>
      <AppButton v-if="step === 'picker'" variant="ghost" size="sm" @click="step = 'form'">
        <Icon icon="mdi:arrow-left" width="14" />
        上一步
      </AppButton>
      <AppButton variant="ghost" size="sm" @click="emit('cancel')">取消</AppButton>
      <AppButton v-if="step === 'form'" variant="primary" size="sm" :disabled="!label.trim()" @click="nextStep">
        下一步
        <Icon icon="mdi:arrow-right" width="14" />
      </AppButton>
      <AppButton v-else variant="primary" size="sm" @click="handleSubmit">
        确认创建
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.form-fields {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.form-field-row {
  display: flex;
  align-items: center;
}

.form-label {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  letter-spacing: 0.02em;
}

.required-mark {
  color: var(--color-accent);
}

.step-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.step-badge {
  font-size: 0.62rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: var(--color-accent-subtle);
  color: var(--color-text-secondary);
}

.step-text {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}
</style>
