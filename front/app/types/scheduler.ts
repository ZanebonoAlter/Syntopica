export interface SchedulerTask {
  id: number
  name: string
  description: string
  check_interval: number
  last_execution_time: string | null
  next_execution_time: string | null
	status: 'idle' | 'running' | 'error' | 'stopped' | 'triggered'
  last_error: string
  last_error_time: string | null
  total_executions: number
  successful_executions: number
  failed_executions: number
  consecutive_failures: number
  last_execution_duration: number | null
  last_execution_result: string
  created_at: string
  updated_at: string
  success_rate: number
}

export interface SchedulerArticleRef {
	id: number
	feed_id: number
	title: string
}

export interface SchedulerOverview {
	pending_count: number
	processing_count: number
	live_processing_count?: number
	stale_processing_count?: number
	completed_count: number
	failed_count: number
	blocked_count: number
	total_count: number
	ai_configured?: boolean
	stale_processing_article?: SchedulerArticleRef | null
	blocked_reasons?: {
		waiting_for_firecrawl_count: number
		feed_disabled_count: number
		ai_unconfigured_count: number
		ready_but_missing_content_count: number
	}
}

export interface SchedulerRunErrorSample {
	article_id: number
	message: string
	category: 'network' | 'config' | 'content' | 'retries' | 'unknown'
}

export interface SchedulerLastRunSummary {
	started_at?: string
	finished_at?: string
	completed_count?: number
	failed_count?: number
	blocked_count?: number
	stale_processing_count?: number
	trigger_source?: string
	feed_count?: number
	generated_count?: number
	skipped_count?: number
	scanned_feeds?: number
	due_feeds?: number
	triggered_feeds?: number
	already_refreshing_feeds?: number
	reason?: string
	live_processing_count?: number
	current_article?: SchedulerArticleRef | null
	last_processed?: SchedulerArticleRef | null
	stale_processing_article?: SchedulerArticleRef | null
	error_samples?: SchedulerRunErrorSample[]
	zombie_deactivated?: number
	flat_merges_applied?: number
	orphaned_relations?: number
	trees_reviewed?: number
	merges_applied?: number
	moves_applied?: number
	adopt_narrower_processed?: number
	abstract_update_processed?: number
	description_backfilled?: number
	llm_calls_total?: number
	llm_budget_total?: number
	timed_out?: boolean
}

export interface SchedulerTriggerResult {
	name: string
	accepted: boolean
	started: boolean
	effectful?: boolean
	reason?: string
	message?: string
	summary?: SchedulerLastRunSummary | null
}

export interface SchedulerStatus {
	name: string
	description?: string
	running?: boolean
	check_interval: number
	next_run?: string | null
	ai_configured?: boolean
	is_executing?: boolean
	database_state?: SchedulerTask
	status?: 'idle' | 'running' | 'error' | 'stopped' | 'triggered'
	last_execution_time?: string | null
	last_error?: string
	concurrency?: number
	overview?: SchedulerOverview
	current_article?: SchedulerArticleRef | null
	last_processed?: SchedulerArticleRef | null
	live_processing_count?: number
	stale_processing_count?: number
	stale_processing_article?: SchedulerArticleRef | null
	last_run_summary?: SchedulerLastRunSummary | null
}
