#!/usr/bin/env bash
# scenario-trace.sh — Scenario→测试文件映射的归档对账门禁（scenario-test-mapping-gate 引入）
#
# 用法：bash scripts/scenario-trace.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 做三件事（只判不跑——不执行任何测试/编译命令）：
#   1. 解析 <change-dir>/specs/**/*.md 的 delta：ADDED / MODIFIED / RENAMED Requirements
#      节下的 #### Scenario: 标题计入对账；REMOVED 节忽略（删除的场景不再需要测试保障）
#   2. 在 tasks.md 的「## N. 验证」节提取表头为 | Scenario | 测试文件 | 的映射表，
#      逐标题逐字匹配（仅裁首尾空白；跨 spec 重名一行覆盖全部同名实例）
#   3. 映射的测试文件按仓库根相对路径逐一做存在性校验；单元格以「人工」开头视为
#      合法映射（留痕，不做文件校验）；多路径以空白或逗号分隔
#
# 退出码：0 通过；1 有失败（中文输出逐条列原因）。WSL bash 可跑。
# 格式约定详见 openspec/specs/scenario-trace-gate/spec.md（随 change 归档同步）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ]; then
	echo "用法: bash scripts/scenario-trace.sh <change-dir>  （如 openspec/changes/<name>）"
	exit 1
fi
if [ ! -d "$CHANGE_DIR" ]; then
	echo "✗ change 目录不存在: $CHANGE_DIR"
	exit 1
fi

SPECS_DIR="$CHANGE_DIR/specs"
TASKS_MD="$CHANGE_DIR/tasks.md"

# ---------- 小工具 ----------

trim() {
	local s="$1"
	s="${s#"${s%%[![:space:]]*}"}"
	s="${s%"${s##*[![:space:]]}"}"
	printf '%s' "$s"
}

fails=()
add_fail() { fails+=("$1"); }

# ---------- 1. 收集待对账 Scenario（文件 + 标题，重名不去重） ----------

scen_files=()
scen_titles=()
if [ -d "$SPECS_DIR" ]; then
	while IFS= read -r spec_file; do
		in_scope=0
		while IFS= read -r line || [ -n "$line" ]; do
			case "$line" in
				"## ADDED Requirements" | "## MODIFIED Requirements" | "## RENAMED Requirements")
					in_scope=1
					;;
				"## "*)
					in_scope=0
					;;
				"#### Scenario:"*)
					if [ "$in_scope" -eq 1 ]; then
						title="$(trim "${line#*Scenario:}")"
						scen_files+=("${spec_file#./}")
						scen_titles+=("${title:-(空标题)}")
					fi
					;;
			esac
		done < "$spec_file"
	done < <(find "$SPECS_DIR" -type f -name '*.md' | sort)
fi

total=${#scen_titles[@]}

# 无 delta Scenario（无 specs/ 目录、纯 REMOVED 或 skip_specs）→ 直接过
if [ "$total" -eq 0 ]; then
	echo "✓ 通过：无待对账 Scenario（specs 不存在或无 ADDED/MODIFIED/RENAMED 场景），无需映射表"
	exit 0
fi

# ---------- 2. 提取 tasks.md 验证节的映射表 ----------

if [ ! -f "$TASKS_MD" ]; then
	echo "✗ 未通过（$total 个待对账 Scenario，$total 项失败）："
	echo "  - tasks.md 不存在: $TASKS_MD"
	exit 1
fi

# 验证节 = 「## N. 验证」标题行到下一个 ## 级标题之间
section="$(awk '/^## [0-9]+\. *验证/{f=1;next} /^## /{f=0} f' "$TASKS_MD")"
if [ -z "$section" ]; then
	echo "✗ 未通过（$total 个待对账 Scenario，$total 项失败）："
	echo "  - tasks.md 缺少「## N. 验证」节（标题须为 ## <数字>. 验证）"
	exit 1
fi

# 映射表行：表头（空白折叠后 == |Scenario|测试文件|）之后、到空行/非表格行为止；
# 节内允许多张同表头映射表（行累计）。分隔行（只含 - : |）跳过。
map_rows="$(printf '%s\n' "$section" | awk '
	{
		t = $0; gsub(/[[:space:]]+/, "", t)
		if (t == "|Scenario|测试文件|") { intab = 1; next }
		if (!intab) next
		if (t == "") { intab = 0; next }
		if ($0 ~ /^[[:space:]]*\|/) {
			sep = t; gsub(/[-:|]/, "", sep)
			if (sep == "") next
			print $0
		} else {
			intab = 0
		}
	}'
)"
if [ -z "$map_rows" ]; then
	echo "✗ 未通过（$total 个待对账 Scenario，$total 项失败）："
	echo "  - 验证节内未找到映射表（表头须为 | Scenario | 测试文件 |）"
	exit 1
fi

# ---------- 3. 逐 Scenario 对账 ----------

manual_count=0
auto_count=0

for i in $(seq 0 $((total - 1))); do
	title="${scen_titles[$i]}"
	where="${scen_files[$i]}"

	# 找到首个标题逐字相等的映射行
	file_cell=""
	while IFS= read -r row; do
		row_trim="$(trim "$row")"
		body="${row_trim#|}"
		body="${body%|}"
		row_title="$(trim "${body%%|*}")"
		if [ "$row_title" = "$title" ]; then
			file_cell="$(trim "${body#*|}")"
			# markdown 单元格常用反引号包裹代码路径，剥掉再校验（宽容解析，
			# spec 约定的是仓库根相对路径本身，不限制 markdown 包裹）
			file_cell="${file_cell//\`/}"
			file_cell="$(trim "$file_cell")"
			break
		fi
	done <<< "$map_rows"

	if [ -z "$file_cell" ]; then
		add_fail "未映射: 「$title」（$where）"
		continue
	fi

	# 「人工」前缀 → 合法映射，留痕放行
	if [[ "$file_cell" == 人工* ]]; then
		manual_count=$((manual_count + 1))
		continue
	fi

	# 文件存在性：空白/逗号分隔多路径，逐一校验（仓库根相对路径）
	cell_paths="$(printf '%s' "$file_cell" | tr ',' ' ')"
	bad_paths=()
	while IFS= read -r p; do
		[ -n "$p" ] || continue
		if [ ! -e "$p" ]; then
			bad_paths+=("$p")
		fi
	done <<< "$(printf '%s\n' $cell_paths)"
	if [ "${#bad_paths[@]}" -gt 0 ]; then
		for p in "${bad_paths[@]}"; do
			add_fail "映射文件不存在: 「$title」→ $p（$where）"
		done
		continue
	fi
	auto_count=$((auto_count + 1))
done

# ---------- 4. 汇总输出 ----------

if [ "${#fails[@]}" -gt 0 ]; then
	echo "✗ 未通过（$total 个待对账 Scenario，${#fails[@]} 项失败）："
	for f in "${fails[@]}"; do
		echo "  - $f"
	done
	exit 1
fi

echo "✓ 通过：$total 个 Scenario 映射齐全（自动测试 $auto_count / 人工留痕 $manual_count）"
exit 0
