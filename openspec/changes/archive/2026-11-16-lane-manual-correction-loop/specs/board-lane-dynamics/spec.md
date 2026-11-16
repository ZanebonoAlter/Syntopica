# board-lane-dynamics — 矩阵解除只读红线 delta

## MODIFIED Requirements

### Requirement: 泳道动态视图入口

版块态势页（`/tags/:boardId`）SHALL 以「泳道 × 日期」矩阵作为版块选中后的默认首屏概览，取代原「板块内容」tab 内的泳道卡片区。矩阵的**日格** SHALL 保持只读展示（格内不提供任何写操作；治理右轨/抽屉的操作 SHALL NOT 触发矩阵以外的副作用）。矩阵的**行头** SHALL 提供泳道操作入口（见下），写操作 SHALL 统一由泳道编辑面板承载，SHALL NOT 在矩阵内就地改变话题生命周期状态。演进带作为矩阵的选中泳道明细区，与矩阵共用同一聚合响应。

矩阵行头 SHALL 提供「⋯」操作入口，至少包含「编辑泳道」项（打开泳道修正工作面，见 lane-correction-surface 能力）。行头操作 SHALL NOT 触发行选中、选中日变更或泳道选中副作用。

#### Scenario: 默认展示泳道动态区

- **WHEN** 用户选中某版块并进入版块态势页
- **THEN** 页面 SHALL 渲染泳道 × 日期矩阵（泳道动态视图），并在其下方渲染当前选中泳道的演进带

#### Scenario: 只读语义

- **WHEN** 用户点击矩阵的某个日格
- **THEN** 系统 SHALL 只改变当前选中日与选中格，SHALL NOT 在格内提供归档/转正/合并等生命周期写操作；日格 SHALL 保持只读，写操作统一由泳道编辑面板承载

#### Scenario: 行头提供编辑入口

- **WHEN** 用户查看矩阵某行的行头
- **THEN** 行头 SHALL 提供「⋯」操作入口，其中包含「编辑泳道」项，点击后 SHALL 打开泳道编辑面板

#### Scenario: 行头操作不误触发行选中

- **WHEN** 用户点击行头的「⋯」按钮
- **THEN** 系统 SHALL NOT 选中该行、SHALL NOT 改变当前选中日，SHALL NOT 触发泳道选中副作用

#### Scenario: 矩阵内不就地改生命周期

- **WHEN** 用户在矩阵区域操作
- **THEN** 系统 SHALL NOT 在矩阵内直接执行归档/转正/合并等话题生命周期写操作，此类写操作 SHALL 统一在泳道编辑面板内完成
