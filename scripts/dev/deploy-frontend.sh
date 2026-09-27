#!/usr/bin/env bash
# 一键部署前端静态产物到后端同端口（树莓派日常形态，deployment.md §本地裸跑静态托管）。
#
# 用法：bash scripts/dev/deploy-frontend.sh [--no-restart]
#   构建静态产物（NUXT_PUBLIC_API_BASE=/api 走同源相对路径）→ 铺 backend-go/frontend/
#   → 重启后端（同端口 :5100 出 API + 页面）→ 健康检查。
#   --no-restart 只构建铺盘不重启（后端在跑时静态文件即时生效，无需重启的场景用）。
#
# 前端改动收尾顺手跑本脚本，别让用户打开页面发现还是旧版（2026-09-19 用户要求固化为习惯）。
set -euo pipefail

cd "$(dirname "$0")/../.." # 仓库根

FRONT_DIR=front
BACKEND_STATIC=backend-go/frontend
NO_RESTART=false
[[ "${1:-}" == "--no-restart" ]] && NO_RESTART=true

echo "==> [1/3] 构建静态产物（NUXT_PUBLIC_API_BASE=/api，内含完整 build）"
( cd "$FRONT_DIR" && NUXT_PUBLIC_API_BASE=/api pnpm generate )

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
