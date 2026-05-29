import type { TopicCategory } from '../../../api/topicGraph'

export function normalizeTopicCategory(category?: string | null, kind?: string | null): TopicCategory {
  if (kind === 'topic') return 'event'
  if (kind === 'entity') return 'person'
  if (kind === 'keyword') return 'keyword'
  if (category === 'topic') return 'event'
  if (category === 'entity') return 'person'
  if (category === 'event' || category === 'person' || category === 'keyword') return category
  return 'keyword'
}
