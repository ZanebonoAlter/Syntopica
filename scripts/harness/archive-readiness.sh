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
# 只读判定：不修改仓库文件、不执行测试或编译命令。
# UI 验收证据项（门禁检查④'）不在自查范围（仅归档门禁校验）。
#
# 退出码：0 四项全绿；1 存在红项；2 用法错误或 change 目录不存在。
# 测试缝：ARCHIVE_READINESS_BIN_DIR 指向含同名四个脚本的目录时从该目录解析
# （仅 smoke 注入用，判定逻辑与真实脚本零差异）。
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
CHANGE_DIR="openspec/changes/$name"
if [ ! -d "$REPO_ROOT/$CHANGE_DIR" ]; then
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

echo ""
echo "UI 验收证据项不在自查范围（仅归档门禁校验）。"
if [ "$fails" -eq 0 ]; then
	echo "✓ 四项全绿，可以 openspec archive $name（树上并发脏文件时门禁还会 warn 提醒，不阻断）"
	exit 0
fi
echo "✗ ${fails} 项红——按上方各红项输出补齐后重跑本脚本，全绿再归档（勿补一项就试一次归档）"
exit 1
