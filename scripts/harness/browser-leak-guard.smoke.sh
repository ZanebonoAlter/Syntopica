#!/usr/bin/env bash
# browser-leak-guard.smoke.sh — browser-leak-guard.sh 的假进程冒烟（add-browser-leak-guard, design D6）
#
# 红线（2026-11-13 现场就有一套在用的 cityenv-* 栈，测试若碰它会直接毁掉别人验收）：
#   - 绝不启动真浏览器、绝不给仓库外进程发信号；
#   - fixture 全部落在 mktemp -d 下：复制 bash 造「同名假二进制」（agent-browser-linux-arm64 /
#     fake-chromium / agent-browser CLI / chrome_crashpad_handler），用 argv 注入
#     --user-data-dir=<临时前缀>、用 env 注入 AGENT_BROWSER_DAEMON=1 / AGENT_BROWSER_SESSION=smk<pid>-<n>；
#   - 每个用例把 BLG_DAEMON_PATTERN / BLG_PROFILE_PREFIX 指到本用例的临时目录，guard 的全量 /proc
#     扫描天然看不见真实栈（连报告都不会出现），无需额外过滤；
#   - 每个用例独立 XDG_RUNTIME_DIR（跨轮次状态文件 + flock 锁都在里面），绝不污染用户真实状态。
#
# 用例矩阵（tasks.md 3.2 十四组，详见尾部 summary）：
#   ① 陈旧冻结栈 → --apply 回收（首轮 first-seen 不动手，次轮 close + 进程消失 + profile 删除；
#      继承 env 的 daemon 形态与 chrome_crashpad_handler 形态都不能挡住回收、也不能被误杀）
#   ② 有活 CLI 客户端（environ 形式 + argv `--session` 形式各一）→ client-attached
#   ③ CPU 在涨 → busy（JSON cpu_ticks 为 null）
#   ④ 太年轻 → young（JSON cpu_ticks 为 null）
#   ⑤ 首轮 first-seen 只取证、次轮才回收 + 状态文件行格式
#   ⑥ 报告模式 → would-reap 且不动手、退出码 1；--quiet 下 would-reap 与 --json 不被吞
#   ⑦ close 失败 → TERM/KILL 降级成功
#   ⑧ close 返回非 0 但栈已自退 → escalation=close-gone，不发信号
#   ⑨ 独立无关 chromium（父为 shell、独立进程组、无 profile 锚点）不命中、不被误杀
#   ⑩ 多目标 fail-open（一套 no-chromium + 一套正常 → 正常那套仍被回收；--quiet 下 no-chromium 出声）
#   ⑪ --json 账本：dry-run（would-reap）与 apply（reaped）各一 + runtime 残留清理
#      （--json 的 stdout 只留一行 JSON，人读行改走 stderr）
#   ⑫ 非法参数 / 非法注入口 → 退出码 2
#   ⑬ 无泄漏 → 零输出退出码 0
#   ⑭ unit 静态断言（ExecStart 含 --apply --quiet、周期 10 分钟、%h 路径、不写死 node 版本）
#      + 脚本静态断言（failed 走 say_always、MIN_AGE 默认 1800、flock -n）
#
# 用法：bash scripts/harness/browser-leak-guard.smoke.sh  （约 60s，全程不碰真实进程）
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/browser-leak-guard.sh"
UNIT_SERVICE_FILE="$SCRIPT_DIR/systemd/syntopica-browser-leak-guard.service"
UNIT_TIMER_FILE="$SCRIPT_DIR/systemd/syntopica-browser-leak-guard.timer"
BASH_BIN="$(command -v bash)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/blg-smoke.XXXXXX")" || {
	echo "mktemp 失败，无法建立 fixture 目录" >&2
	exit 2
}

PASS=0
FAIL=0
FAILED_CASES=()
FIXTURE_PIDS=()

CLOSE_LOG="$TMP/close.log"     # stub close 的调用记录（argv + 继承到的 session）
CLOSE_PGIDS="$TMP/close.pgids" # stub close 要 TERM 的 PGID（按 session 分文件，见 start_stack）
STUB_OK="$TMP/stub-close-ok.sh"
STUB_FAIL="$TMP/stub-close-fail.sh"
STUB_NOOP="$TMP/stub-close-noop.sh"
STUB_GONE="$TMP/stub-close-gone.sh"

export BLG_SMOKE_CLOSE_LOG="$CLOSE_LOG"
export BLG_SMOKE_CLOSE_PGIDS="$CLOSE_PGIDS"

# session 名带本次运行的 PID 后缀：万一上一轮脚本被强杀留下 fixture（chromium 会继承
# AGENT_BROWSER_SESSION），本轮也不会把它误认成「同 session 的活客户端」而整轮跳过。
RUN_TAG="$$"
S1="smk$RUN_TAG-1"
S2A="smk$RUN_TAG-2a"
S2B="smk$RUN_TAG-2b"
S3="smk$RUN_TAG-3"
S4="smk$RUN_TAG-4"
S5="smk$RUN_TAG-5"
S5B="smk$RUN_TAG-5b"
S6="smk$RUN_TAG-6"
S7="smk$RUN_TAG-7"
S8="smk$RUN_TAG-8"
S9="smk$RUN_TAG-9"
S10A="smk$RUN_TAG-10a"
S10B="smk$RUN_TAG-10b"
S11="smk$RUN_TAG-11"

# ---------------------------------------------------------------------------
# 断言与输出
# ---------------------------------------------------------------------------
ok() { PASS=$((PASS + 1)); printf '  ✅ %s\n' "$1"; }
bad() {
	FAIL=$((FAIL + 1))
	FAILED_CASES+=("$1")
	printf '  ❌ %s\n' "$1"
}
check() { # 名 期望 实际
	if [ "$2" = "$3" ]; then ok "$1"; else bad "$1（期望 [$2] 实际 [$3]）"; fi
}
check_contains() { # 名 haystack needle
	case "$2" in *"$3"*) ok "$1" ;; *) bad "$1（输出里没有 [$3]）" ;; esac
}
check_absent() { # 名 haystack needle（不该出现）
	case "$2" in *"$3"*) bad "$1（不该出现 [$3]）" ;; *) ok "$1" ;; esac
}
alive_str() { kill -0 "$1" 2>/dev/null && printf alive || printf dead; }
present_str() { [ -e "$1" ] && printf here || printf gone; }

# 数某个标记在日志里出现几次（grep -c 命中 0 条时也 exit 1，不能直接 `|| printf 0`）
count_in() { # $1=文件 $2=标记
	local n
	n="$(grep -c -- "$2" "$1" 2>/dev/null)" || n=0
	case "$n" in '' | *[!0-9]*) n=0 ;; esac
	printf '%s' "$n"
}

# ---------------------------------------------------------------------------
# 清理（trap）：先 KILL 记录的 PID，再 KILL 各自进程组，最后删临时目录
# ---------------------------------------------------------------------------
cleanup() {
	local pid
	for pid in "${FIXTURE_PIDS[@]}"; do
		kill -KILL -- "-$pid" 2>/dev/null
		kill -KILL "$pid" 2>/dev/null
	done
	rm -rf "$TMP"
}
trap cleanup EXIT

# 一个用例结束就把自己的 fixture 杀干净（特别是 ③ 的忙等 fixture，否则会烧到脚本结束）
cleanup_case() {
	local pid
	for pid in "${FIXTURE_PIDS[@]}"; do
		kill -KILL -- "-$pid" 2>/dev/null
		kill -KILL "$pid" 2>/dev/null
	done
	FIXTURE_PIDS=()
}

# ---------------------------------------------------------------------------
# stub close：记录调用参数；ok 版按「本 session 的 pgid 清单」拆栈（模拟 agent-browser close
# 的真实效果）。按 session 分文件是必须的——多目标用例里 close 只该关自己那个 session。
# ---------------------------------------------------------------------------
make_stubs() {
	cat >"$STUB_OK" <<'EOS'
#!/usr/bin/env bash
printf 'argv=%s|env=%s\n' "$*" "${AGENT_BROWSER_SESSION:-}" >> "${BLG_SMOKE_CLOSE_LOG:?}"
f="${BLG_SMOKE_CLOSE_PGIDS:?}.${AGENT_BROWSER_SESSION:-}"
if [ -r "$f" ]; then
	while read -r pg; do
		[ -n "$pg" ] && kill -TERM -- "-$pg" 2>/dev/null
	done <"$f"
fi
exit 0
EOS
	cat >"$STUB_FAIL" <<'EOS'
#!/usr/bin/env bash
printf 'argv=%s|env=%s\n' "$*" "${AGENT_BROWSER_SESSION:-}" >> "${BLG_SMOKE_CLOSE_LOG:?}"
echo "stub: 模拟 close 失败" >&2
exit 1
EOS
	cat >"$STUB_NOOP" <<'EOS'
#!/usr/bin/env bash
printf 'argv=%s|env=%s\n' "$*" "${AGENT_BROWSER_SESSION:-}" >> "${BLG_SMOKE_CLOSE_LOG:?}"
exit 0
EOS
	# ⑧ 用：自己把栈拆干净（并等进程真的消失），然后**返回非 0** —— 模拟「close 报错但栈已自退」
	cat >"$STUB_GONE" <<'EOS'
#!/usr/bin/env bash
printf 'argv=%s|env=%s\n' "$*" "${AGENT_BROWSER_SESSION:-}" >> "${BLG_SMOKE_CLOSE_LOG:?}"
f="${BLG_SMOKE_CLOSE_PGIDS:?}.${AGENT_BROWSER_SESSION:-}"
pgs=""
if [ -r "$f" ]; then
	while read -r pg; do
		[ -n "$pg" ] || continue
		pgs="$pgs $pg"
		kill -TERM -- "-$pg" 2>/dev/null
	done <"$f"
fi
i=0
while [ "$i" -lt 40 ]; do
	any=0
	for pg in $pgs; do
		kill -0 -- "-$pg" 2>/dev/null && any=1
	done
	[ "$any" -eq 0 ] && break
	sleep 0.05
	i=$((i + 1))
done
echo "stub: 栈已自退但 close 仍报错" >&2
exit 1
EOS
	chmod +x "$STUB_OK" "$STUB_FAIL" "$STUB_NOOP" "$STUB_GONE"
	: >"$CLOSE_LOG"
}

# ---------------------------------------------------------------------------
# fixture 构造
# ---------------------------------------------------------------------------
# 每个用例独立目录 → 该用例的 BLG_DAEMON_PATTERN / BLG_PROFILE_PREFIX 只可能匹配本用例 fixture
new_case() { # $1 = 用例目录名
	CASE_DIR="$TMP/$1"
	CASE_RUNTIME="$TMP/$1-runtime"
	mkdir -p "$CASE_DIR/agent-browser/bin" "$CASE_RUNTIME"
	cp -f "$BASH_BIN" "$CASE_DIR/agent-browser/bin/agent-browser-linux-arm64"
	cp -f "$BASH_BIN" "$CASE_DIR/fake-chromium"
	chmod +x "$CASE_DIR/agent-browser/bin/agent-browser-linux-arm64" "$CASE_DIR/fake-chromium"
	CASE_DAEMON_PATTERN="$CASE_DIR/agent-browser/bin/agent-browser-"
	CASE_PROFILE_PREFIX="$CASE_DIR/agent-browser-chrome-"
	# 跨轮次状态文件 / flock 锁都落在本用例的运行时目录，绝不碰用户真实状态
	CASE_STATE="$CASE_RUNTIME/syntopica-browser-leak-guard/state"
	: >"$CLOSE_LOG"
	rm -f "$CLOSE_PGIDS".*
	GUARD_MIN_AGE=0
	GUARD_IDLE_SAMPLES=2
	GUARD_MIN_GAP=0 # 跨轮次用例故意连跑两轮，故把取证间隔归零（真实运行默认 300s）
	GUARD_CLOSE_CMD="$STUB_NOOP"
	GUARD_ENV_EXTRA=()
	PRIME_RC=0
	PRIME_OUT=""
}

cmdline_of() {
	local -a parts=()
	mapfile -d '' -t parts 2>/dev/null <"/proc/$1/cmdline" || return 1
	local IFS=' '
	printf '%s' "${parts[*]}"
}

pgid_of() { awk '{print $5}' "/proc/$1/stat" 2>/dev/null; }

# 按 cmdline 里的标记找 pid（fixture 的 pid 不从 $! 反推，避免 setsid fork 干扰）
pid_of() {
	local token="$1" d pid cl
	for d in /proc/[0-9]*; do
		pid="${d#/proc/}"
		[ "$pid" = "$$" ] && continue
		cl="$(cmdline_of "$pid")" || continue
		case "$cl" in *"$token"*) printf '%s' "$pid"; return 0 ;; esac
	done
	return 1
}

wait_for_pid() { # pid 出现且 cmdline 含 token，最多 2s
	local pid="$1" token="$2" i=0 cl
	while [ "$i" -lt 40 ]; do
		cl="$(cmdline_of "$pid")" && case "$cl" in *"$token"*) return 0 ;; esac
		sleep 0.05
		i=$((i + 1))
	done
	return 1
}

# 起一套伪造栈：daemon（bash 副本）+ 其后代 chromium（bash 副本，argv 带 --user-data-dir）
# $1=session 名  $2=anchor|no-anchor  $3=变体（空|busy|term-ign）
#
# 注意：profile 路径一律经环境变量 BLG_SMOKE_PROFILE 在 daemon 内部展开，**不能**作为 daemon 的
# 位置参数传进去——否则 daemon 自己的 cmdline 也会命中 --user-data-dir=<profile>，
# 让「按 profile 找 chromium」与「找 chromium 根」的判定失真。
start_stack() {
	local sess="$1" mode="$2" variant="${3:-}"
	local profile="$CASE_DIR/agent-browser-chrome-$sess"
	local chromium="$CASE_DIR/fake-chromium"
	local spawn body
	mkdir -p "$profile"
	: >"$profile/SingletonLock" # 占位文件：profile 被删才能验证 rm -rf 真的发生了
	if [ "$mode" = "no-anchor" ]; then
		# 仍然可搜（cmdline 带 profile 串）但不是 --user-data-dir= 锚点
		spawn='setsid "$1" -c "sleep 1000; :" fake-chromium "--no-anchor=$BLG_SMOKE_PROFILE" >/dev/null 2>&1 &'
	else
		spawn='setsid "$1" -c "sleep 1000; :" fake-chromium "--user-data-dir=$BLG_SMOKE_PROFILE" >/dev/null 2>&1 &'
	fi
	# 两条命令防止 bash 的 exec 优化（单命令 `-c 'sleep 60'` 会被 exec 掉、cmdline 变成 sleep）
	case "$variant" in
	busy) body="$spawn"$'\n''while :; do :; done' ;;
	term-ign)
		body='trap "" TERM'$'\n'"$spawn"$'\n''while :; do sleep 100; done'
		;;
	*) body="$spawn"$'\n''wait' ;;
	esac

	BLG_SMOKE_PROFILE="$profile" AGENT_BROWSER_DAEMON=1 AGENT_BROWSER_SESSION="$sess" \
		setsid "$CASE_DIR/agent-browser/bin/agent-browser-linux-arm64" -c "$body" _ \
		"$chromium" >/dev/null 2>&1 &
	STACK_DAEMON=$!
	disown 2>/dev/null || true # 不让 shell 在清理时打「Killed」噪音
	if ! wait_for_pid "$STACK_DAEMON" "agent-browser-linux-arm64"; then
		STACK_DAEMON="$(pid_of "$CASE_DAEMON_PATTERN")" || STACK_DAEMON=""
	fi
	[ -n "$STACK_DAEMON" ] && FIXTURE_PIDS+=("$STACK_DAEMON")

	STACK_PROFILE="$profile"
	STACK_CHROMIUM=""
	local i=0
	while [ "$i" -lt 40 ]; do
		STACK_CHROMIUM="$(pid_of "$profile")" && break
		sleep 0.05
		i=$((i + 1))
	done
	[ -n "$STACK_CHROMIUM" ] && FIXTURE_PIDS+=("$STACK_CHROMIUM")

	# 让 stub close 能整组拆栈（daemon 组 + chromium 根组）；按 session 分文件，
	# 多目标用例里 close 只关自己那个 session
	{
		pgid_of "$STACK_DAEMON"
		[ -n "$STACK_CHROMIUM" ] && pgid_of "$STACK_CHROMIUM"
	} >"$CLOSE_PGIDS.$sess"

	# 等 fixture 把「启动期 CPU 抖动」走完再交给 guard 采样：本用例故意用 --min-age 0 让栈
	# 立刻够老，若不 settle，刚 fork 出来的 chromium 的几十毫秒启动开销会落进 1s 采样窗口，
	# 被判成 busy（真实现场不会：新栈先被 young 拦下，根本走不到采样）。
	sleep 1.1
}

# 跑 guard：用例环境由 CASE_* / GUARD_* 变量决定，$@ 是 guard 命令行参数
GUARD_RC=0
GUARD_OUT=""
GUARD_ERR=""
PRIME_RC=0
PRIME_OUT=""
guard_run() {
	env BLG_DAEMON_PATTERN="$CASE_DAEMON_PATTERN" \
		BLG_PROFILE_PREFIX="$CASE_PROFILE_PREFIX" \
		BLG_MIN_AGE="$GUARD_MIN_AGE" \
		BLG_IDLE_SAMPLES="$GUARD_IDLE_SAMPLES" \
		BLG_MIN_GAP="$GUARD_MIN_GAP" \
		BLG_CLOSE_CMD="$GUARD_CLOSE_CMD" \
		XDG_RUNTIME_DIR="$CASE_RUNTIME" \
		"${GUARD_ENV_EXTRA[@]}" \
		"$BASH_BIN" "$GUARD" "$@" >"$TMP/guard.out" 2>"$TMP/guard.err"
	GUARD_RC=$?
	GUARD_OUT="$(cat "$TMP/guard.out")"
	GUARD_ERR="$(cat "$TMP/guard.err")"
	# 排查用：SMOKE_DEBUG=1 时把每次 guard 输出（含 stderr）打出来
	if [ -n "${SMOKE_DEBUG:-}" ]; then
		printf '    [debug] rc=%s out=%s err=%s\n' "$GUARD_RC" "$GUARD_OUT" "$GUARD_ERR"
	fi
}

# 跑两轮：第一轮只取证（跨轮次条件 → first-seen），第二轮才可能动手。
# 跨轮次用例一律用「真跑两轮」实现，绝不 sleep 十分钟。
guard_run_twice() {
	guard_run "$@"
	PRIME_RC=$GUARD_RC
	PRIME_OUT=$GUARD_OUT
	guard_run "$@"
}

# 仅限单栈用例：贪婪 sed 取第一个匹配字段，多栈输出会串味（多栈用例改用 check_contains）
json_field() { # $1 = 字段名 → stdout 值
	printf '%s' "$GUARD_OUT" | sed -n 's/.*"'"$1"'":\([^,}]*\).*/\1/p'
}

# ---------------------------------------------------------------------------
printf '== browser-leak-guard 冒烟（fixture 目录 %s）==\n' "$TMP"
make_stubs

# ---------------------------------------------------------------------------
# ① 陈旧冻结栈 → --apply 回收（首轮 first-seen 不动手，次轮 close + 进程消失 + profile 删除）
#    同场加两个「像客户端但其实不是」的干扰进程，都必须：不挡回收、不被误杀。
#      a) 继承 AGENT_BROWSER_DAEMON=1 且 cmdline 含 agent-browser 的守护进程形态
#      b) 只继承 AGENT_BROWSER_SESSION（无 DAEMON=1）、cmdline 含 agent-browser 的 crashpad helper
#         —— 这条是 B1 的尖点：不显式排除 chrome_crashpad_handler 就会被当成活客户端
# ---------------------------------------------------------------------------
printf '\n① 陈旧冻结栈 → 首轮 first-seen，次轮回收\n'
new_case case1
start_stack "$S1" anchor
cp -f "$BASH_BIN" "$CASE_DIR/agent-browser/bin/other-agent-browser-tool"
AGENT_BROWSER_DAEMON=1 AGENT_BROWSER_SESSION="$S1" \
	setsid "$CASE_DIR/agent-browser/bin/other-agent-browser-tool" -c 'sleep 1000; :' >/dev/null 2>&1 &
STRAY_DAEMON_LIKE=$!
FIXTURE_PIDS+=("$STRAY_DAEMON_LIKE")
disown 2>/dev/null || true
cp -f "$BASH_BIN" "$CASE_DIR/agent-browser/chrome_crashpad_handler"
AGENT_BROWSER_SESSION="$S1" \
	setsid "$CASE_DIR/agent-browser/chrome_crashpad_handler" -c 'sleep 1000; :' >/dev/null 2>&1 &
STRAY_CRASHPAD=$!
FIXTURE_PIDS+=("$STRAY_CRASHPAD")
disown 2>/dev/null || true
sleep 0.2
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
PRIME_RC=$GUARD_RC
PRIME_OUT=$GUARD_OUT
check "① 首轮退出码 0（只取证）" 0 "$PRIME_RC"
check_contains "① 首轮标 reason=first-seen" "$PRIME_OUT" "reason=first-seen"
check_absent "① 首轮不报 would-reap" "$PRIME_OUT" "would-reap"
check "① 首轮未调用 close" 0 "$(count_in "$CLOSE_LOG" "env=$S1")"
check "① 首轮 daemon 仍在" alive "$(alive_str "$STACK_DAEMON")"
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "① 次轮退出码 0" 0 "$GUARD_RC"
check_contains "① 次轮 reaped" "$GUARD_OUT" "reaped session=$S1"
check_contains "① 优雅 close 一次搞定（escalation=close）" "$GUARD_OUT" "escalation=close"
check_contains "① 报告带 procs/rss/profile 字段" "$GUARD_OUT" "procs="
check "① close 被调用一次" 1 "$(count_in "$CLOSE_LOG" "env=$S1")"
check_contains "① close 命令带 --session $S1" "$(cat "$CLOSE_LOG")" "--session $S1"
check "① daemon 已退出" dead "$(alive_str "$STACK_DAEMON")"
check "① chromium 已退出" dead "$(alive_str "$STACK_CHROMIUM")"
check "① profile 目录已删除" gone "$(present_str "$STACK_PROFILE")"
check "① 继承 DAEMON=1 的干扰进程未被误杀" alive "$(alive_str "$STRAY_DAEMON_LIKE")"
check "① 继承 session env 的 crashpad helper 未被误杀" alive "$(alive_str "$STRAY_CRASHPAD")"
cleanup_case

# ---------------------------------------------------------------------------
# ② 有活 CLI 客户端 → client-attached，不抢会话
#    A：environ 的 AGENT_BROWSER_SESSION 形式
#    B：argv 的 --session <名> 形式（environ 不带 session）—— B1 之前这里会漏判
# ---------------------------------------------------------------------------
printf '\n② 活 CLI 客户端（env 形式 + argv --session 形式）→ client-attached\n'
new_case case2
mkdir -p "$CASE_DIR/client"
cp -f "$BASH_BIN" "$CASE_DIR/client/agent-browser"
chmod +x "$CASE_DIR/client/agent-browser"
start_stack "$S2A" anchor
D2A=$STACK_DAEMON C2A=$STACK_CHROMIUM P2A=$STACK_PROFILE
start_stack "$S2B" anchor
D2B=$STACK_DAEMON C2B=$STACK_CHROMIUM P2B=$STACK_PROFILE
# A：env 形式（cmdline 里没有 --session）
AGENT_BROWSER_SESSION="$S2A" setsid "$CASE_DIR/client/agent-browser" -c 'sleep 1000; :' >/dev/null 2>&1 &
CLIENT_ENV=$!
FIXTURE_PIDS+=("$CLIENT_ENV")
disown 2>/dev/null || true
# B：argv 形式（environ 不带 AGENT_BROWSER_SESSION；--session 落在 cmdline 末尾）
setsid "$CASE_DIR/client/agent-browser" -c 'sleep 1000; :' agent-browser --session "$S2B" >/dev/null 2>&1 &
CLIENT_ARGV=$!
FIXTURE_PIDS+=("$CLIENT_ARGV")
disown 2>/dev/null || true
sleep 0.2
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "② 退出码 0（跳过不算泄漏）" 0 "$GUARD_RC"
check "② 两套栈都标 client-attached" 2 "$(count_in "$TMP/guard.out" "reason=client-attached")"
check "② 栈 A 未被 close" 0 "$(count_in "$CLOSE_LOG" "env=$S2A")"
check "② 栈 B 未被 close" 0 "$(count_in "$CLOSE_LOG" "env=$S2B")"
check "② daemon A 仍在" alive "$(alive_str "$D2A")"
check "② daemon B 仍在" alive "$(alive_str "$D2B")"
check "② chromium A 仍在" alive "$(alive_str "$C2A")"
check "② chromium B 仍在" alive "$(alive_str "$C2B")"
check "② 客户端 A 仍在" alive "$(alive_str "$CLIENT_ENV")"
check "② 客户端 B 仍在" alive "$(alive_str "$CLIENT_ARGV")"
# 定时器口径：有人在用浏览器时 --apply --quiet 必须零输出、零信号（否则 journal 每 10 分钟一条假故障）
: >"$CLOSE_LOG"
guard_run --apply --quiet
check "② --quiet 下 client-attached 零输出" "" "$GUARD_OUT"
check "② --quiet 下退出码 0" 0 "$GUARD_RC"
check "② --quiet 下仍未调用 close" 0 "$(count_in "$CLOSE_LOG" "env=$S2A")"
cleanup_case

# ---------------------------------------------------------------------------
# ③ CPU 在涨（正在干活）→ busy
# ---------------------------------------------------------------------------
printf '\n③ CPU 在涨 → busy\n'
new_case case3
start_stack "$S3" anchor busy
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply --json
check "③ 退出码 0" 0 "$GUARD_RC"
check_contains "③ 标 reason=busy" "$GUARD_ERR" "reason=busy"
check "③ daemon 仍在" alive "$(alive_str "$STACK_DAEMON")"
check "③ 未被 close" 0 "$(count_in "$CLOSE_LOG" "env=$S3")"
check "③ 未采样 → cpu_ticks_before 为 null（不是 0）" "null" "$(json_field cpu_ticks_before)"
check "③ 未采样 → cpu_ticks_after 为 null（不是 0）" "null" "$(json_field cpu_ticks_after)"
cleanup_case

# ---------------------------------------------------------------------------
# ④ 太年轻（--min-age 默认 1800s）→ young
# ---------------------------------------------------------------------------
printf '\n④ 太年轻 → young\n'
new_case case4
start_stack "$S4" anchor
GUARD_MIN_AGE=1800
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply --json
check "④ 退出码 0" 0 "$GUARD_RC"
check_contains "④ 标 reason=young" "$GUARD_ERR" "reason=young"
check "④ daemon 仍在" alive "$(alive_str "$STACK_DAEMON")"
check "④ 未采样 → cpu_ticks_before 为 null" "null" "$(json_field cpu_ticks_before)"
cleanup_case

# ---------------------------------------------------------------------------
# ⑤ 首轮只取证不动手、次轮才回收 + 状态文件行格式（B2 跨轮次冻结证据）
# ---------------------------------------------------------------------------
printf '\n⑤ first-seen：首轮不动手，次轮才回收\n'
new_case case5
start_stack "$S5" anchor
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply --json
check "⑤ 首轮退出码 0" 0 "$GUARD_RC"
check_contains "⑤ 首轮标 reason=first-seen" "$GUARD_ERR" "reason=first-seen"
check_contains "⑤ 首轮 JSON reason=first-seen" "$GUARD_OUT" '"reason":"first-seen"'
check_contains "⑤ 首轮未确认冻结→JSON ticks 为 null（S11）" "$GUARD_OUT" '"cpu_ticks_before":null'
check "⑤ 首轮未调用 close" 0 "$(count_in "$CLOSE_LOG" "env=$S5")"
check "⑤ 首轮 daemon 仍在" alive "$(alive_str "$STACK_DAEMON")"
check "⑤ 首轮 profile 仍在" here "$(present_str "$STACK_PROFILE")"
check "⑤ 状态文件已落盘该 session" 1 "$(count_in "$CASE_STATE" "$S5")"
check "⑤ 状态文件行格式 session/starttime/ticks/epoch（4 列全数字）" "$S5" \
	"$(awk -F'\t' -v s="$S5" '$1==s && NF==4 && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ && $4 ~ /^[0-9]+$/ {print $1}' "$CASE_STATE" 2>/dev/null)"
check "⑤ 状态文件权限之外无临时文件残留" "" "$(ls "$CASE_RUNTIME/syntopica-browser-leak-guard/" 2>/dev/null | grep -v '^state$' | grep -v '^lock$' | tr '\n' ' ')"
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "⑤ 次轮退出码 0" 0 "$GUARD_RC"
check_contains "⑤ 次轮 reaped" "$GUARD_OUT" "reaped session=$S5"
check "⑤ 次轮 close 被调用一次" 1 "$(count_in "$CLOSE_LOG" "env=$S5")"
check "⑤ 次轮 daemon 已退出" dead "$(alive_str "$STACK_DAEMON")"
check "⑤ 次轮 profile 已删除" gone "$(present_str "$STACK_PROFILE")"
check "⑤ 回收后 session 条目被清理" 0 "$(count_in "$CASE_STATE" "$S5")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑤b 取证最小间隔：连跑两轮但间隔不足 --min-gap → 仍不动手（审查遗留项防「秒级上膛」）
# ---------------------------------------------------------------------------
printf '\n⑤b min-gap 未满：连跑两轮不回收\n'
new_case case5b
start_stack "$S5B" anchor
GUARD_MIN_GAP=300 GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "⑤b 首轮退出码 0" 0 "$GUARD_RC"
GUARD_MIN_GAP=300 GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "⑤b 次轮（间隔 <300s）不动手" 0 "$(count_in "$CLOSE_LOG" "env=$S5B")"
check "⑤b 次轮栈仍存活" alive "$(alive_str "$STACK_DAEMON")"
check_contains "⑤b 次轮 reason=first-seen" "$GUARD_OUT" "reason=first-seen"
GUARD_MIN_GAP=0 GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply
check "⑤b 间隔满足后才回收" 1 "$(count_in "$CLOSE_LOG" "env=$S5B")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑥ 报告模式（不带 --apply）→ would-reap 且进程/profile 原样，退出码 1
#    S5：--quiet 下「发现可回收泄漏」仍出声，且 --json 账本不被吞
# ---------------------------------------------------------------------------
printf '\n⑥ 报告模式 → would-reap 且不动手（--quiet 仍出声）\n'
new_case case6
start_stack "$S6" anchor
GUARD_CLOSE_CMD="$STUB_OK" guard_run_twice
check "⑥ 退出码 1（报告模式发现泄漏）" 1 "$GUARD_RC"
check_contains "⑥ 标 would-reap" "$GUARD_OUT" "would-reap session=$S6"
check "⑥ 未调用 close" 0 "$(count_in "$CLOSE_LOG" "env=$S6")"
check "⑥ daemon 仍在" alive "$(alive_str "$STACK_DAEMON")"
check "⑥ chromium 仍在" alive "$(alive_str "$STACK_CHROMIUM")"
check "⑥ profile 目录仍在" here "$(present_str "$STACK_PROFILE")"
guard_run --quiet --json
check_contains "⑥ --quiet 下 would-reap 仍出声" "$GUARD_ERR" "would-reap session=$S6"
check_contains "⑥ --quiet 下 --json 不被抑制" "$GUARD_OUT" '"outcome":"would-reap"'
check "⑥ --quiet --json 退出码 1" 1 "$GUARD_RC"
cleanup_case

# ---------------------------------------------------------------------------
# ⑦ close 失败 → TERM/KILL 降级（fixture 装 SIG_IGN TERM，逼到 KILL 一档）
# ---------------------------------------------------------------------------
printf '\n⑦ close 失败 → TERM/KILL 降级\n'
new_case case7
start_stack "$S7" anchor term-ign
GUARD_CLOSE_CMD="$STUB_FAIL" guard_run_twice --apply
check "⑦ 退出码 0（降级后仍回收成功）" 0 "$GUARD_RC"
check_contains "⑦ 报告 reaped" "$GUARD_OUT" "reaped session=$S7"
check_contains "⑦ 走到 KILL 一档（escalation=kill）" "$GUARD_OUT" "escalation=kill"
check_contains "⑦ close 失败有留痕" "$GUARD_OUT" "close 失败"
check "⑦ daemon 已退出" dead "$(alive_str "$STACK_DAEMON")"
check "⑦ chromium 已退出" dead "$(alive_str "$STACK_CHROMIUM")"
check "⑦ profile 目录已删除" gone "$(present_str "$STACK_PROFILE")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑧ close 返回非 0 但栈已自退 → escalation=close-gone，不降级发信号（S3）
# ---------------------------------------------------------------------------
printf '\n⑧ close 返回非 0 但栈已自退 → close-gone\n'
new_case case8
start_stack "$S8" anchor
GUARD_CLOSE_CMD="$STUB_GONE" guard_run_twice --apply
check "⑧ 退出码 0" 0 "$GUARD_RC"
check_contains "⑧ 报告 reaped" "$GUARD_OUT" "reaped session=$S8"
check_contains "⑧ 记 escalation=close-gone" "$GUARD_OUT" "escalation=close-gone"
check_absent "⑧ 未降级发信号" "$GUARD_OUT" "降级信号回收"
check "⑧ daemon 已退出" dead "$(alive_str "$STACK_DAEMON")"
check "⑧ profile 目录已删除" gone "$(present_str "$STACK_PROFILE")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑨ 独立无关 chromium（父为 shell、独立进程组、cmdline 无 profile 锚点）→ 不命中、不被误杀
# ---------------------------------------------------------------------------
printf '\n⑨ 独立无关 chromium → 不命中、不被误杀\n'
new_case case9
start_stack "$S9" anchor
setsid "$CASE_DIR/fake-chromium" -c 'sleep 1000; :' --some-unrelated-flag >/dev/null 2>&1 &
UNRELATED=$!
FIXTURE_PIDS+=("$UNRELATED")
disown 2>/dev/null || true
sleep 0.2
GUARD_CLOSE_CMD="$STUB_OK" guard_run_twice --apply
check "⑨ 退出码 0" 0 "$GUARD_RC"
check_contains "⑨ 真泄漏栈被回收" "$GUARD_OUT" "reaped session=$S9"
check "⑨ 无关 chromium 仍在（未被误杀）" alive "$(alive_str "$UNRELATED")"
check "⑨ 无关 chromium 不在报告名单里" 0 "$(count_in "$TMP/guard.out" "pids=[$UNRELATED]")"
check "⑨ 无关 chromium 不在 JSON pids 数组里" 0 "$(count_in "$TMP/guard.out" ",$UNRELATED,")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑩ 多目标 fail-open：一套 no-chromium + 一套正常 → 正常那套仍被回收（整轮不中断）
#    S5：--quiet 下 no-chromium 仍出声
# ---------------------------------------------------------------------------
printf '\n⑩ 多目标 fail-open（no-chromium + 正常）\n'
new_case case10
start_stack "$S10A" no-anchor
D10A=$STACK_DAEMON C10A=$STACK_CHROMIUM P10A=$STACK_PROFILE
start_stack "$S10B" anchor
D10B=$STACK_DAEMON C10B=$STACK_CHROMIUM P10B=$STACK_PROFILE
GUARD_CLOSE_CMD="$STUB_OK" guard_run_twice --apply
check "⑩ 退出码 0（no-chromium 不算失败）" 0 "$GUARD_RC"
check_contains "⑩ 无锚点那套标 no-chromium" "$GUARD_OUT" "reason=no-chromium"
check_contains "⑩ 正常那套仍被回收（fail-open 不中断）" "$GUARD_OUT" "reaped session=$S10B"
check "⑩ 无锚点 daemon 仍在" alive "$(alive_str "$D10A")"
check "⑩ 无锚点 chromium 仍在" alive "$(alive_str "$C10A")"
check "⑩ 正常那套 daemon 已退出" dead "$(alive_str "$D10B")"
check "⑩ 正常那套 profile 已删除" gone "$(present_str "$P10B")"
check "⑩ 无锚点那套未被 close" 0 "$(count_in "$CLOSE_LOG" "env=$S10A")"
guard_run --quiet
check "⑩ --quiet 下 no-chromium 仍出声" 1 "$(count_in "$TMP/guard.out" "no-chromium")"
check "⑩ --quiet 退出码 0" 0 "$GUARD_RC"
cleanup_case

# ---------------------------------------------------------------------------
# ⑪ --json 账本：dry-run（would-reap）与 apply（reaped）各一 + runtime 残留清理（S6）
# ---------------------------------------------------------------------------
printf '\n⑪ --json 账本（dry-run + apply）与 runtime 清理\n'
new_case case11
start_stack "$S11" anchor
GUARD_CLOSE_CMD="$STUB_OK" guard_run
PRIME_RC=$GUARD_RC
PRIME_OUT=$GUARD_OUT
# 伪造 agent-browser 的 runtime 残留（socket/pid），验证回收后尽力清理
mkdir -p "$CASE_RUNTIME/agent-browser"
: >"$CASE_RUNTIME/agent-browser/$S11.sock"
: >"$CASE_RUNTIME/agent-browser/$S11.pid"
GUARD_CLOSE_CMD="$STUB_OK" guard_run --json
check "⑪ dry-run 退出码 1" 1 "$GUARD_RC"
check_contains "⑪ JSON 有 mode" "$GUARD_OUT" '{"mode":"dry-run"'
check_contains "⑪ outcome=would-reap" "$GUARD_OUT" '"outcome":"would-reap"'
check_contains "⑪ session 字段" "$GUARD_OUT" "\"session\":\"$S11\""
check_contains "⑪ daemon_pid 字段" "$GUARD_OUT" '"daemon_pid":'"$STACK_DAEMON"
check_contains "⑪ pids 数组" "$GUARD_OUT" '"pids":['
check_contains "⑪ pgids 数组" "$GUARD_OUT" '"pgids":['
check_contains "⑪ age_s 字段" "$GUARD_OUT" '"age_s":'
check_contains "⑪ profile 字段" "$GUARD_OUT" '"profile":"'"$STACK_PROFILE"'"'
check_contains "⑪ 汇总 summary" "$GUARD_OUT" '"summary":{"stacks":1,"would_reap":1'
check_contains "⑪ reason 为 null" "$GUARD_OUT" '"reason":null'
check "⑪ cpu_ticks_before == cpu_ticks_after（冻结）" \
	"$(json_field cpu_ticks_before)" "$(json_field cpu_ticks_after)"
# --json 的 stdout 必须是纯 JSON（人读行走 stderr），否则 `--json | jq` 解析不了
check "⑪ --json 的 stdout 不含人读行" 0 "$(printf '%s\n' "$GUARD_OUT" | grep -c '^\[browser-leak-guard\]')"
check "⑪ --json 的 stdout 以 { 开头" "{" "$(printf '%s' "$GUARD_OUT" | cut -c1)"
check "⑪ --json 的 stdout 只有一行" 1 "$(printf '%s\n' "$GUARD_OUT" | wc -l | tr -d ' ')"
check "⑪ dry-run 未动进程" alive "$(alive_str "$STACK_DAEMON")"
check "⑪ dry-run 未删 profile" here "$(present_str "$STACK_PROFILE")"
check "⑪ dry-run 未删 runtime 残留" here "$(present_str "$CASE_RUNTIME/agent-browser/$S11.sock")"
GUARD_CLOSE_CMD="$STUB_OK" guard_run --apply --json
check "⑪ apply 退出码 0" 0 "$GUARD_RC"
check_contains "⑪ apply outcome=reaped" "$GUARD_OUT" '"outcome":"reaped"'
check_contains "⑪ apply escalation=close" "$GUARD_OUT" '"escalation":"close"'
check_contains "⑪ profile_removed=true" "$GUARD_OUT" '"profile_removed":true'
check_contains "⑪ runtime_kept=false" "$GUARD_OUT" '"runtime_kept":false'
check "⑪ runtime socket 已清理" gone "$(present_str "$CASE_RUNTIME/agent-browser/$S11.sock")"
check "⑪ runtime pid 文件已清理" gone "$(present_str "$CASE_RUNTIME/agent-browser/$S11.pid")"
cleanup_case

# ---------------------------------------------------------------------------
# ⑫ 非法参数 / 非法注入口 → 退出码 2（S2 注入口硬校验 + 参数校验）
# ---------------------------------------------------------------------------
printf '\n⑫ 非法参数 / 非法注入口 → exit 2\n'
new_case case12
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX 空串 → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=relative/path)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX 相对路径 → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=/)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX=/ → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=/tmp/)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX 以 / 结尾 → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=/tmp)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX 只有 1 段 → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_PROFILE_PREFIX=/tmp/../etc)
guard_run --apply
check "⑫ BLG_PROFILE_PREFIX 含 .. → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=(BLG_DAEMON_PATTERN=)
guard_run --apply
check "⑫ BLG_DAEMON_PATTERN 空串 → 2" 2 "$GUARD_RC"
GUARD_ENV_EXTRA=()
guard_run --min-age abc
check "⑫ --min-age 非整数 → 2" 2 "$GUARD_RC"
guard_run --idle-samples 1
check "⑫ --idle-samples 1 → 2" 2 "$GUARD_RC"
guard_run --bogus-flag
check "⑫ 未知参数 → 2" 2 "$GUARD_RC"

# ---------------------------------------------------------------------------
# ⑬ 无泄漏（无 fixture）→ 零输出、退出码 0
# ---------------------------------------------------------------------------
printf '\n⑬ 无泄漏 → 零输出退出码 0\n'
new_case case13
guard_run --apply --quiet
check "⑬ 退出码 0" 0 "$GUARD_RC"
check "⑬ --quiet 零输出" "" "$GUARD_OUT"
check "⑬ stderr 干净" "" "$GUARD_ERR"

# ---------------------------------------------------------------------------
# ⑭ unit 静态断言 + 脚本静态断言（failed 路径无法用非 root fixture 造出不可杀进程，改静态兜底）
# ---------------------------------------------------------------------------
printf '\n⑭ unit / 脚本静态断言\n'
SERVICE_TXT="$(cat "$UNIT_SERVICE_FILE" 2>/dev/null)"
TIMER_TXT="$(cat "$UNIT_TIMER_FILE" 2>/dev/null)"
SERVICE_EXEC="$(grep -E '^ExecStart=' "$UNIT_SERVICE_FILE" 2>/dev/null)"
SERVICE_ENV="$(grep -E '^Environment=PATH=' "$UNIT_SERVICE_FILE" 2>/dev/null)"
check_contains "⑭ ExecStart 带 --apply" "$SERVICE_EXEC" "--apply"
check_contains "⑭ ExecStart 带 --quiet" "$SERVICE_EXEC" "--quiet"
check_contains "⑭ ExecStart 用 %h 路径" "$SERVICE_EXEC" "%h/software/Syntopica"
check_contains "⑭ TimeoutStartSec=10min" "$SERVICE_TXT" "TimeoutStartSec=10min"
check_absent "⑭ Environment=PATH 不再写死 nvm node 版本" "$SERVICE_ENV" ".nvm"
check_contains "⑭ timer OnBootSec=5min" "$TIMER_TXT" "OnBootSec=5min"
check_contains "⑭ timer OnUnitActiveSec=10min" "$TIMER_TXT" "OnUnitActiveSec=10min"
check_contains "⑭ timer Unit 指向 service" "$TIMER_TXT" "Unit=syntopica-browser-leak-guard.service"
check_contains "⑭ timer WantedBy=timers.target" "$TIMER_TXT" "WantedBy=timers.target"
check_contains "⑭ 脚本 MIN_AGE 默认 1800" "$(grep -n 'MIN_AGE="\${BLG_MIN_AGE' "$GUARD")" "1800"
check_contains "⑭ 脚本回收失败走 say_always（--quiet 不吞）" "$(grep -n 'say_always "failed' "$GUARD")" 'say_always "failed'
check_contains "⑭ 脚本用 flock -n 单实例" "$(grep -n 'flock -n' "$GUARD")" "flock -n"
check_contains "⑭ 脚本把组杀限制在组首自证集合" "$(grep -n 'NOGROUP_PGIDS+=' "$GUARD")" "NOGROUP_PGIDS"

# ---------------------------------------------------------------------------
printf '\n结果: %d 通过 / %d 失败\n' "$PASS" "$FAIL"
if [ "$FAIL" -gt 0 ]; then
	printf '失败项:\n'
	for f in "${FAILED_CASES[@]}"; do
		printf '  - %s\n' "$f"
	done
	exit 1
fi
exit 0
