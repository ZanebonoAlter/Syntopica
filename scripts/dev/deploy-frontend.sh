#!/usr/bin/env bash
# 一键部署前端静态产物到后端同端口（树莓派日常形态，deployment.md §本地裸跑静态托管）。
#
# 用法：bash scripts/dev/deploy-frontend.sh [--no-restart]
#   构建静态产物（NUXT_PUBLIC_API_BASE=/api 走同源相对路径）→ 铺 backend-go/frontend/
#   → 重启后端（同端口 :5100 出 API + 页面）→ 健康检查。
#   --no-restart 只构建铺盘不重启（后端在跑时静态文件即时生效，无需重启的场景用）。
#
# 前端改动收尾顺手跑本脚本，别让用户打开页面发现还是旧版（2026-09-19 用户要求固化为习惯）。
#
# 构建限载（2026-11-24 固化，用户要求作为标准）：`pnpm generate` 在树莓派上与本机其它 pi 会话
# 叠加时曾把整机压到重启（当日 10:08 事故：构建中整机 reboot，构建产物未上线）。现在构建统一走
# `run_guarded`：nice 19（最低优先级）+ Node 堆上限 + 看门狗（MemAvailable<350MB 或 load1>30 才中止，
# 中止只杀构建进程组、不动后续步骤）。阈值取宽是因为 load average 滞后：构建结束瞬间仍会冲高，
# 按「已结束的构建误杀后续部署」比按「真危险不管」代价更高。
set -euo pipefail

cd "$(dirname "$0")/../.." # 仓库根

FRONT_DIR=front
BACKEND_STATIC=backend-go/frontend
BUILD_LOG=/tmp/syntopica-front-build.log
NO_RESTART=false
[[ "${1:-}" == "--no-restart" ]] && NO_RESTART=true

# 限载执行：后台跑 + 每 20s 采样 load / MemAvailable，危险阈值才中止（杀整个进程组，含 pnpm 子进程）。
run_guarded() {
  local log=$1
  shift
  setsid "$@" >"$log" 2>&1 &
  local pid=$! l m
  while kill -0 "$pid" 2>/dev/null; do
    l=$(cut -d' ' -f1 /proc/loadavg)
    m=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)
    if [ "${m:-9999}" -lt 350 ] || awk "BEGIN{exit !($l > 30)}"; then
      echo "⚠️ 构建中止：load1=${l} mem=${m}MB（阈值 load1>30 或 mem<350MB）" >&2
      kill -TERM -"$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
      echo "--- 构建日志尾部（${log}）---" >&2
      tail -20 "$log" >&2 || true
      return 1
    fi
    printf "   [build] %s load1=%s mem=%sMB\n" "$(date +%H:%M:%S)" "$l" "$m"
    sleep 20
  done
  wait "$pid"
}

echo "==> [1/3] 构建静态产物（NUXT_PUBLIC_API_BASE=/api，nice 19 + 看门狗限载）"
# 2026-10-19：树莓派上默认 2GB Node 堆在 nuxt build 后期 OOM（实测两次），抬到 3.5G 后稳定
export NODE_OPTIONS="--max-old-space-size=3584"
run_guarded "$BUILD_LOG" bash -c 'cd "$1" && NUXT_PUBLIC_API_BASE=/api nice -n 19 pnpm generate' _ "$FRONT_DIR"

echo "==> [2/3] 铺到 ${BACKEND_STATIC}/"
rm -rf "$BACKEND_STATIC"
cp -r "$FRONT_DIR/.output/public" "$BACKEND_STATIC"

if $NO_RESTART; then
  echo "==> 跳过重启（--no-restart）；后端静态托管目录已更新"
  exit 0
fi

echo "==> [3/3] 重启后端并健康检查"
bash scripts/dev/start-dev.sh back --restart

sleep 2
if curl --noproxy '*' -fsS http://localhost:5100/health >/dev/null 2>&1; then
  echo "✅ 部署完成：http://localhost:5100/ （API + 页面同端口）"
else
  echo "⚠️ 后端已重启但 /health 暂未就绪（可能仍在启动），稍后手动 curl http://localhost:5100/health 确认" >&2
  exit 1
fi
