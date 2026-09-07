# 数据生命周期

本文档从数据状态字段变迁角度描述 Syntopica 的核心数据链路。与 `flow/` 的分工：

```
flow/              = "业务怎么跑的"（链路概要设计、函数调用链、API 调用、前后端协作）
DATA_LIFECYCLE.md  = "数据怎么变的"（哪些表被写入、状态字段怎么流转、数据产出依赖）
```

---

## 文章生命周期

一篇文章从 RSS 入库到进入日报生成的完整状态变迁链：

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
│  firecrawl_jobs.status: pending → leased → completed / failed   │
│  (lease 模式: Claim 时 leased_at+lease_expires_at；到期/重启回收)  │
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
│  tag_jobs.status: pending → leased → completed / failed            │
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
│  embedding_queues.status: pending → processing → completed / failed │
│  (worker: SELECT FOR UPDATE SKIP LOCKED；失败需手动 RetryFailed)       │
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
│  手动触发或 tag_quality_score 调度器定期重算                              │
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
│  用户手动触发 或 board_upgrade_suggest 调度器(默认每日06:30)，收集      │
│  ref_count ≥ 语义配置阈值的候选辅助标签
│                                                                          │
│  1. 预聚类：embedding 余弦距离 < 0.7 的候选分为簇                        │
│  2. 补充上下文：每个簇补充 co-tag 事件（30天窗口、top 20、去重>0.85）  │
│  3. LLM 判断：每个簇 → create_new / merge_into_existing / watch        │
│  4. 用户确认后：创建新 SemanticBoard 或更新已有 board_composition       │
│  5. 可触发回填重算 topic_tag_board_labels                               │
│  6. watch GC：超过观察期（默认30天）未确认的 watch 建议自动 dismissed   │
│     (board_upgrade_suggestions.status pending→dismissed, resolved_by='watch_gc') │
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

## 数据清理与保留策略

本节记录各表的真实清理/回收机制（真相源：`internal/admin/scheduler/*` + `internal/app/runtime.go`）。**除明确列出者外，其他表无自动清理**，行会无限累积。

### 调度器驱动的定时清理

| 调度器 | 间隔 | 清理对象与条件 |
| ------ | ---- | -------------- |
| `log_cleanup` | 86400s（每日，启动延迟5min） | `DELETE FROM ai_call_logs WHERE created_at < now()-7天`；`DELETE FROM otel_spans WHERE start_time_unix_nano < now()-7天`。**保留 7 天**。另：`DELETE FROM ai_embedding_cache WHERE created_at < now()-14天`（embedding 结果缓存，仅白名单 operation 落行；存储格式为 bytea 二进制 float32 小端字节流，~10KB/条，见 `models/embedding_codec.go`——optimize-pg-storage：原 jsonb 文本形式 ~31.5KB/条，2026-08-28 起 pre-migrate 非破坏转换）；`DELETE FROM embedding_queues WHERE status='completed' AND created_at < now()-30天`（已完成队列行，保留 30 天）。 |
| `aux_label_cleanup` | 3600s（每时，启动延迟10min） | 软禁用「无活跃引用」的辅助标签：`semantic_labels` 中 `label_type='auxiliary' AND status='active' AND protected=false AND created_at < now()-1天` 且无 `topic_tag_semantic_labels` 引用且不在 `board_composition` 中 → `status='disabled'`（并删其 board_composition 行）。**不硬删**，模式为 disable、宽限1天。 |
| `blocked_article_recovery` | 3600s（每时） | 恢复卡在 `articles.firecrawl_status IN ('waiting_for_firecrawl','blocked')` 且其 `feed.firecrawl_enabled=true` 的文章 → 置回 `pending` 重试。另含 STAT-05 告警（阻塞数>50 时 WARN）。 |
| `preference_update` | 1800s（每30min） | 聚合 `reading_behaviors`→`user_preferences`；并运行孤儿清理：修复/删除 category_id 指向已删分类的 reading_behaviors，删除 feed_id 指向已删源的 reading_behaviors 与 user_preferences。**仅孤儿清理，无时间型 TTL**。 |
| `board_upgrade_suggest` | 墙钟（默认每日06:30） | 生成 discover_new 升级建议 + **watch GC**：`board_upgrade_suggestions` 中 `decision='watch' AND status='pending'` 且创建超过观察期（`ai_settings.semantic_board_upgrade_watch_gc_days`，默认30天）→ `status='dismissed'`、`resolved_by='watch_gc'`。**软回收，不硬删**。 |
| `tag_quality_score` | 3600s | 重算 `topic_tags` 质量分并执行 tag 合并（源 Tag 硬删，见“主题标签生命周期”）。 |

### 进程启动状态重置（resetStaleStates）

服务启动时一次性清理上一进程残留的“进行中”状态（非定时，仅启动时跑）：

- `scheduler_tasks.status: running → idle`
- `feeds.refresh_status: refreshing → idle`
- `articles.firecrawl_status: processing → pending`
- `firecrawl_jobs.status: leased → pending`（清 leased_at / lease_expires_at）
- `tag_jobs.status: leased → pending`（清 leased_at / lease_expires_at）

### 队列状态流转与重试（重点）

> **注意**：两类队列的中间态命名不同，不要混淆。

**firecrawl_jobs / tag_jobs**（lease 租约模型，`internal/.../job_queue.go` + 各 repository）：

- 状态机：`pending → leased → completed / failed`
- Claim 时先回收过期租约（`lease_expires_at <= now`）→ pending；再把 `attempt_count >= max_attempts(默认5)` 的 pending 置为 failed；然后按 `priority DESC, available_at ASC, id ASC` 领取 → `leased` 并 `attempt_count++`。
- 失败重试：`MarkFailed` 以退避时间重置为 `pending`（`available_at = now+backoff`）；`attempt_count` 达到 `max_attempts` 后转 `failed`。**租约到期自动回收**即自动重试。
- **无任何行级清除**：completed/failed 行不被定时删除，无限累积。

**embedding_queues / merge_reembedding_queues**（SELECT FOR UPDATE SKIP LOCKED，`core/embedding_queue.go`）：

- 状态机：`pending → processing → completed / failed`
- worker `Start()` 时一次性把残留 `processing → pending`。失败时 `markFailed` 置 `failed`、`retry_count++`，**无自动重试**，仅靠手动 `RetryFailed()` API 重置 failed→pending。
- 行级清除：`embedding_queues` 的 `completed` 行由 `log_cleanup` 保留 30 天后删除（`merge_reembedding_queues` 及 failed 行仍无限累积）。

### 明确无自动清理的表（累积型）

以下表当前**没有任何基于时间或状态的定时清除**，行会一直增长，需要人工/运维介入：

- 队列表：`firecrawl_jobs` / `tag_jobs` 的 completed/failed 行；`merge_reembedding_queues` 全部行；`embedding_queues` 的 failed 行（completed 行 30 天后由 `log_cleanup` 清除）
- 日报：`board_daily_reports` / `daily_report_sections` / `daily_report_threads` / `daily_report_section_relations`（日报重生成仅删当日同 report 的旧分区，非 TTL 清理）
- 持久话题与观察：`board_persistent_topics`（仅一次性迁移裁剪 candidate、状态机自驱动 candidate→active→archived，无时间型删除）、`board_topic_watches`、`topic_watch_hits`
- 升级建议：`board_upgrade_suggestions`（仅 watch 软回收为 dismissed，不删行；confirmed/dismissed 行累积）
- 阅读与偏好：`reading_behaviors`（仅孤儿清理，无 TTL）、`user_preferences`
- 基础数据：`articles` / `topic_tags` / `topic_tag_embeddings` / `semantic_labels` / `ai_call_logs`（仅 tag 合并会硬删被合并源）

---

### 跨版块关系（add-evidence-backed-cross-board-relations）

- `cross_board_relation_runs`：**append-only 审计**，不清理不修改（可追溯要求；体量随发现频率线性、单用户场景可控）。
- `cross_board_relations`：生命周期行。`confirmed` 行到期由 `relation_expire` 定时任务（每小时）批量转 `expired`（读取路径也即时判过期，双保险）；`dismissed` 永久保留（同 `suggestion_hash` 冷却期默认 14 天内拦截重生，期满允许新 run 再提出）；`unresolved`/`proposed` 无 TTL，等用户裁决或 re-resolve。部分唯一索引保证 open 态幂等，终态行不删除。
- 证据 JSONB（`evidence`/`counterevidence`）与 run 留痕共同满足「每条关系可重建」的追溯要求。

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
6. `topic_tag_board_labels` 记录 tag → SemanticBoard，用于日报匹配输入

### 日报生成

1. 需要至少一个 active SemanticBoard（`semantic_labels.label_type='board'`）才有可生成的日报对象；冷启动无 SemanticBoard 时生成空结果且不报错
2. `daily_report` 调度器需启用（86400s 间隔）
3. 长期语义资产是 SemanticBoard，每日产物是 `board_daily_reports`（见日报生命周期）

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

### 2026-07-19

- 新增「数据清理与保留策略」节，以代码为真相补全全部定时清理/回收机制（log_cleanup / aux_label_cleanup / blocked_article_recovery / preference_update / board_upgrade_suggest watch GC）
- 纠正队列状态流转：firecrawl_jobs / tag_jobs 实为 `pending → leased → completed/failed`（lease 租约模型，非 processing）
- 纠正 embedding_queues / merge_reembedding_queues 为 `pending → processing → completed/failed`（无自动重试）
- 补进程启动状态重置（resetStaleStates）与「无自动清理的累积型表」清单
- 明确各队列 completed/failed 行无定时清除

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
- 从原 `DATABASE_FIELDS.md`（已按域切分为 `tables/` 域文档）迁移"工作流程"、"状态流转图"、"配置要求"三节内容

---

## 相关文档

- [业务流程](../flow/README.md) — 链路概要设计、函数调用链、前后端协作（"业务怎么跑的"）
- [数据库域文档索引](_index.md) — 52 张业务表按域分档的字段字典与 ER 图
- [全局约定](tables/_conventions.md) — FK 真相 / 索引与约束总览 / FK 引用矩阵
- [项目架构总览](../architecture/overview.md) — 系统架构全局视角
