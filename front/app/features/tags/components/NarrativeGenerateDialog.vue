<script setup lang="ts">
import { ref, computed } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type SemanticBoard } from '~/api/semanticBoards'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useDailyReportProgress } from '~/composables/useDailyReportProgress'

const props = defineProps<{
  visible: boolean
  boards: SemanticBoard[]
}>()

const emit = defineEmits<{
  cancel: []
}>()

const { generateDailyReport } = useDailyReportsApi()
const { progress, done, totalSaved, totalBoards, reset } = useDailyReportProgress()

const selectedDate = ref(new Date().toISOString().slice(0, 10))
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
  <Teleport to="body">
    <div v-if="visible" class="ngd-overlay" @click.self="handleClose">
      <div class="ngd-dialog">
        <header class="ngd-header">
          <h3 class="ngd-title">生成日报</h3>
          <button type="button" class="ngd-close" @click="handleClose">
            <Icon icon="mdi:close" width="16" />
          </button>
        </header>

        <div class="ngd-body">
          <label class="ngd-label">
            日期
            <input type="date" v-model="selectedDate" class="ngd-input" />
          </label>

          <label class="ngd-label">
            板块
            <select v-model="selectedBoardId" class="ngd-input">
              <option :value="null">全部板块</option>
              <option v-for="board in boards" :key="board.id" :value="board.id">{{ board.label }}</option>
            </select>
          </label>

          <!-- Progress board -->
          <div v-if="jobId || generating" class="ngd-progress">
            <div class="ngd-progress-title">
              <Icon icon="mdi:progress-clock" width="14" class="text-white/40" />
              <span>{{ done ? '生成完成' : '生成中...' }}</span>
            </div>
            <div v-if="progressEntries.length" class="ngd-progress-list">
              <div
                v-for="entry in progressEntries"
                :key="entry.board_id"
                class="ngd-progress-row"
              >
                <span class="ngd-progress-board">{{ entry.board_name }}</span>
                <span
                  class="ngd-progress-status"
                  :class="{
                    'ngd-progress-status--waiting': entry.status === 'waiting',
                    'ngd-progress-status--generating': entry.status === 'generating',
                    'ngd-progress-status--completed': entry.status === 'completed',
                    'ngd-progress-status--failed': entry.status === 'failed',
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
            <div v-if="done" class="ngd-progress-done">
              共生成 {{ totalSaved }} 篇日报 / {{ totalBoards }} 个板块
            </div>
          </div>

          <div class="ngd-actions">
            <button type="button" class="ngd-btn ngd-btn--cancel" @click="handleClose">
              关闭
            </button>
            <button
              type="button"
              class="ngd-btn ngd-btn--confirm"
              :disabled="generating || !selectedDate"
              @click="handleGenerate"
            >
              {{ generating ? '触发中...' : '开始生成' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.ngd-overlay {
  position: fixed;
  inset: 0;
  z-index: 70;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(8px);
}

.ngd-dialog {
  width: 400px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(20, 24, 32, 0.98);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

.ngd-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.ngd-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.ngd-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
}

.ngd-close:hover {
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.7);
}

.ngd-body {
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.ngd-label {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.5);
}

.ngd-input {
  padding: 0.45rem 0.65rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.8);
  font-size: 0.8rem;
}

.ngd-input:focus {
  outline: none;
  border-color: rgba(240, 138, 75, 0.4);
}

.ngd-input option {
  background: #1a1f2a;
  color: rgba(255, 255, 255, 0.85);
}

.ngd-progress {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.6rem 0.75rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
}

.ngd-progress-title {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.55);
}

.ngd-progress-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.ngd-progress-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.25rem 0;
}

.ngd-progress-board {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.65);
}

.ngd-progress-status {
  font-size: 0.62rem;
  color: rgba(255, 255, 255, 0.35);
}

.ngd-progress-status--waiting {
  color: rgba(255, 255, 255, 0.35);
}

.ngd-progress-status--generating {
  color: rgba(250, 204, 21, 0.8);
}

.ngd-progress-status--completed {
  color: rgba(74, 222, 128, 0.8);
}

.ngd-progress-status--failed {
  color: rgba(248, 113, 113, 0.8);
}

.ngd-progress-done {
  font-size: 0.68rem;
  color: rgba(74, 222, 128, 0.7);
  padding-top: 0.25rem;
  border-top: 1px solid rgba(255, 255, 255, 0.04);
}

.ngd-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 0.5rem;
}

.ngd-btn {
  padding: 0.45rem 1rem;
  border-radius: 10px;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.ngd-btn--cancel {
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  color: rgba(255, 255, 255, 0.5);
}

.ngd-btn--cancel:hover {
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.7);
}

.ngd-btn--confirm {
  border: 1px solid rgba(240, 138, 75, 0.3);
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 220, 200, 0.85);
}

.ngd-btn--confirm:hover {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.5);
}

.ngd-btn--confirm:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
