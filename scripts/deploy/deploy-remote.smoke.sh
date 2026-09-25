#!/usr/bin/env bash
# deploy-remote.sh 契约冒烟：--help / --dry-run 命令拼装（排除清单）/ 免密失效指引。
# 全程不触碰真实目标 10.11.12.59：dry-run 不落地，失败路径用 127.0.0.1 假目标。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/deploy/deploy-remote.sh"

ok()  { printf '  [OK] %s\n' "$1"; }
bad() { printf '  [FAIL] %s\n' "$1" >&2; exit 1; } # 单点失败即终止并指名断言

# 1. 语法
bash -n "$SCRIPT" || bad "bash -n 语法检查失败"
ok "bash -n 通过"

# 2. --help：退出 0 且含用法
HELP_OUT="$(bash "$SCRIPT" --help 2>&1)" || bad "--help 应退出 0（输出：$HELP_OUT）"
echo "$HELP_OUT" | grep -q '用法' || bad "--help 输出缺『用法』（输出：$HELP_OUT）"
echo "$HELP_OUT" | grep -q 'dry-run' || bad "--help 输出缺 dry-run 说明"
ok "--help 含用法与 dry-run 说明"

# 3. --dry-run：命令拼装断言（不触网）
DRY_OUT="$(bash "$SCRIPT" --dry-run 2>&1)" || bad "--dry-run 应退出 0（输出：$DRY_OUT）"
echo "$DRY_OUT" | grep -q 'rsync' || bad "dry-run 缺 rsync 命令"
echo "$DRY_OUT" | grep -q -- '--delete' || bad "dry-run 缺 --delete"
echo "$DRY_OUT" | grep -q 'deploy/compose/docker-compose.yml' || bad "dry-run 缺 compose 新路径"
echo "$DRY_OUT" | grep -q -- '--project-directory' || bad "dry-run 缺 --project-directory"
echo "$DRY_OUT" | grep -q '10\.11\.12\.59' || bad "dry-run 默认目标应为 10.11.12.59"
# 排除清单：本地状态/运行产物必须在（且这些路径不会被推/删）
for ex in '.env' 'data/' 'backups/' 'logs/' 'node_modules/' '.git/'; do
  echo "$DRY_OUT" | grep -q -- "--exclude $ex" || bad "dry-run 排除清单缺: $ex"
done
ok "dry-run 命令拼装（rsync --delete + 排除清单 + compose 新路径）"

# 4. 真跑模式免密失效路径：假目标 127.0.0.1 快速失败，必须打印 ssh-copy-id 指引且非零退出
set +e
FAIL_OUT="$(DEPLOY_TARGET="deploy-smoke-invalid@127.0.0.1" HEALTH_TIMEOUT=5 bash "$SCRIPT" 2>&1)"
FAIL_RC=$?
set -e
[ "$FAIL_RC" -ne 0 ] || bad "免密/连接失败路径应非零退出（输出：$FAIL_OUT）"
echo "$FAIL_OUT" | grep -q 'ssh-copy-id' || bad "失败输出缺 ssh-copy-id 指引（输出：$FAIL_OUT）"
echo "$FAIL_OUT" | grep -q '未推送任何文件' || bad "失败输出缺『未推送任何文件』承诺（输出：$FAIL_OUT）"
ok "免密失效路径：非零退出 + ssh-copy-id 指引 + 不做半程推送"

# 5. --demo 模式 dry-run：本机构建/推镜像/远端 run.yml/5080 端口断言
DEMO_OUT="$(bash "$SCRIPT" --demo --dry-run 2>&1)" || bad "--demo --dry-run 应退出 0（输出：$DEMO_OUT）"
echo "$DEMO_OUT" | grep -q 'deploy/docker/Dockerfile.demo' || bad "demo dry-run 缺 Dockerfile.demo 构建"
echo "$DEMO_OUT" | grep -q 'docker save' || bad "demo dry-run 缺 docker save 推镜像"
echo "$DEMO_OUT" | grep -q 'gunzip | docker load' || bad "demo dry-run 缺远端 load"
echo "$DEMO_OUT" | grep -q 'demo/docker-compose.run.yml' || bad "demo dry-run 缺 run.yml"
echo "$DEMO_OUT" | grep -q '5080/health' || bad "demo dry-run 健康检查端口应为 5080"
echo "$DEMO_OUT" | grep -q -- '--exclude demo/seed/seed.sql' || bad "demo dry-run 仍应排除 seed.sql（不源码推送）"
echo "$DEMO_OUT" | grep -q 'up --build' && bad "demo 模式不应出现源码 up --build（远端零构建）" || true
ok "--demo dry-run：本机构建镜像 + save/load + run.yml + :5080 + seed 排除"

echo "deploy-remote.smoke.sh: 全部断言通过"
