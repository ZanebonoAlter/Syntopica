<script setup lang="ts">
/**
 * 供应商新增/编辑两段式弹窗（ai-provider-model-overhaul 4.2 交付 3）。
 * 第一段：预设目录卡片网格（presets.ts 七项）；第二段：接入配置——预设只露
 * API Key（base_url 可改，llama.cpp 另有启动命令），自定义走全量表单；高级折叠
 * 默认收起（超时/思考方言/结构化输出）。测试连接为未保存表单的内联三态。
 * 保存动作由父层执行（createVendor/updateVendor + 自动拉清单 + toast），
 * 本组件 emit('save', AIVendorUpsertRequest)。
 */
import { Icon } from '@iconify/vue'
import { computed, reactive, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import VendorLogo from './VendorLogo.vue'
import { useAIVendors } from '../composables/useAIVendors'
import { AI_VENDOR_PRESETS, PRESET_GROUPS, findVendorPreset, type AIVendorPreset } from '../presets'
import type { AIVendor, AIVendorUpsertRequest, ProviderType, StructuredOutputMode, ThinkingDialect } from '~/types'

interface Props {
  modelValue: boolean
  /** 非空 = 编辑模式（预填；api_key/start_command 原文不回显，只回 configured 布尔） */
  vendor?: AIVendor | null
  /** 父层保存中（保存按钮 loading 锁） */
  saving?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  vendor: null,
  saving: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [payload: AIVendorUpsertRequest]
}>()

// 内联测试是无缓存动作，leaf 自建实例安全
const vendorsCtx = useAIVendors()

const DIALECTS: ThinkingDialect[] = ['qwen-chat-template', 'deepseek', 'reasoning-effort', 'none']
const SO_OPTIONS: Array<{ value: StructuredOutputMode, label: string }> = [
  { value: 'json_schema', label: 'json_schema（严格）' },
  { value: 'json_object', label: 'json_object（宽松）' },
  { value: 'none', label: 'none（不用）' },
]

const isEdit = computed(() => props.vendor !== null)
const selectedPreset = computed(() => findVendorPreset(form.presetId))
const isCustom = computed(() => form.presetId === 'custom')
const isLlamacpp = computed(() => form.presetId === 'llamacpp')

// '' = 跟随预设（仅非自定义预设出现该选项）；自定义直接取具体档位
const form = reactive({
  presetId: '',
  name: '',
  providerType: 'openai_compatible' as ProviderType,
  baseUrl: '',
  apiKey: '',
  clearApiKey: false,
  startCommand: '',
  clearStartCommand: false,
  timeoutSeconds: 600 as number | '',
  dialect: '' as ThinkingDialect | '',
  structuredOutput: '' as StructuredOutputMode | '',
})

const errors = reactive({ name: '', baseUrl: '' })

// ---- 测试连接三态（未保存表单内联测，testVendorInline 不走库） ----
// 声明需在 watch(immediate) 之前：resetForm → resetTestState
const testState = ref<'idle' | 'testing' | 'ok' | 'fail'>('idle')
const testMessage = ref('')

function resetTestState() {
  testState.value = 'idle'
  testMessage.value = ''
}

function selectPreset(p: AIVendorPreset) {
  form.presetId = p.id
  form.name = p.id === 'custom' ? '' : p.label
  form.providerType = p.providerType
  form.baseUrl = p.baseUrl
  form.apiKey = ''
  form.clearApiKey = false
  form.startCommand = ''
  form.clearStartCommand = false
  form.dialect = p.id === 'custom' ? p.thinkingDialect : ''
  form.structuredOutput = p.id === 'custom' ? p.structuredOutput : ''
  errors.name = ''
  errors.baseUrl = ''
  resetTestState()
}

/** 打开时按模式重建表单（编辑=vendor 预填，新增=空态等待选预设） */
function resetForm() {
  form.presetId = ''
  form.name = ''
  form.providerType = 'openai_compatible'
  form.baseUrl = ''
  form.apiKey = ''
  form.clearApiKey = false
  form.startCommand = ''
  form.clearStartCommand = false
  form.timeoutSeconds = 600
  form.dialect = ''
  form.structuredOutput = ''
  errors.name = ''
  errors.baseUrl = ''
  resetTestState()

  const v = props.vendor
  if (!v) return
  const p = findVendorPreset(v.preset_id)
  form.presetId = p?.id ?? 'custom'
  form.name = v.name
  form.providerType = v.provider_type
  form.baseUrl = v.base_url
  form.timeoutSeconds = v.timeout_seconds || 600
  // 档位与预设一致 → 显示「跟随预设」；覆盖过 → 预填具体档位（编辑可改，spec「档位可改」）
  form.dialect = v.thinking_dialect === p?.thinkingDialect ? '' : v.thinking_dialect
  form.structuredOutput = v.structured_output === p?.structuredOutput ? '' : v.structured_output
}

// immediate：以 modelValue=true 直接挂载（含测试）也完成一次表单重建
watch(() => props.modelValue, (open) => {
  if (open) resetForm()
}, { immediate: true })

function close() {
  if (!props.saving) emit('update:modelValue', false)
}

function validate(): boolean {
  errors.name = form.name.trim() ? '' : '请填写名称'
  errors.baseUrl = /^https?:\/\/.+/i.test(form.baseUrl.trim()) ? '' : '请填写合法 URL'
  return !errors.name && !errors.baseUrl
}

async function onTest() {
  if (testState.value === 'testing' || !validate()) return
  testState.value = 'testing'
  testMessage.value = ''
  const message = await vendorsCtx.testInline({
    base_url: form.baseUrl.trim(),
    api_key: form.apiKey.trim() || undefined,
    provider_type: form.providerType,
    timeout_seconds: typeof form.timeoutSeconds === 'number' ? form.timeoutSeconds : undefined,
  })
  if (message === null) {
    testState.value = 'fail'
    testMessage.value = vendorsCtx.error.value || '连接测试失败'
  } else {
    testState.value = 'ok'
    testMessage.value = message
  }
}

// ---- 保存（载荷组装；CRUD 与后续展开/toast 由父层执行） ----
function onSave() {
  const p = selectedPreset.value
  if (!p || props.saving || !validate()) return

  const payload: AIVendorUpsertRequest = {
    name: form.name.trim(),
    provider_type: form.providerType,
    base_url: form.baseUrl.trim(),
    timeout_seconds: typeof form.timeoutSeconds === 'number' && form.timeoutSeconds > 0 ? form.timeoutSeconds : 600,
    thinking_dialect: form.dialect || p.thinkingDialect,
    structured_output: form.structuredOutput || p.structuredOutput,
  }
  if (p.id !== 'custom') payload.preset_id = p.id
  // key：输入新值才覆盖；clear_api_key=true 时后端置空（优先级高于 api_key 字段）
  if (form.apiKey.trim()) payload.api_key = form.apiKey.trim()
  if (form.clearApiKey) payload.clear_api_key = true
  // start_command：非空=覆盖；clear 开关=显式清除（后端清除标记优先）
  if (form.startCommand.trim()) payload.start_command = form.startCommand.trim()
  if (form.clearStartCommand) payload.clear_start_command = true

  emit('save', payload)
}

const step2Label = computed(() => {
  const p = selectedPreset.value
  if (!p) return '接入配置'
  if (p.id === 'custom') return '自定义接入（OpenAI 兼容）'
  return `接入 ${p.label}${p.needsApiKey ? '（只需 API Key）' : ''}`
})

/* 预设目录搜索（FB-1：44 项分组 + 搜索，ui-design Interaction Contract） */
const presetQuery = ref('')
const groupedPresets = computed(() => {
  const q = presetQuery.value.trim().toLowerCase()
  return PRESET_GROUPS
    .map(g => ({
      id: g.id,
      label: g.label,
      items: AI_VENDOR_PRESETS.filter(p => p.group === g.id
        && (!q || p.label.toLowerCase().includes(q) || p.id.toLowerCase().includes(q))),
    }))
    .filter(g => g.items.length > 0)
})
</script>

<template>
  <AppDialog :model-value="modelValue" size="lg"
    :title="isEdit && vendor ? `编辑供应商：${vendor.name}` : '新增供应商'"
    :close-on-overlay="!saving" @update:model-value="(v: boolean) => { if (!v) close() }">
    <div class="vendor-dialog">
      <!-- 第一段：预设目录（FB-1：44 项五组 + 搜索） -->
      <p class="step-label"><span class="step-n">1</span>选择供应商</p>
      <AppInput v-model="presetQuery" type="text" placeholder="搜索预设（44 项：名称 / id）…" class="preset-search" />
      <div class="preset-scroll">
        <div v-for="g in groupedPresets" :key="g.id" class="preset-group">
          <p class="preset-group__title">{{ g.label }} · {{ g.items.length }}</p>
          <div class="preset-grid" role="listbox" :aria-label="`供应商预设 ${g.label}`">
            <button v-for="p in g.items" :key="p.id" type="button" class="preset-card"
              :class="{ 'preset-card--selected': form.presetId === p.id }"
              :title="p.note" @click="selectPreset(p)">
              <span class="preset-card__name">
                <VendorLogo :symbol-id="p.logo" :name="p.label" :size="14" />
                <span class="preset-card__label">{{ p.label }}</span>
              </span>
              <span class="preset-card__kind">{{ p.isLocal ? '本地' : (p.id === 'custom' ? '全量表单' : '云端') }}</span>
            </button>
          </div>
        </div>
        <p v-if="!groupedPresets.length" class="preset-empty">无匹配预设，试试「自定义」（OpenAI 兼容）</p>
      </div>

      <!-- 第二段：接入配置 -->
      <p class="step-label"><span class="step-n">2</span>{{ step2Label }}</p>

      <template v-if="selectedPreset">
        <div v-if="!isCustom" class="preset-summary">
          预设：<b>{{ selectedPreset.baseUrl }}</b> · {{ selectedPreset.thinkingDialect }} ·
          {{ selectedPreset.structuredOutput }}（高级可覆盖）
        </div>

        <div class="form-grid">
          <div class="field">
            <label>名称 *</label>
            <AppInput v-model="form.name" type="text" :error="errors.name" placeholder="供应商名称" @update:model-value="errors.name = ''" />
          </div>
          <div v-if="isCustom" class="field">
            <label>类型</label>
            <select v-model="form.providerType" class="ai-select">
              <option value="openai_compatible">OpenAI Compatible</option>
              <option value="ollama">Ollama (本地)</option>
            </select>
          </div>
          <div class="field field--full">
            <label>Base URL *</label>
            <AppInput v-model="form.baseUrl" type="text" :error="errors.baseUrl" placeholder="https://api.example.com/v1" @update:model-value="errors.baseUrl = ''" />
            <span v-if="!isCustom" class="help">可改（服务可能在其他主机）</span>
          </div>

          <!-- 预设：只露 API Key（needsApiKey）；无 key 预设提示本地服务 -->
          <div v-if="!isCustom && selectedPreset.needsApiKey" class="field field--full">
            <label>API Key{{ isEdit && vendor?.api_key_configured ? '（已配置）' : '' }}</label>
            <AppInput v-model="form.apiKey" type="password"
              :placeholder="isEdit && vendor?.api_key_configured ? '已配置（输入新值覆盖，留空保持不变）' : 'sk-…'" />
            <label v-if="isEdit && vendor?.api_key_configured" class="clear-line">
              <input v-model="form.clearApiKey" type="checkbox" class="clear-check" />
              清除已保存密钥
            </label>
          </div>
          <div v-else-if="!isCustom" class="field--full no-key-hint">
            <Icon icon="mdi:information-outline" width="14" height="14" /> 本地服务无需密钥
          </div>

          <div v-if="isCustom" class="field field--full">
            <label>API Key（可选）</label>
            <AppInput v-model="form.apiKey" type="password"
              :placeholder="isEdit && vendor?.api_key_configured ? '已配置（输入新值覆盖，留空保持不变）' : '无认证服务可留空'" />
            <label v-if="isEdit && vendor?.api_key_configured" class="clear-line">
              <input v-model="form.clearApiKey" type="checkbox" class="clear-check" />
              清除已保存密钥
            </label>
          </div>

          <!-- llama.cpp 预设与自定义表单露出启动命令 -->
          <div v-if="isLlamacpp || isCustom" class="field field--full">
            <label>启动命令{{ isLlamacpp ? '（llama.cpp，可选）' : '（可选）' }}</label>
            <AppInput v-model="form.startCommand" type="text"
              :placeholder="isEdit && vendor?.start_command_configured ? '已配置（输入新命令覆盖，留空保持不变）' : 'llama-server -m model.gguf --port 8080'" />
            <span class="help">由后端执行的本地启动命令；跨主机部署请留空</span>
            <label v-if="isEdit && vendor?.start_command_configured" class="clear-line">
              <input v-model="form.clearStartCommand" type="checkbox" class="clear-check" />
              清除已配置的启动命令
            </label>
          </div>
        </div>

        <!-- 高级折叠（默认收起） -->
        <details class="advanced">
          <summary>高级（超时 / 思考方言 / 结构化输出）</summary>
          <div class="form-grid advanced-grid">
            <div class="field">
              <label>超时（秒）</label>
              <AppInput v-model="form.timeoutSeconds" type="number" min="1" step="1" />
            </div>
            <div class="field">
              <label>思考方言{{ isCustom ? '' : '覆盖' }}</label>
              <select v-model="form.dialect" class="ai-select">
                <option v-if="!isCustom" value="">跟随预设（{{ selectedPreset.thinkingDialect }}）</option>
                <option v-for="d in DIALECTS" :key="d" :value="d">{{ d }}</option>
              </select>
            </div>
            <div class="field">
              <label>结构化输出{{ isCustom ? '' : '覆盖' }}</label>
              <select v-model="form.structuredOutput" class="ai-select">
                <option v-if="!isCustom" value="">跟随预设（{{ selectedPreset.structuredOutput }}）</option>
                <option v-for="o in SO_OPTIONS" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </div>
          </div>
        </details>
      </template>
      <p v-else class="preset-empty-hint">↑ 先在上方选择供应商</p>
    </div>

    <template #footer>
      <button class="test-btn" type="button" :disabled="testState === 'testing' || saving" @click="onTest">
        <Icon v-if="testState === 'testing'" icon="mdi:loading" width="12" height="12" class="animate-spin" />
        {{ testState === 'testing' ? '测试中…' : '测试连接' }}
      </button>
      <span v-if="testState === 'ok'" class="conn-result conn-result--ok">✓ {{ testMessage }}</span>
      <span v-else-if="testState === 'fail'" class="conn-result conn-result--fail">✕ {{ testMessage }}</span>
      <div class="footer-spacer" />
      <AppButton variant="secondary" :disabled="saving" @click="close">取消</AppButton>
      <AppButton :loading="saving" @click="onSave">{{ isEdit ? '保存' : '保存并拉取模型' }}</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.vendor-dialog {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.step-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin: 0;
}

.step-n {
  display: inline-block;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  background: var(--color-accent);
  color: var(--color-text-inverted);
  font-size: 11px;
  text-align: center;
  line-height: 18px;
}

.preset-search {
  margin-bottom: 10px;
}

.preset-scroll {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  padding: 10px;
}

.preset-group {
  margin-bottom: 12px;
}
.preset-group:last-child {
  margin-bottom: 0;
}

.preset-group__title {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.4px;
  color: var(--color-text-muted);
  margin: 2px 0 6px;
}

.preset-empty {
  font-size: 12px;
  color: var(--color-text-muted);
  padding: 4px 2px;
  margin: 0;
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
}

.preset-card {
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  padding: 9px 12px;
  cursor: pointer;
  font-size: 12.5px;
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  transition: border-color 0.15s, background 0.15s;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.preset-card:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
.preset-card--selected {
  border-color: var(--color-accent);
  box-shadow: inset 0 0 0 1px var(--color-accent);
  background: var(--color-accent-subtle);
}

.preset-card__name {
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  white-space: nowrap;
}

.preset-card__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.preset-card__kind {
  color: var(--color-text-muted);
  font-size: 11px;
  padding-left: 20px;
}

.preset-summary {
  background: var(--color-bg-sunken);
  border-radius: 6px;
  padding: 7px 12px;
  font-size: 12px;
  color: var(--color-text-secondary);
}
.preset-summary b {
  color: var(--color-text-primary);
  word-break: break-all;
}

.preset-empty-hint {
  font-size: 12px;
  color: var(--color-text-muted);
  margin: 4px 0 0;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 18px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.field--full {
  grid-column: 1 / -1;
}
.field label {
  font-size: 12px;
  color: var(--color-text-secondary);
}

.help {
  font-size: 11px;
  color: var(--color-text-muted);
}

.clear-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.clear-check {
  accent-color: var(--color-accent);
}

.no-key-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-radius: 6px;
  padding: 7px 12px;
}

.advanced {
  margin-top: 4px;
  font-size: 12px;
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 10px;
}
.advanced summary {
  cursor: pointer;
  color: var(--color-text-muted);
}
.advanced-grid {
  margin-top: 10px;
}

.ai-select {
  width: 100%;
  padding: 7px 10px;
  font-size: 13px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s;
}
.ai-select:focus {
  border-color: var(--color-input-focus);
}

.test-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12.5px;
  padding: 5px 14px;
  border-radius: 8px;
  border: 1px solid var(--color-border-strong);
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.test-btn:hover {
  border-color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
.test-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.conn-result {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 6px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.conn-result--ok {
  background: var(--color-success-subtle);
  color: var(--color-success);
}
.conn-result--fail {
  background: var(--color-error-subtle);
  color: var(--color-error);
}

.footer-spacer {
  flex: 1;
}

@media (max-width: 767.98px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
