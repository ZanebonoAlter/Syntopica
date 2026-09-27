# Delta Spec: tagging-domain

## ADDED Requirements

### Requirement: LLM 空 keyword 结果不触发规则回填

mono 路径的融合提取 SHALL 原样接受 LLM 返回的空 `keyword_tags` 数组：不得调用启发式规则匹配（分类名、feed 名、模式词表）生成候选关键词回填结果。空 keyword 数组是提示词「宁缺毋滥」约束下的合法结论。事件/人物数组与 keyword 数组 SHALL 独立评判，一方为空不使另一方失效。

#### Scenario: 快讯类文章 keyword 为空时保持 LLM 原样

- **WHEN** 一篇华尔街见闻快讯（分类名「新闻」）经融合提取返回 1 个 event 标签且 `keyword_tags` 为空数组
- **THEN** 提取结果仅含该 event 标签，不出现「新闻」或任何分类名/规则词标签
- **AND** 结果的来源标记保持 `llm`

#### Scenario: 空摘要文章两个数组均为空

- **WHEN** 一篇文章的摘要输入为空串或纯空白，LLM 返回 `{"event_person_tags":[],"keyword_tags":[]}`
- **THEN** 提取结果为零候选，不产生任何规则词候选

### Requirement: heuristic 兜底仅限提取调用失败

heuristic 兜底降级 SHALL 仅在提取调用失败（`err != nil`，含网络/HTTP/超时错误）时触发——无论降级判定位于提取层（extractor 内部零候选降级）还是编排层（tagger 兜底分支）；提取调用成功但结果为零标签时，SHALL 保持文章无标签状态（不写入任何行），等待后续重打流程。原「`len(result.Tags) == 0` 即降级 heuristic」与「extractor 内部零候选即降级 heuristic」的判定 SHALL 移除。

#### Scenario: LLM 调用成功但零标签

- **WHEN** 融合提取成功返回且候选总数为零
- **THEN** 文章不写入任何 `article_topic_tags` 行，不产生 heuristic 来源标签

#### Scenario: LLM 调用失败降级 heuristic

- **WHEN** 融合提取因连接拒绝/超时等返回错误且重试耗尽
- **THEN** 编排按既有行为降级 heuristic 提取，来源标记 `heuristic`

### Requirement: 泛词标签黑名单拦截

所有文章标签写入路径（mono、aggregate、heuristic 兜底、跨 feed 标签复用）SHALL 在持久化前拦截黑名单泛词标签：黑名单至少包含「新闻、论坛、要闻、快讯、文章、内容、技术、发展」，被拦截候选不落库、不进入辅助标签池。黑名单为代码内常量，不要求可配置。

#### Scenario: 分类名回填词被拦截

- **WHEN** 任一提取路径产出 label 为「新闻」的文章标签候选
- **THEN** 该候选在持久化前被丢弃，`article_topic_tags` 不产生新行

#### Scenario: 跨 feed 复用不复制泛词标签

- **WHEN** 一篇文章的 sibling（同 link）文章带有 label 为「新闻」的标签关联，跨 feed 复用逻辑为其复制 sibling 标签
- **THEN**「新闻」关联行不被复制到该文章，其余合法标签照常复制

#### Scenario: 黑名单不误伤正常标签

- **WHEN** 一篇 V2EX 编程讨论文章被提取出「GLM Coding Plan」等非泛词标签
- **THEN** 黑名单不拦截这些标签（黑名单仅含完整匹配的泛词，不做子串匹配）
