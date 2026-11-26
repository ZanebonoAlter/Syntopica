<script setup lang="ts">
// L0 叙事工坊：默认今日雷达。旧 `?board=&report=&annotation=` 深链在挂载后
// 反查日报日期并 replace 到 `/tags/:boardId/:date?annotation=`；旧五 tab 页面
// 不再从 `/tags` 入口渲染（组件保留到 8.4 物理清理）。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import BoardRadarView from '~/features/tags/components/console/BoardRadarView.vue'
import { useDailyReportsApi } from '~/api/dailyReports'

definePageMeta({
  middleware: 'tags-deep-link',
})

const route = useRoute()
const router = useRouter()
const { getDailyReportDetail } = useDailyReportsApi()
const legacyError = ref('')
const legacyLoading = ref(false)

const legacyReportId = computed(() => {
  const raw = Array.isArray(route.query.report) ? route.query.report[0] : route.query.report
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

const legacyBoardId = computed(() => {
  const raw = Array.isArray(route.query.board) ? route.query.board[0] : route.query.board
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

onMounted(async () => {
  if (!legacyReportId.value) return
  legacyLoading.value = true
  try {
    const res = await getDailyReportDetail(legacyReportId.value)
    if (!res.success || !res.data) {
      legacyError.value = res.error || '旧日报深链已失效'
      return
    }
    const report = res.data.report
    const boardId = legacyBoardId.value ?? report.semantic_board_id
    const date = report.period_date.slice(0, 10)
    const rawAnnotation = Array.isArray(route.query.annotation) ? route.query.annotation[0] : route.query.annotation
    const query = rawAnnotation ? { annotation: String(rawAnnotation) } : {}
    await router.replace({ path: `/tags/${boardId}/${date}`, query })
  } finally {
    legacyLoading.value = false
  }
})
</script>

<template>
  <div v-if="legacyReportId" class="legacy-report-redirect" data-testid="legacy-report-redirect">
    <template v-if="legacyError">
      <Icon icon="mdi:alert-circle-outline" width="24" />
      <p>{{ legacyError }}</p>
      <NuxtLink :to="legacyBoardId ? `/tags/${legacyBoardId}` : '/tags'">返回{{ legacyBoardId ? '版块' : '雷达' }}</NuxtLink>
    </template>
    <template v-else>
      <span class="legacy-spinner" aria-hidden="true" />
      <p>{{ legacyLoading ? '正在打开原日报…' : '正在定位日报…' }}</p>
    </template>
  </div>
  <BoardRadarView v-else />
</template>

<style scoped>
.legacy-report-redirect {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  min-height: 60vh;
  color: var(--color-text-muted);
  font-size: 0.82rem;
}

.legacy-report-redirect a {
  color: var(--color-accent);
  font-size: 0.78rem;
}

.legacy-spinner {
  width: 1.2rem;
  height: 1.2rem;
  border: 2px solid var(--color-border-medium);
  border-top-color: var(--color-accent);
  border-radius: 50%;
  animation: legacy-spin 0.8s linear infinite;
}

@keyframes legacy-spin {
  to { transform: rotate(360deg); }
}
</style>
