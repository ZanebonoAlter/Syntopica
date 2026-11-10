<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- ui-prototype: ui-prototype/index.html -->

## User Journey

读者每日打开 `/tags/:boardId/:date` 的期望不是「读一份报告」，而是「30 秒知道今天发生了什么、哪几条值得点进去」。

现状旅程在第 0 步就断了：进页面看到一屏报头，滚一屏才见到第一个话题，读者要么放弃要么乱滚。

目标旅程：

1. **0–3 秒（首屏）**：看到版块名、日期、期号、阅读时长，以及**头条一条 + 要闻三条**——「今天哪几条值得看」在首屏给完。
2. **3–15 秒（扫读）**：往下滑进入「分版」，按 zone 分组，每个话题一行，行内给出文章数、线索数、匹配质量（可解码的徽章，不是色点），决定要不要展开。
3. **15 秒后（深读）**：展开话题看线索摘要，需要溯源就点文章；需要跨日就把话题拉到泳道趋势或侦探墙。
4. **随时（回看）**：往期导航就近可得，不用回工具条来回点。

AIHOT 的版式提供的是第 1、2 步的节奏；第 3、4 步沿用 Syntopica 已有能力，不复刻它的交互。

## Information Architecture

不新建路由、不新建页面壳，继续用 `DailyReportDeskShell`（`workspace` 模式）与现有三栏。

页面自上而下重排为五段，顺序即优先级：

1. **工具条**（保留，sticky top:0）：较早/较新一期、日期、返回矩阵、主页 —— 现状保留，不自作主张删。
2. **报头带**（压缩）：eyebrow → 版块名 → 日期 · 期号 · N 篇 · N 个话题 · 约 N 分钟；高度上限见 Layout Contract。这是「这是谁的日报、哪一天的」，不是主角。
3. **头版三件套**：
   - **头条**：`highlights[0]` 的 title（大标题）+ reason（推荐理由，2–4 行截断，可完整展开）。它是整页唯一的「大」字号对象。
   - **要闻**：`highlights[1..3]`，编号 N°02–N°04 + 标题 + 一句理由，三栏等宽（窄屏一列）。
   - **空 highlights 降级**：不渲染头条/要闻段，退回现有 lead 摘要（`summary`）+ 直接进分版；并在头版位置留一行「今日无精选，共 N 件事」说明，不伪造内容。
4. **分版区**（现有 zone → topic → section → thread → article，结构与交互全保留）：zone 头（关心的话题 / 其他动态）→ 话题行 → 线索 → 文章。字号整体下调一档，装饰密度下调（见 Component Reuse）。
5. **页尾**：板块动态（`dynamics`，有则显示）+ **往期导航**（近 6 期，日期 + 标题 + 篇数）+ colophon。

三栏归属：
- 左栏（侧栏导航）：本期目录（与正文分区同名）、今天有日报、持续追踪、历史日报、版块切换 —— 命名必须与正文一致。
- 中栏：报头带 + 头版三件套 + 分版区 + 页尾。
- 右栏：页边注（划选批注）—— 现有能力，位置与行为不动。

## Interaction Contract

**只改表达，不加新交互。** 允许的交互变化仅限以下，其余一律沿用现状：

1. 头条 reason 超过 4 行时「展开全文」就地展开/收起（不弹层）。
2. 往期导航是一个列表，每项是 `NuxtLink` 到 `/{boardId}/{date}`，与工具条日期间切换等价、不新增路由。
3. 徽章（tier / anchor / watch）hover 或 focus 时给出文字说明；**必须在不 hover 的情况下也能读懂数值或标签**，不允许继续用无图例色点。
4. 现有交互原样保留且不得回归：话题展开/收起（默认仅第一个展开）、线索展开看文章、划选正文生成页边注、泳道趋势展开、重分桶（hover 出现）、剖面跳转矩阵、窄屏抽屉与 FAB、PeelTransition 翻页、`data-mn-anchor` 可划选区域标记。

**硬约束**：不得改动任何数据请求时机与参数；不得把「读日报」变成「触发生成」；不得引入新接口。

## State Matrix

| 状态 | 展示与恢复 |
| --- | --- |
| 加载中 | 骨架屏沿用现状（`DailyReportReader` 骨架），骨架段落位置对应新的五段，不闪布局 |
| 正常 | 报头带 + 头条 + 要闻 + 分版 + 页尾 |
| `highlights` 为空 | 不渲染头条/要闻，head 区显示「今日无精选，共 N 件事」，直接进分版；不占位留白、不造假数据 |
| `summary` 为空 | lead 摘要整块不渲染，不留空容器 |
| `dynamics` 为空 | 页尾板块动态段不渲染 |
| 只有一个 zone / 一个话题 | 分版区照常渲染，不因数量少而切换版式 |
| 读取失败 | 沿用现有错误态与重试，头版各段不单独报错 |
| 窄屏（<768） | 三栏收成一栏：侧栏与页边注进抽屉/FAB（现状行为），头版三件套变单列，要闻编号保留 |
| 暗色主题 | 同构渲染，仅令牌切换；报头不依赖浅色专属装饰（如 0.5px 文字阴影 `DailyReportMasthead.vue:71-86` 在 dark 已去掉，保持一致）、不要渐变/花纹背景 |
| 旧日报（无 highlights / 旧字段） | 走降级分支，观感接近现状，不因缺字段崩 |

## Layout Contract

- **布局模式不变**：继续用 `workspace`（`DailyReportDeskShell.vue:137`，`mode="workspace" :gutter="0"`），属 `layout.md` 已登记例外（固定视口栅格，L3 阅读台）。**不新建页面 shell、不引入第五种布局模式。**
- **三栏保留**：`14rem | minmax(0,1fr) | clamp(15rem,17vw,17rem)`（`DailyReportReader.vue:1052`）。中栏 1440 下实际约 727px，保持不低于 660px；不得为放内容而横向溢出。
- **报头高度上限**：1440×900 视口下报头带 ≤ 300px（现状约 900px），即首屏必须容纳「报头带 + 头条 + 要闻三段起首」。1920×1080 下同样成立。
- **字号三级制**（取代现状 115px ↔ 0.62rem 的断崖）：
  - 一级：版块名 ≤ `clamp(1.75rem, 3.2vw, 2.75rem)`（≈44px 上限）；
  - 二级：头条标题 `clamp(1.5rem, 2.4vw, 2.1rem)`；要闻标题 ≈1rem；话题名 ≤1.3rem；
  - 三级：正文与线索 0.78–0.9rem，元信息 ≥0.68rem（**不再出现 0.62rem 这种读不清的尺寸**）。
- **sticky 基线统一**：工具条 top:0、侧栏与页边注 top 值必须落在同一套值上（工具条高度 + 0.5rem 为基准），消除现状「侧栏 5.5rem / 页边注 4.25rem」的悬空错位。
- **双视口验收**：1440×900 与 1920×1080 截图，无横向溢出、无内容被 sticky 遮挡；窄屏 390×844 与 375×667 检查可达性（现状窄屏行为不变）。

## Component Reuse

- **复用，不重写**：`DailyReportDeskShell`、`DailyReportReader`、`DailyReportMasthead`（改样式与内部排列）、`DailyReportTopicSection`（改字号/装饰）、`DailyReportSidebar`（改命名来源）、`MarginNotesRail`、`LaneTrendOverview`、`DailyReportMiniLifeline`、`SectionTierBadge`/`SectionAnchorBadge`/`SectionWatchBadge`（**改可解码**）。
- **新增组件上限 2 个**：一个头版编排容器（报头带 + 头条 + 要闻），一个往期导航列表。其余一律复用现有组件。
- **令牌**：只用现有三层体系（`front/app/assets/css/main.css` 的 `:root` 原始色 → `@theme` → `[data-theme]` 语义层），语义名一律 `--color-bg-*` / `--color-text-*` / `--color-border-*` / `--color-accent`。**不新造颜色变量、不硬编码 hex。**
- **圆角与边线**：沿用现有取值集合（999px 胶囊 / 8px / 6px / 5px / 4px / 2px），边线继续用 1px subtle/medium/strong 与 3px double；不引入阴影层级，保持「hairline borders」取向。
- **不改**：`reportPresentation.ts` 与 `dailyReportMagazine.ts` 的既有分区/排序口径；阅读时长只在本层估算是纯函数，不写回后端。

## Prototype

`ui-prototype/index.html` —— **左右并排对比原型，可丢弃，不进产品代码**。

- 左栏「现状复刻」：按现状组件真实样式复刻五段内容（报头 115px 巨标题、两栏 + 首字下沉 lead、今日重点三栏、三枚色点、三栏 sticky 偏移），用于直观感受「一屏都是报头」。
- 右栏「参考版」：本合同的假设实现（报头带 ≤300px、头条 + 要闻、分版降噪、徽章可解码、往期导航）。
- 两栏**同一份数据**：`ui-prototype/data.json`，抓自 `GET /api/daily-reports/1332`（生成式 AI 与大模型厂商 · 2026-11-02 · 31 篇 / 11 节 / 3 条 highlights），静态快照，**原型不请求任何接口、不写库**。
- 1440 宽下两栏各 720px；同时提供 1440×900 / 1920×1080 截图存入 `ui-prototype/`（本地留存，不入 git）。
- 原型明确标注「模拟，非真实阅读时长/性能结论」；原型通过 ≠ 产品任务完成。

**审批口径**：用户看图逐条拍板后，把本条改成最终结论并置 `<!-- ui-approval: approved -->`；若信息架构/主流程/布局模式有改，必须重置为 `pending`。

## Acceptance

- **本轮（原型阶段）**：产出 `ui-prototype/index.html` + 双视口截图；用一条 `ui-prototype/verification.md` 建账，记录每项现状问题在左右两栏的对照结果。原型断言不算产品任务完成。
- **获批后（实现阶段，本文件先不定稿）**：待用户拍板再补 `specs/daily-report-front-page/spec.md` 的 Scenario 与 `tasks.md`，测试映射至少覆盖：highlights 空降级、summary 空不渲染、单 zone 不切版式、窄屏单列、双主题同构、徽章可解码（不 hover 也能读到数值）、往期导航链接与工具条等价。
- 验收口径：主交互链路 agent-browser 机械断言 + 双视口视觉检查 + 与已批准原型差异说明，证据落 `ui-verification/report.md`。
