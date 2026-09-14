<script setup lang="ts">
interface Props {
  modelValue?: string | number
  type?: string
  placeholder?: string
  disabled?: boolean
  error?: string
  step?: string | number
  min?: string | number
  max?: string | number
  maxlength?: string | number
  /** 直传内部 input 的 id（label[for] 关联用；attrs 落点在 wrapper div，无法用于关联） */
  inputId?: string
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

function onInput(event: Event) {
  const target = event.target as HTMLInputElement
  if (props.type === 'number') {
    emit('update:modelValue', target.value === '' ? '' : Number(target.value))
  } else {
    emit('update:modelValue', target.value)
  }
}
</script>

<template>
  <div class="app-input-wrapper">
    <input
      :id="inputId"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :step="step"
      :min="min"
      :max="max"
      :maxlength="maxlength"
      class="app-input"
      :class="{ 'is-error': error }"
      @input="onInput"
    />
    <span v-if="error" class="app-input-error">{{ error }}</span>
  </div>
</template>

<style scoped>
.app-input-wrapper {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.app-input {
  padding: 8px 12px;
  font-size: 14px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
  font-family: inherit;
}
.app-input::placeholder {
  color: var(--color-text-muted);
}
.app-input:focus {
  border-color: var(--color-input-focus);
  box-shadow: 0 0 0 2px var(--color-accent-subtle);
}
.app-input.is-error {
  border-color: var(--color-error);
}
.app-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.app-input-error {
  font-size: 12px;
  color: var(--color-error);
}
</style>
