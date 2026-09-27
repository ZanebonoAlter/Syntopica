<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { LaneDynamicsLane } from '~/api/laneDynamics'

/**
 * 泳道动态单卡（overview-lane-dynamics ui-design §5 + tasks 3.3）。
 *
 * 构成：泳道名 + watch 角标（--color-info）+ 态势句（as_of 标注 / 待结算占位条）
 * + 发展时间线（垂直：左日期节点 + --border-subtle 连接线，右事件列表；
 * 日期内多 section 分组；单日事件超 5 条折叠「还有 N 条」就地展开；
 * 卡片超高内部滚动）。整卡可点 → emit select（TagsPage 切话题总览 focus）。
 *
 * 只读红线：本卡不含任何生命周期操作按钮（spec「只读语义」）。
 */
const props = defineProps<{
  lane: LaneDynamicsLane
  windowDays: number
}>()

const emit = defineEmits<{
  select: [topicId: number]
}>()

/** 单日事件折叠阈值（ui-design §3：单日事件超 5 条折叠）。 */
const DAY_EVENT_LIMIT = 5

/** 展平后的事件条目（保留所属 section，渲染时按连续同 section 分组）。 */
interface DayEntry {
  sectionId: number
  sectionLabel: string
  event: string
}

/** 单日视图模型：事件序 + 后端截断未载入数。 */
interface DayViewModel {
  date: string
  entries: DayEntry[]
  foldedFromBackend: number
}

/** 连续同 section 的事件分组（时间线渲染形态）。 */
interface SectionGroup {
  sectionId: number
  label: string
  events: string[]
}

const days = computed<DayViewModel[]>(() =>
  (props.lane.timeline ?? []).map(day => ({
    date: day.date,
    entries: (day.sections ?? []).flatMap(s =>
      (s.events ?? []).map(e => ({ sectionId: s.section_id, sectionLabel: s.label, event: e })),
    ),
    foldedFromBackend: (day.sections ?? []).reduce((sum, s) => sum + (s.folded_count ?? 0), 0),
  })),
)

/** 已展开「还有 N 条」的日期（date 在单卡内唯一）。 */
const expandedDays = ref(new Set<string>())

function isCollapsed(day: DayViewModel): boolean {
  return day.entries.length > DAY_EVENT_LIMIT && !expandedDays.value.has(day.date)
}

/** 折叠态只显示前 DAY_EVENT_LIMIT 条。 */
function visibleEntries(day: DayViewModel): DayEntry[] {
  return isCollapsed(day) ? day.entries.slice(0, DAY_EVENT_LIMIT) : day.entries
}

/** 「还有 N 条」计数 = 前端折叠数 + 后端截断数（如实提示被折叠总数，spec）。 */
function hiddenCount(day: DayViewModel): number {
  const clientFolded = isCollapsed(day) ? day.entries.length - DAY_EVENT_LIMIT : 0
  return clientFolded + day.foldedFromBackend
}

/** 连续同 section 归组（切片可能截断分组边界，按连续性重建）。 */
function groupEntries(entries: DayEntry[]): SectionGroup[] {
  const groups: SectionGroup[] = []
  for (const entry of entries) {
    const last = groups[groups.length - 1]
    if (last && last.sectionId === entry.sectionId) {
      last.events.push(entry.event)
    } else {
      groups.push({ sectionId: entry.sectionId, label: entry.sectionLabel, events: [entry.event] })
    }
  }
  return groups
}

function toggleDay(date: string) {
  const next = new Set(expandedDays.value)
  if (next.has(date)) {
    next.delete(date)
  } else {
    next.add(date)
  }
  expandedDays.value = next
}

/** "2026-09-09" → "9/9"（汇总截止小字）。 */
function formatAsOf(date: string): string {
  const parts = date.split('-')
  if (parts.length < 3) return date
  return `${Number(parts[1])}/${Number(parts[2])}`
}

/** "2026-09-08" → "09-08"（时间线日期节点，等宽小字）。 */
function formatDay(date: string): string {
  return date.length >= 10 ? date.slice(5, 10) : date
}

function onSelect() {
  emit('select', props.lane.topic_id)
}
</script>

<template>
  <article
    class="ldc-card"
    role="button"
    tabindex="0"
    data-testid="lane-card"
    @click="onSelect"
    @keydown.enter.self.prevent="onSelect"
    @keydown.space.self.prevent="onSelect"
  >
    <div class="ldc-top">
      <span class="ldc-name">{{ lane.label }}</span>
      <span v-if="lane.watch_linked" class="ldc-watch" data-testid="lane-watch-badge">
        <Icon icon="mdi:eye-outline" width="11" aria-hidden="true" />
        追踪中
      </span>
      <span class="ldc-count">{{ windowDays }}天 · {{ lane.section_count_14d }} section</span>
    </div>

    <!-- 态势句 / 待结算占位（snapshot=null 降级：时间线照常，卡片不置灰） -->
    <template v-if="lane.snapshot">
      <p class="ldc-stance" data-testid="lane-stance">{{ lane.snapshot.summary }}</p>
      <p class="ldc-asof" data-testid="lane-asof">汇总截止 {{ formatAsOf(lane.snapshot.as_of) }}</p>
    </template>
    <div v-else class="ldc-pending" data-testid="lane-pending">
      态势待结算 · 时间线照常展示
    </div>

    <!-- 发展时间线 -->
    <div class="ldc-timeline" data-testid="lane-timeline">
      <div v-for="day in days" :key="day.date" class="ldc-day" :data-testid="`lane-day-${day.date}`">
        <div class="ldc-date">{{ formatDay(day.date) }}</div>
        <div
          v-for="group in groupEntries(visibleEntries(day))"
          :key="group.sectionId"
          class="ldc-section"
        >
          <div class="ldc-section-label">{{ group.label }}</div>
          <div v-for="(ev, i) in group.events" :key="i" class="ldc-event">{{ ev }}</div>
        </div>
        <button
          v-if="isCollapsed(day)"
          type="button"
          class="ldc-more"
          :data-testid="`lane-day-more-${day.date}`"
          @click.stop="toggleDay(day.date)"
          @keydown.stop
        >
          还有 {{ hiddenCount(day) }} 条
        </button>
        <span v-else-if="day.foldedFromBackend > 0" class="ldc-folded-note">
          另有 {{ day.foldedFromBackend }} 条未载入
        </span>
      </div>
    </div>
  </article>
</template>

<style scoped>
.ldc-card {
  display: flex;
  flex-direction: column;
  max-height: 420px;
  padding: 0.85rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-elevated);
  cursor: pointer;
  transition: box-shadow 0.15s ease, border-color 0.15s ease;
}

.ldc-card:hover,
.ldc-card:focus-visible {
  border-color: var(--color-border-medium);
  box-shadow: var(--shadow-medium);
  outline: none;
}

.ldc-top {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  min-width: 0;
}

.ldc-name {
  font-size: 0.92rem;
  font-weight: 700;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* watch 角标（ui-design：--color-info，与普通 active 卡视觉区分） */
.ldc-watch {
  display: inline-flex;
  align-items: center;
  gap: 0.15rem;
  flex-shrink: 0;
  padding: 0.05rem 0.4rem;
  border: 1px solid var(--color-info);
  border-radius: 4px;
  color: var(--color-info);
  font-size: 0.66rem;
  line-height: 1.4;
}

.ldc-count {
  margin-left: auto;
  flex-shrink: 0;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

/* 态势句：每卡第一视觉重心（字号大于时间线） */
.ldc-stance {
  margin: 0.55rem 0 0.2rem;
  font-size: 0.82rem;
  line-height: 1.7;
  color: var(--color-text-primary);
}

.ldc-asof {
  margin: 0 0 0.5rem;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

/* 待结算占位条（--bg-sunken） */
.ldc-pending {
  margin: 0.55rem 0;
  padding: 0.45rem 0.6rem;
  border-radius: 6px;
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  font-size: 0.72rem;
  text-align: center;
}

/* 发展时间线：左连接线 + 日期节点 */
.ldc-timeline {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  border-left: 2px solid var(--color-border-subtle);
  margin-left: 4px;
  padding-left: 0.9rem;
}

.ldc-day {
  position: relative;
  margin-bottom: 0.7rem;
}

/* 日期节点圆点 */
.ldc-day::before {
  content: '';
  position: absolute;
  left: -1.24rem;
  top: 3px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-bg-base);
  border: 2px solid var(--color-border-medium);
}

.ldc-date {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  margin-bottom: 0.15rem;
}

.ldc-section {
  margin-bottom: 0.3rem;
}

.ldc-section-label {
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin-bottom: 0.1rem;
}

.ldc-event {
  position: relative;
  padding-left: 0.6rem;
  font-size: 0.74rem;
  line-height: 1.6;
  color: var(--color-text-secondary);
}

.ldc-event::before {
  content: '·';
  position: absolute;
  left: 0;
  color: var(--color-text-muted);
}

/* 「还有 N 条」折叠（--color-info，就地展开） */
.ldc-more {
  margin-top: 0.15rem;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-info);
  font-size: 0.68rem;
  cursor: pointer;
}

.ldc-more:hover {
  text-decoration: underline;
}

/* 后端截断的如实标注（不可展开） */
.ldc-folded-note {
  display: inline-block;
  margin-top: 0.15rem;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}
</style>
