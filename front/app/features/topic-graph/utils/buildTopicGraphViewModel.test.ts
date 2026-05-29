import { describe, expect, it } from 'vitest'

import { buildTopicGraphViewModel } from './buildTopicGraphViewModel'

describe('buildTopicGraphViewModel', () => {
  it('prefers quality_score for top topics and shrinks low-quality nodes', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 3,
      article_count: 2,
      feed_count: 2,
      top_topics: [
        { label: 'OpenAI', slug: 'openai', category: 'keyword', score: 2.9, quality_score: 0.22, is_low_quality: true },
        { label: 'AI Agent', slug: 'ai-agent', category: 'keyword', score: 2.4, quality_score: 0.88 },
      ],
      nodes: [
        { id: 'ai-agent', label: 'AI Agent', slug: 'ai-agent', kind: 'topic', weight: 5.2, article_count: 2 },
        { id: 'openai', label: 'OpenAI', slug: 'openai', kind: 'topic', weight: 4.1, article_count: 2 },
        { id: 'feed-1', label: 'OpenAI Blog', kind: 'feed', weight: 1.8, color: '#3b6b87', feed_name: 'OpenAI Blog', category_name: 'AI' },
      ],
      edges: [
        { id: 'ai-agent::openai', source: 'ai-agent', target: 'openai', kind: 'topic_topic', weight: 2.2 },
        { id: 'openai::feed-1', source: 'openai', target: 'feed-1', kind: 'topic_feed', weight: 1.1 },
        { id: 'too-weak', source: 'feed-1', target: 'ai-agent', kind: 'topic_feed', weight: 0.1 },
      ],
    })

    expect(viewModel.stats.heroLabel).toBe('AI Agent')
    expect(viewModel.graph.edges).toHaveLength(2)
    expect(viewModel.graph.nodes[0]!.size).toBeGreaterThan(viewModel.graph.nodes[2]!.size)
    expect(viewModel.graph.nodes.find(node => node.id === 'ai-agent')?.size).toBeGreaterThan(viewModel.graph.nodes.find(node => node.id === 'openai')?.size ?? 0)
    expect(viewModel.graph.nodes.find(node => node.id === 'ai-agent')?.opacity).toBeGreaterThan(viewModel.graph.nodes.find(node => node.id === 'openai')?.opacity ?? 0)
    expect(viewModel.graph.featuredNodeIds).toContain('ai-agent')
    expect(viewModel.graph.featuredNodeIds).toContain('openai')
  })

  it('falls back to weight when quality_score is missing', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 2,
      article_count: 2,
      feed_count: 0,
      top_topics: [
        { label: 'Alpha', slug: 'alpha', category: 'keyword', score: 1.2 },
      ],
      nodes: [
        { id: 'alpha', label: 'Alpha', slug: 'alpha', kind: 'topic', weight: 5.2, article_count: 2 },
        { id: 'beta', label: 'Beta', slug: 'beta', kind: 'topic', weight: 3.1, article_count: 1 },
      ],
      edges: [],
    })

    expect(viewModel.graph.nodes.find(node => node.id === 'alpha')?.size).toBeGreaterThan(viewModel.graph.nodes.find(node => node.id === 'beta')?.size ?? 0)
    expect(viewModel.graph.nodes.find(node => node.id === 'alpha')?.opacity).toBe(0.98)
  })

  it('keeps abstract tags visible even when marked low quality', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 2,
      article_count: 2,
      feed_count: 0,
      top_topics: [
        { label: 'Parent', slug: 'parent', category: 'keyword', score: 0.8, quality_score: 0.12, is_low_quality: false, is_abstract: true },
        { label: 'Child', slug: 'child', category: 'keyword', score: 1.1, quality_score: 0.18, is_low_quality: true },
      ],
      nodes: [
        { id: 'parent', label: 'Parent', slug: 'parent', kind: 'topic', weight: 2.2, article_count: 2, is_abstract: true },
        { id: 'child', label: 'Child', slug: 'child', kind: 'topic', weight: 2.6, article_count: 2 },
      ],
      edges: [],
    })

    expect(viewModel.topTopics[0]?.slug).toBe('child')
    expect(viewModel.graph.nodes.find(node => node.id === 'parent')?.isAbstract).toBe(true)
  })

  it('derives trunk node from top topic and classifies emphasis levels', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 3,
      article_count: 2,
      feed_count: 2,
      top_topics: [
        { label: 'AI Agent', slug: 'ai-agent', category: 'keyword', score: 2.9 },
      ],
      nodes: [
        { id: 'ai-agent', label: 'AI Agent', slug: 'ai-agent', kind: 'topic', weight: 5.2, article_count: 2 },
        { id: 'openai', label: 'OpenAI', slug: 'openai', kind: 'topic', weight: 4.1, article_count: 2 },
        { id: 'feed-1', label: 'OpenAI Blog', kind: 'feed', weight: 1.8, color: '#3b6b87', feed_name: 'OpenAI Blog', category_name: 'AI' },
      ],
      edges: [
        { id: 'ai-agent::openai', source: 'ai-agent', target: 'openai', kind: 'topic_topic', weight: 2.2 },
        { id: 'openai::feed-1', source: 'openai', target: 'feed-1', kind: 'topic_feed', weight: 1.1 },
      ],
    })

    // Trunk node should be the hero topic
    expect(viewModel.trunkNode).not.toBeNull()
    expect(viewModel.trunkNode?.label).toBe('AI Agent')
    expect(viewModel.trunkNode?.id).toBe('ai-agent')

    // Branch nodes are directly connected to trunk
    expect(viewModel.branchNodeIds).toContain('openai')
    expect(viewModel.branchNodeIds).not.toContain('ai-agent')

    // Peripheral nodes are not connected to trunk
    expect(viewModel.peripheralNodeIds).toContain('feed-1')
    expect(viewModel.peripheralNodeIds).not.toContain('ai-agent')
    expect(viewModel.peripheralNodeIds).not.toContain('openai')

    // Emphasis levels are properly assigned
    expect(viewModel.emphasisLevels['ai-agent']).toBe('trunk')
    expect(viewModel.emphasisLevels['openai']).toBe('branch')
    expect(viewModel.emphasisLevels['feed-1']).toBe('peripheral')
  })

  it('sorts edges by weight for chronology presentation', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 3,
      article_count: 2,
      feed_count: 2,
      top_topics: [
        { label: 'AI Agent', slug: 'ai-agent', category: 'keyword', score: 2.9 },
      ],
      nodes: [
        { id: 'ai-agent', label: 'AI Agent', slug: 'ai-agent', kind: 'topic', weight: 5.2, article_count: 2 },
        { id: 'openai', label: 'OpenAI', slug: 'openai', kind: 'topic', weight: 4.1, article_count: 2 },
      ],
      edges: [
        { id: 'weak', source: 'ai-agent', target: 'openai', kind: 'topic_topic', weight: 0.5 },
        { id: 'strong', source: 'ai-agent', target: 'openai', kind: 'topic_topic', weight: 3.5 },
      ],
    })

    // Edge chronology should be sorted by weight descending
    expect(viewModel.edgeChronology).toHaveLength(2)
    expect(viewModel.edgeChronology[0]?.weight).toBe(3.5)
    expect(viewModel.edgeChronology[1]?.weight).toBe(0.5)
  })

  it('creates a safe empty state when the graph payload is empty', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'weekly',
      anchor_date: '2026-03-11',
      period_label: '03-10 - 03-16',
      topic_count: 0,
      article_count: 0,
      feed_count: 0,
      top_topics: [],
      nodes: [],
      edges: [],
    })

    expect(viewModel.stats.heroLabel).toBe('还没有图谱')
    expect(viewModel.graph.nodes).toEqual([])
    expect(viewModel.graph.edges).toEqual([])
    expect(viewModel.graph.featuredNodeIds).toEqual([])
    // New trunk/chronology fields must also be safe
    expect(viewModel.trunkNode).toBeNull()
    expect(viewModel.branchNodeIds).toEqual([])
    expect(viewModel.peripheralNodeIds).toEqual([])
    expect(viewModel.edgeChronology).toEqual([])
    expect(viewModel.emphasisLevels).toEqual({})
  })

  it('maps topic node accents from tag categories', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 3,
      article_count: 2,
      feed_count: 1,
      top_topics: [
        { label: 'wwdc 2026', slug: 'wwdc-2026', category: 'event', score: 3.1 },
        { label: 'sam altman', slug: 'sam-altman', category: 'person', score: 2.7 },
        { label: 'ai agent', slug: 'ai-agent', category: 'keyword', score: 2.3 },
      ],
      nodes: [
        { id: 'wwdc-2026', label: 'wwdc 2026', slug: 'wwdc-2026', kind: 'topic', category: 'event', weight: 4.8, article_count: 2 },
        { id: 'sam-altman', label: 'sam altman', slug: 'sam-altman', kind: 'topic', category: 'person', weight: 4.3, article_count: 2 },
        { id: 'ai-agent', label: 'ai agent', slug: 'ai-agent', kind: 'topic', category: 'keyword', weight: 3.9, article_count: 1 },
        { id: 'feed-1', label: 'OpenAI Blog', kind: 'feed', weight: 1.8, color: '#3b6b87', feed_name: 'OpenAI Blog', category_name: 'AI' },
      ],
      edges: [
        { id: 'wwdc::sam', source: 'wwdc-2026', target: 'sam-altman', kind: 'topic_topic', weight: 2.2 },
        { id: 'sam::feed', source: 'sam-altman', target: 'feed-1', kind: 'topic_feed', weight: 1.1 },
      ],
    })

    expect(viewModel.graph.nodes.find(node => node.id === 'wwdc-2026')?.accent).toBe('#f59e0b')
    expect(viewModel.graph.nodes.find(node => node.id === 'sam-altman')?.accent).toBe('#10b981')
    expect(viewModel.graph.nodes.find(node => node.id === 'ai-agent')?.accent).toBe('#6366f1')
    expect(viewModel.graph.nodes.find(node => node.id === 'feed-1')?.accent).toBe('#3b6b87')
  })

  it('propagates is_abstract to isAbstract on scene nodes', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 3,
      article_count: 2,
      feed_count: 1,
      top_topics: [
        { label: 'AI Agent', slug: 'ai-agent', category: 'keyword', score: 2.9 },
      ],
      nodes: [
        { id: 'ai-agent', label: 'AI Agent', slug: 'ai-agent', kind: 'topic', weight: 5.2, article_count: 2, is_abstract: true },
        { id: 'openai', label: 'OpenAI', slug: 'openai', kind: 'topic', weight: 4.1, article_count: 2 },
        { id: 'feed-1', label: 'OpenAI Blog', kind: 'feed', weight: 1.8, color: '#3b6b87', feed_name: 'OpenAI Blog', category_name: 'AI' },
      ],
      edges: [
        { id: 'ai-agent::openai', source: 'ai-agent', target: 'openai', kind: 'topic_topic', weight: 2.2 },
      ],
    })

    // Abstract node should have isAbstract = true
    const abstractNode = viewModel.graph.nodes.find(n => n.id === 'ai-agent')
    expect(abstractNode?.isAbstract).toBe(true)

    // Non-abstract nodes should have isAbstract = false
    const normalNode = viewModel.graph.nodes.find(n => n.id === 'openai')
    expect(normalNode?.isAbstract).toBe(false)

    // Feed nodes should also have isAbstract = false
    const feedNode = viewModel.graph.nodes.find(n => n.id === 'feed-1')
    expect(feedNode?.isAbstract).toBe(false)
  })

  it('defaults isAbstract to false when is_abstract is undefined', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 1,
      article_count: 1,
      feed_count: 0,
      top_topics: [],
      nodes: [
        { id: 'topic-1', label: 'Topic', slug: 'topic-1', kind: 'topic', weight: 3.0 },
      ],
      edges: [],
    })

    expect(viewModel.graph.nodes[0]?.isAbstract).toBe(false)
  })

  it('handles trunk derivation when hero does not match any node', () => {
    const viewModel = buildTopicGraphViewModel({
      type: 'daily',
      anchor_date: '2026-03-11',
      period_label: '2026-03-11 当日',
      topic_count: 2,
      article_count: 1,
      feed_count: 1,
      top_topics: [
        { label: 'Unknown Topic', slug: 'unknown-topic', category: 'keyword', score: 2.9 },
      ],
      nodes: [
        { id: 'existing', label: 'Existing Node', slug: 'existing', kind: 'topic', weight: 3.0, article_count: 1 },
      ],
      edges: [],
    })

    // Trunk should be null when no matching node exists
    expect(viewModel.trunkNode).toBeNull()
    // All nodes become peripheral when there's no trunk
    expect(viewModel.peripheralNodeIds).toContain('existing')
    expect(viewModel.branchNodeIds).toEqual([])
    expect(viewModel.emphasisLevels['existing']).toBe('peripheral')
  })
})
