<script setup lang="ts">
import { Icon } from '@iconify/vue'

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
  addFeed: []
  addCategory: []
  importOpml: []
  exportOpml: []
  settings: []
  closeRefreshMessage: []
}>()

import '~/components/layout/AppHeader.css'
</script>

<template>
  <header class="app-header">
    <div class="header-left">
      <div class="logo-container">
        <button class="menu-btn" @click="$emit('toggleSidebar')">
          <Icon icon="mdi:menu" width="20" height="20" class="text-gray-600" />
        </button>
        <div class="logo">
          <div class="logo-icon">
            <Icon icon="mdi:rss" class="text-white" width="20" height="20" />
          </div>
          <span class="logo-text">RSS Reader</span>
        </div>
      </div>
    </div>

    <div class="header-right">
      <button class="header-btn" title="刷新" @click="$emit('refresh')">
        <Icon icon="mdi:refresh" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="全部标为已读" @click="$emit('markAllRead')">
        <Icon icon="mdi:email-open-multiple" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="添加订阅" @click="$emit('addFeed')">
        <Icon icon="mdi:plus" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="添加分类" @click="$emit('addCategory')">
        <Icon icon="mdi:folder-plus" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="导入" @click="$emit('importOpml')">
        <Icon icon="mdi:import" width="20" height="20" class="text-gray-600" />
      </button>
      <button class="header-btn" title="导出" @click="$emit('exportOpml')">
        <Icon icon="mdi:export" width="20" height="20" class="text-gray-600" />
      </button>
      <div class="header-divider" />
      <button class="header-btn" title="设置" @click="$emit('settings')">
        <Icon icon="mdi:cog" width="20" height="20" class="text-gray-600" />
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
