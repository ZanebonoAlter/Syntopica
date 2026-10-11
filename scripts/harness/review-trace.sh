#!/usr/bin/env bash
# review-trace.sh — review 痕迹归档对账门禁（add-review-trace-gate 引入）
#
# 用法：bash scripts/harness/review-trace.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 只读对账（不执行任何测试/编译命令、不修改仓库文件），判定按优先级：
#   ① proposal.md 头部机器可读注释区（与 ui-impact / complexity 同层）存在
#      <!-- review: waived --> → 通过（输出注明 waived 形态，供 retro 统计区分）
#   ② review.md 含 ≥1 条合法条目或合法「审毕无发现」声明 → 通过（输出条目计数）
#   ③ 两者皆无 / 空壳文件（有 review.md 但零合法条目）→ 退出码 1 + 两条修复路径
#   优先级：waived 先于 review.md；两者同存在 → 通过且输出注明双痕迹
#
# review.md 建账格式契约（design D3，仿 human-feedback.md 一行正则全解析）：
#   ### R-<N> <YYYY-MM-DD>【High|Medium|Low】<标题>    编号自 R-1 自增
#   - **状态**：<处置词>（括号说明可选）                状态行紧随标题行
#   处置词封闭五值：待处理/已修复/已采纳/已驳回/误报丢弃
#   零发现显式声明行：- 审毕无发现（审查范围：<说明>）
#   等级词表封闭三值 High/Medium/Low；词表外等级/状态 → 该条目不计入合法条目，
#   全文件无其他合法条目则对账失败（空壳同判）。
#
# 退出码：0 通过；1 有失败（中文输出修复路径指引）；1 用法错误/change 目录不存在。
# POSIX bash（Linux / macOS / WSL）可跑。
# 格式约定详见 openspec/specs/review-trace-gate/spec.md（随 change 归档同步）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ]; then
	echo "用法: bash scripts/harness/review-trace.sh <change-dir>  （如 openspec/changes/<name>）"
	exit 1
fi
if [ ! -d "$CHANGE_DIR" ]; then
	echo "✗ change 目录不存在: $CHANGE_DIR"
	exit 1
fi

PROPOSAL="$CHANGE_DIR/proposal.md"
REVIEW_MD="$CHANGE_DIR/review.md"

# ---------- 格式契约正则（与 spec 词表封闭值同步） ----------

# 条目标题行：### R-<N> <YYYY-MM-DD>【High|Medium|Low】<非空标题>
ENTRY_RE='^### R-[0-9]+ [0-9]{4}-[0-9]{2}-[0-9]{2}【(High|Medium|Low)】.+'
# 状态行（紧随标题行）：- **状态**：<五值封闭>（括号说明可选）
STATUS_RE='^- \*\*状态\*\*：(待处理|已修复|已采纳|已驳回|误报丢弃)(（.*）)?[[:space:]]*$'
# 零发现显式声明行：- 审毕无发现（审查范围：<非空说明>）
NOFINDING_RE='^- 审毕无发现（审查范围：.+）[[:space:]]*$'

# ---------- 判定小工具 ----------

# proposal.md 头部机器可读注释区：文件开头连续的 <!-- ... --> 行（空行可穿插），
# 遇首个其他内容行停止——正文段落里的 marker 不属于头部注释区（不识别）
head_markers() {
	awk '
		/^[[:space:]]*<!--.*-->[[:space:]]*$/ { print; next }
		/^[[:space:]]*$/ { next }
		{ exit }
	' "$1" 2>/dev/null
}

# 合法条目计数：标题行合法 + 其后首个非空行是合法状态行 → 1 条（与 human-feedback
# 建账形态同构：标题与状态行间允许空行）；词表外等级/状态不计入
count_entries() {
	local f="$1" n=0 pending=0 line
	[ -f "$f" ] || { echo 0; return; }
	while IFS= read -r line || [ -n "$line" ]; do
		if [[ "$line" =~ $ENTRY_RE ]]; then
			pending=1
			continue
		fi
		if [ "$pending" -eq 1 ]; then
			if [[ "$line" =~ $STATUS_RE ]]; then
				n=$((n + 1))
				pending=0
			elif [[ -z "${line//[[:space:]]/}" ]]; then
				: # 空行：继续等状态行（markdown 标题与列表项间的惯例空行）
			else
				pending=0
			fi
		fi
	done < "$f"
	echo "$n"
}

has_nofinding() { [ -f "$1" ] && grep -Eq "$NOFINDING_RE" "$1"; }

print_fix_paths() {
	cat <<'FIXEOF'
  修复路径（二选一）：
  1. 建账：在 change 目录创建 review.md，按格式契约记录 review 意见——
         ### R-1 2026-10-12【High】<标题>
         - **状态**：待处理
     （等级词表 High/Medium/Low；状态词表 待处理/已修复/已采纳/已驳回/误报丢弃）
     审了没问题的写显式声明行：- 审毕无发现（审查范围：<说明>）
  2. 豁免：在 proposal.md 头部机器可读注释区（与 ui-impact 同层）声明
     <!-- review: waived -->（豁免理由写在 proposal 正文；waived 占比进 retro 统计防滥用）
FIXEOF
}

# ---------- 判定（优先级：waived 先于 review.md） ----------

waived=0
if [ -f "$PROPOSAL" ] && head_markers "$PROPOSAL" | grep -Eq '^<!--[[:space:]]*review[[:space:]]*:[[:space:]]*waived[[:space:]]*-->$'; then
	waived=1
fi

# ① waived 显式豁免 → 通过（双痕迹时注明）
if [ "$waived" -eq 1 ]; then
	dual=""
	if [ -f "$REVIEW_MD" ]; then
		entries="$(count_entries "$REVIEW_MD")"
		if [ "$entries" -gt 0 ]; then
			dual="；另检出 review.md 建账 ${entries} 条（双痕迹并存，以 waived 计）"
		elif has_nofinding "$REVIEW_MD"; then
			dual="；另检出 review.md 审毕无发现声明（双痕迹并存，以 waived 计）"
		fi
	fi
	echo "✓ 通过：waived 显式豁免（proposal.md 头部 <!-- review: waived --> 声明）${dual}"
	exit 0
fi

# ② review.md 建账 / 审毕无发现声明
if [ -f "$REVIEW_MD" ]; then
	entries="$(count_entries "$REVIEW_MD")"
	if [ "$entries" -gt 0 ]; then
		echo "✓ 通过：review.md 建账 ${entries} 条（R-<N>【等级】+ 状态行格式合法）"
		exit 0
	fi
	if has_nofinding "$REVIEW_MD"; then
		echo "✓ 通过：审毕无发现（review.md 含「- 审毕无发现（审查范围：…）」显式声明，非空壳）"
		exit 0
	fi
	echo "✗ 未通过：review.md 存在但零合法条目/无发现声明（空壳文件——有文件不等于审过；"
	echo "  词表外等级/状态的条目不计入合法条目）"
	print_fix_paths
	exit 1
fi

# ③ 皆无 → 红
echo "✗ 未通过：缺 review.md 建账与 <!-- review: waived --> 豁免声明（零痕迹）"
print_fix_paths
exit 1
