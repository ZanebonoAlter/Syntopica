## Purpose

以语义版块为对象的两阶段认知能力：默认生成可扫描的版块简报，帮助用户看清关键变化、泳道关系与不确定项；用户选择具体问题后，再通过多假设、支持与反证生成可核查的深度调查。系统不得预设同版块泳道必然构成统一因果系统。

## ADDED Requirements

### Requirement: 版块简报手动触发

系统 SHALL 以单个语义版块为对象手动生成版块简报，MUST 复用版块 `enrichment_enabled` 门槛。本期 SHALL NOT 定时生成。默认触发只读取内部态势卡与历史认知记录，MUST NOT 自动调用外部搜索，也 MUST NOT 自动继续生成深度调查。

#### Scenario: 版块未开启增强时拒绝

- **WHEN** 版块 `enrichment_enabled=false` 时用户触发版块简报
- **THEN** 请求被拒绝且不产生任何结果

#### Scenario: 默认触发只生成简报

- **WHEN** 用户点击“分析板块”
- **THEN** 系统生成一份 `board_brief` 快照，不调用 web 搜索、不产生 `board_investigation`，并向用户展示可继续调查的问题

#### Scenario: 多次触发保留独立简报

- **WHEN** 用户对同一版块再次生成简报
- **THEN** 新旧简报均作为不可变快照保留，新简报不覆盖旧简报

### Requirement: 简报与调查异步任务可区分

版块简报和调查 SHALL 脱离触发 HTTP 请求在后台运行，每次触发 SHALL 返回唯一 job id、job kind 与 board id，前端 SHALL 可按 job id 轮询 running/finished/error/result_id。同一版块的简报与调查 MUST 串行：已有任一任务运行时，新触发返回 409 并携带当前任务身份；不同版块可并行。客户端断连 MUST NOT 取消后台任务。

#### Scenario: 简报触发立即返回

- **WHEN** 用户触发简报后立即离开页面
- **THEN** 接口返回 `job_kind=board_brief` 的任务身份，后台继续完成，用户重进页面可恢复该 job 的轮询

#### Scenario: 调查状态不冒充简报状态

- **WHEN** 用户触发某问题调查
- **THEN** 状态返回 `job_kind=board_investigation` 与独立 job id，完成后 result_id 指向调查子结果，前端不把它当作新简报

#### Scenario: 同版块任务防重入

- **WHEN** 同一版块已有简报或调查运行时再次触发任一种任务
- **THEN** 新任务不启动，接口返回 409 及当前 job id/job kind，前端恢复当前任务轮询

### Requirement: 简报反映观察而非强制立论

版块简报 SHALL 包含：`summary`、`observations`、`relationships`、`uncertainties`、`research_questions`、`lane_refs`。简报 MUST NOT 强制包含 thesis、固定反转句式、机制层、历史类比或系统重定位。每条 observation SHALL 指向相关泳道并标明依据或截止时间。

#### Scenario: 有素材但没有统一关系

- **WHEN** 多条泳道均有近期素材，但没有足够证据证明共同驱动或因果传导
- **THEN** 简报照常生成关键观察，并明确写“暂未发现统一关系”或等价结论，不硬造跨泳道命题，也不降级为 sparse

#### Scenario: 存在多个并行趋势

- **WHEN** 板块内同时存在方向不同且互不从属的变化
- **THEN** 简报分别展示这些趋势及其分化关系，不把它们压缩成单一底层原因

#### Scenario: 全部素材稀薄

- **WHEN** 所有活跃泳道均缺少可用近期观察
- **THEN** 简报诚实展示素材不足，不生成研究问题，不自动启动调查

### Requirement: 泳道关系有类型、有依据、可为空

系统 SHALL 仅在内部素材支持时声明跨泳道关系。关系类型 SHALL 为 `common_driver | possible_causal | divergent | context_only | unclear`；每条关系 SHALL 带相关 lane id、解释、置信度与观察依据。同期发生 MUST NOT 自动等同于因果关系。

#### Scenario: 仅语义相关

- **WHEN** 两条泳道只因属于同一概念版块而相关，缺少共同驱动或传导证据
- **THEN** 系统标为 `context_only` 或不建立关系，MUST NOT 标为 `possible_causal`

#### Scenario: 因果证据不足

- **WHEN** 素材显示两条泳道可能存在传导但中间环节缺失
- **THEN** 系统最多标为 `possible_causal` 且低/中置信，并把缺失环节写入 uncertainties

#### Scenario: 关系引用幽灵泳道

- **WHEN** LLM 返回不属于当前版块活跃集合的 lane id
- **THEN** 该关系与引用被拒绝或清理，其余合法内容继续保留

### Requirement: 简报提供可选择的研究问题

系统 SHALL 从观察、关系与未知项中生成 0-4 个具体问题式研究候选；每个候选 SHALL 包含问题、为什么值得调查及相关泳道。问题 MUST 可由证据支持或削弱，MUST NOT 只是抽象方法名。用户 MAY 自填问题替代候选。

#### Scenario: 用户选择候选问题

- **WHEN** 用户在简报中选择一个研究问题并点击“深入调查”
- **THEN** 调查以该问题及父简报为输入启动，不要求用户先接受任何预设命题

#### Scenario: 用户自填问题

- **WHEN** 用户输入一个自定义问题并触发调查
- **THEN** 系统记录问题来源为 custom，并按与生成问题相同的调查链处理

#### Scenario: 没有值得调查的问题

- **WHEN** 简报只发现普通、彼此独立的变化且无关键未知项
- **THEN** `research_questions` 可为空，系统不把空列表视为失败

### Requirement: 深度调查必须评估竞争假设

调查 SHALL 在检索前生成 2-4 个竞争假设，且 MUST 至少包含一个 `is_null=true` 的零假设，用于表达“没有统一机制、各变化可分别解释”或与问题等价的朴素解释。每个假设 SHALL 先声明支持它需要什么证据、什么证据会削弱它以及适用范围；此阶段 MUST NOT 预先选定赢家。

#### Scenario: 零假设不可缺席

- **WHEN** 调查问题涉及多个泳道是否存在共同机制
- **THEN** 假设集合中至少有一个零假设；缺少零假设的 LLM 输出不得直接进入研究阶段

#### Scenario: 候选假设都很宏大

- **WHEN** LLM 只返回多个宏大结构解释而没有普通解释
- **THEN** 系统要求重试或机械补入零假设，MUST NOT 在原集合上直接研究

### Requirement: 调研同时寻找支持与反证

研究阶段 SHALL 使用内部泳道工具与可用外部工具，围绕全部假设统一搜集基础事实、支持证据、反证、替代解释与关键缺口。对于每个非零假设，系统 SHALL 至少尝试一次削弱该假设的检索或核查；工具不可用或无命中时 SHALL 记录 gap，不得伪造证据。证据质量与问题相关性优先于证据类型数量。

#### Scenario: 搜索词不得只有既定结论

- **WHEN** 研究 agent 生成检索计划
- **THEN** 计划同时包含中性事实查询和反证/替代解释查询，MUST NOT 只把某一假设的结论词拼进全部搜索词

#### Scenario: 只有同向材料

- **WHEN** 检索只获得支持某假设的新闻转述，未获得一手材料或反证
- **THEN** 该假设不得标为高置信 supported，结果明确记录证据偏向与缺口

#### Scenario: 外部工具不可用

- **WHEN** web_search 或 fetch_page 未配置或失败
- **THEN** 调查继续使用内部材料并标记外部核查缺口，不影响父简报

### Requirement: 调查结论可改写、拆分或放弃假设

最终综合 SHALL 根据研究结果为每个假设标记 `supported | plausible | insufficient | weakened | refuted`，并产出带 confidence、scope 与 boundary 的有限结论。综合阶段 MAY 修改、合并或拆分初始假设，也 MAY 判定所有非零假设证据不足。系统 MUST NOT 强制输出反转句式、历史类比、固定机制层数或宏大系统结论。

#### Scenario: 零假设最符合材料

- **WHEN** 研究只证明多条变化同期出现，未找到统一传导机制
- **THEN** 零假设可成为最可信解释，结论用直白语言说明各变化目前应分别理解

#### Scenario: 所有假设证据不足

- **WHEN** 支持与反证均不足以区分候选假设
- **THEN** 调查结论标为低置信/证据不足，列出下一步需要的材料，不强选赢家

#### Scenario: 初始假设被研究推翻

- **WHEN** 反证直接否定初始最显眼的解释
- **THEN** 最终结果将其标为 weakened/refuted，并允许结论与初始候选方向不同

### Requirement: 调查综合只修复可证明的单一根终止符

调查综合 SHALL 先按严格 JSON 解析。仅当输出已包含完整顶层末字段 `lane_refs`，词法扫描确认字符串与全部内部对象/数组均闭合且无错配、delimiter stack 恰好只剩根 `{`、最后有效字符为 `]` 时，系统 MAY 追加恰好一个 `}` 后继续同一套 schema 与证据校验，并 SHALL 在 generation meta 记录稳定修复原因。任何内部截断、字符串未闭合、括号错配、尾随正文或缺顶层末字段的输出 MUST NOT 被机械补全，继续既有纠错重试；两次仍失败不得落调查行。

#### Scenario: 完整综合只缺根对象右大括号

- **WHEN** `hypotheses`、`conclusion`、`evidence_chain` 与 `lane_refs` 均完整，字符串和内部括号均闭合，但响应只缺最外层最后一个 `}`
- **THEN** 系统只追加该单一终止符，记录修复原因并继续严格结构校验，无需浪费一次 LLM 重试

#### Scenario: 综合响应存在内部截断或括号错配

- **WHEN** 响应停在字符串/内部数组中、存在括号错配、带尾随正文，或尚未产出 `lane_refs`
- **THEN** 系统拒绝机械补全并进入纠错重试；第二次仍非法时保持 0 调查行

### Requirement: 跨 provider 证据字段归一不得产生悬空评估

lane 类型证据的规范持久字段 SHALL 为十进制字符串 `ref`。当且仅当 `ref` 为空时，parser MAY 将正整数且位于 JSON 安全整数域的 `lane_id` 归一为 `ref`，随后 MUST 执行相同的父简报 lane 白名单校验；显式 `ref` 与 `lane_id` 冲突时 SHALL 以 `ref` 为准且不得用别名掩盖非法值。证据清洗后，hypothesis 引用 SHALL 与存活 evidence 的 `supports/counters` 极性确定性合并；`supported` MUST 至少保留一条支持证据，`refuted` 或 `weakened` MUST 至少保留一条反证。违反该一致性 SHALL 进入结构纠错重试，第二次仍失败不得落库。

#### Scenario: fallback 使用 lane_id 表示合法泳道证据

- **WHEN** lane evidence 没有 `ref`、提供白名单内的安全正整数 `lane_id`，且 supports/counters 指向有效假设
- **THEN** 系统将其归一为字符串 `ref`，保留证据及双向假设引用，前端可展示证据并下钻该泳道

#### Scenario: 显式非法 ref 不被 lane_id 掩盖

- **WHEN** lane evidence 同时提供白名单外或非法 `ref` 与白名单内 `lane_id`
- **THEN** 系统以显式 `ref` 为准并剔除该证据，不用别名偷偷改写其身份

#### Scenario: 清洗后确定性评估失去全部对应证据

- **WHEN** `supported` 的支持引用或 `refuted/weakened` 的反证引用在白名单、同源或极性清洗后归零
- **THEN** 该综合视为结构非法并纠错重试，MUST NOT 持久化“没有证据但已证实/已推翻”的调查

#### Scenario: 零证据报告不猜测研究过程

- **WHEN** 合法调查最终没有通过核验、可展示的 evidence
- **THEN** 前端使用中性空态文案，不断言“没有采到材料”或虚构具体失败原因

### Requirement: 调查产物可核查且易读

`board_investigation` SHALL 包含父简报 id、问题、hypotheses、conclusion、evidence_chain、lane_refs 与 method_refs。报告首屏 SHALL 先展示问题、当前判断、置信度、适用范围和边界；支持证据、反证、缺口与来源详情 SHALL 可展开查看。抽象概念首次出现时 SHALL 说明其对应的具体行为、规则、数据或事件。

#### Scenario: 支持与反证分开显示

- **WHEN** 用户展开某个假设
- **THEN** 前端分别展示 support_evidence、counter_evidence 和 gaps，不把相反材料埋入同一段长文

#### Scenario: 避免重复机制长文

- **WHEN** 调查包含多个假设和结论
- **THEN** 前端不再连续重复渲染 argument.layers 与 depth.mechanism_layers；同一论点只在一个主位置表达

### Requirement: 简报与调查形成父子不可变快照

每份调查 SHALL 关联同版块的一份 `board_brief`，并持久化由规范化问题文本生成的 `question_key`；一份简报 MAY 派生多份调查。简报与调查均 append-only，不得因后续调查或追问被改写；追问继续以附属记录沉淀。

#### Scenario: 一份简报派生多份调查

- **WHEN** 用户分别调查同一简报中的两个问题
- **THEN** 系统产生两份独立 `board_investigation`，二者 parent 均指向该简报，互不覆盖

#### Scenario: 跨版块父结果被拒绝

- **WHEN** 调查请求引用另一个版块的简报 id
- **THEN** 请求被拒绝，不产生调查结果

### Requirement: 态势卡、新鲜度与质量信号继续作为内部输入

简报 SHALL 使用每活跃泳道一张态势卡并带 as_of 与质量信号；事实摘要取材顺序 SHALL 为 week lifeline → month lifeline → 有实质内容的近期 section 指纹 → 泳道描述 → 空，MUST NOT 拼接全部泳道全文。分析前 SHALL 对 month/year 有数据周期执行补全：缺行含首份先建、最后写入超过 72h 则重算；week 不参与补全检查，单次补全受限额约束且失败降级。质量信号只用于排序与详略，MUST NOT 直接证明泳道关系。需要细节的调查经泳道工具下钻并可读取 month/year 历史背景。

#### Scenario: 泳道多时简报素材受控

- **WHEN** 版块包含 ≥10 条活跃泳道
- **THEN** 输入仍按态势卡预算控制，输出只保留最重要观察，不按泳道逐条生成长文

#### Scenario: 低质量泳道不被静默删除

- **WHEN** 某泳道长期无命中或历史稀疏
- **THEN** 其卡片降权和缩短但仍可追溯，系统不得仅因质量低便从版块成员中删除

### Requirement: lane 引用与单泳道下钻

简报和调查中的 lane 引用 SHALL 为一等公民并校验属于当前版块。用户可从观察、关系、研究问题、假设证据处下钻对应泳道；下钻预填具体观察或研究问题，用户可修改。独立单泳道入口继续可用。

#### Scenario: 从简报观察下钻

- **WHEN** 用户点击某条 observation 的 lane 引用
- **THEN** 系统打开对应泳道聚焦入口，并以该 observation 作为可修改的预填问题

#### Scenario: 从调查证据下钻

- **WHEN** 用户点击某假设下的 lane 证据
- **THEN** 系统打开对应泳道并预填当前调查问题/证据说明，不把版块结论锁死为单泳道 lens

### Requirement: review 按结果种类隔离

新简报 SHALL 只与同版块上一份简报比较，记录观察、关系和不确定性的变化；调查只有在 `parent_result_id + question_key` 相同的重跑之间才自动比较。review SHALL NOT 在不同问题之间比较，也 SHALL NOT 回写新闻记忆。

#### Scenario: 第二份简报自动对比

- **WHEN** 同版块生成第二份 `board_brief`
- **THEN** review 对比两份简报的观察与关系变化，不读取 legacy thesis 作为上一份

#### Scenario: 不同问题调查不互比

- **WHEN** 同一简报下先后完成两个不同问题的调查
- **THEN** 系统不自动把二者作为前后认知版本比较

### Requirement: 旧论文式版块报告兼容

既有 thesis/argument/depth 形态的版块结果 SHALL 标记为 `legacy_board_analysis` 并保持只读可访问。新触发 SHALL NOT 再生成该形态；迁移 MUST NOT 重写或删除旧 JSON。

#### Scenario: 查看旧报告

- **WHEN** 用户从历史列表打开 legacy 版块结果
- **THEN** 前端使用兼容视图正常渲染并标注“旧版分析”，不要求它符合新简报/调查 schema
