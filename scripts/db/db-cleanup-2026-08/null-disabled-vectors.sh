#!/usr/bin/env bash
# analysis-remediation W1 (task 1.4)：分批将 disabled 标签向量置 NULL（行本体与 aliases 保留）。
# 前置：backup.sh 已跑。用法：bash scripts/db-cleanup-2026-08/null-disabled-vectors.sh
set -euo pipefail

CONTAINER=syntopica-postgres
DB=syntopica
BATCH=50000
total=0

while :; do
  n=$(docker exec "$CONTAINER" psql -U postgres -d "$DB" -Atc "
    WITH upd AS (
      UPDATE semantic_labels
      SET embedding = NULL, merge_embedding = NULL
      WHERE id IN (
        SELECT id FROM semantic_labels
        WHERE status = 'disabled' AND (embedding IS NOT NULL OR merge_embedding IS NOT NULL)
        LIMIT $BATCH
      ) RETURNING 1
    ) SELECT count(*) FROM upd")
  total=$((total + n))
  echo "batch nulled: $n (cumulative $total)"
  [ "$n" -lt "$BATCH" ] && break
  sleep 1
done

echo "disabled vector nulling done: $total rows updated"
