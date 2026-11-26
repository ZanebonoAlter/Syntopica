<script setup lang="ts">
import { computed } from 'vue'
import katex from 'katex'
import 'katex/dist/katex.min.css'

const props = withDefaults(defineProps<{
  latex: string
  display?: boolean
}>(), {
  display: false,
})

const html = computed(() => katex.renderToString(props.latex, {
  displayMode: props.display,
  throwOnError: false,
  strict: false,
}))
</script>

<template>
  <span class="katex-render" :class="{ 'katex-render--display': props.display }" v-html="html" />
</template>

<style scoped>
.katex-render {
  /* 主题 token：白天（editorial）深字浅底、夜间浅字深底——曾写死近白色导致白天模式公式不可见（2026-11-03 用户实机报障） */
  color: var(--color-text-primary);
}

.katex-render--display {
  display: block;
  overflow-x: auto;
  padding: 0.35rem 0;
}
</style>
