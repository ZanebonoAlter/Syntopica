# harness-fact-log Delta

## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十二类事件：`session.start`（90 天）、`session.rollup`（90 天）、`constraint.inject`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`subagent.complete`（30 天）、`mode.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）、`edit.map`（30 天）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件（完成回填以追加新事件表达，MUST NOT 改写既有 dispatch 行）。

`edit.map` 事件由 quality-gate 在 turn_end 聚合追加：change 列为会话绑定的 change，payload 含该 change 累计编辑路径集合；聚合语义（冲突标记、无档会话不计入）见 `concurrent-change-coordination` capability。

`session.rollup` 事件由 harness-telemetry 追加快照：change 列为快照写入时刻会话绑定的 change（可空）；查询侧对同一 session_id MUST 取最新一条作为终值（中间快照不参与聚合），语义对齐 `edit.map` 的快照覆盖取终值约定。

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

#### Scenario: session.rollup 随词汇扩展落库

- **WHEN** 会话 turn_end 满足 rollup 节流条件
- **THEN** events.db 新增一条 kind 为 `session.rollup` 的事件行（payload 契约见「session 效能汇总记账」）；91 天后被 TTL 清扫，既有数据库无需 schema 迁移

## ADDED Requirements

### Requirement: session 效能汇总记账（session.rollup）

harness-telemetry SHALL 从 pi 的 session jsonl 提取单会话效能汇总并写入 `session.rollup` 快照事件：payload MUST 含 `turns`（user 轮数）、`steps`（assistant 消息数）、`toolCalls`（工具结果数）、`tokens`（input/output/cacheRead/cacheWrite/total 五值）、`cost`（元，可空）、`durationSec`、`model`、`final`（布尔，终值标记）。写入路径有二：① turn_end 节流快照（节流条件实现自定义，但 MUST 保证会话最后一条快照与终值偏差有界）；② session_start 时回填 prev session（session.start payload 的 prev 指向的 jsonl 仍存在且账本中该 session 无 `final=true` 快照时，补写一条 `final=true` 终值）。jsonl 缺失、损坏或解析失败 MUST 零写入并 fail-open（不阻断任何扩展钩子），仅旁路告警。

#### Scenario: turn_end 节流快照落库

- **WHEN** 会话内 turn_end 多次触发且满足节流条件
- **THEN** 每次命中节流条件追加一条快照（`final=false`），payload 数值随会话推进单调不减（tokens 累计口径）

#### Scenario: 同 session 取最新一条即终值

- **WHEN** 同一 session_id 存在多条 session.rollup 快照
- **THEN** 查询侧聚合（含 retro 报告）仅取最新一条的数值，不累加中间快照

#### Scenario: session_start 回填 prev 终值

- **WHEN** 新会话启动且 prev session 的 jsonl 存在、账本中该 session 无 `final=true` 快照
- **THEN** 追加一条该 prev session 的 `final=true` 快照事件（session_id 为 prev 的 id，change 列可空）；prev jsonl 不存在时零写入

#### Scenario: 解析失败 fail-open

- **WHEN** session jsonl 不存在或某行不可解析
- **THEN** 不写入 rollup、不抛出、不阻断当次钩子的其余逻辑，仅 console 旁路告警

#### Scenario: rollup 不改写既有事件

- **WHEN** 回填 prev 终值时该 session 已有中间快照
- **THEN** 以追加新事件表达终值，既有快照行内容不变
