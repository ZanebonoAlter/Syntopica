<script setup lang="ts">
/**
 * 版块轻编辑弹窗（change `lane-manual-correction-loop` 6.2；契约见 ui-design「轻量弹窗操作」）。
 *
 * L0 雷达行尾「⋯」→「编辑版块」的落点：可改 **名称 / 描述 / 排序 / 受保护**，保存调既有
 * `updateBoard`（`PUT /semantic-boards/:id`），保存成功后由父层刷新版块列表。
 *
 * 字段定义**抽取共用**（`boardEditFields.ts`）——与 `BoardSettingsForm.vue`（设置页签）同一份
 * 元数据与校验口径，不复制粘贴第二套。
 *
 * 布局契约：轻量弹窗只承载单字段/轻量操作 → `AppDialog size="md"`（560px，92vw 上限）；
 * 弹窗内不再开嵌套浮层。
 */
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'
import {
  BOARD_EDIT_FIELDS,
  createBoardEditDraft,
  toBoardEditPayload,
  validateBoardEditDraft,
  type BoardEditDraft,
} from './boardEditFields'

const props = defineProps<{
  modelValue: boolean
  /** 待编辑版块 id；null = 无目标（弹窗空态）。 */
  boardId: number | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  saved: []
}>()

const api = useSemanticBoardsApi()
const { success: notifySuccess, error: notifyError } = useNotify()

const draft = ref<BoardEditDraft>(createBoardEditDraft(null))
const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)

/** 打开时按 id 取版块（雷达行只有 id，描述/排序/受保护需回查）。 */
async function load(): Promise<void> {
  error.value = null
  if (props.boardId === null) {
    draft.value = createBoardEditDraft(null)
    return
  }
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
    draft.value = createBoardEditDraft(null)
    return
  }
  draft.value = createBoardEditDraft(board)
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

async function save(): Promise<void> {
  if (props.boardId === null || saving.value) return
  const invalid = validateBoardEditDraft(draft.value)
  if (invalid) {
    error.value = invalid
    return
  }
  saving.value = true
  error.value = null
  const res = await api.updateBoard(props.boardId, toBoardEditPayload(draft.value))
  saving.value = false
  if (res.success) {
    notifySuccess('版块已更新')
    emit('saved')
    emit('update:modelValue', false)
  } else {
    error.value = res.error || '保存失败'
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
    title="编辑版块"
    size="md"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="loading" class="bqed-loading" data-testid="bqed-loading">加载中…</div>
    <div v-else class="bqed-fields">
      <div v-for="field in BOARD_EDIT_FIELDS" :key="field.key" class="bqed-field">
        <span class="bqed-label">{{ field.label }}</span>
        <AppInput
          v-if="field.type === 'text'"
          v-model="draft.label"
          :maxlength="field.maxlength"
          :placeholder="field.placeholder"
          :data-testid="`bqed-${field.key}`"
        />
        <textarea
          v-else-if="field.type === 'textarea'"
          v-model="draft.description"
          class="bqed-textarea"
          rows="3"
          :maxlength="field.maxlength"
          :placeholder="field.placeholder"
          :data-testid="`bqed-${field.key}`"
        />
        <AppInput
          v-else-if="field.type === 'number'"
          v-model="draft.displayOrder"
          type="number"
          :placeholder="field.placeholder"
          :data-testid="`bqed-${field.key}`"
        />
        <AppToggle
          v-else-if="field.type === 'toggle'"
          v-model="draft.protected"
          :data-testid="`bqed-${field.key}`"
        />
        <span v-if="field.hint" class="bqed-hint">{{ field.hint }}</span>
      </div>

      <p v-if="error" class="bqed-error" data-testid="bqed-error">
        <Icon icon="mdi:alert-circle-outline" width="14" />
        <span>{{ error }}</span>
      </p>
    </div>

    <template #footer>
      <AppButton variant="secondary" :disabled="saving" @click="close">取消</AppButton>
      <AppButton
        variant="primary"
        :loading="saving"
        data-testid="bqed-save"
        @click="save"
      >
        保存
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.bqed-fields {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}

.bqed-loading {
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.bqed-field {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  min-width: 0;
}

.bqed-label {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.bqed-textarea {
  width: 100%;
  min-height: 4.5rem;
  padding: 0.45rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.78rem;
  line-height: 1.5;
  resize: vertical;
}

.bqed-hint {
  font-size: 0.68rem;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.bqed-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-error);
}
</style>
