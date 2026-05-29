<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { marked } from 'marked'
import { useContentCompletion, type ContentCompletionStatus } from '~/features/articles/composables/useContentCompletion'

interface Props {
  articleId: string
  summaryStatus?: string
  firecrawlContent?: string
  aiSummary?: string
  readonly?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  summaryStatus: 'complete',
  firecrawlContent: '',
  aiSummary: '',
  readonly: false,
})

const emit = defineEmits<{
  completed: [firecrawlContent: string, aiSummary: string]
}>()

const { loading, completeArticle, getCompletionStatus } = useContentCompletion()

const status = ref<ContentCompletionStatus>({
  summaryStatus: props.summaryStatus as ContentCompletionStatus['summaryStatus'],
  attempts: 0,
  error: null,
  summaryGeneratedAt: null,
})

const isIncomplete = computed(() => status.value.summaryStatus === 'incomplete')
const isPending = computed(() => status.value.summaryStatus === 'pending')
const isFailed = computed(() => status.value.summaryStatus === 'failed')
const isComplete = computed(() => status.value.summaryStatus === 'complete' || Boolean(props.firecrawlContent))
const renderedSummary = computed(() => props.aiSummary ? marked.parse(props.aiSummary) as string : '')

const statusText = computed(() => {
  if (isPending.value) return '正在补全...'
  if (isFailed.value) return '补全失败'
  if (isComplete.value && props.firecrawlContent) return '内容已补全'
  return '内容未补全'
})

const statusIcon = computed(() => {
  if (isPending.value) return 'mdi:loading'
  if (isFailed.value) return 'mdi:alert-circle'
  if (isComplete.value && props.firecrawlContent) return 'mdi:check-circle'
  return 'mdi:file-refresh-outline'
})

const statusClasses = computed(() => {
  if (isPending.value) return 'bg-amber-50 text-amber-700 border-amber-200'
  if (isFailed.value) return 'bg-rose-50 text-rose-700 border-rose-200'
  if (isComplete.value && props.firecrawlContent) return 'bg-emerald-50 text-emerald-700 border-emerald-200'
  return 'bg-stone-100 text-stone-700 border-stone-200'
})

const canComplete = computed(() => !props.readonly && (isIncomplete.value || isFailed.value) && !loading.value)

async function handleComplete() {
  await completeArticle(props.articleId, { force: true })
  const newStatus = await getCompletionStatus(props.articleId)
  status.value = newStatus

  if (newStatus.summaryStatus === 'complete') {
    emit('completed', newStatus.firecrawlContent || props.firecrawlContent || '', newStatus.aiContentSummary || props.aiSummary || '')
  }
}

onMounted(async () => {
  try {
    status.value = await getCompletionStatus(props.articleId)
  } catch {
    // Keep initial status from props if polling fails.
  }
})
</script>

<template>
  <div class="content-completion">
    <div class="completion-status rounded-xl border p-3" :class="statusClasses">
      <Icon :icon="statusIcon" class="h-4 w-4" :class="{ 'animate-spin': isPending }" />
      <div class="min-w-0 flex-1">
        <div class="text-sm font-medium">{{ statusText }}</div>
        <div v-if="status.error" class="mt-1 text-xs opacity-80">{{ status.error }}</div>
      </div>
      <button
        v-if="canComplete"
        class="rounded-lg bg-ink-900 px-3 py-1 text-xs font-medium text-white transition hover:bg-ink-700 disabled:opacity-50"
        :disabled="loading"
        @click="handleComplete"
      >
        {{ loading ? '处理中...' : (isFailed ? '重试' : '补全内容') }}
      </button>
    </div>

    <div v-if="renderedSummary" class="mt-4 rounded-xl border border-purple-200 bg-purple-50 p-4">
      <h3 class="mb-2 text-sm font-semibold text-purple-700">AI 总结</h3>
      <div class="prose max-w-none text-sm" v-html="renderedSummary" />
    </div>
  </div>
</template>

<style scoped>
.content-completion {
  @apply flex flex-col gap-2;
}
</style>
