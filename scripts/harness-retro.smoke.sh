#!/usr/bin/env bash
# harness-retro.smoke.sh — harness-retro.sh 的 fixture 冒烟自测（change: harness-retro-loop 5.1）
#
# 在临时目录拼装同构账本（application_id 魔数 + events 表 + 三条索引），旁置真实被测脚本
# 逐 case 断言退出码与关键输出。账本一律用 --db 指向临时库，绝不触碰真实 .pi/harness/events.db。
#
# case 清单（与 harness-retro-loop spec 的 Scenario 对应）：
#   A1 六段齐备且各带指标 / A2 分母加权还原 / A3 基线生成与差值 / A4 change 归属限定 / A5 失败特征归并
#   B1 fail-open 单列不计入 agent 失败 / B2 block 与 bypass 不入故障段 / B3 超阈值提醒入段 / B4 低于阈值不产生噪声
#   C1 只读打开零写入 / C2 超期窗口提示 / C3 常规窗口无提示（含 30 天边界）/ C4 库不存在 fail-loud
#   C5 拒绝非本应用库 / C6 空库不产出伪指标 / C7 有失败仍 exit 0 / C8 运行后账本零新增
#   C9 不触碰注入配置 / C10 基线损坏降级
#   D1 采样缺 n 回退缺省权重 / D2 非法 n 保守计 1 且告警 / D3 不可解析行跳过且告警
#   D4 重复失败热点入段（累计连续重复 ≥ 阈值）/ D5 --json 可解析 / D6 参数非法拒绝 / D7 --change 注入安全
#
# 用法：bash scripts/harness-retro.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RETRO="$SCRIPT_DIR/harness-retro.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0

out=""
rc=0

run_retro() { # $@ = 传给被测脚本的参数
	out="$("$RETRO" "$@" 2>&1)"
	rc=$?
}

check() { # $1=期望退出码 $2=case 名，其后：应包含关键字（可多个）
	local want_rc="$1" name="$2"
	shift 2
	if [ "$rc" != "$want_rc" ]; then
		echo "  ✗ $name（期望 rc=$want_rc 实际 rc=$rc）"
		echo "    输出: $(printf '%s' "$out" | head -6)"
		failn=$((failn + 1))
		return
	fi
	local kw
	for kw in "$@"; do
		if ! printf '%s' "$out" | grep -qF -- "$kw"; then
			echo "  ✗ $name（rc 正确，但输出未包含: $kw）"
			echo "    输出: $(printf '%s' "$out" | head -20)"
			failn=$((failn + 1))
			return
		fi
	done
	echo "  ✓ $name"
	pass=$((pass + 1))
}

check_absent() { # $1=case 名，其后：不应出现的关键字（可多个）
	local name="$1"
	shift
	local kw
	for kw in "$@"; do
		if printf '%s' "$out" | grep -qF -- "$kw"; then
			echo "  ✗ $name（输出不应包含: $kw）"
			echo "    输出: $(printf '%s' "$out" | head -20)"
			failn=$((failn + 1))
			return
		fi
	done
	echo "  ✓ $name"
	pass=$((pass + 1))
}

# ---------- fixture 构造 ----------

esc() { printf '%s' "$1" | sed "s/'/''/g"; }

mk_db() { # $1 = 库路径；$2 = application_id（缺省魔数）
	local db="$1" appid="${2:-1398361684}"
	sqlite3 "$db" "
PRAGMA application_id = $appid;
CREATE TABLE events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,
  session_id TEXT NOT NULL,
  kind       TEXT NOT NULL,
  change     TEXT,
  payload    TEXT NOT NULL
);
CREATE INDEX idx_ev_session ON events(session_id, id);
CREATE INDEX idx_ev_change  ON events(change, id);
CREATE INDEX idx_ev_kind_ts ON events(kind, ts);
"
}

ins() { # $1=db $2=kind $3=change $4=payload [5=session 6=ts]
	local db="$1" kind="$2" chg="$3" pay="$4" sess="${5:-s1}" ts="${6:-}"
	local tsql
	if [ -n "$ts" ]; then
		tsql="'$(esc "$ts")'"
	else
		tsql="strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	fi
	sqlite3 "$db" "INSERT INTO events(ts, session_id, kind, change, payload)
		VALUES ($tsql, '$(esc "$sess")', '$(esc "$kind")', '$(esc "$chg")', '$(esc "$pay")');"
}

count_rows() { sqlite3 "$1" "SELECT COUNT(*) FROM events;"; }

hash_of() { sha256sum "$1" | awk '{print $1}'; }

echo "== harness-retro.smoke =="

# ---------- fixture A：分母还原（spec Scenario 精确数字） ----------
FA="$TMP/a.db"
mk_db "$FA"
for i in 1 2 3 4 5 6; do
	ins "$FA" gate.check cA "{\"cmd\":\"go test -short ./internal/foo/...\",\"phase\":\"turn_end\",\"ok\":false,\"ms\":10,\"diag\":\"FAIL syntopica-backend/internal/foo [build failed]\"}"
done
ins "$FA" gate.check cA '{"cmd":"go test -short ./internal/foo/...","phase":"turn_end","ok":true,"flip":true}'
ins "$FA" gate.check cA '{"cmd":"go test -short ./internal/foo/...","phase":"turn_end","ok":true,"sampled":true,"n":5}'
ins "$FA" gate.check cA '{"cmd":"go test -short ./internal/foo/...","phase":"turn_end","ok":true,"sampled":true,"n":5}'

# ---------- fixture B：综合（分段 / 故障分离 / 软提醒 / 归属 / 注入面） ----------
FB="$TMP/b.db"
mk_db "$FB"
ins "$FB" gate.check probe-A '{"cmd":"golangci-lint","phase":"turn_end","ok":false,"ms":900,"diag":"internal/x.go:3:1: commentFormatting: put a space between `//` and comment text"}'
ins "$FB" gate.check probe-A '{"cmd":"go vet","phase":"turn_end","ok":false,"ms":120,"diag":"internal/admin/service/a.go:323:33: missing '','' in composite literal"}'
ins "$FB" gate.check probe-A '{"cmd":"go vet","phase":"turn_end","ok":true,"flip":true}'
ins "$FB" gate.check probe-A '{"cmd":"go vet","phase":"turn_end","ok":false,"ms":130,"diag":"# syntopica-backend/internal/admin/service"}'
ins "$FB" gate.check probe-B '{"cmd":"go test -short ./internal/admin/...","phase":"turn_end","ok":false,"ms":300,"diag":"FAIL syntopica-backend/internal/admin/service [setup failed]"}'
for i in 1 2 3; do
	ins "$FB" policy.decision probe-A '{"policy":"quality-gate","action":"fail-open","reasonCode":"interop-down","durationMs":6}'
done
ins "$FB" policy.decision probe-A '{"policy":"spec-gate","action":"block","reasonCode":"archive-check-failed","target":"archive"}'
ins "$FB" policy.decision probe-A '{"policy":"spec-gate","action":"bypass","reasonCode":"explicit-bypass","target":"archive"}'
for i in 1 2 3 4 5; do
	ins "$FB" policy.decision probe-A '{"policy":"test-scope-guard","action":"warn","reasonCode":"full-go-test"}'
done
for i in 1 2 3 4; do
	ins "$FB" policy.decision probe-A '{"policy":"ui-design-gate","action":"warn","reasonCode":"ui-impact-mismatch"}'
done
ins "$FB" constraint.inject probe-A '{"path":"docs/reference/flow/daily-report.md","mode":"section","reason":"keyword","bytes":1000}'
ins "$FB" constraint.inject probe-A '{"path":"docs/reference/flow/daily-report.md","mode":"section","reason":"keyword","bytes":1000}'

# ---------- fixture C：口径异常（缺 n / 非法 n / 不可解析行 / 缺 ok 字段） ----------
FC="$TMP/c.db"
mk_db "$FC"
ins "$FC" gate.check cC '{"cmd":"c1","phase":"turn_end","ok":true,"sampled":true}'
ins "$FC" gate.check cC '{"cmd":"c1","phase":"turn_end","ok":true,"sampled":true,"n":0}'
ins "$FC" gate.check cC '{not json at all'
ins "$FC" gate.check cC '{"cmd":"c1","phase":"turn_end"}'
ins "$FC" gate.check cC '{"cmd":"c1","phase":"turn_end","ok":false,"diag":"boom"}'

# ---------- fixture D：空账本 ----------
FD="$TMP/d.db"
mk_db "$FD"

# ---------- fixture E：非本应用库 ----------
FE="$TMP/e.db"
mk_db "$FE" 0

# ---------- fixture F：重复失败热点（归一化 diag 连续重复 6 次） ----------
FF="$TMP/f.db"
mk_db "$FF"
for i in 1 2 3 4 5 6; do
	ins "$FF" gate.check cF "{\"cmd\":\"golangci-lint\",\"phase\":\"turn_end\",\"ok\":false,\"diag\":\"internal/y.go:$i:1: File is not properly formatted (gofmt)\"}" sF
done

# ================= A. 主链路 =================

run_retro --db "$FB" --days 7
check 0 "A1 六段齐备且各带指标" "① 门禁失败聚类" "② 回归翻转" "③ harness 自身故障" "④ 软提醒失效" "⑤ 重复失败热点" "⑥ 注入面健康" "失败条数            : 4" "注入条数 / 字节" "docs/reference/flow/daily-report.md"

run_retro --db "$FA" --days 7
check 0 "A2 分母按锚点与采样权重还原（6 失败 + 1 锚点 + 2×n=5 → 17）" "失败条数            : 6" "还原执行次数（分母）: 17" "失败率              : 35%" "口径                : 失败条计 1"

run_retro --db "$FB" --days 7
check 0 "A5 失败特征归并 + 按 domain 包聚类" "按失败特征归并" "lint 规则" "编译/类型/vet 语法" "测试环境未就绪（setup failed）" "按 domain 包（Top）" "admin"

# ---------- A3 基线生成与差值 ----------
BASE="$TMP/base.json"
run_retro --db "$FA" --days 7 --save-baseline "$BASE"
check 0 "A3a 落基线快照" "基线已写入 $BASE"
if [ -f "$BASE" ] && python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$BASE" 2>/dev/null; then
	echo "  ✓ A3b 基线文件可解析为 JSON"
	pass=$((pass + 1))
else
	echo "  ✗ A3b 基线文件不存在或不可解析"
	failn=$((failn + 1))
fi
base_sha="$(hash_of "$BASE")"
run_retro --db "$FA" --days 7 --baseline "$BASE"
check 0 "A3c 与基线比对（无变化差值为 0）" "⑦ 较基线变化" "gate.failures            +0"
if [ "$(hash_of "$BASE")" = "$base_sha" ]; then
	echo "  ✓ A3d 比对不改写基线文件"
	pass=$((pass + 1))
else
	echo "  ✗ A3d 比对改写了基线文件"
	failn=$((failn + 1))
fi

# ---------- A4 change 归属限定 ----------
run_retro --db "$FB" --days 7 --change probe-A
check 0 "A4a --change 限定（probe-A 失败 3 条）" "归属      : change = probe-A" "失败条数            : 3"
check_absent "A4b --change 限定不纳入他 change 的失败族" "测试环境未就绪（setup failed）"

# ================= B. 故障分离与软提醒 =================

run_retro --db "$FB" --days 7
check 0 "B1 fail-open 单列且不计入 agent 失败" "③ harness 自身故障" "故障条数            : 3" "quality-gate / interop-down : 3"
check_absent "B2 block 与 bypass 不入故障段" "archive-check-failed" "explicit-bypass"
check 0 "B3 超阈值 warn 入段（=5 命中）" "test-scope-guard / full-go-test : 5 次"
check_absent "B4 低于阈值 warn 不产生噪声" "ui-impact-mismatch"

# ================= C. 库异常、窗口与只读 =================

sha_before="$(hash_of "$FB")"
rows_before="$(count_rows "$FB")"
git_before="$(cd "$REPO_ROOT" && git status --porcelain .pi/ 2>/dev/null | sort)"

run_retro --db "$FB" --days 7
check 0 "C7 有失败但退出码为 0" "失败条数            : 4"

run_retro --db "$FB" --days 45
check 0 "C2 超期窗口给出提示（45 > 30）" "TTL 清扫"
run_retro --db "$FB" --days 7
check_absent "C3a 常规窗口无提示" "TTL 清扫"
run_retro --db "$FB" --days 30
check_absent "C3b 窗口等于保留期（30）不提示" "TTL 清扫"
run_retro --db "$FB" --days 31
check 0 "C3c 窗口 31 天（> 保留期）提示" "TTL 清扫"

run_retro --db "$TMP/nonexistent.db" --days 7
check 1 "C4 库不存在时 fail-loud" "账本不存在"
check_absent "C4b 缺库不输出伪造报告" "① 门禁失败聚类"

run_retro --db "$FE" --days 7
check 1 "C5 拒绝非本应用库" "拒绝读取"

run_retro --db "$FD" --days 7
check 0 "C6 空库不产出伪指标" "窗口内无数据"
check_absent "C6b 空库不出现失败率" "失败率"

run_retro --db "$FB" --days 7 --save-baseline "$TMP/b2.json"
if [ "$(hash_of "$FB")" = "$sha_before" ] && [ "$(count_rows "$FB")" = "$rows_before" ]; then
	echo "  ✓ C1/C8 只读打开零写入（库哈希与行数不变）"
	pass=$((pass + 1))
else
	echo "  ✗ C1/C8 账本被改写（sha 或行数变化）"
	failn=$((failn + 1))
fi
git_after="$(cd "$REPO_ROOT" && git status --porcelain .pi/ 2>/dev/null | sort)"
if [ "$git_before" = "$git_after" ]; then
	echo "  ✓ C9 未触碰注入配置与扩展（.pi/ 工作树无变化）"
	pass=$((pass + 1))
else
	echo "  ✗ C9 .pi/ 工作树发生变化"
	echo "    before: $git_before"
	echo "    after : $git_after"
	failn=$((failn + 1))
fi

# ---------- C10 基线损坏降级 ----------
printf 'not json at all' >"$TMP/broken.json"
run_retro --db "$FA" --days 7 --baseline "$TMP/broken.json"
check 0 "C10 基线损坏降级（仍出报告 + 提示）" "基线不可用、已跳过比对" "① 门禁失败聚类"

# ================= D. 口径分支、热点、参数与注入安全 =================

run_retro --db "$FC" --days 7
check 0 "D1/D2/D3 缺 n 回退 5、非法 n 计 1、不可解析行跳过" \
	"失败条数            : 1" "还原执行次数（分母）: 7" \
	"已保守按 1 计入执行次数" "不可解析或缺判定字段"

run_retro --db "$FF" --days 7
check 0 "D4 重复失败热点入段（归一化后连续 6 次）" "⑤ 重复失败热点" "×6"

run_retro --db "$FB" --days 7 --json
if printf '%s' "$out" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["metrics"]["gate.failures"]==4, d["metrics"]; assert "gate" in d and "inject" in d' 2>/dev/null; then
	echo "  ✓ D5 --json 可解析且指标齐全"
	pass=$((pass + 1))
else
	echo "  ✗ D5 --json 输出不可解析或缺指标"
	echo "    输出: $(printf '%s' "$out" | head -5)"
	failn=$((failn + 1))
fi

run_retro --db "$FB" --days abc
check 1 "D6a --days 非数字拒绝" "--days 参数非法"
run_retro --db "$FB" --days 0
check 1 "D6b --days 0 拒绝" "--days 参数非法"
run_retro --db "$FB" --warn-threshold 0
check 1 "D6c --warn-threshold 0 拒绝（不退化为全部命中）" "--warn-threshold 参数非法"
run_retro --db "$FB" --nope
check 1 "D6d 未知参数拒绝并打印用法" "未知参数"

rows_before="$(count_rows "$FB")"
run_retro --db "$FB" --change "probe-A' OR '1'='1" --days 7
check 0 "D7a --change 含 SQL 元字符仍正常出报告（按字面处理，不匹配任何行）" "窗口内无数据"
if [ "$(count_rows "$FB")" = "$rows_before" ]; then
	echo "  ✓ D7b SQL 注入无效（行数不变）"
	pass=$((pass + 1))
else
	echo "  ✗ D7b 注入生效（行数变化）"
	failn=$((failn + 1))
fi

# ================= 汇总 =================
echo
echo "== 通过 $pass / 失败 $failn =="
[ "$failn" -eq 0 ] || exit 1
