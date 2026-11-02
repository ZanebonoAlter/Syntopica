<!-- doc-impact: daily-report -->

## 1. 测试用例先行（§2：用例先行，契约变更 ⓪）

- [x] 1.1 写 `test-cases.md`：主链路故事表（点线索文章 → 直达预览弹窗：步/动作/来源 Scenario/期望/层/落点）+ **继承与调整表**（旧「抽屉内站内阅读→预览」场景 × 处置 × `DailyReportDeskShell.test.ts` 现有断言 × 动作）+ 变体走查（可用性组：加载态/错误态/空态 + 重复点击/超长标题）+ 效果核对留痕。验证：`bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产已逐行处置，文件含全部小节

## 2. 实现（front/app/features/tags/）

- [x] 2.1 `DailyReportDeskShell.vue`：`@open-article` 改 async 直达（`getArticle(id)` → `normalizeArticle` → `previewArticle`/`previewOpen`），新增 `loadingPreviewArticle` 传弹窗既有 prop，失败 `notify.warn` 且不留空弹窗；删除 `articleId` ref 与 `DeskArticleDrawer` import/渲染。验证：`cd front && pnpm test:unit DailyReportDeskShell.test.ts --maxWorkers=2` 全绿
- [x] 2.2 改写 `DailyReportDeskShell.test.ts`：删 drawer stub，新增「emit open-article → 直接渲染 preview stub 且文章数据正确」「加载中 loadingPreviewArticle=true」「接口失败 notify.warn 且 preview 不开」断言；保留收藏乐观更新/回滚/状态回流断言。验证：同 2.1 命令全绿
- [x] 2.3 删除 `DeskArticleDrawer.vue` 文件；核对 `pages/tags/[boardId]/[date].test.ts` 引用面。验证：`grep -rn "DeskArticleDrawer" front/app --include="*.vue" --include="*.ts"` 仅剩删除后零命中；`cd front && pnpm exec nuxi typecheck` 0 error
- [x] 2.4 全量前端收尾检查。验证：`cd front && pnpm lint` 0 error；`cd front && pnpm build` 构建成功

## 3. 文档（flow 活文档同步）

- [x] 3.1 `docs/reference/flow/daily-report.md`：L172 链路图去掉抽屉中转（点击线索 → 直达 ArticlePreviewModal）、L278 代码入口删 `DeskArticleDrawer.vue` 条目并更新 `DailyReportDeskShell.vue` 描述、约束 32 措辞「文章抽屉『站内阅读』打开的预览弹窗」改为「线索直达的预览弹窗」。验证：`grep -n "DeskArticleDrawer" docs/reference/flow/daily-report.md` 零命中，约束 32 语义不变（乐观更新契约保留）

## 4. 验证（命令 + 期望结果）

- [x] 4.1 影响范围机械判定：`bash scripts/harness/change-scope.sh` → 输出含 `pnpm test:unit`（daily-report 组件文件）与 lint/typecheck/build，未命中项留痕
- [x] 4.2 组件测试（受影响文件，树莓派限流）：`cd front && pnpm test:unit DailyReportDeskShell.test.ts "pages/tags" --maxWorkers=2` → 全部通过、0 fail
- [x] 4.3 后端零改动核对：`git status --short backend-go/` → 空（本 change 不触后端，无需跑 go test）
- [x] 4.4 端到端主链路（agent-browser，静态 :5100）：部署后打开 `/tags/:boardId/:date` 阅读台 → 展开线索相关文章 → 点击标题 → 断言预览弹窗出现在视口中心（elementFromPoint 命中弹窗）且页面中不存在 `desk-article-drawer` 元素 → Esc 关闭回到原位；结果记入 `ui-verification/report.md`
- [x] 4.5 部署收尾：`bash scripts/dev/deploy-frontend.sh` → 构建、铺盘、重启、健康检查全通过（用户打开页面即新版）
- [ ] 4.6 归档前置门禁：`bash scripts/harness/doc-impact.sh verify` 与 `bash scripts/harness/check-standards.sh` → 全绿；确认 `rework-workshop-into-radar-and-desk` 已归档（`openspec list` 无该行）——未归档则按 design.md D4 风险项先同步其 delta 再归档本 change，归档后按 §12 在 flow/daily-report.md 补变更溯源链接
