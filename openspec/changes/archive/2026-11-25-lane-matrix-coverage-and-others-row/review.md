# Review 台账：lane-matrix-coverage-and-others-row

> 审查范围：change 制品（proposal / design / specs / ui-design / tasks）+ 两批实现的完整 diff（后端 `lane-snapshot` 聚合、前端矩阵覆盖组 / 就地改名 / 其他话题整行）。主线程逐文件核对，重点检查口径一致性、边界诚实性与 spec 对齐。

### R-1 2026-11-25【High】watch 指向归档话题时，其分区会从泳道与其他话题两边同时消失

- **状态**：已修复（`otherTopicsExcludedTopicIDs` 只在非归档话题里取 active ∪ watch；归档话题即使仍被 watch 指向也计入其他话题；重跑 repository 测试全绿）

### R-2 2026-11-25【Medium】其他话题整行把「档窗外未聚合」说成「无日报」

- **状态**：已修复（壳层下发 `otherTopicsSince = anchor-(days-1)`；档窗外日期 title 改「档窗外，其他话题读数未取到」，仍「—」禁用；补组件用例）

### R-3 2026-11-25【Low】无任何 completed 日报时渲染一整行「—」空壳

- **状态**：已修复（`showOtherTopicRow` 要求 `available && Object.keys(days).length > 0`；补用例）

### R-4 2026-11-25【Low】覆盖组行名字若保持可点选中，会清掉其他话题阅读槽却无可读内容

- **状态**：已采纳（覆盖行名字改静态标签，仅保留 ⋯ 重命名/编辑/归档；样式与活跃行同款）

### R-5 2026-11-25【Low】矩阵「重命名」入口与修正工作面写入口径可能漂移

- **状态**：已采纳（两处都走 `useDailyReportsApi.updateTopic` → `PATCH /daily-reports/topics/:id`，空名校验一致；矩阵侧只多了成功后的名单/覆盖清单刷新）
