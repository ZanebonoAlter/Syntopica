<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { SignalCandidateRow, SignalGranularity, SignalResearchProgress } from '~/api/boardSignals'
import SignalResearchTrace from './SignalResearchTrace.vue'
import {
  candidateStatusLabel,
  validateSignalPeriodFormat,
} from './signalReport'

/**
 * 信号候选列表（board-signal-reports 5.2，FE-1~5）。
 *
 * 工作台主视图上半部：周期工具栏（月/年+period）+「发现信号」+ 候选行列表。
 * 候选字段全部来自服务端 detect 校验后输出；派生状态徽标（待研究/研究中/
 * 已有报告）服务端只读；score 是内部阈值分，不渲染为可信度。
 *
 * 事件边界（FE-1）：点击「发现信号」只 emit discover——组件不发任何
 * research 请求；研究必须逐条点「深入分析」。发现结束即停，由父级
 * composable 决定轮询与重拉。
 */
const props = defineProps<{
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
}>()

const emit = defineEmits<{
  /** 提交周期切换（输入框 change / 粒度切换）：只重拉列表，不触发发现。 */
  (e: 'commit', payload: { granularity: SignalGranularity; period: string }): void
  /** 点击「发现信号」：父级先对齐周期再发 discovery（FE-1 全链唯一触发点）。 */
  (e: 'discover', payload: { granularity: SignalGranularity; period: string }): void
  /** 深入分析/重新研究（regenerate=true 仅来自显式入口）。 */
  (e: 'research', candidateId: number, options: { regenerate: boolean }): void
  /** 阅读报告（跳详情阅读视图）。 */
  (e: 'open-report', resultId: number): void
  /** 查看候选最近一次研究的全程进展（历史入口，tasks 4.8）。 */
  (e: 'open-progress', candidateId: number): void
}>()

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

/** 深入分析可点：待研究且无任何任务在跑（发现中/研究中期间统一禁用，服务端 409 兜底）。 */
function canResearch(row: SignalCandidateRow): boolean {
  return (
    row.status === 'pending' &&
    !props.discoveryRunning &&
    props.researchCandidateId === null
  )
}

function onResearch(row: SignalCandidateRow) {
  if (!canResearch(row)) return
  emit('research', row.id, { regenerate: false })
}

/** 显式重新研究（reported 行的次级入口）：regenerate=true 由父级二次确认。 */
function onRegenerate(row: SignalCandidateRow) {
  if (props.discoveryRunning || props.researchCandidateId !== null) return
  emit('research', row.id, { regenerate: true })
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

    <!-- 候选行列表 -->
    <div v-if="loading" class="signal-empty">
      <Icon icon="mdi:loading" width="14" class="spin" />
      候选加载中…
    </div>
    <div
      v-else-if="candidates.length === 0 && !error"
      class="signal-empty"
      data-testid="signal-candidates-empty"
    >
      还没有候选信号。点击「发现信号」，先看看这个周期哪些问题值得查。发现只保存候选，不会自动生成报告。
    </div>
    <ol v-else class="signal-candidate-list" data-testid="signal-candidate-list">
      <li v-for="(row, idx) in candidates" :key="row.id" class="signal-candidate" :data-candidate-id="row.id" :data-candidate-status="row.status">
        <div class="signal-candidate-head">
          <span class="signal-kicker">候选 {{ String(idx + 1).padStart(2, '0') }}</span>
          <span class="signal-status" :data-status="row.status">{{ candidateStatusLabel(row.status) }}</span>
        </div>
        <h3 class="signal-candidate-title serif">{{ row.signal }}</h3>
        <p class="signal-why">{{ row.why_it_matters }}</p>
        <p class="signal-question">要查的问题：{{ row.research_question }}</p>
        <p class="signal-meta">
          发现于 {{ formatDiscoveredAt(row.discovery_created_at) }} · {{ row.period }}
          <template v-if="row.status === 'reported'"> · 快照不会因等待更新，想用新材料需重新发现</template>
        </p>
        <details v-if="row.evidence_refs.length" class="signal-evidence">
          <summary>新闻依据（{{ row.evidence_refs.length }} 条切片）</summary>
          <ul class="signal-evidence-list">
            <li v-for="refId in row.evidence_refs" :key="refId" class="signal-evidence-chip" title="新闻切片编号（原地展示，不跳转）">
              新闻切片 #{{ refId }}
            </li>
          </ul>
        </details>
        <div class="signal-candidate-actions">
          <AppButton
            v-if="row.status === 'reported'"
            variant="primary"
            size="sm"
            data-testid="signal-open-report"
            @click="onOpenReport(row)"
          >
            阅读报告
          </AppButton>
          <AppButton
            v-else
            variant="primary"
            size="sm"
            :disabled="!canResearch(row)"
            data-testid="signal-research"
            @click="onResearch(row)"
          >
            <Icon v-if="researchCandidateId === row.id" icon="mdi:loading" width="12" class="spin" />
            {{ researchCandidateId === row.id ? (researchPhase === 'compose' ? '成文中…' : '研究中…') : '深入分析' }}
          </AppButton>
          <AppButton
            v-if="row.status === 'reported'"
            variant="ghost"
            size="sm"
            data-testid="signal-regenerate"
            :disabled="discoveryRunning || researchCandidateId !== null"
            title="重新研究：新任务会重新取数与计算，可能消耗预算"
            @click="onRegenerate(row)"
          >
            重新研究
          </AppButton>
          <span v-if="researchCandidateId === row.id" class="signal-progress">{{ researchProgressLabel }}</span>
          <span v-else-if="row.status === 'pending'" class="signal-meta">尚未取数</span>
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
          @click="emit('open-progress', row.id)"
        >
          <Icon icon="mdi:history" width="13" />
          上次研究 · {{ row.last_research_progress?.rounds_done }} 轮 · 取数 {{ row.last_research_progress?.source_calls }} 次
        </button>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.muted { color: var(--color-text-muted); }
.signal-candidates { display: flex; flex-direction: column; gap: 0.75rem; }
.signal-toolbar { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; }
.signal-field { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.78rem; color: var(--color-text-secondary); }
.signal-gran { min-width: 72px; }
.signal-period-input { width: 110px; padding: 0.3rem 0.55rem; font-size: 0.82rem; border: 1px solid var(--color-input-border); border-radius: 8px; background: var(--color-input-bg); color: var(--color-text-primary); outline: none; }
.signal-period-input:focus { border-color: var(--color-input-focus); }
.signal-discover-btn { margin-left: auto; }
.signal-progress { display: inline-flex; align-items: center; gap: 0.3rem; font-size: 0.75rem; color: var(--color-accent); white-space: nowrap; }
.signal-history-btn {
  display: inline-flex; align-items: center; gap: 0.3rem;
  padding: 0.2rem 0.5rem; border: 1px solid var(--color-border-subtle); border-radius: 6px;
  background: transparent; color: var(--color-text-secondary); font-size: 0.75rem; cursor: pointer;
}
.signal-history-btn:hover { color: var(--color-accent); border-color: var(--color-accent); }
.signal-progress .spin, .spin { animation: sig-spin 1s linear infinite; }
@keyframes sig-spin { to { transform: rotate(360deg); } }

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
  padding: 1rem 0 1rem 0.75rem;
  overflow-wrap: anywhere;
}
.signal-candidate + .signal-candidate { border-top: 1px solid var(--color-border-subtle); }
.signal-candidate[data-status='researching'] { border-left-color: var(--color-accent); }
.signal-candidate-head { display: flex; align-items: center; gap: 0.6rem; }
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
.signal-candidate-title { font-size: 1.15rem; line-height: 1.5; margin: 0.45rem 0 0.35rem; font-weight: 600; }
.signal-why { margin: 0.25rem 0; font-size: 0.85rem; color: var(--color-text-secondary); line-height: 1.7; }
.signal-question { margin: 0.3rem 0; font-size: 0.85rem; color: var(--color-text-primary); line-height: 1.7; }
.signal-meta { margin: 0.3rem 0 0; font-size: 0.75rem; color: var(--color-text-muted); }
.signal-evidence { margin-top: 0.5rem; }
.signal-evidence summary { cursor: pointer; font-size: 0.78rem; color: var(--color-text-secondary); }
.signal-evidence-list { list-style: none; display: flex; gap: 0.4rem; flex-wrap: wrap; margin: 0.45rem 0 0; padding: 0; }
.signal-evidence-chip {
  font-size: 0.72rem; padding: 0.12rem 0.55rem; border-radius: 999px;
  background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle);
  color: var(--color-text-secondary);
}
.signal-candidate-actions { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.7rem; }

@media (max-width: 720px) {
  .signal-discover-btn { margin-left: 0; min-height: 36px; }
  .signal-period-input { width: 100%; flex: 1 1 auto; }
  .signal-field { flex: 1 1 auto; }
}
</style>
