# Scheduler Panel Enhancement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 移除 Digest 在定时任务列表中的冗余展示，为 5 个辅助任务补齐专属中文 UI 和运行摘要面板。

**Architecture:** 后端增强 2 个旧式 scheduler 的 status 返回（preference_update、blocked_article_recovery），前端补齐 schedulerMeta 映射和 GlobalSettingsDialog 中的专属面板。

**Tech Stack:** Go/Gin (backend), Vue 3 + TypeScript (frontend), Tailwind CSS v4

---

### Task 1: 移除 Digest scheduler descriptor

**Files:**
- Modify: `backend-go/internal/jobs/handler.go:94-101`

**Step 1: 移除 digest 条目**

在 `schedulerDescriptors()` 函数中删除 `digest` 条目（约第 94-101 行）：

```go
// 删除这整个块：
{
    Name:        "digest",
    DisplayName: "Digest",
    Description: "Run digest cron schedules",
    Get: func() interface{} {
        return runtimeinfo.DigestSchedulerInterface
    },
},
```

**Step 2: 验证编译通过**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add backend-go/internal/jobs/handler.go
git commit -m "refactor: remove digest from generic scheduler list"
```

---

### Task 2: 增强 PreferenceUpdateScheduler 的 status 数据

**Files:**
- Modify: `backend-go/internal/jobs/preference_update.go`

**Step 1: 添加 lastRunSummary 字段**

在 `PreferenceUpdateScheduler` struct 中添加：

```go
type PreferenceUpdateScheduler struct {
	// ... existing fields ...
	lastRunSummary *PreferenceUpdateRunSummary
}

type PreferenceUpdateRunSummary struct {
	TriggerSource      string `json:"trigger_source"`
	TotalPreferences   int    `json:"total_preferences"`
	UpdatedCount       int    `json:"updated_count"`
	OrphanRepaired     int    `json:"orphan_repaired_count"`
	DeletedCount       int    `json:"deleted_count"`
}
```

**Step 2: 在 runUpdate 中记录统计**

修改 `runUpdate()` 方法，调用 `preferenceService` 后记录 summary 统计。需要先查看 `UpdateAllPreferences` 的返回值来决定如何获取统计数据。如果它不返回统计，则改为用 `UpdateAllPreferencesWithStats` 或在调用前后查数据库计数。

关键：在 `s.mu.Lock()` 保护下设置 `s.lastRunSummary`。

**Step 3: 增强 GetStatus 返回 overview 数据**

让 `GetStatus()` 在返回的 `SchedulerStatusResponse` 中填充 `last_run_summary`（通过让 `handler.go` 的 `enrichStatus` 从 DB scheduler_task 读取），或者改为实现 `GetTaskStatusDetails()` 接口。

推荐方案：让 `runUpdate()` 将 summary JSON 写入 `scheduler_tasks.last_execution_result`，这样 `enrichStatus()` 已有的逻辑会自动将其作为 `last_run_summary` 传递给前端。

**Step 4: 验证编译通过**

Run: `cd backend-go && go build ./...`

**Step 5: Commit**

```bash
git add backend-go/internal/jobs/preference_update.go
git commit -m "feat: add structured run summary to preference update scheduler"
```

---

### Task 3: 增强 BlockedArticleRecoveryScheduler 的 status 数据

**Files:**
- Modify: `backend-go/internal/jobs/blocked_article_recovery.go`

**Step 1: 添加 lastRunSummary 字段**

```go
type BlockedArticleRecoveryRunSummary struct {
	TriggerSource    string `json:"trigger_source"`
	RecoveredCount   int    `json:"recovered_count"`
	BlockedCount     int    `json:"current_blocked_count"`
	Threshold        int    `json:"threshold"`
}
```

在 struct 中添加 `lastRunSummary *BlockedArticleRecoveryRunSummary`。

**Step 2: 在 runRecoveryCycle 中记录统计**

在 recovery cycle 成功完成后，构建 `BlockedArticleRecoveryRunSummary` 并赋值给 `s.lastRunSummary`。

**Step 3: 将 summary 持久化到 scheduler_tasks 表**

与 Task 2 同样的方式：使用 `initSchedulerTask` + `updateSchedulerStatus` 模式（参考 `auto_tag_merge.go` 的模式），将 JSON 写入 `scheduler_tasks.last_execution_result`。

**Step 4: 验证编译通过**

Run: `cd backend-go && go build ./...`

**Step 5: Commit**

```bash
git add backend-go/internal/jobs/blocked_article_recovery.go
git commit -m "feat: add structured run summary to blocked article recovery scheduler"
```

---

### Task 4: 前端 schedulerMeta 补齐 5 个任务元数据

**Files:**
- Modify: `front/app/utils/schedulerMeta.ts`

**Step 1: 添加 5 个任务的中文显示名、图标、颜色**

在 `getSchedulerDisplayName` 中补充：
```typescript
const names: Record<string, string> = {
  'auto_refresh': '后台刷新',
  'auto_summary': '自动总结',
  'firecrawl': '全文爬取',
  'preference_update': '偏好更新',
  'blocked_article_recovery': '阻塞恢复',
  'auto_tag_merge': '标签合并',
  'tag_quality_score': '标签评分',
  'narrative_summary': '叙事摘要',
}
```

在 `getSchedulerIcon` 中补充：
```typescript
const icons: Record<string, string> = {
  'auto_refresh': 'mdi:refresh',
  'auto_summary': 'mdi:brain',
  'firecrawl': 'mdi:spider-web',
  'preference_update': 'mdi:heart-outline',
  'blocked_article_recovery': 'mdi:shield-check-outline',
  'auto_tag_merge': 'mdi:merge',
  'tag_quality_score': 'mdi:star-outline',
  'narrative_summary': 'mdi:book-open-page-variant-outline',
}
```

在 `getSchedulerColor` 中补充：
```typescript
const colors: Record<string, string> = {
  'auto_refresh': 'from-blue-500 to-cyan-500',
  'auto_summary': 'from-ink-500 to-amber-500',
  'firecrawl': 'from-rose-500 to-orange-500',
  'preference_update': 'from-pink-500 to-rose-500',
  'blocked_article_recovery': 'from-emerald-500 to-teal-500',
  'auto_tag_merge': 'from-violet-500 to-purple-500',
  'tag_quality_score': 'from-indigo-500 to-blue-500',
  'narrative_summary': 'from-cyan-500 to-sky-500',
}
```

**Step 2: 更新测试**

在 `front/app/utils/schedulerMeta.test.ts` 中补充对新任务的断言。

**Step 3: 验证**

Run: `cd front && pnpm test:unit -- app/utils/schedulerMeta.test.ts`
Run: `cd front && pnpm exec nuxi typecheck`

**Step 4: Commit**

```bash
git add front/app/utils/schedulerMeta.ts front/app/utils/schedulerMeta.test.ts
git commit -m "feat: add display metadata for 5 auxiliary schedulers"
```

---

### Task 5: GlobalSettingsDialog 添加辅助任务摘要面板

**Files:**
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue`

**Step 1: 添加 helper 函数**

在 script setup 中添加各辅助任务的 summary 提取函数：

```typescript
function getPreferenceUpdateSummary(scheduler: SchedulerStatus) {
  if (scheduler.name !== 'preference_update') return null
  return scheduler.last_run_summary ?? null
}

function getBlockedRecoverySummary(scheduler: SchedulerStatus) {
  if (scheduler.name !== 'blocked_article_recovery') return null
  return scheduler.last_run_summary ?? null
}

function getAutoTagMergeSummary(scheduler: SchedulerStatus) {
  if (scheduler.name !== 'auto_tag_merge') return null
  return scheduler.last_run_summary ?? null
}

function getTagQualityScoreSummary(scheduler: SchedulerStatus) {
  if (scheduler.name !== 'tag_quality_score') return null
  return scheduler.last_run_summary ?? null
}

function getNarrativeSummaryRunSummary(scheduler: SchedulerStatus) {
  if (scheduler.name !== 'narrative_summary') return null
  return scheduler.last_run_summary ?? null
}
```

**Step 2: 在 template 的 database_state div 内，添加 5 个辅助任务的摘要面板**

每个面板参考 auto_refresh 的面板样式（`rounded-2xl border bg-gradient-to-br p-4`），根据各任务的数据字段展示指标：

- **preference_update**: 偏好总数、已更新、孤儿修复、已删除
- **blocked_article_recovery**: 已恢复、当前阻塞数、告警阈值
- **auto_tag_merge**: 检查配对数、已合并、跳过、失败
- **tag_quality_score**: 评分标签数
- **narrative_summary**: 已保存叙事摘要数

**Step 3: 更新底部说明文案**

将：
```html
<li>• <b>后台刷新</b>: 自动检查并刷新有更新间隔设置的订阅源</li>
<li>• <b>自动总结</b>: 为启用 AI 总结的订阅源自动生成内容汇总</li>
<li>• <b>文章总结</b>: 用 Firecrawl 全文生成单篇 AI 总结</li>
<li>• <b>全文爬取</b>: 使用 Firecrawl 抓取文章完整内容</li>
```

替换为：
```html
<li>• <b>后台刷新</b>: 自动检查并刷新有更新间隔设置的订阅源</li>
<li>• <b>自动总结</b>: 为启用 AI 总结的订阅源自动生成内容汇总</li>
<li>• <b>文章总结</b>: 用 Firecrawl 全文生成单篇 AI 总结</li>
<li>• <b>全文爬取</b>: 使用 Firecrawl 抓取文章完整内容</li>
<li>• <b>偏好更新</b>: 从阅读行为数据更新订阅偏好</li>
<li>• <b>阻塞恢复</b>: 恢复因配置变更被阻塞的文章</li>
<li>• <b>标签合并</b>: 基于向量相似度自动合并相似标签</li>
<li>• <b>标签评分</b>: 重算话题标签的质量分数</li>
<li>• <b>叙事摘要</b>: 从活跃话题标签生成每日叙事摘要</li>
```

注意：digest 不再出现在说明中。

**Step 4: 验证**

Run: `cd front && pnpm exec nuxi typecheck`
Run: `cd front && pnpm build`

**Step 5: Commit**

```bash
git add front/app/components/dialog/GlobalSettingsDialog.vue
git commit -m "feat: add dedicated summary panels for auxiliary schedulers"
```

---

### Task 6: 端到端验证

**Step 1: 后端测试**

Run: `cd backend-go && go test ./internal/jobs/... -v`

**Step 2: 前端测试**

Run: `cd front && pnpm test:unit`
Run: `cd front && pnpm exec nuxi typecheck`

**Step 3: 全量编译**

Run: `cd backend-go && go build ./...`
Run: `cd front && pnpm build`
