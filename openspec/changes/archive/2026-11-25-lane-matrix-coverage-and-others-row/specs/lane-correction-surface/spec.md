## ADDED Requirements

### Requirement: 矩阵行头就地改名

除修正工作面标题外，系统 SHALL 在矩阵行头「⋯」菜单提供「重命名」入口：选中后该行泳道名 SHALL 就地变为输入框（不打开弹窗、不进入工作面、不改变选中泳道/选中日），Enter SHALL 提交，Esc 或失焦 SHALL 取消并恢复原名；空名 SHALL 被拒绝且保持编辑态。提交 SHALL 复用修正工作面同一条写入（`PATCH /daily-reports/topics/:id` 的 label），成功 SHALL 提示「泳道名已更新」并刷新泳道名单与「窗口内未进日报」覆盖清单；失败 SHALL 保留输入框并如实报错，MUST NOT 在前端自行改写显示名冒充成功。改名 SHALL NOT 触碰成员归属、首义、状态机或任何派生指标。

#### Scenario: 行头就地改名

- **GIVEN** 用户在矩阵某活跃泳道行头点 ⋯ →「重命名」
- **WHEN** 输入新名后按 Enter
- **THEN** 系统 SHALL 调既有 `PATCH /daily-reports/topics/:id` 更新 label，成功后该行与右轨/覆盖组显示新名；不打开任何对话框

#### Scenario: 取消与空名

- **WHEN** 用户按 Esc、点输入框外，或把名字清空后按 Enter
- **THEN** 前两者 SHALL 恢复原名并退出编辑；空名 SHALL 提示「泳道名不能为空」并保持编辑态

#### Scenario: 失败不伪装成功

- **WHEN** 改名请求失败
- **THEN** 输入框 SHALL 保持打开、原名单不刷新成新名，并如实提示失败原因
