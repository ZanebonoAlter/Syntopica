#!/usr/bin/env bash
# browser-leak-guard.sh — agent-browser 无头浏览器栈「无主残留」定时回收（add-browser-leak-guard）
#
# 与 dev-process-guard 的分工（design D1）：
#   - 扩展（.pi/extensions/dev-process-guard.ts）管「本会话窗口内」启动的泄漏类 dev 进程，
#     会话结束时自动清，窗口外历史遗留只软提醒、不自动杀；
#   - 本脚本管它管不到的那一类：跨会话累积、窗口外无主、或会话还在但忘了 close 的
#     agent-browser 浏览器栈。范围只收窄到两个窄锚点（见下），不碰后端/前端/数据库等
#     其他 dev 进程（仍归扩展）。
#
# 栈识别（design D3 前置条件）：
#   守护进程 = /proc/<pid>/cmdline 含 BLG_DAEMON_PATTERN（默认 agent-browser/bin/agent-browser-）
#             ∧ /proc/<pid>/environ 含 AGENT_BROWSER_DAEMON=1；session 名取该 environ 的
#             AGENT_BROWSER_SESSION（缺省 default）。
#   浏览器树 = 守护进程的全部后代中 cmdline 含 --user-data-dir=<BLG_PROFILE_PREFIX>（默认
#             /tmp/agent-browser-chrome-）且不带 --type= 者（= chromium 根，实测是 daemon
#             子进程且自成一个进程组）＋该根的整组＋根的 profile 目录。
#   用户自起 chromium（不带该临时 profile 标志）永不进入候选。
#
# 安全五条件（合取，缺一跳过并标原因；design D3 表 + D3b 跨轮次证据）：
#   ① young           栈年龄 < --min-age（默认 1800s；从守护进程 /proc/<pid>/stat 的 starttime
#                     换算 btime + starttime/CLK_TCK。1800 = 上游 agent-browser daemon 自带
#                     1h idle 关停的一半：既减少钉内存，又不与「人在用、两次命令之间在想事」抢会话）
#   ② busy            两次采样（--idle-samples，默认 2，间隔 1s）栈内 utime+stime 求和不为冻结
#                     （= 正在干活；采样间隔内重读 /proc 并用 starttime 防止 PID 复用）
#   ③ first-seen      跨轮次冻结证据缺失（design D3b）：状态文件
#                     $XDG_RUNTIME_DIR/syntopica-browser-leak-guard/state 里没有「同 session +
#                     同 daemon starttime + 同 cpu ticks」条目 → 本轮只落盘、不动手。单轮 1s
#                     采样窗分不清「真死」与「LLM 两次 CLI 调用之间在思考」，故必须两轮取证。
#                     ticks/starttime 变了 → 用新值覆盖、重新计首轮；session 消失 → 条目清理。
#                     目录不可写 → 打一次警告并跳过该条件（fail-open 到「不回收」）。
#   ④ client-attached 除栈自身外，没有活着的 CLI 客户端：environ 含同 session 名**或** argv 含
#                     --session <名> / --session=<名>（上游主流写法），且 cmdline 含 agent-browser。
#                     排除 AGENT_BROWSER_DAEMON=1 的守护进程本身，以及继承 env 的
#                     chrome_crashpad_handler（实测它继承 AGENT_BROWSER_SESSION/DAEMON，且 cmdline
#                     带 --database=/tmp/agent-browser-chrome-<uuid>/Crashpad，含 agent-browser 字样）。
#                     判定一律用「动手前的即时快照」现扫 /proc，不复用巡检开头的旧快照。
#   ⑤ self-protected  待处置 PGID 为 1 / 脚本自身进程组 / 组首不自证（/proc/<pid>/stat 的 pgrp
#                     != pid）——不自证的组不进组杀名单，只单 PID kill（S1）
#   额外保守跳过：no-chromium（没找到 chromium 锚点 → 枚举不全，不动手）、vanished、error
#
# 回收（design D4，仅 --apply）：先 AGENT_BROWSER_SESSION=<名> <BLG_CLOSE_CMD> --session <名>
#   （默认 `agent-browser close`，整条命令 15s 超时）→ 宽限 5s（每 200ms 轮询）→ close 返回
#   非 0 时先复核栈是否其实已自退（已退记 escalation=close-gone，不发信号）→ 仍存活则
#   kill -TERM 各组首自证的目标 PGID + 未退出的栈内 PID（按 starttime 校验）→ 2s → kill -KILL；
#   全部退出后复核无进程 cmdline/fd 仍引用该 profile 才 rm -rf，并尽力清
#   $XDG_RUNTIME_DIR/agent-browser/<session>.*（复核无持有者；清不掉只记 runtime_kept，不算失败）。
#   每个目标整段 fail-open（一个目标炸不影响其他目标）。
#
# 单实例：flock -n 锁 $XDG_RUNTIME_DIR/syntopica-browser-leak-guard/lock，拿不到锁直接 exit 0
#   （定时器与人工并发时后者静默退出，防两边判同一栈、双关同会话）。
#
# 用法：
#   bash scripts/harness/browser-leak-guard.sh               # 默认 dry-run：只报告
#   bash scripts/harness/browser-leak-guard.sh --apply       # 真回收（定时器用这个）
#   bash scripts/harness/browser-leak-guard.sh --json        # 机器可读账本
#   bash scripts/harness/browser-leak-guard.sh --quiet       # 无泄漏/无动作时零输出（定时器用）
#   bash scripts/harness/browser-leak-guard.sh --min-age 900 --idle-samples 3
#   bash scripts/harness/browser-leak-guard.sh --install     # 装用户级 timer（10 分钟周期）
#   bash scripts/harness/browser-leak-guard.sh --uninstall   # 卸载
#
# 退出码：
#   0 = 无泄漏（含「有栈但五条件不满足 → 有意跳过」）或 --apply 下全部回收成功
#   1 = 报告模式（未带 --apply）发现可回收泄漏，或 --apply 下有目标回收失败
#   2 = 参数/环境错误（非 Linux、/proc 不可读、非法参数、非法注入口）
#
# 注入口（全部走环境变量，冒烟测试靠这些口子彻底隔离真实栈；非法值一律 exit 2）：
#   BLG_DAEMON_PATTERN  守护进程 cmdline 锚点（默认 agent-browser/bin/agent-browser-，必须非空）
#   BLG_PROFILE_PREFIX  chromium 临时 profile 前缀（默认 /tmp/agent-browser-chrome-；必须非空、
#                       绝对路径、不以 / 结尾、≥2 段路径，且不含 ..）
#   BLG_CLOSE_CMD       优雅关闭命令（默认按 PATH 解析 agent-browser close）
#   BLG_MIN_AGE         栈年龄阈值秒（默认 1800）
#   BLG_IDLE_SAMPLES    采样次数（默认 2，最小 2）
#   BLG_MIN_GAP         跨轮次取证的最小间隔秒（默认 300；防手动连跑两轮就攒够证据）
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_DIR="$SCRIPT_DIR/systemd"
UNIT_SERVICE="syntopica-browser-leak-guard.service"
UNIT_TIMER="syntopica-browser-leak-guard.timer"

# ---- 可注入参数 -----------------------------------------------------------
# 注意：用 ${VAR+set} 而不是 ${VAR:-默认}——显式传空串必须落到 validate_injectables 里被拒
# （exit 2），不能被静默替换成默认值（S2：防 rm -rf 越界与全量命中）。
if [ -n "${BLG_DAEMON_PATTERN+set}" ]; then
	DAEMON_PATTERN="$BLG_DAEMON_PATTERN"
else
	DAEMON_PATTERN="agent-browser/bin/agent-browser-"
fi
if [ -n "${BLG_PROFILE_PREFIX+set}" ]; then
	PROFILE_PREFIX="$BLG_PROFILE_PREFIX"
else
	PROFILE_PREFIX="/tmp/agent-browser-chrome-"
fi
MIN_AGE="${BLG_MIN_AGE:-1800}"
IDLE_SAMPLES="${BLG_IDLE_SAMPLES:-2}"
MIN_GAP="${BLG_MIN_GAP:-300}"

# ---- 运行期开关 -----------------------------------------------------------
APPLY=0
JSON=0
QUIET=0
MODE="dry-run"

# ---- 运行时目录 / 跨轮次状态文件 / 单实例锁 --------------------------------
# XDG_RUNTIME_DIR 不可用（未设/只读）时降级到 TMPDIR → /tmp
RUNTIME_BASE="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}"
STATE_DIR="$RUNTIME_BASE/syntopica-browser-leak-guard"
STATE_FILE="$STATE_DIR/state"
LOCK_FILE="$STATE_DIR/lock"
STATE_OK=0     # 1 = 状态目录可写，跨轮次条件可用
STATE_DIRTY=0  # 1 = 内存里的状态与磁盘不一致，收尾要落盘
STATE_WARNED=0 # 警告只打一次
LOCK_FD=""
declare -A STATE_START=() # session → daemon starttime
declare -A STATE_TICKS=() # session → 冻结时的 cpu ticks
declare -A STATE_EPOCH=() # session → 首次观测时刻
declare -A SEEN_SESSIONS=()

# ---- 快照（一次扫描，多个栈复用；关联数组键 = pid） -----------------------
declare -A P_CMD=()   # cmdline（NUL→空格）
declare -A P_PPID=()
declare -A P_PGID=()
declare -A P_START=() # starttime ticks（PID 复用检测）
declare -A P_TICKS=() # utime+stime
declare -A P_SESS=()  # AGENT_BROWSER_SESSION（空 = 未读/无）
declare -A P_DENV=()  # AGENT_BROWSER_DAEMON=1 → 1
ALL_PIDS=()

# ---- 当前目标栈（build_stack 产出，reap 消费） ----------------------------
STACK_PIDS=()
STACK_PGIDS=()  # 只含「组首自证」的 PGID（S1）
NOGROUP_PGIDS=() # 组首不自证、被排除出组杀名单的 PGID（只单 PID kill）
declare -A STACK_SET=()
declare -A PGID_SET=()
declare -A STACK_START=()        # pid → starttime（S4：PID 复用护栏）
declare -A PGID_LEADER_START=()  # pgid → 组首 starttime（S4：组 PGID 复用护栏）
DAEMON_PID=""
SESSION=""
PROFILE=""
AGE_S=0
CPU_BEFORE=0
CPU_AFTER=0
CPU_SAMPLED=0 # 1 = 完整跑完冻结采样（S11：未采样时 JSON 输出 null）
RSS_MB=0
CLIENTS=()
ESCALATION=""
RUNTIME_KEPT="" # 空=未涉及 0=已清理 1=保留

# ---- 计数 ----------------------------------------------------------------
N_STACKS=0
N_WOULD=0
N_REAPED=0
N_FAILED=0
N_SKIPPED=0
N_NOC=0 # no-chromium 计数（异常类，--quiet 下也要出声）
JSON_ITEMS=()

BTIME=0
CLK_TCK=100
OUR_PGID=0
NOW=0

# ---------------------------------------------------------------------------
# 输出（journal 友好：每行固定前缀）
#   say        --quiet 时可静音（正常跳过项）
#   say_always 必须出声：发现可回收泄漏 / 回收失败 / no-chromium / 降级告警（S5）
#   --json 模式：人读行一律走 stderr，stdout 只留一行 JSON 账本（否则 `--json | jq` 解析不了）；
#   journal 同时收 stdout+stderr，所以定时器口径不变。
# ---------------------------------------------------------------------------
emit_text() {
	if [ "$JSON" -eq 1 ]; then
		printf '[browser-leak-guard] %s\n' "$*" >&2
	else
		printf '[browser-leak-guard] %s\n' "$*"
	fi
}
say() { [ "$QUIET" -eq 1 ] || emit_text "$*"; }
say_always() { emit_text "$*"; }
warn() { printf '[browser-leak-guard] 错误: %s\n' "$*" >&2; }
die2() {
	warn "$*"
	exit 2
}

usage() {
	cat <<'EOF'
用法: bash scripts/harness/browser-leak-guard.sh [选项]

回收 agent-browser 无头浏览器栈的「无主残留」（跨会话累积 / 会话还在但忘了 close）。
默认 dry-run：只报告不改动。栈识别只认两个窄锚点（daemon 带 AGENT_BROWSER_DAEMON=1 +
chromium 带 --user-data-dir=/tmp/agent-browser-chrome-），用户自起 chromium 不会命中。

选项:
  --apply               真回收（不带则只报告）。定时器 unit 用它
  --json                输出机器可读账本（JSON 一行）
  --quiet               无泄漏/无动作时零输出；发现泄漏、回收失败、no-chromium 仍会出声
  --min-age <秒>        栈年龄阈值，默认 1800（30 分钟）
  --idle-samples <次>   空闲采样次数，默认 2（最小 2，每次间隔 1s）；越大越保守
  --min-gap <秒>        跨轮次取证最小间隔，默认 300（5 分钟）；手动连跑两轮
                        不能凑够证据（定时器 10 分钟周期天然满足）
  --install             安装用户级 systemd timer（10 分钟周期，开机 5 分钟后首跑）
  --uninstall           卸载用户级 systemd timer（disable --now + 删 unit）
  -h, --help            显示本帮助

退出码:
  0  无泄漏（含「有栈但安全五条件不满足 → 有意跳过」）或 --apply 下全部回收成功
  1  报告模式（未带 --apply）发现可回收泄漏，或 --apply 下有目标回收失败
  2  参数/环境错误（非 Linux、/proc 不可读、非法参数、非法注入口）

安全五条件（合取，任一不满足即跳过并标原因）:
  young            栈年龄 < --min-age
  busy             两次采样栈内 CPU 时间不为冻结（正在干活）
  first-seen       跨轮次冻结证据缺失（本轮只落状态文件、不动手；下一轮仍冻结才回收）
  client-attached  有 agent-browser CLI（eval/screenshot/click...）挂在同 session 上
                   （environ 的 AGENT_BROWSER_SESSION 或 argv 的 --session <名> 都认）
  self-protected   待处置 PGID 为 1 / 脚本自身进程组 / 组首不自证（只单 PID kill）

跨轮次状态文件:
  $XDG_RUNTIME_DIR/syntopica-browser-leak-guard/state
  每行: session<TAB>daemon_starttime<TAB>cpu_ticks<TAB>epoch
  目录不可写时打一次警告并跳过该条件（fail-open 到「不回收」）。

单实例:
  flock -n 锁同目录下 lock；拿不到锁（定时器与人工并发）直接 exit 0。

环境注入口（非法值一律 exit 2）:
  BLG_DAEMON_PATTERN   daemon cmdline 锚点，默认 agent-browser/bin/agent-browser-（非空）
  BLG_PROFILE_PREFIX   chromium 临时 profile 前缀，默认 /tmp/agent-browser-chrome-
                       （非空 / 绝对路径 / 不以 / 结尾 / ≥2 段）
  BLG_CLOSE_CMD        优雅关闭命令，默认按 PATH 解析 agent-browser close
  BLG_MIN_AGE          同 --min-age
  BLG_IDLE_SAMPLES     同 --idle-samples
  BLG_MIN_GAP          同 --min-gap

与 dev-process-guard 的分工:
  扩展管「会话窗口内」启动的泄漏进程（会话结束自动清）；本脚本补位「窗口外常驻无主栈」，
  由用户级 systemd timer 每 10 分钟跑一次。安装/日志/卸载见 unit 文件头。
EOF
}

# ---------------------------------------------------------------------------
# 参数解析
# ---------------------------------------------------------------------------
parse_args() {
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--apply) APPLY=1 ;;
		--json) JSON=1 ;;
		--quiet) QUIET=1 ;;
		--min-age)
			[ "$#" -ge 2 ] || die2 "--min-age 缺参数"
			MIN_AGE="$2"
			shift
			;;
		--min-age=*) MIN_AGE="${1#*=}" ;;
		--idle-samples)
			[ "$#" -ge 2 ] || die2 "--idle-samples 缺参数"
			IDLE_SAMPLES="$2"
			shift
			;;
		--idle-samples=*) IDLE_SAMPLES="${1#*=}" ;;
		--min-gap)
			[ "$#" -ge 2 ] || die2 "--min-gap 缺参数"
			MIN_GAP="$2"
			shift
			;;
		--min-gap=*) MIN_GAP="${1#*=}" ;;
		--install) MODE="install" ;;
		--uninstall) MODE="uninstall" ;;
		-h | --help)
			usage
			exit 0
			;;
		*) die2 "未知参数: $1（--help 看用法）" ;;
		esac
		shift
	done

	[ "$APPLY" -eq 1 ] && MODE="apply"

	case "$MIN_AGE" in
	'' | *[!0-9]*) die2 "--min-age 必须是整数秒: $MIN_AGE" ;;
	esac
	case "$IDLE_SAMPLES" in
	'' | *[!0-9]*) die2 "--idle-samples 必须是整数: $IDLE_SAMPLES" ;;
	esac
	case "$MIN_GAP" in
	'' | *[!0-9]*) die2 "--min-gap 必须是整数秒: $MIN_GAP" ;;
	esac
	# 采样 <2 次等于不验「冻结」这一条 → 必须拦住（design D3 条件②）
	[ "$IDLE_SAMPLES" -ge 2 ] || die2 "--idle-samples 至少 2（默认 2）"
	[ "$IDLE_SAMPLES" -le 20 ] || die2 "--idle-samples 最多 20（每次间隔 1s，别把 unit 跑超时）"
}

# ---------------------------------------------------------------------------
# 注入口硬校验（S2）：不合法一律 exit 2——PROFILE_PREFIX 是 rm -rf 的目标前缀，
# DAEMON_PATTERN 是「哪些进程算守护进程」的锚点，两者都不能靠运气。
# ---------------------------------------------------------------------------
validate_injectables() {
	[ -n "$DAEMON_PATTERN" ] || die2 "BLG_DAEMON_PATTERN 不能为空（否则全量命中）"

	case "$PROFILE_PREFIX" in
	'') die2 "BLG_PROFILE_PREFIX 不能为空（否则 rm -rf 越界）" ;;
	/*) ;;
	*) die2 "BLG_PROFILE_PREFIX 必须是绝对路径: $PROFILE_PREFIX" ;;
	esac
	[ "$PROFILE_PREFIX" = "/" ] && die2 "BLG_PROFILE_PREFIX 不能是 /"
	case "$PROFILE_PREFIX" in
	*/) die2 "BLG_PROFILE_PREFIX 不能以 / 结尾: $PROFILE_PREFIX" ;;
	esac
	case "$PROFILE_PREFIX" in
	*..*) die2 "BLG_PROFILE_PREFIX 不能含 ..: $PROFILE_PREFIX" ;;
	esac
	# ≥2 段：/tmp/agent-browser-chrome- → 2 段；/foo → 1 段 → 拒
	local rest="${PROFILE_PREFIX#/}"
	local slashes="${rest//[!\/]/}"
	if [ "$(( ${#slashes} + 1 ))" -lt 2 ]; then
		die2 "BLG_PROFILE_PREFIX 至少要有 2 段路径: $PROFILE_PREFIX"
	fi
}

# ---------------------------------------------------------------------------
# 环境自检（非 Linux / /proc 不可读 → exit 2）
# ---------------------------------------------------------------------------
env_check() {
	[ -r /proc/self/stat ] || die2 "只支持 Linux：/proc/self/stat 不可读"
	[ -d /proc ] || die2 "只支持 Linux：/proc 不存在"

	BTIME="$(awk '/^btime /{print $2; exit}' /proc/stat 2>/dev/null)"
	case "$BTIME" in '' | *[!0-9]*) die2 "/proc/stat 无 btime 行，无法换算进程年龄" ;; esac

	local tck
	tck="$(getconf CLK_TCK 2>/dev/null || true)"
	case "$tck" in
	'' | *[!0-9]*) CLK_TCK=100 ;;
	*) [ "$tck" -gt 0 ] && CLK_TCK="$tck" ;;
	esac

	NOW="$(date +%s)"
	read_stat "$$" || die2 "读不到自身 /proc/$$/stat"
	OUR_PGID="$ST_PGID"
}

# ---------------------------------------------------------------------------
# /proc 读取（无子 shell：600 个进程全量扫描不能每个都 fork 一次）
# ---------------------------------------------------------------------------
CL=""        # read_cmdline 输出
ST_STATE=""  # read_stat 输出
ST_PPID=""
ST_PGID=""
ST_UTIME=0
ST_STIME=0
ST_START=0
ENV_SESSION="" # read_environ 输出
ENV_DAEMON=0

# cmdline：NUL 分隔 → 空格 join。进程扫描中退出 → 非 0（调用方跳过，fail-open）
read_cmdline() {
	CL=""
	local -a parts=()
	mapfile -d '' -t parts 2>/dev/null <"/proc/$1/cmdline" || return 1
	local IFS=' '
	CL="${parts[*]}"
	return 0
}

# stat：comm 可能含空格与右括号（如 "(next-server (v1.2))"），必须取最后一个 ')' 之后的字段段。
# 字段序（')' 之后 0 起算）：0=state 1=ppid 2=pgrp 11=utime 12=stime 19=starttime
read_stat() {
	local line rest
	local -a f=()
	read -r line 2>/dev/null <"/proc/$1/stat" || return 1
	rest="${line##*)}"
	read -ra f <<<"$rest"
	[ "${#f[@]}" -ge 20 ] || return 1
	ST_STATE="${f[0]}"
	ST_PPID="${f[1]}"
	ST_PGID="${f[2]}"
	ST_UTIME="${f[11]}"
	ST_STIME="${f[12]}"
	ST_START="${f[19]}"
	case "$ST_PGID" in '' | *[!0-9]*) return 1 ;; esac
	case "$ST_START" in '' | *[!0-9]*) return 1 ;; esac
	return 0
}

# environ：只关心两个变量。读不到（权限/进程消失）→ 非 0，调用方按「不是 daemon」处理
read_environ() {
	ENV_SESSION=""
	ENV_DAEMON=0
	local -a parts=()
	mapfile -d '' -t parts 2>/dev/null <"/proc/$1/environ" || return 1
	local kv
	for kv in "${parts[@]}"; do
		case "$kv" in
		AGENT_BROWSER_SESSION=*) ENV_SESSION="${kv#AGENT_BROWSER_SESSION=}" ;;
		AGENT_BROWSER_DAEMON=1) ENV_DAEMON=1 ;;
		esac
	done
	return 0
}

# 全量扫描一次（cmdline + stat 全读；environ 只读可能相关的进程，省掉 600 次读）
snapshot() {
	ALL_PIDS=()
	P_CMD=() P_PPID=() P_PGID=() P_START=() P_TICKS=() P_SESS=() P_DENV=()
	local d pid
	for d in /proc/[0-9]*; do
		pid="${d#/proc/}"
		case "$pid" in '' | *[!0-9]*) continue ;; esac
		read_cmdline "$pid" || continue
		read_stat "$pid" || continue
		P_CMD[$pid]="$CL"
		P_PPID[$pid]="$ST_PPID"
		P_PGID[$pid]="$ST_PGID"
		P_START[$pid]="$ST_START"
		P_TICKS[$pid]=$((ST_UTIME + ST_STIME))
		P_SESS[$pid]=""
		P_DENV[$pid]=0
		case "$CL" in
		*agent-browser* | *"$DAEMON_PATTERN"*)
			# 只有 cmdline 沾 agent-browser（或自定义锚点）的进程才可能是 daemon 或 CLI 客户端
			if read_environ "$pid"; then
				P_SESS[$pid]="$ENV_SESSION"
				P_DENV[$pid]="$ENV_DAEMON"
			fi
			;;
		esac
		ALL_PIDS+=("$pid")
	done
}

# ---------------------------------------------------------------------------
# 跨轮次冻结证据（B2 / design D3b）
#   状态文件每行: session<TAB>daemon_starttime<TAB>cpu_ticks<TAB>epoch
#   内存里按 session 存一条（starttime 变化 = 会话重启 = 新栈，ticks 变化 = 重新取证）
# ---------------------------------------------------------------------------
state_warn() {
	[ "$STATE_WARNED" -eq 1 ] && return 0
	STATE_WARNED=1
	say_always "warn: 状态目录不可写（$STATE_DIR）→ 跨轮次冻结证据不可用，本轮只取证不回收（fail-open）"
}

state_init() {
	STATE_OK=0
	if ! mkdir -p "$STATE_DIR" 2>/dev/null; then
		state_warn
		return 1
	fi
	local probe="$STATE_DIR/.probe.$$"
	if ! : >"$probe" 2>/dev/null; then
		state_warn
		return 1
	fi
	rm -f "$probe" 2>/dev/null
	STATE_OK=1
	return 0
}

state_load() {
	STATE_START=() STATE_TICKS=() STATE_EPOCH=()
	STATE_DIRTY=0
	[ "$STATE_OK" -eq 1 ] || return 1
	[ -r "$STATE_FILE" ] || return 0
	local line sess start ticks epoch
	while IFS=$'\t' read -r sess start ticks epoch; do
		[ -n "$sess" ] || continue
		case "$start" in '' | *[!0-9]*) continue ;; esac
		case "$ticks" in '' | *[!0-9]*) continue ;; esac
		case "$epoch" in '' | *[!0-9]*) epoch=0 ;; esac
		STATE_START[$sess]="$start"
		STATE_TICKS[$sess]="$ticks"
		STATE_EPOCH[$sess]="$epoch"
	done <"$STATE_FILE"
	return 0
}

# 返回 0 = 上轮已观测到「同 session + 同 starttime + 同 ticks」的冻结；
# 返回 1 = 首次观测（已写入内存状态，收尾落盘），调用方标 first-seen 不动手。
cross_round_frozen() {
	local st="${P_START[$DAEMON_PID]:-0}" ticks="$CPU_AFTER"
	if [ "$STATE_OK" -ne 1 ]; then
		state_warn
		return 1
	fi
	# session 名来自 environ：含制表符/换行会破坏状态文件的行格式（甚至伪造出别的 session 条目），
	# 这类怪名字直接跳过跨轮次条件（fail-open 到「不回收」）
	case "$SESSION" in
	*[[:cntrl:]]*) return 1 ;;
	esac
	if [ "${STATE_START[$SESSION]:-}" = "$st" ] && [ "${STATE_TICKS[$SESSION]:-}" = "$ticks" ]; then
		# 两轮之间必须真的隔了 --min-gap 秒：否则手动连跑两次 `--apply` 就能用几秒的
		# 证据给真实栈「上膛」（审查遗留项）。间隔不足时不回收也不刷新 epoch，
		# 保留最早取证时刻，让后续真隔开的轮次能凑满间隔。
		local prev_epoch="${STATE_EPOCH[$SESSION]:-0}"
		if [ "$prev_epoch" -gt 0 ] && [ $((NOW - prev_epoch)) -ge "$MIN_GAP" ]; then
			STATE_EPOCH[$SESSION]="$NOW"
			STATE_DIRTY=1
			return 0
		fi
		return 1
	fi
	STATE_START[$SESSION]="$st"
	STATE_TICKS[$SESSION]="$ticks"
	STATE_EPOCH[$SESSION]="$NOW"
	STATE_DIRTY=1
	return 1
}

# 收尾落盘：清理本轮已不存在的 session 条目；临时文件 + mv 原子替换
state_flush() {
	[ "$STATE_OK" -eq 1 ] || return 0
	local sess tmp
	for sess in "${!STATE_START[@]}"; do
		[ -n "${SEEN_SESSIONS[$sess]:-}" ] && continue
		unset 'STATE_START[$sess]' 'STATE_TICKS[$sess]' 'STATE_EPOCH[$sess]'
		STATE_DIRTY=1
	done
	[ "$STATE_DIRTY" -eq 1 ] || return 0
	tmp="$STATE_DIR/.state.$$"
	{
		for sess in "${!STATE_START[@]}"; do
			printf '%s\t%s\t%s\t%s\n' "$sess" "${STATE_START[$sess]}" "${STATE_TICKS[$sess]}" "${STATE_EPOCH[$sess]:-0}"
		done
	} >"$tmp" 2>/dev/null || {
		state_warn
		rm -f "$tmp" 2>/dev/null
		return 1
	}
	if ! mv -f "$tmp" "$STATE_FILE" 2>/dev/null; then
		state_warn
		rm -f "$tmp" 2>/dev/null
		return 1
	fi
	STATE_DIRTY=0
	return 0
}

# 单实例锁（S7）：非阻塞，拿不到锁直接让调用方 exit 0（静默）
acquire_lock() {
	command -v flock >/dev/null 2>&1 || return 0 # 无 flock(1) 就不加锁，不阻断巡检
	[ "$STATE_OK" -eq 1 ] || return 0           # 目录不可写时反正什么也不会回收，不必加锁
	# 注意：用 { ...; } 2>/dev/null 包住——直接写 `exec {fd}>file 2>/dev/null` 时重定向失败
	# 的报错是在重定向生效前由 shell 报的，会带着裸路径漏到 stderr（journal 里一条无前缀噪音）
	if ! { exec {LOCK_FD}>"$LOCK_FILE"; } 2>/dev/null; then
		return 0
	fi
	if ! flock -n "$LOCK_FD" 2>/dev/null; then
		return 1
	fi
	return 0
}

# ---------------------------------------------------------------------------
# 栈构建
# ---------------------------------------------------------------------------
# 守护进程：cmdline 命中窄锚点 ∧ environ 有 AGENT_BROWSER_DAEMON=1
find_daemons() {
	local pid
	for pid in "${ALL_PIDS[@]}"; do
		case "${P_CMD[$pid]:-}" in
		*"$DAEMON_PATTERN"*) [ "${P_DENV[$pid]:-0}" -eq 1 ] && printf '%s\n' "$pid" ;;
		esac
	done
}

# 后代：迭代扩张 ppid 属于栈的进程（不依赖 ps，避免树被 reparent 影响）
collect_descendants() {
	local changed=1 pid
	while [ "$changed" -eq 1 ]; do
		changed=0
		for pid in "${ALL_PIDS[@]}"; do
			[ -n "${STACK_SET[$pid]:-}" ] && continue
			if [ -n "${STACK_SET[${P_PPID[$pid]:-0}]:-}" ]; then
				STACK_SET[$pid]=1
				STACK_PIDS+=("$pid")
				changed=1
			fi
		done
	done
}

# 从 cmdline 提取 --user-data-dir=<前缀...> 的完整目录
profile_of() {
	local cl="$1" pfx="--user-data-dir="
	case "$cl" in
	*"$pfx"*) ;;
	*) return 1 ;;
	esac
	local tail="${cl#*"$pfx"}"
	printf '%s' "${tail%% *}"
}

build_stack() { # $1 = daemon pid
	DAEMON_PID="$1"
	STACK_PIDS=("$DAEMON_PID")
	STACK_PGIDS=()
	NOGROUP_PGIDS=()
	STACK_SET=()
	PGID_SET=()
	STACK_START=()
	PGID_LEADER_START=()
	STACK_SET[$DAEMON_PID]=1
	SESSION="${P_SESS[$DAEMON_PID]:-}"
	[ -n "$SESSION" ] || SESSION="default"

	collect_descendants

	# chromium 根：后代中 cmdline 带临时 profile 前缀、且不带 --type=（--type= 是子进程）。
	# 守卫：daemon 自己不算（daemon cmdline 可能因为参数里带了 profile 路径而误命中）。
	PROFILE=""
	local pid cl ppid_first="" leader=""
	for pid in "${STACK_PIDS[@]}"; do
		[ "$pid" = "$DAEMON_PID" ] && continue
		cl="${P_CMD[$pid]:-}"
		case "$cl" in
		*"--user-data-dir=$PROFILE_PREFIX"*) ;;
		*) continue ;;
		esac
		case "$cl" in *" --type="*) continue ;; esac
		if [ "${P_PPID[$pid]:-0}" = "$DAEMON_PID" ] && [ -z "$ppid_first" ]; then
			ppid_first="$pid"
		fi
		if [ "${P_PGID[$pid]:-0}" = "$pid" ] && [ -z "$leader" ]; then
			leader="$pid"
		fi
	done
	local root=""
	[ -n "$ppid_first" ] && root="$ppid_first"
	[ -z "$root" ] && root="$leader"
	if [ -z "$root" ]; then
		# 兜底：任一命中者里取 starttime 最早（= 先起的那个）
		local best="" bestt=0
		for pid in "${STACK_PIDS[@]}"; do
			[ "$pid" = "$DAEMON_PID" ] && continue
			cl="${P_CMD[$pid]:-}"
			case "$cl" in *"--user-data-dir=$PROFILE_PREFIX"*) ;; *) continue ;; esac
			case "$cl" in *" --type="*) continue ;; esac
			if [ -z "$best" ] || [ "${P_START[$pid]:-0}" -lt "$bestt" ]; then
				best="$pid"
				bestt="${P_START[$pid]:-0}"
			fi
		done
		root="$best"
	fi
	[ -n "$root" ] || return 1
	PROFILE="$(profile_of "${P_CMD[$root]}")" || return 1
	[ -n "$PROFILE" ] || return 1

	# chromium 根整组并入栈（实测整组 13 个进程都在根所在 PGID）
	local p
	for pid in "${ALL_PIDS[@]}"; do
		p="${P_PGID[$pid]:-0}"
		[ "$p" = "${P_PGID[$root]:-x}" ] || continue
		[ -n "${STACK_SET[$pid]:-}" ] && continue
		STACK_SET[$pid]=1
		STACK_PIDS+=("$pid")
	done

	# 记录每个栈成员的 starttime（S4：kill 与存活判定都要按 starttime 校验，防 PID 复用误杀/误判）
	for pid in "${STACK_PIDS[@]}"; do
		STACK_START[$pid]="${P_START[$pid]:-}"
	done

	# 待处置 PGID：chromium 根组 + daemon 组（去重）
	# S1 组杀自证：只有 /proc/<pgid>/stat 的 pgrp == pid（组首就是自己）才允许整组杀；
	# 不自证的组只进 NOGROUP_PGIDS 留痕，实际只走单 PID kill（防连带杀调用壳兄弟进程）。
	for p in "${P_PGID[$root]:-}" "${P_PGID[$DAEMON_PID]:-}"; do
		[ -n "$p" ] || continue
		[ -n "${PGID_SET[$p]:-}" ] && continue
		[ "$p" -gt 1 ] 2>/dev/null || continue
		[ "$p" = "$OUR_PGID" ] && continue
		if [ "${P_PGID[$p]:-0}" != "$p" ]; then
			NOGROUP_PGIDS+=("$p")
			continue
		fi
		PGID_SET[$p]=1
		STACK_PGIDS+=("$p")
		PGID_LEADER_START[$p]="${P_START[$p]:-}"
	done
	return 0
}

# ---------------------------------------------------------------------------
# 判定
# ---------------------------------------------------------------------------
# cmdline（NUL→空格 join）里是否有 --session <名> / --session=<名>
cmdline_has_session() { # $1=cmdline $2=session
	local cl="$1" s="$2"
	case "$cl" in
	*"--session=$s") return 0 ;;
	*"--session=$s "*) return 0 ;;
	*"--session $s") return 0 ;;
	*"--session $s "*) return 0 ;;
	esac
	return 1
}

# 同 session 的活 CLI 客户端（条件④）——**现扫 /proc**，不复用巡检开头的快照：
# 每个目标判定要 sleep 1s+，且 apply 前必须用「动手前」的即时快照重算（B1）。
# 判据：cmdline 含 agent-browser ∧ 不是 daemon（AGENT_BROWSER_DAEMON=1）∧ 不是 crashpad helper
#       ∧ session 命中（environ 或 argv）∧ 不在本栈内 ∧ 不是本进程/本进程组（含 close 子进程）。
# 2026-11-13 实测反例：chromium 的 crashpad handler 被 reparent 到 init 并**继承**
# AGENT_BROWSER_SESSION/AGENT_BROWSER_DAEMON，且 cmdline 带
# --database=/tmp/agent-browser-chrome-<uuid>/Crashpad（含 agent-browser 字样）——不显式排除它
# 就会把这种残留 helper 误当客户端，导致真泄漏永远跳不过。
scan_clients_fresh() {
	CLIENTS=()
	local d pid cl denv sess hit
	for d in /proc/[0-9]*; do
		pid="${d#/proc/}"
		case "$pid" in '' | *[!0-9]*) continue ;; esac
		[ "$pid" = "$$" ] && continue
		read_stat "$pid" || continue
		[ "$ST_PGID" = "$OUR_PGID" ] && continue # 自身进程组（含 close 子进程）不算
		# 栈内成员：starttime 对得上才算（对不上 = PID 被复用，不是我们的进程，照常判）
		if [ -n "${STACK_SET[$pid]:-}" ] && [ "$ST_START" = "${STACK_START[$pid]:-}" ]; then
			continue
		fi
		read_cmdline "$pid" || continue
		cl="$CL"
		case "$cl" in *agent-browser*) ;; *) continue ;; esac
		case "$cl" in *chrome_crashpad_handler*) continue ;; esac
		denv=0
		sess=""
		if read_environ "$pid"; then
			denv="$ENV_DAEMON"
			sess="$ENV_SESSION"
		fi
		[ "$denv" -eq 1 ] && continue # 另一个守护进程（会话名可能撞车，但它不是客户端）
		if [ "$SESSION" = "default" ]; then
			# 不带 session 的 daemon = default 会话；不带 session 的 CLI 也落在 default 上，
			# 故 default 会话下任何 agent-browser CLI 都算「可能有人在用」→ 保守跳过
			hit=1
		elif [ "$sess" = "$SESSION" ] || cmdline_has_session "$cl" "$SESSION"; then
			hit=1
		else
			hit=0
		fi
		[ "$hit" -eq 1 ] && CLIENTS+=("$pid")
	done
	[ "${#CLIENTS[@]}" -gt 0 ]
}

stack_rss_mb() {
	local pid total=0 k v _
	for pid in "$@"; do
		while read -r k v _; do
			if [ "$k" = "VmRSS:" ]; then
				total=$((total + v))
				break
			fi
		done 2>/dev/null <"/proc/$pid/status"
	done
	RSS_MB=$((total / 1024))
}

# 条件②：两次采样栈内 utime+stime 求和是否冻结。返回 0=冻结 1=栈在变 2=CPU 在涨
# CPU_SAMPLED 只在「完整跑完冻结采样」时置 1（S11：young/busy/first-seen 未采样 → JSON null）
cpu_frozen() {
	CPU_BEFORE=0
	CPU_AFTER=0
	CPU_SAMPLED=0
	local pid sum
	for pid in "$@"; do
		CPU_BEFORE=$((CPU_BEFORE + ${P_TICKS[$pid]:-0}))
	done
	CPU_AFTER="$CPU_BEFORE"
	local i=1
	while [ "$i" -lt "$IDLE_SAMPLES" ]; do
		sleep 1
		sum=0
		for pid in "$@"; do
			read_stat "$pid" || return 1 # 进程消失 = 栈在变
			[ "$ST_START" = "${P_START[$pid]:-}" ] || return 1 # PID 复用 = 栈在变
			sum=$((sum + ST_UTIME + ST_STIME))
		done
		CPU_AFTER="$sum"
		[ "$sum" -ne "$CPU_BEFORE" ] && return 2
		i=$((i + 1))
	done
	CPU_SAMPLED=1
	return 0
}

# 栈是否还有活着的成员（栈内 PID ∪ 目标 PGID）
# S4：PID 必须 starttime 对得上（PID 复用不算本栈）；组 PGID 必须组首 starttime 对得上
#     （组首 PID 被复用 → 不是我们的组）。否则会因 PID 回收把「已清空」误判成「仍存活 → failed」。
stack_alive() {
	local d pid lead_start cur
	for d in /proc/[0-9]*; do
		pid="${d#/proc/}"
		case "$pid" in '' | *[!0-9]*) continue ;; esac
		read_stat "$pid" || continue
		if [ -n "${STACK_SET[$pid]:-}" ]; then
			[ "$ST_START" = "${STACK_START[$pid]:-}" ] && return 0
			continue
		fi
		[ -n "${PGID_SET[$ST_PGID]:-}" ] || continue
		lead_start="${PGID_LEADER_START[$ST_PGID]:-}"
		if [ -z "$lead_start" ]; then
			return 0 # 组首身份未知（构建时就没自证）→ 保守算存活
		fi
		cur=""
		read_stat "$ST_PGID" && cur="$ST_START"
		if [ -n "$cur" ] && [ "$cur" = "$lead_start" ]; then
			return 0
		fi
		# 组首 PID 已被复用（starttime 变了/不存在）→ 不是我们的组
	done
	return 1
}

# 每 200ms 轮询，最多 $1 秒；返回 0 = 全清
wait_gone() {
	local tries=$(($1 * 5)) i=0
	while [ "$i" -le "$tries" ]; do
		stack_alive || return 0
		sleep 0.2
		i=$((i + 1))
	done
	stack_alive && return 1
	return 0
}

# ---------------------------------------------------------------------------
# 回收（design D4）
# ---------------------------------------------------------------------------
# 解析优雅关闭命令：BLG_CLOSE_CMD 优先；否则 PATH 里的 agent-browser；再否则到 nvm
# 安装位兜底（systemd 用户单元里 PATH 可能不含 node 安装位；unit 不再写死 node 版本，
# 就靠这里的 ~/.nvm/versions/node/*/bin glob 兜底）
CLOSE_ARGV=()
resolve_close_cmd() {
	CLOSE_ARGV=()
	if [ -n "${BLG_CLOSE_CMD:-}" ]; then
		read -ra CLOSE_ARGV <<<"$BLG_CLOSE_CMD"
		return 0
	fi
	if command -v agent-browser >/dev/null 2>&1; then
		CLOSE_ARGV=(agent-browser close)
		return 0
	fi
	local p=""
	for p in "$HOME"/.nvm/versions/node/*/bin/agent-browser; do
		[ -x "$p" ] && CLOSE_ARGV=("$p" close)
	done
	[ "${#CLOSE_ARGV[@]}" -gt 0 ]
}

# 跑一条关闭命令，15s 硬超时（自带轮询，不依赖 timeout(1)）
run_close() {
	local sess="$1" cpid i=0
	[ "${#CLOSE_ARGV[@]}" -gt 0 ] || return 127
	env AGENT_BROWSER_SESSION="$sess" "${CLOSE_ARGV[@]}" --session "$sess" >/dev/null 2>&1 &
	cpid=$!
	while kill -0 "$cpid" 2>/dev/null; do
		i=$((i + 1))
		if [ "$i" -gt 150 ]; then
			kill -TERM "$cpid" 2>/dev/null
			sleep 0.5
			kill -KILL "$cpid" 2>/dev/null
			wait "$cpid" 2>/dev/null
			return 124
		fi
		sleep 0.1
	done
	wait "$cpid" 2>/dev/null
	return $?
}

# 组杀：STACK_PGIDS 已在 build_stack 过滤为「组首自证 ∧ >1 ∧ 非自身组」（S1）
kill_groups() { # $1 = TERM|KILL
	local sig="$1" pg pid
	for pg in "${STACK_PGIDS[@]}"; do
		kill -"$sig" -- "-$pg" 2>/dev/null
	done
	# 兜底：组外/组信号没盖到的栈内 PID（daemon 常在自建组里，但别假定）
	# S4：只杀「还是我们那个进程」的 PID（starttime 一致），防 PID 复用误杀无关进程
	for pid in "${STACK_PIDS[@]}"; do
		[ "$pid" -gt 1 ] 2>/dev/null || continue
		if read_stat "$pid" 2>/dev/null && [ "$ST_START" = "${STACK_START[$pid]:-}" ]; then
			kill -"$sig" "$pid" 2>/dev/null
		fi
	done
}

# 路径是否仍被任何进程引用（cmdline 或打开的 fd）：
#   - profile 目录：1s 内新进程复用同名目录的护栏
#   - runtime socket/pid：KILL 路径不会自清，删前复核有没有人还拿着
path_referenced() {
	local target="$1" d pid fd ln
	for d in /proc/[0-9]*; do
		pid="${d#/proc/}"
		case "$pid" in '' | *[!0-9]*) continue ;; esac
		read_cmdline "$pid" || CL=""
		case "$CL" in *"$target"*) return 0 ;; esac
		[ -d "$d/fd" ] || continue
		for fd in "$d"/fd/*; do
			ln="$(readlink "$fd" 2>/dev/null)" || continue
			case "$ln" in *"$target"*) return 0 ;; esac
		done
	done
	return 1
}

# 文本规范化：折叠重复 /、去掉尾部 /（不解析 symlink——目标可能已不存在）
normalize_path() {
	local p="$1"
	while case "$p" in *//*) true ;; *) false ;; esac; do
		p="${p//\/\//\/}"
	done
	while [ "${#p}" -gt 1 ] && [ "${p%/}" != "$p" ]; do
		p="${p%/}"
	done
	printf '%s' "$p"
}

# 删除 profile：规范化 + 「不含 .. ∧ 以 prefix 开头」+ 无引用才动手（S2）
remove_profile() {
	local target
	target="$(normalize_path "$1")"
	[ -n "$target" ] || return 1
	[ "$target" = "/" ] && return 1
	case "$target" in *..*) return 1 ;; esac
	case "$target" in "$PROFILE_PREFIX"?*) ;; *) return 1 ;; esac
	[ -d "$target" ] || return 0
	path_referenced "$target" && return 2
	rm -rf -- "$target" 2>/dev/null
	[ -e "$target" ] && return 1
	return 0
}

# 尽力清 $XDG_RUNTIME_DIR/agent-browser/<session>.* 残留 socket/pid（S6）
# 返回 0 = 已清理或本来就没有；1 = 有文件清不掉（只记 runtime_kept，不算回收失败）
# 注意：不能直接 glob "$dir/$SESSION".* —— session 名来自 environ，含 * 会越界删；
# 先展开目录再用 case 的引号模式做字面前缀匹配。
clean_runtime_state() {
	local dir="$RUNTIME_BASE/agent-browser" f base kept=0
	[ -d "$dir" ] || return 0
	for f in "$dir"/*; do
		[ -e "$f" ] || continue
		base="${f##*/}"
		case "$base" in "$SESSION".*) ;; *) continue ;; esac
		if path_referenced "$f"; then
			kept=1
			continue
		fi
		rm -rf -- "$f" 2>/dev/null
		[ -e "$f" ] && kept=1
	done
	[ "$kept" -eq 0 ] && return 0
	return 1
}

# 回收一个目标。返回 0=成功 1=失败；ESCALATION ∈ close|close-gone|term|kill|none
reap_stack() {
	ESCALATION="none"
	local rc
	if run_close "$SESSION"; then
		if wait_gone 5; then
			ESCALATION="close"
			return 0
		fi
		say "stack session=$SESSION daemon=$DAEMON_PID close 返回 0 但宽限 5s 后仍存活 → 降级信号回收"
	else
		rc=$?
		# S3：close 返回非 0 时先复核栈是不是其实已经自退了（例如 close 自身报了错但浏览器
		# 进程已没了）——已退就不发任何信号，记 close-gone 直接成功。
		if ! stack_alive; then
			ESCALATION="close-gone"
			return 0
		fi
		say "stack session=$SESSION daemon=$DAEMON_PID close 失败(rc=$rc) → 降级信号回收"
	fi
	kill_groups TERM
	if wait_gone 2; then
		ESCALATION="term"
		return 0
	fi
	kill_groups KILL
	if wait_gone 2; then
		ESCALATION="kill"
		return 0
	fi
	return 1
}

# ---------------------------------------------------------------------------
# JSON 账本
# ---------------------------------------------------------------------------
json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

json_int_array() { # $@ = 整数（pid / pgid）
	local out="" v
	for v in "$@"; do
		[ -n "$out" ] && out+=","
		out+="$v"
	done
	printf '[%s]' "$out"
}

push_json() { # $1=outcome $2=reason $3=profile_removed(0/1/-1=未涉及)
	local reason_json="null"
	[ -n "$2" ] && reason_json="\"$(json_escape "$2")\""
	local removed="null"
	case "$3" in
	0) removed="false" ;;
	1) removed="true" ;;
	esac
	# S11：未采样/未确认冻结时输出 null 而不是 0——0 会被读成「采样过且为零」。
	# 只有处置类账本（would-reap / reaped / failed）带 ticks；跳过类
	# （young / busy / first-seen / client-attached / self-protected / no-chromium）一律 null。
	local cb="null" ca="null"
	case "$1" in
	would-reap | reaped | failed)
		if [ "$CPU_SAMPLED" -eq 1 ]; then
			cb="$CPU_BEFORE"
			ca="$CPU_AFTER"
		fi
		;;
	esac
	local rt="null"
	case "${RUNTIME_KEPT:-}" in
	0) rt="false" ;;
	1) rt="true" ;;
	esac
	JSON_ITEMS+=("$(printf '{"session":"%s","daemon_pid":%s,"age_s":%s,"procs":%s,"pids":%s,"pgids":%s,"rss_mb":%s,"cpu_ticks_before":%s,"cpu_ticks_after":%s,"outcome":"%s","reason":%s,"escalation":%s,"clients":%s,"profile":"%s","profile_removed":%s,"runtime_kept":%s}' \
		"$(json_escape "$SESSION")" "${DAEMON_PID:-0}" "$AGE_S" "${#STACK_PIDS[@]}" \
		"$(json_int_array "${STACK_PIDS[@]}")" "$(json_int_array "${STACK_PGIDS[@]}")" \
		"$RSS_MB" "$cb" "$ca" "$1" "$reason_json" \
		"$([ -n "$ESCALATION" ] && printf '"%s"' "$ESCALATION" || printf 'null')" \
		"$(json_int_array "${CLIENTS[@]}")" "$(json_escape "$PROFILE")" "$removed" "$rt")")
}

emit_json() {
	[ "$JSON" -eq 1 ] || return 0
	# --quiet 下：无「泄漏账本」（would-reap/reaped/failed）也无 no-chromium 异常 → 零输出
	# 有泄漏账本时 --quiet MUST NOT 抑制 JSON（S5）
	if [ "$QUIET" -eq 1 ] && [ "$N_WOULD" -eq 0 ] && [ "$N_REAPED" -eq 0 ] && [ "$N_FAILED" -eq 0 ] && [ "$N_NOC" -eq 0 ]; then
		return 0
	fi
	local items="" it
	for it in "${JSON_ITEMS[@]}"; do
		[ -n "$items" ] && items+=","
		items+="$it"
	done
	printf '{"mode":"%s","generated_at":%s,"stacks":[%s],"summary":{"stacks":%s,"would_reap":%s,"reaped":%s,"failed":%s,"skipped":%s}}\n' \
		"$MODE" "$NOW" "$items" "$N_STACKS" "$N_WOULD" "$N_REAPED" "$N_FAILED" "$N_SKIPPED"
}

# ---------------------------------------------------------------------------
# 单个目标的完整处置（整段 fail-open：不 exit、不 set -e，任何异常只记 failed）
# ---------------------------------------------------------------------------
handle_stack() { # $1 = daemon pid
	DAEMON_PID="$1"
	STACK_PIDS=() STACK_PGIDS=() NOGROUP_PGIDS=() STACK_SET=() PGID_SET=()
	STACK_START=() PGID_LEADER_START=()
	PROFILE=""
	AGE_S=0 CPU_BEFORE=0 CPU_AFTER=0 CPU_SAMPLED=0 RSS_MB=0 ESCALATION="" CLIENTS=() RUNTIME_KEPT=""
	SESSION="${P_SESS[$DAEMON_PID]:-}"
	[ -n "$SESSION" ] || SESSION="default"
	SEEN_SESSIONS[$SESSION]=1 # 供 state_flush 清理「本轮已不存在」的条目
	local start_s=$((BTIME + ${P_START[$DAEMON_PID]:-0} / CLK_TCK))
	AGE_S=$((NOW - start_s))
	[ "$AGE_S" -ge 0 ] || AGE_S=0

	if ! build_stack "$DAEMON_PID"; then
		# 枚举不全（没有 chromium 锚点）→ 不动手：宁可漏一轮，也不杀枚举不全的栈
		STACK_PIDS=("$DAEMON_PID")
		N_SKIPPED=$((N_SKIPPED + 1))
		N_NOC=$((N_NOC + 1))
		say_always "skip session=$SESSION daemon=$DAEMON_PID age=${AGE_S}s reason=no-chromium（未找到 --user-data-dir=$PROFILE_PREFIX 的 chromium 根，不动手）"
		push_json skipped "no-chromium" -1
		return 0
	fi

	stack_rss_mb "${STACK_PIDS[@]}"

	local nogroup_str=""
	if [ "${#NOGROUP_PGIDS[@]}" -gt 0 ]; then
		local ng="${NOGROUP_PGIDS[*]}"
		nogroup_str=" nogroup=${ng// /,}"
	fi

	# 条件⑤ self-protected（最便宜，先判）
	local dpg="${P_PGID[$DAEMON_PID]:-0}" blocked=""
	if [ "$dpg" = "1" ] || [ "$dpg" = "$OUR_PGID" ]; then blocked="$dpg"; fi
	[ "$DAEMON_PID" = "$$" ] && blocked="$$"
	local pg
	for pg in "${STACK_PGIDS[@]}"; do
		if [ "$pg" = "1" ] || [ "$pg" = "$OUR_PGID" ]; then blocked="$pg"; fi
	done

	local verdict="" reason=""
	if [ -n "$blocked" ]; then
		reason="self-protected"
		verdict="skip"
	# 条件① young
	elif [ "$AGE_S" -lt "$MIN_AGE" ]; then
		reason="young"
		verdict="skip"
	# 条件④ client-attached（用即时快照，见 scan_clients_fresh）
	elif scan_clients_fresh; then
		reason="client-attached"
		verdict="skip"
	else
		# 条件② busy（最贵：要真等采样间隔）
		if cpu_frozen "${STACK_PIDS[@]}"; then
			# 条件③ 跨轮次冻结证据（B2）
			if cross_round_frozen; then
				verdict="leak"
			else
				reason="first-seen"
				verdict="skip"
			fi
		else
			reason="busy"
			verdict="skip"
		fi
	fi

	local pgids_str="${STACK_PGIDS[*]}"
	pgids_str="${pgids_str// /,}"
	local client_str=""
	[ "${#CLIENTS[@]}" -gt 0 ] && client_str=" client=${CLIENTS[*]}"

	if [ "$verdict" = "skip" ]; then
		N_SKIPPED=$((N_SKIPPED + 1))
		say "skip session=$SESSION daemon=$DAEMON_PID age=${AGE_S}s procs=${#STACK_PIDS[@]} rss=${RSS_MB}MB pgids=$pgids_str$nogroup_str profile=$PROFILE reason=$reason$client_str"
		push_json skipped "$reason" -1
		return 0
	fi

	# ---- 到这里五条件全满足：报告模式只记账，apply 模式真动手 ----
	if [ "$APPLY" -eq 0 ]; then
		N_WOULD=$((N_WOULD + 1))
		# 发现可回收泄漏：--quiet 也必须出声（S5）
		say_always "would-reap session=$SESSION daemon=$DAEMON_PID age=${AGE_S}s procs=${#STACK_PIDS[@]} rss=${RSS_MB}MB pgids=$pgids_str$nogroup_str cpu=$CPU_BEFORE..$CPU_AFTER ticks profile=$PROFILE"
		push_json would-reap "" -1
		return 0
	fi

	# B1：动手前用**即时快照**重算条件④（开头那次快照已经过去 sleep 1s+，会过期）
	if scan_clients_fresh; then
		N_SKIPPED=$((N_SKIPPED + 1))
		say "skip session=$SESSION daemon=$DAEMON_PID age=${AGE_S}s procs=${#STACK_PIDS[@]} pgids=$pgids_str$nogroup_str profile=$PROFILE reason=client-attached（动手前复扫发现客户端） client=${CLIENTS[*]}"
		push_json skipped "client-attached" -1
		return 0
	fi

	local removed=-1
	if reap_stack; then
		remove_profile "$PROFILE"
		case $? in
		0) removed=1 ;;
		*) removed=0 ;;
		esac
		# S6：尽力清 runtime 残留（清不掉只记 runtime_kept，不算失败）
		RUNTIME_KEPT=0
		clean_runtime_state || RUNTIME_KEPT=1
		# 栈已回收 → 该 session 不再存在，状态条目立刻丢掉（不留到下一轮才清）
		if [ "$STATE_OK" -eq 1 ] && [ -n "${STATE_START[$SESSION]:-}" ]; then
			unset 'STATE_START[$SESSION]' 'STATE_TICKS[$SESSION]' 'STATE_EPOCH[$SESSION]'
			STATE_DIRTY=1
		fi
		N_REAPED=$((N_REAPED + 1))
		if [ "$removed" -eq 1 ]; then
			say_always "reaped session=$SESSION daemon=$DAEMON_PID procs=${#STACK_PIDS[@]} rss=${RSS_MB}MB pgids=$pgids_str$nogroup_str escalation=$ESCALATION profile=$PROFILE removed=yes runtime=$([ "$RUNTIME_KEPT" -eq 1 ] && printf kept || printf cleaned)"
		else
			say_always "reaped session=$SESSION daemon=$DAEMON_PID procs=${#STACK_PIDS[@]} rss=${RSS_MB}MB pgids=$pgids_str$nogroup_str escalation=$ESCALATION profile=$PROFILE removed=kept(仍被引用或不在前缀下) runtime=$([ "$RUNTIME_KEPT" -eq 1 ] && printf kept || printf cleaned)"
		fi
		push_json reaped "" "$removed"
		return 0
	fi

	N_FAILED=$((N_FAILED + 1))
	say_always "failed session=$SESSION daemon=$DAEMON_PID procs=${#STACK_PIDS[@]} pgids=$pgids_str$nogroup_str escalation=$ESCALATION reason=still-alive profile=$PROFILE"
	push_json failed "still-alive" -1
	return 0
}

# ---------------------------------------------------------------------------
# 主巡检
# ---------------------------------------------------------------------------
guard_all() {
	resolve_close_cmd
	if [ "${#CLOSE_ARGV[@]}" -eq 0 ]; then
		say "warn: 找不到 agent-browser（PATH 与 ~/.nvm/versions/node/*/bin 均无），close 步骤将直接降级为信号回收"
	fi
	snapshot
	local daemons
	daemons="$(find_daemons)"
	local pid
	if [ -z "$daemons" ]; then
		say "no-agent-browser-daemon（pattern=$DAEMON_PATTERN）"
		N_STACKS=0
	else
		while IFS= read -r pid; do
			[ -n "$pid" ] || continue
			N_STACKS=$((N_STACKS + 1))
			handle_stack "$pid"
		done <<<"$daemons"
	fi

	# 汇总：无泄漏时 --quiet 静音；有泄漏/失败/no-chromium 时出声（S5）
	if [ "$QUIET" -eq 1 ] && [ "$N_WOULD" -eq 0 ] && [ "$N_REAPED" -eq 0 ] && [ "$N_FAILED" -eq 0 ] && [ "$N_NOC" -eq 0 ]; then
		:
	else
		say_always "summary mode=$MODE stacks=$N_STACKS would_reap=$N_WOULD reaped=$N_REAPED failed=$N_FAILED skipped=$N_SKIPPED"
	fi
	emit_json

	if [ "$N_FAILED" -gt 0 ]; then
		return 1
	fi
	if [ "$APPLY" -eq 0 ] && [ "$N_WOULD" -gt 0 ]; then
		return 1
	fi
	return 0
}

# ---------------------------------------------------------------------------
# 安装 / 卸载（幂等；--install 只由用户手动跑，unit 的 ExecStart 不带它）
# ---------------------------------------------------------------------------
need_systemd() {
	[ -r /proc/self/stat ] || die2 "--install/--uninstall 只支持 Linux"
	command -v systemctl >/dev/null 2>&1 || die2 "找不到 systemctl，无法管理用户级 timer"
}

do_install() {
	need_systemd
	local dst="$HOME/.config/systemd/user" f
	for f in "$UNIT_SERVICE" "$UNIT_TIMER"; do
		[ -f "$UNIT_DIR/$f" ] || die2 "缺少 unit 文件: $UNIT_DIR/$f"
	done
	mkdir -p "$dst" || die2 "无法创建 $dst"
	cp -f "$UNIT_DIR/$UNIT_SERVICE" "$dst/$UNIT_SERVICE" || die2 "复制 $UNIT_SERVICE 失败"
	cp -f "$UNIT_DIR/$UNIT_TIMER" "$dst/$UNIT_TIMER" || die2 "复制 $UNIT_TIMER 失败"
	systemctl --user daemon-reload || die2 "systemctl --user daemon-reload 失败"
	systemctl --user enable --now "$UNIT_TIMER" || die2 "enable --now $UNIT_TIMER 失败"
	printf '[browser-leak-guard] installed: %s -> %s\n' "$UNIT_TIMER" "$dst"
	systemctl --user list-timers "$UNIT_TIMER" --no-pager 2>/dev/null | head -3 || true
	printf '[browser-leak-guard] 查日志: journalctl --user -u %s -n 50\n' "$UNIT_SERVICE"
}

do_uninstall() {
	need_systemd
	local dst="$HOME/.config/systemd/user" f
	systemctl --user disable --now "$UNIT_TIMER" >/dev/null 2>&1 || true
	systemctl --user stop "$UNIT_SERVICE" >/dev/null 2>&1 || true
	for f in "$UNIT_SERVICE" "$UNIT_TIMER"; do
		rm -f "$dst/$f"
	done
	systemctl --user daemon-reload >/dev/null 2>&1 || true
	printf '[browser-leak-guard] uninstalled: %s / %s\n' "$UNIT_SERVICE" "$UNIT_TIMER"
}

# ---------------------------------------------------------------------------
main() {
	parse_args "$@"
	env_check
	validate_injectables
	case "$MODE" in
	install)
		do_install
		exit $?
		;;
	uninstall)
		do_uninstall
		exit $?
		;;
	esac
	state_init
	if ! acquire_lock; then
		# 单实例（S7）：定时器与人工并发时后来者静默退出，不判同一栈、不双关同会话
		exit 0
	fi
	state_load
	guard_all
	local rc=$?
	state_flush
	exit "$rc"
}

main "$@"
