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
系统 SHALL 在"升级建议"操作入口旁提供时间窗口下拉选择器，选项为：今天（days=1，默认）、最近3天（days=3）、最近7天（days=7）、最近30天（days=30）、全部（days=0）。用户选择后 SHALL 以查询参数形式传递给升级建议 API。

#### Scenario: 用户切换时间窗口
- **WHEN** 用户从下拉选择器中选择"最近7天"并点击"升级建议"
- **THEN** 前端 SHALL 调用 `POST /api/semantic-boards/upgrade-suggest?days=7`
