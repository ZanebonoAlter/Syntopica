#!/usr/bin/env bash
# test-patrol.sh — 测试欠账滚动巡检（change: test-debt-patrol；白盒用例 openspec/changes/test-debt-patrol/test-cases.md）
#
# 解决「发现了红」与「红被处理」之间的缺口：把全量测试拆成静态分片，按「最久未巡优先」一次推一片；
# 失败进 `test_debt` 台账（生命周期可追溯），每次分片执行写一条 `patrol.check` 事件（流水）。
#
# 退出码：0 分片全绿或管理子命令成功；1 分片跑完但检出红；2 用法/环境错
#   （含 runner 非 0 但零 FAIL 明细——「跑不起来」不当测试红记账，见 test-cases.md §7 A-5①）
#
# 存储：`.pi/harness/events.db`（test-debt-patrol design 决策 1/3：台账用独立表，不走 events 表 TTL；
# sqlite3 CLI 直写）。开库安全契约与 `.pi/extensions/lib/harness-log.ts` 对齐：application_id 必须是
# 本应用魔数 0x53594E54、user_version <= 1；他人库/未来版本库一律拒绝建表与写入，且 fail-open
# （告警可见，巡检结果与退出码不受影响——A-5②）。写入走单事务 + `PRAGMA busy_timeout=5000`（WAL），
# SQL 值统一 `'` → `''` 转义。
#
# 测试缝（test-cases.md §1；smoke 专用，日常使用无需设置）：
#   TEST_PATROL_DB            台账 + 事件库路径（默认 .pi/harness/events.db）
#   TEST_PATROL_FAKE_OUTPUT   runner 输出文本文件；设置后不执行真实测试命令，直接消费该文件
#   TEST_PATROL_FAKE_RC       伪造 runner 退出码
#   TEST_PATROL_FAKE_SLEEP    伪造 runner 耗时（秒；并发写库用例需要写入窗口重叠）
#   TEST_PATROL_DRY_RUN       =1 只打印将要执行的命令（cwd + 逐参数一行），不执行、不写库
#   TEST_PATROL_LOADAVG_FILE  覆盖 loadavg 读取路径
#   TEST_PATROL_PROCESS_LIST  覆盖进程清单（每行一条 cmdline）
#   TEST_PATROL_NOW           覆盖「当前时间」（ISO8601，stale/排序的时间旅行）
#
# POSIX bash（Linux 本机直跑；Windows/WSL 宿主执行策略见 standard/frontend/testing.md）。
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# ---------------------------------------------------------------------------
# 常量
# ---------------------------------------------------------------------------
DB_PATH="${TEST_PATROL_DB:-$REPO_ROOT/.pi/harness/events.db}"
APP_ID=1398361684 # 0x53594E54 = ASCII "SYNT" 小端打包（harness-log.ts 同值）
SCHEMA_VERSION=1
SESSION_ID="${PI_SESSION_ID:-patrol.sh}"
STALE_DAYS=30
LOAD_THRESHOLD=4

# 12 片轮转（静态枚举，顺序即冷启动与平局优先级；be-all 不入轮转）
SHARD_ORDER=(
	be-admin be-dataenrichment be-reader be-tagmanagement be-topicgraph be-skeleton
	fe-tags fe-discovery fe-features fe-core fe-composables fe-components
)
SHARD_ALL="be-all"

EVENTS_DDL="
CREATE TABLE IF NOT EXISTS events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,
  session_id TEXT NOT NULL,
  kind       TEXT NOT NULL,
  change     TEXT,
  payload    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ev_session ON events(session_id, id);
CREATE INDEX IF NOT EXISTS idx_ev_change  ON events(change, id);
CREATE INDEX IF NOT EXISTS idx_ev_kind_ts ON events(kind, ts);
"

DEBT_DDL="
CREATE TABLE IF NOT EXISTS test_debt (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  test_id       TEXT NOT NULL UNIQUE,
  domain        TEXT,
  first_seen    TEXT NOT NULL,
  last_seen     TEXT NOT NULL,
  context       TEXT,
  status        TEXT NOT NULL CHECK (status IN ('open','fixed','waived')),
  fixed_by      TEXT,
  waived_reason TEXT,
  waived_at     TEXT,
  note          TEXT
);
CREATE TABLE IF NOT EXISTS patrol_shard (
  shard       TEXT PRIMARY KEY,
  last_run    TEXT,
  last_ok     INTEGER,
  last_ms     INTEGER,
  last_fails  INTEGER,
  runs        INTEGER NOT NULL DEFAULT 0,
  total_fails INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_td_status ON test_debt(status, first_seen);
"

# ---------------------------------------------------------------------------
# 小工具
# ---------------------------------------------------------------------------
TMP_WORK=""
cleanup() { [ -n "$TMP_WORK" ] && rm -rf "$TMP_WORK"; }
trap cleanup EXIT

ensure_tmp() {
	[ -n "$TMP_WORK" ] && return 0
	TMP_WORK="$(mktemp -d "${TMPDIR:-/tmp}/test-patrol.XXXXXX")" || {
		printf '✗ 无法创建临时目录（TMPDIR=%s）\n' "${TMPDIR:-/tmp}" >&2
		exit 2
	}
}

now_iso() {
	if [ -n "${TEST_PATROL_NOW:-}" ]; then
		date -u -d "$TEST_PATROL_NOW" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || printf '%s' "$TEST_PATROL_NOW"
	else
		date -u +%Y-%m-%dT%H:%M:%SZ
	fi
}

now_ms() { date +%s%3N; }

# 30 天 stale 截止：last_seen < cutoff 即「超过 30 天」（严格大于才入选，边界 ±1s 见 test-cases §7 A-2/B-10）
stale_cutoff() { date -u -d "$(now_iso) - $STALE_DAYS days" +%Y-%m-%dT%H:%M:%SZ; }

# SQL 字符串字面量（值一律经此转义，禁拼接裸值）
sqlq() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"; }

strip_ansi() { printf '%s' "$1" | sed -e 's/\x1b\[[0-9;]*[A-Za-z]//g' -e 's/\x1b\[?[0-9;]*[A-Za-z]//g'; }

json_escape() {
	local s="$1"
	s="${s//\\/\\\\}"
	s="${s//\"/\\\"}"
	s="${s//$'\t'/\\t}"
	s="${s//$'\r'/\\r}"
	printf '%s' "$s"
}

usage_hint() {
	printf '  用法：bash scripts/harness/test-patrol.sh [--shard <name> | --shards <n> | --init | --report |\n'
	printf '                                      --register <id> --context <ctx> | --resolve <id> |\n'
	printf '                                      --waive <id> --reason <text> | --help]\n'
	printf '  可选分片（%d 片轮转 + %s）：\n' "${#SHARD_ORDER[@]}" "$SHARD_ALL"
	local s
	for s in "${SHARD_ORDER[@]}"; do printf '    %s\n' "$s"; done
	printf '    %s（不入轮转，仅显式指定）\n' "$SHARD_ALL"
}

die_usage() {
	printf '✗ %s\n' "$1" >&2
	[ -n "${2:-}" ] && printf '  %s\n' "$2" >&2
	usage_hint >&2
	exit 2
}

# ---------------------------------------------------------------------------
# 分片静态枚举 → cwd / 参数向量
# ---------------------------------------------------------------------------
SHARD_KIND=""
SHARD_CWD=""
SHARD_ARGS=()

shard_setup() { # $1=分片名；未知名 → 返回 1
	case "$1" in
	be-admin) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/admin/...) ;;
	be-dataenrichment) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/dataenrichment/...) ;;
	be-reader) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/reader/...) ;;
	be-tagmanagement) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/tagmanagement/...) ;;
	be-topicgraph) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/topicgraph/...) ;;
	be-skeleton)
		SHARD_KIND=backend
		SHARD_CWD=backend-go
		SHARD_ARGS=(go test -short -count=1 ./internal/platform/... ./internal/models/... ./internal/app/... ./cmd/...)
		;;
	be-all) SHARD_KIND=backend; SHARD_CWD=backend-go; SHARD_ARGS=(go test -short -count=1 ./internal/... ./cmd/...) ;;
	fe-tags) SHARD_KIND=frontend; SHARD_CWD=front; SHARD_ARGS=(pnpm test:unit app/features/tags --maxWorkers=2) ;;
	fe-discovery) SHARD_KIND=frontend; SHARD_CWD=front; SHARD_ARGS=(pnpm test:unit app/features/discovery --maxWorkers=2) ;;
	fe-features)
		SHARD_KIND=frontend
		SHARD_CWD=front
		SHARD_ARGS=(pnpm test:unit app/features/settings app/features/articles app/features/ai app/features/shell --maxWorkers=2)
		;;
	fe-core)
		SHARD_KIND=frontend
		SHARD_CWD=front
		SHARD_ARGS=(pnpm test:unit app/api app/utils app/stores app/plugins app/assets --maxWorkers=2)
		;;
	fe-composables) SHARD_KIND=frontend; SHARD_CWD=front; SHARD_ARGS=(pnpm test:unit app/composables --maxWorkers=2) ;;
	fe-components)
		SHARD_KIND=frontend
		SHARD_CWD=front
		SHARD_ARGS=(pnpm test:unit app/components app/error.test.ts app/spa-loading-template.test.ts --maxWorkers=2)
		;;
	*) return 1 ;;
	esac
	return 0
}

shard_known() { shard_setup "$1" 2>/dev/null; }

# ---------------------------------------------------------------------------
# 开库安全（对齐 harness-log.ts design D2）
# ---------------------------------------------------------------------------
DB_OK=0
DB_REASON=""
DB_APP_ID=""
DB_USER_VERSION=""
DB_TABLE_COUNT=""

db_probe() { # 只读探测；不可打开 → 返回 1
	local out
	out="$(sqlite3 -batch "$DB_PATH" "PRAGMA application_id; PRAGMA user_version; SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';" 2>&1)" || {
		DB_REASON="无法打开数据库：$(printf '%s' "$out" | tr '\n' ' ')"
		return 1
	}
	DB_APP_ID="$(printf '%s\n' "$out" | sed -n 1p)"
	DB_USER_VERSION="$(printf '%s\n' "$out" | sed -n 2p)"
	DB_TABLE_COUNT="$(printf '%s\n' "$out" | sed -n 3p)"
	return 0
}

db_open_write() { # 校验 + 建表；成功 DB_OK=1；失败 DB_OK=0 且 DB_REASON 有原因
	DB_OK=0
	DB_REASON=""
	local dir
	dir="$(dirname "$DB_PATH")"
	if ! mkdir -p "$dir" 2>/dev/null; then
		DB_REASON="无法创建目录 $dir"
		return 1
	fi
	db_probe || return 1
	if [ "$DB_APP_ID" = "0" ] && [ "$DB_TABLE_COUNT" = "0" ]; then
		# 全新库：登记魔数 + 版本（写 PRAGMA 必须在校验通过之后）
		if ! sqlite3 -batch "$DB_PATH" "PRAGMA application_id = $APP_ID; PRAGMA user_version = $SCHEMA_VERSION;" >/dev/null 2>&1; then
			DB_REASON="新库登记 application_id 失败（文件不可写？）"
			return 1
		fi
	elif [ "$DB_APP_ID" = "$APP_ID" ] && [ "$DB_USER_VERSION" -le "$SCHEMA_VERSION" ] 2>/dev/null; then
		: # 本应用库（当前版本或半初始化 version=0）
	else
		if [ "$DB_APP_ID" = "0" ]; then
			DB_REASON="app_id 未登记但已有 $DB_TABLE_COUNT 张用户表（他人未登记库，拒绝建表与写入）"
		elif [ "$DB_APP_ID" != "$APP_ID" ]; then
			DB_REASON="app_id $DB_APP_ID 非本应用魔数 $APP_ID（他人库，拒绝建表与写入）"
		else
			DB_REASON="user_version $DB_USER_VERSION 高于当前支持的 $SCHEMA_VERSION（未来版本库，拒绝建表与写入）"
		fi
		return 1
	fi
	ensure_tmp
	local err="$TMP_WORK/ddl.err"
	if ! sqlite3 -batch "$DB_PATH" "PRAGMA busy_timeout=5000; PRAGMA journal_mode = WAL;$EVENTS_DDL$DEBT_DDL" >/dev/null 2>"$err"; then
		DB_REASON="建表失败：$(tr '\n' ' ' <"$err")"
		return 1
	fi
	DB_OK=1
	return 0
}

warn_db_fail_open() {
	printf '⚠ 写入失败：%s\n  （记账已跳过；巡检查出结果与退出码不受影响，fail-open）\n' "$DB_REASON" >&2
}

# ---------------------------------------------------------------------------
# 失败解析（test-cases §1：函数级可单独喂 fixture）
# ---------------------------------------------------------------------------
normalize_pkg() { # 剥 module 前缀与 ./ 前缀（A-1）
	local p="$1"
	p="${p%/}"
	p="${p#syntopica-backend/}"
	p="${p#./}"
	printf '%s' "$p"
}

parse_backend_fails() { # stdin=go test 输出；stdout=test_id（一行一条，未去重）
	local line pkg="" hp
	declare -a pending=() build_failed=()
	while IFS= read -r line || [ -n "$line" ]; do
		line="$(strip_ansi "$line")"
		line="${line%$'\r'}"
		case "$line" in
		'# '*) # 构建失败头：# syntopica-backend/internal/foo [syntopica-backend/internal/foo.test]
			hp="${line#\# }"
			hp="${hp%% \[*}"
			pkg="$(normalize_pkg "$hp")"
			continue
			;;
		esac
		# 用例级：--- FAIL: TestA (0.00s)；子测试缩进形式同规则
		if [[ "$line" =~ ^[[:space:]]*---[[:space:]]+FAIL:[[:space:]]+([^[:space:]]+) ]]; then
			pending+=("${BASH_REMATCH[1]}")
			continue
		fi
		# 包级行：ok|FAIL|? <pkg> [<time>|<说明>]（TAB/空格分隔）
		if [[ "$line" =~ ^(ok|FAIL|\?)[[:space:]]+([^[:space:]]+)($|[[:space:]]+.*) ]]; then
			local kind="${BASH_REMATCH[1]}" this_pkg
			this_pkg="$(normalize_pkg "${BASH_REMATCH[2]}")"
			[ "$kind" = "?" ] && continue
			if [ "$kind" = "FAIL" ] && [[ "$line" == *"[build failed]"* || "$line" == *"[setup failed]"* ]]; then
				build_failed+=("$this_pkg") # 该包只产出 <build-failed>，不再产出其它记录
				pending=()
				pkg=""
				continue
			fi
			if [ "${#pending[@]}" -gt 0 ]; then
				local n
				for n in "${pending[@]}"; do printf '%s::%s\n' "${pkg:-$this_pkg}" "$n"; done
				pending=()
			fi
			pkg=""
			continue
		fi
	done
	local b
	for b in "${build_failed[@]}"; do printf '%s::<build-failed>\n' "$b"; done
}

parse_frontend_fails() { # stdin=vitest 输出；stdout=test_id（一行一条，未去重）
	local line t cur_file=""
	while IFS= read -r line || [ -n "$line" ]; do
		line="$(strip_ansi "$line")"
		line="${line%$'\r'}"
		t="${line#"${line%%[![:space:]]*}"}"
		[ -n "$t" ] || continue
		case "$t" in
		'❯ '*) # 文件上下文行：❯ app/x.test.ts (17 tests | 17 failed) 297ms
			cur_file="${t#❯ }"
			cur_file="${cur_file%% (*}"
			# 堆栈行（❯ path:105:58）不是文件上下文，不当路径用
			case "$cur_file" in
			*.test.ts | *.test.tsx | *.test.js | *.test.jsx) ;;
			*) cur_file="" ;;
			esac
			;;
		'×'*) front_emit "${t#×}" "$cur_file" ;;
		FAIL*) front_emit "${t#FAIL}" "$cur_file" ;;
		esac
	done
}

front_strip_duration() { # 去掉 vitest 明细行尾部的耗时（28ms / 1.5s / 1m 2s / 297ms）
	local s
	s="$(printf '%s' "$1" | sed -E 's/([[:space:]]+[0-9]+(\.[0-9]+)?(ms|s|m|h))+[[:space:]]*$//')"
	[ -n "$s" ] && printf '%s' "$s" || printf '%s' "$1"
}

front_emit() { # $1=标记后的内容 $2=❯ 文件上下文
	local rest="$1" ctx="$2"
	rest="${rest#"${rest%%[![:space:]]*}"}"
	rest="${rest%"${rest##*[![:space:]]}"}"
	# vitest 明细行（× / ✓）尾部带耗时（ 28ms），剥掉才能与尾部 FAIL 段同 id 去重
	rest="$(front_strip_duration "$rest")"
	[ -n "$rest" ] || return 0
	# 整文件级（collect 失败）：path [ path ]
	if [[ "$rest" =~ ^(.*\.test\.[tj]sx?)[[:space:]]+\[.*\]$ ]]; then
		printf 'front/%s::<file-level>\n' "${BASH_REMATCH[1]}"
		return 0
	fi
	# 用例级：path > suite > case（` > ` 链逐字保留）
	if [[ "$rest" == *" > "* ]] && [[ "${rest%% > *}" == *".test."* ]]; then
		printf 'front/%s::%s\n' "${rest%% > *}" "${rest#* > }"
		return 0
	fi
	# 仅 case 链：靠 ❯ 文件上下文补路径
	if [ -n "$ctx" ] && [ "$rest" != "$ctx" ]; then
		printf 'front/%s::%s\n' "$ctx" "$rest"
		return 0
	fi
	# 兜底：整行就是测试文件路径
	if [[ "$rest" =~ ^(.*\.test\.[tj]sx?)$ ]]; then
		printf 'front/%s::<file-level>\n' "${BASH_REMATCH[1]}"
		return 0
	fi
	return 0
}

dedupe() { awk '!seen[$0]++'; }

# ---------------------------------------------------------------------------
# 资源预检（提醒不阻断；预检本身零写入，且在分片执行之前）
# ---------------------------------------------------------------------------
precheck_load() {
	local file="${TEST_PATROL_LOADAVG_FILE:-/proc/loadavg}" raw load1
	if ! raw="$(head -n1 "$file" 2>/dev/null)" || [ -z "$raw" ]; then
		printf 'ℹ 无法读取负载信息（%s），跳过负载预检\n' "$file"
		return 0
	fi
	load1="${raw%% *}"
	[ -n "$load1" ] || load1="$raw"
	if awk -v v="$load1" -v t="$LOAD_THRESHOLD" 'BEGIN{exit !((v+0) > (t+0))}'; then
		printf '⚠ 负载偏高：load average(1min)=%s > %s，建议延后或错峰巡检（本次照常执行）\n' "$load1" "$LOAD_THRESHOLD"
	fi
}

precheck_processes() {
	local list="" line has_build=0 has_browser=0 sample=""
	if [ -n "${TEST_PATROL_PROCESS_LIST:-}" ]; then
		list="$(cat "$TEST_PATROL_PROCESS_LIST" 2>/dev/null || true)"
	else
		list="$(ps -eo args= 2>/dev/null || true)"
	fi
	while IFS= read -r line; do
		[ -n "$line" ] || continue
		case "$line" in
		*'go build'* | *'go test'* | *'golangci-lint'* | *'pnpm build'* | *'nuxi build'* | *'nuxi typecheck'* | *'vite build'* | *'esbuild'*)
			has_build=1
			[ -n "$sample" ] || sample="$line"
			;;
		esac
		case "$line" in
		*agent-browser* | *chromium* | *playwright* | *puppeteer* | *'chrome'*)
			has_browser=1
			[ -n "$sample" ] || sample="$line"
			;;
		esac
	done <<<"$list"
	local cats=""
	[ "$has_build" = 1 ] && cats="构建"
	[ "$has_browser" = 1 ] && cats="${cats:+$cats/}浏览器自动化"
	if [ -n "$cats" ]; then
		printf '⚠ 检出并发负载进程（类别：%s）：%s\n' "$cats" "${sample:0:120}"
		printf '  建议延后或错峰巡检（本次照常执行）\n'
	fi
}

precheck() {
	precheck_load
	precheck_processes
}

# ---------------------------------------------------------------------------
# 分片执行
# ---------------------------------------------------------------------------
RUN_OUTPUT=""
RUN_RC=0
RUN_MS=0

execute_shard() {
	ensure_tmp
	RUN_OUTPUT="$TMP_WORK/runner.out"
	local t0 t1
	t0="$(now_ms)"
	if [ -n "${TEST_PATROL_FAKE_OUTPUT:-}" ]; then
		if [ ! -f "$TEST_PATROL_FAKE_OUTPUT" ]; then
			printf '✗ 无法读取 TEST_PATROL_FAKE_OUTPUT：%s\n' "$TEST_PATROL_FAKE_OUTPUT" >&2
			return 2
		fi
		[ -n "${TEST_PATROL_FAKE_SLEEP:-}" ] && sleep "$TEST_PATROL_FAKE_SLEEP"
		cat "$TEST_PATROL_FAKE_OUTPUT" >"$RUN_OUTPUT"
		RUN_RC="${TEST_PATROL_FAKE_RC:-0}"
	else
		(cd "$SHARD_CWD" && "${SHARD_ARGS[@]}") >"$RUN_OUTPUT" 2>&1
		RUN_RC=$?
	fi
	t1="$(now_ms)"
	RUN_MS=$((t1 - t0))
	[ "$RUN_MS" -lt 0 ] && RUN_MS=0
	return 0
}

dry_run_shard() {
	printf '[DRY-RUN] cwd=%s\n' "$SHARD_CWD"
	local a
	for a in "${SHARD_ARGS[@]}"; do printf '[DRY-RUN] arg=%s\n' "$a"; done
}

# ---------------------------------------------------------------------------
# 记账（写入失败一律 fail-open：告警可见、退出码不变）
# ---------------------------------------------------------------------------
record_shard() { # $1=shard $2=ok(0/1) $3=ms $4=fails 文件（一行一条）
	local shard="$1" ok="$2" ms="$3" fails_file="$4"
	local n_fails=0 fails_json="" id now sql err
	[ -s "$fails_file" ] && n_fails="$(wc -l <"$fails_file" | tr -d ' ')"
	if [ "$n_fails" -gt 0 ]; then
		while IFS= read -r id; do
			[ -n "$id" ] || continue
			fails_json="${fails_json:+$fails_json,}\"$(json_escape "$id")\""
		done <"$fails_file"
	fi
	local payload="{\"shard\":\"$(json_escape "$shard")\",\"ok\":$([ "$ok" = 1 ] && printf 'true' || printf 'false'),\"ms\":$ms,\"fails\":[$fails_json]}"
	now="$(now_iso)"
	ensure_tmp
	sql="$TMP_WORK/record.sql"
	err="$TMP_WORK/record.err"
	{
		printf 'PRAGMA busy_timeout=5000;\nBEGIN IMMEDIATE;\n'
		printf 'INSERT INTO events (ts, session_id, kind, change, payload) VALUES (%s, %s, %s, NULL, %s);\n' \
			"$(sqlq "$now")" "$(sqlq "$SESSION_ID")" "$(sqlq "patrol.check")" "$(sqlq "$payload")"
		if [ "$ok" = 1 ]; then
			# 转绿闭环：该片 open 记录迁 fixed（fixed_by=patrol）；行不删除、first_seen 保留
			printf 'UPDATE test_debt SET status = %s, fixed_by = %s WHERE status = %s AND domain = %s;\n' \
				"$(sqlq fixed)" "$(sqlq patrol)" "$(sqlq open)" "$(sqlq "$shard")"
		elif [ "$n_fails" -gt 0 ]; then
			while IFS= read -r id; do
				[ -n "$id" ] || continue
				printf "INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status) VALUES (%s, %s, %s, %s, %s, 'open')\n" \
					"$(sqlq "$id")" "$(sqlq "$shard")" "$(sqlq "$now")" "$(sqlq "$now")" "$(sqlq "$shard")"
				printf "ON CONFLICT(test_id) DO UPDATE SET last_seen = excluded.last_seen, status = 'open',\n"
				printf "  note = CASE WHEN test_debt.status IN ('fixed','waived')\n"
				printf "              THEN COALESCE(test_debt.note || char(10), '') || 'reopened from ' || test_debt.status || '@' || substr(excluded.last_seen, 1, 10)\n"
				printf "              ELSE test_debt.note END;\n"
			done <"$fails_file"
		fi
		printf 'INSERT INTO patrol_shard (shard, last_run, last_ok, last_ms, last_fails, runs, total_fails) VALUES (%s, %s, %s, %s, %s, 1, %s)\n' \
			"$(sqlq "$shard")" "$(sqlq "$now")" "$ok" "$ms" "$n_fails" "$n_fails"
		printf 'ON CONFLICT(shard) DO UPDATE SET last_run = excluded.last_run, last_ok = excluded.last_ok,\n'
		printf '  last_ms = excluded.last_ms, last_fails = excluded.last_fails,\n'
		printf '  runs = patrol_shard.runs + 1, total_fails = patrol_shard.total_fails + excluded.total_fails;\n'
		printf 'COMMIT;\n'
	} >"$sql"
	if ! sqlite3 -batch "$DB_PATH" <"$sql" >/dev/null 2>"$err"; then
		DB_REASON="$(tr '\n' ' ' <"$err" | sed 's/  */ /g')"
		warn_db_fail_open
	fi
}

# ---------------------------------------------------------------------------
# 报告（只读；有欠账也 exit 0——报告不是门禁）
# ---------------------------------------------------------------------------
table_exists() {
	local n
	n="$(sqlite3 -readonly -batch "$DB_PATH" "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=$(sqlq "$1");" 2>/dev/null)"
	[ "$n" = "1" ]
}

ro() { sqlite3 -readonly -batch "$DB_PATH" "$1"; }                          # 只读单值
ro_rows() { sqlite3 -readonly -batch -separator '  ' "$DB_PATH" "$1"; }      # 只读多行

cmd_report() {
	local cutoff rows
	cutoff="$(stale_cutoff)"
	printf '== 测试欠账台账（test_debt）==\n'
	if [ ! -f "$DB_PATH" ] || ! table_exists test_debt; then
		printf '状态计数：open 0 / fixed 0 / waived 0（台账未建，先跑 --init 或巡检一片）\n'
		printf 'domain 聚合：（空）\nopen 清单：（空）\n'
		printf 'stale（open 且 last_seen 超 %s 天，截止 %s）：（无）\n' "$STALE_DAYS" "$cutoff"
		printf '== 巡检进度（patrol_shard）==\n（暂无巡检记录）\n'
		return 0
	fi
	local c_open c_fixed c_waived
	c_open="$(ro "SELECT COUNT(*) FROM test_debt WHERE status='open';" 2>/dev/null)"
	c_fixed="$(ro "SELECT COUNT(*) FROM test_debt WHERE status='fixed';" 2>/dev/null)"
	c_waived="$(ro "SELECT COUNT(*) FROM test_debt WHERE status='waived';" 2>/dev/null)"
	printf '状态计数：open %s / fixed %s / waived %s\n' "${c_open:-0}" "${c_fixed:-0}" "${c_waived:-0}"

	printf '\ndomain 聚合（按记录数降序，计数相同按 domain 字母序）：\n'
	rows="$(ro_rows "SELECT COALESCE(domain,'(空)'), COUNT(*) FROM test_debt GROUP BY domain ORDER BY COUNT(*) DESC, domain ASC;" 2>/dev/null)"
	if [ -z "$rows" ]; then printf '  （空）\n'; else printf '  %s\n' "$rows"; fi

	printf '\nopen 清单（first_seen 升序，最老欠账在前）：\n'
	rows="$(ro_rows "SELECT first_seen, '[' || COALESCE(domain,'?') || ']', test_id, 'last_seen=' || last_seen, 'context=' || COALESCE(context,'-') FROM test_debt WHERE status='open' ORDER BY first_seen ASC, test_id ASC;" 2>/dev/null)"
	if [ -z "$rows" ]; then printf '  （空）\n'; else printf '  %s\n' "$rows"; fi

	printf '\nstale（open 且 last_seen 超 %s 天，严格大于；截止 %s）：\n' "$STALE_DAYS" "$cutoff"
	rows="$(ro_rows "SELECT first_seen, test_id, 'last_seen=' || last_seen FROM test_debt WHERE status='open' AND last_seen < $(sqlq "$cutoff") ORDER BY first_seen ASC, test_id ASC;" 2>/dev/null)"
	if [ -z "$rows" ]; then printf '  （无）\n'; else printf '  %s\n' "$rows"; fi

	printf '\n== 巡检进度（patrol_shard）==\n'
	printf 'shard / last_run / last_ok / last_ms / last_fails / runs / total_fails\n'
	rows="$(ro_rows "SELECT shard, 'last_run=' || COALESCE(last_run,'-'), 'last_ok=' || COALESCE(last_ok,'-'), 'last_ms=' || COALESCE(last_ms,'-'), 'last_fails=' || COALESCE(last_fails,'-'), 'runs=' || COALESCE(runs,0), 'total_fails=' || COALESCE(total_fails,0) FROM patrol_shard ORDER BY shard;" 2>/dev/null)"
	if [ -z "$rows" ]; then printf '（暂无巡检记录）\n'; else printf '  %s\n' "$rows"; fi
	return 0
}

# ---------------------------------------------------------------------------
# 管理子命令
# ---------------------------------------------------------------------------
cmd_init() {
	if ! db_open_write; then
		printf '✗ --init 失败：%s\n' "$DB_REASON" >&2
		exit 2
	fi
	printf '✓ [init] 表已就绪：test_debt / patrol_shard（库 %s）\n' "$DB_PATH"
	return 0
}

validate_status() { # A-4 双侧之一：脚本层先校验三态（DDL CHECK 兜底）
	case "$1" in
	open | fixed | waived) return 0 ;;
	*)
		printf '✗ 非法 status：%s（仅 open/fixed/waived）\n' "$1" >&2
		return 1
		;;
	esac
}

cmd_register() {
	if ! db_open_write; then
		warn_db_fail_open
		exit 2
	fi
	validate_status open || exit 2
	local now note_sql sql err
	now="$(now_iso)"
	if [ -n "$REG_NOTE" ]; then note_sql="$(sqlq "$REG_NOTE")"; else note_sql="NULL"; fi
	ensure_tmp
	sql="$TMP_WORK/register.sql"
	err="$TMP_WORK/register.err"
	{
		printf 'PRAGMA busy_timeout=5000;\nBEGIN IMMEDIATE;\n'
		printf 'INSERT INTO test_debt (test_id, domain, first_seen, last_seen, context, status, note) VALUES (%s, %s, %s, %s, %s, %s, %s)\n' \
			"$(sqlq "$REG_ID")" "$(sqlq "$REG_DOMAIN")" "$(sqlq "$now")" "$(sqlq "$now")" "$(sqlq "$REG_CTX")" "$(sqlq open)" "$note_sql"
		printf "ON CONFLICT(test_id) DO UPDATE SET last_seen = excluded.last_seen, status = 'open',\n"
		printf "  note = CASE WHEN test_debt.status IN ('fixed','waived')\n"
		printf "              THEN COALESCE(test_debt.note || char(10), '') || 'reopened from ' || test_debt.status || '@' || substr(excluded.last_seen, 1, 10)\n"
		printf "                        || CASE WHEN excluded.note IS NOT NULL THEN char(10) || excluded.note ELSE '' END\n"
		printf "              WHEN excluded.note IS NOT NULL THEN COALESCE(test_debt.note || char(10), '') || excluded.note\n"
		printf '              ELSE test_debt.note END;\n'
		printf 'COMMIT;\n'
	} >"$sql"
	if ! sqlite3 -batch "$DB_PATH" <"$sql" >/dev/null 2>"$err"; then
		printf '✗ 登记失败：%s\n' "$(tr '\n' ' ' <"$err")" >&2
		exit 2
	fi
	printf '✓ [register] %s（context=%s，domain=%s，status=open）\n' "$REG_ID" "$REG_CTX" "$REG_DOMAIN"
	return 0
}

cmd_resolve() {
	if [ ! -f "$DB_PATH" ]; then
		printf '✗ 台账不存在：%s（未知 id：%s）\n  若这是新发现的失败测试，请用 --register 登记\n' "$DB_PATH" "$RES_ID" >&2
		exit 2
	fi
	if ! db_open_write; then
		warn_db_fail_open
		exit 2
	fi
	local st by
	st="$(sqlite3 -batch "$DB_PATH" "SELECT status FROM test_debt WHERE test_id = $(sqlq "$RES_ID");" 2>/dev/null)"
	if [ -z "$st" ]; then
		printf '✗ 台账无此 id：%s\n  若这是新发现的失败测试，请用 --register 登记（--register <test_id> --context <change名>）\n' "$RES_ID" >&2
		exit 2
	fi
	if [ "$st" != "open" ]; then
		printf '↷ [resolve] %s 已是终态 %s（幂等 no-op，不覆盖终态理由）\n' "$RES_ID" "$st"
		return 0
	fi
	by="${RES_BY:-manual}"
	validate_status fixed || exit 2
	if ! sqlite3 -batch "$DB_PATH" "PRAGMA busy_timeout=5000; UPDATE test_debt SET status = 'fixed', fixed_by = $(sqlq "$by") WHERE test_id = $(sqlq "$RES_ID");" >/dev/null 2>&1; then
		printf '✗ --resolve 写入失败：%s\n' "$RES_ID" >&2
		exit 2
	fi
	printf '✓ [resolve] %s → fixed（fixed_by=%s；first_seen/last_seen 保留）\n' "$RES_ID" "$by"
	return 0
}

cmd_waive() {
	if [ ! -f "$DB_PATH" ]; then
		printf '✗ 台账不存在：%s（未知 id：%s）\n' "$DB_PATH" "$WAIVE_ID" >&2
		exit 2
	fi
	if ! db_open_write; then
		warn_db_fail_open
		exit 2
	fi
	local st now sql err
	st="$(sqlite3 -batch "$DB_PATH" "SELECT status FROM test_debt WHERE test_id = $(sqlq "$WAIVE_ID");" 2>/dev/null)"
	if [ -z "$st" ]; then
		printf '✗ 台账无此 id：%s（未知 id，先 --register 登记）\n' "$WAIVE_ID" >&2
		exit 2
	fi
	if [ "$st" != "open" ]; then
		printf '↷ [waive] %s 已是终态 %s（幂等 no-op，不覆盖终态理由）\n' "$WAIVE_ID" "$st"
		return 0
	fi
	validate_status waived || exit 2
	now="$(now_iso)"
	ensure_tmp
	sql="$TMP_WORK/waive.sql"
	err="$TMP_WORK/waive.err"
	{
		printf 'PRAGMA busy_timeout=5000;\nBEGIN IMMEDIATE;\n'
		printf "UPDATE test_debt SET status = 'waived', waived_reason = %s, waived_at = %s WHERE test_id = %s;\n" \
			"$(sqlq "$WAIVE_REASON")" "$(sqlq "$now")" "$(sqlq "$WAIVE_ID")"
		printf 'COMMIT;\n'
	} >"$sql"
	if ! sqlite3 -batch "$DB_PATH" <"$sql" >/dev/null 2>"$err"; then
		printf '✗ --waive 写入失败：%s\n' "$(tr '\n' ' ' <"$err")" >&2
		exit 2
	fi
	printf '✓ [waive] %s → waived（waived_at=%s；fixed_by 保持 NULL）\n' "$WAIVE_ID" "$now"
	return 0
}

# ---------------------------------------------------------------------------
# 巡检主流程
# ---------------------------------------------------------------------------
declare -A LAST_RUN=()
INVOKE_NOW=""

load_progress() { # 进度表 → LAST_RUN[]
	local out s lr
	LAST_RUN=()
	if [ -f "$DB_PATH" ]; then
		out="$(sqlite3 -readonly -batch -separator '|' "$DB_PATH" "SELECT shard, COALESCE(last_run,'') FROM patrol_shard;" 2>/dev/null || true)"
		while IFS='|' read -r s lr; do
			[ -n "$s" ] || continue
			LAST_RUN["$s"]="$lr"
		done <<<"$out"
	fi
}

pick_shard() { # 最久未巡优先：NULL 最优先 → 最早 last_run → 平局按静态表顺序
	local s best="" best_lr="" lr
	for s in "${SHARD_ORDER[@]}"; do
		if [ -z "${LAST_RUN[$s]:-}" ]; then
			printf '%s' "$s"
			return 0
		fi
	done
	for s in "${SHARD_ORDER[@]}"; do
		lr="${LAST_RUN[$s]:-}"
		if [ -z "$best" ] || [ "$lr" \< "$best_lr" ]; then
			best="$s"
			best_lr="$lr"
		fi
	done
	printf '%s' "$best"
}

run_shard_once() { # $1=shard；返回 0 绿 / 1 红 / 2 环境错
	local shard="$1"
	shard_setup "$shard" || return 2
	if [ "${TEST_PATROL_DRY_RUN:-0}" = "1" ]; then
		dry_run_shard
		printf '↷ [%s] DRY-RUN（未执行、未写库）\n' "$shard"
		LAST_RUN["$shard"]="$INVOKE_NOW"
		return 0
	fi

	execute_shard || return 2
	local fails_file="$TMP_WORK/fails.txt" n_fails rc_out=0
	: >"$fails_file"
	if [ "$SHARD_KIND" = backend ]; then
		parse_backend_fails <"$RUN_OUTPUT" | dedupe >"$fails_file"
	else
		parse_frontend_fails <"$RUN_OUTPUT" | dedupe >"$fails_file"
	fi
	n_fails="$(wc -l <"$fails_file" | tr -d ' ')"

	if [ "$n_fails" -gt 0 ]; then
		rc_out=1
	elif [ "$RUN_RC" -ne 0 ]; then
		# A-5①：runner 非 0 但零 FAIL 明细 = 跑不起来，不当测试红记账（exit 2）
		printf '✗ [%s] runner 退出码 %s 但无法解析失败明细（%sms）——按环境错误处理\n' "$shard" "$RUN_RC" "$RUN_MS"
		printf '  （不新增台账行；patrol.check 仍记 ok=false / fails=[]）\n'
		if db_open_write; then record_shard "$shard" 0 "$RUN_MS" "$fails_file"; else warn_db_fail_open; fi
		LAST_RUN["$shard"]="$INVOKE_NOW"
		return 2
	fi

	if [ "$rc_out" = 0 ]; then
		printf '✓ [%s] 全绿（%sms）\n' "$shard" "$RUN_MS"
	else
		printf '✗ [%s] 检出 %s 处失败（%sms，runner exit %s）：\n' "$shard" "$n_fails" "$RUN_MS" "$RUN_RC"
		sed 's/^/    - /' "$fails_file"
	fi
	# 失败片也完成全部记账（exit 1 不短路记账）
	if db_open_write; then
		record_shard "$shard" "$([ "$rc_out" = 0 ] && printf 1 || printf 0)" "$RUN_MS" "$fails_file"
	else
		warn_db_fail_open
	fi
	LAST_RUN["$shard"]="$INVOKE_NOW"
	return "$rc_out"
}

# ---------------------------------------------------------------------------
# 参数解析（严格；未知参数不静默忽略）
# ---------------------------------------------------------------------------
MODE=""
SHARD=""
SHARDS_N=""
REG_ID=""
REG_CTX=""
REG_DOMAIN="manual"
REG_NOTE=""
RES_ID=""
RES_BY=""
WAIVE_ID=""
WAIVE_REASON=""

need_value() {
	if [ $# -lt 2 ] || [ -z "${2:-}" ]; then
		die_usage "选项 $1 需要一个非空参数"
	fi
}

set_mode() {
	if [ -n "$MODE" ]; then
		die_usage "参数冲突：$1 与 $MODE 不能同时使用"
	fi
	MODE="$1"
}

parse_args() {
	if [ $# -eq 0 ]; then
		MODE=run
		return 0
	fi
	while [ $# -gt 0 ]; do
		case "$1" in
		--help) set_mode help; shift ;;
		--init) set_mode init; shift ;;
		--report) set_mode report; shift ;;
		--shard) need_value "$1" "${2:-}"; set_mode shard; SHARD="$2"; shift 2 ;;
		--shards) need_value "$1" "${2:-}"; set_mode shards; SHARDS_N="$2"; shift 2 ;;
		--register) need_value "$1" "${2:-}"; set_mode register; REG_ID="$2"; shift 2 ;;
		--resolve) need_value "$1" "${2:-}"; set_mode resolve; RES_ID="$2"; shift 2 ;;
		--waive) need_value "$1" "${2:-}"; set_mode waive; WAIVE_ID="$2"; shift 2 ;;
		--context) need_value "$1" "${2:-}"; REG_CTX="$2"; shift 2 ;;
		--domain) need_value "$1" "${2:-}"; REG_DOMAIN="$2"; shift 2 ;;
		--note) need_value "$1" "${2:-}"; REG_NOTE="$2"; shift 2 ;;
		--by) need_value "$1" "${2:-}"; RES_BY="$2"; shift 2 ;;
		--reason) need_value "$1" "${2:-}"; WAIVE_REASON="$2"; shift 2 ;;
		--*) die_usage "未知参数：$1（严格解析，不静默忽略）" ;;
		*) die_usage "意外的位置参数：$1" ;;
		esac
	done
	return 0
}

print_help() {
	cat <<'HELP'
test-patrol.sh — 测试欠账滚动巡检（change: test-debt-patrol）

用法：
  bash scripts/harness/test-patrol.sh                  默认：跑最久未巡的一片（12 片轮转，NULL 最优先）
  bash scripts/harness/test-patrol.sh --shard <name>   指定分片（12 轮转片 + be-all）
  bash scripts/harness/test-patrol.sh --shards <n>     连续跑 n 片（最久未巡优先；n 超 12 时跑满一轮）
  bash scripts/harness/test-patrol.sh --init           建表（test_debt / patrol_shard，幂等）
  bash scripts/harness/test-patrol.sh --report         台账汇总（三态计数 / domain / open 清单 / stale / 进度）
  bash scripts/harness/test-patrol.sh --register <test_id> --context <ctx> [--domain <d>] [--note <text>]
                                               手工登记欠账（归档撞见域外红用；domain 默认 manual）
  bash scripts/harness/test-patrol.sh --resolve <test_id> [--by <change>]
                                               迁 fixed（终态再调为幂等 no-op + exit 0）
  bash scripts/harness/test-patrol.sh --waive <test_id> --reason <text>
                                               迁 waived（终态同 --resolve 语义）
  bash scripts/harness/test-patrol.sh --help           本帮助

退出码：0 全绿或管理子命令成功；1 跑完检出红；2 用法/环境错（含 runner 非 0 但零 FAIL 解析）

台账/事件库：默认 .pi/harness/events.db（可用 TEST_PATROL_DB 覆盖）
测试缝：TEST_PATROL_DB / TEST_PATROL_FAKE_OUTPUT / TEST_PATROL_FAKE_RC / TEST_PATROL_FAKE_SLEEP /
        TEST_PATROL_DRY_RUN=1 / TEST_PATROL_LOADAVG_FILE / TEST_PATROL_PROCESS_LIST / TEST_PATROL_NOW

分片静态枚举（12 片轮转 + be-all；前端恒 --maxWorkers=2，不带 --——`pnpm test:unit -- <filter>` 会吞 filter 跑全量）：
HELP
	local s
	for s in "${SHARD_ORDER[@]}" "$SHARD_ALL"; do
		shard_setup "$s"
		printf '  %-18s cd %-11s %s\n' "$s" "$SHARD_CWD" "${SHARD_ARGS[*]}"
	done
	printf '说明：be-all 不入轮转，仅 `--shard be-all` 显式指定（跑整后端）\n'
}

case_mode() {
	case "$MODE" in
	help)
		print_help
		exit 0
		;;
	init)
		cmd_init
		exit 0
		;;
	report)
		cmd_report
		exit 0
		;;
	register)
		if [ -z "$REG_CTX" ]; then
			die_usage "--register 需要 --context <语境>（change 名 / 存量摸底 / manual）"
		fi
		cmd_register
		exit 0
		;;
	resolve)
		cmd_resolve
		exit 0
		;;
	waive)
		if [ -z "$WAIVE_REASON" ]; then
			die_usage "--waive 需要 --reason <理由>（终态理由必须留痕）"
		fi
		cmd_waive
		exit 0
		;;
	shard)
		if ! shard_known "$SHARD"; then
			die_usage "未知分片：${SHARD:-（空）}"
		fi
		;;
	shards)
		if ! [[ "$SHARDS_N" =~ ^[0-9]+$ ]] || [ "$SHARDS_N" -lt 1 ]; then
			die_usage "--shards 需要一个 ≥1 的整数（收到：${SHARDS_N:-（空）}）"
		fi
		;;
	run) ;;
	*) die_usage "内部错误：未知模式 ${MODE:-（空）}" ;;
	esac
}

# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------
parse_args "$@"
case_mode

INVOKE_NOW="$(now_iso)"

if [ "${TEST_PATROL_DRY_RUN:-0}" != "1" ]; then
	precheck
fi

PICKED=()
OVERALL=0
if [ "$MODE" = "shard" ]; then
	PICKED=("$SHARD")
	for s in "${PICKED[@]}"; do
		run_shard_once "$s"
		rc=$?
		[ "$rc" -gt "$OVERALL" ] && OVERALL="$rc"
	done
else
	# 默认一片 / --shards n：逐片取「最久未巡」，选中即在本进程内虚拟推进 last_run
	# （DRY_RUN 不写库，同样靠内存推进；本轮选中集合因此不会重复）
	load_progress
	if [ "$MODE" = "shards" ]; then n_wanted="$SHARDS_N"; else n_wanted=1; fi
	[ "$n_wanted" -gt "${#SHARD_ORDER[@]}" ] && n_wanted="${#SHARD_ORDER[@]}"
	i=0
	while [ "$i" -lt "$n_wanted" ]; do
		s="$(pick_shard)"
		PICKED+=("$s")
		run_shard_once "$s"
		rc=$?
		[ "$rc" -gt "$OVERALL" ] && OVERALL="$rc"
		i=$((i + 1))
	done
fi

if [ "${TEST_PATROL_DRY_RUN:-0}" = "1" ]; then
	printf '（DRY-RUN：未执行、未写库；共 %d 片）\n' "${#PICKED[@]}"
	exit 0
fi

exit "$OVERALL"
