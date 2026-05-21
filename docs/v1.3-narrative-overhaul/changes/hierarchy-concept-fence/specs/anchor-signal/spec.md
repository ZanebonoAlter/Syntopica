## ADDED Requirements

### Requirement: Anchor 定义
anchor 是已放置标签（有 abstract parent 且 parent status=active）。新标签通过 anchor 找到候选 parent：anchor 的 parent 成为新标签的候选放置目标。

#### Scenario: 有效 anchor 提供候选 parent
- **WHEN** 新标签 "GPT-6 训练动态" 通过 cotag 找到 anchor "GPT-5.5 发布"（parent="OpenAI 产品动态"）
- **THEN** "OpenAI 产品动态" 成为新标签的候选 parent

#### Scenario: 无 parent 的标签不是 anchor
- **WHEN** cotag 找到的标签没有 abstract parent
- **THEN** 该标签被过滤掉，不作为 anchor

### Requirement: Anchor 信号源优先级
anchor 搜索 SHALL 先用 cotag（文章共现），不足时再用 semantic embedding 补充。

#### Scenario: Cotag 优先
- **WHEN** 新标签关联 ≥3 篇文章
- **THEN** 优先通过 article_topic_tags 找同文章的其他标签，筛出有 parent 的作为 anchor

#### Scenario: Cotag 不足时 embedding 补充
- **WHEN** cotag 找到的有效 anchor < 2 个
- **THEN** 用新标签的 semantic embedding 在同 category 标签中搜索相似标签，筛出有 parent 的补充

### Requirement: Anchor 阈值决策
anchor 匹配 SHALL 使用三级阈值：直接跟随（≥0.85）、LLM 投票（0.70-0.85）、放弃（<0.70）。

#### Scenario: 高置信直接跟随
- **WHEN** top-1 anchor 的相似度 ≥ 0.85
- **THEN** 直接跟随该 anchor 的 parent，不调用 LLM

#### Scenario: 中间带 LLM 投票
- **WHEN** top-1 anchor 的相似度 ∈ [0.70, 0.85) 且有 ≥2 个 anchor
- **THEN** 取 top-3 anchor 的 parent 列表，LLM 投票选择最佳 parent

#### Scenario: Anchor 共识无需 LLM
- **WHEN** top-3 anchor 的 parent 都相同
- **THEN** 直接跟随该 parent，不调用 LLM

#### Scenario: 低置信放弃
- **WHEN** top-1 anchor 的相似度 < 0.70
- **THEN** 放弃 anchor 路径，fall through 到 abstract embedding 匹配

### Requirement: Anchor 有效性检查
anchor 的 parent MUST 满足：status=active、属于同一 concept、parentDepth=targetDepth。任一不满足则该 anchor 无效。

#### Scenario: Anchor parent 不属于同一 concept
- **WHEN** anchor 的 parent 属于 concept A，但新标签匹配到 concept B
- **THEN** 该 anchor 无效，不参与投票

#### Scenario: Anchor parent depth 不匹配
- **WHEN** anchor 的 parent depth=2，但 targetDepth=1
- **THEN** 该 anchor 无效
