## MODIFIED Requirements

### Requirement: getTagMatchDetail returns direction_sim

getTagMatchDetail 响应新增 `direction_sim` 字段（float64 或 null）。计算方式：在 handler 层实时计算 cosine(tag identity embedding, board embedding)，**不在** `computeMatchDetail` 内部——direction_sim 是匹配后校验结果，非匹配过程的一部分。

#### Scenario: direction_sim available
- **WHEN** both tag identity embedding and board embedding exist
- **THEN** handler loads both embeddings, computes cosine, returns numeric direction_sim value

#### Scenario: direction_sim unavailable
- **WHEN** tag identity embedding or board embedding is NULL
- **THEN** response direction_sim is null
