<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: semantic-board, topic-graph, daily-report -->

# Proposal: 泳道工作面对齐——画布节点表达统一 + 聚焦可快速加入移出 + 相关推荐自动高亮 + 顶部时间视窗

## Why

用户实操反馈三件事，指向同一个工作面（泳道编排/修正工作台）：

1. **新建泳道与编辑泳道的画布交互不一致**。两套画布组件各自演化：新建泳道画布（`components/compose/LaneComposeCanvas.vue`）是关系图 + 节点卡内「＋选入/移出」当场改；编辑泳道画布（`console/LaneCorrectionCanvas.vue`）是时间轴散点 + 「画布不承担写操作」，点节点只改选中态，加入/移出必须回左栏。用户明确要**新建泳道那种形式**。
   - 已核实：缩放/平移/1:1 像素/节点卡跟随**已经同源**（共用 `composables/laneComposeLogic.ts`），手感层不是问题；不一致在**节点表达**、**写操作落点**、**相关推荐高亮**三层。
2. **泳道工作面顶部没有时间窗口选择**。矩阵页顶部有完整「时间视窗」（档位 7/14/30/365 + 起止日表单 + 刷选曲线 + 平移/缩放），泳道工作面顶部只有一个 `近7天/近14天/近30天` 下拉 + 一行文字标签，能力差一大截。
3. **口径**：时间视窗曲线现在画的是「A 名单里所有泳道的线索合计之和」（板级），放进单条泳道的工作面语义割裂。

## What Changes

### 画布节点表达统一（新建泳道 ⇄ 编辑泳道）

两套画布的节点 SHALL 用同一套表达规则，跨工作面一致：

| 节点身份 | 表达 | 说明 |
| --- | --- | --- |
| 本泳道成员 | 实心、尺寸固定 | 尺寸不编码接近度（避免同屏两套尺子） |
| 候选 / 关联推荐 | 空心或弱填充、尺寸按接近度映射 | 与新建泳道一致 |
| 只读参照（属其它泳道） | 更弱、更小 | 不与候选混淆 |
| 已移出（本次） | 划除线 | 与成员区分 |
| 首义节点 | 虚线标记 | 沿用既有首义虚线样式 |

### 工作面结构：两栏 → 三栏（用户 2026-11-16 拍板）

编辑泳道工作面 SHALL 从两栏改为**三栏**：左 380px（已选侧：本泳道成员 / 更早成员 / 已移出 / 首义范围）｜中央画布自适应｜**右 300px（待选侧：关联推荐）**。左栏 SHALL NOT 再承载关联推荐，两侧内容不重复。

理由：`lane-manual-correction-loop` 的 v3 曾以「左右重复」为由删右栏，但工具链演进后左栏已承担 4 块，再塞推荐会让左栏 5 块纵向堆叠、每块独立内滚；右栏「待选」与左栏「已选」形成对照，也与新建泳道「左已选、右待选」结构对齐。

### 聚焦后快速加入 / 移出（画布内写操作）

聚焦任一节点后，节点卡 SHALL 直接提供「＋加入本泳道」/「移出本泳道」动作，当场生效（与新建泳道节点卡一致）。左栏 SHALL 保留同一份操作入口与同一份勾选态（双向同步，不产生第二份状态）。

> **契约变更前置**：`openspec/specs/workshop-lane-compose/spec.md` 的「查看与选入分离」Scenario 明确「点节点只改变选中态，SHALL NOT 改变成员数」。本 change SHALL 先改该契约，再改代码。

### 相关节点推荐自动高亮

聚焦某节点时，与之相关的节点 SHALL 自动高亮，其余降为弱化：相关 = ① 与该节点有原关系（1 跳）∪ ② 对当前泳道质心较近的推荐节点（与新建泳道「高亮当前关联」同判据）。取消聚焦 SHALL 恢复默认呈现。

### 顶部时间视窗（内容与矩阵顶部一致）

`console/ObservationRangeControls.vue`（纯受控 UI，不请求 API、不写 router）SHALL 原样复用到泳道工作面顶部：档位按钮、起止日表单 + 「应用范围」、刷选曲线、平移/放大/缩小、键盘逐日/Shift 逐周。

**曲线口径按场景分两种**：

- **编辑泳道**：画**本泳道自己**的逐日线索数（泳道级）。数据现成——`GET /api/semantic-boards/:id/lane-dynamics?days=365` 响应自带逐泳道逐日 `clue_count`，前端取当前泳道那一列，**后端零改动**。
- **新增泳道**：泳道尚不存在，画**全部可选的线索数**（可选池中**不属当前 active 泳道**的节点，逐日聚合），语义是「现在可编排的内容有多少」。

**提交语义**：泳道工作面是模态层，SHALL NOT 写 URL query（避免污染矩阵页的 `days`/`start`/`end` 深链），走模态内本地状态。

**档位对齐**：泳道侧取数端点 `compose-candidates` 原上限 90 天（`composeWindowMaxDays`）。**用户 2026-11-16 裁决：放宽到 365**，使新增泳道能看到一年真实可选池。实测（最大版块）90 天 620 条 section / ≈6.2 MB → 365 天 1,035 条 / ≈10.4 MB，成本可接受；但曲线路径 SHALL 走**不带向量的轻量取数**（可压到 ≈200 KB），不得为画曲线拉全量向量。详见 design OQ-2 定案。

## Capabilities

### New Capabilities

（无。）

### Modified Capabilities

- `workshop-lane-compose`: 画布节点表达规则、聚焦后画布内加入/移出、相关节点自动高亮、编排侧「距离」→「接近度」同向统一。
- `lane-correction-surface`: **该能力当前不在主 specs**（只在未归档 change `lane-manual-correction-loop` 的 delta 里）。**前置 P1 = 先归档 `lane-manual-correction-loop`**，把该能力落进主 specs，本 change 再对它出 delta。届时需改两条：①「画布 SHALL NOT 承担写操作」→ 改为节点卡可承担显式写操作（左栏仍保留同一份入口与同一份状态）；②「SHALL NOT 有右栏」→ 改为三栏。在 P1 落地前，本 change SHALL NOT 写该能力的 delta（现在写 MODIFIED 会因主 specs 无此能力而 validate 失败）。

## Prerequisites

- **P1（阻塞 delta spec）**：归档 `lane-manual-correction-loop`（68/68 已完成、实现仍在工作树）。归档后 `lane-correction-surface` 进入主 specs，本 change 才能对它出 delta 并改「查看与选入分离」。
- **P2（依赖）**：`fix-lane-metric-provenance`（数字口径收敛）先行落地并 commit —— 两条 change 在 `useLaneCorrection.ts` 等文件上重叠，必须串行。

## Impact

**前端**
- `front/app/features/tags/components/console/LaneCorrectionCanvas.vue`：节点表达、节点卡写操作、相关高亮。
- 右栏关联推荐（**从 `LaneMemberList.vue` 的「关联推荐」分组迁出为独立右栏组件**）：`LaneMemberList.vue` 缩为三分组 + 新增右栏组件。
- `front/app/features/tags/components/compose/LaneComposeCanvas.vue`：节点表达对齐 + 相关高亮（既有「高亮当前关联」需对齐判据）。
- `front/app/features/tags/components/console/BoardComposeOverlay.vue`：两种模式的顶部时间视窗挂载（写作窗口/本地 state）。
- `front/app/features/tags/components/console/ObservationRangeControls.vue`：复用（不改组件本体，除非要支持泳道级曲线）。
- `front/app/features/tags/composables/useLaneCorrection.ts` / `useLaneCompose.ts`：窗口状态（档位 + 起止）与曲线数据源。
- `front/app/features/tags/composables/laneComposeLogic.ts`：节点表达与相关高亮的共享纯函数。

**后端**（仅当 P1 决策为「泳道侧支持 365 档」时才需改）
- `backend-go/internal/topicgraph/handler/daily_report_handler.go`：`composeWindowMaxDays`（当前 90）与 `from`/`to` 参数。
- `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go`：可选池的逐日线索聚合（新增泳道曲线用）。

**文档**
- `docs/reference/api/semantic-boards.md`（如扩窗口上限）、`docs/reference/flow/semantic-board.md`（§12 变更溯源）。

**并发提示**：本 change SHALL NOT 与 `fix-lane-metric-provenance` 并行改同一文件；两者串行（先口径）。
