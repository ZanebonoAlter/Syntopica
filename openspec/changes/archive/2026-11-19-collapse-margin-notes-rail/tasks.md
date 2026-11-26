## 1. 实现

- [x] 1.1 `DailyReportReader.vue`：新增收起状态 + `localStorage` 记忆（缺省收起，仅显式切换落盘，程序性展开不落盘）。
- [x] 1.2 `DailyReportReader.vue`：`drm-layout` 增加收起类（两列），桌面收起时右边缘渲染竖向入口浮标（计数 + title/aria）。
- [x] 1.3 `MarginNotesRail.vue`：栏头加收起按钮（`canCollapse` prop + `collapse` emit），抽屉实例不启用。
- [x] 1.4 `DailyReportReader.vue`：`focusAnnotation` 收起态自动展开（新建批注 / 点高亮 / 深链定位共用），新建路径保留聚焦输入框。
- [x] 1.5 浮标与窄屏 FAB 移出 `.drm-peel-host`（`perspective` 会改写 `position: fixed` 包含块），改为 `.drm-reader` 直接子元素并提特异度。

## 2. Review

- [x] 2.1 聚焦审查改动与原型一致性，意见按 High/Medium/Low 处置后记入 `review.md`（R-1 High 已修，R-2/R-3/R-4 Medium 已修，R-5~R-11 Low 已修）。

## 3. 测试

- [x] 3.1 `DailyReportReader.test.ts`：三栏断言改为「默许收起 → 两列 + 浮标」，并补「显式展开后三栏 + 栏头收起按钮」。
- [x] 3.2 `MarginNotesRail.test.ts`：`canCollapse` 时渲染收起按钮并上抛 `collapse`、`aria-expanded="true"`；默认（抽屉实例）不渲染。
- [x] 3.3 `DailyReportReader.test.ts` 补持久化三例 + 窄屏分支一例：显式切换写 `'0'/'1'`、`'0'` 时初始展开、程序性展开不写偏好、窄屏不渲染浮标/第三列。

## 4. 验证

- [x] 4.1 `cd front && pnpm test:unit app/features/tags/components/DailyReportReader.test.ts app/features/tags/components/daily-report/MarginNotesRail.test.ts --maxWorkers=2` → 30 passed / 2 files。
- [x] 4.2 `cd front && pnpm lint` → 0 error（7 条既有 warning 均在其他文件）。
- [x] 4.3 `cd front && pnpm exec nuxi typecheck` → exit 0。
- [x] 4.4 `bash scripts/dev/deploy-frontend.sh --no-restart` → 构建 + 铺盘成功；`curl http://localhost:5100/health` → 200。
- [x] 4.5 `openspec validate collapse-margin-notes-rail --strict` → valid。
- [x] 4.6 浏览器验收（1440×900 与 960×800、editorial + dark 双主题），实测数值与断言见 `ui-verification/report.md`（不得只说「页面正常」）。

## 5. 文档

<!-- doc-impact: flow, standard -->
- [x] 5.1 `docs/reference/flow/daily-report.md`：代码入口行更新页边注栏形态（桌面默认收起 + 右缘浮标），2026-10-27 复审段加「2026-11-19 修订」，变更溯源表补一行。
- [x] 5.2 `docs/reference/standard/frontend/layout.md`：核对后无需修改 —— 该文档的四布局模式与固定视口例外表只登记 L0/L1 矩阵页，日报阅读台三栏不在登记范围内，本次也未新增分区/布局模式。
