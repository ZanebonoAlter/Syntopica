# LLM 调用分析 - 常用 SQL 查询

## 基础统计

### 全局概览
```sql
SELECT 
  capability,
  COUNT(*) as cnt,
  MIN(latency_ms) as min_ms,
  ROUND(AVG(latency_ms)::numeric, 0) as avg_ms,
  MAX(latency_ms) as max_ms,
  ROUND(STDDEV(latency_ms)::numeric, 0) as stddev_ms,
  ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*)::numeric, 1) as success_pct
FROM ai_call_logs 
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY capability 
ORDER BY cnt DESC;
```

### 按天趋势
```sql
SELECT 
  date_trunc('day', created_at) as day,
  capability,
  COUNT(*) as cnt
FROM ai_call_logs 
WHERE created_at > NOW() - INTERVAL '14 days'
GROUP BY date_trunc('day', created_at), capability
ORDER BY day DESC, cnt DESC;
```

## Topic Tagging 细分

### Operation 分布
```sql
SELECT 
  (request_meta::jsonb)->>'operation' as operation,
  COUNT(*) as cnt,
  ROUND(AVG(latency_ms)::numeric, 0) as avg_ms,
  SUM(CASE WHEN success THEN 1 ELSE 0 END) as success,
  SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as fail,
  ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*)::numeric, 1) as success_pct
FROM ai_call_logs 
WHERE capability = 'topic_tagging' 
  AND created_at > NOW() - INTERVAL '7 days'
  AND request_meta IS NOT NULL 
  AND request_meta != ''
GROUP BY (request_meta::jsonb)->>'operation'
ORDER BY cnt DESC;
```

### 按 category 分布
```sql
SELECT 
  (request_meta::jsonb)->>'category' as category,
  (request_meta::jsonb)->>'operation' as operation,
  COUNT(*) as cnt,
  ROUND(AVG(latency_ms)::numeric, 0) as avg_ms
FROM ai_call_logs 
WHERE capability = 'topic_tagging' 
  AND created_at > NOW() - INTERVAL '7 days'
  AND request_meta IS NOT NULL 
GROUP BY (request_meta::jsonb)->>'category', (request_meta::jsonb)->>'operation'
ORDER BY cnt DESC;
```

## 错误分析

### 错误码分布
```sql
SELECT 
  error_code,
  COUNT(*) as cnt,
  ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 1) as pct
FROM ai_call_logs 
WHERE success = false 
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY error_code
ORDER BY cnt DESC;
```

### 具体错误样本
```sql
-- 查看某 operation 的错误详情
SELECT 
  id,
  created_at,
  error_code,
  error_message,
  request_meta
FROM ai_call_logs 
WHERE capability = 'topic_tagging' 
  AND success = false 
  AND (request_meta::jsonb)->>'operation' = 'tag_description_person'
  AND created_at > NOW() - INTERVAL '1 day'
ORDER BY created_at DESC
LIMIT 10;
```

## 耗时分析

### P99/P95 耗时
```sql
SELECT 
  capability,
  (request_meta::jsonb)->>'operation' as operation,
  PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY latency_ms) as p50_ms,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) as p95_ms,
  PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms) as p99_ms
FROM ai_call_logs 
WHERE created_at > NOW() - INTERVAL '7 days'
  AND capability = 'topic_tagging'
GROUP BY capability, (request_meta::jsonb)->>'operation'
ORDER BY p95_ms DESC;
```

### 每小时调用量
```sql
SELECT 
  date_trunc('hour', created_at) as hour,
  COUNT(*) as cnt
FROM ai_call_logs 
WHERE capability = 'topic_tagging' 
  AND created_at > NOW() - INTERVAL '2 days'
GROUP BY date_trunc('hour', created_at)
ORDER BY hour DESC;
```

## 关联分析

### 标签覆盖率
```sql
SELECT 
  COUNT(*) as total_articles,
  COUNT(*) FILTER (WHERE EXISTS(
    SELECT 1 FROM article_topic_tags att WHERE att.article_id = articles.id
  )) as tagged,
  ROUND(100.0 * COUNT(*) FILTER (WHERE EXISTS(
    SELECT 1 FROM article_topic_tags att WHERE att.article_id = articles.id
  )) / COUNT(*)::numeric, 1) as pct
FROM articles;
```

### 队列状态
```sql
-- Tag jobs
SELECT 
  status, 
  COUNT(*),
  min(created_at) as oldest,
  max(created_at) as newest
FROM tag_jobs 
GROUP BY status 
ORDER BY status;

-- Abstract tag update queues
SELECT 
  status, 
  COUNT(*) 
FROM abstract_tag_update_queues 
GROUP BY status;

-- Adopt narrower queues
SELECT 
  status, 
  COUNT(*) 
FROM adopt_narrower_queues 
GROUP BY status;
```

### 标签质量
```sql
SELECT 
  CASE
    WHEN quality_score = 0 THEN '0 (unscored)'
    WHEN quality_score < 0.3 THEN '< 0.3 (low)'
    WHEN quality_score < 0.7 THEN '0.3-0.7 (mid)'
    ELSE '>= 0.7 (high)'
  END as bucket,
  count(*)
FROM topic_tags 
WHERE status='active'
GROUP BY bucket 
ORDER BY bucket;
```
