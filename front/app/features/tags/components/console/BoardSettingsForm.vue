<script setup lang="ts">
/**
 * 版块设置页签（rework-workshop-into-radar-and-desk 3.2「设置」）。
 *
 * 版块级字段就地编辑（名称/描述/匹配窗口/上下文档位/数据增强/关系自动发现）；
 * 全局参数（匹配参数、标签合并、回填、升级建议、标签池）不再平铺在工坊常驻工具栏，
 * 统一从今日雷达（总览）的版块治理入口或当前板块「板块管理」进入；设置页完整迁移另立变更。
 *
 * 保存成功后 emit `saved`，壳层刷新版块列表（面包屑/名称同步）。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useSemanticBoardsApi, type SemanticBoard } from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'

const props = defineProps<{
  board: SemanticBoard | null
}>()

const emit = defineEmits<{
  saved: []
  deleted: []
}>()

const api = useSemanticBoardsApi()
const { success: notifySuccess, error: notifyError } = useNotify()

const LAYERS: Array<{ key: string, label: string }> = [
  { key: 'week', label: '周' },
  { key: 'month', label: '月' },
  { key: 'year', label: '年' },
  { key: 'all', label: '全部' },
]

/** 与旧 BoardEditDialog 一致：窗口 1..90 天；清空/非法不发请求（后端 *int 会 400）。 */
const WINDOW_MIN = 1
const WINDOW_MAX = 90

const label = ref('')
const description = ref('')
const windowDays = ref<number | ''>(14)
const layers = ref<string[]>([])
const enrichmentEnabled = ref(false)
const relationEnabled = ref(false)
const saving = ref(false)
const deleting = ref(false)
const error = ref<string | null>(null)

function normalizeLayers(raw: string[] | undefined): string[] {
  const selected = new Set(raw?.length ? raw : LAYERS.map((layer) => layer.key))
  return LAYERS.map((layer) => layer.key).filter((key) => selected.has(key))
}

function resetFromBoard(board: SemanticBoard | null): void {
  label.value = board?.label ?? ''
  description.value = board?.description ?? ''
  windowDays.value = board?.window_days ?? 14
  layers.value = normalizeLayers(board?.context_layers)
  enrichmentEnabled.value = board?.enrichment_enabled ?? false
  relationEnabled.value = (board?.relation_auto_discovery_enabled ?? false) && enrichmentEnabled.value
  error.value = null
}

watch(() => props.board, resetFromBoard, { immediate: true })

// 关系自动发现依赖数据增强（旧 BoardEditDialog 的禁用联动）：关闭增强时一并清掉
watch(enrichmentEnabled, (enabled) => {
  if (!enabled) relationEnabled.value = false
})

const windowValid = computed(() =>
  typeof windowDays.value === 'number'
  && Number.isInteger(windowDays.value)
  && windowDays.value >= WINDOW_MIN
  && windowDays.value <= WINDOW_MAX,
)

const canSave = computed(() =>
  !!props.board
  && label.value.trim().length > 0
  && layers.value.length > 0
  && windowValid.value
  && !saving.value,
)

function toggleLayer(key: string): void {
  const selected = new Set(layers.value)
  if (selected.has(key)) selected.delete(key)
  else selected.add(key)
  layers.value = LAYERS.map((layer) => layer.key).filter((item) => selected.has(item))
}

async function save(): Promise<void> {
  const board = props.board
  if (!board) return
  if (label.value.trim().length === 0) {
    error.value = '版块名称不能为空'
    return
  }
  if (layers.value.length === 0) {
    error.value = '至少保留一个上下文档位'
    return
  }
  if (!windowValid.value) {
    error.value = `匹配窗口需为 ${WINDOW_MIN}–${WINDOW_MAX} 的整数天`
    return
  }
  saving.value = true
  error.value = null
  const res = await api.updateBoard(board.id, {
    label: label.value.trim(),
    description: description.value.trim(),
    window_days: Number(windowDays.value),
    context_layers: [...layers.value],
    enrichment_enabled: enrichmentEnabled.value,
    relation_auto_discovery_enabled: relationEnabled.value,
  })
  saving.value = false
  if (res.success) {
    notifySuccess('版块设置已保存')
    emit('saved')
  } else {
    error.value = res.error || '保存失败'
    notifyError(error.value)
  }
}

async function removeBoard(): Promise<void> {
  const board = props.board
  if (!board || deleting.value) return
  const ok = window.confirm(board.protected
    ? `「${board.label}」受保护，确认仍要删除？此操作不可撤销。`
    : `确认删除版块「${board.label}」？此操作不可撤销。`)
  if (!ok) return
  deleting.value = true
  error.value = null
  const res = await api.deleteBoard(board.id)
  deleting.value = false
  if (res.success) {
    notifySuccess('版块已删除')
    emit('deleted')
  } else {
    error.value = res.error || '删除失败'
    notifyError(error.value)
  }
}
</script>

<template>
  <div class="gsf" data-testid="gov-settings-form">
    <div v-if="!board" class="gsf-empty">版块尚未加载。</div>
    <template v-else>
      <label class="gsf-field">
        <span class="gsf-label">版块名称</span>
        <AppInput v-model="label" data-testid="gov-settings-label" :maxlength="60" />
      </label>

      <label class="gsf-field">
        <span class="gsf-label">描述</span>
        <textarea
          v-model="description"
          class="gsf-textarea"
          rows="3"
          maxlength="500"
          data-testid="gov-settings-description"
          placeholder="这个版块收什么、边界在哪"
        />
      </label>

      <label class="gsf-field">
        <span class="gsf-label">匹配窗口（天）</span>
        <AppInput
          v-model="windowDays"
          type="number"
          data-testid="gov-settings-window"
          :min="WINDOW_MIN"
          :max="WINDOW_MAX"
        />
        <span v-if="!windowValid" class="gsf-hint gsf-hint--error" data-testid="gov-settings-window-error">
          匹配窗口需为 {{ WINDOW_MIN }}–{{ WINDOW_MAX }} 的整数天
        </span>
      </label>

      <div class="gsf-field">
        <span class="gsf-label">上下文档位</span>
        <div class="gsf-chips">
          <button
            v-for="layer in LAYERS"
            :key="layer.key"
            type="button"
            class="gsf-chip"
            :class="{ 'is-on': layers.includes(layer.key) }"
            :data-testid="`gov-settings-layer-${layer.key}`"
            @click="toggleLayer(layer.key)"
          >
            {{ layer.label }}
          </button>
        </div>
        <span class="gsf-hint">解读员读取日报上下文时使用的窗口层级，至少保留一个。</span>
      </div>

      <div class="gsf-row">
        <div>
          <div class="gsf-label">数据增强</div>
          <div class="gsf-hint">开启后入库文章走增强管线（正文/摘要/背景）。</div>
        </div>
        <AppToggle v-model="enrichmentEnabled" data-testid="gov-settings-enrichment" />
      </div>

      <div class="gsf-row">
        <div>
          <div class="gsf-label">跨版块关系自动发现</div>
          <div class="gsf-hint">每日发现与本版块相关的其它版块（只引用现有版块）。</div>
        </div>
        <AppToggle
          v-model="relationEnabled"
          :disabled="!enrichmentEnabled"
          data-testid="gov-settings-relation"
        />
      </div>

      <div v-if="error" class="gsf-error" data-testid="gov-settings-error">
        <Icon icon="mdi:alert-circle-outline" width="14" />
        <span>{{ error }}</span>
      </div>

      <div class="gsf-actions">
        <AppButton
          variant="primary"
          size="sm"
          :loading="saving"
          :disabled="!canSave"
          data-testid="gov-settings-save"
          @click="save"
        >
          保存设置
        </AppButton>
        <span class="gsf-hint">匹配参数、扩充版块、匹配回填、标签合并与标签池的唯一入口在今日雷达（总览）的版块治理；本页随「板块管理」进入，只保存当前板块设置。</span>
        <button type="button" class="gsf-delete" data-testid="gov-settings-delete" :disabled="deleting" @click="removeBoard">
          {{ deleting ? '删除中…' : '删除版块' }}
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.gsf {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
}

.gsf-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 0;
}

.gsf-label {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.gsf-textarea {
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

.gsf-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.gsf-chip {
  padding: 0.3rem 0.6rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}

.gsf-chip.is-on {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.gsf-hint--error {
  color: var(--color-error);
}

.gsf-hint {
  font-size: 0.68rem;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.gsf-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.8rem;
  padding-top: 0.6rem;
  border-top: 1px solid var(--color-border-subtle);
}

.gsf-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  color: var(--color-error);
}

.gsf-delete {
  margin-left: auto;
  border: 1px solid color-mix(in srgb, var(--color-error) 40%, transparent);
  border-radius: 4px;
  padding: 0.32rem 0.55rem;
  background: transparent;
  color: var(--color-error);
  font-size: 0.7rem;
  cursor: pointer;
}

.gsf-delete:hover:not(:disabled) {
  background: color-mix(in srgb, var(--color-error) 8%, transparent);
}

.gsf-actions {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding-top: 0.6rem;
  border-top: 1px solid var(--color-border-subtle);
}

.gsf-empty {
  font-size: 0.75rem;
  color: var(--color-text-muted);
}
</style>
