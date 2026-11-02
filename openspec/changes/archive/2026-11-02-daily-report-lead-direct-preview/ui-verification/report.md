# UI 验收报告 — daily-report-lead-direct-preview

> 变更：日报阅读台点击线索引用文章**直达**站内阅读预览弹窗（`ArticlePreviewModal`），移除中间文章抽屉（`DeskArticleDrawer`）层级。
> 验收方式：agent-browser 实页（真实后端 + 真实数据库）+ Vitest 组件测试；ui-impact: minor（复用既有弹窗契约），验收范围为主链路 + Esc 分层 + 回归核对。
> 日期：2026-10-30　环境：树莓派 arm64 / Debian 13，同源静态托管 `http://localhost:5100`，版块 3030「开发者工具与开源生态」2026-10-30（26 条带引用线索）。

## 一、验收结论

**pass**（本 change 交互面全项通过；含一处实现中发现并当场修复的 Esc 分层回归，见 A-4）。

证据文件（同目录）：

| 文件 | 视口 / 主题 | 内容 |
| --- | --- | --- |
| `lead-preview-1440-editorial.png` | 1440×900 / editorial | 点线索文章标题 → 预览弹窗直达打开，正文完整呈现 |
| `lead-preview-esc-reader-kept.png` | 1440×900 / editorial | Esc 关弹窗后阅读台保留（URL 仍为 `/tags/3030/2026-10-30`） |
| `lead-preview-1440-dark.png` | 1440×900 / dark | 暗色主题下弹窗可读（token 派生底色） |

## 二、判定条目

### A-1 点线索文章直达预览弹窗、无中间抽屉【pass】
实页链路（线索行展开 → 点文章标题）：`.preview-header-text` 出现（"文章预览"）、`elementFromPoint(innerWidth/2, innerHeight/2)` 命中 `.article-title-full`（弹窗正文内文章标题）、URL 保持 `/tags/3030/2026-10-30`；`document.querySelector('[data-testid="desk-article-drawer"]')` 为 null（抽屉已随组件删除，DOM 无该层级）。弹窗内标题 = 点击的文章标题「同一个需求，加上这个 Skill，网页到底差多少？」。

### A-2 详情加载态与失败反馈【pass】
组件测试（`DailyReportDeskShell.test.ts`）：加载中 `visible=true` + `loadingPreviewArticle=true` + 文章为 null（弹窗 header 呈现「正在准备文章预览...」，与 search 页同款通道）；接口失败 `visible=false` + `notify.warn('文章加载失败，请稍后重试')`——不留空白弹窗。

### A-3 直达性组件锚【pass】
`DailyReportDeskShell.test.ts`「点线索文章：直达预览弹窗」：reader emit `openArticle(id)` 直接使弹窗可见——旧链路必须经抽屉 emit `open-article` 才会开，该断言即为「无抽屉中转」的机械锚。`grep -rn "DeskArticleDrawer" front/app` 零命中。

### A-4 Esc 分层：关弹窗保留阅读台【pass，含当场修复】
**修复前实测（回归）**：弹窗开着按 Esc → 弹窗关闭且 URL 退到 `/tags/3030`（阅读台整个退出）。根因：阅读台在 document 级监听 Esc 并查 `overlayStack` 计数让位（`DailyReportReader.handleKeydown` → `hasOpenOverlay()`）；旧链路的 `AppDrawer` 打开时会 `pushOverlay()`，而直达链路唯一浮层 `AppDialog` 不进栈——Reader 查栈得 false，同一次击键连阅读台一起退，违背 delta「保留阅读台上下文」。
**修复**：`ArticlePreviewModal` 自登浮层栈（watch visible → `pushOverlay()`/`popOverlay()` + onBeforeUnmount 兜底，模式照抄 `AppDrawer`；不动共享组件 `AppDialog`）。
**修复后实测**：第一击 Esc → `dialogGone=true`、`readerStill=true`、URL 仍为 `/tags/3030/2026-10-30`（`lead-preview-esc-reader-kept.png`）；第二击 Esc → URL 退到 `/tags/3030`（阅读台既有 Esc 行为未被堵死，分层完整）。对无 Esc 监听的宿主（矩阵页/搜索页）计数增减无副作用（`useTagsPage`/`search` 无 overlayStack 消费）。

### A-5 收藏/状态同步契约回归【pass】
`DailyReportDeskShell.test.ts` 继承断言全绿：点收藏乐观翻转并 `PUT /api/articles/:id`、失败回滚 + `notifyError`、预览内状态更新回流（daily-report flow 约束 32 契约不变）。ArticlePreviewModal 组件测试 6 项、DeskShell 11 项、`pages/tags` 11 项，合计 28 passed。

### A-6 暗色主题可读【pass】
`lead-preview-1440-dark.png`：`data-theme=dark` 下弹窗底色/文字由 semantic token 派生，标题正文可读，无硬编码色。

## 三、未覆盖与理由

- 完整 8 路由矩阵 / 产品任务走查：本 change 为 minor 交互重接线（复用既有弹窗，页面壳/共享导航未动），按 ui-design 验收映射以主链路 + 组件回归为界；`ArticlePreviewModal` 的改动（overlayStack 登记）被 A-4 的宿主分析覆盖（矩阵页/搜索页无 overlayStack 消费者，行为零变化），未另跑全系统路由矩阵。
- 窄屏（375px）：直达弹窗行为与视口无关（AppDialog 92vw 上限既有契约），未单独截图。
- 「打开原文」外链：`ArticleContentView` 工具栏既有能力，本 change 未触碰（组件测试回归保障）。
