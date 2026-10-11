#!/usr/bin/env bash
# archive-readiness.smoke.sh — archive-readiness.sh 的冒烟自测（harden-archive-readiness 1.2）
#
# 聚合逻辑（调度/汇总/退出码/透传）用 ARCHIVE_READINESS_BIN_DIR 测试缝注入 stub 脚本
# 驱动；只读安全 case 真跑本仓库 change（五项真实脚本均为只读）。子脚本各自判定由
# 其自有 smoke 覆盖，此处不重测。
#
# case 清单（对应 archive-readiness spec 的 Scenario）：
#   ① 全绿通过（stub 五脚本 exit 0 → exit 0，5×✓ + UI 边界行）
#   ② 多项红一次性汇总（stub 两红 → exit 1，同时列出两个 ✗ 并透传各自输出）
#   ③ change 不存在（真跑 → exit 2，指明目录不存在）
#   ④ 只读安全（真跑本仓库 change → 前后 git status --short 一致）
#   ⑤ 参数与名字校验（缺参 exit 2 / 非法名 exit 2 / openspec/changes/ 前缀剥离后正常解析）
#   ⑥ ⑥ human-feedback 处置对账（add-change-lifecycle-retro 8.4）：fixture 未处置条目
#      → ⚠ 提醒行 + 透传 verify 输出 + exit 0 不阻断（五项 stub 全绿 + 真实 feedback.sh）
#   ⑦ feedback.sh 缺席 → 跳过提示，不影响五项判定与退出码
#   ⑧ review 痕迹红项聚合（add-review-trace-gate 1.3）：仅 review-trace 红 → exit 1，
#      该项红项输出透传 review-trace.sh 的失败原因与修复路径指引，其余四项绿
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

# ① 全绿通过（stub 只接管五项判定 + feedback stub；目录存在性走 fixture）
S1="$TMP/bin-ok"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh scenario-trace.sh review-trace.sh feedback.sh; do mkstub "$S1" "$s" 0; done
out="$(ARCHIVE_READINESS_BIN_DIR="$S1" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s\n' "$out" | grep -Ec '^✓ [①②③④⑤]' | grep -q '^5$' && printf '%s\n' "$out" | grep -Fq 'UI 验收证据项不在自查范围' && printf '%s\n' "$out" | grep -Fq '✓ ⑥ human-feedback 处置对账'; check "① stub 全绿 → exit 0 + 5×✓ + ⑥ 反馈对账绿 + UI 边界行" $?

# ② 多项红一次性汇总（doc-impact 与 trace 红，另两项绿）
S2="$TMP/bin-mixed"
mkstub "$S2" doc-impact.sh 1 "MISS doc1"
mkstub "$S2" check-standards.sh 0
mkstub "$S2" check-tasks-tail.sh 0
mkstub "$S2" scenario-trace.sh 1 "MISS trace1"
mkstub "$S2" review-trace.sh 0
out="$(ARCHIVE_READINESS_BIN_DIR="$S2" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s\n' "$out" | grep -Fq '✗ ① doc-impact verify' && printf '%s\n' "$out" | grep -Fq 'MISS doc1' && printf '%s\n' "$out" | grep -Fq '✗ ④ scenario-trace' && printf '%s\n' "$out" | grep -Fq 'MISS trace1' && printf '%s\n' "$out" | grep -Fq '✓ ② check-standards'; check "② 两红两绿 → exit 1，红项与各自输出一次列全" $?

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

# ⑥ ⑥ 反馈对账：fixture 未处置条目 → ⚠ 提醒 + 透传 + 不阻断（add-change-lifecycle-retro 8.4）
# 五项 stub 全绿 + 真实 feedback.sh（其自有 smoke 覆盖词表/解析；此处测透传与不阻断语义）
S3="$TMP/bin-fb"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh scenario-trace.sh review-trace.sh; do mkstub "$S3" "$s" 0; done
ln -s "$SCRIPT_DIR/feedback.sh" "$S3/feedback.sh"
FBC="$TMP/fbchanges"
mkdir -p "$FBC/fb-red"
cat >"$FBC/fb-red/human-feedback.md" <<'FBRED'
### FB-1 2026-09-20【产品|frontend】原型返工

- **状态**：待处理
FBRED
out="$(ARCHIVE_READINESS_BIN_DIR="$S3" ARCHIVE_READINESS_CHANGES_DIR="$FBC" "$READY" fb-red 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq '⚠ ⑥ human-feedback 处置对账' && printf '%s' "$out" | grep -Fq 'FB-1 待处理' && printf '%s' "$out" | grep -Fq '提醒不阻断' && printf '%s' "$out" | grep -Fq '✓ 五项全绿'; check "⑥ fixture 未处置反馈条目 → ⚠ 提醒行 + 透传 verify 输出 + exit 0 不阻断" $?

# ⑦ feedback.sh 缺席 → 跳过提示（兼容老环境/部分部署；不影响五项判定与退出码）
out="$(ARCHIVE_READINESS_BIN_DIR="$S2" ARCHIVE_READINESS_CHANGES_DIR="$FIXC" "$READY" live-stub 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '⑥ human-feedback 处置对账跳过'; check "⑦ feedback.sh 缺席 → 跳过提示不影响五项判定（exit 1 维持）" $?

# ⑧ review 痕迹红项聚合（add-review-trace-gate）：仅 review-trace 红 → exit 1，
# 透传真实 review-trace.sh 的失败原因与修复路径，其余四项绿（真跑真判定，不 stub）
S4="$TMP/bin-rt"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh scenario-trace.sh feedback.sh; do mkstub "$S4" "$s" 0; done
ln -s "$SCRIPT_DIR/review-trace.sh" "$S4/review-trace.sh"
RTC="$TMP/rtchanges"
mkdir -p "$RTC/rt-zero"
printf '<!-- ui-impact: none -->\n\n## Why\nx\n' >"$RTC/rt-zero/proposal.md"
out="$(ARCHIVE_READINESS_BIN_DIR="$S4" ARCHIVE_READINESS_CHANGES_DIR="$RTC" "$READY" rt-zero 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s' "$out" | grep -Fq '✗ ⑤ review-trace' && printf '%s' "$out" | grep -Fq '零痕迹' && printf '%s' "$out" | grep -Fq '修复路径（二选一）' && printf '%s' "$out" | grep -Fq '✓ ① doc-impact verify' && printf '%s' "$out" | grep -Fq '✓ ④ scenario-trace'; check "⑧ 仅 review-trace 红 → exit 1 + 透传失败原因与修复路径，其余四项绿" $?

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ archive-readiness.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ archive-readiness.smoke 失败（$failn/$((pass + failn))）"
exit 1
