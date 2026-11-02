# Design: add-ghost-mount-unmount

## Context

direction_mismatch 只标记不摘除（fix-board-ghost-false-positive 起的保守设计），科技巨头板 167 个双挂 ghost 需要一条「用户主权」动线摘除。最大设计坑是防复活：匹配管道弱证据路径（weighted/max_sim）会在重算时挂回同样内容——卸载必须配墓碑，否则动线无效。

## Goals / Non-Goals

**Goals**
- 卸载（删挂载+墓碑）→ 重算不复活 → 可撤销（删墓碑+重算）完整闭环。
- 体检快照附属候选（双挂口径）驱动用户发现；显式 API 执行摘除。

**Non-Goals**
- 不动匹配判定规则（挂载为什么发生归管道治理）；不动方向校验。
- 不做单挂形态摘除（裸奔风险）；不做墓碑 TTL/自动清理（量小，人工可管）。
- 不做前端新布局（复用未收编候选卡片契约）。

## Decisions

### D1. 墓碑模型与过滤点（截断前）
- **结论**: `tag_models.go` 新增 `TopicTagBoardLabelTombstone{ID, TopicTagID, SemanticBoardID, Reason, CreatedAt}`，UNIQUE(topic_tag_id, semantic_board_id)，AutoMigrate 经 wire.go 注入注册（与 TopicTagBoardLabel 同址）。MatchTopicTag 在 `evaluateSemanticBoardMatches` 返回后、`MaxBoards` 截断前调用 `s.filterTombstonedMatches(ctx, topicTagID, matches)`（单查该 tag 墓碑，内存过滤）；所有匹配路径（增量/rematch-all/backfill/撤销重算）都过 MatchTopicTag，单点收口。
- **落点**: `backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:MatchTopicTag`
- **取舍**: 备选在 replaceTopicTagBoardLabels（写库层）过滤——被否：截断发生在 replace 之前，写库层过滤会让墓碑行先占 MaxBoards 名额挤掉合法候选，且 evaluate→截断→replace 的中间语义更难测；备选 evaluate 纯函数内过滤——被否：纯函数无 db 访问，引入墓碑参数破坏「判定与持久化分离」的测试面。

### D2. 卸载 API 事务边界与幂等
- **结论**: `POST /api/semantic-boards/:id/unmount-tags`，body `{topic_tag_ids: []uint}`；handler 校验板存在且 label_type=board（404）后，单事务内逐 ID：删 `topic_tag_board_labels` 行（不存在跳过）+ upsert 墓碑（`ON CONFLICT DO NOTHING`）。响应 `{removed: n, tombstoned: n, skipped: n}`。
- **落点**: `backend-go/internal/tagmanagement/handler/board_crud_handler.go`（与 rematch-all 同注册组）
- **取舍**: 备选逐条独立事务——被否：批量场景部分成功部分失败会让 UI 状态难解释；同事务全成全败语义简单。幂等 upsert 用 DB 层 ON CONFLICT（与 config 写入同模式），避免读-判-写竞态。

### D3. 墓碑管理与撤销重算
- **结论**: `GET /api/semantic-boards/:id/mount-tombstones`（JOIN topic_tags 带 label，checked_at 倒序）；`DELETE /api/semantic-boards/:id/mount-tombstones/:topicTagId`：事务删墓碑，事务外（成功后）调 `MatchTopicTag(tagID)` 重算——重算失败不回滚删墓碑（墓碑已删，重算可由任何后续匹配自愈），响应含 matches 摘要。
- **落点**: `backend-go/internal/tagmanagement/handler/board_crud_handler.go`
- **取舍**: 备选重算放事务内——被否：MatchTopicTag 自带独立事务（replace 全删全写），嵌套事务徒增复杂度；重算失败仅记日志（幂等自愈路径充分：增量匹配/下次 rematch 都会重算）。

### D4. 体检候选收集（双挂口径，快照附属列）
- **结论**: boardhealth `run.go` 新增 `UnmountCandidates(ctx, db, boardID)`（纯读 SQL：mismatch 行 JOIN 同 tag 其它非 mismatch 挂载存在性 + 全库最优板 cosine/名称，mounted cosine 升序 LIMIT N），写入快照新列 `unmount_candidates`（string JSON，模型 nullable，与 UncapturedCandidates 同模式）；N 走 config（`board_health_unmount_candidates_limit` 默认 20，boardhealth Config 既有管道）。
- **落点**: `backend-go/internal/tagmanagement/service/boardhealth/run.go`
- **取舍**: 备选实时端点（体检面板现查）——被否：快照口径让候选随轮次沉淀（趋势可见）、且红线 20 的纯读边界已有 uncaptured 先例；备选进 board_upgrade_suggestions 新 decision——被否：撞红线 20「体检轮次 pending 行零新增」。

### D5. 前端复用未收编候选卡片
- **结论**: `BoardHealthPanel.vue` 在未收编候选卡片下方同构新增「卸载建议」区块（清单+复选+批量卸载 → unmount-tags API）与「已卸载」次级区块（墓碑列表+逐行恢复 → 两个墓碑端点）；api client 加三个方法。无新组件形态。
- **落点**: `front/app/features/tags/components/BoardHealthPanel.vue`
- **取舍**: 无备选：ui-design minor 档复用既有契约（未收编候选交互模式），零新布局。

## Risks / Trade-offs

- [墓碑静默吞挂载（用户忘了墓碑存在）] → 墓碑列表端点+前端「已卸载」区块可见；量小（预计 <300 行）人工可管。
- [双挂口径漏掉单挂真串味] → 明确 Non-Goal，单挂归板构成治理（真家接住问题）另立 change。
- [撤销后重算失败] → 自愈路径充分（增量匹配/下次 rematch 必然重算），失败仅日志。
- [体检 SQL 双挂判定成本] → 单板 mismatch 行数最多 ~180（科技巨头），EXISTS 子查询规模可控。

## Migration Plan

1. 合并（AutoMigrate 自动建表+加列，无需手写迁移）→ 2. 部署重启 → 3. 下一轮体检快照自动带 unmount_candidates → 4. 前端体检面板操作卸载/恢复。
回滚：卸载动线无数据不可逆风险（墓碑可删、挂载可重算）；表回滚 drop table（墓碑数据非业务资产）。

## Open Questions

（无）
