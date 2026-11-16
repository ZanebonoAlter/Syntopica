## Purpose

Board matching 的内存缓存层，缓存版块辅助标签、embedding 和 AI 配置，减少重复 DB 查询和 parsePgVector 开销。

## Requirements

### Requirement: Board Auxiliary Labels Cache

`SemanticBoardMatchingService` SHALL 缓存 `loadBoardAuxiliaries` 的结果。首次调用时从 DB 加载并缓存；后续调用直接返回缓存值。缓存 SHALL 在 board composition 变更（upgrade confirm、手动合并）后失效。

#### Scenario: Cache miss loads from DB

- **WHEN** `MatchTopicTag` 首次调用时缓存为空
- **THEN** 系统 SHALL 从 DB 加载 board auxiliaries，存入缓存，并返回结果

#### Scenario: Cache hit returns cached data

- **WHEN** `MatchTopicTag` 第二次调用时缓存有效（未失效）
- **THEN** 系统 SHALL 直接返回缓存的 board auxiliaries，不执行 DB 查询

#### Scenario: Cache invalidates on upgrade confirm

- **WHEN** 用户确认一个 board upgrade 建议（create_new 或 merge_into_existing）
- **THEN** 系统 SHALL 清除 board auxiliaries 缓存，下次 `MatchTopicTag` 调用时重新从 DB 加载

### Requirement: Board Embeddings Cache

`SemanticBoardMatchingService` SHALL 缓存 `loadBoardEmbeddings` 的结果（已解析为 `map[uint][]float64`）。缓存 SHALL 在以下任一变更后失效，与 board auxiliaries 缓存同生命周期：

- board composition 变更（upgrade confirm、手动合并、手动增删构成标签）；
- **版块 label 或 description 变更**——该变更会重算版块 embedding（`embed("label. description")`），缓存中的旧向量 SHALL NOT 继续被方向校验使用。

缓存失效 SHALL 在版块本体更新请求的同一请求内完成（更新成功后即失效），SHALL NOT 依赖进程重启或 TTL 过期。

#### Scenario: Cache eliminates parsePgVector overhead

- **WHEN** `MatchTopicTag` 第二次调用时 board embeddings 缓存有效
- **THEN** 系统 SHALL 直接返回缓存的 `map[uint][]float64`，不执行 DB 查询和 `parsePgVector` 解析

#### Scenario: Cache invalidates with auxiliaries

- **WHEN** board composition 变更触发缓存失效
- **THEN** 系统 SHALL 同时清除 board embeddings 缓存

#### Scenario: Cache invalidates on board label change

- **GIVEN** board #1980 的 embedding 缓存中存有向量 E_old
- **WHEN** 用户通过 `PUT /api/semantic-boards/:id` 修改该版块的 label，接口重算出 E_new
- **THEN** 系统 SHALL 在该请求内清除 board embeddings 缓存，下次 `MatchTopicTag` SHALL 重新从 DB 加载 E_new，SHALL NOT 继续使用 E_old 做方向校验

#### Scenario: Cache invalidates on board description change

- **WHEN** 用户修改版块 description（接口重算 embedding）
- **THEN** 系统 SHALL 同样清除 board embeddings 缓存，SHALL NOT 因 description 变更而保留旧向量

#### Scenario: 缓存无 TTL 场景下的失效保证

- **GIVEN** board embeddings 缓存无 TTL（只靠显式失效）
- **WHEN** 版块本体更新接口返回成功
- **THEN** 系统 SHALL 已完成缓存失效，SHALL NOT 留下陈旧向量继续参与方向校验

### Requirement: AI Config Cache (TTL)

`SemanticBoardMatchingService` SHALL 缓存 `loadConfig` 的结果，TTL 为 5 分钟。过期后下次调用重新从 `ai_settings` 加载。

#### Scenario: Config returned from cache within TTL

- **WHEN** 距上次 `loadConfig` 加载不到 5 分钟
- **THEN** 系统 SHALL 返回缓存的 `SemanticBoardMatchConfig`，不查询 `ai_settings`

#### Scenario: Config reloaded after TTL

- **WHEN** 距上次 `loadConfig` 加载超过 5 分钟
- **THEN** 系统 SHALL 重新从 `ai_settings` 加载 13 行配置，更新缓存并重置 TTL

### Requirement: Active Auxiliary Labels Cache (TTL)

`AuxiliaryLabelService` SHALL 缓存 `loadActiveAuxiliaryLabels` 的结果，TTL 为 5 分钟。过期后下次调用重新从 DB 加载。缓存 SHALL 在新增或禁用辅助标签时立即失效。

#### Scenario: Labels returned from cache within TTL

- **WHEN** 距上次 `loadActiveAuxiliaryLabels` 加载不到 5 分钟，且 `ResolveAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 使用缓存的标签列表进行匹配，不执行全表扫描

#### Scenario: Labels reloaded after TTL

- **WHEN** 距上次加载超过 5 分钟，且 `ResolveAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 重新执行 `loadActiveAuxiliaryLabels` 全表扫描，更新缓存

#### Scenario: Cache invalidated on label creation

- **WHEN** `ResolveAuxiliaryLabel` 的 L3 阶段创建了新标签
- **THEN** 系统 SHALL 清除 active labels 缓存，下次调用重新从 DB 加载

#### Scenario: Cache invalidated on label disable

- **WHEN** `DisableAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 清除 active labels 缓存

### Requirement: Merge Embedding 向量缓存

`AuxiliaryLabelService` SHALL 缓存所有活跃辅助标签的 `merge_embedding` 向量（已解析为 `map[uint][]float64`），TTL 为 10 分钟。`sqlMergeMatcher` SHALL 优先使用缓存数据，避免每次调用都从 DB 加载 ~216 MB 向量数据。

#### Scenario: sqlMergeMatcher 使用缓存向量

- **WHEN** `ResolveAuxiliaryLabel` 的 L2 阶段调用 `sqlMergeMatcher`，且 merge embedding 缓存有效
- **THEN** 系统 SHALL 使用缓存的 `map[uint][]float64` 计算余弦相似度，不执行 `SELECT id, merge_embedding` DB 查询

#### Scenario: 首次调用或缓存过期时从 DB 加载

- **WHEN** merge embedding 缓存为空或已过期（超过 10 分钟）
- **THEN** 系统 SHALL 执行 `SELECT id, merge_embedding FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`，调用 `ParsePgVector` 解析每行，存入缓存

#### Scenario: 缓存在标签新增时失效

- **WHEN** `ResolveAuxiliaryLabel` 的 L3 阶段创建了新标签
- **THEN** 系统 SHALL 同时清除 merge embedding 缓存和 active labels 缓存

#### Scenario: 缓存在标签禁用时失效

- **WHEN** `DisableAuxiliaryLabel` 被调用
- **THEN** 系统 SHALL 同时清除 merge embedding 缓存和 active labels 缓存

#### Scenario: 添加别名不影响 merge embedding 缓存

- **WHEN** `addAlias` 被调用（标签的 aliases 字段变化）
- **THEN** 系统 SHALL 清除 active labels 缓存，但保留 merge embedding 缓存（向量数据未变化）

#### Scenario: parsePgVector 仅在缓存加载时调用

- **WHEN** merge embedding 缓存有效，10 次 `ResolveAuxiliaryLabel` 调用发生
- **THEN** 系统 SHALL NOT 为 merge embeddings 调用 `ParsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Thread Safety

所有缓存 SHALL 使用 `sync.RWMutex` 保护，支持并发读和互斥写。多个 goroutine 同时调用 `MatchTopicTag`（backfill 并行场景）SHALL NOT 产生数据竞争。

#### Scenario: Concurrent reads during parallel backfill

- **WHEN** 4 个 backfill worker 同时调用 `MatchTopicTag`
- **THEN** 所有 worker SHALL 成功读取缓存，无 panic 或数据竞争

#### Scenario: Concurrent read and cache invalidation

- **WHEN** 一个 goroutine 正在读取缓存，另一个 goroutine 触发缓存失效
- **THEN** 读操作 SHALL 返回失效前的旧数据或等待失效完成后重新加载，不产生竞争条件
