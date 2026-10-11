<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, onMounted, ref } from 'vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useSearxngApi } from '~/api'
import type { SearxngHealth } from '~/api/searxng'

// SearXNG 本地搜索配置（daily-report-margin-notes design D7）：
// 页边注问答联网后端，每次提问现读配置——界面改即时生效，无需重启。
// 无 API key（本地实例），失败静默降级为纯文章+模型知识。
// add-searxng-health-indicator：新增连通性主动探测（真实探针搜索四态），
// 点击「测试连接」即时探测；保存成功后自动探测一次；探测异常不阻塞保存。

const enabled = ref(false)
const endpoint = ref('')
const loading = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)

// 连通性探测态：idle（未探测，无状态行）/ probing / 四态结果。
const probing = ref(false)
const health = ref<SearxngHealth | null>(null)
const probeError = ref<string | null>(null)

/** 四态展示文案（ui-design 连通性区状态表）。 */
const healthView = computed(() => {
  if (!health.value) return null
  switch (health.value.status) {
    case 'ok':
      return { dot: 'ok', text: `连接正常 · ${health.value.latency_ms}ms` }
    case 'degraded':
      return { dot: 'degraded', text: '服务可达但搜索无结果（引擎可能故障）' }
    case 'unreachable':
      return { dot: 'unreachable', text: '无法连接（endpoint 不可达或超时）' }
    case 'disabled':
      return { dot: 'disabled', text: '未启用/未配置' }
    default:
      return null // 集外值按无标记处理（变体走查：status 仅枚举值）
  }
})

onMounted(async () => {
  loading.value = true
  error.value = null
  try {
    const { getConfig } = useSearxngApi()
    const response = await getConfig()
    if (response.success && response.data) {
      enabled.value = response.data.enabled
      endpoint.value = response.data.endpoint
    }
  } catch {
    error.value = '加载 SearXNG 设置失败'
  } finally {
    loading.value = false
  }
})

/** 探测一次连通性（按钮点击 / 保存成功后自动触发共用）；loading 防重复提交。 */
async function probe() {
  if (probing.value) return
  probing.value = true
  probeError.value = null
  try {
    const { checkHealth } = useSearxngApi()
    const response = await checkHealth()
    if (response.success && response.data) {
      health.value = response.data
    } else {
      // 探测请求本身失败（后端不可达等）——不阻塞保存反馈，只落状态行。
      health.value = null
      probeError.value = response.error || '探测请求失败'
    }
  } catch {
    health.value = null
    probeError.value = '探测请求失败'
  } finally {
    probing.value = false
  }
}

async function save() {
  loading.value = true
  error.value = null
  success.value = null
  try {
    const { saveSettings } = useSearxngApi()
    const response = await saveSettings({ endpoint: endpoint.value.trim(), enabled: enabled.value })
    if (!response.success) {
      throw new Error(response.error || '保存失败')
    }
    success.value = 'SearXNG 设置已保存，下次提问即时生效'
    setTimeout(() => { success.value = null }, 2000)
    // 保存成功后自动探测一次（探测异常不影响保存反馈——probe 内部自兜底）。
    await probe()
  } catch {
    error.value = '保存 SearXNG 设置失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="space-y-6">
    <div v-if="loading && !endpoint && !enabled" class="flex items-center justify-center py-12">
      <Icon icon="mdi:loading" width="48" height="48" class="animate-spin" style="color: var(--color-link)" />
    </div>
    <template v-else>
      <div v-if="success" class="p-3 rounded-lg text-sm" style="background: var(--color-success-bg, rgba(61, 138, 74, 0.1)); border: 1px solid var(--color-success-border, rgba(61, 138, 74, 0.25)); color: var(--color-success)">
        {{ success }}
      </div>
      <div v-if="error" class="p-3 rounded-lg text-sm" style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)">
        {{ error }}
      </div>

      <div class="flex items-center justify-between">
        <div>
          <h3 class="font-semibold" style="color: var(--color-text-primary)">SearXNG 本地搜索</h3>
          <p class="text-sm mt-0.5" style="color: var(--color-text-secondary)">
            日报页边注问答的联网补强后端；搜索失败或未配置时静默降级，不影响问答
          </p>
        </div>
        <AppToggle v-model="enabled" />
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--color-text-secondary)">实例地址</label>
          <input
            :value="endpoint"
            type="text"
            class="w-full px-3 py-2 rounded-lg text-sm input"
            placeholder="http://localhost:8889"
            @input="endpoint = ($event.target as HTMLInputElement).value"
          />
          <p class="text-xs mt-1" style="color: var(--color-text-muted)">
            本地 SearXNG 实例的地址（JSON API 需启用），无需 API Key
          </p>
        </div>
      </div>

      <!-- 连通性探测区（add-searxng-health-indicator）：按钮 + 四态标记，手动触发不轮询。 -->
      <div class="flex items-center gap-3" data-testid="searxng-health">
        <button
          type="button"
          class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          style="border: 1px solid var(--color-border); color: var(--color-text-secondary)"
          :disabled="probing"
          data-testid="searxng-health-probe"
          @click="probe"
        >
          <span v-if="probing" class="flex items-center gap-2">
            <Icon icon="mdi:loading" width="14" class="animate-spin" />
            测试中...
          </span>
          <span v-else>测试连接</span>
        </button>
        <span
          v-if="healthView"
          class="flex items-center gap-2 text-sm"
          data-testid="searxng-health-status"
        >
          <i class="searxng-health-dot" :class="`searxng-health-dot--${healthView.dot}`" />
          <span style="color: var(--color-text-secondary)">{{ healthView.text }}</span>
        </span>
        <span v-else-if="probeError" class="text-sm" style="color: var(--color-text-muted)" data-testid="searxng-health-error">{{ probeError }}</span>
      </div>

      <div class="flex justify-end">
        <button
          type="button"
          class="px-4 py-2 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          style="background: var(--color-accent)"
          :disabled="loading"
          @click="save"
        >
          <span v-if="loading" class="flex items-center gap-2">
            <Icon icon="mdi:loading" width="16" class="animate-spin" />
            保存中...
          </span>
          <span v-else>保存设置</span>
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* 四态状态点：中性色系（绿/琥珀/红/灰），与设置面板既有状态呈现同族，
   不新造视觉语言。 */
.searxng-health-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.searxng-health-dot--ok {
  background: var(--color-success, #3d8a4a);
}

.searxng-health-dot--degraded {
  background: #c98a2e;
}

.searxng-health-dot--unreachable {
  background: var(--color-error, #c42f3c);
}

.searxng-health-dot--disabled {
  background: var(--color-text-muted, #999);
}
</style>
