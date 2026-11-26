<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading, topic-graph -->

## Why

站内文章搜索事实上缺失：后端 `articles?search=` 的 tsvector 全文路径因 `simple` 分词器对中文失效（实测同一关键词 tsvector 命中 2 篇 vs LIKE 命中 67 篇，漏召回 97%），前端无任何搜索入口（stores 内存过滤只覆盖当前已加载页，是"假搜索"）。用户需要"找回那篇文章"（关键词）与"发现相关文章"（语义）两种能力；而 pgvector 与 embedding 管线（airouter + topic_tag_embeddings 42K 条向量）已就位却从未用于文章搜索。

关键事实（探索阶段实测）：
- 38,389 篇文章（94.6% 已归档），归档文章 title/description/content 全保留（`article-archive-instead-of-delete` 设计使然：日报引用完整性 + RSS 去重防复活）
- 裸 LIKE 全表扫实测 ~3 秒（树莓派），不可用；pg_trgm 未安装，全库唯一 tsvector 列就是 `articles.search_vector`
- 标签边（article_topic_tags）仅覆盖 7,011 篇（18%），且归档文章的边只保留 7 天（`tag_edge_retention_days` GC）→ 语义路径天然只覆盖"活跃 + 近 7 天归档"，更老的归档文章靠关键词路径

## What Changes

- **新 `/search` 页面**：侧边栏"发现订阅源"下方新增入口；搜索框 + 结果列表（归档文章带角标）+ 过滤器（仅活跃/含归档）+ 命中原因展示（"命中标题关键词"/"命中标签·XX·0.83"）
- **新后端聚合端点 `GET /api/search`**：三路召回 + RRF 融合排序：
  1. 关键词路径（主力）：pg_trgm GIN 索引加速 LIKE，覆盖 title+description，覆盖 100% 文章（含归档）
  2. 语义路径（增强）：query embedding → cosine 对 `topic_tag_embeddings`（identity 型）→ 阈值 cosine≥0.55~0.60 ∧ top 8~10 命中标签 → 经 `article_topic_tags` 召回文章（取 max cosine 为语义分）；覆盖活跃 + 近 7 天归档
  3. LLM 查询扩展（可选增强，**默认关闭**，用户手动开启）：LLM 将 query 扩为 5-8 个关键词，再跑一轮关键词路径，补无标签老文章的字面召回
- **DB 变更**：安装 pg_trgm 扩展 + `articles.title`/`articles.description` 两列 GIN trgm 索引（估算 ~100-200MB，SD 卡余量 60G 充裕）
- **修复既有端点**：`GET /api/articles?search=` 的 tsquery 路径（中文失效）统一换成 trgm 路径，消灭最后一个中文分词坑点
- **降级链**：关键词路径纯 SQL 永远在线；语义路径（embedding 不可用）与 LLM 路径静默跳过并在响应中标记，前端提示不报错

## Capabilities

### New Capabilities

- `article-search`: 站内文章混合搜索能力——三路召回（trgm 关键词/标签向量语义/LLM 查询扩展）、RRF 融合排序、命中解释、归档默认含入、降级链、搜索页 UI 与侧边栏入口、articles 列表端点 search 参数的 trgm 统一

### Modified Capabilities

（无——`article-list-projection` 仅约束字段投影契约，search 匹配行为的替换不触及其 requirement；`article-retention` 行为零改动，7 天边 GC 维持现状）

## Impact

- **后端**：新 search 域服务与 handler（`internal/reader` 或新包）；`internal/platform/database/postgres_migrations.go` 新增 pg_trgm 扩展与索引迁移；`internal/platform/airouter` embedding 消费（只读复用，含缓存）；`article_handler.go` search 过滤路径替换
- **前端**：`front/app/pages/search.vue` 新页面；`AppSidebarView.vue` 新入口；搜索结果卡片复用现有文章列表组件风格
- **数据/DDL**：`CREATE EXTENSION pg_trgm` + 2 个 GIN 索引（一次性迁移，入库 DDL 走 db-migration-execution 红线）
- **外部依赖**：查询时 1 次 embedding API 调用（airouter 缓存命中则免）；LLM 扩展仅手动开启时 1 次调用
- **不受影响**：板块体系/标签系统（只读消费 `topic_tag_embeddings` 与 `article_topic_tags`，零写入）；retention 体系（归档逻辑与 7 天边 GC 原样）

## 非目标与未来演进

- **影子边表**（`article_tag_history`：归档时复制标签边、只增不 GC，让老归档文章保留"曾经标过什么"的语义召回能力）——一期不做；若上线后老文章召回不足，此为现成升级路径，不动现有 retention 语义
- `content` 列 trgm 索引与"深度搜索"开关（content 平均 4KB，全列索引 ~400-800MB 写放大，收益待验证）
- `semantic` embedding_type 双查并集、加权精排（RRF 体验不足时升级）
- 搜索历史存储
