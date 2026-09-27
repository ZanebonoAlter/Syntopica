# demo/seed/seed.sql

Sanitized snapshot of real Syntopica business data, used to bootstrap the
public read-only demo instance. Imported by `demo/entrypoint.sh` after the
backend has created the schema via AutoMigrate + versioned migrations. The
entrypoint first `TRUNCATE ... RESTART IDENTITY CASCADE`s all demo tables,
because migrations seed default rows (e.g. `ai_settings`, `embedding_config`)
at backend boot that would otherwise collide with this snapshot.

## What's in it

- Recent business data only (last 30 days by default; configurable via
  `EXPORT_DAYS`). This sidesteps the backend's `CURRENT_DATE - N days` query
  filters, so the demo is never blank.
- `INSERT` statements + `setval(...)` sequence resets, in topological order
  (the only physical FK — `topic_tags_merged_into_id_fkey` — is satisfied by
  splitting `topic_tags` into an unmerged batch then a merged batch).
- **No schema**: tables/indexes/triggers/pgvector columns are built by the app
  at startup. Do not hand-edit the DDL out of this file — there isn't any.

## What's sanitized (conservative policy)

| Field | Treatment |
| --- | --- |
| `ai_providers` / `ai_routes` / `ai_route_providers` / `ai_settings` **记录** | **不导出**（`Where: "FALSE"`，表结构由应用启动自建；2026-09-22 用户决策：demo 不展示任何 AI 配置记录） |
| `topic_enrichment_result.session_id` | cleared（叙事增强结果里的会话标识） |
| `board_persistent_topics.embedding`, `centroid`；`board_topic_watches.embedding_cache` | `NULL`（pgvector 列不离库） |
| `feeds` | **全量导出无时间窗**（created_at=订阅时间，按窗口过滤会把老订阅滤光） |
| `articles.link`, `image_url` | URL query string stripped |
| `articles.content`, `ai_content_summary` | sensitive token literals (`api_key`/`API_KEY`/`api-key`) → `[redacted-token]`, then capped at 2000 chars |
| `articles.firecrawl_content`, `firecrawl_error`, `completion_error` | cleared |
| `feeds.url` | self-hosted RSSHub host rewritten to `https://rsshub.app` (keeps the route path), then URL query string stripped; `ON CONFLICT (url) DO NOTHING` (rewritten/stripped URLs may collide on the unique key) |
| `feeds.icon` | same rewrite + query strip (favicon-service `?domain=` params can embed the self-hosted host) |
| `feeds.refresh_error` | cleared |
| `reading_behaviors.session_id` | SHA-256 hashed (joins still work) |
| All pgvector `embedding` columns | `NULL` |
| `scheduler_tasks.last_error`, `last_execution_result` | cleared |

## What's excluded entirely

`schema_migrations` (would suppress migrations on the fresh DB), `ai_call_logs`
(request/response snippets), `otel_spans`, `topic_tag_analyses`,
`topic_analysis_cursors`, `tag_merge_suggestions`, queue tables
(`embedding_queues`, `tag_jobs`, `firecrawl_jobs`).

## Regenerating

From a machine with access to a real running database:

```bash
cd backend-go
go run ./cmd/dump-sanitizer
# → writes ../demo/seed/seed.sql
```

Flags / env:

| Var / flag | Default | Purpose |
| --- | --- | --- |
| `DATABASE_DSN` | `configs/config.yaml` | source DB DSN |
| `EXPORT_DAYS` / `--days` | `30` | recent-data window |
| `SEED_OUT` / `--out` | `../demo/seed/seed.sql` | output path |
| `RSSHUB_REWRITE` | `rsshub.app=rsshub.app` | rewrite a self-hosted RSSHub host to the public instance, formatted `sourceHost=targetHost`, to avoid leaking private infrastructure in `feeds.url` |

> **Security review**: after regenerating, confirm the file contains no
> `INSERT INTO ai_call_logs`, no `INSERT INTO schema_migrations`, and that every
> `ai_providers` row has empty `api_key`. See the verification steps in
> `openspec/changes/public-read-only-demo/tasks.md` §9.
