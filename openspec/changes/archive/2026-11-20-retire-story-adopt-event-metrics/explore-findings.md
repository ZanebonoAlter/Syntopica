
## story 读端全清单（雷达/日报/其它消费方）

【结论】story 读端共 3 个后端端点 + 2 组前端组件，切口径落点如下。

## 后端读端

1. `GET /api/semantic-boards/overview`（雷达总览）
   - handler: `backend-go/internal/topicgraph/handler/daily_report_handler.go:36` → `getBoardRadarOverview`
   - repo: `backend-go/internal/topicgraph/repository/board_radar_repository.go`
     - `GetBoardRadarOverview`(:183) → `buildRadarRow`(:349) → `finalizeRadar`(:507) → `finalizeRadarStories`(:536)
   - 读端 DTO: `RadarBoardRow`(:42) 字段 `TodayStories/Last3Stories/Prev3Stories`、`TodayHeat/Last3Heat/Prev3Heat/HeatDays`、`Attention`、`StoriesBackfilled`、`StoryIdentityState`(:81)、`Lanes[].StoryCounts`(:30)
   - 事件线清单段: `RadarStoryFeed`(:130) / `RadarStoryItem`(:98)，跨版块 Top N（上限 `radarTopStoryLimit=8`）、`WarmingCount`、`CrossDayStoryCount`、`Unbackfilled/NoStory/Lagging/Degraded` 四个降级集合
   - 事件线度量: `backend-go/internal/topicgraph/repository/board_radar_stories.go`
     - `RadarBoardStoryPoints`(:145) — 按版块自身锚点日算每条事件线的 heat / 3 日基线 / ratio / 当日有无证据
     - `radarStorySources`(:213) — 独立参与者数（内容身份优先、feed 兜底）
     - `RadarStoryReportDates`(:243) — 事件线落地日报
     - `radarBoardStoryIdentityState`(:72) / `radarBoardStoryIdentityCounts`(:52) — 板级「事件身份是否回填」三态
   - 换口径落点：① 矩阵日格 `RadarLane.StoryCounts` → 改回「当天该泳道线索数」；② 关注度 `Attention`（=Σ lane_heat）→ 改新热度公式；③ 事件线清单段整体退场（见写端停写后无数据）

2. `GET /api/semantic-boards/:id/lane-dynamics?days=14`（泳道动态，雷达矩阵的数据底座）
   - handler: `backend-go/internal/topicgraph/handler/daily_report_handler.go:57` → `getBoardLaneDynamics`
   - repo: `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`
     - `GetBoardLaneDynamics`(:631) — 泳道集合 = **active ∪ watch-linked**（:709-711），candidate 只进 `resp.Candidates`(:304) 且**不带 timeline/thread 数**
     - `LaneDynamicsLane`(:281) 字段 `SectionCount14d/ThreadCount14d/LaneHeat/StoryCount14d/Timeline`
     - `LaneDynamicsTimelineDay`(:259) 字段 `StoryCount/MaxParticipantCount/HottestStoryID/HottestStoryTitle/UnassignedEventCount`
     - `loadLaneStoryAggregates` + `foldDay`(:~560-609) — 当日去重事件线数
   - 换口径落点：`LaneDynamicsTimelineDay.StoryCount` → 「当天该泳道 thread 数」；`LaneDynamicsLane.LaneHeat` → 新热度；**candidate 泳道 thread 数当前完全不在响应里，新热度若要 candidate×0.5 必须新增查询**

3. `GET /api/daily-reports/:id`（日报详情：今日重点 + 事件列 story_id）
   - handler: `backend-go/internal/topicgraph/handler/daily_report_handler.go:42` → `getDailyReport`
   - repo: `backend-go/internal/topicgraph/repository/daily_report_repository.go`
     - `GetReportByID`(:441) → `resolveReportHighlightIdentities`(`report_story_highlights.go`:71)
   - 今日重点（highlights）依赖事件线：
     - `repository/report_story_highlights.go`：`ReportStoryCandidates`(:152) 按 `daily_report_threads.story_id` 取候选 → `rankReportStoryCandidates`(:114) 按 story_heat 排序 → `FinalizeReportHighlights`(:204) 落 `board_daily_reports.highlights`
     - `service/daily_report_story_highlights.go`：`storyHighlightsSystemPrompt`(:18)「从最多 8 条候选事件线中选最多 3 条」；`GenerateStoryHighlights`(:96) LLM 调用；`storyHighlightCandidates`(:61) 截 top-8
   - 事件列 story_id：`daily_report_threads.story_id` 暴露在报告详情 sections[].threads[]（`daily_report_models.go:210`）
   - 换口径落点：`ReportStoryCandidates` / `rankReportStoryCandidates` / `GenerateStoryHighlights` 整链换成「当天同事件聚簇」口径；`resolveReportHighlightIdentities` 的 `IsProgress`（昨日重点对比）也要另定去留

4. `GET /api/semantic-boards/:id/tags/:tagId/related-events`（相关事件，只读）
   - handler: `backend-go/internal/tagmanagement/handler/related_events_handler.go:50`
   - service: `backend-go/internal/tagmanagement/service/related/query.go:87` `RelatedEvents`
   - 与 story 无关（不读 board_stories），但**与新「同事件聚簇」语义最接近**，可作口径参照

5. **无导出面**：全仓无任何导出端点携带 story 字段（已 grep 确认）。

## 前端消费方

| 消费点 | 文件:符号 | 现状口径 | 换口径落点 |
|---|---|---|---|
| 雷达总览行（含 14 日小矩阵） | `front/app/features/tags/components/console/BoardRadarStory.vue` | 小矩阵格值 = 事件线数（`laneCells`，降级格按分区数） | `laneCells` 改线索数；`todayCount`(:47) 改线索数 |
| 雷达右栏「今日该读」 | `front/app/features/tags/components/console/BoardRadarReadingRail.vue` | 事件线清单 + `新起/升温 N×` 徽标 | **整段退场**（`storyMode` 分支、`:45-98`） |
| 雷达总览 composable | `front/app/features/tags/composables/useBoardRadarOverview.ts` | `warming`(:69) 按 todayHeat/last3Heat；`crossDayBuilding`(:91)；`topStories`(:96) | `warming`/`topStories`/`crossDayBuilding` 退场或换源 |
| 雷达 API 归一化 | `front/app/api/normalizers/boardRadar.ts` | `storyCounts`(:129)/`storyCountsKnown`(:130)/`todayStories`/`todayHeat`/`heatDays`/`stories`(:171) | 字段增删 + 旧后端兼容分支同步 |
| 雷达分档常量 | `front/app/features/tags/components/console/boardRadarHelpers.ts` | `RADAR_HOT_ATTENTION=3.3`(:12)、`RADAR_WARM_ATTENTION=0.7`(:13)、`laneLevel`(:78) | 关注度换量纲 → 阈值必须重新标定（红线 29 禁止沿用旧 section 量纲的 50/15） |
| 控制台矩阵日格 | `front/app/features/tags/components/console/ObservationMatrixGrid.vue` + `ObservationMatrixLaneRow.vue:38` | 格值 = `story_count`（缺省回退非综合稿线程合计）；颜色 = 最热事件线来源数分档 | 格值与颜色口径一起换（无事件线后无「最热来源数」→ 颜色通道需另定或退役） |
| 矩阵纯模型 | `front/app/features/tags/components/console/observationLanes.ts` | `buildObservationRows`(:70) 行序 = lane_heat → storyTotal → topicId；`projectMatrixCell`(:135)；`matrixCellView`(:191)；`knownDayTotal`(:252) | 行序主键改新热度；格值改线索数 |
| 矩阵 API 归一 | `front/app/api/normalizers/laneDynamicsWindow.ts` | `dayCellMetrics`(:184) 已同时产出 `storyCount` / `peak`(桶峰值) / `total`(当日线索合计) | **初版口径所需的 `total`/`peak` 已在手上**，切换成本低 |
| 矩阵模型 | `front/app/features/tags/composables/laneMatrixModel.ts` | `clueCounts`(:65) 已按日聚合线索数；`storyCounts`(:67)；行序 :146 | 改 `storyCounts` → `clueCounts` |
| 演进带 | `front/app/features/tags/components/console/BoardLaneEvolution.vue` | 节点数 = `storyCounts[date]`(:68)；文案「N 条事件线」(:153,:189) | 改线索数 + 改文案措辞 |
| 演进节点 | `front/app/features/tags/components/console/LaneEvolutionNodes.vue` | 节点数字 = 事件线数 | 同上 |
| 日报刊头「今日重点」 | `front/app/features/tags/components/daily-report/DailyReportMasthead.vue` | `highlight.story_id` 定位(:70)、`is_progress`(:75)、热度提示 | 今日重点换口径 + `story_id` 深链退场 |
| 日报重点派生 | `front/app/features/tags/components/daily-report/dailyReportMagazine.ts` | `highlightEvidence`(:~170) 用 `story_heat`/`heat_as_of`；`reportHasStoryThread`(:161) 按 `thread.story_id` 判定位 | 整链换口径 |
| 日报阅读器定位 | `front/app/features/tags/components/DailyReportReader.vue` | `?story=` 深链(:58,:314,:354) 按 `thread.story_id` 定位 DOM | 深链参数退场 |
| 日报 API 类型 | `front/app/api/dailyReports.ts` | `DailyReportHighlight.story_id/story_heat/heat_rule_version/heat_as_of/heat_incomplete`(:11-21)；`DailyReportThread.story_id`(:39) | 字段增删 |
| 泳道编排事件线链 | `front/app/features/tags/composables/useLaneCompose.ts` + `components/compose/LaneComposeCandidates.vue` | 活跃 change `lane-compose-story-chain` 正在加「事件线链」维度 | **与本 change 直接冲突**，需一并裁决 |

## 关键判断
- 雷达矩阵换口径是**纯展示层**切换：`dayCellMetrics` 已同时持有 `peak`（桶峰值）与 `total`（当日线索合计），「初版口径」两个候选值都现成，后端 `LaneDynamicsTimelineDay` 只要补一个 `ThreadCount`（当日线索合计）字段即可，前端不用等新接口。
- 「今日该读」事件线清单段退场是**必然连带**：它依赖 `board_stories` 与 `story_signals`，story 停写后新数据不再产生（只剩存量）。
- 矩阵格「颜色 = 最热事件线独立来源数」这条通道在事件线退役后**失去数据源**——`max_participant_count` 来自 story 身份，不是报告几何。要么另找量纲（如当日线索数分档），要么退役颜色通道（红线 28 明确要求「颜色 SHALL NOT 是唯一信息载体」，但也要求有图例与 title/aria，需重新定档）。

**引用**：backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview、backend-go/internal/topicgraph/repository/board_radar_repository.go:buildRadarRow、backend-go/internal/topicgraph/repository/board_radar_repository.go:finalizeRadarStories、backend-go/internal/topicgraph/repository/board_radar_stories.go:RadarBoardStoryPoints、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:LaneDynamicsTimelineDay、backend-go/internal/topicgraph/repository/report_story_highlights.go:ReportStoryCandidates、backend-go/internal/topicgraph/repository/report_story_highlights.go:resolveReportHighlightIdentities、backend-go/internal/topicgraph/service/daily_report_story_highlights.go:GenerateStoryHighlights、backend-go/internal/tagmanagement/handler/related_events_handler.go:getRelatedEvents、front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics、front/app/features/tags/components/console/observationLanes.ts:buildObservationRows、front/app/features/tags/components/console/observationLanes.ts:matrixCellView、front/app/features/tags/components/console/BoardRadarStory.vue:laneCells、front/app/features/tags/components/console/BoardRadarReadingRail.vue、front/app/features/tags/components/console/boardRadarHelpers.ts:RADAR_HOT_ATTENTION、front/app/features/tags/components/daily-report/dailyReportMagazine.ts:highlightEvidence

<!-- pinned 2026-11-11T02:46:39Z -->

## story 写端全清单（归属判定链/建线/CLI/实体层）

【结论】story **写端没有独立 scheduler job**，全部挂在日报生成链的尾部。停写只需掐断 1 个入口（`boundedReportAttribution`）+ 1 个 CLI 入口，不必动 scheduler。

## 写端唯一在线入口

```
service.GenerateAndSaveReport                (service/daily_report_watch.go:53)
  └─ repository.Repo.SaveReport(...)          (repository/daily_report_repository.go:228)  ← 写 board_daily_reports / sections / threads
  └─ finalizeDailyReportHighlights(...)       (service/daily_report_story_highlights.go:239)  ★ 停写切入点
       ├─ boundedReportAttribution(ctx, report)          (:215) ★★ 唯一在线归属判定入口
       │    └─ attributeReportOnline                      (:204)
       │         └─ attributeStoriesForReportWithStats(ctx, store, boardID, reportID, date, dryRun=false, online=true)
       │              (service/story_attribute.go:214)
       │              ├─ attributeOneThread (:271) — 逐 thread 五层判定
       │              │    ├─ fingerprint 解析：service/story_fingerprint.go:72 ParseEventFingerprint / :99 ValidateEventFingerprint
       │              │    ├─ 候选召回：repository/story_repository.go:179 RecallStoryCandidates（pgvector <=>，硬条件 embedding IS NOT NULL）
       │              │    ├─ 判定：service/story_assign.go:125 DecideAssignment → :278 classifyAssignment
       │              │    │    └─ :249 classifyStoryCandidate（subject_key 预筛 → object/action 匹配 → 日期邻近 → 指纹信任 → 合并闸门）
       │              │    │         └─ LLM 兜底：service/story_assign_llm.go:28 NewStoryClassifier（四分类）
       │              │    ├─ 挂载：repository/story_repository.go:439 AttachThreadToBoardStory
       │              │    ├─ 建线：repository/story_repository.go:409 CreateBoardStory（service/story_attribute.go:597 调用）
       │              │    ├─ 合并：repository/story_repository.go:534 MergeBoardStories（service/story_attribute.go:419 调用）
       │              │    └─ 主体归一：service/story_subject_normalize.go
       │              └─ 计数/证据重算：:525 attachThreadToStory / repository/story_repository.go:474 RecomputeBoardStoryCounts
       │                   └─ repository/story_signals_repository.go:16 RecomputeSignalsForStory（写 story_signals 热度证据）
       ├─ applyStoryDedupForReport(report.ID, report.PeriodDate)  (service/daily_report_story_dedup.go:59)  ← 跨期去重改 display_tier
       ├─ repository.Repo.ReportStoryCandidates(...)             (repository/report_story_highlights.go:152) ← 读
       ├─ GenerateStoryHighlights(...)                           (service/daily_report_story_highlights.go:96) ← LLM 选题
       └─ repository.Repo.FinalizeReportHighlights(report, ...)  (repository/report_story_highlights.go:204) ← 写 highlights
```

**停写准确落点（建议，遵循「不删代码、先停入口」）**
1. `service/daily_report_story_highlights.go:215 boundedReportAttribution` —— 加一个开关（ai_settings 或常量）直接 return nil，或把 `finalizeDailyReportHighlights` 里的 `boundedReportAttribution` 调用摘掉。**这是唯一会产生 `board_stories` / `story_signals` / `daily_report_threads.story_id` 的在线路径**，掐断它 = 停写。
2. `applyStoryDedupForReport`（:59）—— 依赖 story_id，story 停写后必然全 skipped；可保留不动（无新 story 时天然 no-op），或一并关。
3. `ReportStoryCandidates` + `GenerateStoryHighlights` —— 读端，随新口径一起替换。
4. `SpawnAttributeStoriesForReport`（service/story_attribute.go:674）—— **当前只有测试调用，无生产调用方**（已 grep 全仓确认），注释明写「Online daily reports instead use boundedReportAttribution before highlights」。停写时不必动它，但要在 spec 里记明它是死代码。

## CLI 入口（可跑、幂等、可重跑）

- `backend-go/cmd/backfill-stories/main.go`
  - `--board/--from/--to/--dry-run` → `service.BackfillStories`（service/story_backfill.go:141）→ `backfillStoryBoard`(:180) → `attributeStoriesForReportWithStats`(:213)
  - `--reattribute --snapshot <file>` → `service.ReattributeStories`（service/story_reattribute.go:227）→ `reattributeBoard`(:411) → 快照 + 重算身份 + `RecomputeSignalsForStory`(:401)
  - `--restore <file>` → `service.RestoreStoryIdentity`(:280)
  - `--rebuild-signals` → `cmd/backfill-stories/rebuild_signals.go`（只重写证据，不改身份）
  - `--backfill-content-keys` → `cmd/backfill-stories/backfill_content_keys.go`（内容身份去重回填）
  - `--fill-story-embeddings` / `--fill-thread-embeddings` → 回填召回向量
- 停写后应**下线/拒绝**这些 CLI 的写模式（至少在 CLI 里加显式拒绝或标注 deprecated），否则「停写」可被一条命令绕过。

## scheduler 现状

- `backend-go/internal/app/runtime.go` 注册的 19 个 job 里**没有任何 story 相关 job**（已逐条核对 registry.Register 清单）。story 全部是日报链的同步/派生动作。
- 日报 job 本身：`registry.Register("daily_report", ...)`（runtime.go:254），`Job: psched.PauseAware(admin.DailyReportJob())`。
- 因此「相关定时任务停用」在本仓**没有可停的对象**——若用户记忆里 story 有独立 job，实际落点是日报 job 的尾部步骤（同一处）。

## 实体层关停现状

- `entity_registry` / `entity_mentions` / `entity_aux_links` 三张表由**活跃 change** `normalize-event-subject-entities`（2026-11-09 建档，未归档）引入，proposal 明确：
  - 默认 `off`（2026-11-10 决策：影子实验结束，实体层**不接入事件线判定**）
  - `shadow` 保留为实验态，需显式配置开启；`active` 本期显式拒绝
  - 「实体 ID 本期不参与既有事件线身份判定（影子模式）」、「不重算存量 tag/story」
  - 故实体层目前**只写三处**：实体层表 + AI 审计 + `topic_tags.metadata.entity_subject` 惰性键
  - 对本 change 的含义：实体层**尚未接线到 story 身份链**，退役 story 时不需要拆实体层；两者解耦是现成事实。但活跃 change 的 spec 文本里仍引用「事件线身份判定」，退役后该 change 的 spec 需同步改写措辞（否则文档与实现分叉）。

## 其他会写 story 数据的路径

- `repository/daily_report_repository.go:406` —— SaveReport 重建报告后对受影响 story 调 `RecomputeSignalsForStory`（重跑日报会重算热度证据）。停写后无新 story，此路径自动空转。
- `daily_report_rebucket.go`（分桶重跑）会删/重建 threads → 触发 SaveReport 的 affectedStoryIDs 路径 → 重算证据。同样随停写空转。
- **手工拆分/指派 story 的接口**：grep 未发现独立的 story CRUD HTTP 端点（`topicgraph/handler/` 无 story 路由）；`story_manual` 只在 `daily_report_threads` 列上，由归属判定逻辑读。退役后此列变为只读历史列。

**引用**：backend-go/internal/topicgraph/service/daily_report_watch.go:GenerateAndSaveReport、backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights、backend-go/internal/topicgraph/service/daily_report_story_highlights.go:boundedReportAttribution、backend-go/internal/topicgraph/service/story_attribute.go:attributeStoriesForReportWithStats、backend-go/internal/topicgraph/service/story_attribute.go:attributeOneThread、backend-go/internal/topicgraph/service/story_assign.go:DecideAssignment、backend-go/internal/topicgraph/service/story_assign.go:classifyStoryCandidate、backend-go/internal/topicgraph/service/story_assign_llm.go:NewStoryClassifier、backend-go/internal/topicgraph/service/story_fingerprint.go:ParseEventFingerprint、backend-go/internal/topicgraph/service/daily_report_story_dedup.go:applyStoryDedupForReport、backend-go/internal/topicgraph/repository/story_repository.go:RecallStoryCandidates、backend-go/internal/topicgraph/repository/story_repository.go:AttachThreadToBoardStory、backend-go/internal/topicgraph/repository/story_signals_repository.go:RecomputeSignalsForStory、backend-go/internal/topicgraph/repository/daily_report_repository.go:SaveReport、backend-go/cmd/backfill-stories/main.go

<!-- pinned 2026-11-11T02:46:53Z -->

## 初版雷达矩阵口径考古（thread 口径的两变体与证据链）

【结论】「初版口径」= **thread（线索）口径**，不是 section 口径、也不是事件线口径。但初版内部有两个不同量纲：**矩阵日格 = 桶峰值**、**演进节点 = 当日线索合计**。用户原话「每个泳道日格显示当天该泳道有多少条线索（thread）」同时指向这两个，必须在 propose 阶段二选一。

## 证据链（时间序）

### ① 2026-10-27 矩阵阶段修订 —— thread 口径的「初版」形态
`openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/specs/workshop-board-console/spec.md:21-33`「Requirement: 泳道热度口径（2026-10-27 矩阵阶段修订）」：
- **矩阵日格 = 该泳道该日各主题桶（section）线索数的最大值（桶峰值）**
- **演进节点 / 泳道趋势 = 该泳道该日各主题桶线索数之和（当日合计）**
- **行序 = 基准响应全窗线索合计降序 → 全窗分区数 → topic id 升序**
- 同稿 `specs/board-lane-dynamics/spec.md:14`：「矩阵格内数字 SHALL 为同日各主题桶（section）线索数的最大值（桶峰值）……选中泳道的演进带 SHALL 展示按日期排列的节点（节点数字与点径 = 当日各主题桶线索数之和）」

实现合同 `implementation-matrix-stage.md`：
- `:64` 「矩阵（日格=桶峰值、去行尾总数、断档 '—'/0 可开）」——`buildLaneRow` 增 `peakCounts[date]=max(thread_count)`（缺省回退 `events.length+folded_count`，再缺=未知显示 `?`，不补桶数）；格值/aria/title 改「**桶峰值 X 条 · N 个主题桶 · 当日合计 M 条**」
- `:82-83` 数据来源表：日格数字（桶峰值）= A名单内同ID显示日 `timeline[].sections[].thread_count` 当日 max
- `:65` 演进带：节点点径 `min(24, 6+√total×5)`、节点数字 = **当日合计**；标签阈值 ≥3 且可见日 ≤31
- `:125` 用户可见变化：「日格数字语义**从分区数改为桶峰值**」→ 证明**再早一版（2026-10-27 之前）矩阵日格 = 分区数**
- `:194` 时间筛选折线图序列 = 逐日「该日全部 A 名单同 ID **桶线索合计**」（`dayCellMetrics(day).total`）

### ② 2026-11-06 add-event-story-heat —— 现行事件线量纲的来源
`openspec/changes/archive/2026-11-06-add-event-story-heat/proposal.md:12-18`「当前口径」表（**退役前最后一版 thread 口径的权威快照**）：
| 视图 | 当时口径 |
|---|---|
| 矩阵日格 | 该话题当日各分区内事件数的**最大值**（桶峰值） |
| 演进节点 / 话题趋势 | 同日各分区事件数**之和** |
| 行序 | `source` 全窗 `clueTotal` → `sectionTotal` → id |
| 雷达关注度 | 版块当日 `section` 数 ×2 + 近3日 + 净增 |
| 雷达「今天可能升温」 | 今日 `section` 数 ≥ 3 且 ≥ 近3日均值 2 倍 |

同一 proposal 的改写声明：
- `:38` 「矩阵与雷达换口径：矩阵日格改为『当日该话题的**事件线数**』，颜色深浅表达『当日最热事件线的参与者数』；行序与雷达关注度改为 `lane_heat`」
- `:57` 「`workshop-board-console`：矩阵日格与行序口径从『桶峰值 / 事件合计』改为『事件线数 / `lane_heat`』；`workshop-radar`：关注度与『今天可能升温』口径从『分区数』改为 `lane_heat` / `story_heat`」
- `:22` 事件线量纲的动机原文：「**计数单位随编辑策略漂移**。事件是 LLM 按 `k = clamp(round(lines/2.5), 2, 6)` 现场拆出来的，同一批内容换个 `k`，格值就变」

→ **现行 semantic-board 红线 28/29 正是本次被替代的量纲**（红线 28「矩阵/演进口径必须用事件线量纲」；红线 29「行序与雷达量纲固定：泳道行序 = lane_heat → story_count_14d → topic id」）。

### ③ 2026-11-09 matrix-clue-fallback-and-rail-merge —— 现行「兜底」口径
`openspec/changes/archive/2026-11-09-matrix-clue-fallback-and-rail-merge/proposal.md`：
- `:12` 触发原因：事件线回填只做到 2026-10-30，10-24～10-29 六天 `story_counts[i] = 0` 但当天明明有线索（132/73/118/123/131/118 条），格子全空
- `:19` 雷达小矩阵逐格降级 = `storyCounts[i] > 0 ? storyCounts[i] : counts[i]`，而 `RadarLane.counts[i] = len(day.Sections)` = **分区数**
- `:20` 控制台矩阵格值与时间筛选折线图同口径降级 = 事件线数 > 0 用事件线数，否则**当日线索合计**（各桶 `thread_count ?? events+folded` 之和）兜底
→ 结论：**「当日线索合计」在控制台矩阵口径下今天就已是现成量**，且已被 spec 认定为合法展示值（只是被降级为「事件线为 0 时的兜底」）。

## 初版的三个候选量纲（propose 阶段必须裁决）

| 候选 | 量纲 | 语义 | 数据可得性 | 与用户原话贴合度 |
|---|---|---|---|---|
| ① 分区数（section 数） | section | 该泳道该日几个主题桶 | 现成 `len(day.Sections)` | 低（用户明说 thread） |
| ② **桶峰值**（初版矩阵日格） | thread，max over 桶 | 「最热那个桶有几条线索」 | 现成 `max(sections[].thread_count)`，前端 `dayCellMetrics.peak` 已有 | 高（是「初版矩阵日格」）但语义是「峰值」不是「有多少条」 |
| ③ **当日线索合计**（初版演进节点/趋势） | thread，sum over 桶 | 「该泳道当天总共几条线索」 | 现成 `sum(sections[].thread_count)`，前端 `dayCellMetrics.total` 已有 | 最高（字面即「当天该泳道有多少条线索」） |

**推荐**：矩阵日格取 ③（当日线索合计），与用户原话字面一致、且与演进带/趋势图统一为同一量纲（不再出现「格值=合计、节点=峰值」的双口径）；② 作为备选。**不要选 ①**（用户明说 thread，且 ① 是「比初版更早」的分区数口径，add-event-story-heat 的 Why 已论证其「拆得细≠更热」缺陷）。

## 现成资产（切换成本极低）

- 后端 `LaneDynamicsTimelineDay` 已带 `Sections[].ThreadCount`（`lane_snapshot_repository.go:237-243` `LaneDynamicsTimelineSection.ThreadCount`），当日合计 = 各 section thread_count 之和；**不需要新 SQL、不需要新表列**。
- 前端 `dayCellMetrics`（`front/app/api/normalizers/laneDynamicsWindow.ts:184-224`）**已经同时算出**：
  - `peak`（同日各桶线索数最大值，null=全未知）
  - `total`（同日各桶线索数之和）
  - `storyCount`（当日事件线数）
  - `displayCount`（现行展示值 = storyCount>0 ? storyCount : total 兜底）+ `degradedFromClue`
  → **改回 thread 口径 = 把 `displayCount` 的主源从 `storyCount` 换成 `total`，并把 `degradedFromClue` 语义退役**；`peak` 可继续留在 aria/title 里作辅助指标（初版 aria 就是三指标：「桶峰值 X 条 · N 个主题桶 · 当日合计 M 条」）。
- 前端 `laneMatrixModel.ts:65 clueCounts` 与 `observationLanes.ts:79-94` 已按日聚合线索数（`clueTotal`/`dayClues`），行序也可直接回到 `clueTotal` 降序。

## 需要同步改写的 spec 文本

- `openspec/specs/workshop-board-console/spec.md`「泳道热度口径（2026-10-27 矩阵阶段修订）」Requirement（红线 28 的载体）
- `openspec/specs/workshop-radar/spec.md`「版块行构成与分档」（红线 29 的载体）
- `docs/reference/flow/semantic-board.md` 红线 28/29（事件线量纲 → thread 量纲）
- `docs/reference/flow/daily-report.md` 红线 42-49（事件身份列语义 / 今日重点选题源）

**引用**：openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/specs/workshop-board-console/spec.md:21、openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/specs/board-lane-dynamics/spec.md:14、openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/implementation-matrix-stage.md:64、openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/implementation-matrix-stage.md:125、openspec/changes/archive/2026-11-06-add-event-story-heat/proposal.md:12、openspec/changes/archive/2026-11-06-add-event-story-heat/proposal.md:38、openspec/changes/archive/2026-11-06-add-event-story-heat/proposal.md:57、openspec/changes/archive/2026-11-09-matrix-clue-fallback-and-rail-merge/proposal.md:19、openspec/changes/archive/2026-11-09-matrix-clue-fallback-and-rail-merge/proposal.md:20、front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics

<!-- pinned 2026-11-11T02:47:43Z -->

## 新口径数据可得性 + 日报同事件聚簇的向量可用性（含方案对比表）

【结论】**风险比预想小，但用户指定的「阈值 0.86 + related_event_embeddings」这条路走不通**。当天 event 标签的**另一种**向量（`topic_tag_embeddings`）在日报开始前**已经就绪**（日报等 embedding 队列清空）。真正的抉择是「用哪种表示 + 阈值是否可继承 0.86」。

## 关键事实一：日报调度时间是 **21:00**，不是 04:00

- `backend-go/internal/admin/scheduler/job_daily_report.go:214 dailyReportWindow` —— 读 `ai_settings.daily_report_time`（**默认 21:00**）与 `daily_report_deadline`（**默认 23:30**）；`:308 NextDailyReportTime` 用同一墙钟。
- 所以「当天新 event 标签在 04:00 前没向量」的担心在时间轴上不成立：日报跑在当天晚上 21:00，标签是白天打的。

## 关键事实二：日报生成**等 embedding 队列清空** —— 当天标签向量已就绪

- `job_daily_report.go:243 countQueuesDrained`：`models.TagJob` 无 pending/leased **且** `models.EmbeddingQueue` 无 pending/processing，两队列都空才放行（`:263 waitForDailyReportWindow` 循环；23:30 兜底强制生成时**不等**）。
- 打标链路**自动**产出标签向量：`tagmanagement/service/core/tagger.go:498-510` 在标签落库后调 `getEmbeddingQueueService().Enqueue(tagID)`；worker（`core/embedding_queue.go`，4 并发 / batch 32）产出三种向量（`core/embedding.go:25-27`）：
  - `identity` = label + aliases + category（**不含 description**）
  - `semantic` = label + ". " + description + aliases + category（**含 description**）
  - `event_keyword` = 逐关键词单行（仅 category='event'，`embedding_queue.go:126-135` 的 `allEmbeddingsCurrent` 会校验每个 keyword 都已算）
  - 文本组装唯一实现：`core/embedding.go:700 buildTagEmbeddingText`
- ⇒ **日报开始生成时，当天 event 标签的 `topic_tag_embeddings` 已经算完**（唯一例外：embedding 队列 failed 的标签、以及 23:30 兜底强制生成路径）。

## 关键事实三：`related_event_embeddings` 确实只有显式刷新才写 —— 用户指定的这条路走不通

- `tagmanagement/service/related/refresh.go:88 Refresher.RefreshBoard` 是**唯一写方**（注释逐字：「It is the ONLY writer of related_event_embeddings」）；CLI 入口 `cmd/related-events --refresh --board N`（`cmd/related-events/main.go:161 runRefresh`）。
- `related/representation.go` 包头 D4：「索引只由显式刷新生成；GET 路径永不调模型、永不写库」。
- 归档 change spec 逐字：「**不匹配当前签名的向量 SHALL 不参与相似计算，直到显式刷新成功**」（`archive/2026-11-11-enable-story-cross-day-continuation/specs/*/spec.md:39`）→ **当天新 event 标签在未 refresh 前没有 related 向量，聚簇覆盖率≈0**。
- 该表示 = label + description + 当前有效 aux 标签，版本 `related-event-v1`（`representation.go:41`），空间键含 generation（`related_event_index_generation`），文本变则失效。
- 阈值 0.86 是**针对这套表示**标定的：`representation.go:44-51` 注释——2026-11-10 board 1980 实测原起点 0.82 偏松（68% 明显相关 vs ≥0.86 时 100%），故抬到 0.86；阈值可经 `ai_settings.related_event_similarity_threshold` 在线改（`representation.go:44`）。
- 归档 explore-findings §边界（`archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:116-120`）已明说：**「label+description+辅助标签」的融合向量是全新表示，必须新建并重新标定阈值（不可套 C1 的 min3 结论）**。

## 关键事实四：报告几何向量在日报里必然现成（备选表示）

- `daily_report_sections.embedding` = 内容化文本向量（`topicgraph/service/daily_report_embed_text.go:buildSectionEmbedText`：各 tag 的 label+description+代表文章摘录拼接）；`daily_report_threads.embedding` 同理（`daily_report_repository.go:314,336,665,693` 补算）。
- 这两个是**报告几何**向量（回答「这两坨材料像不像」），不是「是不是同一件事」——`lane-compose-story-chain` 的 proposal 已用这段论证批评过它。**可作降级兜底，不宜作主口径**。

## 方案对比表（propose 阶段拍板用）

| # | 方案 | 向量来源 | 当天可得 | 阈值 0.86 可否直接用 | 新增模型调用 | 落库 | 主要代价 / 风险 |
|---|---|---|---|---|---|---|---|
| **A** | 复用 `related_event_embeddings`（related-event-v1 表示） | 显式 CLI refresh | ❌ 当天新标签无向量 | ✅ 可（0.86 就是为它标的） | 无（但需在日报前跑一次 refresh） | 需写缓存（与「不落库」冲突） | 必须把 refresh 挪进日报链（增量只算当天新标签，`Refresher.classify:227` 会复用 text_hash 命中的旧行）；等于把「索引刷新」变成日报的前置步骤，与「不落库」冲突，且 refresh 是全板有效 event 标签级（窗口内），成本与板规模相关 |
| **B** | 复用 `topic_tag_embeddings.semantic`（label+description+aliases+category） | embedding 队列自动 | ✅ 日报前已算完 | ❌ **必须重新标定**（表示不同，0.86 无依据） | 无 | 无 | 零新增调用、零延迟、当天新鲜；但阈值要新标定（需跑一次类似 `related.Experiment` 的只读标定），且 semantic 表示含 aliases/category，event 标签本身已是主谓宾长句 → 语义可能偏「标签身份」而非「事件」 |
| **C** | 复用 `topic_tag_embeddings.identity`（label+aliases+category，无 description） | embedding 队列自动 | ✅ | ❌ 需新标定 | 无 | 无 | 同 B，但表示更「硬」；event 标签 label 本身就是完整主谓宾句（平均 20.3 字，见 `archive/2026-11-06-add-event-story-heat/proposal.md:28`）→ identity 可能已经够用；与 dedupe 用的 identity 向量同空间，**与 tag 合并判定的相似度混淆风险**（红线 semantic-board 1-2） |
| **D** | 日报 job 内联现算当天 event 标签的 related-event-v1 向量，**只在内存聚簇不写库** | 日报内调 airouter Embed | ✅ 当天新鲜 | ✅ 可（同一 `BuildRepresentation`，同一 0.86） | ✅ 每板 1 次批量 embed（batch 32，30s 超时） | ❌ 不落库（符合用户约束） | 表示与阈值完全对齐用户口径；代价：新增模型调用（必须带非空 Operation → 写 `ai_call_logs` 审计，ai-summary 红线 1/7/8）、日报尾段延迟、审计量上升。**先例**：`related.Experiment`（`related/experiment.go:95`）就是「只读、只调模型、内存算、不写库」的现成模式，可照抄 |
| **E** | 降级为主体近似（不聚簇，按 `event_fingerprint` 的 subject/action/object 或 subject_key 机械分组） | 报告内已有 metadata | ✅ | N/A | 无 | 无 | 零依赖零延迟；但 subject_key **已知漂移**（`normalize-event-subject-entities` 实测：495 distinct key 里 `person:特朗普` 23 条压过 `person:trump`；`company:deepseek` 与 `product:deepseek` 分属两型），且归档 explore-findings 附加发现列出 4 种 key 形态漂移 → 主体近似质量不可靠，只能当兜底 |
| **F** | 推迟聚簇到 refresh 之后，日报用上一日快照 | related 缓存 | ❌ | ✅ | 无 | 写缓存 | 与「当天热度信号」语义直接冲突（用户要的是当天），不建议 |
| **G** | 改用 section/thread 向量聚簇 | 日报自算 | ✅ 必然有 | ❌ 需新标定 | 无 | 无 | 零新增调用；但单位从「event 标签」变成「分区/线索」，偏离用户口径（「同版块当天 event 标签两两相似」），且相关但不等价的证据已在 `lane-compose-story-chain` proposal 里被论证过 |

### 推荐组合
- **主口径 = D**（日报内联现算 related-event-v1 向量 + 内存聚簇 + 不落库）：唯一同时满足「当天新鲜」「阈值 0.86 有依据」「不落库」「不做身份」的方案，且有 `related.Experiment` 的现成只读先例可照抄。
- **兜底 = C 或 B**（读 `topic_tag_embeddings` 当天已有的向量），在 D 的模型调用失败/超时/无路由时降级，并把「聚簇降级口径」如实标注（不许静默把 D 的结果与兜底结果混为一谈）。
- **再兜底 = E**（主体近似），仅在向量完全不可得时用，且必须在界面上明说「本次按主体近似，非向量聚簇」。
- **A 只在需要「阈值 0.86 已标定且零新增调用」时被选中**，代价是必须把 refresh 变成日报前置步骤（与「不落库」直接冲突），且 refresh 面向全板有效 event 标签而非「当天命中标签」。

### 其它必须一并裁决的实现细节
1. **「簇内文章数」的文章去重**：`daily_report_threads.related_article_ids` 是 tag→文章并集；同一篇内容被多 feed 抓取会重复计数（活跃 change `fix-duplicate-article-heat` 的 Why 已取证 505 组 / 1038 行重复副本）。新热度若按「文章数」计，必须按内容身份去重（与 `story_signals.content_key` 同一口径：`heatParticipantKey`，`story_heat.go:53`），否则热度立刻虚高——这正是 story 层刚修完的坑。
2. **「当天」的时区口径**：全仓统一 CST（UTC+8）日历日，`story_heat.go:21 storyHeatZone`；日报 `period_date` 也是 NormalizeReportDate 的日历日。新口径必须沿用同一日界。
3. **「同版块当天」的候选集边界**：用户要 event 标签**两两**相似 → O(n²)。当天某版块的 event 标签数可达数百（`add-event-story-heat` proposal 记 event 标签全库 10,077）→ 需要分块/近邻策略，不能朴素全对比。
4. **cluster 里的「文章数」与「热度信号」的关系**：用户口径是「当天多篇文章命中同一事件 → 热度」，即簇内文章数 = 热度。所以「文章数」不是「事件线数」，而是**跨 event 标签聚合后的文章数**——即同一篇文章可同时计入多个簇（若它同时命中多个标签）。需在 spec 里写清去重边界（文章去重 or 标签-文章对去重）。
5. **审计与合规**：D 方案必须走 airouter（红线 ai-summary 1：Operation 非空；7：每次调用无论成败写 `ai_call_logs`；8：截断保护）。`related.Operation = "tagmanagement.related_event.embedding"` **故意不在 embedding 缓存白名单**（`representation.go:70-72`）→ 每次都是真实调用，日报每日每板会新增真实调用量，需评估。

**引用**：backend-go/internal/admin/scheduler/job_daily_report.go:dailyReportWindow、backend-go/internal/admin/scheduler/job_daily_report.go:countQueuesDrained、backend-go/internal/tagmanagement/service/core/tagger.go:498、backend-go/internal/tagmanagement/service/core/embedding.go:700、backend-go/internal/tagmanagement/service/core/embedding_queue.go:allEmbeddingsCurrent、backend-go/internal/tagmanagement/service/related/representation.go:41、backend-go/internal/tagmanagement/service/related/refresh.go:88、backend-go/internal/tagmanagement/service/related/experiment.go:95、backend-go/internal/tagmanagement/models/related_event_embedding.go、backend-go/internal/topicgraph/service/daily_report_embed_text.go:buildSectionEmbedText、openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:105

<!-- pinned 2026-11-11T02:48:25Z -->

## 热度换口径的波及面（半衰期实现位置 / 受影响红线 / 话题升温无关）

【结论】热度换口径**不需要迁移任何数据**（热度无落库历史表，纯函数现算）；新公式反而更容易回算历史。**话题升温判定不依赖 story 热度**（用 hit_count + upgrade_threshold），无需改话题生命周期。会受影响的红线集中在 semantic-board 28/29/30 与 daily-report 42-49。

## 现有半衰期实现位置（全部集中在一个文件）

`backend-go/internal/topicgraph/repository/story_heat.go`：
- `:13-19` 常量：`HeatRuleVersion = "heat-v1-48h-hl24h"`、`StoryHeatHalfLifeHours = 24`、`StoryHeatWindowHours = 48`、`LaneHeatHalfLifeHours = 72`、`LaneHeatWindowDays = 14`
- `:21 storyHeatZone` = `time.FixedZone("CST", 8*3600)`（全仓统一日界口径）
- `:110 loadBoardHeatEvidence(boardID, since, until)` —— 一次载全板证据：`board_stories` 身份 + `story_signals` 时间窗（`:174-181` JOIN articles，**从不按 archived 过滤**，注释：归档只是退出 feed 活跃窗，报导事实仍计入热度）
- `:187 ComputeBoardHeat(boardID, at)` → 返回 `(storyHeat, laneHeat)` 两张 map；laneHeat **不是** storyHeat 的和（独立 72h/14d 衰减）
- `:285 boardHeatEvidence.compute(at)` —— 核心公式：`storyHeat[story] += 0.5^(Δh/24)`（Δh ≤ 48h 窗口内）；`laneHeat[lane] += 0.5^(Δh/72)`（窗口 14d）
- `:323 ComputeBoardStoryHeatByDay(boardID, days)` —— 逐日 CST 日终的 story_heat 合计（雷达 today/last3/prev3/heat_days 用）
- `:266 RecomputeHeatForBoard(boardID, at, now)` —— 落后源自愈重算入口
- `:222 BoardHeatCoverage(boardID, now)` —— 落后源标记（`heat_incomplete` / `heat_stale_feed_ids`），阈值 `max(3×refresh_interval, 90min)`（`:204-210`）
- `:53 heatParticipantKey(contentKey, participantKey)` —— **参与者去重口径唯一实现**（内容身份优先、feed 兜底；SQL 侧等价式在 `story_signals` 写入处）

## 相关表 / 字段 / 快照现状

| 载体 | 位置 | 角色 |
|---|---|---|
| `board_stories` | `topicgraph/repository/story_models.go:34-72` | 事件线身份 + `thread_count/article_count/participant_count` + `embedding/subject_embedding` + `merged_into` |
| `story_signals` | `story_models.go:84-104` | 热度证据（复合主键 `(story_id, article_id)`），`participant_key` + `content_key` + `observed_at` + `observed_at_source` |
| `daily_report_threads.story_id` | `daily_report_models.go:210` | 事件→事件线的身份列 |
| **热度历史表** | **不存在** | 热度**不落库**：`heat_days` / `today_heat` 每次请求现算（`ComputeBoardStoryHeatByDay`）。`archive/2026-11-06-add-event-story-heat/proposal.md:78` 明写「旧口径的历史热度不会自动转换，因为此前热度未落历史表」 |
| `topic_lane_snapshots` | `lane_snapshot_repository.go` | 泳道**滚动态势句**快照（as_of + summary），与热度无关，**不受本 change 影响** |

## 新公式落点

用户口径：**关注（active）泳道当天线索数 ×1.0 + 候选（candidate）泳道当天线索数 ×0.5**，只表达当天热度，权重可经 ai_settings 在线调。

- 计算实现落点：**新写一个纯函数**，取代 `ComputeBoardHeat` 的 laneHeat 半支（storyHeat 半支随 story 退役一起退场）。建议落在 `lane_snapshot_repository.go` 或新建 `lane_heat_repository.go`，**不复用 `story_heat.go` 的名字**（`HeatRuleVersion` 字符串要换新版本号，如 `heat-v2-lane-clues-1d`）。
- 数据来源：`daily_report_sections`（`persistent_topic_id`）+ `board_daily_reports`（`period_date`）+ `daily_report_threads`（section_id）→ 每泳道当天 thread 数。**这是报告几何口径，与 story 身份完全解耦** —— 好消息：报告数据是持久化的，**任意历史日都可回算**（不像现在的热度依赖 story_signals 的存量）。
- **candidate 泳道的坑**：`GetBoardLaneDynamics` 的泳道集合是 **active ∪ watch-linked**（`lane_snapshot_repository.go:709-711`），candidate 只出现在 `resp.Candidates`（`:304`）且**不带 timeline / thread 数**。但 DB 层 candidate 话题**确实有锚定 section 与 thread**（候选栏的 hint 查询就是从 `daily_report_sections` 取的，`:852-859`）。→ 新公式要算 candidate×0.5，**必须新增一条按 topic_id 聚合 thread 数的查询**（覆盖 active + candidate 两类），不能只改前端消费。
- 「当天」的日界：沿用 CST（UTC+8）日历日（与 `storyHeatZone`、`NormalizeReportDate` 一致）。
- 权重在线调：`ai_settings` 直读是既有先例（`related/representation.go:234 LoadThreshold` 读 `models.AISettings` 的 `related_event_similarity_threshold`，缺项→默认、坏值→告警回退默认、域为 [0,1]）。新增 `lane_heat_candidate_weight` 走同一模式即可；若要出现在设置 UI，则要走 `GET/PUT /api/semantic-boards/matching-config` 的白名单（`tagmanagement/handler/board_crud_handler.go:138-139` + `isSemanticBoardConfigKey` ≈:993 + `validateSemanticBoardConfigValue` ≈:1000-1019，默认分支限 0..1 浮点）。

## 会因此改写的红线

**semantic-board.md**
- **红线 28**「矩阵/演进口径必须用事件线量纲：矩阵日格 = 当日事件线数（同日多主题桶合并计数，MUST NOT 取桶峰值）；矩阵格颜色 = 当日最热事件线的独立参与者数（max_participant_count，按内容身份去重）分档且有图例与 title/aria；演进/趋势节点 = 当日事件线数；section 数、桶数、thread 条数 MUST NOT 参与任何热度或排序计算」 → **整条反向**：改回 thread 量纲（矩阵日格 + 演进节点 + 颜色通道）。
  - 附带的「展示降级」子句（`matrix-clue-fallback-and-rail-merge`）也随之失效：`story_counts` 缺失回退、`storyCountsKnown=false` → 「N 个分区」措辞、降级格 title/aria —— 因为不再有「事件线数 0 时按线索合计兜底」这层。
  - **颜色通道无源**：颜色原本是「当日最热事件线的独立来源数」，事件线退役后没有「最热事件线」→ 要么另定量纲（如当日线索数分档），要么退役颜色通道（但红线 28 要求「颜色 SHALL NOT 是唯一信息载体」+ 有图例 + title/aria → 需重新表述为「本版无颜色通道，数值 + title/aria 承载全部信息」）。
- **红线 29**「行序与雷达量纲固定：泳道行序 = lane_heat 降序 → 全窗事件线数（story_count_14d）降序 → topic id 升序；雷达关注度 = 版块各泳道 lane_heat 之和（保留一位小数，lane_heat 按 72h 半衰期 / 14 天窗口独立衰减…）；「今天可能升温」= 某条事件线自身的锚点日 story_heat ÷ 其自身近 3 日基线 ≥2…」 → **整条重写**：行序次键（story_count_14d）换 thread 合计；关注度换新公式；「今天可能升温」**整档消失**（它是事件线级判据）。
  - 前端分档常量必须重标：`front/app/features/tags/components/console/boardRadarHelpers.ts:12-13` `RADAR_HOT_ATTENTION=3.3` / `RADAR_WARM_ATTENTION=0.7`（现值按 lane_heat 真实分布 P80/P20 标定）。新量纲下必须重新标定，且红线 29 逐字禁止「沿用旧 section 量纲的 50/15」。
- **红线 30**「事件身份未回填必须保守降级并明示…」 → 随事件身份退场整体退役（`unbackfilled_board_ids` / `no_story_board_ids` / `story_identity_state` 三态全退）。
- 红线 34「相关事件只做同版块直接相关、不改任何身份…」→ **不动**（related-events 与 story 无关，但它正是新聚簇的口径参照）。

**daily-report.md**
- 红线 42「事件身份列语义固定：`scope` 由 `tag_ids` 中 event 标签数推导（≥2=composite），composite 事件永不建/挂事件线、不计 thread_count、不进热度证据；`story_id` 可空且未归属事件照常展示」 → 改写（story_id 退役；scope 仍需要吗？—— 需裁决：scope 是从 event 标签数推导的**独立于 story 的**语义，综合稿判定在「同事件聚簇」口径下仍然有用（综合稿不该算进聚簇热度）→ 建议保留 scope，但删掉 story_id 半句）。
- 红线 43「`story_manual` 人工标记永不被自动覆盖」→ 随 story 退役，该列变只读历史列。
- 红线 44「`display_tier` 只影响展示：身份（story_id）、signals、热度永不读该列」→ display_tier 的存在理由是跨期去重降级，story 退役后**它失去唯一写入方**（`applyStoryDedupForReport` 依赖 story_id）→ 整列退役或保留为历史。
- 红线 45「跨期去重判定口径固定：历史窗口 = 同事件线最近 7 天…重叠率 ≥0.6 且无新 feed → 降级 flash」→ 随 story 退役整条退役（用户口径明确「不跨天」）。
- 红线 46「今日重点选题必须以真实热度候选为源，服务端/模型都不得凭空选题或伪造快照」→ 保留精神，**换源**：候选源从「事件线 Top3」换成「当天同事件聚簇」。
- 红线 47「重建日报必须重算受影响事件线的 signals 与计数，否则热度残留旧证据」→ 随 story 退役重写（新热度从报告几何算，重建日报自然重算，无残留问题 → 这条可改写成「重建日报必须重算聚簇热度，事件线残留不再是问题」）。
- 红线 49「参与者身份必须按内容身份去重、补档扫描窗口必须等于报告期日」→ 前半句随 story 退役，但**「内容身份去重」在新口径下仍然必须做**（簇内文章数按 content identity 去重）→ 建议保留并改写为「聚簇热度内的文章数 MUST 按内容身份去重（与 `heatParticipantKey` 同口径）」。

**topic-graph.md** —— **不受影响**：话题生命周期（candidate 连续命中 `upgrade_threshold` 默认 3 → 人工激活资格；`persistent_topic_candidate_decay_window`；手动建话题直落 active）全部走 `hit_count` / `consecutive_hits` / `upgrade_threshold`（`daily_report_topic_repository.go:66,186,385,608-610`；`daily_report_assignment.go:228,237,311`），**不读 story_heat / lane_heat**（已 grep `daily_report_manual_topic.go` / `daily_report_llm.go` / `tagmanagement/service/board/` 均无热度引用）。
- **重要**：新公式里的 **candidate×0.5** 是**纯热度加权**，绝不能被误读成「candidate 参与生命周期判定」—— 话题状态机红线（topic-graph 1-4）一字不动。

## 其它会受热度量纲变更牵动的点

- `lane_snapshot_repository.go:749-758` —— lane_heat 的取数点（全板一次取），换新公式。
- `lane_snapshot_repository.go:830-838` —— 泳道行序（lane_heat → story_count_14d → id）→ 换次键。
- `board_radar_repository.go:414-442` —— 雷达行 `Attention`（Σ lane_heat）、`row.Lanes` 排序键（`:429-431` 按 StoryCounts 之和）。
- `board_radar_repository.go:444-456` —— `today_heat/last3_heat/prev3_heat/heat_days`（story_heat 逐日）→ **整段退役**（story 退役后无 story_heat）。
- `board_radar_repository.go:507-525 finalizeRadar` —— 版块行排序按 `Attention` 降序 → 换新量纲后需重新标定分档。
- `front/app/api/normalizers/boardRadar.ts:44-51,149-164` —— `today_heat/last3_heat/prev3_heat/heat_days/heat_rule_version/heat_incomplete` 字段 → 增删 + 旧后端兼容分支。
- `front/app/features/tags/composables/useBoardRadarOverview.ts:69-79` —— `warming` 派生（todayHeat/last3Heat 环比）→ 随「今天可能升温」整档退役。

**引用**：backend-go/internal/topicgraph/repository/story_heat.go:13、backend-go/internal/topicgraph/repository/story_heat.go:110、backend-go/internal/topicgraph/repository/story_heat.go:187、backend-go/internal/topicgraph/repository/story_heat.go:222、backend-go/internal/topicgraph/repository/story_heat.go:266、backend-go/internal/topicgraph/repository/story_heat.go:285、backend-go/internal/topicgraph/repository/story_heat.go:323、backend-go/internal/topicgraph/repository/story_models.go:34、backend-go/internal/topicgraph/repository/story_models.go:84、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:749、backend-go/internal/topicgraph/repository/board_radar_repository.go:414、backend-go/internal/topicgraph/repository/board_radar_repository.go:444、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:66、backend-go/internal/tagmanagement/service/related/representation.go:234

<!-- pinned 2026-11-11T02:49:11Z -->

## 数据与迁移面（只读保留 / story_id 语义 / 前端入口 / 立项证据索引）

【结论】旧数据处置只需「**停写 + 只读保留 + 不删**」三件事，**零迁移**。唯一需要新写代码的是「事件线清单 API 的前端入口退场」和「CLI 写模式下线」两处收口。

## 1. `board_stories` 旧数据（只读保留）

- 表结构：`backend-go/internal/topicgraph/repository/story_models.go:34-72`（`BoardStory`）；迁移 `platform/database/postgres_migrations.go:4567`（`add-event-story-heat` 建表 + CHECK + FK + 部分索引，迁移号 20261105_0001）；`daily_report_register_models.go:23` 注册 GORM 模型。
- **不需要迁移**：表留着、列留着、数据留着；`GORM AutoMigrate` 仍会维护它（`topicgraph/repository/daily_report_register_models.go` 注册了 `&StorySignal{}` 等），但没有任何写路径会再产生新行。
- 退役后仍可只读消费的字段：`board_stories.id/title/status/merged_into/first_seen_date/last_seen_date/thread_count/article_count/participant_count`。
- 风险点：GORM `AutoMigrate` 会对这些表持续做列比对（若代码里保留模型）。若希望**彻底只读**，可选：(a) 保留模型但不再调用写方法（最小改动）；(b) 移除模型注册 → 表就完全脱离 GORM 管理（**但会失去 AutoMigrate 的 NOT NULL/CHECK 兜底，且历史数据仍在**，需确认没有其它代码读这些表）。建议 (a)。
- **必须保留**：`daily_report_threads.story_id` 列（FK ON DELETE SET NULL 到 `board_stories`）。历史日报的 threads 仍带 story_id，读路径 `resolveReportHighlightIdentities`（`report_story_highlights.go:71`）会把它映射到存活者；退役后应停止对外暴露（前端不再消费 `story_id`），但列与 FK 不动。

## 2. `daily_report_threads.story_id` 字段语义（退役后的处置）

- 定义：`backend-go/internal/topicgraph/repository/daily_report_models.go:206-210`（可空、索引 `daily_report_threads_story_idx`、FK ON DELETE SET NULL，迁移 20261105_0001 固化）。
- 语义：**事件身份列**（thread → 事件线），composite 线程恒为 NULL（红线 42：`scope` 由 event 标签数 ≥2 推导，综合稿永不建事件身份）。
- 退役后：**列保留、只读、不再写入**。新生成/重建的日报该列恒 NULL。
- 连带：`scope` 列（`daily_report_models.go:216`）—— 它**不依赖 story**（从 `tag_ids` 的 event 标签数推导），在新「同事件聚簇」口径下**仍然有用**（综合稿不应计入聚簇热度）。建议保留 `scope`，只退役 `story_id`/`story_manual`/`display_tier` 的 story 语义。
- `story_manual`（`:224`）：退役后为恒 false 的历史列（人工拆分过的行仍为 true → 只读保留，不触发任何逻辑）。
- `display_tier`（`:233`）：唯一写入方是 `applyStoryDedupForReport`（跨期去重降级，依赖 story_id）→ 退役后恒 `body`，列保留。

## 3. 事件线清单 API 的前端入口

| 入口 | 位置 | 处置建议 |
|---|---|---|
| 雷达右栏「今日该读」清单 | `front/app/features/tags/components/console/BoardRadarReadingRail.vue`（`storyMode` 分支 :124-172，`storyTitle` :55，`isNewStory` :77，`isWarmingStory` :81，`isStaleHeat` :98，`canOpen` :86） | **整段退场**；组件保留文件 |
| 雷达行「事件线数 / 最热来源数」文案 | `front/app/features/tags/components/console/BoardRadarStory.vue`（`todayCount` :47、`anchorText` :49、min矩阵 `laneCells` :~55+） | 改口径（数量改线索数）+ 删「事件线」措辞 |
| 雷达总览 composable | `front/app/features/tags/composables/useBoardRadarOverview.ts`（`stories` :36、`warming` :69、`crossDayBuilding` :91、`topStories` :96、`unbackfilledBoardCount` :98、`partialStories` :100、`laggingBoardCount` :104、`openStory` :156） | 逐项退场 |
| 雷达 API 归一 | `front/app/api/normalizers/boardRadar.ts`（`stories` :171、`RadarStoryItem` :178-196、`storyCounts` :129、`storyCountsKnown` :130、`todayStories/Last3/Prev3` :149-152、`todayHeat` 等 :157-164） | 字段增删 + 旧后端兼容分支同步 |
| 日报刊头「今日重点」定位按钮 | `front/app/features/tags/components/daily-report/DailyReportMasthead.vue`（`onLocate` :25、`highlight.story_id` :70、`is_progress` :75） | 换口径后 `story_id` 失效 → 定位改为按聚簇 id 或取消定位 |
| 日报重点派生 | `front/app/features/tags/components/daily-report/dailyReportMagazine.ts`（`highlightEvidence` ~:170、`reportHasStoryThread` :161、`selectLeadStory` :191） | 改口径 |
| 日报阅读器 `?story=` 深链 | `front/app/features/tags/components/DailyReportReader.vue`（`initialStoryId` :58、:314、`handleLocateHighlight` :354） | 深链参数退场（需确认 URL 参数不破坏旧链接 → 建议保留解析但忽略） |
| 日报 API 类型 | `front/app/api/dailyReports.ts`（`DailyReportHighlight` :6-21、`DailyReportThread.story_id` :39） | 字段增删 |
| 泳道编排「事件线链」 | `front/app/features/tags/composables/useLaneCompose.ts` + `components/compose/LaneComposeCandidates.vue` | **与活跃 change `lane-compose-story-chain` 直接冲突** → 必须一并裁决（见下） |
| 矩阵行 aria 文案 | `front/app/features/tags/components/console/ObservationMatrixLaneRow.vue:38`（「N 个事件线 · 最热 N 个来源 · 未归组 N 个事件 · N 个主题桶」） | 改文案（去掉事件线/最热来源/未归组） |
| 演进带文案 | `front/app/features/tags/components/console/BoardLaneEvolution.vue:153,189,194,225`（「N 条事件线」「最热 N 个来源」） | 改文案 |
| 矩阵格 aria | `front/app/features/tags/components/console/observationLanes.ts:191-237 matrixCellView`（countSegment / heatSegment / label） | 改文案 + 去「未归组事件数」（该字段来自 story 身份） |

**后端对应**：`GET /api/semantic-boards/overview` 的 `stories` 段（`board_radar_repository.go:94 RadarOverview.Stories`）应停止下发或整体置空；`GET /api/daily-reports/:id` 的 `highlights[].story_id` / `story_heat` / `heat_rule_version` / `heat_as_of` / `heat_incomplete` 与 `sections[].threads[].story_id` 应停止暴露。

## 4. 归档 change `2026-11-11-enable-story-cross-day-continuation` 的立项证据（**引用，不复述**）

本 change 的立项依据全部来自该归档 change，**实现/评审阶段直接引用原文，不重述数据**：

| 证据 | 精确位置 |
|---|---|
| 跨天事件线断裂三根因（缺 title 向量永久退出召回池 / 数值型 object 致 fingerprint_trusted=false / 跨天 stage 日期基准=建线原点日） | `openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:6-27` |
| subject_key 提取不稳定（同一产品 4 种 key，类型前缀漂移） | 同上 :23-25 |
| 判定链对照（指纹全同的跨天挂载本有通路，死在召回盲区与信任闸门） | 同上 :27 |
| 实体层落地后的三处适配缺口 | 同上 :31-47 |
| E1/E3 测算口径（可直接复用的工具与算法） | 同上 :49-66 |
| E1–E3 实测结论：收益、误合并风险与 D14 加固 | 同上 :67-82 |
| C1 标定结论（三者独立 T≈0.80 可行、拼短语不占优、实体层可弃） | 同上 :83-104 |
| **event 标签 embedding / 辅助标签 / 关系表 可复用点与边界** | 同上 :105-126 —— 含 `topic_tag_embeddings` 三种 embedding_type 与文本组装口径、`semantic_labels` 双向量、`daily_report_section_relations` 先例、`ai_settings` 在线阈值读法、参数暴露 API/UI 路径，以及**「融合向量是全新表示，必须新建并重新标定阈值」**的边界 |
| 旧 event-story / story-heat-evidence delta 的撤回方式 | 同上 :128-151 |
| UI 落点建议与遗留实验处置（D13/回填工具/实体 off） | 同上 :152-172 |
| 当前有效方案：event 标签 embedding 直接相关（取代旧身份归并） | 同上 :173+ |
| related-events 的「不承诺新 event 入库后立即可查」口径 | `archive/2026-11-11-enable-story-cross-day-continuation/specs/*/spec.md:13,39` |
| 该 change 的重做声明（旧身份归并方向已撤回、不作为新方案要求） | `archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:2-4`（+ `legacy-experiments.md`） |

## 5. 与其它活跃 change 的冲突（propose 阶段必须裁决）

| 活跃 change | 建档 | 冲突点 |
|---|---|---|
| `lane-compose-story-chain` | 2026-11-07 | **直接冲撞**：它正在给泳道编排新增「事件线链」维度（按事件线存活 id 分组跨天分区）。story 退役 → 该维度失去数据源。**必须与该 change 的 owner 协调**（它可能尚未实现，或需回退）。 |
| `fix-duplicate-article-heat` | 2026-11-10 | 改 `story_signals` 参与者去重口径（内容身份优先）。若 story 退役，该 change 的 story 部分失去对象；但它同时修 `MaterializeKeywordWatches` 的日期穿透（A 部分，与 story 无关，**必须保留**）。 |
| `normalize-event-subject-entities` | 2026-11-09 | 实体层默认 `off`、**不接入事件线判定** → 与 story 退役解耦；但其 spec 文本仍引用「事件线身份判定」措辞，退役后需同步改写文档措辞（实现不变）。 |
| `unify-vector-column-dimension` | 2026-11-04 | 向量列维度统一 —— 影响 `related_event_embeddings` / `topic_tag_embeddings` 的列型，本 change 若新增向量使用面需对齐。 |
| `description-repair-batch-matching` / `add-home-pipeline-status-page` / `add-server-capability-negotiation` / `fix-disaster-recovery-channel` | — | 与 story 无关。 |

## 6. 迁移面小结

- **DDL 迁移**：**不需要新迁移**（新热度、新聚簇都从既有表算）。若要加 `ai_settings` 新键（如候选权重），走 `models.AISettings` upsert（既有模式，`related.SetGeneration`）。
- **数据回填**：**不需要**。旧 `board_stories` / `story_signals` 只读保留即可。
- **CLI**：`backend-go/cmd/backfill-stories/main.go` 的写模式（`--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings`）应在退役后**下线或显式拒绝**，否则「停写」可被一条命令绕过。`--dry-run` 预览模式可保留（只读）。
- **建线回填 CLI 的另一条**：`cmd/backfill-stories` 之外还有 `cmd/rebuild-topics` / `cmd/backfill-fingerprints` / `cmd/entity-backfill`（后者属实体层，与 story 无关）。
- **回滚面**：因为旧数据只读保留 + 无新迁移，回滚 = 把停写开关与读端口径改回去，**不需要数据修复**。

**引用**：backend-go/internal/topicgraph/repository/daily_report_models.go:188、backend-go/internal/topicgraph/repository/story_models.go:34、backend-go/internal/topicgraph/repository/story_models.go:84、backend-go/internal/topicgraph/repository/daily_report_register_models.go:20、backend-go/internal/platform/database/postgres_migrations.go:4567、backend-go/internal/platform/database/postgres_migrations.go:4599、backend-go/internal/platform/database/postgres_migrations.go:4753、backend-go/internal/topicgraph/repository/daily_report_repository.go:284、backend-go/internal/topicgraph/repository/daily_report_repository.go:406、backend-go/cmd/backfill-stories/main.go:1、openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:6、openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:105

<!-- pinned 2026-11-11T02:49:32Z -->
