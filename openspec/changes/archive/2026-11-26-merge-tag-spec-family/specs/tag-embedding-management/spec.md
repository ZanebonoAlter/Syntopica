## REMOVED Requirements

### Requirement: SaveEmbedding 清理同 tag 同 type 的旧记录
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: 所有 tagmanagement 测试迁移到 Postgres
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: embedding 测试使用真实 Postgres + pgvector
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: auxiliary_label_service 测试使用 Postgres
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: Board Embeddings 缓存存储已解析数据
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: Merge Embeddings 缓存存储已解析数据
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: Tag Embedding 不缓存
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: tag 删除时 embedding 级联清理（DB FK 兜底）
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: 存量孤儿 embedding 一次性清理
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: embedding 模型名列宽不窄于模型注册表
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: 标签相似查询使用 halfvec ANN 两段式
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: embedding 队列 worker 连续处理且有限并发
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: 队列任务批量调用 embedding
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement

### Requirement: 失败自动重试与可观测
**Reason**: 合并到族级 capability `tag-matching`（结构重构，契约内容不变）

**Migration**: 见 `openspec/specs/tag-matching/spec.md` 中同名 Requirement
