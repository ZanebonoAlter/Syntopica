<script setup lang="ts">
/**
 * 「能力路由」section（tasks 8.3，FB-3+FB-5+FB-7：行式泳道重构）。
 * 布局：顶部 ModelPoolStrip（vendor 分组模型卡，拖拽/点击菜单双通道添加）+
 * 每能力一行 CapabilitySwimlane（行内自左向右 = 降级优先级，卡片间箭头即降级
 * 方向）；降级状态内嵌行内（useRouteHealth 聚合 24h 观测：失败卡红边/接管徽章/
 * 高亮箭头/行头健康点三态）。取代第一轮链式卡片 + chips 追加（FB-3 定稿形态，
 * FB-5 由列式修正为行式，FB-7 补点击菜单与卡内防溢出）。
 * 数据源 useAIRoutes / useAIModels / useAIVendors 单实例自取；保存体 PUT
 * bindings=[{model_id, thinking_level}]（null=继承模型默认档），priority 由
 * 行内顺序派生（后端契约不变）。深链 focus_capability（观测面板「定位」跳入）：
 * 滚动到对应泳道行并脉冲高亮一次，随后清掉 query 参数（保留 section）。
 */
import { Icon } from '@iconify/vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import { useNotify } from '~/composables/useNotify'
import type { ModelKind, ThinkingLevel } from '~/types'
import { useAIModels } from '../composables/useAIModels'
import { capabilityOrder, routeDescriptions, routeLabels, useAIRoutes } from '../composables/useAIRoutes'
import { useRouteHealth } from '../composables/useRouteHealth'
import { useAIVendors } from '../composables/useAIVendors'
import { findVendorPreset } from '../presets'
import CapabilitySwimlane, { type SwimCard } from './CapabilitySwimlane.vue'
import ModelPoolStrip, { type PickerCapability, type PoolGroup } from './ModelPoolStrip.vue'

/** 本地编辑态绑定项（route_models 投影；priority = 数组序） */
interface BindingEdit {
  model_id: number
  thinking_level: ThinkingLevel | null
}

const notify = useNotify()
const routesCtx = useAIRoutes()
const modelsCtx = useAIModels()
const vendorsCtx = useAIVendors()
const healthCtx = useRouteHealth()
const route = useRoute()
const router = useRouter()

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
    const r = routesCtx.routeByCapability.value[cap]
    next[cap] = r
      ? r.route_models
          .slice()
          .sort((a, b) => a.priority - b.priority)
          .map(b => ({ model_id: b.model_id, thinking_level: b.thinking_level }))
      : []
  }
  edits.value = next
}

// ---- 模型信息（模型清单优先，缺失时回退路由绑定内嵌快照） ----
interface ModelInfo {
  name: string
  vendorId: number
  vendorName: string
  logo: string | null
  kind: ModelKind
  supportsXhighMax: boolean
  defaultLevel: ThinkingLevel | null
}

function vendorLogo(vendorId: number): string | null {
  const v = vendorsCtx.vendors.value.find(x => x.id === vendorId)
  return v?.preset_id ? (findVendorPreset(v.preset_id)?.logo ?? null) : null
}

function modelInfo(capability: string, modelId: number): ModelInfo | null {
  const m = modelsCtx.models.value.find(x => x.id === modelId)
  if (m) {
    return {
      name: m.name,
      vendorId: m.vendor_id,
      vendorName: m.vendor?.name ?? '',
      logo: vendorLogo(m.vendor_id),
      kind: m.model_kind,
      supportsXhighMax: m.supports_xhigh_max,
      defaultLevel: m.thinking_level,
    }
  }
  const binding = routesCtx.routeByCapability.value[capability]?.route_models.find(b => b.model_id === modelId)
  if (binding) {
    return {
      name: binding.model.name,
      vendorId: binding.vendor.id,
      vendorName: binding.vendor.name,
      logo: vendorLogo(binding.vendor.id),
      kind: binding.model.model_kind,
      supportsXhighMax: binding.model.supports_xhigh_max,
      defaultLevel: binding.model.thinking_level,
    }
  }
  return null
}

/** 扇出反向视角：该模型被绑定的去重 capability 数 - 当前能力 */
function otherCapabilityCount(modelId: number, currentCapability: string): number {
  let count = 0
  for (const r of routesCtx.routes.value) {
    if (r.capability === currentCapability) continue
    if (r.route_models.some(b => b.model_id === modelId)) count += 1
  }
  return count
}

// ---- 泳道卡视图模型（含降级标注，FB-5；cardHealth 按当前行卡名序列做结构吻合校验） ----
function swimCards(capability: string): SwimCard[] {
  const list = chain(capability)
  const chainNames = list.map(b => modelInfo(capability, b.model_id)?.name ?? `#${b.model_id}`)
  return list.map((b, index) => {
    const info = modelInfo(capability, b.model_id)
    const h = healthCtx.cardHealth(capability, index, chainNames)
    return {
      modelId: b.model_id,
      name: info?.name ?? `#${b.model_id}`,
      vendorName: info?.vendorName ?? '',
      logo: info?.logo ?? null,
      kind: (info?.kind ?? 'llm') as 'llm' | 'embedding',
      defaultLevel: info?.defaultLevel ?? null,
      supportsXhighMax: info?.supportsXhighMax ?? true,
      thinkingLevel: b.thinking_level,
      fanoutOthers: otherCapabilityCount(b.model_id, capability),
      fail: h.fail,
      lastErr: h.lastErr,
      takeover: h.takeover,
    }
  })
}

function rowHealthOf(capability: string): { level: 'ok' | 'warn' | 'err', text: string } {
  const summary = healthCtx.rowHealth(capability, chain(capability).length > 1)
  return { level: summary.level, text: summary.text }
}

// ---- 模型池（usable 全集按 vendor 分组，FB-3） ----
const poolGroups = computed<PoolGroup[]>(() => {
  const groups = new Map<number, PoolGroup>()
  for (const m of modelsCtx.usableModels.value) {
    if (!m.vendor) continue
    let g = groups.get(m.vendor_id)
    if (!g) {
      g = { vendorName: m.vendor.name, logo: vendorLogo(m.vendor_id), models: [] }
      groups.set(m.vendor_id, g)
    }
    g.models.push({ id: m.id, name: m.name, kind: m.model_kind })
  }
  return Array.from(groups.values())
})

/** 点击池卡的候选能力清单（embedding 行只收 embedding 模型，FB-7 菜单过滤） */
function capabilitiesFor(model: { id: number, kind: ModelKind }): PickerCapability[] {
  return capabilityOrder.map((cap) => {
    const needKind: ModelKind = cap === 'embedding' ? 'embedding' : 'llm'
    const mismatch = model.kind !== needKind
    const inChain = !mismatch && chain(cap).some(b => b.model_id === model.id)
    return {
      capability: cap,
      label: routeLabels[cap] || cap,
      firstChar: (routeLabels[cap] || cap).charAt(0),
      disabled: mismatch,
      disabledReason: `仅 ${needKind}`,
      inChain,
      count: chain(cap).length,
    }
  })
}

// ---- 行操作（调档/增删/换位即时反映到本地编辑态） ----
function onLevelChange(capability: string, modelId: number, value: ThinkingLevel | null) {
  const binding = chain(capability).find(b => b.model_id === modelId)
  if (binding) binding.thinking_level = value
}

function inChain(capability: string, modelId: number): boolean {
  return chain(capability).some(b => b.model_id === modelId)
}

function insertAt(capability: string, modelId: number, targetModelId: number | null, before: boolean) {
  const current = [...chain(capability)]
  const binding: BindingEdit = { model_id: modelId, thinking_level: null }
  if (targetModelId === null) {
    current.push(binding)
  } else {
    const idx = current.findIndex(b => b.model_id === targetModelId)
    if (idx < 0) current.push(binding)
    else current.splice(before ? idx : idx + 1, 0, binding)
  }
  edits.value[capability] = current
}

function addModel(capability: string, modelId: number) {
  if (inChain(capability, modelId)) return
  insertAt(capability, modelId, null, true)
}

function removeModel(capability: string, modelId: number) {
  edits.value[capability] = chain(capability).filter(b => b.model_id !== modelId)
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

// ---- DnD（FB-3+FB-7 双通道：池→行拖入 + 行内换位；跨行拖动提示先移除） ----
const dragPoolModelId = ref<number | null>(null)
const dragLane = ref<{ capability: string, modelId: number } | null>(null)
const dragOverCapability = ref<string | null>(null)
const rejectCapability = ref<string | null>(null)
let rejectTimer: ReturnType<typeof setTimeout> | null = null

function kindOf(modelId: number): ModelKind {
  return modelsCtx.models.value.find(x => x.id === modelId)?.model_kind ?? 'llm'
}

function flashReject(capability: string) {
  rejectCapability.value = capability
  if (rejectTimer) clearTimeout(rejectTimer)
  rejectTimer = setTimeout(() => { rejectCapability.value = null }, 450)
}

/** 泳道行 drop 入口（targetModelId=null 行尾；before 仅对有 target 有效） */
function onDropIn(capability: string, targetModelId: number | null, before: boolean) {
  dragOverCapability.value = null
  // 行内换位
  if (dragLane.value) {
    const lane = dragLane.value
    if (lane.capability !== capability) {
      notify.warn('跨行移动：请先 × 移除，再到目标行拖入')
      dragLane.value = null
      return
    }
    const current = [...chain(capability)]
    const fromIdx = current.findIndex(b => b.model_id === lane.modelId)
    if (fromIdx < 0) { dragLane.value = null; return }
    const moved = current.splice(fromIdx, 1)[0]!
    let to: number
    if (targetModelId === null) to = current.length
    else {
      const idx = current.findIndex(b => b.model_id === targetModelId)
      if (idx < 0) to = current.length
      else {
        const base = idx < fromIdx ? idx : idx - 1
        to = before ? base : base + 1
      }
    }
    current.splice(Math.max(0, Math.min(to, current.length)), 0, moved)
    edits.value[capability] = current
    dragLane.value = null
    return
  }
  // 池卡拖入
  if (dragPoolModelId.value === null) return
  const modelId = dragPoolModelId.value
  const needKind: ModelKind = capability === 'embedding' ? 'embedding' : 'llm'
  if (kindOf(modelId) !== needKind) {
    flashReject(capability)
    notify.warn(`「${routeLabels[capability] || capability}」行只接受 ${needKind} 模型`)
    dragPoolModelId.value = null
    return
  }
  if (inChain(capability, modelId)) {
    notify.warn('该模型已在此行中')
    dragPoolModelId.value = null
    return
  }
  insertAt(capability, modelId, targetModelId, before)
  dragPoolModelId.value = null
}

// ---- 保存：逐能力 PUT bindings（空行也发 bindings:[] 持久化清空；失败 toast 停在失败处） ----
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

// ---- 观测面板「定位」跳入：滚动到对应泳道行 + 脉冲高亮一次 ----
const rootEl = ref<HTMLElement | null>(null)
const pulsingCapability = ref<string | null>(null)
const PULSE_MS = 1400
let pulseTimer: ReturnType<typeof setTimeout> | null = null

function focusSection(capability: string) {
  if (!rootEl.value) return
  const el = rootEl.value.querySelector<HTMLElement>(`[data-route-section="${capability}"]`)
  if (!el) return
  el.scrollIntoView?.({ behavior: 'smooth', block: 'center' })
  pulsingCapability.value = capability
  if (pulseTimer) clearTimeout(pulseTimer)
  pulseTimer = setTimeout(() => {
    pulsingCapability.value = null
  }, PULSE_MS)
}

async function loadData() {
  await Promise.all([
    routesCtx.fetchRoutes(),
    modelsCtx.fetchModels(),
    vendorsCtx.fetchVendors(),
    healthCtx.fetchHealth(),
  ])
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

watch(() => route.query.focus_capability, () => {
  if (!initialLoading.value) void consumeFocusCapability()
})

onBeforeUnmount(() => {
  if (pulseTimer) clearTimeout(pulseTimer)
  if (rejectTimer) clearTimeout(rejectTimer)
})
</script>

<template>
  <div ref="rootEl" class="route-card">
    <div class="route-card__header">
      <AppSectionHeader
        title="能力路由"
        description="每能力一行泳道：从模型池拖卡或点击添加；行内自左向右 = 降级优先级，红卡/接管徽章 = 24h 降级状态"
        icon-name="mdi:transit-connection-variant" />
      <AppButton variant="primary" size="sm" :disabled="saving" @click="saveRoutes">
        <Icon v-if="saving" icon="mdi:loading" width="12" height="12" class="animate-spin" />
        保存路由
      </AppButton>
    </div>

    <!-- loading：泳道骨架 -->
    <div v-if="initialLoading" class="route-card__state">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin" style="color: var(--color-text-muted)" />
    </div>

    <!-- error：加载失败内联条 + 重试 -->
    <div v-else-if="loadError" class="route-card__state">
      <div class="error-bar">
        <Icon icon="mdi:alert-circle" width="14" height="14" class="shrink-0" />
        <span class="min-w-0 flex-1">{{ loadError }}</span>
        <button class="retry-btn" type="button" @click="loadData">重试</button>
      </div>
    </div>

    <template v-else>
      <!-- 模型池横条（FB-3/FB-7：拖拽 + 点击菜单双通道） -->
      <ModelPoolStrip
        :groups="poolGroups"
        :capabilities-for="capabilitiesFor"
        @drag-start="dragPoolModelId = $event"
        @add="addModel"
      />

      <!-- 能力泳道行 ×N -->
      <CapabilitySwimlane
        v-for="capability in capabilityOrder"
        :key="capability"
        :capability="capability"
        :label="routeLabels[capability] || capability"
        :description="routeDescriptions[capability]"
        :cards="swimCards(capability)"
        :health-level="rowHealthOf(capability).level"
        :health-text="rowHealthOf(capability).text"
        :dragging-over="dragOverCapability === capability"
        :reject-flash="rejectCapability === capability"
        :pulsing="pulsingCapability === capability"
        @level-change="(modelId, v) => onLevelChange(capability, modelId, v)"
        @remove="modelId => removeModel(capability, modelId)"
        @move="(modelId, dir) => moveModel(capability, modelId, dir)"
        @drop-in="(targetId, before) => onDropIn(capability, targetId, before)"
        @card-drag-start="modelId => { dragLane = { capability, modelId } }"
        @card-drag-end="dragLane = null"
        @drag-over="dragOverCapability = capability"
        @drag-leave="dragOverCapability === capability && (dragOverCapability = null)"
      />

      <div class="route-card__foot">
        <span>行内自左向右 = 降级优先级（序号 1 首选）</span>
        <span>同一模型可出现在多行（扇出可见）</span>
        <span>embedding 行只收 embedding 模型</span>
        <span>红卡=近期失败 · 黄徽=接管 · 红箭头=正在降级</span>
      </div>
    </template>
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

.route-card__foot {
  padding: 9px 20px;
  border-top: 1px solid var(--color-border-subtle);
  font-size: 11.5px;
  color: var(--color-text-muted);
  display: flex;
  gap: 18px;
  flex-wrap: wrap;
}

.error-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 12.5px;
}

.retry-btn {
  border: 1px solid currentColor;
  background: none;
  border-radius: 6px;
  padding: 2px 10px;
  font-size: 12px;
  color: inherit;
  cursor: pointer;
  flex: none;
}
</style>
