<script setup lang="ts">
/**
 * 「AI 模型」section（ai-provider-model-overhaul 4.2 交付 4：供应商与模型管理）。
 * AppSectionHeader + VendorCard 列表 + 两段式 VendorDialog；数据源为
 * useAIVendors / useAIModels / useAIRoutes 三件套（父层单实例，卡片经 props 下发）。
 * 文件名与 section key（ai-providers）保持不变——ui-design「导航零变化」。
 */
import { Icon } from '@iconify/vue'
import { computed, onMounted, ref } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import { useNotify } from '~/composables/useNotify'
import { useAIModels } from '../composables/useAIModels'
import { useAIRoutes } from '../composables/useAIRoutes'
import { useAIVendors } from '../composables/useAIVendors'
import type { AIModel, AIModelUpdateRequest, AIVendor, AIVendorUpsertRequest, VendorModelsRefreshResult } from '~/types'
import VendorCard from './VendorCard.vue'
import VendorDialog from './VendorDialog.vue'

const notify = useNotify()

const vendorsCtx = useAIVendors()
const modelsCtx = useAIModels()
const routesCtx = useAIRoutes()

const vendors = vendorsCtx.vendors
const vendorsLoading = vendorsCtx.loading
const vendorsError = vendorsCtx.error
const models = modelsCtx.models
const modelsLoading = modelsCtx.loading
const modelsError = modelsCtx.error

const loading = computed(() => vendorsLoading.value || modelsLoading.value)
const listError = computed(() => vendorsError.value || modelsError.value)

/** vendor_id → 模型行 */
const modelsByVendor = computed(() => {
  const map = new Map<number, AIModel[]>()
  for (const m of models.value) {
    const list = map.get(m.vendor_id)
    if (list) list.push(m)
    else map.set(m.vendor_id, [m])
  }
  return map
})

function modelsOf(vendorId: number): AIModel[] {
  return modelsByVendor.value.get(vendorId) ?? []
}

/** vendor_id → 真实绑定条数（跨能力 route_models 全量统计；删除确认文案口径，区别于去重能力数） */
function bindingsCountOf(vendorId: number): number {
  let count = 0
  for (const r of routesCtx.routes.value) {
    for (const b of r.route_models) {
      if (b.vendor.id === vendorId) count += 1
    }
  }
  return count
}

// ---- 展开态 ----
const expandedIds = ref<number[]>([])

function toggleExpand(id: number) {
  expandedIds.value = expandedIds.value.includes(id)
    ? expandedIds.value.filter(x => x !== id)
    : [...expandedIds.value, id]
}

// ---- 刷新模型列表（loading 锁 + diff toast；失败保留旧清单） ----
const refreshingId = ref<number | null>(null)
const refreshDiffs = ref<Record<number, VendorModelsRefreshResult>>({})

async function onRefresh(vendor: AIVendor) {
  if (refreshingId.value !== null) return
  refreshingId.value = vendor.id
  try {
    const diff = await vendorsCtx.refreshModels(vendor.id)
    if (!diff) {
      // 刷新失败：库内清单保持刷新前状态（旧清单不动），仅 toast 错误
      notify.error(vendorsCtx.error.value || '刷新模型列表失败')
      return
    }
    await modelsCtx.fetchModels()
    refreshDiffs.value = { ...refreshDiffs.value, [vendor.id]: diff }
    notify.success(`刷新完成：新增 ${diff.added.length} 个 · stale ${diff.stale.length} 个`)
  } finally {
    refreshingId.value = null
  }
}

// ---- 新增/编辑弹窗 ----
const dialogOpen = ref(false)
const dialogVendor = ref<AIVendor | null>(null)
const dialogSaving = ref(false)

function openCreate() {
  dialogVendor.value = null
  dialogOpen.value = true
}

function openEdit(vendor: AIVendor) {
  dialogVendor.value = vendor
  dialogOpen.value = true
}

async function onSave(payload: AIVendorUpsertRequest) {
  dialogSaving.value = true
  try {
    if (dialogVendor.value) {
      const ok = await vendorsCtx.updateVendor(dialogVendor.value.id, payload)
      if (!ok) {
        notify.error(vendorsCtx.error.value || '更新供应商失败')
        return
      }
      dialogOpen.value = false
      notify.success('已保存')
      await modelsCtx.fetchModels()
    } else {
      const result = await vendorsCtx.createVendor(payload)
      if (!result) {
        notify.error(vendorsCtx.error.value || '新增供应商失败')
        return
      }
      dialogOpen.value = false
      await modelsCtx.fetchModels()
      // 保存成功自动展开新卡片（模型清单全部未启用，等用户勾选）
      expandedIds.value = [...expandedIds.value, result.id]
      if (result.models_refreshed) {
        notify.success(`已保存，拉取到 ${result.added ?? 0} 个模型`)
      } else {
        notify.warn(`已保存（清单拉取失败：${result.refresh_error ?? '未知错误'}）`)
      }
    }
  } finally {
    dialogSaving.value = false
  }
}

// ---- 删除（级联确认在 VendorCard 内；这里执行删除 + 路由链重载） ----
async function onDelete(vendor: AIVendor) {
  const result = await vendorsCtx.removeVendor(vendor.id)
  if (!result) {
    notify.error(vendorsCtx.error.value || '删除供应商失败')
    return
  }
  notify.success(`已删除 ${result.deleted_models} 个模型、解除 ${result.detached_bindings} 条绑定`)
  expandedIds.value = expandedIds.value.filter(id => id !== vendor.id)
  const diffs = { ...refreshDiffs.value }
  delete diffs[vendor.id]
  refreshDiffs.value = diffs
  // 模型缓存与能力路由链同步（后端已级联解绑）
  await Promise.all([modelsCtx.fetchModels(), routesCtx.fetchRoutes()])
}

// ---- 清单行配置 ----
async function onUpdateModel(model: AIModel, patch: AIModelUpdateRequest) {
  const ok = await modelsCtx.updateModel(model.id, patch)
  if (!ok) notify.error(modelsCtx.error.value || '更新模型失败')
}

function reload() {
  vendorsCtx.fetchVendors()
  modelsCtx.fetchModels()
  // 删除级联确认的 M 计数（bindingsCountOf）读 routes 绑定数据：挂载/重载时
  // 一并拉取（e2e ⑥ 发现缺此拉取导致确认弹窗恒显示 0）。
  routesCtx.fetchRoutes()
}

onMounted(() => {
  reload()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-start justify-between gap-3 flex-wrap">
      <AppSectionHeader
        title="AI 模型"
        description="管理 AI 供应商接入与模型清单；卡片上的「承载 N 能力」提示该供应商被多少能力路由依赖"
        icon-name="mdi:brain" />
      <AppButton variant="primary" size="sm" class="shrink-0" @click="openCreate">
        <Icon icon="mdi:plus" width="14" height="14" /> 新增供应商
      </AppButton>
    </div>

    <!-- loading：卡片骨架行 -->
    <div v-if="loading && vendors.length === 0" class="py-12 flex justify-center">
      <Icon icon="mdi:loading" width="32" height="32" class="animate-spin text-[var(--color-text-secondary)]" />
    </div>

    <!-- error：列表加载失败内联条 + 重试 -->
    <div v-else-if="listError && vendors.length === 0" class="error-bar">
      <Icon icon="mdi:alert-circle" width="14" height="14" class="shrink-0" />
      <span class="min-w-0 flex-1">{{ listError }}</span>
      <button class="retry-btn" type="button" @click="reload">重试</button>
    </div>

    <!-- empty：还没有供应商 -->
    <div v-else-if="vendors.length === 0" class="empty-state">
      <Icon icon="mdi:brain" width="28" height="28" />
      <p>还没有供应商</p>
      <p class="empty-hint">新增一个供应商并启用模型后，即可在「能力路由」中绑定</p>
      <AppButton variant="primary" size="sm" @click="openCreate">
        <Icon icon="mdi:plus" width="14" height="14" /> 新增供应商
      </AppButton>
    </div>

    <!-- 供应商卡片列表 -->
    <template v-else>
      <VendorCard
        v-for="v in vendors"
        :key="v.id"
        :vendor="v"
        :models="modelsOf(v.id)"
        :bindings-count="bindingsCountOf(v.id)"
        :expanded="expandedIds.includes(v.id)"
        :refreshing="refreshingId === v.id"
        :refresh-diff="refreshDiffs[v.id] ?? null"
        @toggle="toggleExpand(v.id)"
        @refresh="onRefresh(v)"
        @edit="openEdit(v)"
        @delete="onDelete(v)"
        @update-model="onUpdateModel"
      />
      <button class="add-vendor" type="button" @click="openCreate">
        <Icon icon="mdi:plus" width="14" height="14" /> 新增供应商
      </button>
    </template>

    <VendorDialog v-model="dialogOpen" :vendor="dialogVendor" :saving="dialogSaving" @save="onSave" />
  </div>
</template>

<style scoped>
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

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 40px 16px;
  border: 1px dashed var(--color-border-medium);
  border-radius: 12px;
  color: var(--color-text-secondary);
  font-size: 14px;
}
.empty-state p {
  margin: 0;
}
.empty-hint {
  font-size: 12.5px;
  color: var(--color-text-muted);
}

.add-vendor {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  padding: 10px;
  border-radius: 12px;
  border: 1px dashed var(--color-border-strong);
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 13px;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.add-vendor:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
</style>
