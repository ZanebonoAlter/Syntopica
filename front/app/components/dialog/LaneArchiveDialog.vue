<script setup lang="ts">
/**
 * 泳道归档弹窗（change `lane-manual-correction-loop` 6.1；契约见 ui-design「轻量弹窗操作」）。
 *
 * **只管归档**这一件事：
 *  - 改名不在此——改名的主路径是泳道修正工作面标题处的就地编辑（不弹窗）；
 *    泳道管理列表另保留 `sm` 档重命名弹窗作为次要路径（后续批实现）。
 *  - 归档是破坏性操作 → 先走 `useConfirm()` 二次确认，再落 `status=archived`。
 *  - 归档**可逆**（切回启用即恢复），提示文案如实说明「不再吸附新内容」。
 *
 * 布局契约：轻量弹窗只承载单字段/轻量操作 → `AppDialog size="sm"`（420px，92vw 上限）；
 * 弹窗内不再开嵌套浮层（二次确认走全局 `useConfirm` 管道，由 `AppConfirmDialog` 承载）。
 */
import { ref, watch } from 'vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useConfirm } from '~/composables/useConfirm'
import { useNotify } from '~/composables/useNotify'

const props = defineProps<{
  modelValue: boolean
  /** 持久话题（泳道）id。 */
  topicId: number
  /** 泳道名（提示与确认文案用）。 */
  laneLabel: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  archived: []
}>()

const { updateTopic } = useDailyReportsApi()
const { confirm } = useConfirm()
const { success: notifySuccess, error: notifyError } = useNotify()

const archiving = ref(false)
const error = ref<string | null>(null)

watch(() => props.modelValue, (open) => {
  if (open) error.value = null
})

/** 归档：二次确认 → 落 `status=archived` → 提示「不再吸附新内容」。 */
async function archive(): Promise<void> {
  if (archiving.value) return
  const ok = await confirm({
    title: '归档泳道',
    message: `归档「${props.laneLabel}」后，该泳道不再吸附新内容。此操作可逆，重新启用即恢复。`,
    confirmText: '归档',
    danger: true,
  })
  if (!ok) return

  archiving.value = true
  error.value = null
  const res = await updateTopic(props.topicId, { status: 'archived' })
  archiving.value = false
  if (res.success) {
    notifySuccess('已归档，该泳道不再吸附新内容')
    emit('archived')
    emit('update:modelValue', false)
  } else {
    error.value = res.error || '归档失败'
    notifyError(error.value)
  }
}

function close(): void {
  if (archiving.value) return
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    title="归档泳道"
    size="sm"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="lane-archive-body">
      <p class="lane-archive-text" data-testid="lane-archive-text">
        确定归档泳道「{{ laneLabel }}」？
      </p>
      <p class="lane-archive-hint" data-testid="lane-archive-hint">
        归档后该泳道不再吸附新内容，可重新启用恢复。
      </p>
      <p v-if="error" class="lane-archive-error" data-testid="lane-archive-error">{{ error }}</p>
    </div>

    <template #footer>
      <AppButton variant="secondary" :disabled="archiving" @click="close">取消</AppButton>
      <AppButton
        variant="danger"
        :loading="archiving"
        data-testid="lane-archive-confirm"
        @click="archive"
      >
        归档
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.lane-archive-body {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.lane-archive-text {
  margin: 0;
  font-size: 0.86rem;
  line-height: 1.6;
  color: var(--color-text-primary);
}

.lane-archive-hint {
  margin: 0;
  font-size: 0.76rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.lane-archive-error {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-error);
}
</style>
