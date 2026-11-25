<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import ThreadRelatedEvents from './ThreadRelatedEvents.vue'
import SectionWatchBadge from './SectionWatchBadge.vue'
import SectionTierBadge from './SectionTierBadge.vue'
import SectionAnchorBadge from './SectionAnchorBadge.vue'
import SectionQualityExplore from './SectionQualityExplore.vue'
import { topicAnchorTier } from '~/utils/topicAnchor'
import {
  groupSectionsByTopic,
  type QualityZone,
  type RequestCacheEntry,
  type TopicGroup,
} from './dailyReportMagazine'
import LaneTrendOverview from './LaneTrendOverview.vue'
import { contextCacheKey } from '~/features/tags/composables/useLaneTrendData'
import type { LaneDynamicsLane, LaneDynamicsResponse } from '~/api/laneDynamics'
import type { ContextRow } from '~/api/boardEnrichment'
import { isThreadFitDemoted, threadFitLabel } from '~/utils/threadFit'
import { AI_SUSPECT_LABEL, isAiSuspect } from '~/utils/aiDetection'
import { useTopicKeyEvents } from '~/features/tags/composables/useTopicKeyEvents'
import { useThreadRelatedEvents } from '~/features/tags/composables/useThreadRelatedEvents'
import {
  RELATED_WINDOW_DEFAULT,
  RELATED_WINDOW_OPTIONS,
} from '~/utils/threadRelation'
import type { ArticleOpenTarget } from '~/utils/threadRelation'
import { useConfirm } from '~/composables/useConfirm'
import { useMediaQuery } from '~/composables/useMediaQuery'
import KeyEventMarkerActions from '../KeyEventMarkerActions.vue'
import type { ArticleTitle } from '~/features/tags/composables/useDailyReportReader'
import type { DailyReportSection, DailyReportThread } from '~/api/dailyReports'

const props = defineProps<{
  zone: QualityZone
  /** 当期日报 id（相关线索的缓存键维度：一次请求覆盖整份日报）。 */
  reportId: number
  reportDate: string
  articleEntries: Map<number, RequestCacheEntry<ArticleTitle>>
  /** 板块级泳道趋势聚合缓存（宿主 useLaneTrendData；首个泳道展开触发，翻期不重拉）。 */
  laneDynamicsEntry: RequestCacheEntry<LaneDynamicsResponse>
  /** 话题周期归档缓存（键 contextCacheKey(topicId, granularity)，宿主 useLaneTrendData）。 */
  contextEntries: Map<string, RequestCacheEntry<ContextRow[]>>
  /** Section id requested by a watch index or timeline preview. */
  focusSectionId?: number | null
  /** 定位请求（D14）：clusterId = 聚簇代表 event 标签 id，threadId = 展开区指定线索（null = DOM 首条），
   *  nonce 变化重触发；本分区只处理自己的目标，无线程则静默不动作。 */
  locateThread?: { clusterId?: number | null, threadId?: number | null, nonce: number } | null
  /** 正在重新分桶的 section id（由 reader 持有；非空时按钮置灰 + spinner）。 */
  rebucketSectionId?: number | null
  /** 重分桶后被删除的旧锚点 → 该组首条 section（深链回退，add-intra-lane-theme-bucketing）。 */
  sectionAliases?: Map<number, number>
}>()

const emit = defineEmits<{
  ensureArticles: [articleIds: number[]]
  retryArticle: [articleId: number]
  /** 数字 = 普通文章行；对象 = chip 点击（带相关度上下文）。 */
  openArticle: [target: ArticleOpenTarget]
  /** 泳道展开信号：宿主首个展开时拉一次板块级 lane-dynamics（页面级缓存去重）。 */
  ensureLaneDynamics: [retry?: boolean]
  /** 月/年档切档信号：宿主按 topicId+granularity 按需拉取 contexts。 */
  ensureContext: [topicId: number, granularity: 'month' | 'year', retry?: boolean]
  /** 重新分桶动作（宿主调 reader.rebucket 并 toast）。 */
  rebucket: [sectionId: number]
}>()

const groups = computed(() => groupSectionsByTopic(props.zone))
const expandedTopics = ref(new Set<string>())
const expandedThreads = ref(new Set<string>())
let autoExpandedDate = ''

// 线索相关推荐共享缓存（单例）：键含报告与窗口，一次请求覆盖整份日报。
const threadRelations = useThreadRelatedEvents()
/** `7 天 / 30 天` 档位：分区级状态（工具条在每个内容分区的标题行，不放进线索行）。 */
const relatedDays = ref<number>(RELATED_WINDOW_DEFAULT)

// 关键事件共享缓存（单例）：当日/历史 thread 的挑选动作经同一份状态跨入口同步。
const keyEvents = useTopicKeyEvents()
// 重新分桶是弱破坏性但不可逆动作（旧桶分组不保留）→ 先二次确认（复用全局 useConfirm）。
const { confirm: confirmDialog } = useConfirm()

/** 只有算法桶组可重分桶：watch_* 物化组由关注轨自管（服务端也会 400）。 */
function canRebucket(section: DailyReportSection): boolean {
  return !section.lane_tier?.startsWith('watch_')
}

async function triggerRebucket(section: DailyReportSection) {
  if (props.rebucketSectionId != null) return
  const confirmed = await confirmDialog({
    title: '重新分桶',
    message: '会替换该天该泳道的主题分桶。事件线索与已确认的关键事件不会丢失，但旧的分桶分组不保留。',
    confirmText: '重新分桶',
  })
  if (!confirmed) return
  emit('rebucket', section.id)
}

function onKeyEventMark(section: DailyReportSection, thread: DailyReportThread) {
  if (section.persistent_topic_id != null) keyEvents.actThread(section.persistent_topic_id, thread.id, 'mark')
}
function onKeyEventDismiss(section: DailyReportSection, thread: DailyReportThread) {
  if (section.persistent_topic_id != null) keyEvents.actThread(section.persistent_topic_id, thread.id, 'dismiss')
}
function onKeyEventUnmark(section: DailyReportSection, thread: DailyReportThread) {
  if (section.persistent_topic_id == null) return
  const marker = keyEvents.threadView(thread.id).marker
  if (marker != null) keyEvents.actMarker(section.persistent_topic_id, marker.id, 'unmark')
}

const topicStatusLabel: Record<QualityZone['key'], string> = {
  active: '关心 · 持续追踪',
  briefs: '其他动态',
}

function threadRelationsEntry() {
  return threadRelations.entry(props.reportId, relatedDays.value)
}

/** 话题卡片展开即拉一次整份日报的推荐（成功过的键命中缓存，不重取）。 */
function ensureThreadRelations() {
  void threadRelations.ensure(props.reportId, relatedDays.value)
}

function changeRelatedDays(days: number) {
  if (relatedDays.value === days) return
  relatedDays.value = days
  // 已成功过的档位命中缓存（spec「切回 7 天直接使用缓存，不再次请求」）；
  // 失败档位在这里强制重发——档位控件常驻分区工具条，切档是 request 失败后唯一的
  // 显式重试入口（线索行在失败态整行不渲染，不再携带档位控件）。
  const cached = threadRelations.entry(props.reportId, days)
  void threadRelations.ensure(props.reportId, days, cached.status === 'error')
}

// 换报告（含切版块）后旧报告的推荐缓存不再有意义：清掉，后续 ensure 按新键重拉。
watch(() => props.reportId, () => threadRelations.resetLocal())

function articleEntry(articleId: number): RequestCacheEntry<ArticleTitle> {
  return props.articleEntries.get(articleId) ?? { status: 'idle' }
}

/** 展开列表的一条渲染项（判定标记与标题一次到位，不再单独取详情）。 */
interface ThreadArticleRow {
  id: number
  title: string
  suspect: boolean
}

/**
 * 展开列表的渲染项（add-ai-generated-article-detection design D8 / review R-4）：
 * 直接吃后端 `thread.articles` 投影——顺序就是 related_article_ids 的顺序（后端已在
 * populateThreadArticles 把疑似文章稳定排至末尾），前端**不再二次重排**，也不逐篇请求
 * 详情（N+1）。旧响应没有该字段（omitempty）时返回空数组，模板回退到标题缓存、不排序。
 */
function projectedArticleRows(thread: DailyReportThread): ThreadArticleRow[] {
  return (thread.articles ?? []).map(article => ({
    id: article.id,
    title: article.title || `文章 #${article.id}`,
    suspect: isAiSuspect(article.ai_detection_status),
  }))
}

/** 按 topic_id 从板块 lanes 过滤出本泳道聚合数据（无顶层 map，数组线性查）。 */
function laneForTopic(topicId: number): LaneDynamicsLane | null {
  return props.laneDynamicsEntry.data?.lanes.find(lane => lane.topic_id === topicId) ?? null
}

function contextEntry(topicId: number, granularity: 'month' | 'year'): RequestCacheEntry<ContextRow[]> {
  return props.contextEntries.get(contextCacheKey(topicId, granularity)) ?? { status: 'idle' }
}

/** 趋势区切档信号转发（v-if 已保证 topicId 非空；回调内窄化不可达，运行时再兜底）。 */
function forwardTrendContext(topicId: number | undefined, granularity: 'month' | 'year', retry?: boolean) {
  if (topicId != null) emit('ensureContext', topicId, granularity, retry)
}

function toggleTopic(group: TopicGroup) {
  const next = new Set(expandedTopics.value)
  if (next.has(group.key)) next.delete(group.key)
  else {
    next.add(group.key)
    // 线索 chip 行在两个分区都会渲染 → 推荐取数不按分区闸（趋势区仍只对 active 拉）
    ensureThreadRelations()
    if (group.topicId != null && props.zone.key === 'active') emit('ensureLaneDynamics')
  }
  expandedTopics.value = next
}

function threadKey(prefix: string, thread: DailyReportThread): string {
  return `${prefix}-${thread.id}`
}

function toggleThread(prefix: string, thread: DailyReportThread) {
  const key = threadKey(prefix, thread)
  const next = new Set(expandedThreads.value)
  if (next.has(key)) next.delete(key)
  else {
    next.add(key)
    // 后端投影已带标题与判定字段（thread.articles）；只有旧响应缺投影时才按 id 回补标题
    if (!thread.articles?.length && thread.related_article_ids?.length) emit('ensureArticles', thread.related_article_ids)
  }
  expandedThreads.value = next
}

// Thread-fit soft-degrade (observability System 3): off-topic threads are
// de-emphasized but never removed — the row stays visible (faded) with a
// left-aligned flag icon, and the section footer surfaces a plain status note
// ("可能跑题的线索 N 条"). No batch toggle: the note is informational, not an
// action; per-thread articles still open via each thread's own header.
function demotedThreads(section: DailyReportSection): DailyReportThread[] {
  // Defensive: legacy/abnormal payloads may omit threads (the backend now
  // always emits "threads": [], but guard the reader against stale caches).
  return (section.threads ?? []).filter(thread => isThreadFitDemoted(thread.fit_distance))
}

function demotedCount(section: DailyReportSection): number {
  return demotedThreads(section).length
}

// ── 重点定位（surface-story-heat-in-report-top）──

const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
/** 当前点亮的定位目标线程 id；同一时刻至多一个（重复定位替换）。 */
const litThreadId = ref<number | null>(null)
let litTimer: ReturnType<typeof setTimeout> | null = null

function clearThreadLit(): void {
  if (litTimer) {
    clearTimeout(litTimer)
    litTimer = null
  }
  litThreadId.value = null
}

/** 按当前正文数据序（= 渲染 DOM 序）找第一条属于该聚簇的线程：命中条件 = 线程
 *  `tag_ids` 含聚簇代表标签 id；范围限本分区当前 report 的 body 线程（历史节点是别的
 *  report，不参与）；不是最小 thread id。 */
function findThreadByCluster(clusterId: number): { groupKey: string, topicId?: number, threadId: number } | null {
  for (const group of groups.value) {
    for (const section of group.sections) {
      for (const thread of section.threads ?? []) {
        if ((thread.tag_ids ?? []).includes(clusterId)) {
          return { groupKey: group.key, topicId: group.topicId, threadId: thread.id }
        }
      }
    }
  }
  return null
}

/** 按 thread id 精确定位（今日重点展开区的行点击）：同一条数据序，换取所属分组供展开。 */
function findThreadById(threadId: number): { groupKey: string, topicId?: number, threadId: number } | null {
  for (const group of groups.value) {
    for (const section of group.sections) {
      for (const thread of section.threads ?? []) {
        if (thread.id === threadId) {
          return { groupKey: group.key, topicId: group.topicId, threadId: thread.id }
        }
      }
    }
  }
  return null
}

watch(() => props.locateThread, async (request) => {
  if (!request) return
  const hit = request.threadId != null
    ? findThreadById(request.threadId)
    : request.clusterId != null
      ? findThreadByCluster(request.clusterId)
      : null
  if (!hit) return // 目标在其它分区/已消失：本组件静默（阅读容器已按数据预判轻提示，不跳近似位置）
  if (!expandedTopics.value.has(hit.groupKey)) {
    const next = new Set(expandedTopics.value)
    next.add(hit.groupKey)
    expandedTopics.value = next
    ensureThreadRelations()
    if (hit.topicId != null && props.zone.key === 'active') emit('ensureLaneDynamics')
  }
  await nextTick()
  const target = document.getElementById(`report-thread-${hit.threadId}`)
  if (!target) return
  clearThreadLit()
  litThreadId.value = hit.threadId
  if (typeof target.scrollIntoView === 'function') {
    // reduced-motion 不平滑滚动（焦点轮廓仍可见，高亮不只靠颜色——有点亮类与焦点双重表达）。
    target.scrollIntoView({ behavior: prefersReducedMotion.value ? 'auto' : 'smooth', block: 'center' })
  }
  target.focus({ preventScroll: true })
  litTimer = setTimeout(clearThreadLit, 2000)
})

onUnmounted(clearThreadLit)

watch(() => props.reportDate, () => {
  // 切 report：清理定位高亮与计时器，无旧报回跳（spec「重复定位与减弱动画」）。
  clearThreadLit()
})

watch([groups, () => props.reportDate], ([nextGroups, reportDate]) => {
  if (props.zone.key !== 'active' || autoExpandedDate === reportDate || !nextGroups[0]) return
  autoExpandedDate = reportDate
  const first = nextGroups[0]
  expandedTopics.value = new Set([first.key])
  ensureThreadRelations()
  if (first.topicId != null) emit('ensureLaneDynamics')
}, { immediate: true })

watch(() => props.focusSectionId, async (requestedId) => {
  if (requestedId == null) return
  // 重分桶会删建 section 行：旧锚点失效时回退到该组首条 section（组也没了则放弃）。
  const sectionId = props.sectionAliases?.get(requestedId) ?? requestedId
  const group = groups.value.find(item => item.sections.some(section => section.id === sectionId))
  if (!group) return

  const next = new Set(expandedTopics.value)
  next.add(group.key)
  expandedTopics.value = next
  ensureThreadRelations()
  if (group.topicId != null && props.zone.key === 'active') emit('ensureLaneDynamics')

  await nextTick()
  const target = document.getElementById(`report-section-${sectionId}`)
  if (target && typeof target.scrollIntoView === 'function') {
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
})
</script>

<template>
  <section :id="`report-zone-${zone.key}`" class="drm-zone" :data-zone="zone.key">
    <header class="drm-zone__header">
      <div>
        <span class="drm-zone__eyebrow">{{ zone.eyebrow }}</span>
        <h2>{{ zone.label }}</h2>
      </div>
      <div class="drm-zone__aside">
        <span class="drm-zone__count">{{ groups.length }} 个话题</span>
        <!-- 相关线索档位：分区级工具条（不放进线索行——否则无候选的线索行会留下一条只
             有档位控件的空行）。全分区共用，切换即以新档位重取；成功过的档位命中缓存。 -->
        <div class="drm-zone__days" role="group" aria-label="相关线索时间范围">
          <button
            v-for="option in RELATED_WINDOW_OPTIONS"
            :key="option"
            type="button"
            class="drm-zone__day"
            :class="{ active: relatedDays === option }"
            :aria-pressed="relatedDays === option"
            :data-testid="`zone-related-days-${option}`"
            @click="changeRelatedDays(option)"
          >
            {{ option }} 天
          </button>
        </div>
      </div>
    </header>

    <div class="drm-zone__topics">
      <article
        v-for="(group, groupIndex) in groups"
        :id="group.topicId != null ? `report-topic-${group.topicId}` : undefined"
        :key="group.key"
        class="drm-topic"
        :style="{ '--topic-color': group.color || 'var(--color-accent)' }"
      >
        <button
          type="button"
          class="drm-topic__header"
          :aria-expanded="expandedTopics.has(group.key)"
          @click="toggleTopic(group)"
        >
          <span class="drm-topic__number">{{ String(groupIndex + 1).padStart(2, '0') }}</span>
          <span class="drm-topic__heading">
            <span class="drm-topic__status">
              <i aria-hidden="true" />
              {{ topicStatusLabel[zone.key] }}
            </span>
            <strong>{{ group.label }}</strong>
            <small>
              <i class="drm-topic__color-dot" aria-hidden="true" />
              <template v-if="group.canonicalLabel && group.canonicalLabel !== group.label">
                规范名 {{ group.canonicalLabel }} ·
              </template>
              {{ group.articleCount }} 篇文章 · {{ group.threadCount }} 条线索 ·
              {{ expandedTopics.has(group.key) ? '收起话题泳道' : '展开话题泳道' }}
            </small>
          </span>
          <Icon :icon="expandedTopics.has(group.key) ? 'mdi:minus' : 'mdi:plus'" width="18" />
        </button>

        <div v-if="expandedTopics.has(group.key)" class="drm-topic__body">
          <LaneTrendOverview
            v-if="zone.key === 'active' && group.topicId != null"
            :topic-id="group.topicId"
            :topic-color="group.color"
            :lane="laneForTopic(group.topicId)"
            :lane-entry="laneDynamicsEntry"
            :month-entry="contextEntry(group.topicId, 'month')"
            :year-entry="contextEntry(group.topicId, 'year')"
            @retry-lane="emit('ensureLaneDynamics', true)"
            @ensure-context="(granularity, retry) => forwardTrendContext(group.topicId, granularity, retry)"
          />
          <div class="drm-topic__sections">
            <article
              v-for="section in group.sections"
              :id="`report-section-${section.id}`"
              :key="section.id"
              class="drm-section-card"
            >
              <div class="drm-section-card__head">
                <span class="drm-section-card__badges">
                  <SectionWatchBadge v-if="section.lane_tier?.startsWith('watch_')" :lane-tier="section.lane_tier" :watch-label="section.watch_label" />
                  <SectionTierBadge :best-tier="section.best_tier" />
                  <SectionAnchorBadge :tier="topicAnchorTier(section.topic_match_distance, section.topic_match_confidence)" />
                </span>
                <span
                  v-if="section.bucket_title"
                  class="drm-section-bucket"
                  :title="section.bucket_title"
                >{{ section.bucket_title }}</span>
                <h4
                  v-if="group.sections.length > 1 || section.bucket_title"
                  class="drm-section-card__title"
                >{{ section.cluster_label }}</h4>
                <button
                  v-if="canRebucket(section)"
                  type="button"
                  class="drm-section-card__rebucket"
                  :disabled="rebucketSectionId != null"
                  :aria-busy="rebucketSectionId === section.id"
                  @click.stop="triggerRebucket(section)"
                >
                  <Icon
                    :icon="rebucketSectionId === section.id ? 'mdi:loading' : 'mdi:shuffle-variant'"
                    width="14"
                    :class="{ 'drm-section-card__rebucket--spin': rebucketSectionId === section.id }"
                  />
                  <span>重新分桶</span>
                </button>
                <SectionQualityExplore
                  :breakdown="section.quality_breakdown"
                  :topic-label="section.persistent_topic?.label || section.cluster_label"
                  :topic-distance="section.topic_match_distance"
                  :topic-confidence="section.topic_match_confidence"
                  class="drm-section-card__explore"
                />
              </div>
              <div class="drm-section-card__threads">
                <article
                  v-for="thread in section.threads"
                  :id="`report-thread-${thread.id}`"
                  :key="thread.id"
                  class="drm-thread"
                  :class="{ 'drm-thread--demoted': isThreadFitDemoted(thread.fit_distance), 'drm-thread--lit': litThreadId === thread.id }"
                  tabindex="-1"
                >
                  <div class="drm-thread-row">
                  <button
                    type="button"
                    class="drm-thread__header"
                    :disabled="!thread.related_article_ids?.length"
                    :aria-expanded="expandedThreads.has(threadKey(`current-${section.id}`, thread))"
                    @click="toggleThread(`current-${section.id}`, thread)"
                  >
                    <span>
                      <span class="drm-thread__title">
                        <Icon
                          v-if="isThreadFitDemoted(thread.fit_distance)"
                          icon="mdi:alert-circle-outline"
                          width="14"
                          class="drm-thread__flag"
                          aria-label="可能跑题的线索"
                        />
                        <strong>{{ thread.title }}</strong>
                      </span>
                      <!-- data-mn-anchor：页边注划选/锚定容器（批注范围限叙事文本，行头不开放批注但开放划选） -->
                      <small v-if="thread.summary" :data-mn-anchor="`t:${thread.id}`">{{ thread.summary }}</small>
                    </span>
                    <span class="drm-thread__meta">
                      <span v-if="thread.related_article_ids?.length" class="drm-thread__count">
                        {{ thread.related_article_ids.length }} 篇
                        <Icon icon="mdi:chevron-down" width="14" />
                      </span>
                    </span>
                  </button>
                  <KeyEventMarkerActions
                    mode="review"
                    :thread-id="thread.id"
                    :topic-id="section.persistent_topic_id"
                    :view="keyEvents.threadView(thread.id)"
                    @mark="onKeyEventMark(section, thread)"
                    @dismiss="onKeyEventDismiss(section, thread)"
                    @unmark="onKeyEventUnmark(section, thread)"
                  />
                  </div>
                  <!-- 线索相关引用：整宽一行（.drm-thread-row 的两列 grid 由 KeyEventMarkerActions 的多根 fragment 自动摆放，不塞第三块） -->
                  <ThreadRelatedEvents
                    :thread-id="thread.id"
                    :report-id="reportId"
                    :entry="threadRelationsEntry()"
                    @open-article="emit('openArticle', $event)"
                  />
                  <div
                    v-if="expandedThreads.has(threadKey(`current-${section.id}`, thread))"
                    class="drm-articles"
                  >
                    <p class="drm-thread__fit-probe">
                      贴合度<template v-if="typeof thread.fit_distance === 'number' && !Number.isNaN(thread.fit_distance)"> {{ thread.fit_distance.toFixed(2) }} ·</template> {{ threadFitLabel(thread.fit_distance) }}
                    </p>
                    <!-- 主路径：后端投影 thread.articles 已带标题 + 判定状态，顺序即后端排序结果
                         （疑似稳定排末），前端不再逐篇取详情、也不二次重排（review R-4） -->
                    <template v-if="projectedArticleRows(thread).length">
                      <button
                        v-for="article in projectedArticleRows(thread)"
                        :key="article.id"
                        type="button"
                        class="drm-article"
                        @click="emit('openArticle', article.id)"
                      >
                        <Icon icon="mdi:file-document-outline" width="14" />
                        <!-- 标记嵌在标题 span 内：.drm-article 是 3 列 grid，第 4 个直接子元素会挤窄标题列 -->
                        <span>{{ article.title }}<span
                          v-if="article.suspect"
                          class="drm-article__suspect"
                          :data-testid="`report-article-suspect-${article.id}`"
                        >{{ AI_SUSPECT_LABEL }}</span></span>
                      </button>
                    </template>
                    <!-- 旧响应回退：无 articles 投影 → 沿用标题缓存三态渲染（不排序，保持旧行为） -->
                    <template v-else>
                      <template v-for="articleId in thread.related_article_ids ?? []" :key="articleId">
                        <button
                          v-if="articleEntry(articleId).status === 'success'"
                          type="button"
                          class="drm-article"
                          @click="emit('openArticle', articleId)"
                        >
                          <Icon icon="mdi:file-document-outline" width="14" />
                          <!-- 标记嵌在标题 span 内：.drm-article 是 3 列 grid，第 4 个直接子元素会挤窄标题列 -->
                          <span>{{ articleEntry(articleId).data?.title }}<span
                            v-if="isAiSuspect(articleEntry(articleId).data?.aiDetectionStatus)"
                            class="drm-article__suspect"
                            :data-testid="`report-article-suspect-${articleId}`"
                          >{{ AI_SUSPECT_LABEL }}</span></span>
                        </button>
                        <button
                          v-else-if="articleEntry(articleId).status === 'error'"
                          type="button"
                          class="drm-article drm-article--error"
                          @click="emit('retryArticle', articleId)"
                        >
                          <Icon icon="mdi:refresh" width="14" />
                          <span>{{ articleEntry(articleId).error || `文章 #${articleId} 加载失败` }}，点击重试</span>
                        </button>
                        <div v-else class="drm-article drm-article--loading" aria-live="polite">
                          <Icon icon="mdi:loading" width="14" class="drm-spin" />
                          <span>正在查阅文章 #{{ articleId }}</span>
                        </div>
                      </template>
                    </template>
                  </div>
                </article>
                <p v-if="demotedCount(section) > 0" class="drm-thread__hint">
                  <Icon icon="mdi:alert-circle-outline" width="13" aria-hidden="true" />
                  可能跑题的线索 {{ demotedCount(section) }} 条
                </p>
              </div>
            </article>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
.drm-zone {
  scroll-margin-top: 5rem;
  padding: 1.5rem 0 2.75rem;
  border-top: 3px double var(--color-border-strong);
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
  animation-delay: 0.2s;
}

@keyframes drmInkFade {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

.drm-zone__header {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 1rem;
  padding-bottom: 0.85rem;
  border-bottom: 2px solid var(--color-border-strong);
  font-family: "Noto Serif SC", serif;
}

.drm-zone__header h2 {
  display: inline;
  margin: 0;
  color: var(--color-text-primary);
  font-size: clamp(1.5rem, 2.5vw, 2.25rem);
}

.drm-zone__eyebrow {
  margin-right: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  font-style: italic;
}

.drm-zone__count {
  color: var(--color-text-muted);
  font-size: 0.7rem;
}

.drm-zone__aside {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}

/* 档位复用趋势区的分段控件视觉（原生 button + active 态），不自造控件 */
.drm-zone__days {
  display: flex;
  gap: 0.25rem;
}

.drm-zone__day {
  padding: 0.2rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.65rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.drm-zone__day:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text-secondary);
}

.drm-zone__day.active {
  border-color: var(--color-border-strong);
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.drm-zone__day:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

.drm-zone__topics {
  display: grid;
  grid-template-columns: 1fr;
}

.drm-topic {
  min-width: 0;
  border-bottom: 1px solid var(--color-border-medium);
}

.drm-topic__header {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 0.75rem;
  width: 100%;
  padding: 1.25rem 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.drm-topic__header:hover .drm-topic__heading strong,
.drm-topic__header:focus-visible .drm-topic__heading strong {
  color: var(--topic-color);
}

.drm-topic__header:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 3px;
}

.drm-topic__number {
  padding-top: 0.15rem;
  color: var(--color-accent);
  font-family: "Noto Serif SC", serif;
  font-size: 0.72rem;
  font-style: italic;
  font-weight: 700;
}

.drm-topic__heading {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.drm-topic__heading strong {
  margin-top: 0.35rem;
  font-family: "Noto Serif SC", serif;
  font-size: clamp(1.2rem, 1.7vw, 1.55rem);
  font-weight: 700;
  line-height: 1.35;
  transition: color 160ms ease;
}

.drm-topic__heading small {
  margin-top: 0.25rem;
  color: var(--color-text-muted);
  font-size: 0.65rem;
}

.drm-topic__status {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 0.3rem;
  width: max-content;
  padding: 0.2rem 0.5rem;
  border: 0;
  background: color-mix(in srgb, var(--topic-color) 18%, var(--color-bg-base));
  color: color-mix(in srgb, var(--topic-color) 75%, var(--color-text-primary));
  font-size: 0.62rem;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.drm-topic__status i,
.drm-topic__color-dot {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 50%;
  background: var(--topic-color);
}

.drm-topic__color-dot {
  display: inline-block;
  margin-right: 0.2rem;
  vertical-align: 0.05rem;
}

.drm-topic__body {
  grid-column: 1 / -1;
  padding-bottom: 1.25rem;
}

.drm-topic__sections {
  display: grid;
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.drm-section-card {
  position: relative;
  scroll-margin-top: 5rem;
}

.drm-section-card__head {
  position: relative;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-height: 1.25rem;
  margin-bottom: 0.35rem;
}

.drm-section-card__badges {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  flex-shrink: 0;
}

/* Quality probe panel — hidden by default, revealed on hover/focus of the
   section head so it never disturbs immersive reading. Colours and the
   downgrade marker come from SectionQualityExplore + match-quality tokens. */
.drm-section-card__explore {
  position: absolute;
  top: calc(100% + 0.3rem);
  left: 0;
  z-index: 4;
  max-width: 20rem;
  padding: 0.5rem 0.65rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  box-shadow: var(--shadow-medium);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transform: translateY(-2px);
  transition: opacity 0.15s ease, transform 0.15s ease, visibility 0.15s ease;
}

.drm-section-card__head:hover .drm-section-card__explore,
.drm-section-card__head:focus-within .drm-section-card__explore {
  opacity: 1;
  visibility: visible;
  pointer-events: auto;
  transform: translateY(0);
}

/* 主题方向徽标（bucket_title）：与既有徽标族同尺寸/配色语言，不自造排版。
   宽度上限用 em（相对自身 0.7rem 字号）而非 ch——ch 是数字字符宽，汉字约占 2ch，
   12ch 只能放下 6 个汉字，会让 ≤12 字的方向名全部被截断（2026-10-24 真机实测见
   ui-verification/report.md A-4）。14em ≈ 14 个汉字，覆盖 prompt 的 12 字上限。 */
.drm-section-bucket {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  max-width: 14em;
  padding: 0 0.35rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  line-height: 1.5;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.drm-section-card__rebucket {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  margin-left: auto;
  padding: 0.1rem 0.4rem;
  border: 1px solid transparent;
  border-radius: 999px;
  background: transparent;
  color: var(--color-text-tertiary);
  font-size: 0.7rem;
  cursor: pointer;
  opacity: 0.55;
  transition: opacity 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.drm-section-card__head:hover .drm-section-card__rebucket,
.drm-section-card__rebucket:focus-visible {
  opacity: 1;
  border-color: var(--color-border-medium);
  color: var(--color-text-secondary);
}

.drm-section-card__rebucket:disabled {
  cursor: progress;
  opacity: 0.45;
}

.drm-section-card__rebucket--spin {
  animation: drm-rebucket-spin 0.9s linear infinite;
}

@keyframes drm-rebucket-spin {
  to { transform: rotate(360deg); }
}

.drm-section-card__title {
  min-width: 0;
  margin: 0;
  color: var(--color-text-secondary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.85rem;
  font-weight: 600;
  line-height: 1.4;
}

.drm-section-card__threads {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0;
}

.drm-thread {
  position: relative;
  padding: 0.75rem 0 0.75rem 0.875rem;
  border-top: 0;
  border-left: 2px solid var(--color-border-subtle);
  transition: border-color 0.25s ease, background 0.25s ease;
}

.drm-thread:hover {
  border-left-color: var(--color-accent);
  background: var(--color-bg-active);
}

.drm-thread::before {
  position: absolute;
  top: 1.1rem;
  left: -4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-border-strong);
  content: "";
  transition: background 0.25s ease;
}

.drm-thread:hover::before {
  background: var(--color-accent);
}

/* 重点定位点亮（surface-story-heat-in-report-top）：刊头点击后 2s 单目标高亮。
   不只靠颜色：目标线程同时被程序聚焦（焦点轮廓）；reduced-motion 由 JS 侧关平滑滚动。 */
.drm-thread--lit {
  border-left-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 12%, transparent);
}

.drm-thread--lit::before {
  background: var(--color-accent);
}

.drm-thread__header {
  display: flex;
  align-items: start;
  justify-content: space-between;
  gap: 1rem;
  width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  /* 页边注（daily-report-margin-notes）：显式放开 button 内划选；
     划选后的误触由宿主 selection guard 吞 click 消解，行为经行为核定（Chromium 152） */
  user-select: text;
}

/*
  线索行分区（对齐原型 thread-row）：标题/徽标左列（50%），AI 理由等详情右半等宽列
  （50%）；动作组全宽底行右对齐（行右下角）。
  .ke-detail/.ke-actions 的摆放规则住在 KeyEventMarkerActions 自身 scoped 样式：
  多根 fragment 组件根节点不继承宿主 scope 属性，宿主子选择器永不命中（死代码）。
*/
.drm-thread-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  column-gap: 1rem;
  row-gap: 7px;
  align-items: start;
}

.drm-thread-row > .drm-thread__header {
  grid-column: 1;
  margin: 0;
}

/* 摆放规则已移入 KeyEventMarkerActions 自身 scoped 样式（fragment 根不继承宿主 scope，宿主子选择器永不命中） */

.drm-thread__header:not(:disabled) {
  cursor: pointer;
}

.drm-thread__header:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

.drm-thread__header > span:first-child {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.drm-thread__header strong {
  font-size: 0.82rem;
  font-weight: 600;
  line-height: 1.55;
}

.drm-thread__header small {
  margin-top: 0.25rem;
  color: var(--color-text-secondary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.72rem;
  line-height: 1.75;
}

.drm-thread__count {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.65rem;
}

/* Thread-fit soft-degrade (observability System 3): an off-topic thread is
   de-emphasized (faded title + flag) but never removed. Colours derive from
   theme tokens via color-mix so they follow the editorial/dark themes — no
   hard-coded hex, mirroring matchQuality.ts. */
.drm-thread--demoted {
  border-left-color: color-mix(in srgb, var(--color-text-muted) 60%, transparent);
}

.drm-thread--demoted:hover {
  border-left-color: color-mix(in srgb, var(--color-accent) 55%, var(--color-text-muted));
}

.drm-thread--demoted::before {
  background: color-mix(in srgb, var(--color-text-muted) 70%, transparent);
}

.drm-thread--demoted .drm-thread__header strong {
  color: color-mix(in srgb, var(--color-text-primary) 55%, var(--color-text-muted));
  font-weight: 500;
}

.drm-thread--demoted .drm-thread__header small {
  color: var(--color-text-muted);
}

.drm-thread__meta {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  flex: 0 0 auto;
}

.drm-thread__title {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
}

.drm-thread__flag {
  flex: 0 0 auto;
  align-self: center;
  color: color-mix(in srgb, var(--color-text-muted) 80%, var(--color-accent));
}

/* Fit probe lives inside the expanded .drm-articles block, so the distance
   number only surfaces once the reader digs in — it never leaks into the
   collapsed thread header (observability display layering). */
.drm-thread__fit-probe {
  margin: 0 0 0.5rem;
  padding: 0.25rem 0.5rem;
  border-left: 2px solid var(--color-border-subtle);
  color: var(--color-text-muted);
  font-size: 0.65rem;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
}

/* Fit-demotion status note: a plain informational line at the section footer
   ("可能跑题的线索 N 条"), NOT an action. Kept muted and non-interactive so it
   reads as a state annotation rather than masquerading as a toggle — the flag
   icon already marks each off-topic thread at its title (left-aligned). */
.drm-thread__hint {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  margin: 0.4rem 0 0;
  padding: 0 0 0 0.875rem;
  color: var(--color-text-muted);
  font-size: 0.66rem;
}

.drm-articles {
  display: grid;
  gap: 0.35rem;
  padding: 0 0 0.85rem 1rem;
}

.drm-article {
  display: grid;
  grid-template-columns: 3px auto 1fr;
  gap: 0.55rem;
  align-items: center;
  margin-top: 0.3rem;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--color-border-subtle);
  border-left: 0;
  background: var(--color-bg-base);
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  text-align: left;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.drm-article::before {
  width: 3px;
  height: 100%;
  min-height: 1.5rem;
  border-radius: 1px;
  background: var(--color-accent);
  content: "";
}

button.drm-article {
  cursor: pointer;
}

/* 疑似 AI 生成标记（add-ai-generated-article-detection）：随既有线索 chip 的紧凑内联形态，
   中性色、不占规则/警示色语义；仅 suspect 渲染，未判定不占位。 */
.drm-article__suspect {
  display: inline-block;
  margin-left: 0.35rem;
  padding: 0 0.3rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  color: var(--color-text-secondary);
  font-size: 0.66rem;
  line-height: 1.5;
  white-space: nowrap;
}

button.drm-article:hover,
button.drm-article:focus-visible {
  border-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  color: var(--color-text-primary);
  outline: none;
}

.drm-article--error {
  color: var(--color-error);
}

.drm-article--loading {
  color: var(--color-text-muted);
}

.drm-spin {
  animation: drmSpin 0.9s linear infinite;
}

@keyframes drmSpin {
  to { transform: rotate(1turn); }
}

@media (max-width: 900px) {
  .drm-zone__topics {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  /* 窄屏回退单列，不挤压线索标题；.ke-detail/.ke-actions 的窄屏摆放
     （detail r2 全宽、动作组 r3）在 KeyEventMarkerActions 自身 scoped 样式的
     同断点媒体查询里（断点数值必须与这里同步，两处注释互指锚定）。 */
  .drm-thread-row {
    grid-template-columns: minmax(0, 1fr);
  }

  /* 摆放规则已移入 KeyEventMarkerActions 自身 scoped 样式（fragment 根不继承宿主 scope，宿主子选择器永不命中） */

  .drm-zone__header {
    align-items: start;
  }

  .drm-topic__header {
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: 0.55rem;
  }

  .drm-topic__number {
    display: none;
  }

  .drm-section-card__threads {
    padding: 0 0.75rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .drm-zone,
  .drm-zone__day,
  .drm-topic__heading strong,
  .drm-spin {
    transition: none;
    animation: none;
  }
}
</style>
