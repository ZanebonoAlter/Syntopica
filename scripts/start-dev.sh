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
#   bash scripts/start-dev.sh stop           # 停掉两者
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
for arg in "$@"; do
	case "$arg" in
		--restart) RESTART=1 ;;
		-h | --help)
			sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		all | back | backend | front | web | stop | status) TARGET="$arg" ;;
		*)
			echo "未知参数：$arg（见 --help）" >&2
			exit 2
			;;
	esac
done
[ "$TARGET" = "backend" ] && TARGET=back
[ "$TARGET" = "web" ] && TARGET=front

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

stop_port() { # stop_port <port> <名字>
	local port="$1" name="$2" pids
	pids="$(port_pids "$port")"
	if [ -z "$pids" ]; then
		echo "  $name（:$port）本来就没在跑"
		return 0
	fi
	echo "  停 $name（:$port）PID: $pids"
	# 只杀监听进程：它的父进程（pnpm / sh -c）会在子进程退出后自行结束
	# shellcheck disable=SC2086
	kill $pids 2>/dev/null
	local i
	for ((i = 1; i <= 10; i++)); do
		is_up "$port" || return 0
		sleep 1
	done
	echo "  警告：:$port 仍在监听，PID: $(port_pids "$port")（可手动 kill -9）" >&2
	return 1
}

start_back() {
	if is_up "$BACK_PORT"; then
		echo "  后端（:$BACK_PORT）已在跑，跳过（PID: $(port_pids "$BACK_PORT")）"
		return 0
	fi
	echo "  起后端（:$BACK_PORT）→ backend-go/nohup.out"
	(
		cd "$REPO_ROOT/backend-go" || exit 1
		CORS_ORIGINS="$CORS_ORIGINS" nohup setsid go run cmd/server/main.go >> nohup.out 2>&1 < /dev/null &
	)
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
	(
		cd "$REPO_ROOT/front" || exit 1
		NUXT_PUBLIC_API_BASE="$NUXT_PUBLIC_API_BASE" nohup setsid pnpm dev --host >> nohup.out 2>&1 < /dev/null &
	)
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
	for pair in "${BACK_PORT}:后端" "${FRONT_PORT}:前端"; do
		local port="${pair%%:*}" name="${pair##*:}"
		if is_up "$port"; then
			echo "  $name（:$port）：LISTEN，PID $(port_pids "$port")"
		else
			echo "  $name（:$port）：未监听"
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
		stop_port "$FRONT_PORT" 前端
		stop_port "$BACK_PORT" 后端
		exit 0
		;;
	status)
		show_status
		exit 0
		;;
esac

rc=0
if [ "$RESTART" = "1" ]; then
	echo "先停旧进程："
	[ "$TARGET" = "all" ] || [ "$TARGET" = "front" ] && stop_port "$FRONT_PORT" 前端
	[ "$TARGET" = "all" ] || [ "$TARGET" = "back" ] && stop_port "$BACK_PORT" 后端
fi

[ "$TARGET" = "all" ] || [ "$TARGET" = "back" ] && { start_back || rc=1; }
[ "$TARGET" = "all" ] || [ "$TARGET" = "front" ] && { start_front || rc=1; }

echo
echo "入口：$ENTRY_LIST"
[ "$MODE" = "same-origin" ] && echo "（同源模式：CORS 与 apiBase 都不参与，直接开上面任一个地址；改前端代码有 HMR）"
exit "$rc"
