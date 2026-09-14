<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { DiscoveryCandidate } from '~/types/discovery'

/**
 * 候选源新增/编辑弹窗（improve-discovery-recommendations 5.1）。
 *
 * - 语义红线（C7 Candidate Catalog Is Not a Subscription List）：
 *   保存仅写候选库，绝不创建订阅——保存按钮文案「保存到候选库」，
 *   成功提示由 store 发出并明示「尚未订阅」。
 * - 就地校验：名称非空白、RSS 地址合法 http(s) URL；错误显示在字段下，不弹 toast。
 * - 重复地址（409 conflict）：显示「已存在」入口提示 + 查看已有条目按钮，不静默新建。
 * - RSSHub 条目路由地址上游只读（人工补充与上游资料隔离），仅人工字段可编辑。
 * - 提交失败保留全部输入，可重试（State Matrix 编辑/订阅行）。
 */
const props = defineProps<{
  modelValue: boolean
  /** 编辑目标；null = 手动新增 */
  candidate?: DiscoveryCandidate | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  /** 重复地址时跳转已有条目（父级清筛选并高亮） */
  'show-existing': [id: string]
}>()

const store = useDiscoveryStore()

const visible = computed(() => props.modelValue)
const isRsshub = computed(() => props.candidate?.kind === 'rsshub')
const isEdit = computed(() => !!props.candidate)

// —— 表单态 ——
const name = ref('')
const url = ref('')
const description = ref('')
const language = ref('')
const region = ref('')
const participate = ref(true)
/** 重复地址提示（含已有条目入口） */
const duplicateHint = ref<{ existingName: string, existingId: string } | null>(null)
const submitError = ref<string | null>(null)
/** 误提交过一次才显空值红字（不打断初输） */
const touchedName = ref(false)
const touchedUrl = ref(false)

const nameError = computed<string | undefined>(() => {
  if (touchedName.value && !name.value.trim()) return '名称不能为空'
  return undefined
})

/** 仅手动 RSS 地址可编辑/需校验；RSSHub 路由地址只读（上游资料） */
const urlEditable = computed(() => !isRsshub.value)
const urlError = computed<string | undefined>(() => {
  if (!urlEditable.value || !touchedUrl.value) return undefined
  const raw = url.value.trim()
  if (!raw) return 'RSS 地址不能为空'
  try {
    const parsed = new URL(raw)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return '仅支持 http:// 或 https:// 地址'
    }
    if (parsed.username || parsed.password) return '地址不能携带账号或凭据'
  } catch {
    return '请输入完整的 http:// 或 https:// 地址'
  }
  return undefined
})

const canSubmit = computed(() => {
  if (store.candidateSaving) return false
  if (!name.value.trim()) return false
  if (urlEditable.value && !!urlError.value) return false
  return true
})
function resetForm() {
  const c = props.candidate ?? null
  name.value = c?.name ?? ''
  url.value = c?.address ?? ''
  description.value = c?.description ?? ''
  language.value = c?.language ?? ''
  region.value = c?.region ?? ''
  participate.value = c?.recommendationEnabled ?? true
  duplicateHint.value = null
  submitError.value = null
  touchedName.value = false
  touchedUrl.value = false
}

watch(visible, (v) => {
  if (v) resetForm()
}, { immediate: true })

function close() {
  emit('update:modelValue', false)
}

async function submit() {
  touchedName.value = true
  touchedUrl.value = true
  if (!canSubmit.value) return
  duplicateHint.value = null
  submitError.value = null
  const res = await store.saveCandidate(
    {
      name: name.value.trim(),
      url: url.value.trim(),
      description: description.value.trim(),
      language: language.value.trim(),
      region: region.value.trim(),
      recommendationEnabled: participate.value,
    },
    props.candidate?.id,
  )
  if (res.status === 'saved') {
    emit('update:modelValue', false)
    return
  }
  if (res.status === 'duplicate') {
    // 重复地址：不静默新建，给已有条目入口（输入保留）
    duplicateHint.value = res.existing
      ? { existingName: res.existing.name || '未命名候选', existingId: res.existing.id }
      : { existingName: '', existingId: '' }
    return
  }
  submitError.value = res.error
}

function showExisting() {
  if (duplicateHint.value?.existingId) {
    emit('show-existing', duplicateHint.value.existingId)
  }
  close()
}
</script>

<template>
  <AppDialog
    :model-value="visible"
    :title="isEdit ? '编辑候选来源' : '手动新增候选来源'"
    size="md"
    :close-on-overlay="false"
    @update:model-value="close"
  >
    <form class="ced" @submit.prevent="submit">
      <p class="ced__notice">保存仅更新候选源库，不创建订阅，也不抓取文章。</p>

      <div class="ced__field">
        <label class="ced__label" for="ced-name">名称<span class="ced__required">必填</span></label>
        <AppInput
          id="ced-name"
          v-model="name"
          placeholder="如：田野笔记"
          :error="nameError"
          data-testid="candidate-name-input"
          @blur="touchedName = true"
        />
      </div>

      <div class="ced__field">
        <label class="ced__label" for="ced-url">
          {{ isRsshub ? '路由地址（上游只读）' : 'RSS 地址' }}<span v-if="!isRsshub" class="ced__required">必填</span>
        </label>
        <AppInput
          id="ced-url"
          v-model="url"
          placeholder="https://your-source.example/feed.xml"
          :disabled="!urlEditable"
          :error="urlError"
          data-testid="candidate-url-input"
          @blur="touchedUrl = true"
        />
        <p v-if="isRsshub" class="ced__hint">RSSHub 路由地址来自上游目录，人工编辑不会覆盖它。</p>
      </div>

      <div class="ced__field">
        <label class="ced__label" for="ced-description">内容介绍</label>
        <textarea
          id="ced-description"
          v-model="description"
          class="ced__textarea"
          rows="3"
          maxlength="4000"
          placeholder="这个来源主要更新什么内容？"
          data-testid="candidate-description-input"
        />
      </div>

      <div class="ced__row">
        <div class="ced__field">
          <label class="ced__label" for="ced-language">语言</label>
          <AppInput id="ced-language" v-model="language" placeholder="如：中文" data-testid="candidate-language-input" />
        </div>
        <div class="ced__field">
          <label class="ced__label" for="ced-region">地区</label>
          <AppInput id="ced-region" v-model="region" placeholder="如：全球" data-testid="candidate-region-input" />
        </div>
      </div>

      <div class="ced__toggle-row">
        <AppToggle v-model="participate" label="参与推荐" data-testid="candidate-participate-toggle" />
        <p class="ced__hint">参与推荐不是自动订阅；关闭也不会取消已有订阅。</p>
      </div>

      <p v-if="duplicateHint" class="ced__duplicate" role="alert" data-testid="candidate-duplicate-hint">
        候选库已有这个地址，不会重复新建。
        <template v-if="duplicateHint.existingId">
          已有条目：{{ duplicateHint.existingName }}
          <button type="button" class="ced__link" data-testid="candidate-show-existing" @click="showExisting">
            查看已有条目
          </button>
        </template>
      </p>

      <p v-if="submitError" class="ced__error" role="alert" data-testid="candidate-submit-error">
        {{ submitError }}（填写内容已保留，可重试）
      </p>
    </form>

    <template #footer>
      <AppButton variant="ghost" size="sm" data-testid="candidate-cancel" @click="close">
        取消
      </AppButton>
      <AppButton
        variant="primary"
        size="sm"
        :loading="store.candidateSaving"
        data-testid="candidate-save"
        @click="submit"
      >
        保存到候选库
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.ced {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ced__notice {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.ced__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.ced__label {
  font-size: 12px;
  color: var(--color-text-secondary);
}

.ced__required {
  margin-left: 6px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.ced__textarea {
  padding: 8px 12px;
  font-size: 14px;
  font-family: inherit;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  resize: vertical;
}

.ced__textarea:focus {
  border-color: var(--color-input-focus);
  box-shadow: 0 0 0 2px var(--color-accent-subtle);
}

.ced__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.ced__toggle-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.ced__hint {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.ced__duplicate {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.ced__link {
  border: none;
  background: none;
  padding: 0;
  color: var(--color-link, var(--color-accent));
  font-size: 12px;
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.ced__error {
  margin: 0;
  color: var(--color-error);
  font-size: 12px;
}

@media (max-width: 640px) {
  .ced__row {
    grid-template-columns: 1fr;
  }
}
</style>
