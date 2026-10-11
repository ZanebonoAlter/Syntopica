<script setup lang="ts">
/**
 * 供应商卡片（ai-provider-model-overhaul 4.2 交付 2 + 4.5 模型清单管理面板）。
 * 卡片容器复用 route-card 形态（12px 圆角 + header 分隔线）；头部点击展开/收起
 * 模型清单。数据由父层（AIProviderManagement）单实例下发 props，本组件只做
 * 呈现 + 内联测试连接三态 + 删除确认，变更动作全部 emit 回父层。
 */
import { Icon } from '@iconify/vue'
import { computed, ref } from 'vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import VendorLogo from './VendorLogo.vue'
import { useConfirm } from '~/composables/useConfirm'
import { useAIVendors } from '../composables/useAIVendors'
import { findVendorPreset, isLocalBaseUrl } from '../presets'
import type { AIModel, AIModelUpdateRequest, AIVendor, ThinkingLevel } from '~/types'

/** 最近一次清单刷新 diff（「新」标数据源之一；父层在刷新成功后传入） */
interface VendorRefreshDiff {
  added: string[]
  stale: string[]
}

interface Props {
  vendor: AIVendor
  /** 该 vendor 的模型行（父层从 useAIModels 按 vendor_id 过滤） */
  models: AIModel[]
  expanded?: boolean
  /** 刷新中（loading 锁，防重复提交） */
  refreshing?: boolean
  refreshDiff?: VendorRefreshDiff | null
  /** 真实绑定条数（父层从路由链 route_models 按 vendor 统计；删除确认文案口径，区别于去重能力数） */
  bindingsCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  expanded: false,
  refreshing: false,
  refreshDiff: null,
  bindingsCount: 0,
})

const emit = defineEmits<{
  toggle: []
  refresh: []
  edit: []
  delete: []
  'update-model': [model: AIModel, patch: AIModelUpdateRequest]
}>()

const { confirm } = useConfirm()

// 测试连接是无缓存动作（不动 vendors 列表），leaf 组件自建实例安全
const vendorsCtx = useAIVendors()

const THINKING_LEVELS = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const
const LEVEL_LABELS: Record<string, string> = {
  off: '关', minimal: '微', low: '低', medium: '中', high: '高', xhigh: '超高', max: '极',
}

const preset = computed(() => findVendorPreset(props.vendor.preset_id))
const presetLogo = computed(() => preset.value?.logo ?? null)

/** 本地判定镜像后端 hostIsLocal(base_url)：仅本地 vendor 可配 embedding */
const isLocal = computed(() => isLocalBaseUrl(props.vendor.base_url))

const modelCount = computed(() => props.vendor.models_count ?? props.models.length)
const capabilityCount = computed(() => props.vendor.capabilities_count ?? 0)

// ---- 「新」标：refresh diff added 集合 或 last_seen_at 近 5 分钟内 ----
const NEW_WINDOW_MS = 5 * 60 * 1000

function isNewRow(m: AIModel): boolean {
  if (props.refreshDiff?.added.includes(m.name)) return true
  if (!m.last_seen_at) return false
  const seen = new Date(m.last_seen_at).getTime()
  return Number.isFinite(seen) && Date.now() - seen < NEW_WINDOW_MS
}

function isStaleRow(m: AIModel): boolean {
  return m.status === 'stale'
}

// ---- 测试连接三态（待测 / 测试中 / 成功·失败 内联展示） ----
const testState = ref<'idle' | 'testing' | 'ok' | 'fail'>('idle')
const testMessage = ref('')

async function onTest() {
  if (testState.value === 'testing') return
  testState.value = 'testing'
  testMessage.value = ''
  const message = await vendorsCtx.testConnection(props.vendor.id)
  if (message === null) {
    testState.value = 'fail'
    testMessage.value = vendorsCtx.error.value || '连接测试失败'
  } else {
    testState.value = 'ok'
    testMessage.value = message
  }
}

// ---- 删除（级联确认列明 N 模型 / M 绑定（真实绑定条数）；解除在父层） ----
async function onDelete() {
  const n = modelCount.value
  const m = props.bindingsCount
  const ok = await confirm({
    title: `删除供应商「${props.vendor.name}」？`,
    message: `将一并删除 ${n} 个模型、解除 ${m} 条能力路由绑定。该操作不可恢复。`,
    confirmText: '删除',
    danger: true,
  })
  if (ok) emit('delete')
}

// ---- 清单行变更 ----
function onKindChange(m: AIModel, kind: string) {
  // embedding 不得携带思考档（后端 UpsertModel 强校验），切类型时清空继承档
  const patch: AIModelUpdateRequest = kind === 'embedding'
    ? { model_kind: 'embedding', thinking_level: '' }
    : { model_kind: 'llm' }
  emit('update-model', m, patch)
}

/** 默认档候选：模型未声明 xh/max 时钳掉两高档（与 ThinkingLevelSelect 同口径） */
function levelsFor(m: AIModel): string[] {
  return m.supports_xhigh_max
    ? [...THINKING_LEVELS]
    : THINKING_LEVELS.filter(lv => lv !== 'xhigh' && lv !== 'max')
}

function onXhighChange(m: AIModel, checked: boolean) {
  // 勾掉 xh/max 时默认档若在高两档，自动回落 high（与下拉钳制同口径，一次 patch 落库）
  if (!checked && (m.thinking_level === 'xhigh' || m.thinking_level === 'max')) {
    emit('update-model', m, { supports_xhigh_max: false, thinking_level: 'high' })
    return
  }
  emit('update-model', m, { supports_xhigh_max: checked })
}

function onLevelChange(m: AIModel, level: string) {
  // 空串=清除继承档（后端 ThinkingLevel 空串归一为 NULL）
  emit('update-model', m, { thinking_level: level === '' ? '' : (level as ThinkingLevel) })
}
</script>

<template>
  <div class="vendor-card" :class="{ 'vendor-card--open': expanded }">
    <!-- 卡片头：点击展开/收起模型清单 -->
    <div class="vendor-head" role="button" tabindex="0"
      @click="emit('toggle')" @keydown.enter="emit('toggle')" @keydown.space.prevent="emit('toggle')">
      <span class="vendor-head__logo">
        <VendorLogo :symbol-id="presetLogo" :name="vendor.name" :size="16" />
      </span>
      <div class="vendor-head__id">
        <div class="vendor-head__name">{{ vendor.name }}</div>
        <div class="vendor-head__url" :title="vendor.base_url">{{ vendor.base_url }}</div>
      </div>
      <span class="cnt-chip">{{ modelCount }} 模型</span>
      <!-- 承载 N 能力常显徽章（单点依赖警示）：>0 用 accent-subtle 底色 -->
      <span class="fanout-badge" :class="{ 'fanout-badge--zero': capabilityCount === 0 }">
        {{ capabilityCount > 0 ? `承载 ${capabilityCount} 能力` : '未接入路由' }}
      </span>
      <Icon class="vendor-head__caret" :icon="expanded ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" height="16" />
    </div>

    <!-- 模型清单面板（展开式）；刷新中保留旧行渲染，仅追加提示行 -->
    <div v-if="expanded" class="vendor-panel">
      <div v-if="refreshing" class="panel-state">
        <Icon icon="mdi:loading" width="14" height="14" class="animate-spin" /> 正在刷新模型列表…
      </div>
      <div v-else-if="models.length === 0" class="panel-state">
        暂无模型，点击「刷新模型列表」从上游拉取
      </div>

      <div v-if="models.length > 0" class="model-table" role="table">
        <div class="model-row model-row--head" role="row">
          <span>启用</span>
          <span>模型</span>
          <span>类型</span>
          <span>默认思考档</span>
          <span class="col-xhigh" title="声明支持超高/极档（xhigh/max）">xh/max</span>
        </div>
        <div v-for="m in models" :key="m.id" class="model-row" :class="{ 'model-row--stale': isStaleRow(m) }" role="row">
          <span>
            <AppToggle :model-value="m.enabled" :disabled="isStaleRow(m)"
              @update:model-value="(v: boolean) => emit('update-model', m, { enabled: v })" />
          </span>
          <span class="col-name" :title="m.name">
            {{ m.name }}
            <span v-if="isNewRow(m)" class="new-tag">新</span>
            <span v-if="isStaleRow(m)" class="stale-tag">stale</span>
          </span>
          <span>
            <select class="ai-select model-kind-select" :value="m.model_kind"
              :disabled="isStaleRow(m) || !isLocal"
              :title="isLocal ? '模型类型' : '仅本地服务供应商可配置 embedding 模型'"
              @change="onKindChange(m, ($event.target as HTMLSelectElement).value)">
              <option value="llm">llm</option>
              <option v-if="isLocal" value="embedding">embedding</option>
            </select>
          </span>
          <span>
            <span v-if="m.model_kind === 'embedding'" class="dash" title="embedding 模型不携带思考档">—</span>
            <select v-else class="ai-select model-level-select" :value="m.thinking_level ?? ''"
              :disabled="isStaleRow(m)" title="默认思考档（留空继承全局默认 medium）"
              @change="onLevelChange(m, ($event.target as HTMLSelectElement).value)">
              <option value="">默认 (medium)</option>
              <option v-for="lv in levelsFor(m)" :key="lv" :value="lv">{{ LEVEL_LABELS[lv] }}（{{ lv }}）</option>
            </select>
          </span>
          <span class="col-xhigh">
            <input v-if="m.model_kind !== 'embedding'" type="checkbox" class="xhigh-check" :checked="m.supports_xhigh_max"
              :disabled="isStaleRow(m)" :title="m.supports_xhigh_max ? '支持超高/极档' : '未声明支持超高/极档'"
              @change="onXhighChange(m, ($event.target as HTMLInputElement).checked)" />
          </span>
        </div>
      </div>

      <!-- 操作行 -->
      <div class="panel-actions">
        <button class="panel-btn" type="button" :disabled="refreshing" @click="emit('refresh')">
          <Icon v-if="refreshing" icon="mdi:loading" width="12" height="12" class="animate-spin" />
          {{ refreshing ? '刷新中…' : '刷新模型列表' }}
        </button>
        <button class="panel-btn" type="button" :disabled="testState === 'testing'" @click="onTest">
          <Icon v-if="testState === 'testing'" icon="mdi:loading" width="12" height="12" class="animate-spin" />
          {{ testState === 'testing' ? '测试中…' : '测试连接' }}
        </button>
        <button class="panel-btn" type="button" @click="emit('edit')">编辑</button>
        <button class="panel-btn panel-btn--danger" type="button" @click="onDelete">删除</button>
      </div>

      <!-- 测试连接三态内联结果 -->
      <div v-if="testState === 'ok'" class="test-result test-result--ok">
        <Icon icon="mdi:check-circle" width="13" height="13" /> {{ testMessage }}
      </div>
      <div v-else-if="testState === 'fail'" class="test-result test-result--fail">
        <Icon icon="mdi:alert-circle" width="13" height="13" /> {{ testMessage }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.vendor-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  overflow: hidden;
}

.vendor-card--open .vendor-head {
  border-bottom: 1px solid var(--color-border-subtle);
}

.vendor-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 20px;
  cursor: pointer;
  min-width: 0;
}
.vendor-head:hover {
  background: var(--color-bg-hover);
}

.vendor-head__logo {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 12px;
  font-weight: 700;
  overflow: hidden;
}

.vendor-head__id {
  min-width: 0;
  flex: 1;
}

.vendor-head__name {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.vendor-head__url {
  font-size: 11.5px;
  color: var(--color-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cnt-chip {
  font-size: 11px;
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
  border-radius: 99px;
  padding: 1px 9px;
  flex: none;
  white-space: nowrap;
}

.fanout-badge {
  font-size: 11px;
  color: var(--color-accent);
  background: var(--color-accent-subtle);
  border-radius: 6px;
  padding: 1px 8px;
  flex: none;
  white-space: nowrap;
}
.fanout-badge--zero {
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
}

.vendor-head__caret {
  color: var(--color-text-muted);
  flex: none;
}

.vendor-panel {
  padding: 12px 20px 16px;
}

.panel-state {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 0;
  font-size: 12.5px;
  color: var(--color-text-muted);
}

/* 模型清单（grid 行，与 header 同列宽） */
.model-table {
  font-size: 12.5px;
}

.model-row {
  display: grid;
  grid-template-columns: 52px minmax(0, 1fr) 108px 168px 56px;
  gap: 8px;
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}
.model-row:last-child {
  border-bottom: none;
}

.model-row--head {
  font-size: 11px;
  font-weight: 500;
  color: var(--color-text-muted);
  border-bottom: 1px solid var(--color-border-medium);
}

.model-row--stale {
  opacity: 0.65;
}
.model-row--stale .col-name {
  color: var(--color-text-muted);
}

.col-name {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.col-xhigh {
  text-align: center;
}

.new-tag {
  background: var(--color-success-subtle);
  color: var(--color-success);
  font-size: 10.5px;
  padding: 0 6px;
  border-radius: 4px;
  margin-left: 6px;
}

.stale-tag {
  color: var(--color-warning);
  font-size: 11px;
  margin-left: 6px;
}

.dash {
  color: var(--color-text-muted);
}

.model-kind-select,
.model-level-select {
  width: 100%;
  padding: 3px 6px;
  font-size: 12px;
}

.xhigh-check {
  accent-color: var(--color-accent);
  cursor: pointer;
}

.panel-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.panel-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  padding: 3px 11px;
  border-radius: 6px;
  border: 1px solid var(--color-border-strong);
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.panel-btn:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
.panel-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.panel-btn--danger {
  color: var(--color-error);
  border-color: var(--color-error);
}
.panel-btn--danger:hover {
  background: var(--color-error-subtle);
  border-color: var(--color-error);
}

.test-result {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  padding: 5px 10px;
  border-radius: 6px;
  margin-top: 8px;
  min-width: 0;
}
.test-result--ok {
  background: var(--color-success-subtle);
  color: var(--color-success);
}
.test-result--fail {
  background: var(--color-error-subtle);
  color: var(--color-error);
}

/* 下拉与 ai-select 同款（scoped 内复刻基础形态） */
.ai-select {
  border: 1px solid var(--color-input-border);
  border-radius: 6px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s;
}
.ai-select:focus {
  border-color: var(--color-input-focus);
}
.ai-select:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (max-width: 767.98px) {
  .vendor-head {
    flex-wrap: wrap;
  }
  .model-row {
    grid-template-columns: 44px minmax(0, 1fr) 92px 132px 40px;
    gap: 6px;
  }
}
</style>
