# workshop-lane-compose — 编排态节点「已确认关键线索」标记 delta

## ADDED Requirements

### Requirement: 编排态节点显示已确认关键线索标记

编排态画布 SHALL 以星形节点透出人工确认过的主权信号：某 section 下存在 `topic_key_event_markers` 中 `state='confirmed'` 的注记时，该节点 SHALL 由圆点改为星形，SHALL NOT 采用"圆点 + 附加装饰"。

星形 SHALL 只换形状：外接尺寸沿用既有 `pointRadius`，接近度编码不变；SHALL NOT 显示条数，条数只在节点卡内呈现。

节点卡 SHALL 在日期行之后追加「★ 已确认关键线索」区块，逐条列出全部被确认线索的快照标题，SHALL NOT 只取最新一条。

左栏成员条目（日期行右端）与右栏「可补充的事件」条目（日期行首）SHALL 显示 ★，SHALL NOT 新增开关。

★ SHALL 是纯只读投影：不参与选入/移出、接近度、质心、阈值或候选链资格；带星形的节点被选入后 SHALL 保留星形。

#### Scenario: 有确认注记的事件节点

- **WHEN** 画布渲染一个 section，其 `confirmed_key_events` 非空
- **THEN** 该节点 SHALL 以星形渲染（非圆点），节点 aria-label SHALL 含「已确认关键线索 N 条」

#### Scenario: 无确认注记

- **WHEN** section 的 `confirmed_key_events` 为空数组
- **THEN** 节点 SHALL 保持圆点、不追加任何装饰元素，节点卡 SHALL NOT 出现该区块（不留占位符、不写空态文案）

#### Scenario: 卡内明细逐条列出

- **WHEN** 同一事件下有 2 条被确认线索
- **THEN** 节点卡 SHALL 逐条显示两条线索的快照标题，SHALL NOT 截断为一条

#### Scenario: 已选节点仍标记

- **WHEN** 带星形的节点被选入本次泳道
- **THEN** 星形 SHALL 保留并转为实心，中心 ✓ SHALL 按星形内切圆适配且不明显溢出（描边半宽容差内），SHALL NOT 因选中而回退成圆点

#### Scenario: 左右两栏同步

- **WHEN** 左栏成员条目或右栏「可补充的事件」条目的 `confirmed_key_events` 非空
- **THEN** 对应条目 SHALL 显示 ★（左栏日期行右端 / 右栏日期行首），SHALL NOT 引入独立开关

#### Scenario: 星形不改变大小语义

- **WHEN** 同一接近度值下分别渲染带星形与圆点的两个节点
- **THEN** 两者的外接尺寸 SHALL 一致，SHALL NOT 让星形看起来更近或更大

#### Scenario: 不参与计算

- **WHEN** 用户选入或移出任意成员
- **THEN** ★ 的显隐 SHALL 只随 `confirmed_key_events` 变化，SHALL NOT 改变接近度、推荐顺序、候选链资格或成员数

### Requirement: 已确认关键线索标记的归属与数据口径

服务端候选端点 SHALL 为每条 section 返回 `confirmed_key_events` 清单（条目含 marker id、可空的 thread id、快照标题、可空的确认时间）；该字段 SHALL 恒为数组，无标记时 SHALL 为 `[]` 而非 null 或缺席。

标记归属 SHALL 只认 marker 的 `source_section_id` 精确指向；SHALL NOT 按标题、日期、话题或文章集合做模糊匹配补挂。marker 的来源指针为 NULL（日报重建后脱离快照）时，该 marker SHALL NOT 出现在任何节点上。

被标记线索的标题 SHALL 使用服务端冻结快照 `title_snapshot`，SHALL NOT 由客户端拼接、改写或从新闻正文回填。

#### Scenario: 来源脱离的注记

- **WHEN** 某确认注记的 `source_section_id` 为 NULL
- **THEN** 画布 SHALL NOT 在画面任何节点上显示该注记，不得按标题、日期或话题匹配到现存 section

#### Scenario: 空集合契约

- **WHEN** 窗口内没有任何 confirmed 注记
- **THEN** 每条 section 的 `confirmed_key_events` SHALL 为 `[]`，客户端 SHALL 渲染为无标记

#### Scenario: 非确认状态不显示

- **WHEN** 某 section 的注记状态为 `suggested` 或 `dismissed`
- **THEN** 该 section SHALL NOT 显示 ★


### Requirement: 拖拽期间的对象跟随与文本选择隔离

拖拽（单节点或候选链行标签整组）SHALL 让**被拖对象本体**跟随光标实时移动，SHALL NOT 只移动一个替代指示物而让本体停在原地。

进入拖拽的 `pointerdown` SHALL 阻止浏览器默认行为，拖拽期间画布 SHALL NOT 选中任何文本（含节点常显标题与行标签）。

松手时被拖对象 SHALL 按既有落位规则落位（拖出本次泳道行松手 = 移出，沿用既有「移出可撤销」口径）；拖拽取消（Esc / `pointercancel` / 指针捕获丢失）SHALL 清空跟随位移并回到原位，SHALL NOT 产生成员变化。

#### Scenario: 拖单节点时本体跟随

- **WHEN** 用户拖动画布上的一个节点
- **THEN** 该节点 SHALL 随光标移动，其它节点 SHALL 保持原位

#### Scenario: 拖拽不选中文本

- **WHEN** 用户拖拽经过画布上其它节点的标题或行标签
- **THEN** 画布 SHALL NOT 出现文本选中高亮，页面选区 SHALL 保持不变

#### Scenario: 取消清空位移

- **WHEN** 拖拽中按 Esc 或触发 `pointercancel`
- **THEN** 被拖对象 SHALL 回到原位，跟随位移 SHALL 清空，SHALL NOT 改变成员集合

## MODIFIED Requirements

### Requirement: 候选话题侧栏
左侧主列表 SHALL 是候选泳道及其一系列事件，不是散装事件列表。候选 SHALL 按原candidate ID分组，使用当前范围内服务端原relations的连通片段（同ID可沿原identity或similarity边连接，不新增边）；同片最早到最晚报告日期差 SHALL 至少3天（UTC日差≥3），至少有两个节点，中间可以断档。不得按连续命中数、累计3次、至少3节点或仅同名/同ID但无连接替代该条件。不同candidate ID不得因相似边合并成一个原候选；同候选不连通片段不得混选。每条 SHALL 展示原名、真实成员、首末日期、事件数及连接跨度。

候选 SHALL 提供真实组复选框及成员独立复选框；未选/部分选/全选分别映射unchecked/indeterminate/checked，与成员ID、计数、图上已选勾号及保存可用性同步。勾组或点击未全选候选标题 SHALL 一次补齐该片段全部成员，追加去重、不截20条、不吞组外人工补选。取消组框、再点全选标题或点取消整组 SHALL 撤销该组全部成员，保留组外选择。名称为空可自动预填；自动预填名 SHALL 跟随它那一组，该组被取消且名字未被人工改写时 SHALL 一并清空；人工写过的名称 SHALL NOT 被任何预填或历史名单“带入草稿”覆盖。相似推荐 SHALL 与基础成员分开，不在整组采纳时偷偷自动勾选。编排面 SHALL NOT 提供「历史待启用话题」入口（2026-11-23 用户定档隐藏）；管理资格与 can_activate 的数据口径不变，能力保留但不经编排面触达。

#### Scenario: 跨期连接而非连续出现
- **GIVEN** 同一candidate的事件在1日与4日有原关系连接
- **THEN** SHALL 满足跨度3天，即使中间两日无事件、且仅两个节点

#### Scenario: 没连接或跨度不足
- **GIVEN** 多日事件无原连接，或有连接但日期差只有2天
- **THEN** SHALL 不作为合格候选片段；连续1/2/3日出现不等于跨越3天

#### Scenario: 采纳预填
- **WHEN** 点击包含3个成员的候选泳道
- **THEN** SHALL 一次选中这3个成员；不得附带勾选相似事件；明确勾选目标操作去重；再次点全选标题是取消整组，不得以旧追加幂等掩盖取消缺失；组外手动补选保留

#### Scenario: 取消整组与半选
- **WHEN** 逐个取消部分成员，或取消整组
- **THEN** SHALL 显示真实半选或未选状态，候选卡本身 SHALL 看出半选，图勾号/成员数同步；取消本组不得删除组外人工补选；被取消组的自动预填名 SHALL 一并清掉

#### Scenario: 人工名称不被覆盖
- **WHEN** 人工写过名称后，又选组、取消组或从历史名单带入草稿
- **THEN** SHALL 保留人工名称，不得被自动预填或历史带入静默替换

#### Scenario: 清空与取消本次
- **WHEN** 清空已选或取消本次编排
- **THEN** 前者 SHALL 清所有成员并保留手写名；后者有草稿先确认，不得直接丢弃

#### Scenario: 已有泳道显式参考
- **WHEN** 用户查看新建模式工具条
- **THEN** SHALL NOT 出现「显示已有泳道（只读）」控件；只读参照层 SHALL NOT 渲染；取消聚焦 SHALL NOT 改变任何参照开关

#### Scenario: 历史启用资格
- **WHEN** 用户查看编排面工具条
- **THEN** SHALL NOT 出现「历史待启用话题」入口，SHALL NOT 有其它路径在本界面打开历史管理名单；服务端 can_activate 与管理资格口径不变

