## Context

`unify-tag-hierarchy` 将标签体系拆成了多个能力：Sector 管理、层级模板、放置、清理、重建、PendingChange 和 `/tags` 页面。但这些能力目前由 handler、scheduler、frontend dialog 和零散 service 各自拼接，缺少一个端到端编排边界。

当前主要断点：

- `TemplateSettingsDialog` 调用 `PUT /hierarchy/config` 时，后端已经保存配置并启动重建；前端后续“确认重建”只是关闭弹窗。
- `RebuildService` 推送 `rebuild_progress` / `rebuild_complete`，但 `useWebSocketRebuild` 只监听 `hierarchy_rebuild`。
- `AutoGenerateSectors` 未接入 placement scheduler，冷启动时 `MatchTagToConcept` 持续返回 nil。
- `PlaceTagInHierarchy` 找不到 parent Node 时返回 `unplaced`，没有调用已有的 Node 创建路径。
- `GenerateAnchorSignals` 只返回内存结果，cleanup scheduler 只统计数量，placement 流程无法消费。
- Sector LLM confirm API 只返回 message，前端自行推断成功数。

约束：

- 单用户系统，不需要复杂并发控制，但需要避免同一 category 的 rebuild/placement 互相踩踏。
- PostgreSQL + pgvector 是持久层，允许新增轻量表或复用现有 metadata 字段，但优先减少 schema 变化。
- LLM 调用需要限流、可观察和可失败恢复。
- `/tags` 是统一管理入口，TopicGraphPage 不重新承担层级管理职责。

## Goals / Non-Goals

**Goals:**

- 建立一个可测试的用户故事闭环：Inspect → Bootstrap Sector → Place Tags → Validate → Rebuild/Review → Refresh UI。
- 让 Template 变更成为真正的两阶段流程：preview 不产生副作用，apply 才保存并重建。
- 让 `PlaceTagInHierarchy` 真正成为唯一 Node 创建入口，并在无法闭合时返回明确 blocker。
- 统一 rebuild WebSocket 协议，使 `/tags` 能实时显示进度、完成和失败。
- 将 Sector auto bootstrap 接入放置/重建编排，解决无 Sector 时的冷启动。
- 让 LLM Sector 审批显示后端真实执行结果。
- 为闭环路径补足验收测试，防止后续再次“任务打勾但故事断裂”。

**Non-Goals:**

- 不重新设计标签抽取、embedding 生成或文章阅读反馈链路。
- 不恢复 `merged` / `inactive` 的 topic_tags 软状态。
- 不把 `/topic-graph` 重新变成层级管理入口。
- 不引入多用户协作、权限或审计历史。
- 不在本 change 中做大规模包拆分；只在必要范围内收敛编排逻辑。

## Decisions

### D1: 新增层级闭环编排边界

**决策**: 在后端引入一个明确的 hierarchy orchestration service，用于串联以下动作：统计 category 状态、必要时 bootstrap Sector、执行 leaf Tag placement、创建 Node、启动 rebuild、审批 PendingChange、刷新状态。handler 和 scheduler 调用 orchestration service，不再各自拼流程。

**理由**: 当前失败根因是多个局部功能没有共同状态机。编排边界能把“用户故事是否闭合”变成单元测试和验收测试可验证的行为。

**替代方案**: 继续在现有 handler/scheduler 中补 if/else。被否决，因为这会扩大隐式耦合，下一次变更仍然容易漏掉闭环。

### D2: Template 变更拆成 preview/apply 两阶段

**决策**: `PUT /hierarchy/config` 不再同时承担预览、保存和重建。实现方式可以是：

- `POST /hierarchy/config/preview`: 输入 templates，返回 impact + estimated rebuild time，无副作用。
- `PUT /hierarchy/config`: 输入 templates + confirm/apply 标记，保存配置、删除旧 Node、创建并执行 rebuild job。

如果为了兼容保持原路由，必须用明确字段区分 preview/apply；默认行为必须无副作用。

**理由**: 用户确认必须控制副作用。当前前端“确认重建”不触发任何真实动作，是闭环断裂的典型表现。

**替代方案**: 保持现有 PUT 行为，只改按钮文案为“保存并重建”。被否决，因为用户无法取消已保存的高风险模板变更。

### D3: Rebuild 事件使用单一协议

**决策**: 后端统一发送 `type: "hierarchy_rebuild"`，并通过 `status` 表达 `processing` / `completed` / `failed`；payload 包含 job_id、category、processed、total、failed_count、estimated_remaining_seconds、current_tag、error。前端只消费该协议。

**理由**: 前后端消息类型不一致会让进度条完全失效。单一协议也让 acceptance 测试更简单。

**替代方案**: 前端兼容多个 type。可作为短期过渡，但最终仍应收敛为单一协议，避免事件语义漂移。

### D4: PlaceTagInHierarchy 必须闭合 Node 创建或返回 blocker

**决策**: 当 Tag 已匹配 Sector、embedding 已就绪、目标 level 合法，但找不到合适 parent Node 时，`PlaceTagInHierarchy` 必须进入 Node 创建决策：

1. 收集候选 sibling/anchor context。
2. 校验信息增益：至少 2 个候选子 Tag、文章重叠不过高、leaf-to-depth ratio 合格。
3. 创建 Node 并链接 Tag，或返回结构化 blocker，例如 `insufficient_siblings`、`low_information_gain`、`no_anchor_context`。

**理由**: “唯一入口”如果不能创建 Node，就只是放置入口，不是层级生长入口。静默 `unplaced` 会让用户看不到系统为什么无法修复。

**替代方案**: 仍由 `AggregateOrphanTags` 创建 Node。被否决，因为它重新引入第二 Node 创建路径，违背统一入口目标。

### D5: Sector bootstrap 进入 placement/rebuild 前置步骤

**决策**: placement cycle 和 rebuild job 在处理 category 前调用 orchestration 的 bootstrap step：如果 active Sector 为空或 unplaced count 超过阈值，则触发 `AutoGenerateSectors`，成功创建 Sector 后重试本轮 placement。

**理由**: `MatchTagToConcept` 依赖 Sector embedding。没有 bootstrap，所有后续放置都会卡在 `no_matching_concept`。

**替代方案**: 只让用户手动创建 Sector。被否决，因为 proposal 明确要求 auto 解决冷启动，且用户不应先理解内部概念才能完成首次闭环。

### D6: Anchor signal 要么持久可消费，要么移除承诺

**决策**: 优先实现轻量持久化，例如 `hierarchy_anchor_signals` 或复用 JSON metadata，记录 category、center_tag_id、member_tag_ids、expires_at。`PlaceTagInHierarchy` 的 anchor search 读取这些 signals。若不做持久化，则删除 Phase 7 对“anchor signal 输入 placement”的承诺，只保留日志性聚类统计。

**理由**: 只生成不消费的 signal 会误导用户和开发者。闭环系统里每个 phase 必须有明确下游。

**替代方案**: 用内存缓存。被否决，因为 scheduler/rebuild 跨 goroutine、跨重启时不可恢复。

### D7: Sector LLM confirm 返回真实执行结果

**决策**: `confirmRegenerateSectors` 返回逐项结果：每个 add/merge/split 的 status、affected_tag_count、created_ids、moved_tag_count、error。前端只展示后端返回事实。

**理由**: 前端按 diff 估算会把部分失败展示成成功，导致用户误判结构已修复。

**替代方案**: confirm 后重新 GET sectors 并推断。被否决，因为无法表达逐项失败原因。

### D8: PendingChange 审批只执行明确建议，不重新猜测

**决策**: PendingChange approval 应根据 change_type 执行对应动作；如果 change 缺少可执行 payload，则标记 failed 并返回原因。不能简单调用 `PlaceTagInHierarchy` 重新让系统猜一次。

**理由**: 审批语义是“接受这个建议”，不是“重新跑一次自动放置”。否则用户确认的对象和系统执行的对象不一致。

## Risks / Trade-offs

- **[Risk] 编排 service 变成上帝对象** → 缓解：只放流程状态机和调用顺序，具体匹配、创建、重建仍留在已有 domain service。
- **[Risk] Template API 变更影响现有前端调用** → 缓解：前端同步修改；如需兼容，保留原 route 但默认 preview，无 confirm 不执行副作用。
- **[Risk] Node 自动创建质量不稳定** → 缓解：严格信息增益校验；无法满足时返回 blocker 并在 UI 展示未闭合原因。
- **[Risk] Auto Sector 生成增加 LLM 成本** → 缓解：阈值触发、category 限流、复用 ai_call_logs 观测、已有 active rebuild 时不重复触发。
- **[Risk] Anchor signal 持久化增加 schema** → 缓解：先评估是否可复用现有 pending/metadata；如果新增表，保持短 TTL 和简单索引。
- **[Trade-off] 两阶段 Template 流程增加一次 API 调用** → 可接受，因为 Template 变更低频且风险高。

## Migration Plan

1. 先新增/修正测试，复现闭环断点：Template 确认副作用、WebSocket 类型不一致、无 Sector 冷启动、无 parent 时不创建 Node。
2. 引入 orchestration service，但先只让 scheduler 和新 API 调用它，避免一次性替换所有旧入口。
3. 修改 Template API 和前端 dialog，使 preview/apply 语义一致。
4. 统一 rebuild event payload，并更新 `useWebSocketRebuild`。
5. 修正 placement：Sector bootstrap、Node 创建、blocker 返回。
6. 修正 Sector confirm 与 PendingChange approval 的执行结果。
7. 补 `/tags` 用户故事验收，验证完整闭环。

Rollback 策略：保留旧 rebuild job 表和旧层级数据，若新 placement 失败，可停止 scheduler，恢复上一版 API 行为，并通过手动 rebuild 重新放置。由于不恢复软删除状态，rollback 不保证已硬删除 Node 可恢复。

## Open Questions

- Anchor signal 是否值得新增表，还是直接删除该概念并让 placement 使用 cotag/embedding anchors？
- Template preview/apply 是否拆成新路由，还是保持 `/hierarchy/config` 并增加 `mode` 字段？
- UI 是否需要展示每个 unplaced Tag 的 blocker 原因，还是先只展示聚合统计？
