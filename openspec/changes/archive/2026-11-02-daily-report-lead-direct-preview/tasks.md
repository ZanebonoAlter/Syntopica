## 1. 测试用例先行（§2：用例先行，契约变更 ⓪）

- [x] 1.1 写 `test-cases.md`：主链路故事表（点线索文章 → 直达预览弹窗：步/动作/来源 Scenario/期望/层/落点）+ **继承与调整表**（旧「抽屉内站内阅读→预览」场景 × 处置 × `DailyReportDeskShell.test.ts` 现有断言 × 动作）+ 变体走查（可用性组：加载态/错误态/空态 + 重复点击/超长标题）+ 效果核对留痕。验证：`bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产已逐行处置，文件含全部小节

## 2. 实现（front/app/features/tags/）

- [x] 2.1 `DailyReportDeskShell.vue`：`@open-article` 改 async 直达（`getArticle(id)` → `normalizeArticle` → `previewArticle`/`previewOpen`），新增 `loadingPreviewArticle` 传弹窗既有 prop，失败 `notify.warn` 且不留空弹窗；删除 `articleId` ref 与 `DeskArticleDrawer` import/渲染。验证：`cd front && pnpm test:unit DailyReportDeskShell.test.ts --maxWorkers=2` 全绿
- [x] 2.2 改写 `DailyReportDeskShell.test.ts`：删 drawer stub，新增「emit open-article → 直接渲染 preview stub 且文章数据正确」「加载中 loadingPreviewArticle=true」「接口失败 notify.warn 且 preview 不开」断言；保留收藏乐观更新/回滚/状态回流断言。验证：同 2.1 命令全绿
- [x] 2.3 删除 `DeskArticleDrawer.vue` 文件；核对 `pages/tags/[boardId]/[date].test.ts` 引用面。验证：`grep -rn "DeskArticleDrawer" front/app --include="*.vue" --include="*.ts"` 仅剩删除后零命中；`cd front && pnpm exec nuxi typecheck` 0 error
- [x] 2.4 全量前端收尾检查。验证：`cd front && pnpm lint` 0 error；`cd front && pnpm build` 构建成功

## 3. 文档（flow 活文档同步）

<!-- doc-impact: flow -->

- [x] `docs/reference/flow/daily-report.md`
- [x] 3.1 `docs/reference/flow/daily-report.md`：L172 链路图去掉抽屉中转（点击线索 → 直达 ArticlePreviewModal）、L278 代码入口删 `DeskArticleDrawer.vue` 条目并更新 `DailyReportDeskShell.vue` 描述、约束 32 措辞「文章抽屉『站内阅读』打开的预览弹窗」改为「线索直达的预览弹窗」。验证：`grep -n "DeskArticleDrawer" docs/reference/flow/daily-report.md` 零命中，约束 32 语义不变（乐观更新契约保留）

## 4. 验证（命令 + 期望结果）

- [x] 4.1 影响范围机械判定：`bash scripts/harness/change-scope.sh` → 输出含 `pnpm test:unit`（daily-report 组件文件）与 lint/typecheck/build，未命中项留痕
- [x] 4.2 组件测试（受影响文件，树莓派限流）：`cd front && pnpm test:unit DailyReportDeskShell.test.ts "pages/tags" --maxWorkers=2` → 全部通过、0 fail
- [x] 4.3 后端零改动核对：`git status --short backend-go/` → 空（本 change 不触后端，无需跑 go test）
- [x] 4.4 端到端主链路（agent-browser，静态 :5100）：部署后打开 `/tags/:boardId/:date` 阅读台 → 展开线索相关文章 → 点击标题 → 断言预览弹窗出现在视口中心（elementFromPoint 命中弹窗）且页面中不存在 `desk-article-drawer` 元素 → Esc 关闭回到原位；结果记入 `ui-verification/report.md`
- [x] 4.5 部署收尾：`bash scripts/dev/deploy-frontend.sh` → 构建、铺盘、重启、健康检查全通过（用户打开页面即新版）
- [x] 4.6 归档前置门禁：`bash scripts/harness/archive-readiness.sh openspec/changes/daily-report-lead-direct-preview` → 全绿（doc-impact / check-standards / 尾三节 / scenario-trace / review-trace / UI 验收六项）；`rework-workshop-into-radar-and-desk` 未归档（157/160）→ 按 design.md D4 先同步其 delta 的「阅读台文章抽屉与站内阅读预览的状态同步」进主 spec 再归档本 change，并在 radar delta 删除被本 change 取代的该需求、把 thread 点击句改为直达口径（防其归档时复活旧契约）；归档后按 §12 在 flow/daily-report.md 补变更溯源链接
- [x] 4.7 域外红登记：本 change 影响包（`DailyReportDeskShell.test.ts` / `pages/tags` / `ArticlePreviewModal.test.ts`）零失败，未撞见域外红，台账无需新增

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 点击线索文章直达预览弹窗 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 详情加载中呈现加载态 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 详情加载失败可见反馈 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 站内阅读预览收藏即时生效 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 收藏失败回滚并提示 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 连点不丢更新 | front/app/features/articles/composables/useArticleFavoriteToggle.test.ts |
| 预览内文章状态更新回流 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 展示日报卡片列表 | front/app/features/tags/components/DailyReportReader.test.ts |
| 打开全屏日报详情 | front/app/pages/tags/[boardId]/[date].test.ts |
| 宽屏双栏与窄屏降级 | 人工：布局契约沿用 rework-workshop-into-radar-and-desk 归档证据（matrix-stage 双视口），本 change 未改布局 |
| 双主题渲染 | 人工：主题 token 未动，沿用 rework-workshop-into-radar-and-desk 明暗双主题证据 |
| 按持久话题状态分区 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| 展开并缓存话题生命线 | front/app/features/tags/components/DailyReportReader.test.ts |
| identity 连线跨越空白日期 | front/app/features/tags/components/daily-report/dailyReportMagazine.test.ts |
| lifeline 加载失败 | front/app/features/tags/components/DailyReportReader.test.ts |
| 展开 thread 文章并复用缓存 | front/app/features/tags/components/DailyReportReader.test.ts |
| 进入完整话题生命线 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
| 键盘关闭详情 | front/app/features/tags/components/DailyReportReader.test.ts |
| 空状态 | front/app/pages/tags/[boardId]/[date].test.ts |
| 加载超过 30 天 | front/app/features/tags/components/DailyReportReader.test.ts |
| 切换 board 重置状态 | front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts |
