# 实体主体身份层（entity-subjects）

<!--
doc-impact-applies: backend-go/internal/tagmanagement/models/entity_subject.go, backend-go/internal/tagmanagement/repository/entity_repository.go, backend-go/internal/tagmanagement/service/entity/, backend-go/internal/platform/database/entity_subject_migration.go | section=全部
-->

> **真相源 = 代码**：`backend-go/internal/tagmanagement/models/entity_subject.go`（GORM 模型）+ `backend-go/internal/platform/database/entity_subject_migration.go`（迁移 `20261112_0001`）。本文是投影。
> 落地 change：`openspec/changes/normalize-event-subject-entities`（全局稳定实体 ID 层 + 影子模式 + 有界评测入口）。业务语义与流程红线见 `flow/topic-graph.md` 红线 15、`flow/semantic-board.md` 红线 31、`flow/ai-summary.md` 红线 19、`flow/daily-report.md` 红线 48。

## 设计要点（为什么长这样）

- **实体 ID 全局稳定、不复用**：`entity_registry.id` 主键 + 短不透明 `entity_key`（供二轮模型复述，如 `E7F3K9Q2`）。实体不随辅助标签 GC 消失。
- **规范名无唯一约束**：不同实体允许同名并存（两个「苹果」各持一个 ID），不以名称唯一约束或相似度自动合并。
- **无实体类型列**：唯一现成类型来源是模型现编的 `subject_key` 前缀（实测分型漂移：`company:deepseek` vs `product:deepseek`），让模型猜类型会把漂移固化进身份。类型若未来保留只作第一轮可选输出，绝不参与判定。
- **向量列无维度限定**：`vector` 裸类型（无 typmod），model/版本/维度写在行上（两套检索表示各自同 (model, version, dim) 空间内比较，跨空间不混用不 fallback；名路径始终存在，短说明为空只降级为仅名）。
- **aux 溯源是映射不是同体证据**：唯一写入方 = 判据「本次标签的辅助标签列表中规范化文本与本次提及文本规范化相等的那一行」；唯一消费方 = 评测报告「材料溯源」列与运维排查。相似度自动 alias 不得入内。

## 表清单（3 张）

| 表名 | 说明 | 对应模型 |
| ---- | ---- | ---- |
| `entity_registry` | 实体身份行（稳定 ID / entity_key / 规范名 / 稳定短身份说明 / 两套检索向量及空间三元组） | `tagmanagement/models.EntityRegistry` |
| `entity_mentions` | 名称与证据行（提及原文 + 规范化提及键 + 来源标签/文章；既是证据也是叫法登记） | `tagmanagement/models.EntityMention` |
| `entity_aux_links` | 辅助标签溯源行（entity_id + mention_id + aux_label_id） | `tagmanagement/models.EntityAuxLink` |

## entity_registry

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | bigserial | PK | 全局稳定实体 ID，不复用 |
| entity_key | varchar(40) | **UNIQUE**（`uq_entity_registry_key`） | 短不透明句柄，二轮模型复述用 |
| canonical_name | varchar(200) | 普通索引（`idx_entity_registry_name`），**无唯一约束** | 规范名，允许同名多实体 |
| stable_desc | text | — | 稳定短身份说明（第一轮同次真实产出；可空；不从 subject_key 回填不编造） |
| name_embedding | vector（无维度限定） | — | 名向量 |
| name_embedding_model / _version | varchar(120)/varchar(60) | — | 名向量空间三元组（1/2） |
| name_embedding_dim | integer | — | 名向量空间三元组（3/3） |
| name_desc_embedding | vector（无维度限定） | — | 名+短说明向量（可空：短说明为空的实体无第二表示） |
| name_desc_embedding_model / _version / _dim | 同上 | — | 第二表示的空间三元组 |
| created_at / updated_at | timestamptz | — | — |

## entity_mentions

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | bigserial | PK | — |
| entity_id | bigint | FK → `entity_registry(id)` **ON DELETE CASCADE**（迁移 `20261112_0001`）+ 索引 | 删实体连带删证据（正向） |
| mention_text | varchar(400) | — | 提及**原文**（证据，不覆盖规范字段） |
| mention_key | varchar(400) | 索引（`idx_entity_mentions_key`） | `textutil.NormalizeEntityText(mention_text)`；叫法命中检索键 + 幂等键一半 |
| source_tag_id | bigint | **部分唯一索引** `uq_entity_mentions_source_key (source_tag_id, mention_key) WHERE source_tag_id IS NOT NULL`（迁移） | 来源 `topic_tags.id`；幂等键 =（来源标签, 提及键）——**不是**提及文本（同文不同文的裸「苹果」可能指向不同实体）。NULL=评测合成来源（不参与唯一） |
| source_article_id | bigint | — | 提及来源文章（可选溯源） |
| created_at / updated_at | timestamptz | — | — |

## entity_aux_links

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| id | bigserial | PK | — |
| entity_id | bigint | FK → `entity_registry(id)` **ON DELETE CASCADE**（迁移）+ 索引 | — |
| mention_id | bigint | FK → `entity_mentions(id)` **ON DELETE CASCADE**（迁移）+ 索引 | 溯源挂在证据行上 |
| aux_label_id | bigint | FK → `semantic_labels(id)` **ON DELETE CASCADE**（迁移）+ 索引 | **级联方向契约**：aux 硬删 → 只掉本溯源行；**永远没有**从 aux 指向 entity_registry 的级联。aux 被 GC 置 disabled → 溯源行**读取时失效**（join `semantic_labels.status='active'` 过滤，行保留） |
| created_at | timestamptz | — | — |

## 迁移与回滚

- AutoMigrate 建三表（`tagmanagement/wire.go` init 注册 `EntitySubjectModels()`）；迁移 `20261112_0001`（`entity_subject_migration.go`）钉死四条 FK 的级联方向与幂等部分唯一索引，幂等（IF NOT EXISTS / DO $$ 存在性检查），无历史回填。
- 回滚 = 配置改回 `off`（停止一切归一写入）+ 可整层删除三表（不被任何既有表引用），存量数据全程零改动。
