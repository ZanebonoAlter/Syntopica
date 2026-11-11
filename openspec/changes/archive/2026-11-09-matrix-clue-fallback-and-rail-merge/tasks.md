# 任务 — matrix-clue-fallback-and-rail-merge

## 1. 用例先行（后端）

- [x] 1.1 `backend-go/internal/topicgraph/repository/board_radar_stories_test.go`：新增 `TestRadarStoryFeedTopStoriesLimitEight`——造 9 条热度递减事件线，断言 `top_stories` 恰 8 条、第 9 名不在清单。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'RadarStoryFeedTopStoriesLimitEight' -count=1`
- [x] 1.2 同文件：退役契约测试处置——删除 `TestRadarStoryFeedNewStoriesExcludeTopStories` / `TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered` / `TestRadarStoryFeedNewCountNotAffectedByTrim` / `TestRadarStoryFeedNewTotalCount`（`new_stories`/`new_count`/`new_total_count` 字段移除后无断言对象）；`TestRadarStoryFeedCrossDayStoryCount` / `TestRadarStoryFeedTopStoriesIgnoreParticipantFloor` 保留不动（判据未变）。处置逐行见 test-cases.md ⓪ 表。
- [x] 1.3 同文件：`TestRadarStoryFeedWarmingCountWithoutList`——造 3 条升温线 + 2 条非升温线，断言 `warming_count == 3` 且响应结构无 `warming`/`new_stories` 清单（编译期字段已删，断言计数即可）。验证：`-run 'RadarStoryFeedWarmingCountWithoutList' -count=1`

## 2. 用例先行（前端口径：D2）

- [x] 2.1 `front/app/api/normalizers/laneDynamicsWindow.test.ts`：`dayCellMetrics` 新增 `displayCount`/`degradedFromClue` 断言组——事件线 >0 用事件线数；=0 且线索 >0 兜底线索合计并标降级；=0 且线索 =0 仍 0 不标；null 维持 null；旧响应（raw 缺省）值与现状 `storyCount` 相同。验证：`cd front && pnpm test:unit app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2`
- [x] 2.2 `front/app/features/tags/components/console/laneConsoleView.test.ts`：`buildPoolSeries` 新增断言——`story_count=0` 而线索合计 132 的日子曲线值 132；`story_count=3` 而线索 132 的日子曲线值 3（不混加）；既有三用例（无 story_count 夹具）期望值不变（现状兼容）。验证：`pnpm test:unit app/features/tags/components/console/laneConsoleView.test.ts --maxWorkers=2`
- [x] 2.3 `front/app/features/tags/components/console/observationLanes.test.ts`：`matrixCellView` 新增断言——降级格 text=132、aria 含「0 条事件线，按线索合计兜底」；真零格 text=0 无降级尾注；`?` 语义不变。验证：`pnpm test:unit app/features/tags/components/console/observationLanes.test.ts --maxWorkers=2`

## 3. 用例先行（前端右栏合并 + 折叠：D3/D5）

- [x] 3.1 `front/app/api/normalizers/boardRadar.test.ts`：读模型删 `warming`/`newStories`/`newCount`/`newTotalCount` 断言改写（raw 携带这些字段时归一结果不含它们）；`topStories`/`warmingCount`/`crossDayStoryCount` 断言保留。验证：`pnpm test:unit app/api/normalizers/boardRadar.test.ts --maxWorkers=2`
- [x] 3.2 `front/app/features/tags/composables/useBoardRadarOverview.test.ts`：删三档派生断言；新增 `crossDayBuilding`（=0 → true；>0/null → false）。验证：`pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2`
- [x] 3.3 `front/app/features/tags/components/console/BoardRadarView.test.ts`：右栏合并断言——单清单 8 条、`[新起]`/`[升温 3.2×]` 徽标存在性、`radar-new-block`/升温卡片不再渲染、`cross_day_story_count=0` 时脚注、`>0` 无空态、null 无脚注；旧断言（新起三态/升温卡片）删除。验证：`pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2`
- [x] 3.4 同文件：雷达小矩阵降级断言——`storyCounts` 某日 0 而 `counts` 4 时该格 title 含「无事件线（按分区数 4 显示）」；正常格 title「N 条事件线」不变。
- [x] 3.5 `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`：折叠断言——默认折叠一行摘要（五项事实不渲染）；点击展开；异常（过点未出日报/失败任务/phase failed/积压 ≥100）自动展开；异常消除回折叠；读取中摘要不判「未生成」。既有五项事实用例改为先展开再断言。验证：`pnpm test:unit app/features/tags/components/console/BoardRadarStatusBlock.test.ts --maxWorkers=2`

## 4. 后端实现（D4）

- [x] 4.1 `backend-go/internal/topicgraph/repository/board_radar_repository.go`：`radarTopStoryLimit` 5→8；删 `radarWarmingLimit`/`radarNewStoryLimit` 常量、`RadarStoryFeed.Warming`/`NewStories`/`NewCount`/`NewTotalCount` 字段与 `finalizeRadarStories` 的清单产出/去 Top 互斥/计数逻辑（`WarmingCount` 改循环内纯计数，`CrossDayStoryCount` 统计不变）。
- [x] 4.2 验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1 && go test ./internal/topicgraph/handler/ -run 'Radar' -count=1`

## 5. 前端实现（D1/D2/D3/D5）

- [x] 5.1 `front/app/api/normalizers/laneDynamicsWindow.ts`：`DayCellMetrics` 增 `displayCount`/`degradedFromClue`（design D2 公式），既有字段语义零改动。
- [x] 5.2 `front/app/features/tags/components/console/observationLanes.ts`：`matrixCellView` text 与 title/aria 用降级口径（非降级措辞不变）。
- [x] 5.3 `front/app/features/tags/components/console/laneConsoleView.ts`：`poolDayTotal` 改用 `displayCount`（含跳过/守卫语义迁移，design D2）。
- [x] 5.4 `front/app/features/tags/components/console/ObservationRangeControls.vue` + `TrendAxes.vue`：曲线标注两行与 `unitLabel` 改口径文案。
- [x] 5.5 `front/app/features/tags/components/console/BoardRadarStory.vue`：`laneCounts` 改逐格降级 + `cellTitle` 区分降级（design D1）。
- [x] 5.6 `front/app/features/tags/components/console/BoardRadarReadingRail.vue`：结构重排为单清单 + 徽标 + 图例 + 脚注；删升温/新起两卡片（storyMode 分支），旧响应回退分支原样。
- [x] 5.7 `front/app/features/tags/composables/useBoardRadarOverview.ts` + `front/app/api/normalizers/boardRadar.ts`：删三档派生与对应读模型字段；增 `crossDayBuilding`。
- [x] 5.8 `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`：折叠状态机 + 摘要行（design D5）；`BoardRadarView.vue` 传参清理。
- [x] 5.9 验证：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/api/normalizers/laneDynamicsWindow.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/BoardRadarStatusBlock.test.ts --maxWorkers=2 && pnpm exec nuxi typecheck && pnpm lint`

## 6. 测试

- [x] 6.1 `cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1 && go test ./internal/topicgraph/handler/ -run 'Radar' -count=1`
- [x] 6.2 `cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/api/normalizers/laneDynamicsWindow.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/laneConsoleView.test.ts --maxWorkers=2`
- [x] 6.3 先红后绿证据：新增用例先于实现提交运行留红（后端 1.1/1.3 编译或断言红；前端 2.1～3.5 断言红），实现后全绿，证据贴进报告。

## 7. 文档

<!-- doc-impact: api, flow -->

- [x] 7.1 `docs/reference/api/daily-reports.md`：`stories` 段改写——Top N 上限 8、字段表删 `warming`/`new_stories`/`new_count`/`new_total_count` 四行、徽标说明（`has_baseline`/`ratio` 随条目）、JSON 示例同步；端点清单行同步。
- [x] 7.2 `docs/reference/flow/semantic-board.md`：L0 雷达段改写右栏结构（单清单 + 徽标 + 脚注 + 管线折叠 + 小矩阵降级）；红线 28 增展示降级条款、红线 29 增单清单徽标化条款（判定不变）；代码入口行同步（截断 8）；变更溯源加行。
- [x] 7.3 验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/matrix-clue-fallback-and-rail-merge`

## 7b. Review

- [x] 7b.1 派只读子线程做 review（审查制品 + diff），重点：两套兜底口径的文案是否可能被误读（R1）、演进带与矩阵格数字差异是否可接受（R3）、折叠异常判据与生成闸门是否解耦（R4）、删字段后新旧端混布安全性（R5）；发现记入 `review.md` 并逐条处置。

## 8. 验证

- [x] 8.1 `cd backend-go && go build ./... && go vet ./... && golangci-lint run ./internal/topicgraph/... && gofmt -l internal/topicgraph/`
- [x] 8.2 `cd front && pnpm exec nuxi typecheck && pnpm lint`（不跑 `pnpm build`/`generate`，构建由主线程统一做）
- [x] 8.3 `openspec validate matrix-clue-fallback-and-rail-merge --strict && bash scripts/harness/scenario-trace.sh openspec/changes/matrix-clue-fallback-and-rail-merge && bash scripts/harness/check-tasks-tail.sh openspec/changes/matrix-clue-fallback-and-rail-merge`
- [x] 8.4 效果核对（真库只读）：小矩阵降级前后空格子数 62 → 0（`GET /api/semantic-boards/overview` 数格子）；折线口径与格值一致性抽查。

| Scenario | 测试文件 |
| --- | --- |
| 小矩阵历史未回填日按分区数兜底 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 今日该读按事件线跨版块混排 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 排序只按热度与来源数 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 清单上限 8 且不再有独立新起档 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 今日新起单列且不显示倍率 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 升温判定 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 分区多但事件少不升温 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 无升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 无跨天延续事件线时的空态 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 旧响应缺跨天线计数时不冒充无跨天线 | front/app/features/tags/composables/useBoardRadarOverview.test.ts |
| 今日新起与该读互斥 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 新起全被该读覆盖时说明已列入该读 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 新起超出列出上限时附剩余计数摘要 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 该读不按来源数硬过滤 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 今日按各版块自己的锚点日而非自然今天 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 锚点日取该版块自己的最新日报日而非泳道轴末日 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 滞后版块显示自己的日期 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 未回填版块不冒充无事件 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 部分版块延迟时排序不完整明示 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 单版块事件线计算失败同样明示排序不完整 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 徽标判定信息随条目自带 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 同日分桶不冒充分区热度 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 矩阵格值是桶峰值 | front/app/features/tags/components/console/observationLanes.test.ts |
| 历史未回填日按线索合计兜底 | front/app/features/tags/components/console/observationLanes.test.ts |
| 真零事件不兜底成非零 | front/app/features/tags/components/console/observationLanes.test.ts |
| 曲线与矩阵格同口径 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 首屏曲线即含历史 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 状态块常驻右栏底部 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts front/app/features/tags/components/console/BoardRadarView.test.ts |
| 默认折叠成一行摘要 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 异常态自动展开 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 状态块展示五项事实 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 首次对账未成功不呈现业务结论 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 对账失败保留旧值 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 总览取数失败不判未生成 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |
| 颜色表达参与者数 | front/app/features/tags/components/console/observationLanes.test.ts |
| 多线格点名获胜线 | front/app/features/tags/components/console/observationLanes.test.ts |
| 来源同分稳定点名 | front/app/features/tags/components/console/observationLanes.test.ts |
| 无身份不伪造标题 | front/app/features/tags/components/console/observationLanes.test.ts |
| 空标题与零来源 | front/app/features/tags/components/console/observationLanes.test.ts |
| 拖选不重排 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 精确档回切 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 明确切独立档 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 档池分层失败 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 池未取到有提示可重试 | front/app/features/tags/components/console/BoardConsoleShell.test.ts |
| 池失败后切 365 档不残留误述 | front/app/features/tags/components/console/BoardConsoleShell.test.ts |
| 非法日期反馈 | front/app/features/tags/components/console/ObservationRangeControls.test.ts |
| 深链恢复 | front/app/middleware/tags-deep-link.test.ts front/app/features/tags/components/console/BoardConsoleShellNavigation.test.ts |
| 热点行带理由与密度 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 常温版块折叠 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 关注度不因分区数虚高 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 一次请求渲染雷达 | front/app/features/tags/composables/useBoardRadarOverview.test.ts |
| 无数据版块 | backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go |
| 事件线清单随行同响 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
