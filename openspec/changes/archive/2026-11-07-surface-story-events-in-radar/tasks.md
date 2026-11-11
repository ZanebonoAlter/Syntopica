# Tasks

> 本阶段仅完成制品，以下实现/验收任务均未执行。不修改实现或 `docs/reference/**`、不提交。执行前先评审 design、test-cases、ui-design。

## 1. 模型与契约

- [x] 1.1 在 `backend-go/internal/topicgraph/repository/board_radar_repository.go` 增 `RadarStoryFeed` / `RadarStoryItem` 结构与 `RadarOverview.Stories` 字段（snake_case wire 契约，字段名逐字对齐 design.md「数据形状（wire 契约）」节）；验证：`cd backend-go && go build ./...` 通过，`openspec validate surface-story-events-in-radar --strict` 仍 valid。
- [x] 1.2 在 `front/app/api/normalizers/boardRadar.ts` 增 `BoardRadarRawStoryFeed` / `RadarStoryFeed` / `RadarStoryItem` 与 `normalizeRadarStoryFeed`（camelCase + 字符串 ID；缺字段安全默认，旧响应无 `stories` 段时返回空清单而非报错）；验证：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts --maxWorkers=2` 通过。
- [x] 1.3 apply 前跑 `bash scripts/harness/doc-impact.sh suggest` 与 `bash scripts/harness/doc-impact.sh context`，核对本 change 声明的 `flow, api` 两域与实际改动文件一致，把核对结论写进实现记录。

## 2. 后端

- [x] 2.1 在 `backend-go/internal/topicgraph/repository/story_heat.go` 给 `storyHeatIdentity` 增 `Title` 字段并让 `loadBoardHeatEvidence` 的 `board_stories` 查询附带 `title`（附加字段，不改热度计算、不改归并解析、不改参与者去重）；验证：`cd backend-go && go test ./internal/topicgraph/repository -run 'StoryHeat|Radar'` 通过。
- [x] 2.2 新增 `backend-go/internal/topicgraph/repository/board_radar_stories.go`：`RadarBoardStoryPoints(boardID uint, anchor time.Time)`——内部只调用既有 `loadBoardHeatEvidence`（窗口 `[E0-5d, E0]`）与 `boardHeatEvidence.compute(at)`（`E0..E3` 四个 CST 日终），产出当日热度、近 3 日基线（不含 E0）、来源数、当日证据标记；**不得**新增指数衰减公式、不得改 `ComputeBoardStoryHeatByDay` 的返回值语义；验证：新文件单测通过 + `go test ./internal/topicgraph/repository -run 'StoryHeat|Radar'` 仍绿。
- [x] 2.3 在同文件实现 `RadarStoryReportDates(boardID uint, anchor time.Time, candidates []uint)`：按存活身份一跳归一，取 R 之前最近一期含该事件线 body thread（`scope='single'`、`display_tier='body'`、报告状态已完成）的 `period_date`；查不到返回空（不造落点）；验证：新文件单测覆盖「命中/未命中/合并后旧 id 命中」三例。
- [x] 2.4 在 `backend-go/internal/topicgraph/repository/board_radar_repository.go` 增 `radarGlobalAnchor`（一次聚合取活跃版块 `MAX(period_date)`）并把该锚点作为 `finalizeRadar` 的锚点输入；`GetBoardRadarOverview` 在每版块并发扇出内调用 2.2/2.3，扇出后做跨板归并：排序键 `heat desc → participants desc → story_id asc`、今日该看 Top 5、今日升温 Top 5（倍率 ≥ 2 且当日有证据）、今日新起 Top 3（无基线且当日有证据）、未回填版块计数、`partial` / `degraded_board_ids`；保持既有 5s 预算与 `data_delayed` 降级语义；验证：`cd backend-go && go test ./internal/topicgraph/repository -run Radar` 通过，且新增用例覆盖跨板混排、锚点日口径、事件线级升温与新起、未回填降级、超时 partial。
- [x] 2.5 核对 `backend-go/internal/topicgraph/handler/board_radar_handler.go` 无需改动（5s ctx 既有），确认 `backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go` 断言仍绿；验证：`cd backend-go && go test ./internal/topicgraph/handler` 通过。

## 3. 前端

- [x] 3.1 在 `front/app/features/tags/composables/useBoardRadarOverview.ts` 增 `topStories` / `warmingStories` / `newStories` 派生与 `openStory` 跳转（`/tags/{boardId}/{reportDate}?story={storyId}`；条目无 `report_date` 时不跳）；报头「N 处出现升温」改用服务端 `warming_count`；保留旧响应（无 `stories` 段）回退到既有版块级列表的分支；验证：`cd front && pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2` 通过。
- [x] 3.2 改 `front/app/features/tags/components/console/BoardRadarReadingRail.vue`：「今日该读」渲染事件线（版块名 + 事件线标题 + 来源数 + 热度），「今天可能升温」渲染事件线级倍率与自身理由句，并加「今日新起」子档（不带倍率）；补未回填标注、排序不完整标注与空态文案；复用既有 `.rd-read*` / `.rd-warm*` 样式与主题 token；验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` 通过。
- [x] 3.3 在 `front/app/features/tags/components/console/BoardRadarView.vue` 接线 3.1/3.2 的 props 与 `openStory`，报头计数改用 `warming_count`；验证：`cd front && pnpm exec nuxi typecheck` 通过。
- [x] 3.4 在 `front/app/features/tags/components/daily-report/DailyReportDeskShell.vue` 读 `route.query.story` 并下传，在 `front/app/features/tags/components/DailyReportReader.vue` 复用 `handleLocateHighlight` 的既有定位路径（展开 → 滚动 → 聚焦 → 高亮 2s）：本期正文无该事件线时不滚动、提示「本期正文暂无对应事件」；翻页/切版块时该参数被清掉；验证：`cd front && pnpm test:unit app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` 通过。

## 4. 测试

- [x] 4.1 按 `test-cases.md` §2「继承与调整」表逐条处置旧契约：`front/app/features/tags/composables/useBoardRadarOverview.test.ts`、`front/app/features/tags/components/console/BoardRadarView.test.ts`、`front/app/features/tags/components/console/boardRadarHelpers.test.ts` 里「版块级升温」断言改写为事件线级（先证旧断言被改写，再补新断言）；验证：`cd front && pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2` 通过。
- [x] 4.2 在 `backend-go/internal/topicgraph/repository/board_radar_repository_test.go` 增隔离 PG 用例：跨板混排与排序键、锚点日口径（自然今天 ≠ 锚点日）、事件线级升温（自身基线）与今日新起（无基线不显示倍率）、当日无证据不列升温、未回填版块不产生条目、`data_delayed` 时 `partial=true`；仓储测试只用一次性 PostgreSQL/testcontainer，禁止 SQLite、禁止写真库；验证：`cd backend-go && go test ./internal/topicgraph/repository -run Radar` 通过。
- [x] 4.3 在 `front/app/features/tags/components/console/BoardRadarView.test.ts` 增三档渲染与标注用例，在 `front/app/features/tags/composables/useBoardRadarOverview.test.ts` 增旧响应回退用例，在 `front/app/features/tags/components/DailyReportReader.test.ts` 增 `?story=` 定位与失去目标用例；验证：同 4.1 的命令 + `pnpm test:unit app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` 通过。
- [x] 4.4 人工：实施后用 agent-browser 访问静态 :5100 走完整故事（雷达右栏点事件线 → 日报定位到正文 thread），另检 1440×900、1920×1080 与 390×844 换行不溢出，证据建账本 change `ui-verification/report.md`；无证据不得勾选。（**待补**：390×844 窄屏实测样本缺失——1440/1920 两档已有证据，窄屏由主线程补实测后勾选）

## 5. 文档

<!-- doc-impact: flow, api -->

- [x] 5.1 实现阶段更新 `docs/reference/flow/semantic-board.md`：改写红线 29 的「今天可能升温」条款为事件线级（≥2× 阈值保留、热度来源仍只从 `story_signals` 派生、事件线数下限换成「锚点日当天有报道证据」），并补雷达右栏事件线口径与未回填标注。（已改：红线 29/30 改写 + 雷达事件线清单 / 雷达右栏代码入口）
- [x] 5.2 实现阶段更新 `docs/reference/flow/daily-report.md`（`?story=` 深链定位契约与「事件/事件线/话题/分区」术语对齐）与 `docs/reference/api/` 下的 overview 接口文档（`stories` 段字段、锚点日、降级标记）；实施前核对实际文档落点，不另建平行权威文档。（已改：`flow/daily-report.md` 的 `?story=` 深链契约；`api/daily-reports.md` 的 `stories` 段字段表、锚点日/测量时刻、降级标记）
- [ ] 5.3 归档后按规范补两域 flow 的永久 archive 变更溯源链接；完成报告写部署影响 / 所需操作 / 旧数据降级，无自动历史回填。

## 5b. Review

- [x] 5b.1 派只读子线程做 review（审查制品 + diff），重点：跨板归并落点与 5s 预算、锚点日口径、未回填与超时降级、`?story=` 寻址与失去目标、基线口径改写对红线 29 的影响；意见按 High/Medium/Low 处置后在本 change 建 `review.md`；验证：`bash scripts/harness/review-trace.sh openspec/changes/surface-story-events-in-radar` 退出码 0。

## 6. 验证

- [x] 6.1 `openspec validate surface-story-events-in-radar --strict` → 输出 valid。
- [x] 6.2 `bash scripts/harness/scenario-trace.sh openspec/changes/surface-story-events-in-radar` → Scenario 映射齐全（退出码 0）。
- [x] 6.3 `bash scripts/harness/change-scope.sh` → 机械判定影响包；只测影响包，禁止顺手全量。
- [x] 6.4 `cd backend-go && go test ./internal/topicgraph/repository ./internal/topicgraph/handler` → 通过（隔离 PostgreSQL/testcontainer，不用 SQLite，不写真库）。
- [x] 6.5 `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/api/normalizers/boardRadar.test.ts app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` → 定点测试通过（参数不带 `--`）。
- [x] 6.6 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` → 全绿（串行，不与测试/浏览器并行）；实现部署后 `bash scripts/dev/deploy-frontend.sh` → 健康检查通过且静态 :5100 可见新行为。
- [x] 6.7 `bash scripts/harness/doc-impact.sh verify && bash scripts/harness/check-standards.sh` → reference 对账与标准校验通过（归档前）；`git diff --cached --name-only` → 本 change 无暂存；`git diff --check -- openspec/changes/surface-story-events-in-radar` → 无空白错误。
- [x] 6.8 人工：按 `test-cases.md` §4 效果核对记录真实样本数、字段覆盖率与结论（未回填版块占比、事件线清单条数分布、升温/新起档条数），不执行写库 SQL 探测。

| Scenario | 测试文件 |
|---|---|
| 今日该读按事件线跨版块混排 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go, front/app/features/tags/components/console/BoardRadarView.test.ts |
| 排序只按热度与来源数 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 今日按锚点日而非自然今天 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 今日按各版块自己的锚点日而非自然今天 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 滞后版块显示自己的日期 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 升温判定 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go, front/app/features/tags/components/console/BoardRadarView.test.ts |
| 分区多但事件少不升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 无升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 今日新起单列且不显示倍率 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 未回填版块不冒充无事件 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 部分版块延迟时排序不完整明示 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 一次请求渲染雷达 | front/app/features/tags/composables/useBoardRadarOverview.test.ts |
| 无数据版块 | backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go |
| 事件线清单随行同响 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 点击第一条可见正文线程 | front/app/features/tags/components/DailyReportReader.test.ts |
| 失去目标与旧重点 | front/app/features/tags/components/DailyReportReader.test.ts |
| 重复定位与减弱动画 | front/app/features/tags/components/DailyReportReader.test.ts |
| 展示筛选不改变热度 | backend-go/internal/topicgraph/repository/report_story_highlights_test.go |
| 深链参数定位与失去目标 | front/app/features/tags/components/DailyReportReader.test.ts |
| 完整故事：雷达事件线 → 日报定位 | 人工：agent-browser 静态 :5100，证据写本 change ui-verification/report.md |
| 锚点日取该版块自己的最新日报日而非泳道轴末日 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 单版块事件线计算失败同样明示排序不完整 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
