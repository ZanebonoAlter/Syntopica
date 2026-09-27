
## spec-gate warn 投递点与测试基建定位

**spec-gate warn 投递点与测试基建（apply 阶段直接动手，无需重探）**

1. 改动面唯一文件 `.pi/extensions/spec-gate.ts`（457 行）：
   - 检查⑤投递点：`warnAcceptanceWording()`（约 L292 起）——对 `scanAcceptanceWording(r.stdout, files, proposalText)` 返回的文案数组逐条 `warn()` + `auditPolicy(reasonCode:"acceptance-wording", action:"warn")`。tasks.md 读不到（code!==0）静默跳过；`ls` 失败伪造成 `["test-cases.md"]` 抑制⑤a。
   - 检查⑤'投递点：`gateArchive()` 内「3.5 检查⑤'」段（约 L184 起）——`concurrency-status.sh --check` exit 2 时 `warn()` + `auditPolicy(reasonCode:"concurrent-dirty-tree", action:"warn")`；exit 0/3 零输出。
   - `warn(pi, ...)` 辅助函数在文件尾部（约 L439）：`pi.sendMessage({ customType: "spec-gate-warning", content, display: true })`。
   - `auditPolicy(ctx, name, {...})` 已有（lib/policy-decision）；`GateCtx` 已含 `sessionManager.getSessionId`。
   - block 路径（failures 收集、`archive-check-failed`、`ui-verification-missing` 代记）**一行都不能动**；bypass / fail-open / 无名 fail-open 的 warn 不进指纹表。
2. 会话键先例：`constraint-injection.ts` `sessionKey()`——`ctx.sessionManager?.getSessionId?.()` 为空落 `__no-session__` 兜底槽；LRU 上限先例 `sessionStateLimit`（淘汰最久未用）。compact 事件：`pi.on("session_compact")`（constraint-injection D4 同款用法）。
3. 指纹：`node:crypto` createHash("sha256")，⑤'输入=conc.stdout 按行 trim→滤空→排序去重→join；⑤输入=文案数组 join；存 16 字节 hex 前缀。
4. 测试：`.pi/extensions/tests/spec-gate.smoke.cjs` 已有 mock 框架——`makeGatePi(execImpl)` + `runGate(pi, command, cwd)` 固定 `getSessionId:()=> 'sgs1'`；改造时给 runGate 加可选 sessionId 参数即可测「同会话 vs 新会话」；`passExec` mock 返回 `ls: 含 test-cases.md`、`cat proposal: <!-- complexity: simple -->`。跑法：`bash .pi/extensions/tests/run-harness-smoke.sh`（esbuild 先 bundle spec-gate.ts → .sgate.cjs）。
5. 归档门禁触发词：tool_call bash 命令正则 `/openspec\s+archive/`；smoke 里用 `openspec archive foo` 直接驱动 handlers["tool_call"]。

**引用**：.pi/extensions/spec-gate.ts:warnAcceptanceWording、.pi/extensions/spec-gate.ts:gateArchive、.pi/extensions/tests/spec-gate.smoke.cjs:makeGatePi、.pi/extensions/constraint-injection.ts:sessionKey

<!-- pinned 2026-09-23T03:13:51Z -->
