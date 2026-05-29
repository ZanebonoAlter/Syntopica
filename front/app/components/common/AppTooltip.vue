<script setup lang="ts">
import { useFloating } from '@floating-ui/vue'
import { autoUpdate, offset, shift } from '@floating-ui/dom'

interface Props {
  content?: string
  disabled?: boolean
  placement?: 'top' | 'bottom' | 'left' | 'right'
  delay?: number
}

const props = withDefaults(defineProps<Props>(), {
  content: '',
  disabled: false,
  placement: 'top',
  delay: 400,
})

const triggerRef = ref<HTMLElement>()
const floatingRef = ref<HTMLElement>()

const { floatingStyles } = useFloating(triggerRef, floatingRef, {
  placement: props.placement,
  middleware: [
    offset(8),
    shift(),
  ],
  whileElementsMounted: autoUpdate,
})

const isVisible = ref(false)
let timeoutId: ReturnType<typeof setTimeout> | null = null

function show() {
  if (props.disabled || !props.content) return
  timeoutId = setTimeout(() => {
    isVisible.value = true
  }, props.delay)
}

function hide() {
  if (timeoutId) {
    clearTimeout(timeoutId)
    timeoutId = null
  }
  isVisible.value = false
}

function onTriggerEnter() {
  show()
}

function onTriggerLeave() {
  hide()
}

function onFloatingEnter() {
  if (timeoutId) {
    clearTimeout(timeoutId)
    timeoutId = null
  }
}

function onFloatingLeave() {
  hide()
}
</script>

<template>
  <span
    ref="triggerRef"
    @mouseenter="onTriggerEnter"
    @mouseleave="onTriggerLeave"
  >
    <slot />
  </span>

  <Teleport to="body">
    <div
      v-if="isVisible"
      ref="floatingRef"
      class="app-tooltip"
      :style="floatingStyles"
      @mouseenter="onFloatingEnter"
      @mouseleave="onFloatingLeave"
    >
      {{ content }}
    </div>
  </Teleport>
</template>

<style scoped>
.app-tooltip {
  z-index: 9999;
  background: rgb(15, 23, 42);
  color: white;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 400;
  max-width: 280px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
  pointer-events: auto;
}

.app-tooltip::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 6px;
  border: 1px solid rgb(255 255 255 / 0.1);
}
</style>
