import type { TopicGraphSceneEdge, TopicGraphSceneNode } from './buildTopicGraphViewModel'

export interface TopicGraphLinkOpacityOptions {
  linkDisplayMode: 'hidden' | 'selected' | 'all'
  highlightedLinkIds: Set<string>
  highlightedNodeIds?: Set<string>
  relatedEdgeIds?: Set<string>
}

export function resolveTopicGraphLinkOpacity(
  link: TopicGraphSceneEdge,
  options: TopicGraphLinkOpacityOptions,
) {
  const highlightedNodes = options.highlightedNodeIds || new Set<string>()
  const highlightedEdges = options.relatedEdgeIds || new Set<string>()

  if (highlightedNodes.size > 0 || highlightedEdges.size > 0) {
    return isHighlightedTopicGraphEdge(link, highlightedNodes, highlightedEdges) ? 0.85 : 0
  }

  if (options.linkDisplayMode === 'hidden') {
    return 0
  }

  if (options.linkDisplayMode === 'all') {
    return 0.08
  }

  if (options.linkDisplayMode === 'selected') {
    return options.highlightedLinkIds.has(link.id) ? 0.85 : 0
  }

  return 0
}

export function isHighlightedTopicGraphEdge(
  link: TopicGraphSceneEdge,
  highlightedNodes: Set<string>,
  highlightedEdges: Set<string>,
) {
  if (highlightedEdges.has(link.id)) return true

  const sourceId = resolveLinkNodeId(link.source)
  const targetId = resolveLinkNodeId(link.target)
  return highlightedNodes.has(sourceId) && highlightedNodes.has(targetId)
}

function resolveLinkNodeId(node: string | TopicGraphSceneNode) {
  return typeof node === 'string' ? node : node.id
}
