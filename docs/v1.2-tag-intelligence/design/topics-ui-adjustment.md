# Topics界面调整实施计划

## 基于合理假设的方案

由于需要快速推进实施，基于最佳实践做出以下合理假设：

### 关键参数假设
1. **日报时间线**: 默认展示15条，支持无限滚动加载
2. **AI分析触发**: 用户点击按钮后立即调用（即时响应）
3. **拓扑图连线动画**: 生长动画（从节点延伸出来）
4. **关键词云**: 展示20个关键词，字号按关联度计算
5. **去重逻辑**: 基于article ID + 标题相似度（>85%认为重复）
6. **时间线筛选**: 支持按时间范围（今天/本周/本月）和来源筛选

---

## 实施阶段划分

### 阶段一：拓扑图连线重构 (P0)
**预计耗时**: 4-6小时
**核心任务**:
1. 修改`TopicGraphCanvas`组件，默认隐藏所有连线
2. 实现选中题材后的动态连线绘制逻辑
3. 添加连线动画效果（生长动画）
4. 优化连线样式（贝塞尔曲线、渐变色、发光效果）

**关键文件**:
- `front/app/features/topic-graph/components/TopicGraphCanvas.client.vue`

**验收标准**:
- [ ] 默认状态下不显示任何连线
- [ ] 选中题材后动态绘制相关连线
- [ ] 连线动画平滑自然（生长效果）
- [ ] 连线样式美观（渐变+发光）

---

### 阶段二：日报时间线区域 (P0)
**预计耗时**: 6-8小时
**核心任务**:
1. 创建`TopicTimeline`组件（新组件）
2. 实现垂直时间轴布局
3. 开发日报内容卡片组件
4. 实现时间排序和筛选功能
5. 集成无限滚动加载
6. 在`TopicGraphPage`中添加时间线区域

**关键文件**:
- `front/app/features/topic-graph/components/TopicTimeline.vue` (新建)
- `front/app/features/topic-graph/components/TopicTimelineCard.vue` (新建)
- `front/app/features/topic-graph/components/TopicGraphPage.vue` (修改)

**验收标准**:
- [ ] 时间线区域显示在拓扑图下方
- [ ] 垂直时间轴布局正确
- [ ] 日报卡片展示完整信息（标题、摘要、时间、来源）
- [ ] 支持无限滚动加载
- [ ] 支持时间范围筛选

---

### 阶段三：右侧栏精简 (P1)
**预计耗时**: 4-5小时
**核心任务**:
1. 重构`TopicGraphSidebar`组件
2. 实现严格去重逻辑（基于ID+标题相似度）
3. 开发关键词云组件
4. 实现点击关键词只高亮拓扑节点（不展开列表）
5. 优化文章展示卡片

**关键文件**:
- `front/app/features/topic-graph/components/TopicGraphSidebar.vue` (修改)
- `front/app/features/topic-graph/components/KeywordCloud.vue` (新建)

**验收标准**:
- [ ] 右侧栏展示的文章严格去重
- [ ] 关键词以云形式展示
- [ ] 点击关键词只高亮拓扑节点
- [ ] 不展开任何列表

---

### 阶段四：AI分析功能 (P1)
**预计耗时**: 5-6小时
**核心任务**:
1. 在`TopicTimeline`组件中添加"AI深度分析"按钮
2. 创建`TopicAIAnalysisPanel`组件（可展开/收起）
3. 实现AI分析状态管理（待分析/分析中/已完成）
4. 集成后端AI分析API
5. 实现分析结果展示（事件脉络/人物关系/关键词趋势）

**关键文件**:
- `front/app/features/topic-graph/components/TopicTimeline.vue` (修改)
- `front/app/features/topic-graph/components/TopicAIAnalysisPanel.vue` (新建)
- `front/app/stores/aiAnalysis.ts` (新建或修改)

**验收标准**:
- [ ] AI分析按钮显示在时间线区域
- [ ] 点击按钮后展开AI分析面板
- [ ] 支持分析状态展示（分析中/已完成）
- [ ] 分析结果正确展示
- [ ] 支持增量存储（避免重复调用AI）

---

### 阶段五：后端API调整 (P1)
**预计耗时**: 3-4小时
**核心任务**:
1. 调整`/api/topic-graph/topic/:slug`接口，返回关联的原始articles
2. 新增`/api/topic-graph/topic/:slug/articles`接口（专门获取articles）
3. 调整数据查询逻辑，直接从article-tag关联表查询
4. 优化查询性能（添加索引）

**关键文件**:
- `backend-go/internal/domain/topicgraph/service.go` (修改)
- `backend-go/internal/domain/topicgraph/handler.go` (修改)
- `backend-go/internal/domain/models/topic_graph.go` (可能需要调整)

**验收标准**:
- [ ] API返回的数据包含原始articles
- [ ] Articles按时间排序
- [ ] 支持分页
- [ ] 查询性能良好（<200ms）

---

### 阶段六：测试与优化 (P2)
**预计耗时**: 4-5小时
**核心任务**:
1. 编写单元测试（关键组件）
2. 更新E2E测试
3. 性能优化（大数据量下的渲染性能）
4. 兼容性测试（不同浏览器）
5. 用户体验优化（动画流畅度、交互反馈）

**关键文件**:
- `front/tests/unit/TopicTimeline.spec.ts` (新建)
- `front/tests/unit/TopicGraphCanvas.spec.ts` (修改)
- `front/tests/e2e/topic-graph.spec.ts` (修改)

**验收标准**:
- [ ] 单元测试覆盖率>70%
- [ ] E2E测试全部通过
- [ ] 大数据量下（1000+ articles）渲染流畅
- [ ] 主流浏览器兼容（Chrome/Firefox/Safari）
- [ ] 动画流畅（60fps）

---

## 实施时间表

| 阶段 | 任务 | 预计耗时 | 依赖 |
|------|------|----------|------|
| 阶段一 | 拓扑图连线重构 | 4-6小时 | 无 |
| 阶段二 | 日报时间线区域 | 6-8小时 | 阶段一 |
| 阶段三 | 右侧栏精简 | 4-5小时 | 阶段二 |
| 阶段四 | AI分析功能 | 5-6小时 | 阶段二 |
| 阶段五 | 后端API调整 | 3-4小时 | 无（可并行） |
| 阶段六 | 测试与优化 | 4-5小时 | 阶段一至五 |
| **总计** | | **约36-44小时** | |

---

## 风险控制

### 技术风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 3D拓扑图性能问题 | 大数据量下卡顿 | 使用LOD、节点聚类、按需渲染 |
| 动画流畅度不足 | 用户体验差 | 使用requestAnimationFrame、硬件加速 |
| 浏览器兼容性问题 | 部分用户无法使用 | 渐进增强、Polyfill、降级方案 |

### 进度风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 阶段延迟 | 整体进度推迟 | 设置缓冲时间、优先级排序、必要时裁剪功能 |
| 需求变更 | 返工 | 充分沟通、快速原型验证、敏捷迭代 |

---

## 下一步行动

1. **确认实施计划**: 请审查本计划，确认是否符合预期
2. **资源准备**: 确保开发环境就绪，相关依赖已安装
3. **开始实施**: 按照阶段顺序，逐一完成各阶段任务
4. **定期同步**: 每完成一个阶段进行验收和反馈

**建议**: 考虑到工作量较大（约36-44小时），建议分阶段交付，每个阶段完成后进行验收，确保方向正确。

---

*方案制定时间: 2026-03-14*  
*版本: v1.0*