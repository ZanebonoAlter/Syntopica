
## 打标采样改造的落点与数据事实

改造点与关键事实（2026-09-19 实测）：

1. **唯一改造入口**：`backend-go/internal/tagmanagement/service/core/article_tagger.go:347` `buildArticleSummary`——body 选择链 AIContentSummary → FirecrawlContent → Content → Description，超 `maxSummaryRunesForTagging=4000`（:345 常量）一律掐头。双分支 extractor（event/person + keyword 并行两路 Chat，共享 input.Summary）与 aggregate 路径自动继承采样结果，调用方零改动。

2. **切分器必须新写、勿动 splitSections**：`section_splitter.go:23` 只认 `## ` 标题，且带 dropIntro/mergeShort/splitLong/capCount 后处理，是 aggregate「每栏目一次调用」的专用件（content_form=aggregate 检测在 reader/service/content_form.go，标记来自整理稿首行）。新采样切分器（design.md D1 命名 sampling_splitter.go）认 #~###### 全部层级 + 空行兜底，职责是拼预算。

3. **数据画像**（ai_call_logs/articles 近 3-14 天）：86% 文章仅 RSS 短摘要（不超预算，行为不变区）；重灾区 = FirecrawlContent 直打标长文 ~72 篇/天 99% 超限 + 超长整理稿 mono ~17 篇/天；超 12k 字符正文中 95%（409/431）带 `## ` 标题；firecrawl 正文为 markdown，图片 `![](url)` 与行内链接可占长文 20-30% 长度（纯噪声）。

4. **真实样本 fixture 取法**：少数派 feed 近 14 天 41 篇 mono 文章 avg 13.9k 字符（多数带 `## 栏目`），阮一峰 2 篇 aggregate；无标题叙事长文在 14 天 431 篇超长文中占 22 篇——tasks.md 2.2 三类夹具可按此筛选 SQL 取材（见会话内查询）。

**引用**：backend-go/internal/tagmanagement/service/core/article_tagger.go:347 buildArticleSummary、backend-go/internal/tagmanagement/service/core/section_splitter.go:23 splitSections、backend-go/internal/tagmanagement/service/core/extractor_enhanced.go:44 ExtractTags

<!-- pinned 2026-09-19T03:26:09Z -->
