#!/usr/bin/env bash
# harness 事实库烟测：lib/harness-log（安全开库/写入查询/TTL）+ failure-classify（白名单纯函数
# + telemetry 集成）+ spec-gate 检查⑤（验收措辞扫描纯函数 + 归档门禁 policy.decision 记账）+
# policy-decision（helper 有界收敛 + quota-gate/test-scope-guard 行为 + 低噪声边界）+
# tool-output-spill（唯一命名/POSIX 权限/排他写）。
# constraint-injection 的记账回归在 run-smoke.sh（constraint-injection.smoke.cjs）。
# 用法：bash .pi/extensions/tests/run-harness-smoke.sh
set -euo pipefail
cd "$(dirname "$0")"

npx -y esbuild ../lib/harness-log.ts --bundle --platform=node --format=cjs \
  --outfile=./.hlog.cjs >/dev/null
npx -y esbuild ../lib/failure-classify.ts --bundle --platform=node --format=cjs \
  --outfile=./.fcls.cjs >/dev/null
npx -y esbuild ../harness-telemetry.ts --bundle --platform=node --format=cjs \
  --outfile=./.tel.cjs >/dev/null
npx -y esbuild ../tool-output-spill.ts --bundle --platform=node --format=cjs \
  --outfile=./.spill.cjs >/dev/null
npx -y esbuild ../spec-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.sgate.cjs >/dev/null
npx -y esbuild ../lib/policy-decision.ts --bundle --platform=node --format=cjs \
  --outfile=./.pdec.cjs >/dev/null
npx -y esbuild ../quota-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.qgate.cjs >/dev/null
npx -y esbuild ../test-scope-guard.ts --bundle --platform=node --format=cjs \
  --outfile=./.tsg.cjs >/dev/null
npx -y esbuild ../lib/test-case-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.tcg.cjs >/dev/null
npx -y esbuild ../lib/ui-design-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.uig.cjs >/dev/null
npx -y esbuild ../ui-design-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.uige.cjs >/dev/null
npx -y esbuild ../entry-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.egate.cjs >/dev/null
npx -y esbuild ../lib/trigger-set.ts --bundle --platform=node --format=cjs \
  --outfile=./.tgset.cjs >/dev/null
npx -y esbuild ../lib/edit-map.ts --bundle --platform=node --format=cjs \
  --outfile=./.emap.cjs >/dev/null
npx -y esbuild ../lib/gate-sample.ts --bundle --platform=node --format=cjs \
  --outfile=./.gsamp.cjs >/dev/null
npx -y esbuild ../quality-gate.ts --bundle --platform=node --format=cjs \
  --outfile=./.qgateb.cjs >/dev/null

rc=0
node harness-log.smoke.cjs || rc=$?
node failure-classify.smoke.cjs || rc=$?
node spill.smoke.cjs || rc=$?
node spec-gate.smoke.cjs || rc=$?
node quality-gate.smoke.cjs || rc=$?
node quality-gate.behavior.smoke.cjs || rc=$?
node test-case-gate.smoke.cjs || rc=$?
node ui-design-gate.smoke.cjs || rc=$?
node policy-decision.smoke.cjs || rc=$?
rm -f ./.hlog.cjs ./.fcls.cjs ./.tel.cjs ./.spill.cjs ./.sgate.cjs ./.tgset.cjs ./.tcg.cjs ./.egate.cjs ./.gsamp.cjs ./.pdec.cjs ./.qgate.cjs ./.tsg.cjs ./.uig.cjs ./.uige.cjs
exit $rc
