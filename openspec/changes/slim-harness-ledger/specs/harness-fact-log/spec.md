# harness-fact-log 规格增量

## MODIFIED Requirements

### Requirement: 事件类型词汇与保留期

事实库 SHALL 支持十六类事件：`session.start`（90 天）、`session.rollup`（90 天）、`constraint.inject`（30 天）、`constraint.read`（30 天）、`pin.write`（永久）、`pin.read`（30 天）、`gate.check`（30 天）、`subagent.dispatch`（30 天）、`stage.set`（30 天）、`spill.write`（30 天）、`policy.decision`（30 天）、`edit.map`（30 天）、`change.archive`（30 天）、`change.abandon`（30 天）、`mode.set`（legacy 已停写，30 天保留期自然消退）、`patrol.check`（30 天，测试欠账巡检）。每条事件 MUST 携带单调递增 id、ISO 8601 UTC 时间戳、session_id、kind，change 列可空。开库时 MUST 按 kind 分保留期清扫过期行；库文件超过 100MB 时 MUST 触发删最老一半的保险丝。除 TTL 清扫与保险丝外 MUST NOT 修改或删除既有事件。

`stage.set` 事件取代原 `mode.set` 事件（档位机制退役，见 `constraint-injection` capability 的 REMOVED Requirements）。payload 含 `stage`（`explore` / `propose` / `design` / `implement` / `verify` / `archive`）、`stageFamily`（`planning` / `implementation`，派生值）、`boundChange`（可空）与 `source`（归因来源，与原 `mode.set` 同枚举：`command` / `skill` / `edit-dir` / `recover` / `inherit` / `fallback`）。既有 `mode.set` 行 MUST NOT 改写或删除——读方 SHALL 在过渡期双读（先查 `stage.set`，无结果时查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族），30 天保留期自然消退后双读可移除。

`constraint.read` 事件由 constraint-injection 在 agent `read` 工具**成功完成**（tool_execution_end 且非 error）且命中约束文档路径时追加：change 列为读取时刻会话绑定的 change（可空），payload 含被读文档的仓库根相对路径与 boundChange；失败/未回执的读取不记。该事件是归档对账（`constraint-read-trace.sh`）的单一事实源——对账 MUST NOT 依赖其他信号推断「读过没读过」。

`edit.map` 事件由 quality-gate 在 turn_end 聚合追加：change 列为会话绑定的 change，payload 含该 change 累计编辑路径集合；合并后集合与该 change 已有快照逐项全等时 MUST NOT 追加新快照（未变化不落库）。聚合语义（冲突标记、无绑定会话不计入）见 `concurrent-change-coordination` capability。

`session.rollup` 事件由 harness-telemetry 追加快照：change 列为快照写入时刻会话绑定的 change（可空）；查询侧对同一 session_id MUST 取最新一条作为终值（中间快照不参与聚合），语义对齐 `edit.map` 的快照覆盖取终值约定。

`subagent.dispatch` 事件由 harness-telemetry 追加：change 列为派发落账时刻的活跃 change（与 session.rollup 快照同源的 detectActiveChange 判定；解析不到为 null）。`status` 字段 SHALL 表示**派发时刻**的快照（前台派发多为终态、后台派发恒为 `running`），MUST NOT 被读方当作终态；子线程终态与成本以该子会话自身的 `session.rollup` 终值为准（经 `parentSessionId` 归父）。既有无 change 归属的历史 dispatch 行不改写，归属统计自改造落地后有效。

#### Scenario: TTL 分级清扫

- **WHEN** 开库时存在 31 天前的 constraint.inject、policy.decision、edit.map、change.archive 行与 91 天前的 session.start 行
- **THEN** 过期的 constraint.inject、policy.decision、edit.map、change.archive 被删除，91 天前的 session.start 被删除，pin.write 永久保留

#### Scenario: 事件追加不可变

- **WHEN** 事件已写入后再次开库
- **THEN** 除 TTL/保险丝外既有行的 ts、session_id、kind、change、payload 不被任何 API 改写

#### Scenario: spill.write 事件随词汇扩展落库

- **WHEN** spill 扩展成功 spill 一次工具结果
- **THEN** events.db 新增一条 kind 为 `spill.write` 的事件行，payload 含工具名与字节数；31 天后被 TTL 清扫（无 DB schema 迁移，kind 为 TEXT 列）

#### Scenario: policy.decision 随词汇扩展落库

- **WHEN** 任一纳管策略扩展产生显著裁决
- **THEN** events.db 新增一条 kind 为 `policy.decision` 的事件行；31 天后被 TTL 清扫，既有数据库无需 schema 迁移

#### Scenario: edit.map 随词汇扩展落库

- **WHEN** 绑定 change 的会话在 turn_end 检出**新的**编辑路径（合并后集合相对已有快照发生变化）
- **THEN** events.db 新增一条 kind 为 `edit.map` 的事件行（change 列为绑定 change，payload 含累计路径集合）；路径集合未变化时零写入；31 天后被 TTL 清扫，既有数据库无需 schema 迁移

#### Scenario: session.rollup 随词汇扩展落库

- **WHEN** 会话 turn_end 满足 rollup 节流条件
- **THEN** events.db 新增一条 kind 为 `session.rollup` 的事件行（payload 契约见「session 效能汇总记账」）；91 天后被 TTL 清扫，既有数据库无需 schema 迁移

#### Scenario: change.archive 随词汇扩展落库

- **WHEN** `openspec archive <change>` 命令成功执行（未被 spec-gate 阻断且工具结果非错误）
- **THEN** events.db 新增一条 kind 为 `change.archive` 的事件行（payload 契约见「归档成功记账」）；31 天后被 TTL 清扫，既有数据库无需 schema 迁移

#### Scenario: dispatch 携带 change 归属

- **WHEN** detectActiveChange 解析出活跃 change（openspec/changes/ 下存在非 archive change 目录）时派发 Agent 子线程
- **THEN** events.db 新增一条 kind 为 `subagent.dispatch` 的事件行，change 列为该 change 名，payload 契约不变（仅新增归属列值）

#### Scenario: subagent.complete 随词汇扩展落库

- **WHEN** 旧版本曾写入 `subagent.complete` 行（历史账本），或读方在账本中查到该 kind
- **THEN** 该 kind 保持**只读可解析**（新版本 MUST NOT 再写入该事件），历史行 31 天后被 TTL 清扫

#### Scenario: 后台派发的 status 不承诺终态

- **WHEN** 后台派发一个子线程后该子线程完成
- **THEN** 既有 `subagent.dispatch` 行 MUST NOT 被改写（status 仍为 `running`）；子线程的终态与成本由该子会话的 `session.rollup` 终值提供

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

### Requirement: session 效能汇总记账（session.rollup）

harness-telemetry SHALL 从 pi 的 session jsonl 提取单会话效能汇总并写入 `session.rollup` 快照事件：payload MUST 含 `turns`（user 轮数）、`steps`（assistant 消息数）、`toolCalls`（工具结果数）、`tokens`（input/output/cacheRead/cacheWrite/total 五值）、`cost`（元，可空）、`durationSec`、`model`、`final`（布尔，终值标记）；SHALL 含 `models`（逐模型聚合 map：`{[modelId]: {tokens 五值, cost}}`，按 assistant 消息自身携带的 model 字段归属，无 model 字段的消息计入 `unknown` 桶）；子会话 SHALL 含 `parentSessionId`（从自身 jsonl session 头的 `parentSession` 提取的父会话 id；主会话无此头时省略该字段）。写入路径有二：① turn_end 节流快照（节流条件实现自定义，SHALL 为「每 20 回合或 token 增量超过 50%」量级，且 MUST 保证会话最后一条快照与终值偏差有界）；② session_start 时回填 prev session 终值——prev jsonl 的定位 SHALL 由 telemetry 自行扫描 sessions 目录（当前会话文件所在目录下、mtime 最新且文件名会话 id 非本会话的 `*.jsonl`；pi 的 session_start 事件 payload 从不携带 previousSessionFile，MUST NOT 依赖之），该 jsonl 仍存在且账本中该 session 无 `final=true` 快照时，补写一条 `final=true` 终值。jsonl 缺失、损坏或解析失败 MUST 零写入并 fail-open（不阻断任何扩展钩子），仅旁路告警。

#### Scenario: turn_end 节流快照落库

- **WHEN** 会话内 turn_end 多次触发且满足节流条件（第 20 的整数倍回合，或 token 增量超过 50%）
- **THEN** 每次命中节流条件追加一条快照（`final=false`），payload 数值随会话推进单调不减（tokens 累计口径）

#### Scenario: 未达阈值不写

- **WHEN** 会话仅完成 19 个 turn 且 token 增量未超过 50%
- **THEN** 不追加中间快照；会话结束时仍由 `final=true` 终值快照落库

#### Scenario: 同 session 取最新一条即终值

- **WHEN** 同一 session_id 存在多条 session.rollup 快照
- **THEN** 查询侧聚合（含 retro 报告）仅取最新一条的数值，不累加中间快照

#### Scenario: session_start 回填 prev 终值

- **WHEN** 新会话启动且同目录下存在另一会话的 jsonl（mtime 最新且非本会话）、账本中该 session 无 `final=true` 快照
- **THEN** 追加一条该 prev session 的 `final=true` 快照事件（session_id 为 prev 的 id，change 列可空）；目录下无其他 jsonl 时零写入

#### Scenario: 回填不依赖 pi 事件的 previousSessionFile

- **WHEN** session_start 事件的 payload 不含 previousSessionFile（实测从不携带）
- **THEN** 回填仍由自扫 sessions 目录路径独立完成，机制不受该字段缺失影响

#### Scenario: 解析失败 fail-open

- **WHEN** session jsonl 不存在或某行不可解析
- **THEN** 不写入 rollup、不抛出、不阻断当次钩子的其余逻辑，仅 console 旁路告警

#### Scenario: rollup 不改写既有事件

- **WHEN** 回填 prev 终值时该 session 已有中间快照
- **THEN** 以追加新事件表达终值，既有快照行内容不变

#### Scenario: models 按消息级 model 字段归属

- **WHEN** 同一会话先用模型 A 生成 3 条 assistant 消息（cost 合计 0.3 元）、再切模型 B 生成 2 条（cost 合计 0.1 元）
- **THEN** 终值快照 payload 的 `models` 含 A 与 B 两个键，各自 tokens/cost 等于该模型消息的累计值，且各桶之和等于会话总 cost

#### Scenario: 无 model 字段的消息计 unknown 桶

- **WHEN** 某条 assistant 消息带 usage 但不带 model 字段
- **THEN** 其 tokens/cost 计入 `models.unknown` 桶，不静默丢弃、不影响其他桶

#### Scenario: 子会话携带 parentSessionId

- **WHEN** 子线程会话（jsonl session 头含 parentSession 指向父会话文件）turn_end 满足节流条件
- **THEN** 其 rollup payload 含 parentSessionId（父会话 id）；父会话自身的 rollup 无该字段

## REMOVED Requirements

### Requirement: 子线程完成回填（subagent.complete）

**Reason**: 该 Requirement 的完成信号锚点（父线程主动调用 `get_subagent_result` 工具）在真实工作流中从不发生——后台子线程由 pi 主动推送完成消息，agent 不会再手动取结果。全库实测 `subagent.complete` 0 条，419 条 `background=true` 的派发全部停在 `running`，⑦ 效能看板的子线程 tokens/toolUses 恒为空（`samples=0`）。继续保留一个从不产出数据的采集分支，比没有更糟：它让报告看起来有维度、实际是空值。

**Migration**: 子线程成本/终态改由子会话自身的 `session.rollup` 终值提供——该事件已带 `parentSessionId`（覆盖率 98%），B' 每任务成本早已按此归父，复用同一路径即可；retro ⑦ 的「子线程 token 占比」改从 rollup 归父合计计算。`subagent.dispatch` 行保持 append-only 不变，`status` 明确为派发时刻快照（后台恒为 `running`，不承诺终态）。历史 dispatch 行不受影响；历史 `subagent.complete` 行（若有）随 30 天 TTL 自然清扫。
