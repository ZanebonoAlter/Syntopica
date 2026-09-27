## 1. 用例先行（先红后绿）

- [x] 1.1 在 `.pi/extensions/tests/` 新增/扩展 quality-gate 报告注入用例：同指纹连续 3 回合仅首回合注入完整块、后 2 回合零 steer 注入，且粘性重跑仍发生（命令执行次数不减）。验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 中该用例当前红（实现未动）
- [x] 1.2 新增记账用例：同指纹 5 回合仅产生 1 条 `policy.decision(concurrent-mixed)`（现行实现每回合记 → 红）；指纹变化与转绿后再现各允许再记。验证：同 1.1 smoke 中该用例当前红
- [x] 1.3 新增归属用例：包锚点 `# <pkg>` 所指目录∩已知归属集全部为 foreign → 判 [外部]（现行 fail-open → 红）；包目录无归属匹配 → 保守回退 [回归]（现行绿，作防误降级锚点）。**注：实测 baseline 归属构造下现行已可判 [外部]（R1 绿，见 explore-findings 任务1.3 记录），两用例均作防回归锚点
- [x] 1.4 新增防回归锚点：转绿仍输出单行收尾（每失败段至多一次）、指纹变化仍输出完整块（现行已绿，锁住不许改坏）

## 2. 边沿触发注入（design D1/D5）

- [x] 2.1 `quality-gate.ts` 持续指纹分支不再 push 单行摘要与「≥3 回合未修」标记；首次/指纹变化/转绿三分支保持。验证：1.1、1.4 用例转绿
- [x] 2.2 核对粘性重跑集合（`stickyFailures`）、触发集、gate.lock、限核零改动。验证：`git diff .pi/extensions/quality-gate.ts` 中无 `acquireGateLock`/`GOMAXPROCS`/触发集相关行变更，且 1.1 用例断言重跑次数不减

## 3. 记账边沿化（design D2）

- [x] 3.1 `logPolicyDecision(concurrent-mixed)` 移到新指纹判定点（与完整块注入同一边沿）。验证：1.2 用例转绿
- [x] 3.2 指纹变化/失败段转绿重建后允许再记的分支用例。验证：1.2 后半段断言转绿

## 4. 包锚点归属解析（design D3）

- [x] 4.1 失败输出提取扩展 `# <pkg>` / `FAIL <pkg> [` / `vet: <file>:<line>` 三形态，包路径映射 `backend-go/<pkg>` 目录前缀后与（git 脏文件 ∪ 各 `edit.map` 归属集）求交并入 P，空交保守回退。验证：1.3 用例全绿

## 5. 状态查询入口（design D4）

- [x] 5.1 新建 `scripts/harness/gate-status.sh`：sqlite3 `file:…?mode=ro` 只读 + 窗口函数取每 cmd 最新 `gate.check`，红/绿态 + diag 首行摘要 + 时间 + 会话，空态提示，退出码 0/1 约定。验证：真库运行 `bash scripts/harness/gate-status.sh` → 退出码 0 且展示各命令最新态
- [x] 5.2 新建 `scripts/harness/gate-status.smoke.sh`：临时库覆盖红态/绿态/空库/缺库四用例（含只读断言：跑完库字节不变）。验证：`bash scripts/harness/gate-status.smoke.sh` → 退出码 0

## 6. 测试

- [x] 6.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0（含 1.1–1.4 新增用例全绿）
- [x] 6.2 `bash scripts/harness/gate-status.smoke.sh` → 退出码 0
- [x] 6.3 `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0（事实库报告口径未被记账变化打破）
- [x] 6.4 `bash scripts/harness/concurrency-status.smoke.sh` → 退出码 0（归属机制回归）

## 7. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 与开发执行规范属 harness 机制/流程文档，随本 change 就地同步) -->

- [x] 7.1 `docs/reference/harness/pi-extensions.md`：提醒策略章节改为「边沿触发注入 + 状态可查（gate-status.sh）」，记录 concurrent-mixed 记账边沿与包锚点归属扩展
- [x] 7.2 `docs/reference/开发执行规范.md` §4 门禁分层中涉及「回合末复检提醒形态」的表述与新行为对齐（若有）；AGENTS.md 门禁配合四点措辞不变（其描述的行为未变）。**注：§4 经 grep 核实无「回合末复检提醒形态/单行摘要/未修」类表述（粘性重跑语义未变，描述仍准），仅在手动门禁处补 gate-status.sh 查询指引

## 8. 验证

- [x] V1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0（77+ 项断言全绿，含新增）
- [x] V2 `bash scripts/harness/gate-status.smoke.sh && bash scripts/harness/harness-retro.smoke.sh && bash scripts/harness/concurrency-status.smoke.sh` → 均退出码 0
- [x] V3 `bash scripts/harness/doc-impact.sh verify openspec/changes/aggregate-concurrent-gate-warns && bash scripts/harness/scenario-trace.sh openspec/changes/aggregate-concurrent-gate-warns` → 均退出码 0（scenario-trace 18/18 映射）
- [x] V4 `openspec validate aggregate-concurrent-gate-warns --strict` → 通过
- [x] V5 `bash scripts/harness/gate-status.sh`（真库）→ 退出码 0，输出含各命令红/绿态与时间戳
- [x] V6 `git status --short` → 本 change 变更仅含 `.pi/extensions/quality-gate.ts`、`.pi/extensions/tests/*`、`scripts/harness/gate-status.sh`、`scripts/harness/gate-status.smoke.sh`、两份文档、`openspec/changes/aggregate-concurrent-gate-warns/`；其他脏文件为并发改动不纳入
- [x] V7 `bash scripts/harness/harness-retro.sh --save-baseline` → 退出码 0（记录基线；回检指标：`quality-gate/concurrent-mixed` 7 天计数 41 → ≤10、④段失效组 3 → ≤1，观察窗口 7 天，下表留行）

| 回检指标 | 基线（2026-09-22） | 期望 | 观察窗口 |
| --- | --- | --- | --- |
| `concurrent-mixed` warn 计数/7天 | 41 | ≤10 | 上线后 7 天 |
| retro ④段软提醒失效组数 | 3/4 | ≤1 | 上线后 7 天 |
| 同指纹持续回合注入条数 | 每回合 ≥1 | 0 | smoke 断言（归档前） |

### Scenario→测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
| --- | --- |
| 首次失败输出完整块 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 同指纹持续零注入 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 红状态可经查询入口获知 | scripts/harness/gate-status.smoke.sh |
| 失败转绿输出收尾一行 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 粘性重跑语义不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 包级编译失败且目录全属外部时判外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 包锚点目录含本会话文件时维持分级 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 包目录无归属匹配时保守回退 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 混合归属失败标 [并发] 不标 [回归] | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 混合归属照进粘性重跑 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 纯归属两极不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归因信号缺席时保守回退 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 同指纹多回合至多一条记账 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 指纹变化后允许再记 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 展示各命令最新红绿态 | scripts/harness/gate-status.smoke.sh |
| 库缺失时报错退出 | scripts/harness/gate-status.smoke.sh |
| 查询全程只读 | scripts/harness/gate-status.smoke.sh |
| 无任何门禁记账时的空态 | scripts/harness/gate-status.smoke.sh |
