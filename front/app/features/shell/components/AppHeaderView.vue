<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
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

/** 窄屏（<768px）「⋯」溢出菜单：点按钮开合，点外部关闭（document click + onUnmounted 清理） */
const overflowOpen = ref(false)
const overflowRoot = ref<HTMLElement | null>(null)

function toggleOverflow() {
  overflowOpen.value = !overflowOpen.value
}

function onDocumentClick(e: MouseEvent) {
  if (!overflowOpen.value) return
  if (overflowRoot.value && !overflowRoot.value.contains(e.target as Node)) {
    overflowOpen.value = false
  }
}

/** 溢出菜单项：关菜单后执行原 handler（宽屏/窄屏共用同一行为入口；handler 返回值忽略） */
function runOverflowItem(handler: () => unknown) {
  overflowOpen.value = false
  void handler()
}

onMounted(() => {
  loadSchedulersStatus()
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
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

const emit = defineEmits<{
  toggleSidebar: []
  refresh: []
  markAllRead: []
  settings: []
  closeRefreshMessage: []
  /** 窄屏汉堡按钮：打开移动端导航抽屉（AppSidebarDrawer）；宽屏不触发（按钮 display:none） */
  openDrawer: []
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

/* ── 窄屏降级（mobile-viewport-stage1）──
   宽屏零变化红线：新增元素默认 display:none（或 display:contents 不产生盒子），
   仅在 @media (max-width: 767.98px)（< Tailwind md）媒体块内显示。 */
.drawer-menu-btn {
  display: none;
}

/* 宽屏包裹层不生成盒子：按钮仍是 .header-right 的直接 flex item，布局与重构前逐像素一致 */
.wide-only-group {
  display: contents;
}

.mobile-overflow {
  position: relative;
  display: none;
}

.overflow-menu {
  position: absolute;
  top: calc(100% + 4px);
  right: 0;
  z-index: 60;
  min-width: 176px;
  padding: 4px;
  display: flex;
  flex-direction: column;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 0.5rem;
  box-shadow: var(--shadow-strong);
}

.overflow-item {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.5rem 0.625rem;
  border: none;
  border-radius: 0.375rem;
  background: transparent;
  font-size: 0.8125rem;
  color: var(--color-text-primary);
  cursor: pointer;
  text-align: left;
  white-space: nowrap;
}

.overflow-item:hover {
  background: var(--color-bg-hover);
}

/* 窄屏：顶栏压到 56px 单行，汉堡/「⋯」显示，宽屏次要操作整组隐藏 */
@media (max-width: 767.98px) {
  .app-header {
    height: 56px;
    padding: 0 0.75rem;
  }

  /* 溢出菜单层级修复：backdrop-filter 令 .app-header 自成 stacking context，
     内部 z:60 的 .overflow-menu 被锁在本层，与 transform 虚拟列表（DOM 靠后）互叠时被盖。
     窄屏把整个 header 提到内容面板之上（抽屉 scrim 1000 / 面板 1001 仍在其上）。 */
  .app-header {
    position: relative;
    z-index: 900;
  }

  /* 375 顶栏预算：汉堡(40)+进度chip(≤160)+铃铛(40)+溢出(40)+间隙/padding≈满宽，
     logo 及宽屏 toggleSidebar 汉堡让位（防 flex 溢出互叠——logo-container min-content 不收缩） */
  .logo-container {
    display: none;
  }

  .drawer-menu-btn {
    display: flex;
  }

  .mobile-overflow {
    display: block;
  }

  .wide-only-group {
    display: none;
  }
}
</style>

<template>
  <header class="app-header">
    <div class="header-left">
      <!-- 窄屏汉堡：打开移动端抽屉（宽屏 display:none，与宽屏 toggleSidebar 按钮语义隔离） -->
      <button class="menu-btn drawer-menu-btn" title="打开导航" aria-label="打开导航菜单" @click="emit('openDrawer')">
        <Icon icon="mdi:menu" width="20" height="20" class="text-gray-600" />
      </button>
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
      <!-- 宽屏次要操作组 1：宽屏 display:contents 保持原布局，窄屏整组 display:none -->
      <div class="wide-only-group">
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
      </div>
      <!-- 标签队列进度芯片：铃铛左侧（任务态可见，空闲不渲染） -->
      <TagQueueProgressChip />
      <!-- 通知铃铛（notification-center）：全部已读与分隔线之间 -->
      <NotificationBell />
      <!-- 宽屏次要操作组 2：窄屏整组 display:none（divider/设置/引导/主题收进溢出菜单） -->
      <div class="wide-only-group">
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
      <!-- 窄屏「⋯」溢出菜单（宽屏 display:none）：刷新/主题类次要操作收纳 -->
      <div ref="overflowRoot" class="mobile-overflow">
        <button class="header-btn" title="更多操作" @click.stop="toggleOverflow">
          <Icon icon="mdi:dots-horizontal" width="20" height="20" class="text-gray-600" />
        </button>
        <div v-if="overflowOpen" class="overflow-menu">
          <button class="overflow-item" @click="runOverflowItem(toggleAnalysisPause)">
            <Icon :icon="analysisPaused ? 'mdi:play' : 'mdi:pause'" width="18" height="18" />
            <span>{{ analysisPaused ? '恢复分析' : '暂停分析' }}</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(goToAiHealthSettings)">
            <Icon
              icon="mdi:heart-pulse"
              width="18"
              height="18"
              :class="aiHealthy ? 'ai-health-icon--ok' : 'ai-health-icon--down'"
            />
            <span>AI 模型健康</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(() => emit('refresh'))">
            <Icon icon="mdi:refresh" width="18" height="18" />
            <span>刷新</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(() => emit('markAllRead'))">
            <Icon icon="mdi:email-open-multiple" width="18" height="18" />
            <span>全部标为已读</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(() => emit('settings'))">
            <Icon icon="mdi:cog" width="18" height="18" />
            <span>设置</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(startTour)">
            <Icon icon="mdi:compass-outline" width="18" height="18" />
            <span>新手引导</span>
          </button>
          <button class="overflow-item" @click="runOverflowItem(toggleTheme)">
            <Icon :icon="isDark ? 'mdi:white-balance-sunny' : 'mdi:weather-night'" width="18" height="18" />
            <span>{{ isDark ? '切换为浅色模式' : '切换为深色模式' }}</span>
          </button>
        </div>
      </div>
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
