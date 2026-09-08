# harness-fact-log Delta — coordinate-concurrent-changes

## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十一类事件：`session.start`（90 天）、`constraint.inject`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`subagent.complete`（30 天）、`mode.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）、`edit.map`（30 天）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件（完成回填以追加新事件表达，MUST NOT 改写既有 dispatch 行）。

`edit.map` 事件由 quality-gate 在 turn_end 聚合追加：change 列为会话绑定的 change，payload 含该 change 累计编辑路径集合；聚合语义（冲突标记、无档会话不计入）见 `concurrent-change-coordination` capability。

#### Scenario: TTL 分级清扫

- **WHEN** 开库时存在 31 天前的 constraint.inject、policy.decision、edit.map 行与 91 天前的 session.start 行
- **THEN** 过期的 constraint.inject、policy.decision、edit.map 被删除，91 天前的 session.start 被删除，pin.write 永久保留

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

#### Scenario: edit.map 随词汇扩展落库

- **WHEN** 绑定 change 的会话在 turn_end 检出新增/变化编辑路径
- **THEN** events.db 新增一条 kind 为 `edit.map` 的事件行（change 列为绑定 change，payload 含累计路径集合）；31 天后被 TTL 清扫，既有数据库无需 schema 迁移
