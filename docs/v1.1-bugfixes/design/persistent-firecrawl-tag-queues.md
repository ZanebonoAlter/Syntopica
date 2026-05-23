# Persistent Firecrawl And Tag Queues Design

## Goal

Replace the in-memory Firecrawl/tag execution flow with database-backed job queues so that:

- restarting the backend does not lose pending work
- jobs interrupted mid-run can be recovered automatically
- Firecrawl and article tagging share the same operational model
- backlog and failure states are observable instead of being inferred from missing tags

## Current Problems

### Tag queue

- `topicextraction.TagQueue` uses an in-memory buffered channel.
- the queue drops tasks when the channel is full.
- `EnqueueAsync` ignores enqueue errors, so dropped tasks are silent apart from logs.
- any process restart loses queued-but-not-yet-processed tag work.

### Firecrawl flow

- Firecrawl uses `articles.firecrawl_status = pending` as an implicit queue.
- this survives restart, but it does not track job lease ownership, retries, or stale in-flight work cleanly.
- Firecrawl completion retags inline, so a restart between crawl completion and retagging can leave article state partially advanced.

## Chosen Approach

Use two dedicated job tables with one shared execution model:

- `firecrawl_jobs`
- `tag_jobs`

The tables stay business-specific, but the runtime semantics are identical:

- persistent pending queue in SQLite
- claim by lease instead of in-memory ownership
- retry with backoff
- explicit terminal states

This keeps the schema understandable while avoiding a larger one-table generic job framework.

## Data Model

Both job tables use the same core fields.

### Common fields

- `id`
- `article_id`
- `status`: `pending | leased | completed | failed`
- `priority`: integer, higher first
- `attempt_count`
- `max_attempts`
- `available_at`
- `leased_at`
- `lease_expires_at`
- `last_error`
- `created_at`
- `updated_at`

### Firecrawl-specific fields

- `url_snapshot`: optional cached URL for debugging

### Tag-specific fields

- `feed_name_snapshot`
- `category_name_snapshot`
- `force_retag`: whether the worker should call `RetagArticle` instead of `TagArticle`
- `reason`: optional source such as `article_created`, `firecrawl_completed`, `summary_completed`, `manual_retag`

## Queue Semantics

### Enqueue

- enqueue is idempotent per article for active work
- if an active `pending` or `leased` job already exists for the same article, do not create a duplicate
- if the latest job is `failed` or `completed`, a new job may be created when needed

### Claim

Workers claim jobs by atomically transitioning rows from:

- `pending` and `available_at <= now`
- or `leased` with expired `lease_expires_at`

to:

- `leased`
- set `leased_at = now`
- set `lease_expires_at = now + lease_duration`
- increment `attempt_count`

Claim order:

1. higher `priority`
2. older `available_at`
3. older `created_at`

### Success

- worker sets status to `completed`
- keeps row for history/debugging

### Retry

- worker stores `last_error`
- if `attempt_count < max_attempts`, move back to `pending`
- set `available_at` using exponential backoff with a reasonable cap
- clear lease fields

### Permanent failure

- when retries exceed `max_attempts`, set status to `failed`
- job remains visible for manual inspection or later retry tooling

## Producer Changes

### New article ingestion

In `feeds.RefreshFeed`:

- if `feed.FirecrawlEnabled` is true, enqueue `firecrawl_job`
- if Firecrawl is not enabled, enqueue `tag_job`
- remove reliance on the in-memory `TagQueue`

### Firecrawl completion

In the Firecrawl worker:

- after successful crawl, update article Firecrawl fields
- enqueue a `tag_job` with `force_retag = true` and reason `firecrawl_completed`
- do not retag inline inside the Firecrawl worker

### AI summary completion

In content completion flow:

- after saving summary text, enqueue a `tag_job` with `force_retag = true` and reason `summary_completed`
- do not retag inline inside summary completion

## Consumer Changes

### Firecrawl scheduler

Keep the scheduler, but change it from scanning `articles` as a queue to scanning `firecrawl_jobs`.

Worker loop per cycle:

1. claim up to `N` jobs
2. load related article/feed
3. run Firecrawl scrape
4. update article state
5. enqueue retag job
6. mark Firecrawl job success or retry/failure

The article fields `firecrawl_status`, `firecrawl_error`, `firecrawl_content`, and `firecrawl_crawled_at` remain the source of truth for user-visible content status.

### Tag scheduler/worker

Replace `topicextraction.TagQueue` channel worker with a database-backed tag worker.

Worker loop per cycle:

1. claim up to `N` tag jobs
2. load article
3. choose `TagArticle` or `RetagArticle` based on `force_retag`
4. mark success or retry/failure

The tag worker should remain single-purpose and should not attempt to synthesize missing Firecrawl state.

## Lease And Recovery Behavior

- normal restart: pending jobs stay pending
- crash while executing: leased jobs become claimable again after lease expiry
- repeated crashes do not duplicate completed work because tagging is guarded and enqueue is deduped

Recommended defaults:

- Firecrawl lease: 5 minutes longer than configured request timeout, minimum 10 minutes
- Tag lease: 10 minutes
- Firecrawl max attempts: 5
- Tag max attempts: 5

## Deduplication Rules

### Firecrawl jobs

Only one active job per article is allowed when status is `pending` or `leased`.

### Tag jobs

Only one active job per article is allowed when status is `pending` or `leased`.

If a higher-value retag request arrives while a non-force tag job is active, update the existing active job to `force_retag = true` instead of creating a second active row.

## Operational Visibility

Expose queue metrics through scheduler/runtime status:

- `pending_count`
- `leased_count`
- `failed_count`
- `stale_leased_count`
- `oldest_pending_age_seconds`
- `last_processed_job`

This makes it clear whether "missing tags" means pending backlog, stuck lease, or repeated failure.

## Migration Strategy

### Schema

- add `firecrawl_jobs`
- add `tag_jobs`
- add indexes on `status`, `available_at`, `lease_expires_at`, `article_id`

### Backfill

- create Firecrawl jobs for articles with `firecrawl_status = pending`
- create tag jobs for articles without tags that are currently eligible for direct tagging
- do not backfill completed articles that already have article tags unless explicitly retagging

### Runtime cutover

- start new DB-backed tag worker instead of the channel-based `TagQueue`
- switch Firecrawl scheduler to claim jobs from `firecrawl_jobs`
- keep article status fields for UI compatibility
- remove old in-memory queue startup/shutdown wiring once the new worker is active

## Error Handling

- missing article or feed: fail job permanently if the referenced row no longer exists
- transient network/API failure: retry with backoff
- provider/config missing: retry for Firecrawl, because configuration may be fixed later
- tagging extraction failure: retry up to max attempts, then mark failed

## Testing

### Unit tests

- enqueue deduplication
- claim/lease behavior
- retry and lease expiry recovery
- upgrading active tag job to `force_retag`

### Integration-level backend tests

- restart-safe pending Firecrawl jobs are processed after worker restart
- restart-safe pending tag jobs are processed after worker restart
- Firecrawl success enqueues tag job instead of retagging inline
- summary completion enqueues tag job instead of retagging inline

## Out Of Scope

- generic all-purpose job framework for every background task
- UI for manual retry/cancel of failed jobs
- priority tuning beyond simple integer ordering

## Implementation Notes

- keep the change minimal and localized to Firecrawl/tag paths
- do not change frontend API contracts unless new metrics are added
- preserve current article status fields because frontend logic already depends on them
