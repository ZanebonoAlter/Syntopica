#!/usr/bin/env bash
# ui-prototype-check.sh — 原型「危险操作必须有二次确认」的机械检查
#                        （add-ui-prototype-mechanics-gate 引入）
#
# 用法：bash scripts/harness/ui-prototype-check.sh <change-dir>
#   <change-dir> 形如 openspec/changes/<name>（仓库根相对或绝对路径均可）
#
# 退出码：0 = 通过或不适用（fail-open 放行）；1 = 有违规；2 = 参数/路径错误
#
# 只查这一件事，理由是它**不依赖 CSS 解析**：
#   「页面里有破坏性操作（删除/清空/重置…）但没有任何确认机制」是可判的事实。
#   对照之下，热区尺寸 / :focus / :hover 的静态判定只能看「字样在不在」——
#   实测连 `outline: none` 都能过关、用浏览器默认焦点环的反被判红，那属于形式主义，
#   已从本检查撤回（保留为规范里「人核」的设计下限，见
#   docs/reference/standard/frontend/prototype-content.md「可操作性下限」）。
#
# 判定逻辑的单一事实源：spec-gate 检查⑨ 与 archive-readiness 第⑨项只认本脚本退出码。
#
# 扫描范围：ui-prototype/ 顶层 *.html + skeleton/**/*.html。**不递归扫其他子目录**
# ——three.js 首页改版那类视觉探索原型天然有多版本实验页（现存某 change 顶层就 20 个
# 平级 HTML、整目录 504 文件/98M），把它们当功能原型查只会得到噪音。
#
# ⚠️ 所有文本判定一律用 here-string（`grep -q … <<<"$var"`）而不是管道：
#    `set -o pipefail` 下 `printf | grep -q` 会在 grep 命中早退时让写端吃 SIGPIPE(141)，
#    整条管道判非零 → 同一输入随机误红（评审实测 30 次跑出 5 种裁决）。

set -uo pipefail

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ] || [ ! -d "$CHANGE_DIR" ]; then
	printf '用法: bash scripts/harness/ui-prototype-check.sh <change-dir>\n' >&2
	exit 2
fi

PROPOSAL="$CHANGE_DIR/proposal.md"
PROTO_DIR="$CHANGE_DIR/ui-prototype"

# ── 门户⓪：非 syntopica-ui schema（含缺失）→ legacy 豁免 ────────────────
# 与 ui-verification-trace.sh 的 ⓪ 同一口径：旧 change 不因新增检查卡死。
if [ ! -f "$CHANGE_DIR/.openspec.yaml" ]; then
	printf '✓ ui-prototype-check: 无 .openspec.yaml（legacy change），放行\n'
	exit 0
fi

if [ ! -f "$PROPOSAL" ]; then
	printf '✓ ui-prototype-check: 无 proposal.md，放行（fail-open）\n'
	exit 0
fi

# ── 门户①：只查 major 档 ──────────────────────────────────────────────
IMPACT="$(grep -oE '^<!--[[:space:]]*ui-impact:[[:space:]]*(major|minor|none)' "$PROPOSAL" 2>/dev/null | head -1 | grep -oE '(major|minor|none)$')"
if [ "${IMPACT:-}" != "major" ]; then
	printf '✓ ui-prototype-check: ui-impact=%s，不适用（仅 major 档查）\n' "${IMPACT:-未声明}"
	exit 0
fi

# ── 门户②：视觉探索原型不查 ───────────────────────────────────────────
# 功能原型（状态/交互/组件）才适用机械检查；three.js 首页改版这类视觉探索原型
# 产物量天然巨大、状态不闭环是常态，按功能原型的规则查它没有意义。
if grep -qE '^<!--[[:space:]]*ui-prototype-kind:[[:space:]]*exploratory' "$PROPOSAL" 2>/dev/null; then
	printf '✓ ui-prototype-check: ui-prototype-kind=exploratory（视觉探索原型），放行\n'
	exit 0
fi

# ── 门户③：必须有原型目录 ────────────────────────────────────────────
if [ ! -d "$PROTO_DIR" ]; then
	printf '✓ ui-prototype-check: major 档但无 ui-prototype/，无原型可查\n'
	exit 0
fi

# ── 收集扫描目标（顶层 *.html + skeleton/**）──────────────────────────
HTML_FILES=()
while IFS= read -r f; do
	[ -n "$f" ] && HTML_FILES+=("$f")
done < <(find "$PROTO_DIR" -maxdepth 1 -type f -name '*.html' 2>/dev/null | sort)
if [ -d "$PROTO_DIR/skeleton" ]; then
	while IFS= read -r f; do
		[ -n "$f" ] && HTML_FILES+=("$f")
	done < <(find "$PROTO_DIR/skeleton" -type f -name '*.html' 2>/dev/null | sort)
fi

if [ "${#HTML_FILES[@]}" -eq 0 ]; then
	printf '✓ ui-prototype-check: ui-prototype/ 下无 *.html（画布类等形态按 interaction-table.md 承接），放行\n'
	exit 0
fi

# ── 门户④：显式跳过标记（根元素上，理由非空才生效）────────────────────
# 只认 <html …> 与 <body …> 这两个根标签：写在任意子元素上等于一键全免，太松。
SKIP_REASON=""
for f in "${HTML_FILES[@]}"; do
	hit="$(grep -oE '<(html|body)[^>]*data-ui-mechanics-skip="[^"]*"' "$f" 2>/dev/null | head -1 | sed -E 's/.*="(.*)"$/\1/')"
	if [ -n "$hit" ]; then
		SKIP_REASON="$hit"
		break
	fi
done
if [ -n "$SKIP_REASON" ]; then
	printf '✓ ui-prototype-check: 已豁免 — 理由：%s\n' "$SKIP_REASON"
	exit 0
fi

# ── EC：破坏性操作必须有二次确认 ──────────────────────────────────────
# 破坏性识别（双通道，任一命中）：
#   ① 可点元素（button / a[href] / input[type=submit|button] / [role=button]）的直接文本
#      含 删除|清空|重置|移除|彻底|销毁
#   ② 元素或其后代带 data-destructive
# 确认机制（满足任一）：
#   ① 同一标签内 data-confirm / onclick 含 confirm(
#   ② 文件里存在 <dialog（确认对话框）
#   ③ 该元素所在 <form> 内存在含「确认」的按钮
# 满足不了 → 红，并列出命中行（file:line + 元素片段）。

DESTR_WORDS='删除|清空|重置|移除|彻底|销毁'
CLICK_TAG='<(button|a[[:space:]][^>]*href=|input[^>]*type="(submit|button)"|div[^>]*role="button")'
# <input type=submit value="删除"> 的文字在属性里，单独一条通道
INPUT_VALUE_RE='<input[^>]*type="(submit|button)"[^>]*value="[^"]*('"$DESTR_WORDS"')'

FINDINGS=()
for f in "${HTML_FILES[@]}"; do
	has_dialog=0
	grep -qE '<dialog' "$f" 2>/dev/null && has_dialog=1

	while IFS=$'\t' read -r lineno tagline; do
		[ -z "${tagline:-}" ] && continue
		# ① 同一标签内的确认
		if [[ "$tagline" =~ data-confirm|confirm\( ]]; then
			continue
		fi
		# ② 文件级确认对话框
		if [ "$has_dialog" -eq 1 ]; then
			continue
		fi
		# ③ 该行附近（前 3 行内）出现「确认」按钮或文案
		near="$(sed -n "$((lineno > 3 ? lineno - 3 : 1)),$((lineno + 3))p" "$f" 2>/dev/null)"
		if grep -qE '确认' <<<"$near"; then
			continue
		fi
		FINDINGS+=("$f:$lineno  ${tagline:0:110}")
	done < <(
		# 行号 + 行内容：破坏性文字通道
		grep -nE "$CLICK_TAG[^>]*>[^<]*($DESTR_WORDS)" "$f" 2>/dev/null | sed -E 's/^([0-9]+):/\1\t/'
		# 行号 + 行内容：input value 通道（文字在属性里）
		grep -nE "$INPUT_VALUE_RE" "$f" 2>/dev/null | sed -E 's/^([0-9]+):/\1\t/'
		# 行号 + 行内容：data-destructive 通道
		grep -nE 'data-destructive' "$f" 2>/dev/null | sed -E 's/^([0-9]+):/\1\t/'
	)
done

printf 'ui-prototype-check: %s\n' "$CHANGE_DIR"
printf '  扫描 %d 个 HTML\n' "${#HTML_FILES[@]}"

# 无破坏性操作 → 无事可查
for f in "${HTML_FILES[@]}"; do
	if grep -qE "$CLICK_TAG[^>]*>[^<]*($DESTR_WORDS)" "$f" 2>/dev/null ||
		grep -qE "$INPUT_VALUE_RE" "$f" 2>/dev/null ||
		grep -qE 'data-destructive' "$f" 2>/dev/null; then
		HAS_DESTR=1
		break
	fi
done

if [ "${HAS_DESTR:-0}" -eq 0 ]; then
	printf '✓ 无破坏性操作，检查通过\n'
	exit 0
fi

if [ "${#FINDINGS[@]}" -gt 0 ]; then
	printf '  ✗ 破坏性操作缺二次确认（共 %d 处）：\n' "${#FINDINGS[@]}"
	for line in "${FINDINGS[@]}"; do
		printf '      %s\n' "$line"
	done
	printf '\n✗ 检查不通过（规范源：docs/reference/standard/frontend/prototype-content.md「可操作性下限」）\n'
	printf '  修法：给该元素加 data-confirm="<询问文案>"，或改用 <dialog> 确认，或放在 <form> 里配「确认」按钮\n'
	printf '  确需豁免：在 <html> 或 <body> 上加 data-ui-mechanics-skip="<理由>"（理由非空才生效）\n'
	exit 1
fi

printf '✓ 破坏性操作均有二次确认\n'
exit 0
