#!/usr/bin/env bash
# change-abandon.smoke.sh — scripts/harness/change-abandon.sh 白盒冒烟（change: add-change-lifecycle-retro 2.1）
#
# 覆盖 test-cases.md 故事 L3（终态登记）：登记成功（payload.name/change 同名/session_id）、
# 不存在拒绝、已归档拒绝、误传归档目录名拒绝、空/非法参数 usage 退出、append-only 幂等重跑、
# 他人库拒绝零记录。全部 mktemp 临时 fixture（CHANGE_ABANDON_ROOT / CHANGE_ABANDON_DB 注入），
# 绝不触碰真实 .pi/harness/events.db。
#
# 用法：bash scripts/harness/change-abandon.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE="$SCRIPT_DIR/change-abandon.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/change-abandon-smoke.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

ROOT="$TMP/repo"
DB="$TMP/events.db"
mkdir -p "$ROOT/openspec/changes/zombie-change" "$ROOT/openspec/changes/another-active"
mkdir -p "$ROOT/openspec/changes/archive/2026-09-20-already-archived"

pass=0
fail=0
ok() { printf '  [OK] %s\n' "$1"; pass=$((pass + 1)); }
bad() { printf '  [FAIL] %s\n' "$1" >&2; fail=$((fail + 1)); }

expect_rc() { # $1=期望退出码 $2=描述；RC/OUT 由调用方先跑
	[ "$RC" = "$1" ] && ok "$2" || bad "$2（期望 exit $1，实得 $RC；输出：$(printf '%s' "$OUT" | head -2 | tr '\n' ' ')）"
}
expect_contains() { # $1=子串 $2=描述
	printf '%s' "$OUT" | grep -qF -- "$1" && ok "$2" || bad "$2（输出未含：$1）"
}
rows() { sqlite3 "$DB" "SELECT COUNT(*) FROM events WHERE kind='change.abandon';" 2>/dev/null || printf '0'; }
abandon_rows_of() {
	local q
	q="$(printf '%s' "$1" | sed "s/'/''/g")"
	sqlite3 "$DB" "SELECT COUNT(*) FROM events WHERE kind='change.abandon' AND change='$q';" 2>/dev/null || printf '0'
}

echo "== change-abandon.smoke =="

# ---------- 1. 登记成功（故事 L3-1） ----------
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" PI_SESSION_ID="sess-a" bash "$SOURCE" zombie-change 2>&1)"
RC=$?
expect_rc 0 "L3-1a 存在且未归档的 change → exit 0"
expect_contains "已登记放弃终态" "L3-1b 输出确认登记"
[ "$(rows)" = "1" ] && ok "L3-1c change.abandon 落库恰一条" || bad "L3-1c 落库条数=$(rows)"
[ "$(abandon_rows_of zombie-change)" = "1" ] && ok "L3-1d change 列同名绑定" || bad "L3-1d change 列绑定错"
_payload="$(sqlite3 "$DB" "SELECT payload FROM events WHERE kind='change.abandon';")"
[ "$_payload" = '{"name":"zombie-change"}' ] && ok "L3-1e payload 含 name" || bad "L3-1e payload=$_payload"
_sid="$(sqlite3 "$DB" "SELECT session_id FROM events WHERE kind='change.abandon';")"
[ "$_sid" = "sess-a" ] && ok "L3-1f session_id 取 PI_SESSION_ID" || bad "L3-1f session_id=$_sid"
_ts="$(sqlite3 "$DB" "SELECT ts FROM events WHERE kind='change.abandon';")"
printf '%s' "$_ts" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' && ok "L3-1g ts 为 ISO 时间戳" || bad "L3-1g ts=$_ts"

# session_id 缺省回退脚本名（PI_SESSION_ID 不设）
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" env -u PI_SESSION_ID bash "$SOURCE" another-active 2>&1)"
RC=$?
_sid2="$(sqlite3 "$DB" "SELECT session_id FROM events WHERE kind='change.abandon' AND change='another-active';")"
expect_rc 0 "L3-1h 无 PI_SESSION_ID 时 exit 0"
[ "$_sid2" = "change-abandon.sh" ] && ok "L3-1i session_id 缺省取脚本名（对齐 test-patrol 先例）" || bad "L3-1i session_id=$_sid2"

# ---------- 2. 重复登记：append-only 不去重（故事 L3-4） ----------
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" zombie-change 2>&1)"
RC=$?
expect_rc 0 "L3-4a 重复登记 exit 0"
[ "$(abandon_rows_of zombie-change)" = "2" ] && ok "L3-4b append-only 记两条（消费侧按名聚合幂等）" || bad "L3-4b 条数=$(abandon_rows_of zombie-change)"

# ---------- 3. 不存在拒绝（故事 L3-2） ----------
_before="$(rows)"
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" no-such-change 2>&1)"
RC=$?
expect_rc 1 "L3-2a 不存在的 change → 非零退出"
expect_contains "不存在于 openspec/changes/" "L3-2b 提示不存在"
[ "$(rows)" = "$_before" ] && ok "L3-2c 零记录（fail-open 不伪造终态）" || bad "L3-2c 落了记录"

# ---------- 4. 已归档拒绝（故事 L3-3） ----------
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" already-archived 2>&1)"
RC=$?
expect_rc 1 "L3-3a 已归档 change（archive 目录在但 changes/ 也有同名目录）→ 非零退出"
# changes/ 下没有 already-archived 目录 → 走不存在分支也合规；补一个真实「目录还在但已归档」场景：
mkdir -p "$ROOT/openspec/changes/already-archived"
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" already-archived 2>&1)"
RC=$?
expect_rc 1 "L3-3b 已归档（changes/ 目录仍存在 + archive/ 有对应目录）→ 非零退出"
expect_contains "已归档" "L3-3c 提示已归档（终态以 change.archive 为准）"
[ "$(abandon_rows_of already-archived)" = "0" ] && ok "L3-3d 零记录" || bad "L3-3d 落了记录"

# ---------- 5. 误传归档目录名（变体：日期前缀明确报错） ----------
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" 2026-09-20-already-archived 2>&1)"
RC=$?
expect_rc 1 "V-1a 误传归档目录名（YYYY-MM-DD- 前缀）→ 非零退出"
expect_contains "归档目录名" "V-1b 提示带日期前缀（不剥前缀猜测）"

# ---------- 6. 参数非法 ----------
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" 2>&1)"
RC=$?
expect_rc 1 "V-2a 空参数 → usage 非零退出"
expect_contains "用法" "V-2b 打印用法"
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" a b 2>&1)"
RC=$?
expect_rc 1 "V-2c 两个参数 → 非零退出"
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$DB" bash "$SOURCE" "bad name; rm -rf" 2>&1)"
RC=$?
expect_rc 1 "V-2d 非法字符集 → 非零退出（不进 SQL）"
[ "$(rows)" = "3" ] && ok "V-2e 全部非法路径零新增记录（仍 3 条）" || bad "V-2e 条数=$(rows)"

# ---------- 7. 他人库拒绝（开库安全） ----------
FOREIGN_DB="$TMP/foreign.db"
sqlite3 "$FOREIGN_DB" "PRAGMA application_id = 42; CREATE TABLE events (id INTEGER PRIMARY KEY, ts TEXT, session_id TEXT, kind TEXT, change TEXT, payload TEXT);"
mkdir -p "$ROOT/openspec/changes/foreign-target"
OUT="$(CHANGE_ABANDON_ROOT="$ROOT" CHANGE_ABANDON_DB="$FOREIGN_DB" bash "$SOURCE" foreign-target 2>&1)"
RC=$?
expect_rc 2 "S-1a 他人库（app_id 非 0x53594E54）→ exit 2"
expect_contains "拒绝写入" "S-1b 提示拒绝写入"
_frows="$(sqlite3 "$FOREIGN_DB" "SELECT COUNT(*) FROM events WHERE kind='change.abandon';" 2>/dev/null || printf '0')"
[ "$_frows" = "0" ] && ok "S-1c 他人库零记录" || bad "S-1c 他人库落了记录"

# ---------- 汇总 ----------
echo
echo "== 通过 $pass / 失败 $fail =="
[ "$fail" -eq 0 ] || exit 1
