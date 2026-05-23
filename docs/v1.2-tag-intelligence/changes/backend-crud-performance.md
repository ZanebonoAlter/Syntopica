# Backend CRUD 性能优化实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复后端 CRUD 接口在大数据量下的性能瓶颈——N+1 查询、全表扫描、缺少索引、逐条操作。

**Architecture:** 每个 Task 独立可提交，按优先级排序。P0 先行（feeds N+1、articles tag_stats、索引），P1 跟进（stats 合并、批量查重、cleanup 批量化），P2 收尾（全文搜索、安全加固）。所有改动兼容 PostgreSQL，测试使用 SQLite 内存库。

**Tech Stack:** Go, Gin, GORM, PostgreSQL (pgvector), SQLite (测试)

**前置知识：**
- 数据库连接通过 `database.DB` 全局变量访问
- 测试模式：SQLite 内存库 `gorm.Open(sqlite.Open(...))` + `database.DB = db`
- Handler 直接操作 `database.DB`，没有 service 层（feeds 除外）
- Model 的 `ToDict()` 方法做 JSON 序列化
- 迁移文件：`postgres_migrations.go` + `bootstrap_postgres.go`
- 前端 API 约定：返回 `{ success, data, pagination }` 格式

---

## Task 1: 添加缺失的数据库索引 (P0)

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Test: `go test ./internal/platform/database/... -v`（验证迁移不报错）

**问题背景：**
以下查询缺少索引，数据量大时走全表扫描：
- `articles` 表：按 `read`、`favorite` 过滤（`GetArticles`、`GetArticlesStats`）
- `articles` 表：按 `feed_id + pub_date` 排序（`cleanupOldArticles`）
- `article_topic_tags` 表：按 `article_id` 聚合（`tag_stats` 子查询）
- `feeds` 表：按 `category_id` 过滤（`GetFeeds`）

**Step 1: 添加迁移函数**

在 `postgres_migrations.go` 的 `postgresMigrations()` 返回数组末尾追加一个新的 migration：

```go
{
    Version:     "20260417_0001",
    Description: "Add missing indexes for CRUD performance optimization.",
    Up: func(db *gorm.DB) error {
        stmts := []string{
            "CREATE INDEX IF NOT EXISTS idx_articles_read ON articles(read)",
            "CREATE INDEX IF NOT EXISTS idx_articles_favorite ON articles(favorite)",
            "CREATE INDEX IF NOT EXISTS idx_articles_feed_pub_date ON articles(feed_id, pub_date DESC)",
            "CREATE INDEX IF NOT EXISTS idx_article_topic_tags_article_id ON article_topic_tags(article_id)",
            "CREATE INDEX IF NOT EXISTS idx_feeds_category_id ON feeds(category_id)",
            "CREATE INDEX IF NOT EXISTS idx_articles_feed_id_title ON articles(feed_id, title)",
        }
        for _, s := range stmts {
            if err := db.Exec(s).Error; err != nil {
                return fmt.Errorf("create index: %w", err)
            }
        }
        return nil
    },
},
```

**索引用途说明：**

| 索引 | 服务查询 |
|------|----------|
| `idx_articles_read` | `GetArticles WHERE read = ?`, `GetArticlesStats WHERE read = false` |
| `idx_articles_favorite` | `GetArticles WHERE favorite = ?`, `GetArticlesStats WHERE favorite = true` |
| `idx_articles_feed_pub_date` | `cleanupOldArticles ORDER BY pub_date DESC`, `GetArticles ORDER BY pub_date DESC` |
| `idx_article_topic_tags_article_id` | `tag_stats` 子查询 `GROUP BY article_id` |
| `idx_feeds_category_id` | `GetFeeds WHERE category_id = ?` |
| `idx_articles_feed_id_title` | `RefreshFeed WHERE feed_id = ? AND title = ?` 去重查询 |

**Step 2: 验证迁移语法**

Run: `cd backend-go && go build ./...`
Expected: 编译成功，无错误

**Step 3: 提交**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go
git commit -m "perf: add missing database indexes for CRUD optimization"
```

---

## Task 2: 修复 GetFeeds N+1 查询 — 用聚合替代 Preload (P0)

**Files:**
- Modify: `backend-go/internal/domain/feeds/handler.go:48-126`（`GetFeeds` 函数）
- Modify: `backend-go/internal/domain/feeds/handler.go:128-160`（`GetFeed` 函数）
- Modify: `backend-go/internal/domain/models/feed.go:35-71`（`ToDict` 方法）
- Test: `cd backend-go && go build ./...`（编译验证 ToDict 签名变更的所有调用点已更新）

**问题背景：**
当前 `GetFeeds` 对每个 feed 执行 `Preload("Articles")`，把**全部文章**加载到内存只为统计 `article_count` 和 `unread_count`。如果有 50 个 feed，每个 feed 1000 篇文章，会执行 50 次 `SELECT * FROM articles WHERE feed_id = ?`，加载 50000 行到内存。

**Step 1: 修改 Feed.ToDict — 支持直接传入统计数据**

修改 `backend-go/internal/domain/models/feed.go`，新增统计结构体，修改 `ToDict`：

```go
type FeedStats struct {
    ArticleCount int
    UnreadCount  int
}

func (f *Feed) ToDict(stats *FeedStats) map[string]interface{} {
    data := map[string]interface{}{
        "id":                      f.ID,
        "title":                   f.Title,
        "description":             f.Description,
        "url":                     f.URL,
        "category_id":             f.CategoryID,
        "icon":                    f.Icon,
        "color":                   f.Color,
        "last_updated":            FormatDatetimeCSTPtr(f.LastUpdated),
        "created_at":              FormatDatetimeCST(f.CreatedAt),
        "max_articles":            f.MaxArticles,
        "refresh_interval":        f.RefreshInterval,
        "refresh_status":          f.RefreshStatus,
        "refresh_error":           f.RefreshError,
        "last_refresh_at":         FormatDatetimeCSTPtr(f.LastRefreshAt),
        "ai_summary_enabled":      f.AISummaryEnabled,
        "article_summary_enabled": f.ArticleSummaryEnabled,
        "completion_on_refresh":   f.CompletionOnRefresh,
        "max_completion_retries":  f.MaxCompletionRetries,
        "firecrawl_enabled":       f.FirecrawlEnabled,
    }

    if stats != nil {
        data["article_count"] = stats.ArticleCount
        data["unread_count"] = stats.UnreadCount
    }

    return data
}
```

**Step 2: 修改 GetFeeds — 用聚合查询替代 N+1 Preload**

替换 `backend-go/internal/domain/feeds/handler.go` 中 `GetFeeds` 函数体内 `var feeds []models.Feed` 之后的全部逻辑（从分页逻辑开始）：

```go
func GetFeeds(c *gin.Context) {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
    categoryID, _ := strconv.Atoi(c.Query("category_id"))
    uncategorized := c.Query("uncategorized") == "true"

    query := database.DB.Model(&models.Feed{})

    if categoryID > 0 {
        query = query.Where("category_id = ?", categoryID)
    }

    if uncategorized {
        query = query.Where("category_id IS NULL")
    }

    var total int64
    query.Count(&total)

    var feeds []models.Feed
    if err := query.Order("title ASC").Find(&feeds).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   err.Error(),
        })
        return
    }

    // 批量查询统计数据 — 替代 N+1 Preload
    feedIDs := make([]uint, len(feeds))
    for i, f := range feeds {
        feedIDs[i] = f.ID
    }

    type FeedStatRow struct {
        FeedID       uint
        ArticleCount int
        UnreadCount  int
    }
    var statRows []FeedStatRow
    if len(feedIDs) > 0 {
        database.DB.Model(&models.Article{}).
            Select("feed_id, COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
            Where("feed_id IN ?", feedIDs).
            Group("feed_id").
            Scan(&statRows)
    }

    statMap := make(map[uint]models.FeedStats, len(statRows))
    for _, row := range statRows {
        statMap[row.FeedID] = models.FeedStats{
            ArticleCount: row.ArticleCount,
            UnreadCount:  row.UnreadCount,
        }
    }

    // perPage >= 10000 时返回全量（无分页）；否则分页
    data := make([]map[string]interface{}, 0, len(feeds))
    start := 0
    if perPage < 10000 {
        start = (page - 1) * perPage
        if start >= len(feeds) {
            start = len(feeds)
        }
    }
    end := len(feeds)
    if perPage < 10000 {
        end = start + perPage
        if end > len(feeds) {
            end = len(feeds)
        }
    }

    for i := start; i < end; i++ {
        stats := statMap[feeds[i].ID] // zero value = {0, 0}
        data = append(data, feeds[i].ToDict(&stats))
    }

    resultPage := page
    resultPerPage := perPage
    if perPage >= 10000 {
        resultPage = 1
        resultPerPage = len(feeds)
    }

    pages := int(total) / resultPerPage
    if int(total)%resultPerPage > 0 {
        pages++
    }
    if perPage >= 10000 {
        pages = 1
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    data,
        "pagination": gin.H{
            "page":     resultPage,
            "per_page": resultPerPage,
            "total":    total,
            "pages":    pages,
        },
    })
}
```

**关键改动说明：**
- 去掉所有 `Preload("Articles")` 调用
- 用一条聚合 SQL 替代 N 次查询
- 分页改为先查全量 feeds（feeds 数量少，通常 < 100），在 Go 层做切片分页
- stats 查询始终只执行一次

**Step 3: 修改 GetFeed — 同样去掉 Preload**

替换 `GetFeed` 函数（`handler.go:128-160`）：

```go
func GetFeed(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("feed_id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Invalid feed ID",
        })
        return
    }

    var feed models.Feed
    if err := database.DB.First(&feed, uint(id)).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            c.JSON(http.StatusNotFound, gin.H{
                "success": false,
                "error":   "Feed not found",
            })
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false,
                "error":   err.Error(),
            })
        }
        return
    }

    var stats models.FeedStats
    database.DB.Model(&models.Article{}).
        Select("COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
        Where("feed_id = ?", feed.ID).
        Group("feed_id").
        Scan(&stats)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    feed.ToDict(&stats),
    })
}
```

**Step 4: 修改 CreateFeed 和 UpdateFeed — 去掉 Preload**

`CreateFeed`（handler.go:222-223）和 `UpdateFeed`（handler.go:337）中的 `Preload("Articles").First(...)` 都要替换为聚合查询。模式与 GetFeed 一致：

```go
// 替换 database.DB.Preload("Articles").First(&feed, feed.ID)
var stats models.FeedStats
database.DB.Model(&models.Article{}).
    Select("COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
    Where("feed_id = ?", feed.ID).
    Group("feed_id").
    Scan(&stats)
// 然后 feed.ToDict(&stats)
```

**Step 5: 更新所有 ToDict 调用点（关键 — 无自动化测试覆盖）**

旧签名 `ToDict(includeStats bool)` → 新签名 `ToDict(stats *FeedStats)`。

必须更新的 5 个调用点（全部在 `feeds/handler.go`）：
- `handler.go:80` — GetFeeds 全量分支 → 传入 `statMap[feed.ID]`
- `handler.go:108` — GetFeeds 分页分支 → 传入 `statMap[feed.ID]`
- `handler.go:154/158` — GetFeed → 传入聚合查询结果
- `handler.go:223/227` — CreateFeed → 传入零值 `&models.FeedStats{}`
- `handler.go:337/341` — UpdateFeed → 传入零值 `&models.FeedStats{}`

**注意：** 由于没有 handler 层自动化测试，遗漏任何调用点只会被 `go build` 编译错误捕获。编译通过即保证所有调用点已更新。

**Step 6: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 7: 提交**

```bash
git add backend-go/internal/domain/feeds/handler.go backend-go/internal/domain/models/feed.go
# 加上其他引用了 ToDict 的文件
git commit -m "perf: replace GetFeeds/GetFeed N+1 Preload with aggregate query"
```

---

## Task 3: 优化 GetArticles tag_stats 子查询 (P0)

**Files:**
- Modify: `backend-go/internal/domain/articles/handler.go:16-27`（`loadArticleWithTagCount`）
- Modify: `backend-go/internal/domain/articles/handler.go:60-225`（`GetArticles`）
- Test: `cd backend-go && go test ./internal/domain/articles/... -v`

**问题背景：**
`GetArticles` 的 `tag_stats` 子查询对 `article_topic_tags` 做全表 `GROUP BY`：
```sql
LEFT JOIN (SELECT article_id, COUNT(*) AS tag_count FROM article_topic_tags GROUP BY article_id) tag_stats
```
数据量大时（如 10 万条 tag 记录）每次列表查询都要全表扫描 + 排序 + 聚合。Task 1 添加的 `idx_article_topic_tags_article_id` 索引已经改善了 GROUP BY 性能，但子查询仍然是全表。

**优化策略：** 将 tag_stats 改为关联子查询（correlated subquery），只计算当前结果集中匹配的 article 的 tag_count，避免全表扫描。

**Step 1: 修改 GetArticles 中的 base query 构建**

替换 `handler.go:97-99` 的 query 构建：

```go
// 旧代码（全表子查询）：
// query := database.DB.Model(&models.Article{}).
//     Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
//     Joins("LEFT JOIN (SELECT article_id, COUNT(*) AS tag_count FROM article_topic_tags GROUP BY article_id) tag_stats ON tag_stats.article_id = articles.id")

// 新代码（关联子查询）：
query := database.DB.Model(&models.Article{}).
    Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id")
```

然后修改 Select 字段，把 `tag_stats` 的 JOIN 改为关联子查询：

**Step 2: 修改 Select 构建逻辑**

替换 `handler.go:107-118` 的 Select 分支：

```go
if usingWatchedTags && sortBy == "relevance" {
    query = query.
        Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count, (SELECT COALESCE(SUM(CASE WHEN att2.topic_tag_id IN ? THEN 2.0 ELSE 1.0 END), 0) FROM article_topic_tags att2 WHERE att2.article_id = articles.id AND att2.topic_tag_id IN ?) AS relevance_score", childTagIDs, expandedTagIDs).
        Group("articles.id")
} else if usingWatchedTags {
    query = query.
        Select("DISTINCT articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count")
} else {
    query = query.
        Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count")
}
```

**Step 3: 修改 loadArticleWithTagCount**

替换 `handler.go:16-27`：

```go
func loadArticleWithTagCount(articleID uint) (*models.Article, error) {
    var article models.Article
    if err := database.DB.Model(&models.Article{}).
        Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
        Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count").
        First(&article, articleID).Error; err != nil {
        return nil, err
    }

    return &article, nil
}
```

**Step 4: 修改 UpdateArticle 中的重载查询**

替换 `handler.go:414-418`：

```go
database.DB.Model(&models.Article{}).
    Joins("LEFT JOIN feeds ON articles.feed_id = feeds.id").
    Select("articles.*, feeds.category_id AS category_id, (SELECT COUNT(*) FROM article_topic_tags att_cnt WHERE att_cnt.article_id = articles.id) AS tag_count").
    First(&article, uint(id))
```

**Step 5: 运行现有测试验证**

Run: `cd backend-go && go test ./internal/domain/articles/... -v`
Expected: `TestGetArticlesReturnsTagCount`、`TestGetArticleReturnsArticleTags` 等测试通过

**Step 6: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 7: 提交**

```bash
git add backend-go/internal/domain/articles/handler.go
git commit -m "perf: replace tag_stats full-table subquery with correlated subquery"
```

---

## Task 4: 合并 GetArticlesStats 三次 COUNT 为单次查询 (P1)

**Files:**
- Modify: `backend-go/internal/domain/articles/handler.go:43-58`（`GetArticlesStats`）

**问题背景：**
三次独立 `COUNT(*)` 查询扫描 articles 表三次，合并为一次。

**Step 1: 替换 GetArticlesStats 函数体**

```go
func GetArticlesStats(c *gin.Context) {
    type StatsResult struct {
        Total    int64
        Unread   int64
        Favorite int64
    }
    var result StatsResult
    database.DB.Model(&models.Article{}).
        Select("COUNT(*) as total, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread, SUM(CASE WHEN favorite THEN 1 ELSE 0 END) as favorite").
        Scan(&result)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "total":    result.Total,
            "unread":   result.Unread,
            "favorite": result.Favorite,
        },
    })
}
```

**Step 2: 提交**

```bash
git add backend-go/internal/domain/articles/handler.go
git commit -m "perf: merge GetArticlesStats 3x COUNT into single query"
```

---

## Task 5: 优化 RefreshFeed 逐条查重为批量查重 (P1)

**Files:**
- Modify: `backend-go/internal/domain/feeds/service.go:60-112`（`RefreshFeed` 循环中的逐条查重）

**问题背景：**
当前对 RSS feed 中每篇 entry 执行 `WHERE feed_id = ? AND title = ?` 查重。如果 feed 有 100 篇 entry，就是 100 次 DB 查询。

**Step 1: 修改 RefreshFeed — 批量预加载已有 titles**

在 `service.go` 的 `RefreshFeed` 方法中，替换 `for _, entry := range parsed.Entries` 循环。在循环前添加批量查询：

```go
// 批量查询已有 title — 替代逐条查重
var existingTitles []string
database.DB.Model(&models.Article{}).
    Where("feed_id = ?", feed.ID).
    Pluck("title", &existingTitles)
titleSet := make(map[string]bool, len(existingTitles))
for _, t := range existingTitles {
    titleSet[t] = true
}

articlesAdded := 0
for _, entry := range parsed.Entries {
    if entry.Link == "" {
        continue
    }

    if titleSet[entry.Title] {
        continue
    }

    article := s.buildArticleFromEntry(feed, entry)

    if article.PubDate == nil {
        now := time.Now().In(time.FixedZone("CST", 8*3600))
        article.PubDate = &now
    }

    if err := database.DB.Create(&article).Error; err != nil {
        continue
    }

    titleSet[entry.Title] = true

    if err := s.enqueueArticleProcessing(feed, article); err != nil {
        logging.Errorf("Error enqueueing processing for article %d (feed %d): %v", article.ID, feed.ID, err)
    }

    articlesAdded++
    if articlesAdded >= feed.MaxArticles {
        break
    }
}
```

**Step 2: 运行测试**

Run: `cd backend-go && go test ./internal/domain/feeds/... -v`
Expected: `TestBuildArticleFromEntryTracksOnlyRunnableStates`、`TestCleanupOldArticlesKeepsActiveCompletionArticles`、`TestRefreshFeedEnqueues*` 全部通过

**Step 3: 提交**

```bash
git add backend-go/internal/domain/feeds/service.go
git commit -m "perf: batch-load existing titles in RefreshFeed instead of per-entry query"
```

---

## Task 6: 批量化 cleanupOldArticles — 避免 Go 内存全量加载 (P1)

**Files:**
- Modify: `backend-go/internal/domain/feeds/service.go:143-168`（`cleanupOldArticles`）

**问题背景：**
当前加载 feed 下全部 articles 到内存，然后逐条删除。如果 feed 有 10000 篇文章、`max_articles=100`，需要加载 10000 行到内存只为找 9900 条待删除的。

**优化策略：** 用 SQL 直接找出需要保留的文章 ID，再批量删除其余的。保留条件与原逻辑一致（跳过 favorite、跳过活跃状态的）。

**CASCADE 说明：** 原代码逐条删除 `ReadingBehavior`（service.go:164），新代码依赖 PostgreSQL 的 `ON DELETE CASCADE` 外键约束（定义在 reading_behavior.go:17 的 `constraint:OnDelete:CASCADE` 标签）。该约束由 `AutoMigrate` 在 bootstrap 阶段创建（migrator.go:114），PostgreSQL 已确保存在。

**Step 1: 替换 cleanupOldArticles**

```go
func (s *FeedService) cleanupOldArticles(feed *models.Feed) {
    var articleCount int64
    database.DB.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&articleCount)

    if int(articleCount) <= feed.MaxArticles {
        return
    }

    // 找出需要保留的文章 ID（按 pub_date 降序取前 MaxArticles 篇）
    // 其中 favorite=true 或状态活跃的文章优先保留
    var allArticles []struct {
        ID             uint
        Favorite       bool
        FirecrawlStatus string
        SummaryStatus  string
    }
    database.DB.Model(&models.Article{}).
        Select("id, favorite, firecrawl_status, summary_status").
        Where("feed_id = ?", feed.ID).
        Order("pub_date DESC").
        Find(&allArticles)

    keepIDs := make([]uint, 0)
    candidates := make([]uint, 0)

    for _, a := range allArticles {
        isActive := a.FirecrawlStatus == "pending" || a.FirecrawlStatus == "processing" ||
            a.SummaryStatus == "incomplete" || a.SummaryStatus == "pending"

        if a.Favorite || isActive {
            keepIDs = append(keepIDs, a.ID)
        } else {
            candidates = append(candidates, a.ID)
        }
    }

    // 计算还能保留多少篇非活跃、非收藏的文章
    remaining := feed.MaxArticles - len(keepIDs)
    if remaining > 0 {
        keepFromCandidates := candidates
        if len(candidates) > remaining {
            keepFromCandidates = candidates[:remaining]
        }
        keepIDs = append(keepIDs, keepFromCandidates...)
    }

    if len(keepIDs) == 0 {
        return
    }

    // 批量删除不在 keepIDs 中的文章
    database.DB.Where("feed_id = ? AND id NOT IN ?", feed.ID, keepIDs).Delete(&models.Article{})
}
```

**Step 2: 运行测试**

Run: `cd backend-go && go test ./internal/domain/feeds/... -v -run TestCleanup`
Expected: `TestCleanupOldArticlesKeepsActiveCompletionArticles` 通过

**Step 3: 提交**

```bash
git add backend-go/internal/domain/feeds/service.go
git commit -m "perf: batch-delete old articles instead of per-row delete in cleanup"
```

---

## Task 7: 消除 RefreshAllFeeds 重复全量加载 (P1)

**Files:**
- Modify: `backend-go/internal/domain/feeds/handler.go:459-508`（`RefreshAllFeeds` + `refreshAllFeedsWorker`）

**问题背景：**
Handler 中 `database.DB.Find(&feeds)` 加载一次设状态，worker 中 `database.DB.Find(&feeds)` 又加载一次。

**Step 1: 修改 RefreshAllFeeds 和 worker**

```go
func RefreshAllFeeds(c *gin.Context) {
    var feeds []models.Feed
    if err := database.DB.Find(&feeds).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   err.Error(),
        })
        return
    }

    if len(feeds) == 0 {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "No feeds to refresh",
        })
        return
    }

    feedIDs := make([]uint, len(feeds))
    for i, feed := range feeds {
        feedIDs[i] = feed.ID
    }

    now := time.Now()
    database.DB.Model(&models.Feed{}).Where("id IN ?", feedIDs).
        Updates(map[string]interface{}{
            "refresh_status": "refreshing",
            "last_refresh_at": &now,
            "refresh_error":   "",
        })

    go func(ids []uint) {
        refreshAllFeedsWorker(ids)
    }(feedIDs)

    c.JSON(http.StatusAccepted, gin.H{
        "success": true,
        "message": "Started refreshing all feeds in background",
        "data": gin.H{
            "total_feeds": len(feeds),
        },
    })
}

func refreshAllFeedsWorker(feedIDs []uint) {
    feedService := NewFeedService()
    for _, id := range feedIDs {
        if err := feedService.RefreshFeed(context.Background(), id); err != nil {
            continue
        }
    }
}
```

**Step 2: 提交**

```bash
git add backend-go/internal/domain/feeds/handler.go
git commit -m "perf: eliminate duplicate feed loading in RefreshAllFeeds"
```

---

## Task 8: BulkUpdateArticles 安全加固 — 必须指定范围 (P2)

**Files:**
- Modify: `backend-go/internal/domain/articles/handler.go:426-478`（`BulkUpdateArticles`）

**问题背景：**
如果前端传空 body（无 IDs、无 FeedID、无 CategoryID），会更新全表。

**Step 1: 添加范围校验**

在 `BulkUpdateArticles` 的 `updates` 非空检查之后、`query := database.DB.Model(...)` 之前，添加：

```go
if len(req.IDs) == 0 && req.FeedID == nil && req.CategoryID == nil && (req.Uncategorized == nil || !*req.Uncategorized) {
    c.JSON(http.StatusBadRequest, gin.H{
        "success": false,
        "error":   "Must specify a scope: ids, feed_id, category_id, or uncategorized",
    })
    return
}
```

**Step 2: 提交**

```bash
git add backend-go/internal/domain/articles/handler.go
git commit -m "fix: require scope parameter in BulkUpdateArticles to prevent full-table update"
```

---

## Task 9: 全文搜索优化 — PostgreSQL ts_vector (P2, 可选)

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`（添加迁移）
- Modify: `backend-go/internal/domain/articles/handler.go:140-143`（search 过滤）

**问题背景：**
`LIKE '%keyword%'` 无法使用 B-tree 索引，数据量大时全表扫描。PostgreSQL 原生支持 `ts_vector` + GIN 索引。

**注意：** 此 Task 可选，优先级最低。LIKE 查询加上适当的索引和分页限制后，在单用户场景下可能足够。

**Step 1: 添加全文搜索迁移**

```go
{
    Version:     "20260417_0002",
    Description: "Add GIN index for article full-text search using tsvector.",
    Up: func(db *gorm.DB) error {
        stmts := []string{
            `ALTER TABLE articles ADD COLUMN IF NOT EXISTS search_vector tsvector`,
            `CREATE INDEX IF NOT EXISTS idx_articles_search_vector ON articles USING GIN (search_vector)`,
            `CREATE OR REPLACE FUNCTION articles_search_vector_update() RETURNS trigger AS $$
            BEGIN
                NEW.search_vector :=
                    setweight(to_tsvector('simple', COALESCE(NEW.title, '')), 'A') ||
                    setweight(to_tsvector('simple', COALESCE(NEW.description, '')), 'B');
                RETURN NEW;
            END;
            $$ LANGUAGE plpgsql`,
            `DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles`,
            `CREATE TRIGGER articles_search_vector_trigger
                BEFORE INSERT OR UPDATE OF title, description ON articles
                FOR EACH ROW EXECUTE FUNCTION articles_search_vector_update()`,
            `UPDATE articles SET search_vector =
                setweight(to_tsvector('simple', COALESCE(title, '')), 'A') ||
                setweight(to_tsvector('simple', COALESCE(description, '')), 'B')`,
        }
        for _, s := range stmts {
            if err := db.Exec(s).Error; err != nil {
                return fmt.Errorf("full-text search migration: %w", err)
            }
        }
        return nil
    },
},
```

**Step 2: 修改 GetArticles 的 search 过滤 — 保留 LIKE 回退**

```go
if search != "" {
    if database.DB.Dialector.Name() == "postgres" {
        query = query.Where("articles.search_vector @@ plainto_tsquery('simple', ?)", search)
    } else {
        searchTerm := "%" + search + "%"
        query = query.Where("articles.title LIKE ? OR articles.description LIKE ?", searchTerm, searchTerm)
    }
}
```

**注意：** 需要同时更新 countQuery 中的 search 过滤（handler.go:180-183），使用相同的运行时守卫逻辑。

**Step 3: 提交**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go backend-go/internal/domain/articles/handler.go
git commit -m "perf: add PostgreSQL full-text search with tsvector + GIN index"
```

---

## 执行顺序总结

| 顺序 | Task | 优先级 | 影响范围 | 预期收益 |
|------|------|--------|----------|----------|
| 1 | Task 1: 添加索引 | P0 | 全局 | 查询性能基础保障 |
| 2 | Task 2: GetFeeds N+1 | P0 | feeds/handler, models/feed | 10-100x 列表提速 |
| 3 | Task 3: tag_stats 子查询 | P0 | articles/handler | 消除全表扫描 |
| 4 | Task 4: Stats 合并 | P1 | articles/handler | 减少 2 次全表扫描 |
| 5 | Task 5: 批量查重 | P1 | feeds/service | feed 刷新提速 |
| 6 | Task 6: cleanup 批量化 | P1 | feeds/service | 减少内存占用 |
| 7 | Task 7: RefreshAll 去重 | P1 | feeds/handler | 减少 1 次全量查询 |
| 8 | Task 8: BulkUpdate 安全 | P2 | articles/handler | 防止误操作 |
| 9 | Task 9: 全文搜索 | P2 | articles, migrations | 搜索性能大幅提升 |

## 验证 Checklist

每个 Task 完成后执行：

```bash
cd backend-go && go build ./...                          # 编译通过
cd backend-go && go test ./... -v                        # 全量测试
cd backend-go && go test ./internal/domain/feeds/... -v  # feeds 测试
cd backend-go && go test ./internal/domain/articles/... -v # articles 测试
```

全部 Task 完成后，可选重建 GitNexus 索引：
```bash
npx gitnexus analyze
```
