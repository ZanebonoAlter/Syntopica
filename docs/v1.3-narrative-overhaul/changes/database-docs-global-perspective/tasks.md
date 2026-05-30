## 1. DATABASE_FIELDS.md 补齐表覆盖

- [x] 1.1 查询实际 DB schema，为 `ai_summaries`、`ai_summary_feeds`、`ai_summary_topics` 三张表编写字段说明，归入 §6 AI 配置相关表后新增 §6.5 "AI 摘要关联表"
- [x] 1.2 查询实际 DB schema，为 `otel_spans` 编写字段说明，归入 §13 其他表
- [x] 1.3 查询实际 DB schema，为 `hierarchy_config`、`hierarchy_config_versions` 编写字段说明，新增 §8 "层级关系相关表"
- [x] 1.4 查询实际 DB schema，为 `adopt_narrower_queues`、`multi_parent_resolve_queues`、`abstract_tag_update_queues` 编写字段说明，归入 §8 层级关系相关表
- [x] 1.5 为 `ai_summaries`、`ai_summary_feeds`、`ai_summary_topics`、`digest_configs` 四张孤儿表新增 §14 "已废弃/预留表"章节，标注"当前无 Go 代码引用，可能为旧版功能遗留"
- [x] 1.6 补充 `articles.feed_summary_id` 字段到 articles 表字段说明
- [x] 1.7 更新文件头部"完整表清单"表格，从 29 张扩充到 38 张

## 2. DATABASE_FIELDS.md 章节迁移

- [x] 2.1 删除"工作流程"章节（570-614 行，完整的内容处理流程 ASCII 图）
- [x] 2.2 删除"状态流转图"章节（617-632 行，Firecrawl 和 Summary 状态流转）
- [x] 2.3 删除"配置要求"章节（662-677 行，功能启用条件）
- [x] 2.4 修正"相关文档"章节路径：`docs/architecture/data-flow.md` → `docs/reference/architecture/data-flow.md`，新增指向 `ER_DIAGRAM.md` 和 `DATA_LIFECYCLE.md` 的链接

## 3. ER_DIAGRAM.md 新建

- [x] 3.1 编写 ASCII 全局概览图：6 个业务域（Core、Topic Tags、AI Summaries、Narrative、Hierarchy、AI Infrastructure）及其跨域 FK 依赖
- [x] 3.2 编写核心数据面 Mermaid erDiagram：`categories`、`feeds`、`articles`、`firecrawl_jobs`、`tag_jobs`、`reading_behaviors`、`user_preferences`
- [x] 3.3 编写主题标签面 Mermaid erDiagram：`topic_tags`、`topic_tag_embeddings`、`topic_tag_relations`、`article_topic_tags`、`embedding_queues`、`merge_reembedding_queues` 及相关队列表
- [x] 3.4 编写 AI 摘要面 Mermaid erDiagram：`ai_summaries`、`ai_summary_feeds`、`ai_summary_topics` 及其与 `articles`、`feeds`、`topic_tags` 的关系
- [x] 3.5 编写叙事摘要面 Mermaid erDiagram：`narrative_boards`、`narrative_summaries`、`board_concepts` 及其与 `topic_tags`、`categories` 的关系
- [x] 3.6 编写层级关系面 Mermaid erDiagram：`topic_tag_relations`、`hierarchy_config`、`hierarchy_config_versions`、`adopt_narrower_queues`、`multi_parent_resolve_queues`、`abstract_tag_update_queues`、`hierarchy_pending_changes`
- [x] 3.7 编写 AI 基础设施 Mermaid erDiagram：`ai_providers`、`ai_routes`、`ai_route_providers`、`ai_call_logs`、`ai_settings`、`scheduler_tasks`
- [x] 3.8 编写 FK 引用矩阵表格：35 行，列为 source_table、fk_column、target_table、target_column、constraint_name
- [x] 3.9 编写关系模式说明章节：桥接表、自引用、反规范化、JSON-stored ID lists

## 4. DATA_LIFECYCLE.md 新建

- [x] 4.1 编写"文章生命周期"链：RSS 入库 → Firecrawl → AI 总结 → 用户阅读，标注 `firecrawl_status`、`summary_status` 状态字段变迁和涉及的表
- [x] 4.2 编写"主题标签生命周期"链：LLM 提取 → embedding 去重 → 入库 → 向量化 → 自动合并 → 层级关系，标注 `topic_tags.status`、`embedding_queues.status` 等状态字段变迁
- [x] 4.3 编写"阅读反馈生命周期"链：用户行为 → `reading_behaviors` → `user_preferences` → 影响排序权重
- [x] 4.4 编写"叙事生成生命周期"链：活跃 tags → 双轨匹配 → Board 生成 → Summary 生成 → 后处理，标注 `narrative_summaries.status` 等状态字段
- [x] 4.5 编写"预留功能"章节：AI 批量摘要链（`ai_summaries` 关系链）和 Digest 推送链（`digest_configs`），标注"当前未启用"
- [x] 4.6 迁移"配置要求"内容：功能启用条件（Firecrawl 依赖、AI 总结依赖等）
- [x] 4.7 编写与 `data-flow.md` 的边界说明和交叉引用

## 5. _index.md 新建

- [x] 5.1 编写数据库全景概览：38 张表、6 个业务域、35 条 FK、`topic_tags` 作为枢纽（12 条入边）
- [x] 5.2 编写导航链接：`DATABASE_FIELDS.md`、`ER_DIAGRAM.md`、`DATA_LIFECYCLE.md` 各附一句话描述

## 6. 交叉引用更新

- [x] 6.1 更新 `architecture/overview.md` "相关文档"章节，新增指向 `database/ER_DIAGRAM.md` 和 `database/DATA_LIFECYCLE.md` 的链接
- [x] 6.2 更新 `architecture/data-flow.md` 头部，加一句指向 `database/DATA_LIFECYCLE.md` 的交叉引用说明
