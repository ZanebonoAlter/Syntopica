<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { SignalReportRow } from '~/api/boardSignals'
import { isRetrospectiveReport } from './signalReport'

/**
 * 已生成报告列表（board-signal-reports 5.3，FE-6/FE-9）。
 *
 * 按周期列出成功报告：标题（判断式）、目标周期、生成时间、真实取数次数
 * （从 generation_meta.source_calls 读——真实执行记录，不信模型自报）。
 * 历史研究标「事后回顾 · 本次数据版本」（analysis_mode=retrospective）。
 * 无评审/采纳操作：生成即可阅读。
 */
const props = defineProps<{
  reports: SignalReportRow[]
  loading: boolean
  error: string | null
}>()

const emit = defineEmits<{
  (e: 'open', resultId: number): void
}>()

interface ReportRowView {
  id: number
  title: string
  period: string
  createdAt: string
  sourceCalls: number
  calculationCalls: number
  decisions: number
  retrospective: boolean
}

function toView(row: SignalReportRow): ReportRowView {
  const meta = row.sectors?.generation_meta
  return {
    id: row.id,
    title: row.sectors?.report?.title || `信号解读报告 #${row.id}`,
    period: row.period,
    createdAt: row.created_at ? row.created_at.slice(0, 10) : '—',
    sourceCalls: meta?.source_calls ?? 0,
    calculationCalls: meta?.calculation_calls ?? 0,
    decisions: meta?.decisions ?? 0,
    retrospective: row.sectors ? isRetrospectiveReport(row.sectors) : false,
  }
}

const rows = computed<ReportRowView[]>(() => (props.reports ?? []).map(toView))
</script>

<template>
  <div class="signal-reports" data-testid="signal-report-list">
    <div class="signal-reports-head">
      <h3 class="signal-reports-title serif">已生成报告</h3>
      <span class="signal-reports-hint">生成后即可阅读，无后续评审步骤</span>
    </div>

    <div v-if="loading" class="signal-reports-empty">
      <Icon icon="mdi:loading" width="14" class="spin" />
      报告列表加载中…
    </div>
    <div v-else-if="error" class="signal-error" role="alert">
      <Icon icon="mdi:database-alert-outline" width="14" />
      <span>{{ error }}</span>
      <span class="muted">报告列表没有加载成功，并非没有报告。</span>
    </div>
    <div v-else-if="rows.length === 0" class="signal-reports-empty" data-testid="signal-reports-empty">
      你选择信号并完成研究后，报告会出现在这里。
    </div>
    <ul v-else class="signal-report-rows">
      <li v-for="row in rows" :key="row.id" class="signal-report-row" :data-report-id="row.id">
        <button type="button" class="signal-report-link" @click="emit('open', row.id)">
          <span class="signal-report-row-title serif">{{ row.title }}</span>
          <span class="signal-report-row-meta">
            目标周期 {{ row.period }} · 生成于 {{ row.createdAt }} ·
            真实取数 {{ row.sourceCalls }} 次 · 计算 {{ row.calculationCalls }} 次 · 决策 {{ row.decisions }} 轮
          </span>
          <span v-if="row.retrospective" class="signal-retro-tag">事后回顾 · 本次数据版本</span>
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.signal-reports { display: flex; flex-direction: column; gap: 0.5rem; }
.signal-reports-head { display: flex; align-items: baseline; gap: 0.6rem; flex-wrap: wrap; }
.signal-reports-title { font-size: 0.95rem; margin: 0; font-weight: 700; }
.signal-reports-hint { font-size: 0.75rem; color: var(--color-text-muted); }
.signal-reports-empty { padding: 0.9rem 0.5rem; color: var(--color-text-muted); font-size: 0.85rem; display: flex; align-items: center; gap: 0.4rem; }
.signal-report-rows { list-style: none; margin: 0; padding: 0; }
.signal-report-row { border-top: 1px solid var(--color-border-subtle); border-left: 3px solid var(--color-accent); }
.signal-report-link {
  display: flex; flex-direction: column; gap: 0.25rem; width: 100%;
  padding: 0.7rem 0.75rem; border: none; background: transparent;
  text-align: left; cursor: pointer; font-family: inherit;
  color: var(--color-text-primary); overflow-wrap: anywhere;
}
.signal-report-link:hover { background: var(--color-accent-subtle); }
.signal-report-row-title { font-size: 1rem; font-weight: 600; line-height: 1.5; }
.signal-report-row-meta { font-size: 0.75rem; color: var(--color-text-muted); }
.signal-retro-tag {
  align-self: flex-start;
  font-size: 0.7rem; font-weight: 600; padding: 0.08rem 0.5rem; border-radius: 999px;
  color: var(--color-warning); background: var(--color-warning-subtle);
}
.signal-error {
  display: flex; align-items: baseline; gap: 0.45rem; flex-wrap: wrap;
  padding: 0.55rem 0.75rem; border-radius: 8px;
  border: 1px solid var(--color-error, var(--color-accent));
  background: var(--color-accent-subtle);
  color: var(--color-error, var(--color-accent));
  font-size: 0.8rem;
}
.signal-error .muted { color: var(--color-text-muted); }
.spin { animation: sig-reports-spin 1s linear infinite; }
@keyframes sig-reports-spin { to { transform: rotate(360deg); } }
</style>
