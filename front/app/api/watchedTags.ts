import { apiClient } from './client'

export interface WatchedTag {
  id: number
  slug: string
  label: string
  category: string
  watchedAt: string | null
  isAbstract: boolean
  childSlugs: string[]
}

interface WatchedTagPayload {
  id: number
  slug: string
  label: string
  category: string
  watched_at: string | null
  is_abstract: boolean
  child_slugs: string[]
}

export function useWatchedTagsApi() {
  return {
    async listWatchedTags() {
      const res = await apiClient.get<WatchedTagPayload[]>('/topic-tags/watched')
      if (!res.success) return res
      const data = (res.data || []).map((t: WatchedTagPayload) => ({
        id: t.id,
        slug: t.slug,
        label: t.label,
        category: t.category,
        watchedAt: t.watched_at,
        isAbstract: t.is_abstract || false,
        childSlugs: t.child_slugs || [],
      }))
      return { ...res, data } as { success: true; data: WatchedTag[]; message?: string }
    },
    async watchTag(tagId: number) {
      return apiClient.post(`/topic-tags/${tagId}/watch`, {})
    },
    async unwatchTag(tagId: number) {
      return apiClient.post(`/topic-tags/${tagId}/unwatch`, {})
    },
  }
}
