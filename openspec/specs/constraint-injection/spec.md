# constraint-injection Specification

## Purpose
TBD - created by archiving change port-constraint-injection. Update Purpose after archive.

## Requirements

### Requirement: 混合注入通道（稳定层 system prompt + 动态层 steer 消息）

约束注入 SHALL 按内容变化频率分两层通道送达（替代原「每 turn 全量重建 system prompt 注入块」——实测 system prompt 任何字节变化（含尾部追加）均使其后全部对话历史的前缀缓存失效）：

**稳定层（system prompt，`before_agent_start` 追加）**：仅含变化频率与阶段生命周期对齐的内容——索引文档（constraints-index）+ 阶段路由清单（文档路径与节名，不含正文）。稳定层在阶段生命周期内（同一阶段族且绑定 change 不变）SHALL 字节恒定：内容不随对话输入、编辑路径、findings 文件更新而变化；仅在阶段切换/绑定修正时整体变化一次（一次性缓存成本）。**稳定层 SHALL NOT 内联任何业务域约束正文**——正文一律由模型按需自主 read（见 Requirement「阶段驱动的约束路由与自主导入」）。

**动态层（steer 消息，`pi.sendMessage` + `deliverAs:"steer"`）**：阶段首推的红线层、change 级文件（explore-findings / 词汇表）、稳定层差异通知。**动态层 SHALL NOT 注入业务域约束的整节正文**——原关键词命中全节与 JIT 路径命中全节两条通道 SHALL 退役（见 Requirement「阶段驱动的约束路由与自主导入」）。动态层 SHALL 事件驱动发送：维护已发送内容指纹，仅当指纹变化（阶段切换、新命中、文件更新导致内容变化）时发送增量消息，相同指纹零发送（MUST NOT 每 turn 重发）。消息 SHALL 标注类型与来源（customType），display 渲染 SHALL 受控（默认折叠/精简展示，不整块刷屏）。steer 消息参与 LLM 上下文；阶段切换时动态层全量集合重置（新 change 的命中文档与旧 change 无关）。

**指纹重置按 regime 判定**：已发送指纹 SHALL 随投递记录其 regime——投递时 `stableSnapshotKey(stageFamily, boundChange)` 快照键。快照键 SHALL 使用**阶段族**（`planning` / `implementation`）而非完整 stage 值：6 档阶梯若直接入键会导致稳定层在同族内频繁重建（快照重建频率与前缀缓存命中率直接相关）。阶段/绑定可在 turn 中途切换（阶段 marker 变更、change 目录编辑兜底绑定），快照重建则延迟到下一 turn 的 `before_agent_start`；延迟重建时的指纹清空 SHALL 仅在「指纹 regime ≠ 新快照 key」时发生——同 turn 内已在新 regime 下送达的条目 MUST NOT 被延迟重建当作旧 regime 残留清空而原样重投（相同内容 + 相同 regime = 零重发，无论快照是否重建）。fork / 子线程继承时指纹及其 regime SHALL 随 channel 一并拷贝，继承方的快照重建同样按 regime 判定清空（父会话已投递且 regime 一致的条目不重发）。

**compaction 补偿**：`session_compact` 事件后，extension SHALL 重发一次**动态层当前有效集合**（一条消息；稳定层常驻 system prompt、不受 compaction 影响，无需重发——其「摘要」职责由注入块头部行承担），指纹不因重发而跳过。快照重发 SHALL 在 compact 完成后的下一次注入时机合并执行，不额外打断 turn。**例外**：无 `before_agent_start` 事件的子线程（pi-web 托管子会话）MUST NOT 因 compact 重发——该通道不存在，不伪造。

**关键词命中通道退役**：关键词命中源（最近用户输入滚动窗）SHALL NOT 再触发任何约束正文注入。改造前的实测证据：聊 harness 机制含 "discovery" 词即误拉 ~8KB discovery.md 全节；关键词本质重叠，隐式推断必致误注入。

业务域声明 SHALL 解析 proposal.md 头部的 `<!-- constraint-domains: <域>, ... -->` HTML 注释标记（可多行合并），域名合法值 SHALL 为 `docs/reference/flow/*.md` 的 basename；未知域名 SHALL 宽容忽略并在状态提示；无声明 SHALL 不注入任何 flow 约束节并在状态栏提示（不阻断）。

**声明域注入 SHALL 提取目标域「业务约束与不变量」节的红线层**：节内每条约束的首行加粗红线句逐行注入，尾部 SHALL 附细节层取回指引（指向该 flow 文档全文路径与 `constraint-digest.sh` 取数命令）。红线层为空（0 条提取）、拼接后低于节级最小字节下限（`minSectionBytes`，缺省 512）或约束节缺失 SHALL 回退为**取数指引行**（零正文；review R-3：不再回退整节正文，注入体量断言（声明域/路由条目 ≤4000B）对全部域恒成立）。声明域 `constraint.inject` 记账 payload SHALL 附层级标记（`redline` / `pointer`）。红线层 SHALL 仅在**阶段首推**时注入；同阶段内后续回合 MUST NOT 重复注入相同红线层内容（零冗余）。

JIT pathSignals SHALL 复用文档既有 `doc-impact-applies` frontmatter 标签生成，`.pi/constraint-injection.json` MUST NOT 手写 pathSignals。**编辑路径命中 SHALL NOT 触发正文注入**——命中的文档 SHALL 出现在本阶段路由清单中（引导模型自行 read）。

节级注入 SHALL 设最小字节下限：提取的节内容低于下限时 SHALL 回退为取数指引行（零正文，不注入残缺内容）。注入内容（稳定层 + 动态层单条消息）总量 SHALL 受 `budgetBytes` 预算限制（配置缺省 32768 字节），超预算时 SHALL 按命中信号强度分层降级（关键词命中节 → JIT 命中节 → change 级文件 digest 收紧 → 域声明节 → change 级文件降占位；baseDocs 与 header SHALL NOT 降级），降级 MUST NOT 真丢文档（至少保留标题 + read 路径占位），降级决策 SHALL 确定性（相同命中集合 + 相同文档内容 + 相同预算 → 相同结果）。超预算时 SHALL 在注入块头部或消息头部列出已降级路径。

注入块（含 steer 消息正文）SHALL 标注「与 AGENTS.md 优先级宪法冲突时以宪法为准」。

#### Scenario: 稳定层档位生命周期内恒定

- **WHEN** implementation 族阶段绑定 change X 激活后，会话继续 20 个 turn（findings 被 pin 更新、路由清单不变）
- **THEN** system prompt 注入块字节完全不变（新内容全部经 steer 消息送达），历史前缀缓存零失效

#### Scenario: 档位切换一次性变化

- **WHEN** 会话从 planning 族阶段切换为 implementation 族阶段并绑定 change X
- **THEN** 稳定层整体变化一次（索引 → 索引 + 新阶段路由清单），此后 X 生命周期内恒定

#### Scenario: 动态层事件驱动增量发送

- **WHEN** 阶段首推后第 3 个 turn 无关键词命中，第 4~10 个 turn 无阶段变化且已发送内容无变化
- **THEN** 第 3~10 个 turn 零发送（阶段首推已发生一次，此后同阶段零冗余）

#### Scenario: turn 中途切档后同 turn 已投递内容不被延迟重建重投

- **WHEN** 会话在某 turn 内切换到新阶段（`stage.md` marker 变更），同 turn 随后触发该阶段首推（红线层经 steer 消息送达，指纹以新阶段 regime 记录）；下一 turn `before_agent_start` 延迟重建稳定层快照（快照 key 从旧阶段族变化为新阶段族）
- **THEN** 稳定层按新阶段重建（路由清单一次性变化），但动态层对该已投递内容零重发（指纹 regime 与新快照 key 一致，不清空不重投）；`constraint.inject` 记账中该内容在同会话代内不出现第二条同字节记录

#### Scenario: 绑定切换后首轮仍全量重投

- **WHEN** 阶段激活且已投递若干动态条目（指纹 regime 为 `<stageFamily>|<changeX>`），turn 中途 agent 写 change Y 目录触发兜底绑定（edit-dir），下一 turn `before_agent_start` 快照 key 变为 `<stageFamily>|<changeY>`
- **THEN** 指纹 regime 与新快照 key 不一致 → 指纹清空、动态层全量集合按新 change 语境重投（既有设计行为保持：新 change 的命中文档与旧 change 无关）

#### Scenario: fork 继承的指纹不被陈旧快照 key 重投

- **WHEN** 父会话在 turn 中途切换阶段后已投递若干条目（指纹 regime 为新阶段族 key，但快照 key 尚为切换前的旧值），子会话 fork 继承父会话状态（含 channel 与指纹 regime），子会话首个 turn 快照重建（快照 key 与继承的旧快照 key 不致）
- **THEN** 指纹 regime 与新快照 key 一致 → 不清空、父会话已投递条目不重发（fork 语义：父会话已送达的动态层条目不重复）

#### Scenario: findings 更新触发重发

- **WHEN** implementation 族阶段激活且 explore-findings.md 已发送，agent 调 pin_finding 追加新条目
- **THEN** 下一注入时机发送一条包含更新后 findings 的 steer 消息（指纹变化），而非每 turn 重发

#### Scenario: compact 后快照重发

- **WHEN** 会话发生 compaction（steer 消息被摘要移除），档位仍激活
- **THEN** compact 后下一次注入时机合并重发一条约束快照（动态层当前有效集合；稳定层常驻 system prompt 不受影响），红线约束不因压缩丢失

#### Scenario: 关键词域限定

- **WHEN** implementation 档绑定 change X（声明域：scheduler, ai-summary），用户输入含 "discovery"（X 的声明域之外的域关键词）
- **THEN** discovery.md MUST NOT 被注入（关键词命中仅限声明域 ∪ 栈相关 ∪ 索引文档）

#### Scenario: 声明域内关键词正常命中

- **WHEN** implementation 档绑定 change X（声明域含 ai-summary），用户输入含 "airouter"
- **THEN** ai-summary.md 进入本阶段路由清单并标注「本次会话提及」（域限定仍生效），零正文注入（关键词通道已退役，只标注不送正文）

#### Scenario: 稳定层差异走 steer 通知

- **WHEN** implementation 档激活后 proposal.md 的 constraint-domains 声明被编辑（新增一域）
- **THEN** 稳定层保持激活时刻快照不变，新增域的红线层经 steer 消息通知（含取回指引）

#### Scenario: 未激活档仅索引

- **WHEN** 会话未识别任何档位（普通问答/research 语境）且未发生编辑路径命中
- **THEN** 仅稳定层注入索引文档，零动态层发送

#### Scenario: 未激活档关键词通道保持关闭

- **WHEN** 未激活档会话的用户输入含「日报」「定时任务」等域关键词（未发生编辑路径命中）
- **THEN** 不注入任何 flow 域约束节（关键词匹配仍以档位为开关）

#### Scenario: 实现档注入生效

- **WHEN** implementation 档激活且 proposal.md 声明 `constraint-domains: daily-report`
- **THEN** 稳定层含 daily-report.md「业务约束与不变量」节红线层（首行红线句逐行 + 细节层取回指引），模型无法绕过

#### Scenario: JIT 路径细化

- **WHEN** implementation 档激活且 agent edit `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`
- **THEN** 命中 daily-report.md 头部 `doc-impact-applies` 标签路径前缀，该域的路径与节名列进本阶段路由清单（零正文，引导自主 read）

#### Scenario: 节残缺时回退全文

- **WHEN** 某 flow 文档「业务约束与不变量」节处于编辑中间态仅 133B（低于 minSectionBytes），全文完整
- **THEN** 注入回退为取数指引行（零正文，含 read 路径与 digest 命令），记账 bytes 为指引行字节数（残缺节不注入、全文也不自动注入）

#### Scenario: 红线层为空回退全节

- **WHEN** 某声明域约束节尚无规整列表项加粗红线句（红线层提取 0 条），全文完整
- **THEN** 该域声明注入回退为取数指引行（零正文），constraint.inject 记账 payload 层级标记为 `pointer`、bytes 为指引行字节数

#### Scenario: 红线层低于最小字节回退全节

- **WHEN** 某声明域红线层拼接后 380B（低于 minSectionBytes 缺省 512）
- **THEN** 该域声明注入回退为取数指引行（残缺红线层不注入、正文零推送），记账 bytes 如实为指引行字节数

#### Scenario: flow 文档节级注入

- **WHEN** 阶段激活且声明域命中某 flow 域（红线层首推），或最近输入 / 编辑路径命中某 flow 域
- **THEN** 声明域注入为该域红线层（逐行 + 指引尾行），关键词 / 编辑路径命中只把该域列进路由清单（零正文），两种形态均附全文路径指引

#### Scenario: 声明注入层级记账

- **WHEN** 某 change 声明两域，一域红线层 1.8K、另一域红线层回退取数指引（约 0.2K）
- **THEN** 产生两条 reason=declaration 的 constraint.inject，payload 分别附 `layer:redline`（bytes≈1843）与 `layer:pointer`（bytes≤4000）

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

1. 显式 `change` 参数（目录存在）或阶段已绑定（绑定优先；mtime 兜底仅 implementation 族阶段）→ 活跃 change 的 `explore-findings.md`，implementation 族阶段自动注入；
2. 无档且传 `topic` → `docs/research/<topic>/explore-findings.md`；
3. 无档无 topic → 通用池单文件 `docs/research/explore-findings.md`。

任何落点 SHALL NOT 写入 `docs/experience/`。

#### Scenario: 实现阶段自动注入发现

- **WHEN** planning 族阶段 pin 了「告警表结构」发现，随后阶段切换为 implementation 族
- **THEN** 该 explore-findings.md 内容经动态层 steer 消息送达，实现阶段无需重探

#### Scenario: research 语境不落 change

- **WHEN** 无绑定 change 且未传 change 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/` 下（传 topic 落 `<topic>/`，未传落通用单文件），不写入 `openspec/changes/`

#### Scenario: research 语境无 topic 落通用池

- **WHEN** 无绑定 change 且未传 change 与 topic 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/explore-findings.md` 通用池单文件

### Requirement: smoke test 覆盖纯函数

extension 的纯逻辑（**stage marker 三态解析（missing/invalid/ok）**、**阶段族派生**、**阶段路由清单装配**、栈判定、速览提取、节提取、红线层提取与最小字节回退、节最小字节判定与全文回退、落点三级解析、命令匹配、**抢绑条件判定（read 不抢/健康不抢/兜底绑）**、**turn 绑定锁定**、**动态层 diff 指纹计算**、**compact 快照重发判定**、**读取路径归一化**）SHALL 有可脱离 pi harness 运行的 smoke test（node 直跑 .cjs 模式，同源项目 `tests/*.smoke.cjs` 实践）。

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

注入器的**全部会话语义状态** SHALL 按 `sessionId` 隔离存储与读取：阶段（`stage`）、阶段族（`stageFamily`）、绑定 change（`boundChange`）、turn 绑定锁、pin 注入去重集、稳定层快照/指纹/待发快照。任一时刻，某会话的注入内容（含归因字段与 `declaration` 域选择）SHALL 只由该会话自身的状态决定；他会话的绑定变化 MUST NOT 改变本会话的注入内容或归属。

**无自身状态时的取用规则**（防跨会话借用）：会话有自身状态 → 用自身的；会话为父会话的 fork/子线程（父子关系可证）→ SHALL 显式继承父会话状态并以 `stage.set source=inherit` 记账；其余情况（无状态、无父子关系、无法确定来源）→ SHALL 视为未绑定（仅注入索引），MUST NOT 借用任意他会话的状态。按 `sessionId` 从事实库恢复阶段（`resume` / `reload` / `startup`）的既有语义保持不变。

**子线程检测的通道无关性**：父子关系判定 SHALL 只依据会话自身证据（会话文件 header 的 `parentSession`、fork 路径模式），MUST NOT 绑定具体派发通道——pi-subagents 独立 runner 子线程与 pi-web resourceLoader 子会话（profile 开启扩展加载后）SHALL 走同一继承链路；跨进程场景父会话内存状态不可见，继承 SHALL 经事实库回退（用父会话 id 查其最近一条可恢复 `stage.set`）完成；父子关系不可证（无 `parentSession` 且路径不可解）时 SHALL 退化为未激活，MUST NOT 借用他会话状态。跨进程继承的注入通道为全新会话首投递（子线程 system prompt 独立构建），MUST NOT 因父会话已投递过同内容而抑制投递；进程内 fork 的指纹随 channel 拷贝语义保持不变。

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

- **WHEN** 会话 C 从未绑定任何 change（无 `stage.set` 记录），而同进程会话 D 已绑定 change Y
- **THEN** C 只注入常驻索引（未绑定），其注入归因 MUST 为未绑定
- **AND** C 的注入内容中 MUST NOT 出现 Y 的路由清单或 Y 声明的域约束

#### Scenario: 子线程显式继承父会话

- **WHEN** 主会话绑定 change X 后派发子线程（pi-subagents 子线程跑在**独立进程**、独立 sessionId、`session_start` reason=startup；其会话文件 header 带 `parentSession` 指向父会话文件）
- **THEN** 子线程注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现一条该子会话的 `stage.set`，`source=inherit`、`boundChange=X`
- **AND** 子线程活动 MUST NOT 清零或改写主会话的阶段与绑定

#### Scenario: 子线程显式继承父会话（pi-web 通道）

- **WHEN** 主会话绑定 change X 后经 pi-web Agent 工具用带扩展 profile（`load_extensions: true`）派发子线程（独立进程、独立 sessionId、会话文件 header 带 `parentSession`）
- **THEN** 子线程扩展加载后按同一继承链路注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现一条该子会话的 `stage.set`，`source=inherit`、`boundChange=X`
- **AND** 子线程活动 MUST NOT 清零或改写主会话的阶段与绑定

#### Scenario: 命中集按会话隔离

- **WHEN** 会话 A 处于 implementation 族阶段并绑定 change X；同一进程会话 B 处于 planning 族阶段且绑定 change Y
- **THEN** B 的注入内容 MUST NOT 包含 X 的路由清单或 X 声明的域约束
- **AND** A 的「只增不减」缓存稳定性在本会话内保持不变

#### Scenario: turn 绑定锁按会话隔离

- **WHEN** 会话 A 在本 turn 内已完成一次生效绑定，同 turn 会话 B 发生工具调用
- **THEN** A 的绑定在本 turn 内不再切换（锁定语义只受 A 自身事件影响）

#### Scenario: 会话条目有界淘汰

- **WHEN** 新建会话数超过条目上限
- **WHEN** 最久未使用的会话条目被淘汰，且淘汰不影响存活会话的阶段、绑定与快照

#### Scenario: 无 sessionId 语境行为等价

- **WHEN** 事件上下文缺少 `sessionId`（烟测 stub / 非真实会话）
- **THEN** 全部状态读写落在单一兜底槽位，行为与隔离前一致（单例语义）

### Requirement: 派发期约束摘要取数（dispatch constraint digest）

系统 SHALL 提供取数入口 `scripts/harness/constraint-digest.sh`，供主线程在派发子线程前把业务域红线以短文本形态取到手，无需读整个 flow 文档。入口 MUST 支持两种输入形态：

- `--domains <域名>[,<域名>...]`：域名 MUST 为业务域唯一真相源（`docs/reference/flow/*.md` 的 basename，与 `listFlowDomains` 同源）；域名不存在时 MUST 以非零退出并列出可用域名，MUST NOT 静默产出空结果。
- `--paths <路径>[,<路径>...]`：按 `doc-impact-applies` 标签反查受影响文档，再据文档归属导出域清单。路径反查语义 MUST 与 constraint-injection 的 `scanAppliesTags` 一致（仅扫文档头部 15 行、同一正则、纯子串匹配、单文档首个匹配行即止）。

输出的每个域 MUST 为「域标题 + 源文档相对路径 + 逐行红线句 + 细节层 read 指引」，且单域红线正文 SHALL 控制在 8 行以内。入口 MUST 提供 `--brief` 模式：输出**单个** `## 约束（红线，源 <源路径…>）` 标题行（多域源路径逗号分隔）+ 每域 `### <域>（源 …）` 子标题 + 红线行 + 固定兜底行，使结果可被派发任务文本**原样粘贴**且被 `parseConstraintBrief` 识别为 `present`；默认（无 `--brief`）MUST 保持人读形态。

**红线提取 MUST 复用 constraint-injection 的同一实现**（`extractRedlines`），MUST NOT 在脚本内重写第二份正则或判定逻辑；提取结果与扩展实际注入的红线层 MUST 逐字节一致。红线层提取为空或低于 `minSectionBytes` 时 MUST 回退为「该域无结构化红线，见 §业务约束与不变量」的显式提示，MUST NOT 输出残缺集合冒充完整。

`--paths` 在以下情形 MUST 显式报告「未命中」而不是空输出：路径不落在任何 `doc-impact-applies` 信号内、命中的文档无对应 flow 域（如 `standard/` 文档）、文档头部无标签。未命中时 MUST 附一条回退指引（按任务关键词查 `constraints-index.md` 域表后改用 `--domains`）。

#### Scenario: 按域取红线

- **WHEN** 主线程执行 `constraint-digest.sh --domains daily-report`
- **THEN** 输出含 `daily-report` 域标题、源路径 `docs/reference/flow/daily-report.md`、逐行红线句与细节层 read 指引，红线正文不超过 8 行

#### Scenario: 按目标路径反查

- **WHEN** 主线程执行 `constraint-digest.sh --paths backend-go/internal/admin/scheduler/job_daily_report.go`
- **THEN** 按 `doc-impact-applies` 标签反查命中 `flow/daily-report.md` 与 `flow/scheduler.md`，输出这两域红线

#### Scenario: 路径无命中显式报告

- **WHEN** 执行 `constraint-digest.sh --paths front/app/components/Foo.vue` 且无任何 `doc-impact-applies` 信号命中该路径
- **THEN** 退出码保持可判定，输出「未命中」说明与 `constraints-index.md` 域表回退指引，不输出空的红线段落

#### Scenario: 红线提取与扩展实现一致

- **WHEN** smoke 对同一个 flow 文档分别调用脚本与 constraint-injection 的 `extractRedlines`
- **THEN** 两者产出的红线行逐字节一致（含 0 条时的回退形态判定一致）

#### Scenario: brief 形态可原样粘贴并被记账

- **WHEN** 主线程执行 `constraint-digest.sh --brief --domains daily-report,scheduler` 并把输出原样粘进派发任务文本
- **THEN** 输出含单个 `## 约束（红线，源 …）` 标题行与两域 `### <域>（源 …）` 子标题，`parseConstraintBrief` 解析得 `present` 且域清单含 `daily-report` 与 `scheduler`

#### Scenario: 未知域名拒绝

- **WHEN** 执行 `constraint-digest.sh --domains not-a-domain`
- **THEN** 脚本非零退出，列出可用域名，不静默返回空

### Requirement: 派发期约束摘要的规范引导与可观测

开发规范 MUST 把「派发任务文本带相关域红线摘要」从无方法的 MUST 改写为可执行三步：**判域（按目标路径反查）→ 取红线（`constraint-digest.sh`）→ 粘进派发任务文本的 `## 约束（红线，源 <路径>）` 段**。规范 MUST 同时给出 brief 模板片段与子线程自取兜底命令（派出后子线程按目标路径自行补取），使注入不单靠主线程记性。

事实库的 `subagent.dispatch` 事件 payload SHALL **append-only** 追加两个字段：`constraintDomains`（从派发任务文本 `## 约束` 段解析出的域名数组）与 `constraintDigest`（`present` / `absent`）。既有字段 MUST NOT 变更语义或类型。派发任务文本无法解析出约束段时 SHALL 记 `absent`，MUST NOT 因解析失败而阻断派发。

`harness-retro` SHALL 提供「派发带域比例」口径（present 派发数 / 可判定派发数）。历史记录缺少两个新字段时 MUST 计入「未知」并从分母剔除，MUST NOT 把历史数据回溯记为 absent（避免把改造前数据算成遗漏）。

#### Scenario: 带约束段派发被记账

- **WHEN** 主线程派发任务文本含 `## 约束（红线，源 docs/reference/flow/daily-report.md）` 段
- **THEN** `subagent.dispatch` payload 记 `constraintDigest=present` 与 `constraintDomains=["daily-report"]`，派发行为本身不受影响

#### Scenario: 无约束段派发被记账但不阻断

- **WHEN** 派发任务文本无 `## 约束` 段
- **THEN** payload 记 `constraintDigest=absent`、`constraintDomains=[]`，派发照常进行

#### Scenario: 旧记录计未知不计漏

- **WHEN** `harness-retro` 统计窗口内混有改造前的 `subagent.dispatch` 记录（无两个新字段）
- **THEN** 这些记录计入「未知」并从带域比例分母剔除，报告显式标注未知条数

### Requirement: stage 进度标识（替代档位机制）

extension SHALL 以 openspec change 目录内的 `stage.md` 文件作为**进度阶段的唯一机器可读真相源**。`stage.md` 文件头 SHALL 含恰好一条 `<!-- stage: <值> -->` marker——解析按**全文恰好一条**判定（正文中间出现同形 marker 也算重复 → invalid，比「仅头部」更严，防正文示例伪造阶段）。

stage 取值域 SHALL 为以下 6 档单调阶梯（顺序即进度顺序）：

| stage | 语义 | 阶段族 |
| --- | --- | --- |
| `explore` | 探索/脑暴期，尚无制品 | planning |
| `propose` | proposal 制品产出期 | planning |
| `design` | ui-design / specs / design 制品产出期 | planning |
| `implement` | 实现执行期（tasks 勾选推进） | implementation |
| `verify` | 验收期 | implementation |
| `archive` | 归档期 | implementation |

**阶段族 SHALL 为派生值**（由 stage 映射得出），MUST NOT 作为独立可写状态存在。

marker 解析 SHALL 区分三态：

- **missing**：`stage.md` 不存在或无 marker → 视为 legacy change：**不绑定阶段**、MUST NOT 阻断任何流程（含归档）；约束注入退化为「索引 + 声明域红线层」，不提供阶段路由清单（注意：keyword 与 jit-path 两条整节正文通道的退役对**全部** change 生效，与 stage 是否存在无关——legacy change 同样不再收到整节正文）；
- **invalid**：marker 重复出现、或值不在 6 档取值域内 → 视为 legacy（降级不阻断），并记一条 warn 级事实库事件；
- **ok**：恰好一条合法 marker → stage 生效。

`stage.md` SHALL 为非 schema 制品（与 `explore-findings.md` / `human-feedback.md` / `test-cases.md` 同性质），openspec schema 的制品 DAG SHALL NOT 因之改变。

**业务域声明位置 SHALL 支持两处**：proposal.md 头部的 `<!-- constraint-domains: <域>, ... -->`（**存量 change 的既有位置**，保持不变）与 `stage.md` 头部的同名 marker（**实现期活口径**，用于声明实施过程中发现的新增业务域）。生效的域集合 SHALL 为两处声明的**并集**——stage.md SHALL NOT 收窄 proposal.md 已声明的域（域声明按超集安全方向收敛：多声明只是多推几行红线，不会漏推）。

extension SHALL 在所有原「档位绑定变化」的路径上自报 `stage.set` 事实库事件，payload 含 `stage`（6 档取值）、`stageFamily`（`planning` / `implementation`）、`boundChange`（可空）与 `source`（归因来源）。原 `mode.set` 事件 SHALL 停写；读方 SHALL 在过渡期双读（先查 `stage.set`，无结果时查 `mode.set` 并按 `requirements`→`planning`、`implementation`→`implementation` 映射回族），保留期 30 天自然消退后双读可移除。

#### Scenario: stage marker 生效

- **WHEN** `openspec/changes/<name>/stage.md` 头部含恰好一条 `<!-- stage: implement -->`
- **THEN** 该 change 的当前阶段为 `implement`，阶段族为 `implementation`
- **AND** 产生一条 `stage.set` 事件，payload 含 stage=implement、stageFamily=implementation

#### Scenario: 缺失 marker 视为 legacy 不阻断

- **WHEN** 某 change 目录不存在 `stage.md`
- **THEN** 该 change 按 legacy 处理，不绑定阶段
- **AND** 约束注入退化为「索引 + 声明域红线层」，不提供阶段路由清单
- **AND** 归档流程不因此阻断

#### Scenario: 非法 stage 值降级不阻断

- **WHEN** `stage.md` 头部写 `<!-- stage: coding -->`（不在 6 档取值域内）
- **THEN** 视为 legacy 降级，MUST NOT 阻断流程
- **AND** 记一条 warn 级事实库事件，payload 标注 stage 非法

#### Scenario: marker 重复视为非法

- **WHEN** `stage.md` 头部出现两条 `<!-- stage: -->` marker
- **THEN** 视为 invalid（legacy 降级），MUST NOT 取其中任一条生效

#### Scenario: stage.md 可追加业务域声明

- **WHEN** proposal.md 声明域为 `reading`，stage.md 声明域为 `reading, scheduler`
- **THEN** 生效域集合为 `{reading, scheduler}`（并集）
- **AND** stage.md 的声明 MUST NOT 收窄 proposal.md 已声明的域

#### Scenario: stage.md 无域声明时沿用 proposal

- **WHEN** proposal.md 声明域为 `scheduler` 且 stage.md 无 `constraint-domains` marker
- **THEN** 生效域集合为 `{scheduler}`

#### Scenario: 事实库双读兼容旧 mode.set

- **WHEN** 查询某 session 的当前阶段，事实库无 `stage.set` 但存在旧 `mode.set`（payload mode=implementation）
- **THEN** 返回阶段族 `implementation`，且不视为错误

### Requirement: 阶段驱动的约束路由与自主导入

extension SHALL 按当前 stage 组装注入内容，取代改造前「按关键词/编辑路径主动注入整节正文」的形态。

**注入内容 SHALL 分两部分**：

1. **红线层**：当前 change 声明的业务域（`<!-- constraint-domains: -->`）对应的 flow 文档「业务约束与不变量」节的顶层列表项首个加粗句逐行序列。提取 SHALL 复用 `lib/constraint-redlines.ts` 的既有实现（禁止重写第二份正则）。红线层提取为空、低于最小字节或约束节缺失时 SHALL 回退为**取数指引行**（文档路径 + 节名 + `constraint-digest.sh --brief --domains <域>` 命令，零正文——整节正文回退已退役，「正文永不主动推」对全部域恒成立）。
2. **路由清单**：本阶段相关文档的清单，每行含文档路径、节名与取数方式。清单内容 SHALL 按 stage 派生：

| stage | 清单构成 |
| --- | --- |
| `explore` / `propose` | 索引文档 + 声明域红线层的细节层指引 |
| `design` | 上述 + UI 契约相关规范（`standard/frontend/layout.md`、`standard/frontend/interaction.md`） |
| `implement` | 上述 + 代码与测试规范（按栈命中 `standard/backend/` 或 `standard/frontend/`） |
| `verify` / `archive` | 上述 + 验收与归档规范（`standard/shared/test-design.md`、`standard/shared/commit-pr.md`、`standard/shared/doc-authoring.md`） |

路由清单 SHALL 以「路径 + 节名 + 取数方式」形态给出，SHALL NOT 内联文档正文。

**注入时机 SHALL 为阶段首推**：某 stage 首次成为当前阶段时注入一次该 stage 的红线层 + 路由清单；同 stage 内的后续回合 MUST NOT 重复注入相同内容（零冗余）。阶段切换时 SHALL 以增量 steer 消息送达差异部分，SHALL NOT 改写 system prompt 稳定层。

**正文导入方式 SHALL 为自主 read**：扩展 MUST NOT 再主动注入业务域约束的整节正文。模型需要细节时自行 `read` 对应文档，清单 SHALL 同时给出 `bash scripts/harness/constraint-digest.sh --brief --domains <域>` 作为取红线摘要的命令形式。

原 keyword 通道（对话关键词命中 → 整节注入）与 jit-path 通道（编辑路径命中 → 整节注入）SHALL 退役。MUST NOT 再因对话关键词或编辑路径向模型主动推送约束文档正文。

#### Scenario: 阶段首推红线层与清单

- **WHEN** 当前 change 的 stage 由 `propose` 变为 `implement`，声明域含 `daily-report`
- **THEN** 以增量 steer 消息送达 `daily-report` 的红线层（顶层加粗句逐行）+ 本阶段路由清单
- **AND** 该消息不含 `daily-report` 约束节的整节正文

#### Scenario: 同阶段不重复注入

- **WHEN** 连续 5 个回合当前 stage 均为 `implement` 且声明域未变
- **THEN** 只在首个回合注入过一次，后续 4 个回合零注入

#### Scenario: 路由清单含取数命令

- **WHEN** 注入 `implement` 阶段的路由清单
- **THEN** 清单含 `daily-report` 的文档路径与节名
- **AND** 清单含 `bash scripts/harness/constraint-digest.sh --brief --domains daily-report` 命令形式

#### Scenario: 不再注入整节正文

- **WHEN** 用户输入中包含某业务域的关键词（如「discovery」）
- **THEN** 扩展 MUST NOT 因此注入该域约束节的整节正文

#### Scenario: 编辑路径命中不再注入正文

- **WHEN** agent 编辑了命中某 flow 文档 `doc-impact-applies` 标签的路径
- **THEN** 扩展 MUST NOT 因此注入该文档正文
- **AND** 该路径对应的域 SHALL 出现在本阶段路由清单中（引导模型自行 read）

### Requirement: 约束文档读取记账与归档对账

extension SHALL 在 agent `read` 工具调用**成功完成**时（tool_execution_end 且非 error）记录 `constraint.read` 事实库事件，payload 含被读文档的仓库根相对路径与读取时刻所属的 boundChange（可空）；发起即失败（路径拼错/文件不存在）或未回执的读取 MUST NOT 记账。带 offset/limit 的局部读取成功送达内容时同样算读（宽松方向）。

归档对账 SHALL 由独立脚本 `bash scripts/harness/constraint-read-trace.sh <change-dir>` 执行，并 SHALL 被归档门禁（`spec-gate`）与归档就绪自查（`archive-readiness.sh`）共同调用，与其余归档检查项共用同一事实源。

对账口径：对该 change 声明的**每一个**业务域，其 flow 文档 SHALL 在该 change 的实现期内被 `read` 过。证据 SHALL 同时满足：① 归属该 change（事件 change 列 = 该 change 名——他会话/未绑定会话的读取不算证据）；② 落在实现期窗内（起点 = 该 change 首条 implementation 族 `stage.set`（implement/verify/archive）的 ts；无记录时回退 change 创建时刻，宽松不误红；终点 = 当前时刻）。判定的「读文档」SHALL 认成功 `read` 的该文档路径（整档或局部均算，不要求只读该节）。

未读 SHALL 为红项，输出 SHALL 含：未读的域名、对应文档路径、以及取数指引 `bash scripts/harness/constraint-digest.sh --brief --domains <域>`。

legacy change（无 `stage.md`）SHALL 豁免本对账，MUST NOT 因此阻断归档。

#### Scenario: 读过则对账通过

- **WHEN** 某 change 声明域 `daily-report`，事实库存在归属该 change 且落在其实现期窗内的 `constraint.read` 事件（`docs/reference/flow/daily-report.md`）
- **THEN** `constraint-read-trace.sh <change-dir>` 退出码 0

#### Scenario: 未读过则红并给取数指引

- **WHEN** 某 change 声明域 `scheduler`，但事实库无归属该 change 的 `docs/reference/flow/scheduler.md` 成功读取记录
- **THEN** `constraint-read-trace.sh <change-dir>` 退出码 1
- **AND** 输出含域名 `scheduler`、文档路径、以及 `constraint-digest.sh --brief --domains scheduler` 取数指引

#### Scenario: 他会话的读取不算证据

- **WHEN** 某 change 声明域 `daily-report`，而 `daily-report.md` 的 `constraint.read` 事件归属其他 change（change 列不同）
- **THEN** 对该 change 对账仍为红（未读）

#### Scenario: 规划期的读取不算证据

- **WHEN** 某 change 声明域 `daily-report`，其 `constraint.read` 事件归属该 change但 ts 早于该 change 首条 implementation 族 `stage.set` 的 ts
- **THEN** 对该 change 对账仍为红（未读）

#### Scenario: 失败读取不记账

- **WHEN** agent 发起某 flow 文档的 `read` 但执行失败（tool_execution_end 为 error）
- **THEN** 事实库不新增该文档的 `constraint.read` 事件

#### Scenario: legacy change 豁免对账

- **WHEN** 某 change 目录无 `stage.md`（legacy）
- **THEN** `constraint-read-trace.sh <change-dir>` 退出码 0 且输出明示豁免

#### Scenario: 归档门禁接入对账

- **WHEN** agent 执行 `openspec archive <name>` 且对账脚本报红
- **THEN** 归档被阻断，reason 含未读域清单
