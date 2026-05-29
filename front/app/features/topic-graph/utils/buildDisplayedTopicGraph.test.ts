import { describe, expect, it } from 'vitest'

import { buildDisplayedTopicGraph, collectRelatedTopicSlugs } from './buildDisplayedTopicGraph'
import type { TopicGraphSceneEdge, TopicGraphSceneNode } from './buildTopicGraphViewModel'

function createGraph() {
  const nodes: TopicGraphSceneNode[] = [
    { id: 'topic-a', label: 'Topic A', slug: 'topic-a', kind: 'topic', category: 'keyword', weight: 4, article_count: 2, size: 16, opacity: 0.9, accent: '#111', isAbstract: false },
    { id: 'topic-b', label: 'Topic B', slug: 'topic-b', kind: 'topic', category: 'event', weight: 3.8, article_count: 2, size: 15, opacity: 0.82, accent: '#222', isAbstract: false },
    { id: 'topic-c', label: 'Topic C', slug: 'topic-c', kind: 'topic', category: 'person', weight: 3.5, article_count: 1, size: 14, opacity: 0.76, accent: '#333', isAbstract: false },
    { id: 'feed-1', label: 'Feed 1', kind: 'feed', weight: 1.2, size: 8, opacity: 0.98, accent: '#444', color: '#444', feed_name: 'Feed 1', category_name: 'News', isAbstract: false },
  ]

  const edges: TopicGraphSceneEdge[] = [
    { id: 'ab', source: 'topic-a', target: 'topic-b', kind: 'topic_topic', weight: 2.2, opacity: 0.8 },
    { id: 'ac', source: 'topic-a', target: 'topic-c', kind: 'topic_topic', weight: 1.8, opacity: 0.8 },
    { id: 'af', source: 'topic-a', target: 'feed-1', kind: 'topic_feed', weight: 1.1, opacity: 0.4 },
  ]

  return {
    nodes,
    edges,
    featuredNodeIds: ['topic-a', 'feed-1'],
  }
}

describe('buildDisplayedTopicGraph', () => {
  it('keeps only topic nodes and topic-topic edges', () => {
    const graph = createGraph()

    const displayedGraph = buildDisplayedTopicGraph({
      graph,
      visibleTopicSlugs: new Set(['topic-a', 'topic-b']),
    })

    expect(displayedGraph.nodes.map(node => node.id)).toEqual(['topic-a', 'topic-b'])
    expect(displayedGraph.edges.map(edge => edge.id)).toEqual(['ab'])
    expect(displayedGraph.featuredNodeIds).toEqual(['topic-a'])
  })

  it('includes accumulated expanded topics in the visible graph', () => {
    const graph = createGraph()

    const displayedGraph = buildDisplayedTopicGraph({
      graph,
      visibleTopicSlugs: new Set(['topic-a', 'topic-b', 'topic-c']),
    })

    expect(displayedGraph.nodes.map(node => node.id)).toEqual(['topic-a', 'topic-b', 'topic-c'])
    expect(displayedGraph.edges.map(edge => edge.id)).toEqual(['ab', 'ac'])
  })

  it('keeps topic-topic edges when graph library mutates link endpoints into node objects', () => {
    const graph = createGraph()

    graph.edges = graph.edges.map(edge => ({
      ...edge,
      source: graph.nodes.find(node => node.id === edge.source)!,
      target: graph.nodes.find(node => node.id === edge.target)!,
    })) as unknown as typeof graph.edges

    const displayedGraph = buildDisplayedTopicGraph({
      graph,
      visibleTopicSlugs: new Set(['topic-a', 'topic-b', 'topic-c']),
    })

    expect(displayedGraph.edges.map(edge => edge.id)).toEqual(['ab', 'ac'])
  })
})

describe('collectRelatedTopicSlugs', () => {
  it('returns one-hop related topic slugs for a selected topic', () => {
    const graph = createGraph()

    expect(collectRelatedTopicSlugs(graph, 'topic-a')).toEqual(['topic-b', 'topic-c'])
  })

  it('ignores feed edges and unknown topics', () => {
    const graph = createGraph()

    expect(collectRelatedTopicSlugs(graph, 'topic-b')).toEqual(['topic-a'])
    expect(collectRelatedTopicSlugs(graph, 'missing')).toEqual([])
  })

  it('finds related topics when link endpoints are node objects', () => {
    const graph = createGraph()

    graph.edges = graph.edges.map(edge => ({
      ...edge,
      source: graph.nodes.find(node => node.id === edge.source)!,
      target: graph.nodes.find(node => node.id === edge.target)!,
    })) as unknown as typeof graph.edges

    expect(collectRelatedTopicSlugs(graph, 'topic-a')).toEqual(['topic-b', 'topic-c'])
  })
})
