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
#   H1 巡检段数字正确 / H2 JSON effectiveness.patrol 键 / H3 无表无巡检事件降级 / H4 有台账零巡检事件降级
#   I1 截断域名修复后 join 命中（substr 差一回归，harness-retro-sql-fixes）/ I2 jit-path 通道计入命中率 / I3 未识别通道显式计数且非 flow-doc 合法通道不误标 / I4 命中率=fixture 复算值（2/3=67%）
#   J1 diag 含 command not found 归环境族（先于 %lint% 关键词族）/ J2 真实 lint 失败仍归 lint 族（两族并存各归各族）
#
# 用法：bash scripts/harness/harness-retro.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RETRO="$SCRIPT_DIR/harness-retro.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
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

# ---------- fixture G：⑦效能看板（rollup 终值/覆盖率/跨域命中/催修时距/波次） ----------
FG="$TMP/g.db"
mk_db "$FG"
# s-roll：两条中间快照 + 一条 final（终值只取最新：tok=5000，不累加 1000+2000+5000）
ins "$FG" session.rollup chg-eff '{"turns":2,"steps":5,"toolCalls":8,"tokens":{"input":50,"output":20,"cacheRead":900,"cacheWrite":0,"total":1000},"cost":0.1,"durationSec":60,"model":"glm-5.3","final":false}' s-roll
ins "$FG" session.rollup chg-eff '{"turns":4,"steps":10,"toolCalls":16,"tokens":{"input":100,"output":30,"cacheRead":1500,"cacheWrite":0,"total":2000},"cost":0.2,"durationSec":200,"model":"glm-5.3","final":false}' s-roll
ins "$FG" session.rollup chg-eff '{"turns":7,"steps":20,"toolCalls":30,"tokens":{"input":200,"output":80,"cacheRead":4500,"cacheWrite":0,"total":5000},"cost":0.5,"durationSec":600,"model":"glm-5.3","final":true}' s-roll
# s-n1/s-n2：无 rollup（覆盖率 1/3）；催修时距：同 (s-n1,lint) 红→绿相隔 40 分钟
ins "$FG" gate.check chg-eff '{"cmd":"lint","ok":false,"diag":"oops"}' s-n1 "$(date -u -d '-50 min' +%Y-%m-%dT%H:%M:%S.000Z)"
ins "$FG" gate.check chg-eff '{"cmd":"lint","ok":true,"flip":1}' s-n1 "$(date -u -d '-10 min' +%Y-%m-%dT%H:%M:%S.000Z)"
# 跨域注入：chg-x 注入 ai-summary 域（airouter 前缀，与编辑路径不相交，真跨域）→ 命中率 0。
# 注：旧版此处注入 semantic-board，但其 doc-impact-applies 本就含 topicgraph 前缀，与编辑相交；
# substr 差一 bug 修复后会命中 1/6，不再构成跨域 0 场景（harness-retro-sql-fixes 同步修正）。
ins "$FG" constraint.inject chg-x '{"path":"docs/reference/flow/ai-summary.md","mode":"section","reason":"declaration","bytes":800}' s-n1
ins "$FG" edit.map chg-x '{"paths":["backend-go/internal/topicgraph/model.go"],"n":1}' s-n1
# 波次：同 change 同文件在 3 个 session 的快照 → ≥3 session
ins "$FG" edit.map chg-w '{"paths":["backend-go/internal/topicgraph/model.go"],"n":1}' s-roll
ins "$FG" edit.map chg-w '{"paths":["backend-go/internal/topicgraph/model.go"],"n":1}' s-n1
ins "$FG" edit.map chg-w '{"paths":["backend-go/internal/topicgraph/model.go"],"n":1}' s-n2

# ---------- fixture H：测试欠账巡检（test-debt-patrol：patrol.check 流水 + test_debt 台账） ----------
FH="$TMP/h.db"
mk_db "$FH"
sqlite3 "$FH" "
CREATE TABLE test_debt (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  test_id       TEXT UNIQUE NOT NULL,
  domain        TEXT,
  first_seen    TEXT NOT NULL,
  last_seen     TEXT NOT NULL,
  context       TEXT,
  status        TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','fixed','waived')),
  fixed_by      TEXT,
  waived_reason TEXT,
  waived_at     TEXT,
  note          TEXT
);
INSERT INTO test_debt(test_id, domain, first_seen, last_seen, status) VALUES
  ('internal/foo::TestOld',   'foo', datetime('now','-10 day'), datetime('now','-1 day'),  'open'),
  ('internal/bar::TestNew',   'bar', datetime('now','-2 day'),  datetime('now','-2 day'),  'open'),
  ('internal/baz::TestFix',   'baz', datetime('now','-40 day'), datetime('now','-1 day'),  'fixed'),
  ('internal/qux::TestWaive', 'qux', datetime('now','-40 day'), datetime('now','-30 day'), 'waived');
"
ins "$FH" gate.check chg-p '{"cmd":"go test -short ./internal/foo/...","phase":"turn_end","ok":false,"ms":10,"diag":"FAIL internal/foo"}'
ins "$FH" patrol.check '' '{"shard":"be-foo","ok":false,"ms":1200,"fails":["internal/foo::TestA"]}' s-patrol
ins "$FH" patrol.check '' '{"shard":"fe-core","ok":true,"ms":900,"fails":[]}' s-patrol
ins "$FH" patrol.check '' '{"shard":"fe-core","ok":true,"ms":800,"fails":[]}' s-patrol

# ---------- fixture I：注入命中率口径回归（harness-retro-sql-fixes 1.1/1.2/1.3） ----------
# 域映射取自仓库真实 flow 文档头 doc-impact-applies：reading.md → backend-go/internal/reader/、
# ai-summary.md → backend-go/internal/platform/airouter/（仓内固定前缀，join 可精确复算）。
FI="$TMP/i.db"
mk_db "$FI"
# I1 截断域名 join 场景：旧 substr(-5-3) 会把 domain 截成 readin（与 dom_map 永不相等），修后命中
ins "$FI" constraint.inject chg-fix '{"path":"docs/reference/flow/reading.md","mode":"section","reason":"declaration","bytes":900}' s-i1
ins "$FI" edit.map chg-fix '{"paths":["backend-go/internal/reader/model.go"],"n":1}' s-i1
# I2 jit-path 通道注入：旧枚举（declaration/keyword/edit）不认，修后计入分子/分母
ins "$FI" constraint.inject chg-fix '{"path":"docs/reference/flow/ai-summary.md","mode":"section","reason":"jit-path","bytes":700}' s-i1
ins "$FI" edit.map chg-fix '{"paths":["backend-go/internal/platform/airouter/client.go"],"n":1}' s-i1
# I3 未识别通道：flow 文档注入用枚举外 reason（future-chan）→ 显式计数且不静默；同域编辑不贡献 hit
ins "$FI" constraint.inject chg-unk '{"path":"docs/reference/flow/reading.md","mode":"section","reason":"future-chan","bytes":500}' s-i2
ins "$FI" edit.map chg-unk '{"paths":["backend-go/internal/reader/model.go"],"n":1}' s-i2
# I3 反向：非 flow 文档的合法通道（mode-base）不进未识别计数
ins "$FI" constraint.inject chg-unk '{"path":"docs/reference/standard/backend/testing.md","mode":"section","reason":"mode-base","bytes":600}' s-i2

# ---------- fixture J：家族分类前置回归（harness-retro-sql-fixes 1.4） ----------
FJ="$TMP/j.db"
mk_db "$FJ"
# 环境风暴型：diag 含 command not found（旧分类被 %lint% 兑底模式误吸进 lint 规则族）
ins "$FJ" gate.check cJ '{"cmd":"golangci-lint run ./...","phase":"turn_end","ok":false,"ms":40,"diag":"golangci-lint: command not found"}'
# 真实 lint 失败：须仍归 lint 规则族
ins "$FJ" gate.check cJ '{"cmd":"golangci-lint run ./...","phase":"turn_end","ok":false,"ms":900,"diag":"internal/x.go:3:1: File is not properly formatted (gofmt)"}'

# ================= A. 主链路 =================

run_retro --db "$FB" --days 7
check 0 "A1 六段齐备且各带指标" "① 门禁失败聚类" "② 回归翻转" "③ harness 自身故障" "④ 软提醒失效" "⑤ 重复失败热点" "⑥ 注入面健康" "失败条数            : 4" "注入条数 / 字节" "docs/reference/flow/daily-report.md"

# ---------- G. ⑦效能看板 ----------
run_retro --db "$FG" --days 7
check 0 "G1 七段齐备：⑦段与覆盖率降级标注" "⑦ 效能看板" "rollup 覆盖率        : 1 / 3 session（33%）⚠ 数据积累中"
check 0 "G2 rollup 终值取最新快照不累加（tok=5000 非 8000）" "tokens 中位 5000 / P75 5000"
check 0 "G3 跨域注入命中率=0（注入 ai-summary、编辑落 topicgraph 域，两域不相交）" "注入命中率        : 编辑域命中注入域 0 / 6（0%；"
check 0 "G4 催修时距 40 分钟计入中位" "门禁催修时距      : 红→绿 中位 40.0 min（n=1）"
check 0 "G5 返工波次清单（3 session 文件）" "跨 session 编辑波次: 1 个文件 ≥3 session" "backend-go/internal/topicgraph/model.go" "×3 session"
check 0 "G6 pin 复用零值形态" "pin 复用          : write 0 条 / read 0 条（比值 —）"
"$RETRO" --db "$FG" --days 7 --json >"$TMP/fg.json" 2>/dev/null
if python3 - "$TMP/fg.json" <<'FGPY'
import json, sys
d = json.load(open(sys.argv[1]))
m = d['metrics']
assert m['m7.rollup_coverage_pct'] == 33, m['m7.rollup_coverage_pct']
assert m['m7.sess_tok_p50'] == 5000, m['m7.sess_tok_p50']
assert m['m7.domain_hit_pct'] == 0, m['m7.domain_hit_pct']
assert m['m7.repair_med_sec'] is not None and 2390 <= m['m7.repair_med_sec'] <= 2410, m['m7.repair_med_sec']
assert m['m7.wave_files'] == 1, m['m7.wave_files']
assert d['effectiveness']['coverage']['rollup_sessions'] == 1
assert not any('score' in k or 'total_score' in k for k in m), '发现综合分键'
FGPY
then
	echo "  ✓ G7 JSON m7.* 键数值可独立复算且无综合分键"
	pass=$((pass + 1))
else
	echo "  ✗ G7 JSON m7.* 键断言失败"
	failn=$((failn + 1))
fi
check_absent "G8 无综合分措辞" "综合分" "综合评分"

# ---------- H. 测试欠账巡检（test-debt-patrol：patrol.check 流水 + test_debt 台账） ----------
run_retro --db "$FH" --days 7
check 0 "H1 巡检段出现且数字正确（3 次巡检 ok 2；台账 open 2/fixed 1/waived 1；窗内新增 1 修复 1）" \
	"D 测试欠账巡检" "3 次（ok 2 / 失败 1，67% ok）" "（最近分片 fe-core）" \
	"open 2 / fixed 1 / waived 1；窗口内新增 1、修复 1"
"$RETRO" --db "$FH" --days 7 --json >"$TMP/fh.json" 2>/dev/null
if python3 - "$TMP/fh.json" <<'FHPY'
import json, sys
d = json.load(open(sys.argv[1]))
p = d['effectiveness']['patrol']
assert p['checks'] == 3 and p['ok'] == 2 and p['failed'] == 1, p
assert p['last_shard'] == 'fe-core', p
assert p['debt']['open'] == 2 and p['debt']['fixed'] == 1 and p['debt']['waived'] == 1, p['debt']
assert p['debt']['new_in_window'] == 1 and p['debt']['fixed_in_window'] == 1, p['debt']
FHPY
then
	echo "  ✓ H2 JSON effectiveness.patrol 键数值可独立复算"
	pass=$((pass + 1))
else
	echo "  ✗ H2 JSON effectiveness.patrol 断言失败"
	failn=$((failn + 1))
fi
run_retro --db "$FB" --days 7
check 0 "H3 无 test_debt 表 + 无 patrol.check 时降级（仍 exit 0）" "D 测试欠账巡检" \
	"无巡检数据（窗口内无 patrol.check 事件）" "test_debt 表不存在"
sqlite3 "$FH" "DELETE FROM events WHERE kind='patrol.check';"
run_retro --db "$FH" --days 7
check 0 "H4 有台账但零巡检事件（降级不报错）" "无巡检数据（窗口内无 patrol.check 事件）" "open 2 / fixed 1 / waived 1"

# ---------- I. 注入命中率口径回归（harness-retro-sql-fixes） ----------
run_retro --db "$FI" --days 7
check 0 "I1 截断域名修复后 join 命中（旧 substr 差一产出 readin 等永不命中：0/5）" "注入命中率        : 编辑域命中注入域 2 / 5（40%；"
check 0 "I2 jit-path 通道计入（旧枚举不认该通道）" "域映射=flow 头 doc-impact-applies"
check 0 "I3a 未识别通道显式计数（future-chan 1 条，漂移可见非静默归零）" "未识别通道        : 1 条" "⚠ 注入通道枚举已漂移"
check_absent "I3b 非 flow-doc 合法通道（mode-base）不进未识别计数" "未识别通道        : 2 条"
"$RETRO" --db "$FI" --days 7 --json >"$TMP/fi.json" 2>/dev/null
if python3 - "$TMP/fi.json" <<'FIPY'
import json, sys
d = json.load(open(sys.argv[1]))
m = d['metrics']
assert m['m7.domain_hit_pct'] == 40, m['m7.domain_hit_pct']  # hit=2(chg-fix reading+ai-summary)/edited=5(reader 前缀由 reading+content-enrichment 两域共亨：chg-fix 3 域+chg-unk 2 域；旧 substr 下为 0/5)
dh = d['effectiveness']['domain_hit']
assert dh['hit'] == 2 and dh['edited'] == 5, dh
assert dh['unrecognized_reason_rows'] == 1, dh  # 仅 future-chan；mode-base(非 flow-doc) 不计
FIPY
then
	echo "  ✓ I4 JSON 命中率/未识别计数=fixture 复算值（2/5=40%，未知通道 1）"
	pass=$((pass + 1))
else
	echo "  ✗ I4 JSON 口径断言失败"
	failn=$((failn + 1))
fi
run_retro --db "$FG" --days 7
check 0 "I5 零未识别通道时无漂移警告（fixture G 无未知通道）" "未识别通道        : 0 条"
check_absent "I5b 零未识别时不报漂移" "⚠ 注入通道枚举已漂移"

# ---------- J. 家族分类前置回归（harness-retro-sql-fixes） ----------
run_retro --db "$FJ" --days 7
check 0 "J1 command not found 归并发/环境冲突族（先于 %lint% 关键词）" "并发/环境冲突（疑非代码问题） 1"
check_absent "J2 真实 lint 失败不被环境风暴吞掉（仍归 lint 规则族）" "其它/未归类"
"$RETRO" --db "$FJ" --days 7 --json >"$TMP/fj.json" 2>/dev/null
if python3 - "$TMP/fj.json" <<'FJPY'
import json, sys
d = json.load(open(sys.argv[1]))
fams = {f['family']: f['n'] for f in d['gate']['families']}
assert fams.get('并发/环境冲突（疑非代码问题）') == 1, fams  # command not found → 环境族
assert fams.get('lint 规则') == 1, fams                      # 真实 lint 失败 → lint 族
assert fams.get('其它/未归类', 0) == 0, fams
FJPY
then
	echo "  ✓ J3 两类并存各归各族（环境族 1 + lint 族 1，零误归）"
	pass=$((pass + 1))
else
	echo "  ✗ J3 家族分类断言失败"
	failn=$((failn + 1))
fi

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
check 0 "A3c 与基线比对（无变化差值为 0）" "较基线变化（基线" "gate.failures            +0"
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
