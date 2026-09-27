## Purpose

门禁状态的只读人读查询入口：让用户与 agent 随时主动查看各门禁命令的最新红绿状态与失败摘要，替代 turn_end 的逐回合持续提醒（配合 gate-failure-reporting 的边沿触发注入收敛）。

## ADDED Requirements

### Requirement: 状态查询只读展示

仓库 SHALL 提供门禁状态查询入口（`bash scripts/harness/gate-status.sh`），展示本仓库 facts 库中各门禁命令的最新状态：命令名、红/绿态、最近失败特征摘要（diag 首行，截断至合理长度）、记账时间与所属会话标识。

数据源 MUST 为 `.pi/harness/events.db` 的 `gate.check` 记账，按会话与命令取最新一条；因门禁记账协议中转红全量记录、转绿为必记翻转锚点，最新条目 SHALL 忠实反映当前红绿态。脚本 MUST 只读：不写账本、不写任何文件、不触发任何门禁命令执行（与 `concurrency-status.sh` 的只读安全口径一致）。

退出码约定：查询成功（无论展示红态或绿态）为 0；账本库缺失或损坏时为 1 并在 stderr 给出说明，MUST NOT 静默成功。

#### Scenario: 展示各命令最新红绿态

- **WHEN** 账本中某命令最近一次 gate.check 为失败、另一命令最近一次为转绿成功
- **THEN** 输出中前者标红态并附失败特征摘要与时间，后者标绿态与时间，退出码 0

#### Scenario: 库缺失时报错退出

- **WHEN** `.pi/harness/events.db` 不存在或无法打开
- **THEN** stderr 输出说明信息，退出码 1，不产生误导性的状态展示

#### Scenario: 查询全程只读

- **WHEN** 运行状态查询入口
- **THEN** 账本库内容不被修改、不产生新文件、不执行 lint/test/vet 等门禁命令

#### Scenario: 无任何门禁记账时的空态

- **WHEN** 账本可读但不存在任何 gate.check 记账（全新库）
- **THEN** 输出「暂无门禁记账」类空态提示，退出码 0
