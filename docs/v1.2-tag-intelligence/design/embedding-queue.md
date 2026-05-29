# Embedding Queue Visualization Design

## Overview

为embedding生成过程添加实时队列可视化功能，让用户能够追踪后台embedding任务的处理进度。

## Problem Statement

当前embedding生成是fire-and-forget的异步goroutine，用户无法知道：
- 有多少embedding任务在排队等待
- 当前处理状态如何
- 有多少失败的任务

## Solution

新建embedding专用队列系统，包含数据库表、后端API、前端面板。

## Database Design

### embedding_queue 表

```sql
CREATE TABLE embedding_queue (
    id BIGSERIAL PRIMARY KEY,
    tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending/processing/completed/failed
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX idx_embedding_queue_status ON embedding_queue(status);
CREATE INDEX idx_embedding_queue_tag_id ON embedding_queue(tag_id);
```

### 工作流程

1. 新tag需要embedding时，插入`embedding_queue`记录（status=pending）
2. 后台worker定时轮询pending任务，更新status=processing
3. 处理完成后更新status=completed或failed
4. 前端面板查询队列表展示进度

## Backend API Design

### Endpoints

#### 1. GET /api/embedding/queue/status
获取队列状态统计

**Response:**
```json
{
  "success": true,
  "data": {
    "pending": 5,
    "processing": 2,
    "completed": 150,
    "failed": 3,
    "total": 160
  }
}
```

#### 2. GET /api/embedding/queue/tasks
获取任务列表

**Query Parameters:**
- `status` (optional): 筛选状态
- `limit` (optional): 默认50
- `offset` (optional): 分页

**Response:**
```json
{
  "success": true,
  "data": {
    "tasks": [
      {
        "id": 1,
        "tag_id": 123,
        "tag_name": "AI",
        "tag_category": "topic",
        "status": "pending",
        "error_message": null,
        "created_at": "2026-04-13T10:00:00Z",
        "started_at": null,
        "completed_at": null
      }
    ],
    "total": 160
  }
}
```

#### 3. POST /api/embedding/queue/retry
重试所有失败的任务

**Response:**
```json
{
  "success": true,
  "message": "已重试 3 个失败任务"
}
```

### 修改现有逻辑

- `tagger.go` 中 `generateAndSaveEmbedding()` 改为创建队列记录
- `tagger.go` 中 `ensureTagEmbedding()` 改为创建队列记录
- 新建 `embedding_queue_worker.go` 处理pending任务

## Frontend Design

### EmbeddingQueuePanel.vue

**位置:** AI设置页面新增「Embedding队列」标签页

**组件结构:**
```
EmbeddingQueuePanel
├── StatsRow (4个统计卡片)
│   ├── PendingCard (待处理)
│   ├── ProcessingCard (处理中)
│   ├── CompletedCard (已完成)
│   └── FailedCard (失败)
├── ProgressBar (总体进度)
├── TaskTable (任务列表)
│   ├── 状态筛选下拉框
│   └── 任务列表表格
│       ├── Tag名称
│       ├── 状态 (带颜色标签)
│       ├── 创建时间
│       ├── 完成时间
│       └── 错误信息
└── Actions
    └── RetryFailedButton
```

**刷新策略:** 每5秒轮询 `/api/embedding/queue/status`

### API Client

新建 `front/app/api/embeddingQueue.ts`:
- `useEmbeddingQueueApi()`
- `fetchQueueStatus()`
- `fetchQueueTasks(params)`
- `retryFailedTasks()`

## Implementation Order

1. **Backend - 数据库**
   - 创建migration脚本
   - 创建GORM模型

2. **Backend - 队列逻辑**
   - 创建embedding_queue.go (CRUD操作)
   - 修改tagger.go (创建队列记录)
   - 创建embedding_queue_worker.go (worker处理)
   - 注册应用启动时启动worker

3. **Backend - API**
   - 创建embedding_queue_handler.go
   - 注册路由

4. **Frontend - API**
   - 创建embeddingQueue.ts

5. **Frontend - 组件**
   - 创建EmbeddingQueuePanel.vue
   - 集成到AI设置页面

## Success Criteria

- [ ] 新tag创建时自动插入embedding_queue记录
- [ ] worker正确处理pending任务
- [ ] API返回准确的队列状态统计
- [ ] 前端面板实时显示队列进度
- [ ] 可以重试失败的任务
- [ ] 不影响现有tag匹配和topic analysis功能

## Risks

- **Worker并发问题**: 需要确保同一任务不会被多个worker同时处理（使用SELECT FOR UPDATE SKIP LOCKED）
- **队列堆积**: 如果embedding服务不可用，队列会堆积。需要考虑限流机制
- **磁盘空间**: embedding_queue表会持续增长。可考虑定期清理completed记录

## Future Enhancements

- WebSocket推送实时状态更新
- 手动触发批量embedding生成
- embedding质量评估和可视化
