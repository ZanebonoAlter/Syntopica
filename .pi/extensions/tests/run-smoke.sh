#!/usr/bin/env bash
# constraint-injection + 子线程通道扩展层 smoke 总入口（harden-subagent-constraint-channel）：
# ① constraint-injection 全量回归：stub typebox，esbuild 打包 extension，以真实事件形状
#    （args 字段）回放切档→绑定→注入→JIT→pin 链路，含 pi-web 子会话继承用例（19.13）；
# ② 委托 run-harness-smoke.sh 跑 harness 全家桶：含 lib/child-session isChildSession 判定
#    矩阵、quality-gate 子线程 turn_end 降载 bypass（零命令+一条记账）与主会话不受影响、
#    policy.decision 形状/源码断言。
# 用法：bash .pi/extensions/tests/run-smoke.sh
set -euo pipefail
cd "$(dirname "$0")"
cat > .typebox-stub.cjs <<'STUB'
const Type = new Proxy({}, { get: () => () => ({}) });
module.exports = { Type };
STUB
npx -y esbuild ../constraint-injection.ts --bundle --platform=node --format=cjs \
  --alias:typebox="$PWD/.typebox-stub.cjs" --outfile=./.bundle.cjs >/dev/null
rc=0
node constraint-injection.smoke.cjs || rc=$? # set -e 下 node 失败会跳过清理，用 || 兜住
rm -f ./.bundle.cjs ./.typebox-stub.cjs
# harness 全家桶（含本 change 新增用例：child-session 矩阵 + quality-gate 子线程 bypass）
bash ./run-harness-smoke.sh || rc=$?
exit $rc
