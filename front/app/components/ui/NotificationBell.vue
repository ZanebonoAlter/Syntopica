<script setup lang="ts">
import { Icon } from '@iconify/vue'
import NotificationPanel from './NotificationPanel.vue'
import { useNotifications } from '~/composables/useNotifications'
import { useSchedulerStatus } from '~/composables/useSchedulerStatus'

/**
 * 通知铃铛（notification-center）：未读角标 + 点击开关锚定面板
 * ui-design：位置在「全部标为已读」与分隔线之间；>99 显示 99+
 */

const open = ref(false)
const { unreadCount, openPanel, closePanel } = useNotifications()

/**
 * AI 未就绪警示态（ai-health-to-notifications）：意图运行但健康门未通过时切
 * 警示图标/配色，与未读角标正交叠加（角标逻辑/数值完全不动，D3）。
 * 可见性与面板置顶条一致：用户主动暂停（analysisPaused=true）不警示。
 */
const { analysisPaused, aiHealthy } = useSchedulerStatus()
const aiUnready = computed(() => !analysisPaused.value && !aiHealthy.value)

const badgeText = computed(() => {
  if (unreadCount.value <= 0) return ''
  return unreadCount.value > 99 ? '99+' : String(unreadCount.value)
})

async function toggle() {
  if (open.value) {
    open.value = false
    closePanel()
  } else {
    open.value = true
    await openPanel()
  }
}

function onOutsideClick(e: MouseEvent) {
  if (!open.value) return
  const target = e.target
  // 铃铛 wrapper + 面板（Teleport 到 body，根类 .notif-panel）都不算“外”
  // Element 防御：happy-dom 下 document 自身派发的事件 target 无 closest
  if (!(target instanceof Element)) return
  if (target.closest('.notif-bell-wrap, .notif-panel')) return
  open.value = false
  closePanel()
}

function onEscape(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    open.value = false
    closePanel()
  }
}

onMounted(() => {
  document.addEventListener('click', onOutsideClick)
  document.addEventListener('keydown', onEscape)
})

onUnmounted(() => {
  document.removeEventListener('click', onOutsideClick)
  document.removeEventListener('keydown', onEscape)
})
</script>

<template>
  <div class="notif-bell-wrap">
    <button
      class="header-btn"
      data-testid="notification-bell"
      :title="aiUnready ? '通知（AI 模型未就绪）' : '通知'"
      @click="toggle"
    >
      <Icon
        :icon="aiUnready ? 'mdi:bell-alert' : 'mdi:bell-outline'"
        width="20"
        height="20"
        :class="aiUnready ? 'notif-bell--warning' : 'text-gray-600'"
      />
      <span
        v-if="badgeText"
        class="notif-badge"
        data-testid="notification-badge"
      >{{ badgeText }}</span>
    </button>
    <NotificationPanel v-if="open" @close="open = false; closePanel()" />
  </div>
</template>

<style scoped>
.notif-bell-wrap {
  position: relative;
  display: inline-flex;
}

.notif-bell--warning {
  color: var(--color-warning);
}

.notif-badge {
  position: absolute;
  top: 2px;
  right: 0;
  min-width: 1rem;
  height: 1rem;
  padding: 0 0.25rem;
  background: var(--color-accent);
  color: var(--color-text-inverted, #fff);
  border-radius: 0.5rem;
  font-size: 0.625rem;
  font-weight: 700;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px solid var(--color-bg-elevated);
  box-sizing: content-box;
}
</style>
