#!/usr/bin/env bash
# ui-verification-trace.smoke.sh — ui-verification-trace.sh 的冒烟自测（add-ui-verification-trace-gate 1.2）
#
# 临时目录 fixture 驱动（隔离真实仓库 changes；脚本只读，不污染仓库）。
# case 清单（对应 ui-verification-trace-gate spec 的 Scenario）：
#   ① major 实物齐全绿（标题式条目 + 截图在盘；输出条目计数 + 截图对账）
#   ② major 零痕迹红（无 ui-verification/ 目录 → 两条修复路径：建账 / 显式降档）
#   ③ major 空壳红（report.md 存在但零合法条目 → 建账格式契约）
#   ④ major 点名截图缺失红（输出缺失清单）
#   ⑤ 未覆盖无原因红（单条未覆盖、依据无原因 → 不计入合法 → 空壳红）
#   ⑥ minor 组件测试路径存在绿（UI_VERIFICATION_TRACE_REPO_ROOT 缝注入 fixture 仓库根）
#   ⑦ minor 裸关键词红（关键词字样无实物）
#   ⑧ minor agent-browser 建账绿（report.md ≥1 合法条目，无截图也过）
#   ⑨ none N/A 绿（一致性吃狗粮形态）
#   ⑩ 旧 schema 零检查绿（fail-open）
# 白盒附加（test-cases.md §白盒）：
#   ⑪ 实战表格形态 + 速记截图（| A<N> | **pass** | 依据 |；`X.png / -Y.png` 尾段兜底）→ 绿
#   ⑫ 头部注释区边界（proposal 正文段落 marker 不识别 → fail-open 绿）
#   ⑬ 依据行空行穿插（标题与依据行间空行 → 合法条目）
#   ⑭ 词表外判定（【搞定】不计入 → 零合法 → 空壳红）
#
# 用法：bash scripts/harness/ui-verification-trace.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRACE="$SCRIPT_DIR/ui-verification-trace.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0
out=""
rc=0

run_trace() { # $1=change 目录（其余参数原样透传）
	out="$(UI_VERIFICATION_TRACE_REPO_ROOT="${UIV_ROOT:-$REPO}" "$TRACE" "$@" 2>&1)"
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

# ---------- fixture 工具 ----------

REPO="$TMP/repo" # minor 档测试文件存在性判定的伪仓库根（seam 注入）
mkdir -p "$REPO"

# 新 schema change 骨架：$1=目录名 $2=ui-impact 值
mkchg() {
	local d="$TMP/$1"
	mkdir -p "$d"
	printf 'schema: syntopica-ui\ncreated: 2026-10-14\n' > "$d/.openspec.yaml"
	printf '<!-- ui-impact: %s -->\n\n## Why\nx\n' "$2" > "$d/proposal.md"
	printf '%s\n' "$d"
}

# ---------- ① major 实物齐全绿 ----------
C1="$(mkchg major-ok major)"
mkdir -p "$C1/ui-verification"
printf 'x' > "$C1/ui-verification/ke-shot-a-1440.png"
printf 'x' > "$C1/ui-verification/ke-shot-b-375.png"
cat > "$C1/ui-verification/report.md" <<'EOF'
# UI 验收报告：major-ok

### A-1 主链路走通【pass】

打开页面→点击→断言状态（证据 `ke-shot-a-1440.png`）

### A-2 窄屏路径【未覆盖】

数据条件不满足：本期无建议数据，待有数据环境补验（`ke-shot-b-375.png` 为既有基线）
EOF
run_trace "$C1"
check 0 "① major 实物齐全绿（条目计数+截图对账输出）" "建账 2 条" "pass 1" "未覆盖 1" "全部在盘"

# ---------- ② major 零痕迹红 ----------
C2="$(mkchg major-zero major)"
run_trace "$C2"
check 1 "② major 零痕迹红（两条修复路径）" "零痕迹" "修复路径（二选一）" "显式降档"

# ---------- ③ major 空壳红 ----------
C3="$(mkchg major-shell major)"
mkdir -p "$C3/ui-verification"
printf '# 报告占位\n\n验收都做过了，没问题。\n' > "$C3/ui-verification/report.md"
run_trace "$C3"
check 1 "③ major 空壳红（建账格式契约）" "空壳文件" "建账格式契约" "词表封闭三值"

# ---------- ④ major 点名截图缺失红 ----------
C4="$(mkchg major-missing-shot major)"
mkdir -p "$C4/ui-verification"
cat > "$C4/ui-verification/report.md" <<'EOF'
### A-1 主链路【pass】

状态断言通过（证据 `ke-not-taken-1440.png`）
EOF
run_trace "$C4"
check 1 "④ major 点名截图缺失红（列缺失清单）" "1 张缺失" "ke-not-taken-1440.png"

# ---------- ⑤ 未覆盖无原因红 ----------
C5="$(mkchg major-cov-noreason major)"
mkdir -p "$C5/ui-verification"
cat > "$C5/ui-verification/report.md" <<'EOF'
### A-1 建议链路【未覆盖】

待补
EOF
run_trace "$C5"
check 1 "⑤ 未覆盖无原因红（不计入合法 → 零合法空壳红）" "未写明原因" "零合法判定条目"

# ---------- ⑥ minor 组件测试路径存在绿 ----------
C6="$(mkchg minor-comp minor)"
mkdir -p "$REPO/front/app/components"
printf 'x' > "$REPO/front/app/components/Foo.test.ts"
cat > "$C6/ui-design.md" <<'EOF'
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口

x

## Acceptance

组件测试 front/app/components/Foo.test.ts 覆盖交互与状态。
EOF
run_trace "$C6"
check 0 "⑥ minor 组件测试路径存在绿" "组件测试映射" "文件在盘"

# ---------- ⑦ minor 裸关键词红 ----------
C7="$(mkchg minor-bare minor)"
cat > "$C7/ui-design.md" <<'EOF'
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口

x

## 验收

组件测试覆盖；人工验证过。
EOF
run_trace "$C7"
check 1 "⑦ minor 裸关键词红（关键词无实物）" "裸关键词" "指名道姓挂实物"

# ---------- ⑦b minor 零映射红（验收节两词表都无） ----------
C7b="$(mkchg minor-nomap minor)"
cat > "$C7b/ui-design.md" <<'EOF'
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## Acceptance

待定。
EOF
run_trace "$C7b"
check 1 "⑦b minor 零映射红（未指名道姓）" "未指名道姓"

# ---------- ⑧ minor agent-browser 建账绿 ----------
C8="$(mkchg minor-manual minor)"
mkdir -p "$C8/ui-verification"
cat > "$C8/ui-design.md" <<'EOF'
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## Acceptance

agent-browser 走一遍主链路 + 人工双主题目检。
EOF
cat > "$C8/ui-verification/report.md" <<'EOF'
| A1 弹窗开合 | **pass** | 点触发按钮后弹窗出现，Esc 关闭（agent-browser 断言） |
EOF
run_trace "$C8"
check 0 "⑧ minor agent-browser 建账绿（无截图也过）" "人工验证建账" "1 条合法判定条目"

# ---------- ⑨ none N/A 绿 ----------
C9="$(mkchg none-ok none)"
cat > "$C9/ui-design.md" <<'EOF'
<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

纯后端机制改动，无用户可见界面。
EOF
run_trace "$C9"
check 0 "⑨ none N/A 绿（一致性）" "N/A 一致性"

# ---------- ⑩ 旧 schema 零检查绿 ----------
C10="$TMP/legacy-chg"
mkdir -p "$C10"
printf 'schema: spec-driven\ncreated: 2026-01-01\n' > "$C10/.openspec.yaml"
run_trace "$C10"
check 0 "⑩ 旧 schema 零检查绿（fail-open）" "fail-open" "零检查不硬阻断"

# ---------- ⑪ 实战表格形态 + 速记截图 → 绿 ----------
C11="$(mkchg major-table major)"
mkdir -p "$C11/ui-verification"
for s in ke-reader-topics-1440.png ke-overview-lanes-star-1440.png ke-overview-lanes-star-zoom-1440.png ke-focus-detail-panel-1440.png; do
	printf 'x' > "$C11/ui-verification/$s"
done
cat > "$C11/ui-verification/report.md" <<'EOF'
## ② A1–A3 逐条判定

| # | 判定 | 依据 |
| --- | --- | --- |
| A1 日报确认建议 | **未覆盖（本期无建议，按 brief 转 A2）** | 7 个板块最新日报全部 idle、markers 空；遵守红线未触发生成类任务 |
| A2 无建议手标 | **pass** | 鼠标「标为关键」→「已标记」（PUT 200）；截 `ke-reader-topics-1440.png` |
| A3 总览与详情 | **pass** | ★ 徽标出现、原位详情正常；截 `ke-overview-lanes-star-1440/-star-zoom-1440.png`、`ke-focus-detail-panel-1440.png` |
EOF
run_trace "$C11"
check 0 "⑪ 实战表格形态 + 速记截图兜底绿" "建账 3 条" "pass 2" "未覆盖 1" "全部在盘"

# ---------- ⑫ 头部注释区边界：正文 marker 不识别 → fail-open ----------
C12="$TMP/body-marker"
mkdir -p "$C12"
printf 'schema: syntopica-ui\ncreated: 2026-10-14\n' > "$C12/.openspec.yaml"
printf '## Why\n头部之后才出现：<!-- ui-impact: major -->\n' > "$C12/proposal.md"
run_trace "$C12"
check 0 "⑫ 正文 marker 不识别 → fail-open 绿（声明归 ui-design-gate）" "fail-open"

# ---------- ⑬ 依据行空行穿插 → 合法条目 ----------
C13="$(mkchg major-blank major)"
mkdir -p "$C13/ui-verification"
cat > "$C13/ui-verification/report.md" <<'EOF'
### A-1 主链路【pass】


依据行与标题间有空行：断言通过。
EOF
run_trace "$C13"
check 0 "⑬ 依据行空行穿插 → 合法" "建账 1 条"

# ---------- ⑭ 词表外判定 → 不计入 → 空壳红 ----------
C14="$(mkchg major-badword major)"
mkdir -p "$C14/ui-verification"
cat > "$C14/ui-verification/report.md" <<'EOF'
### A-1 主链路【搞定】

看了一圈没问题。
EOF
run_trace "$C14"
check 1 "⑭ 词表外判定不合法 → 空壳红" "零合法判定条目" "词表封闭三值"

# ---------- ⑰ H1 回归钉：C locale 下未覆盖无原因仍红（字符计数 locale 无关） ----------
out="$(LC_ALL=C LANG=C UI_VERIFICATION_TRACE_REPO_ROOT="$REPO" "$TRACE" "$C5" 2>&1)"
rc=$?
check 1 "⑰ C locale 未覆盖无原因仍红（H1）" "未写明原因"

# ---------- ⑱ 表格式未覆盖无原因红（review M4①） ----------
C18="$(mkchg major-table-cov major)"
mkdir -p "$C18/ui-verification"
printf '| A1 建议链路 | **未覆盖** | 待补 |\n' > "$C18/ui-verification/report.md"
run_trace "$C18"
check 1 "⑱ 表格式未覆盖无原因 → 空壳红" "零合法判定条目"

# ---------- ⑲ none 档 marker 不一致红（review M4②） ----------
C19="$(mkchg none-mismatch none)"
cat > "$C19/ui-design.md" <<'EOF'
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason
不一致形态。
EOF
run_trace "$C19"
check 1 "⑲ none 档 ui-design marker(=minor)≠声明 → 红" "不一致"

# ---------- ⑳ 无杠 token 不吃子串（review M4③：shot.png ≠ screenshot.png） ----------
C20="$(mkchg major-substring major)"
mkdir -p "$C20/ui-verification"
printf 'x' > "$C20/ui-verification/screenshot.png"
printf '### A-1 主链路【pass】\n\n断言通过（证据 `shot.png`）\n' > "$C20/ui-verification/report.md"
run_trace "$C20"
check 1 "⑳ 点名 shot.png 而盘上仅 screenshot.png → 仍红（不放松到任意子串）" "1 张缺失" "shot.png"

# ---------- 用法与目录校验 ----------
run_trace
check 1 "⑮ 缺参 → 用法" "用法:"
run_trace "$TMP/no-such-dir"
check 1 "⑯ 目录不存在" "目录不存在"

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ ui-verification-trace.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ ui-verification-trace.smoke 失败（$failn/$((pass + failn))）"
exit 1
