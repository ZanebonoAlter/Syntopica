# harness-fact-log Specification

## Purpose
harness 层事实账本：以 `.pi/harness/events.db`（单表 append-only SQLite）记录约束注入、pin 写读、门禁检查、子线程派发等 harness 层事实，供跨会话审计与规则调优（模型零参与）。
（源设计：harness-facts-tier-a；八类事件词汇、安全开库、TTL 分级与失败白名单见 Requirements。）

## Requirements

### Requirement: 事实库存储与安全开库

系统 SHALL 在 `.pi/harness/events.db` 维护单表 append-only 事件账本（`events(id, ts, session_id, kind, change, payload)`），由 `lib/harness-log.ts` 作为唯一写入方。开库时 MUST 在任何写操作（含 `PRAGMA journal_mode`）之前校验 `application_id` 与 `user_version`：全新库（app_id=0 且无用户表）MUST 初始化魔数 `0x53594E54`、`user_version=1` 并建表；识别到他人库（app_id=0 但有用户表，或 app_id 非本应用）或未来版本（version 大于当前支持）MUST 拒绝打开并走 fail-loud 路径（返回失败、不静默降级、不重建）。写入 MUST 使用 WAL + synchronous=NORMAL + busy_timeout=5000。

#### Scenario: 首次开库初始化

- **WHEN** `.pi/harness/events.db` 不存在且 `logEvent` 被调用
- **THEN** 系统创建数据库，设置 application_id=0x53594E54、user_version=1，建立 events 表与三组索引，事件成功写入且可按 session_id 查回

#### Scenario: 拒绝他人未登记的库

- **WHEN** 路径指向一个已存在、application_id=0 且含用户表的 SQLite 文件
- **THEN** 系统不执行任何写 PRAGMA、不建表，开库失败（logEvent 返回 false），调用方收到一次性 fail-loud 通知

#### Scenario: 拒绝未来版本的库

- **WHEN** 库的 application_id 为本应用魔数但 user_version 大于当前支持的 schema 版本
- **THEN** 系统拒绝打开（返回失败），不降级写入

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

### Requirement: 注入与 pin 记账（constraint-injection 自报）

constraint-injection SHALL 在实现档注入 explore-findings.md 时，按 `## `（二级标题）解析 pin 标题并自报 `pin.read` 事件（payload 含 title、change、doc 路径、是否 digest 模式）；同一会话内同一标题 MUST 只记一次（会话内去重，session_start 重置）。pin_finding 成功写入 md 后 SHALL 自报 `pin.write`（payload 含 title、topic/change、落盘路径；失败调用不记账）。research 语境（无激活档位）写盘时 MUST 在标题行后追加 `<!-- pin:<8hex> -->` 锚点作为持久身份。每次注入送达时 SHALL 按 `{path, mode, reason, bytes}` 自报 `constraint.inject`——注入原因（档位绑定/关键词/规则）是排查"为何未注入"的数据源。

#### Scenario: pin.read 首次注入记账并会话内去重

- **WHEN** 实现档会话第 1 回合注入含 `## 告警表结构` 的 explore-findings.md，第 2 回合再次注入同名文件
- **THEN** 第 1 回合产生一条 pin.read（title=告警表结构），第 2 回合不再重复记账

#### Scenario: pin.write 仅成功路径记账

- **WHEN** pin_finding 因配置缺失失败，随后一次成功写入

- **THEN** 失败调用不产生事件，成功调用产生一条 pin.write 且 payload 含最终落盘路径

#### Scenario: constraint.inject 记录命中原因

- **WHEN** 实现档注入命中文档 X（档位绑定）
- **THEN** 产生一条 constraint.inject 事件，payload 含 X 的完整路径、mode、命中原因与字节数

### Requirement: 门禁记账（gate.check）

quality-gate SHALL 对每条实际执行的门禁命令（golangci-lint / go vet / go build / change-scope 判定的 domain go test / pnpm lint）各记一条 `gate.check` 事件，payload 含 cmd、phase、ok、ms；ok=false 时附带截断摘要 diag（≤512B、单行）。

ok=true 事件 SHALL 采样记账（成功运行诊断价值趋零，全量记账造成写放大）：以（session, cmd）为粒度维护连续成功计数——**会话内该命令的首个成功**及失败转绿后的首个成功 MUST 记账（翻转锚点，payload 附翻转标记），其后每 N 次连续成功记 1 条（N 可配置，缺省 5），payload 附采样标记与 N；连续成功段被失败打断后重新起算。ok=false MUST 全量记账，不采样。失败率统计 SHALL 以「失败条数 + 记到的成功条数按锚点 1、采样条 N 加权还原」为分母口径。

同根因短路：golangci-lint 输出含编译失败特征（typechecking error 族）时，quality-gate MAY 跳过本轮必然失败的 go vet 与 domain go test（跳过即未执行）；被跳过的命令 MUST NOT 记 gate.check（未执行零记账，与「纯文档回合零记录」同族语义）。

事件 MUST 绑定 `detectActiveChange` 检测到的活跃 change（无则 null），该检测 MUST 与 constraint-injection 共享同一实现。纯文档/纯对话回合（未触发门禁命令）MUST 零记录。修复回合不单独记账——失败→通过由相邻 gate.check 的 ok 翻转表达。

diag 提取 SHALL 失败特征优先：从命令输出（stdout+stderr 拼接）中优先取首个含失败特征关键词（如 `FAIL`、`error`、`# <pkg>`、`exit` 等关键词表）的行；无任何命中时回退首个非空行。截断规范不变（单行、剥控制字符、≤512 字节）。提取 MUST 确定性（相同输出字节产生相同 diag）。该优先规则仅适用于 gate.check 记账路径；子线程失败白名单的 diag 规范（首个非空错误行）不受影响。

#### Scenario: 门禁命令失败记账

- **WHEN** 某回合 golangci-lint exit 1、go vet exit 0（非短路场景）
- **THEN** 产生两条 gate.check：一条 ok=false 含 512B 内截断 diag，一条 ok=true（按采样规则判定是否落库），两条 change 列均绑当前活跃 change

#### Scenario: 失败全量记账不采样

- **WHEN** 同一（session, cmd）连续 6 次失败
- **THEN** 产生 6 条 ok=false 事件，每条含 diag，无一条被采样跳过

#### Scenario: 转绿翻转锚点必记

- **WHEN** 某（session, cmd）连续失败 3 次后首次成功，随后又连续成功 5 次（N=5）
- **THEN** 转绿首个成功记 1 条（payload 含翻转标记，计 1），其后再记 1 条采样事件（payload 含采样标记与 N=5）；该连续成功段共 6 次运行落库 2 条

#### Scenario: 分母按采样口径还原

- **WHEN** 某（session, cmd）落库 6 条 ok=false、1 条翻转锚点 ok=true、2 条采样 ok=true（N=5）
- **THEN** 统计侧还原总执行次数 = 6 + 1 + 2×5 = 17，失败率 = 6/17（锚点计 1、采样条按 N 加权）

#### Scenario: 同根因短路未执行不记账

- **WHEN** 某回合 golangci-lint exit 1 且输出含 typechecking error，quality-gate 据此跳过 go vet 与 domain go test
- **THEN** 该回合仅产生 1 条 gate.check（golangci-lint，ok=false 含 diag）；go vet / go test 无事件（未执行零记账）

#### Scenario: 未运行不记账

- **WHEN** 回合只改了 docs/ 下 markdown（门禁放行，未执行任何命令）
- **THEN** 该回合不产生任何 gate.check 事件

#### Scenario: diag 失败特征优先（stdout 噪声行不掩盖真实错误）

- **WHEN** golangci-lint exit 1，stdout 首行为 `0 issues.`，stderr 后续行含编译错误（如 `# syntopica-backend/internal/topicgraph/service`）
- **THEN** ok=false 的 gate.check 事件 diag 含编译错误特征行（`# syntopica-backend/...`），而非 `0 issues.`

#### Scenario: go test 记账不丢 FAIL 行

- **WHEN** `go test -short` exit 1，输出首行为父包正常行 `? pkg [no test files]`，后续行含 `FAIL pkg [build failed]`
- **THEN** diag 含 `FAIL` 特征行，可事后审计还原真实失败原因

### Requirement: 子线程失败白名单

harness-telemetry SHALL 在 Agent 工具 tool_result 为失败时，通过纯函数映射器产出有界结构化失败事实附加到 subagent.dispatch payload：`failure = { stage, category, exitLike, diag }`。category MUST 限定白名单枚举 `quota-block | timeout | gate-fail | model-error | tool-error | unknown`，按有序关键词表首中映射，无法映射时 MUST 落 unknown 且不复制原始错误文本。diag MUST 为首个非空错误行的单行截断（≤512 字节，剥控制字符）。stage MUST 按证据判定：未观察到 tool_call 起点（如门禁拦截）→ dispatch；起点存在且执行失败 → run。成功与用户取消 MUST NOT 产出 failure 对象。程序逻辑 MUST NOT 按 category 名称分支（统计与展示除外）。

#### Scenario: 白名单命中

- **WHEN** Agent 派发被 quota-gate 拦截，错误文本含额度/剩余/重置特征
- **THEN** subagent.dispatch 带 failure `{stage:"dispatch", category:"quota-block", ...}`，diag 为截断后的安全摘要

#### Scenario: 无法映射落 unknown

- **WHEN** 失败错误文本不含任何白名单关键词
- **THEN** category 为 unknown，原始错误文本不进入 payload（仅 512B 内 diag）

#### Scenario: 成功不产出失败事实

- **WHEN** Agent 派发成功返回（isError=false）
- **THEN** subagent.dispatch payload 无 failure 对象

### Requirement: 查询接口

`lib/harness-log.ts` SHALL 导出 `queryBySession(cwd, sessionId, kinds?)` 与 `queryByChange(cwd, changeName)`，按 id 时间序返回行；查询失败 MUST 返回空数组并 console.error（fail-loud 但不抛出）。使用统计（pin 使用次数、最后使用时间、从未使用的 pin）SHALL 通过对既有事件的 SQL 聚合表达，MUST NOT 引入新表或物化列。

#### Scenario: 按会话查询注入史

- **WHEN** 调用 queryBySession 传当前 sessionId 与 kinds=["constraint.inject"]
- **THEN** 返回该会话全部注入事件按 id 升序排列

#### Scenario: pin 使用聚合

- **WHEN** 对 events 执行 `kind='pin.read' GROUP BY json_extract(payload,'$.title')` 聚合并与 pin.write 标题集合作差
- **THEN** 得出每个 pin 的使用次数、最后使用时间与从未被注入的 pin 清单，无需额外存储

### Requirement: 档位记账（mode.set）

constraint-injection SHALL 在档位激活（input 命令命中或 tool_execution_start 的 skill 路径命中）时自报 `mode.set` 事件，payload 含 mode（`requirements` / `implementation`）与 boundChange（可空）；档位绑定 change 修正（提及 change / 写 change 目录）时 SHALL 追记新事件以反映最新绑定。事件写入失败 MUST 不阻断档位激活本身（记账 fail-loud、注入照常）。`session_start{reason:"resume"}` 的恢复取数 MUST 复用既有查询 API；恢复语义（new/fork/reload 不恢复、change 不存在回落）由 constraint-injection capability 定义。

#### Scenario: 档位激活记账

- **WHEN** 用户输入 `/opsx-apply some-change` 激活 implementation 档
- **THEN** 产生一条 mode.set（payload 含 mode=implementation、boundChange=some-change），档位激活不因记账失败而中断

#### Scenario: 绑定修正追记

- **WHEN** implementation 档会话中 agent 写 `openspec/changes/another-change/tasks.md` 使绑定修正
- **THEN** 追记一条 mode.set（boundChange=another-change），此前事件不改写

#### Scenario: TTL 与既有事件兼容

- **WHEN** 升级后首次开库（既有六类事件已存在）
- **THEN** 开库校验与 DDL 幂等通过，mode.set 按写入正常入账，既有事件不受影响

### Requirement: 子线程完成回填（subagent.complete）

harness-telemetry SHALL 在后台派发的子线程真实结束时追加一条 `subagent.complete` 事件，payload 至少含 agentId、status、ms、tokens、toolUses、turnCount、isError，经 agentId 与派发时的 `subagent.dispatch` 事件关联。同一 agentId 的完成事件至多一条（重复完成信号幂等去重）。既有 `subagent.dispatch` 事件 MUST NOT 改写（append-only）。完成信号涵盖成功 / 失败 / 用户取消；取消 SHALL 记 status=cancelled。未观察到完成信号（如进程退出断链）MUST NOT 伪造完成事件——断链本身可经「有 dispatch 无 complete」的查询发现。记账失败 MUST 不影响子线程结果装配（fail-safe）。change 归属 SHALL 复用派发时的绑定（完成回填不重检测）。

#### Scenario: 后台完成回填

- **WHEN** 后台子线程（派发时记 status=background 的 dispatch 事件）成功结束
- **THEN** 追加一条 subagent.complete（payload 含 agentId、ms、tokens、toolUses），原 dispatch 行不变，两事件经 agentId 可关联审计

#### Scenario: 取消记 cancelled

- **WHEN** 用户在后台子线程结束前取消派发
- **THEN** 追加一条 subagent.complete，status=cancelled，isError=false

#### Scenario: 断链不伪造

- **WHEN** 子线程派发后进程退出，完成信号缺失
- **THEN** 不产生 subagent.complete 事件；查询侧按 agentId 无 complete 对应可发现断链，不阻塞不报错

#### Scenario: edit.map 随词汇扩展落库

- **WHEN** 绑定 change 的会话在 turn_end 检出新增/变化编辑路径
- **THEN** events.db 新增一条 kind 为 `edit.map` 的事件行（change 列为绑定 change，payload 含累计路径集合）；31 天后被 TTL 清扫，既有数据库无需 schema 迁移
