#!/usr/bin/env bash
# concurrency-status.smoke.sh — concurrency-status.sh 判定矩阵与只读安全（coordinate-concurrent-changes）
# 用法：bash scripts/concurrency-status.smoke.sh
# fixture：临时 git 仓库 + 直建 events 表（脚本只读不校验魔数，绕过 logEvent 属预期）
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TARGET="$HERE/concurrency-status.sh"

pass=0; fail=0
check() { # check <名> <期望exit> <实际exit> [额外断言命令]
	local name="$1" want="$2" got="$3"
	if [ "$want" = "$want" ] 2>/dev/null && [ "$want" != "$3" ] 2>/dev/null; then :; fi
	if [ "$want" = "$got" ]; then
		if [ $# -ge 4 ] && ! eval "$4"; then
			echo "❌ $name（exit=$got 对，但附加断言失败）"; fail=$((fail+1)); return
		fi
		echo "✅ $name"; pass=$((pass+1))
	else
		echo "❌ $name（期望 exit=$want 实际 $got）"; fail=$((fail+1))
	fi
}

# fixture：临时 git 仓库 + 事件库
new_fixture() {
	local d
	d="$(mktemp -d)"
	(
		cd "$d" && git init -q && git config user.email t@t.t && git config user.name t &&
			mkdir -p openspec/changes/foo openspec/changes/bar .pi/harness &&
			printf '.pi/harness/\n' > .gitignore &&
			echo base > base.txt && git add -A && git commit -qm init
	) >/dev/null 2>&1
	mkdir -p "$d/openspec/changes/archive/baz" 2>/dev/null || true
	sqlite3 "$d/.pi/harness/events.db" \
		"CREATE TABLE events(id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, session_id TEXT NOT NULL, kind TEXT NOT NULL, change TEXT, payload TEXT NOT NULL);" 2>/dev/null
	echo "$d"
}
seed() { # seed <dir> <ts> <session> <kind> <change> <payload>
	sqlite3 "$1/.pi/harness/events.db" \
		"INSERT INTO events(ts,session_id,kind,change,payload) VALUES('$2','$3','$4','$5','$6');" 2>/dev/null
}
now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }
J='application/json'

# ---- C1 工作树干净 → exit 0 ----
D="$(new_fixture)"
out="$( cd "$D" && bash "$TARGET" --check foo 2>/dev/null )"; ec=$?
check "C1 工作树干净 exit 0" 0 "$ec"
rm -rf "$D"

# ---- C2 脏文件全归属本 change → exit 0 ----
D="$(new_fixture)"
echo a > "$D/a.go"
seed "$D" "$(now_iso)" s1 edit.map foo '{"paths":["a.go"],"n":1}'
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1; ec=$?
check "C2 全归属本 change exit 0" 0 "$ec"
rm -rf "$D"

# ---- C3 存在归属其他 active change 的脏文件 → exit 2 + JSON ----
D="$(new_fixture)"
mkdir -p "$D/backend-go"
echo a > "$D/a.go"; echo b > "$D/backend-go/b.go"
seed "$D" "$(now_iso)" s1 edit.map foo '{"paths":["a.go"],"n":1}'
seed "$D" "$(now_iso)" s2 edit.map bar '{"paths":["backend-go/b.go"],"n":1}'
json="$( cd "$D" && bash "$TARGET" --check foo 2>/dev/null )"; ec=$?
check "C3 归属其他 active change exit 2" 2 "$ec" \
	"printf '%s' '$json' | grep -q 'backend-go/b.go' && printf '%s' '$json' | grep -q '\"bar\"'"
rm -rf "$D"

# ---- C4 脏文件全无归属 → exit 3（冷启动跳过）----
D="$(new_fixture)"
echo c > "$D/c.go"
seed "$D" "$(now_iso)" s1 edit.map foo '{"paths":["a.go"],"n":1}' # 库可用但 c.go 无归属
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1; ec=$?
check "C4 全无归属 exit 3（冷启动）" 3 "$ec"
rm -rf "$D"

# ---- C5 冲突文件（foo+bar 双归属）人读带冲突标记 ----
D="$(new_fixture)"
echo d > "$D/d.vue"
seed "$D" "$(now_iso)" s1 edit.map foo '{"paths":["d.vue"],"n":1}'
seed "$D" "$(now_iso)" s2 edit.map bar '{"paths":["d.vue"],"n":1}'
human="$( cd "$D" && bash "$TARGET" foo 2>/dev/null )"
if printf '%s' "$human" | grep -q '冲突 1 个'; then
	echo "✅ C5 冲突文件人读标记"; pass=$((pass+1))
else
	echo "❌ C5 冲突文件人读标记（输出未见「冲突 1 个」）"; fail=$((fail+1))
fi
# 视角 foo：d.vue owners=bar,foo ≠ foo → 计入 FOREIGN（--check exit 2）
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1; ec=$?
check "C5b 冲突文件 --check 按他属处理 exit 2" 2 "$ec"
rm -rf "$D"

# ---- C6 归属已 archive change 视同无归属 → exit 3 ----
D="$(new_fixture)"
echo e > "$D/e.go"
seed "$D" "$(now_iso)" s1 edit.map baz '{"paths":["e.go"],"n":1}' # baz 仅存在于 archive/
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1; ec=$?
check "C6 归属 archive change 视同无归属 exit 3" 3 "$ec"
rm -rf "$D"

# ---- C7 库不存在 fail-open → exit 3 ----
D="$(new_fixture)"
echo f > "$D/f.go"
rm -f "$D/.pi/harness/events.db"
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1; ec=$?
check "C7 库不存在 fail-open exit 3" 3 "$ec"
# 人读模式不炸，输出三段标题
human="$( cd "$D" && bash "$TARGET" foo 2>/dev/null )"; ec2=$?
if [ $ec2 -eq 0 ] && printf '%s' "$human" | grep -q '① 活跃 change' && printf '%s' "$human" | grep -q '③ 近 6h'; then
	echo "✅ C7b 人读模式库缺失降级不炸"; pass=$((pass+1))
else
	echo "❌ C7b 人读模式库缺失降级不炸"; fail=$((fail+1))
fi
rm -rf "$D"

# ---- C8 只读安全：脚本运行后库 mtime 不变 ----
D="$(new_fixture)"
echo a > "$D/a.go"
seed "$D" "$(now_iso)" s1 edit.map foo '{"paths":["a.go"],"n":1}'
seed "$D" "$(now_iso)" s1 gate.check foo '{"cmd":"golangci-lint","ok":true}'
mt_before="$(stat -c %Y "$D/.pi/harness/events.db")"
( cd "$D" && bash "$TARGET" foo ) >/dev/null 2>&1
( cd "$D" && bash "$TARGET" --check foo ) >/dev/null 2>&1
mt_after="$(stat -c %Y "$D/.pi/harness/events.db")"
if [ "$mt_before" = "$mt_after" ]; then
	echo "✅ C8 只读安全（库 mtime 不变）"; pass=$((pass+1))
else
	echo "❌ C8 只读安全（库被修改！）"; fail=$((fail+1))
fi
# gate.check 流水出现（近 6h seed）
human="$( cd "$D" && bash "$TARGET" 2>/dev/null )"
if printf '%s' "$human" | grep -q 'golangci-lint ok'; then
	echo "✅ C8b 近期验证流水段输出 gate.check"; pass=$((pass+1))
else
	echo "❌ C8b 近期验证流水段输出 gate.check"; fail=$((fail+1))
fi
rm -rf "$D"

# ---- 无参数 --check → usage exit 3 ----
D="$(new_fixture)"
( cd "$D" && bash "$TARGET" --check ) >/dev/null 2>&1; ec=$?
check "--check 缺参数 usage exit 3" 3 "$ec"
( cd "$D" && bash "$TARGET" --check no-such-change ) >/dev/null 2>&1; ec=$?
check "--check 不在活跃列表 exit 3" 3 "$ec"
rm -rf "$D"

echo
if [ "$fail" -gt 0 ]; then
	echo "$fail 项失败（$pass 项通过）"
	exit 1
fi
echo "SMOKE OK（$pass 项通过）"
