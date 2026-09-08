# 主阅读页流程（Reading）

<!-- doc-impact-applies: backend-go/internal/reader/, backend-go/internal/admin/service/, backend-go/internal/admin/handler/ | section=业务约束与不变量 -->
> 大功能：应用启动 → 切分类 → 切 feed → 打开文章 → 记录阅读行为。
> 阅读行为是「偏好向量画像」的权重源（画像与订阅源发现见 [discovery.md](discovery.md)）。
> 跨端。互补：`architecture/frontend.md` §页面骨架、§数据映射规则。

## 需求说明

主阅读页是用户使用系统的核心场景，解决「打开应用 → 找到想看的文章 → 阅读并被系统理解」这条主链路：

- **启动即就绪**：应用挂载后并发拉分类 / feed / 文章，首屏不空。
- **导航切换**：切分类、切 feed，列表与正文即时更新；feed 可手动刷新（含进度轮询）。
- **阅读即记录**：打开、滚动、关闭、收藏等行为自动上报进 `reading_behaviors`，作为偏好向量画像的行为权重源（旧「偏好分数」已废弃，见 [discovery.md](discovery.md)）。
- **文章自动打标签**：refresh / Firecrawl / 内容补全完成后按规则入标签队列，用户也可手动重打标签。

## 链路设计

### 前端数据流

```mermaid
flowchart TD
  PAGE[page 挂载] --> SHELL[feature shell / view]
  SHELL --> API["app/api/*"]
  API --> BACK[backend API]
  BACK --> APISTORE["useApiStore 主数据源"]
  APISTORE --> FEEDS[useFeedsStore]
  APISTORE --> ARTICLES[useArticlesStore]
  FEEDS & ARTICLES --> RENDER[组件渲染]
```

### Store 职责

| Store | 角色 | 职责 |
| ------- | ------ | ------ |
| `useApiStore` | 主数据源 | 拉分类/feed/文章；执行 CRUD；OPML 导入导出；AI 总结接口；启动初始化 |
| `useFeedsStore` | 派生订阅视图 | feed 分组、分类视图、未读数 |
| `useArticlesStore` | 派生文章视图 | 筛选条件、当前文章、已读/收藏统计、排序过滤 |

> 偏好画像不再是阅读页 store，改为设置「兴趣画像」section + 发现页消费，见 [discovery.md](discovery.md)。

### 字段映射规则

- 后端响应保留 `snake_case`；前端内部统一 `camelCase`
- 前端 `id` 统一转 `string`
- 转换集中在 API 模块和 `useApiStore`，组件层不散落映射逻辑

### 应用启动

```mermaid
sequenceDiagram
  participant App as app.vue
  participant Store as apiStore
  participant Shell as FeedLayoutShell
  App->>Store: initialize()
  par 并发拉取
    Store->>Store: fetchCategories()
    Store->>Store: fetchFeeds()
    Store->>Store: fetchArticles()
  end
  Store-->>Shell: 数据就绪
  Shell->>Shell: 渲染
```

### 切分类 / 切 feed

```text
切分类: AppSidebar → FeedLayoutShell.handleCategoryClick()
        → apiStore.fetchFeeds(...) → apiStore.fetchArticles(...) → 列表/正文更新

切 feed:  AppSidebar → FeedLayoutShell.handleFeedClick()
          → apiStore.fetchArticles(feed_id) → apiStore.refreshFeed(feed_id)
          → 轮询 refresh_status → 刷新完成后再拉文章
```

### 打开文章（阅读行为上报）

```text
ArticleListPanel → ArticleContentView
  → apiStore.markAsRead()
  → useReadingTracker 记录 open/scroll/close/favorite
  → reading_behavior 接口批量上报（POST /api/reading-behavior/track-batch）
  → 后端聚合进 reading_behaviors 表（偏好向量画像的权重源，见 flow/discovery.md）
```

### Feed 图标获取与渲染（本地化）

```text
RefreshFeed（icon_source ∈ {auto, fallback} 才重算；custom 不碰）
  → 候选管线：RSS <image> → 站点首页 HTML <link rel="icon">（仅 image 缺失时请求）
    → {host}/favicon.ico 猜测
  → 逐候选后端下载验证（10s 超时 / 256KB 上限 / 图片 Content-Type 校验，失败顺延）
  → 首个成功：落盘 data/icons/feeds/<feed_id>.<ext>（临时文件 + rename 原子写），
    DB icon = /icons/feeds/<feed_id>.<ext>、icon_source = auto；全失败：mdi:rss + fallback
  → icon 下载失败不影响 RefreshStatus（仍 success）

前端 FeedIcon.vue 三类值：iconify id → <Icon>（本地子集，零联网）；
  http(s) 远程 URL（存量）→ <img> 直连；/ 开头同源路径 → getApiOrigin() 拼后端源渲染 <img>；
  <img> onerror 降级 mdi:rss
```

UI 图标（mdi:*）同为本地化机制：启动时 `app/plugins/iconify-local.ts` 将 `app/assets/iconify-subset.json`（源码扫描生成的子集，162 个图标）注册进 `@iconify/vue`，运行时不请求 api.iconify.design；新增图标需 `pnpm generate:icons` 重新生成并提交产物。

## 业务约束与不变量

> 本节同时是 constraint-injection extension 的注入数据源——改 `internal/reader/` 或 `internal/admin/`（偏好/行为） 代码前会被自动注入 system prompt，必读。

1. **前端 `id` 一律字符串化、字段一律 camelCase，转换只在 API 模块/useApiStore，组件层不得散落映射**：后端 `snake_case` + 数值 id 到前端必须转 `string`，转换只发生在 API 模块 / `useApiStore`，组件层不得散落映射。违反会触发比较/路由参数类型错配。
2. **旧偏好分数体系已废弃删除，reading_behaviors 采集链路必须保留不动、只作偏好向量画像的行为权重源**：旧 `user_preferences` 表 + `preference_score` + `preference_update` 调度器已整体废弃删除（preference-vector-feed-discovery）；`reading_behaviors` 采集链路本身**保留不动**，改为作为偏好**向量画像**的行为权重源（按 SemanticBoard 聚合的 embedding），画像/调度/约束见 [discovery.md](discovery.md) §业务约束与不变量。
3. **Article 打标签时机必须与 feed 开关耦合：无 Firecrawl 入库即打，开启则经 tag_jobs 异步、含摘要先等正文**：
   - 普通 refresh 新文章：入库后立即打标签（feed 未开启 Firecrawl 时）。
   - feed 开启 Firecrawl：refresh 阶段**先不打标签**；Firecrawl 抓完写入 `tag_jobs` 队列，由 `TagQueue` worker 异步打标签；同时开启 `article_summary_enabled` 时，等 ContentCompletion 生成 `ai_content_summary` 后再 enqueue `tag_jobs`（打标签依赖整理后的正文）。
   - 手动打标签 `POST /api/articles/:article_id/tags`：只 enqueue 返回 `job_id`，`TagQueue` 完成后经 WebSocket 广播 `tag_completed`；mono 路径 LLM 提示词按优先级排序返回，后端写 `article_topic_tags` 前截断到文章级上限（6）作兜底。
   - **按 `content_form` 分流（aggregate-article-tagging）**：`tagArticle` 编排层读 `articles.content_form`——`aggregate`（聚合型文章，由摘要链路形态标记产生，见 [content-enrichment.md](content-enrichment.md) 约束 #10）走切片 map-reduce 路径：纯代码按 `## ` 栏目切片（跳导读、短栏目向后合并、超长按 `###` 细分、上限 8 片，无 `##` 结构回落 mono）→ 每片 1 次融合 prompt LLM 调用（event/person/keyword 三分类合一，每片上限 4 标签，单片失败重试 3 次后跳过不阻断）→ 跨片 `Slugify` 去重（保留首栏目出现者）→ 文章级上限 15、score 按片位置分层（首片 0.9/中间 0.7/尾片 0.5）；`mono` 与空值走原双分支提取路径，输入截断 4000 runes、文章级上限 6、score 一律 0.7。两条路径共用同一入库链（`findOrCreateTag` → aux labels → `createArticleTopicTagLink` → event 标签 enqueue embedding）。
   - **提取容错（aggregate-tagging-resilience）**：JSON 解析入口（`parseRawTagObjects`，经 `jsonutil.SanitizeLLMJSON`）对尾逗号做无损修复；单标签校验失败降级不连坐——event/person 的 aux labels 校验失败时丢 aux 保标签（记 warning），融合路径 keyword 缺 description 跳过该标签（记 warning），mono 双分支同步降级；JSON 整体解析失败仍重试 3 次。聚合路径全部片处理完后标签数为 0（全片失败或全部空产出）时回落 mono 双分支提取（含 heuristic 兜底），聚合文章不以 0 标签结束。
   - `TagQueue.Start()` 首次启动失败不阻塞应用，后台按 30 秒间隔重试最多 10 次。
4. **Feed 图标必须按 auto/custom/fallback 状态机管理并本地落盘 data/icons/feeds/，不用文章封面、下载失败不影响 refresh**：`icon_source` ∈ `auto`（系统抓取，可刷新覆盖）/ `custom`（用户设定，RefreshFeed 不碰）/ `fallback`（占位，可刷新重算）。重算走候选管线（RSS image → 首页 HTML link → favicon.ico 猜测），**后端下载落盘 `data/icons/feeds/`、DB 存 `/icons/...` 同源路径**；不用文章封面图当 feed icon；icon 下载失败不影响 refresh 成功状态。删除 feed 时清理其 icon 文件（失败不阻断）。favicon 探测以 RSS channel link（站点首页）为基准，不用 feed URL（聚合器域名）或 Google s2 等第三方服务。
5. **mdi:* 图标必须全部来自构建产物本地子集 iconify-subset.json，运行时零联网，新增图标须重新生成子集**：`mdi:*` 图标全部来自构建产物 `app/assets/iconify-subset.json`（`pnpm generate:icons` 生成并纳 git），运行时不访问 iconify API；源码新增图标名必须是子集的超集（一致性单测强制）。
6. **文章超限必须归档降级而非物理删除，favorite 永不归档，归档行永久保留（article-archive-instead-of-delete）**：`CleanupOldArticles` 对超出 `max_articles` 的最旧非 favorite 文章执行**归档降级**（`archived=true`），不物理删除——行与全部文本字段永久保留（日报线索按 ID 反查依赖此语义）；归档同时清除衍生数据（`article_topic_tags` 边 + 孤儿 tag 清理、`reading_behaviors`、`search_vector` 置 NULL）。不变量：
   - 活跃窗口计数与归档候选集**仅统计 `archived=false`**（归档行不得侵蚀窗口，否则每次刷新会误归档新文章）；
   - RSS 去重（title dedupe）**含归档文章**（防老条目重复入库）；
   - reader 列表/全局统计/feed 统计默认过滤 `archived=false`，`GET /api/articles?archived=true` 显式查归档集；按文章 ID 的详情查询豁免过滤。
   - `max_articles=0` 或 `9999` 仍为无限制；favorite 永不归档。

## 代码入口

- **后端 reader 域**：`backend-go/internal/reader/handler/`（article/feed/content-completion/firecrawl handler）、`backend-go/internal/reader/service/`（feed_service、content_completion_service、icon_store——feed 图标下载落盘）、`backend-go/internal/reader/repository/`、`backend-go/internal/reader/routes.go`；favicon 两级探测在 `rss_parser.go`（`ProbeFaviconCandidates`）。
- **后端阅读行为（admin 域）**：`backend-go/internal/admin/handler/preferences_handler.go`（仅留 reading-behavior handler）、`backend-go/internal/admin/routes.go`（`/reading-behavior/*`）；旧 `preferences_service.go` / `job_preference_update.go` / `/user-preferences/*` 已删除。
- **后端偏好画像 / 订阅源发现（admin 域）**：`backend-go/internal/admin/service/{preference_profile_service,recommendation_service,catalog_sync_service,catalog_extras,rsshub_config}.go`、`backend-go/internal/admin/handler/{preference_profile_handler,discovery_handler}.go`、`backend-go/internal/admin/scheduler/{job_preference_profile_update,job_rsshub_catalog_sync}.go`，详见 [discovery.md](discovery.md)。
- **打标签（tagmanagement 域）**：`backend-go/internal/tagmanagement/`（`TagQueue`、article_tagger）。
- **前端**：`front/app/features/articles/`（列表/正文/阅读追踪）、`front/app/features/shell/`（FeedLayoutShell、导航）、`front/app/stores/`（api/feeds/articles）、`front/app/composables/useReadingTracker.ts`（阅读行为采集）。偏好画像 UI 与发现页入口见 [discovery.md](discovery.md)。图标：`front/app/components/feed/FeedIcon.vue`（三类 icon 值渲染 + 降级）、`front/app/plugins/iconify-local.ts` + `front/app/assets/iconify-subset.json`（UI 图标本地子集）、`front/scripts/generate-icon-subset.mjs`（子集生成）。
- 应用装配：`backend-go/internal/app/router.go`、`backend-go/internal/app/runtime.go`。

## 变更溯源

| 日期 | 变更 | 摘要 | 归档位置 |
|------|------|------|----------|
| 2026-08-23 | fix-quality-audit-p0 | 修复前端 AI 摘要开关字段错读：stores/api.ts 改读后端真实返回的 `article_summary_enabled`（缺省按 gorm default:false），清理 `aiSummaryEnabled`/`ai_summary_enabled` 死字段与死 key；开关展示从「恒开启」变为真实值 | [`openspec/changes/archive/2026-08-23-fix-quality-audit-p0`](../../../openspec/changes/archive/2026-08-23-fix-quality-audit-p0) |
| 2026-05-13 | backend-package-restructure | 后端按域拆包：`feeds/articles/categories` → `reader/` 域，`preferences`/reading-behavior → `admin/` 域；本 flow「代码入口」的包路径即来自此次重组（旧 user-guide 的 `internal/domain/*`、`internal/jobs/` 路径已失效） | [`openspec/changes/archive/2026-05-13-backend-package-restructure`](../../../openspec/changes/archive/2026-05-13-backend-package-restructure) |
| 2026-07-25 | preference-vector-feed-discovery | 旧「偏好分数」(`user_preferences` 表 / `preference_update` 调度器 / `/api/user-preferences/*` / `usePreferencesStore` / `ReadingPreferencesPanel`) 整体废弃删除；`reading_behaviors` 采集保留并改为偏好**向量画像**权重源；约束迁至 [discovery.md](discovery.md) | [`openspec/changes/archive/2026-07-25-preference-vector-feed-discovery`](../../../openspec/changes/archive/2026-07-25-preference-vector-feed-discovery) |
| 2026-08-04 | slim-header-feed-actions | feed 管理入口迁移（BREAKING 用户习惯）：首页顶栏移除 4 个 feed 管理按钮（添加订阅/添加分类/导入 OPML/导出 OPML，11→7 按钮），聚合到设置页「订阅源」管理区（`/settings?section=feeds` 工具条）；业务链路无变化，仅 UI 入口路径 | [`openspec/changes/archive/2026-08-04-slim-header-feed-actions`](../../../openspec/changes/archive/2026-08-04-slim-header-feed-actions) |
| 2026-08-19 | article-archive-instead-of-delete | feed 超限文章从物理删除改为归档降级（`articles.archived` 字段）：行与原文永久保留、衍生数据清除（tags 边/行为/search_vector）；列表统计默认过滤归档、日报按 ID 反查豁免；旧死链（已删文章）无法复活 | [`openspec/changes/archive/2026-08-19-article-archive-instead-of-delete`](../../../openspec/changes/archive/2026-08-19-article-archive-instead-of-delete) |
| 2026-08-20 | aggregate-article-tagging | 打标链路按 `articles.content_form` 分流：aggregate 走栏目切片 map-reduce（每片融合提取、跨片去重、上限 15、score 0.9/0.7/0.5 分层）；mono/空值走原路径（截断 2000→4000 runes、上限 5→6）；形态标记由摘要链路产出（见 content-enrichment.md） | [`archive/2026-08-22-aggregate-article-tagging`](../../../openspec/changes/archive/2026-08-22-aggregate-article-tagging) |
| 2026-08-22 | aggregate-tagging-resilience | 提取链路容错：JSON 尾逗号无损修复（jsonutil）；单标签 aux/description 校验失败降级保留不再整片报废（mono+聚合同步）；聚合零产出回落 mono 双分支（含 heuristic 兜底）；修复后实测 event 存活（派早报 0→15 标签含 5 event、周刊首栏目 0.9 落地） | [`archive/2026-08-22-aggregate-tagging-resilience`](../../../openspec/changes/archive/2026-08-22-aggregate-tagging-resilience) |
| 2026-08-01 | localize-icons | Feed 图标从「存远程 URL 前端直连」改为「后端下载落盘 `data/icons/feeds/` + DB 存 `/icons/...` 同源路径」；favicon 探测增强（首页 HTML `<link rel="icon">` 解析 + `/favicon.ico` 猜测 + 下载验证）；UI 图标（mdi）改本地子集注册、运行时零联网 | [`openspec/changes/archive/2026-08-01-localize-icons`](../../../openspec/changes/archive/2026-08-01-localize-icons) |
| 2026-09-04 | constraint-declaration-redline | 约束节红线句格式化：本域「业务约束与不变量」节每条约束改写为首行加粗自含红线句 + 细节跟后（语义不变），declaration 注入降为红线层（上线后实测 bytes 降约 60%），细节层经关键词/JIT 全节注入按需补全；本域为格式改写，无业务行为变更 | [`openspec/changes/archive/2026-09-04-constraint-declaration-redline`](../../../openspec/changes/archive/2026-09-04-constraint-declaration-redline) |
