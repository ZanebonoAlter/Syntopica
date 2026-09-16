<script setup lang="ts">
import { Icon } from '@iconify/vue'

// 全局错误兜底页（spa-loading-ux）：接管路由级/未捕获错误（如弱网下页面
// chunk 加载失败），替代白屏；局部可恢复错误仍走 AiHealthBanner/Toast。
// 视觉与 app.vue 初始化错误态同构（ui-design.md 复用契约）。
const error = useError()

const isChunkError = computed(() => {
  const msg = error.value?.message ?? ''
  // 动态 import 失败：浏览器差异文案（Chrome/Firefox/Safari）
  return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module/i.test(msg)
})

function handleReload() {
  if (isChunkError.value || error.value?.statusCode === undefined) {
    // chunk/网络类失败：刷新页面重走首屏加载（拉新 chunk）
    window.location.reload()
  } else {
    // 应用内错误：回首页并清除错误态
    clearError({ redirect: '/' })
  }
}
</script>

<template>
  <div class="h-screen flex items-center justify-center">
    <div class="text-center max-w-md">
      <Icon icon="mdi:alert-circle" width="48" height="48" class="mx-auto mb-4" style="color: var(--color-error)" />
      <h2 class="text-xl font-bold mb-2" style="color: var(--color-text-primary)">页面加载失败</h2>
      <p class="mb-4" style="color: var(--color-text-secondary)">
        {{ isChunkError ? '网络不稳定，资源加载失败，请检查网络后重试' : (error?.message || '发生未知错误') }}
      </p>
      <AppButton variant="primary" @click="handleReload">
        重新加载
      </AppButton>
    </div>
  </div>
</template>
