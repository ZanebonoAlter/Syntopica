import type { GraphNode, TopicCategory, TopicGraphEdge, TopicGraphPayload, TopicTag } from '../../../api/topicGraph'
import { normalizeTopicCategory } from './normalizeTopicCategory'

export interface TopicGraphSceneNode extends GraphNode {
  size: number
  opacity: number
  accent: string
  isAbstract: boolean
  x?: number
  y?: number
  z?: number
}

export interface TopicGraphSceneEdge extends TopicGraphEdge {
  opacity: number
}

export interface TopicGraphViewModel {
  graph: {
    nodes: TopicGraphSceneNode[]
    edges: TopicGraphSceneEdge[]
    featuredNodeIds: string[]
  }
  stats: {
    heroLabel: string
    heroSubline: string
    topicCount: string
    articleCount: string
    feedCount: string
  }
  topTopics: TopicTag[]
  /** Primary trunk node (active/focused topic) */
  trunkNode: TopicGraphSceneNode | null
  /** Direct connections to trunk for branch-level styling */
  branchNodeIds: string[]
  /** Nodes not directly connected to trunk for peripheral styling */
  peripheralNodeIds: string[]
  /** Edges sorted by weight for chronology/lineage presentation */
  edgeChronology: TopicGraphSceneEdge[]
  /** Emphasis levels for visual hierarchy: trunk > branch > peripheral */
  emphasisLevels: Record<string, 'trunk' | 'branch' | 'peripheral'>
}

const FEED_COLOR = '#7b8a96'
const TOPIC_DEFAULT_COLOR = '#f08a4b'
const TOPIC_CATEGORY_ACCENTS: Record<TopicCategory, string> = {
  event: '#f59e0b',
  person: '#10b981',
  keyword: '#6366f1',
}

export function buildTopicGraphViewModel(payload: TopicGraphPayload): TopicGraphViewModel {
  const topTopics = [...payload.top_topics].sort((left, right) => {
    const rightQuality = right.quality_score ?? right.score ?? 0
    const leftQuality = left.quality_score ?? left.score ?? 0
    if (rightQuality === leftQuality) {
      return (right.score ?? 0) - (left.score ?? 0)
    }
    return rightQuality - leftQuality
  })
  const topicCategoryBySlug = new Map(topTopics.map(topic => [topic.slug, normalizeTopicCategory(topic.category, topic.kind)] as const))
  const topicCategoryByLabel = new Map(topTopics.map(topic => [topic.label, normalizeTopicCategory(topic.category, topic.kind)] as const))
  const topicQualityBySlug = new Map(topTopics.map(topic => [topic.slug, topic.quality_score] as const))
  const topicQualityByLabel = new Map(topTopics.map(topic => [topic.label, topic.quality_score] as const))
  const nodes = payload.nodes.map((node) => {
    const normalizedNode = {
      ...node,
      category: resolveTopicCategory(node, topicCategoryBySlug, topicCategoryByLabel),
    }
    const resolvedQuality = resolveNodeQuality(normalizedNode, topicQualityBySlug, topicQualityByLabel)

    return {
      ...normalizedNode,
      size: buildNodeSize(normalizedNode, resolvedQuality),
      opacity: buildNodeOpacity(normalizedNode, resolvedQuality),
      accent: resolveNodeAccent(normalizedNode),
      isAbstract: normalizedNode.is_abstract ?? false,
    }
  })

  const edges = payload.edges
    .filter(edge => edge.weight >= 0.35)
    .map(edge => ({
      ...edge,
      opacity: edge.kind === 'topic_topic' ? 0.82 : 0.48,
    }))

  const hero = topTopics[0]

  // Derive trunk and chronology metadata
  const trunkNode = hero
    ? nodes.find(n => n.label === hero.label || n.slug === hero.slug) || null
    : null

  // Build adjacency set for trunk-connected nodes
  const trunkAdjacency = new Set<string>()
  if (trunkNode) {
    edges.forEach(edge => {
      if (edge.source === trunkNode.id) {
        trunkAdjacency.add(edge.target)
      }
      if (edge.target === trunkNode.id) {
        trunkAdjacency.add(edge.source)
      }
    })
  }

  // Classify nodes: trunk > direct connections > peripheral
  const branchNodeIds: string[] = []
  const peripheralNodeIds: string[] = []
  const emphasisLevels: Record<string, 'trunk' | 'branch' | 'peripheral'> = {}

  nodes.forEach(node => {
    if (trunkNode && node.id === trunkNode.id) {
      emphasisLevels[node.id] = 'trunk'
    } else if (trunkAdjacency.has(node.id)) {
      branchNodeIds.push(node.id)
      emphasisLevels[node.id] = 'branch'
    } else {
      peripheralNodeIds.push(node.id)
      emphasisLevels[node.id] = 'peripheral'
    }
  })

  // Sort edges by weight descending for chronology/lineage presentation
  const edgeChronology = [...edges].sort((left, right) => right.weight - left.weight)

  return {
    graph: {
      nodes,
      edges,
      featuredNodeIds: nodes
        .filter(node => node.kind === 'topic')
        .sort((left, right) => right.weight - left.weight)
        .slice(0, 6)
        .map(node => node.id),
    },
    stats: {
      heroLabel: hero?.label || '还没有图谱',
      heroSubline: hero ? `${payload.period_label} 里最活跃的话题` : '先生成一些 AI 总结，图谱才会长出来。',
      topicCount: String(payload.topic_count ?? 0),
      articleCount: String(payload.article_count ?? 0),
      feedCount: String(payload.feed_count ?? 0),
    },
    topTopics,
    trunkNode,
    branchNodeIds,
    peripheralNodeIds,
    edgeChronology,
    emphasisLevels,
  }
}

function buildNodeSize(node: GraphNode, qualityScore?: number) {
  const base = node.kind === 'feed' ? 5 : 8
  if (node.kind === 'topic' && qualityScore !== undefined) {
    return Math.max(base - 2, Math.round(base + qualityScore * 7))
  }
  return Math.max(base, Math.round(base + node.weight * 2.2))
}

function buildNodeOpacity(node: GraphNode, qualityScore?: number) {
  if (node.kind === 'topic' && qualityScore !== undefined) {
    return Math.max(0.35, Math.min(0.98, 0.35 + qualityScore * 0.63))
  }

  return 0.98
}

function resolveNodeAccent(node: GraphNode) {
  if (node.kind === 'feed') {
    return node.color || FEED_COLOR
  }

  if (node.category) {
    return TOPIC_CATEGORY_ACCENTS[node.category]
  }

  return node.color || TOPIC_DEFAULT_COLOR
}

function resolveTopicCategory(
  node: GraphNode,
  categoryBySlug: Map<string, TopicCategory>,
  categoryByLabel: Map<string, TopicCategory>,
) {
  if (node.kind !== 'topic') return node.category
  if (node.category) return normalizeTopicCategory(node.category)

  if (node.slug && categoryBySlug.has(node.slug)) {
    return categoryBySlug.get(node.slug)
  }

  return categoryByLabel.get(node.label)
}

function resolveNodeQuality(
  node: GraphNode,
  qualityBySlug: Map<string, number | undefined>,
  qualityByLabel: Map<string, number | undefined>,
) {
  if (node.kind !== 'topic') return undefined
  if (node.slug && qualityBySlug.has(node.slug)) return qualityBySlug.get(node.slug)
  return qualityByLabel.get(node.label)
}
