<script setup lang="ts">
/**
 * 日报阅读台 `/tags/:boardId/:date`（rework-workshop-into-radar-and-desk 6.x）。
 * 参数校验留在路由层；非法日期先反查该版块最近一期并 replace，
 * 避免旧链接/手输日期落到空白错误页。
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import DailyReportDeskShell from '~/features/tags/components/daily-report/DailyReportDeskShell.vue'
import { useDailyReportsApi } from '~/api/dailyReports'

const route = useRoute()
const router = useRouter()
const { getBoardDailyReports } = useDailyReportsApi()
const redirecting = ref(false)
const redirectError = ref('')

const boardId = computed(() => {
  const raw = Array.isArray(route.params.boardId) ? route.params.boardId[0] : route.params.boardId
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

const date = computed(() => {
  const raw = Array.isArray(route.params.date) ? route.params.date[0] : route.params.date
  const value = String(raw || '')
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const parsed = new Date(`${value}T12:00:00`)
  return Number.isNaN(parsed.getTime()) ? null : value
})

onMounted(async () => {
  if (boardId.value === null || date.value !== null) return
  redirecting.value = true
  try {
    const response = await getBoardDailyReports(boardId.value, { days: 90 })
    const latest = response.success ? response.data?.reports?.[0]?.period_date?.slice(0, 10) : ''
    if (latest) {
      await router.replace({
        path: `/tags/${boardId.value}/${latest}`,
        query: { ...route.query, fallback: 'invalid-date' },
      })
      return
    }
    redirectError.value = '这个版块还没有日报。'
  } finally {
    redirecting.value = false
  }
})
</script>

<template>
  <DailyReportDeskShell v-if="boardId !== null && date !== null" :board-id="boardId" :date="date" />
  <div v-else class="desk-invalid" data-testid="desk-invalid-date">
    <Icon :icon="redirecting ? 'mdi:loading' : 'mdi:alert-circle-outline'" width="28" :class="{ 'is-spin': redirecting }" />
    <p>{{ redirecting ? '日期无效，正在打开最近一期…' : (redirectError || (boardId === null ? '版块地址无效。' : '日报日期无效。')) }}</p>
    <NuxtLink :to="boardId ? `/tags/${boardId}` : '/tags'">返回版块矩阵</NuxtLink>
  </div>
</template>

<style scoped>
.desk-invalid {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  height: 100vh;
  height: 100dvh;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.desk-invalid a {
  color: var(--color-accent);
  font-size: 0.82rem;
}

.is-spin {
  animation: desk-spin 0.9s linear infinite;
}

@keyframes desk-spin {
  to { transform: rotate(360deg); }
}
</style>
