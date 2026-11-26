#!/usr/bin/env bash
# bench-vectors：三域向量查询 EXPLAIN ANALYZE 对比基准。
#
# 用法：bash scripts/bench-vectors.sh <DSN> <标签>
#
# 三条查询取自生产代码（口径一致，两边跑同一段 SQL）：
#   tag          internal/tagmanagement/service/core/embedding.go:181
#   daily_report internal/topicgraph/repository/daily_report_matching.go:142
#   discovery    internal/discovery/service/discovery_recall.go:540
#
# 输出：每个域的 Execution Time（毫秒）+ 计划摘要，追加写入 $OUT（默认 stdout）。
set -Eeuo pipefail

DSN="${1:?usage: bench-vectors.sh <DSN> <label>}"
LABEL="${2:-run}"
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PSQL="${LAB_PSQL:-$LAB_DIR/_tmp/install/postgresql-18.6.1/bin/psql}"
OUT="${OUT:-/dev/stdout}"

run_sql() { "$PSQL" "$DSN" -X -At -v ON_ERROR_STOP=1 -c "$1"; }
explain() { "$PSQL" "$DSN" -X -v ON_ERROR_STOP=1 -c "EXPLAIN (ANALYZE, BUFFERS) $1"; }

{
  echo "### label=$LABEL dsn=${DSN%%\?*}"
  echo "-- server: $(run_sql 'select version()')"

  # ---- 取真实参数（两库数据一致）----
  TAG_CAT="$(run_sql "select category from topic_tags where status='active' group by category order by count(*) desc limit 1")"
  TAG_TYPE="$(run_sql "select embedding_type from topic_tag_embeddings where embedding is not null group by embedding_type order by count(*) desc limit 1")"
  QVEC="$(run_sql "select embedding::text from topic_tag_embeddings where embedding is not null limit 1")"
  echo "-- params: category=$TAG_CAT embedding_type=$TAG_TYPE vec_len=$(printf '%s' "$QVEC" | tr ',' '\n' | wc -l)"

  # ---- ① tag 域 ----
  echo "-- [tag] topic_tag_embeddings cos 检索"
  explain "SELECT t.id AS tag_id, e.embedding <=> '$QVEC'::vector AS distance
           FROM topic_tag_embeddings e
           JOIN topic_tags t ON t.id = e.topic_tag_id
           WHERE t.category = '$TAG_CAT'
             AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
             AND e.embedding IS NOT NULL
             AND e.embedding_type = '$TAG_TYPE'
           ORDER BY e.embedding <=> '$QVEC'::vector
           LIMIT 20" | grep -E "Execution Time|Planning Time|Seq Scan|Index Scan|Sort  |Limit"
  for i in 1 2 3; do
    explain "SELECT t.id AS tag_id, e.embedding <=> '$QVEC'::vector AS distance
             FROM topic_tag_embeddings e
             JOIN topic_tags t ON t.id = e.topic_tag_id
             WHERE t.category = '$TAG_CAT'
               AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
               AND e.embedding IS NOT NULL
               AND e.embedding_type = '$TAG_TYPE'
             ORDER BY e.embedding <=> '$QVEC'::vector
             LIMIT 20" | grep -E "Execution Time" | tail -1
  done

  # ---- ② daily_report 域（区块两两配对）----
  echo "-- [daily_report] daily_report_sections 两两距离"
  explain "SELECT s1.id AS left_id, s2.id AS right_id, s1.embedding <=> s2.embedding AS dist
           FROM daily_report_sections s1
           JOIN daily_report_sections s2 ON s2.report_id = s1.report_id AND s2.id > s1.id
           WHERE s1.embedding IS NOT NULL AND s2.embedding IS NOT NULL
             AND s1.embedding <=> s2.embedding < 0.35
           LIMIT 50" | grep -E "Execution Time|Planning Time|Seq Scan|Index Scan|Nested Loop|Limit"
  for i in 1 2 3; do
    explain "SELECT s1.id AS left_id, s2.id AS right_id, s1.embedding <=> s2.embedding AS dist
             FROM daily_report_sections s1
             JOIN daily_report_sections s2 ON s2.report_id = s1.report_id AND s2.id > s1.id
             WHERE s1.embedding IS NOT NULL AND s2.embedding IS NOT NULL
               AND s1.embedding <=> s2.embedding < 0.35
             LIMIT 50" | grep -E "Execution Time" | tail -1
  done

  # ---- ③ discovery 域（候选召回）----
  echo "-- [discovery] candidate_embeddings 召回"
  CVEC="$(run_sql "select embedding::text from candidate_embeddings where embedding is not null limit 1")"
  CDIM="$(run_sql "select dimension from candidate_embeddings where embedding is not null limit 1")"
  if [ -z "$CVEC" ]; then
    echo "-- [discovery] candidate_embeddings 无向量数据，跳过"
  else
    explain "SELECT ce.candidate_id AS candidate_id, (ce.embedding <=> '$CVEC'::vector) AS distance
             FROM candidate_embeddings ce
             JOIN feed_candidates fc ON fc.id = ce.candidate_id
             WHERE ce.dimension = $CDIM
               AND (fc.recommendation_enabled IS NULL OR fc.recommendation_enabled = TRUE)
             ORDER BY ce.embedding <=> '$CVEC'::vector
             LIMIT 50" | grep -E "Execution Time|Planning Time|Seq Scan|Index Scan|Sort  |Limit"
    for i in 1 2 3; do
      explain "SELECT ce.candidate_id AS candidate_id, (ce.embedding <=> '$CVEC'::vector) AS distance
               FROM candidate_embeddings ce
               JOIN feed_candidates fc ON fc.id = ce.candidate_id
               WHERE ce.dimension = $CDIM
                 AND (fc.recommendation_enabled IS NULL OR fc.recommendation_enabled = TRUE)
               ORDER BY ce.embedding <=> '$CVEC'::vector
               LIMIT 50" | grep -E "Execution Time" | tail -1
    done
  fi
} | tee -a "$OUT"
