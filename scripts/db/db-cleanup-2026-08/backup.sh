#!/usr/bin/env bash
# analysis-remediation W1 (task 1.1/1.2)：清理前备份受影响表。
# 用法：bash scripts/db-cleanup-2026-08/backup.sh
# 输出：backups/db-cleanup-<timestamp>/
# 注意：大表必须走「容器内 /tmp + docker cp」中转——直接 stdout 重定向到 /mnt/d
# （Windows 盘）只有 ~20MB/s，6GB 级 dump 会超时；-Fc 压缩格式约减半。
set -euo pipefail

CONTAINER=syntopica-postgres
DB=syntopica
STAMP=$(date +%Y%m%d-%H%M%S)
OUT="backups/db-cleanup-$STAMP"
mkdir -p "$OUT"

for t in topic_tag_embeddings semantic_labels embedding_queues; do
  echo "dumping $t (custom format via container /tmp)..."
  docker exec "$CONTAINER" sh -c "pg_dump -U postgres -d $DB -t $t -Fc -f /tmp/$t.dump"
  docker cp "$CONTAINER:/tmp/$t.dump" "$OUT/$t.dump"
  docker exec "$CONTAINER" rm "/tmp/$t.dump"
  docker exec -i "$CONTAINER" pg_restore -l < "$OUT/$t.dump" > /dev/null
  echo "  ok: $OUT/$t.dump ($(du -h "$OUT/$t.dump" | cut -f1))"
done

echo "backup done -> $OUT ($(du -sh "$OUT" | cut -f1))"
