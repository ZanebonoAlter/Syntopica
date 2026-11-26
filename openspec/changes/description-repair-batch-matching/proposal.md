<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

体检确认描述文案后，每条确认各自创建一个修复任务，每个任务的匹配阶段都把全部 ~1.99 万活跃话题标签逐条重算一遍匹配（冻结全量 scope 的既定设计）。同批确认 N 项 = N 遍全量重算（2026-11-06 实测同批 4 项 ≈ 7.95 万条匹配，树莓派上小时级）。而匹配是「tag ↔ 所有版块」的全局重算，与修哪个标签无关——N 遍里 N-1 遍是纯重复劳动，还留下 N 个时间点不一致的匹配结果。进度 UI 只显示当前任务的「已处理 X/Y」，看不到还有任务在排队（后端无列任务端点，前端无法得知），长时间无反馈被误认为卡死。

## What Changes

- **执行调度合并（攒批）**：executor 发现同版块还有未完成任务时，先把批内任务的 embedding / saving（描述+向量提交）全部完成，再跑**一遍**全量匹配；该次匹配完成时刻之后视为已生效，凡描述提交早于它的批内任务，其条目全部按该次结果标记——一次匹配同时推进多个任务的进度。
- **部分失败边界不变**：合并匹配一遍跑完后，某任务若仍有失败条目则标 matching_failed 等待用户显式重试；重试只重跑该任务自己的失败/未完成条目（重试语义与现状一致）。
- **进度可见性**：GET `description-repairs/:job_id` 响应新增排队信息（同版块未结束任务数），前端既有轮询顺带获得；体检进度文案升级为含排队信息的形态（如「更新版块匹配 X/Y · 另有 M 项排队」），复用既有文案派生结构，不改信息架构。
- **匹配计数聚合批量化**：`RecordMatchingItemResult` 当前每记一条条目就对全量 item 表做一次 COUNT 聚合（每任务 O(N²)，N≈1.99 万）——改为批量记录 / 增量推进；计数仍是持久条目的投影，「四类计数之和恒等于 total」不变量不变。
- **不改**：per-label 幂等键与同标签未结束互斥、体检轮次纯读原则、任务创建时冻结全量 scope 的语义、匹配引擎本身（每条 tag 仍逐条过既有引擎）。

### Out of Scope

- 匹配引擎批量化（一次引擎调用处理多条 tag、向量批量计算）——独立重构，留后续 change；
- 跨重启的「批」持久化：批只是执行期调度概念，每个任务仍独立持久、独立恢复，崩溃后按既有 lease/重试机制续跑；
- board-health-triage-advice 剩余 4 个验证任务不受本 change 影响。

## Capabilities

**Modified Capabilities**（requirement 变更 → 需 delta spec）：

- `auxiliary-label-description-repair`：修复任务的执行调度（单任务串行 → 攒批合并匹配）、匹配阶段的推进语义（一次匹配可同时完成多个任务的条目）、进度暴露（排队任务数）。改写对象为 board-health-triage-advice 引入的「修复后更新匹配并独立重试」「修复任务持久且幂等」「更新完成与效果验证分离」等 requirement。

> ⚠️ **基线依赖**：主 spec `openspec/specs/auxiliary-label-description-repair/spec.md` 尚不存在（board-health-triage-advice 未归档，现行 requirement 文本在其 delta 内，77 行 / 5 个 ADDED Requirement）。本 change 的 specs 制品动工前需二选一：① 先归档 triage change（delta 同步进主 spec，推荐——与「归档后基线干净」一致）；② 先 `openspec sync-specs` 将 triage delta 同步进主 spec。二者均未发生前，specs delta 无合法基线可写 MODIFIED。

## Impact

- **后端**：`internal/tagmanagement/service/auxrepair/`（executor 攒批调度、matching 多任务推进、JobView 增字段）、`internal/tagmanagement/repository/auxiliary_description_repair_matching.go`（计数批量化、批内任务条目标记）、`handler/auxiliary_description_repair_handler.go`（响应字段）；`internal/app/runtime.go` executor 接线（config 或新增攒批参数）。
- **前端**：`front/app/features/tags/components/console/board-health/useAuxiliaryDescriptionRepair.ts`（轮询消费排队字段）、`boardHealthWorkspace.ts`（进度文案派生加排队段）。
- **API**：GET `description-repairs/:job_id` 响应非破坏性追加字段；无新端点、无路由变更。
- **DB**：无表结构变更、无迁移；排队数为运行时查询，批内标记复用现有 jobs/items 表。
