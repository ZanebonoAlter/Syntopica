# harness-fact-log Specification

## Purpose

事件账本词汇与归属扩展：subagent.dispatch 事件开始携带 change 归属（add-review-trace-gate，与既有 session.rollup 快照同源的 detectActiveChange 判定），让 retro --lifecycle 的 review 覆盖统计有米下锅（考古基线：169 条 dispatch 事件 change 归属 0 条，complete 回填已带而 dispatch 本体漏带）。

## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十三类事件：`session.start`（90 天）、`session.rollup`（90 天）、`constraint.inject`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`subagent.complete`（30 天）、`mode.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）、`edit.map`（30 天）、`change.archive`（30 天）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件（完成回填以追加新事件表达，MUST NOT 改写既有 dispatch 行）。

`edit.map` 事件由 quality-gate 在 turn_end 聚合追加：change 列为会话绑定的 change，payload 含该 change 累计编辑路径集合；聚合语义（冲突标记、无档会话不计入）见 `concurrent-change-coordination` capability。

`session.rollup` 事件由 harness-telemetry 追加快照：change 列为快照写入时刻会话绑定的 change（可空）；查询侧对同一 session_id MUST 取最新一条作为终值（中间快照不参与聚合），语义对齐 `edit.map` 的快照覆盖取终值约定。

`subagent.dispatch` 事件由 harness-telemetry 追加：change 列为派发落账时刻的活跃 change（与 session.rollup 快照同源的 detectActiveChange 判定；解析不到为 null）。后台派发的 agentId→change 绑定快照与 subagent.complete 回填共用同一判定结果（agentChangeBindings）；dispatch 与 complete 两侧 change 值以各自落账时刻为准，不做跨时刻一致性承诺（会话中途切档属正常）。既有无 change 归属的历史 dispatch 行不改写，归属统计自改造落地后有效。

#### Scenario: TTL 分级清扫

- **WHEN** 开库时存在 31 天前的 constraint.inject、policy.decision、edit.map、change.archive 行与 91 天前的 session.start 行
- **THEN** 过期的 constraint.inject、policy.decision、edit.map、change.archive 被删除，91 天前的 session.start 被删除，pin.write 永久保留

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

#### Scenario: change.archive 随词汇扩展落库

- **WHEN** `openspec archive <change>` 命令成功执行（未被 spec-gate 阻断且工具结果非错误）
- **THEN** events.db 新增一条 kind 为 `change.archive` 的事件行（payload 契约见「归档成功记账」）；31 天后被 TTL 清扫，既有数据库无需 schema 迁移

#### Scenario: dispatch 携带 change 归属

- **WHEN** detectActiveChange 解析出活跃 change（openspec/changes/ 下存在非 archive change 目录）时派发 Agent 子线程
- **THEN** events.db 新增一条 kind 为 `subagent.dispatch` 的事件行，change 列为该 change 名，payload 契约不变（仅新增归属列值）

#### Scenario: 无活跃 change 时 dispatch 归属可空

- **WHEN** openspec/changes/ 无非 archive change 目录（或目录检测失败）时派发 Agent 子线程
- **THEN** 新增 dispatch 行 change 列为 null，事件照常落库，不阻断派发
