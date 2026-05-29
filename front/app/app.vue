<script setup lang="ts">
import { Icon } from '@iconify/vue'

const apiStore = useApiStore()
const loading = ref(true)
const error = ref<string | null>(null)

onMounted(async () => {
  try {
    await apiStore.initialize()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载数据失败'
    console.error('初始化错误:', e)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="h-screen flex items-center justify-center">
    <div class="text-center">
      <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-ink-600 mx-auto mb-4" />
      <p class="text-ink-medium">正在加载...</p>
    </div>
  </div>

  <div v-else-if="error" class="h-screen flex items-center justify-center">
    <div class="text-center max-w-md">
      <Icon icon="mdi:alert-circle" width="48" height="48" class="text-[var(--color-error)] mx-auto mb-4" />
      <h2 class="text-xl font-bold text-ink-dark mb-2">加载失败</h2>
      <p class="text-ink-medium mb-4">{{ error }}</p>
      <button class="btn-primary" @click="$router.go(0)">
        重新加载
      </button>
    </div>
  </div>

  <NuxtPage v-else />
</template>
