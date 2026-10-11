<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAIAdminApi } from '~/api'
import type { AIHealthSnapshot } from '~/types'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import AppButton from '~/components/ui/AppButton.vue'
import DegradationPanel from './DegradationPanel.vue'
import { useHealthReprobe } from '~/composables/useHealthReprobe'

const router = useRouter()

const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const snapshot = ref<AIHealthSnapshot | null>(null)
const autoStart = ref(false)
const { reprobing, reprobeHealth } = useHealthReprobe()

const capabilityLabels: Record<string, string> = {
  summary: '文章总结',
  topic_tagging: '主题提取',
  digest_polish: '日报润色',
  embedding: '向量嵌入',
  feed_discovery: '订阅源发现',
}

function capabilityLabel(capability: string): string {
  return capabilityLabels[capability] || capability
}

function formatTime(iso: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

async function loadHealth() {
  loading.value = true
  error.value = null
  try {
    const res = await useAIAdminApi().getHealth()
    if (!res.success || !res.data) throw new Error(res.error || '加载 AI 健康状态失败')
    snapshot.value = res.data
    autoStart.value = res.data.auto_start_models
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载 AI 健康状态失败'
  } finally {
    loading.value = false
  }
}

async function toggleAutoStart(value: boolean) {
  if (saving.value) return
  saving.value = true
  error.value = null
  const previous = autoStart.value
  autoStart.value = value
  try {
    const res = await useAIAdminApi().setAutoStartModels(value)
    if (!res.success || !res.data) throw new Error(res.error || '保存失败')
    autoStart.value = res.data.enabled
  } catch (err) {
    autoStart.value = previous
    error.value = err instanceof Error ? err.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function handleReprobe() {
  error.value = null
  const snap = await reprobeHealth()
  if (snap) {
    snapshot.value = snap
    autoStart.value = snap.auto_start_models
  }
}

onMounted(() => { void loadHealth() })

/** 降级事件「定位」：跳能力路由 section，并携带 focus_capability 供链式卡片滚动+脉冲 */
function locateCapability(capability: string) {
  void router.push({ query: { section: 'capability-routes', focus_capability: capability } })
}
</script>

<template>
  <div class="space-y-4">
    <div class="health-card">
    <div class="health-card__header">
      <AppSectionHeader title="AI 健康状态" description="后端启动时探测各能力路由主模型的可达性" icon-name="mdi:heart-pulse" />
      <div class="flex items-center gap-2.5">
        <AppButton variant="ghost" size="sm" :loading="reprobing" @click="handleReprobe">重新检测</AppButton>
        <span v-if="snapshot" class="health-badge" :class="snapshot.healthy ? 'health-badge--ok' : 'health-badge--down'">
          <Icon :icon="snapshot.healthy ? 'mdi:check-circle' : 'mdi:alert-circle'" width="13" height="13" />
          {{ snapshot.healthy ? '健康' : '未就绪' }}
        </span>
      </div>
    </div>

    <div class="health-card__body">
      <div v-if="loading && !snapshot" class="py-8 flex justify-center">
        <Icon icon="mdi:loading" width="28" height="28" class="animate-spin" style="color: var(--color-text-muted)" />
      </div>

      <template v-else-if="snapshot">
        <p class="health-checked-at">
          上次检测：{{ snapshot.checked_at ? formatTime(snapshot.checked_at) : '检测中…（后端启动后首次检测完成前，分析任务不会运行）' }}
        </p>

        <div v-if="snapshot.routes.length === 0" class="health-empty">
          暂无路由健康数据（没有启用且绑定了模型的路由）
        </div>

        <table v-else class="health-table">
          <thead>
            <tr>
              <th>路由</th>
              <th>能力</th>
              <th>主模型</th>
              <th>供应商</th>
              <th>类型</th>
              <th>可达</th>
              <th>后端拉起</th>
              <th>上次检测</th>
              <th>错误</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="route in snapshot.routes" :key="route.route_name">
              <td class="health-table__name">{{ route.route_name }}</td>
              <td>{{ capabilityLabel(route.capability) }}</td>
              <td class="health-table__name">{{ route.primary_model }}</td>
              <td>{{ route.vendor }}</td>
              <td>{{ route.model_kind }}</td>
              <td>
                <span class="health-dot" :class="route.reachable ? 'health-dot--ok' : 'health-dot--down'">
                  {{ route.reachable ? '通' : '断' }}
                </span>
              </td>
              <td>
                <span v-if="route.launched_by_backend" class="health-launched">已拉起</span>
                <span v-else style="color: var(--color-text-muted)">—</span>
              </td>
              <td>{{ formatTime(route.last_checked) }}</td>
              <td class="health-table__error" :title="route.error">{{ route.error || '—' }}</td>
            </tr>
          </tbody>
        </table>

        <p class="health-note">健康快照由启动探测刷新；未就绪时后端每 60 秒自动重试，也可点「重新检测」立即重探。</p>
      </template>

      <div class="health-auto-start">
        <div class="flex items-center gap-2.5">
          <AppToggle :model-value="autoStart" :disabled="saving" @update:model-value="toggleAutoStart" />
          <span class="text-sm font-medium" style="color: var(--color-text-primary)">自动拉起本地模型（auto_start_models）</span>
        </div>
        <p class="text-xs mt-1.5" style="color: var(--color-text-muted)">
          开启后，后端启动时会自动拉起配了启动命令且探测不通的本地模型进程（如 llama.cpp）。命令将以后台方式执行，请确保命令可信。
        </p>
      </div>

      <div v-if="error" class="rounded-lg px-4 py-2.5 text-xs flex items-center gap-2" style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)">
        <Icon icon="mdi:alert-circle" width="14" height="14" /> {{ error }}
        <AppButton variant="ghost" size="sm" class="ml-auto" @click="loadHealth">重试</AppButton>
      </div>
    </div>
  </div>

  <!-- 降级观测块（ai-route-observability）：只读，位于健康快照下方 -->
  <DegradationPanel @locate="locateCapability" />
  </div>
</template>

<style scoped>
.health-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  overflow: hidden;
}

.health-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.health-card__header :deep(.app-section-header) {
  margin-bottom: 0;
}

.health-card__body {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.health-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 500;
  padding: 3px 10px;
  border-radius: 9999px;
  flex-shrink: 0;
}

.health-badge--ok {
  background: rgba(61, 138, 74, 0.12);
  color: var(--color-success);
}

.health-badge--down {
  background: var(--color-error-bg, rgba(196, 47, 60, 0.1));
  color: var(--color-error);
}

.health-checked-at {
  font-size: 12px;
  color: var(--color-text-muted);
  margin: 0;
}

.health-empty {
  text-align: center;
  padding: 24px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.health-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.health-table th {
  text-align: left;
  font-weight: 500;
  color: var(--color-text-muted);
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-border-subtle);
  white-space: nowrap;
}

.health-table td {
  padding: 8px;
  border-bottom: 1px solid var(--color-border-subtle);
  color: var(--color-text-secondary);
}

.health-table tr:last-child td {
  border-bottom: none;
}

.health-table__name {
  font-weight: 500;
  color: var(--color-text-primary);
}

.health-table__error {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted);
}

.health-dot {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 9999px;
  font-size: 11px;
  font-weight: 500;
}

.health-dot--ok {
  background: rgba(61, 138, 74, 0.12);
  color: var(--color-success);
}

.health-dot--down {
  background: var(--color-error-bg, rgba(196, 47, 60, 0.1));
  color: var(--color-error);
}

.health-launched {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 9999px;
  font-size: 11px;
  font-weight: 500;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.health-note {
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 0;
}

.health-auto-start {
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 16px;
}
</style>
