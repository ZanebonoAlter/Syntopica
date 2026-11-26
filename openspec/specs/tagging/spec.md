<!-- domain: semantic-board, ui -->

# tagging Specification

## Purpose

标签是 L0 原子概念：话题、版块、匹配、发现、阅读全部建立在它之上（`backend-go/internal/tagmanagement` 被 reader / topicgraph / discovery / dataenrichment 四包 import）。本族定义标签怎么产生——数据模型、从文章提取标签的规则与上限、聚合打标、标签队列调度与进度通知。

## Requirements

### Requirement: 状态机字段的 GORM default tag 保留

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

TopicTag / TagMergeSuggestion 的 `Status` 字段 SHALL 保留 GORM `default:` tag（`default:active` / `default:pending`）。该 tag 为功能必需而非风格选择：无 `default:` tag 时 GORM 对零值字段显式 INSERT `""`，DB 层 DEFAULT 无机会生效，产生非法状态行破坏 `status='active'` 类严格过滤（getBoardArticles 等）。其余非状态机字段的 tag 剥除治理（a0b03bdc）维持不变。`SemanticLabel.ContextLayers` 的默认值 SHALL 由 `BeforeCreate` 钩子填充（JSON 数组 default tag 在 reflect.StructTag 语法下无法表达，`\"` 转义写法会损坏 GORM tag 解析导致显式写 NULL）。

#### Scenario: 新建 TopicTag 未显式设 status 默认 active

- **WHEN** 以 GORM Create 插入 TopicTag 且不设置 Status 字段
- **THEN** 落库行 status='active'（GORM default tag 生效，DB DEFAULT 兜底），而非空串
- **AND** `getBoardArticles` 的 `status='active'` 过滤可命中该 tag

#### Scenario: 新建 SemanticLabel 未显式设 context_layers 由 DB DEFAULT 填充

- **WHEN** 以 GORM Create 插入 SemanticLabel 且 ContextLayers 为 nil
- **THEN** INSERT 语句不显式写该列，落库值为 DB DEFAULT `["week","month","year","all"]`，不触发 not-null 约束错误

#### Scenario: jsonb default tag 转义正确性

- **WHEN** 检查 `SemanticLabel.ContextLayers` 的 GORM tag 字面量
- **THEN** default 表达式内为原生双引号（`default:'["week","month","year","all"]'`），不含反斜杠转义（Go 反引号字符串中 `\"` 是字面反斜杠，会损坏 GORM 解析导致显式写 NULL）

### Requirement: 单主题打标输入与上限参数

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

mono 路径（含 content_form 为空的存量文章）SHALL 将摘要输入采样预算设为 4000 runes，文章级标签上限设为 6。存量文章（content_form 为空）SHALL 走 mono 路径且行为一致。

采样预算的分配规则：

- 剥离 markdown 噪声后计量：图片语法 `![alt](url)` 整体删除（alt 不保留），行内链接 `[text](url)` 仅保留 text；噪声不得占用采样预算。
- 剥离后长度不超过预算的输入 SHALL 原样进入提取（不做任何采样与截断标记）。
- 超预算输入按 markdown 标题（`#` 至 `######`）切段：
  - 段数 > 3（文集型）：SHALL 保留全部段落的标题；正文预算在段落间均分（每段保底 120 runes），段内截断 SHALL 落在句子边界（回退窗口内无句界时允许硬切），被截断段落 SHALL 带省略标记；段数多至保底分配超出总预算时，超出部分的段落 SHALL 退化为仅保留标题（归入一行标题清单）。
  - 段数 ≤ 3（叙事型）：SHALL 按头/中/尾三段采样（约 2:1:1），采样段间带省略标记。
- 采样结果总长度 SHALL 不超过预算与固定开销之和（固定开销为标题与标记字符，允许容差不超过总预算的 5%）。

#### Scenario: 短输入原样通过

- **WHEN** 一篇 mono 文章剥离噪声后的正文为 1200 runes
- **THEN** 进入提取的输入与剥离噪声后的正文一致，不含任何省略标记

#### Scenario: 单主题长摘要截断

- **WHEN** 一篇 mono 文章的 AIContentSummary 剥离噪声后为 7000 runes 且含 15 个 `##` 标题栏目
- **THEN** 进入提取的输入为采样后的全文代表（全部 15 个栏目标题在场，正文按预算均分且段内句界截断带省略标记），总长不超过 4000 runes 的预算与固定开销容差

#### Scenario: 叙事型长文头中尾采样

- **WHEN** 一篇 9000 runes 的正文经标题切段仅得 2 段
- **THEN** 提取输入由头部约 2000 runes、中部约 1000 runes、尾部约 1000 runes 的采样拼接组成，段间带省略标记

#### Scenario: 段数过多退化为标题清单

- **WHEN** 一篇超预算正文含 30 个标题段落，保底 120 runes/段的分配超出 4000 runes 总预算
- **THEN** 前若干段按保底预算采样，其余段落的标题归入一行标题清单，不保留正文

#### Scenario: 图片与链接噪声不占预算

- **WHEN** 一篇正文含 20 处图片语法（合计约 4000 字符的 URL）且剥离后正文仍超预算
- **THEN** 图片语法在采样计量前已被整体删除，链接仅保留文字部分，预算全部分配给正文内容

#### Scenario: 存量文章走 mono 路径

- **WHEN** 一篇 change 合并前入库、content_form 为空的存量文章被重新打标
- **THEN** 其处理路径与新 mono 文章一致（同一采样规则、标签上限 6）

### Requirement: topic_tags.feed_count 周期对账

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

系统 SHALL 在 TagQualityScoreJob（周期调度）中对 `topic_tags.feed_count` 执行全量对账：将 feed_count 重算为 `COUNT(DISTINCT articles.feed_id)`（经 `article_topic_tags` 关联）。对账失败 SHALL 记录 warning 日志且不中断 job 其余步骤，与既有 auxiliary ref_count 对账的容错模式一致。

#### Scenario: 打标漂移后被对账修正
- **WHEN** 新文章打标使某 tag 实际 distinct feed 引用数为 5，而 `topic_tags.feed_count` 仍为旧值 3
- **THEN** 下一次 TagQualityScoreJob 运行后该 tag 的 `feed_count` 为 5

#### Scenario: 无引用标签对账为零
- **WHEN** 某 tag 的所有 article 关联被清除（如 hard merge 后残留）
- **THEN** 对账后该 tag 的 `feed_count` 为 0

#### Scenario: 对账失败不中断 job
- **WHEN** feed_count 对账 SQL 执行失败
- **THEN** job 记录 warning 日志并继续执行后续步骤（quality score 计算等），job 不返回失败

### Requirement: LLM 空 keyword 结果不触发规则回填

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

mono 路径的融合提取 SHALL 原样接受 LLM 返回的空 `keyword_tags` 数组：不得调用启发式规则匹配（分类名、feed 名、模式词表）生成候选关键词回填结果。空 keyword 数组是提示词「宁缺毋滥」约束下的合法结论。事件/人物数组与 keyword 数组 SHALL 独立评判，一方为空不使另一方失效。

**【2026-11-12 本 change 起生效】系统 SHALL NOT 存在任何规则词（heuristic）回填或兜底路径**：规则词词表（分类名、feed 名、模式词表）已随 `retire-heuristic-tag-fallback` 整体删除。

#### Scenario: 快讯类文章 keyword 为空时保持 LLM 原样

- **WHEN** 一篇华尔街见闻快讯（分类名「新闻」）经融合提取返回 1 个 event 标签且 `keyword_tags` 为空数组
- **THEN** 提取结果仅含该 event 标签，不出现「新闻」或任何分类名/规则词标签
- **AND** 结果的来源标记保持 `llm`

#### Scenario: 空摘要文章两个数组均为空

- **WHEN** 一篇文章的摘要输入为空串或纯空白，LLM 返回 `{"event_person_tags":[],"keyword_tags":[]}`
- **THEN** 提取结果为零候选，不产生任何规则词候选

### Requirement: 泛词标签黑名单拦截

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

所有文章标签写入路径（mono、aggregate、跨 feed 标签复用）SHALL 在持久化前拦截黑名单泛词标签：黑名单至少包含「新闻、论坛、要闻、快讯、文章、内容、技术、发展」，被拦截候选不落库、不进入辅助标签池。黑名单为代码内常量，不要求可配置。

**【2026-11-12 本 change 起生效】写入路径清单已无 `heuristic 兜底` 一项**（该路径随 `retire-heuristic-tag-fallback` 退役删除）；黑名单本身保留——LLM 侧仍可能产出泛词候选。

#### Scenario: 分类名回填词被拦截

- **WHEN** 任一提取路径产出 label 为「新闻」的文章标签候选
- **THEN** 该候选在持久化前被丢弃，`article_topic_tags` 不产生新行

#### Scenario: 跨 feed 复用不复制泛词标签

- **WHEN** 一篇文章的 sibling（同 link）文章带有 label 为「新闻」的标签关联，跨 feed 复用逻辑为其复制 sibling 标签
- **THEN**「新闻」关联行不被复制到该文章，其余合法标签照常复制

#### Scenario: 黑名单不误伤正常标签

- **WHEN** 一篇 V2EX 编程讨论文章被提取出「GLM Coding Plan」等非泛词标签
- **THEN** 黑名单不拦截这些标签（黑名单仅含完整匹配的泛词，不做子串匹配）

### Requirement: Event 标签描述的时间锚纪律

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

event 类标签的 LLM 描述生成 SHALL 遵守时间锚纪律：描述中的事件发生时间 MUST 以所给文章上下文（含文章发布日期）为参照，MUST NOT 引入文章上下文中不存在的时间信息（包括模型先验知识中的历史事件时间）。标签抽取与描述生成环节的 prompt 示例 MUST 使用时间中性或虚构的事件示例，MUST NOT 使用自带具体历史时间锚的真实事件作为示例。

#### Scenario: 新建 event 标签时描述时间锚与文章日期一致

- **WHEN** 一篇发布于 D 日的文章触发新建 event 标签，且文章内容未提及其它时间信息
- **THEN** 该标签 description 中的事件时间表述与 D 日（或文章明确给出的相对时间）一致，不出现与文章无关的历史年份

#### Scenario: 描述生成不引入先验知识时间

- **WHEN** 标签名称与某个著名历史事件同名（如真实世界曾发生过的袭击/发布/政策事件），而触发文章讲的是当日语境的其它内容
- **THEN** 生成的 description 以触发文章上下文为准，MUST NOT 复述该历史事件的发生时间

#### Scenario: prompt 示例不含真实历史时间锚

- **WHEN** 检查标签抽取 prompt 与 event 描述生成 prompt 的示例文本
- **THEN** 示例中使用时间中性或虚构的事件示例，不出现“真实事件名 + 具体历史年月”的组合

### Requirement: 示例词泄漏护栏

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

两条 LLM 提取路径（mono 融合提取、aggregate 栏目片提取）SHALL 在响应解析出口过滤「prompt 示例词泄漏」候选：候选标签 label 经 Slugify 归一后命中该路径 system prompt 的示例词集合，且模型可见输入（mono：Title+摘要采样文本；section：Title+栏目标题+栏目片正文）中不包含该 label（大小写不敏感）时，该候选 MUST 在持久化前丢弃。候选主标签的 auxiliary_labels 项 SHALL 按同一规则独立过滤（主标签被丢弃时其全部 auxiliary_labels 一并丢弃）。示例词集合 SHALL 以代码常量与 prompt 文本同源维护，并 SHALL 有防漂移测试断言集合中每个词原文出现在对应 prompt 文本中。被护栏丢弃的候选 SHALL 记入观测通道（mono 记入 ExtractionResult 的 errors 尾注，section 记入 warn 日志），不静默丢弃。跨 feed 标签复用路径不经 LLM 解析出口，不受本护栏影响（heuristic 兜底路径已于 2026-11-12 随 retire-heuristic-tag-fallback 退役，不再存在）。

#### Scenario: 无提及文章的示例词 keyword 被丢弃

- **WHEN** 一篇文章的可见输入（Title+Summary）中不含 postgres 字样，LLM 融合提取返回 `keyword_tags` 含 "PostgreSQL"
- **THEN** 该候选在持久化前被丢弃，文章不因该候选产生 `article_topic_tags` 行或新标签
- **AND** 丢弃被记入 ExtractionResult 的 errors 观测尾注

#### Scenario: 可见输入确有提及的示例词保留

- **WHEN** 文章 Title 为 "PostgreSQL update"、Summary 含 PostgreSQL 版本发布内容，LLM 返回 keyword "PostgreSQL"
- **THEN** 该候选照常进入后续解析与持久化，不被护栏丢弃

#### Scenario: 非示例词候选不受影响

- **WHEN** LLM 返回 "LangChain"（不在示例词集合）keyword，可见输入中亦无该词
- **THEN** 该候选不经护栏拦截，照常进入后续流程

#### Scenario: auxiliary_labels 泄漏项被过滤

- **WHEN** LLM 返回的 event 标签（如"科技周刊推荐开源工具与资源事件"）本身不命中示例词，但其 auxiliary_labels 含可见输入中无提及的 "PostgreSQL" 项
- **THEN** 该 event 标签保留，其 auxiliary_labels 中的 "PostgreSQL" 项被剔除

#### Scenario: section 路径同样生效

- **WHEN** 一篇聚合文章的某栏目片（标题+正文）不含 Kubernetes 字样，该片的 section 提取返回 keyword "Kubernetes"
- **THEN** 该候选在 section 解析出口被丢弃并记 warn 日志，不进入跨片去重与持久化

#### Scenario: 大小写变体同样拦截

- **WHEN** LLM 返回 label 为 "POSTGRESQL"（大小写变体，Slugify 归一后同为 postgresql），可见输入中无任何大小写形式的 PostgreSQL
- **THEN** 该候选命中示例词集合被丢弃

#### Scenario: 示例词集合与 prompt 防漂移

- **WHEN** 修改任一提取 prompt 的示例词而未同步对应代码常量集合（或反之）
- **THEN** 防漂移测试失败（断言集合每个词原文出现在对应 prompt 文本中）

### Requirement: 提取响应形态容错与 prompt 指令一致性

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

打标提取的响应解析 SHALL 容忍模型因 JSON-schema 引导而吐出的 **schema 外壳形态**——即顶层为 `{"type":"object","properties":{...},"required":[...]}`、而数组值被塞在 `properties` 下的响应。解析器 SHALL 识别该外壳并取 `properties` 内的对象继续按既有校验链处理（event/person 与 keyword 双数组、section 路径的单 `tags` 数组各按其既有形态解析）。

外壳判定 SHALL 同时满足三条，缺一不判为外壳（避免误解正常响应）：
1. 顶层是 JSON 对象；
2. 含 `properties` 键且其值是 JSON 对象；
3. 含 `type` 或 `required` 键（schema 特征）。

解包 SHALL 是幂等的：解开一次后顶层不再具备 schema 特征，再解 SHALL 原样返回。

**不放松畸形判定**：外壳解开后若目标键仍缺失（非空对象缺 `event_person_tags` 与 `keyword_tags` 双键，或非数组形态），SHALL 报解析错误触发既有重试，**MUST NOT** 把坏数据当作合法结果或合法零候选。

提取路径的 **user prompt 输出格式指令 SHALL 与下发给模型的 JSON Schema 一致**：merged 提取路径（双数组 `event_person_tags` / `keyword_tags`）的 prompt SHALL 指向双数组形态，MUST NOT 再指向已废弃的单数组 `{"tags": [...]}` 形态；section/aggregate 提取路径（单数组 `tags`）的 prompt 维持单数组指令不变。

#### Scenario: 双数组被包在 schema 外壳内
- **WHEN** 模型响应为 `{"type":"object","properties":{"event_person_tags":[...],"keyword_tags":[...]},"required":[...]}`
- **THEN** 解析器 SHALL 解开外壳并按规范双数组形态处理，返回 event/person 与 keyword 两组标签

#### Scenario: 单数组被包在 schema 外壳内
- **WHEN** section 路径的模型响应为 `{"type":"object","properties":{"tags":[...]},"required":["tags"]}`
- **THEN** 解析器 SHALL 解开外壳并按单数组形态处理

#### Scenario: 外壳内目标键缺失仍判解析错误
- **WHEN** 模型响应为 `{"type":"object","properties":{"other":[1]},"required":["other"]}`
- **THEN** 解包后仍缺目标键，解析 SHALL 返回错误触发重试，MUST NOT 视为合法零候选

#### Scenario: 非外壳响应不被误解
- **WHEN** 模型响应为不含 schema 特征的普通对象或数组
- **THEN** 解析器 SHALL 原样处理，行为与既有形态完全一致

#### Scenario: 提取 prompt 输出指令与 schema 一致
- **WHEN** merged 提取路径构造 user prompt
- **THEN** 末句输出格式指令 SHALL 指向 `event_person_tags` 与 `keyword_tags` 双数组，MUST NOT 出现已废弃的 `{"tags": [标签列表]}` 单数组写法

#### Scenario: section 提取 prompt 指令保持单数组
- **WHEN** section/aggregate 提取路径构造 user prompt
- **THEN** 末句输出格式指令 SHALL 维持 `{"tags": [...]}` 单数组（其 schema 本就是单数组），MUST NOT 被改成双数组

### Requirement: 提取失败不降级，直接判任务失败

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

系统 SHALL NOT 存在任何规则词（heuristic）兜底打标路径。LLM 提取调用失败（含网络/HTTP/超时错误、响应解析失败、重试耗尽）时：

1. 提取层 SHALL 向调用方返回错误，SHALL NOT 产出任何规则词候选；
2. 编排层 SHALL 向上返回错误，文章 SHALL NOT 写入任何 `article_topic_tags` 行、SHALL NOT 产生 `source='heuristic'` 的 `topic_tags`；
3. 打标任务 SHALL 标记为失败，并进入既有队列重试机制（退避重试，超过 `max_attempts` 后停在 `failed` 态，等待人工或批量 retag）——失败 SHALL NOT 被伪装成成功。

提取调用成功但结果为零标签，仍是合法结论：文章保持无标签状态（不写入任何行），任务视为完成，等待后续重打流程。

#### Scenario: 提取失败不产出任何标签

- **WHEN** 融合提取因连接拒绝/超时等返回错误且重试耗尽
- **THEN** 提取 SHALL 返回错误且不产出任何标签候选
- **AND** 文章 SHALL 不写入任何 `article_topic_tags` 行

#### Scenario: 编排层不兜底规则词并把错误上抛

- **WHEN** 编排层拿到提取层的错误
- **THEN** 编排 SHALL 返回错误（不调用任何规则词兜底函数），使打标任务进入失败态
- **AND** 数据库 SHALL 不出现 `source='heuristic'` 的标签行

#### Scenario: 失败任务进入队列重试

- **WHEN** 打标任务因提取失败被标记为失败
- **THEN** 任务 SHALL 以退避策略回到待处理态并被重新领取重试
- **AND** 重试次数达上限后 SHALL 停在 `failed` 态且失败原因可被读取

#### Scenario: 提取成功但零标签仍是合法结论

- **WHEN** 融合提取成功返回且候选总数为零
- **THEN** 文章 SHALL 不写入任何 `article_topic_tags` 行
- **AND** 任务 SHALL 视为完成，SHALL NOT 触发失败重试

### Requirement: 打标提取类别收敛为 event 与 keyword

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->

系统 SHALL 在打标提取 prompt 中只要求提取 `event`（事件）与 `keyword`（关键词），SHALL NOT 要求提取 `person`（人物）；该约束 SHALL 覆盖单篇合并提取与聚合栏目片提取两条路径。

#### Scenario: prompt 不再要求提取人物

- **WHEN** 构建单篇合并提取 prompt 或聚合栏目片提取 prompt
- **THEN** prompt SHALL 包含 event 与 keyword 的提取规则
- **AND** prompt SHALL NOT 包含人物类别的提取要求（如「person（人物）」「每个 event/person 标签」）

#### Scenario: schema 类别描述不招回串放

- **WHEN** 构建单篇合并提取的结构化输出 schema
- **THEN** `event_person_tags[].category` 的描述 SHALL 限定为 event（与 `keyword_tags[].category` 的 keyword 对称）
- **AND** 聚合路径单数组的 `tags[].category` 描述 SHALL 为「event 或 keyword」

#### Scenario: 模型仍返回人物时的兼容

- **WHEN** 模型输出中仍带有 `category: "person"` 的条目
- **THEN** 解析 SHALL 成功（按既有类别校验路径入库），整篇提取 SHALL NOT 因该条目失败
- **AND** 模型 SHALL 继续通过 `event_person_tags` 字段返回 event 条目（字段名保持，维持解析契约）

#### Scenario: 存量人物标签与事件主体前缀不受影响

- **WHEN** 打标 prompt 变更上线后
- **THEN** 既有 `category='person'` 标签 SHALL 保留原状态与引用（不删除、不改状态、不自动归并）
- **AND** 事件指纹解析 SHALL 继续接受以 `person:` 开头的 `subject_key`（该前缀描述「事件主体是人」，取自 event 标签的 `metadata.event_fingerprint`）

### Requirement: 摘要生成产出内容形态标记

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

系统的摘要生成调用（`summarizeContent`）SHALL 在 system prompt 中要求模型将文章判定为单主题（`mono`）或聚合型（`aggregate`），并以摘要第一行 HTML 注释 `<!-- form: mono -->` 或 `<!-- form: aggregate -->` 输出该判定。

系统 SHALL 在持久化 AIContentSummary 前解析并剥离该注释行，将标记值存入 `articles.content_form` 列；剥离后的摘要正文 SHALL NOT 包含任何形态注释残留。

#### Scenario: 聚合型文章被正确标记

- **WHEN** 一篇内容为多栏目合集（如科技周刊：AI 缓存专题 + 科技动态 + 工具推荐 + 言论）的文章完成摘要生成
- **THEN** 该文章 `content_form = 'aggregate'`，且 AIContentSummary 首行不是 HTML 注释

#### Scenario: 单主题文章被正确标记

- **WHEN** 一篇通篇围绕单一主题的文章（如一篇 .NET 教程，即使有 20+ 个章节小标题）完成摘要生成
- **THEN** 该文章 `content_form = 'mono'`

#### Scenario: 模型未输出标记时安全降级

- **WHEN** 模型返回的摘要不含合法的形态注释行
- **THEN** `content_form` 保持为空，摘要原文正常入库（注释解析失败不影响摘要存储）

#### Scenario: 存量文章不回填

- **WHEN** change 合并前已入库的文章被读取
- **THEN** 其 `content_form` 为空，不触发任何回填任务

### Requirement: 摘要按栏目切片

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

系统 SHALL 提供纯代码切片器，输入聚合型文章的 markdown 摘要，输出 4-8 个栏目级文本片：按 `## ` 标题切分，跳过"导读"片（其内容是正文概括，map 后无增量），短栏目（<300 runes）向后并入相邻片；`##` 切完超过 8 片时对超长栏目按 `### ` 细分；仍超过 8 片时从尾部合并相邻片压回 8。切片器 SHALL NOT 发起任何 LLM 调用。

#### Scenario: 标准周刊摘要被切成栏目片

- **WHEN** 输入含"导读 + 正文整理（专题）+ 科技动态 + 工具 + 言论"5 个 `##` 栏目的周刊摘要
- **THEN** 输出 4 片（导读被跳过），每片携带栏目标题上下文与正文

#### Scenario: 短栏目被合并

- **WHEN** 某个 `##` 栏目正文仅 100 runes
- **THEN** 该栏目与其后相邻栏目合并为一片

#### Scenario: 无标题结构时安全回落

- **WHEN** 输入摘要不含任何 `## ` 标题
- **THEN** 聚合路径放弃切片，回落到 mono 路径处理该文章

### Requirement: 逐片融合提取标签

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

聚合路径 SHALL 对每个栏目片发起恰好 1 次 LLM 调用，使用 event/person/keyword 三分类融合的单一 prompt（每片产出上限 4 个标签候选），SHALL NOT 对单篇文章发起超过 8 次提取调用。

每个 `category='event'` 的标签 SHALL 在同一次调用内同时产出结构化指纹 `subject` / `subject_key` / `action` / `object`，`occurred_at` MAY 产出。指纹字段 SHALL NOT 触发额外调用，SHALL NOT 由第二条提取链路补取。`keyword` 与 `person` 标签 SHALL NOT 产出这几项。

`subject_key` SHALL 为归一化实体键，格式 `类型:规范名`（类型限于 `person` / `company` / `org` / `product` / `other`，规范名用小写英文或拼音），由模型在同一次调用内完成别名归并（如 Trump / 川普 / 特朗普 → `person:trump`）。`subject_key` SHALL NOT 依赖 `topic_tags.aliases` 生成——实测 person/event 的 aliases 覆盖率为 0%、keyword 仅 7.3%。

指纹 SHALL 与标签的 `metadata.event_keywords` 做一致性校验：`subject` 与 `object` 均 SHALL 能在关键词集合中找到（允许包含关系与别名展开）。校验不通过或关键词缺失时，标签 SHALL 照常入库并参与既有流程，指纹 SHALL 标记为不可信并记录 warning（含标签名与不一致字段）。

单片调用失败时 SHALL 记录 warning 并跳过该片继续处理其余片，SHALL NOT 因单片失败丢弃整篇或整篇重试。

单个标签的 auxiliary_labels 校验失败或 keyword description 缺失 SHALL 降级处理（丢弃校验不过的部分、保留其余标签），SHALL NOT 因单标签问题让整片候选报废；每次降级 SHALL 记录 warning（含标签名与原因）。JSON 整体解析失败 SHALL 维持原有重试语义（重试 3 次后跳过该片）。

#### Scenario: 每片一次调用产出候选

- **WHEN** 切片器产出 5 个栏目片
- **THEN** 发起 5 次融合 prompt 提取调用，每次调用的输入仅含对应片文本

#### Scenario: 同一次调用产出事件指纹

- **WHEN** 某片返回 3 个 event 标签与 1 个 keyword 标签
- **THEN** 3 个 event 标签 SHALL 各自携带 subject/subject_key/action/object，keyword 标签 SHALL NOT 携带；该片调用次数 SHALL 仍为 1

#### Scenario: 跨语言别名归一为同一主体键

- **WHEN** 两次提取分别返回 subject=`特朗普` 与 subject=`Trump`
- **THEN** 两次的 `subject_key` SHALL 相同（如 `person:trump`），SHALL NOT 因原文字面不同而产出不同键

#### Scenario: 指纹校验失败不阻断

- **WHEN** 某 event 标签的 subject 不在 `event_keywords` 中且无法通过别名匹配
- **THEN** 该标签 SHALL 照常入库，指纹标记不可信并记录 warning，同片其余标签不受影响

#### Scenario: 关键词缺失时指纹不可信

- **WHEN** 某 event 标签的 `metadata.event_keywords` 为空或缺失
- **THEN** 该标签 SHALL 入库，指纹 SHALL 标记不可信，SHALL NOT 因缺少校验依据丢弃标签

#### Scenario: 单片失败不影响其余片

- **WHEN** 5 片中第 3 片的 LLM 调用重试耗尽后仍失败
- **THEN** 第 1、2、4、5 片的标签候选正常进入 reduce 阶段，失败信息记录在日志

#### Scenario: 单标签 aux 校验失败降级保留

- **WHEN** 某片返回 3 个标签，其中 1 个 event 标签的 auxiliary_labels 数量为 2（不满足 3-5 条）
- **THEN** 该 event 标签仍入库（无 aux 锚点），同片其余 2 个标签不受影响，降级记录在 warning 日志

#### Scenario: keyword 缺 description 单标签跳过

- **WHEN** 某片返回的 keyword 标签 description 为空
- **THEN** 该 keyword 标签被跳过并记录 warning，同片其余标签正常进入 reduce 阶段

### Requirement: 事件主体稳定身份说明随同一次提取产出

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

事件标签的提取 SHALL 在产出该标签的**同一次模型调用**内，额外产出该标签主体的原文提及（`subject`）与**稳定短身份说明**（`subject_desc`）；SHALL NOT 为该说明发起额外模型调用。该字段 SHALL **仅在影子模式下请求**：关闭态下第一轮的提示与 schema SHALL 与上线前逐字一致，SHALL NOT 新增字段输出或模型请求。

- 稳定短身份说明 SHALL 只描述主体自身的身份（类别 / 国别 / 角色 / 所属组织等），SHALL NOT 包含当次事件、动作、对象或时间；第一轮提示 SHALL 显式声明该约束。
- 稳定短身份说明 SHALL NOT 写入事件指纹 `event_fingerprint`，SHALL NOT 改变既有指纹完整性判定与关键词一致性校验结论；SHALL 落在独立 metadata 键。
- `keyword` 与 `person` 类别的标签 SHALL NOT 产出稳定短身份说明。
- 稳定短身份说明为空时 SHALL 照常产出标签并照常入库；SHALL NOT 因缺该说明降级、报错或丢弃标签。

#### Scenario: 影子态同一次调用产出主体原文与稳定短身份说明

- **WHEN** 影子模式下某栏目片返回 3 个 event 标签
- **THEN** 3 个标签 SHALL 各自带上主体原文与稳定短身份说明，该片的模型调用次数 SHALL 仍为 1
- **AND** `event_fingerprint` SHALL 与未产出该说明时结构相同，指纹完整性与关键词一致性校验结论 SHALL 不变
- **AND** keyword 与 person 类别的标签 SHALL NOT 携带该说明
- **AND** 某 event 标签的稳定短身份说明为空时，该标签 SHALL 照常入库，SHALL NOT 因缺该说明报错或丢弃

#### Scenario: 关闭态第一轮提示与字段输出不变

- **WHEN** 系统以关闭态（默认）运行提取
- **THEN** 第一轮提示与 schema SHALL 与上线前逐字一致
- **AND** SHALL NOT 因本能力新增字段输出或模型请求

### Requirement: 跨片去重与文章级上限

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

聚合路径 SHALL 以纯代码方式跨片去重（按 `Slugify(label)`，重复时保留首栏目出现者）并将文章级标签上限设为 15。语义级撞车（不同措辞指同一话题）SHALL 交由既有标签合并建议机制处理，SHALL NOT 在本次提取链路中新增 LLM 仲裁调用。

聚合路径全部片处理完成后标签数为 0（全片失败或全部空产出）时 SHALL 回落 mono 提取路径（单次调用 LLM 提取），SHALL NOT 让聚合型文章以 0 标签结束打标。【2026-11-12 本 change 起生效】回落链路 SHALL NOT 再含 heuristic（规则词）兜底——规则词兜底打标整体退役（`extractor_heuristic.go` 整文件删除、`extractWithHeuristic` 与 `legacyExtractTopics` 两处兜底分支删除）；mono 提取失败时 SHALL 返回错误、文章不写任何标签，打标任务标记失败并进入既有队列退避重试。「不为 0 标签收尾」的口径随之收窄为「不因聚合切片本身的问题而放弃 mono 回落」。

#### Scenario: 同名标签跨片去重

- **WHEN** "工具推荐"片与"正文整理"片产出了同名标签 "Kubernetes"
- **THEN** 该标签只保留一份，归属首栏目出现的片（score 更高）

#### Scenario: 文章级上限截断

- **WHEN** reduce 后候选标签共 17 个
- **THEN** 按片顺序与片内优先级保留前 15 个入库

#### Scenario: 全片失败回落 mono 路径

- **WHEN** 某聚合文章的所有栏目片提取均失败（或全部返回空候选）
- **THEN** 该文章走 mono 提取路径打标（单次调用 LLM 提取，不含 heuristic 兜底），回落原因记录在日志；mono 提取成功则标签数不为 0，mono 也失败则该文章判任务失败、不写任何标签（不产出规则词泛词）

### Requirement: 聚合路径 score 按栏目位置分层

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

聚合路径产出的标签 score SHALL 取所在片的位置层级：首个正文栏目 0.9、中间栏目 0.7、尾部栏目 0.5。mono 路径产出的标签 score 维持一律 0.7 不变。

#### Scenario: 主专题标签获得高 score

- **WHEN** 周刊首个正文栏目（本期主打专题）的标签入库
- **THEN** 其 score 为 0.9

#### Scenario: 尾部栏目标签低 score

- **WHEN** "言论/图片"类尾部栏目的标签入库
- **THEN** 其 score 为 0.5

### Requirement: 聚合标签复用既有入库链路

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

聚合路径产出的每个标签 SHALL 复用既有的 `findOrCreateTag`、auxiliary labels 挂载、`createArticleTopicTagLink` 事务链接与 event 标签 embedding 入队链路，SHALL NOT 另建入库通道。

#### Scenario: event 标签照常进入 embedding 队列

- **WHEN** 聚合路径产出 6 个 event 标签入库
- **THEN** 6 个标签均被 enqueue embedding，与 mono 路径行为一致

#### Scenario: 标签命中已有话题

- **WHEN** 聚合片产出的标签 slug 与库中既有 topic_tag 一致
- **THEN** 复用既有标签实体并累加引用，不新建重复标签

### Requirement: 融合提取 JSON 尾逗号容错

<!-- from: aggregate-tagging (2026-08-22 aggregate-article-tagging) -->

融合与 mono 提取共用的 JSON 标签解析入口（`parseRawTagObjects`）SHALL 在反序列化前对输入做无损修复：剥除对象与数组末元素后的多余逗号；修复 SHALL NOT 影响合法 JSON 的解析结果，SHALL NOT 修改字符串字面量内部内容。

#### Scenario: 尾逗号 JSON 被修复解析

- **WHEN** 模型返回的 tags JSON 含 `"label": "X",}` 形式的尾逗号
- **THEN** 修复后解析成功，标签 "X" 正常产出，不触发重试

#### Scenario: 合法 JSON 不受影响

- **WHEN** 模型返回标准合法 JSON
- **THEN** 解析行为与修复前完全一致

#### Scenario: 字符串内逗号结构不误伤

- **WHEN** 标签 description 字符串字面量内容含 `,}` 字符序列
- **THEN** 该字符串内容保持原样，不被修复函数改动

### Requirement: semantic_labels 统一数据模型

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 `semantic_labels` 表统一存储辅助标签、SemanticBoard 和组合标签。表 SHALL 包含以下字段：id, label, slug, embedding (vector, storage embedding), merge_embedding (vector, label-only merge embedding), label_type ("auxiliary"|"board"|"composite"), aliases (jsonb), ref_count, description, display_order, source, status, protected, created_at, updated_at。SemanticBoard SHALL 是全局共享的长期语义板块，不按 tag category 或 feed category 分表。组合标签的行为契约（embedding 生成、去重 canonical 化、治理操作）见 `composite-label` capability。

#### Scenario: 辅助标签写入
- **WHEN** 新辅助标签 "量子计算" 入库（L3 新建）
- **THEN** 创建 semantic_labels 记录，label_type="auxiliary", source="llm_extract", status="active"，merge_embedding 由 label 生成，embedding 由 label + description 生成

#### Scenario: 板块创建
- **WHEN** LLM 从辅助标签簇中生成新板块 "AI与机器学习"
- **THEN** 创建 semantic_labels 记录，label_type="board", source="llm_suggest", status="active", description 为 LLM 生成的描述

#### Scenario: 组合标签写入
- **WHEN** compose 建议确认创建组合标签 "美债收益率"
- **THEN** 创建 semantic_labels 记录，label_type="composite", source="upgrade_suggest", status="active"，embedding 由 LLM 对组合短语生成，merge_embedding 不使用

#### Scenario: SemanticBoard 全局共享
- **WHEN** 不同 feed category 下的 tag 都匹配到 SemanticBoard "AI与机器学习"
- **THEN** 系统 SHALL 复用同一条 label_type="board" 的 semantic_labels 记录

### Requirement: topic_tag_semantic_labels 关联表

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 `topic_tag_semantic_labels` 关联表记录 tag 和辅助标签的多对多关系。表 SHALL 包含 topic_tag_id 和 semantic_label_id 字段，semantic_label_id SHALL 指向 label_type="auxiliary" 的 semantic_labels。

#### Scenario: Tag 关联辅助标签
- **WHEN** tag "happyhorse" 的辅助标签 "AI" 入库后关联到 semantic_label #42
- **THEN** 创建关联记录 (topic_tag_id=tag.id, semantic_label_id=42)

### Requirement: board_composition 构成关系

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 `board_composition` 关联表记录每个 board 由哪些辅助标签组成。表 SHALL 包含 board_id 和 auxiliary_label_id 字段。

#### Scenario: Board 从辅助标签簇生成
- **WHEN** LLM 从簇 [AI, 大语言模型, GPT] 生成新 board "AI与机器学习"
- **THEN** 为该 board 创建 3 条 board_composition 记录，分别指向这 3 个辅助标签

### Requirement: topic_tag_board_labels 持久化 Tag-Board 归属

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 `topic_tag_board_labels` 关联表记录 tag 和 SemanticBoard 的多对多匹配结果。表 SHALL 包含 topic_tag_id, semantic_board_id, score, match_reason, created_at, updated_at 字段，semantic_board_id SHALL 指向 label_type="board" 的 semantic_labels。

#### Scenario: Tag 关联多个 SemanticBoard
- **WHEN** tag "霍尔木兹海峡" 同时匹配 SemanticBoard "地缘政治" 和 "能源安全"
- **THEN** 系统 SHALL 创建两条 topic_tag_board_labels 记录，分别记录匹配分和匹配原因

#### Scenario: 回填重算覆盖旧归属
- **WHEN** 回填任务重算 tag #10 的 board 归属
- **THEN** 系统 SHALL 用新匹配结果替换 tag #10 在 topic_tag_board_labels 中的旧归属

### Requirement: ref_count 自动维护

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 在 tag 关联或取消关联辅助标签时，自动增减对应辅助标签 semantic_label 的 ref_count。系统 SHALL 在 topic_tag 被删除（含 CleanupOrphanedTags 和 HardMergeTags）时，重新计算受影响辅助标签的 ref_count，确保 ref_count 始终反映实际的 topic_tag_semantic_labels 关联数量。SemanticBoard 的 tag_count SHALL 从 topic_tag_board_labels 聚合计算，不复用 ref_count。

#### Scenario: 新 tag 关联辅助标签
- **WHEN** tag 关联到 semantic_label "AI"（当前 ref_count=10）
- **THEN** ref_count 更新为 11

#### Scenario: topic_tag 被删除后 ref_count 重算
- **WHEN** orphan topic_tag #42（关联 aux label #7 和 #8）被 CleanupOrphanedTags 硬删除
- **THEN** aux label #7 和 #8 的 ref_count SHALL 重新计算为 topic_tag_semantic_labels 中实际剩余的行数

#### Scenario: HardMerge source tag 删除后 ref_count 重算
- **WHEN** HardMergeTags 删除 source tag（关联 aux label #3），将其文章迁移到 target tag
- **THEN** aux label #3 的 ref_count SHALL 重新计算为当前 topic_tag_semantic_labels 中实际关联数

#### Scenario: 存量 ref_count 校准
- **WHEN** 执行一次性存量校准脚本
- **THEN** 所有 label_type='auxiliary' 的 semantic_labels.ref_count SHALL 更新为 topic_tag_semantic_labels 中对应的实际计数

### Requirement: 辅助标签双 embedding 字段用途隔离

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 merge_embedding 执行辅助标签 L2 merge 判断，使用 embedding 执行 SemanticBoard 推荐、Tag-Board 匹配、升级聚类和回填。系统 SHALL NOT 使用 storage embedding 做 L2 merge 判断，也 SHALL NOT 使用 merge_embedding 做 board 匹配。

#### Scenario: L2 merge 使用 merge_embedding
- **WHEN** 新辅助标签 "AI绘图" 与已有辅助标签比较是否 merge
- **THEN** 系统 SHALL 使用双方的 merge_embedding 计算 cosine similarity

#### Scenario: Board 匹配使用 storage embedding
- **WHEN** 系统计算 tag 辅助标签与 SemanticBoard composition 的间接匹配
- **THEN** 系统 SHALL 使用 semantic_labels.embedding（storage embedding）计算相似度

### Requirement: disabled 标签不保留向量

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

`semantic_labels` 中 status='disabled' 的行 MUST 将 embedding 与 merge_embedding 置为 NULL（行本体与 aliases 保留）；重新启用时由 llm_extract 重算向量。禁用流程的代码路径 MUST 同步置 NULL，防止新增 disabled 行继续积累向量。

#### Scenario: 禁用标签时向量置 NULL
- **WHEN** 通过 API 或流程将一个 semantic_label 置为 disabled
- **THEN** 该行的 embedding 与 merge_embedding 被置 NULL，向量存储立即释放

#### Scenario: 存量 disabled 行向量清理
- **WHEN** 执行一次性存量清理（分批 UPDATE）
- **THEN** 所有 status='disabled' 且向量非 NULL 的行向量被置 NULL，行数据与 aliases 不受影响

#### Scenario: 重新启用标签
- **WHEN** 一个 disabled 标签被重新启用
- **THEN** 其向量通过 llm_extract 流程重新生成，语义功能恢复

### Requirement: composite_components 组件引用表

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->

系统 SHALL 使用 `composite_components` 关联表存储组合标签的有序组件引用，表 SHALL 包含 composite_id（指向 label_type="composite" 的 semantic_labels）、component_label_id（指向 label_type="auxiliary" 的 semantic_labels）、position（组件顺序）。组件的删除级联与约束细节由 composite-label capability 定义。

#### Scenario: 组合标签组件写入
- **WHEN** 组合标签 "美债收益率" 创建（组件：美国国债、收益率）
- **THEN** 创建 2 条 composite_components 记录，position 分别为 1、2

#### Scenario: 组合标签删除时组件级联
- **WHEN** 组合标签行被删除
- **THEN** 其 composite_components 记录 SHALL 级联删除

### Requirement: 组合标签数据模型

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

系统 SHALL 在 `semantic_labels` 表中以 `label_type="composite"` 存储组合标签，并使用 `composite_components` 关联表存储有序组件引用。每条组件记录 SHALL 包含 composite_id、component_label_id（指向 `label_type="auxiliary"` 的 semantic_labels）、position（组件顺序，从 1 开始）。单个组合标签 SHALL 包含 2-5 个组件。组合标签 SHALL 复用 semantic_labels 的通用字段（label、slug、embedding、aliases、ref_count、description、source、status 等），source 取值 "upgrade_suggest" 或 "manual"。

#### Scenario: 升级建议确认创建组合标签
- **WHEN** 用户确认 compose 建议「美债收益率」（组件：美国国债、收益率）
- **THEN** 创建 semantic_labels 记录（label_type="composite", source="upgrade_suggest", status="active"）及 2 条 composite_components 记录（position=1 指向「美国国债」，position=2 指向「收益率」）

#### Scenario: 手动创建组合标签
- **WHEN** 用户在治理面板手动创建组合标签「中国CPI」，选择组件 [中国, CPI]
- **THEN** 创建 semantic_labels 记录（label_type="composite", source="manual", status="active"）及对应组件引用

#### Scenario: 组件数量约束
- **WHEN** 创建组合标签时提供 6 个组件
- **THEN** 系统 SHALL 拒绝创建并返回明确错误

#### Scenario: 组件必须指向辅助标签
- **WHEN** 创建组合标签时某组件引用的 semantic_label 不是 label_type="auxiliary"
- **THEN** 系统 SHALL 拒绝创建并返回明确错误

### Requirement: 组合标签 embedding 由 LLM 对组合短语生成

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

系统 SHALL 在组合标签创建时，以「组合标签 label + 组件序列化文本」为输入调用 LLM embedder 生成 embedding。系统 SHALL NOT 通过组件向量加权/平均合成组合 embedding——合成结果丢失组合的指向性语义。

#### Scenario: 创建时生成 embedding
- **WHEN** 组合标签「美债收益率」被创建
- **THEN** 系统调用 embedder 对该组合标签的语义短语生成 embedding 并写入 embedding 字段

#### Scenario: embedder 失败则创建失败
- **WHEN** 创建组合标签时 embedder 调用失败
- **THEN** 创建事务回滚，组合标签与组件引用均不落库，错误返回给调用方

### Requirement: 组合标签去重 canonical 化

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

系统 SHALL 对新建组合标签执行两级去重：L1——组件引用先归一到各组件 aux label 自身的 canonical ID，比较 canonical 组件 ID 集合（无序集合相等）是否与既有组合相同，相同则复用既有组合（ref_count++）；L2——组件集合不同但组合 embedding cosine ≥ composite_dedupe_sim（默认 0.95，ai_settings 可配）时，将新组合名作为 alias 追加到既有组合（ref_count++），不新建行。L1/L2 均未命中才新建。系统 SHALL NOT 对组合标签使用 merge_embedding 做去重。

#### Scenario: L1 canonical 集合相等复用
- **WHEN** 创建组合标签「美债收益率」（组件 canonical 集合 {美国国债, 收益率}）时，已存在组合「美国国债收益率」组件 canonical 集合同为 {美国国债, 收益率}
- **THEN** 系统 SHALL NOT 新建，复用既有组合并 ref_count++

#### Scenario: L2 embedding 相似追加 alias
- **WHEN** 新组合「美债利率」组件集合 {美国国债, 利率} 与既有组合 {美国国债, 收益率} 集合不同，但组合 embedding cosine=0.96 ≥ 0.95
- **THEN** 系统 SHALL NOT 新建，将「美债利率」追加到既有组合的 aliases 并 ref_count++

#### Scenario: 均未命中新建
- **WHEN** 新组合的 canonical 组件集合与所有既有组合不同，且组合 embedding 与所有既有组合 cosine < 0.95
- **THEN** 系统新建 label_type="composite" 的 semantic_labels 记录

### Requirement: 组合标签挂载复用关联表

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

系统 SHALL 允许 topic_tag 通过 `topic_tag_semantic_labels` 关联组合标签，允许 SemanticBoard 通过 `board_composition` 挂载组合标签，关联表结构不变。组合标签的 ref_count SHALL 由挂载关系维护。

#### Scenario: tag 挂载组合标签
- **WHEN** tag「美债收益率创两周新高」关联组合标签「美债收益率」
- **THEN** 创建 topic_tag_semantic_labels 记录，组合标签 ref_count++

#### Scenario: 版块 composition 挂载组合标签
- **WHEN** 用户将组合标签「美债收益率」加入版块「美债观察」的构成
- **THEN** 创建 board_composition 记录指向该组合标签

### Requirement: 组合标签禁用即弃向量

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

组合标签的禁用 SHALL 继承「禁用即弃向量」红线：任何将组合标签 status 置为 disabled 的路径 MUST 同事务将 embedding 置 NULL（行本体、组件引用与 aliases 保留）；重新启用时由系统重算 embedding。

#### Scenario: 禁用组合标签
- **WHEN** 用户禁用组合标签「美债收益率」
- **THEN** 该行 embedding 置 NULL，composite_components 引用与 aliases 保留，后续匹配不再使用该组合

### Requirement: 组合标签治理操作

<!-- from: composite-label (2026-09-04 add-composite-labels) -->

系统 SHALL 提供组合标签的列表查询（含组件展示、ref_count、状态过滤）、手动创建（校验组件类型与数量、去重）、禁用/启用 API。列表 SHALL 展示每个组合的组件标签序列。

#### Scenario: 列表展示组件
- **WHEN** 用户查看组合标签列表
- **THEN** 每个组合标签条目 SHALL 展示 label、组件序列（按 position）、ref_count、status

#### Scenario: 手动创建触发去重
- **WHEN** 用户手动创建的组合标签命中 L1/L2 去重
- **THEN** 系统 SHALL 返回复用/合并结果而非报错，并提示既有组合标签

#### Scenario: 组件候选推荐排序
- **WHEN** 用户打开手动创建对话框加载组件候选
- **THEN** 系统 SHALL 按推荐度返回 active 辅助标签候选：被 active 版块挂载者优先（挂载数多者在前），其后按 ref_count，同分按 label；每个候选 SHALL 携带挂载版块名列表，默认列表有限长度，搜索时 SHALL 支持全量模糊检索

#### Scenario: 版块上下文创建组合
- **WHEN** 用户从版块详情的「组合标签」区发起创建（board_id 上下文）
- **THEN** 组件候选 SHALL 将该版块已挂载的辅助标签置顶并标记「本版块」；创建成功后系统 SHALL 自动将组合标签挂载到该版块；版块详情 SHALL 展示已挂载组合（名称 + 组件链）并支持移除

#### Scenario: 已选组件共现联动重排
- **WHEN** 用户在创建对话框选中一个组件
- **THEN** 组件候选 SHALL 按与该组件的同 tag 共现频次实时重排（共现高者上浮并标记共现次数），引导用户发现可组合的搭档组件

#### Scenario: 已选组件提示现有组合
- **WHEN** 用户已选至少一个组件且存在含该组件的现有组合标签
- **THEN** 前端 SHALL 优先展示相关现有组合（名称 + 组件链）供参考；选中集合与某组合组件集完全一致时 SHALL 提示创建将复用该组合

### Requirement: Article detail API returns tag watch status

<!-- from: article-tag-watch-toggle (2026-05-10 article-tag-follow-status) -->

The `GET /api/articles/:article_id` endpoint SHALL include `id` and `is_watched` fields in each tag object within the `tags` array of the response.

#### Scenario: Tag with watched status returned
- **WHEN** a user requests an article that has tags, and some of those tags are watched
- **THEN** the response `tags` array SHALL contain tag objects with `id` (integer), `is_watched` (boolean), and `article_count` (integer) fields

#### Scenario: Tag without watched status returned
- **WHEN** a user requests an article that has tags, and none of those tags are watched
- **THEN** the response `tags` array SHALL contain tag objects with `is_watched: false`

### Requirement: Article tag list supports watch toggle UI

<!-- from: article-tag-watch-toggle (2026-05-10 article-tag-follow-status) -->

The `ArticleTagList` component SHALL support an optional watch toggle mode. When `showWatch` prop is `true`, each tag SHALL display a heart icon indicating its watched status, and clicking the icon SHALL emit a `watchToggle` event.

#### Scenario: Show watch icons when enabled
- **WHEN** `ArticleTagList` receives `showWatch: true` and a `tags` array with `isWatched` fields
- **THEN** each tag pill SHALL render a clickable heart icon: filled heart (`mdi:heart`) when `isWatched` is true, outlined heart (`mdi:heart-outline`) when false

#### Scenario: No watch icons when disabled
- **WHEN** `ArticleTagList` receives `showWatch: false` (default)
- **THEN** no heart icons SHALL be rendered on any tag

#### Scenario: Click heart icon to toggle watch
- **WHEN** a user clicks the heart icon on a tag
- **THEN** the component SHALL emit a `watchToggle` event with the tag's `id` and `slug` as payload

### Requirement: Article detail page handles watch/unwatch actions

<!-- from: article-tag-watch-toggle (2026-05-10 article-tag-follow-status) -->

The `ArticleContentView` component SHALL handle `watchToggle` events from `ArticleTagList` by calling the watch/unwatch API and updating the article's local tag data optimistically.

#### Scenario: Watch a previously unwatched tag
- **WHEN** user clicks an outlined heart on a tag
- **THEN** the UI SHALL immediately show a filled heart, and a `POST /api/topic-tags/:tag_id/watch` request SHALL be sent

#### Scenario: Unwatch a previously watched tag
- **WHEN** user clicks a filled heart on a tag
- **THEN** the UI SHALL immediately show an outlined heart, and a `POST /api/topic-tags/:tag_id/unwatch` request SHALL be sent

#### Scenario: API failure rolls back optimistic update
- **WHEN** the watch/unwatch API request fails
- **THEN** the heart icon SHALL revert to its previous state

#### Scenario: Tag without ID does not show watch icon
- **WHEN** a tag object lacks `id` field
- **THEN** no watch heart icon SHALL be displayed for that tag

### Requirement: 新任务优先消费

<!-- from: tag-queue-scheduling (2026-09-24 night-window-alignment) -->

TagQueue 的 lease 顺序 SHALL 以新任务优先为准：priority 相同时按入队时间新→旧消费；priority 更高的任务 SHALL 优先于更晚入队的低优任务（插队语义保留）。重试退避语义不变：available_at 在未来的任务 SHALL NOT 被 lease。

#### Scenario: 新旧任务并存时新的先消费
- **GIVEN** 队列中存在昨日入队的 pending 任务 A 与今日入队的 pending 任务 B，priority 相同
- **WHEN** TagQueue lease 一批任务
- **THEN** B 先于 A 被 lease 处理

#### Scenario: 高优先级旧任务可插队
- **GIVEN** 旧任务 A priority=10，新任务 B priority=0
- **WHEN** TagQueue lease
- **THEN** A 先于 B 被 lease（priority 高者优先）

#### Scenario: 退避中的任务不被 lease
- **GIVEN** 任务 C 因失败进入退避，available_at 为未来时刻
- **WHEN** TagQueue lease（即使 C 是最新任务）
- **THEN** C 不被 lease，轮到下一顺位任务

### Requirement: 打标作业入队数据库级幂等

<!-- from: tag-queue-scheduling (2026-09-24 night-window-alignment) -->

同一篇文章 SHALL 至多存在一条未完成（pending/leased）打标作业，该唯一性 SHALL 由数据库部分唯一索引兜底，不依赖应用层 check-then-insert 的时序。并发入队撞索引时 SHALL 按复用处理（原地更新既有作业），SHALL NOT 报错中断调用方。

#### Scenario: 并发入队不产生重复作业

- **WHEN** 两个并发调用同时为同一篇文章入队打标作业（如正文抓取完成回调与刷新链同时到达）
- **THEN** 数据库唯一性保证只成功插入一条，另一条按已存在复用，最终未完成作业至多一条

#### Scenario: 存量重复行建索引前先清理

- **WHEN** 迁移执行时 tag_jobs 已存在同文章多条未完成作业
- **THEN** 迁移 SHALL 先清理重复（保留最小 id 行）再创建唯一索引，迁移幂等可重跑

#### Scenario: 重试失败作业不得与已有活动作业冲突

- **WHEN** 运维触发失败作业批量重试（`POST /api/tag-queue/retry`），其中某篇文章已有 pending/leased 作业（失败后再入队的常态）
- **THEN** 该文章的 failed 行 SHALL 保持 failed 不参与本次重试，活动作业保持至多一条
- **AND** 请求 SHALL 成功返回，SHALL NOT 因唯一索引冲突回滚成 500
- **AND** 同文章堆积多条 failed 行时 SHALL 至多重试一条（最新一条），不得把同文章的失败行一起翻成 pending

#### Scenario: 冲突行转终态时不得报错中断调用方

- **WHEN** 入队撞唯一索引后，冲突行在「插入零行」与「重查」之间转入终态（completed/failed），两次插入机会都没能接住
- **THEN** SHALL 记一条告警并按「已存在」语义返回成功，SHALL NOT 向调用方返回错误

### Requirement: 进度芯片常驻展示

<!-- from: tag-queue-progress-chip (2026-09-17 add-notification-center) -->

系统 SHALL 在全局 header（铃铛左侧）展示标签队列进度芯片：内容为"分析中 n/total"（n=已完成计数，total=本轮总任务数）与迷你进度条，数据来源于 `GET /api/tag-queue/status` 计数与 WS `tag_completed`/`tag_failed` 事件驱动刷新。

#### Scenario: 批量打标进行中展示进度
- **WHEN** 队列存在 pending/leased 任务且用户在任何页面
- **THEN** 芯片可见并随事件实时推进（如"分析中 3/40"）

#### Scenario: WS 事件驱动计数刷新
- **WHEN** 收到 tag_completed 事件
- **THEN** 芯片计数即时更新，无需整页刷新或轮询等待

### Requirement: 空闲自动隐藏

<!-- from: tag-queue-progress-chip (2026-09-17 add-notification-center) -->

队列既无活跃任务（pending+leased==0）也无 failed 任务时芯片 SHALL 不渲染（自动隐藏），不占用 header 空间；仅剩 failed 任务时 SHALL 保持可见（呈失败态，可点去处理）。

#### Scenario: 队列排空后芯片消失
- **WHEN** 最后一个任务 completed 且无新增入队且无 failed 任务
- **THEN** 芯片从 header 消失

#### Scenario: 活跃归零但仍有失败时保持可见
- **WHEN** 队列 pending+leased==0 但存在 failed 任务
- **THEN** 芯片保持可见并呈失败态（可点击跳队列区处理）

#### Scenario: 新任务入队芯片重现
- **WHEN** 队列空闲（芯片隐藏）状态下有新文章入队打标
- **THEN** 芯片重新出现并展示新计数

### Requirement: 失败态变红可点击

<!-- from: tag-queue-progress-chip (2026-09-17 add-notification-center) -->

队列存在 failed 任务时芯片 SHALL 呈失败态（红色边框/底色，文案如"n 个失败"），点击行为与正常态一致。

#### Scenario: 失败态视觉区分
- **WHEN** 队列有 2 个 failed 任务且仍有活跃任务
- **THEN** 芯片呈红色失败态，进度条填充满

### Requirement: 点击跳转设置页队列区

<!-- from: tag-queue-progress-chip (2026-09-17 add-notification-center) -->

点击芯片 SHALL 跳转设置页队列区（复用既有 TagQueuePanel），不新做面板。

#### Scenario: 点击芯片跳转
- **WHEN** 用户点击进行中的芯片
- **THEN** 路由跳转至 /settings 队列区，可见任务列表/重试/重打标操作

### Requirement: 队列计数展示语义

<!-- from: tag-queue-progress-chip (2026-09-17 add-notification-center) -->

队列状态展示（芯片与 TagQueuePanel 状态区）SHALL 以活跃量（pending+leased）为主展示；completed 计数展示"今日完成"语义而非累计总量（配合队列行每日清理，累计口径不再有意义）。

#### Scenario: 面板计数语义
- **WHEN** 昨日有 1000 条 completed 且今日完成 20 条、活跃 3 条
- **THEN** 活跃量显示 3、"今日完成"显示今天日期内 completed 计数，不展示含历史的累计 total

### Requirement: Tagging domain package structure

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 将标签相关代码组织为 `internal/tagmanagement/` 域，包含以下子包：

- `tagging/` 根层：`routes.go`（路由注册）、`wire.go`（依赖装配）
- `tagging/models/`：标签域数据模型
- `tagging/repository/`：仓储与队列持久化
- `tagging/handler/`：HTTP 处理器
- `tagging/service/`：业务服务，含下列子包
  - `service/core/`：标签生命周期编排（`article_tagger.go`、`tag_queue.go`）、标签提取（`extractor_enhanced.go`、`extractor_section.go`）、向量与合并队列、共享 helper
  - `service/board/`：辅助标签入库、SemanticBoard 匹配、升级建议、回填
  - `service/merge/`：标签合并、聚类、清理
  - `service/watched/`：关注标签管理
  - `service/entity/`：实体身份层（影子态）
  - `service/auxlabel/`、`service/auxrepair/`：辅助标签描述生成与修复
  - `service/boardhealth/`、`service/sourcestats/`、`service/related/`：体检、来源构成、相关事件

**【2026-11-12 本 change 起生效】`tagging/extraction/` 子包已不存在**——提取逻辑并入 `service/core/`，规则词兜底函数 `ExtractTopics` 随 `retire-heuristic-tag-fallback` 退役删除（本 requirement 的包结构描述此前仍停留在 `internal/domain/tagging/` 时代的旧布局，本次一并拉回实际结构）。

编排层（`service/core/article_tagger.go`）SHALL 在进入提取前按 `articles.content_form` 分流：`aggregate` 走栏目切片 map-reduce 聚合路径，`mono` 与空值走原有提取路径。聚合路径的切片器为纯代码（无 LLM 调用），其逐片 LLM 提取与跨片去重属于编排层职责；提取逻辑只做单次纯文本提取。

#### Scenario: 依赖方向全部单向

- **WHEN** 检查 tagging 域内的 import 关系
- **THEN** 依赖方向为 `handler → service/<子包> → service/core`、`repository → models`；`service/core` 不反向依赖任何 `service` 子包，不存在反向依赖或循环依赖

#### Scenario: extraction 子包只做文本提取

- **WHEN** 标签提取函数（`service/core/extractor_enhanced.go` 的 `ExtractTags`）被调用
- **THEN** 返回单次 LLM 调用的解析结果，不执行 embedding 匹配、标签合并或层级放置

#### Scenario: 按 content_form 分流

- **WHEN** 一篇 `content_form = 'aggregate'` 的文章进入打标编排
- **THEN** 走聚合路径（切片 → 逐片融合提取 → 跨片去重），`mono` 或空值文章走原有提取路径

### Requirement: Domain package singular naming

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 使用单数包名：`feed/`、`article/`、`category/`、`content/`（取代 `feeds/`、`articles/`、`categories/`、`contentprocessing/`）。

#### Scenario: Import 路径全部更新

- **WHEN** 编译后端
- **THEN** 所有 `internal/domain/feeds` 引用变为 `internal/domain/feed`，以此类推，编译无错误

### Requirement: Models ownership by domain

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 将 `models/` 中的 struct 按域分散：
- `feed/model.go` 持有 Feed、FeedStats
- `article/model.go` 持有 Article
- `category/model.go` 持有 Category
- `content/model.go` 持有 FirecrawlJob
- `preferences/model.go` 持有 UserPreference、ReadingBehavior
- `aiadmin/model.go` 持有 AIProvider、AIRouteProvider、AISettings、AICallLog
- `tagging/types.go` 持有 TopicTag 等核心标签实体
- `tagging/embedding/models.go` 持有 Embedding 相关 model，**其中 TopicTagEmbedding 不包含 Vector 文本字段**
- `tagging/semantic/models.go` 持有 SemanticLabel、BoardComposition 等语义标签 model
- `models/` 仅保留 SchedulerTask

#### Scenario: 改 TopicTag 不触发 feed 重编译

- **WHEN** 修改 `tagging/types.go` 中 TopicTag 的定义
- **THEN** `feed/`、`article/`（除直接引用 TopicTag 的代码外）不重编译

#### Scenario: 改 Feed 不触发 tagging 重编译

- **WHEN** 修改 `feed/model.go` 中 Feed 的定义
- **THEN** `tagging/` 及其子包不重编译

#### Scenario: TopicTagEmbedding 无 Vector 字段

- **WHEN** 检查 `TopicTagEmbedding` struct 定义
- **THEN** 不存在 `Vector` 或 `vector` 相关的 text 类型字段，仅保留 `EmbeddingVec`（pgvector 列）用于向量存储

### Requirement: Unified worker lifecycle

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 通过 `tagging/workers.go` 暴露 `StartAllWorkers()` 和 `StopAllWorkers()` 函数，统一管理 TagQueue、EmbeddingQueue、MergeReembeddingQueue、BackfillQueue 四个 worker 的启动和停止。

#### Scenario: runtime.go 收敛

- **WHEN** `runtime.go` 调用 `tagging.StartAllWorkers()` 和 `tagging.StopAllWorkers()`
- **THEN** 4 个 worker 按正确顺序启动和停止，功能等价于原来手动调用

### Requirement: No behavior change

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

重构 SHALL NOT 改变任何 API 路由、数据库 schema（除移除废弃列外）、业务逻辑或测试断言。所有 `go test ./...` 在重构后 MUST 全部通过。

#### Scenario: 全量测试通过

- **WHEN** 重构完成后运行 `go test ./...`
- **THEN** 所有测试通过，无编译错误

### Requirement: Embedding 写入只使用 pgvector 列

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 在生成 embedding 时仅将向量数据写入 `embedding` (vector(2560)) 列，不再写入冗余的 JSON 文本列。

#### Scenario: GenerateEmbedding 不写入 vector text

- **WHEN** `GenerateEmbedding` 或 `GenerateSemanticEmbedding` 创建新的 `TopicTagEmbedding` 记录
- **THEN** 仅设置 `EmbeddingVec`（pgvector 格式），不写入任何 JSON 文本向量数据

### Requirement: FindSimilarTags 使用原始浮点数组构建查询

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 在 `FindSimilarTags` 中直接使用 `GenerateEmbedding` 返回的原始 `[]float64` 构建 pgvector 查询字符串，不从数据库读取文本格式的向量数据再反序列化。

#### Scenario: 相似标签查询不依赖 vector 文本列

- **WHEN** `FindSimilarTags` 被调用以查找相似标签
- **THEN** 使用 `GenerateEmbedding` 返回的原始浮点数组通过 `floatsToPgVector` 构建 SQL 参数，不执行对 `vector` 列的读取或反序列化

### Requirement: 数据库 vector 列被移除

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 通过数据库 migration 从 `topic_tag_embeddings` 表中移除 `vector` text 列。

#### Scenario: vector 列不存在

- **WHEN** 检查 `topic_tag_embeddings` 表结构
- **THEN** 不存在名为 `vector` 的列

### Requirement: 移除 auxiliary_label_service 中的 SQLite fallback 代码

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 从 `auxiliary_label_service.go` 中移除仅为 SQLite 测试兼容而存在的代码路径。`sqlMergeMatcher` 的 Go 侧余弦相似度计算 SHALL 被评估（per design D5）：如果 pgvector SQL 等价方案性能足够，替换为 SQL 查询；如果 Go 侧计算确为性能优化（非 SQLite fallback），保留但更新注释。

#### Scenario: auxiliary_label_service.go 不包含 SQLite 相关注释

- **WHEN** 检查 `auxiliary_label_service.go` 源码
- **THEN** 不存在 "SQLite" 或 "sqlite" 字样（注释或条件分支中的 SQLite 指向）

#### Scenario: sqlMergeMatcher 使用 pgvector 或有明确的性能优化理由

- **WHEN** 检查 `sqlMergeMatcher` 实现
- **THEN** 要么使用 pgvector SQL 距离操作符进行相似度匹配，要么保留 Go 侧计算但注释说明为 "performance optimization for high-dim vectors" 而非 "SQLite fallback"

### Requirement: 不引入新的 SQLite 依赖路径

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

重构 SHALL NOT 在 tagmanagement 域任何生产代码中引入新的 SQLite 兼容逻辑。所有向量操作统一通过 pgvector 处理。

#### Scenario: 新代码不包含 SQLite 条件分支

- **WHEN** 检查 `internal/tagmanagement/` 下所有非 `_test.go` 文件
- **THEN** 不存在 `db.Name() == "sqlite"` 或类似的数据库类型条件判断

### Requirement: 存量病态时间锚标签的可修复性

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 提供一次性存量修复能力：筛出“事件主体时间锚早于标签创建时间至少一年”的 event 标签候选，经确认后按该标签首挂文章（或指定文章）的上下文重新生成 description。修复 SHALL 支持按标签 ID 精准指定；修复操作 MUST 幂等（重复执行不产生重复副作用），且 MUST NOT 触碰非候选标签的 description。

#### Scenario: 筛选存量病态候选

- **WHEN** 对存量 event 标签执行扫描
- **THEN** 输出“事件主体时间锚早于创建时间 ≥1 年”的标签清单（含标签 ID、label、创建时间、当前 description），合法历史参照（如“创某年以来新高”对比句）不在强制修复范围内

#### Scenario: 指定标签重生成描述

- **WHEN** 对清单中的指定标签 ID 执行修复
- **THEN** 该标签 description 基于其首挂文章的发布日期与摘要上下文重新生成，时间锚与新上下文一致；修复后再次执行扫描，该标签不再出现在候选清单中

#### Scenario: 修复不影响健康标签

- **WHEN** 修复流程执行
- **THEN** 时间锚合法（与创建时间同期，或为合法历史参照）的标签 description 保持原样

### Requirement: 存量示例词泄漏关联的一次性清理

<!-- from: tagging-domain (2026-05-13 backend-package-restructure) -->
<!-- kind: impl -->

系统 SHALL 提供一次性清理工具移除存量误挂关联：筛出 `article_topic_tags` 中 topic_tag label 命中示例词集合、且文章 `title+content+firecrawl_content` 拼接中 0 次出现（大小写不敏感）的关联行，删除后按既有孤儿清理逻辑处理标签。工具 SHALL 默认 dry-run 输出待删清单，显式 `--apply` 才执行删除；SHALL 幂等（重复执行 dry-run 清单为空）；MUST NOT 删除可见文本确有提及的关联，MUST NOT 删除标签本体。

#### Scenario: dry-run 输出误挂清单

- **WHEN** 以默认模式对存量数据执行清理工具
- **THEN** 输出待删关联清单（article_id、tag_id、label、文章标题），不执行任何删除

#### Scenario: apply 后误挂关联移除且真实关联保留

- **WHEN** 以 `--apply` 执行清理（预期约 25 行 PostgreSQL 误挂关联）
- **THEN** 误挂关联行被删除，正文确有提及的关联（约 11 行）保留，PostgreSQL 标签本体保留
- **AND** 再次执行 dry-run 清单为空（幂等）

### Requirement: 删除旧概念和层级字段

<!-- from: semantic-label-model (2026-08-22 analysis-remediation) -->
<!-- kind: impl -->

系统 SHALL 删除旧 `board_concepts` 表、`topic_tags.concept_id` 字段，以及层级体系相关表/字段。系统 SHALL 不提供旧数据自动迁移。

#### Scenario: 用户手动清空旧数据后重建
- **WHEN** 新模型迁移完成且用户已手动删除旧数据
- **THEN** 系统 SHALL 仅依赖 semantic_labels、topic_tag_semantic_labels、topic_tag_board_labels、board_composition 进行板块匹配
