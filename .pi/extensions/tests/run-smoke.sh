#!/usr/bin/env bash
# constraint-injection extension smoke test
# stub typebox，esbuild 打包 extension，然后以真实事件形状（args 字段）回放
# 切档→绑定→注入→JIT→pin 链路。用法：bash .pi/extensions/tests/run-smoke.sh
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
exit $rc
