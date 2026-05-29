<script setup lang="ts">
import ArticleListPanelView from '~/features/shell/components/ArticleListPanelView.vue'
import type { Article } from '~/types'

interface Props {
  articles: Article[]
  selectedCategory?: string | null
  selectedFeed?: string | null
  selectedArticle?: Article | null
  loading?: boolean
  hasMore?: boolean
  total?: number
  startDate?: string
  endDate?: string
}

const props = withDefaults(defineProps<Props>(), {
  selectedCategory: null,
  selectedFeed: null,
  selectedArticle: null,
  loading: false,
  hasMore: false,
  total: 0,
  startDate: '',
  endDate: '',
})

const emit = defineEmits<{
  articleClick: [article: Article]
  articleFavorite: [id: string]
  loadMore: []
  dateFilterChange: [startDate: string, endDate: string]
  dateFilterClear: []
}>()
</script>

<template>
  <ArticleListPanelView
    :articles="props.articles"
    :selected-category="props.selectedCategory"
    :selected-feed="props.selectedFeed"
    :selected-article="props.selectedArticle"
    :loading="props.loading"
    :has-more="props.hasMore"
    :total="props.total"
    :start-date="props.startDate"
    :end-date="props.endDate"
    @article-click="(article: Article) => emit('articleClick', article)"
    @article-favorite="(id: string) => emit('articleFavorite', id)"
    @load-more="() => emit('loadMore')"
    @date-filter-change="(startDate: string, endDate: string) => emit('dateFilterChange', startDate, endDate)"
    @date-filter-clear="() => emit('dateFilterClear')"
  />
</template>