# Tasks：退役事件线，改用 event 标签与当天线索热度

> 四个交付故事（见 test-cases.md）：S1 看矩阵当天的线索数与热度、S2 日报当天同事件聚簇上今日重点、S3 story 停写后一切照旧、S4 调候选权重。变体就地走查，不为每个分支另开任务。
> **本轮只出规划制品**：下列任务全部为待实施状态，测试文件路径是计划路径（当前不存在），验证命令须在 apply 阶段实跑并把结果写进 8.1。

## 1. 后端：story 停写与口径替换

- [x] 1.1 在 `ai_settings` 增加布尔键 `story_attribution_enabled`（缺项回退默认=写 story，坏值告警回退），并在 `service/daily_report_story_highlights.go:boundedReportAttribution` 前置判断：关闭时不产生 `board_stories` / `story_signals` / `daily_report_threads.story_id`；不改判定链其余代码、不删表、不删列。验证：扩写 `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/service -run TestStoryAttributionSwitch` → PASS（开关开=行为不变、开关关=零写入、坏值回退三态各一例）。 → PASS（`go test ./internal/topicgraph/service -run TestStoryAttributionSwitch`，1 用例三态：开关开=照常进入归属判定、开关关=零写入/不进入判定、坏值回退默认）。**review R-1 用户拍板（2026-11-11）：开关删除改硬停写**——`ai_settings.story_attribution_enabled` 与 `repository/story_attribution_settings.go` 一并删除，`boundedReportAttribution` 及其在日报尾段的调用摘除（判定函数库与写端实现保留），用例改写为 `TestStoryAttributionHardStop`（硬停写下日报尾段恒零 `board_stories` / `story_signals` / `story_id` 写入）；回滚路径从「改键」改为 `git revert`
- [x] 1.2 把 `backend-go/cmd/backfill-stories/main.go` 的写模式（`--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings`）改为直接拒绝：非零退出 + 打印「事件线已退役」说明；`--dry-run` 与只读 / 实验模式保留可用且零写入。验证：扩写 `backend-go/cmd/backfill-stories/main_test.go`，执行 `cd backend-go && go test ./cmd/backfill-stories` → PASS（每个写模式一例拒绝、只读一例可用）。 → PASS（`go test ./cmd/backfill-stories`，29 用例；新增 TestParseOptionsWriteModesRejected：9 个写模式全拒 + 7 个只读/--dry-run 模式保留）
- [x] 1.3 新写泳道当天热度纯函数（泳道热度 = active 当天线索数 × 1.0 + candidate 当天线索数 × `lane_heat_candidate_weight`；版块关注度 = Σ 泳道热度，保留一位小数；口径版本另立），并新增一条按 `persistent_topic_id` 聚合当天 thread 数的查询（覆盖 active + candidate 两类泳道）；权重读 `ai_settings`，缺项 0.5、坏值告警回退、域 [0,1]。验证：新增 `backend-go/internal/topicgraph/repository/lane_heat_repository_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/repository -run TestLaneHeat` → PASS（隔离 PG，不用 SQLite；用例清单：active/candidate 权重、权重为 0、权重为 1、综合稿不计、桶合并不取峰值、坏值回退、确定性重复计算同值）。 → PASS（`go test ./internal/topicgraph/repository -run TestLaneHeat`，6 用例/纯函数 12 分支：active×1.0、candidate×权重、权重 0/1、综合稿不计、分桶合计不取峰值、跨天不计、坏值回退、确定性）
- [x] 1.4 `repository/lane_snapshot_repository.go:GetBoardLaneDynamics` 的负载换口径：逐日下发「当天线索合计」（不再下发 `story_count` / `hottest_story_id` / `hottest_story_title` / `unassigned_event_count`），`lane_heat` 换成 1.3 的新口径，候选条目补锚点日当天线索数，行序改为「泳道当天热度降序 → 全窗线索合计降序 → topic id 升序」。验证：扩写 `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamics` → PASS。 → PASS（`go test ./internal/topicgraph/repository -run TestLaneDynamics`，7 用例：逐日 clue_count 合计/综合稿不计、候选锚点日线索数+加权热度、热度→全窗线索→id 行序、时间线倒序与窗口边界、快照层级）
- [x] 1.5 `repository/board_radar_repository.go:GetBoardRadarOverview` 换口径：关注度 = Σ 泳道新热度；顶层的故事线条目段替换为「当天线索最多的泳道」清单段（上限 8，条目带版块 id/名、泳道 id/名、当天线索数、所属版块锚点日）；移除 `today_heat` / `last3_heat` / `prev3_heat` / `heat_days` / `story_counts` / 回填状态等已无消费方的字段，版块无数据仍返回零值而非 404。验证：扩写 `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/repository -run TestBoardRadar` → PASS。 → PASS（`go test ./internal/topicgraph/repository -run TestBoardRadar`，7 用例：关注度=Σ泳道热度+候选权重、三窗线索轴、前 3 泳道按线索合计、清单跨板混排/稳定破同分/上限 8/滞后自锚点日/延迟标 partial、退役字段不再下发）

## 2. 后端：日报当天同事件聚簇与今日重点

- [x] 2.1 实现日报当天同事件聚簇服务：取该版块当天命中的 event 标签 → 用 `related.BuildRepresentation` 构造 `related-event-v1` 文本 → 批量 embedding（capability=embedding、operation 非空且与索引刷新 operation 区分、每批 32、单批 30s，与索引刷新同口径）→ 内存两两聚簇（阈值 `related_event_similarity_threshold`，默认 0.86）；降级链 = 语义向量兜底 → 主体近似兜底，并输出本次聚簇口径标记；全程不写 `related_event_embeddings` 与任何业务表 / 缓存 / 身份 / 关系表；每次调用尝试无论成败写 `ai_call_logs`（embedding 路径按仓内 N/A 惯例不记 prompt / response_snippet，截断保护只适用 chat 路径，review R-7；Metadata 带 board_id / report_id / batch_size）。验证：新增 `backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/service -run TestDailyReportEventCluster` → PASS（stub embedder，不调真实模型；用例清单：阈值边界三态、同分破序、跨天不入簇、批量不逐标签调用、零写入、三级降级与口径标记、综合稿不入簇、>500 截断。审计落库不在本文件断言——stub 绕过 airouter，审计保障在 airouter 共享测试 `embed_cache_router_test.go`，review R-9）。 → PASS（`go test ./internal/topicgraph/service -run TestDailyReportEventCluster`，5 用例/阈值三态 3 子例：阈值边界、确定性破序、端到端批量调用+operation 独立+内容身份去重+跨天/综合稿不入簇+零写入、三级降级与口径标记、32 拆批+表示失败显式计入）
- [x] 2.2 把今日重点候选源从事件线换成聚簇：`repository/report_story_highlights.go:ReportStoryCandidates` 与 `service/daily_report_story_highlights.go:GenerateStoryHighlights` 取聚簇 Top 8（簇内文章数降序 → 簇内 event 标签数降序 → 代表标签 id 升序），重写选题 prompt（只从候选白名单选、禁止编造事实与数字），模型返回的 `cluster_id` 必须在候选白名单内、`tag_ids` 必须在该聚簇的本期标签白名单内，非法按既有语义同参重试一次、合法 `[]` 不重试、最终失败落空数组；簇内文章数、聚簇口径、数据截至日、研究时点由服务端补快照。验证：新增 `backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/service -run TestHighlightsCluster` → PASS。 → PASS（`go test ./internal/topicgraph/service -run TestHighlightsCluster`，4 用例：prompt/快照/展示顺序、集外 cluster_id 与越界 tag_ids 同参重试一次、合法 [] 不重试、零候选走标签回退且不伪造聚簇身份）
- [x] 2.3 日报数据模型收口：新生成 / 重建的日报 `story_id` 恒 NULL、`story_manual` 恒 false、`display_tier` 恒 body（历史值只读保留、对外不透出）；`scope` 仍由 `tag_ids` 中 event 标签数推导且 composite 不进聚簇；`highlights` 增加 `cluster_id` / `article_count` / `cluster_mode` / `data_as_of` / `research_at` 快照字段，落库恒为数组、读出把历史 `null` 归一为 `[]`。验证：扩写 `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`，执行 `cd backend-go && go test ./internal/topicgraph/repository -run TestDailyReport` → PASS。 → PASS（`go test ./internal/topicgraph/repository -run TestDailyReport`，4 用例：StoryID/StoryManual/DisplayTier 插入路径收口、scope 仍由 event 标签数推导、highlights 落库恒数组、快照字段读写与坏 cluster 引用置空、thread story_id 不外透）

## 3. 前端：矩阵 / 雷达 / 日报口径替换

- [x] 3.1 矩阵与顶部曲线换口径：`front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics` 的展示值主源从事件线数改为当日线索合计（`total`），退役「事件线为 0 时按线索合计兜底」这层与 `degradedFromClue` 标记；`observationLanes.ts:matrixCellView` 与 `ObservationMatrixLaneRow.vue` 停用颜色通道（`laneLevel` / `laneDegradedLevel` 退场），格值与行头 / 格 aria 改为「N 条线索 · M 个主题桶」；矩阵口径说明改为「数字 = 当日线索合计」；`BoardLaneEvolution.vue` 文案「N 条事件线」改「N 条线索」。验证：扩写 `front/app/features/tags/components/console/observationLanes.test.ts` 与 `front/app/api/normalizers/laneDynamicsWindow.test.ts`，执行 `cd front && pnpm test:unit app/features/tags/components/console/observationLanes.test.ts app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2` → PASS。 → PASS（`observationLanes.test.ts` 22 用例改线索口径：行序 laneHeat→clueTotal→id、`clue_count` 优先/回退、MatrixCellView 无 bucket、aria「N 条线索 · M 个主题桶」、颜色通道与最热事件线点名负向断言；`ObservationMatrixGrid.test.ts`/`BoardLaneEvolution.test.ts`/`useBoardConsole.test.ts` 同步改 clueCount/clueTotal；另修 `ObservationMatrix.test.ts`/`ObservationLaneEvolution.test.ts`/`laneConsoleView.test.ts` 的旧 story 断言。定点 5 文件 83 用例 + 后续 30 文件 517 用例全绿；`pnpm exec nuxi typecheck` 无错误）
- [x] 3.2 雷达首屏换内容：`BoardRadarReadingRail.vue` 的故事线条目段换成「当天线索最多的泳道」清单（版块名 + 泳道名 + 当天线索数，上限 8），移除「升温 N×」「新起」徽标与「跨天延续事件线建设中」脚注，保留空态占位与「本次清单不完整」标注；`BoardRadarStory.vue` 的行内小矩阵改当天线索数（未知标「线索数未知」，不造 0）并去掉色阶；`front/app/api/normalizers/boardRadar.ts` 同步字段增删与旧后端兼容分支；`useBoardRadarOverview.ts` 移除 `warming` / `topStories` / `crossDayBuilding` 派生。验证：新增 `front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts`、扩写 `front/app/features/tags/components/console/BoardRadarView.test.ts`，执行 `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → PASS。 → PASS（`boardRadar.ts` 归一 `clue_counts`/`today_clues`/`clue_days`/`top_lanes`，旧后端缺字段一律 `null` 不造 0；`useBoardRadarOverview` 删 warming/topStories/crossDayBuilding，新增 laneItems/partialLanes/degraded/lagging/todayClueTotal；右栏换「当天线索最多的泳道」清单（版块名+泳道名+线索数+锚点日，上限 8，服务端截断），徽标/跨天脚注/升温卡片整档退场；小矩阵改 clue_counts 且颜色通道退役、未知标「线索数未知」；报头「升温 N」统计换「今日线索 N」。新增 rail 10 用例 + 重写 BoardRadarView 26 用例 + `boardRadar.test.ts` 14 用例 + `useBoardRadarOverview.test.ts` 15 用例 + helpers 10 用例 + masthead/status block/pipeline 同步换 `todayClues`，全部 PASS）
- [x] 3.3 日报刊头换口径与定位：`DailyReportMasthead.vue` 的今日重点改按 `cluster_id` 定位本期正文首条 thread（高亮 2s、reduced-motion 不平滑滚动），依据文案改「N 篇来源 / 聚簇口径 / 数据截至日 / 研究时点」，缺字段省略数字不伪造 0；`DailyReportReader.vue` 的跨页寻址参数改 `?cluster=`，旧 `?story=` 解析后忽略并提示该定位方式已下线（不报错、不定位）。验证：新增 `front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts`、扩写 `front/app/features/tags/components/DailyReportReader.test.ts`，执行 `cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts app/features/tags/components/DailyReportReader.test.ts --maxWorkers=2` → PASS。 → PASS（`api/dailyReports.ts` 换契约：highlight 增 `cluster_id/article_count/cluster_mode/data_as_of/research_at`、删 story 六字段与 thread `story_id`；`dailyReportMagazine` 用 `reportHasClusterThread`（thread.tag_ids 含代表标签 id）替代 `reportHasStoryThread`，`is_progress`/热度快照退役；刊头依据行「N 篇来源 · 聚簇口径 · 数据截至日 · 研究时点」，缺字段整体省略；正文分区定位改 `{clusterId, nonce}`（DOM 首条命中、2s 高亮、reduced-motion 用 `auto`）；`?cluster=` 深链 + 旧 `?story=` 解析后忽略并提示；新增 cluster 主链路 4 用例 + Reader/Masthead/TopicSection/DeskShell 同步改断言，全部 PASS）
- [x] 3.4 在既有「匹配规则」配置面接入 `lane_heat_candidate_weight`（默认 0.5、域 [0,1] 有限数、非法拒绝且不覆盖旧值、缺项回退默认），后端键白名单与取值校验同步加该键，且与 `related_event_similarity_threshold` 互不影响。验证：新增 `front/app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts`、扩写 `backend-go/internal/tagmanagement/handler/related_events_test.go`，执行 `cd front && pnpm test:unit app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts --maxWorkers=2` 与 `cd backend-go && go test ./internal/tagmanagement/handler -run TestMatchingConfig` → PASS。 → PASS（前端：新「当天线索热度」块接 `lane_heat_candidate_weight`，默认 0.5、域 [0,1] 有限数、非法不发 save、缺项回退默认、与相关事件阈值各自独立校验；`MatchingConfigDialog.laneHeat.test.ts` 8 用例 + 既有 relatedEvents 10 用例全绿。后端：键白名单 + GET 实际生效值，取值走默认分支（0..1 有限数、NaN/Inf 拒绝），`TestMatchingConfigLaneHeatCandidateWeight` 5 子例 + 既有 `TestMatchingConfigRelatedEventThreshold` 全绿；键名/域校验落 `platform/aisettings`（避免 handler→topicgraph/repository 循环依赖），repository 侧保留同名转发。`go test ./internal/tagmanagement/handler -run TestMatchingConfig` → ok 31.5s）
- [x] 3.5 矩阵色深修订（2026-11-11 用户看版反馈）：色块按当日线索数行内归一连续深浅，数值+aria 仍为唯一信息承载 → PASS（`observationLanes.ts` 新增纯函数 `clueDepth(clueCount, rowMax)` 0~1 / `clueRowMax(counts)` / `clueMix(depth)`，`CLUE_DEPTH_MAX_MIX = 66`；`matrixCellView` 新增 `depth` 字段，行内归一分母由 `ObservationMatrixGrid.project` 两遍投影注入（可见窗，默认 14 天），`ObservationMatrixLaneRow.vue` 与 `BoardRadarStory.vue` 共用同一函数并以 CSS 变量 `--omx-clue-mix` / `--rd-clue-mix` 接主题色阶（`color-mix(var(--color-accent) ... var(--color-bg-base|hover))`）；0 线索 / `clueCount=null` / 断档不着色，跨行不归一；顶部曲线与演进节点不着色；旧 `laneLevel` / `laneDegradedLevel` 四档不恢复。验证：`cd front && pnpm test:unit app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2` → `Test Files 5 passed / Tests 102 passed`；`pnpm lint` → `0 errors, 8 warnings`（均存量）；`pnpm exec nuxi typecheck` → 无错误；`openspec validate retire-story-adopt-event-metrics --strict` → valid；`bash scripts/harness/scenario-trace.sh openspec/changes/retire-story-adopt-event-metrics` → 103 个 Scenario 映射齐全）

- [x] 3.6 滞后版块沉底弱化（2026-11-11 用户看版反馈）：右栏锚点日新鲜度优先排序+灰显标注，雷达列表滞后版块不挂分档徽标 → PASS（后端 `finalizeRadarTopLanes` 排序改两级：① 锚点日新鲜度（条目 `anchor_date` = 全局锚点日的条目优先，滞后条目整体沉底但仍在清单内）→ ② 当天线索数降序 → 稳定键（版块名 → 泳道名 → 泳道 id）；沉底不是折扣，滞后条目 `clue_count` 原值参与次级排序、不引入时间衰减。前端 `boardRadarHelpers.isLaggingAnchor` 统一滞后判据（任一侧日期缺失不判滞后）；`BoardRadarReadingRail.vue` 滞后条目 `is-lagging` 灰显（opacity 0.58、悬浮恢复对比度）+ 行内「数据至 MM/DD」（今日条目仍为「截至」），排序照抄服务端不改顺序；`BoardRadarStory.vue` 滞后版块撤除「关注强度 · 热点/关注」分档徽标、改中性「数据至 MM/DD」，关注度数值照实显示。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestBoardRadar` → ok 34.9s（新增 `TestBoardRadarTopLanesLaggingSinks` 覆盖今日整体先于滞后 / 滞后内部按线索数降序 / 空清单与全滞后退化，既有排序用例按新口径改断言）；`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2` → `Test Files 4 passed / Tests 69 passed`；`pnpm lint` → 0 errors；`pnpm exec nuxi typecheck` → 无错误；`golangci-lint run ./...` → 0 issues；`go vet ./...` / `go build ./...` → 通过）

- [x] 3.7 今日重点簇内展开 + 当日其他话题计数核对（2026-11-11 用户反馈）→ PASS（`dailyReportMagazine` 新增 `clusterTagIds` / `collectClusterThreads` / `LocateRequest`：簇内标签 = `highlight.tag_ids`（缺失回退代表标签 `cluster_id`），与正文 thread `tag_ids` 求交集，按正文 DOM 序（复用 `buildQualityZones` + `groupSectionsByTopic`，与定位语义同源）列出，代表线索置顶；`DailyReportMasthead.vue` 每条重点加「展开簇内线索（N）」开关（默认收起、`aria-expanded`/`aria-controls`、切 report 重置），展开区逐行标题 + 所属话题名 + 「代表线索」标注，行点击下发 `{clusterId, threadId}`；空交集显示「本期正文未找到对应线索」不硬造；条目主体点击与 `?cluster=` 深链载荷 `{clusterId, threadId: null}` 行为不变（仍定位 DOM 首条）。`DailyReportReader.vue` 定位载荷扩为 `{clusterId, threadId, nonce}`（threadId 精确预判），`DailyReportTopicSection.vue` 加 `findThreadById` 优先按 threadId 定位（否则按 clusterId 首条命中）。计数核对结论：矩阵「当日其他话题」组徽标原为**分区数**（`count = rows.length`），真库 report 1461 的 section 4 有 3 条 thread 却显示 1；改为 `clueCount` = 组内线索合计（与矩阵主行格值同口径），两个组组件（宽/窄）徽标显示「N 条线索」，分区数只留在说明文案。验证：`cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts app/features/tags/components/daily-report/DailyReportMasthead.test.ts app/features/tags/components/DailyReportReader.test.ts app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/DayOtherTopicsGroup.test.ts app/features/tags/components/console/ObservationMatrix.groups.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrix.dayHeaders.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts app/features/tags/components/console/otherTopicSelection.test.ts app/features/tags/components/console/otherTopicSelection.select.test.ts app/features/tags/components/console/BoardConsoleShell.integration.test.ts --maxWorkers=2` → 全绿；`pnpm lint` / `pnpm exec nuxi typecheck` → 无错误）

## 4. 关注度分档阈值重标定
- [x] 4.1 用真库分布实测新关注度的分位点（P80 / P20），把结果写回 `front/app/features/tags/components/console/boardRadarHelpers.ts` 的 `RADAR_HOT_ATTENTION` / `RADAR_WARM_ATTENTION`，并在 design.md 的 D9 留痕（样本范围、采样日期、分位点、原始分布摘要）。验证：人工：把实测命令与数字记入本任务与 design.md D9；执行 `cd front && pnpm test:unit app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2` → PASS（分档边界用例按新阈值断言）。 → PASS（真库只读 SELECT：近 14 个报告日全部版块×日关注度正值 154 样本 / 13 版块，采样日 2026-11-11，**P20 = 2.3 / P80 = 14.5**（一位小数），P50 8.5 / P90 17.9 / min 0.5 / max 24.5 / 均值 8.79；30 日窗复算 P20 2.0 / P80 14.0 口径稳定。已写回 `RADAR_HOT_ATTENTION = 14.5` / `RADAR_WARM_ATTENTION = 2.3`，design.md D9 留痕样本范围/采样日期/分位点/分布摘要与当日影响面；`boardRadarHelpers.test.ts` 10 用例 PASS）
- [x] 4.2 把「新口径下不得沿用旧 3.3 / 0.7，也不得沿用更早的 50 / 15」写成可执行断言（常量值与旧值不相等，且分档边界用例按新值判定）。验证：同 4.1 的定点 vitest 命令 → PASS（该断言为该文件内一条独立用例）。 → PASS（独立用例断言 `RADAR_HOT_ATTENTION`/`RADAR_WARM_ATTENTION` 均不等于 3.3 / 0.7 / 50 / 15，且量级 >10 / >1；分档边界用例按新值判定：14.5 热点、14.4 关注、2.3 关注、2.2 常温）

## 5. 测试

- [x] 5.1 按 test-cases.md 的 S1 / S2 / S3 / S4 主链路与变体落自动化用例：后端只测影响包（`./internal/topicgraph/repository`、`./internal/topicgraph/service`、`./internal/tagmanagement/handler`、`./cmd/backfill-stories`），SQL 一律走 testcontainer PG，前端 `--maxWorkers=2`；白盒附加节的分支表 A / B / C 与边界值清单逐条落断言。验证：1.x–4.x 各条定点命令 → PASS；无全量测试。 → PASS（核对结论：S1 六节拍落在 `ObservationMatrixGrid`/`observationLanes`/`BoardLaneEvolution`/`laneDynamicsWindow`/后端 `lane_heat_repository`+`lane_snapshot_repository`/`BoardRadarView`+`boardRadarHelpers`；S2 六节拍落在 `daily_report_event_cluster_test`/`daily_report_highlights_cluster_test`/`DailyReportMasthead.cluster`/`DailyReportReader`；S3 四节拍落在 `daily_report_story_highlights_test`/`daily_report_repository_test`/`cmd/backfill-stories`/雷达矩阵日报读端；S4 四节拍落在 handler `TestMatchingConfig*`/`lane_heat_repository`/`MatchingConfigDialog.laneHeat`。补漏：本轮补齐前端三处旧 story 断言（`ObservationMatrix.test.ts` 图例/回填 footnote/颜色档、`ObservationLaneEvolution.test.ts` 曲线文案、`laneConsoleView.test.ts` 曲线量纲）与 S4 真库效果核对；白盒 A/B/C 分支表分别由 `lane_heat_repository_test`（1.3 PASS）、`daily_report_event_cluster_test`+`highlights_cluster_test`（2.1/2.2 PASS）、`daily_report_story_highlights_test`+`cmd/backfill-stories`（1.1/1.2 PASS）覆盖。定点前端 30 文件 517 用例全绿；后端影响包见 8.1）
- [x] 5.2 效果核对：取真库一个版块的当天 event 标签，按 test-cases.md S2 的口径人工审核若干簇对「是否值得一起看」，记录聚类覆盖率与簇内篇数分布，并把可复算的数字与结论写回 test-cases.md S2「效果核对」节。验证：人工：test-cases.md 该节填入数字与结论，不留「待实测」字样。 → PASS（版块 15220 / 2026-11-10 的 17 个当天 event 标签复算：17/17 有向量，1 簇（3 标签）+ 14 单例，成簇覆盖率 17.6%，簇内篇数 {3 篇: 1 簇}；簇样本 267178/267275/267597（黄仁勋风波：指控/当事方回应/市场反应），两两相似度 0.8714 / 0.8613，初判「值得一起看」；第二样本 2197 同 16 标签 0 对达标 → 无簇。口径如实标注为降级链第 2 级「语义向量兜底」（真库只读、不调模型以免写 ai_call_logs），结论与「样本已留档用户可复核」已写回 test-cases.md S2）
- [x] 5.3 停写完整性核对：全仓 grep 事件线写路径调用点，确认每个命中要么在测试文件内、要么被 1.1 的开关拦住，不存在第二条在线写入口。验证：执行 `grep -rn "CreateBoardStory\|AttachThreadToBoardStory\|MergeBoardStories\|RecomputeSignalsForStory\|RecomputeBoardStoryCounts" backend-go/internal backend-go/cmd --include=*.go` → 逐条列出并判定（无未拦截的在线写入）；结果写进本任务。 → 已执行，命中 100 行，判定：① 测试文件（`story_repository_test.go`/`story_signals_test.go`/`story_signals_content_key_test.go`/`story_heat_test.go`/`story_attribute_test.go`）仅仓储单测；② `story_repository.go`/`story_signals_repository.go` 为定义处，无调用；③ 唯一在线入口 `service/story_attribute.go` 实现在 `daily_report_story_highlights.go:boundedReportAttribution`，开关关闭即提前返回（1.1 已测三态）；④ `service/story_reattribute.go` 只被 CLI `--reattribute` 调用，写模式在 `cmd/backfill-stories/main.go:171` 前置拒绝；⑤ `cmd/backfill-stories/*.go` 写路径同上被拒。**唯一非测试非开关命中**：`repository/daily_report_repository.go:406`（`SaveReport` 提交后对「重建时被删 thread 的旧事件线」重算 signals/计数）——判定为历史数据维护，无建线/挂线/合并能力；开关关闭后新报不写 `story_id`，重跑时 `affectedStoryIDs` 为空自然 no-op，仅「重跑上线前历史报」会刷新旧线派生计数（不清空 `story_id`）。结论：不存在第二条在线写入口；该遗留路径列 review 6.1 关注点。**review R-2 已修（2026-11-11）**：随 R-1 硬停写一并摘除——`SaveReport` 不再收集 `affectedStoryIDs`、不再提交后重算 `story_signals` / `participant_count`，重跑历史报也不得改写已退役的派生数据；`TestDailyReportRerunKeepsRetiredStoryData` 钉住该行为。

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），重点查三处：停写是否仍有旁路写入口；前后端是否各写了一份等价口径；聚簇降级链的口径标注是否如实（不得把主体近似呈现为向量聚簇）；意见按 High/Medium/Low 处置后 review.md 建账。验证：执行 `bash scripts/harness/review-trace.sh openspec/changes/retire-story-adopt-event-metrics` → 通过；不得用本轮制品审查冒充代码审查。 → PASS（`review.md` 建账）：glm-5.3 review 10 条（H1/M5/L4），R-1～R-4/R-7～R-9 已修，R-5/R-6/R-10 处置留痕；review-trace 通过。

## 7. 文档
<!-- doc-impact: flow, api, database, configuration, standard -->

- [x] 7.1 更新 `docs/reference/flow/semantic-board.md`（红线 28/29/30 改写：矩阵量纲改当日线索合计、颜色通道退役、关注度与分档换口径、事件身份未回填三态退役、事件线清单段退场）、`docs/reference/flow/daily-report.md`（红线 42–49 改写：事件身份列退为只读历史列、跨期去重退役、今日重点候选源换当天聚簇、内容身份去重口径保留）、`docs/reference/flow/topic-graph.md`（涉及事件线身份判定的表述改写，并明确候选权重不进话题生命周期判定）、`docs/reference/database/tables/daily-report-watch.md` 与 `docs/reference/database/_index.md`（事件线两表标注只读保留、`story_id` 退为历史列）、`docs/reference/api/`（总览端点与 lane-dynamics 的字段变更）、`docs/reference/configuration.md`（`story_attribution_enabled` 与 `lane_heat_candidate_weight` 两个 `ai_settings` 键）；归档时按 §12.2 补 `docs/reference/flow/` 的变更溯源链接。验证：执行 `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-story-adopt-event-metrics` → 通过。 → 通过（semantic-board 红线 28/29/30 改写为「当日线索合计 / 颜色通道退役 / 事件身份三态退役 / 关注度=Σ泳道当天热度+候选权重 / 新阈值 14.5-2.3 / top_lanes 清单 + partial-滞后标记」；daily-report 红线 42-49 改写为「身份列只读历史列 / 停写开关与 CLI 拒绝 / display_tier 恒 body / 跨期去重退役 / 当天同事件聚簇候选 + 快照 / 聚簇不落库与降级链 / 实体层不参与 / 内容身份去重保留」，两段链路设计标题下加「已退役」提示；topic-graph 红线 2 补「候选权重不进生命周期」、12-16 标注指纹/归一化/判定链/建线向量的历史状态；database `daily-report-watch.md` 两表标只读保留 + story_id/story_manual/is_development/display_tier 退为历史列，`_index.md` 同步；api `daily-reports.md` overview/lane-dynamics/highlights 三段换字段契约，`semantic-boards.md` matching-config 加权重键；configuration.md 加 `lane_heat_candidate_weight` 与 `story_attribution_enabled` 两键）。review R-1 后：`configuration.md` 的 `story_attribution_enabled` 行已删，`flow/daily-report.md` 红线 43 / `flow/topic-graph.md` 红线 14 / `database/tables/daily-report-watch.md` 同步改写为硬停写；review R-3 另补 `docs/reference/standard/backend/ai-logging.md`「已接入 AI 调用的功能清单」的 `topicgraph.daily_report.event_cluster.embedding` 行

## 8. 验证

- [x] 8.1 跑本 change 的门禁与影响范围验证：openspec strict、scenario-trace、check-tasks-tail、review-trace、doc-impact verify、check-standards，以及 1.x–5.x 各条定点 go test / vitest；把实际命令与结果写进本节执行记录表（含失败项的处置）。验证：逐条命令 + 期望结果；本轮规划阶段不跑代码测试、不连真库、不执行 git 命令。

### 8.1 规划阶段自检记录（2026-11-11，规划线程实跑）

| 命令 | 结果 |
| --- | --- |
| `openspec validate retire-story-adopt-event-metrics --strict` | ✅ 通过：`Change 'retire-story-adopt-event-metrics' is valid`（初稿曾报 5 处 MODIFIED 漏抄原 spec 场景，已按仓库先例 `matrix-clue-fallback-and-rail-merge` 的做法——保留原场景标题、改写场景正文——修正后复跑通过） |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ 通过：`✓ tasks.md 尾三节与 doc-impact 声明标记齐全` |
| `bash scripts/harness/scenario-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ❌ 未通过（**计划态预期为红**）：`103 个待对账 Scenario，33 项失败`，33 项全部为「映射文件不存在」——指向 6 个尚未创建的计划测试文件（`lane_heat_repository_test.go` 6 行 / `daily_report_event_cluster_test.go` 10 行 / `daily_report_highlights_cluster_test.go` 7 行 / `MatchingConfigDialog.laneHeat.test.ts` 1 行 / `BoardRadarReadingRail.lanes.test.ts` 6 行 / `DailyReportMasthead.cluster.test.ts` 3 行）。**未创建空壳测试、未把未建文件伪改为人工**；apply 阶段建文件后重跑应转绿 |
| `bash scripts/harness/review-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ❌ 未通过（**计划态预期为红**）：`✗ 未通过：缺 review.md 建账与 <!-- review: waived --> 豁免声明（零痕迹）`。本 change 是 complex 档且未声明 waived，review 为必做项，review.md 在 apply 阶段由 tasks 6.1 建账 |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-story-adopt-event-metrics` | ✅ 通过：`通过（声明: flow, api, database, configuration  文件:6 个 启发式:fallback）` |
| `bash scripts/harness/check-standards.sh --change retire-story-adopt-event-metrics` | ✅ 通过：`通过 280 / 失败 0` |
| ui-design-gate 特征词扫描（proposal + tasks） | ✅ 无命中：ui-design-gate 的 10 个 major 特征词（词表见 `.pi/extensions/lib/ui-design-gate.ts` 的 `MAJOR_UI_KEYWORDS`）均未出现在 proposal.md 与 tasks.md |
| 代码测试 / 真库核对 / git | **未跑**：本轮只出规划制品，不实现代码、不连真库；且用户禁止本轮执行 git 命令（doc-impact 的归属轨因此走全树回退，已按脚本输出如实标注） |

> 场景计数说明：delta spec 当前 103 个待对账 Scenario（MODIFIED 需求保留了原 spec 的全部场景标题，故计数高于初稿）。REMOVED 节下的 Scenario 不参与对账。
>
> 规划期外部事件（如实记录）：2026-11-11 11:00 左右，另一会话把 `normalize-event-subject-entities` 归档，其 delta 同步进主 spec（新增 `openspec/specs/entity-subject-resolution/spec.md`，并向 `event-story` 的「主体归一化」补了一条场景）。该新能力原文以「事件线身份判定仍只使用 `subject_key` 三层匹配」描述零改动边界，事件线退役后该引用悬空，故本 change 补了 `entity-subject-resolution` 的 MODIFIED delta（只改措辞与边界，实体层行为与写入面一字不动）。本 change 未改动任何主 spec。

### 8.1 apply 阶段执行记录（2026-11-11，实现线程实跑）

| 命令 | 结果 |
| --- | --- |
| `openspec validate retire-story-adopt-event-metrics --strict` | ✅ `Change 'retire-story-adopt-event-metrics' is valid` |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ `✓ tasks.md 尾三节与 doc-impact 声明标记齐全` |
| `bash scripts/harness/scenario-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ 转绿：`✓ 通过：103 个 Scenario 映射齐全（自动测试 102 / 人工留痕 1）`（6 个计划测试文件已创建） |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-story-adopt-event-metrics` | ✅ `通过（声明: flow, api, database, configuration  文件:6 个 启发式:fallback）` |
| `bash scripts/harness/check-standards.sh --change retire-story-adopt-event-metrics` | ✅ `通过 280 / 失败 0`（含 openspec/specs 168 capability validate） |
| `bash scripts/harness/review-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ❌ 仍红（**预期**）：缺 review.md 建账，tasks 6.1 由后续 review 会话执行；本轮未用制品审查冒充代码审查 |
| `cd backend-go && go test ./internal/tagmanagement/handler -run TestMatchingConfig` | ✅ `ok ... 31.5s`（新增 `TestMatchingConfigLaneHeatCandidateWeight` 5 子例 + 既有 relatedEvent 用例） |
| `cd backend-go && go test ./internal/topicgraph/repository -run 'TestLaneHeat\|TestLaneDynamics\|TestBoardRadar'` | ✅ `ok ... 7.2s` |
| `cd backend-go && go test ./internal/topicgraph/service -run 'TestStoryAttributionSwitch\|TestDailyReportEventCluster\|TestHighlightsCluster'` | ✅ `ok ... 17.8s` |
| `cd backend-go && go test ./cmd/backfill-stories` | ✅ `ok ... 0.5s` |
| `cd backend-go && golangci-lint run ./...` | ✅ `0 issues`（首轮报 1 处 gofmt 对齐，`gofmt -w` 后复跑清零） |
| `cd backend-go && go vet ./...` / `go build ./...` | ✅ 均无输出（通过） |
| `cd front && pnpm exec nuxi typecheck` | ✅ 无错误（修完 4 个旧 story 测试文件后全绿） |
| `cd front && pnpm lint` | ✅ `0 errors, 8 warnings`（8 条均为存量测试文件未使用变量/any，非本 change 引入） |
| `cd front && pnpm build` | ✅ `Build complete`（**首跑 OOM**：默认堆在树莓派上 `Aborted`；按仓库既有做法加 `NODE_OPTIONS=--max-old-space-size=3072` 后成功） |
| 前端定点 vitest（30 文件） | ✅ `Test Files 30 passed / Tests 517 passed`（含 3.1 矩阵/演进/曲线 5 文件、3.2 雷达 9 文件、3.3 日报 6 文件、3.4 配置 2 文件、normalizer 4 文件、console 其他 4 文件） |
| `bash scripts/harness/test-patrol.sh --report` | 未跑（本轮已有高负载 build/lint，按「会话收尾若无高负载操作才顺手巡检」跳过） |

> 说明：后端只跑 1.x–2.x 的定点包与 3.4 的 handler 用例，未跑 `go test ./...`（树莓派红线）；前端未跑全量，只跑受影响 30 文件。

### 8.1 review 修复阶段执行记录（2026-11-11，review R-1～R-10 收口）

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && go test ./internal/topicgraph/service -run 'TestStoryAttributionHardStop\|TestHighlightsCluster\|TestDailyReportEventCluster'` | ✅ `ok`（硬停写零写入、聚簇尾段、>500 截断全绿） |
| `cd backend-go && go test ./internal/topicgraph/repository -run 'TestDailyReportRerunKeepsRetiredStoryData'` | ✅ `ok`（重跑历史报不改写 `story_signals` / `participant_count`） |
| `cd backend-go && go test ./cmd/backfill-stories` | ✅ `ok`（CLI 写模式拒绝不变） |
| `cd backend-go && golangci-lint run ./...` / `go vet ./...` / `go build ./...` | ✅ 见收尾记录（无 issues / 无输出） |
| `openspec validate retire-story-adopt-event-metrics --strict` | ✅ valid |
| `bash scripts/harness/scenario-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ 110 个 Scenario 映射齐全（场景标题保留，仅正文措辞改写；3.7 随「当日其他话题」计数口径 MODIFIED 补入 7 条既有场景映射） |
| `bash scripts/harness/review-trace.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ 通过（review.md 建账 + 处置栏） |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/retire-story-adopt-event-metrics` | ✅ 通过（尾三节与 doc-impact 标记齐全） |

> 本阶段改动：R-1 删开关改硬停写（后端 + 制品）、R-2 摘 `SaveReport` 派生重算、R-3 补 ai-logging 登记、R-4 读端删除留痕、R-7/R-9 措辞对齐、R-8 补截断用例、R-5/R-6/R-10 处置留痕；前端不动。

### Scenario 映射表（计划路径，测试文件尚未创建）

> 下表是实现计划，不是现有自动化覆盖：所列测试文件当前不存在（计划路径），因此 `scenario-trace.sh` 在本轮预期为红；不生成空壳测试、不把未建文件伪改为人工以骗过门禁。REMOVED 节下的 Scenario 不参与对账。

| Scenario | 测试文件 |
| --- | --- |
| 活跃与候选按不同权重计 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go |
| 候选权重在线调整后立即生效 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go, front/app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts |
| 热度不因分桶或事件标签数量变化而变 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go |
| 只表达当天不跨天累积 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go |
| 坏权重存量回退默认 | backend-go/internal/tagmanagement/handler/related_events_test.go |
| 候选权重不影响话题状态机 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go |
| 关注度为泳道热度之和 | backend-go/internal/topicgraph/repository/lane_heat_repository_test.go |
| 分档阈值按新分布标定 | 人工：真库分布实测重标定并留痕（tasks 4.1） |
| 分档可读不靠颜色 | front/app/features/tags/components/console/boardRadarHelpers.test.ts |
| 同一内容多 feed 只算一篇 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 内容身份不可判定时退化为 feed 粒度 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 不同消费面口径一致 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 同版块当天同事件聚成一簇 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 聚簇不跨天不做身份 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 聚簇不落库不改索引 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 批量调用并留审计 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 阈值在线可调且与相关事件同源 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 向量不可得时如实降级 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 综合稿不进聚簇 | backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go |
| 创建日报记录 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 日报记录唯一性 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 并发落库同板同日不炸唯一约束 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 日报关联昨日报告 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 线程存储在独立表中 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 兄弟桶共享聚类序号 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 未分桶 section 取默认桶序号 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| highlights 落库恒为数组 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 历史 null highlights 读出归一为数组 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 事件落库时 scope 由事件标签数推导 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 新日报不写事件身份 | backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go |
| 旧事件身份只读保留 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 事件未归组不影响日报 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 点击第一条可见正文线程 | front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts |
| 失去目标与旧重点 | front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts |
| 重复定位与减弱动画 | front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts |
| 展示筛选不改变热度 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 深链参数定位与失去目标 | front/app/features/tags/components/DailyReportReader.test.ts |
| 旧 story 深链被忽略不报错 | front/app/features/tags/components/DailyReportReader.test.ts |
| 候选按簇内文章数截断 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 同分与精度不反转 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 只可从候选事实写文案 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 无聚簇走标签回退 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 少量候选与合法空返回 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 重点快照与旧数据兼容 | backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go |
| 打开工坊首屏 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 从雷达进入版块 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 热点行带理由与密度 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 常温版块折叠 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 关注度不因分区数虚高 | front/app/features/tags/components/console/boardRadarHelpers.test.ts |
| 小矩阵按线索数显示 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 线索数未知不造零 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 一次请求渲染雷达 | front/app/features/tags/composables/useBoardRadarOverview.test.ts |
| 无数据版块 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 事件线清单随行同响 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 徽标判定信息随条目自带 | backend-go/internal/topicgraph/repository/board_radar_repository_test.go |
| 按当天线索数跨版块混排 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 线索数相同则稳定排序 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 清单上限 8 且无徽标 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 按各版块自己的锚点日而非自然今天 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 清单为空渲染空态 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 部分版块延迟时明示不完整 | front/app/features/tags/components/console/BoardRadarReadingRail.lanes.test.ts |
| 同日分桶不冒充分区热度 | front/app/features/tags/components/console/observationLanes.test.ts |
| 矩阵格值是桶峰值 | front/app/features/tags/components/console/observationLanes.test.ts |
| 线索数未知不造零 | front/app/features/tags/components/console/observationLanes.test.ts |
| 真零事件不兜底成非零 | front/app/features/tags/components/console/observationLanes.test.ts |
| 颜色表达参与者数 | front/app/features/tags/components/console/observationLanes.test.ts |
| 渲染矩阵 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 图例可见 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 断档可读 | front/app/features/tags/components/console/ObservationMatrixGrid.test.ts |
| 零线索可开 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 历史未回填日按线索合计兜底 | front/app/features/tags/components/console/observationLanes.test.ts |
| 多线格点名获胜线 | front/app/features/tags/components/console/observationLanes.test.ts |
| 来源同分稳定点名 | front/app/features/tags/components/console/observationLanes.test.ts |
| 无身份不伪造标题 | front/app/features/tags/components/console/observationLanes.test.ts |
| 空标题与零来源 | front/app/features/tags/components/console/observationLanes.test.ts |
| 小矩阵历史未回填日按分区数兜底 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 拖选不重排 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 精确档回切 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 明确切独立档 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 档池分层失败 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 首屏曲线即含历史 | front/app/api/normalizers/laneDynamicsWindow.test.ts |
| 曲线与矩阵格同口径 | front/app/api/normalizers/laneDynamicsWindow.test.ts |
| 池未取到有提示可重试 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 池失败后切 365 档不残留误述 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 非法日期反馈 | front/app/features/tags/composables/useLaneTrendData.test.ts |
| 深链恢复 | front/app/middleware/tags-deep-link.test.ts |
| 发展时间线结构 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 一句话态势展示 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 单日多事件 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 矩阵格与演进节点同量纲 | front/app/features/tags/components/console/observationLanes.test.ts |
| 活跃排序 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 同日多条只计一天 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| watch 标识 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 沉寂不展示 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 候选不进主区 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 单请求聚合 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 存量长版透出 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 无态势快照降级 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 逐日负载为线索合计 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 候选泳道带当天线索数 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 默认关闭态逐字不变 | backend-go/internal/tagmanagement/service/entity/entity_mode_test.go |
| 影子态写入限定三处 | backend-go/internal/tagmanagement/service/entity/entity_mode_test.go |
| 事件线身份判定不读实体 ID | backend-go/internal/tagmanagement/service/entity/entity_mode_test.go |
| 矩阵其他组复用泳道视觉且只绘当日格 | front/app/features/tags/components/console/DayOtherTopicsGroup.test.ts |
| 同 ID 关联日黄点与真实标题 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 黄点点击原子保留阅读目标且普通日期不变 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 换选清点与观察诚实降级 | front/app/features/tags/components/console/otherTopicSelection.select.test.ts, front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 阅读区去重并复用泳道正文视觉 | front/app/features/tags/components/console/LaneChronicle.test.ts |
| 零正式泳道仅保留有用阅读方向 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 历史成员与返回上下文不回归 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
