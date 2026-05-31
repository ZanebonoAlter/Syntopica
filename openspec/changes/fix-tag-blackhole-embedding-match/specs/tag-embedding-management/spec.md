## MODIFIED Requirements

### Requirement: SaveEmbedding 清理同 tag 同 type 的旧记录

`SaveEmbedding` 在保存新 embedding 记录时，SHALL 删除同一 `topic_tag_id + embedding_type` 下 `text_hash` 不匹配的所有旧记录。

#### Scenario: 保存新 embedding 时清理旧记录
- **WHEN** tag 94712 已有 10 条 identity embedding（不同 text_hash），保存一条新的 identity embedding
- **THEN** 旧的 10 条记录被删除，只保留新的 1 条

#### Scenario: text_hash 匹配时更新而非清理
- **WHEN** 保存的 embedding 的 `topic_tag_id + embedding_type + text_hash` 与已有记录完全匹配
- **THEN** 更新已有记录（当前行为不变），不删除其他记录
