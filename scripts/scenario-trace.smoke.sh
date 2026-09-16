#!/usr/bin/env bash
# scenario-trace.smoke.sh — scenario-trace.sh 的 fixture 冒烟自测（scenario-test-mapping-gate 1.1）
#
# 临时目录拼装 change 形状（specs/*.md + tasks.md），旁置真实被测脚本逐 case 断言
# 退出码与关键输出。映射的「存在文件」用仓库内真实路径（scripts/doc-impact.sh 等），
# 因为被测脚本按仓库根解析相对路径。
#
# case 清单（与 scenario-trace-gate spec 的 Scenario 对应）：
#   ① 映射齐全通过 + 多文件单元格（空白/逗号两种分隔）+ 跨 spec 重名一行覆盖
#   ② 缺映射阻断（输出列全部未映射标题）
#   ③ 映射文件不存在阻断
#   ④ 人工映射合法
#   ⑤ 无 delta specs 直接过
#   ⑥ 验证节/映射表缺失 FAIL（a 无验证节 / b 无表头 / c tasks.md 不存在）
#   ⑦ REMOVED 节 Scenario 不计入对账
#
# 用法：bash scripts/scenario-trace.smoke.sh   退出码：0 全过；1 有失败
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRACE="$SCRIPT_DIR/scenario-trace.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0

run_trace() { # $1 = change 目录；输出进 out，退出码进 rc
	out="$("$TRACE" "$1" 2>&1)"
	rc=$?
}

check() { # $1=期望退出码 $2=case 名 $3=输出应包含的关键字（可选，可传多个）
	local want_rc="$1" name="$2"
	shift 2
	if [ "$rc" != "$want_rc" ]; then
		echo "  ✗ $name（期望 rc=$want_rc 实际 rc=$rc）"
		echo "    输出: $out"
		failn=$((failn + 1))
		return
	fi
	local kw
	for kw in "$@"; do
		if ! printf '%s' "$out" | grep -qF "$kw"; then
			echo "  ✗ $name（rc 正确，但输出未包含: $kw）"
			echo "    输出: $out"
			failn=$((failn + 1))
			return
		fi
	done
	echo "  ✓ $name"
	pass=$((pass + 1))
}

mk_change() { # $1 = case 名，echo change 目录（含 specs/cap/spec.md）
	local c="$TMP/$1"
	mkdir -p "$c/specs/cap"
	cat > "$c/specs/cap/spec.md"
	echo "$c"
}

echo "== scenario-trace.smoke =="

# ---------- case ① 映射齐全 + 多文件单元格 + 跨 spec 重名一行覆盖 ----------
c="$(mk_change case1 <<'DELTA'
## ADDED Requirements

### Requirement: 生成链路

#### Scenario: 正常生成

- **WHEN** 输入合法
- **THEN** 产出产物

#### Scenario: 重名场景

- **WHEN** A
- **THEN** B
DELTA
)"
# 第二个 spec 文件出现同名 Scenario（MODIFIED 节）——一行映射覆盖两处
mkdir -p "$c/specs/cap2"
cat > "$c/specs/cap2/spec.md" <<'DELTA2'
## MODIFIED Requirements

### Requirement: 既有需求

#### Scenario: 重名场景

- **WHEN** C
- **THEN** D
DELTA2
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 实现

- [x] 1.1 done

## 2. 验证

- [ ] 2.1 foo

| Scenario | 测试文件 |
| --- | --- |
| 正常生成 | scripts/doc-impact.sh scripts/check-standards.sh |
| 重名场景 | scripts/doc-impact.sh,scripts/change-scope.sh |

## 3. 文档

- 无
TASKS
run_trace "$c"
check 0 "① 映射齐全 + 多文件（空白/逗号分隔）+ 跨 spec 重名一行覆盖" "映射齐全"

# ---------- case ② 缺映射阻断 ----------
c="$(mk_change case2 <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 场景甲

- **WHEN** a
- **THEN** b

#### Scenario: 场景乙

- **WHEN** c
- **THEN** d

#### Scenario: 场景丙

- **WHEN** e
- **THEN** f
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

| Scenario | 测试文件 |
| --- | --- |
| 场景甲 | scripts/doc-impact.sh |
TASKS
run_trace "$c"
check 1 "② 缺映射阻断（列出全部未映射标题）" "未映射" "场景乙" "场景丙"

# ---------- case ③ 映射文件不存在阻断 ----------
c="$(mk_change case3 <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 单场景

- **WHEN** a
- **THEN** b
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

| Scenario | 测试文件 |
| --- | --- |
| 单场景 | scripts/no_such_file_test.go |
TASKS
run_trace "$c"
check 1 "③ 映射文件不存在阻断" "不存在" "scripts/no_such_file_test.go"

# ---------- case ④ 人工映射合法 ----------
c="$(mk_change case4 <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 目视场景

- **WHEN** a
- **THEN** b
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

| Scenario | 测试文件 |
| --- | --- |
| 目视场景 | 人工（UI 目视确认） |
TASKS
run_trace "$c"
check 0 "④ 人工映射合法" "人工留痕"

# ---------- case ⑤ 无 delta specs 直接过 ----------
c="$TMP/case5"
mkdir -p "$c"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

- [ ] 1.1 手动查验
TASKS
run_trace "$c"
check 0 "⑤ 无 delta specs 直接过" "无待对账"

# ---------- case ⑥ 验证节/映射表缺失 ----------
# ⑥a tasks.md 存在但无验证节
c="$(mk_change case6a <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 任意场景

- **WHEN** a
- **THEN** b
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 实现

- [ ] 1.1 foo
TASKS
run_trace "$c"
check 1 "⑥a 无「## N. 验证」节阻断" "验证"

# ⑥b 验证节存在但无规定表头的表
c="$(mk_change case6b <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 任意场景

- **WHEN** a
- **THEN** b
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

| Case | 文件 |
| --- | --- |
| x | y |
TASKS
run_trace "$c"
check 1 "⑥b 验证节内无映射表阻断" "映射表"

# ⑥c tasks.md 不存在
c="$(mk_change case6c <<'DELTA'
## ADDED Requirements

### Requirement: R

#### Scenario: 任意场景

- **WHEN** a
- **THEN** b
DELTA
)"
rm -f "$c/tasks.md"
run_trace "$c"
check 1 "⑥c tasks.md 不存在阻断" "tasks.md 不存在"

# ---------- case ⑦ REMOVED 节不计入对账 ----------
c="$(mk_change case7 <<'DELTA'
## ADDED Requirements

### Requirement: 新需求

#### Scenario: 保留场景

- **WHEN** a
- **THEN** b

## REMOVED Requirements

### Requirement: 旧需求

#### Scenario: 已删场景

- **WHEN** old
- **THEN** gone
DELTA
)"
cat > "$c/tasks.md" <<'TASKS'
# Tasks

## 1. 验证

| Scenario | 测试文件 |
| --- | --- |
| 保留场景 | scripts/doc-impact.sh |
TASKS
run_trace "$c"
check 0 "⑦ REMOVED 节 Scenario 不计入（已删场景无需映射）" "1 个 Scenario"

# ---------- 汇总 ----------
echo
if [ "$failn" -gt 0 ]; then
	echo "✗ scenario-trace.smoke 未通过（$pass 过 / $failn 败）"
	exit 1
fi
echo "✓ scenario-trace.smoke 全过（$pass/$pass）"
exit 0
