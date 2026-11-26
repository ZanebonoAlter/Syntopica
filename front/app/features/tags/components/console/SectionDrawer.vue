<script setup lang="ts">
/**
 * Section 抽屉（rework-workshop-into-radar-and-desk 2.3）。
 *
 * 点矩阵/演进带的格子打开：展示该泳道当日的分区标题、全部事件、当日日报里的线索，
 * 并提供两个动作（打开当日日报 / 查看归档）；不提供跨泳道写入。
 *
 * - 覆盖式呈现（32rem overlay，不挤压矩阵）；遮罩与 Esc 均可关闭，关闭后矩阵选中态保持。
 * - 「当日日报线索」按 section id 精确联查（lane-dynamics 的 section_id = 日报分区 id），
 *   不按 label 猜；无匹配报告时明确提示。
 * - 只读：不提供归档/转正/合并等生命周期写操作。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDrawer from '~/components/ui/AppDrawer.vue'
import { useDailyReportsApi, type DailyReport, type DailyReportThread } from '~/api/dailyReports'
import type { LaneTimelineSection } from '~/api/laneDynamics'

const props = defineProps<{
  open: boolean
  boardId: number
  laneLabel: string
  date: string
  sections: LaneTimelineSection[]
  windowDays: number
  today: string
}>()

const emit = defineEmits<{
  close: []
  openReport: [payload: { reportId: number, date: string }]
  viewArchive: []
}>()

const api = useDailyReportsApi()

const report = ref<DailyReport | null>(null)
const cluesLoading = ref(false)
const cluesError = ref<string | null>(null)
let requestSeq = 0

const totalSections = computed(() => props.sections.length)
const totalEvents = computed(() => props.sections.reduce((sum, s) => sum + (s.events?.length ?? 0), 0))
const foldedCount = computed(() => props.sections.reduce((sum, s) => sum + (s.folded_count ?? 0), 0))

interface SectionClue {
  sectionId: number
  threads: DailyReportThread[]
}

/** 当日报告里与点击分区对应的 clues（按 section_id 精确匹配）。 */
const clues = computed<SectionClue[]>(() => {
  const detail = report.value
  if (!detail) return []
  const ids = new Set(props.sections.map((s) => s.section_id))
  return (detail.sections ?? [])
    .filter((section) => ids.has(section.id))
    .map((section) => ({ sectionId: section.id, threads: section.threads ?? [] }))
})

async function loadClues(): Promise<void> {
  const seq = ++requestSeq
  cluesLoading.value = true
  cluesError.value = null
  report.value = null
  if (!props.open || !props.date) {
    cluesLoading.value = false
    return
  }
  const list = await api.getBoardDailyReports(props.boardId, { days: Math.max(props.windowDays, 7) })
  if (seq !== requestSeq) return
  if (!list.success || !list.data) {
    cluesError.value = list.error || '日报线索加载失败'
    cluesLoading.value = false
    return
  }
  const matched = (list.data.reports ?? []).find((item) => item.period_date === props.date)
  if (!matched) {
    cluesLoading.value = false
    return
  }
  const detail = await api.getDailyReportDetail(matched.id)
  if (seq !== requestSeq) return
  if (!detail.success || !detail.data) {
    cluesError.value = detail.error || '日报线索加载失败'
  } else {
    report.value = detail.data.report
  }
  cluesLoading.value = false
}

watch(
  () => [props.open, props.date, props.boardId] as const,
  () => { void loadClues() },
  { immediate: true },
)

const canOpenReport = computed(() => report.value !== null)

function openReport(): void {
  if (!report.value) return
  emit('openReport', { reportId: report.value.id, date: props.date })
}

function formatKicker(): string {
  return `${props.date} · ${props.date === props.today ? '今天' : '历史'}`
}
</script>

<template>
  <AppDrawer
    :open="open"
    :label="`${date} ${laneLabel} 分区详情`"
    testid="section-drawer"
    close-testid="section-drawer-close"
    @close="emit('close')"
  >
    <template #head>
      <div class="sd-kicker">{{ formatKicker() }}</div>
      <h2 class="sd-title">{{ sections[0]?.label ?? '这一天没有分区' }}</h2>
      <div class="sd-sub">{{ laneLabel }} · {{ totalSections }} 个分区 / {{ totalEvents }} 条事件</div>
    </template>

    <template #default>
          <template v-if="sections.length">
            <div v-for="section in sections" :key="section.section_id" class="sd-section">
              <div class="sd-section-label">{{ section.label }}</div>
              <div v-if="section.events.length" class="sd-events">
                <div v-for="(event, index) in section.events" :key="index" class="sd-event">
                  <span class="sd-event-date">{{ date.slice(5) }}</span>
                  <span class="sd-event-text">{{ event }}</span>
                </div>
              </div>
              <div v-else class="sd-hint">这一格没有载入事件文案。</div>
              <div v-if="(section.folded_count ?? 0) > 0" class="sd-folded">
                另有 {{ section.folded_count }} 条未载入
              </div>
            </div>
            <div v-if="foldedCount > 0" class="sd-folded-note">当天共 {{ foldedCount }} 条事件被后端截断，未随聚合响应载入。</div>
          </template>
          <div v-else class="sd-hint">这一天这条泳道没有分区。</div>

          <!-- 当日日报线索 -->
          <div class="sd-clues" data-testid="section-drawer-clues">
            <div class="sd-clues-title">当日日报里的线索</div>
            <div v-if="cluesLoading" class="sd-hint">线索加载中…</div>
            <div v-else-if="cluesError" class="sd-clue-error">
              <span>{{ cluesError }}</span>
              <button type="button" class="sd-link" @click="loadClues()">重试</button>
            </div>
            <div v-else-if="!report" class="sd-hint">这一天没有日报。</div>
            <div v-else-if="clues.length === 0" class="sd-hint">日报里没有对应分区线索。</div>
            <div v-else class="sd-clue-list">
              <div v-for="clue in clues" :key="clue.sectionId" class="sd-clue">
                <div v-for="thread in clue.threads" :key="thread.id" class="sd-clue-row">
                  <span class="sd-clue-count">{{ thread.related_article_ids?.length ?? 0 }} 篇</span>
                  <span class="sd-clue-text">{{ thread.title }}</span>
                  <span class="sd-clue-conf">{{ thread.confidence?.toFixed(2) }}</span>
                </div>
              </div>
            </div>
          </div>
    </template>

    <template #footer>
      <div class="sd-actions">
          <button
            type="button"
            class="sd-action sd-action--primary"
            data-testid="section-drawer-open-report"
            :disabled="!canOpenReport"
            :title="canOpenReport ? '打开当日日报' : (cluesError ? '日报线索加载失败，稍后重试' : '这一天没有日报')"
            @click="openReport"
          >
            <Icon icon="mdi:file-document-outline" width="14" />
            打开 {{ date.slice(5) }} 日报
          </button>
          <button
            type="button"
            class="sd-action"
            data-testid="section-drawer-archive"
            title="在归档视图核对本版块文章"
            @click="emit('viewArchive')"
          >
            看归档
          </button>
      </div>
    </template>
  </AppDrawer>
</template>

<style scoped>
.sd-kicker {
  font-size: 0.66rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.sd-title {
  margin: 0.35rem 0 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.1rem;
  line-height: 1.45;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
}

.sd-sub {
  margin-top: 0.3rem;
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.sd-section {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.sd-section-label {
  font-size: 0.9rem;
  line-height: 1.5;
  color: var(--color-text-primary);
  font-weight: 600;
  overflow-wrap: break-word;
}

.sd-events {
  display: flex;
  flex-direction: column;
  gap: 0.28rem;
}

.sd-event {
  display: flex;
  gap: 0.6rem;
  font-size: 0.8rem;
  line-height: 1.55;
  color: var(--color-text-secondary);
}

.sd-event-date {
  flex: 0 0 auto;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  padding-top: 0.18rem;
}

.sd-event-text {
  overflow-wrap: break-word;
}

.sd-folded,
.sd-folded-note {
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.sd-folded-note {
  padding-top: 0.2rem;
  border-top: 1px dashed var(--color-border-subtle);
}

.sd-clues {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding-top: 0.8rem;
  border-top: 1px solid var(--color-border-subtle);
}

.sd-clues-title {
  font-size: 0.66rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.sd-clue-list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.sd-clue {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.sd-clue-row {
  display: flex;
  align-items: baseline;
  gap: 0.55rem;
  font-size: 0.78rem;
  color: var(--color-text-secondary);
}

.sd-clue-count {
  flex: 0 0 auto;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.sd-clue-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: break-word;
}

.sd-clue-conf {
  flex: 0 0 auto;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.sd-clue-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.74rem;
  color: var(--color-error);
}

.sd-link {
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
  padding: 0;
}

.sd-hint {
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.sd-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.sd-action {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.76rem;
  cursor: pointer;
}

.sd-action:hover:not(:disabled) {
  color: var(--color-text-primary);
  border-color: var(--color-border-strong);
}

.sd-action--primary {
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 35%, transparent);
  background: var(--color-accent-subtle);
}

.sd-action:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 767.98px) {
  .sd-panel {
    width: 100vw;
    max-width: 100vw;
    border-left: none;
  }
}
</style>
