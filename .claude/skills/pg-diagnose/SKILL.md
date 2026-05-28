---
name: pg-diagnose
description: >
  Diagnose Syntopica backend issues by querying PostgreSQL in Docker container
  `syntopica-postgres`. Use when investigating tag processing failures,
  job queue stalls, scheduler issues, data integrity problems, embedding gaps,
  or any backend state that needs SQL-level inspection. Also use when the user
  asks to check database status, investigate data anomalies, or debug backend
  behavior that depends on persisted state.
---

# PostgreSQL Diagnostic Queries for Syntopica

Container: `syntopica-postgres`
Database: `syntopica` (user `postgres`)

All queries run via:
```bash
docker exec syntopica-postgres psql -U postgres -d syntopica -c "SQL"
```

For multi-line queries use `-c` with heredoc or pass single-line.

## Quick Health Check

Run all of these in parallel for a fast overview:

```sql
-- Tag system health
SELECT count(*) as total,
  count(*) FILTER (WHERE status='active') as active,
  count(*) FILTER (WHERE status='merged') as merged,
  count(*) FILTER (WHERE quality_score > 0) as scored
FROM topic_tags;

-- Job queues
SELECT status, count(*) FROM tag_jobs GROUP BY status ORDER BY status;
SELECT status, count(*) FROM firecrawl_jobs GROUP BY status ORDER BY status;
SELECT status, count(*) FROM embedding_queues GROUP BY status ORDER BY status;

-- Scheduler status
SELECT name, status, last_execution_time, next_execution_time, consecutive_failures
FROM scheduler_tasks ORDER BY name;

-- Abstract tag update queue
SELECT status, count(*) FROM abstract_tag_update_queues GROUP BY status;
```

## Tag System

### Tag counts by category

```sql
SELECT category, kind, source, count(*) as cnt
FROM topic_tags WHERE status='active'
GROUP BY category, kind, source ORDER BY cnt DESC;
```

### Tagging coverage (articles with tags)

```sql
SELECT count(*) as total_articles,
  count(*) FILTER (WHERE EXISTS(
    SELECT 1 FROM article_topic_tags att WHERE att.article_id = articles.id
  )) as tagged,
  round(100.0 * count(*) FILTER (WHERE EXISTS(
    SELECT 1 FROM article_topic_tags att WHERE att.article_id = articles.id
  )) / count(*)::numeric, 1) as pct
FROM articles;
```

### Tags actually linked to articles

```sql
SELECT tt.label, tt.category, tt.kind, count(att.article_id) as article_count
FROM article_topic_tags att
JOIN topic_tags tt ON att.topic_tag_id = tt.id
WHERE tt.status = 'active'
GROUP BY tt.label, tt.category, tt.kind
ORDER BY article_count DESC;
```

### Quality score distribution

```sql
SELECT
  CASE
    WHEN quality_score = 0 THEN '0 (unscored)'
    WHEN quality_score < 0.3 THEN '< 0.3 (low)'
    WHEN quality_score < 0.7 THEN '0.3-0.7 (mid)'
    ELSE '>= 0.7 (high)'
  END as bucket,
  count(*)
FROM topic_tags WHERE status='active'
GROUP BY bucket ORDER BY bucket;
```

### Active tags missing embeddings

```sql
SELECT tt.id, tt.label, tt.category, tt.kind
FROM topic_tags tt
WHERE tt.status='active'
  AND NOT EXISTS (SELECT 1 FROM topic_tag_embeddings tte WHERE tte.topic_tag_id = tt.id)
ORDER BY tt.category, tt.id;
```

## Tag Hierarchy

### Full active hierarchy

```sql
SELECT p.id as parent_id, p.label as parent_label, p.kind as parent_kind,
  c.id as child_id, c.label as child_label, c.kind as child_kind,
  round(r.similarity_score::numeric, 3) as sim
FROM topic_tag_relations r
JOIN topic_tags p ON r.parent_id = p.id
JOIN topic_tags c ON r.child_id = c.id
WHERE p.status='active' AND c.status='active'
ORDER BY p.id, c.id;
```

### Invalid hierarchy: non-abstract parents

Should return 0 rows. If not, there are incorrect relations.

```sql
SELECT r.parent_id, p.label as parent_label, p.kind as parent_kind,
  r.child_id, c.label as child_label
FROM topic_tag_relations r
JOIN topic_tags p ON r.parent_id = p.id
JOIN topic_tags c ON r.child_id = c.id
WHERE p.status='active' AND c.status='active'
  AND r.relation_type='abstract'
  AND p.kind != 'abstract' AND p.source != 'abstract';
```

### Orphan tags (no hierarchy, no articles)

```sql
SELECT tt.id, tt.label, tt.category
FROM topic_tags tt
WHERE tt.status='active' AND tt.kind='topic'
  AND NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE r.child_id = tt.id OR r.parent_id = tt.id)
  AND NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = tt.id)
ORDER BY tt.id;
```

### Stale relations (merged/deleted tags)

```sql
SELECT r.id, r.parent_id, p.status as parent_status, r.child_id, c.status as child_status
FROM topic_tag_relations r
JOIN topic_tags p ON r.parent_id = p.id
JOIN topic_tags c ON r.child_id = c.id
WHERE p.status != 'active' OR c.status != 'active';
```

### Duplicate labels

```sql
SELECT label, count(*) as cnt
FROM topic_tags WHERE status='active'
GROUP BY label HAVING count(*) > 1
ORDER BY cnt DESC;
```

### Slug-label mismatch

```sql
SELECT id, label, slug
FROM topic_tags
WHERE status='active' AND topictypes_slugify(label) != slug
LIMIT 20;
```

Note: `topictypes_slugify` may not exist as DB function. Alternative:

```sql
SELECT id, label, slug FROM topic_tags
WHERE status='active' AND slug != lower(replace(replace(label, ' ', '-'), '/', '-'))
LIMIT 20;
```

## Job Queues

### Tag jobs: stuck leased

```sql
SELECT count(*) as stuck_leases
FROM tag_jobs
WHERE status='leased' AND lease_expires_at < NOW();
```

### Tag jobs: time range

```sql
SELECT status, count(*), min(created_at) as oldest, max(created_at) as newest
FROM tag_jobs GROUP BY status ORDER BY status;
```

### Tag jobs: failed jobs

```sql
SELECT id, article_id, attempt_count, last_error, reason, created_at
FROM tag_jobs WHERE status='failed' ORDER BY created_at DESC;
```

### Firecrawl jobs: status overview

```sql
SELECT status, count(*),
  min(created_at) as oldest, max(created_at) as newest
FROM firecrawl_jobs GROUP BY status ORDER BY status;
```

### Embedding queue status

```sql
SELECT status, count(*) FROM embedding_queues GROUP BY status;
```

### Merge reembedding queue

```sql
SELECT status, count(*) FROM merge_reembedding_queues GROUP BY status;
```

## Schedulers

### All scheduler tasks

```sql
SELECT name, status, last_execution_time, next_execution_time,
  consecutive_failures, last_error, total_executions, successful_executions, failed_executions
FROM scheduler_tasks ORDER BY name;
```

### Tag-related schedulers

```sql
SELECT name, status, last_execution_time, next_execution_time, consecutive_failures, last_error
FROM scheduler_tasks
WHERE name LIKE '%tag%' OR name LIKE '%quality%' OR name LIKE '%hierarchy%'
ORDER BY name;
```

## Articles

### Recent articles tag status

```sql
SELECT a.id, left(a.title, 40) as title,
  EXISTS(SELECT 1 FROM article_topic_tags att WHERE att.article_id = a.id) as has_tags,
  a.firecrawl_status, a.created_at
FROM articles a
ORDER BY a.created_at DESC LIMIT 15;
```

### Articles per feed

```sql
SELECT f.title as feed_title, count(a.id) as article_count
FROM feeds f
LEFT JOIN articles a ON a.feed_id = f.id
GROUP BY f.title ORDER BY article_count DESC LIMIT 20;
```

## Data Cleanup Queries

### Restore expired leased tag jobs

```sql
UPDATE tag_jobs
SET status='pending', leased_at=NULL, lease_expires_at=NULL
WHERE status='leased' AND lease_expires_at < NOW();
```

### Delete invalid abstract relations (non-abstract parents)

```sql
DELETE FROM topic_tag_relations
WHERE relation_type='abstract'
  AND parent_id IN (
    SELECT id FROM topic_tags
    WHERE (kind != 'abstract' AND source != 'abstract') AND status='active'
  );
```

### Delete stale relations referencing merged tags

```sql
DELETE FROM topic_tag_relations
WHERE parent_id IN (SELECT id FROM topic_tags WHERE status != 'active')
   OR child_id IN (SELECT id FROM topic_tags WHERE status != 'active');
```

## Common Investigation Workflows

### Workflow: "Tags not being generated"

1. Check tag_jobs status distribution (pending/leased/completed/failed)
2. Check if backend is running (`netstat -ano | findstr :5000`)
3. Check for stuck leased jobs (lease_expires_at < NOW())
4. Check tagging coverage percentage
5. Check recent articles: do they have tag_jobs?

### Workflow: "Tag hierarchy looks wrong"

1. Run "Invalid hierarchy: non-abstract parents" query
2. Run "Stale relations" query
3. Run "Duplicate labels" query
4. Check abstract_tag_update_queues for stuck tasks
5. Verify scheduler `tag_hierarchy_cleanup` last run

### Workflow: "Quality scores all zero"

1. Check quality score distribution
2. Check `tag_quality_score` scheduler last_execution_time
3. Check article-tag coverage (scores depend on article_count stats)

### Workflow: "Embeddings missing"

1. Run "Active tags missing embeddings"
2. Check embedding_queues status
3. Check abstract_tag_update_queues for pending tasks
4. Verify `embedding_config` table has proper thresholds
