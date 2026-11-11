<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, topic-graph, daily-report, scheduler, ai-summary -->

# 退役事件线，改用 event 标签与当天线索热度

## Why

story（事件线）这一层把「今天有几件事、哪件事几个来源在报」交给了模型能力 + 人工修剪：召回靠向量、判定靠指纹与阶段、跨天靠建线原点日。归档 change `enable-story-cross-day-continuation` 的实测把三条死因钉死了（`openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:6-27`）：

- **召回盲区**：缺 title 向量的事件线永久退出召回池（活跃线 1262 条中 111 条 `embedding IS NULL`），同 key 同 action+object 的 **25 对**跨天候选**全部**被挡在召回门外（同文件 `:71`）。
- **信任闸门**：数值型 object（汇率、人数）永远命不中关键词 → `fingerprint_trusted=false` → 合并闸门直接拒绝（同文件 `:15`）。
- **日期基准**：跨天 stage 的日期基准 = 建线原点日 → 长链第 2 天即 dateVeto（同文件 `:19`）。

叠加主体键漂移：同一规范名对应 >1 个 `subject_key` 的组 **138** 组（跨天出现 95），类型前缀在 product / person / org / other 之间漂移，`NormalizeSubjectKey` 无法归一 → 主体一票否决挡掉本可同线的候选（同文件 `:51`）。指纹全同的跨天挂载**本来有通路**，死在召回盲区与信任闸门，不是判定规则本身（同文件 `:27`）——也就是说继续修 story 是在补三个各自独立的漏，而不是补一条主路。

用户原话（2026-11-11）：「story 这一层太依赖模型能力和修剪了，从产品设计上不可取，雷达/热度/日报用到的 story 我打算换成现在的 event 标签以及其包含的辅助标签」。

本 change 退役 story 的**在线生产**，把雷达矩阵、泳道热度、雷达关注度、日报「今日重点」四处度量口径换成 event 标签与当天线索几何；旧 story 数据只读保留，不删表、不迁移。

## What Changes

- **story 硬停写（不删实现库、不删表）**：日报链尾部的在线归属判定入口及其调用整体摘除（review R-1 用户拍板：删除 `story_attribution_enabled` 开关，不再有灰度态），新生成 / 重建的日报恒不产生 `board_stories` / `story_signals` / `daily_report_threads.story_id`；归属判定函数库与写端实现完整保留（`attributeStoriesForReportWithStats` 及 story 写方法仍在，CLI 仍可编译），不删表、不做数据迁移。回滚 = `git revert` 本 change 的提交，不是切换配置键。
- **回填与重算入口锁写**：`cmd/backfill-stories` 的写模式（`--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings`）退役后直接拒绝，只读 / 实验模式保留，防止「停写」被一条命令绕过。
- **矩阵日格 = 当日线索合计**：矩阵日格、演进节点、顶部时间视窗曲线统一改回 thread 量纲（该泳道该日各主题桶线索数之和），不再取事件线数、不再取桶峰值；**颜色通道整体退役**，数值 + title/aria 承载全部信息。
- **热度换口径**：泳道当天热度 = 活跃泳道当天线索数 × 1.0 + 候选泳道当天线索数 × `lane_heat_candidate_weight`（默认 0.5，域 [0,1] 可在线调）；版块关注度 = 各泳道当天热度之和。只表达当天热度，废半衰期、废跨天累积；候选权重**不参与**话题生命周期判定。
- **雷达右栏换内容**：「今天可能升温」整档退场，右栏换成「当天线索最多的泳道」清单（跨版块混排，每条带版块名 + 泳道名 + 当天线索数）；「新起 / 升温 N×」行内徽标与跨天延续脚注一并退场。
- **日报同事件聚簇（方案 D）**：日报生成时内联现算当天 event 标签的 related-event-v1 向量，内存聚簇，不落库、不做身份、不跨天；阈值沿用 `related_event_similarity_threshold`（默认 0.86，可在线调）；每版块新增 1 批 embedding 调用，走 airouter 并落 `ai_call_logs` 审计；向量不可得时按「语义向量兜底 → 主体近似兜底」如实降级并标注口径。
- **今日重点候选源换聚簇**：「今日重点」仍调 LLM，候选源从事件线 Top 8 换成当天同事件聚簇 Top 8；簇内文章数按**内容身份**去重（同一篇内容被多个 feed 抓取只算 1 篇），prompt 重写并禁止编造；`?story=` 深链退场，定位改按聚簇。
- **旧数据只读保留**：`board_stories` / `story_signals` / `daily_report_threads.story_id` 列与数据全部保留，只是不再写入、不再对外透出事件线概念；`scope`（综合稿判定）保留并继续生效。
- **关注度分档阈值重标定**：新量纲下 `RADAR_HOT_ATTENTION` / `RADAR_WARM_ATTENTION` 用真库分布实测重标，不预设数值、不得沿用旧 3.3 / 0.7，也不得沿用更早的 50 / 15。

## Capabilities

### New Capabilities

- `lane-heat-metric`：事件线退役后的当天热度量纲（泳道热度、版块关注度、分档标定、候选权重在线调、内容身份去重口径的唯一出处）。

### Modified Capabilities

- `workshop-board-console`：矩阵日格 / 演进节点 / 顶部时间视窗曲线改回当日线索合计；颜色通道退役；行序次键改全窗线索合计。
- `workshop-radar`：雷达行构成与关注度分档改新口径；小矩阵改线索数；右栏「今日该读 + 升温提示」整档改为「当天线索最多的泳道」清单；总览端点的事件线清单段退场。
- `board-lane-dynamics`：逐日负载从 `story_count` 改为当天线索数；泳道热度换新口径并补 candidate 泳道当天线索数；行序口径同步。
- `daily-report-system`：`story_id` 退为只读历史列；跨期重复降级、事件线热度先筛重点候选、昨日重点延续徽标三条 Requirement 退役；「今日重点」候选源改为当天同事件聚簇。
- `related-events`：新增「日报同事件聚簇」Requirement，复用 related-event-v1 表示与 0.86 阈值，内联现算、只读不落库。
- `event-story`：身份归一、事件线数据模型、事件归属判定顺序、综合稿不建立事件身份、人工纠正优先、历史回填与降级六条 Requirement 退役（指纹产出那条不动）。
- `story-heat-evidence`：六条 Requirement 全部退役，能力整体退役（`.openspec.yaml` 声明 `retire_capabilities: true`）。
- `entity-subject-resolution`：「影子接入与既有判定零改动」一条改写措辞——该能力（2026-11-11 归档的 `normalize-event-subject-entities`）原文以「事件线身份判定仍只使用 `subject_key` 三层匹配」描述零改动边界，事件线退役后该引用悬空；本 change 只改措辞与边界，实体层行为与写入面一字不动。
## Impact

- **后端写入端**：`backend-go/internal/topicgraph/service/daily_report_story_highlights.go`（在线归属判定入口与其调用整体摘除；`applyStoryDedupForReport` 依赖 `story_id`，随之自然 no-op）、`repository/daily_report_repository.go:SaveReport`（重建日报时的 `affectedStoryIDs` 派生计数重算路径一并摘除，review R-2）。读端死代码随字段退场删除（`board_radar_stories.go` 与 2 个 story 身份集成测试），写端实现与表全部保留（review R-4）。
- **后端读端**：`repository/board_radar_repository.go:GetBoardRadarOverview` / `buildRadarRow` / `finalizeRadarStories`（事件线清单段与 heat 字段）、`repository/lane_snapshot_repository.go:GetBoardLaneDynamics`（lane_heat 与逐日字段）、`repository/story_heat.go:ComputeBoardHeat`（旧半衰期热度）、`repository/report_story_highlights.go:ReportStoryCandidates`（今日重点候选源）。
- **新增**：日报尾段内联聚簇（embedding 调用 + 内存聚簇 + 审计）；新热度纯函数（泳道当天线索数聚合，含 candidate 泳道单独查询）；`ai_settings` 新键 `lane_heat_candidate_weight`（story 停写不再有配置键）。
- **前端**：`front/app/features/tags/components/console/`（BoardRadarStory / BoardRadarReadingRail / ObservationMatrixLaneRow / observationLanes / boardRadarHelpers / BoardLaneEvolution）、`front/app/api/normalizers/boardRadar.ts` 与 `laneDynamicsWindow.ts`、`front/app/api/dailyReports.ts`、`front/app/features/tags/components/daily-report/`（DailyReportMasthead / dailyReportMagazine）、`front/app/features/tags/composables/useBoardRadarOverview.ts`。
- **CLI**：`backend-go/cmd/backfill-stories/main.go` 写模式退役、只读 / 实验模式保留。
- **数据**：无新增表、无 DDL 迁移、无数据回填；`board_stories` / `story_signals` 只读保留。`ai_settings` 新键走既有 upsert 模式。
- **AI 调用**：日报每日每版块新增 1 批 embedding 调用（operation 非空，落 `ai_call_logs`）；`ai_call_logs` 7 天清理窗口不变。
- **调度**：story 无独立 scheduler job（全部挂在日报链尾部），本 change 不新增 / 不停用调度任务。
- **文档**：`docs/reference/flow/semantic-board.md`（红线 28/29/30）、`docs/reference/flow/daily-report.md`（红线 42–49）、`docs/reference/flow/topic-graph.md`（红线 15/16 提及事件线身份处）、`docs/reference/database/tables/daily-report-watch.md`（事件线两表标注只读保留）、`docs/reference/api/`、`docs/reference/configuration.md`。

## 部署后影响与需要的操作

- **本轮只产出规划制品，产品行为不变**：不实现、不部署、不写数据库。
- 将来实现后：矩阵日格与雷达关注度改按当天线索数；雷达右栏换成泳道清单；日报「今日重点」候选改自当天同事件聚簇；日报生成每日每版块多 1 批 embedding 调用。
- 旧 story 数据（`board_stories` / `story_signals` / 历史 `story_id`）保留且只读，用户不需要清理标签、重生成日报或执行任何回填。
- 需要用户执行的只有一项（若希望候选泳道热度权重不同于默认）：在既有「匹配规则」配置里调 `lane_heat_candidate_weight`；默认 0.5 无需操作。
- **story 停写是硬停写，没有开关要关**（review R-1）：上线即生效，不需要改 `ai_settings`、不需要跑任何回填或清理。
- **回滚路径 = `git revert`**（review R-1）：原设计的「把 `story_attribution_enabled` 改回 true」已随开关删除失效；回滚即 revert 本 change 的提交，代码级恢复在线归属入口后重新部署即可，旧数据全程未改、不需要数据修复。
- 关注度分档阈值需在实现阶段用真库分布实测重标（重标定任务见 tasks.md），重标前不得对外宣称新量纲的热点 / 关注分档已定型。
