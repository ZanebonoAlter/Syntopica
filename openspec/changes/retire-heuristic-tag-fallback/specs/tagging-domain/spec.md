# tagging-domain — retire-heuristic-tag-fallback delta

## MODIFIED Requirements

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

## ADDED Requirements

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

## REMOVED Requirements

### Requirement: heuristic 兜底仅限提取调用失败
**Reason**: 规则词（heuristic）兜底打标整体退役（retire-heuristic-tag-fallback，2026-11-12）。原契约「heuristic 兜底降级 SHALL 仅在提取调用失败时触发」的前提——存在一条规则词兜底路径——已不成立：提取层 `extractor_enhanced.go` 的 `extractWithHeuristic` 与编排层 `article_tagger.go` 的 `legacyExtractTopics` 均已删除，规则词表 `extractor_heuristic.go` 整文件退役。兜底吞掉错误导致打标任务被误标 completed、文章永不重打，正是本次报障的病根。

**Migration**: 失败语义由新增 requirement「提取失败不降级，直接判任务失败」接管——提取失败即返回错误，文章不写任何标签，任务标记失败并进入既有队列退避重试（上限 `max_attempts`），仍失败停在 `failed` 态待人工/批量 retag。原 requirement 中「提取调用成功但零标签时文章保持无标签」的有效条款已并入新 requirement。
