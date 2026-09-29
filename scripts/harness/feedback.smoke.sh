#!/usr/bin/env bash
# feedback.smoke.sh — scripts/harness/feedback.sh 白盒冒烟（change: add-change-lifecycle-retro 8.1）
#
# 覆盖 test-cases 与 spec Scenario（human-feedback 反馈建账）：
#   建账模板创建与标签格式（FB-1/待处理）、编号自增（存量 FB-10 → 新 FB-11）、
#   插入位置（处置记录节前，其余内容不变）、词表外归因/类型零写入、verify 三态
#   （待处理 exit 1 / 全处置 exit 0 / 无文件 exit 0）、格式异常（状态行缺失与词表外
#   状态）视同未处置、已归档拒绝建账、change 不存在拒绝、menu 词表、用法错误。
# 全部 mktemp 临时 fixture（FEEDBACK_ROOT 注入），零真实仓库文件接触。
#
# 用法：bash scripts/harness/feedback.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE="$SCRIPT_DIR/feedback.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/feedback-smoke.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

ROOT="$TMP/repo"
CH="$ROOT/openspec/changes"
mkdir -p "$CH/fresh-change" "$CH/has-fb" "$CH/archived-x" "$CH/verify-open" "$CH/verify-done" "$CH/verify-bad" "$CH/no-file"
mkdir -p "$CH/archive/2026-09-20-archived-x"

# 存量 fixture：FB-1~10 + 处置记录节（对齐 ai-provider-model-overhaul 首例结构）
{
	for i in 1 2 3 4 5 6 7 8 9 10; do
		printf '### FB-%s 2026-09-2%d【产品|frontend】存量条目 %s\n\n- **状态**：已消化\n- **原文要点**：存量 %s\n\n' "$i" "$((i % 10))" "$i" "$i"
	done
	printf '## 处置记录\n\n| 日期 | 条目 | 动作 | 结果 |\n|---|---|---|---|\n| 2026-09-29 | FB-1~10 | 存量 | 已消化 |\n'
} >"$CH/has-fb/human-feedback.md"

pass=0
fail=0
ok() { printf '  [OK] %s\n' "$1"; pass=$((pass + 1)); }
bad() { printf '  [FAIL] %s\n' "$1" >&2; fail=$((fail + 1)); }

expect_rc() { # $1=期望退出码 $2=描述；RC/OUT 由调用方先跑
	[ "$RC" = "$1" ] && ok "$2" || bad "$2（期望 exit $1，实得 $RC；输出：$(printf '%s' "$OUT" | head -2 | tr '\n' ' ')）"
}
expect_contains() { # $1=子串 $2=描述
	printf '%s' "$OUT" | grep -qF -- "$1" && ok "$2" || bad "$2（输出未含：$1）"
}
expect_file_contains() { # $1=文件 $2=子串 $3=描述
	grep -qF -- "$2" "$1" 2>/dev/null && ok "$3" || bad "$3（文件未含：$2）"
}

echo "== feedback.smoke =="

# ---------- 1. add 建账：模板创建 + FB-1 + 待处理 + 标签格式（Scenario: add 建账） ----------
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add fresh-change --layer frontend --type 产品 "原型布局返工，列表拥挤" 2>&1)"
RC=$?
expect_rc 0 "S1a 新 change 建账 → exit 0"
expect_contains "已记入 FB-1（待处理，【产品|frontend】）" "S1b 确认行含编号/状态/标签"
FBF="$CH/fresh-change/human-feedback.md"
[ -f "$FBF" ] && ok "S1c 文件按模板创建" || bad "S1c 文件未创建"
expect_file_contains "$FBF" "# Human Feedback — fresh-change" "S1d 模板头部含定位标题"
expect_file_contains "$FBF" "③ 归档对账（archive-readiness 顺带 feedback.sh verify" "S1e 头部含消费方定位说明"
expect_file_contains "$FBF" "### FB-1 20" "S1f 条目标题行 FB-1"
expect_file_contains "$FBF" "【产品|frontend】原型布局返工，列表拥挤" "S1g 标题行含【类型|归因】标签与要点"
expect_file_contains "$FBF" "- **状态**：待处理" "S1h 状态初始化待处理"
expect_file_contains "$FBF" "- **原文要点**：原型布局返工，列表拥挤" "S1i 原文要点落条目体"

# ---------- 2. 编号自增 + 插入位置（Scenario: add 建账编号自增） ----------
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add fresh-change --layer process --type 流程 "审批流程太绕" 2>&1)"
RC=$?
expect_rc 0 "S2a 第二条 → exit 0"
expect_contains "已记入 FB-2" "S2b 编号自增 FB-2"
# 存量 FB-1~10 + 处置记录节 → 新条目 FB-11 插处置记录前，其余内容不变
_md5_before="$(md5sum "$CH/has-fb/human-feedback.md" | cut -d' ' -f1)"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add has-fb --layer backend --type 产品 "默认超时太短" 2>&1)"
RC=$?
expect_rc 0 "S2c 存量 FB-10 建账 → exit 0"
expect_contains "已记入 FB-11" "S2d 编号取现有最大自增 → FB-11"
expect_file_contains "$CH/has-fb/human-feedback.md" "### FB-11 20" "S2e 新条目标题行 FB-11"
expect_file_contains "$CH/has-fb/human-feedback.md" "【产品|backend】默认超时太短" "S2f 标签【产品|backend】"
expect_file_contains "$CH/has-fb/human-feedback.md" "## 处置记录" "S2g 处置记录节仍在（未覆盖）"
expect_file_contains "$CH/has-fb/human-feedback.md" "### FB-10 2026-09-20" "S2h 存量 FB-10 条目不变"
# FB-11 位于处置记录节之前（插入位置：反馈清单尾部）
if awk '/^### FB-11/{a=NR} /^## 处置记录/{b=NR} END{exit !(a>0 && b>0 && a<b)}' "$CH/has-fb/human-feedback.md"; then
	ok "S2i 新条目插在处置记录节之前"
else
	bad "S2i 新条目未插在处置记录节之前"
fi
[ "$(md5sum "$CH/has-fb/human-feedback.md" | cut -d' ' -f1)" != "$_md5_before" ] && ok "S2j 存量文件已被追加（对照 S3 的零写入）" || bad "S2j 存量文件未变化"

# ---------- 3. 词表外值零写入（Scenario: 词表外值零写入） ----------
_md5_before="$(md5sum "$CH/has-fb/human-feedback.md" | cut -d' ' -f1)"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add has-fb --layer ui --type 产品 "词表外归因" 2>&1)"
RC=$?
expect_rc 1 "S3a 归因词表外（ui）→ exit 1"
expect_contains "归因词表外" "S3b 提示归因词表外"
expect_contains "menu" "S3c 提示 menu 查看词表（选择题不是回忆题）"
[ "$(md5sum "$CH/has-fb/human-feedback.md" | cut -d' ' -f1)" = "$_md5_before" ] && ok "S3d 零写入（md5 不变）" || bad "S3d 词表外却写了文件"

OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add has-fb --layer frontend --type 其它 "词表外类型" 2>&1)"
RC=$?
expect_rc 1 "S3e 类型词表外（其它）→ exit 1"
expect_contains "类型词表外" "S3f 提示类型词表外"
[ "$(md5sum "$CH/has-fb/human-feedback.md" | cut -d' ' -f1)" = "$_md5_before" ] && ok "S3g 零写入" || bad "S3g 类型词表外却写了文件"

# ---------- 4. verify 三态（Scenario: verify 拦截未处置条目） ----------
# 待处理/处理中 → exit 1 + 处置提醒
cat >"$CH/verify-open/human-feedback.md" <<'EOF'
# Human Feedback — verify-open

## 反馈清单

### FB-1 2026-09-20【产品|frontend】待处置反馈

- **状态**：待处理

### FB-2 2026-09-21【流程|process】进行中反馈

- **状态**：处理中（规范修订中）

### FB-3 2026-09-22【产品|backend】已消化反馈

- **状态**：已消化
EOF
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" verify "$CH/verify-open" 2>&1)"
RC=$?
expect_rc 1 "S4a 存在待处理/处理中 → exit 1"
expect_contains "已处置 1 / 未处置 2 / 格式异常 0" "S4b 汇总计数可复算"
expect_contains "FB-1 待处理 — 处置提醒" "S4c FB-1 处置提醒"
expect_contains "FB-2 处理中 — 处置提醒" "S4d FB-2 处理提醒（状态词带括号说明仍解析）"
expect_contains "不许静默丢" "S4e 提醒语含不许静默丢"

# 全处置 → exit 0
cat >"$CH/verify-done/human-feedback.md" <<'EOF'
# Human Feedback — verify-done

## 反馈清单

### FB-1 2026-09-20【产品|frontend】已消化

- **状态**：已消化（原型当日修）

### FB-2 2026-09-21【流程|process】已转挂

- **状态**：已转挂 → add-change-lifecycle-retro

### FB-3 2026-09-22【产品|fact-check】已放弃

- **状态**：已放弃
EOF
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" verify "$CH/verify-done" 2>&1)"
RC=$?
expect_rc 0 "S4f 全部处置（已消化/已转挂/已放弃）→ exit 0"
expect_contains "已处置 3 / 未处置 0 / 格式异常 0" "S4g 汇总计数"

# 无文件 → exit 0
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" verify "$CH/no-file" 2>&1)"
RC=$?
expect_rc 0 "S4h 文件缺席 → exit 0"
expect_contains "无 human-feedback.md（未启用反馈建账）" "S4i 缺席降级文案"

# ---------- 5. 格式异常视同未处置（状态行缺失 / 状态词表外） ----------
cat >"$CH/verify-bad/human-feedback.md" <<'EOF'
# Human Feedback — verify-bad

## 反馈清单

### FB-1 2026-09-20【产品|frontend】缺状态行

（正文没有任何状态行）

### FB-2 2026-09-21【产品|backend】状态词表外

- **状态**：已修（当日）

### FB-3 2026-09-22【流程|process】正常已消化

- **状态**：已消化
EOF
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" verify "$CH/verify-bad" 2>&1)"
RC=$?
expect_rc 1 "S5a 状态不明（格式异常）→ exit 1"
expect_contains "已处置 1 / 未处置 0 / 格式异常 2" "S5b 格式异常单列计数"
expect_contains "FB-1 — 状态不明视同未处置（缺状态行）" "S5c 缺状态行条目提醒"
expect_contains "FB-2 — 状态不明视同未处置（状态词表外: 已修" "S5d 词表外状态条目提醒"

# ---------- 6. 已归档拒绝建账（Scenario: 已归档 change 拒绝建账） ----------
# changes/ 目录仍在 + archive/ 有日期前缀同名目录 → 拒绝
printf '# stub\n' >"$CH/archived-x/human-feedback.md"
_md5_before="$(md5sum "$CH/archived-x/human-feedback.md" | cut -d' ' -f1)"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add archived-x --layer frontend --type 产品 "归档后反馈" 2>&1)"
RC=$?
expect_rc 1 "S6a 已归档 change → 非零退出"
expect_contains "已归档 change 拒绝建账" "S6b 拒绝提示"
expect_contains "归档后修订" "S6c 提示走归档后修订流程"
[ "$(md5sum "$CH/archived-x/human-feedback.md" | cut -d' ' -f1)" = "$_md5_before" ] && ok "S6d 文件零修改" || bad "S6d 已归档却写了文件"
# 误传归档目录名（带日期前缀，changes/ 无同名目录）→ 不剥前缀猜测
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add 2026-09-20-archived-x --layer frontend --type 产品 "x" 2>&1)"
RC=$?
expect_rc 1 "S6e 误传归档目录名 → 非零退出（不剥前缀猜测）"
expect_contains "不存在" "S6f 提示不存在"

# ---------- 7. change 不存在 / 非法名 / 用法 ----------
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add no-such-change --layer frontend --type 产品 "x" 2>&1)"
RC=$?
expect_rc 1 "S7a change 不存在 → exit 1"
expect_contains "change 目录不存在" "S7b 不存在提示"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add "bad name; rm" --layer frontend --type 产品 "x" 2>&1)"
RC=$?
expect_rc 1 "S7c 非法 change 名 → exit 1"
expect_contains "非法 change 名" "S7d 非法名提示"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" add fresh-change --layer frontend 2>&1)"
RC=$?
expect_rc 2 "S7e 缺要点/类型 → 用法错误 exit 2"
expect_contains "用法" "S7f 打印用法"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" verify 2>&1)"
RC=$?
expect_rc 2 "S7g verify 缺参 → exit 2"
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" frobnicate 2>&1)"
RC=$?
expect_rc 2 "S7h 未知子命令 → exit 2"

# ---------- 8. menu 词表（选择题不是回忆题） ----------
OUT="$(FEEDBACK_ROOT="$ROOT" bash "$SOURCE" menu 2>&1)"
RC=$?
expect_rc 0 "S8a menu → exit 0"
for w in frontend backend test-gap fact-check process 产品 流程 待处理 处理中 已消化 已转挂 已放弃; do
	expect_contains "$w" "S8b menu 含词表项 $w"
done
expect_contains "归因词表（五类封闭" "S8c 归因词表标题"
expect_contains "状态词表（五值封闭" "S8d 状态词表标题"

# ---------- 汇总 ----------
echo
echo "== 通过 $pass / 失败 $fail =="
[ "$fail" -eq 0 ] || exit 1
