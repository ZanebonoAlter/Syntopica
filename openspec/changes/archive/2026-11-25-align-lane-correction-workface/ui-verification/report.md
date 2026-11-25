<!-- ui-verification: 双视口实机验收（agent-browser 无头 Chromium） -->
<!-- ui-impact: minor -->

# UI 实机验收 — align-lane-correction-workface（任务 10.5）

> 被验对象：泳道修正工作面（`BoardComposeOverlay.vue` mode=correction，中央 `LaneCorrectionCanvas` + 左栏 `LaneMemberList` / `LaneAnchorScope`）与新建泳道画布（`LaneComposeCanvas`）。
> 验证方式：实机 agent-browser（headless Chromium）点击 / 指针拖拽 / DOM 量测，**非组件测试**。前端已构建并静态托管，API 与页面同端口 `http://localhost:5100/`（同源 `/api`），后端单端口运行中。
> 视口：1440×900 与 1920×1080 各独立走一遍（各自 reload + 重新进入工作面，不复用另一视口的内存态）。
> 数据：真实库。板块 `1974 中东地缘政治与美伊关系` → 矩阵视图 → 泳道行头「⋯」→「编辑泳道」，目标泳道 `topic 1204 美伊博弈升级：从经济威胁到全面孤立`，时间窗「7 天」（2026-11-16 ~ 2026-11-22）。入口路径全程未变更 URL，仍是 `/tags/1974`（工作面为全屏 overlay）。
> 实测时刻：2026-11-24。

## 1440×900

### A-1 【入口】矩阵行头「⋯」→「编辑泳道」打开三栏工作面【pass】
- **依据**：`/tags/1974` 矩阵视图点 `[data-testid=lane-menu-trigger-1204]` → 菜单两项逐字为「编辑泳道」「归档泳道」，点前者后全屏 overlay 打开，三栏实测 `bco__left` x:0..380、`bco__correction-canvas` x:380..1140（宽 760）、`bco__right` x:1140..1440，与 `ui-design.md` 声明的 380 / minmax(0,1fr) / 300 一致。截图 `01-correction-workface-1440x900.png`。

### A-2 【拖拽加入】候选拖入成员区：计数 +1、节点落到成员区、刷新后仍在【pass】
- **依据**：把候选 `9054 美国第二次退出《巴黎协定》并缺席COP31`（11-20，候选区 y≈704）拖到成员区（客户端坐标 898,704 → 700,520）。拖拽中成员区出现可落点高亮（`02-drag-member-zone-highlight-1440x900.png`）；松手后左栏 `窗口 12 · 全部 121` → `窗口 13 · 全部 122`，canvas `.lcc-node.is-member` 12 → 13，该节点标题进入成员清单（`03-drag-add-landed-1440x900.png`）。整页 reload + 重进工作面后仍为 `窗口 13 · 全部 122` 且该节点仍在成员清单（`04-after-reload-persisted-1440x900.png`）→ **真落库**，非纯前端。

### A-3 【拖拽移出】成员拖出成员区：计数 −1、左栏出现「已移出（本次）」并可「加回来」【fail】
- **依据（功能正确部分）**：把成员 `9139 特朗普称寻求永久协议同时称若伊朗拥核将开战…`（11-22）由成员区（1061,578）拖到候选区（620,860）：拖拽中候选区落点框 class 实测变 `lcc-canvas__dropzone is-drop-target`（accent 14% 填充 + stroke 1.5），成员区回到无标记默认虚线框、只读参照区恒 `is-drop-other`（`05-drag-out-candidate-zone-1440x900.png`）；松手后 `窗口 13 · 全部 122` → `窗口 12 · 全部 121`，左栏出现「已移出（本次）· 1 条」，行内含 `[data-testid=lane-restore-9139]`「↩ 加回来」，点击后回到 `窗口 13 · 全部 122`（`06-removed-block-after-dragout-1440x900.png`）——写路径与回滚路径都真实可用。
- **期望 vs 实际**：期望「已移出（本次）」行完整可读（节点日期 + 标题 + 加回来按钮完整可见）。实际该行的列表体 `.lml-body` 实测高 **15px**，而一行 `.lml-row` 实测高 **28.5px** → 行被裁切，标题只露出上半截、「↩ 加回来」按钮被切掉下半（`23-left-column-clipped-1440x900.png` 与 `06-removed-block-after-dragout-1440x900.png`）。同一处还有「更早成员 · 109 展开」面板头被裁：`.lml-panel--early` 段落实测高 **27px** < 其 `.lml-head` 高 **37px**，面板头下缘被下一块（首义范围）盖住。
- **复现步骤**：1440×900 → `/tags/1974` → 泳道 1204「⋯」→「编辑泳道」→ canvas 上把任一成员节点拖到候选区松手 → 观察左栏「已移出（本次）」行。
- **定位线索**（只报不修）：`.bco__left` 是 `display:flex; flex-direction:column; overflow-y:auto`，子块 `.lml-panel` / `.las-panel` 未设 `flex-shrink:0`，内容总高 637px > 容器 512px 时子块被压缩到小于自身头部高度，而非按注释所述「整体纵向可滚」。本 change 未改 `LaneMemberList.vue`，但 `LaneAnchorScope.vue` 本次 +162 行（新增「首义来源」三行 +「参与滚动计算的节点」块）抬高了左栏内容总高，是该挤压在 1440×900 复现的直接触发条件（1920×1080 不复现，见 A-11）。

### A-4 【相关度分档】候选圆点大小/深浅肉眼可辨；底部图例是尺寸图例且两项首义标记都在【pass】
- **依据**：候选可见圆半径实测 **5.9 → 7.4px**（直径 11.8 → 14.8，比值 1.25）、填色强度 `--proximity-fill` **18% → 80%**，成员固定 r=7.5、只读参照 r=3（不与候选抢档）。批次内 17 个候选实测半径从 `10.9/5.9`（最小 `9056`）到 `12.4/7.4`（最大 `9141`），连号密集分档、无并列断层；截图中「伊朗举办电竞锦标赛…」明显最大最深、「美国10年期国债收益率…」明显最小最浅（`10-candidate-size-legend-1440x900.png`）。
- **图例原文**：`● 成员 ｜ ○—● 候选大小=接近度 ｜ ○ 已生效首义 ｜ ⊙ 待提交勾选 ｜ ⊘ 已移出 ｜ ○ 相关`——「候选大小=接近度」用的是**两个尺寸不同的小圆（5px / 11px）**构成尺寸图例，不是单一固定圆；`已生效首义`（实线环）与`待提交勾选`（虚线环）两项均存在且形态不同（同截图左下，实测图例框 x:392 y:865 422×25，`bottom=890 ≤ 900` 完整在视口内）。

### A-5 【首义来源】「首义范围」块读到来源行，不可考时不列节点；滚动窗口计数与「窗口内 N · 全部 M」不同名【pass】
- **依据**：左栏「首义范围 / 窗口内 12 · 全部 121 / 次日日报生效」下逐行原文：`当前首义来源：自动（创建话题时写入）`、`首义来源节点：创建时写入 · 来源不可考`、`用下方的节点定一次首义即可留痕。`——`resolved=false` 形态下**未渲染任何来源节点列表**（随后直接是「参与滚动计算的节点（30/30）」折叠块）。见 `09-anchor-scope-block-1440x900.png`。
- **命名隔离**：泳道级窗口计数叫「**窗口内 12 · 全部 121**」，滚动质心窗口块叫「**参与滚动计算的节点（30/30）**」，两者字面不同名；后者是可折叠块，点开后实测 **30 条**、首条 `2026-11-22 特朗普称寻求永久协议同时称若伊朗拥核将开战…`、末条 `2026-11-03`（倒序），按钮字变「收起」，再点复原。

### A-6 【新建泳道高度 + 不可落点】画布填满可用高度、无下方空白带；非法落点有弱化态且与可落点高亮不同【pass】
- **依据（高度）**：顶栏「新建泳道」→ `.lcc-canvas` 实测 y:395→900（高 **505px**）＝ 可用高度全占，`.lcc-canvas__surface` `clientHeight=505` 且 `scrollHeight=505`（无内部溢出、无剩余空白带）；末行「低相关散料 45 条 · 展开」贴到底边（`11-compose-canvas-fullheight-1440x900.png`）。
- **依据（不可落点）**：拖起散料节点后指针悬停在候选链行（`data-row-kind=chain`）上，该行 `class` 实测追加 `is-drop-other`（样式 `opacity: 0.35`），行内文字明显变淡（`12-compose-invalid-drop-dimmed-1440x900.png`）；移到「本次泳道」行（`kind=lane`）时 class 变 `is-drop-target`（落点框 `accent 14%` 填充 + `stroke-width 1.5`），整行落在更深的高亮框里（`13-compose-valid-drop-highlight-1440x900.png`）。两态视觉语言不同：一个是**整行变淡**、一个是**落点框加深**。

### A-7 【无回归】节点卡三按钮、缩放、空白拖动平移仍可用【pass】
- **依据（节点卡）**：`lane-node-9054`（成员）卡内按钮 `⊘ 移出本泳道`（`07-node-card-member-1440x900.png`）；`lane-node-9139`（已移出）卡内按钮 `↩ 加回本泳道`（`08-node-card-restore-1440x900.png`）；候选节点卡内按钮 `＋ 加入本泳道`（1920 侧 `19-node-card-candidate-1920x1080.png` 同款）。三态按钮均实际点击生效（A-2/A-3 的计数变化即由按钮路径单独复验过）。
- **依据（缩放）**：点「＋」→读数字面 `110%`，点「－」→ `99%`，点「重置」→ `100%`（`[data-testid=lane-zoom-readout]` 逐次实读）。
- **依据（平移）**：在空白处按下并向上拖 150px，`.lcc-surface.scrollTop` 实测 **0 → 150**（向下拖回 0），未误触节点选中。

### A-8 【左栏挤压】1440×900 下「更早成员」面板头被压掉下半（与 A-3 同根因）【fail】
- **依据（有「已移出」条目，即左栏三块堆叠时）**：左栏几何实测 `.lml-panel` 388→541（153px）、`.lml-panel--early` 541→568（**27px**）而其 `.lml-head` 542→579（**37px**）、`.lml-panel--out` 568→619、`.las-panel` 619→900；`bco__left` `scrollHeight=637 > clientHeight=512` 却未产生滚动（子块被 flex 压缩）。视觉上「更早成员 · 109 展开 / 重心 · 次日重算」这一行在 y≈568 处被一条硬横切线截断（按钮下边框与文字下半截一起消失），下一块「已移出（本次）」紧贴其上（`23-left-column-clipped-1440x900.png` 顶部）。
- **依据（无「已移出」条目时）**：`.lml-panel--early` 558→588（**29px**）< `.lml-head` 559→596（**37px**），此时仅压掉内边距、文字仍可读（`24-left-column-early-header-clipped-1440x900.png`）——说明这是「内容一多就破」的临界状态，不是稳定布局。
- **期望 vs 实际**：期望左栏超屏时整栏纵向滚动、各面板块保完整高度（`BoardComposeOverlay.vue` L861-863 注释即如此声明：「左栏整体纵向可滚：三块堆叠总高超屏时仍能滚到第三块」）。实际子块被压缩到小于自身头部高度，内容不可读，且没有任何滚动条可以补救。

## 1920×1080

### A-9 【入口】同上入口打开三栏工作面【pass】
- **依据**：同 A-1 路径；1920 下三栏为 `bco__left` 0..380、中央画布 380..1620、`bco__right` 1620..1920，无裁切；`.lcc-legend` 实测 x:392 y:1045 422×25、`bottom=1070 ≤ 1080`。截图 `14-correction-workface-1920x1080.png`。

### A-10 【拖拽加入】候选拖入成员区：计数 +1、刷新后仍在【pass】
- **依据**：候选 `8907 委内瑞拉与美国签署协议恢复军事技术合作`（11-19）由（1056,704）拖到成员区（800,500）；拖拽中成员区高亮（`15-drag-member-zone-highlight-1920x1080.png`）；松手后 `窗口 12 · 全部 121` → `窗口 13 · 全部 122`，`.lcc-node.is-member` 12 → 13，左栏成员清单出现该标题、接近度 `0.79（快照）`（`16-drag-add-landed-1920x1080.png`）。整页 reload + 重进工作面后仍为 `窗口 13 · 全部 122` 且该节点在列 → 真落库。

### A-11 【拖拽移出】计数 −1、出现「已移出（本次）」并可「加回来」（1920 无裁切）【pass】
- **依据**：把成员 `8907` 拖回候选区（700,860）→ `窗口 12 · 全部 121`，左栏出现「已移出（本次）· 1 条」+ 行 `11-19 委内瑞拉与美国签署协议恢复军事技术合作` + `↩ 加回来`，**行与按钮完整可见**（`17-removed-block-after-dragout-1920x1080.png`）；「加回来」按钮同屏可点，点击后 `removedNodes` 由 `[8907]` 变空。同视口几何实测：`.lml-panel--early` 614→653（39px）≥ `.lml-head` 37px、`las-panel` 653→1069 完整——即 A-3/A-8 的挤压在本视口不复现，确认其为 1440×900 的高度压力问题而非结构错误。

### A-12 【相关度分档】候选圆点大小/深浅肉眼可辨；图例含尺寸图例与两项首义标记【pass】
- **依据**：1920 下同一批候选半径 5.9–7.4px、填色 18%–80%，截图中「美国科技系统故障叠加社交媒体焦虑…」明显大而深、「美国10年期国债收益率单日下跌10个基点至4.95%」明显小而浅（`18-candidate-size-legend-1920x1080.png`）；图例原文同 A-4（`● 成员 ｜ ○—● 候选大小=接近度 ｜ ○ 已生效首义 ｜ ⊙ 待提交勾选 ｜ ⊘ 已移出 ｜ ○ 相关`），`已生效首义`/`待提交勾选` 同时在列。

### A-13 【首义来源】来源行三态文案与滚动窗口计数隔离（1920 同款）【pass】
- **依据**：「首义范围 / 窗口内 12 · 全部 121 / 次日日报生效」+ `当前首义来源：自动（创建话题时写入）` + `首义来源节点：创建时写入 · 来源不可考` + `用下方的节点定一次首义即可留痕。`，其后为可折叠「参与滚动计算的节点（30/30）」，无任何来源节点列表（`17-removed-block-after-dragout-1920x1080.png` 下半 / `18-candidate-size-legend-1920x1080.png` 左栏）。

### A-14 【新建泳道高度 + 不可落点】画布填满可用高度、无空白带；非法落点弱化态与可落点高亮不同【pass】
- **依据（高度）**：顶栏「新建泳道」→ `.lcc-canvas` y:395→1080（高 **685px**），`.lcc-canvas__surface` `clientHeight=scrollHeight=685`，画布内最低已绘制元素 `bottom=1080` ＝ 视口底 → 下方无空白带；`data-row-kind=floor` 行被拉长到 779→1044 吃掉剩余高度（`20-compose-canvas-fullheight-1920x1080.png`）。
- **依据（不可落点）**：拖起散料后悬停候选链行 → 该行 class `is-drop-other`、整行明显变淡（`21-compose-invalid-drop-dimmed-1920x1080.png`）；移到「本次泳道」行 → `is-drop-target`、落点框加深加粗（`22-compose-valid-drop-highlight-1920x1080.png`）。

### A-15 【无回归】节点卡按钮、缩放、空白拖动平移【pass】
- **依据**：候选 `8571` 卡内按钮 `＋ 加入本泳道`（`19-node-card-candidate-1920x1080.png`）；缩放读数字面 `100% → 110%（＋）→ 99%（－）→ 100%（重置）`；空白处向上拖 140px，`.lcc-surface.scrollTop` **0 → 140**。

### A-16 【拖拽中视觉】落点分区语义双向一致：可落点=框加深、不可落点（含只读参照）=框弱化【pass】
- **依据（四个象限逐个实读 `class`）**：候选拖起、指针在成员区（y=500）→ `member: is-drop-target / candidate: 无类 / readonly: is-drop-other`；候选拖起、指针在候选区（y=780）→ `candidate: is-drop-other`；成员拖起、指针在成员区（y=490）→ `member: is-drop-other`；成员拖起、指针在候选区（y=800）→ `candidate: is-drop-target`。即「可落点加深、不可落点弱化（只读参照区在拖拽期间恒弱化）」与 `dropAction` 口径完全一致（成员区 = 加入/加回，成员区之外 = 移出，只读参照区/折叠带 = 不可落点）。视觉见 `02-drag-member-zone-highlight-1440x900.png`（候选→成员）与 `05-drag-out-candidate-zone-1440x900.png`（成员→候选）。

## 失败项汇总

| 条目 | 视口 | 现象 | 期望 | 截图 |
|---|---|---|---|---|
| A-3 | 1440×900 | 「已移出（本次）」行被裁：列表体 15px < 行高 28.5px，标题只露上半截、`↩ 加回来` 按钮切掉下半（功能仍可用，但用户读不出是哪个节点） | 行与按钮完整可读 | `23-left-column-clipped-1440x900.png`、`06-removed-block-after-dragout-1440x900.png` |
| A-8 | 1440×900 | 「更早成员 · 109 展开 / 重心 · 次日重算」行在 y≈568 被硬横切截断（段高 27px < 自身头部 37px），按钮下边框与文字下半截消失；`.bco__left` 内容 637px > 容器 512px 却不滚动 | 左栏超屏时整栏纵向滚动，各面板保完整高度 | `23-left-column-clipped-1440x900.png`、`24-left-column-early-header-clipped-1440x900.png` |

> 两条失败项同一根因（左栏 flex 子块缺 `flex-shrink:0`，被本 change 新增的首义来源/滚动窗口块抬高内容总高后触发），**仅在 1440×900 复现**；1920×1080 全部通过（A-11）。

## 验收期间对真实数据的副作用（已复原）

为验证「真落库」与「加回来」，本次验收对泳道 `topic 1204` 做了真实写操作，收尾已复原到进场时的成员集合：

- 进场基线：`窗口 12 · 全部 121`，成员 `8567,8735,8736,8815,8904,8905,9052,8984,8985,9137,9138,9139`，候选含 `9054` / `8907` / `9139 之外` 等。
- 中间态（两视口各测一遍）：加入过 `9054`、`8907`，移出过 `9139`、`9138`、`8907`。
- 收尾复原：移出 `9054`、加回 `9139`；最终 reload 实测 `窗口 12 · 全部 121`，成员集合与进场基线逐项一致，`9054` 回到候选区。
- **残留**：`9139` 走了一次「人工移出 → 人工加回」，其归属快照由人工动作重写（当前界面显示 `接近度 0.96（快照）`）。如需严格回滚到自动归属原值，需重新生成当日日报或人工核对 `topic_status_at_report` 快照 —— 属本功能正常语义，不是缺陷。


## 复验（第二轮）

> 复验时刻：2026-11-24 17:45–18:00。环境与第一轮同一套静态托管（`http://localhost:5100/`，后端 pid 246453，`/health` 200），**未重启服务、未改代码**。
> 动手前先确认「要验的修复确实在盘上」：`backend-go/frontend/index.html` mtime 17:43 晚于源码 mtime 17:40；产物 CSS 实读含 `.lml-panel{display:flex;flex-direction:column;flex-shrink:0;min-height:0}`（`data-v-92115277`）与 `.las-panel{…flex-shrink:0…}`（`data-v-f0390290`）。
> 对象与路径同第一轮：板块 `1974` → 泳道 `topic 1204` → 行头「⋯」→「编辑泳道」，时间窗 7 天。两视口各自 reload + 重新进入，不复用内存态。
> 判定一律取实读几何（`getBoundingClientRect` / `clientHeight` / `scrollHeight` / `elementFromPoint`），截图仅作旁证。上一轮 16 条条目与失败项汇总**未改动**。

### A-3 【复验】1440×900「已移出（本次）」整行与「↩ 加回来」完整可读【pass】
复验：把成员 `9139`（11-22）由成员区（1061,578）拖到候选区（700,810）松手 → 左栏 `窗口 12 · 全部 121 → 11 · 120`，出现「已移出（本次）· 1 条」。几何实读：`.lml-panel--out` `clientHeight 74 = scrollHeight 74`（= 头部 35 + 列表体 38.5），列表体 `.lml-body` `clientHeight 39 = scrollHeight 39`（**第一轮同一处为 15px，本轮不再裁切**），行 `.lml-row` 高 **28.5 / clientHeight 29** → 行高完整；`↩ 加回来` 按钮高 **20.5**，「按钮 ⊆ 行 ⊆ 列表体 ⊆ 面板 ⊆ 左栏」四层包含全部 `true`，`elementFromPoint(按钮中心)` 命中按钮自身（`btnHitSelf=true`）、`btnInViewport=true`。滚到底（`scrollTop 433`）后该面板落在 `409.6→484.1`，整体在左栏 `388→900` 内。截图 `round2-03-left-column-removed-block-1440x900.png`、`round2-04-left-column-early-head-plus-removed-1440x900.png`、`round2-13-left-column-three-blocks-bottom-1440x900.png`。

### A-8 【复验】1440×900「更早成员」面板头完整、左栏整栏可纵向滚到底（首义范围块按钮可点）【pass】
复验：点「更早成员 · 109 展开」后 `.lml-panel--early` 高 **228.5**（第一轮同一处仅 27px），其 `.lml-head` 高 **36.5**、自身 `scrollHeight 37`，`head ⊆ panel` 实测 `true`（0.5px 为亚像素舍入，不构成裁切）；「本泳道成员」面板保持 **226**（第一轮被压到 153）。左栏 `clientHeight 512` 而 `scrollHeight 945`（第一轮 637 且不滚）→ 整栏纵向可滚，`scrollTop` 实测可达最大值 **433**；滚到底时「首义范围」面板落在 `484→900`（底边 = 容器底边 900）完整在容器内，块内四个按钮全部 `inside=true` 且 `elementFromPoint` 命中自身（`参与滚动计算的节点（30/30）` y598→622、`更早成员 · 109` y639→664、`窗口内 · 11 收起` y668→692、`用选中的 0 个节点定首义` y854→884，后者 `disabled=true` 是 0 选中的设计态、非裁切）；`窗口内 · 11` 按钮实测点击生效（字面由「收起」变「展开」）。截图 `round2-01-left-column-early-expanded-1440x900.png`、`round2-02-left-column-scrolled-bottom-1440x900.png`、`round2-13-left-column-three-blocks-bottom-1440x900.png`。

### A-17 【复验】1920×1080 同场景无回归：行高、面板头、底部按钮几何全通过【pass】
复验：1920 下重放同一组操作（成员 `9139` 由（1507,578）拖到候选区）。几何实读：左栏 `clientHeight 692 < scrollHeight 945`（三块堆叠 + 更早成员展开）→ 可滚，`scrollTop` 最大 **253**；`.lml-panel--out` 高 74.5（头 35 + 体 38.5，体 `clientHeight 39 = scrollHeight 39`），行高 **28.5**，`↩ 加回来` 按钮 20.5 且 `按钮 ⊆ 行 ⊆ 体` 全 `true`、`btnHitSelf=true`、`btnInViewport=true`；`.lml-panel--early` 高 228.5、头 36.5 完整；滚到底时「首义范围」面板 `664→1080`（底边 = 容器底边 1080），四个按钮同样 `inside=true / hitSelf=true`。截图 `round2-05-left-column-removed-and-bottom-1920x1080.png`。

### A-18 【复验】「已生效首义」实线环与「待提交勾选」虚线环可分辨，且刷新页面后仍在【pass】
复验：本板块 4 条泳道进场时 `source` 全为 `auto`、`override_count 0`（无人工首义过），故按任务口径做一次真写入——泳道 `topic 1204`，选定节点 `8567`（11-16）+ `9137`（11-22）。① 勾选后：待提交勾选环 2 个，`stroke-dasharray "3px, 3px"`、`stroke-width 1.2`，按钮字面「用选中的 2 个节点定首义」（`round2-06-anchor-pending-rings-1920x1080.png`）；② 提交后：已生效首义环 2 个，选择器 `circle.lcc-anchor-ring--effective`，`stroke-dasharray "none"`（实线）、`stroke-width 1.8`、`r 14.5`，与虚线环同半径但 dash/线宽不同 → 两套标记可辨（`round2-07-anchor-effective-rings-1920x1080.png`）；左栏同步 `当前首义来源：人工（选定的 2 个节点）`、`首义来源节点：伊朗数日内两度袭击巴林，美军三航母齐聚中东，核谈判停滞 · 2026-11-16`，API `GET /api/daily-reports/topics/1204/anchor` 实读 `source=manual, section_ids=[8567,9137], override_count=1`；③ **reload + 重新进入工作面**：`effective 2`（`8567`/`9137`）、`pending 0`，环仍在（`round2-08-anchor-effective-after-reload-1920x1080.png`）；④ 切 1440×900 再 reload 进一次：仍 `effective 2`、来源行同款（`round2-09-anchor-effective-1440x900.png`）。写入已按下方「副作用」节精确回退。

### A-19 【复验】首义来源读失败只显示「读取失败 · 重试」，不显示「当前首义来源：自动」【pass】
复验：以请求拦截（`network route "**/api/daily-reports/topics/*/anchor" --abort`）制造一次真实读失败后重进工作面：`[data-testid=lane-anchor-source]`（「当前首义来源」行）**整行未渲染**（`hasSourceRow=false`），首义来源节点位只显示 `读取失败 · 重试` 按钮，且**未**回落到 `创建时写入 · 来源不可考`；画布仍可用（46 个节点照常渲染，成员操作未阻断）（`round2-10-anchor-read-failure-1440x900.png`）。取消拦截后点该按钮恢复：来源行回 `人工（选定的 2 个节点）`、来源节点解析回标题+日期、实线环恢复 2 个（`round2-11-anchor-read-retry-recovered-1440x900.png`）。**注**：这是主动拦截模拟的失败，不是自然偶发；未拦截时的正常态在同一会话中已多次实读。

### 复验期间对真实数据的副作用与回退

- **首义写入（A-18）**：泳道 `topic 1204`，节点 `8567` + `9137`。写入前基线：`source=auto / section_ids=[] / override_count 0`、`board_persistent_topics.embedding` 的 `md5 = 8bc7badf153a21ccc6a9b5c568d9296a`（长度 24029）、`centroid_pending_recompute` 已为 `true`。
- **回退已执行并逐项校验**：用 `topic_anchor_overrides` 里该行的 `old_embedding` 还原 `embedding`，删除本次追加的留痕行（写入前该表 `count=0`，回退后 `count=0`）；回读 `md5` 与基线**逐字节一致**（`8bc7badf153a21ccc6a9b5c568d9296a`），API 回 `source=auto, section_ids=[], override_count=0, anchor_origin.resolved=false`，工作面实线环 0 个、来源行回 `自动（创建话题时写入）`（`round2-12-anchor-reverted-1440x900.png`）。回退语句（供复现）：
  `UPDATE board_persistent_topics SET embedding = (SELECT old_embedding FROM topic_anchor_overrides WHERE id = <本次行>) WHERE id = 1204;` 然后 `DELETE FROM topic_anchor_overrides WHERE id = <本次行>;`
- **残留**：`board_persistent_topics.updated_at` 因这次写入被推进（无业务读取方，不影响匹配）；`topic_anchor_overrides` 的自增 id 已被消耗（下一行为 `3`），内容为空。
- **归属数据**：本轮对 `9139` 共走 3 次「人工移出 → 加回来」（1440 两次、1920 一次），每次移出后成员集合都当场复原；收尾复核 `窗口 12 · 全部 121`，成员集合与进场基线逐项一致（`8567,8735,8736,8815,8904,8905,9052,8984,8985,9137,9138,9139`），DB `daily_report_sections.persistent_topic_id=1204` 计数仍为 121（与「全部 121」一致）。`9139` 的归属快照因人工动作重写为 `manual` 置信（与第一轮同一节点、同一性质，非新增影响面）。

## 第三轮（实机反馈补丁：节点卡标题换行 / 话题带横条移除 / 空白轻点取消聚焦）

> 复验时刻：2026-11-25 01:05–01:20。跑 `bash scripts/dev/deploy-frontend.sh` 重新构建铺盘（`backend-go/frontend/index.html` mtime 00:40，`/health` 200）。
> 动手前先确认「要验的补丁确实在盘上」：产物 JS 实读含 `clear-focus`、**不含** `lcc-band-bar`（`curl http://127.0.0.1:5100/_nuxt/BNITwRzf.js` 逐字 `grep`）。
> 对象：板块 `3030` → 泳道 `topic 502`「AGENT编程工具相关」→ 行头「⋯」→「编辑泳道」，时间窗 7 天（2026-11-18 — 2026-11-24），画布 62 个节点。判定一律取实读样式/几何，截图仅作旁证。
> **本轮零写入**：只做「点节点 / 点空白 / 点卡片 / 缩放到顶」四类读操作，未触发任何归属或首义写接口，无需回退。

### B-1 【节点卡标题】长标题在卡内完整展示、换行不截断（1440×900 与 1920×1080）【pass】
聚焦 `DeepSeek Harness 发布 v0.2.3-alpha.1，新增任务时间线与代码审阅`（47 字）。实读：卡内标题文本与该节点全标题**逐字相等**；`.lcc-card__title` 计算样式 `white-space: normal`、`overflow-wrap: anywhere`、`text-overflow: clip`（旧实现为 `nowrap` + `ellipsis`）；`titleW 242 == scrollW 242`（无横向溢出），`titleH 35`（≈两行，单行行高 17.4），卡高 `140` 随内容增长、位置仍在可视区内（`rect [781,744,264,140]`）。1920×1080 同款两行完整展示。截图 `round3-01-node-card-full-title-1440x900.png`、`round3-04-node-card-full-title-and-band-1920x1080.png`。

### B-2 【话题带】成员行上方的强调色横条与刻度线已移除，标签与中性基线保留【pass】
实读：`.lcc-band-bar` 命中 **0**、`.lcc-band-tick` 命中 **0**；`.lcc-band-title` 文本仍为「本泳道话题带」、撕下泳道名「AGENT编程工具相关」；`.lcc-track` 存在且 `stroke = rgba(26, 26, 26, 0.15)`（中性边框色，与候选区 / 只读参照区的分区线同色同形）。视觉对照：旧截图的橙色圆角横条与垂到成员的橙色虚线在本轮已不存在。截图 `round3-03-topic-band-no-accent-bar-1440x900.png`、`round3-04-node-card-full-title-and-band-1920x1080.png`。

### B-3 【取消聚焦】轻点画布空白处 → 节点卡收起、顶部「取消聚焦」置灰；点卡片本体不误关【pass】
1440×900：聚焦 `9270`（卡在场）→ 鼠标落点 (890,648)（`elementFromPoint` 已确认非节点/非卡片/非图例/非缩放条）按下并松手 → `[data-testid=lane-node-card]` **消失**，顶部「取消聚焦」按钮 `disabled=true`（即 `focusId=null`）。1920×1080 重放同组操作（落点 (1000,700)）结果一致。
**不误伤**：聚焦后点击卡片本体（(900,780) 落在卡矩形内）→ `cardStillThere=true`（卡体已接住点击，不再穿透到画布）；再次点节点可重新聚焦（卡重新出现）。截图 `round3-02-blank-click-unfocus-1440x900.png`、`round3-05-blank-click-unfocus-1920x1080.png`。
