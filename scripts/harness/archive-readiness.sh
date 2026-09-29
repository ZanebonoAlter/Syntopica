#!/usr/bin/env bash
# archive-readiness.sh — 归档就绪聚合自查（harden-archive-readiness 引入）
#
# 用法：bash scripts/harness/archive-readiness.sh <change>
#   <change> 为 change 名（kebab-case）；容忍 openspec/changes/<name> 前缀路径（剥离）
#
# 与 spec-gate 归档门禁同口径的四项 block 检查聚合自查——四项各自调用门禁同一批
# 脚本（单一事实源，不另立判定）：
#   ① doc-impact verify   ② check-standards   ③ check-tasks-tail   ④ scenario-trace
# 逐项输出 ✓/✗，红项透传该脚本输出尾部 20 行（与门禁 runScript 的 tail 口径一致）。
# 另顺带 ⑤ human-feedback 处置对账（add-change-lifecycle-retro 8.4）：调用
# feedback.sh verify 透传结果语义——未处置/状态不明反馈条目 → ⚠ 提醒行，
# 不计入 fails、不影响退出码（提醒层，区别于四项硬检查，不进 spec-gate 硬门禁）。
# 只读判定：不修改仓库文件、不执行测试或编译命令。
# UI 验收证据项（门禁检查④'）不在自查范围（仅归档门禁校验）。
#
# 退出码：0 四项全绿；1 存在红项；2 用法错误或 change 目录不存在（⑤ 恒不影响）。
# 测试缝：ARCHIVE_READINESS_BIN_DIR 指向含同名五个脚本的目录时从该目录解析
# （仅 smoke 注入用，判定逻辑与真实脚本零差异）；ARCHIVE_READINESS_CHANGES_DIR
# 替换 changes 目录位置（smoke fixture 注入用，缺省 openspec/changes）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HARNESS_DIR="$REPO_ROOT/scripts/harness"
BIN_DIR="${ARCHIVE_READINESS_BIN_DIR:-$HARNESS_DIR}"

name="${1:-}"
if [ -z "$name" ]; then
	echo "用法: bash scripts/harness/archive-readiness.sh <change>  （如 my-change）"
	echo "在 openspec archive 之前先跑本脚本自查，全绿（退出码 0）再归档。"
	exit 2
fi
name="${name#openspec/changes/}"
if ! [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
	echo "✗ 非法 change 名: $name（须为 kebab-case 形态）"
	exit 2
fi
CHANGE_DIR="${ARCHIVE_READINESS_CHANGES_DIR:-openspec/changes}/$name"
# 测试缝注入绝对路径时不与 REPO_ROOT 拼接（否则 /repo/tmp/... 双重前缀恒不存在）
if [[ "$CHANGE_DIR" = /* ]]; then
	CHANGE_ABS="$CHANGE_DIR"
else
	CHANGE_ABS="$REPO_ROOT/$CHANGE_DIR"
fi
if [ ! -d "$CHANGE_ABS" ]; then
	echo "✗ change 目录不存在: $CHANGE_DIR"
	exit 2
fi

# ---------- 四项聚合（标题|脚本|参数…） ----------

run_item() { # $1=检查名 剩余=命令；输出 ✓/✗ + 红项尾部 20 行，rc 进 item_rc
	local label="$1"
	shift
	local out rc
	out="$("$@" 2>&1)"
	rc=$?
	item_rc=$rc
	if [ "$rc" -eq 0 ]; then
		echo "✓ ${label}"
	else
		echo "✗ ${label}（exit ${rc}）"
		if [ -n "$out" ]; then
			printf '%s\n' "$out" | tail -n 20 | sed 's/^/    /'
		fi
	fi
}

fails=0

echo "归档就绪自查：$CHANGE_DIR（与 spec-gate 归档门禁同口径四项）"
echo ""

run_item "① doc-impact verify" bash "$BIN_DIR/doc-impact.sh" verify "$CHANGE_DIR" || true
[ "$item_rc" -ne 0 ] && fails=$((fails + 1))

run_item "② check-standards" bash "$BIN_DIR/check-standards.sh" --change "$name" || true
[ "$item_rc" -ne 0 ] && fails=$((fails + 1))

run_item "③ tasks.md 尾三节 + doc-impact 声明" bash "$BIN_DIR/check-tasks-tail.sh" "$CHANGE_DIR" || true
[ "$item_rc" -ne 0 ] && fails=$((fails + 1))

run_item "④ scenario-trace 映射对账" bash "$BIN_DIR/scenario-trace.sh" "$CHANGE_DIR" || true
[ "$item_rc" -ne 0 ] && fails=$((fails + 1))

# ---------- ⑤ human-feedback 处置对账（add-change-lifecycle-retro 8.4：提醒不阻断） ----------
# 顺带调用 feedback.sh verify 透传结果语义：未处置（待处理/处理中/状态不明）条目 →
# ⚠ 提醒 + 透传输出尾部 10 行；不计入 fails、不影响退出码（轻量对账，与 test-patrol
# 登记、doc-impact verify 同层的脚本级对账，不进 spec-gate 硬门禁）。
fb_remind=0
if [ -f "$BIN_DIR/feedback.sh" ]; then
	fb_out="$(bash "$BIN_DIR/feedback.sh" verify "$CHANGE_DIR" 2>&1)"
	fb_rc=$?
	if [ "$fb_rc" -eq 0 ]; then
		echo "✓ ⑤ human-feedback 处置对账"
	else
		fb_remind=1
		echo "⚠ ⑤ human-feedback 处置对账：存在未处置/状态不明反馈条目（提醒不阻断，不进 spec-gate 硬门禁）"
		if [ -n "$fb_out" ]; then
			printf '%s\n' "$fb_out" | tail -n 10 | sed 's/^/    /'
		fi
	fi
else
	echo "— ⑤ human-feedback 处置对账跳过（feedback.sh 缺席）"
fi

echo ""
echo "UI 验收证据项不在自查范围（仅归档门禁校验）。"
if [ "$fails" -eq 0 ]; then
	echo "✓ 四项全绿，可以 openspec archive $name（树上并发脏文件时门禁还会 warn 提醒，不阻断）"
	if [ "$fb_remind" -eq 1 ]; then
		echo "⚠ 反馈对账提醒见上——转新 change 或更新条目状态后再归档更干净（不阻断）"
	fi
	exit 0
fi
echo "✗ ${fails} 项红——按上方各红项输出补齐后重跑本脚本，全绿再归档（勿补一项就试一次归档）"
exit 1
