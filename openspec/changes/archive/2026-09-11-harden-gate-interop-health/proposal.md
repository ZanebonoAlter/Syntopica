<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

harness 事实库考古（18333 条 gate.check，1265 失败）显示 ~107 次失败是 WSL interop 环境故障（`UtilAcceptVsock: accept4 failed 110`），集中在 8/28、9/3、9/5 三次故障期，9月3日单日 83 次、横跨两个 session 持续 40+ 分钟。后端门禁必须走 cmd.exe interop 调 Windows Go 工具链（WSL 侧 Go 1.18 认不了 go 1.25 的 go.mod），vsock 通道因此成为单点：故障期一轮门禁 5 条命令 × ~10s 白等 ≈ 50s/turn，且 exit≠0 无差别进入 `stickyFailures` 粘性重跑、被标成 [回归] steer，诱导 agent 去"修"不存在的代码问题。需要让门禁能分清"代码烂"和"环境烂"。

## What Changes

- **事前健康探测**：quality-gate 在 turn_end 执行 cmd.exe 链路门禁（后端 Go 三件套 + 域测试 + pnpm lint）前，先跑廉价探测（`cmd.exe /C echo ok`，短超时，正常实测 ~40ms）；探测失败则本轮 cmd.exe 链路门禁整体短路跳过（fail-open），不产生假失败。
- **事后环境故障识别**：gate.check 失败时识别 diag 中的 WSL interop 故障特征（`UtilAcceptVsock` / `WSL (… - ) ERROR`），命中则归类为环境故障：**不进入 stickyFailures 粘性集合**，steer 消息明确提示为 WSL 环境问题而非代码回归（兜底探测通过但执行中途挂掉的场景）。
- **（wsl环境）显式标注**：走 cmd.exe interop 链路的门禁命令失败时，steer 提示显式标注执行链路（wsl环境），让 agent 一眼分清该检查跑在跨系统 interop 上。
- **探测短路记账**：探测失败短路记一条 `policy.decision`（policy=quality-gate，action=fail-open，reasonCode=interop-down），供事后考古一眼区分环境故障期。
- **不改变**：门禁命令本身仍走 cmd.exe 调 Windows 工具链；探测健康时门禁行为、记账采样、短路哨兵（lint typechecking 同根因短路）逻辑不变；故障期不自动重试（实测故障持续 40+ 分钟，重试无意义）。

## Capabilities

### New Capabilities

- `gate-interop-health`: quality-gate 的 cmd.exe interop 链路健康防护——事前探测短路、事后环境故障识别与粘性豁免、steer 链路标注、短路记账。

### Modified Capabilities

- `harness-fact-log`: "策略显著裁决统一记账" requirement 的适用范围扩展——quality-gate 的 interop 探测短路新增 policy.decision 记账（fail-open / interop-down），并保持"同一裁决不双写"（被短路跳过的门禁命令不逐条记 gate.check）。

## Impact

- 代码：`.pi/extensions/quality-gate.ts`（gitignored，快照同步 `docs/research/`）——turn_end 流程新增探测步骤、gateLog/steer 构造新增环境故障分支、stickyFailures 写入加豁免。
- 事件词汇：`policy.decision` 新增 reasonCode `interop-down`（kebab-case 白名单扩展）。
- 文档：`docs/research/harness事实库.md` 词汇表、`docs/reference/constraints-index.md` 若引用门禁分层则小改（§4.1 分层不变，仅补充环境故障路径）。
- 无前端、无数据库 schema、无 API 变更。
