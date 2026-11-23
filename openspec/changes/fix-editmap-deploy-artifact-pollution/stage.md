<!-- stage: propose -->
<!-- constraint-domains: semantic-board -->

# fix-editmap-deploy-artifact-pollution — 阶段载体

- 当前阶段：**propose**（proposal 已落盘；ui-design / specs / design / tasks 待办）。
- 来源：`rework-lane-compose-swimlane-canvas` 归档门禁实测暴露（部署产物被 edit.map 记账 → doc-impact 4 域误报）。
- 关键口径：**存量污染不清洗**（append-only），只做到「新快照不携带 + 读取侧过滤」。
