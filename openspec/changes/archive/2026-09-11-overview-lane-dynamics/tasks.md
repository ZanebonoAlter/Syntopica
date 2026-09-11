# Tasks: overview-lane-dynamics

<!-- doc-impact: flow, api, database -->

## 文档（归档前）

- [x] D1 `docs/reference/flow/daily-report.md` 补泳道动态结算链路（日报管线尾部结算钩子）与变更溯源；`docs/reference/api/` 新端点 + 删 topic-landscape；`docs/reference/database/` 新表 topic_lane_snapshots；`docs/reference/architecture/map.md` 索引同步（实改：daily-report.md 新增「泳道态势结算」节+代码入口+溯源；semantic-board.md 态势版图节替换为泳道动态+退役说明+溯源；api/daily-reports.md 索引行+端点节替换为 lane-dynamics；database/tables/_conventions.md FK 表+daily-report-watch.md ER 图加 topic_lane_snapshots；architecture/map.md 日报行）

## 1. 后端存储与结算

- [x] 1.1 新表 `TopicLaneSnapshot` model（persistent_topic_id UNIQUE + FK ON DELETE CASCADE、rolling_summary、as_of_date）+ AutoMigrate 注册。验证：`go test ./internal/...`（相关包）迁移用例绿 + testcontainer PG 建表断言
  - 实现于 repository/daily_report_models.go（模型）+ daily_report_register_models.go（注册）+ platform/database/postgres_migrations.go 迁移 20260910_0001（FK ON DELETE CASCADE，AutoMigrate 全局禁建 FK，沿 topic_watch_hits 先例）；验证：internal/platform/database/lane_snapshot_fk_migration_test.go（AutoMigrate 无 FK 前置断言 + 孤儿清理 + 级联删除 + 幂等）与 repository/lane_snapshot_repository_test.go（建表/唯一索引/级联）均绿
- [x] 1.2 态势结算函数：输入泳道近 14 天「日期+section 标题+前3条 thread 标题」素材（窗口锚定 MAX(period_date)），airouter 单次 LLM（operation=daily_report.lane_snapshot，写 AICallLog）产出 ≤100 字态势句，upsert 快照（as_of=最新报告期）。验证：service 单测——素材拼接形状 + 快照 upsert 幂等（重复结算覆盖同行）
  - 实现于 service/lane_snapshot.go（laneSnapshotChatFn 可注入，CapabilityDigestPolish，≤100 字双重约束：prompt + truncateRunes 机械截断）；repository/lane_snapshot_repository.go（素材两段式查询 + upsert ON CONFLICT 覆盖）；验证：service/lane_snapshot_test.go（素材形状、窗口含14天前不含15天前、幂等覆盖、截断100 rune、失败继续兄弟泳道）+ repository PG 测试（素材前3条 thread、窗口边界）均绿
- [x] 1.3 日报管线挂点：GenerateAndSaveReport 成功后 detached goroutine 逐泳道结算（串行、单泳道 60s timeout、活跃泳道 clamp 20 按 last_seen_date 降序、recover 全吞记日志）。验证：单测——结算 panic/失败不影响报告落库返回；clamp 截断记日志
  - 挂点在 service/daily_report_watch.go GenerateAndSaveReport 尾部（EvaluateWatchHits 之后）；验证：TestSettleLaneSnapshotsSafe_PanicDoesNotAffectReport（注入 panic，报告行原样保留）、TestSettleBoardLaneSnapshots_ClampActiveLanes（25 活跃泳道 → 恰 20 次结算，超出者无快照，Warn 日志）均绿

## 2. 后端聚合端点

- [x] 2.1 `GET /semantic-boards/:id/lane-dynamics?days=14`：单实现查询返回 lanes（active ∪ watch-linked，watch_linked 经 BoardTopicWatch.PersistentTopicID 判定；含 snapshot 或 null、timeline 日期→section→前5条 thread 标题+折叠计数、section_count_14d 排序）+ candidates（FilterVisibleTopics 口径 + 最新 section 标题 hint）。验证：handler/repository 测试（testcontainer PG）——活跃排序、沉寂排除、watch 标识、待结算 null、候选门槛
  - 实现于 repository/lane_snapshot_repository.go GetBoardLaneDynamics + handler/lane_dynamics_handler.go（路由挂 RegisterDailyReportRoutes）；响应含 D4 全部字段 + 前端补充的顶层 has_reports 与 section 级 folded_count；验证：repository PG 测试 FullMatrix（排序/watch 角标/沉寂排除/待结算 null/候选门槛/7 线索折叠除 5+2）、WindowBoundary（14天前含 15天前不含）均绿
- [x] 2.2 空数据分支：无日报（空 lanes+candidates）、有日报无活跃泳道两态响应形状。验证：单测断言响应结构
  - 验证：TestGetBoardLaneDynamics_NoReports（has_reports=false + lanes/candidates 空数组非 null，JSON 形状断言）、TestGetBoardLaneDynamics_ReportsNoActiveLanes（has_reports=true + lanes 空；含可见候选时候选栏照常返回）均绿

## 3. 前端泳道动态视图

- [x] 3.1 `front/app/api/` 新增 laneDynamics client + 类型。验证：`pnpm lint` + `pnpm exec nuxi typecheck`（Windows cmd）通过
- [x] 3.2 `lane-dynamics/LaneDynamicsPanel.vue` 容器：加载/骨架/错误重试/空态（无日报引导生成，迁移 useDailyReportProgress WS 进度模式，完成后自动刷新）/有日报无泳道态。验证：组件测试（Vitest）覆盖 loading→success、error 重试、空态引导分支
- [x] 3.3 `lane-dynamics/LaneDynamicsCard.vue`：泳道名 + watch 角标 + 态势句（as_of 标注 / 待结算占位）+ 发展时间线（日期节点→当日事件、单日超限「还有 N 条」就地展开、卡片超高内部滚动）+ 点卡 emit。验证：组件测试——待结算降级渲染、折叠展开交互、emit 断言
- [x] 3.4 候选栏（LaneDynamicsPanel 内）：只读列表（名称+最近动向+日期），点击跳话题总览 focus。验证：组件测试——无候选不渲染、点击 emit 断言
- [x] 3.5 `BoardCompositionPanel.vue` 底部挂载点替换为 LaneDynamicsPanel（构成管理区不动），点卡联动接 TagsPage 既有 handleLandscapeSelectTopic 模式。验证：`pnpm test:unit` 相关用例绿

## 4. 态势版图退役

- [x] 4.1 删除 `topic-landscape/` 目录（TopicLandscapePanel/chart-options/StanceCardWall/TopicStanceCard）与 topic-landscape API client、后端路由与 repository 聚合。验证：全仓 grep 无残留引用；`go build ./...` + `pnpm exec nuxi typecheck` 通过（目录 11 文件已删；semanticBoards.ts 类型链已清；后端 repository/handler/常量已删；测试辅助函数提取到 lane_test_helpers_test.go）
- [x] 4.2 清理 topic-landscape 相关旧测试（组件/E2E 用例），保留迁移走的空态/联动等价用例。验证：`pnpm test:unit` + 受影响 e2e 冒烟绿（e2e 无 topic-landscape 引用，确认无需改；semanticBoards.test.ts 两个旧用例已删；空态/联动等价用例在 LaneDynamicsPanel/Card 测试中重建 18 个）

## 5. 验证与收口

- [x] 5.1 后端影响包验证：`bash scripts/change-scope.sh` 判定后跑对应 `go test`（topicgraph/dataenrichment）；`golangci-lint run ./...` + `go vet ./...` + `go build ./...` 全绿（实跑：topicgraph + platform/database PG 集成 13.2s/19.7s 绿；golangci-lint 0 issues；vet/build OK）
- [x] 5.2 前端全量：cmd.exe 跑 `pnpm test:unit`（876 用例全绿 EXIT=0）+ `pnpm build`（EXIT=0）；真实环境主链路断言（后端 5001 + 前端产物 3000 + 真实库 12 板块）：进板块内容 tab → 5 张泳道卡渲染（watch 角标 + 待结算占位 + 时间线 72 日期节点）→ 点卡「美伊形式对市场影响」→ 话题总览 tab active + focus 视图展示该话题 → 候选栏 DOM 断言渲染（旧「话题态势版图」全页无残留）；证据存 acceptance-*.png
- [x] 5.3 视觉验收：1440×900 / 1920×1080 / 1440×900-暗色 三张截图（acceptance-1440x900.png / acceptance-1920x1080.png / acceptance-1440x900-dark.png），视觉子代理检查通过：网格无重叠溢出、时间线清晰、暗色真全暗且对比度可读无 token 失配；与批准原型差异：无重大差异（卡片布局与原型一致，态势句首版为「待结算」占位——部署后首个日报日生效，属预期降级非差异）；候选栏在视口外需滚动，DOM 断言已确认渲染
- [x] 5.4 效果核对（待部署后首个日报日，2026-09-10 21:00 或手动生成日报后）：验证结算产物——快照行数=活跃泳道数（clamp 20 内）、抽样 3 条态势句与该泳道时间线事实一致、`ai_call_logs` 有 operation=daily_report.lane_snapshot 记录且无失败堆积；量化结果补记于此
  - **核对结果（2026-09-11 实测，当晚 21:00 定时日报已结算）**：
    - 快照：9 板块合计 37 行，每板块 2~8 全部 ≤ clamp 20；37/55 差值为窗口内无素材的沉寂泳道（代码注释明确跳过属预期，非缺陷）
    - 抽样 3 条态势句：事实锚点全部来自 prompt 事实清单（GPT-6 Astra/自主入侵/诉讼｜长鑫 LPDDR6/SK 海力士/关税｜英伟达收购 HF/DeepSeek V4.1），as_of=2026-09-11 新鲜
    - ai_call_logs：operation=daily_report.lane_snapshot 共 98 条，失败 0 条，无堆积

| Scenario | 测试文件 |
|---|---|
| 默认展示泳道动态区 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
| 只读语义 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
| 一句话态势展示 | front/app/features/tags/components/lane-dynamics/LaneDynamicsCard.test.ts |
| 发展时间线结构 | front/app/features/tags/components/lane-dynamics/LaneDynamicsCard.test.ts |
| 单日多事件 | front/app/features/tags/components/lane-dynamics/LaneDynamicsCard.test.ts |
| 活跃排序 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| watch 标识 | front/app/features/tags/components/lane-dynamics/LaneDynamicsCard.test.ts |
| 沉寂不展示 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go,backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 候选不进主区 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 候选列出 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
| 无候选 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
| 单请求聚合 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 无态势快照降级 | front/app/features/tags/components/lane-dynamics/LaneDynamicsCard.test.ts |
| 日报完成后结算 | backend-go/internal/topicgraph/service/lane_snapshot_test.go,backend-go/internal/topicgraph/service/daily_report_lane_pipeline_test.go |
| 结算失败不阻塞 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 态势句与时间线同窗 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 点卡片聚焦 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts,front/app/features/tags/components/TagsPage.test.ts |
| 空态引导 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
| 生成后刷新 | front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts |
