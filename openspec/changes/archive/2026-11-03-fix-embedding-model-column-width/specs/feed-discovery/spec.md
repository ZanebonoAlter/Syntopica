## ADDED Requirements

### Requirement: discovery 向量表模型名列宽不窄于模型注册表

discovery 域存放 embedding 模型名的四张表——`candidate_embeddings`、`preference_vectors`、`route_embeddings`、`discovery_interest_entries`——其 `model` 列宽 SHALL NOT 窄于模型注册表 `ai_models.name` 的列宽（当前两者均为 100 字符），SHALL 接受注册表里能存下的**任意**模型名（含本地 llama.cpp / GGUF 模型以文件绝对路径注册的名称）。

模型定义（gorm tag）与 DDL SHALL 保持一致：模型定义中的 `model` 字段长度 SHALL 等于该列实际列宽，SHALL NOT 出现「DDL 已加宽但 tag 仍是旧窄值」的漂移。

#### Scenario: 长模型名写入候选向量成功

- **WHEN** 一条 `candidate_embeddings` 记录的 `model` 为 61 字符的本地模型路径
- **THEN** 写入成功，不抛 `SQLSTATE 22001`（value too long for type character varying）

#### Scenario: 长模型名写入偏好向量成功

- **WHEN** 一条 `preference_vectors` 记录的 `model` 为 61 字符的本地模型路径
- **THEN** 写入成功，不抛 `SQLSTATE 22001`

#### Scenario: 四张表列宽一致且不窄于模型注册表

- **WHEN** 比对 `candidate_embeddings` / `preference_vectors` / `route_embeddings` / `discovery_interest_entries` 的 `model` 列宽与 `ai_models.name` 列宽
- **THEN** 四张表的 `model` 列宽均 ≥ `name` 列宽（当前均为 100）

#### Scenario: 模型定义与 DDL 一致

- **WHEN** GORM AutoMigrate 依据模型定义同步表结构
- **THEN** 四张表的 `model` 列宽保持 100，不被改回 50
