<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: reading -->

## Why

生产部署（树莓派 10.11.12.55）上**所有 feed 图标 404**：`/icons/feeds/2.ico` 之类请求全部落空（日志实测 203 次请求 / 203 次 404），前端 feed 列表图标整片空白。

根因是「DB 走了 dump、图标文件没走」：feed 图标文件落在 `backend-go/data/icons/feeds/`（被 `.gitignore` 的 `data/` 排除，不进 git、不进 dump），而 `feeds` 表的 `icon` 列已从旧机器的 `syntopica-migrate.dump` 恢复成 `/icons/feeds/N.<ext>` + `icon_source=auto`（dump 内实测含 20 条该形态路径）。DB 认为图标已本地化，磁盘上却一个文件都没有。

更麻烦的是**这套状态机不会自愈**：`resolveFeedIcon` 的冻结判据只看 DB 字符串（`icon_source == auto && strings.HasPrefix(icon, "/icons/")` → 直接 return，不下载、不探测），从不检查磁盘文件是否还在，所以每次刷新都跳过图标流水线，404 永久化。同类事故还会以别的方式复现（目录被误删、容器卷丢失、换机迁移），不能靠手工救。

前端还有第二层问题让症状更难看：`FeedIcon.vue` 的 `<img>` onerror 降级把**路径字符串**当 iconify 名传给 `<Icon>`（`<Icon :icon="icon || 'mdi:rss'">`，此时 `icon` = `/icons/feeds/2.ico`），Iconify 解析不出 → 渲染为空，既不显示图标也没有 RSS 占位，与 spec 里「SHALL 降级渲染 mdi:rss，SHALL NOT 留白」直接冲突。

## What Changes

- **后端自愈**：`resolveFeedIcon` 的「auto + 本地路径 → 冻结」判据增加**磁盘存在性校验**——`/icons/` 路径对应的文件不存在时，视为未本地化，重跑候选管线重新下载落盘（文件在则行为完全不变，仍是原来的冻结语义）。
- **前端降级修正**：`FeedIcon.vue` 图片加载失败降级时，只在 `icon` 是合法 iconify 名（`mdi:rss` 那种 `<prefix>:<name>` 形态）时用它，否则强制 `mdi:rss` 占位；阅读页头部 feed 徽标（`ArticleContentToolbar.vue` 直接 `<Icon :icon="feed.icon">`）改为复用 `FeedIcon`——同一症状的另一处（同样会把 `/icons/feeds/N.ico` 当图标名渲染成空 SVG）。
- **运维补漏**：`docs/reference/deployment.md` 迁移清单明确 `data/icons/` 属运行时资产，dump / git 均不携带，迁移须单独同步（否则会再次出现「DB 说有、磁盘没有」）。
- **数据修复（一次性操作，非代码）**：把受影响的 20 个 feed 的 `icon_source` 重置为 `fallback`、`icon` 置 `mdi:rss`，让下一次刷新（或一次手工验证）重新抓取图标落盘。

## Capabilities

### New Capabilities
（无）

### Modified Capabilities
- `feed-icon-management`: ① 「auto + 本地路径跳过重算」由「只看 DB 字符串」收紧为「DB 字符串 + 磁盘文件存在」双条件；② 前端 `FeedIcon` 降级要求明确「必须降级到合法 iconify 占位符，不得把图片路径当图标名渲染」。

## Impact

- 后端：`backend-go/internal/reader/service/feed_service.go`（冻结判据）、`backend-go/internal/reader/service/icon_store.go`（新增本地文件存在性查询）
- 前端：`front/app/components/feed/FeedIcon.vue`、`front/app/features/articles/components/ArticleContentToolbar.vue`（改用 FeedIcon）
- 文档：`docs/reference/flow/reading.md`（图标状态机 + 自愈语义）、`docs/reference/deployment.md`（迁移清单）
- 数据：`feeds` 表 20 行 `icon` / `icon_source` 一次性重置（运维命令，需人工执行 + 验证）
- 不涉及：API 形态、DB schema、配置项（`STORAGE_ICON_DIR` 语义不变）
