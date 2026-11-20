# Tasks: 版块态势页左右轨改为实时信号

> 前置：`ui-design.md` 的 `ui-approval` 由用户批准后置 `approved` 才可开工（ui-design-gate 硬拦截）。
> 后端零改动。
>
> **实现进度（2026-11-20 子线程）**：第 1–4 节全部完成（前端部分，后端零改动，零新增请求）；
> 5.1 已跑 lint / typecheck / 受影响范围单测（`pnpm build` 按主线程要求留给主会话统一跑）；
> 5.2 需先有构建产物（静态托管走 `deploy-frontend.sh`），留给主会话；5.3 / 5.4 / 第 6 节本线程不做。

## 1. 读模型（纯函数先行）

- [x] 1.1 新增 `front/app/features/tags/components/console/railClueAxis.ts`：`buildRailBoardRows(boards, overviewRows, globalDate)` —— 按 `boards` 顺序 join overview 行、**共同轴重对齐**（`位置 = i - offset`，`offset = daysBetween(globalDate, board.anchor_date)`）、峰值 / 强度计算、null（未取到）语义（design D2/D3）。
- [x] 1.2 新增 `railClueAxis.test.ts`：offset 0 / 3 / ≥14 三种情形；join 顺序不等于 overview 顺序；未取到整行斜纹；峰值与强度计算；`data_unavailable` 行。
- [x] 1.3 扩展 `front/app/api/normalizers/laneDayReading.ts`：`normalizeReportDetail` 补 `title` / `summary` / `highlights`（含 `cluster_id` / `article_count` / `cluster_mode` / `data_as_of`）；**不改后端协议、不改其他消费者**。
- [x] 1.4 新增 `front/app/features/tags/components/console/nowBrief.ts`：`buildNowBriefModel(reportDetail, selectedDate, laneDynamicsLanes)` —— 点评层（highlights 原序，不重复 title/summary）+ 泳道层（`topic_status_at_report === 'active'` 且有 `persistent_topic`；句子取 `threads[0].title` 否则 `cluster_label`，两者皆无不占位；线索数取 lane-dynamics 该泳道**选中日**的 `clue_count`（与矩阵日格同值，取不到 = null）；**按线索数降序**，并列保持日报原序，`null` 排末位；只保留能在 lane-dynamics 里按 topic_id 找到的正式泳道）。
- [x] 1.5 新增 `nowBrief.test.ts`：不重复日报标题；`article_count` 缺省不显示；candidate 分区排除；thread 优先于 cluster_label；无 thread 无 label 不出该行；**排序**：线索数 1/4/2/0 → 输出 4/2/1/0；并列保持日报原序；`null` 排末位；lane-dynamics 里找不到的 topic_id 不进右栏。
- [x] 1.6 `useConsoleShellData.ts`：把 `loadReportMeta` 内的 overview 行提升为共享状态（`overviewRows` / `overviewGlobalDate` / `overviewError`），新增 `railRows` computed；**不新增请求**，失败不阻塞 `boards` 与 `watches`。

## 2. 左轨（版块栏）

- [x] 2.1 `BoardRail.vue` props 改为 `rows: RailBoardRow[]`（保留 `selectedBoardId` / `loading` / `error` / `allowAddBoard` / emit；另加 `axisDates` / `signalError`）。
- [x] 2.2 模板重写为一行式：名称（`min-width: 0` + ellipsis）+ 右侧 14 格条码；`title` 带 `tag_count`、锚点日与逐日明细；**不渲染任何数字、不渲染口径说明文字**。
- [x] 2.3 样式：复用 `clueDepth` / `clueMix`；未取到格 = 斜纹占位；行整体强度按 14 天总量分档（opacity 0.42–1）；副标题只留「语义板块」与计数。
- [x] 2.3b 栏体满高自适应（design D9b）：左轨 `height: 100%` + 纵向滚动 + 极淡底色；行高弹性 24–40px（`flex: 1 1 auto` + `max-height: 40px`）+ 行间极淡分隔线；行内名称与条码垂直居中。
- [x] 2.4 动效：入场逐列点亮（`--i` delay，≤420ms）、仅选中行当日格脉动、选中行左缘线 `scaleY` 生长；全部 `prefers-reduced-motion: reduce` 关停（design D10）。
- [x] 2.5 加载骨架改为「名称灰条 + 14 格灰格」，沿用 `rail-pulse`。
- [x] 2.6 新增 `BoardRail.test.ts`：14 格数量、斜纹格、真实 0 与未取到的区别、选中态、`title` 含 `tag_count`、`allowAddBoard` 行为不变。
- [x] 2.7 接线：`BoardConsoleBody.vue` 传 `railRows`；`consoleStageBinding.ts` / 壳层 props 类型同步。

## 3. 右轨（上下文栏）

- [x] 3.1 `RailHealthBrief.vue`：`brief` 变体移除评语首句，保留徽章 / 体检时间 / 刷新 / stale / 错误重试；`full` 变体不动。
- [x] 3.2 新增 `RailNowBrief.vue`：两层（点评行 + 泳道行），收 `NowBriefModel`，不发请求；泳道行显示泳道名 + **当日线索数**（`null` → `—`，不补 0）+ 一句代表线索；点击点评（有 `cluster_id`）emit 定位正文；点击泳道行 emit `locateLane(topicId)`；空态 / 未取到 / 无日报文案按 design D5/D6 与 spec。
- [x] 3.3 `BoardContextRail.vue`：区块顺序改为 体检 → 当日日报 → 话题追踪 → 构成候选 → 来源构成；挂载 `RailNowBrief`，无新增网络。
- [x] 3.3b 右轨满高自适应（design D9b）：右轨 `height: 100%` + 纵向滚动 + 极淡底色；纵向 flex，「当日日报」块 `flex: 1 1 auto`（内容顶部对齐、块内不滚、不得被压扁），其余四块 `flex: 0 0 auto` 贴底。
- [x] 3.4 更新 `BoardContextRail.test.ts`：顺序断言、泳道行不显示条数、candidate 分区不出现、点击事件上抛。
- [x] 3.5 `railContext.ts` / `useConsoleShellRail.ts`：`RailBinding` 增加 `now` 模型与 `locateLane` / `openReportCluster` 转发；确认「当日日报」读的是**选中日**那份日报详情（复用 `useConsoleReading` 已有请求，零新增请求）。

## 4. 矩阵选中列淡底

- [x] 4.1 `ObservationMatrixGrid.vue` / `ObservationMatrixDayHeader.vue` / `observationMatrixRows.css`：选中列整列淡底（`color-mix` 低浓度 `background`），仅在有分区的报日铺底，无分区历史日不铺；不改列头高亮、不动格子色。
- [x] 4.2 既有矩阵测试内补淡底断言（选中 / 未选中 / 无分区日三种）。

## 4b. Review

- [x] 4b.1 派 `change-review` 子线程聚焦 review（制品 proposal/design/spec delta/ui-design + 实现 diff），意见按 High/Medium/Low 处置后 `review.md` 建账。
      → 审出 0 High / 4 Medium / 6 Low；Medium 全修（R-1 色点改用后端稳定色、R-2 flow 措辞同步、R-3 补矩阵淡底 spec delta、R-4 日期闸门下沉纯函数+补测）；Low 修 5 条、驳回 1 条（R-8 跨模块搬迁延后）。处置后 5 文件 73 tests 全绿。详见 `review.md`。

## 5. 文档

<!-- doc-impact: flow=左右轨口径与共同轴对齐规则落在 flow/semantic-board.md 与 flow/daily-report.md；api=两个既有端点的消费方与对齐 MUST 落在 api/daily-reports.md -->

- [x] 5.1 `docs/reference/flow/semantic-board.md`：左轨条码口径与**共同轴对齐**规则；右轨「当日日报」节；§12 变更溯源加本 change。
- [x] 5.2 `docs/reference/flow/daily-report.md`：`highlights` 与 `title`/`summary` 的派生关系（title/summary 取首条重点）——右栏不得重复展示的依据。
- [x] 5.3 `docs/reference/api/daily-reports.md`：`GET /daily-reports/:id` 节补「L1 版块态势页右轨消费方」；`GET /semantic-boards/overview` 节补左轨消费方与共同轴对齐说明。
- [x] 5.4 `bash scripts/harness/doc-impact.sh verify` + `bash scripts/harness/check-standards.sh`。
      → doc-impact 通过（声明 `flow api`）；check-standards 318 通过 / 0 失败（全绿）；其间补了已归档 change `2026-11-17-fix-lane-row-gap-zoom` 缺的一行流程溯源（纯文档、归档门禁硬要求）。
- [x] 5.5 归档前满足《开发执行规范》§11 门禁（含 §11.1-⑤ UI 验收证据）。
      → §11.1-⑤ 验收证据已建账（`ui-verification/report.md`，A-1~A-9 全 pass + 两处验收后微调复测）。

## 6. 测试

<!-- doc-impact: flow=测试口径复用既有 clueDepth/clueMix 与 dayCellMetrics，不新增量纲；api=不改协议只补前端归一字段 -->

- [x] 6.1 `railClueAxis.test.ts`（11 例）：共同轴重对齐 offset 0 / 3 / ≥14、join 顺序不等于 overview 序、未取到整行斜纹、峰值与强度计算、`data_unavailable` 行。
- [x] 6.2 `nowBrief.test.ts`（17 例）：不重复日报标题、`article_count` 缺省不显示、candidate 分区排除、thread 优先于 cluster_label、无 thread 无 label 不出行、**排序 1/4/2/0 → 4/2/1/0**、并列保持日报原序、null 末位、lane-dynamics 里查不到的 topic 不进右栏。
- [x] 6.3 `BoardRail.test.ts`（9 例）：14 格数量、斜纹 vs 真实 0、选中态、`title` 含 `tag_count`、`add` 入口行为不变。
- [x] 6.4 `BoardContextRail.test.ts`（23 例）：右轨顺序、当日日报块存在、泳道行不并排两套量纲、体检徽章即评语入口（BUTTON + `openHealth`）。
- [x] 6.5 `ObservationMatrixLaneRow.test.ts` / `ObservationMatrix` / `ObservationMatrixGrid`：选中列淡底三态（选中 / 未选中 / 无分区日不铺）。
- [x] 6.6 受影响文件全绿：`pnpm test:unit <10 个文件> --maxWorkers=2` → 10 files / 136 tests 全通过；收尾微调后复测 `BoardContextRail + BoardRail` 32 tests 全通过。
- [x] 6.7 存量红测试登记：`LaneTrendChart.test.ts` 2 例（断言旧文案「事件线数」，HEAD 基线即红、非本 change 引起）已 `test-patrol.sh --register fe-lane-trend-chart-wording` 记账。

## 7. 验证

<!-- doc-impact: flow=验收断言以 flow 的线索口径与「未取到≠0」为判据；api=数值一致性经后端 lane-dynamics 逐条对账 -->

- [x] 7.1 `pnpm lint`（0 error，7 warning 全为存量非本 change 文件）/ `pnpm exec nuxi typecheck`（exit 0）/ `pnpm build`（需 `NODE_OPTIONS=--max-old-space-size=3584`）。
- [x] 7.2 `openspec validate console-rails-live-signals --strict` → valid（5/5 artifacts）。
- [x] 7.3 `bash scripts/harness/archive-readiness.sh console-rails-live-signals` 全绿。
- [x] 7.4 agent-browser 主链路（1440×900 + 1920×1080 两档）：左轨条码 → 换版块 → 右轨当日日报 → 点列头换日（右栏跟随 + 列淡底）→ 点泳道行定位。
- [x] 7.5 **数值一致性对账**（核心口径）：右栏 `5/3/2/1 条` = `GET /lane-dynamics` 11-19 的 `5/3/2/1` = 矩阵 11-19 列 → 三者同源同值。
- [x] 7.6 `ui-verification/report.md` 建账：A-1~A-9 全 pass + 两处验收后微调（名称截断、体检行宽度）复测记录；截图 `1440x900.png` / `1920x1080.png` 在盘。
