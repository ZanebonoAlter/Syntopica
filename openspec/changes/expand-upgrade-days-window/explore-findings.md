
## 扩充时间窗落点地图

扩充路加 days 的全部落点（探索已核实）：

前端：
- `front/app/features/tags/components/UpgradeSuggestionPanel.vue:266` 「候选时间窗」select 的 v-if 限定 `genDirection==='create' && genSource==='aux'`，放开即四格可用；:267-268 控件本体；:67 emit 时 days 条件携带（改为扩充方向也带）
- `useTagsPage.ts:114-123` handleGenerateUpgradeSuggestions → `semanticBoards.ts:353-356` POST query 参数透传

后端：
- `handler/board_upgrade_handler.go:96-101` days 解析（已存在，非负整数）
- `service/board/semantic_board_upgrade.go:186-196` 四路分发，req.Days 现仅进 generateCreateAux；:203 CollectCandidates(ctx, config, req.Days)
- **create×aux 的过滤模板**（照抄到扩充路）：semantic_board_upgrade.go:498-505 `EXISTS (SELECT 1 FROM article_topic_tags att JOIN topic_tag_semantic_labels ttsl ON ttsl.topic_tag_id=att.topic_tag_id JOIN articles a ON a.id=att.article_id WHERE ttsl.semantic_label_id=semantic_labels.id AND a.created_at >= ?)` cutoff = now-days
- `semantic_board_expand.go:168` recallExpandAuxCandidates 两路：相似路（全库语义距离 ≤ ExpandSimDistance，无时间概念→结果集加 EXISTS 近期活跃过滤）；共现路 loadExpandCooccurrence :278-279 cutoff = now - CoTagWindowDays（全局配置）→ UI days 收紧：cutoff 取 max(now-days, now-CoTagWindowDays)
- `semantic_board_expand.go:357` generateExpandCompose（扩充×组合）共现窗口同样收紧
- days=0 不过滤 = 现状不变

约束核对：定时 board_upgrade_suggest 仅跑创建方向（扩充纯手动，红线10不受影响）；组合共现门槛 semantic_board_upgrade_composite_min_cooccurrence（红线16）在窗口收紧下自然生效不违反。

<!-- pinned 2026-09-09T15:03:38Z -->
