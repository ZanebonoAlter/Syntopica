#!/usr/bin/env bash
# restore-key-tables.sh — 从备份档恢复关键业务表到一次性独立库（灾备恢复通道）
#
# 用法：
#   bash scripts/db/restore-key-tables.sh --dump <档> [选项]
#
# 选项：
#   --dump <档>        备份档路径（必填）。同目录的 <档>.counts.tsv 作为行数基线。
#   --target-db <库>   目标库名。缺省 = 新建一次性独立库（syntopica_restore_<时间戳>）。
#                      目标库名 == 生产库名时拒绝执行（除非 --allow-prod + 交互二次确认）。
#   --allow-prod       允许把档恢复到生产库（仍需交互二次确认：输入库名）。
#   --keep             成功后保留目标库（缺省成功后删除目标库）。
#   --report <输出>    报告输出路径（缺省只打印到 stdout）。
#   --help             本用法。
#
# 环境变量（仅供冒烟脚本在测试库上跑；生产缺省值不变）：
#   RESTORE_CONTAINER  容器名（缺省 syntopica-postgres）
#   RESTORE_PG_USER    库用户（缺省 postgres）
#   RESTORE_PROD_DB    生产库名（缺省 syntopica）—— 落库硬闸据此判定
#   RESTORE_EXTENSIONS 预装扩展列表（缺省 "vector pg_trgm"）
#   RESTORE_MIGRATION_SRC 迁移源码路径（缺省后端仓库内的 postgres_migrations.go）
#
# 行为契约（openspec capability: disaster-recovery）：
#   1. 落库闸门：缺省新建一次性独立恢复库；目标库名 == 生产库名时非零退出，
#      除非 --allow-prod 且交互二次确认通过。生产库不被写入。
#   2. 扩展预装：restore 之前在目标库建 vector 与 pg_trgm；失败则**在 restore 之前**
#      非零退出并清掉刚建的库（不留半恢复库）。
#   3. TOC 过滤：过滤 SCHEMA public 条目（恢复到已有 public schema 的库会撞名）。
#   4. count 校验：读 <档>.counts.tsv 基线逐表比对恢复库行数；不一致 → 报告差异表
#      并非零退出。档内无基线（L1 之前的历史档）→ 跳过校验并在报告里写明。
#   5. 迁移状态对账：解析 backend-go/internal/platform/database/postgres_migrations.go
#      各迁移闭包触及的表名，与恢复库实际表集合比对，列出「迁移已应用但表不在档内」
#      的缺口。缺口只进报告，不影响退出码（可再生表由 AutoMigrate 重建）。
#   6. 报告：档来源 / 目标库 / count 比对 / 迁移对账 / 耗时。
#
# 退出码：0 成功｜1 用法或前置校验失败｜2 落库闸门拒绝｜3 扩展预装失败
#         4 pg_restore 失败｜5 count 校验不一致
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTAINER="${RESTORE_CONTAINER:-syntopica-postgres}"
PG_USER="${RESTORE_PG_USER:-postgres}"
PG_PASSWORD="${RESTORE_PG_PASSWORD:-postgres}"
PG_HOST="${RESTORE_PG_HOST:-127.0.0.1}"
PG_PORT="${RESTORE_PG_PORT:-5432}"
PROD_DB="${RESTORE_PROD_DB:-syntopica}"
MIGRATION_SRC="${RESTORE_MIGRATION_SRC:-$REPO_ROOT/backend-go/internal/platform/database/postgres_migrations.go}"
EXTENSIONS="${RESTORE_EXTENSIONS:-vector pg_trgm}"

DUMP=""
TARGET_DB=""
ALLOW_PROD=0
KEEP=0
REPORT=""

EXIT_USAGE=1
EXIT_GATE=2
EXIT_EXTENSION=3
EXIT_RESTORE=4
EXIT_COUNT=5

WORK_DIR=""

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
err() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2; }

usage() {
	sed -n '2,40p' "$0"
	exit "$EXIT_USAGE"
}

cleanup() {
	[ -n "$WORK_DIR" ] && rm -rf "$WORK_DIR"
	return 0
}
trap cleanup EXIT

# ---------- PG 客户端定位（重要：客户端**不进容器**） ----------
#
# 2026-11-05 实测：`docker exec` 起的客户端进程是容器里 postmaster(PID 1) 的子进程，
# 客户端**非零退出会被 postmaster 记成「unknown child exited abnormally」→ 整个集群被拆掉**
# （terminating any other active server processes → 崩溃恢复，连带打断生产库连接）。
# 也就是说：一个「恢复失败」会把生产库拖下水。
#
# 对策：客户端一律在**宿主机**跑（连同一个发布端口），退出码与 postmaster 无关；
# 宿主机没有兼容客户端时退回容器内执行，但**必须屏蔽退出码**（让 postmaster 只看到 0）。
HOST_CLIENT_DIR=""
for d in /usr/lib/postgresql/*/bin /usr/pgsql-*/bin /usr/local/pgsql/bin /usr/bin; do
	[ -x "$d/pg_restore" ] || continue
	HOST_CLIENT_DIR="$d"
done
if [ -n "$HOST_CLIENT_DIR" ]; then
	HOST_PG_MAJOR="$("$HOST_CLIENT_DIR/pg_restore" --version | sed -E 's/.* ([0-9]+)\.[0-9]+.*/\1/')"
	SERVER_PG_MAJOR="$(docker exec "$CONTAINER" sh -c "psql -X -U $PG_USER -d postgres -tAc 'show server_version' 2>/dev/null; exit 0" | cut -d. -f1 | tr -dc '0-9' || echo 0)"
	if [ -n "$SERVER_PG_MAJOR" ] && [ "$SERVER_PG_MAJOR" != "0" ] && [ "${HOST_PG_MAJOR:-0}" -lt "$SERVER_PG_MAJOR" ]; then
		HOST_CLIENT_DIR="" # 客户端比 server 老 → 不能用（pg_dump/pg_restore 会拒）
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

# ---------- 参数解析 ----------

while [ $# -gt 0 ]; do
	case "$1" in
	--dump)
		DUMP="${2:-}"
		shift 2 || true
		;;
	--target-db)
		TARGET_DB="${2:-}"
		shift 2 || true
		;;
	--allow-prod)
		ALLOW_PROD=1
		shift
		;;
	--keep)
		KEEP=1
		shift
		;;
	--report)
		REPORT="${2:-}"
		shift 2 || true
		;;
	--help | -h)
		usage
		;;
	*)
		err "未知参数：$1"
		usage
		;;
	esac
done

if [ -z "$DUMP" ]; then
	err "缺少 --dump <备份档>"
	usage
fi

# ---------- 前置校验 ----------

if [ ! -f "$DUMP" ]; then
	err "备份档不存在：$DUMP"
	exit "$EXIT_USAGE"
fi
if [ ! -r "$DUMP" ]; then
	err "备份档不可读：$DUMP"
	exit "$EXIT_USAGE"
fi
DUMP="$(cd "$(dirname "$DUMP")" && pwd)/$(basename "$DUMP")"
BASELINE="$DUMP.counts.tsv"
DB_SIZE="$(du -h "$DUMP" | cut -f1)"

state="$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo missing)"
if [ "$state" != "true" ]; then
	err "容器 $CONTAINER 未运行（state=$state）"
	exit "$EXIT_USAGE"
fi
if [ -n "$HOST_CLIENT_DIR" ]; then
	pg_client pg_isready -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -q
	READY_RC=$PG_RC
else
	docker exec "$CONTAINER" sh -c "pg_isready -U $PG_USER -q; exit 0" >/dev/null 2>&1 && READY_RC=0 || READY_RC=1
fi
if [ "$READY_RC" -ne 0 ]; then
	err "pg_isready 失败（容器 $CONTAINER / $PG_HOST:$PG_PORT）"
	exit "$EXIT_USAGE"
fi

WORK_DIR="$(mktemp -d)"
TOC_ALL="$WORK_DIR/toc.all"
TOC_USED="$WORK_DIR/toc.used"

# 档必须可列目录（损坏的档在这里就被挡住，不必等到 restore 中途）
pg_client pg_restore -l <"$DUMP" >"$TOC_ALL" 2>"$WORK_DIR/toc.err"
if [ "$PG_RC" -ne 0 ]; then
	err "pg_restore -l 列目录失败（档可能损坏）：$(tr '\n' ' ' <"$WORK_DIR/toc.err")"
	exit "$EXIT_USAGE"
fi
if [ ! -s "$TOC_ALL" ]; then
	err "pg_restore -l 输出为空，档不可用"
	exit "$EXIT_USAGE"
fi

# ---------- 落库闸门 ----------

# 库名白名单（拼进 SQL，必须是合法标识符）
if [ -n "$TARGET_DB" ] && ! [[ "$TARGET_DB" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
	err "非法目标库名：$TARGET_DB"
	exit "$EXIT_USAGE"
fi

db_exists() {
	pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname = '$1'" 2>/dev/null | grep -q 1
}

DB_EXISTS=0
if [ -n "$TARGET_DB" ] && db_exists "$TARGET_DB"; then
	DB_EXISTS=1
fi

if [ -n "$TARGET_DB" ] && [ "$TARGET_DB" = "$PROD_DB" ]; then
	if [ "$ALLOW_PROD" -ne 1 ]; then
		err "拒绝执行：目标库 '$TARGET_DB' 是生产库。生产库恢复必须显式加 --allow-prod 并通过交互二次确认。"
		exit "$EXIT_GATE"
	fi
	echo "⚠️  即将把备份档恢复到**生产库** '$TARGET_DB'。" >&2
	echo "    这会覆盖生产库中的同名对象。继续请输入库名 '$TARGET_DB'（其它输入一律取消）：" >&2
	CONFIRM=""
	if [ -t 0 ]; then
		read -r CONFIRM || CONFIRM=""
	else
		read -r CONFIRM || CONFIRM=""
	fi
	if [ "$CONFIRM" != "$TARGET_DB" ]; then
		err "二次确认未通过（输入 '$CONFIRM'），已取消。生产库未被触碰。"
		exit "$EXIT_GATE"
	fi
	log "二次确认通过：允许恢复到生产库 $TARGET_DB"
elif [ -n "$TARGET_DB" ] && [ "$DB_EXISTS" -eq 1 ]; then
	err "拒绝执行：目标库 '$TARGET_DB' 已存在。为避免污染既有数据，请换一个库名（或先人工确认后手动删除）。"
	exit "$EXIT_GATE"
fi

CREATED_DB=0
if [ -z "$TARGET_DB" ]; then
	TARGET_DB="${PROD_DB}_restore_$(date +%Y%m%d_%H%M%S)"
	suffix=1
	while db_exists "$TARGET_DB"; do
		TARGET_DB="${PROD_DB}_restore_$(date +%Y%m%d_%H%M%S)_$suffix"
		suffix=$((suffix + 1))
	done
fi

if [ "$DB_EXISTS" -eq 0 ]; then
	log "新建一次性恢复库：$TARGET_DB"
	pg_client createdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" "$TARGET_DB" >/dev/null 2>&1
	if [ "$PG_RC" -ne 0 ]; then
		err "创建恢复库失败：$TARGET_DB"
		exit "$EXIT_USAGE"
	fi
	CREATED_DB=1
else
	log "目标库已存在：$TARGET_DB（直接恢复其中）"
fi

# 恢复失败时保留现场（便于人工排查）；成功且未 --keep 时删除目标库。
drop_target_db() {
	[ "$CREATED_DB" -eq 1 ] || return 0
	pg_client dropdb -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" --if-exists "$TARGET_DB" >/dev/null 2>&1 || true
}

# ---------- 扩展预装（restore 之前） ----------

EXT_OK=1
for ext in $EXTENSIONS; do
	if ! [[ "$ext" =~ ^[a-z_][a-z0-9_]*$ ]]; then
		EXT_OK=0
		EXT_ERR="非法扩展名 '$ext'"
		err "扩展预装失败：$EXT_ERR"
		break
	fi
	pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$TARGET_DB" -v ON_ERROR_STOP=1 -q \
		-c "CREATE EXTENSION IF NOT EXISTS $ext" >/dev/null 2>"$WORK_DIR/ext-$ext.err"
	if [ "$PG_RC" -ne 0 ]; then
		EXT_OK=0
		EXT_ERR="$(tr '\n' ' ' <"$WORK_DIR/ext-$ext.err")"
		err "扩展预装失败：$ext —— $EXT_ERR"
		break
	fi
done
if [ "$EXT_OK" -ne 1 ]; then
	err "扩展缺失，未进入 restore 阶段；已清理目标库 $TARGET_DB（不留半恢复库）"
	drop_target_db
	exit "$EXIT_EXTENSION"
fi
log "扩展预装完成：vector / pg_trgm"

# ---------- TOC 过滤（去掉 SCHEMA public） ----------

awk '
	/^[[:space:]]*;/ { next }                      # 注释行
	$4 == "SCHEMA" && $6 == "public" { next }      # SCHEMA public（恢复到已有 schema 会撞名）
	{ print }
' "$TOC_ALL" >"$TOC_USED"
TOC_ENTRIES_N="$(grep -c . "$TOC_ALL" || true)"
FILTERED_N="$(grep -c . "$TOC_USED" || true)"
log "TOC 条目：原始 $TOC_ENTRIES_N 条 → 过滤后 $FILTERED_N 条"

# ---------- pg_restore ----------

START_TS="$(date +%s)"
log "pg_restore 开始 → $TARGET_DB"
pg_client pg_restore -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$TARGET_DB" \
	-L "$TOC_USED" --exit-on-error <"$DUMP" >"$WORK_DIR/restore.out" 2>"$WORK_DIR/restore.err"
if [ "$PG_RC" -ne 0 ]; then
	err "pg_restore 失败："
	sed 's/^/    /' "$WORK_DIR/restore.err" | tail -30 >&2
	err "目标库 $TARGET_DB 已保留（供排查）。"
	exit "$EXIT_RESTORE"
fi
RESTORE_SEC=$(( $(date +%s) - START_TS ))
log "pg_restore 完成（${RESTORE_SEC}s）"

# ---------- count 完整性校验 ----------

COUNT_STATUS="跳过（该档无 .counts.tsv 基线）"
COUNT_MISMATCH=0
COUNT_TABLE_N=0
: >"$WORK_DIR/count-diff.txt"
: >"$WORK_DIR/count-all.tsv"

if [ -f "$BASELINE" ]; then
	# 表名白名单校验（基线来自备份产物，只接受合法标识符，防 SQL 注入）
	grep -vE '^[a-z_][a-z0-9_]*[[:space:]]+[0-9]+$' "$BASELINE" >/dev/null 2>&1 || true
	BASELINE_TABLES="$(awk -F'\t' 'NF==2 && $1 ~ /^[a-z_][a-z0-9_]*$/ && $2 ~ /^[0-9]+$/ {print $1}' "$BASELINE")"
	COUNT_TABLE_N="$(printf '%s\n' "$BASELINE_TABLES" | grep -c . || true)"
	if [ "$COUNT_TABLE_N" -eq 0 ]; then
		COUNT_STATUS="跳过（基线清单格式不可用）"
	else
		{
			echo "BEGIN;"
			first=1
			while IFS= read -r t; do
				[ -n "$t" ] || continue
				if [ "$first" -eq 1 ]; then first=0; else echo " UNION ALL"; fi
				printf "SELECT '%s' AS t, count(*)::bigint AS n FROM public.\"%s\"" "$t" "$t"
			done <<<"$BASELINE_TABLES"
			echo ";"
		} >"$WORK_DIR/counts.sql"
		ACTUAL_COUNTS="$(pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$TARGET_DB" -At -v ON_ERROR_STOP=1 \
			<"$WORK_DIR/counts.sql" 2>"$WORK_DIR/counts.err" | awk -F'|' 'NF==2 {print $1 "\t" $2}')" || true
		if [ ! -s "$WORK_DIR/counts.err" ]; then
			printf '%s\n' "$ACTUAL_COUNTS" >"$WORK_DIR/actual-counts.tsv"
		else
			# 基线里的表在恢复库中不存在 → count 查询直接报错（表名不存在）；
			# 这本身就是「不一致」，逐表比对会把它记成 MISSING。
			err "count 查询报错（基线表在恢复库中缺失？）：$(tr '\n' ' ' <"$WORK_DIR/counts.err" | cut -c1-300)"
			: >"$WORK_DIR/actual-counts.tsv"
		fi
		while IFS=$'\t' read -r table expected; do
			[ -n "$table" ] || continue
			actual="$(awk -F'\t' -v t="$table" '$1==t {print $2}' "$WORK_DIR/actual-counts.tsv" | head -1)"
			[ -n "$actual" ] || actual="MISSING"
			if [ "$actual" = "$expected" ]; then
				printf '%s\t%s\t%s\tOK\n' "$table" "$expected" "$actual" >>"$WORK_DIR/count-all.tsv"
			else
				COUNT_MISMATCH=$((COUNT_MISMATCH + 1))
				printf '%s\t%s\t%s\tMISMATCH\n' "$table" "$expected" "$actual" >>"$WORK_DIR/count-all.tsv"
				printf '%s\t基线 %s\t实际 %s\n' "$table" "$expected" "$actual" >>"$WORK_DIR/count-diff.txt"
			fi
		done <"$BASELINE"
		if [ "$COUNT_MISMATCH" -eq 0 ]; then
			COUNT_STATUS="通过（$COUNT_TABLE_N 张表全部一致）"
		else
			COUNT_STATUS="不通过（$COUNT_TABLE_N 张表中 $COUNT_MISMATCH 张不一致）"
		fi
	fi
fi

# ---------- 迁移状态对账 ----------

RECONCILE_SOURCE="无（迁移源码不可读）"
RECONCILE_APPLIED_N=0
RECONCILE_GAPS=0
: >"$WORK_DIR/gaps.txt"
: >"$WORK_DIR/reconcile-pairs.tsv"
: >"$WORK_DIR/dropped-tables.txt"

if [ -f "$MIGRATION_SRC" ]; then
	RECONCILE_SOURCE="backend-go/internal/platform/database/postgres_migrations.go"
	# 解析：Version 行切块，块内抓「动词 + TABLE + 表名」；DROP TABLE 的目标归入「已删除」桶
	# （被迁移删掉的表本就不该在档里，不算缺口）。
	# ⚠️ 必须要求「动词 + TABLE」成对出现：只匹配 TABLE <词> 会把
	# CREATE INDEX / ADD COLUMN / CREATE UNIQUE INDEX 里的关键字当成表名。
	awk '
		/^[[:space:]]*\/\// { next }
		/Version:[[:space:]]*"/ {
			v = $0
			sub(/.*Version:[[:space:]]*"/, "", v)
			sub(/".*/, "", v)
			cur = v
			next
		}
		{
			line = $0
			while (match(line, /(DROP|CREATE|ALTER|TRUNCATE)[[:space:]]+TABLE[[:space:]]+(IF NOT EXISTS |IF EXISTS |ONLY )?[A-Za-z_][A-Za-z0-9_]*/)) {
				s = substr(line, RSTART, RLENGTH)
				t = s
				sub(/^[A-Za-z]+[[:space:]]+TABLE[[:space:]]+(IF NOT EXISTS |IF EXISTS |ONLY )?/, "", t)
				if (s ~ /^DROP/) {
					dropped[t] = 1
				} else if (cur == "") {
					pre[t] = 1
				} else {
					pairs[cur "\t" t] = 1
				}
				line = substr(line, RSTART + RLENGTH)
			}
		}
		END {
			for (p in pairs) print p
			for (t in pre) print "(前置步骤)\t" t
			for (t in dropped) print "(已删除)\t" t
		}
	' "$MIGRATION_SRC" | sort -u >"$WORK_DIR/reconcile-pairs.tsv"

	# 恢复库实际表集合
	pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$TARGET_DB" -At -c \
		"SELECT table_name FROM information_schema.tables
		 WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name" \
		>"$WORK_DIR/restored-tables.txt" 2>/dev/null || true

	# 已应用迁移集合：恢复库带 schema_migrations 时以它为准；
	# 历史档（无该表）→ 回退「源码里的全部迁移」并在报告里标注。
	APPLIED_SRC="$WORK_DIR/applied.txt"
	if grep -qx "schema_migrations" "$WORK_DIR/restored-tables.txt" 2>/dev/null; then
		pg_client psql -X -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$TARGET_DB" -At -c \
			"SELECT version FROM schema_migrations ORDER BY version" >"$APPLIED_SRC" 2>/dev/null || true
		RECONCILE_APPLIED_SRC="恢复库 schema_migrations 表"
	else
		awk -F'\t' '$1 != "(前置步骤)" && $1 != "(已删除)" {print $1}' "$WORK_DIR/reconcile-pairs.tsv" \
			>"$APPLIED_SRC"
		RECONCILE_APPLIED_SRC="恢复库无 schema_migrations（历史档）→ 回退为源码中的全部迁移"
	fi
	RECONCILE_APPLIED_N="$(grep -c . "$APPLIED_SRC" || true)"

	# 缺口 = 已应用迁移触及、但恢复库里没有的表（不含被迁移删除的表）
	while IFS=$'\t' read -r version table; do
		[ -n "$table" ] || continue
		[ "$version" = "(已删除)" ] && continue
		grep -qxF "$version" "$APPLIED_SRC" || continue
		grep -qxF "$table" "$WORK_DIR/restored-tables.txt" && continue
		RECONCILE_GAPS=$((RECONCILE_GAPS + 1))
		printf '%s\t%s\n' "$version" "$table" >>"$WORK_DIR/gaps.txt"
	done <"$WORK_DIR/reconcile-pairs.tsv"
fi

# ---------- 报告 ----------

TOTAL_SEC=$(( $(date +%s) - START_TS ))
REPORT_TEXT="$WORK_DIR/report.md"
{
	echo "# 灾备恢复报告"
	echo
	echo "- 档来源：\`$DUMP\`（$DB_SIZE，生成于 $(date -r "$DUMP" '+%Y-%m-%d %H:%M:%S')）"
	if [ -f "$BASELINE" ]; then
		echo "- 行数基线：\`$BASELINE\`（$(grep -c . "$BASELINE" || true) 张表）"
	else
		echo "- 行数基线：无（该档是 L1 修复前产出的历史档，无 .counts.tsv）"
	fi
	echo "- 目标库：\`$TARGET_DB\`（$( [ "$CREATED_DB" -eq 1 ] && echo 新建 || echo 已存在 )）"
	echo "- 扩展预装：vector ✓ / pg_trgm ✓"
	echo "- TOC 过滤：SCHEMA public 条目已剔除（过滤后 $FILTERED_N 条）"
	echo "- pg_restore 耗时：${RESTORE_SEC}s"
	echo "- 总耗时：${TOTAL_SEC}s"
	echo
	echo "## 1. count 完整性校验"
	echo
	echo "结果：**$COUNT_STATUS**"
	echo
	if [ "$COUNT_TABLE_N" -gt 0 ] && [ -s "$WORK_DIR/count-all.tsv" ]; then
		echo "| 表 | 基线行数 | 实际行数 | 结果 |"
		echo "| --- | --- | --- | --- |"
		while IFS=$'\t' read -r t e a r; do
			[ -n "$t" ] || continue
			if [ "$r" = "OK" ]; then
				echo "| \`$t\` | $e | $a | ✅ |"
			else
				echo "| \`$t\` | $e | $a | ❌ |"
			fi
		done <"$WORK_DIR/count-all.tsv"
		echo
	fi
	if [ "$COUNT_MISMATCH" -gt 0 ]; then
		echo "差异明细（基线 → 实际）："
		echo
		echo '```'
		cat "$WORK_DIR/count-diff.txt"
		echo '```'
		echo
	fi
	echo "## 2. 迁移状态对账"
	echo
	echo "- 迁移源码：$RECONCILE_SOURCE"
	echo "- 已应用迁移来源：${RECONCILE_APPLIED_SRC:-未知}"
	echo "- 已应用迁移数：$RECONCILE_APPLIED_N"
	echo "- 恢复库表数：$(grep -c . "$WORK_DIR/restored-tables.txt" 2>/dev/null || echo 0)"
	echo "- 缺口（迁移已应用、但表不在档内）：$RECONCILE_GAPS"
	if [ "$RECONCILE_GAPS" -gt 0 ]; then
		echo
		echo "| 迁移 | 缺失表 |"
		echo "| --- | --- |"
		while IFS=$'\t' read -r v t; do
			[ -n "$t" ] || continue
			echo "| \`$v\` | \`$t\` |"
		done <"$WORK_DIR/gaps.txt"
		echo
		echo "> 这些表属备份排除清单（可再生数据），恢复后由 AutoMigrate 按 model 重建；是否可接受交人工判读（不影响本脚本退出码）。"
	fi
	echo
	echo "## 3. 耗时"
	echo
	echo "- pg_restore：${RESTORE_SEC}s"
	echo "- 总计：${TOTAL_SEC}s"
} >"$REPORT_TEXT"

echo
cat "$REPORT_TEXT"

if [ -n "$REPORT" ]; then
	REPORT_DIR="$(dirname "$REPORT")"
	mkdir -p "$REPORT_DIR"
	cp "$REPORT_TEXT" "$REPORT"
	log "报告已写入：$REPORT"
fi

# ---------- 收尾 ----------

if [ "$COUNT_MISMATCH" -gt 0 ]; then
	err "count 校验不通过：$COUNT_MISMATCH 张表与基线不一致（目标库 $TARGET_DB 已保留供排查）"
	exit "$EXIT_COUNT"
fi

if [ "$KEEP" -eq 1 ]; then
	log "目标库已保留（--keep）：$TARGET_DB"
else
	log "删除一次性恢复库：$TARGET_DB"
	drop_target_db
fi

log "恢复完成（退出码 0）"
exit 0
