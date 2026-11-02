# 日报阅读台文章预览：收藏状态同步修复的实页复验（2026-10-28）

- 场景：用户反馈「从日报触发的文章预览，点击收藏没反应 / 状态更新不及时」
- 复验人：父线程（实现者），agent-browser 0.37.1，会话 `fix-fav-6d5795ec8cb0`（收尾 `close` 本会话，未 `close --all`）
- 入口：`http://localhost:5100/tags/1974/2026-10-27`（同源静态托管；部署后产物，`bash scripts/dev/deploy-frontend.sh`）
- 结论：**PASS** —— 收藏点击即时翻转并真实落库；取消收藏同样即时；测试数据已还原。

## 1. 缺陷根因（修复前）

`front/app/features/tags/components/daily-report/DailyReportDeskShell.vue` 把站内阅读预览弹窗的状态事件接成空函数：

```vue
@favorite="() => {}"
@article-update="() => {}"
```

→ `ArticlePreviewModal` → `ArticleContentView` → 工具栏星标 `emit('favorite', id)` 后被吞掉：**既不调接口也不改界面**，表现为「点了收藏毫无反应」。同一弹窗在矩阵页/态势页虽已接线（`useTagsPage.handleArticleFavorite`），但要**等接口返回才翻转星标**，且连点会因读到旧值而丢更新——即用户说的「状态更新不及时」。

## 2. 修复内容

| 落点 | 改动 |
| --- | --- |
| `front/app/features/articles/composables/useArticleFavoriteToggle.ts`（新增） | 收藏乐观更新契约：点击即翻转 → `PUT /api/articles/:id` → 失败回滚 + `notify.error`；回滚仅在宿主当前值仍是本次乐观值时执行（迟到的失败响应不覆盖后续点击） |
| `DailyReportDeskShell.vue` | `@favorite` / `@article-update` 从空函数换成真实处理（乐观翻转 / 状态回流） |
| `useTagsPage.ts` | `handleArticleFavorite` 改走同一乐观更新契约（此前等接口返回才翻转） |

## 3. 实页复验步骤与断言

| 步 | 动作 | 机械断言 | 结果 |
| --- | --- | --- | --- |
| 1 | 打开 `/tags/1974/2026-10-27`（2026-10-27 期日报，board 1974） | 页面渲染三栏阅读台，8 个 section | PASS |
| 2 | 线索行展开 → 点「文章 #156734」 | 文章抽屉打开（标题「惠誉：美国更广泛的物价压力推高通胀持续性风险」） | PASS |
| 3 | 点「站内阅读」 | 站内原文预览弹窗打开，工具栏出现「收藏」按钮（title=`收藏`，未收藏态） | PASS |
| 4 | 点「收藏」 | **星标即时翻转**：按钮 title 变为「取消收藏」（未等接口返回）；`GET /api/articles/156734` → `favorite=true` | PASS |
| 5 | 再点一次 | 按钮 title 回到「收藏」；`GET /api/articles/156734` → `favorite=false`（**还原测试前状态**） | PASS |

- 数据副作用控制：验证用的文章 156734 已还原为未收藏；除该收藏位翻转外未改任何业务数据。
- 未验证项（留账）：失败回滚的**实页**提示（网络拦截模拟）只由单测覆盖（`useArticleFavoriteToggle.test.ts` 的 success:false / reject 两条），本轮未在浏览器里断网复验。

## 5. 弹窗内「全屏」层级（同一反馈批次，第二个缺陷）

**现象**：站内阅读预览弹窗里点「全屏」毫无反应。

**根因**：`ArticleContentView` 的全屏层（`Teleport to="body"`，`.fullscreen-article`）硬编码 `z-[100]`，而弹窗宿主 `ArticlePreviewModal` 的 `AppDialog` 遮罩是 `z-index: 9100` 且 `position: fixed; inset: 0` **整屏覆盖** → 全屏层被压在遮罩之下。实测（修复前，同一页面同一操作）：

| 测项 | 修复前 | 修复后 |
| --- | --- | --- |
| `.fullscreen-article` computed z-index | `100` | `9200` |
| `.app-dialog-overlay` z-index | `9100` | `9100` |
| `elementFromPoint(视口中心)` | `preview-body`（**弹窗内容区**） | `article-tag-list article-tag-list--compact`（**全屏层内**） |
| 全屏层矩形 | 已是整屏 1280×633（但不可见） | 整屏 1280×633 且可见 |

**修法**：层级不再由正文组件硬编码，改由宿主经 `ArticleContentView` 的 `fullscreenZIndex` prop 传入；弹窗宿主传 `9200`（> 遮罩 9100，< 全局 toast 9999，保证全屏中的失败提示可见），阅读页原地全屏保持缺省 `100` 不变。

**实页复验**（同会话 `fix-fs-6d5795ec8cb0`，入口同上）：

| 步 | 动作 | 断言 | 结果 |
| --- | --- | --- | --- |
| 1 | 预览弹窗内点「全屏」 | 全屏层 z=9200 > 遮罩 9100；视口中心命中元素落在 `.fullscreen-article` 内 | PASS |
| 2 | 截图 | 全屏正文整屏铺开、工具栏常驻（退出全屏 / 收藏 / 全屏 / 打开原文），无遮罩发灰/模糊 | PASS（`preview-fullscreen-1280.png`） |
| 3 | 全屏层内点「退出全屏」 | 全屏层卸载，回到弹窗常规视图（中心命中 `article-title-full`，位于 `.app-dialog` 内） | PASS（`preview-after-exit-fullscreen-1280.png`） |

## 6. 自动化验证

| 命令 | 结果 |
| --- | --- |
| `pnpm test:unit app/features/articles/composables/useArticleFavoriteToggle.test.ts app/features/tags/components/daily-report/DailyReportDeskShell.test.ts --maxWorkers=2` | 2 files / 15 tests 全绿 |
| `pnpm test:unit app/features/tags/components/ArticlePreviewModal.test.ts app/features/articles/components/ArticleContentView.test.ts` | 2 files / 18 tests 全绿（层级关系 2 条 + 源码锚 3 条 + 既有 13 条） |
| `pnpm test:unit app/features/articles/composables/useArticleFavoriteToggle.test.ts app/features/tags/components/daily-report/DailyReportDeskShell.test.ts app/features/shell/components/FeedLayoutShell.test.ts app/features/tags/components/console/BoardConsoleShell.test.ts app/features/tags/components/TagsPage.test.ts --maxWorkers=2` | 5 files / 72 tests 全绿（预览宿主零回归；阅读页原地全屏行为未变） |
| `pnpm exec eslint <5 个受影响文件>` | exit 0（0 error） |
| `pnpm exec nuxi typecheck` | 本次改动零类型错误；仅剩 `SignalReportView.test.ts` 7 处**他域既存**类型欠账（`redesign-enrichment-research-workbench` 域，本次未碰） |
| `openspec validate rework-workshop-into-radar-and-desk --strict` | valid |

## 7. 边界与遗留

- 本轮只改收藏链路与全屏层级；预览内其余动作（视图切换/手动抓取/总结/打标签）沿用既有实现，未做行为变更。
- **已知遗留（未改）**：全屏态按 `Esc` 是关闭整个预览弹窗（`AppDialog` 的 document 级 Esc 监听），而不是先退出全屏。改这一条需把全屏态提升到宿主并与弹窗 Esc 分层协调，超出本次缺陷范围，留账。
- 阅读台壳（`DailyReportDeskShell.vue`）仍是**未归档 change** `rework-workshop-into-radar-and-desk` 的在建产物，本次修复随该 change 一同归档。
