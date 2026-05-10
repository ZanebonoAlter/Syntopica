# Dual Tag Embeddings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将标签 embedding 拆分为 identity 与 semantic 两类，保证普通标签自动匹配稳定，同时保留 description 对抽象标签合并与 LLM 判断的语义支持。

**Architecture:** 在 `topic_tag_embeddings` 中新增 `embedding_type`，同一标签允许保存两种向量。普通 `TagMatch` 只查询 `identity` 向量（`label + aliases + category`），抽象/整理/语义召回查询 `semantic` 向量（`label + description + aliases + category`）。description 继续生成并保存，但不再污染普通标签身份匹配。

**Tech Stack:** Go, GORM, PostgreSQL + pgvector, Gin, LLM (airouter)

---

## 背景

当前 `buildTagEmbeddingText` 将 `label + description + aliases + category` 混合为单一 embedding 文本，导致两个问题：

- 普通标签自动匹配时，description 会把“首篇文章上下文”带入身份向量，影响 `TagMatch` 的 exact/candidates/no_match 分流。
- 抽象标签整理和 LLM 判断又确实需要 description 来增强语义召回，不能简单全局删除。

解决方案不是在单一向量里继续调权，而是显式分离职责：

- `identity`: 服务普通标签身份匹配与去重
- `semantic`: 服务抽象标签、手动整理、语义候选召回

---

## Task 1: 为 embedding 增加类型字段与唯一约束

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go`
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Test: `backend-go/internal/platform/database/datamigrate/verify_test.go`

**Step 1: 给 `TopicTagEmbedding` 增加 `EmbeddingType` 字段**

在 `TopicTagEmbedding` struct 中新增字段，默认值为 `identity`，并调整唯一索引语义为 `(topic_tag_id, embedding_type)`：

```go
EmbeddingType string `gorm:"size:20;not null;default:identity;uniqueIndex:idx_topic_tag_embeddings_tag_type" json:"embedding_type"`
```

同时把原有只基于 `TopicTagID` 的唯一约束改成联合唯一索引，避免一个 tag 只能有一条 embedding。

**Step 2: 追加 PostgreSQL 迁移**

在 `postgres_migrations.go` 末尾追加迁移，内容包括：

```go
{
    Version:     "20260418_0001",
    Description: "Add embedding_type to topic_tag_embeddings and allow dual embeddings per tag.",
    Up: func(db *gorm.DB) error {
        if err := db.Exec(`ALTER TABLE topic_tag_embeddings ADD COLUMN IF NOT EXISTS embedding_type VARCHAR(20) NOT NULL DEFAULT 'identity'`).Error; err != nil {
            return fmt.Errorf("add embedding_type to topic_tag_embeddings: %w", err)
        }
        if err := db.Exec(`UPDATE topic_tag_embeddings SET embedding_type = 'identity' WHERE embedding_type IS NULL OR embedding_type = ''`).Error; err != nil {
            return fmt.Errorf("backfill embedding_type: %w", err)
        }
        if err := db.Exec(`DROP INDEX IF EXISTS idx_topic_tag_embeddings_topic_tag_id`).Error; err != nil {
            return fmt.Errorf("drop old unique index: %w", err)
        }
        if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_tag_embeddings_tag_type ON topic_tag_embeddings(topic_tag_id, embedding_type)`).Error; err != nil {
            return fmt.Errorf("create topic_tag_embeddings(tag_id, type) unique index: %w", err)
        }
        return nil
    },
},
```

如果当前索引名与实际不一致，执行前先在代码里查模型声明，确保 `DROP INDEX` 名称与现网一致。

**Step 3: 写迁移验证测试或补充断言**

在现有数据库校验测试里补充断言：

- `topic_tag_embeddings.embedding_type` 存在
- 联合唯一索引存在

**Step 4: 运行验证**

Run: `rtk go test ./internal/platform/database/...`

Expected: PASS，迁移相关测试通过。

**Step 5: Commit**

```bash
rtk git add backend-go/internal/domain/models/topic_graph.go backend-go/internal/platform/database/postgres_migrations.go backend-go/internal/platform/database/datamigrate/verify_test.go
rtk git commit -m "feat: support dual embedding types for topic tags"
```

---

## Task 2: 拆分 identity / semantic 文本构造与存取模型

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Test: `backend-go/internal/domain/topicanalysis/embedding_test.go`

**Step 1: 定义 embedding 类型常量**

在 `embedding.go` 中新增常量：

```go
const (
    EmbeddingTypeIdentity = "identity"
    EmbeddingTypeSemantic = "semantic"
)
```

**Step 2: 将单一 `buildTagEmbeddingText` 拆成按类型构造**

把：

```go
func buildTagEmbeddingText(tag *models.TopicTag) string
```

改成：

```go
func buildTagEmbeddingText(tag *models.TopicTag, embeddingType string) string
```

规则：

- `identity`: `label + aliases + category`
- `semantic`: `label + description + aliases + category`

保持 alias 解析逻辑不变，description 只在 `semantic` 下拼接。

**Step 3: 让 `GenerateEmbedding` 接受 embedding 类型**

把：

```go
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, tag *models.TopicTag) (*models.TopicTagEmbedding, error)
```

改成：

```go
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, tag *models.TopicTag, embeddingType string) (*models.TopicTagEmbedding, error)
```

生成结果时写入：

```go
EmbeddingType: embeddingType,
```

`textHash` 必须包含 `embeddingType`，避免 identity 与 semantic 文本一样时被误认为同一版。例如：

```go
textHash := hashText(embeddingType + "\n" + text)
```

**Step 4: 让 `SaveEmbedding` / `GetEmbedding` 按类型读写**

把只按 `topic_tag_id` 查询的逻辑改成按 `topic_tag_id + embedding_type` 查询。

新增接口：

```go
func (s *EmbeddingService) GetEmbedding(tagID uint, embeddingType string) (*models.TopicTagEmbedding, error)
```

保存逻辑用 upsert 或 GORM `OnConflict` 按 `(topic_tag_id, embedding_type)` 更新，避免重复插入。

**Step 5: 写测试**

在 `embedding_test.go` 新增测试：

- `identity` 文本不包含 description
- `semantic` 文本包含 description
- 同一 tag 生成两种 embedding 时 `EmbeddingType` 与 `TextHash` 不同

**Step 6: 运行测试**

Run: `rtk go test ./internal/domain/topicanalysis -run "TestBuildTagEmbeddingText|TestGenerateEmbedding" -v`

Expected: PASS

**Step 7: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/embedding.go backend-go/internal/domain/topicanalysis/embedding_test.go
rtk git commit -m "feat: split tag embeddings into identity and semantic types"
```

---

## Task 3: 让普通标签匹配只使用 identity embedding

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`
- Modify: `backend-go/internal/domain/topicextraction/extractor_enhanced.go`
- Test: `backend-go/internal/domain/topicanalysis/embedding_test.go`

**Step 1: 让 `FindSimilarTags` 支持指定 embedding 类型**

把：

```go
func (s *EmbeddingService) FindSimilarTags(ctx context.Context, tag *models.TopicTag, category string, limit int) ([]TagCandidate, error)
```

改成：

```go
func (s *EmbeddingService) FindSimilarTags(ctx context.Context, tag *models.TopicTag, category string, limit int, embeddingType string) ([]TagCandidate, error)
```

SQL 增加过滤条件：

```sql
AND e.embedding_type = ?
```

查询向量也用相同 `embeddingType` 的文本构造。

**Step 2: `TagMatch` 固定用 `identity`**

`TagMatch` 内部调用 `FindSimilarTags(..., EmbeddingTypeIdentity)`，保证普通标签自动匹配不再读取 semantic 向量。

**Step 3: 检查并修正直接调用 `TagMatch` / `FindSimilarTags` 的代码**

重点检查：

- `backend-go/internal/domain/topicextraction/tagger.go`
- `backend-go/internal/domain/topicextraction/extractor_enhanced.go`

确保普通标签自动打标签、增强提取流程都仍然只走 identity。

**Step 4: 补日志字段**

在已有诊断日志里增加 `embeddingType=identity`，方便线上确认读取路径已切换。

**Step 5: 写测试**

新增或修改测试，验证：

- `TagMatch` 只命中 identity embedding
- semantic embedding 的 description 差异不会影响普通标签匹配结果

**Step 6: 运行测试**

Run: `rtk go test ./internal/domain/topicanalysis ./internal/domain/topicextraction`

Expected: PASS

**Step 7: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/embedding.go backend-go/internal/domain/topicextraction/tagger.go backend-go/internal/domain/topicextraction/extractor_enhanced.go backend-go/internal/domain/topicanalysis/embedding_test.go
rtk git commit -m "feat: use identity embeddings for automatic tag matching"
```

---

## Task 4: 让抽象标签与手动整理使用 semantic embedding

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go`
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go`
- Test: `backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go`

**Step 1: 新增 semantic 查询入口**

可以二选一，优先选更小改动方案：

- 方案 A：新增 `FindSemanticSimilarTags(...)`
- 方案 B：复用 `FindSimilarTags(..., embeddingType)`

推荐 B，避免新 helper 过多。

**Step 2: 手动整理 `OrganizeUnclassifiedTags` 改查 semantic**

把：

```go
es.FindSimilarTags(ctx, currentTag, category, 5)
```

改成：

```go
es.FindSimilarTags(ctx, currentTag, category, 5, EmbeddingTypeSemantic)
```

这样 description 继续为整理标签服务。

**Step 3: 检查抽象层级相关召回**

检查以下函数是否应继续使用 semantic：

- `MatchAbstractTagHierarchy`
- 任何依赖 abstract tag description 的相似检索函数

如果 `FindSimilarAbstractTags` 当前默认读取单一 embedding，也要改为只读 `semantic`。

**Step 4: 写测试**

在 `abstract_tag_service_test.go` 增加测试：

- semantic embedding 存在 description 时，整理流程仍能召回候选
- identity/semantic 共存时，整理流程不会误读 identity

**Step 5: 运行测试**

Run: `rtk go test ./internal/domain/topicanalysis -run "Test.*Organize|Test.*Abstract" -v`

Expected: PASS

**Step 6: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go backend-go/internal/domain/topicanalysis/embedding.go backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
rtk git commit -m "feat: use semantic embeddings for abstract tag organization"
```

---

## Task 5: 队列与描述回填改为同时维护两种 embedding

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding_queue.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`
- Test: `backend-go/internal/domain/topicanalysis/embedding_queue_test.go` 或现有相关测试文件

**Step 1: 明确队列任务语义**

当前 embedding 队列是“一个 tag 一条任务”。保持任务模型不变，但 worker 执行时要为同一 tag 生成两种 embedding。

**Step 2: 修改 worker 处理逻辑**

在队列 worker 里，加载 tag 后顺序执行：

```go
identityEmb, err := es.GenerateEmbedding(ctx, &tag, EmbeddingTypeIdentity)
semanticEmb, err := es.GenerateEmbedding(ctx, &tag, EmbeddingTypeSemantic)
```

然后分别 `SaveEmbedding`。如果 identity 成功、semantic 失败，任务应整体失败并保留错误，避免只生成一半却被当成完成。

**Step 3: description 更新后的回填保持现状，但语义改变为“双重重建”**

`generateTagDescription()` 和 `generatePersonTagDescription()` 不需要改 enqueue 点；它们仍然 enqueue 同一个 tag，由队列 worker 负责重建两种 embedding。

**Step 4: 补测试**

至少验证：

- 队列 worker 处理一个 tag 后落两条 embedding
- description 更新后重新 enqueue，semantic 更新，identity 也会重建且不报错

**Step 5: 运行测试**

Run: `rtk go test ./internal/domain/topicanalysis -run "Test.*EmbeddingQueue|Test.*Description" -v`

Expected: PASS

**Step 6: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/embedding_queue.go backend-go/internal/domain/topicextraction/tagger.go
rtk git commit -m "feat: rebuild identity and semantic embeddings in queue worker"
```

---

## Task 6: 提供一次性回填命令，避免新旧向量混用

**Files:**
- Create: `backend-go/cmd/rebuild-tag-embeddings/main.go`
- Modify: `docs/operations/development.md`

**Step 1: 创建回填命令**

新建命令，支持：

- 默认扫描 `topic_tags`
- 对 active tag 重新 enqueue 或直接重建两种 embedding
- 支持 `--category` / `--source` / `--limit` 便于分批跑

最小接口示例：

```bash
go run cmd/rebuild-tag-embeddings/main.go --source normal --status active
```

如果项目习惯走 queue，就让命令负责 enqueue；如果更需要可控性，可直接批量调用 service。优先选 queue 版，和线上运行模型一致。

**Step 2: 处理旧数据兼容**

回填前，旧 embedding 全是 `identity`。命令运行后：

- 每个 tag 至少有一条 `identity`
- 需要语义检索的 tag 会补出 `semantic`

可选：为 abstract tag 与 active 非 abstract tag 全部补 semantic，避免整理功能读不到旧标签的 semantic。

**Step 3: 文档化运行方式**

在 `docs/operations/development.md` 增加：

- 为什么要回填
- 建议顺序：迁移 -> 启动 worker -> 运行回填命令 -> 观察队列状态

**Step 4: 验证**

Run: `rtk go run cmd/rebuild-tag-embeddings/main.go --limit 10`

Expected: 输出 enqueue 或 rebuild 统计，不报错。

**Step 5: Commit**

```bash
rtk git add backend-go/cmd/rebuild-tag-embeddings/main.go docs/operations/development.md
rtk git commit -m "feat: add tag embedding rebuild command"
```

---

## Task 7: 端到端验证与收尾

**Files:**
- Modify: `docs/architecture/backend-go.md`
- Modify: `docs/README.md` 或相关说明文件（如有必要）

**Step 1: 写一个最小端到端验证清单**

验证场景：

- 普通标签新建时，只看 identity，description 差异不影响复用
- 手动整理标签时，semantic 仍然能根据 description 召回相关候选
- 抽象标签层级匹配仍然正常

**Step 2: 运行后端测试**

Run: `rtk go test ./...`

Expected: PASS

**Step 3: 如有编译入口，运行构建**

Run: `rtk go build ./...`

Expected: PASS

**Step 4: 更新架构文档**

在 `docs/architecture/backend-go.md` 记录：

- `topic_tag_embeddings` 现在有两种 `embedding_type`
- `TagMatch` 走 identity
- `OrganizeUnclassifiedTags` / abstract 流程走 semantic

**Step 5: Commit**

```bash
rtk git add docs/architecture/backend-go.md
rtk git commit -m "docs: document identity and semantic tag embeddings"
```

---

## 执行注意事项

- 不要在第一步就删掉旧 embedding 读取逻辑；先完成 schema + save/get + queue，再切换查询路径。
- migration 与回填之间，线上会出现“只有 identity、没有 semantic”的短暂状态，因此 semantic 查询代码要对缺失结果保持可恢复，不要 panic。
- description 生成逻辑不是这次重构目标，不要顺手改 prompt 或回填时机。
- 如果 `FindSimilarAbstractTags` 直接依赖 embedding 表，也必须同步指定 `embedding_type='semantic'`，否则会混读 identity。

## 验收标准

- 普通标签自动匹配仅依赖 `identity` 向量
- 手动整理/抽象候选召回使用 `semantic` 向量
- description 仍然保存并参与 semantic embedding
- 同一 tag 可以稳定保存两条 embedding
- 回填命令可把存量数据补齐到双 embedding 模型
- `go test ./...` 与 `go build ./...` 均通过

Plan complete and saved to `docs/plans/2026-04-18-dual-tag-embeddings.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
