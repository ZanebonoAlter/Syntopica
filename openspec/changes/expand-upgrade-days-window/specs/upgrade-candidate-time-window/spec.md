## MODIFIED Requirements

### Requirement: 前端时间窗口选择器
系统 SHALL 在升级建议弹窗的生成参数区提供「候选时间窗」下拉选择器，选项为：今天（days=1，默认）、最近3天（days=3）、最近7天（days=7）、最近30天（days=30）、全部（days=0）。该选择器 SHALL 对全部四格方向组合（创建×单标签 / 创建×组合 / 扩充×单标签 / 扩充×组合）可用，选择后 SHALL 以查询参数形式随生成请求传递。

#### Scenario: 扩充方向显示时间窗选择器
- **WHEN** 用户在升级建议弹窗选择「版块扩充」方向
- **THEN** 「候选时间窗」下拉 SHALL 保持可见可选，与创建方向一致

#### Scenario: 用户切换时间窗口
- **WHEN** 用户从下拉选择器中选择"最近7天"并点击"生成建议"（任意方向组合）
- **THEN** 前端 SHALL 调用生成 API 并携带 days=7 查询参数

#### Scenario: 组合标签方向不受时间窗影响
- **WHEN** 用户选择「创建×组合」或「扩充×组合」并携带 days
- **THEN** 请求 SHALL 正常发出（组合路候选以共现门槛为准，时间窗仅作用于共现窗口收紧，无独立过滤语义）

## ADDED Requirements

### Requirement: 扩充候选的时间窗过滤
版块扩充方向生成建议时，SHALL 支持可选查询参数 days（非负整数，默认按现状不过滤或前端默认值），并按以下规则作用于候选召回：

- **相似路**：与目标版块 embedding 相似达标的候选，SHALL 额外要求在 days 窗口内的文章中出现过（通过标签-文章关联判定）；days=0 时 SHALL NOT 施加该过滤。
- **共现路**：co-tag 窗口 cutoff SHALL 取「days 窗口」与「全局 CoTagWindowDays 配置窗口」中更严（更近）者；days=0 时 SHALL 使用全局配置窗口（与现状一致）。
- **组合路**：共现窗口收紧规则与共现路相同。

#### Scenario: days 收紧相似路
- **WHEN** 用户扩充版块「美债」且选择 days=7，aux「美债拍卖」相似度达标但近 7 天无文章引用
- **THEN**「美债拍卖」SHALL NOT 进入扩充候选

#### Scenario: days 收紧共现窗口
- **WHEN** 全局 CoTagWindowDays=30，用户选择 days=7
- **THEN** 共现统计窗口 SHALL 为最近 7 天（更严者生效）；选择 days=0 时 SHALL 保持 30 天窗口不变

#### Scenario: days=0 等于现状
- **WHEN** 用户扩充任一版块且选择"全部"（days=0）
- **THEN** 召回行为 SHALL 与本 change 之前完全一致（相似路全库、共现路用全局配置窗口）
