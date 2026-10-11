<script setup lang="ts">
/**
 * 思考档下拉（ai-provider-model-overhaul 4.3 交付 1，specs/ai-thinking-levels）。
 * 空值=继承模型默认档（选项文案带模型默认档中文，未配置回退全局默认 medium）；
 * 模型未声明 supports_xhigh_max 时不暴露 xhigh/max 两档（可用性钳制）。
 * 选中非继承档时 emit('override')，父层（链项）据 modelValue 渲染「覆盖」徽章。
 */
import { computed } from 'vue'
import type { ThinkingLevel } from '~/types'

const ALL_LEVELS: ThinkingLevel[] = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']
const LEVEL_LABELS: Record<ThinkingLevel, string> = {
  off: '关闭',
  minimal: '微',
  low: '低',
  medium: '中',
  high: '高',
  xhigh: '超高',
  max: '极',
}

const props = withDefaults(defineProps<{
  modelValue: ThinkingLevel | null
  /** 该模型默认档（继承项显示「模型默认 N」；null 回退全局默认 medium） */
  modelDefault?: ThinkingLevel | null
  /** 模型未声明支持 xhigh/max 时下拉不出现这两档（spec 钳制） */
  supportsXhighMax?: boolean
  disabled?: boolean
}>(), {
  modelDefault: null,
  supportsXhighMax: true,
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: ThinkingLevel | null]
  /** 选中非继承档时触发（父层显示「覆盖」徽章用） */
  override: [value: ThinkingLevel]
}>()

const levels = computed<ThinkingLevel[]>(() =>
  props.supportsXhighMax ? ALL_LEVELS : ALL_LEVELS.filter(lv => lv !== 'xhigh' && lv !== 'max'),
)

const inheritLabel = computed(() => `继承（模型默认 ${LEVEL_LABELS[props.modelDefault ?? 'medium']}）`)

function onChange(event: Event) {
  const raw = (event.target as HTMLSelectElement).value
  const value: ThinkingLevel | null = raw === '' ? null : (raw as ThinkingLevel)
  emit('update:modelValue', value)
  if (value !== null) emit('override', value)
}
</script>

<template>
  <select
    class="ai-select thinking-level-select"
    :value="modelValue ?? ''"
    :disabled="disabled"
    title="思考档（留空继承模型默认档）"
    @change="onChange"
  >
    <option value="">{{ inheritLabel }}</option>
    <option v-for="lv in levels" :key="lv" :value="lv">{{ LEVEL_LABELS[lv] }}</option>
  </select>
</template>

<style scoped>
.thinking-level-select {
  min-width: 88px;
}
</style>
