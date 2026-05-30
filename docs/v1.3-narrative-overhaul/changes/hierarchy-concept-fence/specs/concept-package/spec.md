## ADDED Requirements

### Requirement: Concept 独立包
系统 SHALL 将 concept 相关代码从 narrative 包迁移到新的 domain/concept/ 包。

#### Scenario: Concept 包文件结构
- **WHEN** concept 包创建完成
- **THEN** 包含 service.go (CRUD)、matcher.go (MatchTagToConcept)、bootstrap.go (聚类+LLM命名)、handler.go (API)、embedding.go (concept embedding 生成)

### Requirement: Narrative 包引用 concept 包
narrative 包 SHALL import concept 包使用 MatchTagToConcept，不再自己维护 concept 匹配逻辑。

#### Scenario: GenerateAndSaveForCategory 使用 concept 包
- **WHEN** narrative 的 GenerateAndSaveForCategory 需要匹配 concept
- **THEN** 调用 concept.MatchTagToConcept 而非 narrative 内部的实现

### Requirement: Tagging 包引用 concept 包
tagging 包 SHALL import concept 包使用 MatchTagToConcept 和 bootstrap。

#### Scenario: PlaceTagInHierarchy 使用 concept 包
- **WHEN** PlaceTagInHierarchy 需要 MatchTagToConcept
- **THEN** 调用 concept.MatchTagToConcept

### Requirement: Concept API 路由迁移
concept 的 API 路由 SHALL 从 /api/narratives/board-concepts 迁移到 /api/hierarchy/concepts。

#### Scenario: 新路由注册
- **WHEN** 服务启动
- **THEN** concept handler 注册在 /api/hierarchy/concepts 路径下
