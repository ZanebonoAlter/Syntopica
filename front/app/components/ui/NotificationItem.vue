<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { AppNotification } from '~/api/notifications'

/**
 * 单条通知（notification-center）：图标（成功/失败）+ 标题 + 摘要 + 相对时间
 * 未读强调（左点 + 淡底）只在「本次浏览会话」内保留；hover 显「标已读」次操作
 * 超长摘要 line-clamp 截断，不撑破面板
 */

const props = defineProps<{
  item: AppNotification
  highlightUnread: boolean
}>()

const emit = defineEmits<{
  click: []
  'mark-read': []
}>()

/** 相对时间（今昨日常见窗口；更旧显示日期） */
const relativeTime = computed(() => {
  const ts = new Date(props.item.created_at).getTime()
  if (Number.isNaN(ts)) return ''
  const diff = Date.now() - ts
  const min = Math.floor(diff / 60_000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hour = Math.floor(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.floor(hour / 24)
  if (day === 1) return '昨天'
  return new Date(props.item.created_at).toLocaleDateString('zh-CN')
})
</script>

<template>
  <div
    class="notif-item"
    :class="{ 'notif-item--unread': highlightUnread, 'notif-item--error': item.type === 'error' }"
    data-testid="notification-item"
    @click="emit('click')"
  >
    <div class="notif-item__icon" :class="item.type === 'error' ? 'is-error' : 'is-success'">
      <Icon :icon="item.type === 'error' ? 'mdi:alert-circle-outline' : 'mdi:check-circle-outline'" width="15" height="15" />
    </div>
    <div class="notif-item__body">
      <div class="notif-item__title">{{ item.title }}</div>
      <div class="notif-item__desc">{{ item.summary }}</div>
      <div class="notif-item__meta">
        <span class="notif-item__time">{{ relativeTime }}</span>
        <button
          v-if="!item.is_read"
          class="notif-item__mark"
          data-testid="notification-item-mark-read"
          @click.stop="emit('mark-read')"
        >
          标已读
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.notif-item {
  display: flex;
  gap: 0.625rem;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--color-border-subtle);
  cursor: pointer;
  align-items: flex-start;
  position: relative;
}

.notif-item:hover {
  background: var(--color-bg-hover);
}

.notif-item--unread {
  background: var(--color-info-subtle);
}

.notif-item--unread::before {
  content: "";
  position: absolute;
  left: 0.375rem;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--color-accent);
}

.notif-item__icon {
  flex-shrink: 0;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 0.375rem;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 0.125rem;
}

.notif-item__icon.is-success {
  background: var(--color-success-subtle);
  color: var(--color-success);
}

.notif-item__icon.is-error {
  background: var(--color-error-subtle, rgba(196, 47, 60, 0.12));
  color: var(--color-error);
}

.notif-item__body {
  flex: 1;
  min-width: 0;
}

.notif-item__title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--color-text-primary);
  line-height: 1.4;
}

.notif-item--unread .notif-item__title {
  font-weight: 700;
}

.notif-item__desc {
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  line-height: 1.5;
  margin-top: 0.125rem;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.notif-item__meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 0.25rem;
}

.notif-item__time {
  font-size: 0.6875rem;
  color: var(--color-text-muted);
}

.notif-item__mark {
  opacity: 0;
  transition: opacity 0.15s;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--color-text-muted);
  font-size: 0.6875rem;
  padding: 0.125rem;
}

.notif-item:hover .notif-item__mark {
  opacity: 1;
}

.notif-item__mark:hover {
  color: var(--color-text-primary);
}
</style>
