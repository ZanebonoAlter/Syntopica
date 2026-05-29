/**
 * AI 相关类型定义
 */

/**
 * AI 设置数据
 */
export interface AISettings {
  baseURL?: string
  apiKey?: string
  model?: string
  summaryEnabled?: boolean
  providerId?: number
  providerName?: string
  routeName?: string
  timeRange?: number
  apiKeyConfigured?: boolean
}

export interface AIProvider {
  id: number
  name: string
  provider_type: string
  base_url: string
  model: string
  enabled: boolean
  timeout_seconds: number
  max_tokens?: number | null
  temperature?: number | null
  enable_thinking: boolean
  metadata?: string
  api_key_configured: boolean
}

export interface AIRouteProviderLink {
  id: number
  route_id: number
  provider_id: number
  priority: number
  enabled: boolean
  provider: AIProvider
}

export interface AIRoute {
  id: number
  name: string
  capability: string
  enabled: boolean
  strategy: string
  description: string
  route_providers: AIRouteProviderLink[]
}

export interface AIProviderUpsertRequest {
  name: string
  provider_type?: string
  base_url: string
  api_key?: string
  model: string
  enabled?: boolean
  timeout_seconds?: number
  max_tokens?: number | null
  temperature?: number | null
  enable_thinking?: boolean
  metadata?: string
}

/**
 * AI Analysis Types for Topic Graph
 */

/**
 * Topic category types
 */
export type TopicCategoryType = 'event' | 'person' | 'keyword'

/**
 * AI Analysis status
 */
export type AIAnalysisStatus = 'idle' | 'pending' | 'processing' | 'completed' | 'failed'

/**
 * Event timeline item for AI analysis
 */
export interface EventTimelineItem {
  date: string
  title: string
  summary: string
  sources: Array<{
    articleId: number
    title: string
  }>
}

/**
 * Related entity in event analysis
 */
export interface RelatedEntity {
  name: string
  type: 'person' | 'organization' | 'location' | 'concept'
}

/**
 * Event analysis result
 */
export interface EventAnalysis {
  timeline: EventTimelineItem[]
  keyMoments: string[]
  relatedEntities: RelatedEntity[]
  summary: string
}

/**
 * Person profile for AI analysis
 */
export interface PersonProfile {
  name: string
  role: string
  background: string
}

/**
 * Person appearance record
 */
export interface PersonAppearance {
  date: string
  context: string
  quote: string
  articleId: number
}

/**
 * Trend data point
 */
export interface TrendPoint {
  date: string
  value: number
}

/**
 * Person analysis result
 */
export interface PersonAnalysis {
  profile: PersonProfile
  appearances: PersonAppearance[]
  trend: TrendPoint[]
  summary: string
}

/**
 * Related topic in keyword analysis
 */
export interface RelatedTopic {
  slug: string
  label: string
  category: TopicCategoryType
  score: number
}

/**
 * Co-occurrence item in keyword analysis
 */
export interface CoOccurrence {
  term: string
  count: number
}

/**
 * Context example in keyword analysis
 */
export interface ContextExample {
  text: string
  source: string
  articleId: number
}

/**
 * Keyword analysis result
 */
export interface KeywordAnalysis {
  trendData: TrendPoint[]
  relatedTopics: RelatedTopic[]
  coOccurrence: CoOccurrence[]
  contextExamples: ContextExample[]
  summary: string
}

/**
 * AI Analysis metadata
 */
export interface AIAnalysisMetadata {
  analysisTime: string
  modelVersion: string
}

/**
 * Complete AI analysis result
 */
export interface AIAnalysisResult {
  type: TopicCategoryType
  eventAnalysis?: EventAnalysis
  personAnalysis?: PersonAnalysis
  keywordAnalysis?: KeywordAnalysis
  metadata: AIAnalysisMetadata
}

/**
 * Topic info for AI analysis
 */
export interface TopicInfo {
  id: number
  slug: string
  label: string
  category: TopicCategoryType
}

/**
 * AI Analysis state for a single topic
 */
export interface TopicAnalysisState {
  topic: TopicInfo | null
  status: AIAnalysisStatus
  progress: number
  result: AIAnalysisResult | null
  error: string | null
  lastUpdated: string | null
}
