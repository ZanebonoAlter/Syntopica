# Embedding Queue Visualization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为embedding生成添加实时队列可视化，让用户能够追踪后台embedding任务的处理进度。

**Architecture:** 新建embedding专用队列系统，包含数据库表、后端API、前端面板。修改现有embedding生成逻辑为创建队列记录，后台worker异步处理。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, Vue 3, TypeScript, Tailwind CSS

---

## Task 1: 创建数据库迁移

**Files:**
- Create: `backend-go/cmd/migrate-embedding-queue/main.go`

**Step 1: 写迁移脚本**

```go
package main

import (
	"fmt"
	"log"
	"my-robot-backend/internal/platform/database"
)

func main() {
	database.InitDB()

	db := database.DB

	// 创建 embedding_queue 表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS embedding_queue (
		id BIGSERIAL PRIMARY KEY,
		tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		error_message TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		started_at TIMESTAMP,
		completed_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_embedding_queue_status ON embedding_queue(status);
	CREATE INDEX IF NOT EXISTS idx_embedding_queue_tag_id ON embedding_queue(tag_id);
	`

	if err := db.Exec(createTableSQL).Error; err != nil {
		log.Fatalf("Failed to create embedding_queue table: %v", err)
	}

	fmt.Println("✓ embedding_queue table created successfully")
}
```

**Step 2: 运行迁移验证**

```bash
cd backend-go && go run cmd/migrate-embedding-queue/main.go
```

Expected: `✓ embedding_queue table created successfully`

**Step 3: Commit**

```bash
git add backend-go/cmd/migrate-embedding-queue/
git commit -m "feat: add embedding queue migration script"
```

---

## Task 2: 创建EmbeddingQueue模型和队列服务

**Files:**
- Create: `backend-go/internal/domain/models/embedding_queue.go`
- Create: `backend-go/internal/domain/topicanalysis/embedding_queue.go`

**Step 1: 创建GORM模型**

```go
// backend-go/internal/domain/models/embedding_queue.go
package models

import "time"

const (
	EmbeddingQueueStatusPending    = "pending"
	EmbeddingQueueStatusProcessing = "processing"
	EmbeddingQueueStatusCompleted  = "completed"
	EmbeddingQueueStatusFailed     = "failed"
)

type EmbeddingQueue struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	TagID        uint       `gorm:"not null;index" json:"tag_id"`
	Status       string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`

	Tag *TopicTag `gorm:"foreignKey:TagID" json:"tag,omitempty"`
}

func (EmbeddingQueue) TableName() string {
	return "embedding_queue"
}
```

**Step 2: 创建队列服务**

```go
// backend-go/internal/domain/topicanalysis/embedding_queue.go
package topicanalysis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

type EmbeddingQueueService struct {
	db       *gorm.DB
	es       *EmbeddingService
	logger   *zap.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
}

func NewEmbeddingQueueService(db *gorm.DB, logger *zap.Logger) *EmbeddingQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EmbeddingQueueService{
		db:     db,
		es:     NewEmbeddingService(),
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Enqueue 创建一个新的embedding队列任务
func (s *EmbeddingQueueService) Enqueue(tagID uint) error {
	// 检查是否已有pending/processing的任务
	var count int64
	s.db.Model(&models.EmbeddingQueue{}).
		Where("tag_id = ? AND status IN ?", tagID, []string{models.EmbeddingQueueStatusPending, models.EmbeddingQueueStatusProcessing}).
		Count(&count)
	if count > 0 {
		return nil // 已有任务在队列中
	}

	// 检查是否已有embedding
	var embCount int64
	s.db.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ?", tagID).Count(&embCount)
	if embCount > 0 {
		return nil // 已有embedding
	}

	task := &models.EmbeddingQueue{
		TagID:  tagID,
		Status: models.EmbeddingQueueStatusPending,
	}
	return s.db.Create(task).Error
}

// GetStatus 获取队列状态统计
func (s *EmbeddingQueueService) GetStatus() (map[string]int64, error) {
	status := map[string]int64{
		"pending":    0,
		"processing": 0,
		"completed":  0,
		"failed":     0,
		"total":      0,
	}

	var results []struct {
		Status string
		Count  int64
	}

	s.db.Model(&models.EmbeddingQueue{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&results)

	for _, r := range results {
		status[r.Status] = r.Count
		status["total"] += r.Count
	}

	return status, nil
}

// GetTasks 获取任务列表
func (s *EmbeddingQueueService) GetTasks(status string, limit, offset int) ([]models.EmbeddingQueue, int64, error) {
	query := s.db.Model(&models.EmbeddingQueue{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var tasks []models.EmbeddingQueue
	err := query.Preload("Tag").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&tasks).Error

	return tasks, total, err
}

// RetryFailed 重试所有失败的任务
func (s *EmbeddingQueueService) RetryFailed() (int64, error) {
	result := s.db.Model(&models.EmbeddingQueue{}).
		Where("status = ?", models.EmbeddingQueueStatusFailed).
		Updates(map[string]interface{}{
			"status":        models.EmbeddingQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
		})
	return result.RowsAffected, result.Error
}

// Start 启动worker
func (s *EmbeddingQueueService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	s.running = true
	s.stopCh = make(chan struct{})
	s.wg.Add(1)

	go s.worker()
	s.logger.Info("embedding queue worker started")
}

// Stop 停止worker
func (s *EmbeddingQueueService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopCh)
	s.wg.Wait()
	s.running = false
	s.logger.Info("embedding queue worker stopped")
}

func (s *EmbeddingQueueService) worker() {
	defer s.wg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processNext()
		}
	}
}

func (s *EmbeddingQueueService) processNext() {
	var task models.EmbeddingQueue

	// 使用事务获取并锁定任务
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = ?", models.EmbeddingQueueStatusPending).
			Order("created_at ASC").
			First(&task).Error; err != nil {
			return err
		}

		now := time.Now()
		task.Status = models.EmbeddingQueueStatusProcessing
		task.StartedAt = &now
		return tx.Save(&task).Error
	})

	if err != nil {
		return // 没有待处理任务或出错
	}

	// 加载tag
	var tag models.TopicTag
	if err := s.db.First(&tag, task.TagID).Error; err != nil {
		s.updateTaskFailed(&task, fmt.Sprintf("failed to load tag: %v", err))
		return
	}

	// 生成embedding
	embedding, err := s.es.GenerateEmbedding(context.Background(), &tag)
	if err != nil {
		s.updateTaskFailed(&task, fmt.Sprintf("failed to generate embedding: %v", err))
		return
	}

	// 保存embedding
	if err := s.es.SaveEmbedding(embedding); err != nil {
		s.updateTaskFailed(&task, fmt.Sprintf("failed to save embedding: %v", err))
		return
	}

	// 更新任务状态为完成
	now := time.Now()
	task.Status = models.EmbeddingQueueStatusCompleted
	task.CompletedAt = &now
	task.ErrorMessage = ""
	if err := s.db.Save(&task).Error; err != nil {
		s.logger.Error("failed to update completed task", zap.Error(err))
	}

	s.logger.Info("embedding task completed", zap.Uint("tag_id", task.TagID))
}

func (s *EmbeddingQueueService) updateTaskFailed(task *models.EmbeddingQueue, errMsg string) {
	now := time.Now()
	task.Status = models.EmbeddingQueueStatusFailed
	task.CompletedAt = &now
	task.ErrorMessage = errMsg
	if err := s.db.Save(task).Error; err != nil {
		s.logger.Error("failed to update failed task", zap.Error(err))
	}
	s.logger.Warn("embedding task failed", zap.Uint("tag_id", task.TagID), zap.String("error", errMsg))
}
```

**Step 3: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/models/embedding_queue.go backend-go/internal/domain/topicanalysis/embedding_queue.go
git commit -m "feat: add embedding queue model and service"
```

---

## Task 3: 修改tagger.go使用队列

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`

**Step 1: 添加队列服务单例**

在文件顶部添加（约第20行后）：

```go
var (
	embeddingService     *topicanalysis.EmbeddingService
	embeddingServiceOnce sync.Once
	embeddingQueueService     *topicanalysis.EmbeddingQueueService
	embeddingQueueServiceOnce sync.Once
)

func getEmbeddingQueueService() *topicanalysis.EmbeddingQueueService {
	embeddingQueueServiceOnce.Do(func() {
		embeddingQueueService = topicanalysis.NewEmbeddingQueueService(database.DB, nil)
	})
	return embeddingQueueService
}
```

**Step 2: 修改 generateAndSaveEmbedding 函数 (约第242行)**

将：
```go
func generateAndSaveEmbedding(es *topicanalysis.EmbeddingService, tag *models.TopicTag) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[WARN] Embedding generation panicked for tag %d: %v\n", tag.ID, r)
		}
	}()

	embedding, err := es.GenerateEmbedding(context.Background(), tag)
	if err != nil {
		fmt.Printf("[WARN] Failed to generate embedding for tag %d: %v\n", tag.ID, err)
		return
	}
	if err := es.SaveEmbedding(embedding); err != nil {
		fmt.Printf("[WARN] Failed to save embedding for tag %d: %v\n", tag.ID, err)
	}
}
```

改为：
```go
func generateAndSaveEmbedding(es *topicanalysis.EmbeddingService, tag *models.TopicTag) {
	qs := getEmbeddingQueueService()
	if err := qs.Enqueue(tag.ID); err != nil {
		fmt.Printf("[WARN] Failed to enqueue embedding for tag %d: %v\n", tag.ID, err)
	}
}
```

**Step 3: 修改 ensureTagEmbedding 函数 (约第261行)**

将：
```go
func ensureTagEmbedding(es *topicanalysis.EmbeddingService, tagID uint) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[WARN] Embedding backfill panicked for tag %d: %v\n", tagID, r)
		}
	}()

	// Check if embedding already exists
	var count int64
	database.DB.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ?", tagID).Count(&count)
	if count > 0 {
		return // Already has embedding
	}

	// Load the tag
	var tag models.TopicTag
	if err := database.DB.First(&tag, tagID).Error; err != nil {
		return
	}

	embedding, err := es.GenerateEmbedding(context.Background(), &tag)
	if err != nil {
		fmt.Printf("[WARN] Failed to backfill embedding for tag %d: %v\n", tagID, err)
		return
	}
	if err := es.SaveEmbedding(embedding); err != nil {
		fmt.Printf("[WARN] Failed to save backfilled embedding for tag %d: %v\n", tagID, err)
	}
}
```

改为：
```go
func ensureTagEmbedding(es *topicanalysis.EmbeddingService, tagID uint) {
	qs := getEmbeddingQueueService()
	if err := qs.Enqueue(tagID); err != nil {
		fmt.Printf("[WARN] Failed to enqueue embedding for tag %d: %v\n", tagID, err)
	}
}
```

**Step 4: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "refactor: use embedding queue service for tagger"
```

---

## Task 4: 创建API Handler

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/embedding_queue_handler.go`

**Step 1: 创建Handler**

```go
package topicanalysis

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/platform/database"
)

var queueService *EmbeddingQueueService

func init() {
	queueService = NewEmbeddingQueueService(database.DB, nil)
}

func GetEmbeddingQueueStatus(c *gin.Context) {
	status, err := queueService.GetStatus()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": status})
}

func GetEmbeddingQueueTasks(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	tasks, total, err := queueService.GetTasks(status, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"tasks": tasks,
			"total": total,
		},
	})
}

func RetryEmbeddingQueueFailed(c *gin.Context) {
	count, err := queueService.RetryFailed()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "已重试 " + strconv.FormatInt(count, 10) + " 个失败任务",
	})
}

func RegisterEmbeddingQueueRoutes(rg *gin.RouterGroup) {
	queue := rg.Group("/embedding/queue")
	{
		queue.GET("/status", GetEmbeddingQueueStatus)
		queue.GET("/tasks", GetEmbeddingQueueTasks)
		queue.POST("/retry", RetryEmbeddingQueueFailed)
	}
}

func StartEmbeddingQueueWorker() {
	queueService.Start()
}

func StopEmbeddingQueueWorker() {
	queueService.Stop()
}
```

**Step 2: 注册路由和启动worker**

修改 `backend-go/internal/app/router.go`，在 `SetupRoutes` 函数中添加：

约第164行后（在 `topicanalysisdomain.RegisterEmbeddingConfigRoutes(api)` 之后）：
```go
		topicanalysisdomain.RegisterEmbeddingConfigRoutes(api)
		topicanalysisdomain.RegisterEmbeddingQueueRoutes(api)
```

修改 `backend-go/cmd/server/main.go`，在启动服务前添加worker启动（查看现有代码确定位置）。

**Step 3: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/embedding_queue_handler.go backend-go/internal/app/router.go backend-go/cmd/server/main.go
git commit -m "feat: add embedding queue API endpoints"
```

---

## Task 5: 创建前端API客户端

**Files:**
- Create: `front/app/api/embeddingQueue.ts`

**Step 1: 创建API客户端**

```typescript
import { useApiClient } from './client'

export interface EmbeddingQueueStatus {
  pending: number
  processing: number
  completed: number
  failed: number
  total: number
}

export interface EmbeddingQueueTask {
  id: number
  tag_id: number
  status: 'pending' | 'processing' | 'completed' | 'failed'
  error_message: string | null
  created_at: string
  started_at: string | null
  completed_at: string | null
  tag?: {
    id: number
    label: string
    category: string
    slug: string
  }
}

export interface EmbeddingQueueTasksResponse {
  tasks: EmbeddingQueueTask[]
  total: number
}

export function useEmbeddingQueueApi() {
  const api = useApiClient()

  return {
    getStatus: () =>
      api.get<EmbeddingQueueStatus>('/embedding/queue/status'),

    getTasks: (params?: { status?: string; limit?: number; offset?: number }) =>
      api.get<EmbeddingQueueTasksResponse>('/embedding/queue/tasks', { params }),

    retryFailed: () =>
      api.post<{ message: string }>('/embedding/queue/retry'),
  }
}
```

**Step 2: 确保导出**

检查 `front/app/api/index.ts`，确保有导出：
```typescript
export * from './embeddingQueue'
```

**Step 3: 验证类型检查**

```bash
cd front && pnpm exec nuxi typecheck
```

**Step 4: Commit**

```bash
git add front/app/api/embeddingQueue.ts front/app/api/index.ts
git commit -m "feat: add embedding queue API client"
```

---

## Task 6: 创建EmbeddingQueuePanel组件

**Files:**
- Create: `front/app/features/ai/components/EmbeddingQueuePanel.vue`

**Step 1: 创建组件**

```vue
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useEmbeddingQueueApi, type EmbeddingQueueStatus, type EmbeddingQueueTask } from '~/api'

const loading = ref(false)
const error = ref<string | null>(null)
const status = ref<EmbeddingQueueStatus>({
  pending: 0,
  processing: 0,
  completed: 0,
  failed: 0,
  total: 0,
})
const tasks = ref<EmbeddingQueueTask[]>([])
const totalTasks = ref(0)
const statusFilter = ref('')
const currentPage = ref(1)
const pageSize = 20
const retrying = ref(false)

let refreshTimer: ReturnType<typeof setInterval> | null = null

const api = useEmbeddingQueueApi()

async function loadStatus() {
  try {
    const response = await api.getStatus()
    if (response.success && response.data) {
      status.value = response.data
    }
  } catch (err) {
    console.error('Failed to load queue status:', err)
  }
}

async function loadTasks() {
  loading.value = true
  error.value = null
  try {
    const response = await api.getTasks({
      status: statusFilter.value || undefined,
      limit: pageSize,
      offset: (currentPage.value - 1) * pageSize,
    })
    if (response.success && response.data) {
      tasks.value = response.data.tasks
      totalTasks.value = response.data.total
    } else {
      throw new Error(response.error || '加载任务列表失败')
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function retryFailed() {
  retrying.value = true
  try {
    const response = await api.retryFailed()
    if (response.success) {
      await Promise.all([loadStatus(), loadTasks()])
    }
  } catch (err) {
    console.error('Failed to retry:', err)
  } finally {
    retrying.value = false
  }
}

function getStatusColor(s: string) {
  switch (s) {
    case 'pending': return 'bg-yellow-100 text-yellow-800'
    case 'processing': return 'bg-blue-100 text-blue-800'
    case 'completed': return 'bg-green-100 text-green-800'
    case 'failed': return 'bg-red-100 text-red-800'
    default: return 'bg-gray-100 text-gray-800'
  }
}

function getStatusLabel(s: string) {
  switch (s) {
    case 'pending': return '待处理'
    case 'processing': return '处理中'
    case 'completed': return '已完成'
    case 'failed': return '失败'
    default: return s
  }
}

function formatDate(dateStr: string | null) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

const progressPercent = computed(() => {
  if (status.value.total === 0) return 0
  return Math.round((status.value.completed / status.value.total) * 100)
})

const totalPages = computed(() => Math.ceil(totalTasks.value / pageSize))

function changePage(page: number) {
  currentPage.value = page
  loadTasks()
}

function changeFilter(value: string) {
  statusFilter.value = value
  currentPage.value = 1
  loadTasks()
}

async function refreshAll() {
  await Promise.all([loadStatus(), loadTasks()])
}

onMounted(async () => {
  await refreshAll()
  refreshTimer = setInterval(loadStatus, 5000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-purple-500 to-purple-700 flex items-center justify-center">
          <Icon icon="mdi:playlist-check" width="20" height="20" class="text-white" />
        </div>
        <div>
          <h3 class="font-semibold text-gray-900">Embedding 队列</h3>
          <p class="text-xs text-gray-500">实时追踪embedding生成进度</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 transition-colors"
          @click="refreshAll"
        >
          <Icon icon="mdi:refresh" width="16" height="16" />
        </button>
        <button
          v-if="status.failed > 0"
          class="px-4 py-2 text-sm font-medium text-white bg-orange-600 rounded-lg hover:bg-orange-700 transition-colors disabled:opacity-50"
          :disabled="retrying"
          @click="retryFailed"
        >
          {{ retrying ? '重试中...' : `重试失败 (${status.failed})` }}
        </button>
      </div>
    </div>

    <!-- Status Cards -->
    <div class="grid grid-cols-4 gap-3">
      <div class="rounded-lg border border-gray-200 p-3 bg-yellow-50">
        <div class="text-2xl font-bold text-yellow-700">{{ status.pending }}</div>
        <div class="text-xs text-yellow-600">待处理</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-blue-50">
        <div class="text-2xl font-bold text-blue-700">{{ status.processing }}</div>
        <div class="text-xs text-blue-600">处理中</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-green-50">
        <div class="text-2xl font-bold text-green-700">{{ status.completed }}</div>
        <div class="text-xs text-green-600">已完成</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-red-50">
        <div class="text-2xl font-bold text-red-700">{{ status.failed }}</div>
        <div class="text-xs text-red-600">失败</div>
      </div>
    </div>

    <!-- Progress Bar -->
    <div v-if="status.total > 0" class="space-y-1">
      <div class="flex justify-between text-xs text-gray-500">
        <span>总体进度</span>
        <span>{{ progressPercent }}% ({{ status.completed }}/{{ status.total }})</span>
      </div>
      <div class="h-2 bg-gray-200 rounded-full overflow-hidden">
        <div
          class="h-full bg-gradient-to-r from-purple-500 to-purple-600 transition-all duration-300"
          :style="{ width: `${progressPercent}%` }"
        />
      </div>
    </div>

    <!-- Filter -->
    <div class="flex items-center gap-2">
      <span class="text-sm text-gray-500">筛选:</span>
      <button
        v-for="s in ['', 'pending', 'processing', 'completed', 'failed']"
        :key="s"
        class="px-3 py-1 text-xs rounded-full transition-colors"
        :class="statusFilter === s ? 'bg-purple-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'"
        @click="changeFilter(s)"
      >
        {{ s === '' ? '全部' : getStatusLabel(s) }}
      </button>
    </div>

    <!-- Tasks Table -->
    <div v-if="loading" class="py-8 flex justify-center">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin text-purple-600" />
    </div>

    <div v-else-if="error" class="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
      {{ error }}
    </div>

    <div v-else-if="tasks.length === 0" class="py-8 text-center text-gray-500">
      暂无任务
    </div>

    <div v-else class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-200">
            <th class="text-left py-2 px-3 font-medium text-gray-600">标签</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">状态</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">创建时间</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">完成时间</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">错误信息</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="border-b border-gray-100 hover:bg-gray-50">
            <td class="py-2 px-3">
              <span v-if="task.tag">{{ task.tag.label }}</span>
              <span v-else class="text-gray-400">Tag #{{ task.tag_id }}</span>
            </td>
            <td class="py-2 px-3">
              <span
                class="px-2 py-0.5 text-xs rounded-full"
                :class="getStatusColor(task.status)"
              >
                {{ getStatusLabel(task.status) }}
              </span>
            </td>
            <td class="py-2 px-3 text-gray-500">{{ formatDate(task.created_at) }}</td>
            <td class="py-2 px-3 text-gray-500">{{ formatDate(task.completed_at) }}</td>
            <td class="py-2 px-3 text-red-600 text-xs max-w-xs truncate">
              {{ task.error_message || '-' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div v-if="totalPages > 1" class="flex items-center justify-between">
      <div class="text-sm text-gray-500">
        共 {{ totalTasks }} 条任务
      </div>
      <div class="flex items-center gap-1">
        <button
          class="px-3 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage <= 1"
          @click="changePage(currentPage - 1)"
        >
          上一页
        </button>
        <span class="px-3 py-1 text-sm">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          class="px-3 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage >= totalPages"
          @click="changePage(currentPage + 1)"
        >
          下一页
        </button>
      </div>
    </div>
  </div>
</template>
```

**Step 2: 验证类型检查**

```bash
cd front && pnpm exec nuxi typecheck
```

**Step 3: Commit**

```bash
git add front/app/features/ai/components/EmbeddingQueuePanel.vue
git commit -m "feat: add EmbeddingQueuePanel component"
```

---

## Task 7: 集成到AI设置页面

**Files:**
- Modify: `front/app/pages/ai.vue` 或相关的AI设置页面

**Step 1: 查找AI设置页面**

找到现有AI设置页面，可能是：
- `front/app/pages/ai.vue`
- `front/app/pages/settings.vue`
- `front/app/pages/settings/ai.vue`

**Step 2: 添加Embedding队列标签页**

在AI设置页面的tabs中添加新标签：

```vue
<!-- 在现有的tabs列表中添加 -->
{ id: 'embedding-queue', label: 'Embedding 队列', icon: 'mdi:playlist-check' }
```

并在对应的标签内容区域添加：

```vue
<div v-if="activeTab === 'embedding-queue'">
  <EmbeddingQueuePanel />
</div>
```

**Step 3: 验证类型检查和构建**

```bash
cd front && pnpm exec nuxi typecheck && pnpm build
```

**Step 4: Commit**

```bash
git add front/app/pages/ai.vue
git commit -m "feat: integrate embedding queue panel into AI settings"
```

---

## Verification

运行完整验证：

```bash
# Backend
cd backend-go && go test ./... && go build ./...

# Frontend
cd front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | 数据库迁移 | `cmd/migrate-embedding-queue/main.go` |
| 2 | 模型和服务 | `models/embedding_queue.go`, `topicanalysis/embedding_queue.go` |
| 3 | 修改tagger | `topicextraction/tagger.go` |
| 4 | API Handler | `topicanalysis/embedding_queue_handler.go`, `router.go`, `main.go` |
| 5 | 前端API | `api/embeddingQueue.ts` |
| 6 | 组件 | `ai/components/EmbeddingQueuePanel.vue` |
| 7 | 集成页面 | `pages/ai.vue` |
