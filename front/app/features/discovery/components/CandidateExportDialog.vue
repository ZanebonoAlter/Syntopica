<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useDiscoveryApi } from '~/api/discovery'
import type { CatalogExportResult } from '~/types/discovery'

/**
 * 候选目录导出弹窗（improve-discovery-recommendations 5.3，spec C4 默认安全导出）。
 *
 * - 打开即调 export（默认安全导出）：预览将导出条数与三类排除计数（私有 / 含 query / 敏感复核），
 *   并说明文件仅目录配置——不含订阅状态、阅读记录或兴趣画像。
 * - 「包含私有来源」开关：v1 后端不支持（include_private=true 恒 400），前端预拦截——
 *   勾选时给出明确提示并禁用导出，绝不发必败请求。
 * - 确认导出：用预览响应中的文件本体组 Blob 触发真实下载
 *   （URL.createObjectURL + a[download]，文件名 syntopica-candidates-<日期>.json）。
 */
const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const api = useDiscoveryApi()

const loading = ref(false)
const error = ref<string | null>(null)
const result = ref<CatalogExportResult | null>(null)
const includePrivate = ref(false)
const downloaded = ref(false)

const visible = computed(() => props.modelValue)

watch(visible, (v) => {
  if (v) {
    includePrivate.value = false
    downloaded.value = false
    void load()
  }
})

async function load() {
  loading.value = true
  error.value = null
  result.value = null
  const res = await api.exportCandidates()
  loading.value = false
  if (res.success && res.data) {
    result.value = res.data
  } else {
    error.value = res.error || '导出预览失败'
  }
}

/** 排除计数展示行（0 也展示，明确「没有排除」而非隐藏） */
const exclusionRows = computed(() => {
  if (!result.value) return []
  return [
    { key: 'private', count: result.value.excluded_private, label: '私有来源', reason: '非公开来源不写入导出文件' },
    { key: 'query', count: result.value.excluded_query, label: '地址含查询串', reason: '可能携带 token，保守排除' },
    { key: 'sensitive', count: result.value.excluded_sensitive, label: '敏感地址', reason: '含凭据或内网地址，保守排除' },
  ]
})

const canDownload = computed(() => !!result.value && !includePrivate.value && !loading.value)

/** 勾选私有导出：前端预拦截（后端 v1 恒 400），不发必败请求 */
const privateNotice = computed(() =>
  includePrivate.value
    ? '第一版暂不支持连私有来源一起导出：私有地址与凭据永不写入导出文件。请关闭此开关后再导出。'
    : null,
)

function dateStamp(): string {
  return new Date().toISOString().slice(0, 10)
}

function download() {
  if (!canDownload.value || !result.value) return
  const blob = new Blob([JSON.stringify(result.value.export, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `syntopica-candidates-${dateStamp()}.json`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
  downloaded.value = true
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="visible"
    title="导出候选目录"
    size="md"
    :close-on-overlay="false"
    @update:model-value="close"
  >
    <div class="ced-export">
      <!-- 加载态 -->
      <div v-if="loading" class="ced-export__state" data-testid="export-loading">
        <Icon icon="mdi:loading" width="28" height="28" class="ced-export__spin" />
        <p class="ced-export__state-text">正在生成导出预览…</p>
      </div>

      <!-- 错误态：可重试 -->
      <div v-else-if="error" class="ced-export__state" data-testid="export-error">
        <Icon icon="mdi:alert-circle-outline" width="28" height="28" style="color: var(--color-error)" />
        <p class="ced-export__state-text">{{ error }}</p>
        <AppButton size="sm" variant="secondary" data-testid="export-retry" @click="load">重试</AppButton>
      </div>

      <template v-else-if="result">
        <!-- 预览范围 -->
        <div class="ced-export__summary" data-testid="export-summary">
          <p class="ced-export__lead">
            将导出 <strong>{{ result.export.entries.length }}</strong> 条候选配置。
          </p>
          <ul class="ced-export__exclusions">
            <li v-for="row in exclusionRows" :key="row.key" class="ced-export__exclusion" :data-testid="`export-excluded-${row.key}`">
              <span class="ced-export__exclusion-label">{{ row.label }}：{{ row.count }} 条</span>
              <span class="ced-export__exclusion-reason">{{ row.reason }}</span>
            </li>
          </ul>
          <p class="ced-export__note" data-testid="export-scope-note">
            导出文件仅包含目录配置（名称、地址、人工说明与推荐开关），不含订阅状态、阅读记录或兴趣画像。
          </p>
        </div>

        <!-- 私有开关：显示但 v1 不支持，勾选即预拦截 -->
        <div class="ced-export__toggle">
          <AppToggle
            v-model="includePrivate"
            label="包含私有来源"
            data-testid="export-include-private"
          />
          <p v-if="privateNotice" class="ced-export__private-notice" role="alert" data-testid="export-private-notice">
            {{ privateNotice }}
          </p>
        </div>

        <p v-if="downloaded" class="ced-export__done" data-testid="export-downloaded">
          已开始下载 syntopica-candidates-{{ dateStamp() }}.json。
        </p>
      </template>
    </div>

    <template #footer>
      <AppButton variant="ghost" size="sm" data-testid="export-cancel" @click="close">取消</AppButton>
      <AppButton
        variant="primary"
        size="sm"
        :disabled="!canDownload"
        data-testid="export-confirm"
        @click="download"
      >
        导出并下载
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.ced-export {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.ced-export__state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px 16px;
}

.ced-export__state-text {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
  text-align: center;
}

.ced-export__spin {
  color: var(--color-link, var(--color-accent));
  animation: ced-export-spin 1s linear infinite;
}

@keyframes ced-export-spin {
  to { transform: rotate(360deg); }
}

.ced-export__summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-sunken);
}

.ced-export__lead {
  margin: 0;
  font-size: 14px;
  color: var(--color-text-primary);
}

.ced-export__exclusions {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ced-export__exclusion {
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-size: 12px;
}

.ced-export__exclusion-label {
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.ced-export__exclusion-reason {
  color: var(--color-text-muted);
}

.ced-export__note {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.ced-export__toggle {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ced-export__private-notice {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--color-warning-subtle, var(--color-border-medium));
  background: var(--color-warning-subtle, var(--color-bg-sunken));
  color: var(--color-warning, var(--color-text-secondary));
  font-size: 12px;
  line-height: 1.6;
}

.ced-export__done {
  margin: 0;
  font-size: 12px;
  color: var(--color-success);
}
</style>
