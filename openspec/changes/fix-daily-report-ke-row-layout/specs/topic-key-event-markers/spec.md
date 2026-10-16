## MODIFIED Requirements

### Requirement: 条目标记交互

AI建议徽标、推荐理由、确认/不采纳/新手动标记 SHALL 只在当日/历史日报的现有thread中呈现。话题总览（包括lanes、focus和就地详情）SHALL NOT 展示未确认建议、理由或上述挑选按钮；其普通来源新闻 SHALL 仍按原section语义保留，只有confirmed注记能新增关键强调。共享缓存中的suggested/dismissed SHALL NOT 泄漏为总览的待确认节点。话题节点读端 SHALL 默认只返回confirmed，显式请求非confirmed的state参数 SHALL 返回400，日报读端可返回当前报告待处理状态。总览可提供跳转来源日报挑选的导航；已确认节点可取消维护，包括没有有效来源日报锚点的快照。

新增日报动作 SHALL 在现有全屏日报阅读器的thread行中作为文章展开按钮的同级控件；SHALL NOT 嵌入既有drm-thread__header按钮、替换原页边注t:thread.id锚或把整块TopicGroup/section变成关键开关。新增日报动作 SHALL 是现有条目内的单步动作，标记与文章展开互不触发，不新增确认弹窗；日报中标记状态 SHALL 在线索行行内可辨（状态徽标随行尾动作组呈现），不取代已有跑题提示。读写 loading、empty、error SHALL 有局部反馈与恢复入口；正在保存时同条目的互斥动作 SHALL 暂禁用，其他内容仍可用。

线索行呈现 SHALL 采用左右等宽两列分区：左列为标题/摘要（含篇数计数），右列为AI推荐理由与依据数（带分隔线，`pre-line` 折行不截断）；动作组（状态徽标+该状态全部动作按钮）SHALL 呈现为线索行底部全宽行并右对齐（行右下角），横排呈现（空间不足时可整组折行但保持右对齐，不错位堆叠）；窄屏（≤720px）SHALL 回退单列，理由与动作组依次全宽。上述摆放规则 SHALL 由 KeyEventMarkerActions 组件自身 scoped 样式承载（多根 fragment 组件的根节点不继承宿主 scope 属性，宿主样式表 SHALL NOT 保留指向其根节点的子选择器死规则）。

#### Scenario: 建议只在日报展示
- **WHEN** 同一来源条目有尚未确认的AI建议，用户分别打开日报和话题总览
- **THEN** 日报显示建议/理由/确认或不采纳入口；总览保留对应普通新闻及原状态色/计数，但不出现建议徽标、理由、待确认aria文案或额外候选注记

#### Scenario: 总览不承担新选择
- **WHEN** 用户在总览或focus选择未确认新闻并尝试新挑选，随后进入对应来源日报确认
- **THEN** 总览只打开原新闻详情且无确认/不采纳/新手标按钮；导航到正确版块/来源日报条目后才可挑选，成功后总览新增confirmed高亮且普通节点身份/坐标不变；已确认及失效快照仍可取消维护

#### Scenario: 标记不误展开文章
- **WHEN** 用户点击标记按钮或键盘激活
- **THEN** 只提交对应标记动作，不展开文章、切换页面或触发研究；无文章展开能力不自动禁用有效标记

#### Scenario: 未知状态与重复提交反馈
- **WHEN** 正在加载状态或该条目正在保存
- **THEN** 局部显示忙碌，相关写按钮暂禁用；错误保留原文和重试入口，不阻塞其他条目

#### Scenario: 建议行左右等宽且动作右下
- **WHEN** 日报中一条线索带未确认AI建议（理由与依据数非空）
- **THEN** 理由块与标题/摘要列各占行宽一半，理由带分隔线且长文本折行不截断；AI建议徽标与确认/不采纳按钮横排呈现在该线索行底部右对齐（行右下角），不与标题同列堆叠、不覆盖理由文本

#### Scenario: 摆放规则不因scope断链失效
- **WHEN** 构建产物中检查 KeyEventMarkerActions 根节点（.ke-detail/.ke-actions）的 grid 摆放规则来源
- **THEN** 规则由组件自身 scoped 样式命中（根节点带自家scope属性），宿主 DailyReportTopicSection 样式表不含 `.drm-thread-row > .ke-detail/.ke-actions` 选择器；组件测试含源码守卫断言防止规则迁回宿主死区
