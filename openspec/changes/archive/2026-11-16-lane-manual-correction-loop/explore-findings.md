
## 修正：调质心=改首义+剔除节点（推翻早前「多锚点」方案）

⚠️ **本条修正了同会话早前存档的「多锚点 / 立即重算」方案——那个方案已被用户否掉，最终设计见下。以本条为准。**

## 最终结论（用户拍板）
「调质心」**不是独立功能**，而是「剔除误入节点」的副产品：
- **核心动作 1**：人工重设泳道首义（定义面）
- **核心动作 2**：剔除窗口内节点 = 改挂到**当天其它话题**（内容不删除，以后可再参与候选）
- 其余（编辑面板、入口接线）都是衍生

## 为什么「改首义」比「加第二个锚点」好
- 质心 = α·首义 + (1-α)·近 30 条命中 section 均值（α=0.5）
- **滚动项**：剔除节点 → section 不再计入窗口 → 次日自动重算 → 重心自然挪位。**零新机制**
- **首义项**：冻结列，删内容改不了它 → 需人工重设（选内容均值 或 写一句话向量）
- 首义只有 3 个读取点（`topic_repository.go:999` α项 / `:1034` 退化 / `topicAnchorVec` 兜底），爆炸半径极小
- 重设首义后**漂移速度减半**（一半权重锚在人工定义的稳定向量上）
- 仓库已有两处同款先例：`CreateManualTopic`（选 section 均值当首义）、`CreateWatchTopic`（人写的话当首义，且 Embedding+Centroid 同写）

## 关键约束（用户明确）
- **不做「立即重算」**——改事实，派生值让算法每天自己刷；不重写已有输出
- **不做人工直写向量**（会被次日覆盖）
- 重算重心**不动任何历史输出**（已核：centroid 只写一列；关系重建不读它；真空统计读历史 distance 快照；日报正文/泳道快照/雷达都不读它）

## 写这轮时补上的方案漏洞（原方案没覆盖）
**重设首义后，质心不一定会重算**——`SaveReport` 只对当日 touched 话题刷新，话题若无新内容命中就不进集合 → 新首义永远不生效。
**解法**：新增 `board_persistent_topics.centroid_pending_recompute` 布尔列，人工修正置位；质心刷新范围 = touched ∪ 待重算；重算成功清标记、失败保留。这既尊重「不立即重算」，又保证修正确实生效。

## 顺带挖到的真 bug（已进 change）
`PUT /api/semantic-boards/:id` 改 label/description 后**不调 `InvalidateMatchCache()`**，而 board embeddings 缓存**无 TTL**（`semantic_board_cache.go:128` 自陈）→ 方向校验仍用旧向量。全仓 `InvalidateMatchCache()` 生产调用点只有 2 处（删/增 composition）。

## change 已开
`openspec/changes/lane-manual-correction-loop` —— 5 制品全 done + validate 通过：
proposal（complex / ui-impact major / domains: topic-graph, daily-report, semantic-board）、ui-design（八节 + ui-prototype/index.html，approval=pending 待用户确认）、specs（7 个 delta：lane-manual-correction 新 / lane-correction-surface 新 / workshop-lane-compose 改 / persistent-topic 改 / board-lane-dynamics 改 / board-editing-ui 改 / board-match-cache 改）、design（13 条决策，结论/落点/取舍三行式）、tasks（12 组，尾节测试/文档/验证）。

**⚠️ 后续两次用户评审修正（以本条为准）**：
1. **主战场是编排画布，不是弹窗**。v1 的「泳道编辑面板」方案作废，改为既有泳道编排画布（`BoardComposeOverlay`）新增「修正模式」；弹窗降级为轻量单字段操作（改名/归档/编辑版块）。新增 `lane-correction-surface` 与 `workshop-lane-compose` 两个 delta 替换原 `lane-edit-panel`。
2. **首义 = 手动选定的话题节点范围**，不是写一句话、也不复用一句话追踪机制。用户对「这条泳道是什么」的描述，其可操作形态就是「哪些节点定义它」——画布上看得见当时选了哪些节点，服务端按这组节点的 embedding 取均值（mean pooling）。`PATCH /topics/:id/anchor` 的请求体只收 `section_ids`，传自然语言须 4xx。

design.md 里的 D1-D12 是完整决策集，比本条更详细。

<!-- pinned 2026-11-13T07:45:40Z -->

## 终稿设计：画布主战场 + 首义节点范围 + 归属双向变更 + 零解说

change `lane-manual-correction-loop` 的最终设计（历经 5 轮用户评审修正，本条为准）。

## 主战场：编排画布（不是弹窗）
- 既有泳道编排画布 `BoardComposeOverlay.vue` 加「修正模式」；**两栏**：左栏 380px 固定（承载全部信息与操作）+ 中央 SVG 画布自适应，**页面内不存在右栏**
- 左栏三块：本泳道成员 / 关联推荐（成员质心）/ 首义范围（定义面）；**无「当天其它话题」区块**
- 中央画布**只做可视化**（点节点 = 选中查看 + 节点卡跟随），写操作全部从左栏发起
- 弹窗只承载轻量单字段：泳道归档（sm 420）、版块编辑（md 560）、版块归档

## 首义 = 手动选定的话题节点范围（不是写一句话、不复用一句话追踪）
- 用户评审明确：「不是复用一句话追踪机制，而是能够手动选择首义的话题节点范围」
- `PATCH /api/daily-reports/topics/:id/anchor` 请求体**只收 `section_ids`**，传自然语言须 4xx
- 服务端 mean pooling 聚合（复用 `daily_report_manual_topic.go:aggregateEmbeddings`）
- 被选节点须：属本话题 · 同 board · embedding 存在且维度一致，否则**整体拒绝**（不静默跳过）

## 归属变更是双向的
- 端点 `PATCH /api/daily-reports/sections/:id/assignment`，body `{topic_id: number|null}`
- `null` = 移除（`persistent_topic_id`/distance/confidence/`topic_status_at_report` 四列置 NULL，`lane_tier` 保留）
- `N` = 加回（仅限当前无归属的 section；`confidence=manual`、distance 实算到目标锚点）
- **不提供「A 泳道 → B 泳道」改挂**（用户评审：「移除直接就是不归属任何泳道，不需要选择其他泳道改挂」）
- 「已移出（本次）」分组可「加回来」（用户评审：「已移出的本次应该也能再加回来」）

## 改名：画布标题就地编辑
- 既有画布底部已有「泳道名称」输入区（`lane-compose-name`/`nameDraft`）→ 修正模式沿用，**点标题文字即改，Enter 保存 / Esc 取消 / 失焦取消，不弹对话框**
- 「重命名弹窗」降级为「归档弹窗」

## 界面文字纪律（新增契约）
产品界面只允许三类文字：**动作标签 / 状态结论（徽标、空态、错误）/ 必要反馈（toast、二次确认）**。
禁止：操作口径并列解释、派生值机制科普、画布长图例、节点卡教学语。生效时机只用结论型徽标（「立即生效」「次日日报」）。解说内容只放**评审外壳**（原型顶栏）。

## 关键机制：待重算标记
新增 `board_persistent_topics.centroid_pending_recompute`。质心刷新范围 = 当日 touched ∪ 待重算。**否则话题当日无新命中时改了首义/移除节点永远不生效**。重算成功清标记、失败保留。

## 红线改动（唯一一处）
`topic-graph.md` 红线 17 首句「首义 = 创建时写入、之后不可变」→ 改为「算法不得改写；人工可显式改写且须留痕」。其余红线（快照不得回填 / 归档只能显式 / planLifecycle 不改 status / 红线 26 跨版块影响面）均不动。

## 顺带修的真 bug
`PUT /api/semantic-boards/:id` 改 label/description 后**不调 `InvalidateMatchCache()`**，而 board embeddings 缓存**无 TTL**（`semantic_board_cache.go:128` 自陈）→ 方向校验仍用旧向量。全仓 `InvalidateMatchCache()` 生产调用点只有 2 处（删/增 composition）。

## 制品
7 个 delta spec：lane-manual-correction（新）/ lane-correction-surface（新）/ workshop-lane-compose（改）/ persistent-topic（改）/ board-lane-dynamics（改）/ board-editing-ui（改）/ board-match-cache（改）。design.md 含「行为契约图解」5 张 mermaid（已用 mermaid 12.0.0 解析 + 浏览器渲染双验）。tasks.md 62 个任务，尾节测试/文档/验证。

<!-- pinned 2026-11-13T13:02:41Z -->

## v5 终稿：就地改名 + 界面文字纪律 + 左栏可滚（附原型实测 bug 手法）

change `lane-manual-correction-loop` 第 5 轮评审后的终稿补充（承接前一条终稿设计）。

## v5 两处新增
### 1. 泳道名就地编辑（主路径）
- 修正工作面**标题处**泳道名点文字即改：Enter 保存 / Esc 取消 / 失焦取消，**零弹窗**
- ⚠️ **我之前写错过一处**：既有编排画布的「泳道名称」输入在 **`AppDialog size="md"` 命名弹窗**里（`BoardComposeOverlay.vue:387-414`，标题「给这条线起个名字」），**不在画布底部**。修正模式只沿用同一 `AppInput` 控件形态，不另加底部输入区。
- 泳道管理列表另保留 `sm` 档重命名弹窗作次要路径 → 组件：`LaneRenameDialog.vue`（改名）+ `LaneArchiveDialog.vue`（归档）

### 2. 界面文字纪律（新增契约节）
产品界面**只允许三类文字**：动作标签 / 状态结论（徽标、空态、错误）/ 必要反馈（toast、二次确认）。
禁止四类：操作口径并列解释、派生值机制科普、画布长图例、节点卡教学语。生效时机只用结论型徽标（「立即生效」「次日日报」）。解说内容全部归**评审外壳**（原型顶栏折叠栏）。

## v5 暴露的原型 bug → 已补进契约
左栏三块纵向堆叠总高超屏时，首义面板被 `flex:0 1 auto` + `overflow:hidden` 压扁，**排在块尾的「用选中的 N 个节点定首义」按钮被裁到视口外且滚不到**（实测 btn top=627 > vh=633；`.wf__left` scrollHeight==clientHeight=380 → 假可达）。
契约已补：左栏 SHALL 整体纵向可滚；SHALL NOT 用「压缩某块 + overflow:hidden」腾空间；块内长清单各自内部滚动但**该块操作按钮 SHALL 始终可见**；新增 Acceptance A-10。

## 终检发现并修掉的残留
- `design.md` 留痕列名写错（`manual_reassigned_*` → `manual_assignment_at` / `manual_assignment_from_topic_id` / `manual_assignment_action`）
- `persistent-topic` spec 里「剔除/改挂节点」旧术语 → 「移除节点、加回节点」
- `workshop-lane-compose` spec 里「右栏 SHALL 展示…」 → 「左栏 SHALL 展示『本泳道成员 / 关联推荐 / 首义范围』」
- `lane-correction-surface` spec 里「WHEN 用户查看画布右栏」 → 左栏
- `ui-design.md` Prototype 节记全了 v1→v5 每轮修正的理由

## 端点终态
```
PATCH /api/daily-reports/sections/:id/assignment   body {topic_id: number|null}
PATCH /api/daily-reports/topics/:id/anchor        body {section_ids: [...]}
POST  /api/daily-reports/topics/:id/detach-date    body {period_date}
```

## 规模
7 个 delta spec · design.md 含 5 张 mermaid 行为契约图（mermaid 12.0.0 解析 + 浏览器出图双验）· tasks.md 62 个任务 · validate 全绿 · ui-approval 仍 pending。

<!-- pinned 2026-11-13T13:54:08Z -->

## v5.2 展示字段溯源：删「疑似串味」等无来源徽标，接近度口径锚定

用户评审定下的硬规矩（2026-11-13）：**原型/产品界面每个字段须有可解释来源，无来源的装饰性徽标一律不做**。本轮（v5.2）处置记录：

## 查实结论（全仓 grep 证据）
- 「疑似串味」：**全仓无任何计算字段**（只命中 prompt 文案/测试注释）→ 删。顶栏复合徽标「疑似串味 · 成员 4 条跨 2 天」整枚删除（「跨 2 天」还是写死字面量，重算都不更新）；泳道管理列表 `l.smell` fixture 列同删。
- 换马甲也不行：我把「疑似串味」换成「成员偏多」（members>5）后被自己否掉——任意阈值判断同样无口径，一并删。
- 「当前首义来源：自动（首条命中内容）」是**编造文案**：真实机制 = topic-graph 红线 17「首义 = 创建时写入、之后不可变」，人工改写走 `topic_anchor_overrides`。正确文案「自动（创建话题时写入）」。spec 的 GIVEN 同病同修。
- 「接近度」此前 spec 无口径、标签混用（推荐行「接近 .62」vs 节点卡「接近度 0.94」）→ 统一：**接近度 = 1 − 余弦距离**；成员取 `daily_report_sections.topic_match_distance` 换算，候选取对当前泳道质心 `board_persistent_topics.centroid` 实时计算；标签统一「接近度」。

## 契约落点
- ui-design.md 新增**「展示字段溯源」节**：字段→来源全表（现库/派生/交互态/契约语义/非数据五类），规则对齐 test-design ⑤问（展示字段来源语义须有 spec 锚）。
- 断言 **A-11**：产品界面数据字段 SHALL 命中溯源表；无来源装饰性徽标 SHALL NOT 出现。
- lane-correction-surface spec 新增 Requirement「展示字段数据溯源」+ 两 Scenario（无来源徽标不出现 / 接近度口径统一）。

## 有来源可保留的
「AI 前沿」版块徽标（`semantic_labels.label`）、「N 条」成员计数（窗口内 section 数，与列表同源）、timing 徽标（契约语义非数据）。

v5.1 sticky 吸底取舍已拍板保留（标准模式，滚到底解除遮挡，A-10 实测过）。浏览器实测：串味字样全消失、接近度统一、锚文案已换、A-10 无回归、溢出 0、validate 绿。ui-approval 仍 pending。

<!-- pinned 2026-11-13T14:30:47Z -->

## v5.3 更早折叠带：窗口外首义节点可见化（选择池+非比例折叠）

用户评审提出（2026-11-13）：首义节点常常早于 7 天窗口，画布上看不见 → 设计「更早折叠带」，v5.3 已落地原型并写入契约。

## 设计定稿（比"左侧截断"收紧半步）
- **只有人工 override 首义才有节点集**（`topic_anchor_overrides.section_ids`）。自动首义 = 建题向量，无节点集（`CreateManualTopic` 只落 embedding+归属，不存首义集）→ **不许虚构列表**（字段溯源纪律）。watch 首义 = 检索句向量，同样只文字标签。
- **画布左缘「更早」折叠带**：当前首义集含窗口外节点时，时间轴首日前显示固定窄列（68px）：虚线分隔 +「更早 · N」+ 首义虚线圆点纵向堆叠，**非比例折叠**（斜纹底 + ⋯ 断开标记，不按时间距离定位防误读）；节点可点出同款节点卡、勾选圈（`data-act=anchor-toggle`）与左栏「首义范围」面板「更早 · N」分组双向同步；上限 6 +「+M」溢出，全量在左栏。
- **选择池口径**：窗口内本话题节点 + 当前首义集（含窗外）；更早的其它历史节点不在池内（切 14/30 天窗口扩大视野）；API 校验口径不变（属本话题即可）。
- **窗口切换按日期判断不写死**：30 天窗口覆盖 10-21/10-28 时节点落回正常时间轴位置、折叠带消失（实测 bandAfter30=0、n9OnAxis=true）。

## 契约落点
- `lane-correction-surface` spec：画布要求段新增折叠带 SHALL 段 + 3 个 Scenario（窗口外展示 / 自动不虚构 / 选择池边界）；顺修同段残留的「首条命中内容」→「创建话题时写入」。
- `ui-design.md`：Layout Contract 画布格、溯源表新增「更早折叠带节点 / 更早·N 分组」行（来源 `topic_anchor_overrides` 窗外节点）、断言 **A-12**。
- 原型 v5.3（harness 状态 6「首义·更早」演示 manual 态；默认态保持 auto 演示无带）+ README「变更 4」。

## 实测（agent-browser，主会话独立复验）
带渲染 2 节点、节点卡（10-21 周三 · 接近度 0.69）、勾选双向同步（计数 3→2→3）、30 天带消节点归位、auto 态产品区无「更早」（productEarlier=[]）、A-10 无回归、溢出 0、就地改名零弹窗、validate 绿。

## 过程备注
该轮原型由子线程完成大半后被 MiniMax 额度 429 掐断（未静默换模型，主会话接手验收补漏）；额度恢复前原型类任务主会话直接做。

<!-- pinned 2026-11-13T15:38:29Z -->

## 批A落地：首义重设端点/留痕/待重算标记的实现契约

批A（tasks 1.1-1.4 + 2.1-2.6）已实现，接批B（§3 移除/加回）时直接用下面这些已落地的接口，不要重新探索。

【迁移】backend-go/internal/platform/database/lane_manual_correction_migration.go（三个加法迁移，幂等）：
- 20261113_0001 board_persistent_topics.centroid_pending_recompute boolean NOT NULL DEFAULT false（历史行 false）
- 20261113_0002 topic_anchor_overrides（id bigserial / topic_id bigint / old_embedding vector NULL / new_embedding vector / source varchar(32) / input_summary text / operator varchar(64) / created_at timestamptz；索引 idx_topic_anchor_overrides_topic；**topic_id 无外键**——留痕不随话题硬删而消失）
- 20261113_0003 daily_report_sections 留痕三列 manual_assignment_at timestamptz / manual_assignment_from_topic_id bigint / manual_assignment_action text（均可空，历史行 NULL）
模型在 backend-go/internal/topicgraph/repository/daily_report_models.go：BoardPersistentTopic.CentroidPendingRecompute + 新模型 TopicAnchorOverride（**未注册 AutoMigrate**，建表只认上面那条迁移）；DailyReportSection 已加 ManualAssignmentAt/ManualAssignmentFromTopicID/ManualAssignmentAction 三字段（供 §3 写留痕用）。

【首义重设 repository】repository/daily_report_topic_repository.go：
- `SetTopicAnchorFromSections(topicID uint, sectionIDs []uint, operator string) (*AnchorFromSectionsResult, error)`：单事务三写（写 board_persistent_topics.embedding + 追加 topic_anchor_overrides 一行 + 置 centroid_pending_recompute=true），**不碰 centroid 列**。聚合复用 daily_report_manual_topic.go 的 aggregateEmbeddings（mean pooling，与手动建泳道同口径）。校验整体拒绝，四类 AnchorSelectionError.Reason：not_in_topic / other_board / no_embedding / dim_mismatch（外加 section_not_found）；另有两个哨兵错误 ErrTopicNotFound / ErrTopicArchived。
- 常量：TopicAnchorSourceSelectedSections="selected_sections"、AnchorEffectiveFromNextReport="next_report"、AnchorOperatorUser="user"。
- 辅助：ListPendingCentroidTopicIDs(boardID)、centroidRefreshTargets(boardID, touched)、RefreshBoardCentroids(boardID, touched) (updated, failed)、refreshCentroidsIsolated(ids, refresh, clearPending)（纯函数，逐话题隔离：失败不清标记、不中断其余）、clearCentroidPendingFlag(tx, topicID)。

【质心刷新范围】daily_report_repository.go 的 SaveReport post-commit 已从「只刷 touched」改为 `r.RefreshBoardCentroids(report.SemanticBoardID, touchedTopicIDs)`——范围 = touched ∪ 本板 centroid_pending_recompute=true 的话题；成功清标记、失败保留待下次重试。质心刷新只 UPDATE centroid，**不写 embedding**（算法不得改写首义）。

【端点】handler/daily_report_anchor_handler.go：`PATCH /api/daily-reports/topics/:id/anchor`，body 只收 section_ids（[]uint）；自然语言/非数组/缺字段/空数组 → 400；话题不存在 → 404；话题已归档 → 409；选中节点非法 → 400。响应 data = {topic_id, source:"selected_sections", selected_sections:[ids], override_count, effective_from:"next_report"}。路由注册在 handler/daily_report_handler.go 的 RegisterDailyReportRoutes（:id/anchor 与既有 :id/merge/:id/split 同前缀，:id 参数名一致，gin 不冲突）。

【留给批B的现成件】留痕三列已在模型里；RemoveManualAssignmentAt/FromTopicID/Action 三字段写法就绪；`RebuildBoardRelations(tx, boardID)` 事务末尾调用的现成范式见 daily_report_manual_topic.go:CreateManualTopic。批B 需自建「移除/加回」的 repository 方法 + `PATCH /api/daily-reports/sections/:id/assignment` + `POST /api/daily-reports/topics/:id/detach-date` 端点，并复用上面的 AnchorSelectionError/哨兵错误范式做错误映射。

【未做（后续批）】flow/topic-graph.md 红线 17「首义创建后不可变」尚未改写（task 11.1/11.2 属 §11 批）；docs/reference/api/daily-reports.md 端点登记（task 11.4）也未做。

**引用**：backend-go/internal/platform/database/lane_manual_correction_migration.go、backend-go/internal/topicgraph/repository/daily_report_models.go、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go、backend-go/internal/topicgraph/repository/daily_report_repository.go、backend-go/internal/topicgraph/handler/daily_report_anchor_handler.go

<!-- pinned 2026-11-14T04:02:59Z -->

## 泳道修正工作面（批D 落地）：首义读侧端点 + 三个数据源

批D（前端修正工作面）落地时补了一个**后端只读端点**：`GET /api/daily-reports/topics/:id/anchor`（repository `daily_report_anchor_read.go: GetTopicAnchorState` + handler `daily_report_anchor_read_handler.go`）。原因：批A 只落了写端点 `PATCH .../anchor`，**没有读路径**，而工作面契约（5.4 / A-11 / A-12 / 「首义来源可辨」）必须显示「当前首义来源 + 当前首义节点集（含窗口外节点）」——无处可读则只能虚构（违反「自动首义无节点集，不虚构列表」）。响应：`{topic_id, source:"auto"|"manual", section_ids:[], override_count, effective_from}`；manual 的节点集来自 `topic_anchor_overrides` 最新一行的 `input_summary`（追加表，只读不改）。**该端点需在 docs/reference/api/daily-reports.md 登记（§11 批 task 11.4 尚未做）。**

工作面三个数据源（`front/app/features/tags/composables/useLaneCorrection.ts` 是唯一口径）：
1. `GET /semantic-boards/:id/persistent-topics/compose-candidates?days=N` → 窗口内全部 section（成员 / 候选 / 其它泳道只读）+ 阈值；
2. `GET /daily-reports/topics/:id/lifeline` → 泳道**全量**成员（窗口外的首义节点靠它补标题 / 日期 / `topic_match_distance`）；
3. 新增的 `GET .../topics/:id/anchor` → 当前首义来源与节点集。

接近度口径（统一「接近度」= 1 − 余弦距离）：成员取 `topic_match_distance` 换算；候选取对「成员向量均值」的实时距离（复用 `laneComposeLogic.centroidRecommendations`，阈值内才算推荐）。**首义选择池 = 窗口内本泳道节点 + 当前首义集（含窗外）**；已移出节点与「更早的其它历史节点」都不在池内——服务端只接受**属本话题**的节点（`not_in_topic` 400）。自动首义 → 无节点集 → 不出折叠带、不出「更早」分组。

**引用**：backend-go/internal/topicgraph/repository/daily_report_anchor_read.go、backend-go/internal/topicgraph/handler/daily_report_anchor_read_handler.go、front/app/features/tags/composables/useLaneCorrection.ts、front/app/features/tags/components/console/LaneCorrectionCanvas.vue、front/app/features/tags/components/console/LaneAnchorScope.vue、front/app/features/tags/components/console/LaneMemberList.vue

<!-- pinned 2026-11-14T04:59:14Z -->
