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
# review 任务行前移判定（review-task-frontload 1.2）：
#   ⑦ complex 缺行 fail（fixture 含误伤样本：无勾选框的 review 字样行、无关键词的普通任务行）
#   ⑧ complex 含行 pass
#   ⑨ complex + review: waived pass
#   ⑩ simple 缺行 pass（向后兼容）
#   ⑪ complexity marker 在正文（非头部注释区）→ 不识别 → pass（marker 解析边界）
# UI 验收任务行判定（add-ui-verification-trace-gate 2.3）：
#   ⑫ major+新 schema 缺行 fail（误伤样本：无勾选框的 report.md 引用行不算任务行）
#   ⑬ major 含行 pass   ⑭ major+旧 schema 缺行 pass（向后兼容）
#   ⑮ none 档缺行 pass  ⑯ .openspec.yaml 缺席 → fail-open pass
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

# ⑦ complex 缺 review 任务行 → fail（fixture 含误伤样本：正文 review 字样无勾选框、普通任务行无关键词）
mkdir -p "$TMP/c7"
printf '<!-- complexity: complex -->\n<!-- ui-impact: none -->\n\n## Why\n本 change 需要 review，评审范围见下。\n' > "$TMP/c7/proposal.md"
printf '## 1. 任务\n- [ ] 1.1 干活\n正文引用：归档前过一遍 review 清单\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' > "$TMP/c7/tasks.md"
run_tail "$TMP/c7"
check 1 "⑦ complex 缺行（误伤样本不救场）→ 报缺 review 任务行" "review 任务行"

# ⑧ complex 含 review 任务行 → pass
mkdir -p "$TMP/c8"
printf '<!-- complexity: complex -->\n' > "$TMP/c8/proposal.md"
printf '## 1. 任务\n- [ ] 6.1 派子线程聚焦 review 并 review.md 建账\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' > "$TMP/c8/tasks.md"
run_tail "$TMP/c8"
check 0 "⑧ complex 含行 → 通过" "齐全"

# ⑨ complex + review: waived → 豁免 pass（tasks 无行也不拦）
mkdir -p "$TMP/c9"
printf '<!-- complexity: complex -->\n<!-- review: waived -->\n\n纯文字类 change，豁免理由见正文。\n' > "$TMP/c9/proposal.md"
printf '## 1. 任务\n- [ ] 1.1 改文档\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' > "$TMP/c9/tasks.md"
run_tail "$TMP/c9"
check 0 "⑨ complex+waived → 豁免通过" "齐全"

# ⑩ simple 缺 review 任务行 → pass（向后兼容）
mkdir -p "$TMP/c10"
printf '<!-- complexity: simple -->\n' > "$TMP/c10/proposal.md"
printf '## 1. 任务\n- [ ] 1.1 干活\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' > "$TMP/c10/tasks.md"
run_tail "$TMP/c10"
check 0 "⑩ simple 缺行 → 不受影响" "齐全"

# ⑪ complexity marker 出现在正文（非头部注释区）→ 不识别 → 按 simple 处理 pass
mkdir -p "$TMP/c11"
printf '## Why\n头部之后才出现 marker：<!-- complexity: complex -->\n' > "$TMP/c11/proposal.md"
printf '## 1. 任务\n- [ ] 1.1 干活\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' > "$TMP/c11/tasks.md"
run_tail "$TMP/c11"
check 0 "⑪ 正文 marker 不识别 → 不受影响" "齐全"

# UI 验收任务行判定（add-ui-verification-trace-gate 2.3，三道防线第一道）：
# ⑫ major+新 schema 缺 UI 任务行 fail（误伤样本：无勾选框的 report.md 引用行不算任务行）
mk_tasks() { printf '## 1. 任务\n%s\n\n## 2. 测试\n\n## 3. 文档\n<!-- doc-impact: none(x) -->\n\n## 4. 验证\n' "$1"; }
mk_yamls() { printf 'schema: %s\ncreated: 2026-10-14\n' "$1"; }
mkdir -p "$TMP/c12"
printf '<!-- ui-impact: major -->\n\n## Why\n验收建账见 ui-verification/report.md 说明（无勾选框不算任务行）。\n' > "$TMP/c12/proposal.md"
mk_yamls syntopica-ui > "$TMP/c12/.openspec.yaml"
mk_tasks '- [ ] 1.1 干活' > "$TMP/c12/tasks.md"
run_tail "$TMP/c12"
check 1 "⑫ major+新 schema 缺 UI 任务行 → 报缺行" "UI 验收任务行"

# ⑬ major+新 schema 含 UI 任务行 → pass
mkdir -p "$TMP/c13"
printf '<!-- ui-impact: major -->\n' > "$TMP/c13/proposal.md"
mk_yamls syntopica-ui > "$TMP/c13/.openspec.yaml"
mk_tasks '- [x] 4.4 人工/agent-browser 验收：走 ui-design.md 主故事，判定建账 ui-verification/report.md' > "$TMP/c13/tasks.md"
run_tail "$TMP/c13"
check 0 "⑬ major 含 UI 任务行 → 通过" "齐全"

# ⑭ major+旧 schema 缺行 → 不受影响（旧 change 不卡死）
mkdir -p "$TMP/c14"
printf '<!-- ui-impact: major -->\n' > "$TMP/c14/proposal.md"
mk_yamls spec-driven > "$TMP/c14/.openspec.yaml"
mk_tasks '- [ ] 1.1 干活' > "$TMP/c14/tasks.md"
run_tail "$TMP/c14"
check 0 "⑭ major+旧 schema 缺行 → 不受影响" "齐全"

# ⑮ none/new schema 缺 UI 任务行 → 不受影响（仅 major 启用）
mkdir -p "$TMP/c15"
printf '<!-- ui-impact: none -->\n' > "$TMP/c15/proposal.md"
mk_yamls syntopica-ui > "$TMP/c15/.openspec.yaml"
mk_tasks '- [ ] 1.1 干活' > "$TMP/c15/tasks.md"
run_tail "$TMP/c15"
check 0 "⑮ none 档缺行 → 不受影响" "齐全"

# ⑯ major+新 schema，无 .openspec.yaml（读不到 schema 视作 legacy）→ 不受影响
mkdir -p "$TMP/c16"
printf '<!-- ui-impact: major -->\n' > "$TMP/c16/proposal.md"
mk_tasks '- [ ] 1.1 干活' > "$TMP/c16/tasks.md"
run_tail "$TMP/c16"
check 0 "⑯ schema 读不到 → fail-open 不受影响" "齐全"

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ check-tasks-tail.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ check-tasks-tail.smoke 失败（$failn/$((pass + failn))）"
exit 1
