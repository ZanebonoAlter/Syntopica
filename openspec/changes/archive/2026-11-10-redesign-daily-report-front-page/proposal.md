<!-- complexity: medium -->
<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- constraint-domains: daily-report, reading -->
<!-- review: waived -->

## 归档说明（放弃实现）

2026-11-10：用户看过 ui-prototype 对比原型后决定不实施本 change，直接归档。实际产出仅 proposal + ui-design + 原型对比材料，未拆 specs/design/tasks，未改任何 `front/` 代码，无部署影响。归档走豁免通道（`--force` 留痕；review 同步豁免——无实现即无评审对象）。

## Why

日报阅读页（`/tags/:boardId/:date`）现在的问题不是「不好看」，是**扫读跑不通**。现状解剖（子线程只读核查，证据带行号）：

1. **首屏被报头吃光**。报头独占约 900px（标题 `clamp(2.5rem,8vw,8rem)` 在 1440 宽下约 115px、`DailyReportMasthead.vue:74`；highlights 上外距 4rem、`:174`），而 1440×900 扣掉 56px 工具条后可视区只有 844px —— 进页面一屏，**0 个章节、0 个话题**，全是报头。
2. **字号断崖 180:1**。报头 115px/900 与正文最大 1.55rem（`DailyReportTopicSection.vue:637`）、最小 0.62rem（`:652`）同屏，缺「今天到底有哪几件事」这一中间层。
3. **三枚无语义色点**。tier / anchor 用 0.5rem、0.4rem 纯色圆点表达匹配质量与锚定强度（`SectionTierBadge.vue:52-61`、`SectionAnchorBadge.vue:68-80`），无图例无数字，含义藏在 hover 浮层里。
4. **侧栏与正文两套命名**。「本期目录」只有「头条/话题正文」两条（`DailyReportSidebar.vue:42-46`），正文分区实际叫「关心的话题/其他动态」（`dailyReportMagazine.ts:98-101`）；侧栏用 `canonicalLabel`、正文用当天 `label`（`dailyReportMagazine.ts:135`），同一对象两个名字。

参考对象是 AIHOT 的日报版式（`apps/web/app/features/report/ReportPaper.tsx`，调研存档 `docs/research/aihot-reference/explore-findings.md`）：**报头克制（报头+期号+日期+阅读时长）→ 头条一条 → 要闻三条 → 分版展开 → 往期导航**。它把「今天哪几条值得看」放在第一屏，把细节留给往下滚。

## What Changes

**范围：只改阅读页的表达层（信息层级 + 版面节奏 + 字段显隐）。不动数据模型、不动生成链路、不加后端字段、不改路由。**

- **报头压缩**：保留品牌感（eyebrow「Syntopica · 每日脉络」+ 版块名 + 日期 + 元信息），但标题字号从 `clamp(2.5rem,8vw,8rem)` 收到有上限的尺度，目标报头占首屏 ≤ 1/3；期号 / 阅读时长按 AIHOT 做法补进 meta 行。
- **新增「头条 + 要闻」两层**，数据全部来自现有 `highlights`（`title` + `reason`，当前已展示但埋在报头底部）：
  - 头条 = `highlights[0]`，大标题 + 推荐理由；
  - 要闻 = `highlights[1..3]`，编号三栏；
  - 无 highlights（如 1303 那种空数组）降级为当前 lead 摘要，不伪造内容；
  - 阅读时长按 `article_count` 与线程字数估算，标注「估算」口径，不当真实阅读数据。
- **分版区**：保留现有 zone → topic → section → thread → article 结构与全部既有交互（折叠、展开、划选批注、泳道趋势、重分桶），只下调字号与装饰密度，让「分版」成为第三层，不再跟报头争视线。
- **徽章可解码**：tier / anchor 的纯色圆点改为带文字与数字的徽章，或给一条图例；不允许继续用无图例的色点表达语义。
- **往期导航**：把工具条的「较早/较新一期」升级为报头侧或页尾的往期列表（复用 `GET /semantic-boards/:id/daily-reports`，**无新接口**）。
- **侧栏命名对齐**：侧栏目录项与正文分区用同一套名字。
- **不做什么**：不改 `DailyReport`/`Section`/`Thread` 的字段结构；不改 `DailyReportReader` 的「页面不滚、内容滚」工作区壳（`workspace` 布局模式保持不变）；不动生成 prompt、分桶、锚定、话题归属任何逻辑。

## 本轮交付：只出对比原型，不做实现

第一轮只产 `ui-prototype/index.html`：**左右并排**——左栏「现状复刻」、右栏「参考版」，同一份真实数据（`GET /api/daily-reports/1332`，生成式 AI 与大模型厂商 · 2026-11-02 · 31 篇 / 11 节 / 3 条 highlights，快照落 `ui-prototype/data.json`），同一视口各 720px。附 1440×900 与 1920×1080 截图。

用户看着图拍板「哪几条要、哪几条不要」，`ui-approval` 才从 `pending` 改 `approved`，之后再拆 `specs/` + `design.md` + `tasks.md` 进实现。**原型不通、不批准，就不动 `front/` 一行代码。**

## Capabilities

### New Capabilities

- `daily-report-front-page`：日报阅读页的头版层级合同——报头尺寸上限、头条/要闻/分版三层的信息架构、徽章可解码、往期导航、空 highlights 降级。只管表达层，不接管生成与数据口径。

### Modified Capabilities

- 无。既有 `daily-report` spec 的数据口径、生成与锚定要求全部不动；本 change 只在其上叠一层阅读页表达契约。

## Impact

- **代码**：`front/app/features/tags/components/daily-report/**`（`DailyReportMasthead.vue`、`DailyReportTopicSection.vue`、`DailyReportSidebar.vue`、`DailyReportReader.vue` 的样式与排列）、新增头版编排组件；`reportPresentation.ts` / `dailyReportMagazine.ts` 可能加纯函数（阅读时长估算、highlights 分层）。**本轮原型阶段零代码改动。**
- **接口**：无新增、无字段变更，只用现有 `GET /daily-reports/:id` 与 `GET /semantic-boards/:id/daily-reports`。
- **数据**：无迁移。
- **主题**：editorial / dark 双主题都要成立，语义令牌照现有三层体系（`front/app/assets/css/main.css`），不新造一套。
- **用户可见变化（实现后）**：进日报页第一屏不再只见报头，而是「报头 + 头条 + 要闻」；往下才是分版正文。旧数据的 highlights 为空的日报会走降级分支，观感与现在接近。
