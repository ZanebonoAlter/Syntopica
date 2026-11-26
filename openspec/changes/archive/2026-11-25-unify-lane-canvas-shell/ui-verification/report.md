# UI 验证台账 · unify-lane-canvas-shell

> 分步落地（design D7），截图与机械核对按步登记。**before 基线 = 改动前**（本 change 第一步之前）。

## 1. 基线（tasks 1.3）——已采集

见 `before/README.md`（采集条件：1440×900 / 30 天窗口 / 横向 `scrollLeft = 520` / 浅色 / 静态托管 `http://localhost:5100/tags/1980`）。

| 文件 | 观察结论 |
|---|---|
| `before/correction-scrolled-h520.png` | 编辑侧分区名（本泳道话题带 / 候选区 / 只读参照）**整片滚出视口**，左缘直接是 11-01 起的日期列 —— 对应漂移 ③ 的「失向」 |
| `before/compose-scrolled-h520.png` | 新建侧行标签列**仍在左缘**（本次泳道 / 候选链名 / 散料），日期刻度从 11-01 起 |

两张图对照即 design.md §Context 第 3 号漂移的视觉证据。

## 2. 第一步（外壳 composable）后

**视觉判据：两侧视觉必须与 before 逐像素一致。** 未重截，原因见 §5。

反证走**既有用例**（同一批断言在改动前后都跑）：

| 断言 | 覆盖用例 | 结果 |
|---|---|---|
| 100% 时 `viewBox` 宽 == 容器 `clientWidth`（1:1 契约） | `LaneComposeCanvas.test.ts` Z2 | 绿 |
| 缩放只改呈现、不改几何与选择 | Z3 / Z7 / 11.5 组 | 绿 |
| 平移：指针分流 + 4px 阈值 + 拖后吞 click | P1–P3 组 | 绿 |
| 横向吸顶位移 == `scrollLeft / zoom`；底与 6 级阴影跟着走 | 11.3 组 | 绿 |
| 日期刻度只纵向吸顶、横向随内容走 | 11.3 末条 | 绿 |
| 编辑侧落点分区判定 / 拖拽取消 / 空态与骨架 | `LaneCorrectionCanvas.test.ts` 全套 54 条 | 绿 |

三处漂移的对齐由**新增两条用例**兜底（`LaneCorrectionCanvas.test.ts`「手势外壳与编排画布同源（2.4 / D3）」组）：

- `D3-①` 拖拽节点期间滚轮不缩放、松手后恢复 —— 已反证：把 `isZoomBlocked` 改成恒 `false` 时该用例**红**；
- `D3-②` 平移中按 `Esc` 取消手势后指针继续移动不再改 `scrollLeft` —— 已反证：去掉 `panStart = null` 复位的 `Esc` 分支时该用例**红**。

## 3. 第二步（抽吸顶标签列单元）后

**判据：新建画布视觉必须与第一步后逐像素一致（纯内联去重）。** 截图未重截（§5），改走**DOM 逐项对照**（design 风险段第 1 条点名的「机械验证基准」）。

用一次性脚本（已删除，不入库）把三处标签列的 `outerHTML` 规范化（剥 `data-v-*` scope 属性）后与重构前的模板逐项比对，结论：**逐属性一致**。

普通行（30 天窗口、`scrollLeft = 260`，`clientWidth 900`）：

```
GROUPS=5  BG=5  TINT=5  SHADOW=5  LABELSHADOWS=30   ← 5 组 × 6 级 = 30，钩子数量关系不变

<g class="lcc-canvas__row-label" data-role="row-label-group" transform="translate(260, 0)">
  <rect class="lcc-canvas__label-bg"    data-role="row-label-bg"    x="0" y="138" width="132" height="82">
  <rect class="lcc-canvas__sticky-tint" data-role="row-label-tint"  x="0" y="138" width="132" height="82">
  <rect class="lcc-canvas__row-handle"  data-role="row-handle" data-row="1" x="4" y="140" width="122" height="28">
  <text class="lcc-canvas__row-name"    data-role="row-label" x="10" y="156">候选链0</text>
  <text class="lcc-canvas__row-count"   x="10" y="167">剩余 1 条</text>
  <text class="lcc-canvas__row-tally"   x="124" y="167" text-anchor="end">0/1</text>
  <rect class="lcc-canvas__label-shadow" data-role="row-label-shadow" x="132" … opacity="0.28">
  … x=133 0.23500000000000004 / 134 0.19000000000000003 / 135 0.14500000000000002 /
     136 0.10000000000000003 / 137 0.05500000000000005
</g>
```

只读梯子（`showAssigned`、`scrollLeft = 180`，`y=244 height=42`）：结构同上，槽位是「已有泳道 · 只读」一行 + 泳道名，`tint` 与 6 级阴影位置/透明度同数值 —— 与重构前内联版**一致**。

叠在置顶层（`pinnedLaneLabelVisible`）的那一份由既有用例兜底：`overlay` 外层 `translate(0, 400)`、内层标签组 `translate(200, 0)`（`LaneComposeCanvas.test.ts`「纵横同时滚动」条），且「`row-label-shadow` 数 === `row-label-group` 数」「不溢出时 `row-label-bg` 数量为 0」两条既有断言继续通过。

**结论：第二步 === 第一步后，视觉无差异**（机械对照，非人工截图）。

## 4. 第三步（编辑侧接入吸顶，tasks 4.1–4.4）

**判据：这一步是唯一有视觉变化的一步。** 实拍截图仍不可用（原因见 §5：`http://localhost:5100` 托管的是改动前产物，而本批禁止 `pnpm build`），改走 **机械 DOM 断言 + `before/` 图对照**，不伪造截图。

### 4.1 编辑画布横滚前 / 后对照

实现：标签列（话题带两行标签 + 两个分区名 + 「更早」折叠带及其首义勾选框）整组进 `LaneCanvasStickyRail`，列宽取 `axisX0`（150 ↔ 226），`offset = scrollLeftLogical`；日期轴刻度层单独一组钉顶 + 横向裁切。

| 观察项 | 改动前（`before/correction-scrolled-h520.png`） | 改动后（DOM 投影；部署后按同条件重截待补） |
|---|---|---|
| 固定列出现 | **无**——左缘直接是 11-01 起的日期列 | 有：`[data-role="row-label-group"]` 下发 `translate(520, 0)`，列宽 = `axisX0`（有折叠带 226 / 无 150），不透明底 `row-label-bg` 从画布顶（`y=0`）盖到第二个分区标签下缘 |
| 分区名可见 | 三处分区名（本泳道话题带 / 候选区 / 只读参照）**整片滚出视口** | 仍在视口左缘：四行分区名的 `x` 属性保持内容坐标 `14`，屏幕位置 = `14 + 520 − 520 = 14`（无双重平移） |
| 折叠带与勾选框 | 随内容一起滚走 | 在固定列内：勾选框 `x ∈ [196, 206]` ⊂ `[0, 226]`，点击仍 `emit('toggleAnchor')` |
| 右缘阴影 | 无 | 6 级渐隐（`x = 列宽 + 0…5`，`opacity = 0.28 − 0.045 × (band−1)`；1px 实景宽） |
| 内容遮挡 / 错位 | 无（本来就没有固定列） | 固定列画在节点与标题**之后**，滑到其下方的点与标题被不透明底整片盖住；节点标题落位下限 `x1 ≥ axisX0 + 14`（`labelPerLine` 让标签宽 ≤ 列宽），故列内不会被内容“穿透”留残字 |
| 日期轴（纵滚） | `y=14` 内容坐标，随内容一起滚走 | 钉顶：`translate(0, scrollTopLogical)`，视口 y 恒 14 ∈ `[0, DATE_AXIS_H=36]` |
| 日期轴（横滚） | 随内容左移（对得上列，但会滑到标签列上方） | 仍随内容左移（**不做横向吸顶**），但被裁切到标签列右侧：裁切框左界 = `axisX0 + scrollLeftLogical`（视口恒 `axisX0`） |

### 4.2 折叠带出现时的固定列宽度跳变（一次，内容不重排）

| 折叠带 | `axisX0` | 固定列宽 | 现象 |
|---|---|---|---|
| 无（`foldNodes = []`） | `LEFT_PAD` = 150 | 150 | 列底/阴影宽 150，run 内一切按内容坐标就位 |
| 有（窗外首义节点非空） | 150 + 68 + 8 = 226 | 226 | 宽度跳变一次；标签与勾选框的 `x` 全为内容坐标（未参与列宽推导），故**内容不重排**，只是列底与右缘阴影右移 76px |

### 4.3 反证（把吸顶去掉，哪条用例变红）

把 rail 的 `:overflowing` 恒 `false`、日期轴 `transform` 恒 `undefined` 后跑新用例：

- 红：`横滚后分区名仍可见`、`横向溢出：固定列从画布顶开始、宽度取日期轴起点（无折叠带 = 150）`、`纵滚后日期轴仍可见`、`折叠带存在时固定列不吞掉勾选框`（4 条）
- 仍绿：`不溢出时不出现吸顶痕迹`（它断言的是“不出现”，去掉吸顶当然仍绿——正说明剩下 4 条真的在测吸顶）

### 4.4 遗留观察（①已补，②③非阻塞）

① **【已补】日期刻度层没有自己的不透明底色** —— 纵滚 40–60px 后话题带与分区线就会滑进 `y ∈ [0, DATE_AXIS_H = 36]` 的刻度区，日期数字直接压在节点圆 / 标题 / 分区线上（刻度不可读）。已按新建画布吸顶带的三件套补齐：`<g class="lcc-axis-ticks">` 里新增通栏不透明底 `data-role="date-axis-bg"`（`--color-bg-base`）+ 提档 `date-axis-tint`（`--color-bg-elevated`）+ 下缘 1px 硬分隔 `date-axis-edge`，三者同受 `overflowing` 门控；底从 `axisX0` 起（不从 0）以免盖住左侧吸顶标签列；`y = -scrollTopLogical` + `height = scrollTopLogical + 36`，即在视口坐标里恒盖 `[0, 36]`。用例：「纵滚后日期轴仍可见」补强 + 新增「不溢出时日期轴不画底色」；两次反证（门恒 true / 恒 false）各自能打红对应那条。

② 固定列 `tint`（`--color-bg-elevated`）与新建侧三处 rail 一致（D8 允许的两个色都在既有 token 内，不新增语义色）；若不希望左栏整条提一档，把 `tint` 去掉即回落到 `--color-bg-base`（仅改一个布尔）。

③ 【已知小代价，不处理】日期轴吸顶带的底从 `axisX0` 起、画在吸顶标签列之后，因此纵滚时会把固定列右缘 6 级阴影的顶上 36px 盖住；两层都是提档色看不出分界，只表现为阴影从带下缘才开始。与新建侧同一层序口径。

## 5. 为什么「第一步 / 第二步后」没有实拍截图

`http://localhost:5100` 与 `http://<pi-ip>/` 托管的是**改动前的静态产物**（`/srv/www` 与 `backend-go/frontend/`，构建于本 change 动手前）。要拍「改动后」的图必须 `pnpm build` / `pnpm generate` + `deploy-frontend.sh` —— 本批任务明确禁止跑 `pnpm build`（树莓派 4 核，且可能与其它会话的构建相互干扰；实测本批期间宿主 load average 一度到 8.6）。

替代证据：§2 的既有用例 + §3 的 DOM 逐项对照，覆盖的都是「视觉不变」的机械判据。

**遗留动作（verify 阶段，tasks 8.4 / 8.8 一并做）**：构建并部署后，按 `before/README.md` 的同一条件（1440×900 / 30 天窗口 / 浅色 / 静态托管 `:5100/tags/1980`）重截：

1. 新建画布 `scrollLeft = 520` → 与 `before/compose-scrolled-h520.png` 逐区域比对（行标签列底色 / 6 级右缘阴影 / 文字位置），期望**无差异**；
2. 编辑画布 `scrollLeft = 520` → 预期出现固定列（分区名常驻），与 `before/correction-scrolled-h520.png` 对照记录「固定列出现 / 阴影 / 无内容遮挡错位」，并单独记一次折叠带出现时的固定列宽度跳变（150 ↔ 226）。

## 6. 验收判定台账（ui-verification-trace-gate 口径）

> 本 change 为 `ui-impact: minor`，验收映射按 `ui-design.md` §Acceptance。判定词表封闭三值。

| 条目 | 判定 | 依据 |
|---|---|---|
| A1 编辑画布横滚后分区名仍可见 | **pass** | 实现期临时探针（已按 test-design 清理）+ 部署后实拍 `after/correction-scrolled-h520.png`（§7）；反证：吸顶门恒 false → 探针变红 |
| A2 编辑画布纵滚后日期轴仍可见 | **pass** | 临时探针断言 `date-axis-bg` 恒覆盖视口 `[0,36]` + 部署后实拍 `after/correction-vscroll-300.png`（实测 `y=-300`/`height=336`/`x=150`，§7） |
| A3 不溢出时不出现吸顶痕迹 | **pass** | 临时探针（不溢出时 `row-label-bg` / `date-axis-bg` 计数均为 0）+ 实拍 `after/correction-h0.png` 与改动前一致；反证：门恒 true → 探针变红 |
| A4 折叠带存在时固定列不吞掉勾选框 | **pass**（未实拍） | 临时探针断言 `axisX0 = 226` 且首义勾选框仍可命中；本泳道 `foldNodes` 为空、该分支无实拍数据，已在 §4.2 与 §7 末尾如实标注 |
| A5 新建画布吸顶行为无回归 | **pass** | `LaneComposeCanvas.test.ts` 94 条全绿，含「不溢出时 `row-label-bg` 数量为 0」「`row-label-shadow` 数 === `row-label-group` 数」；`STICKY_SHADOW_BANDS` 全仓唯一定义点 |
| A6 拖拽期间滚轮不缩放、`Esc` 同时取消拖拽与平移 | **pass** | **保留为永久 Bug 回归**：`LaneCorrectionCanvas.test.ts` 组「手势外壳与编排画布同源」；反证：`isZoomBlocked` 恒 false → 红，去掉 `panStart` 复位 → 红 |
| A7 1440×900 下编辑画布横滚前后视觉对照 | **pass** | 构建部署后按 before 同条件实拍：`after/correction-scrolled-h520.png` vs `before/correction-scrolled-h520.png`——改前分区名整片消失、改后常驻左缘；`after/compose-scrolled-h520.png` 与 before 逐区域一致，见 §7 |
| A8 1920×1080 下两档双视口视觉检查 | **pass** | `after/correction-scrolled-h520-1920x1080.png`：固定列与日期轴表现一致、无横向溢出与错位，见 §7 |

## 7. 构建部署后的实拍对照（tasks 4.4 / 8.4 / 8.8）

构建走仓库限载路径 `bash scripts/dev/deploy-frontend.sh`（`nice -n 19` + `NODE_OPTIONS=--max-old-space-size=3584` + 看门狗 + `pnpm generate`）。裸 `pnpm build` 在树莓派上 **Node OOM（exit 134）**，已改口径，见 tasks 8.4。

采集条件与 `before/README.md` 完全一致：静态托管 `http://localhost:5100/tags/1980`、30 天窗口（2026-10-26 → 2026-11-24）、浅色、`scrollLeft = 520`。

| 文件 | 视口 | 观察 |
|---|---|---|
| `after/correction-h0.png` | 1440×900 | 横滚 0：与改动前一致（无固定列底、无位移） |
| `after/correction-scrolled-h520.png` | 1440×900 | **分区名常驻左缘**（本泳道话题带 / 候选区 / 未选入本泳道），日期轴 11-03 起；对照 `before/correction-scrolled-h520.png`（左缘直接是 11-01 日期列、分区名整片消失）——**漂移已消除** |
| `after/correction-vscroll-300.png` | 1440×900 | 纵滚 300：日期轴钉在画布顶（11-03…11-11），刻度带不透明底生效（实测 `date-axis-bg` `y=-300` / `height=336` / `x=150`，即视口恒盖 `[0,36]`），无内容与刻度叠字 |
| `after/correction-scrolled-h520-1920x1080.png` | 1920×1080 | 双视口档：固定列与日期轴表现一致，无横向溢出、无错位 |
| `after/compose-scrolled-h520.png` | 1440×900 | 与 `before/compose-scrolled-h520.png` **逐区域一致**（行标签列常驻、`11-01 / 11-04` 刻度位置相同、无新增/缺失元素）——吸顶单元抽取未改变编排画布视觉 |

`hasFold` 在本泳道为空（`[data-testid=lane-fold-band]` 不存在），折叠带宽度 150↔226 的跳变未能实拍；该分支由实现期探针断言覆盖（`axisX0 = 226` 且勾选框可命中），已在 §4.2 表格记录。**如实标注为未实拍，不伪造。**

## 8. 验收后实机发现的缺陷与修复（2026-11-25，用户反馈）

**用户反馈**：「左侧都错位了」（编辑泳道，深色主题 + 146% 缩放 + 时间视窗曲线面板收起）。

**为什么前一版验收没抓到**：§1–§7 的实拍全部是「**浅色主题 + 100% 缩放 + 30 天窗口**」这一组合。缺陷只在**纵向滚动 + 缩放**下出现，而这恰好不在验收矩阵内。这是验收设计的漏，不是"看起来没差别"。

**根因（实现缺陷，非设计）**：吸顶标签列的不透明底高度写死为**内容坐标** `0 → zone2Line + 30`（逻辑 759px）。后果：

- 纵向滚动时这块底跟着内容上移，底部露出一截**没有底的空带**；
- 中间留下**一道硬接缝**（上tint / 下画布底），视觉上像一块错位的色块，而不是一列钉住的标签栅。

**修复**：底色改为**按容器可视高度铺满**，与日期轴吸顶带同一算法但**符号相反**——日期轴那组自带纵向 `translate(0, S)`（两者抵消，取 `y = -S`）；标签列这组只有横向位移、无纵向 translate，故取 `y = +S`、`height = 容器可视高 / zoom`。实测 `scrollTop=300`/`146%` 下底色包围盒 = 容器 `[388, 1392]`，与视口逐像素重合（修复前是 `[-212, 1092]`）。

**顺带补强**：标签列右缘加 1px 竖向分隔（`data-role="row-label-divider"`，`divider` prop，默认关），让它读作「钉住的一列」而不是「浮着的一块」。不溢出时不画（沿用既有门控）。

**复测状态矩阵**（深色主题，视口 3282×1392）：

| 窗口 | 缩放 | 滚动 | 结果 |
|---|---|---|---|
| 7 天 | 100% | 无横溢 | 不画吸顶（门控正确） |
| 7 天 | 146% | h=200 | 列满高、无接缝 ✓ |
| 7 天 | 146% | h=200, v=300 | 列满高覆盖 `[388,1392]` ✓ |
| 30 天 | 146% | h=600, v=500 | 列满高、分隔线在右缘 ✓ |
| 30 天 | 100% | h=520 | 无横溢 → 不出列（正确） |

影响包用例 263 通过 / 4 文件；`pnpm lint` 0 error；`nuxi typecheck` exit 0。

**仍未解释的现象**：用户截图里有一块**浅色（亮底深字）矩形**盖在左面板区域，含「只读参照 / 属其它泳道」两行。在本地按「深色 + 146% + 7 天 + 面板收起 + 横纵滚动」均**未复现**；且该块颜色与深色主题无关（`lane-correction-surface` 与 `LaneCanvasStickyRail` 内无任何硬编码浅色，`fill` 全部走 `--color-bg-*`），也超出 `.lcc-surface` 的 `overflow:auto` 裁切范围（不可能是画布内的 SVG）。**待用户提供复现状态或刷新后复看。**
