#!/usr/bin/env bash
# check-tasks-tail.smoke.sh — check-tasks-tail.sh 的 fixture 冒烟自测（harden-archive-readiness 1.1）
#
# 临时目录拼 tasks.md 形状，旁置真实被测脚本逐 case 断言退出码与关键输出。
# case 清单（含从 spec-gate.ts checkTasksMd 迁移的判定样本）：
#   ① 全过（三节乱序 + 标记）
#   ② 缺「测试」节（输出列「## N. 测试」）
#   ③ 缺 doc-impact 标记（输出列声明标记；-excuse 变体不算命中）
#   ④ 子节 `### 2.1 测试` 不误命中
#   ⑤ 正文引用「说明见 ## 2. 测试」不误命中
#   ⑥ tasks.md 不存在
#
# 用法：bash scripts/harness/check-tasks-tail.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAIL="$SCRIPT_DIR/check-tasks-tail.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0

run_tail() { # $1 = change 目录（含 tasks.md）；输出进 out，退出码进 rc
	out="$("$TAIL" "$1" 2>&1)"
	rc=$?
}

check() { # $1=期望退出码 $2=case 名 $3=输出应包含的关键字（可选）
	local want_rc="$1" name="$2" kw="${3:-}"
	if [ "$rc" -eq "$want_rc" ] && { [ -z "$kw" ] || printf '%s' "$out" | grep -Fq "$kw"; }; then
		pass=$((pass + 1))
	else
		failn=$((failn + 1))
		echo "✗ $name（rc=$rc 期望 $want_rc${kw:+，应含『$kw』}）"
		echo "$out" | sed 's/^/    /'
	fi
}

# ① 全过：三节乱序 + 标记
mkdir -p "$TMP/c1"
printf '## 1. 任务\n- [ ] 1.1 干活\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 2. 测试\n\n## 4. 验证\n' > "$TMP/c1/tasks.md"
run_tail "$TMP/c1"
check 0 "① 三节乱序+标记 → 通过" "齐全"

# ② 缺「测试」节
mkdir -p "$TMP/c2"
printf '## 1. 任务\n\n## 2. 文档\n<!-- doc-impact: none(x) -->\n\n## 3. 验证\n' > "$TMP/c2/tasks.md"
run_tail "$TMP/c2"
check 1 "② 缺测试节 → 报「## N. 测试」" "「## N. 测试」"

# ③ 缺 doc-impact 标记；且 -excuse 变体不算命中
mkdir -p "$TMP/c3"
printf '## 1. 测试\n\n## 2. 文档\n<!-- doc-impact-excuse: flow=x -->\n\n## 3. 验证\n' > "$TMP/c3/tasks.md"
run_tail "$TMP/c3"
check 1 "③ 缺标记（-excuse 不算）→ 报声明标记" "doc-impact:"

# ④ 子节不误命中：只有 `### 2.1 测试` 子节形态
mkdir -p "$TMP/c4"
printf '## 1. 任务\n### 2.1 测试小节\n\n## 2. 文档\n<!-- doc-impact: none(x) -->\n\n## 3. 验证\n' > "$TMP/c4/tasks.md"
run_tail "$TMP/c4"
check 1 "④ 子节 ### 2.1 测试 不误命中 → 报缺「测试」" "「## N. 测试」"

# ⑤ 正文引用不误命中：`## 2. 测试` 出现在行中（前有文字）
mkdir -p "$TMP/c5"
printf '## 1. 任务\n说明见 ## 2. 测试 的要求\n\n## 2. 文档\n<!-- doc-impact: none(x) -->\n\n## 3. 验证\n' > "$TMP/c5/tasks.md"
run_tail "$TMP/c5"
check 1 "⑤ 正文引用不误命中 → 报缺「测试」" "「## N. 测试」"

# ⑥ tasks.md 不存在
mkdir -p "$TMP/c6"
run_tail "$TMP/c6"
check 1 "⑥ tasks.md 不存在 → 读取失败" "不存在或不可读"

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ check-tasks-tail.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ check-tasks-tail.smoke 失败（$failn/$((pass + failn))）"
exit 1
