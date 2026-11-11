#!/usr/bin/env bash
# run-backend-embedded：把现有后端二进制指向 embedded PG 跑一轮（0-7 B2 的「换 DSN」论断）。
#
# 用法：bash scripts/run-backend-embedded.sh <DSN> [端口] [触发日报的日期]
#
# 行为：
#   1) DATABASE_DSN=<DSN> SERVER_PORT=<端口> 起 backend-go/cmd/server（cwd=backend-go，
#      因为 configs/config.yaml 是相对路径）；日志落 backend-go/logs/app.log。
#   2) 轮询 /api/health 直到 200（记录启动耗时）。
#   3) POST /api/daily-reports/generate 触发一轮日报管线（记录耗时与响应）。
#   4) 停后端（POST /admin/shutdown，失败则 SIGTERM）。
#
# ⚠️ 绝不用真库 DSN：本脚本只接受 embedded PG 的 DSN（端口非 5432）。
set -Eeuo pipefail

DSN="${1:?usage: run-backend-embedded.sh <DSN> [port] [date]}"
PORT="${2:-5199}"
DATE="${3:-$(date +%F)}"
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$LAB_DIR/../../.." && pwd)"
BACKEND="$REPO_ROOT/backend-go"
RESULTS="$LAB_DIR/_tmp/results"
mkdir -p "$RESULTS"

case "$DSN" in
  *:5432/*) echo "ERROR: 拒绝指向 5432（真库）" >&2; exit 2 ;;
esac

echo "### 后端指向 embedded PG：port=$PORT dsn=${DSN%%\?*}"

cd "$BACKEND"
export DATABASE_DSN="$DSN"
export SERVER_PORT="$PORT"
export CORS_ORIGINS="http://localhost:3000"
# 外网 LLM（DeepSeek 云路由）需代理
export HTTPS_PROXY="${HTTPS_PROXY:-http://127.0.0.1:7890}"
export HTTP_PROXY="${HTTP_PROXY:-http://127.0.0.1:7890}"
export no_proxy="localhost,127.0.0.1,::1"

BIN="${BIN:-$LAB_DIR/_tmp/syntopica-server}"
if [ ! -x "$BIN" ]; then
  echo "先构建：cd backend-go && go build -o ../desktop/lab/embeddedpg/_tmp/syntopica-server ./cmd/server" >&2
  exit 2
fi

T0=$(date +%s.%N)
nohup "$BIN" > "$RESULTS/backend-embedded.log" 2>&1 &
BACK_PID=$!
echo "backend_pid=$BACK_PID"

# 等健康
READY=""
for i in $(seq 1 180); do
  if curl -s --noproxy '*' -o /dev/null -w '' "http://127.0.0.1:$PORT/health" 2>/dev/null; then
    READY=$(date +%s.%N); break
  fi
  sleep 1
done
if [ -z "$READY" ]; then
  echo "ERROR: 后端 180s 内没起来，看 $RESULTS/backend-embedded.log"
  tail -30 "$RESULTS/backend-embedded.log"
  kill "$BACK_PID" 2>/dev/null || true
  exit 1
fi
echo "backend_ready_seconds=$(echo "$READY - $T0" | bc)"

# 触发日报管线
echo "### POST /api/daily-reports/generate date=$DATE"
T1=$(date +%s.%N)
RESP="$(curl -s --noproxy '*' -X POST "http://127.0.0.1:$PORT/api/daily-reports/generate" \
  -H 'Content-Type: application/json' -d "{\"date\":\"$DATE\"}" || true)"
T2=$(date +%s.%N)
echo "daily_report_trigger_seconds=$(echo "$T2 - $T1" | bc)"
echo "response=${RESP:0:2000}"

# 停后端
curl -s --noproxy '*' -X POST "http://127.0.0.1:$PORT/admin/shutdown" -o /dev/null 2>/dev/null || true
sleep 10
kill "$BACK_PID" 2>/dev/null || true
pkill -f "syntopica-server" 2>/dev/null || true
echo "### done"
