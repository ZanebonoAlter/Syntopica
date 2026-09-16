#!/usr/bin/env bash
# analysis-remediation W1 (task 1.3)：分批清理 topic_tag_embeddings 孤儿行（≤BATCH/批，批间 sleep）。
# 前置：backup.sh 已跑。用法：bash scripts/db-cleanup-2026-08/clean-orphan-embeddings.sh
set -euo pipefail

CONTAINER=syntopica-postgres
DB=syntopica
BATCH=50000
total=0

while :; do
  n=$(docker exec "$CONTAINER" psql -U postgres -d "$DB" -Atc "
    WITH del AS (
      DELETE FROM topic_tag_embeddings
      WHERE id IN (
        SELECT e.id FROM topic_tag_embeddings e
        WHERE NOT EXISTS (SELECT 1 FROM topic_tags t WHERE t.id = e.topic_tag_id)
        LIMIT $BATCH
      ) RETURNING 1
    ) SELECT count(*) FROM del")
  total=$((total + n))
  echo "batch deleted: $n (cumulative $total)"
  [ "$n" -lt "$BATCH" ] && break
  sleep 1
done

echo "orphan cleanup done: $total rows deleted"
