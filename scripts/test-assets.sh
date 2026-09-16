#!/usr/bin/env bash
# test-assets.sh — capability 测试资产反向索引（test-case-design-standard 引入）
#
# 用法：bash scripts/test-assets.sh <capability>
#   <capability> 形如 scenario-trace-gate / case-first-testing（openspec/specs/ 下的目录名）
#
# 场景：新 change 改契约（MODIFIED/REMOVED Requirements）时回归走查（test-design.md 问句⓪）——
# 旧 test-cases.md 故事文档与 Scenario→测试文件映射躺在 archive 里无人知晓，本脚本反查。
#
# 三段输出（只读只判不猜，学 scenario-trace.sh / change-scope.sh 的机械无猜测）：
#   1. 主 specs 现状：openspec/specs/<cap>/spec.md 的 Requirement / Scenario 清单（现状节拍）
#   2. archive 历史：openspec/changes/archive/<date>-<name>/ 中 specs/<cap>/ 存在的 change，
#      并标注该 change 是否含 test-cases*.md（含 = 有完整故事用例文档）
#   3. 历史映射重建：上述 change 的 tasks.md 验证节（## N. 验证）里
#      | Scenario | 测试文件 | 表逐行提取（Scenario→测试文件反向索引）
#
# 退出码：0 正常（哪怕段2/段3为空也如实输出「无」）；1 capability 在主 specs 与 archive 均不存在。
# POSIX bash（Linux / macOS / WSL）可跑。不执行任何测试、不改任何文件。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CAP="${1:-}"
if [ -z "$CAP" ]; then
	echo "用法: bash scripts/test-assets.sh <capability>  （如 scenario-trace-gate）"
	exit 1
fi

SPEC_FILE="openspec/specs/${CAP}/spec.md"
ARCHIVE_DIR="openspec/changes/archive"

# ---------- 收集 archive 命中 change（specs/<cap>/spec.md 存在） ----------

hit_changes=()
if [ -d "$ARCHIVE_DIR" ]; then
	while IFS= read -r -d '' cap_spec; do
		# 归一化为 change 目录（archive/<date>-<name>）：spec.md → <cap> → specs → <change> 共剥三层
		change_dir="$(dirname "$(dirname "$(dirname "$cap_spec")")")"
		hit_changes+=("$change_dir")
	done < <(find "$ARCHIVE_DIR" -type f -path "*/specs/${CAP}/spec.md" -print0 2>/dev/null | sort -z)
fi

# capability 完全不存在 → 报错退出（不猜测、不出空段充数）
if [ ! -f "$SPEC_FILE" ] && [ "${#hit_changes[@]}" -eq 0 ]; then
	echo "✗ capability 无匹配：主 specs（openspec/specs/${CAP}/）与 archive（delta specs/${CAP}/）均不存在"
	echo "  提示：capability 名 = openspec/specs/ 下的目录名，可用 ls openspec/specs/ 查看全部"
	exit 1
fi

# ---------- 段 1：主 specs 现状节拍 ----------

echo "══ 1. 主 specs 现状（${SPEC_FILE}）══"
if [ -f "$SPEC_FILE" ]; then
	reqs=0
	scens=0
	while IFS= read -r line; do
		case "$line" in
			"### Requirement:"*)
				echo "  ${line#\#\#\# }"
				reqs=$((reqs + 1))
				;;
			"#### Scenario:"*)
				echo "      ${line#\#\#\#\# }"
				scens=$((scens + 1))
				;;
		esac
	done < "$SPEC_FILE"
	echo "  （${reqs} Requirements / ${scens} Scenarios）"
else
	echo "  （主 specs 不存在——该 capability 已无现行契约，仅存 archive 历史）"
fi

# ---------- 段 2：archive 命中 change（含 test-cases*.md 标注） ----------

echo ""
echo "══ 2. archive 历史 change（delta 含 specs/${CAP}/）══"
if [ "${#hit_changes[@]}" -eq 0 ]; then
	echo "  （无——该 capability 从未被历史 change 修改/新增过 delta）"
else
	for change_dir in "${hit_changes[@]}"; do
		name="$(basename "$change_dir")"
		if ls "${change_dir}"/test-cases*.md >/dev/null 2>&1; then
			echo "  ${name}  [含 test-cases*.md ✅ 完整故事用例文档]"
		else
			echo "  ${name}  [无 test-cases*.md]"
		fi
	done
fi

# ---------- 段 3：历史映射重建（tasks.md 验证节 | Scenario | 测试文件 | 表） ----------

echo ""
echo "══ 3. 历史 Scenario→测试文件映射重建（来自上述 change 的 tasks.md 验证节）══"
total_rows=0
if [ "${#hit_changes[@]}" -eq 0 ]; then
	echo "  （无历史 change，无映射可重建）"
else
	for change_dir in "${hit_changes[@]}"; do
		name="$(basename "$change_dir")"
		tasks_md="${change_dir}/tasks.md"
		[ -f "$tasks_md" ] || continue
		# 验证节（## N. 验证）内的映射表行：以 | 开头且含非表头内容
		rows=$(awk -v cname="$name" '
			/^## [0-9]+\. ?验证/ {in_verify=1; next}
			/^## / {in_verify=0}
			in_verify && /^\|/ && $0 !~ /^[[:space:]:|-]+$/ && $0 !~ /^\|[[:space:]]*Scenario[[:space:]]*\|/ {
				print "  [" cname "] " $0
			}
		' "$tasks_md")
		if [ -n "$rows" ]; then
			echo "$rows"
			total_rows=$((total_rows + $(printf '%s\n' "$rows" | wc -l)))
		fi
	done
	if [ "$total_rows" -eq 0 ]; then
		echo "  （历史 change 的验证节无映射表——早期 change 可能未按 scenario-trace 格式留映射）"
	else
		echo "  （共 ${total_rows} 行映射）"
	fi
fi

exit 0
