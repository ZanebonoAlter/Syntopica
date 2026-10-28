# tagging-domain 变更：示例词泄漏护栏

## ADDED Requirements

### Requirement: 示例词泄漏护栏

两条 LLM 提取路径（mono 融合提取、aggregate 栏目片提取）SHALL 在响应解析出口过滤「prompt 示例词泄漏」候选：候选标签 label 经 Slugify 归一后命中该路径 system prompt 的示例词集合，且模型可见输入（mono：Title+摘要采样文本；section：Title+栏目标题+栏目片正文）中不包含该 label（大小写不敏感）时，该候选 MUST 在持久化前丢弃。候选主标签的 auxiliary_labels 项 SHALL 按同一规则独立过滤（主标签被丢弃时其全部 auxiliary_labels 一并丢弃）。示例词集合 SHALL 以代码常量与 prompt 文本同源维护，并 SHALL 有防漂移测试断言集合中每个词原文出现在对应 prompt 文本中。被护栏丢弃的候选 SHALL 记入观测通道（mono 记入 ExtractionResult 的 errors 尾注，section 记入 warn 日志），不静默丢弃。heuristic 兜底与跨 feed 标签复用路径不经 LLM 解析出口，不受本护栏影响。

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
