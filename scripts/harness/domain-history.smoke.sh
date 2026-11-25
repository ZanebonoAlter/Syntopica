#!/usr/bin/env bash
# domain-history.smoke.sh — domain-history.sh 的 fixture 冒烟自测（add-spec-domain-index）
#
# 用 DOMAIN_HISTORY_ROOT 指向临时造的仓库形状（specs/ + changes/archive/），
# 旁置真实被测脚本逐 case 断言输出与退出码。
#
# case 清单：
#   ① 单域 marker → 第 1 段列出该 capability
#   ② 多域 marker（逗号分隔）→ 两个域都能查到
#   ③ 未标 marker → 第 1 段提示「无」
#   ④ --list → 15 个域全列出
#   ⑤ 未知域 → 打印可用域清单，退出码 0（不打断自动化）
#   ⑥ 无参数 → 打印用法，退出码 0
#   ⑦ 两跳推导 → archive change 出现在本域时间线
#   ⑧ REMOVED / MODIFIED 提取 → 第 4/5 段列出 Requirement 标题
#   ⑨ 未标 domain 的 orphan delta → 不进第 4/5 段，但第 2 段标注「含孤儿」
#   ⑩ 精确匹配 → semantic-board 不因 board 前缀误命中 topic-graph
#
# 用法：bash scripts/harness/domain-history.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DH="$SCRIPT_DIR/domain-history.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0

check() { # $1=期望退出码 $2=case 名 $3=输出应包含（grep -F，可空）
	local want_rc="$1" name="$2" kw="${3:-}"
	if [ "$rc" -ne "$want_rc" ]; then
		echo "✗ $name：退出码 $rc，期望 $want_rc"
		echo "$out" | sed 's/^/    /' | head -5
		failn=$((failn + 1))
		return
	fi
	if [ -n "$kw" ] && ! printf '%s' "$out" | grep -qF -- "$kw"; then
		echo "✗ $name：输出缺少「$kw」"
		echo "$out" | sed 's/^/    /' | head -8
		failn=$((failn + 1))
		return
	fi
	echo "✓ $name"
	pass=$((pass + 1))
}

check_absent() { # $1=case 名 $2=不应出现的字符串
	local name="$1" kw="$2"
	if printf '%s' "$out" | grep -qF -- "$kw"; then
		echo "✗ $name：输出不应出现「$kw」"
		failn=$((failn + 1))
		return
	fi
	echo "✓ $name"
	pass=$((pass + 1))
}

# ── fixture：一个最小仓库形状 ────────────────────────────
R="$TMP/repo"
mkdir -p "$R/openspec/specs"/{alpha,beta,gamma}
mkdir -p "$R/openspec/changes/archive/2026-01-02-add-alpha"/specs/alpha
mkdir -p "$R/openspec/changes/archive/2026-01-03-change-alpha"/specs/{alpha,retired-x}
mkdir -p "$R/openspec/changes/archive/2026-01-04-orphan"/specs/retired-cap

cat >"$R/openspec/specs/alpha/spec.md" <<'EOF'
<!-- domain: semantic-board -->
# alpha Specification

## Purpose
版块相关。
EOF

cat >"$R/openspec/specs/beta/spec.md" <<'EOF'
<!-- domain: topic-graph, daily-report -->
# beta Specification

## Purpose
话题与日报两用。
EOF

cat >"$R/openspec/specs/gamma/spec.md" <<'EOF'
# gamma Specification

## Purpose
没有 marker。
EOF

cat >"$R/openspec/changes/archive/2026-01-02-add-alpha/specs/alpha/spec.md" <<'EOF'
## ADDED Requirements

### Requirement: 新建版块流程
场景。

## Modified Requirements

### Requirement: 大小写变体的节头
值为 M 的 Modified 也要被扫到（R-8）。
EOF

cat >"$R/openspec/changes/archive/2026-01-03-change-alpha/specs/alpha/spec.md" <<'EOF'
## MODIFIED Requirements

### Requirement: 版块排序口径
改成热度降序。

## REMOVED Requirements

### Requirement: 旧版块拖拽
已退役。
EOF

cat >"$R/openspec/changes/archive/2026-01-04-orphan/specs/retired-cap/spec.md" <<'EOF'
## ADDED Requirements

### Requirement: 已退役能力
EOF

# 真实孤儿：命中本域的 change 里挂一个无 domain 的 delta（R-1：smoke 不得假阳性）
cat >"$R/openspec/changes/archive/2026-01-03-change-alpha/specs/retired-x/spec.md" <<'EOF'
## REMOVED Requirements

### Requirement: 已退役的旧能力
EOF

run() { out="$(DOMAIN_HISTORY_ROOT="$R" bash "$DH" "$@" 2>&1)"; rc=$?; }

# ── ① 单域 marker ────────────────────────────────────────
run semantic-board
check 0 "① 单域 marker 命中" "alpha"

# ── ② 多域 marker ────────────────────────────────────────
run topic-graph
check 0 "② 多域 marker 第一域" "beta"
run daily-report
check 0 "② 多域 marker 第二域" "beta"

# ── ③ 未标 marker ────────────────────────────────────────
run "dev"
check 0 "③ 未标 marker 时第 1 段提示无" "（无——该域还没有任何 capability 声明它"

# ── ④ --list ─────────────────────────────────────────────
run --list
check 0 "④ --list 列业务域" "semantic-board"
out2="$out"; out="$(printf '%s\n' "$out2" | grep -cE '^  [a-z-]+$')"
check 0 "④ --list 列 15 个域" "15"

# ── ⑤ 未知域 ─────────────────────────────────────────────
run bogus-domain
check 0 "⑤ 未知域退出码 0" "未知域名"
check 0 "⑤ 未知域列出可用域" "semantic-board"

# ── ⑥ 无参数 ─────────────────────────────────────────────
run
check 0 "⑥ 无参数打印用法" "用法"

# ── ⑦ 两跳推导 ───────────────────────────────────────────
run semantic-board
check 0 "⑦ 时间线含两次 change" "2 个 archived change 碰过本域"

# ── ⑧ REMOVED / MODIFIED 提取 ────────────────────────────
check 0 "⑧ 第 4 段列出 REMOVED 标题" "旧版块拖拽"
check 0 "⑧ 第 5 段列出 MODIFIED 标题" "版块排序口径"

# ── ⑨ orphan delta：按行断言，不靠全输出 grep（R-1 假阳性教训）──
line_with="$(printf '%s' "$out" | grep 'change-alpha' | head -1)"
line_without="$(printf '%s' "$out" | grep ' add-alpha ' | head -1)"
case "$line_with" in
*"含孤儿"*) echo "✓ ⑨ 含孤儿的 change 带标记"; pass=$((pass + 1)) ;;
*) echo "✗ ⑨ 含孤儿的 change 未带标记：$line_with"; failn=$((failn + 1)) ;;
esac
case "$line_without" in
*"含孤儿"*) echo "✗ ⑨ 无孤儿的 change 被误标：$line_without"; failn=$((failn + 1)) ;;
*) echo "✓ ⑨ 无孤儿的 change 不带标记"; pass=$((pass + 1)) ;;
esac
check 0 "⑨ 第 6 段列出孤儿 capability" "retired-x"
check_absent "⑨ 孤儿 REMOVED 标题不进第 4 段" "已退役的旧能力"
check 0 "⑨ 第 6 段计数" "个 archived change 含孤儿 delta"

# ── ⑪ 节头大小写不敏感（R-8）──
check 0 "⑪ Modified（大写 M）也被扫到" "大小写变体的节头"

# ── ⑨ orphan delta ───────────────────────────────────────
# （旧的全输出 grep 断言已由上方按行版本取代 —— R-1：全输出 grep 会被其他行巧合通过）

# ── ⑩ 精确匹配 ───────────────────────────────────────────
run topic-graph
check_absent "⑩ topic-graph 不误命中 semantic-board 的 alpha" "alpha"

echo
if [ "$failn" -gt 0 ]; then
	echo "✗ 失败 $failn 项 / 通过 $pass 项"
	exit 1
fi
echo "✓ 全部 $pass 项通过"
exit 0
