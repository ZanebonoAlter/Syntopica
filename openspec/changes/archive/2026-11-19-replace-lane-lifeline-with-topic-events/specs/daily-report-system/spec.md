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

**话题卡片与线索相关事件**：active 话题卡片 SHALL 支持原位展开。展开后每条当日线索（thread）SHALL 在其行内渲染「相关线索」chip 行，数据来自 `thread-related-events` 的 `GET /api/daily-reports/:reportId/thread-relations`；首次展开时 SHALL 以 `days=7` 请求一次并覆盖整份日报的全部线索，成功后按 `(reportId, days)` 缓存。MUST NOT 为每条线索各发一次请求。

每条线索 SHALL 最多渲染 3 个 chip，其余折叠为「还有 N 条」（N 来自响应中该线索的推荐总数，展开后仍只渲染 chip，不跳转、不弹层）。**没有相关事件的线索 SHALL NOT 渲染 chip 行**——不占位、不留最小高度、不给「暂无相关」文案。

chip SHALL 在指针悬停或键盘聚焦时显示预览浮层，内容为：候选线索的标题、其摘要、来源文章标题、相似度档位与分数、该候选的日报日期。预览浮层 SHALL 以固定定位渲染并做视口边界翻转，MUST NOT 被线索行的 `overflow` 裁剪。

点击 chip SHALL 直接打开站内阅读预览弹窗（复用既有入口与既有出口），弹窗内 SHALL 显示该推荐的相关度档位、分数与候选日期作为阅读上下文；MUST NOT 跳转离开阅读台。

相关线索档位 SHALL 为 `7 天 / 30 天` 两档，默认 7 天，且 SHALL 以**分区级工具条**呈现（每个内容分区一条，不放在线索行内——否则无候选的线索行会留下只有档位控件的空行）。切换档位 SHALL 以新档位重新请求（含失败后重试），切回已成功加载过的档位 SHALL 命中缓存不再请求。推荐加载失败 SHALL NOT 阻断日报阅读：该线索的 chip 行不渲染，卡片其余内容照常呈现。

daily-report 阅读台 SHALL NOT 再渲染 7 天折线、节点、贝塞尔连线、节点计数或「在侦探墙打开完整生命线」入口。本 change 之后「完整话题生命线」在界面上**暂无入口**（原来的唯一入口随 mini 生命线退役，版块态势页当前未挂载侦探墙）——该缺口 SHALL 如实登记为已知限制，MUST NOT 在文档里宣称「从版块态势页进入」而实际不可达。原「话题总览」入口与 desk 索引里的「在矩阵中查看这条线」入口已于 2026-10-27 复审随新版索引一并撤下；泳道/矩阵聚焦入口保留在版块态势页自身，阅读台不再内嵌该出口。

**thread 文章**：thread 有 `related_article_ids` 时 SHALL 可原位展开文章标题列表。前端只加载尚未缓存的 article id，相同文章跨 thread 复用缓存；单篇失败不阻塞其他文章并提供重试。点击文章 SHALL 直接打开站内阅读预览弹窗（详情拉取期间弹窗呈现加载态、失败给出可见错误提示），并保留阅读台上下文。

**section 生命周期**：点击 section header SHALL 继续打开右侧 SectionLifecyclePanel，不因新增线索相关事件 chip 而移除 section 维度入口。

"加载更早" SHALL 每次将 `days` 增加 7 并重新请求该完整时间范围。组件 SHALL 用响应替换当前列表，避免累计请求造成重复日报。切换 board SHALL 将 `days` 重置为 14（阅读台按 14 天展示历史列表，可继续「加载更早」），并清理与旧 board 相关的展开状态。

#### Scenario: 展示日报卡片列表
- **WHEN** 选中 board "AI与机器学习"，该 board 有 3 天的日报
- **THEN** BoardDailyReportTimeline SHALL 渲染 3 张日报卡片，按日期倒序

#### Scenario: 打开全屏日报详情
- **WHEN** 用户点击某日报卡片或从版块态势页点击「打开今日日报」
- **THEN** 路由 SHALL 变为 `/tags/:boardId/:date` 并渲染占满视口的三栏阅读台（左目录 / 正文 / 页边注）
- **AND** 返回后 SHALL 回到该版块矩阵并保留进入前的选中泳道

#### Scenario: 宽屏双栏与窄屏降级
- **WHEN** 分别以 1920px、1100px 和 720px viewport 打开同一日报
- **THEN** 1920px SHALL 显示「左目录 + 宽屏正文 + 右页边注栏」三栏，刊头与「今日重点」top3 可见，不出现右栏演进
- **AND** 1100px SHALL 单栏并把页边注收为右下浮动入口 + 抽屉
- **AND** 720px SHALL 单栏且不产生页面级横向溢出

#### Scenario: 双主题渲染
- **WHEN** 用户在 editorial 与 dark 主题间切换
- **THEN** 日报 surface、文字、边框和交互状态 SHALL 使用对应主题语义 token 并保持可读

#### Scenario: 按持久话题状态分区
- **WHEN** 日报包含 2 个 active、1 个 candidate 和 1 个未归属 section
- **THEN** 页面 SHALL 分别在"关心的话题""突发的新话题""其他动态"展示 2、1、1 个 section

#### Scenario: 线索行渲染相关事件 chip
- **WHEN** 用户展开一个 active 话题卡片，其中某条线索在窗口内有 5 条推荐
- **THEN** 该线索行 SHALL 渲染前 3 个 chip 与「还有 2 条」
- **AND** 整份日报的推荐 SHALL 由一次 `thread-relations` 请求取得

#### Scenario: 无相关事件的线索不渲染 chip 行
- **WHEN** 某条线索在窗口内没有任何相似度达到阈值的候选线索（任一档位）
- **THEN** 该线索 MUST NOT 渲染 chip 行或任何占位/空态文案
- **AND** 线索标题、摘要、文章数 SHALL 与未引入本特性时一致
- **AND** 档位控件 SHALL 位于分区级工具条，MUST NOT 因该线索无候选而出现在线索行内

#### Scenario: chip 悬停出预览
- **WHEN** 用户将指针悬停在某个 chip 上，或用 Tab 聚焦它
- **THEN** 系统 SHALL 显示预览浮层，含候选线索标题、摘要、来源文章标题、相似度档位与分数与该候选日期
- **AND** 浮层 MUST NOT 被线索行容器裁剪

#### Scenario: 点击 chip 打开阅读预览
- **WHEN** 用户点击某个 chip
- **THEN** 系统 SHALL 打开站内阅读预览弹窗并加载该来源文章
- **AND** 弹窗内 SHALL 显示该推荐的关联度与出现次数
- **AND** 路由 MUST NOT 离开当前日报阅读台

#### Scenario: 7/30 档位切换与缓存
- **WHEN** 用户从 7 天档切到 30 天档，再切回 7 天档
- **THEN** 切到 30 天 SHALL 以 `days=30` 请求一次
- **AND** 切回 7 天 SHALL 直接使用缓存，SHALL NOT 再次请求

#### Scenario: 推荐加载失败不阻断阅读
- **WHEN** `thread-relations` 返回错误
- **THEN** 线索的 chip 行 SHALL 不渲染
- **AND** 日报正文、线索标题摘要、文章列表与关键事件动作 SHALL 照常可用

#### Scenario: identity 连线跨越空白日期
- **WHEN** 窗口内周一与周三有节点、周二无节点，且存在周一到周三的 identity 关系
- **THEN** 日报 SHALL NOT 渲染任何连线或节点图
- **AND** identity 关系 SHALL 继续由后端保留，供版块态势页与侦探墙使用

#### Scenario: 展开并缓存话题生命线
- **WHEN** 用户展开一个 active 话题卡片
- **THEN** 日报 SHALL NOT 再调用 `getTopicLifeline(topicId)`
- **AND** 该位置的能力改由线索相关事件承担，行为以「线索行渲染相关事件 chip」为准
- **AND** 后端 `lifeline` 端点 SHALL 继续存在（版块态势页与侦探墙仍使用）

#### Scenario: lifeline 加载失败
- **WHEN** 旧 lifeline 端点返回错误
- **THEN** 日报话题卡片 SHALL NOT 因此进入错误态（该调用已从日报退役）
- **AND** 错误呈现归属线索相关事件，见「推荐加载失败不阻断阅读」

#### Scenario: 进入完整话题生命线
- **WHEN** 用户在日报阅读台展开 active 话题卡片
- **THEN** 系统 SHALL NOT 渲染「在侦探墙打开完整生命线」入口
- **AND** 完整生命线入口当前 SHALL 视为已知缺口（无界面入口），MUST NOT 渲染任何不可达的入口按钮

#### Scenario: 窄容器排版不崩坏
- **WHEN** 阅读台正文列宽度在 690–900px（视口 900–1400px）且话题卡片已展开
- **THEN** 线索行 SHALL 不产生区块级横向溢出
- **AND** chip 行 SHALL 在容器宽内换行，MUST NOT 出现逐字换行的竖排列
- **AND** 布局 MUST NOT 按「容器宽 / 7 列 × 2」计算宽度

#### Scenario: 展开 thread 文章并复用缓存
- **WHEN** 两个 thread 引用同一个 article id，用户依次展开两个 thread
- **THEN** 前端 SHALL 只为该 article id 请求一次文章详情
- **AND** 点击文章 SHALL 直接打开站内阅读预览弹窗并保留阅读台上下文

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
