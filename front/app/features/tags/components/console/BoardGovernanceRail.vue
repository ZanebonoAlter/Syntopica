<script setup lang="ts">
/**
 * 治理右轨（rework-workshop-into-radar-and-desk 3.1/3.3）。
 *
 * 常驻只读信号（一眼判断）：体检评分/趋势/幽灵匹配 → 来源构成 top 3 →
 * 研究 / 追踪 / 治理抽屉入口。需要动手的构成表、组合、候选全量、来源全量、
 * 版块设置收进 `BoardGovernanceDrawer`。
 *
 * 硬约束：
 * - 右轨失败 MUST NOT 阻塞矩阵与版块栏：体检区与来源区各自局部 loading/error/retry；
 * - 无体检快照显示「等待首轮体检」，构成管理入口仍可用；
 * - 来源口径与全量表格同源（`useBoardSources`），右轨只取 top 3，不复制公式。
 */
import { toRef } from 'vue'
import { Icon } from '@iconify/vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'
import type {
  BoardHealthCandidate,
  BoardHealthSnapshotDTO,
} from '~/api/semanticBoards'
import { useBoardSources } from '../../composables/useBoardSources'
import BoardHealthSignal from './BoardHealthSignal.vue'
import type { GovernanceTab } from './governanceTabs'

const props = defineProps<{
  boardId: number
  /** 体检信号（壳层 `useBoardHealth` 持有，避免右轨/抽屉重复取数）。 */
  snapshot: BoardHealthSnapshotDTO | null
  loading: boolean
  error: string | null
  score: number | null
  tier: 'good' | 'mid' | 'low' | null
  checkedAtText: string
  trendPolyline: string
  trendSummary: string
  capturingId: number | null
  watchCount: number
}>()

const emit = defineEmits<{
  retry: []
  capture: [candidate: BoardHealthCandidate]
  openDrawer: [tab: GovernanceTab]
  openWatch: []
  openResearch: []
  openTopics: []
  openAdvanced: [kind: 'upgrade' | 'backfill' | 'matching' | 'merge' | 'labels' | 'add-board']
}>()

const {
  windowDays,
  breakdown,
  loading: sourcesLoading,
  error: sourcesError,
  retry: retrySources,
  sortedSources,
  totalArticles,
  sourceCount,
  shareText,
} = useBoardSources(toRef(props, 'boardId'), { window: 7 })

const topSources = sortedSources
</script>

<template>
  <div class="gvr" data-testid="board-governance-rail">
    <BoardHealthSignal
      :snapshot="snapshot"
      :loading="loading"
      :error="error"
      :score="score"
      :tier="tier"
      :checked-at-text="checkedAtText"
      :trend-polyline="trendPolyline"
      :trend-summary="trendSummary"
      :capturing-id="capturingId"
      :candidate-limit="3"
      show-detail-entry
      @retry="emit('retry')"
      @capture="emit('capture', $event)"
      @open-detail="emit('openDrawer', 'quality')"
    />

    <section class="gvr-block" data-testid="gov-sources">
      <button type="button" class="gvr-block-head" @click="emit('openDrawer', 'sources')">
        <span class="gvr-block-title">
          <Icon icon="mdi:source-branch" width="14" />
          来源构成 · {{ windowDays }} 天
        </span>
        <span class="gvr-link">{{ breakdown ? `全部 ${sourceCount} 个 →` : '查看全部 →' }}</span>
      </button>

      <div v-if="sourcesLoading" class="gvr-hint" data-testid="gov-sources-loading">来源统计加载中…</div>
      <div v-else-if="sourcesError" class="gvr-error" data-testid="gov-sources-error">
        <span>{{ sourcesError }}</span>
        <button type="button" class="gvr-retry" data-testid="gov-sources-retry" @click="retrySources">重试</button>
      </div>
      <div v-else-if="!breakdown || totalArticles === 0" class="gvr-hint">近 7 天没有文章归入本版块，试试切到 30 / 90 天。</div>
      <div v-else class="gvr-source-list" data-testid="gov-sources-top">
        <div
          v-for="source in topSources.slice(0, 3)"
          :key="source.feed_id"
          class="gvr-source"
          :data-testid="`gov-source-row-${source.feed_id}`"
        >
          <div class="gvr-source-line">
            <span class="gvr-source-title" :title="source.title">{{ source.title }}</span>
            <span class="gvr-source-count">{{ source.articles }} 篇</span>
          </div>
          <div class="gvr-bar">
            <span class="gvr-bar-track">
              <i class="gvr-bar-fill" :style="{ width: `${Math.min(100, source.share * 100)}%` }" />
            </span>
            <span class="gvr-bar-text">{{ shareText(source.share) }}</span>
          </div>
        </div>
        <div class="gvr-total">近 7 天 {{ totalArticles }} 篇 · {{ sourceCount }} 个来源</div>
      </div>
    </section>

    <section class="gvr-block" data-testid="gov-entries">
      <AppSectionHeader title="入口" icon-name="mdi:door-open" />
      <button type="button" class="gvr-entry" data-testid="gov-entry-research" @click="emit('openResearch')">
        <span class="gvr-entry-label">研究工作台</span>
        <Icon icon="mdi:chevron-right" width="14" />
      </button>
      <button type="button" class="gvr-entry" data-testid="gov-entry-settings" @click="emit('openDrawer', 'settings')">
        <span class="gvr-entry-label">数据增强设置</span>
        <Icon icon="mdi:chevron-right" width="14" />
      </button>
      <button type="button" class="gvr-entry" data-testid="gov-entry-topics" @click="emit('openTopics')">
        <span class="gvr-entry-label">话题管理（重命名 / 归档 / 删除）</span>
        <Icon icon="mdi:tag-edit-outline" width="14" />
      </button>
      <button type="button" class="gvr-entry" data-testid="gov-entry-watch" @click="emit('openWatch')">
        <span class="gvr-entry-label">我在追踪</span>
        <span class="gvr-entry-value">{{ watchCount }}</span>
      </button>
      <button type="button" class="gvr-entry" data-testid="gov-entry-drawer" @click="emit('openDrawer', 'quality')">
        <span class="gvr-entry-label">治理抽屉（构成 / 组合 / 设置）</span>
        <Icon icon="mdi:chevron-right" width="14" />
      </button>
    </section>

    <section class="gvr-block" data-testid="gov-advanced">
      <AppSectionHeader title="版块治理" icon-name="mdi:tools" />
      <div class="gvr-advanced-grid">
        <button type="button" class="gvr-action" data-testid="gov-entry-add-board" @click="emit('openAdvanced', 'add-board')">
          <Icon icon="mdi:plus-box-outline" width="14" />
          新建版块
        </button>
        <button type="button" class="gvr-action" data-testid="gov-entry-upgrade" @click="emit('openAdvanced', 'upgrade')">
          <Icon icon="mdi:auto-fix" width="14" />
          扩充版块
        </button>
        <button type="button" class="gvr-action" data-testid="gov-entry-backfill" @click="emit('openAdvanced', 'backfill')">
          <Icon icon="mdi:backup-restore" width="14" />
          匹配回填
        </button>
        <button type="button" class="gvr-action" data-testid="gov-entry-matching" @click="emit('openAdvanced', 'matching')">
          <Icon icon="mdi:tune" width="14" />
          匹配规则
        </button>
        <button type="button" class="gvr-action" data-testid="gov-entry-merge" @click="emit('openAdvanced', 'merge')">
          <Icon icon="mdi:call-merge" width="14" />
          标签合并
        </button>
        <button type="button" class="gvr-action" data-testid="gov-entry-labels" @click="emit('openAdvanced', 'labels')">
          <Icon icon="mdi:tag-multiple-outline" width="14" />
          标签池
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.gvr {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  min-height: 0;
}

.gvr-block {
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 0.8rem;
  min-width: 0;
}

.gvr-block-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  width: 100%;
  padding: 0;
  border: none;
  background: none;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.gvr-block-title {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.74rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  min-width: 0;
}

.gvr-link {
  flex: 0 0 auto;
  font-size: 0.68rem;
  color: var(--color-accent);
}

.gvr-hint {
  margin-top: 0.5rem;
  font-size: 0.7rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.gvr-advanced-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.3rem;
  margin-top: 0.55rem;
}

.gvr-action {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  min-height: 1.95rem;
  padding: 0.25rem 0.45rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 5px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.68rem;
  text-align: left;
  cursor: pointer;
}

.gvr-action:hover,
.gvr-action:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}

.gvr-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.5rem;
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.gvr-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.gvr-source-list {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  margin-top: 0.55rem;
}

.gvr-source {
  min-width: 0;
}

.gvr-source-line {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.4rem;
  font-size: 0.72rem;
  line-height: 1.4;
}

.gvr-source-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.gvr-source-count {
  flex: 0 0 auto;
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.gvr-bar {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-top: 0.25rem;
}

.gvr-bar-track {
  flex: 1;
  min-width: 0;
  height: 4px;
  border-radius: 999px;
  background: var(--color-bg-hover);
  overflow: hidden;
}

.gvr-bar-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--color-accent);
}

.gvr-bar-text {
  flex: 0 0 auto;
  width: 2.1rem;
  text-align: right;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.gvr-total {
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.gvr-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  width: 100%;
  padding: 0.42rem 0;
  border: none;
  border-top: 1px solid var(--color-border-subtle);
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
  text-align: left;
}

.gvr-entry:first-of-type {
  border-top: none;
}

.gvr-entry:hover {
  color: var(--color-text-primary);
}

.gvr-entry-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gvr-entry-value {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
  color: var(--color-accent);
}
</style>
