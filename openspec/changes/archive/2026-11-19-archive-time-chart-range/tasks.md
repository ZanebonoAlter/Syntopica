# 任务：archive-time-chart-range

> schema `syntopica-ui`；`complexity: complex`；`ui-impact: minor`。动工前读 `design.md` §2/§3 的状态模型与组件契约，别从 `BoardArchiveView.vue` 现场反推。

## 1. 后端：按日计数端点

- [x] 1.1 新建 `backend-go/internal/tagmanagement/handler/board_article_daily_counts_handler.go`：`getBoardArticleDailyCounts`，取 `boardTagIDs`（`topic_tag_board_labels.semantic_board_id`，不过滤标签 status——与列表主查询一致），空集合直接返回 `{earliest_date:null, latest_date:null, days:[]}`，不打库。
- [x] 1.2 聚合查询：`articles a ⋈ article_topic_tags att(topic_tag_id IN boardTagIDs) ⋈ feeds f`，`SELECT DATE(a.pub_date) AS day, COUNT(DISTINCT a.id) AS count ... GROUP BY DATE(a.pub_date) ORDER BY day ASC`；`earliest_date`/`latest_date` 取结果首末行，不另发 MIN/MAX 查询。
- [x] 1.3 响应 DTO 用 snake_case tag：`{success, data:{earliest_date, latest_date, days:[{date,count}]}}`；`days` 稀疏（只含 count>0）。
- [x] 1.4 路由注册 `boards.GET("/:id/article-daily-counts", handler.getBoardArticleDailyCounts)`，紧挨 `/articles`（`board_crud_handler.go`）。
- [x] 1.5 在 handler 注释里写明「成员口径与 `getBoardArticles` 主查询逐字一致」+「忽略 feed_id/auxiliary_label_id/report_status/show_direction_mismatch」两条不变量（口径漂移的唯一防线）。

## 2. 前端：数据层与纯函数

- [x] 2.1 `front/app/api/semanticBoards.ts` 新增 `getBoardArticleDailyCounts(id)` + 导出类型 `BoardArticleDailyCounts`（`earliest_date`/`latest_date`/`days`），挂到 `useSemanticBoardsApi()` 返回对象与 `export type` 区。
- [x] 2.2 新建 `front/app/features/tags/composables/useArchiveDailyCounts.ts`：按 `boardId` 拉取，模块级 `Map<boardId,{payload,at}>` 60s TTL，stale-while-revalidate，错误不覆盖旧值；导出 `{earliest, latest, byDate, loading, error, reload}`。
- [x] 2.3 新建 `front/app/features/tags/components/console/archiveTimeSeries.ts`：`toCountsMap` / `viewportFor(anchorEnd, earliest, upperBound, 90)` / `sliceSeries`（缺日补 **0**，注释写清与 `laneTrendSeries` 的 null 语义相反）/ `shiftViewport`（越界返回 null）/ `shiftRangeWithViewport` / `buildStripSeries`；日期算术复用 `observationRange.ts` 的 `shiftDate`/`daysBetweenInclusive`/`clampRangeToBounds`。
- [x] 2.4 `useBoardArchive.ts`：`normalizeArchiveRange` 的非法回退由宿主本地 `todayLocal()` 改为 `todayInShanghai()`；确认 `archiveWindowRange` 无调用方后删除（`grep -rn "archiveWindowRange" front/`）。

## 3. 前端：复用组件扩展（缺省即现状）

- [x] 3.1 `ObservationRangeControls.vue` 新增可选 props `unitLabel`（默认 `线索数`）、`title`（默认 `时间视窗`）、`collapseStorageKey`（默认 `syntopica.orc.collapsed`），替换硬编码的 `unit-label="线索数"`、两处标题文案与 `COLLAPSE_STORAGE_KEY`。
- [x] 3.2 无曲线提示改为按 `title` 生成（去掉「365 日池」「不补 0」字样——归档口径缺日是已知 0），矩阵默认渲染文案保持等价。
- [x] 3.3 新增具名插槽 `footer`（绘图区/反馈行之后），缺省不渲染任何 DOM；`tiers` 容器加 `v-if="tiers.length"`。
- [x] 3.4 新增/补充 `ObservationRangeControls.test.ts` 用例：不传新 props 时单位文案仍「线索数」、标题仍「时间视窗」、localStorage 键仍 `syntopica.orc.collapsed`、无 footer 时 DOM 无新增节点、传 `tiers=[]` 时不渲染档位分组。

## 4. 前端：导航条组件

- [x] 4.1 新建 `front/app/features/tags/components/console/ArchiveHistoryStrip.vue`（受控纯 UI）：props `earliest`/`latest`/`byDate`/`viewport`/`disabled`，emits `viewport-preview` / `viewport-commit` / `viewport-cancel`。
- [x] 4.2 绘制：全历史密度折线 + 面积（`viewBox 1000×28`、`preserveAspectRatio="none"`），按容器真实宽度分桶（超过 4000 天时降采样），两端 `MM-DD` 标签；视窗窗口矩形 + 2px 边。
- [x] 4.3 手势：窗口内拖 = 平移；窗外按下 = 先居中跳转再继续拖；`setPointerCapture`；Esc/`pointercancel` → `viewport-cancel`；松手 → `viewport-commit`（一次）。
- [x] 4.4 键盘：窗口 `tabindex=0` / `role="slider"` / `aria-valuetext="MM-DD ~ MM-DD"`，←/→ 1 天、Shift+←/→ 7 天、Home/End 到边界（步进同样只 emit commit 一次）。
- [x] 4.5 窄屏与禁用态：`disabled` 时不响应手势但保留轮廓；与主图共用语义色彩 token，不引入新色值。

## 5. 前端：归档视图改造

- [x] 5.1 `BoardArchiveView.vue`：删除 `PRESETS`/`applyPreset`/`isPreset` 与工具行两个 `AppInput type="date"`；工具行只留排序/筛选/⚙/ⓘ。
- [x] 5.2 默认日期改 `initialDate || todayInShanghai()`（删 `asOf` prop），`BoardConsoleStage.vue` 移除 `:as-of` 绑定；`archived` 默认 `committedRange = {start: 基准日, end: 基准日}`。
- [x] 5.3 挂载 `<ObservationRangeControls>`（`bounds=viewport`、`range=draftRange??committedRange`、`sourceDays=viewportDays`、`series=sliceSeries(...)`、`tiers=[]`、`maxSpanDays=viewportDays`、`selectableBounds=fullBounds`、`unitLabel="篇"`、`title="时间范围"`、`collapseStorageKey="syntopica.archive.time.collapsed"`、`caption` 两行），`footer` 插槽注入 `ArchiveHistoryStrip`。
- [x] 5.4 事件接线：`@preview` 只写 `draftRange`；`@commit` 写 `committedRange`（并经 `useBoardArchive` 的 `watch(range)` 重拉）；导航条 `viewport-preview` 同步更新 `anchorEnd` + `draftRange`，`viewport-commit` 同时提交两者，`viewport-cancel` 回滚。
- [x] 5.5 越界跟随：日期表单 commit 落在视窗外时先把 `anchorEnd` 移到 `range.end`（clamp 到 `[earliest, upperBound]`）再写区间，保证 `committedRange ⊆ viewport` 恒成立。
- [x] 5.6 状态：计数加载中 → 主图骨架（日期选择可用）；`earliest=null` → 「本版块还没有命中文章，暂无时间曲线」；失败 → 主图右上「时间曲线加载失败 + 重试」，列表照常。
- [x] 5.7 保证曲线口径固定：`article-daily-counts` 只在 `boardId` 变化时请求，`feedId`/`labelId`/`showDirectionMismatch`/`filter`/`sortMode` 变化 MUST NOT 触发。

## 6. Review（complex 档，前移到实现完成即做）

- [x] 6.1 派 `change-review` 子线程聚焦 review（审查 proposal/ui-design/specs/design/tasks 与最终 diff），意见按 High/Medium/Low 处置后写 `openspec/changes/archive-time-chart-range/review.md` 建账；High/Must 项修完再进归档。——已完成：12 条意见全部处置为「已修复」（R-1 NULL 分组、R-2 时区口径、R-3 不变量失守、R-4/R-5 测试缺口、R-6~R-12 口径与制品对齐）。

## 7. 测试

- [x] 7.1 `backend-go/internal/tagmanagement/handler/board_article_handler_test.go`：新增「按日聚合与列表 total 一致（同文多标签只计一次）」/「跨两日稀疏返回」/「方向不符仍计入」/「空版块 null+[]」四个用例，复用既有 fixture。
- [x] 7.2 新增 `front/app/features/tags/components/console/archiveTimeSeries.test.ts`：`viewportFor`（跨年、不足 90 天贴齐 earliest、上界 clamp）、`sliceSeries` 缺日补 0、`shiftViewport` 越界 null、`shiftRangeWithViewport` 保持宽度、`buildStripSeries` 分桶。
- [x] 7.3 新增 `front/app/features/tags/components/console/ArchiveHistoryStrip.test.ts`：拖动 emit preview 连续 + commit 一次、窗外按下先居中、Esc/pointercancel 走 cancel、键盘 ←/Shift+←/Home/End。
- [x] 7.4 改 `front/app/features/tags/components/console/BoardArchiveView.test.ts`：删 `archive-preset-*` 用例；新增「默认查日历今天」/「initialDate 优先」/「今天无文章显示空态」/「主图 commit 才重拉且参数正确」/「切筛选不重拉计数」/「计数失败不阻塞列表」。
- [x] 7.5 跑受影响包：`cd backend-go && go test ./internal/tagmanagement/handler`、`cd front && pnpm test:unit app/features/tags/components/console/archiveTimeSeries.test.ts app/features/tags/components/console/ArchiveHistoryStrip.test.ts app/features/tags/components/console/BoardArchiveView.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2`。
- [x] 7.6 按 review 意见补测：`useArchiveDailyCounts.test.ts`（TTL 缓存 / 竞态守卫 / reload / 失败保旧值）+ `BoardArchiveView.test.ts`（导航条 Esc 取消、同值 commit 不重拉、计数迟到把区间钳回视窗）+ 后端 `TestBoardArticleDailyCounts_NilPubDate`。

## 8. 文档

- [x] 8.1 `docs/reference/api/semantic-boards.md`：新增 `GET /api/semantic-boards/:id/article-daily-counts` 条目（参数、返回、成员口径、稀疏语义、与 `/articles` 的一致性不变量）。
- [x] 8.2 `docs/reference/flow/semantic-board.md`：更新「L1 归档」行（默认今天、时间导航块形态、删快捷档）与「业务约束与不变量」新增红线（曲线=每日进版块文章数；成员口径与列表逐字一致；不随筛选变化；缺日已知 0）。
- [x] 8.3 `docs/reference/flow/semantic-board.md` 变更溯源表补一行（本 change 归档后回填链接，遵循 §12）。——已回填（指向 `archive/2026-11-19-archive-time-chart-range`）。
- [x] 8.4 检查 `docs/reference/standard/frontend/interaction.md` / `layout.md` 是否需要登记「双层时间导航」——已检查：主图与导航条同属既有 `panel` 内 `canvas` 语义（复用 `ObservationRangeControls` 刷选契约，未引入新交互范式、未改变页面骨架），**两文件均不改**。

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=backend-go 改动里唯一的 AutoMigrate 出现在 handler 测试的 SQLite 内存库 fixture，不是 schema 变更，无 database 文档需改 -->

## 9. 验证

- [x] 9.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/... && go vet ./internal/tagmanagement/...` → 0 issue。
- [x] 9.2 `cd backend-go && go test ./internal/tagmanagement/handler -run 'DailyCounts|BoardArticles' -v` → 新用例全 PASS。
- [x] 9.3 真库只读实测（**不做任何写操作**；端点尚未部署，改用只读 psql + 临时 Go 探针等价实测：board 1980 的 2026-11-13 聚合 298 = `/articles?start_date=2026-11-13&end_date=2026-11-13` 的 `pagination.total` 298）：`curl --noproxy '*' -s "http://localhost:5100/api/semantic-boards/1/article-daily-counts" | head -c 400` → 返回 `earliest_date`/`latest_date`/稀疏 `days`，且把某日 `count` 与 `…/articles?start_date=该日&end_date=该日&per_page=1` 的 `pagination.total` 对齐（两值相等）。
- [x] 9.4 `cd front && pnpm exec nuxi typecheck` → 0 error。
- [x] 9.5 `cd front && pnpm test:unit app/features/tags/components/console/archiveTimeSeries.test.ts app/features/tags/components/console/ArchiveHistoryStrip.test.ts app/features/tags/components/console/BoardArchiveView.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → 全 PASS。
- [x] 9.6 `cd front && pnpm lint` → 0 error。
- [x] 9.7 `cd front && pnpm build` → 构建成功；随后 `bash scripts/dev/deploy-frontend.sh` 部署静态产物并健康检查通——已跑（`deploy-frontend.sh` 内含 `NODE_OPTIONS=--max-old-space-size=3584` + `pnpm generate`；裸 `pnpm build` 在树莓派 2G 堆会 OOM，已在脚本里抬堆）。
- [x] 9.8 手工点检（部署后）：切到「文章归档」→ 默认显示今天 1 天 → 拖选 5 天松手后列表与 deck 同步 → 拖导航条到去年后主图/日期表单/列表一起跟到该区间 → 窄屏展开/收起正常。——已发现：默认今天 ✅（起止 2026-11-19）、deck 136 篇 ↔ 曲线末日 136 ✅、导航条渲染 ✅；导航条平移与拖选受当前库数据限制无法实测（所有版块历史 ≤ 90 天，一屏装得下、无可平移区），该路径由 `archiveTimeSeries` / `ArchiveHistoryStrip` / `BoardArchiveView` 单测覆盖。
- [x] 9.9 `bash scripts/harness/doc-impact.sh verify openspec/changes/archive-time-chart-range` → 无缺失；`bash scripts/harness/check-tasks-tail.sh openspec/changes/archive-time-chart-range` → 通过。
