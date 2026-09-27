# Tasks

## 1. 用例先行（先红后绿）

- [x] 1.1 在 `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` 补 test-cases.md B1 组 8 条用例（锁生命周期：抢到/held 跳过零记账/stale 覆盖/finally 释放/不误删他方锁/短路轮不抢锁）与 B3 组 5 条（classify 三态与保守回退）、B4 组 8 条（[并发] 前缀/进粘性/⟳ 抑制/转绿/判性翻转/双记账/同回合并存/两极回归）；跑 `bash .pi/extensions/tests/run-harness-smoke.sh quality-gate.behavior` 确认新用例红（缝未存在）
- [x] 1.2 在 `.pi/extensions/tests/quality-gate.smoke.cjs` 补 B2 组 5 条命令形态用例（GOMAXPROCS=2 前缀两形态/lint --concurrency=2/vet-build 串行/eslint 不变）；同上确认红

## 2. 门禁互斥锁（design D1）

- [x] 2.1 在 `.pi/extensions/quality-gate.ts` 实现 `acquireGateLock` / `releaseGateLock`：`fs.openSync(.pi/harness/gate.lock, "wx")` 原子创建，held 时读 mtime 判 stale（TTL 180s，超时覆盖）；锁内容单行 JSON `{sessionId, ts, cmd}`；释放比对 sessionId 再删、吞 ENOENT（B1-7 语义）
- [x] 2.2 turn_end 主链路接入：位置在 step 3.5 探测短路之后、step 3.7 归因预取之前；held → 记 `logPolicyDecision(fail-open/gate-lock-held)` 后整轮 return（零命令零 gate.check 零 steer，粘性集合不动；action 用 fail-open 而非原稿 skip——主 spec action 枚举固定四值，与 interop-down 同构）；acquired → 命令执行与 steer 段整体包进 try/finally 释放；短路轮（interop-down / toolchain-down / 两侧均跳过）不抢锁

## 3. 限核（design D2）

- [x] 3.1 `runBackend` 的 cmdline 统一加 `GOMAXPROCS=2 ` 前缀（native bash 分支）；windows 分支改 `set GOMAXPROCS=2 && ` 形态；golangci-lint 追加 `--concurrency=2`（保留 `--allow-parallel-runners`）
- [x] 3.2 vet/build 从 `Promise.all` 改顺序 await；域测试循环天然串行不动；前端 eslint 命令零改动

## 4. 三态归因与记账词汇（design D3/D4）

- [x] 4.1 新增纯函数 `classifyFailureOwnership({paths, mine, foreign})` → `foreign|mixed|mine`（判定序：纯外部 → 混合 → 保守 mine；paths 空/异常回退 mine）；`formatMixedFailure(cmd, myPaths, foreignPaths)` 产出 [并发] 行（双方各 ≤3 条 + 计数 + 「可能非本会话所致，归档前仍需全绿」）
- [x] 4.2 `gateLog` 失败分支改三态分流：mixed → 进 stickyFailures + `failureReports` 条目加 `mixed: true` + failures 行用 formatMixedFailure（不取 d.failPrefix）+ 每命中回合记 `logPolicyDecision(warn/concurrent-mixed)`；同指纹 ⟳ / 转绿 ✓ / 判性翻转恢复完整块复用既有指纹状态机；foreign / mine 两极与 envFailures（interop/toolchain）路径零改动
- [x] 4.3 既有 step 3.7 归因预取（mine/foreign 集合构建）零改动，仅消费侧换 classify 三态

## 5. 测试

- [x] T1 `bash .pi/extensions/tests/run-harness-smoke.sh quality-gate.behavior quality-gate.smoke` → 1.1/1.2 全部用例转绿（26 条新增断言）
- [x] T2 全量扩展 smoke 回归：`bash .pi/extensions/tests/run-harness-smoke.sh`（及 `run-smoke.sh` 若涉及 quality-gate bundle）无新增红
- [x] T3 实机双会话演练一次锁路径（两个 pi 会话并发改后端文件触发门禁；若实机不可控则以 T1 覆盖为准并记录跳过理由）：后到会话该轮记 skip/gate-lock-held、先到会话门禁正常跑完释放
  - 跳过理由（归档期留痕）：归档时点无可控并发窗口，实机演练未发生；锁路径行为由 T1 的 B1 组 8 条真 tmp 目录用例全链路覆盖（抢到/held 跳过零记账/stale 覆盖/finally 释放/不误删他方锁/短路轮不抢锁）。实机效果挂账：后续并发窗口的 events.db 可回检 gate-lock-held / concurrent-mixed 计数（同 attribute-concurrent-gate-noise 4.4 模式）。

## 6. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 属 harness 扩展机制全景文档，随本 change 就地同步) -->
- **无 flow 影响**（§12.2 归档后回填：纯 quality-gate 并发调参，非业务链路改动，不触及任何 `flow/*.md` 业务链路，E 段溯源豁免）

- [x] D1 `docs/reference/harness/pi-extensions.md`：quality-gate 节补「并发互斥与限核（gate-lock-held / GOMAXPROCS=2 / vet-build 串行）」与「混合归属降级（concurrent-mixed）」小节；记账口径表补两行新 reasonCode
- [x] D2 `docs/research/quality-gate-concurrency/explore-findings.md` 顶部标注：三个影响渠道已由本 change 处理（真增量命令方案 C 仍开放为后续 change）

## 7. 验证

- [x] V1 `openspec validate tune-quality-gate-concurrency` → valid（归档前复验通过，2026-09-19）
- [x] V2 `bash .pi/extensions/tests/run-harness-smoke.sh` → exit 0 全绿
- [x] V3 事件账本抽查：演练回合后 `policy.decision` 出现 gate-lock-held / concurrent-mixed 词汇且 payload 形状与 D4 一致
  - 跳过理由（归档期留痕）：前置演练（T3）未发生，账本现状 0 命中已记录在案（2026-09-19 查）；词汇与 payload 形状由 T1 断言覆盖（B1-4 恰 1 条 fail-open/gate-lock-held、B4-6 双记账 concurrent-mixed）。实机回检挂账同 T3。
- [x] V4 归档前映射检查：specs 10 个 Scenario（实数；原稿写 11 不准）↔ test-cases B1–B4 组 ↔ smoke 断言三层可追溯（映射表见下方验证附录；归档前复核 spec 实数 10 条与附录逐行对上，scenario-trace 通过）

### 验证附录：spec Scenario ↔ test-cases ↔ smoke 断言三层追溯（V4）

| Scenario | 测试文件 |
| --- | --- |
| 抢到锁正常执行门禁 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 锁被并发会话持有时本轮跳过 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 残留锁超 TTL 后可抢占 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 门禁命令异常时锁仍释放 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| go 命令以限核参数执行 | .pi/extensions/tests/quality-gate.smoke.cjs |
| vet 与 build 串行 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 混合归属失败标 [并发] 不标 [回归] | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 混合归属照进粘性重跑 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 纯归属两极不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归因信号缺席时保守回退 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |

三层细粒度追溯（用例编号 ↔ smoke 断言，供人工回查；表头非门禁格式，scenario-trace 不解析）：

| spec Scenario（10 实数） | test-cases 用例 | smoke 断言（quality-gate.behavior / .smoke） |
| --- | --- | --- |
| R1① 抢到锁正常执行门禁 | B1-1 / B1-5 | B1-1（acquired+内容）/ B1-5（执行期间锁在、回合结束锁删） |
| R1② 锁被并发会话持有时本轮跳过 | B1-2 / B1-4 | B1-2（held+不改写）/ B1-4（零命令零 gate.check 零 steer+恰 1 条 fail-open/gate-lock-held+粘性保留重跑） |
| R1③ 残留锁超 TTL 后可抢占 | B1-3 | B1-3（mtime 超 180s 覆盖为本会话） |
| R1④ 门禁命令异常时锁仍释放 | B1-6 | B1-6（异常冒泡+锁文件不存在） |
| R2① go 命令以限核参数执行 | B2-1 / B2-2 / B2-3 | B2-1（native GOMAXPROCS=2 前缀）/ B2-2（windows set…&& 形态+cd /d 不破坏）/ B2-3（lint --concurrency=2+--allow-parallel-runners） |
| R2② vet 与 build 串行 | B2-4 | B2-4（build 发起不早于 vet 结算，异步窗口检测） |
| R3① 混合归属失败标 [并发] 不标 [回归] | B4-1 / B4-7 | B4-1（[并发] 前缀+双方路径+全绿提示+无 [回归]/[中间态] 行前缀）/ B4-7（同回合并存分列） |
| R3② 混合归属照进粘性重跑 | B4-2 / B4-3 / B4-4 | B4-2/3（粘性重跑+⟳ 单行）/ B4-4（转绿 ✓ 收尾+指纹清除） |
| R3③ 纯归属两极不变 | B4-8（+既有 N/D/L 场景不动照绿） | B4-8（纯外部 [外部] 不进粘性/纯 mine [回归] 分级）+ N1–N6/D/L 既有断言 |
| R3④ 归因信号缺席时保守回退 | B3-4 / B3-5 | B3-4（paths 空→mine）/ B3-5（异常输入→mine）+ N8（解析不出）/ P3（库不可用） |

补充映射：锁语义边界 B1-7（不误删他方锁）对应 R1①/R1④ 的释放语义；B1-8（短路轮不抢锁）对应 R1 Requirement 正文「未执行零 gate.check 记账」约束的短路前置条件；B4-5（判性翻转）与 B4-6（双记账）对应 R3 Requirement 正文的指纹状态机复用与每命中回合记账。既有 policy-decision.smoke「恰七处」断言同步更新（interop×1+toolchain×2+child-session×1+foreign-breakage×1+gate-lock-held×1+concurrent-mixed×1）。
