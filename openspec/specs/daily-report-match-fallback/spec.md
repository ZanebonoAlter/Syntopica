## Purpose

日报收集时对有辅助标签但无板块归属记录的 event tag 执行现场补算，避免因自动匹配遗漏导致日报数据不完整。

## Requirements

### Requirement: 日报收集对无板块归属 tag 兜底补算
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

collectBoardTags 的主查询和 fallback 补算均排除 direction_mismatch=true 的标签。主查询添加 `AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)` 条件。fallback 补算后过滤匹配结果时排除 direction_mismatch=true。

#### Scenario: 主查询排除 direction_mismatch
- **WHEN** tag 在 topic_tag_board_labels 中 direction_mismatch=true
- **THEN** tag 被排除出日报主查询结果

#### Scenario: fallback 排除 direction_mismatch
- **WHEN** fallback 补算产生的匹配结果 direction_mismatch=true
- **THEN** 该 tag 被排除出日报
