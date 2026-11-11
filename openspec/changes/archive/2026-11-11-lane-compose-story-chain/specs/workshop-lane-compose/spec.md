## MODIFIED Requirements

### Requirement: 候选话题侧栏
左侧主列表 SHALL 是候选泳道及其一系列事件，不是散装事件列表。候选 SHALL 按原candidate ID分组，使用当前范围内服务端原relations的连通片段（同ID可沿原identity或similarity边连接，不新增边）；同片最早到最晚报告日期差 SHALL 至少3天（UTC日差≥3），至少有两个节点，中间可以断档。不得按连续命中数、累计3次、至少3节点或仅同名/同ID但无连接替代该条件。不同candidate ID不得因相似边合并成一个原候选；同候选不连通片段不得混选。每条 SHALL 展示原名、真实成员、首末日期、事件数及连接跨度。

候选 SHALL 提供真实组复选框及成员独立复选框；未选/部分选/全选分别映射unchecked/indeterminate/checked，与成员ID、计数、图上已选勾号及保存可用性同步。勾组或点击未全选候选标题 SHALL 一次补齐该片段全部成员，追加去重、不截20条、不吞组外人工补选。取消组框、再点全选标题或点取消整组 SHALL 撤销该组全部成员，保留组外选择。名称为空可自动预填；自动预填名 SHALL 跟随它那一组，该组被取消且名字未被人工改写时 SHALL 一并清空；人工写过的名称 SHALL NOT 被任何预填或历史名单“带入草稿”覆盖。相似推荐 SHALL 与基础成员分开，不在整组采纳时偷偷自动勾选。历史管理名单 SHALL 通过明确入口保留，直接启用资格沿服务端can_activate，不改全局累计命中/状态机规则。

左侧主列表 SHALL 提供两条并列的候选维度：**话题链**（既有，按 candidate 话题分组与原 relations 连通片段切分）与**事件线链**（按存活事件线分组，详见「事件线链（跨天同事件选材维度）」需求）。两条维度 SHALL 共用同一份「本次成员」集合与同一套组复选框/成员复选框三态逻辑，但各自独立分组、独立组标题与独立元信息。两条链的分组键正交：同一分区 SHALL 可同时属于一条话题链与多条事件线链（也可只属于其中一条）；勾选任一条链 SHALL 只按成员 ID 去重合并进本次成员，SHALL NOT 改写另一条链的分组、连接、跨度或资格。事件线维度 SHALL NOT 改变话题链的既有分组与合格条件（≥2 节点、UTC 日差≥3），事件线链的合格条件 SHALL 与话题链同口径。两条链的呈现 SHALL 至少通过组图标与文字标签区分（事件线链 SHALL 带「事件线」字样），颜色 SHALL NOT 是唯一区分手段。

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
- **WHEN** 用户开启已有泳道显示或取消当前聚焦
- **THEN** 仅前者 SHALL 改变参照开关；取消聚焦保持用户当前偏好，不偷偷开/关参照

#### Scenario: 历史启用资格
- **WHEN** 打开历史管理候选
- **THEN** 原启用动作 SHALL 仅沿服务端can_activate；本界面连接跨度不改写管理资格或直接PATCH状态

#### Scenario: 两条链并列且互不干扰
- **GIVEN** 同一批分区同时构成一条话题链与一条事件线链，且两链成员有交集
- **WHEN** 用户在两条维度之间切换并分别勾选组框
- **THEN** 两条链 SHALL 各自独立呈现且组标题/图标/文字标签可区分；勾选 SHALL 只按成员 ID 去重合并进本次成员，SHALL NOT 改写另一条链的分组、连接、跨度或资格，SHALL NOT 让一条链的取消清掉另一条链独有的成员

#### Scenario: 事件线维度不可用时诚实降级
- **GIVEN** 服务端返回该版块事件身份未回填且无任何事件线链
- **WHEN** 用户查看左侧候选栏
- **THEN** 事件线维度 SHALL 明示「事件身份未回填，本板暂无事件线链」并不可选，SHALL NOT 以「暂无候选」冒充，SHALL NOT 影响话题链维度的可用性与既有空态


### Requirement: 成员质心与关联推荐
推荐 SHALL 基于当前全部有效已选成员向量的均值，不是首个成员、点击焦点、旧候选Embedding或后台滚动Centroid。每次加入/移除 SHALL 重算。对范围内未选事件 SHALL 计算余弦距离，以接口原matchThreshold筛选并按距离/ID稳定排序；未入active正式泳道的事件（含其他candidate成员） SHALL 为优先组，active仅次级只读参照。推荐不是原关系邻接列表，不要求已有连线；对待选推荐 SHALL 按真实成员质心余弦距离有序映射颜色深浅与点大小（半径可受间距上限约束），越近不更浅/不更小；不得把它当置信度。已选 SHALL 另用实心勾号/原生复选反馈，与相近程度区分，不能让大点被误认成选中；推荐点半径 SHALL NOT 大于已选点半径，大小只表达接近程度，选中只由实心＋✓表达；不生成事件关系边。推荐展示可以折叠，但不得把未展开项当不存在。

事件线链 SHALL NOT 参与本推荐的召回与排序：事件线身份 SHALL NOT 进入「相似 section 推荐」的候选集、SHALL NOT 改变 matchThreshold 筛选口径、SHALL NOT 改变距离/ID 稳定排序、SHALL NOT 参与质心向量计算；事件线链只提供「整组勾选」入口，SHALL NOT 被当作置信度或相近程度的替代表达。

#### Scenario: 采纳后质量刷新
- **WHEN** 候选3个成员整组带入
- **THEN** SHALL 用这3个向量平均产生待选推荐；有效均值来源可查看，不新增默认质量评分墙

#### Scenario: 点击节点推荐关联
- **WHEN** 仅点击一个节点查看
- **THEN** 可浏览该节点原关系，但 SHALL 不替换成员均值或把焦点当推荐质心

#### Scenario: 从推荐项加入
- **WHEN** 明确加入一个优先推荐
- **THEN** SHALL 才将其选中、从待选组移除并按新成员均值重新推荐

#### Scenario: 无关联
- **WHEN** 没有符合阈值的补充推荐
- **THEN** SHALL 展示空态，人工补选仍可用，不凑推荐

#### Scenario: 无距离数据
- **WHEN** 无有效成员向量或向量不可比较
- **THEN** SHALL 不伪造距离/相似推荐，不自动取消已选成员

#### Scenario: 相近程度不等于已选
- **WHEN** 均值推荐刷新
- **THEN** 待选点颜色/大小 SHALL 由实际距离计算，只有明确加入才有勾号；不得自动勾选近点

#### Scenario: 事件线不进入相似推荐召回
- **GIVEN** 某分区与已选成员同属一条事件线，但与已选质心的 cosine 距离大于 matchThreshold
- **WHEN** 刷新「可补充的事件」推荐
- **THEN** 该分区 SHALL NOT 因事件线身份进入推荐集、SHALL NOT 改变既有距离排序与阈值，SHALL NOT 以「同一事件」名义被标为相近或被自动勾选


### Requirement: 保存与只读边界
合法人工保存 SHALL 创建active/manual话题，不受candidate启用门禁限制，初始向量取有效所选均值；不得把创建本次草稿混为启用原candidate。名称空白 SHALL 拒绝；关闭命名 SHALL 保留草稿。active节点 SHALL 只读，任何跨正式泳道移入须另行审批，不在本轮静默执行。状态转正/归档与历史快照规则不变。

事件线链的整组带入 SHALL NOT 产生、合并、激活或改写任何话题：SHALL NOT 写 `board_persistent_topics` 的 status/hit_count/consecutive_hits/last_seen_date 等任何字段，SHALL NOT 改 `daily_report_sections.persistent_topic_id`，SHALL NOT 绕过 candidate 的连续命中门禁，SHALL NOT 改变「手动建话题直落 active」的既有路径；保存仍只经既有 active/manual 路径创建本次泳道话题。

#### Scenario: 缺名称
- **WHEN** 输入空串或纯空白名称并提交
- **THEN** SHALL 显示错误并保留成员，不执行创建

#### Scenario: 保存成功
- **WHEN** 合法人工创建成功
- **THEN** 产品 SHALL 返回原上下文并更新新泳道；本轮离线原型仅明确模拟，不可声称真实落库

#### Scenario: 事件线链整组带入不改候选状态机
- **GIVEN** 用户按一条事件线链整组勾选跨天分区后保存
- **THEN** SHALL 只按既有 active/manual 路径创建本次泳道话题；事件线涉及的候选话题其命中计数、连续命中与 status SHALL NOT 变化，SHALL NOT 出现任何候选因「同一事件」被转 active，SHALL NOT 改写这些分区原的话题归属


## ADDED Requirements

### Requirement: 事件线链（跨天同事件选材维度）
系统 SHALL 提供「事件线链」作为与话题链并列的候选维度，让用户按「同一事件」一次勾齐跨天材料。事件线链 SHALL 按**存活事件线 id** 分组：窗口内任一分区，只要其承载的线索（thread）归属于同一存活事件线（`daily_report_threads.story_id` 经 `board_stories.merged_into` **一跳**解析后的存活 id），即归入该事件线链；一个分区可承载多个事件，故同一分区 SHALL 可同时属于多条事件线链。

成链门槛 SHALL 与话题链同口径：成员分区数 SHALL ≥2，且首末成员的报告日期 UTC 日差 SHALL ≥3（中间允许断档）；窗口内分区跨度不足 3 天或只有 1 个分区的事件线 SHALL NOT 成为事件线链。事件线自身 `board_stories.first_seen_date/last_seen_date` 覆盖的窗口外分区 SHALL NOT 被拉进链；事件线链 SHALL 使用与候选、整组成员、画布、质心推荐**完全相同的生效窗口**。

成员域 SHALL 与编排态可勾选域一致：事件线链成员 SHALL 取窗口内带存活事件线身份、且其归属话题**不是 active** 的分区（candidate 话题分区、未归属分区、已归档来源分区均可入链）；active 泳道分区 SHALL NOT 进入事件线链（仍是画布上的只读参照）。

事件线链 SHALL 只是**辅助选材维度**：SHALL NOT 产生、合并、激活或改写任何话题，SHALL NOT 改写 `daily_report_sections.persistent_topic_id`，SHALL NOT 影响候选话题的连续命中门禁与手动建话题直落 active 的路径；SHALL NOT 参与质心召回与相似推荐排序（见「成员质心与关联推荐」）；SHALL NOT 写入 `daily_report_section_relations` 或任何新增表列，SHALL NOT 改变既有 section status 动态推导与 lifecycle API 的边集。

服务端 SHALL 只读消费事件线身份。板内事件身份**未回填**（窗口内所有线索 `story_id` 为 NULL）时 SHALL 返回「未回填」标记与空事件线链，界面 SHALL 明示该降级（SHALL NOT 冒充无候选）；**部分回填**时 SHALL 额外标记不完整。坏引用、两层合并链或环 SHALL 跳过该线索并记录告警，SHALL NOT 影响其它链的构造。

#### Scenario: 同一事件跨天直连成链
- **GIVEN** 同一存活事件线的线索分别落在第 1 日与第 4 日的两个分区，且两个分区分属不同 candidate 话题
- **WHEN** 服务端组装事件线链
- **THEN** SHALL 产生一条以该存活事件线为键的链，成员为这 2 个分区、跨度记 3 天，即使两日之间没有该事件的分区；两条链的分组键 SHALL 正交，不得因此把两个话题合并或让事件线链替代话题链

#### Scenario: 成员数或跨度不足不成链
- **GIVEN** 同一事件线在窗口内只落在相邻两日（UTC 日差 1），或窗口内只有 1 个带该事件线的分区
- **THEN** SHALL 不产生该事件线链；连续多日出现但未跨 3 天不等于跨天成链，事件线自身的 first_seen_date/last_seen_date SHALL NOT 把窗口外分区拉进来充数

#### Scenario: 链键按存活事件线归一
- **GIVEN** 事件线 A 已合并进存活事件线 B，另有一条线索指向坏引用或两层合并链
- **WHEN** 组装事件线链
- **THEN** 归属于 A 与 B 的分区 SHALL 归入同一条以存活 id 为键的链；坏引用与两层链 SHALL 跳过该线索并记录告警，SHALL NOT 影响其它链，SHALL NOT 按原始 id 分裂成两条链

#### Scenario: 一个分区可同属多条事件线链
- **GIVEN** 某分区同时承载了事件线 X 与事件线 Y 的线索，且 X、Y 各自在窗口内还有其它跨天分区
- **WHEN** 组装事件线链
- **THEN** 该分区 SHALL 同时出现在 X 链与 Y 链的成员里，SHALL NOT 被强行拆成两份分区副本，SHALL NOT 因此把 X 与 Y 判为同一事件线

#### Scenario: 事件线链只提供整组勾选
- **WHEN** 用户按一条事件线链的组复选框或标题整组勾选
- **THEN** SHALL 一次补齐该链全部成员（按成员 ID 去重合并进本次成员），SHALL NOT 附带勾选其它链的成员，SHALL NOT 附带勾选相似推荐项；取消该链 SHALL 只撤销该链成员，保留其它链与人工补选

#### Scenario: 事件线链不产生或激活话题
- **GIVEN** 用户按一条事件线链整组勾选跨天分区后保存
- **THEN** SHALL 只按既有 active/manual 路径创建本次泳道话题；候选话题的命中计数、连续命中与 status SHALL NOT 变化，SHALL NOT 出现任何候选因「同一事件」被转 active，SHALL NOT 改写这些分区原的话题归属

#### Scenario: 未回填板诚实降级
- **GIVEN** 板内窗口内所有线索的 `story_id` 均为 NULL
- **WHEN** 用户打开编排态的事件线维度
- **THEN** SHALL 显示「事件身份未回填，本板暂无事件线链」并禁用该维度，SHALL NOT 显示「暂无候选」，SHALL NOT 影响话题链维度与其既有空态

#### Scenario: 部分回填标记不完整
- **GIVEN** 板内窗口内部分线索已回填事件线、部分仍为 NULL
- **WHEN** 用户打开编排态的事件线维度
- **THEN** SHALL 在既有事件线链基础上给出「部分事件未回填，事件线链不完整」提示，SHALL NOT 把未回填线索伪造成独立事件线，SHALL NOT 静默隐藏该不完整状态

#### Scenario: 事件线链不写入关系表
- **WHEN** 服务端组装事件线链并返回编排态取数结果
- **THEN** SHALL 只返回内存构造的事件线链与未回填/不完整标记，SHALL NOT 向 `daily_report_section_relations` 写入任何新边、SHALL NOT 改变返回的原 relations 边集、SHALL NOT 新增表列
