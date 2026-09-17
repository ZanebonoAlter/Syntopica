<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { PipelineLine } from '../composables/useArticleProcessingStatus'

interface Props {
  /** 标题；失败态传「抓取失败/总结失败」并配 error tone，否则「处理详情」 */
  title?: string
  titleTone?: 'error' | 'neutral'
  lines: PipelineLine[]
  /** 失败错误文案（默认完整可见，超长滚动） */
  error?: string
  hint?: string
}

withDefaults(defineProps<Props>(), {
  title: '处理详情',
  titleTone: 'neutral',
  error: '',
  hint: '',
})

const emit = defineEmits<{ close: [] }>()

/** 底边空间不足时向上翻（虚拟列表 overflow 容器会裁剪向下溢出的浮层，design D2） */
const dropUp = ref(false)
const rootRef = ref<HTMLElement | null>(null)

onMounted(() => {
  const el = rootRef.value
  const scroller = el?.closest('.virtual-list') as HTMLElement | null
  if (!el || !scroller) return
  const er = el.getBoundingClientRect()
  const sr = scroller.getBoundingClientRect()
  if (er.bottom > sr.bottom - 4 && er.height < er.top - sr.top) {
    dropUp.value = true
  }
})
</script>

<template>
  <div ref="rootRef" class="row-status-popover" :class="{ 'rsp-up': dropUp }" role="dialog" aria-label="处理详情" @click.stop>
    <div class="rsp-title" :class="`rsp-title-${titleTone}`">
      <Icon :icon="titleTone === 'error' ? 'mdi:alert-circle' : 'mdi:information-slab-circle'" width="13" height="13" />
      <span>{{ title }}</span>
      <button class="rsp-close" aria-label="关闭" @click="$emit('close')">
        <Icon icon="mdi:close" width="12" height="12" />
      </button>
    </div>
    <div class="rsp-body">
      <div v-for="line in lines" :key="line.label" class="rsp-line">
        <span class="rsp-label">{{ line.label }}</span>
        <span class="rsp-value" :class="`rsp-value-${line.tone}`">{{ line.value }}</span>
      </div>
      <div v-if="error" class="rsp-error">{{ error }}</div>
    </div>
    <div v-if="hint" class="rsp-hint">{{ hint }}</div>
  </div>
</template>
