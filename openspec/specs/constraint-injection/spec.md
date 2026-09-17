# constraint-injection Specification

## Purpose
TBD - created by archiving change port-constraint-injection. Update Purpose after archive.

## Requirements

### Requirement: 档位识别与 change 绑定

extension SHALL 通过 `input` 事件识别阶段命令设置会话内档位（`requirements` / `implementation`），命令集覆盖本仓 openspec 斜杠命令（`/opsx-*`、`/skill:openspec-*`）与对应 skill 文件读取（`tool_execution_start` 的 read 路径命中 skill 目录）。

档位 SHALL 绑定活跃 change，绑定规则分档位：**输入提及优先**（命令参数/输入文本/写 change 目录文件时修正）；**mtime 最新兜底仅 implementation 档**（apply/verify/archive 无参语境）；requirements 档（explore/propose 新想法语境）SHALL NOT mtime 兜底——未明确提及时不绑定（关键词命中源不含无关 change 文本，pin_finding 落 research 库）。未激活档 SHALL 不显示、不分析任何 change。绑定 change 归档或删除后 SHALL 自动回落未激活档。

**写 change 目录的绑定修正在多 change 并行下 SHALL 条件化**（防抢绑污染，实测 2026-09-16：并行工具调用一毫秒内 4 连绑、read 参考文档即被抢绑）：

- read 工具 SHALL NOT 触发绑定修正（read 其他 change 目录属参考性访问）；
- write/edit 命中 `openspec/changes/<name>/` 路径时，仅当当前绑定不健康（未绑定、或绑定 change 目录已消失）SHALL 兜底绑上 `<name>`；当前绑定健康时 SHALL NOT 抢绑（多 change 轮转会话的归因不因触碰其他 change 目录而漂移）；
- 同一 turn 内绑定 SHALL 锁定：turn 内首个生效的绑定事件定绑，同 turn 后续工具调用 SHALL NOT 再切换绑定（消除并行工具调用完成顺序不定导致的非确定绑定；下一 turn 首个绑定事件可再切换）。

档位状态 SHALL 持久化：**所有绑定与档位变化路径**（input 命令命中、skill 路径命中、写 change 目录兜底、resume/reload/startup 恢复、子线程继承、mtime 兜底显式化）MUST 向事实库记 `mode.set` 事件（payload 含 mode 与绑定 change，并附 `source` 字段区分激活来源：`command` / `skill` / `edit-dir` / `recover` / `inherit` / `fallback`）——隐性绑定（无记账的绑定变化）MUST NOT 存在，任何时刻的绑定状态都可从事实库归因。会话边界的恢复按 reason 区分，**三条恢复路径（resume / reload / startup）均 SHALL 仅按同 sessionId 的最近一条 `mode.set` 恢复，MUST NOT 全局兜底**（多 pi 窗口并行下，全局最新 mode.set 几乎必然属于其他会话）：`resume` 与 `reload` SHALL 清零后按同 sessionId 恢复；`startup` SHALL NOT 清零（pi-subagents 派发共用模块实例），但档位为空时 SHALL 按同 sessionId 恢复（pi 进程重启恢复会话的真实路径），恢复成功 SHALL 以 `source:"recover"` 记账；`new`/`fork` SHALL 维持清零语义（不恢复）。无本会话 mode.set 记录、或无法恢复（记录过期、绑定 change 目录已不存在）MUST 回落未激活档。恢复的档位与绑定遵循既有回落规则（change 归档后自动回落）。

#### Scenario: explore 新会话不绑定无关 change

- **WHEN** 新会话读 openspec-explore skill 激活 requirements 档，输入未提及任何 change，且存在 mtime 更新的其他 change 目录
- **THEN** 注入块活跃变更显示「无」，关键词命中源不含无关 change 文本，pin_finding 落 research 库而非 mtime 最新 change

#### Scenario: requirements 档提及后正常绑定

- **WHEN** requirements 档下用户输入提及某 change 名（如 `/opsx-continue <name>`）
- **THEN** 档位绑定该 change，其探索发现与约束照常送达

#### Scenario: reload 后档位恢复

- **WHEN** 档位激活的会话执行 `/reload`（reason=reload，sessionId 不变且有 mode.set 记录）
- **THEN** 清零后按同 sessionId 恢复档位与 change 绑定（记 mode.set source=recover），无需重新触发阶段命令

#### Scenario: resume 无法恢复回落未激活

- **WHEN** resume 时事实库无可用 mode.set（无本会话记录/过期），或记录绑定的 change 目录已不存在
- **THEN** 档位回落未激活（仅稳定层注入索引），不报错

#### Scenario: 新会话不继承档位

- **WHEN** 以 `session_start{reason:"new"|"fork"}` 开始会话，此前另一会话刚记录过 mode.set
- **THEN** 档位为未激活，不从事实库恢复

#### Scenario: 斜杠命令激活档位

- **WHEN** 用户输入 `/opsx-apply add-change-scope`
- **THEN** 档位切换为 implementation 并绑定 change `add-change-scope`，记一条 `mode.set`（payload source=command）

#### Scenario: skill 读取激活档位

- **WHEN** agent 自动 read `.agents/skills/openspec-propose/SKILL.md`
- **THEN** 档位切换为 requirements（skill 路径 signal 命中），记一条 `mode.set`（payload source=skill）

#### Scenario: change 归档后回落

- **WHEN** 档位绑定的 change 目录已不存在（已归档）
- **THEN** 下一次 `before_agent_start` 前档位回落未激活，不再注入该 change 相关约束

#### Scenario: read 不抢绑

- **WHEN** implementation 档健康绑定 change A，agent 并行 read `openspec/changes/B/tasks.md` 与其他文件（参考/巡检）
- **THEN** 绑定保持 A 不变，无 mode.set 事件，注入与记账归因不漂移

#### Scenario: 绑定健康时写其他 change 目录不抢绑

- **WHEN** implementation 档健康绑定 change A，agent write/edit `openspec/changes/B/proposal.md`
- **THEN** 绑定保持 A 不变，无 mode.set 事件（多 change 轮转的归因稳定）

#### Scenario: 无绑定时写 change 目录兜底绑定

- **WHEN** 档位激活但绑定为空（或绑定 change 目录已消失），agent write `openspec/changes/<name>/tasks.md` 勾选任务
- **THEN** 绑定兜底设为 `<name>`，记一条 `mode.set`（payload source=edit-dir）

#### Scenario: 同 turn 绑定锁定（并行竞态消除）

- **WHEN** 同一 turn 内并行工具调用分别触碰 change X / Y / Z 的目录（write/edit 均满足兜底条件）
- **THEN** 该 turn 绑定仅取首个生效事件（顺序由事件到达序决定且仅一次），同 turn 其余事件不切换绑定、不产生额外 mode.set

#### Scenario: apply 中断后 resume 恢复档位

- **WHEN** implementation 档会话（绑定 change X）中断后以 `session_start{reason:"resume"}` 恢复，事实库存在该会话线最近的 mode.set 且 change X 目录仍存在
- **THEN** 档位恢复为 implementation 并绑定 X（记 mode.set source=recover），约束注入与 explore-findings 注入不丢失

#### Scenario: pi 重启恢复会话后档位恢复（真实路径）

- **WHEN** pi 进程退出后重启并恢复原会话（`session_start{reason:"startup"}`，sessionId 与此前 mode.set 记录一致，模块档位为空）
- **THEN** 档位按同 sessionId 的最近一条 mode.set 恢复（含 change 绑定，记 mode.set source=recover），约束注入不丢失

#### Scenario: 全新 pi 会话启动不继承其他会话档位

- **WHEN** pi 冷启动打开全新会话（reason=startup，该 sessionId 无任何 mode.set 记录），而事实库存在其他会话的 mode.set
- **THEN** 档位为未激活（startup 恢复无全局兜底），零注入零记账

#### Scenario: 无自身档位历史的会话 reload/resume 不继承他窗口档位

- **WHEN** 某会话自身无任何 mode.set 记录（新窗口探索会话），执行 `/reload` 或 `/resume`（reason=reload/resume），而事实库全局最新一条 mode.set 属于其他并行窗口
- **THEN** 该会话档位为未激活（三条恢复路径均仅同 sessionId 取数，MUST NOT 全局兜底）

#### Scenario: 子线程派发不清零主会话档位

- **WHEN** 主会话 implementation 档激活，pi-subagents 派发子线程触发 `session_start{reason:"startup"}`（共用模块实例）
- **THEN** 主会话档位与命中保持不变（不清零、不重复恢复、不产生 mode.set）

#### Scenario: mtime 兜底显式记账

- **WHEN** `/opsx-apply` 无参触发 implementation 档且无提及 change，mtime 兜底绑定最新 change `<name>`
- **THEN** 记一条 `mode.set`（payload source=fallback），注入块头部活跃变更可归因

### Requirement: 混合注入通道（稳定层 system prompt + 动态层 steer 消息）

约束注入 SHALL 按内容变化频率分两层通道送达（替代原「每 turn 全量重建 system prompt 注入块」——实测 system prompt 任何字节变化（含尾部追加）均使其后全部对话历史的前缀缓存失效）：

**稳定层（system prompt，`before_agent_start` 追加）**：仅含变化频率与档位生命周期对齐的内容——索引文档（constraints-index）+ mode-base 基础文档 + 声明域红线层。稳定层在档位生命周期内（同一档位且绑定 change 不变）SHALL 字节恒定：内容不随对话输入、编辑路径、findings 文件更新而变化；仅在档位切换/绑定修正时整体变化一次（一次性缓存成本）。声明域红线层仍每回合从文件重解析内容（proposal 编辑生效），但重解析结果与上次不同时 SHALL 以 steer 消息通知差异部分而不改写稳定层（稳定层保持激活时刻快照）。

**动态层（steer 消息，`pi.sendMessage` + `deliverAs:"steer"`）**：关键词命中全节、JIT 路径命中全节、change 级文件（explore-findings / 词汇表）、稳定层差异通知。动态层 SHALL 事件驱动发送：维护已发送内容指纹，仅当指纹变化（新命中、文件更新导致内容变化、档位切换后首轮）时发送增量消息，相同指纹零发送（MUST NOT 每 turn 重发）。消息 SHALL 标注类型与来源（customType），display 渲染 SHALL 受控（默认折叠/精简展示，不整块刷屏）。steer 消息参与 LLM 上下文；档位切换时动态层全量集合重置（新 change 的命中文档与旧 change 无关）。

**compaction 补偿**：`session_compact` 事件后，extension SHALL 重发一次当前约束快照（稳定层摘要 + 动态层当前有效集合，一条消息），指纹不因重发而跳过。快照重发 SHALL 在 compact 完成后的下一次注入时机合并执行，不额外打断 turn。

**关键词命中域限定**：关键词命中源 SHALL 仅含最近用户输入（滚动窗），change 产物全文 MUST NOT 触发关键词命中；命中文档范围 SHALL 限定为「当前 change 声明域 ∪ 栈检测相关文档 ∪ 索引文档」，声明域之外的跨域关键词命中 MUST NOT 触发注入（实测修复：聊 harness 机制含 "discovery" 词误拉 ~8KB discovery.md 全节）。ASCII 关键词 SHALL 按词边界整词匹配（CJK 关键词保持子串匹配）。

业务域声明 SHALL 解析 proposal.md 头部的 `<!-- constraint-domains: <域>, ... -->` HTML 注释标记（可多行合并），域名合法值 SHALL 为 `docs/reference/flow/*.md` 的 basename；未知域名 SHALL 宽容忽略并在状态提示；无声明 SHALL 不注入任何 flow 约束节并在状态栏提示（不阻断）。

**声明域注入 SHALL 提取目标域「业务约束与不变量」节的红线层**：节内每条约束的首行加粗红线句逐行注入，尾部 SHALL 附细节层取回指引。红线层为空（0 条提取）或拼接后低于节级最小字节下限（`minSectionBytes`，缺省 512）SHALL 回退注入全节（fail-safe）。声明域 `constraint.inject` 记账 payload SHALL 附层级标记（`redline` / `full`）。

JIT pathSignals SHALL 复用文档既有 `doc-impact-applies` frontmatter 标签生成，`.pi/constraint-injection.json` MUST NOT 手写 pathSignals。JIT 与关键词命中的注入内容 SHALL 为完整约束节（细节层经此通道按需到达模型）。

节级注入 SHALL 设最小字节下限：提取的节内容低于下限时 SHALL 回退注入该文档全文（fail-safe）。注入内容（稳定层 + 动态层单条消息）总量 SHALL 受 `budgetBytes` 预算限制（配置缺省 32768 字节），超预算时 SHALL 按命中信号强度分层降级（关键词命中节 → JIT 命中节 → change 级文件 digest 收紧 → 域声明节 → change 级文件降占位；baseDocs 与 header SHALL NOT 降级），降级 MUST NOT 真丢文档（至少保留标题 + read 路径占位），降级决策 SHALL 确定性（相同命中集合 + 相同文档内容 + 相同预算 → 相同结果）。超预算时 SHALL 在注入块头部或消息头部列出已降级路径。

注入块（含 steer 消息正文）SHALL 标注「与 AGENTS.md 优先级宪法冲突时以宪法为准」。

#### Scenario: 稳定层档位生命周期内恒定

- **WHEN** implementation 档绑定 change X 激活后，会话继续 20 个 turn（新增关键词命中、编辑代码触发 JIT 命中、findings 被 pin 更新）
- **THEN** system prompt 注入块字节完全不变（新内容全部经 steer 消息送达），历史前缀缓存零失效

#### Scenario: 档位切换一次性变化

- **WHEN** 会话从未激活档切换为 implementation 绑定 change X
- **THEN** 稳定层整体变化一次（索引 → 索引 + mode-base + X 的声明域红线层），此后 X 生命周期内恒定

#### Scenario: 动态层事件驱动增量发送

- **WHEN** 档位激活后第 3 个 turn 用户输入命中 daily-report 域关键词，第 4~10 个 turn 无新命中且已发送内容无变化
- **THEN** 第 3 个 turn 发送一条含 daily-report 全节的 steer 消息，第 4~10 个 turn 零发送

#### Scenario: findings 更新触发重发

- **WHEN** implementation 档激活且 explore-findings.md 已发送，agent 调 pin_finding 追加新条目
- **THEN** 下一注入时机发送一条包含更新后 findings 的 steer 消息（指纹变化），而非每 turn 重发

#### Scenario: compact 后快照重发

- **WHEN** 会话发生 compaction（steer 消息被摘要移除），档位仍激活
- **THEN** compact 后下一次注入时机合并重发一条约束快照（稳定层摘要 + 动态层当前有效集合），红线约束不因压缩丢失

#### Scenario: 关键词域限定

- **WHEN** implementation 档绑定 change X（声明域：scheduler, ai-summary），用户输入含 "discovery"（X 的声明域之外的域关键词）
- **THEN** discovery.md MUST NOT 被注入（关键词命中仅限声明域 ∪ 栈相关 ∪ 索引文档）

#### Scenario: 声明域内关键词正常命中

- **WHEN** implementation 档绑定 change X（声明域含 ai-summary），用户输入含 "airouter"
- **THEN** ai-summary.md 完整约束节经 steer 消息送达（细节层）

#### Scenario: 稳定层差异走 steer 通知

- **WHEN** implementation 档激活后 proposal.md 的 constraint-domains 声明被编辑（新增一域）
- **THEN** 稳定层保持激活时刻快照不变，新增域的红线层经 steer 消息通知（含取回指引）

#### Scenario: 未激活档仅索引

- **WHEN** 会话未识别任何档位（普通问答/research 语境）
- **THEN** 仅稳定层注入索引文档，零动态层发送

#### Scenario: 实现档注入生效

- **WHEN** implementation 档激活且 proposal.md 声明 `constraint-domains: daily-report`
- **THEN** 稳定层含 daily-report.md「业务约束与不变量」节红线层（首行红线句逐行 + 细节层取回指引），模型无法绕过

#### Scenario: JIT 路径细化

- **WHEN** implementation 档激活且 agent edit `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`
- **THEN** 命中 daily-report.md 头部 `doc-impact-applies` 标签路径前缀，该域完整约束节经 steer 消息送达

#### Scenario: 节残缺时回退全文

- **WHEN** 某 flow 文档「业务约束与不变量」节处于编辑中间态仅 133B（低于 minSectionBytes），全文完整
- **THEN** 注入回退为该文档全文，记账 bytes 为全文字节数（残缺节不注入）

#### Scenario: 红线层为空回退全节

- **WHEN** 某声明域约束节尚无规整列表项加粗红线句（红线层提取 0 条），全文完整
- **THEN** 该域声明注入回退为全节，constraint.inject 记账 payload 层级标记为 `full`、bytes 为全节字节数

#### Scenario: 红线层低于最小字节回退全节

- **WHEN** 某声明域红线层拼接后 380B（低于 minSectionBytes 缺省 512）
- **THEN** 该域声明注入回退为全节（残缺红线层不注入），记账 bytes 如实为全节

#### Scenario: flow 文档节级注入

- **WHEN** 档位激活且声明域命中某 flow 域（稳定层注红线层），或最近输入 / JIT 路径命中某 flow 域（动态层注全节）
- **THEN** 声明域注入为该域红线层（逐行 + 指引尾行），关键词 / JIT 命中注入为该域完整约束节（细节层），两种形态节尾均附全文路径指引

#### Scenario: 声明注入层级记账

- **WHEN** 某 change 声明两域，一域红线层 1.8K、另一域红线层回退全节 12.9K
- **THEN** 产生两条 reason=declaration 的 constraint.inject，payload 分别附 `layer:redline`（bytes≈1843）与 `layer:full`（bytes≈12900）

#### Scenario: 命中只增不减（缓存稳定）

- **WHEN** 档位激活且用户输入「日报」命中 daily-report 域（动态层送达），后续输入使该词滚出最近输入窗
- **THEN** 该文档仍在动态层有效集合中（集合只增不减，不因词滚出而移除），且因指纹未变而不重发（稳态零消息）

#### Scenario: change 文本不触发关键词命中

- **WHEN** implementation 档激活且 change 的 proposal/design/tasks 全文含 `stage`、`topic`、`digest` 等域关键词，但 proposal 未声明对应域、最近输入亦未提及
- **THEN** 不注入 semantic-board / topic-graph / daily-report 任一域约束节

#### Scenario: ASCII 关键词词边界整词匹配

- **WHEN** 最近输入含英文单词 "stage"（不含独立词 "tag"）
- **THEN** 不命中 semantic-board 域关键词

#### Scenario: 无声明不注入并提示

- **WHEN** implementation 档激活且绑定 change 的 proposal.md 无 `constraint-domains` 标记
- **THEN** 不注入任何 flow 约束节，状态栏 widget 提示「无域声明」，不阻断会话

#### Scenario: 未知域名宽容忽略

- **WHEN** proposal.md 声明 `constraint-domains: daily-report, not-a-domain`
- **THEN** 注入 daily-report 域红线层，忽略 `not-a-domain` 并在状态提示

#### Scenario: 超预算分层降级

- **WHEN** budgetBytes=16384 且当前命中内容合计 43K（超预算）
- **THEN** 依分层顺序降级（关键词命中节先于 JIT 命中节、change 级文件先 digest 收紧、域声明节后降）直到预算内，baseDocs 与 header 不降级

#### Scenario: 降级永不真丢

- **WHEN** 预算极小（如 budgetBytes=2048）且命中 3 个 flow 节
- **THEN** 每个命中文档至少保留一行「标题 + read 路径」占位，无任何文档从注入内容中完全消失

#### Scenario: 模型可见省略通知

- **WHEN** 本回合发生降级
- **THEN** 注入块头部或消息头部列出全部已降级路径，模型可据此 read 对应文档补取全文

#### Scenario: 降级确定性（缓存友好）

- **WHEN** 命中集合与文档内容、预算均未变，连续两个回合重建注入内容
- **THEN** 两次降级结果与送达字节完全一致

#### Scenario: 降级记账

- **WHEN** daily-report 约束节（红线层）本回合被降级为占位行
- **THEN** 产生一条 constraint.inject，payload 附降级标记与降级后字节数（区别于未降级回合）

#### Scenario: steer 消息 UI 受控

- **WHEN** 动态层发送一条含 8KB 约束节的 steer 消息
- **THEN** TUI 展示为精简/折叠形态（类型 + 标题 + 摘要），不整块刷屏

### Requirement: pin_finding 落点解析

extension SHALL 注册 `pin_finding` 工具持久化探索发现，落点与 research-retention 规则对齐，三级解析：

1. 显式 `change` 参数（目录存在）或档位激活（档位绑定优先；mtime 兜底仅 implementation 档）→ 活跃 change 的 `explore-findings.md`，implementation 档自动注入；
2. 无档且传 `topic` → `docs/research/<topic>/explore-findings.md`；
3. 无档无 topic → 通用池单文件 `docs/research/explore-findings.md`。

任何落点 SHALL NOT 写入 `docs/experience/`。

#### Scenario: 实现阶段自动注入发现

- **WHEN** requirements 档 pin 了「告警表结构」发现，随后档位切换为 implementation
- **THEN** 该 explore-findings.md 内容经动态层 steer 消息送达，实现阶段无需重探

#### Scenario: research 语境不落 change

- **WHEN** 无激活档位且未传 change 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/` 下（传 topic 落 `<topic>/`，未传落通用单文件），不写入 `openspec/changes/`

#### Scenario: research 语境无 topic 落通用池

- **WHEN** 无激活档位且未传 change 与 topic 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/explore-findings.md` 通用池单文件

### Requirement: smoke test 覆盖纯函数

extension 的纯逻辑（档位匹配、栈判定、速览提取、节提取、红线层提取与最小字节回退、节最小字节判定与全文回退、落点三级解析、命令匹配、**抢绑条件判定（read 不抢/健康不抢/兜底绑）**、**turn 绑定锁定**、**动态层 diff 指纹计算**、**关键词命中域过滤**、**compact 快照重发判定**）SHALL 有可脱离 pi harness 运行的 smoke test（node 直跑 .cjs 模式，同源项目 `tests/*.smoke.cjs` 实践）。

#### Scenario: smoke test 直跑

- **WHEN** 执行 smoke test 脚本（不启动 pi）
- **THEN** 全部断言通过，退出码 0

#### Scenario: 抢绑条件纯函数直跑

- **WHEN** smoke test 对抢绑判定函数分别传入 (read, 绑定健康)、(write, 绑定健康)、(write, 无绑定)、(edit, 绑定 change 目录消失) 四组输入
- **THEN** 判定结果分别为不抢、不抢、兜底绑、兜底绑，断言通过

#### Scenario: 动态层 diff 指纹纯函数直跑

- **WHEN** smoke test 对 diff 计算函数传入两次相同命中集合与文档内容
- **THEN** 指纹相同、增量消息为空（零发送），断言通过

#### Scenario: 红线层提取纯函数直跑

- **WHEN** smoke test 对红线层提取函数传入含 `N. **句**：细节` 规整列表项的约束节文本
- **THEN** 提取结果为各列表项首个加粗块的逐行序列（保留原文顺序），断言通过

#### Scenario: 红线层零提取回退

- **WHEN** smoke test 对红线层提取函数传入无列表项加粗（纯段落）的约束节
- **THEN** 判定结果为回退全节（与节不存在回落同族），断言通过

#### Scenario: 节低于最小字节下限回退全文

- **WHEN** smoke test 对节提取纯函数传入低于 `minSectionBytes` 的节内容与完整全文
- **THEN** 判定结果为回退全文（与节不存在回落同族），断言通过

#### Scenario: 节提取回落

- **WHEN** 配置了 `section` 的文档内不存在该 `## 节名`（文档结构变更未同步配置）
- **THEN** 回落全文注入（不报错、不静默跳过，fail-safe）

### Requirement: 会话作用域状态隔离

注入器的**全部会话语义状态** SHALL 按 `sessionId` 隔离存储与读取：档位（`mode`）、绑定 change（`boundChange`）、turn 绑定锁、最近输入窗、JIT 命中集、关键词命中集、pin 注入去重集、稳定层快照/指纹/待发快照。任一时刻，某会话的注入内容（含归因字段与 `declaration` 域选择）SHALL 只由该会话自身的状态决定；他会话的绑定变化 MUST NOT 改变本会话的注入内容或归属。

**无自身状态时的取用规则**（防跨会话借用）：会话有自身状态 → 用自身的；会话为父会话的 fork/子线程（父子关系可证）→ SHALL 显式继承父会话状态并以 `mode.set source=inherit` 记账；其余情况（无状态、无父子关系、无法确定来源）→ SHALL 视为未绑定（仅注入索引），MUST NOT 借用任意他会话的状态。按 `sessionId` 从事实库恢复档位（`resume` / `reload` / `startup`）的既有语义保持不变。

**有界性**：会话条目 SHALL 有数量上限与最近使用淘汰（长跑进程不得无界增长）；extension rebind 时 SHALL 只重置本会话条目。无 `sessionId` 的非真实会话语境（烟测 stub）SHALL 使用单一兜底槽位，行为与隔离前等价。

#### Scenario: 两会话交叉绑定互不污染

- **WHEN** 同一进程内会话 A 绑定 change X、会话 B 绑定 change Y，且两者交替编辑各自 change 目录
- **THEN** A 的注入归属恒为 X（含 X 声明的域），B 的注入归属恒为 Y
- **AND** 任一时刻都不出现「A 的注入带 Y」或「B 的注入带 X」

#### Scenario: 他会话绑定变化不刷新本会话稳定层

- **WHEN** 会话 B 在同一进程内切换绑定（或首次绑定），会话 A 的档位与绑定未变
- **THEN** A 的稳定层快照字节不变、不产生新的稳定层注入记账
- **AND** 动态层指纹不因 B 的变化而变化

#### Scenario: 无自身绑定且无父子关系时不借用他会话

- **WHEN** 会话 C 从未绑定任何 change（无 `mode.set` 记录），而同进程会话 D 已绑定 change Y
- **THEN** C 只注入常驻索引（未激活档），其注入归因 MUST 为未绑定
- **AND** C 的注入内容中 MUST NOT 出现 Y 的档位基础块或 Y 声明的域约束

#### Scenario: 子线程显式继承父会话

- **WHEN** 主会话绑定 change X 后派发子线程（pi-subagents 子线程跑在**独立进程**、独立 sessionId、`session_start` reason=startup；其会话文件 header 带 `parentSession` 指向父会话文件）
- **THEN** 子线程注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现一条该子会话的 `mode.set`，`source=inherit`、`boundChange=X`
- **AND** 子线程活动 MUST NOT 清零或改写主会话的档位与命中集

> 实现口径：跨进程时父会话的内存状态不可见，继承 SHALL 经**事实库回退**（用父会话 id 查其最近一条可恢复 `mode.set`）完成；父子关系不可证（无 `parentSession` 且路径不可解）时 SHALL 退化为未激活，MUST NOT 借用他会话状态。

#### Scenario: 命中集按会话隔离

- **WHEN** 会话 A 编辑代码命中 JIT 文档、或输入命中关键词；同一进程会话 B 未命中
- **THEN** B 的注入内容 MUST NOT 包含 A 命中的 JIT 文档或关键词文档
- **AND** A 命中集「只增不减」的缓存稳定性在本会话内保持不变

#### Scenario: turn 绑定锁按会话隔离

- **WHEN** 会话 A 在本 turn 内已完成一次生效绑定，同 turn 会话 B 发生工具调用
- **THEN** A 的绑定在本 turn 内不再切换（锁定语义只受 A 自身事件影响）

#### Scenario: 会话条目有界淘汰

- **WHEN** 新建会话数超过条目上限
- **THEN** 最久未使用的会话条目被淘汰，且淘汰不影响存活会话的档位、绑定与快照

#### Scenario: 无 sessionId 语境行为等价

- **WHEN** 事件上下文缺少 `sessionId`（烟测 stub / 非真实会话）
- **THEN** 全部状态读写落在单一兜底槽位，行为与隔离前一致（单例语义）
