<script setup lang="ts">
/**
 * 通用右侧抽屉（layout.md 抽屉档：默认 32rem，`max-width: 92vw`）。
 *
 * 约定：
 * - 覆盖式呈现（overlay 不挤压宿主布局），遮罩点击 / Esc / 右上角关闭按钮均可关闭；
 * - 打开期间锁定 body 滚动（窄屏页面态时尤为重要），关闭后恢复原值；
 * - 头部由 `#head` 插槽承载（kicker/title/sub 由调用方决定），关闭按钮与滚动容器由本组件提供；
 * - `#footer` 可选，固定在抽屉底部（动作条不随正文滚动）。
 *
 * 消费者：`SectionDrawer`（32rem 分区抽屉）。
 */
import { onUnmounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { popOverlay, pushOverlay } from './overlayStack'

const props = withDefaults(defineProps<{
  open: boolean
  /** 无障碍名称（role=dialog 的 aria-label）。 */
  label: string
  /** 抽屉宽度，默认 32rem；窄屏统一压到 92vw。 */
  width?: string
  /** 根节点 data-testid（测试与验收锚点）。 */
  testid?: string
  /** 关闭按钮 data-testid。 */
  closeTestid?: string
}>(), {
  width: '32rem',
  testid: 'app-drawer',
  closeTestid: 'app-drawer-close',
})

const emit = defineEmits<{
  close: []
}>()

let previousOverflow: string | null = null
let overlayPushed = false

function lockScroll(): void {
  if (typeof document === 'undefined') return
  if (previousOverflow !== null) return
  previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
}

function unlockScroll(): void {
  if (typeof document === 'undefined') return
  if (previousOverflow === null) return
  document.body.style.overflow = previousOverflow
  previousOverflow = null
}

const bodyRef = ref<HTMLElement | null>(null)

function scrollToTop(): void {
  bodyRef.value?.scrollTo({ top: 0 })
}

defineExpose({ scrollToTop })

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  // 上层还有对话框（如组合标签编辑）时，Esc 交给它，避免连抽屉一起关掉丢表单
  const stacked = Array.from(document.querySelectorAll('.app-dialog-overlay'))
    .some((el) => (el as HTMLElement).offsetParent !== null)
  if (stacked) return
  emit('close')
}

watch(() => props.open, (open) => {
  if (typeof window === 'undefined') return
  if (open) {
    window.addEventListener('keydown', onKeydown)
    lockScroll()
    if (!overlayPushed) {
      pushOverlay()
      overlayPushed = true
    }
  } else {
    window.removeEventListener('keydown', onKeydown)
    unlockScroll()
    if (overlayPushed) {
      popOverlay()
      overlayPushed = false
    }
  }
}, { immediate: true })

onUnmounted(() => {
  if (typeof window === 'undefined') return
  window.removeEventListener('keydown', onKeydown)
  unlockScroll()
  if (overlayPushed) {
    popOverlay()
    overlayPushed = false
  }
})
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="ad" :data-testid="testid">
      <div class="ad-scrim" aria-hidden="true" @click="emit('close')" />
      <aside
        class="ad-panel"
        role="dialog"
        aria-modal="true"
        :aria-label="label"
        :style="{ width }"
      >
        <header class="ad-head">
          <div class="ad-head-main">
            <slot name="head" />
          </div>
          <button
            type="button"
            class="ad-close"
            title="关闭"
            :data-testid="closeTestid"
            @click="emit('close')"
          >
            <Icon icon="mdi:close" width="16" />
          </button>
        </header>

        <div ref="bodyRef" class="ad-body">
          <slot />
        </div>

        <footer v-if="$slots.footer" class="ad-foot">
          <slot name="footer" />
        </footer>
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
.ad {
  position: fixed;
  inset: 0;
  z-index: 60;
}

.ad-scrim {
  position: absolute;
  inset: 0;
  z-index: 60;
  background: var(--color-bg-overlay, rgba(0, 0, 0, 0.45));
}

.ad-panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 61;
  max-width: 92vw;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--color-bg-elevated);
  border-left: 1px solid var(--color-border-medium);
  box-shadow: var(--shadow-strong);
}

.ad-head {
  display: flex;
  align-items: flex-start;
  gap: 0.8rem;
  padding: 1.1rem 1.2rem 0.9rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.ad-head-main {
  flex: 1;
  min-width: 0;
}

.ad-close {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
}

.ad-close:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

.ad-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 1rem 1.2rem 1.4rem;
}

.ad-foot {
  flex: 0 0 auto;
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.8rem 1.2rem;
  border-top: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
}

@media (max-width: 767.98px) {
  .ad-panel {
    width: 100vw !important;
    max-width: 100vw;
  }
}
</style>
