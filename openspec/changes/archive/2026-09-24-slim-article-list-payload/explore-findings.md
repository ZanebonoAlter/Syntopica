
## 文章列表 payload 构成与前端连发调用链（实测）

**列表 payload 构成（真库只读实测 2026-09-24）**
- `GET http://127.0.0.1:5100/api/articles?per_page=20`（identity）响应体 **834.2 KB**；字段合计 748.4 KB，其中 `content` 349.5 KB + `description` 345.7 KB + `firecrawl_content` 48.2 KB = 743 KB，其余 22 个字段合计仅 5 KB（差额 ~85 KB 是 JSON 转义放大）。
- `description` 与 `content` 实测为同一份 HTML（单篇样本各 89.3 KB 逐字节相同）→ 重复搬运。
- 列表行实际只消费 `title/link/author/read/favorite`（`front/app/features/articles/components/ArticleCardView.vue`）。
- 服务端生成该响应仅 ~20ms（本机 ::1 自测 32–140ms）→ 代价全在传输；未压缩 525 KB / gzip 94 KB（per_page=20）、1.74 MB / 312 KB（per_page=100）。

**后端投影位置**：`backend-go/internal/reader/handler/article_handler.go` 的 `GetArticles`（:20 无筛选分支、:135 relevance 分支、:139/:144 DISTINCT 分支、:147 concept/aux 分支）均 `Select("articles.*, ...")`；详情接口 `:506` 同类写法（详情接口本次不改）。`maxPerPage=100` 静默 clamp（`per_page=10000` → 100 条）。

**前端连发链条**
- `front/app/features/feeds/composables/useAutoRefresh.ts:36` 是**唯一** `fetchArticles({ per_page: 10000 })` 调用点；`initialize()` 为每个 feed（`feed.refreshInterval > 0`）建独立 `setInterval`，页面挂载时毫秒级批量创建 → 同分钟集中触发。DB 实测：23 个 feed `refresh_interval=60`（分钟）+ 1 个 = 15。
- 生产日志（单客户端 24h）：`/api/articles` 3475 次 = 总流量 91%（2903/3197 MB gzip 口径）；峰值单分钟 20–41 次、22–44 MB；该接口 p50 12–31s、max 59s，同期 1 KB 小接口 2 ms。8 并发实测每个 20–21s、聚合 ~430 KB/s（单发 ~300 KB/s）。

**阅读页两段式（关键：砍列不会破正文）**
- `front/app/features/shell/components/FeedLayoutShell.vue:204 hydrateSelectedArticle()`：先把列表行赋给 `selectedArticle`（首帧），再 `articlesApi.getArticle(id)` 取详情并用 `normalizeArticle` 覆盖（含 content/description/firecrawl_content）。
- `front/app/features/articles/composables/useArticleContentView.ts:101 mergedArticle` 只做「props.article + liveStatus 覆盖」，**不含详情 fetch**；`displayContent`（:156）取自 `mergedArticle.content` → 正文最终来自 `hydrateSelectedArticle` 的详情结果。
- 导语渲染点：`ArticleContentPreviewPanel.vue:117 <div v-html="article?.description" />`。

**不要踩的坑**：`articles.tag_count` 列**97.5% 为 NULL**（24259/24873 NULL、328 行与实际计数不符，只读实测）→ 列表 `tag_count` 不能改为直接读该列，保留逐行子查询（`article_topic_tags(article_id)` 有两个等价索引）。

**引用**：backend-go/internal/reader/handler/article_handler.go:65、front/app/features/feeds/composables/useAutoRefresh.ts:36、front/app/features/shell/components/FeedLayoutShell.vue:204、front/app/features/articles/composables/useArticleContentView.ts:101、front/app/features/articles/components/ArticleContentPreviewPanel.vue:117

<!-- pinned 2026-09-24T08:19:36Z -->

## 部署态验证：后端瘦身生效，但 nginx 入口/旧标签页仍跑旧前端

2026-09-24 部署后效果验证（用户反馈"还是有点卡"后的实测）：

【后端瘦身已生效】后端 PID 3085917（18:07 启动）已含新代码：`GET /api/articles?per_page=20` 实测 12,126B（基线 834KB，↓69x）、无 content 字段、excerpt 20/20；per_page=100 → 59KB（经 nginx gzip → 10.9KB）；耗时 50-80ms；详情 /api/articles/:id 抽样 1.5-31KB / 2-10ms，契约未变。

【卡顿根因：浏览器端跑的还是旧前端】
1. nginx 静态根 /srv/www 停留在 9/19 旧构建（change 构建 9/24 17:26 只铺了 backend-go/frontend/，即 :5100 直连入口）；:80 nginx 入口（192.168.5.24 / 100.83.247.38）端的是 5 天前旧前端。
2. backend-go/logs/app.log 实锤旧前端仍在跑：18:12、19:12 各一波 24 个 `per_page=10000` WARN（"clamped to 100"），时间戳间隔 50-60ms = 旧 useAutoRefresh 每 feed 一个 setInterval 的整点齐发风暴；另有 15min 间隔的单发（@15min feed）。用户有浏览器标签页自部署前一直开着（SPA 旧 JS 驻留内存）。旧前端下每个请求已被钳到 59KB（原 1.7MB），但 24 连发 + 旧前端整表替换渲染的卡顿感仍在。
3. 部署链路缺口：scripts/dev/deploy-frontend.sh 只铺 BACKEND_STATIC=backend-go/frontend，不同步 nginx 的 /srv/www；nginx conf（/etc/nginx/conf.d/syntopica.conf）注释的标准流程是 `sudo cp -r front/.output/public/. /srv/www/`。两个静态托管入口（:5100 后端直服 / :80 nginx）只有前者被部署脚本覆盖。

【次要观察】后端无 gzip 中间件，:5100 直连 JSON 裸传（59KB 不压缩）；:5100 的 index.html 无 Cache-Control（仅 Last-Modified 协商缓存）。经 nginx 的 JSON gzip 正常（59KB→10.9KB）。

【修复方向】① sudo rsync backend-go/frontend/ → /srv/www/；② 用户关闭/强刷旧标签页；③ 可选 follow-up：deploy-frontend.sh 增加 /srv/www 同步、后端加 gzip。

**引用**：scripts/dev/deploy-frontend.sh、backend-go/logs/app.log、/etc/nginx/conf.d/syntopica.conf

<!-- pinned 2026-09-24T11:18:37Z -->
