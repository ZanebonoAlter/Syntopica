<script setup lang="ts">
/**
 * 版块归档弹窗（change `lane-manual-correction-loop` 6.5；契约见 ui-design「轻量弹窗操作」）。
 *
 * 归档是**软删**：`status=disabled`（后端同时清 embedding），可逆——重新启用即恢复。
 * 沿用「手打版块名」确认惯例：必须逐字输入版块名才放行归档按钮（防误触）。
 * 受保护版块（`protected=true`）**禁用归档**并说明原因。
 *
 * 布局契约：轻量弹窗只承载单字段/轻量操作 → `AppDialog size="md"`（560px，92vw 上限）。
 */
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'

const props = defineProps<{
  modelValue: boolean
  /** 待归档版块 id；null = 无目标。 */
  boardId: number | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  archived: []
}>()

const api = useSemanticBoardsApi()
const { success: notifySuccess, error: notifyError } = useNotify()

const boardLabel = ref('')
const protectedBoard = ref(false)
const confirmText = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)

/** 归档按钮放行条件：逐字命中版块名（手打确认）。 */
const canConfirm = computed(() =>
  boardLabel.value.length > 0 && confirmText.value.trim() === boardLabel.value.trim(),
)

async function load(): Promise<void> {
  error.value = null
  confirmText.value = ''
  boardLabel.value = ''
  protectedBoard.value = false
  if (props.boardId === null) return
  loading.value = true
  const res = await api.getBoards()
  loading.value = false
  if (!res.success) {
    error.value = res.error || '加载版块失败'
    return
  }
  const board = res.data?.items.find(item => item.id === props.boardId)
  if (!board) {
    error.value = '版块不存在'
    return
  }
  boardLabel.value = board.label
  protectedBoard.value = board.protected
}

watch(
  () => [props.modelValue, props.boardId] as const,
  ([open]) => {
    if (!open) {
      error.value = null
      return
    }
    void load()
  },
  { immediate: true },
)

async function archive(): Promise<void> {
  if (props.boardId === null || saving.value || !canConfirm.value) return
  saving.value = true
  error.value = null
  const res = await api.updateBoard(props.boardId, { status: 'disabled' })
  saving.value = false
  if (res.success) {
    notifySuccess('已归档')
    emit('archived')
    emit('update:modelValue', false)
  } else {
    error.value = res.error || '归档失败'
    notifyError(error.value)
  }
}

function close(): void {
  if (saving.value) return
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    title="归档版块"
    size="md"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="loading" class="bad-loading" data-testid="bad-loading">加载中…</div>
    <div v-else class="bad-body">
      <p class="bad-text" data-testid="bad-text">
        确定归档版块「{{ boardLabel }}」？
      </p>
      <p class="bad-hint">归档后该版块不再出现在雷达与矩阵里，重新启用即恢复。</p>

      <label class="bad-field">
        <span class="bad-label">输入版块名「{{ boardLabel }}」以确认</span>
        <AppInput
          v-model="confirmText"
          type="text"
          placeholder="版块名称"
          data-testid="bad-confirm-input"
        />
      </label>

      <p v-if="protectedBoard" class="bad-protected" data-testid="bad-protected">
        <Icon icon="mdi:shield-lock-outline" width="14" />
        <span>该版块受保护，不可归档。</span>
      </p>

      <p v-if="error" class="bad-error" data-testid="bad-error">{{ error }}</p>
    </div>

    <template #footer>
      <AppButton variant="secondary" :disabled="saving" @click="close">取消</AppButton>
      <AppButton
        variant="danger"
        :loading="saving"
        :disabled="!canConfirm || protectedBoard"
        data-testid="bad-confirm"
        @click="archive"
      >
        归档
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.bad-body {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.bad-loading {
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.bad-text {
  margin: 0;
  font-size: 0.86rem;
  line-height: 1.6;
  color: var(--color-text-primary);
}

.bad-hint {
  margin: 0;
  font-size: 0.76rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.bad-field {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.bad-label {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.bad-protected {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-muted);
}

.bad-error {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-error);
}
</style>
