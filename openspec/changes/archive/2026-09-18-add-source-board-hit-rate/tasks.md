# Tasks — add-source-board-hit-rate

用例先行：实现前先读 `test-cases.md`（B1–B7 组，28 个 Scenario 全覆盖映射见其 §9）。口径以 `specs/source-board-hit-rate/spec.md` 为唯一权威。

## 1. 后端：聚合服务（口径唯一实现）

- [x] 1.1 新建 `backend-go/internal/tagmanagement/service/sourcestats/`：`ParseWindow(raw string) (int, error)`（白名单 {7,30,90}，空串→7，非法→错误）+ 包级口径常量与注释（注释 MUST 指向 spec 的三条硬约束：含归档、按文章去重、限窗口）。验证：单元测试覆盖 B3-03/04/05
- [x] 1.2 `FeedBoardHitStats(ctx, db, windowDays)`：单条聚合返回每源 `articles/in_board/tagged_no_board/untagged_pending/untagged_settled/tagging_enabled`。截止时间在 Go 侧算并传参；计数用 `COUNT(DISTINCT a.id)` + `SUM(CASE WHEN …)`；命中/有标签/排队中三个布尔位用 `EXISTS` 子查询（禁止 JOIN 造成行倍增）。验证：B1-01..12、B2-01..08 全绿
- [x] 1.3 `FeedBoardHitStats` 的板块分布：第二条聚合按 `(feed_id, semantic_board_id)` 去重计数，仅取 `label_type='board' AND status='active'`。验证：B1-01 断言 `SUM(boards) ≥ in_board`
- [x] 1.4 `BoardSourceBreakdown(ctx, db, boardID, windowDays)`：先校验板块存在且 `label_type='board'`（不存在/非 board → 返回可映射 404 的错误），再按源聚合 `articles/share/feed_articles/feed_hit_rate`。验证：B5-01..06 全绿

## 2. 后端：只读端点

- [x] 2.1 `GET /api/feeds/board-hit-stats?window=7`：`internal/reader/handler/feed_handler.go` 新增 handler + `internal/reader/routes.go` 注册（静态段与 `:feed_id` 共存，对齐既有 `/api/articles/stats` 先例）；响应沿用 `{success, data}` 信封。验证：B4-01..07 全绿
- [x] 2.2 `GET /api/semantic-boards/:id/source-breakdown?window=7`：`internal/tagmanagement/handler/` 新增 handler + `RegisterSemanticBoardRoutes` 注册。验证：B5-01..08 全绿
- [x] 2.3 错误映射：非法 window → 400、板块不存在/非 board → 404、聚合失败 → 500；两端点均不写库、不触发打标或匹配。验证：B4-05（连续调用库状态不变）、B3-04

## 3. 前端：数据层

- [x] 3.1 `front/app/types/feed.ts` 扩展：新增 `FeedBoardHitStats` / `FeedBoardHitBoard` / `BoardSourceBreakdown` 类型（字段名与 spec 一致，snake_case）。验证：`pnpm exec nuxi typecheck`
- [x] 3.2 `front/app/api/feeds.ts` 新增 `getBoardHitStats(windowDays)`；`front/app/api/semanticBoards.ts`（或既有板块 api 文件）新增 `getBoardSourceBreakdown(boardId, windowDays)`。验证：typecheck + 单测（如有既有 api 测试范式则补一条）
- [x] 3.3 新增组合式函数（设置侧）统一持有：窗口状态（默认 7）、统计数据、loading/error、`retry()`；供列表工具栏、行 pill、详情块共用（spec 要求窗口双向同步）。验证：B6-14

## 4. 前端：设置 → 订阅源

- [x] 4.1 `FeedMasterList.vue` 新增工具栏：窗口分段控件（7/30/90 天）、排序选择器（默认 / 分类内 · 入板块率 ↑ / 分类内 · 篇数 ↓ / 分类内 · 杂音量 ↓）、「只看低命中」筛选 chip（条件：≥5 篇 且 率 <30% 且 `taggingEnabled !== false`）。排序在既有分类分组内生效，不打乱分组。验证：B6-09..13
- [x] 4.2 新增 `FeedSourceQualityPill.vue`：高（≥60%）/中（30–60%）/低（<30%）三档 + 「样本少」（<5 篇）/「无新文」（0 篇）/「打标关闭」/加载中「—」占位；宽度固定不抖动。验证：B6-01..08
- [x] 4.3 列表项接入：meta 行扩展为「N 天内 X 篇 · 入板块 Y」，行右端放 pill。验证：B6-01/02
- [x] 4.4 新增 `FeedSourceQualityBlock.vue` 并插入 `FeedDetailEditor.vue` 状态条之后、设置表单之前：窗口控件（与列表共享状态）、四分解堆叠条 + 图例、率（分子/分母）、板块分布 chips（Top 6 + 其他 N 个）、空态/样本不足/打标关闭/失败重试四态。验证：B6-15..20
- [x] 4.5 `SettingsSectionFeeds.vue` 装配：`fetchFeeds` 与统计请求并行（任一失败不阻塞另一个），把统计按 `feed_id` 合成到 feed 视图模型；失败只影响 pill/详情块。验证：B6-07/08/20

## 5. 前端：/tags 板块页「板块内容」tab 内来源面板

- [x] 5.1 `TagsPage.vue`：**不新增 tab**；在「板块内容」tab 内、`BoardCompositionPanel` 之后插入 `BoardSourcePanel.vue`（同受 `contentTab === 'composition'` 渲染控制），选中板块变化时随 `loadComposition` 并行重取。验证：B6-21
- [x] 5.2 `BoardSourcePanel.vue`：汇总行（窗口控件 / 本板块篇数 / 来源数 / 最大来源及占比）、排序（按篇数 ↓ 默认 / 按该源入板块率 ↑）、5 列来源表（含占比条与「该源入板块率」pill）、口径脚注（同一文章多板块各计一次 + 只读说明）、空态与失败重试。行内 MUST NOT 有写动作。验证：B6-22..25
- [x] 5.3 切换板块 / 切换窗口时按新参数重取；旧请求结果不得覆盖新选中板块（沿用既有面板的请求竞态处理惯例）。验证：B6-21（快速切换板块后表格来源与当前板块一致）

## 测试

- [x] T1 `sourcestats_test.go`：B1（口径与去重 12 条）+ B2（未打标两分 8 条）+ B3（窗口边界 7 条）落成断言（sqlite 内存库 + AutoMigrate，注入固定 now）
- [x] T2 源视角端点测试：B4 全 7 条（httptest + gin 路由）
- [x] T3 板块视角端点测试：B5 全 8 条（含 404 与空板块）
- [x] T4 真库不变量抽检 `sourcestats_pg_test.go`：B7-01..05（`database.InitDB` 既有范式，`-short` 下 skip；只断言不变量，不冻结数字）
- [x] T5 前端组件测试：B6 全 25 条映射到 `FeedSourceQualityPill` / `FeedMasterList` / `FeedSourceQualityBlock` / `BoardSourcePanel` 的组件单测
- [x] T6 `test-cases.md` 用例 ID 与测试代码可对账（grep 计数覆盖 B1–B7 各组），§9 映射表逐条落实

## 文档

<!-- doc-impact: flow, api -->

- [x] D1 `docs/reference/api/feeds.md`：新增 `GET /api/feeds/board-hit-stats` 条目（参数 window 白名单、响应字段与口径三条硬约束）
- [x] D2 `docs/reference/api/semantic-boards.md`：新增 `GET /api/semantic-boards/:id/source-breakdown` 条目（响应字段、404 语义、与既有 `/articles` 的口径差异说明）
- [x] D3 `docs/reference/flow/reading.md`：链路设计补「源视角观测」小节（源 → 文章 → 标签 → 板块反向聚合入口）+ 变更溯源行
- [x] D4 `docs/reference/flow/semantic-board.md`：链路设计补「板块来源构成」小节（供血源视角、与板块文章列表的口径差异）+ 变更溯源行
- [x] D5 `docs/reference/architecture/map.md`：阅读域 / 语义版块域的「前端入口」列补来源面板（若既有列已足够表达则不改，说明理由）

## 验证

- [x] V1 `openspec validate add-source-board-hit-rate` → 输出 valid（delta spec 与 proposal Capabilities 一致）
- [x] V2 `cd backend-go && go test -short ./internal/tagmanagement/... ./internal/reader/...` → 全绿（含 T1–T3；-short 下真库用例 skip）
- [x] V3 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` → 无告警、构建成功
- [x] V4 `cd backend-go && go test -run TestSourceStatsPostgres ./internal/tagmanagement/service/sourcestats/ -v`（Docker Postgres 在跑）→ 真库不变量断言全绿：每源 `articles == in_board + tagged_no_board + untagged_pending + untagged_settled`、`SUM(boards) ≥ in_board`、含归档源 `in_board > 0`
- [x] V5 口径人工核对（一次性）：对现网 7 天窗口跑一次端点，逐源与该源在 `psql` 里手写 SQL 的结果比对，重点核 3 个快照（华尔街见闻-实时快讯-要闻 1662/437/739/486、凤凰网 499/160/268/64、最热文章 67/56/7/4）→ 数字一致
- [x] V6 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <本次改动文件...> --maxWorkers=2 && pnpm build` → 全绿（树莓派上单跑、串行，不与其它构建并行）
- [x] V7 opencli 主链路断言（ui-design Acceptance ①）：设置 → 订阅源 → 切 30 天（列表与详情同步）→ 排序切「入板块率 ↑」→ 开「只看低命中」→ 选中源看三分解；`/tags` → 选板块（「板块内容」tab 底部来源面板）→ 断言来源篇数合计 = 汇总「本板块 N 篇」→ 全通过
- [x] V8 双视口视觉检查（ui-design Acceptance ②）：1440×900 与 1920×1080 各截图（设置页 + 板块页）→ 无横向溢出、pill 不换行、堆叠条与图例对齐、表格按百分比拉伸
- [x] V9 `bash scripts/harness/doc-impact.sh verify add-source-board-hit-rate` + `bash scripts/harness/check-standards.sh` → 对账通过（flow/api 域已更新）
- [x] V10 写完 ui-design.md Acceptance 第 3 项（实现与批准原型的差异说明；重大差异需重置 `ui-approval: pending`）

### Scenario → 测试映射（§11 归档门禁）

| Scenario（delta spec） | 落点 |
| --- | --- |
| 一篇文章多标签多板块只计一次 | T1 / TC-B1-01、TC-B1-02 |
| 归档文章计入窗口统计 | T1 / TC-B1-07；T4 / TC-B7-03 |
| 非法窗口值被拒绝 | T1 / TC-B3-04、TC-B3-05；T3 / TC-B5-07 |
| 打标未完成与零标签可区分 | T1 / TC-B2-01..05 |
| 返回全量源的统计 | T2 / TC-B4-01、TC-B4-06 |
| 板块分布可超过命中数 | T1 / TC-B1-12；T2 / TC-B4-03 |
| 只读端点无副作用 | T2 / TC-B4-05 |
| 来源篇数合计等于板块总数 | T3 / TC-B5-01；T4 / TC-B7-04 |
| 上下文列帮助识别只偶尔命中的源 | T3 / TC-B5-03 |
| 板块不存在 | T3 / TC-B5-04、TC-B5-05 |
| 空板块 | T3 / TC-B5-06 |
| 显示来源构成 | T5 / TC-B6-22 |
| 切换板块重新拉取 | T5 / TC-B6-21 |
| 空态引导切窗口 | T5 / TC-B6-23 |
| 面板无写动作 | T5 / TC-B6-22 |
| 高命中源显示高指标 | T5 / TC-B6-01 |
| 高流量低命中源显示低指标 | T5 / TC-B6-02 |
| 样本不足不显示百分比 | T5 / TC-B6-04 |
| 打标关闭的源不显示 0% | T5 / TC-B6-06、TC-B6-19 |
| 排序在分类内生效 | T5 / TC-B6-09/10/11 |
| 窗口切换双向同步 | T5 / TC-B6-14 |
| 筛出杂音源 | T5 / TC-B6-12 |
| 打标关闭源不被判为杂音 | T5 / TC-B6-12 |
| 筛选无结果可复位 | T5 / TC-B6-13 |
| 展示三分解与率 | T5 / TC-B6-15 |
| 展示板块分布 | T5 / TC-B6-16 |
| 打标关闭不等于零命中 | T5 / TC-B6-19 |
| 统计失败不阻断设置 | T5 / TC-B6-20、TC-B6-25 |
