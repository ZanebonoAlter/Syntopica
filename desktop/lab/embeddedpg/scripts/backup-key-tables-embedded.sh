#!/usr/bin/env bash
# backup-key-tables-embedded：backup-key-tables.sh 的 embedded PG 平移版。
#
# 与原脚本（scripts/db/backup-key-tables.sh）的差异 ONLY 在连接方式与输出目录：
#   原版：docker exec syntopica-postgres pg_dump ...  → ./backups/pg-key-<ts>.dump
#   本版：直接连 embedded PG 端口的本地 pg_dump      → $BACKUP_DIR/pg-key-<ts>.dump
# EXCLUDED 排除清单、flock 重入保护、.part 临时名 + pg_restore -l 校验、
# 「成功才轮转 / 失败不删旧档」、KEEP=7 按档名时间戳字典序轮转——全部原样保留。
#
# 用法：
#   PGHOST=127.0.0.1 PGPORT=55433 PGUSER=postgres PGDATABASE=syntopica \
#   BACKUP_DIR=/tmp/xxx bash scripts/backup-key-tables-embedded.sh
#
# ⚠️ 输出目录默认是 lab 的 _tmp/backups，绝不写仓库的 ./backups/（那是 cron 的救命恢复通道）。
set -Eeuo pipefail

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-55433}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-syntopica}"
PGPASSWORD="${PGPASSWORD:-postgres}"
BACKUP_DIR="${BACKUP_DIR:-$LAB_DIR/_tmp/backups}"
KEEP="${KEEP:-7}"
PG_BIN="${PG_BIN:-$LAB_DIR/_tmp/install/postgresql-18.6.1/bin}"
STAMP="$(date +%Y%m%d-%H%M%S)"
PART="$BACKUP_DIR/.pg-key-$STAMP.dump.part"

# 排除清单（可再生数据：日志/追踪/缓存/队列/通知/运行记录/运行时状态/行为明细/迁移记录）
# ⚠️ 与 scripts/db/backup-key-tables.sh 的 EXCLUDED 保持逐字一致——改语义等于拆安全阀。
EXCLUDED=(
  ai_call_logs
  otel_spans
  ai_embedding_cache
  firecrawl_jobs
  tag_jobs
  embedding_queues
  merge_reembedding_queues
  notifications
  discovery_runs
  discovery_run_items
  cross_board_relation_runs
  topic_watch_hits
  scheduler_tasks
  reading_behaviors
  schema_migrations
)
EXCLUDE_ARGS=()
for t in "${EXCLUDED[@]}"; do
  EXCLUDE_ARGS+=("--exclude-table=public.$t")
done

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

# ---- 重入保护：同一时刻至多一个备份进程 ----
exec 9>/tmp/pg-key-backup-embedded.lock
if ! flock -n 9; then
  log "SKIP: another backup is running"
  exit 0
fi

# ---- 前置校验：库可连（失败即非零退出，不产出任何档） ----
if ! "$PG_BIN/pg_isready" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -q; then
  log "ERROR: pg_isready failed on $PGDATABASE@$PGHOST:$PGPORT"
  exit 1
fi

mkdir -p "$BACKUP_DIR"
trap 'rm -f "$PART"' ERR

# ---- dump（-Fc 压缩；stdout 流出落盘临时名） ----
log "dumping key tables -> $PART"
export PGPASSWORD
"$PG_BIN/pg_dump" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -n public -Fc "${EXCLUDE_ARGS[@]}" > "$PART"

# ---- 校验：pg_restore -l 能完整列出目录才算成功 ----
if ! "$PG_BIN/pg_restore" -l < "$PART" > /dev/null 2>&1; then
  rm -f "$PART"
  log "ERROR: pg_restore -l validation failed"
  exit 1
fi

# ---- 改名 + 汇报 ----
FINAL="$BACKUP_DIR/pg-key-$STAMP.dump"
mv "$PART" "$FINAL"
log "backup ok: $FINAL ($(du -h "$FINAL" | cut -f1))"

# ---- 轮转：仅成功备份后执行，保留最近 KEEP 份（按档名时间戳字典序，勿用 mtime）----
while IFS= read -r old; do
  rm -f "$old"
  log "rotated: $old"
done < <(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | sort | head -n -"$KEEP")
log "rotation done: kept $(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | wc -l) backup(s)"
