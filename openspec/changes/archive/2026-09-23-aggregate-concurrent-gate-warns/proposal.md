<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: 无（纯 harness change，不涉业务域代码） -->

## Why

quality-gate 的失败提醒是「每回合都有话说」：同指纹失败即使一字未变，每回合仍注入一行摘要（连续 ≥3 回合还追加「未修」标记），纯对话回合照发，成功侧转绿也插一条——体感就是「会话收尾后还在追着提醒，不管对错都插」。harness-retro ④段（2026-09-15~22 窗口）把 `quality-gate/concurrent-mixed` 41 次、`spec-gate/concurrent-dirty-tree` 31 次判为软提醒失效组：提醒密度已超过接收方可 action 的程度（「脏文件不是我的」→ 无事可做 → 下回合又来一遍）。

同期还暴露两处归因盲区（2026-09-22 现场，本会话对同一并发失败连收 5 轮 [回归]/⟳ steer）：并发 change 的在途测试文件是 `??` 未跟踪新文件，且失败输出以**包锚点**（`FAIL …/internal/dataenrichment/handler [build failed]`）呈现，解析不到 foreign 集合 → 按 spec 的保守策略 fail-open 成 [回归]，催修一个名下 0 个相关文件的会话。

## What Changes

1. **边沿触发注入（收敛提醒）**：同会话同 `(cmd, 失败指纹)` 的报告强度从「每回合单行摘要 + ≥3 回合『未修』标记」收紧为**状态未变零注入**；首次全文、指纹变化、转绿收尾（每失败段至多一次）保留。门禁命令的执行频率、触发集、粘性重跑语义、归档前全绿硬要求**均不变**——只改「注入什么、何时注入」。
2. **并发 warn 记账边沿化**：`policy.decision(reasonCode=concurrent-mixed)` 从每命中回合一条收紧为**每指纹会话内至多一条**（转绿或指纹/判性变化后可再记）。`spec-gate/concurrent-dirty-tree` 本就是每次归档尝试至多一条（按尝试边沿），不在本次改动面内。
3. **状态可查替代持续提醒**：新增 `scripts/harness/gate-status.sh`——只读 facts 库 `gate.check` 记账，人读展示各门禁命令最新红/绿状态、失败特征摘要与最近转绿时间；agent/用户随时手查，不依赖 turn_end 追着提醒。
4. **包锚点归属解析（补归因盲区）**：失败归属判定中，失败输出的包锚点（`# pkg` / `FAIL pkg` 形态）SHALL 解析为其目录下的文件路径集合参与 `mine/foreign` 判定——纯外部的包级失败因此能正确落 [外部]，不再 fail-open 成 [回归]。仍保持「P ⊆ foreign 才降级」的精确语义，不引入宽松推断。

## Capabilities

### New Capabilities

- `gate-status-query`: 门禁状态的人读查询入口（脚本行为契约：数据源、展示字段、只读安全）。

### Modified Capabilities

- `gate-failure-reporting`: 「失败指纹与重复抑制」的持续回合报告强度由单行摘要收紧为零注入，保底催修职责移交「状态可查 + 归档前全绿硬门禁」；「并发失败归属判定」补充包锚点到文件路径集合的解析规则。
- `gate-concurrency-control`: 「混合归属失败降级」的 `concurrent-mixed` 记账补充按指纹边沿的条数约束（同指纹会话内至多一条）。

## Impact

- 代码：`.pi/extensions/quality-gate.ts`（报告注入点 + 归属解析 + policy.decision 记账时机）；新增 `scripts/harness/gate-status.sh`。
- 文档：`docs/reference/harness/pi-extensions.md`（提醒策略与查询入口章节）、`docs/reference/开发执行规范.md` §4 门禁分层相关表述（如提及「回合末复检」提醒形态）——归档时经 `doc-impact.sh verify` 对账。
- 事实库：`policy.decision` 词汇表**不变**（reasonCode/action 不新增不删除）；预期 `concurrent-mixed` 7 天计数 41 → ≤10（回检指标）。
- 风险：提醒变稀后，若 agent 依赖 turn_end 提醒才知道红状态，改查 `gate-status.sh`；兜底不变——spec-gate 归档检查①全绿硬要求仍在，红状态漏不掉。
