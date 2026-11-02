<script setup lang="ts">
/**
 * 选中泳道演进带（rework-workshop-into-radar-and-desk 矩阵阶段 G3，批准版面）。
 *
 * 组合层：状态分层（loading/首错/空态/stale）+ 头部口径 + 态势面板（LaneStancePanel：
 * 最新快照 asOf 非本视窗总结、默认折叠）+ 短窗逐日节点（LaneEvolutionNodes，点径 =
 * min(24, 6+√日合计×5)）/ 长窗 G2 趋势图 + 折叠逐日索引（LaneEvolutionDayIndex）。
 *
 * 受控契约：selectedDate 由父持有（与 G4 编年共用），选日只 emit selectDate、
 * 不抢焦点/不回顶、不另养 selectedDate。G2 图只对已知值日发 select-date
 * （含 gap 的已知 0），本层守 content 门槛（gap 日点击只提示不 emit）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import LaneTrendChart from './LaneTrendChart.vue'
import LaneStancePanel from './LaneStancePanel.vue'
import LaneEvolutionNodes from './LaneEvolutionNodes.vue'
import LaneEvolutionDayIndex from './LaneEvolutionDayIndex.vue'
import type { LaneTrendPoint } from './laneTrendSeries'
import {
  EVOLUTION_NODE_MAX_DAYS,
  evolutionSeriesValue,
  projectEvolutionNodes,
  type LaneDayResolver,
  type ObservationLaneRow,
} from './observationLanes'

const props = withDefaults(defineProps<{
  lane: ObservationLaneRow | null
  /** 可见观察轴（升序；>EVOLUTION_NODE_MAX_DAYS 切趋势图形态）。 */
  axis: string[]
  today: string
  /** 受控选中日（父持有；节点/图点高亮用）。 */
  selectedDate?: string | null
  resolveDay: LaneDayResolver
  loading?: boolean
  error?: string | null
  staleNotice?: boolean
  /** source 档天数（meta 行口径说明；0 = 不展示档说明）。 */
  sourceDays?: number
}>(), {
  selectedDate: null,
  loading: false,
  error: null,
  staleNotice: false,
  sourceDays: 0,
})

const emit = defineEmits<{
  /** 选日（content 日；父更新 selectedDate 与下方阅读，不强滚）。 */
  selectDate: [date: string]
  retry: []
}>()

const { warn: notifyWarn } = useNotify()

/** 逐日节点投影（resolveDay 在此被追踪：池加载完成自动重投影；短窗节点与长窗索引共用）。 */
const nodes = computed(() => {
  if (!props.lane) return []
  return projectEvolutionNodes(props.lane.topicId, props.axis, props.resolveDay, props.today)
})

const longWindow = computed(() => props.axis.length > EVOLUTION_NODE_MAX_DAYS)

/** 趋势图 series：content=已知合计（全未知 null）、gap=已知 0、unavailable=null（不补 0）。 */
const series = computed<LaneTrendPoint[]>(() => {
  if (!props.lane) return []
  const topicId = props.lane.topicId
  return props.axis.map(date => ({ date, value: evolutionSeriesValue(props.resolveDay(topicId, date)) }))
})

const todayTotal = computed(() => nodes.value.find(node => node.isToday)?.total ?? null)

/** G2 图对已知值日（含 gap 的已知 0）都会发 select-date，此处按三态再过滤。 */
function onChartSelect(date: string): void {
  if (!props.lane) return
  const resolution = props.resolveDay(props.lane.topicId, date)
  if (resolution.kind === 'content') {
    emit('selectDate', date)
    return
  }
  if (resolution.kind === 'gap') notifyWarn(`${date} 这一天「${props.lane.label}」没有分区`)
  else notifyWarn(`${date} 该日历史数据未取到`)
}
</script>

<template>
  <section class="oev" data-testid="lane-evolution">
    <div v-if="!lane && loading" class="oev-state" data-testid="lane-evolution-loading">
      <div class="oev-skeleton-line" />
      <div class="oev-skeleton-line is-short" />
      <div class="oev-skeleton-ribbon" />
    </div>
    <div v-else-if="!lane && error" class="oev-state oev-state--error" data-testid="lane-evolution-error">
      <Icon icon="mdi:alert-circle-outline" width="15" />
      <span>演进带加载失败{{ error ? `（${error}）` : '' }}</span>
      <button type="button" class="oev-retry" data-testid="lane-evolution-retry" @click="emit('retry')">重试</button>
    </div>
    <div v-else-if="!lane" class="oev-empty" data-testid="lane-evolution-empty">
      选一条泳道，看它怎么走到今天。
    </div>

    <template v-else>
      <div v-if="staleNotice || error" class="oev-stale" data-testid="lane-evolution-stale">
        刷新失败，当前展示上次数据
      </div>
      <header class="oev-head">
        <span class="oev-name" :title="lane.label">{{ lane.label }}</span>
        <span v-if="lane.watchLinked" class="oev-watch">追踪</span>
        <span class="oev-meta">
          可见 {{ axis.length }} 日<span v-if="sourceDays"> · {{ sourceDays }} 日档全窗 {{ lane.clueTotal }} 条线索</span> · 今日 {{ todayTotal ?? '—' }} 条
        </span>
      </header>

      <!-- 态势句：最新滚动快照逐字（asOf 非本视窗总结，默认折叠）；无快照 = 待结算占位 -->
      <LaneStancePanel :snapshot="lane.snapshot" />

      <div v-if="axis.length === 0" class="oev-empty">观察范围未就绪。</div>

      <!-- 短窗（≤31 日）：逐日节点带 -->
      <LaneEvolutionNodes
        v-else-if="!longWindow"
        :nodes="nodes"
        :selected-date="selectedDate"
        @select-date="date => emit('selectDate', date)"
      />

      <!-- 长窗（>31 日）：G2 趋势图 + 折叠逐日索引 -->
      <template v-else>
        <LaneTrendChart
          :series="series"
          :selected-date="selectedDate"
          label="所选泳道 · 所示日期内日合计（非矩阵桶峰值）"
          @select-date="onChartSelect"
        />
        <LaneEvolutionDayIndex :nodes="nodes" @select-date="date => emit('selectDate', date)" />
      </template>
    </template>
  </section>
</template>

<style scoped>
.oev {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 0.9rem 1rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-bg-elevated) 70%, transparent);
}

.oev-state {
  padding: 0.9rem 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.oev-state--error {
  flex-direction: row;
  align-items: center;
  gap: 0.5rem;
}

.oev-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}

.oev-skeleton-line {
  height: 0.9rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
}

.oev-skeleton-line.is-short {
  width: 45%;
}

.oev-skeleton-ribbon {
  height: 2.2rem;
  border-radius: 8px;
  background: var(--color-bg-hover);
  margin-top: 0.3rem;
}

.oev-stale {
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  background: var(--color-warning-subtle, var(--color-bg-hover));
  color: var(--color-text-secondary);
  font-size: 0.7rem;
}

.oev-empty {
  padding: 0.6rem 0.2rem;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.oev-head {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}

.oev-name {
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.05rem;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.oev-watch {
  font-size: 0.6rem;
  color: var(--color-info);
  border: 1px solid color-mix(in srgb, var(--color-info) 30%, transparent);
  border-radius: 2px;
  padding: 0 0.22rem;
}

.oev-meta {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
</style>
