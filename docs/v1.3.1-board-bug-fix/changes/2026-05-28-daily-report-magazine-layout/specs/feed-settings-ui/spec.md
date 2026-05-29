## ADDED Requirements

### Requirement: 文章时间筛选快捷 chip
TagsPage 文章 tab 的筛选区域 SHALL 在辅助标签 chips 下方新增一行快捷时间 chip：[今天] [3天] [7天] [30天]。点击 chip SHALL 自动计算 startDate/endDate 并触发筛选。默认 SHALL 选中「今天」。快捷 chip 行右侧保留原有的自定义 date input 作为 fallback。

#### Scenario: 默认选中今天
- **WHEN** 用户切换到某 board 的文章 tab
- **THEN** 时间筛选 SHALL 默认选中「今天」chip，startDate 设为当天日期，endDate 设为当天日期

#### Scenario: 点击快捷 chip
- **WHEN** 用户点击「7天」chip
- **THEN** startDate SHALL 设为 7 天前，endDate SHALL 设为今天，文章列表 SHALL 自动刷新

#### Scenario: 自定义日期仍可用
- **WHEN** 用户手动修改 date input 的值
- **THEN** 快捷 chip 的选中状态 SHALL 取消（全部取消高亮），文章列表 SHALL 按自定义日期筛选

#### Scenario: 切换 board 时重置
- **WHEN** 用户切换到另一个 board
- **THEN** 时间筛选 SHALL 重置为默认的「今天」
