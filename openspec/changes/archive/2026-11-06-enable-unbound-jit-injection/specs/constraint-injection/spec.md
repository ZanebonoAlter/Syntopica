## MODIFIED Requirements

### Requirement: 混合注入通道（稳定层 system prompt + 动态层 steer 消息）

约束注入 SHALL 按内容变化频率分两层通道送达（替代原「每 turn 全量重建 system prompt 注入块」——实测 system prompt 任何字节变化（含尾部追加）均使其后全部对话历史的前缀缓存失效）：

**稳定层（system prompt，`before_agent_start` 追加）**：仅含变化频率与档位生命周期对齐的内容——索引文档（constraints-index）+ mode-base 基础文档 + 声明域红线层。稳定层在档位生命周期内（同一档位且绑定 change 不变）SHALL 字节恒定：内容不随对话输入、编辑路径、findings 文件更新而变化；仅在档位切换/绑定修正时整体变化一次（一次性缓存成本）。声明域红线层仍每回合从文件重解析内容（proposal 编辑生效），但重解析结果与上次不同时 SHALL 以 steer 消息通知差异部分而不改写稳定层（稳定层保持激活时刻快照）。

**动态层（steer 消息，`pi.sendMessage` + `deliverAs:"steer"`）**：关键词命中全节、JIT 路径命中全节、change 级文件（explore-findings / 词汇表）、稳定层差异通知。**未激活档（`mode = null`）的编辑路径命中 SHALL 同样经本层追加**（见 Requirement「无档位编辑路径 JIT 追加注入」）；未激活档关键词通道保持关闭。动态层 SHALL 事件驱动发送：维护已发送内容指纹，仅当指纹变化（新命中、文件更新导致内容变化、档位切换后首轮）时发送增量消息，相同指纹零发送（MUST NOT 每 turn 重发）。消息 SHALL 标注类型与来源（customType），display 渲染 SHALL 受控（默认折叠/精简展示，不整块刷屏）。steer 消息参与 LLM 上下文；档位切换时动态层全量集合重置（新 change 的命中文档与旧 change 无关）。

**指纹重置按 regime 判定**：已发送指纹 SHALL 随投递记录其 regime——投递时 `stableSnapshotKey(mode, boundChange)` 快照键。档位/绑定可在 turn 中途切换（`tool_execution_start` 的 skill 文件读取激活、change 目录编辑兜底绑定），快照重建则延迟到下一 turn 的 `before_agent_start`；延迟重建时的指纹清空 SHALL 仅在「指纹 regime ≠ 新快照 key」时发生——同 turn 内已在新 regime 下送达的 JIT 条目 MUST NOT 被延迟重建当作旧 regime 残留清空而原样重投（相同内容 + 相同 regime = 零重发，无论快照是否重建）。fork / 子线程继承时指纹及其 regime SHALL 随 channel 一并拷贝，继承方的快照重建同样按 regime 判定清空（父会话已投递且 regime 一致的条目不重发）。

**compaction 补偿**：`session_compact` 事件后，extension SHALL 重发一次当前约束快照（稳定层摘要 + 动态层当前有效集合，一条消息），指纹不因重发而跳过。快照重发 SHALL 在 compact 完成后的下一次注入时机合并执行，不额外打断 turn。**例外**：无 `before_agent_start` 事件的子线程（pi-web 托管子会话）MUST NOT 因 compact 重发——该通道不存在，不伪造。

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

#### Scenario: turn 中途切档后同 turn 已投递内容不被延迟重建重投

- **WHEN** 会话未激活档的某 turn 内，agent read requirements 类 skill 文件（`tool_execution_start` 激活档位），同 turn 随后 edit 命中某文档 `doc-impact-applies` 标签触发 JIT 即时投递（该节经 steer 消息送达，指纹以新档位 regime 记录）；下一 turn `before_agent_start` 延迟重建稳定层快照（快照 key 从未激活档变化为当前档位）
- **THEN** 稳定层按新档位重建（mode-base 一次性变化），但动态层对该已投递节零重发（指纹 regime 与新快照 key 一致，不清空不重投）；`constraint.inject` 记账中该节在同会话代内不出现第二条同字节的 jit-path 记录

#### Scenario: 绑定切换后首轮仍全量重投

- **WHEN** 档位激活且已投递若干动态条目（指纹 regime 为 `<mode>|<changeX>`），turn 中途 agent 写 change Y 目录触发兜底绑定（edit-dir），下一 turn `before_agent_start` 快照 key 变为 `<mode>|<changeY>`
- **THEN** 指纹 regime 与新快照 key 不一致 → 指纹清空、动态层全量集合按新 change 语境重投（既有设计行为保持：新 change 的命中文档与旧 change 无关）

#### Scenario: fork 继承的指纹不被陈旧快照 key 重投

- **WHEN** 父会话在 turn 中途切档后已 JIT 投递若干条目（指纹 regime 为新档位 key，但快照 key 尚为切档前的旧值），子会话 fork 继承父会话状态（含 channel 与指纹 regime），子会话首个 turn 快照重建（快照 key 与继承的旧快照 key 不一致）
- **THEN** 指纹 regime 与新快照 key 一致 → 不清空、父会话已投递条目不重发（fork 语义：父会话已送达的动态层条目不重复）

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

## ADDED Requirements

### Requirement: 无档位编辑路径 JIT 追加注入

未激活档（`mode = null`）会话在 `write` / `edit` 工具调用命中文档头部 `doc-impact-applies` 路径标签（前 15 行内首条标签生效）时，SHALL 把命中文档追加进本会话的动态注入集合，并以动态层 steer 消息送达该文档完整约束节（节尾附全文路径指引，与档位激活时的 JIT 形态一致）。

**只开路径通道**：关键词匹配（`matchText`）、关键词域限定（`allowedKeywordDocs`）、change 绑定（`boundChange` / turn 绑锁 / `mode.set`）SHALL 继续以档位为开关——未激活档 MUST NOT 因编辑路径命中而绑定 change、MUST NOT 因对话输入关键词而注入域约束。

**去重**：同一会话同一文档 SHALL 只追加一次（粘性集合，只增不减）；未命中任何标签路径时 MUST 零注入。

**记账**：无档 JIT 注入 SHALL 记 `constraint.inject`，`reason = jit-path`，并在 payload 追加 append-only 字段 `tier`（值 `none` / `requirements` / `implementation`，自报会话档位）——现有 `mode` 字段表示文档形态（`section`/`digest`/`full`），MUST NOT 复用；历史记录无 `tier` 字段时统计 SHALL 计「未知」并剔除分母。

**预算**：无档 JIT 追加内容 SHALL 受既有 `budgetBytes` 约束，降级顺位与档位激活时一致（关键词 → JIT → change 级文件 → 声明域），降级 MUST NOT 真丢文档。

**子线程**：未激活档的子线程编辑命中标签路径时 SHALL 同样追加（子线程无 `before_agent_start`，经 steer 即时投递），使子线程在不继承档位时也能按自己的真实目标路径自愈。

#### Scenario: 未激活档编辑命中业务路径

- **WHEN** 会话未激活档，agent edit `backend-go/internal/platform/airouter/embedding.go`（命中 ai-summary.md 头部标签，节字节 12.5KB 在默认预算内）
- **THEN** 一条 steer 消息送达 ai-summary.md「业务约束与不变量」完整节（含全文路径指引），`constraint.inject` 记 `reason=jit-path`、`tier="none"`

#### Scenario: 未激活档编辑未映射路径

- **WHEN** 会话未激活档，agent edit `scripts/harness/constraint-digest.sh`（无任何 `doc-impact-applies` 标签命中）
- **THEN** 零动态层发送，零 `constraint.inject` 记账

#### Scenario: 未激活档同域多次编辑只注一次

- **WHEN** 会话未激活档连续 edit 同一域下的两个不同文件（均命中同一 flow 文档标签）
- **THEN** 该文档只追加一次，第二条 steer 消息不重发（指纹去重）

#### Scenario: 未激活档 JIT 不绑定 change

- **WHEN** 会话未激活档，agent edit `openspec/changes/<某 change>/tasks.md` 且该路径同时命中某文档标签
- **THEN** 可追加该文档约束，但 `boundChange` 仍为空、不产生 `mode.set` 事件、档位仍为未激活

#### Scenario: 未激活档 JIT 受预算降级

- **WHEN** 未激活档会话命中 JIT 文档合计 40K，`budgetBytes = 32768`
- **THEN** 按既有分层顺位降级到预算内，降级条目在消息头部列出路径，文档至少保留「标题 + read 路径」占位

#### Scenario: 子线程未继承档位时按自身路径自愈

- **WHEN** 主会话未激活档派发子线程（子线程继承 `mode = null`），子线程 edit `backend-go/internal/platform/airouter/embedding.go`（命中 ai-summary.md 标签）
- **THEN** 子线程收到 ai-summary.md 完整约束节 steer 消息（节字节 12.5KB 在默认预算内；不必依赖主线程任务文本里的 brief）

#### Scenario: legacy 通道不即时投递

- **WHEN** `channel = legacy` 且未激活档会话 edit 命中某业务域路径
- **THEN** 本 turn 不发送 steer 消息；下一 turn `before_agent_start` 时含该文档的计划随 system prompt 注入（既有 legacy 语义，不新增通道）

#### Scenario: 未激活档 compact 后重发 JIT 集合

- **WHEN** 未激活档主会话已 JIT 命中两域，发生 compaction
- **THEN** 下一注入时机重发索引 + 当前 JIT 集合（`pendingSnapshot` 既有机制），红线不因压缩丢失

#### Scenario: 无 before_agent_start 的子线程 compact 不重发

- **WHEN** 未激活档子线程（无 `before_agent_start` 事件）已 JIT 命中一域后发生 compaction
- **THEN** 该子线程不因 compact 重发（已知边界，不伪造通道）；同文档重复编辑也不重投（去重语义），丢失内容需显式 read 或由主线程 brief 补齐

#### Scenario: 同进程 fork 继承命中集不重投

- **WHEN** 父会话未激活档已 JIT 投递 daily-report 节，进程内 fork 子会话继承 `jitDocHits` 与指纹
- **THEN** 子会话首投只含稳定层索引，不重投父已送达的 daily-report 节（指纹 regime 一致）

#### Scenario: reload 后命中集清零

- **WHEN** 未激活档会话已 JIT 命中若干域，发生 reload / 会话状态重建
- **THEN** `jitDocHits` 清零，不凭历史命中集重投；后续编辑重新命中
