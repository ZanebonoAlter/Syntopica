// --- Merge with custom name ---

export interface MergeWithCustomNameRequest {
  sourceTagId: number
  targetTagId: number
  newName: string
}

export interface MergeWithCustomNameResult {
  sourceId: number
  targetId: number
  newLabel: string
  mergedAt?: string
}

export interface MergeSummary {
  mergedCount: number
  skippedCount: number
  failedCount: number
  mergedDetails: Array<{
    sourceId: number
    sourceLabel: string
    targetId: number
    newLabel: string
  }>
}

// --- Scan progress ---

export interface ScanProgress {
  status: 'idle' | 'scanning' | 'done' | 'error'
  total: number
  scanned: number
  current_category: string
  new_suggestions: number
  error?: string
}

// --- LLM Evaluate types ---

export interface LLMVerdict {
  should_merge: boolean
  suggested_name: string
  reason: string
}

export interface MergeSuggestion {
  id: number
  new_tag_id: number
  new_label: string
  new_slug: string
  similarity: number
  new_articles: number
  llm_verdict: string | null
  source: string
}

export interface MergeGroup {
  target_tag_id: number
  target_label: string
  target_slug: string
  target_articles: number
  category: string
  suggestions: MergeSuggestion[]
}

export interface MergeGroupResponse {
  groups: MergeGroup[]
  total_groups: number
  evaluated: boolean
}

export interface EvaluateProgress {
  status: 'idle' | 'evaluating' | 'done' | 'error'
  total_groups: number
  completed: number
  current_target: string
  error?: string
}
