#!/usr/bin/env bash
# add-pg-key-tables-backup：定时备份 PostgreSQL 关键业务表（排除日志/缓存/队列等可再生表）。
#
# 用法：bash scripts/db/backup-key-tables.sh   （cron 每日 04:00 自动跑，也可手动）
# 输出：backups/pg-key-<timestamp>.dump（pg_dump -Fc 自定义格式）
# 保留：最近 7 份，成功备份后自动轮转；失败不轮转、不留残档。
# 日志：脚本只写 stdout/stderr（带时间戳步骤行），cron 条目重定向到 logs/db-backup.log。
#
# 维护注意：本脚本用「排除法」圈定范围——新加的业务表自动被备份；新加的
# 日志/缓存/队列/运行时状态表必须手动补进下方 EXCLUDED 清单，否则会进备份档。
#
# 已知边界：pg_dump 18 无序列级排除（--exclude-sequence 不存在，--filter 也不支持
# sequence 对象），排除表的 serial 序列（<表>_id_seq）会作为空壳进档（无表无数据）。
# 实测恢复无害：PG 对同名已存在序列直接复用，AutoMigrate 的 bigserial 建表不受影响。
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTAINER=syntopica-postgres
DB=syntopica
PG_USER=postgres
BACKUP_DIR="$REPO_ROOT/backups"
KEEP=7
STAMP="$(date +%Y%m%d-%H%M%S)"
PART="$BACKUP_DIR/.pg-key-$STAMP.dump.part"

# 排除清单（可再生数据：日志/追踪/缓存/队列/通知/运行记录/运行时状态/行为明细/迁移记录）
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
exec 9>/tmp/pg-key-backup.lock
if ! flock -n 9; then
  log "SKIP: another backup is running"
  exit 0
fi

# ---- 前置校验：容器在跑 + 库可连（失败即非零退出，不产出任何档） ----
state="$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo missing)"
if [ "$state" != "true" ]; then
  log "ERROR: container $CONTAINER not running (state=$state)"
  exit 1
fi
if ! docker exec "$CONTAINER" pg_isready -U "$PG_USER" -d "$DB" -q; then
  log "ERROR: pg_isready failed on $DB"
  exit 1
fi

mkdir -p "$BACKUP_DIR"
trap 'rm -f "$PART"' ERR

# ---- dump（容器内 pg_dump 与 server 版本一致；-Fc 压缩；stdout 流出落盘临时名） ----
log "dumping key tables -> $PART"
docker exec "$CONTAINER" pg_dump -U "$PG_USER" -d "$DB" -n public -Fc "${EXCLUDE_ARGS[@]}" > "$PART"

# ---- 校验：pg_restore -l 能完整列出目录才算成功 ----
if ! docker exec -i "$CONTAINER" pg_restore -l < "$PART" > /dev/null 2>&1; then
  rm -f "$PART"
  log "ERROR: pg_restore -l validation failed"
  exit 1
fi

# ---- 改名 + 汇报 ----
FINAL="$BACKUP_DIR/pg-key-$STAMP.dump"
mv "$PART" "$FINAL"
log "backup ok: $FINAL ($(du -h "$FINAL" | cut -f1))"

# ---- 轮转：仅成功备份后执行，保留最近 KEEP 份（按档名时间戳字典序，勿用 mtime——
# touch/cp 会污染 mtime 导致误删真备份）----
while IFS= read -r old; do
  rm -f "$old"
  log "rotated: $old"
done < <(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | sort | head -n -"$KEEP")
log "rotation done: kept $(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | wc -l) backup(s)"

