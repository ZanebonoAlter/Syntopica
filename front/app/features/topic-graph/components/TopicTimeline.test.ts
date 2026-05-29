import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TopicTimeline from './TopicTimeline.vue'
import type { TopicCategory } from '~/api/topicGraph'
import type { TimelineAggregationGroup } from '~/types/timeline'

function createMockGroup(overrides: Partial<TimelineAggregationGroup> = {}): TimelineAggregationGroup {
  const startDate = new Date('2026-03-14T08:00:00+08:00')
  const endDate = new Date('2026-03-15T08:00:00+08:00')
  return {
    key: '2026-03-14',
    label: '3月14日 周六',
    startDate,
    endDate,
    articles: [
      {
        id: '1',
        title: 'Source 1',
        link: 'https://example.com/1',
        pubDate: '2026-03-14T08:30:00+08:00',
        feedName: 'OpenAI Blog',
        tags: [{ slug: 'ai-agent', label: 'AI Agent', category: 'keyword' }],
      },
    ],
    ...overrides,
  }
}

function createMockTopic(overrides: { slug?: string; label?: string; category?: TopicCategory } = {}) {
  return {
    slug: 'ai-agent',
    label: 'AI Agent',
    category: 'keyword' as TopicCategory,
    ...overrides,
  }
}

describe('TopicTimeline', () => {
  it('renders aggregation groups', () => {
    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: true,
          TimelineItem: true,
        },
      },
      props: {
        selectedTopic: createMockTopic(),
        groups: [createMockGroup(), createMockGroup({ key: '2026-03-13' })],
        aggregationMode: 'day',
        totalCount: 2,
      },
    })

    expect(wrapper.find('.topic-timeline').exists()).toBe(true)
    expect(wrapper.find('.timeline-list').exists()).toBe(true)
    expect(wrapper.findAllComponents({ name: 'TimelineItem' })).toHaveLength(2)
  })

  it('shows empty state without selected topic', () => {
    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: true,
        },
      },
      props: {
        selectedTopic: null,
        groups: [],
        aggregationMode: 'day',
        totalCount: 0,
      },
    })

    expect(wrapper.find('.timeline-empty').text()).toContain('请先选择')
  })

  it('shows empty state when no groups', () => {
    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: true,
        },
      },
      props: {
        selectedTopic: createMockTopic(),
        groups: [],
        aggregationMode: 'day',
        totalCount: 0,
      },
    })

    expect(wrapper.find('.timeline-empty').text()).toContain('还没有关联文章')
  })

  it('emits open-article from timeline item', async () => {
    const TimelineItemStub = {
      template: '<button @click="$emit(\'openArticle\', 12)">Open</button>',
      props: ['group', 'isFirst', 'isLast', 'aggregationMode'],
      emits: ['openArticle'],
    }

    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: true,
          TimelineItem: TimelineItemStub,
        },
      },
      props: {
        selectedTopic: createMockTopic(),
        groups: [createMockGroup()],
        aggregationMode: 'day',
        totalCount: 1,
      },
    })

    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('open-article')).toEqual([[12]])
  })

  it('emits select-group from timeline item', async () => {
    const TimelineItemStub = {
      template: '<button @click="$emit(\'select\', \'2026-03-14\')">Select</button>',
      props: ['group', 'isFirst', 'isLast', 'isActive', 'aggregationMode'],
      emits: ['select'],
    }

    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: true,
          TimelineItem: TimelineItemStub,
        },
      },
      props: {
        selectedTopic: createMockTopic(),
        groups: [createMockGroup()],
        aggregationMode: 'day',
        totalCount: 1,
        activeGroupKey: '2026-03-14',
      },
    })

    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('select-group')).toEqual([['2026-03-14']])
  })

  it('emits update:aggregationMode from header', async () => {
    const TimelineHeaderStub = {
      template: '<button @click="$emit(\'update:aggregationMode\', \'hour\')">Toggle</button>',
      props: ['topic', 'totalCount', 'aggregationMode'],
      emits: ['update:aggregationMode'],
    }

    const wrapper = mount(TopicTimeline, {
      global: {
        stubs: {
          TimelineHeader: TimelineHeaderStub,
        },
      },
      props: {
        selectedTopic: createMockTopic(),
        groups: [createMockGroup()],
        aggregationMode: 'day',
        totalCount: 1,
      },
    })

    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('update:aggregationMode')).toEqual([['hour']])
  })
})
