<script setup lang="ts">
import { computed } from 'vue'
import { DIALOG_SIZES, type DialogSize } from './layout-contract'

interface Props {
  modelValue: boolean
  title?: string
  /** 尺寸档（新代码首选）：sm=420 / md=560 / lg=760 / xl=1040，统一受 92vw 约束（standard/frontend/layout.md） */
  size?: DialogSize
  /** @deprecated 存量兼容：自由宽度。新代码禁用，改用 size；仅在未传 size 时生效 */
  width?: string
  closeOnOverlay?: boolean
  closeOnEscape?: boolean
  showClose?: boolean
  zIndex?: number
}

const props = withDefaults(defineProps<Props>(), {
  size: undefined,
  width: '480px',
  closeOnOverlay: true,
  closeOnEscape: true,
  showClose: true,
  zIndex: 1000,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

/** size 优先：注入 --dialog-w 档位变量，宽度由 .is-sized 样式 min(var, 92vw) 统一约束；
 *  旧 width 保持原 maxWidth 行为（渲染兼容，新代码禁用） */
const dialogStyle = computed(() => {
  if (props.size) return { '--dialog-w': `${DIALOG_SIZES[props.size]}px` }
  return { maxWidth: props.width }
})

function close() {
  emit('update:modelValue', false)
}

function onOverlayClick() {
  if (props.closeOnOverlay) close()
}

function onKeydown(e: KeyboardEvent) {
  if (props.closeOnEscape && e.key === 'Escape') close()
}

watch(() => props.modelValue, (v) => {
  if (v) {
    document.addEventListener('keydown', onKeydown)
  } else {
    document.removeEventListener('keydown', onKeydown)
  }
}, { immediate: true })

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="dialog">
      <div v-if="modelValue" class="app-dialog-overlay" :style="{ zIndex }" @click.self="onOverlayClick">
        <div class="app-dialog" :class="{ 'is-sized': !!size }" :style="dialogStyle">
          <div v-if="title || $slots.header" class="app-dialog__header">
            <slot name="header">
              <h2 class="app-dialog__title">{{ title }}</h2>
            </slot>
            <button v-if="showClose" class="app-dialog__close" @click="close">✕</button>
          </div>
          <div class="app-dialog__body">
            <slot />
          </div>
          <div v-if="$slots.footer" class="app-dialog__footer">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.app-dialog-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-overlay);
  backdrop-filter: blur(4px);
}

.app-dialog {
  background: var(--color-dialog-bg);
  backdrop-filter: blur(16px);
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  box-shadow: var(--shadow-strong);
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  width: 90vw;
}

.app-dialog.is-sized {
  /* 尺寸档：档位宽度与 92vw 取小（standard/frontend/layout.md）；min() 在样式表内由浏览器求值 */
  width: min(var(--dialog-w, 560px), 92vw);
}

.app-dialog__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-dialog-divider);
  background: var(--color-dialog-header);
}

.app-dialog__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.app-dialog__close {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}
.app-dialog__close:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.app-dialog__body {
  padding: 20px;
  overflow-y: auto;
  flex: 1;
  color: var(--color-text-primary);
}

.app-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid var(--color-dialog-divider);
}

/* Transition */
.dialog-enter-active,
.dialog-leave-active {
  transition: opacity 0.2s ease;
}
.dialog-enter-active .app-dialog,
.dialog-leave-active .app-dialog {
  transition: transform 0.2s ease, opacity 0.2s ease;
}
.dialog-enter-from {
  opacity: 0;
}
.dialog-enter-from .app-dialog {
  transform: scale(0.97);
  opacity: 0;
}
.dialog-leave-to {
  opacity: 0;
}
.dialog-leave-to .app-dialog {
  transform: scale(0.97);
  opacity: 0;
}
</style>
