import { defineStore } from 'pinia'
import { useTopicGraphApi } from '~/api/topicGraph'
import type {
  AIAnalysisStatus,
  AIAnalysisResult,
  TopicInfo,
  TopicCategoryType,
  TopicAnalysisState,
  EventTimelineItem,
  RelatedEntity,
  PersonProfile,
  PersonAppearance,
  TrendPoint,
  RelatedTopic,
  CoOccurrence,
  ContextExample,
} from '~/types/ai'

/**
 * Store key for caching analysis results
 */
function getCacheKey(topic: TopicInfo, windowType: string, anchorDate: string): string {
  return `${topic.slug}-${topic.category}-${windowType}-${anchorDate}`
}

export const useAIAnalysisStore = defineStore('aiAnalysis', () => {
  // State
  const analysisStates = ref<Map<string, TopicAnalysisState>>(new Map())
  const currentTopic = ref<TopicInfo | null>(null)
  const windowType = ref<'daily' | 'weekly'>('daily')
  const anchorDate = ref<string>(new Date().toISOString().split('T')[0] || new Date().toISOString().slice(0, 10))

  // Polling state
  const pollingTimers = ref<Map<string, ReturnType<typeof setTimeout>>>(new Map())
  const pollingInterval = 2000 // 2 seconds

  // Cache for completed analyses
  const analysisCache = ref<Map<string, AIAnalysisResult>>(new Map())

  /**
   * Get analysis state for a topic
   */
  function getAnalysisState(topic: TopicInfo | null): TopicAnalysisState | undefined {
    if (!topic) return undefined
    const key = getCacheKey(topic, windowType.value, anchorDate.value)
    return analysisStates.value.get(key)
  }

  /**
   * Set analysis state for a topic
   */
  function setAnalysisState(topic: TopicInfo, state: Partial<TopicAnalysisState>) {
    const key = getCacheKey(topic, windowType.value, anchorDate.value)
    const currentState = analysisStates.value.get(key) || {
      topic,
      status: 'idle' as AIAnalysisStatus,
      progress: 0,
      result: null,
      error: null,
      lastUpdated: null,
    }
    analysisStates.value.set(key, { ...currentState, ...state, topic })
  }

  /**
   * Request AI analysis for a topic
   */
  async function requestAnalysis(topic: TopicInfo): Promise<{ success: boolean; error?: string }> {
    const api = useTopicGraphApi()
    const key = getCacheKey(topic, windowType.value, anchorDate.value)

    // Check cache first
    if (analysisCache.value.has(key)) {
      setAnalysisState(topic, {
        status: 'completed',
        progress: 100,
        result: analysisCache.value.get(key) || null,
        error: null,
        lastUpdated: new Date().toISOString(),
      })
      return { success: true }
    }

    // Set pending state
    setAnalysisState(topic, {
      status: 'pending',
      progress: 0,
      error: null,
    })

    try {
      // Use the topic's id for the tagID
      const statusResponse = await api.getAnalysisStatus({
        tagID: topic.id,
        analysisType: topic.category as 'event' | 'person' | 'keyword',
        windowType: windowType.value,
        anchorDate: anchorDate.value,
      })

      if (statusResponse.success && statusResponse.data) {
        const statusData = statusResponse.data

        if (statusData.status === 'completed' && statusData.result) {
          // Analysis already completed
          const result = transformAnalysisResult(topic.category, statusData.result)
          analysisCache.value.set(key, result)
          setAnalysisState(topic, {
            status: 'completed',
            progress: 100,
            result,
            error: null,
            lastUpdated: new Date().toISOString(),
          })
          return { success: true }
        }

        if (statusData.status === 'processing') {
          // Start polling for progress
          setAnalysisState(topic, {
            status: 'processing',
            progress: statusData.progress || 0,
          })
          startPolling(topic)
          return { success: true }
        }

        if (statusData.status === 'failed') {
          setAnalysisState(topic, {
            status: 'failed',
            error: statusData.error || 'Analysis failed',
          })
          return { success: false, error: statusData.error || 'Analysis failed' }
        }
      }

      // If status is 'pending' or 'missing', start the analysis
      const rebuildResponse = await api.rebuildTopicAnalysis({
        tagID: topic.id,
        analysisType: topic.category as 'event' | 'person' | 'keyword',
        windowType: windowType.value,
        anchorDate: anchorDate.value,
      })

      if (rebuildResponse.success) {
        setAnalysisState(topic, {
          status: 'processing',
          progress: 0,
        })
        startPolling(topic)
        return { success: true }
      }

      return { success: false, error: 'Failed to start analysis' }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error'
      setAnalysisState(topic, {
        status: 'failed',
        error: errorMessage,
      })
      return { success: false, error: errorMessage }
    }
  }

  /**
   * Rebuild AI analysis for a topic
   */
  async function rebuildAnalysis(topic: TopicInfo): Promise<{ success: boolean; error?: string }> {
    const api = useTopicGraphApi()
    const key = getCacheKey(topic, windowType.value, anchorDate.value)

    // Clear cache
    analysisCache.value.delete(key)

    // Set processing state
    setAnalysisState(topic, {
      status: 'processing',
      progress: 0,
      error: null,
    })

    try {
      const response = await api.rebuildTopicAnalysis({
        tagID: topic.id,
        analysisType: topic.category as 'event' | 'person' | 'keyword',
        windowType: windowType.value,
        anchorDate: anchorDate.value,
      })

      if (response.success) {
        startPolling(topic)
        return { success: true }
      }

      setAnalysisState(topic, {
        status: 'failed',
        error: 'Failed to rebuild analysis',
      })
      return { success: false, error: 'Failed to rebuild analysis' }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error'
      setAnalysisState(topic, {
        status: 'failed',
        error: errorMessage,
      })
      return { success: false, error: errorMessage }
    }
  }

  /**
   * Start polling for analysis progress
   */
  function startPolling(topic: TopicInfo) {
    const key = getCacheKey(topic, windowType.value, anchorDate.value)

    // Clear existing timer
    stopPolling(topic)

    const timer = setInterval(async () => {
      await pollAnalysisStatus(topic)
    }, pollingInterval)

    pollingTimers.value.set(key, timer)
  }

  /**
   * Stop polling for a topic
   */
  function stopPolling(topic: TopicInfo) {
    const key = getCacheKey(topic, windowType.value, anchorDate.value)
    const timer = pollingTimers.value.get(key)
    if (timer) {
      clearInterval(timer)
      pollingTimers.value.delete(key)
    }
  }

  /**
   * Poll analysis status
   */
  async function pollAnalysisStatus(topic: TopicInfo) {
    const api = useTopicGraphApi()
    const key = getCacheKey(topic, windowType.value, anchorDate.value)

    try {
      const response = await api.getAnalysisStatus({
        tagID: topic.id,
        analysisType: topic.category as 'event' | 'person' | 'keyword',
        windowType: windowType.value,
        anchorDate: anchorDate.value,
      })

      if (response.success && response.data) {
        const { status, progress, error, result } = response.data

        if (status === 'completed' && result) {
          const transformedResult = transformAnalysisResult(topic.category, result)
          analysisCache.value.set(key, transformedResult)
          setAnalysisState(topic, {
            status: 'completed',
            progress: 100,
            result: transformedResult,
            error: null,
            lastUpdated: new Date().toISOString(),
          })
          stopPolling(topic)
        } else if (status === 'processing') {
          setAnalysisState(topic, {
            status: 'processing',
            progress: progress || 0,
          })
        } else if (status === 'failed') {
          setAnalysisState(topic, {
            status: 'failed',
            error: error || 'Analysis failed',
          })
          stopPolling(topic)
        }
      }
    } catch (error) {
      console.error('Error polling analysis status:', error)
    }
  }

  /**
   * Transform API result to frontend format
   */
  function transformAnalysisResult(type: TopicCategoryType, apiResult: Record<string, unknown>): AIAnalysisResult {
    const meta = (apiResult.metadata || {}) as Record<string, unknown>
    const result: AIAnalysisResult = {
      type,
      metadata: {
        analysisTime: (meta.analysisTime || apiResult.analysis_time || 'N/A') as string,
        modelVersion: (meta.modelVersion || apiResult.model_version || 'N/A') as string,
      },
    }

    if (type === 'event') {
      result.eventAnalysis = {
        timeline: (apiResult.timeline || []) as EventTimelineItem[],
        keyMoments: (apiResult.keyMoments || apiResult.key_moments || []) as string[],
        relatedEntities: (apiResult.relatedEntities || apiResult.related_entities || []) as RelatedEntity[],
        summary: (apiResult.summary || '') as string,
      }
    } else if (type === 'person') {
      result.personAnalysis = {
        profile: (apiResult.profile || { name: '', role: '', background: '' }) as PersonProfile,
        appearances: (apiResult.appearances || []) as PersonAppearance[],
        trend: (apiResult.trend || []) as TrendPoint[],
        summary: (apiResult.summary || '') as string,
      }
    } else if (type === 'keyword') {
      result.keywordAnalysis = {
        trendData: (apiResult.trendData || apiResult.trend_data || []) as TrendPoint[],
        relatedTopics: (apiResult.relatedTopics || apiResult.related_topics || []) as RelatedTopic[],
        coOccurrence: (apiResult.coOccurrence || apiResult.co_occurrence || []) as CoOccurrence[],
        contextExamples: (apiResult.contextExamples || apiResult.context_examples || []) as ContextExample[],
        summary: (apiResult.summary || '') as string,
      }
    }

    return result
  }

  /**
   * Set current topic
   */
  function setCurrentTopic(topic: TopicInfo | null) {
    currentTopic.value = topic
  }

  /**
   * Set window type
   */
  function setWindowType(type: 'daily' | 'weekly') {
    windowType.value = type
  }

  /**
   * Set anchor date
   */
  function setAnchorDate(date: string) {
    anchorDate.value = date
  }

  /**
   * Clear analysis state for a topic
   */
  function clearAnalysis(topic: TopicInfo) {
    const key = getCacheKey(topic, windowType.value, anchorDate.value)
    stopPolling(topic)
    analysisStates.value.delete(key)
    analysisCache.value.delete(key)
  }

  /**
   * Clear all analysis states
   */
  function clearAll() {
    pollingTimers.value.forEach((timer) => clearInterval(timer))
    pollingTimers.value.clear()
    analysisStates.value.clear()
    analysisCache.value.clear()
  }

  return {
    // State
    analysisStates,
    currentTopic,
    windowType,
    anchorDate,
    analysisCache,

    // Getters
    getAnalysisState,

    // Actions
    requestAnalysis,
    rebuildAnalysis,
    setCurrentTopic,
    setWindowType,
    setAnchorDate,
    clearAnalysis,
    clearAll,
  }
})
