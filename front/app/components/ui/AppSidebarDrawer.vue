<script setup lang="ts">
import { onUnmounted, watch } from 'vue'

/**
 * 窄屏导航抽屉容器（mobile-viewport-stage1，ui-design.md Component Reuse）：
 * 只做容器壳——Teleport + 遮罩 + 左侧滑入面板，内容由默认 slot 供给
 * （宽屏常驻侧栏与窄屏抽屉渲染同一份信息结构，见 design.md D3：不复用 AppDialog）。
 */
interface Props {
  /** 打开态：驱动面板滑入/滑出与遮罩淡入/淡出 */
  open: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  close: []
}>()

/** 抽屉宽度契约：min(80vw, 320px)（ui-design.md Layout Contract「抽屉」行） */
const PANEL_WIDTH = 'min(80vw, 320px)'

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

watch(() => props.open, (open) => {
  if (open) {
    window.addEventListener('keydown', onKeydown)
  } else {
    window.removeEventListener('keydown', onKeydown)
  }
}, { immediate: true })

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <!-- 根只做定位与开合过渡壳；遮罩与面板为兄弟层（aria-hidden 祖先不得包裹 role=dialog 面板） -->
    <div class="app-sidebar-drawer" :class="{ 'is-open': open }">
      <div class="app-sidebar-drawer__scrim" aria-hidden="true" @click="emit('close')"></div>
      <div
        class="app-sidebar-drawer__panel"
        role="dialog"
        aria-modal="true"
        :style="{ '--drawer-w': PANEL_WIDTH }"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.app-sidebar-drawer {
  position: fixed;
  inset: 0;
  z-index: 1000;
  visibility: hidden;
  /* 关闭时延迟挂起 visibility：等遮罩淡出走完再隐藏，防止关闭动效被截断闪烁 */
  transition: visibility 0s linear 260ms;
}

.app-sidebar-drawer.is-open {
  visibility: visible;
  transition: visibility 0s;
}

.app-sidebar-drawer__scrim {
  position: absolute;
  inset: 0;
  z-index: 1000;
  background: var(--color-bg-overlay);
  opacity: 0;
  transition: opacity 260ms ease-out;
}

.app-sidebar-drawer.is-open .app-sidebar-drawer__scrim {
  opacity: 1;
}

.app-sidebar-drawer__panel {
  position: absolute;
  top: 0;
  left: 0;
  bottom: 0;
  z-index: 1001; /* 高于遮罩 */
  width: var(--drawer-w);
  background: var(--color-bg-elevated);
  border-right: 1px solid var(--color-border-medium);
  box-shadow: var(--shadow-strong);
  overflow-y: auto;
  transform: translateX(-100%);
  transition: transform 260ms ease-out;
}

.app-sidebar-drawer.is-open .app-sidebar-drawer__panel {
  transform: translateX(0);
}
</style>
