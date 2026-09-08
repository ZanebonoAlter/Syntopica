<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import DailyReportMiniLifeline from './DailyReportMiniLifeline.vue'
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
import { isThreadFitDemoted, threadFitLabel } from '~/utils/threadFit'
import type { ArticleTitle, TopicLifelineData } from '~/features/tags/composables/useDailyReportReader'
import type { DailyReport, DailyReportSection, DailyReportThread } from '~/api/dailyReports'

const props = defineProps<{
  zone: QualityZone
  reportDate: string
  lifelineEntries: Map<number, RequestCacheEntry<TopicLifelineData>>
  articleEntries: Map<number, RequestCacheEntry<ArticleTitle>>
  reportDetails: Map<number, DailyReport>
  /** Section id requested by a watch index or timeline preview. */
  focusSectionId?: number | null
}>()

const emit = defineEmits<{
  ensureLifeline: [topicId: number, retry?: boolean]
  ensureArticles: [articleIds: number[]]
  retryArticle: [articleId: number]
  loadHistorical: [reportIds: number[]]
  openArticle: [articleId: number]
  openDetective: [topicId: number]
}>()

const groups = computed(() => groupSectionsByTopic(props.zone))
const expandedTopics = ref(new Set<string>())
const selectedDayByTopic = ref(new Map<number, string>())
const expandedThreads = ref(new Set<string>())
let autoExpandedDate = ''

const topicStatusLabel: Record<QualityZone['key'], string> = {
  active: '关心 · 持续追踪',
  briefs: '其他动态',
}

function lifelineEntry(topicId?: number): RequestCacheEntry<TopicLifelineData> {
  return topicId == null ? { status: 'idle' } : props.lifelineEntries.get(topicId) ?? { status: 'idle' }
}

function articleEntry(articleId: number): RequestCacheEntry<ArticleTitle> {
  return props.articleEntries.get(articleId) ?? { status: 'idle' }
}

function toggleTopic(group: TopicGroup) {
  const next = new Set(expandedTopics.value)
  if (next.has(group.key)) next.delete(group.key)
  else {
    next.add(group.key)
    if (group.topicId != null && props.zone.key === 'active') emit('ensureLifeline', group.topicId)
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
    if (thread.related_article_ids?.length) emit('ensureArticles', thread.related_article_ids)
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

function selectLifelineDay(topicId: number, dayKey: string) {
  const next = new Map(selectedDayByTopic.value)
  if (next.get(topicId) === dayKey) next.delete(topicId)
  else next.set(topicId, dayKey)
  selectedDayByTopic.value = next

  const reportIds = (lifelineEntry(topicId).data?.sections ?? [])
    .filter(section => section.period_date.slice(0, 10) === dayKey)
    .map(section => section.report_id)
  if (reportIds.length) emit('loadHistorical', [...new Set(reportIds)])
}

function historicalSections(group: TopicGroup): DailyReportSection[] {
  if (group.topicId == null) return []
  const dayKey = selectedDayByTopic.value.get(group.topicId)
  if (!dayKey) return []
  const nodes = (lifelineEntry(group.topicId).data?.sections ?? [])
    .filter(section => section.period_date.slice(0, 10) === dayKey)
  return nodes.flatMap((node) => {
    const report = props.reportDetails.get(node.report_id)
    const section = report?.sections.find(item => item.id === node.id)
    return section ? [section] : []
  })
}

function historicalPending(group: TopicGroup): boolean {
  if (group.topicId == null) return false
  const dayKey = selectedDayByTopic.value.get(group.topicId)
  if (!dayKey) return false
  const nodes = (lifelineEntry(group.topicId).data?.sections ?? [])
    .filter(section => section.period_date.slice(0, 10) === dayKey)
  return nodes.some(node => !props.reportDetails.has(node.report_id))
}

watch([groups, () => props.reportDate], ([nextGroups, reportDate]) => {
  if (props.zone.key !== 'active' || autoExpandedDate === reportDate || !nextGroups[0]) return
  autoExpandedDate = reportDate
  const first = nextGroups[0]
  expandedTopics.value = new Set([first.key])
  if (first.topicId != null) emit('ensureLifeline', first.topicId)
}, { immediate: true })

watch(() => props.focusSectionId, async (sectionId) => {
  if (sectionId == null) return
  const group = groups.value.find(item => item.sections.some(section => section.id === sectionId))
  if (!group) return

  const next = new Set(expandedTopics.value)
  next.add(group.key)
  expandedTopics.value = next
  if (group.topicId != null && props.zone.key === 'active') emit('ensureLifeline', group.topicId)

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
      <span>{{ groups.length }} 个话题</span>
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
                <h4 v-if="group.sections.length > 1" class="drm-section-card__title">{{ section.cluster_label }}</h4>
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
                  :key="thread.id"
                  class="drm-thread"
                  :class="{ 'drm-thread--demoted': isThreadFitDemoted(thread.fit_distance) }"
                >
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
                      <small v-if="thread.summary">{{ thread.summary }}</small>
                    </span>
                    <span class="drm-thread__meta">
                      <span v-if="thread.related_article_ids?.length" class="drm-thread__count">
                        {{ thread.related_article_ids.length }} 篇
                        <Icon icon="mdi:chevron-down" width="14" />
                      </span>
                    </span>
                  </button>
                  <div
                    v-if="expandedThreads.has(threadKey(`current-${section.id}`, thread))"
                    class="drm-articles"
                  >
                    <p class="drm-thread__fit-probe">
                      贴合度<template v-if="typeof thread.fit_distance === 'number' && !Number.isNaN(thread.fit_distance)"> {{ thread.fit_distance.toFixed(2) }} ·</template> {{ threadFitLabel(thread.fit_distance) }}
                    </p>
                    <template v-for="articleId in thread.related_article_ids" :key="articleId">
                      <button
                        v-if="articleEntry(articleId).status === 'success'"
                        type="button"
                        class="drm-article"
                        @click="emit('openArticle', articleId)"
                      >
                        <Icon icon="mdi:file-document-outline" width="14" />
                        <span>{{ articleEntry(articleId).data?.title }}</span>
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
                  </div>
                </article>
                <p v-if="demotedCount(section) > 0" class="drm-thread__hint">
                  <Icon icon="mdi:alert-circle-outline" width="13" aria-hidden="true" />
                  可能跑题的线索 {{ demotedCount(section) }} 条
                </p>
              </div>
            </article>
          </div>

          <DailyReportMiniLifeline
            v-if="zone.key === 'active' && group.topicId != null"
            :topic-id="group.topicId"
            :topic-color="group.color"
            :report-date="reportDate"
            :entry="lifelineEntry(group.topicId)"
            :selected-day-key="selectedDayByTopic.get(group.topicId)"
            @retry="emit('ensureLifeline', group.topicId, true)"
            @select-day="selectLifelineDay(group.topicId, $event)"
            @open-detective="emit('openDetective', $event)"
          >
            <template #details>
              <section
                v-if="selectedDayByTopic.get(group.topicId)"
                class="drm-history"
                aria-label="历史节点详情"
              >
                <header>
                  <span>{{ selectedDayByTopic.get(group.topicId) }}</span>
                  <button type="button" @click="selectLifelineDay(group.topicId, selectedDayByTopic.get(group.topicId)!)">
                    收起
                  </button>
                </header>
                <div v-if="historicalPending(group)" class="drm-history__loading">
                  <Icon icon="mdi:loading" width="16" class="drm-spin" />
                  正在加载当日线索…
                </div>
                <div v-else-if="!historicalSections(group).length" class="drm-history__loading">当日暂无可展开线索。</div>
                <article v-for="section in historicalSections(group)" :key="section.id" class="drm-history__section">
                  <h3>{{ section.cluster_label }}</h3>
                  <article
                    v-for="thread in section.threads"
                    :key="thread.id"
                    class="drm-thread"
                    :class="{ 'drm-thread--demoted': isThreadFitDemoted(thread.fit_distance) }"
                  >
                    <button
                      type="button"
                      class="drm-thread__header"
                      :disabled="!thread.related_article_ids?.length"
                      :aria-expanded="expandedThreads.has(threadKey(`history-${section.id}`, thread))"
                      @click="toggleThread(`history-${section.id}`, thread)"
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
                        <small v-if="thread.summary">{{ thread.summary }}</small>
                      </span>
                      <span class="drm-thread__meta">
                        <span v-if="thread.related_article_ids?.length" class="drm-thread__count">
                          {{ thread.related_article_ids.length }} 篇
                          <Icon icon="mdi:chevron-down" width="14" />
                        </span>
                      </span>
                    </button>
                    <div
                      v-if="expandedThreads.has(threadKey(`history-${section.id}`, thread))"
                      class="drm-articles"
                    >
                      <p class="drm-thread__fit-probe">
                        贴合度<template v-if="typeof thread.fit_distance === 'number' && !Number.isNaN(thread.fit_distance)"> {{ thread.fit_distance.toFixed(2) }} ·</template> {{ threadFitLabel(thread.fit_distance) }}
                      </p>
                      <template v-for="articleId in thread.related_article_ids" :key="articleId">
                        <button
                          v-if="articleEntry(articleId).status === 'success'"
                          type="button"
                          class="drm-article"
                          @click="emit('openArticle', articleId)"
                        >
                          <Icon icon="mdi:file-document-outline" width="14" />
                          <span>{{ articleEntry(articleId).data?.title }}</span>
                        </button>
                        <button
                          v-else-if="articleEntry(articleId).status === 'error'"
                          type="button"
                          class="drm-article drm-article--error"
                          @click="emit('retryArticle', articleId)"
                        >
                          <Icon icon="mdi:refresh" width="14" />
                          <span>文章加载失败，点击重试</span>
                        </button>
                        <div v-else class="drm-article drm-article--loading">
                          <Icon icon="mdi:loading" width="14" class="drm-spin" />
                          <span>正在查阅文章 #{{ articleId }}</span>
                        </div>
                      </template>
                    </div>
                  </article>
                  <p v-if="demotedCount(section) > 0" class="drm-thread__hint">
                    <Icon icon="mdi:alert-circle-outline" width="13" aria-hidden="true" />
                    可能跑题的线索 {{ demotedCount(section) }} 条
                  </p>
                </article>
              </section>
            </template>
          </DailyReportMiniLifeline>
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

.drm-zone__header > span {
  color: var(--color-text-muted);
  font-size: 0.7rem;
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

.drm-section-card__title {
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
}

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

.drm-history {
  margin: 0.75rem 1rem 0;
  padding: 1rem;
  border-left: 2px solid var(--color-accent);
  background: var(--color-bg-sunken);
}

.drm-history > header {
  display: flex;
  justify-content: space-between;
  padding-bottom: 0.65rem;
  border-bottom: 1px solid var(--color-border-medium);
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.68rem;
}

.drm-history > header button {
  border: 0;
  background: transparent;
  color: var(--color-link);
  cursor: pointer;
}

.drm-history__loading {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem 0;
  color: var(--color-text-muted);
  font-size: 0.72rem;
}

.drm-history__section h3 {
  margin: 1rem 0 0.35rem;
  font-family: "Noto Serif SC", serif;
  font-size: 0.92rem;
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
  .drm-topic__heading strong,
  .drm-spin {
    transition: none;
    animation: none;
  }
}
</style>
