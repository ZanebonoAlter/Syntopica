<script setup lang="ts">
/**
 * BoardSourcePanel — /tags 板块页「板块内容」tab 底部只读「来源构成」面板
 * （add-source-board-hit-rate §5，位于既有 BoardCompositionPanel 之后，不新增 tab）。
 *
 * 数据：GET /api/semantic-boards/:id/source-breakdown?window=N；选中板块变化或
 * 切窗口时重取（seq 竞态防护：旧请求结果不得覆盖新板块/新窗口）；排序纯前端。
 * 只读硬约定：行内 MUST NOT 出现任何写动作控件（处置入口仍在设置 → 订阅源）。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import FeedSourceQualityPill from '~/features/settings/components/FeedSourceQualityPill.vue'
import type { BoardSourceBreakdown, BoardSourceBreakdownSource } from '~/types'
import { STATS_WINDOWS, formatPercent, type StatsWindowDays } from '~/features/settings/utils/sourceQuality'

const props = defineProps<{
  boardId: number
}>()

const api = useSemanticBoardsApi()

const windowDays = ref<StatsWindowDays>(7)
/** 按篇数 ↓（默认，后端同序）/ 按该源入板块率 ↑（纯前端重排） */
const sortMode = ref<'articles' | 'hit_rate'>('articles')

const breakdown = ref<BoardSourceBreakdown | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

/** 竞态防护：快速切板块/连切窗口时，过期请求结果直接丢弃。 */
let requestSeq = 0

async function load(): Promise<void> {
  const seq = ++requestSeq
  loading.value = true
  error.value = null
  const res = await api.getBoardSourceBreakdown(props.boardId, windowDays.value)
  if (seq !== requestSeq) return // 过期请求：放弃（不覆盖新板块/新窗口的数据）
  if (res.success && res.data) {
    breakdown.value = res.data
  } else {
    error.value = res.error || '统计加载失败'
  }
  loading.value = false
}

watch(() => props.boardId, () => { void load() })
watch(windowDays, () => { void load() })
onMounted(() => { void load() })

function retry(): void {
  void load()
}

const sortedSources = computed<BoardSourceBreakdownSource[]>(() => {
  const sources = [...(breakdown.value?.sources ?? [])]
  if (sortMode.value === 'hit_rate') {
    sources.sort((a, b) =>
      (a.feed_hit_rate - b.feed_hit_rate) || (b.articles - a.articles),
    )
  } else {
    sources.sort((a, b) => b.articles - a.articles)
  }
  return sources
})

/** 最大来源及其占比（固定按篇数取，不随排序切换变化）。 */
const topSource = computed(() => {
  const sources = [...(breakdown.value?.sources ?? [])].sort((a, b) => b.articles - a.articles)
  return sources[0] ?? null
})

function shareText(share: number): string {
  const pct = Math.round(share * 100)
  return share > 0 && pct === 0 ? '<1%' : `${pct}%`
}

function initialOf(title: string): string {
  return title ? title.slice(0, 1) : '?'
}
</script>

<template>
  <section class="bsp-panel" data-testid="board-source-panel">
    <div class="bsp-header">
      <Icon icon="mdi:source-branch" width="15" class="bsp-header-icon" />
      <span class="bsp-title">来源构成</span>
      <div class="bsp-seg" role="group" aria-label="统计窗口">
        <button
          v-for="w in STATS_WINDOWS"
          :key="w"
          type="button"
          class="bsp-seg-btn"
          :class="{ 'bsp-seg-btn--on': windowDays === w }"
          :data-window="w"
          @click="windowDays = w"
        >
          {{ w }} 天
        </button>
      </div>
      <template v-if="breakdown">
        <span class="bsp-summary-item">本板块 <b>{{ breakdown.total_articles }}</b> 篇</span>
        <span class="bsp-summary-item">来源 <b>{{ breakdown.source_count }}</b> 个</span>
        <span v-if="topSource" class="bsp-summary-item bsp-summary-item--top">
          最大来源 <b>{{ topSource.title }}</b>（{{ shareText(topSource.share) }}）
        </span>
      </template>
      <span class="bsp-spacer" />
      <select v-model="sortMode" class="bsp-sort" data-testid="board-source-sort" aria-label="来源表排序">
        <option value="articles">按篇数 ↓</option>
        <option value="hit_rate">按该源入板块率 ↑</option>
      </select>
    </div>

    <!-- 加载：骨架占位（不阻塞上方板块构成面板） -->
    <div v-if="loading" class="bsp-loading" data-testid="board-source-loading">
      <div v-for="i in 3" :key="i" class="bsp-skeleton-row" />
    </div>

    <!-- 失败：块内重试，不影响板块构成与其它 tab -->
    <div v-else-if="error" class="bsp-state" data-testid="board-source-error">
      <Icon icon="mdi:alert-circle-outline" width="16" height="16" />
      <span>统计加载失败{{ error ? `（${error}）` : '' }}</span>
      <button type="button" class="bsp-retry" data-testid="board-source-retry" @click="retry">重试</button>
    </div>

    <!-- 空态：引导切窗口 -->
    <div v-else-if="!breakdown || breakdown.total_articles === 0" class="bsp-state bsp-state--empty" data-testid="board-source-empty">
      <span>近 {{ windowDays }} 天没有文章归入本板块</span>
      <span class="bsp-state-hint">试试切到 30 天或 90 天窗口</span>
    </div>

    <!-- 来源表（只读，行内无任何写动作）；sources 空但总数 > 0（理论不可能）→ 如实提示并保留总数 -->
    <template v-else>
      <div v-if="sortedSources.length === 0" class="bsp-state" data-testid="board-source-unavailable">
        <span>来源解析不可用</span>
        <span class="bsp-state-hint">本板块 {{ breakdown.total_articles }} 篇，但未能解析来源分布</span>
      </div>
      <table v-else class="bsp-table">
        <thead>
          <tr>
            <th style="width: 34%">订阅源</th>
            <th style="width: 12%" class="bsp-num">本板块篇数</th>
            <th style="width: 22%">占本板块</th>
            <th style="width: 16%">该源入板块率</th>
            <th style="width: 16%" class="bsp-num bsp-col-total">该源窗口内总量</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="source in sortedSources" :key="source.feed_id" data-testid="board-source-row">
            <td>
              <div class="bsp-source">
                <span class="bsp-source-icon">{{ initialOf(source.title) }}</span>
                <span class="bsp-source-title">{{ source.title }}</span>
              </div>
            </td>
            <td class="bsp-num">{{ source.articles }}</td>
            <td>
              <div class="bsp-share">
                <span class="bsp-share-track">
                  <i class="bsp-share-fill" :style="{ width: `${Math.min(100, source.share * 100)}%` }" />
                </span>
                <span class="bsp-share-text">{{ shareText(source.share) }}</span>
              </div>
            </td>
            <td>
              <FeedSourceQualityPill
                :articles="source.feed_articles"
                :hit-rate="source.feed_hit_rate"
              />
            </td>
            <td class="bsp-num bsp-col-total">{{ source.feed_articles }}</td>
          </tr>
        </tbody>
      </table>

      <p class="bsp-footnote">
        口径：窗口内文章（按 coalesce(pub_date, created_at) 取 7/30/90 天，含已归档）中至少 1 个标签挂到启用板块的篇数；
        本板块内按文章去重，同一篇文章命中多个板块时在每个板块各计一次（故各来源篇数合计 = 本板块总数）。<br>
        只读面板：处置噪声源请到 设置 → 订阅源（调低最大文章数 / 关闭打标 / 退订）。
      </p>
    </template>
  </section>
</template>

<style scoped>
.bsp-panel {
  margin-top: 1rem;
  padding: 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-elevated);
}

.bsp-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
}

.bsp-header-icon {
  color: var(--color-text-muted);
}

.bsp-title {
  font-weight: 600;
  color: var(--color-text-primary);
}

.bsp-seg {
  display: inline-flex;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  overflow: hidden;
}

.bsp-seg-btn {
  background: none;
  border: none;
  font-size: 11px;
  padding: 4px 9px;
  color: var(--color-text-secondary);
  cursor: pointer;
  white-space: nowrap;
}

.bsp-seg-btn + .bsp-seg-btn {
  border-left: 1px solid var(--color-border-subtle);
}

.bsp-seg-btn--on {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-weight: 600;
}

.bsp-summary-item {
  font-variant-numeric: tabular-nums;
}

.bsp-summary-item b {
  font-size: 0.8rem;
  color: var(--color-text-primary);
}

.bsp-summary-item--top b {
  font-size: 0.75rem;
}

.bsp-spacer {
  flex: 1;
}

.bsp-sort {
  font-size: 11px;
  color: var(--color-text-secondary);
  background: var(--color-bg-base);
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  padding: 4px 6px;
  cursor: pointer;
}

.bsp-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.bsp-table th {
  text-align: left;
  font-weight: 500;
  color: var(--color-text-muted);
  font-size: 11px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-border-medium);
}

.bsp-table td {
  padding: 8px;
  border-bottom: 1px solid var(--color-border-subtle);
  vertical-align: middle;
  color: var(--color-text-primary);
}

.bsp-num {
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.bsp-source {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.bsp-source-icon {
  width: 20px;
  height: 20px;
  border-radius: 5px;
  background: var(--color-bg-sunken);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  color: var(--color-text-muted);
  flex: none;
}

.bsp-source-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bsp-share {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bsp-share-track {
  width: 110px;
  height: 6px;
  background: var(--color-bg-sunken);
  border-radius: 3px;
  overflow: hidden;
  flex: none;
}

.bsp-share-fill {
  display: block;
  height: 100%;
  background: var(--color-tag-keyword);
}

.bsp-share-text {
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

/* 窄屏次要列：隐藏「该源窗口内总量」（ui-design Layout Contract） */
@media (max-width: 1024px) {
  .bsp-col-total {
    display: none;
  }
}

.bsp-state {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 12px 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.bsp-state--empty {
  justify-content: center;
  padding: 32px 0;
  flex-direction: column;
}

.bsp-state-hint {
  font-size: 11px;
  color: var(--color-text-muted);
}

.bsp-retry {
  padding: 3px 10px;
  font-size: 11px;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  background: var(--color-bg-base);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bsp-retry:hover {
  background: var(--color-bg-hover);
}

.bsp-loading {
  padding: 8px 0;
}

.bsp-skeleton-row {
  height: 14px;
  border-radius: 4px;
  background: var(--color-bg-sunken);
  margin: 8px 0;
}

.bsp-skeleton-row:nth-child(2) {
  width: 85%;
}

.bsp-skeleton-row:nth-child(3) {
  width: 70%;
}

.bsp-footnote {
  margin: 12px 0 0;
  font-size: 10px;
  color: var(--color-text-muted);
  line-height: 1.7;
}
</style>
