# detective-wall-interaction

## Purpose

交互层——处理 Raycaster 悬停/点击、BFS 生命线展开（日期窗口约束）、2D 面板叠加、时间范围切换。协调 Vue 状态和 Three.js 场景之间的双向通信。TBD.

## Requirements

### Requirement: InteractionLayer 交互管理

InteractionLayer SHALL 管理 Raycaster 悬停/点击、BFS 生命线展开、2D 面板、时间范围切换，并协调 Vue 与 Three.js 双向通信。

- `enable()` / `disable()`：转场期间调用
- `setTimeRange(days)`：切换天数后触发重新加载
- `resetToOverview()`：点击空白区域返回总览
- `setHoverSuspended(suspended)`：拖拽相机时暂停 hover（由 WallCameraControls 调用）
- `enterLifecycle(sections, relations, dateRange)` / `exitLifecycle()`：完整生命周期模式

#### Scenario: tooltip 点击转发

- **WHEN** InteractionLayer enable()
- **THEN** 系统 SHALL 在 `scene.css2d.domElement` 注册 click 监听
- **AND** 命中 `[data-card-id]` 时转发为对应卡片的 `handleCardClick`

#### Scenario: 键盘 ESC

- **WHEN** ESC 键按下
- **THEN** Vue 层 keydown 监听处理（lifecycle→exitLifecycle / focusing→closePanel / idle→close），不在 InteractionLayer 内

### Requirement: BFS 生命线算法

系统 SHALL 实现独立的 `bfsLifeline` 算法，严格受日期窗口约束，不可复用现有的 `bfsHighlight`（意图不同：高亮连通分量 vs 日期窗口 BFS 生命线）。

#### Scenario: 邻接表构造

- **WHEN** bfsLifeline 被调用
- **THEN** 系统 SHALL 从 SectionRelation[] 构建无向邻接表（Map<number, Set<number>>）
- **AND** 邻接表构造无歧义（独立 ensure 模式，避免变量复用导致游离数组 bug）

#### Scenario: 日期窗口约束

- **WHEN** BFS 遍历邻居节点
- **THEN** 系统 SHALL 检查节点的 period_date 是否在 dateRange 内
- **AND** 窗口外节点不进入结果（但仍记录 visited 避免重复入队）

#### Scenario: 规范化 edge key

- **WHEN** 记录 BFS 遍历到的边
- **THEN** 系统 SHALL 使用 `minId-maxId` 规范化 key（与 relation 方向无关）
- **AND** 下游匹配 relations 时使用同一规范化规则

#### Scenario: BFS depth Map

- **WHEN** bfsLifeline 返回结果
- **THEN** 结果 SHALL 包含 depth: Map<number, number>（start 为 0，每跳 +1）
- **AND** 深度用于动画 stagger（非数组索引，因 cards 顺序随机）

### Requirement: BFS 动画序列

系统 SHALL 以 GSAP Timeline 编排 BFS 展开动画。

#### Scenario: 动画阶段

- **WHEN** 卡片被点击
- **THEN** 系统 SHALL 计算 BFS 结果（同步，<1ms）
- **AND** 启动 GSAP Timeline：非相关卡片 dim() stagger 0.02s
- **AND** 相机 transitionTo(topicFocus) 并行
- **AND** BFS 节点按 depth stagger highlight（delay = 0.1 + depth × 0.08）
- **AND** 选中卡片上方红色 PointLight（selectionLight）点亮
- **AND** 对应红线按 depth 绘制（drawProgress 0→1, 0.15s/条，比目标节点提前 0.05s）
- **AND** 2D 详情面板在 Timeline 完成后滑入

### Requirement: Raycaster 交互

系统 SHALL 在 requestAnimationFrame 中执行 Raycaster 检测，不在 pointermove 回调中直接计算。

#### Scenario: 悬停

- **WHEN** Raycaster 命中 PinCard.paperMesh
- **THEN** 系统 SHALL 调用 card.elevate()、高亮直接邻居红线、显示 CSS2D tooltip
- **WHEN** 未命中
- **THEN** 系统 SHALL 当前 hovered card.settle()、取消红线高亮、隐藏 tooltip

#### Scenario: 点击卡片

- **WHEN** pointerdown+pointerup 位移 < 5px 且命中卡片
- **THEN** mode='idle' → 执行 BFS 进入 'focusing'
- **AND** mode='focusing' → 同一卡片 resetToOverview()，不同卡片重新 BFS
- **AND** mode='lifecycle' → 不执行 BFS，仅更新 selectionLight + onCardClick

#### Scenario: 点击红线

- **WHEN** 命中红线
- **THEN** 系统 SHALL 以对端节点为起点重新 BFS，相机平移到新焦点

#### Scenario: 点击空白

- **WHEN** 命中空白
- **THEN** mode='focusing' → resetToOverview()
- **AND** mode='lifecycle' → onBackgroundClick()（交 Vue 处理 exitLifecycle，因需 re-fetch timeline）
- **AND** mode='idle' → onBackgroundClick()
- **AND** lifecycle 模式下禁止 resetToOverview()（会错误退出且不 re-fetch）

### Requirement: 2D 详情面板

详情面板 SHALL 使用普通 Vue overlay（position:fixed），不用 CSS2DRenderer。卡片悬停 tooltip 才用 CSS2DRenderer（跟随 3D 卡片坐标）。

#### Scenario: 面板内容

- **WHEN** BFS 完成后填充面板数据
- **THEN** 面板 SHALL 显示案件编号、clusterLabel、文章/线索统计、statusLabel（中文化）、汇总区、生命线节点列表
- **AND** 生命线节点列表用滚动容器（max-height:40vh），不限数量

#### Scenario: 面板定位与动画

- **WHEN** 面板显示
- **THEN** 面板 SHALL 固定在屏幕右侧（right:2rem, top:50%, translateY(-50%), width:280px）
- **AND** 动画使用 motion-v 过渡（x:50, opacity:0 → enter），不用 gsap（遵循 2D/3D 分工）

#### Scenario: 查看详细线索

- **WHEN** 用户查看详细线索
- **THEN** 系统 SHALL 调用 getDailyReportDetail 获取 thread 列表
- **AND** 点击线索 → toggleThreadArticles 二级展开文章列表（批量 getArticle，最多 10 篇，超出显示"还有 N 篇…"）
- **AND** 点击具体文章 → openArticle（不再默认取首篇）

#### Scenario: 查看完整生命周期

- **WHEN** 用户点击查看完整生命周期
- **THEN** 系统 SHALL 调用 getSectionLifecycle(sectionId)
- **AND** interaction.enterLifecycle()：mode='lifecycle'，迷雾 disable，清空旧卡片，渲染 lifecycle 数据，相机 lifecycleFull

#### Scenario: 面板关闭

- **WHEN** lifecycle 模式关闭面板
- **THEN** exitLifecycle() → fog.enable(days) + re-fetch timeline + 相机回 todayFocus
- **WHEN** focusing 模式关闭面板
- **THEN** closePanel() → interaction.resetToOverview()（仅关面板，不退出 3D）
- **AND** 面板按钮「关闭面板/返回时间线」≠ 顶栏「返回」（close，退出整个 3D 视图）

### Requirement: 时间范围切换

系统 SHALL 支持 7/14/30/60 天时间范围切换。

#### Scenario: 切换流程

- **WHEN** 用户点击时间范围
- **THEN** mode='focusing' → 保留 focusedNodeId，用新日期窗口重新 BFS
- **AND** scene.loadBoardData() 加载新数据
- **AND** fog.animateToDensity(newDensity, 0.8)
- **AND** 相机按模式恢复（idle→todayFocus, focusing→topicFocus, lifecycle→lifecycleFull）
- **AND** 新卡片从迷雾边缘 stagger fade in，旧卡片在窗口外消失在迷雾中

### Requirement: Vue ↔ Three.js 桥接

TopicDetectiveWall.client.vue SHALL 管理 DOM/canvas/overlay，调用 API 获取数据，传给 TopicWallScene，接收 InteractionLayer 回调更新 Vue 响应式状态，渲染 2D overlay。

#### Scenario: 桥接职责

- **WHEN** TopicDetectiveWall.client.vue 挂载并加载板块数据
- **THEN** 组件 SHALL 管理 DOM/canvas/overlay，把数据传给 TopicWallScene 渲染
- **AND** InteractionLayer 回调 SHALL 更新 Vue 响应式状态，驱动 2D overlay 渲染

### Requirement: 依赖与约束

- Raycaster 检测在 requestAnimationFrame 中执行，不在 pointermove 回调中直接计算
- BFS 计算同步，数据量 <100 节点时 <1ms，不需要 Web Worker
- 详情面板用 Vue overlay + motion-v，卡片 tooltip 用 CSS2DRenderer
- 完整生命周期模式使用 getSectionLifecycle（不限天数），BFS 模式使用 getBoardSectionTimeline（受 days 限制），两个 API 不能混用
- BFS 动画 highlight stagger 必须用 bfsLifeline 返回的 depth Map
- 移动端（<768px）不提供 3D 入口
- ChapterTransition 当前无 BoardSelector 入口，相关 watch/wipe/cover DOM 已移除，转场期间禁用交互的约束暂不生效

#### Scenario: 计算与渲染分工

- **WHEN** 侦探墙交互运行
- **THEN** Raycaster 检测 SHALL 在 requestAnimationFrame 中执行（不在 pointermove 回调中计算）
- **AND** BFS 计算 SHALL 同步完成（<100 节点 <1ms，不引入 Web Worker）
- **AND** 详情面板 SHALL 用 Vue overlay + motion-v，卡片 tooltip SHALL 用 CSS2DRenderer

#### Scenario: 数据 API 分工

- **WHEN** 加载生命线数据
- **THEN** 完整生命周期模式 SHALL 使用 getSectionLifecycle（不限天数），BFS 模式 SHALL 使用 getBoardSectionTimeline（受 days 限制），二者 SHALL NOT 混用
- **AND** BFS 动画 highlight stagger SHALL 使用 bfsLifeline 返回的 depth Map

#### Scenario: 移动端无 3D 入口

- **WHEN** 视口宽度 < 768px
- **THEN** 前端 SHALL 不提供 3D 侦探墙入口
