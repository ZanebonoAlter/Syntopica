<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import {
  useAnalysisMethodsApi,
  type AnalysisMethod,
  type AnalysisMethodSelectionMeta,
} from '~/api/analysisMethods'
import { useNotify } from '~/composables/useNotify'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import AppInput from '~/components/ui/AppInput.vue'

/**
 * 分析方法卡管理 —— board-level-deep-analysis 2.6。
 *
 * 方法卡是「过程/核查清单」档案（研究规程），不是作者人格或文风画像：
 * 全局入库但仅在未来调查按问题适配选择 0-2 张后生效，简报/事实阶段不注入。
 * 表单覆盖适用/禁用/证据/失败模式四组元数据（每行一条）；legacy 迁移项默认停用，
 * 需人工整理边界后显式启用，本组件绝不自动启用。
 */

const api = useAnalysisMethodsApi()
const { success: notifySuccess, error: notifyError } = useNotify()

const methods = ref<AnalysisMethod[]>([])
const loading = ref(false)
const loadError = ref<string | null>(null)
const saving = ref(false)
const togglingId = ref<number | null>(null)
const deletingId = ref<number | null>(null)

const formVisible = ref(false)

/** 表单模型：四组元数据用「每行一条」的纯文本编辑，提交时再转数组。 */
interface MethodFormModel {
  id: number
  isNew: boolean
  legacy: boolean
  name: string
  title: string
  summary: string
  content: string
  enabled: boolean
  applicableWhen: string
  avoidWhen: string
  requiredEvidence: string
  failureModes: string
}

const emptyForm = (): MethodFormModel => ({
  id: 0, isNew: true, legacy: false,
  name: '', title: '', summary: '', content: '', enabled: true,
  applicableWhen: '', avoidWhen: '', requiredEvidence: '', failureModes: '',
})

const form = ref<MethodFormModel>(emptyForm())

function linesToText(lines?: string[]): string {
  return (lines ?? []).join('\n')
}

function textToLines(text: string): string[] {
  return text.split('\n').map(s => s.trim()).filter(Boolean)
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const res = await api.listMethods()
    if (res.success && res.data) methods.value = res.data
    else loadError.value = res.error || '加载分析方法失败'
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : '加载分析方法失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openCreate() {
  form.value = emptyForm()
  formVisible.value = true
}

function openEdit(m: AnalysisMethod) {
  form.value = {
    id: m.id, isNew: false, legacy: m.legacy,
    name: m.name, title: m.title || '', summary: m.summary || '', content: m.content,
    enabled: m.enabled,
    applicableWhen: linesToText(m.selection_meta?.applicable_when),
    avoidWhen: linesToText(m.selection_meta?.avoid_when),
    requiredEvidence: linesToText(m.selection_meta?.required_evidence),
    failureModes: linesToText(m.selection_meta?.failure_modes),
  }
  formVisible.value = true
}

function closeForm() {
  formVisible.value = false
}

async function save() {
  const f = form.value
  if (!f) return
  const name = f.name.trim()
  const content = f.content.trim()
  if (!name) { notifyError('短名不能为空'); return }
  if (!content) { notifyError('操作指引不能为空'); return }

  saving.value = true
  try {
    const selectionMeta: AnalysisMethodSelectionMeta = {
      applicable_when: textToLines(f.applicableWhen),
      avoid_when: textToLines(f.avoidWhen),
      required_evidence: textToLines(f.requiredEvidence),
      failure_modes: textToLines(f.failureModes),
    }
    const trimmedTitle = f.title.trim()
    const trimmedSummary = f.summary.trim()
    const body = {
      name,
      // 编辑时 title/summary 显式发送空串：后端 update 按指针语义，缺字段 = 不修改，
      // 转 undefined 会让「清空」静默失效；新建维持留空不传的可选语义。
      title: f.isNew ? (trimmedTitle || undefined) : trimmedTitle,
      summary: f.isNew ? (trimmedSummary || undefined) : trimmedSummary,
      selection_meta: selectionMeta,
      content,
      enabled: f.enabled,
    }
    const res = f.isNew
      ? await api.createMethod(body)
      : await api.updateMethod(f.id, body)
    if (res.success) {
      notifySuccess(f.isNew ? '已创建方法卡' : '已保存')
      formVisible.value = false
      await load()
    } else {
      notifyError(res.error || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(m: AnalysisMethod) {
  // legacy 卡首次启用（停用→启用）前确认：需先人工整理适用条件/证据边界，取消则不调用 API
  if (m.legacy && !m.enabled) {
    const ok = confirm(`启用旧画像迁移的方法卡「${m.title || m.name}」？\n请确认已整理适用条件 / 证据边界（适用/禁用/证据/失败模式）——未整理的旧画像可能在调查选择中失准。`)
    if (!ok) return
  }
  togglingId.value = m.id
  try {
    const res = await api.setEnabled(m.id, !m.enabled)
    if (res.success && res.data) {
      const idx = methods.value.findIndex(x => x.id === m.id)
      if (idx >= 0) methods.value[idx] = res.data
      notifySuccess(res.data.enabled ? '已启用' : '已停用')
    } else {
      notifyError(res.error || '切换失败')
    }
  } finally {
    togglingId.value = null
  }
}

async function removeMethod(m: AnalysisMethod) {
  if (!confirm(`删除分析方法「${m.title || m.name}」？\n软删除后从列表消失，历史调查仍保留方法标题与内容 hash，可追溯。`)) return
  deletingId.value = m.id
  try {
    const res = await api.deleteMethod(m.id)
    if (res.success) {
      notifySuccess('已删除')
      await load()
    } else {
      notifyError(res.error || '删除失败')
    }
  } finally {
    deletingId.value = null
  }
}

const metaFieldDefs: Array<{ key: 'applicableWhen' | 'avoidWhen' | 'requiredEvidence' | 'failureModes'; label: string; hint: string }> = [
  { key: 'applicableWhen', label: '适用条件', hint: '何时适合用这个方法，每行一条' },
  { key: 'avoidWhen', label: '禁用条件', hint: '命中即优先阻止选择，每行一条' },
  { key: 'requiredEvidence', label: '所需证据', hint: '方法生效所需的数据/证据类型，每行一条' },
  { key: 'failureModes', label: '失败模式', hint: '已知会走偏的情况，每行一条' },
]
</script>

<template>
  <div class="am-card">
    <div class="am-card__header">
      <AppSectionHeader
        title="分析方法"
        description="方法卡是研究规程（适用/禁用/证据/失败模式），不是作者人格画像。全局入库但仅在未来调查按问题适配选择 0-2 张后生效，简报与事实阶段不注入。"
        icon-name="mdi:book-open-outline"
      />
      <AppButton variant="primary" size="sm" @click="openCreate">
        <Icon icon="mdi:plus" width="14" height="14" /> 新建方法卡
      </AppButton>
    </div>

    <div class="am-card__body">
      <div v-if="loading && !methods.length" class="am-empty">
        <Icon icon="mdi:loading" width="24" height="24" class="animate-spin" style="color: var(--color-text-muted)" />
        <span>加载中…</span>
      </div>

      <div v-else-if="loadError" class="am-error">
        <Icon icon="mdi:alert-circle" width="14" height="14" /> {{ loadError }}
        <AppButton variant="ghost" size="sm" class="am-error__retry" @click="load">重试</AppButton>
      </div>

      <p v-else-if="!methods.length" class="am-empty">
        还没有分析方法卡。新建一张「研究规程」，让深度调查在适配的问题上有可遵循的核查步骤。
      </p>

      <ul v-else class="am-list">
        <li v-for="m in methods" :key="m.id" class="am-item" :class="{ 'am-item--off': !m.enabled }">
          <div class="am-item__main">
            <div class="am-item__head">
              <span class="am-item__name">{{ m.name }}</span>
              <span v-if="m.title" class="am-item__title">{{ m.title }}</span>
              <span class="am-item__status" :class="m.enabled ? 'on' : 'off'">{{ m.enabled ? '启用中' : '已停用' }}</span>
              <span v-if="m.legacy" class="am-item__legacy-badge">旧画像迁移</span>
            </div>

            <div v-if="m.legacy" class="am-item__legacy">
              <Icon icon="mdi:information-outline" width="14" height="14" />
              旧画像迁移项：需人工整理适用条件 / 证据边界后再启用
            </div>

            <p v-if="m.summary" class="am-item__summary">{{ m.summary }}</p>

            <p class="am-item__meta">
              适用 {{ m.selection_meta?.applicable_when?.length || 0 }}
              · 禁用 {{ m.selection_meta?.avoid_when?.length || 0 }}
              · 证据 {{ m.selection_meta?.required_evidence?.length || 0 }}
              · 失败模式 {{ m.selection_meta?.failure_modes?.length || 0 }}
            </p>
          </div>

          <div class="am-item__actions">
            <AppToggle
              :model-value="m.enabled"
              :label="`启用 ${m.title || m.name}`"
              :disabled="togglingId === m.id"
              @update:model-value="toggleEnabled(m)"
            />
            <AppButton variant="ghost" size="sm" @click="openEdit(m)">
              <Icon icon="mdi:pencil-outline" width="13" height="13" /> 编辑
            </AppButton>
            <AppButton variant="ghost" size="sm" :loading="deletingId === m.id" class="am-item__delete" @click="removeMethod(m)">
              <Icon icon="mdi:delete-outline" width="13" height="13" /> 删除
            </AppButton>
          </div>
        </li>
      </ul>
    </div>

    <!-- 新建 / 编辑对话框 -->
    <AppDialog v-model="formVisible" :title="form.isNew ? '新建方法卡' : `编辑：${form.name}`" width="680px">
      <div class="am-form">
        <div v-if="form.legacy" class="am-form__legacy">
          <Icon icon="mdi:information-outline" width="14" height="14" />
          这是旧参考角色迁移的方法卡，请先补齐适用条件与证据边界，再显式启用；保存不会自动启用。
        </div>

        <div class="am-form__grid">
          <label class="am-field">
            <span class="am-field__label">短名（唯一标识）</span>
            <AppInput v-model="form.name" :disabled="!form.isNew" placeholder="causal-check" />
          </label>
          <label class="am-field">
            <span class="am-field__label">标题（展示用）</span>
            <AppInput v-model="form.title" placeholder="因果链检验" />
          </label>
        </div>

        <label class="am-field">
          <span class="am-field__label">摘要（选择阶段可见，正文只在选中后加载）</span>
          <AppInput v-model="form.summary" placeholder="一句话说明这个方法解决什么问题" />
        </label>

        <label class="am-field">
          <span class="am-field__label">操作指引（分析步骤 / 核查清单，注入调查阶段）</span>
          <textarea v-model="form.content" rows="10" class="am-textarea" placeholder="每行或每段一个步骤，例如「概念考古：先问这个词是谁发明的、为谁服务…」" />
        </label>

        <div class="am-form__meta">
          <label v-for="def in metaFieldDefs" :key="def.key" class="am-field">
            <span class="am-field__label">{{ def.label }} <em class="am-field__hint">{{ def.hint }}</em></span>
            <textarea v-model="form[def.key]" rows="3" class="am-textarea" :placeholder="def.hint" />
          </label>
        </div>

        <label class="am-check">
          <AppToggle v-model="form.enabled" :label="form.enabled ? '启用（保存后参与后续调查选择）' : '停用（保存后不参与选择）'" />
        </label>
      </div>

      <template #footer>
        <AppButton variant="ghost" @click="closeForm">取消</AppButton>
        <AppButton variant="primary" :loading="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</AppButton>
      </template>
    </AppDialog>
  </div>
</template>

<style scoped>
.am-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  overflow: hidden;
}

.am-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  padding: 14px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.am-card__header :deep(.app-section-header) {
  margin-bottom: 0;
}

.am-card__body {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.am-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--color-text-muted);
  font-size: 13px;
  padding: 28px 0;
  border: 1px dashed var(--color-border-subtle);
  border-radius: 10px;
  margin: 0;
}

.am-error {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--color-error);
  background: var(--color-error-bg, rgba(196, 47, 60, 0.1));
  border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25));
  border-radius: 10px;
  padding: 10px 14px;
}

.am-error__retry {
  margin-left: auto;
}

.am-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.am-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-base);
}

.am-item--off {
  opacity: 0.72;
}

.am-item__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.am-item__head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.am-item__name {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  font-size: 13px;
  color: var(--color-text-primary);
}

.am-item__title {
  font-size: 13px;
  color: var(--color-text-secondary);
}

.am-item__status {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 9999px;
}

.am-item__status.on {
  color: var(--color-success);
  background: rgba(61, 138, 74, 0.12);
}

.am-item__status.off {
  color: var(--color-text-muted);
  background: var(--color-bg-hover);
}

.am-item__legacy-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 9999px;
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.am-item__legacy {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  color: var(--color-accent);
  background: var(--color-accent-subtle);
  border-radius: 8px;
  padding: 8px 10px;
  line-height: 1.5;
}

.am-item__legacy :deep(svg),
.am-item__legacy svg {
  flex-shrink: 0;
  margin-top: 1px;
}

.am-item__summary {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
  line-height: 1.5;
}

.am-item__meta {
  margin: 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.am-item__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.am-item__delete:hover :deep(.app-button) {
  color: var(--color-error);
}

/* 表单 */
.am-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.am-form__legacy {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  color: var(--color-accent);
  background: var(--color-accent-subtle);
  border-radius: 8px;
  padding: 8px 10px;
  line-height: 1.5;
}

.am-form__legacy svg {
  flex-shrink: 0;
  margin-top: 1px;
}

.am-form__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.am-form__meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.am-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.am-field__label {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-secondary);
}

.am-field__hint {
  font-style: normal;
  font-weight: 400;
  color: var(--color-text-muted);
  margin-left: 4px;
}

.am-textarea {
  padding: 8px 10px;
  font-size: 13px;
  line-height: 1.6;
  border: 1px solid var(--color-input-border, var(--color-border-subtle));
  border-radius: 8px;
  background: var(--color-input-bg, var(--color-bg-base));
  color: var(--color-text-primary);
  outline: none;
  resize: vertical;
  font-family: ui-monospace, monospace;
}

.am-textarea:focus {
  border-color: var(--color-input-focus, var(--color-accent));
  box-shadow: 0 0 0 2px var(--color-accent-subtle);
}

.am-check {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 768px) {
  .am-form__grid,
  .am-form__meta {
    grid-template-columns: 1fr;
  }
}
</style>
