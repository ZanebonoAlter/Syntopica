## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十类事件：`session.start`（90 天）、`constraint.inject`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`subagent.complete`（30 天）、`mode.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件（完成回填以追加新事件表达，MUST NOT 改写既有 dispatch 行）。

#### Scenario: TTL 分级清扫

- **WHEN** 开库时存在 31 天前的 constraint.inject、policy.decision 行与 91 天前的 session.start 行
- **THEN** 过期的 constraint.inject、policy.decision 被删除，91 天前的 session.start 被删除，pin.write 永久保留

#### Scenario: 事件追加不可变

- **WHEN** 事件已写入后再次开库
- **THEN** 除 TTL/保险丝外既有行的 ts、session_id、kind、change、payload 不被任何 API 改写

#### Scenario: spill.write 事件随词汇扩展落库

- **WHEN** spill 扩展成功 spill 一次工具结果
- **THEN** events.db 新增一条 kind 为 `spill.write` 的事件行，payload 含工具名与字节数；31 天后被 TTL 清扫（无 DB schema 迁移，kind 为 TEXT 列）

#### Scenario: subagent.complete 随词汇扩展落库

- **WHEN** 后台子线程完成，harness-telemetry 追加完成事件
- **THEN** events.db 新增一条 kind 为 `subagent.complete` 的事件行（既有 `subagent.dispatch` 行内容不变）；31 天后被 TTL 清扫

#### Scenario: policy.decision 随词汇扩展落库

- **WHEN** 任一纳管策略扩展产生显著裁决
- **THEN** events.db 新增一条 kind 为 `policy.decision` 的事件行；31 天后被 TTL 清扫，既有数据库无需 schema 迁移

## ADDED Requirements

### Requirement: 策略显著裁决统一记账

spec-gate、quota-gate、test-scope-guard SHALL 将显著裁决追加为 `policy.decision` 事件。payload MUST 含 `policy`、`action`、`reasonCode`：`policy` 限定为稳定扩展标识，`action` 限定为 `block | warn | bypass | fail-open`，`reasonCode` MUST 是稳定、非空、kebab-case 的有界代码；按需附加的 `target` MUST 是不含密钥、完整命令和远端响应正文的有界摘要，`durationMs` 若存在 MUST 为非负数。事件的 change 列 SHALL 优先绑定裁决明确指向的 change，否则使用当前可检测的活跃 change，无法确定时为 null。

普通成功放行 MUST NOT 写 `policy.decision`；quality-gate 与 entry-gate 继续使用 `gate.check`，MUST NOT 为同一裁决重复写 `policy.decision`。记账失败 MUST NOT 改变原策略的放行、提醒、阻断或 fail-open 结果。

#### Scenario: spec-gate 阻断归档被记录

- **WHEN** spec-gate 因归档前检查失败阻断 `openspec archive <change>`
- **THEN** 追加一条 policy=spec-gate、action=block 的 policy.decision，reasonCode 表示归档检查失败且 change 绑定被归档 change

#### Scenario: spec-gate 显式豁免被记录

- **WHEN** 归档命令通过 `--force` 或 `SPEC_GATE_BYPASS=1` 绕过检查
- **THEN** 追加一条 policy=spec-gate、action=bypass 的 policy.decision；放行行为保持不变

#### Scenario: quota-gate 阻断与 fail-open 被区分

- **WHEN** quota-gate 分别因额度不足阻断一次派发、因额度查询失败放行一次派发
- **THEN** 分别追加 action=block 与 action=fail-open 的 policy.decision，target 仅含 provider 等安全摘要，不含 API key 或响应正文

#### Scenario: test-scope 软硬模式被记录

- **WHEN** test-scope-guard 在 soft 模式提醒一次、在 hard 模式阻断一次非归档语境的全量测试
- **THEN** 分别追加 action=warn 与 action=block、reasonCode=full-go-test 的 policy.decision

#### Scenario: 正常放行零记录

- **WHEN** spec-gate 的归档检查全部通过、quota-gate 判定额度充足，或命令未命中 test-scope-guard
- **THEN** 不产生 policy.decision 事件

#### Scenario: 记账故障不改变裁决

- **WHEN** events.db 不可写且策略本应阻断、提醒、豁免或 fail-open
- **THEN** 策略仍执行原裁决，记账仅走既有 fail-loud/fail-safe 错误路径
