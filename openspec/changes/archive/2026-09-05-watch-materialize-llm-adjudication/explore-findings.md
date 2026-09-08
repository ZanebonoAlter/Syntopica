
## 库内数据观察与用户定调（裁决标准）

库内实证（board 1974/1980，2026-08-25~09-04，3 个 watch：harness@keyword_topic / vibe coding开源生态跟踪@sentence_topic / 美伊形式对市场影响@sentence_topic，共 28 个物化 section）：

**质量问题证据**：
1. 美伊板块 09-02 期（section 3882，20 篇）约一半与"市场影响"无关：纯战报（俄助伊朗研发导弹、击落MQ-9）、人道新闻（婚礼现场遭袭）、外交表态（沙特谴责）——根因：句子向量命中宽泛标签（美伊冲突/中东局势）后 ListArticlesByTagsForDay 全量并集，限定词"对市场影响"从未参与过滤。
2. vibe coding 板块 09-03 期（section 3940，12 篇）退化为泛 AI 编程流：SIE 推理引擎、ReAct 教程、LangChain 实战、AIOps、Scaling Harness Intelligence 论文——"vibe coding"+"开源生态"双限定词全失效。
3. harness 板块 09-03 期（section 3941，18 篇）相当部分标题不含 harness（Google AI 补丁、T3 Code、AgentTeams），仅摘要正文顺带提及命中；与 vibe coding 板块大量重叠（企业 Harness、Codex Harness 套壳、Scaling Harness Intelligence 双边出现）。
4. 对照组：label 轨 AI 命中 reason 质量明显高（"美伊紧张关系重燃直接推高布伦特原油价格，直接关联能源市场影响"）——LLM 裁决在同系统已被证明有效。

**bug 证据**：topic_watch_hits 中 sentence_topic/keyword_topic watch 从 08-26 起持续有 AI 风格命中记录。代码根因：daily_report_watch.go evaluateWatchHitsWithChat 分流 `if w.Type == WatchTypeKeyword {keyword组} else {label组}`——只排除 'keyword'，'keyword_topic'/'sentence_topic' 全落 else 进 AI 判定。违反 topic-watch spec 红线（物化轨 SHALL NOT 产生命中提示）。修复=分流收紧为仅 type=label 走 AI 组。

**用户定调（裁决标准与范围）**：
- 裁决标准：意图限定词 + **实质因果链**——"沙特谴责伊朗袭击船只"是美伊军事行动波及、有因果关系，要保留；不是字面紧扣关键词。
- 跨板块重叠：暂不去重（明确排除）。
- 板块标题：LLM 起当日标题，watch 名作为小装饰保留，展示同现有话题板块。
- 提示轨 keyword 零 AI 红线不动；召回层（向量阈值/DNF）不动。

**其他事实**：sentence 轨文章池无 LIMIT（20 篇是自然量非截断）；thread summary 是原文 HTML 摘录未清洗（另一层面问题，不在本 change 范围）；ListArticlesByTagsForDay 在 topic_watch_repository.go:416。

<!-- pinned 2026-09-05T02:48:58Z -->
