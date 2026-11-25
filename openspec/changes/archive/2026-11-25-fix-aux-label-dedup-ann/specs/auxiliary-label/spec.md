# auxiliary-label Delta

## ADDED Requirements

### Requirement: 辅助标签语义去重走数据库 ANN 召回

系统 SHALL 通过 `semantic_labels.merge_embedding` 上的 halfvec HNSW 部分索引召回 L2 候选，SHALL NOT 在进程内全量加载辅助标签向量逐条计算余弦；召回 SHALL 设置 `hnsw.ef_search` 不小于本次候选上限，召回查询所在事务 SHALL NOT 包含 embedder 调用。

- 索引 SHALL 只覆盖 `label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL` 的行；召回查询的 WHERE 谓词 SHALL 与该部分索引谓词一致。
- 索引表达式 SHALL 内嵌当前运行时 embedding 维度（`halfvec(dim)`）；维度变化时 SHALL 在建索引前清理存量旧维度行后重建，SHALL NOT 继续使用旧维度表达式索引；列维度过渡（`ALTER COLUMN … TYPE`）执行前 SHALL 先删除该索引。

#### Scenario: L2 候选召回走索引而非全量遍历

- **WHEN** 一次打标签请求产生 N 个 L1 未命中的辅助标签
- **THEN** 每个标签的 L2 判定 SHALL 由数据库索引检索完成，扫描行数 SHALL 与 active 辅助标签总数无关
- **AND** 进程 SHALL NOT 因去重而持有全部 active 辅助标签的 `merge_embedding` 向量副本

#### Scenario: 召回候选被截断时的兜底

- **WHEN** 首次召回的候选上限条候选经精确重排后全部不低于去重阈值
- **THEN** 系统 SHALL 以更大的候选上限（并相应提高 `hnsw.ef_search`）重查一次后再裁决

#### Scenario: 维度变化后索引重建

- **WHEN** embedding 运行时维度由 2560 变为其它值（更换模型）
- **THEN** 系统 SHALL 在存量旧维度行清理后重建索引以匹配新维度
- **AND** 存量维度与运行时维度不一致时 SHALL 显式报错并给出清理指引，SHALL NOT 声称「降级为顺序扫描且结果正确」
- **AND** SHALL NOT 继续使用旧维度表达式的索引
- **AND** 若需执行列维度过渡语句，该语句执行前旧索引 SHALL 已被删除

### Requirement: L2 阈值裁决使用原列精确距离

系统 SHALL 以召回候选在原 `vector` 列上的精确余弦距离做去重阈值裁决，SHALL NOT 用 halfvec 近似距离裁决。

- 候选裁决沿用既有口径：cosine ≥ 去重阈值、`RefCount` 大者优先、同 `RefCount` 取 `ID` 小者。
- 归并路径的其它不变量 SHALL 保持不变：只 addAlias、不改 Label、不重算 MergeEmbedding、不触碰 `embedding` / `merge_embedding` 列。

#### Scenario: 阈值判定不受 halfvec 近似误差影响

- **WHEN** 某候选的 halfvec 近似距离与精确 float32 距离分别落在去重阈值两侧
- **THEN** 系统 SHALL 以精确 float32 距离为准判定是否命中

#### Scenario: 归并结果与全量遍历一致

- **WHEN** 同一批样本标签分别用旧实现（进程内全量余弦）与新实现（召回 + 精确重排）判定
- **THEN** 阈值内的候选裁决 SHALL 一致（同一 label 归并到同一目标 ID，或同样判定为 L3 新建）
- **AND** 对账中出现的差异 SHALL 逐条记录并归因，SHALL NOT 默许

#### Scenario: 归并路径仍不触碰向量列

- **WHEN** L2 命中并执行 alias 归并
- **THEN** `embedding` 与 `merge_embedding` 列 SHALL 保持原值不变（沿用既有的「Alias 变更不得触碰向量列」要求）

### Requirement: L1 索引增量写入对并发读者安全

L1 精确匹配索引的增量写入 SHALL 与无锁读者互斥，SHALL NOT 在读者可能并发访问时原地写 map。

#### Scenario: 并发打标签不崩进程

- **WHEN** 打标队列并发执行多个 attach，其中一个走到 L3 新建（增量吸收）而其它正在做 L1 匹配
- **THEN** 系统 SHALL 不出现 `concurrent map read and map write` 致命错误
