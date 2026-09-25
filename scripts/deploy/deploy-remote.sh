#!/usr/bin/env bash
# 一键免密部署到远程 Linux/Docker 主机（默认 zanebono@10.11.12.59 → ~/software/Syntopica）。
#
# 两种模式：
#   默认（全量）：本机预编译 Go 二进制 → rsync 源码 → 远程 compose 源码构建 → /health :5100
#   --demo（demo）：本机构建 demo 镜像（需 demo/seed/seed.sql）→ docker save 推镜像
#                  → 远端 demo/docker-compose.run.yml 拉起（Pi 内存限额调优）→ /health :5080
#   demo 是「对外展示」形态：只读（DEMO_READ_ONLY=1）、脱敏 seed、无后台调度。
#
# 共同流程：SSH 免密/远端 Docker 自检 → 推送 → 远端起栈 → /health 轮询。
# - 自检失败非零退出并给修复动作（如 ssh-copy-id 指引），不做半程推送。
# - rsync -a --delete，排除清单镜像 .gitignore（目标端 .env / data/ / backups/ /
#   logs/ / *.tar / demo/seed/seed.sql 不推不删——seed 在镜像里，不上源码推送）。
#
# 用法：
#   bash scripts/deploy/deploy-remote.sh              # 全量部署（默认目标）
#   bash scripts/deploy/deploy-remote.sh --demo       # demo 镜像部署（对外展示形态）
#   bash scripts/deploy/deploy-remote.sh --dry-run    # 只打印将执行的命令，不落地
#   bash scripts/deploy/deploy-remote.sh user@host    # 覆盖目标（或用 DEPLOY_TARGET 环境变量）
# 环境变量：DEPLOY_TARGET（目标）、REMOTE_PATH（远端目录）、HEALTH_TIMEOUT（秒，默认 300）、
#           DEMO_IMAGE_TAG（demo 镜像 tag，默认 latest；远端旧 tag 保留可用 DEMO_TAG 切换回滚）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TARGET="${DEPLOY_TARGET:-zanebono@10.11.12.59}"
REMOTE_PATH="${REMOTE_PATH:-software/Syntopica}"   # 相对远端 ~（可覆盖为绝对路径）
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-300}"
HEALTH_PORT="${HEALTH_PORT:-5100}"
DEMO_IMAGE_TAG="${DEMO_IMAGE_TAG:-latest}"
DEMO_IMAGE="zanebonoalter/syntopica-demo:${DEMO_IMAGE_TAG}"
DRY_RUN=0
MODE="full"

usage() { sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'; }

for arg in "$@"; do
  case "$arg" in
    --demo) MODE="demo"; HEALTH_PORT=5080 ;;
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "未知参数: $arg" >&2; usage >&2; exit 2 ;;
    *) TARGET="$arg" ;;
  esac
done

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new)
info() { printf '→ %s\n' "$1"; }
ok()   { printf '  ✓ %s\n' "$1"; }
fail() { printf '  ✗ %s\n' "$1" >&2; exit 1; }

REMOTE_DIR="$REMOTE_PATH"
HEALTH_URL="http://${TARGET#*@}:${HEALTH_PORT}/health"

COMPOSE_MAIN="docker compose --project-directory . -f deploy/compose/docker-compose.yml"
COMPOSE_DEMO="docker compose -f demo/docker-compose.run.yml"

# ── rsync 排除清单（镜像 .gitignore 的本地状态/运行产物；被排除路径不受 --delete 影响）──
EXCLUDES=(
  --exclude '.git/'
  --exclude 'node_modules/'
  --exclude 'data/'
  --exclude 'backups/'
  --exclude 'logs/'
  --exclude 'front/.nuxt/'
  --exclude 'front/.output/'
  --exclude 'backend-go/frontend/'
  --exclude '.env'
  --exclude 'nohup.out'
  --exclude '**/nohup.out'
  --exclude '.codegraph/'
  --exclude '.worktrees/'
  --exclude '.pi/harness/'
  --exclude '.pi/prompts/'
  --exclude '.pi/skills/'
  --exclude '.pi/run/'
  --exclude '.pi/subagents/'
  --exclude '.pi/tmp/'
  --exclude '__pycache__/'
  --exclude '.venv/'
  --exclude '.pytest_cache/'
  --exclude '*.dump'
  --exclude '*.tar'
  --exclude '*.exe'
  --exclude 'data.7z'
  --exclude 'analysis-reports/'
  --exclude 'artifacts/'
  --exclude 'demo/seed/seed.sql'
)

RSYNC_CMD=(rsync -a --delete "${EXCLUDES[@]}" ./ "$TARGET:$REMOTE_DIR/")
SSH_CMD=(ssh "${SSH_OPTS[@]}" "$TARGET")

if [ "$MODE" = "demo" ]; then
  REMOTE_UP_CMD="cd \"$REMOTE_DIR\" && DEMO_TAG=$DEMO_IMAGE_TAG $COMPOSE_DEMO up -d"
else
  REMOTE_UP_CMD="cd \"$REMOTE_DIR\" && $COMPOSE_MAIN up --build -d"
fi

printf '部署模式: %s｜目标: %s:%s｜健康检查: %s\n' \
  "$([ "$MODE" = demo ] && echo 'demo（对外展示）' || echo 'full（全量源码构建）')" \
  "$TARGET" "$REMOTE_DIR" "$HEALTH_URL"

if [ "$DRY_RUN" -eq 1 ]; then
  info "[dry-run] 将执行（不落地）:"
  if [ "$MODE" = "demo" ]; then
    printf '  # 前置：demo/seed/seed.sql 存在（否则提示先跑 dump-sanitizer）\n'
    printf '  docker build -f deploy/docker/Dockerfile.demo -t %s .\n' "$DEMO_IMAGE"
    printf '  docker save %s | gzip -1 | ssh <target> %s\n' "$DEMO_IMAGE" "'gunzip | docker load'"
    printf '  # rsync 推送源码（compose 文件/docker/postgres init 等；seed.sql 已排除——已在镜像内）\n'
  else
    printf '  # 前置：本机 CGO_ENABLED=0 go build -o backend-go/syntopica ./cmd/server\n'
  fi
  printf '  '; printf '%q ' "${RSYNC_CMD[@]}"; printf '\n'
  printf '  '; printf '%q ' "${SSH_CMD[@]}" "$REMOTE_UP_CMD"; printf '\n'
  printf '  # 然后轮询 %s 直到 200（超时 %ss）\n' "$HEALTH_URL" "$HEALTH_TIMEOUT"
  exit 0
fi

# ── 自检 ────────────────────────────────────────────────────────────────────
info "自检：本机工具"
command -v rsync &>/dev/null || fail "rsync 未安装"
command -v ssh &>/dev/null || fail "ssh 未安装"
[ "$MODE" = "full" ] || command -v docker &>/dev/null || fail "本机 docker 未安装（demo 模式需本机构建镜像）"
ok "rsync / ssh$([ "$MODE" = demo ] && echo ' / docker') 可用"

info "自检：SSH 免密（$TARGET）"
if ! ssh "${SSH_OPTS[@]}" "$TARGET" true 2>/dev/null; then
  echo "    SSH 免密未生效。先在本机执行（需输一次目标密码）：" >&2
  echo "      ssh-copy-id -i ~/.ssh/id_ed25519.pub $TARGET" >&2
  fail "SSH 免密自检失败，未推送任何文件"
fi
ok "SSH 免密生效"

info "自检：远端 Docker"
remote_docker_err="$(ssh "${SSH_OPTS[@]}" "$TARGET" 'docker compose version >/dev/null 2>&1 && docker info >/dev/null 2>&1' 2>&1)" \
  || fail "远端 Docker/compose 不可用（docker 组权限或 Docker 未装）：${remote_docker_err}"
ok "远端 docker compose 可用"

# ── 本机构建（demo：镜像；full：预编译二进制）────────────────────────────────
if [ "$MODE" = "demo" ]; then
  info "自检：demo seed"
  if [ ! -s "$REPO_ROOT/demo/seed/seed.sql" ]; then
    echo "    demo/seed/seed.sql 缺失（gitignore 不入库，需从本地库生成）：" >&2
    echo "      cd backend-go && go run ./cmd/dump-sanitizer" >&2
    fail "demo seed 缺失，未推送任何文件"
  fi
  ok "demo/seed/seed.sql 就绪（$(( $(stat -c%s "$REPO_ROOT/demo/seed/seed.sql") / 1024 / 1024 ))MB）"

  info "本机构建 demo 镜像（$DEMO_IMAGE；前端 generate + Go 编译都在镜像内）"
  docker build -f "$REPO_ROOT/deploy/docker/Dockerfile.demo" -t "$DEMO_IMAGE" "$REPO_ROOT" \
    || fail "demo 镜像构建失败，未推送任何文件"
  ok "镜像构建完成"

  info "推送镜像（docker save | gzip | docker load，远端零构建）"
  docker save "$DEMO_IMAGE" | gzip -1 | ssh "${SSH_OPTS[@]}" "$TARGET" 'gunzip | docker load' \
    || fail "镜像推送失败"
  ok "镜像已载入远端"
else
  info "本机预编译后端二进制（CGO_ENABLED=0，运行阶段 alpine 需静态链接）"
  ( cd "$REPO_ROOT/backend-go" && CGO_ENABLED=0 go build -o syntopica ./cmd/server ) \
    || fail "后端构建失败，未推送任何文件"
  ok "backend-go/syntopica 就绪（将随 rsync 推送）"
fi

# ── 推送源码（compose 文件 / init SQL / 源码保持同步；seed 已排除）───────────
info "rsync 推送（--delete；.env/data/backups/seed.sql 等被排除不删不推）"
"${RSYNC_CMD[@]}" --stats | tail -4
ok "推送完成"

# ── 远程起栈 ────────────────────────────────────────────────────────────────
if [ "$MODE" = "demo" ]; then
  info "远程: DEMO_TAG=$DEMO_IMAGE_TAG $COMPOSE_DEMO up -d"
else
  info "远程: $COMPOSE_MAIN up --build -d"
fi
ssh "${SSH_OPTS[@]}" "$TARGET" "$REMOTE_UP_CMD" \
  || fail "远程 compose 启动失败（详见上方输出）"

# ── 健康检查 ────────────────────────────────────────────────────────────────
info "等待 /health 200（最多 ${HEALTH_TIMEOUT}s）: $HEALTH_URL"
deadline=$((SECONDS + HEALTH_TIMEOUT))
until curl -fsS --noproxy '*' --connect-timeout 3 "$HEALTH_URL" >/dev/null 2>&1; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "── 远端容器状态 ──" >&2
    if [ "$MODE" = "demo" ]; then
      ssh "${SSH_OPTS[@]}" "$TARGET" "cd \"$REMOTE_DIR\" && $COMPOSE_DEMO ps" >&2 || true
      ssh "${SSH_OPTS[@]}" "$TARGET" "docker logs --tail=40 syntopica-demo" >&2 || true
    else
      ssh "${SSH_OPTS[@]}" "$TARGET" "cd \"$REMOTE_DIR\" && $COMPOSE_MAIN ps" >&2 || true
      ssh "${SSH_OPTS[@]}" "$TARGET" "cd \"$REMOTE_DIR\" && $COMPOSE_MAIN logs --tail=40 syntopica" >&2 || true
    fi
    fail "/health 在 ${HEALTH_TIMEOUT}s 内未返回 200"
  fi
  sleep 5
done
ok "/health 返回 200"

if [ "$MODE" = "demo" ]; then
  printf 'demo 部署成功: %s（http://%s:%s/）\n' "$TARGET" "${TARGET#*@}" "$HEALTH_PORT"
  printf '注意：entrypoint 在 health 后才导入 seed（约几分钟），页面数据会稍后填充；回滚: DEMO_TAG=<旧tag> 重跑\n'
else
  printf '部署成功: %s（页面/API 入口 http://%s:%s/）\n' "$TARGET" "${TARGET#*@}" "$HEALTH_PORT"
fi
