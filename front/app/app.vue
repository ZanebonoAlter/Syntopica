<script setup lang="ts">
import { Icon } from '@iconify/vue'

// 初始化主题系统
useTheme()

// 通知中心常驻订阅（client-only：角标对账 + WS notification 事件，不随组件卸载断开）
useNotifications().ensureStarted()

const apiStore = useApiStore()
const loading = ref(true)
const error = ref<string | null>(null)

// 拟真进度 + 游戏风短句（loading-progress-tips）：ease-out 爬升≤90%，finish 后 100%
const { progress, finished, tip, start, finish, dispose } = useFakeProgress()

// 100% 完成态短暂停留再卸载（避免进度条闪跳）；error 分支立即切换不停留
let finishHoldTimer: ReturnType<typeof setTimeout> | null = null
watch(finished, (done) => {
  if (!done) return
  finishHoldTimer = setTimeout(() => {
    finishHoldTimer = null
    loading.value = false
  }, 250)
})

function teardownProgress() {
  dispose()
  if (finishHoldTimer !== null) {
    clearTimeout(finishHoldTimer)
    finishHoldTimer = null
  }
}

onMounted(async () => {
  start()
  try {
    await apiStore.initialize()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载数据失败'
    console.error('初始化错误:', e)
    // 失败路径：停止进度推进（不假完成 100%），直接切错误屏
    teardownProgress()
    loading.value = false
    return
  }
  // 成功路径：watch(finished) 在 100% 停留 250ms 后卸载加载屏
  finish()
})

onUnmounted(teardownProgress)
</script>

<template>
  <!-- 路由切换/页面 chunk 懒加载期间的顶部进度条（spa-loading-ux）
       颜色跟随主题 accent 令牌，双主题均可见 -->
  <NuxtLoadingIndicator :color="'var(--color-accent)'" :height="2" />

  <!-- 路由切换 >250ms 未完成的居中加载反馈（fix-spa-nav-loading-ux D1）：
       状态由 plugins/nav-loading.ts 驱动，空闲时不渲染任何内容 -->
  <NavLoadingOverlay />

  <div v-if="loading" class="h-dvh flex items-center justify-center">
    <div class="text-center">
      <Icon icon="mdi:loading" width="48" height="48" class="animate-spin mx-auto mb-4" style="color: var(--color-text-secondary)" />
      <!-- 游戏风加载短句：随机一条，长加载约 4s 轮换；aria-live 播报首条与轮换 -->
      <p aria-live="polite" style="color: var(--color-text-secondary)">{{ tip }}</p>
      <!-- 拟真进度条：爬升≤90%，finish 后 100%；reduced-motion 下宽度跳变直显 -->
      <div
        class="mt-4 mx-auto w-[min(280px,60vw)] h-1 rounded-full overflow-hidden"
        style="background: color-mix(in srgb, var(--color-text-secondary) 20%, transparent)"
        role="progressbar"
        :aria-valuenow="Math.round(progress)"
        aria-valuemin="0"
        aria-valuemax="100"
      >
        <div
          class="h-full rounded-full transition-[width] duration-200 ease-out motion-reduce:transition-none"
          :style="{ width: progress + '%', background: 'var(--color-accent)' }"
        />
      </div>
      <p class="mt-2 text-xs tabular-nums" style="color: var(--color-text-secondary)">{{ Math.round(progress) }}%</p>
    </div>
  </div>

  <div v-else-if="error" class="h-dvh flex items-center justify-center">
    <div class="text-center max-w-md w-full max-md:px-6 break-words">
      <Icon icon="mdi:alert-circle" width="48" height="48" class="text-[var(--color-error)] mx-auto mb-4" />
      <h2 class="text-xl font-bold mb-2" style="color: var(--color-text-primary)">加载失败</h2>
      <p class="mb-4" style="color: var(--color-text-secondary)">{{ error }}</p>
      <AppButton variant="primary" class="max-md:w-full" @click="$router.go(0)">
        重新加载
      </AppButton>
    </div>
  </div>

  <NuxtPage v-else />

  <!-- AI 模型未就绪全局提示（用户意图运行但健康门未通过时） -->
  <AiHealthBanner />

  <!-- 全局 Toast 通知 -->
  <NotifyContainer />
</template>
