## MODIFIED Requirements

### Requirement: 分类版块列表数据源

`GetScopes` API SHALL 从 `narrative_boards` 表聚合分类信息，而非从 `narrative_summaries`。

返回值中 SHALL 使用 `board_count` 字段（替代原 `narrative_count`），表示该分类下有多少个 board。

#### Scenario: 有 board 但无 summary 的分类也能展示

- **WHEN** 某分类下有 3 个 narrative_boards 但 0 条 narrative_summaries
- **THEN** `GetScopes` 返回该分类，`board_count` 为 3

#### Scenario: 分类列表展示 board 数

- **WHEN** 前端调用 `GET /api/narratives/scopes?date=2026-04-30`
- **THEN** 响应中每个 category 对象的 `board_count` 字段表示 board 数量

### Requirement: GetScopes 支持多日范围

`GetScopes` SHALL 接受可选的 `days` 查询参数（默认 7），返回该时间范围内有 boards 的分类。

#### Scenario: 默认 7 天范围

- **WHEN** 前端调用 `GET /api/narratives/scopes?date=2026-04-30` 不带 days 参数
- **THEN** 返回 2026-04-24 至 2026-04-30 范围内有 boards 的分类

#### Scenario: 自定义天数

- **WHEN** 前端调用 `GET /api/narratives/scopes?date=2026-04-30&days=3`
- **THEN** 返回 2026-04-28 至 2026-04-30 范围内有 boards 的分类

## ADDED Requirements

### Requirement: 切换到分类模式时加载分类列表

前端 `switchScope('category')` 函数 SHALL 调用 `loadScopes()` 获取分类数据。

#### Scenario: 从全局切换到分类模式

- **WHEN** 用户点击"分类版块"按钮
- **THEN** `loadScopes()` 被调用，分类列表正确展示有 board 数据的分类行
