<script setup lang="ts">
/**
 * 「能力路由」section（ai-provider-model-overhaul 4.3：链式卡片升级）。
 * 每 capability 一条绑定链：链项 = 序号 + 模型@供应商 + 思考档下拉（ThinkingLevelSelect）
 * + 覆盖徽章 + ↑↓×；hover 模型名提示「另承载 N 个能力」；底部 + chips 按 vendor 分组
 * 追加（embedding 行只列 embedding 模型）。drag + ↑↓× 交互沿用既有实现。
 * 数据源 useAIRoutes / useAIModels 单实例自取；保存体 PUT
 * bindings=[{model_id, thinking_level}]（null=继承模型默认档），失败 toast 后端原文。
 * 深链 focus_capability（观测面板「定位」跳入）：滚动到对应 route-section 并脉冲高亮一次，
 * 随后清掉 query 参数（保留 section），刷新不重复触发。
 */
import { Icon } from '@iconify/vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import { useNotify } from '~/composables/useNotify'
import type { AIModel, ModelKind, ThinkingLevel } from '~/types'
import { useAIModels } from '../composables/useAIModels'
import { capabilityOrder, routeLabels, useAIRoutes } from '../composables/useAIRoutes'
import ThinkingLevelSelect from './ThinkingLevelSelect.vue'

/** 本地编辑态绑定项（route_models 投影） */
interface BindingEdit {
  model_id: number
  thinking_level: ThinkingLevel | null
}

interface ModelInfo {
  name: string
  vendorName: string
  kind: ModelKind
  supportsXhighMax: boolean
  defaultLevel: ThinkingLevel | null
}

const notify = useNotify()
const routesCtx = useAIRoutes()
const modelsCtx = useAIModels()
const route = useRoute()
const router = useRouter()

/** 保存锁（saving）由 useAIRoutes 维护；首屏加载错误取两数据源第一条 */
const saving = routesCtx.saving
const loadError = computed(() => routesCtx.error.value || modelsCtx.error.value)

// ---- 本地编辑态（初始 = route_models[].thinking_level，保存前所有调档/增删/换位只动这里） ----
const edits = ref<Record<string, BindingEdit[]>>({})
const initialLoading = ref(true)

function chain(capability: string): BindingEdit[] {
  return edits.value[capability] ?? []
}

function hydrateEdits() {
  const next: Record<string, BindingEdit[]> = {}
  for (const cap of capabilityOrder) {
    const route = routesCtx.routeByCapability.value[cap]
    next[cap] = route
      ? route.route_models
          .slice()
          .sort((a, b) => a.priority - b.priority)
          .map(b => ({ model_id: b.model_id, thinking_level: b.thinking_level }))
      : []
  }
  edits.value = next
}

// ---- 链项展示信息（模型清单优先，缺失时回退路由绑定内嵌快照） ----
function modelInfo(capability: string, modelId: number): ModelInfo | null {
  const m = modelsCtx.models.value.find(x => x.id === modelId)
  if (m) {
    return {
      name: m.name,
      vendorName: m.vendor?.name ?? '',
      kind: m.model_kind,
      supportsXhighMax: m.supports_xhigh_max,
      defaultLevel: m.thinking_level,
    }
  }
  const binding = routesCtx.routeByCapability.value[capability]?.route_models.find(b => b.model_id === modelId)
  if (binding) {
    return {
      name: binding.model.name,
      vendorName: binding.vendor.name,
      kind: binding.model.model_kind,
      supportsXhighMax: binding.model.supports_xhigh_max,
      defaultLevel: binding.model.thinking_level,
    }
  }
  return null
}

function displayName(capability: string, modelId: number): string {
  const info = modelInfo(capability, modelId)
  if (!info) return `#${modelId}`
  return info.vendorName ? `${info.name}@${info.vendorName}` : info.name
}

/** 扇出反向视角：该模型被绑定的去重 capability 数 - 当前能力（=其他能力数） */
function otherCapabilityCount(modelId: number, currentCapability: string): number {
  let count = 0
  for (const r of routesCtx.routes.value) {
    if (r.capability === currentCapability) continue
    if (r.route_models.some(b => b.model_id === modelId)) count += 1
  }
  return count
}

function fanoutTip(capability: string, modelId: number): string {
  const name = modelInfo(capability, modelId)?.name ?? `#${modelId}`
  const n = otherCapabilityCount(modelId, capability)
  return n > 0 ? `${name} 另承载 ${n} 个能力` : '未承载其他能力'
}

// ---- 链操作（调档/增删/换位即时反映到本地状态） ----
function onLevelChange(capability: string, modelId: number, value: ThinkingLevel | null) {
  const binding = chain(capability).find(b => b.model_id === modelId)
  if (binding) binding.thinking_level = value
}

function inChain(capability: string, modelId: number): boolean {
  return chain(capability).some(b => b.model_id === modelId)
}

function addModel(capability: string, modelId: number) {
  if (inChain(capability, modelId)) return
  edits.value[capability] = [...chain(capability), { model_id: modelId, thinking_level: null }]
}

function removeModel(capability: string, modelId: number) {
  edits.value[capability] = chain(capability).filter(b => b.model_id !== modelId)
}

/** chips 点选：不在链中→链尾追加；已在链中→移除（同 ×，可重加） */
function toggleModel(capability: string, modelId: number) {
  if (inChain(capability, modelId)) removeModel(capability, modelId)
  else addModel(capability, modelId)
}

function moveModel(capability: string, modelId: number, direction: -1 | 1) {
  const current = [...chain(capability)]
  const index = current.findIndex(b => b.model_id === modelId)
  const nextIndex = index + direction
  if (index < 0 || nextIndex < 0 || nextIndex >= current.length) return
  const cur = current[index]
  const nxt = current[nextIndex]
  if (!cur || !nxt) return
  current[index] = nxt
  current[nextIndex] = cur
  edits.value[capability] = current
}

// ---- drag 换位（沿用既有实现，作用于本地编辑态） ----
const draggingCapability = ref<string | null>(null)
const draggingModelId = ref<number | null>(null)

function handleDragStart(capability: string, modelId: number) {
  draggingCapability.value = capability
  draggingModelId.value = modelId
}

function handleDragEnd() {
  draggingCapability.value = null
  draggingModelId.value = null
}

function handleDropOnModel(capability: string, targetModelId: number) {
  if (draggingCapability.value !== capability || draggingModelId.value === null) {
    handleDragEnd()
    return
  }
  const current = [...chain(capability)]
  const fromIdx = current.findIndex(b => b.model_id === draggingModelId.value)
  const toIdx = current.findIndex(b => b.model_id === targetModelId)
  if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) {
    handleDragEnd()
    return
  }
  const [moved] = current.splice(fromIdx, 1)
  if (!moved) {
    handleDragEnd()
    return
  }
  current.splice(toIdx, 0, moved)
  edits.value[capability] = current
  handleDragEnd()
}

// ---- chips：usable 模型按 vendor 分组（embedding 行只列 embedding 模型） ----
function chipGroups(capability: string): Array<{ vendorName: string, models: AIModel[] }> {
  const groups = new Map<string, AIModel[]>()
  for (const m of modelsCtx.usableModelsForCapability(capability)) {
    if (!m.vendor) continue
    const list = groups.get(m.vendor.name)
    if (list) list.push(m)
    else groups.set(m.vendor.name, [m])
  }
  return Array.from(groups.entries()).map(([vendorName, models]) => ({ vendorName, models }))
}

// ---- 保存：逐能力 PUT bindings（空链也发 bindings:[] 以持久化清空；失败 toast 后端原文并停在失败处，未保存能力本地态保留） ----
async function saveRoutes() {
  const hasBindings = capabilityOrder.some(cap => chain(cap).length > 0)
  if (!hasBindings) {
    notify.error('至少为一条能力路由配置一个模型')
    return
  }
  for (const cap of capabilityOrder) {
    const bindings = chain(cap)
    const ok = await routesCtx.saveRoute(cap, {
      bindings: bindings.map(b => ({ model_id: b.model_id, thinking_level: b.thinking_level })),
    })
    if (!ok) {
      notify.error(routesCtx.error.value || `保存 ${routeLabels[cap] || cap} 路由失败`)
      return
    }
  }
  notify.success('能力路由绑定已保存')
  hydrateEdits()
}

// ---- 观测面板「定位」跳入：滚动到对应 route-section + 脉冲高亮一次 ----
const rootEl = ref<HTMLElement | null>(null)
const pulsingCapability = ref<string | null>(null)
const PULSE_MS = 1400
let pulseTimer: ReturnType<typeof setTimeout> | null = null

function focusSection(capability: string) {
  if (!rootEl.value) return
  const el = rootEl.value.querySelector<HTMLElement>(`[data-route-section="${capability}"]`)
  if (!el) return
  el.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  pulsingCapability.value = capability
  if (pulseTimer) clearTimeout(pulseTimer)
  pulseTimer = setTimeout(() => {
    pulsingCapability.value = null
  }, PULSE_MS)
}

async function loadData() {
  await Promise.all([routesCtx.fetchRoutes(), modelsCtx.fetchModels()])
  initialLoading.value = false
  hydrateEdits()
}

/** 消费 focus_capability：聚焦脉冲一次后清掉该 query 参数（保留 section 等其余参数） */
async function consumeFocusCapability() {
  const focus = typeof route.query.focus_capability === 'string' ? route.query.focus_capability : null
  if (!focus || !capabilityOrder.includes(focus)) return
  await nextTick()
  if (!rootEl.value?.querySelector(`[data-route-section="${focus}"]`)) return
  focusSection(focus)
  const { focus_capability: _omit, ...rest } = route.query
  void router.replace({ query: rest })
}

onMounted(async () => {
  await loadData()
  await consumeFocusCapability()
})

// 组件挂载期间 query 再变化（非重挂载路径）同样定位；首载期间交给 onMounted 处理
watch(() => route.query.focus_capability, () => {
  if (!initialLoading.value) void consumeFocusCapability()
})

onBeforeUnmount(() => {
  if (pulseTimer) clearTimeout(pulseTimer)
})
</script>

<template>
  <div ref="rootEl" class="route-card">
    <div class="route-card__header">
      <AppSectionHeader
        title="能力路由"
        description="按能力分配模型与降级顺序；每项可单独指定思考档（默认继承模型配置）"
        icon-name="mdi:transit-connection-variant" />
      <AppButton variant="primary" size="sm" :disabled="saving" @click="saveRoutes">
        <Icon v-if="saving" icon="mdi:loading" width="12" height="12" class="animate-spin" />
        保存路由
      </AppButton>
    </div>

    <!-- loading：route-section 骨架 -->
    <div v-if="initialLoading" class="route-card__state">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin" style="color: var(--color-text-muted)" />
    </div>

    <!-- error：列表加载失败内联条 + 重试 -->
    <div v-else-if="loadError" class="route-card__state">
      <div class="error-bar">
        <Icon icon="mdi:alert-circle" width="14" height="14" class="shrink-0" />
        <span class="min-w-0 flex-1">{{ loadError }}</span>
        <button class="retry-btn" type="button" @click="loadData">重试</button>
      </div>
    </div>

    <div v-else class="route-card__sections">
      <div
        v-for="capability in capabilityOrder"
        :key="capability"
        class="route-section"
        :class="{ 'route-section--pulse': pulsingCapability === capability }"
        :data-route-section="capability"
      >
        <div class="route-section__title-row">
          <div
            class="route-section__badge"
            :class="chain(capability).length > 0 ? 'route-section__badge--active' : 'route-section__badge--empty'">
            {{ (routeLabels[capability] || capability).charAt(0) }}
          </div>
          <span class="route-section__name">{{ routeLabels[capability] || capability }}</span>
          <span class="route-section__count">{{ chain(capability).length }} 模型</span>
        </div>

        <!-- 空链：虚线引导（沿用现有空态） -->
        <div v-if="chain(capability).length === 0" class="route-section__empty">
          点击下方按钮添加模型
        </div>

        <TransitionGroup v-else tag="div" name="bind" class="route-section__providers">
          <div
            v-for="(binding, index) in chain(capability)"
            :key="binding.model_id"
            draggable="true"
            class="route-provider"
            :class="{
              'route-provider--dragging': draggingCapability === capability && draggingModelId === binding.model_id,
            }"
            @dragstart="handleDragStart(capability, binding.model_id)"
            @dragend="handleDragEnd"
            @dragover.prevent
            @drop.prevent="handleDropOnModel(capability, binding.model_id)"
          >
            <span class="route-provider__rank" :class="index === 0 ? 'route-provider__rank--first' : ''">{{ index + 1 }}</span>
            <Icon icon="mdi:drag" width="12" height="12" class="route-provider__drag" />

            <!-- 模型@供应商 + hover 扇出提示 -->
            <div class="route-provider__model" :title="displayName(capability, binding.model_id)">
              <span class="route-provider__name">{{ displayName(capability, binding.model_id) }}</span>
              <span class="fanout-tip">{{ fanoutTip(capability, binding.model_id) }}</span>
            </div>

            <!-- 覆盖徽章：非继承档时显示（accent-subtle 底色） -->
            <span v-if="binding.thinking_level !== null" class="route-provider__override">覆盖</span>

            <!-- 思考档下拉；embedding 绑定不携带覆盖值（spec 禁止） -->
            <ThinkingLevelSelect
              v-if="modelInfo(capability, binding.model_id)?.kind !== 'embedding'"
              :model-value="binding.thinking_level"
              :model-default="modelInfo(capability, binding.model_id)?.defaultLevel ?? null"
              :supports-xhigh-max="modelInfo(capability, binding.model_id)?.supportsXhighMax ?? true"
              class="route-provider__level"
              @update:model-value="(value: ThinkingLevel | null) => onLevelChange(capability, binding.model_id, value)"
            />
            <span v-else class="route-provider__dash" title="embedding 绑定不携带思考档">—</span>

            <div class="flex items-center gap-0.5 shrink-0">
              <button class="route-icon-btn" :disabled="index === 0" title="上移" @click="moveModel(capability, binding.model_id, -1)">
                <Icon icon="mdi:chevron-up" width="14" height="14" />
              </button>
              <button class="route-icon-btn" :disabled="index === chain(capability).length - 1" title="下移" @click="moveModel(capability, binding.model_id, 1)">
                <Icon icon="mdi:chevron-down" width="14" height="14" />
              </button>
              <button class="route-icon-btn route-icon-btn--danger" title="移除" @click="removeModel(capability, binding.model_id)">
                <Icon icon="mdi:close" width="13" height="13" />
              </button>
            </div>
          </div>
        </TransitionGroup>

        <!-- + chips 按 vendor 分组（点击追加链尾 / 移除已在链中的项） -->
        <div class="route-section__actions">
          <template v-if="chipGroups(capability).length > 0">
            <template v-for="group in chipGroups(capability)" :key="group.vendorName">
              <span class="route-chip-group">{{ group.vendorName }}：</span>
              <button
                v-for="m in group.models"
                :key="m.id"
                class="route-chip"
                :class="inChain(capability, m.id) ? 'route-chip--selected' : ''"
                @click="toggleModel(capability, m.id)">
                {{ inChain(capability, m.id) ? '✓' : '+' }} {{ m.name }}
              </button>
            </template>
          </template>
          <span v-else class="route-section__hint">
            先在「AI 模型」里启用一个 {{ capability === 'embedding' ? 'embedding' : 'llm' }} 类型的模型
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.route-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  overflow: hidden;
}

.route-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.route-card__header :deep(.app-section-header) {
  margin-bottom: 0;
}

.route-card__state {
  padding: 32px 20px;
  display: flex;
  justify-content: center;
}

.route-card__sections {
  display: flex;
  flex-direction: column;
}

.route-section {
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.route-section:last-child {
  border-bottom: none;
}

/* 定位跳入脉冲：一次后由 JS 移除 class */
.route-section--pulse {
  animation: section-pulse 1.2s ease;
}

@keyframes section-pulse {
  0% {
    background: var(--color-accent-subtle);
    box-shadow: inset 0 0 0 2px var(--color-accent);
  }
  100% {
    background: transparent;
    box-shadow: none;
  }
}

.route-section__title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.route-section__badge {
  width: 24px;
  height: 24px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  flex-shrink: 0;
}

.route-section__badge--active {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.route-section__badge--empty {
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
}

.route-section__name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
}

.route-section__count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.route-section__empty {
  text-align: center;
  padding: 12px 0;
  font-size: 11px;
  color: var(--color-text-muted);
  border-radius: 8px;
  border: 1px dashed var(--color-border-medium);
  margin-bottom: 12px;
}

.route-section__providers {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}

.route-provider {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-sunken);
  cursor: move;
  user-select: none;
  transition: opacity 0.15s, box-shadow 0.15s, border-color 0.15s;
}

.route-provider:hover {
  border-color: var(--color-border-medium);
}

.route-provider--dragging {
  opacity: 0.4;
  box-shadow: 0 0 0 2px var(--color-link);
}

/* chips 追加链尾入场过渡 / 换位移动过渡（TransitionGroup） */
.bind-enter-active {
  animation: bind-in 0.3s ease;
}

.bind-leave-active {
  display: none;
}

.bind-move {
  transition: transform 0.25s ease;
}

@keyframes bind-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

.route-provider__rank {
  width: 20px;
  height: 20px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.route-provider__rank--first {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.route-provider__drag {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.route-provider__model {
  position: relative;
  min-width: 0;
  flex: 1;
  cursor: help;
}

.route-provider__name {
  display: block;
  font-size: 13px;
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 扇出反向视角 tooltip：hover 模型名出现（「另承载 N 个能力」/「未承载其他能力」） */
.fanout-tip {
  display: none;
  position: absolute;
  left: 0;
  bottom: calc(100% + 6px);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  box-shadow: var(--shadow-strong, 0 8px 24px rgba(0, 0, 0, 0.12));
  font-size: 11.5px;
  color: var(--color-text-secondary);
  padding: 4px 10px;
  white-space: nowrap;
  z-index: 10;
}

.route-provider__model:hover .fanout-tip {
  display: block;
}

.route-provider__override {
  font-size: 10.5px;
  color: var(--color-accent);
  background: var(--color-accent-subtle);
  border-radius: 4px;
  padding: 0 6px;
  flex-shrink: 0;
}

.route-provider__level {
  flex-shrink: 0;
}

.route-provider__dash {
  color: var(--color-text-muted);
  font-size: 12px;
  flex-shrink: 0;
}

.route-icon-btn {
  padding: 2px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.route-icon-btn:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.route-icon-btn--danger:hover {
  background: var(--color-error-subtle);
  color: var(--color-error);
}

.route-icon-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.route-section__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.route-chip-group {
  font-size: 11px;
  color: var(--color-text-muted);
  margin-right: 2px;
}

.route-chip {
  padding: 2px 10px;
  font-size: 11px;
  font-weight: 500;
  border-radius: 99px;
  border: 1px solid var(--color-border-medium);
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.route-chip:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

.route-chip--selected {
  border-color: var(--color-accent);
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.route-section__hint {
  font-size: 11px;
  color: var(--color-text-muted);
  align-self: center;
}

.error-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 13px;
  background: var(--color-error-subtle);
  border: 1px solid var(--color-error);
  color: var(--color-error);
}

.retry-btn {
  font-size: 12px;
  padding: 3px 12px;
  border-radius: 6px;
  border: 1px solid var(--color-error);
  background: transparent;
  color: var(--color-error);
  cursor: pointer;
}

.retry-btn:hover {
  background: var(--color-error-subtle);
}

/* 窄屏（767.98px 统一断点）：链项允许换行，操作可达不破版 */
@media (max-width: 767.98px) {
  .route-provider {
    flex-wrap: wrap;
  }

  .route-provider__model {
    flex-basis: 100%;
    order: 5;
  }

  .route-provider__level {
    margin-left: auto;
  }
}
</style>
