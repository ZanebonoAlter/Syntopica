# lane-correction-surface 过期审计

- 审计对象：`openspec/specs/lane-correction-surface/spec.md`（718 行 / 19 Requirement）
- 代码基线：HEAD `c0d67874`（含 `ad6e1841` align-lane-correction-workface、`7ed32c5d` unify-lane-canvas-shell）
- 判定口径：spec 的具体主张（路由 / 组件 / 布局模式 / 尺寸断点 / 手势 / 端点字段 / 可见性 / 数值）逐条对当前代码

## 总览

- Requirement 总数 **19**
- **有效 15**：R2 R4 R5 R8 R9 R10 R11 R12 R13 R14 R15 R16 R17 R18 R19（15 + 4 = 19，全部核对过代码）
- **过期 4**：R1（三处子断言：列宽 / 块数 / 三态）、R7（行距数值全错）、R6（内容归属错位 + 漏修正侧曲线）、R3（代码已越界两处文案）
- **无法核实 1**：R10 的像素高度（260–300px → 32–36px），行为完全有效，只有这组数字没实测
- 一句话结论：**骨架还能当契约用，数值不能**。三栏工作面、三类写入口、拖拽纪律、首义只读块、吸顶外壳都与代码严丝合缝；但 4 条数值/结构断言已漂移，其中「行距 44px」「左 380/右 300 固定」两处照抄会直接做出缺陷。
- 总纲段（1–174 行）单独评价：**可验证契约，不是设计说明**——动作表逐格能指到组件、写入口能指到 emit/API、窄屏门限能指到 768。但它承载 3 条已过期子断言：14 行列宽、41/49/103/161 行「四块」、31 行「三态」。
- Purpose 是 `TBD - created by archiving change lane-manual-correction-loop`（第 4–5 行）：归档遗留，不算过期问题，但应当补。

## 过期条款（按严重度）

### E-1 [严重] 泳道修正工作面是修正操作的主战场 — 三栏宽度写死 380 / 300

- spec 说：`spec.md:14`「**左栏 380px 固定**｜中央 SVG 画布自适应｜**右栏 300px 固定**」；`:49` 场景重复同一组数字。
- 代码实际：`BoardComposeOverlay.vue:860-862` `.bco__workbench--correction { grid-template-columns: 380px minmax(0,1fr) 300px }`；但 `BoardComposeOverlay.vue:1029-1033` `@media (max-width:1279.98px){ .bco__workbench { grid-template-columns: 240px minmax(0,1fr) 280px } }`——同特异性、源码在后，**在 ≤1279.98px 覆盖修正模式**。而修正模式在 768–1279px 照常渲染（窄屏门限 768：`BoardComposeOverlay.vue:323-326`）。
- 判定依据：窄档规则来自 `4d79ea5b`（2026-11-02），380/300 定稿于 `e33821d3`（2026-11-17）——spec 从写下那天起就只在 ≥1280px 成立，且从未记录该断点。
- 影响：照 spec 做 `layout.md` 的「各栏 min/max」登记会写错；若有人「对齐 spec」把 380/300 改成无条件，1024 宽视口中央画布只剩约 300px，7 天档列宽跌破 64px 下限（`LaneCorrectionCanvas.vue:357`），画布常年横向滚动。

### E-2 [严重] 同一区同一日期不重叠 — 行距与占高数值整体漂移，spec 的「口径警示」现在是错的

- spec 说：`:310`「行距 = 圆直径 + 标签起笔间距 + 标签最多行数 × 行高 + 4」；`:324`「成员半径取固定基准常量、两行标题时为 **40 + 4 = 44px** 量级」；`:312` 警告「圆心→标签底是 **34.5**，按它算得 **38.5**」。
- 代码实际：`LaneCorrectionCanvas.vue:161` `MEMBER_BASE_RADIUS = 7.5`（不是 5.5）；`:194-195` `LABEL_BOX_PAD = 4` + `NODE_BLOCK_H = 7.5×2 + 5 + 24 + 4 = 48`；`:376` `nodeRowGap = max(16, 48+4) = 52`。
- 判定依据：`5.5 → 7.5` 与 `LABEL_BOX_PAD` 都随 `ad6e1841`（2026-11-25）落地，同提交更新了 spec 其它段却没动这两处；连代码自己的头注释都还写着「半径恒为 5.5 / 44px」（`LaneCorrectionCanvas.vue:9-11`）与「NODE_BLOCK_H（40）」（`:407`）。
- 影响：**最危险的一条**。spec 的 44 比实际 52 少 8px，照它实现会把行距压到 44 < 占高 48——正好复现它自己在 `:312` 警告的「标签框底与下一圆顶相切」缺陷，而警示里那组对照数（34.5 / 38.5）现在两处都不成立，读者无法自证。

### E-3 [中等] 左栏「四块」实为五块（缺「辅助标签约束」）

- spec 说：`:14`、`:41`、`:49`、`:103`、`:161`「左栏 SHALL 只承载「已选侧」**四块**（本泳道成员 / 更早成员 / 已移出（本次）/ 首义范围）」。
- 代码实际：`BoardComposeOverlay.vue:462-505` 左栏依次 `LaneMemberList`（成员 / 更早成员 / 已移出）→ `LaneAnchorScope`（首义范围）→ `LaneConstraintScope`（`LaneConstraintScope.vue`，辅助标签约束，默认折叠、逐条落库）；aside 的 `aria-label` 已写「…与辅助标签约束」（`:466-467`）。新建模式左栏同样有第五块（`:574`）。
- 判定依据：约束块由 change `topic-lane-auxiliary-label-constraint` 引入；`LaneConstraintScope.vue:7` 注释仍写「左栏已有…四块」，连代码注释都停在四块时代。
- 影响：`:161` 的「三栏内容不重复」是验收条款，按 spec 数块会漏掉第五块；这个块因此没有归属条款可依（见 G-1）。

### E-4 [中等] 「首义来源三态」不是一条字段

- spec 说：`:31`「其状态 SHALL 为**三态**：人工 / 自动·创建时写入（不可考）/ 自动·来源可考」；`:177` 场景要求三者「呈现为三种可区分的来源状态」。
- 代码实际：两个独立字段——`source` 只有 `auto|manual`（后端 `daily_report_anchor_read.go:22-28`；前端 `useLaneCorrection.ts:209`），另有 `anchor_origin.resolved` 布尔（同文件 `:39-47`）。来源行文案只有两句（`useLaneCorrection.ts:408-412`）：`人工（选定的 N 个节点）` / `自动（创建话题时写入）`；可考/不可考落在**另一行**（`LaneAnchorScope.vue:182-192`：resolved → 「标题 · 日期」，否则「创建时写入 · 来源不可考」）。
- 判定依据：`lane-ops:23` 已把 `anchor_origin.resolved` 定为唯一语义开关，与 `source` 正交；本 spec 把两者压成一条「三态」。
- 影响：照三态实现会去给 `source` 加第三个枚举值（后端没有、也不会下发）；反向也可能只读来源行、以为自动首义必有节点列表。

### E-5 [中等] 「新增泳道的时间视窗曲线口径」装着两件不属于本 spec 的事，且漏了修正模式自己的曲线

- spec 说：`:277`「**新建泳道**工作面顶部的时间视窗…口径为全部可选线索数…与 `compose-candidates` 同源」；`:299-305` 结尾却挂了个「相关节点自动高亮」场景（讲的是画布聚焦高亮，与曲线无关）。
- 代码实际：新建侧曲线在 `useLaneCompose.ts:488-525`（`getComposeCandidates(..., {days:365, omitEmbeddings:true})`）+ `BoardComposeOverlay.vue:404-422`；修正侧曲线是**另一条数据源**（lane-dynamics 目标泳道 365 日，`useLaneCorrection.ts:735-780`，含「曲线未知 / 加载失败 · 重试历史池」三态，`BoardComposeOverlay.vue:379-401`）——spec 对它一字未写。
- 判定依据：`workshop-lane-compose` 已持有编排态全部曲线与候选池口径；本 spec 复述一份必然漂移。
- 影响：读者以为本 spec 管着新建编排的曲线；同时修正侧曲线（会误报「曲线未知」的那条路径）无契约可依（见 G-2）。

### E-6 [轻微] 产品界面不承载解说性长句 — 代码已越界两处

- spec 说：`:205-221` 界面只留动作标签 / 状态结论 / 必要确认与反馈。
- 代码实际：`LaneAnchorScope.vue:199`「用下方的节点定一次首义即可留痕。」（节点卡式教学语）；`BoardComposeOverlay.vue:399`「本泳道曲线未知（目标泳道未返回；不影响成员与首义）。」（解释机制）。
- 影响：方向没变，但照 spec 做 UI 验收会判不过；这类句子每加一句都要重新解释为什么它合规。

### E-7 [轻微] 轻量弹窗表漏项 + 自相矛盾

- spec 说：`:183-203` 三行表（泳道重命名 sm / 泳道归档 sm / 编辑版块 md）+「危险操作 SHALL 用 `useConfirm()` 二次确认（归档版块 SHALL 沿用「手打版块名」）」。
- 代码实际：`LaneArchiveDialog.vue:77-104` = sm + `useConfirm`（符合）；`BoardQuickEditDialog.vue:112-115` = md + 名称/描述/排序/受保护（符合）；**版块归档** `BoardArchiveDialog.vue:102-105` = **md 560** + 手打版块名、**无 useConfirm**——表里没有这一行，且「用 useConfirm」与「手打名」两句在同一段里互相否证。
- 影响：轻微；新增弹窗时无法从表里判断该用哪档、该走哪种确认。

## 与 workshop-lane-compose / lane-ops 的重复条款（重复即碎片）

| 本 spec 条款 | 重复对象 | 建议归属 |
|---|---|---|
| `:616` 修正画布高度贴合容器（`max(容器高, 内容高)`、多余高给候选/只读区） | `workshop-lane-compose:510` 编排画布高度贴合容器（同一公式、同一条吸顶判据） | 公式与判据归 workshop（写成两画布共用），修正侧只留差异「成员区不因富余高度下移」 |
| `:661` 修正画布横向吸顶标签列（`vbWidth×zoom > clientWidth` 门控 + 不透明底 + 右缘渐隐） | `workshop-lane-compose:547`「吸顶标签列是两支画布共用的单元」（门控、底色、阴影级数逐字重复） | 机制归 workshop；修正侧只留「宽度取 `axisX0`（含折叠带 68+8）」这一条本地参数 |
| `:682` 纵向吸顶日期轴 | `workshop-lane-compose:541-543` 明说「通栏吸顶带底」不属共用单元 | 保留在本 spec（唯一出处），但把门控那句改成引用共用单元 |
| `:701` 缩放/滚动外壳与编排画布同源 | `workshop-lane-compose:547`（同一实现）+ `:487` 拖拽期间文本选择隔离 | 保留「同源」一句 + 引用，不重述手势细节 |
| `:57`、`:147` 窄屏 <768 禁用 | `workshop-lane-compose:388`「窄屏禁用编排」 | 合成一条「两支画布共用 768 门限」，两处只留提示文案差异 |
| `:252` 展示字段溯源（成员快照 / 候选批内相对归一） | `workshop-lane-compose:395` 批内相对归一（含「SHALL NOT 距离÷阈值」同一论证） | 归一算法只写一处（`relativeProximities`），两画布引用 |
| `:565` 首义读端点零写副作用 | `lane-ops:29` 同一条（写表/质心/待重算/回填全列） | 数据契约归 lane-ops，本 spec 只留「读失败不阻断写入口」 |

## 代码有但 spec 没写

### G-1 左栏第五块「辅助标签约束」
- 代码：`LaneConstraintScope.vue`（修正模式 `BoardComposeOverlay.vue:501`；新建模式草稿 `:574`）；后端 `daily_report_handler.go:89-92`（`GET/PUT /api/daily-reports/topics/:id/constraints`）
- 建议：加一条「左栏附属块」条款——默认折叠、块头「次日日报生效」徽标、≤1280px 窄栏下的堆叠顺序、与「四块」的从属关系

### G-2 修正模式 365 日 lane-dynamics 本泳道曲线
- 代码：`useLaneCorrection.ts:735-780`（`reloadCurve`，目标泳道未返回 → 「未知」不造全 0）；UI `BoardComposeOverlay.vue:379-401`（caption「365 日 · 本泳道逐日线索数（历史参考）」、未知态、失败可重试）
- 建议：与 E-5 一起处理——新建侧曲线搬走，修正侧曲线写进来

### G-3 候选质心池含「更早成员」向量
- 代码：`useLaneCorrection.ts:308-329`（质心池 = 窗口内 sections + 更早成员的 embedding；注释明说「否则窗口内成员为 0 时质心为空、候选区全空」）
- spec `:38` 只写「对**当前有效成员** embedding 均值」
- 建议：补一句「质心池 = 窗口内有效成员 + 窗口外成员（向量可用者）」

### G-4 节点卡的第三个写入口「↩ 加回本泳道」
- 代码：`LaneCorrectionCanvas.vue:1438-1444`（`cardWriteKind === 'restore'`）
- spec 动作表 `:18-24` 只给节点卡「＋加入 / ⊘移出」，加回只写在左栏
- 建议：动作表与 `:31` 的「写入口三类」补全为三态卡

### G-5 块内生效徽标（归属·立即生效 / 重心·次日重算）
- 代码：`LaneMemberList.vue:110,161,208`、`LaneAnchorScope.vue:170`、`LaneConstraintScope.vue:114`
- spec `:431-458` 只规定**顶部标题行**三枚徽标
- 建议：明说块头徽标是顶部结论的就地重复（允许），或收敛到只保留顶部

## 建议

1. **该改**（按优先级）：E-2 数值（44 → 52，34.5/38.5 作废，并补 `LABEL_BOX_PAD`）→ E-1 列宽（补 ≤1279.98 档 240/280，把「固定」改成「≥1280 固定」）→ E-3 四块 → 五块 → E-4 三态改成「`source` 二值 + `anchor_origin.resolved`」两字段口径。
2. **该搬**：`:275-305` 整条搬到 `workshop-lane-compose`（曲线口径本就在那儿验），其中 `:299` 的「相关节点自动高亮」搬回 R1 的相关高亮条款；R15/R17/R19 收敛成对共用口径的引用。E-6/E-7 属于「改代码或改 spec」的二选一，建议先修文案。
3. **该补**：G-1、G-2 是真缺（有 UI、有数据路径、无契约）；G-3..G-5 是一句话补全。
4. **728 行该拆，建议分界**：`工作面结构 / 交互`（R1、R2、R3、R4、R12、R13、R14、R16）与 `画布几何 / 外壳`（R7、R8、R9、R15、R17、R18、R19）。理由：后一簇全是「同一日 / 同一列 / 不越界 / 吸顶 / 高度」的纯排版契约，与语义修正无关，且与 `workshop-lane-compose:395/510/547` 同源——拆开后三处重复条款才有地方合并。R5 归 `lane-ops`，R6 归 `workshop-lane-compose`，本 spec 最终约 8 条、300 行上下。
