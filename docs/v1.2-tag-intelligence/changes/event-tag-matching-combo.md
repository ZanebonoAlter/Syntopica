# Event 标签匹配组合方案实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 通过三层防线解决 event 类别标签的去重匹配问题——语义 embedding 增强 + TagMatch 使用 semantic embedding + findOrCreateTag candidates 分支增强。

**Architecture:** 当前 event 标签（如"伊朗维护霍尔木兹权益"和"伊朗袭击霍尔木兹海峡船只"）因 label 文本差异大，纯向量相似度无法匹配。方案分三层：(1) semantic embedding 文本增加关联文章标题作为上下文；(2) TagMatch 对 event 类别使用 semantic embedding 而非 identity；(3) findOrCreateTag 的 candidates 分支对 event 类别做增强判断。

**Tech Stack:** Go, GORM, pgvector, LLM (airouter)

---

## 前置知识

### 关键文件
- `backend-go/internal/domain/topicanalysis/embedding.go` — `buildTagEmbeddingText`, `TagMatch`, `FindSimilarTags`, `GenerateEmbedding`
- `backend-go/internal/domain/topicanalysis/embedding_queue.go` — `Enqueue`, worker `processNext`
- `backend-go/internal/domain/topicextraction/tagger.go` — `findOrCreateTag`（主匹配入口）, `TagSummary`, `TagArticle`
- `backend-go/internal/domain/topicextraction/extractor_enhanced.go` — `resolveCandidate`, `aiJudgment`（备用路径）
- `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` — `ExtractAbstractTag`（findOrCreateTag candidates 分支使用的判断逻辑）
- `backend-go/internal/domain/models/topic_graph.go` — `TopicTag`, `AISummaryTopic`, `ArticleTopicTag`

### 关键数据表
- `topic_tags` — 标签主表（category: event/person/keyword）
- `topic_tag_embeddings` — 向量表（embedding_type: identity/semantic）
- `ai_summary_topics` — 摘要↔标签关联
- `article_topic_tags` — 文章↔标签关联

### 实际调用链（重要！）

主路径：
```
TagSummary/TagArticle
  → ExtractTags (extractor_enhanced.go) — 提取候选标签
    → resolveCandidate — 对每个候选做 slug/alias/embedding 匹配（预处理）
  → findOrCreateTag (tagger.go:144) — 对每个已提取标签做最终去重
    → TagMatch (embedding.go:211) — 向量匹配
      → candidates → ExtractAbstractTag — 抽象/合并判断
```

**关键点：** `findOrCreateTag` 有自己独立的匹配逻辑（slug → TagMatch → candidates → ExtractAbstractTag），`resolveCandidate` 也有（slug → alias → TagMatch → top-1 复用）。两者是串联关系，`findOrCreateTag` 是最终决定。

### embedding 文本构建（`buildTagEmbeddingText`）
- Identity: `Label + Aliases + Category`
- Semantic: `Label + Description + Aliases + Category`

### 所有 GenerateEmbedding 调用点

| 文件 | 行 | 说明 |
|------|------|------|
| `embedding.go:96` | 函数定义 | 签名变更处 |
| `embedding.go:142` | FindSimilarTags 内部调用 | 为查询生成 embedding |
| `embedding_queue.go:251` | worker identity | 不需要上下文 |
| `embedding_queue.go:262` | worker semantic | **需要传上下文** |
| `abstract_tag_service.go:209` | 抽象标签处理 | 不需要上下文 |
| `abstract_tag_service.go:536` | 抽象标签处理 | 不需要上下文 |
| `merge_reembedding_queue.go:234` | merge 后 identity | 不需要上下文 |
| `merge_reembedding_queue.go:245` | merge 后 semantic | **需要传上下文** |
| `abstract_tag_update_queue.go:240` | 更新 identity | 不需要上下文 |
| `abstract_tag_update_queue.go:249` | 更新 semantic | **需要传上下文** |
| `cmd/migrate-tags/main.go:165,176` | 迁移工具 | 可选传上下文 |

---

## Task 1: Semantic Embedding 增加文章上下文

**目标：** event 类别标签的 semantic embedding 文本中拼入关联文章标题，让同源事件的标签向量靠近。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go` — `buildTagEmbeddingText`, `GenerateEmbedding`, 新增 `GetTagContextTitles`
- Modify: `backend-go/internal/domain/topicanalysis/embedding_queue.go` — worker 和 Enqueue 中传上下文
- Modify: `backend-go/internal/domain/topicanalysis/merge_reembedding_queue.go` — semantic 调用处传上下文
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go` — semantic 调用处传上下文
- Test: `backend-go/internal/domain/topicanalysis/embedding_test.go`

### Step 1: 新增 `EmbeddingTextOptions` 类型和修改 `buildTagEmbeddingText` 签名

在 `embedding.go` 中添加类型并修改函数签名：

```go
type EmbeddingTextOptions struct {
    ContextTitles []string
}

func buildTagEmbeddingText(tag *models.TopicTag, embeddingType string, opts ...EmbeddingTextOptions) string {
    text := tag.Label

    if embeddingType == EmbeddingTypeSemantic && tag.Description != "" {
        text += ". " + tag.Description
    }

    if tag.Aliases != "" {
        var aliases []string
        if err := json.Unmarshal([]byte(tag.Aliases), &aliases); err == nil {
            for _, alias := range aliases {
                text += " " + alias
            }
        } else {
            text += " " + tag.Aliases
        }
    }

    text += " " + tag.Category

    if embeddingType == EmbeddingTypeSemantic && tag.Category == "event" {
        for _, o := range opts {
            if len(o.ContextTitles) > 0 {
                text += ". 相关报道: " + strings.Join(o.ContextTitles, "；")
                break
            }
        }
    }

    return text
}
```

### Step 2: 新增 `GetTagContextTitles` 导出函数

在 `embedding.go` 中添加（注意大写导出）：

```go
func GetTagContextTitles(tagID uint, limit int) []string {
    var titles []string
    query := `
        SELECT DISTINCT a.title
        FROM article_topic_tags att
        JOIN articles a ON a.id = att.article_id
        WHERE att.topic_tag_id = ?
        ORDER BY a.created_at DESC
        LIMIT ?
    `
    database.DB.Raw(query, tagID, limit).Scan(&titles)

    if len(titles) >= limit {
        return titles
    }

    remaining := limit - len(titles)
    var summaryTitles []string
    query2 := `
        SELECT DISTINCT s.title
        FROM ai_summary_topics ast
        JOIN ai_summaries s ON s.id = ast.summary_id
        WHERE ast.topic_tag_id = ?
          AND s.title NOT IN (SELECT DISTINCT a.title FROM article_topic_tags att JOIN articles a ON a.id = att.article_id WHERE att.topic_tag_id = ?)
        ORDER BY ast.created_at DESC
        LIMIT ?
    `
    database.DB.Raw(query2, tagID, tagID, remaining).Scan(&summaryTitles)
    titles = append(titles, summaryTitles...)
    return titles
}
```

### Step 3: 修改 `GenerateEmbedding` 签名

```go
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, tag *models.TopicTag, embeddingType string, opts ...EmbeddingTextOptions) (*models.TopicTagEmbedding, error) {
    text := buildTagEmbeddingText(tag, embeddingType, opts...)
    textHash := hashText(embeddingType + "\n" + text)
    // ... 后续不变
}
```

### Step 4: 修改 embedding queue worker (`embedding_queue.go:processNext`)

在 worker 生成 semantic embedding 时查询上下文：

```go
// identity 不需要上下文
identityEmb, err := s.embedding.GenerateEmbedding(ctx, &tag, EmbeddingTypeIdentity)

// semantic 时为 event 类别查询上下文
var semOpts []EmbeddingTextOptions
if tag.Category == "event" {
    titles := GetTagContextTitles(tag.ID, 5)
    if len(titles) > 0 {
        semOpts = append(semOpts, EmbeddingTextOptions{ContextTitles: titles})
    }
}
semanticEmb, semErr := s.embedding.GenerateEmbedding(ctx, &tag, EmbeddingTypeSemantic, semOpts...)
```

### Step 5: 更新 `Enqueue` 中的 hash 去重逻辑 (`embedding_queue.go:42`)

```go
// 原: semHash := hashText(EmbeddingTypeSemantic + "\n" + buildTagEmbeddingText(&tag, EmbeddingTypeSemantic))
// 改为:
var semOpts []EmbeddingTextOptions
if tag.Category == "event" {
    titles := GetTagContextTitles(tag.ID, 5)
    if len(titles) > 0 {
        semOpts = append(semOpts, EmbeddingTextOptions{ContextTitles: titles})
    }
}
semHash := hashText(EmbeddingTypeSemantic + "\n" + buildTagEmbeddingText(&tag, EmbeddingTypeSemantic, semOpts...))
```

### Step 6: 更新其他 semantic embedding 调用点

对以下文件中的 `GenerateEmbedding(..., EmbeddingTypeSemantic)` 调用，添加上下文查询：

- `merge_reembedding_queue.go:245` — merge 后重生成 semantic，如果 target 是 event 类别需要传上下文
- `abstract_tag_update_queue.go:249` — abstract tag 更新后重生成 semantic，如果 tag 是 event 类别需要传上下文

每个调用点都用相同模式：
```go
var semOpts []topicanalysis.EmbeddingTextOptions
if tag.Category == "event" {
    titles := topicanalysis.GetTagContextTitles(tag.ID, 5)
    if len(titles) > 0 {
        semOpts = append(semOpts, topicanalysis.EmbeddingTextOptions{ContextTitles: titles})
    }
}
semanticEmb, semErr := s.embedding.GenerateEmbedding(context.Background(), &tag, topicanalysis.EmbeddingTypeSemantic, semOpts...)
```

注意：`abstract_tag_service.go:209,536` 和 `embedding.go:142`（FindSimilarTags 内部）和所有 identity 调用点不需要修改（`opts ...EmbeddingTextOptions` 是可选参数，不传就是原行为）。

### Step 7: 写测试 + 验证

```go
func TestBuildTagEmbeddingTextWithContextTitles(t *testing.T) {
    tag := &models.TopicTag{
        Label:       "伊朗袭击霍尔木兹海峡船只",
        Category:    "event",
        Description: "指伊朗在霍尔木兹海峡对多艘船只发动的三次袭击事件",
    }

    text := buildTagEmbeddingText(tag, EmbeddingTypeSemantic)
    assert.NotContains(t, text, "相关报道")

    text = buildTagEmbeddingText(tag, EmbeddingTypeSemantic, EmbeddingTextOptions{
        ContextTitles: []string{"伊朗在霍尔木兹海峡的军事行动", "霍尔木兹海峡局势升级"},
    })
    assert.Contains(t, text, "相关报道")
    assert.Contains(t, text, "伊朗在霍尔木兹海峡的军事行动")

    // non-event category should not include context even with opts
    tag.Category = "person"
    text = buildTagEmbeddingText(tag, EmbeddingTypeSemantic, EmbeddingTextOptions{
        ContextTitles: []string{"some title"},
    })
    assert.NotContains(t, text, "相关报道")
}
```

运行：`go test ./internal/domain/topicanalysis/... -v -run TestBuildTagEmbeddingTextWithContextTitles`

### Step 8: Commit

```bash
git add backend-go/internal/domain/topicanalysis/embedding.go backend-go/internal/domain/topicanalysis/embedding_queue.go backend-go/internal/domain/topicanalysis/merge_reembedding_queue.go backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go backend-go/internal/domain/topicanalysis/embedding_test.go
git commit -m "feat: enrich event tag semantic embedding with article context titles"
```

---

## Task 2: TagMatch 对 event 类别使用 Semantic embedding

**目标：** event 类别的标签去重（TagMatch）使用 semantic embedding 而非 identity，因为 semantic 包含了文章上下文信息。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go` — `TagMatch`

### Step 1: 修改 `TagMatch` 中 `FindSimilarTags` 的 embedding type 选择

在 `TagMatch` 函数中（`embedding.go:247`），将硬编码的 `EmbeddingTypeIdentity` 改为根据 category 选择：

```go
// 原: candidates, err := s.FindSimilarTags(ctx, candidate, category, 20, EmbeddingTypeIdentity)
// 改为:
embType := EmbeddingTypeIdentity
if category == "event" {
    embType = EmbeddingTypeSemantic
}
candidates, err := s.FindSimilarTags(ctx, candidate, category, 20, embType)
```

### Step 2: 测试

运行：`go test ./internal/domain/topicanalysis/... -v`

### Step 3: Commit

```bash
git add backend-go/internal/domain/topicanalysis/embedding.go
git commit -m "feat: use semantic embedding for event tag matching in TagMatch"
```

---

## Task 3: 增强 findOrCreateTag 的 candidates 分支（event 类别）

**目标：** 在 `findOrCreateTag` 的 candidates 分支中，对 event 类别增加更宽松的匹配策略。当 `ExtractAbstractTag` 判断 no_action 时，如果 top 候选相似度在合理范围内，允许复用而非创建新标签。

**背景：** `findOrCreateTag`（`tagger.go:189`）的 candidates 分支目前调用 `ExtractAbstractTag` 做 merge/abstract 判断。如果 `ExtractAbstractTag` 返回 no_action，当前代码会 break 到创建新标签路径。对 event 类别，这意味着即使有合理候选也被跳过。

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — `findOrCreateTag` candidates 分支

### Step 1: 在 candidates 分支中增加 event fallback

在 `findOrCreateTag` 的 `case "candidates":` 中，当 `ExtractAbstractTag` 返回 no_action 后，增加 event 类别的 fallback 逻辑：

```go
case "candidates":
    candidates := matchResult.Candidates
    logging.Infof("findOrCreateTag: label=%q category=%s matchType=candidates candidateCount=%d topSimilarity=%.4f", tag.Label, category, len(candidates), matchResult.Similarity)
    result, judgmentErr := topicanalysis.ExtractAbstractTag(ctx, candidates, tag.Label, category, topicanalysis.WithCaller("findOrCreateTag"))
    if judgmentErr != nil || result == nil || !result.HasAction() {
        logging.Infof("findOrCreateTag: label=%q category=%s judgment=no_action err=%v", tag.Label, category, judgmentErr)

        // [NEW] event 类别的 fallback：如果 ExtractAbstractTag 返回 no_action
        // 但 top 候选相似度足够高，直接复用 top 候选
        if category == "event" && len(candidates) > 0 && candidates[0].Tag != nil {
            topSim := candidates[0].Similarity
            thresholds := es.GetThresholds()
            if topSim >= thresholds.LowSimilarity {
                logging.Infof("findOrCreateTag: label=%q category=%s event_fallback: reusing top candidate (sim=%.4f)", tag.Label, category, topSim)
                existing := candidates[0].Tag
                existing.Label = tag.Label
                existing.Category = category
                existing.Source = source
                if len(tag.Aliases) > 0 {
                    aJSON, _ := json.Marshal(tag.Aliases)
                    existing.Aliases = string(aJSON)
                }
                if tag.Icon != "" {
                    existing.Icon = tag.Icon
                }
                existing.Kind = kind
                if err := database.DB.Save(existing).Error; err != nil {
                    logging.Warnf("Failed to save event fallback tag %d: %v", existing.ID, err)
                } else {
                    go ensureTagEmbedding(es, existing.ID)
                    go backfillTagDescription(existing.ID, existing.Label, existing.Category, existing.Description, articleContext)
                    return existing, nil
                }
            }
        }

        break
    }
    // ... 后续 merge/abstract 逻辑不变
```

**设计说明：** 这里不引入 `aiJudgment`（那是 `resolveCandidate` 的逻辑），而是利用已有的 `ExtractAbstractTag` + 相似度阈值做简单 fallback。`aiJudgment` 需要更多 LLM 调用开销，且 `ExtractAbstractTag` 本身已经做了 AI 判断。对 event 类别，我们放宽的是"AI 判断 no_action 后仍允许复用"的策略，而非增加额外的 LLM 调用。

### Step 2: 测试

运行：`go test ./internal/domain/topicextraction/... -v`

### Step 3: Commit

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "feat: add event tag fallback reuse in findOrCreateTag candidates branch"
```

---

## Task 4: 启用 AI Judgment for Event Tags in resolveCandidate

**目标：** 在 `resolveCandidate` 中，当 event 类别的 `TagMatch` 返回 candidates 且 top similarity 不够高时，调用 `aiJudgment` 让 LLM 判断。这是 `findOrCreateTag` 之前的预处理步骤，可以提前决定标签复用。

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/extractor_enhanced.go` — `resolveCandidate`

### Step 1: 修改 `resolveCandidate` 的 candidates 分支

当前 `resolveCandidate` 在 candidates 分支（`extractor_enhanced.go:180`）直接取 top-1 复用。改为对 event 类别使用 AI Judgment：

```go
case "candidates":
    if len(matchResult.Candidates) > 0 && matchResult.Candidates[0].Tag != nil {
        best := matchResult.Candidates[0]

        if category == "event" && best.Similarity < te.embeddingService.GetThresholds().HighSimilarity {
            judgment, jErr := te.aiJudgment(ctx, candidate, matchResult.Candidates, input)
            if jErr == nil && judgment != nil {
                if judgment.Decision == "reuse" && judgment.ReuseTagID > 0 {
                    var reuseTag models.TopicTag
                    if err := database.DB.First(&reuseTag, judgment.ReuseTagID).Error; err == nil {
                        return &topictypes.TopicTag{
                            Label:     reuseTag.Label,
                            Slug:      reuseTag.Slug,
                            Category:  reuseTag.Category,
                            Icon:      reuseTag.Icon,
                            Aliases:   parseAliases(reuseTag.Aliases),
                            Score:     candidate.Confidence,
                            IsNew:     false,
                            MatchedTo: reuseTag.ID,
                        }, false, nil
                    }
                }
                // AI 判断创建新标签 → 走创建新标签逻辑
            }
        } else {
            return &topictypes.TopicTag{
                Label:     best.Tag.Label,
                Slug:      best.Tag.Slug,
                Category:  best.Tag.Category,
                Icon:      best.Tag.Icon,
                Aliases:   parseAliases(best.Tag.Aliases),
                Score:     candidate.Confidence * best.Similarity,
                IsNew:     false,
                MatchedTo: best.Tag.ID,
            }, false, nil
        }
    }
```

### Step 2: 增强 `aiJudgment` 的上下文

当前 `buildResolutionUserPrompt` 只传 `SummaryContext: fmt.Sprintf("标题: %s\n来源: %s", input.Title, input.FeedName)`。对 event 类别，增加摘要内容：

```go
if input.Summary != "" {
    req.SummaryContext += fmt.Sprintf("\n摘要: %s", truncateString(input.Summary, 500))
}
```

### Step 3: 增强 AI Judgment prompt

在 `buildResolutionSystemPrompt` 中增加对 event 类别的特别指导：

```
对于 event（事件）类别的标签，请特别注意：
- 同一事件可能有完全不同的表述方式，例如"伊朗维护霍尔木兹权益"和"伊朗袭击霍尔木兹海峡船只"可能是同一事件
- 重点比较事件的核心主体（谁）和核心行为（做了什么），而非字面文本相似度
- 如果两个标签指向同一核心事件，即使表述差异很大，也应复用
```

### Step 4: 测试

运行：`go test ./internal/domain/topicextraction/... -v`

### Step 5: Commit

```bash
git add backend-go/internal/domain/topicextraction/extractor_enhanced.go
git commit -m "feat: enable AI judgment for event tag resolution in resolveCandidate"
```

---

## Task 5: 标签关联文章变化时触发 semantic embedding 重生成

**目标：** 当文章与 event 标签建立新关联时，触发该标签的 semantic embedding 重生成（因为上下文标题变了）。

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — 在标签关联到文章/摘要后，如果是 event 类别，入队重生成

### Step 1: 在标签关联后检查是否需要重生成

找到标签与文章/摘要关联的代码（`tagger.go` 中 `TagSummary` 和 `TagArticle` 的关联逻辑），在关联建立后：

```go
if dbTag.Category == "event" {
    qs := getEmbeddingQueueService()
    if err := qs.Enqueue(dbTag.ID); err != nil {
        logging.Warnf("Failed to enqueue re-embedding for event tag %d: %v", dbTag.ID, err)
    }
}
```

**注意：** `Enqueue` 已有去重（检查 pending task + hash 检查），不会重复创建队列任务。

### Step 2: 测试

运行：`go test ./internal/domain/topicextraction/... -v`

### Step 3: Commit

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "feat: trigger semantic re-embedding when event tag gets new article associations"
```

**注意：** 此 Task 需要和 Task 3 合并提交或协调，因为都改了 `tagger.go`。可以选择在 Task 3 中一并完成 Task 5 的修改，然后一起提交。

---

## Task 6: 集成验证

**目标：** 端到端验证三层防线协同工作。

### Step 1: 编译

```bash
cd backend-go && go build ./...
```

### Step 2: 单元测试

```bash
cd backend-go && go test ./internal/domain/topicanalysis/... ./internal/domain/topicextraction/... -v
```

### Step 3: 手动验证场景

1. 模拟两篇不同标题但同一事件的新闻摘要
2. 第一篇提取"伊朗维护霍尔木兹权益"，生成 semantic embedding（含文章标题上下文）
3. 第二篇提取"伊朗袭击霍尔木兹海峡船只"时：
   - TagMatch 用 semantic 搜索 → 找到第一篇的标签作为 candidate
   - findOrCreateTag candidates 分支 → ExtractAbstractTag 判断，如果 no_action → event fallback 复用
   - resolveCandidate candidates 分支 → AI Judgment（如果相似度不够高）

---

## 执行注意事项

1. **`GenerateEmbedding` 签名变更** 使用 `opts ...EmbeddingTextOptions` 可选参数，向后兼容。
2. **文章标题查询** 使用 `article_topic_tags` + `ai_summary_topics` 双表查询，取最多 5 条，限制为 event 类别。
3. **AI Judgment 只在 resolveCandidate 中对 event 启用**，findOrCreateTag 用 ExtractAbstractTag + event fallback。
4. **重生成频率** 由 `Enqueue` 的 hash 去重 + pending task 去重控制。
5. **两层候选判断互补：** resolveCandidate（预处理层）用 AI Judgment 精细判断，findOrCreateTag（最终层）用 ExtractAbstractTag + event fallback 宽松兜底。
6. **所有 semantic embedding 调用点**（worker、merge queue、update queue）都需要为 event 类别传上下文，确保 hash 一致。
7. **`FindSimilarTags` 内部调用 `GenerateEmbedding`** 时不需要传上下文（它只是为查询向量生成临时 embedding，不存入数据库）。
