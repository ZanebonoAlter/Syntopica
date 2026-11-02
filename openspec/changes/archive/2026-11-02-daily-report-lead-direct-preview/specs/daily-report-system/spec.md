<!-- 归档顺序依赖：本 delta 的 REMOVED/MODIFIED 基线是 rework-workshop-into-radar-and-desk
     的 delta（未归档，其「阅读台文章抽屉与站内阅读预览的状态同步」Requirement 与
     desk 版「日报时间线组件」全文将随其归档同步进主 spec）。
     本 change SHALL 在 rework-workshop-into-radar-and-desk 归档之后归档，
     避免 delta 同步互相覆盖。 -->

## ADDED Requirements

### Requirement: 阅读台线索文章直达站内阅读预览

日报阅读台（`/tags/:boardId/:date`）内点击线索引用文章 SHALL **直接打开站内原文预览弹窗**（复用既有 `ArticlePreviewModal`），SHALL NOT 先弹出中间文章抽屉。打开弹窗前 SHALL 拉取文章详情：详情请求进行中弹窗 SHALL 呈现加载态；请求失败 SHALL 给出可见错误提示且不留空弹窗。预览弹窗内的收藏等状态动作 SHALL 真实落库并即时反映到界面：收藏 SHALL 采用乐观更新——点击后星标立即翻转，不等待网络往返；请求失败 SHALL 回滚到点击前状态并给出可见错误提示。SHALL NOT 出现「点击无响应」的空转实现（事件被空函数吞掉）或状态长时间不更新。

**适用范围**：本 Requirement 覆盖日报阅读台（`DailyReportDeskShell`）与版块矩阵页日报时间线（`useTagsPage.openArticlePreview`）两条预览入口；其他宿主的收藏入口沿用同一乐观更新契约。预览弹窗内部的全屏层级、内嵌网页高度等契约由既有 Requirement 承载，本 Requirement 不重复约束。

#### Scenario: 点击线索文章直达预览弹窗

- **WHEN** 用户在阅读台点击线索展开列表中的一篇引用文章
- **THEN** 系统 SHALL 直接打开站内阅读预览弹窗展示该文章
- **AND** SHALL NOT 出现中间文章抽屉层级

#### Scenario: 详情加载中呈现加载态

- **WHEN** 用户点击文章且详情请求尚未返回
- **THEN** 预览弹窗 SHALL 呈现加载态
- **AND** 请求返回后 SHALL 以完整文章内容替换加载态

#### Scenario: 详情加载失败可见反馈

- **WHEN** 文章详情请求失败
- **THEN** 系统 SHALL 给出可见的错误提示
- **AND** SHALL NOT 留下空白无内容的弹窗

#### Scenario: 站内阅读预览收藏即时生效

- **WHEN** 用户在日报阅读台打开的预览弹窗内点击工具栏的收藏
- **THEN** 星标 SHALL 立即切换为已收藏
- **AND** 系统 SHALL 调用文章更新接口持久化该收藏状态

#### Scenario: 收藏失败回滚并提示

- **WHEN** 收藏请求失败
- **THEN** 星标 SHALL 回滚为点击前的状态
- **AND** 系统 SHALL 给出可见的错误提示

#### Scenario: 连点不丢更新

- **WHEN** 用户在收藏请求返回前再次点击收藏
- **THEN** 界面 SHALL 以最新一次点击为准展示状态
- **AND** 迟到的失败响应 SHALL NOT 覆盖更新后的状态

#### Scenario: 预览内文章状态更新回流

- **WHEN** 预览弹窗发出文章状态更新
- **THEN** 弹窗当前文章 SHALL 应用该更新

## REMOVED Requirements

### Requirement: 阅读台文章抽屉与站内阅读预览的状态同步

**Reason**：2026-10-30 用户反馈：线索点击先弹右侧文章抽屉、再点「站内阅读」才能进入预览弹窗，两级链路两次打断；预览弹窗本就是站内阅读入口且页边注引文已直达使用，中间抽屉层冗余。点开文章改为直达预览弹窗后，「文章抽屉」层级不复存在，以「点开文章先开抽屉」为前提的本 Requirement 失效。

**Migration**：抽屉与预览弹窗两级链路收敛为直达预览弹窗，由 ADDED Requirement「阅读台线索文章直达站内阅读预览」承接——原 Requirement 的状态同步契约（收藏乐观更新、失败回滚、连点不丢更新、状态回流）四个场景逐字迁移保留；「打开原文」能力由预览弹窗内 `ArticleContentView` 工具栏既有「在新窗口打开原文」承载；`DeskArticleDrawer.vue` 组件随本 change 删除。

## MODIFIED Requirements

### Requirement: 日报时间线组件 BoardDailyReportTimeline（报纸布局）
前端 SHALL 提供 `BoardDailyReportTimeline.vue` 组件，展示板块日报列表，并以**阅读台**（`/tags/:boardId/:date`）呈现选中日报。阅读台 SHALL 沿用旧版阅读层的**三栏**：左侧目录栏（本期目录 / 持续追踪 / 历史日报 / 版块切换）、正文（刊头 + 分区正文）、宽屏下常驻的右侧页边注栏；MUST NOT 渲染已撤下的右栏演进/本期话题。页边注在 <1100px SHALL 收为右下浮动入口 + 抽屉。日期历史 SHALL 由左侧目录栏的历史列表承载（「加载更早」增量），不再使用日期下拉。

**入口与容器**：阅读台 SHALL 作为独立路由页面渲染，取代原先 `position: fixed; inset: 0` 的全屏浮层。离开阅读台（返回按钮或 Esc）SHALL 回到该版块的矩阵视图，并保留进入前的选中泳道。阅读台 SHALL 在页面容器内整体滚动（顶栏 sticky，左目录与页边注栏各自 sticky），MUST NOT 把正文锁进固定高度的小滚动窗。

**宽屏布局**：桌面宽屏（>1100px）SHALL 采用三栏——左目录定宽、正文自适应铺满剩余宽度（MUST NOT 再压成 40rem 居中窄栏）、右侧页边注栏 `clamp(15rem, 17vw, 17rem)`；刊头（含头条与「今日重点」最多 3 条 highlights）SHALL 在正文上方全宽展示，MUST NOT 隐藏 highlights。1100px 及以下 SHALL 单栏（页边注收为右下浮动按钮 + 抽屉）；767.98px 及以下 SHALL 单栏且不得产生页面级横向溢出。

**主题**：阅读台的 surface、文字、边框、阴影和交互状态 SHALL 使用现有 semantic theme token，并同时支持 editorial/dark。topic 的稳定 `persistent_topic.color` MAY 用作身份强调色。组件 SHALL NOT 以固定浅色背景为前提硬编码黑色透明色。

**内容映射**：
1. masthead 展示真实日报标题、日期、article_count 和 cluster_count；刊头 SHALL NOT 固定声称"号外"。
2. 头条优先取 `highlights[0]`；highlights 为空时回退到质量最高的 section，且 SHALL NOT 生成 API 中不存在的新闻文案。
3. highlights 按 API 顺序最多展示三项；为空时不渲染该区块。
4. section 按 `persistent_topic.status` 分为"关心的话题"(active)、"突发的新话题"(candidate/已归属但未 active)和"其他动态"(未归属)。分区内继续按 best_tier 升序、avg_score 降序排列。
5. `dynamics` 为空时不渲染"板块动态"区块。

**目录与日期联动**：左侧目录栏 SHALL 提供「头条」「话题正文」锚点、持续追踪话题、历史日报日期列表与版块切换；历史日报 SHALL 按日期倒序列出当前已加载日报，点击 SHALL 切换日期；「加载更早」SHALL 扩充该列表的日期范围。分区索引下拉与 scroll-spy 已于 2026-10-27 用户复审撤下，阅读台 SHALL NOT 依赖它们。

**话题卡片与 mini 生命线**：active 话题卡片 SHALL 支持原位展开。首次展开调用 `getTopicLifeline(topicId)`，展示以当前日报日期为终点的最近七个自然日；加载成功后按 topic id 缓存，重复展开不再次请求。加载、失败、重试和空状态 SHALL 明确可见。

mini 生命线 SHALL 按日期列展示真实 section，同日多 section 合并为一个节点并显示数量。连线只使用响应中 `relation_type="identity"` 的真实关系，采用贝塞尔路径连接端点；空白日期不生成假节点，系统 SHALL NOT 根据日期相邻关系臆造连线或混入 similarity relation。节点点击 SHALL 原位展开当日 threads。

原「话题总览」入口与 desk 索引里的「在矩阵中查看这条线」入口已于 2026-10-27 复审随新版索引一并撤下；泳道/矩阵聚焦入口保留在版块态势页自身，阅读台不再内嵌该出口。

**thread 文章**：thread 有 `related_article_ids` 时 SHALL 可原位展开文章标题列表。前端只加载尚未缓存的 article id，相同文章跨 thread 复用缓存；单篇失败不阻塞其他文章并提供重试。点击文章 SHALL 直接打开站内阅读预览弹窗（详情拉取期间弹窗呈现加载态、失败给出可见错误提示），并保留阅读台上下文。

**section 生命周期**：点击 section header SHALL 继续打开右侧 SectionLifecyclePanel，不因新增 topic mini 生命线而移除 section 维度入口。

"加载更早" SHALL 每次将 `days` 增加 7 并重新请求该完整时间范围。组件 SHALL 用响应替换当前列表，避免累计请求造成重复日报。切换 board SHALL 将 `days` 重置为 14（阅读台按 14 天展示历史列表，可继续「加载更早」），并清理与旧 board 相关的展开状态。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 打开全屏日报详情
<!-- 保留主 spec 的 Scenario 身份锚以维持旧测试资产追溯；本 change 的设计标签为「打开日报阅读台」，行为以以下 WHEN/THEN 为准。此处仅修复标题兼容，不代表日报设计已随矩阵阶段扩批或交付。 -->
- **WHEN** 用户点击某日报卡片或从版块态势页点击「打开今日日报」
- **THEN** 路由 SHALL 变为 `/tags/:boardId/:date` 并渲染占满视口的三栏阅读台（左目录 / 正文 / 页边注）
- **AND** 返回后 SHALL 回到该版块矩阵并保留进入前的选中泳道

#### Scenario: 宽屏双栏与窄屏降级
<!-- 保留主 spec 的历史 Scenario 身份锚；本 change 拟议的「宽屏三栏与窄屏降级」仍由以下 WHEN/THEN 描述，本轮不改正文、不扩大矩阵阶段的 UI 批准范围。 -->
- **WHEN** 分别以 1920px、1100px 和 720px viewport 打开同一日报
- **THEN** 1920px SHALL 显示「左目录 + 宽屏正文 + 右页边注栏」三栏，刊头与「今日重点」top3 可见，不出现右栏演进
- **AND** 1100px SHALL 单栏并把页边注收为右下浮动入口 + 抽屉
- **AND** 720px SHALL 单栏且不产生页面级横向溢出

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 日报 surface、文字、边框、SVG 和交互状态 SHALL 使用对应主题语义 token 并保持可读

#### Scenario: 按持久话题状态分区
- **WHEN** 日报包含 2 个 active、1 个 candidate 和 1 个未归属 section
- **THEN** 页面 SHALL 分别在"关心的话题""突发的新话题""其他动态"展示 2、1、1 个 section

#### Scenario: 展开并缓存话题生命线
- **WHEN** 用户首次展开一个 active 话题卡片并在加载完成后收起、再次展开
- **THEN** 系统 SHALL 仅调用一次 `getTopicLifeline(topicId)`
- **AND** 再次展开 SHALL 使用缓存结果

#### Scenario: identity 连线跨越空白日期
- **WHEN** lifeline 在周一和周三有节点、周二无节点，且响应包含周一到周三的 identity relation
- **THEN** mini 生命线 SHALL 用一条贝塞尔路径连接周一和周三的真实节点
- **AND** SHALL NOT 为周二生成假节点
- **AND** 该跨天连线 SHALL 以弱化不透明度呈现，与相邻节点的强连线区分

#### Scenario: lifeline 加载失败
- **WHEN** `getTopicLifeline(topicId)` 返回错误
- **THEN** 话题卡片 SHALL 显示局部错误和重试操作，不关闭日报详情或影响其他话题

#### Scenario: 展开 thread 文章并复用缓存
- **WHEN** 两个 thread 引用同一个 article id，用户依次展开两个 thread
- **THEN** 前端 SHALL 只为该 article id 请求一次文章详情
- **AND** 点击文章 SHALL 直接打开站内阅读预览弹窗并保留阅读台上下文

#### Scenario: 进入完整话题生命线
- **WHEN** 用户点击 active topic 的「在矩阵中查看这条线」
- **THEN** 系统 SHALL 返回版块态势页并选中该泳道；无 topic id 时该入口 SHALL 隐藏

#### Scenario: 键盘关闭详情
- **WHEN** 阅读台打开且用户按 Esc
- **THEN** 系统 SHALL 返回矩阵视图，并将焦点恢复到打开阅读台的入口

#### Scenario: 空状态
- **WHEN** 选中 board 但该 board 无日报
- **THEN** 组件 SHALL 展示"暂无日报"并提供生成日报入口

#### Scenario: 加载超过 30 天
- **WHEN** 当前 `days=28`，用户连续两次点击"加载更早"
- **THEN** 组件 SHALL 依次以 `days=35` 和 `days=42` 请求，并展示对应范围内的日报

#### Scenario: 切换 board 重置状态
- **WHEN** 用户从一个 board 切换到另一个 board
- **THEN** 组件 SHALL 将 `days` 重置为 14、加载新 board 日报，并清理旧 board 的 topic/thread 展开状态
