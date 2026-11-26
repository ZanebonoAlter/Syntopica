## ADDED Requirements

### Requirement: 关键事件视图切换与行集合

版块态势页主舞台 SHALL 提供两个同级视图：「矩阵 / 追踪」（现状）与「关键事件」。两视图 MUST 共用同一条观察轴（同一时间视窗控件与同一份 `visibleAxis`）、同一份持久化话题行序与同一组受控选中状态（`selectedLane` / `selectedDate`）；切换视图 MUST NOT 重置其中任何一项。时间范围 SHALL 由用户在同一个时间视窗控件里选择（7 / 14 / 30 / 365 档或自由区间），关键事件视图 MUST NOT 维护第二套跨度状态。

行集合 SHALL 只包含持久化话题（active），按矩阵行序各占一行；候选话题（candidate）MUST NOT 占行，其确认事件 MUST NOT 在本视图呈现。

#### Scenario: 切换视图保留选中态

- **WHEN** 用户在矩阵中选中「大模型商业化路径」与 11-19，然后切到「关键事件」
- **THEN** 关键事件视图 SHALL 保持该行选中、11-19 列选中与当前观察窗，MUST NOT 回落到默认选中

#### Scenario: 时间视窗两侧共用

- **WHEN** 用户在矩阵把时间视窗切到 30 天，再切到「关键事件」
- **THEN** 关键事件视图 SHALL 直接使用这 30 天轴，MUST NOT 出现第二个跨度控件或不同的日期范围

#### Scenario: 候选话题不占行

- **WHEN** 观察窗内有 13 个候选话题各自带有确认事件
- **THEN** 本视图行集合 SHALL 仍只有持久化话题，MUST NOT 生成候选行或把候选事件并入其他行

#### Scenario: 右轨选中切回矩阵视图

- **WHEN** 用户在关键事件视图里从右轨（当日其他话题 / 话题条目）选中一个话题
- **THEN** 主舞台 SHALL 切回「矩阵 / 追踪」视图，并把该话题泳道滚入视野 + 高亮，MUST NOT 在关键事件视图里留下一个无对应行的死选择
- **AND** 本视图内部的选中（点行头、点事件节点）SHALL 仍留在本视图，MUST NOT 把用户踢出时间轴上下文

### Requirement: 关键事件节点与名称

行内 SHALL 渲染一条横向日期轴：每条 `state='confirmed'` 的注记在其 `source_period_date` 对应列落一个节点，并在节点旁显示事件名称（`title_snapshot`）。`suggested` 与 `dismissed` MUST NOT 进入本视图；无事件的日期 MUST NOT 落节点，也 MUST NOT 伪造 0。

#### Scenario: 只呈现确认注记

- **WHEN** 某话题在窗口内同时存在 confirmed 与 suggested 注记
- **THEN** 行内 SHALL 只出现 confirmed 的节点与名称，MUST NOT 出现建议态注记

#### Scenario: 无事件日不落节点

- **WHEN** 某持久化话题在窗口内没有任何 confirmed 注记
- **THEN** 该行 SHALL 只渲染虚线轴与「无确认事件」说明，MUST NOT 落节点或显示 0

### Requirement: 关键事件标签布局与横向滚动

同日多条事件 SHALL 竖向堆叠且标签 MUST NOT 重叠；标签 SHALL 完整呈现标题，MUST NOT 用省略号截断。

行内列宽 SHALL 均分可用宽度且不低于下限（64px），总宽超出舞台时 SHALL 在舞台容器内横向滚动且行头保持可见，MUST NOT 为塞进一屏而压缩列宽；滚动时 SHALL 将选中日保持在视口内。标签堆叠层数未超上限时 SHALL 照常显示事件名称；超上限时 SHALL 收起文字标签、只保留节点与当日事件计数，名称改由选中日阅读区承担。收起与否 MUST NOT 仅由观察窗天数决定：长窗 MUST NOT 整体收成光点致用户看不到任何事件名称。

点行头 SHALL 写入与矩阵相同的选中话题；点节点或标签 SHALL 写入相同的选中日，下方阅读区展示该日全部确认事件。

下方事件卡 SHALL 可展开确认时冻结的关联文章（`article_ids_snapshot`），点文章 SHALL 复用既有站内阅读链路打开全文预览，关闭后 SHALL 回到原事件卡且保持展开态与滚动位置。本视图 SHALL 只读，MUST NOT 提供注记写入口。

#### Scenario: 同日多条事件

- **WHEN** 11-12 某持久化话题有 3 条确认事件
- **THEN** 该行在 11-12 列 SHALL 显示 3 个节点与 3 个互不重叠的标签，MUST NOT 合并成 1 条或只显示第一条

#### Scenario: 标签不截断

- **WHEN** 事件标题为「婚外情曝光触发董事会调查与罢免程序」
- **THEN** 标签 SHALL 完整显示该标题，MUST NOT 渲染为省略号残句

#### Scenario: 左键拖拽平移

- **WHEN** 用户在时间轴空白处按住左键向右拖动超过 5px
- **THEN** 时间轴 SHALL 随指针平移且列宽保持不变；位移未超 5px 的按下 SHALL 视为点选，MUST NOT 触发平移

#### Scenario: 列宽均分填满

- **WHEN** 视口宽 2560px、可见 14 天
- **THEN** 每列 SHALL 均分可用宽度（约 165px），MUST NOT 固定成小列宽而在右侧留白

#### Scenario: 展开关联文章

- **WHEN** 某事件在确认时冻结了 5 篇关联文章，用户点「关联文章 5 篇」
- **THEN** 卡片 SHALL 就地展开这 5 篇的标题、来源与日期，再点一次 SHALL 收起

#### Scenario: 文章预览闭合

- **WHEN** 用户点展开列表中的某篇文章，进入站内阅读预览后再关闭
- **THEN** 界面 SHALL 回到原事件卡，卡片 SHALL 保持展开态，滚动位置 MUST NOT 跳回顶部

#### Scenario: 无关联文章

- **WHEN** 某事件确认时没有冻结任何文章
- **THEN** 卡片 MUST NOT 显示「关联文章」入口，MUST NOT 用空列表占位

#### Scenario: 宽列横向滚动

- **WHEN** 用户切到 30 天，行内总宽超出主舞台
- **THEN** 日期轴 SHALL 在舞台容器内横向滚动，行头 SHALL 保持可见，当前选中日 SHALL 自动滚入视口，MUST NOT 压缩列宽或让标签重叠

#### Scenario: 层数超限才收起标签

- **WHEN** 某行的事件标签堆叠层数超过上限（行高封顶）
- **THEN** 该行 SHALL 只显示事件节点与当日事件计数，事件名称改由选中日阅读区呈现，MUST NOT 让标签互相重叠或溢出容器

#### Scenario: 长窗仍看得见事件名称

- **WHEN** 观察窗超过 30 日（可见日更多、列更稀）
- **THEN** 行内 SHALL 照常显示事件名称，MUST NOT 仅因观察窗天数长就整体收成节点；标签层数未超上限时 MUST NOT 收起

#### Scenario: 无确认事件

- **WHEN** 观察窗内该版块没有 confirmed 注记
- **THEN** 本视图 SHALL 照常渲染行与日期轴并显示「这段时间没有确认的关键事件」说明，MUST NOT 用空白轴冒充有内容，MUST NOT 隐藏视图切换入口

#### Scenario: 读取失败

- **WHEN** 版块级关键事件读取失败
- **THEN** 本视图 SHALL 显示错误说明与重试入口，MUST NOT 显示为 0 条事件

#### Scenario: 窄屏不渲染

- **WHEN** 视口宽度小于 768px
- **THEN** 本视图与其切换按钮 MUST NOT 渲染，沿用版块摘要的既有窄屏降级
