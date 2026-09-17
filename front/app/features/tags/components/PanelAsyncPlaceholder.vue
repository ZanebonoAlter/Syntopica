<script setup lang="ts">
import { Icon } from '@iconify/vue'

// 懒加载面板占位（fix-spa-nav-loading-ux D3）：defineAsyncComponent 的 loadingComponent，
// 异步面板 chunk 就绪前显示；视觉构件与 NavLoadingOverlay 同构（delay=200 防暖切闪烁）。
</script>

<template>
  <div class="panel-async-placeholder" role="status" aria-live="polite">
    <Icon
      icon="mdi:loading"
      width="48"
      height="48"
      class="animate-spin motion-reduce:animate-none"
      style="color: var(--color-accent)"
    />
  </div>
</template>

<style scoped>
.panel-async-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 3rem 0;
}
</style>
