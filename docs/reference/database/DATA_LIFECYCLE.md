# 数据生命周期

本文档从数据状态字段变迁角度描述 RSS Reader 的核心数据链路。与 `architecture/data-flow.md` 的分工：

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

从 LLM 提取到建立层级关系的完整链路。统一术语：**Tag** = 叶标签 (source='llm'/'heuristic')，**Node** = 抽象标签 (source='abstract')，**Sector** = 板块概念 (board_concepts 表)。

```
┌─ LLM 标签提取 ───────────────────────────────────────────────────────────┐
│  来源: tag_jobs 处理 (article_lifecycle 触发)                            │
│                                                                          │
│  LLM → 候选标签列表 (label + category)                                  │
│                                                                          │
│  INSERT INTO ai_call_logs (capability='tag_extraction', ...)            │
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
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Tag 合并（源 DELETE）───────────────────────────────────────────────────┐
│  AutoTagMerge 调度器 (3600s)                                             │
│                                                                          │
│  pgvector 余弦相似度 > 0.97 的标签对:                                    │
│  → 迁移 article_topic_tags (source → target)                             │
│  → 迁移 topic_tag_relations (source children → target)                   │
│  → DELETE topic_tag_embeddings WHERE source                              │
│  → DELETE topic_tags WHERE id = source                                   │
│                                                                          │
│  注：不再使用 status='merged' 或 status='inactive'，源 Tag 硬删除。      │
│  → INSERT INTO merge_reembedding_queues (重算目标 Tag embedding)         │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ Tag 归属 Sector ────────────────────────────────────────────────────────┐
│  Tag embedding 就绪后，计算与该 category 下所有活跃 Sector 的余弦相似度 │
│  · 最高相似度 ≥ 0.6 (可配置) → UPDATE topic_tags SET concept_id = <Sector>│
│  · 低于阈值 → concept_id = NULL (unplaced)                              │
│                                                                          │
│  Sector 生成模式:                                                         │
│  · auto:  unplaced Tag 数 > auto_sector_threshold (默认 15) → LLM 提议  │
│  · LLM:   用户触发 → LLM 增量建议 (keep/add/merge/split) → 预览确认    │
│  · manual: 用户输入 label → LLM 补全 description → protected=true       │
│                                                                          │
│  board_concepts 新增字段: source ('auto'/'llm'/'manual'), protected,    │
│                          declining, peak_tag_count                       │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 质量评分 ───────────────────────────────────────────────────────────────┐
│  TagQualityScore 调度器 (3600s)                                          │
│                                                                          │
│  UPDATE topic_tags SET quality_score = <基于 article_count /             │
│         feed_count / hierarchy_depth 的综合评分>                         │
│                                                                          │
│  低质量 Tag (quality_score ≈ 0) → 清理 Phase 2 可能被删除               │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 层级放置 ───────────────────────────────────────────────────────────────┐
│  唯一入口: PlaceTagInHierarchy                                            │
│  embedding 就绪 → MatchTagToConcept → depth 检查 → 按 template 放置      │
│  唯一出口: DELETE (硬删除，无 merged/inactive 软状态)                      │
│                                                                          │
│  → Node 判定 (LLM + hierarchy_config templates)                          │
│  → INSERT INTO topic_tag_relations (parent_id, child_id,                │
│                                      relation_type='abstract')           │
│                                                                          │
│  → adopt_narrower_queues:     status: pending → processing → completed   │
│    (收养窄 Tag 到新建 Node)                                              │
│  → multi_parent_resolve_queues: status: pending → processing → completed │
│    (AI 判断多父级消歧)                                                   │
│  → abstract_tag_update_queues: status: pending → processing → completed  │
│    (子节点变化后刷新 Node 描述和 embedding)                               │
│                                                                          │
│  重建任务 (rebuild_jobs):                                                 │
│  → 模板变更时: DELETE 旧 Node/relations → 创建 rebuild_job              │
│  → rebuild_jobs.status: pending → running → completed / paused / failed  │
│  → 按 batch (默认 20) 处理，支持断点续传 (last_tag_id)                  │
│  → WebSocket 推送 hierarchy_rebuild (processing/completed/failed)        │
│                                                                          │
│  话题: topic_tags.status: active (无 merged/inactive 状态)               │
│        topic_tag_relations — 持续增量更新                                │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 清理机制 (7 Phase) ─────────────────────────────────────────────────────┐
│  tag_hierarchy_cleanup 调度器，按顺序执行，受 time budget 限制:          │
│                                                                          │
│  Phase 1 — 僵尸 Tag: DELETE 无文章/无关系/age>7d 的 Tag + embeddings    │
│  Phase 2 — 低质量 Tag: DELETE quality<0.15 且 article_count=1 的 Tag    │
│  Phase 3 — 空 Node:     DELETE 无子节点的 Node (source='abstract')      │
│  Phase 4 — 同 Level 去重: 同 Sector 同 Level Node 相似>0.90 → 合并删除 │
│  Phase 5 — Template 校验: 检测 depth/leaf 位置/children 超限 →          │
│            INSERT INTO hierarchy_pending_changes (status='pending')      │
│  Phase 6 — Sector 健康检查: auto 空→DELETE, LLM 衰退→declining,        │
│            manual 不动                                                   │
│  Phase 7 — 聚类: ClusterUnclassifiedTags 输出 anchor 信号，不创建 Node │
│            信号作为 PlaceTagInHierarchy 的上下文输入                     │
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
│  SELECT topic_tags WHERE quality_score > 0 AND status='active'          │
│  → 活跃标签列表                                                          │
│                                                                          │
│  SELECT article_topic_tags + articles WHERE pub_date within window      │
│  → 标签-文章关联                                                         │
│                                                                          │
│  SELECT topic_tag_relations WHERE relation_type='abstract'              │
│  → 抽象标签层级树                                                        │
└─────────────────────────────────────────────────────────────────────────┘
                              ↓
┌─ 双轨匹配 → Board 生成 ──────────────────────────────────────────────────┐
│  分类维度: global / feed_category                                        │
│                                                                          │
│  Pass 1: CollectAbstractTreeInputs                                       │
│    · 节点数 ≥ narrative_board_hotspot_threshold (6) → 热点板            │
│      INSERT INTO narrative_boards (is_system=true, abstract_tag_id,     │
│             abstract_tag_ids, event_tag_ids, period_date, scope_type)   │
│                                                                          │
│    · 节点数 < 6 → MatchTagToConcept                                      │
│      pgvector cosine similarity vs board_concepts.embedding             │
│      匹配阈值: narrative_board_embedding_threshold (0.7)                │
│                                                                          │
│      · 匹配成功 → INSERT INTO narrative_boards (board_concept_id, ...)  │
│      · 匹配失败 → 归入未归类桶                                           │
│                                                                          │
│  Pass 2: CollectUnclassifiedEventTags                                    │
│    · 未归类的 event 标签 → MatchTagToConcept → 概念板或保持未归类       │
│                                                                          │
│  后处理:                                                                  │
│  → runFallbackAssociations: 关联前日叙事 →                               │
│    UPDATE narrative_boards SET prev_board_ids = <前日 Board IDs>        │
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
2. `embedding_config` 表必须配置 embedding 模型和阈值
3. 可选：`embedding_config.narrative_board_embedding_threshold` 和 `narrative_board_hotspot_threshold`
4. `hierarchy_config` 表配置 HierarchyTemplate（Level 定义、max_children、is_leaf）
5. `rebuild_jobs` 表支持模板变更后的批量重建

### 叙事摘要

1. 需要至少 6 个活跃 Tag 在同一个 Node 下才能生成热点板
2. 需要 `board_concepts` 表中有活跃 Sector 才能做概念匹配
3. `NarrativeSummaryScheduler` 调度器需启用（86400s 间隔）
4. Sector 支持三种生成模式：auto / LLM / manual

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

### 2026-05-16

- 统一术语：Tag (叶标签) / Node (抽象标签) / Sector (板块概念)
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
- [数据库字段说明](DATABASE_FIELDS.md) — 38 张表的完整字段字典
- [全局实体关系图](ER_DIAGRAM.md) — FK 关系图与约束矩阵
- [项目架构总览](../architecture/overview.md) — 系统架构全局视角
