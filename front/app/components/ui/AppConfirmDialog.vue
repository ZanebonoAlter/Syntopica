<script setup lang="ts">
/**
 * 全局统一确认弹窗（fix-provider-delete-route-deadlock 2b，design D6）。
 * 渲染端：消费 useConfirm() 的全局 state；调用端在任意组合式函数里
 * `const { confirm } = useConfirm(); await confirm({ ... })`。
 *
 * 交互契约（对齐原生 confirm 的阻断语义）：
 * - 仅「取消/确认」两钮 + Escape（=取消）可关；不点遮罩关、无右上关闭钮
 * - danger=true 时确认按钮为 danger 样式（删除类动作）
 */
import AppDialog from './AppDialog.vue'
import AppButton from './AppButton.vue'
import { useConfirm } from '~/composables/useConfirm'

const { state, settle } = useConfirm()

function cancel() {
  if (state.value) settle(state.value.id, false)
}
function accept() {
  if (state.value) settle(state.value.id, true)
}
</script>

<template>
  <AppDialog
    :model-value="state?.open ?? false"
    :title="state?.title ?? ''"
    size="sm"
    :close-on-overlay="false"
    :show-close="false"
    @update:model-value="(v: boolean) => { if (!v) cancel() }"
  >
    <p class="confirm-dialog__message">{{ state?.message }}</p>
    <template #footer>
      <div class="confirm-dialog__actions">
        <AppButton variant="secondary" @click="cancel">{{ state?.cancelText }}</AppButton>
        <AppButton :variant="state?.danger ? 'danger' : 'primary'" @click="accept">
          {{ state?.confirmText }}
        </AppButton>
      </div>
    </template>
  </AppDialog>
</template>

<style scoped>
.confirm-dialog__message {
  margin: 0;
  line-height: 1.6;
  color: var(--color-text-primary);
  white-space: pre-line;
}

.confirm-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
