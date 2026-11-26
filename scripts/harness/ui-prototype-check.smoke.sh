#!/usr/bin/env bash
# ui-prototype-check.smoke.sh — 原型「危险操作确认」门禁的断言集
#                              （add-ui-prototype-mechanics-gate）
#
# 用法：bash scripts/harness/ui-prototype-check.smoke.sh
# 全部断言在临时目录造的假 change 上跑，不碰真 change。
#
# 含 R-1 回归：大样式文本 + 多次连跑，断言裁决稳定（pipefail/SIGPIPE 曾让同一输入
# 跑出 5 种裁决，fixture 太小则测不出来——所以这里专门造 100KB 输入连跑 5 次）。

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/scripts/harness/ui-prototype-check.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); printf '  ✓ %s\n' "$1"; }
no() { FAIL=$((FAIL + 1)); printf '  ✗ %s\n' "$1"; }

# mk <name> <ui-impact> [html] [proposal 额外注释行]
mk() {
	local name="$1" impact="$2" html="${3:-}" extra="${4:-}"
	local d="$TMP/$name"
	mkdir -p "$d/ui-prototype"
	{
		printf '<!-- ui-impact: %s -->\n' "$impact"
		[ -n "$extra" ] && printf '%s\n' "$extra"
		printf '\n# P\n'
	} >"$d/proposal.md"
	printf 'schema: syntopica-ui\ncreated: 2026-11-26\n' >"$d/.openspec.yaml"
	[ -n "$html" ] && printf '%s\n' "$html" >"$d/ui-prototype/index.html"
	printf '%s' "$d"
}

run() { bash "$SCRIPT" "$1" >/dev/null 2>&1; echo $?; }

GOOD='<html><body><button data-confirm="确定删除？">删除记录</button></body></html>'
BAD='<html><body><button>删除记录</button></body></html>'

# 1 有 data-confirm → 0
d=$(mk ok major "$GOOD")
[ "$(run "$d")" = "0" ] && ok "data-confirm 放行" || no "data-confirm 应放行"

# 2 无确认 → 1
d=$(mk bad major "$BAD")
[ "$(run "$d")" = "1" ] && ok "无确认判红" || no "无确认应红"

# 3 无破坏性操作 → 0
d=$(mk clean major '<html><body><button>保存</button></body></html>')
[ "$(run "$d")" = "0" ] && ok "无破坏性操作放行" || no "无破坏性应放行"

# 4 onclick confirm → 0
d=$(mk onclick major '<html><body><button onclick="return confirm(1)">删除</button></body></html>')
[ "$(run "$d")" = "0" ] && ok "onclick confirm 放行" || no "onclick confirm 应放行"

# 5 <dialog> 文件级确认 → 0
d=$(mk dialog major '<html><body><button>删除</button><dialog>确定？</dialog></body></html>')
[ "$(run "$d")" = "0" ] && ok "<dialog> 放行" || no "dialog 应放行"

# 6 data-destructive 通道识别 → 1（无确认）
d=$(mk datadestr major '<html><body><button data-destructive>清理</button></body></html>')
[ "$(run "$d")" = "1" ] && ok "data-destructive 通道判红" || no "data-destructive 应红"

# 7 input[type=submit] 文字通道（R-5）→ 1
d=$(mk inputbtn major '<html><body><form><input type="submit" value="删除"></form></body></html>')
[ "$(run "$d")" = "1" ] && ok "input[type=submit] 被识别（R-5）" || no "input 应被识别"

# 8 div[role=button] 通道（R-5）→ 1
d=$(mk divbtn major '<html><body><div role="button">删除</div></body></html>')
[ "$(run "$d")" = "1" ] && ok "div[role=button] 被识别（R-5）" || no "div 按钮应被识别"

# 9 跳过标记带理由 → 0 且回显
d=$(mk skip major "$BAD" '')
printf '%s\n' '<html data-ui-mechanics-skip="视觉实验页，无真实数据"><body><button>删除</button></body></html>' >"$d/ui-prototype/index.html"
out=$(bash "$SCRIPT" "$d" 2>&1); rc=$?
[ "$rc" = "0" ] && grep -q '视觉实验页' <<<"$out" && ok "根元素跳过标记生效并回显理由" || no "跳过标记应生效"

# 10 空理由豁免无效 → 1
d=$(mk skip-empty major "$BAD" '')
printf '%s\n' '<html data-ui-mechanics-skip=""><body><button>删除</button></body></html>' >"$d/ui-prototype/index.html"
[ "$(run "$d")" = "1" ] && ok "空理由豁免无效" || no "空理由应无效"

# 11 子元素上的跳过标记不生效（R-8）→ 1
d=$(mk skip-nested major "$BAD" '')
printf '%s\n' '<html><body><div data-ui-mechanics-skip="想偷懒"><button>删除</button></div></body></html>' >"$d/ui-prototype/index.html"
[ "$(run "$d")" = "1" ] && ok "子元素跳过标记不生效（R-8）" || no "子元素标记不应生效"

# 12 minor 档跳过 → 0
d=$(mk minor minor "$BAD")
[ "$(run "$d")" = "0" ] && ok "minor 档跳过" || no "minor 应跳过"

# 13 exploratory 原型跳过 → 0
d=$(mk exploratory major "$BAD" '<!-- ui-prototype-kind: exploratory -->')
[ "$(run "$d")" = "0" ] && ok "视觉探索原型跳过" || no "exploratory 应跳过"

# 14 声明缺失 → 0
d="$TMP/no-flag"
mkdir -p "$d/ui-prototype"
printf '# P\n' >"$d/proposal.md"
printf 'schema: syntopica-ui\ncreated: 2026-11-26\n' >"$d/.openspec.yaml"
printf '%s' "$BAD" >"$d/ui-prototype/index.html"
[ "$(run "$d")" = "0" ] && ok "ui-impact 缺失 fail-open" || no "缺失声明应放行"

# 15 legacy（无 yaml）→ 0
d="$TMP/legacy"
mkdir -p "$d/ui-prototype"
printf '<!-- ui-impact: major -->\n' >"$d/proposal.md"
printf '%s' "$BAD" >"$d/ui-prototype/index.html"
[ "$(run "$d")" = "0" ] && ok "legacy change 放行" || no "legacy 应放行"

# 16 无原型目录 → 0
d="$TMP/no-proto"
mkdir -p "$d"
printf '<!-- ui-impact: major -->\n' >"$d/proposal.md"
printf 'schema: syntopica-ui\ncreated: 2026-11-26\n' >"$d/.openspec.yaml"
[ "$(run "$d")" = "0" ] && ok "无原型目录跳过" || no "无原型应跳过"

# 17 画布类无 HTML → 0
d="$TMP/canvas"
mkdir -p "$d/ui-prototype"
printf '<!-- ui-impact: major -->\n' >"$d/proposal.md"
printf 'schema: syntopica-ui\ncreated: 2026-11-26\n' >"$d/.openspec.yaml"
printf '{}' >"$d/ui-prototype/fixture.json"
[ "$(run "$d")" = "0" ] && ok "无 HTML 的画布类跳过" || no "画布类应跳过"

# 18 实验子目录不参与（只扫顶层 + skeleton）
d=$(mk subdir major '<html><body><button>保存</button></body></html>')
mkdir -p "$d/ui-prototype/city-assembly-01"
printf '%s' "$BAD" >"$d/ui-prototype/city-assembly-01/exp.html"
[ "$(run "$d")" = "0" ] && ok "实验子目录不参与检查" || no "子目录不应参与"

# 19 无参数 → 2
bash "$SCRIPT" >/dev/null 2>&1
[ $? -eq 2 ] && ok "无参数 exit 2" || no "无参数应 exit 2"

# 20 R-1 回归：100KB 样式 + 破坏性按钮，连跑 5 次裁决必须一致
d=$(mk big major "$BAD" '')
{
	printf '<html><head><style>\n'
	for i in $(seq 1 4000); do printf '.c%d{color:#%03d}\n' "$i" "$((i % 999))"; done
	printf '</style></head><body><button>删除</button></body></html>\n'
} >"$d/ui-prototype/index.html"
first="$(run "$d")"
stable=1
for _ in 1 2 3 4; do
	[ "$(run "$d")" = "$first" ] || stable=0
done
[ "$stable" -eq 1 ] && [ "$first" = "1" ] && ok "100KB 输入连跑 5 次裁决一致（R-1 回归）" || no "裁决不稳定（R-1 复发）"

# 21 同上但应放行：大样式 + 有确认
d=$(mk big-ok major "$GOOD" '')
{
	printf '<html><head><style>\n'
	for i in $(seq 1 4000); do printf '.c%d{color:#%03d}\n' "$i" "$((i % 999))"; done
	printf '</style></head><body><button data-confirm="确定？">删除</button></body></html>\n'
} >"$d/ui-prototype/index.html"
first="$(run "$d")"
stable=1
for _ in 1 2 3 4; do
	[ "$(run "$d")" = "$first" ] || stable=0
done
[ "$stable" -eq 1 ] && [ "$first" = "0" ] && ok "大输入 + 有确认连跑 5 次稳定放行（R-1 回归）" || no "大输入放行不稳定"

printf '\n==============================\n  smoke 通过 %d / 失败 %d\n==============================\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
