<script setup lang="ts">
/**
 * 右轨「本板块上下文」（rework-workshop-into-radar-and-desk 矩阵阶段 G5，获批 scope-cleanup；
 * console-rails-live-signals 重排区块顺序并新增「当日日报」）。
 *
 * 常驻顺序（结论 → 信号 → 我的关注 → 属性）：**版块体检 → 当日日报 → 话题追踪 → 构成候选 → 来源构成**。
 * - 体检 = 一行状态条（真实评分/时间/刷新）；评语按需进详情（右轨不常驻评语首句）。
 * - 当日日报 = 跟随矩阵选中日（唯一日期源；本轨无任何日期选择器）的两层：当日点评 + 各泳道一句；
 *   同块内另有「当日其他话题」子块（add-other-topics-to-context-rail，≥1440px 才显示：
 *   与泳道层分块分单位——泳道层「条」、子块「条线索」）。
 * - 候选 top3 真实名称与清单数；watch 名称 + active/paused；来源贡献条 + 总量。
 *   依据与全量列表在单级宽工作面（emit open*）按需查看，不在此塞满。
 * - 三类不同实体分列，不互相冒充：构成候选（辅助标签，经体检/构成面板收编，人工确认）、
 *   话题候选（lane-dynamics 候选，人工激活，不自动转正）、话题追踪（watch）。
 * - 追踪管理作用域只有 `boardId`（WatchManagePanel 既有语义）；关注记录的
 *   persistentTopicId 仅用于「定位对应泳道」，不是管理过滤条件。
 * - 全库治理六项/新建版块不在本轨（唯一入口在总览 BoardRadarGovernance）。
 * - 右轨任一区块失败只局部降级（各自 loading/error/retry），不阻塞矩阵与版块栏。
 *
 * 本组件无网络、无自有筛选状态：health/watch/candidates/now 由壳层注入，来源卡自带 sourcestats 读链。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { RailHealthSnapshotView } from '~/api/normalizers/consoleRail'
import type { TopicWatch } from '~/api/topicWatches'
import type { OtherTopicsView } from './dayOtherTopics'
import type { NowBriefModel } from './nowBrief'
import RailHealthBrief from './RailHealthBrief.vue'
import RailNowBrief from './RailNowBrief.vue'
import RailOtherTopicsBrief from './RailOtherTopicsBrief.vue'
import RailSourcesBrief from './RailSourcesBrief.vue'

const props = withDefaults(defineProps<{
  boardId: number
  /** 体检信号（壳层 useBoardHealth 持有；stale=刷新失败保留上次成功快照）。 */
  healthSnapshot: RailHealthSnapshotView | null
  healthLoading: boolean
  healthError: string | null
  healthStale?: boolean
  healthScore: number | null
  healthTier: 'good' | 'mid' | 'low' | null
  healthCheckedAtText: string
  healthTrendPolyline: string
  healthTrendSummary: string
  /** 话题追踪（真实 topic-watches，纯板块作用域）。 */
  watches: TopicWatch[]
  watchesLoading: boolean
  watchesError: string | null
  /** A 名单 topicId（仅用于把关注记录的 persistentTopicId 定位到当前泳道）。 */
  laneIds: readonly string[]
  /** 话题候选数（lane-dynamics 返回的候选；人工激活入口接本板块话题管理）。 */
  topicCandidateCount: number
  /** 「当日日报」视图模型（跟随矩阵选中日；壳层用既有读取链组装，本组件不发请求）。 */
  now: NowBriefModel
  /**
   * 「当日其他话题」组视图模型（矩阵同源派生；唯一实现点 `dayOtherTopics.ts`，
   * 本组件只消费，不重算排除口径与计数）。
   */
  otherTopics: OtherTopicsView
  /** rail=右轨常驻；full=单级宽工作面（候选/watch/来源均全量）。 */
  variant?: 'rail' | 'full'
}>(), {
  healthStale: false,
  variant: 'rail',
})

const emit = defineEmits<{
  retryHealth: []
  openHealth: []
  openCandidates: []
  openSources: []
  openWatchManage: []
  retryWatches: []
  locateLane: [topicId: string]
  openTopics: []
  /** 点当日点评（可定位时）→ 跳到该期日报正文的对应聚簇。 */
  openReport: [payload: { date: string, clusterId: number | null }]
  /** 当日日报详情读取失败 → 受控重试（只重取当日详情，不重建索引）。 */
  retryNow: [date: string]
  /**
   * 点「当日其他话题」某行 → 选中该分区（主区既有阅读槽承接；右轨不改选择态）。
   * `date` = 唯一日期源（矩阵选中日，即本块所跟的当日日报日）。
   */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
}>()

const candidateLimit = computed(() => (props.variant === 'full' ? Number.MAX_SAFE_INTEGER : 3))

const candidates = computed(() => {
  const list = props.healthSnapshot?.candidates ?? []
  return list.slice(0, candidateLimit.value)
})

const candidateTotal = computed(() => props.healthSnapshot?.candidates.length ?? null)

const laneIdSet = computed(() => new Set(props.laneIds))

/** 关注记录 → 当前名单内可定位泳道（只认真实 persistentTopicId，不按名字猜）。 */
function locateTarget(watch: TopicWatch): string | null {
  const id = watch.persistentTopicId
  if (!id) return null
  return laneIdSet.value.has(id) ? id : null
}
</script>

<template>
  <div class="bcr" :class="{ 'is-rail': variant === 'rail' }" data-testid="board-context-rail">
    <!-- 1. 版块体检（结论常驻，完整依据按需） -->
    <section class="bcr-block" data-testid="rail-block-health">
      <RailHealthBrief
        :snapshot="healthSnapshot"
        :loading="healthLoading"
        :error="healthError"
        :stale="healthStale"
        :score="healthScore"
        :tier="healthTier"
        :checked-at-text="healthCheckedAtText"
        :trend-polyline="healthTrendPolyline"
        :trend-summary="healthTrendSummary"
        variant="brief"
        @retry="emit('retryHealth')"
        @open-detail="emit('openHealth')"
      />
    </section>

    <!-- 2. 当日日报（跟随矩阵选中日；右轨无日期选择器） -->
    <section class="bcr-block bcr-block--grow" data-testid="rail-block-now">
      <RailNowBrief
        :model="now"
        @locate-lane="emit('locateLane', $event)"
        @open-report="emit('openReport', $event)"
        @retry="emit('retryNow', now.date ?? '')"
      />
      <!-- 「当日其他话题」子块（add-other-topics-to-context-rail）：≥1440px 才显示，
           填该宽度段「当日日报」块底部留白；1200–1439.98px 右轨随页面流动，不出该子块。 -->
      <div class="bcr-other" data-testid="rail-other-topics-block">
        <RailOtherTopicsBrief
          :view="otherTopics"
          @select-other="emit('selectOther', { ...$event, date: now.date ?? '' })"
          @retry="emit('retryNow', now.date ?? '')"
        />
      </div>
    </section>

    <!-- 3. 话题追踪（boardId 作用域）+ 话题候选（人工激活）分列 -->
    <section class="bcr-block" data-testid="rail-block-watch">
      <div class="bcr-head">
        <span class="bcr-title">
          <Icon icon="mdi:eye-outline" width="14" />
          话题追踪
        </span>
        <button
          type="button"
          class="bcr-manage"
          data-testid="rail-watch-manage"
          title="按当前版块管理关注记录（不按所选泳道过滤）"
          @click="emit('openWatchManage')"
        >管理 ↗</button>
      </div>

      <div v-if="watchesLoading" class="bcr-hint" data-testid="rail-watch-loading">关注记录加载中…</div>
      <div v-else-if="watchesError" class="bcr-error" data-testid="rail-watch-error">
        <Icon icon="mdi:alert-circle-outline" width="13" />
        <span>{{ watchesError }}</span>
        <button type="button" class="bcr-link" data-testid="rail-watch-retry" @click="emit('retryWatches')">重试</button>
      </div>
      <p v-else-if="watches.length === 0" class="bcr-hint" data-testid="rail-watch-empty">暂无关注标记。</p>
      <ul v-else class="bcr-watches" data-testid="rail-watches">
        <li
          v-for="watch in watches"
          :key="watch.id"
          class="bcr-watch"
          :class="{ 'is-paused': watch.status === 'paused' }"
          :data-testid="`rail-watch-${watch.id}`"
        >
          <button
            v-if="locateTarget(watch)"
            type="button"
            class="bcr-watch-label bcr-watch-label--link"
            :data-testid="`rail-watch-locate-${watch.id}`"
            :title="`定位到泳道「${watch.label}」`"
            @click="emit('locateLane', locateTarget(watch)!)"
          >{{ watch.label }}</button>
          <span v-else class="bcr-watch-label" :title="watch.label">{{ watch.label }}</span>
          <span class="bcr-watch-state">{{ watch.status === 'paused' ? '已暂停' : '关注中' }}</span>
        </li>
      </ul>

      <button type="button" class="bcr-entry" data-testid="rail-topic-candidates" @click="emit('openTopics')">
        话题候选 · {{ topicCandidateCount }}
        <Icon icon="mdi:chevron-right" width="13" />
      </button>
    </section>

    <!-- 4. 构成候选（辅助标签；高频共现 ≠ 必须收编） -->
    <section class="bcr-block" data-testid="rail-block-candidates">
      <div class="bcr-head">
        <span class="bcr-title">
          <Icon icon="mdi:tag-plus-outline" width="14" />
          构成候选
        </span>
        <span class="bcr-count" data-testid="rail-candidates-count">
          {{ candidateTotal === null ? '—' : candidateTotal }}
        </span>
      </div>

      <p v-if="candidateTotal === null" class="bcr-hint" data-testid="rail-candidates-pending">
        等待体检快照后显示——不补造候选。
      </p>
      <p v-else-if="candidateTotal === 0" class="bcr-hint" data-testid="rail-candidates-empty">
        暂无未收编构成候选。
      </p>
      <ul v-else class="bcr-cands" data-testid="rail-candidates">
        <li
          v-for="candidate in candidates"
          :key="candidate.auxId"
          class="bcr-cand"
          :data-testid="`rail-candidate-${candidate.auxId}`"
        >
          <span class="bcr-cand-label" :title="candidate.label">{{ candidate.label }}</span>
          <span class="bcr-cand-meta">{{ candidate.memberTags }} tags · {{ candidate.memberArticles }} 篇</span>
        </li>
      </ul>
      <button type="button" class="bcr-entry" data-testid="rail-candidates-detail" @click="emit('openCandidates')">
        查看候选
        <Icon icon="mdi:chevron-right" width="13" />
      </button>
    </section>

    <!-- 5. 来源构成（独立滚动统计窗，不随观察日期重算） -->
    <section class="bcr-block" data-testid="rail-block-sources">
      <RailSourcesBrief
        :board-id="boardId"
        :variant="variant === 'full' ? 'full' : 'brief'"
        @open-detail="emit('openSources')"
      />
    </section>
  </div>
</template>

<style scoped>
.bcr {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  min-width: 0;
}

/* 满高栏体（design D9b）：高度 = 视口高减顶栏 + 纵向滚动 + 极淡底色（栏读作「面」）。 */
@media (min-width: 1440px) {
  .bcr.is-rail {
    flex: 1 1 auto;
    min-height: 0;
    height: 100%;
    overflow-y: auto;
    background: rgba(232, 227, 218, 0.022);
  }
}

.bcr-block {
  display: flex;
  flex-direction: column;
  flex: 0 0 auto;
  gap: 0.4rem;
  padding-top: 0.8rem;
  border-top: 1px solid var(--color-border-subtle);
  min-width: 0;
}

/* 「当日日报」块撑开剩余高度（内容顶部对齐、块内不滚、不得被压扁）；其余四块贴底。 */
@media (min-width: 1440px) {
  .bcr.is-rail .bcr-block--grow {
    flex: 1 1 auto;
  }
}

/* 「当日其他话题」子块：默认隐藏，只在 ≥1440px 出现（该宽度段才有「当日日报」块留白可填；
   1200–1439.98px 右轨随页面流动，出该子块会改变该视口下的页面高度）。 */
.bcr-other {
  display: none;
}

@media (min-width: 1440px) {
  .bcr-other {
    display: block;
  }
}

.bcr-block:first-child {
  padding-top: 0;
  border-top: 0;
}

.bcr-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.45rem;
  min-width: 0;
}

.bcr-title {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.74rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  min-width: 0;
  white-space: nowrap;
}

.bcr-count {
  flex: 0 0 auto;
  font-size: 0.7rem;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
}

.bcr-manage {
  flex: 0 0 auto;
  border: 0;
  background: none;
  padding: 0;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.bcr-cands,
.bcr-watches {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.bcr-cand,
.bcr-watch {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  min-width: 0;
  font-size: 0.72rem;
  line-height: 1.5;
}

.bcr-cand-label,
.bcr-watch-label {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.bcr-watch-label--link {
  border: 0;
  padding: 0;
  background: none;
  font: inherit;
  text-align: left;
  color: var(--color-accent);
  cursor: pointer;
}

.bcr-watch-label--link:hover {
  text-decoration: underline;
}

.bcr-cand-meta,
.bcr-watch-state {
  flex: 0 0 auto;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.bcr-watch.is-paused .bcr-watch-label {
  color: var(--color-text-muted);
}

.bcr-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  width: 100%;
  padding: 0.3rem 0 0;
  border: 0;
  border-top: 1px solid var(--color-border-subtle);
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  text-align: left;
  cursor: pointer;
}

.bcr-entry:hover,
.bcr-entry:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.bcr-hint {
  margin: 0;
  font-size: 0.66rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.bcr-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  color: var(--color-error);
}

.bcr-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}
</style>
