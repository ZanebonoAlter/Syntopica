#!/usr/bin/env bash
# gate-status.smoke.sh — gate-status.sh 烟测（gate-status-query 四用例 + 只读断言 + 坏库）
# 用例来源：openspec/changes/aggregate-concurrent-gate-warns/test-cases.md 故事 S4。
# 用法：bash scripts/harness/gate-status.smoke.sh
# 退出码：0 = 全部用例通过；1 = 存在失败用例。
set -uo pipefail
cd "$(dirname "$0")/../.."

SCRIPT="scripts/harness/gate-status.sh"
SCRIPT_ABS="$PWD/$SCRIPT"  # 先固定绝对路径：用例里会 cd 进临时仓库
pass=0
fail=0
ok() { echo "✅ $1"; pass=$((pass + 1)); }
ng() { echo "❌ $1"; fail=$((fail + 1)); }

D="$(mktemp -d "${TMPDIR:-/tmp}/gate-status-smoke.XXXXXX")"
trap 'rm -rf "$D"' EXIT

# 桥接：脚本以 git root 定位库，smoke 在临时 git 仓库里跑（gitRouter 同款思路）
make_repo() {
	local r="$1"
	mkdir -p "$r/.pi/harness"
	git -C "$r" init -q 2>/dev/null || true
}

seed() { # repo session kind payload
	sqlite3 "$1/.pi/harness/events.db" \
		"INSERT INTO events(ts, session_id, kind, change, payload) VALUES (datetime('now'), '$2', '$3', NULL, '$4');"
}

init_db() {
	sqlite3 "$1/.pi/harness/events.db" "
    CREATE TABLE events (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      ts TEXT NOT NULL,
      session_id TEXT NOT NULL,
      kind TEXT NOT NULL,
      change TEXT,
      payload TEXT NOT NULL
    );"
}

# ---------------------------------------------------------------------------
# 用例 1：红态展示（含同一 cmd 多条历史 → 取最新；S4 V1/V2）
# ---------------------------------------------------------------------------
R1="$(mktemp -d "${TMPDIR:-/tmp}/gs-r1.XXXXXX")"
make_repo "$R1"
init_db "$R1"
seed "$R1" s1 gate.check '{"cmd":"golangci-lint","ok":false,"ms":10,"diag":"old failure"}'
seed "$R1" s1 gate.check '{"cmd":"golangci-lint","ok":true,"ms":10}'
seed "$R1" s1 gate.check '{"cmd":"golangci-lint","ok":false,"ms":10,"diag":"FAIL syntopica-backend/internal/x [build failed]"}'
seed "$R1" s1 gate.check '{"cmd":"go vet","ok":true,"ms":10}'
out1="$(cd "$R1" && bash "$SCRIPT_ABS" 2>&1)"
rc1=$?
if [ $rc1 -eq 0 ] && printf '%s' "$out1" | grep -q '^\[红\] golangci-lint' \
	&& printf '%s' "$out1" | grep -q 'FAIL syntopica-backend/internal/x' \
	&& ! printf '%s' "$out1" | grep -q 'old failure' \
	&& printf '%s' "$out1" | grep -q '^\[绿\] go vet'; then
	ok "C1 红态展示 + 同 cmd 多条历史取最新（旧 diag 不出现），退出码 0"
else
	ng "C1 红态展示（rc=$rc1）"
	printf '%s\n' "$out1" | sed 's/^/    /'
fi

# ---------------------------------------------------------------------------
# 用例 2：转绿后最新为绿态（rc 仍 0；S4 红→绿翻转）
# ---------------------------------------------------------------------------
seed "$R1" s1 gate.check '{"cmd":"golangci-lint","ok":true,"ms":10,"flip":true}'
out2="$(cd "$R1" && bash "$SCRIPT_ABS" 2>&1)"
rc2=$?
if [ $rc2 -eq 0 ] && printf '%s' "$out2" | grep -q '^\[绿\] golangci-lint' \
	&& printf '%s' "$out2" | grep -q '合计：红 0 / 绿 2'; then
	ok "C2 转绿后最新条目展示绿态（红态清零），退出码 0"
else
	ng "C2 绿态展示（rc=$rc2）"
	printf '%s\n' "$out2" | sed 's/^/    /'
fi

# ---------------------------------------------------------------------------
# 用例 3：空态（有库有表零 gate.check → 提示 + rc 0；S4-3）
# ---------------------------------------------------------------------------
R3="$(mktemp -d "${TMPDIR:-/tmp}/gs-r3.XXXXXX")"
make_repo "$R3"
init_db "$R3"
out3="$(cd "$R3" && bash "$SCRIPT_ABS" 2>&1)"
rc3=$?
if [ $rc3 -eq 0 ] && printf '%s' "$out3" | grep -q '暂无门禁记账'; then
	ok "C3 空库零记账 → 空态提示，退出码 0"
else
	ng "C3 空态（rc=$rc3）"
fi

# ---------------------------------------------------------------------------
# 用例 4：缺库 → 报错退出 1（S4-2）
# ---------------------------------------------------------------------------
R4="$(mktemp -d "${TMPDIR:-/tmp}/gs-r4.XXXXXX")"
make_repo "$R4"
err4="$(cd "$R4" && bash "$SCRIPT_ABS" 2>&1 >/dev/null)"
rc4=$?
if [ $rc4 -eq 1 ] && [ -n "$err4" ] && printf '%s' "$err4" | grep -q '不存在'; then
	ok "C4 账本库缺失 → stderr 说明 + 退出码 1（无误导性展示）"
else
	ng "C4 缺库（rc=$rc4）"
fi

# ---------------------------------------------------------------------------
# 用例 5：坏库（非 SQLite 字节）→ 报错退出 1（S4-2 损坏分支）
# ---------------------------------------------------------------------------
R5="$(mktemp -d "${TMPDIR:-/tmp}/gs-r5.XXXXXX")"
make_repo "$R5"
printf 'this is not a sqlite database' >"$R5/.pi/harness/events.db"
err5="$(cd "$R5" && bash "$SCRIPT_ABS" 2>&1 >/dev/null)"
rc5=$?
if [ $rc5 -eq 1 ] && [ -n "$err5" ]; then
	ok "C5 损坏库 → stderr 说明 + 退出码 1"
else
	ng "C5 坏库（rc=$rc5）"
fi

# ---------------------------------------------------------------------------
# 用例 6：只读断言——跑完库字节不变、零新文件（S4-3）
# ---------------------------------------------------------------------------
before_sum="$(md5sum "$R1/.pi/harness/events.db" | cut -d' ' -f1)"
before_n="$(find "$R1" -type f | wc -l)"
(cd "$R1" && bash "$SCRIPT_ABS" >/dev/null 2>&1)
after_sum="$(md5sum "$R1/.pi/harness/events.db" | cut -d' ' -f1)"
after_n="$(find "$R1" -type f | wc -l)"
if [ "$before_sum" = "$after_sum" ] && [ "$before_n" = "$after_n" ]; then
	ok "C6 只读：库字节 md5 不变、零新文件"
else
	ng "C6 只读断言（sum $before_sum → $after_sum，files $before_n → $after_n）"
fi

echo
if [ "$fail" -gt 0 ]; then
	echo "$fail 项失败"
	exit 1
fi
echo "GATE-STATUS SMOKE OK（$pass 项）"
