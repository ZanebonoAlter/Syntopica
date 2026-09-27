<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import { useApiStore } from '~/stores/api'
import { useDiscoveryApi } from '~/api/discovery'
import { useDiscoveryStore } from '~/stores/discovery'
import { useRsshubApi } from '~/api/rsshub'
import {
  DEFAULT_RSSHUB_BASE_URL,
  DEFAULT_RSSHUB_DOC_BASE,
  buildRouteDocUrl,
  buildRouteParamSpecs,
  buildRSSHubFeedUrl,
} from '~/utils/routeParams'
import type { CandidateDetail } from '~/types/discovery'

/**
 * 候选订阅弹窗（improve-discovery-recommendations 5.3，R6 原生/RSSHub 双路径）。
 *
 * 数据源：打开时 GET /discovery/candidates/:id 拉候选详情（rss 附 feed_url、rsshub 附
 * 上游 Route）；RSSHub 实例基址从 settings 拉取，失败兜底默认常量。
 *
 * - 原生 RSS：展示规范化后地址，用户确认后直接建源（POST /api/feeds）。
 * - RSSHub 需参数：复用 buildRouteParamSpecs（目录 options 次之 / 无选项输入框兜底）
 *   + 官方文档链接；提交先 POST /feeds/fetch 验证可解析，通过才建源。
 * - 原生 RSS 不显示 RSSHub 文档链接（不伪造）。
 * - 成功「已订阅」+ 提交禁用（防重复创建）；失败保留输入与所选分类，可重试。
 * - 409（地址已存在）：视为已订阅，不重复创建。
 */
const props = defineProps<{
  modelValue: boolean
  /** 目标候选 id（候选库条目或查询 run 条目的 candidateId） */
  candidateId: string | null
  /** 预填名称（run 条目响应无详情时兜底展示） */
  presetName?: string
  /** RSSHub 官方文档基址（父级注入）；缺省兜底默认常量 */
  docBase?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'subscribed': [candidateId: string]
}>()

const discoveryApi = useDiscoveryApi()
const rsshubApi = useRsshubApi()
const apiStore = useApiStore()
const store = useDiscoveryStore()

const loading = ref(false)
const loadError = ref<string | null>(null)
const detail = ref<CandidateDetail | null>(null)
/** RSSHub 实例基址（settings 拉取，失败兜底默认常量） */
const rsshubBase = ref(DEFAULT_RSSHUB_BASE_URL)
/** RSSHub 官方文档基址 */
const effectiveDocBase = ref('')

/** 填参表单值 */
const paramValues = ref<Record<string, string>>({})
const categoryId = ref('')
const submitError = ref<string | null>(null)
/** 已订阅终态（成功或 409 后锁定，禁用提交防重复） */
const done = ref(false)
const doneAsExisting = ref(false)

const visible = computed(() => props.modelValue)

watch(visible, (v) => {
  if (v) void load()
})

async function load() {
  loading.value = true
  loadError.value = null
  detail.value = null
  submitError.value = null
  done.value = false
  doneAsExisting.value = false
  paramValues.value = {}
  categoryId.value = ''

  // 候选详情与 RSSHub 配置并行拉取；配置失败兜底默认值不阻塞订阅
  const [detailRes, statusRes] = await Promise.all([
    props.candidateId ? discoveryApi.getCandidateDetail(props.candidateId) : Promise.resolve(null),
    rsshubApi.getStatus(),
  ])
  if (statusRes.success && statusRes.data?.rsshub_base_url) {
    rsshubBase.value = statusRes.data.rsshub_base_url
  }
  effectiveDocBase.value = props.docBase
    || (statusRes.success && statusRes.data?.rsshub_doc_base ? statusRes.data.rsshub_doc_base : '')
    || DEFAULT_RSSHUB_DOC_BASE

  loading.value = false
  if (detailRes && detailRes.success && detailRes.data) {
    detail.value = detailRes.data
    if (detail.value.subscribed) done.value = true
    // 填参表单初值
    const specs = paramSpecs.value
    paramValues.value = Object.fromEntries(specs.map(s => [s.name, '']))
    return
  }
  if (detailRes && !detailRes.success) {
    loadError.value = detailRes.error || '候选详情加载失败'
    return
  }
  loadError.value = '缺少候选标识，无法订阅'
}

/* —————— 路由参数规格（复用现有规则：目录 options 次之 / 输入框兜底） —————— */

const route = computed(() => detail.value?.route ?? null)

const docUrl = computed(() => {
  if (!route.value) return ''
  return buildRouteDocUrl(effectiveDocBase.value || DEFAULT_RSSHUB_DOC_BASE, route.value.namespace, route.value.path)
})

const paramSpecs = computed(() => {
  if (!route.value) return []
  return buildRouteParamSpecs(route.value.path, route.value.parameters, undefined, docUrl.value)
})

const requiresParams = computed(() =>
  route.value?.requiresParameters === true && paramSpecs.value.some(s => s.required),
)

const missingRequired = computed(() =>
  paramSpecs.value.some(s => s.required && !(paramValues.value[s.name] ?? '').trim()),
)

/** 最终订阅地址：原生 = feed_url；RSSHub = 实例基址 + 路由（填参后）；缺必填为空串 */
const finalUrl = computed(() => {
  const d = detail.value
  if (!d) return ''
  if (d.kind === 'rss') return d.feedUrl
  if (!route.value) return ''
  const parameters: Record<string, string> = {}
  for (const spec of paramSpecs.value) {
    const v = (paramValues.value[spec.name] ?? '').trim()
    if (v) parameters[spec.name] = v
  }
  return buildRSSHubFeedUrl({
    baseUrl: rsshubBase.value,
    namespace: route.value.namespace,
    path: route.value.path,
    parameters,
    example: route.value.example,
    usableDirectly: route.value.usableDirectly,
  })
})

const submitting = computed(() =>
  !!props.candidateId && store.subscribingIds.includes(props.candidateId),
)

const canSubmit = computed(() =>
  !loading.value
  && !loadError.value
  && !done.value
  && !submitting.value
  && !!finalUrl.value
  && !missingRequired.value,
)

const displayName = computed(() => detail.value?.name || props.presetName || '未知来源')

const kindLabel = computed(() => (detail.value?.kind === 'rsshub' ? 'RSSHub 路由' : '原生 RSS'))

/* —————— 提交 —————— */

async function submit() {
  if (!canSubmit.value || !props.candidateId) return
  submitError.value = null
  const res = await store.subscribeFeed({
    candidateId: props.candidateId,
    url: finalUrl.value,
    title: detail.value?.name || props.presetName || undefined,
    categoryId: categoryId.value || undefined,
    // RSSHub 需参数路由：填参后先验证可解析再建源（R6「验证成功后才创建」）；原生确认即建
    verify: detail.value?.kind === 'rsshub',
  })
  if (res.status === 'subscribed' || res.status === 'already') {
    done.value = true
    doneAsExisting.value = res.status === 'already'
    store.markSubscribed(props.candidateId)
    emit('subscribed', props.candidateId)
    return
  }
  // 失败保留输入（参数值 / 分类选择），可重试
  submitError.value = res.error
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <AppDialog
    :model-value="visible"
    title="订阅候选来源"
    size="md"
    :close-on-overlay="false"
    @update:model-value="close"
  >
    <div class="sub">
      <!-- 加载详情 -->
      <div v-if="loading" class="sub__state" data-testid="subscribe-loading">
        <Icon icon="mdi:loading" width="26" height="26" class="sub__spin" />
        <p class="sub__state-text">正在加载候选详情…</p>
      </div>

      <!-- 详情加载失败：可重试 -->
      <div v-else-if="loadError" class="sub__state" data-testid="subscribe-load-error">
        <Icon icon="mdi:alert-circle-outline" width="26" height="26" style="color: var(--color-error)" />
        <p class="sub__state-text">{{ loadError }}</p>
        <AppButton size="sm" variant="secondary" data-testid="subscribe-load-retry" @click="load">重试</AppButton>
      </div>

      <template v-else-if="detail">
        <!-- 概要 -->
        <div class="sub__head">
          <h4 class="sub__name u-break-title">{{ displayName }}</h4>
          <span class="sub__kind">{{ kindLabel }}</span>
        </div>

        <!-- 已订阅终态 -->
        <div v-if="done" class="sub__done" data-testid="subscribe-done">
          <Icon icon="mdi:check-circle-outline" width="20" height="20" />
          <span>{{
            doneAsExisting
              ? '该地址已在订阅列表中，未重复创建。'
              : '已订阅。候选入库与订阅是两件事，目录条目不会因此消失。'
          }}</span>
        </div>

        <template v-else>
          <!-- 原生 RSS：确认规范化后地址 -->
          <div v-if="detail.kind === 'rss'" class="sub__field" data-testid="subscribe-rss-confirm">
            <label class="sub__label">确认订阅地址（已规范化）</label>
            <p class="sub__url u-break-url" data-testid="subscribe-final-url">{{ finalUrl }}</p>
            <p class="sub__hint">确认后按此地址创建订阅；相同地址不会重复创建。</p>
          </div>

          <!-- RSSHub：可用性说明 + 填参表单（复用现有规则） -->
          <template v-else-if="route">
            <div class="sub__field">
              <label class="sub__label">路由地址（{{ route.namespace }}{{ route.path }}）</label>
              <p v-if="finalUrl" class="sub__url u-break-url" data-testid="subscribe-final-url">
                {{ finalUrl }}
              </p>
              <p v-else-if="requiresParams && missingRequired" class="sub__hint" data-testid="subscribe-missing-required">
                还有必填参数未填写，填完后会展示最终地址。
              </p>
            </div>

            <div v-for="spec in paramSpecs" :key="spec.name" class="sub__field">
              <label class="sub__label">
                {{ spec.name }}
                <span v-if="spec.required" class="sub__required">必填</span>
                <span v-else class="sub__optional">（可选）</span>
              </label>
              <p v-if="spec.description" class="sub__hint">{{ spec.description }}</p>
              <select
                v-if="spec.options && spec.options.length > 0"
                v-model="paramValues[spec.name]"
                class="sub__select"
                :data-testid="`subscribe-param-${spec.name}`"
              >
                <option value="" disabled>{{ spec.required ? '请选择（必填）' : '不填则用默认' }}</option>
                <option v-for="opt in spec.options" :key="opt.value" :value="opt.value">
                  {{ opt.label }}（{{ opt.value }}）
                </option>
              </select>
              <AppInput
                v-else
                v-model="paramValues[spec.name]"
                :placeholder="spec.required ? '必填' : '不填则用默认'"
                :data-testid="`subscribe-param-${spec.name}`"
              />
            </div>

            <!-- 官方文档链接：仅 RSSHub 路由展示（原生不伪造 RSSHub 文档链接） -->
            <a
              class="sub__doc-link"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              data-testid="subscribe-doc-link"
            >
              <Icon icon="mdi:book-open-outline" width="14" height="14" />
              官方文档
            </a>
          </template>

          <!-- 分类选择（两种路径共用） -->
          <div class="sub__field">
            <label class="sub__label">订阅到分类</label>
            <select v-model="categoryId" class="sub__select" data-testid="subscribe-category">
              <option value="">不分类</option>
              <option v-for="cat in apiStore.categories" :key="cat.id" :value="cat.id">
                {{ cat.name }}
              </option>
            </select>
          </div>

          <p v-if="submitError" class="sub__error" role="alert" data-testid="subscribe-submit-error">
            {{ submitError }}（已填内容保留，可重试）
          </p>
        </template>
      </template>
    </div>

    <template #footer>
      <AppButton variant="ghost" size="sm" data-testid="subscribe-cancel" @click="close">
        {{ done ? '关闭' : '取消' }}
      </AppButton>
      <AppButton
        v-if="!done"
        variant="primary"
        size="sm"
        :loading="submitting"
        :disabled="!canSubmit"
        data-testid="subscribe-submit"
        @click="submit"
      >
        {{ detail?.kind === 'rsshub' ? '验证并订阅' : '确认订阅' }}
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.sub {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.sub__state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 20px 16px;
}

.sub__state-text {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-secondary);
  text-align: center;
}

.sub__spin {
  color: var(--color-link, var(--color-accent));
  animation: sub-spin 1s linear infinite;
}

@keyframes sub-spin {
  to { transform: rotate(360deg); }
}

.sub__head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.sub__name {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 0;
}

.u-break-title {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.u-break-url {
  overflow-wrap: anywhere;
  word-break: break-all;
}

.sub__kind {
  flex-shrink: 0;
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
}

.sub__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.sub__label {
  font-size: 12px;
  color: var(--color-text-secondary);
}

.sub__required {
  margin-left: 6px;
  font-size: 11px;
  color: var(--color-error);
}

.sub__optional {
  margin-left: 6px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.sub__url {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  border: 1px solid var(--color-border-subtle);
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  color: var(--color-text-secondary);
  min-width: 0;
}

.sub__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.sub__select {
  padding: 8px 12px;
  font-size: 13px;
  border-radius: 8px;
  border: 1px solid var(--color-input-border);
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
}

.sub__doc-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  align-self: flex-start;
  font-size: 13px;
  color: var(--color-link);
  text-decoration: none;
}

.sub__doc-link:hover {
  text-decoration: underline;
}

.sub__done {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--color-success-subtle);
  color: var(--color-success);
  font-size: 13px;
  line-height: 1.6;
}

.sub__error {
  margin: 0;
  color: var(--color-error);
  font-size: 12px;
  line-height: 1.6;
}
</style>
