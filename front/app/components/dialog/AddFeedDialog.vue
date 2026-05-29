<script setup lang="ts">
import { Icon } from "@iconify/vue";
import { useFeedsApi } from '~/api/feeds'

const feedsApi = useFeedsApi()

const emit = defineEmits<{
  close: []
  added: []
}>()

const apiStore = useApiStore()
const feedsStore = useFeedsStore()

const url = ref('')
const categoryId = ref<number | undefined>(undefined)
const loading = ref(false)
const error = ref<string | null>(null)
const previewing = ref(false)
const preview = ref<any>(null)

async function handlePreview() {
  if (!url.value) return

  previewing.value = true
  error.value = null

  try {
    const response = await feedsApi.fetchFeed(url.value)
    if (categoryId.value !== undefined && response.success && response.data) {
      preview.value = {
        ...response.data,
        category_id: categoryId.value,
      }
      return
    }

    if (response.success && response.data) {
      preview.value = response.data
    } else {
      error.value = response.error || 'Failed to fetch feed'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch feed'
  } finally {
    previewing.value = false
  }
}

async function handleAdd() {
  if (!url.value) return

  loading.value = true
  error.value = null

  const response = await apiStore.createFeed({
    url: url.value,
    category_id: categoryId.value,
  })

  loading.value = false

  if (response.success) {
    emit('added')
    emit('close')
  } else {
    error.value = response.error || 'Failed to add feed'
  }
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
        class="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50 p-4"
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
            class="bg-white/95 backdrop-blur-sm rounded-lg shadow-strong w-full max-w-lg overflow-hidden border border-ink-200"
            @click.stop
          >
            <!-- Header -->
            <div class="px-6 py-5 bg-gradient-to-r from-ink-50 to-paper-cream border-b border-ink-200">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <div class="w-11 h-11 rounded-lg bg-gradient-to-br from-ink-500 to-ink-700 flex items-center justify-center shadow-md">
                    <Icon icon="mdi:rss" class="text-white" width="24" height="24" />
                  </div>
                  <div>
                    <h2 class="text-xl font-bold text-gray-800">添加订阅源</h2>
                    <p class="text-sm text-gray-500">输入 RSS 订阅地址</p>
                  </div>
                </div>
                <button
                  class="p-2.5 hover:bg-white/70 rounded-xl transition-all hover:shadow-md active:scale-95"
                  @click="emit('close')"
                >
                  <Icon icon="mdi:close" width="20" height="20" class="text-gray-500" />
                </button>
              </div>
            </div>

            <div class="p-6 space-y-5">
              <!-- URL Input -->
              <div class="space-y-2">
                <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                  <Icon icon="mdi:link-variant" width="16" height="16" class="text-ink-500" />
                  RSS 订阅地址
                  <span class="text-red-500">*</span>
                </label>
                <div class="flex gap-2">
                  <input
                    v-model="url"
                    type="url"
                    placeholder="https://example.com/feed.xml"
                    class="input flex-1"
                    @keyup.enter="handlePreview"
                  >
                  <button
                    class="btn-secondary px-4"
                    :disabled="!url || previewing"
                    @click="handlePreview"
                  >
                    <Icon
                      :icon="previewing ? 'mdi:loading' : 'mdi:magnify'"
                      :class="{ 'animate-spin': previewing }"
                      width="20"
                      height="20"
                    />
                  </button>
                </div>
              </div>

              <!-- Category Select -->
              <div class="space-y-2">
                <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                  <Icon icon="mdi:folder" width="16" height="16" class="text-ink-500" />
                  分类
                  <span class="text-xs font-normal text-gray-400">(可选)</span>
                </label>
                <select
                  v-model="categoryId"
                  class="input w-full cursor-pointer"
                >
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

              <!-- Preview -->
              <Transition
                enter-active-class="transition ease-out duration-200"
                enter-from-class="opacity-0 translate-y-1"
                enter-to-class="opacity-100 translate-y-0"
                leave-active-class="transition ease-in duration-150"
                leave-from-class="opacity-100 translate-y-0"
                leave-to-class="opacity-0 translate-y-1"
              >
                <div
                  v-if="preview && !previewing"
                  class="p-4 bg-gradient-to-br from-ink-50 to-paper-cream rounded-lg border-2 border-ink-200"
                >
                  <div class="flex items-start gap-3">
                  <div class="w-11 h-11 rounded-lg bg-gradient-to-br from-ink-500 to-ink-700 flex items-center justify-center shrink-0 shadow-md">
                      <Icon icon="mdi:rss" class="text-white" width="22" height="22" />
                    </div>
                    <div class="flex-1 min-w-0">
                      <h3 class="font-semibold text-gray-900 mb-1 truncate">
                        {{ preview.title || '无标题' }}
                      </h3>
                      <p class="text-sm text-gray-600 mb-2.5 line-clamp-2">
                        {{ preview.description || '无描述' }}
                      </p>
                      <div class="flex items-center gap-2">
                        <span class="inline-flex items-center px-3 py-1.5 rounded-lg bg-ink-100 text-ink-700 text-xs font-semibold">
                          <Icon icon="mdi:article" width="14" height="14" class="mr-1.5" />
                          {{ preview.article_count || 0 }} 篇文章
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              </Transition>

              <!-- Error -->
              <Transition
                enter-active-class="transition ease-out duration-200"
                enter-from-class="opacity-0 translate-y-1"
                enter-to-class="opacity-100 translate-y-0"
                leave-active-class="transition ease-in duration-150"
                leave-from-class="opacity-100 translate-y-0"
                leave-to-class="opacity-0 translate-y-1"
              >
                <div
                  v-if="error"
                  class="flex items-start gap-3 p-4 bg-red-50/80 backdrop-blur-sm border-2 border-red-200 rounded-lg"
                >
                  <Icon icon="mdi:alert-circle" width="20" height="20" class="text-red-500 shrink-0 mt-0.5" />
                  <p class="text-sm font-medium text-red-700">{{ error }}</p>
                </div>
              </Transition>
            </div>

            <!-- Footer -->
            <div class="px-6 py-4 bg-white/40 border-t border-white/40 flex justify-end gap-3">
              <button class="btn-secondary" @click="emit('close')">
                取消
              </button>
              <button
                class="btn-primary flex items-center gap-2"
                :disabled="!url || loading"
                @click="handleAdd"
              >
                <Icon
                  :icon="loading ? 'mdi:loading' : 'mdi:plus'"
                  :class="{ 'animate-spin': loading }"
                  width="18"
                  height="18"
                />
                {{ loading ? '添加中...' : '添加订阅源' }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
