<script setup lang="ts">
/**
 * FeedSourceQualityBlock — 订阅源详情只读「来源质量」块（add-source-board-hit-rate §4.4）。
 *
 * 位置契约：状态条之后、设置表单之前（先给观测结论，再往下调参数）。
 * 内容：窗口分段控件（与列表工具栏共享同一状态，spec 要求两处同步）、
 * 四分解堆叠条 + 图例（含篇数）、入板块率（分子/分母）、命中板块分布 chips（Top 6 + 其他 N）。
 * 只读：块内不承载任何写操作（配置仍在既有设置表单）。
 * 状态机见 ui-design State Matrix：loading 骨架 / empty / 样本不足 / 打标关闭 / error 重试 / ready。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { FeedBoardHitStats, RssFeed } from '~/types'
import {
  MIN_SAMPLE_ARTICLES,
  STATS_WINDOWS,
  formatRateOneDecimal,
  resolvePillState,
  type StatsWindowDays,
} from '../utils/sourceQuality'

const props = withDefaults(defineProps<{
  feed: RssFeed
  /** 该源当前窗口统计；undefined = 无数据 */
  stats?: FeedBoardHitStats
  loading?: boolean
  error?: string | null
  windowDays?: StatsWindowDays
}>(), {
  stats: undefined,
  loading: false,
  error: null,
  windowDays: 7,
})

const emit = defineEmits<{
  'set-window': [days: StatsWindowDays]
  retry: []
}>()

type BlockState = 'error' | 'loading' | 'off' | 'empty' | 'low-sample' | 'ready'

const state = computed<BlockState>(() => {
  if (props.error) return 'error'
  if (props.loading || !props.stats) return 'loading'
  // 打标关闭：以统计里的 tagging_enabled 为准（后端事实），缺省回落到 feed 配置
  if (!(props.stats.tagging_enabled ?? props.feed.taggingEnabled ?? true)) return 'off'
  if (props.stats.articles === 0) return 'empty'
  if (props.stats.articles < MIN_SAMPLE_ARTICLES) return 'low-sample'
  return 'ready'
})

/** 堆叠条四段（宽度 = 篇数 / 窗口总篇数；分母为 0 时整条置空）。 */
const segments = computed(() => {
  const s = props.stats
  if (!s || s.articles === 0) return []
  const pct = (count: number) => (count / s.articles) * 100
  return [
    { key: 'in_board', label: '入板块', count: s.in_board, color: 'var(--color-success)', width: pct(s.in_board) },
    { key: 'tagged_no_board', label: '有标签无板块', count: s.tagged_no_board, color: 'var(--color-warning)', width: pct(s.tagged_no_board) },
    { key: 'untagged_pending', label: '打标排队中', count: s.untagged_pending, color: 'var(--color-border-strong)', width: pct(s.untagged_pending) },
    { key: 'untagged_settled', label: '已处理无标签', count: s.untagged_settled, color: 'var(--color-bg-sunken)', width: pct(s.untagged_settled) },
  ]
})

/** 命中板块 chips：篇数降序取 Top 6，其余合并为「其他 N 个板块」。 */
const boardChips = computed(() => {
  const boards = [...(props.stats?.boards ?? [])].sort((a, b) => b.articles - a.articles)
  return {
    top: boards.slice(0, 6),
    otherCount: Math.max(0, boards.length - 6),
  }
})

const ratePercent = computed(() => {
  const s = props.stats
  if (!s || s.articles === 0) return ''
  return formatRateOneDecimal(s.hit_rate)
})

const rateFraction = computed(() => {
  const s = props.stats
  if (!s || s.articles === 0) return ''
  return `（${s.in_board} / ${s.articles}）`
})

/** 率数字三档着色（原型：高绿/低黄/中中性）；分档逻辑复用 pill 同一状态机。 */
const rateTier = computed(() => {
  const s = props.stats
  if (!s || s.articles === 0) return 'mid'
  return resolvePillState({
    articles: s.articles,
    hit_rate: s.hit_rate,
    tagging_enabled: s.tagging_enabled,
  })
})
</script>

<template>
  <section class="fsq-block" data-testid="feed-source-quality-block">
    <!-- 头部：标题 + 口径 + 窗口分段（与列表共享状态） -->
    <div class="fsq-head">
      <span class="fsq-title">来源质量</span>
      <span class="fsq-sub">窗口内文章命中板块的比例（含已归档）</span>
      <div class="fsq-seg" role="group" aria-label="统计窗口">
        <button
          v-for="w in STATS_WINDOWS"
          :key="w"
          type="button"
          class="fsq-seg-btn"
          :class="{ 'fsq-seg-btn--on': windowDays === w }"
          :data-window="w"
          @click="emit('set-window', w)"
        >
          {{ w }} 天
        </button>
      </div>
    </div>

    <!-- 错误：块内重试，不影响下方设置表单 -->
    <div v-if="state === 'error'" class="fsq-state" data-testid="fsq-error">
      <Icon icon="mdi:alert-circle-outline" width="16" height="16" />
      <span>统计加载失败{{ error ? `（${error}）` : '' }}</span>
      <button type="button" class="fsq-retry" data-testid="fsq-retry" @click="emit('retry')">重试</button>
    </div>

    <!-- 加载：骨架条 + 图例置灰 -->
    <div v-else-if="state === 'loading'" class="fsq-state" data-testid="fsq-loading">
      <div class="fsq-skeleton-bar" />
      <span class="fsq-skeleton-text">统计加载中…</span>
    </div>

    <!-- 打标关闭：明示不参与板块归属，不显示 0% 率 -->
    <div v-else-if="state === 'off'" class="fsq-state" data-testid="fsq-off">
      <Icon icon="mdi:tag-off-outline" width="16" height="16" />
      <span>该源已关闭打标，不参与板块归属</span>
    </div>

    <!-- 窗口内 0 篇 -->
    <div v-else-if="state === 'empty'" class="fsq-state" data-testid="fsq-empty">
      <span>近 {{ windowDays }} 天没有新文章</span>
    </div>

    <!-- 样本不足：<5 篇不给百分比，避免小样本误判 -->
    <div v-else-if="state === 'low-sample'" class="fsq-state" data-testid="fsq-low-sample">
      <span>样本不足（{{ stats?.articles }} 篇）</span>
      <span class="fsq-state-hint">入板块率需 ≥ {{ MIN_SAMPLE_ARTICLES }} 篇样本才展示，避免小样本误判</span>
    </div>

    <!-- 就绪：堆叠条 + 图例 + 率 + 板块 chips -->
    <template v-else>
      <div class="fsq-stackbar" role="img" aria-label="窗口内文章四分解堆叠条">
        <i
          v-for="seg in segments"
          :key="seg.key"
          class="fsq-stackbar-seg"
          :data-segment="seg.key"
          :style="{ width: `${seg.width}%`, backgroundColor: seg.color }"
        />
      </div>
      <div class="fsq-legend">
        <span v-for="seg in segments" :key="seg.key" class="fsq-legend-item">
          <i class="fsq-legend-swatch" :style="{ backgroundColor: seg.color }" />
          {{ seg.label }} {{ seg.count }}
        </span>
      </div>
      <p class="fsq-rate">
        入板块率
        <b
          class="fsq-rate-value"
          :class="{
            'fsq-rate-value--high': rateTier === 'high',
            'fsq-rate-value--low': rateTier === 'low',
          }"
        >{{ ratePercent }}</b>
        <span class="fsq-rate-fraction">{{ rateFraction }}</span>
      </p>
      <div v-if="boardChips.top.length > 0" class="fsq-boards">
        <div class="fsq-boards-title">命中板块分布（Top 6，同一文章命中多板块时各计一次）</div>
        <div class="fsq-boards-chips">
          <span v-for="b in boardChips.top" :key="b.board_id" class="fsq-bchip">
            {{ b.label }} <b>{{ b.articles }}</b>
          </span>
          <span v-if="boardChips.otherCount > 0" class="fsq-bchip fsq-bchip--more">
            其他 {{ boardChips.otherCount }} 个板块
          </span>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.fsq-block {
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-base);
  padding: 12px 14px;
}

.fsq-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}

.fsq-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.fsq-sub {
  font-size: 10px;
  color: var(--color-text-muted);
}

.fsq-seg {
  display: inline-flex;
  margin-left: auto;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  overflow: hidden;
  flex: none;
}

.fsq-seg-btn {
  background: none;
  border: none;
  font-size: 11px;
  padding: 4px 9px;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.fsq-seg-btn + .fsq-seg-btn {
  border-left: 1px solid var(--color-border-subtle);
}

.fsq-seg-btn--on {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-weight: 600;
}

/* 非就绪态：单行提示 */
.fsq-state {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 10px 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.fsq-state-hint {
  font-size: 11px;
  color: var(--color-text-muted);
}

.fsq-retry {
  padding: 3px 10px;
  font-size: 11px;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.fsq-retry:hover {
  background: var(--color-bg-hover);
}

.fsq-skeleton-bar {
  width: 100%;
  height: 10px;
  border-radius: 5px;
  background: var(--color-bg-sunken);
}

.fsq-skeleton-text {
  font-size: 11px;
  color: var(--color-text-muted);
}

/* 堆叠条 */
.fsq-stackbar {
  display: flex;
  height: 10px;
  border-radius: 5px;
  overflow: hidden;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-sunken);
}

.fsq-stackbar-seg {
  display: block;
  height: 100%;
}

.fsq-legend {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 8px;
  font-size: 10px;
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.fsq-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.fsq-legend-swatch {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  display: inline-block;
}

.fsq-rate {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.fsq-rate-value {
  font-size: 15px;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
  margin-left: 2px;
}

.fsq-rate-value--high {
  color: var(--color-success);
}

.fsq-rate-value--low {
  color: var(--color-warning);
}

.fsq-rate-fraction {
  font-size: 11px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

/* 板块分布 chips */
.fsq-boards {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed var(--color-border-subtle);
}

.fsq-boards-title {
  font-size: 10px;
  color: var(--color-text-muted);
  margin-bottom: 6px;
}

.fsq-boards-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.fsq-bchip {
  font-size: 10px;
  padding: 3px 8px;
  border-radius: 10px;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.fsq-bchip b {
  font-weight: 600;
  color: var(--color-text-primary);
}

.fsq-bchip--more {
  color: var(--color-text-muted);
}
</style>
