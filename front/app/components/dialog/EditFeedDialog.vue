<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { RssFeed } from '~/types'

const props = defineProps<{
  feed: RssFeed
}>()

const emit = defineEmits<{
  close: []
  updated: []
  deleted: []
}>()

const apiStore = useApiStore()

const url = ref(props.feed.url)
const categoryId = ref<number | undefined>(props.feed.category ? Number(props.feed.category) : undefined)
const title = ref(props.feed.title)
const description = ref(props.feed.description)
const loading = ref(false)
const deleting = ref(false)
const error = ref<string | null>(null)
const showDeleteConfirm = ref(false)

const articleSummaryEnabled = ref(props.feed.articleSummaryEnabled ?? false)
const completionOnRefresh = ref(props.feed.completionOnRefresh ?? true)
const maxCompletionRetries = ref(props.feed.maxCompletionRetries ?? 3)
const firecrawlEnabled = ref(props.feed.firecrawlEnabled ?? false)

watch(() => props.feed, (newFeed) => {
  if (!newFeed) return

  url.value = newFeed.url
  categoryId.value = newFeed.category ? Number(newFeed.category) : undefined
  title.value = newFeed.title
  description.value = newFeed.description
  articleSummaryEnabled.value = newFeed.articleSummaryEnabled ?? false
  completionOnRefresh.value = newFeed.completionOnRefresh ?? true
  maxCompletionRetries.value = newFeed.maxCompletionRetries ?? 3
  firecrawlEnabled.value = newFeed.firecrawlEnabled ?? false
}, { deep: true })

const capabilityItems = computed(() => [
  {
    label: '自动总结',
    enabled: articleSummaryEnabled.value,
    icon: 'mdi:brain',
  },
  {
    label: '全文抓取',
    enabled: firecrawlEnabled.value,
    icon: 'mdi:spider-web',
  },
  {
    label: '刷新后自动总结',
    enabled: completionOnRefresh.value,
    icon: 'mdi:refresh-auto',
  },
])

async function handleSubmit() {
  if (!url.value) return

  loading.value = true
  error.value = null

  const response = await apiStore.updateFeed(props.feed.id, {
    url: url.value,
    category_id: categoryId.value,
    article_summary_enabled: articleSummaryEnabled.value,
    completion_on_refresh: completionOnRefresh.value,
    max_completion_retries: maxCompletionRetries.value,
    firecrawl_enabled: firecrawlEnabled.value,
  })

  loading.value = false

  if (response.success) {
    emit('updated')
    emit('close')
    return
  }

  error.value = response.error || '更新订阅源失败'
}

async function handleDelete() {
  deleting.value = true
  error.value = null

  const response = await apiStore.deleteFeed(props.feed.id)

  deleting.value = false

  if (response.success) {
    emit('deleted')
    emit('close')
    return
  }

  error.value = response.error || '删除订阅源失败'
  showDeleteConfirm.value = false
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition ease-out duration-200"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition ease-in duration-150"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
        @click.self="emit('close')"
      >
        <Transition
          enter-active-class="transition ease-out duration-200"
          enter-from-class="opacity-0 scale-95 translate-y-2"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-150"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-2"
        >
          <div
            class="w-full max-w-2xl overflow-hidden rounded-2xl border border-ink-200 bg-white/95 shadow-strong backdrop-blur-sm"
            @click.stop
          >
            <div class="border-b border-ink-200 bg-linear-to-r from-ink-50 to-paper-cream px-6 py-5">
              <div class="flex items-center justify-between gap-4">
                <div class="flex items-center gap-3">
                  <div class="flex h-11 w-11 items-center justify-center rounded-xl bg-linear-to-br from-primary-600 to-primary-800 shadow-lg">
                    <Icon icon="mdi:pencil-box" class="text-white" width="24" height="24" />
                  </div>
                  <div>
                    <h2 class="text-xl font-bold text-gray-800">编辑订阅源</h2>
                    <p class="text-sm text-gray-500">管理抓取与自动总结能力</p>
                  </div>
                </div>
                <button
                  class="rounded-xl p-2.5 transition-all hover:bg-white/70 hover:shadow-md active:scale-95"
                  @click="emit('close')"
                >
                  <Icon icon="mdi:close" width="20" height="20" class="text-gray-500" />
                </button>
              </div>
            </div>

            <div class="grid gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_280px]">
              <div class="space-y-5">
                <div class="rounded-2xl border-2 border-ink-200 bg-linear-to-br from-ink-50 to-paper-cream p-4">
                  <div class="flex items-start gap-3">
                    <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-ink-500 to-ink-700 shadow-md">
                      <Icon icon="mdi:rss" class="text-white" width="22" height="22" />
                    </div>
                    <div class="min-w-0 flex-1">
                      <h3 class="truncate font-semibold text-gray-900">{{ title }}</h3>
                      <p class="mt-1 line-clamp-2 text-sm text-gray-600">
                        {{ description || '暂无描述' }}
                      </p>
                    </div>
                  </div>
                </div>

                <div class="space-y-2">
                  <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                    <Icon icon="mdi:link-variant" width="16" height="16" class="text-ink-500" />
                    RSS 地址
                    <span class="text-red-500">*</span>
                  </label>
                  <input
                    v-model="url"
                    type="url"
                    placeholder="https://example.com/feed.xml"
                    class="input w-full"
                  >
                </div>

                <div class="space-y-2">
                  <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                    <Icon icon="mdi:folder" width="16" height="16" class="text-ink-500" />
                    分类
                    <span class="text-xs font-normal text-gray-400">可选</span>
                  </label>
                  <select v-model="categoryId" class="input w-full cursor-pointer">
                    <option :value="undefined">未分类</option>
                    <option
                      v-for="category in apiStore.categories"
                      :key="category.id"
                      :value="Number(category.id)"
                    >
                      {{ category.name }}
                    </option>
                  </select>
                </div>

                <div class="space-y-4 rounded-2xl border border-blue-200 bg-blue-50/60 p-4">
                  <div class="flex items-center gap-2 text-sm font-semibold text-blue-900">
                    <Icon icon="mdi:brain" width="16" height="16" />
                    自动总结设置
                  </div>

                  <label class="flex cursor-pointer items-center justify-between gap-4">
                    <div>
                      <div class="text-sm font-medium text-gray-800">启用自动总结</div>
                      <div class="text-xs text-gray-500">Firecrawl 完成后进入总结队列</div>
                    </div>
                    <input v-model="articleSummaryEnabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500">
                  </label>

                  <label class="flex cursor-pointer items-center justify-between gap-4">
                    <div>
                      <div class="text-sm font-medium text-gray-800">刷新后自动总结</div>
                      <div class="text-xs text-gray-500">新文章创建后自动进入处理链路</div>
                    </div>
                    <input v-model="completionOnRefresh" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500">
                  </label>

                  <div class="space-y-2">
                    <label class="text-sm font-medium text-gray-800">最大重试次数</label>
                    <input
                      v-model.number="maxCompletionRetries"
                      type="number"
                      min="1"
                      max="10"
                      class="input w-full"
                    >
                  </div>
                </div>

                <div class="space-y-4 rounded-2xl border border-sky-200 bg-sky-50/60 p-4">
                  <div class="flex items-center gap-2 text-sm font-semibold text-sky-900">
                    <Icon icon="mdi:spider-web" width="16" height="16" />
                    全文抓取设置
                  </div>

                  <label class="flex cursor-pointer items-center justify-between gap-4">
                    <div>
                      <div class="text-sm font-medium text-gray-800">启用全文抓取</div>
                      <div class="text-xs text-gray-500">使用 Firecrawl 抓取文章全文后再交给总结能力</div>
                    </div>
                    <input v-model="firecrawlEnabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500">
                  </label>
                </div>

                <div
                  v-if="error"
                  class="flex items-start gap-3 rounded-xl border-2 border-red-200 bg-red-50/80 p-4"
                >
                  <Icon icon="mdi:alert-circle" width="20" height="20" class="mt-0.5 shrink-0 text-red-500" />
                  <p class="text-sm font-medium text-red-700">{{ error }}</p>
                </div>

                <div
                  v-if="showDeleteConfirm"
                  class="rounded-xl border-2 border-red-200 bg-red-50/80 p-4"
                >
                  <div class="flex items-start gap-3">
                    <Icon icon="mdi:alert-circle" class="mt-0.5 shrink-0 text-red-600" width="22" height="22" />
                    <div class="flex-1">
                      <h4 class="mb-1 font-semibold text-red-900">确认删除订阅源？</h4>
                      <p class="mb-3 text-sm text-red-700">
                        删除“{{ title }}”后，该订阅源下的文章也会一起删除，这个操作不可撤销。
                      </p>
                      <div class="flex gap-2">
                        <button class="btn-secondary px-4 py-2" :disabled="deleting" @click="showDeleteConfirm = false">
                          取消
                        </button>
                        <button
                          class="flex items-center gap-2 rounded-xl bg-red-600 px-4 py-2 font-semibold text-white shadow-md transition-all hover:bg-red-700 hover:shadow-lg active:scale-95 disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none disabled:active:scale-100"
                          :disabled="deleting"
                          @click="handleDelete"
                        >
                          <Icon :icon="deleting ? 'mdi:loading' : 'mdi:delete'" :class="{ 'animate-spin': deleting }" width="16" height="16" />
                          {{ deleting ? '删除中...' : '确认删除' }}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <aside class="space-y-4 rounded-2xl border border-ink-200 bg-paper-cream/70 p-4">
                <div>
                  <h3 class="text-sm font-semibold text-ink-800">当前能力状态</h3>
                  <p class="mt-1 text-xs leading-5 text-ink-medium">这里展示这个订阅源会不会进入全文抓取和自动总结链路。</p>
                </div>

                <div class="space-y-3">
                  <div
                    v-for="item in capabilityItems"
                    :key="item.label"
                    class="flex items-center justify-between gap-3 rounded-xl border border-white/70 bg-white/80 px-3 py-2"
                  >
                    <div class="flex items-center gap-2 text-sm text-ink-dark">
                      <Icon :icon="item.icon" width="16" height="16" />
                      <span>{{ item.label }}</span>
                    </div>
                    <span
                      class="inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-semibold"
                      :class="item.enabled ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-stone-200 bg-stone-100 text-stone-500'"
                    >
                      {{ item.enabled ? '已开启' : '未开启' }}
                    </span>
                  </div>
                </div>

                <div class="rounded-xl border border-dashed border-ink-200 bg-white/80 p-4">
                  <div class="text-xs uppercase tracking-[0.08em] text-ink-light">重试策略</div>
                  <div class="mt-2 text-2xl font-semibold text-ink-black">{{ maxCompletionRetries }}</div>
                  <div class="mt-1 text-sm text-ink-medium">自动总结失败后最多重试 {{ maxCompletionRetries }} 次。</div>
                </div>
              </aside>
            </div>

            <div class="flex items-center justify-between border-t border-white/40 bg-white/40 px-6 py-4">
              <button
                v-if="!showDeleteConfirm"
                class="flex items-center gap-2 rounded-xl px-4 py-2.5 font-semibold text-red-600 transition-all hover:bg-red-50/80 active:scale-95"
                @click="showDeleteConfirm = true"
              >
                <Icon icon="mdi:delete" width="18" height="18" />
                删除订阅源
              </button>
              <div v-else class="w-28" />

              <div v-if="!showDeleteConfirm" class="flex gap-3">
                <button class="btn-secondary" @click="emit('close')">取消</button>
                <button
                  class="btn-primary flex items-center gap-2"
                  :disabled="!url || loading"
                  @click="handleSubmit"
                >
                  <Icon :icon="loading ? 'mdi:loading' : 'mdi:check'" :class="{ 'animate-spin': loading }" width="18" height="18" />
                  {{ loading ? '保存中...' : '保存更改' }}
                </button>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
