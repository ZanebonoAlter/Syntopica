<!-- stage: implement -->

# fix-editmap-deploy-artifact-pollution — 阶段载体

- 当前阶段：**propose**（proposal 已落盘；ui-design / specs / design / tasks 待办）。
- 来源：`rework-lane-compose-swimlane-canvas` 归档门禁实测暴露（部署产物被 edit.map 记账 → doc-impact 4 域误报）。
- 关键口径：**存量污染不清洗**（append-only），只做到「新快照不携带 + 读取侧过滤」。
- 2026-11-23：实现完成（edit-map 采集侧 + doc-impact 读取侧同源过滤），烟测 77 项全绿；待归档。
