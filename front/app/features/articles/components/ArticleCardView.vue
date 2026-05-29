<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { Article, RssFeed } from '~/types'
import {
  getFirecrawlStatusMeta,
  getStatusToneClasses,
  getSummaryStatusMeta,
  shouldShowFirecrawlStatus,
  shouldShowSummaryStatus,
} from '~/features/articles/composables/useArticleProcessingStatus'

import '~/components/article/ArticleCard.css'

interface Props {
  article: Article
  compact?: boolean
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  compact: false,
  selected: false,
})

const emit = defineEmits<{
  click: [article: Article]
  favorite: [id: string]
}>()

const feedsStore = useFeedsStore()

const feed = computed(() => feedsStore.feeds.find((f: RssFeed) => f.id === props.article.feedId))
const category = computed(() => feedsStore.getCategoryBySlug(props.article.category))
const firecrawlMeta = computed(() => getFirecrawlStatusMeta(props.article))
const summaryMeta = computed(() => getSummaryStatusMeta(props.article))
const showFirecrawlStatus = computed(() => shouldShowFirecrawlStatus(props.article, feed.value))
const showSummaryStatus = computed(() => shouldShowSummaryStatus(props.article, feed.value))
const hasError = computed(() => Boolean(props.article.firecrawlError || props.article.completionError))
const errorHint = computed(() => props.article.completionError || props.article.firecrawlError || '')
</script>

<template>
  <article
    class="paper-card group article-card cursor-pointer overflow-hidden mx-2 mb-2 first:mt-2"
    :class="{ 'opacity-60': article.read, selected }"
    @click="emit('click', article)"
  >
    <div
      v-if="article.imageUrl && !compact"
      class="aspect-video w-full overflow-hidden bg-paper-warm"
    >
      <img
        :src="article.imageUrl"
        :alt="article.title"
        class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
        loading="lazy"
      >
    </div>

    <div class="p-4">
      <div class="flex items-start gap-3">
        <div
          v-if="feed && !compact"
          class="w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0"
          :style="{ backgroundColor: `${feed.color}20` }"
        >
          <FeedIcon
            :icon="feed.icon"
            :feed-id="article.feedId"
            :color="feed.color"
            :size="20"
          />
        </div>

        <div class="flex-1 min-w-0">
          <div class="flex items-start justify-between gap-2">
            <div class="flex-1 min-w-0">
              <h3
                class="font-semibold text-ink-black group-hover:text-ink-500 transition-colors line-clamp-2"
                :class="{ 'text-sm': compact, 'text-base': !compact }"
              >
                {{ article.title }}
              </h3>

              <div class="mt-2 flex flex-wrap items-center gap-2">
                <span
                  v-if="showFirecrawlStatus"
                  class="inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] font-medium"
                  :class="getStatusToneClasses(firecrawlMeta.tone)"
                >
                  <Icon
                    :icon="firecrawlMeta.icon"
                    width="12"
                    height="12"
                    :class="{ 'animate-spin': article.firecrawlStatus === 'processing' }"
                  />
                  {{ firecrawlMeta.label }}
                </span>
                <span
                  v-if="showSummaryStatus"
                  class="inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] font-medium"
                  :class="getStatusToneClasses(summaryMeta.tone)"
                >
                  <Icon
                    :icon="summaryMeta.icon"
                    width="12"
                    height="12"
                    :class="{ 'animate-spin': article.summaryStatus === 'pending' }"
                  />
                  {{ summaryMeta.label }}
                </span>
                <span
                  class="inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] font-medium"
                  :class="article.tagCount ? 'border-amber-200 bg-amber-50 text-amber-700' : 'border-stone-200 bg-stone-100 text-stone-600'"
                >
                  <Icon :icon="article.tagCount ? 'mdi:tag-multiple' : 'mdi:tag-off-outline'" width="12" height="12" />
                  {{ article.tagCount ? `已标记 ${article.tagCount}` : '待打标签' }}
                </span>
              </div>

              <div
                v-if="hasError"
                class="mt-2 text-xs text-rose-600 line-clamp-1"
                :title="errorHint"
              >
                {{ errorHint }}
              </div>
            </div>

            <button
              class="flex-shrink-0 p-2 hover:bg-amber-50/80 rounded-xl transition-all"
              :class="{ 'text-amber-500': article.favorite, 'text-ink-muted hover:text-amber-500': !article.favorite }"
              @click.stop="emit('favorite', article.id)"
            >
              <Icon
                :icon="article.favorite ? 'mdi:star' : 'mdi:star-outline'"
                width="18"
                height="18"
              />
            </button>
          </div>

          <div class="flex flex-wrap items-center gap-2 mt-3 text-xs text-ink-light">
            <span
              v-if="category"
              class="px-2.5 py-1 rounded-full"
              :style="{ backgroundColor: `${category.color}20`, color: category.color }"
            >
              {{ category.name }}
            </span>
            <span v-if="feed" class="text-ink-medium">{{ feed.title }}</span>
            <span>{{ $dayjs(article.pubDate).fromNow() }}</span>
            <span v-if="article.author">{{ article.author }}</span>
            <span v-if="article.read" class="text-ink-muted">已读</span>
          </div>
        </div>
      </div>
    </div>
  </article>
</template>
