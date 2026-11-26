#!/usr/bin/env bash
# bench-vectors-ab：两库交替跑同一组向量查询，抵消 OS page cache 状态对测量的影响。
#
# 用法：bash scripts/bench-vectors-ab.sh <DSN_A> <label_A> <DSN_B> <label_B> [轮数]
#
# 口径：每轮 A、B 各跑一次同样的三段 SQL（SQL 取自生产代码），
#       交替执行 → 两边面对的缓存状态基本对称；取每边的中位数对比。
set -Eeuo pipefail

DSN_A="${1:?}"; LABEL_A="${2:?}"
DSN_B="${3:?}"; LABEL_B="${4:?}"
ROUNDS="${5:-5}"
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PSQL_A="${LAB_PSQL_A:-$LAB_DIR/_tmp/install/postgresql-18.6.1/bin/psql}"
PSQL_B="${LAB_PSQL_B:-/usr/bin/psql}"

sql_tag() {
  local vec="$1"
  cat <<SQL
SELECT t.id AS tag_id, e.embedding <=> '$vec'::vector AS distance
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
WHERE t.category = 'keyword'
  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
  AND e.embedding IS NOT NULL
  AND e.embedding_type = 'identity'
ORDER BY e.embedding <=> '$vec'::vector
LIMIT 20
SQL
}
sql_report() {
  cat <<SQL
SELECT s1.id AS left_id, s2.id AS right_id, s1.embedding <=> s2.embedding AS dist
FROM daily_report_sections s1
JOIN daily_report_sections s2 ON s2.report_id = s1.report_id AND s2.id > s1.id
WHERE s1.embedding IS NOT NULL AND s2.embedding IS NOT NULL
  AND s1.embedding <=> s2.embedding < 0.35
LIMIT 50
SQL
}
sql_disc() {
  local vec="$1" dim="$2"
  cat <<SQL
SELECT ce.candidate_id AS candidate_id, (ce.embedding <=> '$vec'::vector) AS distance
FROM candidate_embeddings ce
JOIN feed_candidates fc ON fc.id = ce.candidate_id
WHERE ce.dimension = $dim
  AND (fc.recommendation_enabled IS NULL OR fc.recommendation_enabled = TRUE)
ORDER BY ce.embedding <=> '$vec'::vector
LIMIT 50
SQL
}

exec_time() { # $1=psql $2=dsn $3=sql
  "$1" "$2" -X -Atc "EXPLAIN (ANALYZE) $3" 2>/dev/null | grep "Execution Time" | tail -1 | grep -oE '[0-9]+\.[0-9]+' | head -1
}

run_side() { # $1=psql $2=dsn $3=tag_sql $4=report_sql $5=disc_sql
  local t r d
  t="$(exec_time "$1" "$2" "$3")"
  r="$(exec_time "$1" "$2" "$4")"
  d="$(exec_time "$1" "$2" "$5")"
  echo "tag=$t report=$r discovery=$d"
}

# 查询向量与参数统一从 A 侧取（两库数据同源）
VEC="$("$PSQL_A" "$DSN_A" -X -Atc "select embedding::text from topic_tag_embeddings where embedding is not null limit 1")"
DIM="$("$PSQL_A" "$DSN_A" -X -Atc "select dimension from candidate_embeddings where embedding is not null limit 1")"
TAG_SQL="$(sql_tag "$VEC")"; REP_SQL="$(sql_report)"; DISC_SQL="$(sql_disc "$VEC" "$DIM")"

echo "round,side,label,tag_ms,report_ms,discovery_ms"
for i in $(seq 1 "$ROUNDS"); do
  for side in A B; do
    if [ "$side" = A ]; then psql="$PSQL_A"; dsn="$DSN_A"; label="$LABEL_A"; else psql="$PSQL_B"; dsn="$DSN_B"; label="$LABEL_B"; fi
    out="$(run_side "$psql" "$dsn" "$TAG_SQL" "$REP_SQL" "$DISC_SQL")"
    t="$(echo "$out" | sed -E 's/.*tag=([0-9.]+).*/\1/')"
    r="$(echo "$out" | sed -E 's/.*report=([0-9.]+).*/\1/')"
    d="$(echo "$out" | sed -E 's/.*discovery=([0-9.]+).*/\1/')"
    echo "$i,$side,$label,$t,$r,$d"
  done
done
