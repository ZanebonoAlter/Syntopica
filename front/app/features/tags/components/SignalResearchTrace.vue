<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { SignalResearchProgress } from '~/api/boardSignals'

/**
 * 研究进展回看（board-signal-reports tasks 4.8，ui-design「研究进度」契约）。
 *
 * 展示服务端 research-progress 的真实计数与全量账本：每步取数（工具/问题/
 * 状态/观测数）、代码计算、缺口。数据全部来自服务端每轮滚动落库的
 * board_signal_research_progress（与报告 appendix 同源结构）——如实呈现，
 * 不美化失败、不用模型自报计数。
 *
 * 使用位置：研究进行中（实时刷新）与研究失败/任务失效后（回看已落库进展）。
 * 报告成功后的每步链路由 SignalReportView 的 appendix 展示，不在本组件职责内。
 */
const props = defineProps<{
  progress: SignalResearchProgress | null
  /** 场景注记：进行中不传；终态回看可注明「最后保留的进展」。 */
  caption?: string
}>()

const summary = computed(() => {
  const p = props.progress
  if (!p) return null
  return {
    rounds: p.rounds_done,
    calls: p.source_calls,
    calcs: p.calculation_calls,
    updatedAt: formatTime(p.updated_at),
  }
})

const calls = computed(() => props.progress?.ledger?.calls ?? [])
const calculations = computed(() => props.progress?.ledger?.calculations ?? [])
const gaps = computed(() => props.progress?.ledger?.gaps ?? [])

const ledgerEmpty = computed(
  () => calls.value.length === 0 && calculations.value.length === 0 && gaps.value.length === 0,
)

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function callStatusIcon(status: string): string {
  return status === 'ok' ? 'mdi:check-circle-outline' : 'mdi:alert-circle-outline'
}
</script>

<template>
  <div v-if="progress" class="signal-trace" data-testid="signal-research-trace">
    <p class="signal-trace-summary">
      <template v-if="summary">
        已进行 {{ summary.rounds }}/40 轮 · 取数 {{ summary.calls }} · 计算 {{ summary.calcs }}
        · 更新于 {{ summary.updatedAt }}
      </template>
      <template v-if="caption">
        <span class="muted">（{{ caption }}）</span>
      </template>
    </p>
    <details class="signal-trace-details">
      <summary>研究过程（每步调用明细）</summary>
      <p v-if="ledgerEmpty" class="signal-trace-empty">
        还没有调用记录。进展在研究每轮结束后落库一次，轮内进行中的调用要等本轮收账。
      </p>
      <template v-else>
        <ul v-if="calls.length" class="signal-trace-list">
          <li v-for="call in calls" :key="call.call_id" class="signal-trace-item" :data-status="call.status">
            <Icon :icon="callStatusIcon(call.status)" width="13" />
            <span class="signal-trace-id">{{ call.call_id }}</span>
            <span class="signal-trace-tool">{{ call.tool }}</span>
            <span class="signal-trace-question">{{ call.question }}</span>
            <span class="signal-trace-meta">{{ call.observations?.length ?? 0 }} 条观测</span>
            <span v-if="call.error" class="signal-trace-error">{{ call.error }}</span>
          </li>
        </ul>
        <ul v-if="calculations.length" class="signal-trace-list">
          <li v-for="calc in calculations" :key="calc.calc_id" class="signal-trace-item" :data-status="calc.status">
            <Icon icon="mdi:calculator-variant-outline" width="13" />
            <span class="signal-trace-id">{{ calc.calc_id }}</span>
            <span class="signal-trace-tool">{{ calc.op }}</span>
            <span class="signal-trace-question mono">{{ calc.expression }}</span>
            <span class="signal-trace-meta">
              {{ calc.status === 'ok' ? `${calc.value ?? ''} ${calc.unit ?? ''}`.trim() : calc.status }}
            </span>
          </li>
        </ul>
        <ul v-if="gaps.length" class="signal-trace-list">
          <li v-for="(gap, i) in gaps" :key="i" class="signal-trace-item" data-status="gap">
            <Icon icon="mdi:map-marker-question-outline" width="13" />
            <span class="signal-trace-question">{{ gap.reason }}</span>
          </li>
        </ul>
      </template>
    </details>
  </div>
</template>

<style scoped>
.muted { color: var(--color-text-muted); }
.signal-trace { margin-top: 0.35rem; font-size: 0.78rem; }
.signal-trace-summary { margin: 0; color: var(--color-text-secondary); }
.signal-trace-details summary {
  cursor: pointer; color: var(--color-accent); font-size: 0.78rem;
  user-select: none; margin-top: 0.15rem;
}
.signal-trace-empty { margin: 0.4rem 0 0; color: var(--color-text-muted); }
.signal-trace-list { list-style: none; margin: 0.45rem 0 0; padding: 0; display: flex; flex-direction: column; gap: 0.3rem; }
.signal-trace-item {
  display: flex; align-items: baseline; gap: 0.45rem; flex-wrap: wrap;
  padding: 0.3rem 0.5rem; border-left: 2px solid var(--color-border-subtle);
}
.signal-trace-item[data-status='error'], .signal-trace-item[data-status='rejected'], .signal-trace-item[data-status='missing'] { border-left-color: var(--color-error, var(--color-accent)); }
.signal-trace-id { font-family: var(--font-mono, monospace); font-size: 0.72rem; color: var(--color-text-muted); }
.signal-trace-tool { font-size: 0.72rem; color: var(--color-text-secondary); }
.signal-trace-question { flex: 1 1 auto; min-width: 12rem; }
.signal-trace-question.mono { font-family: var(--font-mono, monospace); font-size: 0.74rem; }
.signal-trace-meta { font-size: 0.72rem; color: var(--color-text-muted); white-space: nowrap; }
.signal-trace-error { width: 100%; font-size: 0.72rem; color: var(--color-error, var(--color-accent)); }
</style>
