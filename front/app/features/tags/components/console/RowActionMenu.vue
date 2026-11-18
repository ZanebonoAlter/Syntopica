<script setup lang="ts">
/**
 * 行尾「⋯」操作菜单（change `lane-manual-correction-loop` 6.3 / 6.5 共用）。
 *
 * 契约（`lane-correction-surface`「入口接线」）：行尾 / 行头的「⋯」按钮
 * **SHALL NOT 触发行点击进入版块态势页或触发行选中**——本组件的触发器一律 `@click.stop`，
 * 菜单本体 Teleport 到 body（天然不在行 DOM 内，冒泡不到行）。
 *
 * 浮层契约（`standard/frontend/layout.md`「浮层层级契约」）：层级由 `Z_INDEX` 档位表内联绑定
 * 注入（菜单档 `toast`），`<style>` 内不写死 z-index；菜单用 floating-ui 按触发器定位，
 * 不受行内 sticky 列与横向滚动容器的裁剪影响。
 *
 * 界面文字纪律：菜单项只出现**动作标签**与必要状态（受保护原因），不写操作口径解说。
 */
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import { useFloating, autoUpdate, offset, shift, flip } from '@floating-ui/vue'
import { Z_INDEX } from '~/components/ui/layout-contract'
import type { RowActionMenuItem } from './rowActionMenu'

const props = withDefaults(defineProps<{
  items: RowActionMenuItem[]
  /** 触发器无障碍名（如「XX 的更多操作」）。 */
  triggerLabel: string
  /** 触发器 testid（行内唯一）。 */
  triggerTestid: string
  placement?: 'bottom-start' | 'bottom-end'
}>(), {
  placement: 'bottom-end',
})

const emit = defineEmits<{
  select: [key: string]
}>()

const triggerRef = ref<HTMLElement>()
const floatingRef = ref<HTMLElement>()
const isOpen = ref(false)

const { floatingStyles } = useFloating(triggerRef, floatingRef, {
  placement: props.placement,
  middleware: [offset(4), shift(), flip()],
  whileElementsMounted: autoUpdate,
})

function toggle(): void {
  isOpen.value = !isOpen.value
}

function close(): void {
  isOpen.value = false
}

function handleSelect(item: RowActionMenuItem): void {
  if (item.disabled) return
  close()
  emit('select', item.key)
}

function handleOutsideClick(event: MouseEvent): void {
  if (!isOpen.value || !triggerRef.value || !floatingRef.value) return
  const target = event.target as Node
  if (!triggerRef.value.contains(target) && !floatingRef.value.contains(target)) close()
}

onMounted(() => {
  document.addEventListener('click', handleOutsideClick)
})

onUnmounted(() => {
  document.removeEventListener('click', handleOutsideClick)
})
</script>

<template>
  <button
    ref="triggerRef"
    type="button"
    class="row-menu-trigger"
    :aria-label="triggerLabel"
    aria-haspopup="menu"
    :aria-expanded="isOpen"
    :data-testid="triggerTestid"
    @click.stop="toggle"
  >
    <Icon icon="mdi:dots-horizontal" width="16" />
  </button>

  <Teleport to="body">
    <div
      v-if="isOpen"
      ref="floatingRef"
      class="row-menu"
      :style="[floatingStyles, { zIndex: Z_INDEX.toast }]"
      role="menu"
      data-testid="row-action-menu"
      @click.stop
    >
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        class="row-menu-item"
        :class="{ 'is-danger': item.danger }"
        :disabled="item.disabled"
        :data-testid="`${triggerTestid}-${item.key}`"
        role="menuitem"
        @click.stop="handleSelect(item)"
      >
        <span class="row-menu-item-label">{{ item.label }}</span>
        <small v-if="item.hint" class="row-menu-item-hint">{{ item.hint }}</small>
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.row-menu-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 1.35rem;
  height: 1.35rem;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
}

.row-menu-trigger:hover,
.row-menu-trigger:focus-visible {
  border-color: var(--color-border-medium);
  color: var(--color-text-primary);
  outline: none;
}

.row-menu {
  display: flex;
  flex-direction: column;
  min-width: 9rem;
  padding: 0.25rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-dialog-bg);
  box-shadow: var(--shadow-strong);
}

.row-menu-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.1rem;
  width: 100%;
  padding: 0.35rem 0.5rem;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.76rem;
  text-align: left;
  cursor: pointer;
}

.row-menu-item:hover:not(:disabled) {
  background: var(--color-bg-hover);
}

.row-menu-item.is-danger {
  color: var(--color-error);
}

.row-menu-item:disabled {
  color: var(--color-text-muted);
  cursor: not-allowed;
}

.row-menu-item-hint {
  font-size: 0.62rem;
  color: var(--color-text-muted);
}
</style>
