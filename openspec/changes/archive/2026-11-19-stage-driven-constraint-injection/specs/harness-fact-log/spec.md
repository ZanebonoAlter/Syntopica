## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十七类事件：`session.start`（90 天）、`session.rollup`（90 天）、`constraint.inject`（30 天）、`constraint.read`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`subagent.complete`（30 天）、`stage.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）、`edit.map`（30 天）、`change.archive`（30 天）、`change.abandon`（30 天）、`mode.set`（legacy 已停写，30 天保留期自然消退）、`patrol.check`（30 天，测试欠账巡检）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件（完成回填以追加新事件表达，MUST NOT 改写既有 dispatch 行）。

`stage.set` 事件取代原 `mode.set` 事件（档位机制退役，见 `constraint-injection` capability 的 REMOVED Requirements）。payload 含 `stage`（`explore` / `propose` / `design` / `implement` / `verify` / `archive`）、`stageFamily`（`planning` / `implementation`，派生值）、`boundChange`（可空）与 `source`（归因来源，与原 `mode.set` 同枚举：`command` / `skill` / `edit-dir` / `recover` / `inherit` / `fallback`）。既有 `mode.set` 行 MUST NOT 改写或删除——读方 SHALL 在过渡期双读（先查 `stage.set`，无结果时查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族），30 天保留期自然消退后双读可移除。

`constraint.read` 事件由 constraint-injection 在 agent `read` 工具**成功完成**（tool_execution_end 且非 error）且命中约束文档路径时追加：change 列为读取时刻会话绑定的 change（可空），payload 含被读文档的仓库根相对路径与 boundChange；失败/未回执的读取不记。该事件是归档对账（`constraint-read-trace.sh`）的单一事实源——对账 MUST NOT 依赖其他信号推断「读过没读过」。

`edit.map` 事件由 quality-gate 在 turn_end 聚合追加：change 列为会话绑定的 change，payload 含该 change 累计编辑路径集合；聚合语义（冲突标记、无绑定会话不计入）见 `concurrent-change-coordination` capability。

`session.rollup` 事件由 harness-telemetry 追加快照：change 列为快照写入时刻会话绑定的 change（可空）；查询侧对同一 session_id MUST 取最新一条作为终值（中间快照不参与聚合），语义对齐 `edit.map` 的快照覆盖取终值约定。

`subagent.dispatch` 事件由 harness-telemetry 追加：change 列为派发落账时刻的活跃 change（与 session.rollup 快照同源的 detectActiveChange 判定；解析不到为 null）。后台派发的 agentId→change 绑定快照与 subagent.complete 回填共用同一判定结果（agentChangeBindings）；dispatch 与 complete 两侧 change 值以各自落账时刻为准，不做跨时刻一致性承诺（会话中途切阶段属正常）。既有无 change 归属的历史 dispatch 行不改写，归属统计自改造落地后有效。

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

#### Scenario: stage.set 事件入库

- **WHEN** 会话阶段从 `propose` 切换为 `implement` 并绑定 change X
- **THEN** 事实库追加一条 `stage.set`，payload 含 stage=implement、stageFamily=implementation、boundChange=X

#### Scenario: 旧 mode.set 行仍可读

- **WHEN** 某 session_id 只有改造前的 `mode.set`（payload mode=requirements）而无 `stage.set`
- **THEN** 按该 session 查阶段族的读方返回 `planning`，MUST NOT 报错

#### Scenario: constraint.read 事件入库

- **WHEN** agent 成功读取 `docs/reference/flow/daily-report.md`（read 成功回执）
- **THEN** 事实库追加一条 `constraint.read`，payload 含该路径，change 列为会话绑定的 change（可空）；发起但失败的读取不追加

### Requirement: 阶段记账（stage.set）

constraint-injection SHALL 在**阶段或绑定变化的全部路径**自报 `stage.set` 事件：阶段 marker 变更、写 change 目录的兜底绑定、resume/reload/startup 恢复、父子继承、mtime 兜底显式化。payload 含 stage（`explore` / `propose` / `design` / `implement` / `verify` / `archive`）、stageFamily（`planning` / `implementation`，派生值）、boundChange（可空）与 **source 字段**（`command` / `skill` / `edit-dir` / `recover` / `inherit` / `fallback`——绑定来源可归因，隐性绑定 MUST NOT 存在）。事件写入失败 MUST NOT 阻断阶段激活本身（记账 fail-loud、注入照常）。`session_start{reason:"resume"}` 的恢复取数 MUST 复用既有查询 API；恢复语义（new/fork/reload 不恢复、change 不存在回落）由 constraint-injection capability 定义。

既有 `mode.set` 行 MUST NOT 改写或删除（30 天保留期自然消退）；读方 SHALL 过渡期双读（先查 `stage.set`，无结果时查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族）。

#### Scenario: 档位激活记账

- **WHEN** 会话阶段切换为 `implement` 并绑定 change `some-change`
- **THEN** 产生一条 stage.set（payload 含 stage=implement、stageFamily=implementation、boundChange=some-change、source=command），阶段激活不因记账失败而中断

#### Scenario: 绑定修正追记

- **WHEN** implementation 族阶段的会话绑定为空，agent 写 `openspec/changes/another-change/tasks.md` 触发兜底绑定修正
- **THEN** 追记一条 stage.set（boundChange=another-change、source=edit-dir），此前事件不改写

#### Scenario: TTL 与既有事件兼容

- **WHEN** 升级后首次开库（既有事件已存在）
- **THEN** 开库校验与 DDL 幂等通过，stage.set 按写入正常入账，既有 mode.set 行保留不改写

#### Scenario: 恢复路径记账

- **WHEN** pi 重启后按同 sessionId 恢复阶段为 implement 绑定 X
- **THEN** 记一条 stage.set（payload source=recover）

#### Scenario: 记账失败不阻断激活

- **WHEN** 事实库写入异常（库锁/磁盘），档位命令命中
- **THEN** 档位照常激活、注入照常执行，console 报记账失败，会话不中断

## RENAMED Requirements

- FROM: `### Requirement: 档位记账（mode.set）`
- TO: `### Requirement: 阶段记账（stage.set）`
