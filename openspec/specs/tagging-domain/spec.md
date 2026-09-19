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

系统 SHALL 将标签相关代码组织为 `internal/domain/tagging/` 域，包含以下子包：

- `tagging/` 根层：标签生命周期编排（tagger.go）、共享类型（types.go）、共享 helper（helpers.go）、queue 入口（workers.go、tag_queue.go）
- `tagging/extraction/`：纯文本标签提取，输入文章文本，输出 `[]TopicTag`
- `tagging/analysis/`：主题分析 CRUD + 分析队列
- `tagging/embedding/`：向量化服务 + embedding/merge 队列
- `tagging/merge/`：标签合并、聚类、清理
- `tagging/watched/`：关注标签管理
- `tagging/semantic/`：辅助标签入库、SemanticBoard 匹配、升级建议、回填

编排层（tagger.go）SHALL 在进入提取前按 `articles.content_form` 分流：`aggregate` 走栏目切片 map-reduce 聚合路径，`mono` 与空值走原有提取路径。聚合路径的切片器为纯代码（无 LLM 调用），其逐片 LLM 提取与跨片去重属于编排层职责；`extraction/` 子包继续只做单次纯文本提取。

#### Scenario: 依赖方向全部单向

- **WHEN** 检查 tagging 域内所有子包的 import 关系
- **THEN** 依赖方向为：`extraction → helpers/embedding`、`merge → helpers/embedding`、`semantic → helpers/embedding`、`watched → helpers`；不存在任何反向依赖或循环依赖

#### Scenario: extraction 子包只做文本提取

- **WHEN** `extraction.ExtractTopics(input)` 被调用
- **THEN** 返回 `[]TopicTag` 原始标签列表，不执行 embedding 匹配、LLM 判断、合并或层级放置

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
