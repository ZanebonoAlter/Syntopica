# Tagging Domain

## Purpose

(TBD)

## Requirements

### Requirement: 状态机字段的 GORM default tag 保留

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

### Requirement: Tagging domain package structure

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

### Requirement: 单主题打标输入与上限参数

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

### Requirement: Domain package singular naming

系统 SHALL 使用单数包名：`feed/`、`article/`、`category/`、`content/`（取代 `feeds/`、`articles/`、`categories/`、`contentprocessing/`）。

#### Scenario: Import 路径全部更新

- **WHEN** 编译后端
- **THEN** 所有 `internal/domain/feeds` 引用变为 `internal/domain/feed`，以此类推，编译无错误

### Requirement: Models ownership by domain

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

系统 SHALL 通过 `tagging/workers.go` 暴露 `StartAllWorkers()` 和 `StopAllWorkers()` 函数，统一管理 TagQueue、EmbeddingQueue、MergeReembeddingQueue、BackfillQueue 四个 worker 的启动和停止。

#### Scenario: runtime.go 收敛

- **WHEN** `runtime.go` 调用 `tagging.StartAllWorkers()` 和 `tagging.StopAllWorkers()`
- **THEN** 4 个 worker 按正确顺序启动和停止，功能等价于原来手动调用

### Requirement: No behavior change

重构 SHALL NOT 改变任何 API 路由、数据库 schema（除移除废弃列外）、业务逻辑或测试断言。所有 `go test ./...` 在重构后 MUST 全部通过。

#### Scenario: 全量测试通过

- **WHEN** 重构完成后运行 `go test ./...`
- **THEN** 所有测试通过，无编译错误

### Requirement: Embedding 写入只使用 pgvector 列

系统 SHALL 在生成 embedding 时仅将向量数据写入 `embedding` (vector(2560)) 列，不再写入冗余的 JSON 文本列。

#### Scenario: GenerateEmbedding 不写入 vector text

- **WHEN** `GenerateEmbedding` 或 `GenerateSemanticEmbedding` 创建新的 `TopicTagEmbedding` 记录
- **THEN** 仅设置 `EmbeddingVec`（pgvector 格式），不写入任何 JSON 文本向量数据

### Requirement: FindSimilarTags 使用原始浮点数组构建查询

系统 SHALL 在 `FindSimilarTags` 中直接使用 `GenerateEmbedding` 返回的原始 `[]float64` 构建 pgvector 查询字符串，不从数据库读取文本格式的向量数据再反序列化。

#### Scenario: 相似标签查询不依赖 vector 文本列

- **WHEN** `FindSimilarTags` 被调用以查找相似标签
- **THEN** 使用 `GenerateEmbedding` 返回的原始浮点数组通过 `floatsToPgVector` 构建 SQL 参数，不执行对 `vector` 列的读取或反序列化

### Requirement: 数据库 vector 列被移除

系统 SHALL 通过数据库 migration 从 `topic_tag_embeddings` 表中移除 `vector` text 列。

#### Scenario: vector 列不存在

- **WHEN** 检查 `topic_tag_embeddings` 表结构
- **THEN** 不存在名为 `vector` 的列

### Requirement: 移除 auxiliary_label_service 中的 SQLite fallback 代码

系统 SHALL 从 `auxiliary_label_service.go` 中移除仅为 SQLite 测试兼容而存在的代码路径。`sqlMergeMatcher` 的 Go 侧余弦相似度计算 SHALL 被评估（per design D5）：如果 pgvector SQL 等价方案性能足够，替换为 SQL 查询；如果 Go 侧计算确为性能优化（非 SQLite fallback），保留但更新注释。

#### Scenario: auxiliary_label_service.go 不包含 SQLite 相关注释

- **WHEN** 检查 `auxiliary_label_service.go` 源码
- **THEN** 不存在 "SQLite" 或 "sqlite" 字样（注释或条件分支中的 SQLite 指向）

#### Scenario: sqlMergeMatcher 使用 pgvector 或有明确的性能优化理由

- **WHEN** 检查 `sqlMergeMatcher` 实现
- **THEN** 要么使用 pgvector SQL 距离操作符进行相似度匹配，要么保留 Go 侧计算但注释说明为 "performance optimization for high-dim vectors" 而非 "SQLite fallback"

### Requirement: 不引入新的 SQLite 依赖路径

重构 SHALL NOT 在 tagmanagement 域任何生产代码中引入新的 SQLite 兼容逻辑。所有向量操作统一通过 pgvector 处理。

#### Scenario: 新代码不包含 SQLite 条件分支

- **WHEN** 检查 `internal/tagmanagement/` 下所有非 `_test.go` 文件
- **THEN** 不存在 `db.Name() == "sqlite"` 或类似的数据库类型条件判断

### Requirement: topic_tags.feed_count 周期对账
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

### Requirement: 存量病态时间锚标签的可修复性

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

### Requirement: 示例词泄漏护栏

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

### Requirement: 存量示例词泄漏关联的一次性清理

系统 SHALL 提供一次性清理工具移除存量误挂关联：筛出 `article_topic_tags` 中 topic_tag label 命中示例词集合、且文章 `title+content+firecrawl_content` 拼接中 0 次出现（大小写不敏感）的关联行，删除后按既有孤儿清理逻辑处理标签。工具 SHALL 默认 dry-run 输出待删清单，显式 `--apply` 才执行删除；SHALL 幂等（重复执行 dry-run 清单为空）；MUST NOT 删除可见文本确有提及的关联，MUST NOT 删除标签本体。

#### Scenario: dry-run 输出误挂清单

- **WHEN** 以默认模式对存量数据执行清理工具
- **THEN** 输出待删关联清单（article_id、tag_id、label、文章标题），不执行任何删除

#### Scenario: apply 后误挂关联移除且真实关联保留

- **WHEN** 以 `--apply` 执行清理（预期约 25 行 PostgreSQL 误挂关联）
- **THEN** 误挂关联行被删除，正文确有提及的关联（约 11 行）保留，PostgreSQL 标签本体保留
- **AND** 再次执行 dry-run 清单为空（幂等）

### Requirement: 提取响应形态容错与 prompt 指令一致性

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
