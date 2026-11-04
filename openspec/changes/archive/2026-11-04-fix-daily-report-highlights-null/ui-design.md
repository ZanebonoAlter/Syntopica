<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# ui-design: fix-daily-report-highlights-null

> minor 档轻量契约（入口 / 状态 / 复用 / 验收）。本次**不新增页面、不改布局模式、不改弹窗档位**——只在既有刊头结构里补一个**空态占位**，因此不需要原型与人工审批（`ui-approval: not-required`）。

## 入口

- **入口位置不变**：日报阅读台（`/tags/:boardId/:date`）→ 刊头组件 `front/app/features/tags/components/daily-report/DailyReportMasthead.vue`。
- **触发条件不变**：「今日重点」区块仍是刊头的固定组成部分，**没有**新增按钮、菜单、tab 或跳转入口。
- **本次唯一的新交互面**：当 `report.highlights` 为空（`null` 或 `[]`）时，刊头在原「今日重点」位置渲染一个**静态占位文本**（无按钮、无可点元素、无二级操作）。
- **不进不进目录/页边注/批注锚**：占位块不参与划选批注、不进左目录、不产生 `data-mn-anchor`。

## 状态

| 状态 | 触发条件 | 刊头表现 | 交互 |
| --- | --- | --- | --- |
| 正常 | `highlights.length >= 1` | 三栏网格（`#report-highlights`），最多 3 条，每条「N°01 序号 + 标题 + 理由」 | 静态展示（与现状完全一致，**零视觉变化**） |
| 空态 | `highlights` 为 `[]` 或 `null` | 单条占位文本「本期暂无重点」，沿用刊头边框语言（上下双线 + 居中弱化文字），**占据原三栏区域的位置** | 静态展示，无可点元素 |
| 加载中 | 报告未返回 | 刊头整体不渲染（现状不变，不引入新骨架屏） | — |
| 错误 | 报告请求失败 | 由既有阅读台错误态处理（现状不变） | — |

**关键约束**：空态与正常态**互斥**（`v-if` / `v-else`），任一时刻刊头只有一个分支存在 → `id="report-highlights"` 不会重复。

## 复用

- **结构复用**：沿用 `DailyReportMasthead.vue` 既有 `.drm-highlights` / `.drm-highlight` 类名与边框语言（`border-top: 1px` + `border-bottom: 4px double`），空态只加一个 `--empty` 变体修饰类，**不引入新组件文件**。
- **文案家族复用**：「本期」= 当期报纸范围（对齐既有「本期分区 / 本期目录 / 本期话题」）；「暂无 X」= 空态陈述（对齐既有「暂无日报节点 / 暂无归档摘要 / 暂无事件脉络 / 暂无批注」）。最终文案：**本期暂无重点**。
- **可测性复用**：空态元素带 `data-testid="highlights-empty"`（对齐既有 `trend-empty` / `trend-pending` / `trend-events-empty` 命名法），供组件测试断言。
- **布局契约**：沿用 reader 页既有四模式中的**当前 reader 模式**，宽度/分栏/断点全部不变；空态块在 ≤720px 断点下随三栏网格降为单列（与正常态同断点）。
- **主题复用**：仅用既有 CSS 变量（`--color-text-muted` / `--color-border-medium` / `--color-border-strong` / `--color-accent`），不引入新色值、不改深色模式分支。

## 验收

- **组件层（自动化）**：`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`（本次新建）断言三条：
  1. `highlights` 3 条 → 渲染 3 条 `.drm-highlight`，**不出现** `[data-testid="highlights-empty"]`；
  2. `highlights` 为 `[]` → 渲染 `[data-testid="highlights-empty"]`，文案含「本期暂无重点」，**不出现** `.drm-highlight`；
  3. `highlights` 为 `null` → 同 2（**不抛错**，前端契约层兜底）。
- **回归（自动化）**：`front/app/features/tags/components/DailyReportReader.test.ts` 已有用例「刊头保留『今日重点』top3 高亮（不再隐藏）」→ 照跑，断言不变。
- **视觉（人工留痕）**：静态发布路径（`NUXT_PUBLIC_API_BASE=/api pnpm generate` → 铺 `backend-go/frontend/` → `:5100` 同源）打开一篇**坏日报**（当前为 board 1980 / 2026-11-03，`highlights` 为 null）与一篇**正常日报**，确认：
  - 坏日报刊头出现「本期暂无重点」占位，**占位不与头条/刊头其它内容重叠**（量矩形不相交）；
  - 正常日报 3 条要闻渲染**与改动前像素级一致**；
  - 窄视口（≤720px）下占位单列显示、不溢出。
- **机械断言**：不依赖目测——组件测试用 `data-testid` 判定存在性；重叠以 DOM 矩形相交检测为准。
