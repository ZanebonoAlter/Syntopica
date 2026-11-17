<script setup lang="ts">
/**
 * 观察范围精确表单（rework-workshop-into-radar-and-desk 矩阵阶段 G2，ObservationRangeControls 子块）。
 *
 * 批准原型 range-controls 的表单行为产品化：YYYY-MM-DD 严格文本输入（G1
 * observationRange.validateObservationRange 四类文案，非法不静默纠正不提交）+ 应用按钮 +
 * 平移/缩放按钮（离散动作，每次点击即一次完成提交）。字段随上层 range 回流实时同步
 * （拖动过程中跟随显示，原型 paint 同构）；本组件不写 URL、不持业务名单。
 */
import { ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import {
  shiftRange,
  validateObservationRange,
  zoomRange,
  type DateBounds,
  type ObservationRange,
} from './observationRange'

const props = defineProps<{
  bounds: DateBounds
  range: ObservationRange
}>()

const emit = defineEmits<{
  /** 表单应用/平移/缩放完成（父走默认提交写 URL）。 */
  commit: [range: ObservationRange]
  /** 校验问题文案（null = 清除）；展示宿主在反馈行呈现。 */
  issue: [message: string | null]
}>()

const startText = ref(props.range.start)
const endText = ref(props.range.end)

watch(
  () => props.range,
  (range) => {
    startText.value = range.start
    endText.value = range.end
    emit('issue', null)
  },
)

function applyForm(): void {
  const result = validateObservationRange(startText.value, endText.value, props.bounds)
  if (!result.ok) {
    emit('issue', result.issue.message) // 不静默纠正、不误提交
    return
  }
  emit('issue', null)
  emit('commit', result.range)
}

function panBy(direction: -1 | 1): void {
  emit('commit', shiftRange(props.range, direction, props.bounds))
}

function zoomBy(factor: number): void {
  emit('commit', zoomRange(props.range, factor, props.bounds))
}
</script>

<template>
  <form class="orf" novalidate @submit.prevent="applyForm">
    <label class="orf-field" for="orc-start">起始日 · YYYY-MM-DD</label>
    <AppInput
      v-model="startText"
      input-id="orc-start"
      placeholder="YYYY-MM-DD"
      maxlength="10"
      class="orf-input"
    />
    <span class="orf-dash" aria-hidden="true">—</span>
    <label class="orf-field" for="orc-end">截止日 · YYYY-MM-DD</label>
    <AppInput
      v-model="endText"
      input-id="orc-end"
      placeholder="YYYY-MM-DD"
      maxlength="10"
      class="orf-input"
    />
    <AppButton type="submit" variant="secondary" size="sm" class="orf-apply">应用范围</AppButton>
    <div class="orf-nav">
      <button type="button" class="orf-chip" aria-label="向前平移一个视窗" @click="panBy(-1)">← 平移</button>
      <button type="button" class="orf-chip" aria-label="向后平移一个视窗" @click="panBy(1)">平移 →</button>
      <button
        type="button"
        class="orf-chip orf-chip--icon"
        aria-label="放大视窗（宽度减半）"
        title="放大视窗（宽度减半）"
        @click="zoomBy(0.5)"
      >
        <svg viewBox="0 0 20 20" aria-hidden="true" focusable="false">
          <circle cx="8.5" cy="8.5" r="5.6" fill="none" stroke="currentColor" stroke-width="1.7" />
          <path d="M8.5 5.9v5.2M5.9 8.5h5.2" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          <path d="M12.9 12.9 17.2 17.2" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
      </button>
      <button
        type="button"
        class="orf-chip orf-chip--icon"
        aria-label="缩小视窗（宽度加倍）"
        title="缩小视窗（宽度加倍）"
        @click="zoomBy(2)"
      >
        <svg viewBox="0 0 20 20" aria-hidden="true" focusable="false">
          <circle cx="8.5" cy="8.5" r="5.6" fill="none" stroke="currentColor" stroke-width="1.7" />
          <path d="M5.9 8.5h5.2" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          <path d="M12.9 12.9 17.2 17.2" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
      </button>
    </div>
  </form>
</template>

<style scoped>
.orf {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  margin-top: 10px;
}

.orf-field {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.orf-input {
  width: clamp(140px, 11vw, 210px);
}

.orf-dash {
  color: var(--color-text-muted);
}

.orf-apply {
  white-space: nowrap;
}

.orf-nav {
  display: flex;
  gap: 5px;
  margin-left: auto;
  flex-wrap: wrap;
}

.orf-chip {
  border: 1px solid var(--color-border-medium);
  background: transparent;
  color: var(--color-text-secondary);
  padding: 3px 10px;
  font-size: 0.85em;
  cursor: pointer;
  white-space: nowrap;
}

.orf-chip--icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 8px;
  line-height: 0;
}

.orf-chip--icon svg {
  width: 20px;
  height: 20px;
  display: block;
}

.orf-chip:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

@media (max-width: 767.98px) {
  .orf { gap: 8px; }

  .orf-field {
    flex: 1;
    min-width: 0;
  }

  .orf-input { width: 100%; }
  .orf-dash { display: none; }

  .orf-nav {
    width: 100%;
    margin-left: 0;
  }
}
</style>
