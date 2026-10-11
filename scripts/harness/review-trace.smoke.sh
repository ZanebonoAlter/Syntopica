#!/usr/bin/env bash
# review-trace.smoke.sh — review-trace.sh 的冒烟自测（add-review-trace-gate 1.2）
#
# 临时目录 fixture 驱动（隔离真实仓库 changes；脚本只读，不污染仓库）。
# case 清单（对应 review-trace-gate spec 的 Scenario）：
#   ① 建账通过（含条目计数输出；词表外条目不计入 → 覆盖「标准条目解析」+「词表外等级不合法」）
#   ② 审毕无发现声明通过（显式声明非空壳）
#   ③ waived 豁免通过（proposal 头部机器可读注释区 marker）
#   ④ 零痕迹红（review.md 与 waived 皆无 → 两条修复路径指引）
#   ⑤ 空壳红（有 review.md 但零合法条目/无发现声明）
#   ⑥ 词表外全红（等级 Critical + 状态「搞定」条目不合法）
#   ⑦ 声明位置不识别（marker 只出现在 proposal 正文段落 → 按零痕迹处理）
#   ⑧ 双痕迹（waived + review.md 并存 → 通过且输出注明双痕迹，以 waived 计）
#   ⑨ 用法与目录校验（缺参 / 目录不存在 → exit 1）
#
# 用法：bash scripts/harness/review-trace.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRACE="$SCRIPT_DIR/review-trace.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0
out=""
rc=0

run_trace() { # $@ = 传给被测脚本的参数
	out="$("$TRACE" "$@" 2>&1)"
	rc=$?
}

check() { # $1=期望退出码 $2=case 名，其后：应包含关键字（可多个）
	local want_rc="$1" name="$2"
	shift 2
	if [ "$rc" != "$want_rc" ]; then
		echo "  ✗ $name（期望 rc=$want_rc 实际 rc=$rc）"
		echo "    输出: $(printf '%s' "$out" | head -6)"
		failn=$((failn + 1))
		return
	fi
	local kw
	for kw in "$@"; do
		if ! printf '%s' "$out" | grep -qF -- "$kw"; then
			echo "  ✗ $name（rc 正确，但输出未包含: $kw）"
			echo "    输出: $(printf '%s' "$out" | head -20)"
			failn=$((failn + 1))
			return
		fi
	done
	echo "  ✓ $name"
	pass=$((pass + 1))
}

# ---------- fixture：临时 change 目录 ----------

mkchg() { # $1=change 目录名 → echo 目录路径
	mkdir -p "$TMP/$1"
	printf '%s\n' "$TMP/$1"
}

# ① 建账通过：2 条合法（High 已修复 + Low 误报丢弃带括号说明）+ 1 条词表外（Critical 不计入）
C1="$(mkchg ok-entries)"
cat >"$C1/review.md" <<'EOF'
# Review — ok-entries

## 意见清单

### R-1 2026-10-12【High】并发窗口内双重 upsert

- **状态**：已修复

### R-2 2026-10-12【Low】命名与既有惯例不一致

- **状态**：误报丢弃（与 feedback.sh 同构命名，维持）

### R-3 2026-10-12【Critical】词表外等级条目

- **状态**：已采纳
EOF
run_trace "$C1"
check 0 "① 建账通过：合法条目计数 2（词表外等级 Critical 不计入）" "✓ 通过" "建账 2 条"

# ② 审毕无发现声明通过（无问题条目 + 显式声明行）
C2="$(mkchg ok-nofinding)"
cat >"$C2/review.md" <<'EOF'
# Review — ok-nofinding

## 意见清单

- 审毕无发现（审查范围：proposal/design/tasks 三制品与 diff 全量走查，无阻塞问题）
EOF
run_trace "$C2"
check 0 "② 审毕无发现声明通过（显式声明行合法）" "✓ 通过" "审毕无发现"

# ③ waived 豁免通过（proposal 头部机器可读注释区 marker）
C3="$(mkchg ok-waived)"
cat >"$C3/proposal.md" <<'EOF'
<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- review: waived -->

## Why

纯文档 change，豁免理由写在正文。
EOF
run_trace "$C3"
check 0 "③ waived 豁免通过（头部 marker 识别）" "✓ 通过" "waived 显式豁免"

# ④ 零痕迹红：review.md 与 waived 皆无 → 两条修复路径指引
C4="$(mkchg zero-trace)"
cat >"$C4/proposal.md" <<'EOF'
<!-- ui-impact: minor -->

## Why

x
EOF
run_trace "$C4"
check 1 "④ 零痕迹红：缺建账与豁免声明 + 两条修复路径" "✗ 未通过" "零痕迹" "修复路径（二选一）" "review.md" "waived"

# ⑤ 空壳红：有 review.md 但零合法条目/无发现声明
C5="$(mkchg empty-shell)"
cat >"$C5/review.md" <<'EOF'
# Review — empty-shell

（正文没有任何合法条目，也没有无发现声明）
EOF
run_trace "$C5"
check 1 "⑤ 空壳红：有文件零合法条目（有文件不等于审过）" "✗ 未通过" "空壳"

# ⑥ 词表外全红：等级 Critical + 状态「搞定」条目均不合法 → 全文件零合法条目
C6="$(mkchg bad-vocab)"
cat >"$C6/review.md" <<'EOF'
# Review — bad-vocab

### R-1 2026-10-12【Critical】词表外等级

- **状态**：搞定

### R-2 2026-10-12【Medium】合法等级配词表外状态

- **状态**：done
EOF
run_trace "$C6"
check 1 "⑥ 词表外红：Critical 等级与「搞定」状态条目不计入合法条目" "✗ 未通过"

# ⑦ 声明位置不识别：marker 只出现在 proposal 正文段落 → 按零痕迹处理
C7="$(mkchg marker-in-body)"
cat >"$C7/proposal.md" <<'EOF'
<!-- ui-impact: none -->

## Why

正文段落里写 <!-- review: waived --> 不属于头部机器可读注释区，不识别。
EOF
run_trace "$C7"
check 1 "⑦ 声明位置不识别：正文 marker 不算豁免（按零痕迹处理）" "✗ 未通过" "零痕迹"

# ⑧ 双痕迹：waived + review.md 建账并存 → 通过且注明双痕迹（以 waived 计）
C8="$(mkchg dual)"
cat >"$C8/proposal.md" <<'EOF'
<!-- review: waived -->

## Why

x
EOF
cat >"$C8/review.md" <<'EOF'
# Review — dual

### R-1 2026-10-12【Medium】双痕迹并存样例

- **状态**：已采纳
EOF
run_trace "$C8"
check 0 "⑧ 双痕迹通过：waived 优先 + 注明 review.md 建账（以 waived 计）" "waived 显式豁免" "双痕迹并存，以 waived 计" "建账 1 条"

# ⑨ 用法与目录校验
run_trace
check 1 "⑨a 缺参 → exit 1 + 用法" "用法"
run_trace "$TMP/no-such-change-xyz"
check 1 "⑨b 目录不存在 → exit 1" "change 目录不存在"

# ---------- POSIX bash 语法校验（任务 1.1 验证项） ----------
if bash -n "$TRACE" 2>/dev/null; then
	echo "  ✓ bash -n 语法校验通过"
	pass=$((pass + 1))
else
	echo "  ✗ bash -n 语法校验失败"
	failn=$((failn + 1))
fi

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ review-trace.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ review-trace.smoke 失败（$failn/$((pass + failn))）"
exit 1
