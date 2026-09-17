## Purpose

按文章活动时间窗口过滤升级候选辅助标签，允许用户选择不同时间范围收集候选。

## Requirements

### Requirement: 候选按文章活动时间过滤
系统 SHALL 在收集升级候选时，支持按文章活动时间（articles.created_at）过滤。只收集在指定时间窗口内的文章中，通过 article_topic_tags → topic_tag_semantic_labels 关联出现的候选辅助标签。时间过滤 SHALL 与 ref_count 阈值同时生效（双重过滤）。

#### Scenario: 默认只收集今天的候选
- **WHEN** 用户点击"升级建议"且未指定时间窗口
- **THEN** 系统 SHALL 只收集今天（now() - interval '1 day'）文章中出现的、满足 ref_count ≥ 5 且未归入已有 board 的辅助标签

#### Scenario: 用户选择最近7天
- **WHEN** 用户在时间窗口选择器中选择"最近7天"
- **THEN** 系统 SHALL 只收集最近7天文章中出现的候选辅助标签

#### Scenario: 用户选择全部
- **WHEN** 用户选择"全部"（days=0）
- **THEN** 系统 SHALL 不按时间过滤，收集所有满足 ref_count ≥ 5 且未归入已有 board 的辅助标签（行为与原系统相同）

#### Scenario: 时间过滤与引用门槛双重过滤
- **WHEN** 辅助标签 "华为"（ref_count=30）在今天的文章中出现，且未归入已有 board
- **THEN** 系统 SHALL 将其收集为候选（同时满足时间窗口和 ref_count 门槛）
- **WHEN** 辅助标签 "某冷门词"（ref_count=3）在今天的文章中出现
- **THEN** 系统 SHALL NOT 将其收集为候选（ref_count < 5，即使满足时间条件）

### Requirement: 升级建议 API 支持时间窗口参数
`POST /api/semantic-boards/upgrade-suggest` SHALL 接受可选查询参数 `days`（int，默认 1）。`days > 0` 表示按最近 N 天的文章活动时间过滤候选；`days = 0` 表示不过滤。

#### Scenario: API 调用带 days 参数
- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade-suggest?days=3`
- **THEN** 系统 SHALL 收集最近3天文章中出现的候选辅助标签，后续聚类和 LLM 判断流程不变

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
