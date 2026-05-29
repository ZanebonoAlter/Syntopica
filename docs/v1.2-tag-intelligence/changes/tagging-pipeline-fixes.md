# Tagging Pipeline 修复计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复打标签流程中发现的 7 个潜在问题，提升标签匹配准确性和系统健壮性

**Architecture:** 问题按严重度排列，每个 Task 独立可验证。已完成的修复：(1) 去掉 resolveCandidate 的重复匹配逻辑，(2) 添加 LLMExplicitNone 机制防止 event_fallback 绕过 LLM 判断，(3) 添加 0.85 相似度阈值检查。

**Tech Stack:** Go, GORM, PostgreSQL + pgvector, LLM (via airouter)

---

## 已完成

- [x] **问题 1（严重）**：resolveCandidate 和 findOrCreateTag 双重匹配 → 已去掉 resolveCandidate 的匹配逻辑
- [x] **问题 4（中等）**：LLM 判断后 event_fallback 绕过 → 已添加 LLMExplicitNone + 0.85 阈值检查

## Task 2: non-event 类别 similarity >= 0.78 直接复用，跳过 LLM

**问题**: `extractor_enhanced.go` 的 `resolveCandidate` 已简化，此问题在 `TagArticle` 主路径不再存在。但 `TagSummary` 路径中 `findOrCreateTag` → `ExtractAbstractTag` 对 non-event 类别仍走 LLM 判断，这是正确行为。

**结论**: 问题 2 随问题 1 的修复已自然解决（`resolveCandidate` 不再做匹配决策）。

**Status:** ✅ 已解决

---

## Task 3: TagSummary 路径 articleID=0，co-tag 扩展失效

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go:100-128`

**问题**: `TagSummary` 调用 `findOrCreateTag(ctx, tag, source, articleContext, 0)` 时 articleID=0，导致 `ExpandEventCandidatesByArticleCoTags(ctx, 0, 0, existingIDs)` 无法通过 article 反查关联标签。

**Step 1: 确认问题**

在 `TagSummary` 中，summary 可以关联到 article。实际模型没有 `summary.ArticleID` 字段，关联来源是 `summary.Articles` 中保存的文章 ID JSON。

**Step 2: 从 summary 反查关联 article**

```go
// 在 TagSummary 的 for 循环前，从 summary.Articles 解析首个有效 article ID
articleID := primaryArticleIDForSummary(summary)
```

**Step 3: 将 articleID 传入 findOrCreateTag**

```go
dbTag, err := findOrCreateTag(context.Background(), tag, source, articleContext, articleID)
```

**Step 4: 验证**

```bash
cd backend-go && go build ./internal/domain/topicextraction/...
```

**Step 5: Commit**

---

## Task 4: cleanupOrphanedTags 可能误删有摘要关联的标签

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/article_tagger.go:369-392`

**问题**: `cleanupOrphanedTags` 只检查 `article_topic_tags`，不检查 `ai_summary_topics`。如果标签只有摘要关联，`RetagArticle` 后会被误删。

**Step 1: 修改 orphan 检测 SQL**

```go
func cleanupOrphanedTags(tagIDs []uint) {
    if len(tagIDs) == 0 {
        return
    }

    var orphanIDs []uint
    database.DB.Model(&models.TopicTag{}).
        Where("id IN ?", tagIDs).
        Where("id NOT IN (SELECT topic_tag_id FROM article_topic_tags)").
        Where("id NOT IN (SELECT topic_tag_id FROM ai_summary_topics)").  // 新增
        Pluck("id", &orphanIDs)

    // ... 其余不变
}
```

**Step 2: 验证**

```bash
cd backend-go && go build ./internal/domain/topicextraction/...
```

**Step 3: Commit**

---

## Task 5: TagMatch alias 查询加载全量标签（性能优化）

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go:225-238`

**问题**: alias 匹配加载该 category 的所有活跃标签到内存。可以用 SQL 替代。

**Step 1: 用 SQL 替代内存匹配**

```go
// 替换 lines 225-238
if aliases != "" {
    var aliasMatch models.TopicTag
    aliasSQL := `category = ? AND aliases IS NOT NULL AND aliases != '' AND ? = ANY(
        SELECT jsonb_array_elements_text(aliases::jsonb)
    )`
    if err := database.DB.Scopes(activeTagFilter).Where(aliasSQL, category, label).First(&aliasMatch).Error; err == nil {
        logging.Infof("TagMatch: label=%q category=%s result=exact reason=alias existingID=%d existingLabel=%q",
            label, category, aliasMatch.ID, aliasMatch.Label)
        return &TagMatchResult{
            MatchType:   "exact",
            ExistingTag: &aliasMatch,
            Similarity:  1.0,
        }, nil
    }
}
```

**Step 2: 验证**

```bash
cd backend-go && go build ./internal/domain/topicanalysis/...
```

**Step 3: Commit**

---

## Task 6: embedding 类型不一致（person/keyword 用 identity，abstract 用 semantic）

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go:247-250`

**问题**: `TagMatch` 对 person/keyword 用 identity embedding，但 `FindSimilarAbstractTags` 硬编码用 semantic。当抽象标签是 person/keyword 类别时，可能匹配不到。

**Step 1: 统一使用 semantic embedding**

```go
// 替换 lines 247-250
embType := EmbeddingTypeSemantic
```

**理由**: semantic embedding 包含 description 信息，匹配质量更高。identity embedding 只用于标签刚创建、还没有 description 的场景。`TagMatch` 应该始终用最好的 embedding 做匹配。

**Step 2: 验证**

```bash
cd backend-go && go build ./internal/domain/topicanalysis/...
```

**Step 3: Commit**

---

## Task 7: event_fallback 在 LLM error 时仍触发

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go:224-227`

**问题**: 当 `judgmentErr != nil` 时 `result == nil`，event_fallback 条件 `(result == nil || !result.LLMExplicitNone)` 成立，会静默复用 top candidate。

**Step 1: LLM error 时也跳过 fallback**

```go
// 替换 line 227 的条件
if category == "event" && len(candidates) > 0 && candidates[0].Tag != nil && result != nil && !result.LLMExplicitNone {
```

**说明**: 去掉 `result == nil` 分支。LLM error 时 `result == nil`，不再触发 fallback。

**Step 2: 添加 LLM error 日志**

```go
if judgmentErr != nil {
    logging.Warnf("findOrCreateTag: label=%q category=%s LLM judgment failed, skipping event_fallback: %v", tag.Label, category, judgmentErr)
}
```

**Step 3: 验证**

```bash
cd backend-go && go build ./internal/domain/topicextraction/...
```

**Step 4: Commit**

---

## Task 8: extractor_enhanced.go 的 slugify 和 topictypes.Slugify 正则不同

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/extractor_enhanced.go:509-521`

**问题**: `slugifyWithPunc` 用 `[a-z0-9\u4e00-\u9fff\-]`，`topictypes.Slugify` 用 `[^\p{L}\p{N}\s]+`。对典型中文标签效果一致，但对特殊字符（如日文、韩文）行为不同。

**Step 1: 确认 slugifyWithPunc 已删除，统一使用 topictypes.Slugify**

`slugifyWithPunc` 当前已经不存在（`resolveCandidate` 已改用 `topictypes.Slugify`）。无需代码修改，只需验证构建。

**Step 2: 验证**

```bash
cd backend-go && go build ./internal/domain/topicextraction/...
```

**Step 3: Commit**

---

## 验证顺序

```bash
cd backend-go && go test ./internal/domain/topicanalysis/... -count=1 -timeout 30s
cd backend-go && go test ./internal/domain/topicextraction/... -count=1 -timeout 30s
cd backend-go && go build ./...
```
