<script setup lang="ts">
/**
 * 日报阅读台路由壳（rework-workshop-into-radar-and-desk 6.1）。
 * 四区由 DailyReportReader(mode="desk") 渲染；本壳负责版块/日期取数、路由回写、
 * 返回矩阵保留 `?lane=`、线索文章直达站内阅读预览（daily-report-lead-direct-preview）。
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import DailyReportReader from '../DailyReportReader.vue'
import ArticlePreviewModal from '../ArticlePreviewModal.vue'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { useArticlesApi } from '~/api/articles'
import { useArticleFavoriteToggle } from '~/features/articles/composables/useArticleFavoriteToggle'
import { useNotify } from '~/composables/useNotify'
import type { Article } from '~/types'

const props = defineProps<{
  boardId: number
  date: string
}>()

const route = useRoute()
const router = useRouter()
const boardsApi = useSemanticBoardsApi()
const articlesApi = useArticlesApi()
const notify = useNotify()
const { toggleFavorite } = useArticleFavoriteToggle()

const laneQuery = computed(() => {
  const raw = Array.isArray(route.query.lane) ? route.query.lane[0] : route.query.lane
  return typeof raw === 'string' && /^\d+$/.test(raw) ? raw : ''
})

const annotationId = computed(() => {
  const raw = Array.isArray(route.query.annotation) ? route.query.annotation[0] : route.query.annotation
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

/**
 * `?cluster=` 深链参数（D14）：定位本期正文中该聚簇的第一条 thread。
 * 非数字 / 0 / 负数的非法值忽略（不报错阻断阅读）。
 */
const clusterId = computed(() => {
  const raw = Array.isArray(route.query.cluster) ? route.query.cluster[0] : route.query.cluster
  if (typeof raw !== 'string' || !/^\d+$/.test(raw)) return null
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

/** 旧 `?story=` 深链已下线（D14）：仍被解析，但只用于提示，不定位、不报错。 */
const legacyStory = computed(() => {
  const raw = Array.isArray(route.query.story) ? route.query.story[0] : route.query.story
  return typeof raw === 'string' && raw.length > 0
})

const boards = ref<{ id: number, label: string }[]>([])
const boardTitle = ref('')
const previewOpen = ref(false)
const previewArticle = ref<ReturnType<typeof normalizeArticle> | null>(null)
const loadingPreviewArticle = ref(false)

onMounted(() => {
  if (route.query.fallback !== 'invalid-date') return
  notify.warn('日报日期无效，已打开最近一期')
  const query = { ...route.query }
  delete query.fallback
  void router.replace({ path: route.path, query })
})

onMounted(async () => {
  const response = await boardsApi.getBoards()
  if (response.success && response.data) {
    boards.value = response.data.items.map(board => ({ id: board.id, label: board.label }))
    boardTitle.value = boards.value.find(board => board.id === props.boardId)?.label ?? ''
  }
})

function backToMatrix(): void {
  void router.push({
    path: `/tags/${props.boardId}`,
    query: laneQuery.value ? { lane: laneQuery.value, focus: 'report' } : {},
  })
}

function replaceDate(next: string): void {
  if (next === props.date) return
  void router.replace({
    path: `/tags/${props.boardId}/${next}`,
    query: laneQuery.value ? { lane: laneQuery.value } : {},
  })
}

function openTodayBoard(payload: { boardId: number, date: string }): void {
  void router.replace({
    path: `/tags/${payload.boardId}/${payload.date}`,
    query: laneQuery.value ? { lane: laneQuery.value } : {},
  })
}

function openLane(topicId: number): void {
  void router.push({ path: `/tags/${props.boardId}`, query: { lane: String(topicId) } })
}

/** 线索文章点击直达站内阅读预览（daily-report-lead-direct-preview）：
 * 先开弹窗呈现加载态（header「正在准备文章预览...」），详情返回后填充；
 * 失败关弹窗 + 全局通知，不留空白弹窗（无抽屉中转层） */
async function openLeadArticle(id: number): Promise<void> {
  previewArticle.value = null
  previewOpen.value = true
  loadingPreviewArticle.value = true
  try {
    const response = await articlesApi.getArticle(id)
    if (!response.success || !response.data) {
      previewOpen.value = false
      notify.warn('文章加载失败，请稍后重试')
      return
    }
    previewArticle.value = normalizeArticle(response.data as unknown as ArticlePayload)
  } catch {
    previewOpen.value = false
    notify.warn('文章加载失败，请稍后重试')
  } finally {
    loadingPreviewArticle.value = false
  }
}

/** 预览弹窗收藏：乐观翻转 + 失败回滚（此前为空实现，点收藏毫无反应） */
function handlePreviewFavorite(articleId: string): void {
  void toggleFavorite(articleId, {
    get: () => previewArticle.value,
    apply: (article: Article) => { previewArticle.value = article },
  })
}

/** 预览弹窗内文章状态更新回流到当前文章（如已读等） */
function handlePreviewArticleUpdate(articleId: string, updates: Partial<Article>): void {
  if (!previewArticle.value || previewArticle.value.id !== articleId) return
  previewArticle.value = { ...previewArticle.value, ...updates }
}

function switchBoard(nextBoardId: number): void {
  if (!nextBoardId || nextBoardId === props.boardId) return
  void router.replace({
    path: `/tags/${nextBoardId}/${props.date}`,
    query: laneQuery.value ? { lane: laneQuery.value } : {},
  })
}
</script>

<template>
  <AppPageShell mode="workspace" :gutter="0" as="div">
    <div class="desk-page" data-testid="daily-report-desk">
      <DailyReportReader
        mode="desk"
        :board-id="boardId"
        :board-title="boardTitle"
        :boards="boards"
        :initial-date="date"
        :initial-annotation-id="annotationId"
        :initial-cluster-id="clusterId"
        :initial-legacy-story="legacyStory"
        @close="backToMatrix"
        @select-date="replaceDate"
        @open-lane="openLane"
        @open-article="openLeadArticle($event)"
        @open-today-board="openTodayBoard"
        @select-board="switchBoard"
      />
      <ArticlePreviewModal
        :visible="previewOpen"
        :selected-preview-article="previewArticle"
        :preview-articles="[]"
        :loading-preview-article="loadingPreviewArticle"
        @close="previewOpen = false"
        @favorite="handlePreviewFavorite"
        @article-update="handlePreviewArticleUpdate"
        @navigate="(a) => previewArticle = a"
      />
    </div>
  </AppPageShell>
</template>

<style scoped>
.desk-page {
  height: 100vh;
  height: 100dvh;
  min-height: 0;
  overflow: hidden;
  background: var(--color-bg-base);
}
</style>
