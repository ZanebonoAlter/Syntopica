# Tag Quality Score Design

## Problem

Tags extracted by LLM have a `confidence` (0-1) stored as `Score`, but this value has poor discrimination (mostly 0.7-0.95) and is not used meaningfully downstream. Tags like "2026", "technology", "development" get the same confidence as "RAG", "Transformer architecture".

## Solution

Introduce a `quality_score` field on `topic_tags`, calculated hourly by a scheduler using objective multi-dimensional signals.

## Formula

### Normal Tags

```
QualityScore = 0.4×频率分 + 0.2×共现分 + 0.2×来源分散度 + 0.2×语义匹配分
```

| Dimension | Source | Normalization |
|-----------|--------|---------------|
| 频率分 | `article_topic_tags` count for this tag | Percentile rank |
| 共现分 | Average distinct co-occurring tags per article | Percentile rank |
| 来源分散度 | Distinct feed count from `article_topic_tags JOIN articles` | Percentile rank |
| 语义匹配分 | Average similarity from `topic_tag_embeddings` last match | Direct 0-1 |

Percentile rank: `rank(tag) / total_tags` — robust to outliers, always 0-1.

### Abstract Tags

```
QualityScore = Σ(child_i.QualityScore × child_i.ArticleCount) / Σ(child_i.ArticleCount)
```

Weighted average of child tag scores by article count. Must calculate normal tags first, then abstract tags.

### Tags with insufficient data

- No `article_topic_tags` entries: `quality_score = 0`
- Fewer than 3 tags in system: all dimensions default to 0.5

## Data Model

```go
// In models/topic_graph.go TopicTag struct
QualityScore float64 `gorm:"default:0" json:"quality_score"`
```

Auto-migration handles the new column.

## Scheduler

- Name: `tag_quality_score`
- Interval: 3600s (hourly)
- Pattern: Same as `auto_tag_merge` scheduler (cron-based, mutex, TriggerNow support)
- Location: `internal/jobs/tag_quality_score.go`

## Calculation Steps (SQL)

1. Compute raw metrics per tag:
```sql
SELECT
  t.id,
  COUNT(DISTINCT att.article_id) AS article_count,
  COUNT(DISTINCT a.feed_id) AS feed_diversity,
  AVG(cooc.cooc_count) AS avg_cooccurrence
FROM topic_tags t
LEFT JOIN article_topic_tags att ON att.topic_tag_id = t.id
LEFT JOIN articles a ON a.id = att.article_id
LEFT JOIN (
  SELECT att1.article_id, COUNT(DISTINCT att1.topic_tag_id) - 1 AS cooc_count
  FROM article_topic_tags att1
  GROUP BY att1.article_id
) cooc ON cooc.article_id = att.article_id
WHERE t.status = 'active'
GROUP BY t.id
```

2. Compute percentile ranks in Go for each dimension.

3. For tags with embeddings, get average similarity from last match (use `topic_tag_embeddings` directly — cosine similarity to self is 1.0, so use the average similarity from the merge/judgment history if available, else default to 0.7).

4. Compute weighted sum → write to `topic_tags.quality_score`.

5. For abstract tags: load children via `topic_tag_relations`, compute weighted average.

## Usage

### Tag Sorting

Replace `score` desc with `quality_score` desc in:
- `topicgraph/service.go` — `GetTopicsByCategory`, `GetTopicGraph`
- `topicanalysis/abstract_tag_service.go` — `GetUnclassifiedTags`
- Frontend API responses where tags are listed

### Hide Low Quality

- API responses: tags with `quality_score < 0.3` are included but marked `is_low_quality: true`
- Frontend: these are hidden by default with a toggle
- Does NOT apply to abstract tags (always shown)

### Topic Graph Visualization

- Node opacity/size proportional to `quality_score`
- Low quality nodes rendered smaller and more transparent

## What Doesn't Change

- Existing `Score` field on `AISummaryTopic` and `ArticleTopicTag` — preserves LLM confidence as raw signal
- Topic Graph edge weight calculation (score accumulation for co-occurrence)
- Abstract tag extraction flow
- Tag merge scheduler

## Files to Modify

### Backend
- `internal/domain/models/topic_graph.go` — add `QualityScore` field
- `internal/jobs/tag_quality_score.go` — new scheduler (copy pattern from `auto_tag_merge.go`)
- `internal/app/runtime.go` — register scheduler
- `internal/app/runtimeinfo/runtime_info.go` — add interface
- `internal/jobs/handler.go` — add TriggerNow route
- `internal/domain/topicgraph/service.go` — sort by `quality_score`
- `internal/domain/topicgraph/handler.go` — add manual trigger endpoint
- `internal/domain/topicanalysis/abstract_tag_service.go` — sort unclassified by `quality_score`

### Frontend
- Topic graph component — node size/opacity based on `quality_score`
- Tag list components — hide low quality by default, add toggle
- API types — add `quality_score` field
