# Design：描述修复任务批量合并匹配

## Context

现状执行模型（见 explore-findings C 节）：executor 单槽串行，`runJob` 在一个 attempt 内「claim → embedding → saving → matching」一口气跑完；matching 是 per-job 的全量 scope 逐条过引擎（`ExecuteMatchingPhase` → `MatchTopicTag` 逐 tag）。同批确认 N 项 = N 个 job 各跑一遍全量（≈N×1.99 万条引擎调用），且留下 N 个时间点不一致的匹配结果。

决定性事实（explore-findings D 节）：`MatchTopicTag(tagID)` 计算的是「该 tag ↔ **全部版块**」的全局匹配，与修哪个标签无关；合并后引擎调用 N×全量 → 1×全量，且「批内描述全部提交后一次性评估」产出**一致快照**，语义比现状更正确。

约束：重启恢复要求批可丢可重建（批是执行期概念，不持久化）；红线 26「每 job 四类计数之和恒等于 total」与「任务持久幂等」语义全部保持；实现契约层面「计数是 items 的投影」在 auxrepair repository 注释与 implementation-contract（triage change）中成文。

## Goals / Non-Goals

**Goals**

- 同版块（实践中=同轮体检确认）的多个未结束 job：先全部完成 embedding/saving，再合并跑**一遍**全量 matching，按 tag 去重、结果广播到批内各 job；
- 进度可见：JobView 追加 `queued_jobs`，前端 matching 阶段文案显示排队数（ui-design.md 已定契约）；
- 计数结算从每条一聚合（O(N²)）改为每 K 条批量结算（O(N²/K)→投影语义零变更）；
- 崩溃/重启后零人工干预恢复，兼容回滚到旧代码。

**Non-Goals**（proposal Out of Scope 之外的设计级边界）

- 不引入持久化批实体（无 batch_id 列、无批表）——批由执行期动态发现与重建；
- 不改匹配引擎、冻结 scope 创建语义、per-label 幂等/互斥、claim/lease/heartbeat 的数据库时钟纪律；
- 不做多 executor 实例并发攒批（生产仍是单实例单槽；SKIP LOCKED 机制天然防重复执行，但本设计不优化多实例吞吐）。

## Decisions

### D1. 攒批机制：执行期两段式调度（先排干提交、再合并匹配），无持久化批
- **结论**: `runJob` 拆两段。第一段：claim 到未提交 job 只跑 embedding+saving，跑完把 job 留在 matching 态（不再就地进行 matching），立即继续 claim 下一个 pending job 重复此段。第二段：当无 pending 可领（或攒满 `BatchMaxJobs`）时，一个事务内 `FOR UPDATE SKIP LOCKED` 领取**全部** matching 态可领 job（lease 过期含在内）成批，统一 heartbeat、合并 matching、逐 job FinishMatchingPhase。批在任何时刻丢失（崩溃/重启/lease 过期），各 job 停留在自身持久状态，下一轮调度重新发现与重建——恢复语义零特殊分支。
- **落点**: `backend-go/internal/tagmanagement/service/auxrepair/executor.go:runJob` + `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_executor.go:ClaimNextRepairJob`
- **取舍**: 否决「创建时分组（batch_id 列）」——引入持久化实体就要处理批生命周期迁移、批与 lease 的所有权交叉、旧数据回填，而批的正确性根本不依赖身份同一性，只依赖「description_committed 早于 matching 开始」这一时序；执行期动态集合零迁移、零回滚负担。也否决「就地把后提交 job 的 matching 延迟到前序 job 完成后」的排队模型——那仍会多跑引擎，只省尾部。

### D2. 合并匹配：批内按 topic_tag_id 去重，单次引擎调用结果广播到批内各 job 的 item 行
- **结论**: 匹配阶段装载批内全部 job 的 pending/failed items，按 topic_tag_id 去重成执行集；每 tag 恰好一次 `GetScopeTagState` + 一次 `MatchTopicTag`，成功/失败/not_applicable 按 tag 判定一次，结果写入批内**每个** job 的对应 item 行（引擎对全部版块用当前数据评估，天然包含批内全部已提交描述——这是广播合法性的根基：对任一批内 job，该结果都是「含自己新向量的匹配」）。not_applicable 判定本身是 tag 级的，与 job 无关，去重模型下天然自洽。
- **落点**: `backend-go/internal/tagmanagement/service/auxrepair/matching.go:ExecuteMatchingPhase` + `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go:ListRepairScopeItems`
- **取舍**: 否决「逐 job 顺序匹配但跳过已覆盖条目」——「覆盖」判定需要引入匹配版本时间戳（新列+新契约），且批内 job 提交时刻交叉时仍会重复跑；去重广播模型一次评估、一致快照，是 D 节证明的语义更优解。

### D3. 计数结算批量化：每 K 条一个事务（K 条 item 更新 + 一次聚合 + 一次 job 计数更新），投影语义零变更
- **结论**: `RecordMatchingItemResult` 从「每条一事务一全量聚合」改为「每 K 条（`BatchSettleSize` 默认 50）一个事务：批量 UPDATE K 条 item + `aggregateRepairCounters` 一次 + 批内各 job 计数一次更新」。进度粒度从 1 变 50——3 秒轮询下全量跑几十分钟，50 条粒度用户无感；收尾 `FinishMatchingPhase` 的最终聚合对账不变，四类计数之和恒等 total 的不变量与「计数是 items 投影」的实现契约**逐字保持**。
- **落点**: `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go:RecordMatchingItemResult`
- **取舍**: 否决「DB 增量维护（processed = processed + 1）」——它把「投影」契约改成「增量账本」，崩溃窗口内 job 行与 items 可能漂移，需要收尾对账兜底却失去了任意时刻可复核性；也否决「完全不更新中间进度、只收尾结算一次」——小时级任务全程进度冻结正是本次要消除的「以为卡死」体验。K=50 在 O(N²/K) 与进度实时性间取平衡。

### D4. queued_jobs：全局未结束任务数（不含当前 job），单 COUNT 查询，组装层拼接
- **结论**: 语义取「全局未结束（pending/embedding/saving/matching）任务数 − 1」，不做「同版块」细分——单用户产品里同时未结束的修复任务几乎必然来自同一轮体检确认，全局计数一行 SQL 即满足「另有 M 项排队」的用户心智；job 表本无 board_id，「同版块」细分需要 aux label → 版块的多跳 join，收益不成比例。查询在 handler/service 组装层做（`getRepairJob` 路径），`JobView` 追加 `queued_jobs` 字段透出。
- **落点**: `backend-go/internal/tagmanagement/repository/auxiliary_description_repair.go:GetLatestRepairJob`（同文件新增 `CountUnfinishedRepairJobs(ctx, excludeJobID)`）+ `backend-go/internal/tagmanagement/service/auxrepair/auxrepair.go:JobView`
- **取舍**: 否决「新增列任务端点（GET /description-reports?status=unfinished）」——前端轮询是单目标单端点，为排队数引入列表端点+分页+过滤是过度设计；也否决「同版块精确计数」——多跳 join 换不来单用户场景的实际差异。

### D5. specs delta 延后：基线等 triage 归档，锚定关系先行声明
- **结论**: 本 change 的 MODIFIED 对象（「修复后更新匹配并独立重试」「修复任务持久且幂等」「更新完成与效果验证分离」）现行文本在未归档的 board-health-triage-advice delta 内；按用户决策，specs 制品等 triage 归档（主 spec 就位）后撰写，design/tasks 先行。specs 动工的前置条件与改写清单已在 explore-findings B 节逐字备妥。
- **落点**: 无代码落点：specs delta（`openspec/changes/description-repair-batch-matching/specs/`），因为基线 `openspec/specs/auxiliary-label-description-repair/spec.md` 依赖 board-health-triage-advice 归档后才存在
- **取舍**: 无备选：openspec validate 拒绝无基线的 MODIFIED，且宪法规定不绕过 spec-gate。

### D6. 批失败隔离：saving 失败/未提交的 job 不进合并匹配，不连坐
- **结论**: 攒批第一段中某 job embedding/saving 失败 → 该 job 按现状标 embedding_failed/save_failed（显式失败等用户重试），**立即从批内剔除**；第二段合并匹配只收 description_committed=true 的 job（谓词天然排除未提交者）。批内某 job 的 matching items 有失败 → 仅该 job 标 matching_failed，同批其它 job 正常 FinishMatchingPhase 为 completed——失败按 job 结算，永不连坐。
- **落点**: `backend-go/internal/tagmanagement/service/auxrepair/executor.go:runJob` + `backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go:FinishMatchingPhase`
- **取舍**: 否决「批内任一失败整批回滚重试」——破坏既有「失败等待明确重试不自动重跑」红线（spec：失败等待明确重试），且把单标签的 AI 生成失败放大成整批不可用。

## Risks / Trade-offs

- [一个 owner 同时持批内多 job 的 lease，heartbeat 复杂度上升] → heartbeat 循环遍历批内 job 逐个续租（批 ≤ BatchMaxJobs，个位数）；任一续租失败即取消整个批 attempt（复用现有「丢租约即停」路径），各 job 回 matching 态等待重新发现——不新增恢复分支。
- [合并 matching 中途 panic / 进程死] → 各 job 留 matching 态、items 部分结算；重启后重新攒批，已 succeeded 的 item 不重跑（`ListRepairScopeItems` 只取 pending/failed），引擎 replace 挂载幂等——零人工干预。
- [攒批中途用户又确认新任务] → 新 job 为 pending，若合并 matching 未开始则自然并入（下轮 claim 先排干 pending 的调度规则保证），已开始则进下一批——无损，仅延后。
- [极端下同版块堆积大量未结束 job（如脚本批量确认）] → `BatchMaxJobs` 上限截断攒批规模，超出部分成为下一批；上限值 Open Question 可调。
- [前端 3s 轮询 × K=50 结算粒度] → 进度条步进 50；总时长几十分钟下无感知差异；K 可配置。
- [与 board-health-triage-advice 的并行窗口] → 本 change 实现阶段必须等 triage 归档（D5）；若两 change 同窗动 auxrepair 代码会互相踩测试基线——tasks 编排显式前置。

## Migration Plan

1. 前置：board-health-triage-advice 归档（主 spec 就位）→ 本 change specs → 实现；
2. 部署：**零 DB 迁移**（无新表/新列/新索引；`queued_jobs` 为运行时查询，计数批量化复用现有行）。重启后端即生效；重启瞬间在跑的 job 由 lease 过期（60s）自然进入新调度——旧代码留下的 matching 态 job 会被新逻辑重新攒批，已结算 items 不重跑；
3. 回滚：回滚代码重启即可。job 状态词表、items 行、计数语义全部未变，旧代码可无缝接管任何中间状态（旧逻辑对 matching 态 job 照旧按自身 scope 续跑）；
4. 用户可见变化：同批确认多项时总匹配耗时约除以 N；matching 阶段进度文案追加「另有 M 项排队」；无数据再生成、无清理操作。

## Open Questions

- `BatchMaxJobs` 与 `BatchSettleSize` 的默认值（拟 8 / 50）：不影响结构，实现时用生产数据规模标定后定稿。
