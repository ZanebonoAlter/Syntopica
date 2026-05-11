<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useArticlesApi } from '~/api/articles'
import {
  useTopicGraphApi,
  type HotspotDigestCard,
  type TopicCategory,
  type TopicGraphDetailPayload,
  type TopicGraphFilters,
  type TopicGraphType,
  type TopicKind,
  type TopicsByCategoryPayload,
} from '~/api/topicGraph'
import type { Article } from '~/types'
import type { TimelineDigest, TimelineDigestSelection, PendingArticle, TimelineAggregationGroup, TimelineAggregationMode, TimelineAggregationArticle } from '~/types/timeline'
import ArticleContentView from '~/features/articles/components/ArticleContentView.vue'
import { useApiStore } from '~/stores/api'
import { normalizeArticle, type ArticlePayload } from '../../articles/utils/normalizeArticle'
import FeedCategoryFilter from '~/features/topic-graph/components/FeedCategoryFilter.vue'
import TopicGraphCanvas from '~/features/topic-graph/components/TopicGraphCanvas.client.vue'
import TopicGraphFooterPanels from '~/features/topic-graph/components/TopicGraphFooterPanels.vue'
import TopicGraphHeader from '~/features/topic-graph/components/TopicGraphHeader.vue'
import TopicGraphSidebar from '~/features/topic-graph/components/TopicGraphSidebar.vue'
import TopicTimeline from '~/features/topic-graph/components/TopicTimeline.vue'
import NarrativePanel from '~/features/topic-graph/components/NarrativePanel.vue'
import BoardConceptManager from '~/features/topic-graph/components/BoardConceptManager.vue'
import TagHierarchy from '~/features/topic-graph/components/TagHierarchy.vue'
import { buildDisplayedTopicGraph, collectRelatedTopicSlugs } from '~/features/topic-graph/utils/buildDisplayedTopicGraph'
import { buildTopicGraphViewModel } from '~/features/topic-graph/utils/buildTopicGraphViewModel'
import { normalizeTopicCategory } from '~/features/topic-graph/utils/normalizeTopicCategory'

// Timeline panel drag state
const timelinePanelRef = ref<HTMLDivElement | null>(null)
const isDragging = ref(false)
const dragOffset = ref({ x: 0, y: 0 })
const timelinePosition = ref({ x: 0, y: 0 })
const initialRect = ref({ left: 0, top: 0 })
let initialPositionSet = false

function handleTimelineDragStart(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (target.closest('button')) {
    return
  }
  
  const rect = timelinePanelRef.value?.getBoundingClientRect()
  if (!rect) return
  
  if (!initialPositionSet) {
    initialRect.value = { left: rect.left, top: rect.top }
    initialPositionSet = true
  }
  
  isDragging.value = true
  dragOffset.value = {
    x: event.clientX - rect.left,
    y: event.clientY - rect.top,
  }
  event.preventDefault()
  document.addEventListener('mousemove', handleTimelineDrag)
  document.addEventListener('mouseup', handleTimelineDragEnd)
}

function handleTimelineDrag(event: MouseEvent) {
  if (!isDragging.value) return
  timelinePosition.value = {
    x: event.clientX - dragOffset.value.x - initialRect.value.left,
    y: event.clientY - dragOffset.value.y - initialRect.value.top,
  }
}

function handleTimelineDragEnd() {
  isDragging.value = false
  document.removeEventListener('mousemove', handleTimelineDrag)
  document.removeEventListener('mouseup', handleTimelineDragEnd)
}

function resetTimelinePanelPosition() {
  timelinePosition.value = { x: 0, y: 0 }
  initialPositionSet = false
  initialRect.value = { left: 0, top: 0 }
}

onBeforeUnmount(() => {
  document.removeEventListener('mousemove', handleTimelineDrag)
  document.removeEventListener('mouseup', handleTimelineDragEnd)
})


const topicGraphApi = useTopicGraphApi()
const articlesApi = useArticlesApi()
const apiStore = useApiStore()


function formatDateInput(date = new Date()) {
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

const selectedType = ref<TopicGraphType>('daily')
const selectedDate = ref(formatDateInput())
const selectedFilterCategoryId = ref<string | null>(null)
const selectedFilterFeedId = ref<string | null>(null)
const graphPayload = ref<Awaited<ReturnType<typeof topicGraphApi.getGraph>>['data'] | null>(null)
const selectedTopicSlug = ref<string | null>(null)
const selectedCategory = ref<TopicCategory | null>(null)
const selectedKeywordSlug = ref<string | null>(null)
const selectedDigestId = ref<string | null>(null)
const previewDigestId = ref<string | null>(null)
const graphFocusRequestKey = ref(0)
const detail = ref<TopicGraphDetailPayload | null>(null)
const loadingGraph = ref(false)
const loadingDetail = ref(false)
const loadingPreviewArticle = ref(false)
const notice = ref<string | null>(null)
const selectedPreviewArticle = ref<Article | null>(null)
const previewArticles = ref<Article[]>([])

// Hotspot topics state (from getTopicsByCategory API)
const hotspotData = ref<TopicsByCategoryPayload | null>(null)
const loadingHotspots = ref(false)
const graphVisibilityOverrides = ref<Record<string, boolean>>({})
const expandedTopicSlugs = ref<string[]>([])

// Hotspot digests state (reverse trace: tag -> articles -> digests)
const hotspotDigests = ref<HotspotDigestCard[]>([])
const loadingHotspotDigests = ref(false)
const selectedHotspotTag = ref<{ slug: string; label: string; category: TopicCategory } | null>(null)

// Pending articles state
const pendingArticles = ref<PendingArticle[]>([])
const selectedPendingNode = ref(false)
const loadingPendingArticles = ref(false)

// Timeline aggregation state
const aggregationMode = ref<TimelineAggregationMode>('day')
const aggregatedArticles = ref<TimelineAggregationArticle[]>([])
const loadingAggregatedArticles = ref(false)
const selectedGroupKey = ref<string | null>(null)

// Abstract tag node tracking (for sidebar detail panel)
const abstractNodeSlug = ref<string | null>(null)
const abstractNodeLabel = ref<string | null>(null)
const selectedChildTagSlug = ref<string | null>(null)

// Timeline floating panel state
const timelineOpen = ref(false)
const showConceptManager = ref(false)

watch(timelineOpen, (open) => {
  if (!open) {
    resetTimelinePanelPosition()
    isDragging.value = false
  }
})

// Active view tab state (graph / hierarchy)
const activeTab = ref<'graph' | 'hierarchy' | 'narrative'>('graph')

const viewModel = computed(() => graphPayload.value
  ? buildTopicGraphViewModel(graphPayload.value)
  : buildTopicGraphViewModel({
      type: selectedType.value,
      anchor_date: selectedDate.value,
      period_label: '正在载入',
      topic_count: 0,
      article_count: 0,
      feed_count: 0,
      top_topics: [],
      nodes: [],
      edges: [],
    }))

const activeTopicNode = computed(() => {
  const focusSlug = selectedKeywordSlug.value || selectedTopicSlug.value
  return displayedGraph.value.nodes.find(node => node.slug === focusSlug) || null
})
const highlightedNodeIds = computed(() => {
  const highlighted = new Set<string>()
  const focusSlug = selectedKeywordSlug.value || selectedTopicSlug.value
  if (!focusSlug) return []

  const focusNode = displayedGraph.value.nodes.find(node => node.slug === focusSlug)
  if (!focusNode) return []

  highlighted.add(focusNode.id)

  for (const edge of displayedGraph.value.edges) {
    const sourceId = resolveGraphLinkNodeId(edge.source)
    const targetId = resolveGraphLinkNodeId(edge.target)

    if (sourceId === focusNode.id) {
      highlighted.add(targetId)
    }
    if (targetId === focusNode.id) {
      highlighted.add(sourceId)
    }
  }

  return Array.from(highlighted)
})
const relatedEdgeIds = computed(() => {
  const highlightedSet = new Set(highlightedNodeIds.value)
  if (!highlightedSet.size) return []

  return displayedGraph.value.edges
    .filter(edge => {
      return highlightedSet.has(resolveGraphLinkNodeId(edge.source)) && highlightedSet.has(resolveGraphLinkNodeId(edge.target))
    })
    .map(edge => edge.id)
})

function resolveGraphLinkNodeId(node: string | { id: string }) {
  return typeof node === 'string' ? node : node.id
}

function isTopicShownInGraph(slug: string) {
  return graphVisibleTopicSlugs.value.has(slug)
}

function ensureTopicShownInGraph(slug: string) {
  if (isTopicShownInGraph(slug)) return

  graphVisibilityOverrides.value = {
    ...graphVisibilityOverrides.value,
    [slug]: true,
  }
}

function toggleTopicGraphVisibility(slug: string) {
  const nextVisible = !isTopicShownInGraph(slug)
  const defaultVisible = defaultGraphTopicSlugs.value.has(slug)
  const nextOverrides = { ...graphVisibilityOverrides.value }

  if (nextVisible === defaultVisible) {
    delete nextOverrides[slug]
  } else {
    nextOverrides[slug] = nextVisible
  }

  graphVisibilityOverrides.value = nextOverrides
}

function expandRelatedTopics(slug: string) {
  const relatedSlugs = collectRelatedTopicSlugs(viewModel.value.graph, slug)
  const nextExpanded = new Set(expandedTopicSlugs.value)
  const nextOverrides = { ...graphVisibilityOverrides.value }

  nextExpanded.add(slug)
  nextOverrides[slug] = true

  relatedSlugs.forEach((relatedSlug) => {
    nextExpanded.add(relatedSlug)
    nextOverrides[relatedSlug] = true
  })

  expandedTopicSlugs.value = Array.from(nextExpanded)
  graphVisibilityOverrides.value = nextOverrides
}

// Hotspot categories now use data from getTopicsByCategory API
// Hotspot search state
const hotspotSearchQueries = ref<Record<string, string>>({ event: '', person: '', keyword: '' })
const hotspotDropdownOpen = ref<Record<string, boolean>>({ event: false, person: false, keyword: false })
const hotspotShowAll = ref<Record<string, boolean>>({ event: false, person: false, keyword: false })

// Refs for hotspot search containers
const hotspotSearchRefs = ref<Record<string, HTMLDivElement | null>>({ event: null, person: null, keyword: null })

// Filter topics based on search query
function filterTopics(topics: any[], query: string) {
  if (!query.trim()) return topics
  const lowerQuery = query.toLowerCase()
  return topics.filter(topic =>
    topic.label.toLowerCase().includes(lowerQuery) ||
    topic.slug.toLowerCase().includes(lowerQuery)
  )
}

function isDefaultVisibleTopic(topic: { is_low_quality?: boolean; is_abstract?: boolean }) {
  return topic.is_abstract || !topic.is_low_quality
}

function sortTopicsByFrequency<T extends { score: number; quality_score?: number; is_low_quality?: boolean }>(topics: T[]) {
  return [...topics].sort((left, right) => {
    // 低质量标签排在底部
    const leftLowQuality = left.is_low_quality ? 1 : 0
    const rightLowQuality = right.is_low_quality ? 1 : 0
    if (leftLowQuality !== rightLowQuality) {
      return leftLowQuality - rightLowQuality
    }

    const leftQuality = left.quality_score ?? left.score ?? 0
    const rightQuality = right.quality_score ?? right.score ?? 0
    if (rightQuality === leftQuality) {
      return (right.score ?? 0) - (left.score ?? 0)
    }
    return rightQuality - leftQuality
  })
}

function buildCategoryTopicState(category: TopicCategory, query: string, showAll: boolean) {
  const categoryTopics = category === 'event'
    ? hotspotData.value?.events
    : category === 'person'
      ? hotspotData.value?.people
      : hotspotData.value?.keywords
  const sourceTopics = sortTopicsByFrequency(categoryTopics || buildFallbackTopics(category))
  const filteredTopics = filterTopics(sourceTopics, query || '')
  const displayTopics = showAll ? filteredTopics : filteredTopics.slice(0, 8)

  return {
    topics: filteredTopics,
    filteredTopics,
    displayTopics,
    hasMore: filteredTopics.length > displayTopics.length,
    hiddenLowQualityCount: filteredTopics.filter(topic => topic.is_low_quality && !topic.is_abstract).length,
    showAll,
  }
}

function buildFallbackTopics(category: TopicCategory) {
  return sortTopicsByFrequency(
    viewModel.value.topTopics.filter(topic => normalizeTopicCategory(topic.category, topic.kind) === category)
  )
}

// Toggle show all topics
function toggleShowAll(categoryKey: string) {
  hotspotShowAll.value[categoryKey] = !hotspotShowAll.value[categoryKey]
}

// Close specific dropdown
function closeHotspotDropdown(categoryKey: string) {
  hotspotDropdownOpen.value[categoryKey] = false
}

// Close all dropdowns when clicking outside
function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node
  
  Object.keys(hotspotSearchRefs.value).forEach((key) => {
    const container = hotspotSearchRefs.value[key]
    if (container && !container.contains(target)) {
      hotspotDropdownOpen.value[key] = false
    }
  })
}

// Add/remove click outside listener
watch(() => Object.values(hotspotDropdownOpen.value).some(Boolean), (isAnyOpen) => {
  if (isAnyOpen) {
    document.addEventListener('click', handleClickOutside, true)
  } else {
    document.removeEventListener('click', handleClickOutside, true)
  }
})

// Cleanup on unmount
onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside, true)
})

const hotspotCategories = computed(() => ([
  {
    key: 'event',
    label: '事件',
    icon: 'mdi:calendar-alert-outline',
    headerClass: 'topic-category-header--event',
    ...buildCategoryTopicState('event', hotspotSearchQueries.value.event || '', !!hotspotShowAll.value.event),
  },
  {
    key: 'person',
    label: '人物',
    icon: 'mdi:account-voice-outline',
    headerClass: 'topic-category-header--person',
    ...buildCategoryTopicState('person', hotspotSearchQueries.value.person || '', !!hotspotShowAll.value.person),
  },
  {
    key: 'keyword',
    label: '关键词',
    icon: 'mdi:key-variant',
    headerClass: 'topic-category-header--keyword',
    ...buildCategoryTopicState('keyword', hotspotSearchQueries.value.keyword || '', !!hotspotShowAll.value.keyword),
  },
]))
const defaultGraphTopicSlugs = computed(() => {
  const slugs = new Set<string>()
  const lowQualitySlugs = new Set<string>()

  viewModel.value.topTopics.forEach((topic) => {
    if (topic.is_low_quality && topic.slug) {
      lowQualitySlugs.add(topic.slug)
    }
  })

  viewModel.value.graph.nodes.forEach((node) => {
    if (node.kind === 'topic' && node.slug && !lowQualitySlugs.has(node.slug)) {
      slugs.add(node.slug)
    }
  })

  return slugs
})
const graphVisibleTopicSlugs = computed(() => {
  const slugs = new Set(defaultGraphTopicSlugs.value)

  expandedTopicSlugs.value.forEach(slug => slugs.add(slug))

  Object.entries(graphVisibilityOverrides.value).forEach(([slug, visible]) => {
    if (visible) {
      slugs.add(slug)
      return
    }

    slugs.delete(slug)
  })

  return slugs
})
const displayedGraph = computed(() => buildDisplayedTopicGraph({
  graph: viewModel.value.graph,
  visibleTopicSlugs: graphVisibleTopicSlugs.value,
}))
const timelineItems = computed((): TimelineDigest[] => {
  const summaries = detail.value?.summaries || []

  return summaries.map(summary => ({
    id: String(summary.id),
    title: summary.title,
    summary: summary.summary,
    createdAt: summary.created_at,
    feedName: summary.feed_name,
    feedIcon: summary.feed_icon,
    categoryName: summary.category_name,
    articleCount: summary.article_count,
    tags: summary.aggregated_tags.map(topic => ({
      slug: topic.slug,
      label: topic.label,
      category: normalizeTopicCategory(topic.category, topic.kind),
    })),
    articles: summary.articles.map(article => ({
      id: article.id,
      title: article.title,
      link: article.link,
    })),
  }))
})

// Hotspot digests converted to TimelineDigest format for display
const hotspotTimelineItems = computed((): TimelineDigest[] => {
  if (!hotspotDigests.value.length) return []

  return hotspotDigests.value.map(digest => ({
      id: String(digest.id),
      title: digest.title,
      summary: digest.summary,
      createdAt: digest.created_at,
      feedName: digest.feed_name,
      feedIcon: digest.feed_icon,
      categoryName: digest.category_name,
      articleCount: digest.article_count,
      tags: (digest.aggregated_tags || []).map(tag => ({
        slug: tag.slug,
        label: tag.label,
        category: normalizeTopicCategory(tag.category, tag.kind),
      })),
      articles: digest.matched_articles?.map(article => ({
        id: article.id,
        title: article.title,
        link: '',
        feedName: article.feed_name,
        feedIcon: article.feed_icon,
        feedColor: article.feed_color,
      })) || [],
    }))
})

// Effective timeline items: use hotspot digests when available, otherwise use topic detail summaries
// This allows hotspot tag clicks to show digests containing articles with that tag
const effectiveTimelineItems = computed((): TimelineDigest[] => {
  // When a hotspot tag is selected and we have hotspot digests, prioritize them
  if (selectedHotspotTag.value && hotspotDigests.value.length > 0) {
    return hotspotTimelineItems.value
  }
  // Otherwise use the topic detail summaries
  return timelineItems.value
})
const selectedDigest = computed<TimelineDigestSelection | null>(() => {
  if (!selectedDigestId.value) return null
  const digest = effectiveTimelineItems.value.find(item => item.id === selectedDigestId.value)
  if (!digest) return null

  // For hotspot digests, use hotspot tag to fetch matched articles
  if (selectedHotspotTag.value && hotspotDigests.value.length > 0) {
    const hotspotDigest = hotspotDigests.value.find(d => String(d.id) === selectedDigestId.value)
    if (hotspotDigest) {
      return {
        ...digest,
        matchedArticleIds: hotspotDigest.matched_articles?.map(a => a.id) || [],
        matchedArticlesTags: hotspotDigest.matched_articles_tags?.map(tag => ({
          slug: tag.slug,
          label: tag.label,
          category: normalizeTopicCategory(tag.category, tag.kind),
        })),
      }
    }
  }

  // For topic detail digests, match against topic articles
  if (!detail.value) return null
  const topicArticleIds = new Set(detail.value.articles.map(article => article.id))
  const matchedArticleIds = digest.articles
    .map(article => article.id)
    .filter(id => topicArticleIds.has(id))

  return {
    ...digest,
    matchedArticleIds,
  }
})
const previewDigest = computed(() => {
  if (!previewDigestId.value) return null
  return effectiveTimelineItems.value.find(item => item.id === previewDigestId.value) || null
})
function buildCurrentFilters(): TopicGraphFilters | undefined {
  if (selectedFilterFeedId.value) return { feedId: selectedFilterFeedId.value }
  if (selectedFilterCategoryId.value && selectedFilterCategoryId.value !== '__uncategorized__') {
    return { categoryId: selectedFilterCategoryId.value }
  }
  return undefined
}

const pageState = computed(() => {
  if (loadingGraph.value) return 'loading'
  if (selectedPreviewArticle.value) return 'article-preview'
  if (detail.value) return 'detail'
  if (graphPayload.value) return 'graph-ready'
  return 'empty'
})

const selectedTopicInfo = computed(() => {
  if (detail.value?.topic) {
    return {
      id: detail.value.topic.id,
      slug: detail.value.topic.slug,
      label: detail.value.topic.label,
      category: normalizeTopicCategory(detail.value.topic.category, detail.value.topic.kind),
      description: detail.value.topic.description,
    }
  }

  if (!selectedTopicSlug.value) return null

  const topic = viewModel.value.topTopics.find(item => item.slug === selectedTopicSlug.value)
  if (!topic) return null

    return {
      id: topic.id ?? 0,
      slug: topic.slug,
      label: topic.label,
      category: normalizeTopicCategory(topic.category, topic.kind),
      description: topic.description,
    }
})

async function loadHotspots() {
  loadingHotspots.value = true
  try {
    const response = await topicGraphApi.getTopicsByCategory(selectedType.value, selectedDate.value, buildCurrentFilters())
    if (response.success && response.data) {
      hotspotData.value = response.data
    } else {
      console.error('Failed to load hotspots:', response.error)
      hotspotData.value = null
    }
  } catch (error) {
    console.error('Failed to load hotspots:', error)
    hotspotData.value = null
  } finally {
    loadingHotspots.value = false
  }
}

async function loadGraph() {
  loadingGraph.value = true
  notice.value = null

  try {
    const response = await topicGraphApi.getGraph(selectedType.value, selectedDate.value, buildCurrentFilters())
    if (!response.success || !response.data) {
      notice.value = response.error || '主题图谱没拉下来'
      graphPayload.value = null
      detail.value = null
      return
    }

    graphPayload.value = response.data
    selectedTopicSlug.value = response.data.top_topics[0]?.slug || null
    selectedCategory.value = response.data.top_topics[0]
      ? normalizeTopicCategory(response.data.top_topics[0].category, response.data.top_topics[0].kind)
      : null
    selectedKeywordSlug.value = null
    selectedDigestId.value = null
    previewDigestId.value = null
    expandedTopicSlugs.value = []

    if (selectedTopicSlug.value) {
      const firstTopic = response.data.top_topics[0]
      selectedHotspotTag.value = {
        slug: selectedTopicSlug.value,
        label: firstTopic?.label || selectedTopicSlug.value,
        category: selectedCategory.value || 'keyword',
      }
      void loadTopicDetail(selectedTopicSlug.value)
      void loadHotspotDigests(selectedTopicSlug.value, selectedCategory.value || undefined)
      void loadPendingArticles(selectedTopicSlug.value)
    } else {
      detail.value = null
      selectedHotspotTag.value = null
      hotspotDigests.value = []
    }

    // Load hotspot data in parallel
    void loadHotspots()
  } catch (error) {
    console.error('Failed to load topic graph:', error)
    notice.value = error instanceof Error ? error.message : '主题图谱加载失败'
  } finally {
    loadingGraph.value = false
  }
}

async function loadTopicDetail(slug: string) {
  selectedTopicSlug.value = slug
  selectedKeywordSlug.value = null
  selectedDigestId.value = null
  previewDigestId.value = null
  const topic = viewModel.value.topTopics.find(item => item.slug === slug) || null
  if (topic?.category) {
    selectedCategory.value = normalizeTopicCategory(topic.category, topic.kind)
  }
  loadingDetail.value = true

  try {
    const response = await topicGraphApi.getTopicDetail(slug, undefined, undefined, buildCurrentFilters())
    if (response.success && response.data) {
      detail.value = response.data
      selectedCategory.value = normalizeTopicCategory(response.data.topic.category, response.data.topic.kind)
      selectedDigestId.value = null
      previewDigestId.value = null
      return
    }

    detail.value = null
    notice.value = response.error || '话题详情加载失败'
  } catch (error) {
    console.error('Failed to load topic detail:', error)
    detail.value = null
    notice.value = error instanceof Error ? error.message : '话题详情加载失败'
  } finally {
    loadingDetail.value = false
  }
}

async function handleTagSelect(slug: string, category: TopicCategory) {
  ensureTopicShownInGraph(slug)
  expandRelatedTopics(slug)
  graphFocusRequestKey.value += 1
  selectedCategory.value = category
  selectedTopicSlug.value = slug

  // Find the tag label from hotspot data
  let tagLabel = slug
  const allTags = [
    ...(hotspotData.value?.events || []),
    ...(hotspotData.value?.people || []),
    ...(hotspotData.value?.keywords || []),
  ]
  const foundTag = allTags.find(t => t.slug === slug)
  if (foundTag) {
    tagLabel = foundTag.label
  }

  // Check if this is an abstract tag
  const isAbstract = foundTag?.is_abstract ?? false
  const childSlugs = foundTag?.child_slugs ?? []
  abstractNodeSlug.value = isAbstract ? slug : null
  abstractNodeLabel.value = isAbstract ? tagLabel : null

  // Update selected hotspot tag
  selectedHotspotTag.value = { slug, label: tagLabel, category }

  // Reset pending node selection when selecting a new tag
  selectedPendingNode.value = false

  // For abstract tags, load digests for all child tags
  if (isAbstract && childSlugs.length > 0) {
    await loadAbstractTagDigests(childSlugs)
  } else {
    // Load digests for this tag (reverse trace: tag -> articles -> digests)
    await loadHotspotDigests(slug, category)
  }

  // Load pending articles for this tag
  void loadPendingArticles(slug)

  // Load aggregated articles for timeline
  void loadAggregatedArticles(slug)

  // Also load topic detail for the sidebar
  void loadTopicDetail(slug)
}

async function loadHotspotDigests(tagSlug: string, kind?: TopicCategory) {
  loadingHotspotDigests.value = true
  try {
    const response = await topicGraphApi.getDigestsByArticleTag(
      tagSlug,
      undefined,
      undefined,
      20,
      kind,
    )
    if (response.success && response.data) {
      hotspotDigests.value = response.data.digests || []
    } else {
      hotspotDigests.value = []
      console.error('Failed to load hotspot digests:', response.error)
    }
  } catch (error) {
    console.error('Failed to load hotspot digests:', error)
    hotspotDigests.value = []
  } finally {
    loadingHotspotDigests.value = false
  }
}

async function loadAbstractTagDigests(childSlugs: string[]) {
  loadingHotspotDigests.value = true
  try {
    const results = await Promise.all(
      childSlugs.map(slug =>
        topicGraphApi.getDigestsByArticleTag(slug, undefined, undefined, 20)
      )
    )
    const seenIds = new Set<number>()
    const merged: HotspotDigestCard[] = []
    for (const response of results) {
      if (!response.success || !response.data) continue
      for (const digest of response.data.digests || []) {
        if (!seenIds.has(digest.id)) {
          seenIds.add(digest.id)
          merged.push(digest)
        }
      }
    }
    merged.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    hotspotDigests.value = merged
  } catch (error) {
    console.error('Failed to load abstract tag digests:', error)
    hotspotDigests.value = []
  } finally {
    loadingHotspotDigests.value = false
  }
}

async function handleChildTagSelect(childSlug: string, childLabel: string) {
  selectedChildTagSlug.value = childSlug

  // Update selectedHotspotTag to child tag for proper filtering
  selectedHotspotTag.value = {
    slug: childSlug,
    label: childLabel,
    category: 'keyword',
  }

  // Load digests for the child tag (not the abstract parent)
  await loadHotspotDigests(childSlug, selectedHotspotTag.value.category)

  // Open timeline panel if not already open
  if (!timelineOpen.value) {
    timelineOpen.value = true
  }

  // Load pending articles for the child tag
  void loadPendingArticles(childSlug)

  // Load aggregated articles for timeline
  void loadAggregatedArticles(childSlug)

  // Load topic detail for the sidebar
  void loadTopicDetail(childSlug)

  // Ensure the child tag is shown in the graph
  ensureTopicShownInGraph(childSlug)
  expandRelatedTopics(childSlug)
  graphFocusRequestKey.value += 1

  // Reset pending node selection
  selectedPendingNode.value = false
}

function handleNarrativeTagSelect(tag: { id: number; slug: string; label: string; category: string; kind?: string }) {
  const category = normalizeTopicCategory(tag.category as TopicCategory, tag.kind as TopicKind | undefined)
  handleTagSelect(tag.slug, category)
}

async function handleAbstractTagSelect(abstractSlug: string) {
  selectedChildTagSlug.value = null

  // Find abstract tag info
  const allTags = [
    ...(hotspotData.value?.events || []),
    ...(hotspotData.value?.people || []),
    ...(hotspotData.value?.keywords || []),
  ]
  const abstractTag = allTags.find(t => t.slug === abstractSlug)
  const abstractLabel = abstractTag?.label || abstractSlug
  const childSlugs = abstractTag?.child_slugs || []

  // Update selectedHotspotTag to abstract tag
  selectedHotspotTag.value = {
    slug: abstractSlug,
    label: abstractLabel,
    category: abstractTag ? normalizeTopicCategory(abstractTag.category, abstractTag.kind) : 'keyword',
  }

  // Load digests for all child tags (aggregate view)
  if (childSlugs.length > 0) {
    await loadAbstractTagDigests(childSlugs)
  } else {
    await loadHotspotDigests(abstractSlug, selectedHotspotTag.value.category)
  }

  // Open timeline panel if not already open
  if (!timelineOpen.value) {
    timelineOpen.value = true
  }

  // Load pending articles for the abstract tag
  void loadPendingArticles(abstractSlug)

  // Load aggregated articles for timeline
  void loadAggregatedArticles(abstractSlug)

  // Load topic detail for the sidebar
  void loadTopicDetail(abstractSlug)

  // Ensure the abstract tag is shown in the graph
  ensureTopicShownInGraph(abstractSlug)
  expandRelatedTopics(abstractSlug)
  graphFocusRequestKey.value += 1

  // Reset pending node selection
  selectedPendingNode.value = false
}

async function loadPendingArticles(tagSlug: string) {
  loadingPendingArticles.value = true
  try {
    const response = await topicGraphApi.getPendingArticlesByTag(
      tagSlug
    )
    if (response.success && response.data) {
      pendingArticles.value = (response.data.articles || []).map(article => ({
        id: article.id,
        title: article.title,
        link: article.link,
        pubDate: (article as any).pub_date || article.pubDate,
        feedName: (article as any).feed_name || article.feedName,
        feedIcon: (article as any).feed_icon || article.feedIcon,
        feedColor: (article as any).feed_color || article.feedColor,
      }))
    } else {
      pendingArticles.value = []
    }
  } catch (error) {
    console.error('Failed to load pending articles:', error)
    pendingArticles.value = []
  } finally {
    loadingPendingArticles.value = false
  }
}

async function loadAggregatedArticles(tagSlug: string) {
  loadingAggregatedArticles.value = true
  try {
    const isAbstract = hotspotData.value && [
      ...hotspotData.value.events,
      ...hotspotData.value.people,
      ...hotspotData.value.keywords,
    ].find(t => t.slug === tagSlug)?.is_abstract

    const childSlugs = hotspotData.value && [
      ...hotspotData.value.events,
      ...hotspotData.value.people,
      ...hotspotData.value.keywords,
    ].find(t => t.slug === tagSlug)?.child_slugs

    const slugs = (isAbstract && childSlugs?.length) ? childSlugs : [tagSlug]

    const allArticles: TimelineAggregationArticle[] = []
    const seenIds = new Set<string>()

    for (const slug of slugs) {
      const response = await topicGraphApi.getTopicArticles({
        slug,
        page: 1,
        pageSize: 100,
      })
      if (response.success && response.data) {
        for (const article of response.data.articles) {
          if (!seenIds.has(article.id)) {
            seenIds.add(article.id)
            allArticles.push({
              id: article.id,
              title: article.title,
              link: article.link,
              pubDate: article.pub_date,
              feedName: article.feed_name,
              feedIcon: article.image_url || '',
              tags: (article.tags || []).map(t => ({
                slug: t.slug,
                label: t.label,
                category: t.category,
              })),
            })
          }
        }
      }
    }

    allArticles.sort((a, b) => new Date(b.pubDate).getTime() - new Date(a.pubDate).getTime())
    aggregatedArticles.value = allArticles
  } catch (error) {
    console.error('Failed to load aggregated articles:', error)
    aggregatedArticles.value = []
  } finally {
    loadingAggregatedArticles.value = false
  }
}

const timelineAggregationGroups = computed((): TimelineAggregationGroup[] => {
  if (!aggregatedArticles.value.length) return []

  const groups: TimelineAggregationGroup[] = []
  const groupMap = new Map<string, TimelineAggregationArticle[]>()

  for (const article of aggregatedArticles.value) {
    const date = new Date(article.pubDate)
    if (Number.isNaN(date.getTime())) continue

    let key: string
    let startDate: Date
    let endDate: Date
    let label: string

    if (aggregationMode.value === 'hour') {
      const hour = date.getHours()
      key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}-${hour}`
      startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour, 0, 0)
      endDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour + 1, 0, 0)
      const startFmt = `${String(hour).padStart(2, '0')}:00`
      const endFmt = `${String(hour + 1).padStart(2, '0')}:00`
      label = `${startFmt} - ${endFmt}`
    } else {
      key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
      startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), 0, 0, 0)
      endDate = new Date(date.getFullYear(), date.getMonth(), date.getDate() + 1, 0, 0, 0)
      label = new Intl.DateTimeFormat('zh-CN', {
        month: 'short',
        day: 'numeric',
        weekday: 'short',
      }).format(startDate)
    }

    if (!groupMap.has(key)) {
      groupMap.set(key, [])
    }
    groupMap.get(key)!.push(article)
  }

  const sortedKeys = Array.from(groupMap.keys()).sort((a, b) => b.localeCompare(a))

  for (const key of sortedKeys) {
    const articles = groupMap.get(key)!
    if (!articles.length) continue
    const first = articles[0]!
    const date = new Date(first.pubDate)

    let startDate: Date
    let endDate: Date

    if (aggregationMode.value === 'hour') {
      const hour = date.getHours()
      startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour, 0, 0)
      endDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour + 1, 0, 0)
    } else {
      startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate(), 0, 0, 0)
      endDate = new Date(date.getFullYear(), date.getMonth(), date.getDate() + 1, 0, 0, 0)
    }

    groups.push({
      key,
      label: '',
      startDate,
      endDate,
      articles,
    })
  }

  return groups
})

const totalAggregatedCount = computed(() => aggregatedArticles.value.length)

function handleTimelineGroupSelect(groupKey: string) {
  selectedGroupKey.value = groupKey
  selectedPendingNode.value = false
  selectedDigestId.value = null
}

function handleAggregationModeChange(mode: TimelineAggregationMode) {
  aggregationMode.value = mode
  selectedGroupKey.value = null
}

const selectedGroupArticles = computed((): TimelineAggregationArticle[] => {
  if (!selectedGroupKey.value) return aggregatedArticles.value
  const group = timelineAggregationGroups.value.find(g => g.key === selectedGroupKey.value)
  return group?.articles || []
})

function handleSelectPending() {
  selectedPendingNode.value = true
  selectedDigestId.value = null
  previewDigestId.value = null
}

function handleNodeClick(node: { slug?: string; kind: string; category?: TopicCategory; label?: string; isAbstract?: boolean }) {
  if (node.kind !== 'topic' || !node.slug) return

  ensureTopicShownInGraph(node.slug)
  expandRelatedTopics(node.slug)
  graphFocusRequestKey.value += 1

  if (node.category) {
    selectedCategory.value = node.category
  }

  // Track whether this is an abstract tag node
  abstractNodeSlug.value = node.isAbstract ? node.slug : null
  abstractNodeLabel.value = node.isAbstract ? (node.label || node.slug) : null

  // Set hotspot tag state for the clicked node and load its digests
  selectedHotspotTag.value = {
    slug: node.slug,
    label: node.label || node.slug,
    category: node.category || 'keyword',
  }

  // Reset pending node selection when clicking a node
  selectedPendingNode.value = false

  // Load digests for this node (similar to handleTagSelect)
  void loadHotspotDigests(node.slug, node.category)

  // Load pending articles for this node
  void loadPendingArticles(node.slug)

  // Load aggregated articles for timeline
  void loadAggregatedArticles(node.slug)

  void loadTopicDetail(node.slug)
}

function handleKeywordHighlight(keywordSlug: string | null) {
  if (!keywordSlug) {
    selectedKeywordSlug.value = null
    return
  }

  const existsInGraph = displayedGraph.value.nodes.some(node => node.kind === 'topic' && node.slug === keywordSlug)
  selectedKeywordSlug.value = existsInGraph ? keywordSlug : null
}

function handleDigestSelect(digestId: string) {
  selectedDigestId.value = digestId
  selectedPendingNode.value = false
}

function handlePreviewDigest(digestId: string) {
  selectedDigestId.value = digestId
  previewDigestId.value = digestId
  selectedPendingNode.value = false
}

function closeDigestPreview() {
  previewDigestId.value = null
}

async function openArticlePreview(articleId: number) {
  previewDigestId.value = null
  loadingPreviewArticle.value = true

  try {
    const response = await articlesApi.getArticle(articleId)
    if (!response.success || !response.data) {
      notice.value = response.error || '文章预览加载失败'
      return
    }

    selectedPreviewArticle.value = normalizeArticle(response.data as unknown as ArticlePayload)

    if (detail.value?.summaries) {
      const ids = detail.value.summaries.flatMap(summary => summary.articles.map(article => article.id))
      const uniqueIds = Array.from(new Set(ids))
      const articleResponses = await Promise.all(uniqueIds.slice(0, 12).map(id => articlesApi.getArticle(id)))
      previewArticles.value = articleResponses
        .filter(item => item.success && item.data)
        .map(item => normalizeArticle(item.data as unknown as ArticlePayload))
    }
  } catch (error) {
    console.error('Failed to open article preview:', error)
    notice.value = error instanceof Error ? error.message : '文章预览加载失败'
  } finally {
    loadingPreviewArticle.value = false
  }
}

function closeArticlePreview() {
  selectedPreviewArticle.value = null
}

async function handleArticleFavorite(articleId: string) {
  const currentFavorite = selectedPreviewArticle.value?.id === articleId
    ? selectedPreviewArticle.value.favorite
    : previewArticles.value.find(a => a.id === articleId)?.favorite

  const response = await articlesApi.updateArticle(Number(articleId), { favorite: !currentFavorite })
  if (!response.success) return

  const target = previewArticles.value.find(article => article.id === articleId)
  if (target) {
    target.favorite = !target.favorite
  }

  if (selectedPreviewArticle.value?.id === articleId) {
    selectedPreviewArticle.value = {
      ...selectedPreviewArticle.value,
      favorite: !selectedPreviewArticle.value.favorite,
    }
  }
}

function handleArticleUpdate(articleId: string, updates: Partial<Article>) {
  const target = previewArticles.value.find(article => article.id === articleId)
  if (target) {
    Object.assign(target, updates)
  }

  if (selectedPreviewArticle.value?.id === articleId) {
    Object.assign(selectedPreviewArticle.value, updates)
  }
}

watch(selectedType, () => {
  resetTimelinePanelPosition()
  void loadGraph()
})

watch(selectedDate, () => {
  resetTimelinePanelPosition()
  void loadGraph()
})

watch([selectedFilterCategoryId, selectedFilterFeedId], () => {
  void loadGraph()
})

watch(effectiveTimelineItems, (items) => {
  if (!items.length) {
    selectedDigestId.value = null
    previewDigestId.value = null
    selectedPendingNode.value = false
    return
  }

  const currentExists = selectedDigestId.value && items.some(item => item.id === selectedDigestId.value)
  if (!currentExists) {
    selectedDigestId.value = items[0]?.id || null
    selectedPendingNode.value = false
  }
}, { immediate: true })

watch(timelineAggregationGroups, (groups) => {
  if (!groups.length) {
    selectedGroupKey.value = null
    return
  }
  const currentExists = selectedGroupKey.value && groups.some(g => g.key === selectedGroupKey.value)
  if (!currentExists) {
    selectedGroupKey.value = groups[0]?.key || null
  }
})

await loadGraph()
</script>

<template>
  <div
    class="topic-stage min-h-screen px-4 py-5 md:px-6 md:py-7"
    data-testid="topic-graph-page"
    :data-state="pageState"
  >
    <div class="topic-shell mx-auto w-full">
      <section class="topic-layout grid gap-5 2xl:grid-cols-[minmax(0,2.15fr)_minmax(430px,0.95fr)]">
        <div class="space-y-5">
          <article class="topic-canvas-shell rounded-[34px] p-4 md:p-5">
            <div class="topic-studio grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
              <aside class="topic-studio__rail rounded-[30px] p-4 md:p-5">
                <TopicGraphHeader
                  :selected-type="selectedType"
                  :selected-date="selectedDate"
                  :loading="loadingGraph"
                  :hero-label="viewModel.stats.heroLabel"
                  :hero-subline="viewModel.stats.heroSubline"
                  @update:type="selectedType = $event"
                  @update:date="selectedDate = $event"
                  @refresh="loadGraph"
                />

                <div class="mt-6">
                  <p class="text-xs uppercase tracking-[0.3em] text-white/42">Graph Field</p>
                  <h2 class="mt-2 font-serif text-2xl text-white md:text-[2.25rem]">{{ graphPayload?.period_label || '话题网络' }}</h2>
                  <p class="mt-3 text-sm leading-6 text-[rgba(255,255,255,0.68)]">
                     事件、人物、关键词的热点题材默认全部进入拓扑图；底部可单独控制各节点的显示与隐藏。
                  </p>
                </div>

                <FeedCategoryFilter
                  :selected-category-id="selectedFilterCategoryId"
                  :selected-feed-id="selectedFilterFeedId"
                  @update:selected-category-id="selectedFilterCategoryId = $event"
                  @update:selected-feed-id="selectedFilterFeedId = $event"
                />


                <!-- View tabs -->
                <div class="mt-4 flex gap-1.5">
                  <button
                    type="button"
                    class="th-tab-btn"
                    :class="{ 'th-tab-btn--active': activeTab === 'graph' }"
                    @click="activeTab = 'graph'"
                  >
                    <Icon icon="mdi:graph-outline" width="14" />
                    <span>图谱</span>
                  </button>
                  <button
                    type="button"
                    class="th-tab-btn"
                    :class="{ 'th-tab-btn--active': activeTab === 'hierarchy' }"
                    @click="activeTab = 'hierarchy'"
                  >
                    <Icon icon="mdi:file-tree-outline" width="14" />
                    <span>标签层级</span>
                  </button>
                  <button
                    type="button"
                    class="th-tab-btn"
                    :class="{ 'th-tab-btn--active': activeTab === 'narrative' }"
                    @click="activeTab = 'narrative'"
                  >
                    <Icon icon="mdi:book-open-page-variant-outline" width="14" />
                    <span>叙事</span>
                  </button>
                </div>

              </aside>

              <div class="space-y-4">
                <!-- Graph view (default) -->
                <template v-if="activeTab === 'graph'">
                <TopicGraphCanvas
                  :nodes="displayedGraph.nodes"
                  :edges="displayedGraph.edges"
                  :featured-node-ids="displayedGraph.featuredNodeIds"
                  :active-node-id="activeTopicNode?.id || null"
                  :focus-request-key="graphFocusRequestKey"
                  :selected-category="selectedCategory"
                  :highlighted-node-ids="highlightedNodeIds"
                  :related-edge-ids="relatedEdgeIds"
                  @node-click="handleNodeClick"
                />

                <article class="topic-note rounded-[30px] px-5 py-4 text-sm leading-6 text-[rgba(255,255,255,0.78)]">
                  <div class="flex items-start gap-3">
                    <Icon icon="mdi:orbit-variant" width="20" height="20" class="mt-1 text-[rgba(240,138,75,0.92)]" />
                    <p>
                      先看结构，再读内容：亮色主节点是当前焦点，周边只保留一跳关系的高亮，更多细节放到右侧阅读栏。
                    </p>
                  </div>
                </article>

                <section class="topic-hotspot-strip rounded-[30px] p-4 md:p-5">
                  <div class="topic-hotspot-strip__header">
                    <div>
                      <p class="text-xs uppercase tracking-[0.24em] text-white/42">热点题材</p>
                      <h3 class="mt-2 font-serif text-xl text-white">把最热的话题平铺到底部，避免和左侧控制重复。</h3>
                    </div>
                  </div>

                  <div class="mt-4 grid gap-3 xl:grid-cols-3">
                    <section
                      v-for="category in hotspotCategories"
                      :key="category.key"
                      class="topic-category-column rounded-[22px] p-3"
                      :data-testid="`hotspot-category-${category.key}`"
                    >
                      <div class="topic-category-header" :class="category.headerClass">
                        <Icon :icon="category.icon" width="14" />
                        <span>{{ category.label }}</span>
                        <span class="topic-count">({{ category.topics.length }})</span>
                      </div>

                        <!-- Search Input -->
                      <div :ref="el => { if (el) hotspotSearchRefs[category.key] = el as HTMLDivElement }" class="topic-search-wrapper mt-3">
                        <div class="topic-search-input-wrapper" @click="hotspotDropdownOpen[category.key] = true">
                          <Icon icon="mdi:magnify" width="14" class="topic-search-icon" />
                          <input
                            v-model="hotspotSearchQueries[category.key]"
                            type="text"
                            class="topic-search-input"
                            placeholder="搜索..."
                            @focus="hotspotDropdownOpen[category.key] = true"
                          />
                          <button
                            v-if="hotspotSearchQueries[category.key]"
                            class="topic-search-clear"
                            @click.stop="hotspotSearchQueries[category.key] = ''"
                          >
                            <Icon icon="mdi:close" width="12" />
                          </button>
                        </div>

                        <!-- Dropdown -->
                        <div
                          v-if="hotspotDropdownOpen[category.key] && category.filteredTopics.length > 0"
                          class="topic-search-dropdown"
                          @mousedown.prevent
                        >
                          <div class="topic-dropdown-scroll">
                            <div
                              v-for="topic in category.displayTopics"
                              :key="topic.slug"
                              class="topic-dropdown-row"
                            >
                              <button
                                type="button"
                                class="topic-dropdown-item"
                                :class="{
                                  'topic-dropdown-item--active': selectedTopicSlug === topic.slug,
                                  'topic-dropdown-item--abstract': topic.is_abstract,
                                }"
                                @click="handleTagSelect(topic.slug, normalizeTopicCategory(topic.category, topic.kind)); hotspotDropdownOpen[category.key] = false"
                              >
                                <Icon v-if="topic.is_abstract" icon="mdi:tag-group" width="14" class="text-amber-400/80" />
                                <Icon v-else-if="topic.icon" :icon="topic.icon" width="14" />
                                <span>{{ topic.label }}</span>
                                <span v-if="topic.is_abstract" class="ml-1 text-[10px] text-amber-400/60">({{ topic.child_slugs?.length ?? 0 }})</span>
                                <span v-else-if="topic.is_low_quality" class="ml-1 text-[10px] text-white/35">低质量</span>
                              </button>
                              <button
                                type="button"
                                class="topic-graph-toggle"
                                :class="{ 'topic-graph-toggle--active': isTopicShownInGraph(topic.slug) }"
                                :aria-label="isTopicShownInGraph(topic.slug) ? `从拓扑图隐藏 ${topic.label}` : `在拓扑图展示 ${topic.label}`"
                                :title="isTopicShownInGraph(topic.slug) ? '从拓扑图隐藏' : '在拓扑图展示'"
                                @click.stop="toggleTopicGraphVisibility(topic.slug)"
                              >
                                <Icon :icon="isTopicShownInGraph(topic.slug) ? 'mdi:eye-outline' : 'mdi:eye-off-outline'" width="14" />
                              </button>
                            </div>
                          </div>
                          <button
                            v-if="category.hasMore"
                            class="topic-dropdown-toggle"
                            @click="toggleShowAll(category.key)"
                          >
                            <Icon :icon="category.showAll ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="16" />
                            {{ category.showAll ? '恢复默认筛选' : `显示全部标签 (${category.filteredTopics.length})` }}
                          </button>
                        </div>

                        <!-- No Results -->
                        <div
                          v-if="hotspotDropdownOpen[category.key] && hotspotSearchQueries[category.key] && category.filteredTopics.length === 0"
                          class="topic-search-no-results"
                        >
                          未找到匹配的结果
                          <button
                            class="topic-dropdown-close"
                            @click.stop="closeHotspotDropdown(category.key)"
                          >
                            关闭
                          </button>
                        </div>
                      </div>

                      <!-- Quick Tags (show top 5 without search) -->
                      <div v-if="!hotspotSearchQueries[category.key]" class="topic-quick-tags mt-3">
                        <div
                          v-for="topic in category.topics.slice(0, 5)"
                          :key="topic.slug"
                          class="topic-badge-row"
                        >
                          <button
                            type="button"
                            class="topic-badge text-left"
                            :class="{
                              'topic-badge--event': normalizeTopicCategory(topic.category, topic.kind) === 'event',
                              'topic-badge--person': normalizeTopicCategory(topic.category, topic.kind) === 'person',
                              'topic-badge--keyword': normalizeTopicCategory(topic.category, topic.kind) === 'keyword',
                              'topic-badge--active': selectedTopicSlug === topic.slug,
                              'topic-badge--abstract': topic.is_abstract,
                            }"
                            @click="handleTagSelect(topic.slug, normalizeTopicCategory(topic.category, topic.kind))"
                          >
                            <Icon v-if="topic.is_abstract" icon="mdi:tag-group" width="14" class="text-amber-400/80" />
                            <Icon v-else-if="topic.icon" :icon="topic.icon" width="14" />
                            {{ topic.label }}
                            <span v-if="topic.is_abstract" class="ml-1 text-[10px] text-amber-400/60">({{ topic.child_slugs?.length ?? 0 }})</span>
                            <span v-else-if="topic.is_low_quality" class="ml-1 text-[10px] text-white/35">低质量</span>
                          </button>
                          <button
                            type="button"
                            class="topic-badge-toggle"
                            :class="{ 'topic-badge-toggle--active': isTopicShownInGraph(topic.slug) }"
                            :aria-label="isTopicShownInGraph(topic.slug) ? `从拓扑图隐藏 ${topic.label}` : `在拓扑图展示 ${topic.label}`"
                            :title="isTopicShownInGraph(topic.slug) ? '从拓扑图隐藏' : '在拓扑图展示'"
                            @click.stop="toggleTopicGraphVisibility(topic.slug)"
                          >
                            <Icon :icon="isTopicShownInGraph(topic.slug) ? 'mdi:eye-outline' : 'mdi:eye-off-outline'" width="14" />
                          </button>
                        </div>
                        <button
                          v-if="category.topics.length > 5"
                          class="topic-more-hint"
                          @click="hotspotSearchQueries[category.key] = ''; hotspotDropdownOpen[category.key] = true"
                        >
                          +{{ category.topics.length - 5 }} 更多
                        </button>
                      </div>
                    </section>
                  </div>
                </section>

                <TopicGraphFooterPanels :detail="detail" />
                </template>

                <!-- Hierarchy view -->
                <template v-else-if="activeTab === 'hierarchy'">
                  <article class="rounded-[30px] p-4 md:p-5 border border-[rgba(255,255,255,0.08)] bg-[rgba(11,18,24,0.4)] backdrop-blur-xl">
                    <TagHierarchy
                      :feed-id="selectedFilterFeedId"
                      :category-id="selectedFilterCategoryId"
                      :anchor-date="selectedDate"
                      @select-tag="handleTagSelect"
                    />
                  </article>
                </template>

                <!-- Narrative view -->
                <template v-else-if="activeTab === 'narrative'">
                  <article class="rounded-[30px] p-4 md:p-5 border border-[rgba(255,255,255,0.08)] bg-[rgba(11,18,24,0.4)] backdrop-blur-xl">
                    <div class="flex items-center justify-between mb-4">
                      <h3 class="text-base font-medium text-[rgba(255,233,220,0.88)]">叙事板块</h3>
                      <button
                        class="px-3 py-1.5 rounded-lg border border-[rgba(255,255,255,0.1)] bg-[rgba(255,255,255,0.04)] text-xs text-[rgba(255,233,220,0.6)] hover:bg-[rgba(255,255,255,0.08)] hover:text-[rgba(255,233,220,0.88)] transition-colors"
                        @click="showConceptManager = !showConceptManager"
                      >
                        板块概念
                      </button>
                    </div>
                    <BoardConceptManager v-if="showConceptManager" class="mb-4" />
                    <NarrativePanel
                      :date="selectedDate"
                      @select-tag="handleNarrativeTagSelect"
                    />
                  </article>
                </template>
              </div>
            </div>
          </article>

          <p v-if="notice" class="rounded-[24px] border border-[rgba(240,138,75,0.28)] bg-[rgba(240,138,75,0.1)] px-4 py-3 text-sm text-[rgba(255,233,220,0.88)]">
            {{ notice }}
          </p>

          <!-- Timeline floating trigger -->
          <button
            type="button"
            class="timeline-fab"
            :class="{ 'timeline-fab--active': timelineOpen }"
            @click="timelineOpen = !timelineOpen"
          >
            <Icon icon="mdi:timeline-clock-outline" width="18" />
            <span class="timeline-fab__label">时间线</span>
            <span v-if="totalAggregatedCount" class="timeline-fab__badge">{{ totalAggregatedCount }}</span>
          </button>
        </div>

        <div class="topic-reading-rail" data-testid="topic-graph-sidebar-region">
          <TopicGraphSidebar
            :detail="detail"
            :selected-digest="selectedDigest"
            :loading="loadingDetail"
            :error="notice"
            :data-state="detail ? 'detail' : (loadingDetail ? 'loading' : 'empty')"
            :selected-keyword="selectedKeywordSlug"
            :selected-tag-slug="selectedHotspotTag?.slug"
            :pending-articles="selectedPendingNode ? pendingArticles : []"
            :selected-pending-node="selectedPendingNode"
            :abstract-node-slug="abstractNodeSlug"
            :abstract-node-label="abstractNodeLabel"
            :timeline-group-articles="selectedGroupArticles"
            :timeline-group-key="selectedGroupKey"
            @open-article="openArticlePreview"
            @highlight-keyword="handleKeywordHighlight"
            @select-child-tag="handleChildTagSelect"
            @select-abstract-tag="handleAbstractTagSelect"
          />
        </div>
      </section>
    </div>

    <!-- Timeline floating panel -->
    <Teleport to="body">
      <Transition name="timeline-slide">
        <div
          v-if="timelineOpen"
          ref="timelinePanelRef"
          class="timeline-float-panel"
          :class="{ 'timeline-float-panel--dragging': isDragging }"
          :style="{
            transform: timelinePosition.x || timelinePosition.y
              ? `translate(${timelinePosition.x}px, ${timelinePosition.y}px)`
              : 'none',
          }"
        >
          <header
            class="timeline-float-panel__header"
            @mousedown="handleTimelineDragStart"
          >
            <div class="flex items-center gap-2">
              <Icon icon="mdi:drag-horizontal-variant" width="16" class="text-white/40 cursor-grab" />
              <Icon icon="mdi:timeline-clock-outline" width="16" class="text-[rgba(240,138,75,0.8)]" />
              <span class="font-serif text-sm text-white">时间线</span>
              <span v-if="totalAggregatedCount" class="text-xs text-white/40">{{ totalAggregatedCount }} 篇</span>
            </div>
            <button
              type="button"
              class="btn-ghost min-h-8 min-w-8 px-0"
              @mousedown.stop
              @click="timelineOpen = false"
            >
              <Icon icon="mdi:close" width="16" />
            </button>
          </header>
          <div class="timeline-float-panel__body">
            <TopicTimeline
              :selected-topic="selectedTopicInfo"
              :groups="timelineAggregationGroups"
              :active-group-key="selectedGroupKey"
              :aggregation-mode="aggregationMode"
              :total-count="totalAggregatedCount"
              @select-group="handleTimelineGroupSelect"
              @open-article="openArticlePreview"
              @update:aggregation-mode="handleAggregationModeChange"
            />
          </div>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="selectedPreviewArticle"
        class="topic-article-modal"
        data-testid="topic-graph-article-preview"
        @click.self="closeArticlePreview"
      >
        <div class="topic-article-modal__panel">
          <header class="topic-article-modal__header">
            <p class="truncate text-sm text-ink-medium">
              {{ loadingPreviewArticle ? '正在准备文章预览...' : '文章预览里保留项目已有的阅读、收藏和抓取动作。' }}
            </p>

            <button
              class="btn-ghost min-h-11 min-w-11 px-0"
              type="button"
              aria-label="关闭文章弹窗"
              data-testid="topic-graph-article-preview-close"
              @click="closeArticlePreview"
            >
              <Icon icon="mdi:close" width="18" />
            </button>
          </header>

          <div class="topic-article-modal__body">
            <ArticleContentView
              :article="selectedPreviewArticle"
              :articles="previewArticles"
              :highlighted-tag-slugs="selectedTopicSlug ? [selectedTopicSlug] : []"
              @navigate="selectedPreviewArticle = $event"
              @favorite="handleArticleFavorite"
              @article-update="handleArticleUpdate"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.topic-stage {
  background:
    radial-gradient(circle at top left, rgba(240, 138, 75, 0.18), transparent 24%),
    radial-gradient(circle at 85% 12%, rgba(63, 124, 255, 0.18), transparent 24%),
    linear-gradient(180deg, #0e161d 0%, #172733 54%, #10212e 100%);
}

.topic-shell {
  width: min(100%, calc(100vw - 1.5rem));
}

.topic-canvas-shell {
  position: relative;
  z-index: 2;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(11, 18, 24, 0.4);
  box-shadow: 0 40px 120px rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(20px);
}

.topic-note {
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(11, 18, 24, 0.7);
  box-shadow: 0 24px 80px rgba(6, 10, 16, 0.24);
  backdrop-filter: blur(12px);
}

.topic-hotspot-strip {
  position: relative;
  z-index: 4;
  overflow: visible;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background:
    radial-gradient(circle at 12% 18%, rgba(240, 138, 75, 0.12), transparent 24%),
    linear-gradient(180deg, rgba(12, 19, 27, 0.86), rgba(8, 14, 22, 0.96));
  box-shadow: 0 24px 80px rgba(6, 10, 16, 0.22);
}

.topic-hotspot-strip__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.topic-timeline-shell {
  position: relative;
  z-index: 1;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(11, 18, 24, 0.4);
  box-shadow: 0 40px 120px rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(20px);
}

.topic-layout {
  align-items: start;
}

.topic-studio__rail {
  display: flex;
  flex-direction: column;
  border: 1px solid rgba(255, 255, 255, 0.04);
  background: linear-gradient(180deg, rgba(15, 23, 31, 0.85), rgba(8, 14, 20, 0.95));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05);
}

.topic-stat-card {
  border: 1px solid rgba(255, 255, 255, 0.04);
  background: rgba(0, 0, 0, 0.2);
}

.topic-stat-card__label {
  font-size: 0.7rem;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  color: rgba(255,255,255,0.46);
}

.topic-stat-card__value {
  margin-top: 0.55rem;
  font-size: 1.8rem;
  font-weight: 700;
  color: white;
}

.topic-reading-rail {
  position: sticky;
  top: 1rem;
}

.topic-digest-modal {
  position: fixed;
  inset: 0;
  z-index: 78;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(8, 12, 18, 0.7);
  backdrop-filter: blur(10px);
}

.topic-digest-modal__panel {
  width: min(1100px, 100%);
  max-height: calc(100vh - 2rem);
  overflow: auto;
  border-radius: 1.75rem;
  background: linear-gradient(180deg, rgba(17, 27, 38, 0.98), rgba(9, 15, 23, 1));
  box-shadow: 0 30px 100px rgba(0, 0, 0, 0.32);
}

.topic-digest-modal__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  padding: 1.1rem 1.25rem 1rem;
}

.topic-digest-modal__body {
  display: grid;
  gap: 1rem;
  padding: 1.2rem 1.25rem 1.35rem;
}

.topic-digest-modal__summary {
  line-height: 1.8;
  color: rgba(236, 242, 248, 0.9);
  white-space: pre-wrap;
}

.topic-digest-modal__tags,
.topic-digest-modal__sources {
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem;
}

.topic-digest-modal__tag,
.topic-digest-modal__source {
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.04);
}

.topic-digest-modal__tag {
  padding: 0.32rem 0.7rem;
  font-size: 0.78rem;
  color: rgba(245, 227, 212, 0.88);
}

.topic-digest-modal__source {
  padding: 0.4rem 0.78rem;
  color: rgba(241, 246, 251, 0.84);
}

.topic-article-modal {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: stretch;
  justify-content: center;
  background: rgba(8, 12, 18, 0.7);
  padding: 1rem;
  backdrop-filter: blur(10px);
}

.topic-article-modal__panel {
  display: flex;
  height: calc(100vh - 2rem);
  width: min(1500px, 100%);
  flex-direction: column;
  overflow: hidden;
  border-radius: 1.75rem;
  background: rgba(255, 252, 248, 0.98);
  box-shadow: 0 30px 100px rgba(0, 0, 0, 0.28);
}

.topic-article-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid rgba(20, 33, 44, 0.08);
  padding: 1rem 1.25rem;
}

.topic-article-modal__body {
  min-height: 0;
  flex: 1;
}

@media (min-width: 1280px) {
  .topic-shell {
    width: min(100%, calc(100vw - 2rem));
  }
}

@media (min-width: 1600px) {
  .topic-shell {
    width: min(100%, calc(100vw - 2.75rem));
  }
}

.topic-badge {
  display: inline-flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.35rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 0.55rem 0.9rem;
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.78);
  background: rgba(255,255,255,0.04);
  transition:
    transform 0.15s ease,
    border-color 0.15s ease,
    background 0.15s ease,
    color 0.15s ease,
    box-shadow 0.15s ease;
}

.topic-badge:hover,
.topic-badge:focus-visible {
  transform: translateY(-1px);
  color: white;
}

.topic-badge--event {
  border-color: rgba(245, 158, 11, 0.72);
  background: rgba(245, 158, 11, 0.18);
}

.topic-badge--person {
  border-color: rgba(16, 185, 129, 0.72);
  background: rgba(16, 185, 129, 0.18);
}

.topic-badge--keyword {
  border-color: rgba(99, 102, 241, 0.72);
  background: rgba(99, 102, 241, 0.18);
}

.topic-badge--active {
  color: white;
  box-shadow: 0 12px 28px rgba(4, 8, 14, 0.24);
}

.topic-badge--abstract {
  border-color: rgba(251, 191, 36, 0.55);
  background: rgba(251, 191, 36, 0.12);
  font-weight: 500;
}

.topic-category-column {
  position: relative;
  z-index: 5;
  overflow: visible;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: linear-gradient(180deg, rgba(20, 29, 40, 0.74), rgba(10, 15, 23, 0.92));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.topic-category-header {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.topic-category-header--button {
  border: 1px solid transparent;
  border-radius: 999px;
  padding: 0.28rem 0.62rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.topic-category-header--button:hover,
.topic-category-header--button:focus-visible {
  border-color: rgba(255, 255, 255, 0.28);
}

.topic-category-header--active {
  border-color: rgba(255, 255, 255, 0.44);
  background: rgba(255, 255, 255, 0.12);
}

.topic-category-header--event {
  color: rgba(252, 211, 77, 0.9);
}

.topic-category-header--person {
  color: rgba(110, 231, 183, 0.9);
}

.topic-category-header--keyword {
  color: rgba(165, 180, 252, 0.92);
}

.topic-category-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

/* Hotspot Search Styles */
.topic-search-wrapper {
  position: relative;
  z-index: 6;
  width: 100%;
}

.topic-search-input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 999px;
  padding: 0.35rem 0.75rem;
  transition: all 0.2s ease;
}

.topic-search-input-wrapper:focus-within {
  border-color: rgba(240, 138, 75, 0.5);
  background: rgba(0, 0, 0, 0.3);
}

.topic-search-icon {
  color: rgba(255, 255, 255, 0.4);
  flex-shrink: 0;
}

.topic-search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: rgba(255, 255, 255, 0.9);
  font-size: 0.8rem;
  padding: 0 0.5rem;
  min-width: 0;
}

.topic-search-input::placeholder {
  color: rgba(255, 255, 255, 0.35);
}

.topic-search-clear {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  transition: all 0.15s ease;
  flex-shrink: 0;
}

.topic-search-clear:hover {
  background: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.9);
}

.topic-search-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background: rgba(22, 28, 38, 0.98);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  padding: 0.5rem;
  max-height: 400px;
  display: flex;
  flex-direction: column;
  z-index: 50;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(20px);
}

.topic-dropdown-scroll {
  overflow-y: auto;
  max-height: 320px;
  padding-right: 0.25rem;
}

.topic-dropdown-scroll::-webkit-scrollbar {
  width: 4px;
}

.topic-dropdown-scroll::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.05);
  border-radius: 2px;
}

.topic-dropdown-scroll::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.15);
  border-radius: 2px;
}

.topic-dropdown-scroll::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.25);
}

.topic-dropdown-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.6rem 0.75rem;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.75);
  font-size: 0.82rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
}

.topic-dropdown-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.topic-dropdown-row + .topic-dropdown-row {
  margin-top: 0.15rem;
}

.topic-dropdown-item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.95);
}

.topic-dropdown-item--active {
  background: rgba(240, 138, 75, 0.2) !important;
  color: rgba(255, 235, 220, 0.95) !important;
}

.topic-dropdown-item--abstract {
  background: rgba(251, 191, 36, 0.08);
  font-weight: 500;
}

.topic-graph-toggle,
.topic-badge-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.55);
  transition:
    border-color 0.15s ease,
    background 0.15s ease,
    color 0.15s ease,
    transform 0.15s ease;
}

.topic-graph-toggle:hover,
.topic-graph-toggle:focus-visible,
.topic-badge-toggle:hover,
.topic-badge-toggle:focus-visible {
  transform: translateY(-1px);
  border-color: rgba(240, 138, 75, 0.38);
  color: rgba(255, 238, 227, 0.92);
}

.topic-graph-toggle {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  border-radius: 999px;
}

.topic-badge-toggle {
  flex-shrink: 0;
  width: 2rem;
  min-height: 2rem;
  border-radius: 999px;
}

.topic-graph-toggle--active,
.topic-badge-toggle--active {
  border-color: rgba(240, 138, 75, 0.44);
  background: rgba(240, 138, 75, 0.16);
  color: rgba(255, 234, 220, 0.96);
}

.topic-dropdown-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  width: 100%;
  padding: 0.6rem 0.75rem;
  margin-top: 0.35rem;
  border-radius: 8px;
  border: none;
  background: rgba(240, 138, 75, 0.15);
  color: rgba(255, 220, 200, 0.85);
  font-size: 0.78rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.topic-dropdown-toggle:hover {
  background: rgba(240, 138, 75, 0.25);
  color: rgba(255, 235, 220, 0.95);
}

.topic-search-no-results {
  padding: 1rem;
  text-align: center;
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.8rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.topic-dropdown-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.4rem 0.9rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.15s ease;
  align-self: center;
}

.topic-dropdown-close:hover {
  background: rgba(255, 255, 255, 0.15);
  color: rgba(255, 255, 255, 0.9);
}

.topic-quick-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.topic-badge-row {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.topic-more-hint {
  display: inline-flex;
  align-items: center;
  padding: 0.4rem 0.75rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.75rem;
  border: none;
  cursor: pointer;
  transition: all 0.15s ease;
}

.topic-more-hint:hover {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.6);
}

.th-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  background: none;
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.7rem;
  padding: 0.28rem 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.th-tab-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.7);
}

.th-tab-btn--active {
  border-color: rgba(240, 138, 75, 0.45);
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 220, 200, 0.88);
}

.topic-count {
  margin-left: 0.5rem;
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.7rem;
}

/* Timeline floating action button */
.timeline-fab {
  position: fixed;
  bottom: 1.5rem;
  right: 1.5rem;
  z-index: 60;
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border: 1px solid rgba(240, 138, 75, 0.35);
  border-radius: 999px;
  background: linear-gradient(135deg, rgba(20, 29, 40, 0.92), rgba(12, 18, 26, 0.96));
  color: rgba(255, 220, 200, 0.9);
  font-size: 0.78rem;
  padding: 0.55rem 1rem;
  cursor: pointer;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35), 0 0 0 1px rgba(240, 138, 75, 0.08);
  backdrop-filter: blur(16px);
  transition: all 0.2s ease;
}

.timeline-fab:hover {
  transform: translateY(-2px);
  border-color: rgba(240, 138, 75, 0.55);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(240, 138, 75, 0.15);
}

.timeline-fab--active {
  border-color: rgba(240, 138, 75, 0.6);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.18), rgba(20, 29, 40, 0.94));
}

.timeline-fab__label {
  white-space: nowrap;
}

.timeline-fab__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.2rem;
  height: 1.2rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.25);
  color: rgba(255, 220, 200, 0.95);
  font-size: 0.65rem;
  padding: 0 0.3rem;
}

/* Timeline floating panel */
.timeline-float-panel {
  position: fixed;
  bottom: 4.5rem;
  right: 1.5rem;
  z-index: 75;
  display: flex;
  flex-direction: column;
  width: min(520px, calc(100vw - 3rem));
  max-height: 65vh;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 1.25rem;
  background: linear-gradient(180deg, rgba(17, 27, 38, 0.97), rgba(9, 15, 23, 0.99));
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45), 0 0 0 1px rgba(255, 255, 255, 0.04);
  backdrop-filter: blur(20px);
  overflow: hidden;
  transition: box-shadow 0.2s ease;
}

.timeline-float-panel--dragging {
  box-shadow: 0 32px 100px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(240, 138, 75, 0.2);
}

.timeline-float-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.timeline-float-panel__body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0.75rem 1rem 1rem;
}

.timeline-float-panel__body::-webkit-scrollbar {
  width: 4px;
}
.timeline-float-panel__body::-webkit-scrollbar-track {
  background: transparent;
}
.timeline-float-panel__body::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.12);
  border-radius: 2px;
}

/* Slide transition */
.timeline-slide-enter-active,
.timeline-slide-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.timeline-slide-enter-from,
.timeline-slide-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.96);
}
</style>
