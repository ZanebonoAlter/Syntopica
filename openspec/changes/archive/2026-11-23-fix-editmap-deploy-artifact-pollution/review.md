# Review 台账 · fix-editmap-deploy-artifact-pollution

### R-1 2026-11-23【Medium】部署产物与 CLI 开关被当成「本次编辑的文件」

- **状态**：已修复
- **发现**（启用本 change 的实测）：change `rework-lane-compose-swimlane-canvas` 归档时 `edit.map` 归属集里混着
  `--full-page`（CLI 开关）、`backend-go/.frontend-before-matrix-copy.KtDRQM/_nuxt/*.js`（部署备份）、
  `backend-go/frontend/**`（静态托管产物）等 1195 条非本 change 的路径，导致 `doc-impact verify`
  对 api/database/architecture/standard 四个域全部误报「疑似遗漏」。
- **处置**：① `isToolManagedPath` 扩到 5 类构建/部署产物前缀；② 新增 `isNonPathToken`（`^-` 开头）
  与统一过滤点 `isExcludedPath`，采集侧两处 + 读取侧一处全部改走它；③ `doc-impact.sh` 的
  `filter_blacklist` 同源加 5 条前缀 + `^-`；④ 烟测补 F3d/E/F/G 与 F5b。
- **审毕无发现（审查范围：与 `.pi/extensions/lib/edit-map.ts` 现有 `isToolManagedPath` 口径一致性、
  存量污染不清洗纪律、fail-open 语义）**
