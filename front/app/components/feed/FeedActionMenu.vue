<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useFloating } from '@floating-ui/vue'
import { autoUpdate, offset, shift, flip } from '@floating-ui/dom'
import AppTooltip from '~/components/common/AppTooltip.vue'

interface Props {
  feedId: string
  feedTitle: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  markAsRead: [feedId: string]
  edit: [feedId: string]
}>()

const triggerRef = ref<HTMLElement>()
const floatingRef = ref<HTMLElement>()

const isOpen = ref(false)

const { floatingStyles } = useFloating(triggerRef, floatingRef, {
  placement: 'bottom-end',
  middleware: [
    offset(4),
    shift(),
    flip(),
  ],
  whileElementsMounted: autoUpdate,
})

function toggle() {
  isOpen.value = !isOpen.value
}

function close() {
  isOpen.value = false
}

function handleMarkAsRead() {
  emit('markAsRead', props.feedId)
  close()
}

function handleEdit() {
  emit('edit', props.feedId)
  close()
}

function handleOutsideClick(event: MouseEvent) {
  if (isOpen.value && triggerRef.value && floatingRef.value) {
    const target = event.target as Node
    if (!triggerRef.value.contains(target) && !floatingRef.value.contains(target)) {
      close()
    }
  }
}

onMounted(() => {
  document.addEventListener('click', handleOutsideClick)
})

onUnmounted(() => {
  document.removeEventListener('click', handleOutsideClick)
})
</script>

<template>
  <AppTooltip content="更多操作">
    <button
      ref="triggerRef"
      class="feed-action-trigger"
      @click.stop="toggle"
    >
      <Icon icon="mdi:dots-vertical" width="16" height="16" class="text-gray-500" />
    </button>
  </AppTooltip>

  <Teleport to="body">
    <div
      v-if="isOpen"
      ref="floatingRef"
      class="feed-action-menu"
      :style="floatingStyles"
    >
      <button
        class="menu-item"
        @click.stop="handleMarkAsRead"
      >
        <Icon icon="mdi:check-all" width="16" height="16" />
        <span>标记为全部已读</span>
      </button>
      <button
        class="menu-item"
        @click.stop="handleEdit"
      >
        <Icon icon="mdi:pencil" width="16" height="16" />
        <span>编辑订阅源</span>
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.feed-action-trigger {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  border: none;
  background: transparent;
  cursor: pointer;
  color: rgb(107 114 128);
  transition: all 0.15s ease;
}

.feed-action-trigger:hover {
  background: rgb(243 244 246);
  color: rgb(75 85 99);
}

.feed-action-menu {
  z-index: 9999;
  background: white;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgb(0 0 0 / 0.15), 0 2px 4px rgb(0 0 0 / 0.1);
  border: 1px solid rgb(229 231 235);
  min-width: 180px;
  overflow: hidden;
  padding: 4px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 12px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 400;
  color: rgb(31 41 55);
  text-align: left;
  transition: background 0.1s ease;
}

.menu-item:hover {
  background: rgb(243 244 246);
}

.menu-item svg {
  color: rgb(107 114 128);
}
</style>
