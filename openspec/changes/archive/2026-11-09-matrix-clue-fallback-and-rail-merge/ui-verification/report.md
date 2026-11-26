# UI 验收报告 — matrix-clue-fallback-and-rail-merge

> 环境：静态托管 `http://127.0.0.1:5100/`（同源），真库 13 个活跃版块、1366 份日报（最新 2026-11-06）。
> 浏览器：`agent-browser` 限核 `taskset -c 2,3` 单会话串行；结束后 chromium 进程归零。
> 验收执行人：主线程（GLM / MiniMax 额度均撞上限，机械断言不需模型推理，未再等待）。

## 真接口实况（`:5100/api/semantic-boards/overview`）

```
partial: false          degraded_board_ids: []
top_stories: 8          条（上限由 5 提到 8）
warming_count: 0        cross_day_story_count: 0
lagging_board_ids: [3122, 246030]   unbackfilled: 0   no_story: 0
stories 段字段（后端口径）：anchor_date, cross_day_story_count, degraded_board_ids,
  heat_rule_version, lagging_board_ids, no_story_board_count, no_story_board_ids,
  partial, top_stories, unbackfilled_board_count, unbackfilled_board_ids, warming_count
  → new_stories / new_count / new_total_count 已随独立卡片一并移除（无死契约）
接口延迟：0.42s（AutoMigrate 冷启动后首请求曾触发 5s 预算 → partial=true，复测稳定后 false）
```

## 验收条目

### A-1 矩阵逐格降级【pass】

依据：接口侧统计 378 个矩阵格子中 **222 个「事件线数 = 0」**，其中 **122 个**「该日有分区」——这 122 个正是本次修复救回的历史格子（改前全部空白）。DOM 侧格子 `aria-label` 实测：

```
"Agent 架构重构与标准化协作生态 · 2026-10-24 · 无事件线（按分区数 1 显示），进入泳道"
"Agent 架构重构与标准化协作生态 · 2026-10-25 · 无事件线（按分区数 1 显示），进入泳道"
"Agent 架构重构与标准化协作生态 · 2026-10-26 · 无事件线（按分区数 1 显示），进入泳道"
```

降级格子明示「无事件线（按分区数 N 显示）」，**不静默冒充事件线数**（红线 30 诚实降级同一纪律）。`matrix_cells` 实测 378，与接口侧统计一致。

### A-2 降级口径与控制台矩阵/折线一致【pass（单测覆盖）】

依据：雷达小矩阵兜底值是**分区数**（`RadarLane.counts[i] = len(day.Sections)`，雷达行不携带逐日线索合计）；控制台矩阵与 `buildPoolSeries` 折线用的是**线索数**（`threadCount ?? fallbackClueCount`），已按同一「事件线 > 0 用事件线，否则线索口径」规则对齐。两处数据源不同导致兜底值量纲不同（分区数 vs 线索合计），该差异已在代码注释、spec 与 API 文档写明。由 `laneConsoleView.test.ts` / `laneDynamicsWindow.test.ts` / `BoardRadarView.test.ts` 覆盖。

### A-3 右栏合并为单清单 8 条【pass】

依据：DOM 实测 `[data-testid^="radar-story-"]` 计数 = **8**（1440 档与 390 档一致）。

### A-4 行内徽标【pass】

依据：`[data-testid^="radar-badge-new-"]` 计数 = **8**，`radar-badge-warm-*` = **0**。当前真库 `cross_day_story_count = 0`（无任何事件线同时具备锚点日前 3 日窗证据与锚点日当天证据），故全部 8 条都是「新起」、无「升温」徽标——与数据一致，非缺陷。

截图 `rail-390.png` 可见每条标题右侧 `新起` 徽标。

> **徽标零区分度与残热旧线标记（review R-7，2026-11-09 补）**：真库 8 条同档 → 徽标不携带区分信息（**已知数据缺口**，等 `enable-story-cross-day-continuation` 造出真实跨天线后区分度自然恢复；设计上**不隐藏徽标**——红线 29 要求分档以行内徽标表达）。残热旧线（`has_today_evidence = false`，锚点日只剩 48h 残热）在条目 meta 行标「今日无报道证据」（`[data-testid^="radar-no-evidence-"]`）——真库当前 Top 8 均带当日证据，该标记**不出现**，故 A-4 的徽标计数不受影响。

### A-5 三张卡片与摘要行均已移除【pass】

依据：`storyMode=true` 分支下旧后端回退块不渲染——组件源码中 `rd-warm-block` 仅存在于 `v-if="!storyMode"` 的旧响应回退分支（`BoardRadarReadingRail.vue:183`），实测 DOM 无该节点；`rd-new-block` / `radar-new-summary` 在组件源码中已不存在（`grep` 零命中），渲染树也无对应节点。原「今天可能升温」「今日新起」两张独立卡片与「另有 N 条」摘要行全部清除，无残留死标题。（review R-8：证据从「选择器查询恒 false」改为「源码位置 + 渲染树」双依据。）

### A-6 跨天延续脚注【pass】

依据：右栏脚注实测两项——「2 个版块数据滞后（各条目按自己的锚点日计）。」与「跨天延续事件线建设中。」。升温空态不再占独立卡片，仅在无跨天线时以脚注一行表达。

### A-7 管线状态折叠为一行【pass】

依据：`[data-testid="radar-status-block"]` 存在；折叠摘要行 `[data-testid="radar-status-summary"]` 的 `aria-expanded` = **"false"**，文本实测「管线正常 · 日报 0/13 · 队列 40」，整块高度 **93px**（改前常驻展开为多行 + 进度条）。异常自动展开由 `BoardRadarStatusBlock.vue` 的折叠状态机实现（`autoCollapsed` 跟随 `abnormal` + `manualCollapsed` 用户手动态；已过生成时刻而当日日报未生成 / 队列活跃量超阈值 → 自动展开，异常消除回折叠），由单测覆盖（review R-3 修订：取数窗口内「过点未出日报」判据粘滞，折叠态**不抖动**；用户手动态在两次真实异常变迁之间粘滞）。

### A-8 滞后/未回填/排序不完整提示保留【pass】

依据：`lagging_board_ids = [3122, 246030]` → 右栏提示「2 个版块数据滞后」在位（**正向实测**）；`partial = false`、`unbackfilled_board_count = 0`、`no_story_board_count = 0` 时不出提示（反向证据，防假阳性）。**未回填与「排序不完整」两条在当前真库无正向样本**（分别为 0 与 false），其正向路径由组件单测覆盖：`BoardRadarView.test.ts` 的未回填/partial 渲染用例（见 tasks 3.3 的映射），本报告不把反向证据记作「三条都实测过」。（review R-8）

### A-9 窄屏 390×844 不破版【pass】

依据：视口 390 档 `document.documentElement.scrollWidth - innerWidth` = **0**；8 条条目**无一条被裁切**（`scrollWidth > clientWidth + 2` 计数 = 0）；8 个徽标完整渲染。截图 `rail-390.png`。

### A-10 1440×900 无溢出、无 NaN/undefined【pass】

依据：1440 档 `scrollWidth - innerWidth` ≤ 2；`document.body.innerText` 不含 `NaN` / `undefined`。截图 `rail-1440.png`。

## 截图

| 文件 | 描述 |
|---|---|
| `rail-390.png` | 390×844：单清单 8 条 + 行内「新起」徽标 + 滞后提示（无独立升温/新起卡片） |
| `rail-1440.png` | 1440×900：同上桌面形态 |

## 单元测试与门禁

```
cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1   → ok 10.186s
cd backend-go && go test ./internal/topicgraph/handler/ -run 'Radar' -count=1      → ok 31.250s
go build ./... && go vet ./... && gofmt -l internal/topicgraph/                    → 通过
golangci-lint run ./internal/topicgraph/...                                        → 0 issues
cd front && pnpm test:unit <5 文件> --maxWorkers=2                                  → 5 passed / 125 tests
pnpm exec nuxi typecheck                                                           → 通过（无 error TS）
pnpm lint                                                                          → 0 errors（8 warnings 均存量）
openspec validate --strict                                                         → valid
scenario-trace                                                                     → 53/53 映射齐全
check-tasks-tail                                                                   → 尾三节 + doc-impact 声明齐全
```

## 观察（非缺陷，记录备查）

徽标的区分能力当前**受限于数据**：`cross_day_story_count = 0` 使所有事件线都落「新起」，8 条全带同一徽标。等 `enable-story-cross-day-continuation` 造出真实跨天线后，「升温 N×」徽标才会出现、徽标才产生区分度。这是既有数据缺口的如实呈现，不是本 change 的显示问题。

---

## 归档后补记 — 矩阵降级格色阶修复（2026-11-09 缺陷修复，用户报「矩阵格子的颜色没修复」）

> 归档后补证据（允许）。本节记录**缺陷修复**的验收，不改本 change 的 spec 正文。
> 根因：本 change 的 ui-design 决策「降级格与正常格同着档逻辑（值换成分区数后照常走
> `laneLevel(count, laneMax)`）、不新增降级专属色」——事件线数与分区数是**两种量纲**，
> 共用一条色阶导致 ① 代理值与已确证值同色同档、② 代理值稀释已确证格对比度、
> ③ 行内事件线稀疏时代理格独占最深实心档（零事件线的一天被画成最热的一天）。
> 修复：已确证格走 `.b1..b4`（基准 = 行内事件线数最大值），降级格走**独立色阶**
> `.d1..d4`（基准 = 行内分区数最大值，色强封顶在 `.b3` 之下）+ `.is-degraded` 斜纹形态。

### 证据来源说明

静态站 `:5100` 在本次修复时尚未重建（主线程统一构建部署），故本次证据取自
**同一份真库数据的 dev server**（`http://127.0.0.1:3000/tags?z=<纳秒戳>`，
`NUXT_PUBLIC_API_BASE=http://127.0.0.1:5100/api`）；修复前后的对照数据取自
`:5100` 旧构建（同样的 378 格 / 27 泳道 / 13 版块、同一批真实数据）。

### B-1 降级格色阶由兜底值决定（不是事件线数）【pass】

依据：泳道「AGENT编程工具相关」（行内事件线峰值 14、代理值峰值 6）实测 class 与
computed background-color：

| title | 改前 class | 改前 bg | 改后 class | 改后 bg |
|---|---|---|---|---|
| 无事件线（按分区数 1 显示） | `b1` | `srgb(0.946745 0.792235 0.777725)` | `is-degraded d1` | `srgb(0.972627 0.927922 0.90949)` |
| 无事件线（按分区数 2 显示） | `b1` | `srgb(0.946745 0.792235 0.777725)` | `is-degraded d2` | `srgb(0.957098 0.84651 0.830431)` |
| 无事件线（按分区数 3 显示） | `b1` | `srgb(0.946745 0.792235 0.777725)` | `is-degraded d2` | `srgb(0.957098 0.84651 0.830431)` |
| 无事件线（按分区数 4 显示） | `b2` | `srgb(0.919569 0.649765 0.639373)` | `is-degraded d3` | `srgb(0.933804 0.724392 0.711843)` |
| 无事件线（按分区数 6 显示） | `b2` | `srgb(0.919569 0.649765 0.639373)` | `is-degraded d4` | `srgb(0.905333 0.575137 0.566902)` |
| 4 条事件线 | `b2` | `srgb(0.919569 0.649765 0.639373)` | `b2` | `srgb(0.919569 0.649765 0.639373)`（不变） |
| 14 条事件线 | `b4` | `rgb(217, 74, 74)` | `b4` | `rgb(217, 74, 74)`（不变） |

改前三个不同兜底值（1/2/3）落到**同一色** `b1`（被行内事件线峰值 14 压平），
改后随兜底值分档（d1/d2/d3/d4，4 个互不相同的 computed color）。

### B-2 降级格与事件线格视觉可区分【pass】

依据：降级格 `class` 恒含 `is-degraded`（+ `background-image: repeating-linear-gradient(45deg, …)`
斜纹），事件线格恒为 `b*` 且 `background-image: none`；全量 378 格统计——
**降级格与已确证格的 computed background-color 相同对数 = 0**（两种量纲不再同色）。
改前同例：分区数 3 的降级格与 4 条事件线格 color 完全相同（`srgb(0.919569 …)`）。

### B-3 降级格不再冒充行内峰值【pass】

依据：泳道「俄日领土主权纷争」唯一的非空格（无事件线，按分区数 1 显示）改前
`class="rd-mini-cell b4"`、`bg = rgb(217, 74, 74)`（**满档实心红 = 全表最热**）；
改后 `class="rd-mini-cell is-degraded d4"`、`bg = srgb(0.905333 0.575137 0.566902)`
+ 斜纹（代理中档，非峰值实心）。全量统计：降级格最深只到 `d4`（58% 色强），
与已确证 `b4`（100% 满档实心）不相等。

### B-4 已确证格颜色零变化（对比度不被代理值稀释）【pass】

依据：378 格逐格对照旧构建 —— **class 变化 122 格 = 恰为全部降级格**，
**已确证格（含「0 条事件线」真断档与旧后端 sections 口径格）class 变化 = 0**；
title/aria 文案 378 格逐字未变。

### B-5 双视口不破版【pass】

依据：1440×900 视口 `scrollWidth - innerWidth` ≤ 2，页面文本无 `NaN` / `undefined`；
截图 `matrix-color-1440.png`（小矩阵可见，实心格=已确证事件线、斜纹格=分区数兜底）。
390×844 视口 `scrollWidth - innerWidth` = 0，页面文本无 `NaN` / `undefined`；
截图 `matrix-color-390.png`（**小矩阵在 <1440px 按既有断点整体隐藏**，
本档证据 = 窄屏不受影响、不破版）。

## 截图（归档后补记）

| 文件 | 描述 |
|---|---|
| `matrix-color-1440.png` | 1440×900：已确证格 = 实心红四档；降级格 = 斜纹 + 独立 d 档（d1 淡 → d4 中浓，恒不占满档） |
| `matrix-color-390.png` | 390×844：窄屏形态（小矩阵按既有断点隐藏，布局不破版） |
