# test-cases: daily-report-lead-direct-preview

测试单元 = 一个 Requirement 的用户故事：**「用户在日报阅读台点击线索引用文章，不经过抽屉、一步进入站内阅读预览弹窗」**（delta ADDED Requirement「阅读台线索文章直达站内阅读预览」+ MODIFIED「日报时间线组件」thread 文章句）。simple 档，组件层（Vitest）为主链路落点，agent-browser 承担端到端主链路验收。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 线索原位展开相关文章标题列表（既有行为，不改） | 展开 thread 文章并复用缓存（前半） | 标题可见、缓存复用不回归 | 组件 | 既有 `DailyReportTopicSection.test.ts` / `dailyReportMagazine.test.ts`（不修改，回归保障） |
| 2 | 点击文章标题 | 点击线索文章直达预览弹窗 | shell 收 `openArticle(id)` → `getArticle` → 弹窗直接打开；无 `DeskArticleDrawer` 渲染 | 组件 | `DailyReportDeskShell.test.ts`（改写） |
| 3 | 详情请求进行中 | 详情加载中呈现加载态 | `loading-preview-article=true` 传入弹窗，返回后替换为文章 | 组件 | `DailyReportDeskShell.test.ts`（新增） |
| 4 | 详情请求失败 | 详情加载失败可见反馈 | `notify.warn` 被调、弹窗不开（不留空白弹窗） | 组件 | `DailyReportDeskShell.test.ts`（新增） |
| 5 | 预览内收藏（继承契约） | 站内阅读预览收藏即时生效 / 收藏失败回滚并提示 / 连点不丢更新 / 预览内文章状态更新回流 | 乐观翻转→落库；失败回滚+提示；回流应用 | 组件 | `DailyReportDeskShell.test.ts`（继承保留，入口改直达） |
| 6 | 端到端主链路 | 主链路 2-4 整串 | 点标题 → elementFromPoint 命中弹窗、无 `desk-article-drawer` 元素 → Esc 关闭 | agent-browser | `ui-verification/report.md`（静态 :5100） |

## 继承与调整表（⓪ 契约变更：MODIFIED/REMOVED daily-report-system）

| 旧 Scenario/断言 | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 抽屉打开 →「站内阅读」→ 预览（`openPreviewWith` 经 drawerStub emit） | **改** | `DailyReportDeskShell.test.ts:openPreviewWith` | 打开方式改为 readerStub emit `openArticle(158497)`，shell 直达；`~/api/articles` mock 补 `getArticle` 返回原文章对象，断言数据不变 |
| 站内阅读预览收藏即时生效 | 继承 | 「点收藏：立即翻转并落库」 | 保留，仅入口换直达 |
| 收藏失败回滚并提示 | 继承 | 「收藏失败：回滚到点击前状态并提示」 | 保留 |
| 连点不丢更新 | 继承 | （由乐观更新实现覆盖，旧文件无独立用例） | 维持现状，不补新用例 |
| 预览内状态更新回流 | 继承 | 「预览内文章状态更新回流」 | 保留 |
| 阅读台文章抽屉与站内阅读预览的状态同步（需求本体） | **删** | drawerStub / mountShell stubs | stub 删除；组件文件 `DeskArticleDrawer.vue` 删除 |
| `[boardId]/[date].test.ts` | 无涉 | 全程 stub DailyReportDeskShell，无抽屉耦合 | 不改，跑一遍防回归 |

## 变体走查

- 输入组：**不适用**——入参是数字 article id（组件 emit），无文本输入面（划除：空串/纯空白/分隔符/大小写/特殊字符/超长）。
- 前置组：单元素（不存在的文章 id → `getArticle` 返回失败 → 步 4 错误反馈覆盖）；越界引用（线索存了已删文章 id → 同上，接口 404 走失败分支）；重复/部分满足 → 同前。
- 时间窗口组：**不适用**（无时间语义，划除）。
- 幂等组：重复执行（同一文章点两次 → 重新拉详情重开弹窗，结果一致，可接受）；部分失败重试（加载失败后再点标题 = 重试，走步 4 同一分支）；并发 → **不适用**（单用户前端，无线程安全声明，划除）。
- 可用性组（UI 必检前三）：误输入反馈 → **不适用**（无输入框，划除）；空态 → **不适用**（详情接口非空即有内容，空正文由 ArticleContentView 既有回退链承载，划除）；错误态 → 步 4；加载态 → 步 3；超长文本 → 弹窗内滚动为 ArticleContentView 既有契约（不新增，回归保障划除）；重复提交 → 幂等组已走查。

## 效果核对

**不适用**：本 change 为纯前端交互重接线，无数据覆盖率/LLM 行为依赖；组件断言即效果断言（弹窗开/关、通知调用、接口参数），无需真库量化核对。

## 验收措辞对账

每步落点给出可执行验证：组件层 `cd front && pnpm test:unit DailyReportDeskShell.test.ts --maxWorkers=2`（全绿）；端到端 agent-browser 断言结果记 `ui-verification/report.md`；删除核验 `grep -rn "DeskArticleDrawer" front/app` 零命中。
