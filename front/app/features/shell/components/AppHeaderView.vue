<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import { useTheme } from '~/composables/useTheme'
import { useOnboarding } from '~/composables/useOnboarding'
import { useSchedulerStatus } from '~/composables/useSchedulerStatus'
import { useAnalysisPauseFavicon } from '~/composables/useAnalysisPauseFavicon'
import { useNotify } from '~/composables/useNotify'
import NotificationBell from '~/components/ui/NotificationBell.vue'
import TagQueueProgressChip from './TagQueueProgressChip.vue'

const { toggleTheme, isDark } = useTheme()
const { startTour } = useOnboarding()
const { analysisPaused, aiHealthy, loadSchedulersStatus, setAnalysisPaused } = useSchedulerStatus()
useAnalysisPauseFavicon()
const notify = useNotify()
const analysisPauseToggling = ref(false)

async function toggleAnalysisPause() {
  if (analysisPauseToggling.value) return
  analysisPauseToggling.value = true
  try {
    const result = await setAnalysisPaused(!analysisPaused.value)
    if (result.ok) {
      notify.success(result.message)
    } else {
      notify.error(result.message)
    }
  } finally {
    analysisPauseToggling.value = false
  }
}

function goToAiHealthSettings() {
  return navigateTo('/settings?section=ai-health')
}

onMounted(() => {
  loadSchedulersStatus()
})

interface Props {
  showRefreshMessage?: boolean
  refreshMessage?: string
  refreshMessageType?: 'success' | 'error' | 'info'
}

withDefaults(defineProps<Props>(), {
  showRefreshMessage: false,
  refreshMessage: '',
  refreshMessageType: 'info'
})

defineEmits<{
  toggleSidebar: []
  refresh: []
  markAllRead: []
  settings: []
  closeRefreshMessage: []
}>()

import '~/components/layout/AppHeader.css'
</script>

<style scoped>
.ai-health-icon--ok {
  color: var(--color-success);
}

.ai-health-icon--down {
  color: var(--color-warning);
}
</style>

<template>
  <header class="app-header">
    <div class="header-left">
      <div class="logo-container">
        <button class="menu-btn" @click="$emit('toggleSidebar')">
          <Icon icon="mdi:menu" width="20" height="20" class="text-gray-600" />
        </button>
        <div class="logo">
          <div class="logo-icon">
            <img src="/favicon.png" alt="Syntopica" width="32" height="32" />
          </div>
          <span class="logo-text">Syntopica</span>
        </div>
      </div>
    </div>

    <div class="header-right">
      <button
        class="header-btn"
        :title="analysisPaused ? '分析已暂停 · 点击恢复' : '暂停分析'"
        :disabled="analysisPauseToggling"
        @click="toggleAnalysisPause"
      >
        <Icon
          :icon="analysisPaused ? 'mdi:play' : 'mdi:pause'"
          width="20"
          height="20"
          :class="analysisPaused ? 'text-amber-500' : 'text-gray-600'"
        />
      </button>
      <button
        class="header-btn ai-health-btn"
        :title="aiHealthy ? 'AI 模型健康' : 'AI 模型未就绪（LLM/Embedding 未连通）'"
        @click="goToAiHealthSettings"
      >
        <Icon
          icon="mdi:heart-pulse"
          width="20"
          height="20"
          :class="aiHealthy ? 'ai-health-icon--ok' : 'ai-health-icon--down'"
        />
      </button>
      <button class="header-btn" title="刷新" @click="$emit('refresh')">
        <Icon icon="mdi:refresh" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="全部标为已读" @click="$emit('markAllRead')">
        <Icon icon="mdi:email-open-multiple" width="20" height="20" class="text-gray-600" />
      </button>
      <!-- 标签队列进度芯片：铃铛左侧（任务态可见，空闲不渲染） -->
      <TagQueueProgressChip />
      <!-- 通知铃铛（notification-center）：全部已读与分隔线之间 -->
      <NotificationBell />
      <div class="header-divider" />
      <button class="header-btn" title="设置" @click="$emit('settings')">
        <Icon icon="mdi:cog" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="新手引导" @click="startTour">
        <Icon icon="mdi:compass-outline" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" :title="isDark ? '切换为浅色模式' : '切换为深色模式'" @click="toggleTheme">
        <Icon :icon="isDark ? 'mdi:white-balance-sunny' : 'mdi:weather-night'" width="20" height="20" class="text-gray-600" />
      </button>
    </div>
  </header>

  <transition
    enter-active-class="transition ease-out duration-300"
    enter-from-class="transform opacity-0 translate-y-2"
    enter-to-class="transform opacity-100 translate-y-0"
    leave-active-class="transition ease-in duration-200"
    leave-from-class="transform opacity-100 translate-y-0"
    leave-to-class="transform opacity-0 translate-y-2"
  >
    <div
      v-if="showRefreshMessage"
      class="refresh-toast"
    >
      <div
        class="toast-content"
        :class="`toast-${refreshMessageType}`"
      >
        <Icon
          :icon="
            refreshMessageType === 'success'
              ? 'mdi:check-circle'
              : refreshMessageType === 'error'
                ? 'mdi:alert-circle'
                : 'mdi:information'
          "
          width="22"
          height="22"
          :class="`icon-${refreshMessageType}`"
        />
        <span class="toast-message">{{ refreshMessage }}</span>
        <button
          class="toast-close"
          @click="$emit('closeRefreshMessage')"
        >
          <Icon icon="mdi:close" width="18" height="18" class="text-gray-400" />
        </button>
      </div>
    </div>
  </transition>
</template>
