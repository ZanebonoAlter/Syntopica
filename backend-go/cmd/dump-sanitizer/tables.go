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
			// demo 不展示用户 AI 配置（2026-09-22 用户决策）：保留导出结构，记录清空。
			Where: "FALSE",
		},
		{
			Table:   "ai_routes",
			Columns: []string{"id", "name", "capability", "enabled", "priority", "strategy", "description", "max_concurrency", "created_at", "updated_at"},
			// 记录清空（同 ai_providers，2026-09-22）。
			Where: "FALSE",
		},
		{
			Table:   "ai_settings",
			Columns: []string{"id", "key", "value", "description", "created_at", "updated_at"},
			Sanitizers: map[string]func(string) string{
				"value": emptyJSON,
			},
			// 记录清空（同 ai_providers，2026-09-22）。
			Where: "FALSE",
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
			// 全量导出：created_at 是「订阅时间」而非更新时间，按窗口过滤会把老订阅
			// 全滤掉（2026-09-22 实测 24 个订阅只剩 1 个），文章对不上源。
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
			// 记录清空（同 ai_providers，2026-09-22）。
			Where: "FALSE",
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
			// 泳道归属/追踪列必须导出（2026-09-22 教训：漏 persistent_topic_id 时
			// 远程 demo 的泳道动态按「沉寂不展示」过滤后 lanes 全空；lane_tier/watch_id
			// 是日报详情 watch 徽章的持久化来源，topic_status_at_report 供 active/candidate 分类）。
			Table:         "daily_report_sections",
			Columns:       []string{"id", "report_id", "cluster_index", "cluster_label", "cluster_tag_ids", "article_count", "best_tier", "avg_score", "embedding", "quality_breakdown", "persistent_topic_id", "topic_match_distance", "topic_match_confidence", "topic_status_at_report", "lane_tier", "watch_id", "created_at"},
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

		// --- Layer 6: narrative enrichment（叙事工坊增强面板，2026-09-22 补）---
		// demo 的叙事工坊原先全空：这批表从来没进过白名单（白名单是早期版本）。
		// 全量导出（无时间窗）——分析存量资产，行数小（合计 ~2.5k），按窗口过滤会把面板清空。
		// 排序满足物理 FK：watches/lane → persistent_topics；qa/review → result；其余指向 semantic_labels。
		{
			Table:   "analysis_methods",
			Columns: []string{"id", "name", "title", "summary", "selection_meta", "content", "enabled", "legacy", "deleted_at", "created_at", "updated_at"},
		},
		{
			Table:   "reference_roles",
			Columns: []string{"id", "name", "title", "content", "enabled", "created_at", "updated_at"},
		},
		{
			Table:         "board_persistent_topics",
			Columns:       []string{"id", "semantic_board_id", "label", "description", "embedding", "status", "first_seen_date", "last_seen_date", "hit_count", "consecutive_hits", "created_at", "updated_at", "source", "centroid", "is_vacuum", "vacuum_strong", "vacuum_mid"},
			VectorColumns: map[string]bool{"embedding": true, "centroid": true},
		},
		{
			Table:      "composite_components",
			Columns:    []string{"composite_id", "component_label_id", "position"},
			NoSequence: true,
		},
		{
			Table:         "board_topic_watches",
			Columns:       []string{"id", "semantic_board_id", "label", "status", "created_at", "updated_at", "type", "query", "embedding_cache", "persistent_topic_id"},
			VectorColumns: map[string]bool{"embedding_cache": true},
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
		{
			Table:   "topic_enrichment_result",
			Columns: []string{"id", "persistent_topic_id", "evolution_assessment", "sectors", "causal_chain", "tool_calls", "input_snapshot", "session_id", "created_at", "semantic_board_id", "analysis_scope", "result_kind", "parent_result_id", "question_key", "granularity", "period", "source_signal_id"},
			Sanitizers: map[string]func(string) string{
				"session_id": clearAll,
			},
		},
		{
			Table:   "topic_lifeline_context",
			Columns: []string{"id", "persistent_topic_id", "granularity", "content", "as_of_date", "source", "created_at", "updated_at", "period"},
		},
		{
			Table:   "topic_lane_snapshots",
			Columns: []string{"id", "persistent_topic_id", "rolling_summary", "as_of_date", "created_at", "updated_at", "rolling_detail"},
		},
		{
			Table:   "topic_enrichment_qa",
			Columns: []string{"id", "topic_enrichment_result_id", "question", "answer", "tool_calls", "source", "sedimented", "created_at"},
		},
		{
			Table:   "topic_enrichment_review",
			Columns: []string{"id", "persistent_topic_id", "prev_result_id", "curr_result_id", "deviation_summary", "affected_context", "confidence", "applied", "source", "created_at", "updated_at", "verdict", "semantic_board_id"},
		},
		{
			Table:   "cross_board_relations",
			Columns: []string{"id", "run_id", "source_board_id", "target_board_id", "target_lane_id", "target_concept", "mapping_snapshot", "relation_type", "claim", "mechanism", "verification_verdict", "quality_grade", "evidence", "counterevidence", "gaps", "status", "suggestion_hash", "evidence_version", "expires_at", "confirmed_at", "dismissed_at", "expired_at", "dismiss_reason", "resolved_by", "created_at", "updated_at"},
		},
		{
			Table:   "board_upgrade_suggestions",
			Columns: []string{"id", "batch_id", "mode", "decision", "board_label", "description", "target_board_id", "auxiliary_label_ids", "confidence", "evidence", "status", "dismiss_reason", "created_at", "resolved_at", "resolved_by", "suggestion_hash"},
		},
	}
}
