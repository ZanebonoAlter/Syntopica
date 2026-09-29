<script setup lang="ts">
/**
 * 降级观测块（ai-provider-model-overhaul 4.4，specs/ai-route-observability
 * 「观测面板界面契约」）：24h/7d 时间窗统计 + 降级事件流（时间/能力/顺位迁移/
 * 模型/类型标/错误码/定位）。数据源 ai_call_logs 只读聚合（getDegradations），
 * 本块无任何写操作。「定位」emit('locate', capability)，
 * 跳转（section=capability-routes + focus_capability）由宿主执行。
 */
import { onMounted, ref } from 'vue'
import { useAIAdminApi } from '~/api'
import { routeLabels } from '~/features/ai/composables/useAIRoutes'
import { formatDate } from '~/utils/date'
import type { DegradationEvent, DegradationReport } from '~/types'

const emit = defineEmits<{
  /** 事件「定位」：携带事件 capability，宿主跳能力路由 section 并脉冲高亮 */
  locate: [capability: string]
}>()

const WINDOWS = ['24h', '7d'] as const
type WindowKey = (typeof WINDOWS)[number]

const windowKey = ref<WindowKey>('24h')
const report = ref<DegradationReport | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

function capabilityLabel(capability: string): string {
  return routeLabels[capability] || capability
}

function formatOccurredAt(iso: string): string {
  return formatDate(iso, 'MM-DD HH:mm')
}

/** 顺位迁移：fallback 携带 from/to（0-based）→ 顺位 N→M；from/to 为空=全链失败；structured 无顺位概念 */
function moveText(evt: DegradationEvent): string {
  if (evt.type === 'structured') return '—'
  if (evt.from === null || evt.to === null) return '全链失败'
  return `顺位${evt.from + 1}→${evt.to + 1}`
}

function isChainFailure(evt: DegradationEvent): boolean {
  return evt.type === 'fallback' && (evt.from === null || evt.to === null)
}

function typeLabel(type: DegradationEvent['type']): string {
  return type === 'structured' ? '结构化降级' : '供应商失败'
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const res = await useAIAdminApi().getDegradations(windowKey.value)
    if (!res.success || !res.data) throw new Error(res.error || '加载降级观测失败')
    report.value = res.data
  } catch (err) {
    // State Matrix「错误恢复」：保留上次结果，仅追加内联提示
    error.value = err instanceof Error ? err.message : '加载降级观测失败'
  } finally {
    loading.value = false
  }
}

/** 时间窗切换即按新窗口重新请求（统计行与事件流一并刷新） */
function pickWindow(next: WindowKey) {
  if (next === windowKey.value || loading.value) return
  windowKey.value = next
  void load()
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="obs-card">
    <div class="obs-head">
      <h3 class="obs-head__title">降级观测</h3>
      <div v-if="report" class="obs-head__stats">
        <b>{{ report.stats.calls }}</b> 调用 ·
        <b>{{ report.stats.degradations }}</b> 降级 ·
        <b>{{ report.stats.failures }}</b> 失败
      </div>
      <div v-else class="obs-head__stats obs-head__stats--pending">— 调用 · — 降级 · — 失败</div>
      <div class="obs-seg" role="group" aria-label="观测时间窗">
        <button
          v-for="w in WINDOWS"
          :key="w"
          type="button"
          class="obs-seg__btn"
          :class="{ 'obs-seg__btn--on': windowKey === w }"
          @click="pickWindow(w)">
          {{ w }}
        </button>
      </div>
    </div>

    <!-- loading 骨架行（首载；窗口切换保留旧结果不闪骨架） -->
    <div v-if="loading && !report" class="obs-skeleton" aria-hidden="true">
      <div v-for="i in 3" :key="i" class="obs-skeleton__row" :style="{ animationDelay: `${i * 0.15}s` }" />
    </div>

    <!-- error：内联条 + 重试（有旧结果时叠加在列表上方） -->
    <div v-if="error" class="obs-error">
      <span class="min-w-0 flex-1">{{ error }}</span>
      <button class="obs-error__retry" type="button" @click="load">重试</button>
    </div>

    <template v-if="report">
      <!-- 空窗口态 -->
      <div v-if="report.events.length === 0" class="obs-empty">窗口内无记录</div>

      <!-- 事件流（只读） -->
      <div v-else class="evt-list">
        <div v-for="(evt, index) in report.events" :key="index" class="evt-row">
          <span class="evt-row__time">{{ formatOccurredAt(evt.occurred_at) }}</span>
          <span class="evt-row__cap" :title="capabilityLabel(evt.capability)">{{ capabilityLabel(evt.capability) }}</span>
          <span class="evt-row__move" :class="{ 'evt-row__move--fail': isChainFailure(evt) }">{{ moveText(evt) }}</span>
          <span class="evt-row__model" :title="evt.model">{{ evt.model }}</span>
          <span class="evt-row__type" :class="`evt-row__type--${evt.type}`">{{ typeLabel(evt.type) }}</span>
          <span class="evt-row__code" :title="evt.code">{{ evt.code }}</span>
          <button class="evt-row__locate" type="button" @click="emit('locate', evt.capability)">定位</button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.obs-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  padding: 14px 20px 16px;
}

.obs-head {
  display: flex;
  align-items: baseline;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}

.obs-head__title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.obs-head__stats {
  font-size: 12.5px;
  color: var(--color-text-secondary);
}

.obs-head__stats b {
  font-weight: 600;
  color: var(--color-text-primary);
}

.obs-head__stats--pending {
  color: var(--color-text-muted);
}

.obs-seg {
  display: inline-flex;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  overflow: hidden;
  margin-left: auto;
}

.obs-seg__btn {
  border: none;
  border-radius: 0;
  padding: 3px 14px;
  font-size: 11.5px;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.obs-seg__btn--on {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.obs-skeleton {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 6px 0;
}

.obs-skeleton__row {
  height: 14px;
  border-radius: 4px;
  background: var(--color-bg-sunken);
  animation: obs-skel 1.2s ease-in-out infinite;
}

.obs-skeleton__row:first-child {
  width: 92%;
}

.obs-skeleton__row:nth-child(2) {
  width: 78%;
}

.obs-skeleton__row:nth-child(3) {
  width: 84%;
}

@keyframes obs-skel {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}

.obs-error {
  display: flex;
  align-items: center;
  gap: 8px;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 12px;
  margin-bottom: 8px;
  background: var(--color-error-subtle);
  border: 1px solid var(--color-error);
  color: var(--color-error);
}

.obs-error__retry {
  font-size: 11px;
  padding: 2px 10px;
  border-radius: 6px;
  border: 1px solid var(--color-error);
  background: transparent;
  color: var(--color-error);
  cursor: pointer;
  flex-shrink: 0;
}

.obs-error__retry:hover {
  background: var(--color-error-subtle);
}

.obs-empty {
  text-align: center;
  padding: 20px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.evt-list {
  display: flex;
  flex-direction: column;
}

.evt-row {
  display: flex;
  gap: 12px;
  align-items: baseline;
  padding: 6px 2px;
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: 12.5px;
  min-width: 0;
}

.evt-row:last-child {
  border-bottom: none;
}

.evt-row__time {
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  flex: none;
}

.evt-row__cap {
  font-weight: 600;
  color: var(--color-text-primary);
  flex: none;
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.evt-row__move {
  color: var(--color-warning);
  flex: none;
  white-space: nowrap;
}

.evt-row__move--fail {
  color: var(--color-error);
  font-weight: 500;
}

.evt-row__model {
  color: var(--color-text-secondary);
  min-width: 0;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 类型标两色：结构化降级=accent / 供应商失败=warning */
.evt-row__type {
  font-size: 10.5px;
  border-radius: 4px;
  padding: 0 7px;
  flex: none;
  white-space: nowrap;
}

.evt-row__type--structured {
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.evt-row__type--fallback {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
}

.evt-row__code {
  color: var(--color-text-muted);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.evt-row__locate {
  padding: 1px 10px;
  font-size: 11px;
  border-radius: 6px;
  border: 1px solid var(--color-border-strong);
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
  flex: none;
  transition: border-color 0.15s, background 0.15s;
}

.evt-row__locate:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

/* 窄屏（767.98px 统一断点）：事件流隐藏次要列（时间/错误码），模型名保留 */
@media (max-width: 767.98px) {
  .evt-row__time,
  .evt-row__code {
    display: none;
  }
}
</style>
