# Topics界面重构实施计划

> **目标**: 重构topics界面，更新热点题材展示，移除站内动作和外部入口，将历史温度改为事件/人物/关键词分类界面，并实现AI增量分析存储。

---

## 需求总览

| 需求 | 状态 | 优先级 |
|------|------|--------|
| 热点题材更新为新的分类展示 | 待实现 | P0 |
| 移除站内动作栏目，添加返回首页按钮 | 待实现 | P0 |
| 移除外部入口 | 待实现 | P0 |
| 历史温度改为事件/人物/关键词分类 | 待实现 | P1 |
| AI增量分析存储 | 待实现 | P2 |
| 拓扑图选中效果联动 | 待实现 | P1 |

---

## 当前代码结构分析

### 关键文件位置

**Topics页面入口:**
- `front/app/pages/topics.vue` - 路由入口
- `front/app/features/topic-graph/components/TopicGraphPage.vue` - 主容器

**热点题材(HotTopics):**
- 实现在 `TopicGraphPage.vue` 内
- 数据来自 `viewModel.topTopics.slice(0, 6)`
- 按 event/person/keyword 分类样式

**站内动作和外部入口:**
- `front/app/features/topic-graph/components/TopicGraphFooterPanels.vue`
- 包含"站内动作"、"外部入口"、"历史温度"三个面板

**历史温度:**
- 同样在 `TopicGraphFooterPanels.vue`
- 读取 `detail.history` 展示时间线

**拓扑图:**
- `front/app/features/topic-graph/components/TopicGraphCanvas.client.vue`
- 使用 `3d-force-graph` 渲染

**API层:**
- `front/app/api/topicGraph.ts` - 所有topics相关API

---

## 实施阶段

### 阶段一: P0 - 基础UI改造

**任务1: 修改TopicGraphHeader添加返回首页按钮**

文件: `front/app/features/topic-graph/components/TopicGraphHeader.vue`

变更:
- 在"刷新图谱"按钮下方添加"返回首页"按钮
- 使用 `NuxtLink` 链接到 `/`
- 左侧放置，与刷新按钮形成层级

**任务2: 移除站内动作和外部入口面板**

文件: `front/app/features/topic-graph/components/TopicGraphFooterPanels.vue`

变更:
- 删除"站内动作"相关代码 (`detail.app_links` 渲染)
- 删除"外部入口"相关代码 (`detail.search_links` 渲染)
- 保留"历史温度"面板（后续重构）

**任务3: 更新热点题材展示**

文件: `front/app/features/topic-graph/components/TopicGraphPage.vue`

变更:
- 重新设计热点题材区域
- 改为按分类(事件/人物/关键词)分组展示
- 每个分类显示top标签
- 添加点击事件连接到分类分析面板

---

### 阶段二: P1 - 分类分析系统

**任务4: 创建新的数据模型(后端)**

文件: `backend-go/internal/domain/models/topic_tag_analysis.go` (新建)

定义:
```go
// 主题标签分析快照
type TopicTagAnalysis struct {
    ID            uint64    `gorm:"primaryKey"`
    TopicTagID    uint64    `gorm:"index"`
    AnalysisType  string    // event, person, keyword
    WindowType    string    // daily, weekly
    AnchorDate    time.Time `gorm:"index"`
    SummaryCount  int
    PayloadJSON   string    // 分析结果JSON
    Source        string    // ai, heuristic
    Version       int
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

// 分析游标(增量追踪)
type TopicAnalysisCursor struct {
    ID              uint64    `gorm:"primaryKey"`
    TopicTagID      uint64    `gorm:"uniqueIndex"`
    AnalysisType    string
    WindowType      string
    LastSummaryID   uint64
    UpdatedAt       time.Time
}
```

**任务5: 创建分析服务层(后端)**

文件: `backend-go/internal/domain/topicgraph/analysis_service.go` (新建)

功能:
- `GetOrCreateAnalysis(tagID, analysisType, windowType, date)`: 获取或创建分析
- `IncrementalUpdate(tagID, analysisType)`: 增量更新
- `AnalyzeWithAI(tagID, analysisType, summaries)`: AI分析
- `BuildTimeline(events)`: 构建时间线
- `BuildPersonProfile(person, events)`: 构建人物档案
- `BuildKeywordTrends(keyword, events)`: 构建关键词趋势

**任务6: 创建分析Handler(后端)**

文件: `backend-go/internal/domain/topicgraph/analysis_handler.go` (新建)

路由:
- `GET /api/topic-graph/analysis/:tagID/:analysisType`: 获取分析
- `POST /api/topic-graph/analysis/:tagID/:analysisType/rebuild`: 重建分析
- `GET /api/topic-graph/analysis/:tagID/timeline`: 获取时间线

**任务7: 前端创建分类Tabs组件**

文件: `front/app/features/topic-graph/components/TopicAnalysisTabs.vue` (新建)

功能:
- 三个Tab: 事件 | 人物 | 关键词
- 每个Tab显示对应分类的标签列表
- 点击标签触发分析展示
- 与TopicGraphPage状态同步

**任务8: 创建分析展示面板组件**

文件: `front/app/features/topic-graph/components/TopicAnalysisPanel.vue` (新建)

功能:
- 根据analysisType显示不同视图:
  - 事件: 时间线视图(垂直时间轴)
  - 人物: 人物档案卡片 + 相关新闻列表
  - 关键词: 趋势图 + 相关文章
- 支持展开/收起详情
- 提供"重新分析"按钮

**任务9: 修改TopicGraphFooterPanels整合新面板**

文件: `front/app/features/topic-graph/components/TopicGraphFooterPanels.vue`

变更:
- 删除旧的历史温度面板代码
- 引入TopicAnalysisTabs + TopicAnalysisPanel
- 调整布局适应新的分类界面

**任务10: 实现拓扑图与选中效果联动**

文件: `front/app/features/topic-graph/components/TopicGraphCanvas.client.vue`

变更:
- 扩展高亮逻辑支持分类关联高亮
- 当选择某分类(事件/人物/关键词)时:
  - 高亮该分类下的所有标签节点
  - 淡化其他节点
  - 高亮相关边
- 添加动画过渡效果

文件: `front/app/features/topic-graph/components/TopicGraphPage.vue`

变更:
- 统一选中状态管理:
  - `selectedCategory: 'event'|'person'|'keyword'|null`
  - `selectedTagSlug: string|null`
- 所有点击事件(热点标签/分类Tab/侧栏列表)统一调用 `selectTag(slug, category)`
- 选中状态同步到拓扑图高亮

---

### 阶段三: P2 - AI增量分析存储

**任务11: 实现增量分析队列(后端)**

文件: `backend-go/internal/domain/topicgraph/analysis_queue.go` (新建)

功能:
- `Enqueue(tagID, analysisType, priority)`: 入队
- `Dequeue()`: 出队
- `ProcessJob(job)`: 处理任务
- 支持优先级(手动触发 > 自动增量)

**任务12: 实现AI分析服务(后端)**

文件: `backend-go/internal/domain/topicgraph/ai_analysis.go` (新建)

功能:
- `AnalyzeTopicWithAI(tagID, summaries, analysisType)`: 调用AI分析
- `BuildPrompt(analysisType, summaries)`: 构建提示词
- `ParseAIResponse(response, analysisType)`: 解析AI返回
- 支持多种analysisType的不同提示词模板

**任务13: 集成到摘要生成后置钩子(后端)**

文件: `backend-go/internal/domain/summaries/summary_queue.go`

变更:
- 在 `TagSummary` 调用后追加 `EnqueueTopicAnalysis(summaryID)`
- 仅对新生成的、有关联标签的summary触发

文件: `backend-go/internal/jobs/auto_summary.go`

变更:
- 同样位置添加触发逻辑

**任务14: 前端实现分析状态展示**

文件: `front/app/features/topic-graph/components/TopicAnalysisPanel.vue`

变更:
- 添加分析状态显示:
  - `pending`: 显示"分析中..."进度条
  - `completed`: 显示分析结果
  - `failed`: 显示"分析失败" + 重试按钮
- 添加轮询机制检查分析状态

---

### 阶段四: P3 - 测试与文档

**任务15: 后端单元测试**

文件:
- `backend-go/internal/domain/topicgraph/analysis_service_test.go` (新建)
- `backend-go/internal/domain/topicgraph/analysis_handler_test.go` (新建)
- `backend-go/internal/domain/topicgraph/ai_analysis_test.go` (新建)

测试内容:
- 分析服务的增量更新逻辑
- AI分析提示词构建
- Handler的API响应格式
- 数据库存取和游标更新

**任务16: 前端单元测试**

文件:
- `front/app/features/topic-graph/components/TopicAnalysisTabs.test.ts` (新建)
- `front/app/features/topic-graph/components/TopicAnalysisPanel.test.ts` (新建)
- `front/app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts` (更新)

测试内容:
- Tabs组件切换逻辑
- Panel组件根据analysisType渲染不同视图
- ViewModel构建和高亮逻辑

**任务17: E2E测试更新**

文件: `front/tests/e2e/topic-graph.spec.ts`

更新内容:
- 删除对旧footer面板的断言
- 新增分类Tabs切换测试
- 新增选中高亮联动测试
- 新增分析面板展开/收起测试

**任务18: 文档更新**

文件:
- `docs/architecture/frontend.md` - 更新Topics页面架构说明
- `docs/architecture/backend-go.md` - 新增分析服务架构说明
- `docs/api/topic-graph.md` (新建) - API接口文档

文档内容:
- 新数据模型说明
- API接口详细说明
- 前端组件架构图
- AI分析流程说明

---

## 数据模型详细设计

### 1. topic_tag_analyses (分析快照表)

```go
type TopicTagAnalysis struct {
    ID            uint64    `gorm:"primaryKey"`
    TopicTagID    uint64    `gorm:"index:idx_tag_analysis_date,unique"`
    AnalysisType  string    `gorm:"index:idx_tag_analysis_date,unique"` // event, person, keyword
    WindowType    string    `gorm:"index:idx_tag_analysis_date,unique"` // daily, weekly
    AnchorDate    time.Time `gorm:"index:idx_tag_analysis_date,unique"`
    SummaryCount  int
    PayloadJSON   string    // 存储分析结果的JSON
    Source        string    // ai, heuristic, cached
    Version       int       // 分析版本号，用于增量更新
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

PayloadJSON结构示例:

```json
{
  "event": {
    "timeline": [
      {"date": "2024-01-01", "title": "事件A发生", "summary": "...", "sources": [...]},
      {"date": "2024-01-15", "title": "事件B进展", "summary": "...", "sources": [...]}
    ],
    "key_moments": [...],
    "related_entities": [...]
  },
  "person": {
    "profile": {"name": "...", "role": "...", "background": "..."},
    "appearances": [
      {"date": "2024-01-01", "context": "...", "quote": "...", "article_id": ...}
    ],
    "trend": [...]
  },
  "keyword": {
    "trend_data": [...],
    "related_topics": [...],
    "co_occurrence": [...],
    "context_examples": [...]
  }
}
```

### 2. topic_analysis_cursors (分析游标表)

```go
type TopicAnalysisCursor struct {
    ID              uint64    `gorm:"primaryKey"`
    TopicTagID      uint64    `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
    AnalysisType    string    `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
    WindowType      string    `gorm:"uniqueIndex:idx_cursor_tag_type_window"`
    LastSummaryID   uint64
    LastUpdatedAt   time.Time
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

---

## 实施优先级与依赖关系

```
P0 (基础UI):
├── 任务1: 添加返回首页按钮 [独立]
├── 任务2: 移除站内动作/外部入口 [独立]
└── 任务3: 更新热点题材展示 [依赖API]

P1 (核心功能):
├── 任务4-6: 后端API实现 [独立]
├── 任务7-9: 前端分类组件 [依赖API]
└── 任务10: 拓扑图联动 [依赖UI状态]

P2 (AI增量):
├── 任务11-13: 队列和AI分析 [依赖P1 API]
└── 任务14: 前端状态展示 [依赖后端]

P3 (测试文档):
├── 任务15-17: 测试 [依赖全部功能]
└── 任务18: 文档 [独立]
```

---

## 验收标准

### P0 验收

- [ ] Topics页面左侧显示"返回首页"按钮，点击跳转首页
- [ ] 页面不再显示"站内动作"和"外部入口"区域
- [ ] 热点题材区域按事件/人物/关键词三列展示
- [ ] 点击热点标签能打开详情面板

### P1 验收

- [ ] 后端 `/api/topic-graph/analysis/*` 接口正常工作
- [ ] 前端分类Tabs可以切换事件/人物/关键词
- [ ] 选择分类后拓扑图高亮相关节点
- [ ] 分析面板展示对应分类的内容

### P2 验收

- [ ] 新summary生成后自动触发分析任务
- [ ] 分析结果增量存储到数据库
- [ ] 前端显示分析状态(进行中/完成/失败)
- [ ] 可以手动触发重新分析

### P3 验收

- [ ] 单元测试覆盖率 > 60%
- [ ] E2E测试通过
- [ ] 文档更新完成
- [ ] 代码审查通过

---

## 风险与注意事项

### 技术风险

1. **AI分析性能**: AI分析可能较慢，需要设计好队列和缓存机制
2. **拓扑图性能**: 节点高亮逻辑复杂时可能影响渲染性能
3. **数据一致性**: 增量更新需要保证cursor和数据的幂等性

### 缓解措施

1. AI分析使用异步队列，前端显示进度
2. 拓扑图使用Web Worker或分批渲染
3. 数据库使用upsert和事务保证一致性

### 依赖项

- AI服务可用
- 数据库迁移权限
- 前端依赖版本兼容

---

*计划创建时间: 2026-03-14*
*版本: 1.0*
