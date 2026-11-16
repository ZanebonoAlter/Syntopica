<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: topic-graph, daily-report, semantic-board -->

## Why

用户报障「版块创建完成后就没地方编辑」。挖下去发现是三层问题叠在一起，而且真正的病根在设计层：

**症状一 · 编辑入口埋得极深，等于没有。**
版块名/描述的编辑实际在 L1 顶栏折叠菜单 `<details>`「板块管理」→「板块设置 / 删除」→ 1040px 工作面的第 7 个页签；雷达列表页（版块列表所在处）零入口；另有三套僵尸实现从未接线（`AddSemanticBoardDialog.editMode` 无调用方传参、`BoardEditDialog.vue` 全仓无 import、`BoardGovernanceDrawer.vue` 全仓无 import）。

**症状二 · 「调质心」没有人工通道，而且就算加了通道也白加。**
`board_persistent_topics.centroid` 是**纯派生缓存**：全仓写入点只有 3 处且全是系统侧（`daily_report_topic_repository.go:1055` 每次 SaveReport 后无条件 UPDATE、一次性迁移 SQL、backfill 内部 running-mean），无 HTTP 端点、无 pin/锁定机制。`is_vacuum` 同理（唯一写入点 `:1165` `RecomputeVacuumStats`）。算法判定链里读它们的只有 L1/L2/L3 分桶（`daily_report_lane.go:116-124`、`:197-200`），关系重建与真空统计都不读——所以**重心每天被系统重算覆盖，人工写不进去，写了也会被冲掉**。

**症状三 · 「移除误入内容」的粒度最粗只到整条泳道。**
`UpdateSectionTopicAssignment`（`daily_report_topic_repository.go:448-473`）全仓仅 3 个系统侧调用者，**没有 HTTP handler 把它从当前话题摘出来**。用户只能整条 `MergeTopics`（`:651`）或拆出新建 `SplitTopic`（`:728`），无法把单条内容从这条泳道移除。

**根因**：系统里能持久写的人工状态（`topic.status` / `topic.label` / `ai_settings` 阈值）与算法每次重算的派生缓存（centroid / is_vacuum / 泳道快照）之间，**缺一条「人工修正 → 算法判定」的通路**。人工改了，算法不读；算法读了，人工改不动。

**为什么现在做**：用户已经在日常使用中撞上这个问题——泳道串味、名字不准，但没有任何可操作的出口。同时 `anchor-topic-centroid-with-embedding`（2026-11-12 归档）刚把质心公式换成「首义锚定 + 滚动均值」混合并留下 `POST /api/daily-reports/recompute-topic-centroids` 一次性重算入口，**公式层已经准备好，缺的是人工层的定义面与内容面**。现在补上人工层，能直接复用既有的每日自动重算机制，不需要任何"立即重算"的新写路径。

## What Changes

### 核心 1 · 人工重设泳道首义（定义面）

`board_persistent_topics.embedding`（首义向量）是质心公式里权重 α=0.5 的那一半，也是自动话题唯一的定义来源（自动话题首义 = **第一条命中 section 的向量**，`daily_report_assignment.go:308`）。

- 新增「重设首义」写端点：人工显式改写首义，输入为**手动选定的一组话题节点**（节点范围），服务端按 mean pooling 聚合为新首义（与手动建话题同口径）。用户对「这条泳道是什么」的描述，其可操作形态就是「哪些节点定义它」——画布上看得见当时选了哪些节点。
- **留痕**：记录改前值、改后值、时间、操作者；首义从「创建后不可变」改为「算法不得改写，人工可显式改写且留痕」。
- **不立即重算**：写完等次日日报生成的 `UpdateCentroidOnSectionChange` 自然生效（α=0.5 的混合机制本就每日滚动）。
- 效果：定义面校正后，重心被拽回正确方向，**漂移速度同时减半**（一半权重锚在人工定义的稳定向量上）。

### 核心 2 · 移除泳道窗口内的误入节点

质心的滚动项 = **最近 30 条命中 section embedding 的等权均值**（`persistent_topic_centroid_window` 默认 30，按 `period_date DESC` 取，`daily_report_topic_repository.go:975-990`）。移除节点即把这些 section 从该泳道摘出，滚动窗口自然收缩。

- 新增「归属变更」写端点：`PATCH /api/semantic-boards/../sections/:id/assignment`，`topic_id=null` 表示把 section 从当前话题**解除归属**（`persistent_topic_id` 置空，**不再归属任何泳道**，不需要指定去处），`topic_id=N` 表示把一个当前无归属的 section **加回**某条泳道（含本次刚移除的节点）。两个方向共用一个端点，内容**永远不删，以后可再参与候选**。
- 新增「批量移除某日」端点：一次摘掉某一天误入该泳道的全部节点。
- 快照列口径对齐既有硬删话题时的解绑（`daily_report_topic_repository.go:371-381`）：`persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `topic_status_at_report` 写为 NULL，`lane_tier` 保留原值。
- 事后动作复用既有链路：`RebuildBoardRelations`（`daily_report_matching.go:298`，整板重建，非增量）——identity 边由 `persistent_topic_id` 派生，解除归属必改。
- **派生值不立即刷新**：centroid / is_vacuum / 泳道快照全部等次日日报自然重算（与用户诉求一致：修正只影响往后，不重写已有输出）。

### 衍生 1 · 泳道修正工作面（画布）与入口接线

- **主战场是编排画布，不是弹窗**——既有泳道编排画布（`workshop-lane-compose`）新增「修正模式」：进入一条既有泳道，铺开它的现有成员节点。画布上能「看得见当时选了哪个节点」，也能把节点挪到别的话题。
- **左栏承载全部信息与操作**（本泳道成员 / 关联推荐 / 首义范围三块纵向堆叠，380px 固定），**中央画布只做可视化**（点节点 = 选中查看），**无右栏**。
- 画布承载三个动作：移除节点（取消勾选 → **直接移除，不选去处**）、加入节点（勾选 / 关联推荐加入）、设定首义范围（勾选一组节点）。
- 弹窗降级为**轻量单字段操作**（泳道重命名 / 归档、版块编辑），不再试图在一个弹窗里解决全部问题。
- 矩阵行新增「⋯」就地操作（解除 `BoardMatrixView.vue:9-10` 的只读红线，该红线是本 change 显式修改对象）。
- 「话题管理（候选人工激活）」更名为「泳道管理」——它本来就管 persistent topic（泳道实体），旧文案指向的是候选激活这一个子功能。
- 雷达行新增「⋯」菜单（版块编辑 / 归档入口）——补上"版块列表页零入口"的缺口。

### 衍生 2 · 版块编辑入口的诚实化修复

`PUT /api/semantic-boards/:id` 改了 label/description 后：
- 重算了 board embedding（`board_crud_handler.go:287-295`）✓
- **但没有失效匹配缓存** ✘ —— 全仓 `InvalidateMatchCache()` 生产调用点只有 2 处（`:431` 删 composition、`:1144` 加 composition）
- **而 board embeddings 缓存无 TTL**（`semantic_board_cache.go:128` 自陈「embeddings 缓存无 TTL，不失效则匹配/回填持续用旧数据」）
- **也没有重跑匹配** ✘ —— 旧 `topic_tag_board_labels` 行留存

后果：**改完版块名，方向校验（绝对门 0.5 / 相对门 0.15 / gap 豁免 0.10）仍在用旧向量**。本 change 修掉缓存失效（`updateSemanticBoard` 补 `InvalidateMatchCache()`），使暴露出来的编辑入口是**诚实**的；匹配表重跑保持手动（复用已有 `POST /api/semantic-boards/backfill?mode=board` 后台 job），不在本 change 自动触发——自动重跑会牵动共享辅助标签描述修复链的跨版块影响面（`semantic-board.md` 红线 26）。

### 本 change 范围外（后续可选）

- per-topic 的 α / L1 阈值覆盖（把 `ai_settings` 现有全局键 per-topic 化）
- 只读试算（用新首义 + 新内容集合模拟一遍会怎么分桶）
- 排除样本护栏（用户点某条误入内容 → 记下它的"长相" → 以后近似的都不许进这条泳道）
- 版块层面的构成标签修正（误入根因可能在 composition 的泛化词，但那属于版块层）

## Capabilities

### New Capabilities
- `lane-manual-correction`: 泳道人工修正闭环——重设首义（人工选定节点范围定义首义 + 留痕 + 不立即重算）与归属变更（section 解除归属 / 加回 / 批量移除某日），含写端点契约、快照列口径与派生值刷新时机
- `lane-correction-surface`: 泳道修正工作面（全屏画布，主战场）与入口接线——画布两栏（左栏承载移除/加入/首义范围，中央画布只做可视化）、轻量弹窗只管单字段、矩阵行与雷达行入口接线
- `workshop-lane-compose`: 既有泳道编排画布新增「修正模式」——进入既有泳道、沿用既有画布纪律（查看与选入分离、拖拽平移不误伤点选、窄屏禁用编排）、新建模式草稿不被修正模式污染

### Modified Capabilities
- `persistent-topic`: 首义向量由「创建后不可变」改为「算法不得改写、人工可显式改写并留痕」；新增 section 手动解除归属的归属场景；质心滚动窗口随人工移除自然收缩（不新增立即重算路径）
- `board-lane-dynamics`: 矩阵由只读解除——泳道行可就地触发编辑与移除操作（移除现有只读红线）
- `board-editing-ui`: 新增雷达行的版块编辑/归档入口（补上版块列表页无编辑入口的缺口）
- `board-match-cache`: board embeddings 缓存的失效条件从「仅 composition 变更」扩展到「版块 label/description 变更」，消除改完版块名后方向校验仍用旧向量的缺陷

## Impact

**后端（`backend-go/internal/topicgraph/`）**
- `handler/daily_report_handler.go` — 新增重设首义、移除节点、批量移除某日三个写端点
- `repository/daily_report_topic_repository.go` — `UpdateTopic` 扩展（首义 + 留痕）；新增「解除归属」repository 方法；`ComputeTopicCentroid` 的滚动窗口 SQL 不变（移除后自然收缩）
- `repository/daily_report_manual_topic.go` — 复用 mean-pooling 聚合逻辑作为「选内容定首义」的输入
- `repository/daily_report_matching.go` — `RebuildBoardRelations` 复用不改（整板重建语义不变）
- `handler/board_crud_handler.go` — `updateSemanticBoard` 补 `InvalidateMatchCache()`（1 行）

**前端（`front/app/features/tags/`）**
- 新增泳道修正工作面组件（`components/console/`）+ `components/dialog/` 内的轻量弹窗
- `components/console/ObservationMatrixLaneRow.vue` — 行头加「⋯」就地操作
- `components/dialog/TopicManageDialog.vue` — 更名「泳道管理」，补重设首义入口
- `components/console/BoardRadarStory.vue` — 行内加「⋯」菜单
- `components/console/BoardSettingsForm.vue` — 复用现有版块本体编辑表单（不重写）
- `api/dailyReports.ts` / `api/semanticBoards.ts` — 新端点客户端

**数据模型**
- `board_persistent_topics` 新增首义改写留痕字段（改前值 / 改后值 / 时间 / 操作者）+ 首义改写次数
- `daily_report_sections` 新增人工移除留痕字段（来源话题 / 移除时间）——用于可追溯「这条内容为什么被移出」
- 无破坏性变更，无数据迁移需求（旧数据首义照旧视为人工未改写）

**红线影响**
- `topic-graph.md` 红线 17 首句「首义 = 创建时写入、之后不可变」**需改写**为「算法不得改写；人工显式改写允许且须留痕」——这是本 change 对既有红线文本的**唯一一处修改**，修改后仍保留「质心漂移有界」的原意
- 红线「快照不得回填」「归档只能由用户显式操作」「planLifecycle 不自动改 status」均**不动**
- 红线 26（共享辅助标签描述改写影响其它版块归属）不动——本 change 不自动重跑匹配表

**验证**
- 后端：受影响包 `go test ./internal/topicgraph/... ./internal/tagmanagement/...`
- 前端：泳道编辑面板与三处入口的组件测试 + `pnpm exec nuxi typecheck`
- major UI 验收：`ui-verification/report.md` 建账 + 1440×900 与 1920×1080 双视口
