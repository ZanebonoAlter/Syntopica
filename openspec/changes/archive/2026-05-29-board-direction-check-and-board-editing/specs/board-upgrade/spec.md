## MODIFIED Requirements

### Requirement: LLM upgrade create_new generates board embedding

`semantic_board_upgrade.go` 的 Confirm 方法中，`create_new` 分支创建 SemanticLabel 时必须调用 embedder 生成 embedding。当前代码（L157-167）直接 `Create(&board)` 未调用 embedder，导致所有 LLM 建议创建的板块 embedding 为 NULL。

#### Scenario: create_new with embedding
- **WHEN** user confirms a create_new suggestion
- **THEN** system calls semanticBoardLabelEmbedder(label + ". " + description) to generate embedding (description 为空时仅用 label)，saves board with embedding populated

#### Scenario: embedding generation fails
- **WHEN** embedder returns error during create_new
- **THEN** confirmation fails with error, board NOT created
