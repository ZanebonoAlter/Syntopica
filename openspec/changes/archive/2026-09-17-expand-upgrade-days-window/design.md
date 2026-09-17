# Design: expand-upgrade-days-window

## Context

见 proposal.md「Why」。现状：`UpgradeSuggestionPanel.vue:266` 的候选时间窗 `v-if` 限定 create×aux 单格；后端 `semantic_board_upgrade.go` 四路分发中 `req.Days` 仅进 `generateCreateAux`；扩充路（`semantic_board_expand.go`）相似路无时间概念、共现路用全局 `CoTagWindowDays`。create×aux 已有成熟的 EXISTS 文章活动过滤模板可复用。

## Goals / Non-Goals

**Goals**
- days 参数贯通扩充×单标签（相似路 + 共现路）与扩充×组合（共现窗口）三处召回
- days=0 与现状逐字节一致（零行为漂移的回归底线）

**Non-Goals**
- 不改创建方向行为（不动 `CollectCandidates` 现有逻辑）
- 不改定时任务 `board_upgrade_suggest`（仅跑创建方向，扩充纯手动）
- 不做时间窗组件抽象/公共化（弹窗内单一控件，无复用方）
- 不改 LLM 裁决 prompt（时间窗只影响送裁前的候选集）

## Decisions

**D1 相似路过滤位置：候选集出队后、排序前施加 EXISTS 过滤**
备选是在 SQL 里 join（像 create×aux 那样进 WHERE）。但扩充相似路是「全量拉 active aux 到内存逐个算距离」，没有单条 SQL 可挂 WHERE；对召回结果（上限 `expandRecallLimit` 量级）逐个做 EXISTS 子查询代价可接受，且实现上复用同一个子查询模板。
- 落点：`recallExpandAuxCandidates` 相似路 simHits 收集处，days>0 时对每个命中标签执行「近 N 天文章引用」EXISTS 查询（批量 `IN` 查询避免 N+1：一次查出窗口内有文章引用的标签 ID 集，再过滤内存集合）。

**D2 共现路收紧：cutoff = max(now-days, now-CoTagWindowDays)**
即取更早时间戳者失效（更严窗口）。实现：`loadExpandCooccurrence` 与 compose 段的窗口计算处，days>0 时用 `min(days, CoTagWindowDays)` 作为有效窗口天数。不做 UI 覆盖全局配置（保留全局兜底下限语义），days 只收紧不放宽。

**D3 days 传递路径：handler 已解析 → 分发时透传**
`board_upgrade_handler.go:96-101` 已解析 days 进 `UpgradeGenerateRequest`；改动仅在 `generateExpandSuggestions` / `generateExpandCompose` 签名与调用处透传 `req.Days`，与 create 路同一字段。

**D4 前端：只放开显隐 + 请求携带，不动控件本体**
`v-if` 从 `genDirection==='create' && genSource==='aux'` 放开为恒显；`:67` 的 days 携带条件同步放开（扩充方向也带）。组合方向带 days 无害（后端仅用于共现窗口收紧）。

## Risks / Trade-offs

- [相似路窗口过滤后候选骤减，用户误以为功能坏了] → 扩充结果空列表文案已有「该版块可能已充分覆盖」，补充提示语义（可含时间窗描述）在 UI 文案层处理，不进本 design 范围
- [EXISTS 批量查询在大标签库上的延迟] → 相似路召回结果本身有 `expandRecallLimit` 上限，`IN` 批量查询单次执行，风险可控；实测超预期再加 `article_topic_tags.created_at` 索引评估
- [days 只收紧不放宽导致「想要更长共现窗口」无法表达] → 记录为已知限制，全局配置仍是上限；当前无此用户需求，不预设

## Migration Plan

无表结构变更、无 API 形状变更，直接部署。回滚 = revert 单次提交。

## Open Questions

无。
