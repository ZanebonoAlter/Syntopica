#!/usr/bin/env bash
# Syntopica dev 栈启动脚本 —— 把「容易漏、漏了就全站不可访问」的环境变量固化在一处。
#
# 为什么需要它：`NUXT_PUBLIC_API_BASE` 与 `CORS_ORIGINS` 都是**非持久化**的进程环境变量，
#   - 同源反代部署（deploy/same-origin/nginx.conf）下前端必须带 `NUXT_PUBLIC_API_BASE=/api`，
#     否则页面在 `:80` 上、API 却打 `:5100`，被后端 CORS 白名单（精确匹配、无通配）拦掉，
#     症状是「页面能开、列表全空」；
#   - 直连形态下两个变量必须配对，少一个换一种报错。
#   2026-09-16 实际发生过一次：后端重启漏了 `export CORS_ORIGINS`，整个前端无法访问。
#
# 用法：
#   bash scripts/start-dev.sh                # 起后端 + 前端（已在跑的不动）
#   bash scripts/start-dev.sh front          # 只起前端（或 back 只起后端）
#   bash scripts/start-dev.sh --restart      # 先停再起（等同于 stop + 起）
#   bash scripts/start-dev.sh stop           # 停掉两者（stop back / stop front 只停一个）
#   bash scripts/start-dev.sh status         # 看端口/PID/健康/入口地址
#   bash scripts/start-dev.sh --help
#
# 模式自动判定（可用环境变量覆盖）：
#   装了同源入口（/etc/nginx/conf.d/syntopica.conf 存在）→ 前端 apiBase=「/api」，入口 http://<ip>/
#   否则                                                → 前端 apiBase=http://<ip>:5100/api，入口 http://<ip>:3000
#
# 覆盖示例：
#   NUXT_PUBLIC_API_BASE=/api CORS_ORIGINS=http://192.168.1.9:3000 bash scripts/start-dev.sh --restart front
#
# 日志：后端 → backend-go/nohup.out，前端 → front/nohup.out（`tail -f` 看即可）
# 需要：Linux / macOS / WSL bash（Windows 原生按 docs/reference/development.md 的三步手动起）

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACK_PORT=5100
FRONT_PORT=3000
NGINX_CONF=/etc/nginx/conf.d/syntopica.conf

# ── 参数解析 ────────────────────────────────────────────────────────────────
RESTART=0
TARGET="all"
STOP_WHAT="all" # 「stop back / stop front」的可选第二参数：只停指定端
for arg in "$@"; do
	case "$arg" in
		--restart) RESTART=1 ;;
		-h | --help)
			sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		stop) TARGET="stop" ;;
		all | back | backend | front | web | status | selftest)
			if [ "$TARGET" = "stop" ]; then
				# stop 之后再跟 back/front：只停指定端；其余一律拒绝，防「stop all」静默变起服务
				case "$arg" in
					back | backend | front | web) STOP_WHAT="$arg" ;;
					*)
						echo "stop 后只能跟 back / front（收到：$arg）" >&2
						exit 2
						;;
				esac
			else
				TARGET="$arg"
			fi
			;;
		*)
			echo "未知参数：$arg（见 --help）" >&2
			exit 2
			;;
	esac
done
[ "$TARGET" = "backend" ] && TARGET=back
[ "$TARGET" = "web" ] && TARGET=front
[ "$STOP_WHAT" = "backend" ] && STOP_WHAT=back
[ "$STOP_WHAT" = "web" ] && STOP_WHAT=front

# ── 环境与模式 ──────────────────────────────────────────────────────────────
# 这台机器可能有多个网段（LAN / ZeroTier / Tailscale / docker 网桥），所以：
#   - apiBase 默认取第一个非环回 IPv4（可用环境变量覆盖）；
#   - CORS 白名单把**所有**非 docker 网段 IPv4 都列上，避免「浏览器用哪个 IP 访问」
#     跟脚本猜的那个不一致就又被拦（docker 网桥 172.16-31.x 对浏览器没意义，滤掉）。
ALL_IPS="$(hostname -I 2>/dev/null | tr ' ' '\n' \
	| grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' \
	| grep -vE '^(127\.|172\.(1[6-9]|2[0-9]|3[01])\.)' || true)"
IP="$(printf '%s\n' "$ALL_IPS" | head -1)"
IP="${IP:-127.0.0.1}"

CORS_DEFAULT="http://localhost:${FRONT_PORT},http://127.0.0.1:${FRONT_PORT}"
if [ -n "$ALL_IPS" ]; then
	while read -r one; do
		[ -n "$one" ] && CORS_DEFAULT="${CORS_DEFAULT},http://${one}:${FRONT_PORT}"
	done <<<"$ALL_IPS"
fi

if [ -f "$NGINX_CONF" ]; then
	MODE="same-origin"
	DEFAULT_API_BASE="/api"
	ENTRY_LIST=""
	while read -r one; do
		[ -n "$one" ] && ENTRY_LIST="${ENTRY_LIST}http://${one}/  "
	done <<<"${ALL_IPS:-$IP}"
else
	MODE="direct"
	DEFAULT_API_BASE="http://${IP}:${BACK_PORT}/api"
	ENTRY_LIST="http://${IP}:${FRONT_PORT}/  "
fi
export NUXT_PUBLIC_API_BASE="${NUXT_PUBLIC_API_BASE:-$DEFAULT_API_BASE}"
# 同源模式下后端 CORS 不参与，但保留一份白名单，便于有人直连 :3000 时也能用
export CORS_ORIGINS="${CORS_ORIGINS:-$CORS_DEFAULT}"

# ── 工具函数 ────────────────────────────────────────────────────────────────
port_pids() { lsof -ti "tcp:$1" -sTCP:LISTEN 2>/dev/null | tr '\n' ' '; }
is_up() { [ -n "$(port_pids "$1")" ]; }

wait_http() { # wait_http <url> <尝试次数>
	local url="$1" tries="${2:-30}" i
	for ((i = 1; i <= tries; i++)); do
		if curl -sf -o /dev/null --max-time 3 "$url" 2>/dev/null; then return 0; fi
		sleep 2
	done
	return 1
}

pidfile_path() { printf '%s/.pi/run/%s.pgid' "$REPO_ROOT" "$1"; }

group_alive() { # 进程组存活探测（PGID≤1 一律判无效，防 kill -- -1 误伤全系统）
	[ "$1" -gt 1 ] 2>/dev/null && kill -0 -- "-$1" 2>/dev/null
}

spawn_detached() { # spawn_detached <pidfile名> <workdir> <cmd...>
	# 在 workdir 下 nohup setsid 脱离终端起服务，把会话首进程 PID（= PGID）写入
	# .pi/run/<名>.pgid。该 pidfile 是 dev-process-guard 扩展的白名单：经本脚本
	# 托管的服务永不判泄漏。env（CORS_ORIGINS / NUXT_PUBLIC_API_BASE）已在脚本
	# 头部 export，子进程天然继承，与旧的内联前缀写法语义等价。
	local name="$1" workdir="$2" pid
	shift 2
	mkdir -p "$REPO_ROOT/.pi/run"
	pid="$(
		cd "$workdir" || exit 1
		nohup setsid "$@" >> nohup.out 2>&1 < /dev/null &
		echo "$!"
	)"
	echo "$pid" > "$(pidfile_path "$name")"
}

stop_port() { # stop_port <port> <名字> <pidfile名>
	# 清理目标 = 端口监听 PID（lsof，外部手起的服务）∪ pidfile 记录的进程组（本脚本托管）；
	# 端口已释放但 pidfile 组仍存活（僵尸栈）也必须清。完成后删除 pidfile。
	local port="$1" name="$2" pname="$3"
	local pids pgid_file pgid i
	pids="$(port_pids "$port")"
	pgid_file="$(pidfile_path "$pname")"
	pgid=""
	if [ -f "$pgid_file" ]; then
		pgid="$(tr -d '[:space:]' <"$pgid_file")"
		# 内容非数字 / PGID≤1 / 指向已死进程组 → 无主 pidfile，直接删除不报错
		if ! [[ "$pgid" =~ ^[0-9]+$ ]] || ! group_alive "$pgid"; then
			echo "  $name pidfile 无主（损坏或组已死），已清理：$pgid_file"
			rm -f "$pgid_file"
			pgid=""
		fi
	fi
	if [ -z "$pids" ] && [ -z "$pgid" ]; then
		echo "  $name（:$port）本来就没在跑"
		return 0
	fi
	if [ -n "$pids" ]; then
		echo "  停 $name（:$port）PID: $pids"
		# 只杀监听进程：它的父进程（pnpm / sh -c）会在子进程退出后自行结束
		# shellcheck disable=SC2086
		kill $pids 2>/dev/null
	fi
	if [ -n "$pgid" ]; then
		# pidfile 进程组整组 TERM（go run 父子 / pnpm→sh→nuxt 链共享 PGID，单杀必留孤儿）
		echo "  停 $name pidfile 进程组（PGID: $pgid）"
		kill -- "-$pgid" 2>/dev/null
	fi
	for ((i = 1; i <= 10; i++)); do
		if [ -z "$(port_pids "$port")" ] && { [ -z "$pgid" ] || ! group_alive "$pgid"; }; then
			break
		fi
		sleep 1
	done
	# TERM 10s 仍存活的托管组升级 KILL 兜底
	if [ -n "$pgid" ] && group_alive "$pgid"; then
		echo "  警告：$name 进程组（PGID $pgid）10s 未退出，kill -9 兜底" >&2
		kill -9 -- "-$pgid" 2>/dev/null
	fi
	# 端口 PID 同样升级：后端优雅关停偏慢（Registry.StopAll 上限 30s+，端口要等
	# os.Exit(0) 才释放），10s 观察窗内大概率未让端口——kill -9 兜底保证 restart
	# 确定性（dev 栈无状态可丢，不依赖优雅收尾）
	if is_up "$port"; then
		local stubborn
		stubborn="$(port_pids "$port")"
		echo "  警告：$name（:$port）TERM 后仍在监听（多为优雅关停偏慢），kill -9 兜底：$stubborn" >&2
		# shellcheck disable=SC2086
		kill -9 $stubborn 2>/dev/null
		sleep 1
	fi
	rm -f "$pgid_file"
	if is_up "$port"; then
		echo "  警告：:$port 仍在监听，PID: $(port_pids "$port")（可手动 kill -9）" >&2
		return 1
	fi
	return 0
}

start_back() {
	if is_up "$BACK_PORT"; then
		echo "  后端（:$BACK_PORT）已在跑，跳过（PID: $(port_pids "$BACK_PORT")）"
		return 0
	fi
	echo "  起后端（:$BACK_PORT）→ backend-go/nohup.out"
	spawn_detached backend "$REPO_ROOT/backend-go" go run cmd/server/main.go
	if wait_http "http://127.0.0.1:${BACK_PORT}/health" 30; then
		echo "  ✓ 后端就绪（/health 200，PID: $(port_pids "$BACK_PORT")）"
	else
		echo "  ✗ 后端 60s 内没起来，看 backend-go/nohup.out" >&2
		return 1
	fi
}

start_front() {
	if is_up "$FRONT_PORT"; then
		echo "  前端（:$FRONT_PORT）已在跑，跳过（PID: $(port_pids "$FRONT_PORT")）"
		echo "    注意：已在跑的那个进程用的是它启动时的 apiBase；要换 base 请加 --restart"
		return 0
	fi
	echo "  起前端（:$FRONT_PORT，apiBase=$NUXT_PUBLIC_API_BASE）→ front/nohup.out"
	spawn_detached front "$REPO_ROOT/front" pnpm dev --host
	# Nuxt dev 冷启动在树莓派上可达 60~90s
	if wait_http "http://127.0.0.1:${FRONT_PORT}/" 60; then
		echo "  ✓ 前端就绪（PID: $(port_pids "$FRONT_PORT")）"
	else
		echo "  ✗ 前端 120s 内没起来，看 front/nohup.out" >&2
		return 1
	fi
}

show_status() {
	echo "模式：$MODE（同源入口 nginx ${NGINX_CONF} $([ -f "$NGINX_CONF" ] && echo 存在 || echo 不存在)）"
	for trio in "${BACK_PORT}:后端:backend" "${FRONT_PORT}:前端:front"; do
		local port name pname pgid_file pgid
		IFS=: read -r port name pname <<<"$trio"
		if is_up "$port"; then
			echo "  $name（:$port）：LISTEN，PID $(port_pids "$port")"
		else
			echo "  $name（:$port）：未监听"
		fi
		# pidfile 托管状态（只读不删）
		pgid_file="$(pidfile_path "$pname")"
		if [ -f "$pgid_file" ]; then
			pgid="$(tr -d '[:space:]' <"$pgid_file")"
			if [[ "$pgid" =~ ^[0-9]+$ ]] && group_alive "$pgid"; then
				echo "    pidfile 接管：PGID $pgid（组存活）"
			else
				echo "    pidfile 过期（组已死），已可清理"
			fi
		else
			echo "    无 pidfile（非 start-dev.sh 托管或未起）"
		fi
	done
	if is_up "$BACK_PORT"; then
		printf "  /health → "; curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "http://127.0.0.1:${BACK_PORT}/health" || echo "不可达"
		printf "  feed 图标 → "
		curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "http://127.0.0.1:${BACK_PORT}/icons/feeds/1.ico" || echo "不可达"
	fi
	echo "  前端 apiBase（本脚本会注入）：$NUXT_PUBLIC_API_BASE"
	echo "  后端 CORS 白名单（本脚本会注入）：$CORS_ORIGINS"
	echo "  入口：$ENTRY_LIST"
	if [ "$MODE" = "same-origin" ] && is_up "$FRONT_PORT" && ! curl -sf -o /dev/null --max-time 5 "http://127.0.0.1/"; then
		echo "  ⚠ 同源入口不可达：检查 nginx（systemctl status nginx / nginx -t）" >&2
	fi
}

# ── 主流程 ──────────────────────────────────────────────────────────────────
echo "Syntopica dev 栈（模式：$MODE）"

case "$TARGET" in
	stop)
		if [ "$STOP_WHAT" = "all" ] || [ "$STOP_WHAT" = "front" ]; then
			stop_port "$FRONT_PORT" 前端 front
		fi
		if [ "$STOP_WHAT" = "all" ] || [ "$STOP_WHAT" = "back" ]; then
			stop_port "$BACK_PORT" 后端 backend
		fi
		exit 0
		;;
	status)
		show_status
		exit 0
		;;
	selftest)
		# 隐藏目标：只验证 spawn_detached + pidfile 代码路径（冒烟脚本专用，不碰真实端口）
		spawn_detached selftest "$REPO_ROOT" sleep 300
		echo "selftest OK：$(pidfile_path selftest) 已写，PGID $(cat "$(pidfile_path selftest)")（sleep 300，由调用方清理）"
		exit 0
		;;
esac

rc=0
if [ "$RESTART" = "1" ]; then
	echo "先停旧进程："
	[ "$TARGET" = "all" ] || [ "$TARGET" = "front" ] && stop_port "$FRONT_PORT" 前端 front
	[ "$TARGET" = "all" ] || [ "$TARGET" = "back" ] && stop_port "$BACK_PORT" 后端 backend
fi

[ "$TARGET" = "all" ] || [ "$TARGET" = "back" ] && { start_back || rc=1; }
[ "$TARGET" = "all" ] || [ "$TARGET" = "front" ] && { start_front || rc=1; }

echo
echo "入口：$ENTRY_LIST"
[ "$MODE" = "same-origin" ] && echo "（同源模式：CORS 与 apiBase 都不参与，直接开上面任一个地址；改前端代码有 HMR）"
exit "$rc"
