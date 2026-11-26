## Context

事件线（story）是本仓最深的一层耦合：它把「今天有几件事、哪件事几个来源在报」交给模型判定 + 人工修剪，然后把结果同时喂给矩阵、雷达、热度与日报重点四处消费。本 change 把它退役，换成 event 标签与当天线索几何。

退役理由与实测见 proposal.md 的 Why（引 `openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation/explore-findings.md:6-27` / `:51` / `:71`）。本节只记约束本设计落点的现状事实：

- **story 无独立调度任务**：`internal/app/runtime.go` 注册的 19 个 job 里没有 story 相关项，story 全部是日报生成链的同步派生动作（`service/daily_report_watch.go:GenerateAndSaveReport` → `repository/daily_report_repository.go:SaveReport` → `service/daily_report_story_highlights.go:finalizeDailyReportHighlights`）。因此「停写」不需要动 scheduler.Registry（scheduler 红线 1：调度器清单由 Registry 自动发现，不得维护第二份 descriptor 列表）。
- **在线写入入口只有一个**：`service/daily_report_story_highlights.go:boundedReportAttribution` → `service/story_attribute.go:attributeStoriesForReportWithStats` → `repository/story_repository.go:AttachThreadToBoardStory` / `CreateBoardStory` / `MergeBoardStories` / `story_signals_repository.go:RecomputeSignalsForStory`。掐断它即停写。
- **唯一旁路是 CLI**：`backend-go/cmd/backfill-stories/main.go` 的写模式可绕过日报链直接写 `board_stories` / `story_signals`。
- **读端三处**：`GET /api/semantic-boards/overview`（雷达总览，含事件线清单段）、`GET /semantic-boards/:id/lane-dynamics`（矩阵数据底座）、`GET /api/daily-reports/:id`（今日重点 + `story_id`）。
- **量纲现成**：矩阵需要的「当日线索合计」在 `front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics` 里已经同时算出（`total` = 各桶线索数之和，`peak` = 桶峰值，`storyCount` = 事件线数），换口径不需要新 SQL、不需要新表列。
- **热度无落库历史表**：`repository/story_heat.go:ComputeBoardHeat` 每次请求现算，半衰期写在 `story_heat.go:13-19` 的常量里（`HeatRuleVersion = "heat-v1-48h-hl24h"`），新公式同样不落历史表，旧数据也无需迁移。
- **当天向量可得性**：`backend-go/internal/admin/scheduler/job_daily_report.go:dailyReportWindow` 的日报时刻默认 21:00、兜底 23:30；正常路径 `countQueuesDrained` 等打标队列与 embedding 队列清空后才放行，当天 event 标签的 `topic_tag_embeddings` 已就绪；23:30 兜底路径不等队列，向量可能缺。
- **related-events 的表示与阈值可复用**：`backend-go/internal/tagmanagement/service/related/representation.go:BuildRepresentation`（`related-event-v1` = 名称 + 描述 + 当前有效辅助标签）与 `ThresholdKey`（默认 0.86，`ai_settings` 在线可调）是现成的同口径资产；`backend-go/internal/tagmanagement/service/related/experiment.go:Experiment` 是「只读、只调模型、内存算、不写库」的现成先例。

## Goals / Non-Goals

**Goals:** ① story 在线生产停写、旧数据只读保留、不删表不迁移；② 矩阵 / 演进 / 曲线 / 泳道行序统一为 thread 量纲（当日线索合计）；③ 用一个可在线调的当天热度公式替代半衰期热度，并让版块关注度与分档跟着换口径；④ 雷达右栏换成「当天线索最多的泳道」；⑤ 日报「今日重点」候选源换成当天同事件聚簇（方案 D：内联现算向量 + 内存聚簇 + 不落库）；⑥ 全部口径有唯一落点，不在前端或多个包各写一份等价式。

**Non-Goals:** 不删 `board_stories` / `story_signals` 表与列，不删归属判定的实现库与写端实现（只停在线调用）；读端死代码随字段退场一起删除（`board_radar_stories.go` 与 2 个 story 身份集成测试，见 D1）；不做数据回填与迁移；不把候选权重接进话题生命周期判定；不做跨天事件聚类与跨天重点延续；不新建事件身份或合并关系；不改 topic-graph 的话题生命周期、命中计数与转正门槛；不承诺新口径的关注度分档阈值「已定型」（必须实测重标）；不改 related-events 的既有关联查询能力，只新增日报侧的聚簇用法。

## Decisions

### D1. story 硬停写：摘除在线归属判定入口，实现库与表保留（review R-1 修订）

- **结论**：在线归属判定入口 `service/daily_report_story_highlights.go:boundedReportAttribution` 与其在 `finalizeDailyReportHighlights` 里的调用一并删除——日报链**永久**不产生 `board_stories` / `story_signals` / `daily_report_threads.story_id`，不存在开关态。归属判定函数库（`attributeStoriesForReportWithStats` 与 `storyStore` 写方法）完整保留：`cmd/backfill-stories` 仍编译、仍可读，写模式照旧拒绝。`applyStoryDedupForReport`（依赖 `story_id`）保留但永为 no-op。死代码 `SpawnAttributeStoriesForReport`（当前只有测试调用、无生产调用方）保留但不接线。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights`
- **取舍**：否决「保留开关默认关」——制品宣称停写、默认行为却取决于一个可被改回的键，审计与未来 Agent 无法据代码判断稳态（R-1 的矛盾根源）；否决「删表 + 删列」——不可逆且回滚需要数据修复，违反「旧数据只读保留」；否决「保留入口函数只留断言」——成为无调用方的死代码，lint 也会报 unused。
- **读端删除留痕（review R-4）**：「不删代码」承诺的范围是**实现库与写端**；读端死代码（`repository/board_radar_stories.go` 及其单测、2 个 story 身份集成测试）是「事件线清单段退场」的字段退场连带删除，不构成能力回退。读端恢复与前端口径回滚一并随 `git revert` 完成。
- **回滚**：`git revert` 本 change 的提交即恢复在线入口与读端代码，旧数据全程未改、不需要数据修复。

### D2. 决策修订（review R-1，用户拍板 2026-11-11）：删除停写开关，改硬停写

- **原设计**：`ai_settings` 布尔键 `story_attribution_enabled`（默认开启以保持兼容），关闭即停写、改回 true 即恢复，作为灰度与回滚手段。
- **修订后结论**：该键整体删除（`repository/story_attribution_settings.go` 不存在），停写从「可灰度开关」变为**编译期硬停写**；`docs/reference/configuration.md` 不再登记该键。设计评审 R-1 指出的矛盾（delta 与红线断言「新日报恒 NULL」，而默认开关开着会让系统继续写）以删开关消除，而不是在文档里加限定语。
- **落点**：无配置面（删除 `backend-go/internal/topicgraph/repository/story_attribution_settings.go`）。
- **取舍**：否决 review 建议的「文档级修复」（补部署动作 / 给 SHALL 加开关限定 / 默认改 false）——三者都保留了一个「制品说停写、行为可被配置改回」的分叉面；用户拍板直接消除分叉。代价是失去在线灰度：上线即停写、回滚需 `git revert` 重新部署（不是改键），与 D3 的 CLI 拒绝同属代码级约束。

### D3. 回填 CLI 写模式改为直接拒绝，只读 / 实验模式保留

- **结论**：`cmd/backfill-stories` 的 `--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings` 在停写后一律拒绝并打印退役说明（退出码非 0）；`--dry-run` 预览与任何只读 / 实验模式保留。CLI 本身不删除，便于回滚时重新启用。
- **落点**：`backend-go/cmd/backfill-stories/main.go`
- **取舍**：否决「直接删除 CLI」——回滚时无法恢复，且用户可能仍想用只读模式核对存量；否决「只加警告不拒绝」——一条命令就能绕过 D1 的硬停写，等于停写没有约束力。

### D4. 矩阵日格 / 演进节点 / 顶部曲线统一为「当日线索合计」

- **结论**：矩阵日格 = 该泳道该日各主题桶 `thread_count` 之和（与演进节点、顶部时间视窗曲线同一量纲）；后端 `LaneDynamicsTimelineDay` 增加「当天线索合计」字段，前端 `dayCellMetrics` 的 `displayCount` 主源从 `storyCount` 换成 `total`，`degradedFromClue` 降级层退役（不再有「事件线为 0 时按线索合计兜底」这层）。桶峰值 `peak` 只保留在 aria/title 作辅助说明，不作展示值。
- **落点**：`front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics`
- **取舍**：否决「矩阵日格取桶峰值」——用户原话「当天该泳道有多少条线索」字面即合计，且演进带与趋势图本来就是合计，取峰值会让同一屏出现两套量纲；否决「继续用事件线数」——事件线停写后无新数据，只能长期靠存量回填，方向与本 change 相反。

### D5. 矩阵颜色通道整体退役

- **结论**：矩阵日格与雷达小矩阵不再渲染颜色分档（原色阶表达「当日最热事件线的独立参与者数」，事件线退役后该量纲无数据源）。数值 + `title` / `aria` 承载全部信息；矩阵口径说明由「数字 = 事件线数；颜色 = 来源数分档」改为「数字 = 当日线索合计」。`laneLevel` / `laneDegradedLevel` 两个行内分档函数一并退场。
- **落点**：`front/app/features/tags/components/console/observationLanes.ts:matrixCellView`
- **取舍**：否决「另找量纲补颜色（如按当日线索数分档）」——用户已拍板退役，且现行红线要求颜色不得是唯一信息载体；数值 + aria 已满足可读性，取消颜色反而消除了「两种量纲挤在一条色阶上」的既有隐患。否决「保留降级代理色」——代理值的量纲与真实值不同，留着会继续制造误读。

### D6. 新热度是独立纯函数，不改造 `story_heat.go` 的半衰期分支

- **结论**：新写一个泳道当天热度纯函数：泳道热度 = 活跃泳道当天线索数 × 1.0 + 候选泳道当天线索数 × `lane_heat_candidate_weight`；版块关注度 = 各泳道热度之和（保留一位小数）。落在新的 repository 文件（`lane_heat_repository.go`），不并入 `story_heat.go`：口径版本另立（如 `heat-v2-lane-clues-1d`），`story_heat.go` 的 storyHeat 半支随 story 退役退场，laneHeat 半支不再被消费。
- **落点**：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics`
- **取舍**：否决「在 `ComputeBoardHeat` 里改 laneHeat 半支」——那会把「证据衰减」与「当天线索计数」两套语义塞进同一个函数、沿用同一个 `heat-v1-48h-hl24h` 版本号，事后无法区分新旧口径读数；否决「热度落历史表」——旧口径都没有历史表，新口径靠日报几何可任意回算，落表反而引入一致性问题。

### D7. candidate 泳道的当天线索数需要新增一条聚合查询

- **结论**：`GetBoardLaneDynamics` 的泳道集合是 active ∪ watch-linked，candidate 只出现在 `resp.Candidates` 且不带 timeline / thread 数；但 DB 层 candidate 话题确实有锚定 section 与 thread。新增一条按 `persistent_topic_id` 聚合当天 thread 数的查询（覆盖 active + candidate 两类），供新热度按候选权重计入，并在候选栏条目上回传该值。
- **落点**：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics`
- **取舍**：否决「candidate 不参与热度」——用户口径明确含候选权重项；否决「只在前端按候选栏提示文本估」——前端拿不到 thread 数，且违反「口径唯一实现」纪律。

### D8. 候选权重经 `ai_settings` 在线可调，键名 `lane_heat_candidate_weight`

- **结论**：新增 `ai_settings` 键 `lane_heat_candidate_weight`（默认 0.5，有效值 `[0,1]` 有限数），缺项回退默认、坏存量值告警回退、非法提交整体拒绝不覆盖。若要出现在既有「匹配规则」配置界面，走 `GET/PUT /api/semantic-boards/matching-config` 的键白名单与取值校验（默认分支已限 0..1 浮点）。该键 SHALL NOT 参与话题生命周期判定。
- **落点**：`backend-go/internal/tagmanagement/handler/board_crud_handler.go:isSemanticBoardConfigKey`
- **取舍**：否决「把权重写死 0.5」——candidate 的含义是「还没转正」，其热度权重是产品口味而非技术常量，应可在线调；否决「做成两个独立开关（纳入 / 不纳入）」——权重 0 已能表达「不计入」，多一个开关只会多一个不一致的组合。

### D9. 关注度分档阈值必须用真库分布重标定，本期不预设数值

- **结论**：`front/app/features/tags/components/console/boardRadarHelpers.ts` 的 `RADAR_HOT_ATTENTION` / `RADAR_WARM_ATTENTION`（现值 3.3 / 0.7，按旧 lane_heat 分布 P80/P20 标定）在新关注度量纲下失效。新口径落地后用真库分布实测重标（样本、日期、分位点留痕），把结果写回该常量。重标定前不得对外宣称新量纲的热点 / 关注分档已定型。
- **落点**：`front/app/features/tags/components/console/boardRadarHelpers.ts:RADAR_HOT_ATTENTION`
- **取舍**：否决「沿用 3.3 / 0」——量纲不同，旧值无意义；也否决「沿用更早的 section 口径 50 / 15」——现行红线逐字禁止。否决「直接拍一个数」——分档是用户可见的产品决策，必须由真实分布决定。

**实测定标记录（2026-11-11，apply 阶段 4.1 执行）**

- **量纲**：版块关注度 = Σ 泳道当天热度（active 泳道线索数 × 1.0 + candidate 泳道线索数 × `lane_heat_candidate_weight`；权重缺项按默认 0.5），后端保留一位小数（`heat-v2-lane-clues-1d`）。
- **样本**：真库（Docker `syntopica-postgres`，只读 SELECT）近 14 个报告日内**全部版块 × 日**的关注度正值——154 个样本 / 13 个版块（同日无分区的版块不产样本；零值不计入，与旧标定「只取正值」同口径）。
- **采样日期**：2026-11-11（库内最新报告日 2026-11-10）。
- **分位点（一位小数）**：**P20 = 2.3**、P50 = 8.5、**P80 = 14.5**、P90 = 17.9，min 0.5 / max 24.5，均值 8.79。
- **稳定性复算**：同一 SQL 把窗口放宽到近 30 日（321 个样本 / 14 个版块）→ P20 = 2.0、P80 = 14.0（口径稳定，分档不因窗口选择翻转）。
- **写入值**：`RADAR_HOT_ATTENTION = 14.5`、`RADAR_WARM_ATTENTION = 2.3`（`front/app/features/tags/components/console/boardRadarHelpers.ts`）。
- **旧值处置**：3.3 / 0.7（lane_heat 量纲）与 50 / 15（分区量纲）一律不得沿用；4.2 已落可执行断言（常量不等于这四个值，且分档边界按新值判定）。
- **当日影响面（如实记录）**：以 2026-11-10 的 14 个版块为例，按新阈值 0 个达到热点、11 个进入「关注」档——这是「当天没有任何版块落在近 14 日分布的前 20%」的真实结论，不是漏算。

### D10. 雷达右栏换成「当天线索最多的泳道」清单

- **结论**：右栏条目单位从事件线改为泳道：跨版块按该泳道当天线索数降序混排，每条带版块名 + 泳道名 + 当天线索数，上限 8。「今天可能升温」「今日新起」「升温 N×」行内徽标与「跨天延续事件线建设中」脚注整档退场（判据全部依赖事件线热度环比）。条目不再提供 `?story=` 深链。
- **落点**：`front/app/features/tags/components/console/BoardRadarReadingRail.vue`
- **取舍**：否决「把事件线清单换成「按线索数的事件线」」——事件线本身停写，存量条目会一天天变旧；否决「留一个空的右栏」——右栏是首屏信息密度的主体，留空会掏空首屏；泳道清单是同一版块内真实存在的、每天都在产生的数据。

### D11. 日报同事件聚簇走方案 D：内联现算 `related-event-v1` 向量 + 内存聚簇

- **结论**：日报生成时，对该版块当天命中的 event 标签用 `related.BuildRepresentation` 构造 `related-event-v1` 文本，批量调 airouter embedding（分批上限与单批超时沿用 `related/refresh.go` 的既有口径：32 / 30s），在内存里两两算余弦并按 `related_event_similarity_threshold`（默认 0.86）聚簇。结果只用于当天的「今日重点」候选，**不落库、不写 `related_event_embeddings`、不做身份、不跨天**。表示与阈值与 related-events 同源，不另立。
- **落点**：`backend-go/internal/tagmanagement/service/related/representation.go:BuildRepresentation`
- **取舍**：否决「方案 A：复用 `related_event_embeddings` 索引」——该索引只由显式刷新写入，当天新标签在未刷新前没有向量（覆盖率≈0），且「把索引刷新挪进日报链」与「不落库」直接冲突；否决「方案 B/C：直接读 `topic_tag_embeddings.semantic` / `identity`」——表示不同，0.86 阈值无依据必须重标，且 semantic 表示含 aliases/category，偏向「标签身份」而非「事件」；否决「方案 G：用 section/thread 向量聚簇」——单位从 event 标签退化成叙事单元，偏离用户口径；否决「方案 F：用上一日快照」——与「当天热度」语义直接冲突。方案 D 是唯一同时满足「当天新鲜 / 阈值有依据 / 不落库 / 不做身份」的选项，且有 `related/experiment.go:Experiment` 的现成只读先例可照抄。

### D12. 聚簇降级链 D → 语义向量兜底 → 主体近似兜底，逐级标注口径

- **结论**：embedding 调用失败 / 超时 / 无可用路由时，降级到读 `topic_tag_embeddings` 当天已有的语义向量；仍不可得时降级到主体近似（按事件指纹的 subject/action/object 机械分组）。每一级都在日报重点依据处如实标注本次口径（向量聚簇 / 语义向量兜底 / 主体近似），不同口径的结果 SHALL NOT 混为一谈。主体近似只作最后兜底：`subject_key` 已知漂移（138 组），质量不可靠。
- **落点**：`backend-go/internal/tagmanagement/service/related/experiment.go:Experiment`
- **取舍**：否决「向量不可得就跳过聚簇」——日报「今日重点」会长期空降级，等于砍掉一半价值；否决「静默降级」——用户看到的必须是真实口径，不能把主体近似呈现为向量聚簇。

### D13. 今日重点仍调 LLM，候选源换聚簇，prompt 与白名单重写

- **结论**：`repository/report_story_highlights.go:ReportStoryCandidates`（现按 `story_id` 取候选）与 `service/daily_report_story_highlights.go:GenerateStoryHighlights`（现「从最多 8 条候选事件线中选最多 3 条」）改为从当天聚簇取候选：候选 = 聚簇 Top 8（按簇内文章数降序 → 簇内 event 标签数降序 → 代表标签 id 升序），每簇附代表标签、description、簇内文章数（内容身份去重）、聚簇口径标记。LLM 仍只从这 8 条里选至多 3 条，模型返回的 `cluster_id` 必须在候选白名单内、`tag_ids` 必须在该聚簇的本期标签白名单内，非法按既有语义同参重试一次、合法 `[]` 不重试。簇内文章数、聚簇口径、数据截至日、研究时点由服务端补快照，模型不输出数字。
- **落点**：`backend-go/internal/topicgraph/repository/report_story_highlights.go:ReportStoryCandidates`
- **取舍**：否决「不用 LLM，直接按簇内文章数取前 3」——重点的「为什么值得看」是语义判断，纯排序会退化成条数榜；否决「保留事件线候选作为回退源」——事件线停写后只剩存量，回退源会长期给出过期条目；改为「零候选时走既有标签聚类回退」，与既有降级链一致。

### D14. 定位与深链从 `story_id` 改 `cluster_id`，旧 `?story=` 解析后忽略

- **结论**：刊头重点的定位目标改为该聚簇在本期正文中 DOM 顺序的第一条 thread（锚点仍是 `report-thread-{thread.id}`）；跨页寻址参数由 `?story=` 改为 `?cluster=`；旧 `?story=` 链接继续被解析但被忽略并提示该定位方式已下线，不报错、不定位到任何内容。
- **落点**：`front/app/features/tags/components/daily-report/DailyReportMasthead.vue:onLocate`
- **取舍**：否决「取消定位能力」——刊头重点「定位到正文」是既有主故事的一部分，直接砍掉是能力遗漏；否决「保留 `?story=`」——事件线身份不再对外透出，保留即继续透出已退役概念。

### D15. `scope` 保留、综合稿不进聚簇；事件线能力整体退役但保留指纹产出

- **结论**：`daily_report_threads.scope`（由 `tag_ids` 中 event 标签数推导）保留并继续生效；`scope=composite` 的 thread SHALL NOT 进入任何聚簇、SHALL NOT 计入簇内文章数（综合稿是多事件混合，算进任一簇都会虚高）。能力边界上：`story-heat-evidence` 六条 Requirement 全部退役、该能力整体退役（`.openspec.yaml` 声明 `retire_capabilities: true`）；`event-story` 退役身份相关的六条，但保留「事件指纹结构化产出」——指纹仍由打标链产出、仍被实体层影子模式读（`tagmanagement/service/entity/hook.go`），删掉会让该契约失去 spec 锚。
- **落点**：`backend-go/internal/topicgraph/repository/daily_report_models.go`
- **取舍**：否决「把 `event-story` 能力整体删除」——指纹产出契约仍有活消费方（实体层），整体删除会让它变成无锚的隐式契约；否决「`scope` 一起退役」——综合稿判定独立于 story，且新聚簇需要它来排除多事件混合稿。

### D16. 新增 embedding 调用必须带 operation 并留审计

- **结论**：日报聚簇的 embedding 调用走标准 airouter 路由，capability 为 `embedding`，operation 另立（`topicgraph.daily_report.event_cluster.embedding`，与索引刷新的 `tagmanagement.related_event.embedding` 区分，便于按操作审计与统计），非空；每次调用尝试无论成败都写 `ai_call_logs`，token/latency 成败均记。**embedding 路径按仓内 N/A 惯例不记 prompt / response_snippet**（airouter 的 embedding `LogCall` 只带 operation/capability/token/latency/metadata，prompt/response_snippet 截断（10000 / 20000 runes）只适用于 chat 路径，review R-7），`Metadata` 带 `board_id` / `report_id` / `batch_size` / `representation_version`。该 operation SHALL NOT 进 embedding 持久化缓存白名单，避免把日报的当天向量当成可复用的通用缓存命中。
- **落点**：`backend-go/internal/tagmanagement/service/related/refresh.go:RefreshBoard`
- **取舍**：否决「复用索引刷新的 operation 名」——两者频次与语义不同（一次性 vs 每日每板），混在一起会让审计统计失真、也让缓存白名单决策混淆；否决「不进审计」——ai-summary 红线要求每次调用尝试都留痕。

## Risks / Trade-offs

- **聚簇规模与 O(n²)**：当天某版块的 event 标签数可达数百，内存两两比较在 n≈500 时约 12.5 万对、n≈2000 时约 200 万对，均可在毫秒级完成，**计算不是瓶颈**；真正的瓶颈是 embedding 的**分批调用与总时长**（每批 32、单批 30s 超时）。→ 缓解：聚簇设总预算上限，超限时按「取当天命中数最多的前 N 个标签」截断并标注截断，且截断只影响重点候选、不影响日报主体；分批失败按 D12 降级。
- **日报尾段延迟**：聚簇排在日报生成尾部，可能把 21:00 的日报推后。→ 缓解：聚簇失败只降级重点、不阻断日报主体（沿用既有「降级不阻断」语义）；单批超时 30s 与总预算上限双重约束。
- **embedding 失败降级链的质量断层**：D（向量）→ 语义向量兜底 → 主体近似三级的质量是递减的，主体近似一层因 `subject_key` 漂移（138 组）而不可靠。→ 缓解：口径逐级标注、不混用；界面明示「本次按主体近似，非向量聚簇」。
- **23:30 兜底路径向量可能缺**：兜底强制生成不等 embedding 队列，此时当天部分 event 标签的 `topic_tag_embeddings` 尚未算出，语义向量兜底层可能不完整。→ 缓解：D 方案自算向量不依赖队列状态；兜底层缺向量时进一步落到主体近似并标注；日报主体不受影响。
- **每日新增模型调用量**：每版块每日一次批量 embedding 调用（按 32/批分批）。→ 缓解：分批上限与单批超时沿用既有口径；operation 独立可单独统计；审计 7 天清理窗口不变，超量可先在少量版块灰度。
- **分档阈值未重标定**：新关注度量纲的分档阈值在实现完成前是未知的。→ 缓解：重标定作为独立任务（tasks 4.1），标定依据（样本、日期、分位点）必须留痕；未重标定前不得对外宣称分档已定型。
- **旧数据与新口径的断层**：历史 `board_stories` / `story_signals` 只读保留，但不再有任何读端消费它们——旧事件线数据会逐步失去展示入口。→ 缓解：这是「退役」的预期结果（用户明示「界面不再透出事件线概念」）；不删数据，回滚可恢复。
- **话题生命周期不受影响是硬边界**：候选权重只进热度，SHALL NOT 进命中计数 / 转正门槛。→ 缓解：实现时把权重读取点限制在热度计算函数内，不复用话题状态判定路径；测试用例单列一条断言候选权重变化不影响话题状态。
- **能力退役的归档副作用**：`story-heat-evidence` 整体退役会在归档时删除其主 spec（可经 git 恢复）；`event-story` 残留「事件指纹结构化产出」一条，其 Purpose 文本已与残留内容不完全相符。→ 缓解：归档时同步改写 `openspec/specs/event-story/spec.md` 的 Purpose（该文件只能在归档时直改，本 change 不改主 spec）。

## Migration / Rollback

- **无 DDL 迁移、无数据回填**：新热度从既有日报几何算，聚簇在内存算，`ai_settings` 新键走既有 upsert。
- **上线顺序**：① 后端上线即硬停写（无开关、无灰度态，review R-1）+ 新热度 / 新聚簇；② 前端切换口径（旧读端随之失去消费方）；③ 实测重标分档阈值（实现阶段完成，见 D9）；④ CLI 写模式一并拒绝。
- **回滚**：`git revert` 本 change 的提交后重新部署——在线入口与读端代码一并恢复；旧数据全程未改，回滚**不需要数据修复**。已无 `story_attribution_enabled` 可切（review R-1：开关删除，回滚是代码级而非配置级）。
- **灰度**：新口径可按版块观察数据，但 story 停写**不可灰度**：没有开关，上线即全量停写——这是用户拍板的取舍（R-1）。

## Open Questions

- **关注度分档阈值的具体数值**：待实现后按真库分布实测确定（tasks 4.1），本期不预设。该问题不影响 specs、方案与任务拆解，只影响常量取值。
- **`event-story` 主 spec 的最终归属**：残留「事件指纹结构化产出」一条后，能力目录名与内容不完全相符；是改名（迁到 `tagging-domain` 一类更贴切的能力）还是保留原名，属归档期的文档整理决策，不影响本 change 的实现。
- **日报聚簇的向量缓存策略**：本期明确不落库；若后续实测显示每日重算成本不可接受，再单独立项评估「按天缓存向量」——该评估需要另开 change，不在本期范围。
