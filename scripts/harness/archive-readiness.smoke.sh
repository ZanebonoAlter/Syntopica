#!/usr/bin/env bash
# archive-readiness.smoke.sh — archive-readiness.sh 的冒烟自测（harden-archive-readiness 1.2）
#
# 聚合逻辑（调度/汇总/退出码/透传）用 ARCHIVE_READINESS_BIN_DIR 测试缝注入 stub 脚本
# 驱动；只读安全 case 真跑本仓库 change（六项真实脚本均为只读）。子脚本各自判定由
# 其自有 smoke 覆盖，此处不重测。
#
# case 清单（对应 archive-readiness spec 的 Scenario）：
#   ① 全绿通过（stub 七脚本 exit 0 → exit 0，6×✓ + ⑦ 反馈对账绿）
#   ② 多项红一次性汇总（stub 两红 → exit 1，同时列出两个 ✗ 并透传各自输出）
#   ③ change 不存在（真跑 → exit 2，指明目录不存在）
#   ④ 只读安全（真跑本仓库 change → 前后 git status --short 一致）
#   ⑤ 参数与名字校验（缺参 exit 2 / 非法名 exit 2 / openspec/changes/ 前缀剥离后正常解析）
#   ⑥ ⑥ human-feedback 处置对账（add-change-lifecycle-retro 8.4）：fixture 未处置条目
#      → ⚠ 提醒行 + 透传 verify 输出 + exit 0 不阻断（五项 stub 全绿 + 真实 feedback.sh）
#   ⑦ feedback.sh 缺席 → 跳过提示，不影响六项判定与退出码
#   ⑧ review 痕迹红项聚合（add-review-trace-gate 1.3）：仅 review-trace 红 → exit 1，
#      该项红项输出透传 review-trace.sh 的失败原因与修复路径指引，其余五项绿
#   ⑨ ⑤ UI 验收痕迹对账在聚合内可见且参与汇总（add-ui-verification-trace-gate 2.4）：
#      真 ui-verification-trace.sh 红项（major 零痕迹）→ ✗ ⑤ 透传修复路径，其余项绿
#   ⑩ ⑥ 约束文档读取对账在聚合内可见且参与汇总（stage-driven-constraint-injection D6）：
#      真 constraint-read-trace.sh 红项（声明域未读）→ ✗ ⑥ 透传未读域清单与取数指引
#
# 用法：bash scripts/harness/archive-readiness.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
READY="$SCRIPT_DIR/archive-readiness.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0

check() { # $1=断言名 $2=布尔结果
	if [ "$2" = "0" ]; then
		pass=$((pass + 1))
	else
		failn=$((failn + 1))
		echo "✗ $1"
		[ -n "${out:-}" ] && printf '%s\n' "$out" | sed 's/^/    /'
	fi
}

mkstub() { # $1=目录 $2=脚本名 $3=退出码 $4=输出行（可选）
	mkdir -p "$1"
	printf '#!/usr/bin/env bash\necho "%s"\nexit %s\n' "${4:-stub $2 ok}" "$3" > "$1/$2"
	chmod +x "$1/$2"
}

# fixture changes 目录（①②⑤c⑦共用；隔离真实仓库状态——引用的 change 后续归档不再
# 破坏 smoke；2026-09-29 实例：harden-archive-readiness 归档后 ①②⑤c 集体红）
FIXC="$TMP/fchanges"
mkdir -p "$FIXC/live-stub"
printf '# stub proposal\n' >"$FIXC/live-stub/proposal.md"

# ① 全绿通过（stub 只接管六项判定 + feedback stub；目录存在性走 fixture）
S1="$TMP/bin-ok"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh review-trace.sh ui-verification-trace.sh constraint-read-trace.sh diagrams-trace.sh feedback.sh; do mkstub "$S1" "$s" 0; done
out="$(ARCHIVE_READINESS_BIN_DIR="$S1" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s\n' "$out" | grep -Ec '^✓ (①|②|③|④|⑤|⑥|⑧) ' | grep -q '^7$' && printf '%s\n' "$out" | grep -Fq '✓ ⑤ UI 验收痕迹对账' && printf '%s\n' "$out" | grep -Fq '✓ ⑥ 约束文档读取对账' && printf '%s\n' "$out" | grep -Fq '✓ ⑧ diagrams marker' && printf '%s\n' "$out" | grep -Fq '✓ ⑦ human-feedback 处置对账'; check "① stub 全绿 → exit 0 + 7×✓（含⑤⑥⑧）+ ⑦ 反馈对账绿" $?

# ② 多项红一次性汇总（doc-impact 与 review-trace 红，另两项绿）
S2="$TMP/bin-mixed"
mkstub "$S2" doc-impact.sh 1 "MISS doc1"
mkstub "$S2" check-standards.sh 0
mkstub "$S2" check-tasks-tail.sh 0
mkstub "$S2" review-trace.sh 1 "MISS trace1"
mkstub "$S2" ui-verification-trace.sh 0
mkstub "$S2" constraint-read-trace.sh 0
mkstub "$S2" diagrams-trace.sh 0
out="$(ARCHIVE_READINESS_BIN_DIR="$S2" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s\n' "$out" | grep -Fq '✗ ① doc-impact verify' && printf '%s\n' "$out" | grep -Fq 'MISS doc1' && printf '%s\n' "$out" | grep -Fq '✗ ④ review-trace' && printf '%s\n' "$out" | grep -Fq 'MISS trace1' && printf '%s\n' "$out" | grep -Fq '✓ ② check-standards'; check "② 两红两绿 → exit 1，红项与各自输出一次列全" $?

# ③ change 不存在（真跑，无 stub）
out="$("$READY" no-such-change-xyz 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq 'change 目录不存在'; check "③ change 不存在 → exit 2 + 明确报错" $?

# ④ 只读安全（真跑本仓库当前活跃 change，四项真实脚本均只读；动态取名——
# 引用固定 change 名会在其归档后漂移红，2026-09-29 实例：harden-archive-readiness）
live_change="$(find "$REPO_ROOT/openspec/changes" -mindepth 1 -maxdepth 1 -type d ! -name archive -printf '%f\n' 2>/dev/null | head -1)"
if [ -n "$live_change" ]; then
	before="$(git -C "$REPO_ROOT" status --short)"
	out="$(("$READY" "$live_change") 2>&1)"
	rc=$?
	after="$(git -C "$REPO_ROOT" status --short)"
	[ "$before" = "$after" ]; check "④ 真跑本仓库 change（$live_change）→ git status 前后一致（零修改；rc=$rc 属判定结果不设期望）" $?
else
	check "④ 无活跃 change 可真跑（跳过）" 0
fi

# ⑤ 参数与名字校验
out="$("$READY" 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq '用法'; check "⑤a 缺参 → exit 2 + 用法" $?
out="$("$READY" 'bad/name!!' 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq '非法 change 名'; check "⑤b 非法名 → exit 2" $?
out="$(ARCHIVE_READINESS_BIN_DIR="$S1" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" 'openspec/changes/live-stub' 2>&1)"
rc=$?
[ "$rc" -eq 0 ]; check "⑤c openspec/changes/ 前缀容忍 → 剥离后正常解析（exit 0）" $?

# ⑥ ⑦ 反馈对账：fixture 未处置条目 → ⚠ 提醒 + 透传 + 不阻断（add-change-lifecycle-retro 8.4）
# 六项 stub 全绿 + 真实 feedback.sh（其自有 smoke 覆盖词表/解析；此处测透传与不阻断语义）
S3="$TMP/bin-fb"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh review-trace.sh ui-verification-trace.sh constraint-read-trace.sh diagrams-trace.sh; do mkstub "$S3" "$s" 0; done
ln -s "$SCRIPT_DIR/feedback.sh" "$S3/feedback.sh"
FBC="$TMP/fbchanges"
mkdir -p "$FBC/fb-red"
cat >"$FBC/fb-red/human-feedback.md" <<'FBRED'
### FB-1 2026-09-20【产品|frontend】原型返工

- **状态**：待处理
FBRED
out="$(ARCHIVE_READINESS_BIN_DIR="$S3" ARCHIVE_READINESS_CHANGES_DIR="$FBC" "$READY" fb-red 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq '⚠ ⑦ human-feedback 处置对账' && printf '%s' "$out" | grep -Fq 'FB-1 待处理' && printf '%s' "$out" | grep -Fq '提醒不阻断' && printf '%s' "$out" | grep -Fq '✓ 六项全绿'; check "⑥ fixture 未处置反馈条目 → ⚠ 提醒行 + 透传 verify 输出 + exit 0 不阻断" $?

# ⑦ feedback.sh 缺席 → 跳过提示（兼容老环境/部分部署；不影响六项判定与退出码）
out="$(ARCHIVE_READINESS_BIN_DIR="$S2" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '⑦ human-feedback 处置对账跳过'; check "⑦ feedback.sh 缺席 → 跳过提示不影响六项判定（exit 1 维持）" $?

# ⑧ review 痕迹红项聚合（add-review-trace-gate）：仅 review-trace 红 → exit 1，
# 透传真实 review-trace.sh 的失败原因与修复路径，其余项绿（真跑真判定，不 stub）
S4="$TMP/bin-rt"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh feedback.sh constraint-read-trace.sh; do mkstub "$S4" "$s" 0; done
ln -s "$SCRIPT_DIR/review-trace.sh" "$S4/review-trace.sh"
ln -s "$SCRIPT_DIR/ui-verification-trace.sh" "$S4/ui-verification-trace.sh"
RTC="$TMP/rtchanges"
mkdir -p "$RTC/rt-zero"
printf '<!-- ui-impact: none -->\n\n## Why\nx\n' >"$RTC/rt-zero/proposal.md"
out="$(ARCHIVE_READINESS_BIN_DIR="$S4" ARCHIVE_READINESS_CHANGES_DIR="$RTC" "$READY" rt-zero 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '✗ ④ review-trace' && printf '%s' "$out" | grep -Fq '零痕迹' && printf '%s' "$out" | grep -Fq '修复路径（二选一）' && printf '%s' "$out" | grep -Fq '✓ ① doc-impact verify' && printf '%s' "$out" | grep -Fq '✓ ③ tasks.md 尾三节 + doc-impact 声明' && printf '%s' "$out" | grep -Fq '✓ ⑤ UI 验收痕迹对账'; check "⑧ 仅 review-trace 红 → exit 1 + 透传失败原因与修复路径，其余五项绿" $?

# ⑨ ⑤ UI 验收痕迹对账参与汇总（add-ui-verification-trace-gate 2.4）：真脚本红项
# （major 零痕迹 fixture）→ ✗ ⑤ 透传修复路径；旧 schema fixture 走 fail-open 绿
S5="$TMP/bin-uiv"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh review-trace.sh feedback.sh constraint-read-trace.sh; do mkstub "$S5" "$s" 0; done
ln -s "$SCRIPT_DIR/ui-verification-trace.sh" "$S5/ui-verification-trace.sh"
UIC="$TMP/uichanges"
mkdir -p "$UIC/ui-zero"
printf 'schema: syntopica-ui\ncreated: 2026-10-14\n' >"$UIC/ui-zero/.openspec.yaml"
printf '<!-- ui-impact: major -->\n\n## Why\nx\n' >"$UIC/ui-zero/proposal.md"
out="$(ARCHIVE_READINESS_BIN_DIR="$S5" ARCHIVE_READINESS_CHANGES_DIR="$UIC" "$READY" ui-zero 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '✗ ⑤ UI 验收痕迹对账' && printf '%s' "$out" | grep -Fq '零痕迹' && printf '%s' "$out" | grep -Fq '显式降档'; check "⑨ major 零痕迹 → ✗ ⑤ 透传修复路径 + exit 1（UI 验收入聚合自查）" $?

# ⑩ ⑥ 约束文档读取对账参与汇总（stage-driven-constraint-injection D6）：真脚本红项
# （声明域 flow 文档未读）→ ✗ ⑥ 透传未读域清单与取数指引；legacy change → ✓ ⑥ 豁免
S6="$TMP/bin-crt"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh review-trace.sh ui-verification-trace.sh diagrams-trace.sh feedback.sh; do mkstub "$S6" "$s" 0; done
ln -s "$SCRIPT_DIR/constraint-read-trace.sh" "$S6/constraint-read-trace.sh"
CRC="$TMP/crtchanges"
mkdir -p "$CRC/crt-red" "$CRC/crt-legacy"
printf 'schema: syntopica-ui\ncreated: %s\n' "$(date -u +%Y-%m-%d)" >"$CRC/crt-red/.openspec.yaml"
printf '<!-- stage: implement -->\n' >"$CRC/crt-red/stage.md"
printf '<!-- constraint-domains: daily-report -->\n' >"$CRC/crt-red/proposal.md"
printf '<!-- complexity: simple -->\n' >"$CRC/crt-legacy/proposal.md"
CRDB="$TMP/crt-events.db"
sqlite3 "$CRDB" "CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, session_id TEXT, kind TEXT, change TEXT, payload TEXT);"
out="$(CONSTRAINT_READ_TRACE_DB="$CRDB" ARCHIVE_READINESS_BIN_DIR="$S6" ARCHIVE_READINESS_CHANGES_DIR="$CRC" "$READY" crt-red 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '✗ ⑥ 约束文档读取对账' && printf '%s' "$out" | grep -Fq 'daily-report' && printf '%s' "$out" | grep -Fq 'constraint-digest.sh --brief --domains daily-report' && printf '%s' "$out" | grep -Fq '✓ ① doc-impact verify'; check "⑩ 声明域未读 → ✗ ⑥ 透传未读域与取数指引 + exit 1" $?
out="$(CONSTRAINT_READ_TRACE_DB="$CRDB" ARCHIVE_READINESS_BIN_DIR="$S6" ARCHIVE_READINESS_CHANGES_DIR="$CRC" "$READY" crt-legacy 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq '✓ ⑥ 约束文档读取对账'; check "⑩b legacy change（无 stage.md）→ ✓ ⑥ 豁免 + exit 0" $?

# ⑩c–⑩e 直接驱动 constraint-read-trace.sh（R-2/R-4 口径）：证据须同时满足
# 「归属本 change」+「实现期窗内」——他会话的读取、规划期的读取都不算证据。
CRT="$SCRIPT_DIR/constraint-read-trace.sh"
mk_crt_change() {
	mkdir -p "$CRC/$1"
	printf 'schema: syntopica-ui\ncreated: 2026-01-01\n' >"$CRC/$1/.openspec.yaml"
	printf '<!-- stage: implement -->\n' >"$CRC/$1/stage.md"
	printf '<!-- constraint-domains: daily-report -->\n' >"$CRC/$1/proposal.md"
}
mk_crt_change crt-green
mk_crt_change crt-other
mk_crt_change crt-plan
T0="$(date -u -d '-3 hours' +%Y-%m-%dT%H:%M:%S.000Z)"
T1="$(date -u -d '-2 hours' +%Y-%m-%dT%H:%M:%S.000Z)"
T2="$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%S.000Z)"
ins() { sqlite3 "$CRDB" "INSERT INTO events (ts, session_id, kind, change, payload) VALUES ('$1', '$2', '$3', '$4', '$5');"; }
# 绿：归属本 change + 实现期（impl stage.set 于 T0）之后的成功读取（T1）
ins "$T0" s-g stage.set crt-green '{"stage":"implement","stageFamily":"implementation","boundChange":"crt-green","source":"command"}'
ins "$T1" s-g constraint.read crt-green '{"path":"docs/reference/flow/daily-report.md","boundChange":"crt-green"}'
out="$(CONSTRAINT_READ_TRACE_DB="$CRDB" bash "$CRT" "$CRC/crt-green" 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq '实现期窗'; check "⑩c 归属本 change 的实现期读取 → ✓ 通过 + exit 0" $?
# 红（R-2 归属）：读取归属他会话（change=other-change），即使落在同窗口
ins "$T0" s-o stage.set crt-other '{"stage":"implement","stageFamily":"implementation","boundChange":"crt-other","source":"command"}'
ins "$T1" s-o constraint.read other-change '{"path":"docs/reference/flow/daily-report.md","boundChange":"other-change"}'
out="$(CONSTRAINT_READ_TRACE_DB="$CRDB" bash "$CRT" "$CRC/crt-other" 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '归属 change=crt-other'; check "⑩d 他会话的读取不算本 change 证据 → ✗ + exit 1（R-2 归属）" $?
# 红（R-2 窗口）：读取归属本 change 但发生在 impl stage.set 之前（规划期）
ins "$T2" s-p stage.set crt-plan '{"stage":"implement","stageFamily":"implementation","boundChange":"crt-plan","source":"command"}'
ins "$T1" s-p constraint.read crt-plan '{"path":"docs/reference/flow/daily-report.md","boundChange":"crt-plan"}'
out="$(CONSTRAINT_READ_TRACE_DB="$CRDB" bash "$CRT" "$CRC/crt-plan" 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq 'daily-report'; check "⑩e 规划期读取不算实现期证据 → ✗ + exit 1（R-2 窗口）" $?

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ archive-readiness.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ archive-readiness.smoke 失败（$failn/$((pass + failn))）"
exit 1
