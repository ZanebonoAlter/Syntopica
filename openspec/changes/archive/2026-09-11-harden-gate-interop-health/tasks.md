## 1. 探测与短路（gate-interop-health spec：探测/短路/异常三态）

- [x] 1.1 在 quality-gate.ts turn_end 流程加 interop 健康探测：仅当本轮需执行 cmd 链路门禁（后端或前端触发/粘性）时发起 `cmd.exe /C echo ok`（timeout 2s，catch 异常同故障路径）；验证：改一行后端代码触发 turn_end，正常探测通过、门禁照常执行且 gate.check 落库（行为验证 A1-A4 ✅）
- [x] 1.2 探测失败短路：跳过本轮全部 cmd 链路门禁命令（后端三件套+域测试+pnpm lint），steer 输出「WSL interop 环境故障，非代码问题，建议 wsl --shutdown 重启」提示段；验证：临时把探测超时调至 1ms 模拟故障，确认门禁零执行、回合不被阻断（行为验证 B4/B5 ✅：mock cmd.exe 全 reject 模拟故障）
- [x] 1.3 短路记账：logEvent 一条 policy.decision（policy=quality-gate, action=fail-open, reasonCode=interop-down, durationMs=探测耗时），被跳过命令不记 gate.check；验证：短路后 sqlite 查 events.db 出现该事件且本轮无新 gate.check（行为验证 B1/B2/B3 ✅，统一走 logPolicyDecision helper）

## 2. 环境故障识别与粘性豁免（spec：特征识别/粘性豁免/真实失败不变）

- [x] 2.1 gateLog 增加 diag 特征识别（`<N>WSL (… - ) ERROR` 前缀 + `UtilAcceptVsock` 双关键字锚定）：命中的失败不 add stickyFailures、不推 [回归]/[中间态] failures，单独构造环境故障提示段；验证：构造 diag 含 UtilAcceptVsock 的失败后，下 turn 纯对话不重跑该命令（smoke I1-I8 + 行为验证 C1-C5 ✅，特征函数下沉 lib/failure-classify.ts isInteropFailure）
- [x] 2.2 真实失败路径回归验证：正常编译错误/lint 失败仍进粘性、按既有分级 steer；验证：引入一处真实编译错误，确认 [回归]/[中间态] 语义与探测机制引入前一致（行为验证 D1-D3 ✅）

## 3. （wsl环境）链路标注（spec：标注不弱化修复义务）

- [x] 3.1 cmd 链路门禁命令的失败 steer 行加（wsl环境）标注（如 `[golangci-lint (wsl环境)] exit 1`），真实失败与环境失败均生效；验证：观察 1.1 与 2.2 场景的 steer 消息均含标注且真实失败修复义务不受影响（行为验证 D2/C3 ✅，gateLog 统一标注）

## 4. 文档与快照同步

<!-- doc-impact: none(纯 harness 工具链：.pi/extensions gitignored + docs/research 快照 + AGENTS/SKILL 规则文档，无 reference 活文档变更) -->（无 flow 影响：纯工具链 change，§12.2 豁免）

- [x] 4.1 docs/research/harness事实库.md：policy.decision reasonCode 词汇表补 interop-down、gate.check 环境故障分支说明、quality-gate 探测机制描述；同步 .pi/extensions 快照到 docs/research/；验证：文档 grep interop-down 命中、快照与实文件一致（快照同步 quality-gate.ts/lib/failure-classify.ts/两个 smoke；harness事实库.md 补 3.3 节与注意事项；SKILL.md 词汇表 +interop-down；AGENTS.md 全景表更新 ✅）
- [ ] 4.2 完工汇报：按 AGENTS.md 要求说明部署影响（harness 行为变化：故障期门禁静默跳过+提示、账本新增事件类型）与无需用户手动操作的说明

## 5. 测试

- [x] 5.1 harness smoke 全量：`bash .pi/extensions/tests/run-harness-smoke.sh` → 6 文件全 SMOKE OK，277 断言 0 失败（新增 I1-I8 isInteropFailure 边界用例；修订双写隔离断言为「logPolicyDecision 仅 interop 短路一处」）
- [x] 5.2 行为级四场景（mock pi 驱动真实 turn_end handler + tmp events.db）：正常态 A1-A4 / 短路态 B1-B5 / 兜底态 C1-C5 / 真实失败态 D1-D3，17 断言全过

## 6. 验证

- [x] 6.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 期望 0 失败（实测 277/277 绿）
- [x] 6.2 `grep -l interop-down docs/research/quality-gate.ts docs/research/harness事实库.md .agents/skills/harness-facts/SKILL.md` → 期望三文件全命中（快照/文档/词汇表同步，实测 ✓）
- [x] 6.3 reload 后真实环境回归：探测健康时门禁行为与旧版一致（gate.check 正常落库、零 policy.decision）——reload 后首个 cmd 链路门禁 turn_end 验证（实测 2026-09-11T14:48Z：touch 后端文件触发，lint/vet/build/topicgraph 域测试/pnpm lint 全绿各记 flip 锚点，零 policy.decision，探测健康零额外开销 ✓）

| Scenario | 测试文件 |
|---|---|
| 探测失败短路整轮 cmd 链路门禁 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 探测成功不改变既有门禁行为 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 探测调用异常也 fail-open | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 环境故障失败不触发粘性重跑 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs,.pi/extensions/tests/failure-classify.smoke.cjs |
| 真实代码失败保持既有语义 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 后端门禁失败提示含链路标注 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 环境故障归因与链路标注同时呈现 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| spec-gate 阻断归档被记录 | .pi/extensions/tests/spec-gate.smoke.cjs,.pi/extensions/tests/policy-decision.smoke.cjs |
| spec-gate 显式豁免被记录 | .pi/extensions/tests/spec-gate.smoke.cjs,.pi/extensions/tests/policy-decision.smoke.cjs |
| quota-gate 阻断与 fail-open 被区分 | .pi/extensions/tests/policy-decision.smoke.cjs |
| test-scope 软硬模式被记录 | .pi/extensions/tests/policy-decision.smoke.cjs |
| interop 探测短路被记账且不双写 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs,.pi/extensions/tests/policy-decision.smoke.cjs |
| 正常放行零记录 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs,.pi/extensions/tests/policy-decision.smoke.cjs |
| 记账故障不改变裁决 | .pi/extensions/tests/policy-decision.smoke.cjs,.pi/extensions/tests/harness-log.smoke.cjs |
