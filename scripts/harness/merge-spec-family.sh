#!/usr/bin/env bash
# 把多个 capability 的 spec 合并成族级 spec（结构重构工具，不改语义）。
#
# 用法：
#   bash scripts/harness/merge-spec-family.sh <族名> <cap1> <cap2> ... [--out DIR]
#   bash scripts/harness/merge-spec-family.sh --list          # 查看内置族映射
#
# 行为：
#   1. 按给定顺序抽每个 capability 的 Requirement 块（含 Scenario）
#   2. 每条 Requirement 下插入来源注释 `<!-- from: <cap> (<归档日期> <归档change>) -->`
#      archive 里零引用的 capability 只写 `<!-- from: <cap> -->`，不编造来源
#   3. 拼成 `<族名> Specification` 全文（Purpose 取自内置文案，缺省留 TODO）
#   4. 默认输出到 stdout；--out DIR 写到 DIR/<族名>.md
#
# 不做判断：排序、去重、措辞都留给人工。脚本只保证「条数守恒、内容不丢」。
# 复用于其他族（版块诊断、泳道画布等）时，把族映射加到 FAMILIES 即可。
set -euo pipefail

ROOT="${SPEC_MERGE_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
export SPEC_MERGE_ROOT="$ROOT"

if [ "${1:-}" = "--list" ]; then
	cat <<'EOF'
内置族（可由命令行覆盖）：
  tagging <tagging-domain aggregate-tagging semantic-label-model composite-label article-tag-watch-toggle tag-queue-scheduling tag-queue-progress-chip>
  auxiliary-label <auxiliary-label aux-label-gc auxiliary-label-description-repair tag-hierarchy-quality tag-merge-suggestions tag-merge-ui tag-match-identity-protection>
  tag-matching <tag-to-board-matching tag-embedding-management board-affinity-metadata board-match-auto-trigger board-match-cache match-score-visualization match-detail-ondemand source-board-hit-rate cross-board-relation-discovery>
EOF
	exit 0
fi

out_dir=""
delta_out=""
kind_file=""
kind_offset=0
args=()
while [ $# -gt 0 ]; do
	case "$1" in
		--out) out_dir="${2:?--out 需要目录名}"; shift 2 ;;
		--kind-file) kind_file="${2:?--kind-file 需要文件}"; shift 2 ;;
		--kind-offset) kind_offset="${2:?--kind-offset 需要数字}"; shift 2 ;;
		--delta-out) delta_out="${2:?--delta-out 需要目录}"; shift 2 ;;
		*) args+=("$1"); shift ;;
	esac
done
[ "${#args[@]}" -ge 2 ] || { echo "用法: $0 <族名> <cap> [cap...] [--out DIR] [--kind-file FILE]" >&2; exit 2; }

python3 - "$out_dir" "$delta_out" "$kind_file" "$kind_offset" "${args[@]}" <<'PY'
import os, re, sys, glob

root = os.environ["SPEC_MERGE_ROOT"]
out_dir = sys.argv[1]
delta_out = sys.argv[2]
kind_file = sys.argv[3]
kind_offset = int(sys.argv[4] or 0)
fam, caps = sys.argv[5], sys.argv[6:]

# 序号 → business|impl（全局序号按「族 → cap → Requirement」生成顺序）
KINDS = {}
if kind_file:
    for line in open(kind_file):
        line = line.strip()
        if not line:
            continue
        idx, _, kind = line.partition(" ")
        KINDS[int(idx)] = kind.strip()
        if kind.strip() not in ("business", "impl"):
            sys.exit(f"!! kind 非法: {line}")

# 族级 Purpose 文案（写不出来就留 TODO，不硬编）
PURPOSE = {
    "tagging": (
        "标签是 L0 原子概念：话题、版块、匹配、发现、阅读全部建立在它之上"
        "（`backend-go/internal/tagmanagement` 被 reader / topicgraph / discovery / dataenrichment 四包 import）。"
        "本族定义标签怎么产生——数据模型、从文章提取标签的规则与上限、聚合打标、标签队列调度与进度通知。"
    ),
    "auxiliary-label": (
        "辅助标签是可人工编辑的标签描述层，用于提升版块匹配质量。"
        "本族定义它怎么维护——模型与生命周期、层级质量校验、合并建议与合并界面、清理回收、身份保护。"
    ),
    "tag-matching": (
        "标签匹配到版块（Board）是版块自动化的核心：用 embedding 相似度加辅助标签做匹配，产出候选与分数。"
        "本族定义匹配输入、规则与阈值、缓存、自动触发、分数展示与详情、命中率统计、跨版块关系发现。"
    ),
    "board-diagnostics": (
        "诊断 = 判定版块质量、产出信号/评分/分诊建议，**不改变版块本身**。"
        "本族定义两类判定：①版块体检——贴合度归因、事实标签画像、LLM 综合评分与降级、快照趋势、异常通知打扰裁决、分诊信号与合理性闸门、体检纯读边界；"
        "②匹配方向校验——除 direct_hit 外的命中须算 tag identity 与 board 向量 cosine，低于阈值标 `direction_mismatch`，并规定该标记在日报与 API 的消费口径。"
        "两者都只读不写，故归诊断；匹配决策本身见 `tag-matching`。"
    ),
    "board-research": (
        "研究 = 人工触发、异步产出、可核查报告，研究成本受用户控制（不自动查询、不设评审链）。"
        "本族定义两条研究线：①外部新闻信号——周期材料与固定候选快照、四十轮问题驱动研究、证据与数据时效声明、失败恢复、候选忽略与恢复；"
        "②版块语义调查——可扫描简报、用户选择问题后展开多假设/正反证的深度调查、父子不可变快照、旧报告兼容。"
        "两条线共享「候选 → 人工触发 → 异步产出 → 可核查」骨架，产出物都待人判断而非系统结论。"
    ),
}


def first_change(cap: str) -> str:
    """只认 `## ADDED Requirements` 的 change（= 真正创建该 capability 的那次）。
    MODIFIED 的 change 说明 capability 早已存在，把它当来源会误导（R-4）。"""
    for d in sorted(glob.glob(f"{root}/openspec/changes/archive/*/")):
        p = f"{d}specs/{cap}/spec.md"
        if not os.path.isfile(p):
            continue
        if re.search(r"(?m)^## ADDED Requirements", open(p).read()):
            return os.path.basename(d.rstrip("/"))
    return ""


def requirement_blocks(cap: str):
    path = f"{root}/openspec/specs/{cap}/spec.md"
    if not os.path.isfile(path):
        sys.exit(f"!! 缺 spec: {path}")
    text = open(path).read()
    m = re.search(r"(?m)^## Requirements\s*$", text)
    body = text[m.end():] if m else text
    blocks = []
    for part in re.split(r"(?m)^(?=### Requirement: )", body):
        if not part.startswith("### Requirement: "):
            continue
        # 截掉块尾的非 Requirement 内容（如 ## Notes）
        part = re.split(r"(?m)^## ", part, maxsplit=1)[0].rstrip() + "\n"
        blocks.append(part)
    return blocks


lines = [
    f"# {fam} Specification",
    "",
    "## Purpose",
    "",
    PURPOSE.get(fam, "TODO: 补族级 Purpose（本族解决什么问题、边界在哪）"),
    "",
    "## Requirements",
    "",
]

# 先收集 (kind, 全局序号, 标题, 来源注释, 正文)
collected = []
idx = kind_offset
for cap in caps:
    ch = first_change(cap)
    note = f"<!-- from: {cap} ({ch[:10]} {ch[11:]}) -->" if ch else f"<!-- from: {cap} -->"
    blocks = requirement_blocks(cap)
    if not blocks:
        sys.exit(f"!! {cap} 没有 Requirement")
    for b in blocks:
        idx += 1
        kind = KINDS.get(idx, "business")
        title, rest = b.split("\n", 1)
        collected.append((kind, idx, title, note, rest.rstrip()))

if KINDS:
    missing = [i for i in range(kind_offset + 1, idx + 1) if i not in KINDS]
    if missing:
        sys.exit(f"!! 分类文件缺序号 {missing[:10]}（族内 {kind_offset + 1}..{idx}）")

# business 在前、impl 在后；各自保持来源顺序
collected.sort(key=lambda x: (x[0] != "business", x[1]))

n_business = sum(1 for c in collected if c[0] == "business")  # 本族内 business 数
for kind, _, title, note, rest in collected:
    lines.append(title)
    lines.append("")
    lines.append(note)
    if kind == "impl":
        lines.append("<!-- kind: impl -->")
    lines.append("")
    lines.append(rest)
    lines.append("")

total_req = idx - kind_offset

body = "\n".join(lines).rstrip() + "\n"

if out_dir:
    os.makedirs(out_dir, exist_ok=True)
    dest = os.path.join(out_dir, f"{fam}.md")
    open(dest, "w").write(body)
    print(f"{dest}: {fam} <- {len(caps)} capability / {total_req} Requirement（business {n_business} / impl {total_req - n_business}）/ {len(body.splitlines())} 行", file=sys.stderr)

if delta_out:
    # 族名与某个旧 capability 重名时视为「就地扩容」：该 capability 保留，
    # 只把其余成员的 Requirement 以 ADDED 追加进去，不写它自己的 REMOVED。
    reuse = fam in caps
    added_caps = [c for c in caps if c != fam] if reuse else caps
    if reuse:
        print(f"注意：{fam} 与旧 capability 同名 → 就地扩容，ADDED 只含 {len(added_caps)} 个成员", file=sys.stderr)
    fam_dir = os.path.join(delta_out, fam)
    os.makedirs(fam_dir, exist_ok=True)
    purpose = PURPOSE.get(fam, "")
    added = ["## Purpose", "", purpose, "", "## ADDED Requirements", ""]
    for kind, _, title, note, rest in collected:
        if reuse and f"<!-- from: {fam} " in note:
            continue
        added.append(title)
        added.append("")
        added.append(note)
        if kind == "impl":
            added.append("<!-- kind: impl -->")
        added.append("")
        added.append(rest)
        added.append("")
    open(os.path.join(fam_dir, "spec.md"), "w").write("\n".join(added).rstrip() + "\n")

    for cap in added_caps:
        cap_dir = os.path.join(delta_out, cap)
        os.makedirs(cap_dir, exist_ok=True)
        rem = ["## REMOVED Requirements", ""]
        for b in requirement_blocks(cap):
            title = b.split("\n", 1)[0][len("### Requirement: "):].strip()
            rem += [f"### Requirement: {title}",
                    f"**Reason**: 合并到族级 capability `{fam}`（结构重构，契约内容不变）", "",
                    f"**Migration**: 见 `openspec/specs/{fam}/spec.md` 中同名 Requirement", ""]
        open(os.path.join(cap_dir, "spec.md"), "w").write("\n".join(rem).rstrip() + "\n")
    print(f"{delta_out}: 写入 {fam} 的 ADDED + {len(added_caps)} 个旧 capability 的 REMOVED", file=sys.stderr)
else:
    sys.stdout.write(body)
PY
