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
  color: rgba(255, 245, 235, 0.88);
}

.katex-render--display {
  display: block;
  overflow-x: auto;
  padding: 0.35rem 0;
}
</style>
