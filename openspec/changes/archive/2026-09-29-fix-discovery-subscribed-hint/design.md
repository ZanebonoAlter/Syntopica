## Context

现状与四个洞的定位见 `explore-findings.md` 与 proposal.md「Why」。设计上真正要回答的问题只有一个：**「这个候选/推荐/查询结果对应的来源，用户是不是已经订阅过了？」如何用一条统一、只读、可测试的规则回答**，并把它送到四个前端入口。

约束：

- 候选表 `feed_candidates` 对 rsshub 行不存地址（`feed_url` 恒 NULL），路由资料在 `rsshub_routes`（`namespace`/`path`/`example`），订阅真相在 `feeds.url`；三者之间**没有任何持久关联**（design D1 的既有约定：候选 ≠ 订阅，不建外键、不写关联）。判定必须纯读推导。
- `feeds.url` 是建源时规范化过的最终地址；RSSHub 实例基址由 `resolveRSSHubBaseURL(db)` 解析（`ai_settings.rsshub_base_url`，缺省 `https://rsshub.app`）。
- 召回期去重 SQL（`discovery_recall.go:searchCandidates`）现在按 `feeds.url = base + route.example` 精确匹配，只在「建卡那一刻」生效；本 change 明确**不动它**（见 Non-Goals）。
- 前端四个入口分属不同组件与不同接口响应，字段需要一次补齐，否则又会出现口径分裂。

## Goals / Non-Goals

**Goals:**

- 一条可单测的判定规则：RSSHub 路由级、原生 RSS 精确地址；纯读、批量、无 N+1。
- 四个入口（推荐卡 / 查询结果卡 / 候选库行 / 订阅弹窗）显示同口径的「已订阅」提示与已有订阅地址。
- 保留订阅入口（用户决策）：提示 ≠ 禁止再订别的参数；重复保护落在「最终地址」上。

**Non-Goals:**

- 不改召回期资格过滤 SQL（仍精确 URL 去重），不给推荐池新增/收紧过滤；历史 pending 卡不清理、不自动改写状态。
- 不改数据库 schema、不建候选↔订阅关联、不做数据迁移。
- 不改订阅/建源写路径（`CreateOrReuseFeed` 的幂等与 409 行为保持原样）。
- 不做「跨 RSSHub 实例基址」的历史订阅识别（见 Risks）。

## Decisions

### D1 判定规则：路由固定前缀 + 段边界

新增 `backend-go/internal/discovery/service/subscription_state.go`（纯函数 + 只读索引）：

- `routeFixedPrefix(namespace, path)`：取 `path` 中**首个 `:` 参数段之前**的部分，末尾去 `/`；如 `/event/:category?` → `/event`，`/znews/:section?` → `/znews`，`/newsflash` → `/newsflash`，`/:id?` → ``（空）。
- `routeSubscriptionPrefix(baseURL, namespace, path)`：`strings.TrimRight(baseURL,"/") + "/" + namespace + routeFixedPrefix(...)`。
  - 注意 `{regex}` 约束段永远跟在参数段之后，首个 `:` 之前的截断不受影响；这正是 `buildFeedURL` 的 `stripBraceConstraints`/`stripOptionalParams` 处理过的同一族路径语法，语义与之对齐。
- 命中判定 `feedURLMatchesRoutePrefix(feedURL, prefix)`：`feedURL == prefix`（可选参数缺省 = 裸路径）或 `HasPrefix(feedURL, prefix+"/")` 或 `HasPrefix(feedURL, prefix+"?")`（参数以 query 形式挂在裸路径上）。
  - **必须带段边界**：不能用裸 `HasPrefix(url, prefix)`，否则 `/zaobao/znews2` 会误命中 `/zaobao/znews`。
- 原生 RSS：`candidate.feed_url` 与 `feeds.url` **精确相等**（与现状一致）。
- 备选方案：SQL `LIKE prefix||'%'` 多前缀 OR（要转义 `%`/`_`，且不利于纯函数单测）；忽略 host 只比 path（跨基址更稳，但会把 `https://example.com/oschina/event` 这类原生源误判成 RSSHub 路由订阅）——均不采纳。

### D2 最长前缀优先（路由嵌套消歧）

同一响应内若同时出现 `/gov` 与 `/gov/ndrc/fggz/:category{.+}?` 这类前缀套前缀的路由，裸前缀规则会让 `/gov` 抢占更具体路由的订阅。判定按**最长匹配优先**：给定本响应所有候选路由前缀集合，某地址只算它命中的**最长**前缀所属路由的订阅。实现为 `SubscriptionIndex.MatchRouteSubscriptions(prefixes []string) map[string][]string`，在请求内一次性完成，跨请求的极端不一致（更具体路由恰好不在本响应里）记入 Risks。

### D3 索引装载：一次读全部订阅地址

`LoadSubscriptionIndex(ctx, db)` 一次 `SELECT url FROM feeds`，Go 侧做精确/前缀匹配；不逐 view 拼 SQL、不用 LIKE。

- 理由：单用户应用、`feeds` 量级几十~几百行；避免 LIKE 转义坑，避免每个 builder 各写一套 SQL，天然批量无 N+1。
- 备选：按需 `WHERE url IN (…)` + 前缀 LIKE（复杂度换来的收益在这个量级不存在）。

### D4 响应字段（三处统一）

| 响应 | 字段 |
| --- | --- |
| `CandidateView`（列表 + 详情，`GET /api/discovery/candidates[/:id]`） | `subscribed`（修正取值）、`subscribed_urls`（新增，排序稳定、空为 `[]`） |
| `RecommendationCard`（`GET /api/discovery/recommendations`） | `subscribed`、`subscribed_urls`（新增） |
| `DiscoveryRunItemView`（`GET /api/discovery/runs/:id`） | `subscribed`、`subscribed_urls`（新增） |

- `subscribed_urls` 语义：该来源已有订阅的 `feeds.url` 列表（rsshub = 路由前缀命中集合；rss = 精确命中集合），供前端 `title` 提示与弹窗对照。
- 后端字段均为新增/修正，向后兼容，无 BREAKING。
- 推荐卡与 run 条目的候选信息：run 条目经 `feed_candidates` 取 `kind` 与 `route_id` 再取路由（display 仍用现有 `feed_recommendations → rsshub_routes` 的 join，不改展示字段）；推荐卡按 `CandidateID` 批量取候选（legacy 无候选的行保持 `subscribed=false`）。

### D5 前端：提示与动作分离

「提示」只看 `subscribed`；「能否再点」按地址是否可能新增判断：

| 入口 | 提示 | 订阅入口 |
| --- | --- | --- |
| 候选库·原生 RSS | 已订阅（状态位，已有） | `subscribed` → 隐藏（精确命中，同址无新可订，维持现状） |
| 候选库·RSSHub | 已订阅（状态位，本次修正） | **保留** |
| 为你推荐卡 | 头部新增「已订阅」徽标（`title` = 已有订阅地址） | **保留**（一键订阅/填参表单原样） |
| 查询结果卡 | 新增「已订阅」徽标 | **保留**（原先「会话内已订阅 → 按钮换文本」改为徽标 + 按钮并存） |
| 订阅弹窗 | 「已有订阅」信息块（列出该路由已有地址） | 原生 RSS 详情已订阅 → 「已订阅」终态（同现状）；RSSHub 用 `subscribed_urls` 与实时计算的 `finalUrl` 比对，命中则锁定提交并说明「该地址已在订阅列表」，未命中但有其它订阅 → 只提示不锁定 |

- 前端 `kind === 'rss'` 与 `'rsshub'` 的按钮策略差异是刻意的：前者订阅是"地址本身"，后者订阅是"地址的一个参数实例"。
- 会话内标记 `store.subscribedRunIds` 保留（订阅成功即时反馈），与后端字段取**或**；不再借它隐藏按钮。

### D6 测试分层
- **纯函数**（`subscription_state_test.go`，SQLite 免 DB 亦可）：前缀截断（无参/可选参/必填参/regex 约束/裸 `/:id?`）、段边界、query 形态、尾斜杠、最长前缀消歧、精确匹配不误伤。
- **服务层**（SQLite，`testutil.SetupTestDB`）：候选库 rsshub 路由级命中与 `subscribed_urls`、原生精确命中、推荐卡与 run 条目字段（各自 `_test.go`）。
- **组件**（Vitest + happy-dom）：四个组件的徽标 / 入口保留 / 弹窗锁定。
- **人工**：本地实例 opencli/走查（可选，minor 档）。

### D7 「最终地址」必须逐字节可比（聚焦 review M1 补）

弹窗的 `finalUrl` 与后端写入的 `feeds.url` 是同一性判定的两串，必须逐字节可比，否则「同址即同订阅」在两侧各成一套。两道对齐：

- **path 段编码**：前端改用与 Go `url.PathEscape` 对齐的 `pathEscape`（保留 `A-Za-z0-9-_.~$&+:=@`，转义 `!'()*,;/?` 等）；`encodeURIComponent` 在 `+ & = : @ $ ! ' ( ) *` 上与 Go 分叉（实测 `C++` → Go `C++` / 前端 `C%2B%2B`）。
- **未填参数判定**：两侧从「整串含 `:`」改为「段首 `:` 残留」（后端 `hasUnfilledParamSegment` / 前端 `split('/').some(startsWith(':'))`）——编码对齐后参数值里的 `:` 会字面保留，旧判定会把本可用的地址误判为未填必填（连带缺陷）。

备选：由后端下发 canonical 最终地址供前端展示/比对（需新增字段与请求，且弹窗需随填参实时更新）——本次不做，两道对齐后两侧拼装同口径，成本更低。

## Risks / Trade-offs

- [前缀只按当前配置的 RSSHub 基址比对，用户改过 `rsshub_base_url` 后旧订阅不算命中] → 与召回期去重 SQL 同口径（不引入新的不一致）；提示缺省不误导（显示未订阅而不是错误显示已订阅），记入本 change 已知限制。
- [路由前缀套前缀的跨响应极端不一致（D2 只在响应内消歧）] → 影响面仅"提示"，且订阅入口保留，不产生错误写入；单测覆盖响应内消歧。
- [候选库 RSSHub 条目现在会显「已订阅」，用户误以为不能订别的参数] → 徽标旁保留订阅入口，弹窗以信息块 + 精确地址锁定说明"可继续订别的参数"。
- [推荐卡与 run 条目新增字段被旧前端忽略] → 字段是增量，旧前端行为与今天一致（无提示），不产生错误。
- [一次读全量 `feeds.url` 的查询开销] → 单用户量级可忽略；若未来 feeds 上万，可换前缀索引/物化表（不影响本 change 的对外语义）。

## Migration Plan

无数据迁移。部署顺序：后端先上（新增字段向后兼容）→ 前端发布（消费新字段）→ 重启后端静态托管即可；回滚按同一顺序反向（前端回滚后行为退回今天：无提示，不报错）。

## Open Questions

无（设计问题已在 change 内闭合；`subscribed_urls` 是否需要在列表页展示更多细节留待后续体验反馈，不影响本 change 的 spec 与任务拆分）。
