<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type SemanticBoard } from '~/api/semanticBoards'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useDailyReportProgress } from '~/composables/useDailyReportProgress'
import { todayKey } from '~/utils/todayKey'

const props = defineProps<{
  visible: boolean
  boards: SemanticBoard[]
}>()

const emit = defineEmits<{
  cancel: []
  /** 生成真实完成（进度 done 首次为 true）；壳层据此刷新元信息与 A 档。 */
  completed: []
}>()

const show = computed({
  get: () => props.visible,
  set: (val: boolean) => { if (!val) handleClose() }
})

const { generateDailyReport } = useDailyReportsApi()
const { progress, done, totalSaved, totalBoards, reset } = useDailyReportProgress()

watch(done, (value, previous) => {
  if (value && !previous) emit('completed')
})

// 默认日期用**本地**日（todayKey）——`toISOString()` 是 UTC 截断，在 UTC+8 的
// 00:00–08:00 会得到「昨天」（默认日期落在昨天 = 生成窗口错一天）。
const selectedDate = ref(todayKey())
const selectedBoardId = ref<number | null>(null) // null = all
const generating = ref(false)
const jobId = ref<string | null>(null)

const progressEntries = computed(() => Array.from(progress.value.values()))

async function handleGenerate() {
  generating.value = true
  reset()
  jobId.value = null
  try {
    const params: { date: string; board_id?: number } = { date: selectedDate.value }
    if (selectedBoardId.value !== null) {
      params.board_id = selectedBoardId.value
    }
    const res = await generateDailyReport(params)
    if (res.success && res.data) {
      jobId.value = res.data.job_id
    }
  } finally {
    generating.value = false
  }
}

function handleClose() {
  generating.value = false
  jobId.value = null
  reset()
  emit('cancel')
}
</script>

<template>
  <AppDialog v-model="show" title="生成日报" width="400px">
    <div class="ngd-form">
      <label class="form-field">
        <span class="form-label">日期</span>
        <AppInput v-model="selectedDate" type="date" />
      </label>

      <label class="form-field">
        <span class="form-label">板块</span>
        <select v-model="selectedBoardId" class="native-select">
          <option :value="null">全部板块</option>
          <option v-for="board in boards" :key="board.id" :value="board.id">{{ board.label }}</option>
        </select>
      </label>

      <!-- Progress board -->
      <div v-if="jobId || generating" class="progress-card">
        <div class="progress-title">
          <Icon icon="mdi:progress-clock" width="14" style="color: var(--color-text-muted)" />
          <span>{{ done ? '生成完成' : '生成中...' }}</span>
        </div>
        <div v-if="progressEntries.length" class="progress-list">
          <div
            v-for="entry in progressEntries"
            :key="entry.board_id"
            class="progress-row"
          >
            <span class="progress-board">{{ entry.board_name }}</span>
            <span
              class="progress-status"
              :class="{
                'progress-status--waiting': entry.status === 'waiting',
                'progress-status--generating': entry.status === 'generating',
                'progress-status--completed': entry.status === 'completed',
                'progress-status--failed': entry.status === 'failed',
              }"
            >
              <template v-if="entry.status === 'waiting'">等待中</template>
              <template v-else-if="entry.status === 'generating'">生成中 {{ entry.progress }}</template>
              <template v-else-if="entry.status === 'completed'">完成 ({{ entry.saved }})</template>
              <template v-else-if="entry.status === 'failed'">失败</template>
              <template v-else>{{ entry.status }}</template>
            </span>
          </div>
        </div>
        <div v-if="done" class="progress-done">
          共生成 {{ totalSaved }} 篇日报 / {{ totalBoards }} 个板块
        </div>
      </div>
    </div>

    <template #footer>
      <AppButton variant="ghost" size="sm" @click="handleClose">关闭</AppButton>
      <AppButton
        variant="primary"
        size="sm"
        :disabled="generating || !selectedDate"
        @click="handleGenerate"
      >
        {{ generating ? '触发中...' : '开始生成' }}
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.ngd-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.form-label {
  font-size: 0.75rem;
  color: var(--color-text-secondary);
}

.native-select {
  padding: 0.45rem 0.65rem;
  border-radius: 8px;
  border: 1px solid var(--color-input-border);
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font-size: 0.8rem;
  outline: none;
}

.native-select:focus {
  border-color: var(--color-input-focus);
}

.progress-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.6rem 0.75rem;
  border-radius: 10px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
}

.progress-title {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.progress-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.progress-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.25rem 0;
}

.progress-board {
  font-size: 0.7rem;
  color: var(--color-text-secondary);
}

.progress-status {
  font-size: 0.62rem;
  color: var(--color-text-muted);
}

.progress-status--generating {
  color: #facc15;
}

.progress-status--completed {
  color: #4ade80;
}

.progress-status--failed {
  color: #f87171;
}

.progress-done {
  font-size: 0.68rem;
  color: #4ade80;
  padding-top: 0.25rem;
  border-top: 1px solid var(--color-border-subtle);
}
</style>
