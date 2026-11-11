## REMOVED Requirements

### Requirement: 日报后滚动态势结算

**Reason**:: 长短两版产物形态整体废除——长版（≤1000 字成段叙述）实测为逐日大事记复述，与同区块「逐日事件」展开职责完全重复；短版为「定性+事件罗列」，无趋势支撑。改为单版演进叙事态势句（见 ADDED 同名 Requirement）。

**Migration**: `topic_lane_snapshots` 表与列不变；`rolling_detail` 停止产出（固定写空），存量历史长版保留原值不清理不回填，下个日报日结算自然覆盖；读取方中，日报阅读视图趋势区（LaneTrendOverview）已随本 change 只读 `rolling_summary`；console 泳道演进带（BoardLaneEvolution）保留存量 `rolling_detail` 透出至该泳道下个日报日结算自愈（读旧值无害，新结算 detail 恒空后自然退化为 summary 全文）。

## ADDED Requirements

### Requirement: 日报后泳道态势演进结算

每日日报生成完成（该板块报告保存成功）后，系统 SHALL 为该板块的活跃泳道异步结算滚动 14 天态势：输入为该泳道近 14 天锚定 section 的「日期｜节标题｜thread 标题」清单，并附带**量级信号段**（系统计算的每日线索量与近 7 天 / 上个 7 天合计对比，非 LLM 产出、随素材注入）；输出为**单版演进叙事态势句**——一段 150-200 字的中文纯文本，按「阶段划分（带大致日期锚）→ 每阶段驱动/转折（事件仅作转折标记，不展开）→ 当前落点与局势判断 → 量级佐证」的演进叙事组织，随每日结算覆盖更新（每泳道保留一份当前快照，含 as_of 汇总截止日）。态势句 SHALL NOT 逐日罗列事件、SHALL NOT 预测未来走向（只描述已发生的变化）、SHALL NOT 编造清单与量级信号之外的数字。快照的 `rolling_detail` 字段 SHALL 固定写入空值（不再产长版）；存量快照的历史长版数据 SHALL 保留原值不清理不回填，下个日报日结算自然覆盖。单次结算调用的输出 token 预算 SHALL 覆盖单版上限。结算 SHALL NOT 阻塞或失败中断日报主流程（松耦合：失败仅记日志，下个日报日自愈）。

#### Scenario: 日报完成后结算

- **WHEN** 某板块当日日报生成成功
- **THEN** 系统 SHALL 异步为该板块活跃泳道逐个刷新滚动 14 天态势快照（单版演进叙事态势句，写入 `rolling_summary`；`rolling_detail` 置空）

#### Scenario: 量级信号随素材注入

- **WHEN** 结算调用构造 prompt 素材
- **THEN** 系统 SHALL 在事件清单之外附带量级信号段：窗口内每日线索量与近 7 天 / 上个 7 天合计对比，由系统按素材统计计算，SHALL NOT 依赖模型自行计数

#### Scenario: 演进叙事定位约束

- **WHEN** 结算调用构造 system 指令
- **THEN** 指令 SHALL 要求按阶段组织叙述（事件仅作转折标记）、SHALL 包含对逐日罗列与未来预测的显式禁止

#### Scenario: 机械截断保护

- **WHEN** 模型输出的态势句超过 200 字
- **THEN** 系统 SHALL 在 200 字窗口内最后一个句末标点（`。`/`！`/`？`/`…`）处收口后入库，SHALL NOT 从字中间切断；窗口内无可用句末标点（或标点过早，低于预算 60%）时 SHALL 退回 rune 安全截断

#### Scenario: 空输出守卫

- **WHEN** 模型输出为空或纯空白
- **THEN** 该泳道本次结算 SHALL 记为失败并跳过，快照保持旧值，下个日报日重试

#### Scenario: 结算失败不阻塞

- **WHEN** 某泳道结算 LLM 调用失败
- **THEN** 日报主流程 SHALL 不受影响，失败记日志；该泳道快照保持旧值（或缺失态），下个日报日重试

#### Scenario: 存量快照兼容

- **WHEN** 某泳道快照为存量数据（`rolling_detail` 含历史长版）
- **THEN** 系统 SHALL 保持该值原样入库不清理，读取方 SHALL 以 `rolling_summary` 为唯一态势内容源，下个日报日结算覆盖时 `rolling_detail` 自然置空

#### Scenario: 态势句与时间线同窗

- **THEN** 态势句的素材窗口与时间线窗口 SHALL 同为滚动 14 天（以最近一份已完成日报为期）

## REMOVED Requirements

### Requirement: 泳道动态批量端点

**Reason**: 响应契约中「长版叙述存在时携带、缺失时明示缺失」的两级语义随长短两版形态废除（slim-lane-snapshot-trend）；快照缺失单级降级保留（见 ADDED「泳道动态聚合端点」）。

**Migration**: 端点路径与响应形状不变；`detail` 字段仅存量历史值透出（omitempty），新结算恒空不再出现；消费方（板块内容卡片 / console 演进带 / 日报阅读趋势区）已按「存量自愈 + 只读 summary」适配。

## ADDED Requirements

### Requirement: 泳道动态聚合端点

系统 SHALL 提供 `GET /semantic-boards/:id/lane-dynamics?days=14` 端点，单次返回该板块全部泳道卡数据：泳道元信息（含 watch 关联标识）、滚动态势（`summary` 单版演进叙事态势句 + `as_of` 与快照存在性；`detail` 仅存量历史值透出，新结算恒空、omitempty 不出现）、逐日发展时间线（日期 → 事件 thread 标题列表）、候选清单。days 参数 SHALL 支持指定时间线窗口（默认 14）。

#### Scenario: 单请求聚合

- **WHEN** 前端请求某板块的 lane-dynamics
- **THEN** 响应 SHALL 包含主卡区全部数据与候选栏数据，前端无需再发 N 次逐泳道请求

#### Scenario: 存量长版透出

- **WHEN** 某泳道快照含存量历史长版（其泳道尚未经历新结算）
- **THEN** 响应中该泳道态势对象 SHALL 原样透出存量 `detail` 值（读旧值无害）；新结算后该字段恒空、不再出现，不存在「长版缺失」二级降级语义

#### Scenario: 无态势快照降级

- **WHEN** 某泳道无滚动态势快照（如新泳道或首个日报日前）
- **THEN** 响应 SHALL 明示快照缺失，前端 SHALL 展示「待结算」占位且发展时间线照常渲染
