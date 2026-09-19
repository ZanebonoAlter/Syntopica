<script setup lang="ts">
import { computed } from 'vue'
import { ArticleContentView } from '~/features/articles/public'
import type { Article } from '~/types'

const props = defineProps<{
  visible: boolean
  selectedPreviewArticle: Article | null
  previewArticles: Article[]
  loadingPreviewArticle: boolean
}>()
const emit = defineEmits<{
  close: []
  navigate: [article: Article]
  favorite: [articleId: string]
  'article-update': [articleId: string, updates: Partial<Article>]
}>()

const show = computed({
  get: () => props.visible,
  set: (val: boolean) => { if (!val) emit('close') }
})
</script>

<template>
  <AppDialog v-model="show" width="90vw" :close-on-overlay="true" :close-on-escape="true" :z-index="9100">
    <template #header>
      <p class="preview-header-text">
        {{ loadingPreviewArticle ? '正在准备文章预览...' : '文章预览' }}
      </p>
    </template>
    <!-- 确定高度：85vh（AppDialog 上限）− header ≈61px − body 上下 padding 40px，留 9px 余量（宁小勿大防双滚动条）；
         AppDialog body 非 flex，flex:1 解析不出高度，下游 h-full/flex:1 链（iframe 模式）依赖此确定高度 → change fix-preview-dialog-iframe-height D1 -->
    <div class="preview-body" :style="{ height: 'calc(85vh - 110px)' }">
      <ArticleContentView
        v-if="selectedPreviewArticle"
        :article="selectedPreviewArticle"
        :articles="previewArticles"
        @navigate="(a: Article) => emit('navigate', a)"
        @favorite="(id: string) => emit('favorite', id)"
        @article-update="(id: string, updates: Partial<Article>) => emit('article-update', id, updates)"
      />
    </div>
  </AppDialog>
</template>

<style scoped>
.preview-header-text {
  font-size: 0.875rem;
  color: var(--color-text-muted);
}
</style>
