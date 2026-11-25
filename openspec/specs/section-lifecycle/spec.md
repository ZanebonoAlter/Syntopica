<!-- domain: topic-graph -->

## Purpose

Section 级别的生命周期管理：通过 `daily_report_section_relations` 关系表拓扑结构动态推导 section 的 status（emerging/continuing/split/merge）和 ended 标记，提供 timeline 和 lifecycle API，前端展示话题总览 DAG 时间线和 Section Lifecycle Panel。
## Requirements
### Requirement: Section status 动态推导（基于关系表，两阶段）
系统 SHALL 从 `daily_report_section_relations` 表的拓扑关系动态推导 section 的 `status`（关系状态）和 `ended`（结束标记），不再存储 status 字段。

**阶段一：关系状态（status）**：
1. 无 from 关系（无 relation 的 to_section_id 指向它）→ `emerging`
2. 有多个 from 关系（多个旧 section 指向它，to 入度 > 1）→ `merge`
3. 前驱 section 还被其他 section 指向（from 出度 > 1，该 section 是分化出的子叙事之一）→ `split`
4. 有 from 关系且 from 出度 = 1 → `continuing`

**阶段二：结束标记（ended）**：
- 无 to 关系（无 relation 的 from_section_id 指向它）且不是最新一天 → `ended = true`
- 最新一天的 section 即使无 to 关系 SHALL NOT 被标记为 ended

#### Scenario: 新兴叙事
- **WHEN** section #50 无任何 relation 指向它
- **THEN** status SHALL 为 `emerging`，ended SHALL 为 false

#### Scenario: 延续叙事
- **WHEN** section #60 只被 section #50 指向，且 #50 只指向 #60
- **THEN** section #60 的 status SHALL 为 `continuing`

#### Scenario: 叙事分化
- **WHEN** section #50 同时被 section #60 和 #61 指向（#50 出度 = 2）
- **THEN** #60 和 #61 的 status SHALL 为 `split`（它们是从 #50 分化出的子叙事）

#### Scenario: 叙事合并
- **WHEN** section #70 同时被 section #50 和 #55 指向（to 入度 = 2）
- **THEN** section #70 的 status SHALL 为 `merge`

#### Scenario: 延续后结束
- **WHEN** section #50 有 from 关系（status=continuing），无后续 relation，且不是最新一天
- **THEN** section #50 的 status SHALL 为 `continuing`，ended SHALL 为 true

### Requirement: Section Timeline API（含关系）
`GET /api/semantic-boards/:id/section-timeline?days=14` SHALL 返回 sections 扁平列表和 relations 关系列表。SectionTimelineNode 不再包含 `prev_section_id` 字段。

响应格式：
```json
{
  "sections": [{id, report_id, period_date, cluster_label, status, ended, article_count, thread_count}],
  "relations": [{"from_id": 50, "to_id": 60, "distance": 0.25}]
}
```

#### Scenario: 查询板块 section 时间线
- **WHEN** 请求 `GET /api/semantic-boards/5/section-timeline?days=14`
- **THEN** 系统 SHALL 返回 sections 列表和 relations 列表

### Requirement: Section Lifecycle API（适配关系表）
`GET /api/daily-reports/sections/:id/lifecycle` SHALL 基于 `daily_report_section_relations` 表，从目标 section 沿 from/to 关系双向遍历，返回目标 section 所在的完整连通分量。

响应中的 `sections` SHALL 包含连通分量内全部 section；`relations` SHALL 只包含两端都位于该 `sections` 集合中的关系。status 和 ended SHALL 基于完整返回集合及其内部关系推导。

#### Scenario: 查询 section 生命周期含多跳分叉
- **WHEN** 存在 #40 → #50、#50 → #60、#50 → #61，用户请求 #60 的 lifecycle
- **THEN** 系统 SHALL 返回 sections [#40, #50, #60, #61] 和这三条内部 relations

#### Scenario: 查询 section 生命周期含合并
- **WHEN** 存在 #50 → #70、#55 → #70、#70 → #80，用户请求 #50 的 lifecycle
- **THEN** 系统 SHALL 返回 sections [#50, #55, #70, #80] 和这三条内部 relations

#### Scenario: 不返回连通分量外的关系
- **WHEN** 返回 sections 集合为 [#50, #60, #70]
- **THEN** 每条返回 relation 的 from_id 和 to_id SHALL 都位于该集合中

#### Scenario: 孤立 section
- **WHEN** section #20 无任何 relation
- **THEN** 系统 SHALL 返回 sections [#20]，relations 为空

#### Scenario: 环形关系终止遍历
- **WHEN** 存在 #50 → #60、#60 → #70、#70 → #50
- **THEN** 系统 SHALL 各返回三个 section 一次且遍历正常终止

### Requirement: 话题总览组件（DAG 时间线，d3-dag 布局）
前端 `BoardThreadBrowser` SHALL 使用 d3-dag Sugiyama 布局算法 + 自定义 SVG 渲染，展示话题之间的关系网络。

**布局引擎**：d3-dag `sugiyama()` 算法，rankdir=LR（从左到右按时间），自动计算节点 x/y 坐标和边路径。

**渲染要求**：
- 横轴 = 日期列（最近 14 天），节点按 `period_date` 分配到对应列
- 纵轴 = lane（话题链），分支时子话题分配到新 lane，合并时 lane 汇聚
- 节点 = 圆形，颜色按 status：emerging=绿、continuing=蓝、split=橙、merge=紫
- ended=true 的节点用降低透明度或灰色边框标记，保留 status 对应的填充色
- 连线 = SVG path 贝塞尔曲线，基于 d3-dag 计算的边路径点渲染
- 同一天多个 to 节点指向同一 from → 分叉（多条曲线从同一节点扇出到不同 lane）
- 多个 from 节点指向同一 to → 合并（多条曲线汇聚到同一节点）
- 点击节点弹出详情卡片

**无关系的节点**：不参与 DAG 布局，单独以独立圆点形式显示在对应日期列。

#### Scenario: 展示叙事分化
- **WHEN** section #50 (06-01) 被 section #60 和 #61 (06-02) 同时指向（#50 出度 = 2）
- **THEN** DAG SHALL 将 #60 和 #61 分配到不同 lane，从 #50 扇出两条贝塞尔曲线分别到 #60 和 #61，#60 和 #61 显示为橙色

#### Scenario: 展示叙事合并
- **WHEN** section #70 (06-02) 被 section #50 和 #55 (06-01) 同时指向（to 入度 = 2）
- **THEN** DAG SHALL 从 #50 和 #55 所在 lane 各画一条贝塞尔曲线汇聚到 #70，#70 显示为紫色

#### Scenario: 展示已结束叙事
- **WHEN** section #50 (06-01, status=continuing) 无后续 relation，最新天为 06-03
- **THEN** DAG SHALL 以蓝色显示 #50（continuing），但用降低透明度或灰色边框标记 ended=true

#### Scenario: 展示独立话题
- **WHEN** section #80 无任何 relation（emerging，无后续）
- **THEN** DAG SHALL 在对应日期列显示一个独立圆点，不画连线

### Requirement: Section Lifecycle Panel（垂直 DAG，d3-dag 布局）
`SectionLifecyclePanel` SHALL 使用 d3-dag Sugiyama 布局算法 + 自定义 SVG 渲染，以 git log --graph 风格展示话题的完整上下游关系。

**布局引擎**：d3-dag `sugiyama()` 算法，rankdir=TB（从上到下按时间分层），自动计算节点 x/y 坐标和边路径。

**渲染要求**：
- 以选中话题为中心，展示后端 BFS 返回的完整连通子图
- 同一天（同层）的节点水平排列，跨天通过边连接
- 分支点：一个节点扇出多条边到多个子节点（不同 lane）
- 合并点：多条边汇聚到一个节点
- 当前选中的话题（sectionId）高亮显示
- hover 节点时使用共享的有界 BFS 高亮节点和内部关系边
- 节点显示日期、cluster_label、status 徽章、文章/线索数量
- 节点可点击 emit navigate 跳转到对应日报
- 面板宽度 320px，SVG 区域支持垂直滚动
- ended=true 的节点降低透明度

**无关系的孤立话题**：显示单个节点，无连线。

#### Scenario: 展示分叉生命周期
- **WHEN** section #50 有两个后续 section #60 和 #61
- **THEN** 面板 SHALL 将 #60 和 #61 排列在不同 lane，从 #50 扇出两条边分别到 #60 和 #61

#### Scenario: 展示合并生命周期
- **WHEN** section #70 被 section #50 和 #55 同时指向
- **THEN** 面板 SHALL 从 #50 和 #55 各画一条边汇聚到 #70

#### Scenario: 展示多跳生命周期
- **WHEN** API 返回 #40 → #50 → #60 → #70
- **THEN** 面板 SHALL 展示全部四个节点和三条关系边

#### Scenario: 悬停高亮多跳关系
- **WHEN** 面板显示 #50 → #60 → #70，用户悬停 #50
- **THEN** #50、#60、#70 及两条关系边 SHALL 高亮

#### Scenario: 孤立话题
- **WHEN** section #20 无任何 relation
- **THEN** 面板 SHALL 显示单个节点 "独立话题"，无连线

### Requirement: 报纸视图 section 卡片适配
`BoardDailyReportTimeline` 的报纸视图中，section 卡片 SHALL 移除 status 徽章显示。section 的 status 仅在话题总览（BoardThreadBrowser DAG 时间线）和 SectionLifecyclePanel 中展示。

#### Scenario: 报纸视图不显示 section status
- **WHEN** 用户打开日报详情报纸视图
- **THEN** 每个 section 卡片 SHALL 不显示 status 徽章（emerging/continuing 等）

### Requirement: Thread 列表移除 Lineage 入口
`BoardDailyReportTimeline` 的 thread 列表中，SHALL 移除 sitemap 图标按钮（ThreadLineagePanel 入口）。Thread 不再支持血统追踪。

#### Scenario: thread 不显示 lineage 入口
- **WHEN** 用户展开某 section 的 thread 列表
- **THEN** 每个 thread 条目 SHALL 不显示 sitemap 图标按钮

### Requirement: 话题总览工作台（占满 + 工具条 + 弹窗弃用）

前端 `BoardThreadBrowser` 的话题总览 SHALL 升级为占满 `tags-content` 区域的全局工作台（顶部栏 + 左侧版块栏 + 右侧 tabs 的真实布局内，占满 content 宽高），不再悬浮留白。

工作台 SHALL 提供工具条（取代原"日报列表 ↔ 话题总览"切换的单按钮），包含：
- 时间范围选择器（默认 14 天，可选 7 / 30 / 全部），切换时重载总览数据。
- 视图模式分段（时间线 timeline / 泳道 lanes）。
- 回刷归属（原 `TopicManageDialog` 能力迁入）。
- 合并预览（原 `TopicManageDialog` 能力迁入）。
- 新建泳道（进入编排态，见下条 Requirement）。

lanes 视图 SHALL 占满工作台主体：左侧泳道标签列（状态点 + 名 + 计数 + 跨度/最近日期）+ 右侧时间网格（按时间范围选择器窗口渲染列数）。同一天多条 section 节点 SHALL 维持纵向堆叠（基于现有 `subOffset` + 自适应 `laneH`，本变更不改算法，只要求布局撑满）。

系统 SHALL 弃用 `TopicManageDialog.vue`：其全部能力（回刷 / 重命名 / 归档 / 合并 / 删除）SHALL 迁移到工作台工具条与泳道 hover 操作菜单。hover 任意泳道 SHALL 出现就地操作（重命名 / 归档或恢复 / 删除），`source=manual` 与 `source=auto` 话题在 hover 操作上一视同仁。

#### Scenario: 总览占满 content

- **WHEN** 用户进入话题总览
- **THEN** lanes 视图 SHALL 占满 content 宽高（无悬浮留白），泳道多时纵向滚动

#### Scenario: 时间范围切换

- **WHEN** 用户将时间范围从"最近 14 天"切到"最近 30 天"
- **THEN** 总览 SHALL 重载并渲染 30 天的泳道与日期列

#### Scenario: 弹窗能力迁入工具条

- **WHEN** 用户点击工具条"回刷归属"
- **THEN** 系统 SHALL 触发回刷（原弹窗能力），SHALL NOT 打开 `TopicManageDialog`

#### Scenario: 泳道就地操作

- **WHEN** 用户 hover 某条 active 话题泳道
- **THEN** SHALL 出现重命名 / 归档 / 删除操作按钮；点击归档 SHALL 将该话题转 archived（不弹窗）

#### Scenario: 同天多节点纵向堆叠

- **GIVEN** 06-20 有 2 条 section 同属话题「中东局势」
- **WHEN** lanes 渲染该泳道
- **THEN** 06-20 列 SHALL 显示 2 个纵向堆叠的节点（横向位置相同、纵向错开），SHALL NOT 横向并列

### Requirement: 手动建泳道编排态（预览 + 候选池 + 体检报告）

用户点击「新建泳道」SHALL 进入编排态。编排态 SHALL 为 lanes 视图上的 `composeMode` 布尔**叠加态**，SHALL NOT 切换 `viewMode`（不再使用 `viewMode='compose'` 全屏替换）。「新建泳道」入口 SHALL 同时存在于工作台工具条与 unassigned（待确认/未分类）泳道头部（主战场入口）。

**就地叠加布局**：编排态 SHALL 保持 lanes 泳道主视图可见——已有 active 泳道 SHALL 淡显（低 opacity、约 0.3）保留为背景参照，SHALL NOT 被整体盖掉。编排态 SHALL 提供顶部浮工具条（泳道名输入 + 已勾计数 + 「聚类质量」单卡 + 取消/保存）与右侧滑出候选侧边栏，SHALL NOT 提供原全屏三栏（预览时间轴 / 候选 section 池 / 体检报告三卡）。

**主战场就地勾选**：unassigned 泳道 SHALL 为编排主战场，其每个 section 节点 SHALL 渲染 checkbox 支持就地勾选。勾选/取消勾选 SHALL 实时重算聚合锚点（`aggregatePreview`，mean pooling）并重算全员 `cosineDistance`：节点 SHALL 标注 distance 数字 + 边框分层（`distanceTier`：good / boundary / outlier）+ 离群标黄（`outlierFlags`，distance > match_threshold × 1.3，标黄但 SHALL 保持勾选状态由用户决定，不自动移除）。同一天多节点 SHALL 维持纵向堆叠。

**active 泳道可勾走（成员移出）**：淡显 active 泳道中的 section 节点 SHALL 亦可勾选，勾选语义为「从原 active 泳道移出到新泳道」。勾选 active section 时节点 SHALL 实时标注「将从【原泳道名】移出」，原泳道名 SHALL 取自候选数据的 `persistentTopic.label`。**移出口径 SHALL 只认 `persistentTopic.status === 'active'`**（与 lanes 视图 `sectionLaneKey` 同口径）——归属 candidate/archived topic 的 section 在 lanes 显示为「未分类」，SHALL NOT 判为移出（不计入 moveOut 计数 / 不进保存二次确认 / 不进推荐次组），SHALL 与无归属 section 同等对待（归推荐主组、可自由勾选建新泳道）。

**聚类质量单卡**：顶部浮工具条 SHALL 提供单张「聚类质量」卡（取代原三卡），实时展示成员数 / 平均距离 / 离群数。数据计算：`aggregatePreview` 取 mean → 各选中向量 `cosineDistance(v, mean)` 求平均 → `outlierFlags(distances, threshold)` 计离群数。原「撞车检查」卡与「未来预期」卡 SHALL NOT 保留（未来预期 v1 本就未实现；撞车改为保存时动作提示，见下）。

**保存与移出确认**：编排态 SHALL 提供泳道名输入（`AppInput`）、保存/取消（`AppButton`），SHALL NOT 使用 `window.alert/prompt/confirm`。保存 SHALL 触发手动建泳道 API（`createManualLane`，label + 选中 section_ids）；section 重指新 topic 即自动离开原 active 泳道，无需额外 API。**存在 active 移入项时 SHALL 先弹二次确认**，列出全部将从原 active 泳道移出的 section（含原泳道名），用户确认后才提交；无移入项可直接保存。成功后 SHALL 退出编排态、刷新总览，新泳道以 active + source=manual 出现在 lanes，被勾 section 从 unassigned / 原 active 泳道移出。

**取消**：取消 SHALL 清空勾选与名字、退出编排态，SHALL 无副作用（不发 API）。

编排态 SHALL 一次只建一条新泳道（不支持并发编排多条）。

#### Scenario: 进入编排态不切视图

- **GIVEN** 用户在 lanes 视图查看现有泳道
- **WHEN** 用户点 unassigned 泳道头部「新建泳道」
- **THEN** SHALL 进入 `composeMode` 叠加态，`viewMode` SHALL 保持 `lanes` 不变；active 泳道 SHALL 淡显保留可见，SHALL NOT 被全屏视图盖掉

#### Scenario: unassigned 节点就地勾选实时贴合度

- **GIVEN** 编排态已勾选 unassigned 中 5 条 section
- **WHEN** 用户取消勾选其中 1 条
- **THEN** 聚合锚点 SHALL 用剩余 4 条重算（mean 向量），全员 distance SHALL 重算，节点 distance 数字 / 边框分层 / 离群标黄 SHALL 相应更新

#### Scenario: 离群标黄不自动删

- **GIVEN** 勾选 5 条，其中「荷兰扩 ASML 限制」到锚点距离 0.41 > 1.3×0.30
- **WHEN** 渲染节点
- **THEN** 该节点 SHALL 标黄 + 提示建议剔除，但 SHALL 保持勾选状态（不自动移除），由用户决定

#### Scenario: 勾走 active section 标移出提示

- **GIVEN** 编排态淡显背景含 active 泳道「中东局势」，其中 1 条 section 被勾选
- **WHEN** 该节点被勾选
- **THEN** 节点 SHALL 实时标注「将从【中东局势】移出」，顶部计数 SHALL 反映「N 个来自现有泳道，将移出」

#### Scenario: candidate 归属不判移出（对齐 lanes 未分类口径）

- **GIVEN** 某 section 归属一个 status=candidate 的 topic（在 lanes 视图显示为「未分类」）
- **WHEN** 用户在编排态勾选该 section
- **THEN** 节点 SHALL NOT 标注移出、SHALL NOT 计入 moveOut 计数、保存时 SHALL NOT 进移出二次确认；该 section SHALL 视同未归属（可自由并入新泳道，归推荐主组）

#### Scenario: 聚类质量单卡实时

- **GIVEN** 编排态勾选 5 条
- **THEN** 顶部「聚类质量」卡 SHALL 显示成员数 5 / 平均距离 / 离群数，随勾选实时更新；SHALL NOT 显示原「撞车检查」「未来预期」卡

#### Scenario: 保存前移出二次确认

- **GIVEN** 勾选含 3 条来自 active 泳道「中东局势」的 section
- **WHEN** 用户点保存
- **THEN** SHALL 先弹二次确认列出「3 条将从『中东局势』移出」，用户确认后才调 `createManualLane`；无移入项时 SHALL 直接保存不弹确认

#### Scenario: 保存后新泳道出现并移出

- **WHEN** 用户填名「美伊博弈」并确认保存，API 成功
- **THEN** SHALL 退出编排态、刷新总览，「美伊博弈」以 active 出现在 lanes；被勾的 unassigned section 与从「中东局势」勾走的 section SHALL 均归入新泳道、从原位置移出

#### Scenario: 取消无副作用

- **GIVEN** 编排态勾选若干 section 并填名
- **WHEN** 用户点取消
- **THEN** SHALL 清空勾选与名字、退出编排态，SHALL NOT 发起任何 API 请求

#### Scenario: manual confidence 节点样式区分

- **GIVEN** 手动建好的 topic #20 下有 section（confidence=manual）
- **WHEN** 总览 lanes 渲染 #20 的节点
- **THEN** 该节点 SHALL 用独立样式（双环描边）区分于算法三态（实心/虚线/空心），hover 显示「人工归属」，SHALL NOT 套用算法 distance 三态样式

### Requirement: 编排态候选池语义搜索（渐进收敛排序）

编排态 SHALL 在右侧候选侧边栏提供自然语言语义搜索，帮用户从大量未归类 section（unassigned 主战场）中快速定位相关条目，并通过渐进收敛的贴合度分层降低人工挑选成本（原「候选池列表排序」演化为「就地节点分层 + 侧边栏搜索」）：

- **侧边栏搜索框**：右侧候选侧边栏 SHALL 提供自然语言搜索输入框（`AppInput`）。用户输入文本并停顿（debounce）后，系统 SHALL 调用文本嵌入端点（`POST /persistent-topics/embed-query`）获取查询向量，并按「查询向量 ↔ 各未勾选 section embedding」的 cosine 距离对 unassigned 节点做相关度高亮/置顶提示（最相关的视觉强提示）。
- **勾选即接管主信号**：一旦用户勾选任意 section，主信号 SHALL 切换为「已选集合的聚合向量（mean pooling，镜像 `aggregatePreview`）」——已选是用户确证信号，优先级高于文本查询；此时所有节点 SHALL 按到聚合锚点的距离重算 distance/tier（取代查询向量排序）。勾选更多时锚点 SHALL 持续重算、节点分层 SHALL 持续更新（渐进收敛）。
- **搜索框降级**：勾选后搜索框 SHALL 保留可见但不再作为主信号；清空文本不影响已选聚合分层。
- **默认分层**：未输入文本且未勾选任何 section 时，节点 SHALL 回退默认（按 `period_date` 倒序的自然 lanes 布局，无额外相关度分层）。
- **模型一致性**：文本嵌入端点 SHALL 复用与 section embedding 相同的全局模型（`CapabilityEmbedding`），保证 cosine 相似度可比。
- **失败降级**：文本嵌入端点失败或返回空向量时 SHALL NOT 阻断编排——回退默认分层并给轻量错误提示，用户仍可手动浏览勾选。

#### Scenario: 文本搜索冷启动高亮

- **GIVEN** 编排态 unassigned 有 40 条 section，未勾选任何
- **WHEN** 用户在侧边栏搜索框输入「半导体出口管制」并停顿
- **THEN** SHALL 调用嵌入端点获取查询向量，与「半导体」语义相近的未勾选节点 SHALL 视觉强提示（高亮/置顶提示），SHALL 不改变 `viewMode`

#### Scenario: 勾选后聚合向量接管分层

- **GIVEN** 用户已输入搜索文本并勾选了 2 条相关 section
- **WHEN** 节点分层重算
- **THEN** 主信号 SHALL 切换为这 2 条的聚合向量（mean pooling），所有节点 SHALL 按到聚合锚点的 distance/tier 重算；文本查询向量不再决定主信号

#### Scenario: 渐进收敛

- **GIVEN** 勾选 2 条，节点按聚合锚点分层
- **WHEN** 用户再勾选 1 条
- **THEN** 聚合锚点 SHALL 用 3 条重算，节点分层 SHALL 按新锚点更新

#### Scenario: 清空回退默认

- **GIVEN** 用户清空搜索框且取消全部勾选
- **THEN** 节点 SHALL 回退默认 lanes 布局（按 `period_date` 倒序），无额外相关度分层

#### Scenario: 搜索失败不阻断

- **WHEN** 嵌入端点返回错误或空向量
- **THEN** SHALL 回退默认分层 + 显示轻量错误提示，SHALL NOT 阻塞勾选与保存流程

### Requirement: 编排态候选话题引导（连续命中候选的一键激活/并入）

编排态 SHALL 在右侧候选侧边栏提供「连续命中候选话题」区（原「候选池上方引导区」演化为侧边栏），把 board 内 `status=candidate` 的持久化话题摆出来，作为比「从 unassigned 逐条勾选」更直接的编排入口（迁移自原 `TopicManageDialog` 的候选激活能力）。

侧边栏 SHALL 列出当前 board 的全部 candidate 话题，每条 SHALL 显示 label、连续命中天数（`consecutive_hits`）、所含 section 数（`section_count`）。已达 `upgrade_threshold`（`can_activate=true`）的候选 SHALL 置顶并高亮，与未达标候选视觉区分。

每条候选 SHALL 提供两个动作：
1. **确认启用**：仅 `can_activate=true`（`consecutive_hits >= upgrade_threshold`）时可点。点击 SHALL 调用话题状态更新（status→active），成功后 SHALL 刷新总览（新 active 话题以泳道出现在 lanes）。未达标时该按钮 SHALL 禁用并提示「需先满足连续多天出现条件」。
2. **采纳（并入新泳道）**：点击 SHALL 预填新泳道名输入框（用该候选 label）+ 把 unassigned 中与该候选 centroid 距离在 `matchThreshold` 内的 section 预勾加入当前选中集（一键起步；预勾数量超过上限时 SHALL 截断并提示）。此为纯前端操作，SHALL NOT 调用任何 API。窗口外或不在此候选范围内的 section SHALL 不受影响。

board 无 candidate 话题时，侧边栏该区 SHALL 隐藏（不占位）。

#### Scenario: 侧边栏列出连续命中候选

- **GIVEN** board 有候选话题「美伊博弈」（consecutive_hits=3，section_count=4，can_activate=true）与「油价波动」（consecutive_hits=1，section_count=1，can_activate=false）
- **WHEN** 用户进入编排态
- **THEN** 右侧侧边栏 SHALL 列出两条候选，「美伊博弈」因 can_activate=true SHALL 置顶高亮，「油价波动」的「确认启用」按钮 SHALL 禁用

#### Scenario: 一键激活达标候选

- **GIVEN** 候选「美伊博弈」can_activate=true
- **WHEN** 用户点「确认启用」
- **THEN** SHALL 调用状态更新将其转 active，成功后 SHALL 刷新总览，「美伊博弈」以 active 泳道出现在 lanes

#### Scenario: 采纳预填名并预勾相关 section

- **GIVEN** 编排态已勾选 1 条 section，候选「美伊博弈」centroid 附近有 3 条 unassigned section 在 matchThreshold 内
- **WHEN** 用户点「美伊博弈」的「采纳」
- **THEN** 泳道名输入框 SHALL 预填「美伊博弈」，那 3 条 section SHALL 被预勾加入选中集（选中数变 4），聚合锚点与聚类质量卡 SHALL 实时重算；SHALL NOT 发起任何 API 请求

#### Scenario: 无候选时侧边栏该区隐藏

- **GIVEN** board 无 status=candidate 话题
- **THEN** 右侧侧边栏候选话题区 SHALL 隐藏，不占用布局空间

#### Scenario: 已中断候选单列分组

- **GIVEN** 候选话题「断连续」consecutive_hits=0（近期未命中），「美伊博弈」consecutive_hits=1
- **WHEN** 渲染侧边栏候选区
- **THEN** SHALL 分两组：「正在连续命中」（含「美伊博弈」）与「已中断·近期未命中」（含「断连续」）；已中断组 SHALL 显示「近期未命中」而非「连续 0 天」，且 SHALL NOT 提供「确认启用」按钮（仅「采纳」），视觉弱化

#### Scenario: 已中断候选组默认折叠

- **GIVEN** 侧边栏候选话题区存在「已中断·近期未命中」组
- **THEN** 该组 SHALL 默认折叠（内容不渲染），标题 SHALL 显示计数（如「已中断·近期未命中（3）」）并带展开控件（aria-expanded=false）；用户点击标题 SHALL 切换展开/收起

### Requirement: 编排态已勾选 section 查看线索

编排态候选 section 池中，**已被勾选**的 section SHALL 提供就地「查看线索」入口，让用户在决定是否将某条 section 串进新泳道前，能看到该 section 包含哪些线索（thread）及其文章数，解决「勾选前不知道这条 section 具体讲什么」的判断缺口。未勾选的 section SHALL NOT 显示该入口。

点选「查看线索」SHALL 就地在候选条目下方展开线索列表（不弹窗、不跳转）。列表数据 SHALL 通过复用日报详情端点（`getDailyReportDetail(report_id)`）加载——编排态候选 section 的 `report_id` SHALL 由 `compose-candidates` 端点随 section 一并返回。每条线索 SHALL 显示标题与文章数；编排态 SHALL NOT 在线索列表内再展开文章正文（编排态聚焦「挑 section」，读文章走总览）。

线索加载 SHALL 有 loading 态与失败降级（失败时给出轻量提示，SHALL NOT 阻断勾选/保存流程）。再次点选同一 section 的「查看线索」SHALL 收起；切换到另一已勾选 section SHALL 切换内容（单选展开）。

取消勾选某条 section SHALL 收起其线索区（避免展开态悬空）。

#### Scenario: 已勾选才显示入口

- **GIVEN** 候选池有 section A（未勾选）与 B（已勾选）
- **THEN** B 下方 SHALL 出现「查看线索」入口，A SHALL NOT 出现该入口

#### Scenario: 就地展开看线索

- **GIVEN** section B 已勾选
- **WHEN** 用户点 B 的「查看线索」
- **THEN** SHALL 就地在 B 下方展开线索列表（不弹窗/不跳转），调用 `getDailyReportDetail(B.report_id)` 拿到线索，每条显示标题 + 文章数

#### Scenario: 不展开文章正文

- **THEN** 编排态线索列表 SHALL 只显示线索标题与文章数，SHALL NOT 提供文章正文展开入口

#### Scenario: 切换单选

- **GIVEN** B 的线索已展开
- **WHEN** 用户点另一已勾选 section C 的「查看线索」
- **THEN** 线索内容 SHALL 切换为 C 的线索，SHALL NOT 同时展开两条

#### Scenario: 取消勾选收起

- **GIVEN** B 的线索已展开
- **WHEN** 用户取消勾选 B
- **THEN** B 的线索区 SHALL 收起（不悬空）

#### Scenario: 加载失败不阻断

- **WHEN** `getDailyReportDetail` 返回错误
- **THEN** SHALL 显示轻量错误提示，SHALL NOT 阻断勾选与保存流程

#### Scenario: report_id 由端点返回

- **WHEN** `compose-candidates` 返回候选 section
- **THEN** 每条 section SHALL 含 `report_id` 字段（供编排态查线索复用）

### Requirement: 编排态相似 section 推荐向导

编排态 SHALL 在右侧侧边栏提供「相似 section 推荐」区，作为比在散点图逐个扫视更聚焦的勾选向导：根据当前主信号（已选聚合锚点 `anchor` 优先，否则冷启动搜索词向量 `queryVec`，即 `activeSignal`），从候选池中推荐语义最相近的**未勾选** section，辅助用户快速扩充选中集。推荐 SHALL 与主视图 nodeInfo 同信号源——主视图标注的 good(贴合, d≤threshold) 节点必然入选。

**信号与匹配**：推荐 SHALL 复用主视图同一主信号 `activeSignal`（已选聚合锚点 `anchor` 优先，否则搜索词向量 `queryVec`）。对候选池中每个未勾选 section，SHALL 计算 `cosineDistance(embedding, activeSignal)`；distance ≤ `matchThreshold` 的入选。已勾选的 section SHALL NOT 出现在推荐中。

**标题**：推荐区标题 SHALL 随信号源动态——已选聚合为主时「与你已选最相近」，搜索词为主时「与搜索词最相近」。

**分组与排序**：推荐 SHALL 分两组——
1. **待确认来源（主组）**：`persistentTopicId` 为空（unassigned）的未勾选 section，按 distance 升序取 top 5。
2. **现有泳道来源（次组·弱化）**：`persistentTopic.status === 'active'`（归属 active 泳道，对齐 lanes `sectionLaneKey`）的未勾选 section，按 distance 升序取 top 3；SHALL 视觉弱化（降低不透明度 / sunken 背景），置于主组之下。归属 candidate/archived topic 或无归属的 section SHALL 归主组。

**来源标注**：次组每条 SHALL 显示来源泳道名（取自 `persistentTopic.label`，对象缺失兜底「现有泳道」），表明点击即「从该泳道移出」。

**交互**：每条推荐 SHALL 可点击；点击 SHALL 立即 toggle 勾选该 section（加入选中集）。勾选后推荐 SHALL 实时重算（已勾项移出、聚合锚点更新带动全员分层重算）。次组项的点击等同「勾走移出」，SHALL 复用现有移出提示与保存前二次确认。

**空态**：无任何主信号（既未勾选 section 且未输入搜索词）或两组皆空（无匹配）时，该区 SHALL 整体隐藏，不占布局空间。

**覆盖范围**：推荐仅覆盖当前时间窗候选池（`getComposeCandidates`，带 embedding 的 section）内有 section 的范围——与现有「采纳」同口径，属既有限制。

#### Scenario: 无信号时推荐区隐藏

- **GIVEN** 编排态既未勾选任何 section、也未输入搜索词
- **THEN** 侧边栏「相似 section 推荐」区 SHALL 不渲染

#### Scenario: 搜索冷启动也推荐

- **GIVEN** 编排态未勾选任何 section，用户在侧边栏搜索框输入关键词得到查询向量（queryVec）
- **THEN** 推荐区 SHALL 以查询向量为主信号，列出 unassigned 中 good(d≤threshold) 的未勾选 section；标题 SHALL 显示「与搜索词最相近」

#### Scenario: 勾选后分组升序并按阈值过滤

- **GIVEN** 已勾选 1 条 unassigned section，池内另有 3 条 unassigned（距离 0.05 / 0.20 / 0.50）与 2 条 active（距离 0 / 0.10）在 matchThreshold(0.3) 内
- **THEN** 主组 SHALL 列 2 条 unassigned（0.05、0.20 升序；0.50 超阈值排除）；次组 SHALL 列 2 条 active（0、0.10 升序）

#### Scenario: 点击推荐即勾选并重算

- **GIVEN** 推荐主组列出 section「半导体管制」(distance 0.18)
- **WHEN** 用户点击该推荐
- **THEN** 「半导体管制」SHALL 被勾选加入选中集并从推荐移除；聚合锚点 SHALL 用新选中集重算并带动全员分层更新

#### Scenario: 次组项点击走移出路径

- **GIVEN** 推荐次组列出 active 来源 section「中东战报A」(从「中东局势」移出)
- **WHEN** 用户点击该推荐
- **THEN** 该 section SHALL 被勾选（计入移出），节点 SHALL 标注「将从【中东局势】移出」；保存时 SHALL 走现有移出二次确认

#### Scenario: top-N 截断

- **GIVEN** matchThreshold 内有 7 条 unassigned 与 5 条 active 未勾选 section
- **THEN** 主组 SHALL 最多列 5 条、次组 SHALL 最多列 3 条（各按距离升序取最近者）

#### Scenario: 次组来源名兜底

- **GIVEN** active 来源 section 的 persistentTopic 对象缺失
- **THEN** 该推荐项来源标签 SHALL 显示「现有泳道」

