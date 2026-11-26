#!/usr/bin/env bash
# 按 plan 把 spec 拆分成族级 capability（结构重构工具，不改语义）。
#
# 用法：
#   bash scripts/harness/split-spec-family.sh <plan.tsv> <delta-out> [--kind FILE] [--purpose FILE]
#
# plan.tsv 格式（每行一条，`*` = 该 capability 全取）：
#   族名<TAB>源 capability<TAB>Requirement 标题
#
# 行为：
#   - 目标族名 == 源 capability 名 → 就地扩容：同一个 delta 内写 `## ADDED`（新并入的）+ `## REMOVED`（搬走的）
#   - 目标族名 != 源 capability 名 → 新建 capability：写 ADDED；源 cap 写 REMOVED
#   - 未出现在 plan 里的源 cap 条目留在原位，不出现在 REMOVED 里（支持「部分搬运」）
#   - --kind FILE：每行 `<Requirement标题><TAB>business|impl`，impl 排到族末尾并标 `<!-- kind: impl -->`
#   - --purpose FILE：每行 `<族名><TAB>Purpose 文案`（新建族必需，否则归档后落 TBD 占位）
#
# 不判断：切分方案由人定（plan），脚本只保证「条数守恒、内容不丢、顺序可复现」。
set -euo pipefail

ROOT="${SPLIT_SPEC_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
export SPLIT_SPEC_ROOT="$ROOT"

[ $# -ge 2 ] || { echo "用法: $0 <plan.tsv> <delta-out> [--kind FILE] [--purpose FILE]" >&2; exit 2; }
plan="$1"; out="$2"; shift 2
kind=""; purpose=""
while [ $# -gt 0 ]; do
	case "$1" in
		--kind) kind="${2:?}"; shift 2 ;;
		--purpose) purpose="${2:?}"; shift 2 ;;
		*) echo "未知参数: $1" >&2; exit 2 ;;
	esac
done

python3 - "$plan" "$out" "$kind" "$purpose" <<'PY'
import re, os, sys, glob, collections

root = os.environ["SPLIT_SPEC_ROOT"]
plan_path, out_dir, kind_file, purpose_file = sys.argv[1:5]

KINDS, PURPOSES = {}, {}
if kind_file and os.path.isfile(kind_file):
    for line in open(kind_file):
        line = line.rstrip("\n")
        if not line or line.startswith("#"):
            continue
        t, _, k = line.rpartition("\t")
        KINDS[t.strip()] = k.strip()
if purpose_file and os.path.isfile(purpose_file):
    for line in open(purpose_file):
        line = line.rstrip("\n")
        if not line or line.startswith("#"):
            continue
        fam, _, txt = line.partition("\t")
        PURPOSES[fam] = txt


def all_blocks(text):
    """main spec 全文的 Requirement 块（## Requirements 之后）。"""
    m = re.search(r"(?m)^## Requirements\s*$", text)
    body = text[m.end():] if m else text
    out = collections.OrderedDict()
    for p in re.split(r"(?m)^(?=### Requirement: )", body):
        if not p.startswith("### Requirement: "):
            continue
        t = p.split("\n", 1)[0][len("### Requirement: "):].strip()
        rest = re.split(r"(?m)^## ", p.split("\n", 1)[1], maxsplit=1)[0].rstrip() + "\n"
        out[t] = rest
    return out


def first_change(cap):
    for d in sorted(glob.glob(f"{root}/openspec/changes/archive/*/")):
        p = f"{d}specs/{cap}/spec.md"
        if os.path.isfile(p) and re.search(r"(?m)^## ADDED Requirements", open(p).read()):
            return os.path.basename(d.rstrip("/"))
    return ""


plan = collections.OrderedDict()
for line in open(plan_path):
    line = line.rstrip("\n")
    if not line or line.startswith("#"):
        continue
    fam, cap, title = line.split("\t")
    plan.setdefault(fam, []).append((cap, title))

src = {}
for fam, items in plan.items():
    for cap, _ in items:
        if cap not in src:
            src[cap] = all_blocks(open(f"{root}/openspec/specs/{cap}/spec.md").read())

taken = collections.defaultdict(set)
os.makedirs(out_dir, exist_ok=True)
total_added = total_removed = 0

for fam, items in plan.items():
    rows = []          # (kind, 序号, 标题, 来源注释, 正文)
    for cap, title in items:
        pool = src[cap]
        picked = list(pool) if title == "*" else [title]
        for t in picked:
            if t not in pool:
                sys.exit(f"!! {cap} 里没有 Requirement: {t}")
            ch = first_change(cap)
            note = f"<!-- from: {cap} ({ch[:10]} {ch[11:]}) -->" if ch else f"<!-- from: {cap} -->"
            rows.append((KINDS.get(t, "business"), len(rows), t, note, pool[t].rstrip()))
            taken[cap].add(t)

    # business 在前、impl 在后；各自保持 plan 顺序
    rows.sort(key=lambda r: (r[0] != "business", r[1]))

    lines = []
    if PURPOSES.get(fam):
        lines += ["## Purpose", "", PURPOSES[fam], ""]
    lines += ["## ADDED Requirements", ""]
    for kind, _, t, note, body in rows:
        lines += [f"### Requirement: {t}", "", note]
        if kind == "impl":
            lines.append("<!-- kind: impl -->")
        lines += ["", body, ""]

    os.makedirs(f"{out_dir}/{fam}", exist_ok=True)
    open(f"{out_dir}/{fam}/spec.md", "w").write("\n".join(lines).rstrip() + "\n")
    n_impl = sum(1 for r in rows if r[0] == "impl")
    print(f"{fam}: ADDED {len(rows)}（business {len(rows) - n_impl} / impl {n_impl}）")
    total_added += len(rows)

# REMOVED：搬走的条目
for cap, ts in taken.items():
    if not ts:
        continue
    remain = [t for t in src[cap] if t not in ts]
    rem = ["## REMOVED Requirements", ""]
    for t in src[cap]:
        if t not in ts:
            continue
        rem += [f"### Requirement: {t}",
                "**Reason**: 迁入族级 capability（结构重构，契约内容不变）", "",
                "**Migration**: 见对应族 spec 中同名 Requirement", ""]
    path = f"{out_dir}/{cap}/spec.md"
    if cap in plan:   # 就地扩容：与 ADDED 合并在同一文件
        open(path, "a").write("\n" + "\n".join(rem).rstrip() + "\n")
        print(f"{cap}: REMOVED {len(ts)}（就地扩容，保留 {len(remain)} 条）")
    else:
        os.makedirs(f"{out_dir}/{cap}", exist_ok=True)
        open(path, "w").write("\n".join(rem).rstrip() + "\n")
        print(f"{cap}: REMOVED {len(ts)} / 原 {len(src[cap])}"
              + ("（整体退役）" if not remain else f"（保留 {len(remain)} 条）"))
    total_removed += len(ts)

print(f"合计：ADDED {total_added} / REMOVED {total_removed}（守恒需 ADDED == REMOVED）", file=sys.stderr)
PY
