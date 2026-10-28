
## 打标示例泄漏根因与护栏挂点

打标链路与示例泄漏事实（change: guard-tag-exemplar-leakage）

【链路】article_tagger.go:tagArticle → content_form 为空走 mono 路径 extractor.ExtractTags(input)；content_form=="aggregate" 走 tagAggregateArticle（按 AIContentSummary 切栏目逐段抽）。ExtractionInput.Summary = buildArticleSummary(article)：AIContentSummary → FirecrawlContent → Content → Description 优先级，经 sampleTaggingSummary 采样到 4000 runes（long-form-sampled-tagging）。周刊 416 (id=153877) 走 mono，输入= FirecrawlContent 采样。

【根因】extractor_enhanced.go:buildMergedExtractionPrompt() 的 system prompt 把 PostgreSQL 当 few-shot 正例出现 3 次：keyword 定义举例「"Transformer架构"、"PostgreSQL"」、description 示例「"PostgreSQL" → "开源关系型数据库"」、输出格式正例 JSON {"label":"PostgreSQL",...,"description":"开源关系型数据库"}。本地小模型 Qwen3.5-4B-UD-Q8_K_XL.gguf 照抄示例：ai_call_logs id=2428355 响应 keyword_tags 含 {"label":"PostgreSQL","description":"开源关系型数据库"}（与 prompt 示例一字不差），且 event 标签的 auxiliary_labels 里也被塞了 PostgreSQL。

【正文验证】articles 153877 的 content(12977)/firecrawl_content(10280) 中 postgres/数据库/SQL/sqlite/supabase 全部 0 命中；周刊外部原文（GitHub md + 博客版）与 Thorsten Ball 原推图片 OCR 也无 Postgres。

【污染面】topic_tags id=1556 PostgreSQL 挂 36 篇文章，25 篇（69%）正文（title+content+firecrawl_content 拼接）0 次出现 postgres = 误打。对照组 Transformer 标签(2168) 6 篇全部有真实提及。

【护栏挂点】mono: ExtractTags 解析后返回前；aggregate: extractTagsFromSection（extractor_enhanced.go）。已有工具：Slugify()（归一对比用）、CleanupOrphanedTags(tagIDs)（删关联后清孤儿）、limitArticleTags。护栏语义 = 「模型输出必须在其可见输入（Title+Summary）中有依据」：输出 label 的 Slugify 归一 ∈ prompt 示例词集合 且 Title+Summary 中不 Contains（大小写不敏感）→ 丢弃。auxiliary_labels 同规则逐项过滤。示例词集合提取为常量 + 防漂移单测（断言集合词均出现在 prompt 文本中）。

**引用**：backend-go/internal/tagmanagement/service/core/extractor_enhanced.go:buildMergedExtractionPrompt、backend-go/internal/tagmanagement/service/core/article_tagger.go:buildArticleSummary、backend-go/internal/tagmanagement/service/core/article_tagger_aggregate.go:tagAggregateArticle

<!-- pinned 2026-10-24T05:01:16Z -->
