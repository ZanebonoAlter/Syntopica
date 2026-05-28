## Purpose

Event tag embedding 完成后自动触发 board 匹配，确保标签完成 embedding 后立即进行板块归属判定，无需手动触发。

## Requirements

### Requirement: Event tag embedding 完成后自动触发 board 匹配
系统 SHALL 在 `EmbeddingQueueService.processNext` 完成 event tag 的所有 embedding（identity、semantic、event_keyword）生成后，自动调用 `MatchTopicTag`。匹配结果 SHALL 写入 `topic_tag_board_labels`。

#### Scenario: 新 event tag embedding 完成后自动匹配
- **WHEN** event tag #500 的所有 embedding 生成完毕，且该 tag 有辅助标签
- **THEN** 系统 SHALL 自动调用 `MatchTopicTag(ctx, 500)`，匹配结果写入 `topic_tag_board_labels`

#### Scenario: tag 无辅助标签时跳过匹配
- **WHEN** event tag #501 的 embedding 生成完毕，但该 tag 没有任何辅助标签
- **THEN** 系统 SHALL NOT 调用 `MatchTopicTag`，直接标记 embedding task 完成

#### Scenario: 非 event tag 不触发匹配
- **WHEN** keyword tag #502 的 embedding 生成完毕
- **THEN** 系统 SHALL NOT 调用 `MatchTopicTag`

#### Scenario: 自动匹配失败不阻塞 embedding 完成
- **WHEN** `MatchTopicTag` 调用返回错误
- **THEN** 系统 SHALL 记录 warning 日志，仍将 embedding task 标记为 completed
