## MODIFIED Requirements

### Requirement: evaluateSemanticBoardMatches accepts direction data

evaluateSemanticBoardMatches 函数签名新增两个参数：`tagEmbedding []float64`（tag identity embedding，可为 nil）和 `boardEmbeddings map[uint][]float64`（boardID → board embedding，缺失的 boardID 跳过方向校验）。仅 max_sim 规则匹配成功后执行方向校验。

#### Scenario: max_sim with direction data
- **WHEN** max_sim matches AND tagEmbedding != nil AND boardEmbeddings[boardID] exists
- **THEN** cosine(tagEmbedding, boardEmbeddings[boardID]) computed, direction_mismatch set accordingly

#### Scenario: max_sim without direction data
- **WHEN** tagEmbedding is nil OR boardEmbeddings[boardID] missing
- **THEN** direction check skipped, direction_mismatch=false

### Requirement: MatchTopicTag loads direction data

MatchTopicTag 在加载 tagAuxiliaries 和 boardAuxiliaries 之外，额外加载 tag identity embedding 和所有活跃 board embedding，传入 evaluateSemanticBoardMatches。

#### Scenario: successful load
- **WHEN** tag has identity embedding AND boards have embeddings
- **THEN** both passed to evaluateSemanticBoardMatches

#### Scenario: partial data
- **WHEN** some boards lack embeddings
- **THEN** only boards with embeddings included in boardEmbeddings map, matching proceeds normally
