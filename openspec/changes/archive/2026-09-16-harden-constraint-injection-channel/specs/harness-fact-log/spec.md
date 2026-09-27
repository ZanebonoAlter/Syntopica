# harness-fact-log Delta

## MODIFIED Requirements

### Requirement: 注入与 pin 记账（constraint-injection 自报）

constraint-injection SHALL 在实现档注入 explore-findings.md 时，按 `## `（二级标题）解析 pin 标题并自报 `pin.read` 事件（payload 含 title、change、doc 路径、是否 digest 模式）；同一会话内同一标题 MUST 只记一次（会话内去重，session_start 重置）。pin_finding 成功写入 md 后 SHALL 自报 `pin.write`（payload 含 title、topic/change、落盘路径；失败调用不记账）。research 语境（无激活档位）写盘时 MUST 在标题行后追加 `<!-- pin:<8hex> -->` 锚点作为持久身份。

`constraint.inject` SHALL 在注入内容**实际送达时**记账（payload 含 `{path, mode, reason, bytes}`，注入原因是排查"为何未注入"的数据源），**MUST NOT 每 turn 重复记账**（混合通道下：稳定层在档位切换/绑定修正导致注入块变化时记一次；动态层在 steer 消息实际发送时按消息内条目记；compact 后快照重发按快照内条目记一次，payload 附 `source:"compact-resend"` 区分）。同会话内字节与原因均未变化的条目 SHALL NOT 重复记账。

#### Scenario: pin.read 首次注入记账并会话内去重

- **WHEN** 同一会话内同一 pin 标题的 explore-findings 被多次送达（档位激活注入一次、findings 更新后 steer 消息再送一次）
- **THEN** pin.read 仅记首次一条

#### Scenario: pin.write 仅成功路径记账

- **WHEN** pin_finding 因配置缺失失败，随后一次成功写入
- **THEN** 失败调用不产生事件，成功调用产生一条 pin.write 且 payload 含最终落盘路径

#### Scenario: constraint.inject 记录命中原因

- **WHEN** 某文档的约束节经 steer 消息首次送达
- **THEN** 产生一条 constraint.inject 事件，payload 含完整路径、mode、命中原因与字节数

#### Scenario: 稳定层档位生命周期内不重复记账

- **WHEN** implementation 档绑定 change X 激活（稳定层注入记账一轮），此后 20 个 turn 稳定层字节不变
- **THEN** 稳定层条目零新增 constraint.inject（不再每 turn 重复记）

#### Scenario: compact 重发快照记账标记

- **WHEN** compaction 后约束快照重发，快照含 3 个文档条目
- **THEN** 产生 3 条 constraint.inject，payload 附 source=compact-resend

### Requirement: 档位记账（mode.set）

constraint-injection SHALL 在**档位或绑定变化的全部路径**自报 `mode.set` 事件：input 命令命中、tool_execution_start 的 skill 路径命中、写 change 目录的兜底绑定、resume/reload/startup 恢复、mtime 兜底显式化。payload 含 mode（`requirements` / `implementation`）、boundChange（可空）与 **source 字段**（`command` / `skill` / `edit-dir` / `recover` / `inherit` / `fallback`——绑定来源可归因，隐性绑定 MUST NOT 存在）。事件写入失败 MUST 不阻断档位激活本身（记账 fail-loud、注入照常）。`session_start{reason:"resume"}` 的恢复取数 MUST 复用既有查询 API；恢复语义（new/fork/reload 不恢复、change 不存在回落）由 constraint-injection capability 定义。

#### Scenario: 档位激活记账

- **WHEN** 用户输入 `/opsx-apply some-change` 激活 implementation 档
- **THEN** 产生一条 mode.set（payload 含 mode=implementation、boundChange=some-change、source=command），档位激活不因记账失败而中断

#### Scenario: 绑定修正追记

- **WHEN** implementation 档会话绑定为空，agent 写 `openspec/changes/another-change/tasks.md` 触发兑底绑定修正
- **THEN** 追记一条 mode.set（boundChange=another-change、source=edit-dir），此前事件不改写

#### Scenario: TTL 与既有事件兼容

- **WHEN** 升级后首次开库（既有事件已存在）
- **THEN** 开库校验与 DDL 幂等通过，mode.set 按写入正常入账，既有事件不受影响

#### Scenario: 恢复路径记账

- **WHEN** pi 重启后按同 sessionId 恢复档位为 implementation 绑定 X
- **THEN** 记一条 mode.set（payload source=recover）

#### Scenario: 记账失败不阻断激活

- **WHEN** 事实库写入异常（库锁/磁盘），档位命令命中
- **THEN** 档位照常激活、注入照常执行，console 报记账失败，会话不中断
