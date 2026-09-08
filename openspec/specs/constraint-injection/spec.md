# constraint-injection Specification

## Purpose
TBD - created by archiving change port-constraint-injection. Update Purpose after archive.

## Requirements

### Requirement: 档位识别与 change 绑定

extension SHALL 通过 `input` 事件识别阶段命令设置会话内档位（`requirements` / `implementation`），命令集覆盖本仓 openspec 斜杠命令（`/opsx-*`、`/skill:openspec-*`）与对应 skill 文件读取（`tool_execution_start` 的 read 路径命中 skill 目录）。

档位 SHALL 绑定活跃 change，绑定规则分档位：**输入提及优先**（命令参数/输入文本/写 change 目录文件时修正）；**mtime 最新兜底仅 implementation 档**（apply/verify/archive 无参语境）；requirements 档（explore/propose 新想法语境）SHALL NOT mtime 兜底——未明确提及时不绑定（关键词命中源不含无关 change 文本，pin_finding 落 research 库）。未激活档 SHALL 不显示、不分析任何 change。绑定 change 归档或删除后 SHALL 自动回落未激活档；agent 写 `openspec/changes/<name>/` 下文件时 SHALL 修正绑定。

档位状态 SHALL 持久化：每次档位激活（命令命中或 skill 路径命中）时 MUST 向事实库记 `mode.set` 事件（payload 含 mode 与绑定 change）。会话边界的恢复按 reason 区分，**三条恢复路径（resume / reload / startup）均 SHALL 仅按同 sessionId 的最近一条 `mode.set` 恢复，MUST NOT 全局兜底**（多 pi 窗口并行下，全局最新 mode.set 几乎必然属于其他会话——实测 2026-08-23 探索窗口 reload 被灌入他窗口 implementation 档）：`resume`（/resume 切换目标会话，sessionId 即目标会话 id）与 `reload`（同会话扩展重载）SHALL 清零后按同 sessionId 恢复；`startup` SHALL NOT 清零（pi-subagents 派发共用模块实例），但档位为空时 SHALL 按同 sessionId 恢复（pi 进程重启恢复会话的真实路径）；`new`/`fork` SHALL 维持清零语义（不恢复）。无本会话 mode.set 记录、或无法恢复（记录过期、绑定 change 目录已不存在）MUST 回落未激活档。恢复的档位与绑定遵循既有回落规则（change 归档后自动回落、写 change 目录修正绑定）。

#### Scenario: 斜杠命令激活档位

- **WHEN** 用户输入 `/opsx-apply add-change-scope`
- **THEN** 档位切换为 implementation 并绑定 change `add-change-scope`

#### Scenario: skill 读取激活档位

- **WHEN** agent 自动 read `.agents/skills/openspec-propose/SKILL.md`
- **THEN** 档位切换为 requirements（skill 路径 signal 命中）

#### Scenario: change 归档后回落

- **WHEN** 档位绑定的 change 目录已不存在（已归档）
- **THEN** 下一次 `before_agent_start` 前档位回落未激活，不再注入该 change 相关约束

#### Scenario: explore 新会话不绑定无关 change

- **WHEN** 新会话读 openspec-explore skill 激活 requirements 档，输入未提及任何 change，且存在 mtime 更新的其他 change 目录
- **THEN** 注入块活跃变更显示「无」，关键词命中源不含无关 change 文本，pin_finding 落 research 库而非 mtime 最新 change

#### Scenario: requirements 档提及后正常绑定

- **WHEN** requirements 档下用户输入提及某 change 名（如 `/opsx-continue <name>`）
- **THEN** 档位绑定该 change，其探索发现与约束照常注入

#### Scenario: apply 中断后 resume 恢复档位

- **WHEN** implementation 档会话（绑定 change X）中断后以 `session_start{reason:"resume"}` 恢复，事实库存在该会话线最近的 mode.set 且 change X 目录仍存在
- **THEN** 档位恢复为 implementation 并绑定 X，约束注入与 explore-findings 注入不丢失，无需重新触发 `/opsx-apply`

#### Scenario: pi 重启恢复会话后档位恢复（真实路径）

- **WHEN** pi 进程退出后重启并恢复原会话（`session_start{reason:"startup"}`，sessionId 与此前 mode.set 记录一致，模块档位为空）
- **THEN** 档位按同 sessionId 的最近一条 mode.set 恢复（含 change 绑定），约束注入不丢失

#### Scenario: 全新 pi 会话启动不继承其他会话档位

- **WHEN** pi 冷启动打开全新会话（reason=startup，该 sessionId 无任何 mode.set 记录），而事实库存在其他会话的 mode.set
- **THEN** 档位为未激活（startup 恢复无全局兜底）

#### Scenario: 无自身档位历史的会话 reload/resume 不继承他窗口档位

- **WHEN** 某会话自身无任何 mode.set 记录（新窗口探索会话），执行 `/reload` 或 `/resume`（reason=reload/resume），而事实库全局最新一条 mode.set 属于其他并行窗口（如另一窗口正在 apply 某 change）
- **THEN** 该会话档位为未激活（三条恢复路径均仅同 sessionId 取数，MUST NOT 全局兜底）

#### Scenario: 子线程派发不清零主会话档位

- **WHEN** 主会话 implementation 档激活，pi-subagents 派发子线程触发 `session_start{reason:"startup"}`（共用模块实例）
- **THEN** 主会话档位与命中保持不变（不清零、不重复恢复）

#### Scenario: reload 后档位恢复

- **WHEN** 档位激活的会话执行 `/reload`（reason=reload，sessionId 不变且有 mode.set 记录）
- **THEN** 清零后按同 sessionId 恢复档位与 change 绑定，无需重新触发阶段命令

#### Scenario: resume 无法恢复回落未激活

- **WHEN** resume 时事实库无可用 mode.set（无本会话记录/过期），或记录绑定的 change 目录已不存在
- **THEN** 档位回落未激活（仅注入索引），不报错

#### Scenario: 新会话不继承档位

- **WHEN** 以 `session_start{reason:"new"|"fork"}` 开始会话，此前另一会话刚记录过 mode.set
- **THEN** 档位为未激活，不从事实库恢复

### Requirement: 每 turn system prompt 强制注入

`before_agent_start` SHALL 按档位命中将约束文档追加进 system prompt：未激活档仅注入索引文档；激活档注入 baseDocs + 业务域声明（**红线层**）+ 关键词命中（全节）+ JIT 路径命中（全节）。**关键词命中与 JIT 命中 SHALL 会话内只增不减**（命中不因输入滚动窗词条滚出而移除、注入顺序稳定，保 system prompt 前缀缓存——系统提示变一字则全部历史缓存报废）。注入块 SHALL 标注「与 AGENTS.md 优先级宪法冲突时以宪法为准」。

关键词命中源 SHALL 仅含最近用户输入（滚动窗），change 产物全文（proposal/design/tasks）MUST NOT 触发关键词命中——harness/AI 类 change 的正当词汇与业务域关键词本质重叠，隐式推断必致误注入。ASCII 关键词 SHALL 按词边界整词匹配（CJK 关键词保持子串匹配）。

业务域声明 SHALL 解析 proposal.md 头部的 `<!-- constraint-domains: <域>, ... -->` HTML 注释标记（可多行合并），域名合法值 SHALL 为 `docs/reference/flow/*.md` 的 basename；未知域名 SHALL 宽容忽略并在状态提示；无声明 SHALL 不注入任何 flow 约束节并在状态栏提示（不阻断）。

**声明域注入 SHALL 提取目标域「业务约束与不变量」节的红线层**：节内每条约束的首行加粗红线句（每条约束以列表项呈现、首词组加粗为自含红线句，格式规范由 doc-authoring 定义）；红线层按原文顺序逐行注入，尾部 SHALL 附细节层取回指引（read 该文档约束节）。**红线层为空（0 条提取）或拼接后低于节级最小字节下限时 SHALL 回退注入全节**（fail-safe，与「节不存在回落全文」同族）。声明域注入每回合从文件重解析，SHALL NOT 进入只增不减粘性集合。声明域 `constraint.inject` 记账 payload SHALL 附层级标记（`redline` / `full`），bytes 为实际注入层级字节数。

JIT pathSignals SHALL 复用文档既有 `doc-impact-applies` frontmatter 标签生成（standard 与 flow 文档统一单一真相源，不发明第二套映射），`.pi/constraint-injection.json` MUST NOT 手写 pathSignals。JIT 与关键词命中的注入内容 SHALL 为完整约束节（细节层经此通道按需到达模型，声明层只供红线层）。

节级注入 SHALL 设最小字节下限（`minSectionBytes`，配置缺省 512）：提取的节内容低于下限时 SHALL 回退注入该文档全文（fail-safe），避免文档编辑中间态注入残缺节；红线层提取同样受该下限约束（低于下限回退全节）。回退全文时 `constraint.inject` 记账 bytes SHALL 为实际注入的全文字节数（不虚标节字节数）。

注入块（文档节 + change 级文件）总量 SHALL 受 `budgetBytes` 预算限制（配置缺省默认 32768 字节）。超预算时 SHALL 按命中信号强度分层降级（先降推断信号、后降声明信号：关键词命中节 → JIT 命中节 → change 级文件 digest 收紧 → 域声明节 → change 级文件降占位；baseDocs 与注入块 header SHALL NOT 降级；域声明节的降级基线为其红线层形态）。降级 MUST NOT 真丢文档——每个命中文档至少保留一行占位（标题 + `read` 全文路径），模型可自行补取。降级决策 SHALL 确定性（相同命中集合 + 相同文档内容 + 相同预算 → 相同降级结果，顺序按命中信号分层与首次命中顺序稳定）。超预算发生时注入块头部 SHALL 列出已降级路径（模型可见省略通知）。记账时被降级文档的 `constraint.inject` payload MUST 附降级标记与降级后字节数；预算降级到占位行的 explore-findings SHALL NOT 记 `pin.read`（模型未见 pin 标题）。

#### Scenario: 实现档注入生效

- **WHEN** implementation 档激活且 proposal.md 声明 `constraint-domains: daily-report`
- **THEN** system prompt 追加 daily-report.md「业务约束与不变量」节的**红线层**（各条约束首行红线句 + 细节层取回指引），非全节全文，模型无法绕过

#### Scenario: JIT 路径细化

- **WHEN** implementation 档激活且 agent edit `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`
- **THEN** 命中 daily-report.md 头部 `doc-impact-applies` 标签所含路径前缀，该文档「业务约束与不变量」节**全文**（细节层）会话内追加注入（json 无手写 pathSignals）

#### Scenario: flow 文档节级注入

- **WHEN** 档位激活且声明域命中某 flow 域（注入红线层），或最近输入 / JIT 路径命中某 flow 域（注入全节）
- **THEN** 声明域注入为该域红线层（逐行 + 指引尾行），关键词 / JIT 命中注入为该域完整约束节（细节层经命中通道到达），两种形态节尾均附全文路径指引

#### Scenario: 节残缺时回退全文

- **WHEN** 某 flow 文档「业务约束与不变量」节处于编辑中间态仅 133B（低于 minSectionBytes 下限），全文完整
- **THEN** 注入回退为该文档全文，constraint.inject 记账 bytes 为全文字节数（残缺节不进 system prompt）

#### Scenario: 红线层为空回退全节

- **WHEN** 某声明域约束节尚无规整列表项加粗红线句（红线层提取 0 条），全文完整
- **THEN** 该域声明注入回退为全节，constraint.inject 记账 payload 层级标记为 `full`、bytes 为全节字节数

#### Scenario: 红线层低于最小字节回退全节

- **WHEN** 某声明域红线层拼接后 380B（低于 minSectionBytes 缺省 512）
- **THEN** 该域声明注入回退为全节（残缺红线层不进 system prompt），记账 bytes 如实为全节

#### Scenario: 声明注入层级记账

- **WHEN** 某 change 声明两域，一域红线层 1.8K、另一域红线层回退全节 12.9K
- **THEN** 产生两条 reason=declaration 的 constraint.inject，payload 分别附 `layer:redline`（bytes≈1843）与 `layer:full`（bytes≈12900）

#### Scenario: 未激活档仅索引

- **WHEN** 会话未识别任何档位（普通问答/research 语境）
- **THEN** 仅注入约束索引文档，不做域声明注入与关键词命中

#### Scenario: change 文本不触发关键词命中

- **WHEN** implementation 档激活且 change 的 proposal/design/tasks 全文含 `stage`、`topic`、`digest`、`模型` 等历史撞车词，但 proposal 未声明对应域、最近输入亦未提及
- **THEN** 不注入 semantic-board / topic-graph / daily-report / ai-summary 任一域约束节

#### Scenario: ASCII 关键词词边界整词匹配

- **WHEN** 最近输入含英文单词 "stage"（不含独立词 "tag"）
- **THEN** 不命中 semantic-board 域关键词

#### Scenario: 命中只增不减（缓存稳定）

- **WHEN** 档位激活且用户输入「日报」命中 daily-report 域，后续输入使该词滚出最近输入窗
- **THEN** 该文档仍留在注入块中，块内既有内容与顺序字节不变

#### Scenario: 无声明不注入并提示

- **WHEN** implementation 档激活且绑定 change 的 proposal.md 无 `constraint-domains` 标记
- **THEN** 不注入任何 flow 约束节，状态栏 widget 提示「无域声明」，不阻断会话

#### Scenario: 未知域名宽容忽略

- **WHEN** proposal.md 声明 `constraint-domains: daily-report, not-a-domain`
- **THEN** 注入 daily-report 域约束节红线层，忽略 `not-a-domain` 并在状态提示

#### Scenario: 超预算分层降级

- **WHEN** budgetBytes=16384 且当前命中文档节合计 43K（超预算）
- **THEN** 依分层顺序降级（关键词命中节先于 JIT 命中节、change 级文件先 digest 收紧、域声明节后降）直到总量的预算内，baseDocs（索引）与注入块 header 不降级

#### Scenario: 降级永不真丢

- **WHEN** 预算极小（如 budgetBytes=2048）且命中 3 个 flow 节
- **THEN** 每个命中文档至少保留一行「标题 + read 路径」占位，无任何文档从注入块中完全消失

#### Scenario: 模型可见省略通知

- **WHEN** 本回合发生降级
- **THEN** 注入块头部列出全部已降级路径，模型可据此 read 对应文档补取全文

#### Scenario: 降级确定性（缓存友好）

- **WHEN** 命中集合与文档内容、预算均未变，连续两个回合重建注入块
- **THEN** 两次降级结果与注入块字节完全一致

#### Scenario: 降级记账

- **WHEN** daily-report 约束节（红线层）本回合被降级为占位行
- **THEN** 产生一条 constraint.inject，payload 附降级标记与降级后字节数（区别于未降级回合）

### Requirement: pin_finding 落点解析

extension SHALL 注册 `pin_finding` 工具持久化探索发现，落点与 research-retention 规则对齐，三级解析：

1. 显式 `change` 参数（目录存在）或档位激活（档位绑定优先；mtime 兜底仅 implementation 档）→ 活跃 change 的 `explore-findings.md`，implementation 档自动注入；
2. 无档且传 `topic` → `docs/research/<topic>/explore-findings.md`；
3. 无档无 topic → 通用池单文件 `docs/research/explore-findings.md`。

任何落点 SHALL NOT 写入 `docs/experience/`。

#### Scenario: 实现阶段自动注入发现

- **WHEN** requirements 档 pin 了「告警表结构」发现，随后档位切换为 implementation
- **THEN** 该 explore-findings.md 内容随 system prompt 注入，实现阶段无需重探

#### Scenario: research 语境不落 change

- **WHEN** 无激活档位且未传 change 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/` 下（传 topic 落 `<topic>/`，未传落通用单文件），不写入 `openspec/changes/`

#### Scenario: research 语境无 topic 落通用池

- **WHEN** 无激活档位且未传 change 与 topic 参数时调用 pin_finding
- **THEN** 落点为 `docs/research/explore-findings.md` 通用池单文件

### Requirement: smoke test 覆盖纯函数

extension 的纯逻辑（档位匹配、栈判定、速览提取、**节提取**、**红线层提取与最小字节回退**、**节最小字节判定与全文回退**、**命中粘性**、落点三级解析、命令匹配）SHALL 有可脱离 pi harness 运行的 smoke test（node 直跑 .cjs 模式，同源项目 `tests/*.smoke.cjs` 实践）。

#### Scenario: smoke test 直跑

- **WHEN** 执行 smoke test 脚本（不启动 pi）
- **THEN** 全部断言通过，退出码 0

#### Scenario: 红线层提取纯函数直跑

- **WHEN** smoke test 对红线层提取函数传入含 `N. **句**：细节` 规整列表项的约束节文本
- **THEN** 提取结果为各列表项首个加粗块的逐行序列（保留原文顺序），断言通过

#### Scenario: 红线层零提取回退

- **WHEN** smoke test 对红线层提取函数传入无列表项加粗（纯段落）的约束节
- **THEN** 判定结果为回退全节（与节不存在回落同族），断言通过

#### Scenario: 节提取回落

- **WHEN** 配置了 `section` 的文档内不存在该 `## 节名`（文档结构变更未同步配置）
- **THEN** 回落全文注入（不报错、不静默跳过，fail-safe）

#### Scenario: 节低于最小字节下限回退全文

- **WHEN** smoke test 对节提取纯函数传入低于 `minSectionBytes` 的节内容与完整全文
- **THEN** 判定结果为回退全文（与节不存在回落同族），断言通过
