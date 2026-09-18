## Context

见 [proposal.md](proposal.md) 的 Why（每天 700–800 篇、只有少数进板块、目前没有反视角聚合）。规划前已完成的取证写在 [proposal.md](proposal.md) 的三条硬约束与 ui-design.md 的 Prototype fixture 里，要点：

- 数据链路无需新建：`feeds → articles → article_topic_tags → topic_tag_board_labels → semantic_labels(label_type='board')`，全部是既有多对多关系，缺的只是反向聚合。
- 现网规模：23 个源、13 个板块（12 active）、`articles` 2 万行、`article_topic_tags` 1.27 万行、`topic_tag_board_labels` 2505 行。真库 `EXPLAIN ANALYZE` 跑「7 天窗口按源三分解」**81ms**（树莓派本机，shared hit 8300），无缓存压力。
- 既有入口可直接挂：`GET /api/feeds` 已用「批量 `feed_id IN (...)` 聚合 + `FeedStats` 注入」范式（`backend-go/internal/reader/handler/feed_handler.go`），`GET /api/articles/stats` 与 `GET /api/articles/:article_id` 已有「静态段与参数段同级共存」先例，新增 `/api/feeds/board-hit-stats` 不会与 `/api/feeds/:feed_id` 冲突。
- 既有测试范式：handler 测试用 `glebarez/sqlite` 内存库 + `AutoMigrate`（如 `feed_create_handler_test.go`），因此聚合 SQL 必须避开 Postgres 专有语法。

## Goals / Non-Goals

**Goals**

- 一套口径、两处视角（源视角 / 板块视角）、两处展示面（设置列表+详情 / 板块页新 tab），全部只读。
- 口径实现唯一：任何消费方（端点、测试、未来脚本）都走同一份聚合代码，杜绝「界面一套口径、脚本另一套」。
- 在树莓派上零压力：单次批量查询、无定时任务、无物化表、无缓存层。

**Non-Goals**

- 不做一键降噪动作（调 max_articles / 关打标 / 退订）——处置仍走既有控件；不做自动建议清单。
- 不把命中率反馈进订阅源发现推荐（`feed-discovery` / `preference_vectors`）——后续独立 change。
- 不做趋势/环比、不做按来源过滤阅读列表、不做命中结果物化表或日报联动。
- 不改既有 toggle、最大文章数、OPML、订阅源增删改的任何契约。

## Decisions

### D1 聚合实现放一个共享包，两个 handler 共用

新增 `backend-go/internal/tagmanagement/service/sourcestats`，导出：

- `FeedBoardHitStats(ctx, db, windowDays) ([]FeedStat, error)`
- `BoardSourceBreakdown(ctx, db, boardID, windowDays) (BoardBreakdown, error)`

消费方：`internal/reader/handler/feed_handler.go`（源视角端点）与 `internal/tagmanagement/handler/board_*_handler.go`（板块视角端点）。

**理由**：口径（含归档、去重、未打标两分、active 板块判定）是本 change 最容易被写歪的东西，两个 handler 各自写 SQL 必然漂移。`reader/handler` 已 import `tagmanagement`（`article_handler.go`、`firecrawl_handler.go`），依赖方向既有，不产生环。

**替代方案**：① 各 handler 自己写（口径漂移，否）；② 放 `internal/reader/repository`（板块/标签匹配的判定逻辑不在 reader 域，反向不自然，否）；③ 放 `internal/platform`（业务语义放平台层不当，否）。

### D2 SQL 形态：Go 侧算截止时间 + 可移植聚合

- 截止时间在 Go 计算（`time.Now().AddDate(0,0,-windowDays)`）作为参数传入：`coalesce(a.pub_date, a.created_at) >= ?`。**不写 `now() - interval '7 days'`**——sqlite 测试库不认该语法，传参同时让窗口可注入（测试用固定时间）。
- 计数用 `COUNT(DISTINCT a.id)` + `SUM(CASE WHEN ... THEN 1 ELSE 0 END)` 家族，**不用 `FILTER (WHERE ...)`**：仓库既有 `unread_count` 就是 `SUM(CASE WHEN NOT read THEN 1 ELSE 0 END)`（`feed_handler.go`），沿用同族写法可保证 sqlite/Postgres 双跑。
- 「命中」「有标签」「打标中」三个布尔位用 `EXISTS` 子查询表达，**不 JOIN**：JOIN 会让一篇文章按标签数/板块数倍增，正是 proposal 记录的 1662→5193 错算根因。
- 板块分布单独一条聚合（`GROUP BY a.feed_id, bl.semantic_board_id`），与源级三分解分开算，避免把「按板块计次」混进「按文章去重」的分母。
- 窗口内包含归档文章：查询**不过滤 `archived`**（口径明确写在 spec，代码处加注释指向 spec，防后来者"顺手"加过滤）。

### D3 「未打标」两分的判定

`untagged_pending` = 无标签 **且** `EXISTS (SELECT 1 FROM tag_jobs j WHERE j.article_id = a.id AND j.status IN ('pending','leased'))`；`untagged_settled` = 无标签且无 `pending`/`leased` 任务（含 `completed`、`failed`、以及从未入队——例如该源 `tagging_enabled=false`）。状态常量取 `models.JobStatusPending/Leased`，不写字面量。

### D4 端点契约

| 端点 | 说明 |
| --- | --- |
| `GET /api/feeds/board-hit-stats?window=7` | 全量源一次返回（23 条量级），字段见 spec |
| `GET /api/semantic-boards/:id/source-breakdown?window=7` | 单板块来源构成，字段见 spec |

两者都：只读；`window` 白名单 `{7,30,90}`，缺省 7，非法 → 400；响应沿用仓库既有 `{success, data}` 信封；板块端点非 `board` 类型或不存在 → 404（复用既有 `label_type='board'` 校验写法）。

### D5 前端数据装配：一次拉全量，排序筛选在前端

- 设置页 `SettingsSectionFeeds.vue` 已经是「`fetchFeeds({per_page: 10000})` 拉全量 + 前端组装分类分组」，本 change 只需并行拉一份 `board-hit-stats`，按 `feed_id` 合成到 feed 视图模型（不引入新 store，不新增按源查询端点）。
- 窗口状态提升到 `SettingsSectionFeeds.vue`，向下传给 `FeedMasterList`（工具栏 + 行 pill）与 `FeedDetailEditor`（来源质量块），实现 spec 要求的「任一处切换两处同步」。
- 阈值常量（低命中 30%、最小样本 5 篇）放前端常量文件（或组件内常量 + 注释指向 spec），后端不参与「低命中」判定（后端只给事实数字）。
- 板块页：**不新增 tab**；「板块内容」tab（`contentTab === 'composition'`，默认 tab）内、既有 `BoardCompositionPanel` 之后插入 `BoardSourcePanel.vue`，同受 tab 渲染控制。数据走新 composable，选中板块变化时与既有 `loadComposition` 并行重取（不再用 `lazyPanel()` 懒加载——面板随默认 tab 首屏可见，懒加载语义已不成立）。

### D6 不做缓存与物化

实测 81ms、只读、按需触发（打开设置页 / 选中板块），无缓存层；窗口限定天然限制了扫描量。**触发条件**：若实测 p95 > 300ms（例如文章量十倍后），再评估短 TTL 或物化表，不在本 change 预先设计。

### D7 测试策略

| 层 | 手段 |
| --- | --- |
| 聚合服务 | sqlite 内存库 + `AutoMigrate` 单元测试，覆盖三条不变量：分母去重（多标签多板块只计一次）、归档文章计入、未打标两分 |
| 端点 | `httptest` + gin 路由（对齐 `feed_create_handler_test.go` 范式）：窗口白名单 400、空窗口 0 篇非错误、板块 404、来源篇数合计 = 板块总数 |
| 真库抽检 | 用 `database.InitDB` 连 Docker Postgres 跑一次口径对照（`db_test.go` 既有范式），断言含归档与去重口径与实测快照一致 |
| 前端 | 组件单测（pill 三态/样本不足/打标关闭、筛选与排序、详情块渲染、面板空态）+ `pnpm exec nuxi typecheck` |

## Risks / Trade-offs

- **[高频源的文章几乎全在归档区] → 误判源头**：口径明确「含归档」，spec 与代码注释双写；测试用「归档里才有命中」的用例钉住。
- **[打标队列滞后导致短期窗口的「未打标」被误读成杂音] →** 拆 `untagged_pending` / `untagged_settled` 两个数并在 UI 分色展示；窗口默认 7 天但展示 pending 量，用户能看出"还在排队"。
- **[慢源在 30/90 天窗口仍样本不足] →** 前端「样本少」规则（<5 篇不给百分比）与排序把无样本项后置。
- **[查询随文章量线性增长] →** 窗口限定 + 既有索引（`idx_articles_feed_pub_date`、`article_topic_tags(article_id)` 索引、`topic_tag_board_labels` 主键）；实测 81ms，超过 300ms 再考虑缓存/物化（D6）。
- **[口径漂移] →** 聚合集中在单一包 + spec 固定 + 三不变量的测试用例；未来若要改口径必须先改 spec。
- **[打标关闭的源被读成 0% 杂音] →** 端点返回 `tagging_enabled`，前端单独标注并排除出「只看低命中」筛选。

## Migration Plan

- **无 DB 迁移、无写路径**：只新增只读端点与前端展示；部署只需重新构建前端 + 重启后端。
- **旧数据**：无需回填或降级——统计按需实算，任何时候打开都是当前真库口径；打标覆盖低的老文章只影响窗口选择（默认 7 天正为规避这一点）。
- **回滚**：回滚代码即可，无数据副作用（不新增表、不新增写入）。

## Open Questions

以下均为可延后、不影响本 change 规格与任务拆解的问题：

- 是否需要趋势视图（本周 vs 上周命中率变化）——先看用户实际使用频率再定。
- 是否把命中率作为订阅源发现推荐的排序信号（`feed-discovery`）——独立 change。
- 「低命中」阈值 30% 是否需要做成用户可配——当前硬编码常量，观察一段时间后议。
