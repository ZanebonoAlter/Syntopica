
## pgx 参数左操作数绑定失效 + 自相似击穿（匹配系统两坑）

Syntopica 后端（GORM + pgx + PostgreSQL）写参数化 SQL 时的两个实证陷阱（fix-board-match-cross-domain 实现期踩中，2026-10-07）：

1. **参数作比较左操作数静默失效**：`Where("? <= 0 OR NOT EXISTS(...)", gap, gap)` 形态——pgx 对「参数直接作 `<=` 左操作数」的绑定求值异常，实测 `0.3 <= 0` 求值为 true，且 EXPLAIN 计划中该 OR 条件整体被优化器消除（子查询独立求值 EXISTS=true，嵌入后不生效）。修法：开关判断放 Go 侧分支（gap<=0 不拼该 Where），SQL 内参数只出现在右侧位置（`score + ?` 形态实测完全正常）。

2. **自相似击穿相似度路径**：scoreSemanticBoardSimilarity 是 tag 辅助标签向量 vs board 构成标签向量的两两余弦——词面相同的标签（同一 ID 在 tag 关联和 board_composition 两边都有）余弦恒为 1.0。任何「按重叠标签拦截挂载」的逻辑若不把重叠标签从后续相似度证据中剔除，拦截会被 max_sim 满分变相绕过（1.0 分甚至比 direct_hit 的 0.7 更高，且与真归属并列分数令排他失效）。修法见 semantic_board_matching.go 的 boardOverlapIDs + tagVectorsExcluding（证据不得换赛道原则）。

排查方法论：DryRun Session 拿 GORM 最终 SQL+vars → Raw 同 SQL 执行 → EXPLAIN 看条件是否被优化器消除（条件消失=绑定层问题，不是逻辑问题）。注意 Scan 到标量时 SELECT 列的含义（SELECT id Scan 到 int 拿到的是第一行 id 值不是行数，易误读）。

<!-- pinned 2026-10-07T10:11:58Z -->
