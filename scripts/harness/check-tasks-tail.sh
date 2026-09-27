#!/usr/bin/env bash
# check-tasks-tail.sh — tasks.md 尾三节 + doc-impact 声明标记检查（spec-gate 检查③抽出为
# 独立脚本，harden-archive-readiness：spec-gate 与 archive-readiness.sh 共用同一判定，单一事实源）
#
# 用法：bash scripts/harness/check-tasks-tail.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 判定（与原 spec-gate.ts checkTasksMd 逐字保真，迁移时勿改文案）：
#   1. 「## N. 测试」「## N. 文档」「## N. 验证」三个标题各自独立命中一次（顺序不限）
#      —— 行首锚定 + 严格两井号：命中 `## N. 测试`，不命中 `### N.x` 子节与正文引用
#   2. 文内含 `<!-- doc-impact:` 声明标记（含冒号，天然排除 -excuse 变体）
#
# 退出码：0 通过；1 失败（tasks.md 不存在/不可读或缺项）。POSIX bash，只判不跑。
set -u

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ]; then
	echo "用法: bash scripts/harness/check-tasks-tail.sh <change-dir>  （如 openspec/changes/<name>）"
	exit 1
fi

TASKS_MD="$CHANGE_DIR/tasks.md"
if [ ! -r "$TASKS_MD" ]; then
	echo "✗ 读取 $TASKS_MD 失败：文件不存在或不可读"
	exit 1
fi

missing=()
for sec in 测试 文档 验证; do
	# 行首锚定 + 严格两井号 + 数字编号；[[:space:]] 对齐 TS 正则 \s（仅 ASCII 空白，比 JS \s 略严——全角空白编号属病态输入，两版均视为缺节）
	if ! grep -Eq "^##[[:space:]]+[0-9]+\.[[:space:]]*${sec}" "$TASKS_MD"; then
		missing+=("「## N. ${sec}」")
	fi
done
# 含冒号的完整标记；grep -F 固定串对齐 TS includes
if ! grep -Fq '<!-- doc-impact:' "$TASKS_MD"; then
	missing+='`<!-- doc-impact:` 声明标记'
fi

if [ "${#missing[@]}" -eq 0 ]; then
	echo "✓ tasks.md 尾三节与 doc-impact 声明标记齐全"
	exit 0
fi

printf '✗ 缺失：%s。要求尾三节标题各自独立存在（顺序不限），且文内含 <!-- doc-impact: domain ... --> 声明标记\n' "$(
	IFS='、'
	echo "${missing[*]}"
)"
exit 1
