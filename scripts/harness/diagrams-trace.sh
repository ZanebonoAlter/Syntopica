#!/usr/bin/env bash
# diagrams marker 与图/理由一致性对账（archive-readiness ⑧；契约源 change-diagram-convention）
#
# 只查三件眼睛不可靠的事（design D3）：
#   1. proposal.md 头部有 <!-- diagrams: full(理由) --> 或 <!-- diagrams: none(理由) -->
#   2. none 档理由非空
#   3. full 档 design-diagram.md 存在且含至少一块 ```mermaid
#
# 不查：图的类型是否恰当、mermaid 语法、图是否准确、none 理由是否成立。
#
# legacy 豁免（不追溯）：无 .openspec.yaml，或目录已位于 changes/archive/ 下。
#
# 退出码：0 通过（含 legacy 跳过）；1 不通过；2 用法错误。
set -uo pipefail

CHANGE_DIR="${1:-}"
if [ -z "$CHANGE_DIR" ] || [ ! -d "$CHANGE_DIR" ]; then
	echo "用法: $0 <change-dir>" >&2
	exit 2
fi
CHANGE_DIR="${CHANGE_DIR%/}"

# 已归档补档豁免：相对/绝对路径都要认（加不加前导 / 都算）
case "/$CHANGE_DIR/" in
*/changes/archive/*) echo "— legacy（已归档补档），跳过"; exit 0 ;;
esac
if [ ! -f "$CHANGE_DIR/.openspec.yaml" ]; then
	echo "— legacy（无 .openspec.yaml），跳过"
	exit 0
fi

PROPOSAL="$CHANGE_DIR/proposal.md"
if [ ! -f "$PROPOSAL" ]; then
	echo "✗ 缺 proposal.md（diagrams marker 的载体）"
	exit 1
fi

# 只在头部找 marker：首个「非注释、非空、非标题」正文行之前的 HTML 注释区
marker="$(python3 - "$PROPOSAL" <<'PY'
import re, sys
text = open(sys.argv[1], encoding='utf-8').read()
head = []
for line in text.splitlines():
    s = line.strip()
    if not s:
        continue
    if s.startswith('<!--'):
        head.append(s)
        continue
    break  # 头部结束
blob = '\n'.join(head)
m = re.search(r'<!--\s*diagrams:\s*(full|none)\s*\((.*?)\)\s*-->', blob, re.S)
if m:
    print(m.group(1) + '\t' + m.group(2).strip())
PY
)"

if [ -z "$marker" ]; then
	# 区分「完全没有」与「有但不是二值」
	if python3 - "$PROPOSAL" <<'PY'
import re, sys
text = open(sys.argv[1], encoding='utf-8').read()
head = []
for line in text.splitlines():
    s = line.strip()
    if not s:
        continue
    if s.startswith('<!--'):
        head.append(s); continue
    break
sys.exit(0 if re.search(r'<!--\s*diagrams:', '\n'.join(head)) else 1)
PY
	then
		echo "✗ diagrams marker 值非法：只认 <!-- diagrams: full(理由) --> 或 none(理由)"
	else
		echo "✗ 缺 diagrams marker：proposal.md 头部需写 <!-- diagrams: full(理由) --> 或 <!-- diagrams: none(理由) -->"
	fi
	exit 1
fi

kind="${marker%%	*}"
reason="${marker#*	}"

if [ -z "$reason" ]; then
	echo "✗ diagrams: none/full 的括号里必须写一句大白话理由（当前为空）"
	exit 1
fi

if [ "$kind" = "none" ]; then
	echo "✓ diagrams: none（$(printf '%s' "$reason" | cut -c1-40)…）— 理由正确性由 review 判"
	exit 0
fi

DIAGRAM="$CHANGE_DIR/design-diagram.md"
if [ ! -f "$DIAGRAM" ]; then
	echo "✗ diagrams: full 但缺 design-diagram.md（图必须在 change 根）"
	exit 1
fi

blocks="$(grep -c '^[`][`][`]mermaid' "$DIAGRAM" 2>/dev/null || true)"
if [ "${blocks:-0}" -eq 0 ]; then
	echo "✗ design-diagram.md 里没有 mermaid 代码块（三个反引号 + mermaid）"
	exit 1
fi

echo "✓ diagrams: full（${blocks} 张图）— 类型与准确性由看图的人判"
exit 0
