#!/usr/bin/env bash
# test-patrol.smoke.sh — scripts/harness/test-patrol.sh 白盒冒烟（change: test-debt-patrol）
#
# 覆盖 test-cases.md 的 A/B/C/V/R/F/P/X/SUR 组（92 条里的可自动判定部分；§5 的 MAN-* 长跑项不在本脚本范围）。
# 全部用 mktemp 临时库 + fake runner（TEST_PATROL_FAKE_OUTPUT / FAKE_RC / FAKE_SLEEP）——
# **毫秒级，绝不执行真实测试**（树莓派资源红线：不跑 pnpm test:unit 全量、不跑 go test 全量）。
#
# 用法：bash scripts/harness/test-patrol.smoke.sh   → 逐条 [OK]/[FAIL]，退出码 0 = 全绿
# 参照 scripts/harness/check-standards.smoke.sh 的 mktemp fixture + 计数风格。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"
SOURCE="$REPO_ROOT/scripts/harness/test-patrol.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/test-patrol-smoke.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

DB="$TMP/events.db"
FI="$TMP/fixtures"
mkdir -p "$FI"

pass=0
fail=0
ok() { printf '  [OK] %s\n' "$1"; pass=$((pass + 1)); }
bad() {
	printf '  [FAIL] %s\n' "$1" >&2
	fail=$((fail + 1))
}
group() { printf '\n== %s ==\n' "$1"; }

# ---------------------------------------------------------------------------
# 夹具与工具
# ---------------------------------------------------------------------------
RC=0
OUT=""

p() { # p [env 赋值...] bash "$SOURCE" <参数...>   → RC / OUT
	OUT="$(env "$@" 2>&1)"
	RC=$?
}

run_fx() { # run_fx <fixture> <fake_rc> <patrol 参数...>
	local f="$1" r="$2"
	shift 2
	p TEST_PATROL_FAKE_OUTPUT="$f" TEST_PATROL_FAKE_RC="$r" bash "$SOURCE" "$@"
}

q() { sqlite3 -batch "$DB" "$1"; }             # 单值 / 脚本
qr() { sqlite3 -batch -separator '|' "$DB" "$1"; } # 多行
snapshot() { printf '%s|%s|%s' "$(q 'SELECT COUNT(*) FROM test_debt;')" "$(q 'SELECT COUNT(*) FROM patrol_shard;')" "$(q 'SELECT COUNT(*) FROM events;')"; }
pick_of() { printf '%s' "$OUT" | grep -oE '^[✓✗↷] \[[A-Za-z-]+\]' | head -1 | sed 's/.*\[\(.*\)\]/\1/'; }
pick_all() { printf '%s' "$OUT" | grep -oE '^[✓✗↷] \[[A-Za-z-]+\]' | sed 's/.*\[\(.*\)\]/\1/'; }
summary_lines() { printf '%s' "$OUT" | grep -cE '^[✓✗↷] \['; }

assert_eq() { # label expected actual
	if [ "$2" = "$3" ]; then ok "$1"; else bad "$1（期望 [$2]，实际 [$3]）"; fi
}
assert_rc() { # label expected
	if [ "$RC" = "$2" ]; then ok "$1"; else bad "$1（期望 exit $2，实际 $RC：$(printf '%s' "$OUT" | head -3 | tr '\n' ' ')）"; fi
}
assert_has() { # label haystack needle
	case "$2" in *"$3"*) ok "$1" ;; *) bad "$1（缺 [$3]；输出：$(printf '%s' "$2" | head -3 | tr '\n' ' ')）" ;; esac
}
assert_not_has() { # label haystack needle
	case "$2" in *"$3"*) bad "$1（不应含 [$3]）" ;; *) ok "$1" ;; esac
}

reset_db() {
	sqlite3 -batch "$DB" "DELETE FROM test_debt; DELETE FROM patrol_shard; DELETE FROM events;"
}
clear_progress() { sqlite3 -batch "$DB" "DELETE FROM patrol_shard;"; }

NOW0='2026-09-17T00:00:00Z'
nth_now() { date -u -d "$NOW0 + $1 seconds" +%Y-%m-%dT%H:%M:%SZ; }

make_fixtures() {
	printf 'ok  \tsyntopica-backend/internal/reader/service\t0.123s\n?   \tsyntopica-backend/internal/reader/nodb\t[no test files]\n' >"$FI/be-green.out"
	printf '' >"$FI/empty.out"
	printf -- '--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\nFAIL\nFAIL\tsyntopica-backend/internal/reader/service\t0.123s\n' >"$FI/be-fail1.out"
	printf -- '--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\n    --- FAIL: TestA/case_x (0.00s)\n        a_test.go:5: sub boom\nFAIL\nFAIL\tsyntopica-backend/internal/reader/service\t0.123s\n' >"$FI/be-subtest.out"
	printf -- '--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\nFAIL\nFAIL\tsyntopica-backend/internal/admin/handler\t0.002s\n--- FAIL: TestB (0.00s)\n    b_test.go:5: boom2\nFAIL\nFAIL\tsyntopica-backend/internal/reader/store\t0.003s\n' >"$FI/be-twopkg.out"
	printf '# syntopica-backend/internal/foo [syntopica-backend/internal/foo.test]\ninternal/foo/x.go:3:2: undefined: bar\nFAIL\tsyntopica-backend/internal/foo [build failed]\n' >"$FI/be-buildfailed.out"
	printf 'ok  \tsyntopica-backend/internal/foo\t0.1s\n' >"$FI/be-foo-ok.out"
	# 前端：整文件级 collect 失败
	printf ' FAIL  app/plugins/chunk-error-fallback.test.ts [ app/plugins/chunk-error-fallback.test.ts ]\n' >"$FI/fe-filelevel.out"
	# 前端：用例级（嵌套 describe 链，含括号/双引号/===）
	printf ' FAIL  app/composables/useOnboarding.test.ts > useOnboarding > first-run detection (isFirstRun) > is true when syntopica_onboarding_complete === "absent"\n' >"$FI/fe-caselevel.out"
	# 前端：同 id 在 × 明细段与尾部 FAIL 段各出现一次（同批次去重）
	printf '× app/x.test.ts > suite > case\n FAIL  app/x.test.ts > suite > case\n' >"$FI/fe-dup1.out"
	# 前端：17 条 × 2 段（双 reporter 段去重）—— 真实 vitest 形态：❯ 文件头 + × 明细（无路径、带耗时）+ 尾部 FAIL（带路径）+ 堆栈行
	{
		printf ' ❯ app/composables/useOnboarding.test.ts (17 tests | 17 failed) 1234ms\n'
		local i
		for i in $(seq 1 17); do printf '   × suite%s > case%s 19ms\n' "$i" "$i"; done
		printf ' Test Files  1 failed | 99 passed (100)\n'
		for i in $(seq 1 17); do printf ' FAIL  app/composables/useOnboarding.test.ts > suite%s > case%s\n' "$i" "$i"; done
		for i in $(seq 1 17); do printf ' ❯ app/composables/useOnboarding.test.ts:%s:1\n' "$i"; done
	} >"$FI/fe-dup17.out"
	# 前端：真实 vitest 形态取样（2026-09-17 fe-discovery 实跑）—— × 明细带耗时无路径 + FAIL 尾段带路径 + ❯ 堆栈行
	{
		printf ' ❯ app/features/discovery/components/CandidateImportDialog.test.ts (10 tests | 1 failed) 297ms\n'
		printf '   ✓ CandidateImportDialog — 其它 > ok 3ms\n'
		printf '   × CandidateImportDialog — 取消零副作用（C3） > 预览后直接退出 28ms\n'
		printf '      → expected undefined to deeply equal [ false ]\n'
		printf ' ❯ app/features/discovery/components/CandidateEditDialog.test.ts (10 tests | 2 failed) 309ms\n'
		printf '   × CandidateEditDialog — 保存语义（S9） > 保存成功后关闭弹窗 32ms\n'
		printf ' FAIL  app/features/discovery/components/CandidateImportDialog.test.ts > CandidateImportDialog — 取消零副作用（C3） > 预览后直接退出\n'
		printf ' ❯ app/features/discovery/components/CandidateImportDialog.test.ts:267:58\n'
		printf ' FAIL  app/features/discovery/components/CandidateEditDialog.test.ts > CandidateEditDialog — 保存语义（S9） > 保存成功后关闭弹窗\n'
	} >"$FI/fe-real.out"
	# 前端：全绿
	printf ' Test Files  1 passed (1)\n' >"$FI/fe-green.out"
	# 长输出全绿（并发写库用例：拉长解析/写入窗口）
	{
		local i
		for i in $(seq 1 500); do printf 'ok  \tsyntopica-backend/internal/platform/pkg%s\t0.001s\n' "$i"; done
	} >"$FI/green-long.out"
}

# 真实负载/进程环境不参与断言：注入低负载 + 空进程清单
printf '0.50 1.00 1.50 1/500 1234\n' >"$TMP/loadavg-low"
printf '4.00 1.00 1.50 1/500 1234\n' >"$TMP/loadavg-4.00"
printf '4.01 1.00 1.50 1/500 1234\n' >"$TMP/loadavg-4.01"
printf '8.30 1.00 1.50 1/500 1234\n' >"$TMP/loadavg-8.30"
: >"$TMP/procs-empty"
printf '%s\n' '/usr/lib/go-1.27/bin/go build -o /tmp/x ./cmd/server' >"$TMP/procs-build"
printf '%s\n' '/usr/bin/chromium --headless=new --remote-debugging-port=9222' >"$TMP/procs-browser"

export TEST_PATROL_DB="$DB"
export TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-low"
export TEST_PATROL_PROCESS_LIST="$TMP/procs-empty"
export TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out"
export TEST_PATROL_FAKE_RC=0

make_fixtures
bash "$SOURCE" --init >/dev/null 2>&1 || {
	echo "✗ 前置失败：--init 无法建库（$DB）" >&2
	exit 1
}

# ---------------------------------------------------------------------------
# F 组：CLI 参数校验与退出码
# ---------------------------------------------------------------------------
expect_usage_error() { # label + args（断言 exit 2 且三表零变化、无 ✓）
	local label="$1"
	shift
	local before after
	before="$(snapshot)"
	p bash "$SOURCE" "$@"
	after="$(snapshot)"
	if [ "$RC" -eq 2 ] && [ "$before" = "$after" ] && [ "$before" != "$after" ]; then
		bad "$label（不可达分支）"
	elif [ "$RC" -eq 2 ] && [ "$before" = "$after" ] && [[ "$OUT" != *'✓'* ]] && [ -n "$OUT" ]; then
		ok "$label"
	elif [ "$RC" -eq 2 ] && [ "$before" = "$after" ] && [[ "$OUT" != *'✓'* ]]; then
		bad "$label（错误信息为空，需可见提示）"
	else
		bad "$label（exit $RC，期望 2；三表 $before→$after；输出：$(printf '%s' "$OUT" | head -3 | tr '\n' ' ')）"
	fi
}

group_F() {
	group "F 组 CLI 参数校验与退出码"

	# TC-F01 / TC-V09 / B-20：--init 幂等
	rm -f "$DB" "$DB-wal" "$DB-shm"
	p bash "$SOURCE" --init
	assert_rc "TC-F01 --init 首次 exit 0" 0
	p bash "$SOURCE" --init
	assert_rc "TC-F01 --init 第二次仍 exit 0" 0
	assert_not_has "TC-F01 第二次无 ERROR 行" "$OUT" 'ERROR'
	sqlite3 -batch "$DB" "INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status) VALUES ('preset::X','manual','$NOW0','$NOW0','preset','open');" >/dev/null
	sqlite3 -batch "$DB" "INSERT INTO events (ts, session_id, kind, change, payload) VALUES ('$NOW0','s','gate.check',NULL,'{}');" >/dev/null
	p bash "$SOURCE" --init
	assert_eq "TC-F01 已有数据后 --init 不动既有行（debt=1）" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-V09 --init 不动既有 events 行（events=1）" "1" "$(q 'SELECT COUNT(*) FROM events;')"
	sqlite3 -batch "$DB" "DELETE FROM test_debt; DELETE FROM events;" >/dev/null

	# TC-F02 表结构契约
	assert_eq "TC-F02 test_debt 11 列" "11" "$(q "SELECT COUNT(*) FROM pragma_table_info('test_debt');")"
	assert_eq "TC-F02 patrol_shard 7 列" "7" "$(q "SELECT COUNT(*) FROM pragma_table_info('patrol_shard');")"
	assert_eq "TC-F02 test_debt 列名齐全" "id,test_id,domain,first_seen,last_seen,context,status,fixed_by,waived_reason,waived_at,note" \
		"$(q "SELECT group_concat(name,',') FROM pragma_table_info('test_debt') ORDER BY cid;")"
	assert_eq "TC-F02 patrol_shard 列名齐全" "shard,last_run,last_ok,last_ms,last_fails,runs,total_fails" \
		"$(q "SELECT group_concat(name,',') FROM pragma_table_info('patrol_shard') ORDER BY cid;")"
	if [ "$(q "SELECT COUNT(*) FROM pragma_index_list('test_debt') WHERE \"unique\"=1;")" -ge 1 ] 2>/dev/null; then
		ok "TC-F02 test_id 有 UNIQUE 索引（PRAGMA index_list 可证）"
	else
		bad "TC-F02 test_id UNIQUE 索引缺失"
	fi

	# TC-A14 status 三态：DDL CHECK 双侧（脚本层在 --register/--resolve/--waive 路径）
	assert_has "TC-A14 .schema 含 CHECK(status IN (open,fixed,waived))" "$(q ".schema test_debt")" "CHECK (status IN ('open','fixed','waived'))"
	err_out="$(sqlite3 -batch "$DB" "INSERT INTO test_debt (test_id, first_seen, last_seen, status) VALUES ('bad::X','$NOW0','$NOW0','closed');" 2>&1)"
	assert_has "TC-A14 直接 INSERT status='closed' 被 DDL CHECK 拒绝" "$err_out" 'CHECK constraint failed'
	assert_eq "TC-A14 库中不出现第四种状态" "0" "$(q "SELECT COUNT(*) FROM test_debt WHERE status NOT IN ('open','fixed','waived');")"

	# TC-F03 --help 九入口
	p bash "$SOURCE" --help
	assert_rc "TC-F03 --help exit 0" 0
	for token in '默认' '--shard' '--shards' '--init' '--report' '--register' '--resolve' '--waive' '--help'; do
		assert_has "TC-F03 --help 含入口 $token" "$OUT" "$token"
	done

	# TC-F04 未知分片 / 空串
	expect_usage_error "TC-F04 --shard bogus → exit 2" --shard bogus
	assert_eq "TC-F04 错误输出列出 13 个可选分片" "13" "$(printf '%s' "$OUT" | grep -cE '^    (be|fe)-[a-z-]+')"
	expect_usage_error "TC-F04 --shard 空串 → exit 2" --shard ''

	# TC-F05 --shards 校验
	expect_usage_error "TC-F05 --shards 0 → exit 2" --shards 0
	p bash "$SOURCE" --shards -1
	[ "$RC" -eq 2 ] && ok "TC-F05 --shards -1 → exit 2（负数不当作选项）" || bad "TC-F05 --shards -1 → exit $RC，期望 2"
	expect_usage_error "TC-F05 --shards abc → exit 2" --shards abc
	expect_usage_error "TC-F05 --shards 空串 → exit 2" --shards ''

	# TC-F06 未知子命令
	expect_usage_error "TC-F06 --foo → exit 2" --foo
	assert_has "TC-F06 提示 --help" "$OUT" '--help'

	# TC-F07 --register 必填项
	expect_usage_error "TC-F07 --register 无参 → exit 2" --register
	assert_has "TC-F07 提示 --context 必填（无参）" "$OUT" '--context'
	expect_usage_error "TC-F07 --register 空串 → exit 2" --register '' --context x
	expect_usage_error "TC-F07 --register 缺 --context → exit 2" --register 'front/a.test.ts::x'
	assert_has "TC-F07 提示 --context 必填（缺项）" "$OUT" '--context'

	# TC-F08 / TC-F09 未知 id
	expect_usage_error "TC-F08 --resolve 未知 id → exit 2" --resolve 'nope::TestX'
	assert_has "TC-F08 提示改用 --register" "$OUT" '--register'
	expect_usage_error "TC-F09 --waive 未知 id → exit 2" --waive 'nope::TestX' --reason x
	assert_has "TC-F09 提示未知 id" "$OUT" '未知 id'

	# TC-F10 --waive 必填 reason
	sqlite3 -batch "$DB" "INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status) VALUES ('w1::X','manual','$NOW0','$NOW0','c','open');" >/dev/null
	expect_usage_error "TC-F10 --waive 缺 --reason → exit 2" --waive 'w1::X'
	p bash "$SOURCE" --waive 'w1::X' --reason ''
	[ "$RC" -eq 2 ] && ok "TC-F10 --waive --reason 空串 → exit 2" || bad "TC-F10 --reason 空串 → exit $RC，期望 2"
	assert_eq "TC-F10 记录仍 open" "open" "$(q "SELECT status FROM test_debt WHERE test_id='w1::X';")"
	assert_eq "TC-F10 waived_reason 仍 NULL" "" "$(q "SELECT COALESCE(waived_reason,'') FROM test_debt WHERE test_id='w1::X';")"

	# TC-F11 未知选项
	expect_usage_error "TC-F11 --bogus 未知选项 → exit 2" --register 'r1::X' --context x --bogus 1
	sqlite3 -batch "$DB" "DELETE FROM test_debt WHERE test_id='w1::X';" >/dev/null
}

# ---------------------------------------------------------------------------
# A 组：台账状态机
# ---------------------------------------------------------------------------
group_A() {
	group "A 组 台账状态机"

	# TC-A01 空台账 + 全绿片
	reset_db
	run_fx "$FI/be-green.out" 0 --shard be-admin
	assert_rc "TC-A01 全绿 exit 0" 0
	assert_eq "TC-A01 台账仍 0 行" "0" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-A01 进度 runs=1,last_ok=1,last_fails=0" "1|1|0" "$(q "SELECT runs||'|'||last_ok||'|'||last_fails FROM patrol_shard WHERE shard='be-admin';")"

	# TC-A02 新红自动登记
	reset_db
	p TEST_PATROL_FAKE_OUTPUT="$FI/fe-dup1.out" TEST_PATROL_FAKE_RC=1 TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --shard fe-tags
	assert_rc "TC-A02 检出红 exit 1" 1
	assert_eq "TC-A02 新增恰 1 条" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-A02 status/first/last/context/domain + 终态字段 NULL" "open|$NOW0|$NOW0|fe-tags|fe-tags|1|1|1" \
		"$(q "SELECT status||'|'||first_seen||'|'||last_seen||'|'||context||'|'||domain||'|'||(fixed_by IS NULL)||'|'||(waived_reason IS NULL)||'|'||(waived_at IS NULL) FROM test_debt WHERE test_id='front/app/x.test.ts::suite > case';")"

	# TC-A03/A15 已有 open 记录不重复登记、last_seen 前进、note 不被覆盖
	sqlite3 -batch "$DB" "UPDATE test_debt SET note='' WHERE 1;" >/dev/null
	p TEST_PATROL_FAKE_OUTPUT="$FI/fe-dup1.out" TEST_PATROL_FAKE_RC=1 TEST_PATROL_NOW="$(nth_now 60)" bash "$SOURCE" --shard fe-tags
	assert_eq "TC-A03 行数不变" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-A03 last_seen 前进 / first_seen 冻结 / status 仍 open" "$(nth_now 60)|$NOW0|open" \
		"$(q "SELECT last_seen||'|'||first_seen||'|'||status FROM test_debt WHERE test_id='front/app/x.test.ts::suite > case';")"
	assert_eq "TC-A15 open 记录重复登记不写 note" "" "$(q "SELECT COALESCE(note,'') FROM test_debt WHERE test_id='front/app/x.test.ts::suite > case';")"
	assert_eq "TC-A03 context 不被覆盖" "fe-tags" "$(q 'SELECT context FROM test_debt;')"

	# TC-A04 同批次去重（× 段 + FAIL 段各一次 → 1 条）
	assert_eq "TC-A04 × 段与 FAIL 段去重为 1 条" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-A04 payload.fails 长度 1" "1" "$(q "SELECT json_array_length(payload,'\$.fails') FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"

	# TC-A05 --register 手工登记
	reset_db
	p TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --register 'front/app/y.test.ts::suite > case' --context 'test-debt-patrol' --domain manual
	assert_rc "TC-A05 --register exit 0" 0
	assert_eq "TC-A05 context/domain 原样写入" "test-debt-patrol|manual|open" \
		"$(q "SELECT context||'|'||domain||'|'||status FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"
	p bash "$SOURCE" --report
	assert_has "TC-A05 --report open 清单含该条" "$OUT" 'front/app/y.test.ts::suite > case'

	# TC-A06 fixed 记录复现 → 迁回 open + reopened 痕迹 + 保留 fixed_by/first_seen
	p bash "$SOURCE" --resolve 'front/app/y.test.ts::suite > case' --by fix-xyz
	assert_rc "TC-A08 --resolve exit 0" 0
	assert_eq "TC-A08 status=fixed/fixed_by/first_seen 保留/waived_* 仍 NULL" "fixed|fix-xyz|$NOW0|$NOW0|1" \
		"$(q "SELECT status||'|'||fixed_by||'|'||first_seen||'|'||last_seen||'|'||(waived_reason IS NULL) FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"
	p TEST_PATROL_NOW="$(nth_now 120)" bash "$SOURCE" --register 'front/app/y.test.ts::suite > case' --context 'test-debt-patrol'
	assert_eq "TC-A06 迁回 open" "open" "$(q "SELECT status FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"
	assert_has "TC-A06 note 含 reopened from fixed@" "$(q "SELECT note FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")" 'reopened from fixed@'
	assert_eq "TC-A06 fixed_by 保留" "fix-xyz" "$(q "SELECT fixed_by FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"
	assert_eq "TC-A06 first_seen 不变" "$NOW0" "$(q "SELECT first_seen FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"

	# TC-A11/A07 waived 分支（独立新记录，确保 fixed_by 仍 NULL）
	p TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --register 'front/app/z.test.ts::waive-case' --context 'test-debt-patrol'
	p TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --waive 'front/app/z.test.ts::waive-case' --reason '已知环境问题，见 survey §3'
	assert_rc "TC-A11 --waive exit 0" 0
	assert_eq "TC-A11 waived 字段留痕（reason 原文 / waived_at 非空 / fixed_by 仍 NULL）" "waived|已知环境问题，见 survey §3|1|1" \
		"$(q "SELECT status||'|'||waived_reason||'|'||(waived_at IS NOT NULL)||'|'||(fixed_by IS NULL) FROM test_debt WHERE test_id='front/app/z.test.ts::waive-case';")"
	p TEST_PATROL_NOW="$(nth_now 180)" bash "$SOURCE" --register 'front/app/z.test.ts::waive-case' --context 'test-debt-patrol'
	assert_eq "TC-A07 waived 记录迁回 open" "open" "$(q "SELECT status FROM test_debt WHERE test_id='front/app/z.test.ts::waive-case';")"
	assert_has "TC-A07 note 含 reopened from waived@" "$(q "SELECT note FROM test_debt WHERE test_id='front/app/z.test.ts::waive-case';")" 'reopened from waived@'
	assert_eq "TC-A07 waived_reason 保留" "已知环境问题，见 survey §3" "$(q "SELECT waived_reason FROM test_debt WHERE test_id='front/app/z.test.ts::waive-case';")"

	# TC-A12 终态幂等 no-op（A-3）
	sqlite3 -batch "$DB" "UPDATE test_debt SET status='fixed', fixed_by='c9', waived_reason=NULL WHERE test_id='front/app/y.test.ts::suite > case';" >/dev/null
	p bash "$SOURCE" --resolve 'front/app/y.test.ts::suite > case' --by other
	assert_rc "TC-A12 终态再 --resolve exit 0（幂等）" 0
	assert_has "TC-A12 提示已是终态" "$OUT" '终态'
	assert_eq "TC-A12 字段零变化（fixed_by 不被覆盖）" "fixed|c9|" \
		"$(q "SELECT status||'|'||fixed_by||'|'||COALESCE(waived_reason,'') FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"
	p bash "$SOURCE" --waive 'front/app/y.test.ts::suite > case' --reason '事后改理由'
	assert_rc "TC-A12 终态再 --waive exit 0（幂等）" 0
	assert_eq "TC-A12 终态理由不被事后改写" "fixed|c9|" \
		"$(q "SELECT status||'|'||fixed_by||'|'||COALESCE(waived_reason,'') FROM test_debt WHERE test_id='front/app/y.test.ts::suite > case';")"

	# TC-A09/A10 转绿闭环 + 不误伤
	reset_db
	sqlite3 -batch "$DB" "INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status) VALUES ('internal/reader/service::TestA','be-reader','$NOW0','$NOW0','be-reader','open');" >/dev/null
	sqlite3 -batch "$DB" "INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status, note) VALUES ('manual::Keep','manual','$NOW0','$NOW0','manual','open','keepme');" >/dev/null
	run_fx "$FI/be-green.out" 0 --shard be-reader
	assert_eq "TC-A09 该片 open 记录迁 fixed_by=patrol" "fixed|patrol|$NOW0" \
		"$(q "SELECT status||'|'||fixed_by||'|'||first_seen FROM test_debt WHERE test_id='internal/reader/service::TestA';")"
	assert_eq "TC-A10 manual 记录全字段零变化" "open||manual|$NOW0|$NOW0|manual|keepme" \
		"$(q "SELECT status||'|'||COALESCE(fixed_by,'')||'|'||domain||'|'||first_seen||'|'||last_seen||'|'||context||'|'||note FROM test_debt WHERE test_id='manual::Keep';")"

	# TC-A13 终态不得物理删除 + 脚本无 DELETE
	assert_eq "TC-A13 终态记录未被物理删除" "1" "$(q "SELECT COUNT(*) FROM test_debt WHERE status='fixed';")"
	assert_eq "TC-A13 台账行只增不减（≥2）" "1" "$([ "$(q 'SELECT COUNT(*) FROM test_debt;')" -ge 2 ] && printf 1 || printf 0)"
	assert_eq "TC-A13 脚本无 DELETE FROM test_debt" "0" "$(grep -c 'DELETE FROM test_debt' "$SOURCE" || true)"
}

# ---------------------------------------------------------------------------
# B 组：分片调度
# ---------------------------------------------------------------------------
SHARDS_STATIC=(be-admin be-dataenrichment be-reader be-tagmanagement be-topicgraph be-skeleton fe-tags fe-discovery fe-features fe-core fe-composables fe-components)

group_B() {
	group "B 组 分片调度（最久未巡优先 / be-all 不入轮转）"

	# TC-B01 冷启动选中首片
	reset_db
	p TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 TEST_PATROL_NOW="$NOW0" bash "$SOURCE"
	assert_rc "TC-B01 无参 exit 0" 0
	assert_eq "TC-B01 冷启动选中 be-admin" "be-admin" "$(pick_of)"
	assert_eq "TC-B01 仅 be-admin 有进度行" "1" "$(q 'SELECT COUNT(*) FROM patrol_shard;')"
	assert_eq "TC-B01 be-admin runs=1" "1" "$(q "SELECT runs FROM patrol_shard WHERE shard='be-admin';")"

	# TC-B02 第二片推进
	p TEST_PATROL_NOW="$(nth_now 60)" bash "$SOURCE"
	assert_eq "TC-B02 第二片 = be-dataenrichment" "be-dataenrichment" "$(pick_of)"
	assert_eq "TC-B02 两片 last_run 非空且 be-admin 更早" "1" \
		"$(q "SELECT (SELECT last_run FROM patrol_shard WHERE shard='be-admin') < (SELECT last_run FROM patrol_shard WHERE shard='be-dataenrichment');")"

	# TC-B03 一轮完成后回到起点：13 次无参 = 静态表整轮循环
	reset_db
	seq_out=""
	i=1
	while [ "$i" -le 13 ]; do
		p TEST_PATROL_NOW="$(nth_now "$i")" bash "$SOURCE"
		seq_out="$seq_out $(pick_of)"
		i=$((i + 1))
	done
	expected_seq=" ${SHARDS_STATIC[*]} be-admin"
	assert_eq "TC-B03 13 次选中序列 = 静态表整轮循环" "$expected_seq" "$seq_out"

	# TC-B04 NULL 优先于任何时间戳
	reset_db
	i=0
	for s in "${SHARDS_STATIC[@]}"; do
		i=$((i + 1))
		sqlite3 -batch "$DB" "INSERT INTO patrol_shard (shard,last_run,last_ok,last_ms,last_fails,runs,total_fails) VALUES ('$s','$(nth_now 100)',1,1,0,1,0);" >/dev/null
	done
	sqlite3 -batch "$DB" "UPDATE patrol_shard SET last_run=NULL WHERE shard='fe-core';" >/dev/null
	p bash "$SOURCE"
	assert_eq "TC-B04 NULL 片最优先（即使其它片时间戳很旧）" "fe-core" "$(pick_of)"

	# TC-B05 平局按静态表顺序且稳定
	reset_db
	for s in "${SHARDS_STATIC[@]}"; do
		sqlite3 -batch "$DB" "INSERT INTO patrol_shard (shard,last_run,last_ok,last_ms,last_fails,runs,total_fails) VALUES ('$s','$(nth_now 500)',1,1,0,1,0);" >/dev/null
	done
	sqlite3 -batch "$DB" "UPDATE patrol_shard SET last_run='$(nth_now 100)' WHERE shard IN ('be-reader','be-topicgraph');" >/dev/null
	p TEST_PATROL_NOW="$(nth_now 600)" bash "$SOURCE"
	first="$(pick_of)"
	# 重建同一平局状态（第一次运行会推进 be-reader 的 last_run），再跑一次验证无随机
	sqlite3 -batch "$DB" "UPDATE patrol_shard SET last_run='$(nth_now 100)' WHERE shard IN ('be-reader','be-topicgraph');" >/dev/null
	p TEST_PATROL_NOW="$(nth_now 601)" bash "$SOURCE"
	second="$(pick_of)"
	assert_eq "TC-B05 平局选静态表靠前者" "be-reader" "$first"
	assert_eq "TC-B05 平局结果稳定（第二次仍同上）" "be-reader" "$second"

	# TC-B06 12 次跑满一轮：无重复无遗漏、be-all 从不入选
	reset_db
	seen=""
	i=0
	while [ "$i" -lt 12 ]; do
		p TEST_PATROL_NOW="$(nth_now "$((i + 1))")" bash "$SOURCE"
		seen="$seen $(pick_of)"
		i=$((i + 1))
	done
	assert_eq "TC-B06 选中集合恰为 12 个轮转片（无重复/无遗漏）" "$(printf '%s\n' "${SHARDS_STATIC[@]}" | sort | tr '\n' ' ' | sed 's/ $//')" "$(printf '%s' "$seen" | tr ' ' '\n' | awk 'NF' | sort | tr '\n' ' ' | sed 's/ $//')"
	assert_eq "TC-B06 be-all 从未被选中" "0" "$(printf '%s' "$seen" | grep -c 'be-all' || true)"
	assert_eq "TC-B06 进度表无 be-all 行" "0" "$(q "SELECT COUNT(*) FROM patrol_shard WHERE shard='be-all';")"

	# TC-B07 be-all 仅显式指定
	reset_db
	p TEST_PATROL_DRY_RUN=1 bash "$SOURCE" --shard be-all
	assert_has "TC-B07 DRY_RUN be-all 含 ./internal/..." "$OUT" 'arg=./internal/...'
	assert_has "TC-B07 DRY_RUN be-all 含 ./cmd/..." "$OUT" 'arg=./cmd/...'
	run_fx "$FI/be-green.out" 0 --shard be-all
	assert_eq "TC-B07 进度表新增 be-all 行" "1" "$(q "SELECT COUNT(*) FROM patrol_shard WHERE shard='be-all';")"

	# TC-B08 --shards 3（冷启动 = 静态表前 3）
	reset_db
	run_fx "$FI/be-green.out" 0 --shards 3
	assert_eq "TC-B08 stdout 3 行摘要" "3" "$(summary_lines)"
	assert_eq "TC-B08 顺序 = 静态表前 3 片" "be-admin be-dataenrichment be-reader" "$(pick_all | tr '\n' ' ' | sed 's/ $//')"
	assert_eq "TC-B08 3 片 runs 均为 1" "3" "$(q 'SELECT COUNT(*) FROM patrol_shard WHERE runs=1;')"

	# TC-B09 --shards 99 跑满一轮不重复
	reset_db
	i=1
	for s in "${SHARDS_STATIC[@]}"; do
		sqlite3 -batch "$DB" "INSERT INTO patrol_shard (shard,last_run,last_ok,last_ms,last_fails,runs,total_fails) VALUES ('$s','$(nth_now "$i")',1,1,0,1,0);" >/dev/null
		i=$((i + 1))
	done
	p TEST_PATROL_NOW="$(nth_now 1000)" bash "$SOURCE" --shards 99
	assert_rc "TC-B09 --shards 99 exit 0" 0
	assert_eq "TC-B09 恰 12 行摘要（不重复）" "12" "$(summary_lines)"
	assert_eq "TC-B09 12 片各再跑一次（runs=2）" "12" "$(q 'SELECT COUNT(*) FROM patrol_shard WHERE runs=2;')"

	# TC-B10 --shards 12 == 总数
	reset_db
	run_fx "$FI/be-green.out" 0 --shards 12
	assert_eq "TC-B10 恰跑 12 片" "12" "$(q 'SELECT COUNT(*) FROM patrol_shard;')"
	assert_eq "TC-B10 be-all 不在其中" "0" "$(q "SELECT COUNT(*) FROM patrol_shard WHERE shard='be-all';")"

	# TC-B11 DRY_RUN 不执行不写库
	reset_db
	before="$(snapshot)"
	p TEST_PATROL_DRY_RUN=1 bash "$SOURCE"
	assert_rc "TC-B11 DRY_RUN exit 0" 0
	assert_has "TC-B11 打印 cwd" "$OUT" 'cwd='
	assert_has "TC-B11 逐参数一行" "$OUT" 'arg='
	assert_eq "TC-B11 三表零变化" "$before" "$(snapshot)"

	# TC-B12 枚举完整性 + 与目录现状一致
	p bash "$SOURCE" --help
	missing=""
	for s in "${SHARDS_STATIC[@]}" be-all; do
		case "$OUT" in *"$s"*) ;; *) missing="$missing $s" ;; esac
	done
	if [ -z "$missing" ]; then ok "TC-B12 --help 逐字列出 12 轮转片 + be-all"; else bad "TC-B12 --help 缺分片：$missing"; fi
	fe_total=0
	for d in app/features/tags app/features/discovery app/features/settings app/features/articles app/features/ai app/features/shell app/api app/utils app/stores app/plugins app/assets app/composables app/components; do
		n="$(find "$REPO_ROOT/front/$d" -name '*.test.ts' 2>/dev/null | wc -l | tr -d ' ')"
		fe_total=$((fe_total + n))
		[ "$n" -ge 1 ] || bad "TC-B12 前端片目录 $d 无 *.test.ts"
	done
	root_n="$(find "$REPO_ROOT/front/app" -maxdepth 1 -name '*.test.ts' | wc -l | tr -d ' ')"
	fe_total=$((fe_total + root_n))
	all_n="$(find "$REPO_ROOT/front/app" -name '*.test.ts' | wc -l | tr -d ' ')"
	assert_eq "TC-B12 前端 6 片覆盖全部测试文件（$fe_total/$all_n）" "$all_n" "$fe_total"
	be_rows=""
	for d in admin dataenrichment reader tagmanagement topicgraph platform models app; do
		n="$(find "$REPO_ROOT/backend-go/internal/$d" -name '*.go' 2>/dev/null | wc -l | tr -d ' ')"
		[ "$n" -ge 1 ] || bad "TC-B12 后端片目录 internal/$d 无 .go 文件"
	done
	be_rows="$(find "$REPO_ROOT/backend-go/internal" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
	assert_eq "TC-B12 后端 8 个 internal 顶层目录都被分片覆盖" "8" "$be_rows"
	assert_eq "TC-B12 cmd/ 子目录非空" "5" "$(find "$REPO_ROOT/backend-go/cmd" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
}

# ---------------------------------------------------------------------------
# C 组：执行引擎与失败解析
# ---------------------------------------------------------------------------
group_C() {
	group "C 组 执行引擎与失败解析"

	# TC-C01/C02/C03 命令形态（DRY_RUN）
	p TEST_PATROL_DRY_RUN=1 bash "$SOURCE" --shard be-reader
	args="$(printf '%s' "$OUT" | sed -n 's/^\[DRY-RUN\] arg=//p' | tr '\n' ' ' | sed 's/ $//')"
	assert_eq "TC-C01 后端命令逐参数" "go test -short -count=1 ./internal/reader/..." "$args"
	assert_has "TC-C01 cwd=backend-go" "$OUT" 'cwd=backend-go'
	assert_not_has "TC-C01 无 ./... 全仓模式" "$args" ' ./... '
	assert_not_has "TC-C01 无 -run" "$args" ' -run'

	p TEST_PATROL_DRY_RUN=1 bash "$SOURCE" --shard fe-tags
	args="$(printf '%s' "$OUT" | sed -n 's/^\[DRY-RUN\] arg=//p' | tr '\n' ' ' | sed 's/ $//')"
	assert_has "TC-C02 cwd=front" "$OUT" 'cwd=front'
	assert_has "TC-C02 含 pnpm test:unit" "$args" 'pnpm test:unit'
	assert_has "TC-C02 含 filter app/features/tags" "$args" 'app/features/tags'
	assert_has "TC-C02 含 --maxWorkers=2" "$args" '--maxWorkers=2'
	assert_eq "TC-C02 参数序列无单独的 -- 元素" "0" "$(printf '%s' "$OUT" | sed -n 's/^\[DRY-RUN\] arg=//p' | grep -cx -- '--' || true)"

	p TEST_PATROL_DRY_RUN=1 bash "$SOURCE" --shards 13
	cwd_n="$(printf '%s' "$OUT" | grep -c '^\[DRY-RUN\] cwd=')"
	assert_eq "TC-C03 12 片各恰 1 次调用（不重试）" "12" "$cwd_n"
	assert_eq "TC-C03 全片命令无单独的 -- 元素" "0" "$(printf '%s' "$OUT" | sed -n 's/^\[DRY-RUN\] arg=//p' | grep -cx -- '--' || true)"
	assert_eq "TC-C03 --shards 13 不执行 be-all" "0" "$(printf '%s' "$OUT" | grep -c 'internal/\.\.\. \./cmd' || true)"

	# TC-C04 后端全绿
	reset_db
	run_fx "$FI/be-green.out" 0 --shard be-admin
	assert_rc "TC-C04 全绿 exit 0" 0
	assert_eq "TC-C04 台账零变" "0" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C04 payload.fails == []" "0" "$(q "SELECT json_array_length(payload,'\$.fails') FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"

	# TC-C05 后端失败解析（剥 module 前缀）
	reset_db
	run_fx "$FI/be-fail1.out" 1 --shard be-reader
	assert_eq "TC-C05 test_id 恰为 internal/reader/service::TestA" "internal/reader/service::TestA" "$(q 'SELECT test_id FROM test_debt;')"
	assert_eq "TC-C05 无额外记录" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"

	# TC-C06 子测试保留 /
	reset_db
	run_fx "$FI/be-subtest.out" 1 --shard be-reader
	assert_eq "TC-C06 父子两条且 / 原样保留" "internal/reader/service::TestA,internal/reader/service::TestA/case_x" \
		"$(q "SELECT group_concat(test_id,',') FROM (SELECT test_id FROM test_debt ORDER BY test_id);")"

	# TC-C07 两包混排不串包
	reset_db
	run_fx "$FI/be-twopkg.out" 1 --shard be-skeleton
	assert_eq "TC-C07 两条归属正确" "internal/admin/handler::TestA,internal/reader/store::TestB" \
		"$(q "SELECT group_concat(test_id,',') FROM (SELECT test_id FROM test_debt ORDER BY test_id);")"

	# TC-C08 构建失败标识
	reset_db
	run_fx "$FI/be-buildfailed.out" 1 --shard be-admin
	assert_rc "TC-C08 构建失败 exit 1" 1
	assert_eq "TC-C08 恰 1 条 <build-failed>" "internal/foo::<build-failed>" "$(q 'SELECT test_id FROM test_debt;')"

	# TC-C09 前端整文件级
	reset_db
	run_fx "$FI/fe-filelevel.out" 1 --shard fe-core
	assert_eq "TC-C09 整文件级 test_id" "front/app/plugins/chunk-error-fallback.test.ts::<file-level>" "$(q 'SELECT test_id FROM test_debt;')"

	# TC-C10 前端用例级（嵌套 describe 逐字保留）
	reset_db
	run_fx "$FI/fe-caselevel.out" 1 --shard fe-composables
	assert_eq "TC-C10 用例级 test_id 逐字保留" 'front/app/composables/useOnboarding.test.ts::useOnboarding > first-run detection (isFirstRun) > is true when syntopica_onboarding_complete === "absent"' "$(q 'SELECT test_id FROM test_debt;')"

	# TC-C11 双 reporter 段去重（17 条）
	reset_db
	run_fx "$FI/fe-dup17.out" 1 --shard fe-composables
	assert_eq "TC-C11 台账恰 17 条（不双计）" "17" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C11 payload.fails 长度 17" "17" "$(q "SELECT json_array_length(payload,'\$.fails') FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"
	assert_eq "TC-C11 无残留耗时后缀（[0-9]ms / [0-9]s）" "0" "$(q "SELECT COUNT(*) FROM test_debt WHERE test_id GLOB '*[0-9]ms' OR test_id GLOB '*[0-9]s';")"
	# 真实 vitest 形态回归（fe-discovery 实跑取样）：× 明细（无路径 + 带耗时）与 FAIL 尾段同 id 去重 + ❯ 堆栈行不污染
	reset_db
	run_fx "$FI/fe-real.out" 1 --shard fe-discovery
	assert_eq "TC-C11b 真实 vitest 形态恰 2 条（× 明细 vs FAIL 尾段去重）" "2" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C11b 堆栈行 ❯ path:line:col 不被当文件上下文" "0" "$(q "SELECT COUNT(*) FROM test_debt WHERE test_id LIKE '%:267:58%';")"
	assert_eq "TC-C11b 已确认 id 无耗时后缀" "2" "$(q "SELECT COUNT(*) FROM test_debt WHERE test_id LIKE 'front/app/features/discovery/components/%::Candidate% > %';")"

	# TC-C12 退出码映射三口径
	reset_db
	run_fx "$FI/be-green.out" 0 --shard be-admin
	assert_rc "TC-C12① FAKE_RC=0 + 全绿 → exit 0" 0
	run_fx "$FI/be-fail1.out" 1 --shard be-reader
	assert_rc "TC-C12② FAKE_RC=1 + 有 FAIL → exit 1" 1
	before="$(snapshot)"
	run_fx "$FI/empty.out" 1 --shard be-reader
	assert_rc "TC-C12③ FAKE_RC=1 + 零 FAIL → exit 2" 2
	assert_has "TC-C12③ 提示无法解析失败明细" "$OUT" '无法解析失败明细'
	assert_eq "TC-C12③ 不新增台账行（前 $before → 后 $(snapshot)）" "$(printf '%s' "$before" | cut -d'|' -f1)" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C12③ patrol.check 仍写 ok=false/fails=[]" "1|0" \
		"$(q "SELECT (json_extract(payload,'\$.ok') = 0), json_array_length(payload,'\$.fails') FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"
	assert_eq "TC-C12③ 进度 last_ok=0" "0" "$(q "SELECT last_ok FROM patrol_shard WHERE shard='be-reader';")"

	# TC-C13 失败片四件事齐备
	reset_db
	run_fx "$FI/be-fail1.out" 1 --shard be-reader
	assert_rc "TC-C13 exit 1 不短路" 1
	assert_eq "TC-C13 ①台账新增" "1" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C13 ②patrol.check 写入" "1" "$(q "SELECT COUNT(*) FROM events WHERE kind='patrol.check';")"
	assert_eq "TC-C13 ③进度 last_ok=0/last_fails=1" "0|1" "$(q "SELECT last_ok||'|'||last_fails FROM patrol_shard WHERE shard='be-reader';")"
	assert_has "TC-C13 ④stdout 摘要" "$OUT" '✗ [be-reader]'

	# TC-C14 全绿片零台账变更 + 耗时口径
	sqlite3 -batch "$DB" "DELETE FROM patrol_shard WHERE shard='be-reader';" >/dev/null
	sqlite3 -batch "$DB" "INSERT INTO patrol_shard (shard,last_run,last_ok,last_ms,last_fails,runs,total_fails) VALUES ('be-reader','$NOW0',0,9,3,1,3);" >/dev/null
	before_rows="$(q 'SELECT COUNT(*) FROM test_debt;')"
	run_fx "$FI/be-green.out" 0 --shard be-reader
	assert_eq "TC-C14 不新增台账行" "$before_rows" "$(q 'SELECT COUNT(*) FROM test_debt;')"
	assert_eq "TC-C14 last_ok=1/last_fails=0/runs+1" "1|0|2" "$(q "SELECT last_ok||'|'||last_fails||'|'||runs FROM patrol_shard WHERE shard='be-reader';")"
	assert_eq "TC-C14 total_fails 不变（累加 0）" "3" "$(q "SELECT total_fails FROM patrol_shard WHERE shard='be-reader';")"
	ms="$(q "SELECT last_ms FROM patrol_shard WHERE shard='be-reader';")"
	pms="$(q "SELECT json_extract(payload,'\$.ms') FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"
	if [ "$ms" -ge 0 ] 2>/dev/null && [ "$ms" = "$pms" ]; then
		ok "TC-C14 last_ms 非负且 == payload.ms（$ms）"
	else
		bad "TC-C14 last_ms=$ms / payload.ms=$pms 不一致或为负"
	fi
}

# ---------------------------------------------------------------------------
# V 组：事件记账
# ---------------------------------------------------------------------------
group_V() {
	group "V 组 patrol.check 事件记账"

	reset_db
	run_fx "$FI/be-fail1.out" 1 --shard be-reader
	assert_eq "TC-V01 恰 1 条 patrol.check" "1" "$(q "SELECT COUNT(*) FROM events WHERE kind='patrol.check';")"
	assert_eq "TC-V01 payload 键集合恰为 {shard,ok,ms,fails}" "fails,ms,ok,shard" \
		"$(q "SELECT group_concat(k,',') FROM (SELECT key AS k FROM json_each((SELECT payload FROM events WHERE kind='patrol.check')) ORDER BY key);")"
	assert_eq "TC-V01 JSON 合法 / ok=false / has_ms / fails 含该 id" "1|0|1|1" \
		"$(q "SELECT json_valid(payload), json_extract(payload,'\$.ok'), (json_extract(payload,'\$.ms') IS NOT NULL), (SELECT COUNT(*) FROM json_each((SELECT payload FROM events WHERE kind='patrol.check')) WHERE key='fails' AND value LIKE '%internal/reader/service::TestA%') FROM events WHERE kind='patrol.check';")"
	assert_eq "TC-V02 change IS NULL / session_id 非空 / ts ISO8601 UTC" "1|1|1" \
		"$(q "SELECT (change IS NULL), (session_id <> ''), (ts GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z') FROM events WHERE kind='patrol.check';")"

	run_fx "$FI/be-green.out" 0 --shard be-admin
	assert_eq "TC-V03 全绿 payload.ok=true / fails=[]（空数组非 null）" "1|0|1" \
		"$(q "SELECT json_extract(payload,'\$.ok'), json_array_length(payload,'\$.fails'), json_type(payload,'\$.fails')='array' FROM events WHERE kind='patrol.check' ORDER BY id DESC LIMIT 1;")"

	# TC-V04 保留期 30 天（静态断言；行为断言在 .pi/extensions/tests/harness-log.smoke.cjs）
	retention="$(grep -c '"patrol.check": 30' "$REPO_ROOT/.pi/extensions/lib/harness-log.ts" || true)"
	assert_eq "TC-V04 harness-log RETENTION_DAYS 含 patrol.check:30" "1" "$retention"
	assert_eq "TC-V04 扩展侧行为 smoke 已覆盖 patrol.check" "1" \
		"$(grep -c 'patrol.check' "$REPO_ROOT/.pi/extensions/tests/harness-log.smoke.cjs" | awk '{print ($1 >= 1)}')"

	# TC-V05 事件删除不影响台账（不双写）
	ledger_before="$(qr 'SELECT test_id,status,fixed_by,first_seen,last_seen FROM test_debt;')"
	progress_before="$(qr 'SELECT * FROM patrol_shard;')"
	p TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --report
	report_before="$OUT"
	sqlite3 -batch "$DB" "DELETE FROM events WHERE kind='patrol.check';" >/dev/null
	assert_eq "TC-V05 台账零变化" "$ledger_before" "$(qr 'SELECT test_id,status,fixed_by,first_seen,last_seen FROM test_debt;')"
	assert_eq "TC-V05 进度零变化" "$progress_before" "$(qr 'SELECT * FROM patrol_shard;')"
	p TEST_PATROL_NOW="$NOW0" bash "$SOURCE" --report
	assert_eq "TC-V05 --report 输出不变" "$report_before" "$OUT"

	# TC-V06 库不可写 → fail-open
	reset_db
	before="$(snapshot)"
	mkdir -p "$TMP/isdir"
	p TEST_PATROL_DB="$TMP/isdir" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_rc "TC-V06 不可写库仍 exit 0（fail-open）" 0
	assert_has "TC-V06 告警含「写入失败」与原因" "$OUT" '写入失败'
	assert_has "TC-V06 分片摘要照常打印" "$OUT" '✓ [be-admin]'
	assert_eq "TC-V06 原库无半条脏行" "$before" "$(snapshot)"

	# TC-V07 他人库拒绝
	FOREIGN="$TMP/foreign.db"
	rm -f "$FOREIGN"
	sqlite3 -batch "$FOREIGN" "PRAGMA application_id=123; CREATE TABLE someone_else(x);" >/dev/null
	p TEST_PATROL_DB="$FOREIGN" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard fe-core
	assert_rc "TC-V07 他人库 fail-open exit 0" 0
	assert_has "TC-V07 告警可见" "$OUT" '写入失败'
	assert_eq "TC-V07 不建 test_debt/patrol_shard" "someone_else" "$(sqlite3 -batch "$FOREIGN" "SELECT group_concat(name) FROM sqlite_master WHERE type='table';")"
	assert_eq "TC-V07 不 INSERT events" "0" "$(sqlite3 -batch "$FOREIGN" "SELECT COUNT(*) FROM sqlite_master WHERE name='events';")"

	# TC-V08 未来版本库拒绝
	FUTURE="$TMP/future.db"
	rm -f "$FUTURE"
	sqlite3 -batch "$FUTURE" "PRAGMA application_id=1398361684; PRAGMA user_version=2; CREATE TABLE t(x);" >/dev/null
	p TEST_PATROL_DB="$FUTURE" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard fe-core
	assert_rc "TC-V08 未来版本库 fail-open exit 0" 0
	assert_has "TC-V08 告警含版本原因" "$OUT" 'user_version 2'
	assert_eq "TC-V08 不建新表" "t" "$(sqlite3 -batch "$FUTURE" "SELECT group_concat(name) FROM sqlite_master WHERE type='table';")"

	# TC-V10 busy_timeout + WAL
	assert_eq "TC-V10 源码含 busy_timeout" "1" "$(awk '/busy_timeout/{n++} END{print (n>=1)}' "$SOURCE")"
	assert_eq "TC-V10 写入后 journal_mode 仍为 wal" "wal" "$(q 'PRAGMA journal_mode;')"
}

# ---------------------------------------------------------------------------
# R 组：报告聚合
# ---------------------------------------------------------------------------
group_R() {
	group "R 组 报告聚合"

	# TC-R01 空库
	reset_db
	p bash "$SOURCE" --report
	assert_rc "TC-R01 空库 --report exit 0" 0
	assert_has "TC-R01 三态计数行" "$OUT" 'open 0 / fixed 0 / waived 0'
	open_empty="$(printf '%s' "$OUT" | grep -A1 'open 清单' | tail -1 | tr -d ' ')"
	assert_eq "TC-R01 open 清单为空提示" "（空）" "$open_empty"
	assert_has "TC-R01 stale 段无条目" "$OUT" '（无）'
	assert_has "TC-R01 进度段暂无记录" "$OUT" '（暂无巡检记录）'
	assert_not_has "TC-R01 无 SQL 报错" "$OUT" 'Error:'

	# TC-R02 / TC-R03 三态 + domain 聚合
	sqlite3 -batch "$DB" "
INSERT INTO test_debt (test_id,domain,first_seen,last_seen,context,status) VALUES
 ('r::o1','be-reader','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z','c','open'),
 ('r::o2','be-reader','2026-09-02T00:00:00Z','2026-09-02T00:00:00Z','c','open'),
 ('r::o3','be-reader','2026-09-03T00:00:00Z','2026-09-03T00:00:00Z','c','open'),
 ('r::o4','manual','2026-09-04T00:00:00Z','2026-09-04T00:00:00Z','c','open'),
 ('r::o5','fe-core','2026-09-05T00:00:00Z','2026-09-05T00:00:00Z','c','open'),
 ('r::f1','be-reader','2026-09-06T00:00:00Z','2026-09-06T00:00:00Z','c','fixed'),
 ('r::f2','be-reader','2026-09-07T00:00:00Z','2026-09-07T00:00:00Z','c','fixed'),
 ('r::w1','manual','2026-09-08T00:00:00Z','2026-09-08T00:00:00Z','c','waived');
" >/dev/null
	p bash "$SOURCE" --report
	assert_has "TC-R02 状态计数精确 5/2/1" "$OUT" 'open 5 / fixed 2 / waived 1'
	assert_has "TC-R03 domain 聚合 be-reader=5" "$OUT" 'be-reader  5'
	assert_has "TC-R03 domain 聚合 manual=2" "$OUT" 'manual  2'
	assert_has "TC-R03 domain 聚合 fe-core=1" "$OUT" 'fe-core  1'

	# TC-R04 open 清单按 first_seen 升序
	reset_db
	sqlite3 -batch "$DB" "
INSERT INTO test_debt (test_id,domain,first_seen,last_seen,context,status) VALUES
 ('r::late','d','2026-09-03T00:00:00Z','2026-09-03T00:00:00Z','c','open'),
 ('r::early','d','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z','c','open'),
 ('r::mid','d','2026-09-02T00:00:00Z','2026-09-02T00:00:00Z','c','open'),
 ('r::done','d','2026-08-01T00:00:00Z','2026-08-01T00:00:00Z','c','fixed');
" >/dev/null
	p bash "$SOURCE" --report
	order="$(printf '%s' "$OUT" | sed -n '/open 清单/,/stale/p' | grep -oE 'r::[a-z]+' | tr '\n' ' ' | sed 's/ $//')"
	assert_eq "TC-R04 open 清单按 first_seen 升序" "r::early r::mid r::late" "$order"
	assert_not_has "TC-R04 fixed 不进 open 清单" "$(printf '%s' "$OUT" | sed -n '/open 清单/,/stale/p')" 'r::done'

	# TC-R05 / TC-R06 stale 过滤与 ±1s 边界
	reset_db
	T="2026-10-17T00:00:00Z"
	sqlite3 -batch "$DB" "
INSERT INTO test_debt (test_id,domain,first_seen,last_seen,context,status) VALUES
 ('s::31d','d','$T','2026-09-16T23:59:59Z','c','open'),
 ('s::1d','d','$T','2026-10-16T00:00:00Z','c','open'),
 ('s::fixed40d','d','$T','2026-09-01T00:00:00Z','c','fixed'),
 ('s::30d-1s','d','$T','$(date -u -d "$T - 30 days - 1 second" +%Y-%m-%dT%H:%M:%SZ)','c','open'),
 ('s::30d','d','$T','$(date -u -d "$T - 30 days" +%Y-%m-%dT%H:%M:%SZ)','c','open'),
 ('s::30d+1s','d','$T','$(date -u -d "$T - 30 days + 1 second" +%Y-%m-%dT%H:%M:%SZ)','c','open');
" >/dev/null
	p TEST_PATROL_NOW="$T" bash "$SOURCE" --report
	stale="$(printf '%s' "$OUT" | sed -n '/^stale/,$p' | grep -oE 's::[0-9a-z+-]+' | tr '\n' ' ' | sed 's/ $//')"
	assert_eq "TC-R06 stale 三段边界（仅 30d+1s 入选）" "s::30d-1s s::31d" "$stale"
	assert_not_has "TC-R05 open 新记录不进 stale" "$stale" 's::1d'
	assert_not_has "TC-R05 fixed 老记录不进 stale" "$stale" 's::fixed40d'

	# TC-R07 有欠账也 exit 0
	assert_rc "TC-R07 有欠账仍 exit 0（报告不是门禁）" 0

	# TC-R08 进度段字段齐全 + total_fails 累加口径
	reset_db
	run_fx "$FI/be-fail1.out" 1 --shard be-reader
	p TEST_PATROL_FAKE_OUTPUT="$FI/fe-dup1.out" TEST_PATROL_FAKE_RC=1 bash "$SOURCE" --shard fe-tags
	p bash "$SOURCE" --report
	progress_section="$(printf '%s' "$OUT" | sed -n '/巡检进度/,$p')"
	for f in last_run= last_ok= last_ms= last_fails= runs= total_fails=; do
		case "$progress_section" in *"$f"*) ;; *) bad "TC-R08 进度段缺字段 $f" ;; esac
	done
	ok "TC-R08 进度段字段齐全（last_run/last_ok/last_ms/last_fails/runs/total_fails）"
	assert_has "TC-R08 be-reader total_fails=1" "$progress_section" 'be-reader  last_run='
	assert_eq "TC-R08 两片 total_fails 分别 1/1、runs 各 1" "1|1|1|1" \
		"$(q "SELECT (SELECT total_fails FROM patrol_shard WHERE shard='be-reader'), (SELECT total_fails FROM patrol_shard WHERE shard='fe-tags'), (SELECT runs FROM patrol_shard WHERE shard='be-reader'), (SELECT runs FROM patrol_shard WHERE shard='fe-tags');")"

	# TC-A09 补：A-9 累加语义（重复复现也计入 total_fails）
	p TEST_PATROL_FAKE_OUTPUT="$FI/be-fail1.out" TEST_PATROL_FAKE_RC=1 bash "$SOURCE" --shard be-reader
	assert_eq "TC-A09 total_fails 累加（1 → 2）" "2" "$(q "SELECT total_fails FROM patrol_shard WHERE shard='be-reader';")"

	# TC-R09 --report 只读
	before_dump="$(sqlite3 -batch "$DB" ".dump")"
	before="$(snapshot)"
	p bash "$SOURCE" --report
	p bash "$SOURCE" --report
	assert_eq "TC-R09 三表行数不变" "$before" "$(snapshot)"
	assert_eq "TC-R09 全库内容逐字节不变" "$before_dump" "$(sqlite3 -batch "$DB" ".dump")"
}

# ---------------------------------------------------------------------------
# P 组：资源预检
# ---------------------------------------------------------------------------
group_P() {
	group "P 组 资源预检（提醒不阻断）"

	reset_db
	p TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-low" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_eq "TC-P01 低负载无告警行（基准）" "0" "$(printf '%s' "$OUT" | grep -c '负载偏高' || true)"
	assert_has "TC-P01 巡检正常（摘要）" "$OUT" '✓ [be-admin]'
	assert_eq "TC-P01 记账齐备" "1|1" "$(q "SELECT (SELECT COUNT(*) FROM patrol_shard), (SELECT COUNT(*) FROM events WHERE kind='patrol.check');")"

	p TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-4.00" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_eq "TC-P02 4.00 无告警（阈值为严格 >4）" "0" "$(printf '%s' "$OUT" | grep -c '负载偏高' || true)"
	p TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-4.01" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_has "TC-P02 4.01 有告警（含数值）" "$OUT" 'load average(1min)=4.01'
	assert_has "TC-P02 告警含延后/错峰建议" "$OUT" '错峰'

	reset_db
	p TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-8.30" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_rc "TC-P03 高负载仍照常执行（exit 0）" 0
	assert_has "TC-P03 摘要存在" "$OUT" '✓ [be-admin]'
	assert_eq "TC-P03 台账/事件照写" "1|1" "$(q "SELECT (SELECT COUNT(*) FROM patrol_shard), (SELECT COUNT(*) FROM events WHERE kind='patrol.check');")"

	p TEST_PATROL_PROCESS_LIST="$TMP/procs-build" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_has "TC-P04 检出并发构建进程（含类别）" "$OUT" '类别：构建'
	assert_has "TC-P04 仍继续执行" "$OUT" '✓ [be-admin]'
	p TEST_PATROL_PROCESS_LIST="$TMP/procs-browser" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_has "TC-P04 检出浏览器自动化（含类别）" "$OUT" '浏览器自动化'

	p TEST_PATROL_LOADAVG_FILE="$TMP/loadavg-8.30" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	warn_ln="$(printf '%s' "$OUT" | grep -n '负载偏高' | head -1 | cut -d: -f1)"
	sum_ln="$(printf '%s' "$OUT" | grep -n '^✓ \[' | head -1 | cut -d: -f1)"
	if [ -n "$warn_ln" ] && [ -n "$sum_ln" ] && [ "$warn_ln" -lt "$sum_ln" ]; then
		ok "TC-P05 告警行号($warn_ln) < 分片摘要行号($sum_ln)：预检在跑之前"
	elif [ -z "$warn_ln" ]; then
		bad "TC-P05 高负载未出告警行"
	else
		bad "TC-P05 预检时机不对（warn=$warn_ln sum=$sum_ln）"
	fi

	before="$(snapshot)"
	p TEST_PATROL_LOADAVG_FILE="$TMP/does-not-exist" TEST_PATROL_FAKE_OUTPUT="$FI/be-green.out" TEST_PATROL_FAKE_RC=0 bash "$SOURCE" --shard be-admin
	assert_rc "TC-P06 负载文件不可读不阻断（exit 0）" 0
	assert_has "TC-P06 允许提示无法读取负载" "$OUT" '无法读取负载信息'
	assert_has "TC-P06 摘要与记账照常" "$OUT" '✓ [be-admin]'
	assert_eq "TC-P06 预检本身不写三表之外的状态" "0" "$(q "SELECT COUNT(*) FROM test_debt;")"
	: "$before"
}

# ---------------------------------------------------------------------------
# X 组：并发与输入安全
# ---------------------------------------------------------------------------
group_X() {
	group "X 组 并发与输入安全"

	# TC-X01/X02 并发两片写库（直接后台执行，输出落文件；带 sleep 制造写入窗口重叠）
	reset_db
	env TEST_PATROL_FAKE_OUTPUT="$FI/green-long.out" TEST_PATROL_FAKE_RC=0 TEST_PATROL_FAKE_SLEEP=0.4 bash "$SOURCE" --shard be-admin >"$TMP/x1.out" 2>&1 &
	pid1=$!
	env TEST_PATROL_FAKE_OUTPUT="$FI/green-long.out" TEST_PATROL_FAKE_RC=0 TEST_PATROL_FAKE_SLEEP=0.4 bash "$SOURCE" --shard be-reader >"$TMP/x2.out" 2>&1 &
	pid2=$!
	wait $pid1
	rc1=$?
	wait $pid2
	rc2=$?
	cat "$TMP/x1.out" "$TMP/x2.out" >"$TMP/x-both.out"
	assert_eq "TC-X01 两进程各自 exit 0（$rc1|$rc2）" "0|0" "$rc1|$rc2"
	assert_has "TC-X01 两片摘要均存在" "$(cat "$TMP/x-both.out")" '✓ [be-reader]'
	assert_eq "TC-X01 无 SQLITE_BUSY / database is locked" "0" "$(grep -ciE 'SQLITE_BUSY|database is locked' "$TMP/x-both.out" || true)"
	assert_eq "TC-X01 进度两行都在" "2" "$(q "SELECT COUNT(*) FROM patrol_shard WHERE shard IN ('be-admin','be-reader');")"
	assert_eq "TC-X01 patrol.check 恰 2 条（无丢失）" "2" "$(q "SELECT COUNT(*) FROM events WHERE kind='patrol.check';")"
	assert_eq "TC-X02 journal_mode = wal" "wal" "$(q 'PRAGMA journal_mode;')"
	if [ -f "$DB-wal" ]; then ok "TC-X02 写入期 -wal 文件存在"; else ok "TC-X02 journal_mode=wal（-wal 已 checkpoint 回收，模式为持久证据）"; fi

	# TC-X03 分片名注入式输入
	reset_db
	before="$(snapshot)"
	p bash "$SOURCE" --shard "be-admin'; DROP TABLE test_debt;--"
	assert_rc "TC-X03 注入式分片名 exit 2" 2
	assert_eq "TC-X03 test_debt 仍存在" "1" "$(q "SELECT COUNT(*) FROM sqlite_master WHERE name='test_debt';")"
	assert_eq "TC-X03 三表零变化" "$before" "$(snapshot)"

	# TC-X04 test_id 注入式输入
	INJ="internal/x::Test'; DROP TABLE test_debt;--"
	p bash "$SOURCE" --register "$INJ" --context 'test-debt-patrol'
	assert_rc "TC-X04 注入式 test_id register exit 0" 0
	assert_eq "TC-X04 读回逐字节相等" "$INJ" "$(q "SELECT test_id FROM test_debt WHERE test_id LIKE 'internal/x%';")"
	assert_eq "TC-X04 表仍在" "1" "$(q "SELECT COUNT(*) FROM sqlite_master WHERE name='test_debt';")"
	p bash "$SOURCE" --report
	assert_rc "TC-X04 --report 无 SQL 语法错" 0
	assert_not_has "TC-X04 --report 无 SQL 报错" "$OUT" 'Error:'

	# TC-X05 特殊字符转义闭环（写读同一路径）
	SPECIAL='front/app/x.test.ts::a > b "q" / [x]'
	p bash "$SOURCE" --register "$SPECIAL" --context c
	assert_eq "TC-X05 特殊字符读回逐字节相等" "$SPECIAL" "$(q "SELECT test_id FROM test_debt WHERE test_id LIKE 'front/app/x.test.ts::a%';")"
	p bash "$SOURCE" --resolve "$SPECIAL" --by spec-change
	assert_rc "TC-X05 --resolve 命中同一行" 0
	assert_eq "TC-X05 该行 status=fixed" "fixed|spec-change" "$(q "SELECT status||'|'||fixed_by FROM test_debt WHERE test_id='$SPECIAL';")"

	# TC-X06 4KB 超长 test_id（A-7 接受并原样存回）
	LONG="$(head -c 4096 /dev/zero | tr '\0' 'L')"
	p bash "$SOURCE" --register "$LONG" --context big
	assert_rc "TC-X06 4KB test_id 接受（exit 0）" 0
	assert_eq "TC-X06 读回长度 == 输入长度（无截断）" "4096" "$(q "SELECT length(test_id) FROM test_debt WHERE context='big';")"

	# TC-X07 note 含换行/引号
	NOTE=$'第一行\n第二行 "引号"'
	p bash "$SOURCE" --register 'note-case::x' --context c --note "$NOTE"
	assert_rc "TC-X07 note 含换行/引号 register exit 0" 0
	assert_eq "TC-X07 note 读回逐字节相等" "$NOTE" "$(q "SELECT note FROM test_debt WHERE test_id='note-case::x';")"
	p bash "$SOURCE" --report
	assert_rc "TC-X07 --report 可读结构不被换行破坏" 0
}

# ---------------------------------------------------------------------------
# SUR 组：存量摸底复核
# ---------------------------------------------------------------------------
group_SUR() {
	group "SUR 组 存量摸底复核"

	reset_db
	p bash "$SOURCE" --register 'front/app/plugins/chunk-error-fallback.test.ts::<file-level>' --context '存量摸底' --domain fe-core
	p bash "$SOURCE" --register 'front/app/composables/useOnboarding.test.ts::<17 条用例级>' --context '存量摸底' --domain fe-composables
	p bash "$SOURCE" --report
	assert_has "TC-SUR01 存量红 1 已登记且 open" "$OUT" 'front/app/plugins/chunk-error-fallback.test.ts::<file-level>'
	assert_has "TC-SUR01 存量红 2 已登记且 open" "$OUT" 'front/app/composables/useOnboarding.test.ts::<17 条用例级>'
	assert_has "TC-SUR01 发现语境可辨识" "$OUT" 'context=存量摸底'
	assert_has "TC-SUR01 状态计数 open 2" "$OUT" 'open 2 / fixed 0 / waived 0'

	survey="$REPO_ROOT/openspec/changes/archive/2026-09-18-test-debt-patrol/survey.md"
	design="$REPO_ROOT/openspec/changes/archive/2026-09-18-test-debt-patrol/design.md"
	assert_has "TC-SUR02 survey 记录存量红 2 文件 / 18 用例" "$(sed -n '1,40p' "$survey")" '2 个文件 / 18 个用例'
	assert_has "TC-SUR02 survey 记录后端 -short 全量 42s" "$(sed -n '1,40p' "$survey")" '**42s**'
	assert_has "TC-SUR02 design 保留后端 6 片定夺" "$(sed -n '1,80p' "$design")" '共 6 片'
	p bash "$SOURCE" --help
	assert_eq "TC-SUR02 分片枚举 12+1 与 design 一致" "13" "$(printf '%s' "$OUT" | grep -cE '^  (be|fe)-[a-z-]+ ')"
}

# ---------------------------------------------------------------------------
# 汇总
# ---------------------------------------------------------------------------
group_F
group_A
group_B
group_C
group_V
group_R
group_P
group_X
group_SUR

printf '\ntest-patrol smoke：通过 %d / 失败 %d\n' "$pass" "$fail"
[ "$fail" -eq 0 ] && printf 'SMOKE OK\n'
[ "$fail" -eq 0 ]
