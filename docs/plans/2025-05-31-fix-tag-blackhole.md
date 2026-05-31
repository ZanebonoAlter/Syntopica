# Fix Tag Blackhole Embedding Match — Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 修复标签黑洞 bug — embedding 高相似度匹配不再自动合并标签，提供手动合并 UI 作为兜底。

**Architecture:** 三部分改动：(1) 后端 `TagMatch` 降级 embedding 高相似度为 candidates + 删除 keyword override + `SaveEmbedding` 清理旧记录；(2) 前端 TagsPage 接入 TagMergePreview；(3) 数据修复脚本。

**Tech Stack:** Go (Gin/GORM), Vue 3 + Nuxt 4, PostgreSQL

---

## Task 1: TagMatch embedding 降级 + 删除 keyword override + 测试

**Files:**
- Modify: `backend-go/internal/domain/tagging/embedding.go:374-382` (TagMatch 高相似度改返回 candidates)
- Modify: `backend-go/internal/domain/tagging/embedding.go:51-56` (删除 CategoryThresholdOverrides keyword 条目)
- Modify: `backend-go/internal/domain/tagging/embedding.go:34` (更新注释，HighSimilarity 不再用于 auto-reuse)
- Modify: `backend-go/internal/domain/tagging/embedding_test.go:353-422` (更新两个现有测试)

**Step 1: 修改 TagMatch — embedding 高相似度降级**

在 `embedding.go` 的 `TagMatch` 方法中，将第 374-382 行：

```go
if validCandidates[0].Similarity >= thresholds.HighSimilarity {
    top := validCandidates[0]
    logging.Infof("TagMatch: label=%q category=%s result=exact reason=high_similarity existingID=%d existingLabel=%q similarity=%.4f", label, category, top.Tag.ID, top.Tag.Label, top.Similarity)
    return &TagMatchResult{
        MatchType:   "exact",
        ExistingTag: top.Tag,
        Similarity:  top.Similarity,
    }, nil
}
```

替换为：

```go
if validCandidates[0].Similarity >= thresholds.HighSimilarity {
    top := validCandidates[0]
    logging.Infof("TagMatch: label=%q category=%s result=candidates reason=high_similarity_downgraded existingID=%d existingLabel=%q similarity=%.4f", label, category, top.Tag.ID, top.Tag.Label, top.Similarity)
    return &TagMatchResult{
        MatchType:  "candidates",
        Similarity: top.Similarity,
        Candidates: validCandidates,
    }, nil
}
```

关键变化：`MatchType` 从 `"exact"` 改为 `"candidates"`，`ExistingTag` 不再设置，匹配结果放入 `Candidates`。这样 `findOrCreateTag` 会 fall through 到创建新 tag。

**Step 2: 删除 CategoryThresholdOverrides keyword 条目**

将 `embedding.go` 第 48-56 行：

```go
// CategoryThresholdOverrides defines per-category threshold adjustments.
// Keys are category names; the corresponding HighSimilarity overrides the default
// when TagMatch processes a tag of that category.
var CategoryThresholdOverrides = map[string]EmbeddingMatchThresholds{
    "keyword": {
        HighSimilarity: 0.90,
        LowSimilarity:  0.78,
    },
}
```

替换为：

```go
// CategoryThresholdOverrides defines per-category threshold adjustments.
// Keys are category names; the corresponding HighSimilarity overrides the default
// when TagMatch processes a tag of that category.
var CategoryThresholdOverrides = map[string]EmbeddingMatchThresholds{}
```

同时更新 `EmbeddingMatchThresholds` 结构体的注释（第 34-35 行），把 `HighSimilarity` 的注释从 "High similarity - auto-reuse existing tag" 改为 "High similarity - include as candidate (no longer auto-reuse)"：

```go
type EmbeddingMatchThresholds struct {
    // High similarity - include as candidate (no longer auto-reuse)
    HighSimilarity float64
    // Low similarity - auto-create new tag
    LowSimilarity float64
    // Middle band - requires AI judgment
    // Tags with similarity between LowSimilarity and HighSimilarity need AI decision
}
```

**Step 3: 更新现有测试**

在 `embedding_test.go` 中：

3a. 更新 `TestThresholdsForCategory`（第 353 行）：将 keyword 测试用例的期望值改为默认阈值：

```go
{
    name:        "keyword uses default (override removed)",
    category:    "keyword",
    wantHighSim: 0.97,
    wantLowSim:  0.78,
},
```

3b. 更新 `TestThresholdsForCategoryOverrideIsolation`（第 399 行）：这个测试验证动态修改 `CategoryThresholdOverrides` 的行为。由于 keyword 条目已删除，改为测试动态添加一个新类别 override：

```go
func TestThresholdsForCategoryOverrideIsolation(t *testing.T) {
    // Clean up any test override after test
    defer func() {
        delete(CategoryThresholdOverrides, "_test_category")
    }()

    CategoryThresholdOverrides["_test_category"] = EmbeddingMatchThresholds{
        HighSimilarity: 0.85,
        LowSimilarity:  0.70,
    }

    got := ThresholdsForCategory("_test_category")
    if got.HighSimilarity != 0.85 {
        t.Errorf("HighSimilarity = %.2f, want 0.85", got.HighSimilarity)
    }
    if got.LowSimilarity != 0.70 {
        t.Errorf("LowSimilarity = %.2f, want 0.70", got.LowSimilarity)
    }

    // Default should be unaffected
    eventGot := ThresholdsForCategory("event")
    if eventGot.HighSimilarity != 0.97 {
        t.Errorf("event HighSimilarity should be unaffected, got %.2f", eventGot.HighSimilarity)
    }
}
```

**Step 4: 运行测试验证**

```bash
cd backend-go && go test ./internal/domain/tagging/ -run "TestThresholdsForCategory" -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/tagging/embedding.go backend-go/internal/domain/tagging/embedding_test.go
git commit -m "fix: downgrade embedding high-similarity match to candidates, remove keyword override

- TagMatch: embedding similarity >= HighSimilarity now returns candidates instead of exact
- Remove keyword category threshold override (0.90 high), use default (0.97)
- Prevents tag blackhole where embedding match overwrites existing tag identity"
```

---

## Task 2: SaveEmbedding 清理旧记录 + 测试

**Files:**
- Modify: `backend-go/internal/domain/tagging/embedding.go:500-524` (SaveEmbedding 方法)
- Modify: `backend-go/internal/domain/tagging/embedding_test.go` (新增测试)

**Step 1: 修改 SaveEmbedding — 添加清理逻辑**

在 `SaveEmbedding` 方法中，在验证 tag 存在之后、查找 existing 之前，添加清理旧记录的逻辑。将第 515-523 行：

```go
var existing models.TopicTagEmbedding
err := database.DB.Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash).First(&existing).Error

if err == nil {
    embedding.ID = existing.ID
    return database.DB.Save(embedding).Error
}

return database.DB.Create(embedding).Error
```

替换为：

```go
// Clean up stale embeddings: same tag + type but different text_hash
database.DB.Where(
    "topic_tag_id = ? AND embedding_type = ? AND text_hash != ?",
    embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash,
).Delete(&models.TopicTagEmbedding{})

var existing models.TopicTagEmbedding
err := database.DB.Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash).First(&existing).Error

if err == nil {
    embedding.ID = existing.ID
    return database.DB.Save(embedding).Error
}

return database.DB.Create(embedding).Error
```

**Step 2: 编写测试**

在 `embedding_test.go` 中新增测试 `TestSaveEmbeddingCleansUpStaleRecords`：

```go
func TestSaveEmbeddingCleansUpStaleRecords(t *testing.T) {
    db := setupTestDB(t)
    svc := NewEmbeddingService()

    // Create a topic tag
    tag := models.TopicTag{Label: "test-tag", Slug: "test-tag", Category: "keyword"}
    require.NoError(t, db.Create(&tag).Error)

    // Create multiple stale embeddings with different text hashes
    for i := 0; i < 3; i++ {
        emb := &models.TopicTagEmbedding{
            TopicTagID:    tag.ID,
            EmbeddingType: "identity",
            TextHash:      fmt.Sprintf("old-hash-%d", i),
            Dimension:     3,
            EmbeddingVec:  "[0.1,0.2,0.3]",
        }
        require.NoError(t, svc.SaveEmbedding(emb))
    }

    // Verify we have 3 stale records
    var countBefore int64
    db.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, "identity").Count(&countBefore)
    assert.Equal(t, int64(3), countBefore)

    // Save a new embedding with a different text hash — should clean up old ones
    newEmb := &models.TopicTagEmbedding{
        TopicTagID:    tag.ID,
        EmbeddingType: "identity",
        TextHash:      "new-hash",
        Dimension:     3,
        EmbeddingVec:  "[0.4,0.5,0.6]",
    }
    require.NoError(t, svc.SaveEmbedding(newEmb))

    // Should have only 1 record now (the new one)
    var countAfter int64
    db.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, "identity").Count(&countAfter)
    assert.Equal(t, int64(1), countAfter)

    // Verify it's the new record
    var saved models.TopicTagEmbedding
    require.NoError(t, db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, "identity").First(&saved).Error)
    assert.Equal(t, "new-hash", saved.TextHash)
}
```

注意：参考同文件中已有的 `TestSaveEmbeddingReturnsTagNotFoundWhenParentDeleted` 测试来确认 `setupTestDB` 和 import 的写法。

**Step 3: 运行测试**

```bash
cd backend-go && go test ./internal/domain/tagging/ -run "TestSaveEmbedding" -v
```

Expected: PASS（包括新增和原有的 TestSaveEmbeddingReturnsTagNotFoundWhenParentDeleted）

**Step 4: Commit**

```bash
git add backend-go/internal/domain/tagging/embedding.go backend-go/internal/domain/tagging/embedding_test.go
git commit -m "fix: SaveEmbedding cleans up stale records with same tag+type but different hash

Prevents embedding count inflation (e.g. tag 94712 with 144 records).
Only the latest embedding per tag+type is kept."
```

---

## Task 3: TagsPage 接入 TagMergePreview

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Context:**
- `TagMergePreview` 组件位于 `front/app/features/topic-graph/components/TagMergePreview.vue`
- 组件 Props: `visible: boolean`, `scopeCategoryId?: string | null`, `scopeFeedId?: string | null`, `standalone?: boolean`（默认 true）
- 组件 Events: `close`, `merged: [summary: MergeSummary]`
- TagsPage 左侧栏操作按钮区已有按钮：添加板块、升级建议、匹配回填、匹配参数、整理叙事
- TagsPage 已有 `loadAuxiliaryLabels` 和 `loadBoards` 方法用于刷新数据
- 需要导入 `TagMergePreview` 组件和 `MergeSummary` 类型

**Step 1: 添加 import 和状态**

在 `<script setup>` 的 import 区域（约第 18 行 `import BoardDailyReportTimeline` 之后）添加：

```ts
import TagMergePreview from '~/features/topic-graph/components/TagMergePreview.vue'
import type { MergeSummary } from '~/types/tagMerge'
```

在状态变量区域（约第 62 行 `showGenerateDialog` 之后）添加：

```ts
const showMergePreview = ref(false)
```

**Step 2: 添加合并完成回调**

在 `handleOpenMatchingConfig` 函数之后添加：

```ts
function handleMergeComplete(summary: MergeSummary) {
  void loadAuxiliaryLabels()
  void loadBoards()
}
```

**Step 3: 在左侧栏操作按钮区添加按钮**

在 template 中找到操作按钮区（约第 648 行 `<button ... @click="handleTriggerBackfill">` 匹配回填按钮之后），添加：

```html
<button type="button" class="sb-action-btn sb-action-btn--ghost" @click="showMergePreview = true">
  <Icon icon="mdi:call-merge" width="14" />
  标签合并
</button>
```

放在"匹配回填"按钮之后、"匹配参数"按钮之前。

**Step 4: 在 template 底部添加 TagMergePreview 组件**

在 `<!-- Narrative Generate Dialog -->` 之后、`<Teleport to="body">` (文章预览) 之前，添加：

```html
<!-- Tag Merge Preview -->
<TagMergePreview
  :visible="showMergePreview"
  @close="showMergePreview = false"
  @merged="handleMergeComplete"
/>
```

**Step 5: lint 检查**

```bash
cd front && pnpm lint
```

Expected: 无错误

**Step 6: typecheck (必须 Windows cmd)**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
```

Expected: 无错误

**Step 7: Commit**

```bash
git add front/app/features/tags/components/TagsPage.vue
git commit -m "feat: add tag merge UI to TagsPage sidebar

Wire up existing TagMergePreview component as a manual fallback
for merging synonym tags (e.g. 'Trump'/'特朗普') now that
embedding no longer auto-merges on high similarity."
```

---

## Task 4: 数据修复 — 清理 tag 94712 + 排查其他污染

**Files:**
- 无代码文件修改，直接执行 SQL

**Context:**
- tag 94712（"共产党员"）是已知的污染 tag
- 需要清理其冗余 embedding 记录（保留最新一对 identity+semantic）
- 需要清理 article_topic_tags 中与"共产党员"无关的关联
- 需要排查是否有其他 tag 存在类似问题（embedding 数量异常多）

**Step 1: 排查其他异常 tag**

```sql
-- 找 embedding 数量异常的 tag（正常 tag 2-4 条，异常的可能 >10）
SELECT tt.id, tt.label, tt.category, COUNT(tte.id) as emb_count
FROM topic_tags tt
JOIN topic_tag_embeddings tte ON tte.topic_tag_id = tt.id
WHERE tt.deleted_at IS NULL
GROUP BY tt.id, tt.label, tt.category
HAVING COUNT(tte.id) > 10
ORDER BY emb_count DESC;
```

记录结果，评估是否需要处理。

**Step 2: 清理 tag 94712 冗余 embedding**

```sql
-- 先看现有 embedding
SELECT id, topic_tag_id, embedding_type, text_hash, created_at
FROM topic_tag_embeddings
WHERE topic_tag_id = 94712
ORDER BY embedding_type, created_at DESC;

-- 删除旧记录，每种 type 只保留最新一条
DELETE FROM topic_tag_embeddings
WHERE topic_tag_id = 94712
AND id NOT IN (
    SELECT DISTINCT ON (embedding_type) id
    FROM topic_tag_embeddings
    WHERE topic_tag_id = 94712
    ORDER BY embedding_type, created_at DESC
);
```

**Step 3: 清理 tag 94712 错误的 article_topic_tags**

```sql
-- 查看关联的文章
SELECT att.article_id, att.topic_tag_id, att.source, a.title
FROM article_topic_tags att
JOIN articles a ON a.id = att.article_id
WHERE att.topic_tag_id = 94712
ORDER BY att.source;

-- 手动审查：只保留 source='llm_extract' 且文章标题确实与"共产党员"相关的记录
-- 删除明显无关的关联（义诊、股市、Codex、电影票房等）
DELETE FROM article_topic_tags
WHERE topic_tag_id = 94712
AND article_id NOT IN (
    -- 手动填入确认相关的 article_id 列表
    SELECT article_id FROM article_topic_tags
    WHERE topic_tag_id = 94712 AND source = 'llm_extract'
    -- 进一步人工确认
);
```

注意：这个步骤需要根据实际数据人工判断，不能全自动化。先列出所有关联，人工审查后再删。

**Step 4: Commit (如有 SQL 脚本)**

如果生成了修复 SQL 脚本，保存到 `docs/experience/tag-94712-cleanup.sql` 并 commit。

---

## Summary

| Task | 描述 | 依赖 | 预估 |
|------|------|------|------|
| 1 | TagMatch 降级 + 删 keyword override + 测试 | 无 | 后端独立 |
| 2 | SaveEmbedding 清理旧记录 + 测试 | 无 | 后端独立 |
| 3 | TagsPage 接入 TagMergePreview | 无 | 前端独立 |
| 4 | 数据修复 | Task 1, 2 | 需要 DB 访问 |

Task 1, 2, 3 完全独立，可以并行派发。Task 4 需要等 1、2 部署后再执行。
