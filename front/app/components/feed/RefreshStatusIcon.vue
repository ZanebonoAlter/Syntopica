<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { RssFeed } from '~/types'

interface Props {
  feed: RssFeed
}

const props = defineProps<Props>()

const statusIcon = computed(() => {
  switch (props.feed.refreshStatus) {
    case 'refreshing':
      return 'mdi:loading'
    case 'error':
      return 'mdi:alert-circle'
    case 'success':
      return 'mdi:check-circle'
    default:
      return null
  }
})

const statusColor = computed(() => {
  switch (props.feed.refreshStatus) {
    case 'refreshing':
      return 'text-ink-500'
    case 'error':
      return 'text-error'
    case 'success':
      return 'text-success'
    default:
      return ''
  }
})

const statusTitle = computed(() => {
  switch (props.feed.refreshStatus) {
    case 'error':
      return props.feed.refreshError || '刷新失败'
    case 'success':
      return '刷新成功'
    default:
      return ''
  }
})
</script>

<template>
  <Icon
    v-if="statusIcon"
    :icon="statusIcon"
    width="14"
    height="14"
    :class="[statusColor, { 'animate-spin': feed.refreshStatus === 'refreshing' }]"
    :title="statusTitle"
  />
</template>
