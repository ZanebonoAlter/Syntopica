<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { SignalCandidateRow, SignalGranularity, SignalReportRow, SignalResearchProgress } from '~/api/boardSignals'
import SignalResearchTrace from './SignalResearchTrace.vue'
import {
  candidateStatusLabel,
  validateSignalPeriodFormat,
} from './signalReport'

/**
 * 信号候选索引（board-signal-reports 5.2 → redesign-enrichment-research-workbench 4.1）。
 *
 * 工作台左栏紧凑索引：周期工具栏（月/年+period）+「发现信号」+ 筛选 tab
 * （全部/未研究/可阅读为本地客户端 status 过滤；已忽略切 visibility='ignored'
 * 由父级重拉）+ 候选行 + 「已加载 N 条」计数。
 *
 * 事件边界（FE-1）：点击「发现信号」只 emit discover——组件不发任何
 * research 请求；忽略/恢复/研究/阅读/选中全部 emit 给父级（useSignalWorkbench
 * 裁决）。派生状态徽标（待研究/研究中/已有报告）来自服务端 status 字段只读；
 * score 是内部阈值分，不渲染为可信度。
 */
const props = withDefaults(defineProps<{
  candidates: SignalCandidateRow[]
  loading: boolean
  error: string | null
  granularity: SignalGranularity
  period: string
  discoveryRunning: boolean
  discoveryPhase: string | null
  discoveryError: string | null
  researchCandidateId: number | null
  researchPhase: string | null
  researchError: string | null
  researchProgressLabel: string
  /** 实时研究进展（进行中每轮刷新；失败/任务失效后保留供回看，tasks 4.8）。 */
  researchProgress: SignalResearchProgress | null
  /** 已生成报告列表（真实计数来源：按 source_signal_id 归到候选行）。 */
  reports?: SignalReportRow[]
  /** 父级候选可见性视图（visible/ignored/all）：已忽略 tab 读取 ignored。 */
  visibility?: 'visible' | 'ignored' | 'all'
}>(), {
  reports: () => [],
  visibility: 'visible',
})

const emit = defineEmits<{
  /** 提交周期切换（输入框 change / 粒度切换）：只重拉列表，不触发发现。 */
  (e: 'commit', payload: { granularity: SignalGranularity; period: string }): void
  /** 点击「发现信号」：父级先对齐周期再发 discovery（FE-1 全链唯一触发点）。 */
  (e: 'discover', payload: { granularity: SignalGranularity; period: string }): void
  /** 深入分析（pending 行显式入口）。 */
  (e: 'research', candidateId: number, options: { regenerate: boolean }): void
  /** 阅读报告（跳详情阅读视图）。 */
  (e: 'open-report', resultId: number): void
  /** 查看候选最近一次研究的全程进展（历史入口，tasks 4.8）。 */
  (e: 'open-progress', candidateId: number): void
  /** 选中候选：父级打开详情（窄屏进入详情单栏）。 */
  (e: 'select', candidateId: number): void
  /** 忽略候选（disposition PUT 由父级 composable 裁决）。 */
  (e: 'ignore', candidateId: number): void
  /** 恢复已忽略候选（不触发研究）。 */
  (e: 'restore', candidateId: number): void
  /** 切换可见性视图（已忽略 tab → ignored，其余 → visible）：父级重拉列表。 */
  (e: 'set-visibility', visibility: 'visible' | 'ignored' | 'all'): void
  /** 筛选 tab（R-2）：view 在服务端分页前生效；visibility 配合已忽略视图。 */
  (e: 'select-view', payload: { visibility: 'visible' | 'ignored' | 'all'; view: 'all' | 'pending' | 'readable' }): void
}>()

// ── 筛选 tab（redesign 4.1）：全部/未研究/可阅读 = 客户端 status 过滤；
// 已忽略 = 服务端 visibility 重拉。 ─────────────────────────────────────────
type IndexTab = 'all' | 'pending' | 'readable' | 'ignored'
const TABS: { id: IndexTab; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'pending', label: '未研究' },
  { id: 'readable', label: '可阅读' },
  { id: 'ignored', label: '已忽略' },
]
const activeTab = ref<IndexTab>('all')

const isIgnoredView = computed(() => activeTab.value === 'ignored')

const visibleRows = computed<SignalCandidateRow[]>(() => props.candidates)

function onSelectTab(tab: IndexTab) {
  activeTab.value = tab
  emit('select-view', {
    visibility: tab === 'ignored' ? 'ignored' : 'visible',
    view: tab === 'pending' ? 'pending' : tab === 'readable' ? 'readable' : 'all',
  })
}

// 父级可见性被外部改回 visible（切板块重置等）时，本地 tab 同步回「全部」，
// 避免「已忽略」tab 高亮却显示 visible 候选的错位。
watch(() => props.visibility, (v) => {
  if (v !== 'ignored' && activeTab.value === 'ignored') activeTab.value = 'all'
})

/** 该候选的报告版本数（真实列表按 source_signal_id 统计，不做总数断言）。 */
function versionCount(candidateId: number): number {
  return props.reports.filter((r) => r.source_signal_id === candidateId).length
}

// 周期输入本地草稿：change（回车/失焦）才提交，避免每键一次请求
const periodDraft = ref(props.period)
watch(
  () => props.period,
  (p) => {
    periodDraft.value = p
  },
)

const periodPlaceholder = computed(() =>
  props.granularity === 'year' ? '如 2026' : '如 2026-09',
)

function onGranularityChange(gran: SignalGranularity) {
  // 换粒度时周期形状随之变化：切到当前月/年（客户端时钟近似，未来周期由
  // 服务端按业务时区拒），连同新周期一起提交
  const now = new Date()
  const nextPeriod =
    gran === 'year'
      ? String(now.getFullYear())
      : `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  periodDraft.value = nextPeriod
  emit('commit', { granularity: gran, period: nextPeriod })
}

/** 行内 trace 可见：进行中（实时数据）或历史查看（无失败态——失败态由失败区渲染防重复）。 */
function rowTraceVisible(rowId: number): boolean {
  if (!props.researchProgress) return false
  if (props.researchError) return false
  if (props.researchCandidateId === rowId) return true
  return props.researchCandidateId === null
    && props.researchProgress.candidate_id === rowId
}

/** 历史入口可见：候选有已落库进展、当前无进行中任务、也没在显示 trace。 */
function historyEntryVisible(row: SignalCandidateRow): boolean {
  return props.researchCandidateId === null
    && !props.researchError
    && !rowTraceVisible(row.id)
    && (row.last_research_progress?.rounds_done ?? 0) > 0
}

function onPeriodChange() {
  const p = periodDraft.value.trim()
  if (!validateSignalPeriodFormat(props.granularity, p)) {
    // 形状非法不提交：保留输入现场，点击「发现信号」时父级会给完整提示
    return
  }
  if (p === props.period) return
  emit('commit', { granularity: props.granularity, period: p })
}

function onDiscover() {
  emit('discover', { granularity: props.granularity, period: periodDraft.value.trim() })
}

const discoveryPhaseLabel = computed(() => {
  switch (props.discoveryPhase) {
    case 'detect':
      return '识别信号中'
    default:
      return '装配周期材料中'
  }
})

function formatDiscoveredAt(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toISOString().slice(0, 10)
}

/** 动作互斥：发现中/研究中期间忽略/恢复/深入研究统一禁用（服务端 409 兜底）。 */
const taskBusy = computed(() => props.discoveryRunning || props.researchCandidateId !== null)

/** 深入分析可点：待研究且无任何任务在跑。 */
function canResearch(row: SignalCandidateRow): boolean {
  return row.status === 'pending' && !taskBusy.value
}

function onResearch(row: SignalCandidateRow) {
  if (!canResearch(row)) return
  emit('research', row.id, { regenerate: false })
}

function onIgnore(row: SignalCandidateRow) {
  if (taskBusy.value) return
  emit('ignore', row.id)
}

function onRestore(row: SignalCandidateRow) {
  if (taskBusy.value) return
  emit('restore', row.id)
}

function onOpenReport(row: SignalCandidateRow) {
  if (row.latest_result_id) emit('open-report', row.latest_result_id)
}
</script>

<template>
  <div class="signal-candidates">
    <!-- 周期工具栏 -->
    <div class="signal-toolbar">
      <label class="signal-field">
        <span>粒度</span>
        <select
          class="ew-select signal-gran"
          aria-label="发现粒度"
          :value="granularity"
          :disabled="discoveryRunning || researchCandidateId !== null"
          @change="onGranularityChange((($event.target as HTMLSelectElement).value) as SignalGranularity)"
        >
          <option value="month">月</option>
          <option value="year">年</option>
        </select>
      </label>
      <label class="signal-field">
        <span>周期</span>
        <input
          v-model="periodDraft"
          class="ew-input signal-period-input"
          :placeholder="periodPlaceholder"
          :aria-label="granularity === 'year' ? '年度周期' : '月度周期'"
          :disabled="discoveryRunning || researchCandidateId !== null"
          @change="onPeriodChange"
        />
      </label>
      <AppButton
        class="signal-discover-btn"
        variant="primary"
        size="sm"
        :disabled="discoveryRunning || researchCandidateId !== null"
        data-testid="signal-discover"
        @click="onDiscover"
      >
        <Icon icon="mdi:radar" width="14" />
        {{ discoveryRunning ? '发现中…' : '发现信号' }}
      </AppButton>
      <span v-if="discoveryRunning" class="signal-progress" role="status" aria-live="polite" data-testid="signal-discovery-progress">
        <Icon icon="mdi:loading" width="13" class="spin" />
        {{ discoveryPhaseLabel }}…只保存候选，不生成报告
      </span>
    </div>

    <!-- 筛选 tab（R-2）：全部/未研究/可阅读/已忽略全部服务端筛选（view 分页前生效 + visibility 忽略标记） -->
    <div class="signal-tabs" role="tablist" aria-label="候选筛选" data-testid="signal-tabs">
      <button
        v-for="t in TABS"
        :key="t.id"
        type="button"
        class="signal-tab"
        :class="{ on: activeTab === t.id }"
        :aria-pressed="activeTab === t.id"
        :data-testid="`signal-tab-${t.id}`"
        @click="onSelectTab(t.id)"
      >
        {{ t.label }}
      </button>
    </div>

    <!-- 发现失败：区别于空态，可重试（FE-4） -->
    <div v-if="discoveryError" class="signal-error" role="alert" data-testid="signal-discovery-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ discoveryError }}</span>
      <span class="muted">可调整周期后重试。</span>
    </div>

    <!-- 研究失败（全局唯一任务）：候选保留可重试；有保留进展时可回看全程 -->
    <div v-if="researchError" class="signal-error" role="alert" data-testid="signal-research-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ researchError }}</span>
    </div>
    <SignalResearchTrace
      v-if="researchError && researchProgress"
      :progress="researchProgress"
      caption="任务已结束，以下为最后保留的进展"
    />

    <!-- 列表加载失败：错误态不伪装无数据（FE-12） -->
    <div v-if="error" class="signal-error" role="alert">
      <Icon icon="mdi:database-alert-outline" width="14" />
      <span>{{ error }}</span>
      <span class="muted">候选列表没有加载成功，并非无数据。</span>
    </div>

    <!-- 候选行索引 -->
    <div v-if="loading" class="signal-empty">
      <Icon icon="mdi:loading" width="14" class="spin" />
      候选加载中…
    </div>
    <div
      v-else-if="visibleRows.length === 0 && !error"
      class="signal-empty"
      data-testid="signal-candidates-empty"
    >
      <template v-if="isIgnoredView">没有已忽略的候选。忽略只是收纳，不影响已有报告。</template>
      <template v-else-if="activeTab === 'all'">本周期还没有候选信号。点击「发现信号」，先看看这个周期哪些问题值得查。发现只保存候选，不会自动生成报告。</template>
      <template v-else>这个周期暂无该状态的线索。</template>
    </div>
    <template v-else>
      <ol class="signal-candidate-list" data-testid="signal-candidate-list">
        <li v-for="(row, idx) in visibleRows" :key="row.id" class="signal-candidate" :data-candidate-id="row.id" :data-candidate-status="row.status">
          <!-- 行主体可点：选中候选 → 父级打开详情（运行中仍可浏览） -->
          <div
            class="signal-candidate-main"
            data-testid="signal-candidate-row"
            @click="emit('select', row.id)"
          >
            <div class="signal-candidate-head">
              <span class="signal-kicker">候选 {{ String(idx + 1).padStart(2, '0') }}</span>
              <span class="signal-status" :data-status="row.status">{{ candidateStatusLabel(row.status) }}</span>
              <span v-if="row.ignored_at" class="signal-ignored-tag">已忽略</span>
              <span v-if="row.status === 'reported'" class="signal-versions-tag" title="该候选下的报告版本数">
                报告 ×{{ versionCount(row.id) }}
              </span>
            </div>
            <h3 class="signal-candidate-title serif">{{ row.signal }}</h3>
            <p class="signal-why">{{ row.why_it_matters }}</p>
            <p class="signal-question">要查的问题：{{ row.research_question }}</p>
            <p class="signal-meta">
              发现于 {{ formatDiscoveredAt(row.discovery_created_at) }} · {{ row.period }}
              <template v-if="row.status === 'reported'"> · 快照不会因等待更新，想用新材料需重新发现</template>
            </p>
            <details v-if="row.evidence_refs.length" class="signal-evidence" @click.stop>
              <summary>新闻依据（{{ row.evidence_refs.length }} 条切片）</summary>
              <ul class="signal-evidence-list">
                <li v-for="refId in row.evidence_refs" :key="refId" class="signal-evidence-chip" title="新闻切片编号（原地展示，不跳转）">
                  新闻切片 #{{ refId }}
                </li>
              </ul>
            </details>
          </div>
          <div class="signal-candidate-actions">
            <AppButton
              v-if="!isIgnoredView && row.status === 'pending'"
              variant="primary"
              size="sm"
              :disabled="!canResearch(row)"
              data-testid="signal-research"
              title="逐条启动，不自动研究全部线索"
              @click.stop="onResearch(row)"
            >
              <Icon v-if="researchCandidateId === row.id" icon="mdi:loading" width="12" class="spin" />
              {{ researchCandidateId === row.id ? (researchPhase === 'compose' ? '成文中…' : '研究中…') : '深入分析' }}
            </AppButton>
            <AppButton
              v-if="row.status === 'reported' && row.latest_result_id"
              variant="primary"
              size="sm"
              data-testid="signal-open-report"
              title="阅读报告（版本切换在详情）"
              @click.stop="onOpenReport(row)"
            >
              阅读
            </AppButton>
            <AppButton
              v-if="!isIgnoredView"
              variant="ghost"
              size="sm"
              :disabled="taskBusy"
              data-testid="signal-ignore"
              :title="taskBusy ? '发现/研究进行中，结束后才能忽略' : '忽略后从默认索引隐藏，可在「已忽略」中恢复'"
              @click.stop="onIgnore(row)"
            >
              忽略
            </AppButton>
            <AppButton
              v-else
              variant="ghost"
              size="sm"
              :disabled="taskBusy"
              data-testid="signal-restore"
              :title="taskBusy ? '发现/研究进行中，结束后才能恢复' : '恢复到默认索引，不触发研究'"
              @click.stop="onRestore(row)"
            >
              恢复
            </AppButton>
            <span v-if="researchCandidateId === row.id" class="signal-progress">{{ researchProgressLabel }}</span>
          </div>
          <SignalResearchTrace
            v-if="rowTraceVisible(row.id)"
            :progress="researchProgress"
            :caption="researchCandidateId === row.id ? undefined : '以下为该候选最近一次研究的进展'"
          />
          <button
            v-else-if="historyEntryVisible(row)"
            type="button"
            class="signal-history-btn"
            data-testid="signal-history-progress"
            @click.stop="emit('open-progress', row.id)"
          >
            <Icon icon="mdi:history" width="13" />
            上次研究 · {{ row.last_research_progress?.rounds_done }} 轮 · 取数 {{ row.last_research_progress?.source_calls }} 次
          </button>
        </li>
      </ol>
      <!-- 已加载计数（当前实际加载条数，不冒充周期总数） -->
      <div class="signal-loaded" data-testid="signal-loaded-count">已加载 {{ visibleRows.length }} 条</div>
    </template>
  </div>
</template>

<style scoped>
.muted { color: var(--color-text-muted); }
.signal-candidates { display: flex; flex-direction: column; gap: 0.75rem; min-width: 0; }
.signal-toolbar { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; }
.signal-field { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.78rem; color: var(--color-text-secondary); }
.signal-gran { min-width: 72px; }
.signal-period-input { width: 110px; padding: 0.3rem 0.55rem; font-size: 0.82rem; border: 1px solid var(--color-input-border); border-radius: 8px; background: var(--color-input-bg); color: var(--color-text-primary); outline: none; }
.signal-period-input:focus { border-color: var(--color-input-focus); }
.signal-discover-btn { margin-left: auto; }
.signal-progress { display: inline-flex; align-items: center; gap: 0.3rem; font-size: 0.75rem; color: var(--color-accent); white-space: nowrap; }
.signal-progress .spin, .spin { animation: sig-spin 1s linear infinite; }
@keyframes sig-spin { to { transform: rotate(360deg); } }

/* 筛选 tab */
.signal-tabs { display: flex; gap: 0.25rem; border-bottom: 1px solid var(--color-border-subtle); }
.signal-tab {
  padding: 0.3rem 0.55rem; border: none; background: none; cursor: pointer;
  font-family: inherit; font-size: 0.75rem; color: var(--color-text-muted);
  border-bottom: 2px solid transparent; white-space: nowrap;
}
.signal-tab:hover { color: var(--color-text-primary); }
.signal-tab.on { color: var(--color-text-primary); font-weight: 600; border-bottom-color: var(--color-accent); }

.signal-error {
  display: flex; align-items: baseline; gap: 0.45rem; flex-wrap: wrap;
  padding: 0.55rem 0.75rem; border-radius: 8px;
  border: 1px solid var(--color-error, var(--color-accent));
  background: var(--color-accent-subtle);
  color: var(--color-error, var(--color-accent));
  font-size: 0.8rem;
}
.signal-error .muted { color: var(--color-text-muted); }

.signal-empty {
  display: flex; align-items: center; gap: 0.4rem;
  padding: 1.4rem 0.5rem; color: var(--color-text-muted); font-size: 0.85rem;
}
.signal-candidate-list { list-style: none; margin: 0; padding: 0; }
.signal-candidate {
  border-top: 1px solid var(--color-border-subtle);
  border-left: 3px solid transparent;
  padding: 0.75rem 0 0.75rem 0.75rem;
  overflow-wrap: anywhere;
}
.signal-candidate[data-status='researching'] { border-left-color: var(--color-accent); }
.signal-candidate-main { cursor: pointer; }
.signal-candidate-main:hover .signal-candidate-title { color: var(--color-accent); }
.signal-candidate-head { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.signal-kicker {
  font-size: 0.7rem; font-weight: 700; letter-spacing: 0.18em;
  color: var(--color-accent);
}
.signal-status {
  font-size: 0.72rem; padding: 0.1rem 0.55rem; border-radius: 999px;
  background: var(--color-bg-sunken); color: var(--color-text-muted); font-weight: 600;
}
.signal-status[data-status='researching'] { color: var(--color-accent); background: var(--color-accent-subtle); }
.signal-status[data-status='reported'] { color: var(--color-success); background: var(--color-success-subtle); }
.signal-ignored-tag {
  font-size: 0.7rem; padding: 0.1rem 0.5rem; border-radius: 999px;
  background: var(--color-bg-sunken); color: var(--color-text-muted);
  border: 1px dashed var(--color-border-medium);
}
.signal-versions-tag { font-size: 0.7rem; color: var(--color-text-muted); white-space: nowrap; }
.signal-candidate-title { font-size: 0.95rem; line-height: 1.5; margin: 0.4rem 0 0.3rem; font-weight: 600; color: var(--color-text-primary); }
.signal-why { margin: 0.2rem 0; font-size: 0.78rem; color: var(--color-text-secondary); line-height: 1.65; }
.signal-question { margin: 0.25rem 0; font-size: 0.78rem; color: var(--color-text-primary); line-height: 1.65; }
.signal-meta { margin: 0.25rem 0 0; font-size: 0.72rem; color: var(--color-text-muted); }
.signal-evidence { margin-top: 0.4rem; }
.signal-evidence summary { cursor: pointer; font-size: 0.75rem; color: var(--color-text-secondary); }
.signal-evidence-list { list-style: none; display: flex; gap: 0.4rem; flex-wrap: wrap; margin: 0.4rem 0 0; padding: 0; }
.signal-evidence-chip {
  font-size: 0.72rem; padding: 0.12rem 0.55rem; border-radius: 999px;
  background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle);
  color: var(--color-text-secondary);
}
.signal-candidate-actions { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.55rem; }
.signal-history-btn {
  display: inline-flex; align-items: center; gap: 0.3rem; margin-top: 0.4rem;
  padding: 0.2rem 0.5rem; border: 1px solid var(--color-border-subtle); border-radius: 6px;
  background: transparent; color: var(--color-text-secondary); font-size: 0.75rem; cursor: pointer;
}
.signal-history-btn:hover { color: var(--color-accent); border-color: var(--color-accent); }
.signal-loaded { font-size: 0.72rem; color: var(--color-text-muted); padding: 0.25rem 0.1rem 0; }

@media (max-width: 720px) {
  .signal-discover-btn { margin-left: 0; min-height: 36px; }
  .signal-period-input { width: 100%; flex: 1 1 auto; }
  .signal-field { flex: 1 1 auto; }
}
</style>
