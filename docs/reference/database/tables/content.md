# 内容域（`tables/content.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 1.1 articles（文章表）

存储 RSS 文章的核心数据。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `feed_id` | INTEGER | NOT NULL; index | 所属订阅源 ID（逻辑关联 `feeds.id`，无 DB FK） |
| `title` | VARCHAR(500) | NOT NULL | 文章标题 |
| `description` | TEXT | — | 文章摘要。**入库即纯文本化**（fix-reader-lede-blank-space）：RSS 条目的 `description` 可能是整篇文章的 HTML（智源社区均值 66 KB、最大 864 KB），入库时经 `htmltext.PlainSummary(raw, 300)` 收敛为 ≤300 字符纯文本（去 script/style → 剔隐藏子树 → 去标签 → 还原实体 → 折叠空白 → 按 rune 截断加省略号）；清洗在入库层 `feed_service.buildArticleFromEntry`，解析层保持源站原文。存量可一次性回填：`go run ./cmd/clean-article-descriptions`（默认 dry-run，`--apply` 才写库，只改本列）。参与全文检索，权重 B；trgm 索引 `idx_articles_description_trgm` 覆盖本列 |
| `content` | TEXT | — | RSS 原始内容（HTML 片段）。**保留源站原文、不被摘要清洗改写**（fix-reader-lede-blank-space）：正文源切换、Firecrawl 兜底与整理稿降级都依赖它；阅读页原文源渲染前在前端做保守清洗（`utils/articleContentHtml.ts`） |
| `link` | VARCHAR(1000) | index `idx_articles_link`；部分唯一索引 `uq_articles_feed_link`（迁移 `20260917_0001`） | 文章链接。**入库判重键 = `(feed_id, link)`**（标题不参与判重），唯一部分索引 `WHERE link <> ''` 兜底并发刷新（空 link 行不受约束）；跨 feed 复用打标与存量归并都依赖 `idx_articles_link` |
| `image_url` | VARCHAR(1000) | — | 封面图 |
| `pub_date` | TIMESTAMP | — | 发布时间 |
| `author` | VARCHAR(200) | — | 作者 |
| `read` | BOOLEAN | DEFAULT false | 是否已读（索引 `idx_articles_read`） |
| `favorite` | BOOLEAN | DEFAULT false | 是否收藏（索引 `idx_articles_favorite`）；归档免死标记 |
| `archived` | BOOLEAN | DEFAULT false（迁移 `20260818_0001`） | 归档标记：超出 feed `max_articles` 活跃窗口的超限文章置 true。归档行**保留全部文本字段**（日报线索按 ID 反查），但 topic tags 边 / reading_behaviors 已删；reader 列表与统计默认过滤归档行，按 ID 详情豁免，站内搜索默认含入（见 `flow/reading.md` §业务约束 6/10） |
| `summary_status` | VARCHAR(20) | DEFAULT 'complete' | AI 总结状态：`incomplete` / `pending` / `complete` / `failed` |
| `summary_generated_at` | TIMESTAMP | — | AI 总结生成时间 |
| `summary_processing_started_at` | TIMESTAMP | — | AI 总结开始处理时间 |
| `completion_attempts` | INTEGER | DEFAULT 0 | AI 总结重试次数 |
| `completion_error` | TEXT | — | AI 总结错误信息 |
| `ai_content_summary` | TEXT | — | AI 生成的优化总结内容（Markdown） |
| `content_form` | VARCHAR(20) | —（可空，AutoMigrate 自动加列） | 内容形态标记：`mono`（单主题）/ `aggregate`（聚合型合集）/ 空（存量文章或模型未输出标记）。由摘要链路解析首行 HTML 注释产出，下游打标按此分流（见 `flow/reading.md` §业务约束 3） |
| `firecrawl_status` | VARCHAR(20) | DEFAULT 'pending' | Firecrawl 抓取状态：`pending` / `processing` / `completed` / `failed` |
| `firecrawl_error` | TEXT | — | Firecrawl 抓取错误信息 |
| `firecrawl_content` | TEXT | — | Firecrawl 抓取的完整网页内容（Markdown） |
| `firecrawl_crawled_at` | TIMESTAMP | — | Firecrawl 抓取时间 |
| `created_at` | TIMESTAMP | — | 创建时间 |

**全文检索（tsvector 路径已彻底退场，2026-10-13）**：`search_vector` 列（tsvector）与触发器/旧 GIN 索引自迁移 `20260417_0002` 引入，但业务零引用且 `simple` 分词器对中文失效（实测漏 97%）——索引与触发器先于 2026-08-20 删除，列随迁移 `20261013_0002` DROP（add-article-search；列数据可由 title/description 重算，可再生；归档 cleanup 置 NULL 的行为已同步移除）。

**虚拟字段（`gorm:"->"` 计算列，非持久化）**：`tag_count`（文章标签数）、`relevance_score`（相关度评分）。

> ⚠️ 旧文档曾列 `feed_summary_id` / `feed_summary_generated_at`：**代码 `Article` struct 无此字段**，已删除，请勿引用。

**复合索引（迁移 `20260417_0001`）**：`idx_articles_feed_pub_date(feed_id, pub_date DESC)`、`idx_articles_feed_id_title(feed_id, title)`。

**去重相关索引（迁移 `20260917_0001`）**：`idx_articles_link(link)`（普通索引，服务跨 feed 打标复用查询与存量归并）+ `uq_articles_feed_link(feed_id, link) WHERE link <> ''`（部分唯一索引，同 feed 同 link 只能有一行——并发刷新双方的 `Create` 冲突被刷新路径 `continue` 吞掉，空 link 行豁免）。

**搜索 trgm 索引（迁移 `20261013_0001`，add-article-search）**：`idx_articles_title_trgm`（`title` 列 `gin_trgm_ops`）+ `idx_articles_description_trgm`（`description` 列 `gin_trgm_ops`），依赖 `CREATE EXTENSION pg_trgm`。服务 `GET /api/search` 与 `GET /api/articles?search=` 的中文子串/trigram 匹配（裸 LIKE 全表扫实测 ~3s，走索引后毫秒级）；估算体积 100~200MB，文章写入频率低（日增百级），GIN 维护开销可忽略。

### 1.2 feeds（订阅源表）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `title` | VARCHAR(200) | NOT NULL | 订阅源标题 |
| `description` | TEXT | — | 描述 |
| `url` | VARCHAR(500) | UNIQUE NOT NULL | RSS URL |
| `category_id` | INTEGER | index（`idx_feeds_category_id` 迁移补） | 所属分类 ID（逻辑关联 `categories.id`） |
| `icon` | VARCHAR(1000) | DEFAULT 'rss' | 图标值（iconify id 或图片 URL） |
| `icon_source` | VARCHAR(20) | DEFAULT 'fallback' | 图标来源状态机：`auto` / `custom` / `fallback` |
| `color` | VARCHAR(20) | DEFAULT '#8b5cf6' | 颜色 |
| `last_updated` | TIMESTAMP | — | 最后更新时间 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `max_articles` | INTEGER | DEFAULT 100 | 最大文章数 |
| `refresh_interval` | INTEGER | DEFAULT 60 | 刷新间隔（秒） |
| `refresh_status` | VARCHAR(20) | DEFAULT 'idle' | 刷新状态 |
| `refresh_error` | TEXT | — | 刷新错误信息 |
| `last_refresh_at` | TIMESTAMP | — | 最后刷新时间 |
| `article_summary_enabled` | BOOLEAN | DEFAULT false | AI 总结主开关（能力层：关→无任何总结含手动入口；开→文章页可手动生成） |
| `completion_on_refresh` | BOOLEAN | DEFAULT false | 刷新后自动总结闸门（自动层：仅控制刷新后是否自动排队总结；存量已由迁移 20260920_0002 全置 false） |
| `max_completion_retries` | INTEGER | DEFAULT 3 | AI 总结最大重试次数 |
| `firecrawl_enabled` | BOOLEAN | DEFAULT false | 是否启用 Firecrawl 抓取 |
| `tagging_enabled` | BOOLEAN | DEFAULT true | 是否启用自动打标签 |

> ⚠️ 旧文档曾列 `ai_summary_enabled`（DEFAULT true）：**代码无此字段**。实际是 `article_summary_enabled`（DEFAULT false），两者曾被混淆，请勿再引用 `ai_summary_enabled`。

`Feed.Articles` 声明了 `constraint:OnDelete:CASCADE`（GORM 层）；`Feed.Category` 为逻辑关联。

### 1.3 categories（分类表）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `name` | VARCHAR(100) | UNIQUE NOT NULL | 分类名称 |
| `slug` | VARCHAR(50) | UNIQUE | URL 友好标识 |
| `icon` | VARCHAR(50) | DEFAULT 'folder' | 图标 |
| `color` | VARCHAR(20) | DEFAULT '#6366f1' | 颜色 |
| `description` | TEXT | — | 描述 |
| `created_at` | TIMESTAMP | — | 创建时间 |

`Category.Feeds` 声明了 `constraint:OnDelete:CASCADE`（GORM 层）。

---


## 字段用途说明：文章三个内容字段

| 字段 | 来源 | 格式 | 特点 | 用途 |
| -------- | ------ | ------ | ------ | ------ |
| `content` | RSS Feed 解析 | HTML 片段 | 可能不完整，含 HTML 标签 | 基础内容展示 |
| `firecrawl_content` | Firecrawl 抓取 | Markdown | 完整网页内容，过滤广告/导航栏 | AI 总结输入源，不对用户直接展示 |
| `ai_content_summary` | AI 生成 | Markdown | 保留核心，移除冗余 | 前端默认展示内容 |

---


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 13 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

```mermaid
erDiagram
    articles ||--o{ article_topic_tags : "article_id"
    articles ||--o{ firecrawl_jobs : "article_id"
    articles ||--o{ reading_behaviors : "article_id"
    articles ||--o{ tag_jobs : "article_id"
    categories ||--o{ ai_summaries : "category_id (已废弃)"
    categories ||--o{ feeds : "category_id"
    categories ||--o{ user_preferences : "category_id (已废弃)"
    feeds ||--o{ ai_summaries : "feed_id (已废弃)"
    feeds ||--o{ articles : "feed_id"
    feeds ||--o{ reading_behaviors : "feed_id"
    feeds ||--o{ user_preferences : "feed_id (已废弃)"

    ai_summaries {
    }
    article_topic_tags {
    }
    articles {
        SERIAL id PK
        INTEGER feed_id FK
    }
    categories {
        SERIAL id PK
    }
    feeds {
        SERIAL id PK
        INTEGER category_id FK
    }
    firecrawl_jobs {
    }
    reading_behaviors {
    }
    tag_jobs {
    }
    user_preferences {
    }
```

