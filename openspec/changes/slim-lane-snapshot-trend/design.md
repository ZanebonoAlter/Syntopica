# Design: slim-lane-snapshot-trend

## Context

现状链路：`GenerateAndSaveReport` 成功后 `spawnLaneSnapshotSettlement` 逐 active 泳道调 `settleLaneSnapshot`（`lane_snapshot.go`）——素材 `ListLaneSnapshotMaterial`（section 级 rows：日期｜节标题｜前 3 条 thread 标题）→ 单次 LLM 调用（`daily_report.lane_snapshot`，JSON 双字段 200/1000 字，MaxTokens 1536）→ `parseLaneSnapshotOutput`（JSON 解析 + 坏 JSON 降级）→ upsert `topic_lane_snapshots`。前端 `LaneTrendOverview.vue` 14 天档 detail 优先、缺失回退短版。动机见 proposal.md - Why。

## Goals / Non-Goals

**Goals**：产物收敛单版演进叙事；量级信号系统计算注入；解析链简化；前端删降级链；存量数据自然自愈。

**Non-Goals**：月/年档（enrichment contexts）不动；`rolling_detail` 列不删（无 migration）；板块内容 tab 泳道卡片不动（仍读 `rolling_summary` 展示一句话态势）；逐日事件展开不动；结算时机/松耦合框架（60s timeout、失败全吞、detached goroutine）不动。

## Decisions

### D1. 量级信号由 repository 新增按日计数查询供给

- **结论**: 新增 `ListDailyThreadCounts(topicID, sinceDate)`（按 `period_date` 聚合该泳道当日锚定 section 的 thread 数，含当日 section 数），service 在素材构造时拼装「每日线索量 + 近 7 天 / 上个 7 天合计」文本段；无报告日不返回行，service 端在量级段明示「无报告」（缺日不计入趋势判断）。7 天窗口锚定 as_of（`MAX(period_date)`），与素材窗口口径一致（非 now()）。
- **落点**: `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:ListDailyThreadCounts`、`backend-go/internal/topicgraph/service/lane_snapshot.go:buildLaneSnapshotUserPrompt`
- **取舍**: 备选「扩展现有素材 SQL 全量带回 ThreadTitles 再计数」被拒——素材行被截断的前 3 条是渲染约定，混入计数职责会让一次查询承载两种语义且传输放大；备选「LLM 自行从清单感知量级」被拒——数数不可靠（讨论期用户已确认要系统算好喂现成）。

### D2. system prompt 重写为演进叙事定位（纯文本输出）

- **结论**: prompt 骨架：「你是新闻态势总结助手。基于一条话题泳道近 14 天的事实清单与量级信号，写一段 150-200 字的中文态势总结，按演进叙事组织：划分阶段（带大致日期锚）→ 每阶段驱动与转折（事件仅作转折标记点出，不展开细节）→ 当前落点与局势判断 → 用量级信号佐证走向。禁止逐日罗列事件；禁止预测未来走向（只描述已发生的变化）；禁止使用清单与量级信号之外的数字；直接输出这一段纯文本，不要 JSON、前后缀或代码块围栏。」
- **落点**: `backend-go/internal/topicgraph/service/lane_snapshot.go:laneSnapshotSystemPrompt`
- **取舍**: 备选「保留 JSON 单字段输出」被拒——单版后 JSON 包装零收益，反而保留围栏/引号解析失败面；纯文本 + trim + 截断 + 空守卫是最小解析面。

### D3. 解析路径收敛为纯文本三步守卫

- **结论**: `parseLaneSnapshotOutput` 重写：trim → 空/纯空白则 error（该泳道结算跳过）→ `truncateRunes(200)` 入库；`rolling_detail` 固定写空串。删除 JSON 解析分支、`degraded` 概念与「坏 JSON 截 200 当短版」降级。MaxTokens 1536 → 500（200 中文字 + 呼吸空间）。
- **落点**: `backend-go/internal/topicgraph/service/lane_snapshot.go:parseLaneSnapshotOutput`
- **取舍**: 无备选：spec 已定纯文本；500 取值 = 200 字中文 ≈ 400 token 上浮 25%。

### D4. 前端 14 天档收敛单内容源

- **结论**: `summaryText` 直接取 `summary`（删 `detail || summary`）；删 `detailMissing` 计算与「长版随下次日报结算生成」提示分支；`data-testid` 统一 `trend-summary`。快照缺失占位、月/年档、逐日事件、tab 逻辑不动。
- **落点**: `front/app/features/tags/components/daily-report/LaneTrendOverview.vue`
- **取舍**: 无备选：ui-design.md minor 契约已锁定复用现有布局。

### D5. LLM capability 维持 DigestPolish

- **结论**: 结算调用继续走 `airouter.CapabilityDigestPolish`、温度 0.3（未动）、operation `daily_report.lane_snapshot` 不变，仅 MaxTokens 与 prompt 变。
- **落点**: `backend-go/internal/topicgraph/service/lane_snapshot.go:settleLaneSnapshot`
- **取舍**: 无备选：任务性质（事实约束下的摘要改写）未变。

## Risks / Trade-offs

- [LLM 把演进叙事写成压缩版逐日罗列（「10日…15日…24日」式速记）] → prompt 显式禁止逐日罗列 + 讨论期判据沉淀（阶段必须带驱动/转折语义）；归档前人工抽查当日结算产物核对（效果核对，见 test-cases）。
- [量级信号把「无报告日=0」误读为降温] → prompt 注明「无报告日不计入趋势判断」；素材段标注口径。
- [存量快照在前端仍渲染旧长版] → D4 直接删 detail 消费路径，前端编译期即不再读该字段；后端读路径（板块内容 tab）本就不读 detail。
- [单版 200 字对多阶段泳道可能截断语义] → 与旧 1000 字相比信息密度提升是有意取舍；截断只发生在模型超写时（机械保护），prompt 已要求 150-200 字。

## Migration Plan

部署即生效（无 schema 变更、无开关）：下个日报日起各泳道结算产出单版态势句并置空 `rolling_detail`；存量长版在其泳道下个日报日前仍在库中但前端不再渲染。回滚 = revert 代码（旧结算逻辑恢复后下个日报日覆盖回长短两版）。

## Open Questions

（无）
