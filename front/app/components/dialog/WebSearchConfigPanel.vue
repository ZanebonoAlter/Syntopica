<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import { useWebSearchApi } from '~/api'
import type { WebSearchHealth, WebSearchSettings } from '~/api/webSearch'

// 网页搜索配置（switch-margin-note-search-to-deepseek design D6 / settings-workspace
// spec「数据源连通性探测」）：页边注问答的联网后端换成 DeepSeek Search API，密钥
// 复用「AI 模型」分区（section=ai-providers）里已配置的 DeepSeek 供应商——本面板
// **没有可编辑字段**（同一把 key 不允许两处可改），只展示关联状态 + 按需探测连通性。
// 探测不阻塞任何操作，搜索失败时问答侧静默降级（那层语义不在这里）。

const loading = ref(false)
const settings = ref<WebSearchSettings | null>(null)
const loadError = ref<string | null>(null)

// 连通性探测态：idle（无状态行）/ probing / 四态结果。
const probing = ref(false)
const health = ref<WebSearchHealth | null>(null)
const probeError = ref<string | null>(null)

/** unreachable 的针对性主文案（spec：按 error_kind 分流，别只丢一句「失败」）。 */
function unreachableText(errorKind: WebSearchHealth['error_kind']): string {
  switch (errorKind) {
    case 'auth':
      return '鉴权失败（检查 DeepSeek 供应商 API Key）'
    case 'quota':
      return '余额或额度不足'
    case 'timeout':
      return '请求超时'
    case 'rate_limited':
      return '请求过于频繁，稍后再试'
    default:
      return '搜索服务暂不可用'
  }
}

/** auth / quota 额外给一行去向提示（ui-design D3：纯文字指引，不新造跨分区跳转控件）。 */
function vendorHintText(errorKind: WebSearchHealth['error_kind']): string | null {
  switch (errorKind) {
    case 'auth':
      return '请到「AI 模型」分区检查 DeepSeek 供应商的 API Key'
    case 'quota':
      return '请到「AI 模型」分区检查 DeepSeek 账户余额或套餐额度'
    default:
      return null
  }
}

/** 四态展示（沿用数据源面板的同一视觉语言：色点 + 一句话结论 + 次级说明）。 */
const healthView = computed(() => {
  const result = health.value
  if (!result) return null
  switch (result.status) {
    case 'ok':
      return { dot: 'ok', text: `连接正常 · ${result.latency_ms}ms`, detail: result.detail, hint: null }
    case 'degraded':
      return { dot: 'degraded', text: '服务可达但搜索无结果', detail: result.detail, hint: null }
    case 'unreachable':
      return {
        dot: 'unreachable',
        text: unreachableText(result.error_kind),
        detail: result.detail,
        hint: vendorHintText(result.error_kind),
      }
    case 'disabled':
      return { dot: 'disabled', text: '未关联供应商，联网参考已关闭', detail: result.detail, hint: null }
    default:
      return null // 集外值按无标记处理（API 层已归一到四态）
  }
})

async function loadSettings() {
  loading.value = true
  loadError.value = null
  try {
    const { getSettings } = useWebSearchApi()
    const response = await getSettings()
    if (response.success && response.data) {
      settings.value = response.data
    } else {
      settings.value = null
      loadError.value = response.error || '读取网页搜索配置失败'
    }
  } catch {
    settings.value = null
    loadError.value = '读取网页搜索配置失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadSettings)

/** 探测一次连通性；loading 防重复提交，探测异常只落状态行不弹阻断错误。 */
async function probe() {
  if (probing.value || loading.value) return
  probing.value = true
  probeError.value = null
  try {
    const { getHealth } = useWebSearchApi()
    const response = await getHealth()
    if (response.success && response.data) {
      health.value = response.data
    } else {
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
</script>

<template>
  <div class="space-y-6">
    <div>
      <h3 class="font-semibold" style="color: var(--color-text-primary)">网页搜索</h3>
      <p class="text-sm mt-0.5" style="color: var(--color-text-secondary)">
        页边注问答的联网参考由 DeepSeek Search API 提供，复用「AI 模型」分区中已配置的 DeepSeek 供应商密钥。
      </p>
    </div>

    <div
      v-if="loadError"
      class="rounded-lg px-4 py-2.5 text-sm flex items-center gap-2"
      style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)"
      data-testid="web-search-settings-error"
    >
      <span>{{ loadError }}</span>
      <AppButton variant="ghost" size="sm" class="ml-auto" data-testid="web-search-settings-retry" @click="loadSettings">
        重试
      </AppButton>
    </div>

    <p v-else-if="loading" class="text-sm" style="color: var(--color-text-muted)" data-testid="web-search-settings-loading">
      正在读取关联状态...
    </p>

    <p v-else-if="settings?.vendor_configured" class="text-sm" style="color: var(--color-text-secondary)" data-testid="web-search-vendor">
      已关联供应商：{{ settings.vendor_name || '（未命名）' }}
    </p>

    <p v-else-if="settings" class="text-sm" style="color: var(--color-text-muted)" data-testid="web-search-vendor-missing">
      网页搜索使用已配置的 DeepSeek 供应商密钥，请先在「AI 模型」分区添加 DeepSeek 供应商
    </p>

    <!-- 连通性探测区：按钮 + 四态标记，手动触发不轮询（settings-workspace spec）。 -->
    <div class="flex items-start gap-3" data-testid="web-search-health">
      <AppButton
        variant="secondary"
        size="sm"
        :loading="probing"
        :disabled="loading"
        data-testid="web-search-health-probe"
        @click="probe"
      >
        {{ probing ? '测试中...' : '测试连接' }}
      </AppButton>
      <div v-if="healthView" class="flex flex-col gap-1 text-sm" data-testid="web-search-health-status">
        <span class="flex items-center gap-2">
          <i class="web-search-health-dot" :class="`web-search-health-dot--${healthView.dot}`" />
          <span style="color: var(--color-text-secondary)">{{ healthView.text }}</span>
        </span>
        <span v-if="healthView.detail" class="text-xs" style="color: var(--color-text-muted)" data-testid="web-search-health-detail">
          {{ healthView.detail }}
        </span>
        <span v-if="healthView.hint" class="text-xs" style="color: var(--color-text-muted)" data-testid="web-search-health-hint">
          {{ healthView.hint }}
        </span>
      </div>
      <span v-else-if="probeError" class="text-sm" style="color: var(--color-text-muted)" data-testid="web-search-health-error">
        {{ probeError }}
      </span>
    </div>
  </div>
</template>

<style scoped>
/* 四态状态点：中性色系（绿/琥珀/红/灰），与设置面板既有状态呈现同族。 */
.web-search-health-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.web-search-health-dot--ok {
  background: var(--color-success, #3d8a4a);
}

.web-search-health-dot--degraded {
  background: var(--color-warning, #c98a2e);
}

.web-search-health-dot--unreachable {
  background: var(--color-error, #c42f3c);
}

.web-search-health-dot--disabled {
  background: var(--color-text-muted, #999);
}
</style>
