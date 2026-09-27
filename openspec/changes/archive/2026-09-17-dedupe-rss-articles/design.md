# Design — RSS 文章去重治理

## Context

见 proposal.md - Why。现状：`RefreshFeed`（`backend-go/internal/reader/service/feed_service.go` L67-107）以 `(feed_id, title)` 内存快照判重，不查 link、无跨 feed 归并、`articles` 表无唯一约束兜底。已确认数据：409 个重复 link 组（同 feed 标题变 243 / 跨 feed 123 / 同 feed 同标题竞态 41 / 混合 3），其中 89 组内 active 与 archived 混存（`CleanupOldArticles` 归档产物）。

## Goals / Non-Goals

**Goals**
- 新增重复归零：(feed_id, link) 判重 + 数据库唯一索引双保险
- 快讯滚动更新收敛为单条 upsert，实质变化才重走处理链
- 跨 feed 副本打标零 AI 重复调用
- 存量 409 组一次性归并，关联数据（标签/阅读行为/jobs）无损

**Non-Goals**
- 不做全局 link 归并存储（跨 feed 各留一份，feed 视图语义不变）
- 不做打标记录视图的跨 feed 聚合展示（UI 无改动）
- 不新增 RSS content_hash 持久列（见 D2）

## Decisions

### D1: 判重快照 link 化，不加持久 hash 列

`titleSet`（Pluck title）改为 `linkSet`（Pluck link），循环内 `linkSet[entry.Link]` 判重；命中后加载已存条目做后续判断。同 feed 内文章量级不变，Pluck 成本等价。

**替代方案（否决）**：新增 `rss_content_hash` 列——差异检测只需对比 `title` 与 `description` 两个字符串（RSS 原文，处理链不会改写这两个字段：firecrawl 写 `firecrawl_content`、AI 摘要写 `ai_content_summary`，均独立列），现算对比即够，加列徒增迁移与维护成本。

### D2: 更新语义复用 buildArticleFromEntry 状态机 + 事件驱动重打标

link 命中后：

- **title 与 description 均未变** → 跳过（不动内容、不触发任何处理链）。
- **任一变化** → 单条 UPDATE：内容字段（title/description/content/image_url/author/pub_date）+ 状态字段按 `buildArticleFromEntry` 同款规则重置（firecrawl 开 → `firecrawl_status=pending`；摘要开 → `summary_status=pending/incomplete`；都关 → complete）+ 清空衍生字段（`firecrawl_content`/`firecrawl_error`/`ai_content_summary`/`completion_*`）+ 删除旧 `article_topic_tags`（复用 `RetagArticle` 的清理模式，含 `CleanupOrphanedTags`）→ 调 `enqueueArticleProcessing` 重走链。

重打标不单独触发：链条完成事件（`firecrawl_completed`/`summary_completed`/`article_created`，取决于 feed 开关）按既有 tag_jobs 路径接力，此时旧标签已删、`existingCount=0`，正常走 AI 打标。**效果：快讯未实质变化时零 AI 消耗，实质变化时全链刷新一次。**

**替代方案（否决）**：更新时直接同步调 AI 重打——绕过抓取/摘要链，标签基于旧 summary，且快讯高频更新会烧调用。

### D3: 跨 feed 复用插在 tagArticle 入口，source='reuse'

`tagArticle`（非 Force）在 **`existingCount` 检查之前**、extractor 调用之前：

1. 查同 link 其它副本的打标（空 link 直接跳过）：`article_topic_tags JOIN articles ON ... WHERE articles.link = ? AND article_id != ?`，按 `score DESC` 排序（依赖新增 `idx_articles_link` 普通索引）
2. 命中且有可补空间 → 按分数从高到低把缺失的标签复制到本副本（跳过本副本已存在的 `(article_id, topic_tag_id)`，`idx_article_topic_tags_link` 唯一索引兜底），`source='reuse'`、score 沿用，**总数不超过 `maxArticleTags`**；补入即返回，**不调 AI**
3. 本副本已满 `maxArticleTags` → 返回未复用 → 落到既有 `existingCount` 跳过（不打标、不调 AI）
4. 无同 link 副本标签 → 走原 AI 提取路径

**为什么前置而非 existingCount 之后**：design 初稿把复用放在 `existingCount` 检查之后，同时要求“跳过本副本已存在的 `(article_id, topic_tag_id)`”——两者自相矛盾（跳过后本副本必无标签，不可能冲突）。前置后语义自洽且更幂等：**同一篇文章的各副本标签向同 link 的完整集合收敛**（部分标签副本被补齐），同时保留原有的“本副本已有标签且兄弟无可复用”时跳过 AI 的保护。

选 `tagArticle` 而非入队前判断：打标有多个触发入口（article_created / firecrawl_completed / summary_completed / manual），收敛在执行点覆盖最全。`RetagArticle`（Force）明确不复用，也不受 `maxArticleTags` 补空间限制（走完整 AI 提取 + `limitArticleTags`）。

`topic_tags.feed_count` 影响小（reuse 不走 findOrCreate 少累计），已有周期对账任务兜底（tagging-domain spec），不特殊处理。

### D4: 存量归并为单个版本化迁移，单事务含唯一索引

版本 `20260917_0001`（挂 `postgres_migrations.go` 既有 `Version` 注册模式，事务内执行）：

1. `CREATE INDEX idx_articles_link`（普通索引，先建加速归并查询，兼供 D3 运行期使用）
2. 找重复组：`GROUP BY feed_id, link HAVING COUNT(*)>1 AND link != ''`
3. 每组保留条打分（高者优先，并列取 id 最小）：
   `!archived × 1000`（89 个混合组必须 active 优先，否则归并后文章凭空离开活跃窗口）`+ tag_count>0 × 100 + firecrawl_content 非空 × 10 + ai_content_summary 非空 × 10 + (read|favorite) × 1`
4. 副本数据处理（顺序执行）：
   - `article_topic_tags`：保留条无同 topic_tag_id 则 UPDATE article_id 改指向，有则删除副本行
   - `reading_behaviors`：保留条已有行为记录则删副本行，否则改指向
   - `tag_jobs`/`firecrawl_jobs`：completed 删除；pending/processing 改 article_id 指向保留条
   - 删除副本 articles 行
5. 保留条 `tag_count` 重算（与实际标签数对齐）
6. `CREATE UNIQUE INDEX uq_articles_feed_link ON articles(feed_id, link) WHERE link != ''`——部分索引防历史空 link 行；同事务保证"清理成功 ⟺ 约束生效"原子性

幂等：迁移版本表跳过已应用版本（既有机制）；逻辑本身无重复组时 no-op。12 万行建索引的事务内锁表时长单用户产品可接受。

**替代方案（否决）**：管理命令手动清理（dry-run 预览）——多一套代码路径与一次人工操作；打分规则已静态可验证，且迁移失败自动整体回滚。

### D5: 并发兜底零代码

唯一索引生效后，同 feed 并发刷新双插撞索引 → `Create` 返回错误 → 现有 `if err != nil { continue }` 语义正好吞掉冲突且不中断刷新，无需额外处理。

## Risks / Trade-offs

- [快讯高频实质更新重走 firecrawl 链，烧抓取配额] → 仅 title+description 双变触发；单 feed 可关 FirecrawlEnabled 降级为纯打标链
- [迁移删除数据不可逆] → 保留条打分保证完整度 ≥ 副本；单事务失败全回滚；上线部署前建议 pg_dump 一次
- [跨 feed 复制的 score 语义] → 沿用原副本分数，source='reuse' 可审计区分
- [归并后 feed 活跃文章数下降] → 预期效果（重复挤占窗口），`CleanupOldArticles` 窗口不再被重复侵蚀
- [reuse 副本标签与 AI 直打可能风格不一致] → 同一篇文章标签本就应一致，复制反而保证一致性

## Migration Plan

1. 合并部署：启动时 `RunMigrations` 自动执行归并迁移（先于 feed 刷新调度）
2. 部署后首个刷新周期：新逻辑生效，观察 `ai_call_logs` 中 extractor 调用量下降、无新增重复组
3. 验证 SQL：重复组计数归零（跨 feed 组保留，同 feed 组归零）
4. 回滚：代码可回滚（迁移版本已记录，旧代码无新列依赖）；数据不自动还原，依赖部署前备份
