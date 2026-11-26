#!/usr/bin/env bash
# check-tasks-tail.sh — tasks.md 尾三节 + doc-impact 声明标记检查（spec-gate 检查③抽出为
# 独立脚本，harden-archive-readiness：spec-gate 与 archive-readiness.sh 共用同一判定，单一事实源）
#
# 用法：bash scripts/harness/check-tasks-tail.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 判定（1/2 与原 spec-gate.ts checkTasksMd 逐字保真，迁移时勿改文案）：
#   1. 「## N. 测试」「## N. 文档」「## N. 验证」三个标题各自独立命中一次（顺序不限）
#      —— 行首锚定 + 严格两井号：命中 `## N. 测试`，不命中 `### N.x` 子节与正文引用
#   2. 文内含 `<!-- doc-impact:` 声明标记（含冒号，天然排除 -excuse 变体）
#   3. review 任务行前移判定（review-task-frontload，仅 complex 档）：
#      proposal.md 头部机器可读注释区（解析边界沿用 review-trace.sh：开头连续 <!-- --> 行，
#      空行可穿插，遇正文停）声明 `<!-- complexity: complex -->` 且无 `<!-- review: waived -->`
#      时，tasks.md 须存在 `- [ ]`/`- [x]` 开头且含 review/审查（不区分大小写）的任务行，
#      缺则退出码 1 + 两条修复指引；simple/无声明/proposal 缺席 change 不受影响
#   4. UI 验收任务行判定（add-ui-verification-trace-gate，三道防线第一道）：proposal 头部
#      声明 `<!-- ui-impact: major -->` 且 .openspec.yaml schema 为 syntopica-ui 时，
#      tasks.md 须存在 `- [ ]`/`- [x]` 开头且含 `ui-verification/report.md` 的任务行，
#      缺则退出码 1 + 修复指引；旧 schema（spec-driven）/非 major 不受影响。
#      实现期若误报率高可按 design Open Question 降级为归档单防线（不改变 spec 判定）
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

# proposal.md 头部机器可读注释区：文件开头连续的 <!-- ... --> 行（空行可穿插），
# 遇首个其他内容行停止——与 review-trace.sh head_markers 同实现（同词表同边界）
head_markers() {
	awk '
		/^[[:space:]]*<!--.*-->[[:space:]]*$/ { print; next }
		/^[[:space:]]*$/ { next }
		{ exit }
	' "$1" 2>/dev/null
}

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

# review 任务行判定：complex 且未 waived 才启用；marker 逐行全串匹配（与 review-trace.sh
# 同款正则），正文里的 complexity/review 字样不属于头部注释区，不识别
review_missing=0
ui_missing=0
PROPOSAL_MD="$CHANGE_DIR/proposal.md"
if [ -r "$PROPOSAL_MD" ]; then
	markers="$(head_markers "$PROPOSAL_MD")"
	if printf '%s\n' "$markers" | grep -Eq '^<!--[[:space:]]*complexity[[:space:]]*:[[:space:]]*complex[[:space:]]*-->$' \
		&& ! printf '%s\n' "$markers" | grep -Eq '^<!--[[:space:]]*review[[:space:]]*:[[:space:]]*waived[[:space:]]*-->$'; then
		# 任务行：`- [ ]`/`- [x]` 开头 + 行内含 review/审查（-i 不区分大小写，正文引用无勾选框前缀不命中）
		if ! grep -Eiq '^- \[[x ]\].*(review|审查)' "$TASKS_MD"; then
			review_missing=1
		fi
	fi
	# UI 验收任务行判定：major 且新 schema（syntopica-ui）才启用（旧 schema 零检查不硬阻断）
	if printf '%s\n' "$markers" | grep -Eq '^<!--[[:space:]]*ui-impact[[:space:]]*:[[:space:]]*major[[:space:]]*-->$' \
		&& grep -Eq '^schema:[[:space:]]*syntopica-ui[[:space:]]*$' "$CHANGE_DIR/.openspec.yaml" 2>/dev/null; then
		if ! grep -Eiq '^- \[[x ]\].*ui-verification/report\.md' "$TASKS_MD"; then
			ui_missing=1
		fi
	fi
fi

if [ "${#missing[@]}" -eq 0 ] && [ "$review_missing" -eq 0 ] && [ "$ui_missing" -eq 0 ]; then
	echo "✓ tasks.md 尾三节与 doc-impact 声明标记齐全"
	exit 0
fi

if [ "${#missing[@]}" -gt 0 ]; then
	printf '✗ 缺失：%s。要求尾三节标题各自独立存在（顺序不限），且文内含 <!-- doc-impact: domain ... --> 声明标记\n' "$(
		IFS='、'
		echo "${missing[*]}"
	)"
fi

if [ "$review_missing" -eq 1 ]; then
	cat <<'FIXEOF'
✗ complex 档 tasks.md 缺 review 任务行（proposal 头部声明 complexity: complex 且未豁免）——
  需至少一条「- [ ] 或 - [x] 开头且含 review/审查」的任务行
  修复路径（二选一）：
  1. 补任务行：tasks.md 增加 Review 节，如
     `- [ ] N.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账`
  2. 豁免：proposal.md 头部机器可读注释区（与 ui-impact 同层）声明
     `<!-- review: waived -->`（豁免理由写在 proposal 正文；纯文字类 change 才适用）
FIXEOF
fi

if [ "$ui_missing" -eq 1 ]; then
	cat <<'FIXEOF'
✗ major 档（新 schema）tasks.md 缺 UI 验收任务行（proposal 头部声明 ui-impact: major）——
  需至少一条「- [ ] 或 - [x] 开头且含 ui-verification/report.md」的任务行（三道防线第一道）
  修复：tasks.md 验证节（或人工任务行）补一行，如
  `- [ ] N.x 人工/agent-browser 验收：走 ui-design.md 主故事，判定建账 ui-verification/report.md`
  （归档时由 scripts/harness/ui-verification-trace.sh 对账建账实物）
FIXEOF
fi
exit 1
