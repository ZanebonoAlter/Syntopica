#!/usr/bin/env bash
# doc-impact.smoke.sh — verify 双轨输入（coordinate-concurrent-changes）+ 误归因收敛（fix-doc-impact-misattribution）
#   + suggest 归属轨（attribute-concurrent-gate-noise，test-cases.md D 组，用例名 suggest-D*）
#   D1 归属轨：他人 handler 脏文件不触发「声明 none 但命中」
#   D2 回退轨：无 edit.map 时全树扫描命中仅提示不判 FAIL（降级语义，fix 后）
#   D3 空数组防御：paths=[] 视同无记录回退
#   D4 既有 excuse 注释读取兼容不判 FAIL
#   D5 规则5（路径不存在）在双轨下不变
#   D6 库不存在 → 回退全树（fail-open，命中仅提示）
#   N1 归档移动产物/openspec 元数据不触发任何域（黑名单）
#   N2 active change 元数据（configure-* 字样）不命中 configuration 域
#   N3 归属集合仅含共享文档不触发「声明 none 但命中」（契约锁定）
#   N4 共享文档在集合中不干扰真实命中（黑名单不误伤整集合）
#   N5 真实配置文件命中面不变（正则收紧不误伤）
#   suggest-D1 --change 归属轨：预勾选只吃本 change 集合（flow 命中、api 不勾）
#   suggest-D2 PI_SESSION_ID 查最新 mode.set boundChange（latest-wins）
#   suggest-D3 显式 --change 优先于会话绑定
#   suggest-D4a/b 三源解析不到 / 有名无 edit.map → 回退全树 + 显式标注
#   suggest-D5 三桶分列：本 change 参与预勾选，其他/无归属只列出不参与
#   suggest-D6 sqlite3 缺失 → 回退全树不报错不非零
#   suggest-D7 桶超 20 条：20 行 + 「另有 N 个」
#   suggest-D8 特殊字符 change 名 SQL 转义安全
# 用法：bash scripts/harness/doc-impact.smoke.sh
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TARGET="$HERE/doc-impact.sh"

pass=0; fail=0
ok()   { echo "✅ $1"; pass=$((pass+1)); }
bad()  { echo "❌ $1"; fail=$((fail+1)); }
expect() { # expect <名> <期望exit> <实际exit> [附加断言]
	local n="$1" w="$2" g="$3"
	if [ "$w" = "$g" ] && { [ $# -lt 4 ] || eval "$4"; }; then ok "$n"; else bad "$n（期望 exit=$w 实际 $g）"; fi
}

# fixture：临时 git 仓库 + change 目录 + tasks.md + 事件库
# $1=dir $2=doc-impact 声明行 $3=额外 tasks 行（可空）
new_fixture() {
	local d="$1" decl="$2" extra="${3:-}"
	(
		cd "$d" && git init -q && git config user.email t@t.t && git config user.name t &&
			printf '.pi/harness/\n' > .gitignore &&
			echo base > base.txt && git add -A && git commit -qm init
	) >/dev/null 2>&1
	mkdir -p "$d/openspec/changes/foo" "$d/scripts/harness" "$d/.pi/harness" "$d/backend-go/internal/reader/handler"
	# 复制被测脚本进 fixture：doc-impact.sh 开头 cd 到自身 REPO_ROOT，
	# 必须让 REPO_ROOT=fixture 根（git/events.db/tasks.md 才是 fixture 的）
	cp "$TARGET" "$d/scripts/harness/doc-impact.sh"
	# 他人 change 的 handler 脏文件（api 域启发式必命中）
	echo x > "$d/backend-go/internal/reader/handler/foreign.go"
	# tasks.md：声明行 + 尾三节骨架
	{
		echo "## 1. 任务"
		echo "- [ ] 1.1 干活"
		echo
		echo "## 2. 文档"
		echo "$decl"
		echo
		echo "## 3. 验证"
		[ -n "$extra" ] && echo "$extra"
	} > "$d/openspec/changes/foo/tasks.md"
	# 事件库（直建表，verify 只读不校验魔数）
	sqlite3 "$d/.pi/harness/events.db" \
		"CREATE TABLE events(id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, session_id TEXT NOT NULL, kind TEXT NOT NULL, change TEXT, payload TEXT NOT NULL);" 2>/dev/null
}
seed_edit_map() { # $1=dir $2=change（单引号经 '' 转义） $3=payload
	sqlite3 "$1/.pi/harness/events.db" \
		"INSERT INTO events(ts,session_id,kind,change,payload) VALUES('2026-09-04T00:00:00Z','s1','edit.map','${2//\'/\'\'}','$3');" 2>/dev/null
}
seed_mode_set() { # $1=dir $2=session $3=boundChange（payload 形如 quality-gate 落库侧）
	sqlite3 "$1/.pi/harness/events.db" \
		"INSERT INTO events(ts,session_id,kind,change,payload) VALUES('2026-09-04T00:00:00Z','$2','mode.set','$3','{\"mode\":\"implementation\",\"boundChange\":\"$3\",\"source\":\"skill\"}');" 2>/dev/null
}
verify() { ( cd "$1" && bash "$1/scripts/harness/doc-impact.sh" verify openspec/changes/foo ); } # 调用方捕获 exit

# suggest 用例执行器：$1=dir $2=PI_SESSION_ID（空=env -u 清除，防外层 pi 注入的
# 真实 session 污染 fixture 判定）其余=suggest 参数；输出+stderr → $1/suggest.out
suggest_run() {
	local d="$1" sid="$2"
	shift 2
	if [ -n "$sid" ]; then
		( cd "$d" && PI_SESSION_ID="$sid" bash "$d/scripts/harness/doc-impact.sh" suggest "$@" ) >"$d/suggest.out" 2>&1
	else
		( cd "$d" && env -u PI_SESSION_ID bash "$d/scripts/harness/doc-impact.sh" suggest "$@" ) >"$d/suggest.out" 2>&1
	fi
}

NONE_DECL='<!-- doc-impact: none(纯内部重构) -->'

# ---- D1 归属轨：他人 handler 脏文件不触发「声明 none 但命中」→ 通过 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
seed_edit_map "$D" foo '{"paths":["scripts/a.sh","docs/research/n.md"],"n":2}'
out="$(verify "$D")"; ec=$?
expect "D1 归属轨免疫他人 handler 脏文件（exit 0）" 0 "$ec" \
	"printf '%s' '$out' | grep -q '启发式:ownership'"
rm -rf "$D"

# ---- D2 回退轨：无 edit.map 全树命中降级为提示（不判 FAIL）→ 通过 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
out="$(verify "$D" 2>&1)"; ec=$?
expect "D2 回退轨降级为提示（命中 api 仅提示 exit 0）" 0 "$ec" \
	"printf '%s' '$out' | grep -q '无法归属.*api'"
rm -rf "$D"

# ---- D3 空数组防御：paths=[] 视同无记录回退 → 同 D2 降级 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
seed_edit_map "$D" foo '{"paths":[],"n":0}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "D3 空数组回退全树降级（exit 0 同 D2）" 0 "$ec"
rm -rf "$D"

# ---- D4 既有 excuse 注释：读取兼容不报错（回退轨 + excuse=api 豁免 → 通过） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL" '<!-- doc-impact-excuse: api=他人 change 脏文件; -->'
out="$(verify "$D" 2>&1)"; ec=$?
expect "D4 既有 excuse 兼容（豁免生效 exit 0）" 0 "$ec"
rm -rf "$D"

# ---- D5 规则5（声明文档不存在）在归属轨下不变 → FAIL ----
D=$(mktemp -d)
new_fixture "$D" '<!-- doc-impact: none(理由) -->'
# checkbox 指向不存在的文档（文档节内 checkbox 行的 docs/ 路径参与对账）
sed -i 's|## 3. 验证|- [ ] 2.1 更新 docs/never-exists.md 相关内容\n## 3. 验证|' "$D/openspec/changes/foo/tasks.md"
seed_edit_map "$D" foo '{"paths":["scripts/a.sh"],"n":1}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "D5 规则5 声明文档不存在（归属轨下仍 FAIL）" 1 "$ec" \
	"printf '%s' '$out' | grep -q '声明的文档不存在'"
rm -rf "$D"

# ---- D6 库不存在 → fail-open 回退全树降级 → 同 D2 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
rm -f "$D/.pi/harness/events.db"
out="$(verify "$D" 2>&1)"; ec=$?
expect "D6 库不存在回退全树降级（exit 0）" 0 "$ec" \
	"printf '%s' '$out' | grep -q '无法归属.*api'"
rm -rf "$D"

# ---- 归属轨下真实命中仍 FAIL（防白名单化：自己改了 handler 没声明 api） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/handler/mine.go"],"n":1}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "归属轨自身 handler 命中仍 FAIL（不白名单化）" 1 "$ec" \
	"printf '%s' '$out' | grep -q '声明 none 但启发式命中 api'"
rm -rf "$D"

# ---- N1 归档移动产物不触发任何域（黑名单，回退轨全树输入含 configure 字样的 archive 路径） ----
D=$(mktemp -d); new_fixture "$D" '<!-- doc-impact: api -->'
mkdir -p "$D/openspec/changes/archive/2026-09-01-configure-legacy"
echo x > "$D/openspec/changes/archive/2026-09-01-configure-legacy/.openspec.yaml"
echo x > "$D/openspec/changes/archive/2026-09-01-configure-legacy/tasks.md"
rm -f "$D/backend-go/internal/reader/handler/foreign.go" # 移除 api 干扰，只验证黑名单
out="$(verify "$D" 2>&1)"; ec=$?
expect "N1 归档移动产物（含 configure 字样）不触发 configuration" 0 "$ec" \
	"! printf '%s' '$out' | grep -q '无法归属.*configuration'"
rm -rf "$D"

# ---- N2 active change 元数据（configure-* 字样）不命中 configuration 域（归属轨） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
rm -f "$D/backend-go/internal/reader/handler/foreign.go"
seed_edit_map "$D" foo '{"paths":["openspec/changes/configure-x/.openspec.yaml"],"n":1}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "N2 configure-*.openspec.yaml 不命中 configuration（exit 0）" 0 "$ec"
rm -rf "$D"

# ---- N3 归属集合仅含共享文档不触发「声明 none 但命中」（契约锁定） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
rm -f "$D/backend-go/internal/reader/handler/foreign.go"
seed_edit_map "$D" foo '{"paths":["AGENTS.md","front/AGENTS.md","docs/reference/constraints-index.md"],"n":3}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "N3 共享文档在归属集合不触发命中（exit 0）" 0 "$ec"
rm -rf "$D"

# ---- N4 共享文档在集合中不干扰真实命中（黑名单不误伤整集合） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
rm -f "$D/backend-go/internal/reader/handler/foreign.go"
seed_edit_map "$D" foo '{"paths":["AGENTS.md","backend-go/internal/reader/handler/mine.go"],"n":2}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "N4 共享文档不干扰 handler 命中（仍 FAIL 报 api）" 1 "$ec" \
	"printf '%s' '$out' | grep -q '声明 none 但启发式命中 api'"
rm -rf "$D"

# ---- N5 真实配置文件命中面不变（正则收紧不误伤） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
rm -f "$D/backend-go/internal/reader/handler/foreign.go"
seed_edit_map "$D" foo '{"paths":["backend-go/config.yaml"],"n":1}'
out="$(verify "$D" 2>&1)"; ec=$?
expect "N5 backend-go/config.yaml 仍命中 configuration（exit 1）" 1 "$ec" \
	"printf '%s' '$out' | grep -q '声明 none 但启发式命中 configuration'"
rm -rf "$D"

# ===========================================================================
# suggest 归属轨（attribute-concurrent-gate-noise，test-cases.md D 组）
#   预勾选口径：change 名三源解析 → 归属轨（预勾选只吃本 change 归属集合）/ 回
#   退轨（全树 + 显式标注）；脏文件三桶分列不静默过滤；退出码恒 0、只读。
# ===========================================================================

# ---- suggest-D1 --change 归属轨：预勾选只由本 change 集合推导（他人 handler 脏文件不勾 api） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service"
echo x > "$D/backend-go/internal/reader/service/a.go"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/service/a.go"],"n":1}'
suggest_run "$D" "" --change foo; ec=$?
expect "suggest-D1 归属轨预勾选只吃本 change 集合（flow 命中+api 不勾）" 0 "$ec" \
	"grep -q '归属轨（change=foo）' '$D/suggest.out' && grep -qF '[x] flow' '$D/suggest.out' && ! grep -qF '[x] api' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D2 PI_SESSION_ID → 最新 mode.set boundChange（后写 foo 覆盖先写 bar） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service"
echo x > "$D/backend-go/internal/reader/service/a.go"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/service/a.go"],"n":1}'
seed_edit_map "$D" bar '{"paths":["backend-go/internal/reader/handler/bar.go"],"n":1}'
seed_mode_set "$D" se0 bar
seed_mode_set "$D" se0 foo
suggest_run "$D" se0; ec=$?
expect "suggest-D2 会话绑定取最新 mode.set（foo 胜出非 bar）" 0 "$ec" \
	"grep -q '归属轨（change=foo）' '$D/suggest.out' && grep -qF '[x] flow' '$D/suggest.out' && ! grep -qF '[x] api' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D3 显式 --change 优先于会话绑定（boundChange=foo 但查 bar） ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/service/a.go"],"n":1}'
seed_edit_map "$D" bar '{"paths":["backend-go/internal/reader/handler/bar.go"],"n":1}'
seed_mode_set "$D" se0 foo
suggest_run "$D" se0 --change bar; ec=$?
expect "suggest-D3 显式 --change 优先（以 bar 为查询目标）" 0 "$ec" \
	"grep -q '归属轨（change=bar）' '$D/suggest.out' && grep -qF '[x] api' '$D/suggest.out' && ! grep -qF '[x] flow' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D4a 三源解析不到（无 --change、无 PI_SESSION_ID）→ 回退全树 + 显式标注 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
suggest_run "$D" ""; ec=$?
expect "suggest-D4a 三源解析不到回退全树并标注（全树预勾选生效）" 0 "$ec" \
	"grep -q '回退轨：全树 diff，可能含其他会话/其他 change 的改动' '$D/suggest.out' && grep -qF '[x] api' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D4b change 名解析到（会话绑定 foo）但 foo 无 edit.map → 回退全树 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
seed_mode_set "$D" se0 foo
suggest_run "$D" se0; ec=$?
expect "suggest-D4b 有 change 名无 edit.map 回退全树并标注" 0 "$ec" \
	"grep -q '回退轨：全树 diff，可能含其他会话/其他 change 的改动' '$D/suggest.out' && ! grep -q '归属轨' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D5 三桶分列：本 change 参与预勾选，其他 change/无归属只列出不参与 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service" "$D/front/app"
echo x > "$D/backend-go/internal/reader/service/a.go"
echo x > "$D/front/app/x.vue"
echo x > "$D/misc-orphan.txt"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/service/a.go"],"n":1}'
seed_edit_map "$D" bar '{"paths":["front/app/x.vue"],"n":1}'
suggest_run "$D" "" --change foo; ec=$?
expect "suggest-D5 三桶分列（本change=预勾选；其他/无归属=列出不参与）" 0 "$ec" \
	"grep -q '归属轨（change=foo）' '$D/suggest.out' \
	&& grep -q '本 change 归属' '$D/suggest.out' && grep -q 'backend-go/internal/reader/service/a.go' '$D/suggest.out' \
	&& grep -q '归属其他 active change' '$D/suggest.out' && grep -q 'front/app/x.vue' '$D/suggest.out' \
	&& grep -q '无归属' '$D/suggest.out' && grep -q 'misc-orphan.txt' '$D/suggest.out' \
	&& grep -qF '[x] flow' '$D/suggest.out' && ! grep -qF '[x] api' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D6 sqlite3 缺失 → 回退全树 + 标注；无报错堆栈、退出码 0 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service"
echo x > "$D/backend-go/internal/reader/service/a.go"
seed_edit_map "$D" foo '{"paths":["backend-go/internal/reader/service/a.go"],"n":1}'
NBIND="$(mktemp -d)" # 构造无 sqlite3 的受限 PATH（保留脚本用到的外部命令，含首行 dirname）
for t in git grep head sed sort awk xargs dirname; do
	p="$(command -v "$t" 2>/dev/null)" && ln -sfn "$p" "$NBIND/$t"
done
BASH_ABS="$(command -v bash)"
( cd "$D" && env -u PI_SESSION_ID PATH="$NBIND" "$BASH_ABS" "$D/scripts/harness/doc-impact.sh" suggest --change foo ) >"$D/suggest.out" 2>&1
ec=$?
expect "suggest-D6 sqlite3 缺失回退全树（不报错不非零）" 0 "$ec" \
	"grep -q '回退轨：全树 diff' '$D/suggest.out' && grep -qF '[x] api' '$D/suggest.out' && ! grep -qi 'sqlite' '$D/suggest.out' && ! grep -q '归属轨' '$D/suggest.out'"
rm -rf "$D" "$NBIND"

# ---- suggest-D7 任一桶超 20 条 → 显示 20 行 + 「另有 N 个」 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service"
paths="" i=1
while [ "$i" -le 25 ]; do
	f="backend-go/internal/reader/service/gen$i.go"
	echo x > "$D/$f"
	[ -n "$paths" ] && paths="$paths,"
	paths="$paths\"$f\""
	i=$((i + 1))
done
seed_edit_map "$D" foo "{\"paths\":[$paths],\"n\":25}"
suggest_run "$D" "" --change foo; ec=$?
expect "suggest-D7 桶超 20 条截断（20 行 + 另有 5 个）" 0 "$ec" \
	"grep -q '另有 5 个' '$D/suggest.out' && ! grep -q 'gen5\.go' '$D/suggest.out' && grep -qF '[x] flow' '$D/suggest.out'"
rm -rf "$D"

# ---- suggest-D8 特殊字符 change 名（单引号）SQL 转义安全，不产生语法错误 ----
D=$(mktemp -d); new_fixture "$D" "$NONE_DECL"
mkdir -p "$D/backend-go/internal/reader/service"
echo x > "$D/backend-go/internal/reader/service/ob.go"
seed_edit_map "$D" "o'brien" '{"paths":["backend-go/internal/reader/service/ob.go"],"n":1}'
suggest_run "$D" "" --change "o'brien"; ec=$?
expect "suggest-D8 单引号 change 名转义安全（归属轨正常输出）" 0 "$ec" \
	"grep -qF \"归属轨（change=o'brien）\" '$D/suggest.out' && grep -qF '[x] flow' '$D/suggest.out'"
rm -rf "$D"

echo
if [ "$fail" -gt 0 ]; then
	echo "$fail 项失败（$pass 项通过）"
	exit 1
fi
echo "SMOKE OK（$pass 项通过）"
