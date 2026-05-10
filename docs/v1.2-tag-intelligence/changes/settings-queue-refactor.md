# 全局设置队列 Tab 重构 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将全局设置中的「后端队列」和「标签合并」两个 tab 替换为「标签打标队列」（展示 tag_jobs，带文章标题），同时保留 Embedding 队列面板。

**Architecture:** 后端新增 `tag_queue_handler.go` 查询 `tag_jobs` 表并 JOIN `articles` 获取文章标题；前端新增 `TagQueuePanel.vue` 展示；`GlobalSettingsDialog.vue` 改用新组件并加回 `EmbeddingQueuePanel`。

**Tech Stack:** Go/Gin, Vue 3 Composition API, TypeScript

---

## 当前进度

### ✅ 已完成
1. **后端 `tag_queue_handler.go`** — 已创建，查询 `tag_jobs` 表 + Preload Article 获取标题，3 个端点：`/api/tag-queue/status`、`/api/tag-queue/tasks`、`/api/tag-queue/retry`。`go build ./...` 通过。
2. **路由注册** — `router.go` 已添加 `topicextractiondomain.RegisterTagQueueRoutes(api)`，import 已加。`go build ./...` 通过。
3. **前端 API `tagQueue.ts`** — 已创建，`useTagQueueApi()` 含 getStatus/getTasks/retryFailed。
4. **前端 API index.ts** — 已导出 `useTagQueueApi` 和类型。
5. **`analysisQueue.ts`** — 已创建但现在不需要（应该删除），用 `tagQueue.ts` 代替。
6. **`GlobalSettingsDialog.vue`** — 已移除 `TagMergePreview`、`MergeReembeddingQueuePanel`、`EmbeddingQueuePanel` 的 import 和对应的 tab；已添加 `analysis-queue` tab。但引入的组件名不对（应为 `TagQueuePanel` 不是 `AnalysisQueuePanel`）。
7. **`AnalysisQueuePanel.vue`** — 已创建但现在需要改写为 `TagQueuePanel.vue`（展示 tag_jobs 数据，含文章标题）。
8. **旧的 `analysis_queue_handler.go`** — 已从 `topicanalysis` 包删除。

### ❌ 未完成（以下 Task）
- `analysisQueue.ts` 是残留文件，需删除
- `AnalysisQueuePanel.vue` 需要改写为 `TagQueuePanel.vue`
- `GlobalSettingsDialog.vue` 需要改 import 和加回 `EmbeddingQueuePanel`
- 前端 typecheck 验证

---

### Task 1: 清理残留文件

**Files:**
- Delete: `front/app/api/analysisQueue.ts`

**Step 1: 删除 analysisQueue.ts**

```bash
rm front/app/api/analysisQueue.ts
```

**Step 2: 从 index.ts 移除 analysisQueue 导出**

File: `front/app/api/index.ts`

确认没有 `analysisQueue` 相关的 export 行。当前状态应该是只有 `tagQueue` 的导出，没有 `analysisQueue`。

**Step 3: 删除旧的 AnalysisQueuePanel.vue**

```bash
rm front/app/features/topic-graph/components/AnalysisQueuePanel.vue
```

---

### Task 2: 创建 TagQueuePanel.vue

**Files:**
- Create: `front/app/features/topic-graph/components/TagQueuePanel.vue`
- Reference: `front/app/features/ai/components/EmbeddingQueuePanel.vue`（现有风格参考）

**Step 1: 创建 TagQueuePanel.vue**

关键数据字段（来自 `useTagQueueApi`）：
- `article_title` — 文章标题（重点展示）
- `feed_name_snapshot` — 来源 feed
- `category_name_snapshot` — 分类
- `status` — pending / leased / completed / failed
- `attempt_count` / `max_attempts` — 重试情况
- `last_error` — 错误信息
- `created_at` — 创建时间

组件结构参照 `EmbeddingQueuePanel.vue` 的风格：
- 紫色渐变 header 图标
- 4 格状态卡片（待处理/打标中/已完成/失败）
- 进度条
- 筛选按钮
- 表格：文章标题 | 来源 | 分类 | 状态 | 重试 | 创建时间 | 错误
- 分页
- 5 秒自动刷新 status

**Step 2: 验证 import 能正常解析**

确认 `useTagQueueApi` 从 `~/api` 可导入。

---

### Task 3: 更新 GlobalSettingsDialog.vue

**Files:**
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue`

**Step 1: 更新 import**

将：
```ts
import AnalysisQueuePanel from '~/features/topic-graph/components/AnalysisQueuePanel.vue'
```
改为：
```ts
import EmbeddingQueuePanel from '~/features/ai/components/EmbeddingQueuePanel.vue'
import TagQueuePanel from '~/features/topic-graph/components/TagQueuePanel.vue'
```

**Step 2: 更新 tab 类型**

将 `activeTab` 类型中的 `'analysis-queue'` 改为 `'queues'`（更通用的名字，因为里面放两个面板）。

**Step 3: 更新 tab 按钮文字**

将「标签分析队列」改为「标签 & 队列」。

**Step 4: 更新 tab 内容区域**

将：
```html
<div v-if="activeTab === 'analysis-queue'" class="space-y-6">
  <AnalysisQueuePanel />
</div>
```
改为：
```html
<div v-if="activeTab === 'queues'" class="space-y-6">
  <EmbeddingQueuePanel />
  <TagQueuePanel />
</div>
```

---

### Task 4: 验证

**Step 1: 后端编译**

```bash
cd backend-go && go build ./...
```
Expected: 无错误

**Step 2: 前端 typecheck**

```bash
cd front && pnpm exec nuxi typecheck
```
Expected: 无错误

**Step 3: 提交**

```bash
git add -A
git commit -m "refactor: replace backend-queues & tag-merge tabs with tag queue + embedding panel"
```
