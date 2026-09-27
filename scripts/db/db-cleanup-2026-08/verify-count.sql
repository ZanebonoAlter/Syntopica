-- analysis-remediation W1：清理前后核对计数（task 1.1 verify-count.sql）
-- 用法：docker exec -i syntopica-postgres psql -U postgres -d syntopica < scripts/db-cleanup-2026-08/verify-count.sql
SELECT count(*) AS orphan_embeddings
FROM topic_tag_embeddings e
WHERE NOT EXISTS (SELECT 1 FROM topic_tags t WHERE t.id = e.topic_tag_id);

SELECT count(*) AS disabled_with_vectors,
       (SELECT count(*) FROM semantic_labels WHERE status = 'active') AS active_labels,
       (SELECT count(*) FROM semantic_labels) AS total_labels
FROM semantic_labels
WHERE status = 'disabled' AND (embedding IS NOT NULL OR merge_embedding IS NOT NULL);

SELECT pg_size_pretty(pg_database_size('syntopica')) AS db_size,
       pg_size_pretty(pg_total_relation_size('topic_tag_embeddings')) AS embeddings_total_size,
       pg_size_pretty(pg_total_relation_size('semantic_labels')) AS labels_total_size;
