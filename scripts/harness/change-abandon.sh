#!/usr/bin/env bash
# change-abandon.sh — change 放弃终态登记（change: add-change-lifecycle-retro，design D5）
#
# 用法：bash scripts/harness/change-abandon.sh <change-name>
#
# 语义（spec harness-fact-log「change 终态记账」）：对「不再推进且不会归档」的 change
# 直写一条 change.abandon 事件（与 change.archive 家族对称）：payload {name}、change 列同名、
# append-only 不去重（重跑登记记多条，消费侧按名聚合天然幂等）。保留期 30 天（TTL 清扫同
# change.archive，登记在 .pi/extensions/lib/harness-log.ts）。
#
# 写入方为专用登记脚本直写账本（先例：test-patrol.sh 直写 patrol.check）——openspec CLI 无
# abandon 原生命令可配对拦截，不做伪命令形态。session_id 取 PI_SESSION_ID 或脚本名。
#
# 校验（fail-open 零伪造：参数非法一律零记录非零退出）：
#   1. 参数恰好一个且匹配合法 change 名字符集（[A-Za-z0-9][A-Za-z0-9._-]*）
#   2. openspec/changes/<name>/ 目录存在（不存在的 change 不登记放弃）
#   3. 未归档：openspec/changes/archive/ 下无 <date>-<name> 目录（已归档 → 拒绝，
#      终态以 change.archive 事实为准，提示误操作）；误传归档目录名（带日期前缀）同样拒绝并提示
#
# 开库安全（对齐 harness-log.ts design D2 / test-patrol.sh）：application_id 魔数校验，
# 他人库拒绝写入；全新库登记魔数后建表；写 PRAGMA 在校验通过之后。
#
# 环境变量（smoke 注入用）：
#   CHANGE_ABANDON_ROOT  仓库根（缺省脚本定位的仓库根；openspec 树与账本都相对它解析）
#   CHANGE_ABANDON_DB    账本路径（缺省 <root>/.pi/harness/events.db）
#   PI_SESSION_ID        记账 session_id（缺省 change-abandon.sh）
#
# 退出码：0 登记成功；1 参数/状态校验失败（零记录）；2 环境错误（库不可写等，零记录）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${CHANGE_ABANDON_ROOT:-$REPO_ROOT}"
DB_PATH="${CHANGE_ABANDON_DB:-$ROOT/.pi/harness/events.db}"
SESSION_ID="${PI_SESSION_ID:-change-abandon.sh}"
APP_ID=1398361684 # 0x53594E54 = ASCII "SYNT" 小端打包（harness-log.ts 同值）
SCHEMA_VERSION=1

CHANGES_DIR="$ROOT/openspec/changes"
ARCHIVE_DIR="$CHANGES_DIR/archive"

EVENTS_DDL="
CREATE TABLE IF NOT EXISTS events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,
  session_id TEXT NOT NULL,
  kind       TEXT NOT NULL,
  change     TEXT,
  payload    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ev_session ON events(session_id, id);
CREATE INDEX IF NOT EXISTS idx_ev_change  ON events(change, id);
CREATE INDEX IF NOT EXISTS idx_ev_kind_ts ON events(kind, ts);
"

usage() {
	cat <<'USAGE'
change-abandon — 登记 change 放弃终态（直写 change.abandon 事件，与 change.archive 对称）

用法: bash scripts/harness/change-abandon.sh <change-name>

  <change-name>  须存在于 openspec/changes/ 且未归档（archive/ 下无同名目录）

语义: 该 change 不再推进且不会归档；重跑登记 append-only 记多条（消费侧按名聚合幂等）。
退出码: 0 登记成功；1 参数/状态校验失败（零记录）；2 环境错误（零记录）。
USAGE
}

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

# ---------- 1. 参数校验 ----------

[ "$#" -eq 1 ] || {
	usage >&2
	[ "$#" -eq 0 ] && printf '✗ 缺少 change 名参数\n' >&2 || printf '✗ 恰好接受一个参数（收到 %s 个）\n' "$#" >&2
	exit 1
}
NAME="$1"
case "$NAME" in
'' | *[!A-Za-z0-9._-]*)
	fail "change 名非法: '$NAME'（合法字符集 [A-Za-z0-9][A-Za-z0-9._-]*）"
	;;
esac
case "$NAME" in
[!A-Za-z0-9]*)
	fail "change 名须以字母或数字开头: '$NAME'"
	;;
esac

# 误传归档目录名（YYYY-MM-DD-<name>）：显式拒绝并提示，不剥前缀猜测（实现取「明确报错」，smoke 断言）
case "$NAME" in
[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-*)
	fail "看起来传的是归档目录名 '$NAME'（带 YYYY-MM-DD- 日期前缀）：已归档 change 终态以 change.archive 事实为准，无需登记放弃"
	;;
esac

# ---------- 2. change 状态校验 ----------

[ -d "$CHANGES_DIR/$NAME" ] || fail "change 不存在于 openspec/changes/: '$NAME'（放弃终态只登记真实存在过的 change，不伪造）"
if compgen -G "$ARCHIVE_DIR/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-$NAME" >/dev/null; then
	fail "change 已归档（$ARCHIVE_DIR/ 下存在 *-$NAME 目录）：终态以 change.archive 事实为准，无需登记放弃"
fi

# ---------- 3. 开库安全（对齐 harness-log.ts design D2） ----------

mkdir -p "$(dirname "$DB_PATH")" 2>/dev/null || {
	printf '✗ 无法创建账本目录: %s\n' "$(dirname "$DB_PATH")" >&2
	exit 2
}
probe="$(sqlite3 -batch "$DB_PATH" "PRAGMA application_id; PRAGMA user_version; SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';" 2>&1)" || {
	printf '✗ 无法打开账本: %s\n  %s\n' "$DB_PATH" "$(printf '%s' "$probe" | tr '\n' ' ')" >&2
	exit 2
}
APP_ID_NOW="$(printf '%s\n' "$probe" | sed -n 1p)"
USER_VER="$(printf '%s\n' "$probe" | sed -n 2p)"
TABLE_COUNT="$(printf '%s\n' "$probe" | sed -n 3p)"
if [ "$APP_ID_NOW" = "0" ] && [ "$TABLE_COUNT" = "0" ]; then
	sqlite3 -batch "$DB_PATH" "PRAGMA application_id = $APP_ID; PRAGMA user_version = $SCHEMA_VERSION;" >/dev/null 2>&1 || {
		printf '✗ 新库登记 application_id 失败（文件不可写？）\n' >&2
		exit 2
	}
elif [ "$APP_ID_NOW" = "$APP_ID" ] && [ "$USER_VER" -le "$SCHEMA_VERSION" ] 2>/dev/null; then
	: # 本应用库（当前版本或半初始化 version=0）
else
	if [ "$APP_ID_NOW" = "0" ]; then
		printf '✗ app_id 未登记但已有 %s 张用户表（他人未登记库，拒绝写入）\n' "$TABLE_COUNT" >&2
	elif [ "$APP_ID_NOW" != "$APP_ID" ]; then
		printf '✗ app_id %s 非本应用魔数 %s（他人库，拒绝写入）\n' "$APP_ID_NOW" "$APP_ID" >&2
	else
		printf '✗ user_version %s 高于当前支持的 %s（未来版本库，拒绝写入）\n' "$USER_VER" "$SCHEMA_VERSION" >&2
	fi
	exit 2
fi
if ! sqlite3 -batch "$DB_PATH" "PRAGMA busy_timeout=5000; PRAGMA journal_mode = WAL;$EVENTS_DDL" >/dev/null 2>&1; then
	printf '✗ 建表失败（库不可写？）\n' >&2
	exit 2
fi

# ---------- 4. 直写事件（append-only 不去重） ----------

NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PAYLOAD="{\"name\":\"$NAME\"}" # NAME 已过字符集白名单，无需转义
if ! sqlite3 -batch "$DB_PATH" "PRAGMA busy_timeout=5000; BEGIN IMMEDIATE;
	INSERT INTO events (ts, session_id, kind, change, payload)
	VALUES ('$NOW', '$SESSION_ID', 'change.abandon', '$NAME', '$PAYLOAD');
	COMMIT;" >/dev/null 2>&1; then
	printf '✗ 写入 change.abandon 失败（库被锁或只读？零记录，可重试）\n' >&2
	exit 2
fi

printf '✓ 已登记放弃终态：change.abandon（name=%s，session=%s，%s）\n' "$NAME" "$SESSION_ID" "$DB_PATH"
printf "  消费侧（harness-retro --lifecycle / B' 幸存者偏差统计）将把该 change 区分为「已放弃」而非「活跃」。\n"
exit 0
