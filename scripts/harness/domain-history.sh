#!/usr/bin/env bash
# domain-history.sh — 按业务域反查该域的历史契约（spec-domain-index，add-spec-domain-index）
#
# 用法：
#   bash scripts/harness/domain-history.sh <域名>       # 反查该域
#   bash scripts/harness/domain-history.sh --list       # 列出可用域名
#
# 数据来源（两跳推导，不回填历史 change）：
#   openspec/specs/*/spec.md 头部 <!-- domain: a, b -->  → capability 的域归属
#   openspec/changes/archive/*/specs/<cap>/spec.md       → 历史 change 碰过哪些 capability
#   历史 change 的域 = 其 delta capability 的域并集
#
# 输出五段（契约见 openspec/specs/spec-domain-index/spec.md）：
#   1 本域 main spec 清单  2 历史 change 时间线  3 flow 红线
#   4 可能冲突（历史 REMOVED）  5 必须沿用（历史 MODIFIED）
#   第 4/5 段是**提示性罗列，不做判断**——冲突判定仍由人 / review 完成。
#
# 退出码：恒 0（无命中不是错误，嵌进自动化链路时不打断）。唯一例外是用例缺失的参数错误也仍返回 0，
# 只打印用法——本脚本定位是「查得到就查，查不到不挡路」。
set -u

ROOT="${DOMAIN_HISTORY_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
SPECS_DIR="$ROOT/openspec/specs"
ARCHIVE_DIR="$ROOT/openspec/changes/archive"
DIGEST="$ROOT/scripts/harness/constraint-digest.sh"

# 词表单点：加域名只改这一行（值来源 docs/reference/flow/*.md basename + 5 个工具域）
BUSINESS_DOMAINS="semantic-board topic-graph daily-report content-enrichment data-enrichment ai-summary discovery reading scheduler research-data-sources"
TOOL_DOMAINS="harness ui dev api db"
ALL_DOMAINS="$BUSINESS_DOMAINS $TOOL_DOMAINS"

usage() {
	cat <<'EOF'
用法:
  bash scripts/harness/domain-history.sh <域名>    # 反查该域历史契约
  bash scripts/harness/domain-history.sh --list    # 列出可用域名
EOF
}

list_domains() {
	echo "业务域（= docs/reference/flow/*.md basename）："
	for d in $BUSINESS_DOMAINS; do echo "  $d"; done
	echo "工具域："
	for d in $TOOL_DOMAINS; do echo "  $d"; done
}

in_list() {
	# $1 = 候选值, $2 = 空格分隔清单；精确匹配，防 semantic-board 命中 board
	local needle="$1" hay="$2" x
	for x in $hay; do [ "$x" = "$needle" ] && return 0; done
	return 1
}

# 读 spec 头部 8 行内的 <!-- domain: a, b -->，输出逗号分隔的原始值（可能为空）
spec_domain_raw() {
	sed -n '1,8{
		/<!--[[:space:]]*domain:/{
			s/.*domain:[[:space:]]*//
			s/-->.*//
			s/[[:space:]]*$//
			p
			q
		}
	}' "$1" 2>/dev/null
}

# 该 spec 是否属于目标域（多域写法按逗号切分后精确比较）
spec_in_domain() {
	local raw
	raw="$(spec_domain_raw "$1")"
	[ -z "$raw" ] && return 1
	local IFS=','
	local d
	for d in $raw; do
		d="$(echo "$d" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
		[ "$d" = "$2" ] && return 0
	done
	return 1
}

last_change_date() {
	git -C "$ROOT" log -1 --format=%ad --date=short -- "$1" 2>/dev/null
}

REQ_TITLES_BY_SECTION() {
	# $1 = delta spec 文件, $2 = ADDED|MODIFIED|REMOVED|RENAMED（节头大小写不敏感，R-8）
	awk -v want="$(printf '%s' "$2" | tr 'A-Z' 'a-z')" '
		/^## / { insec = (tolower($0) ~ ("^## " want " requirements")) ; next }
		insec && /^### Requirement:/ { sub(/^### Requirement:[[:space:]]*/, ""); print }
	' "$1" 2>/dev/null
}

DOMAIN="${1:-}"
if [ -z "$DOMAIN" ] || [ "$DOMAIN" = "-h" ] || [ "$DOMAIN" = "--help" ]; then
	usage
	exit 0
fi
if [ "$DOMAIN" = "--list" ]; then
	list_domains
	exit 0
fi
if ! in_list "$DOMAIN" "$ALL_DOMAINS"; then
	echo "✗ 未知域名：$DOMAIN"
	echo
	list_domains
	exit 0
fi
if [ ! -d "$SPECS_DIR" ]; then
	echo "✗ 找不到 $SPECS_DIR（请在仓库根执行）"
	exit 0
fi

# ── 建表：capability → domain（一次遍历，后续查表）──────────────
declare -A CAP_DOMAIN=()
declare -A CAP_LINES=()
declare -A CAP_PATH=()

echo "══════ $DOMAIN ══════"
echo

# ── 第 1 段：本域 main spec 清单 ───────────────────────────
echo "## 1. 本域 main spec"
count_spec=0
declare -A IN_DOMAIN=()
for f in "$SPECS_DIR"/*/spec.md; do
	[ -r "$f" ] || continue
	cap="${f%/spec.md}"; cap="${cap##*/}"
	raw="$(spec_domain_raw "$f")"
	CAP_DOMAIN["$cap"]="$raw"
	CAP_PATH["$cap"]="$f"
	CAP_LINES["$cap"]="$(wc -l <"$f" | tr -d ' ')"
	if spec_in_domain "$f" "$DOMAIN"; then
		IN_DOMAIN["$cap"]=1
		count_spec=$((count_spec + 1))
		printf '  %-42s %5s 行  %s\n' "$cap" "${CAP_LINES[$cap]}" "$(last_change_date "$f")"
	fi
done
if [ "$count_spec" -eq 0 ]; then
	echo "  （无——该域还没有任何 capability 声明它；新 capability 建立时请补 <!-- domain: $DOMAIN -->）"
fi
echo

# ── 第 2 段：历史 change 时间线 ────────────────────────────
# 每个 archived change 的域 = 其 delta capability 的域并集
echo "## 2. 历史 change 时间线"
hits=0
orphans=0
timeline=""
ORPHAN_LINES=""
declare -a MATCHED_CDIRS=()
for cdir in "$ARCHIVE_DIR"/*/; do
	[ -d "$cdir" ] || continue
	cname="${cdir%/}"; cname="${cname##*/}"
	change_domains=""
	change_caps=""
	has_orphan="" # 空串=无孤儿；1=有孤儿（用 :+ 时判值不判非空，R-1）
	orphan_caps=""
	for df in "$cdir"specs/*/spec.md; do
		[ -r "$df" ] || continue
		cap="${df%/spec.md}"; cap="${cap##*/}"
		change_caps="$change_caps $cap"
		if [ -n "${CAP_DOMAIN[$cap]:-}" ]; then
			change_domains="$change_domains ${CAP_DOMAIN[$cap]}"
		else
			has_orphan=1
			orphan_caps="$orphan_caps $cap"
		fi
	done
	# 域串按逗号空格切分后精确比较
	matched=0
	if [ -n "$change_domains" ]; then
		oldifs="$IFS"
		IFS=', '
		for d in $change_domains; do
			[ "$d" = "$DOMAIN" ] && { matched=1; break; }
		done
		IFS="$oldifs"
	fi
	if [ "$matched" -eq 1 ]; then
		hits=$((hits + 1))
		MATCHED_CDIRS+=("$cdir")
		timeline="${timeline}${cname}|${change_caps}${has_orphan:+|orphan}
"
		if [ -n "$has_orphan" ]; then
			orphans=$((orphans + 1))
			ORPHAN_LINES="${ORPHAN_LINES}${cname}|${orphan_caps}
"
		fi
	fi
done
echo "  $hits 个 archived change 碰过本域"
if [ "$hits" -gt 0 ]; then
	echo "${timeline}" | sort -r | head -12 | while IFS='|' read -r cname caps flag; do
		[ -z "$cname" ] && continue
		date_part="$(echo "$cname" | grep -oE '^[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)"
		name_part="${cname#"${date_part}"-}"
		printf '  %-12s %-46s ←%s%s\n' "${date_part:-(无日期)}" "${name_part:0:44}" "$caps" "${flag:+ [含孤儿 delta]}"
	done
	echo "  （只显示最近 12 条）"
fi
echo

# ── 第 3 段：flow 红线 ────────────────────────────────────
echo "## 3. flow 红线"
if in_list "$DOMAIN" "$BUSINESS_DOMAINS" && [ -x "$DIGEST" ]; then
	bash "$DIGEST" --brief --domains "$DOMAIN" 2>/dev/null | sed 's/^/  /' || echo "  （constraint-digest 调用失败）"
else
	echo "  （工具域无 flow 红线；业务域才会输出）"
fi
echo

# ── 第 4 段：可能冲突（历史 REMOVED）────────────────────────
echo "## 4. 可能冲突 —— 历史 REMOVED 的 Requirement（提示性罗列，不判断）"
found=0
for cdir in "${MATCHED_CDIRS[@]:-}"; do
	[ -n "$cdir" ] && [ -d "$cdir" ] || continue
	cname="${cdir%/}"; cname="${cname##*/}"
	for df in "$cdir"specs/*/spec.md; do
		[ -r "$df" ] || continue
		cap="${df%/spec.md}"; cap="${cap##*/}"
		# 未标 domain 的 capability 不参与域反查（孤儿在第 2 段计数，第 4/5 段不拿它凑热闹）
		[ -z "${IN_DOMAIN[$cap]:-}" ] && continue
		while IFS= read -r req; do
			[ -z "$req" ] && continue
			found=$((found + 1))
			printf '  [%s] %s · %s\n    ✗ %s\n' "${cname:0:10}" "${cname#*-*-*-}" "$cap" "$req"
		done < <(REQ_TITLES_BY_SECTION "$df" REMOVED)
	done
done
[ "$found" -eq 0 ] && echo "  （该域无历史 REMOVED 条目）"
echo

# ── 第 5 段：可能必须沿用（历史 MODIFIED，按日期倒序取前 15）──────
echo "## 5. 可能必须沿用 —— 历史 MODIFIED 的 Requirement（提示性罗列，按 change 日期倒序取前 15）"
found=0
lines5=""
for cdir in "${MATCHED_CDIRS[@]:-}"; do
	[ -n "$cdir" ] && [ -d "$cdir" ] || continue
	cname="${cdir%/}"; cname="${cname##*/}"
	for df in "$cdir"specs/*/spec.md; do
		[ -r "$df" ] || continue
		cap="${df%/spec.md}"; cap="${cap##*/}"
		[ -z "${IN_DOMAIN[$cap]:-}" ] && continue
		while IFS= read -r req; do
			[ -z "$req" ] && continue
			found=$((found + 1))
			lines5="${lines5}${cname}|${cap}|${req}
"
		done < <(REQ_TITLES_BY_SECTION "$df" MODIFIED)
	done
done
if [ "$found" -eq 0 ]; then
	echo "  （该域无历史 MODIFIED 条目）"
else
	printf '%s' "$lines5" | sort -r | head -15 | while IFS='|' read -r cname cap req; do
		[ -z "$cname" ] && continue
		printf '  [%s] %s · %s\n    ~ %s\n' "${cname:0:10}" "${cname#*-*-*-}" "$cap" "$req"
	done
	echo "  （共 $found 条，上列按 change 日期倒序，只显示最近 15 条）"
fi
echo

# ── 第 6 段：孤儿 delta（R-3）─────────────────────────────
echo "## 6. 孤儿 delta —— 指向的能力没有域归属（需人工补标）"
if [ -z "${ORPHAN_LINES:-}" ]; then
	echo "  （无：所有历史 delta 都指向已标 domain 的 capability）"
else
	printf '%s' "$ORPHAN_LINES" | sort -r | while IFS='|' read -r cname caps; do
		[ -z "$cname" ] && continue
		printf '  [%s] %s ←%s\n' "${cname:0:10}" "${cname#*-*-*-}" "$caps"
	done
	echo "  （共 $orphans 个 archived change 含孤儿 delta；补标方式：给对应 main spec 加 <!-- domain: ... -->；能力已退役且 main spec 已删的无需处理）"
fi
echo
echo "提示：第 4/5/6 段只罗列不断言。改完功能后，冲突与沿用判断仍由人 / review 完成。"
exit 0
