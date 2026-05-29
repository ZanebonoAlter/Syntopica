import type { TopicGraphViewModel, TopicGraphSceneEdge, TopicGraphSceneNode } from './buildTopicGraphViewModel'

interface BuildDisplayedTopicGraphOptions {
  graph: TopicGraphViewModel['graph']
  visibleTopicSlugs: Set<string>
}

interface DisplayedTopicGraph {
  nodes: TopicGraphSceneNode[]
  edges: TopicGraphSceneEdge[]
  featuredNodeIds: string[]
}

export function buildDisplayedTopicGraph({ graph, visibleTopicSlugs }: BuildDisplayedTopicGraphOptions): DisplayedTopicGraph {
  const visibleTopicNodeIds = new Set(
    graph.nodes
      .filter(node => node.kind === 'topic' && node.slug && visibleTopicSlugs.has(node.slug))
      .map(node => node.id)
  )

  const nodes = graph.nodes.filter(node => node.kind === 'topic' && visibleTopicNodeIds.has(node.id))
  const edges = graph.edges.filter(edge => {
    if (edge.kind !== 'topic_topic') return false
    return visibleTopicNodeIds.has(resolveLinkNodeId(edge.source)) && visibleTopicNodeIds.has(resolveLinkNodeId(edge.target))
  })

  return {
    nodes,
    edges,
    featuredNodeIds: graph.featuredNodeIds.filter(id => visibleTopicNodeIds.has(id)),
  }
}

export function collectRelatedTopicSlugs(graph: TopicGraphViewModel['graph'], selectedSlug: string) {
  const topicById = new Map(
    graph.nodes
      .filter(node => node.kind === 'topic' && node.slug)
      .map(node => [node.id, node] as const)
  )
  const selectedNode = graph.nodes.find(node => node.kind === 'topic' && node.slug === selectedSlug)
  if (!selectedNode) return []

  const relatedSlugs = new Set<string>()

  graph.edges.forEach((edge) => {
    if (edge.kind !== 'topic_topic') return

    const sourceId = resolveLinkNodeId(edge.source)
    const targetId = resolveLinkNodeId(edge.target)

    if (sourceId === selectedNode.id) {
      const targetNode = topicById.get(targetId)
      if (targetNode?.slug) relatedSlugs.add(targetNode.slug)
    }

    if (targetId === selectedNode.id) {
      const sourceNode = topicById.get(sourceId)
      if (sourceNode?.slug) relatedSlugs.add(sourceNode.slug)
    }
  })

  return Array.from(relatedSlugs)
}

function resolveLinkNodeId(node: string | TopicGraphSceneNode) {
  return typeof node === 'string' ? node : node.id
}
