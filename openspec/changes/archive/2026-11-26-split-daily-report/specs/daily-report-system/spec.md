## ADDED Requirements

### Requirement: 日报收集对无板块归属 tag 兜底补算

<!-- from: daily-report-match-fallback -->

系统 SHALL 在 `collectBoardTags` 执行时，对有辅助标签但无 `topic_tag_board_labels` 记录的 event tag 执行现场补算。补算 SHALL 调用 `MatchTopicTag`，匹配结果写入 `topic_tag_board_labels`，然后合并到日报输入。

#### Scenario: 正常情况不触发兜底
- **WHEN** 日期 2026-05-27 的所有 event tag 都有 `topic_tag_board_labels` 记录
- **THEN** `collectBoardTags` SHALL NOT 执行兜底补算，直接使用现有匹配结果

#### Scenario: 存在未匹配 tag 时兜底补算
- **WHEN** board #2853 在 2026-05-27 有 10 个 event tag，其中 3 个有辅助标签但无 `topic_tag_board_labels`
- **THEN** `collectBoardTags` SHALL 对这 3 个 tag 调用 `MatchTopicTag` 补算，将匹配到当前 board 的 tag 合并到日报输入中

#### Scenario: 补算失败不阻塞日报生成
- **WHEN** 兜底补算中某个 tag 的 `MatchTopicTag` 调用失败
- **THEN** 系统 SHALL 记录 warning 日志，跳过该 tag，继续生成日报

#### Scenario: 补算后 tag 仍未匹配到目标 board
- **WHEN** 兜底补算后某个 tag 匹配到了其他 board 但不匹配目标 board
- **THEN** 该 tag SHALL NOT 出现在目标 board 的日报中

### Requirement: 日报排除 direction_mismatch 标签

<!-- from: daily-report-match-fallback -->


collectBoardTags 的主查询和 fallback 补算均排除 direction_mismatch=true 的标签。主查询添加 `AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)` 条件。fallback 补算后过滤匹配结果时排除 direction_mismatch=true。

#### Scenario: 主查询排除 direction_mismatch
- **WHEN** tag 在 topic_tag_board_labels 中 direction_mismatch=true
- **THEN** tag 被排除出日报主查询结果

#### Scenario: fallback 排除 direction_mismatch
- **WHEN** fallback 补算产生的匹配结果 direction_mismatch=true
- **THEN** 该 tag 被排除出日报

## REMOVED Requirements

### Requirement: 日报生成 API — 异步触发
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报生成 WebSocket 进度广播
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报查询 API
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报时间线组件 BoardDailyReportTimeline（报纸布局）
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报生成进度前端
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: TagsPage 内容 Tab
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 定时任务复用
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 缺档日报自动补档
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报重建窗口守卫
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报推荐与历史兼容
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 重新分桶 API
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报调度窗口的生成时刻与兜底时刻可读
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报终态通知的计数口径与 WS 终态消息一致
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 站内阅读预览的全屏层必须盖过宿主浮层
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 阅读台线索文章直达站内阅读预览
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 刊头「今日重点」空态可见性
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 重点依据与正文定位
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报缺口可见性
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 日报缺口异步补填
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement

### Requirement: 补填运行留档与终态告知
**Reason**: 迁入族级 capability（结构重构，契约内容不变）

**Migration**: 见对应族 spec 中同名 Requirement
