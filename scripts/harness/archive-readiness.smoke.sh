#!/usr/bin/env bash
# archive-readiness.smoke.sh — archive-readiness.sh 的冒烟自测（harden-archive-readiness 1.2）
#
# 聚合逻辑（调度/汇总/退出码/透传）用 ARCHIVE_READINESS_BIN_DIR 测试缝注入 stub 脚本
# 驱动；只读安全 case 真跑本仓库 change（四项真实脚本均为只读）。子脚本各自判定由
# 其自有 smoke 覆盖，此处不重测。
#
# case 清单（对应 archive-readiness spec 的 Scenario）：
#   ① 全绿通过（stub 四脚本 exit 0 → exit 0，4×✓ + UI 边界行）
#   ② 多项红一次性汇总（stub 两红 → exit 1，同时列出两个 ✗ 并透传各自输出）
#   ③ change 不存在（真跑 → exit 2，指明目录不存在）
#   ④ 只读安全（真跑本仓库 change → 前后 git status --short 一致）
#   ⑤ 参数与名字校验（缺参 exit 2 / 非法名 exit 2 / openspec/changes/ 前缀剥离后正常解析）
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

# ① 全绿通过（stub 只接管四项判定；change 名须真实存在——目录存在性是真检查）
S1="$TMP/bin-ok"
for s in doc-impact.sh check-standards.sh check-tasks-tail.sh scenario-trace.sh; do mkstub "$S1" "$s" 0; done
out="$(ARCHIVE_READINESS_BIN_DIR="$S1" "$READY" harden-archive-readiness 2>&1)"
rc=$?
[ "$rc" -eq 0 ] && printf '%s\n' "$out" | grep -Ec '^✓ [①②③④]' | grep -q '^4$' && printf '%s\n' "$out" | grep -Fq 'UI 验收证据项不在自查范围'; check "① stub 全绿 → exit 0 + 4×✓ + UI 边界行" $?

# ② 多项红一次性汇总（doc-impact 与 trace 红，另两项绿）
S2="$TMP/bin-mixed"
mkstub "$S2" doc-impact.sh 1 "MISS doc1"
mkstub "$S2" check-standards.sh 0
mkstub "$S2" check-tasks-tail.sh 0
mkstub "$S2" scenario-trace.sh 1 "MISS trace1"
out="$(ARCHIVE_READINESS_BIN_DIR="$S2" "$READY" harden-archive-readiness 2>&1)"
rc=$?
[ "$rc" -eq 1 ] && printf '%s\n' "$out" | grep -Fq '✗ ① doc-impact verify' && printf '%s\n' "$out" | grep -Fq 'MISS doc1' && printf '%s\n' "$out" | grep -Fq '✗ ④ scenario-trace' && printf '%s\n' "$out" | grep -Fq 'MISS trace1' && printf '%s\n' "$out" | grep -Fq '✓ ② check-standards'; check "② 两红两绿 → exit 1，红项与各自输出一次列全" $?

# ③ change 不存在（真跑，无 stub）
out="$("$READY" no-such-change-xyz 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq 'change 目录不存在'; check "③ change 不存在 → exit 2 + 明确报错" $?

# ④ 只读安全（真跑本仓库 change，四项真实脚本均只读）
before="$(git -C "$REPO_ROOT" status --short)"
out="$("$READY" harden-archive-readiness 2>&1)"
rc=$?
after="$(git -C "$REPO_ROOT" status --short)"
[ "$before" = "$after" ]; check "④ 真跑本仓库 change → git status 前后一致（零修改；rc=$rc 属判定结果不设期望）" $?

# ⑤ 参数与名字校验
out="$("$READY" 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq '用法'; check "⑤a 缺参 → exit 2 + 用法" $?
out="$("$READY" 'bad/name!!' 2>&1)"
rc=$?
[ "$rc" -eq 2 ] && printf '%s' "$out" | grep -Fq '非法 change 名'; check "⑤b 非法名 → exit 2" $?
out="$(ARCHIVE_READINESS_BIN_DIR="$S1" "$READY" 'openspec/changes/harden-archive-readiness' 2>&1)"
rc=$?
[ "$rc" -eq 0 ]; check "⑤c openspec/changes/ 前缀容忍 → 剥离后正常解析（exit 0）" $?

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ archive-readiness.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ archive-readiness.smoke 失败（$failn/$((pass + failn))）"
exit 1
