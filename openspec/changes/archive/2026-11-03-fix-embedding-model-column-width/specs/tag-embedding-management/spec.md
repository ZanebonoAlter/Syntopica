## ADDED Requirements

### Requirement: embedding 模型名列宽不窄于模型注册表

存 embedding 模型名的列 SHALL NOT 窄于模型注册表 `ai_models.name` 的列宽（当前两者均为 100 字符）。`topic_tag_embeddings.model` SHALL 接受注册表里能存下的**任意**模型名——包括本地 llama.cpp / GGUF 模型以文件绝对路径注册的名称——SHALL NOT 因超限导致向量保存失败。

模型定义（gorm tag）与 DDL SHALL 保持一致：模型定义中的 `model` 字段长度 SHALL 等于该列实际列宽，SHALL NOT 出现「DDL 已加宽但 tag 仍是旧窄值」的漂移（否则下次 AutoMigrate 会把列改回窄列）。

#### Scenario: 长模型名写入成功

- **WHEN** 一条 `topic_tag_embeddings` 记录的 `model` 为 61 字符的本地模型路径（`/home/zanebono/.syntopica/models/Qwen3-Embedding-4B-Q6_K.gguf`）
- **THEN** 写入成功，`model` 列原样保存该字符串，不抛 `SQLSTATE 22001`（value too long for type character varying）

#### Scenario: 列宽不窄于模型注册表

- **WHEN** 比对 `topic_tag_embeddings.model` 与 `ai_models.name` 的列宽
- **THEN** `model` 列宽 ≥ `name` 列宽（当前均为 100）

#### Scenario: 模型定义与 DDL 一致

- **WHEN** GORM AutoMigrate 依据模型定义同步表结构
- **THEN** `topic_tag_embeddings.model` 列宽保持 100，不被改回 50

#### Scenario: 模型定义变更不得顺带丢掉存量列的 NOT NULL

- **WHEN** 模型定义的 `model` 字段长度变更（`size:50` → `size:100`）触发 GORM AutoMigrate 对**存量列**发 ALTER
- **THEN** `topic_tag_embeddings.model` 的 NOT NULL 约束 SHALL 保持（不因 ALTER 被顺带 DROP）；若已被丢弃，版本化迁移 SHALL 把它恢复（幂等）

#### Scenario: 既有向量不受影响

- **WHEN** 列宽变更在有存量行的表上执行（含既有 `model` 为短名的历史行）
- **THEN** 存量行的 `model` 值与 `embedding` 向量原样保留，无截断、无丢行
