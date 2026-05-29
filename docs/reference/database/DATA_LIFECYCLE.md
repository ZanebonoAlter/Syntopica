# 数据生命周期

本文档从数据状态字段变迁角度描述 Syntopica 的核心数据链路。与 `architecture/data-flow.md` 的分工：

```
data-flow.md       = "代码怎么跑的"（函数调用链、API 调用、前端 store 交互）
DATA_LIFECYCLE.md  = "数据怎么变的"（哪些表被写入、状态字段怎么流转、数据产出依赖）
```

---

## 文章生命周期

一篇文章从 RSS 入库到进入叙事摘要的完整状态变迁链：

```
┌─ RSS 入库 ──────────────────────────────────────────────────────────────┐
│  feeds → articles                                                        │
│  INSERT INTO articles (feed_id, title, content, firecrawl_status, ...)  │
│  articles.firecrawl_status = 'pending'                                  │
│  articles.summary_status   = 'complete' (默认)                           │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 可选: Firecrawl 全文抓取 ───────────────────────────────────────────────┐
│  条件: feed.firecrawl_enabled = true                                     │
│  需要: 全局 Firecrawl API 配置（ai_settings / AI Provider/Route）        │
│                                                                          │
│  INSERT INTO firecrawl_jobs (article_id, status='pending', ...)         │
│  firecrawl_jobs.status: pending → processing → completed                │
│                                                                          │
│  成功时:                                                                  │
│  UPDATE articles SET                                                     │
│    firecrawl_content = <完整 Markdown 正文>,                             │
│    firecrawl_status  = 'completed',                                     │
│    firecrawl_crawled_at = NOW(),                                         │
│    summary_status    = 'incomplete'  ← 标记需要 AI 总结                 │
│                                                                          │
│  失败时:                                                                  │
│  UPDATE articles SET                                                     │
│    firecrawl_status = 'failed',                                         │
│    firecrawl_error  = <错误信息>                                        │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 可选: AI 文章级总结 ────────────────────────────────────────────────────┐
│  条件: articles.summary_status = 'incomplete'                            │
│        AND feed.article_summary_enabled = true                           │
│  需要: AI Provider/Route 配置                                            │
│                                                                          │
│  UPDATE articles SET summary_status = 'pending'                         │
│  articles.summary_status: incomplete → pending → processing → complete  │
│                                                                          │
│  成功时:                                                                  │
│  UPDATE articles SET                                                     │
│    ai_content_summary          = <AI 生成的 Markdown 整理稿>,           │
│    summary_status              = 'complete',                            │
│    summary_generated_at        = NOW(),                                 │
│    summary_processing_started_at = <处理开始时间>                        │
│                                                                          │
│  失败时:                                                                  │
│  UPDATE articles SET                                                     │
│    summary_status = 'failed',                                           │
│    completion_error = <错误信息>,                                        │
│    completion_attempts = completion_attempts + 1                         │
│                                                                          │
│  日志: INSERT INTO ai_call_logs (capability, success, latency_ms, ...)  │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 标签提取 ───────────────────────────────────────────────────────────────┐
│  INSERT INTO tag_jobs (article_id, status='pending', ...)               │
│  tag_jobs.status: pending → processing → completed                       │
│                                                                          │
│  LLM 从 firecrawl_content / ai_content_summary / content 提取标签        │
│  → INSERT/UPDATE article_topic_tags (article_id, topic_tag_id, score)   │
│  → INSERT/UPDATE topic_tags (label, category, slug, ...)                │
│  → INSERT INTO embedding_queues (tag_id, status='pending')              │
│                                                                          │
│  条件: articles.firecrawl_status = 'completed' OR 始终启用               │
│        需要: tag_extraction capability AI 配置                           │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 用户阅读 ───────────────────────────────────────────────────────────────┐
│  前端 tracking → POST /api/reading-behavior                             │
│  INSERT INTO reading_behaviors (article_id, event_type, scroll_depth,   │
│                                  reading_time, session_id, ...)          │
│                                                                          │
│  PreferenceUpdate 调度器 (30min) → 聚合 →                                │
│  INSERT/UPDATE user_preferences (feed_id, category_id, preference_score)│
│  → 影响文章排序权重                                                      │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 主题标签生命周期

从 LLM 提取标签到 SemanticBoard 匹配的完整链路。统一术语：**Tag** = 事件/关键词/人物标签 (source='llm'/'heuristic')，**Auxiliary Label** = 辅助标签 (semantic_labels.label_type='auxiliary')，**SemanticBoard** = 语义板块 (semantic_labels.label_type='board')。

```
┌─ LLM 标签提取 ───────────────────────────────────────────────────────────┐
│  来源: tag_jobs 处理 (article_lifecycle 触发)                            │
│                                                                          │
│  LLM → 候选标签列表 (label + category) + 3-5 个辅助标签               │
│                                                                          │
│  INSERT INTO ai_call_logs (capability='tag_extraction', ...)            │
│  → INSERT/UPDATE topic_tags (tag 入库)                                  │
│  → 辅助标签同步入库（见下方入库流程）                                    │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Embedding 去重 + 入库 ──────────────────────────────────────────────────┐
│  embedding_queues.status: pending → processing → completed               │
│                                                                          │
│  调用 embedding API 生成向量 →                                            │
│  pgvector cosine similarity 与已有标签比较:                               │
│                                                                          │
│  · 相似度 ≥ high_similarity_threshold (0.97) → 复用已有标签             │
│  · 相似度 ≤ low_similarity_threshold (0.78)  → 创建新标签               │
│  · 中间地带 → 标记为需要人工判断                                         │
│                                                                          │
│  INSERT INTO topic_tags (label, category, slug, source='llm', ...)      │
│  INSERT INTO topic_tag_embeddings (topic_tag_id, embedding,             │
│                                     dimension, model, text_hash)         │
│  INSERT INTO article_topic_tags (article_id, topic_tag_id, score)       │
│                                                                          │
│  --- 辅助标签入库 ---                                                    │
│  对每个 tag 的辅助标签，按三级匹配入库：                                │
│  L1: slug/alias 精确匹配 → 复用已有 auxiliary label (ref_count++)       │
│  L2: embedding ≥ 0.95 合并 → 小方 label 加入大方 aliases (ref_count++) │
│  L3: 无匹配 → 新建 semantic_label(label_type=auxiliary) + 生成 embedding│
│  写入 topic_tag_semantic_labels (tag → auxiliary label 关联)            │
│  禁用标签 (status=disabled) 不参与 L1/L2 匹配                           │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Tag 合并（源 DELETE）───────────────────────────────────────────────────┐
│  AutoTagMerge 调度器 (3600s)                                             │
│                                                                          │
│  pgvector 余弦相似度 > 0.97 的标签对:                                    │
│  → 迁移 article_topic_tags (source → target)                             │
│  → DELETE topic_tag_embeddings WHERE source                              │
│  → DELETE topic_tags WHERE id = source                                   │
│                                                                          │
│  注：不再使用 status='merged' 或 status='inactive'，源 Tag 硬删除。      │
│  → INSERT INTO merge_reembedding_queues (重算目标 Tag embedding)         │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard 匹配 ──────────────────────────────────────────────────────┐
│  读取 tag 的辅助标签和 active SemanticBoard composition                  │
│                                                                          │
│  · 直接命中: tag 的辅助标签 ∈ board 构成标签 → 直接挂载                 │
│  · 命中率 > 50% → 直接挂载                                              │
│  · max_sim ≥ 0.8 → 直接挂载                                              │
│  · 加权综合: 0.6×max_sim + 0.4×hit_rate ≥ 阈值 → 挂载                  │
│                                                                          │
│  默认最多 3 个 board，按匹配分排序                                       │
│  写入 topic_tag_board_labels (topic_tag_id, semantic_board_id, score,    │
│    match_reason)                                                          │
│                                                                          │
│  匹配参数从 ai_settings 读取: semantic_board_match_*                     │
│  冷启动无 SemanticBoard 时：不匹配，不报错                               │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard 升级建议（手动触发）───────────────────────────────────────┐
│  用户手动触发，收集 ref_count ≥ 语义配置阈值的候选辅助标签            │
│                                                                          │
│  1. 预聚类：embedding 余弦距离 < 0.7 的候选分为簇                        │
│  2. 补充上下文：每个簇补充 co-tag 事件（30天窗口、top 20、去重>0.85）  │
│  3. LLM 判断：每个簇 → create_new / merge_into_existing / skip          │
│  4. 用户确认后：创建新 SemanticBoard 或更新已有 board_composition       │
│  5. 可触发回填重算 topic_tag_board_labels                               │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 回填队列 ───────────────────────────────────────────────────────────────┐
│  支持 all / unassigned / board 三种回填模式                              │
│  异步逐个执行 Board 匹配并重写 topic_tag_board_labels                   │
│  已有归属会被新匹配结果覆盖（幂等）                                     │
│  回填进度和失败记录可查询                                               │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 阅读反馈生命周期

用户行为如何转化为偏好数据并影响排序：

```
┌─ 用户交互 ───────────────────────────────────────────────────────────────┐
│  前端 ArticleContentView:                                                 │
│  → open (打开文章) / scroll (滚动) / close (关闭) / favorite (收藏)     │
│  → useReadingTracker 批量收集                                            │
│  → POST /api/reading-behavior                                           │
│                                                                          │
│  INSERT INTO reading_behaviors (                                         │
│    article_id, feed_id, category_id,                                    │
│    session_id, event_type, scroll_depth, reading_time                   │
│  )                                                                       │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 偏好聚合 ───────────────────────────────────────────────────────────────┐
│  PreferenceUpdate 调度器 (1800s)                                         │
│                                                                          │
│  SELECT 聚合 reading_behaviors:                                          │
│    AVG(reading_time), AVG(scroll_depth), COUNT(*), MAX(created_at)       │
│  GROUP BY feed_id, category_id                                          │
│                                                                          │
│  INSERT/UPDATE user_preferences (                                        │
│    feed_id, category_id,                                                │
│    preference_score  = <加权计算>,                                      │
│    avg_reading_time  = <平均阅读时间>,                                   │
│    interaction_count = <总交互数>,                                       │
│    scroll_depth_avg  = <平均滚动深度>,                                   │
│    last_interaction_at = NOW()                                           │
│  )                                                                       │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 影响排序 ───────────────────────────────────────────────────────────────┐
│  articles.relevance_score (SQL 计算列)                                   │
│  = preference_score * (标签匹配度) + 新鲜度衰减                          │
│                                                                          │
│  前端 fetchArticles 使用 ORDER BY relevance_score DESC                  │
│  → 用户偏好的 feed/category 文章排前                                    │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 叙事生成生命周期

从活跃标签到每日叙事摘要的完整链路：

```
┌─ 输入收集 ───────────────────────────────────────────────────────────────┐
│  NarrativeSummary 调度器 (86400s)                                        │
│                                                                          │
│  SELECT article_topic_tags + articles WHERE pub_date within window      │
│  → 标签-文章关联                                                         │
│                                                                          │
│  SELECT semantic_labels WHERE label_type='board' AND status='active'    │
│  → 全局共享 SemanticBoard                                                │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ SemanticBoard → NarrativeBoard 生成 ────────────────────────────────────┐
│  分类维度: global / feed_category                                        │
│                                                                          │
│  CollectSemanticBoardNarrativeInputs                                     │
│    · 按 date + scope + semantic_board_id 收集 active event tags          │
│    · 数据源为 topic_tag_board_labels 持久化匹配结果                      │
│    · category scope 通过 articles → feeds.category_id 限定文章范围       │
│                                                                          │
│  对每个有事件的 SemanticBoard:                                           │
│    · INSERT INTO narrative_boards (semantic_board_id, event_tag_ids,     │
│             period_date, scope_type, scope_category_id, scope_label)     │
│    · prev_board_ids 按 semantic_board_id + scope + 前一日匹配            │
│    · 同一 event tag 可出现在多个 NarrativeBoard，用于多视角叙事          │
│                                                                          │
│  无 SemanticBoard 或无匹配 event tags 时生成 0 个 NarrativeBoard，不报错│
│                                                                          │
│  后处理:                                                                  │
│  → DeriveBoardConnections: 派生 Board 间关系                            │
│  → runFeedbackFromTodayNarratives: 回写标签质量反馈                     │
│  → cleanEmptyBoards: 删除无 tag 的 Board                                │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Summary 生成 ───────────────────────────────────────────────────────────┐
│  每个 Board 调用 LLM 生成叙事摘要:                                        │
│                                                                          │
│  INSERT INTO narrative_summaries (                                       │
│    title, summary, status, period, period_date,                         │
│    related_tag_ids, related_article_ids,                                │
│    parent_ids, board_id, scope_type, scope_category_id                  │
│  )                                                                       │
│                                                                          │
│  narrative_summaries.status: emerging / continuing / splitting /        │
│                              merging / ending                            │
│                                                                          │
│  INSERT INTO ai_call_logs (capability='narrative_summary', ...)         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 配置要求

### Firecrawl 全文抓取

1. 全局配置（AI Provider/Route 配置中的 Firecrawl capability）
2. Feed 级别配置：`feeds.firecrawl_enabled = true`

### AI 文章级总结

1. 全局配置（AI Provider/Route 配置）
2. Feed 级别配置：`feeds.article_summary_enabled = true`

**依赖关系**：AI 总结功能依赖 Firecrawl 先抓取完整内容。如果 Firecrawl 失败（`articles.firecrawl_status = 'failed'`），AI 总结会被跳过。

### 主题标签相关

1. 全局配置：AI Provider/Route（`tag_extraction` capability）
2. `embedding_config` 表必须配置 embedding 模型
3. `ai_settings` 中的 `semantic_board_match_*` 控制 tag → SemanticBoard 匹配
4. `ai_settings` 中的 `semantic_board_upgrade_*` 控制升级建议
5. `topic_tag_semantic_labels` 记录 tag → auxiliary label
6. `topic_tag_board_labels` 记录 tag → SemanticBoard，用于叙事板输入

### 叙事摘要

1. 需要 active SemanticBoard（`semantic_labels.label_type='board'`）且当日有匹配 event tags 才会生成 NarrativeBoard
2. 冷启动无 SemanticBoard 或无匹配 event tags 时生成 0 个 NarrativeBoard，不报错
3. `NarrativeSummaryScheduler` 调度器需启用（86400s 间隔）
4. NarrativeBoard 是每日/scope 实例，长期语义资产是 SemanticBoard
5. Board 叙事上下文来自 SemanticBoard 的 label 和 description

---

## 预留功能

以下功能的数据表和字段在数据库中已建立，但当前没有 Go 代码调用，标记为"预留/已废弃"。

### AI 批量摘要（Feed 级）

```
ai_summaries (Feed 级批量摘要)
  ← feeds.feed_id
  ← ai_summary_feeds (关联快照)
  → ai_summary_topics → topic_tags
  → articles.feed_summary_id (文章关联回摘要)

状态: 无 Go 代码引用，0 行数据
表: ai_summaries, ai_summary_feeds, ai_summary_topics
```

### Digest 推送（日报/周报）

```
digest_configs (推送配置)
  → 飞书 Webhook / Obsidian 导出

状态: 无 Go 代码引用，0 行数据
表: digest_configs
```

---

## 更新日志

### 2026-05-22

- 语义标签/板块体系重构：移除层级放置、Sector 归属、质量评分、7 Phase 清理
- 新增辅助标签入库（L1/L2/L3）、SemanticBoard 匹配、升级建议、回填队列
- 叙事生成改为 SemanticBoard 派生，移除 abstract tree 和 board_concepts 路径
- 冷启动允许无 board，同一事件可出现在多个 NarrativeBoard

### 2026-05-16

- 统一术语：Tag (叶标签) / Auxiliary Label (辅助标签) / SemanticBoard (语义板块)
- 合并逻辑改为源 DELETE，移除 status='merged'/'inactive'
- 新增 Sector 生成模式 (auto/LLM/manual) 和归属流程
- 新增 rebuild_jobs 重建任务生命周期
- 清理机制更新为 7 Phase 模板感知清理

### 2026-05-14

- 初始版本：4 条核心生命周期链 + 配置要求 + 2 条预留功能说明
- 从 `DATABASE_FIELDS.md` 迁移"工作流程"、"状态流转图"、"配置要求"三节内容

---

## 相关文档

- [代码执行流](../architecture/data-flow.md) — 函数调用链、API 调用、前端 store 交互（"代码怎么跑的"）
- [数据库字段说明](DATABASE_FIELDS.md) — 35 张表的完整字段字典
- [全局实体关系图](ER_DIAGRAM.md) — FK 关系图与约束矩阵
- [项目架构总览](../architecture/overview.md) — 系统架构全局视角
