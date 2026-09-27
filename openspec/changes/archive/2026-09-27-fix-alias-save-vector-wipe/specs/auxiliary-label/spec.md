## ADDED Requirements

### Requirement: Alias 变更不得触碰向量列

辅助标签的 alias 积累路径（L2 归并 addAlias、手动 alias 合并）SHALL 仅更新 aliases 列（及归属维护所需的 ref_count/状态列），SHALL NOT 写入或清空 embedding / merge_embedding 向量列。已入库向量的「吸引力」以既有向量为准，alias 变更不改变其物理表示（对应 flow 红线：L2 命中只 addAlias、不改 Label、不重算 MergeEmbedding）。

#### Scenario: L2 归并新别名后向量保持不变

- **WHEN** 新辅助标签文本 "高市早苗政府" 的 merge embedding 与已有标签 "高市早苗"（embedding/merge_embedding 均非 NULL）cosine ≥ 去重阈值，触发 L2 归并加入 aliases
- **THEN** 该已有标签的 embedding 与 merge_embedding 保持归并前的值不变，仅 aliases 列追加 "高市早苗政府"

#### Scenario: 手动 alias 合并后目标标签向量保持不变

- **WHEN** 用户将源辅助标签的 label/aliases 合并进目标辅助标签（目标标签向量非 NULL）
- **THEN** 目标标签的 embedding / merge_embedding 保持合并前的值不变

#### Scenario: 归并路径不产生 active 且向量缺失的行

- **WHEN** 任意数量的别名变体依次归并到同一已有标签（含并发打标批处理场景）
- **THEN** 该标签行始终满足 status=active ⇒ embedding IS NOT NULL（不出现 active 但向量为 NULL 的静默损坏行）

### Requirement: 死向量回填（active aux 向量重建）

系统 SHALL 提供手动触发的辅助标签死向量回填端点：异步扫描 `status=active AND label_type=auxiliary` 且 embedding 或 merge_embedding 为 NULL 的行，按 L3 同款生成规则重建向量（storage embedding 由 "label: description"（有 description 时，否则仅 label）生成、merge embedding 由 label 生成），逐行独立容错。该端点 SHALL 幂等：已有向量的行不重算，重复触发只处理仍缺失向量的行；单行生成失败 SHALL 跳过并记录，不阻塞其余行；全部行完成后任务结束，可再次触发续漏。

#### Scenario: 触发回填补齐死向量行

- **WHEN** 库中存在 active aux 标签 "高市早苗"（embedding/merge_embedding 为 NULL），用户调用回填端点
- **THEN** 系统异步为该行重建 embedding（由 "高市早苗: <description>" 生成）与 merge_embedding（由 "高市早苗" 生成），完成后该行两向量均非 NULL

#### Scenario: 已有向量的行不受回填影响

- **WHEN** 回填运行时某 active aux 标签两向量均非 NULL
- **THEN** 该行的向量与别名、描述等其余列保持原值，不重算

#### Scenario: 单行生成失败不阻塞批处理

- **WHEN** 回填过程中某行的 embedding 调用失败（如 LLM 网络错误）
- **THEN** 系统跳过该行并记录失败，继续处理其余行；任务结束后再次触发时仅重试仍缺向量的行

#### Scenario: 无死向量时回填为空操作

- **WHEN** 库中不存在 active 且向量缺失的 aux 标签，用户调用回填端点
- **THEN** 任务立即完成（零行处理），不产生任何写入或 LLM 调用
