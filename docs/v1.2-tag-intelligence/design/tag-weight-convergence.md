# Tag Weight Convergence Design

**Date:** 2026-03-23
**Status:** Draft

## Goal

解决 tag 越打越多、没有收敛机制的问题。核心思路：给每个 tag 维护一个 feed 维度的权重（被多少不同 feed 的文章引用），打 tag 时用 embedding 相似度 × 权重作为综合评分，优先复用已有高权重 tag，抑制新 tag 滥生。

## Why This Route

- 当前 `findOrCreateTag()` 只做 slug 精确匹配，不同拼写直接新建 tag，没有收敛
- embedding 相似度匹配确实存在（`extractor_enhanced.go:resolveCandidate`），但 threshold 过高（0.97），且两套逻辑不统一
- 一个 tag 被多个 feed 独立报导，说明它是跨源热点，语义上更值得复用
- `ln(feed_count + 1)` 作为乘数：feed_count=1 → ×0.69，feed_count=3 → ×1.39，feed_count=10 → ×2.40，避免线性放大的极端化

## Current Architecture Fit

需要修改的关键文件：

| 文件 | 改动点 |
|------|--------|
| `backend-go/internal/domain/models/topic_graph.go` | TopicTag 加 `FeedCount` 字段 |
| `backend-go/internal/domain/topictypes/types.go` | TopicTag/AggregatedTopicTag 加 `FeedCount` |
| `backend-go/internal/domain/topicextraction/extractor_enhanced.go` | 重写 `resolveCandidate`，统一评分逻辑 |
| `backend-go/internal/domain/topicextraction/tagger.go` | `findOrCreateTag` 接入统一评分 |
| `backend-go/internal/domain/topicextraction/article_tagger.go` | tag 关联创建/删除时更新 `feed_count` |
| `backend-go/internal/domain/topicanalysis/embedding.go` | 新增 `WeightedTagMatch` 方法，接受 weight 输入 |

## Design

### 1. DB Schema 变更

在 `topic_tags` 表新增字段：

```go
type TopicTag struct {
    // ... existing fields ...
    FeedCount int `gorm:"default:0" json:"feed_count"` // 引用该 tag 的 distinct feed 数量
}
```

**feed_count 维护时机**：

```
article_topic_tags 创建时 → 重新计算该 tag 的 feed_count
article_topic_tags 删除时 → 重新计算该 tag 的 feed_count
```

计算逻辑：

```sql
SELECT COUNT(DISTINCT articles.feed_id)
FROM article_topic_tags
JOIN articles ON articles.id = article_topic_tags.article_id
WHERE article_topic_tags.topic_tag_id = ?
```

### 2. 综合评分公式

```
score(tag) = embedding_similarity × ln(feed_count + 1)
```

| feed_count | ln(fc+1) | 效果 |
|-----------|----------|------|
| 0 | 0 | 新建/孤立 tag，similarity 不放大 |
| 1 | 0.69 | 1 个 feed 引用，轻微放大 |
| 3 | 1.39 | 3 个 feed，similarity 放大约 40% |
| 5 | 1.79 | |
| 10 | 2.40 | 跨源热点，强复用倾向 |

**决策阈值**：

```go
// 默认阈值
ReuseScoreThreshold = 0.75  // score >= 0.75 → 复用
CreateNewThreshold  = 0.50  // score < 0.50 → 新建
// 中间区间 → AI 判断（保持现有能力）
```

示例：candidate tag "OpenAI"，数据库中已有 "openai" tag（feed_count=3），
embedding similarity=0.85 → score = 0.85 × 1.39 = 1.18 ≥ 0.75 → 直接复用

### 3. 统一 resolve 流程

当前问题：`resolveCandidate`（enhanced）和 `findOrCreateTag`（tagger）是两套独立逻辑。

**统一后**：

```
ExtractCandidate(label, category)
  → step 1: slug 精确匹配（category 内）→ 直接复用
  → step 2: alias 匹配 → 直接复用
  → step 3: embedding 相似度检索 top-K 同类 tag
      → 每个 candidate: score = similarity × ln(feed_count + 1)
      → 最高 score >= 0.75 → 复用
      → 最高 score < 0.50 → 新建
      → 中间 → AI 判断（prompt 中附带 weight 信息）
  → step 4: 无 embedding provider → slug 模糊匹配 + weight 兜底
```

### 4. Embedding 优化

当前问题：
- `FindSimilarTags` 加载同类别所有 embedding 逐个比对，不可扩展
- `TagMatch` threshold 过高（0.97），相似但不同的拼写被放过

优化方向：

1. **降低 HighSimilarity threshold**：0.97 → 0.92（更积极复用）
2. **保留全量比对**：SQLite 不支持原生向量索引，数据量级（几百到几千 tag）可接受全量比对
3. **`FindSimilarTags` 返回时附带 `feed_count`**：调用方直接用 score 公式计算

### 5. FeedCount 维护

新建辅助函数：

```go
// RecalculateFeedCount recalculates feed_count for a single tag
func RecalculateFeedCount(tagID uint) error

// RecalculateFeedCounts batch recalculates for multiple tags
func RecalculateFeedCounts(tagIDs []uint) error
```

调用点：
- `article_tagger.go:tagArticle()` 创建 ArticleTopicTag 后
- `article_tagger.go:tagArticle(Force=true)` 删旧标签后
- `tagger.go:TagSummary()` 创建 AISummaryTopic 后
- 手动打标签接口 `POST /api/articles/:id/tags`

### 6. 前端展示

`topic_tags.feed_count` 作为可选展示字段，供前端在 topic graph 节点上显示"跨 N 个 feed"等信息，但非本次必须。

## 数据迁移

1. 给 `topic_tags` 表加 `feed_count INTEGER DEFAULT 0` 列
2. 一次性回填：

```sql
UPDATE topic_tags SET feed_count = (
    SELECT COUNT(DISTINCT articles.feed_id)
    FROM article_topic_tags
    JOIN articles ON articles.id = article_topic_tags.article_id
    WHERE article_topic_tags.topic_tag_id = topic_tags.id
);
```

3. GORM AutoMigrate 会自动加列（SQLite `ALTER TABLE ADD COLUMN`）

## Implementation Steps

### Phase 1: 数据层
1. `models/topic_graph.go` — TopicTag 加 FeedCount 字段
2. `topictypes/types.go` — TopicTag/AggregatedTopicTag 加 FeedCount
3. `topicextraction/article_tagger.go` — 新增 `RecalculateFeedCount` 函数
4. 写迁移脚本回填历史数据

### Phase 2: 核心逻辑
5. `topicanalysis/embedding.go` — 新增 `WeightedTagMatch(label, category, limit)` 方法
   - 接受候选 label，生成 embedding
   - 返回 top-K similar tags 附带 feed_count
   - 调用方自己算综合 score
6. 降低 `DefaultThresholds.HighSimilarity` 到 0.92

### Phase 3: 统一 resolve
7. `topicextraction/extractor_enhanced.go` — 重写 `resolveCandidate`
   - 使用 `WeightedTagMatch` 获取候选
   - 计算综合 score = similarity × ln(feed_count + 1)
   - 决策逻辑：>= 0.75 复用，< 0.50 新建，中间 AI 判断
8. `topicextraction/tagger.go` — `findOrCreateTag` 也接入统一评分
   - slug 精确匹配 → 直接复用
   - 不匹配 → 走 embedding + weight 流程

### Phase 4: 维护与测试
9. 所有 ArticleTopicTag 创建/删除点接入 `RecalculateFeedCount`
10. 补充单元测试
11. 手动验证：多次打 tag 后观察 tag 总数是否收敛

## Risks & Mitigations

| 风险 | 缓解措施 |
|------|---------|
| Embedding API 不可用 | fallback 到 slug alias 匹配 + weight，不阻塞打标签 |
| feed_count 更新有延迟（并发） | SQLite 单用户场景并发低，问题不大；最终一致 |
| 复用导致 tag 语义模糊 | threshold 不宜过低，0.75 是保守值；AI judgment 作为中间兜底 |
| 已有脏数据 feed_count=0 | 迁移脚本一次性回填 |

## Verification

1. 单元测试：`go test ./internal/domain/topicextraction/... -v`
2. 集成测试：`go test ./internal/domain/topicanalysis/... -v`
3. 手动验证：
   - 启动后端，触发 feed 刷新
   - 观察新打的 tag 是否复用已有 tag（而非新建）
   - 检查 `topic_tags.feed_count` 是否递增
   - topic graph 页面确认 tag 总数未膨胀
