# Feed Icon Management

## Purpose

定义 Feed 图标的来源状态机与获取策略。通过 `IconSource` 字段显式区分"系统自动抓取的图标（可刷新）"与"用户设定的图标（不可覆盖）"，统一处理 favicon 获取（RSS channel link → /favicon.ico）、默认值收敛、前端降级渲染，消除 icon 字段语义混乱、favicon 取错域名、Google s2 国内被墙等问题。

## Requirements

### Requirement: Feed 模型 icon_source 字段
Feed 模型 SHALL 包含 `icon_source` string 字段，取值范围为 `auto` / `custom` / `fallback`，数据库 default 为 `fallback`。该字段与 `icon` 字段配合，`icon` 承载值（iconify id 或图片 URL），`icon_source` 承载来源语义。

#### Scenario: 新建 feed 默认 fallback
- **WHEN** 创建新 feed（未显式指定 icon）
- **THEN** icon_source 为 `fallback`，icon 为 `mdi:rss`

#### Scenario: OPML 导入默认 fallback
- **WHEN** 通过 OPML 导入 feed
- **THEN** icon_source 为 `fallback`，icon 为 `mdi:rss`

### Requirement: RefreshFeed 按状态机决定是否重算 icon
`RefreshFeed` SHALL 仅当 `icon_source` 为 `auto` 或 `fallback` 时考虑重算 icon。当 `icon_source` 为 `custom` 时，SHALL 跳过 icon 重算，保持用户设定值不变。当 `icon_source` 为 `auto` 且 icon 已是本地化路径（`/icons/` 开头）时，SHALL 增加**磁盘存在性校验**后才跳过重算——仅当对应本地文件确实存在时跳过（已落盘的好图标不因远程临时故障被覆盖为 fallback）；本地文件不存在时 SHALL 视为未本地化，执行候选管线重新下载并落盘。判定「不存在」SHALL 仅限明确证明路径不可能存在的错误（`fs.ErrNotExist` / `ENOTDIR`），其余 stat 错误（权限、I/O）SHALL 保守视为存在（不因瞬时故障重抓并覆盖）。`auto` 但 icon 仍为远程 URL（存量未本地化数据）时 SHALL 执行重算以完成本地化。

#### Scenario: custom 状态不被刷新覆盖
- **WHEN** feed.icon_source = `custom`，执行 RefreshFeed
- **THEN** icon 和 icon_source 均不变

#### Scenario: fallback 状态触发重算
- **WHEN** feed.icon_source = `fallback`，执行 RefreshFeed
- **THEN** 尝试用候选管线重算并下载本地化 icon

#### Scenario: auto + 本地路径跳过重算
- **WHEN** feed.icon_source = `auto` 且 icon 为 `/icons/feeds/42.png`，且 `data/icons/feeds/42.png` 文件存在，执行 RefreshFeed
- **THEN** icon 管线不执行（不下载、不探测首页），icon 与 icon_source 均不变

#### Scenario: auto + 本地路径但文件缺失时自愈重抓
- **WHEN** feed.icon_source = `auto` 且 icon 为 `/icons/feeds/42.png`，但本地文件已不存在（如仅 DB 迁移、图标目录被清理），执行 RefreshFeed
- **THEN** 视为未本地化，执行候选管线；下载成功则 icon 换为新落盘的本地路径且 icon_source 保持 `auto`

#### Scenario: 自愈重抓全失败时收敛为 fallback
- **WHEN** feed.icon_source = `auto` 且 icon 为 `/icons/feeds/42.png` 但文件缺失，且全部候选下载失败
- **THEN** icon = `mdi:rss`、icon_source = `fallback`，RefreshFeed 仍返回成功

#### Scenario: auto + 存量远程 URL 触发本地化
- **WHEN** feed.icon_source = `auto` 且 icon 为 `https://example.com/favicon.ico`（存量数据），执行 RefreshFeed
- **THEN** 执行候选管线，下载成功后 icon 换为本地路径

### Requirement: icon 选图优先级
当 RefreshFeed 重算 icon 时，SHALL 按以下候选顺序尝试获取并**下载验证**：1) RSS `<image>` 标签 URL（parsed.Image）；2) 站点首页 HTML 中 `<link rel="icon" / "shortcut icon" / "apple-touch-icon">` 的 href（相对路径转绝对）；3) RSS channel link 的 host + `/favicon.ico` 猜测。首个下载成功的候选落盘为本地文件，置 icon = 本地路径、icon_source = `auto`；全部失败则保持 icon_source = `fallback`、icon = `mdi:rss`。候选 2 仅在候选 1 缺失时执行（避免每次 refresh 都请求站点首页）。

#### Scenario: RSS image 优先
- **WHEN** parsed.Image 非空且下载成功
- **THEN** icon = 下载后的本地路径，icon_source = `auto`

#### Scenario: RSS image 缺失时用 HTML link 探测
- **WHEN** parsed.Image 为空，站点首页 HTML 含 `<link rel="icon" href="/static/icon.png">` 且下载成功
- **THEN** icon = 该 URL 下载后的本地路径，icon_source = `auto`

#### Scenario: HTML link 缺失时回退 favicon.ico 猜测
- **WHEN** parsed.Image 为空、首页 HTML 无 icon link 标签，`{host}/favicon.ico` 下载成功
- **THEN** icon = 下载后的本地路径，icon_source = `auto`

#### Scenario: 候选下载失败顺延
- **WHEN** parsed.Image 非空但下载失败（404/超时/非图片/超限）
- **THEN** 继续尝试后续候选，不直接判 fallback

#### Scenario: 全部候选失败
- **WHEN** 三个候选均缺失或下载失败
- **THEN** icon = `mdi:rss`，icon_source = `fallback`

#### Scenario: icon 下载失败不影响 refresh 结果
- **WHEN** 文章抓取成功但全部 icon 候选下载失败
- **THEN** RefreshFeed 返回成功（RefreshStatus = success），仅 icon 保持 fallback

### Requirement: 不再使用文章封面图作为 feed icon
RefreshFeed 重算 icon 时 SHALL NOT 读取文章的 `ImageURL`。文章封面图属于文章级别资源，语义上不等于站点 logo。

#### Scenario: 文章图存在但不被用作 feed icon
- **WHEN** feed 下存在带 ImageURL 的文章，但 parsed.Image 和站点 favicon 均失败
- **THEN** feed.icon 保持 `mdi:rss`，不回退到文章图

### Requirement: UpdateFeed 设置 icon 时置 custom
`PATCH /api/feeds/:id` SHALL 在接受非空 `icon` 字段时，将 `icon_source` 置为 `custom`，确保用户设定的图标不被后续 RefreshFeed 覆盖。

#### Scenario: 用户更新 icon
- **WHEN** 发送 PATCH 请求 body 包含 `{"icon": "mdi:github"}`
- **THEN** feed.icon = `mdi:github`，feed.icon_source = `custom`

### Requirement: FetchFaviconURL 使用站点 URL 而非 feed URL
favicon 探测 SHALL 以站点首页 URL（RSS channel link）为基准，SHALL NOT 使用 RSS feed URL（聚合器域名）或 Google s2 等第三方服务。探测 SHALL 包含首页 HTML `<link rel="icon">` 解析与 `{scheme}://{host}/favicon.ico` 猜测两个层级；站点 URL 为空或无法解析 host 时返回空（由调用方保持 fallback 态）。

#### Scenario: 从站点 URL 拼 favicon
- **WHEN** siteURL = `https://example.com/articles`，HTML 中无 icon link
- **THEN** 猜测候选为 `https://example.com/favicon.ico`

#### Scenario: HTML link 相对路径转绝对
- **WHEN** siteURL = `https://example.com/articles`，HTML 含 `<link rel="icon" href="/a/b.png">`
- **THEN** 探测候选为 `https://example.com/a/b.png`

#### Scenario: 站点 URL 无法解析
- **WHEN** siteURL 为空或解析失败
- **THEN** 返回空（由调用方保持 fallback 态）

### Requirement: 前端 FeedIcon 图片加载失败降级
前端 `FeedIcon.vue` SHALL 在 `<img>` 加载失败（onerror）时降级渲染 `mdi:rss` Icon 组件，SHALL NOT 留白（display:none）。降级时 SHALL 只在 icon 值为合法 iconify 名（`<prefix>:<name>` 形态，prefix/name 为小写字母数字连字符段）时沿用该 icon 值，否则 SHALL 强制使用 `mdi:rss` —— 图片路径（`/icons/...`）、远程 URL、`data:` URL 与历史占位值（如 `rss`）SHALL NOT 被当作 iconify 名传给 `<Icon>` 组件（非法名会渲染成空 `<svg>`，即留白）。直接渲染 `feed.icon` 的其它组件（如阅读页 feed 徽标）SHALL 复用 `FeedIcon` 或应用等价的合法名校验。

#### Scenario: favicon URL 加载失败
- **WHEN** icon 为 `https://example.com/favicon.ico` 但加载触发 onerror
- **THEN** 渲染 `mdi:rss` Icon 组件替代

#### Scenario: 本地路径加载失败降级为占位符
- **WHEN** icon 为 `/icons/feeds/42.png` 但加载触发 onerror（文件缺失）
- **THEN** 渲染 `mdi:rss` Icon 组件（不得把该路径作为 iconify 名渲染）

#### Scenario: 正常 URL 加载成功
- **WHEN** icon 为有效图片 URL 且加载成功
- **THEN** 正常渲染 `<img>`

#### Scenario: iconify 名沿用
- **WHEN** icon 为 `mdi:github` 且渲染走 Icon 分支
- **THEN** 渲染 `mdi:github`

#### Scenario: 非图标名形态的存量值降级为占位符
- **WHEN** icon 为 `rss`（历史占位值）或 `data:image/png;base64,...`
- **THEN** 渲染 `mdi:rss` Icon 组件（不得作为 iconify 名渲染产生空 `<svg>`）

#### Scenario: 阅读页 feed 徽标复用降级逻辑
- **WHEN** 阅读页头部徽标渲染 `feed.icon = /icons/feeds/42.png` 且后端文件缺失（404）
- **THEN** 显示 `mdi:rss` 占位图标，不留白

### Requirement: 存量数据迁移幂等
icon_source 字段的迁移 SHALL 幂等：GORM AutoMigrate 加列带 default `fallback`，再用幂等 `UPDATE ... CASE WHEN` SQL 按 icon 现值推断 icon_source。重复执行不影响已迁移的行。

#### Scenario: URL 值归入 auto
- **WHEN** 存量 feed.icon = `https://example.com/logo.png`
- **THEN** 迁移后 icon_source = `auto`

#### Scenario: 占位符归入 fallback
- **WHEN** 存量 feed.icon IN ('', 'rss', 'mdi:rss')
- **THEN** 迁移后 icon_source = `fallback`，icon 统一为 `mdi:rss`

#### Scenario: 其他 iconify id 归入 custom
- **WHEN** 存量 feed.icon = `mdi:github`
- **THEN** 迁移后 icon_source = `custom`（保守保护可能的用户设定）

### Requirement: icon 下载本地化存储
后端 SHALL 将下载成功的 feed icon 写入本地文件系统（默认 `data/icons/feeds/<feedID>.<ext>`，目录可通过配置覆盖），并以「临时文件 + rename」原子写入。下载 SHALL 带超时（≤10s）与大小上限（≤256KB），并校验响应为图片内容。DB `icon` 字段存 `/icons/feeds/<feedID>.<ext>` 形式的同源相对路径。

#### Scenario: 下载成功落盘
- **WHEN** 候选 URL 返回 200 且 Content-Type 为 image/*（或 .ico 内容），大小 ≤256KB
- **THEN** 文件写入 `data/icons/feeds/<feedID>.<ext>`，DB icon 更新为对应 `/icons/...` 路径

#### Scenario: 超限或非图片拒绝写入
- **WHEN** 响应大小超 256KB 或内容非图片
- **THEN** 不落盘，视为该候选下载失败

#### Scenario: 同 feed 换扩展名清理旧文件
- **WHEN** feed 已有 `<feedID>.ico`，新下载图标为 png
- **THEN** 写入 `<feedID>.png` 并删除旧的 `<feedID>.ico`

### Requirement: 本地 icon 静态服务
后端 SHALL 注册 `/icons` 静态路由，直接服务 icon 存储目录，使前端可通过后端同源地址访问本地 icon。该路由独立于前端产物托管逻辑，dev 与生产模式均可用。

#### Scenario: 访问已落盘 icon
- **WHEN** GET `/icons/feeds/42.png` 且文件存在
- **THEN** 返回 200 与图片字节，带合理 Content-Type，且响应头含 `X-Content-Type-Options: nosniff` 与 `Content-Security-Policy: sandbox`（防 SVG 存储型 XSS）

#### Scenario: 访问不存在的 icon
- **WHEN** GET `/icons/feeds/999.png` 且文件不存在
- **THEN** 返回 404，不影响其他接口

### Requirement: 删除 feed 时清理 icon 文件
删除 feed 时 SHALL 一并删除其本地 icon 文件（若存在）；清理失败 SHALL NOT 阻断删除流程。

#### Scenario: 删除带本地 icon 的 feed
- **WHEN** DELETE feed 且 `data/icons/feeds/<feedID>.*` 存在
- **THEN** feed 记录删除成功，对应 icon 文件被删除

### Requirement: 前端同源路径解析与远程 URL 兼容
前端 `FeedIcon.vue` SHALL 识别三类 icon 值：iconify id（如 `mdi:rss`）、远程 http(s) URL（存量数据）、`/` 开头的同源相对路径。相对路径 SHALL 用 `getApiOrigin()` 拼成后端绝对地址渲染 `<img>`；远程 URL 按原逻辑直接渲染，保证存量数据在下次 refresh 前仍可见。

#### Scenario: 本地路径拼源渲染
- **WHEN** icon = `/icons/feeds/42.png`，dev 模式前端端口 3000
- **THEN** `<img>` src 渲染为 `http://localhost:5000/icons/feeds/42.png`

#### Scenario: 存量远程 URL 仍渲染
- **WHEN** icon = `https://example.com/favicon.ico`（未完成本地化的存量数据）
- **THEN** `<img>` 按原远程地址渲染，失败仍降级 `mdi:rss`
