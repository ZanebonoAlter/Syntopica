<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useAuxiliaryLabelsApi, type AuxiliaryLabel } from '~/api/auxiliaryLabels'
import { useCompositeLabelsApi, type CompositeLabel, type ComponentOption } from '~/api/compositeLabels'
import { useSemanticBoardsApi } from '~/api/semanticBoards'

/**
 * 手动创建组合标签对话框：label + 描述 + 组件选择器（auxiliary active，2-5 个）。
 * 去重命中（reused_l1 / alias_l2）不视为错误——展示后端 message 与既有组合信息。
 *
 * 交互（design D7）：默认候选按推荐度排序（版块挂载数 → ref_count，服务端算好），
 * 搜索时降级全量模糊；chip 带「挂 N 版块」信号；已选组件命中现有组合时优先展示，
 * 组件集完全一致时预告 L1 复用。组合列表由 Pool props 传入（对话框零额外请求）。
 */
const props = defineProps<{
  visible: boolean
  composites?: CompositeLabel[]
  /** 版块上下文（design D7）：本版块挂载组件置顶 + 创建成功后自动挂载到该版块 */
  boardId?: number
  boardLabel?: string
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const auxApi = useAuxiliaryLabelsApi()
const { createLabel, getComponentOptions } = useCompositeLabelsApi()
const boardsApi = useSemanticBoardsApi()

const label = ref('')
const description = ref('')
const search = ref('')
const selectedIds = ref<number[]>([])
const optionItems = ref<ComponentOption[]>([])
const auxLoading = ref(false)
const submitting = ref(false)
const error = ref('')
const resultMessage = ref('')
const resultOutcome = ref('')

const MIN_COMPONENTS = 2
const MAX_COMPONENTS = 5

const selectedCount = computed(() => selectedIds.value.length)

/** 最近一次选中的组件（联动信号 related_to）：候选列表按「与它的共现频次」实时重排。 */
const latestSelectedId = ref<number | null>(null)
watch(selectedIds, (next, prev) => {
  if (next.length > prev.length) {
    const added = next.filter(id => !prev.includes(id))
    latestSelectedId.value = added[added.length - 1] ?? null
    // 打开状态且无搜索词时联动重拉候选（design D7：选完一个标签，可组合的浮上来）
    if (props.visible && search.value.trim() === '') {
      loadOptions()
    }
  }
})
const countError = computed(() => {
  if (selectedCount.value === 0) return ''
  if (selectedCount.value < MIN_COMPONENTS) return `至少选择 ${MIN_COMPONENTS} 个组件（当前 ${selectedCount.value}）`
  if (selectedCount.value > MAX_COMPONENTS) return `最多选择 ${MAX_COMPONENTS} 个组件（当前 ${selectedCount.value}）`
  return ''
})

const canSubmit = computed(() => {
  return label.value.trim() !== '' && selectedCount.value >= MIN_COMPONENTS && selectedCount.value <= MAX_COMPONENTS && !submitting.value
})

/** 见过的候选 label 累积缓存：联动重拉后最新选中组件（related_to 自身共现=0）会被
 * top50 截断掉，已选序列的展示不能依赖当前候选列表——从缓存取，永不丢。 */
const labelCache = ref(new Map<number, string>())
const selectedAuxLabels = computed(() => {
  const out: { id: number, label: string }[] = []
  for (const id of selectedIds.value) {
    const label = labelCache.value.get(id)
    if (label !== undefined) out.push({ id, label })
  }
  return out
})

/** 已选组件命中的现有组合（含任一已选组件即相关，优先展示防重复创建）。 */
const relatedComposites = computed<CompositeLabel[]>(() => {
  if (selectedIds.value.length === 0) return []
  const selected = new Set(selectedIds.value)
  return (props.composites ?? []).filter(c => c.components.some(comp => selected.has(comp.label_id)))
})

/** 选中集合与某现有组合组件集完全一致 → 创建时后端 L1 去重会复用它。 */
const l1ReuseComposite = computed<CompositeLabel | null>(() => {
  if (selectedIds.value.length === 0) return null
  const selected = new Set(selectedIds.value)
  return (props.composites ?? []).find(c => {
    if (c.components.length !== selected.size) return false
    return c.components.every(comp => selected.has(comp.label_id))
  }) ?? null
})

watch(() => props.visible, (v) => {
  if (v) {
    label.value = ''
    description.value = ''
    search.value = ''
    selectedIds.value = []
    latestSelectedId.value = null
    error.value = ''
    resultMessage.value = ''
    resultOutcome.value = ''
    loadOptions()
  }
}, { immediate: true })

watch(search, () => loadOptions())

/** 默认拉推荐列表（版块挂载数 → ref_count）；搜索时降级全量模糊。 */
async function loadOptions() {
  auxLoading.value = true
  try {
    if (search.value.trim() === '') {
      const res = await getComponentOptions({
        board_id: props.boardId,
        related_to: latestSelectedId.value ?? undefined,
      })
      optionItems.value = res.data?.items ?? []
      for (const o of optionItems.value) labelCache.value.set(o.id, o.label)
    }
    else {
      // 搜索降级：全量模糊检索（无推荐信号，board_count=0）
      const res = await auxApi.getLabels({ status: 'active', search: search.value.trim(), per_page: 100 })
      optionItems.value = (res.data?.items ?? []).map(toFallbackOption)
      for (const o of optionItems.value) labelCache.value.set(o.id, o.label)
    }
  }
  catch {
    // 推荐接口失败降级回 aux 全量（S12 变体 3），仍失败则空列表
    try {
      const res = await auxApi.getLabels({ status: 'active', per_page: 100 })
      optionItems.value = (res.data?.items ?? []).map(toFallbackOption)
      for (const o of optionItems.value) labelCache.value.set(o.id, o.label)
    }
    catch {
      optionItems.value = []
    }
  }
  finally {
    auxLoading.value = false
  }
}

/** 搜索/降级路径的映射：aux 列表 → 无推荐信号的候选。 */
function toFallbackOption(a: AuxiliaryLabel): ComponentOption {
  return {
    id: a.id,
    label: a.label,
    ref_count: a.ref_count,
    board_count: 0,
    in_board: false,
    cooccurrence: 0,
    mounted_boards: [],
  }
}

function toggleComponent(id: number) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) {
    selectedIds.value = selectedIds.value.filter(x => x !== id)
    return
  }
  // 超上限时不再勾选（保留已选，防误输入 6 个）
  if (selectedIds.value.length >= MAX_COMPONENTS) return
  selectedIds.value = [...selectedIds.value, id]
}

/** 已选组件上移/下移（顺序决定组件序列 position）。 */
function moveSelected(id: number, offset: number) {
  const idx = selectedIds.value.indexOf(id)
  const target = idx + offset
  if (idx < 0 || target < 0 || target >= selectedIds.value.length) return
  const next = [...selectedIds.value]
  const a = next[idx]
  const b = next[target]
  if (a === undefined || b === undefined) return
  next[idx] = b
  next[target] = a
  selectedIds.value = next
}

async function submitCreate() {
  if (!canSubmit.value) return
  submitting.value = true
  error.value = ''
  resultMessage.value = ''
  try {
    const res = await createLabel({
      label: label.value.trim(),
      description: description.value.trim() || undefined,
      component_label_ids: [...selectedIds.value],
    })
    const data = res.data
    if (data) {
      resultOutcome.value = data.outcome
      resultMessage.value = data.message
      if (data.outcome === 'created') {
        // 版块上下文：创建即挂载到该版块（组合标签服务于版块归类）
        if (props.boardId) {
          try {
            const mount = await boardsApi.addComposition(props.boardId, data.id)
            if (mount.success) {
              resultMessage.value = data.message + `（已挂载到「${props.boardLabel ?? '当前版块'}」）`
            }
          } catch {
            // 挂载失败不影响创建结果，用户可手动挂载
          }
        }
        emit('confirm')
        // 创建成功后短暂展示结果再关闭
        setTimeout(() => emit('cancel'), 600)
      }
      else {
        // 去重命中：保留对话框展示复用信息（含既有组合 id），用户确认后手动关闭
        emit('confirm')
      }
    }
  }
  catch (e) {
    const msg = (e as { message?: string })?.message ?? ''
    error.value = msg || '创建失败，请稍后重试'
  }
  finally {
    submitting.value = false
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="cld-overlay" @click.self="emit('cancel')">
      <div class="cld-card" data-testid="composite-label-dialog">
        <div class="cld-header">
          <h3 class="cld-title">{{ boardLabel ? `为「${boardLabel}」新建组合标签` : '新建组合标签' }}</h3>
          <button type="button" class="cld-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div class="cld-body">
          <label class="cld-field">
            <span class="cld-field-label">组合名称 *</span>
            <input v-model="label" type="text" class="cld-input" placeholder="如：美债收益率" data-testid="composite-label-name" />
          </label>
          <label class="cld-field">
            <span class="cld-field-label">描述</span>
            <input v-model="description" type="text" class="cld-input" placeholder="一句话说明组合含义（可选）" />
          </label>

          <div v-if="selectedCount > 0" class="cld-selected" data-testid="composite-label-selected">
            <span class="cld-field-label">组件序列（顺序即组合方向）：</span>
            <div class="cld-selected-list">
              <div v-for="(aux, i) in selectedAuxLabels" :key="aux.id" class="cld-selected-item" :data-testid="`composite-label-component-${i}`">
                <span class="cld-selected-pos">{{ i + 1 }}</span>
                <span class="cld-selected-label" :title="aux.label">{{ aux.label }}</span>
                <button type="button" class="cld-move" :disabled="i === 0" title="上移" @click="moveSelected(aux.id, -1)">↑</button>
                <button type="button" class="cld-move" :disabled="i === selectedCount - 1" title="下移" @click="moveSelected(aux.id, 1)">↓</button>
                <button type="button" class="cld-remove" title="移除" @click="toggleComponent(aux.id)">
                  <Icon icon="mdi:close" width="12" />
                </button>
              </div>
            </div>
            <p v-if="countError" class="cld-error-inline" data-testid="composite-label-count-error">{{ countError }}</p>
          </div>

          <div class="cld-picker">
            <div class="cld-picker-toolbar">
              <span class="cld-field-label">选择组件（active 辅助标签，2-5 个）</span>
              <input v-model="search" type="text" class="cld-input cld-input--search" placeholder="搜索组件..." />
            </div>
            <div v-if="auxLoading" class="cld-loading">
              <Icon icon="mdi:loading" width="16" class="animate-spin" /> 加载组件...
            </div>
            <div v-else class="cld-picker-list" data-testid="composite-label-picker">
              <button
                v-for="aux in optionItems"
                :key="aux.id"
                type="button"
                class="cld-picker-item"
                :class="{ 'is-selected': selectedIds.includes(aux.id), 'is-disabled': !selectedIds.includes(aux.id) && selectedCount >= MAX_COMPONENTS }"
                :title="aux.board_count > 0 ? `已被 ${aux.board_count} 个版块挂载：${aux.mounted_boards.map(b => b.label).join('、')}` : aux.label"
                @click="toggleComponent(aux.id)"
              >
                <Icon :icon="selectedIds.includes(aux.id) ? 'mdi:checkbox-marked' : 'mdi:checkbox-blank-outline'" width="13" />
                {{ aux.label }}
                <span v-if="aux.cooccurrence > 0" class="cld-cooc-badge" data-testid="composite-label-cooc-badge">共现{{ aux.cooccurrence }}</span>
                <span v-if="aux.in_board" class="cld-board-badge" data-testid="composite-label-inboard-badge">本版块</span>
                <span v-else-if="aux.board_count > 0" class="cld-board-badge cld-board-badge--muted" data-testid="composite-label-board-badge">挂{{ aux.board_count }}版块</span>
              </button>
              <p v-if="optionItems.length === 0" class="cld-empty">暂无可用辅助标签</p>
            </div>
          </div>

          <div v-if="relatedComposites.length > 0" class="cld-related" data-testid="composite-label-related">
            <span class="cld-field-label">相关现有组合（含你已选的组件，优先考虑复用）：</span>
            <ul class="cld-related-list">
              <li v-for="c in relatedComposites" :key="c.id" class="cld-related-item" :class="{ 'is-reuse': c.id === l1ReuseComposite?.id }">
                <span class="cld-related-name">{{ c.label }}</span>
                <span class="cld-related-chain">= {{ c.components.map(x => x.label).join(' × ') }}</span>
                <span v-if="c.id === l1ReuseComposite?.id" class="cld-reuse-tag" data-testid="composite-label-reuse-hint">创建将复用此组合</span>
                <span v-else-if="c.status !== 'active'" class="cld-related-status">（{{ c.status === 'disabled' ? '已禁用' : c.status }}）</span>
              </li>
            </ul>
          </div>

          <p v-if="error" class="cld-error" data-testid="composite-label-error">{{ error }}</p>
          <p v-if="resultMessage" class="cld-result" :data-outcome="resultOutcome" data-testid="composite-label-result">{{ resultMessage }}</p>
        </div>

        <div class="cld-footer">
          <button type="button" class="cld-btn" @click="emit('cancel')">关闭</button>
          <button
            type="button"
            class="cld-btn cld-btn--primary"
            :disabled="!canSubmit"
            data-testid="composite-label-submit"
            @click="submitCreate"
          >
            <Icon v-if="submitting" icon="mdi:loading" width="14" class="animate-spin" />
            创建
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.cld-overlay { position: fixed; inset: 0; z-index: 60; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.45); }
.cld-card { display: flex; flex-direction: column; width: min(560px, 92vw); max-height: 86vh; border: 1px solid var(--color-border-medium); border-radius: 12px; background: var(--color-bg-elevated); }
.cld-header { display: flex; align-items: center; justify-content: space-between; padding: 0.85rem 1.1rem; border-bottom: 1px solid var(--color-border-subtle); }
.cld-title { font-family: serif; font-size: 0.95rem; font-weight: 600; color: var(--color-text-primary); }
.cld-close { display: flex; align-items: center; justify-content: center; width: 26px; height: 26px; border: none; border-radius: 8px; background: none; color: var(--color-text-muted); cursor: pointer; }
.cld-close:hover { background: var(--color-bg-hover); color: var(--color-text-secondary); }
.cld-body { flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 0.85rem; padding: 1rem 1.1rem; }
.cld-field { display: flex; flex-direction: column; gap: 0.3rem; }
.cld-field-label { font-size: 0.72rem; color: var(--color-text-secondary); }
.cld-input { padding: 0.45rem 0.6rem; border: 1px solid var(--color-border-medium); border-radius: 8px; background: var(--color-bg-base); color: var(--color-text-primary); font-size: 0.82rem; }
.cld-input:focus { outline: none; border-color: var(--color-accent); }
.cld-input--search { width: 180px; }
.cld-selected { display: flex; flex-direction: column; gap: 0.35rem; }
.cld-selected-list { display: flex; flex-direction: column; gap: 0.25rem; }
.cld-selected-item { display: flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.5rem; border: 1px solid var(--color-border-subtle); border-radius: 8px; background: var(--color-bg-sunken); }
.cld-selected-pos { flex: 0 0 auto; width: 18px; font-family: ui-monospace, monospace; font-size: 0.68rem; color: var(--color-text-muted); }
.cld-selected-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.78rem; color: var(--color-text-primary); }
.cld-move, .cld-remove { display: flex; align-items: center; justify-content: center; width: 20px; height: 20px; border: 1px solid var(--color-border-medium); border-radius: 6px; background: none; color: var(--color-text-muted); font-size: 0.7rem; cursor: pointer; }
.cld-move:disabled { opacity: 0.35; cursor: default; }
.cld-move:hover:not(:disabled), .cld-remove:hover { border-color: var(--color-accent); color: var(--color-accent); }
.cld-picker { display: flex; flex-direction: column; gap: 0.4rem; }
.cld-picker-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 0.6rem; }
.cld-picker-list { display: flex; flex-wrap: wrap; gap: 0.3rem; max-height: 180px; overflow-y: auto; padding: 0.4rem; border: 1px solid var(--color-border-subtle); border-radius: 8px; background: var(--color-bg-base); }
.cld-picker-item { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.25rem 0.5rem; border: 1px solid var(--color-border-subtle); border-radius: 999px; background: none; color: var(--color-text-secondary); font-size: 0.74rem; cursor: pointer; }
.cld-picker-item:hover { border-color: var(--color-accent); color: var(--color-accent); }
.cld-picker-item.is-selected { border-color: var(--color-accent); background: var(--color-accent-subtle); color: var(--color-accent); }
.cld-picker-item.is-disabled { opacity: 0.4; cursor: not-allowed; }
.cld-board-badge { padding: 0.05rem 0.35rem; border: 1px solid var(--color-accent, #4a7dbd); border-radius: 999px; background: var(--color-accent-subtle, rgba(74,125,189,0.1)); color: var(--color-accent, #4a7dbd); font-size: 0.62rem; line-height: 1.3; }
.cld-board-badge--muted { border-color: var(--color-border-medium); background: none; color: var(--color-text-muted); }
.cld-cooc-badge { padding: 0.05rem 0.35rem; border: 1px solid rgba(168,85,247,0.45); border-radius: 999px; background: rgba(168,85,247,0.1); color: #a855f7; font-size: 0.62rem; line-height: 1.3; }
.cld-related { display: flex; flex-direction: column; gap: 0.3rem; padding: 0.5rem 0.6rem; border: 1px dashed var(--color-border-medium); border-radius: 8px; background: var(--color-bg-sunken); }
.cld-related-list { display: flex; flex-direction: column; gap: 0.25rem; margin: 0; padding: 0; list-style: none; }
.cld-related-item { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; font-size: 0.74rem; }
.cld-related-item.is-reuse { border-left: 2px solid var(--color-accent); padding-left: 0.4rem; }
.cld-related-name { font-weight: 600; color: var(--color-text-primary); }
.cld-related-chain { color: var(--color-text-secondary); }
.cld-reuse-tag { padding: 0.05rem 0.4rem; border: 1px solid var(--color-accent); border-radius: 999px; background: var(--color-accent-subtle, rgba(74,125,189,0.1)); color: var(--color-accent); font-size: 0.64rem; }
.cld-related-status { color: var(--color-text-muted); font-size: 0.68rem; }
.cld-loading, .cld-empty { display: flex; align-items: center; gap: 0.35rem; padding: 0.6rem; font-size: 0.75rem; color: var(--color-text-muted); }
.cld-error, .cld-error-inline { font-size: 0.74rem; color: var(--color-danger, #c0392b); }
.cld-result { padding: 0.45rem 0.6rem; border: 1px solid var(--color-success-border, rgba(61,138,74,0.3)); border-radius: 8px; background: var(--color-success-bg, rgba(61,138,74,0.08)); font-size: 0.76rem; color: var(--color-success, #3d8a4a); }
.cld-footer { display: flex; justify-content: flex-end; gap: 0.5rem; padding: 0.75rem 1.1rem; border-top: 1px solid var(--color-border-subtle); }
.cld-btn { padding: 0.4rem 0.9rem; border: 1px solid var(--color-border-medium); border-radius: 8px; background: none; color: var(--color-text-secondary); font-size: 0.78rem; cursor: pointer; }
.cld-btn:hover { border-color: var(--color-border-strong); color: var(--color-text-primary); }
.cld-btn--primary { border-color: var(--color-accent); background: var(--color-accent); color: var(--color-accent-contrast, #fff); }
.cld-btn--primary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
