package main

// exportSpecs returns the ordered list of tables to export, grouped by
// topological dependency layer (see design.md D8). The ":days" placeholder in
// Where clauses is replaced at runtime with the configured export window.
//
// Sanitization follows the conservative policy in design.md D7: credentials and
// error/log text are cleared, URL query strings are stripped, session_id is
// hashed, and all pgvector embedding columns are projected to NULL.
func exportSpecs() []ExportSpec {
	recent := "created_at >= NOW() - INTERVAL ':days days'"

	return []ExportSpec{
		// --- Layer 0: leaf tables (no outbound FK, no time filter) ---
		{
			Table:   "categories",
			Columns: []string{"id", "name", "slug", "icon", "color", "description", "created_at"},
		},
		{
			Table: "semantic_labels",
			Columns: []string{
				"id", "label", "slug", "embedding", "merge_embedding", "label_type",
				"aliases", "ref_count", "description", "display_order", "source",
				"status", "protected", "created_at", "updated_at",
			},
			VectorColumns: map[string]bool{"embedding": true, "merge_embedding": true},
		},
		{
			Table:   "ai_providers",
			Columns: []string{"id", "name", "provider_type", "base_url", "api_key", "model", "enabled", "timeout_seconds", "max_tokens", "temperature", "enable_thinking", "metadata", "created_at", "updated_at"},
			Sanitizers: map[string]func(string) string{
				"api_key":  clearAll,
				"base_url": clearAll,
				"metadata": emptyJSON,
			},
		},
		{
			Table:   "ai_routes",
			Columns: []string{"id", "name", "capability", "enabled", "priority", "strategy", "description", "max_concurrency", "created_at", "updated_at"},
		},
		{
			Table:   "ai_settings",
			Columns: []string{"id", "key", "value", "description", "created_at", "updated_at"},
			Sanitizers: map[string]func(string) string{
				"value": emptyJSON,
			},
		},
		{
			Table:   "embedding_config",
			Columns: []string{"id", "key", "value", "description", "created_at", "updated_at"},
		},
		{
			Table:   "scheduler_tasks",
			Columns: []string{"id", "name", "description", "check_interval", "last_execution_time", "next_execution_time", "status", "last_error", "last_error_time", "total_executions", "successful_executions", "failed_executions", "consecutive_failures", "last_execution_duration", "last_execution_result", "created_at", "updated_at"},
			// Clear error text and result blobs; they may leak internal URLs/keys.
			Sanitizers: map[string]func(string) string{
				"last_error":            clearAll,
				"last_execution_result": clearAll,
			},
		},
		// --- Layer 1: topic_tags batch #1 (unmerged, recent) ---
		// Self-reference topic_tags_merged_into_id_fkey requires merged-into rows
		// to exist before their dependents, so export unmerged rows first.
		{
			Table: "topic_tags",
			Columns: []string{
				"id", "slug", "label", "category", "icon", "aliases", "description",
				"is_canonical", "source", "feed_count", "status", "merged_into_id",
				"is_watched", "watched_at", "quality_score", "metadata",
				"created_at", "updated_at", "kind",
			},
			Where: "merged_into_id IS NULL AND updated_at >= NOW() - INTERVAL ':days days'",
		},

		// --- Layer 2: feeds + articles ---
		{
			Table:   "feeds",
			Columns: []string{"id", "title", "description", "url", "category_id", "icon", "icon_source", "color", "last_updated", "created_at", "max_articles", "refresh_interval", "refresh_status", "refresh_error", "last_refresh_at", "article_summary_enabled", "completion_on_refresh", "max_completion_retries", "firecrawl_enabled", "tagging_enabled"},
			Where:   recent,
			Sanitizers: map[string]func(string) string{
				// Rewrite self-hosted RSSHub to the public instance first (avoids
				// leaking private infra), then strip tracking query strings.
				"url":  composeSanitizers(rewriteRSSHubHost, stripQuery),
				"icon": composeSanitizers(rewriteRSSHubHost, stripQuery),
				// icon may embed the self-host host as a favicon-service domain
				// param (e.g. google favicons ?domain=...); stripQuery drops it.
				"refresh_error": clearAll,
			},
			ConflictClause: "ON CONFLICT (url) DO NOTHING",
		},
		{
			// NOTE: category_id is excluded — it is a gorm:"-" virtual field on the
			// model (article.go:10) and does not exist as a physical column.
			Table:   "articles",
			Columns: []string{"id", "feed_id", "title", "description", "content", "link", "image_url", "pub_date", "author", "read", "favorite", "summary_status", "summary_generated_at", "summary_processing_started_at", "completion_attempts", "completion_error", "ai_content_summary", "firecrawl_status", "firecrawl_error", "firecrawl_content", "firecrawl_crawled_at", "created_at"},
			Where:   recent,
			Sanitizers: map[string]func(string) string{
				"link":              stripQuery,
				"image_url":         stripQuery,
				"firecrawl_content": clearAll,
				"firecrawl_error":   clearAll,
				"completion_error":  clearAll,
				// Cap long text to keep the seed small; the demo only needs a
				// readable preview, not full article bodies.
				"content":            composeSanitizers(redactSensitiveTokens, truncateContent(2000)),
				"ai_content_summary": composeSanitizers(redactSensitiveTokens, truncateContent(2000)),
			},
		},

		// --- Layer 2b: topic_tags batch #2 (merged, recent) ---
		{
			Table: "topic_tags",
			Columns: []string{
				"id", "slug", "label", "category", "icon", "aliases", "description",
				"is_canonical", "source", "feed_count", "status", "merged_into_id",
				"is_watched", "watched_at", "quality_score", "metadata",
				"created_at", "updated_at", "kind",
			},
			Where: "merged_into_id IS NOT NULL AND updated_at >= NOW() - INTERVAL ':days days'",
		},

		// --- Layer 3: association tables (composite or single PK) ---
		{
			Table:      "topic_tag_semantic_labels",
			Columns:    []string{"topic_tag_id", "semantic_label_id"},
			NoSequence: true,
		},
		{
			Table:      "topic_tag_board_labels",
			Columns:    []string{"topic_tag_id", "semantic_board_id", "score", "match_reason", "downgraded", "direction_mismatch", "created_at", "updated_at"},
			NoSequence: true,
			Where:      "updated_at >= NOW() - INTERVAL ':days days'",
		},
		{
			Table:      "board_composition",
			Columns:    []string{"board_id", "auxiliary_label_id"},
			NoSequence: true,
		},
		{
			Table:   "ai_route_providers",
			Columns: []string{"id", "route_id", "provider_id", "priority", "enabled", "created_at", "updated_at"},
		},
		{
			Table:   "topic_tag_relations",
			Columns: []string{"id", "parent_id", "child_id", "relation_type", "similarity_score", "created_at"},
			Where:   recent,
		},
		{
			Table:   "article_topic_tags",
			Columns: []string{"id", "article_id", "topic_tag_id", "score", "source", "created_at", "updated_at"},
			Where:   recent,
		},
		// --- Layer 4: daily report (detective wall core data) ---
		{
			Table:   "board_daily_reports",
			Columns: []string{"id", "semantic_board_id", "period_date", "title", "summary", "highlights", "dynamics", "article_count", "event_tag_count", "cluster_count", "status", "raw_clusters", "prev_report_id", "generation_prompt_version", "created_at", "updated_at"},
			Where:   "period_date >= NOW() - INTERVAL ':days days'",
		},
		{
			Table:         "daily_report_sections",
			Columns:       []string{"id", "report_id", "cluster_index", "cluster_label", "cluster_tag_ids", "article_count", "best_tier", "avg_score", "embedding", "created_at"},
			VectorColumns: map[string]bool{"embedding": true},
			// No date column on sections; filter by report recency via join.
			Where: "report_id IN (SELECT id FROM board_daily_reports WHERE period_date >= NOW() - INTERVAL ':days days')",
		},
		{
			Table:   "daily_report_threads",
			Columns: []string{"id", "report_id", "section_id", "title", "summary", "tag_ids", "confidence", "related_article_ids", "created_at"},
			Where:   "section_id IN (SELECT ds.id FROM daily_report_sections ds JOIN board_daily_reports bdr ON bdr.id = ds.report_id WHERE bdr.period_date >= NOW() - INTERVAL ':days days')",
		},
		{
			Table:   "daily_report_section_relations",
			Columns: []string{"id", "from_section_id", "to_section_id", "distance", "relation_type", "created_at"},
			Where:   "from_section_id IN (SELECT ds.id FROM daily_report_sections ds JOIN board_daily_reports bdr ON bdr.id = ds.report_id WHERE bdr.period_date >= NOW() - INTERVAL ':days days')",
		},

		// --- Layer 5: behavior (optional, small) ---
		{
			Table:   "reading_behaviors",
			Columns: []string{"id", "article_id", "feed_id", "category_id", "session_id", "event_type", "scroll_depth", "reading_time", "created_at"},
			Where:   recent,
			Sanitizers: map[string]func(string) string{
				"session_id": sha256Hash,
			},
		},
		{
			Table:   "user_preferences",
			Columns: []string{"id", "feed_id", "category_id", "preference_score", "avg_reading_time", "interaction_count", "scroll_depth_avg", "last_interaction_at", "created_at", "updated_at"},
			Where:   recent,
		},
		{
			// Board signal discovery 批次（board-signal-reports，2026-09-24 补）。
			// 全量导出不加时间窗：candidate/discovery 被 topic_enrichment_result 的
			// 复合 FK 引用（source_signal_id 等），窗口过滤会滤掉被引用行、演示机
			// 导入即炸 FK（2026-09-24 事故：漏导两张表，手工补块救场后正式修复）。
			Table:   "board_signal_discovery",
			Columns: []string{"id", "semantic_board_id", "granularity", "period", "analysis_mode", "cutoff", "input_snapshot", "session_id", "candidate_count", "created_at"},
			Sanitizers: map[string]func(string) string{
				// 会话标识与 topic_enrichment_result.session_id 同口径清空。
				"session_id": clearAll,
			},
		},
		{
			// Detect candidates：信号报告的数据源，紧随其批次之后导出（FK 顺序）。
			Table:   "board_signal_candidate",
			Columns: []string{"id", "discovery_id", "semantic_board_id", "granularity", "period", "signal", "why_it_matters", "research_question", "evidence_refs", "score", "rationale", "created_at"},
		},
	}
}
