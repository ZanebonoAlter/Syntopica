<script setup lang="ts">
/**
 * 雷达总览报头（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 * 纯展示：eyebrow 日期、大标题与动态副注、活跃/热点/当日线索统计、「全局治理」菜单
 * （对齐已批准原型措辞，原「更多」）与唯一「新建版块」入口。统计与业务文案在首次
 * 取数失败时（statsVisible=false / sub=failed）不得把初始化 0 呈现为业务结论（R-17）；
 * 本次有版块没取到时（sub=partial）热点数与当日线索显示「—」（未知），不显示 0。
 * 治理动作经 advanced 事件交 BoardRadarGovernance 承接，本组件零数据依赖。
 */
import { Icon } from '@iconify/vue'
import type { RadarSubKind } from '~/features/tags/composables/useBoardRadarOverview'

type AdvancedKind = 'upgrade' | 'backfill' | 'matching' | 'merge' | 'labels'

defineProps<{
  dateText: string
  headline: string
  sub: RadarSubKind
  /** 达到热点阈值的版块数；null = 本次有版块没取到（未知，显示「—」）。 */
  hotCount: number | null
  rowsCount: number
  /** 全版块当日线索合计；null = 未知（本次未取到或旧后端缺该字段）。 */
  todayClueTotal: number | null
  /** 首次取数失败时隐藏统计（0/0/0 是未知不是结论）。 */
  statsVisible: boolean
  creatingBoard: boolean
}>()

const emit = defineEmits<{
  (e: 'advanced', kind: AdvancedKind): void
  (e: 'add-board'): void
}>()

function pick(kind: AdvancedKind, event: MouseEvent): void {
  emit('advanced', kind)
  const details = (event.currentTarget as HTMLElement | null)?.closest('details') as HTMLDetailsElement | null
  if (details) details.open = false
}
</script>

<template>
  <header class="rd-issue-head">
    <div class="rd-heading">
      <div class="rd-title-wrap">
        <p class="rd-eyebrow">总览{{ dateText ? ` · ${dateText}` : '' }}</p>
        <h1 class="rd-headline">{{ headline }}</h1>
        <p class="rd-sub">
          <template v-if="sub === 'hot'">
            <b>{{ hotCount ?? '—' }}</b> 个版块达到热点阈值，先扫热度再进版块。
          </template>
          <template v-else-if="sub === 'no-hot'">
            今天没有达到热点阈值的板块，下面按关注度从高到低排。
          </template>
          <template v-else-if="sub === 'empty'">
            今天没有需要看的版块。
          </template>
          <template v-else-if="sub === 'partial'">
            本次数据未取全。
          </template>
          <template v-else>
            总览读取失败，重试后恢复今日雷达。
          </template>
          <span v-if="dateText && sub !== 'failed'" class="rd-asof">数据截至 {{ dateText }}</span>
        </p>
      </div>
    </div>
    <div class="rd-issue-actions">
      <div v-if="statsVisible" class="rd-meta">
        <span>活跃板块 <b>{{ rowsCount }}</b></span>
        <span>热点 <b>{{ hotCount ?? '—' }}</b></span>
        <span v-if="todayClueTotal !== null || sub === 'partial'">今日线索 <b>{{ todayClueTotal ?? '—' }}</b></span>
      </div>
      <NuxtLink to="/" class="rd-home rd-action-btn" aria-label="返回主页" data-testid="radar-home">
        <Icon icon="mdi:home-outline" width="14" />
        <span>主页</span>
      </NuxtLink>
      <details class="rd-more" data-testid="radar-more">
        <summary class="rd-action-btn">全局治理</summary>
        <div class="rd-more-menu">
          <button type="button" data-testid="radar-more-upgrade" @click="pick('upgrade', $event)">扩充版块</button>
          <button type="button" data-testid="radar-more-backfill" @click="pick('backfill', $event)">匹配回填（全库）</button>
          <button type="button" data-testid="radar-more-matching" @click="pick('matching', $event)">匹配规则</button>
          <button type="button" data-testid="radar-more-merge" @click="pick('merge', $event)">标签合并</button>
          <button type="button" data-testid="radar-more-labels" @click="pick('labels', $event)">标签池</button>
        </div>
      </details>
      <AppButton
        variant="primary"
        size="sm"
        data-testid="radar-add-board"
        :disabled="creatingBoard"
        @click="emit('add-board')"
      >
        <Icon icon="mdi:plus" width="14" />
        新建版块
      </AppButton>
    </div>
  </header>
</template>

<style scoped>
.rd-issue-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1.5rem;
  flex: 0 0 auto;
  padding-bottom: 0.9rem;
  border-bottom: 2px solid var(--color-border-medium);
}

.rd-heading {
  align-items: flex-end;
  min-width: 0;
}

/* 操作按钮档：报头右侧的「主页」「全局治理」共用同一套边框/字号/悬停表现，
   避免返回入口自成一体、与操作组视觉脱节（8F / FB-13）。
   .rd-home 目前仅作语义标记（无独立样式），改档位请改 .rd-action-btn。 */
.rd-action-btn {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.3rem;
  min-height: 2rem;
  padding: 0 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  white-space: nowrap;
  text-decoration: none;
}

.rd-action-btn:hover,
.rd-action-btn:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}


.rd-title-wrap {
  min-width: 0;
}

.rd-eyebrow {
  margin: 0 0 0.3rem;
  color: var(--color-accent);
  font-size: 0.66rem;
  letter-spacing: 0.14em;
}

.rd-headline {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: clamp(1.5rem, 2vw, 2.3rem);
  font-weight: 650;
  letter-spacing: -0.01em;
  line-height: 1.3;
}

.rd-sub {
  margin: 0.4rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.76rem;
}

.rd-sub b {
  color: var(--color-accent);
  font-family: var(--font-mono, monospace);
}

.rd-issue-actions {
  display: flex;
  align-items: flex-end;
  gap: 0.85rem;
  flex: 0 0 auto;
}

.rd-meta {
  display: flex;
  gap: 1rem;
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.66rem;
  letter-spacing: 0.06em;
  white-space: nowrap;
}

.rd-meta b {
  margin-left: 0.2rem;
  color: var(--color-text-primary);
  font-family: var(--font-mono, monospace);
  font-size: 0.95rem;
  font-weight: 500;
}

.rd-more {
  position: relative;
}

.rd-more summary {
  cursor: pointer;
  list-style: none;
}

.rd-more summary::-webkit-details-marker {
  display: none;
}

.rd-more[open] summary {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.rd-more-menu {
  position: absolute;
  top: calc(100% + 0.3rem);
  right: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  min-width: 9rem;
  padding: 0.3rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-dialog-bg);
  box-shadow: var(--shadow-strong);
}

.rd-more-menu button {
  padding: 0.4rem 0.55rem;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  text-align: left;
  cursor: pointer;
}

.rd-more-menu button:hover {
  background: var(--color-bg-hover);
  color: var(--color-accent);
}

@media (max-width: 767.98px) {
  .rd-issue-head {
    align-items: flex-start;
    flex-direction: column;
    gap: 0.6rem;
  }

  .rd-heading {
    align-items: flex-start;
    width: 100%;
  }

  .rd-issue-actions {
    align-items: center;
    justify-content: flex-start;
    width: 100%;
    flex-wrap: wrap;
    gap: 0.55rem;
  }

  .rd-meta {
    width: auto;
    justify-content: flex-start;
    flex-wrap: wrap;
    gap: 0.55rem 0.7rem;
  }

  .rd-headline {
    font-size: 1.35rem;
  }
}

/* 极窄屏：主标题平衡换行，避免句尾孤字（如「暂未出现升温」尾字「温」独占一行）。 */
@media (max-width: 400px) {
  .rd-headline {
    text-wrap: balance;
  }
}
</style>
