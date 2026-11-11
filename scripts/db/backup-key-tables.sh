#!/usr/bin/env bash
# add-pg-key-tables-backup：定时备份 PostgreSQL 关键业务表（排除日志/缓存/队列等可再生表）。
#
# 用法：bash scripts/db/backup-key-tables.sh   （cron 每日 04:00 自动跑，也可手动）
# 输出：backups/pg-key-<timestamp>.dump            （pg_dump -Fc 自定义格式）
#       backups/pg-key-<timestamp>.dump.counts.tsv （同时间戳行数基线：每行「表名\t行数」）
# 保留：最近 7 份 dump（基线文件随对应 dump 一并轮转删除，不留孤儿），成功备份后自动轮转；
#       失败不轮转、不留残档。
# 日志：脚本只写 stdout/stderr（带时间戳步骤行），cron 条目重定向到 logs/db-backup.log。
#
# 维护注意：本脚本用「排除法」圈定范围——新加的业务表自动被备份；新加的
# 日志/缓存/队列/运行时状态表必须手动补进下方 EXCLUDED 清单，否则会进备份档。
#
# 为什么 schema_migrations 必须在档内（不要再把它加回 EXCLUDED）：
#   它不是可再生数据，而是「恢复保真」的必要输入。启动序列是
#   Phase 1 RunAutoMigrate（建表/加列，注释明确 "Never drops or alters"）
#   → Phase 2 runDatabaseMigrations（版本迁移，重放的是 version 迁移闭包）。
#   档内缺 schema_migrations ⇒ 恢复后**在有数据的库上**从零重放整条版本迁移链
#   （维度/约束 DDL 在存量数据上执行 ⇒ 冲突风险，见 disaster-recovery spec）。
#   历史表述「恢复时 AutoMigrate 自动重跑」是错的：AutoMigrate 只建表/加列，
#   重放版本迁移的是 Phase 2，且它正是在有数据的库上跑的那一步。
#
# 已知边界：pg_dump 18 无序列级排除（--exclude-sequence 不存在，--filter 也不支持
# sequence 对象），排除表的 serial 序列（<表>_id_seq）会作为空壳进档（无表无数据）。
# 实测恢复无害：PG 对同名已存在序列直接复用，AutoMigrate 的 bigserial 建表不受影响。
#
# 基线与档的一致性：dump 与行数基线共用同一个 MVCC 快照
# （pg_export_snapshot → pg_dump --snapshot + SET TRANSACTION SNAPSHOT），
# 否则 10 分钟 dump 窗口内的业务写入会让基线与档内容漂移，恢复侧逐表比对必然误报。
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# 生产默认值（cron 路径不受影响）；环境变量仅供冒烟脚本在一次性测试库/临时目录上跑
CONTAINER="${BACKUP_CONTAINER:-syntopica-postgres}"
DB="${BACKUP_DB:-syntopica}"
PG_USER=postgres
PG_PASSWORD="${BACKUP_PG_PASSWORD:-postgres}"
PG_HOST="${BACKUP_PG_HOST:-127.0.0.1}"
PG_PORT="${BACKUP_PG_PORT:-5432}"
BACKUP_DIR="${BACKUP_DIR:-$REPO_ROOT/backups}"
KEEP="${BACKUP_KEEP:-7}"
STAMP="$(date +%Y%m%d-%H%M%S)"
PART="$BACKUP_DIR/.pg-key-$STAMP.dump.part"
COUNTS_PART="$BACKUP_DIR/.pg-key-$STAMP.counts.tsv.part"

# 排除清单（可再生数据：日志/追踪/缓存/队列/通知/运行记录/运行时状态/行为明细）
# ⚠ schema_migrations 不在此列表——迁移历史必须随档走（见文件头说明）。
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
)
EXCLUDE_ARGS=()
for t in "${EXCLUDED[@]}"; do
  EXCLUDE_ARGS+=("--exclude-table=public.$t")
done

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

# ---------- PG 客户端定位（重要：客户端**不进容器**） ----------
#
# 2026-11-05 实测：`docker exec` 起的客户端进程是容器里 postmaster(PID 1) 的子进程，
# 客户端**非零退出会被 postmaster 记成「unknown child exited abnormally」→ 整个集群被拆掉**
# （terminating any other active server processes → 崩溃恢复，连带打断生产库连接）。
# 也就是说：一个「备份失败」会把生产库拖下水。
#
# 对策：客户端一律在**宿主机**跑（连同一个发布端口），退出码与 postmaster 无关；
# 宿主机没有兼容客户端时退回容器内执行，但**必须屏蔽退出码**（让 postmaster 只看到 0）。
HOST_CLIENT_DIR=""
for d in /usr/lib/postgresql/*/bin /usr/pgsql-*/bin /usr/local/pgsql/bin /usr/bin; do
	[ -x "$d/pg_dump" ] || continue
	HOST_CLIENT_DIR="$d"
done
if [ -n "$HOST_CLIENT_DIR" ]; then
	HOST_PG_MAJOR="$("$HOST_CLIENT_DIR/pg_dump" --version | sed -E 's/.* ([0-9]+)\.[0-9]+.*/\1/')"
	SERVER_PG_MAJOR="$(docker exec "$CONTAINER" sh -c "psql -X -U $PG_USER -d $DB -tAc 'show server_version' 2>/dev/null; exit 0" | cut -d. -f1 | tr -dc '0-9' || echo 0)"
	if [ -n "$SERVER_PG_MAJOR" ] && [ "$SERVER_PG_MAJOR" != "0" ] && [ "${HOST_PG_MAJOR:-0}" -lt "$SERVER_PG_MAJOR" ]; then
		HOST_CLIENT_DIR="" # 客户端比 server 老 → 不能用（pg_dump 会拒）
	fi
fi

# pg_client <bin> <args...>：执行 PG 客户端，退出码写入全局 PG_RC（不直接返回）。
pg_client() {
	local bin="$1"
	shift
	if [ -n "$HOST_CLIENT_DIR" ]; then
		PG_RC=0
		# `|| PG_RC=$?`：`set -e` 下失败的命令会直接终止脚本（错误分支就永远不会执行），
		# 用 `||` 把失败变成「被处理过的值」，退出码由调用方判定。
		PGPASSWORD="$PG_PASSWORD" PGHOST="$PG_HOST" PGPORT="$PG_PORT" PGUSER="$PG_USER" \
			"$HOST_CLIENT_DIR/$bin" "$@" || PG_RC=$?
	else
		# 宿主机没有可用客户端：容器内执行并**屏蔽退出码**（否则客户端失败会拆集群）。
		# 连接参数要翻译成**容器内**的（宿主发布端口在容器内不存在）。
		local rc_file="/tmp/syntopica-pgclient-rc-$$"
		local -a cargs=()
		local skip_next=0 a
		for a in "$@"; do
			if [ "$skip_next" = "1" ]; then skip_next=0; continue; fi
			case "$a" in
			-h) cargs+=(-h 127.0.0.1); skip_next=1 ;;
			-p) cargs+=(-p 5432); skip_next=1 ;;
			-h*) cargs+=(-h 127.0.0.1) ;;
			-p*) cargs+=(-p 5432) ;;
			*) cargs+=("$a") ;;
			esac
		done
		docker exec -i -e PGPASSWORD="$PG_PASSWORD" "$CONTAINER" sh -c \
			"'$bin' \"\$@\"; echo \$? > '$rc_file'" sh "${cargs[@]}" || true
		PG_RC="$(docker exec "$CONTAINER" sh -c "cat '$rc_file' 2>/dev/null; exit 0" || echo 1)"
		docker exec "$CONTAINER" rm -f "$rc_file" >/dev/null 2>&1 || true
	fi
	return 0
}
PG_RC=0

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
if ! docker exec "$CONTAINER" sh -c "pg_isready -U $PG_USER -d $DB -q; exit 0" >/dev/null 2>&1; then
  log "ERROR: pg_isready failed on $DB"
  exit 1
fi
if ! docker exec "$CONTAINER" sh -c "psql -X -U $PG_USER -d $DB -tAc 'SELECT 1' >/dev/null 2>&1; exit 0" >/dev/null 2>&1; then
  log "ERROR: psql failed on $DB"
  exit 1
fi

mkdir -p "$BACKUP_DIR"

SNAP_ID=""
SNAP_PID=""
SNAP_TMP=""
SNAP_HOLDER_PID=""
cleanup() {
  rm -f "$PART" "$COUNTS_PART"
  # 快照持有者：杀**宿主侧** psql 客户端即可——服务端 backend 检测到客户端断开后
  # 正常退出（exit 0），不会触发 postmaster 的集群重启。
  # ⚠️ 不要用 pg_terminate_backend：SIGTERM 杀 backend 会被 postmaster 当成 backend 崩溃
  # → 整个集群被拆掉（连带打断生产库连接）。
  if [ -n "$SNAP_HOLDER_PID" ]; then
    kill "$SNAP_HOLDER_PID" 2>/dev/null || true
    wait "$SNAP_HOLDER_PID" 2>/dev/null || true
  fi
  [ -n "$SNAP_TMP" ] && rm -rf "$SNAP_TMP"
  return 0
}
trap cleanup EXIT

# ---- 一致性快照持有者：导出快照并保持事务打开，直到 dump + 基线都跑完 ----
# pg_export_snapshot 需在 REPEATABLE READ 事务内；事务不提交 ⇒ 快照对
# pg_dump --snapshot 与 SET TRANSACTION SNAPSHOT 同时可见。
#
# 持有者必须是**宿主侧**的 psql 进程：① 它的 PID 可直接 kill → 服务端 backend
# 看到客户端断开后**正常退出**（exit 0，不触发 postmaster 的集群重启）；
# ② 它不在 PG 容器的 PID 命名空间里，退出码与 postmaster 无关。
# 宿主机没有兼容客户端时退化为「无一致快照」（基线可能与档漂移，有告警）。
SNAP_TMP="$(mktemp -d)"
SNAP_ID=""
SNAP_PID=""
if [ -n "$HOST_CLIENT_DIR" ]; then
	PGPASSWORD="$PG_PASSWORD" PGHOST="$PG_HOST" PGPORT="$PG_PORT" PGUSER="$PG_USER" \
		"$HOST_CLIENT_DIR/psql" -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$DB" -At -v ON_ERROR_STOP=1 \
		>"$SNAP_TMP/snapshot.txt" 2>"$SNAP_TMP/snapshot.err" <<'SQL' &
BEGIN ISOLATION LEVEL REPEATABLE READ;
SELECT pg_backend_pid();
SELECT pg_export_snapshot();
SELECT pg_sleep(3600);
ROLLBACK;
SQL
	SNAP_HOLDER_PID=$!

	for _ in $(seq 1 30); do
		SNAP_ID="$(grep -Eo '^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{8}-[0-9]+$' "$SNAP_TMP/snapshot.txt" 2>/dev/null | head -1 || true)"
		[ -n "$SNAP_ID" ] && break
		kill -0 "$SNAP_HOLDER_PID" 2>/dev/null || break
		sleep 1
	done
	SNAP_PID="$(grep -Eo '^[0-9]+$' "$SNAP_TMP/snapshot.txt" 2>/dev/null | head -1 || true)"
else
	log "WARN: 宿主机无兼容 PG 客户端 → 本次不做一致性快照（基线与档内容可能存在漂移）"
fi

SNAP_ARGS=()
if [ -n "$SNAP_ID" ]; then
  log "snapshot exported: $SNAP_ID"
  SNAP_ARGS=("--snapshot=$SNAP_ID")
else
  log "WARN: 一致性快照不可用（$(tr '\n' ' ' <"$SNAP_TMP/snapshot.err" 2>/dev/null)）"
  log "WARN: 本次基线与档内容可能存在漂移（dump 窗口内的写入会造成行数基线偏移）"
fi

# ---- 档名去重：同一秒内重复跑备份会撞名（后一次覆盖前一次，轮转计数失真） ----
while [ -e "$BACKUP_DIR/pg-key-$STAMP.dump" ]; do
	sleep 1
	STAMP="$(date +%Y%m%d-%H%M%S)"
	PART="$BACKUP_DIR/.pg-key-$STAMP.dump.part"
	COUNTS_PART="$BACKUP_DIR/.pg-key-$STAMP.counts.tsv.part"
done

# ---- dump（宿主客户端与服务端同 major 版本；-Fc 压缩；stdout 流出落盘临时名） ----
log "dumping key tables -> $PART"
pg_client pg_dump -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$DB" -n public -Fc "${SNAP_ARGS[@]}" "${EXCLUDE_ARGS[@]}" > "$PART"
if [ "$PG_RC" -ne 0 ]; then
	rm -f "$PART"
	log "ERROR: pg_dump failed"
	exit 1
fi

# ---- 校验：pg_restore -l 能完整列出目录才算成功 ----
pg_client pg_restore -l < "$PART" > /dev/null 2>&1
if [ "$PG_RC" -ne 0 ]; then
	rm -f "$PART"
	log "ERROR: pg_restore -l validation failed"
	exit 1
fi

# ---- 档内表清单（从 TOC 解析，保证「基线覆盖的表」== 「档里真实有的表」） ----
TABLES="$(pg_client pg_restore -l < "$PART" 2>/dev/null |
	awk '$4 == "TABLE" && $5 == "public" && $6 ~ /^[a-z_][a-z0-9_]*$/ {print $6}' | sort -u)"
if [ -z "$TABLES" ]; then
  rm -f "$PART"
  log "ERROR: 无法从 dump TOC 解析出表清单"
  exit 1
fi
TABLE_COUNT="$(printf '%s\n' "$TABLES" | grep -c .)"
log "archive tables: $TABLE_COUNT"

# ---- 行数基线：与 dump 同一快照（无快照时退化为 dump 之后的即时查询） ----
{
  echo "BEGIN ISOLATION LEVEL REPEATABLE READ;"
  [ -n "$SNAP_ID" ] && echo "SET TRANSACTION SNAPSHOT '$SNAP_ID';"
  first=1
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    if [ "$first" -eq 1 ]; then first=0; else echo " UNION ALL"; fi
    printf "SELECT '%s' AS t, count(*)::bigint AS n FROM public.\"%s\"" "$t" "$t"
  done <<< "$TABLES"
  echo ";"
  echo "COMMIT;"
} >"$SNAP_TMP/counts.sql"

if ! pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$DB" -At -v ON_ERROR_STOP=1 \
  <"$SNAP_TMP/counts.sql" | awk -F'|' 'NF == 2 {print $1 "\t" $2}' >"$COUNTS_PART"; then
  rm -f "$PART" "$COUNTS_PART"
  log "ERROR: 行数基线采集失败"
  exit 1
fi

BASELINE_COUNT="$(grep -c . "$COUNTS_PART" || true)"
if [ "$BASELINE_COUNT" != "$TABLE_COUNT" ]; then
  rm -f "$PART" "$COUNTS_PART"
  log "ERROR: 基线表数($BASELINE_COUNT) 与档内表数($TABLE_COUNT) 不一致"
  exit 1
fi

# ---- 改名 + 汇报 ----
FINAL="$BACKUP_DIR/pg-key-$STAMP.dump"
FINAL_COUNTS="$FINAL.counts.tsv"
mv "$PART" "$FINAL"
mv "$COUNTS_PART" "$FINAL_COUNTS"
log "backup ok: $FINAL ($(du -h "$FINAL" | cut -f1))"
log "baseline ok: $FINAL_COUNTS ($BASELINE_COUNT tables)"

# ---- 轮转：仅成功备份后执行，保留最近 KEEP 份（按档名时间戳字典序，勿用 mtime——
# touch/cp 会污染 mtime 导致误删真备份）；基线文件随对应 dump 一并删除 ----
while IFS= read -r old; do
  rm -f "$old" "$old.counts.tsv"
  log "rotated: $old"
done < <(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | sort | head -n -"$KEEP")
# 清孤儿基线文件（对应 dump 已不存在——轮转中断/手工删除档时可能残留）
while IFS= read -r orphan; do
  rm -f "$orphan"
  log "rotated orphan baseline: $orphan"
done < <(
  for f in "$BACKUP_DIR"/pg-key-*.dump.counts.tsv; do
    [ -e "$f" ] || continue
    [ -e "${f%.counts.tsv}" ] || echo "$f"
  done
)
log "rotation done: kept $(ls -1 "$BACKUP_DIR"/pg-key-*.dump 2>/dev/null | wc -l) backup(s), $(ls -1 "$BACKUP_DIR"/pg-key-*.dump.counts.tsv 2>/dev/null | wc -l) baseline(s)"
