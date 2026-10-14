#!/usr/bin/env bash
# ui-verification-trace.sh — UI 验收痕迹归档对账门禁（add-ui-verification-trace-gate 引入）
#
# 用法：bash scripts/harness/ui-verification-trace.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 只读对账（不执行测试/构建、不修改仓库文件），UI 验收证据判定的单一事实源：
# spec-gate 检查⑥' 与 archive-readiness 聚合自查只认本脚本退出码，判定逻辑不在扩展内二次耦合。
#
# 判定优先级（proposal.md 头部机器可读注释区 ui-impact 声明分档）：
#   ⓪ .openspec.yaml 非 syntopica-ui schema（含缺失）→ 零检查退出码 0（fail-open，旧 change 不卡死）
#   ⓪' proposal 声明缺失/非法 → 同 fail-open 退出码 0（声明完整性归 ui-design-gate 前置门禁）
#   ① none 档：ui-design.md 含 `## …N/A`（或「不适用」）记录节；ui-design.md 头部 ui-impact
#      marker（若有）须与声明一致（none）
#   ② minor 档：ui-design.md Acceptance/验收 节映射须指名道姓——
#      · 组件测试（词表：组件测试/component/单测/单元测试）→ 节内写出仓库根相对测试文件路径
#        且该文件真实存在；
#      · agent-browser/人工（opencli 历史口径按人工验证同通道处理，有建账即过）→
#        `ui-verification/report.md` 含 ≥1 条合法判定条目；
#      · 两词表都无 / 关键词后无实物 → 红（裸关键词不构成验收证据）
#   ③ major 档：`ui-verification/` 目录 + report.md 存在 + ≥1 条合法判定条目 +
#      条目/清单点名截图文件真实存在于目录 + 未覆盖条目带原因
#
# report.md 建账格式契约（design D4，自 add-topic-key-event-markers 实战形态归纳，双形态合法）：
#   标题式：### A-<N> <场景标题>【pass|fail|未覆盖】（编号自 A-1 自增，判定词表封闭三值）
#           下一行（空行可穿插）为依据行
#   表格式（实战形态）：| A<N> <场景标题> | **pass|fail|未覆盖**（括号说明可选） | 依据列 |
#   · 未覆盖条目须带原因：判定标注（…）+ 依据合并后 ≥6 字（locale 无关字符计数，review H1；
#     占位符式依据如「待补」不算）
#   · 词表外判定（如「搞定」「基本可用」）→ 该条目不计入合法条目；全文件无其他合法条目则对账失败
#   · 编号 A-<N> 不校验连续性/自增（宽松执行，编号是给人看的锚点，review L3）
#   · 截图以文件名出现在条目/清单中即入实物对账；速记形态 `X.png / -Y.png`（Y 为 X 尾段替换）
#     按「目录内存在以该段结尾且前邻字符非字母数字的文件」兜底匹配；跨两层目录的路径提及
#     （如 ui-prototype/evidence/x.png，review M1）不算点名证据，不入对账
#
# 测试缝：UI_VERIFICATION_TRACE_REPO_ROOT 覆盖 minor 档测试文件存在性的仓库根
# （仅 smoke fixture 注入用；缺省为本脚本所在仓库根）。
#
# 退出码：0 通过（含 fail-open）；1 有失败（中文输出修复路径指引）/ 用法错误 / 目录不存在。
# POSIX bash（Linux / macOS / WSL）可跑。
# 格式约定详见 openspec/specs/ui-verification-trace-gate/spec.md（随 change 归档同步）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ]; then
	echo "用法: bash scripts/harness/ui-verification-trace.sh <change-dir>  （如 openspec/changes/<name>）"
	exit 1
fi
CHANGE_DIR="${CHANGE_DIR%/}"
if [ ! -d "$CHANGE_DIR" ]; then
	echo "✗ change 目录不存在: $CHANGE_DIR"
	exit 1
fi

PROPOSAL="$CHANGE_DIR/proposal.md"
DESIGN="$CHANGE_DIR/ui-design.md"
REPORT_ROOT="$CHANGE_DIR/ui-verification"
REPORT="$REPORT_ROOT/report.md"
SCHEMA_YAML="$CHANGE_DIR/.openspec.yaml"
# 测试缝（smoke 注入）：minor 档测试文件存在性判定的仓库根
PATH_ROOT="${UI_VERIFICATION_TRACE_REPO_ROOT:-$REPO_ROOT}"

# ---------- 格式契约正则（与 spec 词表封闭值同步） ----------

# 标题式条目：### A-<N> <非空标题>【pass|fail|未覆盖】（标注括号可选）
HEADING_RE='^### A-[0-9]+ .+【(pass|fail|未覆盖)】(（[^）]*）)?[[:space:]]*$'
# 组件测试词表（-i）与 agent-browser/人工词表（opencli 历史口径按人工同通道）
COMP_RE='组件测试|component|单测|单元测试'
MANUAL_RE='agent-browser|人工|opencli'
# minor 档测试文件路径 token（仓库根相对）
TESTPATH_RE='(front|backend-go)/[A-Za-z0-9][A-Za-z0-9._/-]*\.(test|spec)\.(ts|tsx|js|jsx|mjs|cts|mts)|[A-Za-z0-9][A-Za-z0-9._/-]*_test\.go'

trim() { local s="$1"; s="${s#"${s%%[![:space:]]*}"}"; s="${s%"${s##*[![:space:]]}"}"; printf '%s' "$s"; }

# locale 无关的 UTF-8 字符计数（review H1：bash ${#} 在 C locale 按字节计，CJK 阈值失效）——
# 转 UTF-32 后每码点恒 4 字节；iconv 缺席/非法输入按 0 处理（保守方向判无原因）
utf8_charlen() {
	local n
	n="$(printf '%s' "$1" | iconv -f UTF-8 -t UTF-32LE 2>/dev/null | wc -c)"
	echo $(( ${n:-0} / 4 ))
}

# proposal.md / ui-design.md 头部机器可读注释区：文件开头连续的 <!-- ... --> 行（空行可穿插），
# 遇首个其他内容行停止——与 review-trace.sh head_markers 同实现（同边界）
head_markers() {
	awk '
		/^[[:space:]]*<!--.*-->[[:space:]]*$/ { print; next }
		/^[[:space:]]*$/ { next }
		{ exit }
	' "$1" 2>/dev/null
}

# 从头部注释区提取首个合法 ui-impact 值（none|minor|major）；无 → 空串
declared_impact() {
	head_markers "$1" | sed -n 's/^<!--[[:space:]]*ui-impact[[:space:]]*:[[:space:]]*\(none\|minor\|major\)[[:space:]]*-->$/\1/p' | head -n 1
}

# 抽取某 `## ` 节正文（$2=标题 alias ERE，如 Acceptance|验收）：命中标题行起、下一 `## ` 止
extract_section() {
	awk -v al="$2" '
		$0 ~ "^##[[:space:]]+.*(" al ")" { inf=1; next }
		/^##[[:space:]]/ { inf=0 }
		inf { print }
	' "$1" 2>/dev/null
}

# ---------- 条目解析（双形态） ----------

N_LEGAL=0; N_PASS=0; N_FAIL=0; N_UNCOV=0
DROPPED_NOTES=""

# 未覆盖带原因：标注（…）+ 依据合并去空白后 ≥6 字
reason_ok() { # $1=annot $2=basis
	local joined="$(trim "${1:-} $(trim "${2:-}")")"
	joined="${joined// /}"
	[ "$(utf8_charlen "$joined")" -ge 6 ]
}

# 计一条合法/丢弃（$1=verdict $2=annot $3=basis $4=形态描述）
count_entry() {
	case "$1" in
		pass) N_PASS=$((N_PASS + 1)); N_LEGAL=$((N_LEGAL + 1)) ;;
		fail) N_FAIL=$((N_FAIL + 1)); N_LEGAL=$((N_LEGAL + 1)) ;;
		未覆盖)
			if reason_ok "$2" "$3"; then
				N_UNCOV=$((N_UNCOV + 1)); N_LEGAL=$((N_LEGAL + 1))
			else
				DROPPED_NOTES+="  ⚠ 未计入合法条目（${4}）：未覆盖条目未写明原因——依据须说明原因（如数据条件不满足/环境缺建议数据），不留悬空"$'\n'
			fi
			;;
	esac
}

# 表格行形态判定：| A<N> <标题> | **verdict**（标注可选） | 依据 |（≥3 列且依据列非空）
# 成功时设 TRV（verdict）/ TRA（annot）/ TRB（basis）
TRV=""; TRA=""; TRB=""
is_table_row() { # $1=line
	TRV=""; TRA=""; TRB=""
	local line="$1"
	[[ "$line" =~ ^[[:space:]]*\|.*\|[[:space:]]*$ ]] || return 1
	local cells
	IFS='|' read -ra cells <<< "$line"
	[ "${#cells[@]}" -ge 4 ] || return 1
	local c1 c2
	c1="$(trim "${cells[1]}")"
	c2="$(trim "${cells[2]}")"
	[[ "$c1" =~ ^A-?[0-9]+([[:space:]].+)?$ ]] || return 1
	[[ "$c2" =~ ^\*{0,2}(pass|fail|未覆盖)(（[^）]*）)?\*{0,2}$ ]] || return 1
	TRV="${BASH_REMATCH[1]}"
	TRA="${BASH_REMATCH[2]:-}"
	local i rest=""
	for ((i = 3; i < ${#cells[@]}; i++)); do rest+="${cells[i]} "; done
	TRB="$(trim "$rest")"
	[ -n "$TRB" ] || return 1
	return 0
}

# 扫描 report.md：标题式（### A-N【…】+ 依据行，空行穿插容忍）+ 表格式（即时判定）。
# 词表外判定行不匹配任何形态 → 自然不计入（全文件零合法 → 空壳红）
scan_report() { # $1=report.md
	local file="$1" line trimmed pending="" pending_annot="" basis
	while IFS= read -r line || [ -n "$line" ]; do
		trimmed="$(trim "$line")"
		if [ -z "$trimmed" ]; then
			continue # 空行：标题式 pending 保持（markdown 标题与依据行间惯例空行）
		fi
		if [[ "$line" =~ $HEADING_RE ]]; then
			if [ -n "$pending" ]; then
				DROPPED_NOTES+="  ⚠ 未计入合法条目（标题式）：条目缺紧随依据行"$'\n'
			fi
			pending="${BASH_REMATCH[1]}"
			pending_annot="${BASH_REMATCH[2]:-}"
			continue
		fi
		if is_table_row "$line"; then
			if [ -n "$pending" ]; then
				DROPPED_NOTES+="  ⚠ 未计入合法条目（标题式）：条目缺紧随依据行"$'\n'
				pending=""
			fi
			count_entry "$TRV" "$TRA" "$TRB" "表格式 ${trimmed%%|*}"
			continue
		fi
		if [ -n "$pending" ]; then
			case "$trimmed" in
				'#'*|'|'*)
					# 标题行/表格行不作依据行（review M3：防吞标题洗白缺依据条目），按缺依据丢弃后正常处理本行
					DROPPED_NOTES+="  ⚠ 未计入合法条目（标题式）：条目缺紧随依据行"$'\n'
					pending=""; pending_annot=""
					;;
			esac
		fi
		if [ -n "$pending" ]; then
			count_entry "$pending" "$pending_annot" "$trimmed" "标题式"
			pending=""
			pending_annot=""
			continue
		fi
		# A-N 编号条目但词表外判定/缺【】标注 → 显式丢弃提示（review M3：不静默消失）
		if [[ "$trimmed" =~ ^###[[:space:]]+A-?[0-9]+ ]]; then
			DROPPED_NOTES+="  ⚠ 未计入合法条目：词表外判定或缺【pass|fail|未覆盖】标注——${trimmed}"$'\n'
		fi
	done < "$file"
	if [ -n "$pending" ]; then
		DROPPED_NOTES+="  ⚠ 未计入合法条目（标题式）：条目缺紧随依据行"$'\n'
	fi
}

# ---------- 截图实物对账 ----------

# 提取 report.md 点名的截图文件名段（token 按 / 拆段，取带图片扩展名的段，去重）。
# 跨两层以上目录的路径提及（如 ui-prototype/evidence/x.png，review M1）不算点名证据，整段跳过
shot_tokens() { # $1=report.md
	grep -oE '[A-Za-z0-9][A-Za-z0-9._/-]*\.(png|jpg|jpeg|webp|gif)' "$1" 2>/dev/null |
		awk -F/ '{ if (NF > 2) next; for (i = 1; i <= NF; i++) if ($i ~ /\.(png|jpg|jpeg|webp|gif)$/) print $i }' |
		sort -u
}

# 段命中：精确文件存在，或目录内存在「以该段结尾」的兜底文件——
#   · 段以 '-' 开头（速记尾段，普通文件名不以杠开头）→ 直接尾匹配；
#   · 无杠段 → 仅当前邻字符非字母数字时命中（不放松到任意子串，保住「点名不存在图」的牙齿）
shot_exists() { # $1=段 $2=目录
	local tok="$1" dir="$2" f base head
	[ -f "$dir/$tok" ] && return 0
	for f in "$dir"/*; do
		[ -f "$f" ] || continue
		base="${f##*/}"
		case "$base" in
			*"$tok")
				head="${base%"$tok"}"
				if [ -z "$head" ] || [[ "$tok" == -* ]]; then return 0; fi
				if [[ "$head" =~ [^A-Za-z0-9]$ ]]; then return 0; fi
				;;
		esac
	done
	return 1
}

# 对账全部点名截图：结果设进全局 N_MISS / SHOTS_LINE（不用命令替换——子 shell 变量传不出）
N_MISS=0
SHOTS_LINE=""
check_shots() { # $1=report.md $2=目录
	N_MISS=0
	local tok missing="" total=0
	while IFS= read -r tok; do
		[ -n "$tok" ] || continue
		total=$((total + 1))
		if ! shot_exists "$tok" "$2"; then
			missing+="$tok、"
			N_MISS=$((N_MISS + 1))
		fi
	done < <(shot_tokens "$1")
	if [ "$N_MISS" -gt 0 ]; then
		SHOTS_LINE="缺失截图清单：${missing%、}"
	else
		SHOTS_LINE="点名截图 ${total} 张全部在盘（含速记形态兜底匹配）"
	fi
}

# ---------- 修复指引 ----------

fix_major_zero() {
	cat <<'FIXEOF'
  修复路径（二选一）：
  1. 补做验收并建账：用 agent-browser（无头浏览器驱动真实浏览器）走 ui-design.md 主故事，
     逐场景判定记入 ui-verification/report.md——
       ### A-1 <场景标题>【pass】
       <依据行：断言依据，可点名截图文件名>
     （判定词表封闭三值 pass|fail|未覆盖；未覆盖须写原因；点名截图落同目录）
  2. 显式降档：本 change 实际不构成 major UI 改动时，改 proposal.md 头部声明为
     <!-- ui-impact: minor -->（或 none），在 proposal 正文写明降档理由后按新档重对账
FIXEOF
}

fix_shell_report() {
	cat <<'FIXEOF'
  建账格式契约（判定词表封闭三值 pass|fail|未覆盖；词表外判定/缺依据行的条目不计入合法条目）：
    标题式：### A-1 <场景标题>【pass】
            <依据行：断言依据，可点名截图文件名>
    表格式：| A1 <场景标题> | **pass** | <依据列> |
  未覆盖条目的依据须含原因说明（≥6 字，占位符如「待补」不算）；条目/清单点名的截图文件须真实存在于 ui-verification/ 目录。
FIXEOF
}

fix_minor_bare() {
	cat <<'FIXEOF'
  修复路径（按实际验收方式二选一，写进 ui-design.md Acceptance/验收 节）：
  1. 组件测试映射：写出真实存在的测试文件路径（仓库根相对，
     如 front/app/components/X.test.ts），文件必须已在仓库中
  2. agent-browser/人工验证：建 ui-verification/report.md 记 ≥1 条合法判定条目
     （opencli 历史口径按人工验证同通道处理，有建账即过，不强制改写历史文本）
  仅出现「组件测试/人工」等关键词字样不构成验收证据——须指名道姓挂实物。
  （注意：词表按字面命中，否定句如「无需人工验证」也会触发该通道，措辞请避开词表词）
FIXEOF
}

# ---------- 判定主流程 ----------

# ⓪ 旧 schema / 未声明 schema → 零检查（fail-open，与 spec-gate ⑥' 既有语义对齐）
if ! grep -Eq '^schema:[[:space:]]*syntopica-ui[[:space:]]*$' "$SCHEMA_YAML" 2>/dev/null; then
	echo "✓ 通过（fail-open）：非 syntopica-ui schema（或未声明）→ 旧 change 零检查不硬阻断"
	exit 0
fi

# ⓪' proposal 声明缺失/非法 → fail-open（声明完整性归 ui-design-gate 前置门禁管）
decl="$(declared_impact "$PROPOSAL")"
if [ -z "$decl" ]; then
	echo "✓ 通过（fail-open）：proposal 头部无合法 ui-impact 声明，本脚本不判定（声明缺失由 ui-design-gate 门禁负责）"
	exit 0
fi

case "$decl" in
none)
	# ① none 档：N/A 记录节 + 头部 marker 一致性
	if [ ! -f "$DESIGN" ]; then
		echo "✗ 未通过：none 档缺 ui-design.md，N/A 记录节无从对账（新 schema change 该文件必经）"
		exit 1
	fi
	if ! grep -Eq '^##[[:space:]]+.*(N/A|不适用)' "$DESIGN"; then
		echo "✗ 未通过：none 档 ui-design.md 缺 \`## N/A Reason\`（或「不适用」）记录节——"
		echo "  修复：ui-design.md 补一节说明为何无需 UI 原型与视觉验收（豁免理由）"
		exit 1
	fi
	dim="$(declared_impact "$DESIGN")"
	# marker 缺失（dim 空）在此宽容：声明完整性由 lib/ui-design-gate 严格判定（缺失即红），
	# 本脚本只对账「存在但不一致」——双门禁合并后语义闭合（review L1 口径说明）
	if [ -n "$dim" ] && [ "$dim" != "none" ]; then
		echo "✗ 未通过：ui-design.md 头部 ui-impact($dim) 与 proposal 声明(none)不一致——"
		echo "  修复：以 proposal 声明为准改 ui-design.md marker，或修正 proposal 声明后重对账"
		exit 1
	fi
	echo "✓ 通过：none 档 N/A 一致性（ui-design.md 含 N/A 记录节，无影响等级不一致）"
	exit 0
	;;
minor)
	# ② minor 档：验收映射指名道姓
	if [ ! -f "$DESIGN" ]; then
		echo "✗ 未通过：minor 档缺 ui-design.md，验收映射无从对账（新 schema change 该文件必经）"
		exit 1
	fi
	acc="$(extract_section "$DESIGN" 'Acceptance|验收')"
	comp_hit=0; manual_hit=0; minor_fail=0
	if printf '%s' "$acc" | grep -Eqi "$COMP_RE"; then comp_hit=1; fi
	if printf '%s' "$acc" | grep -Eqi "$MANUAL_RE"; then manual_hit=1; fi
	if [ "$comp_hit" -eq 0 ] && [ "$manual_hit" -eq 0 ]; then
		echo "✗ 未通过：minor 档验收映射未指名道姓（验收节须映射到 组件测试 或 agent-browser/人工验证 至少其一）"
		fix_minor_bare
		exit 1
	fi
	if [ "$comp_hit" -eq 1 ]; then
		tp="$(printf '%s' "$acc" | grep -oE "$TESTPATH_RE" | head -n 1)"
		if [ -z "$tp" ]; then
			echo "✗ 未通过：minor 档映射到组件测试，但验收节未写出测试文件路径（裸关键词）——"
			fix_minor_bare
			exit 1
		fi
		if [ ! -f "$PATH_ROOT/$tp" ]; then
			echo "✗ 未通过：minor 档映射的测试文件不存在：$tp（须为仓库根相对路径且真实存在）——"
			echo "  修复：先落盘该测试文件，或把验收节路径改为已存在的测试文件"
			minor_fail=1
		else
			echo "✓ 组件测试映射：$tp（文件在盘）"
		fi
	fi
	if [ "$manual_hit" -eq 1 ]; then
		if [ ! -f "$REPORT" ]; then
			echo "✗ 未通过：minor 档映射到 agent-browser/人工验证，但缺 ui-verification/report.md 建账——"
			fix_minor_bare
			exit 1
		fi
		scan_report "$REPORT"
		printf '%s' "$DROPPED_NOTES"
		if [ "$N_LEGAL" -eq 0 ]; then
			echo "✗ 未通过：ui-verification/report.md 零合法判定条目（空壳不构成人工验证证据）——"
			fix_shell_report
			exit 1
		fi
		printf '✓ 人工验证建账：%s 条合法判定条目（pass %s / fail %s / 未覆盖 %s）\n' "$N_LEGAL" "$N_PASS" "$N_FAIL" "$N_UNCOV"
	fi
	if [ "$minor_fail" -eq 1 ]; then exit 1; fi
	printf '✓ 通过：minor 档验收映射指名道姓（组件测试→测试文件在盘 / 人工→建账在案）\n'
	exit 0
	;;
esac

# ③ major 档：实物对账
if [ ! -d "$REPORT_ROOT" ] || [ ! -f "$REPORT" ]; then
	echo "✗ 未通过：major 档缺 ui-verification/ 验收目录或 report.md（零痕迹——写一句验收字样不构成证据）"
	fix_major_zero
	exit 1
fi

scan_report "$REPORT"
printf '%s' "$DROPPED_NOTES"
if [ "$N_LEGAL" -eq 0 ]; then
	echo "✗ 未通过：ui-verification/report.md 存在但零合法判定条目（空壳文件——有文件不等于验过）"
	fix_shell_report
	exit 1
fi

check_shots "$REPORT" "$REPORT_ROOT"
if [ "$N_MISS" -gt 0 ]; then
	echo "✗ 未通过：判定条目/清单点名截图 ${N_MISS} 张缺失（勾了勾了、图不存在 = 空转）"
	echo "  $SHOTS_LINE"
	echo "  修复：补齐截图落 $REPORT_ROOT/，或删除条目中对不存在文件的点名引用"
	exit 1
fi

printf '✓ 通过：report.md 建账 %s 条（pass %s / fail %s / 未覆盖 %s），%s\n' "$N_LEGAL" "$N_PASS" "$N_FAIL" "$N_UNCOV" "$SHOTS_LINE"
exit 0
