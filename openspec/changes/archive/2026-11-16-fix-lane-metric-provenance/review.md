# Review 台账 · fix-lane-metric-provenance

审查方式：派子线程聚焦 review（审查制品 + diff）。首轮派发的全量 review 子线程 30 分钟超时无产出（留痕：run `5dafec45`），改派窄口径 review 子线程（只审 5 个高风险点、最多 8 条意见，`60caea01`）取得 8 条意见，逐条处置如下。实现线程另做一轮自查（口径唯一性 / 诚实降级 / 落点可解析）。

## 意见与处置

### R-1 2026-11-16【Medium】「桶回退」口径的下界被当完整值展示
- **状态**：已修复
- **定位**：`front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics` + `front/app/features/tags/components/console/observationLanes.ts:contentLabel`/`matrixCellView` + `laneChronicleMetrics.ts:formatDayTotal`
- **问题**：旧响应回退各桶合计且部分桶不可数时，`clueCount = 已知部分之和` 只是**下界**，但格值 `text`、`aria`、「N 条线索」都直接当完整值写；而编年计数（`formatDayTotal`）同一情形已写 `≥N` —— 两处口径不一致。
- **处置**：`DayCellMetrics` 增 `isLowerBound`（= `clueCountSource==='buckets' && hasUnknown`）；`matrixCellView.text` 写下界 `≥N`、`contentLabel` 写 `≥N 条线索`、`formatDayTotal` 用同一字段判下界。新增用例 `MX-9`（`observationLanes.test.ts`）+ 正常化色 test（`partial.isLowerBound === true`）。

### R-2 2026-11-16【Medium】结算素材与量级信号未按 completed 过滤
- **状态**：已修复
- **定位**：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:ListLaneSnapshotMaterial` / `ListDailyThreadCounts`
- **问题**：锚点已改 completed 口径，但这两条查询仍无 status 条件 → 同日重跑中的 `generating` 报告分区会进结算 prompt 素材与量级计数（未发布内容 + 重复计数），破坏 D10「窗口三处同口径」。
- **处置**：两处查询补 `r.status = ReportStatusCompleted`；`ListLaneSnapshotMaterial` 加注释说明理由。回归：`go test ./internal/topicgraph/repository -run 'TestLaneSnapshot|TestListDailyThreadCounts'` → PASS。

### R-3 2026-11-16【Medium】显式 from/to 窗口下「窗口 N + 更早数 ≠ 全部 M」
- **状态**：已修复
- **定位**：`backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:getComposeEarlyMembers`
- **问题**：向量判据本身对称，但集合切分不对称——窗口取 `[cutoff, upper]`、窗口外只取 `period_date < cutoff`；显式 `from`/`to` 且 `to < 最新 completed 日` 时，`to` 与最新日之间的成员**两侧都不出现**，D9 的「N + 窗口外 = M」不成立。
- **处置**：`getComposeEarlyMembers` 增加 `upper` 参数，取 `(period_date < cutoff OR period_date > upper)` —— 窗口外 = **窗口之外的全体成员**（与 delta spec「窗口外成员集合」措辞一致）。新增用例 `TestComposeCandidatesExplicitWindowKeepsMemberTotal`（显式窗口 + 上界之后成员 → 窗口 1 + 窗口外 2 = 全部 3）。`docs/reference/api/daily-reports.md` 同步说明。

### R-4 2026-11-16【Low】「各桶 N / M 条」与桶数对不上
- **状态**：已修复
- **定位**：`front/app/features/tags/components/console/observationLanes.ts:contentLabel`
- **问题**：`bucketClues` 跳过未知桶，长度可能小于 `sectionCount`，文案出现「3 个主题桶（各桶 2 / 1 条）」数目不符。
- **处置**：有未知桶（`hasUnknown`）时不给「各桶 …」明细，只留合计（下界形式）+ 桶数；全可数时才列明细。`MX-9` 断言 `aria` 不含「各桶」。

### R-5 2026-11-16【Low】权威 clue_count 存在时「另有 N 条综合稿未计入」整条消失
- **状态**：已驳回
- **定位**：`front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics`
- **理由**：任一桶 `composite_count` 缺失时 `compositeCount` 置 null 是**有意**的诚实降级——用已知桶之和当差额会把「另有 N 条」写成偏小的数（正是本 change 要消灭的「同一句出现两个口径」反向版本）。后端在同一响应里保证 `thread_count` 与 `composite_count` 成对下发（同一 `listLaneSectionThreads` 扫描），所以「chunk 级缺失」只发生在旧后端整体缺失的情形，那时连桶线索数都不可数、根本没有「另有」可言。若未来需要日级综合稿合计，应由后端下发日级 `composite_count`（另开 change，不在本 change 辖区）。

### R-6 2026-11-16【Low】`Math.max(0, …)` 把口径异常抹成「已知 0」
- **状态**：已修复
- **定位**：`front/app/api/normalizers/laneDynamicsWindow.ts:laneSectionClueCount`
- **问题**：`composite_count > thread_count`（后端统计异常）时返回 0，被当成合法已知零。
- **处置**：差值 `< 0` 返回 `null`（不可数），不抹成 0。新增用例：`threadCount=1, compositeCount=2` → `clueCount=null`、`clueCountSource='none'`。

### R-7 2026-11-16【Low】`topic_match_distance` 去 `omitempty` 未真正区分「0」与「无距离」
- **状态**：已驳回（转挂观察）
- **定位**：`backend-go/internal/topicgraph/repository/daily_report_repository.go:SectionTimelineNode` / `daily_report_models.go:DailyReportSection`
- **核查结论**：review 子线程确认**没有任何消费方**把 NULL 距离误当「距离 0」——`DailyReportTopicSection.vue` 的 `topicAnchorTier` 与 `SectionQualityExplore.vue` 都要求 `distance > 0` 才展示，两种语义在日报阅读面同归「未锚定」。
- **理由**：本 change 的验收锚是「响应 SHALL 含 `topic_match_distance: 0`，客户端 SHALL 渲染接近度 1.00」——泳道工作面的接近度换算（`useLaneCorrection`：`1 − distance`）在字段被省略时得到 null（显示空），去 `omitempty` 后得到 1.00，契约已满足。把它改成可空指针会牵动 `DailyReportSection` 这个 GORM 模型的写入路径与日报徽标的分档语义（0 → 「极紧锚定」），属日报阅读面行为变更，**不在本 change 辖区**；已记入 design Open Questions Q5 留待后续。

### R-8 2026-11-16【Low】锚点改 completed 后，结算快照 `as_of` 可能回退并覆盖
- **状态**：已驳回（转挂独立 change）
- **定位**：`backend-go/internal/topicgraph/service/lane_snapshot.go:settleBoardLaneSnapshots`（经 `GetBoardReportAnchor` + `UpsertLaneSnapshot`）
- **问题**：最新报告处于 generating/failed 时，锚点回退到上一个 completed 日，`UpsertLaneSnapshot` 会以更早的 `as_of` 覆盖已有快照。
- **核查结论**：`GetBoardReportAnchor` 的消费方只有结算（`lane_snapshot.go:231`）与 `GetBoardLaneDynamics`；`listLaneWindowSections` 只被 `GetBoardLaneDynamics` 使用——**没有非本 change 的消费方受影响**。
- **理由**：快照 `as_of` 单调性属**结算纪律**（横跨结算窗口语义、重试语义、失败降级），修它需要独立设计与 PG 集成用例，且「结算失败/回退不阻塞日报」是既有红线；本 change 只收敛锚点口径。已记入 design Risks R3 + Open Questions Q4，留给独立 change 承接。

## 未发现问题

- **点 1（桶线索数公式与三态）**：`thread_count − composite_count`、任一缺失 → 未知、`clue_count` 权威值不被桶明细未知降级——实现正确（下界标注与桶明细问题已在 R-1/R-4 修掉）。
- **点 3（向量判据对称）**：两侧完全对称（`embedding IS NOT NULL` + 解析失败/长度 0 跳过）；残余只在窗口上界，已由 R-3 修复并加用例。
- **点 4（NULL 距离误读）**：未发现任何消费方把 NULL 当「距离 0」（详见 R-7）。
- **点 5（同屏两个口径）**：`contentLabel` 中「各桶 N/M 条」与当日合计同为非综合稿口径、综合稿只以「另有 N 条未计入」出现，未违反 delta spec（下界标注缺失已在 R-1 修掉）。

## 实现线程自查（口径唯一性 / 落点可解析）

- 口径唯一性：全仓 `grep fallbackClueCount` → 0 命中（旧「events+folded 回退」已删除）；`thread_count` 只出现在「桶 thread 总数（含综合稿）」的语义注释与「− composite_count」公式里；矩阵格、tooltip、「当日其他话题」合计、池曲线、编年计数五处都消费 `dayCellMetrics`/`laneSectionClueCount` 同一条派生。
- 落点可解析：design.md 15 条 Decision 的「落点」逐条核对——文件存在、符号可 `grep` 到（新增符号 `laneSectionClueCount` / `DayCellMetrics.isLowerBound` / `clueCountSource` / `laneSectionThreads.Composite` / `LaneCorrectionNode.proximityKind` / `LaneAnchorScope.totalCount` 均在源码内检索命中）。
- 制品一致：delta spec 的 26 个 Scenario 全部映射到实际测试文件（`scenario-trace.sh` 通过）。
