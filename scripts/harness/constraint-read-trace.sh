#!/usr/bin/env bash
# constraint-read-trace.sh — 约束文档读取记账的归档对账门禁
# （openspec change stage-driven-constraint-injection D6；spec「约束文档读取记账与归档对账」）
#
# 用法：bash scripts/harness/constraint-read-trace.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 判定（只判不跑——不执行任何测试/编译命令、不修改仓库文件、事实库只读打开）：
#   ① legacy 豁免：change 目录无 stage.md（或 stage.md 无合法 stage marker）→ 退出码 0，
#      输出明示豁免（与扩展三态一致：missing/invalid 均按 legacy 降级，MUST NOT 阻断归档）
#   ② 域集合 = proposal.md 头部 `<!-- constraint-domains: ... -->` 与 stage.md 头部的同名
#      marker 的**并集**（解析语义与扩展一致：可多行合并、多个块合并、按 , 与换行切分、
#      首次出现去重；未知域名宽容忽略——合法域名 = docs/reference/flow/*.md basename）
#   ③ 无有效域声明 → 退出码 0（无声明即无义务，明示输出）
#   ④ 逐域核对事实库 .pi/harness/events.db 的 constraint.read 事件：按**路径 + 归属**
#      判定（json_extract(payload,'$.path') = docs/reference/flow/<域>.md 且 change 列 =
#      本 change——他会话/未绑定会话的读取不算本 change 证据，review R-2）；读取时间须
#      落在该 change 的**实现期窗**内（起点 = 首条 implementation 族 stage.set（implement/
#      verify/archive）的 ts 与生命周期起点（created: 日期 UTC 零点 vs 目录 mtime 取更早者）
#      中**更晚**者——规划期的读取不算证据；无 implementation 族记录时回退生命周期起点，
#      宽松避免误红；终点 = 当前时刻）。写入侧保证只有成功 read（tool_execution_end 且
#      非 error）才记账，失败/局部读取语义见扩展（review R-4）
#   ⑤ 全部读过 → 退出码 0（一行绿）；任一未读 → 退出码 1，逐条列出「域名 + 文档路径 +
#      取数指引 bash scripts/harness/constraint-digest.sh --brief --domains <域>」
#
# 退出码：0 通过 / 豁免；1 有未读域（或事实库不可用无法取证）；2 用法错误 / change 目录不存在。
# POSIX bash（Linux / macOS / WSL）可跑。
#
# 测试缝（仅 smoke 用，判定逻辑与真实运行零差异）：
#   CONSTRAINT_READ_TRACE_DB       事实库路径（缺省 <repo>/.pi/harness/events.db）
#   CONSTRAINT_READ_TRACE_FLOW_DIR 业务域目录（缺省 <repo>/docs/reference/flow）
set -u

# 自身路径（解符号链接——smoke 用 symlink 旁置被测脚本时仓库根仍解析到真仓库）
SELF="${BASH_SOURCE[0]}"
while [ -L "$SELF" ]; do
	link="$(readlink "$SELF")"
	case "$link" in
		/*) SELF="$link" ;;
		*) SELF="$(dirname "$SELF")/$link" ;;
	esac
done
REPO_ROOT="$(cd "$(dirname "$SELF")/../.." && pwd)"
cd "$REPO_ROOT"

FLOW_DIR="${CONSTRAINT_READ_TRACE_FLOW_DIR:-$REPO_ROOT/docs/reference/flow}"
DB_PATH="${CONSTRAINT_READ_TRACE_DB:-$REPO_ROOT/.pi/harness/events.db}"

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ]; then
	echo "用法: bash scripts/harness/constraint-read-trace.sh <change-dir>  （如 openspec/changes/<name>）"
	exit 2
fi
if [ ! -d "$CHANGE_DIR" ]; then
	echo "✗ change 目录不存在: $CHANGE_DIR"
	exit 2
fi

STAGE_MD="$CHANGE_DIR/stage.md"
PROPOSAL_MD="$CHANGE_DIR/proposal.md"
CHANGE_NAME="$(basename "${CHANGE_DIR%/}")"

# ---------- 小工具 ----------

# 空格分隔的列表 → 逗号分隔（展示用）
join_commas() {
	local v out=""
	for v in $1; do
		if [ -z "$out" ]; then out="$v"; else out="$out, $v"; fi
	done
	printf '%s' "$out"
}

trim() {
	local s="$1"
	s="${s#"${s%%[![:space:]]*}"}"
	s="${s%"${s##*[![:space:]]}"}"
	printf '%s' "$s"
}

# 文件 mtime → epoch 秒（GNU stat 优先，BSD stat 回退；非数字输出视为取不到）
mtime_of() {
	local p="$1" m=""
	m="$(stat -c %Y "$p" 2>/dev/null)" || m=""
	case "$m" in '' | *[!0-9]*) m="" ;; esac
	if [ -z "$m" ]; then
		m="$(stat -f %m "$p" 2>/dev/null)" || m=""
		case "$m" in '' | *[!0-9]*) m="" ;; esac
	fi
	[ -n "$m" ] || return 1
	printf '%s' "$m"
}

# epoch 秒 → ISO 8601 UTC（毫秒位补 .000，与事实库 ts 同形，便于字典序比较）
iso_of_epoch() {
	local e="$1"
	date -u -d "@$e" +%Y-%m-%dT%H:%M:%S.000Z 2>/dev/null ||
		date -u -r "$e" +%Y-%m-%dT%H:%M:%S.000Z 2>/dev/null
}

# ---------- ① legacy 判定（扩展三态一致：missing / invalid → legacy → 豁免） ----------

STAGE_RE='<!--[[:space:]]*stage[[:space:]]*:[[:space:]]*([^>]*)-->'
stage_value=""
legacy_reason=""
if [ ! -f "$STAGE_MD" ]; then
	legacy_reason="无 stage.md"
else
	stage_text="$(tr '\n' ' ' <"$STAGE_MD")"
	stage_count=0
	stage_raw=""
	rest="$stage_text"
	while [[ "$rest" =~ $STAGE_RE ]]; do
		stage_count=$((stage_count + 1))
		stage_raw="${BASH_REMATCH[1]}"
		rest="${rest#*"${BASH_REMATCH[0]}"}"
	done
	stage_raw="$(trim "$stage_raw")"
	if [ "$stage_count" -eq 0 ]; then
		legacy_reason="stage.md 无 stage marker"
	elif [ "$stage_count" -gt 1 ]; then
		legacy_reason="stage.md 的 stage marker 重复（${stage_count} 条）"
	else
		case "$stage_raw" in
			explore | propose | design | implement | verify | archive) stage_value="$stage_raw" ;;
			*) legacy_reason="stage.md 的 stage 值非法（${stage_raw}）" ;;
		esac
	fi
fi

if [ -n "$legacy_reason" ]; then
	echo "✓ 豁免：legacy change（$legacy_reason）——约束文档读取对账不适用（不阻断归档）"
	exit 0
fi

# ---------- ② 域声明解析（proposal.md ∪ stage.md，语义与扩展一致） ----------

# 从文件里抽 `<!-- constraint-domains: ... -->` 的域名（逐行输出，未校验白名单）：
# 与扩展的 /<!--[\s]*constraint-domains:([\s\S]*?)-->/ 同语义——「<!--」与关键字之间只允许
# 空白、内容取到最近的「-->」、内容按 , 与换行切分。
extract_domains() {
	[ -f "$1" ] || return 0
	awk '
		function isws(ch) { return (ch == " " || ch == "\t" || ch == "\n" || ch == "\r") }
		{ buf = buf $0 "\n" }
		END {
			n = length(buf); i = 1
			while (i <= n) {
				p = index(substr(buf, i), "constraint-domains:")
				if (p == 0) break
				c = i + p - 1                      # 「constraint-domains:」起始位置
				j = c - 1
				while (j >= 1 && isws(substr(buf, j, 1))) j--
				if (j >= 4 && substr(buf, j - 3, 4) == "<!--") {
					s = c + length("constraint-domains:")
					tail = substr(buf, s)
					e = index(tail, "-->")
					if (e > 0) {
						content = substr(tail, 1, e - 1)
						gsub(/,/, "\n", content)
						m = split(content, arr, "\n")
						for (k = 1; k <= m; k++) {
							t = arr[k]
							while (length(t) > 0 && isws(substr(t, 1, 1))) t = substr(t, 2)
							while (length(t) > 0 && isws(substr(t, length(t), 1))) t = substr(t, 1, length(t) - 1)
							if (t != "") print t
						}
						i = s + e - 1 + 3
						continue
					}
				}
				i = c + 1
			}
		}
	' "$1"
}

# 合法业务域名（唯一真相源 = docs/reference/flow/*.md basename，README 除外）
VALID_DOMAINS=" "
for f in "$FLOW_DIR"/*.md; do
	[ -e "$f" ] || continue
	b="${f##*/}"
	[ "$b" = "README.md" ] && continue
	VALID_DOMAINS="$VALID_DOMAINS${b%.md} "
done

DOMAINS=""
UNKNOWN_DOMAINS=""
SEEN_DOMAINS=" "
add_domains() {
	local d
	while IFS= read -r d; do
		[ -n "$d" ] || continue
		case "$SEEN_DOMAINS" in *" $d "*) continue ;; esac
		SEEN_DOMAINS="$SEEN_DOMAINS$d "
		case "$VALID_DOMAINS" in
			*" $d "*) DOMAINS="$DOMAINS $d" ;;
			*) UNKNOWN_DOMAINS="$UNKNOWN_DOMAINS $d" ;;
		esac
	done < <(extract_domains "$1")
}

add_domains "$PROPOSAL_MD"
add_domains "$STAGE_MD"

DOMAIN_LIST="$(trim "$DOMAINS")"
UNKNOWN_LIST="$(trim "$UNKNOWN_DOMAINS")"
UNKNOWN_NOTE=""
if [ -n "$UNKNOWN_LIST" ]; then
	UNKNOWN_NOTE="（宽容忽略未知域名：$UNKNOWN_LIST）"
fi

# ---------- ③ 无域声明 → 无义务 ----------

if [ -z "$DOMAIN_LIST" ]; then
	echo "✓ 通过：无业务域声明（proposal.md / stage.md 均无有效 constraint-domains）——无对账义务${UNKNOWN_NOTE}"
	exit 0
fi

# ---------- ④ 生命周期基准窗（⑤a 会再收敛到实现期起点） ----------

END_ISO="$(date -u +%Y-%m-%dT%H:%M:%S.000Z)"
START_ISO=""

created_raw=""
if [ -f "$CHANGE_DIR/.openspec.yaml" ]; then
	created_raw="$(awk '
		/^[[:space:]]*created[[:space:]]*:/ {
			line = $0
			sub(/^[^:]*:[[:space:]]*/, "", line)
			gsub(/["'\'']/, "", line)
			print line
			exit
		}
	' "$CHANGE_DIR/.openspec.yaml" 2>/dev/null)"
fi
if [[ "$created_raw" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
	START_ISO="${created_raw}T00:00:00.000Z"
fi
mtime_iso=""
mtime_epoch="$(mtime_of "$CHANGE_DIR" 2>/dev/null)" || mtime_epoch=""
if [ -n "$mtime_epoch" ]; then
	mtime_iso="$(iso_of_epoch "$mtime_epoch")"
	# 起点取更早者（宽松窗口，避免误红）：created: 早于目录 mtime 时以 created 为准
	if [ -z "$START_ISO" ] || [[ "$mtime_iso" < "$START_ISO" ]]; then
		START_ISO="$mtime_iso"
	fi
fi
if [ -z "$START_ISO" ]; then
	START_ISO="1970-01-01T00:00:00.000Z"
fi

# ---------- ⑤ 读取证据（事实库只读；归属 + 实现期窗） ----------

if ! command -v sqlite3 >/dev/null 2>&1; then
	echo "✗ 未通过：sqlite3 不可用——无法查询事实库取证（$DB_PATH）"
	exit 1
fi
if [ ! -f "$DB_PATH" ]; then
	echo "✗ 未通过：事实库不存在——无法取证（$DB_PATH）"
	exit 1
fi

# ⑤a 实现期起点（review R-2）：取本 change 首条 implementation 族 stage.set
# （implement/verify/archive）的 ts——规划期（explore/propose/design）的读取不算
# 实现期证据；无记录时回退生命周期起点（宽松，避免 stage.md 后补的 change 误红）。
IMPL_START=""
impl_out="$(sqlite3 "file:$DB_PATH?mode=ro" "SELECT MIN(ts) FROM events WHERE kind='stage.set' AND change='$CHANGE_NAME' AND json_extract(payload, '\$.stage') IN ('implement','verify','archive');" 2>/dev/null)" || impl_out=""
if [ -n "$(trim "$impl_out")" ]; then
	IMPL_START="$(trim "$impl_out")"
	if [[ "$IMPL_START" > "$START_ISO" ]]; then
		START_ISO="$IMPL_START"
	fi
fi

# ⑤b 证据只认归属本 change 的成功读取（review R-2/R-4）：change 列 = 本 change；
# 失败/局部读取语义由写入侧保证（只在 tool_execution_end 且非 error 时记账）。
SQL="SELECT json_extract(payload, '\$.path') FROM events WHERE kind='constraint.read' AND change='$CHANGE_NAME' AND ts >= '$START_ISO' AND ts <= '$END_ISO';"
read_out="$(sqlite3 "file:$DB_PATH?mode=ro" "$SQL" 2>&1)"
read_rc=$?
if [ "$read_rc" -ne 0 ]; then
	echo "✗ 未通过：事实库查询失败（exit $read_rc）——无法取证（$DB_PATH）"
	printf '%s\n' "$read_out" | head -n 3 | sed 's/^/  /'
	exit 1
fi

READ_PATHS="|"
while IFS= read -r p; do
	[ -n "$p" ] || continue
	READ_PATHS="$READ_PATHS$p|"
done <<<"$read_out"

# ---------- ⑥ 逐域判定 ----------

unread=""
unread_count=0
total_count=0
for d in $DOMAIN_LIST; do
	total_count=$((total_count + 1))
	doc="docs/reference/flow/$d.md"
	case "$READ_PATHS" in
		*"|$doc|"*) : ;; # 读过
		*)
			unread_count=$((unread_count + 1))
			unread="$unread$d "
			;;
	esac
done

if [ "$unread_count" -eq 0 ]; then
	echo "✓ 通过：$total_count 个声明域的约束文档均在该 change（stage=$stage_value）的实现期窗内被读过（$(join_commas "$DOMAIN_LIST")）${UNKNOWN_NOTE}"
	exit 0
fi

echo "✗ 未通过：$unread_count/$total_count 个声明域的约束文档未在该 change（stage=$stage_value）的实现期窗内被读过："
for d in $unread; do
	echo "  - 域 $d：未读到 docs/reference/flow/$d.md（归属本 change 的成功 read）"
	echo "    取数指引：bash scripts/harness/constraint-digest.sh --brief --domains $d"
	echo "    读对应 flow 文档后重跑本脚本（成功整档/局部 read 均算读）"
done
echo "  时间窗：$START_ISO ~ $END_ISO（事实库 $DB_PATH，kind=constraint.read，归属 change=$CHANGE_NAME）"
echo "  未读域清单：$(join_commas "$(trim "$unread")")"
exit 1
