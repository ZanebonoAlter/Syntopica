/**
 * Articles feature — public facade
 *
 * 跨 feature 共享的稳定 Interface。
 * 其他 feature 如需使用 articles 的组件，必须通过此文件导入，
 * 不得直接深 import features/articles/components/ 内部。
 */

export { default as ArticleContentView } from './components/ArticleContentView.vue'
export { default as ArticleCardView } from './components/ArticleCardView.vue'
export { default as RowStatusPopover } from './components/RowStatusPopover.vue'
export { useArticlePagination } from './composables/useArticlePagination'
export { getArticlePipelineState, getPipelineStateMeta, type PipelineState } from './composables/useArticleProcessingStatus'
