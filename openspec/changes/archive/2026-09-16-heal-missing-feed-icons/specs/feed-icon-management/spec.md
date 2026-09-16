## MODIFIED Requirements

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
