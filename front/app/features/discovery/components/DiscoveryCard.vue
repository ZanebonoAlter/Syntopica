<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useDiscoveryStore } from '~/stores/discovery'
import { useApiStore } from '~/stores/api'
import { buildRouteDocUrl, buildRouteParamSpecs, DEFAULT_RSSHUB_DOC_BASE } from '~/utils/routeParams'
import type { DiscoveryRecommendation } from '~/types/discovery'

const props = defineProps<{
  card: DiscoveryRecommendation
  /** RSSHub 官方文档基址（DiscoveryPanel 注入）；缺省兜底默认常量 */
  docBase?: string
}>()

const store = useDiscoveryStore()
const apiStore = useApiStore()

// 填参表单本地状态
const formOpen = ref(false)
const paramValues = ref<Record<string, string>>({})
const categoryId = ref('')

/** 官方文档链接（design D4）；docBase 缺省时兜底默认常量，保证链接始终可达 */
const docUrl = computed(() =>
  buildRouteDocUrl(props.docBase || DEFAULT_RSSHUB_DOC_BASE, props.card.routeNamespace, props.card.routePath),
)

const paramSpecs = computed(() =>
  buildRouteParamSpecs(props.card.routePath, props.card.parameters, props.card.paramOptions, docUrl.value),
)

const missingRequired = computed(() =>
  paramSpecs.value.some(s => s.required && !(paramValues.value[s.name] ?? '').trim()),
)

/** 相似度 0-1 转百分比展示（如 0.855 → 86%） */
const scorePercent = computed(() => Math.round(props.card.score * 100))

/** 匹配版块名；无版块时兜底「全局推荐」 */
const boardLabel = computed(() => props.card.boardLabel || '全局推荐')

const acting = computed(() => store.actingIds.includes(props.card.id))

function openForm() {
  paramValues.value = Object.fromEntries(paramSpecs.value.map(s => [s.name, '']))
  categoryId.value = ''
  formOpen.value = true
}

async function handleAccepted(promise: Promise<boolean>) {
  const ok = await promise
  if (ok) {
    formOpen.value = false
    // 新订阅源进列表：重拉 feeds，侧边栏立即可见
    void apiStore.fetchFeeds({ per_page: 10000 })
  }
}

function acceptDirect() {
  void handleAccepted(
    store.accept(props.card.id, { categoryId: categoryId.value || undefined }),
  )
}

function submitParamForm() {
  if (missingRequired.value) return
  const parameters: Record<string, string> = {}
  for (const spec of paramSpecs.value) {
    const v = (paramValues.value[spec.name] ?? '').trim()
    if (v) parameters[spec.name] = v
  }
  void handleAccepted(
    store.accept(props.card.id, {
      parameters,
      categoryId: categoryId.value || undefined,
    }),
  )
}

function dismiss() {
  void store.dismiss(props.card.id)
}
</script>

<template>
  <div class="discovery-card">
    <div class="discovery-card__head">
      <div class="discovery-card__title-wrap">
        <span
          v-if="card.source === 'qa'"
          class="discovery-card__badge discovery-card__badge--qa"
          aria-label="来自问答"
        >问答</span>
        <span
          v-if="card.requiresParameters"
          class="discovery-card__badge discovery-card__badge--param"
        >需填参数</span>
        <span
          v-else-if="card.usableDirectly"
          class="discovery-card__badge discovery-card__badge--direct"
        >一键订阅</span>
        <strong class="discovery-card__title">{{ card.routeName || card.routePath }}</strong>
      </div>
      <button
        class="discovery-card__dismiss"
        title="不感兴趣（30 天内不再推荐）"
        :disabled="acting"
        @click="dismiss"
      >
        <Icon icon="mdi:close" width="16" height="16" />
      </button>
    </div>

    <p class="discovery-card__ns">{{ card.routeNamespace }}{{ card.routePath }}</p>

    <p class="discovery-card__meta">
      <span class="discovery-card__score">相似度 {{ scorePercent }}%</span>
      <span class="discovery-card__meta-sep" aria-hidden="true">·</span>
      <span class="discovery-card__board">{{ card.boardLabel ? `匹配版块：${boardLabel}` : boardLabel }}</span>
    </p>

    <p v-if="card.llmReason" class="discovery-card__reason">{{ card.llmReason }}</p>

    <p v-if="card.routeExample" class="discovery-card__example">示例：{{ card.routeExample }}</p>

    <div v-if="!formOpen" class="discovery-card__actions">
      <template v-if="card.usableDirectly">
        <AppButton size="sm" :loading="acting" @click="acceptDirect">
          订阅
        </AppButton>
        <select
          v-model="categoryId"
          class="discovery-card__select discovery-card__select--inline"
          title="订阅到分类"
          :disabled="acting"
        >
          <option value="">不分类</option>
          <option v-for="cat in apiStore.categories" :key="cat.id" :value="cat.id">
            {{ cat.name }}
          </option>
        </select>
      </template>
      <AppButton v-else size="sm" variant="secondary" :disabled="acting" @click="openForm">
        填写参数并订阅
      </AppButton>
    </div>

    <!-- 填参表单 -->
    <div v-else class="discovery-card__form">
      <div v-for="spec in paramSpecs" :key="spec.name" class="discovery-card__field">
        <label class="discovery-card__label">
          {{ spec.name }}
          <span v-if="spec.required" class="discovery-card__required">*</span>
          <span v-else class="discovery-card__optional">（可选）</span>
        </label>
        <p v-if="spec.description" class="discovery-card__desc">{{ spec.description }}</p>
        <select
          v-if="spec.options && spec.options.length > 0"
          v-model="paramValues[spec.name]"
          class="discovery-card__select"
        >
          <option value="" disabled>{{ spec.required ? '请选择（必填）' : '不填则用默认' }}</option>
          <option v-for="opt in spec.options" :key="opt.value" :value="opt.value">
            {{ opt.label }}（{{ opt.value }}）
          </option>
        </select>
        <AppInput v-else v-model="paramValues[spec.name]" :placeholder="spec.required ? '必填' : '不填则用默认'" />
      </div>

      <div class="discovery-card__field">
        <label class="discovery-card__label">订阅到分类</label>
        <select v-model="categoryId" class="discovery-card__select">
          <option value="">不分类</option>
          <option v-for="cat in apiStore.categories" :key="cat.id" :value="cat.id">
            {{ cat.name }}
          </option>
        </select>
      </div>

      <div class="discovery-card__form-actions">
        <AppButton size="sm" :loading="acting" :disabled="missingRequired" @click="submitParamForm">
          验证并订阅
        </AppButton>
        <AppButton size="sm" variant="ghost" :disabled="acting" @click="formOpen = false">
          取消
        </AppButton>
        <a
          class="discovery-card__doc-link"
          :href="docUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          <Icon icon="mdi:book-open-outline" width="14" height="14" />
          官方文档
        </a>
      </div>
    </div>
  </div>
</template>

<style scoped>
.discovery-card {
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  padding: 14px 16px;
  background: var(--color-bg-elevated);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.discovery-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}

.discovery-card__title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.discovery-card__title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.discovery-card__badge {
  flex-shrink: 0;
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  font-weight: 500;
}

.discovery-card__badge--direct {
  background: var(--color-success-subtle);
  color: var(--color-success);
}

.discovery-card__badge--param {
  background: var(--color-warning-subtle);
  color: var(--color-warning);
}

.discovery-card__badge--qa {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.discovery-card__dismiss {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.discovery-card__dismiss:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.discovery-card__ns {
  margin: 0;
  font-size: 12px;
  font-family: monospace;
  color: var(--color-text-muted);
  word-break: break-all;
}

.discovery-card__meta {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.discovery-card__score {
  font-weight: 600;
  color: var(--color-accent);
}

.discovery-card__meta-sep {
  color: var(--color-text-muted);
}

.discovery-card__board {
  color: var(--color-text-muted);
}

.discovery-card__reason {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-secondary);
}

.discovery-card__example {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-muted);
  word-break: break-all;
}

.discovery-card__actions {
  display: flex;
  gap: 8px;
  margin-top: 2px;
}

.discovery-card__form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border-top: 1px dashed var(--color-border-subtle);
  padding-top: 10px;
}

.discovery-card__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.discovery-card__label {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
}

.discovery-card__required {
  color: var(--color-error);
}

.discovery-card__optional {
  color: var(--color-text-muted);
  font-weight: 400;
}

.discovery-card__desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--color-text-muted);
  /* 限 3 行防超长上游介绍撑爆卡片 */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
}

.discovery-card__select {
  padding: 8px 12px;
  font-size: 14px;
  border-radius: 8px;
  border: 1px solid var(--color-input-border);
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
}

.discovery-card__select--inline {
  flex: 1;
  min-width: 0;
  max-width: 160px;
  padding: 5px 10px;
  font-size: 13px;
}

.discovery-card__form-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}

.discovery-card__doc-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  font-size: 13px;
  color: var(--color-link);
  text-decoration: none;
}

.discovery-card__doc-link:hover {
  text-decoration: underline;
}
</style>
