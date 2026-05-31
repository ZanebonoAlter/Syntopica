# 候选时间窗口过滤 Implementation Plan

> **REQUIRED SUB-SKILL:** Use the subagent-driven-development skill to implement this plan task-by-task.

**Goal:** 为升级建议流程添加按文章活动时间过滤候选的功能，默认只收集今天的候选，用户可在前端选择时间窗口。

**Architecture:** 后端 `collectCandidates` 从简单 WHERE 查询改为 4 表 JOIN（semantic_labels → topic_tag_semantic_labels → article_topic_tags → articles），按 `articles.created_at >= now() - interval 'N days'` 过滤。前端在"获取 LLM 建议"按钮旁添加时间窗口下拉选择器。

**Tech Stack:** Go/Gin/GORM (后端), Vue 3/Nuxt (前端)

---

### Task 1: 后端 — `collectCandidates` 支持 `days` 参数

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:115` (GenerateSuggestions 签名)
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:244` (collectCandidates 签名和 SQL)
- Test: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: 修改 `collectCandidates` 签名，添加 `days int` 参数**

在 `backend-go/internal/domain/tagging/semantic_board_upgrade.go`:

```go
// 第 244 行，函数签名从：
func (s *SemanticBoardUpgradeService) collectCandidates(ctx context.Context, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCandidate, error) {
// 改为：
func (s *SemanticBoardUpgradeService) collectCandidates(ctx context.Context, config SemanticBoardUpgradeConfig, days int) ([]SemanticBoardUpgradeCandidate, error) {
```

**Step 2: 修改 `collectCandidates` 查询逻辑**

替换原有查询（第 246-252 行），改为：

```go
func (s *SemanticBoardUpgradeService) collectCandidates(ctx context.Context, config SemanticBoardUpgradeConfig, days int) ([]SemanticBoardUpgradeCandidate, error) {
	var labels []models.SemanticLabel
	query := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND ref_count >= ? AND embedding IS NOT NULL", "auxiliary", "active", config.RefCountThreshold).
		Where("NOT EXISTS (SELECT 1 FROM board_composition WHERE board_composition.auxiliary_label_id = semantic_labels.id)")

	if days > 0 {
		subQuery := s.db.WithContext(ctx).
			Table("topic_tag_semantic_labels AS ttsl").
			Select("DISTINCT ttsl.semantic_label_id").
			Joins("JOIN article_topic_tags AS att ON att.topic_tag_id = ttsl.topic_tag_id").
			Joins("JOIN articles ON articles.id = att.article_id AND articles.created_at >= ?", time.Now().AddDate(0, 0, -days))
		query = query.Where("semantic_labels.id IN (?)", subQuery)
	}

	if err := query.Order("id ASC").Find(&labels).Error; err != nil {
		return nil, err
	}

	candidates := make([]SemanticBoardUpgradeCandidate, 0, len(labels))
	for _, label := range labels {
		vector, err := parsePgVector(*label.Embedding)
		if err != nil {
			continue
		}
		candidates = append(candidates, SemanticBoardUpgradeCandidate{ID: label.ID, Label: label.Label, Slug: label.Slug, RefCount: label.RefCount, Embedding: vector})
	}
	return candidates, nil
}
```

**Step 3: 修改 `GenerateSuggestions` 签名，透传 `days`**

```go
// 第 115 行，从：
func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
// 改为：
func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context, days int) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
```

```go
// 第 120 行，从：
candidates, err := s.collectCandidates(ctx, config)
// 改为：
candidates, err := s.collectCandidates(ctx, config, days)
```

**Step 4: 修改 handler `suggestUpgrades`，从查询参数读取 `days` 并传递**

在 `backend-go/internal/domain/tagging/semantic_board_handler.go`:

```go
// 第 1187 行，从：
func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	service := NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	suggestions, clusters, err := service.GenerateSuggestions(c.Request.Context())
// 改为：
func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	days := 1 // default: today
	if d, err := strconv.Atoi(c.Query("days")); err == nil && d >= 0 {
		days = d
	}
	service := NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	suggestions, clusters, err := service.GenerateSuggestions(c.Request.Context(), days)
```

handler 文件顶部已有 `"strconv"` import，确认不需要额外 import。

**Step 5: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译通过，无错误

**Step 6: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade.go backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat: add days parameter to collectCandidates for time-window filtering"
```

---

### Task 2: 后端 — 单元测试

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: 找到现有 `collectCandidates` 相关测试**

搜索现有测试中调用 `collectCandidates` 或 `GenerateSuggestions` 的地方，需要更新签名。

```bash
grep -n "collectCandidates\|GenerateSuggestions" backend-go/internal/domain/tagging/semantic_board_upgrade_test.go
```

所有调用点都需要添加 `days` 参数。对于现有测试（不需要时间过滤），传 `0`。

**Step 2: 新增时间窗口过滤测试**

```go
func TestCollectCandidatesWithTimeWindow(t *testing.T) {
	// 此测试验证 days 参数的行为：
	// days=0: 不过滤时间，行为与原来相同
	// days=1: 只收集今天文章中出现的候选
	// 需要用 test DB mock 或真实测试 DB
	// 如果现有测试使用 mock DB，在此测试中构建 mock 数据：
	//   - 插入一个 auxiliary label (ref_count=5, embedding 非空, 无 board_composition)
	//   - 插入关联的 topic_tag_semantic_label + article_topic_tag + article
	//   - 验证 days=0 时能查到，days=1 且 article.created_at 是今天时能查到
	//   - 验证 days=1 且 article.created_at 是昨天时查不到
}
```

具体测试代码取决于现有测试基础设施。先看 `semantic_board_upgrade_test.go` 中的测试模式和 DB 设置方式。

**Step 3: 运行受影响的测试**

Run: `cd backend-go && go test ./internal/domain/tagging/ -run "TestCollectCandidates|TestGenerateSuggestions|TestSemanticBoardUpgrade" -v`
Expected: 全部 PASS

**Step 4: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade_test.go
git commit -m "test: add time window filtering tests for collectCandidates"
```

---

### Task 3: 前端 — API 层支持 `days` 参数

**Files:**
- Modify: `front/app/api/semanticBoards.ts:269`

**Step 1: 修改 `suggestUpgrade` 函数签名**

```typescript
// 从：
async function suggestUpgrade(): Promise<ApiResponse<UpgradeSuggestResponse>> {
    return apiClient.post('/semantic-boards/upgrade-suggest')
}

// 改为：
async function suggestUpgrade(days: number = 1): Promise<ApiResponse<UpgradeSuggestResponse>> {
    return apiClient.post(`/semantic-boards/upgrade-suggest?days=${days}`)
}
```

**Step 2: 验证 TypeScript 编译**

Run (Windows cmd): `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: 无错误

**Step 3: Commit**

```bash
git add front/app/api/semanticBoards.ts
git commit -m "feat: add days parameter to suggestUpgrade API call"
```

---

### Task 4: 前端 — UpgradeSuggestionPanel 添加时间窗口选择器

**Files:**
- Modify: `front/app/features/tags/components/UpgradeSuggestionPanel.vue`
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Step 1: UpgradeSuggestionPanel 新增 `days` 相关 props/emit**

在 `UpgradeSuggestionPanel.vue`:

1. 新增 `selectedDays` ref 和时间窗口选项：
```typescript
const timeWindowOptions = [
  { label: '今天', days: 1 },
  { label: '最近 3 天', days: 3 },
  { label: '最近 7 天', days: 7 },
  { label: '最近 30 天', days: 30 },
  { label: '全部', days: 0 },
]
const selectedDays = ref(1)
```

2. 修改 `suggest` emit，携带 `days`：
```typescript
const emit = defineEmits<{
  suggest: [days: number]
  execute: [suggestion: UpgradeSuggestion]
  cancel: []
}>()
```

3. 在模板中，"获取 LLM 建议" 按钮旁添加下拉选择器，和按钮同行显示。下拉放在按钮左侧，样式与现有 merge dropdown 一致（暗色主题、圆角、半透明背景）。两个触发 `suggest` 的按钮都传 `selectedDays`。

**Step 2: TagsPage 修改 `handleSuggestUpgrade` 传递 `days`**

在 `TagsPage.vue`:

```typescript
// 从：
async function handleSuggestUpgrade() {
  upgradeSuggesting.value = true
  upgradeBackfillNotice.value = false
  const res = await sbApi.suggestUpgrade()
  // ...
}

// 改为：
async function handleSuggestUpgrade(days: number = 1) {
  upgradeSuggesting.value = true
  upgradeBackfillNotice.value = false
  const res = await sbApi.suggestUpgrade(days)
  // ...
}
```

同时更新模板中 `@suggest` 的事件处理，透传 `days`。

**Step 3: 验证前端编译**

Run: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm lint"`
Expected: 无错误

Run: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: 无错误

**Step 4: Commit**

```bash
git add front/app/features/tags/components/UpgradeSuggestionPanel.vue front/app/features/tags/components/TagsPage.vue
git commit -m "feat: add time window selector to upgrade suggestion panel"
```

---

### Task 5: 文档更新

**Files:**
- Modify: `docs/reference/api/semantic-boards.md`

**Step 1: 更新 upgrade-suggest 端点文档**

在 upgrade-suggest API 说明中添加 `days` 查询参数：
- 参数名: `days`
- 类型: `int`
- 默认值: `1`
- 说明: 按最近 N 天的文章活动时间过滤候选；`0` 表示不过滤（全量）

**Step 2: Commit**

```bash
git add docs/reference/api/semantic-boards.md
git commit -m "docs: document days query parameter for upgrade-suggest API"
```
