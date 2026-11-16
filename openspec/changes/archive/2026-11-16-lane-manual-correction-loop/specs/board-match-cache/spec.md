# board-match-cache — embeddings 缓存失效条件扩展 delta

## MODIFIED Requirements

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
