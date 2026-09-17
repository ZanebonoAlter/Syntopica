#!/usr/bin/env bash
# start-dev.sh pidfile 契约冒烟：复制脚本到临时目录并换成假端口（15901/15902），
# 在副本上验证 spawn_detached 写 pidfile / stop 组清 + 删文件 / 僵尸栈清理 /
# 损坏 pidfile 降级 / status 只读显示。全程绝不触碰真实 dev 栈（5100/3000）
# 与真实仓库的 .pi/run/。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="$REPO_ROOT/scripts/start-dev.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/start-dev-smoke.XXXXXX")"

# 现场兜底：杀掉本冒烟起过的所有假进程组（含意外路径），再删临时目录
FAKE_PGIDS=()
cleanup() {
	local g
	for g in "${FAKE_PGIDS[@]:-}"; do
		[ -n "$g" ] && kill -- "-$g" 2>/dev/null || true
	done
	rm -rf "$TMP"
}
trap cleanup EXIT

ok() { printf '  [OK] %s\n' "$1"; }
bad() { printf '  [FAIL] %s\n' "$1" >&2; exit 1; } # 单点失败即终止并指名断言

# 副本构建：scripts/ 放进 $TMP 后，脚本内 REPO_ROOT（= scripts/..）解析为 $TMP，
# pidfile 天然落在 $TMP/.pi/run/，与真实环境隔离。
mkdir -p "$TMP/scripts"
cp "$SOURCE" "$TMP/scripts/start-dev.sh"
sed -i 's/^BACK_PORT=5100$/BACK_PORT=15901/; s/^FRONT_PORT=3000$/FRONT_PORT=15902/' "$TMP/scripts/start-dev.sh"
grep -q '^BACK_PORT=15901$' "$TMP/scripts/start-dev.sh" || bad '前置：副本端口替换失败（BACK_PORT）'
grep -q '^FRONT_PORT=15902$' "$TMP/scripts/start-dev.sh" || bad '前置：副本端口替换失败（FRONT_PORT）'
RUN="$TMP/scripts/start-dev.sh"
PIDDIR="$TMP/.pi/run"

RUN_OUTPUT=""
run_copy() { # run_copy <zero|nonzero> <args...>：跑副本，断言退出码并保留输出到 RUN_OUTPUT
	local expect="$1" rc
	shift
	set +e
	RUN_OUTPUT="$(bash "$RUN" "$@" 2>&1)"
	rc=$?
	set -e
	if [ "$expect" = "zero" ] && [ "$rc" -ne 0 ]; then
		bad "副本应退出 0 实际 $rc（args: $*，输出：$RUN_OUTPUT）"
	elif [ "$expect" = "nonzero" ] && [ "$rc" -eq 0 ]; then
		bad "副本应退出非 0（args: $*，输出：$RUN_OUTPUT）"
	fi
}

port_listening() { [ -n "$(lsof -ti "tcp:$1" -sTCP:LISTEN 2>/dev/null)" ]; }
group_alive() { kill -0 -- "-$1" 2>/dev/null; }
group_dead() { ! kill -0 -- "-$1" 2>/dev/null; }

wait_group_dead() { # 等进程组消失，最多 ~5s
	local i
	for ((i = 1; i <= 10; i++)); do
		group_dead "$1" && return 0
		sleep 0.5
	done
	return 1
}

wait_port_listening() { # 等端口进入 LISTEN，最多 ~10s
	local i
	for ((i = 1; i <= 20; i++)); do
		port_listening "$1" && return 0
		sleep 0.5
	done
	return 1
}

# 前置：假端口必须空闲，避免误伤他人/被他人干扰
for p in 15901 15902; do
	port_listening "$p" && bad "前置：假端口 $p 已被占用，环境不干净"
done
command -v python3 >/dev/null 2>&1 || bad '前置：环境缺 python3（用例 B 需要它起假前端）'

# ── 用例 A：selftest 起 sleep 组并写 pidfile（验证 spawn_detached 代码路径） ──
echo '用例 A：selftest 写 pidfile，$! == PGID'
run_copy zero selftest
[ -f "$PIDDIR/selftest.pgid" ] || bad 'A1：selftest 未生成 .pi/run/selftest.pgid'
pid="$(tr -d '[:space:]' <"$PIDDIR/selftest.pgid")"
[[ "$pid" =~ ^[0-9]+$ ]] || bad "A2：pidfile 内容非数字（$pid）"
FAKE_PGIDS+=("$pid")
[ -n "$(ps -o pgid= -p "$pid" 2>/dev/null)" ] || bad "A3：PID $pid 不存在（进程没起来）"
pgid="$(ps -o pgid= -p "$pid" | tr -d '[:space:]')"
[ "$pgid" = "$pid" ] || bad "A4：pidfile 记录 $pid，实际 PGID 是 $pgid（$! ≠ PGID，契约破）"
kill -- "-$pid" 2>/dev/null || true
wait_group_dead "$pid" || bad 'A5：kill 组后 sleep 300 未退出'
ok '用例 A：spawn_detached 写 pidfile、PGID==PID、组可清理'

# ── 用例 B：stop 常规（端口监听 + pidfile 双清）与僵尸栈（端口已释放、组存活） ──
echo '用例 B1：stop 清端口 + 进程组 + pidfile'
setsid python3 -m http.server 15902 >/dev/null 2>&1 < /dev/null &
fake_front="$!"
FAKE_PGIDS+=("$fake_front")
wait_port_listening 15902 || bad 'B0：假前端（python3 http.server）没监听 15902'
echo "$fake_front" >"$PIDDIR/front.pgid"
run_copy zero stop
port_listening 15902 && bad 'B2：stop 后 15902 仍被监听'
wait_group_dead "$fake_front" || bad 'B3：stop 后假前端进程组仍存活'
[ ! -f "$PIDDIR/front.pgid" ] || bad 'B4：stop 后 front.pgid 未删除'
ok '用例 B1：stop 常规清理（端口释放 + 组死 + pidfile 删）'

echo '用例 B2：僵尸栈（端口未占、pidfile 组存活）也清'
setsid sleep 300 >/dev/null 2>&1 < /dev/null &
zombie="$!"
FAKE_PGIDS+=("$zombie")
echo "$zombie" >"$PIDDIR/front.pgid"
port_listening 15902 && bad 'B5 前置：僵尸栈场景 15902 不应被占用'
run_copy zero stop
wait_group_dead "$zombie" || bad 'B5：僵尸栈 stop 后进程组仍存活'
[ ! -f "$PIDDIR/front.pgid" ] || bad 'B6：僵尸栈 stop 后 pidfile 未删除'
ok '用例 B2：僵尸栈被组清 + pidfile 删 + stop 退出 0'

# ── 用例 C：损坏 pidfile 降级（无报错、文件被删、不触发 set -e 中断） ──
echo '用例 C：损坏 pidfile 视为无主'
echo '这-不-是-PID!!!' >"$PIDDIR/front.pgid"
run_copy zero stop
[ ! -f "$PIDDIR/front.pgid" ] || bad 'C1：损坏 pidfile 未被删除'
ok '用例 C：非数字 pidfile 降级删除，stop 退出 0'

# ── 用例 D：status 只读显示 pidfile 接管状态 ──
echo '用例 D：status 显示「pidfile 接管」'
setsid sleep 300 >/dev/null 2>&1 < /dev/null &
live="$!"
FAKE_PGIDS+=("$live")
echo "$live" >"$PIDDIR/front.pgid"
run_copy zero status
[[ "$RUN_OUTPUT" == *'pidfile 接管'* ]] || bad "D1：status 输出缺「pidfile 接管」行（输出：$RUN_OUTPUT）"
[[ "$RUN_OUTPUT" == *'无 pidfile'* ]] || bad "D2：status 输出缺「无 pidfile」行（后端应显示缺失态，输出：$RUN_OUTPUT）"
kill -- "-$live" 2>/dev/null || true
rm -f "$PIDDIR/front.pgid"
ok '用例 D：status 只读显示 pidfile 接管 / 无 pidfile'

# ── 用例 E：端口 PID 忽略 TERM（后端优雅关停偏慢的等价形态）→ stop 升级 KILL 释放端口 ──
echo '用例 E：TERM 免疫端口进程升级 kill -9'
setsid bash -c 'trap "" TERM; exec python3 -m http.server 15902' >/dev/null 2>&1 < /dev/null &
stubborn="$!"
FAKE_PGIDS+=("$stubborn")
wait_port_listening 15902 || bad 'E0 前置：TERM 免疫假前端没监听 15902'
run_copy zero stop
port_listening 15902 && bad 'E1：stop 后 15902 仍被监听（KILL 升级未生效）'
wait_group_dead "$stubborn" || bad 'E2：stop 后 TERM 免疫进程组仍存活'
ok '用例 E：TERM 免疫端口进程被 KILL 升级清理，端口释放'

echo
echo 'start-dev smoke：全部用例通过'
printf 'SMOKE OK\n'
