## MODIFIED Requirements

### Requirement: Embedding 高相似度匹配降级为 candidates

`TagMatch` 方法中，当 embedding 余弦相似度达到 `HighSimilarity` 阈值时，SHALL 返回 `MatchType: "candidates"`（而非 `"exact"`），将匹配结果作为候选列表返回。`findOrCreateTag` 对 `candidates` 的已有行为是 fall through 到创建新 tag。

#### Scenario: embedding 高相似度不再返回 exact
- **WHEN** 一个标签通过 embedding 搜索匹配到已有标签，cosine 相似度为 0.96（≥ HighSimilarity 0.97）
- **THEN** `TagMatch` 返回 `MatchType: "candidates"`，`ExistingTag` 为 nil，`Candidates` 包含匹配结果
- **AND** `findOrCreateTag` 创建新的独立 tag

#### Scenario: slug 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 slug+category 在数据库中找到完全匹配的记录
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并（更新 label/slug 等）

#### Scenario: alias 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 alias 字段匹配到已有标签
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并

### Requirement: 删除 keyword 类别阈值覆盖

`CategoryThresholdOverrides` map SHALL NOT 包含 `"keyword"` 键。所有类别的 embedding 匹配统一使用 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）。

#### Scenario: keyword 类别使用默认阈值
- **WHEN** `ThresholdsForCategory("keyword")` 被调用
- **THEN** 返回 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）
