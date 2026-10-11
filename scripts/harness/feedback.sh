#!/usr/bin/env bash
# feedback.sh — human-feedback.md 反馈建账三件套（add-change-lifecycle-retro design D7/D8）
#
# 把「反馈归因」从回忆题变选择题（对齐 doc-impact.sh 白名单模式）。三个子命令：
#   menu                                打印归因/类型/状态词表与条目格式契约
#   add <change> --layer <归因> --type <产品|流程> "<反馈要点>"
#                                      定位 change 目录（已归档拒绝建账，提示走归档后修订）→
#                                      human-feedback.md 不存在时按模板创建（头部含定位说明）；
#                                      编号取现有最大 FB-N 自增；状态初始化「待处理」；
#                                      词表外归因/类型非零退出且零写入
#   verify <change-dir>                扫描条目状态：待处理/处理中/状态不明（格式异常）→
#                                      处置提醒 + 退出码 1；全部处置/文件缺席 → 退出码 0
#
# 条目格式契约（design D7，一行正则全解析；标签放条目级而非文件头——一个文件多条、
# 每条归因不同）：
#   ### FB-<N> <日期>【<类型>|<归因>】<标题>
#   - **状态**：<状态词>（括号说明可选）
# 归因五类封闭（高发层→规范整理去向映射）：frontend/backend/test-gap/fact-check/process；
# 状态五值封闭：待处理/处理中/已消化/已转挂/已放弃。
# human-feedback.md 不进 events.db（文件随 change 归档永久保存；账本有 TTL）。
#
# 详见 openspec/changes/add-change-lifecycle-retro/design.md D7/D8。
# 退出码：0 成功/通过；1 词表外值/未处置条目/change 不存在/已归档拒绝；2 用法错误。
# 测试缝：FEEDBACK_ROOT 指向仓库根（默认脚本自定位；smoke 注入临时 fixture）。
# POSIX bash（Linux / macOS / WSL）可跑。

set -u

REPO_ROOT="${FEEDBACK_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
CHANGES_DIR="$REPO_ROOT/openspec/changes"
ARCHIVE_DIR="$CHANGES_DIR/archive"

# 归因词表（封闭枚举；与 retro --lifecycle 第五段解析器、doc-authoring.md 契约三方同步）
LAYER_WHITELIST="frontend backend test-gap fact-check process"
# 类型词表（封闭二值）
TYPE_WHITELIST="产品 流程"
# 状态词表（封闭五值；状态只用于 add 初始化 + 归档对账 + 体检卡展示，不算指标）
STATUS_WHITELIST="待处理 处理中 已消化 已转挂 已放弃"

# ---------------------------------------------------------------------------
# 公共：change 目录定位（未归档 changes/；已归档 = archive/ 下存在日期前缀同名目录）
#   echo 目录绝对路径；已归档 exit 1（提示归档后修订流程）；不存在 exit 1。
# ---------------------------------------------------------------------------
locate_change_dir() {
	local name="$1"
	if ! [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
		echo "✗ 非法 change 名: $name（须为 kebab-case 形态）" >&2
		return 1
	fi
	if [ ! -d "$CHANGES_DIR/$name" ]; then
		echo "✗ change 目录不存在: openspec/changes/$name" >&2
		return 1
	fi
	# 已归档拒绝建账（终态以归档为准；归档后制品的反馈另开 change 记录）
	if compgen -G "$ARCHIVE_DIR/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-$name" >/dev/null 2>&1; then
		echo "✗ 已归档 change 拒绝建账: $name（archive/ 存在日期前缀同名目录）——走归档后修订流程：另开新 change 记录反馈" >&2
		return 1
	fi
	printf '%s\n' "$CHANGES_DIR/$name"
}

# ---------------------------------------------------------------------------
# 公共：取现有最大 FB-N（容忍格式漂移——只认 `### FB-数字` 前缀，不管标签部分）
# ---------------------------------------------------------------------------
max_fb_no() {
	local f="$1" max=0 n
	[ -f "$f" ] || { echo 0; return 0; }
	for n in $(grep -oE '^### FB-[0-9]+' "$f" 2>/dev/null | grep -oE '[0-9]+$'); do
		[ "$n" -gt "$max" ] && max="$n"
	done
	echo "$max"
}

# ---------------------------------------------------------------------------
# menu：打印词表（选择题而非自由发挥）
# ---------------------------------------------------------------------------
cmd_menu() {
	cat <<'EOF'
human-feedback 反馈建账（design D7/D8：收到用户对产物的反馈时记入 change 目录 human-feedback.md）：

  归因词表（五类封闭，add --layer 取值；高发层→规范整理去向）：
    frontend    前端设计/交互/视觉 → standard/frontend/
    backend     后端业务/参数/数据 → standard/backend/ + flow/
    test-gap    测试该拦未拦 → test-design.md
    fact-check  探索或搬运时未核实就写（同一问题多因按根因优先归此类）→ 探索纪律
    process     流程/harness 本身 → harness/执行规范

  类型词表（二值封闭，add --type 取值）：产品 | 流程
    产品 = 改本 change 的东西（原型/实现/文档）
    流程 = 改做事的方式与 harness

  状态词表（五值封闭，条目状态行取值）：待处理 | 处理中 | 已消化 | 已转挂 | 已放弃

  建账：bash scripts/harness/feedback.sh add <change> --layer <归因> --type <产品|流程> "<反馈要点>"
  对账：bash scripts/harness/feedback.sh verify <change-dir>
        （归档前顺带自查——待处理/处理中/状态不明条目 → 处置提醒，退出码 1）

  条目格式（一行正则全解析；编号 change 内自增）：
    ### FB-<N> <日期>【<类型>|<归因>】<标题>
    - **状态**：<状态词>（括号说明可选，解析取状态词前缀）
EOF
}

# ---------------------------------------------------------------------------
# add：建账（定位目录 → 词表校验 → 编号自增 → 模板创建/插入 → 状态初始化待处理）
# ---------------------------------------------------------------------------
cmd_add() {
	local change="" layer="" type="" note=""
	while [ $# -gt 0 ]; do
		case "$1" in
		--layer)
			layer="${2:-}"
			shift 2
			;;
		--type)
			type="${2:-}"
			shift 2
			;;
		-*)
			echo "✗ 未知选项: $1" >&2
			exit 2
			;;
		*)
			if [ -z "$change" ]; then
				change="$1"
			elif [ -z "$note" ]; then
				note="$1"
			else
				echo "✗ 多余参数: $1（add 只收 change 名与一条要点）" >&2
				exit 2
			fi
			shift
			;;
		esac
	done
	if [ -z "$change" ] || [ -z "$layer" ] || [ -z "$type" ] || [ -z "$note" ]; then
		echo "用法: feedback.sh add <change> --layer <归因> --type <产品|流程> \"<反馈要点>\"" >&2
		echo "词表: bash scripts/harness/feedback.sh menu" >&2
		exit 2
	fi

	# 词表校验（零写入前提；词表外值非零退出）
	if ! printf '%s' "$LAYER_WHITELIST" | tr ' ' '\n' | grep -qxF "$layer"; then
		echo "✗ 归因词表外: $layer（五类封闭：$LAYER_WHITELIST；bash scripts/harness/feedback.sh menu 查看）" >&2
		exit 1
	fi
	if ! printf '%s' "$TYPE_WHITELIST" | tr ' ' '\n' | grep -qxF "$type"; then
		echo "✗ 类型词表外: $type（二值封闭：产品|流程）" >&2
		exit 1
	fi

	local dir
	dir="$(locate_change_dir "$change")" || exit 1

	local f="$dir/human-feedback.md"
	local no
	no=$(( $(max_fb_no "$f") + 1 ))
	local date
	date="$(date +%F)"

	# 写入（python3：换行单行化 + 模板创建/处置记录节前插入；对齐 ai-provider-model-overhaul 首例文件结构）
	if ! FEEDBACK_PY_FILE="$f" FEEDBACK_PY_CHANGE="$change" FEEDBACK_PY_NO="$no" \
		FEEDBACK_PY_TYPE="$type" FEEDBACK_PY_LAYER="$layer" FEEDBACK_PY_NOTE="$note" \
		FEEDBACK_PY_DATE="$date" python3 <<'FBADDPY'
import os
import re

f = os.environ['FEEDBACK_PY_FILE']
change = os.environ['FEEDBACK_PY_CHANGE']
no = os.environ['FEEDBACK_PY_NO']
ftype = os.environ['FEEDBACK_PY_TYPE']
layer = os.environ['FEEDBACK_PY_LAYER']
note = re.sub(r'\s+', ' ', os.environ['FEEDBACK_PY_NOTE']).strip()
date = os.environ['FEEDBACK_PY_DATE']

header = '''# Human Feedback — {change}

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

'''.format(change=change)

entry = '''### FB-{no} {date}【{ftype}|{layer}】{note}

- **状态**：待处理
- **原文要点**：{note}
- **已核实事实**：（待补——核实后再写影响面与后续动作；探索/搬运未核实属 fact-check，根因优先）
- **影响面**：（待补）
- **后续动作**：（待补）
'''.format(no=no, date=date, ftype=ftype, layer=layer, note=note)

if os.path.exists(f):
    with open(f, encoding='utf-8') as fh:
        content = fh.read()
    # 插入「## 处置记录」节之前（对齐首例文件结构：反馈清单 → 处置记录）；无该节则追加文件尾
    m = re.search(r'(?m)^## 处置记录\s*$', content)
    if m:
        content = content[:m.start()] + entry + '\n' + content[m.start():]
    else:
        content = content.rstrip('\n') + '\n\n' + entry
    with open(f, 'w', encoding='utf-8') as fh:
        fh.write(content)
else:
    with open(f, 'w', encoding='utf-8') as fh:
        fh.write(header + entry)
FBADDPY
	then
		echo "✗ 写入失败: $f" >&2
		exit 1
	fi
	echo "已记入 FB-$no（待处理，【$type|$layer】）：$f"
}

# ---------------------------------------------------------------------------
# verify：归档对账（扫描状态行；待处理/处理中/状态不明 → 提醒 + exit 1）
#   格式异常（状态行缺失/状态词表外）视同未处置——状态不明不允许静默放行（不许静默丢）。
# ---------------------------------------------------------------------------
cmd_verify() {
	local dir="${1:-}"
	if [ -z "$dir" ]; then
		echo "用法: feedback.sh verify <change-dir>" >&2
		exit 2
	fi
	local f="$dir/human-feedback.md"
	if [ ! -f "$f" ]; then
		echo "verify: $dir — 无 human-feedback.md（未启用反馈建账）"
		exit 0
	fi
	FEEDBACK_PY_FILE="$f" FEEDBACK_PY_DIR="$dir" python3 <<'FBVERIFYPY'
import os
import re
import sys

f = os.environ['FEEDBACK_PY_FILE']
dir_ = os.environ['FEEDBACK_PY_DIR']
STATUSES = ('待处理', '处理中', '已消化', '已转挂', '已放弃')

with open(f, encoding='utf-8') as fh:
    content = fh.read()

# 按 FB 条目标题行分块（### FB-数字 开头；不匹配的头部/表尾忽略）
blocks = [b for b in re.split(r'(?m)^(?=### FB-\d+)', content) if re.match(r'^### FB-\d+', b)]
open_ids, bad_ids = [], []
done = 0
for b in blocks:
    no = re.match(r'^### FB-(\d+)', b.split('\n', 1)[0]).group(1)
    sm = re.search(r'(?m)^- \*\*状态\*\*：\s*(.*)$', b)
    st_word = None
    if sm:
        st_word = next((w for w in STATUSES if sm.group(1).strip().startswith(w)), None)
    if st_word is None:
        bad_ids.append((no, '状态词表外: %s' % sm.group(1).strip()[:20] if sm else '缺状态行'))
    elif st_word in ('待处理', '处理中'):
        open_ids.append('FB-%s %s' % (no, st_word))
    else:
        done += 1

total = len(blocks)
print('verify: %s — %s 条目：已处置 %s / 未处置 %s / 格式异常 %s' % (dir_, total, done, len(open_ids), len(bad_ids)))
for it in open_ids:
    print('  ⚠ %s — 处置提醒：转新 change 或更新状态后再归档（不许静默丢）' % it)
for no, why in bad_ids:
    print('  ⚠ FB-%s — 状态不明视同未处置（%s），待修正（格式契约见文件头；menu 查看词表）' % (no, why))
sys.exit(1 if (open_ids or bad_ids) else 0)
FBVERIFYPY
}

# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------
sub="${1:-}"
case "$sub" in
menu)
	cmd_menu
	;;
add)
	shift
	cmd_add "$@"
	;;
verify)
	shift
	cmd_verify "$@"
	;;
"" | -h | --help | help)
	sed -n '2,27p' "$0"
	echo
	echo "用法: bash scripts/harness/feedback.sh <menu|add|verify> [...]"
	;;
*)
	echo "未知子命令: $sub" >&2
	exit 2
	;;
esac
