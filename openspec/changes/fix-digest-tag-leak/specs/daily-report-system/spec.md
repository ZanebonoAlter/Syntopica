## MODIFIED Requirements

### Requirement: 事件标签去重

系统 SHALL 在生成日报前对收集到的事件标签进行程序化精确去重，不使用 LLM。去重 SHALL 应用两条规则：(1) 关联文章集合完全相同的标签合并为一个；(2) article_count=1 且关联同一篇文章的标签合并为一个。合并时存活者 SHALL 按以下优先级选出：

1. article_count 更大者存活；
2. article_count 相同时，tag identity embedding 与当前版块 embedding 的 cosine 更高者存活（版块相关性 tiebreak）；
3. cosine 相同或任一 embedding 缺失时，id 更小者存活（回退现行为）。

去重 SHALL 不改变原始标签数据，仅在生成流程中使用去重后的列表。

#### Scenario: 文章集合完全相同的标签去重
- **WHEN** 标签 A 和标签 B 关联的文章 ID 集合均为 {101, 102, 103}
- **THEN** 系统 SHALL 将二者合并为一个（按三级 tiebreak 保留存活者）

#### Scenario: 单篇文章标签去重
- **WHEN** 标签 X (article_count=1, 关联文章 200) 和标签 Y (article_count=1, 关联文章 200) 为不同标签但指向同一文章
- **THEN** 系统 SHALL 将二者合并为一个（存活者按三级 tiebreak 选出）

#### Scenario: 同文章双事件标签保留贴近版块者
- **WHEN** 同一篇 digest 文章拆出事件标签 X（如「OpenAI 营收口径反转」，identity 与当前版块 cosine=0.60）和事件标签 Y（如「特朗普与普京达成柴油供应协议」，identity 与当前版块 cosine=0.80），二者 article_count 均为 1 且关联同一文章
- **THEN** 系统 SHALL 保留 Y（cosine 更高，贴近当前版块），丢弃 X

#### Scenario: embedding 缺失回退 ID 顺序
- **WHEN** 标签 X 与 Y 同文章、article_count 相同，且任一标签无 identity embedding 或版块无 embedding
- **THEN** 系统 SHALL 保留 id 更小者（现行为兜底）

#### Scenario: 去重不影响原始数据
- **WHEN** 去重流程执行后
- **THEN** `topic_tags` 表中的原始标签记录 SHALL 保持不变，去重仅在内存中进行
