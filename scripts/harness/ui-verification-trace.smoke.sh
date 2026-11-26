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

# ---------- harden-ui-delivery-acceptance D01–D24：真实 PNG + v1 合同 ----------
# 完整的单色 PNG（含 IDAT/IEND 和 CRC），不是仅伪造尺寸的文件名/头。
png_fixture() {
	case "$2" in
		1440) printf '%s' 'iVBORw0KGgoAAAANSUhEUgAABaAAAAOEAQAAAAB/Nqt/AAAAtUlEQVR4nO3BMQEAAADCoPVPbQsvoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAhwF8cgABzwpMvAAAAABJRU5ErkJggg==' ;;
		2560) printf '%s' 'iVBORw0KGgoAAAANSUhEUgAACgAAAAWgAQAAAAC6eydyAAAB10lEQVR4nO3BMQEAAADCoPVPbQ0PoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAIB3Aw4JAAGh8xlHAAAAAElFTkSuQmCC' ;;
		1280) printf '%s' 'iVBORw0KGgoAAAANSUhEUgAABQAAAAJ5AQAAAADZFaDdAAAAd0lEQVR4nO3BAQEAAACAkP6v7ggKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABojigAAbqZ3ZMAAAAASUVORK5CYII=' ;;
	esac | base64 -d > "$1"
}
v1_reset() {
	V="$(mkchg delivery major)"
	mkdir -p "$V/ui-verification/子目录"
	png_fixture "$V/ui-verification/a.png" 1440
	png_fixture "$V/ui-verification/b.png" 2560
	cp "$V/ui-verification/a.png" "$V/ui-verification/子目录/有 空格.png"
	cp "$V/ui-verification/b.png" "$V/ui-verification/子目录/宽屏.png"
	cat > "$V/ui-design.md" <<'EOF'
<!-- ui-impact: major -->

<!-- ui-acceptance-contract: v1 -->

## Acceptance
### Delivery Contract
| A-ID | 必验故事与打开状态 | 必验视口 |
| --- | --- | --- |
| A-55 | 打开选择面板并返回正文 | 1440x900,2560x1440 |
EOF
	printf '| A55 故事 | **pass** | 打开态断言通过 a.png b.png |\n' > "$V/ui-verification/report.md"
}
vcheck() { run_trace "$V" --delivery; check "$@"; }
v1_reset
vcheck 0 'D02/D18 v1逐故事两视口通过' '交付结果' 'A-55'
first="$out"; vcheck 0 '幂等：同fixture重复通过' '交付结果'
[ "$first" = "$out" ] || { echo '✗ 幂等输出不一致'; failn=$((failn + 1)); }
printf '| A9 旁支 | **fail** | 转挂 existing-change，问题仍在 a.png |\n| A10 旁支 | **未覆盖** | 环境缺少真实建议数据 |\n' >> "$V/ui-verification/report.md"
mkdir -p "$TMP/existing-change"
run_trace "$V"; check 0 'D01 默认只检查证据不阻断fail' '仅证据' 'fail 1'
vcheck 1 'D03 非必验最新fail不能转挂洗绿' 'A-9' 'fail'
v1_reset
printf '| A55 故事 | **未覆盖** | 环境缺少真实建议数据 |\n' > "$V/ui-verification/report.md"
vcheck 1 'D04 核心未覆盖' 'A-55' '未覆盖'
printf '| A1 别的故事 | pass | 检查状态通过 a.png b.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D04 必验从未记录' 'A-55' '缺失'
v1_reset
printf '| A9 旁支 | 未覆盖 | 环境缺少真实建议数据 |\n' >> "$V/ui-verification/report.md"
vcheck 0 'D05 非必验未覆盖逐条提醒' 'A-9' '环境缺少真实建议数据'
v1_reset
cp "$V/ui-verification/a.png" "$V/ui-verification/new.png"
printf '| A55 故事 | fail | 面板遮挡 a.png |\n### A-55 同场景【pass】\n\n复验：面板可见且点击命中 new.png b.png\n' > "$V/ui-verification/report.md"
vcheck 0 'D06 混排同号复验保留历史' '建账 2 条' 'fail 1'
printf '| A55 故事 | 未覆盖 | 环境缺少真实建议数据 |\n| A-55 故事 | pass | 面板可见且点击命中 new.png b.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D07 无复验标识不解除' 'A-55' '复验'
printf '| A55 故事 | fail | 面板遮挡 a.png b.png |\n| A-55 故事 | pass | 复验：面板可见且点击命中 a.png b.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D08 旧图不解除' 'A-55' '新截图'
sed -i 's/复验：面板可见且点击命中 a.png b.png/复验 a.png b.png/' "$V/ui-verification/report.md"
vcheck 1 'D08 只有复验和图无实际结果' 'A-55' '实际结果'
for bad in '| A55 故事 | 搞定 | 状态通过 a.png |' '| A55 故事 | pass | |' '| A? 故事 | pass | 状态通过 a.png |' '### A-0 故事【pass】' '| a55 故事 | pass | 状态通过 a.png |'; do
	v1_reset; printf '%s\n' "$bad" >> "$V/ui-verification/report.md"
	vcheck 1 "D09 非法候选阻断：$bad" '非法条目'
	done
v1_reset
cat >> "$V/ui-verification/report.md" <<'EOF'
> A43 旧失败仍保留说明
- A55 是说明，不是新记录
A43 和 A55 的历史说明散文
| A-ID | 判定 | 依据 |
| --- | --- | --- |
```markdown
| A? | 搞定 | 假依据 |
### A55 假场景【fail】
```
EOF
vcheck 0 'D09b/c 引用列表散文表头围栏都不是条目' '建账 1 条'
printf '```markdown\n| A55 假场景 | pass | 状态通过 a.png b.png |\n```\n| A1 别的故事 | pass | 状态通过 a.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D09c 代码里的pass不能满足必验' 'A-55' '缺失'
for row in '' '| A55 | 重复 | 1440x900 |' '| A-0 | 故事 | 1440x900 |' '| A--1 | 故事 | 1440x900 |' '| | 故事 | 1440x900 |' '| A55 | | 1440x900 |' '| A55 | 故事 | 0x900 |' '| A55 | 故事 | -1x900 |' '| A55 | 故事 | 1440 |' '| A55 | 故事 | 1x2x3 |' '| A55 | 故事 | 1440x900提示 |' '| A55 | 故事 | |' '| A55 | 故事 | 1440x900, |'; do
	v1_reset
	if [[ "$row" == *重复* ]]; then printf '%s\n' "$row" >> "$V/ui-design.md"; else sed -i '/^| A-55 /d' "$V/ui-design.md"; printf '%s\n' "$row" >> "$V/ui-design.md"; fi
	vcheck 1 "D10–D12 合同坏行：$row" '合同'
done
v1_reset; printf '\n### Delivery Contract\n' >> "$V/ui-design.md"
vcheck 1 'D10 重复合同节' '唯一'
v1_reset; sed -i '/### Delivery Contract/d' "$V/ui-design.md"
vcheck 1 'D10 缺合同节' 'Delivery Contract'
for marker in '<!-- ui-acceptance-contract: v2 -->' '<!-- ui-acceptance-contract: -->'; do
	v1_reset; sed -i "s/<!-- ui-acceptance-contract: v1 -->/$marker/" "$V/ui-design.md"
	vcheck 1 "D13 非法版本：$marker" '合同 marker'
done
v1_reset; printf '\n<!-- ui-acceptance-contract: v1 -->\n' >> "$V/ui-design.md"
vcheck 1 'D13 重复且正文marker' '合同 marker'
v1_reset; sed -i '/ui-acceptance-contract/d' "$V/ui-design.md"; printf '\n> 引用说明之后\n<!-- ui-acceptance-contract: v1 -->\n' >> "$V/ui-design.md"
vcheck 1 'D13 老major正文marker不能降级' '位置'
v1_reset
cat >> "$V/ui-design.md" <<'EOF'
`<!-- ui-acceptance-contract: v2 -->` 是行内示例
~~~markdown
<!-- ui-acceptance-contract: v2 -->
~~~
EOF
vcheck 0 'D13b 开头可穿空行，围栏行内示例忽略' '交付结果'
for date in 2026-10-26 2026-10-27 2026-10-28 missing 2026-02-30 garbage; do
	v1_reset; sed -i '/ui-acceptance-contract/d' "$V/ui-design.md"
	cat >> "$V/ui-design.md" <<'EOF'
`<!-- ui-acceptance-contract: v1 -->`
```
<!-- ui-acceptance-contract: v1 -->
```
EOF
	printf 'schema: syntopica-ui\ncreated: %s\n' "$date" > "$V/.openspec.yaml"
	if [ "$date" = missing ]; then sed -i '/created/d' "$V/.openspec.yaml"; fi
	if [ "$date" = 2026-10-26 ]; then vcheck 0 'D14 老major明确降级' '仅证据齐备，未迁移交付合同'; else vcheck 1 "D14/D16 新major或坏日期：$date" '合同'; fi
done
v1_reset; printf '| A55 故事 | fail | 面板遮挡 a.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D15 老日期显式v1仍查fail' 'A-55' 'fail'
for c in "$C6" "$C8" "$C9" "$C10"; do run_trace "$c" --delivery; check 0 "D17 兼容非major/旧schema：$c"; done
v1_reset
printf '| A55 故事 | pass | 状态通过 a.png |\n| A57 另一页 | pass | 状态通过 b.png |\n清单 b.png\n' > "$V/ui-verification/report.md"
vcheck 1 'D19 其他A/全局图不能借用' 'A-55' '2560x1440'
printf '| A55 故事 | pass | 状态通过 a.png b.png |\n| A55 故事 | pass | 最新通过 a.png |\n' > "$V/ui-verification/report.md"
vcheck 1 'D19 旧记录图不能借用' 'A-55' '2560x1440'
v1_reset; png_fixture "$V/ui-verification/a.png" 1280
vcheck 1 'D20 文件名不等于真实尺寸' 'A-55' '1440x900'
for kind in missing text signature truncated zero; do
	v1_reset
	case "$kind" in
		missing) rm "$V/ui-verification/a.png" ;;
		text) printf 'not PNG' > "$V/ui-verification/a.png" ;;
		signature) printf 'bad signxxxxIHDR12345678' > "$V/ui-verification/a.png" ;;
		truncated) head -c 20 "$V/ui-verification/b.png" > "$V/ui-verification/a.png" ;;
		zero) printf '\211PNG\r\n\032\n\000\000\000\015IHDR\000\000\000\000\000\000\000\000\001\000\000\000\000\000\000\000\000' > "$V/ui-verification/a.png" ;;
	esac
	vcheck 1 "D21 缺失或坏PNG：$kind" 'a.png'
done
v1_reset
printf '| A55 故事 | pass | 打开态通过 [普通](a.png) [宽屏](子目录/宽屏.png) [空格](<子目录/有 空格.png>) |\n' > "$V/ui-verification/report.md"
vcheck 0 'D22 Markdown中文子目录空格目标' '交付结果'
printf '| A55 故事 | pass | 打开态通过 子目录/宽屏.png a.png |\n' > "$V/ui-verification/report.md"
vcheck 0 'D22 裸相对子目录中文' '交付结果'
cp "$V/ui-verification/a.png" "$TMP/outside.png"
for path in '子目录/不存在.png' '../outside.png' "$TMP/outside.png" 'escape.png'; do
	v1_reset; ln -sf "$TMP/outside.png" "$V/ui-verification/escape.png"
	printf '| A55 故事 | pass | 打开态通过 a.png b.png [%s](<%s>) |\n' "$path" "$path" > "$V/ui-verification/report.md"
	vcheck 1 "D22/D23 非法路径：$path" "$path"
done
v1_reset; printf '| A55 故事 | fail | 复验后仍遮挡 a.png |\n' >> "$V/ui-verification/report.md"
vcheck 1 'D24 最后再次fail仍红' 'A-55' 'fail'

echo ""
if [ "$failn" -eq 0 ]; then
	echo "✓ ui-verification-trace.smoke 全过（$pass case）"
	exit 0
fi
echo "✗ ui-verification-trace.smoke 失败（$failn/$((pass + failn))）"
exit 1
