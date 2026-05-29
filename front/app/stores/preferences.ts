import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { UserPreference, ReadingStats } from '~/types'
import { useReadingBehaviorApi } from '~/api/reading_behavior'

export const usePreferencesStore = defineStore('preferences', () => {
  const api = useReadingBehaviorApi()

  const preferences = ref<UserPreference[]>([])
  const stats = ref<ReadingStats | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const feedPreferences = computed(() =>
    preferences.value.filter((p) => p.feed_id)
  )

  const categoryPreferences = computed(() =>
    preferences.value.filter((p) => p.category_id)
  )

  const topFeeds = computed(() =>
    feedPreferences.value
      .sort((a, b) => b.preference_score - a.preference_score)
      .slice(0, 5)
  )

  const topCategories = computed(() =>
    categoryPreferences.value
      .sort((a, b) => b.preference_score - a.preference_score)
      .slice(0, 5)
  )

  async function fetchPreferences(type?: 'feed' | 'category') {
    loading.value = true
    error.value = null

    try {
      const response = await api.getUserPreferences(type)

      if (response.success && response.data) {
        preferences.value = response.data.filter(
          preference => Boolean(preference.feed_title || preference.category_name)
        )
      } else {
        error.value = response.error || 'Failed to fetch preferences'
      }
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch preferences'
    } finally {
      loading.value = false
    }
  }

  async function fetchStats() {
    loading.value = true
    error.value = null

    try {
      const response = await api.getReadingStats()

      if (response.success && response.data) {
        stats.value = response.data
      } else {
        error.value = response.error || 'Failed to fetch stats'
      }
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch stats'
    } finally {
      loading.value = false
    }
  }

  async function triggerUpdate() {
    loading.value = true
    error.value = null

    try {
      const response = await api.triggerPreferenceUpdate()

      if (!response.success) {
        error.value = response.error || 'Failed to trigger update'
      }
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to trigger update'
    } finally {
      loading.value = false
    }
  }

  function getPreferenceScore(feedId?: number, categoryId?: number): number {
    if (feedId) {
      const pref = preferences.value.find((p) => p.feed_id === feedId)
      if (pref) return pref.preference_score
    }

    if (categoryId) {
      const pref = preferences.value.find((p) => p.category_id === categoryId)
      if (pref) return pref.preference_score
    }

    return 0
  }

  return {
    preferences,
    stats,
    loading,
    error,
    feedPreferences,
    categoryPreferences,
    topFeeds,
    topCategories,
    fetchPreferences,
    fetchStats,
    triggerUpdate,
    getPreferenceScore,
  }
})
