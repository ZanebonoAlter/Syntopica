import { describe, expect, it } from 'vitest'

import { resolveTopicGraphLinkOpacity } from './topicGraphCanvasLinks'
import type { TopicGraphSceneEdge } from './buildTopicGraphViewModel'

const highlightedEdge: TopicGraphSceneEdge = {
  id: 'edge-new',
  source: 'topic-a',
  target: 'topic-b',
  kind: 'topic_topic',
  weight: 1.6,
  opacity: 0.8,
}

describe('resolveTopicGraphLinkOpacity', () => {
  it('keeps highlighted edges visible when graph data changes but related edges are already recomputed', () => {
    expect(resolveTopicGraphLinkOpacity(highlightedEdge, {
      linkDisplayMode: 'selected',
      highlightedLinkIds: new Set(['edge-old']),
      highlightedNodeIds: new Set(['topic-a', 'topic-b']),
      relatedEdgeIds: new Set(['edge-new']),
    })).toBe(0.85)
  })

  it('keeps unrelated edges hidden in selected mode', () => {
    expect(resolveTopicGraphLinkOpacity({
      ...highlightedEdge,
      id: 'edge-other',
      target: 'topic-c',
    }, {
      linkDisplayMode: 'selected',
      highlightedLinkIds: new Set(['edge-old']),
      highlightedNodeIds: new Set(['topic-a', 'topic-b']),
      relatedEdgeIds: new Set(['edge-new']),
    })).toBe(0)
  })
})
