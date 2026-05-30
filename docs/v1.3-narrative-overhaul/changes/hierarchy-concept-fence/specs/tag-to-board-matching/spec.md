## MODIFIED Requirements

### Requirement: Embedding-based tag-to-board matching
MatchTagToConcept SHALL 从 topic_tag_embeddings 表读取 tag 的 semantic embedding（不复用 narrative 的 GenerateTagEmbedding 重新生成），与 board_concepts.embedding 做 cosine similarity 比较。匹配 SHALL 限定在同 category 的 concept 内。

#### Scenario: 复用已有 semantic embedding
- **WHEN** tag 有 semantic embedding 在 topic_tag_embeddings 表中
- **THEN** 直接使用该 embedding 与 concept embedding 比较，不调用 embedding API

#### Scenario: Category 过滤
- **WHEN** event 标签执行 MatchTagToConcept
- **THEN** 只与 category='event' 且 status='active' 的 concept 比较

#### Scenario: 无 embedding 时返回 nil
- **WHEN** tag 在 topic_tag_embeddings 表中没有 semantic embedding
- **THEN** MatchTagToConcept 返回 nil，不生成新 embedding

### Requirement: Unclassified bucket
此需求保持不变。Tags 无法匹配任何 concept 时放入 unclassified bucket。

## REMOVED Requirements

### Requirement: Board concept LLM cold-start suggestion
**Reason**: 替换为 concept bootstrap 聚类 + LLM 命名（见 concept-fence spec）
**Migration**: 使用 POST /api/hierarchy/concepts/bootstrap 触发聚类生成 concept 建议，替代 SuggestBoardConcepts
