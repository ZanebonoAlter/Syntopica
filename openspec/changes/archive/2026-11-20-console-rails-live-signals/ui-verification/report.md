# UI 验收报告 · console-rails-live-signals

> 验收对象：`/tags/1974`（中东地缘政治与美伊关系）真实数据，静态托管 `http://localhost:5100`。
> 验收时间：2026-11-19（本地服务，日报锚点日 11-19）。
> 截图证据：本院 `1440x900.png` / `1920x1080.png`（构建产物，非 dev server）。
> 查证方式：agent-browser `eval` 断言 + 直连 `GET /api/semantic-boards/1974/lane-dynamics?days=14` 对账。

## 数据对账基线（口径正确性的锚）

后端 `lane-dynamics`（anchor = 2026-11-19）逐泳道当日线索：

| topic_id | 泳道 | 11-18 | 11-19 |
| --- | --- | --- | --- |
| 1204 | 美伊博弈升级：从经济威胁到全面孤立 | 0 | **5** |
| 5 | 霍尔木兹海峡封锁解除与航运恢复 | 0 | **3** |
| 1844 | 沙特与也门胡塞武装冲突 | 4 | **2** |
| 1205 | 美伊形式对市场影响（watch 关联，14 天分区 2 → 休眠折叠） | — | **1** |
| 2 | 以黎冲突升级：停火破裂与军事打击 | 2 | — |
| 9 | 协议落地引发能源市场剧烈震荡 | — | — |

---

### A-1 左轨每行 = 名称 + 14 格条码，无数字无口径说明 — 【pass】

- 依据：`[data-testid="board-rail"]` 内 13 行，每行 `.rail-cell` 数量集合 = `[14]`（无一行多/少）。
- 依据：左轨 `innerText` = 「语义板块 13 + 13 个版块名」，**不含任何数字**（正则 `\d{3,}` 不命中），**不含**「每格 / 数字 = / 线索数」类说明文字。
- 依据：`tag_count` 只在行 `title`（`railRowTitle` 组装「标签挂载 N · 数据截至 D · 逐日明细」），行内不占位。
- 依据（宽度）：调窄条码后 13 行 `scrollWidth === clientWidth`，**无一个名称被截断**（列宽 144px，「中东地缘政治与美伊关系」完整）。

### A-2 未取到 / 停更 / 安静 三态可区分 — 【pass】

- 依据（未取到 → 整行斜纹）：`日本新闻` 14 格全部 `is-missing`（`data_unavailable` / 无 `clue_days`）。
- 依据（停更 → 尾段斜纹）：`刚果疫情新闻` = `-vXXXXXXXXXXXX`（第 2 格有值、后 12 格 `is-missing`），其锚点日落后全局锚点 12 天。
- 依据（安静 → 空槽）：`卫生健康` 14 格全部非 `is-missing` 且非 `is-val`（真实 0，空槽），与斜纹视觉不同。
- 依据（共同轴重对齐）：`台海新闻`/`游戏版块` 等尾部出现 `is-missing`，说明 offset 平移生效（后端按各自锚点给轴，前端对齐到全局轴）。

### A-3 右轨顺序 = 体检 → 当日日报 → 追踪 → 候选 → 来源；体检不显示评语 — 【pass】

- 依据：右轨块 `data-testid` 顺序实测 = `rail-block-health → rail-now → rail-block-watch → rail-block-candidates → rail-block-sources`。
- 依据：体检行 `innerText` = 「版块体检 88 上次体检 11/19 21:06 刷新 评语↗」——一行，**无评语首句**，评语入口在 `评语↗`（进详情）。
- 依据：「最新快照 · 非所选历史评分」标注不在右轨（沿用 `simplify-workshop-matrix-copy` 已定口径，本 change 的 spec delta 已同步改正该条）。

### A-4 当日日报两层 + 线索数与矩阵同值 — 【pass】

- 依据（点评层）：3 条，标题 = 「特朗普考虑对伊朗实施新袭击 / 美国国安局IT系统故障 / 国际能源署评估伊朗战争影响」，各带 `reason` 与「6 篇来源 / 1 篇来源 / 1 篇来源」。
- 依据（不重复报标题）：块内**不出现** `report.title` / `report.summary` 单独渲染（`nowBrief` 模型不含这两个字段，组件断言无该节点）。
- 依据（泳道层）：4 条，每条 = 色点 + 泳道名 + 线索数 + 一句代表线索。
- 依据（数值同源同值，**关键**）：右栏 = `5 / 3 / 2 / 1 条`，与上文 `lane-dynamics` 11-19 值逐条一致；与矩阵 11-19 列（5 / 3 / 2 / —）一致。
- 依据（不显示分区 thread 数）：右栏未出现含综合稿口径的数字，源码只走 `dayCellMetrics(day).clueCount`（与矩阵日格同一函数）。

### A-5 日期唯一：矩阵换日 → 右栏跟随；右轨无日期控件 — 【pass】

- 依据：点矩阵列头 `1118` 后，右栏块标题 `rail-now-date` 变为 `· 11-18`，点评与泳道内容随之切换（11-18 的 3 条点评 + 4 条泳道）。
- 依据：`[data-testid="rail-now"]` 内 `input / select / [type=date]` 数量 = **0**。
- 依据：刷新后默认落在锚点日 11-19（`selectedDate` 既有回落逻辑），右栏自动显示 11-19。

### A-6 矩阵选中列整列淡底，且只铺有分区的格 — 【pass】

- 依据：11-19 列 `is-col-selected` 格 = **3 个**，与「该列有内容（5/3/2）」的格数相同；`—` 无分区格 **0 个**被铺底（`dashSelected = 0`），即无分区历史日不会被灰带盖住。
- 依据：淡底走 `::after` 叠层（`color-mix(accent 7%)`），格子自身 `background` 未改（行内色阶仍生效）；列头 `is-selected` 高亮保持原样。

### A-7 动效克制 + 只有选中行脉动 — 【pass】

- 依据：`.rail-cell.is-now` 数量 = **1**（13 行中只有当前选中版块的当日格脉动），非 13 行同闪。
- 依据：入场逐列 `rail-cell-in` 240ms + `--i × 12ms`（13 格 ≈ 396ms ≤ 420ms 预算）；选中行左缘线 `scaleY` 160ms。
- 依据：`prefers-reduced-motion: reduce` 下 `animation: none`（样式表内既有媒体查询覆盖 `.rail-cell` 与 `.rail-cell.is-now`）。
- 依据：无 `transition: all`（全仓 grep 该组件无命中）。

### A-8 两栏满高自适应 — 【pass】

- 依据（1440×900）：视口 900，左轨 `height = 844`（= 900 − 顶栏 56），右轨 `height = 804`；两栏均**不随内容收缩**。
- 依据（右轨分布）：「当日日报」块父容器 `flex-grow = 1`（撑开剩余高度），其余四块固定尺寸贴底；`right.scrollHeight > clientHeight` = true（内容超出时栏内滚动，非整页滚动）。
- 依据（1920×1080）：截图 `1920x1080.png`，两栏同样满高，空白落在「当日日报」块内。
- 依据（左轨行高）：13 行在 844px 高度下 `flex: 1 1 auto` 撑开且有 `max-height: 40px` 上限；行间有极淡分隔线。

### A-9 泳道层按线索数降序 — 【pass】

- 依据：右栏泳道线索数序列 = `[5, 3, 2, 1]`，严格降序；与 `lane-dynamics` 的 `lane_heat` 序（5 / 3 / 2 / 1）一致。
- 依据（并列原序兜底）：`nowBrief.ts` 的 `compareLanes` 显式带 `order` 次键（不依赖引擎排序稳定性），`null` 排末位。
- 单测覆盖：`nowBrief.test.ts` 断言 `1/4/2/0 → 4/2/1/0`。

---

## 偏差与说明（如实记录）

1. **「右栏每条在矩阵有对应格」的表述过强**。实测第 4 条 `美伊形式对市场影响`（topic 1205）是 `watch_linked=true` 且 14 天仅 2 个分区 → 矩阵判为**休眠**并折叠在「休眠泳道 · 1」里，首屏看不到该行，但右栏显示了它（当日 1 条）。
   - 行为本身正确（它是真实追踪泳道且有当日线索，数值与 `lane-dynamics` 一致）；**不修代码**。
   - 已在 design D5 / ui-design Data Provenance 把「保证矩阵有对应格」修正为「在 `lane-dynamics` 名单内（含休眠折叠与 watch 关联泳道）」，避免契约说谎。
2. **体检「最新快照」标注**：原 delta spec 要求右轨常驻该标注，与既有 `simplify-workshop-matrix-copy` 已定口径（该口径移到详情、右轨不重复）冲突。已按后者实现，并**修正本 change 的 delta spec**（不是绕过 spec，是纠正我写错的条款）。
3. **构建内存**：`pnpm build` 在裸跑时 OOM（exit 134），需与 `deploy-frontend.sh` 同款 `NODE_OPTIONS=--max-old-space-size=3584`。已在验收流程按此执行，属本机既有约束，非本 change 引入。
4. **存量红测试**：`LaneTrendChart.test.ts` 2 例（断言旧文案「事件线数」），HEAD 基线即红，与本 change 无关，已由实现线程 `test-patrol.sh --register fe-lane-trend-chart-wording` 登记台账。

## 验收后微调与复测

首轮截图发现左轨名称被截断（需要 141px、实得 128px，「中东地缘政治与美伊关系」显示成「中东地缘政治与美…」）。调整 `.rail-item` gap `0.5rem → 0.35rem`、`.rail-cell` 宽 `6px → 5px` 后重建、重新部署、复测：

- 依据：13 行 `scrollWidth === clientWidth` **全部不截断**（列宽 144px）。
- 依据：仍为 14 格/行；A-1 ~ A-9 全部断言重跑通过（见上文数字）。
- 复测单测：`BoardRail / nowBrief / railClueAxis / BoardContextRail` 4 文件 **60 tests 全通过**。
- 复测 `pnpm lint`：0 error（7 warning 全为存量、非本 change 文件）；`pnpm exec nuxi typecheck`：exit 0。

## 门禁与文档对账

- `openspec validate console-rails-live-signals --strict` → valid（5/5 artifacts）。
- `bash scripts/harness/doc-impact.sh verify openspec/changes/console-rails-live-signals` → **通过**（声明 `flow api`，文件 3 个）。
- `bash scripts/harness/check-standards.sh` → 通过 315 / 失败 2，**两项均非本 change**：
  - `未溯源 2026-11-17-fix-lane-row-gap-zoom`（已归档 change 缺溯源行，历史文档债）
  - `doc-impact 失败 fix-radar-overview-false-zero`（另一会话的活跃 change）
  - 本 change 的 doc-impact 项为 `[OK]`。

## 验收后第二处微调（用户裁决 2026-11-19）

体检行原为 `88 · 上次体检 … · 刷新 · 评语↗` 四元素，1440 下（右轨 18rem = 288px）把体检时间挤成 `11/19 2…`。用户裁决「徽章可点、行末只剩刷新」：

- 评分徽章改为 `<button>`（`title`「查看完整评语与依据」），点击 emit `openHealth` 进体检详情；无快照时仍是不可点的「等待首轮体检」span。
- 「评语↗」独立按钮移除（入口收进徽章，不丢可达性）。
- 「刷新」改为图标钮（`mdi:refresh`，`aria-label` + `title`）省出宽度。
- 复测：`rail-health-checked-at` 文本 = 「上次体检 11/19 21:06」且 `scrollWidth === clientWidth`（**不再截断**）；`rail-health-detail` 节点不存在（测试已改为断言「徽章是 BUTTON 且点击 emit openHealth」）。
- 复测单测：`BoardContextRail + BoardRail` 2 文件 **32 tests 全通过**；typecheck exit 0；lint 0 error。
- 截图已更新为本院 `1440x900.png` / `1920x1080.png`。

## 结论

A-1 ~ A-9 **全部 pass**（其中 A-4 的数值一致性另经后端 API 逐条对账）。无 fail、无未覆盖项。
