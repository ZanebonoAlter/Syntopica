<!-- doc-impact: none(纯 harness 记账口径改动，不涉业务链路/api/表结构/架构/规范正文) -->

## 1. 实现

- [x] 1.1 `.pi/extensions/lib/edit-map.ts`：`isToolManagedPath` 扩到构建/部署产物（`backend-go/frontend/`、`backend-go/.frontend-before-*`、`front/.output/`、`front/.nuxt/`、`front/dist/`）；新增 `isNonPathToken`（`^-` 开头的 CLI 开关）与统一过滤点 `isExcludedPath`；`syncEditMap`（采集侧两处）与 `latestEditMapByChange`（读取侧）统一改走 `isExcludedPath`。
- [x] 1.2 `scripts/harness/doc-impact.sh`：`filter_blacklist` 加同源 5 条产物前缀 + `^-`，与 edit-map 口径一一对应（采集侧止增、读取侧同源过滤）。
- [x] 1.3 烟测：`quality-gate.smoke.cjs` 新增 F3d（部署产物剔除）/F3e（只认 `backend-go/frontend/` 边界）/F3f（非路径 token）/F3g（统一过滤点）/F5b（产物+开关混合 trigger 不进归属集）。

## 2. 验证

- [x] 2.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → **SMOKE OK（77 项断言全绿）**，F1/F2/F3/F3d/F3e/F3f/F3g/F5/F5b 全 ✅。
- [x] 2.2 `filter_blacklist` 实测：`--full-page` / `backend-go/frontend/index.html` / `backend-go/.frontend-before-matrix-copy.X/a.js` / `front/.output/public/x.js` 全被剔除，只留 `front/app/features/tags/a.vue`。
- [x] 2.3 `doc-impact.sh verify`（本 change 空壳，无代码改动）通过；`openspec validate` valid。

## 3. 文档

- [x] 3.1 无需改 `docs/reference/`：口径只影响 harness 记账，机制说明在代码注释与 `harness/pi-extensions.md` 既有章节内（本次不新增正文，避免重复描述）。

## 4. 测试

- [x] 4.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK（77 项断言全绿）。新增断言：
  F3d 部署产物剔除、F3e 边界（只认 `backend-go/frontend/`）、F3f 非路径 token、F3g 统一过滤点、
  F5b 产物 + 开关混合 trigger 不进归属集合。
- [x] 4.2 `filter_blacklist` 行为实测（5 行输入只留 1 行合法路径）。
<!-- doc-impact: none(纯 harness 记账口径，不涉业务链路/api/表结构/架构/规范正文) -->
