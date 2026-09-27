<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import { useDiscoveryApi } from '~/api/discovery'
import { useDiscoveryStore } from '~/stores/discovery'
import type {
  CatalogApplyResult,
  CatalogExportFile,
  CatalogImportPreview,
} from '~/types/discovery'

/**
 * 候选目录导入弹窗（improve-discovery-recommendations 5.3，spec C3）。
 *
 * 状态机：pick（选择/拖入文件）→ preview（分类列表）→ result（逐项结果）；
 * confirm 提交中为瞬时态（按钮 loading）。取消/关闭零副作用——preview 零写库，
 * confirm 之前不发生任何写请求。
 *
 * - 前端预检：≤2MiB（超限就地报错可换文件）、可解析 JSON（非 JSON 就地报错可换文件）。
 * - 预览分类：new / duplicate / conflict / invalid 各自徽标与逐条原因（invalid 原因就地展示）。
 * - 确认按钮仅当存在 new 条目时可用；confirm 携带 {fingerprint, local_revision, 文件本体}。
 * - 409 stale_preview：提示「目录已变化，请重新预览」并自动以原完整文件重新预览。
 * - 重试失败项：以 failed 条目组成子集重新走 preview+confirm，已成功项不在子集中、不重复。
 */
const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const api = useDiscoveryApi()
const store = useDiscoveryStore()

/** 导入文件上限（后端 CatalogImportMaxBytes = 2 MiB，前端预检同值） */
const MAX_FILE_BYTES = 2 * 1024 * 1024

type Phase = 'pick' | 'preview' | 'result'

const phase = ref<Phase>('pick')
/** 最初解析的完整文件（stale 重新预览 / 判定文件级问题用） */
const originalFile = ref<CatalogExportFile | null>(null)
/** 当前工作集（重试失败项后 = 失败子集） */
const workingFile = ref<CatalogExportFile | null>(null)
const previewData = ref<CatalogImportPreview | null>(null)
const applyResult = ref<CatalogApplyResult | null>(null)
/** pick 步的文件级错误（超限/坏 JSON/后端 400），就地展示可换文件 */
const pickError = ref<string | null>(null)
const previewing = ref(false)
const confirming = ref(false)
const staleNotice = ref<string | null>(null)
const confirmError = ref<string | null>(null)
/** 重试来源标记：result 视图里区分「初次结果」与「重试后的结果」 */
const retrying = ref(false)

const visible = computed(() => props.modelValue)

watch(visible, (v) => {
  if (v) reset()
})

function reset() {
  phase.value = 'pick'
  originalFile.value = null
  workingFile.value = null
  previewData.value = null
  applyResult.value = null
  pickError.value = null
  staleNotice.value = null
  confirmError.value = null
  retrying.value = false
}

/* —————— 分类展示 —————— */

const classificationMeta: Record<string, { label: string, tone: string }> = {
  new: { label: '新增', tone: 'is-new' },
  duplicate: { label: '重复', tone: 'is-duplicate' },
  conflict: { label: '冲突', tone: 'is-conflict' },
  invalid: { label: '无效', tone: 'is-invalid' },
}

const hasNew = computed(() => (previewData.value?.counts.new ?? 0) > 0)

const canConfirm = computed(() => hasNew.value && !confirming.value && !previewing.value)

/* —————— pick：文件选择 / 拖入 —————— */

async function pickFile(file: File | null | undefined) {
  if (!file) return
  pickError.value = null
  if (file.size > MAX_FILE_BYTES) {
    pickError.value = `文件超过 2 MiB 上限（当前 ${(file.size / 1024 / 1024).toFixed(1)} MiB），请换一个小文件`
    return
  }
  let text: string
  try {
    text = await file.text()
  } catch {
    pickError.value = '文件读取失败，请重新选择'
    return
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    pickError.value = '文件不是有效的 JSON，请检查后重选'
    return
  }
  const fileJson = parsed as CatalogExportFile
  if (!fileJson || typeof fileJson !== 'object' || !Array.isArray(fileJson.entries)) {
    pickError.value = '文件结构不符合候选目录格式（缺少 entries 列表），请检查后重选'
    return
  }
  originalFile.value = fileJson
  workingFile.value = fileJson
  retrying.value = false
  await runPreview(fileJson)
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  void pickFile(input.files?.[0])
  input.value = ''
}

function onDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  void pickFile(e.dataTransfer?.files?.[0])
}

const dragOver = ref(false)

function onDragOver(e: DragEvent) {
  e.preventDefault()
  dragOver.value = true
}

function onDragLeave() {
  dragOver.value = false
}

/* —————— preview —————— */

async function runPreview(file: CatalogExportFile) {
  previewing.value = true
  previewData.value = null
  const res = await api.previewCatalogImport(file)
  previewing.value = false
  if (res.success && res.data) {
    previewData.value = res.data
    phase.value = 'preview'
    return
  }
  // 文件级 400（未知 format/version/非法 kind/超限）：回 pick 就地报错可换文件
  pickError.value = res.error || '预览失败，请检查文件'
  phase.value = 'pick'
}

/** 回到选择文件步（换文件；已预览数据作废——预览零写库，无副作用） */
function backToPick() {
  phase.value = 'pick'
  previewData.value = null
  originalFile.value = null
  workingFile.value = null
  staleNotice.value = null
}

/* —————— confirm —————— */

async function confirm() {
  if (!canConfirm.value || !previewData.value || !workingFile.value) return
  confirming.value = true
  confirmError.value = null
  staleNotice.value = null
  const res = await api.confirmCatalogImport({
    fingerprint: previewData.value.fingerprint,
    localRevision: previewData.value.local_revision,
    file: workingFile.value,
  })
  confirming.value = false
  if (res.success && res.data) {
    applyResult.value = res.data
    phase.value = 'result'
    // 导入改变了候选库：静默刷新列表（不打断结果展示）
    if (store.candidatesLoaded) void store.loadCandidates()
    return
  }
  if (res.status === 409) {
    // 目录已变化：提示并自动以原完整文件重新预览（用户重新确认，不自动写库）
    staleNotice.value = '候选目录在预览后发生了变化，已为你重新预览，请核对后再确认。'
    confirmError.value = null
    if (originalFile.value) {
      workingFile.value = originalFile.value
      retrying.value = false
      await runPreview(originalFile.value)
    }
    return
  }
  confirmError.value = res.error || '确认导入失败，请重试'
}

/* —————— result：重试失败项 —————— */

const failedKeys = computed(() => applyResult.value?.failed.map(f => f.stable_key) ?? [])

const canRetryFailed = computed(() => failedKeys.value.length > 0)

/**
 * 重试失败项：以 failed 条目组成子集重新走 preview+confirm。
 * 已成功条目不在子集中——服务端按 stable_key 幂等，重复确认也是 duplicate 复用。
 */
async function retryFailed() {
  if (!canRetryFailed.value || !originalFile.value) return
  const subset: CatalogExportFile = {
    format: originalFile.value.format,
    version: originalFile.value.version,
    entries: originalFile.value.entries.filter(e => failedKeys.value.includes(e.stable_key)),
  }
  if (subset.entries.length === 0) return
  workingFile.value = subset
  retrying.value = true
  applyResult.value = null
  confirmError.value = null
  staleNotice.value = null
  await runPreview(subset)
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="visible"
    title="导入候选目录"
    size="lg"
    :close-on-overlay="false"
    @update:model-value="close"
  >
    <div class="imp">
      <!-- 选择/拖入文件步 -->
      <template v-if="phase === 'pick'">
        <label
          class="imp__drop"
          :class="{ 'is-over': dragOver }"
          data-testid="import-dropzone"
          @dragover="onDragOver"
          @dragleave="onDragLeave"
          @drop="onDrop"
        >
          <Icon icon="mdi:file-upload-outline" width="32" height="32" style="color: var(--color-text-muted)" />
          <span class="imp__drop-text">选择或拖入导出的 JSON 文件（≤ 2 MiB）</span>
          <span class="imp__drop-hint">导入只入候选库，不自动订阅、不发起网络探测。</span>
          <input
            type="file"
            accept="application/json,.json"
            class="imp__file-input"
            data-testid="import-file-input"
            @change="onFileChange"
          >
        </label>
        <p v-if="pickError" class="imp__error" role="alert" data-testid="import-pick-error">{{ pickError }}</p>
        <p v-else-if="previewing" class="imp__muted" data-testid="import-preview-loading">正在解析并预览…</p>
      </template>

      <!-- 预览步：分类列表 -->
      <template v-else-if="phase === 'preview' && previewData">
        <p v-if="staleNotice" class="imp__stale" role="alert" data-testid="import-stale-notice">{{ staleNotice }}</p>

        <div class="imp__counts" data-testid="import-counts">
          <span class="imp__count is-new">新增 {{ previewData.counts.new }}</span>
          <span class="imp__count is-duplicate">重复 {{ previewData.counts.duplicate }}</span>
          <span class="imp__count is-conflict">冲突 {{ previewData.counts.conflict }}</span>
          <span class="imp__count is-invalid">无效 {{ previewData.counts.invalid }}</span>
        </div>
        <p v-if="retrying" class="imp__muted" data-testid="import-retry-note">
          以下是重试范围（仅上次失败的条目）；已成功的条目不会重复创建。
        </p>

        <ul class="imp__items" data-testid="import-preview-items">
          <li
            v-for="(item, i) in previewData.items"
            :key="`${item.stable_key}-${i}`"
            class="imp__item"
            :data-testid="`import-item-${i}`"
          >
            <div class="imp__item-head">
              <span class="imp__badge" :class="classificationMeta[item.classification]?.tone">
                {{ classificationMeta[item.classification]?.label ?? item.classification }}
              </span>
              <span class="imp__item-name u-break-title">{{ item.name || item.stable_key }}</span>
              <span class="imp__item-kind">{{ item.kind || 'rss' }}</span>
            </div>
            <p v-if="item.reason" class="imp__item-reason u-break-url" :data-testid="`import-item-reason-${i}`">
              {{ item.reason }}
            </p>
          </li>
        </ul>

        <p class="imp__muted">
          确认后只应用「新增」条目；重复项复用已有记录，冲突默认保留本地不覆盖。
        </p>
        <p v-if="confirmError" class="imp__error" role="alert" data-testid="import-confirm-error">{{ confirmError }}</p>
      </template>

      <!-- 结果步：逐项结果 -->
      <template v-else-if="phase === 'result' && applyResult">
        <p class="imp__result-lead" data-testid="import-result-lead">
          导入完成：应用 {{ applyResult.applied.length }} 条，重复复用 {{ applyResult.skipped_duplicate.length }} 条，
          冲突跳过 {{ applyResult.skipped_conflict.length }} 条，失败 {{ applyResult.failed.length }} 条。
          已导入的条目均未订阅。
        </p>

        <section v-if="applyResult.applied.length > 0" class="imp__group" data-testid="import-result-applied">
          <h4 class="imp__group-title is-ok">已应用（未订阅）</h4>
          <ul class="imp__plain-list">
            <li v-for="key in applyResult.applied" :key="key" class="u-break-url">{{ key }}</li>
          </ul>
        </section>

        <section v-if="applyResult.skipped_duplicate.length > 0" class="imp__group" data-testid="import-result-duplicate">
          <h4 class="imp__group-title">重复（复用已有记录）</h4>
          <ul class="imp__plain-list">
            <li v-for="key in applyResult.skipped_duplicate" :key="key" class="u-break-url">{{ key }}</li>
          </ul>
        </section>

        <section v-if="applyResult.skipped_conflict.length > 0" class="imp__group" data-testid="import-result-conflict">
          <h4 class="imp__group-title is-warn">冲突（保留本地）</h4>
          <ul class="imp__plain-list">
            <li v-for="(f, i) in applyResult.skipped_conflict" :key="`${f.stable_key}-${i}`" class="u-break-url">
              {{ f.stable_key }} — {{ f.reason }}
            </li>
          </ul>
        </section>

        <section v-if="applyResult.failed.length > 0" class="imp__group" data-testid="import-result-failed">
          <h4 class="imp__group-title is-error">失败</h4>
          <ul class="imp__plain-list">
            <li v-for="(f, i) in applyResult.failed" :key="`${f.stable_key}-${i}`" class="u-break-url">
              {{ f.stable_key }} — {{ f.reason }}
            </li>
          </ul>
          <AppButton
            size="sm"
            variant="secondary"
            :disabled="confirming || previewing"
            data-testid="import-retry-failed"
            @click="retryFailed"
          >
            重试失败项
          </AppButton>
        </section>

        <section v-if="applyResult.invalid.length > 0" class="imp__group" data-testid="import-result-invalid">
          <h4 class="imp__group-title is-muted">无效（未写入）</h4>
          <ul class="imp__plain-list">
            <li v-for="(f, i) in applyResult.invalid" :key="`${f.stable_key}-${i}`" class="u-break-url">
              {{ f.stable_key }} — {{ f.reason }}
            </li>
          </ul>
        </section>

        <p v-if="confirmError" class="imp__error" role="alert" data-testid="import-confirm-error">{{ confirmError }}</p>
      </template>
    </div>

    <template #footer>
      <template v-if="phase === 'pick'">
        <AppButton variant="ghost" size="sm" data-testid="import-cancel" @click="close">取消</AppButton>
      </template>
      <template v-else-if="phase === 'preview'">
        <AppButton variant="ghost" size="sm" data-testid="import-back-pick" @click="backToPick">换文件</AppButton>
        <AppButton
          variant="primary"
          size="sm"
          :loading="confirming"
          :disabled="!canConfirm"
          data-testid="import-confirm"
          @click="confirm"
        >
          {{ hasNew ? `确认导入（${previewData?.counts.new ?? 0} 条新增）` : '没有可应用的新增条目' }}
        </AppButton>
      </template>
      <template v-else>
        <AppButton variant="primary" size="sm" data-testid="import-done" @click="close">完成</AppButton>
      </template>
    </template>
  </AppDialog>
</template>

<style scoped>
.imp {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  max-height: 60vh;
  overflow-y: auto;
}

.imp__drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 36px 20px;
  border: 1.5px dashed var(--color-border-medium);
  border-radius: 12px;
  text-align: center;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}

.imp__drop.is-over {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.imp__drop-text {
  font-size: 14px;
  color: var(--color-text-primary);
}

.imp__drop-hint {
  font-size: 12px;
  color: var(--color-text-muted);
}

.imp__file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.imp__error {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  color: var(--color-error);
  font-size: 12px;
  line-height: 1.6;
}

.imp__stale {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--color-warning-subtle, var(--color-border-medium));
  background: var(--color-warning-subtle, var(--color-bg-sunken));
  color: var(--color-warning, var(--color-text-secondary));
  font-size: 12px;
  line-height: 1.6;
}

.imp__muted {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.imp__counts {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.imp__count {
  font-size: 12px;
  font-weight: 500;
  padding: 2px 10px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
}

.imp__count.is-new {
  color: var(--color-success);
  border-color: var(--color-success-subtle);
  background: var(--color-success-subtle);
}

.imp__count.is-conflict {
  color: var(--color-warning);
}

.imp__count.is-invalid {
  color: var(--color-error);
}

.imp__items {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.imp__item {
  padding: 8px 12px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  min-width: 0;
}

.imp__item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.imp__badge {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
}

.imp__badge.is-new {
  color: var(--color-success);
  border-color: var(--color-success-subtle);
  background: var(--color-success-subtle);
}

.imp__badge.is-conflict {
  color: var(--color-warning);
}

.imp__badge.is-invalid {
  color: var(--color-error);
}

.imp__item-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 0;
}

.imp__item-kind {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.imp__item-reason {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.u-break-title {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.u-break-url {
  overflow-wrap: anywhere;
  word-break: break-all;
}

.imp__result-lead {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-primary);
}

.imp__group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.imp__group-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.imp__group-title.is-ok {
  color: var(--color-success);
}

.imp__group-title.is-warn {
  color: var(--color-warning);
}

.imp__group-title.is-error {
  color: var(--color-error);
}

.imp__group-title.is-muted {
  color: var(--color-text-muted);
}

.imp__plain-list {
  list-style: none;
  margin: 0;
  padding: 0 0 4px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 12px;
  color: var(--color-text-secondary);
}
</style>
