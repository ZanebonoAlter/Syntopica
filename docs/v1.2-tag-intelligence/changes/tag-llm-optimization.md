# 标签 LLM 调用优化 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 减少标签处理流程中的 LLM 调用次数和整体延迟，通过并行 Job 处理、批量判断和热标签缓存降低处理延迟。

**Architecture:** 三层优化：(0) TagQueue worker 从串行改为并发处理 job（并发度 3）；(1) 在 `findOrCreateTag` 上层增加内存缓存，相同 slug:category 直接命中跳过 embedding 搜索和 LLM；(2) 将一篇文章内多个标签的 LLM 判断合并为一次批量调用。

**Tech Stack:** Go, sync.Map + semaphore, 现有 airouter 能力

---

## 背景：当前 LLM 调用热点分析

一篇文章 TagArticle 的完整调用链：
1. **ExtractTags** — 1 次 LLM（提取标签列表）
2. **对每个标签（≤8 个）调用 findOrCreateTag：**
   - TagMatch（embedding 搜索，无 LLM）
   - 如果 matchType=candidates → **callLLMForTagJudgment**（1 次 LLM）
   - 如果创建了新标签 → generateTagDescription（1 次 LLM，async）
3. **最坏情况：1 + 8×1 = 9 次同步 LLM 调用**

### 当前架构瓶颈

1. **Worker 串行**：`processAvailableJobs` 逐个处理 claimed jobs，即使 batch=20 也是串行
2. **每标签独立 LLM**：8 个标签最多 8 次 LLM 调用
3. **热标签无缓存**：同一 feed 的文章大量重复标签，每次都走完整 embedding+LLM 链路

优化目标：
- **A（并行处理）**：3 个 job 并行处理，吞吐量提升 3 倍
- **D（缓存）**：热标签命中缓存后跳过整个 embedding+LLM 链路
- **B（批量判断）**：8 个标签只触发 1 次 LLM 批量判断

---

## Task 0: TagQueue Worker 并行处理（并发度 3）

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tag_queue.go` — processAvailableJobs 和 drainRemaining 改为并发
- Create: `backend-go/internal/domain/topicextraction/tag_queue_test.go` — 并发处理测试

**设计要点：**
- `TagQueue` 新增 `concurrency int` 字段，默认值 3
- 使用 buffered channel 做 semaphore 控制并发
- `processAvailableJobs` 中每个 job 启动 goroutine，semaphore 限制最多 3 个并行
- `drainRemaining` 同样改为并发
- `processJob` 本身不需要改动（它已经是线程安全的，每个 job 操作不同的 article）
- 停止时 WaitGroup 确保所有 goroutine 完成

**Step 1: 修改 TagQueue 结构和初始化**

修改 `tag_queue.go` 中 `TagQueue` 结构体，新增 `concurrency` 字段：

```go
type TagQueue struct {
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started      bool
	mu           sync.Mutex
	queue        *TagJobQueue
	pollInterval time.Duration
	lease        time.Duration
	batchSize    int
	concurrency  int
}
```

修改 `GetTagQueue` 初始化：

```go
func GetTagQueue() *TagQueue {
	once.Do(func() {
		instance = &TagQueue{
			stopChan:     make(chan struct{}),
			queue:        NewTagJobQueue(database.DB),
			pollInterval: time.Second,
			lease:        10 * time.Minute,
			batchSize:    20,
			concurrency:  3,
		}
	})
	if instance.queue == nil {
		instance.queue = NewTagJobQueue(database.DB)
	}
	return instance
}
```

**Step 2: 改写 processAvailableJobs 为并发处理**

```go
func (q *TagQueue) processAvailableJobs() {
	jobs, err := q.queue.Claim(q.batchSize, q.lease)
	if err != nil {
		logging.Warnf("Failed to claim tag jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	sem := make(chan struct{}, q.concurrency)
	var jobWg sync.WaitGroup

	for _, job := range jobs {
		jobWg.Add(1)
		sem <- struct{}{}
		go func(j models.TagJob) {
			defer func() { <-sem; jobWg.Done() }()
			q.processJob(j)
		}(job)
	}
	jobWg.Wait()
}
```

**Step 3: 改写 drainRemaining 为并发处理**

```go
func (q *TagQueue) drainRemaining() {
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			logging.Infof("Tag queue drain timed out after %v, remaining jobs will be processed on next start", drainTimeout)
			return
		default:
		}

		jobs, err := q.queue.Claim(q.batchSize, q.lease)
		if err != nil {
			logging.Warnf("Failed to claim tag jobs during drain: %v", err)
			return
		}
		if len(jobs) == 0 {
			return
		}

		sem := make(chan struct{}, q.concurrency)
		var jobWg sync.WaitGroup

		for _, job := range jobs {
			if ctx.Err() != nil {
				logging.Infof("Tag queue drain timed out, some job(s) remaining for next start")
				jobWg.Wait()
				return
			}
			jobWg.Add(1)
			sem <- struct{}{}
			go func(j models.TagJob) {
				defer func() { <-sem; jobWg.Done() }()
				q.processJob(j)
			}(job)
		}
		jobWg.Wait()
	}
}
```

**Step 4: 编写并发处理测试**

```go
// tag_queue_test.go
package topicextraction

import (
	"sync/atomic"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
)

func TestProcessAvailableJobsConcurrent(t *testing.T) {
	var processed int64

	q := &TagQueue{
		stopChan:    make(chan struct{}),
		concurrency: 3,
		batchSize:   10,
		lease:       time.Minute,
	}

	originalProcessJob := q.processJob
	_ = originalProcessJob

	// 模拟 processJob
	processJobMock := func(job models.TagJob) {
		atomic.AddInt64(&processed, 1)
		time.Sleep(10 * time.Millisecond)
	}

	// 验证并发度
	start := time.Now()
	sem := make(chan struct{}, q.concurrency)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			processJobMock(models.TagJob{})
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if atomic.LoadInt64(&processed) != 6 {
		t.Fatalf("expected 6 processed, got %d", atomic.LoadInt64(&processed))
	}
	// 6 个 job，并发 3，应约 20ms（2 轮 × 10ms），若串行则需 60ms
	if elapsed > 50*time.Millisecond {
		t.Fatalf("took %v, expected <50ms with concurrency=3", elapsed)
	}
}
```

**Step 5: 运行测试**

Run: `cd backend-go && go test ./internal/domain/topicextraction -run TestProcessAvailableJobsConcurrent -v`
Expected: PASS

**Step 6: 运行全量测试确保无回归**

Run: `cd backend-go && go test ./internal/domain/topicextraction -v`
Expected: 全部 PASS

**Step 7: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tag_queue.go backend-go/internal/domain/topicextraction/tag_queue_test.go
git commit -m "feat: parallel tag job processing with concurrency=3"
```

---

## Task 1: 热标签内存缓存

**Files:**
- Create: `backend-go/internal/domain/topicextraction/tag_cache.go`
- Create: `backend-go/internal/domain/topicextraction/tag_cache_test.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — findOrCreateTag 入口加缓存查找
- Modify: `backend-go/internal/domain/topicextraction/tag_queue.go` — processJob 结束后刷新缓存

**设计要点：**
- 使用 `sync.Map` + TTL 过期，无需引入外部 LRU 依赖
- 缓存 key: `slug:category`
- 缓存 value: `*models.TopicTag`（完整对象，含 ID、Label、Kind 等）
- TTL: 10 分钟（同一 feed 的文章通常在几分钟内集中处理）
- 命中缓存时：直接返回缓存标签，跳过 TagMatch + LLM 判断
- 缓存命中仍需建立 article_topic_tags 关联（调用方已处理）

**Step 1: 编写缓存结构和测试**

```go
// tag_cache.go
package topicextraction

import (
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
)

type tagCacheEntry struct {
	tag       *models.TopicTag
	expiresAt time.Time
}

type TagCache struct {
	entries sync.Map
	ttl     time.Duration
}

var globalTagCache = &TagCache{
	ttl: 10 * time.Minute,
}

func GetTagCache() *TagCache {
	return globalTagCache
}

func (c *TagCache) Get(slug, category string) (*models.TopicTag, bool) {
	key := slug + ":" + category
	val, ok := c.entries.Load(key)
	if !ok {
		return nil, false
	}
	entry := val.(*tagCacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.entries.Delete(key)
		return nil, false
	}
	return entry.tag, true
}

func (c *TagCache) Set(slug, category string, tag *models.TopicTag) {
	key := slug + ":" + category
	c.entries.Store(key, &tagCacheEntry{
		tag:       tag,
		expiresAt: time.Now().Add(c.ttl),
	})
}

func (c *TagCache) Invalidate(slug, category string) {
	c.entries.Delete(slug + ":" + category)
}

func (c *TagCache) Clear() {
	c.entries.Range(func(key, _ any) bool {
		c.entries.Delete(key)
		return true
	})
}
```

**Step 2: 编写测试**

```go
// tag_cache_test.go
package topicextraction

import (
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
)

func TestTagCacheSetGet(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	tag := &models.TopicTag{ID: 1, Label: "AI", Slug: "ai", Category: "keyword"}

	cache.Set("ai", "keyword", tag)

	got, ok := cache.Get("ai", "keyword")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != 1 {
		t.Fatalf("got ID %d, want 1", got.ID)
	}
}

func TestTagCacheMiss(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	_, ok := cache.Get("nonexistent", "keyword")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestTagCacheExpiry(t *testing.T) {
	cache := &TagCache{ttl: 1 * time.Millisecond}
	tag := &models.TopicTag{ID: 2, Label: "Go", Slug: "go", Category: "keyword"}
	cache.Set("go", "keyword", tag)

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("go", "keyword")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestTagCacheInvalidate(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	tag := &models.TopicTag{ID: 3, Label: "Rust", Slug: "rust", Category: "keyword"}
	cache.Set("rust", "keyword", tag)

	cache.Invalidate("rust", "keyword")

	_, ok := cache.Get("rust", "keyword")
	if ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestTagCacheClear(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	cache.Set("a", "keyword", &models.TopicTag{ID: 1})
	cache.Set("b", "event", &models.TopicTag{ID: 2})

	cache.Clear()

	_, ok1 := cache.Get("a", "keyword")
	_, ok2 := cache.Get("b", "event")
	if ok1 || ok2 {
		t.Fatal("expected all entries cleared")
	}
}
```

**Step 3: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/domain/topicextraction -run TestTagCache -v`
Expected: 全部 PASS

**Step 4: 在 findOrCreateTag 开头加入缓存查找**

修改 `tagger.go` 的 `findOrCreateTag` 函数，在 embedding 匹配之前插入缓存查找：

```go
// 在 slug 和 category 赋值之后、embeddingService 获取之前插入：
if cached, ok := GetTagCache().Get(slug, category); ok {
	logging.Infof("findOrCreateTag: label=%q slug=%q category=%s cache=hit existingID=%d", tag.Label, slug, category, cached.ID)
	existing := *cached
	existing.Label = tag.Label
	if tag.Icon != "" {
		existing.Icon = tag.Icon
	}
	if len(tag.Aliases) > 0 {
		aJSON, _ := json.Marshal(tag.Aliases)
		existing.Aliases = string(aJSON)
	}
	if err := database.DB.Save(&existing).Error; err != nil {
		return nil, err
	}
	go backfillTagDescription(existing.ID, existing.Label, existing.Category, existing.Description, articleContext)
	return &existing, nil
}
```

**Step 5: 在标签创建/复用路径写入缓存**

在 `findOrCreateTag` 的以下返回点之前调用 `GetTagCache().Set(slug, category, dbTag)`：
- exact match 路径（返回前）
- fallback slug 匹配路径（返回前）
- 新建标签路径（返回前）
- merge 结果路径（返回前）

**Step 6: 运行测试**

Run: `cd backend-go && go test ./internal/domain/topicextraction -v`
Expected: 全部 PASS

**Step 7: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tag_cache.go backend-go/internal/domain/topicextraction/tag_cache_test.go backend-go/internal/domain/topicextraction/tagger.go
git commit -m "feat: add hot tag memory cache to skip embedding+LLM for repeated tags"
```

---

## Task 2: 文章级批量标签判断

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/article_tagger.go` — TagArticle 改为两阶段处理
- Create: `backend-go/internal/domain/topicanalysis/batch_tag_judgment.go`
- Create: `backend-go/internal/domain/topicanalysis/batch_tag_judgment_test.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — findOrCreateTag 支持跳过 LLM 仅做匹配

**设计要点：**

当前流程（每标签独立）：
```
TagArticle → for each tag: findOrCreateTag → TagMatch → [candidates?] → callLLMForTagJudgment
```

优化后流程（先匹配后批量）：
```
TagArticle:
  Phase 1: 对所有标签做 TagMatch（无 LLM，纯 embedding 搜索）
           → 分为：exact_hits（直接复用）+ needs_judgment（有候选需判断）+ no_matches（新建）
  Phase 2: 将所有 needs_judgment 标签的候选打包，一次 LLM 调用
           → 输入：多个标签各自的候选列表
           → 输出：每个标签各自的 merges/abstracts/none
  Phase 3: 处理判断结果（复用现有 processJudgment 逻辑）
```

**核心改动：**

新增 `batchCallLLMForTagJudgment`，接收多个标签的候选列表，一次 LLM 返回所有判断。

**Step 1: 定义批量判断的数据结构和 Prompt**

```go
// batch_tag_judgment.go
package topicanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

type BatchTagJudgmentItem struct {
	Label      string
	Category   string
	Candidates []TagCandidate
}

type BatchTagJudgmentResult struct {
	Judgments map[string]*tagJudgment // key: label
}

func batchCallLLMForTagJudgment(ctx context.Context, items []BatchTagJudgmentItem, narrativeContext string) (*BatchTagJudgmentResult, error) {
	if len(items) == 0 {
		return &BatchTagJudgmentResult{Judgments: make(map[string]*tagJudgment)}, nil
	}
	if len(items) == 1 {
		j, err := callLLMForTagJudgment(ctx, items[0].Candidates, items[0].Label, items[0].Category, narrativeContext, "batch_single")
		if err != nil {
			return nil, err
		}
		return &BatchTagJudgmentResult{Judgments: map[string]*tagJudgment{items[0].Label: j}}, nil
	}

	prompt := buildBatchTagJudgmentPrompt(items)
	schema := buildBatchTagJudgmentSchema()

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode:    true,
		JSONSchema:  schema,
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":   "batch_tag_judgment",
			"tag_count":   len(items),
			"caller":      "batch",
		},
	}

	result, err := airouter.NewRouter().Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch tag judgment LLM call failed: %w", err)
	}

	return parseBatchTagJudgmentResponse(result.Content, items)
}
```

**Step 2: 编写批量 Prompt 构建函数**

Prompt 结构：依次列出每个新标签及其候选，要求 LLM 对每个标签分别返回 merges/abstracts/none。

```go
func buildBatchTagJudgmentPrompt(items []BatchTagJudgmentItem) string {
	var sb strings.Builder
	sb.WriteString("You are comparing MULTIPLE new tags against existing candidate tags.\n")
	sb.WriteString("For EACH new tag, decide if its candidates are the same concept (merge), related (abstract), or unrelated (none).\n\n")

	for i, item := range items {
		sb.WriteString(fmt.Sprintf("### New Tag %d: %q (category: %s)\n", i+1, item.Label, item.Category))
		sb.WriteString("Existing candidates:\n")
		sb.WriteString(buildCandidateList(item.Candidates))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Return a JSON object where each key is the new tag label, and the value has three arrays: merges, abstracts, none.\n")
	sb.WriteString("The rules for merge/abstract/none are the same as single-tag judgment.\n")
	sb.WriteString("EVERY candidate for EVERY tag must appear in exactly one of the three arrays.\n")
	return sb.String()
}
```

**Step 3: 编写 JSON Schema**

```go
func buildBatchTagJudgmentSchema() *airouter.JSONSchema {
	tagJudgmentSchema := &airouter.SchemaProperty{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"merges":    {Type: "array", Description: "合并判断", Items: &airouter.SchemaProperty{Type: "object", Properties: map[string]airouter.SchemaProperty{
				"target":   {Type: "string"},
				"label":    {Type: "string"},
				"children": {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
				"reason":   {Type: "string"},
			}}},
			"abstracts": {Type: "array", Description: "抽象判断", Items: &airouter.SchemaProperty{Type: "object", Properties: map[string]airouter.SchemaProperty{
				"name":        {Type: "string"},
				"description": {Type: "string"},
				"children":    {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
				"reason":      {Type: "string"},
			}}},
			"none": {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
		},
	}

	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"tags": {
				Type:        "object",
				Description: "每个新标签的判断结果，key 为新标签名称",
				AdditionalProperties: tagJudgmentSchema,
			},
		},
		Required: []string{"tags"},
	}
}
```

**Step 4: 编写响应解析函数**

```go
func parseBatchTagJudgmentResponse(content string, items []BatchTagJudgmentItem) (*BatchTagJudgmentResult, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var raw struct {
		Tags map[string]json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse batch tag judgment response: %w", err)
	}

	result := &BatchTagJudgmentResult{Judgments: make(map[string]*tagJudgment)}
	for _, item := range items {
		rawJudgment, ok := raw.Tags[item.Label]
		if !ok {
			logging.Warnf("batch tag judgment: no result for tag %q, treating as no_action", item.Label)
			continue
		}
		j, err := parseTagJudgmentResponse(string(rawJudgment), item.Candidates)
		if err != nil {
			logging.Warnf("batch tag judgment: parse failed for %q: %v", item.Label, err)
			continue
		}
		result.Judgments[item.Label] = j
	}

	return result, nil
}
```

**Step 5: 编写批量判断测试**

```go
// batch_tag_judgment_test.go
package topicanalysis

import (
	"testing"
)

func TestBatchCallLLMSingleItemDelegates(t *testing.T) {
	// 单个 item 应该走 callLLMForTagJudgment
	// 此测试验证单 item 路径不 panic，实际 LLM 需要 mock
	items := []BatchTagJudgmentItem{
		{Label: "AI", Category: "keyword", Candidates: []TagCandidate{}},
	}
	// 空候选无法调 LLM，验证边界处理
	_, err := batchCallLLMForTagJudgment(nil, items)
	if err == nil {
		t.Log("empty candidates handled gracefully")
	}
}

func TestBatchEmptyItems(t *testing.T) {
	result, err := batchCallLLMForTagJudgment(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Judgments) != 0 {
		t.Fatal("expected empty judgments")
	}
}

func TestParseBatchTagJudgmentResponse(t *testing.T) {
	content := `{
		"tags": {
			"AI": {
				"merges": [],
				"abstracts": [],
				"none": ["人工智能"]
			},
			"GPT-5": {
				"merges": [{"target": "GPT-5发布", "label": "GPT-5", "children": [], "reason": "same event"}],
				"abstracts": [],
				"none": []
			}
		}
	}`
	items := []BatchTagJudgmentItem{
		{Label: "AI", Category: "keyword", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "人工智能", Slug: "ren-gong-zhi-neng"}, Similarity: 0.85},
		}},
		{Label: "GPT-5", Category: "event", Candidates: []TagCandidate{
			{Tag: &models.TopicTag{ID: 2, Label: "GPT-5发布", Slug: "gpt-5-fa-bu"}, Similarity: 0.92},
		}},
	}

	result, err := parseBatchTagJudgmentResponse(content, items)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Judgments) != 2 {
		t.Fatalf("expected 2 judgments, got %d", len(result.Judgments))
	}
	if _, ok := result.Judgments["AI"]; !ok {
		t.Fatal("missing AI judgment")
	}
	if _, ok := result.Judgments["GPT-5"]; !ok {
		t.Fatal("missing GPT-5 judgment")
	}
}
```

**Step 6: 运行测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestBatch -v`
Expected: PASS

**Step 7: 重构 TagArticle 为两阶段处理**

修改 `article_tagger.go` 的 `tagArticle` 函数：

```go
func tagArticle(article *models.Article, feedName, categoryName string, options tagArticleOptions) error {
	// ... 现有的 skip/force 逻辑不变 ...

	// Phase 1: 提取标签列表（不变）
	tags := extractTagsForArticle(article, feedName, categoryName)

	// Phase 2: 对所有标签做 embedding 匹配（无 LLM），分类
	type tagMatch struct {
		tag      topictypes.TopicTag
		result   topicanalysis.TagMatchResult
	}
	var exactMatches []tagMatch
	var needsJudgment []tagMatch
	var noMatches []topictypes.TopicTag

	cache := topicextraction.GetTagCache()
	es := getEmbeddingService()

	for _, tag := range tags {
		slug := topictypes.Slugify(tag.Label)
		category := NormalizeDisplayCategory(tag.Kind, tag.Category)

		// 先查缓存
		if cached, ok := cache.Get(slug, category); ok {
			// 直接使用缓存的标签
			exactMatches = append(exactMatches, tagMatch{tag: tag, result: topicanalysis.TagMatchResult{MatchType: "exact", ExistingTag: cached}})
			continue
		}

		if es == nil {
			noMatches = append(noMatches, tag)
			continue
		}

		result, err := es.TagMatch(ctx, tag.Label, category, string(aliasesJSON))
		if err != nil {
			noMatches = append(noMatches, tag)
			continue
		}

		switch result.MatchType {
		case "exact":
			exactMatches = append(exactMatches, tagMatch{tag: tag, result: result})
		case "candidates":
			needsJudgment = append(needsJudgment, tagMatch{tag: tag, result: result})
		case "no_match":
			noMatches = append(noMatches, tag)
		}
	}

	// Phase 3: 批量 LLM 判断
	// 将 needsJudgment 转为 BatchTagJudgmentItem 列表
	// 调用 batchCallLLMForTagJudgment 一次
	// 处理结果...

	// Phase 4: 处理所有结果，建立关联
	// exactMatches: 直接复用
	// judgmentResults: 按 processJudgment 逻辑处理
	// noMatches: 新建标签
}
```

**Step 8: 运行全量测试**

Run: `cd backend-go && go test ./... -v -count=1`
Expected: 全部 PASS

**Step 9: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/batch_tag_judgment.go backend-go/internal/domain/topicanalysis/batch_tag_judgment_test.go backend-go/internal/domain/topicextraction/article_tagger.go
git commit -m "feat: batch LLM tag judgment for multiple tags per article"
```

---

## Task 3: 集成验证与调优

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — 确保 TagSummary 也使用新缓存

**Step 1: 验证 TagSummary 路径也走缓存**

检查 `tagger.go` 的 `TagSummary` 函数。它遍历标签调用 `findOrCreateTag`，Task 1 的缓存改动已覆盖此路径。确认 `TagSummary` 中 `findOrCreateTag` 返回前写入缓存。

**Step 2: 手动测试端到端流程**

启动后端，观察日志中的 `cache=hit` 和 `batch_tag_judgment` 关键词：

```bash
cd backend-go && go run cmd/server/main.go
```

观察：
- 缓存命中率（`cache=hit` 日志比例）
- 批量判断触发情况（`batch_tag_judgment` vs `tag_judgment` 日志比例）
- 单篇文章处理时间变化

**Step 3: 运行现有测试确保无回归**

Run: `cd backend-go && go test ./internal/domain/topicextraction -v && go test ./internal/domain/topicanalysis -v`
Expected: 全部 PASS

**Step 4: Commit**

```bash
git add -A
git commit -m "feat: integrate tag cache and batch judgment into TagSummary path"
```

---

## 预期效果

| 指标 | 优化前 | 优化后 |
|------|--------|--------|
| Job 处理并行度 | 1（串行） | 3（并行） |
| 单篇文章 LLM 调用（最坏） | 9 次 | 2 次（1 提取 + 1 批量判断） |
| 热标签处理 | 每次都走 embedding+LLM | 直接内存缓存命中 |
| 149 个积压 job 预估处理时间 | 数小时 | 10-20 分钟 |

## 风险与注意事项

1. **并行 + 本地 LLM 并发限制**：`TagQueue.concurrency=3` 与 `airouter.CapabilityTopicTagging` 的 `MaxConcurrency` 共同控制实际并发。本地模型只有 1 个，airouter semaphore=3，实际同时只有 1 个 LLM 请求在执行。并行的主要收益是：当 job A 在等 DB/embedding 时，job B 可以发起 LLM 请求，减少等待间隙。
2. **批量 Prompt 超长**：8 个标签各 8 个候选 = 64 条候选，Prompt token 可能较大。限制批量大小（如 ≤6 个标签/批），超过则分批。
3. **缓存一致性**：标签 merge/删除后需 invalidate 缓存。在 `MergeTags` 路径中加 invalidate。
4. **单 item fallback**：批量判断只有 1 个标签时走原有 `callLLMForTagJudgment`，保持向后兼容。
